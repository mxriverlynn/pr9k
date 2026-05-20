package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// sendStateHeaderAndWait is a helper that sends a StateHeader to the pane via
// the server, then sends WorkspaceDone and waits for DoneAck to ensure the
// message was processed before the test reads the output buffer.
func sendStateHeaderAndWait(t *testing.T, server *interactionchannel.Channel, hdr interactionchannel.StateHeader, buf *syncBuffer) {
	t.Helper()
	server.SendStateHeader(hdr)
	_ = server.Send(interactionchannel.WorkspaceDone{ExitCode: 0})
	if !waitForDoneAck(t, server, "header", 2*time.Second) {
		t.Fatal("header pane did not send DoneAck")
	}
	// After DoneAck the channel happens-before any buffer read is established.
	_ = buf
}

// sendStateLogAndWait sends a StateLog to the log pane, then WorkspaceDone,
// and waits for DoneAck to ensure the lines were written to the output buffer.
func sendStateLogAndWait(t *testing.T, server *interactionchannel.Channel, msg interactionchannel.StateLog, buf *syncBuffer) {
	t.Helper()
	server.SendStateLog(msg)
	_ = server.Send(interactionchannel.WorkspaceDone{ExitCode: 0})
	if !waitForDoneAck(t, server, "log", 2*time.Second) {
		t.Fatal("log pane did not send DoneAck")
	}
	_ = buf
}

// ---------------------------------------------------------------------------
// Header pane: StateHeader rendering
// ---------------------------------------------------------------------------

// TestDisplayPane_StateHeader_StepFailed_RendersX verifies that when the header
// pane receives a StateHeader with a StepFailed state at position 0, the
// rendered output contains "[✗]".
func TestDisplayPane_StateHeader_StepFailed_RendersX(t *testing.T) {
	socketPath := shortSockPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf syncBuffer
	server := startServerAndWaitForRole(t, ctx, socketPath, "header", func() {
		go func() {
			_ = runCmuxDisplayPaneWith(ctx, socketPath, "header", &buf)
		}()
	})
	defer server.Close()

	sendStateHeaderAndWait(t, server, interactionchannel.StateHeader{
		StepNames:  []string{"build"},
		StepStates: []int{int(ui.StepFailed)},
	}, &buf)

	if !strings.Contains(buf.String(), "[✗]") {
		t.Errorf("expected [✗] in header output for StepFailed, got: %q", buf.String())
	}
}

// TestDisplayPane_StateHeader_RetrySuccess_RendersCheck verifies that a
// StateHeader with StepDone produces "[✓]" in the output — the "subsequent
// success renders [✓]" behavior after a retry.
//
// StateHeader uses latest-wins delivery semantics; this test sends StepDone
// as the final state and verifies [✓] appears, which is the observable outcome
// the operator sees once a retry succeeds.
func TestDisplayPane_StateHeader_RetrySuccess_RendersCheck(t *testing.T) {
	socketPath := shortSockPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf syncBuffer
	server := startServerAndWaitForRole(t, ctx, socketPath, "header", func() {
		go func() {
			_ = runCmuxDisplayPaneWith(ctx, socketPath, "header", &buf)
		}()
	})
	defer server.Close()

	// Send the final StepDone state (as after a successful retry).
	sendStateHeaderAndWait(t, server, interactionchannel.StateHeader{
		StepNames:  []string{"build"},
		StepStates: []int{int(ui.StepDone)},
	}, &buf)

	if !strings.Contains(buf.String(), "[✓]") {
		t.Errorf("expected [✓] in header output for StepDone (retry success), got: %q", buf.String())
	}
}

// TestDisplayPane_StateHeader_IterationLine_IsRendered verifies that the
// iteration line from a StateHeader message appears in the output.
func TestDisplayPane_StateHeader_IterationLine_IsRendered(t *testing.T) {
	socketPath := shortSockPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf syncBuffer
	server := startServerAndWaitForRole(t, ctx, socketPath, "header", func() {
		go func() {
			_ = runCmuxDisplayPaneWith(ctx, socketPath, "header", &buf)
		}()
	})
	defer server.Close()

	sendStateHeaderAndWait(t, server, interactionchannel.StateHeader{
		IterationLine: "Iteration 1/3 — Issue #42",
		StepNames:     []string{"build"},
		StepStates:    []int{int(ui.StepPending)},
	}, &buf)

	if !strings.Contains(buf.String(), "Iteration 1/3 — Issue #42") {
		t.Errorf("expected iteration line in header output, got: %q", buf.String())
	}
}

