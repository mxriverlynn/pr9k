package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/mxriverlynn/pr9k/src/internal/cmuxctl"
	"github.com/mxriverlynn/pr9k/src/internal/logger"
)

// errorReFireInterval is the cadence for persistent error-mode notifications.
const errorReFireInterval = 60 * time.Second

// cmuxNotifier fires cmux workspace notifications at three lifecycle moments:
// completion (one-shot), run-aborted (one-shot), and error-mode (persistent at
// errorReFireInterval cadence). It is parallel to cmuxSidebar: single-purpose,
// constructed at runCmuxOrchestratorWith alongside the sidebar, and deferred.
//
// All mutable fields are guarded by mu (snapshot-then-unlock per
// docs/coding-standards/concurrency.md).
type cmuxNotifier struct {
	client     cmuxctl.CmuxClient
	ws         cmuxctl.Workspace
	log        *logger.Logger
	projectDir string
	// newTicker is injectable for tests; production uses errorReFireInterval.
	newTicker func() (<-chan time.Time, func())

	mu           sync.Mutex
	errorActive  bool
	snapshotName string
	cancelRefire context.CancelFunc
	resolved     chan struct{}

	// goroutines tracks all active firePersistent goroutines so callers can
	// synchronize with their termination (used by tests via awaitStopped).
	goroutines sync.WaitGroup
}

func newCmuxNotifier(client cmuxctl.CmuxClient, ws cmuxctl.Workspace, log *logger.Logger, projectDir string) *cmuxNotifier {
	n := &cmuxNotifier{
		client:     client,
		ws:         ws,
		log:        log,
		projectDir: projectDir,
	}
	n.newTicker = func() (<-chan time.Time, func()) {
		t := time.NewTicker(errorReFireInterval)
		return t.C, t.Stop
	}
	return n
}

func (n *cmuxNotifier) repoBasename() string {
	return filepath.Base(n.projectDir)
}

// FireCompletion fires a one-shot completion notification. The caller swallows
// the error; it is returned for transparency.
func (n *cmuxNotifier) FireCompletion(ctx context.Context) error {
	if n == nil || n.client == nil {
		return nil
	}
	body := fmt.Sprintf("pr9k run completed in %s", n.repoBasename())
	return n.client.WorkspaceNotify(ctx, n.ws, cmuxctl.NotificationCompletion, body)
}

// FireRunAborted fires a one-shot run-aborted notification. Abort-path
// semantics (spec D10): *cmuxctl.TimeoutError is treated as non-fatal and
// logged; all other errors are returned. Caller swallows with _.
func (n *cmuxNotifier) FireRunAborted(ctx context.Context) error {
	if n == nil || n.client == nil {
		return nil
	}
	body := fmt.Sprintf("pr9k run aborted in %s", n.repoBasename())
	if err := n.client.WorkspaceNotify(ctx, n.ws, cmuxctl.NotificationRunAborted, body); err != nil {
		var te *cmuxctl.TimeoutError
		if errors.As(err, &te) {
			// Abort-path timeout is non-fatal (spec D10).
			_ = n.log.Log("cmuxNotifier", fmt.Sprintf("FireRunAborted: timeout (non-fatal): %v", err))
			return nil
		}
		return err
	}
	return nil
}

