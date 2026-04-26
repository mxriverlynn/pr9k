# 04 — Outline Panel

The left pane in every loaded-workflow render: phase grouping, env / containerEnv / statusLine sections, +Add affordance, gripper, focused row, collapsed state, scroll indicator, empty section.

The outline pane is a fixed `min(40, max(20, ⌊W × 0.4⌋))` columns wide. All renders below show the outline pane only — the `│` at the right is the pane separator (column 41 in a 120-col terminal).

## A: Full populated outline (all six sections expanded)

```text
│  ▾ env  (1)                          │
│    ⋮⋮ MY_TOKEN                       │
│    + Add env variable                │
│  ▾ containerEnv  (2)                 │
│    ⋮⋮ ANTHROPIC_API_KEY              │
│    ⋮⋮ DEBUG                          │
│    + Add container env entry         │
│  ▾ statusLine  (1)                   │
│    ⋮⋮ [≣] script-based               │
│  ▾ Initialize  (1)                   │
│    ⋮⋮ [≡] splash      sonnet         │
│    + Add step                        │
│  ▾ Iteration  (3)                    │
│    ⋮⋮ [≡] iterate     sonnet  [F1]   │
│    ⋮⋮ [≡] test-plan   opus           │
│    ⋮⋮ [$] commit                  — │
│    + Add step                        │
│  ▾ Finalize  (2)                     │
│    ⋮⋮ [≡] code-review opus           │
│    ⋮⋮ [$] update-docs             — │
│    + Add step                        │
```

- Section headers in **light gray**, with `▾` glyph and parens-wrapped count
- Step rows have `⋮⋮` gripper + kind glyph + name + right-aligned model column
- Shell-step rows show `—` instead of a model name (right-aligned at the same column)
- The `[F1]` red marker on `iterate` indicates 1 fatal finding attached to that step
- `+ Add` rows in **light gray**
- The `[≣]` glyph distinguishes a statusLine block from a Claude step

## B: Focused step row

```text
│  ▾ Iteration  (3)                    │
│ (white)> ⋮⋮ [≡] iterate     sonnet  [F1](/)│
│    ⋮⋮ [≡] test-plan   opus           │
```

- Focused row has `> ` prefix (replacing the leading `  `)
- Entire row's text rendered in **white**
- **No reverse-video** — reverse-video is reserved for reorder mode and open-dropdown highlights only

## C: Focused section header

```text
│ (white)> ▾ Iteration  (3)                   (/) │
│    ⋮⋮ [≡] iterate     sonnet  [F1]   │
```

- Same focus-state rules as a step row applied to the section header

## D: Focused `+ Add` affordance

```text
│    ⋮⋮ [$] commit                  — │
│ (white)> + Add step                         (/) │
```

- `+ Add step` rendered with `> ` prefix in **white**
- Status footer shows `Enter add` while focus is on this row

## E: Collapsed section

```text
│  ▾ env  (1)                          │
│    ⋮⋮ MY_TOKEN                       │
│    + Add env variable                │
│  ▸ containerEnv  (2)                 │
│  ▾ statusLine  (1)                   │
```

- The `▸` glyph indicates collapsed state
- Item rows hidden but the count `(2)` remains visible in the section header (per behavioral D28)

## F: Empty section

```text
│  ▾ env  (0)                          │
│    (gray)(empty — no items)(/)               │
│    + Add env variable                │
```

- `(empty — no items)` rendered in **light gray**, non-focusable
- `+ Add env variable` row remains the focusable affordance

## G: Scroll indicator (content exceeds pane height)

When the outline content is taller than the visible pane area, a single-column scroll indicator runs down the rightmost column of the outline pane.

Top of outline scrolled (more above):

