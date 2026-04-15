package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// heartbeatSilenceThreshold is the minimum silence duration before the
// heartbeat indicator is shown in the iteration line (D23).
const heartbeatSilenceThreshold = 15 * time.Second

// heartbeatTick returns a tea.Cmd that fires HeartbeatTickMsg after 1 second.
// Called from Init() and re-scheduled from Update() to keep the ticker alive.
func heartbeatTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return HeartbeatTickMsg(t)
	})
}

// headerModel wraps StatusHeader and applies header messages from the
// orchestration goroutine (sent via headerProxy → program.Send).
type headerModel struct {
	header *StatusHeader
}

func newHeaderModel(h *StatusHeader) headerModel {
	return headerModel{
		header: h,
	}
}

// apply mutates the underlying StatusHeader in response to a header message.
// All mutations happen on the Bubble Tea Update goroutine.
func (m headerModel) apply(msg tea.Msg) headerModel {
	switch msg := msg.(type) {
	case headerStepStateMsg:
		m.header.SetStepState(msg.idx, msg.state)
	case headerIterationLineMsg:
		m.header.RenderIterationLine(msg.iter, msg.max, msg.issue)
	case headerInitializeLineMsg:
		m.header.RenderInitializeLine(msg.stepNum, msg.stepCount, msg.stepName)
	case headerFinalizeLineMsg:
		m.header.RenderFinalizeLine(msg.stepNum, msg.stepCount, msg.stepName)
	case headerPhaseStepsMsg:
		m.header.SetPhaseSteps(msg.names)
	}
	return m
}

// iterLine returns the current iteration line string for title construction.
func (m headerModel) iterLine() string {
	return m.header.IterationLine
}

// Model is the root Bubble Tea model. It holds three sub-models — one for the
// header (checkbox grid), one for the scrollable log panel, and one for
// keyboard dispatch — plus the terminal dimensions and a version label for the
// shortcut footer.
type Model struct {
	header          headerModel
	log             logModel
	keys            keysModel
	width           int
	height          int
	versionLabel    string
	heartbeat       HeartbeatReader // nil when heartbeat is disabled
	heartbeatSuffix string          // current " ⋯ thinking (Ns)" suffix; empty when inactive
}

// NewModel constructs the root Model. initialHeader must be pre-populated
// with the first phase's step set and active state so the first rendered frame
// shows real content. keyHandler owns the mode state machine.
func NewModel(initialHeader *StatusHeader, keyHandler *KeyHandler, versionLabel string) Model {
	return Model{
		header:       newHeaderModel(initialHeader),
		log:          newLogModel(0, 0), // sized on first tea.WindowSizeMsg
		keys:         newKeysModel(keyHandler),
		versionLabel: versionLabel,
	}
}

// WithHeartbeat returns a copy of the Model with the given HeartbeatReader
// installed. Call this after NewModel in main.go to enable the D23 heartbeat
// indicator. Passing nil disables the indicator (same as the zero value).
//
// Tests that do not need heartbeat behaviour should use NewModel directly
// (which leaves heartbeat nil), so Init() still returns nil and no ticker is
// started.
func (m Model) WithHeartbeat(h HeartbeatReader) Model {
	m.heartbeat = h
	return m
}

// Init satisfies tea.Model. When a HeartbeatReader is set, Init starts the
// 1-second heartbeat ticker (D23). Otherwise returns nil so tests that do not
// inject a heartbeat reader remain unaffected.
func (m Model) Init() tea.Cmd {
	if m.heartbeat != nil {
		return heartbeatTick()
	}
	return nil
}

