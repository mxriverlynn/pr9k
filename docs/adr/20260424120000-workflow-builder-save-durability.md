# ADR: Workflow Builder Save Durability

- **Date:** 2026-04-24
- **Status:** Accepted
- **Authors:** River Bailey

## Context

The workflow builder (`pr9k workflow`) allows users to edit `config.json` workflow bundles and their companion files (prompt files, scripts) through an interactive TUI. When the user saves, multiple files on disk must be updated atomically to prevent a partial write from producing a corrupt bundle.

Key concerns:
1. A crash during save must not leave `config.json` in a truncated or half-written state.
2. Companion files (prompt files, scripts) must be written before `config.json` so that a crash between companion writes and the config write never leaves the config referencing missing files.
3. If another process (or a previous crashed session) is writing the same files concurrently, the builder must detect this and avoid silently overwriting the concurrent changes.
4. Cross-device rename (EXDEV) is a real risk on systems where the bundle lives on a different filesystem from the temp directory.

## Decisions

### (a) Atomic write via temp-file + rename

**Decision:** All file writes (companions and `config.json`) go through `atomicwrite.Write`, which creates a sibling temp file (`<basename>.<pid>-<epoch-ns>.tmp`), writes and syncs it, then renames it over the target. If the rename fails, the temp file is cleaned up.

**Rationale:** Rename is atomic on all POSIX filesystems when the source and destination are on the same device. This prevents the file from ever being in a partially-written state. `O_EXCL` on temp creation prevents two concurrent writers from colliding on the same temp name.

**Trade-off:** Cross-device rename (EXDEV) is not recoverable without a copy+rename fallback. `workflowio.Save` classifies EXDEV as `SaveErrorEXDEV` and surfaces it to the TUI as an actionable error, rather than silently falling back to a non-atomic write.

### (b) Companion-first write ordering

**Decision:** Companion files are written before `config.json`. The ordering is deterministic: all companions are written in map-iteration order, then `config.json` is written last.

**Rationale (D-20):** If a crash occurs between companion writes and the config write, the orphaned new companion files are benign — they exist but are not yet referenced by `config.json`. The reverse ordering (config first, then companions) could leave `config.json` referencing prompt files that do not yet exist, which would cause `pr9k` to fail on the next run.

**Accepted limitation:** A crash between companion writes and the config write leaves orphaned companion files (new companion versions that the old config does not reference). These are harmless and will be overwritten on the next successful save.

### (c) Nanosecond-precision mtime conflict detection

**Decision:** After each successful save, the builder records the `ModTime` and `Size` of the written `config.json` as a `SaveSnapshot`. Before the next save, the snapshot is compared against the current on-disk mtime using `time.Time.Equal()` (nanosecond precision). If they differ, the save is blocked and a conflict dialog is shown.

**Rationale (D-41):** Second-precision mtime comparison misses rapid edits (two writes within the same second). Nanosecond precision is available on all modern filesystems (ext4, APFS, NTFS) and correctly detects concurrent modifications by another process.

**Trade-off:** On filesystems with lower-precision timestamps (e.g., FAT32 with 2-second resolution), false-conflict alerts may appear. This is an acceptable trade-off given that workflow bundles are typically stored on modern filesystems.

### (d) Crash-temp file detection on startup

**Decision:** On startup, `workflowio.DetectCrashTempFiles` scans `workflowDir` for files matching the pattern `<basename>.<pid>-*.tmp` for each companion target (config.json and each companion file). For each matching temp file, the PID is parsed and `kill(pid, 0)` is used to check whether the originating process is still alive. Temp files from dead processes are reported to the TUI as stale crash artifacts (D-16).

**Rationale:** Without detection, stale temp files accumulate silently after crashes. Surfacing them lets the user decide whether to delete them before starting a new session.

**Accepted limitation:** PID reuse means a temp file from a dead pid could be falsely attributed to a live process with the same PID. This is an edge case that resolves itself on the next system restart.

### (e) EXDEV, ENOSPC, EACCES/EPERM classification

**Decision:** `workflowio.Save` classifies save errors into `SaveErrorKind` values before returning them to the TUI. The classification happens entirely in the `workflowio` layer; the TUI layer never imports `syscall`.

**Rationale:** Keeps syscall-specific error handling in a single, well-tested package. The TUI renders the appropriate per-kind error dialog based on the `SaveErrorKind` value.

| Error | Kind |
|-------|------|
| `syscall.EACCES` or `syscall.EPERM` | `SaveErrorPermission` |
| `syscall.ENOSPC` | `SaveErrorDiskFull` |
| `syscall.EXDEV` | `SaveErrorEXDEV` |
| `time.Time` mtime mismatch | `SaveErrorConflictDetected` |
| Companion path escapes workflowDir | `SaveErrorSymlinkEscape` |
| Companion target not a regular file | `SaveErrorTargetNotRegularFile` |
| JSON marshal failure | `SaveErrorParse` |
| Any other error | `SaveErrorOther` |

## Consequences

- `internal/atomicwrite` is a direct dependency of `internal/workflowio`; all other packages that need atomic file writes must use it instead of `os.WriteFile` or direct `O_TRUNC` writes.
- Crash recovery (detecting and removing orphaned temp files) is a user-visible feature requiring explicit UI affordance.
- Conflict detection adds a `SaveSnapshot` field to the TUI model; it must be reset on session transitions (`Ctrl+N`, File > Open).
- The EXDEV classification means users on cross-device setups see an actionable error rather than a silent fallback to a non-atomic write.

## Apply when

Apply this ADR when:
- Modifying file-write code in `internal/workflowio` or `internal/atomicwrite`
- Evaluating proposals to add `O_TRUNC` writes or `os.WriteFile` calls for user-facing files
- Adding new companion file types that need to be written by the builder
- Evaluating cross-device or network-filesystem support for workflow bundles

## Notes

**Key files:**
- `src/internal/atomicwrite/write.go` — `Write(path, data, mode)`, `resolveRealPath`
- `src/internal/workflowio/save.go` — `Save`, `SaveFS`, `SaveErrorKind`, `SaveSnapshot`, `SaveResult`
- `src/internal/workflowio/crashtemp.go` — `DetectCrashTempFiles`
- `src/internal/workflowio/detect.go` — `DetectSymlink`, `DetectReadOnly`, `DetectExternalWorkflow`, `CreateEmptyCompanion`

**Related docs:**
- [`docs/coding-standards/file-writes.md`](../coding-standards/file-writes.md) — File-write coding standards (Rule 1: atomic-rename required; Rule 2: O_EXCL for new temp files)
- [`docs/code-packages/atomicwrite.md`](../code-packages/atomicwrite.md) — `atomicwrite` package API reference
- [`docs/code-packages/workflowio.md`](../code-packages/workflowio.md) — `workflowio` package API reference
