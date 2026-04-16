# TUI Status Header & Log Display

Manages the visual status display for the ralph-tui terminal interface, showing iteration progress, step checkboxes, log panel rhythm, and the full-width phase banners / per-step headings written into the log body.

- **Last Updated:** 2026-04-16
- **Authors:**
  - River Bailey

## Overview

- `StatusHeader` is a struct that holds the checkbox grid state and iteration line; mutations are applied on the Bubble Tea Update goroutine via `headerProxy` message-passing
- The iteration line is embedded into the top-border title (not rendered as a separate inner row); it shows `Iteration N/M` in bounded mode and `Iteration N` (no total) when total is 0 (unbounded mode)
- When no stream-json event arrives for ≥15s during an active claude step, the title is suffixed with `  ⋯ thinking (Ns)` — a passive heartbeat indicator (D23) that replaces the "feels alive" contribution of token-level streaming without requiring `--include-partial-messages`. The suffix is pure view state: it updates in-place each second and is never appended to the log ring buffer
- Displays step progress as a dynamic grid of rows (4 checkboxes per row), sized at startup to fit the largest phase
- Each step shows one of five states: `[ ]` pending, `[▸]` active, `[✓]` done, `[✗]` failed, `[-]` skipped; the active step marker (`▸`) is rendered in green; all other chrome is light gray
- Switches between phases (initialize, iteration, finalize) by sending `headerPhaseStepsMsg` through `headerProxy`
- The log body is structured with phase banners, iteration separators, per-step "Starting step" banners, variable capture logs, and a final completion summary — all spaced with blank lines (helpers in `log.go`)
- Terminal width for full-width phase banner underlines is detected via `ui.TerminalWidth()` (ioctl TIOCGWINSZ) with an 80-column fallback
- The completion summary line is written to the log body (not the header) as the last non-blank line before `Run` returns
- `View()` hand-builds the entire rounded frame row-by-row (no `lipgloss.Border` wrapper) so the two horizontal rules can use `├─┤` T-junction glyphs that visually connect to the `│` side borders
- The top-border title renders two-tone: the app name (from the `AppTitle` constant, `Power-Ralph.9000`) in green and the iteration detail after ` — ` in white; log body content renders in white; the version label renders in white; shortcut-bar keys render in white with gray descriptions

Key files:
- `ralph-tui/internal/ui/model.go` — Root Bubble Tea `Model`; `Update` routes messages to sub-models; `View` assembles the full TUI output
- `ralph-tui/internal/ui/header.go` — StatusHeader struct, RenderInitializeLine, RenderIterationLine, RenderFinalizeLine, SetPhaseSteps, SetStepState
- `ralph-tui/internal/ui/header_proxy.go` — `HeaderProxy` — sends header mutations as messages via `program.Send`
- `ralph-tui/internal/ui/header_test.go` — Unit tests for header state management
- `ralph-tui/internal/ui/log.go` — Log-body helpers: StepSeparator, RetryStepSeparator, StepStartBanner, PhaseBanner, CaptureLog, CompletionSummary
- `ralph-tui/internal/ui/log_test.go` — Unit tests for log-body helper formatting
- `ralph-tui/internal/ui/terminal.go` — TerminalWidth() and DefaultTerminalWidth for sizing full-width banners
- `ralph-tui/internal/ui/messages.go` — Message types including `HeartbeatReader` interface and `HeartbeatTickMsg` (D23)

## Architecture

```
  Orchestration goroutine        main.go heartbeat goroutine
  (calls headerProxy methods)    (1-second ticker, D23)
         │                              │
         │  program.Send(headerMsg)     │  program.Send(HeartbeatTickMsg)
         ▼                              ▼
  ┌──────────────────────────────────────────────────┐
  │            Bubble Tea Update goroutine            │
  │                                                   │
  │  Model.Update(msg) dispatches:                    │
  │  ├─ headerStepStateMsg   → headerModel.apply()   │
  │  ├─ headerPhaseStepsMsg  → headerModel.apply()   │
  │  ├─ headerIterationLineMsg → apply + SetWindowTitle│
  │  ├─ LogLinesMsg          → logModel.Update()     │
  │  ├─ tea.KeyMsg           → keysModel.Update()    │
  │  ├─ HeartbeatTickMsg     → StatusHeader.HandleHeartbeatTick()│
  │  └─ tea.WindowSizeMsg    → resize viewport        │
  │                                                   │
  │  Model.View() assembles (hand-built frame,       │
  │  row-by-row, no lipgloss.Border wrapper):         │
  │  ┌──────────────────────────────────────────────┐│
  │  │╭── Power-Ralph.9000 — Iteration 2/5 — … ──╮ ││  ← dynamic title in top border (green/white)
  │  │ [▸] Feature work   [ ] Test planning      │ ││  ← checkbox grid (per-cell color)
  │  │ [ ] Test writing   [ ] Code review        │ ││
  │  │├───────────────────────────────────────── ┤ ││  ← HRule (T-junctions)
  │  │ [log panel — bubbles/viewport]            │ ││  ← scrollable log viewport (white text)
  │  │├───────────────────────────────────────── ┤ ││  ← HRule (T-junctions)
  │  │ ↑/k up  ↓/j down  n next  q quit          │ ││  ← shortcut footer (ShortcutLine)
  │  │                     ralph-tui v0.4.1      │ ││  ← version label (right-aligned, white)
  │  │╰─────────────────────────────────────────╯  ││  ← bottom border
  │  └──────────────────────────────────────────────┘│
  └──────────────────────────────────────────────────┘
```

## Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/internal/ui/model.go` | Root Bubble Tea Model: Update message routing, View assembly, renderTopBorder |
| `ralph-tui/internal/ui/header.go` | StatusHeader struct and state mutation methods |
| `ralph-tui/internal/ui/header_proxy.go` | HeaderProxy — sends header mutations via program.Send |
| `ralph-tui/internal/ui/header_test.go` | Tests for iteration/finalization state transitions |
| `ralph-tui/internal/ui/log.go` | Log-body helpers: step/phase banners, capture log, completion summary |
| `ralph-tui/internal/ui/log_test.go` | Tests for log-body helper output |
| `ralph-tui/internal/ui/log_panel.go` | logModel: viewport wrapper, 2000-entry ring buffer (`logRingBufferCap`), word-wrap via `rewrap()`, auto-scroll, logContentStyle |
| `ralph-tui/internal/ui/log_panel_test.go` | Tests for logModel ring buffer, auto-scroll, Home/End key handling |
| `ralph-tui/internal/ui/log_panel_wrap_test.go` | 30 unit tests covering word-wrap, rawOffset accuracy, ring-buffer eviction, SetSize edge cases, auto-scroll behavior, ANSI preservation, and integration via tea.WindowSizeMsg |
| `ralph-tui/internal/ui/selection.go` | `pos` and `selection` data types; `visible()`, `normalized()`, `contains()`, `extractText()`, `mouseToViewport()`, `visualColToRawOffset()` pure helpers |
| `ralph-tui/internal/ui/selection_test.go` | 39 unit tests for all selection data types and helpers, including bounds-guard, edge cases, input immutability, and zero-value safety |
| `ralph-tui/internal/ui/select_mode_test.go` | 16 integration tests for `ModeSelect` entry/exit, shortcut bar, routing guard, and external mode-change clearing |
| `ralph-tui/internal/ui/log_panel_ring_eviction_test.go` | 6 tests for ring-buffer eviction rawIdx adjustment, underflow clearing, auto-scroll suppression, and visual-coord recompute |
| `ralph-tui/internal/ui/log_panel_eviction_recompute_test.go` | 20 edge-case tests for `recomputeSelectionVisualCoords`, `findVisualPos`, eviction adjustment, auto-scroll suppression, SetSize interaction, and model.go integration |
| `ralph-tui/internal/ui/clipboard.go` | `copyToClipboard` (clipboard write + OSC 52 fallback to stderr), `copySelectedText` (async `tea.Cmd` wrapper that appends a feedback `LogLinesMsg`) |
| `ralph-tui/internal/ui/clipboard_copy_test.go` | 9 tests for the clipboard copy flow: y/Enter exits ModeSelect, OSC 52 fallback, `[copied N chars]` / `[copy failed: ...]` feedback, empty-selection no-op, raw coordinate payload, test-seam isolation |
| `ralph-tui/internal/ui/clipboard_additional_test.go` | 21 additional tests across 7 categories: `copyToClipboard` unit, `copySelectedText` helper, model.go routing, `handleSelect` separation of concerns, test-seam safety, LogLinesMsg integration, go mod tidy audit |
| `ralph-tui/internal/ui/mouse_selection_test.go` | 16 integration tests for the mouse selection flow: left-drag selects, release commits, auto-scroll at edges, shift-click extends, bare click re-anchors, wheel scroll unaffected, `SelectCommittedShortcuts` on release, mid-drag resize force-commits |
| `ralph-tui/internal/ui/mouse_selection_extra_test.go` | 24 additional tests across 7 categories: `resolveVisualPos` edge cases, `HandleMouse` unit tests, model.go mouse routing, `selectJustReleased` lifecycle, auto-scroll clamping, shift-click edge cases, `SelectCommittedShortcuts` shortcut path |
| `ralph-tui/internal/ui/terminal.go` | `TerminalWidth()` via ioctl + `DefaultTerminalWidth` fallback |

## Core Types

```go
// StepState represents the display state of a single workflow step.
type StepState int

const (
    StepPending StepState = iota  // [ ]
    StepActive                     // [▸]
    StepDone                       // [✓]
    StepFailed                     // [✗]
    StepSkipped                    // [-]
)

// HeaderCols is the number of checkbox columns per row; constant to fit 80-column terminals.
const HeaderCols = 4

// StatusHeader manages the checkbox grid state and iteration line.
// Each cell stores its content split across parallel Prefixes/Markers/Suffixes/Colors
// fields so the marker glyph can be colored independently from the brackets and name.
type StatusHeader struct {
    IterationLine string               // e.g. "Iteration 2/5 — Issue #42"

    Rows          [][HeaderCols]string // legacy single-string labels ("[X] name") — test assertions only

    // Split-cell fields: the checkbox grid is rendered from these.
    Prefixes     [][HeaderCols]string
    Markers      [][HeaderCols]string
    Suffixes     [][HeaderCols]string
    MarkerColors [][HeaderCols]lipgloss.Color
    NameColors   [][HeaderCols]lipgloss.Color

    stepNames []string // current phase's step name list

    // D23 heartbeat indicator fields (set via SetHeartbeatReader / HandleHeartbeatTick).
    heartbeat       HeartbeatReader // nil when disabled; set by main.go before program.Run
    heartbeatSuffix string          // "  ⋯ thinking (Ns)" when active; "" otherwise
}
```

## Implementation Details

### Startup Sizing

`NewStatusHeader` takes the maximum step count across all phases and sizes the grid to fit. The row count is computed via ceiling division so all steps fit without overflow:

```go
func NewStatusHeader(maxStepsAcrossPhases int) *StatusHeader {
    rowCount := max((maxStepsAcrossPhases+HeaderCols-1)/HeaderCols, 1) // ceil division, min 1
    return &StatusHeader{
        Rows:         make([][HeaderCols]string, rowCount),
        Prefixes:     make([][HeaderCols]string, rowCount),
        Markers:      make([][HeaderCols]string, rowCount),
        Suffixes:     make([][HeaderCols]string, rowCount),
        MarkerColors: make([][HeaderCols]lipgloss.Color, rowCount),
        NameColors:   make([][HeaderCols]lipgloss.Color, rowCount),
    }
}
```

### Phase Switching

`SetPhaseSteps` replaces the current step name list and re-renders all checkbox slots. Call this at the start of each phase to swap the header to the new phase's step set. Trailing slots beyond the current phase's step count are cleared. Panics if `len(names)` exceeds the grid capacity — this is a programming error, not a user-reachable path:

```go
func (h *StatusHeader) SetPhaseSteps(names []string) {
    // panics if len(names) > len(h.Rows)*HeaderCols
    h.stepNames = append(h.stepNames[:0], names...)  // copy — does not alias input
    // fills each slot with pending state; clears trailing slots
}
```

### Iteration Line and Step State

