package workflowedit

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
)

// renderFindingsPanel renders the D38 findings panel dialog with actual finding
// entries. When m.helpOpen is true (help modal open above), entries and the
// footer are rendered in Color("8") (dim) so the help overlay reads cleanly
// over the panel (dim-under-help coexistence).
//
// D39: An acknowledged non-fatal finding carries the [WARN ✓] prefix instead of
// [WARN], so the user can see which warnings have been suppressed for this session.
func (m Model) renderFindingsPanel() string {
	dimmed := m.helpOpen

	var rows []string
	if len(m.findingsPanel.entries) == 0 {
		text := "No validation findings."
		if dimmed {
			text = lipgloss.NewStyle().Foreground(uichrome.Dim).Render(text)
		}
		rows = []string{text}
	} else {
		for _, e := range m.findingsPanel.entries {
			prefix := findingEntryPrefix(e, m.findingsPanel.ackSet)
			line := prefix + " " + e.text
			if dimmed {
				line = lipgloss.NewStyle().Foreground(uichrome.Dim).Render(line)
			}
			rows = append(rows, line)
		}
	}

	footer := "[ Enter  acknowledge ]  [ Esc  close ]"
	if dimmed {
		footer = lipgloss.NewStyle().Foreground(uichrome.Dim).Render(footer)
	}

	body := dialogBody{
		title:  "Findings",
		rows:   rows,
		footer: footer,
	}
	return renderDialogShell(body, m.width, m.height)
}

// findingEntryPrefix returns the severity prefix for a finding entry.
//   - Fatal findings:              [FATAL]
//   - Non-fatal, acknowledged:     [WARN ✓]  (D39)
//   - Non-fatal, not acknowledged: [WARN]
func findingEntryPrefix(e findingEntry, ackSet map[string]bool) string {
	if e.isFatal {
		return "[FATAL]"
	}
	if ackSet[e.key] {
		return "[WARN ✓]"
	}
	return "[WARN]"
}
