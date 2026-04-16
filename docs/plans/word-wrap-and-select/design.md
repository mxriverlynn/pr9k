# Plan: Word-wrap log content + mouse/keyboard text selection and copy

## Context

The ralph-tui log panel has two ergonomic gaps:

1. **Long lines don't wrap.** Streaming subprocess output longer than the viewport width is silently clipped at the outer frame by lipgloss's `MaxWidth`. Content past the right edge is hidden.
2. **No way to select/copy text.** The program runs under `tea.WithAltScreen()` + `tea.WithMouseCellMotion()`, which together defeat native terminal text selection. The user can't select a log line to paste elsewhere — a routine need when debugging a Ralph run.

Goal: the user can highlight any region of the log panel with **mouse drag** or **keyboard** and copy it to the system clipboard. Long lines wrap at word boundaries to the viewport width so nothing is hidden off-screen.

## Approach

Two independent but related changes, both landing in `ralph-tui/internal/ui/`.

1. **Word-wrap on render.** Ring buffer continues to store raw (unwrapped) lines. On every `SetContent`, wrap each raw line to `viewport.Width` using `github.com/charmbracelet/x/ansi.Wrap` (which does word-wrap with a hard-wrap fallback for over-long tokens in a single call). Maintain a map `visualLine -> (rawIdx, rawOffset)` so the selection layer can recover the original text for the clipboard.

2. **Custom in-TUI selection + copy.** Add `ModeSelect` to the keyboard state machine. Intercept `tea.MouseMsg` left-button events in the root model (routing wheel events to the viewport for scrolling as today). Maintain selection anchor/cursor in visual-line space. Render selected cells with reverse-video style. Copy via `github.com/atotto/clipboard` (macOS/Linux/Windows portable).

### Dependencies

- **Wrapping:** reuse `github.com/charmbracelet/x/ansi` (already an indirect dep of ralph-tui via bubbletea/lipgloss — promote to a direct dep). `ansi.Wrap(s, limit, breakpoints)` does word-wrap with hard-wrap fallback for over-long tokens, is ANSI-aware, and grapheme-aware. No new external dependency is needed. (`ansi.Wordwrap` + `ansi.Hardwrap` are available separately if we ever want finer control.) **Rejected alternative:** `github.com/muesli/reflow` — duplicates `x/ansi.Wrap` functionality while adding a new dep.
- **Clipboard:** add `github.com/atotto/clipboard` — portable clipboard (macOS pbcopy / Linux xclip+xsel / Windows clipboard API).

## User-visible behavior

### Wrapping

- Lines longer than the inner viewport width wrap at the nearest preceding space. A single token with no spaces (long URL, stream-json blob) hard-wraps at the width boundary. **No hanging indent** — wrapped lines start at column 0.
- On terminal resize, content re-wraps to the new width.
- Scroll position is preserved: if the user was at bottom, re-wrap still ends at bottom; otherwise the top of the visible window stays anchored to the same raw-line.

### Selection — mouse

- From `ModeNormal` or `ModeDone`, **left-button press** on the viewport area starts a selection (anchor = cell under cursor) and transitions to `ModeSelect` (saving `prevMode`). Presses in `ModeError`, `ModeQuitConfirm`, `ModeNextConfirm`, and `ModeQuitting` are ignored (same rationale as `v`).
- If a committed selection from a previous drag is already visible, the **next left-press does not reset the anchor**; it is ignored until the user either (a) starts motion (drag) — which begins a new selection — or (b) presses `Esc`/`y`. This prevents a stray focus-click from destroying a committed selection before it can be copied. Entering drag from a fresh press (no prior committed selection) keeps the familiar press-to-anchor behavior.
- **Drag** (button-left motion) extends the selection's cursor endpoint. Dragging **above** the viewport top or **below** the viewport bottom auto-scrolls the viewport by one line per motion event in that direction, so selections can extend past the visible window.
- **Release**: selection becomes "final" (non-active, still visible). Shortcut bar shows `y copy  esc cancel`.
- Mouse **wheel** continues to scroll regardless of mode (wheel events are still forwarded to the viewport, which already ignores non-wheel `MouseMsg` events).
- A **click without drag** (press+release at same cell with no intervening motion) **when no committed selection is visible** clears any in-flight selection state and returns to `prevMode`. When a committed selection is visible, the click is ignored (see the press-protection rule above) so the user can copy before a stray click eats the selection.
- Auto-scroll-to-bottom is **suppressed while a selection is `visible()`** (active drag in progress, or a committed non-empty range is displayed). Clearing the selection re-arms auto-scroll.

