package workflowedit

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/mxriverlynn/pr9k/src/internal/workflowmodel"
)

// detailPane is the right-hand field-editing pane.
type detailPane struct {
	vp            viewport.Model
	cursor        int
	revealedField int // index of unmasked containerEnv field; -1 if none
	dropdownOpen  bool
	width         int
	height        int
	scrolls       int // incremented per scroll event; aids test assertions
}

func newDetailPane(width, height int) detailPane {
	return detailPane{
		vp:            viewport.New(width, height),
		revealedField: -1,
		width:         width,
		height:        height,
	}
}

// ShortcutLine returns the shortcut hints appropriate for the current detail
// state (D-11).
func (d detailPane) ShortcutLine() string {
	if d.dropdownOpen {
		return "type to filter  ·  Enter  confirm  ·  Esc  cancel"
	}
	return "Tab  outline  ·  ↑/↓  navigate  ·  Enter  edit  ·  r  reveal/mask"
}

// render builds the visible detail string for the given step.
func (d detailPane) render(step workflowmodel.Step) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Name: %s\n", step.Name)
	fmt.Fprintf(&sb, "Kind: %s\n", string(step.Kind))
	if step.Kind == workflowmodel.StepKindClaude {
		fmt.Fprintf(&sb, "Model: %s\n", step.Model)
		fmt.Fprintf(&sb, "PromptFile: %s\n", step.PromptFile)
	}
	if step.Kind == workflowmodel.StepKindShell {
		fmt.Fprintf(&sb, "Command: %v\n", step.Command)
	}
	for i, env := range step.Env {
		if env.IsLiteral {
			val := env.Value
			if d.revealedField != i {
				val = GlyphMasked
			}
			fmt.Fprintf(&sb, "containerEnv[%d]: %s=%s\n", i, env.Key, val)
		} else {
			fmt.Fprintf(&sb, "env[%d]: %s\n", i, env.Key)
		}
	}
	return sb.String()
}
