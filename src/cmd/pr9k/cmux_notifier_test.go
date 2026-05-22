package main

// W4 unit and integration tests for cmuxNotifier.
//
// Unit tests use FakeClient directly; integration tests use FakeClient +
// FakeInteractionChannel to drive runCmuxWorkflowAdapted end-to-end.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// newTestNotifier builds a cmuxNotifier with a FakeClient and a controlled
// ticker channel. The caller owns tickCh and can send to it to simulate ticks.
func newTestNotifier(t *testing.T, fc *cmuxctl.FakeClient, projectDir string) (*cmuxNotifier, chan time.Time) {
	t.Helper()
	log := mustNewTestLogger(t)
	ws := cmuxctl.Workspace{ID: "ws:test"}
	n := newCmuxNotifier(fc, ws, log, projectDir)
	tickCh := make(chan time.Time, 1)
	n.newTicker = func() (<-chan time.Time, func()) {
		return tickCh, func() {}
	}
	return n, tickCh
}

// snapshotClasses returns the notification class list from a race-safe snapshot.
func snapshotClasses(fc *cmuxctl.FakeClient) []cmuxctl.NotificationClass {
	calls := fc.NotifyCallsSnapshot()
	out := make([]cmuxctl.NotificationClass, len(calls))
	for i, c := range calls {
		out[i] = c.Class
	}
	return out
}

// snapshotBodies returns the notification body list from a race-safe snapshot.
func snapshotBodies(fc *cmuxctl.FakeClient) []string {
	calls := fc.NotifyCallsSnapshot()
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.Body
	}
	return out
}

// waitForNotifyCount polls until the NotifyCalls snapshot reaches count or timeout.
func waitForNotifyCount(t *testing.T, fc *cmuxctl.FakeClient, count int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(fc.NotifyCallsSnapshot()) >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout: wanted %d NotifyCalls, got %d", count, len(fc.NotifyCallsSnapshot()))
}

// stopAndSync stops the notifier's error-mode session and waits for all
// firePersistent goroutines to exit before returning.
func stopAndSync(n *cmuxNotifier) {
	_ = n.ExitErrorMode()
	n.awaitStopped()
}

// ---------------------------------------------------------------------------
// B1: EnterErrorMode records one NotifyCalls entry with class NotificationErrorMode
// ---------------------------------------------------------------------------

func TestNotifier_EnterErrorMode_RecordsOneCall(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	n, _ := newTestNotifier(t, fc, "/repos/myrepo")
	defer stopAndSync(n)

	if err := n.EnterErrorMode(context.Background(), "test-step"); err != nil {
		t.Fatalf("EnterErrorMode: %v", err)
	}

	classes := snapshotClasses(fc)
	if len(classes) != 1 {
		t.Fatalf("expected 1 NotifyCalls entry, got %d", len(classes))
	}
	if classes[0] != cmuxctl.NotificationErrorMode {
		t.Errorf("class = %q, want NotificationErrorMode", classes[0])
	}
}

// ---------------------------------------------------------------------------
// B2: EnterErrorMode body text matches spec D2
// ---------------------------------------------------------------------------

func TestNotifier_EnterErrorMode_BodyText(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	n, _ := newTestNotifier(t, fc, "/repos/myrepo")
	defer stopAndSync(n)

	_ = n.EnterErrorMode(context.Background(), "fail-step")

	bodies := snapshotBodies(fc)
	if len(bodies) == 0 {
		t.Fatal("no NotifyCalls recorded")
	}
	want := "fail-step failed in myrepo — Focus the footer pane to respond"
	if bodies[0] != want {
		t.Errorf("body = %q, want %q", bodies[0], want)
	}
}

// ---------------------------------------------------------------------------
// B3: Re-fire: one call per simulated tick
// ---------------------------------------------------------------------------

func TestNotifier_ReFire_OneCallPerTick(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	n, tickCh := newTestNotifier(t, fc, "/repos/proj")
	defer stopAndSync(n)

	_ = n.EnterErrorMode(context.Background(), "some-step")
	// Initial call: 1 entry.
	if len(fc.NotifyCallsSnapshot()) != 1 {
		t.Fatalf("expected 1 entry after EnterErrorMode, got %d", len(fc.NotifyCallsSnapshot()))
	}

	// Simulate a tick — should produce one more call.
	tickCh <- time.Now()
	waitForNotifyCount(t, fc, 2, time.Second)

	// Simulate another tick — should produce one more call.
	tickCh <- time.Now()
	waitForNotifyCount(t, fc, 3, time.Second)

	// All calls are NotificationErrorMode.
	for i, c := range snapshotClasses(fc) {
		if c != cmuxctl.NotificationErrorMode {
			t.Errorf("NotifyCalls[%d].Class = %q, want NotificationErrorMode", i, c)
		}
	}
}

