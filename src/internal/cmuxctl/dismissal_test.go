package cmuxctl_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
)

// fastCfg returns a DismissalConfig with a 1ms poll interval and 20ms per-call
// timeout so tests complete without real timer waits.
func fastCfg() cmuxctl.DismissalConfig {
	return cmuxctl.DismissalConfig{
		PollInterval: time.Millisecond,
		PollTimeout:  20 * time.Millisecond,
	}
}

var theWS = cmuxctl.Workspace{ID: "ws-uuid", Ref: "workspace:1"}

func wsInfo(ids ...string) []cmuxctl.WorkspaceInfo {
	out := make([]cmuxctl.WorkspaceInfo, len(ids))
	for i, id := range ids {
		out[i] = cmuxctl.WorkspaceInfo{ID: id, Ref: "workspace:" + id}
	}
	return out
}

func surfList(n int) []cmuxctl.SurfaceInfo {
	out := make([]cmuxctl.SurfaceInfo, n)
	for i := range out {
		out[i] = cmuxctl.SurfaceInfo{SurfaceID: "s" + itoa(int64(i))}
	}
	return out
}

// Arm 1: the pr9k workspace disappears from workspace.list → one non-fatal event.
func TestDismissal_WorkspaceRemoved_FiresOnce(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	n := 0
	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(_ context.Context) ([]cmuxctl.WorkspaceInfo, error) {
			mu.Lock()
			c := n
			n++
			mu.Unlock()
			if c == 0 {
				return wsInfo("ws-uuid", "other"), nil
			}
			return wsInfo("other"), nil // pr9k workspace gone
		},
		SurfaceListFunc: func(_ context.Context, _ cmuxctl.Workspace) ([]cmuxctl.SurfaceInfo, error) {
			return surfList(3), nil
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	obs := cmuxctl.StartDismissalObserver(ctx, fake, theWS, 3, fastCfg())
	defer obs.Cancel()
	defer obs.Wait()

	select {
	case evt := <-obs.Ch:
		if evt.Fatal {
			t.Error("expected non-fatal dismissal")
		}
	case <-ctx.Done():
		t.Error("timed out waiting for workspace-removed dismissal")
	}
	select {
	case <-obs.Ch:
		t.Error("received a second dismissal event")
	default:
	}
}

// Arm 2: the live surface count drops below the count pr9k created.
func TestDismissal_SurfaceCountDrop_FiresOnce(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	n := 0
	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(_ context.Context) ([]cmuxctl.WorkspaceInfo, error) {
			return wsInfo("ws-uuid"), nil
		},
		SurfaceListFunc: func(_ context.Context, _ cmuxctl.Workspace) ([]cmuxctl.SurfaceInfo, error) {
			mu.Lock()
			c := n
			n++
			mu.Unlock()
			if c == 0 {
				return surfList(3), nil
			}
			return surfList(2), nil // a pane was closed
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	obs := cmuxctl.StartDismissalObserver(ctx, fake, theWS, 3, fastCfg())
	defer obs.Cancel()
	defer obs.Wait()

	select {
	case evt := <-obs.Ch:
		if evt.Fatal {
			t.Error("expected non-fatal dismissal")
		}
	case <-ctx.Done():
		t.Error("timed out waiting for surface-count-drop dismissal")
	}
}

// surface.list returning a not_found CmuxError (workspace vanished) is a
// dismissal, not a timeout.
func TestDismissal_SurfaceListNotFound_FiresDismissal(t *testing.T) {
	t.Parallel()
	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(_ context.Context) ([]cmuxctl.WorkspaceInfo, error) {
			return wsInfo("ws-uuid"), nil
		},
		SurfaceListFunc: func(_ context.Context, _ cmuxctl.Workspace) ([]cmuxctl.SurfaceInfo, error) {
			return nil, &cmuxctl.CmuxError{Code: "not_found", Message: "Workspace not found"}
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	obs := cmuxctl.StartDismissalObserver(ctx, fake, theWS, 3, fastCfg())
	defer obs.Cancel()
	defer obs.Wait()

	select {
	case <-obs.Ch:
	case <-ctx.Done():
		t.Error("timed out: not_found surface.list should fire dismissal")
	}
}

// N consecutive poll timeouts escalate to a fatal dismissal event.
func TestDismissal_ConsecutiveTimeouts_Fatal(t *testing.T) {
	t.Parallel()
	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(ctx context.Context) ([]cmuxctl.WorkspaceInfo, error) {
			<-ctx.Done() // always exceed the per-call timeout
			return nil, ctx.Err()
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	obs := cmuxctl.StartDismissalObserver(ctx, fake, theWS, 3, fastCfg())
	defer obs.Cancel()
	defer obs.Wait()

	select {
	case evt := <-obs.Ch:
		if !evt.Fatal {
			t.Error("expected Fatal=true after consecutive timeouts")
		}
	case <-ctx.Done():
		t.Error("timed out waiting for fatal escalation")
	}
}

// SetShuttingDown suppresses a fired observation (self-close double-fire).
func TestDismissal_ShuttingDownSuppresses(t *testing.T) {
	t.Parallel()
	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(_ context.Context) ([]cmuxctl.WorkspaceInfo, error) {
			return wsInfo("other"), nil // pr9k workspace already absent
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	obs := cmuxctl.StartDismissalObserver(ctx, fake, theWS, 3, fastCfg())
	obs.SetShuttingDown()
	obs.Cancel()
	obs.Wait()

	select {
	case <-obs.Ch:
		t.Error("dismissal fired despite SetShuttingDown")
	default:
	}
}

// Cancel makes the goroutine exit without firing.
func TestDismissal_CancelExitsWithoutFiring(t *testing.T) {
	t.Parallel()
	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(_ context.Context) ([]cmuxctl.WorkspaceInfo, error) {
			return wsInfo("ws-uuid"), nil // never dismissed
		},
		SurfaceListFunc: func(_ context.Context, _ cmuxctl.Workspace) ([]cmuxctl.SurfaceInfo, error) {
			return surfList(3), nil
		},
	}
	obs := cmuxctl.StartDismissalObserver(context.Background(), fake, theWS, 3, fastCfg())
	obs.Cancel()
	done := make(chan struct{})
	go func() { obs.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("observer did not exit after Cancel")
	}
}

func TestDismissal_ErrorsAsCmuxError(t *testing.T) {
	// Sanity: errors.As unwraps the typed CmuxError used by the not_found arm.
	var ce *cmuxctl.CmuxError
	err := error(&cmuxctl.CmuxError{Code: "not_found"})
	if !errors.As(err, &ce) || ce.Code != "not_found" {
		t.Fatalf("errors.As failed for CmuxError: %v", err)
	}
}
