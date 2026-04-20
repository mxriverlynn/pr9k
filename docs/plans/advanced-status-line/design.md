# Advanced Status Line — Design

**Status:** Draft for review — ready to implement
**Date:** 2026-04-20
**Author:** River Bailey
**Feeds into:** a single landable PR implementing Option 1″ (capture-and-forward) from [`research.md`](./research.md)

---

## 1. Goal

Expose Claude Code's native statusLine payload (`rate_limits.*`, `cost.total_lines_added/removed`, `context_window.*`, etc.) on pr9k's own status line, so user-authored status scripts can render what Claude is doing right now — not just pr9k workflow metadata.

**Explicit non-goals:**

- Deriving these fields from the NDJSON stream. Rate limits, lines-added/removed, and Claude's own context-window percentages are not in the stream, and stream-derived token counts are noisy (`research.md` §4.1).
- Replacing pr9k's existing statusLine payload fields. The Claude fields are additive.
- Supporting Claude-on-host. This plan assumes the Docker sandbox model fixed by ADR `docs/adr/20260413160000-require-docker-sandbox.md`.
- Python bridges or a Claude Agent SDK integration. No Go SDK exists for that surface (`research.md` §7).

The research phase resolved all eight open questions (`research.md` §14) and eliminated earlier Option 1′ complications (scratch `CLAUDE_CONFIG_DIR`, derived Docker image, Unix-socket IPC). This design implements the survivor: **Option 1″**.

---

## 2. Design summary

When the user opts in with `statusLine.captureClaudeStatusLine: true`:

1. **At every claude step spawn**, pr9k writes three files under `<projectDir>/.pr9k/`:
   - `sandbox-settings.json` — a pr9k-authored Claude `settings.json` whose only top-level key is `statusLine` pointing at the shim.
   - `statusline-shim.sh` — a three-line `/bin/sh` script, mode `0755`, that does an atomic-rename write of its stdin to `statusline-current.json`.
   - `statusline-current.json` is created by the shim at runtime — not pre-written.
2. **A file-level Docker bind mount** is added to the `docker run` argv: `-v <projectDir>/.pr9k/sandbox-settings.json:/home/agent/.claude/settings.json`. This layers on top of the existing directory-level profile mount, so Claude inside the container reads pr9k's `settings.json` while every other file in `~/.claude` (sessions, credentials, plugins) is the user's real data. **The user's real `~/.claude/settings.json` on disk is never written.**
3. **A host-side `fsnotify` watcher** observes `.pr9k/` for the shim's atomic rename, reads `statusline-current.json`, validates it parses as JSON, stores a `claudePayloadSnapshot` (`{raw, ingestedAt}`) via an `atomic.Pointer[claudePayloadSnapshot]` on `statusline.Runner`, and fires `Trigger()` so the user's statusLine script runs with the fresh data.
4. **pr9k's stdin payload** to the user's script gains a `claude.native` field carrying Claude's full payload verbatim when present; the field is absent when the flag is off or Claude has not yet invoked the shim.
5. **On every startup**, pr9k removes stale `sandbox-settings.json`, `statusline-shim.sh`, and `statusline-current.json` idempotently. Shutdown cleanup is not reliable (main exits via `os.Exit`, which skips defers), so startup cleanup is the only mechanism — see §4.8.

When the flag is off (default), pr9k behaves identically to today — no bind mount, no overlay files, no watcher goroutine.

---

## 3. Architecture

### 3.1 Artifacts

| Path (host) | Writer | Reader | Mode | Created | Deleted |
|---|---|---|---|---|---|
| `<projectDir>/.pr9k/sandbox-settings.json` | pr9k host process (`statusline_overlay.WriteOverlay`) | Claude in container via bind mount | `0644` | Before each claude step spawn (when flag on) | On startup (stale cleanup) only |
| `<projectDir>/.pr9k/statusline-shim.sh` | pr9k host process (`statusline_overlay.WriteOverlay`) | Claude in container (invoked by `statusLine.command`) | `0755` | Before each claude step spawn (when flag on) | On startup (stale cleanup) only |
| `<projectDir>/.pr9k/statusline-current.json` | shim in container (atomic rename) | pr9k host process (`claude_watcher`) | `0644` | On first shim invocation by Claude | On startup (stale cleanup) only |
| `<projectDir>/.pr9k/statusline-current.json.<pid>.tmp` | shim in container | renamed away | — | Transient | Consumed by the `mv` in the shim; stragglers from a failed `mv` are swept by `CleanOverlay` on next startup |

All four files live under `.pr9k/`, which is already gitignored per ADR `docs/adr/20260418175134-pr9k-rename-and-pr9k-layout.md`.

### 3.2 Data flow (one shim invocation)

```
Claude (in container) →  statusLine trigger (new assistant message, permission change, vim toggle, or refreshInterval tick)
                      →  spawns /home/agent/.claude/settings.json's statusLine.command
                             (resolves to /home/agent/workspace/.pr9k/statusline-shim.sh)
                      →  shim reads its stdin (Claude's native payload JSON)
                      →  shim writes tmp file, renames atomically to statusline-current.json
                             (VirtioFS bind mount propagates to host file)
pr9k host process     →  fsnotify Create/Rename event for statusline-current.json
                      →  read file contents (few KB)
                      →  json.Unmarshal to validate shape (ignore on parse error, log)
                      →  store as json.RawMessage in runner.claudePayload.Store(&raw)
                      →  runner.Trigger() — non-blocking, drop-on-full
Runner worker loop    →  execScript: build stdin JSON with runner.claudePayload.Load() embedded at .claude.native
                      →  invoke user's statusLine script with that stdin
                      →  user script prints one line; pr9k caches it, TUI footer re-renders
```

### 3.3 IPC choice — atomic-rename + fsnotify

- **Unix-domain sockets on Docker Desktop macOS bind mounts are still unreliable in 2026** (`research.md` Q-OPEN-5). Rejected.
- **Atomic rename on VirtioFS is reliable** — POSIX `rename(2)` atomicity survives VirtioFS passthrough. The reader always sees either the old file or the new file, never a partial write.
- **fsnotify is running on the host**, watching a host directory. The container bind-mount propagates the rename to the host inode-level change, which fsnotify receives via FSEvents (macOS) or inotify (Linux).
- **Latency budget:** Claude debounces at 300 ms; VirtioFS write-propagate is low-ms for a few-KB file; fsnotify wake-up is sub-ms. Well within human perception.

### 3.4 Concurrency model

- **`claudePayload atomic.Pointer[claudePayloadSnapshot]`** lives on `statusline.Runner`. Single shared cell carrying both the raw JSON and the ingestion timestamp (needed for the `claude.pr9k.age_seconds` staleness signal in §4.9).
- `claudePayloadSnapshot` is an immutable struct `{raw json.RawMessage; at time.Time}`. The watcher always allocates a fresh snapshot and stores its pointer — never mutates an existing one.
- **Writer:** the fsnotify watcher goroutine (single goroutine). Calls `claudePayload.Store(snap)` after each successful ingest.
- **Readers:** the runner's `execScript` goroutine via `claudePayload.Load()` during `BuildPayload`. `atomic.Pointer` provides the release-acquire ordering this pattern needs — no mutex.
- **`json.RawMessage` is a `[]byte`** under the hood; the watcher must not mutate the slice after storing it. The pattern is always "allocate fresh → store pointer" (no reuse).
- **Single-flight on the user script** is unchanged — `Runner.running atomic.Bool` already serializes `execScript`.

### 3.5 What does NOT change

- `internal/claudestream/` — parser, aggregator, renderer, pipeline. Unchanged.
- `internal/workflow/`:
  - `Runner`, `Orchestrate`, `StepExecutor` **interface surface**, resume gates: unchanged.
  - `RunConfig` gains one bool field (`CaptureClaudeStatusLine`) and `buildStep` gains one parameter. No cross-package effect beyond `cmd/pr9k/wiring.go` and test-double updates.
- `internal/preflight/` — no new preflight check (the feature is opt-in; absence of fsnotify or a read-only `.pr9k/` is surfaced at `Start()` time, not as a startup gate).
- `internal/sandbox/image.go` — `ImageTag`, `ContainerRepoPath`, `ContainerProfilePath`, `BuiltinEnvAllowlist`. Unchanged.
- `docker/sandbox-templates:claude-code` base image. No derived image.
- `CLAUDE_CONFIG_DIR` env var. Still hardcoded to `ContainerProfilePath`.

---

## 4. Detailed component design

### 4.1 Config schema change — `StatusLineConfig`

**File:** `src/internal/steps/steps.go:51-57`

Add one field to `StatusLineConfig`:

```go
type StatusLineConfig struct {
    Type                      string `json:"type,omitempty"`
    Command                   string `json:"command"`
    RefreshIntervalSeconds    *int   `json:"refreshIntervalSeconds,omitempty"`
    CaptureClaudeStatusLine   *bool  `json:"captureClaudeStatusLine,omitempty"` // default nil → off
}
```

`*bool` (not `bool`) so the validator can distinguish absent from explicit false, matching the existing `IsClaude`/`ResumePrevious` pattern.

### 4.2 Validator — accept the new field

**File:** `src/internal/validator/validator.go:93-97`

Add the matching field to `vStatusLine`:

