# Investigation: Running pr9k in a Git Worktree

One-sentence summary: We want pr9k to optionally run its workflow in an isolated git worktree so the user's primary checkout stays usable while pr9k mutates the next feature.

## Problem Statement

- **Symptom**: When pr9k runs against the user's main checkout, it commits, switches branches, and rewrites files continuously. The user cannot run the app or test other features in the same checkout while pr9k is iterating because the code is in flux.
- **Expected behavior**: A `useWorktrees: true` toggle at the top level of `workflow/config.json` should redirect pr9k's entire workflow execution into a freshly created git worktree. The user's primary checkout remains untouched and runnable.
- **Conditions**: Only matters when the user wants to keep using the primary checkout in parallel; otherwise the existing in-place behavior is fine. Default must remain in-place (no breaking change).
- **Impact**: Without this, pr9k and the developer compete for the same working tree; one of them has to stop.

## Evidence Summary

Evidence is grouped by the angle each agent investigated. Source files for the full quoted command output and code snippets are listed at the top of each group; only the load-bearing items appear below.

### Group A — Git worktree mechanics
**Source file:** [`01-worktree-mechanics.md`](01-worktree-mechanics.md). Git version tested: 2.50.1 (Apple Git-155).

#### E1: A branch can be checked out in only one worktree at a time
- **Source:** `01-worktree-mechanics.md:84` (live `git worktree add` output)
- **Finding:**
  ```
  fatal: 'main' is already used by worktree at '.../main-repo'
  ```
- **Relevance:** If the user is on branch X in the primary checkout, pr9k cannot create a worktree on the same branch X. The new worktree must sit on a different branch (or a detached HEAD). This is the single most important constraint shaping the design.

#### E2: `git worktree add <path>` with no branch arg auto-creates a branch named after the path basename
- **Source:** `01-worktree-mechanics.md:574`
- **Finding:** `git worktree add ../wt-foo` creates a new branch `wt-foo` from HEAD.
- **Relevance:** We can use this form (or the explicit `-b <name>` form) to manufacture a fresh branch off the primary's HEAD without touching the primary's branch.

#### E3: `.git` in a linked worktree is a gitfile, and `gh`/`git rev-parse --show-toplevel` honor it
- **Source:** `01-worktree-mechanics.md:150` (admin dir layout) and `01-worktree-mechanics.md:484, 496` (gh/git behavior tests)
- **Finding:** Inside a worktree, `git rev-parse --show-toplevel` returns the worktree root; `gh issue list`, `gh pr create`, and `git push` all behave identically to the primary checkout because remote config is shared.
- **Relevance:** No script in `workflow/scripts/` needs to know it's running in a worktree. `gh` and `git` Just Work.

#### E4: `git status` noise depends on worktree placement
- **Source:** `01-worktree-mechanics.md:610`
- **Finding:** Sibling and `.git/`-internal worktrees produce no `git status` noise; child-of-repo worktrees show as untracked dirs.
- **Relevance:** Eliminates "child of project root" as a viable location.

#### E5: Worktree cleanup paths
- **Source:** `01-worktree-mechanics.md:182`
- **Finding:** `git worktree remove <path>` is the clean path; if the directory is deleted out-of-band, `git worktree prune` cleans the admin dir. `--force` is needed to remove a worktree with uncommitted changes; locked worktrees survive single `--force` and require `--force --force`.
- **Relevance:** pr9k's cleanup path needs `git worktree remove --force` (because the workflow inherently leaves uncommitted intermediate files like `progress.txt`, `deferred.txt`, etc., before pushing). A startup probe needs `git worktree prune` for stale entries from crashed runs.

#### E6: `extensions.worktreeConfig` exists but isn't needed for our use case
- **Source:** `01-worktree-mechanics.md:413`
- **Finding:** Per-worktree `git config` is opt-in via `extensions.worktreeConfig`. Without it, all worktrees share the same config, hooks, and ignore rules.
- **Relevance:** We don't need any per-worktree config divergence. Skip this feature.

### Group B — pr9k ProjectDir flow (20 integration points)
**Source file:** [`02-projectdir-flow.md`](02-projectdir-flow.md).

#### E7: `Runner.projectDir` is the single seam
- **Source:** `src/cmd/pr9k/main.go:112`, `src/internal/workflow/workflow.go:119`
- **Finding:** `projectDir` is captured at startup (CLI resolution), then passed once into `workflow.NewRunner`, where it becomes `r.projectDir`. Every subprocess `cmd.Dir`, every Docker bind-mount, every artifact path, every `{{PROJECT_DIR}}` substitution, and every statusline payload reads from this single field.
- **Relevance:** Redirecting `projectDir` to a worktree path before `NewRunner` propagates the change everywhere downstream. This is the critical leverage point.

