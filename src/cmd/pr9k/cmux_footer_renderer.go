package main

import (
	"strings"

	"github.com/mxriverlynn/pr9k/src/internal/ui"
	"github.com/mxriverlynn/pr9k/src/internal/uichrome"
	"github.com/mxriverlynn/pr9k/src/internal/version"
)

// cmuxFooterRenderer holds the display state for the cmux footer pane.
type cmuxFooterRenderer struct {
	termW        int
	termH        int
	helpExpanded bool
	keyHandler   *ui.KeyHandler
}

// newCmuxFooterRenderer creates a renderer. SetStatusLineActive(true) is called
// unconditionally so ? always opens the inline help expansion (D-22).
func newCmuxFooterRenderer() *cmuxFooterRenderer {
	actions := make(chan ui.StepAction, 10)
	h := ui.NewKeyHandler(nil, actions)
	h.SetStatusLineActive(true)
	return &cmuxFooterRenderer{keyHandler: h}
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

	if statusLine != "" {
		return versionLabel + "\n" + statusLine
	}
	return versionLabel
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
