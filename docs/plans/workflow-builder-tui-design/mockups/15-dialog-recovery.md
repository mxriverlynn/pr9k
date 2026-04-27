# 15 — Dialog: Recovery (Parse-Error Recovery View)

`DialogRecovery` — replaces the edit view when a target `config.json` exists but cannot be parsed. Mode 28 starting state.

The recovery view occupies the entire pane area (both outline and detail panes), unlike the findings panel which only takes the detail pane. The recovery view is structurally a dialog because it has the same overlay-with-actions shape, but it sits in the pane area rather than as a centered overlay.

## A: Render — JSON parse error in target file

```text
╭── (green)Power-Ralph.9000(/) — Workflow Builder — ~/projects/foo/.pr9k/workflow/config.json ──────────────────────╮
│  (white)F(/)ile                                                                                                   │
├───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  ~/projects/foo/.pr9k/workflow/config.json    (red)[parse error](/)                                                 │
├───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                                                   │
│   (white)Could not parse config.json(/)                                                                              │
│                                                                                                                   │
│   (red)Parse error at line 14, column 23: invalid character '}' looking for beginning of object key string(/)        │
│                                                                                                                   │
│   (white)Raw file content (first 8 KB, ANSI escapes stripped):(/)                                                    │
│                                                                                                                   │
│   (gray)──────────────────────────────────────────────────────────────────────────────────────────────────(/)         │
│   (gray)  1 │ {(/)                                                                                                  │
│   (gray)  2 │   "defaultModel": "claude-sonnet-4-6",(/)                                                              │
│   (gray)  3 │   "steps": [(/)                                                                                        │
│   (gray)  …(/)                                                                                                       │
│   (gray) 13 │     {(/)                                                                                                │
│   (gray) 14 │       "name": "iterate",(/) (red)}(/)  (yellow)← unexpected character here(/)                                       │
│   (gray) 15 │       "model": "claude-sonnet-4-6",(/)                                                                  │
│   (gray)  …(/)                                                                                                       │
│   (gray)──────────────────────────────────────────────────────────────────────────────────────────────────(/)         │
│                                                                                                                   │
│   Your edits are not in memory because the file could not be loaded.                                              │
│                                                                                                                   │
│     (white)e(/)  Open in editor      open the raw file in your $VISUAL/$EDITOR                                       │
│     (white)r(/)  Reload              re-attempt parse from disk                                                      │
│     (white)d(/)  Discard             return to empty-editor state                                                    │
│     (white)c(/)  Cancel              return to empty-editor state                                                    │
│                                                                                                                   │
├───────────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│  (white)e(/) open in editor  (white)r(/) reload  (white)d(/) discard  (white)Esc(/) cancel                                              (white)v0.7.3(/) │
╰───────────────────────────────────────────────────────────────────────────────────────────────────────────────────╯
```

## Annotations

- Top border / menu bar / hrules / footer / bottom border: unchanged from edit view
- Session header: target path + a `[parse error]` tag in **red** in place of the dirty indicator and banner
- Pane area: the recovery view replaces both panes. No outline visible because there's no parsed structure to render
- Body sections (top to bottom):
  1. Header `Could not parse config.json` in **white**
  2. Parse error message in **red** with line + column
  3. Section label `Raw file content (first 8 KB, ANSI escapes stripped):` in **white**
  4. Boxed raw-bytes view in **light gray** with line numbers; the offending character highlighted in **red** with a yellow `← unexpected character here` annotation
  5. In-memory state commitment sentence
  6. Four lettered actions
- Status footer: recovery-specific shortcuts

## B: Variant — empty file

```text
│   (white)Could not parse config.json(/)                                                                              │
│                                                                                                                   │
│   (red)The file is empty.(/)                                                                                          │
│                                                                                                                   │
│   (white)Raw file content:(/)                                                                                        │
│                                                                                                                   │
│   (gray)──────────────────────────────────────────────────────────────────────────────────────────────────(/)         │
│   (gray)  (file is 0 bytes)(/)                                                                                       │
│   (gray)──────────────────────────────────────────────────────────────────────────────────────────────────(/)         │
│                                                                                                                   │
│   Your edits are not in memory because the file could not be loaded.                                              │
│                                                                                                                   │
│     (white)e(/)  Open in editor                                                                                      │
│     (white)r(/)  Reload                                                                                              │
│     (white)d(/)  Discard             return to empty-editor state                                                    │
│     (white)c(/)  Cancel              return to empty-editor state                                                    │
```

## C: Variant — UTF-8 BOM detected

```text
│   (red)File begins with a UTF-8 byte-order mark (BOM). Strip the BOM and reload.(/)                                  │
```

- The first 3 bytes of the raw view annotated as the BOM:

```text
│   (gray)──────────────────────────────────────────────────────────────────────────────────────────────────(/)         │
│   (gray)  1 │ (red)[BOM](/) (gray){(/)                                                                                          │
│   (gray)  2 │   "defaultModel": "claude-sonnet-4-6",(/)                                                              │
```

## D: Variant — non-UTF-8 encoding

```text
│   (red)File is not UTF-8 encoded. Re-save it as UTF-8 and reload.(/)                                                 │
```

- Raw view shows hex bytes for the first 8 KB:

```text
│   (gray)──────────────────────────────────────────────────────────────────────────────────────────────────(/)         │
│   (gray) 0x00 │ ff fe 7b 00 0a 00 20 00  20 00 22 00 64 00 65 00  │ ..{... . ."."d"e".(/)                            │
│   (gray) 0x10 │ 66 00 61 00 75 00 6c 00  74 00 4d 00 6f 00 64 00  │ "f"a"u"l"t"M"o"d".(/)                            │
│   (gray)  …(/)                                                                                                       │
```

## E: Auto-reload after open-in-editor (post-handoff)

When the user picks `e` (Open in editor) and the editor exits, the recovery view auto-attempts a reload. If the reload succeeds, the recovery view dissolves to the standard edit view; if it fails again, the recovery view re-renders with the new error.

## Outcome paths

- `e` / Open in editor — opens the raw config.json in `$VISUAL`/`$EDITOR`; on exit, auto-reloads (per behavioral D36)
- `r` / Reload — re-parses the file from disk
- `d` / Discard — returns to empty-editor state
- `c` / Cancel / Esc — same as Discard (returns to empty-editor)

## Cross-references

- Behavioral spec: [Alternate Flows — Parse-error recovery](../../workflow-builder/feature-specification.md#parse-error-recovery), [D36](../../workflow-builder/artifacts/decision-log.md#d36-parse-error-recovery-reload), [D43](../../workflow-builder/artifacts/decision-log.md#d43-load-time-integrity-checks).
- Impl decisions: [D-23](../../workflow-builder/artifacts/implementation-decision-log.md) (8 KiB cap, ANSI strip, banner-before-recovery-view ordering).
- Mode coverage: row 28.
