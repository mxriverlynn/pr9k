# 10 — Dialog: NewChoice

`DialogNewChoice` — three-option dialog presented after the user invokes File > New (and any unsaved-changes interception has cleared). Mode 1's transition target.

## Render (centered over edit view or empty-editor)

```text
                  ╭─ Create new workflow ──────────────────────────────────╮
                  │                                                        │
                  │   How should the new workflow start?                   │
                  │                                                        │
                  │     (white)c(/)  Copy from default workflow                       │
                  │           Loads the bundled default as your starting   │
                  │           point. You will pick a destination next.     │
                  │                                                        │
                  │     (white)e(/)  Start with empty workflow                        │
                  │           Creates a single placeholder iteration step. │
                  │                                                        │
                  │                                       [(green) Cancel (/)]              │
                  ╰────────────────────────────────────────────────────────╯
```

## Annotations

- Centered overlay per [D36](../artifacts/decision-log.md#d36-dialog-centered-overlay-shape-and-borders)
- Title `Create new workflow` in **white** in the top border
- Two-row entries for each option:
  1. `c` or `e` shortcut letter in **white** + label in **white**
  2. Indented description in **light gray** explaining the choice
- Footer: `[ Cancel ]` in **green** as the keyboard default per [D37](../artifacts/decision-log.md#d37-dialog-keyboard-default-rendering); other actions reachable by typing the shortcut letter
- Esc equivalent to Cancel (per behavioral D48 + D7)

## Footer in this mode

```text
│  (white)c(/) copy default  (white)e(/) empty workflow  (white)Esc(/) cancel                                                  (white)v0.7.3(/) │
```

## Outcome paths

- `c` — closes this dialog, opens `DialogCopyBrokenRef` if integrity check fails (see [`19-dialog-copy-broken-ref.md`](19-dialog-copy-broken-ref.md)), otherwise advances to `DialogPathPicker` with PickerKindNew
- `e` — closes this dialog, advances to `DialogPathPicker` with PickerKindNew (no integrity check needed)
- `Cancel` / `Esc` — closes the dialog and returns the user to the prior view (edit view if a workflow was loaded, empty-editor if not)

## Cross-references

- Behavioral spec: [Primary Flow §3 + Alternate Flows](../../workflow-builder/feature-specification.md#alternate-flows-and-states), [D69](../../workflow-builder/artifacts/decision-log.md#d69-file--new-flow).
- Visual decisions: [D36](../artifacts/decision-log.md#d36-dialog-centered-overlay-shape-and-borders), [D37](../artifacts/decision-log.md#d37-dialog-keyboard-default-rendering).
- Mode coverage: row 1 (transition), row 2 (Esc cancels).