#### E8: Docker bind-mount takes `projectDir` as a parameter
- **Source:** `src/internal/sandbox/command.go:47`
- **Finding:** `BuildRunArgs` constructs `--mount source=<projectDir>,target=/home/agent/workspace,...`. The container always sees its workspace at `/home/agent/workspace` regardless of host path.
- **Relevance:** A worktree path bind-mounts cleanly. Container code paths require zero changes.

#### E9: All non-claude subprocesses use `cmd.Dir = r.projectDir` — no `os.Chdir` is ever called
- **Source:** `src/internal/workflow/workflow.go:433` and `:807`
- **Finding:** The Go process's working directory is never changed; subprocess CWDs are set explicitly per-call.
- **Relevance:** We do not need to (and must not) `os.Chdir` into the worktree. The seam at `r.projectDir` is sufficient. This also means scripts are isolated via `cmd.Dir` — they can't accidentally observe the parent's cwd.

#### E10: Logs and artifact paths construct `<projectDir>/.pr9k/logs/...` in four places
- **Source:** `src/cmd/pr9k/main.go:104`, `src/internal/logger/logger.go:28`, `src/internal/workflow/run.go:387`, `src/internal/workflow/iterationlog.go:48`
- **Finding:** Each call site joins `projectDir` with a `.pr9k/...` suffix.
- **Relevance:** **Scattered concern.** If `projectDir` becomes the worktree and we delete the worktree on cleanup, all four artifact streams vanish. Either (a) keep the worktree on disk after the run, (b) split a `logDir` apart from `projectDir` so logs land in the primary checkout, or (c) accept that logs live in the worktree and require the user to opt out of cleanup for postmortems.

#### E11: WorkflowDir resolution is independent of the worktree decision
- **Source:** `src/internal/cli/args.go:27`, `src/internal/steps/steps.go:144`
- **Finding:** `config.json` is loaded from `workflowDir`, which is resolved from `<projectDir>/.pr9k/workflow/` first (in-repo override) then `<executableDir>/.pr9k/workflow/` (shipped bundle). The in-repo override path uses the **primary checkout's** `projectDir` because the worktree doesn't exist yet at config-load time.
- **Relevance:** `useWorktrees: true` can be read before the worktree exists — there is no chicken-and-egg. After the worktree is created, we should NOT re-resolve the workflow dir; it stays anchored to the primary checkout (so the workflow bundle isn't subject to the worktree's mid-iteration code mutations).

#### E12: `sandbox shell` and `pr9k workflow` (builder) resolve their own ProjectDir
- **Source:** `src/cmd/pr9k/sandbox_shell.go:42` and `02-projectdir-flow.md` E23
- **Finding:** Both subcommands call `os.Getwd()` independently; neither flows through the workflow runner.
- **Relevance:** These subcommands are out of scope for `useWorktrees`. The toggle is a workflow-execution concern only.

#### E13: No script in `workflow/scripts/` calls `git rev-parse --show-toplevel` or otherwise hard-codes a path
- **Source:** `02-projectdir-flow.md` E26
- **Finding:** All scripts rely on `cmd.Dir` (set by the runner) and on `gh`/`git` discovering the repo from cwd.
- **Relevance:** No script changes needed.

### Group C — Behavioral seams and risks
**Source file:** [`03-behavioral-boundaries.md`](03-behavioral-boundaries.md).

#### E14: The `git_push` script silently swallows failures
- **Source:** `workflow/scripts/git_push` (`trap 'exit 0' EXIT`)
- **Finding:** Any push failure exits 0, so the workflow continues happily even when the push didn't happen.
- **Relevance:** **Becomes a new failure mode in the worktree case.** A new worktree branch has no upstream tracking ref; a bare `git push` from inside it will fail. The script's silent-exit habit will hide the failure: issues get closed, no code lands. This is a pre-existing issue but worktree mode forces us to confront it. Mitigation: either fix the script (use `git push -u origin HEAD` and let the trap go), or have pr9k set the upstream when it creates the worktree branch, or both.

#### E15: There is no deferred cleanup or startup probe for stale worktrees
- **Source:** `03-behavioral-boundaries.md` B17
- **Finding:** No code anywhere calls `git worktree remove`, `git worktree prune`, or otherwise tracks active worktrees. SIGKILL, panic, or power loss leaves the worktree on disk indefinitely.
- **Relevance:** New capability gap. We need both (a) a graceful cleanup path on quit/finish/error and (b) a startup probe that detects and prunes stale entries (or warns and asks).

#### E16: Statusline runs continuously with `cmd.Dir = r.projectDir`
- **Source:** `src/internal/statusline/statusline.go:288`
- **Finding:** The statusline subprocess inherits the runner's `projectDir`, so any `git status`/`git log` inside the user's statusline script reflects the worktree's state.
- **Relevance:** Free win — statusline shows useful info about the worktree branch with no changes.

