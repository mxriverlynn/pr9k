# Status Line

The `internal/statusline` package implements a user-configured command that runs on a schedule and displays its output in the TUI footer. It provides the `Runner` struct, immutable `State` snapshots, a JSON payload builder for stdin delivery, and an ANSI sanitizer for safe output display.

- **Last Updated:** 2026-04-17
- **Authors:**
  - River Bailey

## Overview

- A status-line command is defined in the optional top-level `statusLine` block in `ralph-steps.json`
- The command runs as a subprocess in a background goroutine; it is not sandboxed (it inherits the full host environment)
- The command receives workflow state as JSON on stdin and writes its output to stdout; the first non-empty line is sanitized and cached
- Refreshes are triggered at phase boundaries, iteration boundaries, step boundaries, mode changes, and on a configurable timer
- All exported methods on `Runner` are goroutine-safe; `State` is an immutable copy-by-value snapshot

Key files:

- `ralph-tui/internal/statusline/statusline.go` — `Config`, `Runner`, `NewNoOp`, `New`, `Start`, `Shutdown`, `Trigger`, `PushState`, `LastOutput`, `HasOutput`, `SetSender`, `SetModeGetter`, `DefaultRefreshInterval`
- `ralph-tui/internal/statusline/state.go` — `State` struct (immutable snapshot)
- `ralph-tui/internal/statusline/payload.go` — `BuildPayload` (stdin JSON marshaling)
- `ralph-tui/internal/statusline/sanitize.go` — `Sanitize` (ANSI control-sequence filtering)
- `ralph-tui/internal/statusline/statusline_test.go` — All unit tests

## Architecture

```
workflow goroutine                    statusline worker goroutine
─────────────────                    ───────────────────────────
VarTable.Captures  ──copy──▶  State
                                │
runner.PushState(s) ──────────▶│ stateMu (snapshot store)
runner.Trigger()  ──▶ triggerCh│
                                │
                          execScript()
                          ┌─────────────────────────────────────┐
                          │  Build stdin JSON (BuildPayload)    │
                          │  Exec command (2 s timeout)         │
                          │  Read stdout (≤ 8 KB LimitReader)   │
                          │  Drain stderr concurrently (WaitGroup)│
                          │  On exit 0: firstNonEmptyLine       │
                          │           → Sanitize → cache update │
                          │  On non-zero: log error, keep cache │
                          └─────────────────────────────────────┘
                                │
                         program.Send(StatusLineUpdatedMsg)
                         (dropped if runner is stopped)

timer goroutine (optional, interval > 0):
  tick every RefreshIntervalSeconds → runner.Trigger()
```

## Refresh Triggers

The status-line runner is driven by two independent trigger sources:

### Workflow-side push closure

After every meaningful VarTable mutation in `workflow.Run`, a `push(phase)` closure calls `cfg.Runner.PushState(buildState(vt, phase, ...))` then `cfg.Runner.Trigger()`. The five call sites are:

| Event | VarTable call | push call |
|---|---|---|
| Phase set | `vt.SetPhase` | `push(phase)` |
| Iteration number | `vt.SetIteration` | `push(vars.Iteration)` |
| Iteration reset | `vt.ResetIteration` | `push(vars.Iteration)` |
| Step update | `vt.SetStep` | `push(phase)` |
| Capture bind | `vt.Bind` | `push(phase)` |

In addition, one initial `PushState` (without a `Trigger`) is emitted immediately after `vars.New` — before any `vt.SetPhase` call — so the timer goroutine never fires against a zero-value `State`. The invariant is: `triggers == len(pushes) − 1`.

`buildState` is phase-pure: it accepts phase as a parameter and does not consult any internal phase field on `VarTable`, so the initial push is safe to call before `vt.SetPhase`.

### Model-side mode-change choke point

`ui.Model.Update` detects mode transitions via a `prevObservedMode` field that stores the mode observed at the end of each `Update` call. When the mode changes between two consecutive `Update` calls — whether from keyboard, mouse, or an external `SetMode` call by the orchestration goroutine — the trigger function installed via `WithModeTrigger` fires exactly once. `tea.QuitMsg` short-circuits before this check; any mode transition that emits `tea.Quit` is reflected by the preceding `Update`, not by the `QuitMsg` handler.

The trigger function must be non-blocking. `Runner.Trigger()` satisfies this via a buffered channel (capacity 4) that drops on full — a slow status-line script cannot back-pressure the bubbletea goroutine.

`WithModeTrigger` follows the same returning-Model builder shape as `WithHeartbeat`. Both `cfg.Runner == nil` and `triggerFn == nil` are safe. `main.go` wires the runner's `Trigger` method as the mode-transition callback via `WithModeTrigger(statusRunner.Trigger)`.

## Configuration

The `statusLine` block is optional. When absent, `StepFile.StatusLine` is nil and `New` returns a no-op `Runner`.

