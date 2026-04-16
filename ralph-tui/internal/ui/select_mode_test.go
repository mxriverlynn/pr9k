package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// newSelectTestModel returns a Model sized at 80×24 with the viewport
// sized to 76×10. It starts in the given mode. Pre-populating lines is
// done separately by the caller.
func newSelectTestModel(t *testing.T, mode Mode) Model {
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
	return m
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
	m := newSelectTestModel(t, ModeNormal)
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
			m := newSelectTestModel(t, startMode)
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
			m := newSelectTestModel(t, startMode)
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
	m := newSelectTestModel(t, ModeNormal)
	// No lines added — log is empty.

	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)

	if m.keys.handler.Mode() != ModeNormal {
		t.Errorf("expected mode unchanged (ModeNormal) for empty log, got %v", m.keys.handler.Mode())
	}
}

// --- TP-104-05: v places cursor at last visible visual row, column 0 ---

func TestKeys_V_StartsAtLastVisibleLine(t *testing.T) {
	m := newSelectTestModel(t, ModeNormal)
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
			m := newSelectTestModel(t, startMode)
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
	m := newSelectTestModel(t, ModeNormal)
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

// --- TP-104-08: j in ModeSelect does not scroll the viewport ---

func TestKeys_InSelectMode_DoesNotDoubleDispatchToViewport(t *testing.T) {
	m := newSelectTestModel(t, ModeNormal)
	// Populate enough lines to make j scrollable.
	populateLog(t, &m, 30)
	// Scroll to top so j would have room to move.
	m.log.viewport.GotoTop()
	offsetBefore := m.log.viewport.YOffset

	// Enter ModeSelect.
	next, _ := m.Update(keyMsg("v"))
	m = next.(Model)
	if m.keys.handler.Mode() != ModeSelect {
		t.Fatalf("precondition: expected ModeSelect, got %v", m.keys.handler.Mode())
	}

	// Press j — in ModeSelect this is a no-op in this ticket; the routing
	// guard must prevent it from being double-dispatched to the viewport.
	next, _ = m.Update(keyMsg("j"))
	m = next.(Model)

	if m.log.viewport.YOffset != offsetBefore {
		t.Errorf("viewport.YOffset changed after j in ModeSelect: before=%d after=%d",
			offsetBefore, m.log.viewport.YOffset)
	}
}

// --- TP-104-09: ModeSelect shortcut line ---

func TestShortcuts_SelectMode_RendersSelectShortcuts(t *testing.T) {
	m := newSelectTestModel(t, ModeNormal)
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
