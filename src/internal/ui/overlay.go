package ui

import "github.com/mxriverlynn/pr9k/src/internal/uichrome"

// overlay splices the modal string over the base frame. top and left are the
// 0-indexed row and column offsets of the modal's top-left corner within the
// base frame. Lines in modal that would fall outside the base frame are
// silently clipped. Negative top/left values are clipped to zero.
//
// Delegates to uichrome.Overlay — the canonical implementation lives there.
func overlay(base, modal string, top, left int) string {
	return uichrome.Overlay(base, modal, top, left)
}

// spliceAt replaces the visual column range [left, left+lipgloss.Width(insert))
// in base with insert. ANSI escape sequences and wide runes are handled
// correctly. Delegates to uichrome.SpliceAt.
func spliceAt(base, insert string, left int) string {
	return uichrome.SpliceAt(base, insert, left)
}
