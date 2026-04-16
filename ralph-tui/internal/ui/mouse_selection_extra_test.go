package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Category 1: resolveVisualPos edge cases ───────────────────────────────────

// C1-01: visualRow negative → returns (p, false)
func TestResolveVisualPos_NegativeRow_ReturnsFalse(t *testing.T) {
	m := newLogModel(80, 10)
	m.lines = append(m.lines, "hello world")
	m.rewrap(80)

	p := pos{visualRow: -1, col: 0}
	_, ok := m.resolveVisualPos(p)
	if ok {
		t.Error("expected resolveVisualPos to return false for negative visualRow")
	}
}

// C1-02: visualRow past end of visualLines → returns (p, false)
func TestResolveVisualPos_RowPastEnd_ReturnsFalse(t *testing.T) {
	m := newLogModel(80, 10)
	m.lines = append(m.lines, "hello world")
	m.rewrap(80)

	p := pos{visualRow: len(m.visualLines), col: 0}
	_, ok := m.resolveVisualPos(p)
	if ok {
		t.Error("expected resolveVisualPos to return false when visualRow >= len(visualLines)")
	}
}

// C1-03: col past row width → clamped to lipgloss.Width(vl.text)
func TestResolveVisualPos_ColPastRowWidth_ClampsToEnd(t *testing.T) {
	m := newLogModel(80, 10)
	m.lines = append(m.lines, "hello")
	m.rewrap(80)

	rowWidth := lipgloss.Width(m.visualLines[0].text) // 5 for "hello"
	p := pos{visualRow: 0, col: rowWidth + 100}
	resolved, ok := m.resolveVisualPos(p)
	if !ok {
		t.Fatal("expected resolveVisualPos to succeed for valid row")
	}
	if resolved.col != rowWidth {
		t.Errorf("expected col clamped to rowWidth=%d, got col=%d", rowWidth, resolved.col)
	}
}

// C1-04: col negative → clamped to 0
func TestResolveVisualPos_NegativeCol_ClampsToZero(t *testing.T) {
	m := newLogModel(80, 10)
	m.lines = append(m.lines, "hello world")
	m.rewrap(80)

	p := pos{visualRow: 0, col: -5}
	resolved, ok := m.resolveVisualPos(p)
	if !ok {
		t.Fatal("expected resolveVisualPos to succeed for valid row")
	}
	if resolved.col != 0 {
		t.Errorf("expected col clamped to 0, got col=%d", resolved.col)
	}
}

// ── Category 2: HandleMouse unit tests ───────────────────────────────────────

// C2-01: Press with empty visualLines → no-op (resolveVisualPos returns false)
func TestHandleMouse_PressWithEmptyVisualLines_NoOp(t *testing.T) {
	m := newLogModel(80, 10)
	// No lines added; visualLines is empty.

	p := pos{visualRow: 0, col: 0}
	mAfter, _ := m.HandleMouse(p, tea.MouseActionPress, false)

	if mAfter.sel.active || mAfter.sel.committed {
		t.Error("expected no selection change on press with empty visualLines")
	}
}

// C2-02: Motion without active selection → no-op, no auto-scroll
func TestHandleMouse_MotionWithoutActiveSelection_NoOp(t *testing.T) {
	m := newLogModel(80, 10)
	for i := 0; i < 20; i++ {
		m.lines = append(m.lines, strings.Repeat("x", 40))
	}
	m.rewrap(80)
	m.viewport.SetYOffset(5)
	initialOffset := m.viewport.YOffset

	p := pos{visualRow: 0, col: 0}
	mAfter, _ := m.HandleMouse(p, tea.MouseActionMotion, false)

	if mAfter.sel.active || mAfter.sel.committed {
		t.Error("expected no selection change on motion without active selection")
	}
	if mAfter.viewport.YOffset != initialOffset {
		t.Errorf("expected no auto-scroll without active selection: initial=%d, now=%d",
			initialOffset, mAfter.viewport.YOffset)
	}
}

