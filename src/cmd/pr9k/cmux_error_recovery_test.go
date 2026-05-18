package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// pollModeErrorCount blocks until at least n StateFooter(ModeError) messages
// appear in ch.SentMessages(), returning true, or until timeout elapses (false).
// Used to coordinate intent injection with the orchestrator's error-mode state.
func pollModeErrorCount(ch *interactionchannel.FakeInteractionChannel, n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		count := 0
		for _, m := range ch.SentMessages() {
			if footer, ok := m.(interactionchannel.StateFooter); ok {
				if ui.Mode(footer.Mode) == ui.ModeError {
					count++
				}
			}
		}
		if count >= n {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// TestCmuxErrorRecovery_RoundTrip is the W-5 round-trip integration test for
// Phase 3 interactive error recovery. It exercises the wired composition
// (runCmuxWorkflowAdapted + FakeInteractionChannel) end-to-end across four
// sub-cases covering spec D1–D3, RAID R4, and the D4 structural invariant.
//
// Phase 2's isolated unit tests (TestDisplayPane_*, TestAdapter_*, etc.) remain
// unchanged and cover the component-level guarantees; this test validates the
// wired composition only. No new test double is introduced; all doubles are the
// Phase 2 FakeInteractionChannel (mutex-protected) and helpers from
// cmux_workflow_test.go.
func TestCmuxErrorRecovery_RoundTrip(t *testing.T) {

	// Sub-case 1: On a step failure the orchestrator enters error mode (spec D1),
	// pushes StateFooter(ModeError), and marks the failing step [✗] in StateHeader
	// (spec D2). An intent injected the instant ModeError is observed (race-window)
	// is not dropped — the orchestrator acts on it once in error mode (spec D1).
	//
	// The component-level race-window guarantee is covered by Phase 2's
	// TestAdapter_ModeErrorRaceWindow_NoDroppedKeystroke (reused unchanged); this
	// sub-case covers the same guarantee at the wired-composition level: we wait
	// for StateFooter(ModeError) then inject IntentQuit, verifying the orchestrator
	// consumes it and returns exit code 1.
	t.Run("error-mode-and-StepFailed-on-failure", func(t *testing.T) {
		projectDir := t.TempDir()
		workflowDir := t.TempDir()
		writeFailingWorkflowConfig(t, workflowDir)
		sf := loadTestStepFile(t, workflowDir)
		log := mustNewTestLogger(t)
		ch := interactionchannel.NewFakeInteractionChannel()

		done := make(chan int, 1)
		go func() {
			done <- runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf)
		}()

		// Wait for the first StateFooter(ModeError) — the step has failed and the
		// orchestrator is blocking on <-h.Actions (error-mode race window opens).
		if !pollModeErrorCount(ch, 1, 5*time.Second) {
			t.Fatal("StateFooter(ModeError) did not appear within 5s; orchestrator did not enter error mode")
		}

		// Inject IntentQuit into the open race window. The orchestrator must act
		// on it and return exit code 1 (spec D1 no-dropped-choice guarantee).
		ch.InjectMessage(interactionchannel.Intent{Kind: interactionchannel.IntentQuit})

		select {
		case code := <-done:
			if code != 1 {
				t.Errorf("expected exit code 1 (IntentQuit acted on in error mode), got %d", code)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("runCmuxWorkflowAdapted did not return after IntentQuit")
		}

		msgs := ch.SentMessages()

		// D1: StateFooter(ModeError) was confirmed present by the poll above.
		// Double-check it appears in sent messages (belt-and-suspenders).
		sawModeError := false
		for _, m := range msgs {
			if footer, ok := m.(interactionchannel.StateFooter); ok {
				if ui.Mode(footer.Mode) == ui.ModeError {
					sawModeError = true
					break
				}
			}
		}
		if !sawModeError {
			t.Error("D1: StateFooter(ModeError) not found in sent messages; orchestrator did not enter error mode on step failure")
		}

		// D2: StateHeader with StepFailed state must be pushed when a step fails
		// (header marks the failing step [✗]).
		sawStepFailed := false
	outer1:
		for _, m := range msgs {
			if hdr, ok := m.(interactionchannel.StateHeader); ok {
				for _, s := range hdr.StepStates {
					if ui.StepState(s) == ui.StepFailed {
						sawStepFailed = true
						break outer1
					}
				}
			}
		}
		if !sawStepFailed {
			t.Error("D2: StateHeader with StepFailed state not found; header did not mark failing step [✗]")
		}
	})

	// Sub-case 2: Retry path — separator-before-output invariant (spec D3) and
	// re-failure re-entering error mode (spec D2 re-marks [✗], spec D1 re-shows prompt).
	//
	// Sequence: wait for first ModeError → inject IntentRetry → wait for second
	// ModeError (re-failure) → inject IntentQuit. Intents are injected only after
	// the orchestrator is known to be blocking on <-h.Actions, avoiding the
	// pre-step quit-check race described in orchestrate.go.
	t.Run("retry-separator-and-refailure", func(t *testing.T) {
		projectDir := t.TempDir()
		workflowDir := t.TempDir()
		writeFailingWorkflowConfig(t, workflowDir)
		sf := loadTestStepFile(t, workflowDir)
		log := mustNewTestLogger(t)
		ch := interactionchannel.NewFakeInteractionChannel()

		done := make(chan int, 1)
		go func() {
			done <- runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf)
		}()

		// Wait for first StateFooter(ModeError) then inject IntentRetry.
		// The synchronous WriteToLog→SendStateLog adapter (plan D-5) guarantees the
		// retry separator is enqueued before any retried-step output.
		if !pollModeErrorCount(ch, 1, 5*time.Second) {
			t.Fatal("first StateFooter(ModeError) did not appear within 5s")
		}
		ch.InjectMessage(interactionchannel.Intent{Kind: interactionchannel.IntentRetry})

		// Wait for the second StateFooter(ModeError) (re-failure after retry).
		if !pollModeErrorCount(ch, 2, 5*time.Second) {
			t.Fatal("second StateFooter(ModeError) did not appear within 5s; re-failure did not occur")
		}
		ch.InjectMessage(interactionchannel.Intent{Kind: interactionchannel.IntentQuit})

		select {
		case code := <-done:
			if code != 1 {
				t.Errorf("expected exit code 1 (IntentQuit after re-failure), got %d", code)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("runCmuxWorkflowAdapted did not return after retry+quit")
		}

		msgs := ch.SentMessages()

		// D3: RetryStepSeparator must appear in StateLog messages.
		// RetryStepSeparator("fail-step") contains "(retry)".
		separatorIdx := -1
	outer2:
		for i, m := range msgs {
			if sl, ok := m.(interactionchannel.StateLog); ok {
				for _, line := range sl.Lines {
					if strings.Contains(string(line), "(retry)") {
						separatorIdx = i
						break outer2
					}
				}
			}
		}
		if separatorIdx < 0 {
			t.Error("D3: retry separator not found in StateLog messages; WriteToLog(RetryStepSeparator) must be called on retry")
		}

		// D2 re-failure: at least two StepFailed StateHeaders must be pushed.
		var stepFailedIndices []int
		for i, m := range msgs {
			if hdr, ok := m.(interactionchannel.StateHeader); ok {
				for _, s := range hdr.StepStates {
					if ui.StepState(s) == ui.StepFailed {
						stepFailedIndices = append(stepFailedIndices, i)
						break
					}
				}
			}
		}
		if len(stepFailedIndices) < 2 {
			t.Errorf("D2 re-failure: expected at least 2 StepFailed StateHeaders (initial + re-failure), got %d", len(stepFailedIndices))
		}

		// D3 separator-before-output: the separator StateLog must appear before
		// the second StepFailed StateHeader. The synchronous WriteToLog adapter
		// guarantees this ordering in the sent-message queue (plan D-5).
		if separatorIdx >= 0 && len(stepFailedIndices) >= 2 {
			if separatorIdx >= stepFailedIndices[1] {
				t.Errorf("D3: separator (msg idx %d) must appear before second StepFailed StateHeader (msg idx %d); separator-before-output ordering violated",
					separatorIdx, stepFailedIndices[1])
			}
		}

		// D1 re-failure: StateFooter(ModeError) must appear at least twice.
		modeErrorCount := 0
		for _, m := range msgs {
			if footer, ok := m.(interactionchannel.StateFooter); ok {
				if ui.Mode(footer.Mode) == ui.ModeError {
					modeErrorCount++
				}
			}
		}
		if modeErrorCount < 2 {
			t.Errorf("D1 re-failure: expected at least 2 StateFooter(ModeError) messages (initial + re-failure), got %d", modeErrorCount)
		}
	})

	// Sub-case 3: Quit-path transient StateFooter(ModeNormal) flash (plan RAID R4).
	// Orchestrate calls h.SetMode(ModeNormal) after consuming any action from h.Actions,
	// regardless of action type (orchestrate.go:runStepWithErrorHandling). On the
	// quit path this pushes a brief StateFooter(ModeNormal) before WorkspaceDone.
	// This is accepted as non-blocking (spec D4/parent T4; plan RAID R4); the on-disk
	// artifact is authoritative.
	//
	// Assertion: the transient flash does not prevent clean workflow termination —
	// the last StateFooter pushed by the orchestrator must be ModeDone.
	t.Run("quit-path-ModeNormal-flash-accepted", func(t *testing.T) {
		projectDir := t.TempDir()
		workflowDir := t.TempDir()
		writeFailingWorkflowConfig(t, workflowDir)
		sf := loadTestStepFile(t, workflowDir)
		log := mustNewTestLogger(t)
		ch := interactionchannel.NewFakeInteractionChannel()

		done := make(chan int, 1)
		go func() {
			done <- runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf)
		}()

		// Wait for first StateFooter(ModeError) then quit — same race window as sub-case 1.
		if !pollModeErrorCount(ch, 1, 5*time.Second) {
			t.Fatal("StateFooter(ModeError) did not appear within 5s")
		}
		ch.InjectMessage(interactionchannel.Intent{Kind: interactionchannel.IntentQuit})

		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("runCmuxWorkflowAdapted did not return")
		}

		// The transient ModeNormal flash (if present) must not prevent the
		// orchestrator from pushing a final StateFooter(ModeDone). The last
		// StateFooter in sent messages must be ModeDone.
		var lastFooterMode = -1
		for _, m := range ch.SentMessages() {
			if footer, ok := m.(interactionchannel.StateFooter); ok {
				lastFooterMode = footer.Mode
			}
		}
		if lastFooterMode != int(ui.ModeDone) {
			t.Errorf("RAID R4: last StateFooter.Mode = %d, want ModeDone (%d); transient ModeNormal flash must not prevent clean termination",
				lastFooterMode, int(ui.ModeDone))
		}
	})

	// Sub-case 4: D4 structural invariant — the orchestrator never sends Intent
	// messages outbound to the interaction channel. Intents flow inbound only
	// (footer pane → orchestrator). Control keys pressed on display panes produce
	// no outbound Intent; this is the orchestrator-side view of that guarantee.
	//
	// The pane-side D4 guarantee is covered by Phase 2's
	// TestDisplayPane_HeaderPane_HasNoKeyPath and TestDisplayPane_LogPane_HasNoKeyPath,
	// which are reused unchanged.
	t.Run("orchestrator-sends-no-Intent", func(t *testing.T) {
		projectDir := t.TempDir()
		workflowDir := t.TempDir()
		writeMinimalWorkflowConfig(t, workflowDir)
		sf := loadTestStepFile(t, workflowDir)
		log := mustNewTestLogger(t)
		ch := interactionchannel.NewFakeInteractionChannel()

		runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf)

		// The orchestrator must never send Intent messages — Intents are inbound
		// (footer pane → orchestrator), not outbound. Every sent message must be
		// a state push (StateHeader, StateLog, StateFooter) or WorkspaceDone.
		for _, m := range ch.SentMessages() {
			if _, ok := m.(interactionchannel.Intent); ok {
				t.Errorf("D4: orchestrator sent an unexpected Intent message %v; orchestrator must only send state pushes", m)
			}
		}
	})
}
