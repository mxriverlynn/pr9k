package cmuxctl

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout is the per-call timeout applied to every cmux RPC call.
const DefaultTimeout = 8 * time.Second

// RealClient connects to cmux over a Unix socket and issues cmux v2 calls
// through a single-goroutine sequential queue. On per-call timeout the socket
// is closed; subsequent calls re-dial for the next attempt (D-21).
type RealClient struct {
	socketPath string
	timeout    time.Duration
	calls      chan rpcCall
	done       chan struct{}
	stopOnce   sync.Once
	stopped    chan struct{}
}

var _ CmuxClient = (*RealClient)(nil)

// ---- cmux v2 wire types -----------------------------------------------------

// rpcRequest is the request line. cmux v2 reads id/method/params and ignores
// any extra field, so the legacy "jsonrpc" key is harmless but omitted.
type rpcRequest struct {
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
	ID     int64  `json:"id"`
}

// rpcErrorPayload is the cmux v2 error object. Code is a string
// (e.g. "auth_required", "not_found") — verified at cmux 2f96c15c2.
type rpcErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// rpcResponse is the cmux v2 envelope: {"id","ok":bool,"result"|"error"}.
type rpcResponse struct {
	OK     *bool            `json:"ok"`
	Result json.RawMessage  `json:"result,omitempty"`
	Error  *rpcErrorPayload `json:"error,omitempty"`
	ID     json.RawMessage  `json:"id"`
}

// CmuxError is a structured cmux v2 error response. Callers (preflight)
// classify on Code.
type CmuxError struct {
	Code    string
	Message string
}

func (e *CmuxError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("cmux error %s: %s", e.Code, e.Message)
	}
	return "cmux error " + e.Code
}

// PlaintextError is the bare "ERROR: …" line cmux writes to a non-descendant
// client (default cmuxOnly mode) before reading any request, then closes.
// It is not JSON; preflight classifies it into actionable guidance.
type PlaintextError struct {
	Raw string
}

func (e *PlaintextError) Error() string { return e.Raw }

// IsAccessDenied reports whether a PlaintextError is cmux's descendants-only
// rejection (cmuxOnly mode, caller not a cmux descendant).
func (e *PlaintextError) IsAccessDenied() bool {
	r := e.Raw
	return strings.Contains(r, "Access denied") ||
		strings.Contains(r, "only processes started inside cmux") ||
		strings.Contains(r, "Unable to verify client process")
}

// ---- internal queue types ---------------------------------------------------

type rpcCall struct {
	method string
	params any
	reply  chan rpcResult
}

type rpcResult struct {
	raw json.RawMessage
	err error
}

type ioResult struct {
	resp *rpcResponse
	err  error
}

// ---- constructor / lifecycle ------------------------------------------------

// NewProductionClient creates a RealClient using cmux's socket-discovery
// contract (resolveCmuxSocketPath) and DefaultTimeout.
func NewProductionClient() *RealClient {
	socketPath, _ := resolveCmuxSocketPath(realSocketDeps())
	return NewRealClient(socketPath, DefaultTimeout)
}

// NewRealClient creates a RealClient and starts its internal queue goroutine.
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
func (c *RealClient) Stop() {
	c.stopOnce.Do(func() { close(c.done) })
	<-c.stopped
}

// ---- queue goroutine --------------------------------------------------------