// C2-03: Release without active selection → no-op
func TestHandleMouse_ReleaseWithoutActiveSelection_NoOp(t *testing.T) {
	m := newLogModel(80, 10)
	m.lines = append(m.lines, strings.Repeat("x", 40))
	m.rewrap(80)

	p := pos{visualRow: 0, col: 0}
	mAfter, _ := m.HandleMouse(p, tea.MouseActionRelease, false)

	if mAfter.sel.active || mAfter.sel.committed {
		t.Error("expected no selection change on release without active selection")
	}
}

// C2-04: Shift-click with no committed selection → bare press (new anchor)
func TestHandleMouse_ShiftPressNoCommittedSelection_ActsAsBarePress(t *testing.T) {
	m := newLogModel(80, 10)
	for i := 0; i < 5; i++ {
		m.lines = append(m.lines, strings.Repeat("x", 40))
	}
	m.rewrap(80)

	// No prior selection; shift-click should behave as bare press.
	p := pos{visualRow: 0, col: 5}
	mAfter, _ := m.HandleMouse(p, tea.MouseActionPress, true /* shift */)

	if !mAfter.sel.active {
		t.Error("expected shift-press with no committed selection to start new active selection")
	}
	if mAfter.sel.committed {
		t.Error("expected committed=false after shift-press with no prior selection")
	}
}

// C2-05: Motion with negative visualRow → cursor clamped to first visual line
func TestHandleMouse_MotionNegativeVisualRow_ClampsToFirstLine(t *testing.T) {
	m := newLogModel(80, 10)
	for i := 0; i < 5; i++ {
		m.lines = append(m.lines, strings.Repeat("x", 40))
	}
	m.rewrap(80)
	m.viewport.SetYOffset(3)

	// Start a drag at row 3.
	startP := pos{visualRow: 3, col: 0}
	m, _ = m.HandleMouse(startP, tea.MouseActionPress, false)

	// Motion with a negative visual row (pointer above all content).
	p := pos{visualRow: -2, col: 0}
	mAfter, _ := m.HandleMouse(p, tea.MouseActionMotion, false)

	if mAfter.sel.cursor.visualRow != 0 {
		t.Errorf("expected cursor clamped to row 0 on negative visualRow, got row=%d",
			mAfter.sel.cursor.visualRow)
	}
}

// ── Category 3: model.go mouse routing edge cases ────────────────────────────

// C3-01: Left-press above viewport bounds (Y < logTopRow) → no-op
func TestMouse_PressAboveViewport_NoOp(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	populateLog(t, &m, 20)

	topRow := logTopRowForModel(m)
	// Click one row above the viewport content area.
	next, _ := m.Update(pressMsg(1, topRow-1))
	m = next.(Model)

	if m.log.sel.active || m.log.sel.committed {
		t.Error("expected no selection after click above viewport")
	}
	if m.keys.handler.Mode() != ModeNormal {
		t.Errorf("expected mode unchanged (ModeNormal), got %v", m.keys.handler.Mode())
	}
}

// C3-02: Left-press below viewport bounds (Y > logTopRow + height - 1) → no-op
func TestMouse_PressBelowViewport_NoOp(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	populateLog(t, &m, 20)

	topRow := logTopRowForModel(m)
	belowRow := topRow + m.log.viewport.Height // one past last valid row
	next, _ := m.Update(pressMsg(1, belowRow))
	m = next.(Model)

	if m.log.sel.active || m.log.sel.committed {
		t.Error("expected no selection after click below viewport")
	}
	if m.keys.handler.Mode() != ModeNormal {
		t.Errorf("expected mode unchanged (ModeNormal), got %v", m.keys.handler.Mode())
	}
}

// C3-03: Motion event without prior press (sel.active=false) → ignored
func TestMouse_MotionWithoutPriorPress_Ignored(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	populateLog(t, &m, 20)

	topRow := logTopRowForModel(m)
	next, _ := m.Update(motionMsg(5, topRow+2))
	m = next.(Model)

	if m.log.sel.active || m.log.sel.committed {
		t.Error("expected no selection after motion without prior press")
	}
}

