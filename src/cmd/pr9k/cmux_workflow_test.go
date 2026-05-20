package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/logger"
	"github.com/mxriverlynn/pr9k/src/internal/steps"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
	"github.com/mxriverlynn/pr9k/src/internal/workflow"
)

// --- test helpers ---

func mustNewTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(t.TempDir())
	if err != nil {
		t.Fatalf("create test logger: %v", err)
	}
	t.Cleanup(func() { _ = log.Close() })
	return log
}

// writeMinimalWorkflowConfig writes a config.json to dir with a single non-claude
// iteration step that runs "true" (always succeeds, empty output) and breaks the
// loop on empty capture. This lets runCmuxWorkflowAdapted complete in one iteration
// without requiring Docker or network access.
func writeMinimalWorkflowConfig(t *testing.T, dir string) {
	t.Helper()
	cfg := map[string]interface{}{
		"iteration": []map[string]interface{}{
			{
				"name":             "test-step",
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

// loadTestStepFile loads steps from a workflow dir created by writeMinimalWorkflowConfig.
func loadTestStepFile(t *testing.T, workflowDir string) steps.StepFile {
	t.Helper()
	sf, err := steps.LoadSteps(workflowDir)
	if err != nil {
		t.Fatalf("load steps: %v", err)
	}
	return sf
}

// setupWorkflowInProjectDir creates <projectDir>/.pr9k/workflow/ and writes a
// minimal config.json there so cli.ResolveWorkflowDir can find the bundle.
// Returns the workflow directory path.
func setupWorkflowInProjectDir(t *testing.T, projectDir string) string {
	t.Helper()
	workflowDir := filepath.Join(projectDir, ".pr9k", "workflow")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("create .pr9k/workflow: %v", err)
	}
	writeMinimalWorkflowConfig(t, workflowDir)
	return workflowDir
}

// injectAcksAfterWorkspaceDone starts a goroutine that polls
// fake.SentMessages() until WorkspaceDone is observed, then injects DoneAcks
// for all three display roles into fake.Recv(). This matches real-world timing
// where panes respond to WorkspaceDone after the workflow completes, not before.
// The goroutine exits after injecting or after timeout.
func injectAcksAfterWorkspaceDone(fake *interactionchannel.FakeInteractionChannel, timeout time.Duration) {
	go func() {
		deadline := time.NewTimer(timeout)
		defer deadline.Stop()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-deadline.C:
				return
			case <-ticker.C:
				for _, m := range fake.SentMessages() {
					if _, ok := m.(interactionchannel.WorkspaceDone); ok {
						fake.InjectMessage(interactionchannel.DoneAck{Role: "header"})
						fake.InjectMessage(interactionchannel.DoneAck{Role: "log"})
						fake.InjectMessage(interactionchannel.DoneAck{Role: "footer"})
						return
					}
				}
			}
		}
	}()
}

// injectTwoAcksAfterWorkspaceDone is like injectAcksAfterWorkspaceDone but
// injects only header and log DoneAcks — footer never acks. Used to exercise
// the ack-timeout path in awaitDoneAcks.
func injectTwoAcksAfterWorkspaceDone(fake *interactionchannel.FakeInteractionChannel, timeout time.Duration) {
	go func() {
		deadline := time.NewTimer(timeout)
		defer deadline.Stop()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-deadline.C:
				return
			case <-ticker.C:
				for _, m := range fake.SentMessages() {
					if _, ok := m.(interactionchannel.WorkspaceDone); ok {
						fake.InjectMessage(interactionchannel.DoneAck{Role: "header"})
						fake.InjectMessage(interactionchannel.DoneAck{Role: "log"})
						return
					}
				}
			}
		}
	}()
}

// --- cmuxHeader adapter tests ---

