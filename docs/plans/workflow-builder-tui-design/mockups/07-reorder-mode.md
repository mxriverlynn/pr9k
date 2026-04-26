# 07 — Reorder Mode

The visual treatment when reorder mode is active. Modes 10–13 in the coverage table.

Reorder mode is entered by pressing `r` on a focused outline step row, or by pressing `Alt+↑` / `Alt+↓` (which is implemented as a transient one-keypress reorder cycle but produces the same visual frames).

## A: Entering reorder mode (frame just after `r` pressed)

```text
╭── (green)Power-Ralph.9000(/) — Workflow Builder — ~/projects/foo/.pr9k/workflow/config.json ──────────────────────╮
│  (white)F(/)ile                                                                                                   │
├───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  ~/projects/foo/.pr9k/workflow/config.json (green)●(/)                                                                       │
├──────────────────────────────────────┬────────────────────────────────────────────────────────────────────────────┤
│  ▾ Iteration  (3)                    │                                                                            │
│    ⋮⋮ [≡] test-plan   opus           │                                                                            │
(reverse)│   (green)⋮⋮(/) [≡] iterate     sonnet     │(/)                                                                            │
│    ⋮⋮ [$] commit                  — │                                                                            │
│                                      │                                                                            │
…                                                                                                                   │
├──────────────────────────────────────┴────────────────────────────────────────────────────────────────────────────┤
│  (white)↑/↓(/) move step  (white)Enter(/) commit  (white)Esc(/) cancel                                                            (white)v0.7.3(/) │
╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

- Session header: **unchanged** — any priority banner stays put (here there is none, just the dirty indicator)
- Outline: the moving step (`iterate`) rendered with reverse-video background, gripper `⋮⋮` in **green** (instead of light gray)
- Footer: reorder shortcuts (the only mode-change signal in the chrome — outline gets the other two)

## B: After `↑` (step moved up)

```text
│  ~/projects/foo/.pr9k/workflow/config.json (green)●(/)                                                                       │
├──────────────────────────────────────┬────────────────────────────────────────────────────────────────────────────┤
│  ▾ Iteration  (3)                    │                                                                            │
(reverse)│   (green)⋮⋮(/) [≡] iterate     sonnet     │(/)                                                                            │
│    ⋮⋮ [≡] test-plan   opus           │                                                                            │
│    ⋮⋮ [$] commit                  — │                                                                            │
```

- The `iterate` row has moved above `test-plan`
- Reverse-video and green gripper continue to mark the moving row
- Session header unchanged

## C: At phase boundary (Initialize/Iteration)

When the user attempts to move the step past the phase boundary, the row visibly stops at the edge and a one-frame yellow flash signals the rejection.

```text
│  ▾ Initialize  (1)                   │
│    ⋮⋮ [≡] splash      sonnet         │
(yellow)│ ──────────────────────────────────── │(/)   ← one-frame flash on boundary
│  ▾ Iteration  (3)                    │
(reverse)│   (green)⋮⋮(/) [≡] iterate     sonnet     │(/)   ← held at top of Iteration
│    ⋮⋮ [≡] test-plan   opus           │
│    ⋮⋮ [$] commit                  — │
```

- The boundary indicator is a one-frame inverted line on the section-header row separating Initialize from Iteration
- After the flash, the `iterate` step remains pinned at the top of Iteration (cannot cross into Initialize)

## D: Commit with `Enter`

After `Enter` commits the move, the visual chrome returns to normal edit-view rendering. The moved step now occupies its new position; the `●` dirty indicator is set.

```text
│  ~/projects/foo/.pr9k/workflow/config.json (green)●(/)                                                                       │
├──────────────────────────────────────┬────────────────────────────────────────────────────────────────────────────┤
│  ▾ Iteration  (3)                    │  Step  ·  iterate                                                          │
│ (white)> ⋮⋮ [≡] iterate     sonnet  [F1](/)│   …                                                                          │
│    ⋮⋮ [≡] test-plan   opus           │   (white)Name:(/)         [iterate                            ]   identifier only │
│    ⋮⋮ [$] commit                  — │   …                                                                          │
…                                                                                                                    │
├──────────────────────────────────────┴────────────────────────────────────────────────────────────────────────────┤
│  (white)↑↓(/) navigate  (white)Tab(/) detail pane  (white)Enter(/) edit  (white)Del(/) remove  (white)r(/) reorder  (white)a(/) add step       (white)v0.7.3(/) │
╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