// C3-04: Release event without prior press (sel.active=false) → ignored
func TestMouse_ReleaseWithoutPriorPress_Ignored(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	populateLog(t, &m, 20)

	topRow := logTopRowForModel(m)
	next, _ := m.Update(releaseMsg(5, topRow+2))
	m = next.(Model)

	if m.log.sel.active || m.log.sel.committed {
		t.Error("expected no selection after release without prior press")
	}
	if m.keys.handler.selectJustReleased {
		t.Error("expected selectJustReleased=false after spurious release")
	}
}

// ── Category 4: selectJustReleased flag lifecycle ────────────────────────────

// C4-01: Wheel event after release clears selectJustReleased and restores SelectShortcuts
func TestSelectJustReleased_ClearedByWheelEvent(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = strings.Repeat("x", 30)
	}
	next, _ := m.Update(LogLinesMsg{Lines: lines})
	m = next.(Model)

	topRow := logTopRowForModel(m)

	// Press, drag, release to set selectJustReleased.
	next, _ = m.Update(pressMsg(1, topRow))
	m = next.(Model)
	next, _ = m.Update(motionMsg(10, topRow+1))
	m = next.(Model)
	next, _ = m.Update(releaseMsg(10, topRow+1))
	m = next.(Model)

	if !m.keys.handler.selectJustReleased {
		t.Fatal("setup: expected selectJustReleased=true after release")
	}

	// A wheel event must clear selectJustReleased.
	next, _ = m.Update(wheelUpMsg(10, topRow+3))
	m = next.(Model)

	if m.keys.handler.selectJustReleased {
		t.Error("expected selectJustReleased=false after wheel event")
	}
	if m.keys.handler.ShortcutLine() != SelectShortcuts {
		t.Errorf("expected SelectShortcuts after wheel event, got %q", m.keys.handler.ShortcutLine())
	}
}

// C4-02: Second mouse press after committed selection clears selectJustReleased
func TestSelectJustReleased_ClearedBySecondPress(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = strings.Repeat("x", 30)
	}
	next, _ := m.Update(LogLinesMsg{Lines: lines})
	m = next.(Model)

	topRow := logTopRowForModel(m)

	// First drag → release → selectJustReleased set.
	next, _ = m.Update(pressMsg(1, topRow))
	m = next.(Model)
	next, _ = m.Update(motionMsg(10, topRow+1))
	m = next.(Model)
	next, _ = m.Update(releaseMsg(10, topRow+1))
	m = next.(Model)

	if !m.keys.handler.selectJustReleased {
		t.Fatal("setup: expected selectJustReleased=true after first release")
	}

	// Second press in a different cell must clear the flag.
	next, _ = m.Update(pressMsg(1, topRow+3))
	m = next.(Model)

	if m.keys.handler.selectJustReleased {
		t.Error("expected selectJustReleased=false after second press")
	}
}

// C4-03: Entering ModeSelect via keyboard v does NOT set selectJustReleased
func TestSelectJustReleased_NotSetByKeyboardV(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	populateLog(t, &m, 10)

	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)

	if m.keys.handler.Mode() != ModeSelect {
		t.Fatal("setup: expected ModeSelect after v")
	}
	if m.keys.handler.selectJustReleased {
		t.Error("expected selectJustReleased=false when entering ModeSelect via keyboard v")
	}
	if m.keys.handler.ShortcutLine() != SelectShortcuts {
		t.Errorf("expected SelectShortcuts after v, got %q", m.keys.handler.ShortcutLine())
	}
}

// ── Category 5: Drag auto-scroll clamping and boundary ───────────────────────