## Root Cause Analysis

### Summary

There is no real "root cause" — this is a feature investigation, not a bug fix. The "cause" of the user's problem is that pr9k operates in-place by design (E7), so any developer activity in the same checkout collides with whatever pr9k is doing.

### Detailed Analysis

The feature can be implemented because pr9k's architecture is friendly to it:

1. The `Runner.projectDir` field is a single seam (E7). Set it to a worktree path once at startup and the entire downstream chain — Docker bind-mounts (E8), subprocess CWDs (E9), `{{PROJECT_DIR}}` substitution, statusline (E16), `gh` discovery (E3) — automatically follows.

2. Git worktrees are well-supported and battle-tested (E1–E6). The branch-uniqueness rule (E1) is the only hard constraint we must encode in our design: pr9k must not try to put the worktree on the same branch as the primary checkout. Creating a fresh branch off the primary's HEAD (E2) is the natural answer.

3. `gh` and all workflow scripts work transparently inside a worktree (E3, E13). Zero script changes required.

4. Three concerns must be solved deliberately, not by accident:
   - **Log durability (E10)**: scattered across four files. We must decide: keep the worktree, split `logDir`, or accept ephemeral logs.
   - **Crash cleanup (E15)**: needs a deferred remove on the happy path and a startup probe for stale entries.
   - **Silent push failure (E14)**: pre-existing bug that becomes a much louder problem under worktrees.

## Coding Standards Reference

| Standard | Source | Applies To |
|----------|--------|------------|
| Errors are package-prefixed and include file paths in I/O messages | `docs/coding-standards/error-handling.md` | New `worktree` package error messages |
| Atomic file writes via `atomicwrite.Write` | `docs/coding-standards/file-writes.md` | Any persistence we add (e.g., a "worktree state" file) — but our preferred design avoids new persisted state |
| Symlink-safe path resolution | `docs/coding-standards/go-patterns.md` | Computing the worktree path; we already evaluate symlinks for projectDir |
| Race-detector tests required | `docs/coding-standards/testing.md` | Anything we add involving lifecycle — keep it simple to limit shared state |
| Narrow-reading principle: pr9k is a generic step runner; workflow content lives in `config.json` | ADR `20260410170952-narrow-reading-principle.md` | Validates putting `useWorktrees` in `config.json` rather than in Go code or as a hidden CLI flag |
| Two-directory split: `WorkflowDir` (bundle) vs `ProjectDir` (target repo) | ADR `20260413162428-workflow-project-dir-split.md` | The worktree change introduces a third directory concept (the worktree path); we should reuse this ADR's vocabulary and consider whether worktree path is a third distinct directory or a redirected ProjectDir |
| Versioning: `config.json` schema is part of pr9k's public API | `docs/coding-standards/versioning.md` | Adding a top-level field is a backwards-compatible minor bump |

## Planned Fix (Sketch — full design lives in the feature spec)

This is a sketch. The actual feature specification will be produced by `/plan-a-feature` in `docs/plans/git-worktrees/feature-specification.md`. The sketch exists to anchor the validation step.

### Summary

When `useWorktrees: true` is set in `config.json`, pr9k creates a fresh git worktree off the primary checkout's HEAD before constructing the workflow runner, runs the entire workflow inside it, and tears it down on graceful completion (with a configurable keep-on-failure policy).

### Sketch of changes

#### `src/internal/workflowmodel/workflowmodel.go` (or wherever `WorkflowDoc` lives)
- **Change:** Add `UseWorktrees bool` (or a struct `Worktree { Enabled bool; Location string; KeepOnFailure bool }`) to `WorkflowDoc` JSON schema.
- **Evidence:** E11 — config loads from WorkflowDir before worktree exists.
- **Standards:** Versioning, narrow-reading principle.
- **Details:** Add validator rules in `internal/validator` (e.g., D-style numbered rule). Document in `docs/code-packages/workflowmodel.md` and `docs/code-packages/validator.md`.