// Update routes incoming Bubble Tea messages to the appropriate sub-model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		var kcmd tea.Cmd
		m.keys, kcmd = m.keys.Update(msg)
		cmds = append(cmds, kcmd)
		var lcmd tea.Cmd
		m.log, lcmd = m.log.Update(msg) // viewport scroll
		cmds = append(cmds, lcmd)

	case LogLinesMsg:
		var lcmd tea.Cmd
		m.log, lcmd = m.log.Update(msg)
		cmds = append(cmds, lcmd)

	case tea.MouseMsg:
		// Forward mouse events (wheel up/down, etc.) to the log viewport.
		// bubbles/viewport enables MouseWheelEnabled by default, so wheel
		// press events scroll the log body by MouseWheelDelta (3) lines.
		var lcmd tea.Cmd
		m.log, lcmd = m.log.Update(msg)
		cmds = append(cmds, lcmd)

	case headerStepStateMsg, headerPhaseStepsMsg:
		m.header = m.header.apply(msg)

	case headerIterationLineMsg, headerInitializeLineMsg, headerFinalizeLineMsg:
		prevLine := m.header.iterLine()
		m.header = m.header.apply(msg)
		newLine := m.header.iterLine()
		if newLine != prevLine {
			cmds = append(cmds, tea.SetWindowTitle(m.titleString()))
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Chrome rows consumed: top border (1) + grid rows +
		// hrule below grid (1) + hrule below log (1) + footer (1) +
		// bottom border (1). The iteration line is rendered inside the
		// top border as the title, so it does not consume an inner row.
		gridRows := len(m.header.header.Rows)
		chromeRows := 1 + gridRows + 1 + 1 + 1 + 1
		vpHeight := m.height - chromeRows
		if vpHeight < 1 {
			vpHeight = 1
		}
		vpWidth := m.width - 2 // inside border
		if vpWidth < 1 {
			vpWidth = 1
		}
		m.log.SetSize(vpWidth, vpHeight)
		var lcmd tea.Cmd
		m.log, lcmd = m.log.Update(msg)
		cmds = append(cmds, lcmd)

	case HeartbeatTickMsg:
		// Update the heartbeat suffix (D23) and reschedule for the next second.
		if m.heartbeat != nil {
			silentFor, active := m.heartbeat.HeartbeatSilence()
			if active && silentFor >= heartbeatSilenceThreshold {
				m.heartbeatSuffix = fmt.Sprintf("  ⋯ thinking (%ds)", int(silentFor.Seconds()))
			} else {
				m.heartbeatSuffix = ""
			}
		}
		cmds = append(cmds, heartbeatTick())

	case tea.QuitMsg:
		return m, tea.Quit
	}

	return m, tea.Batch(cmds...)
}

