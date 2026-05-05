# Run-mode TUI breaks on terminal zoom / resize

## Problem Statement

**Symptom.** When the terminal is resized (font zoom in/out, window resize, tab font change), the run-mode TUI's chrome frame is rendered with no top border, and the checkbox grid appears to start mid-screen with rows in apparently shuffled order. The user observed it "after transitioning from initialize to iteration phase" but identified the actual trigger as having zoomed the terminal at some earlier point.

**Expected behavior.** The TUI must re-flow cleanly on every terminal resize event. Either the frame fits inside the new dimensions, or a clear "terminal too small" placeholder is shown — never an oversized frame that gets clipped by the terminal.

**Conditions.**
- Run-mode TUI (`internal/ui`), not the workflow-builder TUI (which already handles this — see E1).
- Triggered any time `m.height < chromeRows + 1`, where `chromeRows = 1 + gridRows + 4` and `gridRows = ceil(maxStepsAcrossPhases / 4)`.
- Phase transition is incidental: the grid row count is allocated once at startup at the maximum across all phases (E4); transitioning from initialize → iteration only fills in previously-empty cells, making the (already-broken) layout visually obvious.

**Impact.** TUI is unusable on small terminals and after some zoom operations until the user happens to resize back to a large enough size. The top border, iteration title, and first grid row(s) get pushed off the top of the alt-screen.

## Evidence Summary

- **E1** — Run-mode TUI lacks a min-size guard. The workflow-builder TUI (`internal/workflowedit/render_frame.go:30`) returns "Terminal too small — resize to at least 60×16" when `msg.Width < uichrome.MinTerminalWidth || msg.Height < uichrome.MinTerminalHeight`. `internal/ui/model.go` has no equivalent. The constants exist (`uichrome/constants.go:5,8`) but `internal/ui` never imports them for guard purposes.

- **E2** — `WindowSizeMsg` clamps viewport to ≥1 but does NOT clamp the total frame to `m.height`. `internal/ui/model.go:334-354`:
  ```go
  gridRows := len(m.header.header.Rows)
  chromeRows := 1 + gridRows + 1 + 1 + 1 + 1
  vpHeight := m.height - chromeRows
  if vpHeight < 1 {
      vpHeight = 1
  }
  ```
  When `m.height ≤ chromeRows`, `vpHeight` is forced to 1, so emitted output is `chromeRows + 1 = gridRows + 7` lines — guaranteed to exceed `m.height`.

- **E3** — `View()` always emits the full chrome unconditionally (`internal/ui/model.go:398-513`). Order: top border, every grid row, hrule, every viewport line, hrule, footer, bottom border. There is no "skip top border / collapse grid" branch for short terminals. Total output length is purely a function of `gridRows + vpHeight + 5`, never of `m.height`.

- **E4** — Header row count is fixed at startup (`cmd/pr9k/main.go:158-159`, `internal/ui/header.go:88-98`):
  ```go
  maxSteps := max(len(stepFile.Initialize), len(stepFile.Iteration), len(stepFile.Finalize))
  header := ui.NewStatusHeader(maxSteps)
  ```
  `SetPhaseSteps` (`header.go:183-198`) only writes/clears cells; it never reallocates `Rows`. **Phase transitions do not change `gridRows`.** This refutes the original "step list grows" hypothesis.

- **E5** — `WindowSizeMsg` is the only place that re-budgets `vpHeight`. `m.log.SetSize` is never called outside the `WindowSizeMsg` branch. Bubble Tea hooks `SIGWINCH` (verified in vendored `bubbletea@v1.3.10/tty.go`) and re-sends `WindowSizeMsg` on every resize, so the resize wiring is correct — the bug is in what the handler does, not whether it fires.

- **E6** — Help modal already has explicit overflow handling (`internal/ui/model.go:528-536`) — `if modalH > m.height && m.height >= 2 { modalLines = modalLines[:m.height]; ...pin bottom rows }`. This same problem in the base frame has no equivalent code, confirming the gap was thought about in one place and missed in another.

- **E7** — `HeaderCols = 4` is a hard-coded constant (`header.go:57`), never adapted to width. Width does not affect `gridRows`.

