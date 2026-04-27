# 22 — Dialog: Generic Error (D56 Four-Element Template)

`DialogError` — the generic error dialog that every Edge-Cases-table failure renders into. Per behavioral D56's four-element error template.

The four elements:

1. **What happened** — single sentence in user-visible vocabulary
2. **Why** — short phrase naming the condition (OS error, file path, validator finding)
3. **In-memory state commitment** — explicit "Your edits are still in memory" or "No edits were lost"
4. **Available action** — labeled options from the D48 lexicon, safe option keyboard default

## A: Disk full at save

```text
                ╭─ Could not save workflow ──────────────────────────────╮
                │                                                        │
                │   Could not write the configuration file.              │
                │                                                        │
                │   Path:     ~/projects/foo/.pr9k/workflow/config.json  │
                │   OS error: no space left on device                    │
                │                                                        │
                │   Your edits are still in memory. The previous version │
                │   of the file on disk is unchanged.                    │
                │                                                        │
                │     (white)r(/)  Retry          re-attempt the save                │
                │     (white)c(/)  Cancel         return to edit view                 │
                │                                                        │
                │                       Retry   [(green) Cancel (/)]                  │
                ╰────────────────────────────────────────────────────────╯
```

## B: Permission denied at save

```text
                ╭─ Could not save workflow ──────────────────────────────╮
                │                                                        │
                │   Could not write the configuration file.              │
                │                                                        │
                │   Path:     /usr/local/share/pr9k/workflow/config.json │
                │   OS error: permission denied                          │
                │                                                        │
                │   Your edits are still in memory. The previous version │
                │   of the file on disk is unchanged.                    │
                │                                                        │
                │     (white)r(/)  Retry          re-attempt the save                │
                │     (white)c(/)  Cancel         return to edit view                 │
                │                                                        │
                │                       Retry   [(green) Cancel (/)]                  │
                ╰────────────────────────────────────────────────────────╯
```

## C: Cross-device rename (EXDEV)

```text
                ╭─ Could not save workflow ──────────────────────────────╮
                │                                                        │
                │   Save failed — target is on a different filesystem    │
                │   than the builder's scratch directory.                │
                │                                                        │
                │   Path:     ~/.shared/wf/config.json                   │
                │   Cause:    POSIX atomic rename requires the temp file │
                │             and target to share a filesystem.          │
                │                                                        │
                │   Your edits are still in memory. No changes were      │
                │   written to disk.                                     │
                │                                                        │
                │     (white)r(/)  Retry          re-attempt the save                │
                │     (white)c(/)  Cancel         return to edit view                 │
                │                                                        │
                │                       Retry   [(green) Cancel (/)]                  │
                ╰────────────────────────────────────────────────────────╯
```

## D: Target directory deleted during session

```text
                ╭─ Target directory missing ─────────────────────────────╮
                │                                                        │
                │   The workflow's directory no longer exists.           │
                │                                                        │
                │   Path:     ~/projects/foo/.pr9k/workflow/             │
                │   Cause:    directory was removed or renamed during    │
                │             this session                               │
                │                                                        │
                │   Your edits are still in memory. Restore the          │
                │   directory and retry, or quit and pick another        │
                │   target.                                              │
                │                                                        │
                │     (white)r(/)  Retry          re-attempt the save                │
                │     (white)c(/)  Cancel         return to edit view                 │
                │                                                        │
                │                       Retry   [(green) Cancel (/)]                  │
                ╰────────────────────────────────────────────────────────╯
```

## E: Companion file missing on external-editor return

```text
                ╭─ Referenced file no longer exists ─────────────────────╮
                │                                                        │
                │   Could not re-read the prompt file after editor       │
                │   exit.                                                │
                │                                                        │
                │   Path:     prompts/iterate.md                         │
                │   Cause:    file was renamed or deleted during the     │
                │             editor session (Save As?)                  │
                │                                                        │
                │   Your edits are still in memory. The reference in     │
                │   the configuration still points to the original path  │
                │   — update it manually if you saved to a new path.     │
                │                                                        │
                │                                       [(green) Close (/)]              │
                ╰────────────────────────────────────────────────────────╯
```

## Annotations

- Every variant follows the same four-element shape
- Title varies but always names the operation that failed
- Path / OS-error / Cause rendered as label-value lines in **light gray**
- Two lettered actions in most variants (Retry / Cancel); one action (Close) for non-retriable cases
- Footer right-aligned with action labels; `[ Cancel ]` or `[ Close ]` in **green** as the keyboard default

## Footer in these modes

```text
│  (white)r(/) retry  (white)c(/) cancel  (white)Esc(/) cancel                                                                       (white)v0.7.3(/) │
```

or, for non-retriable variants:

```text
│  (white)Enter(/) close  (white)Esc(/) close                                                                                  (white)v0.7.3(/) │
```

## Cross-references

- Behavioral spec: [Edge Cases](../../workflow-builder/feature-specification.md#edge-cases-and-failure-modes), [D56](../../workflow-builder/artifacts/decision-log.md#d56-error-message-template-for-edge-case-failures), [D48](../../workflow-builder/artifacts/decision-log.md#d48-dialog-convention-standard).
- Impl decisions: [D-19](../../workflow-builder/artifacts/implementation-decision-log.md) (EXDEV via D56 template), [D-47](../../workflow-builder/artifacts/implementation-decision-log.md) (`SaveResult` typed error kinds).
