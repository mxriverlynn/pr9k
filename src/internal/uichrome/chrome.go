package uichrome

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// WrapLine wraps a single content line in side border characters (│), truncating
// to innerWidth and right-padding with spaces so the right border stays
// vertically aligned across all rows.
func WrapLine(content string, innerWidth int) string {
	gray := lipgloss.NewStyle().Foreground(LightGray)
	vbar := gray.Render("│")
	if innerWidth <= 0 {
		return vbar + vbar
	}
	truncated := lipgloss.NewStyle().MaxWidth(innerWidth).Render(content)
	pad := innerWidth - lipgloss.Width(truncated)
	if pad < 0 {
		pad = 0
	}
	return vbar + truncated + strings.Repeat(" ", pad) + vbar
}

// HRuleLine returns a horizontal rule using ├─┤ T-junction glyphs, sized to
// innerWidth fill characters between the caps.
func HRuleLine(innerWidth int) string {
	gray := lipgloss.NewStyle().Foreground(LightGray)
	return gray.Render("├" + strings.Repeat("─", innerWidth) + "┤")
}

// BottomBorder returns the bottom border string using ╰─╯ glyphs, sized to
// innerWidth fill characters between the caps.
func BottomBorder(innerWidth int) string {
	gray := lipgloss.NewStyle().Foreground(LightGray)
	return gray.Render("╰" + strings.Repeat("─", innerWidth) + "╯")
}

// RenderTopBorder constructs the hand-built top border row with the dynamic
// title embedded. width is the total terminal column count (including corners).
// When the terminal is too narrow to fit even the corners, a plain rule is
// returned.
//
// Target shape: "╭── Title text ─────────────────────────────────────────────╮"
func RenderTopBorder(title string, width int) string {
	const tl, tr, h = "╭", "╮", "─"
	innerWidth := width - 2 // subtract corner glyphs
	const leadDashes = 2
	titleBudget := innerWidth - leadDashes - 1
	if titleBudget < 0 || width == 0 {
		rule := strings.Repeat(h, max(innerWidth, 0))
		return lipgloss.NewStyle().Foreground(LightGray).Render(tl + rule + tr)
	}

	// Do width math on the plain title, then apply coloring last so the
	// visible width stays accurate regardless of ANSI codes.
	plainTitle := title
	plainSegment := " " + plainTitle + " "
	titleWidth := lipgloss.Width(plainSegment)
	if titleWidth > titleBudget {
		plainTitle = lipgloss.NewStyle().MaxWidth(titleBudget - 2).Render(plainTitle)
		plainSegment = " " + plainTitle + " "
		titleWidth = lipgloss.Width(plainSegment)
	}
	titleSegment := " " + ColorTitle(plainTitle) + " "

	fillCount := innerWidth - leadDashes - titleWidth
	if fillCount < 0 {
		fillCount = 0
	}

	grayStyle := lipgloss.NewStyle().Foreground(LightGray)
	return grayStyle.Render(tl+strings.Repeat(h, leadDashes)) +
		titleSegment +
		grayStyle.Render(strings.Repeat(h, fillCount)+tr)
}
