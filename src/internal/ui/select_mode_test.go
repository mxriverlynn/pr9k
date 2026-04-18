package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// newSelectTestModel returns a Model sized at 80×24 with the viewport
// sized to 76×10, and the underlying *KeyHandler. It starts in the given
// mode. Pre-populating lines is done separately by the caller. Callers
// that don't need the KeyHandler can discard it with _.
func newSelectTestModel(t *testing.T, mode Mode) (Model, *KeyHandler) {
	t.Helper()
	header := NewStatusHeader(1)
	header.SetPhaseSteps([]string{"step-one"})
	actions := make(chan StepAction, 10)
	kh := NewKeyHandler(func() {}, actions)
	m := NewModel(header, kh, "v0")
	m.width = 80
	m.height = 24
	m.log.SetSize(76, 10)
	kh.SetMode(mode)
	return m, kh
}

// populateLog adds n distinct lines to m's log model, re-rendering content.
func populateLog(t *testing.T, m *Model, n int) {
	t.Helper()
	lines := make([]string, n)
	for i := range n {
		lines[i] = strings.Repeat("x", i+1)
	}
	next, _ := m.Update(LogLinesMsg{Lines: lines})
	*m = next.(Model)
}

// --- TP-104-01: v enters ModeSelect from Normal ---

func TestKeys_V_EntersSelectMode_FromNormal(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	populateLog(t, &m, 5)

	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)

	if m.keys.handler.Mode() != ModeSelect {
		t.Errorf("expected ModeSelect after v, got %v", m.keys.handler.Mode())
	}
	if m.keys.handler.ShortcutLine() != SelectShortcuts {
		t.Errorf("expected SelectShortcuts footer, got %q", m.keys.handler.ShortcutLine())
	}
	if m.log.SelectedText() != "" {
		t.Errorf("expected SelectedText()=\"\" for empty cursor, got %q", m.log.SelectedText())
	}
}

// --- TP-104-02: v enters ModeSelect from both allowed idle modes ---

func TestKeys_V_EntersSelectMode_FromNormal_FromDone(t *testing.T) {
	for _, startMode := range []Mode{ModeNormal, ModeDone} {
		t.Run(startMode.String(), func(t *testing.T) {
			m, _ := newSelectTestModel(t, startMode)
			populateLog(t, &m, 5)

			next, _ := m.Update(keyMsg("v"))
			m = next.(Model)

			if m.keys.handler.Mode() != ModeSelect {
				t.Errorf("mode %v: expected ModeSelect after v, got %v", startMode, m.keys.handler.Mode())
			}
		})
	}
}

// --- TP-104-03: v is a no-op in Error / QuitConfirm / NextConfirm / Quitting ---

func TestKeys_V_IgnoredIn_Error_QuitConfirm_NextConfirm_Quitting(t *testing.T) {
	modes := []Mode{ModeError, ModeQuitConfirm, ModeNextConfirm, ModeQuitting}
	for _, startMode := range modes {
		t.Run(startMode.String(), func(t *testing.T) {
			m, _ := newSelectTestModel(t, startMode)
			populateLog(t, &m, 5)

			next, _ := m.Update(keyMsg("v"))
			m = next.(Model)

			if m.keys.handler.Mode() != startMode {
				t.Errorf("mode %v: expected mode unchanged after v, got %v", startMode, m.keys.handler.Mode())
			}
		})
	}
}

// --- TP-104-04: v with empty log buffer is a no-op ---

func TestKeys_V_EmptyViewport_NoOp(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	// No lines added — log is empty.

	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)

	if m.keys.handler.Mode() != ModeNormal {
		t.Errorf("expected mode unchanged (ModeNormal) for empty log, got %v", m.keys.handler.Mode())
	}
}

// --- TP-104-05: v places cursor at last visible visual row, column 0 ---