```go
type vStatusLine struct {
    Type                    string `json:"type,omitempty"`
    Command                 string `json:"command"`
    RefreshIntervalSeconds  *int   `json:"refreshIntervalSeconds,omitempty"`
    CaptureClaudeStatusLine *bool  `json:"captureClaudeStatusLine,omitempty"`
}
```

No cross-field constraint — the bool is independently valid in any combination. The `DisallowUnknownFields` decoder at `validator.go:164-168` will continue to reject misspellings.

**Field independence:** `captureClaudeStatusLine` has no cross-field validation constraints. An empty `command` is already an error under existing rules (`validator.go:263`), so the combination `{command: "", captureClaudeStatusLine: true}` never reaches runtime — it's rejected upstream.

### 4.3 Overlay writer — new package file

**File:** `src/internal/sandbox/statusline_overlay.go` (new, ~80 LoC)

Two exported functions, one package-local constant:

```go
package sandbox

import (
    "encoding/json"
    "errors"
    "fmt"
    "io/fs"
    "os"
    "path/filepath"
)

// OverlaySettingsTemplate is the JSON content pr9k writes into
// sandbox-settings.json. Claude reads this via the bind mount at
// /home/agent/.claude/settings.json. Minimum keys Claude needs: type + command.
// No custom sibling keys — nothing we'd read back justifies the risk of a
// future Claude schema tightening rejecting the overlay.
const OverlaySettingsTemplate = `{
  "statusLine": {
    "type": "command",
    "command": "%s"
  }
}`

// ShimScriptContent is the shell script pr9k writes to statusline-shim.sh.
// It reads its stdin to a PID-qualified tmp file and, only on success,
// atomically renames it to the live file so the host fsnotify watcher never
// sees a partial write.
//
// Three defenses required here:
//  1. `set -eu` aborts the script on any failure; `cat` failing (e.g., under
//     Claude-side SIGKILL of the shim) stops the script before mv runs.
//  2. `&&` chains `mv` to succeed only if `cat` succeeded, belt-and-suspenders
//     to set -e.
//  3. The tmp path embeds `$$` (shell PID). Claude's docs say in-flight shim
//     invocations are cancelled when a new update arrives; if two shims ever
//     overlap (or if Claude spawns a replacement before the killed one exits),
//     each writes to its own PID-unique tmp. Without this, two concurrent
//     `cat > statusline-current.json.tmp` calls write to the same inode and
//     interleave bytes — the final mv then installs a corrupted payload.
const ShimScriptContent = `#!/bin/sh
set -eu
TMP="/home/agent/workspace/.pr9k/statusline-current.json.$$.tmp"
cat > "$TMP" && \
  mv "$TMP" /home/agent/workspace/.pr9k/statusline-current.json
`

// ShimContainerPath is the absolute path Claude-in-container will execute
// when its statusLine.command fires. Matches the ContainerRepoPath mount
// point plus the .pr9k-relative shim filename.
const ShimContainerPath = ContainerRepoPath + "/.pr9k/statusline-shim.sh"

// Artifact filenames under <projectDir>/.pr9k/.
const (
    overlaySettingsBasename = "sandbox-settings.json"
    shimBasename            = "statusline-shim.sh"
    payloadBasename         = "statusline-current.json"
)

// WriteOverlay writes the settings overlay and shim into <projectDir>/.pr9k/
// using write-tmp-then-rename for atomicity. `os.WriteFile` truncates and then
// writes — a container reading the overlay during that window would see a
// zero-length file. Claude startup inside the container reads settings.json
// exactly once per invocation; if it races the truncation window the overlay
// is a no-op for that run. Tmp+rename makes the file appear atomically.
//
// It is idempotent: re-writes are safe (rename replaces the old file).
// Returns the host-absolute settings path so callers can build the file-level
// bind mount, and an error when any write fails.
func WriteOverlay(projectDir string) (settingsPath string, err error) {
    dir := filepath.Join(projectDir, ".pr9k")
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return "", fmt.Errorf("sandbox: mkdir .pr9k: %w", err)
    }

    settings := fmt.Sprintf(OverlaySettingsTemplate, ShimContainerPath)
    if !json.Valid([]byte(settings)) {
        return "", fmt.Errorf("sandbox: overlay settings template produced invalid JSON")
    }

    settingsPath = filepath.Join(dir, overlaySettingsBasename)
    if err := atomicWrite(settingsPath, []byte(settings), 0o644); err != nil {
        return "", fmt.Errorf("sandbox: write %s: %w", settingsPath, err)
    }

    shimPath := filepath.Join(dir, shimBasename)
    if err := atomicWrite(shimPath, []byte(ShimScriptContent), 0o755); err != nil {
        return "", fmt.Errorf("sandbox: write %s: %w", shimPath, err)
    }

    return settingsPath, nil
}

// atomicWrite writes data to path via a PID-suffixed tmp file plus rename,
// then chmods the final path to mode. Two concurrent pr9k processes writing
// to the same projectDir produce unique tmp names and cannot collide.
// The final rename is atomic on POSIX (rename(2)) and on Windows (via
// os.Rename) — an observer of the final path sees either the old bytes or
// the new bytes, never partial.
//
// Uses POSIX tmp-in-same-directory so rename(2) is guaranteed to be atomic
// (cross-device rename would fall back to copy+unlink, which is not atomic).
func atomicWrite(path string, data []byte, mode os.FileMode) error {
    tmp := fmt.Sprintf("%s.%d.tmp", path, os.Getpid())
    if err := os.WriteFile(tmp, data, mode); err != nil {
        return err
    }
    // os.WriteFile respects umask; chmod explicitly so mode lands even under
    // a restrictive umask (e.g. 077 would clip 0o755 to 0o700, making the
    // shim unreadable by "other" in the container).
    if err := os.Chmod(tmp, mode); err != nil {
        _ = os.Remove(tmp)
        return err
    }
    if err := os.Rename(tmp, path); err != nil {
        _ = os.Remove(tmp)
        return err
    }
    return nil
}

// CleanOverlay removes all three artifact files under <projectDir>/.pr9k/
// idempotently, plus any leaked PID-qualified tmp files from a shim whose
// mv failed mid-run (pattern: statusline-current.json.*.tmp). Missing files
// are ignored. Errors surface only for unexpected failures (permission
// denied, etc.).
//
// Uses errors.Is(err, fs.ErrNotExist) per docs/coding-standards/error-handling.md
// (wrapped-error compatibility). os.IsNotExist does not unwrap.
func CleanOverlay(projectDir string) error {
    dir := filepath.Join(projectDir, ".pr9k")
    var firstErr error
    remove := func(path string) {
        if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
            if firstErr == nil {
                firstErr = fmt.Errorf("sandbox: remove %s: %w", filepath.Base(path), err)
            }
        }
    }
    for _, name := range []string{overlaySettingsBasename, shimBasename, payloadBasename} {
        remove(filepath.Join(dir, name))
    }
    // Sweep leaked shim tmp files (statusline-current.json.<pid>.tmp). Glob
    // errors only on malformed patterns, which this literal string is not —
    // a nil-match return is expected and normal on a clean tree.
    matches, _ := filepath.Glob(filepath.Join(dir, payloadBasename+".*.tmp"))
    for _, m := range matches {
        remove(m)
    }
    return firstErr
}

// PayloadPath is the host path to the shim-written payload file. Exposed
// so the statusline package can both watch it and read its contents.
func PayloadPath(projectDir string) string {
    return filepath.Join(projectDir, ".pr9k", payloadBasename)
}
```

### 4.4 Sandbox `BuildRunArgs` — add the file-level bind mount