// ---------------------------------------------------------------------------
// B4: ExitErrorMode stops the re-fire timer
// ---------------------------------------------------------------------------

func TestNotifier_ExitErrorMode_StopsReFire(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	n, tickCh := newTestNotifier(t, fc, "/repos/proj")

	_ = n.EnterErrorMode(context.Background(), "fail-step")
	stopAndSync(n) // cancel + wait for goroutine to exit

	// Send a tick — no additional call should be produced.
	tickCh <- time.Now()
	time.Sleep(20 * time.Millisecond)

	if len(fc.NotifyCallsSnapshot()) != 1 {
		t.Errorf("expected exactly 1 call (initial only), got %d", len(fc.NotifyCallsSnapshot()))
	}
}

// ---------------------------------------------------------------------------
// B5: RestartErrorModeTimer produces a fresh cadence
// ---------------------------------------------------------------------------

func TestNotifier_RestartErrorModeTimer_ProducesFreshCadence(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	n, _ := newTestNotifier(t, fc, "/repos/proj")

	ctx := context.Background()
	_ = n.EnterErrorMode(ctx, "fail-step")
	// 1 call after entry.

	stopAndSync(n) // stop the session

	// Restart: fresh resolved + goroutine.
	newTickCh := make(chan time.Time, 1)
	n.newTicker = func() (<-chan time.Time, func()) {
		return newTickCh, func() {}
	}
	n.RestartErrorModeTimer(ctx)

	// Send a tick on the new channel.
	newTickCh <- time.Now()
	waitForNotifyCount(t, fc, 2, time.Second)

	if len(fc.NotifyCallsSnapshot()) != 2 {
		t.Errorf("expected 2 calls after restart+tick, got %d", len(fc.NotifyCallsSnapshot()))
	}

	stopAndSync(n)
}

// ---------------------------------------------------------------------------
// B6: IntentContinue path stops the cadence
// ---------------------------------------------------------------------------

func TestNotifier_ExitErrorMode_AfterContinue_StopsReFire(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	n, tickCh := newTestNotifier(t, fc, "/repos/proj")

	_ = n.EnterErrorMode(context.Background(), "fail-step")
	stopAndSync(n) // simulate IntentContinue path calling ExitErrorMode

	tickCh <- time.Now()
	time.Sleep(20 * time.Millisecond)
	if len(fc.NotifyCallsSnapshot()) != 1 {
		t.Errorf("IntentContinue: expected 1 call (initial only), got %d", len(fc.NotifyCallsSnapshot()))
	}
}

// ---------------------------------------------------------------------------
// B7: IntentRetry path stops the cadence
// ---------------------------------------------------------------------------

func TestNotifier_ExitErrorMode_AfterRetry_StopsReFire(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	n, tickCh := newTestNotifier(t, fc, "/repos/proj")

	_ = n.EnterErrorMode(context.Background(), "fail-step")
	stopAndSync(n) // simulate IntentRetry path calling ExitErrorMode

	tickCh <- time.Now()
	time.Sleep(20 * time.Millisecond)
	if len(fc.NotifyCallsSnapshot()) != 1 {
		t.Errorf("IntentRetry: expected 1 call (initial only), got %d", len(fc.NotifyCallsSnapshot()))
	}
}

// ---------------------------------------------------------------------------
// B8: FireCompletion records one entry with NotificationCompletion
// ---------------------------------------------------------------------------

func TestNotifier_FireCompletion_RecordsOneCall(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	n, _ := newTestNotifier(t, fc, "/repos/myrepo")

	if err := n.FireCompletion(context.Background()); err != nil {
		t.Fatalf("FireCompletion: %v", err)
	}

	classes := snapshotClasses(fc)
	if len(classes) != 1 {
		t.Fatalf("expected 1 NotifyCalls entry, got %d", len(classes))
	}
	if classes[0] != cmuxctl.NotificationCompletion {
		t.Errorf("class = %q, want NotificationCompletion", classes[0])
	}
	want := "pr9k run completed in myrepo"
	if bodies := snapshotBodies(fc); bodies[0] != want {
		t.Errorf("body = %q, want %q", bodies[0], want)
	}
}

