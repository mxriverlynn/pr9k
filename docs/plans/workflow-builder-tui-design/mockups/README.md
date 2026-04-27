# Mockups: Workflow-Builder TUI

Every mockup in this folder is a fenced ` ```text ` block depicting the rendered terminal output at a specific state. Each frame is sized assuming a **120-column × 32-row terminal** unless explicitly noted; renders at other sizes are derivable by stretching/compressing the variable-width content slots while keeping the chrome layout intact.

The mockups deliberately do not encode color (markdown can't render ANSI). Each block is annotated with color cues in plain English where they matter for correctness — e.g., `(green) Power-Ralph.9000` or `(red) [FATAL]`. The full color palette is documented in the spec at [`../feature-specification.md#color-palette-extensions`](../feature-specification.md#color-palette-extensions) and [decision D45](../artifacts/decision-log.md#d45-color-palette-extensions).

## Index

### Full-frame layouts
- [`00-full-layout-empty-editor.md`](00-full-layout-empty-editor.md) — Empty-editor state (mode 1 starting state)
- [`01-full-layout-edit-view.md`](01-full-layout-edit-view.md) — Populated edit view with outline, detail pane, footer
- [`24-full-layout-browse-only.md`](24-full-layout-browse-only.md) — Read-only browse-only view (greyed Save, `[ro]` banner, no dirty tracking)

### Persistent chrome elements
- [`02-menu-bar.md`](02-menu-bar.md) — Closed menu, open menu, item-greyed states
- [`03-session-header-banners.md`](03-session-header-banners.md) — All five banner types, multi-banner `[N more]`, dirty `●`, transient feedback (saved / no changes)
- [`04-outline-panel.md`](04-outline-panel.md) — Phase grouping, env / containerEnv / statusLine sections, +Add affordance, gripper, collapsed, scroll indicator, empty section
- [`05-detail-pane-fields.md`](05-detail-pane-fields.md) — Every field kind: text, choice (closed + open), numeric, secret-mask, model-suggest, multi-line, section-summary
- [`06-status-footer-modes.md`](06-status-footer-modes.md) — Footer shortcut bars for every mode in the 28-mode coverage
- [`07-reorder-mode.md`](07-reorder-mode.md) — Entering reorder, moving step, commit, cancel, phase-boundary flash

### Modals and panels
- [`08-help-modal.md`](08-help-modal.md) — Workflow-builder help modal (centered overlay)
- [`09-findings-panel.md`](09-findings-panel.md) — Findings panel with severity prefixes, jump action, ack visual, help-over-findings coexistence

### Dialogs (15 named DialogKind variants — original 14 + DialogSaveInProgress added per F13)
- [`10-dialog-new-choice.md`](10-dialog-new-choice.md) — `DialogNewChoice`
- [`11-dialog-path-picker.md`](11-dialog-path-picker.md) — `DialogPathPicker` (PickerKindOpen + PickerKindNew + completion behaviors + warnings)
- [`12-dialog-unsaved-changes.md`](12-dialog-unsaved-changes.md) — `DialogUnsavedChanges`
- [`13-dialog-quit-confirm.md`](13-dialog-quit-confirm.md) — `DialogQuitConfirm`
- [`14-dialog-file-conflict.md`](14-dialog-file-conflict.md) — `DialogFileConflict`
- [`15-dialog-recovery.md`](15-dialog-recovery.md) — `DialogRecovery`
- [`16-dialog-crash-temp-notice.md`](16-dialog-crash-temp-notice.md) — `DialogCrashTempNotice`
- [`17-dialog-first-save-confirm.md`](17-dialog-first-save-confirm.md) — `DialogFirstSaveConfirm` (external + symlink variants)
- [`18-dialog-external-editor-opening.md`](18-dialog-external-editor-opening.md) — `DialogExternalEditorOpening`
- [`19-dialog-copy-broken-ref.md`](19-dialog-copy-broken-ref.md) — `DialogCopyBrokenRef`
- [`20-dialog-remove-confirm.md`](20-dialog-remove-confirm.md) — `DialogRemoveConfirm`
- [`21-dialog-editor-error.md`](21-dialog-editor-error.md) — `DialogEditorError` (no editor / not on PATH / shell metacharacters / spawn failure)
- [`22-dialog-error-template.md`](22-dialog-error-template.md) — Generic `DialogError` (D56 four-element template)
- [`23-dialog-save-in-progress.md`](23-dialog-save-in-progress.md) — `DialogSaveInProgress` (transient: validate / save / save-during-quit)

## Reading the mockups

Every render uses these conventions:

- **Box-drawing chrome** — `╭`, `╮`, `╰`, `╯`, `│`, `─`, `├`, `┤`, `┬`, `┴` are rendered literally.
- **Color cues** — annotations like `(green)`, `(red)`, `(white)` next to a span describe the foreground color. The default chrome and unfocused content is `LightGray` (palette color 245); annotations are added only where the color differs from gray.
- **Cursor rows** — `> ` prefix in place of the leading `  ` indent indicates the focused row. The entire focused row's text renders in `White` instead of `LightGray`.
- **Reverse video** — annotations like `[reversed]` indicate that span renders with a reverse-video background.
- **Cursor in input** — `▏` glyph at the cursor position inside a bracketed input.
- **Truncation** — `…` indicates truncated text.
- **Glyph reference** — see [`../feature-specification.md#glyph-reference`](../feature-specification.md#glyph-reference) for every glyph's meaning.

## Cross-reference

Every mockup links back to:
- Behavioral spec: [`../../workflow-builder/feature-specification.md`](../../workflow-builder/feature-specification.md)
- Mode coverage table: [`../../workflow-builder/artifacts/tui-mode-coverage.md`](../../workflow-builder/artifacts/tui-mode-coverage.md)
- Visual decision log: [`../artifacts/decision-log.md`](../artifacts/decision-log.md)

When the mockup depicts a behavioral mode from the 28-mode coverage table, the mode number is named in the file's header. When the mockup depicts a visual state without a behavioral mode entry (e.g., a banner combination), the relevant decision IDs are named instead.
