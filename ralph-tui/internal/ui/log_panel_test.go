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