func TestKeys_V_StartsAtLastVisibleLine(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	// Populate 20 lines — more than the 10-row viewport.
	populateLog(t, &m, 20)

	// Scroll to the top so the viewport is not at the bottom.
	m.log.viewport.GotoTop()

	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)

	if m.keys.handler.Mode() != ModeSelect {
		t.Fatalf("expected ModeSelect, got %v", m.keys.handler.Mode())
	}

	sel := m.log.sel
	// Cursor must be at column 0.
	if sel.cursor.col != 0 {
		t.Errorf("expected cursor col=0, got %d", sel.cursor.col)
	}
	if sel.anchor.col != 0 {
		t.Errorf("expected anchor col=0, got %d", sel.anchor.col)
	}
	// Cursor must be on the last visible visual row (YOffset + Height - 1).
	wantRow := m.log.viewport.YOffset + m.log.viewport.Height - 1
	if wantRow >= len(m.log.visualLines) {
		wantRow = len(m.log.visualLines) - 1
	}
	if sel.cursor.visualRow != wantRow {
		t.Errorf("expected cursor visualRow=%d (last visible), got %d", wantRow, sel.cursor.visualRow)
	}
	if sel.anchor.visualRow != wantRow {
		t.Errorf("expected anchor visualRow=%d (last visible), got %d", wantRow, sel.anchor.visualRow)
	}
	// Cursor must not be at the viewport top row.
	if m.log.viewport.Height > 1 && sel.cursor.visualRow == m.log.viewport.YOffset {
		t.Error("cursor landed at viewport top row, not last visible row")
	}
}

// --- TP-104-06: Esc returns to the pre-Select mode ---

func TestKeys_Esc_ReturnsToPrevMode_FromSelect(t *testing.T) {
	for _, startMode := range []Mode{ModeNormal, ModeDone} {
		t.Run(startMode.String(), func(t *testing.T) {
			m, _ := newSelectTestModel(t, startMode)
			populateLog(t, &m, 5)

			// Enter ModeSelect.
			next, _ := m.Update(keyMsg("v"))
			m = next.(Model)
			if m.keys.handler.Mode() != ModeSelect {
				t.Fatalf("precondition: expected ModeSelect, got %v", m.keys.handler.Mode())
			}

			// Press Esc.
			escMsg := tea.KeyMsg{Type: tea.KeyEsc}
			next, _ = m.Update(escMsg)
			m = next.(Model)

			if m.keys.handler.Mode() != startMode {
				t.Errorf("expected mode restored to %v after esc, got %v", startMode, m.keys.handler.Mode())
			}
			// Selection must be cleared immediately (not on the next Update).
			if m.log.sel.active || m.log.sel.committed {
				t.Error("expected selection cleared after esc, but sel is still active/committed")
			}
		})
	}
}

// --- TP-104-07: external SetMode transition clears selection ---

func TestSetMode_ExternalTransition_ClearsSelection(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	populateLog(t, &m, 5)

	// Enter ModeSelect via v.
	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatalf("precondition: expected ModeSelect, got %v", m.keys.handler.Mode())
	}
	// Ensure selection is set.
	if !m.log.sel.active {
		t.Fatal("precondition: expected sel.active after v")
	}

	// Simulate orchestration goroutine calling SetMode(ModeError) externally.
	m.keys.handler.SetMode(ModeError)

	// Push any message through Model.Update — the guard fires at the top.
	next, _ = m.Update(HeartbeatTickMsg(time.Now()))
	m = next.(Model)

	if m.log.sel.active || m.log.sel.committed {
		t.Error("expected selection cleared after external SetMode(ModeError), but sel is still active/committed")
	}
}

// --- TP-104-08: j in ModeSelect moves the cursor; viewport follows via autoscroll only ---

