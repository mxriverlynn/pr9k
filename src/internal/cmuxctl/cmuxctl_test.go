package cmuxctl_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
)

// testTimeout is the per-call timeout used for RealClient tests that expect
// success; short enough to keep the suite fast.
const testTimeout = 2 * time.Second

// shortTimeout triggers a timeout quickly against a hanging mock server.
const shortTimeout = 50 * time.Millisecond

// ---- FakeClient tests -------------------------------------------------------

func TestFakeClient_ScriptedSystemIdentify(t *testing.T) {
	f := &cmuxctl.FakeClient{
		SystemIdentifyFunc: func(_ context.Context) (cmuxctl.Identity, error) {
			return cmuxctl.Identity{Name: "cmux", Version: "0.64.6"}, nil
		},
	}
	id, err := f.SystemIdentify(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Name != "cmux" || id.Version != "0.64.6" {
		t.Errorf("got %+v, want {Name:cmux Version:0.64.6}", id)
	}
}

func TestFakeClient_ScriptedWorkspaceList(t *testing.T) {
	want := []string{"alpha", "beta"}
	f := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(_ context.Context) ([]string, error) {
			return want, nil
		},
	}
	got, err := f.WorkspaceList(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFakeClient_ScriptedSurfaceList(t *testing.T) {
	want := []cmuxctl.PaneInfo{{ID: "p1", Exited: false}, {ID: "p2", Exited: true}}
	f := &cmuxctl.FakeClient{
		SurfaceListFunc: func(_ context.Context, ws string) ([]cmuxctl.PaneInfo, error) {
			if ws != "my-ws" {
				t.Errorf("unexpected workspace name: %q", ws)
			}
			return want, nil
		},
	}
	got, err := f.SurfaceList(context.Background(), "my-ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "p1" || got[1].Exited != true {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestFakeClient_RecordsCreateCalls(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	if err := f.WorkspaceCreate(context.Background(), "my-ws"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.CreateCalls) != 1 || f.CreateCalls[0] != "my-ws" {
		t.Errorf("CreateCalls = %v, want [my-ws]", f.CreateCalls)
	}
}

func TestFakeClient_RecordsCloseCalls(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	if err := f.WorkspaceClose(context.Background(), "my-ws"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.CloseCalls) != 1 || f.CloseCalls[0] != "my-ws" {
		t.Errorf("CloseCalls = %v, want [my-ws]", f.CloseCalls)
	}
}

func TestFakeClient_RecordsSelectCalls(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	if err := f.WorkspaceSelect(context.Background(), "my-ws"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.SelectCalls) != 1 || f.SelectCalls[0] != "my-ws" {
		t.Errorf("SelectCalls = %v, want [my-ws]", f.SelectCalls)
	}
}

func TestFakeClient_RecordsSpawnCalls(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	argv := []string{"sh", "-c", "echo hi"}
	if err := f.SurfaceSpawn(context.Background(), "p1", argv); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.SpawnCalls) != 1 || f.SpawnCalls[0].PaneID != "p1" {
		t.Errorf("SpawnCalls = %v", f.SpawnCalls)
	}
}

func TestFakeClient_RecordsHideCalls(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	if err := f.SurfaceHide(context.Background(), "p1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.HideCalls) != 1 || f.HideCalls[0] != "p1" {
		t.Errorf("HideCalls = %v, want [p1]", f.HideCalls)
	}
}

func TestFakeClient_RecordsSplitCalls(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	opts := cmuxctl.SplitOpts{PaneID: "parent"}
	if _, err := f.SurfaceSplit(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.SplitCalls) != 1 || f.SplitCalls[0].PaneID != "parent" {
		t.Errorf("SplitCalls = %v", f.SplitCalls)
	}
}

func TestFakeClient_HangNextRelease(t *testing.T) {
	f := &cmuxctl.FakeClient{
		HangNext:    make(chan struct{}, 1),
		HangRelease: make(chan struct{}),
	}
	f.HangNext <- struct{}{} // queue one hang

	done := make(chan error, 1)
	go func() {
		done <- f.WorkspaceCreate(context.Background(), "ws")
	}()

	// Give the goroutine time to reach the hang point.
	time.Sleep(20 * time.Millisecond)

	// Verify it is still blocked.
	select {
	case <-done:
		t.Fatal("WorkspaceCreate returned before HangRelease; hang did not work")
	default:
	}

	// Release and expect success.
	f.HangRelease <- struct{}{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WorkspaceCreate did not return after HangRelease")
	}
}

func TestFakeClient_HangContextCancel(t *testing.T) {
	f := &cmuxctl.FakeClient{
		HangNext:    make(chan struct{}, 1),
		HangRelease: make(chan struct{}),
	}
	f.HangNext <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- f.WorkspaceCreate(ctx, "ws")
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WorkspaceCreate did not return after context cancel")
	}
}

