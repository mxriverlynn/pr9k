# 23 — Dialog: Save In Progress

`DialogSaveInProgress` — non-dismissable transient dialog presented when the user invokes `Ctrl+Q` while a save is already in flight (mode-coverage rows 20–21). Per F13's resolution: this dialog is committed by the mode-coverage table but was missing from the original `DialogKind` enumeration; this visual spec adds it.

The dialog also appears as the user-facing surface for the validate-then-save sequence's intermediate states (mode 18's `validateInProgress` and `saveInProgress` legs) when the operation takes long enough to be noticed by the user (per F17 / GAP-003).

## A: Save in progress (mode 20: Ctrl+Q during active save)

```text
                       ╭─ Save in progress ─────────────────────────╮
                       │                                            │
                       │   A save is in progress. The builder will  │
                       │   complete the quit when the save returns. │
                       │                                            │
                       │   Saving ~/.pr9k/workflow/config.json…     │
                       │                                            │
                       │   (cannot be dismissed)                    │
                       │                                            │
                       ╰────────────────────────────────────────────╯
```

- Title `Save in progress` in **white**
- Body explains the deferred-quit behavior: the user's Ctrl+Q is queued and will fire after the save returns
- Dialog is **not dismissible** by user input — there's no Cancel option, no Esc handler. The dialog automatically dismisses when the `saveCompleteMsg` arrives
- After dismissal, the builder routes to the appropriate next step: if the post-save state is clean, opens `DialogQuitConfirm`; if dirty (rare — user edited during the in-flight save), opens `DialogUnsavedChanges` (per mode-coverage row 21)

## B: Validating phase (mode 18 first leg)

When validation alone takes more than a fraction of a second, the dialog appears with a different title:

```text
                       ╭─ Validating ───────────────────────────────╮
                       │                                            │
                       │   Running validation against the in-memory │
                       │   workflow.                                │
                       │                                            │
                       │   This usually takes a moment…             │
                       │                                            │
                       │   (cannot be dismissed)                    │
                       │                                            │
                       ╰────────────────────────────────────────────╯
```

- Title `Validating` in **white**
- Body explains the operation
- Same non-dismissible behavior; auto-dismisses when validation returns

## C: Saving phase (mode 18 second leg, when save takes time)

```text
                       ╭─ Saving ───────────────────────────────────╮
                       │                                            │
                       │   Writing the workflow bundle to disk.     │
                       │                                            │
                       │   Saving ~/.pr9k/workflow/config.json…     │
                       │                                            │
                       │   (cannot be dismissed)                    │
                       │                                            │
                       ╰────────────────────────────────────────────╯
```

- Title `Saving` in **white**
- Same non-dismissible behavior

## Footer in these modes

```text
│  Saving — please wait…                                                                              (white)v0.7.3(/) │
```

or

```text
│  Validating…                                                                                        (white)v0.7.3(/) │
```

- Prompt-mode rendering: entire string in **white** (per [D34](../artifacts/decision-log.md#d34-status-footer-shortcut-vs-prompt-rendering-rule))

## Outcome paths

These dialogs are auto-dismissed by the runtime; the user's keystrokes are consumed but produce no visible effect (mode-coverage row 20 explicitly states "no new goroutine spawned; state unchanged" for the save-gated case).

After auto-dismiss:
- `DialogValidating` → if findings, opens `DialogFindingsPanel`; if no findings and no changes, dismisses with `No changes to save` banner; if no findings and changes, transitions to `DialogSaving`
- `DialogSaving` → on success, dismisses with `Saved at HH:MM:SS` banner and clears dirty indicator; on failure, transitions to a `DialogError` variant per the four-element error template

## Cross-references

- Behavioral spec: [Primary Flow §9](../../workflow-builder/feature-specification.md#primary-flow), [User Interactions — Feedback](../../workflow-builder/feature-specification.md#user-interactions).
- Mode coverage: rows 18, 19, 20, 21.
- Visual decisions: [D36](../artifacts/decision-log.md#d36-dialog-centered-overlay-shape-and-borders), [D44](../artifacts/decision-log.md#d44-external-editor-handoff-two-step-rendering).
- Team findings: [F13](../artifacts/team-findings.md#f13-dialogsaveinprogress-is-missing--no-dialogkind-constant-no-mockup), [F17](../artifacts/team-findings.md#f17-validating-transient-state-has-no-full-frame-mockup).