### Selection — keyboard

- `v` enters `ModeSelect` from `ModeNormal` and `ModeDone`. Excluded from `ModeError`, `ModeQuitConfirm`, `ModeNextConfirm`, and `ModeQuitting`: in Error mode the orchestration goroutine is blocked waiting on `KeyHandler.Actions` (`keys.go:76-78`), and entering Select would leave that goroutine parked until the user remembers to exit Select. The three prompt/terminal modes have confirmations in flight. Mouse-left-press follows the same rule — left-press while in `ModeError` does nothing; the user must press `c`/`r`/`q` first. Initial anchor and cursor = top-left cell of the currently visible viewport window (`YOffset, 0`). The cursor cell renders with reverse video so the user has an immediate visual indicator of select mode even before moving the cursor.
- In `ModeSelect`:
  - `h`/`←`, `j`/`↓`, `k`/`↑`, `l`/`→` move the cursor, extending selection from the fixed anchor. Viewport auto-scrolls to keep the cursor visible.
  - The cursor column is **virtual** (vim-style): when moving to a shorter line, the cursor renders clamped at that line's last column, but the virtual column is remembered so moving further down to a longer line restores it.
  - `y` or `Enter` → copy selected text to clipboard, clear selection, return to `prevMode`.
  - `Esc` → clear selection, return to `prevMode`.
  - `q` → clear selection, transition to `ModeQuitConfirm`. `prevMode` for QuitConfirm is set to the mode we entered `ModeSelect` from (i.e., `ModeNormal` or `ModeDone`), **not** `ModeSelect`, so Escape from QuitConfirm restores the real idle mode rather than a degenerate empty-selection state.

### Copy format

Selected text is reconstructed from the **raw** lines via the wrap map, so wrapping doesn't inject artificial newlines into the clipboard. One original line → one clipboard line, regardless of how many visual rows it occupied.

Raw lines in the ring buffer are plain text: `claudestream.Renderer` emits unstyled strings, and `logContentStyle` is a single ANSI envelope applied at `SetContent` time (not stored in `lines`). So the clipboard receives clean UTF-8 without embedded ANSI escape codes. No stripping step required.

### Streaming + selection: ring buffer eviction

`logRingBufferCap = 2000`. When lines stream past the cap, `logModel.Update` re-slices `m.lines = m.lines[len(m.lines)-logRingBufferCap:]`, which shifts raw-line indices. If an active selection holds `rawIdx=N` captured before an eviction of `k` lines, its intended raw line is now at `N-k`.

Handling: on every eviction in `logModel.Update(LogLinesMsg)`, decrement both `anchor.rawIdx` and `cursor.rawIdx` by the number of lines dropped. If the resulting `rawIdx < 0`, the anchor or cursor now references content that has been dropped from the buffer — clear the selection entirely (it is no longer recoverable). This keeps the invariant that `(rawIdx, rawOffset)` always points into the current `lines` slice.

The selection state thus holds `rawIdx`/`rawOffset` **plus** a derived `visualRow`/`col` that is recomputed from `visualLines` on every rewrap. The visual coordinates are display-only; authoritative coordinates are raw.

## Files to modify

### `ralph-tui/internal/ui/log_panel.go` (modify)

- Keep `lines []string` (raw). Add `visualLines []visualLine` rebuilt on every `SetContent`/resize:
  ```go
  type visualLine struct {
      text      string // wrapped segment with ANSI styling applied
      raw       string // plaintext segment used for copy
      rawIdx    int    // index into m.lines
      rawOffset int    // byte offset within lines[rawIdx] where this segment starts
  }
  ```