// ---------------------------------------------------------------------------
// Log pane: StateLog rendering
// ---------------------------------------------------------------------------

// TestDisplayPane_StateLog_LinesInDeliveryOrder verifies that StateLog lines
// appear in the output in delivery order.
func TestDisplayPane_StateLog_LinesInDeliveryOrder(t *testing.T) {
	socketPath := shortSockPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf syncBuffer
	server := startServerAndWaitForRole(t, ctx, socketPath, "log", func() {
		go func() {
			_ = runCmuxDisplayPaneWith(ctx, socketPath, "log", &buf)
		}()
	})
	defer server.Close()

	sendStateLogAndWait(t, server, interactionchannel.StateLog{
		Lines: [][]byte{
			[]byte("first line"),
			[]byte("second line"),
			[]byte("third line"),
		},
	}, &buf)

	out := buf.String()
	firstIdx := strings.Index(out, "first line")
	secondIdx := strings.Index(out, "second line")
	thirdIdx := strings.Index(out, "third line")

	if firstIdx < 0 || secondIdx < 0 || thirdIdx < 0 {
		t.Errorf("not all lines found in log output: %q", out)
		return
	}
	if firstIdx >= secondIdx || secondIdx >= thirdIdx {
		t.Errorf("lines not in delivery order in log output: %q", out)
	}
}

// TestDisplayPane_StateLog_SeparatorBeforeOutput verifies that a separator line
// sent before output lines appears before those output lines in the rendered
// log (preserving the W-2 synchronous adapter ordering guarantee).
func TestDisplayPane_StateLog_SeparatorBeforeOutput(t *testing.T) {
	socketPath := shortSockPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf syncBuffer
	server := startServerAndWaitForRole(t, ctx, socketPath, "log", func() {
		go func() {
			_ = runCmuxDisplayPaneWith(ctx, socketPath, "log", &buf)
		}()
	})
	defer server.Close()

	sendStateLogAndWait(t, server, interactionchannel.StateLog{
		Lines: [][]byte{
			[]byte("── build (retry) ─────────────"),
			[]byte("retried output"),
		},
	}, &buf)

	out := buf.String()
	sepIdx := strings.Index(out, "── build (retry) ─────────────")
	outputIdx := strings.Index(out, "retried output")

	if sepIdx < 0 {
		t.Errorf("separator not found in log output: %q", out)
		return
	}
	if outputIdx < 0 {
		t.Errorf("output not found in log output: %q", out)
		return
	}
	if sepIdx >= outputIdx {
		t.Errorf("separator must appear before output, got sep@%d output@%d in: %q", sepIdx, outputIdx, out)
	}
}

// ---------------------------------------------------------------------------
// WorkspaceDone after state messages
// ---------------------------------------------------------------------------

// TestDisplayPane_WorkspaceDone_AfterStateMessages_SendsDoneAck verifies that
// the pane sends DoneAck and holds (does not return) even after receiving state
// messages followed by WorkspaceDone.
func TestDisplayPane_WorkspaceDone_AfterStateMessages_SendsDoneAck(t *testing.T) {
	socketPath := shortSockPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf syncBuffer
	paneErr := make(chan error, 1)
	server := startServerAndWaitForRole(t, ctx, socketPath, "header", func() {
		go func() {
			paneErr <- runCmuxDisplayPaneWith(ctx, socketPath, "header", &buf)
		}()
	})
	defer server.Close()

	// Send state messages first, then WorkspaceDone.
	server.SendStateHeader(interactionchannel.StateHeader{
		StepNames:  []string{"build", "test"},
		StepStates: []int{int(ui.StepDone), int(ui.StepFailed)},
	})
	_ = server.Send(interactionchannel.WorkspaceDone{ExitCode: 1})

	if !waitForDoneAck(t, server, "header", 2*time.Second) {
		t.Fatal("header pane did not send DoneAck after state messages + WorkspaceDone")
	}

	// Pane must NOT return before context cancel — it holds for final-state render.
	select {
	case err := <-paneErr:
		t.Errorf("pane returned before context cancel (should hold): %v", err)
	case <-time.After(150 * time.Millisecond):
		// Good — pane is still running.
	}
}

