package main

import (
	"context"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// footerChanAdapter is the subset of the interaction channel API that the key
// handler adapter needs: send mode state to the footer and receive intents.
type footerChanAdapter interface {
	SendStateFooter(msg interactionchannel.StateFooter)
	Recv() <-chan interactionchannel.Message
}

// newKeyHandlerAdapter wires h to the interaction channel ch:
//
//   - Mode changes on h (via SetMode or ForceQuit) push a StateFooter message
//     to ch so the footer pane can update its shortcut bar.
//   - Intent messages arriving on ch.Recv() are translated to StepAction sends
//     on h.Actions so workflow.Run sees no difference from standard mode.
//
// Returns a cancel function; calling it stops the adapter goroutine. The
// goroutine also exits when ctx is cancelled.
func newKeyHandlerAdapter(ctx context.Context, h *ui.KeyHandler, ch footerChanAdapter) func() {
	h.SetOnModeChange(func(mode ui.Mode, line string) {
		ch.SendStateFooter(interactionchannel.StateFooter{
			Mode:         int(mode),
			ShortcutLine: line,
		})
	})

	cctx, cancel := context.WithCancel(ctx)
	go runAdapterLoop(cctx, h, ch)
	return cancel
}

func runAdapterLoop(ctx context.Context, h *ui.KeyHandler, ch footerChanAdapter) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch.Recv():
			if !ok {
				return
			}
			intent, ok := msg.(interactionchannel.Intent)
			if !ok {
				continue
			}
			forwardIntent(ctx, h, intent)
		}
	}
}

func forwardIntent(ctx context.Context, h *ui.KeyHandler, intent interactionchannel.Intent) {
	switch intent.Kind {
	case interactionchannel.IntentRetry:
		select {
		case h.Actions <- ui.ActionRetry:
		case <-ctx.Done():
		}
	case interactionchannel.IntentContinue:
		select {
		case h.Actions <- ui.ActionContinue:
		case <-ctx.Done():
		}
	case interactionchannel.IntentQuit:
		h.ForceQuit()
	case interactionchannel.IntentSkip:
		select {
		case h.Actions <- ui.ActionSkip:
		case <-ctx.Done():
		}
	case interactionchannel.IntentNext:
		select {
		case h.Actions <- ui.ActionNext:
		case <-ctx.Done():
		}
	}
}
