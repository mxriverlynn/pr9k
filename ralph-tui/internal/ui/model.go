package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// headerModel wraps StatusHeader and applies header messages from the
// orchestration goroutine (sent via headerProxy → program.Send).
type headerModel struct {
	header        *StatusHeader
	iterationLine string // mirrored from header.IterationLine for title tracking
}

func newHeaderModel(h *StatusHeader) headerModel {
	return headerModel{
		header:        h,
		iterationLine: h.IterationLine,
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
		m.iterationLine = m.header.IterationLine
	case headerInitializeLineMsg:
		m.header.RenderInitializeLine(msg.stepNum, msg.stepCount, msg.stepName)
		m.iterationLine = m.header.IterationLine
	case headerFinalizeLineMsg:
		m.header.RenderFinalizeLine(msg.stepNum, msg.stepCount, msg.stepName)
		m.iterationLine = m.header.IterationLine
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
	header       headerModel
	log          logModel
	keys         keysModel
	width        int
	height       int
	versionLabel string
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

// Init satisfies tea.Model. No startup commands needed; the workflow goroutine
// runs independently of the program event loop.
func (m Model) Init() tea.Cmd {
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
		// Chrome rows consumed: top border (1) + iteration line (1) + grid rows +
		// first hrule (1) + bottom hrule (1) + footer (1) + bottom border (1).
		gridRows := len(m.header.header.Rows)
		chromeRows := 1 + 1 + gridRows + 1 + 1 + 1 + 1
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

	case tea.QuitMsg:
		return m, tea.Quit
	}

	return m, tea.Batch(cmds...)
}

// View assembles the complete TUI output.
func (m Model) View() string {
	title := m.titleString()
	innerWidth := m.width - 2
	if innerWidth < 0 {
		innerWidth = 0
	}

	borderStyle := lipgloss.NewStyle().
		Foreground(LightGray).
		Border(lipgloss.RoundedBorder()).
		BorderTop(false).
		BorderForeground(LightGray).
		Width(innerWidth)

	var sb strings.Builder

	// Top border with dynamic title.
	sb.WriteString(m.renderTopBorder(title))
	sb.WriteString("\n")

	// Inner content: assembled as a string, then wrapped in the partial border.
	var inner strings.Builder

	// Iteration line.
	inner.WriteString(lipgloss.NewStyle().Foreground(LightGray).Render(m.header.header.IterationLine))
	inner.WriteString("\n")

	// HRule.
	inner.WriteString(lipgloss.NewStyle().Foreground(LightGray).Render(strings.Repeat("─", innerWidth)))
	inner.WriteString("\n")

	// Checkbox grid.
	for r := range m.header.header.Rows {
		for c := range HeaderCols {
			if c > 0 {
				inner.WriteString("  ")
			}
			prefix := lipgloss.NewStyle().Foreground(m.header.header.NameColors[r][c]).Render(m.header.header.Prefixes[r][c])
			marker := lipgloss.NewStyle().Foreground(m.header.header.MarkerColors[r][c]).Render(m.header.header.Markers[r][c])
			suffix := lipgloss.NewStyle().Foreground(m.header.header.NameColors[r][c]).Render(m.header.header.Suffixes[r][c])
			inner.WriteString(prefix + marker + suffix)
		}
		inner.WriteString("\n")
	}

	// HRule.
	inner.WriteString(lipgloss.NewStyle().Foreground(LightGray).Render(strings.Repeat("─", innerWidth)))
	inner.WriteString("\n")

	// Log panel (viewport).
	inner.WriteString(m.log.View())
	inner.WriteString("\n")

	// HRule.
	inner.WriteString(lipgloss.NewStyle().Foreground(LightGray).Render(strings.Repeat("─", innerWidth)))
	inner.WriteString("\n")

	// Shortcut footer: shortcut bar on the left, version label on the right.
	shortcut := m.keys.handler.ShortcutLine()
	footerWidth := innerWidth
	versionWidth := lipgloss.Width(m.versionLabel)
	shortcutWidth := footerWidth - versionWidth - 1
	if shortcutWidth < 0 {
		shortcutWidth = 0
	}
	shortcutTrunc := lipgloss.NewStyle().MaxWidth(shortcutWidth).Foreground(LightGray).Render(shortcut)
	spacerWidth := footerWidth - lipgloss.Width(shortcutTrunc) - versionWidth
	if spacerWidth < 0 {
		spacerWidth = 0
	}
	footer := shortcutTrunc +
		strings.Repeat(" ", spacerWidth) +
		lipgloss.NewStyle().Foreground(LightGray).Render(m.versionLabel)
	inner.WriteString(footer)

	// Wrap inner content in the border (sides + bottom, no top — we hand-built it).
	sb.WriteString(borderStyle.Render(inner.String()))

	return sb.String()
}

// titleString builds the OS window title and in-TUI border title from the
// current header iteration line.
func (m Model) titleString() string {
	if m.header.iterLine() == "" {
		return "ralph-tui"
	}
	return "ralph-tui — " + m.header.iterLine()
}

// renderTopBorder constructs the hand-built top border row with the dynamic
// title embedded. When the terminal is too narrow to fit even the corners,
// a plain rule is returned.
//
// Target shape: "╭── ralph-tui — Iteration 2/5 — Issue #42 ─ … ─╮"
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

	titleSegment := " " + title + " "
	titleWidth := lipgloss.Width(titleSegment)
	if titleWidth > titleBudget {
		// Title overflows: truncate to titleBudget-2 (leave room for the two
		// surrounding spaces) using Lip Gloss MaxWidth (rune-and-ANSI-aware),
		// then re-wrap in the spacer pair.
		inner := lipgloss.NewStyle().MaxWidth(titleBudget - 2).Render(title)
		titleSegment = " " + inner + " "
		titleWidth = lipgloss.Width(titleSegment)
	}

	fillCount := innerWidth - leadDashes - titleWidth
	if fillCount < 0 {
		fillCount = 0
	}

	return lipgloss.NewStyle().Foreground(LightGray).Render(
		tl + strings.Repeat(h, leadDashes) + titleSegment + strings.Repeat(h, fillCount) + tr,
	)
}
