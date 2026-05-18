package cmuxctl

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout is the per-call timeout applied to every cmux RPC call.
const DefaultTimeout = 8 * time.Second

// RealClient connects to cmux over a Unix socket and issues JSON-RPC 2.0 calls
// through a single-goroutine sequential queue. On per-call timeout the socket
// is closed; subsequent calls re-dial for the next attempt (D-21).
type RealClient struct {
	socketPath string
	timeout    time.Duration
	calls      chan rpcCall  // unbuffered; owned by the run() goroutine
	done       chan struct{} // closed by Stop()
	stopOnce   sync.Once
	stopped    chan struct{} // closed by run() on exit
}

// Compile-time interface assertion — must stay in the production file.
var _ CmuxClient = (*RealClient)(nil)

// ---- JSON-RPC 2.0 wire types ------------------------------------------------

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      int64  `json:"id"`
}

type rpcErrorPayload struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *rpcErrorPayload `json:"error,omitempty"`
	ID      int64            `json:"id"`
}

// ---- Internal queue types ---------------------------------------------------

type rpcCall struct {
	method string
	params any
	reply  chan rpcResult // buffered(1); always written before run() moves on
}

type rpcResult struct {
	raw json.RawMessage
	err error
}

type ioResult struct {
	resp *rpcResponse
	err  error
}

// ---- Constructor and lifecycle ----------------------------------------------

// NewProductionClient creates a RealClient using CMUX_SOCKET_PATH (or the
// platform default) and DefaultTimeout. Intended for use in main() only.
func NewProductionClient() *RealClient {
	socketPath := strings.TrimSpace(os.Getenv("CMUX_SOCKET_PATH"))
	if socketPath == "" {
		socketPath = defaultSocketPath
	}
	return NewRealClient(socketPath, DefaultTimeout)
}

// NewRealClient creates a RealClient and starts its internal queue goroutine.
// timeout is the per-call deadline; use DefaultTimeout for production.
func NewRealClient(socketPath string, timeout time.Duration) *RealClient {
	c := &RealClient{
		socketPath: socketPath,
		timeout:    timeout,
		calls:      make(chan rpcCall),
		done:       make(chan struct{}),
		stopped:    make(chan struct{}),
	}
	go c.run()
	return c
}

// Stop shuts down the internal queue goroutine and waits for it to exit.
// Stop is not on the CmuxClient interface; YAGNI-5 defers interface-level
// lifecycle until Phase 4 requires concurrent RPCs.
func (c *RealClient) Stop() {
	c.stopOnce.Do(func() {
		close(c.done)
	})
	<-c.stopped
}

// ---- Queue goroutine --------------------------------------------------------

func (c *RealClient) run() {
	defer close(c.stopped)

	var conn net.Conn
	var enc *json.Encoder
	var dec *json.Decoder
	var nextID int64

	disconnect := func() {
		if conn != nil {
			conn.Close()
			conn = nil
			enc = nil
			dec = nil
		}
	}
	defer disconnect()

	dial := func() error {
		disconnect()
		var err error
		conn, err = net.DialUnix("unix", nil, &net.UnixAddr{Name: c.socketPath, Net: "unix"})
		if err != nil {
			return fmt.Errorf("cmuxctl: dial %s: %w", c.socketPath, err)
		}
		enc = json.NewEncoder(conn)
		dec = json.NewDecoder(conn)
		return nil
	}

	for {
		select {
		case call := <-c.calls:
			if conn == nil {
				if err := dial(); err != nil {
					call.reply <- rpcResult{err: err}
					continue
				}
			}

			nextID++
			req := rpcRequest{
				JSONRPC: "2.0",
				Method:  call.method,
				Params:  call.params,
				ID:      nextID,
			}

			// Capture encoder/decoder values; the sub-goroutine must not
			// see enc/dec becoming nil if disconnect() fires on timeout.
			capturedEnc := enc
			capturedDec := dec
			ioDone := make(chan ioResult, 1)
			go func() {
				if err := capturedEnc.Encode(req); err != nil {
					ioDone <- ioResult{err: fmt.Errorf("cmuxctl: write %s: %w", req.Method, err)}
					return
				}
				var resp rpcResponse
				if err := capturedDec.Decode(&resp); err != nil {
					ioDone <- ioResult{err: fmt.Errorf("cmuxctl: read %s: %w", req.Method, err)}
					return
				}
				ioDone <- ioResult{resp: &resp}
			}()

			timer := time.NewTimer(c.timeout)
			select {
			case res := <-ioDone:
				timer.Stop()
				c.handleIOResult(call, res, disconnect)
			case <-timer.C:
				// Non-blocking re-check per go-patterns.md: if I/O completed
				// in the same scheduler slice as the timer, treat as done.
				select {
				case res := <-ioDone:
					c.handleIOResult(call, res, disconnect)
				default:
					// True timeout: close socket per D-21.
					disconnect()
					call.reply <- rpcResult{err: fmt.Errorf("cmuxctl: %s timed out after %s", call.method, c.timeout)}
				}
			case <-c.done:
				// Stop() called while call in flight; abandon and exit.
				timer.Stop()
				disconnect()
				call.reply <- rpcResult{err: fmt.Errorf("cmuxctl: %s: client stopped", call.method)}
				return
			}

		case <-c.done:
			return
		}
	}
}