// View assembles the complete TUI output. The frame is hand-built row by
// row (rather than wrapped in a single lipgloss border) so the horizontal
// rules between grid/log and log/footer can use ├─┤ T-junction glyphs
// that visually connect to the │ side borders.
func (m Model) View() string {
	title := m.titleString()
	innerWidth := m.width - 2
	if innerWidth < 0 {
		innerWidth = 0
	}

	gray := lipgloss.NewStyle().Foreground(LightGray)
	vbar := gray.Render("│")

	// wrapLine wraps a single content line in side borders, truncating to
	// innerWidth and right-padding with spaces so the right border stays
	// vertically aligned across all rows.
	wrapLine := func(content string) string {
		if innerWidth <= 0 {
			return vbar + vbar
		}
		truncated := lipgloss.NewStyle().MaxWidth(innerWidth).Render(content)
		pad := innerWidth - lipgloss.Width(truncated)
		if pad < 0 {
			pad = 0
		}
		return vbar + truncated + strings.Repeat(" ", pad) + vbar
	}

	hruleLine := gray.Render("├" + strings.Repeat("─", innerWidth) + "┤")
	bottomBorder := gray.Render("╰" + strings.Repeat("─", innerWidth) + "╯")

	var sb strings.Builder

	// Top border with dynamic title.
	sb.WriteString(m.renderTopBorder(title))
	sb.WriteString("\n")

	// Checkbox grid — the iteration line lives in the top border as the
	// title, so the grid is the first row below the top border.
	//
	// Compute cell width so the grid fills the full terminal width.
	// Each row has HeaderCols cells separated by 2-space gaps.  The cell
	// width is derived from innerWidth so the grid stretches edge-to-edge,
	// falling back to the widest content width when the terminal is too
	// narrow to hold all cells.
	contentMaxWidth := 0
	for r := range m.header.header.Rows {
		for c := range HeaderCols {
			if w := lipgloss.Width(m.header.header.Rows[r][c]); w > contentMaxWidth {
				contentMaxWidth = w
			}
		}
	}
	separatorWidth := (HeaderCols - 1) * 2
	termCellWidth := (innerWidth - separatorWidth) / HeaderCols
	cellWidth := max(termCellWidth, contentMaxWidth)

	for r := range m.header.header.Rows {
		var row strings.Builder
		for c := range HeaderCols {
			if c > 0 {
				row.WriteString("  ")
			}
			prefix := lipgloss.NewStyle().Foreground(m.header.header.NameColors[r][c]).Render(m.header.header.Prefixes[r][c])
			marker := lipgloss.NewStyle().Foreground(m.header.header.MarkerColors[r][c]).Render(m.header.header.Markers[r][c])
			suffix := lipgloss.NewStyle().Foreground(m.header.header.NameColors[r][c]).Render(m.header.header.Suffixes[r][c])
			row.WriteString(prefix + marker + suffix)
			// Pad to cellWidth so all columns are equally wide and
			// the grid fills the terminal width.
			if pad := cellWidth - lipgloss.Width(m.header.header.Rows[r][c]); pad > 0 {
				row.WriteString(strings.Repeat(" ", pad))
			}
		}
		sb.WriteString(wrapLine(row.String()))
		sb.WriteString("\n")
	}

	// HRule between grid and log.
	sb.WriteString(hruleLine)
	sb.WriteString("\n")

	// Log panel (viewport) — split into lines and wrap each in sidebars.
	// bubbles/viewport pads its output to the configured Height, so we get
	// exactly vpHeight rows here.
	for _, line := range strings.Split(m.log.View(), "\n") {
		sb.WriteString(wrapLine(line))
		sb.WriteString("\n")
	}

	// HRule between log and footer.
	sb.WriteString(hruleLine)
	sb.WriteString("\n")

	// Shortcut footer: shortcut bar on the left, version label on the right.
	shortcut := m.keys.handler.ShortcutLine()
	footerWidth := innerWidth
	versionWidth := lipgloss.Width(m.versionLabel)
	shortcutWidth := footerWidth - versionWidth - 1
	if shortcutWidth < 0 {
		shortcutWidth = 0
	}
	// Color the shortcut line (mapped keys white, descriptions gray), then
	// truncate — MaxWidth is ANSI-aware so coloring survives truncation.
	shortcutTrunc := lipgloss.NewStyle().MaxWidth(shortcutWidth).Render(colorShortcutLine(shortcut))
	spacerWidth := footerWidth - lipgloss.Width(shortcutTrunc) - versionWidth
	if spacerWidth < 0 {
		spacerWidth = 0
	}
	footer := shortcutTrunc +
		strings.Repeat(" ", spacerWidth) +
		lipgloss.NewStyle().Foreground(White).Render(m.versionLabel)
	sb.WriteString(wrapLine(footer))
	sb.WriteString("\n")

	// Bottom border.
	sb.WriteString(bottomBorder)

	return sb.String()
}

// titleString builds the OS window title and in-TUI border title from the
// current header iteration line. When the heartbeat suffix is active (D23),
// it is appended after the iteration detail — e.g.
// "Power-Ralph.9000 — Iteration 2/5 — Issue #42  ⋯ thinking (17s)".
func (m Model) titleString() string {
	iter := m.header.iterLine()
	if iter == "" {
		return AppTitle
	}
	return AppTitle + " — " + iter + m.heartbeatSuffix
}