// TestCmuxHeader_SetPhaseSteps_SendsStateHeader verifies that SetPhaseSteps
// pushes a StateHeader message with the correct step names and zero-value states.
func TestCmuxHeader_SetPhaseSteps_SendsStateHeader(t *testing.T) {
	ch := interactionchannel.NewFakeInteractionChannel()
	h := newCmuxHeader(ch)
	h.SetPhaseSteps([]string{"step-a", "step-b"})

	msg, ok := ch.ExpectStatePush()
	if !ok {
		t.Fatal("expected StateHeader message after SetPhaseSteps")
	}
	hdr, ok := msg.(interactionchannel.StateHeader)
	if !ok {
		t.Fatalf("expected StateHeader, got %T", msg)
	}
	if len(hdr.StepNames) != 2 || hdr.StepNames[0] != "step-a" || hdr.StepNames[1] != "step-b" {
		t.Errorf("unexpected StepNames: %v", hdr.StepNames)
	}
	if len(hdr.StepStates) != 2 {
		t.Errorf("expected 2 step states, got %d", len(hdr.StepStates))
	}
	// All states should be zero (StepPending) after SetPhaseSteps.
	for i, s := range hdr.StepStates {
		if s != int(ui.StepPending) {
			t.Errorf("StepStates[%d] = %d, want StepPending (%d)", i, s, int(ui.StepPending))
		}
	}
}

// TestCmuxHeader_SetStepState_SendsUpdatedStateHeader verifies that SetStepState
// updates the state for the given index and pushes a StateHeader.
func TestCmuxHeader_SetStepState_SendsUpdatedStateHeader(t *testing.T) {
	ch := interactionchannel.NewFakeInteractionChannel()
	h := newCmuxHeader(ch)
	h.SetPhaseSteps([]string{"step-a"})
	ch.ExpectStatePush() // drain SetPhaseSteps push

	h.SetStepState(0, ui.StepActive)

	msg, ok := ch.ExpectStatePush()
	if !ok {
		t.Fatal("expected StateHeader after SetStepState")
	}
	hdr, ok := msg.(interactionchannel.StateHeader)
	if !ok {
		t.Fatalf("expected StateHeader, got %T", msg)
	}
	if len(hdr.StepStates) == 0 || hdr.StepStates[0] != int(ui.StepActive) {
		t.Errorf("expected StepStates[0] = StepActive (%d), got %v", int(ui.StepActive), hdr.StepStates)
	}
}

// TestCmuxHeader_SetStepState_OutOfBounds_NoPanic verifies that SetStepState
// with an out-of-bounds index is a no-op and does not panic.
func TestCmuxHeader_SetStepState_OutOfBounds_NoPanic(t *testing.T) {
	ch := interactionchannel.NewFakeInteractionChannel()
	h := newCmuxHeader(ch)
	h.SetPhaseSteps([]string{"step-a"})
	ch.ExpectStatePush()

	// Should not panic for out-of-bounds index.
	h.SetStepState(99, ui.StepDone)

	// A StateHeader is still pushed (with unchanged states).
	_, ok := ch.ExpectStatePush()
	if !ok {
		t.Fatal("expected StateHeader push even for out-of-bounds SetStepState")
	}
}

// TestCmuxHeader_RenderIterationLine_SendsStateHeader verifies that
// RenderIterationLine pushes a StateHeader with the formatted iteration line.
func TestCmuxHeader_RenderIterationLine_SendsStateHeader(t *testing.T) {
	ch := interactionchannel.NewFakeInteractionChannel()
	h := newCmuxHeader(ch)
	h.RenderIterationLine(2, 5, "42")

	msg, ok := ch.ExpectStatePush()
	if !ok {
		t.Fatal("expected StateHeader after RenderIterationLine")
	}
	hdr, ok := msg.(interactionchannel.StateHeader)
	if !ok {
		t.Fatalf("expected StateHeader, got %T", msg)
	}
	if !strings.Contains(hdr.IterationLine, "2") || !strings.Contains(hdr.IterationLine, "5") {
		t.Errorf("IterationLine %q should contain iter/max counts", hdr.IterationLine)
	}
}

// TestCmuxHeader_RenderIterationLine_UnboundedOmitsMax verifies that when
// maxIter == 0, the iteration line omits the "/max" suffix.
func TestCmuxHeader_RenderIterationLine_UnboundedOmitsMax(t *testing.T) {
	ch := interactionchannel.NewFakeInteractionChannel()
	h := newCmuxHeader(ch)
	h.RenderIterationLine(3, 0, "")

	msg, _ := ch.ExpectStatePush()
	hdr, ok := msg.(interactionchannel.StateHeader)
	if !ok {
		t.Fatalf("expected StateHeader, got %T", msg)
	}
	if strings.Contains(hdr.IterationLine, "/0") {
		t.Errorf("unbounded iteration line should not contain /0, got %q", hdr.IterationLine)
	}
}