// C5-01: Multiple motion events above viewport each scroll one line
func TestDragAutoScroll_MultipleAboveViewport_ScrollsOneLineEach(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	populateLog(t, &m, 30)

	topRow := logTopRowForModel(m)
	m.log.viewport.SetYOffset(10)

	next, _ := m.Update(pressMsg(1, topRow+2))
	m = next.(Model)
	offset0 := m.log.viewport.YOffset

	// First motion above the viewport content area.
	next, _ = m.Update(motionMsg(1, topRow-1))
	m = next.(Model)
	offset1 := m.log.viewport.YOffset

	// Second motion above the viewport content area.
	next, _ = m.Update(motionMsg(1, topRow-1))
	m = next.(Model)
	offset2 := m.log.viewport.YOffset

	if offset1 >= offset0 {
		t.Errorf("first above-viewport motion should scroll up: before=%d, after=%d", offset0, offset1)
	}
	if offset2 >= offset1 {
		t.Errorf("second above-viewport motion should scroll up again: before=%d, after=%d", offset1, offset2)
	}
	if offset0-offset2 != 2 {
		t.Errorf("expected 2 lines scrolled after 2 events, got %d", offset0-offset2)
	}
}

// C5-02: Auto-scroll at YOffset=0 does not go negative
func TestDragAutoScroll_AtTopBoundary_StaysAtZero(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	populateLog(t, &m, 30)

	topRow := logTopRowForModel(m)
	m.log.viewport.SetYOffset(0)

	// Press inside viewport then immediately move above it.
	next, _ := m.Update(pressMsg(1, topRow))
	m = next.(Model)
	next, _ = m.Update(motionMsg(1, topRow-1))
	m = next.(Model)

	if m.log.viewport.YOffset < 0 {
		t.Errorf("YOffset must not go negative, got %d", m.log.viewport.YOffset)
	}
	if m.log.viewport.YOffset != 0 {
		t.Errorf("expected YOffset=0 at top boundary after auto-scroll attempt, got %d",
			m.log.viewport.YOffset)
	}
}

// C5-03: Auto-scroll at bottom boundary does not exceed max YOffset
func TestDragAutoScroll_AtBottomBoundary_DoesNotExceedMax(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	populateLog(t, &m, 30)

	// Scroll to the bottom and record the maximum offset.
	m.log.viewport.GotoBottom()
	maxOffset := m.log.viewport.YOffset

	topRow := logTopRowForModel(m)

	// Press inside viewport (does not change YOffset).
	next, _ := m.Update(pressMsg(1, topRow+2))
	m = next.(Model)

	// Motion below viewport: auto-scroll would try to increase YOffset beyond max.
	vpBottom := topRow + m.log.viewport.Height
	next, _ = m.Update(motionMsg(1, vpBottom+1))
	m = next.(Model)

	if m.log.viewport.YOffset > maxOffset {
		t.Errorf("YOffset must not exceed max=%d after bottom-boundary auto-scroll, got %d",
			maxOffset, m.log.viewport.YOffset)
	}
}

// ── Category 6: Shift-click edge cases ───────────────────────────────────────

// C6-01: Shift-click on the same cell as the cursor → no change, still committed
func TestShiftClick_OnSameCellAsCursor_NoChange(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = strings.Repeat("c", 40)
	}
	next, _ := m.Update(LogLinesMsg{Lines: lines})
	m = next.(Model)

	topRow := logTopRowForModel(m)

	// Establish a committed selection.
	next, _ = m.Update(pressMsg(1, topRow)) // col=0
	m = next.(Model)
	next, _ = m.Update(motionMsg(11, topRow)) // col=10
	m = next.(Model)
	next, _ = m.Update(releaseMsg(11, topRow)) // commit
	m = next.(Model)

	if !m.log.sel.committed {
		t.Fatal("setup: expected committed selection")
	}

	cursorRow := m.log.sel.cursor.visualRow
	cursorCol := m.log.sel.cursor.col

	// Compute the terminal coordinates that map back to the same visual pos.
	// mouseToViewport: visualRow = YOffset + (Y - topRow); col = X - logLeftCol(1)
	// So X = cursorCol + 1, Y = topRow + (cursorRow - YOffset)
	termX := cursorCol + 1
	termY := topRow + (cursorRow - m.log.viewport.YOffset)

	// Shift-click at the cursor's current position.
	next, _ = m.Update(pressMsgShift(termX, termY))
	m = next.(Model)

	if m.log.sel.active {
		t.Error("expected active=false after shift-click on same cell")
	}
	if !m.log.sel.committed {
		t.Error("expected committed=true after shift-click on same cell")
	}
}

