package main

// W-4 integration tests: orchestrator wiring — liveness emitter and abort routing.
//
// These tests verify:
//   - The abort gate triggers the full 5-step abort sequence exactly once
//   - The liveness emitter sends Liveness{} heartbeats at the configured cadence
//   - Concurrent gate triggers produce exactly one FireRunAborted notification
//   - A trigger after runCompleted.done() is a no-op (detection-path guard)
//   - The abort path returns normally (deferred cleanup runs; no os.Exit)

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// TestW4_AbortGateTrigger_FiresAbortSequence verifies the 5-step abort sequence
// (D-4, D-11) fires when the gate is triggered while a workflow is blocked in
// error mode. Expects: return (1, true), WorkspaceDone{Aborted:true} broadcast,
// and exactly one NotificationRunAborted in the FakeClient.
func TestW4_AbortGateTrigger_FiresAbortSequence(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeFailingWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()
	fc := &cmuxctl.FakeClient{}
	notifier, _ := newTestNotifier(t, fc, projectDir)

	gate := newAbortGate()
	completed := &runCompleted{}

	done := make(chan struct {
		code    int
		aborted bool
	}, 1)
	go func() {
		code, aborted := runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil, notifier, gate, completed)
		done <- struct {
			code    int
			aborted bool
		}{code, aborted}
	}()

	// Wait for error mode (workflow blocked), then trigger the gate.
	if !pollModeErrorCount(ch, 1, 5*time.Second) {
		t.Fatal("ModeError did not appear; workflow did not enter error mode")
	}
	gate.trigger(AbortCauseStallDetected)

	var result struct {
		code    int
		aborted bool
	}
	select {
	case result = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("runCmuxWorkflowAdapted did not return after abort gate trigger")
	}

	if result.code != 1 {
		t.Errorf("abort path: expected exit code 1, got %d", result.code)
	}
	if !result.aborted {
		t.Error("abort path: expected aborted=true")
	}

	// WorkspaceDone{Aborted:true} must be in the sent messages.
	var abortDone int
	for _, m := range ch.SentMessages() {
		if wd, ok := m.(interactionchannel.WorkspaceDone); ok && wd.Aborted {
			abortDone++
		}
	}
	if abortDone != 1 {
		t.Errorf("expected exactly 1 WorkspaceDone{Aborted:true}, got %d", abortDone)
	}

	// FireRunAborted produces exactly one NotificationRunAborted.
	notifier.awaitStopped()
	var runAbortedCount int
	for _, c := range fc.NotifyCallsSnapshot() {
		if c.Class == cmuxctl.NotificationRunAborted {
			runAbortedCount++
		}
	}
	if runAbortedCount != 1 {
		t.Errorf("expected exactly 1 NotificationRunAborted, got %d", runAbortedCount)
	}
}

// TestW4_LivenessEmitter_SendsLiveness verifies that runLivenessEmitter
// broadcasts at least one Liveness{} message at the given cadence and stops
// cleanly when the context is cancelled.
func TestW4_LivenessEmitter_SendsLiveness(t *testing.T) {
	ch := interactionchannel.NewFakeInteractionChannel()
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runLivenessEmitter(ctx, ch, 20*time.Millisecond)
	}()

	// Wait long enough for multiple ticks.
	time.Sleep(80 * time.Millisecond)
	cancel()
	wg.Wait()

	var count int
	for _, m := range ch.SentMessages() {
		if _, ok := m.(interactionchannel.Liveness); ok {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected at least 2 Liveness messages, got %d", count)
	}
}

// TestW4_ConcurrentGateTriggers_ExactlyOneAbort verifies that concurrent
// gate.trigger calls from multiple goroutines produce exactly one
// WorkspaceDone{Aborted:true} broadcast and exactly one FireRunAborted
// notification (-race safe).
func TestW4_ConcurrentGateTriggers_ExactlyOneAbort(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeFailingWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()
	fc := &cmuxctl.FakeClient{}
	notifier, _ := newTestNotifier(t, fc, projectDir)

	gate := newAbortGate()
	completed := &runCompleted{}

	done := make(chan struct {
		code    int
		aborted bool
	}, 1)
	go func() {
		code, aborted := runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil, notifier, gate, completed)
		done <- struct {
			code    int
			aborted bool
		}{code, aborted}
	}()

	// Wait for error mode, then fire 10 concurrent triggers.
	if !pollModeErrorCount(ch, 1, 5*time.Second) {
		t.Fatal("ModeError did not appear")
	}
	var triggerWg sync.WaitGroup
	for range 10 {
		triggerWg.Add(1)
		go func() {
			defer triggerWg.Done()
			gate.trigger(AbortCauseStallDetected)
		}()
	}
	triggerWg.Wait()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("runCmuxWorkflowAdapted did not return")
	}

	// Exactly one WorkspaceDone{Aborted:true}.
	var abortDone int
	for _, m := range ch.SentMessages() {
		if wd, ok := m.(interactionchannel.WorkspaceDone); ok && wd.Aborted {
			abortDone++
		}
	}
	if abortDone != 1 {
		t.Errorf("concurrent triggers: expected 1 WorkspaceDone{Aborted:true}, got %d", abortDone)
	}

	// Exactly one FireRunAborted.
	notifier.awaitStopped()
	var runAbortedCount int
	for _, c := range fc.NotifyCallsSnapshot() {
		if c.Class == cmuxctl.NotificationRunAborted {
			runAbortedCount++
		}
	}
	if runAbortedCount != 1 {
		t.Errorf("concurrent triggers: expected 1 NotificationRunAborted, got %d", runAbortedCount)
	}
}

