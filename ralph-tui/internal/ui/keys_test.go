package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newKeysTestHandler creates a KeyHandler in the given mode with a buffered
// actions channel large enough to absorb all sends without blocking.
func newKeysTestHandler(t *testing.T, mode Mode) (*KeyHandler, chan StepAction) {
	t.Helper()
	actions := make(chan StepAction, 10)
	h := NewKeyHandler(func() {}, actions)
	h.SetMode(mode)
	return h, actions
}

// keyMsg is a convenience constructor for rune-based tea.KeyMsg values.
func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// --- TP-010: early return for non-KeyMsg ---

func TestKeysModel_Update_NonKeyMsg_NoOp(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	m := newKeysModel(h)

	// LogLinesMsg is not a tea.KeyMsg — must be ignored.
	next, cmd := m.Update(LogLinesMsg{Lines: []string{"line"}})
	if cmd != nil {
		t.Error("expected nil cmd for non-KeyMsg")
	}
	if next.handler.Mode() != ModeNormal {
		t.Errorf("mode changed unexpectedly: got %v", next.handler.Mode())
	}
}

func TestKeysModel_Update_WindowSizeMsg_NoOp(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	m := newKeysModel(h)

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if cmd != nil {
		t.Error("expected nil cmd for WindowSizeMsg")
	}
	if next.handler.Mode() != ModeNormal {
		t.Errorf("mode changed unexpectedly: got %v", next.handler.Mode())
	}
}

// --- TP-003: handleNormal ---

func TestHandleNormal_N_NilCancel_NoCmd(t *testing.T) {
	actions := make(chan StepAction, 10)
	h := NewKeyHandler(nil, actions)
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("n"))
	if cmd != nil {
		t.Error("expected nil cmd for n with nil cancel")
	}
	if len(actions) != 0 {
		t.Errorf("expected no actions, got %d", len(actions))
	}
}

func TestHandleNormal_Q_TransitionsToQuitConfirm(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("q"))
	if cmd != nil {
		t.Error("expected nil cmd for q in normal mode")
	}
	if h.Mode() != ModeQuitConfirm {
		t.Errorf("expected ModeQuitConfirm, got %v", h.Mode())
	}
	if h.ShortcutLine() != QuitConfirmPrompt {
		t.Errorf("expected QuitConfirmPrompt shortcut, got %q", h.ShortcutLine())
	}
}

func TestHandleNormal_Q_SavesPrevModeAsNormal(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	m := newKeysModel(h)

	m.Update(keyMsg("q"))

	h.mu.Lock()
	prev := h.prevMode
	h.mu.Unlock()

	if prev != ModeNormal {
		t.Errorf("expected prevMode ModeNormal, got %v", prev)
	}
}

func TestHandleNormal_UnrecognizedKey_NoOp(t *testing.T) {
	h, actions := newKeysTestHandler(t, ModeNormal)
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("x"))
	if cmd != nil {
		t.Error("expected nil cmd for unrecognized key in normal mode")
	}
	if h.Mode() != ModeNormal {
		t.Errorf("mode changed unexpectedly: got %v", h.Mode())
	}
	if len(actions) != 0 {
		t.Errorf("expected no actions, got %d", len(actions))
	}
}

// --- TP-001: handleError ---

func TestHandleError_C_SendsActionContinue(t *testing.T) {
	h, actions := newKeysTestHandler(t, ModeError)
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("c"))
	if cmd != nil {
		t.Error("expected nil cmd for c in error mode")
	}

	select {
	case action := <-actions:
		if action != ActionContinue {
			t.Errorf("expected ActionContinue, got %v", action)
		}
	default:
		t.Error("expected ActionContinue in actions channel")
	}
}

func TestHandleError_R_SendsActionRetry(t *testing.T) {
	h, actions := newKeysTestHandler(t, ModeError)
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("r"))
	if cmd != nil {
		t.Error("expected nil cmd for r in error mode")
	}

	select {
	case action := <-actions:
		if action != ActionRetry {
			t.Errorf("expected ActionRetry, got %v", action)
		}
	default:
		t.Error("expected ActionRetry in actions channel")
	}
}

func TestHandleError_Q_TransitionsToQuitConfirm(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeError)
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("q"))
	if cmd != nil {
		t.Error("expected nil cmd for q in error mode")
	}
	if h.Mode() != ModeQuitConfirm {
		t.Errorf("expected ModeQuitConfirm, got %v", h.Mode())
	}
}

