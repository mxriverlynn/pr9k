package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
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
	sel         selection    // current text selection; zero value = no selection
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
			//
			// NOTE: This assumes only ASCII space (' ') is used as a word-break
			// separator, matching the breakpoints string " -" passed to
			// ansi.Wrap. Non-ASCII Unicode whitespace (e.g. no-break space,
			// em space) is not treated as a break point and does not need to be
			// skipped.
			//
			// TODO(#103): rawOffset is computed from len(seg) — the byte length
			// of the wrapped output segment — which diverges from the number of
			// raw-line bytes consumed when ansi.Wrap rewrites ANSI escape
			// sequences at wrap boundaries (e.g. inserting reset/re-open codes
			// to make each segment independently renderable). If raw lines ever
			// carry ANSI codes (e.g. coloured compiler output forwarded
			// verbatim), scroll-position restoration and future copy/select will
			// silently produce wrong offsets. Fix: strip ANSI from seg before
			// using its length for offset math, or walk the raw line directly to
			// match each segment's visible content.
			offset += len(seg)
			for offset < len(rawLine) && rawLine[offset] == ' ' {
				offset++
			}
		}
	}
}

// renderContent joins all visual-line text into a single string and wraps it
// in logContentStyle. When a selection is active or committed, cells within
// the selected range are rendered with reverse-video highlighting. An empty
// selection (anchor == cursor at the same position) still renders the single
// cursor cell with reverse video so the user has an immediate visual indicator
// of select mode.
func (m logModel) renderContent() string {
	showSel := m.sel.active || m.sel.committed
	if !showSel {
		texts := make([]string, len(m.visualLines))
		for i, vl := range m.visualLines {
			texts[i] = vl.text
		}
		return logContentStyle.Render(strings.Join(texts, "\n"))
	}

	reverseStyle := lipgloss.NewStyle().Reverse(true)
	start, end := m.sel.normalized()
	isEmpty := start.visualRow == end.visualRow && start.col == end.col

	texts := make([]string, len(m.visualLines))
	for i, vl := range m.visualLines {
		if isEmpty {
			// Empty selection: show a single cursor cell in reverse-video at
			// the cursor's visual row and column.
			if i != start.visualRow {
				texts[i] = vl.text
				continue
			}
			before, cursor, after := splitAtCol(vl.text, start.col)
			texts[i] = before + reverseStyle.Render(cursor) + after
			continue
		}

		// Non-empty range: highlight the selected cells on this row.
		// Rows fully inside the range are fully highlighted.
		// Start/end rows are partially highlighted.
		if i < start.visualRow || i > end.visualRow {
			texts[i] = vl.text
			continue
		}
		if i == start.visualRow && i == end.visualRow {
			// Selection confined to a single row.
			before, sel, after := splitAtCols(vl.text, start.col, end.col)
			texts[i] = before + reverseStyle.Render(sel) + after
		} else if i == start.visualRow {
			// Highlight from start.col to end of row.
			before, sel, _ := splitAtCols(vl.text, start.col, lipgloss.Width(vl.text))
			texts[i] = before + reverseStyle.Render(sel)
		} else if i == end.visualRow {
			// Highlight from start of row to end.col.
			_, sel, after := splitAtCols(vl.text, 0, end.col)
			texts[i] = reverseStyle.Render(sel) + after
		} else {
			// Middle row: fully highlighted.
			texts[i] = reverseStyle.Render(vl.text)
		}
	}
	return logContentStyle.Render(strings.Join(texts, "\n"))
}

// splitAtCol splits s at the given display-column index, returning the text
// before the column, the first grapheme cluster at that column (or a space if
// the row is empty or the column is past the end), and the text after.
func splitAtCol(s string, col int) (before, cursor, after string) {
	byteOff := colToByteOffset(s, col)
	if byteOff >= len(s) {
		return s, " ", ""
	}
	// Advance one rune.
	for _, r := range s[byteOff:] {
		size := len(string(r))
		return s[:byteOff], s[byteOff : byteOff+size], s[byteOff+size:]
	}
	return s, " ", ""
}

// splitAtCols splits s at two display-column boundaries, returning the
// three segments: before startCol, between startCol and endCol (the selection),
// and after endCol.
func splitAtCols(s string, startCol, endCol int) (before, sel, after string) {
	startByte := colToByteOffset(s, startCol)
	endByte := colToByteOffset(s, endCol)
	if startByte > len(s) {
		startByte = len(s)
	}
	if endByte > len(s) {
		endByte = len(s)
	}
	if startByte > endByte {
		startByte = endByte
	}
	return s[:startByte], s[startByte:endByte], s[endByte:]
}

// colToByteOffset converts a display-column index to a byte offset in s.
// It walks forward through s counting display cells (via lipgloss.Width per
// rune). Returns len(s) when col exceeds the string's total cell width.
func colToByteOffset(s string, col int) int {
	cellCount := 0
	byteOff := 0
	for _, r := range s {
		if cellCount >= col {
			break
		}
		w := runewidth.RuneWidth(r)
		cellCount += w
		byteOff += len(string(r))
	}
	return byteOff
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

// SelectedText returns the currently selected text reconstructed from raw
// ring-buffer lines. Returns "" when there is no selection or the selection
// has become invalid after ring-buffer eviction.
func (m logModel) SelectedText() string {
	if !m.sel.active && !m.sel.committed {
		return ""
	}
	start, end := m.sel.normalized()
	return extractText(m.lines, start, end)
}

// SetSelection replaces the current selection and re-renders the viewport
// content with the new selection overlay.
func (m logModel) SetSelection(sel selection) logModel {
	m.sel = sel
	m.viewport.SetContent(m.renderContent())
	return m
}

// ClearSelection removes the current selection and re-renders the viewport
// content without a selection overlay.
func (m logModel) ClearSelection() logModel {
	m.sel = selection{}
	m.viewport.SetContent(m.renderContent())
	return m
}

// initSelectionAtLastVisibleRow returns a new selection whose anchor and
// cursor are both at column 0 of the last visible visual row
// (YOffset + viewport.Height - 1, clamped to len(visualLines) - 1).
// The selection is marked active so visible() returns true and the cursor
// cell renders immediately.
// Returns an empty (zero-value) selection when there are no visual lines.
func (m logModel) initSelectionAtLastVisibleRow() selection {
	if len(m.visualLines) == 0 {
		return selection{}
	}
	lastRow := m.viewport.YOffset + m.viewport.Height - 1
	if lastRow >= len(m.visualLines) {
		lastRow = len(m.visualLines) - 1
	}
	if lastRow < 0 {
		lastRow = 0
	}
	vl := m.visualLines[lastRow]
	p := pos{
		rawIdx:    vl.rawIdx,
		rawOffset: vl.rawOffset,
		visualRow: lastRow,
		col:       0,
	}
	return selection{anchor: p, cursor: p, active: true}
}