Three phase-specific render methods update `IterationLine`. `SetStepState` updates individual step checkboxes (0-indexed within the current phase's step list):

```go
// RenderInitializeLine: "Initializing 1/2: Splash"
func (h *StatusHeader) RenderInitializeLine(stepNum, stepCount int, stepName string)

// RenderIterationLine: "Iteration 2/5 — Issue #42" (bounded, with issue)
//                      "Iteration 3" (unbounded, no issue)
func (h *StatusHeader) RenderIterationLine(iter, maxIter int, issueID string)

// RenderFinalizeLine: "Finalizing 1/3: Deferred work"
func (h *StatusHeader) RenderFinalizeLine(stepNum, stepCount int, stepName string)

func (h *StatusHeader) SetStepState(idx int, state StepState) {
    if idx < 0 || idx >= len(h.stepNames) { return }  // bounds guard
    r, c := idx/HeaderCols, idx%HeaderCols
    h.writeCell(r, c, state, h.stepNames[idx])
}
```

### headerProxy Message Passing

The orchestration goroutine never mutates `StatusHeader` directly. Instead, it calls methods on `HeaderProxy`, which wraps each mutation as a typed message and sends it via `program.Send`. The Bubble Tea `Update` goroutine receives the message and calls `headerModel.apply()`, which performs the actual mutation on the `StatusHeader`. This eliminates data races that previously existed when goroutines wrote to header fields directly.

```go
// HeaderProxy funnels all StatusHeader mutations through program.Send.
type HeaderProxy struct {
    send func(tea.Msg)
}

func NewHeaderProxy(send func(tea.Msg)) *HeaderProxy {
    return &HeaderProxy{send: send}
}

func (p *HeaderProxy) SetStepState(idx int, state StepState) {
    p.send(headerStepStateMsg{idx: idx, state: state})
}
// ... RenderIterationLine, SetPhaseSteps, etc. similarly
```

### Heartbeat Indicator (D23)

During active claude steps, stream-json can be silent for 30+ seconds while claude thinks or waits on a slow tool. The heartbeat indicator shows `  ⋯ thinking (Ns)` in the top-border title after 15 seconds of silence, updating in place each second until the next event arrives.

**Architecture:**
- `ui.HeartbeatReader` interface (in `messages.go`) exposes `HeartbeatSilence() (time.Duration, bool)` — implemented by `workflow.Runner`
- `Runner.HeartbeatSilence()` reads `activePipeline.LastEventAt()` under `processMu`. When no events have arrived yet (zero `LastEventAt`), it counts silence from `activePipelineStartedAt` instead
- `Runner.activePipeline` is set when a claude step starts (in `RunSandboxedStep`) and cleared in a LIFO defer before `pipeline.Close()`
- `StatusHeader.SetHeartbeatReader(r HeartbeatReader)` installs the reader; in `main.go` this is called directly on the `*StatusHeader` before constructing the `Model`
- `Model.WithHeartbeat(h HeartbeatReader)` is a convenience wrapper that delegates to `StatusHeader.SetHeartbeatReader`; used in tests to avoid calling `header.SetHeartbeatReader` directly
- `Model.Init()` returns nil unconditionally — the ticker is **not** started here
- The 1-second `HeartbeatTickMsg` ticker is owned by an explicit goroutine in `main.go` that calls `program.Send(HeartbeatTickMsg{})` once per second; the goroutine terminates with the process
- `Model.Update()` handles `HeartbeatTickMsg` by calling `m.header.header.HandleHeartbeatTick()` — no reschedule cmd is returned because the ticker is managed by the main.go goroutine, not by the Bubble Tea event loop
- `StatusHeader.HandleHeartbeatTick()` queries the installed reader: if `silentFor >= 15s` and active, sets `heartbeatSuffix`; otherwise clears it. If no reader is installed, clears the suffix (safe no-op)
- `Model.titleString()` appends `m.header.header.heartbeatSuffix` to the iteration line: `"Power-Ralph.9000 — Iteration 2/5 — Issue #42  ⋯ thinking (17s)"`

The suffix is pure view state — it is never sent to the log ring buffer and does not affect the log body.

```go
// ui.HeartbeatReader — implemented by workflow.Runner
type HeartbeatReader interface {
    HeartbeatSilence() (time.Duration, bool)
}

// StatusHeader fields added for D23:
//   heartbeat       HeartbeatReader  // nil when disabled
//   heartbeatSuffix string           // "  ⋯ thinking (Ns)"; empty when inactive

// StatusHeader.SetHeartbeatReader installs the reader.
func (h *StatusHeader) SetHeartbeatReader(r HeartbeatReader) { h.heartbeat = r }

// StatusHeader.HandleHeartbeatTick updates heartbeatSuffix. Call on every HeartbeatTickMsg.
func (h *StatusHeader) HandleHeartbeatTick() {
    if h.heartbeat == nil { h.heartbeatSuffix = ""; return }
    silentFor, active := h.heartbeat.HeartbeatSilence()
    if active && silentFor >= heartbeatSilenceThreshold {
        h.heartbeatSuffix = fmt.Sprintf("  ⋯ thinking (%ds)", int(silentFor.Seconds()))
    } else {
        h.heartbeatSuffix = ""
    }
}

// main.go — wiring (runs before program.Run):
header.SetHeartbeatReader(runner)
go func() {
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    for range ticker.C {
        program.Send(ui.HeartbeatTickMsg{})
    }
}()

// workflow.Runner — satisfies HeartbeatReader
func (r *Runner) HeartbeatSilence() (time.Duration, bool) {
    r.processMu.Lock()
    pipeline := r.activePipeline
    startedAt := r.activePipelineStartedAt
    r.processMu.Unlock()

    if pipeline == nil {
        return 0, false
    }
    t := pipeline.LastEventAt()
    if t.IsZero() {
        return time.Since(startedAt), true
    }
    return time.Since(t), true
}
```

### View Assembly

`Model.View()` hand-builds the complete TUI output row-by-row each render cycle, using a small `wrapLine` helper that wraps each content line in `│ … │` side borders with right-padding to `innerWidth` (and `MaxWidth` truncation for overflow):

1. **Top border with title** — `renderTopBorder(titleString())` builds a hand-crafted `╭── Power-Ralph.9000 — <iterationLine> ─ … ─╮`. The title is two-tone colored via `colorTitle()`: the `AppTitle` constant (`Power-Ralph.9000`) renders in green (color 10) and the iteration detail after the ` — ` separator renders in white (color 15). `lipgloss.Width` provides rune-aware truncation when the title overflows.
2. **Checkbox grid** — before rendering, a first pass computes `contentMaxWidth` by calling `lipgloss.Width` on every `Rows[r][c]` value across all rows and columns. A terminal-derived `termCellWidth` is also computed as `(innerWidth - separatorWidth) / HeaderCols`, and `cellWidth = max(termCellWidth, contentMaxWidth)` takes whichever is larger — this makes the grid fill the terminal edge-to-edge on wide terminals and fall back to content-width on narrow ones. A second pass renders each cell as three adjacent Lip Gloss spans: `Prefix` + `Marker` + `Suffix`, each with its own `lipgloss.Color` from `NameColors`/`MarkerColors`. After the three spans are written, trailing spaces are appended until the cell reaches `cellWidth`, so all four columns occupy the same visual width and the step list is distributed evenly across the header. Active steps appear in white/green; all other states in light gray. Each row is wrapped in sidebars via `wrapLine`.
3. **HRule** — `gray.Render("├" + strings.Repeat("─", innerWidth) + "┤")` uses T-junction glyphs so the rule visually connects to the `│` side borders at both ends.
4. **Log viewport** — `m.log.View()` from the `bubbles/viewport` sub-model is split on `\n` and each resulting line is wrapped individually via `wrapLine`. The viewport content is set through `logContentStyle` (`White` foreground) so log body text pops against the gray chrome.
5. **HRule** — same T-junction form as step 3.
6. **Footer** — shortcut bar (left, from `m.keys.handler.ShortcutLine()`, passed through `colorShortcutLine` for per-key coloring) + spacer + version label (right, from `m.versionLabel`, rendered white), wrapped via `wrapLine`.
7. **Bottom border** — `gray.Render("╰" + strings.Repeat("─", innerWidth) + "╯")`.

There is **no** `lipgloss.Border` wrapper around the inner block. The previous approach (wrapping inner content in a `Border(RoundedBorder()).BorderTop(false)` style) was scrapped because it forced plain `─` on the hrule rows and left visual gaps at the side-border intersections; hand-building each row is what lets the hrules use the `├─┤` T-junction glyphs.

### Shortcut Footer Coloring

`colorShortcutLine()` applies the footer's per-key color scheme. For the key-mapping lines (`NormalShortcuts`, `ErrorShortcuts`), the string is split on `"  "` (double-space) into groups; within each group the first whitespace-separated token is the mapped key (rendered `White`) and the rest is its description (rendered `LightGray`). The `"  "` separators between groups remain `LightGray`.

For status-message lines (`QuitConfirmPrompt`, `NextConfirmPrompt`, `QuittingLine`) the whole string renders in `White`, with one exception: when the line is `QuitConfirmPrompt`, `colorShortcutLine` splits the string on the `AppTitle` substring and renders that substring in `Green` to match the top-border title's brand color, with the surrounding prompt text in `White`. `DoneShortcuts` (`"q quit"`) uses the default two-tone rendering (key white, description gray) since it follows the standard key-mapping format.

### Checkbox Label Formatting

Each cell is split into three adjacent spans using the split-cell fields. `cellStyle` returns the marker glyph and per-cell colors for a given step state:

```go
func cellStyle(state StepState) (marker string, nameColor, markerColor lipgloss.Color) {
    switch state {
    case StepActive:  return "▸", ActiveStepFG, ActiveMarkerFG  // white name, green marker
    case StepDone:    return "✓", LightGray, LightGray
    case StepFailed:  return "✗", LightGray, LightGray
    case StepSkipped: return "-", LightGray, LightGray
    default:          return " ", LightGray, LightGray           // pending
    }
}
```

### Word-wrap

The log panel wraps long lines at word boundaries to the current viewport width, so no content is hidden off the right edge.

**Core types and helpers** (all in `log_panel.go`):

```go
// visualLine is one word-wrapped segment of a raw log line.
// rawIdx and rawOffset provide stable back-references into lines[]
// for scroll-position preservation across resizes and future copy/select work.
type visualLine struct {
    text      string // wrapped segment (plain text; logContentStyle applied at render time)
    raw       string // same as text for plain-text ring buffers; reserved for copy
    rawIdx    int    // index into m.lines
    rawOffset int    // byte offset within lines[rawIdx] where this segment starts
}

// logModel holds both the raw ring buffer and the derived visual-line slice.
type logModel struct {
    viewport    viewport.Model
    lines       []string     // ring buffer (cap 2000); one entry per original logical line
    visualLines []visualLine // rebuilt on every rewrap; one entry per wrapped visual row
}
```

**`rewrap(width int)`** — rebuilds `visualLines` by calling `ansi.Wrap(line, width, " -")` once per raw line. `ansi.Wrap` handles word-wrap with a hard-break fallback for over-long tokens. Results are split on `\n` to extract individual segments; `rawOffset` is advanced by `len(seg)` then by any trailing space that `ansi.Wrap` consumed as a word-break separator. When `width < 1` (before the first `tea.WindowSizeMsg`), `ansi.Wrap` returns the input unchanged — each raw line produces exactly one visual segment.

**`renderContent()`** — joins all visual-line text fields with `"\n"` and wraps the result in `logContentStyle` (white foreground). This is the single string passed to `viewport.SetContent`.

**Tab normalization** — on `LogLinesMsg` ingest, each raw line is passed through `strings.ReplaceAll(line, "\t", "    ")` before being appended to `lines[]`. This converts tabs to four spaces so `visualColToRawOffset` (in `selection.go`) remains a simple byte walk.

**Resize position preservation** — when `SetSize` is called with a changed width:
1. Before rewrapping, the `visualLine` at `viewport.YOffset` is snapshotted to capture `(rawIdx, rawOffset)`.
2. `rewrap(width)` and `renderContent()` rebuild the visual-line slice for the new width.
3. After rewrap, `visualLines` is scanned for the largest index `i` such that `visualLines[i].rawIdx == snap.rawIdx && visualLines[i].rawOffset <= snap.rawOffset`, and `viewport.SetYOffset(i)` restores that position. Using `rawOffset` (not just `rawIdx`) prevents the viewport from jumping backward to the first segment of a multi-segment raw line when the terminal narrows.

Height-only changes skip the rewrap step entirely — only `viewport.Width` and `viewport.Height` are updated.

**Known limitation (TODO #103):** `rawOffset` is advanced by `len(seg)` — the byte length of the *wrapped output* segment. If a raw line contains ANSI escape sequences, `ansi.Wrap` may insert reset/re-open codes at wrap boundaries, making the segment byte length diverge from the raw-line bytes consumed. Scroll-position restoration and future copy/select will silently produce wrong offsets in that case. Currently low severity (claudestream emits unstyled text). Tracked as issue #103.

### ModeSelect: Keyboard Text Selection

Pressing `v` from `ModeNormal` or `ModeDone` enters `ModeSelect`. The cursor appears as a single reverse-video cell at column 0 of the last visible visual row. `Esc` clears the selection and returns to the prior mode. `y` or `Enter` copies the selected text to the clipboard and exits `ModeSelect`.

**Entry conditions:**
- `v` is accepted only in `ModeNormal` and `ModeDone`. It is blocked in `ModeError` (the orchestration goroutine is blocked on `KeyHandler.Actions`), `ModeQuitConfirm`, `ModeNextConfirm`, and `ModeQuitting`.
- `v` with an empty log buffer is a no-op (no transition to `ModeSelect`).

**Selection state on `logModel`:**
- `logModel.sel selection` holds the current selection (zero value = no selection).
- `SetSelection(sel selection) logModel` replaces the selection and re-renders viewport content.
- `ClearSelection() logModel` removes the selection and re-renders.
- `SelectedText() string` reconstructs the selected text from raw ring-buffer lines (returns `""` for an empty or invalid selection).
- `initSelectionAtLastVisibleRow() selection` returns an empty selection (anchor == cursor) at column 0 of the last visible visual row (`YOffset + Height - 1`, clamped). Used when `v` is pressed.

**Reverse-video rendering:**
`renderContent()` applies `lipgloss.NewStyle().Reverse(true)` to cells within the selection range. An empty selection (anchor == cursor) renders a single reverse-video cursor cell at the cursor position so the user has an immediate visual indicator before any movement key is pressed. Rendering is only active when `sel.active || sel.committed`.

**Key routing guard (regression prevention):**
When `m.keys.Mode() == ModeSelect`, `Model.Update` skips the `m.log.Update(msg)` forward for `tea.KeyMsg`. This prevents movement keys (h/j/k/l) from double-dispatching to the viewport scroll handler when `handleSelectKey` drives viewport positioning explicitly via `autoscrollToCursor`.

**External mode-change guard:**
`Model` tracks `prevObservedMode Mode`. At the start of each `Update` call, if `prevObservedMode == ModeSelect` and the current mode is not `ModeSelect`, `m.log.ClearSelection()` is called. This covers `orchestrate.go` calling `h.SetMode(ModeError)` while a selection is visible — the stale overlay is cleared on the next Bubble Tea update cycle. `prevObservedMode` is updated at the end of `Update` (after all dispatch) so it reflects the mode visible to the previous rendered frame.

#### Cursor Movement in ModeSelect

When the mode is `ModeSelect` and a key has not caused a mode transition (Esc and q are intercepted by `keys.go` before reaching this layer), `Model.Update` calls `m.log.handleSelectKey(msg)`, which dispatches to one of the movement methods below. All methods share a guard invariant — they return `(m, nil)` immediately if `!sel.active && !sel.committed` — so no movement can occur on a zero-value (uninitialised) selection.

| Method | Trigger keys | Behavior |
|--------|-------------|----------|
| `MoveSelectionCursor(dx, dy)` | h/l/←/→ (dx), j/k/↓/↑ (dy) | Moves cursor by one display column or visual row. Horizontal moves clamp to `[0, lastColOfRow]` and update `virtualCol`. Vertical moves restore `virtualCol` clamped to the new row's width (vim-style). `rawIdx` and `rawOffset` are recomputed after every move via `visualColToRawOffset`. |
| `JumpSelectionCursorToLineStart()` | `0`, `Home` | Sets cursor to column 0 of the current visual row. Sets `rawOffset = visualLines[row].rawOffset` (segment start). Resets `virtualCol = 0`. |
| `JumpSelectionCursorToLineEnd()` | `$`, `End` | Sets cursor to `lipgloss.Width(vl.text)` of the current visual row. Recomputes `rawOffset` via `visualColToRawOffset`. Updates `virtualCol`. |
| `ExtendSelectionByLine(dy)` | `J`/`Shift+↓` (+1), `K`/`Shift+↑` (-1) | Thin wrapper over `MoveSelectionCursor(0, dy)`. Moves by one whole visual row; virtual column is preserved. |
| `PageSelectionCursor(dy)` | `PgDn` (+1), `PgUp` (-1) | Calls `MoveSelectionCursor(0, dy*(viewport.Height-1))`. Page step is at least 1 when viewport height is 1. |

**`virtualCol`** is a separate `int` field on `logModel` (not inside `pos`) that stores the intended column across vertical moves. It is set whenever the cursor moves horizontally or jumps to a line boundary, and is restored (clamped) on each vertical step so that a cursor on a short line snaps back to the original column when it returns to a longer line.

**`autoscrollToCursor()`** is called at the end of every movement method, before `viewport.SetContent(m.renderContent())`. It adjusts `viewport.YOffset` so `sel.cursor.visualRow` is within `[YOffset, YOffset+Height)`. Moving above the viewport scrolls up; moving below scrolls down.

**Bounds guard on ring-buffer eviction:** before indexing `m.lines[vl.rawIdx]`, `MoveSelectionCursor` and `JumpSelectionCursorToLineEnd` check `vl.rawIdx >= 0 && vl.rawIdx < len(m.lines)` and return `(m, nil)` if the index is stale. This prevents panics when the ring buffer wraps and an old `rawIdx` no longer points to a valid entry.

#### Clipboard Copy in ModeSelect

Pressing `y` or `Enter` from `ModeSelect` copies the selected text to the clipboard and exits ModeSelect (restoring prevMode). The copy flow spans two files:

**`keys.go`** — `handleSelect` intercepts `y`/`Enter` and transitions the mode to `prevMode` (same path as `Esc`, but without clearing the selection yet). The actual payload extraction and copy command are delegated to `model.go`, which has access to both `keysModel` and `logModel`.

**`model.go`** — In the "immediate selection clear" block, a `y`/`Enter` case extracts `m.log.SelectedText()` before calling `m.log.ClearSelection()`, then enqueues `copySelectedText(text)` as a `tea.Cmd`.

**`clipboard.go`** — `copySelectedText(text string) tea.Cmd` returns a closure that:
1. Calls `copyToClipboard(text)` asynchronously (inside the `tea.Cmd` so it does not block the Bubble Tea Update goroutine).
2. On success, emits `LogLinesMsg{Lines: []string{"[copied N chars]"}}` (byte count, not rune count — cosmetic).
3. On failure with a TTY stderr, writes an OSC 52 escape sequence (`\x1b]52;c;<base64>\x07`) to stderr and emits nothing (best-effort, no acknowledgement protocol).
4. On failure without a TTY stderr, emits `LogLinesMsg{Lines: []string{"[copy failed: install xclip/xsel or run in a terminal that supports OSC 52]"}}`.

If `text` is empty, `copySelectedText` returns `nil` (no copy attempt, no feedback, silent no-op).

Three package-level vars provide test seams: `copyFn` (default: `clipboard.WriteAll`), `isTTYFn` (default: `term.IsTerminal(stderr)`), and `stderrWriter` (default: `os.Stderr`). None are mutex-protected — tests must not call `t.Parallel()` while mutating them.

**go.mod** — `github.com/atotto/clipboard v0.1.4` and `golang.org/x/term v0.42.0` were promoted to direct dependencies.

### Mouse Selection

Left-clicking and dragging in the log viewport selects text without requiring the user to press `v` first. The feature is implemented across `log_panel.go` (`resolveVisualPos`, `HandleMouse`), `model.go` (mouse routing), and `ui.go` (`selectJustReleased`, `clearJustReleasedLocked`, `SelectCommittedShortcuts`).

**`resolveVisualPos(p pos) (pos, bool)`** — fills in `rawIdx`, `rawOffset`, and (clamped) `col` on a `pos` whose only valid fields are `visualRow` and `col`. Returns `(p, false)` when `p.visualRow` is out of range or the backing raw line is invalid. `p.col` is clamped to `[0, rowWidth]` so that clicks past the end of a short row anchor at the last column rather than overflowing into the next raw line.

**`HandleMouse(p pos, action tea.MouseAction, shift bool) (logModel, tea.Cmd)`** — applies a single translated mouse event to the selection state. All three action variants are handled:

| Action | Behavior |
|--------|----------|
| `MouseActionPress` (bare) | Clears any existing selection; anchors a new one at `p` with `active=true`. If `resolveVisualPos` returns `false` (click below content), selection stays unchanged and the method returns immediately without setting `active`. |
| `MouseActionPress` (shift) | If a committed selection exists, moves only the cursor to `p` (shift-click extend); anchor stays fixed. |
| `MouseActionMotion` | No-op when `!sel.active`. When active: auto-scrolls one line per event when the pointer is above or below the visible window (`YOffset-1` or `YOffset+Height`), then clamps `visualRow` to `[0, len(visualLines)-1]` before resolving raw coords and updating `sel.cursor`. |
| `MouseActionRelease` | No-op when `!sel.active`. Commits: sets `active=false`, `committed=true`. |

**Mode transitions on mouse press (model.go):**
- `ModeNormal` or `ModeDone` → `ModeSelect`: triggered when a left-press lands on content and `m.log.sel.active == true` after `HandleMouse`. The `prevMode` is saved and `updateShortcutLineLocked` updates the footer.
- `ModeSelect` left-press: re-anchors the selection cursor (handled inside `HandleMouse`); mode stays `ModeSelect`.
- `ModeError`, `ModeQuitConfirm`, `ModeNextConfirm`, `ModeQuitting`: left-press is ignored.

**Mouse wheel:** wheel events (`WheelUp`/`WheelDown`/`WheelLeft`/`WheelRight`) are always forwarded to the viewport via `m.log.Update(msg)` regardless of mode, so scrolling works in every mode including `ModeSelect`.

**Non-left-button guard:** the `else if msg.Button == tea.MouseButtonLeft || msg.Button == tea.MouseButtonNone` guard prevents right-click and middle-click events from accidentally triggering selection. `MouseButtonNone` covers terminal emulators that report motion events without a button field.

**`SelectCommittedShortcuts`** (`"y copy  esc cancel  drag for new selection"`) replaces `SelectShortcuts` in the footer immediately after a drag release. The `selectJustReleased bool` field on `KeyHandler` (protected by `mu`) is set to `true` inside `updateShortcutLineLocked` when `mode == ModeSelect && selectJustReleased`. The flag is cleared on the next key or mouse event via `clearJustReleasedLocked()`.

**Mid-drag resize:** when `SetSize` is called while a drag is in progress (`sel.active == true`), the selection is force-committed (`active=false; committed=true`) before rewrap. This preserves the raw coordinates already captured by preceding Motion events and avoids corrupting the selection state during viewport dimension changes.

**Auto-scroll detail:** during a drag, `Motion` events pass an unclamped `visualRow` computed as `viewport.YOffset + (msg.Y - logTopRow)` to `HandleMouse`. Negative rows (pointer above viewport) trigger `SetYOffset(-1)`; rows past `YOffset+Height-1` trigger `SetYOffset(+1)`. The row is then clamped before `resolveVisualPos` so the cursor endpoint remains inside the actual content bounds.

**Coordinate translation:** `logTopRow` (first row of viewport content) is computed as `len(m.header.header.Rows) + 2` (1 top border + gridRows checkbox rows + 1 hrule). `logLeftCol` is always 1 (inside the left border character). `mouseToViewport` (in `selection.go`) maps the raw `tea.MouseMsg` coordinates to viewport-space `(visualRow, col)` and returns `ok=false` when the click lands on chrome outside the viewport height.

### Ring-Buffer Eviction and Selection Recompute

When new log lines push the ring buffer past `logRingBufferCap` (2000), old lines are evicted from the front. Two interleaved concerns must be handled on every `LogLinesMsg`:

**Eviction adjustment:** If a selection is active or committed and lines are evicted, the selection's `rawIdx` values are decremented by the eviction count so they still point to the same logical content (now at a lower index). If either `rawIdx` underflows to < 0, the entire selection is cleared — half-valid ranges would produce corrupt text on copy.

```
if evicted > 0 && (sel.active || sel.committed) {
    sel.anchor.rawIdx -= evicted
    sel.cursor.rawIdx -= evicted
    if sel.anchor.rawIdx < 0 || sel.cursor.rawIdx < 0 {
        sel = selection{}  // clear the whole thing
    }
}
```

**Visual coordinate recompute:** After every `rewrap`, `recomputeSelectionVisualCoords()` is called to update `anchor.visualRow`/`col` and `cursor.visualRow`/`col` from their authoritative `rawIdx`/`rawOffset` coordinates. This keeps `renderContent` highlighting the correct rows after wrapping changes how many visual lines each raw line occupies. If either position cannot be found in `visualLines` (e.g., `rawIdx` was evicted and the underflow guard missed it, or the offset is past the end of the raw line), the selection is cleared.

**`recomputeSelectionVisualCoords()`** — delegates to `findVisualPos` for both anchor and cursor. Returns the zero-value `selection{}` when the selection is not visible or either position is not locatable.

**`findVisualPos(p pos) (pos, bool)`** — scans `visualLines` for the last entry where `rawIdx == p.rawIdx && rawOffset <= p.rawOffset`, sets `p.visualRow` to that index, and recomputes `p.col` as `lipgloss.Width(rawLine[vl.rawOffset:p.rawOffset])`. Returns `(p, false)` when no matching segment exists.

**Auto-scroll suppression:** `wasAtBottom` is snapshotted before the rewrap. After the rewrap and visual-coord recompute, `GotoBottom()` is only called when `wasAtBottom && !sel.visible()`. This prevents live-streaming output from dragging the viewport away from the selected range while a selection is active or committed. Auto-scroll re-arms as soon as the selection is cleared.

**Known limitation (P1, deferred):** `SetSize` does not call `recomputeSelectionVisualCoords` after its rewrap. Visual coordinates on an active selection will be stale until the next `LogLinesMsg` triggers a recompute. Low severity in practice because resizes and active selections rarely coincide.

### Selection Primitives

`ralph-tui/internal/ui/selection.go` defines the coordinate and selection data types used for text selection in the log panel, along with pure helper functions.

**Core types:**

```go
// pos is a cursor position in the log panel.
// rawIdx and rawOffset are the authoritative coordinates — stable across
// rewrap and ring-buffer eviction decrements.
// visualRow and col are derived render-time coordinates; recomputed from
// visualLines on every rewrap. col is virtual (vim-style): movement preserves
// the intended column across shorter lines.
type pos struct {
    rawIdx    int
    rawOffset int
    visualRow int
    col       int
}

// selection holds the anchor and cursor positions for a text selection.
// active is true while a mouse drag is in progress (mid-drag).
// committed is true once the drag has been released (or a keyboard selection
// has been finalised) and the range is ready to copy.
type selection struct {
    anchor    pos
    cursor    pos
    active    bool
    committed bool
}
```

**Pure helpers:**

- **`visible() bool`** — returns `true` when the selection should be displayed: active (mid-drag) or committed with a non-empty range (`anchor != cursor` by raw coordinates). Used to gate auto-scroll-to-bottom suppression.

- **`normalized() (start, end pos)`** — returns anchor and cursor ordered in reading order by `(rawIdx, rawOffset)`, so `start` is always earlier in the document than `end`. Visual coordinates are copied as-is and may be stale.

- **`contains(row, col int) bool`** — reports whether the given visual `(row, col)` falls within the selected range using a half-open col convention (start included, end excluded). Callers must ensure visual coordinates are up-to-date with the current `visualLines` before calling; `contains` operates on derived render-time coordinates that may be stale between a rewrap and the next visual-coordinate refresh.

- **`extractText(lines []string, start, end pos) string`** — reconstructs the selected text from raw ring-buffer lines using raw coordinates, so wrap-induced visual segments never inject artificial newlines into the result. Returns `""` if any index or offset is out of range (stale `rawIdx` after ring-buffer eviction, `rawOffset` past end of line, or reversed offsets on the same line). Guards prevent panics on any invalid input.

- **`mouseToViewport(msg tea.MouseMsg, topRow, leftCol int, vp viewport.Model) (pos, bool)`** — translates a `tea.MouseMsg` into viewport-content coordinates. Returns a `pos` with `visualRow = vp.YOffset + (msg.Y - topRow)` and `col = msg.X - leftCol`. `ok` is `false` when `msg.Y` is above `topRow` or below `topRow + vp.Height - 1` (click landed on chrome). Unexported — all callers are package-internal.

- **`visualColToRawOffset(rawLine string, segmentStart int, col int) int`** — converts a display-column index within a visual segment to a byte offset within `rawLine`. Walks forward from `segmentStart` counting display cells via Unicode grapheme-cluster boundaries (`github.com/rivo/uniseg`). If `col` falls within a multi-cell grapheme (e.g., a 2-cell-wide CJK character), returns the byte offset of that grapheme's start. Returns end of `rawLine` when `col` exceeds available cells.

### Log-Body Helpers

`ralph-tui/internal/ui/log.go` owns every helper that writes structured chrome into the log body. Every helper returns a plain string (or a tuple of strings) — the workflow loop calls `executor.WriteToLog()` with the result.

```go
// Iteration separator: "── Iteration 1 ─────────────"
func StepSeparator(stepName string) string

// Retry separator: "── <step name> (retry) ─────────────"
func RetryStepSeparator(stepName string) string

// StepStartBanner: returns the two-line "Starting step: <name>" heading and
// a "─"-character underline whose rune count matches the heading width.
func StepStartBanner(stepName string) (heading, underline string)

// PhaseBanner: returns the phase name plus a full-width "═"-character
// underline `width` runes wide. Widths <= 0 are clamped to 1.
func PhaseBanner(phaseName string, width int) (heading, underline string)

// CaptureLog: returns a single-line `Captured VAR = "value"` log entry.
func CaptureLog(varName, value string) string

// CompletionSummary: the final body line written before Run returns.
// Format: "Ralph completed after N iteration(s) and M finalizing tasks."
func CompletionSummary(iterationsRun, finalizeCount int) string
```

### Log Body Rhythm

`workflow.Run` interleaves helper output with subprocess streaming to produce a readable run log. A local `emitBlank` closure writes a single blank separator line before every content block (no-op on the very first content so the log does not begin with a blank line). The rhythm for a run with one init step, two iterations of two steps each, and two finalize steps — assuming the second iteration step in each iteration has `captureAs: "ISSUE_ID"` — is:

```
Initializing
════════════════════════════════════════

Starting step: Get GitHub user
──────────────────────────────

[init step output]

════════════════════════════════════════
Iterations
════════════════════════════════════════

── Iteration 1 ─────────────

Starting step: Get next issue
─────────────────────────────

[step output]

Captured ISSUE_ID = "42"

Starting step: Feature work
───────────────────────────

[step output]

── Iteration 2 ─────────────

Starting step: Get next issue
─────────────────────────────

[step output]

Captured ISSUE_ID = "43"

Starting step: Feature work
───────────────────────────

[step output]

Finalizing
════════════════════════════════════════

Starting step: Deferred work
────────────────────────────

[final step output]

Starting step: Final git push
─────────────────────────────

[final step output]

Ralph completed after 2 iteration(s) and 2 finalizing tasks.
```

Key properties:
- Phase banners are full-width (`═` repeated `cfg.LogWidth` runes); the underline fills the log panel so phase transitions stand out
- Iteration separators (`── Iteration N ─────────────`) are a fixed-width `StepSeparator`
- Per-step banners (`Starting step: <name>` + `─` underline of matching width) are written by `Orchestrate` for every started step — initialize, iteration, and finalize phases all share this via `StepStartBanner`
- Capture logs appear immediately after the step whose `captureAs` produced them, offset by a blank line for visual separation from raw subprocess output
- The completion summary is the last non-blank line — readable as a terminal state even if the panel is scrolled to the bottom

### Terminal Width Detection

`ui.TerminalWidth()` wraps `unix.IoctlGetWinsize(stdout, TIOCGWINSZ)` to return the current terminal column count. Non-TTY or ioctl failure returns `ui.DefaultTerminalWidth` (80). `main.go` computes `LogWidth` once at startup from `ui.TerminalWidth() - 2` (subtracting the rounded-border glyphs) and passes it through `workflow.RunConfig.LogWidth`. `workflow.Run` re-derives the effective width each call, falling back to `DefaultTerminalWidth` when `LogWidth <= 0` so test doubles without a TTY still produce deterministic banners.

## First-Frame Pre-Population

Before `program.Run()` is called, `main()` pre-populates the header so the first rendered frame shows real content instead of empty slots. A `stepNames()` helper (in `main.go`) extracts `Step.Name` from a step slice:

```go
func stepNames(ss []steps.Step) []string {
    names := make([]string, len(ss))
    for i, s := range ss {
        names[i] = s.Name
    }
    return names
}
```

The pre-population block runs immediately after `NewStatusHeader`:

```go
if len(stepFile.Initialize) > 0 {
    header.SetPhaseSteps(stepNames(stepFile.Initialize))
    header.SetStepState(0, ui.StepActive)
    header.IterationLine = "Initializing 1/" + strconv.Itoa(len(stepFile.Initialize)) + ": " + stepFile.Initialize[0].Name
} else {
    header.SetPhaseSteps(stepNames(stepFile.Iteration))
    header.SetStepState(0, ui.StepActive)
    if cfg.Iterations > 0 {
        header.IterationLine = "Iteration 1/" + strconv.Itoa(cfg.Iterations)
    } else {
        header.IterationLine = "Iteration 1"
    }
}
```

- If an initialize phase exists, the header is set to that phase with the first step marked active and `IterationLine` showing `Initializing 1/N: <step name>`
- Otherwise the header starts on the iteration phase with `Iteration 1/M` (bounded) or `Iteration 1` (unbounded)
- The workflow goroutine then sends header messages via `headerProxy` as it progresses, which the Update goroutine applies to the `StatusHeader`

## Testing

- `ralph-tui/internal/ui/header_test.go` — Tests for NewStatusHeader (row count computation, negative input), RenderInitializeLine/RenderIterationLine/RenderFinalizeLine (bounded and unbounded modes, with/without issueID, substitute template correctness), SetPhaseSteps (short/long phases, phase transition clearing, overflow panic, input immutability), SetStepState (state updates, failed steps, skipped steps, out-of-bounds no-op, grid arithmetic for multi-row layouts)
- `ralph-tui/internal/ui/log_test.go` — Tests for every log-body helper: StepSeparator / RetryStepSeparator formatting, StepStartBanner (ASCII/empty/Unicode rune-count assertions), PhaseBanner (width matching, clamp on non-positive width, `═` fill), CaptureLog (simple/empty/multi-line-escaped/embedded-quotes), CompletionSummary (format exactness)
- `ralph-tui/internal/ui/model_test.go` — Smoke test for `View()` (non-empty output, contains version label, contains step name), panic-safety test (zero-dimension WindowSizeMsg), header message routing, title assembly, renderTopBorder edge cases, viewport clamping, checkbox grid even-spacing (`TestView_CheckboxGrid_EqualCellWidth` plus TP-001 through TP-005: multi-row global max, empty trailing cells, equal-width no-op padding, truncation interaction, single-step minimum), `colorShortcutLine` plain-text preservation for `NormalShortcuts` and `ErrorShortcuts`, `QuitConfirmPrompt` AppTitle embedding, `QuittingLine` pass-through, D23 heartbeat integration tests via `WithHeartbeat` delegation: `TestModel_Init_ReturnsNil` (Init always returns nil), `TestModel_HeartbeatTick_ReturnsNilCmd` (Update returns nil cmd — ticker owned by main.go), suffix shows at ≥15s, no suffix when inactive, no suffix below threshold, suffix cleared on transition to inactive, exact 15s boundary shows suffix, suffix suppressed when iteration line is empty, fractional seconds truncated to whole seconds
- `ralph-tui/internal/ui/header_test.go` — D23 heartbeat unit tests on `StatusHeader` directly: `TestStatusHeader_HandleHeartbeatTick_NilReader` (clears suffix, no-op), `TestStatusHeader_HandleHeartbeatTick_ShowsSuffix` (≥15s silence → suffix set), `TestStatusHeader_HandleHeartbeatTick_NoSuffix_BelowThreshold` (<15s → no suffix), `TestStatusHeader_HandleHeartbeatTick_ClearsSuffix_Inactive` (inactive → suffix cleared), `TestStatusHeader_HandleHeartbeatTick_CallsReader` (reader call count incremented), `TestStatusHeader_SetHeartbeatReader_ExplicitNil_Disables` (nil after non-nil → suffix cleared on next tick)
- `ralph-tui/internal/ui/header_proxy_test.go` — Tests for each `HeaderProxy` method (correct message type and fields)
- `ralph-tui/internal/ui/selection_test.go` — 39 tests across 8 categories: `normalized()` edge cases (same-position, rawIdx ordering priority), `extractText` edge cases (three-or-more lines, full raw line, empty middle line, boundary offsets), `extractText` bounds guards (negative rawIdx, out-of-range rawIdx/rawOffset, reversed same-line offsets — 7 guard paths), `mouseToViewport` edge cases (exact top/bottom boundary, negative col no-panic), `visualColToRawOffset` edge cases (col=0 returns segmentStart, empty rawLine, segmentStart at end, multi-byte grapheme "é"), `contains()` edge cases (empty selection, reversed anchor/cursor, single-row col=0 included), `visible()` edge cases (active+committed simultaneously, committed reversed anchor/cursor), input immutability (lines slice not mutated after `extractText`), zero-value struct safety (`pos{}` and `selection{}` don't panic in any function)
- `ralph-tui/internal/ui/log_panel_selection_test.go` — 17 tests across 4 categories: `renderContent` overlay correctness (empty selection cursor cell, single-row range, multi-row range, no-selection fast-path, committed selection), split helper unit tests (`splitAtCol` start/past-end/empty, `splitAtCols` empty-range/full-row, `colToByteOffset` multi-byte), `initSelectionAtLastVisibleRow` edge cases (empty visual lines, fewer lines than viewport, rawIdx/rawOffset values), `SetSelection`/`ClearSelection`/`SelectedText` accessor correctness
- `ralph-tui/internal/ui/keys_select_movement_test.go` — 16 tests covering all cursor movement acceptance criteria: h/j/k/l single-cell move, anchor stays fixed, arrow keys move cursor, 0/Home → line start, $/End → line end, K/J/Shift+↑↓ extend by one row, PgUp/PgDn by viewport.Height-1, virtual column preserved across shorter lines, viewport autoscrolls to keep cursor visible, cursor clamps to line end on narrow rows, q from ModeSelect enters QuitConfirm with pre-Select `prevMode` preserved, q clears selection before entering QuitConfirm, SelectedText updated after cursor moves
- `ralph-tui/internal/ui/log_panel_ring_eviction_test.go` — 6 tests: rawIdx decremented after eviction (SelectedText unchanged), anchor underflow clears selection, cursor underflow clears whole selection, auto-scroll suppressed during visible selection, auto-scroll re-arms after selection cleared, stale visualRow corrected by post-eviction recompute
- `ralph-tui/internal/ui/log_panel_eviction_recompute_test.go` — 20 edge-case tests across 7 categories: `recomputeSelectionVisualCoords` (no-op on zero-value, invalid rawIdx returns zero, preserves active/committed flags), `findVisualPos` (empty visualLines, last matching segment for wrapped line, col recomputed from rawOffset, col skip when rawOffset past end), eviction adjustment (inactive no-op, boundary rawIdx==0 survives, successive evictions cumulative), auto-scroll suppression (active selection suppresses, empty committed does not, not-at-bottom blocks independently), eviction + recompute interaction (stale visualRow corrected, underflow clears before recompute), SetSize interaction (does not recompute; subsequent LogLinesMsg corrects), model.go integration (LogLinesMsg preserves selection in ModeSelect, external SetMode clears selection on next Update)
- `ralph-tui/internal/ui/clipboard_copy_test.go` — 9 tests for the clipboard copy action: y/Enter copies and exits ModeSelect, OSC 52 fallback when clipboard daemon unavailable, `[copied N chars]` feedback on success, `[copy failed: ...]` on failure, empty selection is a silent no-op, clipboard payload uses raw coordinates (no wrap-induced newlines), `github.com/atotto/clipboard` is a direct dep, copyFn/stderrWriter test-seam isolation, resetClipboardFns restores defaults
- `ralph-tui/internal/ui/clipboard_additional_test.go` — 21 additional tests across 7 categories: `copyToClipboard` unit tests (empty string, multi-byte UTF-8 OSC 52 encoding, no stderr leak on success, large payload >64 KB), `copySelectedText` helper isolated (empty returns nil cmd, success produces `[copied N chars]` LogLinesMsg, failure produces error LogLinesMsg), model.go routing (multi-line selection payload correct, double-y second is no-op, Enter from Done restores Done, single-char selection feedback), `handleSelect` separation of concerns (y does not call copyFn directly, Enter restores prevMode not hardcoded Normal, shortcut footer updates on mode exit), test seam safety (resetClipboardFns, stderrWriter restore, no-parallel audit), LogLinesMsg integration (copied and copy-failed lines appear in viewport, byte-vs-rune count documented and verified), go mod tidy audit (no diff)
- `ralph-tui/internal/ui/mouse_selection_test.go` — 16 integration tests covering all mouse selection acceptance criteria: left-drag selects text with live reverse-video feedback, release commits and shows `SelectCommittedShortcuts`, dragging past top/bottom edge auto-scrolls one line per motion, bare click re-anchors selection, shift-click extends committed selection's cursor, left-press in Error/QuitConfirm/NextConfirm/Quitting is a no-op, wheel scrolls in every mode, mid-drag resize force-commits without losing raw coords, `y` copies committed selection and exits ModeSelect
- `ralph-tui/internal/ui/mouse_selection_extra_test.go` — 24 additional tests across 7 categories: `resolveVisualPos` (negative row false, row past end false, col clamped to row width, col clamped to 0 for negative), `HandleMouse` unit tests (empty visualLines no-op, motion guard `!active`, release guard `!active`, shift-press on no committed selection clears-and-re-anchors, negative row clamped to 0), model.go mouse routing (press above viewport `ok=false` no-op, press below content no-op, stray motion no-op when not active, stray release no-op), `selectJustReleased` lifecycle (cleared by wheel event, cleared by second press, NOT set by keyboard `v`), auto-scroll clamping (multi-event scrolls accumulate, stops at top boundary, stops at bottom boundary), shift-click edge cases (same cell as cursor is no-op, click before anchor moves cursor left, click during active drag ignored), `SelectCommittedShortcuts` constant and `updateShortcutLineLocked` path

## Additional Information

- [Architecture Overview](../architecture.md) — System-level view showing how the header fits into the TUI
- [Workflow Orchestration](workflow-orchestration.md) — How step state transitions are triggered during orchestration
- [Keyboard Input & Error Recovery](keyboard-input.md) — How the shortcut bar text changes with keyboard modes
- [Subprocess Execution & Streaming](subprocess-execution.md) — How WriteToLog injects separator lines into the log stream
- [Step Definitions & Prompt Building](step-definitions.md) — Where step names displayed in the header originate
- [API Design](../coding-standards/api-design.md) — Coding standards for bounds guards on array indexers (used by SetStepState)
