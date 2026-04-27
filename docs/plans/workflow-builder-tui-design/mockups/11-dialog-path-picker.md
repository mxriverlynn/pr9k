# 11 — Dialog: Path Picker

`DialogPathPicker` — single-text-input file/directory picker with filesystem tab-completion. Used by File > New (PickerKindNew) and File > Open (PickerKindOpen). Mode 3 starting state.

## A: PickerKindOpen — initial render

```text
            ╭─ Open workflow file ───────────────────────────────────────────────╮
            │                                                                    │
            │   Path: [/Users/me/projects/foo/.pr9k/workflow/config.json(white)▏(/)        ]    │
            │                                                                    │
            │   tab completes  ·  enter opens  ·  esc cancels                    │
            │                                                                    │
            │                                              [(green) Cancel (/)]  Open    │
            ╰────────────────────────────────────────────────────────────────────╯
```

- Title `Open workflow file` in **white**
- Path input pre-filled with `<projectDir>/.pr9k/workflow/config.json`, cursor at end
- Hint row in **light gray**
- Footer right-aligned: `[ Cancel ]` (green default) and `Open` in **white**

## B: PickerKindNew — initial render

```text
            ╭─ Where should the new workflow be saved? ──────────────────────────╮
            │                                                                    │
            │   Path: [/Users/me/projects/foo/.pr9k/workflow/(white)▏(/)                    ]   │
            │                                                                    │
            │   tab completes  ·  enter creates  ·  esc cancels                  │
            │                                                                    │
            │                                              [(green) Cancel (/)]  Create  │
            ╰────────────────────────────────────────────────────────────────────╯
```

- Title `Where should the new workflow be saved?` in **white**
- Pre-filled with `<projectDir>/.pr9k/workflow/` (note trailing slash — picker targets a directory)
- Footer action: `Create`

## C: Multiple-match completion (Tab pressed)

When the user types `/Users/me/projects/foo/.pr9k/wf` and presses Tab, multiple matches surface inline:

```text
            ╭─ Open workflow file ───────────────────────────────────────────────╮
            │                                                                    │
            │   Path: [/Users/me/projects/foo/.pr9k/wf(white)▏(/)                          ]   │
            │                                                                    │
            │   tab completes  ·  enter opens  ·  esc cancels                    │
            │                                                                    │
            │   (gray)[hint] matches: workflow/  workflow-archive/  workflow-old/(/)       │
            │                                                                    │
            │                                              [(green) Cancel (/)]  Open    │
            ╰────────────────────────────────────────────────────────────────────╯
```

- Inline match list in **light gray** beneath the hint row
- Repeated Tab cycles through matches; each cycle replaces the input with the next match and shows `(N/M)` counter:

```text
            │   Path: [/Users/me/projects/foo/.pr9k/workflow(white)▏(/)                    ]   │
            │   tab completes  ·  enter opens  ·  esc cancels                    │
            │   (gray)[hint] matches: workflow/ ← (1/3)  workflow-archive/  workflow-old/(/)│
```

## D: Single-match auto-completion

```text
            ╭─ Open workflow file ───────────────────────────────────────────────╮
            │                                                                    │
            │   Path: [/Users/me/projects/foo/.pr9k/workflow/(white)▏(/)                    ]   │
            │                                                                    │
            │   tab completes  ·  enter opens  ·  esc cancels                    │
            │                                                                    │
            │   (gray)[hint] completed to single match(/)                                   │
            │                                                                    │
            │                                              [(green) Cancel (/)]  Open    │
            ╰────────────────────────────────────────────────────────────────────╯
```

- The path is auto-completed with no cycling
- Brief feedback `completed to single match` in **light gray**

## E: Inline warning — typed path is a directory (PickerKindOpen)

```text
            ╭─ Open workflow file ───────────────────────────────────────────────╮
            │                                                                    │
            │   Path: [/Users/me/projects/foo/.pr9k/workflow/(white)▏(/)                    ]   │
            │                                                                    │
            │   tab completes  ·  enter opens  ·  esc cancels                    │
            │                                                                    │
            │   (yellow)[warn](/) (yellow)↳ that is a directory — add /config.json to open it(/)        │
            │                                                                    │
            │                                              [(green) Cancel (/)]  Open    │
            ╰────────────────────────────────────────────────────────────────────╯
```

- Inline-warning row in **yellow** with `↳` glyph
- Open button visually disabled until the warning clears

## F: Inline warning — typed path does not exist (PickerKindOpen)

```text
            │   (yellow)[warn](/) (yellow)↳ no config.json at that path — use File > New to create one(/)│
```

## G: Inline warning — destination already contains a config.json (PickerKindNew)

```text
            ╭─ Where should the new workflow be saved? ──────────────────────────╮
            │                                                                    │
            │   Path: [/Users/me/projects/foo/.pr9k/workflow/(white)▏(/)                    ]   │
            │                                                                    │
            │   tab completes  ·  enter creates  ·  esc cancels                  │
            │                                                                    │
            │   (yellow)[warn](/) (yellow)↳ a config.json already exists here — Create overwrites on save(/)│
            │                                                                    │
            │                                              [(green) Cancel (/)]  Create  │
            ╰────────────────────────────────────────────────────────────────────╯
```

- Warning is non-blocking: user can still proceed (atomic-rename ensures no torn write)

## H: `~` expansion (typed by user)

When the user types `~/wf` and presses Tab:

```text
            │   Path: [/Users/me/wf(white)▏(/)                                              ]   │
```

- The `~` is expanded to `os.UserHomeDir()` before completion

## Cross-references

- Behavioral spec: [Primary Flow §3, §4, Alternate Flows](../../workflow-builder/feature-specification.md#alternate-flows-and-states), [D71](../../workflow-builder/artifacts/decision-log.md#d71-path-picker-design).
- Impl decisions: [D-25](../../workflow-builder/artifacts/implementation-decision-log.md) (custom minimal `pathcomplete`).
- Visual decisions: [D42](../artifacts/decision-log.md#d42-path-picker-input-line-and-completion-behavior).
- Mode coverage: row 3.
