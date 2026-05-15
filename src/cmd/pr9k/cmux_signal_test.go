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
	runCmuxSignalHandler(sigCh, &once, teardownFn, setShuttingDown, func(int) {})

	sendSignal(sigCh, syscall.SIGINT)

	select {
	case <-teardownDone:
	case <-time.After(time.Second):
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

	var once sync.Once
	runCmuxSignalHandler(sigCh, &once, teardownFn, func() {}, exitFn)

	// First signal: triggers cleanup goroutine.
	sendSignal(sigCh, syscall.SIGTERM)
	// Wait for cleanup goroutine to consume first signal and open the watchdog gate.
	time.Sleep(20 * time.Millisecond)
	// Second signal: watchdog fires exitFn(1).
	sendSignal(sigCh, syscall.SIGINT)

	select {
	case code := <-exitCalled:
		if code != 1 {
			t.Errorf("exitFn called with code %d, want 1", code)
		}
	case <-time.After(time.Second):
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
	runCmuxSignalHandler(sigCh, &once, teardownFn, func() {}, func(int) {})

	sendSignal(sigCh, syscall.SIGHUP)

	select {
	case <-teardownDone:
	case <-time.After(time.Second):
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
	runCmuxSignalHandler(sigCh, &once, teardownFn, func() {}, func(int) {})

	sendSignal(sigCh, syscall.SIGTERM)

	select {
	case <-teardownDone:
	case <-time.After(time.Second):
		t.Fatal("teardown not called within 1s after SIGTERM")
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
	runCmuxSignalHandler(sigCh, &once, teardownFn, func() {}, func(int) {})

	sendSignal(sigCh, syscall.SIGINT)

	select {
	case <-firstDone:
	case <-time.After(time.Second):
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
