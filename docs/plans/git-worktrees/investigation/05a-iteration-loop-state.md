# Investigation: Iteration Loop State and Cross-Run Resume

**Purpose:** Evidence-based analysis of what state lives where during a pr9k run, what would need to change to support resuming across invocations, and exactly which mechanisms exist or are missing.

---

## E1: `Run` function signature and where the VarTable is born

- **Source:** `src/internal/workflow/run.go:314-315`
- **Finding:**
```go
func Run(executor StepExecutor, header RunHeader, keyHandler *ui.KeyHandler, cfg RunConfig) RunResult {
	vt := vars.New(cfg.WorkflowDir, executor.ProjectDir(), cfg.Iterations)
```
- **Relevance:** The `VarTable` (`vt`) is constructed fresh at the start of every `Run` call. It is a local stack variable with no persistence. There is no mechanism to seed it from prior-run state.

---

## E2: RunConfig fields — what is constructed at startup vs per-call

- **Source:** `src/internal/workflow/run.go:179-207`
- **Finding:**
```go
type RunConfig struct {
	WorkflowDir     string
	Iterations      int
	Env             []string
	ContainerEnv    map[string]string
	InitializeSteps []steps.Step
	Steps           []steps.Step
	FinalizeSteps   []steps.Step
	LogWidth        int
	RunStamp        string
	Runner          StatusRunner
}
```
- **Relevance:** `RunStamp` is the per-run artifact directory identifier. It is passed in from `log.RunStamp()` (a timestamp generated at `NewLogger` construction). All other fields come from the parsed config. None are persisted across runs.

---

## E3: Per-iteration state that is reset each loop

- **Source:** `src/internal/workflow/run.go:473-501`
- **Finding:**
```go
for i := 1; cfg.Iterations == 0 || i <= cfg.Iterations; i++ {
    iterationsRun = i
    vt.ResetIteration()
    push(vars.Iteration)
    vt.SetIteration(i)
    ...
    captureStates := make(map[string]ui.StepState)
    var prevIterStats claudestream.StepStats
    var prevIterState ui.StepState
```
- **Relevance:** Three items are explicitly reset each iteration: the iteration VarTable scope (`ResetIteration`), `captureStates` (tracking which captures succeeded), and `prevIterStats`/`prevIterState` (resume-gate trackers). All are in-memory only. A cross-run resume would need to reconstruct `captureStates` and the persistent VarTable scope from prior records.

---

## E4: Persistent VarTable scope (initialize-phase captures) — the most important resume state

- **Source:** `src/internal/vars/vars.go:54-76`
- **Finding:**
```go
type VarTable struct {
	persistent map[string]string
	iteration  map[string]string
	finalize   map[string]string
	phase      Phase
}

func New(workflowDir, projectDir string, maxIter int) *VarTable {
	vt := &VarTable{
		persistent: make(map[string]string),
		...
	}
	vt.persistent["WORKFLOW_DIR"] = workflowDir
	vt.persistent["PROJECT_DIR"] = projectDir
	vt.persistent["MAX_ITER"] = strconv.Itoa(maxIter)
	return vt
}
```
- **Relevance:** The persistent table accumulates initialize-phase `captureAs` bindings (e.g., `GH_USER`, `COMMIT_SHA`) across all iterations. These are in-memory only. Any cross-run resume must somehow restore these. Iteration-phase bindings (e.g., `ISSUE_ID`) are cleared each iteration and do **not** need persistence.

---

## E5: `IterationRecord` structure — what is written per step

- **Source:** `src/internal/workflow/iterationlog.go:15-27`
- **Finding:**
```go
type IterationRecord struct {
	SchemaVersion int     `json:"schema_version"`
	IssueID       string  `json:"issue_id"`
	IterationNum  int     `json:"iteration_num"`
	StepName      string  `json:"step_name"`
	Model         string  `json:"model,omitempty"`
	Status        string  `json:"status"` // "done" | "skipped" | "failed" | "unknown"
	DurationS     float64 `json:"duration_s"`
	InputTokens   int     `json:"input_tokens,omitempty"`
	OutputTokens  int     `json:"output_tokens,omitempty"`
	SessionID     string  `json:"session_id,omitempty"`
	Notes         string  `json:"notes,omitempty"`
}
```
- **Relevance:** `IterationRecord` records step outcomes but does **not** record the captured variable values (`captureAs` binding values). It records `IssueID` and `IterationNum` — enough to know what was processed, but not enough to reconstruct the VarTable persistent scope without re-running the initialize steps or deriving values from the record itself.

---

## E6: `AppendIterationRecord` write semantics — flushed per-write, not buffered

