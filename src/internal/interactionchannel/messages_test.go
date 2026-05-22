package interactionchannel

import (
	"testing"
)

// TestLivenessRoundTrip verifies Liveness{} serializes and deserializes correctly.
func TestLivenessRoundTrip(t *testing.T) {
	t.Parallel()

	msg := Liveness{}
	data, err := MarshalMessage(msg)
	if err != nil {
		t.Fatalf("MarshalMessage(Liveness{}): %v", err)
	}
	got, err := UnmarshalMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalMessage(liveness): %v", err)
	}
	if _, ok := got.(Liveness); !ok {
		t.Fatalf("UnmarshalMessage returned %T, want Liveness", got)
	}
	if got.WireType() != "liveness" {
		t.Fatalf("WireType() = %q, want %q", got.WireType(), "liveness")
	}
}

// TestWorkspaceDoneAbortedTrue verifies WorkspaceDone{Aborted: true} round-trips correctly.
func TestWorkspaceDoneAbortedTrue(t *testing.T) {
	t.Parallel()

	msg := WorkspaceDone{ExitCode: 1, Aborted: true}
	data, err := MarshalMessage(msg)
	if err != nil {
		t.Fatalf("MarshalMessage: %v", err)
	}
	got, err := UnmarshalMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalMessage: %v", err)
	}
	wd, ok := got.(WorkspaceDone)
	if !ok {
		t.Fatalf("UnmarshalMessage returned %T, want WorkspaceDone", got)
	}
	if !wd.Aborted {
		t.Fatal("WorkspaceDone.Aborted: got false, want true")
	}
	if wd.ExitCode != 1 {
		t.Fatalf("WorkspaceDone.ExitCode: got %d, want 1", wd.ExitCode)
	}
}

// TestWorkspaceDoneAbortedZeroValue verifies a zero-value WorkspaceDone decodes Aborted as false.
func TestWorkspaceDoneAbortedZeroValue(t *testing.T) {
	t.Parallel()

	msg := WorkspaceDone{}
	data, err := MarshalMessage(msg)
	if err != nil {
		t.Fatalf("MarshalMessage: %v", err)
	}
	got, err := UnmarshalMessage(data)
	if err != nil {
		t.Fatalf("UnmarshalMessage: %v", err)
	}
	wd, ok := got.(WorkspaceDone)
	if !ok {
		t.Fatalf("UnmarshalMessage returned %T, want WorkspaceDone", got)
	}
	if wd.Aborted {
		t.Fatal("WorkspaceDone.Aborted: got true, want false for zero value")
	}
}

// TestRunAbortedToken verifies the exported constant has the correct value.
func TestRunAbortedToken(t *testing.T) {
	t.Parallel()

	const want = "run aborted"
	if RunAbortedToken != want {
		t.Fatalf("RunAbortedToken = %q, want %q", RunAbortedToken, want)
	}
}
