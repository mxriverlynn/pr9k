# internal/uichrome

Shared chrome primitives consumed by both the run-mode TUI (`internal/ui`) and the workflow-builder TUI (`internal/workflowedit`). Centralising border helpers, color palette, overlay utilities, and geometry constants here avoids an import cycle: `workflowedit` cannot import `internal/ui` (which imports bubbletea program-level types), and `internal/ui` should not know about workflow-builder details. `uichrome` is the neutral meeting point (D-2).

- **Last Updated:** 2026-04-27
- **Authors:**
  - River Bailey

## Exported API

### Border helpers

```go
func WrapLine(content string, innerWidth int) string
func HRuleLine(innerWidth int) string
func BottomBorder(innerWidth int) string
func RenderTopBorder(title string, width int) string
```

**WrapLine** wraps a content string in side-border `│` characters, truncating to `innerWidth` and right-padding with spaces so the right border aligns vertically across all rows.

**HRuleLine** returns a `├─┤` T-junction rule of the specified inner width.

**BottomBorder** returns a `╰─╯` bottom border of the specified inner width.

**RenderTopBorder** constructs the top border row with an embedded title: `╭── Title text ──────╮`. The title segment is colored via `ColorTitle` and the fill is sized to `width` total columns (including corner glyphs).

### Overlay

```go
func Overlay(base, overlay string, top, left int) string
func SpliceAt(base, overlay string, row, col int) string
```

**Overlay** splices a multi-line overlay string onto a multi-line base frame at the given top/left offset. Used by `workflowedit.View()` to splice dropdown menus, dialogs, and help modals over the persistent chrome frame. Handles ANSI escape sequences and wide runes correctly via `charmbracelet/x/ansi`.

**SpliceAt** splices a single row of the overlay at the specified frame row and column.

### Shortcut and title styling

```go
func ColorShortcutLine(line string) string
func ColorTitle(plain string) string
```

**ColorShortcutLine** applies two-tone shortcut-bar coloring: key labels in `White`, descriptions in `LightGray`. Accepts the raw shortcut line and returns the ANSI-colored version.

**ColorTitle** splits a title at the ` — ` separator and colors the left segment `Green` and the right segment `White`. Used by `RenderTopBorder` for the workflow-builder title.

### Color palette

```go
var LightGray  = lipgloss.Color("245") // chrome: brackets, borders, shortcut bar
var White      = lipgloss.Color("15")  // content area, key labels
var Green      = lipgloss.Color("10")  // app name in top border, reorder gripper
var Red        = lipgloss.Color("9")   // fatal findings, read-only banner
var Yellow     = lipgloss.Color("11")  // warn findings, external/symlink/shared banners
var Cyan       = lipgloss.Color("14")  // info findings, unknown-field banner
var Dim        = lipgloss.Color("8")   // placeholders, dimmed-when-overlaid content
var ActiveStepFG   = lipgloss.Color("15") // active step name in run-mode TUI
var ActiveMarkerFG = lipgloss.Color("10") // active step marker (▸) in run-mode TUI
```

### Geometry constants

```go
const MinTerminalWidth  = 60  // D48 minimum terminal width
const MinTerminalHeight = 16  // D48 minimum terminal height
const DialogMaxWidth    = 72  // maximum inner width for dialog overlays (D36)
const DialogMinWidth    = 30  // minimum inner width for dialog overlays (D36)
const HelpModalMaxWidth = 72  // maximum inner width for the help modal overlay
```

`MinTerminalWidth` and `MinTerminalHeight` are the minimum terminal dimensions the workflow-builder TUI is designed for. When the terminal is smaller, `workflowedit.View()` returns the D48 fallback message: `"Terminal too small — resize to at least 60×16"`.

## Design notes

### Why a shared package instead of copying constants

Both `internal/ui` (run-mode TUI) and `internal/workflowedit` (workflow-builder TUI) need the same palette colors, border characters, and minimum-size constants. Duplicating them creates drift risk; sharing them via a common package keeps the two TUIs visually consistent at zero duplication cost.

### No import of bubbletea or lipgloss model types

`uichrome` imports only `lipgloss` (for color and style types) and `charmbracelet/x/ansi` (for ANSI-safe string width computation in `Overlay`). It does NOT import `bubbletea` or any TUI model types. This keeps the package import-cycle safe: any package in the module can import `uichrome` without pulling in TUI framework dependencies.

## Dependencies

```
internal/uichrome
    ├── github.com/charmbracelet/lipgloss   (color and style types)
    └── github.com/charmbracelet/x/ansi     (ANSI-safe string width for Overlay)
```

## Testing

Tests are in `src/internal/uichrome/uichrome_test.go`. Coverage includes:

- `WrapLine` truncation and padding at various inner widths
- `RenderTopBorder` title embedding and truncation
- `Overlay` correct splice position for single-row and multi-row overlays
- `ColorTitle` split at ` — ` and plain-title fallback

## Related Documentation

- [`docs/features/workflow-builder.md`](../features/workflow-builder.md) — user-facing workflow-builder TUI behavior
- [`docs/code-packages/workflowedit.md`](workflowedit.md) — `internal/workflowedit` Visual Layout and render decomposition
