package ui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

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
	case ModeNextConfirm:
		return m.handleNextConfirm(key)
	case ModeDone:
		return m.handleDone(key)
	case ModeSelect:
		return m.handleSelect(key)
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
		m.handler.mu.Lock()
		m.handler.prevMode = m.handler.mode
		m.handler.mode = ModeNextConfirm
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
		return m, nil
	case "q":
		m.handler.mu.Lock()
		m.handler.prevMode = m.handler.mode
		m.handler.mode = ModeQuitConfirm
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
		return m, nil
	case "v":
		// v enters ModeSelect. The guard for len(lines) == 0 and the
		// selection initialisation (initSelectionAtLastVisibleRow) are
		// handled in model.go's root Update, which has access to logModel.
		m.handler.mu.Lock()
		m.handler.prevMode = m.handler.mode
		m.handler.mode = ModeSelect
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

func (m keysModel) handleNextConfirm(key tea.KeyMsg) (keysModel, tea.Cmd) {
	switch key.String() {
	case "y":
		cancel := m.handler.Cancel()
		m.handler.mu.Lock()
		m.handler.mode = m.handler.prevMode
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
		if cancel == nil {
			return m, nil
		}
		return m, func() tea.Msg {
			cancel()
			return nil
		}
	case "n", "esc":
		m.handler.mu.Lock()
		m.handler.mode = m.handler.prevMode
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
	}
	return m, nil
}

func (m keysModel) handleDone(key tea.KeyMsg) (keysModel, tea.Cmd) {
	switch key.String() {
	case "q":
		m.handler.mu.Lock()
		m.handler.prevMode = m.handler.mode
		m.handler.mode = ModeQuitConfirm
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
	case "v":
		// v enters ModeSelect. See handleNormal for the same pattern.
		m.handler.mu.Lock()
		m.handler.prevMode = m.handler.mode
		m.handler.mode = ModeSelect
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
	}
	return m, nil
}

// handleSelect handles key events in ModeSelect. Esc returns to the prior
// mode; q enters ModeQuitConfirm; y/Enter transitions back to the prior mode
// so model.go can perform the copy (it has access to logModel.SelectedText()).
// Cursor movement keys (hjkl/arrows, 0/$, shift+↑↓, J/K, PgUp/PgDn) are
// handled by logModel.handleSelectKey via model.go after this handler returns.
func (m keysModel) handleSelect(key tea.KeyMsg) (keysModel, tea.Cmd) {
	switch key.String() {
	case "esc":
		// Return to the pre-select mode. Selection clearing is handled
		// immediately by model.go's post-dispatch guard in the same Update
		// call. The prevObservedMode guard covers the external SetMode path.
		m.handler.mu.Lock()
		m.handler.mode = m.handler.prevMode
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
	case "q":
		// The pre-Select idle mode (Normal or Done) is already saved in
		// prevMode from when `v` was pressed. Do not overwrite it — Esc from
		// QuitConfirm must restore the real idle mode, not ModeSelect itself.
		// Selection clearing happens via model.go's post-dispatch guard.
		m.handler.mu.Lock()
		m.handler.mode = ModeQuitConfirm
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
	case "y", "enter":
		// Return to the pre-select mode. The actual copy (clipboard write,
		// feedback log line) is performed by model.go's routing after key
		// dispatch, which has access to logModel.SelectedText(). Selection
		// clearing also happens there via the post-dispatch guard.
		m.handler.mu.Lock()
		m.handler.mode = m.handler.prevMode
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
	}
	return m, nil
}

// copySelectedText performs the clipboard copy for a committed selection and
// returns a tea.Cmd that appends the appropriate feedback log line. Called by
// model.go after y/Enter exits ModeSelect. text is the raw selected text
// (already extracted by model.go before ClearSelection is called).
//
// If text is empty, no copy is attempted and nil is returned (silent no-op).
func copySelectedText(text string) tea.Cmd {
	if text == "" {
		return nil
	}
	err := CopyToClipboard(text)
	var line string
	if err == nil {
		line = fmt.Sprintf("[copied %d chars]", len(text))
	} else {
		line = "[copy failed: install xclip/xsel or run in a terminal that supports OSC 52]"
	}
	return func() tea.Msg {
		return LogLinesMsg{Lines: []string{line}}
	}
}

func (m keysModel) handleQuitConfirm(key tea.KeyMsg) (keysModel, tea.Cmd) {
	switch key.String() {
	case "y":
		// Offload ForceQuit (which calls cancel, blocking up to 3s) via
		// tea.Cmd so the Update goroutine is not frozen. ForceQuit sets
		// ModeQuitting internally under h.mu, so the footer updates on the
		// next tick. ForceQuit is idempotent, so a second call from the
		// signal path racing with y-confirm is harmless. Return tea.QuitMsg
		// so the TUI exits even when there is no workflow goroutine to call
		// program.Quit() (e.g. ModeDone).
		return m, func() tea.Msg {
			m.handler.ForceQuit()
			return tea.QuitMsg{}
		}
	case "n", "esc":
		m.handler.mu.Lock()
		m.handler.mode = m.handler.prevMode
		m.handler.updateShortcutLineLocked()
		m.handler.mu.Unlock()
	}
	return m, nil
}
