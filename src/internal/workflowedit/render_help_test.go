package workflowedit

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestHelpModal_RenderShape verifies the D40 help modal has bordered chrome (╭)
// and contains keyboard shortcut content.
func TestHelpModal_RenderShape(t *testing.T) {
	m := newTestModel()
	m.helpOpen = true
	view := stripView(m)
	if !strings.Contains(view, "╭") {
		t.Errorf("help modal should have D40 bordered chrome (╭); view:\n%s", view)
	}
	if !strings.Contains(view, "Ctrl+N") {
		t.Errorf("help modal should contain Ctrl+N shortcut; view:\n%s", view)
	}
	if !strings.Contains(view, "Ctrl+S") {
		t.Errorf("help modal should contain Ctrl+S shortcut; view:\n%s", view)
	}
}

// TestHelpModal_FitsFrame verifies that assertModalFits passes at the minimum
// terminal size (60×16), i.e. the help modal does not break the chrome frame.
func TestHelpModal_FitsFrame(t *testing.T) {
	m := applyMsg(newTestModel(), tea.WindowSizeMsg{Width: 60, Height: 16})
	m.helpOpen = true
	assertModalFits(t, m)
}