func (c *RealClient) run() {
	defer close(c.stopped)

	var conn net.Conn
	var enc *json.Encoder
	var rd *bufio.Reader
	var dec *json.Decoder
	var nextID int64

	disconnect := func() {
		if conn != nil {
			_ = conn.Close()
			conn = nil
			enc = nil
			rd = nil
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
		// Buffered reader so the response's first byte can be peeked (to detect
		// cmux's pre-request plaintext "ERROR: …" line) without losing bytes for
		// the JSON path. The decoder MUST read through the same buffered reader.
		rd = bufio.NewReader(conn)
		dec = json.NewDecoder(rd)
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
			req := rpcRequest{Method: call.method, Params: call.params, ID: nextID}

			capturedEnc := enc
			capturedRd := rd
			capturedDec := dec
			ioDone := make(chan ioResult, 1)
			go func() {
				if err := capturedEnc.Encode(req); err != nil {
					ioDone <- ioResult{err: fmt.Errorf("cmuxctl: write %s: %w", req.Method, err)}
					return
				}
				// Peek the first non-space byte. cmux's cmuxOnly rejection is a
				// bare "ERROR: …\n" line written before any JSON; classify it.
				if perr := peekPlaintextError(capturedRd); perr != nil {
					ioDone <- ioResult{err: perr}
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
				select {
				case res := <-ioDone:
					c.handleIOResult(call, res, disconnect)
				default:
					disconnect()
					call.reply <- rpcResult{err: &TimeoutError{Method: call.method, Duration: c.timeout}}
				}
			case <-c.done:
				timer.Stop()
				disconnect()
				call.reply <- rpcResult{err: fmt.Errorf("cmuxctl: %s: client stopped", call.method)}
				<-ioDone
				return
			}

		case <-c.done:
			return
		}
	}
}

// peekPlaintextError returns a *PlaintextError if the next response does not
// begin with a JSON value (cmux wrote a bare "ERROR: …" line). It only
// consumes bytes when it returns an error; on nil the buffered bytes remain
// for the JSON decoder.
func peekPlaintextError(rd *bufio.Reader) error {
	// Skip leading ASCII whitespace without consuming the first real byte.
	for i := 1; ; i++ {
		b, err := rd.Peek(i)
		if err != nil {
			// Not enough bytes yet / EOF: let the JSON decoder produce the
			// definitive error (it has the same view of the stream).
			return nil
		}
		ch := b[i-1]
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			continue
		}
		if ch == '{' || ch == '[' {
			return nil // JSON value ahead.
		}
		line, err := rd.ReadString('\n')
		if err != nil && line == "" {
			return nil
		}
		return &PlaintextError{Raw: strings.TrimSpace(line)}
	}
}

func (c *RealClient) handleIOResult(call rpcCall, res ioResult, disconnect func()) {
	if res.err != nil {
		disconnect()
		call.reply <- rpcResult{err: res.err}
		return
	}
	resp := res.resp
	if resp.Error != nil {
		call.reply <- rpcResult{err: &CmuxError{Code: resp.Error.Code, Message: resp.Error.Message}}
		return
	}
	if resp.OK != nil && !*resp.OK {
		call.reply <- rpcResult{err: &CmuxError{Code: "error", Message: "cmux reported ok=false with no error object"}}
		return
	}
	call.reply <- rpcResult{raw: resp.Result}
}

// ---- RPC dispatch -----------------------------------------------------------

