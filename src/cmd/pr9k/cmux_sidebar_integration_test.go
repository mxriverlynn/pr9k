package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
	"github.com/mxriverlynn/pr9k/src/internal/workflow"
)

// writeTwoIterOneFinalizeConfig writes a config.json with two non-claude
// iteration steps and one non-claude finalization step. No breakLoopIfEmpty —
// the loop exits after exactly Iterations runs.
func writeTwoIterOneFinalizeConfig(t *testing.T, dir string) {
	t.Helper()
	cfg := map[string]interface{}{
		"iteration": []map[string]interface{}{
			{"name": "iter-step-1", "isClaude": false, "command": []string{"true"}},
			{"name": "iter-step-2", "isClaude": false, "command": []string{"true"}},
		},
		"finalize": []map[string]interface{}{
			{"name": "finalize-step", "isClaude": false, "command": []string{"true"}},
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

// TestSidebarCallSequence_FullWorkflow is the W-7 round-trip integration test.
// It drives the full sidebar call sequence through a two-iteration workflow with
// one finalization step and asserts the ordered recorder-slice entries:
//
//  1. First SetStatusCalls entry: first iteration step name ("iter-step-1").
//  2. First SetProgressCalls entry: fraction 0.5, label "1 / 2".
//  3. Second SetProgressCalls entry: fraction 1.0, label "2 / 2".
//  4. SetStatusCalls advances through both iterations (iter-step-2 from iter 1,
//     iter-step-1 again from iter 2).
//  5. ClearProgressCalls non-empty before ClearAll is called (triggered at first
//     RenderFinalizeLine inside workflow.Run).
//  6. ClearStatusCalls non-empty after ClearAll (graceful-path cleanup).
func TestSidebarCallSequence_FullWorkflow(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeTwoIterOneFinalizeConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)
	log := mustNewTestLogger(t)

	fc := &cmuxctl.FakeClient{}
	ws := cmuxctl.Workspace{ID: "ws:test"}
	sidebar := newCmuxSidebar(fc, ws, log)

	ch := interactionchannel.NewFakeInteractionChannel()
	inner := newCmuxHeader(ch)
	h := &sidebarAwareHeader{inner: inner, sidebar: sidebar, ctx: context.Background()}

	runner := workflow.NewRunner(log, projectDir)
	runner.SetSender(func(line string) {
		ch.SendStateLog(interactionchannel.StateLog{Lines: [][]byte{[]byte(line)}})
	})

	actions := make(chan ui.StepAction, 10)
	keyHandler := newCmuxKeyHandler(runner, actions)
	keyHandler.SetOnModeChange(func(mode ui.Mode, line string) {
		ch.SendStateFooter(interactionchannel.StateFooter{Mode: int(mode), ShortcutLine: line})
	})

	runCfg := workflow.RunConfig{
		WorkflowDir:     workflowDir,
		Iterations:      2,
		Env:             sf.Env,
		ContainerEnv:    sf.ContainerEnv,
		InitializeSteps: sf.Initialize,
		Steps:           sf.Iteration,
		FinalizeSteps:   sf.Finalize,
		RunStamp:        log.RunStamp(),
	}

	workflow.Run(runner, h, keyHandler, runCfg)

	// Snapshot ClearProgressCalls length before calling ClearAll. A non-zero
	// value proves ClearProgress fired inside workflow.Run (finalization phase),
	// which is structurally before the ClearStatus recorded by ClearAll below.
	clearProgressBeforeClearAll := len(fc.ClearProgressCalls)

	// Mirror the graceful-path D-6 cleanup from runCmuxWorkflowAdapted.
	_ = sidebar.ClearAll(context.Background())

	// 1. First SetStatus entry must be the first iteration step.
	if len(fc.SetStatusCalls) == 0 {
		t.Fatal("SetStatusCalls: expected entries, got none")
	}
	if fc.SetStatusCalls[0].Value != "iter-step-1" {
		t.Errorf("SetStatusCalls[0].Value = %q, want %q", fc.SetStatusCalls[0].Value, "iter-step-1")
	}

	// 2–3. SetProgress entries for both iterations.
	if len(fc.SetProgressCalls) < 2 {
		t.Fatalf("SetProgressCalls: expected at least 2, got %d", len(fc.SetProgressCalls))
	}
	if fc.SetProgressCalls[0].Fraction != 0.5 || fc.SetProgressCalls[0].Label != "1 / 2" {
		t.Errorf("SetProgressCalls[0] = {Fraction: %v, Label: %q}, want {0.5, \"1 / 2\"}",
			fc.SetProgressCalls[0].Fraction, fc.SetProgressCalls[0].Label)
	}
	if fc.SetProgressCalls[1].Fraction != 1.0 || fc.SetProgressCalls[1].Label != "2 / 2" {
		t.Errorf("SetProgressCalls[1] = {Fraction: %v, Label: %q}, want {1.0, \"2 / 2\"}",
			fc.SetProgressCalls[1].Fraction, fc.SetProgressCalls[1].Label)
	}

	// 4. SetStatusCalls advances through both iterations: iter-step-2 must appear
	// after index 0, then iter-step-1 must reappear (iter 2 started).
	foundIterStep2 := false
	foundIterStep1Again := false
	for _, c := range fc.SetStatusCalls[1:] {
		if !foundIterStep2 && c.Value == "iter-step-2" {
			foundIterStep2 = true
			continue
		}
		if foundIterStep2 && c.Value == "iter-step-1" {
			foundIterStep1Again = true
			break
		}
	}
	if !foundIterStep2 {
		t.Errorf("SetStatusCalls: iter-step-2 not found after index 0; calls: %v", fc.SetStatusCalls)
	}
	if !foundIterStep1Again {
		t.Errorf("SetStatusCalls: iter-step-1 did not reappear after iter-step-2 (both iterations not covered); calls: %v", fc.SetStatusCalls)
	}

	// 5. ClearProgress must have fired inside workflow.Run (at first RenderFinalizeLine)
	// before ClearAll was called — asserting triggering condition occurred first.
	if clearProgressBeforeClearAll == 0 {
		t.Error("ClearProgressCalls: expected at least one entry from finalization before ClearAll; got none — RenderFinalizeLine did not trigger ClearProgress")
	}

	// 6. ClearAll must have produced a ClearStatus call (graceful-path cleanup).
	if len(fc.ClearStatusCalls) == 0 {
		t.Error("ClearStatusCalls: expected entry from graceful ClearAll, got none")
	}
}
