# 02 — Menu Bar States

The menu bar row in three visible states: closed, open, and item greyed.

## State A: Closed (default)

```text
│  (white)F(/)ile                                                                                                   │
```

- `F` of `File` in **white** (mnemonic accent for `Alt+F`)
- `ile` in **light gray**
- Remainder of row space-padded to the right border

## State B: Closed with mouse hover

```text
│  (reverse)File(/)                                                                                                   │
```

- `File` rendered in reverse-video while the mouse pointer is over the label (cell-motion hover detection)
- Indicates the label is clickable

## State C: Open

```text
│  (reverse)File(/)                                                                                                   │
├──╮                                                                                                                │
│   ╭────────────────────────╮                                                                                      │
│   │ (reverse)New                Ctrl+N(/)│                                                                                      │
│   │ Open               Ctrl+O│                                                                                      │
│   │ Save               Ctrl+S│                                                                                      │
│   │ Quit               Ctrl+Q│                                                                                      │
│   ╰────────────────────────╯                                                                                      │
│                                                                                                                   │
```

- `File` label in reverse-video (signaling the menu is open and `File` has focus)
- Bordered dropdown panel anchored to the `File` label's left edge, extending downward
- Each row contains the item label (left), shortcut label (right) padded to the dropdown width
- The currently-highlighted item (the cursor position inside the menu — `New` in this example) renders in reverse-video
- All item labels in **white**; all shortcut labels in **light gray**

The dropdown uses the same chrome glyphs as the outer frame and overlays the underlying frame using the same overlay splice helper as the help modal.

## State D: Item greyed (Save unavailable)

```text
│  (reverse)File(/)                                                                                                   │
├──╮                                                                                                                │
│   ╭────────────────────────╮                                                                                      │
│   │ New                Ctrl+N│                                                                                      │
│   │ (reverse)Open               Ctrl+O(/)│                                                                                      │
│   │ (gray)Save                      (/)│                                                                                      │
│   │ Quit               Ctrl+Q│                                                                                      │
│   ╰────────────────────────╯                                                                                      │
│                                                                                                                   │
```

- `Save` rendered in **light gray** (not white) and its shortcut label is **omitted**
- Reasons for `Save` greyed: empty-editor (no workflow loaded), browse-only mode (read-only target)
- The cursor is allowed to navigate over the greyed item (with `↑/↓`) but pressing Enter on it is a no-op

## Footer in open-menu state

While the menu is open, the status footer text changes to the menu navigation hints (per behavioral spec D66 "shortcut footer updates to show menu-navigation hints"):

```text
│  (white)↑↓(/) navigate  (white)Enter(/) select  (white)Esc(/) close                                                                          (white)v0.7.3(/) │
```

## Cross-references

- Behavioral spec: [Primary Flow §2 + §5](../../workflow-builder/feature-specification.md#primary-flow), [D64–D67](../../workflow-builder/artifacts/decision-log.md#d64-menu-bar-target-selection-model).
- Visual decisions: [D4](../artifacts/decision-log.md#d4-menu-bar-row-content-and-states), [D10](../artifacts/decision-log.md#d10-menu-bar-mnemonic-accent-and-hover-feedback), [D11](../artifacts/decision-log.md#d11-menu-dropdown-rendering-and-overlay-anchoring), [D12](../artifacts/decision-log.md#d12-greyed-menu-item-rendering-and-disable-rules).
- Mode coverage: row 4 (`F10` opens menu from empty-editor).
