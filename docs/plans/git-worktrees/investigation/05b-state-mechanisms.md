# Investigation: Persistent-State Mechanisms and Resume-State-File Integration

**Date:** 2026-05-07  
**Investigator:** Claude Code  
**Goal:** Map existing persistent-state patterns to understand exactly how a new `.pr9k/active-run.json` state file would integrate.

---

## E1: `atomicwrite.Write` — Public API and Signature

**Source:** `src/internal/atomicwrite/write.go:34`

```go
func Write(path string, data []byte, mode os.FileMode) error
```

Single exported symbol. Internally: creates `<basename>.<pid>-<nanoseconds>.tmp` alongside the target, writes with `O_CREATE|O_EXCL|O_WRONLY, 0o600`, `fsync`s, renames into place (POSIX atomic), then best-effort fsyncs the parent directory. EXDEV errors propagate unwrapped so callers can `errors.Is(err, syscall.EXDEV)`. On ENOENT (first save), it walks back to the lowest existing ancestor to find a writable parent — meaning the parent directory must already exist, but the file itself need not.

**Relevance:** This is the only write path approved by the project's file-write standards. A new `active-run.json` must go through this function.

---

## E2: `atomicwrite.Write` — Two Example Callers

**Source:** `src/internal/workflowio/save.go:119`

```go
func (realSaveFS) WriteAtomic(path string, data []byte, mode os.FileMode) error {
    return atomicwrite.Write(path, data, mode)
}
```

The `workflowio.Save` function wraps `atomicwrite.Write` behind a `SaveFS` interface for testability (`src/internal/workflowio/save.go:62–98`). All companion files and `config.json` are written via this path with mode `0o600`.

**Source (implicit):** `src/internal/atomicwrite/write.go:45` — `atomicwrite` itself uses `os.Getpid()` for temp-file naming. This is the only production call site of `atomicwrite.Write` that is not mediated through `workflowio.Save`.

**Negative evidence:** `grep -rn "atomicwrite.Write"` in production code finds exactly one direct caller: `workflowio/save.go:119`. All other writes in the codebase (e.g., `iterationlog.go`, `logger.go`) use `os.OpenFile` with `O_APPEND|O_CREATE` — those are append-only patterns, not replacement patterns, and are explicitly out-of-scope for `atomicwrite`.

---

## E3: `.pr9k/` Directory Contents and Lifecycle

Files created under `<projectDir>/.pr9k/`:

| Path | Creator | Line |
|---|---|---|
| `.pr9k/` (umbrella dir) | `preflight.Run` via `os.MkdirAll` | `src/internal/preflight/run.go:37` |
| `.pr9k/logs/` | `logger.NewLoggerWithPrefix` via `os.MkdirAll` | `src/internal/logger/logger.go:29` |
| `.pr9k/logs/<stamp>.log` | `logger.NewLoggerWithPrefix` | `src/internal/logger/logger.go:39` |
| `.pr9k/logs/<stamp>/` (artifact dir) | `main.startup` via `os.MkdirAll` | `src/cmd/pr9k/main.go:104–105` |
| `.pr9k/logs/<stamp>/<phase>-<idx>-<slug>.jsonl` | `claudestream.NewRawWriter` | `src/internal/workflow/run.go:387` |
| `.pr9k/iteration.jsonl` | `workflow.AppendIterationRecord` | `src/internal/workflow/iterationlog.go:48–49` |

**Gitignore status** (`/.gitignore`):
- `.pr9k/logs/` — ignored
- `.pr9k/iteration.jsonl` — ignored
- `.pr9k/artifacts/` — ignored (legacy path)
- `.pr9k/workflow/` — NOT in gitignore (it is the in-repo workflow override, committed by choice)
- `.pr9k/active-run.json` — not currently listed (would need to be added)

**Timing:** `preflight.Run` is called at `src/cmd/pr9k/main.go:70` (inside `startup()`), before `logger.NewLogger` (line 96), before the artifact dir mkdir (line 104–105). So `.pr9k/` is guaranteed to exist before any subsequent write to it.

---

