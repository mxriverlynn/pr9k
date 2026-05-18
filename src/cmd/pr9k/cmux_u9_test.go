package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
)

// ---------------------------------------------------------------------------
// awaitDoneAcks unit tests
// ---------------------------------------------------------------------------

// TestAwaitDoneAcks_CollectsThreeAcks verifies that awaitDoneAcks returns
// promptly when all three distinct display roles send DoneAck.
func TestAwaitDoneAcks_CollectsThreeAcks(t *testing.T) {
	recv := make(chan interactionchannel.Message, 10)
	recv <- interactionchannel.DoneAck{Role: "header"}
	recv <- interactionchannel.DoneAck{Role: "log"}
	recv <- interactionchannel.DoneAck{Role: "footer"}

	start := time.Now()
	awaitDoneAcks(recv, 5*time.Second)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("awaitDoneAcks took too long with 3 acks pre-loaded: %v", elapsed)
	}
}

// TestAwaitDoneAcks_TimesOutAfterDeadline verifies that awaitDoneAcks returns
// after the timeout even when not all DoneAcks arrive.
func TestAwaitDoneAcks_TimesOutAfterDeadline(t *testing.T) {
	recv := make(chan interactionchannel.Message, 10)
	recv <- interactionchannel.DoneAck{Role: "header"}
	recv <- interactionchannel.DoneAck{Role: "log"}
	// footer ack never arrives

	timeout := 100 * time.Millisecond
	start := time.Now()
	awaitDoneAcks(recv, timeout)
	elapsed := time.Since(start)

	if elapsed < timeout {
		t.Errorf("awaitDoneAcks returned before timeout: %v < %v", elapsed, timeout)
	}
	if elapsed > 3*timeout {
		t.Errorf("awaitDoneAcks ran far past timeout: %v", elapsed)
	}
}

// TestAwaitDoneAcks_ClosedChannel_ReturnsEarly verifies that awaitDoneAcks
// returns early if the receive channel is closed before all acks arrive.
func TestAwaitDoneAcks_ClosedChannel_ReturnsEarly(t *testing.T) {
	recv := make(chan interactionchannel.Message, 10)
	recv <- interactionchannel.DoneAck{Role: "header"}
	close(recv)

	start := time.Now()
	awaitDoneAcks(recv, 5*time.Second)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("awaitDoneAcks should return immediately on closed channel, took: %v", elapsed)
	}
}

// ---------------------------------------------------------------------------
// Orchestrator WorkspaceDone protocol tests (using FakeInteractionChannel)
// ---------------------------------------------------------------------------

// TestWorkspaceDone_OrchestratorBroadcastsWorkspaceDone verifies that
// runCmuxOrchestratorWith sends WorkspaceDone after the readiness handshake.
func TestWorkspaceDone_OrchestratorBroadcastsWorkspaceDone(t *testing.T) {
	fake := interactionchannel.NewFakeInteractionChannel()
	// Pre-inject DoneAcks so the orchestrator can complete.
	fake.InjectMessage(interactionchannel.DoneAck{Role: "header"})
	fake.InjectMessage(interactionchannel.DoneAck{Role: "log"})
	fake.InjectMessage(interactionchannel.DoneAck{Role: "footer"})

	projectDir := t.TempDir()
	ctx := context.Background()

	if err := runCmuxOrchestratorWith(ctx, "", projectDir, 5*time.Second, fake); err != nil {
		t.Fatalf("runCmuxOrchestratorWith: %v", err)
	}

	var found bool
	for _, m := range fake.SentMessages() {
		if _, ok := m.(interactionchannel.WorkspaceDone); ok {
			found = true
			break
		}
	}
	if !found {
		t.Error("orchestrator did not broadcast WorkspaceDone")
	}
}

// TestWorkspaceDone_OrchestratorClosesChannelAfterAllAcks verifies that the
// orchestrator closes the channel (and thus unlinks the socket) after receiving
// DoneAck from all three display roles.
func TestWorkspaceDone_OrchestratorClosesChannelAfterAllAcks(t *testing.T) {
	fake := interactionchannel.NewFakeInteractionChannel()
	fake.InjectMessage(interactionchannel.DoneAck{Role: "header"})
	fake.InjectMessage(interactionchannel.DoneAck{Role: "log"})
	fake.InjectMessage(interactionchannel.DoneAck{Role: "footer"})

	projectDir := t.TempDir()
	ctx := context.Background()

	if err := runCmuxOrchestratorWith(ctx, "", projectDir, 5*time.Second, fake); err != nil {
		t.Fatalf("runCmuxOrchestratorWith: %v", err)
	}

	if !fake.IsClosed() {
		t.Error("orchestrator did not close the interaction channel after all DoneAcks")
	}
}