// ---------------------------------------------------------------------------
// Display-only: no key path
// ---------------------------------------------------------------------------

// TestDisplayPane_HeaderPane_HasNoKeyPath verifies that the header pane is
// display-only: it produces no intent even when the dispatch loop receives
// messages (there is no key-handling surface). This is verified structurally
// by confirming that runCmuxDisplayPaneWith sends only DoneAck (no Intent) to
// the server.
func TestDisplayPane_HeaderPane_HasNoKeyPath(t *testing.T) {
	socketPath := shortSockPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf syncBuffer
	server := startServerAndWaitForRole(t, ctx, socketPath, "header", func() {
		go func() {
			_ = runCmuxDisplayPaneWith(ctx, socketPath, "header", &buf)
		}()
	})
	defer server.Close()

	server.SendStateHeader(interactionchannel.StateHeader{
		StepNames:  []string{"build"},
		StepStates: []int{int(ui.StepFailed)},
	})
	_ = server.Send(interactionchannel.WorkspaceDone{ExitCode: 0})
	if !waitForDoneAck(t, server, "header", 2*time.Second) {
		t.Fatal("header pane did not send DoneAck")
	}

	// Drain any remaining messages and verify no Intent was sent.
	deadline := time.NewTimer(100 * time.Millisecond)
	defer deadline.Stop()
	for {
		select {
		case msg, ok := <-server.Recv():
			if !ok {
				return
			}
			if _, ok := msg.(interactionchannel.Intent); ok {
				t.Error("header pane sent an Intent — it must be display-only with no key path")
			}
		case <-deadline.C:
			return
		}
	}
}

// ---------------------------------------------------------------------------
// renderCmuxStateHeader unit tests (test the rendering function directly)
// ---------------------------------------------------------------------------

// TestRenderCmuxStateHeader_StepFailed_ContainsX verifies that
// renderCmuxStateHeader produces "[✗]" for StepFailed.
func TestRenderCmuxStateHeader_StepFailed_ContainsX(t *testing.T) {
	msg := interactionchannel.StateHeader{
		StepNames:  []string{"build"},
		StepStates: []int{int(ui.StepFailed)},
	}
	out := renderCmuxStateHeader(msg)
	if !strings.Contains(out, "[✗]") {
		t.Errorf("renderCmuxStateHeader: expected [✗] for StepFailed, got: %q", out)
	}
}

// TestRenderCmuxStateHeader_StepDone_ContainsCheck verifies that
// renderCmuxStateHeader produces "[✓]" for StepDone.
func TestRenderCmuxStateHeader_StepDone_ContainsCheck(t *testing.T) {
	msg := interactionchannel.StateHeader{
		StepNames:  []string{"build"},
		StepStates: []int{int(ui.StepDone)},
	}
	out := renderCmuxStateHeader(msg)
	if !strings.Contains(out, "[✓]") {
		t.Errorf("renderCmuxStateHeader: expected [✓] for StepDone, got: %q", out)
	}
}

// TestRenderCmuxStateHeader_StepPending_ContainsSpace verifies that
// renderCmuxStateHeader produces "[ ]" for StepPending.
func TestRenderCmuxStateHeader_StepPending_ContainsSpace(t *testing.T) {
	msg := interactionchannel.StateHeader{
		StepNames:  []string{"build"},
		StepStates: []int{int(ui.StepPending)},
	}
	out := renderCmuxStateHeader(msg)
	if !strings.Contains(out, "[ ]") {
		t.Errorf("renderCmuxStateHeader: expected [ ] for StepPending, got: %q", out)
	}
}