## E4: JSON Schemas in `.pr9k/` — NDJSON vs Single-Object Conventions

**iteration.jsonl** (`src/internal/workflow/iterationlog.go:15–27`):

```go
type IterationRecord struct {
    SchemaVersion int     `json:"schema_version"`
    IssueID       string  `json:"issue_id"`
    IterationNum  int     `json:"iteration_num"`
    StepName      string  `json:"step_name"`
    Model         string  `json:"model,omitempty"`
    Status        string  `json:"status"`
    DurationS     float64 `json:"duration_s"`
    InputTokens   int     `json:"input_tokens,omitempty"`
    OutputTokens  int     `json:"output_tokens,omitempty"`
    SessionID     string  `json:"session_id,omitempty"`
    Notes         string  `json:"notes,omitempty"`
}
```

Written with `O_APPEND` — NDJSON format (one JSON object per line, growing file, never replaced). Uses `schema_version: 1` for future compatibility.

**Per-step `.jsonl` artifacts** (`src/internal/claudestream/` — raw NDJSON event streams from the claude API). Also O_TRUNC on retry (documented exception to the file-write standard).

**Negative evidence:** No other single-object JSON files exist under `.pr9k/` today. The new `active-run.json` would be the first single-object JSON file written there. The convention for single-object durable state is: `atomicwrite.Write` + `json.Marshal` (same pattern as `workflowio.Save` uses for `config.json`).

---

## E5: Validator — `DisallowUnknownFields` and Lockstep Change Requirements

**Source:** `src/internal/validator/validator.go:204–210`

```go
dec := json.NewDecoder(bytes.NewReader(data))
dec.DisallowUnknownFields()
var vf vFile
if err := dec.Decode(&vf); err != nil {
    return []Error{cfgErr("parse", "config", "", ...)}
}
```

`vFile` (`validator.go:111–119`) is the strict struct for `config.json`. Adding a new top-level field (e.g., `resumePolicy`) requires changes in **four lockstep locations**:

1. **`vFile`** in `src/internal/validator/validator.go:111` — add the field with JSON tag, else the decoder returns an unknown-fields error and blocks startup.
2. **`steps.StepFile`** in `src/internal/steps/steps.go:104–113` — the runtime load struct; add field so `LoadSteps` unmarshals it.
3. **`workflowmodel.WorkflowDoc`** in `src/internal/workflowmodel/model.go:71–79` — the TUI editor's in-memory model; add field (as pointer for optional blocks, per existing `*StatusLineBlock`, `*DefaultsBlock` pattern).
4. **`docToVFile`** in `src/internal/validator/validator.go:215–244` — the scaffold-fallback conversion; add field mapping from `WorkflowDoc` to `vFile`.

`validateVFile` at `validator.go:297` would also need a new validation block if the field has constraints.

---

## E6: `workflowmodel` — Canonical Config Type and Optional-Block Conventions

**Source:** `src/internal/workflowmodel/model.go:71–79`

```go
type WorkflowDoc struct {
    DefaultModel string
    StatusLine   *StatusLineBlock    // pointer — optional block
    Defaults     *DefaultsBlock      // pointer — optional block
    Env          []string
    ContainerEnv map[string]string
    Steps        []Step
}
```

**Pattern:** Optional top-level blocks are **pointer types** (`*StatusLineBlock`, `*DefaultsBlock`). A nil pointer means the block is absent from `config.json`. Required arrays (`Env`, `Steps`) use slice types (nil slice = empty list). A new `ResumePolicy *ResumePolicyBlock` field should follow the pointer pattern.

---

## E7: Crash-Temp Detection Pattern (Closest Analogue to Stale Active-Run Detection)

**Source:** `src/internal/workflowio/crashtemp.go:37–79`

```go
func DetectCrashTempFiles(workflowDir string) ([]CrashTempFile, error) { ... }
```

`CrashTempFile` carries `Path`, `PID`, `MTime`, and `Classification` (Active vs Crash). Classification is via `syscall.Kill(pid, 0)`: if the signal succeeds, the PID is still alive; if it fails (ESRCH), the process has exited. This is the **exact mechanism** for a stale active-run file: write `{"pid": <os.Getpid()>, "stamp": "<runStamp>", "start_time": "<rfc3339>"}` at startup; on the next startup, read the file, check liveness of the PID, and surface "stale run from <stamp>" if the PID is dead.

