# workflowio

The `internal/workflowio` package provides load, save, and detect operations for a workflow bundle on disk. It depends on `workflowmodel`, `atomicwrite`, and `ansi`; it never imports internal TUI packages or `syscall` directly from the TUI layer (all error classification happens here).

- **Last Updated:** 2026-04-24
- **Authors:**
  - River Bailey

## Overview

- `Load(workflowDir string) (LoadResult, error)` — reads `config.json` and companion files from a bundle directory; detects symlinks before parsing; rejects companion paths that escape `workflowDir`
- `Save(fs, workflowDir, diskDoc, memDoc, companions) SaveResult` — atomically writes companion files then `config.json`; companions written first (D-20); rejects companion keys whose resolved path escapes `workflowDir`
- `DetectSymlink`, `DetectReadOnly`, `DetectExternalWorkflow`, `DetectSharedInstall` — pre-flight checks for advisory warnings
- `CreateEmptyCompanion` — creates a companion file with `O_EXCL` if it does not exist
- `DetectCrashTempFiles` — scans for orphaned `.*.tmp` files from previous crashed sessions (D-16)
- `marshalDoc` — serializes a `WorkflowDoc` to config.json JSON

Key files:

- `src/internal/workflowio/load.go` — `Load`, `LoadResult`
- `src/internal/workflowio/save.go` — `Save`, `SaveFS`, `SaveErrorKind`, `SaveResult`, `SaveSnapshot`, `classifySaveError`, `RealSaveFS`
- `src/internal/workflowio/marshal.go` — `marshalDoc`
- `src/internal/workflowio/detect.go` — `DetectSymlink`, `DetectReadOnly`, `DetectExternalWorkflow`, `CreateEmptyCompanion`
- `src/internal/workflowio/detect_unix.go` — `DetectSharedInstall` (build-tagged `!windows`)
- `src/internal/workflowio/crashtemp.go` — `DetectCrashTempFiles`

## Load

```go
// LoadResult is the outcome of a Load call.
type LoadResult struct {
    Doc         workflowmodel.WorkflowDoc
    Companions  map[string][]byte  // keyed by path relative to workflowDir
    ModTime     time.Time          // config.json mtime at load time
    Size        int64              // config.json size at load time
    IsSymlink   bool               // true if workflowDir/config.json is a symlink
    RecoveryView []byte            // up to 8 KiB of ANSI-stripped config.json on parse error
}

// Load reads config.json from workflowDir and all companion files referenced
// by the document. Returns a non-nil LoadResult on success or partial results
// on parse failure (IsSymlink + RecoveryView set).
func Load(workflowDir string) (LoadResult, error)
```

### Load Ordering

`Load` checks for a symlink before parsing (D-23): if `config.json` is a symlink, `LoadResult.IsSymlink` is set to `true` and the load proceeds normally. The symlink check happens before any parse attempt so the TUI can display the symlink warning alongside parse errors.

### Parse Error Recovery

When `config.json` exists but cannot be parsed (invalid JSON), `Load` returns an error and sets `LoadResult.RecoveryView` to up to 8 KiB of ANSI-stripped file content (F-94). The TUI displays this snippet in an error dialog to help the user diagnose the issue without leaving the TUI.

### Non-Regular File Rejection

`Load` rejects any path that is not a regular file (F-109). FIFOs, sockets, and device files return `ErrNotRegularFile`.

### Companion Loading

Missing companions are silently skipped — the editor creates them via `CreateEmptyCompanion` the first time the user opens them. Non-regular companions return `ErrNotRegularFile`.

### Path-Traversal Hardening

`Load` validates each companion path with `pathContainedIn(workflowDir, candidatePath)` before reading it. If a `promptFile` value such as `../../etc/passwd` would resolve outside `workflowDir`, `Load` returns `ErrPathEscape`:

```go
// ErrPathEscape is returned when a companion path resolves outside the workflow directory.
var ErrPathEscape = errors.New("workflowio: path escapes workflow directory")
```

`pathContainedIn` uses `filepath.EvalSymlinks` on both the directory and the candidate before the prefix check (OI-1 pattern), with `filepath.Abs` as a fallback when `EvalSymlinks` fails (e.g. file does not exist yet).

## Save

