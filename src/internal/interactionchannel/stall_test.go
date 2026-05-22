package interactionchannel_test

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
)

// TestStallFiresWhenNoMessages verifies that onStall fires after the injected
// threshold when the connection delivers no messages.
func TestStallFiresWhenNoMessages(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	const threshold = 30 * time.Millisecond
	stallFired := make(chan struct{}, 1)
	server.SetStallConfig(threshold, func() {
		select {
		case stallFired <- struct{}{}:
		default:
		}
	})

	client, err := interactionchannel.Dial(ctx, sock, "log")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	// Exchange one message to confirm the connection is established.
	if err := client.Send(interactionchannel.Ready{Role: "log"}); err != nil {
		t.Fatalf("client.Send Ready: %v", err)
	}
	select {
	case <-server.Recv():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Ready on server")
	}

	// Now go silent — stall should fire within 5× threshold.
	select {
	case <-stallFired:
		// pass
	case <-time.After(5 * threshold):
		t.Fatalf("onStall not fired within %v", 5*threshold)
	}
}

// TestStallResetOnMessage verifies that a message arriving before the threshold
// resets the timer so the stall does not fire at the original deadline.
func TestStallResetOnMessage(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	const threshold = 60 * time.Millisecond
	stallFired := make(chan struct{}, 1)
	server.SetStallConfig(threshold, func() {
		select {
		case stallFired <- struct{}{}:
		default:
		}
	})

	client, err := interactionchannel.Dial(ctx, sock, "log")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	// Establish connection.
	if err := client.Send(interactionchannel.Ready{Role: "log"}); err != nil {
		t.Fatalf("client.Send Ready: %v", err)
	}
	select {
	case <-server.Recv():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server recv")
	}

	// Send a second message at ≈threshold/2 to reset the stall timer.
	time.Sleep(threshold / 2)
	if err := client.Send(interactionchannel.StateHeader{IterationLine: "ping"}); err != nil {
		t.Fatalf("client.Send StateHeader: %v", err)
	}
	select {
	case <-server.Recv():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for StateHeader on server")
	}

	// Stall must NOT fire within threshold/2 after the reset message.
	select {
	case <-stallFired:
		t.Fatal("stall fired early — timer was not reset by the message")
	case <-time.After(threshold / 2):
		// Good: no premature stall.
	}

	// After the full threshold from the reset, stall should eventually fire.
	select {
	case <-stallFired:
		// pass
	case <-time.After(3 * threshold):
		t.Fatalf("stall did not fire within %v after reset", 3*threshold)
	}
}

// TestCleanDisconnectWithoutWaitingForStall verifies that a peer disconnect
// (EOF / broken connection) is detected promptly, not delayed by the stall threshold.
func TestCleanDisconnectWithoutWaitingForStall(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	// Long stall threshold — disconnect must not wait for it.
	const stallDelay = 5 * time.Second
	server.SetStallConfig(stallDelay, func() {
		t.Errorf("onStall fired — should not be called for a clean disconnect")
	})

	client, err := interactionchannel.Dial(ctx, sock, "log")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// Establish connection.
	if err := client.Send(interactionchannel.Ready{Role: "log"}); err != nil {
		t.Fatalf("client.Send Ready: %v", err)
	}
	select {
	case <-server.Recv():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server recv")
	}

	disconnectAt := time.Now()
	client.Close()

	// Server should detect the disconnect well within the stall threshold.
	// Allow 1 second for OS to propagate the close.
	const disconnectDetectLimit = 1 * time.Second
	deadline := time.After(disconnectDetectLimit)

	// The server's Recv channel won't deliver more messages; we detect the
	// disconnect by observing that the server's connection list drains.
	// Use a Send-returns-error probe: once all connections are removed,
	// server.Send returns an error promptly.
	for {
		select {
		case <-deadline:
			t.Fatalf("disconnect not detected within %v (stall threshold is %v)", disconnectDetectLimit, stallDelay)
		case <-time.After(5 * time.Millisecond):
			err := server.Send(interactionchannel.StateHeader{})
			if err != nil {
				elapsed := time.Since(disconnectAt)
				if elapsed > disconnectDetectLimit {
					t.Errorf("disconnect detected, but took %v — longer than %v limit", elapsed, disconnectDetectLimit)
				}
				return
			}
		}
	}
}

// TestStallTimerNoLeak verifies that the stall timer stops when readLoop ends
// so that goroutine counts return to baseline after Close.
// Not run in parallel because it reads runtime.NumGoroutine() globally.
func TestStallTimerNoLeak(t *testing.T) {
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}

	const threshold = 30 * time.Millisecond
	server.SetStallConfig(threshold, nil) // nil = default (close nc)

	client, err := interactionchannel.Dial(ctx, sock, "log")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// Exchange a message to ensure goroutines are running.
	if err := client.Send(interactionchannel.Ready{Role: "log"}); err != nil {
		t.Fatalf("client.Send: %v", err)
	}
	select {
	case <-server.Recv():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server recv")
	}

	during := runtime.NumGoroutine()

	cancel()
	server.Close()
	client.Close()

	deadline := time.Now().Add(2 * time.Second)
	var final int
	for time.Now().Before(deadline) {
		final = runtime.NumGoroutine()
		if final < during {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if final >= during {
		t.Errorf("goroutine leak: during=%d final=%d", during, final)
	}
}
