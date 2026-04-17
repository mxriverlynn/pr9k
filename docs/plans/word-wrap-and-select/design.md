# Plan: Word-wrap log content + mouse/keyboard text selection and copy

## Context

The ralph-tui log panel has two ergonomic gaps:

1. **Long lines don't wrap.** Streaming subprocess output longer than the viewport width is silently clipped at the outer frame by lipgloss's `MaxWidth`. Content past the right edge is hidden.
2. **No way to select/copy text.** The program runs under `tea.WithAltScreen()` + `tea.WithMouseCellMotion()`, which together defeat native terminal text selection. The user can't select a log line to paste elsewhere — a routine need when debugging a Ralph run.

Goal: the user can highlight any region of the log panel with **mouse drag** or **keyboard** and copy it to the system clipboard. Long lines wrap at word boundaries to the viewport width so nothing is hidden off-screen.

## User flows (the three common paths)

The plan below covers many edge cases — but the rubric for "simple to use" is: do the three common paths take a minimum number of steps, and are they discoverable from the shortcut bar?

1. **Mouse: grab a chunk I see on screen.** Click-drag across it → release → press `y`. (3 steps. Shortcut bar after release shows `y copy  esc cancel`.)
2. **Keyboard: grab the most recent log line.** Press `v` → cursor lands at the start of the **last visible line** (not viewport top) → press `$` (or `End`) to extend to line end → press `y`. (4 keystrokes for the most common case. See "Keyboard cursor initial position" below.)
3. **Keyboard: grab several lines.** Press `v` → extend with `hjkl`/arrows or `shift+↑`/`shift+↓` for whole-line steps → `y`.

The shortcut bar is the source of truth for what's possible in each mode. Anything a user can do must appear there, using no more than the two-line footer budget.

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

- From `ModeNormal` or `ModeDone`, **left-button press** on the viewport area starts a new selection (anchor = cell under cursor, cursor = same cell) and transitions to `ModeSelect` (saving `prevMode`). Presses in `ModeError`, `ModeQuitConfirm`, `ModeNextConfirm`, and `ModeQuitting` are ignored (same rationale as `v`).
- **Drag** (button-left motion) extends the selection's cursor endpoint. Dragging **above** the viewport top or **below** the viewport bottom auto-scrolls the viewport by one line per motion event in that direction, so selections can extend past the visible window.
- **Release**: selection becomes "final" (non-active, still visible). Shortcut bar shows `y copy  esc cancel  v again for new selection`.
- Mouse **wheel** continues to scroll regardless of mode (wheel events are still forwarded to the viewport, which already ignores non-wheel `MouseMsg` events).
- **Press+release at the same cell with no intervening motion (bare click)**: always clears the current selection and starts a new empty one anchored at the click cell. This preserves the universal "click = place cursor, click+drag = select" convention and avoids the complexity of distinguishing "protected committed selection" from "in-flight drag." The user's copy affordance is `y` immediately after release; a stray focus-click *will* clear an uncopied selection, but that matches what any GUI text editor or terminal does. Keyboard `Esc` is the non-destructive cancel.
- **Shift+click**: extends an existing committed selection to the click cell (cursor moves to click, anchor stays). `tea.MouseEvent.Shift` makes this trivial. Standard convention across editors/terminals; covers the "I missed a few chars, let me extend" case without forcing a full redrag.
- Auto-scroll-to-bottom is **suppressed while a selection is `visible()`** (active drag in progress, or a committed non-empty range is displayed). Clearing the selection re-arms auto-scroll.

### Selection — keyboard

