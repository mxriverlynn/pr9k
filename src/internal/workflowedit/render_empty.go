package workflowedit

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
)

// renderEmptyEditor returns the D43 split layout shown when no workflow is loaded:
// a bordered outline pane (left) with a "(no workflow open)" placeholder and a
// bordered detail pane (right) with hint text inviting File > New / File > Open.
// Returns an empty string when dimensions are not yet initialized (viewFallback path).
func (m Model) renderEmptyEditor() string {
	if m.outline.width <= 0 || m.detail.width <= 0 {
		return ""
	}
	outlineStr := renderEmptyOutlinePane(m.outline.width, m.outline.height)
	detailStr := renderEmptyDetailPane(m.detail.width, m.detail.height)
	return lipgloss.JoinHorizontal(lipgloss.Top, outlineStr, detailStr)
}

// renderEmptyOutlinePane renders a bordered outline pane for the empty-editor
// state: same chrome as the populated outline, but with only a placeholder line.
func renderEmptyOutlinePane(width, height int) string {
	if width <= 0 || height < 3 {
		return strings.Repeat("\n", max(height-1, 0))
	}
	innerW := width - 2
	contentH := height - 2

	gray := lipgloss.NewStyle().Foreground(uichrome.LightGray)
	vbar := gray.Render("│")

	var sb strings.Builder
	sb.WriteString(uichrome.RenderTopBorder("Outline", width))
	sb.WriteString("\n")

	const placeholder = "(no workflow open)"
	for i := 0; i < contentH; i++ {
		var content string
		if i == 0 {
			content = placeholder
		}
		w := lipgloss.Width(content)
		switch {
		case w < innerW:
			content += strings.Repeat(" ", innerW-w)
		case w > innerW:
			content = lipgloss.NewStyle().MaxWidth(innerW).Render(content)
		}
		sb.WriteString(vbar + content + gray.Render(" ") + "\n")
	}
	sb.WriteString(uichrome.BottomBorder(innerW))
	return sb.String()
}

// renderEmptyDetailPane renders a bordered detail pane for the empty-editor
// state, containing D43 hint text centered vertically in the pane.
func renderEmptyDetailPane(width, height int) string {
	if width <= 0 || height < 3 {
		return strings.Repeat("\n", max(height-1, 0))
	}
	innerW := width - 2
	contentH := height - 2

	gray := lipgloss.NewStyle().Foreground(uichrome.LightGray)
	vbar := gray.Render("│")

	hintLines := []string{
		"File > New (Ctrl+N) — create a workflow",
		"File > Open (Ctrl+O) — open an existing config.json",
	}
	mid := contentH / 2

	var sb strings.Builder
	sb.WriteString(uichrome.RenderTopBorder("", width))
	sb.WriteString("\n")

	for i := 0; i < contentH; i++ {
		lineIdx := i - mid + len(hintLines)/2
		var content string
		if lineIdx >= 0 && lineIdx < len(hintLines) {
			content = hintLines[lineIdx]
		}
		w := lipgloss.Width(content)
		switch {
		case w < innerW:
			content += strings.Repeat(" ", innerW-w)
		case w > innerW:
			content = lipgloss.NewStyle().MaxWidth(innerW).Render(content)
		}
		sb.WriteString(vbar + content + vbar + "\n")
	}
	sb.WriteString(uichrome.BottomBorder(innerW))
	return sb.String()
}