// TestCmuxHeader_RenderInitializeLine_SendsStateHeader verifies that
// RenderInitializeLine pushes a StateHeader with the initialize-phase format.
func TestCmuxHeader_RenderInitializeLine_SendsStateHeader(t *testing.T) {
	ch := interactionchannel.NewFakeInteractionChannel()
	h := newCmuxHeader(ch)
	h.RenderInitializeLine(1, 3, "splash")

	msg, ok := ch.ExpectStatePush()
	if !ok {
		t.Fatal("expected StateHeader after RenderInitializeLine")
	}
	hdr, ok := msg.(interactionchannel.StateHeader)
	if !ok {
		t.Fatalf("expected StateHeader, got %T", msg)
	}
	if !strings.Contains(hdr.IterationLine, "splash") {
		t.Errorf("IterationLine %q should contain step name", hdr.IterationLine)
	}
}

// TestCmuxHeader_RenderFinalizeLine_SendsStateHeader verifies that
// RenderFinalizeLine pushes a StateHeader with the finalize-phase format.
func TestCmuxHeader_RenderFinalizeLine_SendsStateHeader(t *testing.T) {
	ch := interactionchannel.NewFakeInteractionChannel()
	h := newCmuxHeader(ch)
	h.RenderFinalizeLine(2, 3, "cleanup")

	msg, ok := ch.ExpectStatePush()
	if !ok {
		t.Fatal("expected StateHeader after RenderFinalizeLine")
	}
	hdr, ok := msg.(interactionchannel.StateHeader)
	if !ok {
		t.Fatalf("expected StateHeader, got %T", msg)
	}
	if !strings.Contains(hdr.IterationLine, "cleanup") {
		t.Errorf("IterationLine %q should contain step name", hdr.IterationLine)
	}
}

// --- newCmuxKeyHandler tests ---

// TestCmuxKeyHandler_CancelIsNonNil verifies that the KeyHandler constructed
// via newCmuxKeyHandler has a non-nil cancel function (D-4).
func TestCmuxKeyHandler_CancelIsNonNil(t *testing.T) {
	log := mustNewTestLogger(t)
	runner := workflow.NewRunner(log, t.TempDir())
	runner.SetSender(func(string) {})
	actions := make(chan ui.StepAction, 10)

	kh := newCmuxKeyHandler(runner, actions)

	if kh.Cancel() == nil {
		t.Error("KeyHandler cancel must be non-nil")
	}
}

// TestCmuxKeyHandler_CancelInvokesRunnerTerminate verifies that calling the
// cancel function from the constructed KeyHandler invokes runner.Terminate,
// which sets runner.WasTerminated() to true (D-4).
func TestCmuxKeyHandler_CancelInvokesRunnerTerminate(t *testing.T) {
	log := mustNewTestLogger(t)
	runner := workflow.NewRunner(log, t.TempDir())
	runner.SetSender(func(string) {})
	actions := make(chan ui.StepAction, 10)

	kh := newCmuxKeyHandler(runner, actions)

	kh.Cancel()() // retrieve and call the cancel function
	if !runner.WasTerminated() {
		t.Error("cancel() must invoke runner.Terminate; WasTerminated() is false")
	}
}

// --- runCmuxWorkflowAdapted integration tests ---

// TestRunCmuxWorkflowAdapted_InitialModeNormalFooterSent verifies that
// runCmuxWorkflowAdapted sends at least one StateFooter with ModeNormal before
// the workflow begins (D-6 prime-the-channel).
func TestRunCmuxWorkflowAdapted_InitialModeNormalFooterSent(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeMinimalWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()

	runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil)

	for _, m := range ch.SentMessages() {
		if footer, ok := m.(interactionchannel.StateFooter); ok {
			if footer.Mode == int(ui.ModeNormal) {
				return
			}
		}
	}
	t.Error("expected at least one StateFooter with ModeNormal among sent messages")
}

