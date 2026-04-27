package workflowedit

import "github.com/mxriverlynn/pr9k/src/internal/uichrome"

// renderHelpModal returns the D40 bordered help-modal overlay listing every
// keyboard shortcut available in the current mode. The modal is rendered using
// the same renderDialogShell chrome as other dialogs and is spliced over the
// content frame by View() via uichrome.Overlay when m.helpOpen is true.
func (m Model) renderHelpModal() string {
	body := dialogBody{
		title: "Help",
		rows: []string{
			"Global:",
			"  Ctrl+N   New workflow",
			"  Ctrl+O   Open workflow",
			"  Ctrl+S   Save",
			"  Ctrl+Q   Quit",
			"",
			"Outline navigation:",
			"  Up/Down   Move cursor",
			"  Space     Collapse / expand section",
			"  a         Add step in section",
			"  Tab       Edit step fields",
			"  Del       Delete step",
			"  r         Rename step",
			"  Alt+Up/Down  Reorder step",
			"",
			"  ?    Close help",
			"  Esc  Close help",
		},
		footer: "[ ?  close ]  [ Esc  close ]",
		width:  uichrome.HelpModalMaxWidth,
	}
	return renderDialogShell(body, m.width, m.height)
}
