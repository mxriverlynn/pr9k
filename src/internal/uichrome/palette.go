// Package uichrome provides shared chrome primitives (color palette, border
// helpers, overlay utilities, and chrome constants) consumed by both the
// run-mode TUI (internal/ui) and the workflow-builder TUI (internal/workflowedit).
// Centralising these here avoids an import cycle that would arise if
// workflowedit imported internal/ui directly (D-2).
package uichrome

import "github.com/charmbracelet/lipgloss"

var (
	// LightGray is the default foreground color for chrome: brackets, markers,
	// step names, shortcut bar, version label, and the box border.
	LightGray = lipgloss.Color("245")

	// White is the foreground color for the main log/content area and for
	// key tokens in the shortcut bar.
	White = lipgloss.Color("15")

	// Green is the foreground color for the app name in the top-border title
	// and for moving-step gripper in reorder mode.
	Green = lipgloss.Color("10")

	// Red is the foreground color for FATAL findings and the read-only banner (D45).
	Red = lipgloss.Color("9")

	// Yellow is the foreground color for WARN findings and external/symlink/
	// shared-install banners; also used for phase-boundary flash in reorder mode (D45).
	Yellow = lipgloss.Color("11")

	// Cyan is the foreground color for INFO findings and the unknown-field banner (D45).
	Cyan = lipgloss.Color("14")

	// Dim is the foreground color for placeholders and dimmed-when-overlaid content (D45).
	Dim = lipgloss.Color("8")

	// ActiveStepFG is the foreground color for the currently running step's
	// brackets and name — white so the active row pops against the light-gray chrome.
	ActiveStepFG = lipgloss.Color("15")

	// ActiveMarkerFG is the foreground color for the active step's marker glyph
	// (▸) so the triangle reads as "this one is running" at a glance.
	ActiveMarkerFG = lipgloss.Color("10")
)
