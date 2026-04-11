# Reading the TUI

Ralph-tui streams everything the workflow does into a single terminal view. This guide walks through what each region means so you can read a run at a glance — even when you've scrolled back through a long log. For the Go-level implementation, see [TUI Status Header & Log Display](../features/tui-display.md).

## The four regions

The screen is a single rounded-border `VBox` titled "Ralph" with these children in order:

```
╭─ Ralph ─────────────────────────────────────────────╮
│ [✓] Feature work      [✓] Test planning             │
│ [▸] Test writing      [ ] Code review               │  ← checkbox grid
│ [ ] Review fixes      [ ] Close issue               │
│ [ ] Update docs       [ ] Git push                  │
│─────────────────────────────────────────────────────│  ← HRule
│ Iteration 2/3 — Issue #42                           │  ← iteration line
│─────────────────────────────────────────────────────│  ← HRule
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
│─────────────────────────────────────────────────────│  ← HRule
│ ↑/k up  ↓/j down  n next step  q quit  ralph-tui v0.1.0 │  ← shortcut footer + version
╰─────────────────────────────────────────────────────╯
```

All four regions are bound to pointers: Glyph reads them on every render cycle without any explicit refresh call, so state updates from the workflow goroutine appear on the next frame. The checkbox grid sits at the top of the VBox, with the iteration status line *below* it; both regions are part of the same header state but rendered on opposite sides of an `HRule`.

## Region 1 — the checkbox grid

The topmost region. Step progress for the *current phase*, laid out as rows of 4 checkboxes each. The grid is sized at startup to fit whichever phase has the most steps. When the workflow enters a new phase, `SetPhaseSteps` swaps the step names into the same slots and trailing slots clear to empty.

The five possible states:

| Glyph | Name | Meaning |
|-------|------|---------|
| `[ ] <name>` | Pending | Step hasn't started yet |
| `[▸] <name>` | Active | Currently running |
| `[✓] <name>` | Done | Completed successfully, or user-terminated with `n` (treated as a skip) |
| `[✗] <name>` | Failed | Returned non-zero exit and the user chose `c` to continue past it |
| `[-] <name>` | Skipped | Marked skipped because an earlier step with `breakLoopIfEmpty` exited the iteration |

**Note:** The initialize phase does not update the checkbox grid — it uses a `noopHeader` during `Orchestrate` so initialize step state isn't rendered. Only the iteration line changes during init. Checkbox rendering resumes at the start of the iteration phase.

## Region 2 — the iteration line

A single line *below* the checkbox grid (separated from it by a horizontal rule) that tells you *what* the workflow is doing right now.

| Phase | Format | Example |
|-------|--------|---------|
| Initialize | `Initializing N/M: <step name>` | `Initializing 1/2: Splash` |
| Iteration (bounded) | `Iteration N/M — Issue #<id>` (issue suffix appears after `ISSUE_ID` is bound) | `Iteration 2/5 — Issue #42` |
| Iteration (unbounded) | `Iteration N — Issue #<id>` (no total when `--iterations 0`) | `Iteration 7 — Issue #91` |
| Iteration (no issue yet) | `Iteration N/M` or `Iteration N` — issue suffix omitted | `Iteration 1/3` |
| Finalize | `Finalizing N/M: <step name>` | `Finalizing 1/3: Deferred work` |

After the finalize phase ends, the iteration line keeps its last finalize value — **the completion summary is not in the header**, it's the last line of the log panel (see Region 3).

## Region 3 — the log panel

The bulk of the screen. This is a scrollable `glyph.Log` widget that caps at 500 lines and supports `↑`/`k`/`↓`/`j` vim-style scrolling. Content is streamed into it from three sources:

1. **Subprocess stdout/stderr** — every line a running step emits, streamed in real time through an `io.Pipe`
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
| `Ralph completed after N iteration(s) and M finalizing tasks.` | The final line of the run, written before the workflow waits for the "any key to exit" keypress |

Phase banners use `═` (double horizontal) and are full-width; per-step banners use `─` (single horizontal) and match the heading width. This three-tier hierarchy — phase > iteration > step — lets you visually trace where you are in the log at a glance.

### Scrolling

The log panel accepts `↑`/`k` to scroll up and `↓`/`j` to scroll down while you're in Normal or Done mode. In Error or QuitConfirm mode, keypresses are consumed by the mode handlers instead.

## Region 4 — the shortcut footer

A single line at the bottom of the VBox. It is actually an `HBox` with three children: the mode-dependent shortcut bar on the left, a flex spacer in the middle, and the app version label (`ralph-tui v<semver>`) pinned to the right. The version label is sourced from `internal/version.Version` so the same string is visible both here and via `ralph-tui --version`. See [Versioning](../coding-standards/versioning.md) for the single-source-of-truth rule.

The left-side shortcut bar is the clearest way to tell what state the handler is in:

| Footer text | Mode |
|-------------|------|
| `↑/k up  ↓/j down  n next step  q quit` | Normal — a step is running; you can scroll or skip |
| `c continue  r retry  q quit` | Error — a step failed; you need to decide what to do |
| `Quit ralph? (y/n, esc to cancel)` | QuitConfirm — you pressed `q`, waiting for confirmation |
| `Quitting...` | Quitting — you confirmed the quit, shutdown is unwinding |

When the workflow finishes normally, the completion summary is written to the log body and the process exits on its own — no final keypress required.

See [Recovering from Step Failures](recovering-from-step-failures.md) for the Error-mode decision tree and [Quitting Gracefully](quitting-gracefully.md) for the quit flow.

## Related documentation

- [Getting Started](getting-started.md) — Install and first-run walk-through
- [TUI Status Header & Log Display](../features/tui-display.md) — Implementation details: StatusHeader struct, log helpers, terminal width detection
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — Five-mode state machine that drives the footer
- [Workflow Orchestration](../features/workflow-orchestration.md) — Where the log chrome comes from — what `Run` writes, what `Orchestrate` writes
- [Recovering from Step Failures](recovering-from-step-failures.md) — Error-mode keyboard controls
- [Quitting Gracefully](quitting-gracefully.md) — Quit-confirm, Escape cancel, SIGINT
- [Debugging a Run](debugging-a-run.md) — Reading the persisted log file
