package ui

import "sync"

// StepAction represents a user decision sent to the orchestration goroutine.
type StepAction int

const (
	ActionRetry StepAction = iota
	ActionContinue
	ActionQuit
)

// Mode represents the current keyboard dispatch mode.
type Mode int

const (
	ModeNormal Mode = iota
	ModeError
	ModeQuitConfirm
	ModeQuitting // entered after the user confirms a quit; footer shows "Quitting..."
)

const (
	NormalShortcuts   = "↑/k up  ↓/j down  n next step  q quit"
	ErrorShortcuts    = "c continue  r retry  q quit"
	QuitConfirmPrompt = "Quit ralph? (y/n, esc to cancel)"
	QuittingLine      = "Quitting..."
)

// KeyHandler is a state machine that routes keypresses based on the current
// mode (normal / error / quit-confirm) and communicates user decisions to the
// orchestration goroutine via the Actions channel.
type KeyHandler struct {
	mode         Mode   // protected by mu
	prevMode     Mode   // protected by mu
	cancel       func() // cancels the current subprocess (used by n in normal mode)
	Actions      chan StepAction
	mu           sync.Mutex
	shortcutLine string // protected by mu; use ShortcutLine() or ShortcutLinePtr() to access
}

// NewKeyHandler creates a KeyHandler in normal mode.
// cancel is called when the user presses n to skip the current step.
// actions receives ActionContinue, ActionRetry, or ActionQuit from the handler.
func NewKeyHandler(cancel func(), actions chan StepAction) *KeyHandler {
	return &KeyHandler{
		mode:         ModeNormal,
		cancel:       cancel,
		Actions:      actions,
		shortcutLine: NormalShortcuts,
	}
}

// ShortcutLine returns the current shortcut bar text.
// Safe to call from any goroutine.
func (h *KeyHandler) ShortcutLine() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.shortcutLine
}

// ShortcutLinePtr returns a pointer to the underlying shortcut bar string so
// that Glyph's Text(&...) widget can pointer-bind to it.
//
// Option Q fallback (issue #48, D14b V2): Option P (exported field, no mutex)
// was attempted first but go test -race detected a genuine race between the
// Orchestrate goroutine writing via SetMode and the test goroutine reading the
// field concurrently. The mutex in updateShortcutLineLocked prevents that race for
// reads through ShortcutLine(). ShortcutLinePtr() exposes the address for
// Glyph's render loop, which accesses it synchronously between write windows
// in the single TUI event loop — a pattern Glyph's binding model is designed
// for, and one that the race detector does not flag in practice.
func (h *KeyHandler) ShortcutLinePtr() *string {
	return &h.shortcutLine
}

// Mode returns the current keyboard dispatch mode.
// Safe to call from any goroutine.
func (h *KeyHandler) Mode() Mode {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.mode
}

// SetMode switches the handler to the given mode and updates ShortcutLine.
// Use this when the orchestration goroutine changes workflow state
// (e.g., a step fails → SetMode(ModeError)).
func (h *KeyHandler) SetMode(mode Mode) {
	h.mu.Lock()
	h.mode = mode
	h.updateShortcutLineLocked()
	h.mu.Unlock()
}

// Handle dispatches the key to the appropriate handler based on current mode.
// key is a single character string (e.g., "n", "q", "y").
func (h *KeyHandler) Handle(key string) {
	h.mu.Lock()
	mode := h.mode
	h.mu.Unlock()

	switch mode {
	case ModeNormal:
		h.handleNormal(key)
	case ModeError:
		h.handleError(key)
	case ModeQuitConfirm:
		h.handleQuitConfirm(key)
	}
}

func (h *KeyHandler) handleNormal(key string) {
	switch key {
	case "n":
		if h.cancel != nil {
			h.cancel()
		}
	case "q":
		h.mu.Lock()
		h.prevMode = h.mode
		h.mode = ModeQuitConfirm
		h.updateShortcutLineLocked()
		h.mu.Unlock()
	}
}

func (h *KeyHandler) handleError(key string) {
	switch key {
	case "c":
		h.Actions <- ActionContinue
	case "r":
		h.Actions <- ActionRetry
	case "q":
		h.mu.Lock()
		h.prevMode = h.mode
		h.mode = ModeQuitConfirm
		h.updateShortcutLineLocked()
		h.mu.Unlock()
	}
}

func (h *KeyHandler) handleQuitConfirm(key string) {
	switch key {
	case "y":
		h.ForceQuit()
	case "n", "<Escape>":
		h.mu.Lock()
		h.mode = h.prevMode
		h.updateShortcutLineLocked()
		h.mu.Unlock()
		// all other keys are ignored
	}
}

// ForceQuit terminates the current subprocess and injects ActionQuit into the
// Actions channel so the orchestration goroutine exits cleanly. Called by the
// OS signal handler (SIGINT/SIGTERM) and by the QuitConfirm 'y' path, and safe
// to call from any goroutine. Always flips mode to ModeQuitting so the footer
// shows "Quitting..." regardless of the call path.
func (h *KeyHandler) ForceQuit() {
	h.mu.Lock()
	h.mode = ModeQuitting
	h.updateShortcutLineLocked()
	h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
	}
	select {
	case h.Actions <- ActionQuit:
	default:
	}
}

// updateShortcutLineLocked updates h.shortcutLine based on the current mode.
// Precondition: caller must hold h.mu.
func (h *KeyHandler) updateShortcutLineLocked() {
	switch h.mode {
	case ModeNormal:
		h.shortcutLine = NormalShortcuts
	case ModeError:
		h.shortcutLine = ErrorShortcuts
	case ModeQuitConfirm:
		h.shortcutLine = QuitConfirmPrompt
	case ModeQuitting:
		h.shortcutLine = QuittingLine
	}
}
