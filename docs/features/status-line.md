# Status Line Feature

The status-line feature lets users replace the TUI's default shortcut bar with the output of a custom script, mirroring the behavior of the Claude Code status line. The script receives live workflow state on stdin and prints a single line to stdout; that line appears at the bottom of the TUI next to a `? Help` shortcut.

- **Last Updated:** 2026-04-17
- **Authors:**
  - River Bailey

## Overview

- When a `statusLine` block is present in `config.json`, pr9k launches a background `Runner` that executes the configured command on a schedule and after key workflow events
- The command's first non-empty stdout line is sanitized and displayed in the TUI footer, replacing the shortcut bar in Normal mode
- A `? Help` shortcut appears next to the status text so users can still access the keyboard shortcut modal
- When no `statusLine` block is configured, the TUI footer shows the default shortcut bar (unchanged behavior)
- All exported `Runner` methods are goroutine-safe; the feature imposes no visible latency on the TUI

Key implementation files:

- `src/internal/statusline/` — `Runner`, `State`, `BuildPayload`, `Sanitize`
- `src/internal/ui/model.go` — footer rendering switch, help-modal overlay
- `src/internal/ui/ui.go` — `ModeHelp`, `HelpModeShortcuts`, `StatusLineActive`
- `src/internal/ui/keys.go` — `?` handler, `handleHelp`
- `src/cmd/src/main.go` — wiring (construct Runner, wire sender, set mode getter)
- `src/internal/workflow/run.go` — push closure at every VarTable mutation site

See [`docs/code-packages/statusline.md`](../code-packages/statusline.md) for the package-level reference (Runner API, State, BuildPayload, Sanitize).

## Configuration

Add an optional `statusLine` block to `config.json`:

```json
{
  "statusLine": {
    "type": "command",
    "command": "scripts/statusline",
    "refreshIntervalSeconds": 10
  }
}
```

| Field | Required | Description |
|-------|----------|-------------|
| `type` | No | Must be `"command"` or omitted (reserved for future types) |
| `command` | Yes | Path relative to workflowDir, absolute path, or bare name resolved via PATH |
| `refreshIntervalSeconds` | No | Nil → default 5 s; `0` → disable timer; `>0` → custom interval in seconds |

When absent, `StepFile.StatusLine` is nil and `statusline.New` returns a no-op `Runner` — all methods are safe to call, `Enabled()` returns false, and the TUI footer shows the default shortcut bar.

### Path resolution

`command` is resolved identically to non-claude step `command[0]`:

1. If the value contains `/`, it is joined with `workflowDir` (for relative paths) or used as-is (for absolute paths). The resolved path must exist on disk.
2. Otherwise it is looked up via `exec.LookPath` (bare name on PATH).

## Script contract

### Environment and working directory

- The script runs on the **host** (not inside Docker). It inherits the full host environment, including `ANTHROPIC_API_KEY`, `GITHUB_TOKEN`, `CLAUDE_CONFIG_DIR`, and other secrets present in the shell. This is an explicit trust-model decision — the script is operator-supplied and treated with the same level of trust as the workflow binary itself.
- There is no working-directory guarantee; use absolute paths or paths relative to fields from the stdin payload.

### Stdin

The script receives a single JSON object on stdin:

```json
{
  "sessionId": "20260417-093045-123",
  "version": "0.7.2",
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

| Field | Type | Description |
|-------|------|-------------|
| `sessionId` | string | Timestamp-based session identifier |
| `version` | string | pr9k version (`version.Version`) |
| `phase` | string | `"initialize"`, `"iteration"`, or `"finalize"` |
| `iteration` | int | Current iteration number; `0` outside iteration phase |
| `maxIterations` | int | Maximum iterations (`0` = unbounded) |
| `step.num` | int | Current step number within phase (1-based) |
| `step.count` | int | Total step count in phase |
| `step.name` | string | Step name from `config.json` |
| `mode` | string | Current UI mode: `"normal"`, `"error"`, `"quitconfirm"`, `"nextconfirm"`, `"done"`, `"select"`, `"quitting"`, or `"help"` |
| `workflowDir` | string | Workflow bundle directory |
| `projectDir` | string | Target repository directory |
| `captures` | object | All user-defined captured variables visible in current phase; built-in variables (WORKFLOW_DIR, PROJECT_DIR, MAX_ITER, ITER, STEP_NUM, STEP_COUNT, STEP_NAME) are excluded |

All fields are always present. `captures` is always a JSON object (never null).

**The script must drain stdin before exiting.** If the script exits without reading, pr9k's stdin write blocks until the pipe closes, which triggers the 2-second command timeout. The sample `scripts/statusline` uses `input=$(cat)` to drain and capture stdin for processing.

### Stdout

- The **first non-empty line** of stdout is used as the status text. Remaining lines are discarded.
- Output is capped at **8 KB**. Output beyond 8 KB is truncated and a `[statusline] stdout truncated at 8 KB` line is logged.
- ANSI SGR color codes are preserved. All other control sequences (`\r`, CSI non-SGR, non-OSC-8 OSC, bare ESC, BEL, trailing whitespace) are stripped by `Sanitize`.

### Stderr

Stderr is drained concurrently and forwarded to the pr9k log file with the `[statusline]` step prefix.

### Exit code and timeout

- **Exit 0** — stdout is cached and displayed in the footer. The `StatusLineUpdatedMsg` is sent to trigger a re-render.
- **Non-zero exit** — logged; the previous cached output is preserved unchanged.
- **Timeout (2 s)** — SIGTERM is sent; the process has 1 additional second to exit before SIGKILL. If the command ignores SIGTERM, it receives SIGKILL after the 1-second wait delay.
- **Empty stdout** — the cache is not updated; previously displayed text remains.

## Refresh triggers

The status-line runner is triggered from two sources:

### Workflow-side push closure

After every meaningful `VarTable` mutation in `workflow.Run`, a `push(phase)` closure calls `PushState` then `Trigger` on the runner. The five call sites are:

| Event | VarTable call |
|-------|---------------|
| Phase set | `vt.SetPhase` |
| Iteration number | `vt.SetIteration` |
| Iteration reset | `vt.ResetIteration` |
| Step update | `vt.SetStep` |
| Capture bind | `vt.Bind` |

One initial `PushState` (without a `Trigger`) is emitted after `vars.New` — before any phase is set — so the timer never fires against a zero-value `State`.

### Mode-change choke point

`ui.Model.Update` detects mode transitions via `prevObservedMode` and calls `Runner.Trigger()` exactly once per mode change (keyboard, mouse, or external `SetMode` call). `tea.QuitMsg` is excepted.

### Timer

When `refreshIntervalSeconds > 0` (or nil for the default 5-second interval), a timer goroutine fires `Trigger()` on every tick. Setting `refreshIntervalSeconds: 0` disables the timer entirely.

## Cold-start and footer fallback

Until the first successful (exit 0) script run, `HasOutput()` returns false and the footer shows `NormalShortcuts` (the default shortcut bar). Once the first successful run completes, subsequent `View()` calls show the status text.

## Help modal (`?`)

When a `statusLine` command is configured, `main.go` calls `keyHandler.SetStatusLineActive(true)`, enabling the `?` key in Normal mode:

- Pressing `?` in Normal mode enters `ModeHelp` and opens the help modal
- The footer shows `"esc  close"` while the modal is visible
- The modal body displays a per-mode two-column shortcut grid (Normal, Select, Error, Done sections)
- `<Escape>` from Help restores the previous mode; `q` enters QuitConfirm (previous mode is preserved so Esc from QuitConfirm returns to the pre-Help mode)
- Mouse clicks in ModeHelp do not enter ModeSelect; wheel events are forwarded to the viewport
- `?` is a no-op in all non-Normal modes and when `StatusLineActive()` is false

When no `statusLine` is configured, `?` is always a no-op and ModeHelp is unreachable.

## Concurrency model

- **Single-flight** — an atomic `running` flag prevents overlapping script invocations. If a new trigger arrives while a run is in progress, it is dropped.
- **Drop-on-full** — the trigger channel has capacity 4. Triggers beyond capacity are dropped so slow scripts cannot back-pressure the Bubble Tea goroutine.
- **Mutex-protected cache** — `LastOutput()` and `HasOutput()` read from a mutex-guarded cache that is only written on exit 0.
- **Setter synchronization** — `SetSender` and `SetModeGetter` must be called before `Start`. They are mutex-protected to prevent races if a caller violates this precondition.

## Lifecycle and shutdown

1. `main.go` constructs the `Runner` via `statusline.New(cfg, workflowDir, projectDir, log)`
2. `SetSender` and `SetModeGetter` are configured
3. `Runner.Start(ctx)` is called before `program.Run` — starts the worker goroutine and optional timer goroutine
4. `RunConfig.Runner` is set so the workflow push closure can call `PushState` and `Trigger`
5. After `program.Run` returns, `Runner.Shutdown()` is called — sets a `stopped` atomic flag, cancels the internal context, and waits up to 2 seconds for goroutines to drain. Sends after `Shutdown` returns are dropped.
6. The workflow `<-workflowDone` channel is drained after `Shutdown` returns.

`NewNoOp()` is available for contexts where no command is configured; all methods are no-ops and `Enabled()` returns false.

## Observability

Log lines from the status-line runner are prefixed with `[statusline]`:

- `[statusline] stderr: <text>` — stderr output from the script
- `[statusline] stdout truncated at 8 KB` — output exceeded the 8 KB cap
- `[statusline] error: exit status N` — non-zero exit
- `[statusline] error: signal: killed` — timeout SIGKILL

## Out of scope

The following are not provided by this feature:

- **Sandboxing** — the script runs on the host with the full environment. Isolation (if desired) must be implemented in the script itself.
- **Live config reload** — `SetStatusLineActive` is called once at startup; changes to `config.json` require a restart.
- **Multiple status-line commands** — only one `statusLine` block is supported.
- **Windows** — SIGTERM/SIGKILL semantics and path resolution assume Unix.
- **Scrollable / multi-line output** — only the first non-empty line is displayed.