- New helper `rewrap(width int)` — rebuilds `visualLines` by calling `ansi.Wrap(line, width, " -")` per raw line (single pass; `ansi.Wrap` already does word-wrap with a hard-break fallback for over-long tokens). Splits the result on `\n` to build visualLines.
- New helper `renderContent(sel *selection)` — joins `visualLines` into a single string and wraps cells inside the selection range with `lipgloss.NewStyle().Reverse(true)`. Called from `Update` on `LogLinesMsg`, from `SetSize`, and whenever the selection changes.
- `Update` changes:
  - `LogLinesMsg`: append raw lines → `rewrap(viewport.Width)` → `renderContent(sel)` → `SetContent` → `GotoBottom` **only if** `wasAtBottom && !selection.visible()`.
  - New internal message `selectionChangedMsg` triggers a re-render without re-wrapping.
- `SetSize`: if width changed, snapshot `(rawIdx, rawOffset)` at `visualLines[viewport.YOffset]` **before** `rewrap` (skip entirely if `len(visualLines) == 0` or `YOffset >= len(visualLines)`), then call `rewrap(width)` → re-`SetContent`. After the new content is set, scan `visualLines` for the largest `i` such that `visualLines[i].rawIdx == snapshot.rawIdx && visualLines[i].rawOffset <= snapshot.rawOffset` and `SetYOffset(i)`. Snapshotting `rawOffset` (not just `rawIdx`) matters: if the user was viewing segment 2 of a 3-segment wrapped raw line before resize, jumping to segment 1 of the same raw line after rewrap would yank the view upward unexpectedly. Height-only changes skip the rewrap step.
- Accessors: `SelectedText() string` and `SetSelection(sel selection)`.

### `ralph-tui/internal/ui/selection.go` (new)

- `type selection struct { anchor, cursor pos; active, committed bool }`.
- `type pos struct { rawIdx, rawOffset int; visualRow, col int }` — `rawIdx`/`rawOffset` are the authoritative coordinates (stable across rewrap and ring-buffer eviction decrement); `visualRow`/`col` are derived render-time coordinates in display cells, rune-aware via `github.com/rivo/uniseg` (already transitive) or the already-transitive `x/ansi` grapheme helpers. `col` is virtual (vim-style) — movement preserves the intended column across short lines.
- `func (s selection) visible() bool` — true when a selection is active (mid-drag) or committed with a non-empty range. Used to gate auto-scroll.
- `func (s selection) normalized() (start, end pos)` — orders anchor/cursor in reading order by `(rawIdx, rawOffset)`.
- `func (s selection) contains(row, col int) bool` — given a visual row/col, returns whether it falls in the selected range.
- `func extractText(lines []string, start, end pos) string` — uses raw coords (not visual) to slice the raw lines: for `start.rawIdx == end.rawIdx`, returns `lines[rawIdx][start.rawOffset:end.rawOffset]`; otherwise joins `lines[start.rawIdx][start.rawOffset:]`, full middle lines, `lines[end.rawIdx][:end.rawOffset]` with `\n`. This avoids dedup work over visualLines and always yields the original newline structure.
- `func MouseToViewport(msg tea.MouseMsg, topRow, leftCol int, vp viewport.Model) (pos, bool)` — returns `(row, col)` in viewport-content space (so `visualRow = vp.YOffset + (msg.Y - topRow)`). If `msg.Y` is above topRow or below `topRow + vp.Height - 1`, returns ok=false for chrome-hit detection, **but** when called during an active drag, the caller still consumes the event and scrolls the viewport instead of dropping it.
- `func visualColToRawOffset(rawLine string, segmentStart int, col int) int` — walks forward from `segmentStart` (the byte offset in `rawLine` where this visual segment begins) counting display cells with `uniseg.GraphemeClusterCount`/`ansi.StringWidth` until `col` cells have been consumed, returning the byte offset. If `col` exceeds the segment's cell width, returns the segment's end offset. This is the single linchpin for converting `(visualRow, col)` → `rawOffset`; it is called by `MouseToViewport` and `MoveSelectionCursor` to compute the authoritative raw coordinates stored on `selection.anchor`/`cursor`.

