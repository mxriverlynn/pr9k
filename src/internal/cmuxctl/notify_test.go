package cmuxctl_test

import (
	"context"
	"sync"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
)

// ---- B1: NotificationClass constants exist ----------------------------------

func TestNotificationClass_Constants(t *testing.T) {
	// All three constants must be non-empty and distinct.
	classes := []cmuxctl.NotificationClass{
		cmuxctl.NotificationCompletion,
		cmuxctl.NotificationRunAborted,
		cmuxctl.NotificationErrorMode,
	}
	seen := map[cmuxctl.NotificationClass]bool{}
	for _, c := range classes {
		if c == "" {
			t.Errorf("NotificationClass constant is empty")
		}
		if seen[c] {
			t.Errorf("duplicate NotificationClass value %q", c)
		}
		seen[c] = true
	}
}

// ---- B2: CmuxClient interface satisfied by RealClient (compile-time) --------
// This is enforced by the existing var _ in real.go; the test here is
// documentation that WorkspaceNotify must be on the interface.

var _ cmuxctl.CmuxClient = (*cmuxctl.FakeClient)(nil)

// ---- B3: FakeClient records NotifyCalls in order ---------------------------

func TestFakeClient_WorkspaceNotify_Records(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	ctx := context.Background()

	ws1 := cmuxctl.Workspace{ID: "ws-1", Ref: "workspace:1"}
	ws2 := cmuxctl.Workspace{ID: "ws-2", Ref: "workspace:2"}

	if err := f.WorkspaceNotify(ctx, ws1, cmuxctl.NotificationCompletion, "done"); err != nil {
		t.Fatalf("WorkspaceNotify: %v", err)
	}
	if err := f.WorkspaceNotify(ctx, ws2, cmuxctl.NotificationRunAborted, "aborted"); err != nil {
		t.Fatalf("WorkspaceNotify: %v", err)
	}

	calls := f.NotifyCalls
	if len(calls) != 2 {
		t.Fatalf("want 2 NotifyCalls, got %d", len(calls))
	}
	if calls[0].Workspace != ws1 || calls[0].Class != cmuxctl.NotificationCompletion || calls[0].Body != "done" {
		t.Errorf("call[0] = %+v", calls[0])
	}
	if calls[1].Workspace != ws2 || calls[1].Class != cmuxctl.NotificationRunAborted || calls[1].Body != "aborted" {
		t.Errorf("call[1] = %+v", calls[1])
	}
}

// ---- B4: FakeClient WorkspaceNotifyFunc is called when set -----------------

func TestFakeClient_WorkspaceNotify_Func(t *testing.T) {
	var called bool
	f := &cmuxctl.FakeClient{
		WorkspaceNotifyFunc: func(_ context.Context, ws cmuxctl.Workspace, class cmuxctl.NotificationClass, body string) error {
			called = true
			return nil
		},
	}
	if err := f.WorkspaceNotify(context.Background(), cmuxctl.Workspace{}, cmuxctl.NotificationErrorMode, "err"); err != nil {
		t.Fatalf("WorkspaceNotify: %v", err)
	}
	if !called {
		t.Error("WorkspaceNotifyFunc was not called")
	}
}

// ---- B5: FakeClient NotifyCalls is mutex-safe under concurrent writes -------

func TestFakeClient_WorkspaceNotify_ConcurrentSafe(t *testing.T) {
	f := &cmuxctl.FakeClient{}
	ctx := context.Background()
	var wg sync.WaitGroup
	const n = 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = f.WorkspaceNotify(ctx, cmuxctl.Workspace{}, cmuxctl.NotificationCompletion, "x")
		}()
	}
	wg.Wait()
	if len(f.NotifyCalls) != n {
		t.Errorf("want %d NotifyCalls, got %d", n, len(f.NotifyCalls))
	}
}

// ---- B6: Preflight fails with method-named error on method_not_found -------