// ---------------------------------------------------------------------------
// B9: FireRunAborted returns nil on TimeoutError (abort-path non-fatal)
// ---------------------------------------------------------------------------

func TestNotifier_FireRunAborted_ReturnsNilOnTimeout(t *testing.T) {
	fc := &cmuxctl.FakeClient{
		WorkspaceNotifyFunc: func(_ context.Context, _ cmuxctl.Workspace, _ cmuxctl.NotificationClass, _ string) error {
			return &cmuxctl.TimeoutError{Method: "WorkspaceNotify", Duration: 8 * time.Second}
		},
	}
	n, _ := newTestNotifier(t, fc, "/repos/myrepo")

	if err := n.FireRunAborted(context.Background()); err != nil {
		t.Errorf("FireRunAborted: expected nil on TimeoutError (abort-path), got %v", err)
	}
}

func TestNotifier_FireRunAborted_BodyText(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	n, _ := newTestNotifier(t, fc, "/repos/myrepo")

	_ = n.FireRunAborted(context.Background())

	bodies := snapshotBodies(fc)
	if len(bodies) == 0 {
		t.Fatal("no NotifyCalls recorded")
	}
	want := "pr9k run aborted in myrepo"
	if bodies[0] != want {
		t.Errorf("body = %q, want %q", bodies[0], want)
	}
}

// ---------------------------------------------------------------------------
// B10: Post-resolution call is logged-not-fatal (spec D16)
// ---------------------------------------------------------------------------

func TestNotifier_PostResolution_CallIsNonFatal(t *testing.T) {
	// The tick fires while ExitErrorMode races to close resolved. The goroutine
	// should see resolved closed after the call and log-not-fatal, not panic.
	fc := &cmuxctl.FakeClient{}
	n, tickCh := newTestNotifier(t, fc, "/repos/proj")

	_ = n.EnterErrorMode(context.Background(), "step")

	// Close the session before sending the tick. The goroutine may still be
	// alive in the select when we send the tick.
	_ = n.ExitErrorMode()

	// Tick may or may not be received (goroutine may have exited already).
	// The important thing is no panic.
	select {
	case tickCh <- time.Now():
	default:
	}

	n.awaitStopped() // wait for goroutine to exit cleanly
}

// ---------------------------------------------------------------------------
// B11: Step name snapshotted at entry; reused across re-fires
// ---------------------------------------------------------------------------

func TestNotifier_SnapshotName_ReusedAcrossRefires(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	n, tickCh := newTestNotifier(t, fc, "/repos/proj")

	_ = n.EnterErrorMode(context.Background(), "original-step")
	tickCh <- time.Now()
	waitForNotifyCount(t, fc, 2, time.Second)

	stopAndSync(n)

	for i, b := range snapshotBodies(fc) {
		if !strings.Contains(b, "original-step") {
			t.Errorf("NotifyCalls[%d].Body = %q, want to contain 'original-step'", i, b)
		}
	}
}

func TestNotifier_FreshSession_CapturesFreshSnapshot(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	n, _ := newTestNotifier(t, fc, "/repos/proj")

	_ = n.EnterErrorMode(context.Background(), "old-step")
	stopAndSync(n)

	// Reuse same notifier (simulate IntentErrorQuitCancelled → Restart → new error).
	n2, _ := newTestNotifier(t, fc, "/repos/proj")
	_ = n2.EnterErrorMode(context.Background(), "new-step")
	stopAndSync(n2)

	bodies := snapshotBodies(fc)
	last := bodies[len(bodies)-1]
	if !strings.Contains(last, "new-step") {
		t.Errorf("fresh session body = %q, want to contain 'new-step'", last)
	}
}

// ---------------------------------------------------------------------------
// B12: ExitErrorMode is idempotent
// ---------------------------------------------------------------------------

func TestNotifier_ExitErrorMode_Idempotent(t *testing.T) {
	fc := &cmuxctl.FakeClient{}
	n, _ := newTestNotifier(t, fc, "/repos/proj")

	_ = n.EnterErrorMode(context.Background(), "step")
	stopAndSync(n)
	// Second call must not panic (already stopped).
	_ = n.ExitErrorMode()
}