func TestKeys_InSelectMode_DoesNotDoubleDispatchToViewport(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	// Populate enough lines that j can move the cursor past the viewport edge.
	populateLog(t, &m, 30)
	// Scroll to top so the cursor starts at the last visible row (row 9).
	m.log.viewport.GotoTop()

	// Enter ModeSelect.
	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatalf("precondition: expected ModeSelect, got %v", m.keys.handler.Mode())
	}
	cursorBefore := m.log.sel.cursor

	// Press j — in ModeSelect this moves the selection cursor down by 1 row.
	// The routing guard must prevent j from ALSO being dispatched to the
	// viewport's native scroll handler (which would over-scroll by one extra row).
	next, _ = m.Update(keyMsg("j"))
	m = next.(Model)

	// Cursor must have moved exactly 1 row down.
	wantRow := cursorBefore.visualRow + 1
	if m.log.sel.cursor.visualRow != wantRow {
		t.Errorf("expected cursor.visualRow=%d after j, got %d", wantRow, m.log.sel.cursor.visualRow)
	}
	// Viewport YOffset must reflect autoscroll only — not an extra viewport-level j-scroll.
	// With YOffset=0 and Height=10, cursor at row 10 triggers autoscroll to YOffset=1.
	wantOffset := 0
	if m.log.sel.cursor.visualRow >= m.log.viewport.Height {
		wantOffset = m.log.sel.cursor.visualRow - m.log.viewport.Height + 1
	}
	if m.log.viewport.YOffset != wantOffset {
		t.Errorf("viewport.YOffset=%d, want %d (autoscroll only, no double-dispatch)",
			m.log.viewport.YOffset, wantOffset)
	}
}

// --- TP-104-09: ModeSelect shortcut line ---

func TestShortcuts_SelectMode_RendersSelectShortcuts(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	populateLog(t, &m, 5)

	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)

	got := m.keys.handler.ShortcutLine()
	if got != SelectShortcuts {
		t.Errorf("expected SelectShortcuts footer, got %q", got)
	}
}

// --- TP-104-10: v select appears in Normal/Done shortcuts but not Error ---

func TestShortcuts_NormalAndDone_IncludeVSelect(t *testing.T) {
	if !strings.Contains(NormalShortcuts, "v select") {
		t.Errorf("NormalShortcuts does not contain 'v select': %q", NormalShortcuts)
	}
	if !strings.Contains(DoneShortcuts, "v select") {
		t.Errorf("DoneShortcuts does not contain 'v select': %q", DoneShortcuts)
	}
	if strings.Contains(ErrorShortcuts, "v") && strings.Contains(ErrorShortcuts, "select") {
		t.Errorf("ErrorShortcuts should not contain 'v select': %q", ErrorShortcuts)
	}
}

// --- TP-104-11: v from Done restores Done on Esc ---

func TestModel_VFromDone_RestoresDoneOnEsc(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeDone)
	populateLog(t, &m, 5)

	// Enter ModeSelect from Done.
	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatalf("precondition: expected ModeSelect, got %v", m.keys.handler.Mode())
	}

	// Esc must restore ModeDone, not ModeNormal.
	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	next, _ = m.Update(escMsg)
	m = next.(Model)

	if m.keys.handler.Mode() != ModeDone {
		t.Errorf("expected ModeDone after Esc from ModeSelect (entered from Done), got %v",
			m.keys.handler.Mode())
	}
}

// --- TP-104-12: Esc clears selection in the same Update call ---

func TestModel_EscClearsImmediately_NotNextUpdate(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	populateLog(t, &m, 5)

	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatalf("precondition: expected ModeSelect, got %v", m.keys.handler.Mode())
	}
	if !m.log.sel.active {
		t.Fatal("precondition: expected sel.active after v")
	}

	// Esc must clear the selection immediately — not on the next Update call.
	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	next, _ = m.Update(escMsg)
	m = next.(Model)

	if m.log.sel.active || m.log.sel.committed {
		t.Error("expected selection cleared immediately after Esc, got active/committed sel")
	}
	if m.keys.handler.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal after Esc, got %v", m.keys.handler.Mode())
	}
}

// --- TP-104-13: prevObservedMode double-guard is idempotent ---

