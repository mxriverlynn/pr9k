# Resume Behavior Analysis: Cross-Run Resume Options

This document traces the runtime behavior of six proposed cross-run resume strategies for pr9k, answering the twelve questions posed in the investigation prompt. Findings are numbered B-R1 through B-R28 and follow the four analysis dimensions used throughout this investigation.

---

## Context and Grounding

The six options under analysis:

- **A** — Persistent active-run pointer + stamped worktree (`.pr9k/active-run.json` records the current run-stamp; a new invocation reads it and re-enters the same worktree).
- **B** — Fixed worktree path (single per primary, branch reuse; e.g. `<primary>-pr9k-work/`).
- **C** — No worktrees — separate clone managed by pr9k (a full `git clone` into `<primary>-pr9k-clone/`).
- **D** — State file with per-iteration bookmarks (`.pr9k/active-run.json` records which issue + iteration was in progress; Run loop gains a skip-already-done-iterations mode).
- **E** — In-place workflow + branch management (no worktree; pr9k operates directly in the primary checkout but manages a dedicated branch).
- **F** — Hybrid: fixed worktree + state file (B's directory layout with D's bookmark file).

The iteration loop code under analysis is `/Users/mxriverlynn/dev/mxriverlynn/pr9k/src/internal/workflow/run.go`. The startup sequence under analysis is `/Users/mxriverlynn/dev/mxriverlynn/pr9k/src/cmd/pr9k/main.go`. The issue-selection script is `/Users/mxriverlynn/dev/mxriverlynn/pr9k/workflow/scripts/get_next_issue`. The push script is `/Users/mxriverlynn/dev/mxriverlynn/pr9k/workflow/scripts/git_push`.

---

## Question 1: What does "resume" mean at the iteration-loop level?

**B-R1: The loop already provides free resume-by-re-entry when the issue stays open**
- **Dimension:** Data Flow
- **File(s):** `src/internal/workflow/run.go:474–618`, `workflow/scripts/get_next_issue`
- **Finding:** The iteration loop in `Run` has no concept of "skip already-done iterations." Each pass through the loop calls `vt.ResetIteration()` and then the first capture step (`get_next_issue`) which determines what work to do:

```go
// run.go:474–483
for i := 1; cfg.Iterations == 0 || i <= cfg.Iterations; i++ {
    iterationsRun = i
    vt.ResetIteration()
    push(vars.Iteration)
    vt.SetIteration(i)
    ...
```

```bash
# get_next_issue: returns lowest-numbered open ralph issue
issuenum=$(gh api graphql ... | jq -r '[.data.search.nodes[] | select(...) | .number] | sort | .[0] // empty')
echo $issuenum
```

If the prior run was killed mid-iteration, the issue is still open, and `get_next_issue` returns the same issue number on the new run. The loop begins that iteration from step 1 (feature-work), with a clean `VarTable`.

**Impact:** For the mid-iteration case, "resume" is behaviorally equivalent to "start over on the same issue, in the same (or equivalent) working directory." No skip-mode is required. The simpler version — "re-enter with the same branch state; let feature-work re-run" — fully satisfies the user's goal of not losing the prior run's committed work.

---

## Question 2: Push-failure / closed-issue cascade on resume

**B-R2: The cascade: issue closed, push failed, user quit — per-option behavior**
- **Dimension:** Error Propagation
- **File(s):** `workflow/scripts/git_push`, `workflow/scripts/close_gh_issue`, `workflow/config.json`
- **Finding:** The default iteration sequence is:

```json
// config.json iteration steps (abbreviated):
"Close issue"  → scripts/close_gh_issue
"Git push"     → scripts/git_push
```

The push script currently traps all exits to zero:

```bash
# workflow/scripts/git_push:1-8
#!/usr/bin/env bash
trap 'exit 0' EXIT
result=$(git push)
echo $result
exit 0
```

Under D7 (the decision to surface push failures), this script will be changed to surface non-zero exits and route them into the c/r/q error-recovery prompt. But in the current codebase the push failure is swallowed, making the cascade invisible. Assuming D7 is implemented, the behavior on resume differs by option:

**Option A (state file + stamped worktree):** The state file records that the iteration completed (issue closed) but the push step failed. On resume, pr9k reads the state file, re-enters the stamped worktree. The branch has all the committed work. The push step runs again against the same branch with the same commits. If the blocking reason (e.g., no upstream) is fixed, the push succeeds. This is the correct behavior — the work is durable on the branch, the push retries cleanly.

**Option B (fixed worktree):** No state file. `get_next_issue` returns the next open issue (the closed one is skipped). The completed-but-unpushed iteration's work lives on the fixed branch, which now has commits for the closed issue. The next iteration adds more commits on top. The push that eventually succeeds pushes everything, including the prior iteration's work. This is correct but accidental — the user gets the push they needed, just batched with subsequent work.