func TestHandleError_Q_SavesPrevModeAsError(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeError)
	m := newKeysModel(h)

	m.Update(keyMsg("q"))

	h.mu.Lock()
	prev := h.prevMode
	h.mu.Unlock()

	if prev != ModeError {
		t.Errorf("expected prevMode ModeError, got %v", prev)
	}
}

func TestHandleError_UnrecognizedKey_NoOp(t *testing.T) {
	h, actions := newKeysTestHandler(t, ModeError)
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("x"))
	if cmd != nil {
		t.Error("expected nil cmd for unrecognized key in error mode")
	}
	if h.Mode() != ModeError {
		t.Errorf("mode changed unexpectedly: got %v", h.Mode())
	}
	if len(actions) != 0 {
		t.Errorf("expected no actions, got %d", len(actions))
	}
}

// --- handleNextConfirm ---

func TestHandleNormal_N_EntersNextConfirm(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("n"))
	if cmd != nil {
		t.Error("expected nil cmd for n entering NextConfirm")
	}
	if h.Mode() != ModeNextConfirm {
		t.Errorf("expected ModeNextConfirm, got %v", h.Mode())
	}
	if h.ShortcutLine() != NextConfirmPrompt {
		t.Errorf("expected NextConfirmPrompt, got %q", h.ShortcutLine())
	}
}

func TestHandleNormal_N_SavesPrevModeAsNormal(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	m := newKeysModel(h)

	m.Update(keyMsg("n"))

	h.mu.Lock()
	prev := h.prevMode
	h.mu.Unlock()

	if prev != ModeNormal {
		t.Errorf("expected prevMode ModeNormal, got %v", prev)
	}
}

func TestHandleNextConfirm_Y_RestoresModeAndReturnsCmd(t *testing.T) {
	cancelCalled := false
	actions := make(chan StepAction, 10)
	h := NewKeyHandler(func() { cancelCalled = true }, actions)
	m := newKeysModel(h)

	// Press n to enter ModeNextConfirm.
	m.Update(keyMsg("n"))
	if h.Mode() != ModeNextConfirm {
		t.Fatalf("precondition: expected ModeNextConfirm, got %v", h.Mode())
	}

	// Press y to confirm skip.
	_, cmd := m.Update(keyMsg("y"))
	if h.Mode() != ModeNormal {
		t.Errorf("expected mode restored to ModeNormal, got %v", h.Mode())
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for y in next-confirm mode")
	}
	_ = cmd()
	if !cancelCalled {
		t.Error("expected cancel to be called after y cmd execution")
	}
}

func TestHandleNextConfirm_N_RevertsMode(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	m := newKeysModel(h)

	m.Update(keyMsg("n")) // enter ModeNextConfirm
	_, cmd := m.Update(keyMsg("n"))
	if cmd != nil {
		t.Error("expected nil cmd for n in next-confirm mode")
	}
	if h.Mode() != ModeNormal {
		t.Errorf("expected mode reverted to ModeNormal, got %v", h.Mode())
	}
	if h.ShortcutLine() != NormalShortcuts {
		t.Errorf("expected NormalShortcuts after n, got %q", h.ShortcutLine())
	}
}

func TestHandleNextConfirm_Esc_RevertsMode(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	m := newKeysModel(h)

	m.Update(keyMsg("n")) // enter ModeNextConfirm
	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	_, cmd := m.Update(escMsg)
	if cmd != nil {
		t.Error("expected nil cmd for esc in next-confirm mode")
	}
	if h.Mode() != ModeNormal {
		t.Errorf("expected mode reverted to ModeNormal, got %v", h.Mode())
	}
}

func TestHandleNextConfirm_UnrecognizedKey_NoOp(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	m := newKeysModel(h)

	m.Update(keyMsg("n")) // enter ModeNextConfirm
	_, cmd := m.Update(keyMsg("x"))
	if cmd != nil {
		t.Error("expected nil cmd for unrecognized key in next-confirm mode")
	}
	if h.Mode() != ModeNextConfirm {
		t.Errorf("mode changed unexpectedly: got %v", h.Mode())
	}
}

// --- handleDone ---

