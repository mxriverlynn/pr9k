package interactionchannel_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
)

// connectThreePanes connects header, log, and footer FakeDisplayPanes to the
// server socket, sends Ready for each, and waits for AwaitReady to complete.
// Returns the three panes; caller owns calling Disconnect on each.
func connectThreePanes(
	t *testing.T,
	ctx context.Context,
	server *interactionchannel.Channel,
	sock string,
) (header, log, footer *interactionchannel.FakeDisplayPane) {
	t.Helper()
	panes := map[string]**interactionchannel.FakeDisplayPane{
		"header": &header,
		"log":    &log,
		"footer": &footer,
	}
	for role, ptr := range panes {
		p := interactionchannel.NewFakeDisplayPane(role)
		if err := p.Connect(ctx, sock); err != nil {
			t.Fatalf("FakeDisplayPane(%s).Connect: %v", role, err)
		}
		if err := p.SendReady(); err != nil {
			t.Fatalf("FakeDisplayPane(%s).SendReady: %v", role, err)
		}
		*ptr = p
	}
	if err := server.AwaitReady(ctx, 5*time.Second); err != nil {
		t.Fatalf("AwaitReady: %v", err)
	}
	return header, log, footer
}

// waitForMessage polls pane.ExpectMessage until it sees a message satisfying
// pred, or the deadline passes. Returns true on success.
func waitForMessage(
	t *testing.T,
	pane *interactionchannel.FakeDisplayPane,
	pred func(interactionchannel.Message) bool,
	timeout time.Duration,
) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if msg, ok := pane.ExpectMessage(); ok && pred(msg) {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return false
}

// ---------------------------------------------------------------------------
// Fan-out isolation
// ---------------------------------------------------------------------------

// TestFanout_SlowConsumerIsolation verifies that a slow log pane does not
// block SendStateHeader from delivering to the header pane. This exercises
// the per-connection goroutine isolation required by D-2.
func TestFanout_SlowConsumerIsolation(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	headerPane, logPane, footerPane := connectThreePanes(t, ctx, server, sock)
	defer headerPane.Disconnect()
	defer logPane.Disconnect()
	defer footerPane.Disconnect()

	// Make the log pane a slow consumer so its outbound path is congested.
	logPane.SetSlowConsumer(true)

	// Flood the log role — SendStateLog must not block the caller even if the
	// log pane is slow (drop-oldest semantics per D-2).
	const burst = 300 // exceeds logBufSize=256
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < burst; i++ {
			server.SendStateLog(interactionchannel.StateLog{
				Lines: [][]byte{[]byte(fmt.Sprintf("line %d", i))},
			})
		}
	}()

	// Concurrently, send a StateHeader to the header role.
	want := interactionchannel.StateHeader{IterationLine: "Iteration 1/3 (isolation-check)"}
	server.SendStateHeader(want)

	// Header pane must receive its message promptly — not blocked by log.
	if !waitForMessage(t, headerPane, func(m interactionchannel.Message) bool {
		sh, ok := m.(interactionchannel.StateHeader)
		return ok && sh.IterationLine == want.IterationLine
	}, 2*time.Second) {
		t.Error("header pane did not receive StateHeader while log pane was slow")
	}

	// The log-flood goroutine must also finish quickly (SendStateLog is non-blocking).
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("SendStateLog flood did not complete within timeout — possible blocking")
	}
}

// ---------------------------------------------------------------------------
// Drop-oldest log semantics
// ---------------------------------------------------------------------------

