package uichrome

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ColorTitle applies the top-border title's two-tone palette: the app name
// (everything before the first " — " separator) renders green, and the
// iteration detail that follows renders white. When the title has no
// separator (e.g. the bare app name before any iteration starts), the
// whole string renders green.
func ColorTitle(title string) string {
	const sep = " — "
	green := lipgloss.NewStyle().Foreground(Green)
	white := lipgloss.NewStyle().Foreground(White)
	if idx := strings.Index(title, sep); idx >= 0 {
		return green.Render(title[:idx]) + white.Render(title[idx:])
	}
	return green.Render(title)
}