func TestPreflight_WorkspaceNotify_MethodNotFound(t *testing.T) {
	dir := socketTempDir(t)
	socketPath, ln := createSocket(t, dir)
	t.Cleanup(func() { _ = ln.Close() })
	t.Setenv("CMUX_SOCKET_PATH", socketPath)

	client := &cmuxctl.FakeClient{
		SystemIdentifyFunc: func(_ context.Context) (cmuxctl.Identity, error) {
			return cmuxctl.Identity{SocketPath: "/fake/cmux.sock"}, nil
		},
		WorkspaceNotifyFunc: func(_ context.Context, _ cmuxctl.Workspace, _ cmuxctl.NotificationClass, _ string) error {
			return &cmuxctl.CmuxError{Code: "method_not_found", Message: "no such method"}
		},
	}

	errs := cmuxctl.Preflight(context.Background(), installedProber(), client)
	if len(errs) != 1 {
		t.Fatalf("want 1 error, got %d: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	if !hasSubstr(msg, "WorkspaceNotify") {
		t.Errorf("error missing 'WorkspaceNotify': %q", msg)
	}
	if !hasSubstr(msg, "notification.create_for_target") {
		t.Errorf("error missing wire method name: %q", msg)
	}
	if !hasSubstr(msg, "upgrade cmux") {
		t.Errorf("error missing upgrade guidance: %q", msg)
	}
}

// ---- B7: Preflight fails with method-named error on unknown_method ----------

func TestPreflight_WorkspaceNotify_UnknownMethod(t *testing.T) {
	dir := socketTempDir(t)
	socketPath, ln := createSocket(t, dir)
	t.Cleanup(func() { _ = ln.Close() })
	t.Setenv("CMUX_SOCKET_PATH", socketPath)

	client := &cmuxctl.FakeClient{
		SystemIdentifyFunc: func(_ context.Context) (cmuxctl.Identity, error) {
			return cmuxctl.Identity{SocketPath: "/fake/cmux.sock"}, nil
		},
		WorkspaceNotifyFunc: func(_ context.Context, _ cmuxctl.Workspace, _ cmuxctl.NotificationClass, _ string) error {
			return &cmuxctl.CmuxError{Code: "unknown_method", Message: "no such method"}
		},
	}

	errs := cmuxctl.Preflight(context.Background(), installedProber(), client)
	if len(errs) != 1 {
		t.Fatalf("want 1 error, got %d: %v", len(errs), errs)
	}
	if !hasSubstr(errs[0].Error(), "WorkspaceNotify") {
		t.Errorf("error missing 'WorkspaceNotify': %q", errs[0].Error())
	}
}

// ---- B8: Preflight passes when notify returns any other error ---------------

func TestPreflight_WorkspaceNotify_OtherError_Passes(t *testing.T) {
	dir := socketTempDir(t)
	socketPath, ln := createSocket(t, dir)
	t.Cleanup(func() { _ = ln.Close() })
	t.Setenv("CMUX_SOCKET_PATH", socketPath)

	client := &cmuxctl.FakeClient{
		SystemIdentifyFunc: func(_ context.Context) (cmuxctl.Identity, error) {
			return cmuxctl.Identity{SocketPath: "/fake/cmux.sock"}, nil
		},
		WorkspaceNotifyFunc: func(_ context.Context, _ cmuxctl.Workspace, _ cmuxctl.NotificationClass, _ string) error {
			// Simulate a zero-workspace rejection — not a method-not-found.
			return &cmuxctl.CmuxError{Code: "not_found", Message: "no workspace"}
		},
	}

	errs := cmuxctl.Preflight(context.Background(), installedProber(), client)
	if len(errs) != 0 {
		t.Errorf("want no errors (other errors pass probe), got: %v", errs)
	}
}

// ---- B9: Preflight happy path still passes (regression) --------------------

func TestPreflight_WorkspaceNotify_HappyPath(t *testing.T) {
	dir := socketTempDir(t)
	socketPath, ln := createSocket(t, dir)
	t.Cleanup(func() { _ = ln.Close() })
	t.Setenv("CMUX_SOCKET_PATH", socketPath)

	errs := cmuxctl.Preflight(context.Background(), installedProber(), cmuxClient())
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}
