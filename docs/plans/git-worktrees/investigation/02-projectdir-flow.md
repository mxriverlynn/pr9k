# ProjectDir Flow Investigation

## Scope

Every place in the codebase that touches, stores, or derives behavior from
the "target repo where the workflow runs" path — the value that would need
to change or be derived from a git worktree for worktree support.

---

## E1: ProjectDir resolution at startup — `os.Getwd` + symlink eval

**Source:** `src/internal/cli/args.go:59–76`

```go
func resolveProjectDir() (string, error) {
    cwd, err := os.Getwd()
    if err != nil {
        return "", err
    }
    resolved, err := filepath.EvalSymlinks(cwd)
    if err != nil {
        return "", err
    }
    info, err := os.Stat(resolved)
    if err != nil {
        return "", fmt.Errorf("project dir %q: %w", resolved, err)
    }
    if !info.IsDir() {
        return "", fmt.Errorf("project dir %q is not a directory", resolved)
    }
    return resolved, nil
}
```

**Relevance:** This is the zero-argument default. `os.Getwd()` captures the
user's shell CWD at startup. Symlinks are resolved via `filepath.EvalSymlinks`.
No git-awareness: the function does not consult `git rev-parse --show-toplevel`
or detect worktrees.

---

## E2: `--project-dir` flag handling (explicit override path)

**Source:** `src/internal/cli/args.go:93–109`

```go
if cfg.ProjectDir == "" {
    dir, err := resolveProjectDir()
    ...
    cfg.ProjectDir = dir
} else {
    resolved, err := filepath.EvalSymlinks(cfg.ProjectDir)
    ...
    info, err := os.Stat(resolved)
    if err != nil || !info.IsDir() {
        return fmt.Errorf("cli: --project-dir %q is not a directory", cfg.ProjectDir)
    }
    cfg.ProjectDir = resolved
}
```

**Relevance:** When `--project-dir` is given, symlinks are resolved but again
no git-awareness is applied. The resolved path is stored in `cfg.ProjectDir`
and flows downstream as the canonical project directory.

---

## E3: Config struct — single field carries the resolved path forward

**Source:** `src/internal/cli/args.go:17–21`

```go
type Config struct {
    Iterations  int
    WorkflowDir string
    ProjectDir  string
}
```

**Relevance:** `ProjectDir` is the single string that flows from CLI parsing
into every downstream consumer. All downstream usages listed below receive
this value.

---

## E4: `startup()` fans ProjectDir into four downstream sites

**Source:** `src/cmd/pr9k/main.go:62–114`

```go
func startup(cfg *cli.Config, projectDir, profileDir string, ...) (*services, bool) {
    ...
    preflightResult := preflight.Run(projectDir, profileDir, ...)     // line 70
    ...
    log, err := logger.NewLogger(projectDir)                          // line 96
    ...
    artifactDir := filepath.Join(projectDir, ".pr9k", "logs", log.RunStamp())  // line 104
    ...
    runner: workflow.NewRunner(log, projectDir),                       // line 112
```

**Relevance:** `startup` is called with `cfg.ProjectDir` at `main.go:141`.
All four usages (preflight, logger, artifact dir, runner) receive the same
`projectDir` string. This is the root fan-out.

---

## E5: StatusLine runner also receives ProjectDir

**Source:** `src/cmd/pr9k/main.go:155`

```go
statusRunner := statusline.New(buildStatusLineConfig(stepFile.StatusLine), cfg.WorkflowDir, cfg.ProjectDir, log)
```

**Relevance:** A fifth downstream consumer of `cfg.ProjectDir`. The statusline
runner uses it as the script's working directory (see E14).

---

## E6: Preflight creates `.pr9k/` under ProjectDir

**Source:** `src/internal/preflight/run.go:31–38`

```go
func Run(projectDir, profileDir string, hasClaudeSteps bool, p Prober) Result {
    ...
    pr9kDir := filepath.Join(projectDir, ".pr9k")
    if err := os.MkdirAll(pr9kDir, 0o755); err != nil {
        result.Errors = append(result.Errors, fmt.Errorf("preflight: could not create .pr9k in %s: %w", projectDir, err))
    }
```

