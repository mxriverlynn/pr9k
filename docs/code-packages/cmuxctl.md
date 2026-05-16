# internal/cmuxctl

Package `cmuxctl` provides the cmux JSON-RPC 2.0 client interface and implementations used by pr9k's cmux mode. It exposes the `CmuxClient` interface, a production `RealClient`, a test double `FakeClient`, the five-condition `Preflight` check, and the `RunPhase1` workspace lifecycle.

Source files: `src/internal/cmuxctl/`

## CmuxClient interface

```go
type CmuxClient interface {
    SystemIdentify(ctx context.Context) (Identity, error)
    WorkspaceCurrent(ctx context.Context) (string, error)
    WorkspaceList(ctx context.Context) ([]string, error)
    WorkspaceCreate(ctx context.Context, name string) error
    WorkspaceClose(ctx context.Context, name string) error
    WorkspaceSelect(ctx context.Context, name string) error
    SurfaceSplit(ctx context.Context, opts SplitOpts) (string, error)
    SurfaceSpawn(ctx context.Context, paneID string, argv []string) error
    SurfaceHide(ctx context.Context, paneID string) error
    SurfaceList(ctx context.Context, workspaceName string) ([]PaneInfo, error)
}
```

All methods accept a context for cancellation and deadline propagation. The interface wraps the ten JSON-RPC 2.0 methods that Phase 1 requires: `system.identify`, `workspace.current`, `workspace.list`, `workspace.create`, `workspace.close`, `workspace.select`, `surface.split`, `surface.spawn`, `surface.hide`, `surface.list`.

**Key types:**

```go
type Identity struct {
    Name    string `json:"name"`
    Version string `json:"version"`
}

type SplitOpts struct {
    PaneID string `json:"pane_id,omitempty"` // empty means the current pane
}

type PaneInfo struct {
    ID     string `json:"id"`
    Exited bool   `json:"exited"`
}
```

D-13 notes: method-presence capability check is accepted; JSON schema validation for individual responses is deferred.

## RealClient

`RealClient` connects to cmux over a Unix socket and issues JSON-RPC 2.0 calls through a **single-goroutine sequential queue** (D-5). Callers submit calls via an unbuffered channel; the queue goroutine serialises all I/O, eliminating the need for connection-level mutexes or request-interleaving guards.

```go
const DefaultTimeout = 8 * time.Second

func NewProductionClient() *RealClient         // uses CMUX_SOCKET_PATH or /run/cmux.sock
func NewRealClient(socketPath string, timeout time.Duration) *RealClient
func (c *RealClient) Stop()                    // shuts down queue goroutine; waits for exit
```

`NewProductionClient` is the only constructor intended for `main()`. `NewRealClient` is for tests that supply a test socket path or a shorter timeout.

**Per-call timeout and reconnect (D-21):** Each call races its I/O sub-goroutine against a `time.Timer` of `c.timeout` duration. On timeout, the socket is closed immediately and the error is returned to the caller. The next call re-dials automatically. This bounds how long a hung cmux call can stall the queue.

`Stop` must be called by the owner when the `RealClient` is no longer needed. It closes the `done` channel (idempotent via `sync.Once`) and waits for the queue goroutine to exit. `Stop` is not part of the `CmuxClient` interface (D-5 / YAGNI-5: deferred until Phase 4 requires concurrent RPCs).

## FakeClient

`FakeClient` is a test double that satisfies `CmuxClient`. Every method is individually scriptable via a corresponding `Func` field; a `nil` func returns the method's zero value and `nil` error.

```go
type FakeClient struct {
    SystemIdentifyFunc   func(ctx context.Context) (Identity, error)
    WorkspaceCurrentFunc func(ctx context.Context) (string, error)
    WorkspaceListFunc    func(ctx context.Context) ([]string, error)
    WorkspaceCreateFunc  func(ctx context.Context, name string) error
    WorkspaceCloseFunc   func(ctx context.Context, name string) error
    WorkspaceSelectFunc  func(ctx context.Context, name string) error
    SurfaceSplitFunc     func(ctx context.Context, opts SplitOpts) (string, error)
    SurfaceSpawnFunc     func(ctx context.Context, paneID string, argv []string) error
    SurfaceHideFunc      func(ctx context.Context, paneID string) error
    SurfaceListFunc      func(ctx context.Context, workspaceName string) ([]PaneInfo, error)

    // Call recorders — appended under mu; read after all goroutines have joined.
    CreateCalls []string
    CloseCalls  []string
    SelectCalls []string
    SpawnCalls  []SpawnCall
    HideCalls   []string
    SplitCalls  []SplitOpts

    // Hang inject seams — both channels must be initialised by the caller.
    HangNext    chan struct{} // send to block the next call
    HangRelease chan struct{} // send to release a blocked call
}
```