**Tabs:** raw lines containing `\t` are normalized on ingest (`logModel.Update(LogLinesMsg)`) to four spaces before the append — `strings.ReplaceAll(line, "\t", "    ")`. This keeps `visualColToRawOffset` a simple cell walk; otherwise a single `\t` byte maps to 1–8 cells depending on column position, which is surprisingly painful to unwind during selection. Tabs are rare in stream-json output and never semantically significant in our logs.

### `ralph-tui/internal/ui/model.go` (modify)

- Store the viewport's screen-space top-left (`logTopRow`, `logLeftCol`) computed in the existing `tea.WindowSizeMsg` handler (`logTopRow = 1 + gridRows + 1`, `logLeftCol = 1`).
- Rework `tea.MouseMsg` handler:
  - Wheel events → forward to `m.log.Update(msg)` (existing behavior).
  - Left press / motion / release → translate with `MouseToViewport`; if inside the viewport, delegate to `logModel.HandleMouse(pos pos, action tea.MouseAction) (logModel, tea.Cmd)` which returns the updated `logModel` (selection lives on the `logModel` value — every mutation returns a new logModel, which the parent `Model.Update` re-assigns to `m.log`) and a `tea.Cmd` emitting `selectionChangedMsg`. Also notify `keyHandler` so the shortcut bar updates on `ModeSelect` enter/exit.

### `ralph-tui/internal/ui/keys.go` (modify)

- Add `handleSelect(key tea.KeyMsg)`:
  - `h j k l ← → ↑ ↓` → `m.log.MoveSelectionCursor(dx, dy)`.
  - `y` / `enter` → `clipboard.CopyToClipboard(m.log.SelectedText())`. On error, set transient footer (see clipboard.go). On success, clear selection and return to `prevMode`.
  - `esc` → clear selection, return to `prevMode`.
  - `q` → clear selection; set `prevMode = <the mode we entered ModeSelect from>` (not `ModeSelect`); transition to `ModeQuitConfirm`.
- Extend the root `Update` switch to dispatch `ModeSelect` → `handleSelect`.
- Add `v` key handling in `handleNormal` and `handleDone` only: save `prevMode`, enter `ModeSelect`, initialize selection at viewport top-left with the cursor cell marked for reverse-video rendering.

### `ralph-tui/internal/ui/ui.go` (modify)

- Add `ModeSelect` to the `Mode` enum. Placement: immediately before `ModeQuitting`, since `ModeQuitting` is conceptually terminal. (No code serializes `Mode` ordinals — verified via grep — so enum ordering is purely stylistic.)
- Add `SelectShortcuts = "↑↓←→/hjkl extend  y copy  esc cancel  q quit"`.
- Update `NormalShortcuts` and `DoneShortcuts` to include `v select` (Normal: after the scroll hints, before `n next step`; Done: before `q quit`). Do **not** add it to `ErrorShortcuts` — `v` is blocked in ModeError.
- `updateShortcutLineLocked`: map `ModeSelect → SelectShortcuts`.

### `ralph-tui/internal/ui/clipboard.go` (new)

- Thin wrapper: `func CopyToClipboard(text string) error { return copyFn(text) }` with `var copyFn = clipboard.WriteAll` so tests can swap in a fake.
- When `CopyToClipboard` fails (no `xclip`/`xsel` on Linux, OS permission denial, etc.), `handleSelect` returns a `tea.Cmd` whose closure emits a synthetic `LogLinesMsg{Lines: []string{"[copy failed: install xclip or xsel]"}}`. This routes through the existing `Update` path exactly like a streamed line — no cross-goroutine concerns, no new plumbing into `runner.SetSender`, no transient footer state. Mode still transitions back to `prevMode` in the same `handleSelect` pass.

### `ralph-tui/cmd/ralph-tui/main.go` (no structural change)

- `tea.WithMouseCellMotion()` stays — press+drag is exactly what we need; `tea.WithMouseAllMotion()` is not required.
- `tea.WithAltScreen()` stays — custom selection makes the alt-screen selection issue moot and removing it would break the fullscreen TUI look.

