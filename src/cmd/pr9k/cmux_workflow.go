package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/logger"
	"github.com/mxriverlynn/pr9k/src/internal/steps"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
	"github.com/mxriverlynn/pr9k/src/internal/workflow"
)

// cmuxHeader is the workflow.RunHeader adapter for cmux mode. It tracks the
// current header state in memory and pushes StateHeader messages to the
// interaction channel on every mutation. The snapshot-then-unlock pattern
// (docs/coding-standards/concurrency.md) is used so SendStateHeader is called
// outside the mutex.
type cmuxHeader struct {
	ch       orchChannel
	mu       sync.Mutex
	iterLine string
	names    []string
	states   []int
}

// Compile-time assertion: *cmuxHeader satisfies workflow.RunHeader.
var _ workflow.RunHeader = (*cmuxHeader)(nil)

func newCmuxHeader(ch orchChannel) *cmuxHeader {
	return &cmuxHeader{ch: ch}
}

// snapshotLocked returns a StateHeader reflecting the current state.
// Precondition: caller holds h.mu.
func (h *cmuxHeader) snapshotLocked() interactionchannel.StateHeader {
	names := make([]string, len(h.names))
	copy(names, h.names)
	states := make([]int, len(h.states))
	copy(states, h.states)
	return interactionchannel.StateHeader{
		IterationLine: h.iterLine,
		StepNames:     names,
		StepStates:    states,
	}
}

func (h *cmuxHeader) SetPhaseSteps(names []string) {
	h.mu.Lock()
	h.names = make([]string, len(names))
	copy(h.names, names)
	h.states = make([]int, len(names))
	snap := h.snapshotLocked()
	h.mu.Unlock()
	h.ch.SendStateHeader(snap)
}

func (h *cmuxHeader) SetStepState(idx int, state ui.StepState) {
	h.mu.Lock()
	if idx >= 0 && idx < len(h.states) {
		h.states[idx] = int(state)
	}
	snap := h.snapshotLocked()
	h.mu.Unlock()
	h.ch.SendStateHeader(snap)
}

func (h *cmuxHeader) RenderInitializeLine(stepNum, stepCount int, stepName string) {
	h.mu.Lock()
	h.iterLine = fmt.Sprintf("Initializing %d/%d: %s", stepNum, stepCount, stepName)
	snap := h.snapshotLocked()
	h.mu.Unlock()
	h.ch.SendStateHeader(snap)
}

func (h *cmuxHeader) RenderIterationLine(iter, maxIter int, issueID string) {
	h.mu.Lock()
	if maxIter > 0 {
		h.iterLine = fmt.Sprintf("Iteration %d/%d", iter, maxIter)
	} else {
		h.iterLine = fmt.Sprintf("Iteration %d", iter)
	}
	if issueID != "" {
		h.iterLine += ui.IterationIssueSep + issueID
	}
	snap := h.snapshotLocked()
	h.mu.Unlock()
	h.ch.SendStateHeader(snap)
}

// nameAt returns the step name at index idx, or "" if idx is out of bounds.
func (h *cmuxHeader) nameAt(idx int) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if idx < 0 || idx >= len(h.names) {
		return ""
	}
	return h.names[idx]
}

func (h *cmuxHeader) RenderFinalizeLine(stepNum, stepCount int, stepName string) {
	h.mu.Lock()
	h.iterLine = fmt.Sprintf("Finalizing %d/%d: %s", stepNum, stepCount, stepName)
	snap := h.snapshotLocked()
	h.mu.Unlock()
	h.ch.SendStateHeader(snap)
}

// newCmuxKeyHandler constructs a KeyHandler with runner.Terminate as the cancel
// function (D-4), mirroring main.go:226. Extracted so tests can verify the
// cancel → runner.Terminate binding without going through the full
// runCmuxWorkflowAdapted call.
func newCmuxKeyHandler(runner *workflow.Runner, actions chan ui.StepAction) *ui.KeyHandler {
	return ui.NewKeyHandler(runner.Terminate, actions)
}

