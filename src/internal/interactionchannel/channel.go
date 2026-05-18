package interactionchannel

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// ReadyHandshakeTimeout is the hard-coded deadline for the readiness
// handshake. Pass it to AwaitReady to use the standard 10-second limit.
// A configurable variant was considered and deferred (YAGNI-4).
const ReadyHandshakeTimeout = 10 * time.Second

// recvBufSize is the capacity of the per-Channel inbound message channel.
const recvBufSize = 64

// writeBufSize is the per-connection outbound channel capacity.
const writeBufSize = 32

// conn is an internal per-connection handle owned by Channel.
// It carries the outbound write channel and the underlying net.Conn
// (needed for forced close on context cancellation).
type conn struct {
	nc net.Conn
	wc chan Message
}

// Channel manages a set of connections over a Unix domain socket.
// On the Serve (orchestrator) side it owns the listener and all accepted
// connections. On the Dial (display-pane) side it owns a single connection.
//
// All exported methods are safe for concurrent use.
type Channel struct {
	recv   chan Message
	mu     sync.Mutex
	conns  []*conn
	ln     net.Listener // non-nil on the Serve side
	socket string       // socket path; used for unlink on Close (Serve side)
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Readiness handshake state (Serve side). readyNewCh receives one
	// struct{} for each newly-ready role (capacity 3 — one per distinct role).
	// notifyReady is idempotent: duplicate Ready from the same role is a no-op.
	readyMu    sync.Mutex
	readyRoles map[string]bool
	readyNewCh chan struct{}
}

// Recv returns the receive channel. Incoming messages from all connections
// are delivered on this channel in arrival order (per connection).
func (c *Channel) Recv() <-chan Message { return c.recv }

// Send broadcasts msg to all current connections. For a Dial-side Channel
// there is always exactly one connection. Returns an error if no connection
// is live.
//
// Send blocks until the outbound channel of every connection has room or
// the Channel's context is cancelled.
func (c *Channel) Send(msg Message) error {
	c.mu.Lock()
	cs := make([]*conn, len(c.conns))
	copy(cs, c.conns)
	c.mu.Unlock()

	if len(cs) == 0 {
		return fmt.Errorf("interactionchannel: Send: no active connections on %s", c.socket)
	}
	for _, co := range cs {
		select {
		case co.wc <- msg:
		case <-c.ctx.Done():
			return fmt.Errorf("interactionchannel: Send: channel closed")
		}
	}
	return nil
}

// Close shuts down the Channel: cancels the context (causing all goroutines
// to unblock), closes the listener (Serve side), and waits for all goroutines
// to finish. On the Serve side it also unlinks the socket file.
func (c *Channel) Close() {
	c.cancel()
	if c.ln != nil {
		_ = c.ln.Close()
	}
	c.wg.Wait()
	if c.socket != "" {
		_ = os.Remove(c.socket)
	}
}

// startConn registers nc as a new connection and launches the three goroutines
// (read, write, watcher) that service it.
func (c *Channel) startConn(nc net.Conn) {
	co := &conn{
		nc: nc,
		wc: make(chan Message, writeBufSize),
	}
	c.mu.Lock()
	c.conns = append(c.conns, co)
	c.mu.Unlock()

	c.wg.Add(3)

	// Read goroutine: deserializes frames and forwards to c.recv.
	go func() {
		defer c.wg.Done()
		defer c.removeConn(co)
		c.readLoop(nc, co)
	}()

	// Write goroutine: consumes co.wc and serializes frames to nc.
	go func() {
		defer c.wg.Done()
		c.writeLoop(nc, co)
	}()

	// Watcher goroutine: closes nc when the Channel context is done, which
	// unblocks the read goroutine's io.ReadFull call.
	go func() {
		defer c.wg.Done()
		<-c.ctx.Done()
		_ = nc.Close()
	}()
}

func (c *Channel) removeConn(co *conn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, existing := range c.conns {
		if existing == co {
			c.conns = append(c.conns[:i], c.conns[i+1:]...)
			return
		}
	}
}

func (c *Channel) readLoop(nc net.Conn, _ *conn) {
	br := bufio.NewReaderSize(nc, 4096)
	for {
		msg, err := readMessage(br)
		if err != nil {
			return // EOF, closed, or context cancelled
		}
		if r, ok := msg.(Ready); ok {
			c.notifyReady(r.Role)
		}
		select {
		case c.recv <- msg:
		case <-c.ctx.Done():
			return
		}
	}
}

