package workflowedit

// renderSessionHeader renders session-header row 1 with the D5 5-slot layout:
// title, dirty indicator (D13, D18), banner (D14), [N more warnings] (D15),
// and findings summary + validation indicator right-aligned (D16).
// Overflow drops in D17 priority order. Invoked by render_frame.go for row 3.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
)

// renderSessionHeader returns the session-header row 1 string. The returned
// string is wrapped to innerW by render_frame.go via uichrome.WrapLine.
func (m Model) renderSessionHeader() string {
	innerW := m.width - 2
	if innerW <= 0 {
		// Zero-size model (viewFallback path): render without width constraint.
		return buildSessionHeaderRow(m, 0)
	}
	return buildSessionHeaderRow(m, innerW)
}

// buildSessionHeaderRow assembles the five D5 slots with D17 overflow priority.
func buildSessionHeaderRow(m Model, width int) string {
	// Slot 1: title (workflow path or "(unsaved)").
	title := m.workflowDir
	if title == "" {
		title = "(unsaved)"
	}

	// Slot 2: dirty indicator ● in Green (D13); suppressed for read-only (D18).
	dirtyStr := ""
	if m.IsDirty() && !m.banners.isReadOnly {
		dirtyStr = lipgloss.NewStyle().Foreground(uichrome.Green).Render("●")
	}

	// Slot 3: banner (D14) — highest-priority from allBannerTexts.
	bannerTexts := m.banners.allBannerTexts()
	bannerStr := ""
	moreCount := 0
	if len(bannerTexts) > 0 {
		bannerStr = colorBannerTag(bannerTexts[0])
		moreCount = len(bannerTexts) - 1
	}

	// Slot 4: [N more warnings] affordance (D15) in White.
	moreStr := ""
	if moreCount > 0 {
		moreStr = lipgloss.NewStyle().Foreground(uichrome.White).Render(
			fmt.Sprintf("[%d more warnings]", moreCount))
	}

	// Slot 5 (right side): validation indicator + findings summary (D16).
	validStr := buildValidationIndicator(m)
	findingsStr := buildFindingsSummary(m)
	rightParts := make([]string, 0, 2)
	if validStr != "" {
		rightParts = append(rightParts, validStr)
	}
	if findingsStr != "" {
		rightParts = append(rightParts, findingsStr)
	}
	right := strings.Join(rightParts, " · ")

	return assembleHeaderRow(title, dirtyStr, bannerStr, moreStr, right, width)
}

// assembleHeaderRow builds the row string with D17 overflow priority:
// (1) drop [N more warnings], (2) drop right (findings+validation),
// (3) drop banner, (4) truncate path. Dirty indicator is never dropped while dirty.
func assembleHeaderRow(title, dirtyStr, bannerStr, moreStr, right string, width int) string {
	buildLeft := func(banner, more string) string {
		parts := []string{title}
		if dirtyStr != "" {
			parts = append(parts, dirtyStr)
		}
		if banner != "" {
			parts = append(parts, banner)
		}
		if more != "" {
			parts = append(parts, more)
		}
		return strings.Join(parts, " ")
	}

	padAndJoin := func(left, r string) string {
		if width <= 0 {
			if r == "" {
				return left
			}
			return left + "  " + r
		}
		lw := lipgloss.Width(left)
		rw := lipgloss.Width(r)
		gap := width - lw - rw
		if gap < 1 {
			gap = 1
		}
		if r == "" {
			return left
		}
		return left + strings.Repeat(" ", gap) + r
	}

	if width <= 0 {
		// No-constraint mode: return everything.
		return padAndJoin(buildLeft(bannerStr, moreStr), right)
	}

	fits := func(left, r string) bool {
		lw := lipgloss.Width(left)
		if r == "" {
			return lw <= width
		}
		return lw+1+lipgloss.Width(r) <= width
	}

	// Try with everything.
	left := buildLeft(bannerStr, moreStr)
	if fits(left, right) {
		return padAndJoin(left, right)
	}

	// D17 step 1: drop [N more warnings].
	left = buildLeft(bannerStr, "")
	if fits(left, right) {
		return padAndJoin(left, right)
	}

	// D17 step 2: drop right side (findings + validation).
	if fits(left, "") {
		return left
	}

	// D17 step 3: drop banner.
	left = buildLeft("", "")
	if fits(left, "") {
		return left
	}

	// D17 step 4: truncate path to fit.
	return truncateToWidth(title, dirtyStr, width)
}

// truncateToWidth truncates the title so that title + dirtyStr fits within width.
func truncateToWidth(title, dirtyStr string, width int) string {
	budget := width
	if dirtyStr != "" {
		budget -= lipgloss.Width(dirtyStr) + 1 // +1 for space
	}
	if budget <= 0 {
		return dirtyStr
	}
	runes := []rune(title)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > budget {
		runes = runes[:len(runes)-1]
	}
	t := string(runes)
	if dirtyStr != "" {
		return t + " " + dirtyStr
	}
	return t
}

// colorBannerTag applies D14 severity coloring to a banner tag.
func colorBannerTag(tag string) string {
	var color lipgloss.Color
	switch {
	case strings.HasPrefix(tag, "[ro]"):
		color = uichrome.Red
	case strings.HasPrefix(tag, "[ext]"),
		strings.HasPrefix(tag, "[sym"),
		strings.HasPrefix(tag, "[shared]"):
		color = uichrome.Yellow
	case strings.HasPrefix(tag, "[?fields]"):
		color = uichrome.Cyan
	default:
		color = uichrome.White
	}
	return lipgloss.NewStyle().Foreground(color).Render(tag)
}

// buildValidationIndicator returns the validation state string (WU-5).
// Returns "" before any validation has run (lastValidateOK == nil).
func buildValidationIndicator(m Model) string {
	if m.validateInProgress {
		return "Validating…"
	}
	if m.lastValidateOK == nil {
		return ""
	}
	if *m.lastValidateOK {
		return "Validated ✓"
	}
	return "Validation failed"
}

// buildFindingsSummary returns the D16 format string for non-zero finding counts.
// Format: "<F> fatal · <W> warn"; empty when all counts are zero.
func buildFindingsSummary(m Model) string {
	var fatals, warns int
	for _, e := range m.findingsPanel.entries {
		if e.isFatal {
			fatals++
		} else {
			warns++
		}
	}
	parts := make([]string, 0, 2)
	if fatals > 0 {
		parts = append(parts, fmt.Sprintf("%d fatal", fatals))
	}
	if warns > 0 {
		parts = append(parts, fmt.Sprintf("%d warn", warns))
	}
	return strings.Join(parts, " · ")
}
