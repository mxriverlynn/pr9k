# Implementation Decision Log: useWorktrees

This file records every implementation decision committed while planning the
`useWorktrees` feature. Behavioral and implementation statements live in
[../feature-implementation-plan.md](../feature-implementation-plan.md) — this
file captures the question, rationale, evidence, and rejected alternatives for
each decision. Round-by-round history lives in
[implementation-iteration-history.md](implementation-iteration-history.md).

The spec's own decision log ([decision-log.md](decision-log.md)) records D1–D19
of behavioral commitments (worktree location, branch naming, log durability,
autoCleanup behavior, the `worktrees` config shape, the active-run state file,
the `--fresh` flag, etc.). This implementation log starts from those as
inherited givens and only captures decisions about the *implementation shape*
inside pr9k's existing Go codebase.

## Trivial decisions

- D-16: Concurrent-run "PID N" message is exactly the spec-committed line — no worktree path, start time, or uptime. — Referenced in plan: "External Interfaces", "Definition of Done".
- D-17: Iteration-log size cap / rotation — none. The lessons-learned step's truncation at end-of-run plus worktree removal is the bound. — Referenced in plan: "Operational Readiness", "Deferred (YAGNI)".

## Full decisions

### D-1: State-file lifecycle placement

- **Question:** Where in the entry-point pipeline does the active-run state-file lifecycle (read, worktree decision, write, remove) execute?
- **Decision:** All four phases of the state-file lifecycle live in `src/cmd/pr9k/main.go` (the `main()` body), not inside `startup()`. `startup()` keeps its single-call shape and signature; `main()` reads the state file before `startup()`, decides resume vs fresh, computes the effective `projectDir` (worktree path on enabled-true paths, primary on enabled-false), passes it into `startup()`, writes the state file after `preflight.Run` returns, and removes the state file after `workflow.Run` returns based on the `ExitReason`.
- **Rationale:** The narrow-reading ADR ([`docs/adr/20260410170952-narrow-reading-principle.md`](../../../adr/20260410170952-narrow-reading-principle.md)) says workflow content lives in `config.json` and pr9k must remain a generic step runner. Concentrating worktree-aware logic in `main()` keeps the runner subsystems (`startup`, `workflow.Run`) ignorant of the worktrees feature; they accept whatever `projectDir` they are given. This also matches the codebase's existing `main.go` shape ([`src/cmd/pr9k/main.go`](../../../../src/cmd/pr9k/main.go)) where `main()` already orchestrates phases and `startup()` is a single helper. Splitting `startup()` into phases would force a circular dependency between worktree decision and logger creation.
- **Evidence:** ADR-20260410170952 (narrow-reading); `src/cmd/pr9k/main.go` shape; iteration-history L25, L26.
- **Rejected alternatives:**
  - Split `startup()` into `startupPhase1` / `startupPhase2` so the worktree decision runs between them — rejected because it widens the public surface of an internal helper for one feature, and the same goal is reachable by leaving `startup()` alone and letting `main()` plumb the chosen `projectDir`.
  - Widen `startup()`'s signature with a `primaryDir` parameter and have it own the state-file lifecycle — rejected because it pulls feature-specific knowledge into a generic helper, violating the narrow-reading ADR.
- **Specialist owner:** `behavioral-analyst` (with `junior-developer` reframing).
- **Revisit criterion:** A second feature also needs a "decide projectDir before startup" hook; in that case, extract the pattern instead of building a one-off into `startup()`.
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** D-3, D-4, D-7.
- **Referenced in plan:** "Architecture and Integration Points", "Runtime Behavior", "Decomposition and Sequencing" (work units 6, 7).

### D-2: `RunResult.ExitReason` enum and main.go consumption

