package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// sendKey dispatches a single key to the model and returns the updated model.
func sendKey(t *testing.T, m Model, key tea.KeyMsg) Model {
	t.Helper()
	next, _ := m.Update(key)
	return next.(Model)
}

// enterSelectMode presses v to enter ModeSelect and returns the updated model.
func enterSelectMode(t *testing.T, m Model) Model {
	t.Helper()
	m = sendKey(t, m, keyMsg("v"))
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatalf("expected ModeSelect after v, got %v", m.keys.handler.Mode())
	}
	return m
}

// newSelectModelWithLines builds a test model at 80×24 / viewport 76×10,
// populated with n lines and scrolled to the top, then entered into ModeSelect.
// Each line i contains strings.Repeat("a", (i%40)+1) so lines have varying
// widths that expose virtual-column edge cases.
func newSelectModelWithLines(t *testing.T, n int) Model {
	t.Helper()
	m, _ := newSelectTestModel(t, ModeNormal)
	populateLog(t, &m, n)
	m.log.viewport.GotoTop()
	return enterSelectMode(t, m)
}

// --- TP-105-01: h/j/k/l move the cursor by one cell; anchor stays fixed ---

func TestKeys_HJKL_MovesSelectionCursor(t *testing.T) {
	m := newSelectModelWithLines(t, 15)
	anchor := m.log.sel.anchor

	// l → cursor moves right by 1
	m = sendKey(t, m, keyMsg("l"))
	if m.log.sel.cursor.col != 1 {
		t.Errorf("l: expected cursor.col=1, got %d", m.log.sel.cursor.col)
	}
	if m.log.sel.anchor != anchor {
		t.Error("l: anchor must not move")
	}

	// h → cursor moves left by 1 (back to 0)
	m = sendKey(t, m, keyMsg("h"))
	if m.log.sel.cursor.col != 0 {
		t.Errorf("h: expected cursor.col=0, got %d", m.log.sel.cursor.col)
	}
	if m.log.sel.anchor != anchor {
		t.Error("h: anchor must not move")
	}

	// j → cursor moves down by 1 row
	rowBefore := m.log.sel.cursor.visualRow
	m = sendKey(t, m, keyMsg("j"))
	if m.log.sel.cursor.visualRow != rowBefore+1 {
		t.Errorf("j: expected cursor.visualRow=%d, got %d", rowBefore+1, m.log.sel.cursor.visualRow)
	}
	if m.log.sel.anchor != anchor {
		t.Error("j: anchor must not move")
	}

	// k → cursor moves back up by 1 row
	rowBefore = m.log.sel.cursor.visualRow
	m = sendKey(t, m, keyMsg("k"))
	if m.log.sel.cursor.visualRow != rowBefore-1 {
		t.Errorf("k: expected cursor.visualRow=%d, got %d", rowBefore-1, m.log.sel.cursor.visualRow)
	}
	if m.log.sel.anchor != anchor {
		t.Error("k: anchor must not move")
	}
}

// --- TP-105-02: arrow keys mirror hjkl ---

func TestKeys_Arrows_MovesSelectionCursor(t *testing.T) {
	m := newSelectModelWithLines(t, 15)
	anchor := m.log.sel.anchor

	right := tea.KeyMsg{Type: tea.KeyRight}
	left := tea.KeyMsg{Type: tea.KeyLeft}
	down := tea.KeyMsg{Type: tea.KeyDown}
	up := tea.KeyMsg{Type: tea.KeyUp}

	// right → col +1
	m = sendKey(t, m, right)
	if m.log.sel.cursor.col != 1 {
		t.Errorf("right: expected col=1, got %d", m.log.sel.cursor.col)
	}

	// left → col 0
	m = sendKey(t, m, left)
	if m.log.sel.cursor.col != 0 {
		t.Errorf("left: expected col=0, got %d", m.log.sel.cursor.col)
	}

	rowBefore := m.log.sel.cursor.visualRow
	// down → row +1
	m = sendKey(t, m, down)
	if m.log.sel.cursor.visualRow != rowBefore+1 {
		t.Errorf("down: expected row=%d, got %d", rowBefore+1, m.log.sel.cursor.visualRow)
	}

	// up → row -1
	m = sendKey(t, m, up)
	if m.log.sel.cursor.visualRow != rowBefore {
		t.Errorf("up: expected row=%d, got %d", rowBefore, m.log.sel.cursor.visualRow)
	}

	if m.log.sel.anchor != anchor {
		t.Error("arrow keys must not move anchor")
	}
}