// C6-02: Shift-click before anchor → cursor moves backward, normalized handles it
func TestShiftClick_BeforeAnchor_CursorMovesBackward(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = strings.Repeat("d", 40)
	}
	next, _ := m.Update(LogLinesMsg{Lines: lines})
	m = next.(Model)

	topRow := logTopRowForModel(m)

	// Press at col 10, drag to col 20, release.
	next, _ = m.Update(pressMsg(11, topRow)) // col=10
	m = next.(Model)
	next, _ = m.Update(motionMsg(21, topRow)) // col=20
	m = next.(Model)
	next, _ = m.Update(releaseMsg(21, topRow)) // commit
	m = next.(Model)

	if !m.log.sel.committed {
		t.Fatal("setup: expected committed selection")
	}

	origAnchorCol := m.log.sel.anchor.col

	// Shift-click at col 2 (before the anchor at col ~10).
	next, _ = m.Update(pressMsgShift(3, topRow)) // col=2
	m = next.(Model)

	// Cursor should now be before the anchor.
	if m.log.sel.cursor.col >= origAnchorCol {
		t.Errorf("shift-click before anchor should move cursor before anchor: anchorCol=%d, cursorCol=%d",
			origAnchorCol, m.log.sel.cursor.col)
	}
	// Selection should remain committed.
	if !m.log.sel.committed {
		t.Error("shift-click should keep selection committed")
	}
	if m.log.sel.active {
		t.Error("shift-click should not set active=true")
	}
}

// C6-03: Shift-click while selection is active (not yet committed) → bare press
func TestShiftClick_WhileActive_ActsAsBarePress(t *testing.T) {
	m, _ := mouseTestModel(t, ModeNormal)
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = strings.Repeat("e", 40)
	}
	next, _ := m.Update(LogLinesMsg{Lines: lines})
	m = next.(Model)

	topRow := logTopRowForModel(m)

	// Press only — selection is active but not committed.
	next, _ = m.Update(pressMsg(1, topRow))
	m = next.(Model)

	if !m.log.sel.active {
		t.Fatal("setup: expected active selection after press")
	}
	if m.log.sel.committed {
		t.Fatal("setup: expected NOT committed after press only")
	}

	// Shift-click: since committed=false, the shift-extend condition is false
	// and the event is treated as a bare press → new active selection.
	next, _ = m.Update(pressMsgShift(10, topRow+1))
	m = next.(Model)

	if !m.log.sel.active {
		t.Error("expected active=true after shift-click during active (uncommitted) drag")
	}
	if m.log.sel.committed {
		t.Error("expected committed=false: active selection cannot be shift-extended")
	}
}

// ── Category 7: SelectCommittedShortcuts and updateShortcutLineLocked ─────────

// C7-01: SelectCommittedShortcuts constant contains the documented bindings
func TestSelectCommittedShortcuts_ContainsExpectedBindings(t *testing.T) {
	for _, want := range []string{"y", "esc", "copy", "cancel"} {
		if !strings.Contains(SelectCommittedShortcuts, want) {
			t.Errorf("SelectCommittedShortcuts should contain %q, got %q", want, SelectCommittedShortcuts)
		}
	}
}

// C7-02: updateShortcutLineLocked with selectJustReleased=false in ModeSelect → SelectShortcuts
func TestUpdateShortcutLineLocked_ModeSelect_NotReleased_ShowsSelectShortcuts(t *testing.T) {
	_, kh := mouseTestModel(t, ModeSelect)

	kh.mu.Lock()
	kh.selectJustReleased = false
	kh.updateShortcutLineLocked()
	kh.mu.Unlock()

	shortcut := kh.ShortcutLine()
	if shortcut != SelectShortcuts {
		t.Errorf("expected SelectShortcuts when selectJustReleased=false in ModeSelect, got %q", shortcut)
	}
}
