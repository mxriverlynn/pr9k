# 17 — Dialog: First-Save Confirm

`DialogFirstSaveConfirm` — two-option confirmation dialog presented at the **first save** of a session whose target is either an external workflow (per D22) or contains a symlink (per D17). Subsequent saves in the same session do not re-confirm.

## A: External-workflow first save

```text
                ╭─ Confirm first save (external workflow) ───────────────╮
                │                                                        │
                │   The target is outside your project and home          │
                │   directories.                                         │
                │                                                        │
                │   Path: /tmp/scratch/wf/config.json                    │
                │                                                        │
                │   Save will write here. This is uncommon — confirm     │
                │   that you intended this location.                     │
                │                                                        │
                │     (white)s(/)  Save           proceed with the save             │
                │     (white)c(/)  Cancel         do not save and stay              │
                │                                                        │
                │                       Save   [(green) Cancel (/)]                  │
                ╰────────────────────────────────────────────────────────╯
```

## B: Symlinked target — first save

```text
                ╭─ Confirm first save (symlinked target) ────────────────╮
                │                                                        │
                │   The configuration file is a symbolic link.           │
                │                                                        │
                │   Path:        ~/projects/foo/.pr9k/workflow/config.json│
                │   Resolves to: ~/.shared/wf/config.json                │
                │                                                        │
                │   Save will write to the resolved file (the symlink    │
                │   is left in place).                                   │
                │                                                        │
                │     (white)s(/)  Save           proceed with the save             │
                │     (white)c(/)  Cancel         do not save and stay              │
                │                                                        │
                │                       Save   [(green) Cancel (/)]                  │
                ╰────────────────────────────────────────────────────────╯
```

## C: Both (external + symlink)

When the resolved target is both outside the trusted area AND reached through a symlink:

```text
                ╭─ Confirm first save (external symlink target) ─────────╮
                │                                                        │
                │   The configuration file is a symbolic link AND        │
                │   resolves outside your project and home directories.  │
                │                                                        │
                │   Path:        ~/projects/foo/.pr9k/workflow/config.json│
                │   Resolves to: /tmp/scratch/wf/config.json             │
                │                                                        │
                │   Save will write to the resolved file. This is        │
                │   uncommon — confirm that you intended this location.  │
                │                                                        │
                │     (white)s(/)  Save           proceed with the save             │
                │     (white)c(/)  Cancel         do not save and stay              │
                │                                                        │
                │                       Save   [(green) Cancel (/)]                  │
                ╰────────────────────────────────────────────────────────╯
```

## Annotations

- Title varies with the trigger condition (external / symlink / both)
- Body always shows the relevant path(s) so the user can verify
- "Save will write to …" sentence explains the consequence
- Two lettered actions; Cancel is keyboard default
- Footer right-aligned: Save (in **white**), `[ Cancel ]` (green default)

## Footer in this mode

```text
│  (white)s(/) save  (white)c(/) cancel  (white)Esc(/) cancel  (white)Enter(/) cancel                                                          (white)v0.7.3(/) │
```

## Outcome paths

- `s` / Save — proceeds with the save; subsequent saves in the same session do not re-confirm (the confirmation is per-session)
- `c` / Cancel / Esc / Enter — closes the dialog without saving; the user remains in edit view with the dirty state intact

## Cross-references

- Behavioral spec: [Alternate Flows — External-workflow session](../../workflow-builder/feature-specification.md#external-workflow-session-target-outside-users-project-and-home), [Alternate Flows — Symlinked target or companion file](../../workflow-builder/feature-specification.md#symlinked-target-or-companion-file), [D17](../../workflow-builder/artifacts/decision-log.md#d17-symlink-policy--follow-with-visibility), [D22](../../workflow-builder/artifacts/decision-log.md#d22-external-workflow-warning).
