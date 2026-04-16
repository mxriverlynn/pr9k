package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// LogLinesMsg carries a batch of log lines from the drain goroutine into the
// Bubble Tea Update loop. Using batches rather than single-line messages
// reduces SetContent calls by ~100x under burst, keeping the O(N) viewport
// work amortized.
type LogLinesMsg struct{ Lines []string }

// HeartbeatReader is implemented by types that report the silence duration of
// the active claude step pipeline. It is passed to the Model via
// WithHeartbeat so the TUI can render the D23 heartbeat indicator without
// importing the workflow package directly.
type HeartbeatReader interface {
	// HeartbeatSilence returns the duration since the last observed event from
	// the active claude pipeline, and whether a claude step is currently running.
	// Returns (0, false) when no pipeline is active.
	HeartbeatSilence() (time.Duration, bool)
}

// HeartbeatTickMsg is dispatched once per second by the explicit ticker
// goroutine in main.go to drive the D23 heartbeat indicator update in
// Model.Update → StatusHeader.HandleHeartbeatTick.
type HeartbeatTickMsg time.Time

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

// selectionChangedMsg is emitted (via tea.Cmd) by logModel selection cursor
// movement methods after the cursor has been updated and the viewport content
// has already been refreshed inline. model.go handles it as a no-op; the
// message exists so that future hooks (e.g. a status bar update) can react to
// cursor movement without re-architecting the dispatch path.
type selectionChangedMsg struct{}

// Ensure LogLinesMsg satisfies tea.Msg at compile time.
var _ tea.Msg = LogLinesMsg{}
