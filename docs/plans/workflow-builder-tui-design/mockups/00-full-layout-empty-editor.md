# 00 — Full Layout: Empty-Editor State

This is the **first frame** the user sees after running `pr9k workflow` with no `--workflow-dir` flag. Behavioral spec D68 commits to this state. Corresponds to mode-coverage row 1's *starting state*.

## State

- No workflow loaded
- Menu closed
- No dialog
- No banner
- No findings

## Render (120×32 terminal)

```text
╭── (green)Power-Ralph.9000(/) — Workflow Builder ──────────────────────────────────────────────────────────────────╮
│  (white)F(/)ile                                                                                                   │
├───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  (no workflow open)                                                                                               │
├──────────────────────────────────────┬────────────────────────────────────────────────────────────────────────────┤
│  No workflow open                    │                                                                            │
│                                      │      ╭────────────────────────────────────────────────────╮                │
│                                      │      │                                                    │                │
│                                      │      │   File > New (Ctrl+N) — create a workflow         │                │
│                                      │      │   File > Open (Ctrl+O) — open an existing         │                │
│                                      │      │                            config.json            │                │
│                                      │      │                                                    │                │
│                                      │      ╰────────────────────────────────────────────────────╯                │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
│                                      │                                                                            │
├──────────────────────────────────────┴────────────────────────────────────────────────────────────────────────────┤
│  (white)F10(/) menu  (white)Ctrl+N(/) new  (white)Ctrl+O(/) open  (white)Ctrl+Q(/) quit  (white)?(/) help                                          (white)v0.7.3(/) │
╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

## Annotations

- Top border: `Power-Ralph.9000` is **green**, ` — Workflow Builder` is **white**, surrounding `─` chrome is **light gray**.
- Menu bar row: only `File` rendered. The leading `F` is **white** (mnemonic accent for `Alt+F`); `ile` is **light gray**.
- Session header row: `(no workflow open)` rendered in **light gray**. No banner, no `[N more]`, no findings summary.
- Pane area: a vertical `│` runs through every pane row at column 41 (the outline pane's right edge). The hrule above and below the pane area shows `┬` at column 41 above and `┴` at column 41 below.
- Outline pane: `No workflow open` rendered in **light gray**, vertically near the top of the pane (row 6 of the pane area, column 3).
- Detail pane: a small bordered hint panel sits centered horizontally, rendered with the same chrome glyphs as the outer frame. The two text rows render in **white**; the inner border in **light gray**.
- Status footer: shortcut bar in two-tone (white keys, gray descriptions); right-aligned `v0.7.3` in **white**.

## Cross-references

- Behavioral spec: [Primary Flow §2 + §5](../../workflow-builder/feature-specification.md#primary-flow), [D68](../../workflow-builder/artifacts/decision-log.md#d68-initial-launch-state).
- Visual decisions: [D2](../artifacts/decision-log.md#d2-row-order-of-the-persistent-frame), [D43](../artifacts/decision-log.md#d43-empty-editor-view-visual-layout), [D6](../artifacts/decision-log.md#d6-pane-area-vertical-separator-and-junction-glyphs), [D10](../artifacts/decision-log.md#d10-menu-bar-mnemonic-accent-and-hover-feedback).
- Mode coverage: row 1 starting state.
