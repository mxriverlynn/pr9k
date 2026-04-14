# Reading the TUI

Ralph-tui streams everything the workflow does into a single terminal view. This guide walks through what each region means so you can read a run at a glance — even when you've scrolled back through a long log. For the Go-level implementation, see [TUI Status Header & Log Display](../features/tui-display.md).

## The three inner regions

The screen is assembled row-by-row in `Model.View()` inside a hand-built rounded frame. The current run state — iteration number, issue ID, phase step — is embedded directly into the top border as the window title, so the inner content starts with the checkbox grid and there is no separate iteration-line row. The three inner regions stack top to bottom:

```
╭── Power-Ralph.9000 — Iteration 2/3 — Issue #42 ─────╮  ← top border + title
│ [✓] Feature work      [✓] Test planning             │
│ [▸] Test writing      [ ] Code review               │  ← checkbox grid
│ [ ] Fix review items  [ ] Close issue               │
│ [ ] Update docs       [ ] Git push                  │
├─────────────────────────────────────────────────────┤  ← HRule (T-junctions)
│                                                     │
│ Iterations                                          │
│ ═════════════════════════════════════════════════   │
│                                                     │  ← log panel
│ ── Iteration 2 ─────────────                        │     (scrollable)
│                                                     │
│ Starting step: Test writing                         │
│ ─────────────────────────                           │
│                                                     │
│ [test-writing subprocess output streams here]       │
│                                                     │
├─────────────────────────────────────────────────────┤  ← HRule (T-junctions)
│ ↑/k up  ↓/j down  n next step  q quit  ralph-tui v0.2.1 │  ← shortcut footer + version
╰─────────────────────────────────────────────────────╯
```

The top border itself is a two-tone colored title: `Power-Ralph.9000` renders in **green** and the iteration detail that follows the ` — ` separator renders in **white**. The log body text renders in **white** to pop against the light-gray frame chrome. The two horizontal rules inside the frame use `├` and `┤` T-junction glyphs so they visually connect to the `│` side borders instead of leaving a gap.

State updates from the orchestration goroutine are sent as typed messages via `HeaderProxy` (which calls `program.Send`) and applied on the Bubble Tea Update goroutine — so header changes appear on the next `View()` render cycle without any shared-memory races. The checkbox grid sits at the top of the inner content; the iteration detail it belongs to is rendered in the top border's title.

## Region 1 — the checkbox grid

The topmost region. Step progress for the *current phase*, laid out as rows of 4 checkboxes each. The grid is sized at startup to fit whichever phase has the most steps. When the workflow enters a new phase, `SetPhaseSteps` swaps the step names into the same slots and trailing slots clear to empty.

The five possible states:

| Marker | Name | Meaning |
|--------|------|---------|
| `[ ] <name>` | Pending | Step hasn't started yet |
| `[▸] <name>` | Active | Currently running |
| `[✓] <name>` | Done | Completed successfully, or user-terminated with `n` (treated as a skip) |
| `[✗] <name>` | Failed | Returned non-zero exit and the user chose `c` to continue past it |
| `[-] <name>` | Skipped | Marked skipped because an earlier step with `breakLoopIfEmpty` exited the iteration |

**Note:** The initialize phase does not update the checkbox grid — it uses a `noopHeader` during `Orchestrate` so initialize step state isn't rendered. Only the iteration line changes during init. Checkbox rendering resumes at the start of the iteration phase.

## The iteration line (embedded in the top border title)

The current phase/step text is rendered as part of the top border's title, not as a separate inner row. The same string that appears after the ` — ` separator in the border is also set as the OS window title via `tea.SetWindowTitle`.

| Phase | Format | Example (border title) |
|-------|--------|------------------------|
| Initialize | `Initializing N/M: <step name>` | `Power-Ralph.9000 — Initializing 1/2: Splash` |
| Iteration (bounded) | `Iteration N/M — Issue #<id>` (issue suffix appears after `ISSUE_ID` is bound) | `Power-Ralph.9000 — Iteration 2/5 — Issue #42` |
| Iteration (unbounded) | `Iteration N — Issue #<id>` (no total when `--iterations 0`) | `Power-Ralph.9000 — Iteration 7 — Issue #91` |
| Iteration (no issue yet) | `Iteration N/M` or `Iteration N` — issue suffix omitted | `Power-Ralph.9000 — Iteration 1/3` |
| Finalize | `Finalizing N/M: <step name>` | `Power-Ralph.9000 — Finalizing 1/3: Deferred work` |

After the finalize phase ends, the title keeps its last finalize value — **the completion summary is not in the header**, it's the last line of the log panel (see Region 2).

## Region 2 — the log panel

The bulk of the screen. This is a `bubbles/viewport` sub-model that caps at 500 lines and supports `↑`/`k`/`↓`/`j` vim-style scrolling as well as mouse-wheel scrolling. Content is streamed into it from three sources:

1. **Subprocess stdout/stderr** — every line a running step emits, streamed in real time via the `sendLine` callback through a buffered channel
2. **Structural chrome** — phase banners, iteration separators, per-step banners, capture logs, and the completion summary, all written by `workflow.Run` or `ui.Orchestrate` via `executor.WriteToLog`
3. **Error lines** — `Error preparing initialize step: ...`, `Error preparing steps: ...`, `Error preparing finalize step: ...` when `buildStep` fails

### The chrome rhythm