// EnterErrorMode snapshots the step name, sets errorActive, allocates a fresh
// resolved channel, fires the initial notification synchronously (spec D12),
// and starts the re-fire timer goroutine.
//
// Non-timeout errors are logged and swallowed (spec D7). Timeouts are fatal
// (spec D15) and returned to the caller.
func (n *cmuxNotifier) EnterErrorMode(ctx context.Context, stepName string) error {
	if n == nil || n.client == nil {
		return nil
	}

	n.mu.Lock()
	if n.errorActive {
		// Stop the previous session if still running.
		if n.cancelRefire != nil {
			n.cancelRefire()
		}
		if n.resolved != nil {
			close(n.resolved)
		}
	}
	n.errorActive = true
	n.snapshotName = stepName
	n.resolved = make(chan struct{})
	resolved := n.resolved // local capture for the goroutine (R5 prevention)
	snapshot := stepName   // local capture

	reCtx, cancel := context.WithCancel(ctx)
	n.cancelRefire = cancel
	n.mu.Unlock()

	// Fire the initial notification synchronously (same goroutine — spec D12).
	body := fmt.Sprintf("%s failed in %s — Focus the footer pane to respond", snapshot, n.repoBasename())
	if err := n.client.WorkspaceNotify(ctx, n.ws, cmuxctl.NotificationErrorMode, body); err != nil {
		var te *cmuxctl.TimeoutError
		if errors.As(err, &te) {
			return err // fatal per parent D15
		}
		_ = n.log.Log("cmuxNotifier", fmt.Sprintf("EnterErrorMode: %v", err))
	}

	tickC, stopTicker := n.newTicker()
	n.goroutines.Add(1)
	go func() {
		defer n.goroutines.Done()
		n.firePersistent(reCtx, tickC, stopTicker, resolved, snapshot)
	}()
	return nil
}

// firePersistent is the re-fire timer goroutine body. It ticks every
// errorReFireInterval and fires a WorkspaceNotify until the context is
// cancelled or the session is resolved.
func (n *cmuxNotifier) firePersistent(ctx context.Context, tickC <-chan time.Time, stopTicker func(), resolved chan struct{}, snapshot string) {
	defer stopTicker()
	body := fmt.Sprintf("%s failed in %s — Focus the footer pane to respond", snapshot, n.repoBasename())
	for {
		select {
		case <-ctx.Done():
			return
		case <-tickC:
			// Guard against a race between tick arrival and context cancellation.
			if ctx.Err() != nil {
				return
			}
			err := n.client.WorkspaceNotify(ctx, n.ws, cmuxctl.NotificationErrorMode, body)
			// Check whether the session was resolved while the call was in flight
			// (spec D16: post-answer outcome is non-fatal regardless of result).
			select {
			case <-resolved:
				if err != nil {
					_ = n.log.Log("cmuxNotifier", fmt.Sprintf("firePersistent: post-resolution call (non-fatal): %v", err))
				}
				return
			default:
			}
			if err != nil {
				var te *cmuxctl.TimeoutError
				if errors.As(err, &te) {
					// Fatal timeout in re-fire path: stop the cadence.
					return
				}
				_ = n.log.Log("cmuxNotifier", fmt.Sprintf("firePersistent: %v", err))
			}
		}
	}
}

// ExitErrorMode stops the re-fire timer and marks the error session resolved.
// Idempotent: safe to call when not in error mode.
func (n *cmuxNotifier) ExitErrorMode() error {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.errorActive {
		return nil
	}
	n.errorActive = false
	if n.cancelRefire != nil {
		n.cancelRefire()
		n.cancelRefire = nil
	}
	if n.resolved != nil {
		close(n.resolved)
		n.resolved = nil
	}
	return nil
}

// RestartErrorModeTimer allocates a fresh resolved channel and respawns the
// re-fire goroutine from 0. Called on IntentErrorQuitCancelled: the operator
// cancelled a quit confirmation while in error mode, so the persistent prompt
// resumes.
func (n *cmuxNotifier) RestartErrorModeTimer(ctx context.Context) {
	if n == nil || n.client == nil {
		return
	}
	n.mu.Lock()
	if n.cancelRefire != nil {
		n.cancelRefire()
	}
	snapshot := n.snapshotName
	n.resolved = make(chan struct{})
	resolved := n.resolved // local capture for the goroutine (R5 prevention)
	n.errorActive = true

	reCtx, cancel := context.WithCancel(ctx)
	n.cancelRefire = cancel
	n.mu.Unlock()

	tickC, stopTicker := n.newTicker()
	n.goroutines.Add(1)
	go func() {
		defer n.goroutines.Done()
		n.firePersistent(reCtx, tickC, stopTicker, resolved, snapshot)
	}()
}

// awaitStopped blocks until all firePersistent goroutines have exited.
// Used by tests to synchronize before reading recorder state.
func (n *cmuxNotifier) awaitStopped() {
	n.goroutines.Wait()
}