**PID reuse caveat:** `crashtemp.go:35` documents this as a known accepted limitation. The same acceptance applies to the active-run file.

**Relevance:** The pattern is already designed, documented, and tested in this codebase. The active-run detection function would mirror `classifyPID` at `crashtemp.go:134–140`.

---

## E8: CLI Flag Addition Pattern

**Source:** `src/internal/cli/args.go:17–22` (Config struct) and `src/internal/cli/args.go:134–136` (flag registration)

```go
type Config struct {
    Iterations  int
    WorkflowDir string
    ProjectDir  string
}
```

```go
cmd.Flags().IntVarP(&cfg.Iterations, "iterations", "n", 0, "...")
cmd.Flags().StringVar(&cfg.WorkflowDir, "workflow-dir", "", "...")
cmd.Flags().StringVar(&cfg.ProjectDir, "project-dir", "", "...")
```

Pattern: add field to `cli.Config`, register with `cobra`'s `cmd.Flags().*Var*` family binding directly to the field. A `--fresh` / `--no-resume` flag would be `BoolVar(&cfg.Fresh, "fresh", false, "...")` added at `args.go:136`.

**Downstream:** `buildRunConfig` in `src/cmd/pr9k/wiring.go:74` constructs `workflow.RunConfig` from `cli.Config`. A new `cfg.Fresh` field would flow through `RunConfig` the same way `cfg.Iterations` does via `RunConfig.Iterations` (`wiring.go:76`).

---

## E9: `Runner` Struct and Constructor — Where a `resumeFromStamp` Field Would Slot

**Source:** `src/internal/workflow/workflow.go:36–82` (struct) and `src/internal/workflow/workflow.go:119–128` (constructor)

```go
func NewRunner(log *logger.Logger, projectDir string) *Runner {
    return &Runner{
        log:              log,
        projectDir:       projectDir,
        sessionBlacklist: make(map[string]bool),
        sendLine: func(string) {
            panic("workflow.Runner: sendLine not set — call SetSender before running steps")
        },
    }
}
```

Fields set at construction: `log`, `projectDir`, `sessionBlacklist`, `sendLine` (panic sentinel). Fields mutated during run: `currentProc`, `currentTerminator`, `procDone`, `terminated`, `timeoutFired`, `lastCapture`, `lastStats`, `activePipeline`, `activePipelineStartedAt`.

A `resumeFromStamp string` or `resumed bool` field is a clean addition: set at construction (passed from `main.go`), never mutated during the run, read by `Run` to populate `RunConfig` or emit a banner. It does not interact with any existing mutex-protected state.

---

## E10: `RunStamp()` Getter — Callers and Timing

**Source:** `src/internal/logger/logger.go:61–63`

```go
func (l *Logger) RunStamp() string {
    return l.runStamp  // set once at construction, immutable
}
```

Production callers (non-test):
- `src/cmd/pr9k/main.go:104` — builds artifact dir path immediately after `logger.NewLogger`
- `src/cmd/pr9k/main.go:234` — passes to `buildRunConfig` as `runStamp`
- `src/cmd/pr9k/wiring.go:84` — receives it as a parameter in `RunConfig.RunStamp`

`RunStamp()` is stamped at `logger.NewLoggerWithPrefix` construction time (`logger.go:36`). In the startup sequence, `logger.NewLogger` is called at `main.go:96`. The artifact dir mkdir using `log.RunStamp()` is at `main.go:104`. Writing `active-run.json` with the stamp value would slot in between lines 104 and 110 of `main.go` (after artifact dir creation, inside `startup()`).

**"Use prior stamp" mode:** To resume from a prior run, a caller would need `RunStamp()` to return a stamp read from the state file rather than a freshly generated one. The cleanest approach: pass the desired stamp into `NewLoggerWithPrefix` as its prefix/stamp parameter, or add a `NewLoggerWithStamp(projectDir, stamp string)` constructor variant that skips timestamp generation and uses the provided stamp directly.

