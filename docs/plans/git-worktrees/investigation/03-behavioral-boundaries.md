# Behavioral Boundaries: Git Worktree Runtime Analysis

This document traces runtime behavior across nine boundary areas relevant to the proposed `useWorktrees: true` feature. Findings are numbered B1–B18 and follow the four analysis dimensions: Data Flow, Error Propagation, State Management, and Integration Boundaries.

---

## Findings

**B1: ProjectDir becomes the single structural seam at `NewRunner` construction**
- **Dimension:** State Management
- **File(s):** `src/cmd/pr9k/main.go`, `src/internal/workflow/workflow.go`
- **Finding:** `projectDir` enters the system as `cfg.ProjectDir` (set once in `cli.Execute`) and is frozen into the `Runner` struct at construction time:

```go
// main.go:141
svc, ok := startup(cfg, cfg.ProjectDir, profileDir, preflight.RealProber{}, os.Stderr)

// main.go:113
runner: workflow.NewRunner(log, projectDir),
```

```go
// workflow.go:119-128
func NewRunner(log *logger.Logger, projectDir string) *Runner {
    return &Runner{
        log:        log,
        projectDir: projectDir,
        ...
    }
}
```

Everything downstream reads it through `executor.ProjectDir()`. There is no re-resolution of projectDir during the run. **This is a single seam**: substituting the worktree path for `projectDir` before `NewRunner` is called propagates it everywhere automatically.
- **Impact:** The seam is clean. A worktree implementation can intercept at one point (between `cfg.ProjectDir` resolution and `NewRunner`) and affect the entire run uniformly.

---

**B2: `VarTable` bakes in `PROJECT_DIR` at construction, never updated mid-run**
- **Dimension:** State Management
- **File(s):** `src/internal/vars/vars.go`, `src/internal/workflow/run.go`
- **Finding:** `vars.New` is called once in `Run` with the frozen `executor.ProjectDir()`:

```go
// run.go:315
vt := vars.New(cfg.WorkflowDir, executor.ProjectDir(), cfg.Iterations)
```

```go
// vars.go:65-76
func New(workflowDir, projectDir string, maxIter int) *VarTable {
    ...
    vt.persistent["PROJECT_DIR"] = projectDir
    ...
}
```

The `PROJECT_DIR` value in the `VarTable` (and therefore in every `{{PROJECT_DIR}}` template substitution and every `statusline.State.ProjectDir` push) is fixed for the lifetime of the run. There is no re-reading of cwd or re-resolution during iteration.
- **Impact:** This is **not** a scattered concern — it inherits from the single seam at B1. A correct worktree path passed to `NewRunner` will appear correctly in all template substitutions and statusline payloads throughout the run.

---

**B3: Iteration loop has no cwd re-resolution; worktree can be created lazily before `Run` is called**
- **Dimension:** Data Flow
- **File(s):** `src/internal/workflow/run.go`
- **Finding:** The iteration loop body (`run.go:474–618`) performs no `os.Getwd()`, no re-read of config, and no re-resolution of any directory path. Variables are reset per-iteration (`vt.ResetIteration()`) but built-in paths are not touched. Config is loaded once in `startup` (`main.go:63`) before `Run` is ever called.

The `buildStep` function is called per step but only reads `executor.ProjectDir()` which is the frozen Runner field:

```go
// run.go:771
projectDir := executor.ProjectDir()
argv := sandbox.BuildRunArgs(projectDir, profileDir, uid, gid, cidfile, ...)
```

- **Impact:** A worktree created before `workflow.Run` starts will serve all iterations without any per-iteration re-creation. Lazy creation on iteration 1 is also safe because the first docker `BuildRunArgs` call happens inside the iteration loop.

---

**B4: Docker bind-mount uses `projectDir` directly — worktree path propagates automatically**
- **Dimension:** Integration Boundaries
- **File(s):** `src/internal/sandbox/command.go`, `src/internal/workflow/run.go`
- **Finding:** `BuildRunArgs` takes `projectDir` as a parameter and uses it literally in the `--mount` flag:

```go
// command.go:47-49
"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", projectDir, ContainerRepoPath),
...
"-w", ContainerRepoPath,
```

The container always sees the mounted directory as `/home/agent/workspace` regardless of whether the source is a worktree or the primary checkout. The container's `$PWD` is always `ContainerRepoPath = "/home/agent/workspace"`.

`profileDir` is resolved from `CLAUDE_CONFIG_DIR` / `$HOME/.claude` at every `buildStep` call — it does **not** come from the worktree path and is unaffected:

```go
// run.go:765-766
profileDir := preflight.ResolveProfileDir()
projectDir := executor.ProjectDir()
```

- **Impact:** This is a **clean pass-through seam**. If `projectDir` is the worktree path, the container will mount the worktree. The container sees exactly the worktree content. No scattered concern.

---

**B5: All non-claude `cmd.Dir` is set to `r.projectDir` — explicit, not inherited from parent process cwd**
- **Dimension:** Subprocess Inheritance
- **File(s):** `src/internal/workflow/workflow.go`
- **Finding:** Every subprocess, both `RunStep`/`RunStepFull` and `RunSandboxedStep`, passes through `runCommand`, which sets `cmd.Dir` explicitly:

```go
// workflow.go:432-433
cmd := exec.Command(command[0], command[1:]...)
cmd.Dir = r.projectDir
```

`CaptureOutput` also uses this pattern:

```go
// workflow.go:806-807
cmd := exec.Command(command[0], command[1:]...)
cmd.Dir = r.projectDir
```

The Go process's own working directory (`os.Getwd()`) is **never consulted** after startup. `os.Chdir` is never called anywhere in the codebase. There is no process-global cwd mutation.
- **Impact:** **Clean seam** — no process-level cwd change is needed. Redirecting `r.projectDir` to a worktree path is sufficient to redirect all non-claude subprocess executions.

---

**B6: `gh repo view` in `get_next_issue` discovers the repo from cwd's `.git` — worktree cwd must contain a valid git context**
- **Dimension:** Integration Boundaries
- **File(s):** `workflow/scripts/get_next_issue`
- **Finding:** The script calls `gh repo view` without `--repo`:

```bash
# get_next_issue:4
repo=$(gh repo view --json nameWithOwner -q ".nameWithOwner")
```

`gh` discovers the repository by walking up from the process cwd until it finds `.git`. When the subprocess runs with `cmd.Dir = worktreePath`, `gh` will walk up from the worktree directory. Git worktrees do contain a `.git` file (a gitfile pointing at the main `.git/worktrees/<name>` entry), so `gh` will discover the correct repository from the worktree cwd.

- **Impact:** This **will work correctly** with a worktree because worktrees have a `.git` gitfile that `gh` and `git` both honor. No behavioral risk here, but only if the worktree is created with `git worktree add` (which always creates the gitfile). A bare directory copy would break this.

---

**B7: `get_commit_sha` uses `git rev-parse HEAD` — will reflect the worktree's HEAD, which may differ from primary**
- **Dimension:** Data Flow
- **File(s):** `workflow/scripts/get_commit_sha`, `workflow/config.json`
- **Finding:** The script calls `git rev-parse HEAD` with no arguments:

```bash
# get_commit_sha:4
sha=$(git rev-parse HEAD)
```

When run with `cmd.Dir` pointing at a worktree, this returns the HEAD of the **worktree's branch**, not the primary checkout's HEAD. In the default workflow, this is captured as `STARTING_SHA`:

```json
{ "name": "Get starting SHA", "isClaude": false, "command": ["git", "rev-parse", "HEAD"], "captureAs": "STARTING_SHA" }
```

And then used in a diff: `"git", "diff", "{{STARTING_SHA}}..HEAD"`. This diff happens in the same worktree cwd, so it is self-consistent. The behavior is **correct** for the worktree scenario.
- **Impact:** No behavioral risk. The SHA and diff both come from and operate in the same worktree cwd consistently.

---