- **Question:** How does `main()` know whether to remove the state file and run autoCleanup? `workflow.Run`'s current `RunResult` has no exit-classification field, and `main.go` discards the value.
- **Decision:** Add a typed exit-reason value to `RunResult`. The four return sites in `src/internal/workflow/run.go` emit one of `Completed | LoopBroken | UserQuit`: natural completion → `Completed`; `breakLoopIfEmpty` exit → `LoopBroken`; user-quit early returns at the three phase boundaries → `UserQuit`. `main()` consumes `result.ExitReason` to decide: `Completed | LoopBroken` → remove state file (and run autoCleanup if `worktrees.autoCleanup: true`); `UserQuit` → leave state file in place so the next start auto-resumes.
- **Rationale:** The spec ([D16](decision-log.md#d16-runresult-exitreason)) commits to the field; the plan implements it. A boolean `Completed` field is insufficient because `LoopBroken` (no more issues to process) is a successful exit that should remove the state file but distinct from "all configured iterations finished" — future telemetry or test assertions may want to distinguish the two. An enum keeps the decision space open without recompiling consumers. Driving cleanup from `main()` (rather than inside `Run`) honors the narrow-reading ADR and keeps `Run` testable without filesystem effects.
- **Evidence:** `src/cmd/pr9k/main.go:298` (`_ = workflow.Run(...)`); `src/internal/workflow/run.go:614-617` (breakLoopIfEmpty fall-through); spec [D16](decision-log.md#d16-runresult-exitreason); iteration-history L2, L28.
- **Rejected alternatives:**
  - Boolean `Completed bool` field — rejected because it collapses two semantically distinct successful exits and does not extend.
  - Have `workflow.Run` perform state-file removal and autoCleanup itself — rejected because it pulls feature-specific cleanup into the generic runner (narrow-reading ADR) and makes `Run`'s tests filesystem-dependent.
- **Specialist owner:** `behavioral-analyst`.
- **Revisit criterion:** A new exit class is needed (e.g., timeout-driven exit, fatal-error exit) — extend the enum, do not rework the field shape.
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** D-3.
- **Referenced in plan:** "Runtime Behavior", "Decomposition and Sequencing" (work unit 3).

### D-3: autoCleanup execution location

- **Question:** Who runs the branch + worktree removal when `worktrees.autoCleanup: true` and the run completed cleanly?
- **Decision:** `main()` runs autoCleanup after `workflow.Run` returns, gated on `result.ExitReason ∈ {Completed, LoopBroken}` and on the `worktrees.autoCleanup` config bit. Removal order is the spec-committed branch → worktree → state file (per [D13](decision-log.md#d13-autocleanup-behavior) of the spec). The `workflow.Run` body and the runner subsystems do not know about autoCleanup.
- **Rationale:** Same narrow-reading argument as D-1 / D-2. The spec orders the removal steps; the implementation just lifts that ordering out of `main()` and points it at small helpers in the worktree-lifecycle module (D-10's siblings: `git worktree remove --force` + `git branch -D`).
- **Evidence:** ADR-20260410170952; spec [D13](decision-log.md#d13-autocleanup-behavior); iteration-history L7, L28.
- **Rejected alternatives:**
  - In-`Run` autoCleanup — see D-2 rejection.
  - Background goroutine that fires-and-forgets the cleanup after `main()` returns — rejected because it makes ordering observable to subsequent invocations (a second `pr9k` could start before the cleanup finished and find a half-cleaned worktree); synchronous cleanup gives spec-stable ordering.
- **Specialist owner:** `behavioral-analyst`.
- **Revisit criterion:** Worktree removal becomes slow enough that synchronous cleanup degrades the user experience; in that case, gate it on a measurement.
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** —
- **Referenced in plan:** "Runtime Behavior", "Decomposition and Sequencing" (work unit 7).

### D-4: Worktree-stamp generator surface

- **Question:** How does `main()` mint the worktree-stamp before any logger has been created?
- **Decision:** Export `logger.FormatStamp(t time.Time) string` from `src/internal/logger/`. `main()` calls `logger.FormatStamp(time.Now())` to mint the worktree-stamp before `startup()` runs. The invocation-stamp continues to come from the logger that `startup()` constructs — `logger.NewLogger().RunStamp()` — and is therefore generated slightly later in the lifecycle, which is the intended semantic separation between the two stamps.
- **Rationale:** The format string already exists inside `internal/logger`; promoting it from a private constant to an exported helper is a one-line change that keeps the format definition single-source-of-truth. Creating a second logger instance just to mint the worktree-stamp would either spawn a second log file or require partial-construction APIs the package does not have.
- **Evidence:** `src/internal/logger/` format-string location; spec stamp-format definition; iteration-history L24.
- **Rejected alternatives:**
  - Inline the format string in `main.go` — rejected because it duplicates the format literal and breaks the "logger owns timestamp formatting" invariant; future format changes would have to be applied in two places.
  - Construct a second `*Logger` just to call `RunStamp` — rejected because it has filesystem side effects (creates a log file).
  - Move the stamp generator into a new `internal/stamp` package — rejected on YAGNI grounds (single use, single line).
- **Specialist owner:** `junior-developer`.
- **Revisit criterion:** A third call site needs the stamp format; extract to its own package.
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** D-1.
- **Referenced in plan:** "Architecture and Integration Points", "Decomposition and Sequencing" (work unit 4).

### D-5: Binary-path identity check mechanism

- **Question:** The spec's resume-validation set requires a "PID alive AND binary path matches pr9k's binary" check ([T4](feature-technical-notes.md#t4-active-run-state-file-schema)). What helper performs the binary-path lookup, and what does it do on platforms where the lookup is unsupported or fails?
- **Decision:** For the running binary, use `os.Executable()` followed by `filepath.EvalSymlinks` to canonicalize. For an arbitrary live PID, read `/proc/<pid>/exe` (Linux, no CGo) or shell out to `ps -p <pid> -o comm=` (macOS, no CGo) and `EvalSymlinks` the result. Compare the two canonical paths. Any error reading the live PID's binary path falls through to **"treat as dead → resume"** (this also resolves the L9 TOCTOU concern: process-exit between `syscall.Kill(pid, 0)` and the binary-path read shows up as a read error, which is treated as the process being gone). Windows is out of scope.
- **Rationale:** No CGo precedent exists in the codebase; introducing CGo for one platform-specific call (`proc_pidpath` on macOS) would change the build footprint. `/proc/<pid>/exe` and `ps` cover the only two supported platforms (macOS, Linux). The "treat as dead → resume" fallthrough preserves the spec's "no silent destruction" invariant: a live process whose binary cannot be identified is conservatively assumed to be a different binary that PID-reused, which means the state file is not destroyed by `--fresh` and the resuming run is the legitimate continuation.
- **Evidence:** No CGo in `src/`; [`docs/coding-standards/go-patterns.md`](../../../coding-standards/go-patterns.md) symlink-resolution rule; spec [D10](decision-log.md#d10-concurrent-runs), [T4](feature-technical-notes.md#t4-active-run-state-file-schema); iteration-history L9, L27.
- **Rejected alternatives:**
  - CGo `proc_pidpath` on macOS — rejected because no CGo precedent and the build footprint cost outweighs the benefit.
  - PID-only liveness (skip binary-path comparison) — rejected because it permits PID-reuse to falsely identify an unrelated process as the prior pr9k, which violates the spec's resume-validation set.
  - Treat any read error as "still running, refuse" — rejected because a hard read error blocks resume forever, which violates the spec's auto-resume guarantee.
- **Specialist owner:** `junior-developer`.
- **Revisit criterion:** Windows support requested; or platform-specific test failures; or an incident where PID-reuse-by-unrelated-pr9k-binary slips past the check.
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** D-7, D-8.
- **Referenced in plan:** "Runtime Behavior", "Decomposition and Sequencing" (work unit 6), "Testing Strategy".

### D-6: ENOENT on state-file removal is benign

- **Question:** What does the state-file remover do if the file is already gone when it tries to `os.Remove`?
- **Decision:** Wrap the `os.Remove` in `if err != nil && !os.IsNotExist(err)` and treat `ENOENT` as a no-op success. Other errors propagate.
- **Rationale:** The spec's TOCTOU row (Edge Cases table) commits to this behavior implicitly — a concurrent prune or `--fresh` may remove the state file between the running pr9k's read and its remove call. ENOENT means the desired postcondition is already met; other errors (EACCES, EIO) are real failures.
- **Evidence:** Spec Edge Cases TOCTOU row; iteration-history L8.
- **Rejected alternatives:**
  - Propagate every removal error — rejected because it surfaces a benign race as a failure to the user.
  - Best-effort silent removal — rejected because it hides EACCES / EIO that need user attention.
- **Specialist owner:** `concurrency-analyst`.
- **Revisit criterion:** A new error class (e.g., EBUSY on a network filesystem) needs distinct handling.
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** —
- **Referenced in plan:** "Runtime Behavior".

### D-7: State-file removal compares worktreeStamp before deleting

- **Question:** In the rare two-instance race, what prevents one instance from removing the *other* instance's state file?
- **Decision:** Before `os.Remove`, the remover reads the state file's `worktreeStamp` and compares it to the running invocation's worktreeStamp. On match, remove. On mismatch, log a warning and skip the removal. ENOENT during the read is benign (file already gone — D-6 path).
- **Rationale:** The spec invariant "Local commits must never be silently discarded" extends to "the state file pointing at a worktree owned by a live pr9k must never be silently discarded by an unrelated invocation." The spec's TOCTOU row acknowledges the race exists but is silent on the implementation rule that closes it; this decision documents the rule. The check is cheap (one open + parse), and the failure mode of skipping is preferable to the failure mode of cross-deletion.
- **Evidence:** Spec invariants; spec Edge Cases TOCTOU row; iteration-history L10.
- **Rejected alternatives:**
  - Unconditional removal — rejected because in the documented two-instance race it can destroy state belonging to the live winner.
  - Take a flock on the state file before removal — rejected on YAGNI grounds (single-user workstation; flock semantics differ across filesystems and add complexity for a race the stamp-equality check already closes).
- **Specialist owner:** `behavioral-analyst`.
- **Revisit criterion:** A measured incident shows stamp-equality is insufficient (e.g., clock skew across containers makes stamps non-unique).
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** —
- **Referenced in plan:** "Runtime Behavior", "Testing Strategy".

### D-8: Resume-validation set evaluation order

- **Question:** The spec's resume-validation set has four conditions (worktree exists, primaryPath equality, branch exists, process not alive). In what order are they checked, and which condition's failure triggers the rename-and-fresh path?
- **Decision:** Process-liveness ([T4](feature-technical-notes.md#t4-active-run-state-file-schema) condition 4) is evaluated first. If the recorded PID is alive AND its binary matches pr9k's, the run refuses with the concurrent-run error (per [D10](decision-log.md#d10-concurrent-runs) of the spec). Only when the recorded process is dead (or PID-reuse-by-unrelated-binary) does pr9k evaluate the remaining three conditions; failure of any of them triggers the documented stale-state rename-with-`.stale-<timestamp>` and proceed-fresh path.
- **Rationale:** A live owning process must never have its state file renamed out from under it. Any order that evaluates a stale-trigger before liveness creates a window where the live owner's state is renamed and a second worktree is created on the same primary, violating the "single-active-worktree per primary" invariant.
- **Evidence:** Spec invariant "single-active-worktree per primary"; [T4](feature-technical-notes.md#t4-active-run-state-file-schema); iteration-history L4.
- **Rejected alternatives:**
  - Any order that allows stale-rename to fire before liveness check — see rationale.
  - Evaluate all four conditions in parallel and take a union — rejected because it does not give a deterministic decision when both "stale" and "live" appear simultaneously (e.g., recorded primaryPath drifts but PID still matches an alive pr9k).
- **Specialist owner:** `behavioral-analyst`.
- **Revisit criterion:** A new resume-validation condition is added; reassess ordering.
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** —
- **Referenced in plan:** "Runtime Behavior", "Testing Strategy".

### D-9: invocation_stamp injection layer

- **Question:** Where is `invocation_stamp` set on each `IterationRecord` so that all records carry it without per-call-site additions?
- **Decision:** Set the field exactly once, inside the `newIterationRecord` constructor in `src/internal/workflow/iterationlog.go`. The eleven existing call sites all flow through this constructor (verified: `run.go:417, 437, 656, 698` and the related sites). The struct field is JSON-encoded with `omitempty` so older records on disk continue to round-trip unchanged when read by the lessons-learned and post_issue_summary consumers.
- **Rationale:** Single-layer injection is the spec's [T5](feature-technical-notes.md#t5-iterationrecord-invocation-stamp) §4 commitment; this decision verifies there are no bypass paths in the current codebase that would skip the constructor. `omitempty` lets old records (written by pre-feature pr9k builds) read cleanly without a migration step; the consumers tolerate the field's absence.
- **Evidence:** `src/internal/workflow/run.go:417, 437, 656, 698`; `src/internal/workflow/iterationlog.go:32`; spec [T5](feature-technical-notes.md#t5-iterationrecord-invocation-stamp); iteration-history L5.
- **Rejected alternatives:**
  - Set the field per call site — rejected because it requires touching every call site and is brittle to future record-construction additions.
  - Wrap `IterationRecord` in a new "stamped record" type — rejected because it doubles the schema surface for a single field.
- **Specialist owner:** `behavioral-analyst`.
- **Revisit criterion:** A new record-construction path bypasses `newIterationRecord`; in that case, route it back through the constructor.
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** —
- **Referenced in plan:** "Data Model and Persistence", "Decomposition and Sequencing" (work unit 8).

### D-10: `pr9k worktree prune` placement and shape

- **Question:** Where does the `pr9k worktree prune` subcommand live, and how does it execute its git operations?
- **Decision:** Inline in `src/cmd/pr9k/worktree.go` following the precedent set by `sandbox_create.go` and friends — no new `internal/worktree` package. The subcommand declares its own local `--project-dir` flag (no cobra `PersistentFlags` on the root) and resolves it with the same `EvalSymlinks` pattern used by `cli/args.go:resolveProjectDir`. Git operations (`git worktree list --porcelain`, `git worktree remove --force`, `git branch -D`) execute via direct `exec.Command` rather than `Runner.RunStep` (which is TUI-coupled).
- **Rationale:** `sandbox` is the only existing multi-subcommand group in the codebase, and its pattern is inline files in `src/cmd/pr9k/` — a new package would add no organizational value at the size this code lives at (~100 lines). A local `--project-dir` flag is simpler than promoting the root flag to persistent, and matches the existing per-subcommand convention. `Runner.RunStep` carries TUI streaming machinery the prune subcommand does not need; `exec.Command` is the same primitive the sandbox subcommands and `realDockerRun` use directly.
- **Evidence:** `src/cmd/pr9k/sandbox_create.go`, `sandbox_interactive.go`, `sandbox_shell.go` precedent; `src/internal/cli/args.go:resolveProjectDir`; iteration-history L15, L29, L30.
- **Rejected alternatives:**
  - New `internal/worktree` package — rejected on YAGNI (single caller, single concern; no second subcommand on the horizon needs the package).
  - Cobra `PersistentFlags` on the root — rejected because it changes a public CLI surface for one subcommand's benefit.
  - `Runner.RunStep` — rejected because it couples the subcommand to the TUI run-loop machinery for no benefit.
- **Specialist owner:** `devops-engineer` (with `junior-developer` reframing).
- **Revisit criterion:** A second worktree-related subcommand (e.g., `pr9k worktree list`, `pr9k worktree adopt`) lands; extract a package then.
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** —
- **Referenced in plan:** "External Interfaces", "Decomposition and Sequencing" (work unit 10).

### D-11: `git_push` rewrite shape

- **Question:** What does the rewritten `workflow/scripts/git_push` look like, and does it apply only when `worktrees.enabled: true` or in both modes?
- **Decision:** A 3-line bash script: `set -euo pipefail` then `git push --set-upstream origin "$(git rev-parse --abbrev-ref HEAD)"`. The `--set-upstream` flag is idempotent — the first push of a fresh branch establishes the upstream, and subsequent pushes succeed without modification. The script applies in **both** `worktrees.enabled: true` and `enabled: false` modes (per the resolved OI-1: the spec's prerequisite push fix is universally beneficial, not worktree-specific).
- **Rationale:** The current script (`trap 'exit 0' EXIT; result=$(git push); echo $result; exit 0`) suppresses every non-zero exit, which means a push failure on a fresh branch (no upstream) reaches the workflow as success and the run's commits never leave the machine. The 3-line form surfaces the failure to the workflow runner so c/r/q error mode triggers. Doing the rewrite in both modes (per OI-1 resolution) avoids a bifurcated script and gives the same protection to non-worktree users.
- **Evidence:** `workflow/scripts/git_push:1-8`; spec resolved OI-1; spec [T3](feature-technical-notes.md#t3-default-workflow-prerequisite); `workflow/scripts/review_verdict` precedent for shape; iteration-history L1, L31.
- **Rejected alternatives:**
  - Go-side `git_push` wrapper that establishes the upstream itself — rejected because the simpler-version test passes: 3 lines of bash with the `--set-upstream` flag is idempotent and sufficient. A Go wrapper would add a binary-side commitment for behavior the shell already expresses.
  - A flag added to the existing script to toggle "set upstream on first push" — rejected because `--set-upstream` is already idempotent; the flag would be no-op machinery.
  - Pre-push tracking-ref check that conditionally adds `-u` — rejected for the same reason.
- **Specialist owner:** `devops-engineer`.
- **Revisit criterion:** A custom-workflow user reports the rewrite breaks their wrapper assumption (low likelihood — workflow scripts are not part of pr9k's public API per [`docs/coding-standards/versioning.md`](../../../coding-standards/versioning.md)).
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** —
- **Referenced in plan:** "External Interfaces", "Decomposition and Sequencing" (work unit 1).

### D-12: Three-file coordinated `worktrees`-block schema change

- **Question:** What is the minimum coordinated set of file changes required to add the `worktrees` block to `config.json` without breaking the validator or the workflow-builder TUI's round-trip?
- **Decision:** The `worktrees` block is added simultaneously to three files in a single deliverable:
  1. `src/internal/validator/validator.go:vFile` — adds the block to the schema and a `vWorktrees` validator that rejects `autoCleanup: true` without `enabled: true`.
  2. `src/internal/workflowmodel/scaffold.go:rawConfig` — adds the block to the parse target.
  3. `src/internal/workflowio/marshal.go:outConfig` — adds the block to the outbound JSON shape so the workflow-builder TUI's save path round-trips it.
  A round-trip test asserts the block survives parse → save unchanged.
- **Rationale:** The validator uses `DisallowUnknownFields` ([`src/internal/validator/validator.go:111-119`](../../../../src/internal/validator/validator.go)), so a real user adding the `worktrees` block without all three changes sees a fatal validator error. Splitting the change across PRs would yield a window where `config.json` files using the new block are rejected. The workflow-builder TUI's save path (`src/internal/workflowio/marshal.go`) must include the field or users editing through `pr9k workflow` would silently drop their `worktrees` block on save.
- **Evidence:** `src/internal/validator/validator.go:111-119` (`DisallowUnknownFields`); `src/internal/workflowmodel/scaffold.go:72-80`; iteration-history L3.
- **Rejected alternatives:**
  - Incremental rollout (add to validator first, then scaffold, then marshal in separate PRs) — rejected because the intermediate states are user-facing breakage; nothing depends on the changes being staged.
- **Specialist owner:** `behavioral-analyst` (with `devops-engineer` and `junior-developer`).
- **Revisit criterion:** A larger-scale schema-evolution policy is adopted that staggers schema changes across versions.
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** —
- **Referenced in plan:** "Data Model and Persistence", "Decomposition and Sequencing" (work unit 2), "Testing Strategy".

### D-13: Documentation pack ships with the feature

- **Question:** Which documentation files must land in the same PR as the implementation?
- **Decision:** Five documentation deliverables:
  1. `docs/how-to/using-worktrees.md` — turning the feature on, daily use, branch-protection prerequisite, lessons-learned truncation note.
  2. `docs/how-to/managing-worktrees.md` — `pr9k worktree prune`, `--dry-run`, `--fresh`, manual cleanup.
  3. A feature doc — either a new `docs/features/worktree-mode.md` or an extension to [`docs/features/workflow-orchestration.md`](../../../features/workflow-orchestration.md). The plan keeps this open and lets the implementer pick based on whether the worktree-mode content fits cleanly inside the existing orchestration page.
  4. `docs/code-packages/workflow.md` updates — `RunResult.ExitReason`, `IterationRecord.invocation_stamp`, the new state-file lifecycle.
  5. `CLAUDE.md` index entries for the new how-to and feature files (per [`docs/coding-standards/documentation.md`](../../../coding-standards/documentation.md)).
- **Rationale:** The documentation standard says feature docs ship with the feature, not as follow-ups; one topic per how-to keeps the navigation clean. Multiple existing how-tos (e.g., `setting-up-docker-sandbox.md`, `configuring-defaults.md`) follow the one-topic-per-file precedent. Splitting "using" from "managing" mirrors that precedent — daily-use vs administrative-action.
- **Evidence:** [`docs/coding-standards/documentation.md`](../../../coding-standards/documentation.md); existing how-to corpus in [`docs/how-to/`](../../../how-to); iteration-history L21.
- **Rejected alternatives:**
  - Single combined how-to — rejected because it mixes daily-use ergonomics with administrative cleanup in one navigation entry.
  - Defer doc updates to a follow-up PR — rejected because it violates the documentation standard.
- **Specialist owner:** `devops-engineer` (with `junior-developer`).
- **Revisit criterion:** —
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** —
- **Referenced in plan:** "Decomposition and Sequencing" (work unit 12), "Definition of Done".

### D-14: Version bump category — MINOR

- **Question:** Is this a PATCH or MINOR version bump?
- **Decision:** MINOR — `0.8.1` → `0.9.0`. Bumped in `src/internal/version/version.go`.
- **Rationale:** [`docs/coding-standards/versioning.md`](../../../coding-standards/versioning.md) lists pr9k's public API as the CLI flags, the `config.json` schema, the `{{VAR}}` language, and the `--version` output. This release adds (a) a new top-level `config.json` block (`worktrees`), (b) a new CLI flag (`--fresh`), (c) a new subcommand group (`pr9k worktree prune`), and (d) a user-visible behavior change in the default workflow's push step. Under the 0.y.z rules in the standard, the combination is a MINOR bump.
- **Evidence:** [`docs/coding-standards/versioning.md`](../../../coding-standards/versioning.md); iteration-history L23.
- **Rejected alternatives:**
  - PATCH (`0.8.1` → `0.8.2`) — rejected because the public-API surface adds; a PATCH bump would understate the change.
- **Specialist owner:** `devops-engineer`.
- **Revisit criterion:** —
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** —
- **Referenced in plan:** "Decomposition and Sequencing" (work unit 13), "Definition of Done".

### D-15: Schema-version mismatch handling — rename-and-warn, no migration framework

- **Question:** What does pr9k do when it reads an active-run state file whose `schemaVersion` it does not recognize?
- **Decision:** Rename the file with a `.incompatible-<timestamp>` suffix, log a warning, and proceed as if no state file existed. No migration framework is introduced; the path is unit-tested.
- **Rationale:** No downgrade scenario has been reported. A migration framework is a substantial commitment for a single field that the spec explicitly numbers `schemaVersion: 1`. Renaming preserves the user's data (the file is not destroyed) and the warning surfaces the situation. Unit tests on the rename path are the right granularity.
- **Evidence:** Spec [T4](feature-technical-notes.md#t4-active-run-state-file-schema); iteration-history L20.
- **Rejected alternatives:**
  - Build a migration framework now — rejected on YAGNI.
  - CI schema-change detector / state-file fixture — rejected on YAGNI (defer to "real downgrade scenario causes data loss"); listed in Deferred (YAGNI).
  - Destructive deletion of incompatible file — rejected because it violates the "no silent destruction" invariant.
- **Specialist owner:** `devops-engineer`.
- **Revisit criterion:** A real downgrade scenario causes data loss in practice.
- **Dissent (if any):** None.
- **Driven by rounds:** R1.
- **Dependent decisions:** —
- **Referenced in plan:** "Data Model and Persistence", "Testing Strategy".
