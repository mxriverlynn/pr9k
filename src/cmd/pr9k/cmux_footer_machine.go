package main

import (
	"sync"

	"github.com/mxriverlynn/pr9k/src/internal/interactionchannel"
	"github.com/mxriverlynn/pr9k/src/internal/ui"
)

// FooterKeySource is the source of raw key strings for the footer state machine.
type FooterKeySource interface {
	// Next returns the next available key. Returns ("", false) when no key is
	// ready — callers must wait for Ready() to fire before calling Next.
	Next() (string, bool)
	// Ready returns a channel that receives or is closed when at least one key
	// is available. A nil return blocks forever (used by noopFooterKeySource).
	Ready() <-chan struct{}
}

// FooterStateSink receives mode transitions and resolved intents from the
// footer state machine.
type FooterStateSink interface {
	// SetMode is called whenever the footer machine transitions to a new mode.
	// The int value is the integer representation of ui.Mode.
	SetMode(mode int)
	// RecordIntent is called when a fully-resolved user intent is ready to be
	// forwarded to the orchestrator.
	RecordIntent(kind interactionchannel.IntentType)
}

// footerStateMachine runs the footer pane's keyboard state machine.
// It handles q/y quit-confirmation and n/y next-step-confirmation locally;
// only fully resolved intents reach the sink.
// mu protects mode and prev; sink calls are made outside the lock
// (snapshot-unlock-dispatch, per docs/coding-standards/concurrency.md).
type footerStateMachine struct {
	src  FooterKeySource
	sink FooterStateSink
	mu   sync.Mutex
	mode ui.Mode
	prev ui.Mode
}

// newFooterStateMachine creates a footerStateMachine in ModeNormal.
func newFooterStateMachine(src FooterKeySource, sink FooterStateSink) *footerStateMachine {
	return &footerStateMachine{src: src, sink: sink, mode: ui.ModeNormal}
}

// SetMode updates the machine's current mode from an external push (e.g.,
// a StateFooter message received from the orchestrator). The sink is notified.
func (m *footerStateMachine) SetMode(mode ui.Mode) {
	m.mu.Lock()
	m.mode = mode
	m.mu.Unlock()
	m.sink.SetMode(int(mode))
}

// Step processes one key from the source. Returns false when the source is
// exhausted (no more keys available).
func (m *footerStateMachine) Step() bool {
	key, ok := m.src.Next()
	if !ok {
		return false
	}
	m.mu.Lock()
	mode := m.mode
	m.mu.Unlock()
	switch mode {
	case ui.ModeNormal:
		m.handleNormal(key)
	case ui.ModeError:
		m.handleError(key)
	case ui.ModeQuitConfirm:
		m.handleQuitConfirm(key)
	case ui.ModeNextConfirm:
		m.handleNextConfirm(key)
	case ui.ModeDone:
		m.handleDone(key)
	case ui.ModeQuitting:
		// all keys ignored during shutdown
	}
	return true
}

func (m *footerStateMachine) handleNormal(key string) {
	switch key {
	case "q":
		m.enterMode(ui.ModeQuitConfirm)
	case "n":
		m.enterMode(ui.ModeNextConfirm)
	}
}

func (m *footerStateMachine) handleError(key string) {
	switch key {
	case "c":
		m.sink.RecordIntent(interactionchannel.IntentContinue)
	case "r":
		m.sink.RecordIntent(interactionchannel.IntentRetry)
	case "q":
		m.enterMode(ui.ModeQuitConfirm)
	}
}

func (m *footerStateMachine) handleQuitConfirm(key string) {
	switch key {
	case "y":
		m.sink.RecordIntent(interactionchannel.IntentQuit)
		m.enterMode(ui.ModeQuitting)
	case "n", "esc":
		m.restoreMode()
	}
}

func (m *footerStateMachine) handleNextConfirm(key string) {
	switch key {
	case "y":
		m.sink.RecordIntent(interactionchannel.IntentNext)
		m.restoreMode()
	case "n", "esc":
		m.restoreMode()
	}
}

func (m *footerStateMachine) handleDone(key string) {
	switch key {
	case "q":
		m.enterMode(ui.ModeQuitConfirm)
	}
}

func (m *footerStateMachine) enterMode(next ui.Mode) {
	m.mu.Lock()
	m.prev = m.mode
	m.mode = next
	m.mu.Unlock()
	m.sink.SetMode(int(next))
}

func (m *footerStateMachine) restoreMode() {
	m.mu.Lock()
	m.mode = m.prev
	restored := m.mode
	m.mu.Unlock()
	m.sink.SetMode(int(restored))
}