**B8: `git push` in `git_push` script runs without `--set-upstream` or branch specification — worktree may lack a tracking branch**
- **Dimension:** Integration Boundaries
- **File(s):** `workflow/scripts/git_push`
- **Finding:** The script calls `git push` with no arguments:

```bash
# git_push:4
result=$(git push)
```

When a new worktree is created from a new branch (the expected worktree use case), that branch has no upstream tracking ref set. `git push` without arguments will fail with "The current branch X has no upstream branch." This is not currently a problem for the primary checkout if it already has an upstream, but **a freshly created worktree branch will not have one**.

The script captures this in a variable but discards errors with `trap 'exit 0' EXIT` — the push failure becomes a **silent failure**:

```bash
# git_push:2-5
trap 'exit 0' EXIT

result=$(git push)

echo $result
```

- **Impact:** If the worktree runs on a new branch without an upstream, the git push silently exits 0, writing nothing to stdout. The run completes "successfully" with no push actually occurring. The `STARTING_SHA` flow and issue closure will have already run, leaving closed issues and no pushed code.

---

**B9: `post_issue_summary` reads `.pr9k/iteration.jsonl` with a relative path — resolves against subprocess cwd**
- **Dimension:** Data Flow
- **File(s):** `workflow/scripts/post_issue_summary`
- **Finding:** The script references both files with relative paths:

```bash
# post_issue_summary:10-11
JSONL_FILE=".pr9k/iteration.jsonl"
PROGRESS_FILE="progress.txt"
```

When `cmd.Dir = worktreePath`, these paths resolve to `<worktreePath>/.pr9k/iteration.jsonl` and `<worktreePath>/progress.txt`. Since preflight creates `.pr9k/` in `projectDir` (the worktree path in this scenario), this **will work correctly** — the iteration log is written there and read from there.
- **Impact:** No behavioral risk when logs stay in the worktree. However, see B12 for the scenario where logs are intentionally redirected to the primary repo.

---

**B10: `review_verdict` reads `code-review.md` with a relative path — resolves against subprocess cwd**
- **Dimension:** Data Flow
- **File(s):** `workflow/scripts/review_verdict`
- **Finding:** The file is opened with a relative path:

```bash
# review_verdict:19
REVIEW_FILE="code-review.md"
...
if [[ ! -s "$REVIEW_FILE" ]]; then
```

Claude writes `code-review.md` to `/home/agent/workspace/` inside the container, which is `<worktreePath>/code-review.md` on the host. `review_verdict` runs with `cmd.Dir = worktreePath`, so it reads from the correct location.
- **Impact:** Clean. All intermediate files (`progress.txt`, `deferred.txt`, `test-plan.md`, `code-review.md`) resolve from the subprocess cwd and will land in the worktree.

---

**B11: `project_card` reads `go.mod`, `Makefile`, etc. relative to cwd — worktree cwd contains real source**
- **Dimension:** Data Flow
- **File(s):** `workflow/scripts/project_card`
- **Finding:** The script probes multiple files with bare names:

```bash
# project_card:11,19,26,38
if [ -f go.mod ]; then
if [ -f Makefile ]; then
if [ -f package.json ]; then
if [ -f pyproject.toml ]; then
```

All relative to the subprocess cwd. When `cmd.Dir` is the worktree path, these detect the worktree's files. Git worktrees share the actual source files, so `go.mod` et al. will be present and correct.
- **Impact:** No behavioral risk. Worktrees are full-content directories.

---

**B12: Logger writes to `<projectDir>/.pr9k/logs/` — if ProjectDir is the worktree, logs vanish on cleanup**
- **Dimension:** State Management
- **File(s):** `src/internal/logger/logger.go`, `src/cmd/pr9k/main.go`
- **Finding:** The logger creates its file under `projectDir`:

```go
// logger.go:28-30
func NewLoggerWithPrefix(projectDir, prefix string) (*Logger, error) {
    logsDir := filepath.Join(projectDir, ".pr9k", "logs")
    ...
```

The artifact directory is also placed there:

```go
// main.go:104
artifactDir := filepath.Join(projectDir, ".pr9k", "logs", log.RunStamp())
```

If `projectDir` is the worktree path, the `.pr9k/logs/` directory, all `.jsonl` artifacts, and the run log exist inside the worktree. When the worktree is deleted on completion, these are all removed.

The `iteration.jsonl` file is similarly written to the worktree:

```go
// iterationlog.go:48
path := filepath.Join(projectDir, ".pr9k", "iteration.jsonl")
```

- **Impact:** **Significant behavioral concern**. Logs and artifacts are silently destroyed when the worktree is cleaned up. A debugging session after a completed run would find no `.pr9k/logs/` entries. If logs should persist to the primary repo's `.pr9k/logs/`, then `logger.NewLogger` and `AppendIterationRecord` need to receive a separate `logDir` parameter distinct from `projectDir`. This is a **scattered concern** — the log path is constructed from `projectDir` in `main.go`, `logger.go`, `run.go` (via `artifactPath`), and `iterationlog.go`.

---

**B13: `preflight.Run` creates `.pr9k/` in projectDir — must run against the worktree path, not primary**
- **Dimension:** State Management
- **File(s):** `src/internal/preflight/run.go`
- **Finding:** Preflight creates the `.pr9k` umbrella directory using the `projectDir` it receives:

```go
// preflight/run.go:36-38
pr9kDir := filepath.Join(projectDir, ".pr9k")
if err := os.MkdirAll(pr9kDir, 0o755); err != nil {
    result.Errors = append(result.Errors, ...)
```

If `projectDir` is changed to the worktree path after `startup` completes, the `.pr9k/` directory exists in the primary repo, not the worktree. Since `AppendIterationRecord` writes to `<projectDir>/.pr9k/iteration.jsonl`, and the runner's `projectDir` is the worktree path, the write would fail because `.pr9k/` doesn't exist there.

If preflight runs with the worktree path (i.e., worktree creation happens before `startup`), this resolves — but see B16 for the chicken-and-egg problem.
- **Impact:** Directory creation and the write target must use the same path. If the worktree is created before `startup`, preflight correctly creates `.pr9k/` inside the worktree and everything is consistent.

---

**B14: Statusline script runs with `cmd.Dir = r.projectDir` — git commands in the statusline script see the worktree**
- **Dimension:** Integration Boundaries
- **File(s):** `src/internal/statusline/statusline.go`, `workflow/scripts/statusline`
- **Finding:** The statusline runner executes the script with an explicit working directory:

```go
// statusline.go:289
cmd.Dir = r.projectDir
```

The default `statusline` script calls `git`:

```bash
# workflow/scripts/statusline:19
git rev-parse --git-dir > /dev/null 2>&1 && BRANCH=" | 🌿 $(git branch --show-current 2>/dev/null)"
```

If `projectDir` is the worktree path, `git branch --show-current` returns the **worktree's branch name**, which is exactly what the operator would want to see (confirming which branch the work is happening on). The statusline `projectDir` is set once at `statusline.New` construction in `main.go`:

```go
// main.go:155
statusRunner := statusline.New(buildStatusLineConfig(stepFile.StatusLine), cfg.WorkflowDir, cfg.ProjectDir, log)
```

- **Impact:** The statusline correctly follows `ProjectDir`. **Clean seam** — no scattered concern.

---

**B15: Config is loaded from WorkflowDir, not ProjectDir — no chicken-and-egg on `useWorktrees` flag**
- **Dimension:** Data Flow
- **File(s):** `src/internal/steps/steps.go`, `src/cmd/pr9k/main.go`
- **Finding:** `steps.LoadSteps` reads from `workflowDir`, not `projectDir`:

```go
// steps.go:144-146
func LoadSteps(workflowDir string) (StepFile, error) {
    path := filepath.Join(workflowDir, "config.json")
    data, err := os.ReadFile(path)
```