- `v` enters `ModeSelect` from `ModeNormal` and `ModeDone`. Excluded from `ModeError`, `ModeQuitConfirm`, `ModeNextConfirm`, and `ModeQuitting`: in Error mode the orchestration goroutine is blocked waiting on `KeyHandler.Actions` (`keys.go:76-78`), and entering Select would leave that goroutine parked until the user remembers to exit Select. The three prompt/terminal modes have confirmations in flight. Mouse-left-press follows the same rule — left-press while in `ModeError` does nothing; the user must press `c`/`r`/`q` first.
- **Keyboard cursor initial position**: anchor and cursor = column 0 of the **last visible visual row** in the viewport (`YOffset + visibleHeight - 1`, clamped to `len(visualLines) - 1`). Rationale: in 90%+ of the "I want to copy a line I just saw" case, the line of interest is near the bottom of the viewport (auto-scroll keeps the latest output visible). Starting at the top forces the user to `j` down to it. Starting at the bottom means the very next keystroke is usually `$`/`End` or `shift+↑` to extend — the minimum. The cursor cell renders with reverse video so the user has an immediate visual indicator of select mode even before moving.
- **Mouse click while already in `ModeSelect` (entered via `v`)**: a bare left-press re-anchors both the anchor and cursor to the click cell; a subsequent drag extends, release commits. This unifies the two entry paths so a user who starts with `v` can still use the mouse to jump the cursor without exiting and re-entering select mode.
- In `ModeSelect`:
  - `h`/`←`, `j`/`↓`, `k`/`↑`, `l`/`→` move the cursor one cell, extending selection from the fixed anchor. Viewport auto-scrolls to keep the cursor visible.
  - `0`/`Home` → jump cursor to column 0 of the current visual row. `$`/`End` → jump cursor to the last column of the current visual row. These are the two most common cases for the "grab this log line" flow and make a single-line selection a 2-keystroke operation from `v` (e.g., `v` then `$` then `y`).
  - `shift+↑`/`shift+↓` (or `K`/`J`) → extend selection by a whole visual row while keeping the cursor's virtual column. Makes multi-line selection a direct shortcut rather than `j $` / `k 0`.
  - `PgUp`/`PgDn` → move cursor by `viewport.Height - 1` rows; the viewport follows the cursor. (In `ModeSelect`, the page keys must not scroll the viewport without moving the cursor, or the cursor drifts off-screen.)
  - The cursor column is **virtual** (vim-style): when moving to a shorter line, the cursor renders clamped at that line's last column, but the virtual column is remembered so moving further to a longer line restores it.
  - `y` or `Enter` → copy selected text to clipboard; on success, emit a synthetic `LogLinesMsg{Lines: []string{"[copied N chars]"}}` (via `tea.Cmd`, same mechanism as the error path) so the user sees confirmation in the log panel; clear selection, return to `prevMode`. When the selection is empty (anchor == cursor), skip the copy and the log line — a silent no-op.
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
- **`tea.KeyMsg` routing fix (regression guard)**: the current root `Update` at `model.go:87-95` forwards every `tea.KeyMsg` to *both* `m.keys.Update` *and* `m.log.Update` (the latter routes `j`/`k`/`up`/`down`/`pgup`/`pgdn` to the viewport for scroll). In `ModeSelect`, this would double-fire — pressing `j` would both extend the selection cursor *and* scroll the viewport, with the scroll then fighting the cursor-follow autoscroll. Fix: when `keys.handler.Mode() == ModeSelect`, skip the `m.log.Update(msg)` forward for `tea.KeyMsg`. `handleSelect` then has sole authority over key-driven motion, and it calls `m.log.SetYOffset`/`ScrollToCursor` explicitly to keep the cursor visible.

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
- Add `SelectShortcuts = "hjkl/↑↓←→ extend  0/$ line  ⇧↑↓ line-ext  y copy  esc cancel  q quit"`. This is the keyboard-mode footer. On mouse-release of a committed selection, swap in `SelectCommittedShortcuts = "y copy  esc cancel  drag for new selection"` so the user sees the exact next-step options instead of the keyboard-extension hints they don't need while holding a mouse-release state.
- Update `NormalShortcuts` and `DoneShortcuts` to include `v select` (Normal: after the scroll hints, before `n next step`; Done: before `q quit`). Do **not** add it to `ErrorShortcuts` — `v` is blocked in ModeError.
- `updateShortcutLineLocked`: map `ModeSelect → SelectShortcuts`.

### `ralph-tui/internal/ui/clipboard.go` (new)

- Thin wrapper: `func CopyToClipboard(text string) error` tries, in order:
  1. `copyFn(text)` — `var copyFn = clipboard.WriteAll` so tests can swap in a fake. This covers local macOS/Linux/Windows.
  2. If `copyFn` returns a non-nil error **and** the process is running under a tty, write an **OSC 52** sequence to stderr: `fmt.Fprintf(os.Stderr, "\x1b]52;c;%s\x07", base64.StdEncoding.EncodeToString([]byte(text)))`. Most modern terminals (iTerm2, kitty, WezTerm, tmux with `set -g set-clipboard on`, recent xterm) intercept this and write to the local clipboard — which is the **only** mechanism that works over SSH into a headless host. Return nil if both mechanisms are tried (OSC 52 has no ack; best-effort).
  3. Rationale for shipping OSC 52 now rather than deferring: Ralph's core deployment is "run on a cloud VM via SSH." `atotto/clipboard` on a headless Linux VM fails unconditionally (no `xclip`/`xsel`/X server). Without OSC 52, the keyboard/mouse select feature would be dead-in-the-water in the most common Ralph environment, making "complete solution" a misnomer. OSC 52 adds ~10 lines of Go and zero new dependencies.
