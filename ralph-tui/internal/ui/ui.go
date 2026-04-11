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

// AppTitle is the canonical display name of the application. Use this
// constant anywhere the app's name appears in user-facing text (top-border
// title, quit-confirm prompt, etc.) rather than hardcoding the string —
// renaming the app should require exactly one edit here.
const AppTitle = "Power-Ralph.9000"

const (
	NormalShortcuts   = "↑/k up  ↓/j down  n next step  q quit"
	ErrorShortcuts    = "c continue  r retry  q quit"
	QuitConfirmPrompt = "Quit " + AppTitle + "? (y/n, esc to cancel)"
	QuittingLine      = "Quitting..."
)

// KeyHandler is a state machine that tracks keyboard mode
// (normal / error / quit-confirm / quitting) and communicates user decisions
// to the orchestration goroutine via the Actions channel.
// Dispatch logic lives in keysModel (internal/ui/keys.go), which translates
// tea.KeyMsg events into mode transitions and Actions sends.
type KeyHandler struct {
	mode         Mode   // protected by mu
	prevMode     Mode   // protected by mu
	cancel       func() // cancels the current subprocess (used by n in normal mode)
	Actions      chan StepAction
	mu           sync.Mutex
	shortcutLine string // protected by mu; use ShortcutLine() to access
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

// Mode returns the current keyboard dispatch mode.
// Safe to call from any goroutine.
func (h *KeyHandler) Mode() Mode {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.mode
}

// Cancel returns the cancel function under h.mu so keysModel.handleNormal can
// read it safely on the Update goroutine without racing the workflow goroutine
// that may call NewKeyHandler or SetSender between steps.
func (h *KeyHandler) Cancel() func() {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cancel
}

// SetMode switches the handler to the given mode and updates ShortcutLine.
// Use this when the orchestration goroutine changes workflow state
// (e.g., a step fails → SetMode(ModeError)).
//
// ModeQuitConfirm should only be entered via keysModel's q handler, not via
// SetMode, because the q handler saves prevMode so that Escape can restore it.
// Calling SetMode(ModeQuitConfirm) directly leaves prevMode at its zero value
// (ModeNormal), so Escape would always restore ModeNormal regardless of the
// actual previous mode.
func (h *KeyHandler) SetMode(mode Mode) {
	h.mu.Lock()
	h.mode = mode
	h.updateShortcutLineLocked()
	h.mu.Unlock()
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
	default:
		// Unknown mode: reset to normal shortcuts so the shortcut bar stays
		// usable if a future mode is added without updating this switch.
		h.shortcutLine = NormalShortcuts
	}
}