**Relevance:** `.pr9k/` umbrella directory is created inside the project dir
at startup. This is a structural write under ProjectDir that must exist before
logging and iteration records can be written.

---

## E7: Logger creates `.pr9k/logs/` under ProjectDir

**Source:** `src/internal/logger/logger.go:27–49`

```go
func NewLoggerWithPrefix(projectDir, prefix string) (*Logger, error) {
    logsDir := filepath.Join(projectDir, ".pr9k", "logs")
    if err := os.MkdirAll(logsDir, 0o700); err != nil {
        return nil, fmt.Errorf("logger: could not create logs directory: %w", err)
    }
    ...
    logPath := filepath.Join(logsDir, filename)
    f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
```

**Relevance:** Log file lives at `<projectDir>/.pr9k/logs/ralph-YYYY-MM-DD-HHMMSS.mmm.log`.
`NewLogger` (used by the main workflow) delegates to `NewLoggerWithPrefix` with `"ralph"`.

---

## E8: Per-run artifact directory created eagerly under ProjectDir

**Source:** `src/cmd/pr9k/main.go:104–108`

```go
artifactDir := filepath.Join(projectDir, ".pr9k", "logs", log.RunStamp())
if err := os.MkdirAll(artifactDir, 0o700); err != nil {
    _, _ = fmt.Fprintf(stderr, "error: %v\n", err)
    return nil, false
}
```

**Relevance:** `.pr9k/logs/<run-stamp>/` is created at startup (not deferred
to step execution) to avoid races. Path is entirely under `projectDir`.

---

## E9: Runner stores ProjectDir; `cmd.Dir` set to it on every subprocess

**Source:** `src/internal/workflow/workflow.go:119–128` (construction) and
`src/internal/workflow/workflow.go:433` and `workflow.go:807` (execution)

```go
func NewRunner(log *logger.Logger, projectDir string) *Runner {
    return &Runner{
        log:        log,
        projectDir: projectDir,
        ...
    }
}
```

```go
// runCommand (line 432-433):
cmd := exec.Command(command[0], command[1:]...)
cmd.Dir = r.projectDir
```

```go
// CaptureOutput (line 806-807):
cmd := exec.Command(command[0], command[1:]...)
cmd.Dir = r.projectDir
```

**Relevance:** Every subprocess launched by the runner (all non-claude steps,
all calls to `CaptureOutput`) runs with `cmd.Dir = projectDir`. This is the
mechanism that gives scripts their working directory.

---

## E10: Docker bind-mount: ProjectDir → `/home/agent/workspace`

**Source:** `src/internal/sandbox/command.go:36–51`

```go
func BuildRunArgs(
    projectDir, profileDir string,
    ...
) []string {
    args := []string{
        "docker", "run",
        ...
        "--mount", fmt.Sprintf("type=bind,source=%s,target=%s", projectDir, ContainerRepoPath),
        "--mount", fmt.Sprintf("type=bind,source=%s,target=%s", profileDir, ContainerProfilePath),
        "-w", ContainerRepoPath,
        "-e", "CLAUDE_CONFIG_DIR=" + ContainerProfilePath,
    }
```

**Source:** `src/internal/sandbox/image.go:4–6`

```go
const (
    ImageTag             = "docker/sandbox-templates:claude-code"
    ContainerRepoPath    = "/home/agent/workspace"
    ContainerProfilePath = "/home/agent/.claude"
)
```

**Relevance:** `projectDir` is the host-side bind-mount source. The container
always sees the workspace at `/home/agent/workspace` regardless of the host
path. Any git metadata (`.git` or `.git` file for worktrees) present in
`projectDir` is what gets mounted.

---

## E11: `BuildRunArgs` called from `buildStep` with `executor.ProjectDir()`

**Source:** `src/internal/workflow/run.go:759–774`

```go
func buildStep(workflowDir string, s steps.Step, vt *vars.VarTable, ..., executor StepExecutor, ...) (ui.ResolvedStep, error) {
    if s.IsClaude {
        ...
        projectDir := executor.ProjectDir()
        ...
        argv := sandbox.BuildRunArgs(projectDir, profileDir, uid, gid, cidfile, ...)
```

