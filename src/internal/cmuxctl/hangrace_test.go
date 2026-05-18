package cmuxctl_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
)

// TestFakeClient_HangChannels_ConcurrentAccess hammers HangNext/HangRelease
// from multiple goroutines simultaneously to verify the race detector stays
// clean after the mutex-protection fix (D-19).
//
// Before the fix, concurrent reads of f.HangNext in maybehang and a
// concurrent write via SetHangChannels constitute a data race. After the fix
// both sides snapshot/update under f.mu, eliminating the race.
// TestFakeClient_HangChannels_FunctionalHangRelease verifies that maybehang
// actually blocks WorkspaceCreate until a value is sent to HangRelease. The
// race test (TestFakeClient_HangChannels_ConcurrentAccess) only checks mutex
// safety; this test checks the hang/release behavior itself.
func TestFakeClient_HangChannels_FunctionalHangRelease(t *testing.T) {
	hangNext := make(chan struct{}, 1)
	hangRelease := make(chan struct{}, 1)

	f := &cmuxctl.FakeClient{}
	f.SetHangChannels(hangNext, hangRelease)

	// Prime the next-call hang.
	hangNext <- struct{}{}

	done := make(chan error, 1)
	go func() {
		done <- f.WorkspaceCreate(context.Background(), "ws")
	}()

	// The call must not return within a short window.
	select {
	case <-done:
		t.Fatal("WorkspaceCreate returned before HangRelease was sent")
	case <-time.After(50 * time.Millisecond):
	}

	// Release the hang; WorkspaceCreate must now return nil.
	hangRelease <- struct{}{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WorkspaceCreate returned non-nil error after release: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for WorkspaceCreate to return after HangRelease")
	}
}

func TestFakeClient_HangChannels_ConcurrentAccess(t *testing.T) {
	hangNext := make(chan struct{}, 100)
	hangRelease := make(chan struct{}, 100)

	f := &cmuxctl.FakeClient{}

	var wg sync.WaitGroup

	// Four goroutines each call a method, which reads HangNext/HangRelease
	// inside maybehang.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = f.WorkspaceCreate(context.Background(), "ws")
		}()
	}

	// One goroutine sets the hang channels concurrently via the mutex-
	// protected setter — this is the write side that races without the fix.
	wg.Add(1)
	go func() {
		defer wg.Done()
		f.SetHangChannels(hangNext, hangRelease)
	}()

	wg.Wait()
}
