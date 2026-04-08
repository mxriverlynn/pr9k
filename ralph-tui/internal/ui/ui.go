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
)

const (
	NormalShortcuts   = "↑/k up  ↓/j down  n next step  q quit"
	ErrorShortcuts    = "c continue  r retry  q quit"
	QuitConfirmPrompt = "Quit ralph? (y/n)"
)

// KeyHandler is a state machine that routes keypresses based on the current
// mode (normal / error / quit-confirm) and communicates user decisions to the
// orchestration goroutine via the Actions channel.
type KeyHandler struct {
	mode         Mode
	prevMode     Mode
	cancel       func() // cancels the current subprocess (used by n in normal mode)
	Actions      chan StepAction
	mu           sync.Mutex
	shortcutLine string // protected by mu; use ShortcutLine() to read from other goroutines
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

// SetMode switches the handler to the given mode and updates ShortcutLine.
// Use this when the orchestration goroutine changes workflow state
// (e.g., a step fails → SetMode(ModeError)).
func (h *KeyHandler) SetMode(mode Mode) {
	h.mode = mode
	h.updateShortcutLine()
}

// Handle dispatches the key to the appropriate handler based on current mode.
// key is a single character string (e.g., "n", "q", "y").
func (h *KeyHandler) Handle(key string) {
	switch h.mode {
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
		h.prevMode = h.mode
		h.mode = ModeQuitConfirm
		h.updateShortcutLine()
	}
}

func (h *KeyHandler) handleError(key string) {
	switch key {
	case "c":
		h.Actions <- ActionContinue
	case "r":
		h.Actions <- ActionRetry
	case "q":
		h.prevMode = h.mode
		h.mode = ModeQuitConfirm
		h.updateShortcutLine()
	}
}

func (h *KeyHandler) handleQuitConfirm(key string) {
	switch key {
	case "y":
		h.Actions <- ActionQuit
	case "n":
		h.mode = h.prevMode
		h.updateShortcutLine()
	// all other keys are ignored
	}
}

// ForceQuit terminates the current subprocess and injects ActionQuit into the
// Actions channel so the orchestration goroutine exits cleanly. Safe to call
// from a signal handler goroutine.
func (h *KeyHandler) ForceQuit() {
	if h.cancel != nil {
		h.cancel()
	}
	select {
	case h.Actions <- ActionQuit:
	default:
	}
}

func (h *KeyHandler) updateShortcutLine() {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch h.mode {
	case ModeNormal:
		h.shortcutLine = NormalShortcuts
	case ModeError:
		h.shortcutLine = ErrorShortcuts
	case ModeQuitConfirm:
		h.shortcutLine = QuitConfirmPrompt
	}
}