// runCmuxWorkflowAdapted constructs the adapter layer between the interaction
// channel and workflow.Run, then drives the workflow to completion. It returns
// the process exit code (0 = completed or loop-broken, 1 = user quit).
//
// Adapter responsibilities:
//   - runner.SetSender → ch.SendStateLog (synchronous on the orchestrator
//     goroutine, satisfying D-5: WriteToLog retries enqueue before step output)
//   - cmuxHeader.SetStepState → ch.SendStateHeader
//   - keyHandler.SetOnModeChange → ch.SendStateFooter (registered before
//     workflow.Run, satisfying D-6)
//   - key-adapter goroutine: translates Intent messages from ch.Recv() into
//     StepAction sends on actions (started before workflow.Run, D-6)
//
// The key-adapter goroutine is drained via sync.WaitGroup after workflow.Run
// returns. keyCtx (derived from ctx) is cancelled explicitly after workflow.Run
// so the goroutine exits promptly; the defer is a safety net for early returns.
func runCmuxWorkflowAdapted(ctx context.Context, ch orchChannel, log *logger.Logger, projectDir, workflowDir string, sf steps.StepFile, sidebar *cmuxSidebar, notifier *cmuxNotifier) int {
	runner := workflow.NewRunner(log, projectDir)

	// Wire the log adapter synchronously (D-5): every WriteToLog / subprocess
	// line is forwarded to the log pane as a StateLog on the caller goroutine,
	// so RetryStepSeparator is enqueued before any retried-step output.
	runner.SetSender(func(line string) {
		ch.SendStateLog(interactionchannel.StateLog{Lines: [][]byte{[]byte(line)}})
	})

	header := newCmuxHeader(ch)
	wrappedHeader := &sidebarAwareHeader{inner: header, sidebar: sidebar, ctx: ctx}

	actions := make(chan ui.StepAction, 10)
	keyHandler := newCmuxKeyHandler(runner, actions)

	// Register the mode-change hook BEFORE workflow.Run (D-6). Called
	// synchronously inside SetMode/ForceQuit outside h.mu, so it never deadlocks.
	// The closure is augmented in place (D-2): a second SetOnModeChange call would
	// overwrite Phase 3's footer push, so the sidebar branch lives here.
	keyHandler.SetOnModeChange(func(mode ui.Mode, line string) {
		// Three-step ordering (spec D12): footer broadcast → sidebar mutation →
		// notifier mutation. Sequential in the same goroutine; ordering is structural.
		ch.SendStateFooter(interactionchannel.StateFooter{
			Mode:         int(mode),
			ShortcutLine: line,
		})
		if mode == ui.ModeError {
			_ = sidebar.EnterErrorMode(ctx)
			_ = notifier.EnterErrorMode(ctx, sidebar.LastStepName())
		}
	})

	// Prime the footer with ModeNormal before any step runs (D-6 "prime the
	// channel before blocking receive" pattern: the footer pane must have an
	// initial state to render before it receives the first Intent).
	ch.SendStateFooter(interactionchannel.StateFooter{
		Mode:         int(ui.ModeNormal),
		ShortcutLine: ui.NormalShortcuts,
	})

	// Start the key-adapter goroutine BEFORE workflow.Run (D-6). It translates
	// Intent messages from the footer pane into StepAction sends on actions.
	// keyCtx is cancelled after workflow.Run returns so the goroutine exits
	// promptly; the WaitGroup drains it before we return.
	keyCtx, keyCancel := context.WithCancel(ctx)
	defer keyCancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		keyAdapterLoop(keyCtx, ch, actions, keyHandler, notifier)
	}()

	runCfg := workflow.RunConfig{
		WorkflowDir:     workflowDir,
		Iterations:      0, // unlimited; breakLoopIfEmpty governs loop exit
		Env:             sf.Env,
		ContainerEnv:    sf.ContainerEnv,
		InitializeSteps: sf.Initialize,
		Steps:           sf.Iteration,
		FinalizeSteps:   sf.Finalize,
		RunStamp:        log.RunStamp(),
		Runner:          nil, // no statusline runner in cmux orchestrator (D-9)
	}

	result := workflow.Run(runner, wrappedHeader, keyHandler, runCfg)
	keyHandler.SetMode(ui.ModeDone)

	// Terminal notifications fire between workflow.Run returning and
	// sidebar.ClearAll (spec PD-9, PD-10).
	switch result.ExitReason {
	case workflow.ExitReasonCompleted, workflow.ExitReasonLoopBroken:
		_ = notifier.FireCompletion(ctx)
	case workflow.ExitReasonUserQuit:
		_ = notifier.ExitErrorMode() // stop re-fire timer if active (idempotent)
		_ = notifier.FireRunAborted(ctx)
	}

	_ = sidebar.ClearAll(ctx) // graceful-path sidebar cleanup (D-6)

	keyCancel()
	wg.Wait()

	if result.ExitReason == workflow.ExitReasonUserQuit {
		return 1
	}
	return 0
}

// keyAdapterLoop translates Intent messages received from ch into StepAction
// sends on actions, calling kh.ForceQuit on IntentQuit. It runs until ctx is
// done or ch.Recv() is closed. Extracted so it can be tested without calling
// workflow.Run.
func keyAdapterLoop(ctx context.Context, ch orchChannel, actions chan ui.StepAction, kh *ui.KeyHandler, notifier *cmuxNotifier) {
	for {
		select {
		case msg, ok := <-ch.Recv():
			if !ok {
				return
			}
			intent, ok := msg.(interactionchannel.Intent)
			if !ok {
				continue
			}
			switch intent.Kind {
			case interactionchannel.IntentRetry:
				_ = notifier.ExitErrorMode() // spec D5: first keystroke stops timer
				select {
				case actions <- ui.ActionRetry:
				default:
				}
			case interactionchannel.IntentContinue:
				_ = notifier.ExitErrorMode() // spec D5: first keystroke stops timer
				select {
				case actions <- ui.ActionContinue:
				default:
				}
			case interactionchannel.IntentNext:
				select {
				case actions <- ui.ActionNext:
				default:
				}
			case interactionchannel.IntentSkip:
				select {
				case actions <- ui.ActionSkip:
				default:
				}
			case interactionchannel.IntentQuit:
				kh.ForceQuit()
			case interactionchannel.IntentErrorQuitInitiated:
				_ = notifier.ExitErrorMode() // spec D5: first 'q' keystroke stops timer
			case interactionchannel.IntentErrorQuitCancelled:
				notifier.RestartErrorModeTimer(ctx) // spec D5: cancel restarts from 0
			}
		case <-ctx.Done():
			return
		}
	}
}
