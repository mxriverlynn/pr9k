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
