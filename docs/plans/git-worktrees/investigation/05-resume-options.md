# Investigation: Cross-Run Resume Options for pr9k

One-sentence summary: The user has stated cross-run resume is a hard requirement that the original spec does not satisfy; this report enumerates six options, evaluates them against pr9k's actual architecture, and recommends a refined Option A (stamped worktree per active run + active-run marker file).

## Problem Statement

- **Symptom**: When pr9k is killed mid-run (user interrupt to fix a pr9k bug, system crash, network drop), the next pr9k invocation has no awareness of the prior run's state. It creates a new worktree on a new branch, leaving the prior partial work orphaned. The user must manually find the stale worktree, inspect it, and decide what to do — exactly the manual chore the user said they will not accept.
- **Expected behavior**: pr9k restart MUST automatically detect that a prior run was interrupted and resume into its worktree, branch, and accumulated commits, without user intervention. The mechanism MUST be deterministic.
- **Conditions**: Whenever pr9k is killed for any reason (graceful quit, SIGKILL, panic, OOM, network failure during a step) before the workflow's iteration loop finishes.
- **Impact**: Without cross-run resume, the worktree feature creates more friction than it removes. The user has stated this requirement is a blocker for the entire feature.

## Evidence Summary

Evidence is grouped by the angle each investigator covered. Source files for the full quoted command output and code snippets are listed at the top of each group.

### Group A — Iteration loop and per-run state
**Source file:** [`05a-iteration-loop-state.md`](05a-iteration-loop-state.md).

#### E1: `get_next_issue` is naturally idempotent
- **Source:** `workflow/scripts/get_next_issue` (verified during investigation).
- **Finding:** The script queries `gh issue list --state open --label ralph` and returns the lowest number. If a prior run was killed before the close-issue step, the issue is still open and the script re-delivers the same issue on the next invocation. No additional tracking is needed at the script level.
- **Relevance:** The iteration loop does not need a "skip already-completed iterations" mode. Just calling `get_next_issue` again at the top of a fresh iteration loop naturally re-picks up the in-progress work. This is the single biggest leverage in the design — it eliminates the need for per-iteration bookmarks.

#### E2: VarTable is freshly constructed each run
- **Source:** `src/internal/workflow/run.go:315`, `src/internal/vars/vars.go`, `docs/code-packages/vars.md`.
- **Finding:** `vars.New(...)` is called at the top of `Run`. There is no mechanism to seed it from prior-run state. The persistent scope (initialize-phase `captureAs` bindings like `GH_USER`) is the only scope that needs to survive across runs.
- **Relevance:** If we re-run the initialize phase on every resume, the persistent scope is naturally re-derived. Persistent captures like `GH_USER` are idempotent (`gh api user --jq .login` returns the same value across invocations), so this is safe. Iteration and finalize scopes are intentionally per-iteration; nothing depends on them across runs.

#### E3: `iteration.jsonl` is durable across SIGKILL but is not a state file
- **Source:** `src/internal/workflow/iterationlog.go:15` (schema_version), `:49` (O_APPEND with explicit close per write).
- **Finding:** Each `AppendIterationRecord` call opens the file in `O_APPEND`, writes, and closes — so SIGKILL does not lose committed records. However, it records step *outcomes*, not VarTable values; reconstructing all the persistent captures from `iteration.jsonl` alone would require parsing every step's stdout. It is durable history, not a structured state file.
- **Relevance:** `iteration.jsonl` cannot replace a state file, but it can be append-extended across resumed runs (multiple invocations contributing records to the same file inside a worktree). That is a natural unified history.

#### E4: No existing state file or checkpoint mechanism
- **Source:** Searches across `src/internal/workflow/`.
- **Finding:** No `atomicwrite` usage in the run path, no checkpoint, no lock file, no resume-detection code. The only durable artifacts are `iteration.jsonl`, the `.log` file, and per-step JSONL transcripts.
- **Relevance:** Clean slate to add a state file. No existing pattern to fight.

#### E5: Graceful quit has no hook for additional state writes
- **Source:** `src/cmd/pr9k/main.go:298` (`RunResult` discarded), quit path in `src/internal/workflow/workflow.go`.
- **Finding:** The quit path goes `ForceQuit` → `ActionQuit` → `Run` returns → `log.Close()` and process exits. There is no deferred cleanup hook structure today.
- **Relevance:** Adding a "remove state file on graceful end" step is a small new responsibility for `main.go`, not a refactor of an existing cleanup chain.