- **E8** — `WrapLine` (`uichrome/chrome.go:12-24`) truncates via `MaxWidth`, does not multi-line wrap. So narrow terminals cannot inflate `gridRows` via per-row wrap.

- **E9** — Coverage gap. `model_test.go` has `TestWindowSizeMsg_VerySmall_ViewportClampsToOne`, `TestWindowSizeMsg_Width3_VpWidthClampsToOne`, `TestView_ZeroDimensions_NoPanic` — all assert non-panic and `vpHeight ≥ 1`. None assert `len(strings.Split(View(), "\n")) <= m.height`. The very class of bug — frame taller than terminal — has no test guard.

## Root Cause Analysis (revised after validation)

There are **two distinct bugs**, both triggered by terminal resize:

**Bug A (primary, explains the user's screenshot at 172×39).** Bubble Tea's `standard_renderer` on `WindowSizeMsg` calls `repaint()` — which only invalidates its in-memory line cache — but does NOT emit an alt-screen erase or reset the cursor home (validator V4, confirmed by reading `bubbletea@v1.3.10/standard_renderer.go:213-264, 630-635`). When a previous render had overflowed the terminal (e.g. during a transient mid-zoom size), the alt-screen scrolled and the renderer's cursor-position assumption diverged from the terminal's actual cell layout. After the user finished zooming, the new render writes lines starting from the stale cursor, producing the interleaved/shuffled rows the user observed (row2, row3, row1, row0 missing — V2).

**Bug B (latent, would trigger on genuinely small terminals).** Run-mode `View()` always emits `gridRows + vpHeight + 5` lines with `vpHeight` clamped to ≥ 1 (E2, E3). When the terminal is shorter than the chrome's intrinsic minimum, the frame overflows. Unlike the workflow-builder TUI (E1) and the help-modal overflow path (E6), there is no min-size guard. This is what *produces* the over-tall render in the first place (the input to Bug A's stale-cursor problem) AND would also break the layout independently on truly small terminals.

Phase transitions are coincidental (E4): the grid is pre-allocated to `maxStepsAcrossPhases` at startup, so transitioning from initialize to iteration only populates previously-empty cells. The user noticed the broken layout at the phase transition because that's when populated cells made the breakage visually obvious — the underlying corrupt render state had been there since the zoom event.

## Coding Standards Reference

- **`docs/coding-standards/tui-rendering.md`** — Plain-text-first ANSI composition (single styling pass), `lipgloss.Width()` for visual width, render file decomposition (`render_*.go` per component). The fix will follow these (no styling-pass changes; the new branch is a single `if` + early return).
- **Inferred from `internal/workflowedit/render_frame.go:30`** — When the terminal is below the min-size threshold, return a single-string placeholder "Terminal too small — resize to at least WxH". The run-mode TUI should mirror this pattern for consistency.
- **`docs/coding-standards/testing.md`** — Race detector required (`make test` already uses `-race`); add tests that assert frame line count against `m.height` to close coverage gap E9.

## Planned Fix (revised)

