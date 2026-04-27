# Feature Specification: Workflow-Builder TUI Visual Design

A complete visual layout for the `pr9k workflow` builder so it presents itself as a full-screen, frame-bordered terminal application visually consistent with the standard `pr9k` run-mode TUI — title bar, persistent menu bar, session header, two-pane editing area, status footer, and centered dialogs — instead of the placeholder text the current build emits.

This specification covers **what the builder looks like at every visible state**. It does not redefine behavior — the behavioral spec at `../workflow-builder/feature-specification.md` is authoritative for what the builder *does*. This document is authoritative for what the user *sees*.

## Outcome

A workflow author launching `pr9k workflow` sees a full-screen TUI that visually identifies as part of the same product as `pr9k` (same chrome glyphs, same brand title, same status-footer rhythm), navigates a recognizable menu-bar / session-header / outline / detail-pane / footer layout, and sees every mode, dialog, and finding rendered in a consistent, bordered, focus-aware visual style that matches the 28 enumerated TUI modes from the behavioral spec's mode-coverage table. Every overlay (menu dropdown, dialog, help modal, findings panel) renders inside a bordered frame anchored to the appropriate region, with the keyboard-default option visually distinguished.

## Actors and Triggers

- **Actors** — the *workflow author*, identical to the behavioral spec's actor.
- **Triggers** — the user launches `pr9k workflow`. The visual design is in effect for the entire builder session.
- **Preconditions** — terminal capabilities equivalent to those the run-mode TUI requires (alt-screen, 256-color foreground, mouse cell-motion, Unicode box-drawing characters).

## Layout — the persistent frame

The builder's screen is a single full-terminal frame, hand-built row by row using the same chrome glyphs and color palette the run-mode TUI already uses (`╭`, `╮`, `╰`, `╯`, `│`, `─`, `├`, `┤`; `LightGray` 245 for chrome, `White` 15 for active text, `Green` 10 for the brand) ([D1](artifacts/decision-log.md#d1-chrome-and-palette-reuse-from-run-mode-tui)). From top to bottom, every frame in every mode contains these rows in this order ([D2](artifacts/decision-log.md#d2-row-order-of-the-persistent-frame)):

