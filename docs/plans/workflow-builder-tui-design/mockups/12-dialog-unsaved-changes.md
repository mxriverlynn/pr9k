# 12 — Dialog: Unsaved Changes

`DialogUnsavedChanges` — three-option dialog presented when the user invokes File > Quit, File > New, or File > Open while the in-memory state is dirty. Mode 26 starting state.

## Render

```text
                    ╭─ Unsaved changes ────────────────────────────────────╮
                    │                                                      │
                    │   You have unsaved changes to                        │
                    │   ~/projects/foo/.pr9k/workflow/config.json          │
                    │                                                      │
                    │   What should happen before continuing?              │
                    │                                                      │
                    │     (white)s(/)  Save        write changes and continue        │
                    │     (white)c(/)  Cancel      stay in the current session       │
                    │     (white)d(/)  Discard     drop changes and continue         │
                    │                                                      │
                    │              Save   [(green) Cancel (/)]   Discard               │
                    ╰──────────────────────────────────────────────────────╯
```

## Annotations

- Title `Unsaved changes` in **white**
- Body lists the target path in **white** so the user knows which workflow they're discarding
- Three lettered actions, each in **white** + label in **white** + description in **light gray**
- Footer right-aligned with the three options spatially ordered Save / Cancel / Discard per behavioral D7 + Dialog Conventions: Save (primary safe), Cancel (secondary safe — keyboard default), Discard (destructive — last)
- `[ Cancel ]` in **green** as the keyboard default

## Footer in this mode

```text
│  (white)s(/) save  (white)c(/) cancel  (white)d(/) discard  (white)Esc(/) cancel                                                  (white)v0.7.3(/) │
```

## Outcome paths

- `s` / Save — runs validation; if fatals, opens findings panel and cancels the pending action (per D40); if save succeeds, the pending action (Quit / New / Open) resumes automatically (per D72)
- `c` / Cancel / Esc / Enter — closes the dialog, returns to the current session unchanged
- `d` / Discard — discards in-memory state and resumes the pending action (single-step per R3 simplification of D54)

## Variant: pending action is File > New / File > Open

The dialog body adjusts to mention what comes next:

```text
                    ╭─ Unsaved changes ────────────────────────────────────╮
                    │                                                      │
                    │   You have unsaved changes to                        │
                    │   ~/projects/foo/.pr9k/workflow/config.json          │
                    │                                                      │
                    │   What should happen before opening another          │
                    │   workflow?                                          │
                    │                                                      │
                    │     (white)s(/)  Save        write changes and open           │
                    │     (white)c(/)  Cancel      stay in the current workflow    │
                    │     (white)d(/)  Discard     drop changes and open            │
                    │                                                      │
                    │              Save   [(green) Cancel (/)]   Discard               │
                    ╰──────────────────────────────────────────────────────╯
```

## Variant: Save with fatals (post-save state)

After the user picks Save and validation produces fatals, the dialog closes and the findings panel opens; the pending action (Quit / New / Open) is cancelled (per D40):

See [`09-findings-panel.md`](09-findings-panel.md) for the post-Save-with-fatals state.

## Cross-references

- Behavioral spec: [Primary Flow §10](../../workflow-builder/feature-specification.md#primary-flow), [Alternate Flows — Unsaved-changes interception](../../workflow-builder/feature-specification.md#unsaved-changes-interception-quit-file--new-file--open), [D7](../../workflow-builder/artifacts/decision-log.md#d7-save-semantics-explicit-atomic-unsaved-prompt), [D40](../../workflow-builder/artifacts/decision-log.md#d40-unsaved-quit-compound-state), [D72](../../workflow-builder/artifacts/decision-log.md#d72-unsaved-changes-interception-and-resume-semantics-for-file--new--file--open).
- Mode coverage: rows 26, 27.