**Relevance:** `executor.ProjectDir()` retrieves the `projectDir` stored in
the Runner (E9), passing it to `BuildRunArgs` for the bind-mount. This is the
call chain: `Run` → `buildStep` → `sandbox.BuildRunArgs`.

---

## E12: Iteration log path constructed from ProjectDir

**Source:** `src/internal/workflow/iterationlog.go:47–48`

```go
func AppendIterationRecord(projectDir string, rec IterationRecord) (err error) {
    path := filepath.Join(projectDir, ".pr9k", "iteration.jsonl")
    f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
```

**Relevance:** `<projectDir>/.pr9k/iteration.jsonl` is the JSONL audit log.
`AppendIterationRecord` is called from `run.go` with `executor.ProjectDir()`
at every step in every phase (lines 418, 439, 452, 522, 543, 567, 581, 657,
678, 700, 714 of `run.go`).

---

## E13: Per-step JSONL artifact path constructed from ProjectDir

**Source:** `src/internal/workflow/run.go:382–388`

```go
artifactPath := func(resolved *ui.ResolvedStep, phasePrefix string, stepIdx int) string {
    if !resolved.IsClaude || cfg.RunStamp == "" {
        return ""
    }
    filename := fmt.Sprintf("%s%02d-%s.jsonl", phasePrefix, stepIdx, claudestream.Slug(resolved.Name))
    return filepath.Join(executor.ProjectDir(), ".pr9k", "logs", cfg.RunStamp, filename)
}
```

**Relevance:** Per-step NDJSON artifacts go to
`<projectDir>/.pr9k/logs/<run-stamp>/<phase><nn>-<slug>.jsonl`. Calls
`executor.ProjectDir()` at closure definition time in `Run`.

---

## E14: Statusline script: `cmd.Dir = r.projectDir`

**Source:** `src/internal/statusline/statusline.go:284–289`

```go
cmd := exec.CommandContext(cmdCtx, r.path)
cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
cmd.WaitDelay = 1 * time.Second
cmd.Stdin = bytes.NewReader(payload)
cmd.Dir = r.projectDir
cmd.Env = os.Environ()
```

**Relevance:** The statusline script's working directory is `projectDir`.
Scripts that inspect git state (`git branch --show-current`, `git rev-parse
--git-dir`) execute from `projectDir`. The full host environment is passed
through (`os.Environ()`).

---

## E15: `{{PROJECT_DIR}}` seeded in VarTable from ProjectDir

**Source:** `src/internal/vars/vars.go:65–75`

```go
func New(workflowDir, projectDir string, maxIter int) *VarTable {
    vt := &VarTable{...}
    vt.persistent["WORKFLOW_DIR"] = workflowDir
    vt.persistent["PROJECT_DIR"] = projectDir
    vt.persistent["MAX_ITER"] = strconv.Itoa(maxIter)
    return vt
}
```

**Relevance:** `{{PROJECT_DIR}}` is available in every phase as a persistent
variable. `vt` is constructed at `run.go:315` with
`vars.New(cfg.WorkflowDir, executor.ProjectDir(), cfg.Iterations)`.

---

## E16: `{{WORKFLOW_DIR}}` also seeded — separate path, not ProjectDir

**Source:** `src/internal/vars/vars.go:72`

```go
vt.persistent["WORKFLOW_DIR"] = workflowDir
```

**Relevance:** `WORKFLOW_DIR` resolves to the workflow bundle directory (where
`config.json`, `prompts/`, `scripts/` live). This is intentionally separate
from `PROJECT_DIR`. Not a ProjectDir integration point, but noted to confirm
the two-directory split is correctly maintained in the var table.

---

## E17: Validator knows `{{PROJECT_DIR}}` as a reserved name (sandbox isolation rule)

**Source:** `src/internal/validator/validator.go:166`, `422`, `727–752`

```go
"PROJECT_DIR": true,
```