func (c *RealClient) handleIOResult(call rpcCall, res ioResult, disconnect func()) {
	if res.err != nil {
		disconnect()
		call.reply <- rpcResult{err: res.err}
		return
	}
	if res.resp.Error != nil {
		call.reply <- rpcResult{err: fmt.Errorf("cmuxctl: %s: %s", call.method, res.resp.Error.Message)}
		return
	}
	call.reply <- rpcResult{raw: res.resp.Result}
}

// ---- RPC dispatch -----------------------------------------------------------

// do submits an RPC to the queue goroutine and returns the raw JSON result.
func (c *RealClient) do(ctx context.Context, method string, params any) (json.RawMessage, error) {
	call := rpcCall{
		method: method,
		params: params,
		reply:  make(chan rpcResult, 1),
	}
	select {
	case c.calls <- call:
	case <-ctx.Done():
		return nil, fmt.Errorf("cmuxctl: %s: %w", method, ctx.Err())
	case <-c.done:
		return nil, fmt.Errorf("cmuxctl: %s: client stopped", method)
	}
	select {
	case res := <-call.reply:
		return res.raw, res.err
	case <-ctx.Done():
		return nil, fmt.Errorf("cmuxctl: %s: %w", method, ctx.Err())
	case <-c.done:
		return nil, fmt.Errorf("cmuxctl: %s: client stopped", method)
	}
}

// ---- RPC param helpers ------------------------------------------------------

type nameParam struct {
	Name string `json:"name"`
}

type paneIDParam struct {
	PaneID string `json:"pane_id"`
}

type spawnParam struct {
	PaneID string            `json:"pane_id"`
	Argv   []string          `json:"argv"`
	Env    map[string]string `json:"env,omitempty"`
}

type surfaceListParam struct {
	WorkspaceName string `json:"workspace_name"`
}

// ---- CmuxClient methods -----------------------------------------------------

func (c *RealClient) SystemIdentify(ctx context.Context) (Identity, error) {
	raw, err := c.do(ctx, "system.identify", nil)
	if err != nil {
		return Identity{}, err
	}
	var id Identity
	if err := json.Unmarshal(raw, &id); err != nil {
		return Identity{}, fmt.Errorf("cmuxctl: system.identify response: %w", err)
	}
	return id, nil
}

func (c *RealClient) WorkspaceCurrent(ctx context.Context) (string, error) {
	raw, err := c.do(ctx, "workspace.current", nil)
	if err != nil {
		return "", err
	}
	var result struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("cmuxctl: workspace.current response: %w", err)
	}
	return result.Name, nil
}

func (c *RealClient) WorkspaceList(ctx context.Context) ([]string, error) {
	raw, err := c.do(ctx, "workspace.list", nil)
	if err != nil {
		return nil, err
	}
	var result []string
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("cmuxctl: workspace.list response: %w", err)
	}
	return result, nil
}

func (c *RealClient) WorkspaceCreate(ctx context.Context, name string) error {
	_, err := c.do(ctx, "workspace.create", nameParam{Name: name})
	return err
}

func (c *RealClient) WorkspaceClose(ctx context.Context, name string) error {
	_, err := c.do(ctx, "workspace.close", nameParam{Name: name})
	return err
}

func (c *RealClient) WorkspaceSelect(ctx context.Context, name string) error {
	_, err := c.do(ctx, "workspace.select", nameParam{Name: name})
	return err
}

func (c *RealClient) SurfaceSplit(ctx context.Context, opts SplitOpts) (string, error) {
	raw, err := c.do(ctx, "surface.split", opts)
	if err != nil {
		return "", err
	}
	var result struct {
		PaneID string `json:"pane_id"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("cmuxctl: surface.split response: %w", err)
	}
	return result.PaneID, nil
}

func (c *RealClient) SurfaceSpawn(ctx context.Context, paneID string, argv []string, env map[string]string) error {
	_, err := c.do(ctx, "surface.spawn", spawnParam{PaneID: paneID, Argv: argv, Env: env})
	return err
}

func (c *RealClient) SurfaceHide(ctx context.Context, paneID string) error {
	_, err := c.do(ctx, "surface.hide", paneIDParam{PaneID: paneID})
	return err
}

func (c *RealClient) SurfaceList(ctx context.Context, workspaceName string) ([]PaneInfo, error) {
	raw, err := c.do(ctx, "surface.list", surfaceListParam{WorkspaceName: workspaceName})
	if err != nil {
		return nil, err
	}
	var result []PaneInfo
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("cmuxctl: surface.list response: %w", err)
	}
	return result, nil
}