// TestFanout_DropOldest_LogBurst verifies that sending more StateLog messages
// than the log channel capacity causes the oldest to be dropped rather than
// blocking the caller or panicking. The newest messages are preserved.
func TestFanout_DropOldest_LogBurst(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	// Connect only the log pane for this test; header and footer can remain unconnected
	// (connForRole will return nil-ok for them, which is fine for SendStateLog).
	logPane := interactionchannel.NewFakeDisplayPane("log")
	if err := logPane.Connect(ctx, sock); err != nil {
		t.Fatalf("log pane Connect: %v", err)
	}
	defer logPane.Disconnect()

	// Drain the Ready message the log pane will send.
	if err := logPane.SendReady(); err != nil {
		t.Fatalf("log pane SendReady: %v", err)
	}
	// Wait until the server has processed the Ready so connForRole("log") is set.
	select {
	case <-server.Recv():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server to receive log Ready")
	}

	// Make the log pane very slow so its outbound path fills up.
	logPane.SetSlowConsumer(true)

	// The send loop must complete quickly — no blocking even if the channel
	// is at capacity (drop-oldest kicks in).
	const burst = 400 // well above logBufSize=256
	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		for i := 0; i < burst; i++ {
			server.SendStateLog(interactionchannel.StateLog{
				Lines: [][]byte{[]byte(fmt.Sprintf("log-drop-%d", i))},
			})
		}
	}()

	select {
	case <-sendDone:
	case <-time.After(2 * time.Second):
		t.Fatal("SendStateLog burst did not complete within timeout — caller blocked")
	}

	// Now let the pane drain and verify the last messages were received (not the
	// first ones, which were dropped).
	logPane.SetSlowConsumer(false)

	// Collect whatever arrives within a reasonable window.
	time.Sleep(500 * time.Millisecond)

	received := logPane.Received()
	if len(received) == 0 {
		t.Fatal("log pane received zero messages after burst")
	}

	// The last received message should be among the newest sent messages
	// (oldest were dropped). Verify the final message has a high index.
	last := received[len(received)-1]
	sl, ok := last.(interactionchannel.StateLog)
	if !ok {
		t.Fatalf("last received is %T, want StateLog", last)
	}
	if len(sl.Lines) == 0 {
		t.Fatal("last StateLog has no lines")
	}
	// The newest entries sent were log-drop-399, log-drop-398, ... Drop-oldest
	// means the lowest-index messages are evicted first. The last received
	// should NOT be "log-drop-0".
	if string(sl.Lines[0]) == "log-drop-0" {
		t.Error("drop-oldest: received log-drop-0 as last message — oldest was not dropped")
	}
}

// ---------------------------------------------------------------------------
// Latest-wins header / footer semantics
// ---------------------------------------------------------------------------

// TestFanout_LatestWins_Header verifies that when multiple StateHeader updates
// are sent rapidly, the last message received by the header pane is the latest
// one sent. This exercises the 1-deep slot coalescing required by D-2.
func TestFanout_LatestWins_Header(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	headerPane := interactionchannel.NewFakeDisplayPane("header")
	if err := headerPane.Connect(ctx, sock); err != nil {
		t.Fatalf("header pane Connect: %v", err)
	}
	defer headerPane.Disconnect()

	if err := headerPane.SendReady(); err != nil {
		t.Fatalf("header pane SendReady: %v", err)
	}
	select {
	case <-server.Recv():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for header Ready")
	}

	// Send 10 rapid header updates.
	const n = 10
	for i := 1; i <= n; i++ {
		server.SendStateHeader(interactionchannel.StateHeader{
			IterationLine: fmt.Sprintf("Iteration %d/%d", i, n),
		})
	}

	// Wait until at least one message has arrived (using Received so we don't drain).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(headerPane.Received()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Give any in-flight messages a moment to land, then take a snapshot.
	time.Sleep(100 * time.Millisecond)
	all := headerPane.Received()
	if len(all) == 0 {
		t.Fatal("header pane received no StateHeader messages")
	}

	// The last received message must be the last one sent (latest-wins).
	last, ok := all[len(all)-1].(interactionchannel.StateHeader)
	if !ok {
		t.Fatalf("last message is %T, want StateHeader", all[len(all)-1])
	}
	want := fmt.Sprintf("Iteration %d/%d", n, n)
	if last.IterationLine != want {
		t.Errorf("latest-wins header: last received %q, want %q", last.IterationLine, want)
	}
}

// TestFanout_LatestWins_Footer verifies that rapid StateFooter bursts coalesce
// so the last message received is the most recently sent.
func TestFanout_LatestWins_Footer(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	footerPane := interactionchannel.NewFakeDisplayPane("footer")
	if err := footerPane.Connect(ctx, sock); err != nil {
		t.Fatalf("footer pane Connect: %v", err)
	}
	defer footerPane.Disconnect()

	if err := footerPane.SendReady(); err != nil {
		t.Fatalf("footer pane SendReady: %v", err)
	}
	select {
	case <-server.Recv():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for footer Ready")
	}

	const n = 8
	for i := 1; i <= n; i++ {
		server.SendStateFooter(interactionchannel.StateFooter{
			Mode:         i,
			ShortcutLine: fmt.Sprintf("mode-%d shortcuts", i),
		})
	}

	// Wait until at least one message has arrived (using Received so we don't drain).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(footerPane.Received()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Give any in-flight messages a moment to land, then take a snapshot.
	time.Sleep(100 * time.Millisecond)
	all := footerPane.Received()
	if len(all) == 0 {
		t.Fatal("footer pane received no StateFooter messages")
	}

	last, ok := all[len(all)-1].(interactionchannel.StateFooter)
	if !ok {
		t.Fatalf("last message is %T, want StateFooter", all[len(all)-1])
	}
	if last.Mode != n {
		t.Errorf("latest-wins footer: last mode %d, want %d", last.Mode, n)
	}
}