```go
// line 737-752 (sandbox isolation rule B):
if ref == "PROJECT_DIR" {
    hasProjectDir = true
}
...
if hasProjectDir {
    banned = append(banned, "{{PROJECT_DIR}}")
}
```

**Relevance:** The validator actively prevents `{{PROJECT_DIR}}` from being
embedded in claude-step command arguments (it would be a host path, invisible
inside the container). No validator change needed for worktrees since the
sandbox-isolation rule is path-agnostic.

---

## E18: StatusLine payload includes `projectDir` field

**Source:** `src/internal/statusline/state.go:18` and `src/internal/statusline/payload.go:20,46`

```go
// state.go
ProjectDir string

// payload.go
ProjectDir string `json:"projectDir"`
...
ProjectDir: s.ProjectDir,
```

**Relevance:** The statusline stdin JSON payload exposes `projectDir` to
user-authored scripts. Scripts that branch on this value would receive the
worktree path, not the main checkout path.

---

## E19: `buildState` reads `PROJECT_DIR` from VarTable for statusline state

**Source:** `src/internal/workflow/run.go:57–59`

```go
return statusline.State{
    ...
    ProjectDir: getString("PROJECT_DIR"),
    ...
}
```

**Relevance:** The VarTable's `PROJECT_DIR` (which equals `executor.ProjectDir()`)
is what gets put into the statusline payload. This is consistent with the
Runner's `projectDir` field.

---

## E20: Script CWDs — all scripts run with `cmd.Dir = projectDir`

The runner's `runCommand` sets `cmd.Dir = r.projectDir` for all non-claude
steps (E9). Scripts are resolved via `ResolveCommand` (relative paths joined
to `workflowDir`, not `projectDir`), but they *run* with `projectDir` as
their working directory.

Specific scripts and what they assume about CWD:

**`get_next_issue`** (`workflow/scripts/get_next_issue`): Calls `gh repo view
--json nameWithOwner`. `gh` infers the repo from git remote, which requires
CWD to be inside a git checkout. In a worktree, CWD is inside a valid git
checkout so `gh repo view` will still resolve the correct repo.

**`get_commit_sha`** (`workflow/scripts/get_commit_sha:4`):
```bash
sha=$(git rev-parse HEAD)
```
Runs with CWD = projectDir. In a worktree, `git rev-parse HEAD` resolves to
the worktree's current HEAD — which is correct for the worktree's branch.

**`git_push`** (`workflow/scripts/git_push`):
```bash
result=$(git push)
```
Runs with CWD = projectDir. In a worktree, `git push` pushes the worktree's
branch — correct behavior.

**`post_issue_summary`** (`workflow/scripts/post_issue_summary:10–11`):
```bash
JSONL_FILE=".pr9k/iteration.jsonl"
PROGRESS_FILE="progress.txt"
```
Uses relative paths. CWD = projectDir, so `.pr9k/iteration.jsonl` resolves
to `<projectDir>/.pr9k/iteration.jsonl` — the same path `AppendIterationRecord`
writes to. Consistent.

**`review_verdict`** (`workflow/scripts/review_verdict:19`):
```bash
REVIEW_FILE="code-review.md"
```
Uses a relative path. CWD = projectDir. The prompt writes to
`.pr9k/artifacts/code-review.md` (inside the container at
`/home/agent/workspace/.pr9k/artifacts/code-review.md`), but the script
reads from `code-review.md` directly (no `.pr9k/artifacts/` prefix).
**This is an inconsistency that already exists independent of worktrees.**

**`statusline`** (`workflow/scripts/statusline:19`):
```bash
git rev-parse --git-dir > /dev/null 2>&1 && BRANCH=" | 🌿 $(git branch --show-current 2>/dev/null)"
```
Runs from `projectDir`. In a worktree, `git rev-parse --git-dir` succeeds
(worktrees have a `.git` file pointing back to the main `.git`), and `git
branch --show-current` returns the worktree's branch — correct.

---

## E21: `sandbox shell` subcommand resolves ProjectDir independently

**Source:** `src/cmd/pr9k/sandbox_shell.go:42–53`