---

## E11: `.pr9k/` Directory Creation Timing (mkdir before any state-file write)

**Source:** `src/internal/preflight/run.go:36–38`

```go
pr9kDir := filepath.Join(projectDir, ".pr9k")
if err := os.MkdirAll(pr9kDir, 0o755); err != nil { ... }
```

`preflight.Run` is called at `src/cmd/pr9k/main.go:70` (inside `startup()`), before `logger.NewLogger` at line 96 and before the artifact dir mkdir at line 104. So `.pr9k/` is fully created before any code that would write `active-run.json`. There is no race: `atomicwrite.Write` requires an existing parent directory (its ENOENT walk goes back to the first existing ancestor, so if `.pr9k/` exists, writing `.pr9k/active-run.json` will succeed on first save).

**Sequence in `startup()`:**
1. `steps.LoadSteps` (line 63)
2. `validator.Validate` (line 69) 
3. `preflight.Run` → `.pr9k/` created (line 70) ← **mkdir happens here**
4. `logger.NewLogger` → `.pr9k/logs/` created (line 96) ← could write `active-run.json` here or after
5. artifact dir `os.MkdirAll` (line 104)
6. return `services` (line 110)

---

## E12: Concurrent-Run Prevention — PID-Aware Logic Today

**Source:** `src/internal/atomicwrite/write.go:45`

```go
tempName := fmt.Sprintf("%s.%d-%d.tmp", filepath.Base(realPath), os.Getpid(), time.Now().UnixNano())
```

`atomicwrite` already embeds `os.Getpid()` in temp-file names for crash identification — this is the foundation the crash-temp detector uses.

**Source:** `src/internal/workflowio/crashtemp.go:134–140`

```go
func classifyPID(pid int) CrashTempClassification {
    err := syscall.Kill(pid, 0)
    if err == nil {
        return CrashTempActive
    }
    return CrashTempCrash
}
```

No other PID-aware logic exists in production code today (confirmed by exhaustive grep). There is no lock file, no flock, no advisory file. The active-run state file can double as a soft lock: write `{"pid": <n>, "stamp": "...", "started_at": "..."}` at startup; on next startup, read it — if `classifyPID(pid) == CrashTempActive` (same logic, already exists), refuse to run or warn. The same `syscall.Kill(pid, 0)` approach used by the crash-temp detector applies directly, with the same PID-reuse caveat.

---

## Summary Table: Integration Seams

| Concern | File:Line | Pattern |
|---|---|---|
| Write state file atomically | `atomicwrite.Write` — `src/internal/atomicwrite/write.go:34` | `atomicwrite.Write(path, jsonBytes, 0o600)` |
| Parent dir guaranteed before write | `preflight.Run` — `src/internal/preflight/run.go:36–38` | `.pr9k/` created at startup step 3 |
| Detect stale run (PID liveness) | `classifyPID` — `src/internal/workflowio/crashtemp.go:134–140` | `syscall.Kill(pid, 0)` pattern, reuse directly |
| Add `--fresh` flag | `cli.Config` + `args.go:134–136` | `BoolVar(&cfg.Fresh, "fresh", false, "...")` |
| Flow flag into Run | `wiring.go:74–87` → `RunConfig` | Add field to `RunConfig`, pass in `buildRunConfig` |
| `resumed: bool` in `Runner` | `workflow.go:36–82` + constructor `workflow.go:119` | Set at construction, no mutex needed |
| RunStamp for resumed run | `logger.go:27–48` | New constructor variant taking explicit stamp |
| Surface "RESUMED FROM" in TUI | `header.go:143–154` + `RenderIterationLine` | Append suffix to `IterationLine` or `heartbeatSuffix` pattern |
| Add top-level config field | lockstep: `vFile`, `StepFile`, `WorkflowDoc`, `docToVFile` | 4 files, pointer type for optional block |
| Gitignore the state file | `.gitignore` (line near `.pr9k/iteration.jsonl`) | Add `.pr9k/active-run.json` |
