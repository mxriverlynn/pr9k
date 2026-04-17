package ui

import (
	"strings"
	"sync"
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

// --- ModeHelp tests ---

func TestHandleNormal_QuestionMark_WithStatusLineActive_EntersModeHelp(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	h.SetStatusLineActive(true)
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("?"))
	if cmd != nil {
		t.Error("expected nil cmd for ? in normal mode")
	}
	if h.Mode() != ModeHelp {
		t.Errorf("expected ModeHelp, got %v", h.Mode())
	}
	h.mu.Lock()
	prev := h.prevMode
	h.mu.Unlock()
	if prev != ModeNormal {
		t.Errorf("expected prevMode ModeNormal, got %v", prev)
	}
}

func TestHandleNormal_QuestionMark_WithStatusLineInactive_NoOp(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	// statusLineActive defaults to false — do not call SetStatusLineActive
	m := newKeysModel(h)

	_, cmd := m.Update(keyMsg("?"))
	if cmd != nil {
		t.Error("expected nil cmd for ? when status line inactive")
	}
	if h.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal (no-op), got %v", h.Mode())
	}
}

func TestHandleHelp_Esc_ReturnsToPrevMode(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	h.SetStatusLineActive(true)
	m := newKeysModel(h)

	m.Update(keyMsg("?")) // enter ModeHelp, prevMode = ModeNormal
	if h.Mode() != ModeHelp {
		t.Fatalf("precondition: expected ModeHelp, got %v", h.Mode())
	}

	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	_, cmd := m.Update(escMsg)
	if cmd != nil {
		t.Error("expected nil cmd for esc in help mode")
	}
	if h.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal after esc, got %v", h.Mode())
	}
}

func TestHandleHelp_Q_EntersModeQuitConfirm_LeavesPrevModeUntouched(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	h.SetStatusLineActive(true)
	m := newKeysModel(h)

	m.Update(keyMsg("?")) // enter ModeHelp, prevMode = ModeNormal

	_, cmd := m.Update(keyMsg("q"))
	if cmd != nil {
		t.Error("expected nil cmd for q in help mode")
	}
	if h.Mode() != ModeQuitConfirm {
		t.Errorf("expected ModeQuitConfirm, got %v", h.Mode())
	}
	// prevMode must still be ModeNormal (not ModeHelp) so QuitConfirm.Esc
	// restores the original idle mode.
	h.mu.Lock()
	prev := h.prevMode
	h.mu.Unlock()
	if prev != ModeNormal {
		t.Errorf("expected prevMode ModeNormal (unchanged), got %v", prev)
	}
}

func TestHandleQuitConfirm_Esc_EnteredViaHelp_ReturnsToPrevMode(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	h.SetStatusLineActive(true)
	m := newKeysModel(h)

	m.Update(keyMsg("?")) // Normal → Help, prevMode = ModeNormal
	m.Update(keyMsg("q")) // Help → QuitConfirm, prevMode still ModeNormal

	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	_, cmd := m.Update(escMsg)
	if cmd != nil {
		t.Error("expected nil cmd for esc in quit-confirm mode entered via help")
	}
	if h.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal (not ModeHelp), got %v", h.Mode())
	}
}

func TestUpdateShortcutLineLocked_ModeHelp_SetsHelpModeShortcuts(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	h.SetStatusLineActive(true)
	m := newKeysModel(h)

	m.Update(keyMsg("?"))
	if h.ShortcutLine() != HelpModeShortcuts {
		t.Errorf("expected HelpModeShortcuts %q, got %q", HelpModeShortcuts, h.ShortcutLine())
	}
}

func TestHandleHelp_UnrecognizedKey_NoOp(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	h.SetStatusLineActive(true)
	m := newKeysModel(h)

	m.Update(keyMsg("?")) // enter ModeHelp
	_, cmd := m.Update(keyMsg("x"))
	if cmd != nil {
		t.Error("expected nil cmd for unrecognized key in help mode")
	}
	if h.Mode() != ModeHelp {
		t.Errorf("mode changed unexpectedly: got %v", h.Mode())
	}
}