- Wrapping the fallback in `CopyToClipboard` (not `handleSelect`) keeps the control-flow linear: `handleSelect` sees one `error` return and reacts, regardless of which channel succeeded.
- When `CopyToClipboard` returns an error (both `clipboard.WriteAll` failed **and** stderr is not a tty — e.g. a log-redirected run), `handleSelect` returns a `tea.Cmd` whose closure emits a synthetic `LogLinesMsg{Lines: []string{"[copy failed: install xclip/xsel or run in a terminal that supports OSC 52]"}}`. This routes through the existing `Update` path exactly like a streamed line — no cross-goroutine concerns, no new plumbing into `runner.SetSender`, no transient footer state. Mode still transitions back to `prevMode` in the same `handleSelect` pass.
- On success, `handleSelect` emits a parallel informational line `"[copied N chars]"` via the same synthetic-LogLinesMsg mechanism (see Copy format + feedback below).

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
- `TestKeys_V_StartsAtLastVisibleLine` — populate buffer with N lines, press `v`, assert selection anchor/cursor is on the last visible visual row at column 0 (not viewport top).
- `TestKeys_DollarSign_JumpsToLineEnd_InSelectMode` — from `v`, press `$`, assert cursor lands at last column of the current visual row and anchor is unchanged.
- `TestKeys_ShiftDown_ExtendsByLine_InSelectMode` — verifies whole-line extension preserves the virtual column.
- `TestKeys_InSelectMode_DoesNotDoubleDispatchToViewport` — pressing `j` in `ModeSelect` moves the selection cursor and does **not** also scroll the viewport independently (regression guard for the `model.go` routing fix).
- `TestMouse_BareClick_ClearsExistingSelection_AndReanchors` — commit a selection, then bare-click elsewhere; assert the old selection is gone and a new empty selection is anchored at the click cell (replacing the old "press on committed selection is ignored" semantics).
- `TestMouse_ShiftClick_ExtendsCommittedSelection` — commit a selection, shift-click beyond its end; assert the cursor moves to the click cell and anchor stays put.
- `TestMouse_ClickInSelectMode_EnteredViaV_ReanchorsCursor` — enter select mode via `v`, then left-click in the viewport; assert anchor and cursor both jump to the click cell.
- `TestCopy_Success_AppendsConfirmationLogLine` — fake `copyFn` returns nil, assert a `"[copied N chars]"` log line is appended and mode transitions back.
- `TestCopy_Failure_FallsBackToOSC52` — fake `copyFn` returns error, capture stderr, assert an OSC 52 (`\x1b]52;c;<base64>\x07`) sequence was written with the expected payload, and `CopyToClipboard` returns nil (tty-detected test harness). A parallel `TestCopy_Failure_NoTty_EmitsErrorLogLine` covers the no-tty path.
- `TestSetMode_ExternalTransition_ClearsSelection` — with a committed selection visible in `ModeSelect`, call `h.SetMode(ModeError)` from another goroutine, push any message through `Model.Update`, and assert the selection is cleared before the next render.
- `TestKeys_PgDn_MovesCursor_InSelectMode` — in select mode, `PgDn` moves the cursor and the viewport follows; cursor is still visible afterward.

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
- **Orchestration forces mode change while a selection is visible.** `orchestrate.go:89` calls `h.SetMode(ModeError)` when a step fails, which can fire while the user has an active drag or committed selection in `ModeSelect`. Resolution: `KeyHandler.SetMode` is called only from the Update goroutine (`keys.go` handlers) or the orchestration goroutine; when the external path moves the user *out of* `ModeSelect`, the selection state on `logModel` would otherwise linger — rendering a reverse-video overlay with no mode to act on it. Fix: extend the `Model.Update` path so that when the observed mode transitions away from `ModeSelect` (either via keysModel or via an external `SetMode`), it calls `m.log.ClearSelection()` on the next message. Practically: a cheap `if prevObservedMode == ModeSelect && currentMode != ModeSelect { m.log = m.log.ClearSelection() }` check at the top of root `Update`, using a `prevObservedMode` field on `Model`. This is the single mode-change guard; it also covers any future external `SetMode` callers.
- **Copy when selection is empty.** `y`/`Enter` with an empty selection (e.g., committed but `anchor == cursor`) is a no-op that still returns to `prevMode` — no clipboard write, no transient footer. Keeps the mode exit predictable.
- **Terminal without mouse reporting.** If the user's terminal strips mouse bytes or `WithMouseCellMotion` has no effect, the keyboard (`v` + `hjkl`) path still works. No additional handling required.
- **Very wide lines after rewrap.** `x/ansi.Wrap` with a limit of `viewport.Width` will hard-break any single token longer than the width. No risk of infinite recursion or O(n²) blowup; worst case is `raw_len / width` visual rows per raw line.
- **Pre-first-WindowSizeMsg `LogLinesMsg`.** `newLogModel(0, 0)` is constructed with Width=0 (`model.go:64`). If a line arrives before the first `WindowSizeMsg`, `ansi.Wrap(s, 0, " -")` short-circuits and returns the input unchanged (`wrap.go:293-296`) — the `visualLines` builder must treat that as "one segment per raw line" without panicking. First `WindowSizeMsg` triggers a full rewrap via `SetSize`.
- **Rewrap cost under burst load.** The drain goroutine in `main.go:194-216` coalesces lines into one `LogLinesMsg`. Rewrapping the full `logRingBufferCap=2000` buffer on every batch is ~2000 `ansi.Wrap` calls per message. **Budget:** full-buffer rewrap at 200-col width must complete in <50 ms on the developer's machine (benchmark in `log_panel_bench_test.go`). **Optimization path if the budget is missed:** switch to append-only incremental wrap — only the newly-appended lines feed into `ansi.Wrap`; on eviction, drop the corresponding prefix of `visualLines`; full rewrap is reserved for width changes in `SetSize`. This optimization is optional — ship the simple full-rebuild first, benchmark, and only switch if needed.

