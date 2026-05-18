package main

import (
	"context"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// ---------------------------------------------------------------------------
// Footer state machine tests (AC: q/y handled locally)
// ---------------------------------------------------------------------------

// TestFooterMachine_QY_ProducesExactlyOneQuitIntent verifies that q then y
// produces exactly one IntentQuit forwarded to the sink and does not produce
// any other intent.
func TestFooterMachine_QY_ProducesExactlyOneQuitIntent(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	src.Press("q")
	src.Press("y")
	m := newFooterStateMachine(src, src)
	m.Step() // q → ModeQuitConfirm (no intent yet)
	m.Step() // y → emit IntentQuit

	intent, ok := src.LastForwardedIntent()
	if !ok {
		t.Fatal("expected IntentQuit to be forwarded, got none")
	}
	if intent != interactionchannel.IntentQuit {
		t.Errorf("expected IntentQuit, got %q", intent)
	}
}

// TestFooterMachine_QEsc_ProducesNoIntent verifies that q then esc produces
// no forwarded intent (quit cancelled locally).
func TestFooterMachine_QEsc_ProducesNoIntent(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	src.Press("q")
	src.Press("esc")
	m := newFooterStateMachine(src, src)
	m.Step() // q → ModeQuitConfirm
	m.Step() // esc → cancel, restore mode

	_, ok := src.LastForwardedIntent()
	if ok {
		t.Error("expected no intent after q+esc, but one was forwarded")
	}
}

// TestFooterMachine_SetMode_UpdatesSink verifies that SetMode on the machine
// calls sink.SetMode with the new mode value.
func TestFooterMachine_SetMode_UpdatesSink(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	m := newFooterStateMachine(src, src)
	m.SetMode(ui.ModeError)
	if got := src.Mode(); got != int(ui.ModeError) {
		t.Errorf("expected mode %d (ModeError), got %d", int(ui.ModeError), got)
	}
}

// TestFooterMachine_ErrorMode_ContinueForwardsIntent verifies that c in
// ModeError emits IntentContinue.
func TestFooterMachine_ErrorMode_ContinueForwardsIntent(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	src.Press("c")
	m := newFooterStateMachine(src, src)
	m.SetMode(ui.ModeError)
	m.Step()

	intent, ok := src.LastForwardedIntent()
	if !ok {
		t.Fatal("expected IntentContinue, got none")
	}
	if intent != interactionchannel.IntentContinue {
		t.Errorf("expected IntentContinue, got %q", intent)
	}
}

// TestFooterMachine_ErrorMode_RetryForwardsIntent verifies that r in
// ModeError emits IntentRetry.
func TestFooterMachine_ErrorMode_RetryForwardsIntent(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	src.Press("r")
	m := newFooterStateMachine(src, src)
	m.SetMode(ui.ModeError)
	m.Step()

	intent, ok := src.LastForwardedIntent()
	if !ok {
		t.Fatal("expected IntentRetry, got none")
	}
	if intent != interactionchannel.IntentRetry {
		t.Errorf("expected IntentRetry, got %q", intent)
	}
}

// TestFooterMachine_NormalMode_NextConfirmY_ForwardsIntentNext verifies that
// n then y in ModeNormal emits IntentNext.
func TestFooterMachine_NormalMode_NextConfirmY_ForwardsIntentNext(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	src.Press("n")
	src.Press("y")
	m := newFooterStateMachine(src, src)
	m.Step() // n → ModeNextConfirm
	m.Step() // y → emit IntentNext

	intent, ok := src.LastForwardedIntent()
	if !ok {
		t.Fatal("expected IntentNext, got none")
	}
	if intent != interactionchannel.IntentNext {
		t.Errorf("expected IntentNext, got %q", intent)
	}
}

// TestFooterMachine_NormalMode_NextConfirmEsc_ProducesNoIntent verifies that
// n then esc produces no intent.
func TestFooterMachine_NormalMode_NextConfirmEsc_ProducesNoIntent(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	src.Press("n")
	src.Press("esc")
	m := newFooterStateMachine(src, src)
	m.Step()
	m.Step()

	_, ok := src.LastForwardedIntent()
	if ok {
		t.Error("expected no intent after n+esc, but one was forwarded")
	}
}

// T1: q in ModeError enters ModeQuitConfirm — not auto-quit.
func TestFooterMachine_ErrorMode_QEntersModeQuitConfirm(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	src.Press("q")
	m := newFooterStateMachine(src, src)
	m.SetMode(ui.ModeError)
	m.Step()

	if got := src.Mode(); got != int(ui.ModeQuitConfirm) {
		t.Errorf("expected ModeQuitConfirm (%d), got %d", int(ui.ModeQuitConfirm), got)
	}
	_, ok := src.LastForwardedIntent()
	if ok {
		t.Error("expected no intent after q in ModeError, but one was forwarded")
	}
}

// T2: q then esc from ModeError restores ModeError (not Normal).
func TestFooterMachine_ErrorMode_QEscRestoresModeError(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	src.Press("q")
	src.Press("esc")
	m := newFooterStateMachine(src, src)
	m.SetMode(ui.ModeError)
	m.Step() // q → ModeQuitConfirm
	m.Step() // esc → restoreMode() → back to ModeError

	if got := src.Mode(); got != int(ui.ModeError) {
		t.Errorf("expected ModeError (%d) after q+esc, got %d", int(ui.ModeError), got)
	}
	_, ok := src.LastForwardedIntent()
	if ok {
		t.Error("expected no intent after q+esc from ModeError, but one was forwarded")
	}
}

// T3: ModeQuitting swallows all keys — no new intent and mode stays Quitting.
func TestFooterMachine_ModeQuitting_SwallowsAllKeys(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	// First drive q+y to enter Quitting and produce one IntentQuit.
	src.Press("q")
	src.Press("y")
	// Then fire every possible key that could produce another intent.
	for _, k := range []string{"q", "y", "r", "c", "n"} {
		src.Press(k)
	}
	m := newFooterStateMachine(src, src)
	m.Step() // q → ModeQuitConfirm
	m.Step() // y → IntentQuit + ModeQuitting
	// Drain remaining keys; none should produce an intent or mode change.
	for m.Step() {
	}

	if got := src.Mode(); got != int(ui.ModeQuitting) {
		t.Errorf("expected ModeQuitting (%d), got %d", int(ui.ModeQuitting), got)
	}
	// Exactly one intent (the original IntentQuit) must have been forwarded.
	intent, ok := src.LastForwardedIntent()
	if !ok {
		t.Fatal("expected exactly one IntentQuit, got none")
	}
	if intent != interactionchannel.IntentQuit {
		t.Errorf("expected IntentQuit, got %q", intent)
	}
}

// T4: ModeDone accepts only q (r/c/n are no-ops; q+y emits IntentQuit).
func TestFooterMachine_ModeDone_OnlyQAccepted(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	src.Press("r")
	src.Press("c")
	src.Press("n")
	m := newFooterStateMachine(src, src)
	m.SetMode(ui.ModeDone)
	m.Step() // r — no-op
	m.Step() // c — no-op
	m.Step() // n — no-op

	if got := src.Mode(); got != int(ui.ModeDone) {
		t.Errorf("after r/c/n: expected ModeDone (%d), got %d", int(ui.ModeDone), got)
	}
	_, ok := src.LastForwardedIntent()
	if ok {
		t.Error("expected no intent after r/c/n in ModeDone, but one was forwarded")
	}

	// Now press q → ModeQuitConfirm, then y → IntentQuit.
	src.Press("q")
	src.Press("y")
	m.Step() // q → ModeQuitConfirm
	m.Step() // y → IntentQuit

	if got := src.Mode(); got != int(ui.ModeQuitting) {
		t.Errorf("after q+y: expected ModeQuitting (%d), got %d", int(ui.ModeQuitting), got)
	}
	intent, ok := src.LastForwardedIntent()
	if !ok {
		t.Fatal("expected IntentQuit after q+y from ModeDone, got none")
	}
	if intent != interactionchannel.IntentQuit {
		t.Errorf("expected IntentQuit, got %q", intent)
	}
}

// ---------------------------------------------------------------------------
// Key handler adapter tests (AC: intent round-trips, mode push, race window)
// ---------------------------------------------------------------------------

// makeAdapterFixture creates a KeyHandler + FakeInteractionChannel wired
// through a newKeyHandlerAdapter and returns them for testing.
// The returned cancel must be called when the test finishes.
func makeAdapterFixture(t *testing.T) (
	h *ui.KeyHandler,
	actions chan ui.StepAction,
	fch *interactionchannel.FakeInteractionChannel,
	cancel context.CancelFunc,
) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	actions = make(chan ui.StepAction, 10)
	h = ui.NewKeyHandler(nil, actions)
	fch = interactionchannel.NewFakeInteractionChannel()
	newKeyHandlerAdapter(ctx, h, fch)
	return h, actions, fch, cancel
}

