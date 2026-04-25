package workflowedit

// ShortcutLine returns the shortcut footer text for the current focus and
// overlay state. It delegates to the focused widget's ShortcutLine method
// (D-11). The "?  help" hint is suppressed while any non-findings-panel dialog
// is active (D-8).
func (m Model) ShortcutLine() string {
	var base string
	switch m.focus {
	case focusOutline:
		base = m.outline.ShortcutLine(m.reorderMode)
	case focusDetail:
		base = m.detail.ShortcutLine()
	default:
		base = m.menu.ShortcutLine()
	}

	// Suppress "?  help" for all dialog kinds except DialogFindingsPanel.
	if m.dialog.kind != DialogNone && m.dialog.kind != DialogFindingsPanel {
		return base
	}
	return base + "  ·  ?  help"
}