#### E6: Logger is buffered; SIGKILL truncates the `.log` file
- **Source:** `src/internal/logger/logger.go` (uses `bufio.Writer`).
- **Finding:** The `.log` file may lose its last buffered chunk on hard kill. This is diagnostic data, not state.
- **Relevance:** State must not depend on logger output. State must use `atomicwrite` or `O_APPEND` with explicit close, like `iteration.jsonl` does (E3).

#### E7: Cross-run Claude session resume is blocked by gate G1, but the iteration-loop resume does not need it
- **Source:** `docs/code-packages/workflow.md:90-145`, `src/internal/workflow/workflow.go`.
- **Finding:** The five resume gates G1–G5 control intra-run Claude conversation resume. G1 ("zero-value SessionID") fails on the first step of every new pr9k invocation, so cross-run Claude session resume is currently impossible. But pr9k's loop-level resume does NOT require resuming Claude conversations — it just requires re-running the iteration on the existing branch+commits. Claude starts a fresh conversation; that's fine.
- **Relevance:** No need to touch the resume gates. Cross-run resume operates at the iteration-loop level, not the Claude-session level.

### Group B — Persistent state mechanisms in pr9k
**Source file:** [`05b-state-mechanisms.md`](05b-state-mechanisms.md).

#### E8: `atomicwrite.Write(path, data, mode os.FileMode) error` is the canonical durable-write pattern
- **Source:** `src/internal/atomicwrite/write.go:34`.
- **Finding:** Single exported function. Uses `<basename>.<pid>-<nanoseconds>.tmp` + rename. Requires the parent directory to exist. EXDEV propagates unwrapped. One direct production caller (`workflowio/save.go`).
- **Relevance:** This is exactly the right pattern for writing a small JSON state file. No adapter needed.

#### E9: Crash-temp detection in `workflowio` is the precedent for stale-PID handling
- **Source:** `src/internal/workflowio/crashtemp.go:134`.
- **Finding:** `classifyPID` uses `syscall.Kill(pid, 0)` to test whether a PID is alive. The same pattern is the right way to detect "is the prior pr9k still running, or did it die?" PID reuse is acknowledged as an accepted limitation (`crashtemp.go:35`).
- **Relevance:** Direct analogue. We can use the same pattern to detect "active vs dead pr9k" without reinventing it.

#### E10: `.pr9k/` is created in preflight before any other startup work
- **Source:** `src/internal/preflight/run.go:37` (`os.MkdirAll`).
- **Finding:** Preflight is startup step 3, before logger construction (step 4) and artifact dir creation (step 5). `.pr9k/active-run.json` can be safely written from any point after preflight. The natural insertion point is `main.go` between `cli.Execute` and `startup`, lines 129–141.
- **Relevance:** No new mkdir code needed. The directory is guaranteed.

#### E11: The four-file lockstep rule for adding a top-level config field
- **Source:** `src/internal/validator/validator.go:111` (`vFile`), `:205` (`DisallowUnknownFields`), `:215` (`docToVFile`); `src/internal/steps/steps.go:104` (`StepFile`); `src/internal/workflowmodel/model.go:71` (`WorkflowDoc`).
- **Finding:** Adding a new top-level field requires changes in exactly these four locations. Optional blocks follow the pointer pattern (`*StatusLineBlock`, `*DefaultsBlock`).
- **Relevance:** If the resume policy ends up with config knobs (e.g., `resumePolicy.autoResume: true`), the cost is the same as the original `worktrees` block addition. If we keep it implicit (no config), no schema work is needed.

#### E12: CLI flag pattern is well-established
- **Source:** `src/internal/cli/args.go:17` (`Config`), `:134-136` (flag registration), `src/cmd/pr9k/wiring.go:74` (`buildRunConfig`).
- **Finding:** Adding `--fresh` (or `--no-resume`) is a 2-line change: a flag binding plus a Config field, with downstream propagation through `buildRunConfig`.
- **Relevance:** A "force fresh start" affordance is cheap to add.

#### E13: TUI iteration line has a heartbeat-style suffix seam
- **Source:** `src/internal/ui/header.go:143-154`, `src/internal/ui/model.go:672` (`heartbeatSuffix`).
- **Finding:** The TUI already appends contextual suffixes to the iteration line. Adding a "RESUMED FROM <stamp>" annotation follows the same pattern.
- **Relevance:** TUI surfacing is a one-line addition. No new chrome.

