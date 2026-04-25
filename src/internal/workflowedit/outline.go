package workflowedit

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// outlinePanel is the left-hand step-list pane.
type outlinePanel struct {
	vp      viewport.Model
	cursor  int
	width   int
	height  int
	scrolls int // incremented per scroll event; aids test assertions
}

func newOutlinePanel(width, height int) outlinePanel {
	return outlinePanel{
		vp:     viewport.New(width, height),
		width:  width,
		height: height,
	}
}

// ShortcutLine returns the shortcut hints appropriate for the current outline
// state (D-11).
func (o outlinePanel) ShortcutLine(reorderMode bool) string {
	if reorderMode {
		return "↑/↓  move  ·  Enter  commit  ·  Esc  cancel"
	}
	return "↑/↓  navigate  ·  Tab  detail  ·  Del  delete  ·  r  reorder  ·  Alt+↑/↓  move"
}

// render builds the visible outline string from the steps slice.
func (o outlinePanel) render(steps []workflowmodel.Step, cursor int, reorderMode bool) string {
	if len(steps) == 0 {
		return "(no steps)\n"
	}
	var sb strings.Builder
	for i, step := range steps {
		name := step.Name
		if name == "" {
			name = HintNoName
		}
		var prefix string
		switch {
		case reorderMode && i == cursor:
			prefix = GlyphGripper + " "
		case i == cursor:
			prefix = "> "
		default:
			prefix = "  "
		}
		sb.WriteString(prefix + name + "\n")
	}
	return sb.String()
}
