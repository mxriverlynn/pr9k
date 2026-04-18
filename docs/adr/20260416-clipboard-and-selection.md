# ADR: Clipboard and In-TUI Text Selection

- **Date:** 2026-04-16
- **Status:** Accepted
- **Pairs with:** [`20260413160000-require-docker-sandbox.md`](20260413160000-require-docker-sandbox.md) — both document external runtime dependencies that users may need to install.

## Context

pr9k runs in alt-screen mode with `tea.WithAltScreen()` and `tea.WithMouseCellMotion()`. This combination gives the TUI full mouse event delivery including cell-motion drag events, but it defeats the terminal's native text-selection mechanism — the OS never sees the mouse events and therefore cannot build a drag selection. Users who want to extract log text (for filing bug reports, pasting into prompts, etc.) have no built-in way to do so unless the TUI provides its own selection layer.

## Decisions

### (a) Primary clipboard: `github.com/atotto/clipboard`

**Decision:** Use `github.com/atotto/clipboard v0.1.4` as the primary clipboard write path rather than a pure OSC 52 implementation.

**Rationale:** `atotto/clipboard` provides a single `WriteAll(text)` API that dispatches to the correct platform backend:

| Platform | Backend |
|----------|---------|
| macOS | `pbcopy` |
| Linux desktop | `xclip` (preferred) or `xsel` (fallback) |
| Windows | Native Win32 clipboard API via `golang.org/x/sys/windows` |

A pure OSC 52 path would only work in terminal emulators that support it; `atotto/clipboard` covers the common local desktop case without requiring any special terminal support. OSC 52 is retained as a fallback (see decision (d)).

**Trade-off:** Linux users must have `xclip` or `xsel` installed. The OSC 52 fallback covers the case where neither is available.

### (b) Linux runtime dependency: `xclip` or `xsel`

**Decision:** Document `xclip`/`xsel` as a soft runtime dependency on Linux. Missing tools fall back to OSC 52; if OSC 52 is also unavailable, the TUI shows `[copy failed: install xclip/xsel or run in a terminal that supports OSC 52]` in the log.

**Rationale:** Making the Docker sandbox image (`docker/sandbox-templates:claude-code`) the sole runtime environment was already decided in `20260413160000-require-docker-sandbox.md`. However, pr9k itself runs *outside* the sandbox (as the orchestrator process), so clipboard access depends on the host environment. We cannot unconditionally require `xclip`/`xsel` in the sandbox image because the sandbox is only used for claude steps, not for the TUI process itself.

**How to apply:** When changing clipboard code or the sandbox image, check whether the host environment (not the container) has the required clipboard tool. The `copyToClipboard` function in `clipboard.go` already handles the three-level fallback; do not bypass it.

### (c) In-TUI selection layer required

**Decision:** Implement a custom selection layer inside pr9k (`ModeSelect`, `pos`/`selection` types, `renderContent` overlay) rather than relying on the terminal's native selection.

**Rationale:** `tea.WithAltScreen() + tea.WithMouseCellMotion()` together defeat native terminal text selection. The TUI receives all mouse events exclusively; the OS clipboard integration never fires. A custom selection layer is the only way to provide copy functionality while alt-screen and mouse cell-motion are both active. The selection layer also integrates with word-wrap correctly: copied text uses raw line coordinates, so wrap-induced visual segments never inject artificial newlines into the copy payload.

**How to apply:** Do not remove `tea.WithMouseCellMotion()` or revert to a passive mouse mode to "restore" native selection — that would break drag-to-select and mouse-wheel. If a future change wants to support native selection as an option, it must also disable `WithMouseCellMotion()` for that mode, which removes all other mouse gestures.

### (d) OSC 52 fallback ships with the primary path, not deferred

**Decision:** The OSC 52 stderr fallback (`\x1b]52;c;<base64-payload>\x07`) ships in the same release as the primary clipboard path, not as a follow-up.

**Rationale:** Ralph's core deployment scenario is SSH-to-cloud-VM, where the developer runs the orchestrator on a remote host. In that environment `xclip`/`xsel` are almost never installed and the primary clipboard path fails on every copy. Without OSC 52, the feature is effectively unusable in the deployment where it matters most. Deferring OSC 52 would ship a broken experience to the primary target audience.

**Terminal support:** iTerm2, Kitty, WezTerm, Windows Terminal, and recent xterm all support OSC 52 clipboard writes. tmux users must also set `set -g set-clipboard on` in their `.tmux.conf`.

**How to apply:** The OSC 52 path is implemented in `clipboard.go` alongside `copyToClipboard`. Keep both paths together; the three-level delivery order (`clipboard.WriteAll` → OSC 52 stderr → failure log line) must not be changed without updating this ADR.

## Consequences

- `github.com/atotto/clipboard v0.1.4` is a direct dependency in `src/go.mod`.
- `golang.org/x/term v0.42.0` is a direct dependency (used for `term.IsTerminal(stderr)` to gate the OSC 52 path).
- Linux users on local desktops need `xclip` or `xsel`; this is documented in `docs/how-to/copying-log-text.md`.
- The `v` key binding and `ModeSelect` extend pr9k's public keyboard surface; the minor version was bumped to `0.5.0` per `docs/coding-standards/versioning.md`.
