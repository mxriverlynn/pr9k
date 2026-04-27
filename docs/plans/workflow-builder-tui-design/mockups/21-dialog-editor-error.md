# 21 — Dialog: Editor Error

`DialogEditorError` — error dialog presented when the configured external editor cannot be invoked. Per behavioral D16 and D33.

## A: No editor configured

```text
                ╭─ No external editor configured ────────────────────────╮
                │                                                        │
                │   Neither $VISUAL nor $EDITOR is set.                  │
                │                                                        │
                │   Set one in your shell profile and try again:         │
                │                                                        │
                │     export VISUAL="code --wait"                        │
                │     export EDITOR="nano"                               │
                │                                                        │
                │   Your edits are still in memory. The path you wanted  │
                │   to edit is:                                          │
                │                                                        │
                │     ~/projects/foo/.pr9k/workflow/prompts/iterate.md   │
                │                                                        │
                │                                       [(green) Close (/)]              │
                ╰────────────────────────────────────────────────────────╯
```

- Title in **white**
- Body explains that the builder does not silently fall back to `vi` (per behavioral D16)
- Two-line example in **light gray** showing the user how to set `$VISUAL` / `$EDITOR`
- The path is provided so the user can paste it into their editor manually
- Single action: Close (keyboard default)

## B: Editor binary not on PATH

```text
                ╭─ Editor not found ─────────────────────────────────────╮
                │                                                        │
                │   The configured editor "code" was not found on $PATH. │
                │                                                        │
                │   Configured value: code --wait                        │
                │                                                        │
                │   Your edits are still in memory. Fix your $VISUAL or  │
                │   $EDITOR and try again.                               │
                │                                                        │
                │     (white)r(/)  Retry          re-resolve and try to launch       │
                │     (white)c(/)  Cancel         close this dialog and continue     │
                │                                                        │
                │                       Retry   [(green) Cancel (/)]                  │
                ╰────────────────────────────────────────────────────────╯
```

## C: Editor value contains shell metacharacters

```text
                ╭─ Editor value rejected ────────────────────────────────╮
                │                                                        │
                │   The configured editor contains a shell               │
                │   metacharacter that is not allowed.                   │
                │                                                        │
                │   Configured value: nano && echo done                  │
                │   Rejected because: contains the character "&"         │
                │                                                        │
                │   The builder does not invoke editors through a shell. │
                │   Set $VISUAL to a single command with optional        │
                │   space-separated arguments, e.g.:                     │
                │                                                        │
                │     export VISUAL="code --wait"                        │
                │                                                        │
                │   Your edits are still in memory.                      │
                │                                                        │
                │     (white)r(/)  Retry          re-resolve and try to launch       │
                │     (white)c(/)  Cancel         close this dialog and continue     │
                │                                                        │
                │                       Retry   [(green) Cancel (/)]                  │
                ╰────────────────────────────────────────────────────────╯
```

## D: Editor spawn failed (e.g., permission)

```text
                ╭─ Could not launch editor ──────────────────────────────╮
                │                                                        │
                │   Could not launch the external editor.                │
                │                                                        │
                │   Editor:    /usr/local/bin/code                       │
                │   OS error:  permission denied                         │
                │                                                        │
                │   Your edits are still in memory.                      │
                │                                                        │
                │     (white)r(/)  Retry          re-resolve and try to launch       │
                │     (white)c(/)  Cancel         close this dialog and continue     │
                │                                                        │
                │                       Retry   [(green) Cancel (/)]                  │
                ╰────────────────────────────────────────────────────────╯
```

## Annotations across all four variants

- Title varies by failure category, always in **white**
- Body opens with the user-visible problem
- Configured value(s) shown for verification, in **light gray** preformatted lines where applicable
- "Your edits are still in memory" reassurance from the four-element error template (D56)
- Action footer: Retry (where applicable, in **white**), `[ Cancel ]` (green default) — variant A only has Close as the single action
- Esc equivalent to Cancel / Close

## Footer in this mode

```text
│  (white)r(/) retry  (white)c(/) cancel  (white)Esc(/) cancel                                                                       (white)v0.7.3(/) │
```

## Cross-references

- Behavioral spec: [Alternate Flows — Editor binary cannot be spawned](../../workflow-builder/feature-specification.md#editor-binary-cannot-be-spawned), [Alternate Flows — No external editor configured](../../workflow-builder/feature-specification.md#no-external-editor-configured), [D16](../../workflow-builder/artifacts/decision-log.md#d16-external-editor-fallback-policy), [D33](../../workflow-builder/artifacts/decision-log.md#d33-editor-execution-model), [D56](../../workflow-builder/artifacts/decision-log.md#d56-error-message-template-for-edge-case-failures).
- Impl decisions: [D-22](../../workflow-builder/artifacts/implementation-decision-log.md) (shlex parsing).
