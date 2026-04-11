package ui

import (
	"testing"
)

func newTestHandler(t *testing.T) (*KeyHandler, *bool, chan StepAction) {
	t.Helper()
	cancelCalled := false
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(func() { cancelCalled = true }, actions)
	return h, &cancelCalled, actions
}

// --- Normal mode ---

func TestNormalMode_N_SendsCancelSignal(t *testing.T) {
	h, cancelCalled, _ := newTestHandler(t)

	h.Handle("n")

	if !*cancelCalled {
		t.Error("expected cancel to be called when pressing n in normal mode")
	}
}

func TestNormalMode_Q_ShowsQuitConfirmation(t *testing.T) {
	h, _, _ := newTestHandler(t)

	h.Handle("q")

	if h.mode != ModeQuitConfirm {
		t.Errorf("expected ModeQuitConfirm, got %v", h.mode)
	}
	if h.ShortcutLine() != QuitConfirmPrompt {
		t.Errorf("expected quit confirm prompt, got %q", h.ShortcutLine())
	}
}

func TestNormalMode_OtherKeys_Ignored(t *testing.T) {
	h, cancelCalled, actions := newTestHandler(t)

	h.Handle("x")

	if *cancelCalled {
		t.Error("cancel should not be called for unrecognized key")
	}
	if len(actions) != 0 {
		t.Error("no action should be sent for unrecognized key")
	}
	if h.mode != ModeNormal {
		t.Error("mode should remain ModeNormal for unrecognized key")
	}
}

// --- Quit confirmation from normal mode ---

func TestQuitConfirm_Y_SendsActionQuit(t *testing.T) {
	h, cancelCalled, actions := newTestHandler(t)

	h.Handle("q")
	h.Handle("y")

	if !*cancelCalled {
		t.Error("expected cancel (subprocess terminate) to be called when confirming quit from normal mode")
	}

	select {
	case action := <-actions:
		if action != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", action)
		}
	default:
		t.Error("expected ActionQuit to be sent on channel")
	}
}

func TestQuitConfirm_N_RestoresNormalMode(t *testing.T) {
	h, _, _ := newTestHandler(t)

	h.Handle("q")
	h.Handle("n")

	if h.mode != ModeNormal {
		t.Errorf("expected ModeNormal after dismissing quit, got %v", h.mode)
	}
	if h.ShortcutLine() != NormalShortcuts {
		t.Errorf("expected normal shortcuts, got %q", h.ShortcutLine())
	}
}

func TestQuitConfirm_OtherKey_RemainsInConfirmMode(t *testing.T) {
	h, _, actions := newTestHandler(t)

	h.Handle("q")
	h.Handle("x")

	if h.mode != ModeQuitConfirm {
		t.Errorf("expected ModeQuitConfirm to persist, got %v", h.mode)
	}
	if len(actions) != 0 {
		t.Error("no action should be sent for unrecognized key in quit-confirm mode")
	}
}

// --- Error mode ---

func TestSetMode_Error_UpdatesShortcutLine(t *testing.T) {
	h, _, _ := newTestHandler(t)

	h.SetMode(ModeError)

	if h.ShortcutLine() != ErrorShortcuts {
		t.Errorf("expected error shortcuts, got %q", h.ShortcutLine())
	}
}

func TestErrorMode_C_SendsActionContinue(t *testing.T) {
	h, _, actions := newTestHandler(t)
	h.SetMode(ModeError)

	h.Handle("c")

	select {
	case action := <-actions:
		if action != ActionContinue {
			t.Errorf("expected ActionContinue, got %v", action)
		}
	default:
		t.Error("expected ActionContinue to be sent on channel")
	}
}

func TestErrorMode_R_SendsActionRetry(t *testing.T) {
	h, _, actions := newTestHandler(t)
	h.SetMode(ModeError)

	h.Handle("r")

	select {
	case action := <-actions:
		if action != ActionRetry {
			t.Errorf("expected ActionRetry, got %v", action)
		}
	default:
		t.Error("expected ActionRetry to be sent on channel")
	}
}

