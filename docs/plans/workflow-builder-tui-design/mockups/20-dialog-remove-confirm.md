# 20 — Dialog: Remove Confirm

`DialogRemoveConfirm` — two-option confirmation presented when the user invokes `Del` (or click on the focused row's trash glyph) on a removable outline row. Mode 8 transition target. Per behavioral D30 and impl-decision D-30.

## A: Remove step confirmation

```text
                       ╭─ Delete step? ─────────────────────────────╮
                       │                                            │
                       │   Delete step "iterate" from the           │
                       │   Iteration phase?                         │
                       │                                            │
                       │   The step's prompt file (prompts/         │
                       │   iterate.md) will not be deleted.         │
                       │                                            │
                       │             (white)d(/)  Delete                       │
                       │             (white)c(/)  Cancel                       │
                       │                                            │
                       │                Delete   [(green) Cancel (/)]            │
                       ╰────────────────────────────────────────────╯
```

## Annotations

- Title `Delete step?` in **white** with question mark
- Body names the specific item being removed (step name, phase) so the user can verify
- Subordinate sentence in **light gray** clarifying that the prompt file on disk is untouched
- Two lettered actions: `d` Delete (destructive), `c` Cancel (safe — keyboard default)
- Footer right-aligned: Delete (in **white**), `[ Cancel ]` (green default)

## B: Remove env variable confirmation

```text
                       ╭─ Delete entry? ────────────────────────────╮
                       │                                            │
                       │   Delete env variable "MY_TOKEN"?          │
                       │                                            │
                       │             (white)d(/)  Delete                       │
                       │             (white)c(/)  Cancel                       │
                       │                                            │
                       │                Delete   [(green) Cancel (/)]            │
                       ╰────────────────────────────────────────────╯
```

## C: Remove containerEnv entry confirmation

```text
                       ╭─ Delete entry? ────────────────────────────╮
                       │                                            │
                       │   Delete container env entry              │
                       │   "ANTHROPIC_API_KEY"?                     │
                       │                                            │
                       │             (white)d(/)  Delete                       │
                       │             (white)c(/)  Cancel                       │
                       │                                            │
                       │                Delete   [(green) Cancel (/)]            │
                       ╰────────────────────────────────────────────╯
```

## D: Remove statusLine block confirmation

```text
                       ╭─ Delete statusLine block? ─────────────────╮
                       │                                            │
                       │   Delete the statusLine block?             │
                       │                                            │
                       │   The block's command script (scripts/     │
                       │   statusline) will not be deleted.         │
                       │                                            │
                       │             (white)d(/)  Delete                       │
                       │             (white)c(/)  Cancel                       │
                       │                                            │
                       │                Delete   [(green) Cancel (/)]            │
                       ╰────────────────────────────────────────────╯
```

## Footer in this mode

```text
│  (white)d(/) delete  (white)c(/) cancel  (white)Esc(/) cancel  (white)Enter(/) cancel                                                          (white)v0.7.3(/) │
```

## Outcome paths

- `d` / Delete — removes the item from the in-memory model; the dirty indicator turns on (`●` in green); cursor falls through to the next item per the Edge Cases table behavior
- `c` / Cancel / Esc / Enter — closes the dialog, no state change

## Cross-references

- Behavioral spec: [Primary Flow §7](../../workflow-builder/feature-specification.md#primary-flow), [Edge Cases — Focused step is removed](../../workflow-builder/feature-specification.md#edge-cases-and-failure-modes), [D30](../../workflow-builder/artifacts/decision-log.md#d30-read-only-targets-open-in-browse-only-mode), [D48](../../workflow-builder/artifacts/decision-log.md#d48-dialog-convention-standard).
- Impl decisions: [D-10](../../workflow-builder/artifacts/implementation-decision-log.md) (`Del` scoped to outline focus), [D-28](../../workflow-builder/artifacts/implementation-decision-log.md), [D-30](../../workflow-builder/artifacts/implementation-decision-log.md).
- Mode coverage: row 8.