// ---------------------------------------------------------------------------
// B13: nil notifier is safe for all methods
// ---------------------------------------------------------------------------

func TestNotifier_Nil_Safe(t *testing.T) {
	var n *cmuxNotifier
	ctx := context.Background()

	_ = n.FireCompletion(ctx)
	_ = n.FireRunAborted(ctx)
	_ = n.EnterErrorMode(ctx, "step")
	_ = n.ExitErrorMode()
	n.RestartErrorModeTimer(ctx)
}

// ---------------------------------------------------------------------------
// Integration tests: cmuxNotifier wired through runCmuxWorkflowAdapted
// ---------------------------------------------------------------------------

// IT1: Success path — one NotificationCompletion
func TestNotifier_Integration_SuccessPath(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeMinimalWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()
	fc := &cmuxctl.FakeClient{}
	n, _ := newTestNotifier(t, fc, projectDir)

	code, _ := runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil, n, nil, nil)
	n.awaitStopped()
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	classes := snapshotClasses(fc)
	if len(classes) != 1 {
		t.Fatalf("expected 1 NotifyCalls entry, got %d: %v", len(classes), classes)
	}
	if classes[0] != cmuxctl.NotificationCompletion {
		t.Errorf("class = %q, want NotificationCompletion", classes[0])
	}
}

// IT2: Error mode → continue — NotificationErrorMode on entry; timer stopped
// by IntentContinue; tick after stop produces no additional call.
func TestNotifier_Integration_ErrorModeContinue(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeFailingWorkflowConfig(t, workflowDir) // step always fails → error mode
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()
	fc := &cmuxctl.FakeClient{}

	// After error mode: continue (fails again), then quit.
	ch.InjectMessage(interactionchannel.Intent{Kind: interactionchannel.IntentContinue})
	ch.InjectMessage(interactionchannel.Intent{Kind: interactionchannel.IntentQuit})

	n, tickCh := newTestNotifier(t, fc, projectDir)

	done := make(chan int, 1)
	go func() {
		code, _ := runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil, n, nil, nil)
		done <- code
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runCmuxWorkflowAdapted did not return")
	}
	n.awaitStopped()

	countAfterQuit := len(fc.NotifyCallsSnapshot())

	// Drain any pending tick — should produce no additional call.
	select {
	case tickCh <- time.Now():
	default:
	}
	time.Sleep(20 * time.Millisecond)
	n.awaitStopped()

	countAfterTick := len(fc.NotifyCallsSnapshot())
	if countAfterTick != countAfterQuit {
		t.Errorf("tick after workflow exit produced %d new calls, want 0", countAfterTick-countAfterQuit)
	}
}

// IT3: Error mode → quit with intervening cancel; final NotificationRunAborted.
func TestNotifier_Integration_ErrorModeQuitWithCancel(t *testing.T) {
	projectDir := t.TempDir()
	workflowDir := t.TempDir()
	writeFailingWorkflowConfig(t, workflowDir)
	sf := loadTestStepFile(t, workflowDir)

	log := mustNewTestLogger(t)
	ch := interactionchannel.NewFakeInteractionChannel()
	fc := &cmuxctl.FakeClient{}

	// Sequence: error mode entered; q (Initiated), esc (Cancelled), q (Initiated), y (Quit).
	ch.InjectMessage(interactionchannel.Intent{Kind: interactionchannel.IntentErrorQuitInitiated})
	ch.InjectMessage(interactionchannel.Intent{Kind: interactionchannel.IntentErrorQuitCancelled})
	ch.InjectMessage(interactionchannel.Intent{Kind: interactionchannel.IntentErrorQuitInitiated})
	ch.InjectMessage(interactionchannel.Intent{Kind: interactionchannel.IntentQuit})

	n, _ := newTestNotifier(t, fc, projectDir)

	done := make(chan int, 1)
	go func() {
		code, _ := runCmuxWorkflowAdapted(context.Background(), ch, log, projectDir, workflowDir, sf, nil, n, nil, nil)
		done <- code
	}()

	select {
	case code := <-done:
		if code != 1 {
			t.Errorf("expected exit code 1 (UserQuit), got %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runCmuxWorkflowAdapted did not return")
	}
	n.awaitStopped()

	found := false
	for _, c := range snapshotClasses(fc) {
		if c == cmuxctl.NotificationRunAborted {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected NotificationRunAborted in NotifyCalls, got: %v", snapshotClasses(fc))
	}
}
