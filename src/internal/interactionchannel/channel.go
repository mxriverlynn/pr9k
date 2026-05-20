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

// writeBufSize is the per-connection general outbound channel capacity.
const writeBufSize = 32

// logBufSize is the per-connection log outbound channel capacity.
// Drop-oldest semantics apply when this is exceeded (D-2).
const logBufSize = 256

// conn is an internal per-connection handle owned by Channel.
// It carries the outbound write channel and the underlying net.Conn
// (needed for forced close on context cancellation).
//
// Role-specific outbound channels are initialized in startConn and populated
// via SendStateHeader/Log/Footer after the role is bound by bindRole (D-2).
type conn struct {
	nc net.Conn
	wc chan Message // general broadcast channel (WorkspaceDone, etc.)

	// Log outbound channel: 256-capacity, drop-oldest on overflow (D-2).
	logCh chan StateLog

	// Header latest-wins slot (D-2). Protected by headerMu.
	// headerNotify (capacity 1) signals the write goroutine when the slot is dirty.
	headerMu     sync.Mutex
	headerSlot   StateHeader
	headerDirty  bool
	headerNotify chan struct{}

	// Footer latest-wins slot (D-2). Protected by footerMu.
	footerMu     sync.Mutex
	footerSlot   StateFooter
	footerDirty  bool
	footerNotify chan struct{}
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

	// Role-to-connection mapping (Serve side). Populated by bindRole when
	// a Ready message is received. Protected by rolesMu.
	rolesMu sync.Mutex
	roles   map[string]*conn
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

// SendStateHeader sends a StateHeader message to the header pane's connection
// using latest-wins semantics (D-2). If the write goroutine is busy, the slot
// is overwritten with the newest value so only the latest is delivered.
// No-op if the header role has not yet bound (before AwaitReady returns).
func (c *Channel) SendStateHeader(msg StateHeader) {
	co, ok := c.connForRole("header")
	if !ok {
		return
	}
	co.headerMu.Lock()
	co.headerSlot = msg
	if !co.headerDirty {
		co.headerDirty = true
		select {
		case co.headerNotify <- struct{}{}:
		default:
			// already signaled; write goroutine will read the latest slot
		}
	}
	co.headerMu.Unlock()
}

// SendStateLog sends a StateLog message to the log pane's connection using
// drop-oldest semantics (D-2). If the 256-deep log channel is full, the
// oldest entry is evicted to make room. Never blocks the caller.
// No-op if the log role has not yet bound.
func (c *Channel) SendStateLog(msg StateLog) {
	co, ok := c.connForRole("log")
	if !ok {
		return
	}
	for {
		select {
		case co.logCh <- msg:
			return
		case <-c.ctx.Done():
			return
		default:
		}
		// Channel full — evict oldest entry, then retry.
		select {
		case <-co.logCh:
		case <-c.ctx.Done():
			return
		default:
		}
	}
}

// SendStateFooter sends a StateFooter message to the footer pane's connection
// using latest-wins semantics (D-2), analogous to SendStateHeader.
// No-op if the footer role has not yet bound.
func (c *Channel) SendStateFooter(msg StateFooter) {
	co, ok := c.connForRole("footer")
	if !ok {
		return
	}
	co.footerMu.Lock()
	co.footerSlot = msg
	if !co.footerDirty {
		co.footerDirty = true
		select {
		case co.footerNotify <- struct{}{}:
		default:
		}
	}
	co.footerMu.Unlock()
}

// connForRole looks up the connection bound to role. Returns (nil, false) if
// no connection has been bound yet (before the Ready handshake for that role).
func (c *Channel) connForRole(role string) (*conn, bool) {
	c.rolesMu.Lock()
	co, ok := c.roles[role]
	c.rolesMu.Unlock()
	return co, ok
}

// bindRole associates co with role. Called by readLoop when a Ready message
// arrives, so SendState* methods can target the correct connection. Safe for
// concurrent calls; later calls for the same role silently overwrite (idempotent
// in the duplicate-Ready case, which notifyReady already handles).
func (c *Channel) bindRole(role string, co *conn) {
	c.rolesMu.Lock()
	c.roles[role] = co
	c.rolesMu.Unlock()
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
		nc:           nc,
		wc:           make(chan Message, writeBufSize),
		logCh:        make(chan StateLog, logBufSize),
		headerNotify: make(chan struct{}, 1),
		footerNotify: make(chan struct{}, 1),
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

	// Write goroutine: consumes all outbound channels and serializes to nc.
	go func() {
		defer c.wg.Done()
		defer c.removeConn(co)
		c.writeLoop(nc, co)
		// Close nc so readLoop's io.ReadFull unblocks promptly on write failure,
		// rather than waiting for ctx cancellation to reach the watcher goroutine.
		_ = nc.Close()
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

func (c *Channel) readLoop(nc net.Conn, co *conn) {
	br := bufio.NewReaderSize(nc, 4096)
	for {
		msg, err := readMessage(br)
		if err != nil {
			return // EOF, closed, or context cancelled
		}
		if r, ok := msg.(Ready); ok {
			c.notifyReady(r.Role)
			c.bindRole(r.Role, co)
		}
		select {
		case c.recv <- msg:
		case <-c.ctx.Done():
			return
		}
	}
}

// notifyReady marks role as ready. Only the canonical display roles
// ("header", "log", "footer") are accepted; unknown roles are silently
// dropped to prevent a misbehaving client from satisfying the handshake.
// If the role was not previously ready it sends a signal to readyNewCh.
// Duplicate calls for the same role are no-ops. Safe to call from any goroutine.
func (c *Channel) notifyReady(role string) {
	switch role {
	case "header", "log", "footer":
	default:
		return
	}
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

// writeLoop serializes all outbound messages to nc. It services four sources:
//   - co.wc: general broadcast messages (WorkspaceDone, etc.)
//   - co.logCh: log-line batches with drop-oldest semantics (D-2)
//   - co.headerNotify + co.headerSlot: latest-wins header state (D-2)
//   - co.footerNotify + co.footerSlot: latest-wins footer state (D-2)
func (c *Channel) writeLoop(nc net.Conn, co *conn) {
	bw := bufio.NewWriterSize(nc, 4096)
	for {
		select {
		case msg := <-co.wc:
			if err := writeMessage(bw, msg); err != nil {
				return
			}
		case msg := <-co.logCh:
			if err := writeMessage(bw, msg); err != nil {
				return
			}
		case <-co.headerNotify:
			co.headerMu.Lock()
			v := co.headerSlot
			dirty := co.headerDirty
			co.headerDirty = false
			co.headerMu.Unlock()
			if dirty {
				if err := writeMessage(bw, v); err != nil {
					return
				}
			}
		case <-co.footerNotify:
			co.footerMu.Lock()
			v := co.footerSlot
			dirty := co.footerDirty
			co.footerDirty = false
			co.footerMu.Unlock()
			if dirty {
				if err := writeMessage(bw, v); err != nil {
					return
				}
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
		roles:      make(map[string]*conn),
	}

	ch.wg.Add(1)
	go ch.acceptLoop(ln)

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
		roles:  make(map[string]*conn),
	}
	ch.startConn(nc)
	return ch, nil
}
