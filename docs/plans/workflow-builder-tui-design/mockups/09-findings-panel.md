# 09 — Findings Panel

The findings panel replaces the **detail pane only** (the outline remains visible) when the validator returns findings on save. Per behavioral spec D35 and impl-decision D-31.

The panel is reachable from the session-header `findings summary` slot (clickable), or automatically when Save runs and produces fatal findings (mode 22 starting state).

## A: Findings panel with mixed severity

```text
╭── (green)Power-Ralph.9000(/) — Workflow Builder — ~/projects/foo/.pr9k/workflow/config.json ──────────────────────╮
│  (white)F(/)ile                                                                                                   │
├───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  ~/projects/foo/.pr9k/workflow/config.json (green)●(/)                                  (red)[!](/) 3 fatal · 2 warn · 1 info │
├──────────────────────────────────────┬────────────────────────────────────────────────────────────────────────────┤
│  ▾ env  (1)                          │  Findings  ·  3 fatal · 2 warn · 1 info                                    │
│    ⋮⋮ MY_TOKEN                       │                                                                            │
│    + Add env variable                │  (red)[FATAL](/) schema.steps[2].promptFile                                          │
│  ▾ containerEnv  (2)                 │          Prompt file not found: prompts/missing.md                         │
│    ⋮⋮ ANTHROPIC_API_KEY              │          (white)↩ jump to field(/)                                                       │
│    ⋮⋮ DEBUG                          │                                                                            │
│    + Add container env entry         │  (red)[FATAL](/) schema.steps[3].command                                              │
│  ▾ statusLine  (1)                   │          Script not executable: scripts/commit                             │
│    ⋮⋮ [≣] script-based               │          (white)↩ jump to field(/)  (white)mark executable(/)                                       │
│  ▾ Initialize  (1)                   │                                                                            │
│    ⋮⋮ [≡] splash      sonnet         │  (red)[FATAL](/) schema.statusLine.command                                            │
│    + Add step                        │          Path is a directory, not a file                                   │
│  ▾ Iteration  (3)                    │          (white)↩ jump to field(/)                                                       │
│ (white)> ⋮⋮ [≡] iterate     sonnet  [F1](/)│                                                                            │
│    ⋮⋮ [≡] test-plan   opus           │  (yellow)[WARN](/)  schema.steps[1].timeoutSeconds                                       │
│    ⋮⋮ [$] commit       [F1]      — │          0 disables timeout — was this intentional?                        │
│    + Add step                        │          (white)↩ jump to field(/)  (white)a acknowledge(/)                                       │
│  ▾ Finalize  (2)                     │                                                                            │
│    ⋮⋮ [≡] code-review opus  [F1]    │  (yellow)[WARN](/)  schema.env[0].name                                                  │
│    ⋮⋮ [$] update-docs             — │          Identifier MY_TOKEN shadows OS env var of the same name           │
│    + Add step                        │          (white)↩ jump to field(/)  (white)a acknowledge(/)                                       │
│                                      │                                                                            │
│                                      │  (cyan)[INFO](/)  schema.statusLine.refreshIntervalSeconds                            │
│                                      │          0 disables automatic refresh                                      │
│                                      │          (white)↩ jump to field(/)                                                       │
│                                      │                                                                            │
├──────────────────────────────────────┴────────────────────────────────────────────────────────────────────────────┤
│  (white)↑↓(/) navigate  (white)Enter(/) jump to field  (white)a(/) acknowledge  (white)Esc(/) close panel                              (white)v0.7.3(/) │
╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

- Outline pane: still visible, with `[F1]` red markers on rows that have a fatal finding attached. The user's pre-panel cursor (the `iterate` step) remains focused.
- Detail pane region: replaced by the findings panel. Header `Findings  ·  <counts>` in **white** with light-gray bullet separators.
- Each finding has three or four rows:
  1. Severity prefix in **red** / **yellow** / **cyan**, followed by the category path in **white**
  2. Problem text indented in **white**
  3. Action row indented further: `↩ jump to field` always present in **white**; `a acknowledge` for WARN/INFO; `mark executable` only on the chmod-fix row (per spec table row "missing execute bit")
- Single blank row between findings
- Footer: panel-specific shortcuts

## B: Focused finding (cursor highlight)

When the user navigates with `↑/↓`, the focused finding gets a `> ` cursor prefix and the entire entry (severity prefix, problem text, action row) renders in **white** (consistent with the outline-pane focus-rendering rule from D22):

```text
│  (red)[FATAL](/) schema.steps[2].promptFile                                          │
│          Prompt file not found: prompts/missing.md                         │
│          (white)↩ jump to field(/)                                                       │
│                                                                            │
(white)│ > [FATAL] schema.steps[3].command                                            (/)│
(white)│          Script not executable: scripts/commit                            (/)  │
(white)│          ↩ jump to field  mark executable                                 (/)  │
```

- The focused entry's `> ` prefix replaces the standard 2-space indent on the severity-prefix row
- Entire entry rendered in **white** (severity prefix, category path, problem text, action labels) — no reverse-video
- The user can press Enter to jump to the field referenced by `schema.steps[3].command`

## C: Acknowledged warning (post-`a` press)

After the user presses `a` on the focused warning:

```text
│  (yellow)[WARN ✓](/) schema.steps[1].timeoutSeconds                                       │
│          (gray)~~0 disables timeout — was this intentional?~~(/)                        │
│          (white)↩ jump to field(/)  (white)a un-acknowledge(/)                                      │
```

- Severity prefix becomes `[WARN ✓]` in **yellow**
- Problem text rendered with strikethrough in **light gray**
- Action label changes to `a un-acknowledge` so the user can revert
- The warning **remains visible** in the panel (per behavioral D23 — ack only suppresses the dialog at next save, not the panel)

## D: Empty findings (panel manually opened, but no findings)

If the user opens the findings panel from the session-header summary while no findings exist (e.g., after a fix-and-save round):

```text
│                                      │  (white)Findings  ·  no issues(/)                                                 │
│                                      │                                                                            │
│                                      │  (gray)Validation found no issues. Press Esc to close.(/)                            │
```

## E: Help modal over findings panel (only legal coexistence)

Mode 23. The help modal opens centered as usual; the underlying findings panel is rendered with a 50%-luminance dim using `Color("8")`:

```text
                ╭─ Help: Keyboard Shortcuts ────────────────────────────╮
                │                                                       │
                │  Findings panel                                       │
                │    ↑ / ↓     navigate findings                        │
                │    Enter     jump to field                            │
                │    a         acknowledge warning                      │
                │    Esc       close panel                              │
                │                                                       │
                │                              ?  close help            │
                ╰───────────────────────────────────────────────────────╯