```go
// SaveFS is the injectable filesystem interface used by Save.
type SaveFS interface {
    WriteAtomic(path string, data []byte, mode os.FileMode) error
    Stat(path string) (os.FileInfo, error)
}

// SaveErrorKind classifies a save failure.
type SaveErrorKind int

const (
    SaveErrorNone                 SaveErrorKind = iota
    SaveErrorValidatorFatals
    SaveErrorPermission           // EACCES / EPERM
    SaveErrorDiskFull             // ENOSPC
    SaveErrorEXDEV                // cross-device rename
    SaveErrorConflictDetected     // mtime changed since load
    SaveErrorSymlinkEscape        // companion outside workflowDir
    SaveErrorTargetNotRegularFile
    SaveErrorParse                // JSON marshal failed
    SaveErrorOther
)

// SaveSnapshot records the mtime and size after a successful save.
type SaveSnapshot struct {
    ModTime time.Time
    Size    int64
}

// SaveResult is returned by Save.
type SaveResult struct {
    Kind     SaveErrorKind
    Err      error
    Snapshot *SaveSnapshot  // nil on any error
}

// Save atomically writes memDoc and dirty companion files to workflowDir.
// diskDoc is the snapshot from the last Load; IsDirty skips no-op saves.
// Companions are written before config.json (D-20).
// Returns SaveErrorSymlinkEscape if any companion key resolves outside workflowDir.
func Save(fs SaveFS, workflowDir string, diskDoc, memDoc workflowmodel.WorkflowDoc, companions map[string][]byte) SaveResult

// RealSaveFS returns a SaveFS that delegates to atomicwrite.Write and os.Stat.
func RealSaveFS() SaveFS
```

## Detect Functions

```go
// DetectSymlink reports whether workflowDir/config.json is a symlink.
func DetectSymlink(workflowDir string) (bool, error)

// DetectReadOnly reports whether workflowDir is read-only for the current user.
func DetectReadOnly(workflowDir string) bool

// DetectExternalWorkflow reports whether workflowDir is outside projectDir.
// An external workflow means the user is editing a shared bundle.
func DetectExternalWorkflow(workflowDir, projectDir string) (bool, error)

// DetectSharedInstall reports whether workflowDir is owned by a different user.
// Only available on Unix (build tag: !windows).
func DetectSharedInstall(workflowDir string) (bool, error)

// CreateEmptyCompanion creates a companion file with O_EXCL if it does not
// exist, creating parent directories as needed. Returns an error if the path
// would escape workflowDir (symlink escape check).
func CreateEmptyCompanion(workflowDir, relPath string) error
```

## DetectCrashTempFiles

```go
// CrashTempFile describes an orphaned temp file from a previous crashed session.
type CrashTempFile struct {
    Path string
    PID  int
}

// DetectCrashTempFiles returns any .*.tmp files in workflowDir whose originating
// process is no longer alive (kill(pid, 0) returns ESRCH).
func DetectCrashTempFiles(workflowDir string, targets []string) ([]CrashTempFile, error)
```

The `targets` slice lists the files whose sibling temp names should be checked (e.g., `["config.json", "prompts/step-1.md"]`). Temp files whose PID is still alive are skipped.

## Synchronization

`workflowio` has no internal shared state. All functions are stateless and safe for concurrent calls with different `workflowDir` values. The caller (the TUI model) is responsible for serializing calls to the same `workflowDir`.

## Testing

- `src/internal/workflowio/save_test.go` (7 tests): companion-first ordering, no-op save, companion rollback on config failure, EXDEV classification, SaveSnapshot fields, permission error classification, phase-boundary round-trip
- `src/internal/workflowio/load_test.go` (5 tests): recovery view on parse error, OSC 8 stripping in recovery view, symlink detected before parse, FIFO rejected, regular file accepted
- `src/internal/workflowio/crashtemp_test.go` (4 tests): active PID skipped, dead PID detected, atomicwrite pattern match, non-workflow file excluded
- `src/internal/workflowio/detect_test.go` (6 tests): symlink/non-symlink, writable dir, external/internal workflow, shared-install UID check, `CreateEmptyCompanion` MkdirAll and escape rejection and FIFO rejection
- `src/internal/workflowio/load_ordering_test.go` (1 test): `IsSymlink` and `RecoveryView` both set on a symlinked-and-invalid config (D-23 ordering)

## Related Documentation

- [`docs/adr/20260424120000-workflow-builder-save-durability.md`](../adr/20260424120000-workflow-builder-save-durability.md) — Save durability decision record
- [`docs/code-packages/atomicwrite.md`](atomicwrite.md) — Atomic file write primitive
- [`docs/code-packages/workflowmodel.md`](workflowmodel.md) — `WorkflowDoc` type consumed by Load and Save
- [`docs/code-packages/workflowedit.md`](workflowedit.md) — TUI editor that calls Load and Save
- [`docs/coding-standards/file-writes.md`](../coding-standards/file-writes.md) — When to use `atomicwrite.Write`