- **Source:** `src/internal/workflow/iterationlog.go:47-67`
- **Finding:**
```go
func AppendIterationRecord(projectDir string, rec IterationRecord) (err error) {
	path := filepath.Join(projectDir, ".pr9k", "iteration.jsonl")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	...
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("workflow: iteration log: close %s: %w", path, cerr)
		}
	}()
	data, err := json.Marshal(rec)
	...
	data = append(data, '\n')
	if _, err = f.Write(data); err != nil {
```
- **Relevance:** The file is opened fresh, written, and closed for each record. There is no in-memory buffer that could be lost on SIGKILL. Each record is written atomically as a small POSIX O_APPEND write (well under PIPE_BUF). A SIGKILL mid-write could produce a partial line for the record being written at the exact moment of kill, but all prior records are durable. This means `iteration.jsonl` is a reliable audit trail.

---

## E7: `get_next_issue` script — returns same issue if still open

- **Source:** `workflow/scripts/get_next_issue:1-25`
- **Finding:**
```bash
issuenum=$(gh api graphql -f query='
  query($q: String!) {
    search(query: $q, type: ISSUE, first: 100) {
      nodes {
        ... on Issue {
          number
          ...
          blockedBy(first: 100) { nodes { state } }
        }
      }
    }
  }' -f q="repo:$repo is:issue is:open assignee:$username label:$ralphlabel" \
  | jq -r '[.data.search.nodes[] | select([.blockedBy.nodes[] | select(.state == "OPEN")] | length == 0) | .number] | sort | .[0] // empty'
)

echo $issuenum
```
- **Relevance:** The script queries `is:open` issues. If a run is killed mid-iteration while working on issue #42 — before `close_gh_issue` runs — that issue remains open and will be returned again on the next invocation. The script is stateless and idempotent: it always returns the lowest-numbered open issue with no open blockers. Cross-run resume gets the same issue "for free" without any state tracking.

---

## E8: Graceful shutdown path — q/y to process exit

- **Source:** `src/internal/ui/ui.go:144-157` and `src/cmd/pr9k/main.go:284-302`
- **Finding:**
```go
// ui.go:144
func (h *KeyHandler) ForceQuit() {
	h.mu.Lock()
	h.mode = ModeQuitting
	h.updateShortcutLineLocked()
	h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
	}
	select {
	case h.Actions <- ActionQuit:
	default:
	}
}

// main.go:296-302
go func() {
    defer close(workflowDone)
    _ = workflow.Run(runner, proxy, keyHandler, runCfg)
    _ = log.Close()
    close(lineCh)
    keyHandler.SetMode(ui.ModeDone)
}()
```
- **Relevance:** `ForceQuit` calls `h.cancel()` (which is `runner.Terminate`), sends `ActionQuit` to the Actions channel, and sets mode to `ModeQuitting`. The workflow goroutine's `Run` returns (via `ActionQuit` from `Orchestrate`), then calls `log.Close()` which flushes the buffered logger. No deferred state-write hook exists at this point — there is no mechanism to write a "checkpoint" file before exit.

---

## E9: SIGKILL hard-kill path — what survives

- **Source:** `src/cmd/pr9k/main.go:280-293`
- **Finding:**
```go
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
signaled := make(chan struct{})
go func() {
    <-sigChan
    close(signaled)
    keyHandler.ForceQuit()
    select {
    case <-workflowDone:
    case <-time.After(2 * time.Second):
    }
    program.Kill()
}()
```
- **Relevance:** SIGINT/SIGTERM give a 2-second grace window before `program.Kill()`. A hard `kill -9` (SIGKILL) bypasses this entirely. On SIGKILL: `iteration.jsonl` is safe (per-write flush, E6). The buffered logger (`bufio.Writer` wrapping a file) may lose the last unflushed lines. The VarTable persistent scope is lost entirely (in-memory). No crash-detection sentinel exists.

---

## E10: `resumePrevious` gates — G1 blocks all cross-run resume

- **Source:** `src/internal/workflow/run.go:269-287`
- **Finding:**
```go
func evaluateResumeGates(
	prevStats claudestream.StepStats,
	prevState ui.StepState,
	blacklisted func(string) bool,
) (sessionID string, gate string) {
	if prevStats.SessionID == "" {
		return "", "G1: previous step has no session ID"
	}
	if prevState != ui.StepDone {
		return "", "G2: previous step did not complete successfully"
	}
	...
}
```
- **Relevance:** G1 checks `prevStats.SessionID` which is a local stack variable (`prevIterStats`) reset at the start of each iteration (E3) and to zero at the start of each `Run`. A cross-run resume cannot pass G1 even if the `SessionID` were persisted, because `prevStats` is reconstructed from zero each run. Enabling cross-run Claude-session resume would require persisting `prevStats` (specifically `SessionID`, `InputTokens`, and `StepState`) and seeding the tracker from it. However, cross-run resume of the *iteration loop* (not the Claude session) does not require touching the resume gates at all — `get_next_issue` handles idempotent re-delivery of the same issue (E7).