Every structural line is separated from its neighbors by a single blank line. The rhythm for a typical run is:

```
Initializing
════════════════════════════════════════

Starting step: Get GitHub user
──────────────────────────────

[init step subprocess output]

Captured GITHUB_USER = "octocat"

Iterations
════════════════════════════════════════

── Iteration 1 ─────────────

Starting step: Get next issue
─────────────────────────────

[step subprocess output]

Captured ISSUE_ID = "42"

Starting step: Feature work
───────────────────────────

[long claude output…]

── Iteration 2 ─────────────

Starting step: Get next issue
─────────────────────────────

[step subprocess output]

Captured ISSUE_ID = "43"

[more steps…]

Finalizing
════════════════════════════════════════

Starting step: Deferred work
────────────────────────────

[final step output]

Ralph completed after 2 iteration(s) and 2 finalizing tasks.
```

| Marker | Purpose |
|--------|---------|
| `<phase name>` + `═` underline (full panel width) | Announces entry into a new phase (Initializing, Iterations, Finalizing) |
| `── Iteration N ─────────────` | Marks the top of each iteration inside the iterations phase |
| `Starting step: <name>` + `─` underline (matching width) | Marks the start of every individual step, in every phase |
| `Captured VAR = "value"` | Logged after any step with `captureAs`, showing the bound value |
| `Ralph completed after N iteration(s) and M finalizing tasks.` | The final line of the run, written before the workflow goroutine calls `program.Quit()` and exits |

Phase banners use `═` (double horizontal) and are full-width; per-step banners use `─` (single horizontal) and match the heading width. This three-tier hierarchy — phase > iteration > step — lets you visually trace where you are in the log at a glance.

### Scrolling

The log panel accepts `↑`/`k` to scroll up and `↓`/`j` to scroll down while you're in Normal or Done mode. Mouse-wheel and trackpad-gesture scrolling also work — ralph-tui enables `tea.WithMouseCellMotion()` at the program level and `Model.Update` forwards incoming `tea.MouseMsg` events to the log sub-model, where bubbles/viewport's built-in `MouseWheelEnabled` handler scrolls the body by three lines per wheel tick. In Error or QuitConfirm mode, keypresses are consumed by the mode handlers instead; mouse-wheel scrolling still works in every mode.

### Selecting log text to copy

`tea.WithMouseCellMotion()` enables application mouse capture, which tells mainstream terminals (iTerm2, Ghostty, Kitty, xterm) to forward mouse drags to the application rather than performing local text selection. As a result, a plain drag no longer selects text in the log panel.

To select and copy text from the log panel, hold the modifier key that overrides the application's mouse capture:

| Platform | Override key | Gesture |
|----------|-------------|---------|
| macOS | `Option` | Hold Option, then drag to select |
| Linux / Windows | `Shift` | Hold Shift, then drag to select |

The modifier key bypass is a standard feature of every mainstream terminal that supports application mouse mode.

## Region 3 — the shortcut footer

A single line assembled in `Model.View()` using Lip Gloss layout: the mode-dependent shortcut bar on the left, a spacer in the middle, and the app version label (`ralph-tui v<semver>`) pinned to the right. The version label is sourced from `internal/version.Version` so the same string is visible both here and via `ralph-tui --version`. See [Versioning](../coding-standards/versioning.md) for the single-source-of-truth rule.

The footer uses a two-tone color scheme: the version label on the right renders in **white**. On the left, for the key-mapping lines (Normal and Error modes), each mapped key token (e.g. `↑/k`, `n`, `q`, `c`, `r`) renders in **white** and its trailing description (e.g. `up`, `next step`, `quit`) renders in **light gray**. For the status-message lines the whole line renders in **white** — with one exception: in the quit-confirm prompt, the embedded `Power-Ralph.9000` substring renders in **green** to match the top-border title's brand color, so the confirmation footer and the title line read as the same app.

The left-side shortcut bar is the clearest way to tell what state the handler is in:

| Footer text | Mode |
|-------------|------|
| `↑/k up  ↓/j down  n next step  q quit` | Normal — a step is running; you can scroll or skip |
| `c continue  r retry  q quit` | Error — a step failed; you need to decide what to do |
| `Quit Power-Ralph.9000? (y/n, esc to cancel)` | QuitConfirm — you pressed `q`, waiting for confirmation |
| `Quitting...` | Quitting — you confirmed the quit, shutdown is unwinding |

When the workflow finishes normally, the completion summary is written to the log body and the process exits on its own — no final keypress required.

See [Recovering from Step Failures](recovering-from-step-failures.md) for the Error-mode decision tree and [Quitting Gracefully](quitting-gracefully.md) for the quit flow.

## Related documentation

- [Getting Started](getting-started.md) — Install and first-run walk-through
- [TUI Status Header & Log Display](../features/tui-display.md) — Implementation details: StatusHeader struct, log helpers, terminal width detection
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — Four-mode state machine that drives the footer
- [Workflow Orchestration](../features/workflow-orchestration.md) — Where the log chrome comes from — what `Run` writes, what `Orchestrate` writes
- [Recovering from Step Failures](recovering-from-step-failures.md) — Error-mode keyboard controls
- [Quitting Gracefully](quitting-gracefully.md) — Quit-confirm, Escape cancel, SIGINT
- [Debugging a Run](debugging-a-run.md) — Reading the persisted log file