// --- TP-105-03: $ jumps cursor to last column of current visual row ---

func TestKeys_DollarSign_JumpsToLineEnd_InSelectMode(t *testing.T) {
	m := newSelectModelWithLines(t, 5)
	anchor := m.log.sel.anchor
	row := m.log.sel.cursor.visualRow
	wantLastCol := len(m.log.visualLines[row].text) // ASCII: byte len == display col count

	m = sendKey(t, m, keyMsg("$"))

	if m.log.sel.cursor.col != wantLastCol {
		t.Errorf("$: expected cursor.col=%d (last col), got %d", wantLastCol, m.log.sel.cursor.col)
	}
	if m.log.sel.cursor.visualRow != row {
		t.Errorf("$: cursor.visualRow changed from %d to %d", row, m.log.sel.cursor.visualRow)
	}
	if m.log.sel.anchor != anchor {
		t.Error("$: anchor must not move")
	}
}

// --- TP-105-04: 0 jumps cursor to column 0 ---

func TestKeys_Zero_JumpsToLineStart_InSelectMode(t *testing.T) {
	m := newSelectModelWithLines(t, 5)
	// Move cursor right first so 0 has something to do.
	m = sendKey(t, m, keyMsg("l"))
	m = sendKey(t, m, keyMsg("l"))
	if m.log.sel.cursor.col != 2 {
		t.Fatalf("precondition: expected col=2 after two l presses, got %d", m.log.sel.cursor.col)
	}
	anchor := m.log.sel.anchor
	row := m.log.sel.cursor.visualRow

	m = sendKey(t, m, keyMsg("0"))

	if m.log.sel.cursor.col != 0 {
		t.Errorf("0: expected cursor.col=0, got %d", m.log.sel.cursor.col)
	}
	if m.log.sel.cursor.visualRow != row {
		t.Errorf("0: cursor.visualRow changed from %d to %d", row, m.log.sel.cursor.visualRow)
	}
	if m.log.sel.anchor != anchor {
		t.Error("0: anchor must not move")
	}
}

// --- TP-105-05a: Home jumps cursor to column 0 ---

func TestKeys_Home_JumpsToLineStart_InSelectMode(t *testing.T) {
	m := newSelectModelWithLines(t, 5)
	m = sendKey(t, m, keyMsg("l"))
	m = sendKey(t, m, keyMsg("l"))
	row := m.log.sel.cursor.visualRow

	homeMsg := tea.KeyMsg{Type: tea.KeyHome}
	m = sendKey(t, m, homeMsg)

	if m.log.sel.cursor.col != 0 {
		t.Errorf("Home: expected cursor.col=0, got %d", m.log.sel.cursor.col)
	}
	if m.log.sel.cursor.visualRow != row {
		t.Errorf("Home: cursor.visualRow changed from %d to %d", row, m.log.sel.cursor.visualRow)
	}
}

// --- TP-105-05b: End jumps cursor to last column ---

func TestKeys_End_JumpsToLineEnd_InSelectMode(t *testing.T) {
	m := newSelectModelWithLines(t, 5)
	row := m.log.sel.cursor.visualRow
	wantLastCol := len(m.log.visualLines[row].text)

	endMsg := tea.KeyMsg{Type: tea.KeyEnd}
	m = sendKey(t, m, endMsg)

	if m.log.sel.cursor.col != wantLastCol {
		t.Errorf("End: expected cursor.col=%d, got %d", wantLastCol, m.log.sel.cursor.col)
	}
	if m.log.sel.cursor.visualRow != row {
		t.Errorf("End: cursor.visualRow changed from %d to %d", row, m.log.sel.cursor.visualRow)
	}
}

// --- TP-105-06: shift+↓ extends selection by one row, preserving virtual column ---

