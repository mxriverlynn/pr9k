# 16 — Dialog: Crash-Era Temporary File Notice

`DialogCrashTempNotice` — non-blocking notice presented when the builder finds a temporary file left behind by a previously crashed save (per behavioral D42-a and impl-decision D-16's PID-liveness rule).

## Render

```text
                ╭─ Recovered temp file from a previous session ──────────╮
                │                                                        │
                │   A temporary file was left behind by a previous       │
                │   builder session that did not complete its save.      │
                │                                                        │
                │   Path:    ~/projects/foo/.pr9k/workflow/              │
                │            config.json.13728-1746294117984832000.tmp   │
                │   Created: 2026-04-26 09:14:32 (3,221 bytes)           │
                │   PID:     13728 (no longer running)                   │
                │                                                        │
                │   No edits were lost — the file you are about to load  │
                │   is unchanged. The temp file is leftover scratch.     │
                │                                                        │
                │     (white)d(/)  Delete      remove the temp file silently        │
                │     (white)l(/)  Leave       keep the temp file for your review   │
                │                                                        │
                │                          [(green) Leave (/)]   Delete                │
                ╰────────────────────────────────────────────────────────╯
```

## Annotations

- Title `Recovered temp file from a previous session` in **white**
- Body opens with a one-sentence explanation
- Three-row state block in **light gray** (path, creation time, PID)
- "No edits were lost" reassurance per the four-element error template
- Two lettered actions: Delete (destructive — removes file), Leave (safe — keyboard default)
- Footer right-aligned: `[ Leave ]` (green default), Delete (in **white**)

## Footer in this mode

```text
│  (white)d(/) delete  (white)l(/) leave  (white)Esc(/) leave  (white)Enter(/) leave                                                          (white)v0.7.3(/) │
```

## Outcome paths

- `d` / Delete — removes the temp file silently; opens the actual workflow next
- `l` / Leave / Esc / Enter — proceeds to load the actual workflow without touching the temp file

After this dialog dismisses, the builder advances to the standard load pipeline (read-only check, symlink banner, parse, etc.). The dialog blocks the load until dismissed, but does not block any user keystrokes — it's a "notice" not a "decision required."

## Variant: multiple crash-era temp files

When the detector finds more than one matching temp file, the dialog enumerates them:

```text
                ╭─ Recovered temp files from previous sessions ──────────╮
                │                                                        │
                │   Two temporary files were left behind by previous     │
                │   builder sessions that did not complete their saves.  │
                │                                                        │
                │   1. config.json.13728-1746294117984832000.tmp         │
                │      Created 2026-04-26 09:14:32 · PID 13728 (gone)    │
                │                                                        │
                │   2. config.json.13901-1746295823004171000.tmp         │
                │      Created 2026-04-26 09:43:07 · PID 13901 (gone)    │
                │                                                        │
                │   No edits were lost — the actual workflow is          │
                │   unchanged.                                           │
                │                                                        │
                │     (white)d(/)  Delete all     remove all listed temp files       │
                │     (white)l(/)  Leave all      keep them for your review          │
                │                                                        │
                │                      [(green) Leave all (/)]   Delete all            │
                ╰────────────────────────────────────────────────────────╯
```

- The list shows up to 8 entries; `+ N more` suffix if exceeded
- Bulk action only — no per-file selection in v1

## Cross-references

- Behavioral spec: [Alternate Flows — Crash-era temporary file on open](../../workflow-builder/feature-specification.md#crash-era-temporary-file-on-open), [D42-a](../../workflow-builder/artifacts/decision-log.md#d42-a-crash-era-temp-file-cleanup-contract).
- Impl decisions: [D-16](../../workflow-builder/artifacts/implementation-decision-log.md) (PID-liveness rule).
