# Copying Log Text

pr9k includes built-in text selection so you can copy any visible log output to the clipboard without leaving the TUI. This is particularly useful on SSH sessions where the terminal's native selection mechanism is defeated by application mouse mode.

## Common paths

### Path 1 — Mouse: click-drag → release → `y`

This is the fastest path when your pointer is already on the text you want.

1. **Left-click and drag** over the text in the log panel. The selected region is highlighted in reverse-video as you drag. Dragging past the top or bottom edge of the log panel auto-scrolls one line per event.
2. **Release** the mouse button to commit the selection. The footer switches to `y copy  esc cancel  drag for new selection`.
3. Press **`y`** (or **`Enter`**) to copy the selected text to the clipboard.

A `[copied N chars]` confirmation line appears in the log on success.

### Path 2 — Keyboard, recent line: `v` → `$` → `y`

Use this when you want a single complete line near the bottom of the log.

1. Press **`v`** from Normal or Done mode. The cursor appears at column 0 of the last visible log row.
2. Press **`$`** (or **`End`**) to jump the cursor to the end of the line, selecting the whole line.
3. Press **`y`** (or **`Enter`**) to copy and exit Select mode.

### Path 3 — Keyboard, multiple lines: `v` → `Shift+↓` (or `J`) repeated → `y`

Use this when you want to select a block of consecutive log lines.

1. Press **`v`** from Normal or Done mode. The cursor anchors at the last visible row.
2. Scroll up to the start of the region you want (use `k`/`↑` to move the cursor up, or `PgUp` to jump by a page). The anchor stays fixed; only the cursor moves.
3. Press **`Shift+↓`** (or **`J`**) to extend the selection downward one visual row at a time, or use `j`/`↓` to move the cursor down.
4. Press **`y`** (or **`Enter`**) to copy the selected range and exit Select mode.

## Keyboard reference for Select mode

| Keys | Action |
|------|--------|
| `h` / `←` | Move cursor left one column |
| `l` / `→` | Move cursor right one column |
| `j` / `↓` | Move cursor down one row |
| `k` / `↑` | Move cursor up one row |
| `0` / `Home` | Jump to start of current line |
| `$` / `End` | Jump to end of current line |
| `J` / `Shift+↓` | Extend selection down one row |
| `K` / `Shift+↑` | Extend selection up one row |
| `PgDn` / `PgUp` | Move down / up one page |
| `y` / `Enter` | Copy and exit Select mode |
| `Esc` | Cancel selection, return to prior mode |
| `q` | Enter quit confirmation (selection cleared) |

Vertical movement preserves the intended column (`virtualCol`) across shorter lines — the same behaviour as vim's visual mode. The viewport auto-scrolls to keep the cursor visible.

## Clipboard delivery

pr9k tries three delivery paths in order:

1. **System clipboard daemon** (`pbcopy` on macOS, `xclip`/`xsel` on Linux, native API on Windows). This works in most local desktop sessions and is the default when the tools are present.
2. **OSC 52 escape sequence** (stderr fallback). When the clipboard daemon is unavailable (for example, on a headless Linux VM accessed over SSH), pr9k writes `\x1b]52;c;<base64-payload>\x07` to stderr. Terminal emulators that support OSC 52 — iTerm2, Kitty, WezTerm, Windows Terminal, and tmux with `set -g set-clipboard on` enabled, and recent xterm — can receive this and place the text in the system clipboard on the *local* machine, even though the process is running remotely.
3. **Failure log line**. If stderr is not a terminal (for example, when stdout and stderr are both redirected), a `[copy failed: install xclip/xsel or run in a terminal that supports OSC 52]` line appears in the log.

### What clipboard text contains

The copy payload always uses raw line coordinates, never visual-segment boundaries. This means:

- **Word-wrap does not inject newlines.** If a long line wraps across three visual rows in the log panel, copying any portion of it produces the original text without extra `\n` characters at each wrap boundary.
- **Newlines between selected raw lines are preserved.** Selecting from the middle of one raw line to the middle of another includes the `\n` between them.

### Linux desktop requirement

On Linux, the system clipboard path requires either `xclip` or `xsel` to be installed. Install with your package manager:

```bash
# Debian/Ubuntu
sudo apt install xclip

# Fedora/RHEL
sudo dnf install xclip

# Arch
sudo pacman -S xclip
```

If neither is installed and OSC 52 is also unavailable, the `[copy failed: ...]` feedback line is shown in the log.

### Using terminal native selection as a fallback

If you need to copy text using the terminal's built-in drag-select (for example, to grab content outside the log viewport, or on a terminal that doesn't support OSC 52), hold the modifier that overrides application mouse mode before dragging:

| Platform | Override key |
|----------|-------------|
| macOS | `Option` |
| Linux / Windows | `Shift` |

## Related documentation

- [Reading the TUI](reading-the-tui.md) — Overview of all TUI regions including the log panel and shortcut footer
- [Keyboard Input](../features/keyboard-input.md) — Seven-mode state machine; Select mode entry conditions and all keybindings
- [TUI Display](../features/tui-display.md) — Implementation details: selection data types, reverse-video rendering, clipboard copy flow
