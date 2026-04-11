package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// logModel wraps a bubbles/viewport.Model and a 500-entry ring buffer for
// streaming log lines. All mutations happen on the Bubble Tea Update goroutine.
type logModel struct {
	viewport viewport.Model
	lines    []string // ring buffer, cap 500
}

// newLogModel constructs a logModel with a custom KeyMap that removes f/b/u/d
// as a forward-compatibility guard against future keysModel shortcut
// collisions. Home/End are handled directly in Update.
func newLogModel(width, height int) logModel {
	vp := viewport.New(width, height)
	vp.KeyMap = logViewportKeyMap()
	return logModel{viewport: vp}
}

// logViewportKeyMap returns a viewport.KeyMap that removes f, b, u, d, space
// to avoid future keysModel shortcut collisions, and keeps pgup/pgdn, up/down
// (↑/k and ↓/j). Home/End are handled directly in logModel.Update.
func logViewportKeyMap() viewport.KeyMap {
	return viewport.KeyMap{
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
			key.WithHelp("PgDn", "page down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("PgUp", "page up"),
		),
		HalfPageUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("ctrl+u", "½ page up"),
		),
		HalfPageDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "½ page down"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
	}
}

// Update handles incoming Bubble Tea messages. LogLinesMsg appends lines to the
// ring buffer and calls SetContent once per batch; tea.KeyMsg "home"/"end" jump
// to top/bottom; all other messages are forwarded to the underlying viewport.
func (m logModel) Update(msg tea.Msg) (logModel, tea.Cmd) {
	switch msg := msg.(type) {
	case LogLinesMsg:
		m.lines = append(m.lines, msg.Lines...)
		if len(m.lines) > 500 {
			m.lines = m.lines[len(m.lines)-500:]
		}
		wasAtBottom := m.viewport.AtBottom()
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		if wasAtBottom {
			m.viewport.GotoBottom()
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "home":
			m.viewport.GotoTop()
			return m, nil
		case "end":
			m.viewport.GotoBottom()
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View renders the viewport content.
func (m logModel) View() string {
	return m.viewport.View()
}

// SetSize resizes the viewport.
func (m *logModel) SetSize(width, height int) {
	m.viewport.Width = width
	m.viewport.Height = height
}
