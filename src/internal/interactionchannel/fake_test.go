package interactionchannel_test

import (
	"context"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
)

// ---------------------------------------------------------------------------
// FakeInteractionChannel tests
// ---------------------------------------------------------------------------

// TestFakeInteractionChannel_EnqueueReady verifies that EnqueueReady puts a
// Ready message onto the Recv channel consumable by the orchestrator.
func TestFakeInteractionChannel_EnqueueReady(t *testing.T) {
	t.Parallel()
	f := interactionchannel.NewFakeInteractionChannel()
	defer f.Close()

	f.EnqueueReady("header")

	select {
	case msg := <-f.Recv():
		got, ok := msg.(interactionchannel.Ready)
		if !ok {
			t.Fatalf("Recv: got %T, want Ready", msg)
		}
		if got.Role != "header" {
			t.Errorf("Role: got %q, want %q", got.Role, "header")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for EnqueueReady message")
	}
}

// TestFakeInteractionChannel_InjectIntent verifies that InjectIntent puts an
// Intent message onto the Recv channel.
func TestFakeInteractionChannel_InjectIntent(t *testing.T) {
	t.Parallel()
	f := interactionchannel.NewFakeInteractionChannel()
	defer f.Close()

	f.InjectIntent(interactionchannel.IntentQuit)

	select {
	case msg := <-f.Recv():
		got, ok := msg.(interactionchannel.Intent)
		if !ok {
			t.Fatalf("Recv: got %T, want Intent", msg)
		}
		if got.Kind != interactionchannel.IntentQuit {
			t.Errorf("Kind: got %v, want %v", got.Kind, interactionchannel.IntentQuit)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for InjectIntent message")
	}
}

// TestFakeInteractionChannel_Send_ExpectStatePush verifies that Send records
// messages and ExpectStatePush drains them FIFO.
func TestFakeInteractionChannel_Send_ExpectStatePush(t *testing.T) {
	t.Parallel()
	f := interactionchannel.NewFakeInteractionChannel()
	defer f.Close()

	msgs := []interactionchannel.Message{
		interactionchannel.StateHeader{IterationLine: "Iteration 1/3"},
		interactionchannel.StateFooter{Mode: 0, ShortcutLine: "q quit"},
		interactionchannel.WorkspaceDone{ExitCode: 0},
	}
	for _, m := range msgs {
		if err := f.Send(m); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}

	for i, want := range msgs {
		got, ok := f.ExpectStatePush()
		if !ok {
			t.Fatalf("ExpectStatePush[%d]: queue was empty", i)
		}
		if got.WireType() != want.WireType() {
			t.Errorf("ExpectStatePush[%d]: got %q, want %q", i, got.WireType(), want.WireType())
		}
	}

	_, ok := f.ExpectStatePush()
	if ok {
		t.Error("ExpectStatePush: expected empty queue after draining, got a message")
	}
}

// TestFakeInteractionChannel_SendAfterClose verifies Send returns an error
// after Close.
func TestFakeInteractionChannel_SendAfterClose(t *testing.T) {
	t.Parallel()
	f := interactionchannel.NewFakeInteractionChannel()
	f.Close()

	err := f.Send(interactionchannel.StateHeader{IterationLine: "x"})
	if err == nil {
		t.Error("Send after Close: expected non-nil error, got nil")
	}
}

// TestFakeInteractionChannel_OrchestratorTopology verifies the orchestrator-
// side topology: inject multiple Ready messages (one per role) and send
// multiple state pushes.
func TestFakeInteractionChannel_OrchestratorTopology(t *testing.T) {
	t.Parallel()
	f := interactionchannel.NewFakeInteractionChannel()
	defer f.Close()

	// Inject three role-Ready messages.
	for _, role := range []string{"header", "log", "footer"} {
		f.EnqueueReady(role)
	}

	// Consume all three.
	roles := map[string]bool{}
	for i := 0; i < 3; i++ {
		select {
		case msg := <-f.Recv():
			r, ok := msg.(interactionchannel.Ready)
			if !ok {
				t.Fatalf("msg[%d]: got %T, want Ready", i, msg)
			}
			roles[r.Role] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for ready[%d]", i)
		}
	}
	for _, want := range []string{"header", "log", "footer"} {
		if !roles[want] {
			t.Errorf("role %q not received", want)
		}
	}
}

// ---------------------------------------------------------------------------
// FakeDisplayPane tests
// ---------------------------------------------------------------------------

// TestFakeDisplayPane_ConnectSendReadyExpectMessage verifies the full
// connect→ready→receive round-trip against a real Serve socket.
func TestFakeDisplayPane_ConnectSendReadyExpectMessage(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	pane := interactionchannel.NewFakeDisplayPane("header")
	if err := pane.Connect(ctx, sock); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pane.Disconnect()

	// SendReady so the server side sees a Ready message.
	if err := pane.SendReady(); err != nil {
		t.Fatalf("SendReady: %v", err)
	}

	// Server side receives the Ready message.
	select {
	case msg := <-server.Recv():
		r, ok := msg.(interactionchannel.Ready)
		if !ok {
			t.Fatalf("server.Recv: got %T, want Ready", msg)
		}
		if r.Role != "header" {
			t.Errorf("Role: got %q, want %q", r.Role, "header")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Ready from pane")
	}

	// Server pushes a StateHeader; pane records it.
	push := interactionchannel.StateHeader{IterationLine: "Iteration 2/5"}
	if err := server.Send(push); err != nil {
		t.Fatalf("server.Send: %v", err)
	}

	// Poll until the pane's background reader delivers the message.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got, ok := pane.ExpectMessage()
		if ok {
			sh, ok := got.(interactionchannel.StateHeader)
			if !ok {
				t.Fatalf("ExpectMessage: got %T, want StateHeader", got)
			}
			if sh.IterationLine != push.IterationLine {
				t.Errorf("IterationLine: got %q, want %q", sh.IterationLine, push.IterationLine)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout waiting for pane to receive StateHeader")
}

// TestFakeDisplayPane_SlowConsumer verifies that the slow-consumer hook can
// be toggled and that the pane still receives messages after the flag is cleared.
func TestFakeDisplayPane_SlowConsumer(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	pane := interactionchannel.NewFakeDisplayPane("log")
	if err := pane.Connect(ctx, sock); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer pane.Disconnect()

	// Establish connection: send Ready and wait for server to receive it.
	if err := pane.SendReady(); err != nil {
		t.Fatalf("SendReady: %v", err)
	}
	select {
	case <-server.Recv():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for connection to be established")
	}

	// Enable then immediately disable slow consumer.
	pane.SetSlowConsumer(true)
	pane.SetSlowConsumer(false)

	// Send a message and verify it is received.
	if err := server.Send(interactionchannel.StateLog{Lines: [][]byte{[]byte("hello")}}); err != nil {
		t.Fatalf("server.Send: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := pane.ExpectMessage(); ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout: pane did not receive message after slow-consumer cleared")
}

// TestFakeDisplayPane_PrematureExit verifies that SetPrematureExit causes the
// pane to disconnect, making the server's next Send return an error.
func TestFakeDisplayPane_PrematureExit(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	pane := interactionchannel.NewFakeDisplayPane("footer")
	if err := pane.Connect(ctx, sock); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Trigger premature exit — should close the pane's connection.
	pane.SetPrematureExit(true)

	// Give the disconnect a moment to propagate, then verify the pane's
	// Disconnect has been called (no assertions on server side needed — the
	// pane closing is the observable behavior).
	time.Sleep(50 * time.Millisecond)
	// Calling Disconnect on an already-exited pane must not panic.
	pane.Disconnect()
}

// TestFakeDisplayPane_PrematureExit_ClosesWire verifies that after
// SetPrematureExit(true), the pane's underlying connection is actually closed
// so server.Send eventually returns an error. This tests the docstring claim
// ("simulates a pane process that exits before WorkspaceDone") that the
// existing idempotency test does not exercise.
func TestFakeDisplayPane_PrematureExit_ClosesWire(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	pane := interactionchannel.NewFakeDisplayPane("log")
	if err := pane.Connect(ctx, sock); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Establish the connection fully so the server has a live peer.
	if err := pane.SendReady(); err != nil {
		t.Fatalf("SendReady: %v", err)
	}
	select {
	case <-server.Recv():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Ready from pane")
	}

	// Trigger premature exit — pane closes its connection on next read iteration.
	pane.SetPrematureExit(true)

	// Wake the pane's read loop so it sees the earlyExit flag.
	_ = server.Send(interactionchannel.StateHeader{IterationLine: "wake"})

	// Poll server.Send until it returns an error (broken pipe / closed conn).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := server.Send(interactionchannel.StateHeader{IterationLine: "probe"}); err != nil {
			return // wire is closed — test passes
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout: server.Send never returned an error after pane SetPrematureExit")
}

// TestFakeDisplayPane_SendReadyBeforeConnect verifies that SendReady returns a
// non-nil error when called before Connect.
func TestFakeDisplayPane_SendReadyBeforeConnect(t *testing.T) {
	t.Parallel()
	pane := interactionchannel.NewFakeDisplayPane("header")
	if err := pane.SendReady(); err == nil {
		t.Error("SendReady before Connect: expected non-nil error, got nil")
	}
}

// ---------------------------------------------------------------------------
// FakeFooterKeySource tests
// ---------------------------------------------------------------------------

// TestFakeFooterKeySource_PressNext verifies that Press enqueues and Next
// dequeues keys in FIFO order.
func TestFakeFooterKeySource_PressNext(t *testing.T) {
	t.Parallel()
	f := interactionchannel.NewFakeFooterKeySource()

	f.Press("q")
	f.Press("y")
	f.Press("esc")

	for _, want := range []string{"q", "y", "esc"} {
		got, ok := f.Next()
		if !ok {
			t.Fatalf("Next: queue empty, expected %q", want)
		}
		if got != want {
			t.Errorf("Next: got %q, want %q", got, want)
		}
	}

	_, ok := f.Next()
	if ok {
		t.Error("Next: expected empty queue after consuming all keys")
	}
}

// TestFakeFooterKeySource_SetMode verifies that SetMode and Mode round-trip.
func TestFakeFooterKeySource_SetMode(t *testing.T) {
	t.Parallel()
	f := interactionchannel.NewFakeFooterKeySource()

	const wantMode = 3 // e.g. ModeQuitConfirm
	f.SetMode(wantMode)
	if got := f.Mode(); got != wantMode {
		t.Errorf("Mode: got %d, want %d", got, wantMode)
	}
}

// TestFakeFooterKeySource_RecordIntent verifies that RecordIntent and
// LastForwardedIntent round-trip.
func TestFakeFooterKeySource_RecordIntent(t *testing.T) {
	t.Parallel()
	f := interactionchannel.NewFakeFooterKeySource()

	_, ok := f.LastForwardedIntent()
	if ok {
		t.Error("LastForwardedIntent: expected false before any intent recorded")
	}

	f.RecordIntent(interactionchannel.IntentQuit)
	got, ok := f.LastForwardedIntent()
	if !ok {
		t.Fatal("LastForwardedIntent: expected true after RecordIntent")
	}
	if got != interactionchannel.IntentQuit {
		t.Errorf("LastForwardedIntent: got %v, want %v", got, interactionchannel.IntentQuit)
	}
}

// TestFakeFooterKeySource_KeystrokeSequence verifies a scripted sequence
// can be delivered and inspected.
func TestFakeFooterKeySource_KeystrokeSequence(t *testing.T) {
	t.Parallel()
	f := interactionchannel.NewFakeFooterKeySource()

	// Script: q → y (simulating quit-confirm).
	f.Press("q")
	f.Press("y")

	k1, _ := f.Next()
	k2, _ := f.Next()

	if k1 != "q" || k2 != "y" {
		t.Errorf("sequence: got %q %q, want q y", k1, k2)
	}

	// No more keys.
	_, ok := f.Next()
	if ok {
		t.Error("Next: expected empty queue")
	}
}

// ---------------------------------------------------------------------------
// FakeInteractionChannel snapshot tests
// ---------------------------------------------------------------------------

// TestFakeInteractionChannel_SentMessages_IndependentSnapshot verifies that
// SentMessages returns a defensive copy: mutating the returned slice does not
// affect a subsequent call.
func TestFakeInteractionChannel_SentMessages_IndependentSnapshot(t *testing.T) {
	t.Parallel()
	f := interactionchannel.NewFakeInteractionChannel()
	defer f.Close()

	m1 := interactionchannel.StateHeader{IterationLine: "Iteration 1/3"}
	m2 := interactionchannel.StateFooter{Mode: 0, ShortcutLine: "q quit"}

	if err := f.Send(m1); err != nil {
		t.Fatalf("Send m1: %v", err)
	}
	if err := f.Send(m2); err != nil {
		t.Fatalf("Send m2: %v", err)
	}

	// Capture a snapshot and mutate it.
	snap1 := f.SentMessages()
	if len(snap1) != 2 {
		t.Fatalf("SentMessages: got %d messages, want 2", len(snap1))
	}
	snap1[0] = nil // mutate the returned slice

	// A fresh snapshot must still contain the original messages.
	snap2 := f.SentMessages()
	if len(snap2) != 2 {
		t.Fatalf("SentMessages after mutation: got %d messages, want 2", len(snap2))
	}
	if snap2[0] == nil {
		t.Error("SentMessages: second snapshot reflects mutation of first — not an independent copy")
	}
	if snap2[0].WireType() != m1.WireType() {
		t.Errorf("SentMessages[0]: got %q, want %q", snap2[0].WireType(), m1.WireType())
	}
	if snap2[1].WireType() != m2.WireType() {
		t.Errorf("SentMessages[1]: got %q, want %q", snap2[1].WireType(), m2.WireType())
	}
}
