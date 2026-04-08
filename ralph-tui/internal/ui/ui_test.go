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
	if h.ShortcutLine != QuitConfirmPrompt {
		t.Errorf("expected quit confirm prompt, got %q", h.ShortcutLine)
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
	h, _, actions := newTestHandler(t)

	h.Handle("q")
	h.Handle("y")

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
	if h.ShortcutLine != NormalShortcuts {
		t.Errorf("expected normal shortcuts, got %q", h.ShortcutLine)
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

	if h.ShortcutLine != ErrorShortcuts {
		t.Errorf("expected error shortcuts, got %q", h.ShortcutLine)
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
	if h.ShortcutLine != QuitConfirmPrompt {
		t.Errorf("expected quit confirm prompt, got %q", h.ShortcutLine)
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
	if h.ShortcutLine != ErrorShortcuts {
		t.Errorf("expected error shortcuts to be restored, got %q", h.ShortcutLine)
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
