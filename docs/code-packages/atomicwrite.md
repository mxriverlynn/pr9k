# atomicwrite

The `internal/atomicwrite` package provides a single function, `Write`, for durable atomic file replacement. It is the only approved mechanism for writing user-facing files in pr9k that must not be left in a partially-written state.

- **Last Updated:** 2026-04-24
- **Authors:**
  - River Bailey

## Overview

- `Write(path, data, mode)` writes `data` to `path` atomically using a temp-file + rename pattern
- If `path` is a symlink, the symlink entry is preserved: the file it points to is replaced
- The temp file is created as a sibling of the target with name `<basename>.<pid>-<epoch-ns>.tmp` using `O_CREATE|O_EXCL|O_WRONLY, 0o600` — the `O_EXCL` flag prevents two concurrent callers from colliding on the same temp name
- After a successful write and sync, the temp file is renamed over the target; on failure, the temp file is removed
- EXDEV errors (cross-device rename) propagate unwrapped so callers can detect them with `errors.Is(err, syscall.EXDEV)`
- The parent directory is fsynced after a successful rename (best-effort) so the rename directory entry survives power loss (POSIX requirement)

Key file: `src/internal/atomicwrite/write.go`

## Core API

```go
// Write atomically replaces path with data using mode for a newly created file.
// If path is a symlink, the symlink entry is preserved: the file it points to
// is replaced. Cross-device rename errors propagate unwrapped so callers can
// detect them with errors.Is(err, syscall.EXDEV).
func Write(path string, data []byte, mode os.FileMode) error
```

## Architecture

```
Write(path, data, mode)
  │
  ├─ resolveRealPath(path)
  │    ├─ EvalSymlinks(path) → realPath (symlink follows)
  │    └─ on ENOENT: walk parents until an existing ancestor is found,
  │       then reconstruct the target path
  │
  ├─ tempPath = realDir/<basename>.<pid>-<ns>.tmp
  ├─ OpenFile(tempPath, O_CREATE|O_EXCL|O_WRONLY, 0o600)
  ├─ Write(data)
  ├─ Sync()
  ├─ Close()
  ├─ Rename(tempPath, realPath)   ← atomic on same device
  │    └─ on failure: Remove(tempPath)
  └─ openDir(realDir).Sync()     ← best-effort parent fsync
```

## Symlink Behavior

When `path` is a symlink (e.g., `config.json → /shared/configs/myrepo.json`), `resolveRealPath` calls `filepath.EvalSymlinks` to follow the chain to the final target. The temp file is created alongside the real target, not alongside the symlink. After the rename, the symlink entry still points to the same real file — only the file content changes. This ensures that the caller's view of the path is always consistent.

## ENOENT Handling (First Save)

When `path` does not yet exist (first save), `EvalSymlinks` returns `ENOENT`. `resolveRealPath` walks toward the root until it finds an existing ancestor directory, resolves that ancestor, and appends the unresolved suffix. This ensures the temp file is created in an existing directory even on first write.

## Error Handling

| Scenario | Error |
|----------|-------|
| Temp create fails | `atomicwrite: create temp <path>: <os err>` |
| Write fails | `atomicwrite: write <path>: <os err>` |
| Sync fails | `atomicwrite: sync <path>: <os err>` |
| Close fails | temp is removed; `atomicwrite: close <path>: <os err>` |
| Rename fails (same device) | temp is removed; raw OS error (may be EXDEV) |
| EvalSymlinks fails (non-ENOENT) | `atomicwrite: eval symlinks <path>: <os err>` |
| No existing ancestor | `atomicwrite: no existing ancestor for <path>` |

## Dependency Injection

The real `os` operations are accessed through an unexported `writeFS` interface (`evalSymlinks`, `openFile`, `rename`, `remove`, `openDir`). The production `osFS` struct implements this interface using the standard library. Tests inject `fakeFS` structs that return configured errors for specific operations.

## Testing

- `src/internal/atomicwrite/write_test.go`
- Tests cover: happy-path round-trip, symlink target replacement, ENOENT first-save, EXDEV propagation, write/sync/close failure cleanup, parent-fsync best-effort, race between two concurrent callers

## Related Documentation

- [`docs/adr/20260424120000-workflow-builder-save-durability.md`](../adr/20260424120000-workflow-builder-save-durability.md) — Decision record for durable-save strategy
- [`docs/coding-standards/file-writes.md`](../coding-standards/file-writes.md) — When and how to use `atomicwrite.Write`
- [`docs/code-packages/workflowio.md`](workflowio.md) — `workflowio.Save` which uses `atomicwrite.Write`
