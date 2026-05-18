package interactionchannel

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// FakeInteractionChannel
// ---------------------------------------------------------------------------

// FakeInteractionChannel is a test double for *Channel. It records messages
// passed to Send and allows tests to inject messages into the Recv channel via
// EnqueueReady and InjectIntent. All mutable state is protected by mu.
//
// Supports both orchestrator-side (multi-role) and pane-side (single-role)
// topologies: callers inject incoming messages and drain outgoing ones
// independently of any real socket.
type FakeInteractionChannel struct {
	mu     sync.Mutex
	recv   chan Message
	sent   []Message
	closed bool
}

// NewFakeInteractionChannel returns an initialized FakeInteractionChannel
// with a buffered Recv channel.
func NewFakeInteractionChannel() *FakeInteractionChannel {
	return &FakeInteractionChannel{
		recv: make(chan Message, recvBufSize),
	}
}

// Recv returns the receive channel, mirroring the *Channel API.
func (f *FakeInteractionChannel) Recv() <-chan Message { return f.recv }

// Send records msg in the sent queue. Returns an error if Close has been called.
func (f *FakeInteractionChannel) Send(msg Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return fmt.Errorf("interactionchannel: FakeInteractionChannel: closed")
	}
	f.sent = append(f.sent, msg)
	return nil
}

// SendStateHeader records a StateHeader message, identical to Send but typed.
// Convenience method for orchestrator-side tests that call the role-targeted API.
func (f *FakeInteractionChannel) SendStateHeader(msg StateHeader) {
	_ = f.Send(msg)
}

// SendStateLog records a StateLog message, identical to Send but typed.
func (f *FakeInteractionChannel) SendStateLog(msg StateLog) {
	_ = f.Send(msg)
}

// SendStateFooter records a StateFooter message, identical to Send but typed.
func (f *FakeInteractionChannel) SendStateFooter(msg StateFooter) {
	_ = f.Send(msg)
}

// Close marks the fake closed so subsequent Send calls return an error.
// Idempotent.
func (f *FakeInteractionChannel) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

// IsClosed reports whether Close has been called.
func (f *FakeInteractionChannel) IsClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// AwaitReady is a stub that satisfies the orchChannel interface. It returns nil
// immediately unless the context is already done.
func (f *FakeInteractionChannel) AwaitReady(ctx context.Context, _ time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// EnqueueReady injects a Ready{Role: role} message into the Recv channel as
// if it arrived from the wire. Panics if the channel is full (recvBufSize
// slots — callers must drain between bulk injections).
func (f *FakeInteractionChannel) EnqueueReady(role string) {
	f.recv <- Ready{Role: role}
}

// InjectIntent injects an Intent{Kind: kind} message into the Recv channel.
func (f *FakeInteractionChannel) InjectIntent(kind IntentType) {
	f.recv <- Intent{Kind: kind}
}

// InjectMessage injects any Message into the Recv channel.
func (f *FakeInteractionChannel) InjectMessage(msg Message) {
	f.recv <- msg
}

// ExpectStatePush drains and returns the oldest message from the sent queue.
// Returns (nil, false) if the queue is empty.
func (f *FakeInteractionChannel) ExpectStatePush() (Message, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sent) == 0 {
		return nil, false
	}
	msg := f.sent[0]
	f.sent = f.sent[1:]
	return msg, true
}

// SentMessages returns a snapshot of all messages that have been Send-ed,
// in order, without draining.
func (f *FakeInteractionChannel) SentMessages() []Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Message, len(f.sent))
	copy(out, f.sent)
	return out
}

// ---------------------------------------------------------------------------
// FakeDisplayPane
// ---------------------------------------------------------------------------

// FakeDisplayPane is a test double for a display pane's read+render loop.
// It dials a real *Channel socket, records every state message received, and
// exposes hooks to simulate slow-consumer back-pressure and premature exit
// before WorkspaceDone. All mutable state is protected by mu.
type FakeDisplayPane struct {
	mu        sync.Mutex
	role      string
	ch        *Channel
	received  []Message
	slowC     bool // if true, readLoop pauses between messages
	earlyExit bool // if true, pane disconnects immediately on next read
}

// NewFakeDisplayPane returns an unconnected FakeDisplayPane for the given role.
// Call Connect to establish the socket connection.
func NewFakeDisplayPane(role string) *FakeDisplayPane {
	return &FakeDisplayPane{role: role}
}

