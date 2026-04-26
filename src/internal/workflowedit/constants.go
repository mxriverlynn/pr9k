// Package workflowedit is the Bubble Tea model for the workflow-builder TUI.
// It holds SaveFS and EditorRunner interfaces injected at construction so tests
// can substitute doubles without spawning real processes or touching disk.
package workflowedit

// Affordance glyphs and placeholder strings used across the workflow-builder
// TUI (D-35). Centralising them here gives a single change-site for any
// affordance update.
const (
	// GlyphGripper is shown next to the focused row in reorder mode (F-54).
	GlyphGripper = "⋮⋮"

	// Section-collapse chevrons (D-28).
	GlyphSectionOpen  = "▾"
	GlyphSectionClose = "▸"

	// Aliases used by the structured outline tree (D-PR2-7).
	GlyphChevronExpanded  = GlyphSectionOpen
	GlyphChevronCollapsed = GlyphSectionClose

	// GlyphAddItem prefixes the "+ Add" affordance row (D-46).
	GlyphAddItem = "+"

	// GlyphMasked replaces the visible value of sensitive containerEnv entries.
	GlyphMasked = "••••••••"

	// HintNoName is displayed in the outline when a step has no name.
	HintNoName = "(unnamed)"

	// HintEmpty is the centred hint shown when no workflow is loaded (D-30).
	HintEmpty = "No workflow open.  Ctrl+N  new  ·  Ctrl+O  open"
)
