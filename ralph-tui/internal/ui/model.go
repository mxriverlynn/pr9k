package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
	header           headerModel
	log              logModel
	keys             keysModel
	width            int
	height           int
	versionLabel     string
	prevObservedMode Mode   // used to detect external SetMode transitions out of ModeSelect
	triggerFn        func() // called once per mode transition; nil means no-op
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

// WithHeartbeat installs a HeartbeatReader on the underlying StatusHeader.
// Convenience method for tests: production code should call
// header.SetHeartbeatReader(runner) directly before constructing the model.
func (m Model) WithHeartbeat(h HeartbeatReader) Model {
	m.header.header.SetHeartbeatReader(h)
	return m
}

// WithModeTrigger installs a mode-transition trigger on the Model. fn is
// called exactly once in Model.Update whenever the mode changes from one
// Update call to the next. When the status line is disabled, pass nil (safe).
func (m Model) WithModeTrigger(fn func()) Model {
	m.triggerFn = fn
	return m
}

// Init satisfies tea.Model. Returns nil — the 1-second HeartbeatTickMsg
// ticker is owned by an explicit goroutine in main.go (D23).
func (m Model) Init() tea.Cmd {
	return nil
}

// Update routes incoming Bubble Tea messages to the appropriate sub-model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Mode-change guard: if the mode transitioned away from ModeSelect since the
	// last Update (covers external SetMode calls from the orchestration goroutine),
	// clear any lingering selection overlay so a stale reverse-video highlight
	// never lingers.
	//
	// prevObservedMode is updated at the end of Update (after all dispatch) so
	// it reflects the mode as it was when control last returned to the caller —
	// i.e., the mode seen by the previous rendered frame.
	currentMode := m.keys.handler.Mode()
	if m.prevObservedMode == ModeSelect && currentMode != ModeSelect {
		m.log = m.log.ClearSelection()
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Capture mode before key dispatch so the routing guard uses the mode
		// that was active when this key arrived, not the post-dispatch mode.
		modeBeforeKey := m.keys.handler.Mode()

		// Clear the "just released" committed-shortcut flag on any key in ModeSelect.
		// The flag is set by a mouse drag release and cleared on the next event
		// so that SelectShortcuts is restored when the user resumes keyboard control.
		if modeBeforeKey == ModeSelect {
			m.keys.handler.mu.Lock()
			m.keys.handler.clearJustReleasedLocked()
			m.keys.handler.mu.Unlock()
		}

		var kcmd tea.Cmd
		m.keys, kcmd = m.keys.Update(msg)
		cmds = append(cmds, kcmd)

		// Post-dispatch: handle v entering ModeSelect. keys.go sets the mode
		// unconditionally; we revert if the log buffer is empty, or initialize
		// the selection cursor otherwise.
		if m.keys.handler.Mode() == ModeSelect && modeBeforeKey != ModeSelect {
			if len(m.log.lines) == 0 {
				// Revert: can't select in an empty viewport.
				m.keys.handler.mu.Lock()
				m.keys.handler.mode = m.keys.handler.prevMode
				m.keys.handler.updateShortcutLineLocked()
				m.keys.handler.mu.Unlock()
			} else {
				m.log = m.log.SetSelection(m.log.initSelectionAtLastVisibleRow())
			}
		}

		// Selection movement: if still in ModeSelect after key dispatch,
		// delegate movement keys to the log panel. Mode transitions (esc, q)
		// are handled by handleSelect in keys.go and have already changed the
		// mode before this check.
		if modeBeforeKey == ModeSelect && m.keys.handler.Mode() == ModeSelect {
			var lcmd tea.Cmd
			m.log, lcmd = m.log.handleSelectKey(msg)
			cmds = append(cmds, lcmd)
		}

		// Immediate selection clear: if a key transitioned the mode away from
		// ModeSelect (e.g., Esc, y, Enter), clear the selection overlay now so
		// there is no single-frame stale highlight. For y/Enter, extract the
		// selected text before clearing and enqueue the clipboard copy + feedback
		// log line. The prevObservedMode guard at the top of Update covers the
		// external SetMode path (orchestration goroutine).
		if modeBeforeKey == ModeSelect && m.keys.handler.Mode() != ModeSelect {
			switch msg.String() {
			case "y", "enter":
				// Extract text before ClearSelection removes the selection state.
				text := m.log.SelectedText()
				cmds = append(cmds, copySelectedText(text))
			}
			m.log = m.log.ClearSelection()
		}

		// Key routing guard: in ModeSelect, skip the log.Update forward for
		// tea.KeyMsg. handleSelect has sole authority over key dispatch in this
		// mode; viewport scrolling during selection is driven by the movement
		// methods above, not by double-dispatched key events.
		// Use the pre-dispatch mode so that a key that exits ModeSelect (e.g.,
		// Esc) also doesn't double-dispatch to the viewport.
		if modeBeforeKey != ModeSelect {
			var lcmd tea.Cmd
			m.log, lcmd = m.log.Update(msg) // viewport scroll
			cmds = append(cmds, lcmd)
		}

	case LogLinesMsg:
		var lcmd tea.Cmd
		m.log, lcmd = m.log.Update(msg)
		cmds = append(cmds, lcmd)

	case tea.MouseMsg:
		// On any mouse event while in ModeSelect, clear the "just released"
		// committed-shortcut flag so that SelectShortcuts is restored before
		// the new event is processed (a subsequent press or wheel after a drag
		// release should not keep showing SelectCommittedShortcuts).
		if m.keys.handler.Mode() == ModeSelect {
			m.keys.handler.mu.Lock()
			m.keys.handler.clearJustReleasedLocked()
			m.keys.handler.mu.Unlock()
		}

		// Wheel events: always forward to the viewport for scrolling in any mode.
		// The viewport's built-in wheel handler scrolls MouseWheelDelta (3) lines.
		if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown ||
			msg.Button == tea.MouseButtonWheelLeft || msg.Button == tea.MouseButtonWheelRight {
			var lcmd tea.Cmd
			m.log, lcmd = m.log.Update(msg)
			cmds = append(cmds, lcmd)
		} else if msg.Button == tea.MouseButtonLeft || msg.Button == tea.MouseButtonNone {
			// Left button press / motion / release: handle for text selection.
			// Guard against right-click and middle-click: some terminal emulators
			// (e.g., iTerm2 in extended mouse mode) report non-left buttons as
			// non-wheel events; without this guard they would trigger selection.
			// logTopRow is the 0-indexed terminal row where the viewport content
			// begins: 1 (top border) + gridRows (checkbox grid) + 1 (hrule).
			// logLeftCol is 1 (inside the left border character).
			gridRows := len(m.header.header.Rows)
			logTopRow := gridRows + 2
			logLeftCol := 1

			currentMode := m.keys.handler.Mode()

			switch msg.Action {
			case tea.MouseActionPress:
				// Ignore left-press in modes where selection is not meaningful.
				if currentMode == ModeError || currentMode == ModeQuitConfirm ||
					currentMode == ModeNextConfirm || currentMode == ModeQuitting {
					break
				}
				p, ok := mouseToViewport(msg, logTopRow, logLeftCol, m.log.viewport)
				if !ok {
					break
				}
				var lcmd tea.Cmd
				m.log, lcmd = m.log.HandleMouse(p, msg.Action, msg.Shift)
				cmds = append(cmds, lcmd)
				// Transition from Normal or Done to ModeSelect on left-press.
				// ModeSelect (entered via v) stays in ModeSelect — bare click
				// re-anchors the selection cursor (handled inside HandleMouse).
				// Gate on sel.active: if resolveVisualPos returned false (e.g., the
				// click landed in empty viewport padding below the content), HandleMouse
				// leaves sel.active=false and we must not enter ModeSelect with a
				// zero-value selection.
				if (currentMode == ModeNormal || currentMode == ModeDone) && m.log.sel.active {
					m.keys.handler.mu.Lock()
					m.keys.handler.prevMode = currentMode
					m.keys.handler.mode = ModeSelect
					m.keys.handler.updateShortcutLineLocked()
					m.keys.handler.mu.Unlock()
				}

			case tea.MouseActionMotion:
				// Extend the selection during an active drag. Compute the visual
				// row without clamping to the viewport bounds so that auto-scroll
				// can fire when the pointer moves above or below the content area.
				if m.log.sel.active {
					visualRow := m.log.viewport.YOffset + (msg.Y - logTopRow)
					col := msg.X - logLeftCol
					if col < 0 {
						col = 0 // belt-and-suspenders: resolveVisualPos also clamps to [0, rowWidth]
					}
					p := pos{visualRow: visualRow, col: col}
					var lcmd tea.Cmd
					m.log, lcmd = m.log.HandleMouse(p, msg.Action, msg.Shift)
					cmds = append(cmds, lcmd)
				}

			case tea.MouseActionRelease:
				// Commit any active drag selection.
				if m.log.sel.active {
					// ok is intentionally discarded: HandleMouse.Release does not use p;
					// it only commits the selection whose cursor was already positioned
					// by preceding Motion events.
					p, _ := mouseToViewport(msg, logTopRow, logLeftCol, m.log.viewport)
					var lcmd tea.Cmd
					m.log, lcmd = m.log.HandleMouse(p, msg.Action, msg.Shift)
					cmds = append(cmds, lcmd)
					// Switch the shortcut footer to SelectCommittedShortcuts so
					// the user sees "y copy  esc cancel" immediately after release.
					// The flag is cleared on the next key or mouse event.
					if m.keys.handler.Mode() == ModeSelect {
						m.keys.handler.mu.Lock()
						m.keys.handler.selectJustReleased = true
						m.keys.handler.updateShortcutLineLocked()
						m.keys.handler.mu.Unlock()
					}
				}
			}
		}

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
		// Delegate to StatusHeader (D23). The ticker is owned by main.go —
		// no reschedule cmd is needed here.
		m.header.header.HandleHeartbeatTick()

	case tea.QuitMsg:
		return m, tea.Quit
	}

	// Update prevObservedMode after all dispatch so it reflects the mode that
	// will be in effect when control returns to the caller. The guard at the
	// top of the next Update call uses this to detect external SetMode
	// transitions that happened between two consecutive Bubble Tea updates.
	// When the mode changed, fire the status-line trigger exactly once so the
	// status-line script can reflect the new mode on its next run.
	newMode := m.keys.handler.Mode()
	if newMode != m.prevObservedMode && m.triggerFn != nil {
		m.triggerFn()
	}
	m.prevObservedMode = newMode

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
	return AppTitle + " — " + iter + m.header.header.heartbeatSuffix
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
	if s == NextConfirmPrompt {
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