`workflowDir` is resolved in `cli.Execute` against the **primary repo's `<projectDir>/.pr9k/workflow/`** first, then the executable dir. This resolution happens at startup using the **shell's cwd** (`os.Getwd()`), before any worktree exists.

If `useWorktrees: true` is in the config, the config must be read first to know to create a worktree. The worktree doesn't exist yet when the config is read. This is structurally fine as long as the worktree creation happens between config-load and `NewRunner` construction. The workflow bundle (prompts, scripts) lives in `workflowDir` which never changes — the worktree is only for `projectDir`.
- **Impact:** **Not a chicken-and-egg problem** for the config-loading step specifically. WorkflowDir is independent of ProjectDir. However, the in-repo workflow override search (`<projectDir>/.pr9k/workflow/`) uses the **primary repo's** projectDir (the shell cwd at invocation), not the worktree — which means a workflow override stored in a worktree branch (as opposed to the main branch) would not be found. For the typical case where the workflow bundle is in the primary checkout, this is fine.

---

**B16: Worktree creation must happen after WorkflowDir resolution but before `startup` to avoid `.pr9k/` placement mismatch**
- **Dimension:** State Management / Data Flow
- **File(s):** `src/cmd/pr9k/main.go`
- **Finding:** The current startup sequence is:

```go
// main.go:129-143
cfg, err := cli.Execute(...)  // resolves WorkflowDir and ProjectDir from shell cwd

profileDir := preflight.ResolveProfileDir()
svc, ok := startup(cfg, cfg.ProjectDir, ...)  // creates .pr9k/, NewLogger(projectDir), NewRunner(projectDir)
```