### `ralph-tui/go.mod` / `go.sum`

- Promote `github.com/charmbracelet/x/ansi` from indirect to direct dep (already transitively required via bubbletea/lipgloss).
- Add `github.com/atotto/clipboard v0.1.4` (or current version).

## Docs to update

- `docs/features/tui-display.md` — add a section on viewport wrapping and the selection-highlight overlay.
- `docs/features/keyboard-input.md` — update the "six-mode" framing to seven-mode; document `ModeSelect` entry/exit and key bindings.
- `docs/how-to/reading-the-tui.md` — mention selection/copy as an available interaction.
- New `docs/how-to/copying-log-text.md` — mouse and keyboard walkthroughs.

### ADR + version

- New ADR `docs/adr/YYYYMMDD-clipboard-and-selection.md` documenting (a) chose `atotto/clipboard` over OSC 52, (b) Linux requires `xclip` or `xsel` at runtime, (c) custom in-TUI selection required because `tea.WithAltScreen() + tea.WithMouseCellMotion()` defeat native terminal selection, (d) OSC 52 deferred to a follow-up. This pairs with `20260413160000-require-docker-sandbox.md` in the pattern of documenting external runtime dependencies.
- Per `docs/coding-standards/versioning.md`, the new `v` key binding and `ModeSelect` extend ralph-tui's public keyboard surface — bump the minor version (`internal/version/version.go`) as part of the same branch.

## Existing code to reuse

- `logModel.SetSize` (`internal/ui/log_panel.go:108`) — extend, don't duplicate.
- `wrapLine` in `model.go:172` — untouched; it handles outer-border truncation of the already-composed viewport block, orthogonal to inner content wrapping.
- `viewport.AtBottom()` / `GotoBottom()` — existing scroll control; gate `GotoBottom` on `!selection.visible()`.
- `KeyHandler.SetMode` (`internal/ui/ui.go:101`) and the `prevMode` save/restore pattern — reused verbatim for `ModeSelect` entry/exit.
- `logContentStyle` (`log_panel.go:21`) — still applied to non-selected segments; selection layers `.Reverse(true)` on top.

## Verification

### Unit tests (new, in `internal/ui/*_test.go`)

- `TestWrap_WordBoundary` — "the quick brown fox" at width 10 → expected word-boundary segments.
- `TestWrap_LongToken` — a 40-char token at width 10 hard-wraps into 4 segments.
- `TestWrap_Rewrap_OnResize` — wrap at 80, then 40, verify `visualLines` recount.
- `TestWrap_EmptyLine` — empty raw line remains an empty segment (chrome spacing relies on this).
- `TestWrap_ExactWidthNoSpaces` — a raw line exactly `width` chars with no spaces produces one segment, not an empty trailing segment.
- `TestWrap_WidthZero_NoOp` — `ansi.Wrap(s, 0, " -")` returns `s`; `visualLines` has one segment matching the raw line (pre-first-WindowSizeMsg path).
- `TestWrap_TabNormalization` — raw line with `\t` is stored with 4-space expansion; copied text contains the expanded spaces.
- `TestSelection_Normalize` — anchor below cursor gets swapped.
- `TestSelection_ExtractText_SingleRawLine_MultipleVisual` — selection across two wrapped segments of one raw line yields the raw text, no intra-line `\n`.
- `TestSelection_ExtractText_AcrossRawLines` — selection spanning two raw lines yields text with exactly one `\n` between them.
- `TestMouseToViewport_OutsideArea` — clicks on chrome return ok=false.
- `TestKeys_V_EntersSelectMode_FromNormal`.
- `TestKeys_Y_Copies_Exits` — uses fake `copyFn` to capture the payload.
- `TestLogModel_AutoScroll_SuppressedDuringSelection`.
- `TestLogModel_RingEviction_DecrementsSelectionRawIdx` — append N+k lines (exceeding cap) with an active selection, assert `selection.anchor.rawIdx` decrements by `k` and selection stays on same raw content.
- `TestLogModel_RingEviction_ClearsSelectionWhenRawIdxUnderflow` — selection anchored at a raw line that gets fully evicted → selection clears.
- `TestResize_PreservesTopRawLine` — scroll to a specific raw line, resize narrower, assert the top visible row after rewrap still points to the same raw line.
- `TestResize_PreservesTopRawOffset` — scroll so the top visible row is segment 2 of a 3-segment wrapped raw line; resize narrower; assert the new top visible row's `rawOffset` is ≤ the snapshot's `rawOffset` (user does not get yanked backward to segment 1 of the same raw line).
- `TestKeys_V_EntersSelectMode_FromNormal_FromDone` — parameterized across the two idle modes where `v` is allowed.
- `TestKeys_V_IgnoredIn_Error_QuitConfirm_NextConfirm_Quitting` — non-transition assertion (includes Error since the orchestrator is blocked on `Actions`).
- `TestMouse_LeftPress_IgnoredIn_Error` — left-press in ModeError does nothing.
- `TestMouse_LeftPress_DoesNotResetCommittedSelection` — with a committed non-empty selection, a bare press (no motion) is ignored; only a press+motion starts a new selection.
- `TestKeys_Q_FromSelect_PreservesIdleModeAsPrevMode` — q from `ModeSelect` (entered from `ModeError`), esc from QuitConfirm restores `ModeError`, not `ModeSelect`.
- `TestCopy_EmptySelection_NoOp` — pressing y with an empty committed selection returns to prevMode without invoking copyFn.
- `TestCopy_ClipboardError_AppendsLogLine` — fake copyFn returns error, assert the informational log line is appended to the ring buffer and the mode still transitions back to prevMode.
- `TestExtractText_PartialRawLine_SingleLine` — selection inside one wrapped raw line but not including its start or end → substring only.