func TestErrorMode_Q_ShowsQuitConfirmation(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.SetMode(ModeError)

	h.Handle("q")

	if h.mode != ModeQuitConfirm {
		t.Errorf("expected ModeQuitConfirm, got %v", h.mode)
	}
	if h.ShortcutLine() != QuitConfirmPrompt {
		t.Errorf("expected quit confirm prompt, got %q", h.ShortcutLine())
	}
}

func TestQuitConfirm_N_FromErrorMode_RestoresErrorMode(t *testing.T) {
	h, _, _ := newTestHandler(t)
	h.SetMode(ModeError)

	h.Handle("q")
	h.Handle("n")

	if h.mode != ModeError {
		t.Errorf("expected ModeError to be restored, got %v", h.mode)
	}
	if h.ShortcutLine() != ErrorShortcuts {
		t.Errorf("expected error shortcuts to be restored, got %q", h.ShortcutLine())
	}
}

// --- Constructor ---

func TestNewKeyHandler_InitialState(t *testing.T) {
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(func() {}, actions)

	if h.ShortcutLine() != NormalShortcuts {
		t.Errorf("expected NormalShortcuts, got %q", h.ShortcutLine())
	}
	if h.Actions != actions {
		t.Error("expected Actions to be the provided channel")
	}
}

func TestNewKeyHandler_NilCancel_NKey_NoAction_NoPanic(t *testing.T) {
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)

	h.Handle("n")

	if len(actions) != 0 {
		t.Error("no action should be sent when cancel is nil and n is pressed")
	}
	if h.mode != ModeNormal {
		t.Error("mode should remain ModeNormal")
	}
}

// --- Error mode ---

func TestErrorMode_OtherKeys_Ignored(t *testing.T) {
	h, _, actions := newTestHandler(t)
	h.SetMode(ModeError)

	h.Handle("x")

	if len(actions) != 0 {
		t.Error("no action should be sent for unrecognized key in error mode")
	}
	if h.mode != ModeError {
		t.Errorf("mode should remain ModeError, got %v", h.mode)
	}
	if h.ShortcutLine() != ErrorShortcuts {
		t.Errorf("ShortcutLine should remain ErrorShortcuts, got %q", h.ShortcutLine())
	}
}

// --- Quit confirmation from error mode ---

func TestQuitConfirm_Y_FromErrorMode_SendsActionQuit(t *testing.T) {
	h, cancelCalled, actions := newTestHandler(t)
	h.SetMode(ModeError)

	h.Handle("q")
	h.Handle("y")

	// Terminate is a no-op at runtime when no subprocess is running, but the
	// cancel hook should still fire so quit semantics are identical across modes.
	if !*cancelCalled {
		t.Error("expected cancel to be called when confirming quit from error mode")
	}

	select {
	case action := <-actions:
		if action != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", action)
		}
	default:
		t.Error("expected ActionQuit to be sent on channel")
	}
}

// --- ForceQuit ---

// TestForceQuit_CallsCancelAndInjectsActionQuit verifies that ForceQuit calls
// the cancel function and sends ActionQuit to the Actions channel.
func TestForceQuit_CallsCancelAndInjectsActionQuit(t *testing.T) {
	h, cancelCalled, actions := newTestHandler(t)

	h.ForceQuit()

	if !*cancelCalled {
		t.Error("expected cancel to be called by ForceQuit")
	}

	select {
	case action := <-actions:
		if action != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", action)
		}
	default:
		t.Error("expected ActionQuit to be in Actions channel after ForceQuit")
	}
}

// TestForceQuit_NilCancel_NoPanic verifies that ForceQuit is safe when cancel is nil.
func TestForceQuit_NilCancel_NoPanic(t *testing.T) {
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(nil, actions)

	h.ForceQuit() // must not panic

	select {
	case action := <-actions:
		if action != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", action)
		}
	default:
		t.Error("expected ActionQuit in channel even when cancel is nil")
	}
}