func TestFakeClient_DefaultZeroValues(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	ctx := context.Background()

	if id, err := f.SystemIdentify(ctx); err != nil || id.Name != "" {
		t.Errorf("SystemIdentify: got (%+v, %v)", id, err)
	}
	if s, err := f.WorkspaceCurrent(ctx); err != nil || s != "" {
		t.Errorf("WorkspaceCurrent: got (%q, %v)", s, err)
	}
	if ss, err := f.WorkspaceList(ctx); err != nil || ss != nil {
		t.Errorf("WorkspaceList: got (%v, %v)", ss, err)
	}
	if err := f.WorkspaceCreate(ctx, "ws"); err != nil {
		t.Errorf("WorkspaceCreate: %v", err)
	}
	if err := f.WorkspaceClose(ctx, "ws"); err != nil {
		t.Errorf("WorkspaceClose: %v", err)
	}
	if err := f.WorkspaceSelect(ctx, "ws"); err != nil {
		t.Errorf("WorkspaceSelect: %v", err)
	}
	if s, err := f.SurfaceSplit(ctx, cmuxctl.SplitOpts{}); err != nil || s != "" {
		t.Errorf("SurfaceSplit: got (%q, %v)", s, err)
	}
	if err := f.SurfaceSpawn(ctx, "p1", []string{"sh"}); err != nil {
		t.Errorf("SurfaceSpawn: %v", err)
	}
	if err := f.SurfaceHide(ctx, "p1"); err != nil {
		t.Errorf("SurfaceHide: %v", err)
	}
	if pp, err := f.SurfaceList(ctx, "ws"); err != nil || pp != nil {
		t.Errorf("SurfaceList: got (%v, %v)", pp, err)
	}
}

// FakeClient operations are safe to call from multiple goroutines.
func TestFakeClient_ConcurrentRecorders(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = f.WorkspaceCreate(context.Background(), "ws")
		}()
	}
	wg.Wait()
	if len(f.CreateCalls) != 10 {
		t.Errorf("CreateCalls len = %d, want 10", len(f.CreateCalls))
	}
}

// ---- RealClient test helpers ------------------------------------------------

// rpcMsg is the shape the mock server decodes from the client.
type rpcMsg struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      int64           `json:"id"`
}

// hangingServer starts a Unix listener that accepts connections but never reads
// or writes anything (simulates a cmux that hangs on RPCs).
func hangingServer(t *testing.T) string {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "cmux.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn // hold open; never read/write
		}
	}()
	return socketPath
}

// respondingServer starts a Unix listener that returns `result` as the JSON-RPC
// result for every request. It handles multiple requests per connection.
func respondingServer(t *testing.T, result json.RawMessage) string {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "cmux.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				dec := json.NewDecoder(c)
				enc := json.NewEncoder(c)
				for {
					var req rpcMsg
					if err := dec.Decode(&req); err != nil {
						return
					}
					_ = enc.Encode(map[string]any{
						"jsonrpc": "2.0",
						"result":  result,
						"id":      req.ID,
					})
				}
			}(conn)
		}
	}()
	return socketPath
}

// ---- RealClient tests -------------------------------------------------------

