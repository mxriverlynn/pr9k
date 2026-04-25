package workflowedit

import tea "github.com/charmbracelet/bubbletea"

// ExecCallback is invoked by tea.ExecProcess when the editor process exits.
// The three-way type switch on err is defined in cmd/pr9k (F-107).
type ExecCallback func(err error) tea.Msg

// EditorRunner launches an external editor for the given file path.
// Editor resolution is private to the production implementation (D-6).
type EditorRunner interface {
	Run(filePath string, cb ExecCallback) tea.Cmd
}