---

## E11: Session blacklist — in-memory only, lost on restart

- **Source:** `src/internal/workflow/workflow.go:64-67`
- **Finding:**
```go
type Runner struct {
	...
	// sessionBlacklist records timed-out claude session IDs.
	sessionBlacklist map[string]bool
```
- **Source:** `src/internal/workflow/workflow.go:119-128`
- **Finding:**
```go
func NewRunner(log *logger.Logger, projectDir string) *Runner {
	return &Runner{
		log:              log,
		projectDir:       projectDir,
		sessionBlacklist: make(map[string]bool),
		...
	}
}
```
- **Relevance:** The session blacklist is initialized empty on every `NewRunner` call. It is never persisted to disk. On a restart after a timeout, a previously blacklisted session ID would no longer be blacklisted. Since G1 would also fail (E10) for cross-run resume anyway, this is a secondary concern. However, if cross-run Claude-session resume were ever enabled, blacklist persistence would become required to prevent resuming a timed-out session.

---

## E12: `RunStamp` construction — always fresh, never reused

- **Source:** `src/internal/logger/logger.go:27-48`
- **Finding:**
```go
func NewLoggerWithPrefix(projectDir, prefix string) (*Logger, error) {
	...
	now := time.Now()
	layout := prefix + "-2006-01-02-150405.000"
	filename := now.Format(layout + ".log")
	runStamp := now.Format(layout)
	...
	return &Logger{
		file:     f,
		writer:   bufio.NewWriter(f),
		runStamp: runStamp,
	}, nil
}
```
- **Source:** `src/cmd/pr9k/main.go:104` and `main.go:234`
- **Finding:**
```go
artifactDir := filepath.Join(projectDir, ".pr9k", "logs", log.RunStamp())
...
runCfg := buildRunConfig(cfg, stepFile, statusRunner, logWidth, log.RunStamp())
```
- **Relevance:** The stamp is generated once at `NewLogger` and passed as a plain string into `RunConfig`. It is used only for artifact directory naming. Reusing a prior stamp would require either parsing the stamp from the prior run's log filename or persisting the stamp in a state file. The stamp is observed only via `log.RunStamp()` (single call in `main.go`); changing to reuse a prior stamp is a localized change.

---

## E13: `atomicwrite.Write` signature — suitable for a state file

- **Source:** `src/internal/atomicwrite/write.go:34-37`
- **Finding:**
```go
func Write(path string, data []byte, mode os.FileMode) error {
	return write(osFS{}, path, data, mode)
}
```
- **Relevance:** `atomicwrite.Write` takes a path, byte slice, and file mode. It writes via temp-file + rename with fsync, making it suitable for writing a small JSON checkpoint/state file from `main.go`. No special adapter is required.

---

## E14: Startup sequence — where a "check for existing run" would fit

- **Source:** `src/cmd/pr9k/main.go:128-145`
- **Finding:**
```go
func main() {
	cfg, err := cli.Execute(newSandboxCmd(), newWorkflowCmd())
	...
	profileDir := preflight.ResolveProfileDir()
	svc, ok := startup(cfg, cfg.ProjectDir, profileDir, preflight.RealProber{}, os.Stderr)
	if !ok {
		os.Exit(1)
	}
	log := svc.log
	stepFile := svc.stepFile
	runner := svc.runner
```
- **Source:** `src/internal/preflight/run.go:31-52`
- **Finding:**
```go
func Run(projectDir, profileDir string, hasClaudeSteps bool, p Prober) Result {
	var result Result
	pr9kDir := filepath.Join(projectDir, ".pr9k")
	if err := os.MkdirAll(pr9kDir, 0o755); err != nil {
		result.Errors = append(result.Errors, ...)
	}
	...
```
- **Relevance:** The natural insertion point for "check for existing active run state" is between `cli.Execute` (line 129) and `startup` (line 141) in `main.go`, or inside `startup` itself, or as an additional step within `preflight.Run`. The `.pr9k/` directory is guaranteed to exist after `preflight.Run` completes. A state file check could live at `<projectDir>/.pr9k/run-state.json`.

---

## E15: `Run` exit conditions — what main.go observes