```go
RunE: func(cmd *cobra.Command, args []string) error {
    projectDir, err := os.Getwd()
    if err != nil {
        return fmt.Errorf("sandbox shell: resolve current directory: %w", err)
    }
    ...
    return runSandboxShell(&sandboxShellDeps{
        ...
        projectDir: projectDir,
        ...
    })
},
```

**Relevance:** `pr9k sandbox shell` resolves its `projectDir` independently
via `os.Getwd()` (no `--project-dir` flag). This is a separate resolution
path from the main workflow. The value is passed to `sandbox.BuildShellArgs`
for the bind-mount (same constant path).

---

## E22: Prompt files reference `.pr9k/artifacts/` — resolved inside container

**Source:** `workflow/prompts/feature-work.md:1`, `workflow/prompts/test-planning.md:1`, etc.

```
@.pr9k/artifacts/progress.txt
```

**Relevance:** Prompts use `@`-file references to `.pr9k/artifacts/` paths.
These are relative to the container's working directory
(`/home/agent/workspace`), which is the bind-mount of `projectDir`. As long
as `projectDir` is correctly mounted, prompt-internal artifact paths resolve
correctly regardless of whether it is a worktree.

---

## E23: `workflow builder` subcommand uses its own ProjectDir resolution

**Source:** `src/cmd/pr9k/workflow.go:83–135`

```go
func runWorkflowBuilder(cmd *cobra.Command, projectDirFlag, workflowDirFlag string) error {
    ...
    logBaseDir := resolveBuilderLogBaseDir(projectDirFlag)
    log, err := logger.NewLoggerWithPrefix(logBaseDir, "workflow")
    ...
    model := workflowedit.New(workflowio.RealSaveFS(), realEditorRunner{}, logBaseDir, workflowDirFlag).
        WithLog(log.Writer())
```

```go
func resolveBuilderLogBaseDir(projectDirFlag string) string {
    if projectDirFlag != "" {
        return projectDirFlag
    }
    if cwd, err := osGetwd(); err == nil {
        return cwd
    }
    ...
}
```

**Relevance:** The workflow builder (TUI editor) uses `logBaseDir` for the
builder's log file (not the workflow run log), and passes it as `projectDir`
to `workflowedit.New`. No subprocess execution uses this path; it is only
for the builder's session log and the default new-file save path
(`<projectDir>/.pr9k/workflow/config.json`).

---

## E24: WorkflowDir candidate 1 is derived from ProjectDir

**Source:** `src/internal/cli/args.go:26–37`

```go
func resolveWorkflowDirWith(projectDir, execDir string) (string, error) {
    projCandidate := filepath.Join(projectDir, ".pr9k", "workflow")
    if info, err := os.Stat(projCandidate); err == nil && info.IsDir() {
        return projCandidate, nil
    }
    execCandidate := filepath.Join(execDir, ".pr9k", "workflow")
    ...
}
```

**Relevance:** The first candidate for the workflow bundle is
`<projectDir>/.pr9k/workflow/`. If a worktree's `projectDir` does not have
a `.pr9k/workflow/` directory, resolution falls through to the executable's
directory. This is relevant if the user has a per-repo in-repo workflow
override: it would live in the worktree, not the main checkout.

---

## E25: `config.json` loaded relative to WorkflowDir, not ProjectDir

**Source:** `src/internal/steps/steps.go:144–145`

```go
func LoadSteps(workflowDir string) (StepFile, error) {
    path := filepath.Join(workflowDir, "config.json")
    data, err := os.ReadFile(path)
```

**Relevance:** `config.json` is always loaded from `workflowDir`. It is never
read from `projectDir`. Adding a new top-level field (e.g. `useWorktrees`)
to `config.json` would be parsed here.

---

## E26: No scripts use `git rev-parse --show-toplevel`

Searched all files in `workflow/scripts/` for `show-toplevel`, `is-inside-work-tree`,
`git-dir` (as argument), and worktree-related patterns. Only two git invocations found:

- `get_commit_sha`: `git rev-parse HEAD` — resolves to worktree's HEAD, correct.
- `statusline`: `git rev-parse --git-dir` — tests for git presence, not repo root.

