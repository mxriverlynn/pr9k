# Feature Implementation Plan: useWorktrees (with auto-resume)

This plan implements the spec at [feature-specification.md](feature-specification.md). The feature ships as a single MINOR release (`0.8.1` → `0.9.0`) under the existing release process: a new `worktrees` block in `config.json` (default off), a new `pr9k worktree prune` subcommand, a `--fresh` root-command flag, an `ExitReason` field on `RunResult`, an `invocation_stamp` field on `IterationRecord`, an active-run state file at `<primary>/.pr9k/active-run.json`, and the prerequisite `git_push` rewrite plus iteration-step reorder. The implementation is decomposed into 13 sequential work units; nothing is gated behind a feature flag beyond the `worktrees.enabled` config bit (the bit IS the rollout).

## Source Specification

- **Feature specification:** [feature-specification.md](feature-specification.md)
- **Specification decision log:** [artifacts/decision-log.md](artifacts/decision-log.md)
- **Specification team findings:** [artifacts/team-findings.md](artifacts/team-findings.md)
- **Specification technical notes:** [artifacts/feature-technical-notes.md](artifacts/feature-technical-notes.md)
- **Specification decisions this plan inherits:** D1–D19 (worktree location, branch naming, log durability, autoCleanup behavior, push-and-step-order prerequisite, default-and-config shape, single-active-worktree per primary, concurrent-runs PID+binary check, no per-step bookmarks, autoCleanup ordering, `pr9k worktree prune` subcommand, cross-run resume via state file, `RunResult.ExitReason`, per-worktree iteration log, push-before-close ordering, `--fresh` flag).
- **Specification technical notes this plan inherits:** T1 (sequencing constraint — workflow-bundle resolution before worktree exists), T2 (sandbox bind-mount uses worktree path), T3 (default-workflow prerequisite), T4 (active-run state file schema), T5 (`IterationRecord.invocation_stamp`).
- **Specification open items resolved before this plan:** OI-1 (push fix applies in both modes), OI-2 (review_verdict reads `.pr9k/artifacts/code-review.md`).

## Outcome

When this plan is executed, the following exists in code and on disk:

- A `worktrees` object in `workflow/config.json` accepting `enabled: bool` and `autoCleanup: bool`, parsed by the validator, scaffold, and outbound marshaller in lockstep ([D-12](artifacts/implementation-decision-log.md#d-12-three-file-coordinated-worktrees-block-schema-change)).
- An `ExitReason` typed value on `workflow.RunResult` with the four cases `Completed`, `LoopBroken`, `UserQuit` ([D-2](artifacts/implementation-decision-log.md#d-2-runresultexitreason-enum-and-maingo-consumption)).
- An `invocation_stamp` field on `IterationRecord`, set inside `newIterationRecord` so all eleven existing call sites populate it ([D-9](artifacts/implementation-decision-log.md#d-9-invocation_stamp-injection-layer)).
- An active-run state file at `<primary>/.pr9k/active-run.json` written via `atomicwrite.Write` after preflight succeeds and removed by `main()` after `workflow.Run` returns based on `ExitReason` ([D-1](artifacts/implementation-decision-log.md#d-1-state-file-lifecycle-placement), [D-7](artifacts/implementation-decision-log.md#d-7-state-file-removal-compares-worktreestamp-before-deleting)).
- An exported `logger.FormatStamp(t time.Time) string` so the worktree-stamp can be minted before logger construction ([D-4](artifacts/implementation-decision-log.md#d-4-worktree-stamp-generator-surface)).
- A `pr9k worktree` subcommand group with `prune` and `prune --dry-run`, implemented as inline `src/cmd/pr9k/worktree.go` driving `git` via direct `exec.Command` ([D-10](artifacts/implementation-decision-log.md#d-10-pr9k-worktree-prune-placement-and-shape)).
- A `--fresh` flag on the root command interacting with the resume-validation set ([D-8](artifacts/implementation-decision-log.md#d-8-resume-validation-set-evaluation-order)).
- A 3-line `workflow/scripts/git_push` rewrite that propagates non-zero exits and sets upstream on first push, plus the `Close issue` / `Git push` step swap in the default workflow, plus the `review_verdict` path fix ([D-11](artifacts/implementation-decision-log.md#d-11-git_push-rewrite-shape)).
- TUI surface changes: iteration-line basename prefix and "(resumed)" suffix; final-summary extension with worktree path, branch, and worktree count.
- A documentation pack of five deliverables ([D-13](artifacts/implementation-decision-log.md#d-13-documentation-pack-ships-with-the-feature)).
- `version.Version` bumped to `0.9.0` ([D-14](artifacts/implementation-decision-log.md#d-14-version-bump-category--minor)).

## Context

- **Driving constraint:** A single developer running pr9k on their workstation needs the primary checkout to remain usable for parallel work (running the app, opening a feature branch in their editor, pairing) while pr9k iterates on the next issue. Without worktree mode, pr9k clobbers the primary's branch and working tree. The auto-resume requirement comes from real-world usage where pr9k runs unattended overnight: a kill (terminal closed, machine restarted, OOM) must not require manual recovery.
- **Stakeholders:** the pr9k user (developer running pr9k against their own repo). No other stakeholders — pr9k is a single-user workstation tool.
- **Future-state concerns:**
  1. The iteration-log growth bound is *conditional* on the lessons-learned step running and truncating; a workflow that quits before lessons-learned, repeatedly, leaves an unbounded `iteration.jsonl` in the worktree. Watched, not capped, until a real measurement justifies a cap (deferred under YAGNI).
  2. The PID-reuse soft-lock is a *soft* guarantee: PID-reuse-by-an-unrelated-pr9k-binary on the same workstation could theoretically pass the binary-path check. The binary-path comparison closes the common case; pathological cases remain deferred ([D-5](artifacts/implementation-decision-log.md#d-5-binary-path-identity-check-mechanism)).
  3. The `git_push` rewrite changes user-visible push behavior (errors now propagate). Workflow scripts are not part of pr9k's public API per [`docs/coding-standards/versioning.md`](../coding-standards/versioning.md), but a custom-workflow user wrapping the script may need to update their wrapper.
- **Out-of-scope boundary:**
  - Windows support for the binary-path identity check.
  - Configurable worktree location (worktrees are always sibling-of-primary per spec [D1](artifacts/decision-log.md#d1-worktree-location)).
  - Per-step bookmarks (resume always re-enters at the start of an iteration).
  - A per-iteration `issue_id` filter on `post_issue_summary`'s jq pipeline (cross-issue contamination on resumed runs is an accepted limitation per spec [D17](artifacts/decision-log.md#d17-per-worktree-iteration-log)).

## Team Composition and Participation

| Specialist | Status | Key Input |
|---|---|---|
| `project-manager` | Coordinator | Facilitated R1 and synthesized this plan, decision log, and YAGNI ledger. |
| `junior-developer` | Active | Twelve plain-language Open Questions reframing the spec's silences into implementation choices: logger stamp surface (L24), state-file lifecycle placement (L25), worktree-stamp threading (L26), binary-path lookup mechanism (L27), autoCleanup execution location (L28), prune subcommand `--project-dir` resolution (L29), prune git shell-out pattern (L30); all resolved by codebase precedent in R1. |
| `behavioral-analyst` | Active | Runtime data flow, state-file lifecycle, RunResult.ExitReason plumbing, invocation_stamp injection layer, cleanup ordering crash-resilience, resume-validation set ordering, projectDir redirect (L2, L4–L7, L10, L25–L28, L31). |
| `concurrency-analyst` | Active | TOCTOU on state-file create and remove, PID + binary-path identity check ordering, cleanup-vs-statusline-runner shutdown, signal-handler timing, AppendIterationRecord POSIX atomicity, prune-vs-active-run race (L8–L14). |
| `devops-engineer` | Active | `git_push` script change shape, `pr9k worktree prune` placement, observability YAGNI boundary, autoCleanup ergonomics, branch-protection prerequisites, schema-version handling, documentation pack, soft-lock UX, version bump category, `git_push` mode-applicability (L1, L3, L15–L23, L31). |
| `adversarial-security-analyst` | Stood down — not engaged | The feature operates entirely under the local-user trust boundary (filesystem reads/writes inside the user's repo and home hierarchy; PID and binary-path checks observe local processes only). No new attack surface for remote callers, no new authorization decisions, no PII handling. See "Security Posture" below. |
| `system-architect`, `software-architect`, `structural-analyst`, `risk-analyst`, `user-experience-designer`, `information-architect`, `test-engineer`, `edge-case-explorer`, `gap-analyzer`, `content-auditor`, `evidence-based-investigator`, `adversarial-validator` | Not engaged | The plan's questions resolved cleanly via codebase precedent in R1; no specialist named one of these as a needed handoff for an unsettled finding. Several appear in "Specialist Handoffs for Implementation" below as dispatch points during implementation, not for plan-stage input. |

Round-by-round detail in [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md).

## Implementation Approach

### Architecture and Integration Points

The feature plugs into pr9k's existing entry-point pipeline without restructuring it. The changes concentrate in `src/cmd/pr9k/main.go`; the workflow runner subsystems (`internal/workflow`, `internal/preflight`, `internal/sandbox`, `internal/logger`) accept new inputs but remain ignorant of the worktrees feature itself, in keeping with the narrow-reading ADR ([`docs/adr/20260410170952-narrow-reading-principle.md`](../adr/20260410170952-narrow-reading-principle.md)) and [D-1](artifacts/implementation-decision-log.md#d-1-state-file-lifecycle-placement).

**Touch points** (file:line references from [.discovery-notes.md](artifacts/.discovery-notes.md)):

- `src/cmd/pr9k/main.go` — the active-run state-file lifecycle (read, decide, write, remove) lands here, around the existing `cli.Execute` → `startup` → `workflow.Run` → cleanup sequence ([D-1](artifacts/implementation-decision-log.md#d-1-state-file-lifecycle-placement), [D-3](artifacts/implementation-decision-log.md#d-3-autocleanup-execution-location)).
- `src/internal/cli/args.go` — adds the `--fresh` boolean to the root command's flag set; `resolveProjectDir` is unchanged.
- `src/cmd/pr9k/worktree.go` — new file holding the `worktree` subcommand group and `prune [--dry-run]` ([D-10](artifacts/implementation-decision-log.md#d-10-pr9k-worktree-prune-placement-and-shape)).
- `src/internal/workflow/run.go` — emits `ExitReason` at four return sites (`run.go:460`, `run.go:589`, `run.go:722`, `run.go:749`, plus the `breakLoopIfEmpty` exit near `run.go:614-617`) ([D-2](artifacts/implementation-decision-log.md#d-2-runresultexitreason-enum-and-maingo-consumption)).
- `src/internal/workflow/iterationlog.go` — adds the `invocation_stamp` field to `IterationRecord` and sets it in `newIterationRecord` ([D-9](artifacts/implementation-decision-log.md#d-9-invocation_stamp-injection-layer)).
- `src/internal/workflow/workflow.go` — `NewRunner(log, projectDir)` is unchanged; the worktree path flows in transparently as the `projectDir` argument.
- `src/internal/preflight/run.go` — unchanged. The state-file writer creates `<primary>/.pr9k/` independently because `preflight.Run` only knows about the worktree path.
- `src/internal/logger/` — exports `FormatStamp(t time.Time) string` ([D-4](artifacts/implementation-decision-log.md#d-4-worktree-stamp-generator-surface)).
- `src/internal/validator/validator.go`, `src/internal/workflowmodel/scaffold.go`, `src/internal/workflowio/marshal.go` — coordinated three-file schema change for the `worktrees` block ([D-12](artifacts/implementation-decision-log.md#d-12-three-file-coordinated-worktrees-block-schema-change)).
- `src/internal/atomicwrite/write.go` — reused as-is for the state-file write (atomic temp+rename, parent-dir fsync, EXDEV/ENOSPC/EACCES classification).
- `src/internal/ui/header.go`, `src/internal/ui/log.go` — iteration-line prefix/suffix and final-summary extension.
- `src/internal/version/version.go` — `0.8.1` → `0.9.0` ([D-14](artifacts/implementation-decision-log.md#d-14-version-bump-category--minor)).
- `workflow/config.json`, `workflow/scripts/git_push`, `workflow/scripts/review_verdict` — prerequisites ([D-11](artifacts/implementation-decision-log.md#d-11-git_push-rewrite-shape)).

A small worktree-lifecycle helper (private, file-scoped or in `src/cmd/pr9k/worktree.go`) wraps `git worktree add`, `git worktree list --porcelain`, `git worktree remove --force`, and `git branch -D`. Direct `exec.Command` per [D-10](artifacts/implementation-decision-log.md#d-10-pr9k-worktree-prune-placement-and-shape).

### Data Model and Persistence

**`config.json` `worktrees` block.** Coordinated three-file change ([D-12](artifacts/implementation-decision-log.md#d-12-three-file-coordinated-worktrees-block-schema-change)). The schema is the spec's:

```json
{
  "worktrees": {
    "enabled": false,
    "autoCleanup": false
  }
}
```

The validator rejects `autoCleanup: true` without `enabled: true`. Round-trip test asserts the block survives parse → save through the workflow-builder TUI's path.

**Active-run state file (`<primary>/.pr9k/active-run.json`).** Schema per spec [T4](artifacts/feature-technical-notes.md#t4-active-run-state-file-schema): `schemaVersion: 1`, `worktreeStamp`, `worktreePath`, `primaryPath`, `branch`, `pid`, `binary`. Written via `atomicwrite.Write` (per [`docs/coding-standards/file-writes.md`](../coding-standards/file-writes.md)). The writer creates `<primary>/.pr9k/` itself before the atomic write because `preflight.Run`'s `MkdirAll` runs against the worktree path under the redirect, not the primary.

Schema-version handling: an unknown `schemaVersion` triggers `<file>.incompatible-<timestamp>` rename and a fresh-start ([D-15](artifacts/implementation-decision-log.md#d-15-schema-version-mismatch-handling--rename-and-warn-no-migration-framework)). A corrupted parse triggers `<file>.corrupted-<timestamp>` rename and a fresh-start (per spec Edge Cases).

**`IterationRecord.invocation_stamp`.** New string field, JSON-encoded with `omitempty`. Set inside `newIterationRecord` ([D-9](artifacts/implementation-decision-log.md#d-9-invocation_stamp-injection-layer)). Default-workflow consumers (`post_issue_summary`, `lessons-learned`) tolerate the field's absence on records written by pre-feature builds.

**`workflow.RunResult.ExitReason`.** New typed value (`Completed | LoopBroken | UserQuit`); existing `IterationsRun` field is unchanged ([D-2](artifacts/implementation-decision-log.md#d-2-runresultexitreason-enum-and-maingo-consumption)).

### Runtime Behavior

The runtime call shape, in order, in `main()`:

1. `cli.Execute()` — parses flags, including the new `--fresh`.
2. Read the active-run state file at `<primary>/.pr9k/active-run.json` (read regardless of `worktrees.enabled` per spec).
3. Decide: resume / refuse-as-concurrent / fresh-start / `--fresh` cleanup. Resume-validation evaluation order is process-liveness first, then primaryPath / worktree-existence / branch-existence ([D-8](artifacts/implementation-decision-log.md#d-8-resume-validation-set-evaluation-order)). Process-liveness uses `syscall.Kill(pid, 0)` + binary-path comparison per [D-5](artifacts/implementation-decision-log.md#d-5-binary-path-identity-check-mechanism); any binary-path read error → "treat as dead → resume."
4. Mint the worktree-stamp via `logger.FormatStamp(time.Now())` ([D-4](artifacts/implementation-decision-log.md#d-4-worktree-stamp-generator-surface)) — only on the fresh-start path; on resume the stamp comes from the state file.
5. On the fresh-start path, `git worktree add` + `git checkout -b pr9k-<worktree-stamp>`. On the resume path, no git operations — just plumb the recorded path.
6. Call `startup(workflowDir, projectDir)` where `projectDir` is the worktree path. `startup` runs the validator, `preflight.Run`, opens the logger, returns the `*workflow.Runner`.
7. **After preflight succeeds and before launching the workflow goroutine**, write the state file via `atomicwrite.Write`. The write happens here (not earlier) so a preflight failure does not leave a state file pointing at an unusable worktree.
8. Write the log header lines (worktree path, branch, "RESUMED FROM" if applicable).
9. `signal.Notify` for SIGINT/SIGTERM; launch the workflow goroutine; await `workflow.Run`'s return.
10. On return: classify `result.ExitReason`. `Completed | LoopBroken` → remove state file (with worktreeStamp equality check per [D-7](artifacts/implementation-decision-log.md#d-7-state-file-removal-compares-worktreestamp-before-deleting); ENOENT benign per [D-6](artifacts/implementation-decision-log.md#d-6-enoent-on-state-file-removal-is-benign)) → if `worktrees.autoCleanup: true`, run `git branch -D` then `git worktree remove --force` (in that spec-committed order). `UserQuit` → leave state file in place; emit no autoCleanup.

The TUI continues to receive its iteration line and final summary from the existing `internal/ui` surfaces; the worktree-mode additions are: a basename prefix (the worktree directory name) on the iteration line plus a "(resumed)" suffix when the run is a resume; the final summary extends with worktree path, branch, and worktree count (`git worktree list` count of `pr9k-*` entries).

### External Interfaces

**CLI surfaces added:**

- `pr9k --fresh` — root-command boolean flag. Interacts with the resume-validation set per [D-8](artifacts/implementation-decision-log.md#d-8-resume-validation-set-evaluation-order): refuses with the spec-committed concurrent-run message ([D-16](artifacts/implementation-decision-log.md#trivial-decisions)) when the recorded process is alive; otherwise removes worktree + branch + state file and proceeds fresh.
- `pr9k worktree prune` — bulk remove all `pr9k-*` worktrees on the resolved primary except the active one (state file's `worktreePath`) and except the CWD-containing one (walk-up disambiguation against the result of `git worktree list --porcelain`). Reports uncommitted-files counts before removal.
- `pr9k worktree prune --dry-run` — same set without removing.
- Both subcommand variants declare a local `--project-dir` flag, resolved with the same `EvalSymlinks` pattern as `cli/args.go:resolveProjectDir` ([D-10](artifacts/implementation-decision-log.md#d-10-pr9k-worktree-prune-placement-and-shape)).

**Default-workflow scripts (touched as prerequisites, [D-11](artifacts/implementation-decision-log.md#d-11-git_push-rewrite-shape)):**

- `workflow/scripts/git_push` — replaced with a 3-line bash that propagates non-zero exits and uses `git push --set-upstream origin "$(git rev-parse --abbrev-ref HEAD)"`. Applies in both worktree and in-place modes.
- `workflow/scripts/review_verdict` — `REVIEW_FILE=".pr9k/artifacts/code-review.md"` (one-line fix per resolved OI-2).
- `workflow/config.json` — iteration phase reorders `Git push` to run before `Close issue` (per spec [D18](artifacts/decision-log.md#d18-push-before-close-step-ordering)).

**Concurrent-run message (verbatim):** "another pr9k appears to be running for this primary checkout (PID N)". No additional context — see [D-16](artifacts/implementation-decision-log.md#trivial-decisions).

## Decomposition and Sequencing

| # | Work Unit | Delivers | Depends On | Verification |
|---|---|---|---|---|
| 1 | **Default-workflow prerequisites** ([D-11](artifacts/implementation-decision-log.md#d-11-git_push-rewrite-shape)) | `git_push` 3-line rewrite; `review_verdict` path fix; `config.json` `Close issue`/`Git push` step swap | — | Run the existing default workflow on a sample issue (no worktree config); confirm a push failure propagates to the c/r/q error mode; confirm review_verdict reads the artifact path; confirm push runs before close on a SIGKILL-mid-iteration scenario. |
| 2 | **`worktrees` schema (coordinated three-file change)** ([D-12](artifacts/implementation-decision-log.md#d-12-three-file-coordinated-worktrees-block-schema-change)) | `vFile` validator entry + `vWorktrees`; `rawConfig` parse target; `outConfig` outbound shape | — | Unit test: a `config.json` with the `worktrees` block parses, validates, and round-trips through `workflowio.Save` unchanged. Negative test: `autoCleanup: true` without `enabled: true` is rejected. |
| 3 | **`RunResult.ExitReason`** ([D-2](artifacts/implementation-decision-log.md#d-2-runresultexitreason-enum-and-maingo-consumption)) | New typed field; emit at four return sites + `breakLoopIfEmpty` exit | — | Unit tests covering each `ExitReason` value at its emit site. Race-detector run (`go test -race`) per [`docs/coding-standards/testing.md`](../coding-standards/testing.md). |
| 4 | **`logger.FormatStamp` export** ([D-4](artifacts/implementation-decision-log.md#d-4-worktree-stamp-generator-surface)) | Promote internal format constant to exported `FormatStamp(t time.Time) string` | — | Unit test asserts format is `pr9k-YYYY-MM-DD-HHMMSS.mmm`. |
| 5 | **Worktree-lifecycle helpers** ([D-10](artifacts/implementation-decision-log.md#d-10-pr9k-worktree-prune-placement-and-shape)) | Private helpers wrapping `git worktree add/list/remove` + `git branch -D` via `exec.Command`; porcelain parser | — | Unit test on the porcelain parser; integration test against a temp git repo creates/lists/removes a worktree. |
| 6 | **Active-run state file** ([D-1](artifacts/implementation-decision-log.md#d-1-state-file-lifecycle-placement), [D-5](artifacts/implementation-decision-log.md#d-5-binary-path-identity-check-mechanism), [D-6](artifacts/implementation-decision-log.md#d-6-enoent-on-state-file-removal-is-benign), [D-7](artifacts/implementation-decision-log.md#d-7-state-file-removal-compares-worktreestamp-before-deleting), [D-8](artifacts/implementation-decision-log.md#d-8-resume-validation-set-evaluation-order), [D-15](artifacts/implementation-decision-log.md#d-15-schema-version-mismatch-handling--rename-and-warn-no-migration-framework)) | State-file struct; reader/writer using `atomicwrite.Write`; PID + binary-path identity helper; resume-validation function with the documented evaluation order; corruption / schema-mismatch rename paths; ENOENT-benign + worktreeStamp-equality guards on the remover | 4 | Unit tests on each rename path (corrupted, incompatible, stale-trigger). Race test on identity check fall-through. Atomic-write reuse test. Negative test: removal with mismatched stamp is a no-op. |
| 7 | **`main()` rewiring** ([D-1](artifacts/implementation-decision-log.md#d-1-state-file-lifecycle-placement), [D-3](artifacts/implementation-decision-log.md#d-3-autocleanup-execution-location)) | State-file lifecycle in `main()`; log header lines; autoCleanup gating on `ExitReason` + config | 2, 3, 5, 6 | Integration test: run pr9k with `worktrees.enabled: true`, kill it, re-run, confirm auto-resume into same worktree on same branch via the iteration-log header line. Assert state file contents match the recorded values. |
| 8 | **`IterationRecord.invocation_stamp`** ([D-9](artifacts/implementation-decision-log.md#d-9-invocation_stamp-injection-layer)) | Field added; `newIterationRecord` sets it; `omitempty` on the JSON tag | — | Unit test on every `newIterationRecord` call site. Round-trip test confirms records written by an older build still parse cleanly. |
| 9 | **TUI surfaces** | Iteration-line basename prefix and "(resumed)" suffix; final-summary extension with worktree path + branch + worktree count | 7 | Visual regression / golden-file test per [`docs/coding-standards/tui-rendering.md`](../coding-standards/tui-rendering.md). |
| 10 | **`pr9k worktree prune` subcommand** ([D-10](artifacts/implementation-decision-log.md#d-10-pr9k-worktree-prune-placement-and-shape)) | Inline `src/cmd/pr9k/worktree.go`; `prune` + `--dry-run`; walk-up disambiguation; uncommitted-files counts | 5 | Integration test: temp repo with two `pr9k-*` worktrees and one unrelated worktree; `prune` removes both pr9k worktrees, leaves the unrelated one, refuses to remove the active state file's worktree, refuses to remove the CWD-containing worktree. `--dry-run` produces same set without removing. |
| 11 | **`--fresh` flag** ([D-8](artifacts/implementation-decision-log.md#d-8-resume-validation-set-evaluation-order), [D-16](artifacts/implementation-decision-log.md#trivial-decisions)) | Flag definition; concurrent-run guard interaction; force-removal path; "Discarded prior run" feedback | 6 | Integration tests: `--fresh` while live → spec-committed concurrent-run message; `--fresh` with a dead-PID state file → cleanup proceeds; `--fresh` with no state file → no-op + proceed fresh. |
| 12 | **Documentation pack** ([D-13](artifacts/implementation-decision-log.md#d-13-documentation-pack-ships-with-the-feature)) | `docs/how-to/using-worktrees.md`; `docs/how-to/managing-worktrees.md`; feature doc (new or extension); `docs/code-packages/workflow.md` updates; `CLAUDE.md` index entries | 11 | Manual review against [`docs/coding-standards/documentation.md`](../coding-standards/documentation.md). Verify CLAUDE.md links resolve. |
| 13 | **Version bump** ([D-14](artifacts/implementation-decision-log.md#d-14-version-bump-category--minor)) | `src/internal/version/version.go` → `0.9.0` | 1–12 | `pr9k --version` prints `0.9.0`; `make ci` clean. |

## RAID Log

### Risks

| ID | Risk | Likelihood | Severity | Blast Radius | Reversibility | Owner | Mitigation |
|---|---|---|---|---|---|---|---|
| R1 | The coordinated three-file schema change is split across PRs, leaving an intermediate state where `config.json` files using `worktrees` are rejected by the validator | Low | High | Every user adopting the feature | Easy (revert) | `behavioral-analyst` (during impl) | Treat work unit 2 as a single deliverable; round-trip test included; reviewer enforces atomicity ([D-12](artifacts/implementation-decision-log.md#d-12-three-file-coordinated-worktrees-block-schema-change)) |
| R2 | Binary-path identity helper has no codebase precedent; platform-specific code (`/proc/<pid>/exe` vs `ps`) may behave differently than expected on a fresh macOS or Linux install | Medium | Medium | Resume correctness on first deploy | Easy (the "treat as dead → resume" fallthrough is conservative) | `junior-developer` (during impl) | Platform-tagged unit tests; the fallthrough rule ([D-5](artifacts/implementation-decision-log.md#d-5-binary-path-identity-check-mechanism)) means failures degrade to "create a new worktree" rather than data loss |
| R3 | `iteration.jsonl` cross-issue contamination on resumed runs leaks via `post_issue_summary`'s un-filtered jq pipeline | Accepted | Low | Issue-summary readability | N/A (accepted) | — | Documented as accepted limitation in `using-worktrees.md` per spec [D17](artifacts/decision-log.md#d17-per-worktree-iteration-log) and ledger entry L33 |
| R4 | The `git_push` rewrite breaks a custom workflow that wraps the script and depends on the trap-to-zero behavior | Low | Low | Custom-workflow users only | Easy (the user's wrapper can be re-pinned) | `devops-engineer` (during impl) | Workflow scripts are not part of pr9k's public API per [`docs/coding-standards/versioning.md`](../coding-standards/versioning.md); the change is documented in the release notes; the MINOR bump signals the surface change ([D-14](artifacts/implementation-decision-log.md#d-14-version-bump-category--minor)) |

### Assumptions

| ID | Assumption | What Changes If Wrong | Verifier | Status |
|---|---|---|---|---|
| A1 | The user's git is recent enough to support `git worktree add/list/remove --force` (git ≥ 2.17 covers all flags used) | Worktree operations fail at runtime with a git error | Documented prerequisite in `using-worktrees.md` ([D-13](artifacts/implementation-decision-log.md#d-13-documentation-pack-ships-with-the-feature)) | Documented |
| A2 | `os.Executable()` returns the actual binary on macOS and Linux for the running pr9k (not a wrapper script) | Binary-path identity check yields false negatives | `os.Executable` codebase precedent + manual test on dev machines | Confirmed by precedent |
| A3 | Workflow-config files are not also being concurrently written by the workflow-builder TUI while `pr9k` is running | The round-trip test would not catch a concurrent-write race | `workflow-builder save durability` ADR — atomic temp+rename + mtime conflict detection | Already covered by ADR-20260424120000 |

### Issues

| ID | Issue | Owner | Next Step |
|---|---|---|---|
| — | None active at synthesis time | — | — |

### Dependencies

| ID | Dependency | Owner | Status |
|---|---|---|---|
| Dep1 | Default workflow's three prerequisite changes (work unit 1) must land before any worktree-mode integration test runs end-to-end | implementation owner | Sequenced as work unit 1 |

## Testing Strategy

The test plan follows [`docs/coding-standards/testing.md`](../coding-standards/testing.md): race detector mandatory, observable-behavior framing, test-helper path resolution. Tests use the existing `internal/` package structure with `_test.go` neighbors; new integration tests against a temp git repo follow the precedent of `internal/workflowio/crashtemp_test.go`.

- **Observable behaviors to test:**
  1. `worktrees.enabled: true` + fresh run → worktree created at sibling path; state file written; iteration log header records worktree path + branch.
  2. SIGKILL during iteration + re-invoke → auto-resume into same worktree on same branch; iteration log appends across invocations; "RESUMED FROM" header line emitted.
  3. `q` + `y` graceful quit → state file remains on disk; next invocation auto-resumes.
  4. `worktrees.autoCleanup: true` on `Completed` → branch removed → worktree removed → state file removed (in that order).
  5. `pr9k worktree prune` removes all `pr9k-*` worktrees except active one and CWD-containing one.
  6. `pr9k --fresh` while live → spec-committed concurrent-run message; non-zero exit.
  7. `pr9k --fresh` with dead PID → cleanup + fresh start.
  8. Validator rejects `autoCleanup: true` without `enabled: true`.

- **Test doubles posture:** No mocks for `exec.Command` git invocations — integration tests use a real temp git repo (faster and more reliable than mocking porcelain output). Stub `os.Executable` in the binary-path identity tests via a small adapter so platform-specific behavior is testable cross-platform.

- **Edge cases requiring coverage** (sourced from spec Edge Cases table):
  - State-file corruption rename (`.corrupted-<timestamp>`).
  - Schema-version mismatch rename (`.incompatible-<timestamp>`) ([D-15](artifacts/implementation-decision-log.md#d-15-schema-version-mismatch-handling--rename-and-warn-no-migration-framework)).
  - primaryPath drift (macOS `/var` vs `/private/var`) — `EvalSymlinks` comparison must canonicalize per [`docs/coding-standards/go-patterns.md`](../coding-standards/go-patterns.md).
  - Branch deleted mid-resume → resume-validation set's branch-existence check fails → stale-rename → fresh-start.
  - Prune from inside a worktree: walk-up disambiguation to the primary's `git worktree list`.
  - Prune on a non-`pr9k-`-named worktree whose dir name happens to suffix-match: branch-name check (must match `pr9k-<stamp>` regex) gates removal.
  - TOCTOU stamp-equality on state-file removal: two-instance race; the loser logs and skips ([D-7](artifacts/implementation-decision-log.md#d-7-state-file-removal-compares-worktreestamp-before-deleting)).
  - ENOENT on state-file removal: benign no-op ([D-6](artifacts/implementation-decision-log.md#d-6-enoent-on-state-file-removal-is-benign)).
  - PID-reuse-by-unrelated-binary: binary-path comparison detects mismatch → "treat as dead → resume" path.
  - Resume-validation evaluation order: live process + drifted primaryPath → refuse-as-concurrent (not stale-rename) ([D-8](artifacts/implementation-decision-log.md#d-8-resume-validation-set-evaluation-order)).

- **Test levels:**
  - **Unit:** validator/scaffold/marshal round-trip; `logger.FormatStamp` shape; `RunResult.ExitReason` emit sites; porcelain parser; PID + binary-path identity helper (with stubbed `os.Executable`); state-file rename paths; `newIterationRecord` `invocation_stamp` setter.
  - **Integration:** temp-git-repo worktree lifecycle; `main()` rewiring across SIGKILL/resume; `pr9k worktree prune` walk-up + filtering; `--fresh` flag interactions.
  - **End-to-end:** `make ci` clean; `pr9k --version` prints `0.9.0`.

## Security Posture

**No security specialist was engaged.** The feature operates entirely under the local-user trust boundary:

- Filesystem reads/writes are all inside the user's repository hierarchy and the user's home directory (the existing pr9k profile-dir + `.pr9k/` umbrella).
- PID and binary-path checks observe only local processes the running user can already see.
- No new authorization decisions, no PII handling, no remote-callable surface, no new network endpoints, no new credentials.
- The `git push` rewrite still pushes via the user's existing git credentials — no new credential surface.

No new attack surface for remote callers. If the workstation-trust model changes (e.g., pr9k is moved to a multi-tenant or shared-host environment), `adversarial-security-analyst` should be engaged to re-evaluate the state-file location, the binary-path identity check, and the worktree path readability.

## Operational Readiness

- **Observability:** the four spec-committed surfaces and no extras (per [D-16](artifacts/implementation-decision-log.md#trivial-decisions) and the YAGNI ledger):
  1. Iteration-line basename prefix in the TUI.
  2. "(resumed)" suffix on resumed runs.
  3. Final-summary extension (worktree path, branch, worktree count).
  4. Iteration-log header lines including "RESUMED FROM" on resume.
- **SLO impact:** N/A — pr9k is a workstation tool, not a service.
- **Feature flag:** the `worktrees.enabled` config bit IS the rollout flag. Default is `false`. Users opt in by editing `config.json`.
- **Rollout:** ship as the `0.9.0` release; users adopt at their own pace by adding the `worktrees` block.
- **Rollback:** users remove the `worktrees` block from `config.json`; pr9k reverts to in-place behavior on the next invocation. The state file is read regardless of `worktrees.enabled` so a user who removes the block while a state file exists is warned (one-line) and proceeds in-place — the state file is not modified.
- **Cost and scale:** N/A — local workstation; no compute, storage, egress, or third-party cost change.

## Definition of Done

- [ ] `git_push` exits non-zero on push failure; first push of a fresh branch succeeds without manual intervention (verified against a test repo) ([D-11](artifacts/implementation-decision-log.md#d-11-git_push-rewrite-shape)).
- [ ] `worktrees.enabled: true` + a fresh run creates a worktree, writes the state file, and runs the workflow against the worktree path (verified via the iteration log header and the worktree directory).
- [ ] Killing the run (SIGKILL) and re-invoking pr9k auto-resumes into the same worktree on the same branch (verified via the "RESUMED FROM" log line and `iteration.jsonl` appending across invocations).
- [ ] `q`+`y` graceful quit keeps the state file; next invocation auto-resumes ([D-2](artifacts/implementation-decision-log.md#d-2-runresultexitreason-enum-and-maingo-consumption)).
- [ ] `worktrees.autoCleanup: true` on a `Completed` run removes the branch, worktree, and state file in that order (verified via log records) ([D-3](artifacts/implementation-decision-log.md#d-3-autocleanup-execution-location)).
- [ ] `pr9k worktree prune` removes all `pr9k-*` worktrees except the active one and the CWD-containing one; `--dry-run` shows the same set without removing ([D-10](artifacts/implementation-decision-log.md#d-10-pr9k-worktree-prune-placement-and-shape)).
- [ ] `pr9k --fresh` removes the prior worktree, branch, and state file when the recorded process is dead; refuses with the spec-committed concurrent-run message when alive ([D-8](artifacts/implementation-decision-log.md#d-8-resume-validation-set-evaluation-order), [D-16](artifacts/implementation-decision-log.md#trivial-decisions)).
- [ ] Validator rejects `worktrees.autoCleanup: true` without `worktrees.enabled: true` ([D-12](artifacts/implementation-decision-log.md#d-12-three-file-coordinated-worktrees-block-schema-change)).
- [ ] `IterationRecord.invocation_stamp` is set on every record produced by this build; `omitempty` keeps older records readable ([D-9](artifacts/implementation-decision-log.md#d-9-invocation_stamp-injection-layer)).
- [ ] State-file removal is ENOENT-benign and worktreeStamp-equality-guarded ([D-6](artifacts/implementation-decision-log.md#d-6-enoent-on-state-file-removal-is-benign), [D-7](artifacts/implementation-decision-log.md#d-7-state-file-removal-compares-worktreestamp-before-deleting)).
- [ ] All tests pass under `make ci` (race detector + lint + format + vet + vulncheck + mod-tidy + build).
- [ ] Documentation pack ([D-13](artifacts/implementation-decision-log.md#d-13-documentation-pack-ships-with-the-feature)) lands in the same PR as the code.
- [ ] Version bumped to `0.9.0` in `src/internal/version/version.go` ([D-14](artifacts/implementation-decision-log.md#d-14-version-bump-category--minor)).
- [ ] `CLAUDE.md` updated with index entries for the new how-to and feature files (per [`docs/coding-standards/documentation.md`](../coding-standards/documentation.md)).

## Specialist Handoffs for Implementation

- **`test-engineer`** — dispatch when writing the unit-test suite for the active-run state-file rename paths (corrupted, incompatible, stale-trigger) and the resume-validation evaluation order. Needs the spec's Edge Cases table and [D-5](artifacts/implementation-decision-log.md#d-5-binary-path-identity-check-mechanism) / [D-7](artifacts/implementation-decision-log.md#d-7-state-file-removal-compares-worktreestamp-before-deleting) / [D-8](artifacts/implementation-decision-log.md#d-8-resume-validation-set-evaluation-order) as input.
- **`evidence-based-investigator`** — dispatch if a `git_push` regression appears in CI on a non-default repo (e.g., a custom branch-protection setup the dev team has). Needs the verbatim `git push` error and the failing repo's branch-protection rules.
- **`adversarial-validator`** — dispatch to spot-check the implementation plan once a draft PR is up, against the four/five SIGKILL crash windows enumerated in the spec's Edge Cases table.

## Deferred (YAGNI)

### New `internal/worktree` package for the prune subcommand
- **Why deferred:** Simpler-version test — the codebase precedent (`sandbox_create.go` and friends in `src/cmd/pr9k/`) is inline subcommand files, not a new package. Single-implementation interface anti-pattern: only one subcommand area calls into the worktree-lifecycle helpers, and they fit in `src/cmd/pr9k/worktree.go` at ~100 lines.
- **Reopen when:** a second worktree-related subcommand (e.g., `pr9k worktree list`, `pr9k worktree adopt`) lands; extract a package then.
- **Source:** R1, `devops-engineer` (L15), `junior-developer` (L29).

### Per-record `worktree_path` field on every `IterationRecord`
- **Why deferred:** Evidence test — no consumer needs the path on every record. The `invocation_stamp` field already lets a reader correlate records to invocations, and the worktree path is constant for the worktree's lifetime. Adding a field with no consumer is the named "configuration knob no caller sets" anti-pattern.
- **Reopen when:** a user reports being unable to attribute records to worktrees from `invocation_stamp` alone.
- **Source:** R1, `devops-engineer` (L16).

### Stale-worktree count metric / structured event
- **Why deferred:** Evidence test — no telemetry destination, and the TUI plus log file already carry the count. Named "observability for systems whose telemetry isn't reaching the destination yet" anti-pattern.
- **Reopen when:** a user reports needing structured stale-worktree visibility (e.g., for cross-machine fleet view).
- **Source:** R1, `devops-engineer` (L16).

### `worktreeStamp` / `primaryDir` field on the statusline payload
- **Why deferred:** Evidence test — no consumer; spec [D12](artifacts/decision-log.md#d12-status-line-payload) already deferred `primaryDir`, and `worktreeStamp` has no script reading it.
- **Reopen when:** a user-built status-line script needs the worktree identity.
- **Source:** R1, `devops-engineer` (L16).

### Iteration-log size cap / rotation
- **Why deferred:** Evidence test — no measured incident. Lessons-learned truncation at end-of-run plus worktree removal are the bound. Named "runbook for never-fired alert" anti-pattern.
- **Reopen when:** a user reports the file growing past a noticeable size on real hardware.
- **Source:** R1, `devops-engineer` (L17).

### Cleanup-recovery-on-next-startup machinery
- **Why deferred:** Evidence test — orphan handlers in spec's Primary Flow step 3 and the [D5](artifacts/decision-log.md#d5-stale-state-detection) stale detection cover all crash windows.
- **Reopen when:** a user reports a partial-cleanup recovery gap not covered by orphan handlers.
- **Source:** R1, `devops-engineer` (L18).

### `branchPrefix` config option
- **Why deferred:** Evidence test — no user request. Spec [D2](artifacts/decision-log.md#d2-branch-naming-for-the-run) commits to `pr9k-` as a discoverability convention, not a user-tunable knob.
- **Reopen when:** a user reports a branch-protection conflict on `pr9k-*` they cannot resolve at the remote.
- **Source:** R1, `devops-engineer` (L19).

### Pre-flight check that interrogates GitHub branch-protection rules
- **Why deferred:** Evidence test — the verbatim git error from a failed push is more informative than parsed protection rules and requires no GitHub API call. The branch-protection prerequisite is documented in `using-worktrees.md`.
- **Reopen when:** a user reports being unable to diagnose a branch-protection rejection from the git error alone.
- **Source:** R1, `devops-engineer` (L19).

### CI schema-change detector / state-file fixture
- **Why deferred:** Evidence test — no downgrade scenario reported. The rename-and-warn path ([D-15](artifacts/implementation-decision-log.md#d-15-schema-version-mismatch-handling--rename-and-warn-no-migration-framework)) is unit-tested.
- **Reopen when:** a real schema-version mismatch causes data loss in practice.
- **Source:** R1, `devops-engineer` (L20).

### Richer concurrent-run UX
- **Why deferred:** Evidence test — single-user-workstation context; the user knows their own running process. The spec-committed "PID N" message is sufficient ([D-16](artifacts/implementation-decision-log.md#trivial-decisions)).
- **Reopen when:** a user reports the PID-only message is insufficient (e.g., they cannot find the process).
- **Source:** R1, `devops-engineer` (L22).

## Open Items

None. All Open Questions raised in R1 resolved by codebase evidence and project precedent in a single round; spec-maturity gate did not trip; OI-1 and OI-2 from the spec stage were resolved before synthesis.

## Summary

- **Outcome delivered:** worktree-mode + auto-resume + prune subcommand + `--fresh` flag, shipping under the `worktrees` config block (default off) with the prerequisite default-workflow push fix.
- **Team size:** 5 (PM + junior-developer + behavioral-analyst + concurrency-analyst + devops-engineer) — see [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md).
- **Rounds of facilitation:** 1 — see [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md).
- **Decisions committed:** 17 (15 full, 2 trivial) — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Decisions settled by evidence:** 17 — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Decisions settled by junior-developer reframing:** 0 (junior-developer findings drove evidence-based resolution rather than reframing-as-decision-source).
- **Decisions settled by user input:** 0 — all by codebase evidence and project precedent.
- **Rejected alternatives recorded:** 30+ across the 15 full decisions — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md).
- **Open items remaining:** 0.
- **Recommendation:** **Ship as planned.** No specialist handoff blocks ship. The 13 work units in the Decomposition table can be implemented sequentially; work unit 1 (default-workflow prerequisites) must land first because the integration tests for later units depend on it.
