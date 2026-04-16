package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// logRingBufferCap is the maximum number of lines retained in the ring buffer
// (D22). Under stream-json, a single claude step can emit hundreds of lines;
// 2000 keeps an iteration's worth of chrome visible without significant memory
// cost.
const logRingBufferCap = 2000

// logContentStyle is applied to every streamed log line so the main content
// area renders in bright white, making subprocess output pop against the
// light-gray chrome (border, hrules, iteration line, shortcut footer).
var logContentStyle = lipgloss.NewStyle().Foreground(White)

// visualLine is one word-wrapped segment of a raw log line. The rawIdx and
// rawOffset fields provide stable back-references into the ring buffer so that
// scroll-position can be preserved across terminal resizes and future
// selection/copy work can reconstruct the original text without embedded
// wrap-induced newlines.
type visualLine struct {
	text      string // wrapped segment (plain text; logContentStyle applied at render time)
	raw       string // same as text for plain-text ring buffers; reserved for copy
	rawIdx    int    // index into m.lines
	rawOffset int    // byte offset within lines[rawIdx] where this segment starts
}

// logModel wraps a bubbles/viewport.Model and a 2000-entry ring buffer for
// streaming log lines. All mutations happen on the Bubble Tea Update goroutine.
type logModel struct {
	viewport    viewport.Model
	lines       []string     // ring buffer, cap logRingBufferCap; one entry per original logical line
	visualLines []visualLine // rebuilt on every rewrap; one entry per wrapped visual row
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

// rewrap rebuilds visualLines by word-wrapping each raw line to the given
// width. ansi.Wrap handles word-wrap with a hard-break fallback for over-long
// tokens in a single pass. When width < 1, ansi.Wrap returns the input
// unchanged, so each raw line produces exactly one visual segment — the safe
// pre-WindowSizeMsg path.
func (m *logModel) rewrap(width int) {
	m.visualLines = m.visualLines[:0]
	for rawIdx, rawLine := range m.lines {
		wrapped := ansi.Wrap(rawLine, width, " -")
		segments := strings.Split(wrapped, "\n")
		offset := 0
		for _, seg := range segments {
			m.visualLines = append(m.visualLines, visualLine{
				text:      seg,
				raw:       seg,
				rawIdx:    rawIdx,
				rawOffset: offset,
			})
			// Advance past the bytes of this segment then skip any whitespace
			// that ansi.Wrap consumed as the word-break separator. Hyphens are
			// kept in the segment output and not consumed, so only spaces need
			// to be skipped here.
			offset += len(seg)
			for offset < len(rawLine) && rawLine[offset] == ' ' {
				offset++
			}
		}
	}
}

// renderContent joins all visual-line text into a single string and wraps it
// in logContentStyle. The selection overlay (reverse-video highlighting) is
// not applied yet — that lands in a later ticket.
func (m logModel) renderContent() string {
	texts := make([]string, len(m.visualLines))
	for i, vl := range m.visualLines {
		texts[i] = vl.text
	}
	return logContentStyle.Render(strings.Join(texts, "\n"))
}

// Update handles incoming Bubble Tea messages. LogLinesMsg appends lines to the
// ring buffer and calls SetContent once per batch; tea.KeyMsg "home"/"end" jump
// to top/bottom; all other messages are forwarded to the underlying viewport.
func (m logModel) Update(msg tea.Msg) (logModel, tea.Cmd) {
	switch msg := msg.(type) {
	case LogLinesMsg:
		for _, line := range msg.Lines {
			line = strings.ReplaceAll(line, "\t", "    ")
			m.lines = append(m.lines, line)
		}
		if len(m.lines) > logRingBufferCap {
			m.lines = m.lines[len(m.lines)-logRingBufferCap:]
		}
		wasAtBottom := m.viewport.AtBottom()
		m.rewrap(m.viewport.Width)
		m.viewport.SetContent(m.renderContent())
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

// SetSize resizes the viewport. If the width changed, content is re-wrapped and
// the scroll position is restored to the same raw line (including intra-line
// segment offset) that was at the top of the viewport before the resize.
// Height-only changes skip the rewrap step.
func (m *logModel) SetSize(width, height int) {
	widthChanged := width != m.viewport.Width

	m.viewport.Width = width
	m.viewport.Height = height

	if !widthChanged {
		// Height-only change: no rewrap needed.
		return
	}

	// Snapshot the raw position at the top of the viewport before rewrapping.
	// Skip if there is no content or the offset is out of range.
	snapRawIdx := -1
	snapRawOffset := 0
	if len(m.visualLines) > 0 && m.viewport.YOffset < len(m.visualLines) {
		vl := m.visualLines[m.viewport.YOffset]
		snapRawIdx = vl.rawIdx
		snapRawOffset = vl.rawOffset
	}

	m.rewrap(width)
	m.viewport.SetContent(m.renderContent())

	if snapRawIdx < 0 {
		return
	}

	// Scan visualLines for the largest index i such that
	// visualLines[i].rawIdx == snapRawIdx && visualLines[i].rawOffset <= snapRawOffset.
	// This keeps the top visible row anchored to the same raw-line position
	// rather than jumping back to the first segment of that raw line.
	newYOffset := 0
	for i, vl := range m.visualLines {
		if vl.rawIdx == snapRawIdx && vl.rawOffset <= snapRawOffset {
			newYOffset = i
		}
	}
	m.viewport.SetYOffset(newYOffset)
}
