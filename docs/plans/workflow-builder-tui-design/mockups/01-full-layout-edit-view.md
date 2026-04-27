# 01 — Full Layout: Edit View

The canonical loaded-workflow view. Corresponds to mode-coverage Part B starting states (modes 6–17).

## State

- Workflow loaded: `~/projects/foo/.pr9k/workflow/config.json`
- Outline cursor on iteration step `iterate`
- Detail pane shows that step's fields
- 3 fatal · 2 warn findings exist (visible in session header)
- Workflow is dirty (unsaved changes)

## Render (120×32 terminal)

```text
╭── (green)Power-Ralph.9000(/) — Workflow Builder — ~/projects/foo/.pr9k/workflow/config.json ──────────────────────╮
│  (white)F(/)ile                                                                                                   │
├───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  ~/projects/foo/.pr9k/workflow/config.json (green)●(/)                                  (red)[!](/) 3 fatal · 2 warn │
├──────────────────────────────────────┬────────────────────────────────────────────────────────────────────────────┤
│  ▾ env  (1)                          │  Step  ·  iterate                                                          │
│    ⋮⋮ MY_TOKEN                       │                                                                            │
│    + Add env variable                │   (white)Name:(/)         [iterate                            ]   identifier only │
│  ▾ containerEnv  (2)                 │                                                                            │
│    ⋮⋮ ANTHROPIC_API_KEY              │   (white)Kind:(/)         [Claude (≡)                       ▾]                   │
│    ⋮⋮ DEBUG                          │                                                                            │
│    + Add container env entry         │   (white)Model:(/)        [claude-sonnet-4-6                ▾]                   │
│  ▾ statusLine  (1)                   │                                                                            │
│    ⋮⋮ [≣] script-based               │   (white)Prompt file:(/) prompts/iterate.md                                      │
│  ▾ Initialize  (1)                   │                  (gray)5,237 bytes · last modified 2026-04-26 14:32(/)               │
│    ⋮⋮ [≡] splash      sonnet         │                  [(white)Ctrl+E(/) open in editor]                                  │
│    + Add step                        │                                                                            │
│  ▾ Iteration  (3)                    │   (white)Capture as:(/)   [iteration_output                  ]                    │
│ (white)> ⋮⋮ [≡] iterate     sonnet  [F1](/)│   (white)Capture mode:(/) [lastLine                       ▾]                   │
│    ⋮⋮ [≡] test-plan   opus           │                                                                            │
│    ⋮⋮ [$] commit                  — │   (white)Timeout:(/)      [180         ] seconds   1..86400                       │
│    + Add step                        │   (white)On timeout:(/)   [continue                       ▾]                   │
│  ▾ Finalize  (2)                     │                                                                            │
│    ⋮⋮ [≡] code-review opus           │   (white)Skip if capture empty:(/) [yes                        ▾]                   │
│    ⋮⋮ [$] update-docs             — │   (white)Resume previous:(/)       [no                         ▾]                   │
│    + Add step                        │                                                                            │
│                                      │   (white)Break loop if empty:(/)   [no                         ▾]                   │
│                                      │                                                                            │
│                                      │                                                                            │
├──────────────────────────────────────┴────────────────────────────────────────────────────────────────────────────┤
│  (white)↑↓(/) navigate  (white)Tab(/) detail pane  (white)Enter(/) edit  (white)Del(/) remove  (white)r(/) reorder  (white)a(/) add step       (white)v0.7.3(/) │
╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

## Annotations

- Top border title: brand name in **green**, separator and surface name in **white**, path in **white**, surrounding `─` chrome in **light gray**. Path uses `~` shorthand for the user's home dir.
- Session header: target path in **white**, `●` dirty indicator in **green**, findings summary `[!] 3 fatal · 2 warn` right-aligned with `[!]` in **red** (because fatals exist) and the count text in **light gray** with the bullet `·` glyph.
- Outline pane (left, 40 cols):
  - Six section headers in expanded state, each with `▾`, name, and parens-wrapped count
  - Step rows show gripper `⋮⋮` (light gray), kind glyph `[≡]` for Claude or `[$]` for shell, step name (light gray for unfocused, white for focused), right-aligned model name for Claude steps or `—` for shell steps
  - The `[F1]` red marker appears on the focused `iterate` step indicating one fatal finding attached
  - The focused row (the iterate step) has the `> ` prefix and the entire row text in **white**; **no reverse-video** (reverse-video is reserved for reorder mode and open-dropdown highlights only)
  - Each section ends with a `+ Add <item-type>` row in **light gray**
- Detail pane (right):
  - Header `Step  ·  iterate` in **white**
  - Field labels in **white** (with colon), input boxes in **light gray** with values in **white**
  - The `▾` glyph on Kind, Model, Capture mode, On timeout, Skip-if-capture-empty, Resume previous, Break loop fields signals constrained / suggestion list
  - The Prompt file field shows three rows: path, metadata, action handle
  - The Timeout field shows units (`seconds`) in **light gray** after the input, with range hint `1..86400` in **light gray** at the right
- Status footer: every shortcut for the outline-step-focused mode listed; `v0.7.3` right-aligned in **white**
- Pane separator: `│` runs through every pane row at column 41; `┬` at the top intersection (in the session-header hrule) and `┴` at the bottom intersection (in the footer hrule)

## Cross-references

- Behavioral spec: [Primary Flow §5–§7](../../workflow-builder/feature-specification.md#primary-flow).
- Visual decisions: [D2](../artifacts/decision-log.md#d2-row-order-of-the-persistent-frame), [D5](../artifacts/decision-log.md#d5-session-header-row-content-and-ordering), [D6](../artifacts/decision-log.md#d6-pane-area-vertical-separator-and-junction-glyphs), [D7](../artifacts/decision-log.md#d7-outline-pane-width-rule), [D18](../artifacts/decision-log.md#d18-outline-section-order-and-grouping), [D21](../artifacts/decision-log.md#d21-step-item-row-format-and-glyphs), [D22](../artifacts/decision-log.md#d22-cursor-row-rendering-in-outline), [D26](../artifacts/decision-log.md#d26-detail-pane-field-rendering-grammar).
- Mode coverage: rows 6–17 (Part B starting state).
