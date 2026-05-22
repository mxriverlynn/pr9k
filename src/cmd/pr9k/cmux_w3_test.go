// W3 unit tests: IntentErrorQuitInitiated / IntentErrorQuitCancelled emit sites
// in the footer state machine.
//
// These tests use footerStateMachine directly (not runCmuxFooterMachineWith) with a
// local recordingSink that captures RecordIntent and SetMode events in arrival order.
// This lets the ordering invariant ("intent emitted before mode transition") be asserted
// without coupling to the wired channel or render output.
package main

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// ---------------------------------------------------------------------------
// recordingSink — ordered event log for footerStateMachine unit tests
// ---------------------------------------------------------------------------

type sinkEventKind string

const (
	sinkEventIntent sinkEventKind = "intent"
	sinkEventMode   sinkEventKind = "mode"
)

type sinkEvent struct {
	kind   sinkEventKind
	intent interactionchannel.IntentType
	mode   ui.Mode
}

type recordingSink struct {
	mu     sync.Mutex
	events []sinkEvent
}

func (r *recordingSink) SetMode(mode int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, sinkEvent{kind: sinkEventMode, mode: ui.Mode(mode)})
}

func (r *recordingSink) RecordIntent(kind interactionchannel.IntentType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, sinkEvent{kind: sinkEventIntent, intent: kind})
}

// Events returns a snapshot of all recorded events in arrival order.
func (r *recordingSink) Events() []sinkEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]sinkEvent, len(r.events))
	copy(out, r.events)
	return out
}

// ---------------------------------------------------------------------------
// T-W3-1: q in ModeError emits IntentErrorQuitInitiated before ModeQuitConfirm
// ---------------------------------------------------------------------------

func TestW3_ErrorModeQ_EmitsInitiatedBeforeModeQuitConfirm(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	sink := &recordingSink{}
	m := newFooterStateMachine(src, sink)

	// Drive the machine into ModeError via external push.
	m.SetMode(ui.ModeError)

	// Press 'q' and run one step.
	src.Press("q")
	m.Step()

	events := sink.Events()

	// Find the first IntentErrorQuitInitiated and the first ModeQuitConfirm.
	initiatedIdx := -1
	quitConfirmIdx := -1
	for i, e := range events {
		if e.kind == sinkEventIntent && e.intent == interactionchannel.IntentErrorQuitInitiated && initiatedIdx < 0 {
			initiatedIdx = i
		}
		if e.kind == sinkEventMode && e.mode == ui.ModeQuitConfirm && quitConfirmIdx < 0 {
			quitConfirmIdx = i
		}
	}

	if initiatedIdx < 0 {
		t.Fatal("IntentErrorQuitInitiated was not emitted")
	}
	if quitConfirmIdx < 0 {
		t.Fatal("ModeQuitConfirm was not reached after q in ModeError")
	}
	if initiatedIdx >= quitConfirmIdx {
		t.Errorf("ordering violated: IntentErrorQuitInitiated (idx %d) must appear before ModeQuitConfirm (idx %d)",
			initiatedIdx, quitConfirmIdx)
	}
}

// ---------------------------------------------------------------------------
// T-W3-2: n in ModeQuitConfirm (prev=ModeError) emits IntentErrorQuitCancelled
// ---------------------------------------------------------------------------

func TestW3_QuitConfirmN_PrevError_EmitsCancelled(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	sink := &recordingSink{}
	m := newFooterStateMachine(src, sink)

	m.SetMode(ui.ModeError)
	src.Press("q")
	m.Step() // transitions to ModeQuitConfirm

	src.Press("n")
	m.Step() // should emit IntentErrorQuitCancelled and restore ModeError

	events := sink.Events()
	found := false
	for _, e := range events {
		if e.kind == sinkEventIntent && e.intent == interactionchannel.IntentErrorQuitCancelled {
			found = true
			break
		}
	}
	if !found {
		t.Error("IntentErrorQuitCancelled was not emitted when n pressed in QuitConfirm (prev=ModeError)")
	}

	// Mode must be restored to ModeError.
	lastMode := ui.ModeNormal
	for _, e := range events {
		if e.kind == sinkEventMode {
			lastMode = e.mode
		}
	}
	if lastMode != ui.ModeError {
		t.Errorf("expected ModeError restored after n, got %v", lastMode)
	}
}

