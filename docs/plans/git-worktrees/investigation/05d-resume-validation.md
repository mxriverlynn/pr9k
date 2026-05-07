# Adversarial Validation: Cross-Run Resume Mechanism (Option A)

**Date:** 2026-05-07  
**Validator:** Claude Code (adversarial pass)  
**Target:** `05-resume-options.md` — Option A recommendation and its sub-reports

---

## Validation Items

---

**V1: E1 "get_next_issue is naturally idempotent" breaks after close_gh_issue but before git push**

- **Strategy:** Challenge the Evidence
- **Hypothesis:** E1 claims `get_next_issue` re-delivers the in-progress issue on resume, making the mechanism idempotent. This is only true when the kill happens BEFORE `close_gh_issue`. The config.json step sequence (`workflow/config.json:28-29`) is `Close issue` (step 12) then `Git push` (step 13). A crash in that window — which includes any network failure during `git push` — leaves the issue **closed** and unpushed commits on the branch.
- **Investigation:** Read `workflow/config.json` (steps 28-29 confirm ordering). Read `workflow/scripts/get_next_issue` line 21: query is `is:issue is:open`. Read `workflow/scripts/git_push` lines 1-8: currently traps all exits to zero. Confirmed with D7 in `decision-log.md`: push failure routing is a planned fix, not yet implemented.
- **Result:** Partially Refuted. E1's "idempotency" claim is accurate only for kills during steps 1–11. For kills during step 13 (git push), the issue is already closed. On resume, `get_next_issue` skips it. The next run proceeds with the NEXT open issue, and the prior iteration's commits accumulate on the worktree branch without being pushed — silently. The plan's Option A section (`05-resume-options.md:77`) acknowledges this but attributes the fix to D7, which is unimplemented. Without D7, this is a silent data-loss scenario not guarded by the state file.
- **Impact:** IMPORTANT. E1 should be labeled conditional: "idempotent only when kill precedes close_gh_issue." The plan's statement that `get_next_issue` is "the single biggest leverage" overstates its coverage. The close-then-push window is a real gap. D7 must be listed as a hard prerequisite for Option A's correctness guarantee.
- **Classification:** Important

---

**V2: PID liveness via `syscall.Kill(pid, 0)` — false positive blocks the user permanently until `--fresh`**

- **Strategy:** Challenge the Evidence
- **Hypothesis:** The recommendation's concurrent-run guard (`05-resume-options.md:221`) says: "If alive → exit with clear message." On macOS, PID space is ≤ 99998. If pr9k crashes and the OS reassigns its PID to an unrelated long-running process (daemon, editor, shell), `syscall.Kill(pid, 0)` returns `nil` and pr9k refuses to start, reporting a false concurrent-run conflict. The user is blocked from all pr9k usage until they discover `--fresh`.
- **Investigation:** Read `src/internal/workflowio/crashtemp.go:134-140` — `classifyPID` has no additional checks beyond `syscall.Kill(pid, 0)`: a nil error means "active." The file comments at line 35 acknowledge "PID reuse is a known accepted limitation." But for crash-temp detection, the consequence is a spurious warning; for the state-file concurrent-run guard, the consequence is a **hard block**: pr9k exits without running. Read `05-resume-options.md:221`: "exit with clear message." There is no alternative path and no auto-resolution; the only escape is the `--fresh` flag which the user may not know about.
- **Result:** Confirmed. The false-positive rate is higher than acknowledged. macOS reuses low-numbered PIDs quickly. A user who restarts their laptop, runs other processes, and then restarts pr9k hours later is at meaningful risk. The "soft fail with clear message" framing understates the problem: the message is not soft — it is a complete block.
- **Impact:** IMPORTANT. The implementation must distinguish "PID alive and is actually a pr9k process" from "PID alive and is unrelated." One approach: write the binary path or a process-group ID into the state file and validate it before blocking. Alternatively, downgrade the alive-PID response from "exit" to "warn and continue after N seconds" with a user prompt. The plan must not ship with a silent hard block as the only concurrent-run detection behavior.
- **Classification:** Important

---

**V3: State-file removal requires `RunResult` to be propagated — currently discarded**

