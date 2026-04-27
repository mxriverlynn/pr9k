package workflowedit

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
)

// ShortcutLine returns the shortcut footer text for the current focus and
// overlay state. It delegates to the focused widget's ShortcutLine method
// (D-11). The "?  help" hint is suppressed while any non-findings-panel dialog
// is active.
//
// D-17: early-returns "Validating…" or "Saving…" when those operations are in
// progress so the user gets immediate transient feedback.
//
// D-18: appends a Dim-styled "Ctrl+S  save" hint when browse-only (isReadOnly)
// to signal that saving is unavailable in the current session.
func (m Model) ShortcutLine() string {
	// D-17: transient states override normal shortcuts.
	if m.validateInProgress {
		return "Validating…"
	}
	if m.saveInProgress {
		return "Saving…"
	}

	var base string
	switch m.focus {
	case focusOutline:
		base = m.outline.ShortcutLine(m.reorderMode, m.doc)
	case focusDetail:
		base = m.detail.ShortcutLine()
	default:
		base = m.menu.ShortcutLine()
	}

	// Append the help hint to the plain-text base before two-toning so
	// ColorShortcutLine can style "?" white and " help" gray in one pass.
	// Suppress "? help" for all dialog kinds except DialogFindingsPanel.
	if m.dialog.kind == DialogNone || m.dialog.kind == DialogFindingsPanel {
		base += "  ·  ? help"
	}

	// Apply two-tone palette (D34): keys white, descriptions gray.
	result := uichrome.ColorShortcutLine(base)

	// D-18: browse-only hint appended after two-toning so the pre-styled
	// Dim text is not re-processed by ColorShortcutLine.
	if m.banners.isReadOnly && m.loaded {
		dim := lipgloss.NewStyle().Foreground(uichrome.Dim)
		result += "  ·  " + dim.Render("save  [ro]")
	}

	return result
}
