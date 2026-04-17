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

### Schema change to `ralph-steps.json`

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
  be a breaking schema change for every user's `ralph-steps.json`.
- The D13 validator uses `DisallowUnknownFields` on the strict top-level
  struct (`validator.vFile`), so we get future-schema safety for free.

### Why we drop Claude Code's `"type": "command"` discriminator

Claude Code keeps `type` because there is a hypothetical future where the
value could be a different kind of status-line source. ralph-tui has no such
plan. Dropping `type` is the YAGNI move and shortens the config. If we ever
add a second type, `type` can be added as an optional string defaulting to
`"command"` — still additive and non-breaking.

### Deferred fields (explicitly out of scope for MVP)

- `padding` (horizontal indent).
- `subagentStatusLine` (Claude Code has one; ralph-tui has no subagents).
- `type`: see above.

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

### Stdout contract

- **One line.** ralph-tui takes only the first non-empty line of stdout. The
  footer is one row; multi-line output is truncated. (See "Out of scope" for
  the multi-line follow-up.)
- **ANSI colors pass through.** We use `lipgloss.Width`-based truncation, which
  is ANSI-aware. Scripts can emit `\033[32m` et al.
- **OSC 8 hyperlinks** — best-effort. Lip Gloss does not specifically parse
  them, but they are bytes like any other; if the user's terminal supports
  them, they should render. Not tested in MVP.
- **Trailing whitespace stripped.**

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
- **Timeout** (2 seconds, non-configurable in MVP) → SIGTERM the process;
  treat as non-zero exit. 2s is aggressive enough to keep the footer live and
  loose enough to accommodate `git` invocations on a non-huge repo. Tune
  later if needed.
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
2. **Phase boundary** — on `vars.SetPhase` transitions (initialize →
   iteration → finalize).
3. **Iteration boundary** — on `vars.ResetIteration` at the top of each
   iteration.
4. **Step boundary** — before each step begins and after each step ends.
5. **Mode change into/out of `ModeNormal`** — specifically, whenever the
   user transitions `ModeNormal ↔ anything else`. The status line is only
   rendered in `ModeNormal`, but re-running on transition keeps it fresh when
   the user returns.
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

---

## Validation (D13)

Add a new category `statusline` to `internal/validator/validator.go`. Extend
`vFile`:

```go
type vStatusLine struct {
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

1. **`command` non-empty.** Empty string → `statusline: command must not be empty`.
2. **Resolvable binary.** Reuse `validateCommandPath(workflowDir,
   vf.StatusLine.Command)`. Same messages as existing step-command
   validation — this is deliberate for consistency.
3. **`refreshInterval` bounds.** If set, must be `>= 0`. Negative values →
   `statusline: refreshInterval must be >= 0 (0 disables the timer)`.
   Fractional JSON numbers fail parse because the field decodes to `*int`.
   Absent → default `5`. Explicit `0` → timer disabled.
4. **Unknown fields rejected** — `DisallowUnknownFields()` on the parent
   decoder already handles this since `vStatusLine` has no unexported
   embedded types.

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
if mode == ModeNormal && statusLine.Enabled() {
    footer := statusLineText + "  " + "? Help" + spacer + versionLabel
} else if mode == ModeHelp {
    footer := HelpModeShortcuts + spacer + versionLabel
} else {
    // unchanged — existing shortcut rendering path
    footer := shortcutTrunc + spacer + versionLabel
}
```

The `? Help` segment is styled the same way
`colorShortcutLine("?")` would style a key/description group: white `?`,
gray `Help`, single space between them. Two-space gap between the status
line text and `? Help` matches the existing `"  "` separator convention.

Status-line text truncation: reuse the same `lipgloss.NewStyle().MaxWidth()`
pattern that truncates the shortcut line. Compute
`statusWidth = innerWidth - len("? Help") - 2 /*gap*/ - versionWidth - 1 /*final gap*/`.

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
        m.handler.mu.Lock()
        m.handler.prevMode = ModeNormal  // Esc from QuitConfirm should land
                                         // on Normal, not back to Help
        m.handler.mode = ModeQuitConfirm
        m.handler.updateShortcutLineLocked()
        m.handler.mu.Unlock()
    }
    return m, nil
}
```

Wire it into the `keysModel.Update` switch.

`StatusLineActive()` is a new method on `KeyHandler` (or a plumbed bool on
`Model`) that returns whether `ralph-steps.json` declared a `statusLine`.
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

The mode sections are built from the existing shortcut constants
(`NormalShortcuts`, `SelectShortcuts`, `ErrorShortcuts`, `DoneShortcuts`)
*plus* the new `? show help` line for Normal mode. To avoid divergence,
the modal body is built by the same `colorShortcutLine` helper that colors
the footer.

### Sample script

`scripts/statusline` (new, `chmod +x`):

```bash
#!/bin/bash
# ralph-tui status line.
# Claude Code-compatible: reads a single JSON object from stdin.
# For MVP this script ignores the input and prints a static line.
cat >/dev/null
echo "testing status line"
```

The `cat >/dev/null` drain is important: if ralph-tui's pipe fills because
the script exits without reading, the parent goroutine blocks on its
`stdin.Write` call until the pipe closes. Draining stdin is cheap and
correct.

The `ralph-steps.json` shipped in the repo is **not** updated to reference
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
| `ralph-tui/ralph-steps.json`                              | No change — the default workflow does not opt in                 |
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
- `statusline.Runner` — single-flight drops overlapping triggers, not
  overlaps them; timeout kills process and produces fallback; empty stdout
  returns empty; non-zero exit returns last-good; `refreshInterval: 0`
  disables the timer; default `5` fires at the expected cadence.
- `validator` — `statusLine` missing command, bad command path, negative
  `refreshInterval`, explicit zero, absent (default applies), unknown extra
  field, valid config.
- `ui.overlay` — ANSI-aware splice preserves escape codes in the base lines
  not covered by the modal; modal colors survive; no grapheme splitting
  artifacts with wide runes.

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
   is different.
