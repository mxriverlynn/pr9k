package cmuxctl

import (
	"context"
	"sync"
	"time"
)

const (
	defaultPollInterval    = 500 * time.Millisecond
	defaultPollTimeout     = 5 * time.Second
	maxConsecutiveTimeouts = 3
)

// DismissalEvent is sent on the dismissal channel when the observer fires.
// Fatal is true when N=3 consecutive poll timeouts escalate to fatal teardown.
type DismissalEvent struct {
	Fatal bool
}

// DismissalConfig configures the dismissal-observation goroutine.
// Zero values use package defaults (500ms interval, 5s per-call timeout).
type DismissalConfig struct {
	PollInterval time.Duration
	PollTimeout  time.Duration
}

// DismissalObserver manages the dismissal-observation goroutine lifecycle per
// spec D6, D9, D22. The goroutine polls WorkspaceList and SurfaceList every
// PollInterval; it fires Ch when either dismissal event fires (spec D9) or
// when N=3 consecutive poll timeouts escalate to fatal teardown (D7).
//
// Callers must call Cancel (to stop the goroutine on the next timer tick) and
// Wait (to join before declaring teardown complete). SetShuttingDown suppresses
// post-shutdown observations per D8.
type DismissalObserver struct {
	mu           sync.Mutex
	shuttingDown bool

	// Ch is the buffered (cap 1) dismissal channel. The caller blocks on it
	// between workspace setup and teardown. The goroutine sends at most once.
	Ch     chan DismissalEvent
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// SetShuttingDown sets the shuttingDown flag so the goroutine drops the next
// fired observation (D8 self-close double-fire mitigation). Safe from any
// goroutine; uses snapshot-then-unlock per concurrency standards.
func (o *DismissalObserver) SetShuttingDown() {
	o.mu.Lock()
	o.shuttingDown = true
	o.mu.Unlock()
}

// Cancel cancels the observer's context, causing the goroutine to exit on its
// next timer tick without firing the dismissal channel.
func (o *DismissalObserver) Cancel() {
	o.cancel()
}

// Wait blocks until the observation goroutine has exited (D22 WaitGroup join).
func (o *DismissalObserver) Wait() {
	o.wg.Wait()
}

// StartDismissalObserver starts the dismissal-observation goroutine and returns
// the observer. workspaceName is the pr9k workspace name to watch.
// cfg controls polling cadence and per-call timeout; zero values use defaults.
func StartDismissalObserver(ctx context.Context, client CmuxClient, workspaceName string, cfg DismissalConfig) *DismissalObserver {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.PollTimeout == 0 {
		cfg.PollTimeout = defaultPollTimeout
	}

	childCtx, cancel := context.WithCancel(ctx)
	obs := &DismissalObserver{
		Ch:     make(chan DismissalEvent, 1), // buffered cap 1 per D22
		cancel: cancel,
	}
	obs.wg.Add(1)
	go obs.run(childCtx, client, workspaceName, cfg.PollInterval, cfg.PollTimeout)
	return obs
}

// run is the goroutine body. It selects on ctx.Done() and the poll timer;
// on ctx.Done() it exits immediately without firing dismissal (D22).
func (o *DismissalObserver) run(ctx context.Context, client CmuxClient, workspaceName string, interval, callTimeout time.Duration) {
	defer o.wg.Done()

	consecutiveTimeouts := 0

	for {
		t := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}

		// Guard against a parent-context cancellation that arrived exactly as
		// the timer fired (select chooses randomly between ready cases).
		if ctx.Err() != nil {
			return
		}

		dismissed, fatal, timedOut := o.observe(ctx, client, workspaceName, callTimeout)

		// If the parent context was cancelled during the RPC, exit cleanly.
		if ctx.Err() != nil {
			return
		}

		if timedOut {
			consecutiveTimeouts++
			if consecutiveTimeouts >= maxConsecutiveTimeouts {
				o.fireDismissal(DismissalEvent{Fatal: true})
				return
			}
			continue
		}

		consecutiveTimeouts = 0 // reset on any successful poll

		if dismissed {
			o.fireDismissal(DismissalEvent{Fatal: fatal})
			return
		}
	}
}

// observe performs one poll cycle: WorkspaceList then SurfaceList.
// Returns (dismissed, fatal, timedOut). fatal is always false for normal
// dismissal events; timedOut is true when the per-call context expires before
// the parent context.
func (o *DismissalObserver) observe(ctx context.Context, client CmuxClient, workspaceName string, callTimeout time.Duration) (dismissed, fatal, timedOut bool) {
	pollCtx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	workspaces, err := client.WorkspaceList(pollCtx)
	if err != nil {
		if ctx.Err() != nil {
			return false, false, false // parent cancelled; exit path handles it
		}
		return false, false, true // per-call timeout
	}

	panes, err := client.SurfaceList(pollCtx, workspaceName)
	if err != nil {
		if ctx.Err() != nil {
			return false, false, false
		}
		return false, false, true
	}

	// Check: workspace removed from cmux list? (D9 arm 1)
	workspaceFound := false
	for _, w := range workspaces {
		if w == workspaceName {
			workspaceFound = true
			break
		}
	}
	if !workspaceFound {
		return true, false, false
	}

	// Check: any pane in exited state? (D9 arm 2, D19)
	for _, p := range panes {
		if p.Exited {
			return true, false, false
		}
	}

	return false, false, false
}

// fireDismissal sends evt on Ch unless shuttingDown is set (D8).
// Uses a non-blocking send; the channel is buffered(1) so a concurrent send
// from a race window does not block (D22).
func (o *DismissalObserver) fireDismissal(evt DismissalEvent) {
	o.mu.Lock()
	shutting := o.shuttingDown // snapshot-then-unlock per concurrency standards
	o.mu.Unlock()

	if shutting {
		return
	}

	select {
	case o.Ch <- evt:
	default:
	}
}