func TestHandleDone_Q_EntersQuitConfirm(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeDone)
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("q"))
	if cmd != nil {
		t.Error("expected nil cmd for q in done mode")
	}
	if h.Mode() != ModeQuitConfirm {
		t.Errorf("expected ModeQuitConfirm, got %v", h.Mode())
	}
}

func TestHandleDone_Q_SavesPrevModeAsDone(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeDone)
	m := newKeysModel(h)

	m.Update(keyMsg("q"))

	h.mu.Lock()
	prev := h.prevMode
	h.mu.Unlock()

	if prev != ModeDone {
		t.Errorf("expected prevMode ModeDone, got %v", prev)
	}
}

func TestHandleDone_OtherKeys_NoOp(t *testing.T) {
	for _, key := range []string{"n", "c", "r", "x"} {
		h, _ := newKeysTestHandler(t, ModeDone)
		m := newKeysModel(h)

		_, cmd := m.Update(keyMsg(key))
		if cmd != nil {
			t.Errorf("expected nil cmd for %q in done mode", key)
		}
		if h.Mode() != ModeDone {
			t.Errorf("mode changed unexpectedly on %q: got %v", key, h.Mode())
		}
	}
}

// --- TP-002: handleQuitConfirm ---

func TestHandleQuitConfirm_Y_ReturnsNonNilCmd(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeQuitConfirm)
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("y"))
	if cmd == nil {
		t.Fatal("expected non-nil cmd for y in quit-confirm mode")
	}
	// Executing the cmd calls ForceQuit (sets ModeQuitting) and returns tea.QuitMsg.
	result := cmd()
	if h.Mode() != ModeQuitting {
		t.Errorf("expected ModeQuitting after executing y cmd, got %v", h.Mode())
	}
	if _, ok := result.(tea.QuitMsg); !ok {
		t.Errorf("expected tea.QuitMsg from y cmd, got %T", result)
	}
}

func TestHandleQuitConfirm_Esc_FromDone_RevertsToDone(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeDone)
	m := newKeysModel(h)

	m.Update(keyMsg("q")) // enter QuitConfirm from Done
	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	_, cmd := m.Update(escMsg)
	if cmd != nil {
		t.Error("expected nil cmd for esc in quit-confirm mode")
	}
	if h.Mode() != ModeDone {
		t.Errorf("expected mode reverted to ModeDone, got %v", h.Mode())
	}
	if h.ShortcutLine() != DoneShortcuts {
		t.Errorf("expected DoneShortcuts after esc, got %q", h.ShortcutLine())
	}
}

func TestHandleQuitConfirm_N_RevertsMode(t *testing.T) {
	// Enter QuitConfirm from Normal via 'q' so prevMode is set correctly.
	h, _ := newKeysTestHandler(t, ModeNormal)
	m := newKeysModel(h)
	m.Update(keyMsg("q")) // transitions to QuitConfirm, prevMode = ModeNormal

	_, cmd := m.Update(keyMsg("n"))
	if cmd != nil {
		t.Error("expected nil cmd for n in quit-confirm mode")
	}
	if h.Mode() != ModeNormal {
		t.Errorf("expected mode reverted to ModeNormal, got %v", h.Mode())
	}
	if h.ShortcutLine() != NormalShortcuts {
		t.Errorf("expected NormalShortcuts after n, got %q", h.ShortcutLine())
	}
}

func TestHandleQuitConfirm_Esc_RevertsMode(t *testing.T) {
	// Enter QuitConfirm from Error so we can verify prevMode restoration.
	h, _ := newKeysTestHandler(t, ModeError)
	m := newKeysModel(h)
	m.Update(keyMsg("q")) // transitions to QuitConfirm, prevMode = ModeError

	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	_, cmd := m.Update(escMsg)
	if cmd != nil {
		t.Error("expected nil cmd for esc in quit-confirm mode")
	}
	if h.Mode() != ModeError {
		t.Errorf("expected mode reverted to ModeError, got %v", h.Mode())
	}
	if h.ShortcutLine() != ErrorShortcuts {
		t.Errorf("expected ErrorShortcuts after esc, got %q", h.ShortcutLine())
	}
}

func TestHandleQuitConfirm_UnrecognizedKey_NoOp(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeQuitConfirm)
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("x"))
	if cmd != nil {
		t.Error("expected nil cmd for unrecognized key in quit-confirm mode")
	}
	if h.Mode() != ModeQuitConfirm {
		t.Errorf("mode changed unexpectedly: got %v", h.Mode())
	}
}