// --- Parity test: footer shortcut constants ↔ modal body constants ---
//
// Every key token that appears in a footer shortcut constant must appear in the
// corresponding modal constant, and vice versa. The only allowed modal-only
// token is "?" in HelpModalNormal (it is conditional in the footer but always
// present in the modal so users can read what it does).

// TestHandleNonNormal_QuestionMark_NoOp_EvenWhenStatusLineActive verifies that ?
// is a no-op in every non-Normal mode, even when StatusLineActive is true. This
// pins the contract that ? only enters Help from ModeNormal.
func TestHandleNonNormal_QuestionMark_NoOp_EvenWhenStatusLineActive(t *testing.T) {
	modes := []struct {
		mode Mode
		name string
	}{
		{ModeError, "ModeError"},
		{ModeDone, "ModeDone"},
		{ModeSelect, "ModeSelect"},
		{ModeQuitConfirm, "ModeQuitConfirm"},
		{ModeNextConfirm, "ModeNextConfirm"},
		{ModeQuitting, "ModeQuitting"},
		{ModeHelp, "ModeHelp"},
	}

	for _, tc := range modes {
		t.Run(tc.name, func(t *testing.T) {
			h, actions := newKeysTestHandler(t, tc.mode)
			h.SetStatusLineActive(true)
			m := newKeysModel(h)

			_, cmd := m.Update(keyMsg("?"))
			if cmd != nil {
				t.Errorf("expected nil cmd for ? in %s", tc.name)
			}
			if h.Mode() != tc.mode {
				t.Errorf("mode changed unexpectedly in %s: got %v", tc.name, h.Mode())
			}
			h.mu.Lock()
			prev := h.prevMode
			h.mu.Unlock()
			if prev != ModeNormal {
				t.Errorf("prevMode changed in %s: got %v", tc.name, prev)
			}
			if len(actions) != 0 {
				t.Errorf("unexpected actions in %s: got %d", tc.name, len(actions))
			}
		})
	}
}

// TestHandleHelp_Esc_RestoresPrevModeShortcutLine verifies that escaping from
// Help restores the shortcut line to the pre-Help mode's shortcuts.
func TestHandleHelp_Esc_RestoresPrevModeShortcutLine(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	h.SetStatusLineActive(true)
	m := newKeysModel(h)

	m.Update(keyMsg("?")) // Normal → Help
	if h.Mode() != ModeHelp {
		t.Fatalf("precondition: expected ModeHelp, got %v", h.Mode())
	}

	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if h.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal after esc, got %v", h.Mode())
	}
	if h.ShortcutLine() != NormalShortcuts {
		t.Errorf("expected NormalShortcuts after esc from Help, got %q", h.ShortcutLine())
	}
}

// TestHandleHelp_Q_ShortcutLineShowsQuitConfirmPrompt verifies that pressing q
// from Help transitions the shortcut line to QuitConfirmPrompt.
func TestHandleHelp_Q_ShortcutLineShowsQuitConfirmPrompt(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	h.SetStatusLineActive(true)
	m := newKeysModel(h)

	m.Update(keyMsg("?")) // Normal → Help
	if h.Mode() != ModeHelp {
		t.Fatalf("precondition: expected ModeHelp, got %v", h.Mode())
	}

	m.Update(keyMsg("q"))

	if h.Mode() != ModeQuitConfirm {
		t.Errorf("expected ModeQuitConfirm after q from Help, got %v", h.Mode())
	}
	if h.ShortcutLine() != QuitConfirmPrompt {
		t.Errorf("expected QuitConfirmPrompt after q from Help, got %q", h.ShortcutLine())
	}
}

