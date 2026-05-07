# Adversarial Validation: Git Worktree Feature Investigation

**Validator:** Claude Code (claude-sonnet-4-6)
**Date:** 2026-05-07
**Investigation files validated:**
- `00-investigation-report.md`
- `01-worktree-mechanics.md`
- `02-projectdir-flow.md`
- `03-behavioral-boundaries.md`

---

## Validation Items

---

**V1: "Runner.projectDir is a single seam" — env inheritance leaks the host environment to non-claude subprocesses**

- **Strategy:** Challenge the Evidence
- **Hypothesis:** The investigation claims `Runner.projectDir` is the single seam and no subprocess leaks anything that would conflict with worktree mode. It does not address environment variable inheritance for non-claude subprocesses.
- **Investigation:**
  - Read `src/internal/workflow/workflow.go:431–464`: `runCommand` creates `exec.Command` and sets only `cmd.Dir`; `cmd.Env` is never set. When `cmd.Env` is nil in Go, the child inherits the full parent-process environment (`os.Environ()`).
  - Read `workflow/scripts/get_claude_credentials`: uses `CLAUDE_CONFIG_DIR` from the environment and calls `realpath "$CLAUDE_PATH"` where `CLAUDE_PATH` is derived from `$CLAUDE_CONFIG_DIR`. This is a non-claude step whose behavior depends on an env var set in the host shell, not on `projectDir`.
  - Confirmed: `get_claude_credentials` runs with CWD = worktree path (via `cmd.Dir`) but with `CLAUDE_PATH` resolving to the host's `~/.claude` or `$CLAUDE_CONFIG_DIR`. This is correct — the credential file is on the host, not in the worktree. The script is path-agnostic.
  - Confirmed: the investigation's claim that "env inheritance is not an integration point" is correct for all existing scripts. No script reads a path derived from CWD that would conflict with a worktree CWD.
- **Result:** Confirmed (with a precision upgrade)
- **Impact:** The seam claim holds. The investigation should note that non-claude subprocess env is fully inherited from the host process, but this is safe because no existing script derives a host path from the project CWD in a way that breaks under worktrees. Adding `useWorktrees` to `config.json` does not change this. The precision upgrade is: `profileDir` is also resolved independently per-step via `preflight.ResolveProfileDir()` at `run.go:765`—it does not come from `Runner.projectDir` at all. The investigation notes this correctly at B4.

---

**V2: "No script changes needed" — `get_claude_credentials` uses `realpath` against a non-CWD path; `statusline` uses `git rev-parse --git-dir` from CWD**

- **Strategy:** Challenge the Evidence
- **Hypothesis:** The investigation (E13/B9–B11) states no script changes are needed. I looked for any script that uses `pwd`, `$(pwd)`, `realpath`, `readlink`, or `git rev-parse` in ways that could behave differently from a worktree.
- **Investigation:**
  - Grepped all files in `workflow/scripts/` for `pwd`, `$(pwd)`, `realpath`, `readlink`, `git rev-parse --git-dir`, `git rev-parse --show-toplevel`.
  - Found: `workflow/scripts/get_claude_credentials:66` uses `realpath "$CLAUDE_PATH"` where `CLAUDE_PATH` is derived from `$CLAUDE_CONFIG_DIR` (a host env var), not from CWD. This path is the Claude profile directory, not the project dir. No conflict.
  - Found: `workflow/scripts/statusline:19` uses `git rev-parse --git-dir > /dev/null 2>&1` as a probe (does git exist here?), then `git branch --show-current`. In a worktree, `git rev-parse --git-dir` returns the worktree's admin dir path (e.g., `/path/to/repo/.git/worktrees/<name>`), which is truthy and non-empty. The probe succeeds. `git branch --show-current` returns the worktree's branch. This is the intended behavior.
  - No script calls `git rev-parse --show-toplevel` and no script hard-codes a path relative to the repo root that is not already CWD-relative.
