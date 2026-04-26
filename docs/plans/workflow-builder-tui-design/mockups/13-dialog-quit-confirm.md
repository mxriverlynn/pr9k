# 13 — Dialog: Quit Confirm

`DialogQuitConfirm` — two-option dialog presented when the user invokes File > Quit and the in-memory state is **clean** (no unsaved changes). Mode 24 starting state. Per behavioral D73.

## Render

```text
                       ╭─ Quit ─────────────────────────────────────╮
                       │                                            │
                       │   Quit the workflow builder?               │
                       │                                            │
                       │                  (white)y(/)  Yes — exit              │
                       │                  (white)n(/)  No — stay              │
                       │                                            │
                       │                       Yes   [(green) No (/)]            │
                       ╰────────────────────────────────────────────╯
```

## Annotations

- Title `Quit` in **white**
- Body: a single question in **white**
- Two lettered actions; descriptions in **light gray**
- Footer right-aligned: Yes (left), `[ No ]` (right, green default)
- Per behavioral D73, `No` is the keyboard default — `Enter` cancels the quit
- `Esc` is equivalent to `No` (per D48)

## Footer in this mode

```text
│  Quit the workflow builder? (y/n, esc to cancel)                                                    (white)v0.7.3(/) │
```

- Prompt-mode rendering: entire string in **white** (per [D34](../artifacts/decision-log.md#d34-status-footer-shortcut-vs-prompt-rendering-rule))

## Outcome paths

- `y` / Yes — `program.Quit` returned; builder exits (mode 25)
- `n` / No / Esc / Enter — closes the dialog, returns to the prior view

## Variant: empty-editor + Quit

When invoked from empty-editor (no workflow loaded), the dialog text adjusts:

```text
                       ╭─ Quit ─────────────────────────────────────╮
                       │                                            │
                       │   Quit the workflow builder?               │
                       │                                            │
                       │                  (white)y(/)  Yes — exit              │
                       │                  (white)n(/)  No — stay              │
                       │                                            │
                       │                       Yes   [(green) No (/)]            │
                       ╰────────────────────────────────────────────╯
```

(No body change — the dialog is identical regardless of whether a workflow is loaded.)

## Cross-references

- Behavioral spec: [Primary Flow §10](../../workflow-builder/feature-specification.md#primary-flow), [Alternate Flows — Quit confirmation (no unsaved changes)](../../workflow-builder/feature-specification.md#quit-confirmation-no-unsaved-changes), [D73](../../workflow-builder/artifacts/decision-log.md#d73-quit-confirmation-always-required).
- Visual decisions: [D34](../artifacts/decision-log.md#d34-status-footer-shortcut-vs-prompt-rendering-rule), [D37](../artifacts/decision-log.md#d37-dialog-keyboard-default-rendering).
- Mode coverage: rows 24, 25.