### Group C — Behavioral cleanliness across the six options
**Source file:** [`05c-resume-behavior.md`](05c-resume-behavior.md).

#### E14: `get_next_issue` idempotency is the single biggest design simplifier (behavioral)
- **Source:** Behavioral analysis confirming E1.
- **Finding:** Because the script naturally re-delivers the in-flight issue, "resume" at the iteration-loop level is just "enter the existing worktree and run the loop normally." No skip-N-iterations logic, no per-step bookmarks, no in-flight-step recovery. The simplest version is to re-run the entire iteration from `feature-work` on the existing branch with the existing commits.
- **Relevance:** This is what makes Option B (and refined Option A) feasible without invasive loop changes.

#### E15: The "completed but kept around" worktree is the design pivot
- **Source:** Behavioral analysis on the lifecycle of `autoCleanup: false`.
- **Finding:** When `autoCleanup: false` and a run completes gracefully, the worktree remains on disk. On the next pr9k invocation, the question becomes: "does this existing worktree mean the prior run is still in flight, or did it complete and we just kept it for inspection?" An option without a state file (B, C, E) cannot distinguish these. An option with a state file (A, F) can.
- **Relevance:** This is the case that rules out Option B as a strict winner. Either we mandate `autoCleanup: true` (forcing the user to give up post-run worktree inspection) or we add a state file. The state file is cheaper than removing user choice.

#### E16: Step-level resume bookmarks are not needed
- **Source:** Behavioral analysis on Option D.
- **Finding:** Option D's per-step bookmarks would require modifying the iteration loop's step dispatcher and recording all `captureAs` values into the state file for skipped steps. Massive complexity for no behavioral gain — `get_next_issue` already gives us iteration-level resume for free.
- **Relevance:** Drop D. The simpler option is "always re-run the iteration from the beginning."

## Root Cause Analysis

### Summary

This is not a bug; it is a missing capability. The original spec (D15) declared cross-run resume out of scope on YAGNI grounds. The user has now provided the evidence the YAGNI rule asked for: explicit user need, a named blocker for the feature. D15 is reversed.

### Detailed Analysis

The architecture supports cross-run resume cheaply because:

1. **Issue selection is idempotent** (E1, E14) — `get_next_issue` re-delivers in-flight work without any tracking. The iteration loop just needs to start over.
2. **Persistent variable scope is re-derivable** (E2) — initialize-phase captures are idempotent; re-running them on resume is fine.
3. **Durable state primitives already exist** (E8, E9, E10) — `atomicwrite`, the crash-temp pattern, and the preflight-created `.pr9k/` directory are exactly the building blocks needed.
4. **Insertion seam is clean** (E10) — between `cli.Execute` and `startup` in `main.go`, before any subsystem initialization.
5. **No existing chains to refactor** (E4) — clean slate for state-file logic.

The only architectural cost is distinguishing "active or recoverable run" from "completed run kept for inspection." A small state file (`.pr9k/active-run.json`) does this. Without the state file, we either lose user choice (mandate `autoCleanup: true`) or lose determinism (Option B with `autoCleanup: false` cannot tell completed-and-kept from killed-mid-run).

## Coding Standards Reference

| Standard | Source | Applies To |
|----------|--------|------------|
| `atomicwrite.Write` for durable replacement | `docs/coding-standards/file-writes.md` | The active-run state file write |
| `O_CREATE\|O_EXCL\|O_WRONLY, 0o600` for initial temp files | Same | Atomicwrite handles this internally; verify |
| Concurrency: snapshot-then-unlock | `docs/coding-standards/concurrency.md` | The state file is read once at startup; not concurrent — keep simple |
| Errors: package-prefixed, file paths in I/O messages | `docs/coding-standards/error-handling.md` | New `internal/runstate` package error messages |
| ADR `20260410170952-narrow-reading-principle` | docs/adr/ | The state file is runtime state, not workflow content; lives in `.pr9k/`, not in `config.json` |
| ADR `20260424120000-workflow-builder-save-durability` | docs/adr/ | Atomic writes for any user-facing or run-state file |
| Versioning: `config.json` schema is part of public API | `docs/coding-standards/versioning.md` | If we add a config knob; not strictly needed if resume is always-on |

## The Six Options

### Option A: Stamped worktree per active run + active-run marker file