// colorShortcutLine applies the footer shortcut bar's two-tone palette: the
// mapped key token at the start of each "  "-separated group renders white,
// and its trailing description renders gray. When the footer instead shows
// a status message (quit-confirm prompt, quitting line) the whole string
// renders white so it reads as a foreground message rather than key-mapping
// chrome — with one exception: within the quit-confirm prompt, the embedded
// AppTitle substring renders green to match the top-border title's brand
// color.
func colorShortcutLine(s string) string {
	white := lipgloss.NewStyle().Foreground(White)
	if s == QuitConfirmPrompt {
		green := lipgloss.NewStyle().Foreground(Green)
		if idx := strings.Index(s, AppTitle); idx >= 0 {
			return white.Render(s[:idx]) +
				green.Render(AppTitle) +
				white.Render(s[idx+len(AppTitle):])
		}
		return white.Render(s)
	}
	if s == QuittingLine {
		return white.Render(s)
	}
	gray := lipgloss.NewStyle().Foreground(LightGray)
	groups := strings.Split(s, "  ")
	for i, g := range groups {
		if idx := strings.IndexByte(g, ' '); idx >= 0 {
			groups[i] = white.Render(g[:idx]) + gray.Render(g[idx:])
		} else {
			groups[i] = white.Render(g)
		}
	}
	return strings.Join(groups, gray.Render("  "))
}

// colorTitle applies the top-border title's two-tone palette: the app name
// (everything before the first " — " separator) renders green, and the
// iteration detail that follows renders white. When the title has no
// separator (e.g. the bare app name before any iteration starts), the
// whole string renders green.
func colorTitle(title string) string {
	const sep = " — "
	green := lipgloss.NewStyle().Foreground(Green)
	white := lipgloss.NewStyle().Foreground(White)
	if idx := strings.Index(title, sep); idx >= 0 {
		return green.Render(title[:idx]) + white.Render(title[idx:])
	}
	return green.Render(title)
}

// renderTopBorder constructs the hand-built top border row with the dynamic
// title embedded. When the terminal is too narrow to fit even the corners,
// a plain rule is returned.
//
// Target shape: "╭── Power-Ralph.9000 — Iteration 2/5 — Issue #42 ─ … ─╮"
func (m Model) renderTopBorder(title string) string {
	const tl, tr, h = "╭", "╮", "─"
	innerWidth := m.width - 2 // subtract corner glyphs
	const leadDashes = 2
	titleBudget := innerWidth - leadDashes - 1
	if titleBudget < 0 || m.width == 0 {
		// Terminal is so narrow we can't fit "╭──╮". Emit a plain rule.
		rule := strings.Repeat(h, max(innerWidth, 0))
		return lipgloss.NewStyle().Foreground(LightGray).Render(tl + rule + tr)
	}

	// Do width math on the plain title, then apply coloring as the last
	// step so the visible width stays accurate regardless of ANSI codes.
	plainTitle := title
	plainSegment := " " + plainTitle + " "
	titleWidth := lipgloss.Width(plainSegment)
	if titleWidth > titleBudget {
		// Title overflows: truncate to titleBudget-2 (leave room for the two
		// surrounding spaces) using Lip Gloss MaxWidth (rune-and-ANSI-aware),
		// then re-wrap in the spacer pair.
		plainTitle = lipgloss.NewStyle().MaxWidth(titleBudget - 2).Render(plainTitle)
		plainSegment = " " + plainTitle + " "
		titleWidth = lipgloss.Width(plainSegment)
	}
	titleSegment := " " + colorTitle(plainTitle) + " "

	fillCount := innerWidth - leadDashes - titleWidth
	if fillCount < 0 {
		fillCount = 0
	}

	grayStyle := lipgloss.NewStyle().Foreground(LightGray)
	return grayStyle.Render(tl+strings.Repeat(h, leadDashes)) +
		titleSegment +
		grayStyle.Render(strings.Repeat(h, fillCount)+tr)
}
