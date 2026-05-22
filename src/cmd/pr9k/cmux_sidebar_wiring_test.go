package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
)

// writeTwoStepConfig writes a config.json with two iteration steps: the first
// always fails ("false", no captureAs) so the workflow enters error mode, and
// the second always succeeds ("true", captureAs+breakLoopIfEmpty) so after a
// continue action the loop exits gracefully.
func writeTwoStepConfig(t *testing.T, dir string) {
	t.Helper()
	cfg := map[string]interface{}{
		"iteration": []map[string]interface{}{
			{
				"name":     "fail-step",
				"isClaude": false,
				"command":  []string{"false"},
			},
			{
				"name":             "ok-step",
				"isClaude":         false,
				"command":          []string{"true"},
				"captureAs":        "RESULT",
				"breakLoopIfEmpty": true,
			},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
}

// TestRunCmuxWorkflowAdapted_ErrorModeMarker verifies end-to-end wiring of the
// sidebar adapter into runCmuxWorkflowAdapted (W-5):
//
//  1. sidebarAwareHeader is passed to workflow.Run (not bare cmuxHeader), so
//     step transitions drive sidebar.PushStep via SetStepState(StepActive).
//  2. The augmented SetOnModeChange closure calls sidebar.EnterErrorMode when
//     the orchestrator enters ModeError, pushing the error-mode marker.
//  3. A subsequent step activation (ok-step) calls sidebar.PushStep and
//     overwrites the error-mode marker value.
//  4. sidebar.ClearAll is called after workflow.Run returns (graceful-path D-6),
//     producing a ClearStatusCall.
func TestRunCmuxWorkflowAdapted_ErrorModeMarker(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeTwoStepConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)
	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()
	fc := &cmuxctl.FakeClient{}
	ws := validWorkspace()
	sidebar := mustNewTestSidebar(t, fc, ws)

	done := make(chan int, 1)
	go func() {
		code, _ := runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, sidebar, nil, nil, nil)
		done <- code
	}()

	// Wait for ModeError: fail-step has run and the orchestrator is blocking on
	// h.Actions. This is the same polling pattern used in cmux_error_recovery_test.go.
	if !pollModeErrorCount(ch, 1, 5*time.Second) {
		t.Fatal("StateFooter(ModeError) did not appear within 5s; orchestrator did not enter error mode")
	}

	// Inject continue — advances past the failing step to ok-step, which
	// succeeds and breaks the loop, causing workflow.Run to return.
	ch.InjectMessage(interactionchannel.Intent{Kind: interactionchannel.IntentContinue})

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("expected exit code 0 after graceful completion, got %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runCmuxWorkflowAdapted did not return after IntentContinue")
	}

	calls := fc.SetStatusCalls

	// Assert that the error-mode marker appeared in SetStatusCalls.
	errorMarkerIdx := -1
	for i, c := range calls {
		if strings.HasSuffix(c.Value, errorModeSuffix) {
			errorMarkerIdx = i
			break
		}
	}
	if errorMarkerIdx < 0 {
		t.Errorf("no SetStatusCall with errorModeSuffix %q found; calls: %v", errorModeSuffix, calls)
	}

	// Assert that a subsequent SetStatusCall (ok-step activation) appears after
	// the error-mode marker, confirming the overwrite path.
	if errorMarkerIdx >= 0 {
		foundOverwrite := false
		for i := errorMarkerIdx + 1; i < len(calls); i++ {
			if !strings.HasSuffix(calls[i].Value, errorModeSuffix) {
				foundOverwrite = true
				break
			}
		}
		if !foundOverwrite {
			t.Errorf("no SetStatusCall after the error-mode marker; subsequent step push should overwrite it; calls: %v", calls)
		}
	}

	// Assert that ClearAll was called after workflow.Run returned (D-6 graceful cleanup).
	if len(fc.ClearStatusCalls) == 0 {
		t.Error("ClearAll must call WorkspaceClearStatus after workflow.Run returns; no ClearStatusCall found")
	}
}
