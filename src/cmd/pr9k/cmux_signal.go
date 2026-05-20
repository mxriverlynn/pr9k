package main

import (
	"os"
	"sync"
)

// runCmuxSignalHandler starts the two-goroutine watchdog+cleanup signal handler
// for cmux mode per spec D-9 (issue #223).
//
// The cleanup goroutine receives the first signal from sigCh, calls
// setShuttingDown (mutex-protected per concurrency standards), closes an
// internal "started" gate, then invokes teardownOnce.Do(teardownFn).
//
// The watchdog goroutine waits on the "started" gate (ensuring cleanup has
// consumed the first signal), then waits for a second signal and immediately
// calls exitFn(1) regardless of whether the cleanup goroutine has returned.
//
// done is closed by the caller when cmux mode exits normally. Both goroutines
// select on done so they exit cleanly when no signal was received, rather than
// leaking blocked goroutines if the caller returns without terminating the process.
//
// exitFn replaces os.Exit and is injectable for testing per api-design.md.
// setShuttingDown is injectable so tests can verify the flag transitions
// independently from teardownFn execution.
func runCmuxSignalHandler(
	sigCh <-chan os.Signal,
	done <-chan struct{},
	teardownOnce *sync.Once,
	teardownFn func(),
	setShuttingDown func(),
	exitFn func(int),
) {
	started := make(chan struct{})

	// Cleanup goroutine: receives first signal, sets shuttingDown, then runs teardown.
	// The teardown MAY block on cmux RPCs (each bounded by its own per-call timeout).
	go func() {
		select {
		case <-sigCh:
			setShuttingDown()
			close(started)
			teardownOnce.Do(teardownFn)
		case <-done:
		}
	}()

	// Watchdog goroutine: waits until cleanup has consumed the first signal,
	// then waits for the second signal and exits immediately (spec D-9).
	go func() {
		select {
		case <-started:
		case <-done:
			return
		}
		select {
		case <-sigCh:
			exitFn(1)
		case <-done:
		}
	}()
}