// TestRunCmuxWorkflowAdapted_StateHeaderSent verifies that at least one
// StateHeader message is pushed to the channel during the workflow run.
func TestRunCmuxWorkflowAdapted_StateHeaderSent(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeMinimalWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()

	runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil)

	for _, m := range ch.SentMessages() {
		if _, ok := m.(interactionchannel.StateHeader); ok {
			return
		}
	}
	t.Error("expected at least one StateHeader message")
}

// TestRunCmuxWorkflowAdapted_StateLogSent verifies that at least one StateLog
// message (e.g. the step-start banner written by WriteToLog) is sent.
func TestRunCmuxWorkflowAdapted_StateLogSent(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeMinimalWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()

	runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil)

	for _, m := range ch.SentMessages() {
		if _, ok := m.(interactionchannel.StateLog); ok {
			return
		}
	}
	t.Error("expected at least one StateLog message")
}

// TestRunCmuxWorkflowAdapted_ModeDoneSentAfterWorkflow verifies that the last
// StateFooter pushed has Mode == ModeDone (set after workflow.Run returns).
func TestRunCmuxWorkflowAdapted_ModeDoneSentAfterWorkflow(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeMinimalWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()

	runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil)

	msgs := ch.SentMessages()
	var lastFooter interactionchannel.StateFooter
	var hasFooter bool
	for _, m := range msgs {
		if footer, ok := m.(interactionchannel.StateFooter); ok {
			lastFooter = footer
			hasFooter = true
		}
	}
	if !hasFooter {
		t.Fatal("no StateFooter messages found")
	}
	if lastFooter.Mode != int(ui.ModeDone) {
		t.Errorf("last StateFooter.Mode = %d (%s), want ModeDone (%d)",
			lastFooter.Mode, lastFooter.ShortcutLine, int(ui.ModeDone))
	}
}

// TestRunCmuxWorkflowAdapted_InitialFooterBeforeFirstHeader verifies that the
// initial ModeNormal StateFooter prime (D-6) appears in the sent-message queue
// before the first StateHeader from the workflow (ordering assertion).
func TestRunCmuxWorkflowAdapted_InitialFooterBeforeFirstHeader(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeMinimalWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()

	runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil)

	msgs := ch.SentMessages()
	firstFooterIdx := -1
	firstHeaderIdx := -1
	for i, m := range msgs {
		switch m.(type) {
		case interactionchannel.StateFooter:
			if firstFooterIdx < 0 {
				firstFooterIdx = i
			}
		case interactionchannel.StateHeader:
			if firstHeaderIdx < 0 {
				firstHeaderIdx = i
			}
		}
	}
	if firstFooterIdx < 0 {
		t.Fatal("no StateFooter found")
	}
	if firstHeaderIdx < 0 {
		t.Fatal("no StateHeader found")
	}
	if firstFooterIdx >= firstHeaderIdx {
		t.Errorf("first StateFooter (idx %d) must appear before first StateHeader (idx %d)",
			firstFooterIdx, firstHeaderIdx)
	}
}

// TestRunCmuxWorkflowAdapted_WriteToLogSynchronous verifies that WriteToLog
// delivers a StateLog synchronously on the caller goroutine (D-5): the step
// start banner ("Starting step:..."), written via WriteToLog before the step
// subprocess runs, appears as a StateLog before any step-subprocess output.
func TestRunCmuxWorkflowAdapted_WriteToLogSynchronous(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeMinimalWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()

	runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil)

	// The step-start banner is written by Orchestrate via runner.WriteToLog before
	// the subprocess starts. It must appear as a StateLog in the sent messages.
	found := false
	for _, m := range ch.SentMessages() {
		sl, ok := m.(interactionchannel.StateLog)
		if !ok {
			continue
		}
		for _, line := range sl.Lines {
			if strings.Contains(string(line), "Starting step") {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Error("WriteToLog must deliver 'Starting step' banner as StateLog synchronously; not found in sent messages")
	}
}

// TestRunCmuxWorkflowAdapted_ContextCancelStopsKeyGoroutine verifies that a
// cancelled context causes runCmuxWorkflowAdapted to return without hanging
// (the key-adapter goroutine is drained via WaitGroup on context cancel).
func TestRunCmuxWorkflowAdapted_ContextCancelStopsKeyGoroutine(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeMinimalWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately — goroutine must not block the function

	done := make(chan struct{})
	go func() {
		defer close(done)
		runCmuxWorkflowAdapted(ctx, ch, log, projectDir, workflowDir, sf, nil)
	}()

	select {
	case <-done:
		// correct: returned without hanging
	case <-time.After(5 * time.Second):
		t.Fatal("runCmuxWorkflowAdapted did not return after context cancel (key goroutine leak?)")
	}
}

// TestRunCmuxWorkflowAdapted_ReturnsZeroOnSuccess verifies that a successful
// workflow run returns exit code 0.
func TestRunCmuxWorkflowAdapted_ReturnsZeroOnSuccess(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeMinimalWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()

	code := runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil)
	if code != 0 {
		t.Errorf("expected exit code 0 on successful workflow, got %d", code)
	}
}

