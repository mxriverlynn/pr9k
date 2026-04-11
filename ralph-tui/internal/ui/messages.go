package ui

import tea "github.com/charmbracelet/bubbletea"

// LogLinesMsg carries a batch of log lines from the drain goroutine into the
// Bubble Tea Update loop. Using batches rather than single-line messages
// reduces SetContent calls by ~100x under burst, keeping the O(N) viewport
// work amortized.
type LogLinesMsg struct{ Lines []string }

// headerStepStateMsg updates the checkbox state for step idx.
type headerStepStateMsg struct {
	idx   int
	state StepState
}

// headerIterationLineMsg sets the iteration-phase header line.
type headerIterationLineMsg struct {
	iter  int
	max   int
	issue string
}

// headerInitializeLineMsg sets the initialize-phase header line.
type headerInitializeLineMsg struct {
	stepNum   int
	stepCount int
	stepName  string
}

// headerFinalizeLineMsg sets the finalize-phase header line.
type headerFinalizeLineMsg struct {
	stepNum   int
	stepCount int
	stepName  string
}

// headerPhaseStepsMsg replaces the current step name list.
type headerPhaseStepsMsg struct {
	names []string
}

// Ensure LogLinesMsg satisfies tea.Msg at compile time.
var _ tea.Msg = LogLinesMsg{}