// notifyReady marks role as ready. If the role was not previously ready it
// sends a signal to readyNewCh. Duplicate calls for the same role are no-ops.
// Safe to call from any goroutine.
func (c *Channel) notifyReady(role string) {
	c.readyMu.Lock()
	if c.readyRoles == nil || c.readyRoles[role] {
		c.readyMu.Unlock()
		return
	}
	c.readyRoles[role] = true
	c.readyMu.Unlock()
	select {
	case c.readyNewCh <- struct{}{}:
	default:
		// readyNewCh capacity is 3; this branch is unreachable in normal operation.
	}
}

// AwaitReady blocks until all three display roles ("header", "log", "footer")
// have sent a Ready message, the timeout elapses, or ctx is cancelled.
//
// Returns nil when all roles are ready.
// Returns a structured error naming the missing roles on timeout.
// Returns ctx.Err() on cancellation.
//
// The hard-coded 10-second deadline is exposed as readyHandshakeTimeout.
// A configurable variant was deferred (YAGNI-4).
func (c *Channel) AwaitReady(ctx context.Context, timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	// Fast path: all roles already ready before we start waiting.
	c.readyMu.Lock()
	done := len(c.readyRoles) >= 3
	c.readyMu.Unlock()
	if done {
		return nil
	}

	for {
		select {
		case <-c.readyNewCh:
			c.readyMu.Lock()
			n := len(c.readyRoles)
			c.readyMu.Unlock()
			if n >= 3 {
				return nil
			}
		case <-deadline.C:
			c.readyMu.Lock()
			missing := missingReadyRoles(c.readyRoles)
			c.readyMu.Unlock()
			return fmt.Errorf("interactionchannel: handshake timeout — roles not ready: [%s]",
				strings.Join(missing, " "))
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// missingReadyRoles returns the sorted list of roles not present in ready.
func missingReadyRoles(ready map[string]bool) []string {
	all := []string{"footer", "header", "log"}
	var missing []string
	for _, r := range all {
		if !ready[r] {
			missing = append(missing, r)
		}
	}
	sort.Strings(missing)
	return missing
}

func (c *Channel) writeLoop(nc net.Conn, co *conn) {
	bw := bufio.NewWriterSize(nc, 4096)
	for {
		select {
		case msg := <-co.wc:
			if err := writeMessage(bw, msg); err != nil {
				return
			}
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *Channel) acceptLoop(ln net.Listener) {
	defer c.wg.Done()
	for {
		nc, err := ln.Accept()
		if err != nil {
			return // listener closed or context done
		}
		c.startConn(nc)
	}
}

// Serve creates a Unix-domain socket listener at socketPath and returns a
// Channel that accepts incoming connections. If a stale file exists at
// socketPath it is removed before binding. On context cancellation the
// listener is closed, all connections are torn down, and the socket file is
// unlinked.
func Serve(ctx context.Context, socketPath string) (*Channel, error) {
	if _, err := os.Stat(socketPath); err == nil {
		if err := os.Remove(socketPath); err != nil {
			return nil, fmt.Errorf("interactionchannel: unlink stale socket %s: %w", socketPath, err)
		}
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("interactionchannel: listen %s: %w", socketPath, err)
	}

	ctx, cancel := context.WithCancel(ctx)
	ch := &Channel{
		recv:       make(chan Message, recvBufSize),
		ln:         ln,
		socket:     socketPath,
		ctx:        ctx,
		cancel:     cancel,
		readyRoles: make(map[string]bool),
		readyNewCh: make(chan struct{}, 3),
	}

	ch.wg.Add(1)
	go ch.acceptLoop(ln)

	// Close the listener when the context is cancelled so acceptLoop unblocks.
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	return ch, nil
}

// Dial connects to the Unix-domain socket at socketPath and returns a Channel
// whose read and write goroutines are already running. Sending the Ready
// message is the caller's responsibility (deferred to U2).
func Dial(ctx context.Context, socketPath, _ string) (*Channel, error) {
	nc, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("interactionchannel: dial %s: %w", socketPath, err)
	}

	ctx, cancel := context.WithCancel(ctx)
	ch := &Channel{
		recv:   make(chan Message, recvBufSize),
		ctx:    ctx,
		cancel: cancel,
	}
	ch.startConn(nc)
	return ch, nil
}