**File:** `src/internal/sandbox/command.go:25-93` (signature/body span verified against today's tree)

Add one positional parameter at the end of `BuildRunArgs`:

```go
func BuildRunArgs(
    projectDir, profileDir string,
    uid, gid int,
    cidfile string,
    envAllowlist []string,
    containerEnv map[string]string,
    resumeSessionID string,
    model, prompt string,
    statusLineOverlayPath string, // empty → no overlay mount; non-empty → file-level bind mount
) []string
```

When `statusLineOverlayPath != ""`, emit the file-level mount **after** the two existing mounts, in the same mount-argv block, so the overlay is applied on top of the directory mount at `/home/agent/.claude`:

```go
if statusLineOverlayPath != "" {
    args = append(args,
        "--mount",
        fmt.Sprintf("type=bind,source=%s,target=%s/settings.json",
            statusLineOverlayPath, ContainerProfilePath),
    )
}
```

**Argv ordering matters — but must be validated empirically.** Docker's CLI reference does not explicitly document ordering semantics for file-over-directory bind-mount overlays. The design assumes Docker applies `--mount` flags in argv order and that a file mount whose target is inside an already-mounted directory requires the directory mount to appear first. This is **consistent with the research's empirical validation** (`research.md` §6.1 / Q-OPEN-1) against Claude 2.1.114 and Docker Desktop 4.39+, but is not covered by any in-tree ADR or documentation today. Two-layer test requirement:

1. `TestBuildRunArgs_OverlayMountOrdering` — pin the argv order produced by pr9k (unit).
2. `TestSandboxIntegration_OverlayShadowsUserSettings` — actually run a container and assert the overlay is effective (integration, §7.2). This test is the authoritative guarantor; the unit test is just insurance against argv regressions.

The existing code emits the profile-dir mount after the project-dir mount; append the overlay mount after the profile-dir mount (so both directory mounts are established before the file overlay).

**Test-surface impact:** `BuildRunArgs` has 24 existing call sites — 23 in `src/internal/sandbox/command_test.go` and 1 in `src/internal/workflow/run.go:764` (`image_test.go` has no `BuildRunArgs` callers). Adding a positional parameter breaks all of them; each needs a trailing `""` (mechanical edit). Existing behavior is preserved since the flag opt-out is the empty-string default.

**API-shape alternative considered.** An options struct (e.g. `type RunArgs struct { … ; StatusLineOverlayPath string }`) would let the one new optional field land without churning 24 call sites. Rejected for this PR because (a) there is no in-tree precedent — `BuildLoginArgs` (`command.go:100`) is also positional, (b) the mechanical churn is trivial, and (c) the refactor would double the blast radius of a feature PR. An options-struct refactor is reasonable follow-up work; scope it separately.

**Caller update:** `src/internal/workflow/run.go:764` (inside `buildStep`) passes the path produced by `sandbox.WriteOverlay` (see §4.7 lifecycle). All other callers (including `BuildLoginArgs` which is separate) pass `""`.

### 4.5 Host-side fsnotify watcher — new statusline file

**File:** `src/internal/statusline/claude_watcher.go` (new, ~120 LoC)

```go
package statusline

import (
    "context"
    "encoding/json"
    "errors"
    "io"
    "os"
    "path/filepath"
    "sync/atomic"
    "time"

    "github.com/fsnotify/fsnotify"
)

// payloadSizeLimit caps how much of statusline-current.json pr9k reads.
// Claude's native payload is ~1-2 KB; 16 KB is generous and prevents a
// runaway shim write from ballooning memory. Matches research §6.7.
const payloadSizeLimit = 16 * 1024

// claudePayloadSnapshot is the atomic cell's value type: the raw JSON bytes
// Claude's shim handed us, plus the host-wall-clock time at which pr9k
// finished validating them. Both fields are read-only after Store; callers
// never mutate an observed snapshot. The timestamp is what drives the
// `claude.pr9k.age_seconds` staleness signal documented in §4.9.
type claudePayloadSnapshot struct {
    raw json.RawMessage
    at  time.Time
}

// claudeWatcher watches <projectDir>/.pr9k/ for the shim's atomic-rename
// writes of statusline-current.json, reads them, validates them, and stores
// a fresh snapshot in the atomic pointer. Safe to construct repeatedly;
// stop is idempotent and blocks until the goroutine exits.
type claudeWatcher struct {
    dir     string                                   // <projectDir>/.pr9k/
    payload *atomic.Pointer[claudePayloadSnapshot]
    trigger func()                                   // runner.Trigger
    log     logWriter                                // pr9k file logger

    watcher *fsnotify.Watcher
    cancel  context.CancelFunc
    done    chan struct{}
}

// logWriter is the subset of *logger.Logger that claudeWatcher needs.
type logWriter interface {
    Log(step, msg string) error
}

// startClaudeWatcher creates and starts a watcher. Returns an error on
// fsnotify setup failure (missing dir, permission denied, resource limit).
// The returned watcher is running; caller must call stop() to drain it.
func startClaudeWatcher(
    ctx context.Context,
    dir string,
    payload *atomic.Pointer[claudePayloadSnapshot],
    trigger func(),
    log logWriter,
) (*claudeWatcher, error) {
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return nil, err
    }
    fw, err := fsnotify.NewWatcher()
    if err != nil {
        return nil, err
    }
    if err := fw.Add(dir); err != nil {
        _ = fw.Close()
        return nil, err
    }
    ctx, cancel := context.WithCancel(ctx)
    cw := &claudeWatcher{
        dir:     dir,
        payload: payload,
        trigger: trigger,
        log:     log,
        watcher: fw,
        cancel:  cancel,
        done:    make(chan struct{}),
    }
    go cw.loop(ctx)
    return cw, nil
}

func (cw *claudeWatcher) loop(ctx context.Context) {
    defer close(cw.done)
    target := filepath.Join(cw.dir, "statusline-current.json")
    for {
        select {
        case <-ctx.Done():
            return
        case err, ok := <-cw.watcher.Errors:
            if !ok {
                return
            }
            _ = cw.log.Log("statusline", "claude watcher error: "+err.Error())
        case ev, ok := <-cw.watcher.Events:
            if !ok {
                return
            }
            if ev.Name != target {
                continue
            }
            // Atomic rename fires Create on Linux inotify and
            // Rename+Create on macOS FSEvents. Accept both.
            if ev.Op&(fsnotify.Create|fsnotify.Rename|fsnotify.Write) == 0 {
                continue
            }
            cw.ingest(target)
        }
    }
}

func (cw *claudeWatcher) ingest(path string) {
    f, err := os.Open(path)
    if err != nil {
        if !errors.Is(err, os.ErrNotExist) {
            _ = cw.log.Log("statusline", "claude payload open: "+err.Error())
        }
        return
    }
    defer f.Close()
    // LimitReader caps the read at payloadSizeLimit+1 so we can detect oversize.
    // io.ReadAll returns nil err on EOF; no special-casing needed.
    data, err := io.ReadAll(io.LimitReader(f, payloadSizeLimit+1))
    if err != nil {
        _ = cw.log.Log("statusline", "claude payload read: "+err.Error())
        return
    }
    if len(data) > payloadSizeLimit {
        _ = cw.log.Log("statusline", "claude payload exceeds 16 KB; dropped")
        return
    }
    if !json.Valid(data) {
        _ = cw.log.Log("statusline", "claude payload is not valid JSON; dropped")
        return
    }
    snap := &claudePayloadSnapshot{
        raw: json.RawMessage(data),
        at:  time.Now(),
    }
    cw.payload.Store(snap)
    cw.trigger()
}

// stop blocks until the watcher goroutine exits. Idempotent.
func (cw *claudeWatcher) stop() {
    cw.cancel()
    _ = cw.watcher.Close()
    <-cw.done
}
```

Notes:

- `fsnotify.Create|Rename|Write` is intended to cover atomic-rename event reporting across Linux inotify (fires `Create` on destination) and macOS FSEvents (behavior varies by macOS version; may fire `Create` or `Rename` on destination, or only a coalesced `Create`). The mask is a superset chosen to handle all observed cases. A **platform-gated integration test** (§7.2) runs on both macOS and Linux CI to validate the actual event-path pr9k sees when a container writes via VirtioFS; a unit test on the host filesystem (`TestClaudeWatcher_ObservesAtomicRename`) does NOT exercise the bind-mount path and is insufficient alone.
- `Remove` events (source `.tmp` disappearing after `mv`) are intentionally ignored — they do not carry the new payload.
- Payload-size guard is ingest-side, independent of the 8 KB output cap on user scripts (different concern).
- JSON validity check (`json.Valid`) catches Claude-side malformed writes the research noted Claude itself silently tolerates.

### 4.6 `statusline.Runner` — hold the payload, wire the watcher

**File:** `src/internal/statusline/statusline.go:50-78` and `Start`/`Shutdown`

Add two fields to `Runner`:

```go
type Runner struct {
    // ... existing fields ...
    captureClaude bool
    claudePayload atomic.Pointer[claudePayloadSnapshot]
    claudeWatcher *claudeWatcher // nil when !captureClaude
}
```

`claudePayloadSnapshot` is the unexported struct defined in `claude_watcher.go` (§4.5) that pairs the raw JSON bytes with an ingestion timestamp; both fields feed `BuildPayload` in §4.9.

Extend `Config`:

```go
type Config struct {
    Command                string
    RefreshIntervalSeconds *int
    CaptureClaudeStatusLine bool
}
```

Wire in `New` — plumb `cfg.CaptureClaudeStatusLine` into `Runner.captureClaude`. NoOp runner is unaffected.

**`Start`:**

```go
func (r *Runner) Start(ctx context.Context) {
    if !r.enabled {
        return
    }
    ctx, r.cancel = context.WithCancel(ctx)

    if r.captureClaude {
        dir := filepath.Join(r.projectDir, ".pr9k")
        cw, err := startClaudeWatcher(ctx, dir, &r.claudePayload, r.Trigger, r.log)
        if err != nil {
            r.logLine("claude watcher disabled: " + err.Error())
        } else {
            r.claudeWatcher = cw
        }
    }

    r.wg.Add(1)
    go r.worker(ctx)
    if r.interval > 0 {
        r.wg.Add(1)
        go r.timerLoop(ctx)
    }
}
```

**`Shutdown` — order-critical:**

```go
func (r *Runner) Shutdown() {
    if !r.enabled {
        return
    }
    r.stopped.Store(true)

    // Stop the watcher FIRST, before wg.Wait. If we waited for worker/timer
    // first and then stopped the watcher, the watcher could push one more
    // trigger into the stopped-worker path — the payload pointer would be
    // updated but unobservable, and a racy fsnotify Write could happen
    // concurrently with CleanOverlay on the next startup.
    if r.claudeWatcher != nil {
        r.claudeWatcher.stop() // idempotent; blocks until watcher goroutine exits
    }

    if r.cancel != nil {
        r.cancel()
    }
    done := make(chan struct{})
    go func() {
        r.wg.Wait()
        close(done)
    }()
    select {
    case <-done:
    case <-time.After(2 * time.Second):
    }
}
```

**Overlay cleanup lives in `main.go`, not `Runner.Shutdown`.** Reason: `Runner.Shutdown` early-returns when `!r.enabled` (existing contract at `statusline.go:205-208`), so a user who set `captureClaudeStatusLine: true` with a NoOp runner would skip cleanup. Validator rejects that combination today, but decoupling cleanup from runner state is still the safer contract. Cleanup happens in `main.go` at startup only (see §4.8).

**Payload load site** — inside `execScript` (`statusline.go:263-278`) immediately before `BuildPayload`:

```go
var claudeRaw json.RawMessage
var claudeAt time.Time
if snap := r.claudePayload.Load(); snap != nil {
    claudeRaw = snap.raw
    claudeAt = snap.at
}
payload, err := BuildPayload(s, mode, claudeRaw, claudeAt)
```

A zero-value `claudeAt` pairs with an empty `claudeRaw`, and `BuildPayload` treats both as "no Claude data yet — omit the `claude` field entirely" (§4.9).

**Failure mode:** if the fsnotify watcher fails to start (e.g. inotify limit, permission denied), the runner logs and continues without Claude capture. Error mode is non-fatal — pr9k still runs, just without the `claude.native` field. Document.

### 4.7 Lifecycle — when is `WriteOverlay` called, and by whom?

Two reasonable call sites:

**A. Per-step (call inside `workflow/run.go:749-772` `buildStep` right before `BuildRunArgs`)**
   - Pros: overlay files always present and correct before each claude spawn; if a user edits or deletes them mid-run they are replaced.
   - Cons: the write happens on every claude step spawn — ~20 times per run. Tiny cost (two small file writes) but more opportunities for transient failures.

**B. Once at runner start (call in `main.go` between `statusline.New` and `Runner.Start`)**
   - Pros: one write per process. Simpler failure surface.
   - Cons: if the user `rm`s the files mid-run, the next claude step sees no overlay and starts with the user's real `settings.json` (Claude would not know to invoke the shim).

**Chosen: Option A (per-step).** The file write is a few hundred bytes each, the shim is 3 lines, and idempotent writes are the simplest contract to reason about. Rationale: the files are not precious — recreating them is the cleanest way to guarantee they're correct. Option B's single-write-at-start is an optimization that sacrifices robustness for a negligible perf win.

**Threading — via `workflow.RunConfig`, not `StepExecutor`:**

The flag is configuration (static per run), not executor state. The cleanest path is:

1. Add `CaptureClaudeStatusLine bool` to `workflow.RunConfig` (`src/internal/workflow/run.go`, the struct populated by `buildRunConfig` in `wiring.go:74-87`).
2. Populate it from `stepFile.StatusLine.CaptureClaudeStatusLine` inside `buildRunConfig` (nil-safe: nil StatusLine → false; nil pointer → false; non-nil pointer → dereferenced value).
3. Pass it into `buildStep` as a new parameter so the claude-step branch can consult it without reaching through `executor`.

Signature change:

```go
func buildStep(
    workflowDir string,
    s steps.Step,
    vt *vars.VarTable,
    phase vars.Phase,
    env []string,
    containerEnv map[string]string,
    executor StepExecutor,
    resumeSessionID string,
    captureClaudeStatusLine bool,   // NEW
) (ui.ResolvedStep, error)
```

Each `buildStep` call site (3 in `run.go:403, 527, 662`; 17 in tests — 15 in `run_test.go` and 2 in `run_timeout_test.go`) forwards the bit from `cfg.CaptureClaudeStatusLine`.

**Call site body inside `buildStep`:**

```go
if s.IsClaude {
    prompt, err := steps.BuildPrompt(workflowDir, s, vt, phase)
    if err != nil {
        return ui.ResolvedStep{}, fmt.Errorf("step %q: %w", s.Name, err)
    }
    uid, gid := sandbox.HostUIDGID()
    cidfile, err := sandbox.Path()
    if err != nil {
        return ui.ResolvedStep{}, fmt.Errorf("step %q: cidfile: %w", s.Name, err)
    }
    profileDir := preflight.ResolveProfileDir()
    projectDir := executor.ProjectDir()
    envAllowlist := append([]string{}, sandbox.BuiltinEnvAllowlist...)
    envAllowlist = append(envAllowlist, env...)

    var overlayPath string
    if captureClaudeStatusLine {
        p, err := sandbox.WriteOverlay(projectDir)
        if err != nil {
            return ui.ResolvedStep{}, fmt.Errorf("step %q: overlay: %w", s.Name, err)
        }
        overlayPath = p
    }

    argv := sandbox.BuildRunArgs(projectDir, profileDir, uid, gid, cidfile,
        envAllowlist, containerEnv, resumeSessionID, s.Model, prompt, overlayPath)
    return ui.ResolvedStep{ ... }, nil
}
```

**Why not `StepExecutor.CaptureClaudeStatusLine() bool`?** Rejected in iteration 1: the flag is run configuration, not runner state. Adding an accessor to the interface means every test double has to implement it (and there are many). Threading via `RunConfig` keeps `StepExecutor` focused on executor responsibilities and piggybacks on the existing `RunConfig.Env`/`ContainerEnv` pattern.

### 4.8 Startup cleanup (the only cleanup mechanism)

**File:** `src/cmd/pr9k/main.go`

**`main` has four `os.Exit(...)` sites (lines 120, 129, 305, 310), one `os.Exit(0)` on success (line 312), and one plain `return` on the cobra `--help` early-exit path (line 123).** `os.Exit` skips deferred calls; the plain `return` does not. So deferred cleanup would run for `--help` but not for any other termination path, producing inconsistent behavior. That alone disqualifies shutdown-time defer as the cleanup mechanism.

**Decision: startup-only cleanup.** The overlay files are:
- Non-sensitive (pr9k-owned, no secrets).
- Under `.pr9k/` which is already gitignored.
- Recreated on every claude step spawn, so a stale file from a previous run is overwritten the next time anyway.

Call `sandbox.CleanOverlay(cfg.ProjectDir)` early in `main`, unconditionally, right after `projectDir` is resolved and before any `statusline.Runner` construction:

```go
// Remove any stale overlay files from a previous crash before writing fresh
// ones per step. Unconditional so that disabling the flag after a prior run
// still cleans up. Missing files are ignored.
_ = sandbox.CleanOverlay(cfg.ProjectDir)
```

No preflight failure — cleanup is best-effort. Stale `statusline-current.json` from a previous run is overwritten on the shim's next atomic rename; stale `sandbox-settings.json` and `statusline-shim.sh` are overwritten by `WriteOverlay` on the next claude step spawn. The startup cleanup is purely a "keep `.pr9k/` tidy" concern, not a correctness concern.

**Alternative considered (and rejected):** inject cleanup calls before each `os.Exit(...)` in `main`. Rejected because it sprays the cleanup logic across three unrelated exit paths and still leaves one uncovered (the signal-handler goroutine calls `program.Kill()` but the final `os.Exit` is in `main`; under heavy signal-handling load this is racy to reason about). Startup cleanup is idempotent and covers every case with one line.

### 4.9 Payload marshaling — add `claude.native`

**File:** `src/internal/statusline/payload.go:5-50`

```go
type claudePr9kJSON struct {
    IngestedAt string `json:"ingested_at"`
    AgeSeconds int    `json:"age_seconds"`
}

type claudeJSON struct {
    Native json.RawMessage `json:"native,omitempty"`
    Pr9k   *claudePr9kJSON `json:"pr9k,omitempty"`
}

type payloadJSON struct {
    SessionID     string            `json:"sessionId"`
    Version       string            `json:"version"`
    Phase         string            `json:"phase"`
    Iteration     int               `json:"iteration"`
    MaxIterations int               `json:"maxIterations"`
    Step          stepJSON          `json:"step"`
    Mode          string            `json:"mode"`
    WorkflowDir   string            `json:"workflowDir"`
    ProjectDir    string            `json:"projectDir"`
    Captures      map[string]string `json:"captures"`
    Claude        *claudeJSON       `json:"claude,omitempty"`
}

// BuildPayload grows two new trailing parameters carrying the Claude
// snapshot the caller observed on the runner (§4.6 load site). Both empty
// values together → no Claude ingest has happened yet; the `claude` field is
// omitted entirely (cold-start behavior documented in §4.9 shape guarantee).
// A non-empty claudeRaw is assumed valid (validated at ingest in claudeWatcher).
//
// claudeAt is the host wall-clock time the watcher captured when it stored
// the snapshot. It is rendered as RFC3339 in UTC so user scripts get a stable
// format regardless of host timezone:
//     claudeAt.UTC().Format(time.RFC3339)
// age_seconds is computed at Marshal time against time.Now() (also UTC-safe
// since it is a duration).
func BuildPayload(s State, mode string, claudeRaw json.RawMessage, claudeAt time.Time) ([]byte, error) {
    // ... existing body, plus:
    p.Claude = nil
    if len(claudeRaw) > 0 {
        pr9k := &claudePr9kJSON{
            IngestedAt: claudeAt.UTC().Format(time.RFC3339),
            AgeSeconds: int(time.Since(claudeAt).Seconds()),
        }
        p.Claude = &claudeJSON{Native: claudeRaw, Pr9k: pr9k}
    }
    return json.Marshal(p)
}
```

**Callsite churn — existing `BuildPayload` tests.** The two-new-trailing-args signature breaks every current caller; each needs a trailing `nil, time.Time{}`. Grep of today's tree shows ten callers in `src/internal/statusline/statusline_test.go` (lines 43, 64, 85, 103, 137, 348, 353, 391, 395, and the T5 path at 1024) plus the single production caller in `execScript` (§4.6 load site). Mechanical update — no assertion changes because the new fields are additive and absent when `claudeRaw` is nil.

**Shape guarantee:**

- Flag off → `claude` field entirely absent from stdin payload (documented).
- Flag on, pre-first-shim-invocation → `claude` field absent (documented cold-start gap).
- Flag on, after first shim invocation → `claude.native` is Claude's full payload verbatim; pr9k adds a sibling `claude.pr9k` sub-object with pr9k-observed metadata.

**Stale-payload signaling (`claude.pr9k.ingested_at` / `claude.pr9k.age_seconds`).** `claudePayload` holds the most recent payload pr9k has ever received from any claude step. Between steps, it is stale. Between iterations, more stale. User scripts distinguish fresh from stale data by reading the sibling `claude.pr9k.*` fields the Marshal block above emits:

```json
"claude": {
  "native": { ... Claude's payload verbatim ... },
  "pr9k": {
    "ingested_at": "2026-04-20T15:04:05Z",
    "age_seconds": 12
  }
}
```

The `claudePayloadSnapshot` type defined in §3.4/§4.5 carries both values; the atomic pointer supplies release-acquire between writer (watcher) and reader (`execScript`). The UTC-RFC3339 rendering ensures host-timezone independence.

User scripts read with defensive `jq`: `.claude.native.rate_limits.five_hour.used_percentage // empty`, and may further gate on staleness: `if (.claude.pr9k.age_seconds // 999) < 60 then ... else empty end`.

### 4.10 Default workflow script — demonstration update

**File:** `workflow/scripts/statusline`

Add a short `jq` snippet near the existing fields, guarded by field presence:

```bash
RATE_LIMIT=""
if echo "$input" | jq -e '.claude.native.rate_limits.five_hour' >/dev/null 2>&1; then
    PCT=$(echo "$input" | jq -r '.claude.native.rate_limits.five_hour.used_percentage')
    RATE_LIMIT=" | 5h: ${PCT}%"
fi
```

And append `${RATE_LIMIT}` to the final format string. Zero breakage when the field is absent.

### 4.10b Wiring-layer changes — `cmd/pr9k/wiring.go`

Two small conversions live in `wiring.go`:

**`buildStatusLineConfig` (line 61-69)** — copy the new flag from `steps.StatusLineConfig` into `statusline.Config`. Nil-safe:

```go
func buildStatusLineConfig(slc *steps.StatusLineConfig) *statusline.Config {
    if slc == nil {
        return nil
    }
    cap := false
    if slc.CaptureClaudeStatusLine != nil {
        cap = *slc.CaptureClaudeStatusLine
    }
    return &statusline.Config{
        Command:                 slc.Command,
        RefreshIntervalSeconds:  slc.RefreshIntervalSeconds,
        CaptureClaudeStatusLine: cap,
    }
}
```

**`buildRunConfig` (line 74-87)** — mirror the flag into `workflow.RunConfig`:

```go
func buildRunConfig(cfg *cli.Config, stepFile steps.StepFile, statusRunner workflow.StatusRunner, logWidth int, runStamp string) workflow.RunConfig {
    cap := false
    if stepFile.StatusLine != nil && stepFile.StatusLine.CaptureClaudeStatusLine != nil {
        cap = *stepFile.StatusLine.CaptureClaudeStatusLine
    }
    return workflow.RunConfig{
        // ... existing fields ...
        CaptureClaudeStatusLine: cap,
    }
}
```

**Dual-source rationale:** the flag needs to be visible at two distinct timings: when `statusline.Runner.Start` runs (to start the watcher), and when `buildStep` runs for each claude step (to write the overlay). Reading from a single source at `statusline.Runner` (e.g., having buildStep call `executor.Statusline().CaptureClaude()`) would couple workflow and statusline packages. Splitting the flag across two config channels is the cleaner boundary; both derive from the same source-of-truth `StatusLineConfig` in `stepFile`.

**Test:** `wiring_test.go` already exercises both builders (TP-004, TP-006). Add two table entries verifying the new flag round-trips through each builder for each of {nil, false, true}.

### 4.11 Dependencies

**Go module — new direct dependency:** `github.com/fsnotify/fsnotify` is **not** currently in `src/go.mod` (verified — today's `go.mod` has 10 direct deps; fsnotify is not one). Adding it is a net-new dep. Pin to the latest stable (`v1.7.0` at time of writing, current `1.x` is backwards-compatible). Transitive cost is minimal: `golang.org/x/sys` (already a direct dep) is the main one it pulls.

Steps:
1. `cd src && go get github.com/fsnotify/fsnotify@latest`
2. `make mod-tidy`
3. Confirm `make ci` passes — `make ci` includes `go-vulncheck` which will flag any known CVEs in the pinned version.

**Supply-chain consideration:** fsnotify is maintained by the `fsnotify` organization (formerly `howeyc/fsnotify`); it is used by Kubernetes, Prometheus, HashiCorp, etc. Pin a specific version in `go.mod` rather than a floating major. `make vulncheck` must pass.

---

## 5. Config and schema documentation

### 5.1 Public-API versioning

Per `docs/coding-standards/versioning.md:13-28`, pr9k's formal public API is:

1. CLI surface
2. `config.json` schema
3. `{{VAR}}` language
4. `--version` output

The **statusLine stdin payload** is documented (`docs/features/status-line.md`) but not in the public-API list. Before this feature ships, amend `versioning.md` to explicitly classify `claude.native` as:

> **Pass-through surface** — pr9k-authored keys in the statusLine stdin payload are stable under the `config.json` schema. The `claude.native` sub-object is a verbatim pass-through of Claude's own statusLine payload; pr9k does not version its contents and does not guarantee schema stability across Claude CLI versions.

This is a non-blocking doc edit, but ship it in the same PR (`docs/coding-standards/versioning.md`).

The **`config.json` field `captureClaudeStatusLine`** is a new public-API surface under item 2; its rules once shipped:

- Default: absent/nil/false → feature off.
- Accepted values: `true`, `false`, absent.
- No interaction with other fields.
- Can be added without a major bump (0.y.z rules; additive field).

### 5.2 New docs to write

| Doc | Purpose | Size |
|---|---|---|
| `docs/features/status-line.md` (update) | New section documenting the `captureClaudeStatusLine` field, `claude.native` payload shape, cold-start caveat, pass-through versioning note | ~60 lines added |
| `docs/how-to/capturing-claude-statusline-data.md` (new) | End-to-end how-to: enable the flag, example jq consumption, what fields are available, how to detect cold-start | ~100 lines |
| `CLAUDE.md` | Add the new how-to under "How-To Guides" | 1 line |
| `docs/coding-standards/versioning.md` | Add the pass-through surface clause | ~6 lines |

No new ADR is required (the feature is additive, gated, reversible, and stays inside the existing sandbox contract per ADR `20260413160000-require-docker-sandbox.md`).

### 5.3 Feature doc structure — content outline

For `docs/how-to/capturing-claude-statusline-data.md`:

1. **Why use this?** — two-sentence framing: access Claude-only fields like rate limits and lines-added/removed from your statusLine script.
2. **Enable it** — add `"captureClaudeStatusLine": true` to `statusLine` in `config.json`.
3. **The new field** — `claude.native` in your script's stdin; structure mirrors https://code.claude.com/docs/en/statusline.md.
4. **A working example** — `jq` snippet reading `rate_limits.five_hour.used_percentage` with defensive presence check.
5. **Cold-start behavior** — field absent until Claude's first assistant message.
6. **What it costs** — one extra file-level bind mount per claude step; three small file writes; one host goroutine.
7. **Troubleshooting** — check the pr9k log file for `[statusline] claude watcher` lines; confirm `.pr9k/sandbox-settings.json` exists mid-run; `claude --debug-file` to check the overlay was seen.

---

## 6. Non-functional requirements

### 6.1 Performance

- **Host RSS impact:** one fsnotify goroutine + ~16 KB payload cell = negligible.
- **Per-step I/O:** `sandbox-settings.json` (~100 bytes) + `statusline-shim.sh` (~150 bytes) written once per claude spawn. ~20 writes per typical run. Irrelevant.
- **Shim execution cost:** `cat` + `mv` per Claude trigger. Under 5 ms VirtioFS end-to-end.
- **Host watcher wake-ups:** bounded by Claude's 300 ms debounce. Worst case ~200 events/minute.

### 6.2 Security

- **Overlay file permissions:** `0644` for settings (readable-by-container-user, world-readable; contains no secrets), `0755` for shim (executable).
- **No new host secrets cross the container boundary.** The overlay's `command` points inside the mounted workspace; no env vars added to `BuildRunArgs`.
- **No escalation path.** The shim runs as the container user (host UID/GID). Its only privilege is writing to `.pr9k/` — which it already has via the existing project bind mount.
- **Trust model unchanged.** pr9k still trusts the statusLine script (runs on host with full env). The Claude-supplied `native` payload is JSON and is never `eval`'d — users read it with `jq`.
- **Malformed payload handling:** `json.Valid` gate at ingest drops non-JSON bytes. Payload-size cap at 16 KB prevents runaway allocation.

### 6.3 Portability

- **macOS Docker Desktop (VirtioFS, ≥4.39):** the primary validated target. File-level bind mount and container-write-to-host-fsnotify confirmed supported (`research.md` Q-OPEN-5, §14).
- **Linux native Docker (inotify):** file-level bind mount is a first-class `docker run` feature; container-originated writes reach host inotify watchers via the kernel inode.
- **Windows:** out of scope (matches existing `docs/features/status-line.md:196`).
- **fsnotify on both platforms:** inotify on Linux, FSEvents on macOS — both bundled by the `fsnotify` library with no build tags required.

**Non-Docker-Desktop macOS VMs (Colima, OrbStack, Lima, Rancher Desktop):** explicitly **unsupported for this feature** in v0.8.0. These products run Docker inside a separate VM (Linux VM on top of macOS) with their own FS-passthrough implementations (often 9p, sshfs, or virtiofs over vsock). Container-to-host-VM-to-macOS-FSEvents propagation is not guaranteed; the shim writes the file but the host watcher may never fire. Documented in `docs/how-to/capturing-claude-statusline-data.md` as a known limitation.

**Future fallback:** if demand emerges for non-Docker-Desktop VMs, a low-rate polling fallback (e.g., 1 s `os.Stat` loop when fsnotify has been silent for N seconds since a known Claude invocation) is a one-file addition that does not affect the primary path. Defer to a follow-up PR.

---

## 7. Test plan

### 7.1 Unit tests (race-detector enabled — `go test -race`)

**`sandbox/statusline_overlay_test.go`** (new)

- `TestWriteOverlay_CreatesBothFiles` — verify both files exist with correct modes (0644, 0755) after one call.
- `TestWriteOverlay_IsIdempotent` — two consecutive calls succeed; file contents stable.
- `TestWriteOverlay_CleansTrailingState` — pre-create a corrupt `sandbox-settings.json`; WriteOverlay overwrites it cleanly.
- `TestWriteOverlay_HonorsRestrictiveUmask` — temporarily set 0077 umask; assert `0755` on shim via `os.Chmod`.
- `TestCleanOverlay_RemovesAllThreeFiles` — after writing + creating a fake current.json, CleanOverlay removes all three.
- `TestCleanOverlay_IsIdempotentOnEmptyDir` — no error when files don't exist.
- `TestCleanOverlay_ReportsUnexpectedError` — simulate permission-denied (chmod 0500 parent dir) and assert error is returned.

**`sandbox/command_test.go`** (extend)

- `TestBuildRunArgs_WithOverlayPath_AddsFileMount` — overlay path non-empty; assert argv contains `--mount type=bind,source=<path>,target=/home/agent/.claude/settings.json`.
- `TestBuildRunArgs_WithoutOverlayPath_NoChange` — overlay path empty; argv unchanged vs. today's golden (only the existing mount flags appear).
- `TestBuildRunArgs_OverlayMountOrdering` — pin the overlay mount argv index is greater than both the project-mount and profile-mount indices (parent-directory-first Docker constraint).
- `TestBuildRunArgs_OverlayMountExactFormat` — assert the full flag is `--mount type=bind,source=<abs-path>,target=/home/agent/.claude/settings.json` byte-for-byte (format pinning; no `:` confusions, no missing commas).
- `TestBuildRunArgs_OverlayEmptyParamBackwardCompatibility` — call with `""` and assert argv length + contents match today's golden before the signature change.

**Impact of `BuildRunArgs` signature change on existing tests:** every existing call site in `command_test.go` (~20 calls), `image_test.go`, and `workflow/run.go` gains a trailing `""`. Mechanical update. The existing golden-argv assertions stay correct because `""` is a no-op; tests that count `-e`/`--mount` flags need to be checked for off-by-one drift.

**`statusline/claude_watcher_test.go`** (new)

- `TestClaudeWatcher_ObservesAtomicRename` — create watcher, simulate shim by atomic-rename-writing a tmp file, assert payload is stored and `trigger()` was called.
- `TestClaudeWatcher_DropsInvalidJSON` — write `not-json` atomically, assert no store, assert log line written.
- `TestClaudeWatcher_DropsOversizePayload` — write 20 KB file, assert no store, assert log line.
- `TestClaudeWatcher_SurvivesRapidWrites` — atomic-rename 10× in a loop; assert last payload is stored; run with `-race`.
- `TestClaudeWatcher_StopIsIdempotent` — call `stop()` twice; no panic.
- `TestClaudeWatcher_StartOnMissingDirCreatesIt` — call with a nonexistent `.pr9k/`; assert it's created and watcher functions.
- `TestClaudeWatcher_IgnoresUnrelatedFiles` — write `other.json` in the watched dir; assert no store.
- `TestClaudeWatcher_IgnoresTmpFile` — write `statusline-current.json.tmp` atomically (without the rename); assert no store (only the final filename triggers ingest).
- `TestClaudeWatcher_RapidConcurrentWritesAndReadsRace` — writer goroutine atomic-renames in a loop; reader goroutine calls `payload.Load()` in a loop and dereferences `.raw` / `.at`; run with `-race` to verify `atomic.Pointer[claudePayloadSnapshot]` serialization across both fields.
- `TestClaudeWatcher_HandlesWatcherErrorChannel` — inject a synthetic error on `watcher.Errors`; assert log line written; assert goroutine continues.

**`statusline/payload_test.go`** (extend)

- `TestBuildPayload_ClaudeNativeAbsentWhenNil` — nil claudeRaw (and zero-value `claudeAt`) → no `claude` field in JSON.
- `TestBuildPayload_ClaudeNativeEmbeddedVerbatim` — pass a json.RawMessage; assert it appears at `.claude.native` byte-for-byte.
- `TestBuildPayload_ClaudeNativeNoDoubleMarshaling` — ensure the raw message is not string-quoted.
- `TestBuildPayload_ClaudePr9kEmitsUTCRFC3339` — pass a non-zero `claudeAt` in a non-UTC location; assert `.claude.pr9k.ingested_at` is UTC and parses cleanly via `time.Parse(time.RFC3339, ...)`.
- `TestBuildPayload_ClaudePr9kAgeSecondsIsRelative` — pass `claudeAt = time.Now().Add(-30*time.Second)`; assert `.claude.pr9k.age_seconds` is within `[29, 31]` (window tolerates scheduler jitter).

**`statusline/statusline_test.go`** (extend)

- `TestRunner_CaptureClaudeOffByDefault` — default config → runner does not start watcher, log is quiet.
- `TestRunner_CaptureClaudeOnStartsWatcher` — flag on → watcher started; Shutdown closes it cleanly.
- `TestRunner_WatcherFailureIsNonFatal` — simulate inotify-fail (write-protected dir); runner still processes user-script triggers.

**`validator/validator_test.go`** (extend)

- `TestValidator_CaptureClaudeStatusLineAcceptsTrueFalseAbsent` — all three shapes pass.
- `TestValidator_CaptureClaudeStatusLineRejectsNonBool` — `"yes"` or `1` fail parse via `DisallowUnknownFields` + JSON type-check.
- `TestValidator_CaptureClaudeStatusLineWithoutCommand` — `{ "statusLine": { "captureClaudeStatusLine": true } }` (no `command` field) still rejected because `command: ""` is an error. Pins the "no cross-field constraint needed" claim.

**`workflow/run_test.go`** (extend)

- `TestBuildStep_WriteOverlayRecreatesDeletedShim` — call `buildStep` twice: between calls, delete `.pr9k/statusline-shim.sh` on disk. Assert the second call re-creates it with the correct content and mode `0755`. Covers R9 (mid-run file deletion) in the test plan rather than only in risk narrative.
- `TestBuildStep_FlagOffSkipsOverlay` — flag false → no overlay files written, no bind-mount in argv.
- `TestBuildStep_FlagOnFailedWriteReturnsError` — simulate permission-denied on `.pr9k/` (chmod 0500); assert buildStep returns an error of the form `step "X": overlay: ...` and no partial overlay files remain.

### 7.2 Integration tests

**`src/internal/sandbox/sandbox_integration_test.go`** (new, build-tagged `integration`)

Skip if `$SKIP_DOCKER` or docker is unavailable. Uses the real `docker/sandbox-templates:claude-code` base image; a tiny fake statusLine program takes the place of Claude for in-CI runs.

- `TestSandboxIntegration_OverlayShadowsUserSettings` — given a user profile dir with a real `settings.json` containing `statusLine.command: "user-cmd"`, start a container with pr9k's overlay, assert the container sees pr9k's settings (via `cat /home/agent/.claude/settings.json` inside an `sh -c` wrapper run).
- `TestSandboxIntegration_ShimAtomicRenamePropagates` — inside the container, execute the shim with a canned stdin; on the host, assert `statusline-current.json` appears with the expected bytes AND a watcher on the host sees the fsnotify event (this is the platform-gated test that actually exercises VirtioFS, not just the host filesystem).
- `TestSandboxIntegration_OverlayMountOrderingDocker` — flip the file-mount and dir-mount order in the argv and assert `docker run` fails. Locks the Docker ordering requirement cited in §4.4.

**Platform matrix:** this suite must run on **both** macOS (Docker Desktop VirtioFS) and Linux (native Docker) in CI. A platform-specific event-type assertion lives inside `TestSandboxIntegration_ShimAtomicRenamePropagates`:

```go
if runtime.GOOS == "darwin" { assertFSEventSeenWithin(5 * time.Second) }
if runtime.GOOS == "linux"  { assertInotifyEventSeenWithin(5 * time.Second) }
```

**`src/internal/statusline/e2e_test.go`** (new, build-tagged `integration`)

- `TestE2E_WatcherReceivesShimWrite` — host-only: spin up a real fsnotify watcher + simulate the shim's exact `cat | mv` semantics from a goroutine. Not a substitute for the Docker-integration case but a fast regression for the watcher logic.

**`src/internal/sandbox/claude_e2e_test.go`** (new, build-tagged `e2e`, nightly-only)

- `TestE2EClaude_StatusLinePayloadEndToEnd` — requires `CLAUDE_CONFIG_DIR` to point at a real authenticated profile on the CI runner. Spawns the real `claude` binary via `BuildRunArgs` with the overlay on, sends a trivial prompt, asserts pr9k's host-side fsnotify captures a payload that (a) parses as JSON, (b) contains `session_id` and `model` keys (Claude schema), (c) matches Claude's current documented shape for at least those two keys. Pinned to a specific Claude CLI version via CI env so schema drift is detectable.
- **Gated on non-nightly CI:** skipped unless `$PR9K_E2E=1`. Prevents burning Claude API credits on every PR while still catching Claude-side schema breakage on a regular cadence.

### 7.3 Manual acceptance

Before marking the PR ready:

1. On macOS Docker Desktop, run pr9k with the flag on against a simple two-step workflow. Verify `claude.native.rate_limits.five_hour.used_percentage` appears in the statusline output mid-run.
2. On Linux, repeat.
3. Run with the flag off; confirm no bind-mount line in the docker argv (via `docker inspect` on the running container) and no overlay files appear in `.pr9k/`.
4. Crash pr9k mid-run (`kill -9`); verify stale `.pr9k/statusline-*` files are removed on the next startup.

---

## 8. Rollout

### 8.1 Single PR, single landable change

All code + docs + version bump in one PR. No phased gates, no feature flag beyond the config field.

### 8.2 Commit sequence (suggested)

1. Add `fsnotify` to `go.mod`; `make mod-tidy`.
2. Add `statusline_overlay.go` (with `atomicWrite` helper) + tests (overlay writer atomicity, CleanOverlay).
3. Extend `BuildRunArgs` signature + golden tests update (24 call-site updates).
4. Threading: `StatusLineConfig` / `vStatusLine` / `steps.LoadSteps` changes + validator tests.
5. Add `claude_watcher.go` + tests (including platform-gated integration test).
6. Wire watcher into `statusline.Runner.Start`/`Shutdown` + `BuildPayload` change with staleness timestamp + tests.
7. Thread `CaptureClaudeStatusLine` through `wiring.go` (`buildStatusLineConfig` + `buildRunConfig`) and add new parameter to `buildStep` + update all 20 call sites (3 production in `run.go` + 15 in `run_test.go` + 2 in `run_timeout_test.go`).
8. Call `sandbox.WriteOverlay` in `workflow/run.go` claude step path.
9. Update `workflow/scripts/statusline` demo to read `claude.native` and `claude.pr9k.age_seconds`.
10. Documentation: new how-to + feature doc update + CLAUDE.md index + versioning clause.
11. Bump version from `0.7.0` → `0.8.0` per `versioning.md` §4 (minor: backwards-compatible additive CLI/config surface).

Commits 2–8 each land with green tests. Commits 9–11 are doc/demo only.

### 8.3 Version bump

Per `docs/coding-standards/versioning.md` §32-38, during `0.y.z` any additive change to the `config.json` schema bumps **MINOR** (not patch). Bump `0.7.0` → `0.8.0` in its own commit (`Bump version to 0.8.0`), after the implementation commits land.

---

## 9. Risks and mitigations

| # | Risk | Likelihood | Severity | Mitigation |
|---|---|---|---|---|
| R1 | Silent malformed overlay — Claude tolerates bad JSON silently (`research.md` Q8 bonus) | Low | Med | `json.Valid` round-trip in `WriteOverlay` before returning |
| R2 | Docker Desktop regression breaks file-over-directory overlay | Low | High | Integration test on both macOS and Linux; document platform matrix in feature doc; fallback guidance: swap to project-local `.claude/settings.json` write (still an overlay semantically) |
| R3 | Claude CLI changes statusLine payload schema | Med | Low | Pass-through versioning note — user script consumes at own risk; publish the Claude version we validated against |
| R4 | Shell shim fails on a restricted base image (coreutils stripped) | Low | Med | Document the Go-binary fallback in `research.md` §6.2; swap `WriteOverlay` to write a static Go binary instead |
| R5 | fsnotify inotify-watch limit exceeded on busy systems | Low | Low | Watcher failure is non-fatal; logged, runner continues without Claude capture |
| R6 | Concurrent shim invocations (Claude cancel-and-restart) produce torn writes | Low | Low | PID-qualified tmp (`.tmp.$$`) prevents two shims from writing the same inode; POSIX `rename(2)` atomicity handles the final step; `set -eu && cat ... && mv` aborts on `cat` failure |
| R7 | Cold-start gap confuses users | Med | Low | Document prominently; sample script handles absent field defensively |
| R8 | Users enable flag, then copy-paste a statusLine script that crashes on missing `claude.native` | Med | Low | Default demo script in `workflow/scripts/statusline` uses the defensive pattern |
| R9 | `.pr9k/` deleted mid-run by the user | Very low | Low | Per-step `WriteOverlay` recreates on next claude spawn; current.json re-appears on next Claude trigger |
| R10 | JSON round-trip allocates — GC pressure under high trigger rate | Very low | Low | Claude's 300 ms debounce caps rate; 1–2 KB allocations/event are ~6 KB/s worst case |
| R11 | Two pr9k processes in the same `projectDir` race on overlay files | Very low | Low | Shim uses atomic rename so writes are safe; sandbox-settings.json is a fixed byte sequence so concurrent writes produce the same output; no lockfile added in this PR. Document as non-supported in status-line.md. |
| R12 | Docker bind-mount argv-ordering regression in a future Docker release | Very low | High | Integration test (§7.2) catches a regression; design §4.4 lists fallback: project-local `.claude/settings.json` write, per `research.md` Q-OPEN-3 |
| R13 | User mutates Claude settings via `/config` inside a captureClaudeStatusLine run — the write lands on pr9k's overlay file, not `~/.claude/settings.json`, and is silently reverted on next `WriteOverlay` (per-step) or `CleanOverlay` (next startup) | Low | Low | Document in the how-to that in-container `/config` edits do not persist while the flag is on; user's real profile file is unaffected (which is a feature, but surprising). If surfaces as common confusion later, consider a user-visible TUI log line on first Claude write to the overlay file |

---

## 10. Out of scope / explicitly deferred

These are **NOT** shipping with this PR. Each is independently addable later without changing anything in this design:

- **Typed convenience fields** — `claude.session_id`, `claude.model`, `claude.cost_usd` pulled out of `claude.native` for less brittle scripts. Add on user request.
- **Option 2a (Renderer.Finalize summary string)** — expose pr9k's own per-step summary line as `claude.last_summary`. Cheap; add on request.
- **Option 2 (derive-from-stream)** — in-flight token/cost approximations between Claude triggers. Only useful to close the cold-start gap; not needed today.
- **Static Go shim binary** — fallback if base image ever drops coreutils. Swap in when that happens.
- **Windows support** — existing statusLine feature is Unix-only; no change.
- **Multiple Claude statusLine payloads** — if Claude ever emits per-step-type payloads, we pass through the latest regardless.

---

## 11. Open questions (remaining after research)

- **Q1 (non-blocking):** Should `CaptureClaudeStatusLine` be surfaced via an env var too (e.g. `PR9K_CAPTURE_CLAUDE_STATUSLINE=1`) for ad-hoc enabling without editing `config.json`? **Recommendation:** no — `config.json` is the one-true configuration surface; env-var overrides dilute the schema. Revisit if users ask.
- **Q2 (non-blocking):** Should `WriteOverlay` happen once at `Runner.Start` (Option B in §4.7) with a filesystem-watcher guard to re-write on delete, instead of per-step (Option A)? **Recommendation:** stay with per-step (simpler invariant, negligible cost).
- **Q3 (blocking at doc time, not code):** What exact wording goes into `docs/coding-standards/versioning.md` for the pass-through clause? **Recommendation:** the paragraph in §5.1 above; finalize during doc-review.

---

## 12. References

- [`research.md`](./research.md) — full feasibility trail, open-question resolutions, evidence log.
- Claude Code statusLine docs — https://code.claude.com/docs/en/statusline.md.
- ADR `docs/adr/20260413160000-require-docker-sandbox.md` — Docker is mandatory.
- ADR `docs/adr/20260418175134-pr9k-rename-and-pr9k-layout.md` — `.pr9k/` umbrella.
- `docs/features/status-line.md` — current statusLine surface.
- `docs/coding-standards/versioning.md` — versioning rules.
- `fsnotify` — https://github.com/fsnotify/fsnotify.

---

## 13. Iterative plan review — iteration log

This section records the refinement history of this design under `r-and-d:iterative-plan-review`, including agent validation findings.

### 13.1 Summary

- **Iterations completed:** 3 codebase-grounded review passes, then a stability cut, then agent-assisted validation.
- **Assumptions challenged:** 12 primary/secondary across the three iterations; 2 refuted (P5 threading path, P9 shutdown cleanup).
- **Consolidations made:** 1 — moved overlay cleanup out of `Runner.Shutdown` into a single startup-only call in `main.go` (§4.8).
- **Ambiguities surfaced and resolved:** Q1 (env-var alternative — no), Q2 (per-step vs. per-start — per-step), Q3 (versioning clause wording — in §5.1).
- **Agents run:** 2 — `r-and-d:evidence-based-investigator` (14 findings, 2 critical) and `r-and-d:adversarial-validator` (12 findings, 4 critical).

### 13.2 Iteration 1 — threading path and shutdown cleanup

- **Refuted:** P5 — adding `StepExecutor.CaptureClaudeStatusLine()` is less clean than threading via `RunConfig`. The flag is configuration, not executor state, and every test double would have to implement it. Changed to `RunConfig.CaptureClaudeStatusLine` with a new `buildStep` parameter.
- **Refuted:** P9 — `Runner.Shutdown` early-returns when `!r.enabled`, so cleanup embedded in `Shutdown` is unreliable. Moved cleanup to a single unconditional startup call in `main.go`.
- **Added:** fsnotify is net-new to `go.mod`; supply-chain note in §4.11.

### 13.3 Iteration 2 — shim robustness and watcher code hygiene

- **Refuted shim design:** without `&&`, a SIGKILL'd `cat` would still let `mv` run, atomically installing a partial file. Added `set -eu && cat ... && mv`.
- **Fixed watcher code:** missing `io` import; removed dead `sync.Locker` assertion; tightened `ingest` to use `io.ReadAll(LimitReader)`.
- **Added:** 4 new watcher tests (`IgnoresUnrelatedFiles`, `IgnoresTmpFile`, `RapidConcurrentWritesAndReadsRace`, `HandlesWatcherErrorChannel`).

### 13.4 Iteration 3 — consistency drift

- **Fixed §2/§3.1/§4.8 drift:** earlier versions described cleanup as "startup + shutdown," inconsistent with the iteration-1 change. Unified to "startup-only."
- **Added §4.10b:** wiring-layer changes explicitly call out `buildStatusLineConfig` and `buildRunConfig` updates in `wiring.go` with nil-safe pointer-bool unwrap.
- **Stopped:** iteration 4 would have been cosmetic only. Below 80% probability of meaningful structural improvement.

### 13.5 Agent validation — evidence-based-investigator findings

| # | Severity | Finding | Action taken |
|---|---|---|---|
| E1 | Critical | `main()` has 5 `os.Exit` sites + 1 `return`, not 3 | Corrected §4.8 narrative |
| E2 | Critical | `os.IsNotExist` vs. `errors.Is(err, fs.ErrNotExist)` violates standard | Changed to `errors.Is`; added `fs` import |
| E3 | Important | `image_test.go` has 0 `BuildRunArgs` callers, not the claimed ~20-plus | Corrected to "23 + 1" |
| E7 | Important | No lockfile for `.pr9k/` — concurrent pr9k instances race | New R11 risk documented |
| E8 | Important | File-over-directory bind-mount ordering has no in-tree evidence | Added requirement for integration test + §7.2 `TestSandboxIntegration_OverlayMountOrderingDocker` |
| E13 | Minor | `StatusLineConfig` struct line range was 51-57, not 51-57 struct body | Citation corrected |

### 13.6 Agent validation — adversarial-validator findings

| # | Severity | Finding | Action taken |
|---|---|---|---|
| V1 | Critical | `os.WriteFile` for overlay creates a torn-read window via `O_TRUNC` | Replaced with `atomicWrite` (tmp-file + rename), §4.3 |
| V2 | Critical | FSEvents event type varies by macOS version; unit test on host FS doesn't exercise bind mount | Documented platform variance; integration test is the authoritative validator |
| V3 | Critical | fsnotify unreliable on Colima/OrbStack/Lima (non-Docker-Desktop Mac VMs) | Explicitly out of scope in §6.3; polling fallback documented as future work |
| V4 | Critical | Two concurrent shims `cat > tmp` race on same inode | Changed shim to use `$$` PID-qualified tmp path |
| V5 | Important | `Shutdown` order: `claudeWatcher.stop()` must run BEFORE `wg.Wait()` | Rewrote `Shutdown` with explicit ordering + rationale |
| V7 | Important | Stale payload bleeds between claude steps and iterations | Added `claude.pr9k.ingested_at` / `age_seconds` staleness signal; `atomic.Pointer[claudePayloadSnapshot]` instead of raw `RawMessage` |
| V9 | Minor | `CleanOverlay` snippet missing `errors` and `io/fs` imports | Corrected import block |
| V10 | Minor | No test for mid-run overlay-file deletion recovery | Added `TestBuildStep_WriteOverlayRecreatesDeletedShim` |
| V11 | Minor | `pr9kOwned: true` marker in overlay is unused dead metadata | Removed; §4.1 explains why |
| V12 | Minor | Integration test uses a fake statusLine, never the real Claude | Added `TestE2EClaude_StatusLinePayloadEndToEnd` (build-tagged `e2e`, nightly-only) |

### 13.7 Remaining risks after validation

Captured empirically-unverifiable items that the agents flagged and that this plan cannot resolve on paper:

- Docker Desktop VirtioFS fsnotify delivery from container→host has no in-tree empirical test today; integration tests in §7.2 are the validator. Failure mode is "silent feature inactivity," not crash.
- Claude CLI cancel-and-restart behavior for the statusLine shim is asserted by Anthropic docs but not empirically tested against pr9k's shim. V4 mitigation (PID-unique tmp) hardens the worst case.
- Docker file-over-directory mount ordering is documented by the design's integration test; if Docker changes this behavior, CI catches it.

### 13.8 Iterative plan review — second pass (2026-04-20)

Three codebase-grounded passes after the initial agent validation. Each finding was verified against the working tree at `HEAD` and fixed in-place in this plan.

| # | Severity | Finding | Fix |
|---|---|---|---|
| I1 | Important | Internal inconsistency — §4.5/§4.6 described `atomic.Pointer[json.RawMessage]`; §4.9 required `atomic.Pointer[claudePayloadSnapshot]` for staleness timestamps. Watcher, Runner, and load-site snippets disagreed on the cell type. | Unified on `atomic.Pointer[claudePayloadSnapshot]`: promoted the snapshot struct into §3.4 and §4.5 as the canonical definition; updated §4.6 Runner field and load site; fixed §2 summary and §7.1 test description to match. |
| I2 | Important | `BuildPayload(s, mode, claudeRaw)` signature in §4.9 could not produce `claude.pr9k.age_seconds` because the timestamp was nowhere in the signature. | Added `claudeAt time.Time` as a fourth parameter; documented in the inline comment; rewrote the Marshal block to derive `IngestedAt` / `AgeSeconds` from it; updated §4.6 load site to read `snap.at` and pass it through. |
| I3 | Minor | `ingested_at` JSON example ("2026-04-20T15:04:05Z") implied UTC but the plan never specified the format explicitly, and Go's default `time.Format(time.RFC3339)` uses local TZ. | Wrote the serialization as `claudeAt.UTC().Format(time.RFC3339)` with an explaining comment. |
| I4 | Minor | §4.9 silently changed the `BuildPayload` signature but — unlike §4.4 for `BuildRunArgs` — did not enumerate the existing test-caller churn. | Enumerated the ten existing callers in `statusline_test.go` (verified with `Grep`) and noted the mechanical trailing-arg update. |
| I5 | Minor | Line-number drift: §4.4 cited `run.go:731` (actual 764); §4.7 cited `buildStep` call sites 403/527/638 (actual 403/527/662); §4.7 also pointed at `run.go:716-738` for the `buildStep` body (actual 749-772); §8.2 commit 7 said "19 call sites" (actual 20 = 3 production + 15 in `run_test.go` + 2 in `run_timeout_test.go`). | Corrected all citations against the current tree. |
| I6 | Low | `CleanOverlay` removed only the three named basenames; a failed `mv` would leak `statusline-current.json.<pid>.tmp` files into `.pr9k/` forever (never a correctness issue, but litter). | Widened `CleanOverlay` to glob `statusline-current.json.*.tmp` and remove each. Updated §4.1 artifact table to reflect the sweep. |
| I7 | Low | New risk — a user running `/config` inside a claude step with the flag on writes through the overlay bind mount to pr9k's `sandbox-settings.json`, not to `~/.claude/settings.json`; the write is silently reverted by the next `WriteOverlay` or `CleanOverlay`. Not addressed in §9. | Added R13 to the risk table with a doc mitigation; the user's real `~/.claude/settings.json` is unaffected, which is the safety property. |

**Convergence check (end of iteration 3).** After I1–I7, no remaining internal inconsistency was found in the following targeted searches: all `atomic.Pointer[…]` occurrences now reference `claudePayloadSnapshot`; every `BuildPayload(` reference uses the four-argument form; every `run.go:NNN` citation matches the current tree. Iteration 4 would be cosmetic only — stopped at 3 per the skill's 80%-probability-of-meaningful-structural-improvement rule.
