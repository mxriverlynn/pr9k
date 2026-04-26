package uichrome

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ColorShortcutLine applies the footer shortcut bar's two-tone palette: the
// mapped key token at the start of each "  "-separated group renders white,
// and its trailing description renders gray.
//
// This function handles only the generic two-tone case. Callers that need
// special handling for mode-specific prompt strings (e.g. quit-confirm,
// quitting line) should apply those checks before calling this function.
func ColorShortcutLine(s string) string {
	white := lipgloss.NewStyle().Foreground(White)
	gray := lipgloss.NewStyle().Foreground(LightGray)
	groups := strings.Split(s, "  ")
	for i, g := range groups {
		if idx := strings.IndexByte(g, ' '); idx >= 0 {
			groups[i] = white.Render(g[:idx]) + gray.Render(g[idx:])
		} else {
			groups[i] = white.Render(g)
		}
	}
	return strings.Join(groups, gray.Render("  "))
}