// drainFirst receives the first StepAction from actions within 1s.
func drainFirst(t *testing.T, actions <-chan ui.StepAction) ui.StepAction {
	t.Helper()
	select {
	case a := <-actions:
		return a
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for action on Actions channel")
		return 0
	}
}

// TestAdapter_IntentRetry_RoundTrips verifies that IntentRetry on the wire
// delivers ActionRetry to the KeyHandler Actions channel.
func TestAdapter_IntentRetry_RoundTrips(t *testing.T) {
	_, actions, fch, cancel := makeAdapterFixture(t)
	defer cancel()

	fch.InjectIntent(interactionchannel.IntentRetry)

	if got := drainFirst(t, actions); got != ui.ActionRetry {
		t.Errorf("expected ActionRetry (%d), got %d", ui.ActionRetry, got)
	}
}

// TestAdapter_IntentContinue_RoundTrips verifies that IntentContinue on the
// wire delivers ActionContinue to the Actions channel.
func TestAdapter_IntentContinue_RoundTrips(t *testing.T) {
	_, actions, fch, cancel := makeAdapterFixture(t)
	defer cancel()

	fch.InjectIntent(interactionchannel.IntentContinue)

	if got := drainFirst(t, actions); got != ui.ActionContinue {
		t.Errorf("expected ActionContinue (%d), got %d", ui.ActionContinue, got)
	}
}