1. **Top border row** with the title `Power-Ralph.9000 — Workflow Builder` (empty-editor) or `Power-Ralph.9000 — Workflow Builder — <target-path>` (loaded). The brand name `Power-Ralph.9000` renders green; the rest renders white. When the target path overflows the available title budget, the path is truncated from the left with a `…` prefix so the filename stays visible ([D3](artifacts/decision-log.md#d3-top-border-title-format-and-truncation-rule)).
2. **Menu bar row** containing the left-aligned `File` label and reserved space for future menus. Closed-menu rendering is `  File`; open-menu rendering reverses the `File` label and draws a bordered dropdown extending downward ([D4](artifacts/decision-log.md#d4-menu-bar-row-content-and-states)).
3. **First horizontal rule** (`├` + `─`-fill + `┤` in `LightGray`) separating the menu bar from the session header.
4. **Session header row** showing the target path, the unsaved-changes indicator, at most one banner from the priority-resolved active set, the `[N more warnings]` affordance when more than one banner is active, and the validator findings summary ([D5](artifacts/decision-log.md#d5-session-header-row-content-and-ordering)).
5. **Second horizontal rule** separating the session header from the pane area.
6. **Pane area** — multiple rows containing two side-by-side panes separated by a vertical `│` rule. Left pane (the *outline*) is fixed at min(40, max(20, ⌊terminalWidth × 0.4⌋)) columns; right pane (the *detail pane*) takes the remaining width. The vertical rule joins the surrounding horizontal rules with `┬` and `┴` glyphs at the intersections so the panes read as a structured grid ([D6](artifacts/decision-log.md#d6-pane-area-vertical-separator-and-junction-glyphs), [D7](artifacts/decision-log.md#d7-outline-pane-width-rule)).
7. **Third horizontal rule** separating the pane area from the status footer.
8. **Status footer row** carrying the keyboard-shortcut line for the currently focused widget (left-aligned) and the `pr9k` version label (right-aligned) ([D8](artifacts/decision-log.md#d8-status-footer-row-content-and-ordering)).
9. **Bottom border row** (`╰` + `─`-fill + `╯` in `LightGray`).

The frame's chrome character count is fixed across modes: 5 fixed-content rows (top border, menu bar, session header, status footer, bottom border) plus 3 horizontal rules (between menu bar and session header, between session header and pane area, between pane area and status footer) = **8 chrome rows total**. The pane area absorbs the remaining `terminalHeight - 8` rows ([D9](artifacts/decision-log.md#d9-chrome-row-budget-and-pane-area-vertical-residual)).

### Minimum supported terminal size

The builder requires at least **60 columns × 16 rows** to render the full frame. Below either dimension, the builder draws a single-line `Terminal too small — minimum 60×16 required` centered on the screen and stops drawing the rest of the frame. Resizing back above the threshold restores the full render on the next `tea.WindowSizeMsg` ([D48](artifacts/decision-log.md#d48-minimum-supported-terminal-size-and-fallback-render)).

## Menu bar — visual states

The menu bar row has three visible states ([D4](artifacts/decision-log.md#d4-menu-bar-row-content-and-states)):

- **Closed** — `  File` left-aligned. The `F` of `File` renders in `White` (mnemonic accent for `Alt+F`); the rest of the label renders in `LightGray`. The remainder of the row is space-padded to the right border. Mouse-hover over the `File` label reverses its background ([D10](artifacts/decision-log.md#d10-menu-bar-mnemonic-accent-and-hover-feedback)).
- **Open** — the `File` label renders with reverse-video background, and a bordered dropdown panel anchored to the `File` label's left edge extends downward over the session header and pane area, drawn in the same chrome glyphs as the outer frame. Each dropdown row contains a left-aligned item label (`New`, `Open`, `Save`, `Quit`) and a right-aligned shortcut label (`Ctrl+N`, `Ctrl+O`, `Ctrl+S`, `Ctrl+Q`); the items are separated by single rows; the highlighted (cursor) item renders in reverse video ([D11](artifacts/decision-log.md#d11-menu-dropdown-rendering-and-overlay-anchoring)).
- **Item greyed** — when an item is unavailable in the current state (notably `Save` when no workflow is loaded, or when in browse-only / read-only mode), the item label renders in `LightGray` and the shortcut label is hidden ([D12](artifacts/decision-log.md#d12-greyed-menu-item-rendering-and-disable-rules)).

## Session header — visual content and state

The session header row carries five distinct content slots, packed left-to-right in this order ([D5](artifacts/decision-log.md#d5-session-header-row-content-and-ordering)):

1. **Target path** — the loaded workflow's path, truncated from the left with a `…` prefix when it overflows. Empty-editor state renders `(no workflow open)` in `LightGray`.
2. **Unsaved-changes indicator** — a `●` glyph in `Green`, immediately following the target path with one space of padding. Rendered only when the in-memory state is dirty ([D13](artifacts/decision-log.md#d13-unsaved-changes-indicator-glyph-and-color)).
3. **Banner slot** — at most one banner from the priority-resolved active set (read-only, external-workflow, symlink, shared-install, unknown-field). Each banner type has a distinct foreground color and prefix glyph ([D14](artifacts/decision-log.md#d14-banner-prefix-glyphs-and-severity-coloring)):
   - read-only — `[ro]` in red (`Color("9")`)
   - external-workflow — `[ext]` in yellow (`Color("11")`)
   - symlink — `[sym]` in yellow (`Color("11")`)
   - shared-install — `[shared]` in yellow (`Color("11")`)
   - unknown-field — `[?fields]` in cyan (`Color("14")`)
   - `Saved at HH:MM:SS` — transient post-save banner in green (`Color("10")`); replaces other banners for ~3 seconds and is then cleared
   - `No changes to save` — transient no-op-save banner in `LightGray`; same 3-second behavior
4. **`[N more warnings]` affordance** — rendered immediately after the banner slot when more than one banner is active. Renders in `White` so it reads as actionable; activating it (Enter when focused, click) opens a banner panel listing all active banners ([D15](artifacts/decision-log.md#d15-multi-banner-affordance-rendering-and-activation)).
5. **Findings summary** — right-aligned at the row's right edge. Format `[!] <F> fatal · <W> warn · <I> info` when fatals are present, `[i] <W> warn · <I> info` when only non-fatal findings are present, absent when all counts are zero. The `[!]` tag renders in **red**; the `[i]` tag renders in **cyan**; the count text renders in **light gray** with `·` separators. Activated by clicking it or pressing the help-modal-equivalent shortcut to open the findings panel ([D16](artifacts/decision-log.md#d16-findings-summary-position-and-format)).

When the session header content does not fit the row width even with truncation, the `[N more warnings]` and findings-summary slots are dropped first, then the banner is truncated with `…`, and finally the target path is truncated more aggressively. The unsaved-changes indicator and the at-most-one-banner slot are last to disappear; the dirty indicator is never dropped while the workflow is dirty ([D17](artifacts/decision-log.md#d17-session-header-overflow-priority)).

## Outline pane — visual structure

The outline is a vertically scrolling list of section headers and their items, with focus and collapse state per row. Six section types appear in this order ([D18](artifacts/decision-log.md#d18-outline-section-order-and-grouping)):

1. `env` — top-level environment passthrough list
2. `containerEnv` — top-level container environment list
3. `statusLine` — single-block section (when present) with `+ Add statusLine block` row when absent
4. `Initialize` phase — ordered step list
5. `Iteration` phase — ordered step list
6. `Finalize` phase — ordered step list

Every section renders one **section-header row** and a body of zero-or-more **item rows** terminated by a **`+ Add` affordance row** for list-typed sections (`env`, `containerEnv`, `Initialize`, `Iteration`, `Finalize`). The `statusLine` section's body is either a single item row (when configured) or the `+ Add statusLine block` row (when absent) ([D19](artifacts/decision-log.md#d19-statusline-section-conditional-body-rule)).

Section-header row format ([D20](artifacts/decision-log.md#d20-section-header-row-format-and-collapse-glyph)):

```
▾ Iteration  (3)
▸ Initialize  (1)         ← collapsed
```

The `▾`/`▸` glyph indicates expanded/collapsed. The trailing `(N)` count remains visible when the section is collapsed, satisfying spec D28's "always-visible item count" commitment.

Step item row format ([D21](artifacts/decision-log.md#d21-step-item-row-format-and-glyphs)):

```
  ⋮⋮ [≡] step-name              sonnet
  ⋮⋮ [$] my-shell-step          —
  ⋮⋮ [≡] another-step           sonnet  [F1]
```

- **`⋮⋮`** — persistent gripper glyph in `LightGray` indicating draggable
- **`[≡]`** — Claude-step kind indicator
- **`[$]`** — shell-step kind indicator
- **step name** — left-aligned, `White` when focused or in `LightGray` when not
- **model column** — right-aligned model name for Claude steps, `—` for shell steps
- **`[F<n>]`** — fatal-finding indicator, where `n` is the count of fatals attached to this step. Renders in red

Cursor position ([D22](artifacts/decision-log.md#d22-cursor-row-rendering-in-outline)):

- The focused row is prefixed with `> ` in place of the leading two spaces, with the entire row text rendered in `White` instead of `LightGray`. **No reverse-video on the row.** This matches the run-mode header's "active step" treatment and reserves reverse-video for two distinct states only: reorder mode and the highlighted item inside an open dropdown.

### Step name truncation

When a step's name is wider than the available space between the kind glyph and the right-aligned model column, the name is right-truncated with a `…` suffix. The model column is preserved unless the available space falls below 12 columns, in which case the model column is replaced by a `…` glyph and the entire row width is given to the name ([D49](artifacts/decision-log.md#d49-step-name-truncation-rule)).

`+ Add` affordance row format ([D23](artifacts/decision-log.md#d23-add-affordance-row-format)):

```
  + Add step
  + Add env variable
  + Add container env entry
  + Add statusLine block
```

When focused, the row is prefixed with `> ` and the label renders in `White`.

Empty-section state ([D24](artifacts/decision-log.md#d24-empty-section-state-rendering)):

```
▾ Iteration  (0)
  (empty — no items)
  + Add step
```

The `(empty — no items)` line is rendered in `LightGray` and is non-focusable; it is purely informational. The `+ Add` row is the focusable affordance.

Outline scroll indicator ([D25](artifacts/decision-log.md#d25-outline-scroll-indicator-rendering)):

When the outline content exceeds the visible pane height, a single-column scroll-position glyph runs down the rightmost column of the outline pane. `▲` at the top half if scrolled past the top, `█` for the visible-region indicator, `▼` at the bottom if more content remains below. Empty space in the column is `│` to match the chrome.

## Detail pane — field rendering

The detail pane is a vertically scrolling area showing the fields of the currently selected outline item, or — when an outline section header is selected — a summary of the section's contents. Each field is rendered as a label-plus-input pair on one or two rows depending on the field kind. Every field has a focus state (focused or unfocused), and field kinds have distinct visual signifiers when unfocused so the user can tell them apart at a glance ([D26](artifacts/decision-log.md#d26-detail-pane-field-rendering-grammar)).

The focused field's bracket border `[` `]` brightens from `LightGray` to `White` and the cursor `▏` glyph in `White` appears at the cursor position. **Reverse-video is not applied to the field body** for normal focus — it is reserved for the *highlighted item inside an open dropdown* (choice list, model suggestion list, or menu).

### Field label truncation

Field labels for env / containerEnv entries are user-supplied keys and may be arbitrarily long. Labels are right-truncated with a `…` suffix when they exceed 28 characters, preserving readable space for the input box ([D50](artifacts/decision-log.md#d50-detail-pane-field-label-truncation-rule)).

### Detail-pane scroll indicator

When the detail pane's content exceeds the visible pane height, a single-column scroll-position glyph runs down the second-rightmost column of the detail pane (one column inside the right border, leaving the `│` border intact). `▲` for "more above," `█` for the visible-region indicator, `▼` for "more below," `│` (matching the chrome) elsewhere. Mirrors the outline pane's scroll indicator from [D25](artifacts/decision-log.md#d25-outline-scroll-indicator-rendering) ([D47](artifacts/decision-log.md#d47-detail-pane-scroll-indicator-rendering)).

### Choice-list dropdown overflow

When an open dropdown would extend below the pane bottom, it instead renders **upward**, anchored to the bottom edge of the field row instead of its top edge. If the dropdown also exceeds the available upward space, it scrolls internally with the same scroll-indicator pattern as the detail pane ([D51](artifacts/decision-log.md#d51-choice-list-dropdown-overflow-rule)).

### Plain-text fields

Format ([D27](artifacts/decision-log.md#d27-plain-text-field-rendering)):

```
  Name: [my-step                            ]    identifier only
  Capture: [iteration_output                ]
```

- Label in `LightGray`, colon, single space, then a bracketed input box in `LightGray`
- Input value in `White`
- Cursor when focused: a `▏` block in `White` at the cursor position; the input border brightens to `White` for the focused state
- Right-aligned hint text in `LightGray` for fields with input constraints
- Below-the-input warning row appears when sanitization fired:

```
  ↳ pasted content sanitized (newlines stripped)
```

### Constrained-fields (choice list)

Format closed ([D28](artifacts/decision-log.md#d28-choice-list-closed-and-open-rendering)):

```
  Capture mode: [lastLine ▾]
  On timeout: [continue ▾]
```

The `▾` indicator inside the input box is the spec D27 unfocused signifier and is **always present** for choice-list fields, focused or not, so users can recognize them by sight. When focused, the bracket border brightens to `White`. When opened (Enter or Space), a small bordered dropdown extends below the field showing every choice; the highlighted option renders in reverse video.

### Numeric fields

Format ([D29](artifacts/decision-log.md#d29-numeric-field-rendering)):

```
  Timeout: [60       ] seconds   1..86400
  Refresh: [0        ] seconds   0 disables refresh
```

- Same bracketed-input shape as plain text, but the input area is right-padded with spaces so the digits align right
- A units suffix (`seconds`) renders in `LightGray` after the input
- A right-aligned hint text shows the valid range or, for fields where 0 is special, an inline rationale (matching D29 of the behavioral spec)

### Secret-mask fields

Format default (masked) ([D30](artifacts/decision-log.md#d30-secret-mask-field-rendering-and-toggle-states)):

```
  ANTHROPIC_API_KEY: [••••••••                    ]    r to reveal
```

Format revealed:

```
  ANTHROPIC_API_KEY: [sk-ant-api03-…                ]    r to mask
```

- The `••••••••` glyph in `LightGray`
- The `r to reveal` / `r to mask` hint right-aligned, in `LightGray`, **without bracket delimiters** so it does not look like a clickable affordance
- On focus-leave the value re-masks automatically and the hint reverts to `r to reveal`

### Model-suggestion field

Format closed (free-text + suggestion availability indicator) ([D31](artifacts/decision-log.md#d31-model-suggestion-field-rendering-and-dropdown)):

```
  Model: [claude-sonnet-4-6                ▾]
```

The `▾` here signifies "suggestions available" rather than constrained-list. When opened (Enter or Tab), a dropdown shows the suggestion list — same rendering as the choice-list dropdown, but the input remains editable while the dropdown is open and the user can dismiss the dropdown without committing a suggestion. Typing a character filters the dropdown. Free text outside the suggestion list is accepted on dropdown-close (Escape commits the typed value).

### Multi-line / external-editor fields

Format ([D32](artifacts/decision-log.md#d32-multi-line-field-rendering-and-edit-handle)):

```
  Prompt file: prompts/iterate.md
               5,237 bytes · last modified 2026-04-26 14:32
               [Ctrl+E open in editor]
```

- The path on the first row in `White` (or `LightGray` if the file does not exist; in that case a "not found" inline marker appears next to the path)
- A second informational row in `LightGray` showing size and mtime when the file exists
- A third action row in `White` showing the edit-handle text — when this field is focused, the status footer also shows `Ctrl+E open in editor` (consistent footer-from-widget rule from the behavioral spec D-11)

### Section-summary view

When an outline section header is the focused row, the detail pane renders the section-summary content for that section type ([D33](artifacts/decision-log.md#d33-section-summary-rendering-per-section-type)):

```
  Iteration phase  ·  3 steps

  1. step-name              sonnet
  2. my-shell-step          shell
  3. another-step           opus

  + Add step
```

The `+ Add` row in the section-summary is interactive — pressing Enter on it triggers the same add affordance as pressing Enter on the outline's `+ Add` row.

## Status footer — content per mode

The footer's text is owned by the focused widget (per behavioral spec D-11) and varies across all 28 modes from the mode-coverage table. Every mode's footer string is enumerated in the mockups; here, the spec commits to one shape rule: the footer is always `<key>  <description>  <key>  <description>  …` separated by two spaces, two-tone colored — keys in `White`, descriptions in `LightGray` — exactly matching the run-mode footer's two-tone shortcut-line rendering rule. When the footer is a **prompt** instead of a shortcut row (e.g., `Quit the workflow builder? (y/n, esc to cancel)`), the entire string renders in `White` ([D34](artifacts/decision-log.md#d34-status-footer-shortcut-vs-prompt-rendering-rule)).

The version label `vX.Y.Z` is right-aligned and renders in `White`, identical to the run-mode footer's right edge ([D35](artifacts/decision-log.md#d35-version-label-position-and-color)).

## Dialogs — common visual conventions

Every dialog (the 15 named `DialogKind` constants from the implementation decision log D-8) renders as a centered overlay matching the run-mode help-modal's visual conventions ([D36](artifacts/decision-log.md#d36-dialog-centered-overlay-shape-and-borders)):

- Centered horizontally and vertically over the underlying frame
- Width: `min(terminalWidth - 4, 72)` columns; minimum 30 columns; height grown to content
- Bordered with the same chrome glyphs as the outer frame (`╭`, `╮`, `╰`, `╯`, `│`, `─`)
- A title segment in the top border in `White`, in the form `╭─ Title text ─…─╮`
- A bottom border with the dialog's option footer right-aligned in `White`, in the form `…  Cancel  Save  Discard  ╯`. The keyboard-default option is wrapped in `[ ]` brackets in `Green` so it's visually distinct ([D37](artifacts/decision-log.md#d37-dialog-keyboard-default-rendering))
- A blank row between the top border and the body, and between the body and the option footer
- Body content rendered with a 2-column inner indent

Per the behavioral spec's Dialog Conventions section (D48), Escape is equivalent to Cancel/safe and the safe option is the keyboard default; the visual `[ ]` wrap on the default makes that contract visible at a glance.

## Findings panel — visual layout

The findings panel replaces the **detail pane only** (the outline remains visible) when active. The outline keeps its scroll position and focus state for the post-dismiss focus-restoration target ([D38](artifacts/decision-log.md#d38-findings-panel-occupies-detail-pane-only-not-full-screen)).

Layout:

```
  Findings  ·  3 fatal · 2 warn · 1 info

  [FATAL] schema.steps[2].promptFile
          Prompt file not found: prompts/missing.md
          ↩ jump to field

  [WARN]  schema.steps[1].timeoutSeconds
          0 disables timeout — was this intentional?
          ↩ jump to field      a acknowledge

  [INFO]  schema.statusLine.refreshIntervalSeconds
          0 disables automatic refresh
          ↩ jump to field
```

- Header row: title in `White`, severity counts in `LightGray` separated by `·`
- Each finding entry is three or four rows:
  1. Severity prefix in red (`FATAL`), yellow (`WARN`), cyan (`INFO`); category path in `White`
  2. Problem text indented under the severity prefix in `White`
  3. Action row indented further: `↩ jump to field` always present; `a acknowledge` present only for WARN/INFO entries
- The focused finding renders with reverse-video on the severity-prefix row
- Between findings: a single blank row
- Acknowledged warnings render with `[WARN ✓]` and a strikethrough on the problem row, but remain visible in the panel (per spec D23 — ack only suppresses the dialog, not the panel) ([D39](artifacts/decision-log.md#d39-acknowledged-warning-visual-treatment))

When the help modal opens over the findings panel (the only legal coexistence per impl-decision D-8), the panel itself renders with a 50%-luminance dim — text in `LightGray`'s dimmer variant `Color("8")` — so the help modal reads as the focused surface.

## Help modal — visual layout

The help modal mirrors the run-mode help-modal's centered-overlay rendering ([D40](artifacts/decision-log.md#d40-help-modal-mirrors-run-mode-help-modal-shape)):

- Centered overlay; width `min(terminalWidth - 4, 72)` columns
- Top border with title `╭─ Help: Keyboard Shortcuts ─…─╮`
- Body sectioned by mode label (Edit / Outline focus / Detail focus / Reorder / Dialog / Findings / Path picker / Empty editor)
- Each section: a section label row in `White` followed by a shortcut grid (key in `White`, description in `LightGray`) in the same two-tone format as the status footer
- Bottom row right-aligned `?  close help  esc  close help` in two-tone

The shortcut grid renders in **two columns** when the modal's interior width (modal width minus 4 for borders and padding) is at least 56 columns; below that, the grid renders in **single column**. When content height exceeds the terminal height, the modal scrolls — a single-column scroll indicator runs down the second-rightmost column (one column inside the right border, leaving the `│` border intact), and the dismiss-hint row is pinned to the modal's bottom border row regardless of scroll position.

## Reorder mode — visual treatment

Reorder mode is entered via `r` on a focused step (or persists during `Alt+↑`/`Alt+↓` drag). Visual changes ([D41](artifacts/decision-log.md#d41-reorder-mode-visual-treatment)):

- The moving step row's `⋮⋮` gripper renders in `Green` (instead of `LightGray`)
- The moving step row's content renders with reverse-video background
- The status footer shows `↑/↓ move step  Enter commit  Esc cancel`
- **The session header is unchanged** — any priority banner (`[ro]`, `[ext]`, `[sym]`, `[shared]`, `[?fields]`) remains visible. The three signals above (footer + reverse-video + green gripper) are sufficient to communicate reorder mode without suppressing higher-priority status

When the moving step crosses a phase boundary, the row visibly stops at the boundary — the row remains rendered in its last-valid position and a brief flash on the boundary row signals the rejection.

## Path picker — visual layout

The path picker is a centered dialog with the standard dialog conventions (per [D36](artifacts/decision-log.md#d36-dialog-centered-overlay-shape-and-borders)) plus a single-line text-input that accepts filesystem tab-completion ([D42](artifacts/decision-log.md#d42-path-picker-input-line-and-completion-behavior)). All inline notices in the picker carry a textual prefix tag — `[warn]` (yellow) for warnings, `[hint]` (light gray) for non-warning notes — consistent with the banner vocabulary's color-blind-safety contract.

```
╭─ Open workflow file ──────────────────────────────────╮
│                                                       │
│   Path: [/Users/me/proj/.pr9k/workflow/config.json▏]  │
│   tab completes  ·  enter opens  ·  esc cancels       │
│                                                       │
│   ↳ this is a directory — add /config.json to open    │
│                                                       │
│                            [ Cancel ]  Open           │
╰───────────────────────────────────────────────────────╯
```

- Title varies: `Open workflow file` (PickerKindOpen), `Where should the new workflow be saved?` (PickerKindNew)
- Pre-filled default path appears with cursor at end
- Tab-completion: when multiple matches exist, a small inline match list appears below the hint row in `LightGray`; cycling on repeated Tab is indicated by a transient `(N/M)` counter
- Inline-warning row appears in `Color("11")` (yellow) below the hint row when the typed path raises a known warning (existing config.json, directory mismatch, path not found)
- Option footer renders `[ Cancel ]  Open` (or `[ Cancel ]  Create` for PickerKindNew) with Cancel as the default

## Empty-editor view — visual layout

When the builder is running with no workflow loaded ([D43](artifacts/decision-log.md#d43-empty-editor-view-visual-layout)):

- Outline pane: a single line `No workflow open` rendered in `LightGray`, centered horizontally within the outline pane, vertically a few rows from the top
- Detail pane: a centered hint panel rendered as a small bordered box (smaller than full dialog, no overlay — the box sits inside the detail-pane region) containing two lines:

  ```
  File > New (Ctrl+N) — create a workflow
  File > Open (Ctrl+O) — open an existing config.json
  ```

- Session header: target-path slot reads `(no workflow open)` in `LightGray`; no banner; no findings summary
- File > Save in the menu dropdown is greyed (per [D12](artifacts/decision-log.md#d12-greyed-menu-item-rendering-and-disable-rules))
- Status footer: `F10 menu  Ctrl+N new  Ctrl+O open  Ctrl+Q quit  ?  help`

## External-editor handoff — visual layout

While the editor is being launched and during the editor's own session, the builder cannot render content (the terminal belongs to the editor). The two-step handoff (per impl-decision D-26) produces these visible frames ([D44](artifacts/decision-log.md#d44-external-editor-handoff-two-step-rendering)):

1. **Pre-handoff frame** (cycle 1): `DialogExternalEditorOpening` is open; the dialog overlay shows `Opening editor…` centered with a brief context line naming the file and the resolved editor binary in `LightGray`.
2. **Editor frame** (during handoff): the builder's chrome is gone; the editor owns the screen.
3. **Post-handoff frame** (after editor exits): the builder's chrome reappears immediately; the dialog is gone; the detail pane scrolls to make the just-edited field visible (cursor lands on the same field that was focused before the editor opened); any warning/info banners (e.g., `editor exited non-zero — file re-read anyway`) appear briefly in the session header.

If the terminal was resized during the editor session, the resize is treated as a `tea.WindowSizeMsg` on the first post-return render cycle, and the scroll-to-field rule applies after the resize has been laid out.

## Color palette extensions

The behavioral spec's existing palette is reused unchanged. This visual-design layer adds a small number of named colors used only for severity, warning, and focus-feedback rendering ([D45](artifacts/decision-log.md#d45-color-palette-extensions)):

- `Red = lipgloss.Color("9")` — FATAL findings, read-only banner
- `Yellow = lipgloss.Color("11")` — WARN findings; external/symlink/shared-install banners; reorder boundary flash
- `Cyan = lipgloss.Color("14")` — INFO findings; unknown-field banner
- `Dim = lipgloss.Color("8")` — placeholder labels, dimmed-when-overlaid content (findings panel under help modal)

The four are imported and used purely for foreground colors; backgrounds remain default (terminal background) except where reverse-video is explicitly named.

## Glyph reference

This layer commits to one source-of-truth glyph set, all from the BMP and reliably rendered by every terminal that meets pr9k's existing capability bar ([D46](artifacts/decision-log.md#d46-glyph-set-bmp-only-and-fallback-policy)):

| Glyph | Code point | Use |
|-------|-----------|-----|
| `╭` `╮` `╰` `╯` | U+256D-U+2570 | Frame and dialog corners (rounded) |
| `│` | U+2502 | Vertical chrome and pane separator |
| `─` | U+2500 | Horizontal chrome and dialog top/bottom |
| `├` `┤` `┬` `┴` | U+251C-U+2534 | T-junctions on rules and pane separator |
| `┼` | U+253C | Cross-junction (only when needed; kept off the default rendering) |
| `▾` | U+25BE | Choice-list / suggestion-list dropdown signifier |
| `▸` `▾` | U+25B8 / U+25BE | Outline collapsed / expanded |
| `⋮⋮` | U+22EE × 2 | Step-row gripper (matches behavioral D34's font-coverage commitment) |
| `≡` | U+2261 | Claude-step kind indicator |
| `$` | U+0024 | Shell-step kind indicator |
| `●` | U+25CF | Unsaved-changes indicator |
| `▏` | U+258F | Cursor in text input |
| `↩` | U+21A9 | "Jump to field" finding action |
| `↳` | U+21B3 | Inline below-input warning marker |
| `▲` `▼` `█` | U+25B2 / U+25BC / U+2588 | Outline scroll indicator |
| `…` | U+2026 | Truncation ellipsis |
| `•` | U+2022 | Secret-mask fill |
| `·` | U+00B7 | Inline separator (e.g., `3 fatal · 2 warn`) |

Every dialog and panel mockup uses exactly these glyphs.

## Coordinations

| Coordinating layer | Direction | Interaction |
|--------------------|-----------|-------------|
| `internal/ui` chrome rendering | inbound (reuse) | The visual design borrows the run-mode top-border, line-wrap, hrule construction, and shortcut-line coloring rules verbatim. Any changes to the run-mode chrome propagate to the builder for free. The `┬`/`┴` junction glyphs at the pane separator are the one visual element this spec adds that does not trace to a run-mode precedent. |
| `internal/workflowedit` model | outbound | The visual design supplies the output rules for the builder's render pass. Update logic is unchanged from the behavioral spec. |
| `bubbletea` overlay machinery | outbound (reuse) | Centered overlays for the help modal and dialogs use the same overlay splice helper the run-mode help modal already uses. |
| Terminal capability bar | precondition | The same alt-screen + 256-color + cell-motion-mouse + box-drawing-glyph requirements as run-mode apply unchanged. |

## Out of Scope

- Changing **what** the builder does. This layer defines *appearance*; behavior is owned by `../workflow-builder/feature-specification.md`.
- Adding new modes beyond the 28 in the behavioral spec's mode-coverage table.
- Theming (light mode, color customization, accessibility palette swaps). All colors are fixed to terminal-default backgrounds with the foreground palette listed in [D45](artifacts/decision-log.md#d45-color-palette-extensions).
- Mouse-driven editing affordances beyond what the behavioral spec already commits to (left-pane mouse focus, scroll-wheel routing, gripper drag).
- Internationalization (right-to-left layouts, non-ASCII letter mnemonic keys, double-width-character-aware truncation beyond what `lipgloss.Width` already handles).
- Animations beyond the existing run-mode "thinking dots" heartbeat suffix. The reorder-mode "flash" at phase boundaries is a one-frame inversion, not an animation loop.

## Documentation Obligations

This visual-design spec ships alongside the implementation that realizes it. The implementation PR adds:

- An update to `docs/features/workflow-builder.md` referencing this design spec for the visual layer.
- A new `docs/code-packages/workflowedit.md` section "Visual Layout" cross-linking the mockups in this folder.
- A note in `docs/how-to/using-the-workflow-builder.md` calling out the menu-bar visual states (closed / open / item greyed) and the at-most-one-banner priority rule.
- A pointer in `CLAUDE.md` to this folder under the workflow-builder feature documentation list.

## Mockups

Every mode and overlay enumerated above has an ASCII mockup in `mockups/`. The index lives at [`mockups/README.md`](mockups/README.md). Each mockup file fences its renders inside ` ```text ` blocks so they can be diffed directly against the rendered output.

## Open Items

- **OI-1 (deferred).** Whether to introduce a higher-contrast palette for accessibility (color-blind users) is out of scope here; the current palette inherits from the run-mode TUI which has not made that commitment either. If the project later adds a global accessibility theme switch, this design will inherit it without changes.
- **OI-2 (deferred).** Animations longer than one frame (e.g., a gradient on the cursor blink, animated reorder swap) are not specified. The implementation should not invent any.

## Summary

- **Outcome delivered.** `pr9k workflow` renders a full-screen, frame-bordered TUI matching the run-mode TUI's visual identity, with menu bar, session header, two-pane editing area, status footer, centered dialogs, findings panel, and help modal — visualizing every state the behavioral spec commits to.
- **Decisions committed:** 51 — see [`artifacts/decision-log.md`](artifacts/decision-log.md). 46 from the initial draft; 5 added in review (D47 detail-pane scroll indicator, D48 minimum terminal size, D49 step-name truncation, D50 field-label truncation, D51 dropdown overflow). 10 decisions clarified or revised in review (D3, D6, D9, D15, D16, D22, D30, D40, D41, D42, D44).
- **Mockups produced:** 25 files under [`mockups/`](mockups/) — initial 24 + `23-dialog-save-in-progress.md` (added per F13) + `24-full-layout-browse-only.md` (added per F14). Covers every full-frame state, every outline / detail-pane field, every dialog from the (now 15-strong) `DialogKind` set, the findings panel, the help modal (3 variants including narrow-terminal and scroll cases), reorder mode (with banner-preserved variant), and the session-header overflow stress test.
- **Sub-agents consulted:** `junior-developer` (6 findings), `user-experience-designer` (7 findings), `gap-analyzer` (full report at [`artifacts/visual-gaps.md`](artifacts/visual-gaps.md), 16 gaps), `edge-case-explorer` (9 boundary cases). All 25 findings recorded in [`artifacts/team-findings.md`](artifacts/team-findings.md) and resolved by evidence — zero required user input under auto-mode.
- **Open items:** 2 deferred — palette accessibility (OI-1), animation policy (OI-2). Neither blocks implementation.