- **Source:** `src/internal/workflow/run.go:459-462`, `run.go:588-590`, `run.go:748-750`
- **Finding:**
```go
// ActionQuit from initialize:
if action == ui.ActionQuit {
    return RunResult{}
}

// ActionQuit from iteration:
if action == ui.ActionQuit {
    return RunResult{IterationsRun: iterationsRun}
}

// Normal completion:
return RunResult{IterationsRun: iterationsRun}
```
- **Source:** `src/cmd/pr9k/main.go:298`
- **Finding:**
```go
_ = workflow.Run(runner, proxy, keyHandler, runCfg)
```
- **Relevance:** `main.go` discards the `RunResult` entirely (underscore assignment). The distinction between user-quit, `breakLoopIfEmpty`, and normal completion is not observed at the call site. Adding a checkpoint write on quit-vs-normal-completion would require propagating the exit condition through `RunResult`.

---

## E16: Variable scopes that matter for cross-run resume

- **Source:** `src/internal/vars/vars.go:65-76`
- **Finding:**
```go
func New(workflowDir, projectDir string, maxIter int) *VarTable {
	vt := &VarTable{
		persistent: make(map[string]string),
		iteration:  make(map[string]string),
		finalize:   make(map[string]string),
		phase:      Initialize,
	}
	vt.persistent["WORKFLOW_DIR"] = workflowDir
	vt.persistent["PROJECT_DIR"] = projectDir
	vt.persistent["MAX_ITER"] = strconv.Itoa(maxIter)
	return vt
}
```
- **Relevance:** Three scopes exist:
  1. **Persistent** (initialize-phase captures + built-ins): survives the entire run, must be persisted for cross-run resume. Built-ins (`WORKFLOW_DIR`, `PROJECT_DIR`, `MAX_ITER`, `ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`) can be reconstructed from CLI flags on restart; user-defined initialize captures (e.g., `GH_USER`) cannot without re-running initialize steps.
  2. **Iteration** (cleared each iteration): does not need persistence — `get_next_issue` re-delivers the same issue, and iteration steps re-run from scratch for each issue anyway.
  3. **Finalize** (written once during finalize): finalize has not run if the loop was killed mid-iteration, so no finalize state needs persisting.

---

## E17: Logger uses a `bufio.Writer` — flush is NOT per-write

- **Source:** `src/internal/logger/logger.go:15-18`, `logger.go:76-93`, `logger.go:104-118`
- **Finding:**
```go
type Logger struct {
	mu        sync.Mutex
	file      *os.File
	writer    *bufio.Writer
	...
}

func (l *Logger) Log(stepName string, line string) error {
	...
	_, err := fmt.Fprintln(l.writer, prefix+line)
	return err
}

func (l *Logger) Close() error {
	...
	if err := l.writer.Flush(); err != nil {
		_ = l.file.Close()
		...
	}
```
- **Relevance:** The Logger uses a `bufio.Writer`, which buffers writes. `Close()` calls `Flush()` before closing. On SIGKILL, `Close()` is never called — the last buffer contents (potentially several KB of log lines) are lost. This does not affect `iteration.jsonl` (which is not buffered, E6), but means the `.log` file may be truncated on hard kill. The log file is for diagnostics only, not for state recovery.

---

## E18: Negative evidence — no existing checkpoint/state file mechanism

Searched the following and found no existing cross-run state persistence mechanism:

- `grep -rn "checkpoint\|run-state\|resume.*run\|cross.*run" src/` — no results
- `grep -rn "atomicwrite.Write" src/` — used only in `workflowio` package (workflow builder), not in the run loop
- `find src/ -name "*.go" | xargs grep -l "os.WriteFile\|ioutil.WriteFile"` — no results in run-path code
- `grep -rn "\.pr9k/" src/internal/workflow/` — only `iteration.jsonl` and `.pr9k/logs/` are written during a run

No state file, no checkpoint, no lock file, and no resume-detection code exists anywhere in the run path.

---

## Summary of Key Resume Constraints

| Constraint | Evidence |
|-----------|----------|
| VarTable is always constructed fresh | E1 |
| Initialize-phase captures are in-memory only | E4 |
| `iteration.jsonl` is durable (per-write flush, no buffer) | E6 |
| `get_next_issue` is idempotent — re-delivers same open issue | E7 |
| No checkpoint write on graceful or forced exit | E8, E9, E18 |
| `RunStamp` is always freshly generated | E12 |
| `atomicwrite.Write` is usable for a state file | E13 |
| `RunResult` is discarded in `main.go` | E15 |
| Only persistent-scope captures need cross-run persistence | E16 |
| Session blacklist is in-memory only | E11 |
| G1 blocks all cross-run Claude-session resume | E10 |
