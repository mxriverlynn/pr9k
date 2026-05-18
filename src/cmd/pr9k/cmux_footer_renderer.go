package main

import (
	"strings"

	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
	"github.com/mxriverlynn/pr9k/src/internal/version"
)

// cmuxFooterRenderer holds the display state for the cmux footer pane.
// It is responsible only for rendering: terminal dimensions, help-expand
// toggle, and the current shortcut line pushed from the orchestrator.
type cmuxFooterRenderer struct {
	termW        int
	termH        int
	helpExpanded bool
	shortcutLine string
}

// newCmuxFooterRenderer creates a renderer with a generous default size so
// normal rendering is used until SetSize is called with real terminal dimensions.
func newCmuxFooterRenderer() *cmuxFooterRenderer {
	return &cmuxFooterRenderer{termW: 999, termH: 999}
}

// SetShortcutLine replaces the currently displayed shortcut line. Called when
// a StateFooter message arrives from the orchestrator or when the local footer
// state machine transitions to a new mode.
func (r *cmuxFooterRenderer) SetShortcutLine(line string) {
	r.shortcutLine = line
}

// SetSize updates the pane's terminal dimensions.
func (r *cmuxFooterRenderer) SetSize(w, h int) {
	r.termW = w
	r.termH = h
}

// HandleKey processes a key press and updates the renderer's display state.
func (r *cmuxFooterRenderer) HandleKey(key string) {
	switch key {
	case "?":
		r.helpExpanded = true
	case "esc":
		r.helpExpanded = false
	}
}

// Render returns the string to display in the footer pane. statusLine is the
// current status-line script output (may be empty).
func (r *cmuxFooterRenderer) Render(statusLine string) string {
	if cmuxPaneTooSmall(r.termW, r.termH) {
		return cmuxMinSizeAdvisory()
	}

	versionLabel := "pr9k v" + version.Version

	if r.helpExpanded {
		helpText := buildFooterHelpLines()
		// Reserve 2 rows for normal footer content (version label + status line).
		maxHelpRows := r.termH - 2
		if maxHelpRows <= 0 {
			return versionLabel
		}
		lines := strings.Split(helpText, "\n")
		if len(lines) > maxHelpRows {
			lines = lines[:maxHelpRows]
		}
		return strings.Join(lines, "\n") + "\n" + versionLabel
	}

	parts := []string{versionLabel}
	if r.shortcutLine != "" {
		parts = append(parts, r.shortcutLine)
	}
	if statusLine != "" {
		parts = append(parts, statusLine)
	}
	return strings.Join(parts, "\n")
}

// buildFooterHelpLines returns the help text for the footer pane's inline
// expansion, filtered to the footer's keystroke surface.
func buildFooterHelpLines() string {
	return "" +
		"  q   quit                   n   skip to next step\n" +
		"  ?   close this help"
}

// cmuxPaneTooSmall reports whether the given terminal dimensions are below the
// minimum required for any cmux pane to render normally (D-11).
func cmuxPaneTooSmall(w, h int) bool {
	return w < uichrome.MinTerminalWidth || h < uichrome.MinTerminalHeight
}

// cmuxMinSizeAdvisory returns the single-line advisory text rendered when a
// cmux pane's terminal dimensions are below the minimum threshold (D-11).
func cmuxMinSizeAdvisory() string {
	return "make this pane wider (need ≥60×16)"
}
