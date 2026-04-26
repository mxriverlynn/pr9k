package uichrome

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Overlay splices the modal string over the base frame. top and left are the
// 0-indexed row and column offsets of the modal's top-left corner within the
// base frame. Lines in modal that would fall outside the base frame are
// silently clipped. Negative top/left values are clipped to zero.
func Overlay(base, modal string, top, left int) string {
	baseLines := strings.Split(base, "\n")
	modalLines := strings.Split(modal, "\n")
	for i, mline := range modalLines {
		row := top + i
		if row < 0 || row >= len(baseLines) {
			continue
		}
		baseLines[row] = SpliceAt(baseLines[row], mline, left)
	}
	return strings.Join(baseLines, "\n")
}

// SpliceAt replaces the visual column range [left, left+lipgloss.Width(insert))
// in base with insert. ANSI escape sequences and wide runes are handled
// correctly by the charmbracelet/x/ansi Truncate and TruncateLeft primitives:
// a wide rune straddling a boundary is replaced with a space, and all ANSI
// state outside the replaced region is preserved.
func SpliceAt(base, insert string, left int) string {
	if left < 0 {
		left = 0
	}
	head := ansi.Truncate(base, left, "")
	tail := ansi.TruncateLeft(base, left+lipgloss.Width(insert), "")
	return head + insert + tail
}