**Option C (separate clone):** Same as B — no per-iteration state, the clone's branch has the commits. On resume the next issue runs on top of them.

**Option D (state file, no worktree change):** The bookmark records the failed push step. On resume, pr9k skips completed iterations and goes directly to the push step. This is the most precise recovery — it reissues only the push without re-running feature work. However, this requires a "jump to step N" capability the loop does not have today.

**Option E (in-place + branch):** The branch has the commits. On resume, the same issue-is-closed scenario applies: `get_next_issue` skips it. Next iteration runs on top. Same accidental-but-correct behavior as B and C.

**Option F (fixed worktree + state file):** Combination: the state file says "push failed, all prior steps done." The worktree has the commits. On resume, pr9k can skip to the push step.

**Impact:** Options B, C, and E produce the correct end state (all commits eventually pushed) without per-step bookmarking, at the cost of not cleanly isolating the retry. Options A, D, and F can produce a cleaner targeted retry but require step-level bookmarking and a "resume from step N" capability that does not exist in the current loop.

---

## Question 3: Branch state on resume — is accumulated commits correct?

**B-R3: Accumulating commits on the same branch across resume is the correct behavior**
- **Dimension:** State Management
- **File(s):** `src/internal/workflow/run.go`, `workflow/config.json`
- **Finding:** When a worktree (Options A, B, F) or clone (Option C) is re-entered on resume, the branch already has the prior run's commits. The next iteration's feature-work step invokes claude inside the Docker container, which is bind-mounted to the worktree/clone directory:

```go
// run.go:774
argv := sandbox.BuildRunArgs(projectDir, profileDir, uid, gid, cidfile, envAllowlist, containerEnv, resumeSessionID, s.Model, s.Effort, prompt)
```

```go
// command.go:47-49
"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", projectDir, ContainerRepoPath),
```

The container starts with the existing committed state visible. Feature work is additive — it can see what was already done and build on it.

This matches how the run would have behaved without the kill: commits accumulate on the same branch iteration after iteration. A `git checkout -b pr9k/stamp` started from HEAD and then received N iterations of commits; resume on the same branch starts from those N commits and receives more.

**Impact:** Correct behavior for Options A, B, C, F. No code change required to handle accumulated commits — the docker container always sees the current file state, which is what the feature-work prompt receives.

---

## Question 4: Workflow config divergence between runs

**B-R4: Config is always loaded from WorkflowDir at startup — resume always reads the new config**
- **Dimension:** Data Flow
- **File(s):** `src/cmd/pr9k/main.go:141`, `src/internal/steps/steps.go`
- **Finding:** `startup` calls `steps.LoadSteps(cfg.WorkflowDir)` at every invocation:

```go
// main.go:141
svc, ok := startup(cfg, cfg.ProjectDir, profileDir, preflight.RealProber{}, os.Stderr)

// main.go:63-67
func startup(cfg *cli.Config, projectDir, profileDir string, prober preflight.Prober, stderr io.Writer) (*services, bool) {
    stepFile, err := steps.LoadSteps(cfg.WorkflowDir)
```

`WorkflowDir` is resolved from the primary checkout's `.pr9k/workflow/` or the executable dir — not from any stored state. There is no "config snapshot" written by the prior run.

**Impact:** Every option (A through F) loads the current config at startup. The new run always uses the edited `config.json`. This is the correct behavior: if the user edited a step to fix a bug, the resumed run benefits from the fix. No option has a behavioral divergence risk here.

---

## Question 5: `get_next_issue` semantics on resume

**B-R5: Mid-iteration resume: same issue returned, feature-work re-runs on existing branch**
- **Dimension:** Data Flow
- **File(s):** `workflow/scripts/get_next_issue`, `src/internal/workflow/run.go:503–614`
- **Finding:** `get_next_issue` returns the lowest-numbered open ralph issue:

```bash
issuenum=$(gh api graphql ... | jq -r '[...] | sort | .[0] // empty')
```

When the prior run was killed mid-iteration and the issue is still open, the new run's first call to `get_next_issue` returns the same issue number. The iteration loop starts from step 1 (feature-work) with a clean VarTable.