All mutable state is protected by an internal `sync.Mutex`. Read recorders only after all goroutines that may call the fake have joined.

**Hang injection:** initialise `HangNext` and `HangRelease` as buffered `chan struct{}` (capacity 1). Sending to `HangNext` before a call causes that call to block on `HangRelease` (or context cancellation). Use this to test timeout and cancellation paths without a real socket.

## Preflight

```go
type CmuxProber interface {
    CmuxBinaryAvailable() bool
}

type RealCmuxProber struct{}
func (RealCmuxProber) CmuxBinaryAvailable() bool

func Preflight(ctx context.Context, prober CmuxProber, client CmuxClient) []error
```

`Preflight` runs the five distinguishable cmux failure-condition checks and validates `CMUX_SOCKET_PATH` per D-15. Returns a non-empty slice of errors on any failure; returns `nil` on success. Checks run sequentially; the first blocking condition short-circuits the rest.

**Five failure conditions:**

| # | Condition | Error message |
|---|---|---|
| 1 | Binary absent | `cmuxctl: cmux is not installed; see the cmux setup how-to` |
| 2 | Socket path unresolvable or absent | `cmuxctl: cmux is installed but not running; start cmux and try again` |
| 3 | Caller is not a cmux descendant (EACCES on dial) | `cmuxctl: cmux mode must be launched from inside a cmux session (socket: <path>)` |
| 4 | Socket disabled in cmux config (ECONNREFUSED) | `cmuxctl: cmux socket is disabled in cmux configuration; re-enable it and try again` |
| 5 | `system.identify` error or unexpected name | `cmuxctl: cmux version is incompatible with pr9k cmux mode: <detail>` |

**`CMUX_SOCKET_PATH` validation (D-15):** Before dialling, `resolveSocketPath` applies three checks:
1. `filepath.EvalSymlinks` — resolves symlinks; `ErrNotExist` maps to condition 2.
2. Socket-type check — the resolved path must be a Unix socket (`fs.ModeSocket`).
3. World-writable parent directory check — rejects sockets in directories writable by all users (SEC-003 mitigation).

**Residual risk — symlink-swap window:** `resolveSocketPath` validates the *resolved* canonical path, but `NewProductionClient` independently re-reads `CMUX_SOCKET_PATH` and dials the *unresolved* value. A symlink swap between preflight completion and the first `net.Dial` could redirect the connection to a different socket, defeating the SEC-003 mitigation. This is accepted as a Phase-1 residual risk: the attack window is very narrow (milliseconds between two reads of the same env var), and the validation still closes the persistent symlink classes. Mitigation (threading the validated canonical path into the client constructor) is deferred to a later phase.

**Terminal injection defence (D-14):** error text from `system.identify` is passed through `ansi.StripAll` before it is included in operator-visible messages. This prevents a compromised or malicious cmux daemon from injecting ANSI escape sequences into the launching terminal.

## RunPhase1

```go
func RunPhase1(
    ctx       context.Context,
    client    CmuxClient,
    projectDir string,
    out       io.Writer,
    dismissalCfg DismissalConfig,
) (returnErr error)
```

Implements the complete Phase 1 workspace lifecycle.

**Setup sequence:**

1. `WorkspaceCurrent` — captures the prior workspace name for focus restore; an error is silently recorded as "no prior workspace" (D-10).
2. `SanitizeBasename(filepath.Base(projectDir))` — produces the sanitized basename; the raw basename is never printed (D-23).
3. `composeWorkspaceName(sanitized)` — forms `pr9k-<sanitized>-<timestamp>`.
4. `WorkspaceCreate` — on `ErrWorkspaceExists`, retries once with a fresh timestamp (D-12). A second collision is a fatal error.
5. Prints `pr9k workspace: <name>\n` to `out`.
6. Spawns four panes orchestrator-first (D-3, D-4):
   - `SurfaceSplit` → `orchPaneID`; `SurfaceSpawn(orchPaneID, ["sh", "-c", "tail -f /dev/null"])`; `SurfaceHide(orchPaneID)`.
   - Three visible panes (header, log, footer) each via `SurfaceSplit` + `SurfaceSpawn` with shell one-liner `printf "<label>\n" && tail -f /dev/null`.
7. Starts `StartDismissalObserver`; blocks on `obs.Ch` or `ctx.Done()`.

**Pane labels (Phase 1 placeholders):**
- `header — Phase 1 placeholder`
- `log — Phase 1 placeholder`
- `footer — Phase 1 placeholder`