// TestForceQuit_FullChannel_NoPanic verifies that ForceQuit does not block or
// panic when the Actions channel is already full.
func TestForceQuit_FullChannel_NoPanic(t *testing.T) {
	actions := make(chan StepAction, 1)
	actions <- ActionContinue // fill the channel
	h := NewKeyHandler(func() {}, actions)

	h.ForceQuit() // must not block or panic
}

// T1 — ForceQuit does not mutate mode from ModeNormal.
func TestForceQuit_DoesNotAlterMode_WhenNormal(t *testing.T) {
	h, _, actions := newTestHandler(t)

	h.ForceQuit()
	<-actions // drain so the channel is empty

	if h.mode != ModeNormal {
		t.Errorf("expected ModeNormal after ForceQuit, got %v", h.mode)
	}
	if h.ShortcutLine() != NormalShortcuts {
		t.Errorf("expected NormalShortcuts after ForceQuit, got %q", h.ShortcutLine())
	}
}

// T1 (cont.) — ForceQuit does not mutate mode from ModeError.
func TestForceQuit_DoesNotAlterMode_WhenError(t *testing.T) {
	h, _, actions := newTestHandler(t)
	h.SetMode(ModeError)

	h.ForceQuit()
	<-actions // drain so the channel is empty

	if h.mode != ModeError {
		t.Errorf("expected ModeError after ForceQuit, got %v", h.mode)
	}
	if h.ShortcutLine() != ErrorShortcuts {
		t.Errorf("expected ErrorShortcuts after ForceQuit, got %q", h.ShortcutLine())
	}
}

// T2 — ForceQuit is idempotent: calling it twice does not panic, and the
// second ActionQuit send is dropped because the channel already holds one.
func TestForceQuit_Idempotent_CalledTwice(t *testing.T) {
	cancelCount := 0
	actions := make(chan StepAction, 1)
	h := NewKeyHandler(func() { cancelCount++ }, actions)

	h.ForceQuit()
	h.ForceQuit() // must not panic or block

	if cancelCount != 2 {
		t.Errorf("expected cancel called 2 times, got %d", cancelCount)
	}

	// Exactly one ActionQuit should be in the channel (second send dropped).
	if len(actions) != 1 {
		t.Errorf("expected exactly 1 ActionQuit in channel, got %d", len(actions))
	}
	action := <-actions
	if action != ActionQuit {
		t.Errorf("expected ActionQuit, got %v", action)
	}
}

// --- Race detector: concurrent ShortcutLine access (issue #48, Option Q) ---

// TestShortcutLine_ConcurrentRead_NoRace simulates Glyph's render goroutine
// reading ShortcutLine (via the mutex-protected accessor) concurrently while
// the workflow goroutine cycles modes. Verifies Option Q is race-free.
// Run with: go test -race ./...
func TestShortcutLine_ConcurrentRead_NoRace(t *testing.T) {
	h, _, _ := newTestHandler(t)

	stop := make(chan struct{})
	// Simulate Glyph's render goroutine: continuously reads ShortcutLine.
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_ = h.ShortcutLine()
			}
		}
	}()

	// Workflow goroutine: cycle through all modes.
	modes := []Mode{ModeError, ModeQuitConfirm, ModeNormal, ModeDone}
	for i := range 100 {
		h.SetMode(modes[i%len(modes)])
	}

	close(stop)
}

// --- ShortcutLinePtr ---

// T1: ShortcutLinePtr returns a non-nil pointer.
func TestShortcutLinePtr_ReturnsNonNilPointer(t *testing.T) {
	h, _, _ := newTestHandler(t)

	p := h.ShortcutLinePtr()

	if p == nil {
		t.Error("expected non-nil pointer from ShortcutLinePtr")
	}
}

