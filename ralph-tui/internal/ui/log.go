package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// StepSeparator returns a visual separator line for the start of a step.
// Format: ── <step name> ─────────────
func StepSeparator(stepName string) string {
	return fmt.Sprintf("── %s ─────────────", stepName)
}

// StepStartBanner returns the two-line banner written to the log body when a
// step begins execution: the "Starting step: <name>" heading and an underline
// of box-drawing characters matching the heading's display width.
func StepStartBanner(stepName string) (heading, underline string) {
	heading = "Starting step: " + stepName
	underline = strings.Repeat("─", utf8.RuneCountInString(heading))
	return heading, underline
}

// PhaseBanner returns the two-line banner written to the log body when a
// workflow phase begins: the phase name on its own line followed by a full-
// width underline of double-line box-drawing characters. width is the number
// of underline columns to draw; callers should pass the log body's visible
// width so the underline spans the panel. A width below 1 is clamped to 1.
func PhaseBanner(phaseName string, width int) (heading, underline string) {
	if width < 1 {
		width = 1
	}
	return phaseName, strings.Repeat("═", width)
}

// CaptureLog returns a single-line log entry describing a variable binding
// produced by a step's captureAs. The value is rendered with %q so that
// multi-line, whitespace-heavy, or control-character payloads stay on one
// log line and remain readable.
//
// Example: CaptureLog("ISSUE_ID", "42") → `Captured ISSUE_ID = "42"`
func CaptureLog(varName, value string) string {
	return fmt.Sprintf("Captured %s = %q", varName, value)
}

// RetryStepSeparator returns a visual separator line for a step retry.
// Format: ── <step name> (retry) ─────────────
func RetryStepSeparator(stepName string) string {
	return fmt.Sprintf("── %s (retry) ─────────────", stepName)
}

// CompletionSummary returns the final summary line written to the log body
// after all iterations and finalize steps have completed.
// Example output: "Ralph completed after 3 iteration(s) and 2 finalizing tasks."
func CompletionSummary(iterationsRun, finalizeCount int) string {
	return fmt.Sprintf(
		"Ralph completed after %d iteration(s) and %d finalizing tasks.",
		iterationsRun, finalizeCount,
	)
}
