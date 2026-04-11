package ui

import tea "github.com/charmbracelet/bubbletea"

// keysModel is the Bubble Tea sub-model responsible for keyboard dispatch.
// It holds a reference to the KeyHandler (which owns mode state and the
// Actions channel) and translates tea.KeyMsg events into mode transitions
// and StepAction sends.
type keysModel struct {
	handler *KeyHandler
}

// newKeysModel creates a keysModel backed by the given KeyHandler.
func newKeysModel(handler *KeyHandler) keysModel {
	return keysModel{handler: handler}
}

// Update handles a single Bubble Tea message. Only tea.KeyMsg events are
// dispatched; all other messages are ignored and returned unchanged.
func (m keysModel) Update(msg tea.Msg) (keysModel, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch m.handler.Mode() {
	case ModeNormal:
		return m.handleNormal(key)
	case ModeError:
		return m.handleError(key)
	case ModeQuitConfirm:
		return m.handleQuitConfirm(key)
	case ModeQuitting:
		// All keys silently ignored so a user mashing keys during shutdown
		// can't inject a second ActionQuit or retrigger the cancel hook.
		return m, nil
	}
	return m, nil
}

func (m keysModel) handleNormal(key tea.KeyMsg) (keysModel, tea.Cmd) {
	switch key.String() {
	case "n":
		cancel := m.handler.Cancel()
		if cancel == nil {
			return m, nil
		}
		// Offload the 3-second blocking Terminate call to a goroutine via
		// tea.Cmd so the Update goroutine is not frozen while waiting for
		// SIGTERM / SIGKILL to propagate. We don't need a result message —
		// the workflow goroutine's next RunStep will observe WasTerminated
		// and unwind on its own.
		return m, func() tea.Msg {
			cancel()
			return nil // Bubble Tea ignores nil messages
		}
	case "q":
		m.handler.mu.Lock()
		m.handler.prevMode = m.handler.mode
		m.handler.mode = ModeQuitConfirm
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
		return m, nil
	}
	return m, nil
}

func (m keysModel) handleError(key tea.KeyMsg) (keysModel, tea.Cmd) {
	switch key.String() {
	case "c":
		// Blocking send: the orchestration goroutine is always blocked on
		// <-h.Actions when in error mode, so this drains immediately. The
		// channel capacity (10) provides a buffer against bursts from rapid
		// key repeats, but the invariant is that only one error-mode action
		// is in flight at a time.
		m.handler.Actions <- ActionContinue
	case "r":
		m.handler.Actions <- ActionRetry // same blocking-send invariant as "c" above
	case "q":
		m.handler.mu.Lock()
		m.handler.prevMode = m.handler.mode
		m.handler.mode = ModeQuitConfirm
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
	}
	return m, nil
}

func (m keysModel) handleQuitConfirm(key tea.KeyMsg) (keysModel, tea.Cmd) {
	switch key.String() {
	case "y":
		// Offload ForceQuit (which calls cancel, blocking up to 3s) via
		// tea.Cmd so the Update goroutine is not frozen. ForceQuit sets
		// ModeQuitting internally under h.mu, so the footer updates on the
		// next tick. ForceQuit is idempotent, so a second call from the
		// signal path racing with y-confirm is harmless.
		return m, func() tea.Msg {
			m.handler.ForceQuit()
			return nil
		}
	case "n":
		m.handler.mu.Lock()
		m.handler.mode = m.handler.prevMode
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
	case "esc":
		m.handler.mu.Lock()
		m.handler.mode = m.handler.prevMode
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
	}
	return m, nil
}