## Review summary

- **Iterations completed:** 3 passes + full agent validation (evidence-based-investigator + adversarial-validator), then a follow-up simplicity/completeness pass focused on the end-user flow.
- **Assumptions challenged (12 + 7 follow-up):** ordinal stability of `Mode` (refuted — not serialized), need for `muesli/reflow` (refuted — `x/ansi.Wrap` already transitive with matching API), raw-line ANSI cleanliness (verified), ring-buffer eviction rawIdx stability (refuted gap — now handled with decrement + underflow-clears), YOffset semantics & SetContent auto-bottom (verified), MouseMsg 0-indexing (verified), `v`/`h`/`l` key collisions (verified absent), clipboard-error surfacing via sender path (refuted — unreachable from Update goroutine; switched to synthetic `LogLinesMsg` via `tea.Cmd`), resize preserves top raw line (refuted — needed `rawOffset` too, not just `rawIdx`), `v` from ModeError safe (refuted — orchestrator blocks on `Actions`), HandleMouse pointer/value semantics (was underspecified — pinned to value return), visual-col → raw-byte translation (was implicit — extracted as `visualColToRawOffset` helper with tab-normalization on ingest).
  - **Follow-up pass (simplicity + completeness):** (1) keyboard cursor top-left was wrong for the "copy recent line" flow → now starts at last visible row; (2) no `$`/`0`/`shift+↑↓` shortcuts made single-line copy a 5+ keystroke operation → added; (3) `tea.KeyMsg` was double-dispatched to both keysModel and logModel → routing guard added for `ModeSelect`; (4) external `SetMode` mid-selection left a stale overlay → auto-clear on mode-change guard added; (5) committed-selection press-protection rule violated the universal click-to-reanchor convention → simplified to "bare click always restarts, shift+click extends"; (6) no success feedback → synthetic `"[copied N chars]"` log line; (7) `atotto/clipboard` fails unconditionally over SSH to headless Linux (Ralph's primary deployment) → OSC 52 stderr fallback added.
- **Consolidations:** dropped `muesli/reflow` in favor of already-transitive `x/ansi.Wrap`; replaced transient-footer KeyHandler field with a synthetic `LogLinesMsg`; the success-feedback and failure-feedback paths now share the synthetic-log-line mechanism.
- **Ambiguities resolved:** cursor visibility on keyboard entry (reverse-video initial cell), initial cursor position (last visible row, column 0), prevMode propagation from Select→QuitConfirm (uses the pre-Select idle mode), stray-click on committed selection (restarts — matches convention), mouse-in-keyboard-select (re-anchors cursor), empty-viewport + `v` (no-op), mid-drag resize (force-commit before rewrap), orchestrator-triggered mode change mid-select (auto-clear selection), clipboard failure surface (OSC 52 fallback, then log line), copy success surface (confirmation log line), ADR + semver bump (added).
- **Stopped at 3 iterations + 1 follow-up:** the follow-up pass was triggered by a user-facing rubric ("simple to use, complete solution") that surfaced flow-level gaps not visible in the original structural pass. Further iteration on this plan would produce only cosmetic changes.

## Out of scope (deferred)

- Search-in-log (`/` to find). Natural companion feature, separate task.
- Select-all shortcut (`ctrl+a`). Trivial once selection plumbing exists; defer until requested. The `v` + `gg` + `G` + `y` path is available for power users.
- Double-click to select word / triple-click to select line. `tea.MouseMsg` does not expose click counts; detection requires custom timestamp deltas. Defer until reported. `shift+↑`/`shift+↓` (whole-row extension) + `0`/`$` (line-end jump) cover the most common "grab a line" case.
- Middle-click paste / right-click menu. This plan is about *output* selection, not pasting input.