For each option:
- **A (state file + stamped worktree):** The state file says which issue was in progress. The new run re-enters the worktree. `get_next_issue` returns the same issue. Feature-work runs on the prior commits. Correct — the branch already has partial work and feature-work can complete or extend it.
- **B (fixed worktree):** No state file. `get_next_issue` returns the same open issue. Feature-work starts on the fixed-path branch with its existing commits. Correct.
- **C (separate clone):** `get_next_issue` runs with `cmd.Dir` pointing at the clone. The clone has its own `.git` pointing back to the origin, and `gh` discovers the repo via the gitfile. Same issue returned. Correct.
- **D (state file only, in-place):** State file says issue N, step 5. pr9k skips iterations for already-closed issues and resumes mid-iteration at step 5. This requires loop modification. `get_next_issue` for a mid-skip check must be bypassed or the state file must supply the issue ID directly.
- **E (in-place):** `get_next_issue` returns same open issue. Feature-work runs in-place on the current branch. Correct.
- **F (fixed worktree + state file):** Same issue returned by get_next_issue. State file enables step-level skip. Correct.

**B-R6: Post-iteration resume: next issue returned, loop continues naturally — no option breaks this**
- **Dimension:** Data Flow
- **File(s):** `workflow/scripts/get_next_issue`
- **Finding:** If the prior run completed an iteration (issue closed), `get_next_issue` returns the next open issue. All options handle this identically — the loop simply continues to the next iteration. No option requires special handling for this case; the closed-issue state on GitHub is the authoritative record that the iteration is complete.

**Impact:** The post-iteration case is "free" for all options. The mid-iteration case is free for Options A, B, C, E, F (re-run is safe). Only Option D requires a "skip" mode that does not currently exist in the loop.

---

## Question 6: Variable scoping across resume

**B-R7: Persistent variables re-derive cleanly from startup; no rehydration needed**
- **Dimension:** State Management
- **File(s):** `src/internal/vars/vars.go:65–76`, `src/internal/workflow/run.go:314–315`
- **Finding:** The VarTable is constructed fresh at every `Run` call:

```go
// run.go:315
vt := vars.New(cfg.WorkflowDir, executor.ProjectDir(), cfg.Iterations)

// vars.go:65-76
func New(workflowDir, projectDir string, maxIter int) *VarTable {
    vt := &VarTable{persistent: make(map[string]string), ...}
    vt.persistent["WORKFLOW_DIR"] = workflowDir
    vt.persistent["PROJECT_DIR"] = projectDir
    vt.persistent["MAX_ITER"] = strconv.Itoa(maxIter)
    return vt
}
```

The built-in persistent variables (WORKFLOW_DIR, PROJECT_DIR, MAX_ITER, ITER, STEP_NUM, STEP_COUNT, STEP_NAME) are all derived from startup inputs, not from any saved state.

**B-R8: Initialize-phase captureAs variables are lost across runs — GITHUB_USER must re-derive**
- **Dimension:** State Management
- **File(s):** `workflow/config.json`, `src/internal/vars/vars.go`
- **Finding:** The initialize phase in `config.json` captures `GITHUB_USER`:

```json
{ "name": "Get GitHub user", "isClaude": false, "command": ["scripts/get_gh_user"], "captureAs": "GITHUB_USER" }
```

This is bound to the persistent table in `Bind`:

```go
// vars.go:120-122
case Initialize:
    vt.persistent[name] = value
```

On resume, the initialize phase runs again. `get_gh_user` is a fast, non-destructive script (`gh api user --jq .login`). The re-derivation is correct and cheap. No resume option has a problem here.

**B-R9: Iteration-scoped captures (`ISSUE_ID`, `STARTING_SHA`, `ISSUE_BODY`, etc.) start fresh each iteration — correct on resume**
- **Dimension:** State Management
- **File(s):** `src/internal/workflow/run.go:476`
- **Finding:** At the start of every iteration, the iteration table is reset:

```go
// run.go:476
vt.ResetIteration()
```

All iteration-scoped captures from `config.json` (`ISSUE_ID`, `STARTING_SHA`, `ISSUE_BODY`, `PROJECT_CARD`, `PRE_REVIEW_DIFF`) are re-derived from scratch by the first steps of the iteration. A resumed run gets fresh values for all of them. There is no dependency on any saved prior-run iteration state.

**Impact:** No option needs to save or rehydrate iteration variables. The current VarTable design is resume-safe for iteration scope.

---

## Question 7: TUI continuity

**B-R10: TUI renders fresh on resume — run-stamp, iteration counter, log path are all new**
- **Dimension:** State Management
- **File(s):** `src/cmd/pr9k/main.go`, `src/internal/logger/logger.go`
- **Finding:** Each pr9k invocation generates a new run-stamp from the wall clock at `logger.NewLogger`:

```go
// logger.go:34-36
now := time.Now()
layout := prefix + "-2006-01-02-150405.000"
runStamp := now.Format(layout)
```

The TUI header (`header.IterationLine`) is set at startup from fresh state. There is no mechanism to inherit or display a prior run-stamp.