// ---------------------------------------------------------------------------
// T-W3-3: esc in ModeQuitConfirm (prev=ModeError) emits IntentErrorQuitCancelled
// ---------------------------------------------------------------------------

func TestW3_QuitConfirmEsc_PrevError_EmitsCancelled(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	sink := &recordingSink{}
	m := newFooterStateMachine(src, sink)

	m.SetMode(ui.ModeError)
	src.Press("q")
	m.Step()

	src.Press("esc")
	m.Step()

	found := false
	for _, e := range sink.Events() {
		if e.kind == sinkEventIntent && e.intent == interactionchannel.IntentErrorQuitCancelled {
			found = true
			break
		}
	}
	if !found {
		t.Error("IntentErrorQuitCancelled was not emitted when esc pressed in QuitConfirm (prev=ModeError)")
	}
}

// ---------------------------------------------------------------------------
// T-W3-4: n in ModeQuitConfirm (prev=ModeNormal) does NOT emit IntentErrorQuitCancelled
// ---------------------------------------------------------------------------

func TestW3_QuitConfirmN_PrevNormal_NoCancelled(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	sink := &recordingSink{}
	m := newFooterStateMachine(src, sink)

	// ModeNormal → q → ModeQuitConfirm → n
	src.Press("q")
	m.Step()
	src.Press("n")
	m.Step()

	for _, e := range sink.Events() {
		if e.kind == sinkEventIntent && e.intent == interactionchannel.IntentErrorQuitCancelled {
			t.Error("IntentErrorQuitCancelled must NOT be emitted when prev mode was ModeNormal")
		}
	}
}

// ---------------------------------------------------------------------------
// T-W3-5: esc in ModeQuitConfirm (prev=ModeNormal) does NOT emit IntentErrorQuitCancelled
// ---------------------------------------------------------------------------

func TestW3_QuitConfirmEsc_PrevNormal_NoCancelled(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	sink := &recordingSink{}
	m := newFooterStateMachine(src, sink)

	src.Press("q")
	m.Step()
	src.Press("esc")
	m.Step()

	for _, e := range sink.Events() {
		if e.kind == sinkEventIntent && e.intent == interactionchannel.IntentErrorQuitCancelled {
			t.Error("IntentErrorQuitCancelled must NOT be emitted when prev mode was ModeNormal")
		}
	}
}

// ---------------------------------------------------------------------------
// T-W3-6: new intent types serialize / deserialize correctly via JSON discriminator
// ---------------------------------------------------------------------------

func TestW3_NewIntentTypes_JSONRoundTrip(t *testing.T) {
	cases := []struct {
		kind interactionchannel.IntentType
		want string
	}{
		{interactionchannel.IntentErrorQuitInitiated, `"error_quit_initiated"`},
		{interactionchannel.IntentErrorQuitCancelled, `"error_quit_cancelled"`},
	}
	for _, tc := range cases {
		b, err := json.Marshal(tc.kind)
		if err != nil {
			t.Fatalf("marshal %q: %v", tc.kind, err)
		}
		if string(b) != tc.want {
			t.Errorf("marshal %q: got %s, want %s", tc.kind, b, tc.want)
		}
		var got interactionchannel.IntentType
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal %q: %v", tc.want, err)
		}
		if got != tc.kind {
			t.Errorf("round-trip %q: got %q", tc.kind, got)
		}
	}
}

// ---------------------------------------------------------------------------
// T-W3-7: FakeFooterKeySource.ForwardedIntents records all intents in order
// ---------------------------------------------------------------------------

func TestW3_FakeFooterKeySource_ForwardedIntents_TracksAll(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	src.RecordIntent(interactionchannel.IntentErrorQuitInitiated)
	src.RecordIntent(interactionchannel.IntentErrorQuitCancelled)
	src.RecordIntent(interactionchannel.IntentQuit)

	all := src.ForwardedIntents()
	if len(all) != 3 {
		t.Fatalf("expected 3 forwarded intents, got %d", len(all))
	}
	if all[0] != interactionchannel.IntentErrorQuitInitiated {
		t.Errorf("[0]: want IntentErrorQuitInitiated, got %q", all[0])
	}
	if all[1] != interactionchannel.IntentErrorQuitCancelled {
		t.Errorf("[1]: want IntentErrorQuitCancelled, got %q", all[1])
	}
	if all[2] != interactionchannel.IntentQuit {
		t.Errorf("[2]: want IntentQuit, got %q", all[2])
	}
}
