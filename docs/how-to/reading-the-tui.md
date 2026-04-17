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
│ ↑/k up  ↓/j down  n next step  q quit  ralph-tui v0.6.0 │  ← shortcut footer + version
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

### Heartbeat indicator

When ralph-tui is waiting on a `claude` step and no stream-json event arrives for ≥15 seconds, the iteration title appends a `  ⋯ thinking (Ns)` suffix showing how many seconds have elapsed since the last event. The suffix updates in-place every second and disappears as soon as the next event arrives. It is pure view state — never written to the log panel or persisted to disk.

## Region 2 — the log panel

The bulk of the screen. This is a `bubbles/viewport` sub-model that caps at 2000 lines and supports `↑`/`k`/`↓`/`j` vim-style scrolling as well as mouse-wheel scrolling. Content is streamed into it from three sources:

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

5 turns · 3200/1024 tokens (cache: 256/0) · $0.0120000 · 47s

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

total claude spend across 4 step invocations (including 1 retry): 42 turns · 18432/6144 tokens (cache: 512/2048) · $0.0420000 · 3m22s

Ralph completed after 2 iteration(s) and 2 finalizing tasks.
```

| Marker | Purpose |
|--------|---------|
| `<phase name>` + `═` underline (full panel width) | Announces entry into a new phase (Initializing, Iterations, Finalizing) |
| `── Iteration N ─────────────` | Marks the top of each iteration inside the iterations phase |
| `Starting step: <name>` + `─` underline (matching width) | Marks the start of every individual step, in every phase |
| `Captured VAR = "value"` | Logged after any step with `captureAs`, showing the bound value |
| `N turns · in/out tokens (cache: C/R) · $cost · duration` | Per-step summary emitted after each `isClaude: true` step completes; shows token spend, cost, and wall-clock duration for that single invocation |
| `total claude spend across N step invocation[s]...` | Run-level cumulative summary: total token spend, cost, duration, and retry count across all claude steps; omitted when no claude steps ran |
| `Ralph completed after N iteration(s) and M finalizing tasks.` | The final line of the run, written before the workflow goroutine enters `ModeDone` |

Phase banners use `═` (double horizontal) and are full-width; per-step banners use `─` (single horizontal) and match the heading width. This three-tier hierarchy — phase > iteration > step — lets you visually trace where you are in the log at a glance.

### Word-wrap

Lines longer than the viewport width wrap at word boundaries — no content is hidden off the right edge. A token with no spaces (a long URL or a stream-json blob) hard-wraps at the width boundary. Wrapped segments start at column 0 with no hanging indent.

When you resize the terminal, content re-wraps to the new width and the viewport scrolls so the same logical line stays at the top of the visible area, even if it occupied multiple wrapped rows before the resize.

### Scrolling

The log panel accepts `↑`/`k` to scroll up and `↓`/`j` to scroll down while you're in Normal or Done mode. Mouse-wheel and trackpad-gesture scrolling also work — ralph-tui enables `tea.WithMouseCellMotion()` at the program level and `Model.Update` forwards incoming `tea.MouseMsg` events to the log sub-model, where bubbles/viewport's built-in `MouseWheelEnabled` handler scrolls the body by three lines per wheel tick. In Error or QuitConfirm mode, keypresses are consumed by the mode handlers instead; mouse-wheel scrolling still works in every mode.

### Selecting log text to copy

ralph-tui handles mouse selection natively inside the log viewport. `tea.WithMouseCellMotion()` enables application mouse capture so the TUI receives drag events directly — you do not need a terminal modifier key to select text within the log panel.

**In-app mouse selection (recommended):**
- **Left-click and drag** in the log viewport to select text. As you drag, the selected region is highlighted in reverse-video. Dragging past the top or bottom edge auto-scrolls one line per event.
- **Release** to commit the selection. The footer switches to `y copy  esc cancel  drag for new selection`.
- **Shift-click** to extend the committed selection's cursor without moving the anchor.
- Press **`y`** or **`Enter`** to copy. Press **`Esc`** to cancel.

**Terminal text selection (fallback):** if you need to copy text using the terminal's built-in selection mechanism (for example, to capture content outside the log viewport), hold the modifier key that overrides application mouse mode before dragging:

| Platform | Override key | Gesture |
|----------|-------------|---------|
| macOS | `Option` | Hold Option, then drag to select |
| Linux / Windows | `Shift` | Hold Shift, then drag to select |

The modifier key bypass is a standard feature of every mainstream terminal that supports application mouse mode. In-app selection via left-drag is generally preferred because it correctly tracks word-wrapped visual lines and copies raw text without wrap-induced newlines.

## Region 3 — the shortcut footer

A single line assembled in `Model.View()` using Lip Gloss layout: the mode-dependent shortcut bar on the left, a spacer in the middle, and the app version label (`ralph-tui v<semver>`) pinned to the right. The version label is sourced from `internal/version.Version` so the same string is visible both here and via `ralph-tui --version`. See [Versioning](../coding-standards/versioning.md) for the single-source-of-truth rule.

The footer uses a two-tone color scheme: the version label on the right renders in **white**. On the left, for the key-mapping lines (Normal and Error modes), each mapped key token (e.g. `↑/k`, `n`, `q`, `c`, `r`) renders in **white** and its trailing description (e.g. `up`, `next step`, `quit`) renders in **light gray**. For the status-message lines the whole line renders in **white** — with one exception: in the quit-confirm prompt, the embedded `Power-Ralph.9000` substring renders in **green** to match the top-border title's brand color, so the confirmation footer and the title line read as the same app.

### Status-line footer path

When a `statusLine` command is configured in `ralph-steps.json` and its runner has produced output, the footer in Normal mode switches from the standard shortcut bar to a **status-line display**:

```
[status text…]  ? Help  ralph-tui v0.6.0
```

The status text is the sanitized first non-empty line of the most recent command run. It is left-truncated to protect the `? Help` hint and version label. On very narrow terminals the version label may be truncated first; the `? Help` hint is always preserved. During cold-start (before the first successful run), the footer falls back to the standard shortcut bar.

### `? Help` and the help modal

When the status-line footer is active and you press `?`, the TUI enters **ModeHelp**: a centered overlay modal appears showing the keyboard shortcuts for all four modes (Normal, Select, Error, Done). The footer switches to `esc  close` for the duration. Press `<Escape>` to dismiss; press `q` to enter the quit-confirm prompt instead.

The modal is ANSI-aware: it is splice-rendered over the base frame without disturbing the underlying content or color state. On very short terminals (where the modal is taller than the frame), the bottom border and `esc  close` footer row are always pinned to the last visible rows so the dismissal cue is never hidden.

Non-wheel mouse events (left-click, right-click, middle-click) inside the modal do not transition to ModeSelect. Wheel events still scroll the underlying viewport.

### Footer text by mode

The left-side shortcut bar is the clearest way to tell what state the handler is in:

| Footer text | Mode |
|-------------|------|
| `[status text]  ? Help  [version]` | Normal with status-line active and populated |
| `↑/k up  ↓/j down  v select  n next step  q quit` | Normal — standard shortcut bar (status-line disabled or cold-start) |
| `c continue  r retry  q quit` | Error — a step failed; you need to decide what to do |
| `Skip current step? (y/n, esc to cancel)` | NextConfirm — you pressed `n`, waiting for skip confirmation |
| `Quit Power-Ralph.9000? (y/n, esc to cancel)` | QuitConfirm — you pressed `q`, waiting for quit confirmation |
| `↑/k up  ↓/j down  v select  q quit` | Done — the workflow finished; review output, select text, or press `q` → `y` to exit |
| `hjkl/↑↓←→ extend  0/$ line  ⇧↑↓ line-ext  y copy  esc cancel  q quit` | Select — keyboard cursor visible; move to extend selection, `y` to copy |
| `y copy  esc cancel  drag for new selection` | Select (committed) — shown after a mouse drag release; any key restores the Select footer |
| `esc  close` | Help — the keyboard shortcut modal is open |
| `Quitting...` | Quitting — you confirmed the quit, shutdown is unwinding |

When the workflow finishes normally, the completion summary is written to the log body and the TUI enters `ModeDone`. The process does not exit on its own — press `q` then `y` to exit, giving you time to review the final output. In Done mode you can also press `v` to enter Select mode and select text from the log panel.

### Using Select mode

Enter `ModeSelect` by pressing `v` from Normal or Done mode (keyboard cursor appears at column 0 of the last visible log row), or by left-clicking the log viewport (mouse cursor anchors at the click cell). The footer changes to show the select-mode shortcuts.

Move the keyboard cursor with vim-style keys:

| Keys | Action |
|------|--------|
| `h` / `←` | Move left one column |
| `l` / `→` | Move right one column |
| `j` / `↓` | Move down one row |
| `k` / `↑` | Move up one row |
| `0` / `Home` | Jump to start of current line |
| `$` / `End` | Jump to end of current line |
| `J` / `Shift+↓` | Extend selection down one row |
| `K` / `Shift+↑` | Extend selection up one row |
| `PgDn` / `PgUp` | Move down / up one page |
| `y` / `Enter` | Copy selected text to clipboard and exit Select mode |

The cursor behaves like vim's visual mode: vertical movement remembers the intended column (`virtualCol`) and restores it when returning to a longer line. The viewport auto-scrolls to keep the cursor visible.

Press `y` or `Enter` to copy the selected text to the clipboard and return to Normal or Done mode. A `[copied N chars]` confirmation line appears in the log on success. In headless or SSH environments where no clipboard daemon is available, an OSC 52 escape sequence is sent to the terminal so clipboard-capable terminals (iTerm2, Kitty, Windows Terminal) can still deliver the payload.

Press `Esc` to clear the selection and return to Normal or Done mode without copying. Press `q` to enter the quit confirmation prompt (the selection is cleared automatically).

Note: `v` (keyboard entry) is blocked in Error, QuitConfirm, NextConfirm, and Quitting modes — only Normal and Done accept it. If the log panel is empty, `v` is a no-op. Mouse left-click is similarly ignored in Error, QuitConfirm, NextConfirm, and Quitting modes.

See [Recovering from Step Failures](recovering-from-step-failures.md) for the Error-mode decision tree and [Quitting Gracefully](quitting-gracefully.md) for the quit flow.

For a step-by-step walkthrough of the three common copy paths (mouse drag, keyboard single line, keyboard multi-line), OSC 52 SSH fallback, and Linux clipboard tool requirements, see [Copying Log Text](copying-log-text.md).

## Related documentation

- [Getting Started](getting-started.md) — Install and first-run walk-through
- [Copying Log Text](copying-log-text.md) — Step-by-step walkthroughs for mouse and keyboard selection, OSC 52 fallback, and Linux clipboard dependencies
- [TUI Status Header & Log Display](../features/tui-display.md) — Implementation details: StatusHeader struct, log helpers, terminal width detection
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — Eight-mode state machine that drives the footer
- [Workflow Orchestration](../features/workflow-orchestration.md) — Where the log chrome comes from — what `Run` writes, what `Orchestrate` writes
- [Recovering from Step Failures](recovering-from-step-failures.md) — Error-mode keyboard controls
- [Quitting Gracefully](quitting-gracefully.md) — Quit-confirm, Escape cancel, SIGINT
- [Debugging a Run](debugging-a-run.md) — Reading the persisted log file