No script assumes `git rev-parse --show-toplevel` equals `pwd`. No script
traverses from CWD to the git root.

---

## E27: `gh repo view` resolves repo from git remote, not CWD path structure

**Source:** `workflow/scripts/get_next_issue:4`

```bash
repo=$(gh repo view --json nameWithOwner -q ".nameWithOwner")
```

**Relevance:** `gh repo view` uses the git remote of the repo containing CWD,
not the directory path itself. In a worktree, CWD is inside the same repository
(same `.git` tree via the worktree's `.git` file pointer), so `gh repo view`
resolves the same remote as the main checkout.

---

## E28: No cleanup/quit path writes to ProjectDir outside existing paths

The graceful quit flow (`q`/`y`), SIGINT handling, and error mode
(`c`/`r`/`q`) are implemented in the TUI key handler and the `workflow.Run`
goroutine. On quit:

- `workflow.Run` returns `RunResult{}` (ActionQuit path).
- `log.Close()` is called — flushes and closes the log file under
  `<projectDir>/.pr9k/logs/`.
- No cleanup writes to ProjectDir; cidfile cleanup is in `sandbox.Cleanup`
  which removes a temp file in the OS temp directory, not in ProjectDir.

No evidence found of additional writes to ProjectDir on error or quit paths
beyond the log file and iteration.jsonl.

---

## Integration Point Summary

| # | Location | File:Line | What ProjectDir controls |
|---|----------|-----------|--------------------------|
| 1 | CLI resolution | `src/internal/cli/args.go:59` | Default value: `os.Getwd()` + symlink eval |
| 2 | `--project-dir` flag | `src/internal/cli/args.go:101` | Explicit override: symlink eval only |
| 3 | Preflight `.pr9k/` creation | `src/internal/preflight/run.go:36` | `mkdir <projectDir>/.pr9k` |
| 4 | Logger `.pr9k/logs/` creation | `src/internal/logger/logger.go:28` | Log file path |
| 5 | Artifact dir creation | `src/cmd/pr9k/main.go:104` | `<projectDir>/.pr9k/logs/<run-stamp>/` |
| 6 | Runner construction | `src/cmd/pr9k/main.go:112` | Stored in `Runner.projectDir` |
| 7 | `cmd.Dir` for all subprocesses | `src/internal/workflow/workflow.go:433` | CWD for every non-claude step |
| 8 | `cmd.Dir` for `CaptureOutput` | `src/internal/workflow/workflow.go:807` | CWD for capture commands |
| 9 | Docker bind-mount source | `src/internal/sandbox/command.go:47` | Host path mounted at `/home/agent/workspace` |
| 10 | Docker `-w` working dir | `src/internal/sandbox/command.go:49` | Container CWD = mount target (constant) |
| 11 | Iteration log | `src/internal/workflow/iterationlog.go:48` | `<projectDir>/.pr9k/iteration.jsonl` |
| 12 | Per-step JSONL artifacts | `src/internal/workflow/run.go:387` | `<projectDir>/.pr9k/logs/<stamp>/<file>.jsonl` |
| 13 | Statusline `cmd.Dir` | `src/internal/statusline/statusline.go:288` | CWD for statusline script |
| 14 | `{{PROJECT_DIR}}` var | `src/internal/vars/vars.go:73` | Token in prompts and commands |
| 15 | Statusline payload field | `src/internal/statusline/payload.go:46` | `projectDir` JSON field to scripts |
| 16 | WorkflowDir candidate 1 | `src/internal/cli/args.go:27` | `<projectDir>/.pr9k/workflow/` lookup |
| 17 | `sandbox shell` CWD | `src/cmd/pr9k/sandbox_shell.go:42` | Independent `os.Getwd()` — no `--project-dir` |
| 18 | Script relative paths | `workflow/scripts/post_issue_summary:10` | `.pr9k/iteration.jsonl` relative to CWD |
| 19 | Script relative paths | `workflow/scripts/review_verdict:19` | `code-review.md` relative to CWD |
| 20 | `config.json` load path | `src/internal/steps/steps.go:144` | Read from WorkflowDir, not ProjectDir |