func (c *RealClient) do(ctx context.Context, method string, params any) (json.RawMessage, error) {
	call := rpcCall{method: method, params: params, reply: make(chan rpcResult, 1)}
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

// workspaceParam targets a workspace by UUID. cmux's workspace.close/select
// require a UUID in workspace_id; v2ResolveWorkspace (surface.*) also accepts
// it. Ref is sent too for the methods that resolve either.
type workspaceParam struct {
	WorkspaceID  string `json:"workspace_id,omitempty"`
	WorkspaceRef string `json:"workspace_ref,omitempty"`
}

func wsParam(ws Workspace) workspaceParam {
	return workspaceParam{WorkspaceID: ws.ID, WorkspaceRef: ws.Ref}
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

func (c *RealClient) WorkspaceCurrent(ctx context.Context) (Workspace, error) {
	raw, err := c.do(ctx, "workspace.current", nil)
	if err != nil {
		return Workspace{}, err
	}
	var ws Workspace
	if err := json.Unmarshal(raw, &ws); err != nil {
		return Workspace{}, fmt.Errorf("cmuxctl: workspace.current response: %w", err)
	}
	return ws, nil
}

func (c *RealClient) WorkspaceList(ctx context.Context) ([]WorkspaceInfo, error) {
	raw, err := c.do(ctx, "workspace.list", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Workspaces []WorkspaceInfo `json:"workspaces"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("cmuxctl: workspace.list response: %w", err)
	}
	return result.Workspaces, nil
}

func (c *RealClient) WorkspaceCreate(ctx context.Context, opts WorkspaceCreateOpts) (Workspace, error) {
	raw, err := c.do(ctx, "workspace.create", opts)
	if err != nil {
		return Workspace{}, err
	}
	var ws Workspace
	if err := json.Unmarshal(raw, &ws); err != nil {
		return Workspace{}, fmt.Errorf("cmuxctl: workspace.create response: %w", err)
	}
	if ws.ID == "" && ws.Ref == "" {
		return Workspace{}, errors.New("cmuxctl: workspace.create returned no workspace handle")
	}
	return ws, nil
}

func (c *RealClient) WorkspaceClose(ctx context.Context, ws Workspace) error {
	_, err := c.do(ctx, "workspace.close", wsParam(ws))
	return err
}

func (c *RealClient) WorkspaceSelect(ctx context.Context, ws Workspace) error {
	_, err := c.do(ctx, "workspace.select", wsParam(ws))
	return err
}

func (c *RealClient) SurfaceSplit(ctx context.Context, opts SplitOpts) (Surface, error) {
	// cmux surface.split requires "direction"; the workspace is targeted via
	// workspace_id/ref. Flatten into the wire object.
	body := struct {
		WorkspaceID      string         `json:"workspace_id,omitempty"`
		WorkspaceRef     string         `json:"workspace_ref,omitempty"`
		SurfaceID        string         `json:"surface_id,omitempty"`
		Direction        SplitDirection `json:"direction"`
		WorkingDirectory string         `json:"working_directory,omitempty"`
		InitialCommand   string         `json:"initial_command,omitempty"`
	}{
		WorkspaceID:      opts.Workspace.ID,
		WorkspaceRef:     opts.Workspace.Ref,
		SurfaceID:        opts.SurfaceID,
		Direction:        opts.Direction,
		WorkingDirectory: opts.WorkingDirectory,
		InitialCommand:   opts.InitialCommand,
	}
	raw, err := c.do(ctx, "surface.split", body)
	if err != nil {
		return Surface{}, err
	}
	var s Surface
	if err := json.Unmarshal(raw, &s); err != nil {
		return Surface{}, fmt.Errorf("cmuxctl: surface.split response: %w", err)
	}
	return s, nil
}

// setStatusParam is the wire shape for workspace.set_status.
type setStatusParam struct {
	workspaceParam
	Key   string `json:"key"`
	Value string `json:"value"`
}

// clearStatusParam is the wire shape for workspace.clear_status.
type clearStatusParam struct {
	workspaceParam
	Key string `json:"key"`
}

// setProgressParam is the wire shape for workspace.set_progress.
type setProgressParam struct {
	workspaceParam
	Fraction float64 `json:"fraction"`
	Label    string  `json:"label,omitempty"`
}

func (c *RealClient) WorkspaceSetStatus(ctx context.Context, ws Workspace, key, value string) error {
	_, err := c.do(ctx, "workspace.set_status", setStatusParam{workspaceParam: wsParam(ws), Key: key, Value: value})
	return err
}

func (c *RealClient) WorkspaceClearStatus(ctx context.Context, ws Workspace, key string) error {
	_, err := c.do(ctx, "workspace.clear_status", clearStatusParam{workspaceParam: wsParam(ws), Key: key})
	return err
}

func (c *RealClient) WorkspaceSetProgress(ctx context.Context, ws Workspace, fraction float64, label string) error {
	_, err := c.do(ctx, "workspace.set_progress", setProgressParam{workspaceParam: wsParam(ws), Fraction: fraction, Label: label})
	return err
}

func (c *RealClient) WorkspaceClearProgress(ctx context.Context, ws Workspace) error {
	_, err := c.do(ctx, "workspace.clear_progress", wsParam(ws))
	return err
}

func (c *RealClient) SurfaceList(ctx context.Context, ws Workspace) ([]SurfaceInfo, error) {
	raw, err := c.do(ctx, "surface.list", wsParam(ws))
	if err != nil {
		return nil, err
	}
	// cmux v2 surface.list returns {surfaces:[{id,ref,…}]}. There is no
	// per-surface liveness flag; the dismissal observer treats a drop in
	// surface count (or the workspace vanishing) as the dismissal signal.
	var result struct {
		Surfaces []struct {
			ID string `json:"id"`
		} `json:"surfaces"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("cmuxctl: surface.list response: %w", err)
	}
	out := make([]SurfaceInfo, 0, len(result.Surfaces))
	for _, s := range result.Surfaces {
		out = append(out, SurfaceInfo{SurfaceID: s.ID})
	}
	return out, nil
}