- **Worktree path:** `<primary-parent>/<primary-basename>-pr9k-<worktree-stamp>/`. The stamp in the path is the **worktree-stamp**, baked at first creation; it does NOT change across resumes.
- **Branch:** `pr9k/<worktree-stamp>` — also baked at first creation.
- **State file:** `<primary>/.pr9k/active-run.json`. Created at startup with atomic write; removed on graceful completion; left in place on crash. Carries: `worktreeStamp`, `worktreePath`, `branchName`, `pid`, `startedAt`, `pr9kVersion`.
- **Resume detection:** state file exists at startup → enter the worktree it points to and resume. State file absent → fresh start.
- **Log directory:** each pr9k invocation generates its own **invocation-stamp** for logs (`.pr9k/logs/ralph-<invocation-stamp>.log`, artifact dir). The `iteration.jsonl` inside the worktree's `.pr9k/` accumulates across runs (O_APPEND).
- **autoCleanup interaction:** on graceful completion with `autoCleanup: true`, remove worktree, branch, and state file. On graceful completion with `autoCleanup: false`, remove state file only; worktree+branch stay.
- **Crash interaction:** state file remains; next run resumes.
- **Concurrent-runs:** state file's `pid` is liveness-tested via `syscall.Kill(pid, 0)`. If alive → "another pr9k may be running" warning. If dead → resume.
- **Abandon affordance:** `--fresh` CLI flag removes the state file (and optionally the worktree, via `pr9k worktree prune` semantics).

### Option B: Fixed worktree path, branch reuse, no state file

- **Worktree path:** always `<primary-parent>/<primary-basename>-pr9k/`. No stamp.
- **Branch:** stamped on first creation (`pr9k/<initial-stamp>`); reused on every subsequent run (whatever the worktree is currently on).
- **State file:** none. Worktree existence is the marker.
- **Resume detection:** worktree exists → enter it, continue. Doesn't exist → create it.
- **Problem (E15):** with `autoCleanup: false`, the worktree remains after graceful completion. The next run cannot distinguish "completed and kept for inspection" from "killed mid-run, resume me." The next run would unconditionally reuse, accumulating commits indefinitely on the same branch, which is not the user's expected behavior.
- **Mitigation:** mandate `autoCleanup: true`. This eliminates the keep-for-inspection use case (which the user explicitly wanted preserved per V7).

### Option C: No worktrees — separate clone managed by pr9k

- **Mechanism:** `git clone <primary> <primary>-pr9k/` once; reuse across runs.
- **Resume:** identical to B once the clone exists.
- **Cost:** full clone takes seconds-to-minutes for large repos vs. milliseconds for `git worktree add`. Object storage is duplicated (no sharing).
- **Same problem as B** for the `autoCleanup: false` case.

### Option D: Per-iteration bookmarks in state file

- **Mechanism:** state file tracks per-iteration step completion; resume can jump to a specific step.
- **Cost:** modifies the iteration loop's step dispatcher (the only invasive loop change of any option). Requires recording all `captureAs` values for skipped steps. Massive complexity.
- **Gain:** zero, given E14 — `get_next_issue` already gives iteration-level resume for free.
- **Verdict:** rejected.

### Option E: In-place workflow + branch management

- **Mechanism:** pr9k stays in the primary checkout, switches to its own branch on start, restores user's branch on end.
- **Resume:** if pr9k's branch is checked out, continue.
- **Problem:** the primary checkout is on pr9k's branch during the run. The user cannot keep using their primary for parallel development — the original feature goal fails.
- **Verdict:** rejected on requirement #5.

### Option F: Fixed worktree + state file

- **Mechanism:** B + state file.
- **Effect:** essentially Option A but with a fixed path. Loses the ability to keep multiple historical worktrees on disk for users with `autoCleanup: false`. Otherwise equivalent to A.
- **Verdict:** strictly inferior to A — A subsumes its behavior and adds the historical-worktree capability for `autoCleanup: false`.

## Comparison Matrix

