package interactionchannel_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
)

// sockPath returns a unique temp socket path for the given test.
//
// The socket is bound under a short /tmp directory rather than t.TempDir():
// macOS caps a Unix socket address (sun_path) at 104 bytes, and the
// /var/folders/<...>/T/<TestName>/NNN paths t.TempDir() returns exceed that
// once "/test.sock" is appended, so net.Listen("unix", ...) fails with
// "bind: invalid argument" (EINVAL) on the primary development platform.
// /tmp is present and short on both macOS and Linux; the 0700 mode keeps the
// directory non-world-writable.
func sockPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "p9k")
	if err != nil {
		t.Fatalf("sockPath: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return filepath.Join(dir, "test.sock")
}

// TestServeDialRoundTrip verifies that a Serve-listener accepts a Dial-client
// and can round-trip a message in each direction.
func TestServeDialRoundTrip(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	defer server.Close()

	client, err := interactionchannel.Dial(ctx, sock, "header")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	// Client → server first: the client's write goroutine is running immediately
	// after Dial. The server's accept loop may not have processed the new
	// connection yet, so we let the client send first and wait for the server
	// to receive — this proves the connection is fully established on both
	// sides before we attempt the reverse direction.
	wantReady := interactionchannel.Ready{Role: "header"}
	if err := client.Send(wantReady); err != nil {
		t.Fatalf("client.Send: %v", err)
	}
	select {
	case msg := <-server.Recv():
		got, ok := msg.(interactionchannel.Ready)
		if !ok {
			t.Fatalf("server.Recv: got %T, want Ready", msg)
		}
		if got.Role != "header" {
			t.Errorf("Role: got %q, want %q", got.Role, "header")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for client→server Ready")
	}

	// Server → client: connection is now established on both sides.
	want := interactionchannel.StateHeader{
		IterationLine: "Iteration 2/5",
		StepNames:     []string{"alpha", "beta"},
		StepStates:    []int{1, 2},
	}
	if err := server.Send(want); err != nil {
		t.Fatalf("server.Send: %v", err)
	}
	select {
	case msg := <-client.Recv():
		got, ok := msg.(interactionchannel.StateHeader)
		if !ok {
			t.Fatalf("client.Recv: got %T, want StateHeader", msg)
		}
		if got.IterationLine != want.IterationLine {
			t.Errorf("IterationLine: got %q, want %q", got.IterationLine, want.IterationLine)
		}
		if len(got.StepNames) != 2 || got.StepNames[0] != "alpha" {
			t.Errorf("StepNames: got %v, want %v", got.StepNames, want.StepNames)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for server→client StateHeader")
	}
}

// TestMessageJSONRoundTrip verifies each message type's JSON round-trip preserves field values.
func TestMessageJSONRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  interactionchannel.Message
	}{
		{
			name: "Ready",
			msg:  interactionchannel.Ready{Role: "footer"},
		},
		{
			name: "Intent",
			msg:  interactionchannel.Intent{Kind: interactionchannel.IntentQuit},
		},
		{
			name: "DoneAck",
			msg:  interactionchannel.DoneAck{Role: "log"},
		},
		{
			name: "StateHeader",
			msg: interactionchannel.StateHeader{
				IterationLine: "Finalizing 3/3",
				StepNames:     []string{"step1"},
				StepStates:    []int{2},
			},
		},
		{
			name: "StateLog",
			msg:  interactionchannel.StateLog{Lines: [][]byte{[]byte("hello"), []byte("world")}},
		},
		{
			name: "StateFooter",
			msg:  interactionchannel.StateFooter{Mode: 1, ShortcutLine: "q quit"},
		},
		{
			name: "WorkspaceDone",
			msg:  interactionchannel.WorkspaceDone{ExitCode: 42},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := interactionchannel.MarshalMessage(tc.msg)
			if err != nil {
				t.Fatalf("MarshalMessage: %v", err)
			}
			// Verify "type" field is present.
			var disc struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(data, &disc); err != nil {
				t.Fatalf("unmarshal discriminator: %v", err)
			}
			if disc.Type == "" {
				t.Errorf("missing type field in %s", tc.name)
			}

			// Unmarshal and compare.
			got, err := interactionchannel.UnmarshalMessage(data)
			if err != nil {
				t.Fatalf("UnmarshalMessage: %v", err)
			}
			if got.WireType() != tc.msg.WireType() {
				t.Errorf("WireType: got %q, want %q", got.WireType(), tc.msg.WireType())
			}
			// For StateLog, verify [][]byte round-trips correctly through base64.
			if sl, ok := got.(interactionchannel.StateLog); ok {
				orig := tc.msg.(interactionchannel.StateLog)
				if !reflect.DeepEqual(sl.Lines, orig.Lines) {
					t.Errorf("StateLog.Lines: got %v, want %v", sl.Lines, orig.Lines)
				}
			}
		})
	}
}

