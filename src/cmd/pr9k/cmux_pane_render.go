package main

import (
	"fmt"
	"strings"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// renderCmuxStateHeader converts a StateHeader message into a plain-text
// representation of the step-checkbox grid and iteration line. Each step is
// rendered as "[marker] name" with HeaderCols steps per row, matching the
// markers the standard display uses: ✗ for StepFailed, ✓ for StepDone, etc.
func renderCmuxStateHeader(msg interactionchannel.StateHeader) string {
	var sb strings.Builder
	if msg.IterationLine != "" {
		sb.WriteString(msg.IterationLine)
		sb.WriteString("\n")
	}
	for i := 0; i < len(msg.StepNames); i += ui.HeaderCols {
		for c := range ui.HeaderCols {
			idx := i + c
			if idx >= len(msg.StepNames) {
				break
			}
			if c > 0 {
				sb.WriteString("  ")
			}
			var state ui.StepState
			if idx < len(msg.StepStates) {
				state = ui.StepState(msg.StepStates[idx])
			}
			fmt.Fprintf(&sb, "[%s] %s", cmuxStepMarker(state), msg.StepNames[idx])
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// cmuxStepMarker returns the single-character marker for a step state, matching
// the standard display's cellStyle glyph set.
func cmuxStepMarker(state ui.StepState) string {
	switch state {
	case ui.StepActive:
		return "▸"
	case ui.StepDone:
		return "✓"
	case ui.StepFailed:
		return "✗"
	case ui.StepSkipped:
		return "-"
	case ui.StepTimedOutContinuing:
		return "!"
	default:
		return " "
	}
}

// renderCmuxHeaderPaneContent returns the string to display in the header
// pane for the given terminal dimensions. Below 60×16 it renders the
// minimum-size advisory; above threshold it returns the pane's normal content.
func renderCmuxHeaderPaneContent(w, h int) string {
	if cmuxPaneTooSmall(w, h) {
		return cmuxMinSizeAdvisory()
	}
	return ""
}

// renderCmuxLogPaneContent returns the string to display in the log pane for
// the given terminal dimensions. Below 60×16 it renders the minimum-size
// advisory; above threshold it returns the pane's normal content.
func renderCmuxLogPaneContent(w, h int) string {
	if cmuxPaneTooSmall(w, h) {
		return cmuxMinSizeAdvisory()
	}
	return ""
}

// renderCmuxOrchestratorPaneContent returns the string to display in the
// orchestrator pane for the given terminal dimensions. Below 60×16 it renders
// the minimum-size advisory; above threshold it returns the pane's normal
// content.
func renderCmuxOrchestratorPaneContent(w, h int) string {
	if cmuxPaneTooSmall(w, h) {
		return cmuxMinSizeAdvisory()
	}
	return ""
}