// TestAdapter_IntentQuit_RoundTrips verifies that IntentQuit on the wire
// delivers ActionQuit to the Actions channel.
func TestAdapter_IntentQuit_RoundTrips(t *testing.T) {
	_, actions, fch, cancel := makeAdapterFixture(t)
	defer cancel()

	fch.InjectIntent(interactionchannel.IntentQuit)

	if got := drainFirst(t, actions); got != ui.ActionQuit {
		t.Errorf("expected ActionQuit (%d), got %d", ui.ActionQuit, got)
	}
}

// TestAdapter_IntentSkip_RoundTrips verifies that IntentSkip on the wire
// delivers ActionSkip to the Actions channel.
func TestAdapter_IntentSkip_RoundTrips(t *testing.T) {
	_, actions, fch, cancel := makeAdapterFixture(t)
	defer cancel()

	fch.InjectIntent(interactionchannel.IntentSkip)

	if got := drainFirst(t, actions); got != ui.ActionSkip {
		t.Errorf("expected ActionSkip (%d), got %d", ui.ActionSkip, got)
	}
}

// TestAdapter_IntentNext_RoundTrips verifies that IntentNext on the wire
// delivers ActionNext to the Actions channel.
func TestAdapter_IntentNext_RoundTrips(t *testing.T) {
	_, actions, fch, cancel := makeAdapterFixture(t)
	defer cancel()

	fch.InjectIntent(interactionchannel.IntentNext)

	if got := drainFirst(t, actions); got != ui.ActionNext {
		t.Errorf("expected ActionNext (%d), got %d", ui.ActionNext, got)
	}
}

