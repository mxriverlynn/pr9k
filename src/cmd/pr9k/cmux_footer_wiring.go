package main

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// footerPaneCh is the minimal interaction-channel subset the footer pane's
// state machine loop requires: receive messages from the orchestrator, send
// responses (Intent, DoneAck) back, and close.
type footerPaneCh interface {
	Recv() <-chan interactionchannel.Message
	Send(msg interactionchannel.Message) error
	Close()
}

// footerPaneSink implements FooterStateSink, bridging the footerStateMachine to
// the renderer (mode transitions → re-render) and the pane channel (resolved
// intents → Intent sends). mu guards renderer and statusLine so SetMode and
// status-line updates can race safely.
type footerPaneSink struct {
	mu         sync.Mutex
	renderer   *cmuxFooterRenderer
	ch         footerPaneCh
	out        io.Writer
	statusLine string
}

// SetMode is called by footerStateMachine on every local mode transition. It
// triggers a re-render with the new shortcut line. line is passed directly to
// Render rather than stored on the renderer, eliminating the data race between
// the keystroke goroutine (Step) and the main select loop (external SetMode).
func (s *footerPaneSink) SetMode(mode int) {
	line := footerShortcutForMode(ui.Mode(mode))
	s.mu.Lock()
	sl := s.statusLine
	s.mu.Unlock()
	_, _ = fmt.Fprint(s.out, s.renderer.Render(sl, line))
}

// RecordIntent is called by footerStateMachine when a fully-resolved intent is
// ready. It sends an Intent message to the orchestrator over the pane channel.
// Send errors are intentionally ignored: the orchestrator may have closed the
// channel during shutdown, and there is no logger available in the footer pane
// (D-9). Best-effort delivery is acceptable here.
func (s *footerPaneSink) RecordIntent(kind interactionchannel.IntentType) {
	_ = s.ch.Send(interactionchannel.Intent{Kind: kind})
}

// footerShortcutForMode returns the shortcut-bar text for mode, mirroring
// ui.KeyHandler.updateShortcutLineLocked so the footer pane and orchestrator
// display consistent strings.
func footerShortcutForMode(mode ui.Mode) string {
	switch mode {
	case ui.ModeNormal:
		return ui.NormalShortcuts
	case ui.ModeError:
		return ui.ErrorShortcuts
	case ui.ModeQuitConfirm:
		return ui.QuitConfirmPrompt
	case ui.ModeNextConfirm:
		return ui.NextConfirmPrompt
	case ui.ModeDone:
		return ui.DoneShortcuts
	case ui.ModeQuitting:
		return ui.QuittingLine
	default:
		return ui.NormalShortcuts
	}
}

// runCmuxFooterMachineWith is the testable core of the footer pane's state
// machine loop. src provides keystroke input (FakeFooterKeySource in tests,
// a real stdin reader in production); ch must already be dialed and Ready must
// have been sent before this call returns.
//
// stalledCh is closed when the pane's connection to the orchestrator is lost
// (stall timer or clean disconnect, W-5). Pass a nil channel in tests that do
// not exercise the stall path — a nil channel blocks forever in a select arm.
//
// Behaviour:
//   - A WaitGroup-drained goroutine runs footerStateMachine.Step() in a loop
//     until ctx is cancelled. The WaitGroup is established before the goroutine
//     starts (docs/coding-standards/concurrency.md).
//   - The main select loop dispatches StateFooter (→ machine.SetMode, re-render),
//     WorkspaceDone (→ DoneAck; Aborted renders RunAbortedToken and returns),
//     stalledCh (→ renders RunAbortedToken and returns), and ctx.Done.
//   - Context cancellation exits both the goroutine and the main loop.
func runCmuxFooterMachineWith(ctx context.Context, src FooterKeySource, ch footerPaneCh, out io.Writer, stalledCh <-chan struct{}) {
	renderer := newCmuxFooterRenderer()
	sink := &footerPaneSink{renderer: renderer, ch: ch, out: out}
	machine := newFooterStateMachine(src, sink)

	// Establish WaitGroup before starting the goroutine (concurrency standard).
	// The goroutine blocks on src.Ready() rather than spinning: noopFooterKeySource
	// returns a nil channel (blocks forever); FakeFooterKeySource signals the channel
	// on Press. This prevents 100% CPU burn when no keys are available.
	var wg sync.WaitGroup
	kCtx, kCancel := context.WithCancel(ctx)
	defer kCancel()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-kCtx.Done():
				return
			case <-src.Ready():
				machine.Step()
			}
		}
	}()

	for {
		select {
		case <-stalledCh:
			// Orchestrator-death path (W-5): connection stalled or lost.
			_, _ = fmt.Fprintf(out, "%s\n", interactionchannel.RunAbortedToken)
			kCancel()
			wg.Wait()
			return
		case msg, ok := <-ch.Recv():
			if !ok {
				// Channel closed by orchestrator after a normal (non-aborted) run.
				// stalledCh fires first on disconnect/stall (via onLost) and renders
				// RunAbortedToken on that select arm; this arm only fires on a clean
				// close, so no abort token is needed here.
				kCancel()
				wg.Wait()
				<-ctx.Done()
				return
			}
			switch m := msg.(type) {
			case interactionchannel.StateFooter:
				machine.SetMode(ui.Mode(m.Mode))
			case interactionchannel.WorkspaceDone:
				_ = ch.Send(interactionchannel.DoneAck{Role: "footer"})
				if m.Aborted {
					// Clean-abort path (W-5): orchestrator broadcast abort.
					_, _ = fmt.Fprintf(out, "%s\n", interactionchannel.RunAbortedToken)
					kCancel()
					wg.Wait()
					return
				}
				kCancel()
				wg.Wait()
				<-ctx.Done()
				return
			}
		case <-ctx.Done():
			kCancel()
			wg.Wait()
			return
		}
	}
}