- **Strategy:** Challenge the Fix
- **Hypothesis:** The integration plan (`05-resume-options.md:245`) says: "Remove state file [after `Run` returns gracefully]." But `src/cmd/pr9k/main.go:298` is `_ = workflow.Run(...)` — the `RunResult` is discarded. To distinguish "graceful completion" (state file must be removed) from "user-quit mid-run" (state file should NOT be removed, resume on next start) from "breakLoopIfEmpty" (state file should be removed), the caller must inspect `RunResult`. Currently no such inspection exists.
- **Investigation:** Read `src/cmd/pr9k/main.go:298`: confirmed discard. Read `src/internal/workflow/run.go:459, 588, 749`: ActionQuit returns `RunResult{}` or `RunResult{IterationsRun: i}` — both are indistinguishable from a normal completion that just happened to run zero or i iterations. `RunResult` has only one field: `IterationsRun`. No `ExitReason` or `WasInterrupted` field exists.
- **Result:** Confirmed. The plan's integration table proposes state-file removal "after `Run` returns gracefully" but provides no mechanism to detect whether the return was graceful. A user who presses `q` mid-run will have the state file removed (if the implementation removes it on any return), making the next start a fresh run — which contradicts the auto-resume requirement. Or, if state file is never removed on quit, the distinction between "intentional quit" and "crash" disappears entirely. `RunResult` requires a new field (`Completed bool` or `ExitReason`), and `main.go` must be updated to propagate and act on it.
- **Impact:** CRITICAL. The plan's integration seam for state-file removal is broken as specified. `RunResult` must carry an exit-reason signal. This is not mentioned anywhere in the plan or sub-reports.
- **Classification:** Critical

---

**V4: `post_issue_summary` and `lessons-learned` break on multi-invocation `iteration.jsonl`**

- **Strategy:** Challenge the Fix
- **Hypothesis:** The plan says `iteration.jsonl` "accumulates across resumed runs" as a "natural unified history" (`05-resume-options.md:143`). But `workflow/scripts/post_issue_summary` reads ALL records from the file with no invocation filter (line 16: `jq -r '"- \(.step_name) [\(.status)]..."' "$JSONL_FILE"`). On a resumed run, the issue comment will include steps from the original (killed) run AND the resumed run — potentially duplicating all step names. The `lessons-learned.md` prompt (`workflow/prompts/lessons-learned.md:5`) also reads all records without invocation filtering.
- **Investigation:** Read `workflow/scripts/post_issue_summary:13-16`: confirmed — reads all records, no iteration or invocation filter. Read `workflow/prompts/lessons-learned.md:5,8`: reads all records, then truncates the file. If the first run had `feature-work: failed` and the resume had `feature-work: done`, the issue comment will show both. The lessons-learned Claude step will also see both, potentially misattributing the first run's failures as the current run's context.
- **Result:** Confirmed. The plan claims multi-invocation accumulation is "correct" and "no merge logic is needed" (`05c-resume-behavior.md:276`), but this ignores the behavior of existing consumers that make per-run assumptions. The proposed "session_boundary" record (`05-resume-options.md:225`) with `{type: "session_start"}` requires a new `Type` field in `IterationRecord` (not present today: `src/internal/workflow/iterationlog.go:15-27`). Without that field, scripts cannot filter to the current invocation. Even with the field, `post_issue_summary` and `lessons-learned.md` would need to be updated to filter by the current invocation.
- **Impact:** CRITICAL. The plan declares `iteration.jsonl` accumulation as a solved problem but the evidence shows two existing consumers that will produce corrupt output without changes. These consumers are not listed in the integration plan's file-by-file table.
- **Classification:** Critical

---

**V5: E10 sequencing conflict — the ROOT CAUSE SUMMARY misleads about the write seam**

- **Strategy:** Challenge the Assumptions
- **Hypothesis:** E10 (`05-resume-options.md:69`) states: "`.pr9k/active-run.json` can be safely written from any point after preflight. The natural insertion point is `main.go` between `cli.Execute` and `startup`, lines 129–141." But `preflight.Run` — which creates `.pr9k/` — is called at line 70 INSIDE `startup()`, not before it. The "between `cli.Execute` and `startup`" seam is BEFORE `.pr9k/` exists. `atomicwrite.Write` requires the parent directory to exist (confirmed: `src/internal/atomicwrite/write.go:87-115`).
- **Investigation:** Read `src/cmd/pr9k/main.go:62-115` (`startup` function body): `preflight.Run` is at line 70 inside `startup`. Read `src/cmd/pr9k/main.go:128-145` (`main` function): `startup` is called at line 141, after `cli.Execute` at line 129. The proposed READ seam (before `startup`) is safe because `os.Open` on a missing path returns `ENOENT` (treated as "no active run"). The proposed WRITE seam ("during startup, after worktree resolved") in the integration table is correctly inside `startup`, after `preflight.Run`. But the root-cause rationale (item 4: "insertion seam is clean") and E10 both point to "between `cli.Execute` and `startup`" as THE seam — conflating read and write into one seam that only works for reads.
- **Result:** Partially Refuted. The integration table separates read and write correctly. The root-cause text (E10 and item 4) is misleading by pointing to one seam for both operations. The actual write must go inside `startup` after `preflight.Run`. The plan's prose will cause implementers to place the write in the wrong location unless they read the integration table instead of the rationale.
- **Impact:** Important. The integration table is correct; the root-cause prose is not. The implementation guidance needs clarification: READ before startup, WRITE inside startup after preflight.
- **Classification:** Important