func TestModel_PrevObservedMode_DoubleGuard_Idempotent(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	populateLog(t, &m, 5)

	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)

	// Esc exits ModeSelect and clears selection immediately.
	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	next, _ = m.Update(escMsg)
	m = next.(Model)
	if m.log.sel.active || m.log.sel.committed {
		t.Fatal("precondition: expected sel cleared after Esc")
	}

	// Push a second message — the prevObservedMode guard must be a no-op.
	next, _ = m.Update(keyMsg("j"))
	m = next.(Model)

	if m.log.sel.active || m.log.sel.committed {
		t.Error("second Update must not re-activate sel; prevObservedMode guard must be idempotent")
	}
	if m.keys.handler.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal after second Update, got %v", m.keys.handler.Mode())
	}
}

// --- TP-104-14: LogLinesMsg in ModeSelect does not clear selection ---

func TestModel_LogLinesMsg_InSelect_DoesNotClearSelection(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	populateLog(t, &m, 5)

	// Enter ModeSelect.
	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatalf("precondition: expected ModeSelect, got %v", m.keys.handler.Mode())
	}
	if !m.log.sel.active {
		t.Fatal("precondition: expected sel.active after v")
	}

	// Receive LogLinesMsg while in ModeSelect — must not clear the selection.
	next, _ = m.Update(LogLinesMsg{Lines: []string{"new line"}})
	m = next.(Model)

	if m.keys.handler.Mode() != ModeSelect {
		t.Errorf("expected ModeSelect after LogLinesMsg, got %v", m.keys.handler.Mode())
	}
	if !m.log.sel.active {
		t.Error("expected sel.active preserved after LogLinesMsg in ModeSelect")
	}
}

// --- TP-104-15: Unknown key in ModeSelect is a no-op ---

func TestHandleSelect_UnknownKey_NoOp(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	populateLog(t, &m, 5)

	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatalf("precondition: expected ModeSelect, got %v", m.keys.handler.Mode())
	}

	// Press an arbitrary key — must stay in ModeSelect with no side effects.
	next, _ = m.Update(keyMsg("x"))
	m = next.(Model)

	if m.keys.handler.Mode() != ModeSelect {
		t.Errorf("expected ModeSelect after 'x', got %v", m.keys.handler.Mode())
	}
	if !m.log.sel.active {
		t.Error("expected sel.active preserved after unknown key in ModeSelect")
	}
}

// --- TP-104-16: Home/End in ModeSelect do not scroll the viewport ---

func TestHandleSelect_HomeEnd_NotForwarded(t *testing.T) {
	m, _ := newSelectTestModel(t, ModeNormal)
	populateLog(t, &m, 30)
	// After populateLog, auto-scroll puts us at the bottom.
	// Record that offset and verify Home does not change it.
	offsetBefore := m.log.viewport.YOffset

	// Enter ModeSelect.
	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatalf("precondition: expected ModeSelect, got %v", m.keys.handler.Mode())
	}

	// Press Home — the routing guard must prevent the log viewport from scrolling.
	homeMsg := tea.KeyMsg{Type: tea.KeyHome}
	next, _ = m.Update(homeMsg)
	m = next.(Model)

	if m.log.viewport.YOffset != offsetBefore {
		t.Errorf("Home in ModeSelect changed viewport YOffset: before=%d after=%d",
			offsetBefore, m.log.viewport.YOffset)
	}
	if m.keys.handler.Mode() != ModeSelect {
		t.Errorf("expected ModeSelect after Home, got %v", m.keys.handler.Mode())
	}
}

// Mode.String returns a human-readable mode name for test output.
func (m Mode) String() string {
	switch m {
	case ModeNormal:
		return "ModeNormal"
	case ModeError:
		return "ModeError"
	case ModeQuitConfirm:
		return "ModeQuitConfirm"
	case ModeNextConfirm:
		return "ModeNextConfirm"
	case ModeDone:
		return "ModeDone"
	case ModeSelect:
		return "ModeSelect"
	case ModeQuitting:
		return "ModeQuitting"
	default:
		return "ModeUnknown"
	}
}