// TestWorkspaceDone_OrchestratorClosesChannelAfterTimeout verifies that the
// orchestrator closes the channel after the ack timeout even if not all panes
// have sent DoneAck.
func TestWorkspaceDone_OrchestratorClosesChannelAfterTimeout(t *testing.T) {
	fake := interactionchannel.NewFakeInteractionChannel()
	// Only 2 of 3 acks arrive — footer never acks.
	fake.InjectMessage(interactionchannel.DoneAck{Role: "header"})
	fake.InjectMessage(interactionchannel.DoneAck{Role: "log"})

	projectDir := t.TempDir()
	ctx := context.Background()
	timeout := 100 * time.Millisecond

	start := time.Now()
	if err := runCmuxOrchestratorWith(ctx, "", projectDir, timeout, fake); err != nil {
		t.Fatalf("runCmuxOrchestratorWith: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < timeout {
		t.Errorf("orchestrator returned before ack timeout: %v < %v", elapsed, timeout)
	}
	if elapsed > 3*timeout {
		t.Errorf("orchestrator ran far past timeout: %v", elapsed)
	}
	if !fake.IsClosed() {
		t.Error("orchestrator did not close the channel after timeout")
	}
}

// TestWorkspaceDone_OrchestratorUnlinksSocket is an integration test that
// verifies the orchestrator unlinks the real socket file after completing the
// WorkspaceDone protocol. Three goroutine panes respond automatically.
func TestWorkspaceDone_OrchestratorUnlinksSocket(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	projectDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start pane goroutines before the orchestrator so they can connect
	// as soon as the socket is created.
	paneFinished := make(chan struct{}, 3)
	for _, role := range []string{"header", "log", "footer"} {
		role := role
		go func() {
			defer func() { paneFinished <- struct{}{} }()
			// Retry dial until the socket is ready.
			var ch *interactionchannel.Channel
			for ctx.Err() == nil {
				var err error
				ch, err = interactionchannel.Dial(ctx, socketPath, role)
				if err == nil {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			if ctx.Err() != nil {
				return
			}
			defer ch.Close()
			_ = ch.Send(interactionchannel.Ready{Role: role})
			// Respond to WorkspaceDone with DoneAck, then keep alive.
			for {
				select {
				case msg, ok := <-ch.Recv():
					if !ok {
						return
					}
					if _, ok := msg.(interactionchannel.WorkspaceDone); ok {
						_ = ch.Send(interactionchannel.DoneAck{Role: role})
						// Stay alive so the final-state render loop is exercised.
						<-ctx.Done()
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	if err := runCmuxOrchestratorWith(ctx, socketPath, projectDir, 5*time.Second, nil); err != nil {
		t.Fatalf("runCmuxOrchestratorWith error: %v", err)
	}

	// Socket must be unlinked after orchestrator closes the channel.
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("socket file should be unlinked after orchestrator closes channel; Stat err:", err)
	}
}

// ---------------------------------------------------------------------------
// Display pane WorkspaceDone handling tests
// ---------------------------------------------------------------------------

// startServerAndWaitForRole starts a Channel server at socketPath, then waits
// for a single Ready message for the given role (not using AwaitReady which
// requires all 3 roles).
func startServerAndWaitForRole(t *testing.T, ctx context.Context, socketPath, role string) *interactionchannel.Channel {
	t.Helper()
	server, err := interactionchannel.Serve(ctx, socketPath)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	// Wait for the pane's Ready message.
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case msg, ok := <-server.Recv():
			if !ok {
				t.Fatal("server channel closed before Ready arrived")
			}
			if r, ok := msg.(interactionchannel.Ready); ok && r.Role == role {
				return server
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for Ready from %q pane", role)
		}
	}
}

// waitForDoneAck drains server.Recv until DoneAck{Role: role} is received or
// the deadline passes. Returns true on success.
func waitForDoneAck(t *testing.T, server *interactionchannel.Channel, role string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case msg, ok := <-server.Recv():
			if !ok {
				return false
			}
			if ack, ok := msg.(interactionchannel.DoneAck); ok && ack.Role == role {
				return true
			}
		case <-deadline.C:
			return false
		}
	}
}

// TestWorkspaceDone_HeaderPaneSendsDoneAck verifies that runCmuxHeaderPane
// sends DoneAck{Role:"header"} when it receives WorkspaceDone.
func TestWorkspaceDone_HeaderPaneSendsDoneAck(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	paneErr := make(chan error, 1)
	go func() {
		paneErr <- runCmuxHeaderPane(ctx, socketPath)
	}()

	server := startServerAndWaitForRole(t, ctx, socketPath, "header")
	defer server.Close()

	_ = server.Send(interactionchannel.WorkspaceDone{ExitCode: 0})

	if !waitForDoneAck(t, server, "header", 2*time.Second) {
		t.Error("header pane did not send DoneAck after WorkspaceDone")
	}
}

// TestWorkspaceDone_LogPaneSendsDoneAck verifies that runCmuxLogPane
// sends DoneAck{Role:"log"} when it receives WorkspaceDone.
func TestWorkspaceDone_LogPaneSendsDoneAck(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = runCmuxLogPane(ctx, socketPath)
	}()

	server := startServerAndWaitForRole(t, ctx, socketPath, "log")
	defer server.Close()

	_ = server.Send(interactionchannel.WorkspaceDone{ExitCode: 0})

	if !waitForDoneAck(t, server, "log", 2*time.Second) {
		t.Error("log pane did not send DoneAck after WorkspaceDone")
	}
}

// TestWorkspaceDone_FooterPaneSendsDoneAck verifies that runCmuxFooterPane
// sends DoneAck{Role:"footer"} when it receives WorkspaceDone.
func TestWorkspaceDone_FooterPaneSendsDoneAck(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = runCmuxFooterPane(ctx, socketPath)
	}()

	server := startServerAndWaitForRole(t, ctx, socketPath, "footer")
	defer server.Close()

	_ = server.Send(interactionchannel.WorkspaceDone{ExitCode: 0})

	if !waitForDoneAck(t, server, "footer", 2*time.Second) {
		t.Error("footer pane did not send DoneAck after WorkspaceDone")
	}
}

// TestWorkspaceDone_PaneContinuesAfterSocketClose verifies that a display pane
// does not panic when the server closes the socket (io.EOF). The pane should
// continue rendering its final state until the context is cancelled.
func TestWorkspaceDone_PaneContinuesAfterSocketClose(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	paneErr := make(chan error, 1)
	go func() {
		paneErr <- runCmuxHeaderPane(ctx, socketPath)
	}()

	server := startServerAndWaitForRole(t, ctx, socketPath, "header")

	_ = server.Send(interactionchannel.WorkspaceDone{ExitCode: 0})
	if !waitForDoneAck(t, server, "header", 2*time.Second) {
		t.Fatal("header pane did not send DoneAck")
	}

	// Close the server — pane should see EOF and continue running (not panic/return).
	server.Close()

	// Pane must NOT return before context is cancelled.
	select {
	case err := <-paneErr:
		t.Errorf("header pane returned early after socket close (should stay alive): %v", err)
	case <-time.After(200 * time.Millisecond):
		// Good — pane is still alive after socket close.
	}

	// Cancel context — pane should now return.
	cancel()
	select {
	case err := <-paneErr:
		if err != nil {
			t.Errorf("header pane returned non-nil error after context cancel: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("header pane did not return after context cancel")
	}
}

// ---------------------------------------------------------------------------
// Orchestrator handshake error propagation (G1)
// ---------------------------------------------------------------------------

// TestWorkspaceDone_OrchestratorHandshakeErrorPropagates verifies that a
// failed AwaitReady is returned as a "handshake:" wrapped error and that the
// orchestrator does not broadcast WorkspaceDone in that case.
func TestWorkspaceDone_OrchestratorHandshakeErrorPropagates(t *testing.T) {
	fake := interactionchannel.NewFakeInteractionChannel()

	// A pre-cancelled context causes FakeInteractionChannel.AwaitReady to
	// return context.Canceled immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	projectDir := t.TempDir()

	err := runCmuxOrchestratorWith(ctx, "", projectDir, 5*time.Second, fake)
	if err == nil {
		t.Fatal("expected an error from runCmuxOrchestratorWith but got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected error to wrap context.Canceled; got: %v", err)
	}

	// The orchestrator must not have broadcast WorkspaceDone on handshake failure.
	for _, m := range fake.SentMessages() {
		if _, ok := m.(interactionchannel.WorkspaceDone); ok {
			t.Error("orchestrator broadcast WorkspaceDone despite handshake failure")
		}
	}
}

// TestWorkspaceDone_PaneExactlyOneDoneAck verifies that each display role sends
// exactly one DoneAck — not zero, not two — in response to WorkspaceDone.
func TestWorkspaceDone_PaneExactlyOneDoneAck(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = runCmuxHeaderPane(ctx, socketPath)
	}()

	server := startServerAndWaitForRole(t, ctx, socketPath, "header")
	defer server.Close()

	_ = server.Send(interactionchannel.WorkspaceDone{ExitCode: 0})

	if !waitForDoneAck(t, server, "header", 2*time.Second) {
		t.Fatal("header pane did not send first DoneAck")
	}

	// Give time for a spurious second ack to arrive.
	time.Sleep(100 * time.Millisecond)

	// Drain remaining messages and count DoneAcks.
	count := 1 // already received one above
	for {
		select {
		case msg, ok := <-server.Recv():
			if !ok {
				goto done
			}
			if ack, ok := msg.(interactionchannel.DoneAck); ok && ack.Role == "header" {
				count++
			}
		default:
			goto done
		}
	}
done:
	if count != 1 {
		t.Errorf("expected exactly 1 DoneAck from header pane, got %d", count)
	}
}