func TestKeys_ShiftDown_ExtendsByLine_InSelectMode(t *testing.T) {
	// Use 20 lines so the cursor has room to move down past the 10-row viewport.
	m := newSelectModelWithLines(t, 20)
	// Move cursor right to col 3 to set a non-zero virtual column.
	m = sendKey(t, m, keyMsg("l"))
	m = sendKey(t, m, keyMsg("l"))
	m = sendKey(t, m, keyMsg("l"))
	if m.log.sel.cursor.col != 3 {
		t.Fatalf("precondition: expected col=3, got %d", m.log.sel.cursor.col)
	}
	anchor := m.log.sel.anchor
	rowBefore := m.log.sel.cursor.visualRow

	shiftDown := tea.KeyMsg{Type: tea.KeyShiftDown}
	m = sendKey(t, m, shiftDown)

	if m.log.sel.cursor.visualRow != rowBefore+1 {
		t.Errorf("shift+down: expected row=%d, got %d", rowBefore+1, m.log.sel.cursor.visualRow)
	}
	if m.log.sel.anchor != anchor {
		t.Error("shift+down: anchor must not move")
	}
	// Virtual column is preserved if the new row is at least as wide.
	newRow := m.log.sel.cursor.visualRow
	newRowWidth := len(m.log.visualLines[newRow].text)
	if newRowWidth >= 3 && m.log.sel.cursor.col != 3 {
		t.Errorf("shift+down: virtual col not preserved; expected col=3, got %d", m.log.sel.cursor.col)
	}
}

// --- TP-105-07: J and K extend selection by line (vim-style aliases) ---

func TestKeys_CapitalJ_K_ExtendsByLine_InSelectMode(t *testing.T) {
	// Use 20 lines so the cursor has room to move down past the 10-row viewport.
	m := newSelectModelWithLines(t, 20)
	anchor := m.log.sel.anchor
	rowBefore := m.log.sel.cursor.visualRow

	// J → down 1 row
	m = sendKey(t, m, keyMsg("J"))
	if m.log.sel.cursor.visualRow != rowBefore+1 {
		t.Errorf("J: expected row=%d, got %d", rowBefore+1, m.log.sel.cursor.visualRow)
	}

	// K → up 1 row (back to start)
	m = sendKey(t, m, keyMsg("K"))
	if m.log.sel.cursor.visualRow != rowBefore {
		t.Errorf("K: expected row=%d, got %d", rowBefore, m.log.sel.cursor.visualRow)
	}

	if m.log.sel.anchor != anchor {
		t.Error("J/K: anchor must not move")
	}
}

// --- TP-105-08: PgDn moves cursor by Height-1 rows; viewport follows ---

func TestKeys_PgDn_MovesCursor_InSelectMode(t *testing.T) {
	m := newSelectModelWithLines(t, 30)
	startRow := m.log.sel.cursor.visualRow
	height := m.log.viewport.Height
	pageSize := height - 1

	pgDown := tea.KeyMsg{Type: tea.KeyPgDown}
	m = sendKey(t, m, pgDown)

	wantRow := startRow + pageSize
	if wantRow >= len(m.log.visualLines) {
		wantRow = len(m.log.visualLines) - 1
	}
	if m.log.sel.cursor.visualRow != wantRow {
		t.Errorf("PgDn: expected cursor.visualRow=%d, got %d", wantRow, m.log.sel.cursor.visualRow)
	}
	// Cursor must be visible: row in [YOffset, YOffset+Height-1].
	yo := m.log.viewport.YOffset
	if m.log.sel.cursor.visualRow < yo || m.log.sel.cursor.visualRow >= yo+height {
		t.Errorf("PgDn: cursor not visible: row=%d YOffset=%d Height=%d",
			m.log.sel.cursor.visualRow, yo, height)
	}
}

// --- TP-105-09: PgUp moves cursor by Height-1 rows upward ---

func TestKeys_PgUp_MovesCursor_InSelectMode(t *testing.T) {
	m := newSelectModelWithLines(t, 30)
	// Move cursor down first so there's room to go up.
	pgDown := tea.KeyMsg{Type: tea.KeyPgDown}
	m = sendKey(t, m, pgDown)
	startRow := m.log.sel.cursor.visualRow
	height := m.log.viewport.Height
	pageSize := height - 1

	pgUp := tea.KeyMsg{Type: tea.KeyPgUp}
	m = sendKey(t, m, pgUp)

	wantRow := startRow - pageSize
	if wantRow < 0 {
		wantRow = 0
	}
	if m.log.sel.cursor.visualRow != wantRow {
		t.Errorf("PgUp: expected cursor.visualRow=%d, got %d", wantRow, m.log.sel.cursor.visualRow)
	}
	// Cursor must be visible.
	yo := m.log.viewport.YOffset
	if m.log.sel.cursor.visualRow < yo || m.log.sel.cursor.visualRow >= yo+height {
		t.Errorf("PgUp: cursor not visible: row=%d YOffset=%d Height=%d",
			m.log.sel.cursor.visualRow, yo, height)
	}
}

// --- TP-105-10: virtual column preserved across a shorter line ---