// ---------------------------------------------------------------------------
// Pre-populate first-phase header (D-8)
// ---------------------------------------------------------------------------

// TestFanout_PrePopulate_FirstPhaseHeader verifies that after AwaitReady the
// orchestrator can call SendStateHeader and the header pane receives it as its
// first state message — with no prior state received.
func TestFanout_PrePopulate_FirstPhaseHeader(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	headerPane, logPane, footerPane := connectThreePanes(t, ctx, server, sock)
	defer headerPane.Disconnect()
	defer logPane.Disconnect()
	defer footerPane.Disconnect()

	// Immediately after AwaitReady, push the first-phase header state (D-8).
	firstPhase := interactionchannel.StateHeader{
		IterationLine: "Iteration 1/3",
		StepNames:     []string{"feature", "test"},
		StepStates:    []int{0, 0},
	}
	server.SendStateHeader(firstPhase)

	// The first message the header pane receives must be the pre-populated state.
	if !waitForMessage(t, headerPane, func(m interactionchannel.Message) bool {
		sh, ok := m.(interactionchannel.StateHeader)
		return ok && sh.IterationLine == firstPhase.IterationLine
	}, 2*time.Second) {
		t.Error("header pane did not receive the pre-populated first-phase header as its first message")
	}

	// Log and footer panes must not have received anything (no state pushed to them).
	time.Sleep(50 * time.Millisecond)
	if msg, ok := logPane.ExpectMessage(); ok {
		t.Errorf("log pane unexpectedly received %T after pre-populate header only", msg)
	}
	if msg, ok := footerPane.ExpectMessage(); ok {
		t.Errorf("footer pane unexpectedly received %T after pre-populate header only", msg)
	}
}

// ---------------------------------------------------------------------------
// Heartbeat indicator absent (D-10)
// ---------------------------------------------------------------------------

// TestFanout_NoHeartbeatSuffix verifies that StateHeader messages are
// delivered to the header pane exactly as sent — the channel does not inject
// a heartbeat indicator suffix. This enforces D-10 (YAGNI-1 reopen criterion).
func TestFanout_NoHeartbeatSuffix(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	headerPane := interactionchannel.NewFakeDisplayPane("header")
	if err := headerPane.Connect(ctx, sock); err != nil {
		t.Fatalf("header pane Connect: %v", err)
	}
	defer headerPane.Disconnect()

	if err := headerPane.SendReady(); err != nil {
		t.Fatalf("header pane SendReady: %v", err)
	}
	select {
	case <-server.Recv():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Ready from header pane")
	}

	sent := interactionchannel.StateHeader{
		IterationLine: "Iteration 2/5",
		StepNames:     []string{"alpha", "beta", "gamma"},
		StepStates:    []int{2, 1, 0},
	}
	server.SendStateHeader(sent)

	if !waitForMessage(t, headerPane, func(m interactionchannel.Message) bool {
		sh, ok := m.(interactionchannel.StateHeader)
		if !ok {
			return false
		}
		// Verify the content is preserved exactly — no heartbeat suffix appended.
		if sh.IterationLine != sent.IterationLine {
			t.Errorf("heartbeat check: IterationLine got %q, want %q",
				sh.IterationLine, sent.IterationLine)
		}
		return sh.IterationLine == sent.IterationLine
	}, 2*time.Second) {
		t.Error("header pane did not receive the StateHeader")
	}
}

// ---------------------------------------------------------------------------
// Race detector coverage
// ---------------------------------------------------------------------------

// TestFanout_RaceDetector exercises concurrent SendState* calls to verify the
// race detector finds no data races across fan-out goroutines.
func TestFanout_RaceDetector(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	headerPane, logPane, footerPane := connectThreePanes(t, ctx, server, sock)
	defer headerPane.Disconnect()
	defer logPane.Disconnect()
	defer footerPane.Disconnect()

	// Concurrent sends to all three roles.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			server.SendStateHeader(interactionchannel.StateHeader{
				IterationLine: fmt.Sprintf("H%d", i),
			})
			server.SendStateLog(interactionchannel.StateLog{
				Lines: [][]byte{[]byte(fmt.Sprintf("log %d", i))},
			})
			server.SendStateFooter(interactionchannel.StateFooter{
				Mode:         i % 4,
				ShortcutLine: fmt.Sprintf("F%d", i),
			})
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent sends timed out")
	}
}