- The footer reverts to the standard outline-step-focused shortcuts
- The moved step is now in its new position with the standard focused-row appearance (`> ` prefix + white text, **no reverse-video**)

## E: Cancel with `Esc`

After `Esc`, the step is restored to its pre-reorder position; no `●` dirty change is recorded for the cancelled move.

```text
│  ~/projects/foo/.pr9k/workflow/config.json (green)●(/)                                                                       │
├──────────────────────────────────────┬────────────────────────────────────────────────────────────────────────────┤
│  ▾ Iteration  (3)                    │  Step  ·  iterate                                                          │
│    ⋮⋮ [≡] test-plan   opus           │   …                                                                          │
│ (white)> ⋮⋮ [≡] iterate     sonnet  [F1](/)│   …                                                                          │
│    ⋮⋮ [$] commit                  — │   …                                                                          │
…                                                                                                                    │
├──────────────────────────────────────┴────────────────────────────────────────────────────────────────────────────┤
│  (white)↑↓(/) navigate  (white)Tab(/) detail pane  (white)Enter(/) edit  (white)Del(/) remove  (white)r(/) reorder  (white)a(/) add step       (white)v0.7.3(/) │
╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

## F: Auto-scroll during reorder

When the moving step is dragged past the visible viewport edge, the outline auto-scrolls to keep the step visible:

```text
│  ▾ Iteration  (5)                  ▲│
│    ⋮⋮ [≡] step-2       sonnet      │ │
│    ⋮⋮ [≡] step-3       sonnet      │ │
(reverse)│   (green)⋮⋮(/) [≡] iterate     sonnet  │█│(/)
│    ⋮⋮ [≡] step-4       sonnet      │█│
│    ⋮⋮ [≡] step-5       sonnet      │ │
```

- The scroll indicator shifts position as the moving step's location changes
- The viewport's top row updates to keep the moving step at row 4 (or so) of the visible area

## G: Reorder mode while a `[ro]` priority banner is active

This variant demonstrates that the priority banner is **preserved** during reorder mode (per the F9 / F25 resolution — the previous design had the reorder banner replace the priority banner, which suppressed higher-priority status):

```text
│  /usr/local/share/pr9k/workflow/config.json (green)●(/)  (yellow)[shared](/) editing the bundled default — saves affect all users of this binary │
├──────────────────────────────────────┬────────────────────────────────────────────────────────────────────────────┤
│  ▾ Iteration  (3)                    │                                                                            │
│    ⋮⋮ [≡] test-plan   opus           │                                                                            │
(reverse)│   (green)⋮⋮(/) [≡] iterate     sonnet     │(/)                                                                            │
│    ⋮⋮ [$] commit                  — │                                                                            │
…                                                                                                                    │
├──────────────────────────────────────┴────────────────────────────────────────────────────────────────────────────┤
│  (white)↑/↓(/) move step  (white)Enter(/) commit  (white)Esc(/) cancel                                                            (white)v0.7.3(/) │
╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

- The `[shared]` banner stays visible — the user remains aware that they're editing the bundled default
- Reorder mode communicates itself via three signals only: footer change, reverse-video on the moving row, green gripper

The same pattern applies to `[ro]`, `[ext]`, `[sym]`, `[?fields]` banners — none are suppressed during reorder mode.

## Cross-references

- Behavioral spec: [Primary Flow §7](../../workflow-builder/feature-specification.md#primary-flow), [D34](../../workflow-builder/artifacts/decision-log.md#d34-step-reorder-ux).
- Visual decisions: [D41](../artifacts/decision-log.md#d41-reorder-mode-visual-treatment) (revised — banner removed from the session header).
- Mode coverage: rows 10, 11, 12, 13.
- Team findings: [F9](../artifacts/team-findings.md#f9-reorder-banner-suppresses-higher-priority-ro--ext-banners), [F25](../artifacts/team-findings.md#f25-reorder-mode-preserves-prior-banner--variant-missing).