```text
│  ▾ Iteration  (5)                  ▲│
│    ⋮⋮ [≡] step-3       sonnet      │ │
│    ⋮⋮ [≡] step-4       sonnet      │█│
│    ⋮⋮ [≡] step-5       sonnet      │█│
│    ⋮⋮ [$] commit                  —│█│
│    + Add step                      │ │
│  ▾ Finalize  (2)                   │ │
│    ⋮⋮ [≡] code-review  opus        │ │
│    ⋮⋮ [$] update-docs             —│ │
│    + Add step                      │▼│
```

- The rightmost column of the outline pane (column 40 in a 40-column outline) shows `▲` at the top, `█` for the visible region indicator, `▼` at the bottom, and `│` (matching the chrome) elsewhere
- Spaces in the column are filled with the chrome glyph so the column reads as a continuous scroll track

## H: Statusline section absent

When no statusLine block is configured:

```text
│  ▾ statusLine  (0)                   │
│    + Add statusLine block            │
```

- The single affordance row is the only body content

## I: All sections collapsed

```text
│  ▸ env  (1)                          │
│  ▸ containerEnv  (2)                 │
│  ▸ statusLine  (1)                   │
│  ▸ Initialize  (1)                   │
│  ▸ Iteration  (3)                    │
│  ▸ Finalize  (2)                     │
```

- All sections collapsed; counts visible in headers
- Useful for users who want to scan the structure before drilling into a phase

## J: Reorder mode active on a step

```text
│  ▾ Iteration  (3)                    │
│    ⋮⋮ [≡] test-plan   opus           │
(reverse)│   (green)⋮⋮(/) [≡] iterate     sonnet     │(/)
│    ⋮⋮ [$] commit                  — │
```

- The moving step `iterate` rendered with reverse-video background — this is one of only two states using reverse-video (the other is the highlighted item inside an open dropdown)
- Its gripper `⋮⋮` rendered in **green** (instead of light gray)
- The session header is unchanged (no `[reorder]` banner — see [`07-reorder-mode.md`](07-reorder-mode.md))

## K: Reorder mode at phase boundary

When the moving step is dragged past a phase boundary, the row visibly stops at the boundary and a one-frame yellow flash signals the rejection.

```text
│  ▾ Initialize  (1)                   │
│    ⋮⋮ [≡] splash      sonnet         │
(yellow)│ ──────────────────────────────────── │(/)   ← one-frame flash on boundary
│  ▾ Iteration  (3)                    │
(reverse)│   (green)⋮⋮(/) [≡] iterate     sonnet     │(/)   ← held at top of iteration
│    ⋮⋮ [≡] test-plan   opus           │
```

- The boundary indicator is a one-frame inverted line on the section-header row separating Initialize from Iteration
- After the flash, the `iterate` step remains pinned at the top of Iteration (cannot cross into Initialize)

## Cross-references

- Behavioral spec: [Primary Flow §5–§7](../../workflow-builder/feature-specification.md#primary-flow), [D28](../../workflow-builder/artifacts/decision-log.md#d28-collapsible-section-behavior), [D29](../../workflow-builder/artifacts/decision-log.md#d29-outline-scrollability), [D34](../../workflow-builder/artifacts/decision-log.md#d34-step-reorder-ux), [D46](../../workflow-builder/artifacts/decision-log.md#d46-add-item-affordance-and-keyboard-binding), [D51](../../workflow-builder/artifacts/decision-log.md#d51-section-summary-content).
- Impl decisions: [D-12](../../workflow-builder/artifacts/implementation-decision-log.md), [D-29](../../workflow-builder/artifacts/implementation-decision-log.md), [D-35](../../workflow-builder/artifacts/implementation-decision-log.md).
- Visual decisions: [D7](../artifacts/decision-log.md#d7-outline-pane-width-rule), [D18](../artifacts/decision-log.md#d18-outline-section-order-and-grouping)–[D25](../artifacts/decision-log.md#d25-outline-scroll-indicator-rendering), [D41](../artifacts/decision-log.md#d41-reorder-mode-visual-treatment).
- Mode coverage: rows 6, 9–13, 19.
