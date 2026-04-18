# Plan: Status Line for ralph-tui

## Context

ralph-tui currently renders a fixed keyboard-shortcut bar in the footer row, with
the app version label right-aligned:

```
│ ↑/k up  ↓/j down  v select  n next step  q quit                 ralph-tui v0.5.0 │
```

We want the ralph-tui user to be able to replace the shortcut bar with the
output of a user-supplied script, mirroring
[Claude Code's `statusLine`](https://code.claude.com/docs/en/statusline). The
script receives session-state JSON on stdin and writes display text to stdout.

Because the shortcut bar is the only built-in place a user sees the keyboard
shortcuts, hiding it behind a status line forces us to add a discoverable way
to surface the shortcuts on demand: a `?` key opens a "Help: Keyboard
Shortcuts" modal dialog, and Esc closes it.

This plan is deliberately scoped to the *minimum* surface that mirrors Claude
Code's feature. Everything in the "Out of scope (deferred)" section is left to
a follow-up once the MVP is in use.

---

## Target user experience

### Footer states (before and after)

**Before (unchanged for users with no `statusLine`):**

```
│ ↑/k up  ↓/j down  v select  n next step  q quit                 ralph-tui v0.5.0 │
```

**After, with `statusLine` configured, in `ModeNormal`:**

```
│ [Opus] 📁 pr9k | 🌿 main | ⏱ 12m                      ? Help   ralph-tui v0.5.0 │
```

**After, with `statusLine` configured, in any non-Normal mode:** identical to
the "before" row for that mode — `ErrorShortcuts`, `QuitConfirmPrompt`,
`NextConfirmPrompt`, `SelectShortcuts`, `SelectCommittedShortcuts`,
`QuittingLine`, or `DoneShortcuts`. Status line is Normal-mode only.

### Help modal

Pressing `?` in `ModeNormal` (and only when a `statusLine` is configured)
opens a centered modal over the log panel:

```
╭─ Help: Keyboard Shortcuts ────────────────────────────────────────╮
│                                                                   │
│  Normal                                                           │
│    ↑ / k        scroll up              n      skip to next step   │
│    ↓ / j        scroll down            q      quit                │
│    v            enter select mode      ?      show this help      │
│                                                                   │
│  Select                                                           │
│    hjkl / ↑↓←→  extend selection       y      copy selection      │
│    0 / $        to line start / end    esc    cancel selection    │
│    ⇧ ↑ / ⇧ ↓    extend by line         q      quit                │
│                                                                   │
│  Error                                                            │
│    c            continue to next step  r      retry failed step   │
│    q            quit                                              │
│                                                                   │
│  Done                                                             │
│    ↑ / k        scroll up              v      enter select mode   │
│    ↓ / j        scroll down            q      quit                │
│                                                                   │
│                                                       esc  close  │
╰───────────────────────────────────────────────────────────────────╯
```

While the modal is open, the footer row shows a single line:

```
│ esc  close                                                       ralph-tui v0.5.0 │
```

Esc closes the modal and restores the status line. `q` opens the usual
quit-confirm flow (consistent with every other mode).

---

## Configuration

### Schema change to `config.json`

Add one new, optional top-level key: `statusLine`. The value is an object:

```json
{
  "env": ["GH_TOKEN"],
  "statusLine": {
    "command": "scripts/statusline",
    "refreshInterval": 5
  },
  "initialize": [ /* ... */ ],
  "iteration":  [ /* ... */ ],
  "finalize":   [ /* ... */ ]
}
```

| Field             | Required | Type    | Default | Meaning                                                                                                                  |
|-------------------|----------|---------|---------|--------------------------------------------------------------------------------------------------------------------------|
| `command`         | yes      | string  | —       | Path to the executable to run for each status-line refresh. Path resolution rules below.                                 |
| `refreshInterval` | no       | integer | `5`     | Seconds between timer-driven refreshes, in addition to event triggers. `0` disables the timer. Minimum (when `>0`) is `1`. |

**Path resolution** matches the existing `command[0]` resolution in
`ralph-tui/internal/validator/validator.go:394 validateCommandPath`:

- Contains `/` → resolved relative to `WorkflowDir` (or absolute if the path is
  absolute). Must exist at that location.
- Bare name (no `/`) → looked up via `exec.LookPath`.

The initial script this plan ships lives at `scripts/statusline` — same
convention as `scripts/get_next_issue`, `scripts/close_gh_issue`, etc.

### Why an object and not a string

- Mirrors Claude Code's shape (`{ "type": "command", "command": "..." }`),
  reducing surprise for anyone who already knows that feature.
- Leaves room to add `refreshInterval`, `padding`, or other fields later as
  additive, non-breaking changes. Switching a bare string → object later would
  be a breaking schema change for every user's `config.json`.
- The D13 validator uses `DisallowUnknownFields` on the strict top-level
  struct (`validator.vFile`), so we get future-schema safety for free.

### `"type": "command"` accepted but optional (Claude Code copy-paste)

ralph-tui has no other status-line source type and does not plan one.
But `DisallowUnknownFields` would reject a `type` key pasted in from a
Claude Code config, producing a cryptic `json: unknown field "type"`
error that gives the user no hint about compatibility.

Accept `type` as an optional string; validate that, when present, it is
literally `"command"`. Any other value produces a clear error like
`statusline: type must be "command" (or omitted)`. This is additive,
zero future cost, and makes copy-paste from Claude Code work without
surprise.

### Deferred fields (explicitly out of scope for MVP)

- `padding` (horizontal indent).
- `subagentStatusLine` (Claude Code has one; ralph-tui has no subagents).

---

## Script contract

ralph-tui executes the configured `command` under `projectDir` and pipes a
single JSON object to stdin. The script writes one line of plain text to
stdout; ralph-tui paints that line in the footer.

### cwd, env, sandbox

- **cwd**: `projectDir` (same as existing non-claude steps in
  `workflow.Runner.runCommand`). This lets the script `git branch --show-current`
  or any other per-repo query without extra plumbing.
- **env**: inherits `os.Environ()` directly. The status-line script does *not*
  go through the Docker sandbox env allowlist — it runs on the host, same as
  `scripts/get_next_issue`. The sandbox env denylist (`validator.envDenylist`,
  `envSandboxReserved`) is not consulted; it is a sandbox property, not a host
  property.
- **sandbox**: not sandboxed. ralph-tui runs only `isClaude: true` steps in
  Docker (per ADR `20260413160000-require-docker-sandbox.md`, a decision
  specifically for claude steps because of credential + filesystem risk).
  Shell scripts run on the host today and status-line scripts do the same.

### Stdin JSON payload

```json
{
  "sessionId":     "20260417-093045-123",
  "version":       "0.5.0",
  "phase":         "iteration",
  "iteration":     1,
  "maxIterations": 5,
  "step": {
    "num":   3,
    "count": 10,
    "name":  "Feature work"
  },
  "mode":        "normal",
  "workflowDir": "/path/to/bundle",
  "projectDir":  "/path/to/target",
  "captures": {
    "ISSUE_ID":     "42",
    "GITHUB_USER":  "mxriverlynn",
    "STARTING_SHA": "abc1234"
  }
}
```

Field semantics:

| Field            | Source                                                              |
|------------------|---------------------------------------------------------------------|
| `sessionId`      | `logger.RunStamp()` — the existing ms-precision run timestamp       |
| `version`        | `version.Version`                                                   |
| `phase`          | `"initialize"` \| `"iteration"` \| `"finalize"`                     |
| `iteration`      | 1-based iteration number; `0` during `initialize`/`finalize`        |
| `maxIterations`  | value of `--iterations`; `0` means unbounded                        |
| `step.num`       | 1-based index within the current phase                              |
| `step.count`     | total step count in the current phase                               |
| `step.name`      | the step's `name` field                                             |
| `mode`           | current UI mode as a lower-case string (`"normal"`, `"error"`, ...) |
| `workflowDir`    | resolved workflow bundle directory                                  |
| `projectDir`     | resolved target repo directory                                      |
| `captures`       | snapshot of the `VarTable` visible in the current phase             |

All fields are always present (no optional keys). Empty string or `0` stands
in when a value does not apply (e.g. `iteration: 0` outside the iteration
phase). This is a deliberate departure from Claude Code's "omit when absent"
convention — it simplifies script authoring (`jq -r '.iteration'` always
works) at the cost of losing the "was this unset or zero" distinction, which
ralph-tui's state doesn't need.

### Snapshotting the VarTable across goroutines

`vars.VarTable` (in `ralph-tui/internal/vars/vars.go`) has **no mutex**: it
is written only by the workflow goroutine (`SetPhase`, `SetStep`,
`SetIteration`, `ResetIteration`, `Bind`) and read by the same goroutine
when a step is being prepared. The status-line worker runs in a separate
goroutine and cannot call `vt.Get` / `vt.GetInPhase` directly without a
race.

Resolution: take the snapshot on the workflow goroutine and push it into
the status-line package along with the trigger. Concretely:

- Add a `statusline.State` struct (copy by value, immutable) containing
  every field listed in the payload table (phase, iteration, max, step num/
  count/name, workflowDir, projectDir, and a `map[string]string`
  copy of the visible VarTable). Mode is read separately from
  `KeyHandler.Mode()` at invocation time (see "Refresh triggers").
- The workflow goroutine builds the `State` immediately before each event
  trigger (via a helper `buildState(vt, phase, header)`), then calls
  `runner.PushState(State)` followed by `runner.Trigger()`.
- The workflow goroutine must publish an initial `State` via
  `runner.PushState` **before** `runner.Start(ctx)` returns, so the timer
  goroutine never fires against a zero-value State. A simple
  `buildState(vt, vars.Initialize, header)` call right after `vars.New`
  in `workflow.Run` suffices.
- In `workflow.Run`, an explicit `runner.PushState` + `runner.Trigger`
  must follow:
  - `vt.SetPhase` (all 3 call sites at run.go:216, 252, 313)
  - `vt.SetIteration` + `vt.ResetIteration` at run.go:250-252
  - after each `vt.SetStep` (before `Orchestrate`) and after each
    `vt.Bind` (so captured values propagate)
  This is more trigger sites than the original "phase/iteration/step
  boundary" wording implied — the explicit list prevents stale captures.
- The timer goroutine cannot snapshot the VarTable (it does not own it);
  its trigger carries no data, and the worker reuses the most recently
  pushed State via `runner.snapshotState()` (mutex-guarded).

This keeps all `VarTable` reads on the workflow goroutine and eliminates
the race.

### Stdout contract

- **One line.** ralph-tui takes only the first non-empty line of stdout. The
  footer is one row; multi-line output is truncated. (See "Out of scope" for
  the multi-line follow-up.)
- **Bounded read.** Stdout is read with an 8 KB cap using
  `io.LimitReader(stdoutPipe, 8*1024)` so a runaway script cannot flood
  ralph-tui's memory. Anything beyond 8 KB is discarded; the truncation
  point is logged once at `[statusline]` but not surfaced to the footer.
- **Sanitized control characters.** Before caching, strip:
  - `\r` (carriage return) — would overwrite prior content on the same
    row and can corrupt rows above on some terminals.
  - CSI cursor-movement, erase-line, erase-display sequences (every
    `\x1b[…<letter>` that isn't the SGR `m` set).
  - OSC sequences other than OSC 8 hyperlinks (terminated by `BEL` /
    `ST`).
  - Any bare `\x1b` not followed by a recognized escape introducer.
  - Trailing whitespace.

  What remains: SGR color escapes (`\x1b[…m`) and OSC 8 hyperlinks.
  Implementation sketch lives in a new `statusline.Sanitize(b []byte)
  string` helper with direct test coverage for `\r`, CSI `\x1b[2J`,
  CSI `\x1b[2K`, mid-CSI truncation (unterminated `\x1b[3` at EOF),
  and bare BEL/ST.
- **ANSI colors pass through.** After sanitization, surviving SGR
  escapes render via `lipgloss.Width`-aware truncation. Scripts can
  emit `\033[32m` et al.
- **OSC 8 hyperlinks** — best-effort. Lip Gloss does not specifically parse
  them, but they are bytes like any other; if the user's terminal supports
  them, they should render. Not tested in MVP.

### Stderr

Captured and forwarded to the existing file logger (`internal/logger`) with a
`[statusline]` prefix. Not shown in the TUI log panel. A script that writes
only to stderr produces an empty footer and is treated as a soft failure
(see below).

### Exit code, timeout, failure handling

- **Exit 0 + non-empty stdout** → footer shows stdout.
- **Exit 0 + empty stdout** → footer shows nothing (blank, not the shortcut
  bar). This is what Claude Code does; we match it.
- **Non-zero exit** → log the error and fall back to the last successful
  output. If no previous success exists, show a literal `statusline error —
  see log` message (styled gray) so the user knows something broke without
  being spammed every refresh.
- **Timeout** (2 seconds, non-configurable in MVP) → SIGTERM the process.
  If it does not exit within an additional 1 s grace, send SIGKILL.
  Both outcomes are treated as non-zero exit for cache / fallback purposes.
  2s is aggressive enough to keep the footer live and loose enough to
  accommodate `git` invocations on a non-huge repo. Tune later if needed.

  **Implementation:** use Go 1.20+ `exec.Cmd.Cancel` +
  `exec.Cmd.WaitDelay` (the project targets Go 1.26.2, so these are
  available):

  ```go
  ctx, cancel := context.WithTimeout(runnerCtx, 2*time.Second)
  defer cancel()
  cmd := exec.CommandContext(ctx, path, args...)
  cmd.Cancel = func() error {
      // SIGTERM first when the context fires.
      return cmd.Process.Signal(syscall.SIGTERM)
  }
  cmd.WaitDelay = 1 * time.Second // stdlib escalates to SIGKILL if still alive
  ```

  This replaces the "small helper" wording and avoids hand-rolling the
  escalation timer.
- **Stdin-pipe handling**: use `cmd.Stdin = bytes.NewReader(payload)`
  so the stdlib owns the write pump. `bytes.Reader` returns EOF when
  drained, so the child's `stdin` closes cleanly without a separate
  writer goroutine — this eliminates the goroutine-leak risk on a
  script that ignores SIGTERM. (If a future payload grows large
  enough to warrant streaming, switch to `io.Pipe` with
  `WriteCloser.CloseWithError` on timeout.)
- **Missing binary / startup failure** (detected at `exec.Cmd.Start`) →
  surfaced by the validator at startup (see "Validation" below); a runtime
  `Start` failure logs once and uses the same "statusline error" fallback.

### Concurrency (single-flight)

Only one status-line invocation is in flight at a time. If a refresh fires
while one is already running:

- **Drop the new trigger.** (Claude Code cancels the in-flight script; we
  pick the simpler dual-of-that because status-line refreshes are frequent
  but low-value.) The most recent trigger wins in the sense that the *next*
  completed run after the trigger's timestamp will reflect the latest state.

Implemented as a `sync.Mutex` tryLock or an `atomic.Bool` `running` flag on
the status-line package.

---

## Refresh triggers

The status-line script runs on ralph-tui-side events *and* a periodic timer.
MVP triggers:

1. **Startup** — one run immediately after the first frame paints, so the
   footer isn't empty on the first render.

   **Cold-start fallback:** Until the first successful script run
   populates the cache, `Model.View()` renders the ordinary
   `NormalShortcuts` footer (the same bar users see without a status
   line). This preserves keyboard-shortcut discoverability during the
   cold-start window (first ~100 ms up to several seconds on a slow
   script) and avoids a panicked "statusline error — see log"
   appearing before the user realises anything is wrong. The cache
   flips to status-line output only after the first **successful**
   (exit 0) invocation. A failing first run keeps the cold-start
   fallback until success; the error is logged at `[statusline]`.
2. **Phase boundary** — on `vars.SetPhase` transitions (initialize →
   iteration → finalize).
3. **Iteration boundary** — on `vars.ResetIteration` at the top of each
   iteration.
4. **Step boundary** — before each step begins and after each step ends.
5. **Mode change into/out of `ModeNormal`** — specifically, whenever the
   user transitions `ModeNormal ↔ anything else`. The status line is only
   rendered in `ModeNormal`, but re-running on transition keeps it fresh when
   the user returns.

   Implementation note: there are **many** sites that mutate
   `KeyHandler.mode` — `SetMode` on the workflow goroutine, and *every*
   key handler in `keys.go` (Normal, Error, NextConfirm, QuitConfirm,
   Select, Help), plus two mouse-triggered transitions in
   `model.go:127-129` (empty-log revert) and `model.go:233-237` (left-
   press Normal/Done → Select). Scattering trigger calls across all
   these sites guarantees future drift.
   Instead, dispatch the mode-change trigger from the **single choke
   point** that already exists at `model.go:325`:
   `m.prevObservedMode = m.keys.handler.Mode()` at the end of
   `Model.Update`. Change that block to compare the post-dispatch mode
   against the saved `prevObservedMode` and, when they differ, call
   `statusRunner.Trigger()`. This captures *every* mutation path —
   keyboard, mouse, and external `SetMode` — in one spot. The runner
   reads the current mode via `KeyHandler.Mode()` at script invocation
   time, so the trigger carries no payload.
6. **Timer** — every `refreshInterval` seconds (default `5`). This covers
   wall-clock data (elapsed time, idle-while-claude-thinks) and keeps the
   footer current when ralph-tui is blocked on a long-running claude step.
   Set `refreshInterval: 0` to disable the timer entirely and rely on events
   only.

The timer runs in its own goroutine, started in `main.go` alongside the
`HeartbeatTickMsg` ticker. It emits into the same
`statusline.TriggerCh` buffered channel as the event triggers, so
single-flight de-duplication happens in one place. If the user is actively
causing events (step transitions, etc.) the timer adds no extra work because
a trigger is already in flight or queued.

We deliberately *don't* piggyback on the existing `HeartbeatTickMsg` (1 Hz,
`main.go:157`). Firing the script at 1 Hz is wasteful, and the two timers
have different requirements: heartbeat is a render-side tick owned by the
Bubble Tea program, while the status-line timer wants to drop trigger events
into a channel independent of the UI goroutine.

### Plumbing

Trigger delivery is channel-based, to keep the workflow goroutine from
blocking on exec:

```
workflow / keys goroutine ──► statusline.TriggerCh (buffered, cap 4, drop-on-full)
                                       │
                                       ▼
                          statusline worker goroutine
                                       │
                                       ▼
                          exec.Cmd  (stdin JSON, 2s timeout)
                                       │
                                       ▼
                   statusline.SetLastOutput(text)          ← mutex-guarded
                                       │
                                       ▼
                    program.Send(StatusLineUpdatedMsg{})   ← forces a re-render
```

`Model.View()` reads the cached string via `statusline.LastOutput()` under
the status-line package's own mutex — it never blocks on exec.

### Modal-mode edge cases

- **Mouse events in `ModeHelp`.** Wheel events still forward to the
  viewport (log scrolls under the modal) — acceptable because the modal
  is a true overlay. Left-click events that would otherwise enter
  `ModeSelect` are **ignored** when `mode == ModeHelp`; otherwise a
  stray click would flip into a select mode hidden behind the modal.
  Add a guard in `model.go` Mouse handling: `if currentMode == ModeHelp
  { break }` in the non-wheel branch.
- **Help modal reachable from non-Normal modes (deferred).** Today `?`
  is accepted only in `ModeNormal`. Users who lose their keyboard
  shortcut bar (status line active) but are in `ModeDone` have no
  discoverability path to the modal. Acceptable because `ModeDone` still
  renders `DoneShortcuts` in the footer. If user feedback asks for it,
  adding `?` to `handleDone` is a one-line follow-up.
- **Signal-path shutdown short-circuit.** The SIGINT handler in
  `main.go` may `program.Kill()` after a 2 s wait if `workflowDone`
  does not close in time, then `os.Exit(1)`. In that path, deferred
  cleanup does not run — any in-flight status-line subprocess is
  orphaned briefly until the OS reaps the ralph-tui process tree. This
  matches the existing claude-step behaviour and is acceptable for MVP.

### Lifecycle, shutdown, and logger ordering

The status-line runner is a long-lived goroutine (worker) plus an optional
long-lived timer goroutine. Both must stop cleanly when the workflow ends
so they do not outlive the file logger or the Bubble Tea program.

- `statusline.Runner` exposes `Start(ctx context.Context)` and `Shutdown()`.
- `Start` launches the worker loop (reading from `TriggerCh`) and, if
  `refreshInterval > 0`, the timer goroutine.
- `Shutdown` cancels the context and waits (bounded by a 2 s deadline,
  matching the `terminateGracePeriod` in `workflow.Runner`) for the worker
  goroutine to return. If a script is in-flight, the worker signals
  SIGTERM and drains the process before exiting.
- **Shutdown must NOT run inside the workflow goroutine's defer stack.**
  `handleQuitConfirm`'s `y` path returns `tea.QuitMsg`, which causes
  `program.Run()` in `main.go:252` to return **before** the workflow
  goroutine finishes — if Shutdown lived inside that goroutine, the
  worker could still be mid-script and would call `program.Send(...)`
  after `program.Kill()` had already fired. `program.Send` on a killed
  program is a no-op in recent bubbletea but has been observed to panic
  in older versions, so we must not rely on it.

  Concretely:
  1. The worker's `program.Send(StatusLineUpdatedMsg{})` path guards on
     an `atomic.Bool stopped` flag set by Shutdown. If stopped, it
     skips the Send entirely — the cached `LastOutput` is still updated
     under mutex, but the program is not notified.
  2. Shutdown is called from `main.go` **after** `program.Run` returns,
     before the `workflowDone`/`signaled` select:

  ```go
  _, runErr := program.Run()
  signal.Stop(sigChan)
  statusRunner.Shutdown()          // new — block further Sends, stop worker
  // ... existing workflowDone wait, error handling, exit
  ```

  3. The workflow goroutine's inner shutdown order is unchanged from
     today — `log.Close` happens after `workflow.Run` returns normally
     but is independent of the status-line runner:

  ```go
  _ = workflow.Run(runner, proxy, keyHandler, runCfg)
  _ = log.Close()
  close(lineCh)
  keyHandler.SetMode(ui.ModeDone)
  ```

  Because Shutdown blocks until the worker drains (bounded deadline),
  any final `[statusline]` log lines are written before the workflow
  goroutine reaches `log.Close`.

  *Caveat*: the SIGINT path at `main.go:240` calls `program.Kill()`
  and then `os.Exit(1)` — Shutdown is not invoked in that path. Any
  in-flight script is orphaned until the OS reaps the process tree.
  This matches the existing claude-step behaviour on forced exit and
  is acceptable for MVP.

- `refreshInterval: 0` → the timer goroutine is not started at all
  (Start inspects the parsed interval and skips the timer when zero).
- If `statusLine` is absent from `config.json`, `statusRunner` is a
  nil-safe no-op runner (`Start`, `Shutdown`, `Trigger`, `LastOutput`,
  `Enabled` all safe to call). This avoids a sprinkle of nil checks at
  every call site.

---

## Validation (D13)

Add a new category `statusline` to `internal/validator/validator.go`. Extend
`vFile`:

```go
type vStatusLine struct {
    Type            string `json:"type,omitempty"`    // accepted; must be "" or "command"
    Command         string `json:"command"`
    // Pointer so we can distinguish absent (default applies) from
    // explicit 0 (timer disabled).
    RefreshInterval *int   `json:"refreshInterval,omitempty"`
}

type vFile struct {
    Env        *[]string     `json:"env"`
    StatusLine *vStatusLine  `json:"statusLine,omitempty"`
    Initialize *[]vStep      `json:"initialize"`
    Iteration  *[]vStep      `json:"iteration"`
    Finalize   *[]vStep      `json:"finalize"`
}
```

Validation rules when `vf.StatusLine != nil`:

1. **`type` (optional).** Must be `""` (absent) or literally
   `"command"`. Any other value → `statusline: type must be "command"
   (or omitted)`.
2. **`command` non-empty.** Empty string → `statusline: command must not be empty`.
3. **Resolvable binary.** Reuse `validateCommandPath(workflowDir,
   vf.StatusLine.Command)`. Same messages as existing step-command
   validation — this is deliberate for consistency. (Note: the
   runtime `ResolveCommand` in `workflow/run.go` does not re-check
   existence; startup validation is the sole gate.)
4. **`refreshInterval` bounds.** If set, must be `>= 0`. Negative values →
   `statusline: refreshInterval must be >= 0 (0 disables the timer)`.
   Fractional JSON numbers fail parse because the field decodes to `*int`.
   Absent → default `5`. Explicit `0` → timer disabled.
5. **Unknown fields rejected** — `DisallowUnknownFields()` on the parent
   decoder already handles this since `vStatusLine` has no unexported
   embedded types.

**Default hand-off.** The validator stores `RefreshInterval` as a
`*int` (nil when absent). `statusline.Runner.Start` is responsible for
applying the default:

```go
const DefaultRefreshInterval = 5 // seconds
switch {
case cfg.RefreshInterval == nil:
    interval = DefaultRefreshInterval * time.Second
case *cfg.RefreshInterval == 0:
    interval = 0 // no timer goroutine
default:
    interval = time.Duration(*cfg.RefreshInterval) * time.Second
}
```

Unit tests must cover all three branches (nil, zero, positive).

No new variable-scope rules: status-line scripts receive their data via
stdin JSON, not via `{{VAR}}` expansion. The `command` string is *not*
subject to `{{VAR}}` substitution in MVP. (If we add it later, we reuse the
initialize-phase scope since the status line spans all phases.)

Errors surface in the existing "collect all errors, exit non-zero" flow in
`main.go`.

---

## TUI wiring

### New state-machine mode: `ModeHelp`

Add to `internal/ui/ui.go`:

```go
const (
    ModeNormal Mode = iota
    ModeError
    ModeQuitConfirm
    ModeNextConfirm
    ModeDone
    ModeSelect
    ModeQuitting
    ModeHelp      // new — entered via `?` in ModeNormal; esc/q exit
)
```

Shortcut constants:

```go
const (
    HelpModeShortcuts     = "esc  close"
    // Existing constants unchanged; NormalShortcuts stays the same.
)
```

`updateShortcutLineLocked` gains a `case ModeHelp: h.shortcutLine =
HelpModeShortcuts`. All other mode text is unchanged.

### Footer rendering

`Model.View()` (internal/ui/model.go:424) currently renders:

```
footer := shortcutTrunc + spacer + versionLabel
```

The new rule:

```
if mode == ModeNormal && statusLine.Enabled() && statusLine.HasOutput() {
    footer := statusLineText + "  " + "? Help" + spacer + versionLabel
} else {
    // unchanged — existing shortcut rendering path, including ModeHelp
    // (which updateShortcutLineLocked sets to HelpModeShortcuts).
    footer := shortcutTrunc + spacer + versionLabel
}
```

Notes:
- `HasOutput()` returns true only after the first **successful** run
  has populated the cache; before that, ModeNormal still shows
  `NormalShortcuts` (the cold-start fallback from Refresh Triggers #1).
- ModeHelp is rendered by the `else` branch so it goes through
  `colorShortcutLine` with the same white-key / gray-description
  styling as every other mode. No dedicated branch is needed.

The `? Help` segment is styled the same way
`colorShortcutLine("?")` would style a key/description group: white `?`,
gray `Help`, single space between them. Two-space gap between the status
line text and `? Help` matches the existing `"  "` separator convention.

Status-line text truncation: reuse the same `lipgloss.NewStyle().MaxWidth()`
pattern that truncates the shortcut line. Compute width with `lipgloss.Width`
(ANSI-aware) — not `len()` — because the "? Help" hint is coloured
(white `?`, gray `Help`), so its byte length exceeds its visible width.
Concretely:

```go
helpHint := colorShortcutLine("? Help") // white "?", gray "Help"
helpWidth := lipgloss.Width(helpHint)   // = 6 regardless of ANSI bytes
statusWidth := innerWidth - helpWidth - 2 /*gap before Help*/ - versionWidth - 1 /*final gap*/
```

### Key dispatch

`keys.go handleNormal`:

```go
case "?":
    if !m.handler.StatusLineActive() { return m, nil }
    m.handler.mu.Lock()
    m.handler.prevMode = m.handler.mode
    m.handler.mode = ModeHelp
    m.handler.updateShortcutLineLocked()
    m.handler.mu.Unlock()
    return m, nil
```

New `handleHelp`:

```go
func (m keysModel) handleHelp(key tea.KeyMsg) (keysModel, tea.Cmd) {
    switch key.String() {
    case "esc":
        m.handler.mu.Lock()
        m.handler.mode = m.handler.prevMode
        m.handler.updateShortcutLineLocked()
        m.handler.mu.Unlock()
    case "q":
        // Preserve the mode the user was in before pressing `?` so that
        // Esc from the QuitConfirm prompt returns to it (today that is
        // always ModeNormal, but a future follow-up allowing `?` from
        // ModeDone needs this to work without editing this handler).
        m.handler.mu.Lock()
        // prevMode is already the pre-Help mode (set by handleNormal's
        // `?` case). Do not overwrite — QuitConfirm.Esc must restore it.
        m.handler.mode = ModeQuitConfirm
        m.handler.updateShortcutLineLocked()
        m.handler.mu.Unlock()
    }
    return m, nil
}
```

Wire it into the `keysModel.Update` switch.

`StatusLineActive()` is a new method on `KeyHandler` (or a plumbed bool on
`Model`) that returns whether `config.json` declared a `statusLine`.
Storing it on `KeyHandler` is consistent with how other UI-level config
lives today — `KeyHandler` already holds the cancel func and shortcut line.

### Help modal rendering

The help modal is rendered as a **true overlay** on top of the main View
output. We build the main frame string (unchanged from today) and then
splice the modal's rows into the corresponding rows of the frame, ANSI-aware.

Algorithm (to live in `internal/ui/model.go` or a new
`internal/ui/overlay.go`):

```go
func overlay(base, modal string, top, left int) string {
    baseLines  := strings.Split(base, "\n")
    modalLines := strings.Split(modal, "\n")
    for i, mline := range modalLines {
        row := top + i
        if row < 0 || row >= len(baseLines) { continue }
        baseLines[row] = spliceAt(baseLines[row], mline, left)
    }
    return strings.Join(baseLines, "\n")
}
```

`spliceAt` walks the base line grapheme-by-grapheme, keeping ANSI escape
sequences intact, and replaces the column range `[left, left + lipgloss.Width(mline))`
with `mline`. Existing ANSI-aware helpers in Lip Gloss
(`lipgloss.Width`, `lipgloss.NewStyle().MaxWidth`) cover the width math.

Modal dimensions:

- **Width**: `min(terminalWidth - 4, 70)` — comfortably readable, with a
  2-column margin on each side.
- **Height**: sized to content (header + one blank + N mode groups + blank +
  footer row). Capped at `terminalHeight - 4` with internal scrolling
  deferred (MVP shows a fixed content set that fits on any reasonable
  terminal).
- **Centered** using integer division of the terminal dimensions.

Modal layout:

```
╭─ Help: Keyboard Shortcuts ──────────────────────╮
│                                                 │
│  <mode section 1>                               │
│                                                 │
│  <mode section 2>                               │
│  ...                                            │
│                                                 │
│                                 esc  close      │
╰─────────────────────────────────────────────────╯
```

The mode sections are **separate constants** from the footer shortcut
constants, because the modal uses more descriptive phrasings ("scroll up"
vs. the footer's "up", "enter select mode" vs. "select"). The MVP
approach is:

- Introduce `HelpModalNormal`, `HelpModalSelect`, `HelpModalError`,
  `HelpModalDone` as separate string constants, each formatted as a
  two-column grid suited to the modal width.
- Render them with `colorShortcutLine` for consistent white-key / gray-
  description coloring.
- Because the modal text and the footer text are maintained separately,
  **add a unit test** that asserts every key token ( `↑`, `k`, `v`, `n`,
  `q`, `c`, `r`, `y`, `esc`, `hjkl`, etc.) that appears in any footer
  constant also appears in the corresponding modal constant and vice
  versa. Adding a new shortcut then requires two edits — but the test
  prevents silent divergence.
- The `?` → `show this help` row is added only to `HelpModalNormal`
  (the only mode where `?` is active).

If a future refactor unifies footer + modal wording, the modal can
re-use the footer constants and drop the duplicate.

### Sample script

`scripts/statusline` (new, `chmod +x`):

```bash
#!/usr/bin/env bash
# ralph-tui status line.
# Claude Code-compatible: reads a single JSON object from stdin.
# For MVP this script ignores the input and prints a static line.
cat >/dev/null
echo "testing status line"
```

Uses `#!/usr/bin/env bash` rather than a hardcoded `/bin/bash` to match
the portability pattern used elsewhere and to support non-POSIX layouts
like NixOS where `/bin/bash` may not exist.

The `cat >/dev/null` drain is important: if ralph-tui's pipe fills because
the script exits without reading, the parent goroutine blocks on its
`stdin.Write` call until the pipe closes. Draining stdin is cheap and
correct.

The `config.json` shipped in the repo is **not** updated to reference
this script by default — out-of-the-box behavior stays exactly as today.
The sample is there for users who opt in.

---

## File changes at a glance

| Path                                                      | Change                                                           |
|-----------------------------------------------------------|------------------------------------------------------------------|
| `ralph-tui/internal/steps/steps.go`                       | Add `StatusLine *StatusLineConfig` field to `StepFile`           |
| `ralph-tui/internal/validator/validator.go`               | Add `vStatusLine`, validate command path, new error category     |
| `ralph-tui/internal/statusline/statusline.go`             | **new** — Runner, LastOutput cache, trigger channel, single-flight |
| `ralph-tui/internal/statusline/payload.go`                | **new** — build stdin JSON from run state                        |
| `ralph-tui/internal/statusline/statusline_test.go`        | **new**                                                          |
| `ralph-tui/internal/ui/ui.go`                             | Add `ModeHelp`, `HelpModeShortcuts`, `StatusLineActive` plumbing |
| `ralph-tui/internal/ui/keys.go`                           | Handle `?` in Normal; add `handleHelp`                           |
| `ralph-tui/internal/ui/model.go`                          | Footer rendering switch; help-modal overlay                      |
| `ralph-tui/internal/ui/overlay.go`                        | **new** — ANSI-aware splice                                      |
| `ralph-tui/internal/ui/messages.go`                       | Add `StatusLineUpdatedMsg`                                       |
| `ralph-tui/internal/workflow/workflow.go`                 | Fire statusline triggers at phase/iteration/step boundaries      |
| `ralph-tui/cmd/ralph-tui/main.go`                         | Construct `statusline.Runner`; wire to Model and program.Send    |
| `scripts/statusline`                                      | **new** — initial "testing status line" script                   |
| `Makefile`                                                | No change — `cp -r scripts bin/scripts` already covers it        |
| `src/config.json`                              | No change — the default workflow does not opt in                 |
| `docs/features/status-line.md`                            | **new** — feature doc                                            |
| `docs/features/tui-display.md`                            | Update footer section to describe conditional rendering          |
| `docs/features/keyboard-input.md`                         | Add `ModeHelp`                                                   |
| `docs/features/config-validation.md`                      | Add statusline category                                          |
| `docs/how-to/configuring-a-status-line.md`                | **new** — user-facing how-to                                     |
| `CLAUDE.md`                                               | Link to the new feature + how-to docs                            |

---

## Testing plan

### Unit

- `statusline/payload` — JSON shape matches expected schema for all three
  phases (initialize produces `iteration: 0`, finalize produces `iteration:
  0`).
- `statusline.Runner`
  - single-flight drops overlapping triggers, not overlaps them
  - timeout kills process and produces fallback
  - timeout path with a script that ignores SIGTERM → SIGKILL lands
    via `cmd.WaitDelay`; no goroutine leak
  - empty stdout returns empty
  - non-zero exit returns last-good (only after first successful run)
  - `refreshInterval` absent → default 5 s
  - `refreshInterval: 0` → no timer goroutine started
  - `refreshInterval: 7` → fires every ~7 s
  - cold-start window: `HasOutput()` is false until first exit-0 run;
    failing first run keeps `HasOutput()` false
  - `program.Send` is skipped after `Shutdown()`
- `statusline.Sanitize` — strips `\r`, `\x1b[2J`, `\x1b[2K`, CSI
  cursor-movement, unterminated `\x1b[3`, bare OSC-ST/BEL; preserves
  SGR `\x1b[32m`; preserves OSC 8 hyperlink round-trip.
- `validator` — `statusLine` missing command, bad command path, negative
  `refreshInterval`, explicit zero, absent (default applies), unknown extra
  field, `type: "command"` accepted, `type: "bogus"` rejected, valid config.
- `ui.overlay` — ANSI-aware splice preserves escape codes in the base lines
  not covered by the modal; modal colors survive; no grapheme splitting
  artifacts with wide runes; emoji at a narrow-terminal splice boundary
  does not mid-cell slice.
- `ui.keys.handleHelp` — `?` enters Help from Normal, Esc returns to
  Normal, `q` enters QuitConfirm with prevMode preserved, Esc from
  QuitConfirm returns to the pre-Help mode (not Help itself).
- `ui` mode-trigger choke point — every one of the seven pre-existing
  mode transitions (keyboard Normal→QuitConfirm, Error→QuitConfirm,
  NextConfirm→Normal, Select→Normal, mouse-press Normal→Select, mouse-
  press Done→Select, empty-log Select→Normal) drives exactly one
  `statusRunner.Trigger` call.
- `ui` footer — narrow terminal (innerWidth=30) truncates status-line
  text to the computed budget; emoji-at-boundary does not produce a
  half-cell.

### Integration

- `statusline` end-to-end: spawn `scripts/statusline`, feed JSON, assert
  "testing status line" appears in cached output. Run on Linux and macOS in
  CI (already exercised by the existing matrix).
- `ui` with `ModeHelp`: `?` → `ModeHelp`; Esc → back to Normal; `q` →
  QuitConfirm → Esc → Normal (not Help); `?` with no status line
  configured is a no-op.
- Footer rendering with and without status line; Normal mode vs. each other
  mode.

### Manual smoke

- Launch `./bin/ralph-tui --iterations 1` with and without `statusLine`.
- Confirm the default footer is unchanged without the opt-in.
- Confirm truncation on a narrow terminal (< 40 cols).

---

## Observability / debugging

- `statusline` stderr → file logger with prefix `[statusline]`. Includes
  exit code and duration.
- Last five statusline invocations kept in memory (command line, exit code,
  duration, truncated stdout) for post-mortem inspection via a debug build
  flag. **Deferred — nice-to-have, not MVP.**

---

## Out of scope (deferred)

These are explicitly excluded from the MVP. Each is a clean additive
follow-up if user demand arises.

| Feature                                  | Reason deferred                                                                    |
|------------------------------------------|------------------------------------------------------------------------------------|
| `padding` field                          | Not needed for the bordered layout we have.                                        |
| `type` discriminator                     | YAGNI; additive later.                                                             |
| Multi-line status lines                  | Requires variable-height footer — a larger layout change.                          |
| OSC 8 hyperlinks first-class support     | Bytes pass through; not tested.                                                    |
| Scrollable help modal                    | Content fits on any reasonable terminal.                                           |
| `{{VAR}}` substitution in `command`      | Scripts get data via stdin JSON already.                                           |
| Windows PowerShell examples              | ralph-tui's existing scripts are all bash; platform scope unchanged.               |
| Subagent-equivalent status line          | ralph-tui has no subagents concept.                                                |
| Docker-sandboxed status-line execution   | Status-line scripts run on the host, same as every other non-claude script.        |
| Debug ring buffer of last N invocations  | Observability polish.                                                              |

---

## Risks and open questions

1. **Single-flight "drop new trigger" semantics** may briefly show stale
   status line info if many events fire faster than the 2s timeout. In
   practice events are seconds-to-minutes apart during a run, so this is
   unlikely. Revisit if users report it.

2. **`? Help` discoverability**. A new user who doesn't read docs will see
   `? Help` on the right of the footer once they configure a status line —
   this is the discoverability. A user who *hasn't* configured a status
   line never sees the hint. That's fine: they still see the full shortcut
   bar they always had.

3. **Help modal content freshness**. The modal body references the same
   shortcut constants the footer uses, so the two stay in sync by
   construction. Adding a new shortcut requires exactly one edit (the
   constant in `ui.go`); the modal picks it up automatically.

4. **Terminal width < modal minimum**. The overlay degrades by shrinking to
   `terminalWidth - 4`; at very narrow widths (< 30 cols) content wraps
   ungracefully. Acceptable — ralph-tui's existing TUI also degrades at
   those widths.

5. **Claude Code compatibility**. We match enough of Claude Code's schema
   and script contract that a user can reuse a simple Claude Code
   status-line script with minor edits. We do *not* promise full
   compatibility — our JSON field set is narrower and our refresh cadence
   is different. We do accept an optional `type: "command"` field so
   direct copy-paste of the config produces a helpful error rather than
   a cryptic unknown-field decode failure.

---

## Review summary

This plan has been through three structured review passes plus full
agent validation (evidence-based-investigator + adversarial-validator).

**Iterations completed:** 3

**Assumptions challenged:**
- Plan claim that `validateCommandPath` and the runtime `ResolveCommand`
  are equivalent (minor — validator gates existence, runtime does not;
  noted inline under Validation).
- Plan claim that the VarTable is safe to read from any goroutine —
  *refuted*; VarTable has no mutex, so snapshots must be produced on
  the workflow goroutine and pushed to the runner.
- Plan claim that mode-change refresh can be driven from a "helper
  invoked in keys.go handlers" — *refuted*; `handleError`,
  `handleNextConfirm`, and mouse-driven mode transitions in `model.go`
  make that approach brittle. Moved to a single choke point at
  `Model.Update`'s `prevObservedMode` comparison.
- Plan claim that `exec.CommandContext` alone covers the
  SIGTERM-then-SIGKILL timeout — *refuted*; must use `cmd.Cancel` +
  `cmd.WaitDelay`.
- Plan claim that stripping "trailing whitespace" was sufficient
  sanitization — *refuted*; a script emitting `\r` or a CSI erase
  sequence could corrupt the whole TUI. Added explicit sanitizer.

**Consolidations made:**
- Removed a dead `else if mode == ModeHelp` branch in the footer
  rendering rule; `updateShortcutLineLocked` already sets the
  ModeHelp string so the shared `else` branch suffices.
- Removed the "two-well-named spots" scattering approach in favour of
  one choke point (see above).

**Ambiguities resolved:**
- Cold-start behaviour: render `NormalShortcuts` until the first
  **successful** script run, not an empty segment.
- `refreshInterval` default hand-off: validator stores `*int`; runner
  applies the default 5 s on nil, disables timer on 0, passes through
  on > 0.
- `type` discriminator: accept as optional `"command"` string for
  Claude Code copy-paste compatibility.
- Shutdown ordering: `statusRunner.Shutdown()` is called from
  `main.go` after `program.Run()` returns, not from the workflow
  goroutine's defer stack.

**Agent validation:**
- Evidence-based investigator confirmed 7/10 codebase claims outright,
  with a minor divergence on ResolveCommand semantics (documented) and
  a line-number imprecision (harmless).
- Adversarial validator surfaced 12 findings; 10 of 12 produced
  concrete plan changes above. The remaining two (F5 dead branch
  styling and F12 bash shebang) were addressed as small direct fixes.
