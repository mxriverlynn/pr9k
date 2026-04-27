# TUI Rendering

## Assemble plain text first, apply styling once

When building a styled terminal string from multiple parts, assemble the complete plain-text content first and apply ANSI styling in a single pass. Never pass an already-styled string through a second styling function — this creates double-escape sequences and makes visual width calculations unreliable (escaped bytes are counted as columns).

```go
// Bad — ColorShortcutLine re-processes an already-styled string
result := uichrome.ColorShortcutLine("? help")  // styled
result += "  ·  " + uichrome.Dim.Render("hint") // ok
result = uichrome.ColorShortcutLine(result)      // re-processes styled string — corrupted output

// Good — build plain text, apply two-tone once, then append pre-styled additions
plain := "? help"
result := uichrome.ColorShortcutLine(plain)            // single styling pass
result += "  ·  " + lipgloss.NewStyle()                // separately-styled append, after two-tone
                       .Foreground(uichrome.Dim).
                       Render("save  [ro]")
```

The two-tone shortcut line follows this ordering strictly:

1. Build the entire plain-text shortcut string (`"? help  ·  Ctrl+S save"`).
2. Append the help hint to the plain-text base **before** calling `ColorShortcutLine`, so the parser can apply white-to-keys / gray-to-descriptions in one pass.
3. Call `ColorShortcutLine` once on the assembled plain text.
4. Append any pre-styled additions (dim read-only hint, etc.) **after** `ColorShortcutLine`. These carry their own ANSI state and must not be re-processed.

See `render_footer.go: ShortcutLine()` as the canonical example.

## Never use len() for visual width of styled strings

`len()` counts bytes, not terminal columns. A Lip Gloss-styled string contains ANSI escape sequences whose byte length has no relation to the number of visible columns they occupy. Using `len()` for padding, alignment, or overflow detection on styled strings produces silently wrong layout — tests may pass while production renders are misaligned.

```go
label := lipgloss.NewStyle().Foreground(uichrome.Dim).Render("Save  [ro]")

// Bad — len() counts escape bytes; result is larger than visual width
pad := targetWidth - len(label)

// Bad — rune count is also wrong for ANSI-containing strings
pad := targetWidth - len([]rune(label))

// Good — visual column count; escape sequences are excluded
pad := targetWidth - lipgloss.Width(label)
```

Apply `lipgloss.Width()` anywhere a styled string's column count matters:
- Padding and alignment in render functions
- Overflow guards (detecting a rendered line is too wide for the terminal)
- Width assertions in tests

## Use a generation counter to reject stale async banner messages

When a transient UI state (save banner, flash effect) is created by an async completion and cleared by a delayed message, use a generation counter to reject stale clear messages from previous operations.

Without a counter, this sequence corrupts the UI: user saves twice; the first clear-timer fires during the second save window and prematurely clears the second banner.

```go
type Model struct {
    saveBanner string
    // bannerGen is incremented on each save so stale clear ticks from prior
    // saves are ignored — a clearSaveBannerMsg only fires if its gen matches.
    bannerGen int
}

// clearSaveBannerMsg carries the generation it was issued for.
type clearSaveBannerMsg struct{ gen int }

// On save success — increment gen and schedule clear at the new gen value.
func (m Model) onSaveSuccess() (Model, tea.Cmd) {
    m.saveBanner = "Saved at " + m.nowFn().Format("15:04:05")
    m.bannerGen++
    gen := m.bannerGen
    return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
        return clearSaveBannerMsg{gen: gen}
    })
}

// In Update — only clear if the gen matches current.
case clearSaveBannerMsg:
    if msg.gen == m.bannerGen {
        m.saveBanner = ""
    }
```

The same pattern applies to any visual flash (e.g., `boundaryFlash`) that uses a sequence number rather than a boolean so stale resets are rejected.

Apply whenever a timer-cleared UI state can be re-triggered before the timer fires.

## Decompose View() into render_<component>.go files

When a Bubble Tea model's `View()` or render logic grows beyond one screen, decompose it into per-component files named `render_<component>.go`. Keep the chrome assembly — the code that computes the row budget and stitches the rows together — in `render_frame.go` (or in `View()` itself for small models).

File naming convention:

| File | Responsibility |
|---|---|
| `render_frame.go` | Chrome assembly; owns the row budget; calls component renderers |
| `render_menu.go` | Menu bar rendering |
| `render_session_header.go` | Session header rendering |
| `render_outline.go` | Outline panel rendering |
| `render_detail.go` | Detail pane rendering |
| `render_dialogs.go` | All dialog shells and bodies |
| `render_footer.go` | Shortcut footer |
| `render_help.go` | Help modal |
| `render_empty.go` | Empty-editor placeholder view |

Helper function naming within each file:
- `render<Component>()` or `renderXxx()` — primary renderer; returns a styled string
- `build<Part>Row()` — assembles a sub-element (e.g., a header slot row)
- `<Component>BodyFor(kind)` — maps a kind constant to body content

Each file is reviewable in isolation; any bug in a visible UI region has one obvious file to look in.

## Additional Information

- `src/internal/workflowedit/render_footer.go` — `ShortcutLine()` as the canonical plain-text-first / single-styling-pass example; shows correct ordering of `ColorShortcutLine` then Dim append (D-34, D-18)
- `src/internal/workflowedit/model.go` — `bannerGen` + `clearSaveBannerMsg` as the canonical generation-counter example (D-7)
- `src/internal/uichrome/chrome.go` — `ColorShortcutLine` caller contract: input must be plain text
- [Go Patterns](go-patterns.md) — `lipgloss.JoinHorizontal` for side-by-side panels; `lipgloss.Width` in two-pass layout
- [Testing](testing.md) — ANSI-stripping in render tests; deriving styling assertions from constants