---

**V6: D5 stale-worktree warning fires for the resumed worktree itself**

- **Strategy:** Challenge the Assumptions
- **Hypothesis:** D5 (`decision-log.md:78`) uses a path-prefix filter (`<primary-basename>-pr9k-*`) to detect stale worktrees at startup. Option A creates a stamped worktree that matches this prefix. On a resumed run, D5 will fire a warning for the worktree that pr9k is about to ENTER, misclassifying the active worktree as stale. The plan claims "D5 carries over" unchanged (`05-resume-options.md:213`).
- **Investigation:** Read `decision-log.md:78`: D5 is path-prefix only — "branch state is not used for filtering." The state file `active-run.json` exists when resume is detected. If D5 runs BEFORE the state file is read (D5 is part of startup; state-file read is proposed before startup), the warning would fire before pr9k knows it is resuming. If D5 runs AFTER state-file read, it must be updated to exclude the worktree recorded in the state file. The plan says D5 "carries over" with no such update.
- **Result:** Confirmed. D5 needs to be updated to exclude the worktree matching `active-run.json` (when that file exists and points to a valid worktree). Without this change, every resumed run warns the user about its own working directory, which is a false positive that degrades trust in D5's warnings.
- **Impact:** Important. D5 must be added to the integration plan's file-by-file change list. Sequencing matters: the state-file read must occur before D5's stale-worktree scan, and D5 must receive the active-worktree path to exclude it from the warning list.
- **Classification:** Important

---

**V7: `iteration.jsonl` session_boundary record requires a schema change with no consumer migration path**

- **Strategy:** Challenge the Fix
- **Hypothesis:** The plan proposes appending a `{type: "session_start", invocationStamp: ..., resumedFrom: ...}` record to `iteration.jsonl` on each invocation (`05-resume-options.md:225`). But `IterationRecord` (`src/internal/workflow/iterationlog.go:15-27`) has no `Type` field. The documentation at `docs/code-packages/workflow.md:125` states: "schema_version: Always 1. Third-party parsers should reject unknown versions."
- **Investigation:** Read `src/internal/workflow/iterationlog.go:15-27`: no `Type` field. Read `docs/code-packages/workflow.md:125, 196`: confirmed schema_version is always 1, reject-unknown-versions is the documented contract. Read `workflow/prompts/lessons-learned.md:8`: the lessons-learned step **truncates** `iteration.jsonl` at the end of each run. If session_boundary records exist from a prior invocation, they would be truncated. On the next resumed run, there are no session markers from prior invocations — defeating the purpose of adding them. The plan says the schema bump is "optional" but this understates the tooling impact.
- **Result:** Confirmed. The `session_boundary` approach requires: (1) a `Type` field in `IterationRecord` (schema change), (2) a schema_version bump from 1 to 2, (3) updates to `post_issue_summary`, `lessons-learned.md`, and `deferred-work.md` to handle the new record type, and (4) acknowledgment that `lessons-learned.md` truncates the file, defeating multi-invocation boundary markers. None of these are in the integration plan. The plan lists this as "Optionally: append session-boundary record."
- **Impact:** Important. Either drop the session_boundary feature (simplest path), or list all four required changes explicitly. The current plan treats it as a one-line addition when it is a multi-file coordinated change.
- **Classification:** Important

---

**V8: B-R23's claim that iteration.jsonl records correlate per-invocation via `SessionID` is false**

- **Strategy:** Challenge the Evidence
- **Hypothesis:** `05c-resume-behavior.md:B-R23` claims: "The `run_stamp` is embedded in each record (via `rec.SessionID` indirectly through `cfg.RunStamp` passed through the artifact path), making it possible to correlate records per invocation." This is factually incorrect.
- **Investigation:** Read `src/internal/workflow/iterationlog.go:15-27`: `IterationRecord` has a `SessionID` field (`json:"session_id,omitempty"`). Read `src/internal/workflow/run.go:450, 579, 712`: `rec.SessionID = disp.capturedStats.SessionID` — this is the **Claude session ID** returned by the model, not `cfg.RunStamp`. Shell steps have no SessionID (empty string). `RunStamp` appears in `RunConfig` but is never written into any `IterationRecord` field. There is no invocation correlation field in `IterationRecord`.
- **Result:** Confirmed refutation of B-R23. The `SessionID` in `IterationRecord` is the Claude API session identifier, not the pr9k invocation stamp. Shell steps write empty `SessionID`. There is currently NO mechanism to determine which records in `iteration.jsonl` belong to which pr9k invocation. This makes B-R23's reassurance that "post-run analysis can trace which records belong to which invocation" unfounded.
- **Impact:** Important. B-R23 is a false evidence item that understates the schema-change burden. Add to V7's list of required changes.
- **Classification:** Important (elevates V7)

---

**V9: `--fresh` leaves an orphaned branch when the worktree no longer exists**

- **Strategy:** Challenge the Fix
- **Hypothesis:** If the worktree was manually deleted (or never created due to a crash during worktree creation), the state file still references `branchName: "pr9k/<stamp>"`. The plan says: "state file points to a gone worktree → log warning, clean up, start fresh." But "start fresh" means creating a new stamped worktree on a new branch. The old `pr9k/<stamp>` branch was never deleted — it was just orphaned from its worktree. The plan does not address branch cleanup in this scenario.
- **Investigation:** Read `05-resume-options.md:214`: "state file points to gone worktree → log + clean + start fresh." No mention of the dangling branch. Read `decision-log.md:226` on `pr9k worktree prune`: the subcommand deletes branches matching `pr9k/*`. So the user can run `pr9k worktree prune --dry-run` and find the orphaned branch. But this is a manual step that defeats the automatic recovery requirement. The integration plan table does not list branch cleanup as a step in the "worktree-missing" recovery path.
- **Result:** Confirmed. The "start fresh" path for a missing worktree leaves a dangling `pr9k/<stamp>` branch. Over multiple such events (repeated crashes during worktree creation), branches accumulate. D5's stale-worktree filter would NOT catch these because there is no worktree directory — only the branch. The branch accumulation is silent.
- **Impact:** Important. The missing-worktree recovery path must also run `git branch -D pr9k/<stamp>` (or verify the branch has no commits before deleting). Add this to the integration plan.
- **Classification:** Important

---

**V10: `syscall.Kill` in `crashtemp.go` has no build tag — not a cross-platform precedent**

- **Strategy:** Challenge the Assumptions
- **Hypothesis:** The plan justifies the PID liveness check by citing `src/internal/workflowio/crashtemp.go:134` as a "direct analogue" (`05-resume-options.md:65`). But `crashtemp.go` has no build tag restricting it to non-Windows platforms. `detect_unix.go` does have `//go:build !windows`. This is inconsistent. If pr9k ever supports Windows, `crashtemp.go`'s `syscall.Kill` will fail to compile.
- **Investigation:** Read `src/internal/workflowio/crashtemp.go` header: no build tag. Read `src/internal/workflowio/detect_unix.go:1`: `//go:build !windows`. The Docker ADR (`docs/adr/20260413160000-require-docker-sandbox.md`) requires Docker as a runtime dependency; Docker Desktop runs on Windows. `workflow.go:242, 246` also uses `syscall.Kill` with no build tag. The codebase is de facto POSIX-only due to Docker + `syscall.Kill`, but this is not encoded in build constraints.
- **Result:** Confirmed as informational. pr9k is implicitly POSIX-only (Docker + multiple `syscall.Kill` calls without build constraints). The new `runstate` package would add another `syscall.Kill` with no build tag, consistent with existing practice. This is not a blocker for the recommendation but represents a known gap.
- **Impact:** Informational. No change needed for Option A. If Windows support is ever attempted, all `syscall.Kill` callsites must be wrapped in build-constrained files.
- **Classification:** Informational

---

**V11: Schema-version downgrade on `active-run.json` has no precedent — ownership unclear**

- **Strategy:** Challenge the Assumptions
- **Hypothesis:** The plan proposes a `schema_version` field in `active-run.json` and says: "if the loaded version is unknown, refuse to resume." There is no existing precedent for runtime state-file schema versioning in this codebase. `iteration.jsonl` has `schema_version` but no reader checks it for version mismatch — the field is described as "for future use." Who owns `active-run.json` schema versioning in a new `internal/runstate` package?
- **Investigation:** Read `src/internal/workflow/iterationlog.go:13-16`: `SchemaVersion` is set but never read back in any production code path. No version-checking consumer exists. Read `docs/code-packages/workflow.md:125`: "Third-party parsers should reject unknown versions" — this is documentation for external tools, not enforced in Go code. The plan invents a new behavior (refuse to resume on unknown schema version) with no implementation precedent in the codebase.
- **Result:** Confirmed. The schema-version downgrade refusal is a novel pattern. The cost to implement it safely is non-trivial: the reader must check `schema_version`, compare it to a constant, and produce an actionable error message (not just "corrupt file"). The implementation burden is understated. Additionally, as the hypothesis notes: a user who intentionally downgrades pr9k to fix a regression is now blocked from running pr9k at all (the state file from the newer version has an unknown schema version). The `--fresh` escape hatch mitigates this but requires the user to know about it.
- **Impact:** Important. The plan should either (a) accept schema-version 1 and add tolerance for future versions using an ignore-unknown-fields approach (consistent with how `config.json` handles forward compatibility), or (b) explicitly document the downgrade trap as a known user-hostile edge case.
- **Classification:** Important

---

**V12: Hypothesis 3 (kill between loop-end and state-file removal) is a no-op**

- **Strategy:** Challenge the Evidence
- **Hypothesis:** A kill between "iteration loop ended" and "state file removed" leaves the state file on disk. The next run resumes. `get_next_issue` finds no open issues (all were processed). The loop runs zero iterations. The finalize phase runs. This is the correct behavior — it re-runs finalization steps, which are idempotent (code review, git push, etc.).
- **Investigation:** Read `workflow/config.json:31-40`: finalize steps include code review, check review verdict, fix review items, final CI check, update docs, deferred work, lessons learned, final git push. All of these are safe to re-run on a completed codebase. `breakLoopIfEmpty` on `get_next_issue` would exit the iteration loop immediately. The finalize phase proceeds. The state file is then removed by the completed run.
- **Result:** Confirmed as no-op. The window between loop completion and state-file removal does not produce incorrect behavior. It produces a slightly wasteful extra finalize pass at worst.
- **Impact:** Informational. No design change needed for this scenario. The hypothesis was accurate that this is a failure mode to analyze; the analysis shows it is benign.
- **Classification:** Informational (no concern)

---

## Confidence Assessment

- **Level:** Medium
- **Rationale:** Three critical findings (V3, V4, V8) are directly supported by source code evidence contradicting specific plan claims. The plan's integration table is structurally sound; the critical gaps are in uncovered blast radius (existing script consumers) and an unimplemented cross-cutting concern (RunResult propagation). The recommendation's core mechanism — state file + stamped worktree + `get_next_issue` idempotency — is architecturally correct for the common case (kill before close_gh_issue). The gaps affect specific scenarios and implementation details rather than the overall approach.

---

## Remaining Risks

1. **D7 dependency is hard, not soft.** Option A's correctness guarantee for the close-then-push-fail window requires D7 (push failure surfacing) to be implemented before Option A is deployed. Without D7, that window produces silent commit loss. The plan lists D7 as prerequisite in `05c-resume-behavior.md:B-R24` but does not elevate it as a blocker in the recommendation itself.

2. **`RunResult` exit-reason discrimination is unspecified.** The exact field name, values, and propagation path for distinguishing "graceful completion" from "user quit" from "breakLoopIfEmpty" are not in the plan. This is a spec gap, not just an implementation detail — it affects which scenarios trigger state-file removal and which preserve it for resume.

3. **State-file write inside `startup` interacts with startup's early-return paths.** If `steps.LoadSteps` or `validator.Validate` fails (before `preflight.Run`), `.pr9k/` does not yet exist and the state file cannot be written. The read (before `startup`) would still work. But the recovery path — "if valid state file exists, use it" — must run before the startup failure exits, otherwise a valid prior-run state is silently ignored.

4. **`pr9k worktree prune` and `--fresh` interaction.** The plan says `--fresh` "optionally" removes the prior worktree but does not define what "optionally" means (second flag? smart default?). If `--fresh` deletes only the state file, the worktree becomes stale and D5 warns about it forever. If `--fresh` deletes both, it must invoke `git worktree remove --force` and `git branch -D` — the same machinery as `autoCleanup`. This interaction is unspecified.

5. **The close-then-push window is not guarded by the state file.** Even with Option A implemented, if `close_gh_issue` succeeds and `git push` fails (with D7 implemented and the user presses `q`), the state file is removed (graceful quit). The next run starts fresh, skips the closed issue, and the commits remain unpushed indefinitely. This is the documented D15 rationale (`decision-log.md:243`: "the silent-skip risk is already mitigated by D7's push-failure error-recovery routing") — but only if the user chooses `r` (retry) instead of `q` (quit). Choosing `q` after a push failure still produces this state.
