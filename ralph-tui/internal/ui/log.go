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
