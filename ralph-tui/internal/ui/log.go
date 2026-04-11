package ui

import "fmt"

// StepSeparator returns a visual separator line for the start of a step.
// Format: ── <step name> ─────────────
func StepSeparator(stepName string) string {
	return fmt.Sprintf("── %s ─────────────", stepName)
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