| Concern | A (stamp + file) | B (fixed path) | C (clone) | D (bookmarks) | E (in-place) | F (fixed + file) |
|---|---|---|---|---|---|---|
| Auto-resume after kill | ✓ | ✓ | ✓ | ✓ | ✓ at iter boundary | ✓ |
| Preserves local commits | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| Deterministic (no fuzzy match) | ✓ (state file) | ✓ (path existence) | ✓ (path existence) | ✓ (state file) | ✗ (branch checkout state) | ✓ |
| `autoCleanup: false` works | ✓ | ✗ (reuses indefinitely) | ✗ same as B | ✓ | ✓ | ✗ same as B unless adds workaround |
| Worktree-feature goal (primary stays usable) | ✓ | ✓ | ✓ | ✓ | ✗ | ✓ |
| Loop changes required | None | None | None | Major | None | None |
| New seam point | Single (main.go) | Single (main.go) | Single (main.go) | main.go + run.go | Single | Single |
| Disk cost per active run | ~repo size (worktree) | ~repo size (worktree) | ~2× repo (full clone) | same as A | minimal | ~repo size |
| Multiple historical worktrees on disk | Yes (with `autoCleanup: false`) | No (single fixed path) | No | Yes | No | No |
| TUI surfacing of "resumed" | Easy (read state file) | Possible (compare HEAD vs primary HEAD) | Same as B | Easy | N/A | Easy |
| User affordance to abandon | `--fresh` removes state file | `pr9k worktree prune` only | Same | Same as A | Manual `git checkout` | Same as A |
| Concurrent-run guard | Built-in (PID in state file) | None | None | Built-in | None | Built-in |
| Cross-run config.json change | Picks up new config | Picks up new config | Picks up new config | Risky (bookmarks may reference old steps) | Picks up new config | Picks up new config |

## Recommendation

**Option A: Stamped worktree per active run + active-run marker file.**

Rationale:

1. **Satisfies all six hard requirements.** Auto-resume, commit preservation, determinism (state file is unambiguous), iteration-level recovery (E14 idempotent issue lookup), primary-checkout-stays-usable (worktree feature), concurrent-run guard (PID liveness check via state file).
2. **Preserves user choice on `autoCleanup`.** Both modes work correctly. `autoCleanup: true` → completed runs auto-clean; killed runs resume. `autoCleanup: false` → completed runs keep their worktree (user's stated need from V7); killed runs resume into the same worktree.
3. **No iteration-loop changes.** The seam is `main.go` between `cli.Execute` and `startup`. The runner construction passes the worktree path either freshly stamped or recovered from the state file; everything downstream is unchanged.
4. **Reuses existing patterns.** `atomicwrite.Write` for the state file (E8). `syscall.Kill(pid, 0)` for liveness (E9). The `.pr9k/` directory is already preflight-created (E10).
5. **Builds on the worktree feature already specified.** D1 (sibling location), D2 (stamped branch), D5 (path-prefix detection), D11 (TUI surfacing), D14 (`pr9k worktree prune`) all carry over. Only D8 (config shape), D13 (autoCleanup behavior), and D15 (no cross-run resume) need updates to encode the resume mechanism.
6. **Cleanly handles edge cases** (state file points to gone worktree → log + clean + start fresh; corrupt state file → backup + start fresh; PID alive → soft fail; user upgraded pr9k → state file carries `pr9kVersion` for compatibility checks).

### Sub-decisions

- **Two stamps**: `worktreeStamp` (baked into worktree path + branch on creation, never changes) and `invocationStamp` (generated fresh each pr9k start, used for log file path and per-step artifact dir). The state file records `worktreeStamp`. The TUI shows both: "Worktree pr9k/<worktreeStamp>, this run <invocationStamp>".
- **State-file location**: `<primary>/.pr9k/active-run.json`. Atomic-written. Removed on graceful completion.
- **State-file fields**: `schema_version` (int), `worktreeStamp` (string), `worktreePath` (string, absolute), `branchName` (string), `pid` (int), `startedAt` (RFC3339), `pr9kVersion` (string).
- **Concurrent-runs**: PID liveness test. If alive → exit with clear message "another pr9k appears to be running for this primary checkout (PID N)." If dead → assume crash; resume.
- **Schema-version handling**: state file carries `schema_version`. If the loaded version is unknown to the running pr9k (downgrade scenario), refuse to resume and ask the user to abandon via `--fresh`.
- **Abandon affordance**: `pr9k --fresh` (CLI flag) — at startup, deletes the state file and (optionally) removes the prior worktree. Document in the how-to. Also document `pr9k worktree prune` as the bulk-cleanup equivalent.
- **Default behavior**: resume is automatic (the user's stated requirement). No CLI flag to *enable* resume. Only the *opt-out* (`--fresh`).
- **`iteration.jsonl` across resume**: the file inside the worktree accumulates across pr9k invocations. Each invocation also appends a "session boundary" record (`{type: "session_start", invocationStamp: ..., resumedFrom: ...}`) so post-run analysis can trace which records belong to which invocation.
- **D15 reversal**: D15 (no cross-run resume) is reversed; replace with a new decision documenting the resume mechanism.

### What changes in the existing spec

- **D8 (config shape)**: no change — `worktrees: { enabled, autoCleanup }` is correct as-is. Resume is implicit when `enabled: true`. No new config knob needed (per E11, we avoid the four-file lockstep cost).
- **D13 (autoCleanup)**: clarify that `autoCleanup: true` removes the state file along with the worktree+branch on graceful end. `autoCleanup: false` removes the state file but leaves the worktree+branch.
- **D15 (no cross-run resume)**: REVERSED. Replace with new decision describing the state-file mechanism and resume semantics. Update the "A prior run was killed mid-iteration" alternate flow to describe the auto-resume behavior.
- **Background section**: update the "Runs do not coordinate" paragraph to "When `worktrees.enabled: true`, runs that are killed mid-flight resume automatically on the next invocation."

### Integration plan (file by file)

Implementation-level — the spec's "Planned Fix" section. (Not implemented; planning only.)

| File | Change | Evidence |
|---|---|---|
| `src/internal/runstate/` (new package) | `WriteActive(path, state) error`, `ReadActive(path) (state, error)`, `RemoveActive(path) error`, `IsAlive(pid int) bool`. Uses `atomicwrite.Write`. | E8, E9 |
| `src/internal/runstate/state.go` | Define `ActiveRunState` struct with the schema fields above. JSON tags. Schema-version constant. | E11 (schema discipline) |
| `src/cmd/pr9k/main.go` (between `cli.Execute` and `startup`) | New step: read `<projectDir>/.pr9k/active-run.json`. If exists and worktree exists → set up `RunConfig.ResumedFrom = state`. If exists but worktree missing → log warning, remove state file, proceed fresh. If absent → proceed fresh. If `--fresh` flag → delete state file (and optionally worktree) before this step. | E10, E12 |
| `src/cmd/pr9k/main.go` (during startup, after worktree resolved) | If not resumed: write state file. If resumed: skip write. | E5, E10 |
| `src/cmd/pr9k/main.go` (after `Run` returns gracefully) | Remove state file. (autoCleanup-aware: remove worktree+branch too if `autoCleanup: true`.) | E5 |
| `src/internal/cli/args.go` | Add `--fresh` boolean flag. Add to `Config`. | E12 |
| `src/internal/workflow/iterationlog.go` | Optionally: append session-boundary record at run start. Schema bump from 1 to 2 (or extend optionally). | E3 |
| `src/internal/ui/header.go` or `model.go` | Append "RESUMED FROM <worktreeStamp>" suffix to iteration line when `RunConfig.ResumedFrom != nil`. | E13 |
| `docs/code-packages/runstate.md` (new) | Document the new package per project convention. | inferred |
| `docs/features/cross-run-resume.md` (new) — OR fold into worktree feature doc | User-facing doc. | inferred |
| `docs/how-to/recovering-from-pr9k-crashes.md` (new) | Step-by-step how to recover from a crash, including `--fresh` and `pr9k worktree prune`. | inferred |
| `feature-specification.md` | Update D15, Background, "A prior run was killed mid-iteration" alternate flow. | This investigation |
| `decision-log.md` | New decision D16 documenting the resume mechanism; update D13 to mention state-file removal; reverse D15. | This investigation |

## Validation

(To be filled in by adversarial-validator pass.)

## Final Summary

- **Root cause**: D15 of the existing spec declared cross-run resume out of scope on YAGNI grounds. The user has now provided explicit need; D15 is reversed.
- **Fix**: Add a small state file (`<primary>/.pr9k/active-run.json`) written at run start and removed at graceful end. State-file presence is the deterministic resume marker. Combine with the existing stamped-worktree mechanism (D1, D2). No iteration-loop changes needed because `get_next_issue` is naturally idempotent (E1, E14).
- **Why correct**: The simplest mechanism that satisfies all six hard requirements without forcing the user to abandon worktree inspection (`autoCleanup: false`). Builds on existing patterns (atomicwrite, crash-temp PID liveness check). Single insertion point in `main.go`. The behavioral analyst's preference for Option B was based on assuming `autoCleanup: true` is mandated; with `autoCleanup: false` preserved as a first-class option, A is necessary.
- **Validation outcome**: To be filled in.
- **Remaining risks**: To be filled in.