#### `src/internal/worktree/` (new package)
- **Change:** New package with three operations: `Create(primaryDir, opts) (path, branch, error)`, `Cleanup(path) error`, `PruneStale(primaryDir) error`. Owns all `git worktree` shell-outs.
- **Evidence:** E1, E2, E5, E15.
- **Standards:** Error handling (package-prefixed, file paths), Go patterns (symlink resolution), testing (race-safe — but ideally stateless).
- **Details:** Use `os/exec`, surface stderr in errors, be deterministic about branch naming. Branch name format TBD by feature spec (candidate: `pr9k/<run-stamp>` so it's unique and traceable to the run).

#### `src/cmd/pr9k/main.go`
- **Change:** After CLI resolution and config load, if `useWorktrees` is true, create the worktree and substitute its path for `cfg.ProjectDir` before calling `startup()` and `NewRunner`. Register a cleanup hook that runs on graceful termination paths.
- **Evidence:** E7, E10, E11, E15.
- **Standards:** Concurrency (cleanup ordering with signal handling).
- **Details:** Worktree must exist before `preflight.Run` (which mkdirs `.pr9k/`). Cleanup must run before the process exits but after the workflow runner finishes — including on `q/y`, SIGINT, SIGTERM. Decision required: should logs live under the **primary** projectDir or the **worktree** projectDir? (See open question Q1.)

#### `workflow/scripts/git_push`
- **Change:** Stop swallowing push failures (or at least surface the exit code). Add `-u origin HEAD` so first-push works on a brand-new worktree branch.
- **Evidence:** E14.
- **Standards:** Error handling.
- **Details:** This fix is independently useful. It can ship with the worktree feature or separately.

#### Documentation
- **Change:** Add `docs/how-to/using-git-worktrees.md`, update `docs/architecture.md` (add worktree path to the directory model), update `CLAUDE.md` (note the third directory concept).
- **Evidence:** E11, ADR 20260413162428 (two-directory split must be expanded).
- **Standards:** `docs/coding-standards/documentation.md` — feature ships with docs.
- **Details:** Cross-link from the new ADR/feature spec.

#### Possibly: a new ADR
- **Change:** Add `docs/adr/<stamp>-worktree-isolation.md` recording the design decision (worktree location, branch naming, cleanup policy, log durability strategy).
- **Evidence:** Implicit — non-trivial design decision affecting the user-visible API.
- **Standards:** ADR template.
- **Details:** Record after the feature spec resolves the open questions.

## Key Open Questions for /plan-a-feature

These are the decisions the feature spec must resolve, not the investigation:

- **Q1 (most important): Where do logs live?** Three options:
  1. Worktree (`<worktree>/.pr9k/logs/`) — simple but logs vanish on cleanup unless we keep the worktree.
  2. Primary checkout (`<primary>/.pr9k/logs/`) — durable but requires splitting `logDir` apart from `projectDir`, which touches four call sites (E10).
  3. Hybrid: worktree by default, copy survivors out on cleanup.

- **Q2: Worktree location?** Sibling (`<projectDir>-pr9k-<stamp>`), inside `.git/` (`.git/pr9k-worktrees/<stamp>/`), or under pr9k's own dir (`~/.pr9k/worktrees/<repo-hash>/<stamp>/`). Recommend `.git/pr9k-worktrees/` — invisible to `git status`, lives with the repo, easy to clean.

- **Q3: Branch naming?** `pr9k/<run-stamp>`? `pr9k-wt-<run-stamp>`? Per-iteration branches? Note that the existing workflow already creates per-issue branches via the feature-work step; the worktree's "starting branch" is just a placeholder until the workflow's branch-creation steps run.

- **Q4: Cleanup policy?**
  - Always remove on success.
  - On failure: remove? keep for debugging? user-configurable via `keepOnFailure: true`?
  - Startup: prune stale on every run, or only with `--prune-worktrees`?

- **Q5: Should `useWorktrees` interact with `--project-dir`?** If the user passes `--project-dir /some/repo` AND `useWorktrees: true`, do we create the worktree off `/some/repo`? (Probably yes — it's the same logic, just a different starting projectDir.)

- **Q6: What about the `pr9k workflow` builder and `pr9k sandbox shell` subcommands?** Out of scope (E12) — confirm.

- **Q7: How do we handle the user being on a branch that's already checked out in another worktree (e.g., the user has done `git worktree add` themselves)?** Detect via `git worktree list --porcelain` and handle.

- **Q8: Should we offer per-iteration worktrees** (one per issue, parallelizable) **or per-run worktrees** (one for the whole pr9k run)? Per-run is the simpler MVP and matches the user's stated goal.

## Validation Results

(Validation in the next step.)

## Final Summary

- **What we found**: pr9k has a single architectural seam (`Runner.projectDir`) that makes worktree integration mostly mechanical (E7–E9), git worktrees themselves are well-suited to the use case (E1–E6), and `gh`/scripts work transparently inside them (E3, E13).
- **What needs deliberate design**: log durability (E10), cleanup lifecycle (E15), and the pre-existing silent push failure (E14).
- **What the feature spec must resolve**: where the worktree lives, where logs live, branch naming, cleanup policy, and crash-recovery behavior. Each is a small decision; together they define the feature surface.
- **What is NOT a problem**: WorkflowDir resolution timing (E11), Docker bind-mount mechanics (E8), subprocess CWD propagation (E9), statusline behavior (E16), or any individual workflow script's behavior (E13).
- **Remaining risks**: Filled in after adversarial validation.
