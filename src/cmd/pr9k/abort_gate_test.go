package main

import (
	"sync"
	"testing"
)

func TestAbortGate_IsAbortingFalseBeforeTrigger(t *testing.T) {
	var g abortGate
	if g.IsAborting() {
		t.Fatal("expected IsAborting() false before any trigger call")
	}
}

func TestAbortGate_CauseEmptyBeforeTrigger(t *testing.T) {
	var g abortGate
	if got := g.Cause(); got != "" {
		t.Fatalf("expected Cause() empty before trigger, got %q", got)
	}
}

func TestAbortGate_TriggerSetsAbortingAndCause(t *testing.T) {
	var g abortGate
	g.trigger(AbortCauseStallDetected)
	if !g.IsAborting() {
		t.Fatal("expected IsAborting() true after trigger")
	}
	if got := g.Cause(); got != AbortCauseStallDetected {
		t.Fatalf("expected cause %q, got %q", AbortCauseStallDetected, got)
	}
}

func TestAbortGate_FirstCauseWinsUnderConcurrency(t *testing.T) {
	const goroutines = 200

	for range 50 { // stress the race detector across multiple runs
		var g abortGate
		var wg sync.WaitGroup
		wg.Add(goroutines)
		for i := range goroutines {
			cause := AbortCauseStallDetected
			if i%2 == 0 {
				cause = AbortCauseDisplayPaneExit
			}
			go func(c AbortCause) {
				defer wg.Done()
				g.trigger(c)
			}(cause)
		}
		wg.Wait()

		if !g.IsAborting() {
			t.Fatal("expected IsAborting() true after concurrent triggers")
		}
		got := g.Cause()
		if got != AbortCauseStallDetected && got != AbortCauseDisplayPaneExit {
			t.Fatalf("unexpected cause %q", got)
		}
	}
}

func TestAbortGate_SubsequentTriggersAreNoOps(t *testing.T) {
	var g abortGate
	g.trigger(AbortCauseStallDetected)
	g.trigger(AbortCauseDisplayPaneExit) // must not overwrite
	if got := g.Cause(); got != AbortCauseStallDetected {
		t.Fatalf("expected first cause to win, got %q", got)
	}
}

func TestRunCompleted_FalseByDefault(t *testing.T) {
	var rc runCompleted
	if rc.done() {
		t.Fatal("expected done() false before set()")
	}
}

func TestRunCompleted_TrueAfterSet(t *testing.T) {
	var rc runCompleted
	rc.set()
	if !rc.done() {
		t.Fatal("expected done() true after set()")
	}
}

func TestRunCompleted_IdempotentSet(t *testing.T) {
	var rc runCompleted
	rc.set()
	rc.set() // must not panic or reset
	if !rc.done() {
		t.Fatal("expected done() true after multiple set() calls")
	}
}
