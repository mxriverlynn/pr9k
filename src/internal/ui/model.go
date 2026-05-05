package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
)

// minHeight returns the minimum terminal height required to render the full
// run-mode chrome plus at least one viewport row. Computed from the configured
// step count (gridRows is fixed for the lifetime of the process — see header.go
// NewStatusHeader). Used by View() to gate the placeholder render and by the
// mouse handler to short-circuit coordinate translation when the placeholder
// is showing.
func (m Model) minHeight() int {
	gridRows := len(m.header.header.Rows)
	chromeRows := 1 + gridRows + 1 + 1 + 1 + 1 // top + grid + 2 hrules + footer + bottom
	return chromeRows + 1
}

// tooSmall reports whether the terminal is below the chrome's minimum size.
// View() returns a placeholder string in that case rather than emit an
// over-tall frame the alt-screen would clip from the top.
func (m Model) tooSmall() bool {
	return m.width < uichrome.MinTerminalWidth || m.height < m.minHeight()
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

// StatusReader provides read-only access to the current status-line runner
// state. *statusline.Runner satisfies this interface. Pass nil (or a disabled
// runner) when no status line is configured — Enabled() returning false causes
// the footer to fall back to the shortcut-bar path.
type StatusReader interface {
	Enabled() bool
	HasOutput() bool
	LastOutput() string
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
	prevObservedMode Mode         // holds the mode observed at the end of the previous Update; used both to detect external SetMode transitions out of ModeSelect (selection clearing) and to drive the once-per-transition status-line trigger fired below.
	triggerFn        func()       // called once per mode transition; nil means no-op
	statusRunner     StatusReader // nil or disabled → footer uses shortcut-bar path
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

// WithStatusRunner installs a StatusReader on the Model. When the runner is
// enabled and has output, Model.View() switches the footer from the shortcut
// bar to the status-line display path (status text on the left and a
// right-aligned "? Help | <version>" cluster). Pass nil to disable the
// status-line footer path.
func (m Model) WithStatusRunner(r StatusReader) Model {
	m.statusRunner = r
	return m
}

// WithModeTrigger installs a mode-transition trigger on the Model. fn is
// called exactly once in Model.Update whenever the mode changes from one
// Update call to the next. When the status line is disabled, pass nil (safe).
// Note: tea.QuitMsg short-circuits before the trigger check; any mode
// transition that emits tea.Quit is reflected by the Update call that preceded
// QuitMsg, not by the QuitMsg handler itself. fn must be non-blocking —
// Runner.Trigger satisfies this guarantee via its buffered drop-on-full channel.
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

	case StatusLineUpdatedMsg:
		// Pure re-render trigger: Bubble Tea re-renders automatically after
		// Update returns. The fresh LastOutput() is read in View().

	case tea.MouseMsg:
		// While the placeholder is showing, mouse coordinate translation does
		// not match the rendered output, so ignore mouse events entirely.
		if m.tooSmall() {
			break
		}
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
				// Ignore left-press in modes where selection is not meaningful,
				// and in ModeHelp where the modal is a true overlay (wheel events
				// still scroll the underlying viewport; only non-wheel presses are
				// suppressed so the modal cannot be accidentally dismissed by a click).
				if currentMode == ModeError || currentMode == ModeQuitConfirm ||
					currentMode == ModeNextConfirm || currentMode == ModeQuitting ||
					currentMode == ModeHelp {
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
		// tea.ClearScreen forces a full alt-screen erase before the next render.
		// Bubble Tea's standard_renderer.repaint() on WindowSizeMsg only invalidates
		// its in-memory line cache — it does NOT erase the alt-screen. Without this,
		// any previous over-tall render that scrolled the alt-screen leaves stale
		// rows above the new render's cursor home, producing visibly shuffled rows
		// after a zoom event.
		cmds = append(cmds, lcmd, tea.ClearScreen)

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
	// Min-size guard: if the terminal is smaller than the chrome's intrinsic
	// minimum, render a single-line placeholder rather than an over-tall frame
	// that the alt-screen would clip from the top. Mirrors the workflow-builder
	// guard at internal/workflowedit/render_frame.go:30.
	if m.tooSmall() {
		return fmt.Sprintf("Terminal too small — resize to at least %d×%d",
			uichrome.MinTerminalWidth, m.minHeight())
	}

	title := m.titleString()
	innerWidth := m.width - 2
	if innerWidth < 0 {
		innerWidth = 0
	}

	wrapLine := func(content string) string {
		return uichrome.WrapLine(content, innerWidth)
	}

	hruleLine := uichrome.HRuleLine(innerWidth)
	bottomBorder := uichrome.BottomBorder(innerWidth)

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

	// Shortcut footer — two rendering paths:
	// (1) ModeNormal with a status runner configured, enabled, and populated:
	//     show the status-line text on the left and a right-aligned cluster
	//     of "? Help | <version>" so the help hint and version label stay
	//     pinned to the right edge regardless of the status text width.
	// (2) All other modes (including ModeHelp itself): show the mode's shortcut
	//     string. updateShortcutLineLocked already set HelpModeShortcuts for
	//     ModeHelp, so no dedicated path is needed here.
	footerMode := m.keys.handler.Mode()
	versionWidth := lipgloss.Width(m.versionLabel)
	whiteVer := lipgloss.NewStyle().Foreground(White).Render(m.versionLabel)

	var footer string
	if footerMode == ModeNormal &&
		m.statusRunner != nil && m.statusRunner.Enabled() && m.statusRunner.HasOutput() {
		statusText := m.statusRunner.LastOutput()
		helpHint := colorShortcutLine("? Help")
		helpWidth := lipgloss.Width(helpHint)
		sep := lipgloss.NewStyle().Foreground(LightGray).Render(" | ")
		sepWidth := lipgloss.Width(sep)
		rightCluster := helpHint + sep + whiteVer
		rightWidth := helpWidth + sepWidth + versionWidth
		// Reserve: 2-space gap before the right-aligned cluster.
		statusBudget := innerWidth - rightWidth - 2
		if statusBudget < 0 {
			statusBudget = 0
		}
		statusTrunc := lipgloss.NewStyle().MaxWidth(statusBudget).Render(statusText)
		spacerW := innerWidth - lipgloss.Width(statusTrunc) - rightWidth
		if spacerW < 0 {
			spacerW = 0
		}
		footer = statusTrunc + strings.Repeat(" ", spacerW) + rightCluster
	} else {
		shortcut := m.keys.handler.ShortcutLine()
		footerWidth := innerWidth
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
		footer = shortcutTrunc + strings.Repeat(" ", spacerWidth) + whiteVer
	}
	sb.WriteString(wrapLine(footer))
	sb.WriteString("\n")

	// Bottom border.
	sb.WriteString(bottomBorder)

	frame := sb.String()

	// When in ModeHelp, compute the centered help modal and splice it over the
	// frame using the overlay helper. The modal is a complete ANSI-styled string;
	// the overlay splice preserves base-frame ANSI outside the modal region.
	if footerMode == ModeHelp {
		modal := m.renderHelpModal()
		modalLines := strings.Split(modal, "\n")
		modalH := len(modalLines)
		modalW := lipgloss.Width(modalLines[0])
		// If the modal is taller than the frame, pin the esc hint (second-to-last
		// line) and bottom border (last line) to the last two visible rows so the
		// user always sees the dismissal cue regardless of terminal height.
		if modalH > m.height && m.height >= 2 {
			bottomLine := modalLines[modalH-1]
			footerLine := modalLines[modalH-2]
			modalLines = modalLines[:m.height]
			modalLines[m.height-1] = bottomLine
			modalLines[m.height-2] = footerLine
			modal = strings.Join(modalLines, "\n")
			modalH = m.height
		}
		top := (m.height - modalH) / 2
		left := (m.width - modalW) / 2
		if top < 0 {
			top = 0
		}
		if left < 0 {
			left = 0
		}
		return uichrome.Overlay(frame, modal, top, left)
	}

	return frame
}

// renderHelpModal builds the centered help modal string. The modal is sized
// to min(terminalWidth-4, 70) columns and contains all four mode-section
// tables from the HelpModal* constants, with a title border and a right-
// aligned "esc  close" footer row.
func (m Model) renderHelpModal() string {
	modalWidth := min(m.width-4, 70)
	if modalWidth < 29 {
		modalWidth = 29
	}
	innerW := modalWidth - 2 // subtract the two border characters

	gray := lipgloss.NewStyle().Foreground(LightGray)
	white := lipgloss.NewStyle().Foreground(White)

	wrapLine := func(content string) string {
		return uichrome.WrapLine(content, innerW)
	}

	// Top border: "╭─ Help: Keyboard Shortcuts ──...──╮"
	titleText := " Help: Keyboard Shortcuts "
	leadDash := "─"
	titleWidth := 1 + lipgloss.Width(titleText) // leadDash (1) + titleText
	fillRight := innerW - titleWidth
	if fillRight < 0 {
		fillRight = 0
	}
	topBorder := gray.Render("╭"+leadDash) +
		white.Render(titleText) +
		gray.Render(strings.Repeat("─", fillRight)+"╮")

	var rows []string
	rows = append(rows, wrapLine("")) // blank line after top border

	// addSection renders a section label followed by the multi-line HelpModal*
	// constant. Section labels are white; content lines are colored via
	// colorShortcutLine (white keys, gray descriptions). Content lines from the
	// constants carry a 2-space indent; we prepend another 2 spaces so the
	// label/content relationship is visually clear (label at col 2, content at col 4).
	addSection := func(label, content string) {
		rows = append(rows, wrapLine("  "+white.Render(label)))
		for _, l := range strings.Split(content, "\n") {
			rows = append(rows, wrapLine(colorShortcutLine("  "+l)))
		}
	}

	addSection("Normal", HelpModalNormal)
	rows = append(rows, wrapLine(""))
	addSection("Select", HelpModalSelect)
	rows = append(rows, wrapLine(""))
	addSection("Error", HelpModalError)
	rows = append(rows, wrapLine(""))
	addSection("Done", HelpModalDone)
	rows = append(rows, wrapLine("")) // blank before footer row

	// Footer: "esc  close" right-aligned with 2 trailing spaces.
	footerText := colorShortcutLine(HelpModeShortcuts)
	footerW := lipgloss.Width(footerText)
	footerPad := innerW - footerW - 2
	if footerPad < 0 {
		footerPad = 0
	}
	rows = append(rows, wrapLine(strings.Repeat(" ", footerPad)+footerText+"  "))

	var sb strings.Builder
	sb.WriteString(topBorder + "\n")
	for _, row := range rows {
		sb.WriteString(row + "\n")
	}
	sb.WriteString(uichrome.BottomBorder(innerW))
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

// colorShortcutLine applies the footer shortcut bar's two-tone palette. When
// the footer shows a mode-specific prompt string (quit-confirm, next-confirm,
// quitting line), the whole string renders white — with one exception: within
// the quit-confirm prompt, the embedded AppTitle substring renders green to
// match the top-border brand color. For all other strings the generic
// two-tone delegate uichrome.ColorShortcutLine is used.
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
	return uichrome.ColorShortcutLine(s)
}

// colorTitle delegates to uichrome.ColorTitle: the app name (before the first
// " — " separator) renders green; the iteration detail renders white.
func colorTitle(title string) string {
	return uichrome.ColorTitle(title)
}

// renderTopBorder delegates to uichrome.RenderTopBorder, passing the model's
// current terminal width.
func (m Model) renderTopBorder(title string) string {
	return uichrome.RenderTopBorder(title, m.width)
}