**One-sentence summary.** Force a full alt-screen clear on every `WindowSizeMsg` to recover from any stale renderer state (fixes Bug A, the user's actual symptom), AND add a min-size guard to `View()` so the renderer never produces an over-tall frame in the first place (fixes Bug B, the precondition for Bug A), AND add tests covering both the line-count invariant and a resize-down-then-up scenario.

### File 1: `src/internal/ui/model.go` — `WindowSizeMsg` handler

**Change.** Return a `tea.ClearScreen` cmd on every `WindowSizeMsg` so the alt-screen is fully erased and the renderer's cursor-position assumption is reset before the next frame writes:

```go
case tea.WindowSizeMsg:
    m.width = msg.Width
    m.height = msg.Height
    gridRows := len(m.header.header.Rows)
    chromeRows := 1 + gridRows + 1 + 1 + 1 + 1
    vpHeight := m.height - chromeRows
    if vpHeight < 1 {
        vpHeight = 1
    }
    vpWidth := m.width - 2
    if vpWidth < 1 {
        vpWidth = 1
    }
    m.log.SetSize(vpWidth, vpHeight)
    var lcmd tea.Cmd
    m.log, lcmd = m.log.Update(msg)
    cmds = append(cmds, lcmd, tea.ClearScreen) // ← NEW: erase any stale alt-screen content
```

- Justified by **V4** (Bubble Tea's `repaint()` does not erase the alt-screen, leaving stale rows from the previous over-tall render at the top of the screen).
- This is the actual fix for the user's reported symptom at 172×39 (V1, V2).

### File 2: `src/internal/ui/model.go` — `View()` min-size guard

**Change.** After computing `m.width` / `m.height`, add an early-return branch:

```go
func (m Model) View() string {
    gridRows := len(m.header.header.Rows)
    chromeRows := 1 + gridRows + 1 + 1 + 1 + 1
    minHeight := chromeRows + 1 // chrome + at least one viewport row
    if m.width < uichrome.MinTerminalWidth || m.height < minHeight {
        return fmt.Sprintf("Terminal too small — resize to at least %d×%d",
            uichrome.MinTerminalWidth, minHeight)
    }
    // ...rest of existing View() body unchanged...
}
```

- Justified by **E1, E2, E3, E6**.
- Mirrors `internal/workflowedit/render_frame.go:30`.
- `minHeight` is computed dynamically from `gridRows` (constant per-run, V6) rather than using the static `uichrome.MinTerminalHeight=16`, because workflows with many steps need a taller minimum.
- Side benefit: the early return short-circuits the help-modal block too, so help-modal-on-narrow-terminal corner cases are also fixed (V8).

### File 3: `src/internal/ui/model.go` — mouse handler precondition

**Change.** Mouse-event handling at `model.go:237-321` assumes a fitted frame for coordinate translation. Add the same min-size precondition so mouse events are ignored while the placeholder is showing:

```go
case tea.MouseMsg:
    gridRows := len(m.header.header.Rows)
    minH := 1 + gridRows + 5 + 1
    if m.width < uichrome.MinTerminalWidth || m.height < minH {
        return m, nil // ignore mouse events while in too-small placeholder mode
    }
    // ...existing body...
```

- Justified by **V9**.

### File 4: `src/internal/ui/model_test.go`

**Change.** Add tests:

1. `TestView_FrameNeverExceedsHeight` — table-driven over (width, height, stepCounts). Assert `len(strings.Split(m.View(), "\n")) <= m.height` for every combination.
2. `TestView_TooSmall_RendersPlaceholder` — given `height < chromeRows + 1`, assert View output is the placeholder string and contains no border glyphs (`╭`, `╮`, `╰`, `╯`).
3. `TestView_AfterResizeSmallThenLarge_RendersFullFrame` — send a small `WindowSizeMsg`, then a normal one; assert second `View()` emits the full chrome and that the WindowSizeMsg path returns a `tea.ClearScreen` cmd in its batch (use a recorder fake to capture cmds).
4. `TestWindowSizeMsg_ReturnsClearScreenCmd` — assert every `WindowSizeMsg` Update returns a batch containing `tea.ClearScreen`. This is the regression guard for Bug A.

- Justified by **E9, V7**.
- Note (V7): Unit tests on `View()` cannot detect the renderer-level stale-cursor artifact. Test 4 covers the *cmd contract* — that the model issues the clear — which is the strongest model-level guarantee available without spinning up a full Bubble Tea program.

### Manual reproduction protocol

Document in plan only (not committed code). To verify Bug A is fixed:

1. Start pr9k against any repo with a multi-step workflow.
2. While the TUI is running, zoom the terminal in (Cmd+= on macOS Terminal/iTerm) several times rapidly to force the frame to overflow transiently.
3. Zoom back out to a comfortable size.
4. Trigger a re-render (e.g., wait for the next phase transition or step state change).
5. Verify the top border is intact and grid rows are in the expected order.

### What this fix does NOT change

- `vpHeight < 1 → 1` clamp stays — defensive against `bubbles/viewport` receiving zero dimensions; the new guard ensures `View()` never renders a frame in that regime anyway.
- `HeaderCols = 4` stays. Adapting columns to width is a separate concern.
- The "frame shrinks gracefully" / progressive-degradation alternative was considered and rejected: it doubles the render code paths, and the workflow-builder TUI already established the placeholder pattern. Consistency wins.

## Adjustments Made (after adversarial validation)

- **V1, V2 (root cause was wrong):** Original plan attributed the symptom solely to small-terminal overflow. Validator showed 172×39 is far above chrome height and that row-shuffling cannot be produced by simple top-clipping. Root Cause section now distinguishes Bug A (renderer stale-cursor after resize) and Bug B (frame overflow on small terminals).
- **V4 (fix was incomplete):** Added `tea.ClearScreen` cmd to the `WindowSizeMsg` handler. This is now the primary fix for the user's reported symptom; the min-size guard is the secondary, preventive fix.
- **V7 (test gap):** Added explicit regression test that asserts `tea.ClearScreen` is included in the cmd batch from `WindowSizeMsg`, plus a manual reproduction protocol for the renderer-level artifact a unit test cannot catch.
- **V8 (side benefit not noted):** Documented that the new guard short-circuits help-modal narrow-terminal cases.
- **V9 (mouse handler):** Added File 3 — mouse handler short-circuit so coordinate translation doesn't operate against the placeholder.
- **V6 (minor phrasing):** Acknowledged that `minHeight` is computed once per run (since `gridRows` is constant per E4), not per-WindowSizeMsg.

## Confidence Assessment

- **HIGH** that the combined fix (`tea.ClearScreen` + min-size guard) addresses the user's actual reported symptom. The `tea.ClearScreen` directly fixes the V4 stale-cursor pathway that produced the shuffled-row screenshot at 172×39.
- **MEDIUM** that no other resize-related artifacts remain. `bubbles/viewport`'s own resize behavior (R5 below) was not exhaustively audited — `m.log.SetSize` followed by `m.log.Update(msg)` is the project's existing pattern and is preserved unchanged.

## Remaining Risks

1. **`bubbles/viewport` stale lines (V's R5).** The viewport may itself cache wrapped lines that don't invalidate cleanly on `SetSize`. If so, content inside the log panel could still look stale across resizes even after the screen clear. Mitigation: rely on existing word-wrap-on-resize code path in `log_panel.go`; flag as a separate investigation if it surfaces.
2. **OS window title length (V's R4).** `titleString()` includes `heartbeatSuffix` ("⋯ thinking (Ns)") which could grow long. Not a render bug but worth noting.
3. **Manual reproduction is required.** Test 4 (cmd-contract regression) is necessary but not sufficient; only the manual zoom-then-zoom-back protocol can fully verify Bug A is fixed. CI cannot reproduce real terminal alt-screen behavior.
4. **`tea.ClearScreen` causes a brief flash** on every resize. This is the cost of correctness; users only see it when they resize, which is rare.

## Final Summary

- **Root cause:** Bubble Tea's `standard_renderer` does not erase the alt-screen on `WindowSizeMsg` — it only invalidates its line cache — so any over-tall render that scrolled the alt-screen during a transient mid-zoom size leaves stale rows above the new render's cursor home, producing the shuffled-row layout the user observed at 172×39 (Bug A); separately, the run-mode `View()` has no min-size guard so it can produce over-tall renders in the first place (Bug B).
- **Fix:** Return `tea.ClearScreen` from the `WindowSizeMsg` handler so every resize fully erases the alt-screen, AND add a min-size guard to `View()` that returns a "Terminal too small" placeholder when the terminal is below `MinTerminalWidth × (chromeRows+1)`, mirroring the workflow-builder TUI's pattern, AND short-circuit mouse handling under that same precondition.
- **Why correct:** V4 confirmed via reading vendored `bubbletea/standard_renderer.go:213-264, 630-635` that `repaint()` does not emit any alt-screen erase; V2 confirmed the row shuffling is consistent with stale-cursor + new-render interleaving and inconsistent with simple top-clipping; the `internal/workflowedit` TUI already uses this min-size pattern (E1).
- **Validation outcome:** Original min-size-guard-only plan was refuted by V1/V2/V4 as a phantom fix for the screenshot at 172×39; plan now addresses both the renderer artifact (primary) and the underlying overflow precondition (secondary).
- **Remaining risks:** `bubbles/viewport` may have its own stale-line cache that this fix doesn't address; `tea.ClearScreen` causes a brief flash on resize; manual repro protocol is required because CI cannot reproduce real alt-screen behavior.
