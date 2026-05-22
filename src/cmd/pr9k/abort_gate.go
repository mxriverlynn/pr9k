package main

import (
	"sync"
	"sync/atomic"
)

// AbortCause is a typed reason for an abort-gate transition. Using a distinct
// type prevents unintentional string constants from being passed as causes.
type AbortCause string

const (
	// AbortCauseStallDetected is set when the per-connection stall detector
	// fires and requests an orchestrator abort.
	AbortCauseStallDetected AbortCause = "stall_detected"
	// AbortCauseDisplayPaneExit is set when a display pane exits unexpectedly
	// and the orchestrator must abort to avoid a zombie run.
	AbortCauseDisplayPaneExit AbortCause = "display_pane_exit"
)

// abortGate is a one-way latch: the first call to trigger wins and sets the
// cause; all subsequent calls are no-ops. IsAborting and Cause are safe to
// call from any goroutine at any time.
//
// Use newAbortGate() to construct; the zero value is NOT valid (triggered
// channel is uninitialized).
type abortGate struct {
	once      sync.Once
	aborting  atomic.Bool
	causeOnce AbortCause
	triggered chan struct{} // closed by the winning trigger call
}

// newAbortGate returns an initialized, untriggered abortGate.
func newAbortGate() *abortGate {
	return &abortGate{triggered: make(chan struct{})}
}

// trigger fires the gate with the given cause. If the gate has already been
// triggered, this call is a no-op and the original cause is preserved.
func (g *abortGate) trigger(cause AbortCause) {
	g.once.Do(func() {
		g.causeOnce = cause
		g.aborting.Store(true)
		close(g.triggered)
	})
}

// C returns a channel that is closed when the gate is first triggered. The
// returned channel can be used in a select to wait for an abort signal without
// polling. The channel is never nil when the gate was created with newAbortGate.
func (g *abortGate) C() <-chan struct{} { return g.triggered }

// IsAborting reports whether the gate has been triggered.
func (g *abortGate) IsAborting() bool {
	return g.aborting.Load()
}

// Cause returns the AbortCause passed to the first trigger call, or "" if the
// gate has not been triggered yet.
func (g *abortGate) Cause() AbortCause {
	if !g.aborting.Load() {
		return ""
	}
	return g.causeOnce
}

// runCompleted is a post-run-window guard. It is set to true by the
// orchestrator immediately after workflow.Run returns on the happy path
// (completed or loop-broken). Failure detectors check this flag before firing
// the abort gate: if the run has already completed, no abort is needed.
//
// Contract:
//   - Set exactly once, by the orchestrator goroutine, after workflow.Run
//     returns with a non-abort ExitReason (wired in W-4).
//   - Read by failure detectors before calling abortGate.trigger (wired in W-4).
//   - Never reset.
type runCompleted struct {
	flag atomic.Bool
}

// set marks the run as completed. Idempotent; safe to call multiple times.
func (rc *runCompleted) set() { rc.flag.Store(true) }

// done reports whether the run has completed.
func (rc *runCompleted) done() bool { return rc.flag.Load() }