### Manual verification

Run `make build && ./bin/ralph-tui -n 1` against a scratch repo with a Ralph-labeled issue.

1. **Wrapping:** Resize the terminal narrow during a streaming step. Long lines wrap at word boundaries. Resize wider — wraps disappear, no content lost.
2. **Mouse drag + copy:** Click-drag across several visible lines → reverse-video highlight. Release. Press `y`. Paste elsewhere — clipboard has raw text with original line breaks only (no wrap-induced newlines).
3. **Mouse cancel:** Drag again, press `Esc`. Selection clears, shortcut bar restores.
4. **Keyboard select:** Press `v` in Normal mode. Cursor appears at top-left of visible viewport. Use `j` + `l` to extend. Press `Enter`. Paste and verify.
5. **Streaming while selected:** Start a long step, begin a selection, let more output arrive. Selection stays put; viewport does not auto-scroll. Clear selection — auto-scroll resumes.
6. **Resize mid-select:** Drag-select, then resize the terminal. Anchor is preserved by raw-line idx; visual highlight shifts but copied text is still correct.
7. **Wheel scroll:** Mouse-wheel in any mode still scrolls the viewport.

### Test suites

- `cd ralph-tui && go test -race ./...` — must pass.
- `make lint && make vet` — no new warnings, no `//nolint` (per `docs/coding-standards/lint-and-tooling.md`).
- `make ci` — full pipeline green.

## Edge cases and recovery