**Impact:** A user who knows pr9k was killed and restarted will see iteration counter starting at 1, a new log file path, and no indication that this is a resumed run. For Options A, D, F (which have state files), displaying "RESUMED FROM ralph-2026-05-07-143022.123" in the TUI iteration line is possible with a single string passed through `RunConfig` or `header.RenderIterationLine`. For Options B, C, E (no state file), the information is not available without a file read.

The behavioral risk is low — the user explicitly restarted pr9k and presumably knows they did so. The feedback gap is that a user who is away from the terminal cannot distinguish a fresh run from a resumed one in the log output without correlating log timestamps.

---

## Question 8: Logs across resume

**B-R11: New log directory per invocation is the simpler and correct answer**
- **Dimension:** State Management
- **File(s):** `src/internal/logger/logger.go:27–48`
- **Finding:** The logger creates a new file at every `NewLogger` call:

```go
// logger.go:34-40
now := time.Now()
layout := prefix + "-2006-01-02-150405.000"
filename := now.Format(layout + ".log")
runStamp := now.Format(layout)
logPath := filepath.Join(logsDir, filename)
f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
```

The `iteration.jsonl` file in `.pr9k/` is appended to (not replaced) on every invocation, because `AppendIterationRecord` opens with `os.O_APPEND`:

```go
// iterationlog.go:49
f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
```

This means the iteration log accumulates records across all invocations that share the same `projectDir`. For Options A, B, F (same worktree across runs), the `iteration.jsonl` inside the worktree will contain records from both the original run and the resumed run, with no delimiter between them. For Options C, E (clone/in-place with fixed dir), same behavior in the shared `.pr9k/` directory.

**Impact:** New log file per invocation is the simpler answer and requires no change. The `iteration.jsonl` accumulation across same-directory runs is correct — it provides a full audit trail of all attempts. The recommended answer: keep per-invocation log files, let `iteration.jsonl` accumulate naturally. No merge behavior needed.

---

## Question 9: autoCleanup interaction on resumed runs

**B-R12: autoCleanup on a resumed run removes the worktree that had the prior run's commits**
- **Dimension:** State Management / Error Propagation
- **File(s):** `docs/plans/git-worktrees/artifacts/decision-log.md` (D13), `src/cmd/pr9k/main.go`
- **Finding:** D13 states that `autoCleanup: true` causes pr9k to run `git worktree remove --force` and `git branch -D pr9k/<run-stamp>` at the end of the graceful-shutdown path.

For each option under resume:

**Option A (state file + stamped worktree):** The resumed run re-enters the same worktree (same stamp, same path). On graceful completion, `autoCleanup` removes it. The state file (`.pr9k/active-run.json`) must also be removed at this point — otherwise the next invocation reads a stale active-run record pointing at a branch/directory that no longer exists. If the state file is not cleaned up, the next invocation reads it, tries to `git worktree add` for a branch that was deleted, and fails or creates an orphaned worktree.

**Option B (fixed worktree):** `autoCleanup` removes the single fixed-path worktree and its branch. On the next invocation, `git worktree add` creates a fresh fixed path. The cleanup lifecycle is per-run even though the path is fixed: the worktree is removed and re-created each time. This is the simplest lifecycle.

**Option C (separate clone):** `autoCleanup` would need to decide whether to delete the entire clone directory. A full-clone deletion is more destructive than a worktree removal. If the clone is left in place across runs (as the resume premise requires), autoCleanup cannot safely delete the clone on a first successful run without breaking subsequent resumes.

**Option D (state file, in-place):** autoCleanup does not apply to the directory (no worktree to remove). The state file must be deleted on graceful completion. If it isn't, the next run reads it, decides it's a resume, and skips iterations that were already completed — which is correct. But if it is stale (refers to an issue that no longer exists), the skip logic may fail silently.

**Option E (in-place):** No worktree, no state file. autoCleanup only applies to the branch management. On graceful completion, pr9k would delete the `pr9k/stamp` branch. The primary checkout is unaffected.

**Option F (fixed worktree + state file):** Same lifecycle question as A: both the worktree and the state file must be cleaned up atomically on graceful completion, or the next run may see a stale state file pointing at a deleted worktree.

**Impact:** Options A and F have a compound cleanup requirement: state file + worktree must both be cleaned up together. Missing either leaves the system in an inconsistent state. Options B and E have the simplest cleanup lifecycle (remove worktree, or nothing).

---

## Question 10: "User wants to abandon and start fresh" affordance