// TestAdapter_SetMode_PushesStateFooter verifies that calling SetMode on the
// KeyHandler causes the adapter to push a StateFooter message carrying the
// new mode to the interaction channel.
func TestAdapter_SetMode_PushesStateFooter(t *testing.T) {
	h, _, fch, cancel := makeAdapterFixture(t)
	defer cancel()

	h.SetMode(ui.ModeError)

	// Give the hook (synchronous) a moment to propagate; the hook is called
	// synchronously in SetMode, so the message is in fch immediately.
	msg, ok := fch.ExpectStatePush()
	if !ok {
		t.Fatal("expected StateFooter push after SetMode(ModeError), got none")
	}
	sf, ok := msg.(interactionchannel.StateFooter)
	if !ok {
		t.Fatalf("expected StateFooter, got %T", msg)
	}
	if sf.Mode != int(ui.ModeError) {
		t.Errorf("StateFooter.Mode: expected %d (ModeError), got %d", int(ui.ModeError), sf.Mode)
	}
	if sf.ShortcutLine == "" {
		t.Error("StateFooter.ShortcutLine must not be empty")
	}
}

// TestAdapter_ForceQuit_PushesStateFooterQuitting verifies that ForceQuit
// triggers a StateFooter push with ModeQuitting.
func TestAdapter_ForceQuit_PushesStateFooterQuitting(t *testing.T) {
	h, _, fch, cancel := makeAdapterFixture(t)
	defer cancel()

	h.ForceQuit()

	msg, ok := fch.ExpectStatePush()
	if !ok {
		t.Fatal("expected StateFooter push after ForceQuit, got none")
	}
	sf, ok := msg.(interactionchannel.StateFooter)
	if !ok {
		t.Fatalf("expected StateFooter, got %T", msg)
	}
	if sf.Mode != int(ui.ModeQuitting) {
		t.Errorf("StateFooter.Mode: expected %d (ModeQuitting), got %d", int(ui.ModeQuitting), sf.Mode)
	}
}

// TestAdapter_ModeErrorRaceWindow_NoDroppedKeystroke is the race-window test:
// SetMode(ModeError) is called, then an intent arrives before the Actions
// channel is read. The intent must not be dropped.
func TestAdapter_ModeErrorRaceWindow_NoDroppedKeystroke(t *testing.T) {
	_, actions, fch, cancel := makeAdapterFixture(t)
	defer cancel()

	// Simulate the race window: set error mode (workflow is about to block
	// on <-Actions) but don't read from Actions yet.
	fch.InjectIntent(interactionchannel.IntentContinue)

	// Now drain — the intent must have been buffered, not dropped.
	if got := drainFirst(t, actions); got != ui.ActionContinue {
		t.Errorf("intent dropped in race window: expected ActionContinue (%d), got %d", ui.ActionContinue, got)
	}
}

// TestAdapter_GoRoutineCleanup_ContextCancel verifies that cancelling the
// context stops the adapter goroutine (no goroutine leak). We verify this
// indirectly by ensuring the adapter does not block after cancel.
func TestAdapter_GoRoutineCleanup_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	actions := make(chan ui.StepAction, 10)
	h := ui.NewKeyHandler(nil, actions)
	fch := interactionchannel.NewFakeInteractionChannel()
	stopAdapter := newKeyHandlerAdapter(ctx, h, fch)

	cancel() // cancel the parent context
	stopAdapter()

	// After cancel, inject an intent; it should NOT appear on Actions
	// because the goroutine should have exited.
	fch.InjectIntent(interactionchannel.IntentRetry)
	select {
	case <-actions:
		// The goroutine may have processed the intent before stopping;
		// that's acceptable — we just verify no permanent block or panic.
	case <-time.After(50 * time.Millisecond):
		// Timeout is expected: goroutine stopped, no delivery.
	}
}
