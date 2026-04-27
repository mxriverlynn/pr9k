# 18 — Dialog: External Editor Opening

`DialogExternalEditorOpening` — transient dialog rendered for one Bubble Tea cycle before terminal control is yielded to the external editor. Per impl-decision D-26 two-cycle handoff.

## Render (cycle 1, just before yield)

```text
                  ╭─ Opening editor… ──────────────────────────────────────╮
                  │                                                        │
                  │   Releasing the terminal to your editor.               │
                  │                                                        │
                  │   File:    prompts/iterate.md                          │
                  │   Editor:  code --wait                                 │
                  │                                                        │
                  │   Builder will reclaim the terminal when the editor    │
                  │   exits.                                               │
                  │                                                        │
                  ╰────────────────────────────────────────────────────────╯
```

## Annotations

- Title `Opening editor…` in **white** with trailing ellipsis
- Body shows file path and resolved editor binary in **light gray** so the user can verify
- No action footer — this dialog is **transient**, displayed for ~10ms before the runtime calls `ReleaseTerminal`
- The dialog cannot be dismissed by the user (no keystrokes are accepted between cycle 1 and the editor spawn)

## Footer in this mode

```text
│  Opening editor — terminal will be released momentarily…                                            (white)v0.7.3(/) │
```

- Prompt-mode rendering: entire string in **white**

## Outcome path

After cycle 1, the Bubble Tea runtime executes the `tea.ExecProcess` cmd, calls `ReleaseTerminal`, and spawns the editor. The user then sees their editor; the builder's chrome is gone.

When the editor exits:
- Exit code 0: builder reclaims the terminal, dialog dismisses, detail pane shows the just-edited field with no warning
- Exit code 130 (SIGINT): builder reclaims, dialog dismisses, builder enters quit-confirm flow per impl-decision D-7
- Other non-zero exit: builder reclaims, dialog dismisses, brief warning banner `editor exited non-zero — file re-read anyway` appears in the session header

## Variant: editor binary failed to spawn

If the spawn itself fails (e.g., `code` is on PATH but a permission bit is wrong), the dialog transitions to `DialogEditorError` (see [`21-dialog-editor-error.md`](21-dialog-editor-error.md)) instead of releasing the terminal.

## Cross-references

- Behavioral spec: [Primary Flow §7](../../workflow-builder/feature-specification.md#primary-flow), [Alternate Flows — External-editor invocation](../../workflow-builder/feature-specification.md#external-editor-invocation), [D5](../../workflow-builder/artifacts/decision-log.md#d5-external-editor-for-multi-line-content), [D33](../../workflow-builder/artifacts/decision-log.md#d33-editor-execution-model).
- Impl decisions: [D-26](../../workflow-builder/artifacts/implementation-decision-log.md) (two-cycle handoff), [D-7](../../workflow-builder/artifacts/implementation-decision-log.md) (exit-code 130 routes to quit-confirm), [D-34](../../workflow-builder/artifacts/implementation-decision-log.md) (signal handler does not call `program.Kill` during ExecProcess).
- Visual decisions: [D44](../artifacts/decision-log.md#d44-external-editor-handoff-two-step-rendering).
