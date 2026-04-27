package workflowedit

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestEmpty_OutlinePaneAndHintPanel verifies that the D43 empty-editor layout
// renders both the outline pane and the detail-pane hint with bordered chrome.
// At least two ╭ corner glyphs must appear (frame top border + two bordered panes).
func TestEmpty_OutlinePaneAndHintPanel(t *testing.T) {
	const termW, termH = 80, 24
	m := applyMsg(newTestModel(), tea.WindowSizeMsg{Width: termW, Height: termH})
	view := stripView(m)
	count := strings.Count(view, "╭")
	if count < 2 {
		t.Errorf("empty-editor view has %d ╭ glyphs, want ≥2 (frame top + two bordered panes); view:\n%s",
			count, view)
	}
}

// TestEmpty_HintTextInDetailPane verifies the detail pane carries the D43 hint
// text directing the user to File > New and File > Open.
func TestEmpty_HintTextInDetailPane(t *testing.T) {
	const termW, termH = 80, 24
	m := applyMsg(newTestModel(), tea.WindowSizeMsg{Width: termW, Height: termH})
	view := stripView(m)
	if !strings.Contains(view, "File > New") {
		t.Errorf("empty-editor detail pane should contain \"File > New\" hint; view:\n%s", view)
	}
	if !strings.Contains(view, "File > Open") {
		t.Errorf("empty-editor detail pane should contain \"File > Open\" hint; view:\n%s", view)
	}
}

// TestEmpty_HintEmptyConstantRemoved verifies that the HintEmpty constant was
// removed from constants.go (D-26: superseded by D43 bordered layout).
// Token assembled at runtime so this guard file itself does not contain it as a literal.
func TestEmpty_HintEmptyConstantRemoved(t *testing.T) {
	src, err := os.ReadFile("constants.go")
	if err != nil {
		t.Fatalf("could not read constants.go: %v", err)
	}
	token := "Hint" + "Empty"
	if strings.Contains(string(src), token) {
		t.Errorf("constants.go must not contain %q (D-26: superseded by D43 bordered layout)", token)
	}
}

// TestEmpty_StillUses9RowChrome verifies that the empty-editor view preserves
// the 9-row chrome budget: View() must produce exactly m.height lines.
func TestEmpty_StillUses9RowChrome(t *testing.T) {
	const termW, termH = 80, 24
	m := applyMsg(newTestModel(), tea.WindowSizeMsg{Width: termW, Height: termH})
	view := stripView(m)
	lines := strings.Split(view, "\n")
	if len(lines) != termH {
		t.Errorf("empty-editor view has %d lines, want %d (9-row chrome budget must be preserved)",
			len(lines), termH)
	}
}