// TestHandleHelp_StatusLineFlipsInactive_DoesNotEject verifies that calling
// SetStatusLineActive(false) while already in ModeHelp does not force an exit.
func TestHandleHelp_StatusLineFlipsInactive_DoesNotEject(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	h.SetStatusLineActive(true)
	m := newKeysModel(h)

	m.Update(keyMsg("?")) // Normal → Help
	if h.Mode() != ModeHelp {
		t.Fatalf("precondition: expected ModeHelp, got %v", h.Mode())
	}

	h.SetStatusLineActive(false)

	if h.Mode() != ModeHelp {
		t.Errorf("expected ModeHelp after SetStatusLineActive(false), got %v", h.Mode())
	}
	if h.ShortcutLine() != HelpModeShortcuts {
		t.Errorf("expected HelpModeShortcuts after flip to inactive, got %q", h.ShortcutLine())
	}

	// Esc must still restore to prevMode normally.
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if h.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal after esc post-flip, got %v", h.Mode())
	}
	if h.ShortcutLine() != NormalShortcuts {
		t.Errorf("expected NormalShortcuts after esc from Help, got %q", h.ShortcutLine())
	}
}

// TestKeyHandler_StatusLineActive_ConcurrentAccess verifies that SetStatusLineActive
// and StatusLineActive are race-safe under concurrent access. Run with go test -race.
func TestKeyHandler_StatusLineActive_ConcurrentAccess(t *testing.T) {
	actions := make(chan StepAction, 2000)
	h := NewKeyHandler(func() {}, actions)

	var wg sync.WaitGroup

	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 500 {
				h.SetStatusLineActive(true)
				h.SetStatusLineActive(false)
			}
		}()
	}

	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m := newKeysModel(h)
			key := keyMsg("?")
			for range 500 {
				_ = h.StatusLineActive()
				_, _ = m.Update(key)
			}
		}()
	}

	wg.Wait()
}

// TestShortcutModalParity_NoUnapprovedModalExtras verifies that no shortcut key
// tokens appear in the help modals beyond the approved allowlist. Catches
// forward-drift where a key is added to a modal but not to the handler.
func TestShortcutModalParity_NoUnapprovedModalExtras(t *testing.T) {
	// knownMulticharKeys are recognized key sequences (not description words).
	knownMulticharKeys := map[string]bool{
		"hjkl": true,
		"↑↓←→": true,
		"esc":  true,
	}

	type pair struct {
		name          string
		modal         string
		allowedSingle map[string]bool
		allowedMulti  map[string]bool
	}
	pairs := []pair{
		{
			name:  "Normal",
			modal: HelpModalNormal,
			allowedSingle: map[string]bool{
				"↑": true, "/": true, "k": true, "↓": true, "j": true,
				"v": true, "n": true, "q": true, "?": true,
			},
			allowedMulti: map[string]bool{},
		},
		{
			name:  "Select",
			modal: HelpModalSelect,
			allowedSingle: map[string]bool{
				"/": true, "↑": true, "↓": true, "0": true, "$": true,
				"⇧": true, "y": true, "q": true,
			},
			allowedMulti: map[string]bool{"hjkl": true, "↑↓←→": true, "esc": true},
		},
		{
			name:  "Error",
			modal: HelpModalError,
			allowedSingle: map[string]bool{
				"c": true, "r": true, "q": true,
			},
			allowedMulti: map[string]bool{},
		},
		{
			name:  "Done",
			modal: HelpModalDone,
			allowedSingle: map[string]bool{
				"↑": true, "/": true, "k": true, "↓": true, "j": true,
				"v": true, "q": true,
			},
			allowedMulti: map[string]bool{},
		},
	}

	for _, p := range pairs {
		words := strings.Fields(p.modal)
		for _, word := range words {
			runes := []rune(word)
			if len(runes) == 1 {
				if !p.allowedSingle[word] {
					t.Errorf("modal[%s]: unexpected single-char token %q", p.name, word)
				}
			} else if knownMulticharKeys[word] {
				if !p.allowedMulti[word] {
					t.Errorf("modal[%s]: unexpected multi-char key token %q", p.name, word)
				}
			}
		}
	}
}

// --- TP-001: handleHelp ignores Normal-mode command keys ---

