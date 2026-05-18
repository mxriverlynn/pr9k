package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// waitForOutputFW polls out.String() until pred is true or timeout elapses.
func waitForOutputFW(timeout, interval time.Duration, out *bytes.Buffer, pred func(s string) bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pred(out.String()) {
			return true
		}
		time.Sleep(interval)
	}
	return pred(out.String())
}

// drainIntentFromSent scans fch.SentMessages() for the first Intent and
// returns (kind, true) or (zero, false) if none found.
func drainIntentFromSent(fch *interactionchannel.FakeInteractionChannel) (interactionchannel.IntentType, bool) {
	for _, msg := range fch.SentMessages() {
		if intent, ok := msg.(interactionchannel.Intent); ok {
			return intent.Kind, true
		}
	}
	return "", false
}

// runMachineFixture starts runCmuxFooterMachineWith in a goroutine and returns
// a cancel func and done channel. The caller must call cancel at end of test.
func runMachineFixture(
	t *testing.T,
	src FooterKeySource,
	ch footerPaneCh,
	out *bytes.Buffer,
) (cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		runCmuxFooterMachineWith(ctx, src, ch, out)
	}()
	return cancel, doneCh
}

// waitDone blocks until done closes or the 2-second deadline fires.
func waitDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runCmuxFooterMachineWith did not return after context cancel")
	}
}

// ---------------------------------------------------------------------------
// T-1: StateFooter(ModeNormal) renders normal-mode hints in output
// ---------------------------------------------------------------------------

func TestFooterMachineWith_StateFooterNormal_RendersNormalHints(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	fch := interactionchannel.NewFakeInteractionChannel()
	var out bytes.Buffer

	cancel, done := runMachineFixture(t, src, fch, &out)
	defer cancel()

	fch.InjectMessage(interactionchannel.StateFooter{
		Mode:         int(ui.ModeNormal),
		ShortcutLine: ui.NormalShortcuts,
	})

	if !waitForOutputFW(time.Second, 10*time.Millisecond, &out, func(s string) bool {
		return strings.Contains(s, "quit")
	}) {
		t.Errorf("expected normal-mode hints in output; got: %q", out.String())
	}

	cancel()
	waitDone(t, done)
}

// ---------------------------------------------------------------------------
// T-2: StateFooter(ModeError) renders error-mode hints in output
// ---------------------------------------------------------------------------

func TestFooterMachineWith_StateFooterError_RendersErrorHints(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	fch := interactionchannel.NewFakeInteractionChannel()
	var out bytes.Buffer

	cancel, done := runMachineFixture(t, src, fch, &out)
	defer cancel()

	fch.InjectMessage(interactionchannel.StateFooter{
		Mode:         int(ui.ModeError),
		ShortcutLine: ui.ErrorShortcuts,
	})

	if !waitForOutputFW(time.Second, 10*time.Millisecond, &out, func(s string) bool {
		return strings.Contains(s, "retry")
	}) {
		t.Errorf("expected error-mode hints in output; got: %q", out.String())
	}

	cancel()
	waitDone(t, done)
}

// ---------------------------------------------------------------------------
// T-3: q→y produces exactly one IntentQuit forwarded to the channel
// ---------------------------------------------------------------------------

func TestFooterMachineWith_QY_ForwardsIntentQuit(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	fch := interactionchannel.NewFakeInteractionChannel()
	var out bytes.Buffer

	src.Press("q")
	src.Press("y")

	cancel, done := runMachineFixture(t, src, fch, &out)
	defer cancel()

	// Wait for the Intent to appear in sent messages.
	var gotKind interactionchannel.IntentType
	deadline := time.Now().Add(2 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		if k, ok := drainIntentFromSent(fch); ok {
			gotKind = k
			found = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !found {
		t.Fatal("expected IntentQuit to be forwarded, got none")
	}
	if gotKind != interactionchannel.IntentQuit {
		t.Errorf("expected IntentQuit, got %q", gotKind)
	}

	cancel()
	waitDone(t, done)
}

// ---------------------------------------------------------------------------
// T-4: q→esc produces no intent (quit cancelled locally)
// ---------------------------------------------------------------------------

func TestFooterMachineWith_QEsc_ProducesNoIntent(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	fch := interactionchannel.NewFakeInteractionChannel()
	var out bytes.Buffer

	src.Press("q")
	src.Press("esc")

	cancel, done := runMachineFixture(t, src, fch, &out)
	defer cancel()

	// Give the machine time to process both keys.
	time.Sleep(100 * time.Millisecond)

	if _, ok := drainIntentFromSent(fch); ok {
		t.Error("expected no intent after q+esc, but one was forwarded")
	}

	cancel()
	waitDone(t, done)
}

// ---------------------------------------------------------------------------
// T-5: Goroutine exits cleanly when context is cancelled (no leak)
// ---------------------------------------------------------------------------

func TestFooterMachineWith_ContextCancel_GoRoutineExits(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	fch := interactionchannel.NewFakeInteractionChannel()
	var out bytes.Buffer

	cancel, done := runMachineFixture(t, src, fch, &out)
	cancel()
	waitDone(t, done)
}

// ---------------------------------------------------------------------------
// T-6: WorkspaceDone causes DoneAck{Role:"footer"} to be sent
// ---------------------------------------------------------------------------

func TestFooterMachineWith_WorkspaceDone_SendsDoneAck(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	fch := interactionchannel.NewFakeInteractionChannel()
	var out bytes.Buffer

	cancel, done := runMachineFixture(t, src, fch, &out)
	defer cancel()

	fch.InjectMessage(interactionchannel.WorkspaceDone{ExitCode: 0})

	// Wait for DoneAck.
	deadline := time.Now().Add(2 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		for _, msg := range fch.SentMessages() {
			if ack, ok := msg.(interactionchannel.DoneAck); ok && ack.Role == "footer" {
				found = true
				break
			}
		}
		if found {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !found {
		t.Fatal("expected DoneAck{Role:footer} to be sent after WorkspaceDone")
	}

	cancel()
	waitDone(t, done)
}

// ---------------------------------------------------------------------------
// T-7: Renderer works without isolated KeyHandler (compile-time check)
// ---------------------------------------------------------------------------

func TestFooterRenderer_NoIsolatedKeyHandler(t *testing.T) {
	r := newCmuxFooterRenderer()
	r.SetSize(80, 24)
	out := r.Render("")
	if out == "" {
		t.Error("expected non-empty render output")
	}
}
