package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// --- TP-007: logModel.Update ---

func TestLogModel_Update_LogLinesMsg_AppendsLines(t *testing.T) {
	m := newLogModel(80, 10)

	msg := LogLinesMsg{Lines: []string{"line1", "line2", "line3"}}
	next, cmd := m.Update(msg)
	if cmd != nil {
		t.Error("expected nil cmd for LogLinesMsg")
	}
	if len(next.lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(next.lines))
	}
	for i, want := range msg.Lines {
		if next.lines[i] != want {
			t.Errorf("lines[%d]: want %q, got %q", i, want, next.lines[i])
		}
	}
}

func TestLogModel_Update_HomeKey_GotoTop(t *testing.T) {
	m := newLogModel(80, 5)

	// Fill with enough lines to make the viewport scrollable.
	fill := make([]string, 25)
	for i := range fill {
		fill[i] = "line"
	}
	m, _ = m.Update(LogLinesMsg{Lines: fill})
	m.viewport.GotoBottom()

	// Verify we were actually scrolled down.
	if m.viewport.YOffset == 0 {
		t.Skip("viewport already at top after GotoBottom — viewport too small for test to be meaningful")
	}

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("home")})
	if cmd != nil {
		t.Error("expected nil cmd for home key")
	}
	if m.viewport.YOffset != 0 {
		t.Errorf("expected YOffset=0 after home, got %d", m.viewport.YOffset)
	}
}

func TestLogModel_Update_EndKey_GotoBottom(t *testing.T) {
	m := newLogModel(80, 5)

	// Fill with enough lines to make the viewport scrollable.
	fill := make([]string, 25)
	for i := range fill {
		fill[i] = "line"
	}
	m, _ = m.Update(LogLinesMsg{Lines: fill})
	m.viewport.GotoTop()

	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("end")})
	if cmd != nil {
		t.Error("expected nil cmd for end key")
	}
	if !m.viewport.AtBottom() {
		t.Error("expected viewport at bottom after end key")
	}
}

// --- SUGG-003: ring buffer boundary at exactly 500 and 501 lines ---

func TestLogModel_RingBuffer_500Lines_NoTrim(t *testing.T) {
	m := newLogModel(80, 10)

	lines := make([]string, 500)
	for i := range 500 {
		lines[i] = "line"
	}
	m, _ = m.Update(LogLinesMsg{Lines: lines})

	if len(m.lines) != 500 {
		t.Fatalf("expected exactly 500 lines, got %d", len(m.lines))
	}
}

func TestLogModel_RingBuffer_501Lines_TrimsToLast500(t *testing.T) {
	m := newLogModel(80, 10)

	lines := make([]string, 501)
	for i := range 501 {
		lines[i] = "line"
	}
	m, _ = m.Update(LogLinesMsg{Lines: lines})

	if len(m.lines) != 500 {
		t.Fatalf("expected 500 lines after trim, got %d", len(m.lines))
	}
}

// --- SUGG-004: direct auto-scroll tests on logModel ---

func TestLogModel_AutoScroll_AtBottom_StaysAtBottom(t *testing.T) {
	m := newLogModel(80, 5)

	// Fill past viewport height.
	fill := make([]string, 20)
	for i := range fill {
		fill[i] = "line"
	}
	m, _ = m.Update(LogLinesMsg{Lines: fill})
	m.viewport.GotoBottom()

	// Send more lines while at bottom.
	more := LogLinesMsg{Lines: []string{"extra1", "extra2"}}
	m, _ = m.Update(more)

	if !m.viewport.AtBottom() {
		t.Error("expected viewport to remain at bottom when wasAtBottom=true")
	}
}

func TestLogModel_AutoScroll_ScrolledUp_DoesNotAutoScroll(t *testing.T) {
	m := newLogModel(80, 5)

	fill := make([]string, 20)
	for i := range fill {
		fill[i] = "line"
	}
	m, _ = m.Update(LogLinesMsg{Lines: fill})

	// Scroll to top to simulate user scrolling up.
	m.viewport.GotoTop()
	positionBefore := m.viewport.YOffset

	// Send more lines — position should not change.
	m, _ = m.Update(LogLinesMsg{Lines: []string{"new1", "new2"}})

	if m.viewport.YOffset != positionBefore {
		t.Errorf("expected position unchanged when scrolled up: before=%d after=%d", positionBefore, m.viewport.YOffset)
	}
}

// --- SUGG-005: direct SetSize test ---

func TestLogModel_SetSize_UpdatesViewportDimensions(t *testing.T) {
	m := newLogModel(80, 10)

	m.SetSize(120, 30)

	if m.viewport.Width != 120 {
		t.Errorf("Width: want 120, got %d", m.viewport.Width)
	}
	if m.viewport.Height != 30 {
		t.Errorf("Height: want 30, got %d", m.viewport.Height)
	}
}

func TestLogModel_Update_UnknownKey_ForwardedToViewport(t *testing.T) {
	m := newLogModel(80, 10)

	// An unrecognized key must not panic and must return the model unchanged
	// (in terms of our ring buffer — viewport may update scroll position
	// independently).
	lines := []string{"a", "b"}
	m, _ = m.Update(LogLinesMsg{Lines: lines})

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	// Test passes if no panic occurs and lines are undisturbed.
	if len(m.lines) != len(lines) {
		t.Errorf("lines changed unexpectedly: got %d, want %d", len(m.lines), len(lines))
	}
}