// TestHandleHelp_NormalModeKeys_AreNoOps verifies that keys with Normal-mode
// meanings (n, v, y, enter, c, r) are silent no-ops in ModeHelp. A future
// refactor that flattens dispatch must not let these keys fire their Normal
// actions while the Help modal is open.
func TestHandleHelp_NormalModeKeys_AreNoOps(t *testing.T) {
	keys := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{"n", keyMsg("n")},
		{"v", keyMsg("v")},
		{"y", keyMsg("y")},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}},
		{"c", keyMsg("c")},
		{"r", keyMsg("r")},
	}
	for _, tc := range keys {
		t.Run(tc.name, func(t *testing.T) {
			cancelCalled := false
			actions := make(chan StepAction, 10)
			h := NewKeyHandler(func() { cancelCalled = true }, actions)
			h.SetStatusLineActive(true)
			m := newKeysModel(h)

			m.Update(keyMsg("?")) // enter ModeHelp
			if h.Mode() != ModeHelp {
				t.Fatalf("precondition: expected ModeHelp, got %v", h.Mode())
			}

			_, cmd := m.Update(tc.msg)
			if cmd != nil {
				t.Errorf("key %q: expected nil cmd in ModeHelp, got non-nil", tc.name)
			}
			if h.Mode() != ModeHelp {
				t.Errorf("key %q: mode changed unexpectedly: got %v", tc.name, h.Mode())
			}
			h.mu.Lock()
			prev := h.prevMode
			h.mu.Unlock()
			if prev != ModeNormal {
				t.Errorf("key %q: prevMode changed: got %v", tc.name, prev)
			}
			if len(actions) != 0 {
				t.Errorf("key %q: unexpected actions: got %d", tc.name, len(actions))
			}
			if cancelCalled {
				t.Errorf("key %q: cancel was invoked unexpectedly", tc.name)
			}
		})
	}
}

// --- TP-002: ForceQuit() from ModeHelp transitions to ModeQuitting ---

// TestHandleHelp_ForceQuit_TransitionsToQuitting verifies that calling
// ForceQuit() while in ModeHelp (e.g. on SIGINT) still flips mode to
// ModeQuitting and drains ActionQuit. Ctrl-C must always work.
func TestHandleHelp_ForceQuit_TransitionsToQuitting(t *testing.T) {
	h, actions := newKeysTestHandler(t, ModeNormal)
	h.SetStatusLineActive(true)
	m := newKeysModel(h)

	m.Update(keyMsg("?")) // enter ModeHelp
	if h.Mode() != ModeHelp {
		t.Fatalf("precondition: expected ModeHelp, got %v", h.Mode())
	}

	h.ForceQuit()

	if h.Mode() != ModeQuitting {
		t.Errorf("expected ModeQuitting after ForceQuit from ModeHelp, got %v", h.Mode())
	}
	if h.ShortcutLine() != QuittingLine {
		t.Errorf("expected QuittingLine shortcut, got %q", h.ShortcutLine())
	}
	select {
	case action := <-actions:
		if action != ActionQuit {
			t.Errorf("expected ActionQuit, got %v", action)
		}
	default:
		t.Error("expected ActionQuit on actions channel, got nothing")
	}
}

// --- TP-003: ? is no-op after SetStatusLineActive toggles true→false ---

// TestHandleNormal_QuestionMark_NoOp_AfterStatusLineToggleFalse verifies that
// a ? press is a no-op when StatusLineActive was set true then immediately
// toggled back to false before the keypress.
func TestHandleNormal_QuestionMark_NoOp_AfterStatusLineToggleFalse(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeNormal)
	m := newKeysModel(h)

	h.SetStatusLineActive(true)
	h.SetStatusLineActive(false)

	_, cmd := m.Update(keyMsg("?"))
	if cmd != nil {
		t.Error("expected nil cmd for ? after StatusLineActive toggled to false")
	}
	if h.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal, got %v", h.Mode())
	}
	h.mu.Lock()
	prev := h.prevMode
	h.mu.Unlock()
	if prev != ModeNormal {
		t.Errorf("prevMode changed unexpectedly after no-op ?: got %v", prev)
	}
}

// --- TP-005: SetMode(ModeHelp) updates ShortcutLine and clears prevMode ---