// T2: Dereferencing the pointer returned by ShortcutLinePtr tracks mode changes.
func TestShortcutLinePtr_DereferencesToCurrentValue(t *testing.T) {
	h, _, _ := newTestHandler(t)
	p := h.ShortcutLinePtr()

	if *p != NormalShortcuts {
		t.Errorf("expected NormalShortcuts initially, got %q", *p)
	}

	h.SetMode(ModeError)
	if *p != ErrorShortcuts {
		t.Errorf("expected ErrorShortcuts after SetMode(ModeError), got %q", *p)
	}

	h.SetMode(ModeQuitConfirm)
	if *p != QuitConfirmPrompt {
		t.Errorf("expected QuitConfirmPrompt after SetMode(ModeQuitConfirm), got %q", *p)
	}

	h.SetMode(ModeNormal)
	if *p != NormalShortcuts {
		t.Errorf("expected NormalShortcuts after SetMode(ModeNormal), got %q", *p)
	}
}

// T3: ShortcutLinePtr returns the same address on every call.
func TestShortcutLinePtr_StableAddress(t *testing.T) {
	h, _, _ := newTestHandler(t)

	p1 := h.ShortcutLinePtr()
	p2 := h.ShortcutLinePtr()

	if p1 != p2 {
		t.Errorf("expected stable pointer address, got %p and %p", p1, p2)
	}
}

// T4: *ShortcutLinePtr() always agrees with ShortcutLine() after each SetMode.
func TestShortcutLinePtr_AgreesWithShortcutLine(t *testing.T) {
	h, _, _ := newTestHandler(t)
	modes := []Mode{ModeNormal, ModeError, ModeQuitConfirm, ModeDone}

	for _, mode := range modes {
		h.SetMode(mode)
		got := *h.ShortcutLinePtr()
		want := h.ShortcutLine()
		if got != want {
			t.Errorf("mode %v: *ShortcutLinePtr() = %q, ShortcutLine() = %q", mode, got, want)
		}
	}
}

// --- Done mode ---

// TestSetMode_Done_UpdatesShortcutLine verifies that SetMode(ModeDone) swaps
// the shortcut bar to DoneShortcuts.
func TestSetMode_Done_UpdatesShortcutLine(t *testing.T) {
	h, _, _ := newTestHandler(t)

	h.SetMode(ModeDone)

	if h.ShortcutLine() != DoneShortcuts {
		t.Errorf("expected DoneShortcuts, got %q", h.ShortcutLine())
	}
}

// TestDoneMode_AnyKey_SendsActionQuit verifies that any key press in ModeDone
// sends ActionQuit.
func TestDoneMode_AnyKey_SendsActionQuit(t *testing.T) {
	keys := []string{"q", "n", "y", "c", "r", " ", "x", "\r"}
	for _, key := range keys {
		actions := make(chan StepAction, 1)
		h := NewKeyHandler(func() {}, actions)
		h.SetMode(ModeDone)

		h.Handle(key)

		select {
		case action := <-actions:
			if action != ActionQuit {
				t.Errorf("key %q: expected ActionQuit, got %v", key, action)
			}
		default:
			t.Errorf("key %q: expected ActionQuit to be sent, but channel was empty", key)
		}
	}
}

// --- Keyboard dispatch routes correctly ---

func TestKeyboardDispatch_NormalVsError(t *testing.T) {
	// In normal mode, c and r are ignored (no action sent)
	h, _, actions := newTestHandler(t)
	h.Handle("c")
	h.Handle("r")
	if len(actions) != 0 {
		t.Error("c and r should be ignored in normal mode")
	}

	// In error mode, n does not call cancel
	cancelCalled := false
	h2 := NewKeyHandler(func() { cancelCalled = true }, make(chan StepAction, 1))
	h2.SetMode(ModeError)
	h2.Handle("n")
	if cancelCalled {
		t.Error("n should not trigger cancel in error mode")
	}
}
