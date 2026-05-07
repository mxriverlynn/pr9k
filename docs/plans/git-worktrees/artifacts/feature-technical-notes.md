# Feature Technical Notes: useWorktrees

This file captures load-bearing implementation mechanics whose behavior the [feature specification](../feature-specification.md) relies on. Each note links from the specific spec sentence whose correctness depends on the mechanic.

## T1: Sequencing constraint — workflow bundle resolves before the worktree exists

- **Title:** Workflow bundle resolution must precede worktree creation, and worktree creation must precede project-directory redirect, runtime-directory creation, and runner construction.
- **Context:** [Primary Flow](../feature-specification.md#primary-flow) step 2 ("pr9k parses CLI flags and resolves the workflow bundle from the primary checkout, then loads `config.json`...") and the [Coordinations](../feature-specification.md#coordinations) row for Git (local).
- **Technical detail:** pr9k resolves the workflow bundle directory (which contains `config.json`, `prompts/`, and `scripts/`) from the primary checkout before the worktree exists. If the resolution were deferred until after the worktree was created, pr9k could pick up an in-repo workflow override from inside the worktree — and the worktree's contents start as a copy of the primary's HEAD, which the workflow itself is about to mutate mid-run. Anchoring the workflow bundle to the primary checkout prevents the workflow from rewriting itself out from under the runner. The required ordering is: CLI flag parse → workflow-bundle resolution → `config.json` load → worktree create → project-directory redirect → runtime-directory creation → runner construction. The pr9k process never calls a process-global change-of-directory; the redirect is a single-field substitution in the runtime config consumed by every downstream subsystem.
- **Supports decisions:** [D1](decision-log.md#d1-worktree-location), [D8](decision-log.md#d8-default-and-config-shape)
- **Driven by findings:** —
- **Referenced in spec:** Primary Flow (step 2), Primary Flow (step 4), Coordinations (Git local).

## T2: Branch uniqueness — git refuses to check out a branch in two worktrees

- **Title:** A branch may only be checked out in one worktree at a time, so the run's worktree must sit on a branch the primary doesn't currently have.
- **Context:** [Primary Flow](../feature-specification.md#primary-flow) step 4 (the worktree is checked out on a brand-new branch named `pr9k-<worktree-stamp>` that branches off the primary's current HEAD).
- **Technical detail:** Git enforces branch uniqueness across worktrees: an attempt to check out the primary's current branch in a second worktree fails with `fatal: '<branch>' is already used by worktree at '<path>'`. This is the reason the spec requires a fresh branch per run rather than reusing the primary's branch. Creating a new branch off HEAD with a unique name (`pr9k-<worktree-stamp>`) sidesteps the rule entirely: the new branch is uniquely the worktree's, so the primary's branch is unaffected. The primary's HEAD does not move. The hyphen separator (rather than a slash) is the spec's chosen convention — see review F1. The `pr9k-` prefix is a discoverability convention only; stale-worktree detection at startup ([D5](decision-log.md#d5-stale-worktree-handling)) filters by **directory-name** pattern (`<primary-basename>-pr9k-*`), not by branch name, so the spec's correctness does not depend on the branch prefix at all (review F14 corrected the prior load-bearing claim).
- **Supports decisions:** [D2](decision-log.md#d2-branch-naming-for-the-run), [D5](decision-log.md#d5-stale-worktree-handling)
- **Driven by findings:** review F1, F14
- **Referenced in spec:** Primary Flow (step 4), Edge Cases (detached HEAD, branch already exists).

## T3: First-push upstream — a fresh worktree branch has no remote tracking ref

- **Title:** The first push of the run's branch must establish a remote tracking ref so the push succeeds and subsequent pushes within the run can use bare `git push`.
- **Context:** [Edge Cases](../feature-specification.md#edge-cases-and-failure-modes) ("The first `git push` of the run's branch happens"), [Coordinations](../feature-specification.md#coordinations) ("Git (remote) — first push of the run's branch").
- **Technical detail:** A branch created locally has no upstream. Bare `git push` from such a branch fails because git doesn't know which remote ref to push to. The push step in the default workflow either (a) sets the upstream on first push so that a tracking ref is established and the push succeeds in one step, or (b) refuses to silently swallow non-zero exit codes when the push fails. The current `git_push` script does neither — it runs bare `git push` and traps all exits to zero, so the failure mode is invisible. This T# captures the constraint; the spec commits to "first push establishes a tracking ref AND surfaces non-zero exits" without prescribing a specific mechanism. The implementation plan picks the exact form (a flag added to the script, a pre-push tracking-ref check inside the script, or a wrapper in the runner). Once the upstream is set, the run's subsequent pushes (after later iterations) reuse the established tracking ref unchanged.
- **Supports decisions:** [D7](decision-log.md#d7-first-push-sets-upstream)
- **Driven by findings:** F-02
- **Referenced in spec:** Edge Cases (first push), Coordinations (Git remote — first push), Open Items (OI-1).

## T4: Active-run state file schema and lifecycle

- **Title:** The active-run state file at `<primary>/.pr9k/active-run.json` is the deterministic resume marker; its schema, lifecycle ordering, and PID-identity check are load-bearing for the spec's resume guarantee and concurrent-run guard.
- **Context:** [Primary Flow](../feature-specification.md#primary-flow) step 3 (read), step 7 (write), step 12 (remove). [Alternate Flows](../feature-specification.md#alternate-flows-and-states) "A prior run was killed mid-iteration" and "user runs `pr9k --fresh`". [Edge Cases](../feature-specification.md#edge-cases-and-failure-modes) state-file rows. [Coordinations](../feature-specification.md#coordinations) "Active-run state file" row.
- **Technical detail:** The state file is a single JSON object, atomic-written via `atomicwrite.Write` (the same pattern `workflowio.Save` uses for `config.json`). The file is the deterministic marker for "is there an active or recoverable pr9k run on this primary?" — its presence means yes, its absence means no.

  **Field set:**
  ```
  {
    "schemaVersion":  1,
    "pr9kVersion":    "<semver>",
    "worktreeStamp":  "pr9k-YYYY-MM-DD-HHMMSS.mmm",
    "worktreePath":   "<absolute path>",
    "branchName":     "pr9k-<worktreeStamp>",
    "primaryPath":    "<absolute path>",
    "pid":            <int>,
    "binaryPath":     "<absolute path to pr9k binary>",
    "startedAt":      "<RFC3339>"
  }
  ```

  **Lifecycle ordering (load-bearing for [T1](feature-technical-notes.md#t1-sequencing-constraint)):**
  - Read happens at startup, before subsystem construction, after CLI parse and config load. ENOENT is the dominant case (no active run); it must not be an error. Read happens regardless of `worktrees.enabled` — when `enabled: false` the file is read only to emit the warning described in the Edge Cases table.
  - Write happens *inside* startup, **after** `preflight.Run` creates `<primary>/.pr9k/`. The temp-file open used by `atomicwrite.Write` cannot create a missing parent directory (the underlying `os.OpenFile` returns ENOENT in that case), so `preflight.Run` must run first. Write happens only when `worktrees.enabled: true`.
  - Remove happens at the end of `Run` only on `Completed` or `LoopBroken` `ExitReason`. On `UserQuit`, the file is kept (so the next invocation auto-resumes). On hard kill, no removal code runs at all — the file naturally persists.

  **Resume validation (load-bearing for [D15](decision-log.md#d15-cross-run-resume-via-active-run-state-file); review F9, F11, F20):** before pr9k resumes into the worktree referenced in the state file, four conditions must hold: (1) the worktree directory exists at `worktreePath`; (2) `primaryPath` equals the currently resolved primary; (3) the branch named in `branchName` still exists in the repository; (4) the recorded process is no longer running (PID dead, or PID alive with a non-matching binary). If any of (1)–(3) fail the file is treated as stale (renamed with a `.stale-<timestamp>` suffix, warning emitted, fresh start). If (4) fails (live PID + matching binary), pr9k refuses with the concurrent-run error.

  **Path-comparison semantics (review F20):** both `worktreePath` and `primaryPath` are stored in the state file as **realpaths** (all symlinks resolved at write time). The resume-validation comparisons resolve the corresponding currently observed paths the same way before string-equating. Without this rule, a macOS user whose primary is reachable via both `/var/...` and `/private/var/...`, or whose `$HOME` is symlinked, would see a valid state file flagged as stale on every re-invocation that arrived through a different alias. The codebase's go-patterns coding standard already requires symlink-safe path resolution; this T# is the spec-level commitment that the resume-validation set follows the same standard.

  **Process-identity check (load-bearing for [D10](decision-log.md#d10-concurrent-runs-soft-lock-via-active-run-state-file)):** the recorded PID is checked using POSIX `syscall.Kill(pid, 0)` (the same pattern `internal/workflowio/crashtemp.go:135` uses). If the PID is alive, the running process's binary path is compared against `binaryPath`. If they match, treat as "another pr9k may be running" and refuse to start (or to `--fresh`-clean — review F7). If the PID is alive but the binary path doesn't match, treat as PID reuse by an unrelated process and resume normally. If the PID is dead, resume normally. The exact mechanism for resolving the running process's binary path is implementation detail (e.g., `/proc/<pid>/exe` on Linux, `proc_pidpath` on macOS, or a `ps`-based fallback) — the spec only commits to the behavioural outcome.

  **Corruption handling:** `atomicwrite` prevents partial writes, but manual edits could produce invalid JSON. On parse error, pr9k renames the file to `active-run.json.corrupt-<timestamp>` (preserving evidence) and proceeds as fresh.

  **Schema-version handling (review F15, F23):** if `schemaVersion` is unrecognised (the user downgraded pr9k between runs, or the file was manually edited), pr9k follows the same rename-and-warn pattern as for invalid JSON, but with a **distinct suffix `.incompatible-<timestamp>`** and a warning that names the schema version explicitly. The dedicated suffix avoids misleading the user into thinking their state file was corrupted (the JSON is intact; the data is simply from an incompatible version). The worktree itself is left on disk for the user to inspect or discard. Best-effort partial-parse and a full migration framework were rejected as YAGNI; the simpler corruption-path satisfies the same "no work lost" property because the worktree (which contains the work) is preserved.

- **Supports decisions:** [D5](decision-log.md#d5-stale-worktree-handling-at-startup), [D10](decision-log.md#d10-concurrent-runs-soft-lock-via-active-run-state-file), [D14](decision-log.md#d14-pr9k-worktree-prune-subcommand), [D15](decision-log.md#d15-cross-run-resume-via-active-run-state-file), [D16](decision-log.md#d16-runresult-exitreason), [D19](decision-log.md#d19-fresh-cli-flag)
- **Driven by findings:** V1, V2, V3, V5, V6 (resume validation); user redirect on the resume requirement; review F7, F9, F11, F15.
- **Referenced in spec:** Background (cross-run resume description), Outcome, Primary Flow (steps 3, 7, 12), Alternate Flows (resume, user-quit, hard-kill, prune, --fresh), Edge Cases (state-file rows), Coordinations (active-run state file row).

## T5: Per-worktree iteration log

- **Title:** A single `iteration.jsonl` per worktree, opened append-only by every pr9k invocation that resumes into the worktree. The file's lifetime is the worktree's lifetime; truncation by `lessons-learned` and removal by autoCleanup/`pr9k worktree prune` bound its growth. Each `IterationRecord` carries an `invocation_stamp` field so a forensic reader can attribute records to a specific invocation without splitting them across files.
- **Context:** [Primary Flow](../feature-specification.md#primary-flow) step 6, [Outcome](../feature-specification.md#outcome) (observability bullet), [Coordinations](../feature-specification.md#coordinations) (per-worktree iteration log row), [Edge Cases](../feature-specification.md#edge-cases-and-failure-modes) (iteration-log-spans-invocations row, post_issue_summary-includes-prior-invocations row).
- **Technical detail:** D17 commits to one append-only `iteration.jsonl` per worktree. The default workflow's existing consumers — `workflow/scripts/post_issue_summary` (line 11: hardcoded `JSONL_FILE=".pr9k/iteration.jsonl"`) and `workflow/prompts/lessons-learned.md` (lines 1, 5, 8: hardcoded `.pr9k/iteration.jsonl`) — read a literal relative path and have no mechanism to receive an invocation-stamp at runtime. The naive expectation that consumers could derive the path from `RunStamp()` (a Go-runtime value) is false for shell scripts and prompts (review F3). Keeping the path identical to today's, and keeping the file's content shape identical (one `IterationRecord` per line), means every existing consumer keeps working unchanged.

  Behavioural contract:

  1. **Path:** `<worktree>/.pr9k/iteration.jsonl`. Identical to today's `<projectDir>/.pr9k/iteration.jsonl` apart from the projectDir → worktree redirect.
  2. **Open mode:** `O_WRONLY|O_CREATE|O_APPEND, 0o644` (today's `AppendIterationRecord` semantics in `src/internal/workflow/iterationlog.go:47-49`, unchanged).
  3. **Cross-invocation behaviour:** every pr9k invocation that resumes into the worktree opens the same file and appends to whatever prior invocations left behind. `lessons-learned` (the only writer that truncates) sees the union of every invocation's records since the worktree was created or last truncated.
  4. **Record schema extension:** each `IterationRecord` written by this feature carries a new `invocation_stamp` field set to the current invocation-stamp. Older records (written by prior invocations) may not have the field; consumers tolerate its absence (today they ignore unknown/missing fields). The field is informational — no consumer in the default workflow filters by it.
  5. **Lifetime:** the file is removed only when the worktree is removed (autoCleanup on `Completed`/`LoopBroken`; `pr9k worktree prune`). It is not removed by `UserQuit`, by hard kill, or by routine startup.
  6. **Truncation:** the default workflow's `lessons-learned` step continues to truncate the file at the end of finalization (today's behaviour, see `workflow/prompts/lessons-learned.md` line 8). This is the bound that prevents unbounded growth across many resumed runs into the same worktree.

  **What this technical note does NOT introduce** (as compared to an earlier draft of T5 that committed to a stable-alias mechanism over per-invocation files): no symlinks, no atomic-rename of an alias entry, no per-invocation stamped filenames, no fatal-startup failure mode for alias creation. The single-file-per-worktree model has none of those moving parts because the file is just opened and appended to.

  **Why this preserves cross-invocation forensic value** that the alias model would have hidden: when a user quits pr9k mid-run before `lessons-learned` runs, the killed invocation's records remain in the file. The next invocation appends; `lessons-learned` (when it eventually runs) sees both invocations' records. This is exactly today's in-place behaviour and matches the user's stated requirement.

- **Supports decisions:** [D17](decision-log.md#d17-per-worktree-iteration-log)
- **Driven by findings:** review F3 (consumer-update path); V4 (resume validation); user revision after R2 (kill-before-lessons-learned regression in the per-invocation-files draft).
- **Referenced in spec:** Outcome (observability bullet), Primary Flow (step 6), Coordinations (per-worktree iteration log row), Edge Cases (iteration-log-spans-invocations row, post_issue_summary-includes-prior-invocations row).
