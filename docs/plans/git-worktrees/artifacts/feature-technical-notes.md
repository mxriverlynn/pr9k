# Feature Technical Notes: useWorktrees

This file captures load-bearing implementation mechanics whose behavior the [feature specification](../feature-specification.md) relies on. Each note links from the specific spec sentence whose correctness depends on the mechanic.

## T1: Sequencing constraint — workflow bundle resolves before the worktree exists

- **Title:** Workflow bundle resolution must precede worktree creation, and worktree creation must precede project-directory redirect, runtime-directory creation, and runner construction.
- **Context:** [Primary Flow](../feature-specification.md#primary-flow) step 2 ("pr9k parses CLI flags and resolves the workflow bundle from the primary checkout, then loads `config.json`...") and the [Coordinations](../feature-specification.md#coordinations) row for Git (local).
- **Technical detail:** pr9k resolves the workflow bundle directory (which contains `config.json`, `prompts/`, and `scripts/`) from the primary checkout before the worktree exists. If the resolution were deferred until after the worktree was created, pr9k could pick up an in-repo workflow override from inside the worktree — and the worktree's contents start as a copy of the primary's HEAD, which the workflow itself is about to mutate mid-run. Anchoring the workflow bundle to the primary checkout prevents the workflow from rewriting itself out from under the runner. The required ordering is: CLI flag parse → workflow-bundle resolution → `config.json` load → worktree create → project-directory redirect → runtime-directory creation → runner construction. The pr9k process never calls a process-global change-of-directory; the redirect is a single-field substitution in the runtime config consumed by every downstream subsystem.
- **Supports decisions:** [D1](decision-log.md#d1-worktree-location), [D8](decision-log.md#d8-default-and-config-shape)
- **Driven by findings:** —
- **Referenced in spec:** Primary Flow (step 2), Coordinations (Git local).

## T2: Branch uniqueness — git refuses to check out a branch in two worktrees

- **Title:** A branch may only be checked out in one worktree at a time, so the run's worktree must sit on a branch the primary doesn't currently have.
- **Context:** [Primary Flow](../feature-specification.md#primary-flow) step 3 (the worktree is checked out on a brand-new branch named `pr9k/<run-stamp>` that branches off the primary's current HEAD).
- **Technical detail:** Git enforces branch uniqueness across worktrees: an attempt to check out the primary's current branch in a second worktree fails with `fatal: '<branch>' is already used by worktree at '<path>'`. This is the reason the spec requires a fresh branch per run rather than reusing the primary's branch. Creating a new branch off HEAD with a unique name (`pr9k/<run-stamp>`) sidesteps the rule entirely: the new branch is uniquely the worktree's, so the primary's branch is unaffected. The primary's HEAD does not move. The `pr9k/` prefix is also load-bearing because the stale-worktree detection at startup ([D5](decision-log.md#d5-stale-worktree-handling)) filters by that prefix when listing worktrees from `git worktree list --porcelain`.
- **Supports decisions:** [D2](decision-log.md#d2-branch-naming-for-the-run), [D5](decision-log.md#d5-stale-worktree-handling)
- **Driven by findings:** —
- **Referenced in spec:** Primary Flow (step 3), Edge Cases (detached HEAD, branch already exists).

## T3: First-push upstream — a fresh worktree branch has no remote tracking ref

- **Title:** The first push of the run's branch must establish a remote tracking ref so the push succeeds and subsequent pushes within the run can use bare `git push`.
- **Context:** [Edge Cases](../feature-specification.md#edge-cases-and-failure-modes) ("The first `git push` of the run's branch happens"), [Coordinations](../feature-specification.md#coordinations) ("Git (remote) — first push of the run's branch").
- **Technical detail:** A branch created locally has no upstream. Bare `git push` from such a branch fails because git doesn't know which remote ref to push to. The push step in the default workflow either (a) sets the upstream on first push so that a tracking ref is established and the push succeeds in one step, or (b) refuses to silently swallow non-zero exit codes when the push fails. The current `git_push` script does neither — it runs bare `git push` and traps all exits to zero, so the failure mode is invisible. This T# captures the constraint; the spec commits to "first push establishes a tracking ref AND surfaces non-zero exits" without prescribing a specific mechanism. The implementation plan picks the exact form (a flag added to the script, a pre-push tracking-ref check inside the script, or a wrapper in the runner). Once the upstream is set, the run's subsequent pushes (after later iterations) reuse the established tracking ref unchanged.
- **Supports decisions:** [D7](decision-log.md#d7-first-push-sets-upstream)
- **Driven by findings:** —
- **Referenced in spec:** Edge Cases (first push), Coordinations (Git remote — first push), Open Items (OI-1).
