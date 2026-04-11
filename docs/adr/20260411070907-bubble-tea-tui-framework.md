# Use Bubble Tea for the TUI Framework

- **Status:** accepted
- **Date Created:** 2026-04-11 07:09
- **Last Updated:** 2026-04-11 07:09
- **Authors:**
  - River Bailey (mxriverlynn, river.bailey@testdouble.com)
- **Reviewers:**

## Context

The ralph-tui Go orchestrator currently renders its TUI with [Glyph](https://useglyph.sh/) (`github.com/kungfusheep/glyph`). Glyph gives us a pointer-binding widget model where mutating a `StatusHeader` field triggers a repaint, a `glyph.Log` widget backed by an `io.Pipe` for streaming subprocess output, and vim-style keyboard navigation. See `ralph-tui/cmd/ralph-tui/main.go:86-186` and `ralph-tui/internal/ui/header.go:15-88` for the current integration.

Two user-visible features are now required that Glyph does not support:

1. **Dynamic title.** Today the VBox border carries a hard-coded `.Title("Ralph")` (`main.go:136`). We want the title to reflect the live state of the run (e.g., iteration counter, active phase, or the repo we're targeting). Glyph has no API for setting the terminal's OS-level window title and no way to reactively rebind the border title string.
2. **Mouse-wheel scrolling.** The log panel is `glyph.Log(...).MaxLines(500).BindVimNav()` (`main.go:124`), which supports keyboard-only scrollback (`k`/`j`). Users running in terminals like iTerm2, Ghostty, and modern tmux expect mouse-wheel scroll. Glyph does not emit mouse events.

Rather than fork Glyph or build these features from scratch on top of it, we are migrating to a TUI library that supports both natively and has the ecosystem momentum to keep pace with future needs.

## Decision Drivers

- **Dynamic terminal/window title** must be a first-class capability, reactive to `Model` state changes
- **Mouse-wheel scrolling** on the log panel must work out of the box
- **Streaming subprocess output** must still flow in real time from two `io.Pipe` readers into a scrollable log view without losing the existing append-latency characteristics (`internal/workflow/workflow.go:42-176`)
- **Preserve the three-color chrome** established in commit `5bdb2f0`: light-gray frame (`PaletteColor(245)`), bright-white active step, bright-green active marker — per `docs/features/tui-display.md` and `ralph-tui/internal/ui/header.go:15-28`
- **Preserve the four-mode keyboard state machine** (Normal / Error / QuitConfirm / Quitting) from `docs/features/keyboard-input.md` and `internal/ui/ui.go:36-181`
- **Long-term ecosystem stability** — consistent with the reasoning in [Cobra CLI ADR](./20260409135303-cobra-cli-framework.md), we prefer the library with the largest community and most active maintenance
- **Architectural fit** with our pointer-mutable `StatusHeader` model is desirable but not strictly required — a principled rewrite is acceptable

## Considered Options

1. **charmbracelet/bubbletea + lipgloss + bubbles/viewport** — Elm-architecture TUI framework from Charm. ~30k stars, most popular Go TUI. `tea.SetWindowTitle(string)` command emits the OSC escape sequence for OS window titles. `bubbles/viewport` is a drop-in scrollable panel with native mouse-wheel support when the program is started with `tea.WithMouseCellMotion()`. Lip Gloss provides foreground/background colors, borders, and layout that directly cover our current `glyph.Color` + border + HBox/VBox usage. Active Charm ecosystem includes `bubbles/spinner`, `bubbles/progress`, and `glamour` we could adopt later.

   - Pros: Native support for both required features (`SetWindowTitle`, mouse wheel via `viewport`); largest Go TUI community and most active maintenance; Lip Gloss style tree cleanly expresses the three-color chrome hierarchy; streaming subprocess output maps naturally to a custom `tea.Msg` type consumed in `Update`; well-documented with many production projects to reference (gh-dash, glow, soft-serve, gum, wishlist, lazygit alternatives)
   - Cons: Requires a rewrite of the pointer-binding render loop into an Elm-style `Model`/`Update`/`View` — Glyph's "mutate a field and Glyph repaints" model does not translate directly; the `bubbles/viewport` component holds its own string buffer, so our current `io.Pipe`-to-widget path must become a goroutine that forwards lines as `tea.Msg` values via `tea.Program.Send`

2. **rivo/tview + tcell** — Widget-based framework on top of tcell. ~11k stars. Production usage includes k9s. Primitives provide `Box.SetTitle(string)` per-primitive, and `TextView` supports mouse scroll when mouse is enabled via `Application.EnableMouse(true)`. OS-level window title requires emitting escape sequences directly through tcell.

   - Pros: Mature, stable, rich built-in widget set (Flex, Grid, Modal, Table) that would shorten some layout code; mouse support is available; widely used in production for complex layouts
   - Cons: No first-class API for the OS terminal window title — dynamic title would need a hand-rolled OSC emitter; community momentum has shifted toward Charm, limiting long-term leverage for the ecosystem goals in the Cobra ADR; widget-tree model is further from our current state-plus-repaint mental model than Bubble Tea's `Update`/`View`

3. **gdamore/tcell (direct)** — The terminal cell library underneath tview. ~5k stars. Full control over every byte, including OSC sequences for window titles and raw mouse events.

   - Pros: Maximum flexibility; zero abstractions to fight; underlies both tview and (historically) Bubble Tea's rendering
   - Cons: We would be re-implementing layout, scrollback, focus, and input dispatch from scratch — effectively building our own Glyph replacement in-house; no component ecosystem; not aligned with the "prefer the established standard" thread of the Cobra ADR

4. **awesome-gocui/gocui** — View-based minimalist framework, fork of the now-inactive `jroimartin/gocui`. Used by lazygit. Supports mouse including scroll; view titles are runtime-mutable.

   - Pros: Lightweight; proven in lazygit; mouse scroll available; runtime-mutable view titles
   - Cons: Much smaller community than Bubble Tea or tview; original upstream is unmaintained and the community fork carries reduced momentum; no equivalent to Lip Gloss for styling — we would still be building our own color/layout primitives

5. **Stay on Glyph and build the missing features** — Extend Glyph (or fork it) to add a dynamic title API and mouse event handling.

   - Pros: No migration work; preserves the pointer-binding render loop we already depend on
   - Cons: Blocks both required features on upstream changes we do not control; Glyph has a single-digit star count and no meaningful community; every future feature request carries the same risk; diverges from the "prefer the established standard" thread of the Cobra ADR

## Decision

We will migrate ralph-tui to **Bubble Tea + Lip Gloss + bubbles/viewport**. It is the only option that delivers both required features out of the box and also satisfies the long-term ecosystem-stability driver we applied in the Cobra decision. The required rewrite of the pointer-binding render loop into an Elm-style `Model`/`Update`/`View` is a real cost, but it is a one-time cost against a component set (`viewport`, `spinner`, `progress`, `textinput`) that will make subsequent UI work cheaper.

We specifically reject tview because its lack of first-class window-title support would force a hand-rolled OSC emitter, and we reject gocui and bare tcell because neither has the community leverage we adopted as a decision driver in [Cobra](./20260409135303-cobra-cli-framework.md).

## Consequences

**Positive:**

- `tea.SetWindowTitle(...)` returned from `Update` gives us a dynamic OS window title reactive to iteration/phase state
- `bubbles/viewport` with `tea.WithMouseCellMotion()` gives us mouse-wheel scroll without custom code, and preserves the existing vim-nav bindings (`k`/`j`) through its own `KeyMap`
- Lip Gloss styles cleanly express the three-color chrome hierarchy (light-gray frame, bright-white active step, bright-green active marker), and the per-cell HBox composition in `header.go` translates to Lip Gloss `JoinHorizontal`
- Much larger ecosystem of bubbles components (`spinner`, `progress`, `textinput`, `table`) available for future UI work
- Aligns with the Cobra-ADR precedent of preferring the established Go ecosystem standard

**Negative:**

- The pointer-binding `StatusHeader` pattern must be rewritten as Elm-style state returned from `Update`; the mutation points in `internal/ui/header.go` become pure functions returning a new header state (or methods on the Bubble Tea root Model)
- The `io.Pipe` + `glyph.Log` wiring in `internal/workflow/workflow.go:42-176` must become a goroutine that forwards each scanned line as a custom `tea.Msg` via `tea.Program.Send`, with the Model appending the line into a `viewport.Model` buffer
- Adds new direct dependencies: `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles`
- Keyboard dispatch rewrite: the current `app.Handle(key, fn)` callbacks in `main.go:89-92` must move into `Update(msg tea.Msg) (Model, tea.Cmd)` as a switch on `tea.KeyMsg`; the four-mode state machine in `ui.go` stays the same in spirit but the dispatch surface changes

**Neutral:**

- The four-mode keyboard state machine (`ModeNormal`, `ModeError`, `ModeQuitConfirm`, `ModeQuitting`) carries over unchanged as data; only its dispatch plumbing changes
- The 500-line log cap and auto-scroll-on-tail behavior remain, but are enforced in our Model rather than a widget flag
- File logging in `internal/workflow/workflow.go` is independent of the TUI library and is not affected by this change

## Notes

### Key Files

| File | Purpose |
|------|---------|
| `ralph-tui/cmd/ralph-tui/main.go` | Program entry point — currently constructs the Glyph `App`, wires keybindings, and calls `app.Run()` (lines 86-186). Becomes the `tea.NewProgram(model, tea.WithMouseCellMotion(), tea.WithAltScreen())` call site. |
| `ralph-tui/internal/ui/header.go` | Pointer-mutable `StatusHeader` (lines 61-88). Becomes an immutable header struct with pure state transitions, rendered via Lip Gloss. |
| `ralph-tui/internal/ui/ui.go` | Four-mode keyboard state machine `KeyHandler` (lines 36-181). The `Mode` enum and transition rules stay; dispatch moves from `app.Handle` callbacks to `Update(msg)` on `tea.KeyMsg`. |
| `ralph-tui/internal/ui/orchestrate.go` | Step sequencer that sends `StepAction` over the `Actions` channel. The channel contract stays; only its producer side (keyboard) changes. |
| `ralph-tui/internal/workflow/workflow.go` | Subprocess runner with `io.Pipe`-based streaming (lines 20-49, 97-176). Replace `LogReader()` with a line-forwarding goroutine that calls `tea.Program.Send(logLineMsg{line})`. |
| `ralph-tui/go.mod` | Remove `github.com/kungfusheep/glyph`; add `bubbletea`, `lipgloss`, `bubbles`. |

### Cross-References

- See [Cobra CLI Framework](./20260409135303-cobra-cli-framework.md) — establishes the "prefer the established standard with the largest Go ecosystem" decision pattern we are following here
- See [Narrow Reading Principle](./20260410170952-narrow-reading-principle.md) — the migration is scoped to the UI layer only; no Ralph-specific knowledge should leak into the Bubble Tea `Model` beyond what Glyph already sees today

### Related Docs

- [TUI Display Feature Doc](../features/tui-display.md) — documents the `StatusHeader` regions, the 500-line log cap, and the checkbox-based step progress that must be preserved
- [Keyboard Input Feature Doc](../features/keyboard-input.md) — documents the four-mode state machine that must be preserved
- [Reading the TUI](../how-to/reading-the-tui.md) — documents the four user-visible regions (checkbox grid, iteration line, log panel, shortcut footer) that must be preserved
- [Architecture Overview](../architecture.md) — system-level architecture including the TUI rendering path
- [Bubble Tea Migration Plan](../plans/use-bubble-tea.md) — the execution plan for this migration (to be written)
