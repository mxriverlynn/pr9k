// Polling helpers in this file (waitForOutputFW, drainIntentFromSent) are
// poll-with-condition loops with bounded timeouts, not blind sleeps.
// Standard timeout budget: 1s for render assertions, 2s for goroutine-exit
// assertions, with 10ms poll interval. This keeps the test suite responsive
// while accommodating CI scheduler jitter.
package main

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// syncBuffer is a concurrency-safe bytes.Buffer. The footer machine runs in a
// goroutine that writes to the shared output buffer while the test's polling
// helpers read it; an unsynchronised bytes.Buffer is a data race under -race.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// waitForOutputFW polls out.String() until pred is true or timeout elapses.
func waitForOutputFW(timeout, interval time.Duration, out *syncBuffer, pred func(s string) bool) bool {
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
// A nil stalledCh is passed so the stall arm never fires in non-W5 tests.
func runMachineFixture(
	t *testing.T,
	src FooterKeySource,
	ch footerPaneCh,
	out *syncBuffer,
) (cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		runCmuxFooterMachineWith(ctx, src, ch, out, nil)
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
	var out syncBuffer

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
	var out syncBuffer

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
	var out syncBuffer

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
// T-4: q→esc produces no IntentQuit and restores the error-mode shortcut render.
// IntentErrorQuitInitiated (on q) and IntentErrorQuitCancelled (on esc) are
// expected (W3); no quit is committed until 'y' is pressed.
// ---------------------------------------------------------------------------

func TestFooterMachineWith_QEsc_ProducesNoIntentQuit(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	fch := interactionchannel.NewFakeInteractionChannel()
	var out syncBuffer

	cancel, done := runMachineFixture(t, src, fch, &out)
	defer cancel()

	// Pre-seed ModeError so the machine starts in error mode.
	fch.InjectMessage(interactionchannel.StateFooter{
		Mode:         int(ui.ModeError),
		ShortcutLine: ui.ErrorShortcuts,
	})

	// Wait for error mode to be rendered before pressing keys.
	if !waitForOutputFW(time.Second, 10*time.Millisecond, &out, func(s string) bool {
		return strings.Contains(s, ui.ErrorShortcuts)
	}) {
		t.Fatal("error-mode render did not appear before key presses")
	}

	src.Press("q")
	src.Press("esc")

	// Give the machine time to process both keys.
	time.Sleep(100 * time.Millisecond)

	// No IntentQuit must be forwarded — quit requires 'y' confirmation.
	for _, msg := range fch.SentMessages() {
		if intent, ok := msg.(interactionchannel.Intent); ok {
			if intent.Kind == interactionchannel.IntentQuit {
				t.Error("expected no IntentQuit after q+esc (quit requires y confirmation), but one was forwarded")
			}
		}
	}

	// B: cancel must restore the error-mode shortcut render.
	if !waitForOutputFW(time.Second, 10*time.Millisecond, &out, func(s string) bool {
		// ErrorShortcuts must appear after any QuitConfirmPrompt occurrence.
		eIdx := strings.LastIndex(s, ui.ErrorShortcuts)
		qIdx := strings.LastIndex(s, ui.QuitConfirmPrompt)
		return eIdx != -1 && eIdx > qIdx
	}) {
		t.Errorf("expected ErrorShortcuts to be the last shortcut line after q+esc; output:\n%q", out.String())
	}

	cancel()
	waitDone(t, done)
}

// ---------------------------------------------------------------------------
// T-A: non-confirmation key in quit-confirm is ignored and not buffered
// ---------------------------------------------------------------------------

func TestFooterMachineWith_QXEsc_NonConfirmKeyIgnoredNotBuffered(t *testing.T) {
	src := interactionchannel.NewFakeFooterKeySource()
	fch := interactionchannel.NewFakeInteractionChannel()
	var out syncBuffer

	cancel, done := runMachineFixture(t, src, fch, &out)
	defer cancel()

	// Pre-seed ModeError so the machine starts in error mode.
	fch.InjectMessage(interactionchannel.StateFooter{
		Mode:         int(ui.ModeError),
		ShortcutLine: ui.ErrorShortcuts,
	})

	// Wait for error mode to be rendered before pressing keys.
	if !waitForOutputFW(time.Second, 10*time.Millisecond, &out, func(s string) bool {
		return strings.Contains(s, ui.ErrorShortcuts)
	}) {
		t.Fatal("error-mode render did not appear before key presses")
	}

	src.Press("q")   // enter ModeQuitConfirm; emits IntentErrorQuitInitiated
	src.Press("x")   // non-confirmation key — must be dropped, not buffered
	src.Press("esc") // cancel quit, return to ModeError; emits IntentErrorQuitCancelled

	// Give the machine time to process all keys.
	time.Sleep(100 * time.Millisecond)

	// No IntentQuit must have been forwarded — quit requires 'y' confirmation.
	// IntentErrorQuitInitiated and IntentErrorQuitCancelled are expected (W3).
	for _, msg := range fch.SentMessages() {
		if intent, ok := msg.(interactionchannel.Intent); ok {
			if intent.Kind == interactionchannel.IntentQuit {
				t.Error("expected no IntentQuit after q+x+esc (quit requires y confirmation), but one was forwarded")
			}
		}
	}

	// The 'x' key must not have been replayed into ModeError — the last
	// shortcut render must be ErrorShortcuts, not something triggered by 'x'.
	if !waitForOutputFW(time.Second, 10*time.Millisecond, &out, func(s string) bool {
		eIdx := strings.LastIndex(s, ui.ErrorShortcuts)
		qIdx := strings.LastIndex(s, ui.QuitConfirmPrompt)
		return eIdx != -1 && eIdx > qIdx
	}) {
		t.Errorf("expected ErrorShortcuts after q+x+esc; output:\n%q", out.String())
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
	var out syncBuffer

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
	var out syncBuffer

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
	out := r.Render("", "")
	if out == "" {
		t.Error("expected non-empty render output")
	}
}