**Teardown sequence (wrapped in `sync.Once` — runs exactly once regardless of exit path):**

1. `obs.SetShuttingDown()` + `obs.Cancel()` — stops the observer.
2. Best-effort `WorkspaceClose` with a fresh `context.Background()` (parent ctx may already be cancelled on signal path). On failure, prints orphan diagnostic to `stderr` and sets a non-nil `teardownErr`.
3. Silent `WorkspaceSelect` to prior workspace; any error is ignored (D-10).
4. `obs.Wait()` — joins the observer goroutine.

A deferred wrapper promotes `teardownErr` into `returnErr` when the primary return would otherwise be `nil`.

**Exit-code policy (D-12):** `nil` (exit 0) on successful dismissal. Non-zero on: `WorkspaceClose` failure, partial-setup failure, fatal dismissal (N=3 poll timeouts), or signal-driven context cancellation.

## DismissalObserver

```go
type DismissalConfig struct {
    PollInterval time.Duration // default 500 ms
    PollTimeout  time.Duration // default 5 s (per-call deadline)
    Stderr       io.Writer     // nil means os.Stderr
}

type DismissalEvent struct {
    Fatal bool // true on N=3 consecutive poll timeouts
}

func StartDismissalObserver(
    ctx          context.Context,
    client       CmuxClient,
    workspaceName string,
    cfg          DismissalConfig,
) *DismissalObserver

func (o *DismissalObserver) SetShuttingDown()
func (o *DismissalObserver) Cancel()
func (o *DismissalObserver) Wait()
```

`StartDismissalObserver` launches a goroutine that polls `WorkspaceList` and `SurfaceList` on `cfg.PollInterval` cadence (D-6 single-flight). It fires `obs.Ch` (buffered cap 1) on:

- **Workspace removed** — workspace name absent from `WorkspaceList` result (D-9 arm 1).
- **Any pane exited** — any `PaneInfo.Exited == true` in `SurfaceList` result (D-9 arm 2).
- **N=3 consecutive poll timeouts** — three successive calls that each exceed `cfg.PollTimeout`; fires `DismissalEvent{Fatal: true}` (D-7).

`SetShuttingDown` sets a flag that suppresses a post-shutdown fire (D-8 self-close double-fire mitigation). Callers must call `SetShuttingDown` then `Cancel` before `Wait` to ensure a clean join.

## SanitizeBasename

```go
func SanitizeBasename(s string) string
```

Exported for testing. Applies D-11 sanitization rules:
- Accepted characters: `[a-zA-Z0-9._-]`; anything else becomes `-`.
- Consecutive hyphens collapsed to one.
- Leading and trailing hyphens trimmed.
- Empty result falls back to `"repo"`.

## Testing

Tests live alongside production code (`*_test.go`). All tests run with the race detector (`go test -race`). Key test files:

| File | Coverage |
|---|---|
| `cmuxctl_test.go` | `FakeClient` scripting, `RealClient` JSON-RPC serialisation, timeout and request-serialisation |
| `preflight_test.go` | All five preflight conditions; `CMUX_SOCKET_PATH` validation; socket-type and world-writable-parent checks |
| `dismissal_test.go` | Dismissal observation, consecutive-timeout escalation, `shuttingDown` suppression, race-detector safety |
| `runphase1_test.go` | D-11 sanitisation, collision retry, spawn ordering, teardown sequence, exit codes, `out` writer content |

## Decision cross-references

| Decision | Summary |
|---|---|
| D-5 | Single-goroutine sequential RPC queue for `RealClient` |
| D-6 | 500 ms dismissal polling with single-flight per call |
| D-7 | N=3 consecutive-timeout escalation to fatal dismissal |
| D-8 | `shuttingDown` flag suppresses self-close double-fire |
| D-9 | Two-goroutine watchdog + cleanup signal-handling pattern |
| D-10 | SIGHUP registered; SIGHUP triggers teardown |
| D-11 | Best-effort teardown with orphan diagnostic on `WorkspaceClose` failure |
| D-12 | Exit-code distinction dropped; all normal dismissals return nil |
| D-13 | Method-presence capability check accepted; schema validation deferred |
| D-14 | `ansi.StripAll` applied to cmux error text on diagnostic path |
| D-15 | `CMUX_SOCKET_PATH` validated (EvalSymlinks, socket-type, world-writable parent) before `net.Dial` |
| D-18 | Package layout: interface + `RealClient` + `FakeClient` + `Preflight` + `RunPhase1` |

Full decision text: [implementation-decision-log.md](../plans/cmux-rebuild/phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md)