**B-R13: The simplest mechanism is a CLI flag read before the state file is consulted**
- **Dimension:** Integration Boundaries
- **File(s):** `src/internal/cli/args.go`
- **Finding:** `cli.Config` currently has three fields: `Iterations`, `WorkflowDir`, `ProjectDir`. There is no `--reset` or `--no-resume` flag. The state file (Options A, D, F) would be consulted inside `startup` or between `cli.Execute` and `startup` in `main.go:128–143`.

The cleanest seam is a `--no-resume` flag that, when present, causes the state-file read to be skipped and optionally removes the state file. The seam is in `cli.Config` (one field addition) and in the pre-startup state-file check (one early-return path). This is a single-point change.

For Options B, C, E (no state file), "start fresh" means deleting the fixed worktree/clone before creating a new one, or letting `pr9k worktree prune` do it. The user affordance already exists via `pr9k worktree prune` for B and the equivalent subcommand for C.

**Impact:** For all state-file options (A, D, F): a `--no-resume` flag is the minimal seam, implementable in one place. For non-state-file options (B, C, E): no code change needed — `pr9k worktree prune` or the equivalent removal command is sufficient.

---

## Question 11: Race conditions on startup state-file read

**B-R14: Concurrent starts with a state file produce a clean second-create failure, not silent corruption**
- **Dimension:** Error Propagation
- **File(s):** `src/internal/workflow/run.go`, `docs/plans/git-worktrees/artifacts/decision-log.md` (D10)
- **Finding:** D10 already declares concurrent runs as a precondition violation. For Options A, D, F, two concurrent starts would both read the same state file. Both would agree "this is a resume." Both would attempt to re-enter the same worktree.

For Option A (stamped worktree): `git worktree add` for the same branch/path fails on the second attempt with "fatal: '<path>' already exists" or "fatal: branch '<name>' is already checked out." This is a clean, visible failure — not silent corruption. The second pr9k invocation surfaces the git error and exits.

