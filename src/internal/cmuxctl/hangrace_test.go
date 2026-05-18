package cmuxctl_test

import (
	"context"
	"sync"
	"testing"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
)

// TestFakeClient_HangChannels_ConcurrentAccess hammers HangNext/HangRelease
// from multiple goroutines simultaneously to verify the race detector stays
// clean after the mutex-protection fix (D-19).
//
// Before the fix, concurrent reads of f.HangNext in maybehang and a
// concurrent write via SetHangChannels constitute a data race. After the fix
// both sides snapshot/update under f.mu, eliminating the race.
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
