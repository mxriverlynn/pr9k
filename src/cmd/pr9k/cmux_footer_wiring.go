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
// updates the renderer's shortcut line and triggers a re-render. The shortcut
// line is computed from the mode using the same ui constants that the
// orchestrator's KeyHandler uses.
func (s *footerPaneSink) SetMode(mode int) {
	line := footerShortcutForMode(ui.Mode(mode))
	s.mu.Lock()
	s.renderer.SetShortcutLine(line)
	sl := s.statusLine
	s.mu.Unlock()
	_, _ = fmt.Fprint(s.out, s.renderer.Render(sl))
}

// RecordIntent is called by footerStateMachine when a fully-resolved intent is
// ready. It sends an Intent message to the orchestrator over the pane channel.
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
// Behaviour:
//   - A WaitGroup-drained goroutine runs footerStateMachine.Step() in a loop
//     until ctx is cancelled. The WaitGroup is established before the goroutine
//     starts (docs/coding-standards/concurrency.md).
//   - The main select loop dispatches StateFooter (→ machine.SetMode, re-render)
//     and WorkspaceDone (→ DoneAck, block until ctx done) from ch.Recv().
//   - Context cancellation exits both the goroutine and the main loop.
func runCmuxFooterMachineWith(ctx context.Context, src FooterKeySource, ch footerPaneCh, out io.Writer) {
	renderer := newCmuxFooterRenderer()
	sink := &footerPaneSink{renderer: renderer, ch: ch, out: out}
	machine := newFooterStateMachine(src, sink)

	// Establish WaitGroup before starting the goroutine (concurrency standard).
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
			default:
				machine.Step()
			}
		}
	}()

	for {
		select {
		case msg, ok := <-ch.Recv():
			if !ok {
				// Channel closed by orchestrator — stay alive for final render.
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