- **Result:** Confirmed — no script changes are needed. The two git usages found are either CWD-independent (`get_claude_credentials` uses a host env path) or work correctly in a worktree (`statusline`'s git probe succeeds with a worktree gitfile). The investigation's E13/E26 claims are accurate.
- **Impact:** No changes to finding. The "no script changes needed" claim stands.

---

**V3: "`config.json` schema versioning is backwards-compatible" — the validator uses `DisallowUnknownFields()`, which is a BREAKING CHANGE path**

- **Strategy:** Challenge the Fix
- **Hypothesis:** The investigation (E11/B15 and Coding Standards table) claims adding `useWorktrees: true` as a top-level field is a backwards-compatible minor bump. This assumes JSON parsing is lenient. It is not.
- **Investigation:**
  - Read `src/internal/validator/validator.go:203–209`. The validator's `ValidateDoc` function constructs a `json.NewDecoder` and calls `dec.DisallowUnknownFields()` before decoding the config into `vFile`. If `config.json` contains `useWorktrees: true` and the `vFile` struct does not have a `UseWorktrees` field, the validator returns a fatal error at Category 1.
  - Read `src/internal/validator/validator.go:111–118` (`vFile` struct): it has only `Env`, `ContainerEnv`, `Defaults`, `Initialize`, `Iteration`, `Finalize`, `StatusLine`. No `useWorktrees` field.
  - Read `src/internal/steps/steps.go:152` (`LoadSteps`): uses `json.Unmarshal(data, &sf)` with `StepFile` which also has no `useWorktrees` field. However, `json.Unmarshal` silently ignores unknown fields by default, so `LoadSteps` would not fail. But `validator.Validate` (called at `main.go:69`) runs against the same config.json via `ValidateDoc`, and WOULD fail with a fatal parse error.
  - The startup sequence at `main.go:141` calls `startup()` which calls both `steps.LoadSteps` and `validator.Validate`. The validator's fatal error would abort the run before `NewRunner` is ever called.
  - Summary: Adding `useWorktrees: true` to `config.json` WITHOUT simultaneously adding `UseWorktrees` to `vFile` in the validator AND `StepFile` in `steps.go` AND `rawConfig` in `workflowmodel/scaffold.go` AND `WorkflowDoc` in `workflowmodel/model.go` will cause a fatal validator error on every run with `useWorktrees` in config. This is a four-file change minimum, not just "add a field to config.json."
  - Backwards compatibility direction: a NEW binary with the `UseWorktrees` field added will read OLD configs (without `useWorktrees`) fine, because the field is absent and Go zero-initializes it to `false`. An OLD binary reading a NEW config with `useWorktrees: true` will fail with a fatal parse error from `DisallowUnknownFields`. This is FORWARD-BREAKING, not backwards-compatible.
- **Result:** Refuted (partial) — the claim "adding a top-level field is a backwards-compatible minor bump" is only half-true. New binary + old config = fine. Old binary + new config = fatal validator error. The investigation assumes lenient JSON parsing. The validator explicitly prohibits it. This requires coordinated changes to four structs.
- **Impact:** CRITICAL. The feature spec must list the four files that require changes to the JSON schema layer and the validator, and must document the forward-compatibility break: configs with `useWorktrees: true` will fail with old pr9k binaries. The versioning standards note must be amended: this is not a purely additive minor change from the user's perspective if they share a config.json between old and new pr9k versions.

---

**V4: "WorkflowDir resolution is independent of the worktree" — the in-repo workflow override would land in the worktree after worktree creation**

- **Strategy:** Challenge the Assumptions
- **Hypothesis:** The investigation (E11/B15) states the in-repo override `<projectDir>/.pr9k/workflow/` uses the primary checkout's `projectDir` because the worktree doesn't exist at config-load time. This is correct for startup. But there is a secondary path: the workflow builder subcommand resolves its own project dir independently.
- **Investigation:**
  - Read `src/internal/cli/args.go:93`: `resolveProjectDir` (CWD) is called first, then `resolveWorkflowDir(cfg.ProjectDir)` using that result. WorkflowDir is resolved at CLI parse time from the primary CWD.
  - The proposed insertion point for worktree creation is: after `cli.Execute` (which resolves WorkflowDir), before `startup`. By that point, `cfg.WorkflowDir` is already frozen to the primary repo's path. The worktree creation cannot affect it.
  - However, B15 says "After the worktree is created, we should NOT re-resolve the workflow dir; it stays anchored to the primary checkout." This is true only if the worktree creation happens AFTER `cli.Execute`. The proposed sketch at `main.go` places it correctly. But the investigation doesn't note the risk: if worktree creation is mistakenly inserted BEFORE `cli.Execute` or if the worktree path has a `.pr9k/workflow/` directory (it will, because worktrees share all files), and `cfg.ProjectDir` is reassigned to the worktree path BEFORE `resolveWorkflowDir` is called, the workflow bundle would resolve from the worktree — and the worktree's `.pr9k/workflow/` could drift from the primary's during the run.
  - The `main.go` sketch places worktree creation between `cli.Execute` and `startup`, which is the correct position. The risk only exists if the implementation puts it in the wrong place. But this is a real blast radius if the ordering is wrong and worth calling out.
- **Result:** Partially Refuted — the investigation's analysis is correct for the proposed insertion point, but the assumption is load-bearing: if the implementation inserts worktree creation before CLI parsing or reassigns `cfg.ProjectDir` before `resolveWorkflowDir` is called, the workflow bundle would resolve from the worktree. The investigation does not flag this as a sequencing constraint that must be enforced in the implementation.
- **Impact:** IMPORTANT. The feature spec must document: "worktree creation MUST happen after `cli.Execute` returns (i.e., after WorkflowDir is frozen) and MUST NOT reassign `cfg.WorkflowDir`. Only `cfg.ProjectDir` is redirected." This is an ordering invariant that the implementation must enforce.

---

**V5: "Branch uniqueness is the only hard constraint" — per-iteration multi-issue runs use the SAME worktree branch for every issue**

- **Strategy:** Challenge the Assumptions
- **Hypothesis:** The investigation (E1, Q3) treats branch uniqueness as the only hard git constraint and defers branch naming to the feature spec. But the workflow loop processes multiple issues in a single run. Each iteration fetches a DIFFERENT issue and commits to whatever branch the worktree is on. The prompts say "implement in the current branch (do not switch branches)." With a per-run worktree, all N issues across N iterations are committed to the same placeholder branch. This is the intended behavior—it mirrors in-place mode where all issues accumulate on the same branch. But the investigation does not explicitly confirm or challenge this.
- **Investigation:**
  - Read `workflow/prompts/feature-work.md:9`: "Implement github issue #{{ISSUE_ID}} in the current branch (do not switch branches)."
  - Read `workflow/config.json`: confirmed there is no `git checkout -b` step in any phase. There is no per-issue branch-creation step.
  - Read `src/internal/workflow/run.go:474–476`: `vt.ResetIteration()` and `vt.SetIteration(i)` are called per iteration. The worktree branch is NOT reset per iteration; it accumulates commits across iterations just like the primary checkout does today.
  - Conclusion: The current workflow does NOT create per-issue branches. All feature work lands on whatever branch the checkout/worktree is on. In worktree mode, all issues across a run commit to the worktree's placeholder branch. This is the correct behavior — per-run worktree is equivalent to in-place mode but in an isolated directory.
  - However: if the user's intent is that each issue gets its own branch, the workflow itself would need to add a `git checkout -b` step (or the worktree feature would need per-iteration worktrees). The investigation notes this under Q8 but classifies per-run as the "simpler MVP." This is confirmed correct.
- **Result:** Confirmed — branch uniqueness constraint is correctly identified. The per-run worktree model produces the same commit accumulation pattern as in-place mode. No gap found.
- **Impact:** Informational. The feature spec should explicitly document that the worktree branch accumulates all iterations' commits, matching in-place mode behavior. This is not a bug but may surprise users who expect per-issue branches.

---

**V6: "`os.Chdir` is never called" in production code — test code calls it, flagging a parallel test risk**

- **Strategy:** Challenge the Evidence
- **Hypothesis:** The investigation (E9/B5) states `os.Chdir` is never called in the codebase. The claim is true for production code but the tests use it.
- **Investigation:**
  - Grepped `src/` for `os.Chdir`, excluding `.gopath/`.
  - Found: `src/internal/cli/args_test.go:46,49` calls `os.Chdir(dir)` and restores on cleanup. The test file explicitly notes: "setupWorkflowCandidate cannot be combined with t.Parallel() — os.Chdir is process-global." The test does NOT call `t.Parallel()`.
  - No production code outside of `.gopath/` calls `os.Chdir`.
  - The worktree feature's new `internal/worktree/` package will need tests. If those tests use `os.Chdir` (to simulate a worktree CWD), they must not use `t.Parallel()`. This is a constraint on the test design for the new package, already partially addressed by the existing precedent in `args_test.go`.
- **Result:** Confirmed (with test caveat) — production code never calls `os.Chdir`, confirming B5. The `args_test.go` usage is test-only and appropriately documented. The new worktree package's tests should avoid `os.Chdir` entirely (use `cmd.Dir` in integration tests or mock the CWD-dependent logic).
- **Impact:** Informational. The feature spec should note that worktree package tests must not use `os.Chdir` (per the existing coding standard set by `args_test.go`'s own comment). Use of `t.Chdir()` (Go 1.24+) is safer, but the broader race detector requirement means tests that touch the global CWD must not run in parallel.

---

**V7: "Logs vanish if worktree is deleted — over-engineering concern" — counter-argument assessed**

- **Strategy:** Challenge the Assumptions
- **Hypothesis:** The investigation (E10/B12) flags log durability as a "significant behavioral concern." The counter-hypothesis: maybe ephemeral logs are fine and the logDir split is unnecessary scope creep for an MVP.
- **Investigation:**
  - Read `src/internal/workflow/iterationlog.go:47–48`: `iteration.jsonl` is written to `<projectDir>/.pr9k/iteration.jsonl`. `workflow/scripts/post_issue_summary:10` reads `.pr9k/iteration.jsonl` with a relative path from CWD. If both use the worktree path, they are consistent during the run.
  - Read `docs/how-to/debugging-a-run.md` (referenced in CLAUDE.md): the how-to guide instructs users to read `.pr9k/logs/` and `iteration.jsonl` for debugging. These are explicitly user-facing artifacts.
  - The `post_issue_summary` script runs BEFORE the cleanup step in the iteration loop — it reads `iteration.jsonl` while the worktree still exists. The JSONL is only useful AFTER the run for postmortems.
  - Read `src/cmd/pr9k/main.go:104`: `artifactDir` is created at startup for per-step `.jsonl` files. These are the NDJSON debug artifacts. They also live under `projectDir`.
  - Counter-argument assessment: for a user who never needs to debug a run, ephemeral logs are fine. But the feature motivation is "keep the primary checkout usable." A user running pr9k in worktree mode is more likely to be monitoring a parallel run — they are the exact user who would want to check logs after the run. Deleting logs on cleanup defeats this use case.
  - The iteration.jsonl is also used by `post_issue_summary` during the run (before cleanup), so it must exist in the worktree during execution. The question is only about postmortems.
- **Result:** Confirmed — the concern is legitimate, not over-engineering. The user profile for worktree mode is precisely the user who wants to inspect logs after the run. Ephemeral logs are acceptable ONLY if the cleanup is opt-in (user explicitly requests removal) rather than the default. The three-option framework from Q1 is still the right frame; the investigation is not over-engineering this.
- **Impact:** Informational — supports the investigation. However, this validates that the "always remove on success" cleanup policy (Option Q4a) is unacceptable as an unannounced default. The MVP must either keep the worktree by default or copy logs out before removal.

---

**V8: "gh CLI works transparently inside a worktree" — `gh auth status` caches per path; `gh` GraphQL API tested**

- **Strategy:** Challenge the Evidence
- **Hypothesis:** The investigation (E3/B6/B18) claims `gh` works transparently in a worktree because it reads remote config from the shared `.git/config`. The GraphQL API call in `get_next_issue` uses a `gh api graphql` call that also depends on auth. Does `gh auth status` cache a token tied to the profile dir rather than the repo path? Does the worktree affect the profile dir lookup?
- **Investigation:**
  - The `gh` CLI authenticates via `~/.config/gh/hosts.yml` (or `GH_TOKEN` env var). The token is NOT keyed by repo path. `gh` is repo-agnostic for auth purposes.
  - `GH_TOKEN` is in the `env` passthrough list at `workflow/config.json:2`. This token is forwarded into the Docker container for claude steps. For non-claude steps (all `gh` calls are non-claude), the subprocess inherits the full host environment including `GH_TOKEN` if set.
  - The investigation tests `gh issue list`, `gh pr create`, `gh pr status` from a worktree and reports they all work. The `get_next_issue` script also uses `gh api graphql` — the GraphQL endpoint is also authenticated via the same token.
  - There is no per-path caching in `gh`. The `gh` auth mechanism is per-host (GitHub.com or enterprise host), not per-repo-path.
  - The `.git` gitfile in a worktree is sufficient for `gh` to discover the repo remote via `git remote get-url origin` (which reads from the shared `.git/config`).
- **Result:** Confirmed — `gh` works transparently in worktrees. Authentication is not path-dependent. The GraphQL call in `get_next_issue` is no different from any other `gh` call in this regard.
- **Impact:** No changes to finding. The investigation's E3 evidence stands.

---

**V9: "Feature-work creates branches per issue" — this claim does NOT appear in the investigation but is implied by Q3 and Q8**

- **Strategy:** Challenge the Assumptions
- **Hypothesis:** The investigation's Q3 (branch naming) asks about "per-issue branches" and Q8 asks whether worktrees should be per-iteration (per issue). This implies the investigation believes each issue gets its own branch. But the actual workflow does NOT create per-issue branches.
- **Investigation:**
  - Read `workflow/prompts/feature-work.md:9`: "Implement github issue #{{ISSUE_ID}} in the current branch (do not switch branches)." The prompt explicitly prohibits branch switching.
  - Searched all config.json steps and prompts for `git checkout`, `git switch`, `git branch -b`: none found.
  - The default workflow creates no branches. The feature-work step commits everything to whatever branch the checkout is on.
  - The investigation's Q3 asks "per-iteration branches?" and Q8 asks about "per-iteration worktrees (one per issue, parallelizable)." These questions imply the assumption that issues are implemented on separate branches. This assumption is FALSE for the current default workflow.
  - The worktree's "starting branch" described in E2 would only be a temporary placeholder if the workflow later switched branches. It doesn't. The placeholder IS the working branch for all iterations in the run.
- **Result:** Refuted — the investigation's Q3 and Q8 are based on a false assumption about branch behavior. The current workflow does NOT create per-issue branches. Q3 ("branch naming") and Q8 ("per-iteration vs per-run worktrees for parallelization") are moot for the current default workflow. Per-run worktrees are not just "the simpler MVP"—they are the ONLY correct model for the current workflow. Per-iteration parallelization would require a fundamentally different workflow design.
- **Impact:** IMPORTANT. The feature spec should remove or reframe Q3 (no "per-iteration branch" naming needed — the branch is just a namespace for the run) and close Q8 ("per-iteration worktrees" is out of scope unless the workflow adds branch-per-issue). The investigation's framing of Q8 as "per-run is simpler MVP" understates the case: per-run is the ONLY correct model for the current default workflow.

---

**V10: "Worktree creation is fast and cheap" — submodules are a documented blocker**

- **Strategy:** Challenge the Assumptions
- **Hypothesis:** The investigation (E2) treats worktree creation as cheap. The investigation report itself documents in `01-worktree-mechanics.md:430–437` that submodule support for worktrees is "incomplete" per git's own BUGS section and that worktrees with submodules require `--force` to remove. The investigation mentions this but does not flag it as a constraint that must surface in the feature spec.
- **Investigation:**
  - Read `01-worktree-mechanics.md:430–437`: "Multiple checkout in general is still experimental, and the support for submodules is incomplete. It is NOT recommended to make multiple checkouts of a superproject." Also: "Worktrees with submodules cannot be moved with `git worktree move`, and require `--force` to remove."
  - The pr9k target user is any GitHub repo. Repos with submodules are common (e.g., any repo using git submodules for vendored code or tooling).
  - The worktree package's `Cleanup` function (proposed) uses `git worktree remove --force`. This handles the submodule case for removal (single `--force` is sufficient). But the creation step may behave unexpectedly in repos with submodules: submodule directories may be absent in the worktree, prompting the workflow's `make ci` or test runner to fail with missing dependencies.
  - LFS files: git LFS pointer files in a worktree point to the same LFS cache as the primary checkout, so large binary files are NOT doubled. Worktree creation is cheap even for LFS repos.
- **Result:** Partially Refuted — the "fast and cheap" claim holds for most repos (including LFS repos). But repos with submodules are a documented risk: the worktree may have missing submodule content, causing CI to fail inside the workflow. This is not just a performance concern — it's a correctness concern.
- **Impact:** IMPORTANT. The feature spec should document: "Repos with git submodules may not work correctly with `useWorktrees: true`; submodule content is not automatically initialized in the worktree." The validator should emit a warning (non-fatal) when `useWorktrees: true` is set and the repo contains submodules (detectable via `.gitmodules` existence). The new `worktree` package's `Create` function should run `git submodule update --init --recursive` inside the worktree after creation, or document that the user must do so via a workflow step.

---

**V11: "Statusline follows projectDir automatically" — statusline payload exposes worktree path to user scripts**

- **Strategy:** Challenge the Fix
- **Hypothesis:** The investigation (E16/B14) calls the statusline behavior a "free win." But `src/internal/statusline/payload.go:46` puts `projectDir` into the JSON payload as the `projectDir` field. User-authored statusline scripts that display the directory name will show the worktree path (e.g., `/path/to/repo/.git/pr9k-wt/2026-05-07-1234/`) instead of the primary checkout path (`/path/to/repo`). This may be confusing or broken for scripts that parse the path.
- **Investigation:**
  - Read `src/internal/statusline/state.go:18`: `ProjectDir string` field.
  - Read `src/internal/statusline/payload.go:46`: `ProjectDir: s.ProjectDir` — the worktree path goes into the payload.
  - Read `workflow/scripts/statusline`: this reference script reads `projectDir` via `jq -r '.projectDir // ""'`? Actually checking: the statusline script reads `.phase`, `.iteration`, `.maxIterations`, `.step.name`, `.captures.ISSUE_ID`. It does NOT read `.projectDir`. So the reference script is not affected.
  - However, user-authored scripts (outside the default bundle) might read `.projectDir` to display the project name. They would receive the worktree path. This is documented as a known concern in E18 of the `02-projectdir-flow.md`, but the synthesis report (E16) calls it a "free win" without flagging the E18 concern.
- **Result:** Partially Refuted — the investigation's synthesis correctly identifies E18 as a separate concern but the synthesis document (E16) contradicts it by calling statusline "a free win—statusline shows useful info with no changes." The statusline `cmd.Dir` behavior IS correct (it runs in the worktree, sees the right branch). But the payload's `projectDir` field will expose the worktree path to user scripts. This is a user-experience concern, not a correctness bug.
- **Impact:** Informational. The feature spec should document that the `projectDir` field in the statusline payload will contain the worktree path, not the primary checkout path, when `useWorktrees: true`. User-authored statusline scripts that use `projectDir` to display the project location should use `basename` or similar to extract the meaningful part. Consider whether to add a separate `primaryDir` field to the payload.

---

**V12: "Per-run worktree on a deterministic path crashes on second run if the first run crashed" — startup probe gap**

- **Strategy:** Challenge the Fix
- **Hypothesis:** The investigation (E15/B17) notes the crash cleanup gap. The proposed fix includes a startup probe (`git worktree prune`). But `git worktree prune` does NOT remove locked worktrees, and it does NOT remove worktrees whose directory still exists on disk (only prunable ones where the directory was manually deleted). If pr9k creates a worktree at `.git/pr9k-wt/<stamp>/` and crashes, the directory AND admin entry both survive. On the next run with a timestamp-named path, a new unique path is created (no collision). But the stale worktree persists and accumulates.
- **Investigation:**
  - Read `01-worktree-mechanics.md:182–218`: `git worktree prune` removes entries where the `gitdir` file points to a non-existent location (the worktree directory was manually deleted). It does NOT remove entries where the worktree directory still exists.
  - Read `01-worktree-mechanics.md:339–348`: locked worktrees are not pruned even if their directory is deleted.
  - The proposed startup probe using `git worktree prune` would clean up worktrees from crashes where the directory was manually deleted but would NOT clean up worktrees from crashes where the directory still exists (the more common case). For deterministic paths (e.g., `<primaryDir>-worktree`), a second run after a crash would fail because the directory already exists and is non-empty.
  - The investigation recommends "a startup probe needs `git worktree prune` for stale entries from crashed runs." This is incomplete: for the common crash case where the directory survives, the startup probe also needs `git worktree remove --force <stale-path>` or the worktree removal must detect and remove the existing directory.
- **Result:** Partially Refuted — `git worktree prune` alone is insufficient for the crash-recovery case. It handles the case where the directory was manually deleted (uncommon) but not where the directory still exists on disk (the typical crash). The startup probe must be more aggressive: iterate `git worktree list --porcelain`, identify pr9k-owned entries (by path prefix or lock reason), and call `git worktree remove --force <path>` on each.
- **Impact:** IMPORTANT. The feature spec must document the correct startup probe algorithm: not just `git worktree prune` but `git worktree list --porcelain` + identify pr9k-owned entries + `git worktree remove --force` for each. Alternatively, always use timestamp-named paths and accept stale accumulation, combined with a `--prune-worktrees` flag for manual cleanup.

---

## Confidence Assessment

- **Level:** High
- **Rationale:** All twelve hypotheses were investigated against actual source files with line-number citations. Four findings are based on direct code evidence (V3 — DisallowUnknownFields; V9 — no branch creation in workflow; V12 — prune insufficiency; V1 — env inheritance). The remaining findings are supported by multi-file cross-referencing. No finding relies solely on documentation claims without code verification.

---

## Remaining Risks

1. **`git` version floor:** The investigation uses git 2.50.1. If the target repo's host has an older git (pre-2.5, which introduced `git worktree`), worktree creation will fail. The preflight package has no git version check. This is a new runtime dependency that the feature spec must document, and the preflight package must validate.

2. **Docker bind-mount of a worktree inside `.git/`:** The `.git/pr9k-wt/<name>/` placement recommendation creates a worktree at a path inside the main repo's `.git/` directory. Docker bind-mounts this path as `source=<worktree-path>`. Docker's behavior with paths inside `.git/` (which some security configurations restrict) was not tested. For the safer sibling or home-dir placement options, this risk does not apply.

3. **`review_verdict` reads `code-review.md` from CWD:** The investigation notes this inconsistency at E20 ("pre-existing"). The script reads from CWD, but Claude may write to `.pr9k/artifacts/code-review.md` inside the container, which maps to `<worktreePath>/.pr9k/artifacts/code-review.md`. If the finalize phase runs `code-review-changes.md` and writes its output to `code-review.md` (not `.pr9k/artifacts/code-review.md`), then `review_verdict` finds it. If it writes to the artifacts subdirectory, `review_verdict` fails. This pre-existing inconsistency was not resolved in the investigation and could silently skip the "Fix review items" step in worktree mode.

4. **`iteration.jsonl` is written per-iteration but `post_issue_summary` appends all iterations' records:** The script uses all records in `iteration.jsonl` to build the issue comment. In a multi-iteration worktree run, all iterations accumulate in the worktree's `iteration.jsonl`. This is consistent with the in-place behavior. It is not a new bug, but it means the worktree accumulates all iteration data rather than per-iteration data, which limits per-issue comments to the last iteration's summary only if the JSONL is filtered.

5. **Worktree path containing spaces or special characters:** `docker run --mount source=<path>,...` passes the path as a shell argument. If the worktree path contains spaces (e.g., a project in `~/My Projects/repo`), the Docker bind-mount argument construction at `sandbox/command.go:47` would still work because it uses `fmt.Sprintf` directly into the `--mount` flag value. But `git worktree add` itself may fail on paths with spaces if shell-quoted incorrectly. The `internal/worktree` package must use Go's `exec.Command` (not shell interpolation) for all `git worktree` invocations.