// Connect dials socketPath and starts a background goroutine that reads
// incoming messages and stores them for ExpectMessage. The caller must call
// Disconnect (or cancel ctx) to tear down the connection.
func (p *FakeDisplayPane) Connect(ctx context.Context, socketPath string) error {
	ch, err := Dial(ctx, socketPath, p.role)
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.ch = ch
	p.mu.Unlock()

	go p.readLoop(ctx, ch)
	return nil
}

func (p *FakeDisplayPane) readLoop(ctx context.Context, ch *Channel) {
	for {
		// Check slow-consumer flag before each read.
		p.mu.Lock()
		slow := p.slowC
		early := p.earlyExit
		p.mu.Unlock()

		if early {
			ch.Close()
			return
		}
		if slow {
			select {
			case <-ctx.Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}

		select {
		case msg, ok := <-ch.Recv():
			if !ok {
				return
			}
			p.mu.Lock()
			p.received = append(p.received, msg)
			p.mu.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// SendReady sends Ready{Role: p.role} to the orchestrator. Must be called
// after Connect.
func (p *FakeDisplayPane) SendReady() error {
	p.mu.Lock()
	ch := p.ch
	p.mu.Unlock()
	if ch == nil {
		return fmt.Errorf("interactionchannel: FakeDisplayPane.SendReady: not connected")
	}
	return ch.Send(Ready{Role: p.role})
}

// ExpectMessage drains and returns the oldest received message. Returns
// (nil, false) if no messages have been recorded yet.
func (p *FakeDisplayPane) ExpectMessage() (Message, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.received) == 0 {
		return nil, false
	}
	msg := p.received[0]
	p.received = p.received[1:]
	return msg, true
}

// Disconnect closes the underlying channel. Idempotent.
func (p *FakeDisplayPane) Disconnect() {
	p.mu.Lock()
	ch := p.ch
	p.mu.Unlock()
	if ch != nil {
		ch.Close()
	}
}

// SetSlowConsumer controls whether the pane's read loop pauses between
// messages to simulate back-pressure.
func (p *FakeDisplayPane) SetSlowConsumer(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.slowC = v
}

// SetPrematureExit causes the pane to disconnect on its next read iteration,
// simulating a pane process that exits before receiving WorkspaceDone.
func (p *FakeDisplayPane) SetPrematureExit(v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.earlyExit = v
}

// Received returns a snapshot of all messages received by this pane, in
// arrival order, without draining.
func (p *FakeDisplayPane) Received() []Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Message, len(p.received))
	copy(out, p.received)
	return out
}

// ---------------------------------------------------------------------------
// FakeFooterKeySource
// ---------------------------------------------------------------------------

// FakeFooterKeySource is a test double for the footer pane's keystroke source.
// Tests script a keystroke sequence via Press; the footer state machine
// consumes keys via Next. The state machine reports its current mode via
// SetMode and records forwarded intents via RecordIntent. All mutable state
// is protected by mu.
type FakeFooterKeySource struct {
	mu            sync.Mutex
	keys          []string
	mode          int
	lastIntent    IntentType
	lastIntentSet bool
}

// NewFakeFooterKeySource returns an initialized FakeFooterKeySource with an
// empty key queue.
func NewFakeFooterKeySource() *FakeFooterKeySource {
	return &FakeFooterKeySource{}
}

// Press appends key to the scripted key queue.
func (f *FakeFooterKeySource) Press(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys = append(f.keys, key)
}

// Next pops and returns the next key from the queue. Returns ("", false) if
// the queue is empty.
func (f *FakeFooterKeySource) Next() (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.keys) == 0 {
		return "", false
	}
	k := f.keys[0]
	f.keys = f.keys[1:]
	return k, true
}

// SetMode records the state machine's current mode. Called by the footer
// state machine when it transitions between modes.
func (f *FakeFooterKeySource) SetMode(mode int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
}

// Mode returns the last mode reported via SetMode.
func (f *FakeFooterKeySource) Mode() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.mode
}

// RecordIntent records an intent forwarded to the orchestrator. Called by the
// footer state machine when it decides to forward an intent.
func (f *FakeFooterKeySource) RecordIntent(kind IntentType) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastIntent = kind
	f.lastIntentSet = true
}

// LastForwardedIntent returns the most recently forwarded intent and true, or
// the zero IntentType and false if no intent has been forwarded yet.
func (f *FakeFooterKeySource) LastForwardedIntent() (IntentType, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastIntent, f.lastIntentSet
}