For worktree support, the worktree would need to be created after `cli.Execute` (so `cfg.WorkflowDir` is resolved using the primary repo's `.pr9k/workflow/`) but before `startup` (so `.pr9k/` is created inside the worktree, the logger writes inside the worktree, and `NewRunner` is built with the worktree path).

If worktree creation happens **inside** `startup` (after `.pr9k/` is already created in the primary repo), there would be a split: preflight's `.pr9k/` directory in the primary, but the logger and runner pointing at the worktree path.
- **Impact:** The insertion point for worktree creation is **precisely defined**: after `cli.Execute`, before `startup`. This is a single-point seam in `main.go`.

---

**B17: Crash or SIGKILL while worktree exists leaves `git worktree list` entry permanently — no cleanup on next run**
- **Dimension:** Error Propagation
- **File(s):** `src/cmd/pr9k/main.go`
- **Finding:** The current signal-handling path:

```go
// main.go:283-293
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

A SIGKILL (`kill -9`), power loss, or `program.Kill()` from the SIGINT handler can terminate the process without running any deferred cleanup. Nothing currently owns the worktree lifecycle (no deferred `git worktree remove` or `git worktree prune`). The worktree directory and its `git worktree list` entry persist on disk.

On the next run, if `useWorktrees: true` creates a new uniquely-named worktree (e.g. using a timestamp), stale entries accumulate. If it reuses a deterministic path (e.g. `<primaryDir>-worktree`), the create would fail because the directory already exists.
- **Impact:** **Behavioral risk for crash/interrupt recovery.** The system has no mechanism to detect or prune stale worktrees at startup. A subsequent run must either check for and remove the stale worktree before creating a new one (probing `git worktree list`), or use `git worktree add --force` with a fresh unique path. Neither is currently implemented.

---

**B18: `close_gh_issue` calls `gh issue close` — uses cwd for repo discovery; worktree cwd provides correct `.git` gitfile**
- **Dimension:** Integration Boundaries
- **File(s):** `workflow/scripts/close_gh_issue`
- **Finding:** `gh issue close` with a bare issue number uses cwd to discover the repository, same as `gh repo view`:

```bash
# close_gh_issue:8
gh issue close "$ISSUE_NUM" --comment "$COMMENT_MSG" --reason "$CLOSE_REASON"
```

A worktree's `.git` is a gitfile (text file) pointing to `<primaryRepo>/.git/worktrees/<name>`, which `gh` follows correctly. Same behavior for `gh issue comment` in `post_issue_summary` and `gh issue view` in the config.json iteration step.
- **Impact:** All `gh` calls that rely on cwd-based repo discovery **work correctly in a worktree** because `gh` uses `git rev-parse --show-toplevel` which honors the gitfile. No behavioral risk.

---

## Behavioral Summary

**Focus area analyzed:** All code paths from startup through the iteration loop and finalization that touch directory resolution, subprocess execution, Docker sandbox construction, script invocation, logging, and crash recovery. Runtime traces extended one layer outward into: the Go `exec.Cmd` contract (`cmd.Dir`), the Docker bind-mount argv, all workflow scripts in `workflow/scripts/`, the `gh` CLI's cwd-based repo discovery mechanism, and the git worktree gitfile contract.

**Key concerns:**

1. **Logs and artifacts vanish with the worktree (B12)** — this is a scattered concern. The log path is constructed from `projectDir` in four places (`main.go`, `logger.go`, `run.go`'s `artifactPath` closure, `iterationlog.go`). If `projectDir` is the worktree and the worktree is deleted, all observability artifacts are gone. A separate `logDir` concept is needed if logs should survive cleanup.

2. **Silent git push failure on new branches (B8)** — the `git_push` script traps all exits to 0. A newly created worktree branch with no upstream tracking ref will fail silently, leaving closed issues but no pushed code. This is a pre-existing script behavior that becomes a **new failure mode** specifically in the worktree case (primary checkouts presumably already have an upstream).

3. **Stale worktree accumulation on crash/SIGKILL (B17)** — there is no deferred cleanup for the worktree and no startup probe that would detect or remove a stale one. A deterministic worktree path needs a "remove if exists" guard on create; a timestamp-named path needs a prune step on startup.

**Well-handled areas:**

- **The ProjectDir seam is a single point (B1, B5)**: `r.projectDir` is frozen at construction and read uniformly through `executor.ProjectDir()`. All subprocess `cmd.Dir` settings and Docker bind-mount paths flow from this one value. Changing the value at one point before `NewRunner` propagates everywhere.
- **Docker bind-mounts are clean (B4)**: `BuildRunArgs` takes `projectDir` as a parameter. A worktree path passed in produces a correct `--mount source=<worktree>,...` with no side effects.
- **`gh` CLI works in worktrees (B6, B18)**: `gh` discovers the repository by walking up from cwd and honors the gitfile that git worktrees create. No workaround needed.
- **Config loading is not chicken-and-egg (B15)**: `LoadSteps` uses `workflowDir` (resolved from the primary repo at shell cwd), not `projectDir`. `useWorktrees: true` can be read before the worktree exists.
- **Statusline follows projectDir automatically (B14)**: `cmd.Dir = r.projectDir` in the statusline runner means git commands inside the statusline script will see the worktree branch.

**Seams vs scattered concerns:**

| Boundary | Type | Notes |
|---|---|---|
| Startup → workflow loop (ProjectDir baked in) | **Single seam** (B1) | One insertion point in main.go before startup |
| Per-iteration re-resolution | **Non-issue** (B3) | No re-resolution happens; worktree is safe through all iterations |
| Docker bind-mount | **Single seam** (B4) | Passes projectDir parameter through |
| Non-claude subprocess `cmd.Dir` | **Single seam** (B5) | All through `runCommand`; no process cwd mutation |
| `gh` CLI repo discovery | **Clean pass-through** (B6, B18) | Worktree gitfile satisfies `gh` |
| `git push` silent failure | **Scattered risk** (B8) | Script behavior, not Go code; needs `--set-upstream` or pre-check |
| Log/artifact paths | **Scattered concern** (B12) | Four construction sites using projectDir; need logDir split |
| `.pr9k/` creation and insertion order | **Single seam** (B13, B16) | Worktree must be created before `startup` |
| Crash cleanup | **Missing capability** (B17) | No current deferred cleanup; startup probe not present |
| Config loading (`useWorktrees` read) | **Non-issue** (B15) | WorkflowDir independent of worktree |