- **Empty viewport + v.** If `len(lines) == 0` when `v` is pressed, do nothing (don't transition to `ModeSelect`) so the user doesn't get stuck in an empty select state with no visual cursor.
- **Mid-drag resize.** If a `tea.WindowSizeMsg` arrives while `selection.active` (drag in progress), force-commit the selection (`active = false; committed = true`) before rewrap. The user's raw anchor/cursor are preserved by raw coordinates; continuing the drag after resize would require the terminal to emit further motion events, which requires the button to still be held — which is an unobservable invariant. Committing is the safe choice.
- **Copy when selection is empty.** `y`/`Enter` with an empty selection (e.g., committed but `anchor == cursor`) is a no-op that still returns to `prevMode` — no clipboard write, no transient footer. Keeps the mode exit predictable.
- **Terminal without mouse reporting.** If the user's terminal strips mouse bytes or `WithMouseCellMotion` has no effect, the keyboard (`v` + `hjkl`) path still works. No additional handling required.
- **Very wide lines after rewrap.** `x/ansi.Wrap` with a limit of `viewport.Width` will hard-break any single token longer than the width. No risk of infinite recursion or O(n²) blowup; worst case is `raw_len / width` visual rows per raw line.
- **Pre-first-WindowSizeMsg `LogLinesMsg`.** `newLogModel(0, 0)` is constructed with Width=0 (`model.go:64`). If a line arrives before the first `WindowSizeMsg`, `ansi.Wrap(s, 0, " -")` short-circuits and returns the input unchanged (`wrap.go:293-296`) — the `visualLines` builder must treat that as "one segment per raw line" without panicking. First `WindowSizeMsg` triggers a full rewrap via `SetSize`.
- **Rewrap cost under burst load.** The drain goroutine in `main.go:194-216` coalesces lines into one `LogLinesMsg`. Rewrapping the full `logRingBufferCap=2000` buffer on every batch is ~2000 `ansi.Wrap` calls per message. **Budget:** full-buffer rewrap at 200-col width must complete in <50 ms on the developer's machine (benchmark in `log_panel_bench_test.go`). **Optimization path if the budget is missed:** switch to append-only incremental wrap — only the newly-appended lines feed into `ansi.Wrap`; on eviction, drop the corresponding prefix of `visualLines`; full rewrap is reserved for width changes in `SetSize`. This optimization is optional — ship the simple full-rebuild first, benchmark, and only switch if needed.

## Review summary

- **Iterations completed:** 3 passes + full agent validation (evidence-based-investigator + adversarial-validator).
- **Assumptions challenged (12 total):** ordinal stability of `Mode` (refuted — not serialized), need for `muesli/reflow` (refuted — `x/ansi.Wrap` already transitive with matching API), raw-line ANSI cleanliness (verified), ring-buffer eviction rawIdx stability (refuted gap — now handled with decrement + underflow-clears), YOffset semantics & SetContent auto-bottom (verified), MouseMsg 0-indexing (verified), `v`/`h`/`l` key collisions (verified absent), clipboard-error surfacing via sender path (refuted — unreachable from Update goroutine; switched to synthetic `LogLinesMsg` via `tea.Cmd`), resize preserves top raw line (refuted — needed `rawOffset` too, not just `rawIdx`), `v` from ModeError safe (refuted — orchestrator blocks on `Actions`), HandleMouse pointer/value semantics (was underspecified — pinned to value return), visual-col → raw-byte translation (was implicit — extracted as `visualColToRawOffset` helper with tab-normalization on ingest).
- **Consolidations:** dropped `muesli/reflow` in favor of already-transitive `x/ansi.Wrap`; replaced transient-footer KeyHandler field with a synthetic `LogLinesMsg`.
- **Ambiguities resolved:** cursor visibility on keyboard entry (reverse-video initial cell), prevMode propagation from Select→QuitConfirm (uses the pre-Select idle mode), committed selection protection against stray click (bare click ignored), empty-viewport + `v` (no-op), mid-drag resize (force-commit before rewrap), clipboard failure surface (log line, not footer), ADR + semver bump (added).
- **Validation adjustments:** 6 real gaps and 4 underspecified items from the adversarial pass were incorporated; 2 items (narrow-resize CPU spike, `n` from Select) judged non-blocking and documented.
- **Stopped at 3 iterations:** structural changes after iteration 2 were concentrated in a single simplification (transient footer → log line). Further iteration passes would produce cosmetic wording changes only. Agent validation produced the largest downstream-edit set, which was applied as a final pass.

## Out of scope (deferred)

- OSC 52 clipboard for remote SSH sessions with no local clipboard daemon. `atotto/clipboard` on Linux relies on `xclip`/`xsel`; remote-over-SSH copy would need OSC 52. Capture as a follow-up if reported.
- Search-in-log (`/` to find). Natural companion feature, separate task.
- Select-all shortcut (`ctrl+a`). Trivial once selection plumbing exists; defer until requested.
- Line-granularity selection (shift+↑/↓). Character-level from mouse + keyboard covers the asks.