// TestGoroutineExitOnContextCancel verifies that goroutine pairs exit cleanly
// when the context is cancelled. Not run in parallel because it reads
// runtime.NumGoroutine(), which counts all goroutines globally.
func TestGoroutineExitOnContextCancel(t *testing.T) {
	sock := sockPath(t)
	ctx, cancel := context.WithCancel(context.Background())

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}

	client, err := interactionchannel.Dial(ctx, sock, "log")
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// Exchange one message to prove goroutines on both sides are running before
	// we capture the "during" count. This avoids the race where goroutines
	// are started but not yet scheduled.
	if err := client.Send(interactionchannel.Ready{Role: "log"}); err != nil {
		t.Fatalf("client.Send: %v", err)
	}
	select {
	case <-server.Recv():
	case <-time.After(2 * time.Second):
		t.Fatal("timeout proving goroutines are running")
	}

	during := runtime.NumGoroutine()

	// Cancel context and close → all goroutines should exit.
	cancel()
	server.Close()
	client.Close()

	// Poll until goroutine count drops below "during" or we time out.
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
		t.Errorf("goroutine leak: during=%d final=%d (goroutines did not decrease)", during, final)
	}
}

// TestForwardCompatibilityExtraFields verifies that an unknown field in the JSON
// does not cause UnmarshalMessage to fail, and the known fields are preserved.
func TestForwardCompatibilityExtraFields(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"type":"ready","role":"header","future_field":"ignored"}`)
	msg, err := interactionchannel.UnmarshalMessage(raw)
	if err != nil {
		t.Fatalf("UnmarshalMessage with extra field: %v", err)
	}
	got, ok := msg.(interactionchannel.Ready)
	if !ok {
		t.Fatalf("got %T, want Ready", msg)
	}
	if got.Role != "header" {
		t.Errorf("Role: got %q, want %q", got.Role, "header")
	}
}

// TestStaleSocketUnlinkedBeforeBind verifies that Serve removes a stale socket
// file before binding.
func TestStaleSocketUnlinkedBeforeBind(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)

	// Create a stale regular file at the socket path.
	if err := os.WriteFile(sock, []byte("stale"), 0o600); err != nil {
		t.Fatalf("create stale file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve (stale socket): %v", err)
	}
	defer server.Close()
}

// TestAllIntentTypes verifies IntentType constants are defined.
func TestAllIntentTypes(t *testing.T) {
	t.Parallel()

	types := []interactionchannel.IntentType{
		interactionchannel.IntentRetry,
		interactionchannel.IntentContinue,
		interactionchannel.IntentQuit,
		interactionchannel.IntentSkip,
		interactionchannel.IntentNext,
	}
	for _, it := range types {
		if it == "" {
			t.Errorf("IntentType constant is empty string")
		}
	}
}

// TestSendAfterClose verifies that Send returns an error (not a panic) after
// Close has been called and the connection slice is empty.
func TestSendAfterClose(t *testing.T) {
	t.Parallel()
	sock := sockPath(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := interactionchannel.Serve(ctx, sock)
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	server.Close()

	err = server.Send(interactionchannel.StateHeader{IterationLine: "x"})
	if err == nil {
		t.Error("Send after Close: expected non-nil error, got nil")
	}
}