// TestRenderCmuxStateHeader_MultipleSteps_GridLayout verifies that multiple
// steps are rendered in HeaderCols-wide rows.
func TestRenderCmuxStateHeader_MultipleSteps_GridLayout(t *testing.T) {
	msg := interactionchannel.StateHeader{
		StepNames:  []string{"a", "b", "c", "d", "e"},
		StepStates: []int{int(ui.StepDone), int(ui.StepDone), int(ui.StepFailed), int(ui.StepPending), int(ui.StepPending)},
	}
	out := renderCmuxStateHeader(msg)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// 5 steps with HeaderCols=4 → 2 rows (4 on first, 1 on second)
	// plus the optional iteration line (empty here), so exactly 2 content rows.
	if len(lines) < 2 {
		t.Errorf("renderCmuxStateHeader: expected at least 2 rows for 5 steps, got %d rows in: %q", len(lines), out)
	}
	// Step "c" (index 2) is StepFailed — must appear in the output as [✗].
	if !strings.Contains(out, "[✗]") {
		t.Errorf("renderCmuxStateHeader: expected [✗] for step c (StepFailed), got: %q", out)
	}
}

// ---------------------------------------------------------------------------
// Cross-role guard tests (T-1, T-2)
// ---------------------------------------------------------------------------

// TestDisplayPane_LogPane_IgnoresStateHeader verifies that the log pane does not
// render header content when it receives a StateHeader message. The role guard in
// the dispatch loop must prevent header rendering in the log pane.
func TestDisplayPane_LogPane_IgnoresStateHeader(t *testing.T) {
	socketPath := shortSockPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf syncBuffer
	server := startServerAndWaitForRole(t, ctx, socketPath, "log", func() {
		go func() {
			_ = runCmuxDisplayPaneWith(ctx, socketPath, "log", &buf)
		}()
	})
	defer server.Close()

	// Send a StateHeader with a recognizable step name, then WorkspaceDone.
	server.SendStateHeader(interactionchannel.StateHeader{
		StepNames:  []string{"build-XYZ"},
		StepStates: []int{int(ui.StepFailed)},
	})
	_ = server.Send(interactionchannel.WorkspaceDone{ExitCode: 0})
	if !waitForDoneAck(t, server, "log", 2*time.Second) {
		t.Fatal("log pane did not send DoneAck")
	}

	out := buf.String()
	if strings.Contains(out, "build-XYZ") {
		t.Errorf("log pane rendered StateHeader step name — role guard is missing: %q", out)
	}
	if strings.Contains(out, "[✗]") || strings.Contains(out, "[✓]") {
		t.Errorf("log pane rendered header step markers — role guard is missing: %q", out)
	}
}

// TestDisplayPane_LogPane_HasNoKeyPath verifies that the log pane is
// display-only: it produces no Intent even when it receives a StateLog message.
// Spec D4 requires both display panes to be display-only.
func TestDisplayPane_LogPane_HasNoKeyPath(t *testing.T) {
	socketPath := shortSockPath(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf syncBuffer
	server := startServerAndWaitForRole(t, ctx, socketPath, "log", func() {
		go func() {
			_ = runCmuxDisplayPaneWith(ctx, socketPath, "log", &buf)
		}()
	})
	defer server.Close()

	server.SendStateLog(interactionchannel.StateLog{
		Lines: [][]byte{[]byte("some log output")},
	})
	_ = server.Send(interactionchannel.WorkspaceDone{ExitCode: 0})
	if !waitForDoneAck(t, server, "log", 2*time.Second) {
		t.Fatal("log pane did not send DoneAck")
	}

	// Drain any remaining messages and verify no Intent was sent.
	deadline := time.NewTimer(100 * time.Millisecond)
	defer deadline.Stop()
	for {
		select {
		case msg, ok := <-server.Recv():
			if !ok {
				return
			}
			if _, ok := msg.(interactionchannel.Intent); ok {
				t.Error("log pane sent an Intent — it must be display-only with no key path")
			}
		case <-deadline.C:
			return
		}
	}
}
