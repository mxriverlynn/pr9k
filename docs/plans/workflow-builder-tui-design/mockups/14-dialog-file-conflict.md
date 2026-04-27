# 14 — Dialog: File Conflict

`DialogFileConflict` — three-option dialog presented at save time when the on-disk file has changed since the builder loaded it (mtime + size mismatch detected per behavioral D41).

## Render

```text
                ╭─ Configuration file changed on disk ───────────────────╮
                │                                                        │
                │   The configuration file has been modified since this  │
                │   builder loaded it.                                   │
                │                                                        │
                │   Path:    ~/projects/foo/.pr9k/workflow/config.json   │
                │   On disk: modified 2026-04-26 14:35:12 (1,232 bytes)  │
                │   In mem:  loaded   2026-04-26 14:30:01 (1,197 bytes)  │
                │                                                        │
                │   Your edits are still in memory. No changes were      │
                │   written.                                             │
                │                                                        │
                │     (white)o(/)  Overwrite   replace disk content with your edits │
                │     (white)r(/)  Reload      discard your edits and reload disk   │
                │     (white)c(/)  Cancel      keep both — fix manually later       │
                │                                                        │
                │              Overwrite   Reload   [(green) Cancel (/)]               │
                ╰────────────────────────────────────────────────────────╯
```

## Annotations

- Title `Configuration file changed on disk` in **white**
- Body opens with the user-visible problem (one sentence, no jargon)
- A three-row state block in **light gray** giving exact times and sizes — anchored to columns so the user can compare on-disk and in-mem at a glance
- "In-memory state commitment" sentence per the four-element error template (D56)
- Three lettered actions; the `(c)ancel` is keyboard default per D48 (Cancel is the safe option in a destructive choice)
- Footer right-aligned with Overwrite (destructive primary), Reload (destructive secondary), Cancel (safe — green default)

## Footer in this mode

```text
│  (white)o(/) overwrite  (white)r(/) reload  (white)c(/) cancel  (white)Esc(/) cancel                                                  (white)v0.7.3(/) │
```

## Outcome paths

- `o` / Overwrite — saves anyway, replacing the disk content; the in-memory state becomes the new file content; D41 mtime snapshot is refreshed
- `r` / Reload — discards the in-memory state and reloads the on-disk version; the user's edits are lost; if parsing fails on reload, opens `DialogRecovery`
- `c` / Cancel / Esc / Enter — closes the dialog without saving or reloading; the user can manually inspect both versions

## Variant: filesystem with second-level mtime granularity

When the underlying filesystem stores mtime at second resolution (FAT32, HFS+ on older macOS), the snapshot comparison can produce false positives. The dialog body is unchanged; the documented limitation in the how-to guide informs the user that the conflict detection is best-effort.

## Cross-references

- Behavioral spec: [Edge Cases](../../workflow-builder/feature-specification.md#edge-cases-and-failure-modes) row "Configuration file is modified on disk since the builder loaded it", [D41](../../workflow-builder/artifacts/decision-log.md#d41-cross-session-mutation-detection), [D56](../../workflow-builder/artifacts/decision-log.md#d56-error-message-template-for-edge-case-failures).
- Impl decisions: [D-13](../../workflow-builder/artifacts/implementation-decision-log.md), [D-14](../../workflow-builder/artifacts/implementation-decision-log.md).
- Visual decisions: [D36](../artifacts/decision-log.md#d36-dialog-centered-overlay-shape-and-borders), [D37](../artifacts/decision-log.md#d37-dialog-keyboard-default-rendering).
