package cmuxctl_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
)

// fastCfg returns a DismissalConfig suitable for unit tests: 1ms poll interval
// and 20ms per-call timeout so tests complete quickly without real timer waits.
func fastCfg() cmuxctl.DismissalConfig {
	return cmuxctl.DismissalConfig{
		PollInterval: time.Millisecond,
		PollTimeout:  20 * time.Millisecond,
	}
}

// TP-222-001: workspace-removed observation fires dismissal exactly once.
// FakeClient scripts WorkspaceList to omit the pr9k workspace name on the
// second call; the observer must send exactly one non-fatal DismissalEvent.
func TestDismissal_WorkspaceRemoved_FiresOnce(t *testing.T) {
	t.Parallel()

	const workspaceName = "pr9k-repo-20260101T000000.000000000Z"
	var mu sync.Mutex
	callCount := 0

	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(_ context.Context) ([]string, error) {
			mu.Lock()
			n := callCount
			callCount++
			mu.Unlock()
			if n == 0 {
				return []string{workspaceName, "other"}, nil
			}
			return []string{"other"}, nil // workspace gone on second call
		},
		SurfaceListFunc: func(_ context.Context, _ string) ([]cmuxctl.PaneInfo, error) {
			return []cmuxctl.PaneInfo{{ID: "p1", Exited: false}}, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	obs := cmuxctl.StartDismissalObserver(ctx, fake, workspaceName, fastCfg())
	defer obs.Cancel()
	defer obs.Wait()

	select {
	case evt := <-obs.Ch:
		if evt.Fatal {
			t.Error("expected non-fatal dismissal, got Fatal=true")
		}
	case <-ctx.Done():
		t.Error("timed out waiting for workspace-removed dismissal")
	}

	// After goroutine exits, channel must be empty (only one send ever).
	select {
	case <-obs.Ch:
		t.Error("received second dismissal event; expected exactly one")
	default:
	}
}

// TP-222-002: per-pane exit observation fires dismissal exactly once.
// FakeClient scripts SurfaceList to mark one pane exited; the observer must
// send exactly one non-fatal DismissalEvent.
func TestDismissal_PaneExited_FiresOnce(t *testing.T) {
	t.Parallel()

	const workspaceName = "pr9k-repo-20260101T000000.000000000Z"

	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(_ context.Context) ([]string, error) {
			return []string{workspaceName}, nil // workspace always present
		},
		SurfaceListFunc: func(_ context.Context, _ string) ([]cmuxctl.PaneInfo, error) {
			return []cmuxctl.PaneInfo{
				{ID: "p1", Exited: false},
				{ID: "p2", Exited: true}, // exited pane
			}, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	obs := cmuxctl.StartDismissalObserver(ctx, fake, workspaceName, fastCfg())
	defer obs.Cancel()
	defer obs.Wait()

	select {
	case evt := <-obs.Ch:
		if evt.Fatal {
			t.Error("expected non-fatal dismissal, got Fatal=true")
		}
	case <-ctx.Done():
		t.Error("timed out waiting for pane-exit dismissal")
	}

	select {
	case <-obs.Ch:
		t.Error("received second dismissal event; expected exactly one")
	default:
	}
}

// TP-222-003: three consecutive poll timeouts escalate to fatal sentinel.
// FakeClient queues 3 hangs on WorkspaceList; with a very short pollTimeout the
// per-call contexts expire and the observer escalates to DismissalEvent{Fatal:true}.
func TestDismissal_ThreeTimeouts_FatalEscalation(t *testing.T) {
	t.Parallel()

	const workspaceName = "pr9k-repo-20260101T000000.000000000Z"

	hangNext := make(chan struct{}, 3)
	hangRelease := make(chan struct{}) // never released; ctx expiry unblocks

	hangNext <- struct{}{}
	hangNext <- struct{}{}
	hangNext <- struct{}{}

	fake := &cmuxctl.FakeClient{
		HangNext:    hangNext,
		HangRelease: hangRelease,
		WorkspaceListFunc: func(_ context.Context) ([]string, error) {
			return []string{workspaceName}, nil
		},
		SurfaceListFunc: func(_ context.Context, _ string) ([]cmuxctl.PaneInfo, error) {
			return nil, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 5ms per-call timeout: each hung call expires quickly.
	cfg := cmuxctl.DismissalConfig{
		PollInterval: time.Millisecond,
		PollTimeout:  5 * time.Millisecond,
	}

	obs := cmuxctl.StartDismissalObserver(ctx, fake, workspaceName, cfg)
	defer obs.Cancel()
	defer obs.Wait()

	select {
	case evt := <-obs.Ch:
		if !evt.Fatal {
			t.Error("expected Fatal=true after 3 consecutive timeouts, got Fatal=false")
		}
	case <-ctx.Done():
		t.Error("timed out waiting for fatal dismissal after 3 consecutive timeouts")
	}
}

// TP-222-004: timeout counter resets after a successful poll.
// Pattern: one timeout, then a success, then two more timeouts.
// Total consecutive timeouts never reaches 3, so no fatal escalation.
func TestDismissal_CounterReset_AfterSuccess(t *testing.T) {
	t.Parallel()

	const workspaceName = "pr9k-repo-20260101T000000.000000000Z"

	var mu sync.Mutex
	callCount := 0
	// false = simulate timeout (block until per-call ctx expires)
	// true  = return success immediately
	script := []bool{false, true, false, false}

	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(ctx context.Context) ([]string, error) {
			mu.Lock()
			n := callCount
			callCount++
			mu.Unlock()

			var hang bool
			if n < len(script) {
				hang = !script[n]
			}
			if hang {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return []string{workspaceName}, nil
		},
		SurfaceListFunc: func(_ context.Context, _ string) ([]cmuxctl.PaneInfo, error) {
			return nil, nil
		},
	}

	// Run the observer for 300ms; no fatal should fire (max consecutive = 2).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := cmuxctl.DismissalConfig{
		PollInterval: time.Millisecond,
		PollTimeout:  10 * time.Millisecond,
	}

	obs := cmuxctl.StartDismissalObserver(ctx, fake, workspaceName, cfg)
	defer obs.Cancel()
	defer obs.Wait()

	// Allow enough time for all 4 scripted calls plus a few extra successes.
	wait := time.NewTimer(200 * time.Millisecond)
	defer wait.Stop()
	select {
	case evt := <-obs.Ch:
		t.Errorf("unexpected dismissal event (Fatal=%v); counter should have reset", evt.Fatal)
	case <-wait.C:
		// Good: no fatal escalation
	}
}

// TP-222-005: shuttingDown flag suppresses post-shutdown observations.
// The observer must not send on the dismissal channel once shuttingDown is set.
func TestDismissal_ShuttingDown_Suppresses(t *testing.T) {
	t.Parallel()

	const workspaceName = "pr9k-repo-20260101T000000.000000000Z"

	// ready is closed after shuttingDown is set, unblocking WorkspaceList.
	ready := make(chan struct{})

	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(ctx context.Context) ([]string, error) {
			select {
			case <-ready:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return []string{"other"}, nil // workspace gone → would normally dismiss
		},
		SurfaceListFunc: func(_ context.Context, _ string) ([]cmuxctl.PaneInfo, error) {
			return nil, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := cmuxctl.DismissalConfig{
		PollInterval: time.Millisecond,
		PollTimeout:  time.Second,
	}

	obs := cmuxctl.StartDismissalObserver(ctx, fake, workspaceName, cfg)
	defer obs.Cancel()
	defer obs.Wait()

	// Set shuttingDown before unblocking the WorkspaceList call.
	obs.SetShuttingDown()
	close(ready)

	// Give the goroutine time to run the poll and (not) fire.
	wait := time.NewTimer(100 * time.Millisecond)
	defer wait.Stop()
	select {
	case evt := <-obs.Ch:
		t.Errorf("unexpected dismissal with shuttingDown set: Fatal=%v", evt.Fatal)
	case <-wait.C:
		// Good: dismissed silently
	}
}

// TP-222-006: context cancellation exits the goroutine cleanly.
// wg.Wait() must return within a bounded time after cancel().
func TestDismissal_ContextCancel_ExitsCleanly(t *testing.T) {
	t.Parallel()

	const workspaceName = "pr9k-repo-20260101T000000.000000000Z"

	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(_ context.Context) ([]string, error) {
			return []string{workspaceName}, nil // workspace always present
		},
		SurfaceListFunc: func(_ context.Context, _ string) ([]cmuxctl.PaneInfo, error) {
			return nil, nil // no panes exited
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	obs := cmuxctl.StartDismissalObserver(ctx, fake, workspaceName, fastCfg())

	cancel() // cancel context — goroutine must exit

	done := make(chan struct{})
	go func() {
		obs.Wait()
		close(done)
	}()

	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-done:
		// Good: goroutine exited cleanly
	case <-timer.C:
		obs.Cancel()
		t.Error("goroutine did not exit within 1s after context cancellation")
	}

	// No dismissal should have been sent (ctx.Done() path exits without firing).
	select {
	case evt := <-obs.Ch:
		t.Errorf("unexpected dismissal after ctx cancel: Fatal=%v", evt.Fatal)
	default:
	}
}

// TP-222-007: race-detector clean — concurrent SetShuttingDown and poll goroutine.
// This test intentionally races SetShuttingDown against an active polling loop
// and is meaningful only when run with -race.
func TestDismissal_Race_ConcurrentShuttingDown(t *testing.T) {
	t.Parallel()

	const workspaceName = "pr9k-repo-20260101T000000.000000000Z"
	var mu sync.Mutex
	callCount := 0

	fake := &cmuxctl.FakeClient{
		WorkspaceListFunc: func(_ context.Context) ([]string, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return []string{workspaceName}, nil
		},
		SurfaceListFunc: func(_ context.Context, _ string) ([]cmuxctl.PaneInfo, error) {
			return nil, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	obs := cmuxctl.StartDismissalObserver(ctx, fake, workspaceName, fastCfg())
	defer obs.Wait()
	defer obs.Cancel()

	// Race SetShuttingDown against the polling goroutine.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			obs.SetShuttingDown()
		}()
	}
	wg.Wait()
}

// TP-222-008: RunPhase1 blocks on the dismissal channel after panes are spawned
// and returns nil when dismissal fires (workspace removed, non-fatal).
func TestRunPhase1_BlocksOnDismissal_ReturnsNilOnObservedDismissal(t *testing.T) {
	t.Parallel()

	const repoDir = "/tmp/myrepo"

	fake := &cmuxctl.FakeClient{
		SurfaceSplitFunc: func(_ context.Context, _ cmuxctl.SplitOpts) (string, error) {
			return "pane-x", nil
		},
		// WorkspaceList returns empty → workspace not found → dismissal fires
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var buf [0]byte
	nw := noopWriter{}
	err := cmuxctl.RunPhase1(ctx, fake, repoDir, nw, cmuxctl.DismissalConfig{
		PollInterval: time.Millisecond,
		PollTimeout:  20 * time.Millisecond,
	})
	_ = buf
	if err != nil {
		t.Errorf("RunPhase1 returned non-nil error: %v", err)
	}
}

// noopWriter satisfies io.Writer for RunPhase1's out parameter.
type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }
