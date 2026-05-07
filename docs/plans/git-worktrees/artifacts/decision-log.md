# Decision Log: useWorktrees

This file records every decision settled while specifying the `useWorktrees` feature. Behavioral statements live in [../feature-specification.md](../feature-specification.md); this file captures the history, rationale, evidence, and rejected alternatives for each decision.

## Trivial decisions

- D10: Per-run worktree (one for the entire pr9k run, not per-iteration) — the default workflow does not switch branches per issue (validation finding V9 in `../investigation/04-validation.md`), so per-iteration worktrees have no use today. — Referenced in spec: Outcome, Out of Scope.
- D12: `--project-dir` interaction — when `--project-dir <path>` is passed and `useWorktrees: true` is set, the resolved `--project-dir` is treated as "the primary checkout" and the worktree is created off it. — Referenced in spec: Alternate Flows ("The user passes `--project-dir`...").

## Full decisions

### D1: Worktree location

- **Question:** Where on disk should the worktree live?
- **Decision:** `<primary>/.git/pr9k-worktrees/<run-stamp>/`. A single fixed location, not configurable in the first iteration.
- **Rationale:** Worktrees placed under `.git/` are invisible to `git status` (verified in `../investigation/01-worktree-mechanics.md` E4) and are guaranteed to be inside the same filesystem as the repository (no cross-device clone problems). They live with the repo so the user always knows where to look. A single fixed location avoids a config-schema growth that no user has yet justified.
- **Evidence:** `../investigation/01-worktree-mechanics.md` section 10 (location options tested), section E4 (git status noise comparison); investigation report E4. No ADR exists today on worktree placement.
- **Rejected alternatives:**
  - Sibling directory `<projectDir>-pr9k-<stamp>` — rejected because it requires knowing the parent directory and could collide with user-named directories.
  - User home `~/.pr9k/worktrees/<repo-hash>/<stamp>/` — rejected because it physically separates the worktree from the repo and surprises users who inspect worktrees with `git worktree list`.
  - Configurable location — rejected on YAGNI grounds; deferred until a user reports a real conflict.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D5 (stale worktree detection looks for entries under this path).
- **Referenced in spec:** Outcome, Primary Flow (step 3), Out of Scope.

### D2: Branch naming for the run

- **Question:** What branch should the worktree sit on?
- **Decision:** A brand-new branch named `pr9k/<run-stamp>` created off the primary's HEAD at worktree-creation time. The run-stamp is the millisecond-precision stamp pr9k already uses for its log directory (so the branch and the log dir are the same identifier).
- **Rationale:** Git enforces "one branch can be checked out in only one worktree at a time" (`../investigation/01-worktree-mechanics.md` E1), so the worktree must be on a different branch than whatever the primary has checked out. Creating a fresh branch off the primary's HEAD gives pr9k a clean starting point with no risk of conflict. The `pr9k/` prefix makes branches discoverable with `git branch --list 'pr9k/*'` and removable with the same pattern. The run-stamp is unique by construction.
- **Evidence:** `../investigation/01-worktree-mechanics.md` E1 (branch uniqueness), E2 (auto-create from HEAD); existing run-stamp convention in `docs/code-packages/logger.md`.
- **Rejected alternatives:**
  - Reuse the primary's current branch — rejected because git refuses; this is a hard constraint.
  - Detached HEAD — rejected because it makes pushing painful (`git push origin HEAD:refs/heads/<some-name>` is awkward) and the user can't easily check out the result later.
  - Per-iteration branches inside the worktree — rejected because the default workflow's feature-work prompt explicitly says "do not switch branches" (validation finding V9).
- **Linked technical notes:** [T2](feature-technical-notes.md#t2-branch-uniqueness)
- **Driven by findings:** —
- **Dependent decisions:** D5 (stale-worktree detection uses the `pr9k/` branch prefix), D7 (first push needs upstream because the branch is fresh).
- **Referenced in spec:** Outcome, Primary Flow (step 3), Edge Cases (detached HEAD, branch already exists), Out of Scope.

### D3: Log durability

- **Question:** Where do log files, the iteration log, and per-step transcripts live so they survive after the run?
- **Decision:** Logs and artifacts live inside the worktree's `.pr9k/logs/` exactly as they do today in the in-place model — but the worktree itself is left on disk after the run by default ([D4](#d4-cleanup-policy)), so the logs persist until the user removes the worktree.
- **Rationale:** The simplest version that satisfies the user-stated need ("monitor a parallel run, review logs after"). Splitting a separate `logDir` from `projectDir` would be a four-file change inside pr9k for no behavioral gain — the user's review workflow is the same either way. Validation finding V7 explicitly rules out "ephemeral logs that vanish on cleanup" as the default; keeping the worktree on disk avoids that failure mode without the split.
- **Evidence:** Validation finding V7 in `../investigation/04-validation.md`; the four log/artifact construction sites in `../investigation/02-projectdir-flow.md` E10 that would otherwise need a `logDir` parameter.
- **Rejected alternatives:**
  - Split `logDir` from `projectDir` so logs live in the primary checkout — rejected on simpler-version grounds; deferred to YAGNI with the trigger "auto-remove becomes the default."
  - Hybrid: copy logs out of the worktree on cleanup — rejected because cleanup is opt-in, not the default.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D4 (cleanup policy must keep the worktree because logs are inside it).
- **Referenced in spec:** Outcome, Edge Cases (logs and artifacts on completion).

### D4: Cleanup policy

- **Question:** What happens to the worktree on graceful run completion? On user-requested quit? On crash?
- **Decision:**
  - On graceful completion: pr9k leaves the worktree on disk and prints its path and the `git worktree remove` command in the final summary. The user removes it when ready.
  - On user-requested quit (`q`/`y`, SIGINT, SIGTERM): same as graceful completion — leave on disk, print recovery info.
  - On crash (SIGKILL, panic, power loss): the worktree is left on disk. The next run detects and warns about it ([D5](#d5-stale-worktree-handling)).
- **Rationale:** The simpler-version answer to the cleanup question. Keeping the worktree preserves logs and partial work for review. The user has full control via standard `git worktree` commands. No new flag, no new state to manage, no risk of pr9k accidentally deleting work.
- **Evidence:** Validation finding V7 (logs cannot be ephemeral); investigation open questions Q3 and Q4.
- **Rejected alternatives:**
  - Auto-remove on success, keep on failure — rejected on YAGNI grounds; no user has asked for it. Deferred with the reopening trigger documented in the spec.
  - Auto-remove always — rejected because it destroys logs (V7).
  - Configurable per-run via `worktreeKeep` — rejected on YAGNI grounds; the simpler "always keep" satisfies all known evidence.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D3 (logs live in the worktree because we keep it).
- **Referenced in spec:** Outcome, Primary Flow (step 9–10), Alternate Flows (graceful quit, SIGKILL), Deferred (YAGNI).

### D5: Stale worktree handling at startup

- **Question:** When a new run starts and finds existing `pr9k/...` worktrees from prior crashed runs, what does pr9k do?
- **Decision:** pr9k lists existing worktrees by reading `git worktree list --porcelain`, filters for entries whose path is under `<primary>/.git/pr9k-worktrees/` and whose branch starts with `pr9k/`, and prints a warning to the TUI and the log file naming each stale entry. pr9k does not auto-remove them. The new run proceeds normally and creates its own worktree.
- **Rationale:** Auto-removal is destructive. A stale worktree may contain commits the user hasn't yet pushed (e.g., the prior crash happened before the final push) or files the user wants to recover. Warning is non-destructive, gives the user information, and matches the unix philosophy of "don't take destructive action without consent." `git worktree prune` alone is not enough because it only removes entries whose directory was deleted out-of-band — the typical crash leaves the directory intact, so prune is a no-op (validation finding V12).
- **Evidence:** Validation finding V12 in `../investigation/04-validation.md`; `../investigation/01-worktree-mechanics.md` section 4 (cleanup paths and what `prune` covers).
- **Rejected alternatives:**
  - Auto-prune at startup — rejected because it destroys work without consent.
  - Require an explicit `--prune-worktrees` flag — rejected as unnecessary scope; the user can run `git worktree remove --force <path>` directly.
  - A new `pr9k worktree prune` subcommand — deferred to YAGNI.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Alternate Flows (stale worktree detected), Edge Cases.

### D6: Submodule policy

- **Question:** What happens when `useWorktrees: true` is enabled in a repository that uses git submodules?
- **Decision:** pr9k does not auto-initialize submodules in the new worktree. At startup, if pr9k detects a `.gitmodules` file in the primary checkout, it prints a warning that submodule-using projects may need manual initialization inside the worktree. The user is responsible.
- **Rationale:** pr9k itself has no submodules and no current user has reported a submodule-using project as a target. Auto-initializing submodules is a non-trivial dependency (git-submodule semantics, recursive init, network access for fetch) that adds risk without evidence of need. A warning is cheap and discoverable.
- **Evidence:** Validation finding V10; pr9k repository has no `.gitmodules`.
- **Rejected alternatives:**
  - Auto-run `git submodule update --init --recursive` after worktree creation — rejected on YAGNI grounds; deferred with a reopening trigger.
  - Refuse to run when `.gitmodules` exists — rejected because it would block users who have submodules but don't need them initialized for their workflow.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Preconditions, Edge Cases (repository contains submodules), Deferred (YAGNI).

### D7: First-push sets upstream

- **Question:** The push step in the default workflow runs `git push` (no upstream flag). A brand-new branch in a fresh worktree has no tracking ref, so `git push` fails. What does the spec commit to?
- **Decision:** The push step in the default workflow uses upstream-setting semantics on first push so that a brand-new run-branch is published to the remote and acquires a tracking ref. Subsequent pushes within the same run reuse the established upstream. The push step also surfaces non-zero exit codes rather than silently swallowing them.
- **Rationale:** Without upstream-setting, the first push of `pr9k/<run-stamp>` fails. Without surfacing the failure, pr9k closes the issue, prints "done," and the user is told everything succeeded — but no code is on the remote (validation finding V14 / E14). This is a pre-existing bug that becomes a much louder problem in the worktree path because every `useWorktrees` run starts on a new branch with no upstream.
- **Evidence:** `../investigation/03-behavioral-boundaries.md` (silent push failure); `workflow/scripts/git_push` source; validation finding V14.
- **Rejected alternatives:**
  - Set the upstream at worktree-creation time (e.g., `git push -u origin pr9k/<stamp>` immediately) — rejected because it pushes an empty commit before the workflow has done any work, which is wasted bandwidth and clutters the remote.
  - Leave the script unchanged and document the limitation — rejected because the silent-failure mode means the user can't tell when the workflow "succeeded" but no code landed.
- **Linked technical notes:** [T3](feature-technical-notes.md#t3-first-push-upstream)
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Edge Cases (first push), Coordinations (Git remote), Open Items (OI-1).

### D8: Default and config shape

- **Question:** How is `useWorktrees` exposed in `config.json`? Is there a CLI override?
- **Decision:** A single boolean field `useWorktrees` at the top level of `config.json`. Default is `false` (the current in-place behavior). No CLI flag override.
- **Rationale:** The user explicitly asked for a config-file toggle ("set a `useWorktrees` option at the top level of my workflow config.json"). A CLI flag duplicates the surface area and creates a precedence question (CLI vs. config). No user has asked for a CLI override. Default-false preserves the current behavior for everyone who hasn't opted in.
- **Evidence:** User's stated request in conversation; ADR `20260410170952-narrow-reading-principle.md` (workflow content lives in config.json, not Go code).
- **Rejected alternatives:**
  - A struct (e.g., `worktree: { enabled: true, location: ..., keep: true }`) — rejected on YAGNI grounds; defer the struct shape until a second config knob is needed.
  - A CLI flag `--worktrees` — rejected as duplicate surface area; defer until a real use case (e.g., one-off runs that override the config) appears.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Trigger, Edge Cases (default behavior unchanged).

### D9: Subcommand scope

- **Question:** Do `pr9k sandbox shell` and `pr9k workflow` (the workflow-builder TUI) honor `useWorktrees`?
- **Decision:** No. Both subcommands resolve their own working directory independently and ignore the flag. Only the workflow runner (`pr9k` or `pr9k workflow run`) honors `useWorktrees`.
- **Rationale:** `sandbox shell` is an interactive shell session against the user's chosen project; running it inside an automation worktree would be confusing. The workflow-builder edits the workflow config itself (which lives in the primary checkout's workflow bundle, not the worktree); editing it inside a worktree would mean the edits live in a per-run branch and never reach the bundle. Both behaviors would surprise users.
- **Evidence:** `../investigation/02-projectdir-flow.md` E12, E23 (independent project-dir resolution for these subcommands).
- **Rejected alternatives:**
  - Make all subcommands worktree-aware — rejected because none of them benefit and several are actively confusing.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Edge Cases (subcommand interaction), Coordinations.

### D11: Statusline payload shape

- **Question:** Should the statusline JSON payload gain a `primaryDir` field so user-authored statusline scripts can show both paths?
- **Decision:** Not in the first iteration. The existing `projectDir` field is repurposed to carry the worktree path while `useWorktrees` is true.
- **Rationale:** No user has asked for both paths. Adding a field is additive and backwards-compatible (existing scripts don't read it), but adding it preemptively is YAGNI. The single-path design is simpler and matches the existing semantics ("the directory where the workflow operates").
- **Evidence:** Validation finding V11; `src/internal/statusline/payload.go:46`.
- **Rejected alternatives:**
  - Add `primaryDir` now — rejected on YAGNI grounds; deferred with a reopening trigger.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Out of Scope, Deferred (YAGNI).
