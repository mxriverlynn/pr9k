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

	// D-18: browse-only: append a greyed "save  [ro]" hint to signal that
	// saving is unavailable because the workflow file is read-only.
	if m.banners.isReadOnly && m.loaded {
		dim := lipgloss.NewStyle().Foreground(uichrome.Dim)
		base += "  ·  " + dim.Render("save  [ro]")
	}

	// Suppress "?  help" for all dialog kinds except DialogFindingsPanel.
	if m.dialog.kind != DialogNone && m.dialog.kind != DialogFindingsPanel {
		return base
	}
	return base + "  ·  ?  help"
}
