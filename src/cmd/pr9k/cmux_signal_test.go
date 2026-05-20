package main

import (
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

// sendSignal sends sig into ch without blocking (ch must have capacity).
func sendSignal(ch chan<- os.Signal, sig os.Signal) {
	ch <- sig
}

// TP-223-001: first signal → shuttingDown set AND teardown invoked.
func TestRunCmuxSignalHandler_FirstSignalSetsShuttingDownAndCallsTeardown(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)

	var mu sync.Mutex
	shuttingDown := false
	teardownCalled := false

	setShuttingDown := func() {
		mu.Lock()
		shuttingDown = true
		mu.Unlock()
	}

	teardownDone := make(chan struct{})
	teardownFn := func() {
		mu.Lock()
		teardownCalled = true
		mu.Unlock()
		close(teardownDone)
	}

	var once sync.Once
	done := make(chan struct{})
	defer close(done)
	runCmuxSignalHandler(sigCh, done, &once, teardownFn, setShuttingDown, func(int) {})

	sendSignal(sigCh, syscall.SIGINT)

	select {
	case <-teardownDone:
	case <-time.After(10 * time.Second):
		t.Fatal("teardown not called within 1s after first signal")
	}

	mu.Lock()
	sd := shuttingDown
	tc := teardownCalled
	mu.Unlock()

	if !sd {
		t.Error("shuttingDown not set after first signal")
	}
	if !tc {
		t.Error("teardownFn not called after first signal")
	}
}

// TP-223-002: second signal during teardown → watchdog calls exitFn(1),
// regardless of whether teardown has returned.
func TestRunCmuxSignalHandler_SecondSignalCallsExitFn(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)

	exitCalled := make(chan int, 1)
	exitFn := func(code int) { exitCalled <- code }

	teardownBlock := make(chan struct{})
	teardownFn := func() { <-teardownBlock } // block to simulate in-progress RPC

	// cleanupStarted is closed by setShuttingDown so we know the cleanup goroutine
	// has consumed the first signal and opened the watchdog gate before we send the
	// second signal — deterministic replacement for time.Sleep.
	cleanupStarted := make(chan struct{})
	setShuttingDown := func() { close(cleanupStarted) }

	var once sync.Once
	done := make(chan struct{})
	defer close(done)
	runCmuxSignalHandler(sigCh, done, &once, teardownFn, setShuttingDown, exitFn)

	// First signal: triggers cleanup goroutine.
	sendSignal(sigCh, syscall.SIGTERM)
	// Wait until cleanup goroutine has consumed the first signal and opened the gate.
	select {
	case <-cleanupStarted:
	case <-time.After(10 * time.Second):
		t.Fatal("setShuttingDown not called within 1s after first signal")
	}
	// Second signal: watchdog fires exitFn(1).
	sendSignal(sigCh, syscall.SIGINT)

	select {
	case code := <-exitCalled:
		if code != 1 {
			t.Errorf("exitFn called with code %d, want 1", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("exitFn not called within 1s after second signal")
	}
	close(teardownBlock) // unblock cleanup goroutine so the test goroutine can exit
}

// TP-223-003: SIGHUP triggers the same first-signal (cleanup) path.
func TestRunCmuxSignalHandler_SIGHUPTriggersTeardown(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)

	teardownDone := make(chan struct{})
	teardownFn := func() { close(teardownDone) }

	var once sync.Once
	done := make(chan struct{})
	defer close(done)
	runCmuxSignalHandler(sigCh, done, &once, teardownFn, func() {}, func(int) {})

	sendSignal(sigCh, syscall.SIGHUP)

	select {
	case <-teardownDone:
	case <-time.After(10 * time.Second):
		t.Fatal("teardown not called within 1s after SIGHUP")
	}
}

// TP-223-004: SIGTERM triggers the same first-signal (cleanup) path.
func TestRunCmuxSignalHandler_SIGTERMTriggersTeardown(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)

	teardownDone := make(chan struct{})
	teardownFn := func() { close(teardownDone) }

	var once sync.Once
	done := make(chan struct{})
	defer close(done)
	runCmuxSignalHandler(sigCh, done, &once, teardownFn, func() {}, func(int) {})

	sendSignal(sigCh, syscall.SIGTERM)

	select {
	case <-teardownDone:
	case <-time.After(10 * time.Second):
		t.Fatal("teardown not called within 1s after SIGTERM")
	}
}

// TP-223-006: single signal must NOT call exitFn — first signal is graceful only.
// This pins the "first signal ⇒ zero exitFn calls" invariant so that a regression
// in the started-gate ordering (watchdog winning the first signal) would be caught.
func TestRunCmuxSignalHandler_SingleSignalDoesNotCallExitFn(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)

	exitCalled := make(chan int, 1)
	exitFn := func(code int) { exitCalled <- code }

	teardownDone := make(chan struct{})
	teardownFn := func() { close(teardownDone) }

	shuttingDownSet := make(chan struct{})
	setShuttingDown := func() { close(shuttingDownSet) }

	var once sync.Once
	done := make(chan struct{})
	defer close(done)
	runCmuxSignalHandler(sigCh, done, &once, teardownFn, setShuttingDown, exitFn)

	sendSignal(sigCh, syscall.SIGTERM)

	// Wait for the cleanup goroutine to complete teardown.
	select {
	case <-teardownDone:
	case <-time.After(10 * time.Second):
		t.Fatal("teardownFn not called within 1s after single signal")
	}

	// shuttingDown must be set.
	select {
	case <-shuttingDownSet:
	default:
		t.Error("shuttingDown not set after first signal")
	}

	// exitFn must NOT have been called.
	select {
	case code := <-exitCalled:
		t.Errorf("exitFn called with code %d after only one signal — watchdog must not fire on first signal", code)
	default:
		// correct: no exit after a single signal
	}
}

// TP-223-005: sync.Once semantics — teardown invoked exactly once even when
// both the signal-triggered path and an external caller fire teardownOnce.Do.
func TestRunCmuxSignalHandler_SyncOnce_TeardownCalledExactlyOnce(t *testing.T) {
	t.Parallel()

	sigCh := make(chan os.Signal, 2)

	var mu sync.Mutex
	count := 0
	firstDone := make(chan struct{})
	teardownFn := func() {
		mu.Lock()
		count++
		mu.Unlock()
		// Signal the first call; subsequent calls will not reach here.
		select {
		case firstDone <- struct{}{}:
		default:
		}
	}

	var once sync.Once
	done := make(chan struct{})
	defer close(done)
	runCmuxSignalHandler(sigCh, done, &once, teardownFn, func() {}, func(int) {})

	sendSignal(sigCh, syscall.SIGINT)

	select {
	case <-firstDone:
	case <-time.After(10 * time.Second):
		t.Fatal("teardown not called within 1s")
	}

	// External caller (simulates dismissal-channel path firing concurrently).
	once.Do(teardownFn)

	mu.Lock()
	c := count
	mu.Unlock()

	if c != 1 {
		t.Errorf("teardownFn called %d times, want exactly 1 (sync.Once violated)", c)
	}
}