// TestSetMode_ModeHelp_UpdatesShortcutLine documents that SetMode does not
// save prevMode. Esc from a SetMode-entered ModeHelp therefore restores the
// zero-value mode (ModeNormal), not the mode active before SetMode was called.
func TestSetMode_ModeHelp_UpdatesShortcutLine_ClearsPrevMode(t *testing.T) {
	h, _ := newKeysTestHandler(t, ModeError)
	m := newKeysModel(h)

	h.SetMode(ModeHelp)

	if h.ShortcutLine() != HelpModeShortcuts {
		t.Errorf("expected HelpModeShortcuts after SetMode(ModeHelp), got %q", h.ShortcutLine())
	}

	// prevMode was not saved by SetMode, so it retains its zero value (ModeNormal).
	// Esc from ModeHelp restores prevMode — which is ModeNormal here.
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if h.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal after esc from SetMode-entered Help, got %v", h.Mode())
	}
}

// --- TP-007: ? in ModeNormal does not invoke cancel ---

// TestHandleNormal_QuestionMark_DoesNotInvokeCancel verifies that pressing ?
// in ModeNormal (with StatusLineActive) does not call the cancel callback. A
// future refactor that folds key handlers must not accidentally wire ? to cancel.
func TestHandleNormal_QuestionMark_DoesNotInvokeCancel(t *testing.T) {
	cancelCalled := false
	actions := make(chan StepAction, 10)
	h := NewKeyHandler(func() { cancelCalled = true }, actions)
	h.SetStatusLineActive(true)
	m := newKeysModel(h)

	m.Update(keyMsg("?"))

	if cancelCalled {
		t.Error("? in ModeNormal must not invoke cancel")
	}
}

// --- TP-008: handleHelp does not send on h.Actions channel ---

// TestHandleHelp_CAndR_DoNotSendOnActions verifies that c and r in ModeHelp
// do not produce any StepAction sends. If handleHelp ever accidentally falls
// through to handleError, ghost actions would reach the dispatcher.
func TestHandleHelp_CAndR_DoNotSendOnActions(t *testing.T) {
	h, actions := newKeysTestHandler(t, ModeNormal)
	h.SetStatusLineActive(true)
	m := newKeysModel(h)

	m.Update(keyMsg("?")) // enter ModeHelp
	if h.Mode() != ModeHelp {
		t.Fatalf("precondition: expected ModeHelp, got %v", h.Mode())
	}

	m.Update(keyMsg("c"))
	m.Update(keyMsg("r"))

	if len(actions) != 0 {
		t.Errorf("expected no actions for c/r in ModeHelp, got %d", len(actions))
	}
}

func TestShortcutModalParity(t *testing.T) {
	type pair struct {
		name       string
		footer     string
		modal      string
		tokens     []string // expected in both footer and modal
		modalExtra []string // allowed in modal but not required in footer
	}
	pairs := []pair{
		{
			name:       "Normal",
			footer:     NormalShortcuts,
			modal:      HelpModalNormal,
			tokens:     []string{"↑", "k", "↓", "j", "v", "n", "q"},
			modalExtra: []string{"?"},
		},
		{
			name:   "Select",
			footer: SelectShortcuts,
			modal:  HelpModalSelect,
			tokens: []string{"hjkl", "↑", "↓", "←", "→", "0", "$", "⇧", "y", "esc", "q"},
		},
		{
			name:   "Error",
			footer: ErrorShortcuts,
			modal:  HelpModalError,
			tokens: []string{"c", "r", "q"},
		},
		{
			name:   "Done",
			footer: DoneShortcuts,
			modal:  HelpModalDone,
			tokens: []string{"↑", "k", "↓", "j", "v", "q"},
		},
	}

	for _, p := range pairs {
		// Every shared token must appear in the footer.
		for _, tok := range p.tokens {
			if !strings.Contains(p.footer, tok) {
				t.Errorf("parity[%s]: footer missing token %q", p.name, tok)
			}
		}
		// Every shared token must appear in the modal.
		for _, tok := range p.tokens {
			if !strings.Contains(p.modal, tok) {
				t.Errorf("parity[%s]: modal missing token %q", p.name, tok)
			}
		}
		// Modal-extra tokens must appear in the modal but need not be in the footer.
		for _, tok := range p.modalExtra {
			if !strings.Contains(p.modal, tok) {
				t.Errorf("parity[%s]: modal missing modal-only token %q", p.name, tok)
			}
		}
	}
}