func TestRealClient_TimeoutFiresOnHangingServer(t *testing.T) {
	socketPath := hangingServer(t)
	c := cmuxctl.NewRealClient(socketPath, shortTimeout)
	defer c.Stop()

	start := time.Now()
	_, err := c.WorkspaceList(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed < shortTimeout {
		t.Errorf("elapsed %s < shortTimeout %s; timeout did not fire", elapsed, shortTimeout)
	}
	// Allow up to 5× for scheduler jitter.
	if elapsed > 5*shortTimeout {
		t.Errorf("elapsed %s >> shortTimeout %s; client took too long", elapsed, shortTimeout)
	}
}

func TestRealClient_TimeoutClosesSocket(t *testing.T) {
	socketPath := hangingServer(t)
	c := cmuxctl.NewRealClient(socketPath, shortTimeout)
	defer c.Stop()

	// First call: times out and closes the socket.
	_, err := c.WorkspaceList(context.Background())
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Subsequent call: re-dials; the server still hangs so it times out again.
	// We verify it returns an error (not nil), proving the closed socket did not
	// silently succeed.
	_, err2 := c.WorkspaceList(context.Background())
	if err2 == nil {
		t.Fatal("expected error on subsequent call after timeout, got nil")
	}
}

func TestRealClient_SuccessfulRPC(t *testing.T) {
	result := json.RawMessage(`["ws1","ws2"]`)
	socketPath := respondingServer(t, result)
	c := cmuxctl.NewRealClient(socketPath, testTimeout)
	defer c.Stop()

	got, err := c.WorkspaceList(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "ws1" || got[1] != "ws2" {
		t.Errorf("got %v, want [ws1 ws2]", got)
	}
}

func TestRealClient_SurfaceListResponse(t *testing.T) {
	result := json.RawMessage(`[{"id":"p1","exited":false},{"id":"p2","exited":true}]`)
	socketPath := respondingServer(t, result)
	c := cmuxctl.NewRealClient(socketPath, testTimeout)
	defer c.Stop()

	got, err := c.SurfaceList(context.Background(), "my-ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "p1" || got[1].Exited != true {
		t.Errorf("got %+v", got)
	}
}

func TestRealClient_SystemIdentifyResponse(t *testing.T) {
	result := json.RawMessage(`{"name":"cmux","version":"0.64.6"}`)
	socketPath := respondingServer(t, result)
	c := cmuxctl.NewRealClient(socketPath, testTimeout)
	defer c.Stop()

	id, err := c.SystemIdentify(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.Name != "cmux" || id.Version != "0.64.6" {
		t.Errorf("got %+v", id)
	}
}

func TestRealClient_SurfaceSplitResponse(t *testing.T) {
	result := json.RawMessage(`{"pane_id":"abc123"}`)
	socketPath := respondingServer(t, result)
	c := cmuxctl.NewRealClient(socketPath, testTimeout)
	defer c.Stop()

	paneID, err := c.SurfaceSplit(context.Background(), cmuxctl.SplitOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if paneID != "abc123" {
		t.Errorf("paneID = %q, want %q", paneID, "abc123")
	}
}

// TestRealClient_SerializesRequests verifies the single-goroutine queue
// serializes concurrent callers — no two requests overlap on the wire.
// The race detector validates there are no data races.
func TestRealClient_SerializesRequests(t *testing.T) {
	var mu sync.Mutex
	var order []int64

	socketPath := filepath.Join(t.TempDir(), "cmux.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	// Single-connection serial server: one request, one response, in order.
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		dec := json.NewDecoder(conn)
		enc := json.NewEncoder(conn)
		for {
			var req rpcMsg
			if err := dec.Decode(&req); err != nil {
				return
			}
			mu.Lock()
			order = append(order, req.ID)
			mu.Unlock()
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"result":  []string{},
				"id":      req.ID,
			})
		}
	}()

	c := cmuxctl.NewRealClient(socketPath, testTimeout)
	defer c.Stop()

	const n = 5
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.WorkspaceList(context.Background())
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(order) != n {
		t.Errorf("expected %d serialized RPCs, got %d; order=%v", n, len(order), order)
	}
	// Each request ID must be unique (no double-sends on the shared connection).
	seen := make(map[int64]bool, n)
	for _, id := range order {
		if seen[id] {
			t.Errorf("duplicate request ID %d in order %v", id, order)
		}
		seen[id] = true
	}
}

func TestRealClient_ContextCancelBeforeSend(t *testing.T) {
	socketPath := hangingServer(t)
	c := cmuxctl.NewRealClient(socketPath, testTimeout)
	defer c.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err := c.WorkspaceList(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRealClient_ContextCancelAfterSend(t *testing.T) {
	// A server that blocks until its context is cancelled (simulates slow response).
	socketPath := hangingServer(t)
	c := cmuxctl.NewRealClient(socketPath, testTimeout)
	defer c.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := c.WorkspaceList(ctx)
	if err == nil {
		t.Fatal("expected error from context timeout, got nil")
	}
}

// TP-219-001: RealClient surfaces a JSON-RPC error payload as a Go error,
// and the connection is NOT closed on a protocol-level error (only on
// transport errors), so a subsequent successful call on the same connection
// succeeds.
func TestRealClient_RPCErrorSurfacedAsGoError(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "cmux.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	// Server: first request → JSON-RPC error; subsequent requests → result.
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		dec := json.NewDecoder(conn)
		enc := json.NewEncoder(conn)
		first := true
		for {
			var req rpcMsg
			if err := dec.Decode(&req); err != nil {
				return
			}
			if first {
				first = false
				_ = enc.Encode(map[string]any{
					"jsonrpc": "2.0",
					"error":   map[string]any{"code": -32000, "message": "boom"},
					"id":      req.ID,
				})
			} else {
				_ = enc.Encode(map[string]any{
					"jsonrpc": "2.0",
					"result":  json.RawMessage(`null`),
					"id":      req.ID,
				})
			}
		}
	}()

	c := cmuxctl.NewRealClient(socketPath, testTimeout)
	defer c.Stop()

	// First call: must return a non-nil error containing "cmuxctl:" and "boom".
	err = c.WorkspaceCreate(context.Background(), "ws")
	if err == nil {
		t.Fatal("expected error from JSON-RPC error response, got nil")
	}
	if !contains(err.Error(), "cmuxctl:") {
		t.Errorf("error %q missing package prefix \"cmuxctl:\"", err.Error())
	}
	if !contains(err.Error(), "boom") {
		t.Errorf("error %q missing message \"boom\"", err.Error())
	}

	// Second call: must succeed — protocol errors must not disconnect the socket.
	err = c.WorkspaceClose(context.Background(), "ws")
	if err != nil {
		t.Errorf("expected nil error on follow-up call after RPC error, got: %v", err)
	}
}

// contains is a simple substring check to avoid importing strings in the test.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}())
}

// TP-219-002: RealClient.Stop() is idempotent — calling it twice must not
// panic, deadlock, or hang.
func TestRealClient_StopIdempotent(t *testing.T) {
	result := json.RawMessage(`["ws1"]`)
	socketPath := respondingServer(t, result)
	c := cmuxctl.NewRealClient(socketPath, testTimeout)

	c.Stop()
	// Second Stop must return promptly without panic.
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.Stop()
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("second Stop() did not return within 1s — possible deadlock")
	}
}

// TP-219-003: RealClient encodes the correct method name and params on the wire.
// WorkspaceCreate → method "workspace.create", params {"name":"my-ws"}.
func TestRealClient_WireMethodAndParams(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "cmux.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	captured := make(chan rpcMsg, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		dec := json.NewDecoder(conn)
		enc := json.NewEncoder(conn)
		var req rpcMsg
		if err := dec.Decode(&req); err != nil {
			return
		}
		captured <- req
		_ = enc.Encode(map[string]any{
			"jsonrpc": "2.0",
			"result":  json.RawMessage(`null`),
			"id":      req.ID,
		})
	}()

	c := cmuxctl.NewRealClient(socketPath, testTimeout)
	defer c.Stop()

	if err := c.WorkspaceCreate(context.Background(), "my-ws"); err != nil {
		t.Fatalf("WorkspaceCreate: %v", err)
	}

	select {
	case req := <-captured:
		if req.Method != "workspace.create" {
			t.Errorf("method = %q, want %q", req.Method, "workspace.create")
		}
		var params struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			t.Fatalf("unmarshal params: %v", err)
		}
		if params.Name != "my-ws" {
			t.Errorf("params.Name = %q, want %q", params.Name, "my-ws")
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for captured request")
	}
}