```json
{
  "statusLine": {
    "type": "command",
    "command": "scripts/my-statusline",
    "refreshIntervalSeconds": 10
  }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `type` | No | Must be `"command"` or omitted (forward-compat) |
| `command` | Yes | Path relative to workflowDir, absolute path, or bare name resolved via PATH |
| `refreshIntervalSeconds` | No | Nil → default 5 s; `0` → disable timer; `>0` → custom interval |

## Core Types

### Config

```go
type Config struct {
    Command                string
    RefreshIntervalSeconds *int
}
```

`Config` is populated from `StatusLineConfig` (steps package) by the caller before passing to `New`. The `Command` field holds the already-resolved command string.

### Runner

```go
func New(cfg *Config, workflowDir, projectDir string, log *logger.Logger) *Runner
func NewNoOp() *Runner

func (r *Runner) Enabled() bool
func (r *Runner) Start(ctx context.Context)
func (r *Runner) Shutdown()
func (r *Runner) Trigger()
func (r *Runner) PushState(s State)
func (r *Runner) LastOutput() string
func (r *Runner) HasOutput() bool
func (r *Runner) SetSender(fn func(interface{}))
func (r *Runner) SetModeGetter(fn func() string)

const DefaultRefreshInterval = 5 * time.Second
```

`New` returns a no-op `Runner` (with `Enabled() == false`) if `cfg` is nil or if the command cannot be resolved. All methods are safe to call on a no-op runner.

### StatusLineUpdatedMsg

```go
type StatusLineUpdatedMsg struct{}
```

Sent to the Bubble Tea program via the injected sender after each successful (exit 0) script run. `Model.Update` receives this to refresh the footer.

### State

```go
type State struct {
    SessionID     string
    Version       string
    Phase         string
    Iteration     int
    MaxIterations int
    StepNum       int
    StepCount     int
    StepName      string
    WorkflowDir   string
    ProjectDir    string
    Captures      map[string]string
}
```

`State` is a copy-by-value snapshot. The workflow goroutine must supply a defensive copy of `VarTable.Captures` before calling `PushState` — `State` and `BuildPayload` do not copy the map internally.

## Stdin Payload

`BuildPayload(s State, mode string) ([]byte, error)` marshals State and the current UI mode into the JSON delivered to the command's stdin:

```json
{
  "sessionId": "20260417-093045-123",
  "version": "0.6.0",
  "phase": "iteration",
  "iteration": 1,
  "maxIterations": 3,
  "step": { "num": 4, "count": 10, "name": "test-planning" },
  "mode": "normal",
  "workflowDir": "/home/user/.local/bin",
  "projectDir": "/home/user/myrepo",
  "captures": { "ISSUE_ID": "42", "GITHUB_USER": "river" }
}
```

All fields are always present. `captures` is always a JSON object (never null). `iteration` is 0 when outside the iteration phase.

## Output Sanitization

`Sanitize(b []byte) string` strips control sequences that could corrupt the TUI:

| Sequence | Outcome |
|----------|---------|
| `\r` (CR) | Stripped |
| CSI sequences (`\x1b[…`) except SGR `m` | Stripped |
| OSC sequences except OSC 8 hyperlinks | Stripped |
| Bare `\x1b` not starting `[` or `]` | Stripped |
| BEL (`\x07`) | Stripped |
| Trailing space / tab | Stripped |
| SGR color codes (`\x1b[32m` etc.) | Preserved |
| OSC 8 hyperlinks (well-formed) | Preserved (best-effort) |

Callers should pass a single pre-split line. Truncated or malformed sequences at EOF do not panic.

## Execution Details

- **Timeout**: 2-second `context.WithTimeout`; SIGTERM on cancel; 1-second `cmd.WaitDelay` escalates to SIGKILL.
- **Stdout limit**: `io.LimitReader` at 8 KB; truncation is logged as `[statusline] stdout truncated at 8 KB`.
- **Stderr**: drained concurrently with stdout via `sync.WaitGroup`; forwarded to the file logger with `[statusline]` prefix.
- **Single-flight**: `atomic.Bool running` prevents concurrent invocations; overlapping triggers are dropped (not queued).
- **Cache**: updated only on exit 0 with non-empty output. Non-zero exits log the error and keep the previous cache.
- **Environment**: the command inherits `os.Environ()` (full host environment), including any secrets present in the shell (e.g., `GITHUB_TOKEN`, `ANTHROPIC_API_KEY`). Status-line scripts are user-authored config; this is an explicit trust-model decision.

## Synchronization

| Field | Guard | Writers | Readers |
|-------|-------|---------|---------|
| `sender`, `modeGetter` | `setterMu` (RWMutex) | `SetSender`, `SetModeGetter` | `execScript` (RLock) |
| cached `State` | `stateMu` (RWMutex) | `PushState` | `execScript` (RLock) |
| `lastOutput`, `hasOutput` | `outputMu` (RWMutex) | `execScript` | `LastOutput`, `HasOutput` |
| `running` | `atomic.Bool` | `execScript` (CAS) | `execScript` (CAS) |
| `stopped` | `atomic.Bool` | `Shutdown` | worker goroutine |

## Lifecycle

1. **Before `Start`**: call `SetSender` and `SetModeGetter` (no synchronization required if done before `Start`).
2. **`Start(ctx)`**: launches the worker goroutine; launches an optional timer goroutine if `RefreshIntervalSeconds != 0`.
3. **During run**: the workflow goroutine calls `PushState(s)` then `Trigger()` at phase/iteration/step boundaries; the UI goroutine calls `Trigger()` on mode changes.
4. **`Shutdown()`**: sets `stopped`, cancels the internal context, waits up to 2 s for the worker to drain. Must be called from `main.go` after `program.Run()` returns — **not** from a workflow goroutine defer — to avoid sending messages to a killed Bubble Tea program.

## TUI Display Contract

The status-line feature integrates with the TUI through three touchpoints: the `StatusReader` interface, the footer switch, and the `? Help` trigger.

### StatusReader interface

`model.go` defines `StatusReader`, a read-only view of runner state:

```go
type StatusReader interface {
    Enabled() bool
    HasOutput() bool
    LastOutput() string
}
```

`*statusline.Runner` satisfies this interface (compile-time assertion in `internal/ui/statusreader.go`). Pass the runner to `Model.WithStatusRunner(r)` to wire it into the TUI. Passing nil (or a disabled runner with `Enabled()==false`) falls back to the shortcut-bar path — no panic.

### Footer switch

`Model.View()` selects one of two footer rendering paths each frame:

1. **Status-line path** — active when `footerMode == ModeNormal && runner.Enabled() && runner.HasOutput()`. Renders: `[truncated status text]  [? Help]  [version]` (status budget computed to preserve the `? Help` and version labels). On very narrow terminals the version label may be silently truncated first; the `? Help` hint is always protected.
2. **Shortcut-bar path** — all other modes and conditions, including ModeHelp itself (which shows `HelpModeShortcuts`). Cold-start (HasOutput=false) always takes this path.

### `? Help` trigger and help modal

When the status-line path is active (runner enabled + has output), `SetStatusLineActive(true)` must be called on the `KeyHandler` so the `?` key transitions to `ModeHelp`. In ModeHelp:

- `Model.View()` renders a centered help modal (overlay-spliced over the base frame) with a four-section shortcut grid for Normal / Select / Error / Done modes and an `esc  close` footer row.
- The footer shows `HelpModeShortcuts` (not the status text) for the duration of ModeHelp.
- `<Escape>` restores the prior mode; `q` enters `ModeQuitConfirm`.
- Non-wheel mouse events (left-, right-, middle-click) in ModeHelp are suppressed and do not transition to ModeSelect. Wheel events still scroll the underlying viewport.

## Cold-Start Behavior

`HasOutput()` returns false until the first exit-0 run. The TUI footer displays the keyboard shortcut bar (not a status-line error message) during cold-start.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Config nil or command unresolvable | `New` returns `NewNoOp()` silently |
| Non-zero exit | Error logged; cached output preserved; `StatusLineUpdatedMsg` not sent |
| Timeout | SIGTERM, then SIGKILL after 1 s; treated as non-zero exit |
| Stdout empty on exit 0 | Cache not updated; footer shows previous output or blank |
| Post-shutdown send | `stopped` flag prevents `program.Send`; message dropped |

## Testing

- `ralph-tui/internal/statusline/statusline_test.go` — Unit tests for all four source files

Tests use an `os.Args[0]`-as-script-stub pattern: the test binary re-invokes itself with a special flag (`-test.run=TestHelperProcess`) to act as a deterministic subprocess without filesystem script fixtures.

## Additional Information

- [Status Line Feature](status-line.md) — User-facing feature doc: config schema, script contract, refresh triggers, help modal, lifecycle, and observability
- [Step Definitions](step-definitions.md) — `StatusLineConfig` struct loaded from `ralph-steps.json`
- [Config Validation](config-validation.md) — Validation rules for the `statusLine` block
- [CLI Configuration & Wiring](cli-configuration.md) — `wiring.go` helpers (`modeString`, `newModeGetter`, `newStatusLineSender`, `buildStatusLineConfig`, `runWithShutdown`) that connect the `Runner` to `main.go`
- [TUI Display](tui-display.md) — `WithModeTrigger` mode-change choke point that fires `Runner.Trigger()` on every UI mode transition
- [Architecture Overview](../architecture.md) — Package dependency graph and system block diagram
- [Concurrency Standards](../coding-standards/concurrency.md) — WaitGroup drain and snapshot-then-unlock patterns used by this package
- [Testing Standards](../coding-standards/testing.md) — `waitCondition` helper usage instead of `time.Sleep`
- [Status Line Design Plan](../plans/status-line/design.md) — Full design spec including refresh trigger matrix and UX decisions