For Option D (no worktree): both processes attempt to run the iteration loop against the same directory simultaneously. The second one might execute the same claude steps, write to the same `iteration.jsonl`, and close the same issue again (a no-op from GitHub's perspective, but a wasted API call). This is messier than a clean git error.

For Options B, F (fixed worktree): `git worktree add` for the already-existing fixed path fails clean, same as A.

**Impact:** State-file options that involve worktree creation (A, B, F) benefit from git's natural exclusion — the second `git worktree add` fails with a clear error. Option D (no worktree) has the worst race behavior: two parallel in-place runs with no exclusion mechanism. The race is the same as D10's already-documented risk, so no new mitigation is needed beyond the existing precondition documentation.

---

## Question 12: Corrupt state file

**B-R15: A corrupt or missing state file must be treated as "no active run," not a crash**
- **Dimension:** Error Propagation
- **File(s):** `src/internal/atomicwrite/write.go` (relevant to write safety)
- **Finding:** `atomicwrite.Write` (used throughout the system for durable writes) performs a temp-file-plus-rename to prevent partial writes. If the state file is written via `atomicwrite.Write`, an interrupted write leaves either the old complete file or the new complete file — never a partially written file:

```go
// atomicwrite/write.go: temp-file + rename pattern
```

However, the rename is not transactional with the prior file's existence. A manual edit of `.pr9k/active-run.json` (e.g., a user correcting an issue number) could produce invalid JSON.

For the corrupt-state-file case, the correct behavior is: `json.Unmarshal` returns a non-nil error; the state-file read returns an error; the startup path treats this as "no active run" and proceeds with a fresh run, optionally logging a warning. It must not crash.

The "backup and start fresh" behavior (rename the corrupt file to `.pr9k/active-run.json.corrupt`) preserves the user's ability to inspect what was written, without blocking the run. This is a single-path addition to the state-file read function.

**Impact:** For all state-file options (A, D, F): the read function must handle `json.Unmarshal` errors explicitly and degrade to "no active run" rather than propagating the error to startup as a fatal failure. This is a three-line guard that does not exist today (because no state file exists today).

---

## Per-Option Behavioral Profiles

### Option A: Persistent active-run pointer + stamped worktree

**B-R16: Clean re-entry via state file, but compound cleanup requirement**
- **Dimension:** State Management
- **File(s):** `src/cmd/pr9k/main.go`
- **Finding:** The state file records `{ "runStamp": "ralph-...", "worktreePath": "...", "branchName": "pr9k/..." }`. On resume, pr9k reads it, sets `cfg.ProjectDir` to the worktree path instead of creating a new one, and runs. The VarTable is re-built from scratch (correct per B-R7). The iteration loop re-starts from iteration 1 with `get_next_issue` (correct per B-R1). The prior commits on the branch are visible to feature-work.

The single compound risk: on graceful completion with `autoCleanup: true`, both the worktree and the state file must be removed atomically (or in the right order). The state file must be removed *after* the worktree removal succeeds; if the removal fails and the state file was already deleted, the next run creates a new worktree and loses the resume pointer. If the state file is left after successful worktree removal, the next run reads it, finds the branch and path are gone, and must handle the "worktree no longer exists" case (a call to `git worktree add` would fail with "branch already exists" if the branch wasn't deleted, or would create a new worktree on the old branch if it was deleted).

**Impact:** Option A is behaviorally sound at steady state but requires careful ordering in the cleanup path. The cleanup has two steps (remove worktree, then remove state file) that must be atomic from the perspective of "no observer sees a valid state file pointing at a deleted worktree."

---

### Option B: Fixed worktree path, branch reuse

**B-R17: Zero state files, simplest lifecycle, but no per-step resume**
- **Dimension:** State Management
- **File(s):** `src/cmd/pr9k/main.go`
- **Finding:** A fixed path like `<parent>/<basename>-pr9k-work/` is created if it doesn't exist, or re-entered if it does. The branch on that worktree accumulates all commits from all runs. `get_next_issue` determines what to do next — no bookmarks, no skip logic.

The single behavioral gap: if the fixed worktree has a different branch than expected (e.g., the user manually checked out a different branch inside the fixed worktree), the run proceeds on the wrong branch. There is no guard.

The branch naming also requires a decision: does the fixed worktree reuse the same branch name (e.g., `pr9k/work`) across all runs, or does it create a new branch each time? A single reused branch accumulates all commits from all runs, which is probably the desired behavior for resume continuity but differs from the stamped-branch model in D2.

`autoCleanup` on a fixed worktree removes it and the branch. The next resume invocation would find no worktree, create a fresh one, and start over — losing the branch history. If the user wants resume continuity, they must leave `autoCleanup: false`.

**Impact:** Option B is the simplest state management story — no files to corrupt, no cleanup ordering to enforce. The tradeoff is that resume granularity is coarse (always restarts from `get_next_issue`) and the "start fresh" path requires either `pr9k worktree prune` or a `--reset` flag that recreates the fixed worktree on a new branch.

---

### Option C: Separate clone

**B-R18: Full clone is a first-class repository copy; `gh` discovery works but adds network dependency**
- **Dimension:** Integration Boundaries
- **File(s):** `workflow/scripts/get_next_issue`, `src/internal/sandbox/command.go`
- **Finding:** A separate clone (`git clone <origin> <clone-path>`) creates a full repository. Git commands and `gh` discover the repo via `.git/config`'s `origin` remote. The `gh repo view` in `get_next_issue` walks up from `cmd.Dir` to find `.git/config` and reads the remote URL — this works identically to a worktree's gitfile.

The behavioral difference from worktrees: a clone is a full copy of all objects. Creating it requires a network call to the origin (or a local clone if the primary is local). A `git clone` of a large repository can take seconds to minutes; a `git worktree add` is milliseconds.

On resume, the clone already exists and is re-entered. No network call needed for re-entry. However, the clone's branch may be behind the primary's HEAD (if the user made commits to the primary while pr9k was stopped). A `git fetch origin` before resuming would sync it, but there is no mechanism for this in the current workflow.

**Impact:** Option C has no simpler lifecycle than Option B (worktree) and adds a network-dependent creation step. The Docker bind-mount path (B4 in investigation 03) is unaffected — `source=<clonePath>` works identically to `source=<worktreePath>`. The main behavioral risk is clone staleness across resume invocations with an active upstream.

---

### Option D: State file with per-iteration bookmarks

**B-R19: Requires loop modification — "skip already-done iterations" mode does not exist**
- **Dimension:** Data Flow
- **File(s):** `src/internal/workflow/run.go:474–618`
- **Finding:** The current iteration loop has no concept of starting at a specific step within an iteration. The loop always calls every step in order, relying on `skipIfCaptureEmpty` and `breakLoopIfEmpty` for conditional logic. Adding a "skip to step N" mode requires:

1. A way to pass the bookmark into `Run` (a new `RunConfig` field, e.g., `ResumeFromStep int`).
2. A conditional inside the per-step loop that skips steps before the bookmark.
3. Re-deriving VarTable state for the skipped steps (the captures from those steps will be empty unless the state file also records them).

Step 3 is the hard part. If `ISSUE_ID` was captured by step 3 of the iteration, and the resume bookmark is step 7, the VarTable has no `ISSUE_ID` unless the state file saved it. The state file would need to record all captures at each bookmark, not just the step number.

**Impact:** Option D requires the most invasive loop modification of all six options. It is the only option that requires new behavior inside `run.go`'s iteration loop body. All other options' resume behavior falls out naturally from re-entry with `get_next_issue` as the state-reconstruction step.

---

### Option E: In-place workflow + branch management

**B-R20: Primary checkout mutations are the core behavioral risk**
- **Dimension:** State Management
- **File(s):** `src/cmd/pr9k/main.go`, `src/internal/workflow/workflow.go`
- **Finding:** In-place execution means `r.projectDir = cfg.ProjectDir` (the primary checkout). All claude steps, all `git` commands, all file writes go into the primary checkout. The feature specification's invariant T1 explicitly prohibits this:

> "pr9k must not write to, change branches in, or otherwise mutate the primary checkout's working tree while the run is in progress."

For the resume use case specifically (no worktrees enabled), this invariant does not apply because the user chose not to use worktrees. In-place is today's default behavior, and resume in-place would mean the same things that happen in a worktree happen in the primary checkout instead.

The behavioral risk unique to resume in-place: if pr9k was killed mid-feature-work (uncommitted changes in the primary checkout), the primary checkout has a dirty working tree. The new in-place run starts with those uncommitted changes visible. The feature-work step sees them and may proceed incorrectly or create conflicts.

**Impact:** Option E works correctly at the iteration-boundary level (issue closed, all changes committed, git state is clean). At the mid-step level (killed during feature-work, uncommitted changes), in-place resume has a dirty-working-tree problem that worktrees avoid entirely (because `git worktree remove --force` or `git restore` inside the worktree doesn't touch the primary).

---

### Option F: Hybrid (fixed worktree + state file)

**B-R21: Compound of B's lifecycle and D's precision — both benefits and both complexities**
- **Dimension:** State Management
- **File(s):** `src/cmd/pr9k/main.go`
- **Finding:** Option F adds a state file to Option B. The fixed worktree provides directory stability; the state file provides step-level bookmark precision. The state file must record captures for skipped steps (same problem as B-R19 for step-level skipping). If the state file only records the issue number and not all captures, step-skipping is limited to the coarse "re-run from feature-work on the same issue" behavior — which is identical to what Option B achieves without the state file.

The compound cleanup risk (B-R12): fixed worktree removal + state file removal must be coordinated. On `autoCleanup`, both must be removed. If only the state file is removed (e.g., the worktree removal failed), the next run creates a new worktree at the same fixed path (because the old one was left in place with the same path but no state pointer), then fails with "already exists."

**Impact:** Option F's behavioral advantage over Option B is only realized if the state file records per-step captures. Without that, Option F is Option B with an added state file and an added cleanup ordering constraint — strictly more complex for no behavioral gain.

---

## Cross-Cutting Findings

**B-R22: The `resumePrevious` (intra-run Claude session resume) gates are cross-run unsafe by design**
- **Dimension:** State Management
- **File(s):** `src/internal/workflow/run.go:269–287`
- **Finding:** `evaluateResumeGates` uses `prevStats.SessionID` from the current run's in-memory `prevIterStats`/`prevInitStats`:

```go
// run.go:271-287
func evaluateResumeGates(
    prevStats claudestream.StepStats,
    prevState ui.StepState,
    blacklisted func(string) bool,
) (sessionID string, gate string) {
    if prevStats.SessionID == "" {
        return "", "G1: previous step has no session ID"
    }
```

These are local variables declared fresh at every `Run` call. They never persist to disk. A resumed run always starts with `prevInitStats = claudestream.StepStats{}` (zero value), which causes G1 to fail ("no session ID"), falling back to a fresh Claude session. This is the documented, correct, and safe behavior — the D15 decision log explicitly states "`resumePrevious` does not bridge across runs."

**Impact:** No cross-run resume option can or should rehydrate a prior Claude session ID into `prevStats`. The existing gates prevent it. This is confirmed sound.

---

**B-R23: The `iteration.jsonl` accumulation is the natural resume audit trail**
- **Dimension:** Data Flow
- **File(s):** `src/internal/workflow/iterationlog.go:47–49`
- **Finding:** `AppendIterationRecord` opens with `os.O_APPEND`:

```go
f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
```

For Options A, B, F (same worktree directory), the `iteration.jsonl` file inside the worktree's `.pr9k/` accumulates records from both the original run and the resumed run. The `run_stamp` is embedded in each record (via `rec.SessionID` indirectly through `cfg.RunStamp` passed through the artifact path), making it possible to correlate records per invocation even though they're in the same file.

**Impact:** The `iteration.jsonl` file serves as a natural cross-resume audit log for all options that share a directory. No special merge logic is needed — the file already accumulates. For the TUI "RESUMED FROM" display, the prior run-stamp can be read from the last record in the file before the current run's records start.

---

**B-R24: `git push` silent failure under D7 is the same cross-run risk for all options**
- **Dimension:** Error Propagation
- **File(s):** `workflow/scripts/git_push`
- **Finding:** The current `git_push` script:

```bash
trap 'exit 0' EXIT
result=$(git push)
echo $result
exit 0
```

This traps all exits to 0. D7 commits to fixing this so push failures enter the c/r/q prompt. Until D7 is implemented, all six options share the same risk: a push failure is silent, and the loop continues to the next iteration. The closed-issue-but-no-push scenario (B-R2) applies identically to all options until D7 lands.

After D7 is implemented, the push failure enters the c/r/q prompt. If the user chose `q` at that prompt, the run stops with uncommitted push. On resume, the branch has the commits but they were never pushed. The resume run re-enters the same branch; `get_next_issue` skips the closed issue; the next iteration runs and eventually its push step pushes all accumulated commits — including the prior iteration's. This is the correct behavior described in B-R2.

**Impact:** D7 implementation is the prerequisite for any resume option to correctly surface the push-failure cascade. Without D7, all options have the silent-failure risk. With D7, all options except D (which can skip directly to the push step) handle the cascade by accumulating commits and pushing them together.

---

**B-R25: The `STARTING_SHA` capture and its diff use the worktree HEAD — correct for resume on existing branch**
- **Dimension:** Data Flow
- **File(s):** `workflow/config.json`, `workflow/scripts/`
- **Finding:** The iteration sequence in `config.json`:

```json
{ "name": "Get starting SHA", "command": ["git", "rev-parse", "HEAD"], "captureAs": "STARTING_SHA" },
...
{ "name": "Get post-feature diff", "command": ["git", "diff", "{{STARTING_SHA}}..HEAD", "--stat"], "captureAs": "PRE_REVIEW_DIFF" }
```

On resume where the branch has prior commits, `HEAD` is the tip of those prior commits. `STARTING_SHA` is set to that tip. The diff `STARTING_SHA..HEAD` after feature work shows only the new changes made in *this iteration*, not all prior iterations' changes. This is correct — the diff is meant to describe what feature-work did for the current issue.

**Impact:** No issue. The self-consistent SHA capture works correctly whether the branch is fresh (SHA = primary HEAD) or has prior commits (SHA = tip of prior work). The diff always shows only the current iteration's changes.

---

## Behavioral Summary

**Focus area analyzed:** Runtime behavior of six cross-run resume strategies across all twelve investigation questions. Traces extended through: `src/internal/workflow/run.go` (iteration loop), `src/cmd/pr9k/main.go` (startup sequence), `src/internal/vars/vars.go` (VarTable scoping), `src/internal/workflow/iterationlog.go` (audit trail), `workflow/scripts/get_next_issue`, `workflow/scripts/git_push`, `workflow/config.json` (full step sequence), and all relevant decision-log entries (D2, D7, D10, D13, D14, D15).

**Key concerns:**

1. **Option D requires loop modification (B-R19)** — "skip already-done iterations" is not a free behavior; it requires a new `RunConfig` field, conditional logic inside the per-step loop, and state-file-recorded captures for all skipped steps. This is the only option that requires changes inside `run.go`.

2. **Compound cleanup for state-file options (B-R12, B-R21)** — Options A and F require coordinated removal of both the state file and the worktree on graceful completion. The ordering constraint (worktree first, then state file) creates a failure window that does not exist for Options B, C, or E.

3. **D7 (push failure surfacing) is prerequisite for all options (B-R24)** — without it, the push-failure/closed-issue cascade is silent regardless of resume strategy. D7 must land before any resume option correctly handles this scenario.

**Well-handled areas:**

- **Config loading is always fresh (B-R4)** — all six options naturally pick up an edited `config.json` on resume because `LoadSteps` reads from `WorkflowDir` on every invocation. No option needs special config versioning.
- **VarTable scoping is resume-safe by design (B-R7, B-R8, B-R9)** — built-in variables re-derive from startup inputs; `GITHUB_USER` re-derives cheaply from `get_gh_user`; iteration captures restart each iteration from `get_next_issue` forward. No option needs to rehydrate VarTable state.
- **intra-run `resumePrevious` gates are already cross-run safe (B-R22)** — G1 blocks on the zero-value `prevStats` at the start of every new `Run` call. No option can accidentally resume a Claude session from a prior pr9k invocation.
- **`STARTING_SHA` diff is self-consistent on existing branches (B-R25)** — the SHA captures the branch tip at the start of the iteration, so the diff always shows only the current iteration's work regardless of how many prior commits are on the branch.

**Skipped dimensions:** None skipped. All four dimensions (Data Flow, Error Propagation, State Management, Integration Boundaries) were assessed for each option.
