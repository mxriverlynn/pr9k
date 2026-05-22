package cmuxctl_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
)

const (
	testTimeout  = 2 * time.Second
	shortTimeout = 50 * time.Millisecond
)

// ---- FakeClient tests -------------------------------------------------------

func TestFakeClient_ScriptedSystemIdentify(t *testing.T) {
	f := &cmuxctl.FakeClient{
		SystemIdentifyFunc: func(_ context.Context) (cmuxctl.Identity, error) {
			return cmuxctl.Identity{SocketPath: "/run/x.sock"}, nil
		},
	}
	id, err := f.SystemIdentify(context.Background())
	if err != nil || id.SocketPath != "/run/x.sock" {
		t.Fatalf("got %+v err=%v", id, err)
	}
}

func TestFakeClient_DefaultsDriveRunPhase1(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	id, _ := f.SystemIdentify(context.Background())
	if id.SocketPath == "" {
		t.Error("default SystemIdentify must report a socket_path so preflight passes")
	}
	ws, err := f.WorkspaceCreate(context.Background(), cmuxctl.WorkspaceCreateOpts{Title: "t"})
	if err != nil || ws.Empty() {
		t.Errorf("default WorkspaceCreate must return a usable handle: %+v err=%v", ws, err)
	}
	if len(f.CreateCalls) != 1 || f.CreateCalls[0].Title != "t" {
		t.Errorf("CreateCalls not recorded: %+v", f.CreateCalls)
	}
}

func TestFakeClient_RecordsHandleCalls(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	ws := cmuxctl.Workspace{ID: "w1", Ref: "workspace:1"}
	_ = f.WorkspaceClose(context.Background(), ws)
	_ = f.WorkspaceSelect(context.Background(), ws)
	_, _ = f.SurfaceSplit(context.Background(), cmuxctl.SplitOpts{Direction: cmuxctl.SplitUp})
	if len(f.CloseCalls) != 1 || f.CloseCalls[0].ID != "w1" {
		t.Errorf("CloseCalls = %+v", f.CloseCalls)
	}
	if len(f.SelectCalls) != 1 || f.SelectCalls[0].Ref != "workspace:1" {
		t.Errorf("SelectCalls = %+v", f.SelectCalls)
	}
	if len(f.SplitCalls) != 1 || f.SplitCalls[0].Direction != cmuxctl.SplitUp {
		t.Errorf("SplitCalls = %+v", f.SplitCalls)
	}
}

func TestFakeClient_HangNextRelease(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	next := make(chan struct{}, 1)
	release := make(chan struct{}, 1)
	f.SetHangChannels(next, release)
	next <- struct{}{}

	done := make(chan error, 1)
	go func() { _, err := f.WorkspaceCurrent(context.Background()); done <- err }()
	select {
	case <-done:
		t.Fatal("call returned before HangRelease")
	case <-time.After(50 * time.Millisecond):
	}
	release <- struct{}{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("call did not return after HangRelease")
	}
}

func TestFakeClient_HangContextCancel(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	next := make(chan struct{}, 1)
	release := make(chan struct{}, 1)
	f.SetHangChannels(next, release)
	next <- struct{}{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := f.WorkspaceCurrent(ctx); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("call did not unblock on context cancel")
	}
}

func TestFakeClient_ConcurrentRecorders(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = f.WorkspaceCreate(context.Background(), cmuxctl.WorkspaceCreateOpts{Title: "ws"})
		}()
	}
	wg.Wait()
	if len(f.CreateCalls) != 10 {
		t.Errorf("CreateCalls len = %d, want 10", len(f.CreateCalls))
	}
}

// ---- RealClient v2 wire tests ----------------------------------------------

type rpcMsg struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
	ID     int64           `json:"id"`
}