```

- The findings panel below renders in the dimmer `Color("8")` foreground while the help modal is on top
- Only this coexistence is allowed (per impl-decision D-8 `helpOpen` only flips true when `dialog.kind == DialogFindingsPanel`)

## F: Findings panel — scroll indicator

When the panel content exceeds visible area, a single-column scroll indicator runs down the rightmost column of the detail-pane area:

```text
│                                      │  Findings  ·  12 fatal · 5 warn                                          ▲│
│                                      │                                                                          │ │
│                                      │  (red)[FATAL](/) schema.steps[5].promptFile                                       │█│
│                                      │          Prompt file not found: prompts/old.md                          │█│
│                                      │          (white)↩ jump to field(/)                                                    │█│
│                                      │                                                                          │ │
│                                      │  (red)[FATAL](/) schema.steps[6].name                                              │ │
│                                      │          Empty step name                                                 │ │
│                                      │          (white)↩ jump to field(/)                                                    │▼│
```

## G: Findings panel auto-close (all fatals resolved)

Per behavioral D35 ("when all fatals are resolved, the panel closes automatically and the save proceeds"). The closing is instantaneous; the next frame is the standard edit view with the post-save banner showing.

## Cross-references

- Behavioral spec: [Primary Flow §9](../../workflow-builder/feature-specification.md#primary-flow), [D6](../../workflow-builder/artifacts/decision-log.md#d6-validation-ux-fatal-blocks-warnings-do-not), [D23](../../workflow-builder/artifacts/decision-log.md#d23-per-session-warning-suppression), [D25](../../workflow-builder/artifacts/decision-log.md#d25-severity-text-prefixes), [D35](../../workflow-builder/artifacts/decision-log.md#d35-findings-panel-lifecycle), [D55](../../workflow-builder/artifacts/decision-log.md#d55-focus-restoration-after-findings-panel-dismiss).
- Impl decisions: [D-8](../../workflow-builder/artifacts/implementation-decision-log.md), [D-31](../../workflow-builder/artifacts/implementation-decision-log.md).
- Visual decisions: [D38](../artifacts/decision-log.md#d38-findings-panel-occupies-detail-pane-only-not-full-screen), [D39](../artifacts/decision-log.md#d39-acknowledged-warning-visual-treatment).
- Mode coverage: rows 22, 23, 27.