func TestMoveCursor_VirtualColumn_PreservedAcrossShortLine(t *testing.T) {
	// Build a log with:
	//   row 0: 40 chars wide  (line 0: "a"*40)
	//   row 1: 5 chars wide   (line 1: "b"*5) — shorter
	//   row 2: 40 chars wide  (line 2: "c"*40)
	m, _ := newSelectTestModel(t, ModeNormal)
	lines := []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", // 38 'a's (>30)
		"bbbbb",                                  // 5 chars
		"cccccccccccccccccccccccccccccccccccccc", // 38 'c's (>30)
	}
	next, _ := m.Update(LogLinesMsg{Lines: lines})
	m = next.(Model)
	m.log.viewport.GotoTop()
	m = enterSelectMode(t, m)

	// Cursor starts at last visible row, col 0. Move up so we start at row 0.
	m = sendKey(t, m, keyMsg("k"))
	m = sendKey(t, m, keyMsg("k"))
	if m.log.sel.cursor.visualRow != 0 {
		t.Fatalf("precondition: expected cursor at row 0, got %d", m.log.sel.cursor.visualRow)
	}

	// Move right to col 30 on the wide row.
	for i := 0; i < 30; i++ {
		m = sendKey(t, m, keyMsg("l"))
	}
	if m.log.sel.cursor.col != 30 {
		t.Fatalf("precondition: expected col=30, got %d", m.log.sel.cursor.col)
	}
	if m.log.virtualCol != 30 {
		t.Fatalf("precondition: expected virtualCol=30, got %d", m.log.virtualCol)
	}

	// Move down to the short row (row 1, width 5).
	m = sendKey(t, m, keyMsg("j"))
	if m.log.sel.cursor.visualRow != 1 {
		t.Fatalf("expected cursor at row 1 (short), got %d", m.log.sel.cursor.visualRow)
	}
	// Col must be clamped to 5 (last col of "bbbbb").
	if m.log.sel.cursor.col != 5 {
		t.Errorf("expected col clamped to 5 on short row, got %d", m.log.sel.cursor.col)
	}
	// VirtualCol must still be 30.
	if m.log.virtualCol != 30 {
		t.Errorf("expected virtualCol=30 preserved, got %d", m.log.virtualCol)
	}

	// Move down to the wide row (row 2, width ≥ 30).
	m = sendKey(t, m, keyMsg("j"))
	if m.log.sel.cursor.visualRow != 2 {
		t.Fatalf("expected cursor at row 2 (wide), got %d", m.log.sel.cursor.visualRow)
	}
	// Col must be restored to 30.
	if m.log.sel.cursor.col != 30 {
		t.Errorf("expected col restored to 30 on wide row, got %d", m.log.sel.cursor.col)
	}
}

// --- TP-105-11: moving onto a shorter line clamps col; virtualCol remembered ---

func TestMoveCursor_ClampsToLineEnd(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	lines := []string{
		"aaaaaaaaaaaaaaaaaaaaaa", // 22 chars
		"bb",                     // 2 chars
	}
	next, _ := m.Update(LogLinesMsg{Lines: lines})
	m = next.(Model)
	m.log.viewport.GotoTop()
	m = enterSelectMode(t, m)

	// Move up to row 0 (wide line).
	m = sendKey(t, m, keyMsg("k"))
	if m.log.sel.cursor.visualRow != 0 {
		t.Fatalf("precondition: expected row 0, got %d", m.log.sel.cursor.visualRow)
	}

	// Move right to col 20.
	for i := 0; i < 20; i++ {
		m = sendKey(t, m, keyMsg("l"))
	}
	if m.log.sel.cursor.col != 20 {
		t.Fatalf("precondition: expected col=20, got %d", m.log.sel.cursor.col)
	}

	// Move down to the short line (row 1, width 2).
	m = sendKey(t, m, keyMsg("j"))
	if m.log.sel.cursor.visualRow != 1 {
		t.Fatalf("expected row=1, got %d", m.log.sel.cursor.visualRow)
	}
	// col must be clamped to 2.
	if m.log.sel.cursor.col != 2 {
		t.Errorf("expected col clamped to 2, got %d", m.log.sel.cursor.col)
	}
	// virtualCol remembers 20.
	if m.log.virtualCol != 20 {
		t.Errorf("expected virtualCol=20, got %d", m.log.virtualCol)
	}
}

// --- TP-105-12: viewport autoscrolls when cursor moves out of view ---