// writeFailingWorkflowConfig writes a config.json with a single non-claude
// iteration step that always fails ("false"). No captureAs / breakLoopIfEmpty,
// so the workflow enters error mode and blocks on h.Actions.
func writeFailingWorkflowConfig(t *testing.T, dir string) {
	t.Helper()
	cfg := map[string]interface{}{
		"iteration": []map[string]interface{}{
			{
				"name":     "fail-step",
				"isClaude": false,
				"command":  []string{"false"},
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

// --- T-1: Intent → StepAction routing ---

// TestKeyAdapterLoop_IntentToStepAction verifies that the four intent kinds
// (Retry, Continue, Next, Skip) are each translated to the matching StepAction
// by keyAdapterLoop. The goroutine is wired directly with a controlled actions
// channel so the routing table can be asserted without going through workflow.Run.
func TestKeyAdapterLoop_IntentToStepAction(t *testing.T) {
	cases := []struct {
		intent interactionchannel.IntentType
		want   ui.StepAction
	}{
		{interactionchannel.IntentRetry, ui.ActionRetry},
		{interactionchannel.IntentContinue, ui.ActionContinue},
		{interactionchannel.IntentNext, ui.ActionNext},
		{interactionchannel.IntentSkip, ui.ActionSkip},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.intent), func(t *testing.T) {
			ch := interactionchannel.NewFakeInteractionChannel()
			actions := make(chan ui.StepAction, 10)
			log := mustNewTestLogger(t)
			runner := workflow.NewRunner(log, t.TempDir())
			kh := newCmuxKeyHandler(runner, actions)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				keyAdapterLoop(ctx, ch, actions, kh)
			}()

			ch.InjectMessage(interactionchannel.Intent{Kind: tc.intent})

			select {
			case got := <-actions:
				if got != tc.want {
					t.Errorf("intent %v: got StepAction %v, want %v", tc.intent, got, tc.want)
				}
			case <-time.After(time.Second):
				t.Fatalf("intent %v: timeout waiting for StepAction", tc.intent)
			}
		})
	}
}

// --- T-2: IntentQuit → ExitReasonUserQuit → exit code 1 ---

// TestRunCmuxWorkflowAdapted_IntentQuitReturnsOne verifies that injecting
// IntentQuit while the workflow is running causes runCmuxWorkflowAdapted to
// return exit code 1. A failing step is used so the workflow blocks in error
// mode on h.Actions, giving the key-adapter goroutine time to forward the quit.
func TestRunCmuxWorkflowAdapted_IntentQuitReturnsOne(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeFailingWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()

	// Pre-inject IntentQuit. The key-adapter goroutine forwards it to
	// keyHandler.ForceQuit(), which injects ActionQuit into h.Actions. The
	// failing step blocks in error mode on h.Actions, so the quit is picked up
	// deterministically regardless of goroutine scheduling.
	ch.InjectMessage(interactionchannel.Intent{Kind: interactionchannel.IntentQuit})

	done := make(chan int, 1)
	go func() {
		done <- runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil)
	}()

	select {
	case code := <-done:
		if code != 1 {
			t.Errorf("IntentQuit: expected exit code 1, got %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runCmuxWorkflowAdapted did not return after IntentQuit (possible goroutine leak)")
	}
}