// v2Server starts a Unix listener whose handler is invoked per request line and
// whose return string is written verbatim (the test controls framing).
func v2Server(t *testing.T, handle func(req rpcMsg) string) string {
	t.Helper()
	socketPath := filepath.Join(socketTempDir(t), "cmux.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				dec := json.NewDecoder(c)
				for {
					var req rpcMsg
					if err := dec.Decode(&req); err != nil {
						return
					}
					_, _ = c.Write([]byte(handle(req) + "\n"))
				}
			}(conn)
		}
	}()
	return socketPath
}

func okResp(id int64, result string) string {
	return `{"id":` + itoa(id) + `,"ok":true,"result":` + result + `}`
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func hangingServer(t *testing.T) string {
	t.Helper()
	socketPath := filepath.Join(socketTempDir(t), "cmux.sock")
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn
		}
	}()
	return socketPath
}

func TestRealClient_SystemIdentify_V2Success(t *testing.T) {
	sock := v2Server(t, func(req rpcMsg) string {
		return okResp(req.ID, `{"socket_path":"/run/cmux.sock","focused":null,"caller":null}`)
	})
	c := cmuxctl.NewRealClient(sock, testTimeout)
	defer c.Stop()
	id, err := c.SystemIdentify(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.SocketPath != "/run/cmux.sock" {
		t.Errorf("SocketPath = %q, want /run/cmux.sock", id.SocketPath)
	}
}

func TestRealClient_WorkspaceCreate_ReturnsHandle(t *testing.T) {
	sock := v2Server(t, func(req rpcMsg) string {
		return okResp(req.ID, `{"workspace_id":"uuid-1","workspace_ref":"workspace:1"}`)
	})
	c := cmuxctl.NewRealClient(sock, testTimeout)
	defer c.Stop()
	ws, err := c.WorkspaceCreate(context.Background(), cmuxctl.WorkspaceCreateOpts{Title: "t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.ID != "uuid-1" || ws.Ref != "workspace:1" {
		t.Errorf("handle = %+v", ws)
	}
}

func TestRealClient_WorkspaceList_V2Shape(t *testing.T) {
	// cmux's workspace.list summary objects use "id"/"ref" (verified at
	// 2f96c15c2, v2WorkspaceSummaryPayload) — not workspace_id/workspace_ref.
	sock := v2Server(t, func(req rpcMsg) string {
		return okResp(req.ID, `{"workspaces":[{"id":"a","ref":"workspace:1"},{"id":"b","ref":"workspace:2"}]}`)
	})
	c := cmuxctl.NewRealClient(sock, testTimeout)
	defer c.Stop()
	got, err := c.WorkspaceList(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].Ref != "workspace:2" {
		t.Errorf("got %+v", got)
	}
}

func TestRealClient_SurfaceSplit_ReturnsSurface(t *testing.T) {
	sock := v2Server(t, func(req rpcMsg) string {
		return okResp(req.ID, `{"surface_id":"s1","surface_ref":"surface:1","pane_id":"p1","pane_ref":"pane:1"}`)
	})
	c := cmuxctl.NewRealClient(sock, testTimeout)
	defer c.Stop()
	s, err := c.SurfaceSplit(context.Background(), cmuxctl.SplitOpts{Direction: cmuxctl.SplitDown})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SurfaceID != "s1" || s.PaneRef != "pane:1" {
		t.Errorf("surface = %+v", s)
	}
}

func TestRealClient_V2Error_SurfacedAsCmuxError(t *testing.T) {
	sock := v2Server(t, func(req rpcMsg) string {
		return `{"id":` + itoa(req.ID) + `,"ok":false,"error":{"code":"not_found","message":"Workspace not found"}}`
	})
	c := cmuxctl.NewRealClient(sock, testTimeout)
	defer c.Stop()
	_, err := c.WorkspaceList(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var ce *cmuxctl.CmuxError
	if !errors.As(err, &ce) {
		t.Fatalf("want *CmuxError, got %T: %v", err, err)
	}
	if ce.Code != "not_found" || !strings.Contains(ce.Message, "not found") {
		t.Errorf("CmuxError = %+v", ce)
	}
}

func TestRealClient_PlaintextError_ClassifiedNotJSONDecodeError(t *testing.T) {
	// cmux's cmuxOnly rejection: a bare "ERROR: …" line before any JSON.
	sock := v2Server(t, func(_ rpcMsg) string {
		return "ERROR: Access denied — only processes started inside cmux can connect"
	})
	c := cmuxctl.NewRealClient(sock, testTimeout)
	defer c.Stop()
	_, err := c.SystemIdentify(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var pt *cmuxctl.PlaintextError
	if !errors.As(err, &pt) {
		t.Fatalf("want *PlaintextError, got %T: %v", err, err)
	}
	if !pt.IsAccessDenied() {
		t.Errorf("IsAccessDenied()=false for %q", pt.Raw)
	}
	if strings.Contains(err.Error(), "invalid character") {
		t.Errorf("must not surface a raw JSON decode error: %v", err)
	}
}

func TestRealClient_TimeoutFiresOnHangingServer(t *testing.T) {
	c := cmuxctl.NewRealClient(hangingServer(t), shortTimeout)
	defer c.Stop()
	start := time.Now()
	_, err := c.WorkspaceList(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("timeout took too long: %s", elapsed)
	}
}

func TestRealClient_TimeoutReturnsTypedTimeoutError(t *testing.T) {
	c := cmuxctl.NewRealClient(hangingServer(t), shortTimeout)
	defer c.Stop()
	_, err := c.WorkspaceList(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
	var te *cmuxctl.TimeoutError
	if !errors.As(err, &te) {
		t.Fatalf("want *cmuxctl.TimeoutError via errors.As, got %T: %v", err, err)
	}
	if te.Method == "" {
		t.Error("TimeoutError.Method must be non-empty")
	}
	if te.Duration == 0 {
		t.Error("TimeoutError.Duration must be non-zero")
	}
}

func TestTimeoutError_ErrorMessageFormat(t *testing.T) {
	e := &cmuxctl.TimeoutError{Method: "workspace.list", Duration: 8 * time.Second}
	want := "cmuxctl: workspace.list timed out after 8s"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestRealClient_SerializesRequests(t *testing.T) {
	var mu sync.Mutex
	concurrent, maxConcurrent := 0, 0
	sock := v2Server(t, func(req rpcMsg) string {
		mu.Lock()
		concurrent++
		if concurrent > maxConcurrent {
			maxConcurrent = concurrent
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
		mu.Lock()
		concurrent--
		mu.Unlock()
		return okResp(req.ID, `{"workspaces":[]}`)
	})
	c := cmuxctl.NewRealClient(sock, testTimeout)
	defer c.Stop()
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.WorkspaceList(context.Background())
		}()
	}
	wg.Wait()
	if maxConcurrent != 1 {
		t.Errorf("maxConcurrent = %d, want 1 (RealClient must serialize)", maxConcurrent)
	}
}

func TestRealClient_ContextCancelBeforeSend(t *testing.T) {
	c := cmuxctl.NewRealClient(hangingServer(t), testTimeout)
	defer c.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.WorkspaceList(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestRealClient_StopIdempotent(t *testing.T) {
	c := cmuxctl.NewRealClient(hangingServer(t), testTimeout)
	c.Stop()
	c.Stop()
}

func TestRealClient_WireMethodAndID(t *testing.T) {
	got := make(chan rpcMsg, 1)
	sock := v2Server(t, func(req rpcMsg) string {
		select {
		case got <- req:
		default:
		}
		return okResp(req.ID, `{"workspaces":[]}`)
	})
	c := cmuxctl.NewRealClient(sock, testTimeout)
	defer c.Stop()
	if _, err := c.WorkspaceList(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case req := <-got:
		if req.Method != "workspace.list" {
			t.Errorf("method = %q, want workspace.list", req.Method)
		}
		if req.ID == 0 {
			t.Errorf("id must be non-zero")
		}
	case <-time.After(time.Second):
		t.Fatal("server never received the request")
	}
}

// ---- RealClient sidebar round-trip tests (R1: wrong wire names caught) ------

func TestRealClient_WorkspaceSetStatus_WireMethod(t *testing.T) {
	ws := cmuxctl.Workspace{ID: "ws-uuid-1", Ref: "workspace:1"}
	got := make(chan rpcMsg, 1)
	sock := v2Server(t, func(req rpcMsg) string {
		select {
		case got <- req:
		default:
		}
		return okResp(req.ID, `{}`)
	})
	c := cmuxctl.NewRealClient(sock, testTimeout)
	defer c.Stop()
	if err := c.WorkspaceSetStatus(context.Background(), ws, "pr9k.step", "feature work"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case req := <-got:
		if req.Method != "workspace.set_status" {
			t.Errorf("method = %q, want workspace.set_status", req.Method)
		}
		if !strings.Contains(string(req.Params), `"workspace_id":"ws-uuid-1"`) {
			t.Errorf("params must include workspace_id: %s", req.Params)
		}
		if !strings.Contains(string(req.Params), `"key":"pr9k.step"`) {
			t.Errorf("params must include key: %s", req.Params)
		}
		if !strings.Contains(string(req.Params), `"value":"feature work"`) {
			t.Errorf("params must include value: %s", req.Params)
		}
	case <-time.After(time.Second):
		t.Fatal("server never received the request")
	}
}

func TestRealClient_WorkspaceClearStatus_WireMethod(t *testing.T) {
	ws := cmuxctl.Workspace{ID: "ws-uuid-2", Ref: "workspace:2"}
	got := make(chan rpcMsg, 1)
	sock := v2Server(t, func(req rpcMsg) string {
		select {
		case got <- req:
		default:
		}
		return okResp(req.ID, `{}`)
	})
	c := cmuxctl.NewRealClient(sock, testTimeout)
	defer c.Stop()
	if err := c.WorkspaceClearStatus(context.Background(), ws, "pr9k.step"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case req := <-got:
		if req.Method != "workspace.clear_status" {
			t.Errorf("method = %q, want workspace.clear_status", req.Method)
		}
		if !strings.Contains(string(req.Params), `"workspace_id":"ws-uuid-2"`) {
			t.Errorf("params must include workspace_id: %s", req.Params)
		}
		if !strings.Contains(string(req.Params), `"key":"pr9k.step"`) {
			t.Errorf("params must include key: %s", req.Params)
		}
	case <-time.After(time.Second):
		t.Fatal("server never received the request")
	}
}

func TestRealClient_WorkspaceSetProgress_WireMethod(t *testing.T) {
	ws := cmuxctl.Workspace{ID: "ws-uuid-3", Ref: "workspace:3"}
	got := make(chan rpcMsg, 1)
	sock := v2Server(t, func(req rpcMsg) string {
		select {
		case got <- req:
		default:
		}
		return okResp(req.ID, `{}`)
	})
	c := cmuxctl.NewRealClient(sock, testTimeout)
	defer c.Stop()
	if err := c.WorkspaceSetProgress(context.Background(), ws, 0.5, "2 / 4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case req := <-got:
		if req.Method != "workspace.set_progress" {
			t.Errorf("method = %q, want workspace.set_progress", req.Method)
		}
		if !strings.Contains(string(req.Params), `"workspace_id":"ws-uuid-3"`) {
			t.Errorf("params must include workspace_id: %s", req.Params)
		}
		if !strings.Contains(string(req.Params), `"fraction":0.5`) {
			t.Errorf("params must include fraction: %s", req.Params)
		}
		if !strings.Contains(string(req.Params), `"label":"2 / 4"`) {
			t.Errorf("params must include label: %s", req.Params)
		}
	case <-time.After(time.Second):
		t.Fatal("server never received the request")
	}
}

func TestRealClient_WorkspaceClearProgress_WireMethod(t *testing.T) {
	ws := cmuxctl.Workspace{ID: "ws-uuid-4", Ref: "workspace:4"}
	got := make(chan rpcMsg, 1)
	sock := v2Server(t, func(req rpcMsg) string {
		select {
		case got <- req:
		default:
		}
		return okResp(req.ID, `{}`)
	})
	c := cmuxctl.NewRealClient(sock, testTimeout)
	defer c.Stop()
	if err := c.WorkspaceClearProgress(context.Background(), ws); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case req := <-got:
		if req.Method != "workspace.clear_progress" {
			t.Errorf("method = %q, want workspace.clear_progress", req.Method)
		}
		if !strings.Contains(string(req.Params), `"workspace_id":"ws-uuid-4"`) {
			t.Errorf("params must include workspace_id: %s", req.Params)
		}
	case <-time.After(time.Second):
		t.Fatal("server never received the request")
	}
}

// ---- FakeClient recorder tests (sidebar methods) ----------------------------

func TestFakeClient_WorkspaceSetStatus_Records(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	ws := cmuxctl.Workspace{ID: "w1", Ref: "workspace:1"}
	if err := f.WorkspaceSetStatus(context.Background(), ws, "pr9k.step", "feature work"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := f.WorkspaceSetStatus(context.Background(), ws, "pr9k.step", "test writing"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.SetStatusCalls) != 2 {
		t.Fatalf("SetStatusCalls len = %d, want 2", len(f.SetStatusCalls))
	}
	if f.SetStatusCalls[0].Key != "pr9k.step" || f.SetStatusCalls[0].Value != "feature work" {
		t.Errorf("SetStatusCalls[0] = %+v", f.SetStatusCalls[0])
	}
	if f.SetStatusCalls[1].Value != "test writing" {
		t.Errorf("SetStatusCalls[1].Value = %q, want test writing", f.SetStatusCalls[1].Value)
	}
}

func TestFakeClient_WorkspaceClearStatus_Records(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	ws := cmuxctl.Workspace{ID: "w1", Ref: "workspace:1"}
	if err := f.WorkspaceClearStatus(context.Background(), ws, "pr9k.step"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.ClearStatusCalls) != 1 {
		t.Fatalf("ClearStatusCalls len = %d, want 1", len(f.ClearStatusCalls))
	}
	if f.ClearStatusCalls[0].Key != "pr9k.step" || f.ClearStatusCalls[0].Workspace.ID != "w1" {
		t.Errorf("ClearStatusCalls[0] = %+v", f.ClearStatusCalls[0])
	}
}

func TestFakeClient_WorkspaceSetProgress_Records(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	ws := cmuxctl.Workspace{ID: "w2", Ref: "workspace:2"}
	if err := f.WorkspaceSetProgress(context.Background(), ws, 0.25, "1 / 4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := f.WorkspaceSetProgress(context.Background(), ws, 0.75, "3 / 4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.SetProgressCalls) != 2 {
		t.Fatalf("SetProgressCalls len = %d, want 2", len(f.SetProgressCalls))
	}
	if f.SetProgressCalls[0].Fraction != 0.25 || f.SetProgressCalls[0].Label != "1 / 4" {
		t.Errorf("SetProgressCalls[0] = %+v", f.SetProgressCalls[0])
	}
	if f.SetProgressCalls[1].Fraction != 0.75 || f.SetProgressCalls[1].Label != "3 / 4" {
		t.Errorf("SetProgressCalls[1] = %+v", f.SetProgressCalls[1])
	}
}

func TestFakeClient_WorkspaceClearProgress_Records(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	ws := cmuxctl.Workspace{ID: "w3", Ref: "workspace:3"}
	if err := f.WorkspaceClearProgress(context.Background(), ws); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.ClearProgressCalls) != 1 {
		t.Fatalf("ClearProgressCalls len = %d, want 1", len(f.ClearProgressCalls))
	}
	if f.ClearProgressCalls[0].ID != "w3" {
		t.Errorf("ClearProgressCalls[0].ID = %q, want w3", f.ClearProgressCalls[0].ID)
	}
}