func TestMoveCursor_ViewportFollowsCursor(t *testing.T) {
	m := newSelectModelWithLines(t, 30)
	// Cursor is at last visible row (row 9, YOffset=0, Height=10).
	if m.log.sel.cursor.visualRow != 9 {
		t.Fatalf("precondition: expected cursor at row 9, got %d", m.log.sel.cursor.visualRow)
	}

	// Press j — cursor moves to row 10, which is past YOffset(0)+Height(10).
	m = sendKey(t, m, keyMsg("j"))

	if m.log.sel.cursor.visualRow != 10 {
		t.Errorf("expected cursor.visualRow=10, got %d", m.log.sel.cursor.visualRow)
	}
	// Viewport must scroll so cursor is visible.
	yo := m.log.viewport.YOffset
	h := m.log.viewport.Height
	if m.log.sel.cursor.visualRow < yo || m.log.sel.cursor.visualRow >= yo+h {
		t.Errorf("cursor not visible: row=%d YOffset=%d Height=%d",
			m.log.sel.cursor.visualRow, yo, h)
	}
}

// --- TP-105-13: q from ModeSelect sets prevMode to pre-Select idle mode ---

func TestKeys_Q_FromSelect_PreservesIdleModeAsPrevMode(t *testing.T) {
	for _, idleMode := range []Mode{ModeNormal, ModeDone} {
		t.Run(idleMode.String(), func(t *testing.T) {
			m, _ := newSelectTestModel(t, idleMode)
			populateLog(t, &m, 5)

			// Enter ModeSelect from the idle mode.
			m = enterSelectMode(t, m)
			if m.keys.handler.Mode() != ModeSelect {
				t.Fatalf("expected ModeSelect, got %v", m.keys.handler.Mode())
			}

			// Press q — must enter QuitConfirm with prevMode = idleMode.
			m = sendKey(t, m, keyMsg("q"))

			if m.keys.handler.Mode() != ModeQuitConfirm {
				t.Errorf("expected ModeQuitConfirm after q, got %v", m.keys.handler.Mode())
			}

			// Press Esc from QuitConfirm — must restore to the idle mode, not ModeSelect.
			escMsg := tea.KeyMsg{Type: tea.KeyEsc}
			m = sendKey(t, m, escMsg)
			if m.keys.handler.Mode() != idleMode {
				t.Errorf("Esc from QuitConfirm should restore %v (pre-Select mode), got %v",
					idleMode, m.keys.handler.Mode())
			}
		})
	}
}

// --- TP-105-14: q from ModeSelect clears selection ---

func TestKeys_Q_FromSelect_ClearsSelection(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	populateLog(t, &m, 5)
	m = enterSelectMode(t, m)
	if !m.log.sel.active {
		t.Fatal("precondition: expected sel.active after v")
	}

	// Press q — selection must be cleared immediately.
	m = sendKey(t, m, keyMsg("q"))

	if m.log.sel.active || m.log.sel.committed {
		t.Errorf("expected selection cleared after q from ModeSelect, got active=%v committed=%v",
			m.log.sel.active, m.log.sel.committed)
	}
	if m.keys.handler.Mode() != ModeQuitConfirm {
		t.Errorf("expected ModeQuitConfirm after q, got %v", m.keys.handler.Mode())
	}
}

// --- TP-105-15: SelectedText reflects cursor position after movement ---

func TestExtractText_CursorMoved_UpdatesText(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	// Use a single line so selection text is predictable.
	next, _ := m.Update(LogLinesMsg{Lines: []string{"hello world"}})
	m = next.(Model)
	m.log.viewport.GotoTop()
	m = enterSelectMode(t, m)

	// Cursor starts at the last visible row col 0. With 1 line, that is row 0.
	// At this point anchor == cursor (empty selection), so SelectedText() == "".
	if m.log.SelectedText() != "" {
		t.Fatalf("precondition: expected empty SelectedText, got %q", m.log.SelectedText())
	}

	// Press l 5 times to move cursor to col 5 (past 'hello').
	for i := 0; i < 5; i++ {
		m = sendKey(t, m, keyMsg("l"))
	}
	if m.log.sel.cursor.col != 5 {
		t.Fatalf("expected cursor.col=5, got %d", m.log.sel.cursor.col)
	}

	// Anchor is at col 0, cursor at col 5 → SelectedText should be "hello".
	got := m.log.SelectedText()
	if got != "hello" {
		t.Errorf("expected SelectedText()=%q, got %q", "hello", got)
	}
}
