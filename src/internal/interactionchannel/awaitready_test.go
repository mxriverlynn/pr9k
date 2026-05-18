package interactionchannel_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
)

// connectAndSendReady dials socketPath, sends Ready{Role: role}, and returns
// the client Channel. The caller owns closing it.
func connectAndSendReady(t *testing.T, ctx context.Context, socketPath, role string) *interactionchannel.Channel {
	t.Helper()
	client, err := interactionchannel.Dial(ctx, socketPath, role)
	if err != nil {
		t.Fatalf("Dial(%q): %v", role, err)
	}
	if err := client.Send(interactionchannel.Ready{Role: role}); err != nil {
		t.Fatalf("Send Ready(%q): %v", role, err)
	}
	return client
}

// TestAwaitReady_AllThreeReady verifies that AwaitReady returns nil when all
// three display panes (header, log, footer) signal Ready within the timeout.
func TestAwaitReady_AllThreeReady(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	// All three panes connect and signal ready before we call AwaitReady.
	clients := make([]*interactionchannel.Channel, 3)
	for i, role := range []string{"header", "log", "footer"} {
		clients[i] = connectAndSendReady(t, ctx, sock, role)
		defer clients[i].Close()
	}

	if err := server.AwaitReady(ctx, 2*time.Second); err != nil {
		t.Errorf("AwaitReady: expected nil, got %v", err)
	}
}

// TestAwaitReady_TimeoutMissingRole verifies that AwaitReady returns a
// structured error naming the missing role when the deadline expires before
// all roles have signalled ready.
func TestAwaitReady_TimeoutMissingRole(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	// Only header and log connect; footer never signals.
	for _, role := range []string{"header", "log"} {
		c := connectAndSendReady(t, ctx, sock, role)
		defer c.Close()
	}

	// Give the server a moment to receive the Ready messages, then call
	// AwaitReady with a short deadline.
	time.Sleep(20 * time.Millisecond)
	err = server.AwaitReady(ctx, 100*time.Millisecond)
	if err == nil {
		t.Fatal("AwaitReady: expected non-nil error, got nil")
	}
	if !strings.Contains(err.Error(), "footer") {
		t.Errorf("AwaitReady error should name missing role 'footer', got: %v", err)
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("AwaitReady error should mention 'timeout', got: %v", err)
	}
}

// TestAwaitReady_DuplicateReadyIdempotent verifies that a second Ready from
// the same role (e.g. a reconnect) is treated as idempotent: AwaitReady still
// requires the other two distinct roles and does not double-count.
func TestAwaitReady_DuplicateReadyIdempotent(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	// header connects and sends Ready twice (simulating a reconnect).
	hdr := connectAndSendReady(t, ctx, sock, "header")
	defer hdr.Close()
	if err := hdr.Send(interactionchannel.Ready{Role: "header"}); err != nil {
		t.Fatalf("Send duplicate Ready(header): %v", err)
	}

	// log and footer each connect once.
	for _, role := range []string{"log", "footer"} {
		c := connectAndSendReady(t, ctx, sock, role)
		defer c.Close()
	}

	// AwaitReady must return nil — the duplicate should not prevent completion.
	if err := server.AwaitReady(ctx, 2*time.Second); err != nil {
		t.Errorf("AwaitReady: expected nil with duplicate Ready, got %v", err)
	}
}

// TestAwaitReady_ContextCancellation verifies that cancelling the context
// passed to AwaitReady returns ctx.Err() cleanly without blocking.
func TestAwaitReady_ContextCancellation(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	baseCtx, baseCancel := context.WithCancel(context.Background())
	defer baseCancel()

	server, err := interactionchannel.Serve(baseCtx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	// No panes connect. Cancel the AwaitReady context after a short delay.
	awaitCtx, awaitCancel := context.WithCancel(baseCtx)
	go func() {
		time.Sleep(30 * time.Millisecond)
		awaitCancel()
	}()

	err = server.AwaitReady(awaitCtx, 10*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("AwaitReady: expected context.Canceled, got %v", err)
	}
}