// TestW4_PostRunCompleted_DetectionGuard verifies that the runCompleted guard
// pattern prevents the abort gate from being triggered after workflow.Run
// returns. After runCmuxWorkflowAdapted returns (0, false), completed.done()
// must be true and a conditional trigger must be a no-op.
func TestW4_PostRunCompleted_DetectionGuard(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeMinimalWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()

	gate := newAbortGate()
	completed := &runCompleted{}

	code, aborted := runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil, nil, gate, completed)
	if code != 0 || aborted {
		t.Fatalf("expected clean run (0, false), got (%d, %v)", code, aborted)
	}

	// completed.done() must be true after successful return.
	if !completed.done() {
		t.Error("completed.done() must be true after runCmuxWorkflowAdapted returns")
	}

	// Simulate a detection path that guards on runCompleted.
	if !completed.done() {
		gate.trigger(AbortCauseDisplayPaneExit)
	}

	// Gate must not have been triggered.
	if gate.IsAborting() {
		t.Error("gate must not be triggered when completed.done() is true")
	}

	// No WorkspaceDone{Aborted:true} in the messages.
	for _, m := range ch.SentMessages() {
		if wd, ok := m.(interactionchannel.WorkspaceDone); ok && wd.Aborted {
			t.Error("WorkspaceDone{Aborted:true} must not be sent when detection fires after runCompleted")
		}
	}
}

// TestW4_AbortPath_DeferredCleanupRuns verifies that the abort path returns
// normally so deferred cleanup (e.g. log.Close) runs. This is a structural
// guarantee that no os.Exit() is called on the abort path: if os.Exit fired
// the test process would terminate and the test would fail to report results.
func TestW4_AbortPath_DeferredCleanupRuns(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeFailingWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()

	gate := newAbortGate()
	completed := &runCompleted{}

	cleanupRan := false
	func() {
		// deferred cleanup inside the anonymous func mirrors a caller's defer log.Close().
		defer func() { cleanupRan = true }()

		done := make(chan struct {
			code    int
			aborted bool
		}, 1)
		go func() {
			code, aborted := runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil, nil, gate, completed)
			done <- struct {
				code    int
				aborted bool
			}{code, aborted}
		}()

		if !pollModeErrorCount(ch, 1, 5*time.Second) {
			t.Fatal("ModeError did not appear")
		}
		gate.trigger(AbortCauseStallDetected)

		select {
		case r := <-done:
			if r.code != 1 || !r.aborted {
				t.Errorf("expected (1, true), got (%d, %v)", r.code, r.aborted)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("runCmuxWorkflowAdapted did not return on abort path")
		}
	}()

	if !cleanupRan {
		t.Error("deferred cleanup did not run — abort path may have called os.Exit")
	}
}

// TestW4_FooterModeDoneAfterAbort verifies that the abort sequence still sets
// ModeDone on the footer before broadcasting WorkspaceDone{Aborted:true}.
func TestW4_FooterModeDoneAfterAbort(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeFailingWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()

	gate := newAbortGate()
	completed := &runCompleted{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil, nil, gate, completed)
	}()

	if !pollModeErrorCount(ch, 1, 5*time.Second) {
		t.Fatal("ModeError did not appear")
	}
	gate.trigger(AbortCauseDisplayPaneExit)

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("runCmuxWorkflowAdapted did not return")
	}

	msgs := ch.SentMessages()
	// Find WorkspaceDone{Aborted:true} position and the last ModeDone footer before it.
	abortDoneIdx := -1
	for i, m := range msgs {
		if wd, ok := m.(interactionchannel.WorkspaceDone); ok && wd.Aborted {
			abortDoneIdx = i
			break
		}
	}
	if abortDoneIdx < 0 {
		t.Fatal("WorkspaceDone{Aborted:true} not found")
	}

	hasDoneFooter := false
	for _, m := range msgs[:abortDoneIdx] {
		if footer, ok := m.(interactionchannel.StateFooter); ok {
			if ui.Mode(footer.Mode) == ui.ModeDone {
				hasDoneFooter = true
			}
		}
	}
	if !hasDoneFooter {
		t.Error("expected a StateFooter(ModeDone) before WorkspaceDone{Aborted:true}")
	}
}
