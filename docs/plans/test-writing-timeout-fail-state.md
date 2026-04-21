# Investigation: "Test writing" step forces unattended user into error-mode prompt on timeout

The 900s `timeoutSeconds` on the "Test writing" step is routinely exceeded by productive work, and when the timer fires the orchestrator enters a blocking `c / r / q` prompt — which defeats unattended pr9k runs.

## Problem Statement

- **Symptom:** The screenshot shows the "Test writing" step marked `[x]` (failed) with the `c continue  r retry  q quit` shortcut bar visible at the footer. Prior log body shows Claude was mid-work — writing tests, running the suite, fixing a handful of TP-numbered failures (TP-003, TP-006, TP-015, TP-026). The iteration log (`~/dev/gearjot/gearjot-v2-web/.pr9k/iteration.jsonl`) records the exit as `"status":"failed","duration_s":0,"notes":"timed out after 900s"` — the failure is a wall-clock timeout, not a test-runner non-zero exit. The user misread the symptom as "test runner failed"; the real cause is the 900s per-step cap.
- **Expected behavior:** When pr9k is run unattended (its whole purpose), a per-step timeout on the "Test writing" step should not block waiting on a human. Either the step should be given enough wall-clock budget to finish the documented prompt work, or the timeout should be a soft fail that logs-and-moves-on.
- **Conditions:** Occurs on the "Test writing" step (the only step in the shipped workflow with a non-zero `timeoutSeconds`). Correlates with large prior-phase token counts (`Feature work` output > ~20k tokens) and/or iterations where `.ralph-cache/` Go-toolchain layout needs fixup. Observed in both `gearjot-v2` and `gearjot-v2-web` repos, across multiple days of runs.
- **Impact:** Defeats the unattended design of pr9k for every issue where Test writing runs long. The user either sits at the terminal to press `r` / `c`, or comes back to find a hung TUI that made no further progress overnight. A retry from a fresh session also re-burns 15+ minutes of Claude time because `--resume` is blocked by the G2 and G5 resume gates after a timeout.

## Evidence Summary

### E1: `timeoutSeconds` is declared in the step schema

- **Source:** `src/internal/steps/steps.go:41`, `src/internal/validator/validator.go:88`, `:450-456`
- **Finding:**
  ```go
  // steps.go:41
  TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
  // validator.go:446-456
  if step.TimeoutSeconds != nil {
      if *step.TimeoutSeconds <= 0 {
          *errs = append(*errs, at("schema", "timeoutSeconds must be a positive integer when set"))
      } else if *step.TimeoutSeconds > 86400 {
          *errs = append(*errs, at("schema", "timeoutSeconds must not exceed 86400 (24 hours)"))
      }
  }
  ```
- **Relevance:** Top of the plumbing chain. No sibling policy field (`onTimeout`, `continueOnError`, etc.) exists today.

### E2: The shipped workflow sets 900s on "Test writing" only

- **Source:** `workflow/config.json:26`, pinned by `src/internal/validator/production_steps_test.go:365-420`
- **Finding:**
  ```json
  { "name": "Test writing", "isClaude": true, "model": "sonnet", "promptFile": "test-writing.md", "timeoutSeconds": 900 },
  ```
- **Relevance:** The single timeout in the shipped workflow. The pin test forbids adding timeouts to any other step without an intentional change.

### E3: Test-writing prompt asks for multi-stage work far beyond "run the test suite"

- **Source:** `workflow/prompts/test-writing.md:11-22`
- **Finding:**
  ```
  1. Write all tests specified in test-plan.md
  2. Run all tests, type checks, linting and formatting tools. Fix any issues.
  3. Delete test-plan.md
  4. Commit changes in a single commit.
  5. Add a comment to github issue {{ISSUE_ID}} with your progress
  6. Append your progress to progress.txt
  7. Append all deferred work to deferred.txt
  ...
  Budget: write all tests first, then run the suite ONCE. If >5 tests fail, fix them
  in batch rather than one at a time. Do not exceed 8 minutes of wall-clock test execution.
  ```
- **Relevance:** The "8 minutes" budget is for test *execution*, not step wall-clock. Writing N tests, running the suite, fixing failures, committing, commenting on GitHub, updating progress/deferred files routinely consumes 20-60 minutes on top of that.

### E4: `TimeoutSeconds` flows from config through validator → Step → ResolvedStep → stepDispatcher → Runner

- **Source:** `src/internal/workflow/run.go:150`, `:162`, `:770`, `:786`; `src/internal/workflow/workflow.go:306`, `:344`, `:484-555`
- **Finding:** `buildStep` copies `s.TimeoutSeconds` into `ui.ResolvedStep`; `stepDispatcher.RunStep` forwards it into `RunSandboxedStep`/`RunStepFull`, which pass it to `runCommand`. A goroutine fires after N seconds, sets `r.timeoutFired = true`, and calls the `Terminator` closure that issues SIGTERM (then SIGKILL after 10 s) to the Docker container via cidfile.
- **Relevance:** Wall-clock expiry kills the subprocess; `cmd.Wait()` returns a non-nil error because the process was signaled. There is no timeout-specific exit path — it looks identical to any other subprocess failure to the caller.

### E5: `runStepWithErrorHandling` treats ANY non-nil error as failure and enters ModeError

- **Source:** `src/internal/ui/orchestrate.go:86-111`
- **Finding:**
  ```go
  func runStepWithErrorHandling(idx int, step ResolvedStep, runner StepRunner, header StepHeader, h *KeyHandler) StepAction {
      for {
          err := runner.RunStep(step.Name, step.Command)
          if err == nil || runner.WasTerminated() {
              header.SetStepState(idx, StepDone)
              return ActionContinue
          }
          header.SetStepState(idx, StepFailed)
          h.SetMode(ModeError)
          action := <-h.Actions  // BLOCKS on user input
          h.SetMode(ModeNormal)
          switch action {
          case ActionContinue:
              return ActionContinue
          case ActionRetry:
              runner.WriteToLog(RetryStepSeparator(step.Name))
          case ActionQuit:
              return ActionQuit
          }
      }
  }
  ```
- **Relevance:** **This is the single policy site that forces the `c/r/q` prompt.** `WasTimedOut()` is never consulted — a timeout is indistinguishable from any other failure. The blocking receive at `action := <-h.Actions` parks the orchestrator until a human presses a key.

### E6: `ui.StepRunner` does not expose `WasTimedOut`; only `workflow.StepExecutor` does

- **Source:** `src/internal/ui/orchestrate.go:22-26`, `src/internal/workflow/run.go:66-85`
- **Finding:**
  ```go
  // ui.StepRunner
  type StepRunner interface {
      RunStep(name string, command []string) error
      WasTerminated() bool
      WriteToLog(line string)
  }
  // workflow.StepExecutor (superset)
  type StepExecutor interface {
      ui.StepRunner
      ...
      WasTimedOut() bool
      ...
  }
  ```
- **Relevance:** The UI layer today cannot distinguish a timeout from any other failure. Any fix that routes on timeout must extend `ui.StepRunner`.

### E7: `WasTimedOut` is consulted only for log annotation, never for routing

- **Source:** `src/internal/workflow/run.go:236-243`
- **Finding:**
  ```go
  func setTimeoutNote(rec *IterationRecord, executor StepExecutor, s steps.Step) {
      if executor.WasTimedOut() && s.TimeoutSeconds > 0 {
          rec.Notes = fmt.Sprintf("timed out after %ds", s.TimeoutSeconds)
      }
  }
  ```
- **Relevance:** Confirms the `"notes":"timed out after 900s"` string in the user's iteration.jsonl. No control-flow branch depends on it.

### E8: No existing config knob for soft-fail / continue-on-error / auto-retry

- **Source:** Full-repo search for `continueOnError|allowFailure|nonFatal|onTimeout|onError|skipOnError|softFail|bestEffort` (production code only). Zero matches.
- **Relevance:** A new schema field is required to express "treat timeout as non-fatal" without changing the default behavior of every other step.

### E9: Retry-after-timeout starts a fresh Claude session (no `--resume`)

- **Source:** `src/internal/workflow/run.go:749-786` (`buildStep` is called once per step per iteration, threads `resumeSessionID` into argv); `src/internal/sandbox/command.go:83-84` (argv bakes `--resume` in); `src/internal/ui/orchestrate.go:105-107` (retry loop reuses the same pre-built `step.Command`)
- **Finding:** The retry inside `runStepWithErrorHandling` does NOT rebuild the step or re-derive `resumeSessionID`, so a retry cannot inject `--resume <original_sid>`. The G5 gate (`src/internal/workflow/run.go:273-275`) explicitly blacklists timed-out session IDs to prevent resume: `"G5: previous step session is blacklisted (was timed out)"`. G2 would also fail because the timed-out step ends in `StepFailed` not `StepDone`.
- **Relevance:** A retry loses ~15 minutes of accumulated context. The user's iteration.jsonl shows this — the failed record and the subsequent `"status":"done"` record carry different session IDs.

### E10: Two-record pattern is emitted when the user presses `r`

- **Source:** `src/internal/workflow/run.go:138-144` (`WARN-001` block in `stepDispatcher.RunStep`), `:554-560` (the `onTimeoutRetry` closure); user data at `/Users/mxriverlynn/dev/gearjot/gearjot-v2/.pr9k/iteration.jsonl:75-76`
- **Finding:**
  ```json
  {"step_name":"Test writing","model":"sonnet","status":"failed","duration_s":0,"notes":"timed out after 900s"}
  {"step_name":"Test writing","model":"sonnet","status":"done","duration_s":4709.91,"session_id":"7bc8ae27-..."}
  ```
- **Relevance:** Confirms iter06 was rescued by a 78-minute fresh retry. The `duration_s:0` on the failed record is the signature of the WARN-001 synthetic record; the engine already separates "timed out" from regular failures for accounting — just not for control flow.

### E11: Distribution of "Test writing" duration_s across completed runs — separating organic from retry-after-timeout

- **Source:** `~/dev/gearjot/gearjot-v2-web/.pr9k/iteration.jsonl`, `~/dev/gearjot/gearjot-v2/.pr9k/iteration.jsonl`
- **Finding:** Full record counts: 11 "done", 3 "failed" (all timeouts), 1 "unknown" (crashed pre-iter). Three of the 11 "done" records are **retry-from-fresh durations** (the Claude session was restarted after a timeout — 1662s, 1841s, 4710s). **Organic first-shot successful durations** (n=8): `237, 236, 390, 403, 518, 592, 642, 734`. Mean ≈ 469s, median ≈ 468s, **p95 ≈ 733s, max 734s**. Retry-after-timeout durations are pathological (a fresh session re-derives all context from scratch) and must not be used to size the cap.
- **Relevance:** Under the current 900s cap, 3 out of 11 first-shot attempts timed out (27% timeout rate on first attempt). The organic first-shot p95 is 733s — below the current cap — but the right tail pushes past it. Once `onTimeout=continue` is in place, retries disappear as a signal; the organic distribution is what matters. A cap of 1800s (30 min) covers organic p99 with a generous margin; 3600s was originally proposed based on the conflated p95 of 4710s and is more than necessary.

### E12: At the moment of the v2-web iter05 kill, Claude was on the last TodoWrite item with tests passing

- **Source:** Last ~10 events of `~/dev/gearjot/gearjot-v2-web/.pr9k/logs/ralph-2026-04-21-102430.433/iter05-08-test-writing.jsonl` (session_id `5b4c1118-cb4f-4ca9-9a39-151a585b8491`)
- **Finding:** Final TodoWrite state: `[Fix TP-026 (in_progress), Delete test-plan.md, Run format/lint/check then commit, Comment on GH 156, Append progress/deferred]`. Immediately prior tool_result from `npm run format && npm run lint && npm run check`: `COMPLETED 6241 FILES 0 ERRORS 0 WARNINGS`. Last assistant message: `"All clean. Now run just the failing test file to confirm it passes:"` — then killed.
- **Relevance:** Claude was productively within 2-3 minutes of completion. The user's intuition that "it shouldn't be a fail state" is correct for this concrete case.

### E13: Timeout blacklist gate (G5) is populated at `RunSandboxedStep` return — before any retry

- **Source:** `src/internal/workflow/workflow.go:352-361`
- **Finding:**
  ```go
  r.processMu.Lock()
  if r.timeoutFired && stats.SessionID != "" {
      if r.sessionBlacklist == nil {
          r.sessionBlacklist = make(map[string]bool)
      }
      r.sessionBlacklist[stats.SessionID] = true
  }
  r.processMu.Unlock()
  ```
- **Relevance:** Even if a future redesign wanted to auto-resume after timeout, G5 would reject it. The blacklist is policy (`"a timed-out session's state is unknown"` — `run.go:258`), not a bug. For a soft-timeout fix we do NOT need to change the blacklist — we only need to stop blocking the user.

### E14: The step itself can flail on environment issues and fill 900s with non-productive work

- **Source:** `~/dev/gearjot/gearjot-v2/.pr9k/logs/ralph-2026-04-21-095746.014/iter05-08-test-writing.jsonl` (734s, finished but barely)
- **Finding:** A contiguous block of ~15 bash commands dedicated to fixing Go toolchain layout in `.ralph-cache/`: `chmod +x` on `golang.org/toolchain@v0.0.1-go1.26.2.linux-arm64/bin/go`, copying to `/tmp/go1.26.2`, building `/tmp/goroot1262/pkg/tool`, repeated `GOROOT=... GOPATH=... GOMODCACHE=... go test` invocations.
- **Relevance:** Not every 900s-ish run is "productive work". When `.ralph-cache/` permissions collide with the sandbox UID mapping, Claude can burn real wall-clock on workarounds. A pure "raise the timeout" fix would not distinguish productive work from stuck loops; a "soft-fail on timeout" fix bounds blast radius regardless of the cause.

## Root Cause Analysis

### Summary

The 900s cap on "Test writing" is systematically smaller than the prompt's actual workload (median ≈555s, p95 ≈4710s), and when it fires `runStepWithErrorHandling` forces a blocking `c/r/q` prompt because the orchestrator treats every non-zero subprocess exit identically.

### Detailed Analysis

Two independent defects compound:

1. **Cap mis-sized for workload.** The `timeoutSeconds: 900` on "Test writing" (E2) is below the observed median runtime across 10 successful runs (E11). The prompt asks Claude to write N tests, run the suite, fix failures, commit, comment on GitHub, and update two progress files (E3). Even a clean run routinely exceeds 15 minutes. When Claude is within minutes of completing the documented work (E12), the timer kills it.

2. **Timeout forces human-blocking error mode.** When the timer fires, it SIGTERMs the subprocess via the cidfile Terminator (E4). `cmd.Wait()` returns a non-nil error. `runStepWithErrorHandling` (E5) checks only `err != nil` and `runner.WasTerminated()` (user-initiated skip), enters `ModeError`, and blocks on `<-h.Actions`. `WasTimedOut()` is consulted only for log annotation (E7), never for routing. The `ui.StepRunner` interface does not even expose `WasTimedOut` — only `workflow.StepExecutor` does (E6). Consequently a timeout and a genuine step failure are indistinguishable to the UI layer, and an unattended run becomes a stalled TUI.

On retry, the original session cannot be resumed because `buildStep` is called only once per step and the G5 gate blacklists timed-out session IDs anyway (E9, E13), so 15+ minutes of accumulated context is thrown away. The resulting two-record pattern in `iteration.jsonl` (E10) reflects this: a synthetic `duration_s:0 notes:"timed out..."` record, followed by a fresh-session `done` record at the full retry duration.

The user's phrasing pointed at "the test runner failing" — but the test runner's non-zero exits are handled inline by Claude within the step (retry/batch-fix, per the prompt's budget line). The actual fail-state is always and only the wall-clock timeout.

## Coding Standards Reference

| Standard | Source | Applies To |
|----------|--------|------------|
| Config schema additions are backwards-compatible additions to the public API; omit field → existing behavior preserved. `0.y.z` treats backward-compatible additions as PATCH bumps. | `docs/coding-standards/versioning.md:15-38` | New `onTimeout` field on `steps.Step`, validator, pin tests |
| Validate preconditions at function boundary with a clear error. | `docs/coding-standards/api-design.md:30-41` | Validator rule for `onTimeout` enum values |
| Document unused parameters; no silent drift between `ui.StepRunner` and `workflow.StepExecutor`. | `docs/coding-standards/api-design.md:3-15`, `:108-126` | Extending `ui.StepRunner` with `WasTimedOut` |
| Race-detector-required tests; closeable idempotency; input immutability. | `docs/coding-standards/testing.md` | New orchestrate + validator tests |
| Error messages package-prefixed (`steps:`, `workflow:`, etc.) | `docs/coding-standards/error-handling.md` | New validator error message |
| Narrow-reading principle: workflow content (policy defaults for specific steps) lives in `config.json`, not Go. | `docs/adr/20260410170952-narrow-reading-principle.md` | Decision to put `onTimeout: "continue"` in `workflow/config.json`, not hard-code it |
| Lint suppressions prohibited. | `docs/coding-standards/lint-and-tooling.md` | New code must pass `make ci` without exclusions |

## Planned Fix

### Summary

Add a new optional per-step `onTimeout` policy field (`"fail" | "continue"`, default `"fail"`), teach `runStepWithErrorHandling` to consult it so a timeout with `"continue"` records the step as failed-but-proceeding (new `StepTimedOutContinuing` state with a distinct TUI glyph) and advances without blocking. Explicitly clear the executor's `timeoutFired` flag when the continue branch is taken so the next step's dispatcher does not see stale state. Configure `"Test writing"` with `onTimeout: "continue"` and raise its `timeoutSeconds` to **1800** (30 min) — 2.5× organic p95 (E11), which absorbs legitimate long runs while keeping a ceiling for pathological hangs. Bump the version to 0.7.1.

The two sub-changes together resolve both compounding defects: the larger cap absorbs legitimate long runs, and the `onTimeout: "continue"` policy ensures the rare remaining genuine runaways log-and-move-on instead of parking the TUI overnight.

### Changes

#### `src/internal/steps/steps.go`

- **Change:** Add `OnTimeout string \`json:"onTimeout,omitempty"\`` to `Step`. Document accepted values (`""` == `"fail"` == current behavior; `"continue"` == treat timeout as non-fatal).
- **Evidence:** (E1), (E5), (E8)
- **Standards:** versioning (additive), api-design (document default)
- **Details:** Field doc comment explicitly states that `""` preserves existing behavior so every pre-existing `config.json` is unaffected.

#### `src/internal/validator/validator.go`

- **Change:** Add `vStep.OnTimeout` field. In the Schema 2d block (right after `timeoutSeconds` rules), validate: if set, must be one of `"fail"`, `"continue"`. If `onTimeout` is set without `timeoutSeconds > 0`, emit a non-fatal warning (mirrors how `skipIfCaptureEmpty` relates to `captureAs`).
- **Evidence:** (E1), (E8)
- **Standards:** api-design (precondition at boundary), error-handling (package-prefixed messages)
- **Details:** Reject `"retry"` and other values for now — scope-minimal. Future expansion (auto-retry) can add values without breaking callers that use `"fail"` or `"continue"`.

#### `src/internal/ui/orchestrate.go`

- **Change:** Extend `StepRunner` with `WasTimedOut() bool` AND `ClearTimeoutFlag()`. Add `OnTimeout string` to `ResolvedStep`. Add a new `StepState` value `StepTimedOutContinuing` (with its own glyph — see `StepState` block change below). In `runStepWithErrorHandling`, after the existing `err == nil || WasTerminated()` branch, add a new branch: if `runner.WasTimedOut() && step.OnTimeout == "continue"`, write a one-line banner, call `runner.ClearTimeoutFlag()`, mark the step `StepTimedOutContinuing`, and return `ActionContinue` without entering `ModeError`.
- **Evidence:** (E5), (E6), (E7); addresses V4/V13 (residual flag), V7 (checkbox semantics)
- **Standards:** api-design (narrow interface — add only the two methods we consume; `ClearTimeoutFlag` name is explicit about its effect), concurrency (`WasTimedOut` and `ClearTimeoutFlag` are already mutex-protected in `workflow.Runner`)
- **Details:**
  ```go
  // orchestrate.go (new branch inside runStepWithErrorHandling)
  if err == nil || runner.WasTerminated() {
      header.SetStepState(idx, StepDone)
      return ActionContinue
  }
  if runner.WasTimedOut() && step.OnTimeout == "continue" {
      runner.WriteToLog(TimeoutContinueBanner(step.Name, step.TimeoutSeconds))
      runner.ClearTimeoutFlag()  // V4 fix: prevent next step's dispatcher from firing WARN-001
      header.SetStepState(idx, StepTimedOutContinuing)
      return ActionContinue
  }
  header.SetStepState(idx, StepFailed)
  h.SetMode(ModeError)
  ...
  ```
  Banner copy mirrors the `"timed out after %ds"` string already in iteration.jsonl (E7).
- **Also update `StepRunner` interface:**
  ```go
  type StepRunner interface {
      RunStep(name string, command []string) error
      WasTerminated() bool
      WasTimedOut() bool          // NEW — delegated by stepDispatcher to the executor
      ClearTimeoutFlag()          // NEW — idempotent reset of executor.timeoutFired
      WriteToLog(line string)
  }
  ```

#### `src/internal/ui/ui.go` (or wherever `StepState` lives)

- **Change:** Add `StepTimedOutContinuing StepState` alongside existing states. Map it to a distinct checkbox glyph (recommend `[!]`) so the user can visually distinguish "step was soft-timed out, workflow continued" from "step hard-failed, user pressed continue". Update `stepStatus` mapping in `src/internal/workflow/run.go:220-234` to return `"failed"` for `StepTimedOutContinuing` so iteration.jsonl still records the timeout truthfully (with the `setTimeoutNote` attaching `"timed out after Ns"`).
- **Evidence:** (V7 validation finding)
- **Standards:** TUI chrome is explicitly NOT public API (versioning.md item 3), so adding a glyph is not a breaking change.
- **Details:** Update `tui-display.md` state table and the two how-to guides that reference checkbox meanings (`docs/how-to/reading-the-tui.md`, `docs/how-to/recovering-from-step-failures.md`).

#### `src/internal/workflow/workflow.go`

- **Change:** Add `ClearTimeoutFlag()` method on `*Runner`:
  ```go
  // ClearTimeoutFlag resets the timed-out flag. Called from the ui layer when a
  // soft-fail-on-timeout policy has consumed the flag and the workflow is advancing
  // to the next step. Must be called before the next step's dispatcher consults
  // WasTimedOut(), otherwise a stale true will trigger a spurious WARN-001 record.
  func (r *Runner) ClearTimeoutFlag() {
      r.processMu.Lock()
      defer r.processMu.Unlock()
      r.timeoutFired = false
  }
  ```
- **Evidence:** (V4)
- **Standards:** concurrency (mutex-protected write, matches existing pattern at `workflow.go:195-210`)

#### `src/internal/workflow/run.go`

- **Change:** Three additions:
  1. In `buildStep`, copy `s.OnTimeout` into `ui.ResolvedStep.OnTimeout` in both the claude and non-claude branches (around lines 770, 786).
  2. Add `WasTimedOut()` and `ClearTimeoutFlag()` delegations on `stepDispatcher` (it already wraps the executor; add two 1-line pass-throughs alongside the existing `WasTerminated` delegation at `:165`):
     ```go
     func (d *stepDispatcher) WasTimedOut() bool    { return d.exec.WasTimedOut() }
     func (d *stepDispatcher) ClearTimeoutFlag()    { d.exec.ClearTimeoutFlag() }
     ```
  3. Update `stepStatus` to map `StepTimedOutContinuing` → `"failed"` so iteration.jsonl records the timeout truthfully.
- **Evidence:** (E4), (V3), (V12)
- **Standards:** api-design (consistent field threading across both buildStep branches; dispatcher delegation matches existing pattern at `:165-166`)

#### `src/internal/workflow/workflow_test.go` and `src/internal/workflow/run_timeout_test.go`

- **Change:** Add tests for `Runner.ClearTimeoutFlag()` (idempotent, resets flag). Add a cross-step integration test that executes step A (claude, with `TimeoutSeconds=1, OnTimeout="continue"`, deliberate sleep to trigger timeout) followed by step B (non-claude, success). Assert iteration.jsonl contains exactly:
  - One record for step A: `status="failed", notes="timed out after 1s"`.
  - One record for step B: `status="done"`, and NO spurious WARN-001 record with `timed out after 0s`.
- **Evidence:** (V4), (V13)
- **Standards:** testing (race detector required — new test must run under `go test -race`)

#### `workflow/config.json`

- **Change:** On the `"Test writing"` step, change `"timeoutSeconds": 900` → `"timeoutSeconds": 1800` and add `"onTimeout": "continue"`.
- **Evidence:** (E2), (E3), (E11), (E12), (E14); addresses V1 (retry-vs-organic distribution correction)
- **Standards:** narrow-reading ADR (step policy belongs in config.json, not Go), versioning (new schema field addition + workflow content update)
- **Details:** 1800s (30 min) is ~2.5× organic p95 (733s per revised E11). This absorbs the observed first-shot tail with generous margin while keeping a meaningful ceiling against pathological hangs. With `onTimeout: "continue"`, the rare run that exceeds 1800s is logged-and-proceeded (not retried), so a tighter cap no longer risks losing work to retry pathology.

#### `src/internal/validator/production_steps_test.go`

- **Change:** Update `TestLoadSteps_TestWritingStep_TimeoutSeconds` (line 361-ff) to assert `TimeoutSeconds == 1800` and `OnTimeout == "continue"`. Keep `TestLoadSteps_OnlyTestWritingHasTimeout` unchanged (Test writing is still the only step with a timeout).
- **Evidence:** (E2)
- **Standards:** testing (pin tests lock shipped config intent)

#### `src/internal/ui/orchestrate_test.go`

- **Change:** Update the `stubRunner` and `callbackStubRunner` test doubles (src/internal/ui/orchestrate_test.go:13-44) to implement the two new interface methods (`WasTimedOut() bool`, `ClearTimeoutFlag()`). Add test cases covering:
  - Timeout with `OnTimeout="continue"` marks step state `StepTimedOutContinuing`, calls `ClearTimeoutFlag`, and returns `ActionContinue` without reading from `h.Actions` (no blocking).
  - Timeout with `OnTimeout=""` (default) still enters `ModeError` exactly as today.
  - Timeout with `OnTimeout="fail"` (explicit) behaves identically to the default.
  - Non-timeout failure (`WasTimedOut=false`) with `OnTimeout="continue"` still enters `ModeError` — the policy is timeout-specific, not generic soft-fail.
- **Evidence:** (V3) — enumerates specific test doubles
- **Standards:** testing (race detector required)

#### `src/internal/validator/validator.go`

- **Change:** Add a non-fatal warning when `onTimeout: "continue"` is set on a step that is immediately followed by a step with `resumePrevious: true`. Text: `"onTimeout:continue on step X means the following resumePrevious:true step Y will fall through G2 on timeout paths (fresh session)"`.
- **Evidence:** (V5)
- **Standards:** validator (non-fatal info/warning severity already exists)

#### `src/internal/version/version.go`

- **Change:** `const Version = "0.7.0"` → `const Version = "0.7.1"`.
- **Evidence:** (V10) — versioning.md §"0.y.z" classifies backwards-compatible schema additions as PATCH bumps.
- **Standards:** versioning.md §"How to bump the version" — separate commit, combined with docs-only changes is allowed.

#### `docs/how-to/setting-step-timeouts.md`

- **Change:** Add a section "Soft-fail on timeout" documenting `onTimeout: "continue"`, linking the use case to unattended runs. Cross-link to the `resumePrevious` interaction (V5): "if a step immediately after a `onTimeout:continue` step uses `resumePrevious:true`, a soft timeout causes the resume to fall through G2 and start a fresh session."
- **Evidence:** (E3), (E11), (E12), (V5)
- **Standards:** documentation.md (feature docs ship with feature)

#### `docs/features/tui-display.md`, `docs/how-to/reading-the-tui.md`, `docs/how-to/recovering-from-step-failures.md`

- **Change:** Document the new `StepTimedOutContinuing` checkbox glyph (`[!]` or chosen symbol) and its meaning: the step was soft-timed-out and the workflow advanced. Distinct from `[x]` (hard failure, user consulted via ModeError).
- **Evidence:** (V7)
- **Standards:** documentation.md

#### `docs/code-packages/workflow.md`, `docs/features/workflow-orchestration.md`, `CLAUDE.md`

- **Change:** Update the `runStepWithErrorHandling` description and the G-gate / resume discussion to note the new `onTimeout` branch and `ClearTimeoutFlag` call. Add a one-line entry in the CLAUDE.md index for the new how-to section if documentation.md requires it.
- **Evidence:** project instructions require CLAUDE.md to list new docs
- **Standards:** documentation.md

## Validation Results

### Counter-Evidence Investigated

#### V1: E11 distribution conflates retry-after-timeout durations with organic runtime

- **Hypothesis:** "10 successes / 3 timeouts, p95 ≈ 4710s" is an accurate sizing signal for the 3600s choice.
- **Investigation:** Re-counted both iteration.jsonl files. Actual: 11 successes + 3 timeouts + 1 "unknown" (crashed). Three of the 11 "done" records (1662s, 1841s, 4710s) are retry-from-fresh durations after a prior timeout, NOT first-shot completion times. Organic first-shot distribution: n=8, values `237, 236, 390, 403, 518, 592, 642, 734`, p95 ≈ 733s.
- **Result:** Partially Refuted.
- **Impact:** Plan now sizes the cap against **organic** first-shot p95, not retry-tail p95. Chose **1800s** (~2.5× organic p95 — 733×2.5 = 1832) instead of 3600s. E11 text rewritten to separate organic from retry durations.

#### V2: Is `runStepWithErrorHandling` truly the only ModeError transition?

- **Hypothesis:** Some other site also transitions into ModeError on step failure, making the fix incomplete.
- **Investigation:** Grepped all `ModeError` mentions in `src/`. All non-test production references are **reads** (switch arms on current mode at `keys.go:30`, `ui.go:19/:125/:209`, `wiring.go:21`, `model.go:255`), not writes. Only `orchestrate.go:96` transitions INTO ModeError in production.
- **Result:** Confirmed.
- **Impact:** No additional fix-sites required.

#### V3: Which concrete types implement `StepRunner` and will break on interface extension?

- **Hypothesis:** Extending `ui.StepRunner` with `WasTimedOut` breaks existing test doubles.
- **Investigation:** Enumerated: `*workflow.Runner` (already implements both), `*workflow.stepDispatcher` (needs 1-line delegations), `*ui.stubRunner` at `orchestrate_test.go:13-30` (needs updates), `*ui.callbackStubRunner` at `orchestrate_test.go:34-44` (needs updates). Plan originally said only "test doubles for StepRunner" — too vague.
- **Result:** Partially Refuted.
- **Impact:** Plan's Changes section now explicitly enumerates `stepDispatcher.WasTimedOut()` delegation AND both test-double updates.

#### V4: Residual `WasTimedOut` flag persists across dispatcher boundaries — BLOCKING DEFECT

- **Hypothesis:** After the orchestrate-layer continue branch, the flag `r.timeoutFired` remains true until the next step's `RunStepFull` / `RunSandboxedStep` resets it on entry. BUT the next step's `stepDispatcher.RunStep` checks `d.exec.WasTimedOut()` BEFORE calling the inner executor (at `run.go:138-144`). That check fires `onTimeoutRetry`, which emits a spurious `{"step":"NextStep","status":"failed","notes":"timed out after 0s"}` record in iteration.jsonl.
- **Investigation:** Read `run.go:138-144`, `workflow.go:263-265`, `workflow.go:291-294`. Confirmed: `r.timeoutFired` is reset only at executor entry, never at orchestrate-layer exit. The dispatcher's WARN-001 guard at `run.go:142-143` fires on any true value regardless of whether the dispatcher's own step has a timeout.
- **Result:** Refuted (fix-induced defect).
- **Impact:** Plan now adds `ClearTimeoutFlag()` method on `workflow.Runner` and `stepDispatcher`, exposed via `ui.StepRunner`. Orchestrate calls it inside the continue branch before returning. New cross-step integration test asserts iteration.jsonl contains no spurious record on the following step.

#### V5: `onTimeout:continue` interaction with downstream `resumePrevious:true`

- **Hypothesis:** A downstream step with `resumePrevious:true` would silently lose resume after a soft-timeout because G2 requires `prevState == StepDone`.
- **Investigation:** Confirmed G2 check at `run.go:267-269`. Shipped `workflow/config.json` has no `resumePrevious:true` anywhere, so no immediate blast radius. But future users combining the two would be surprised.
- **Result:** Confirmed (no immediate impact, but a documented gotcha).
- **Impact:** Plan adds a validator non-fatal warning when `onTimeout:"continue"` precedes a `resumePrevious:true` step, and a cross-link in the how-to doc.

#### V6: `WasTimedOut()` flag lifecycle at the orchestrate-layer check point

- **Hypothesis:** The flag correctly reflects the step that just ran when `runStepWithErrorHandling` checks it after `runner.RunStep` returns.
- **Investigation:** Traced: entry reset → timer goroutine set under `!r.terminated` guard → `cmd.Wait` returns → orchestrate reads. The existing WARN-003 re-check prevents setting the flag for a successful completion that raced the timer. Lifecycle is correct at the check point.
- **Result:** Confirmed. The read site is safe; the bug is the downstream persistence (V4).
- **Impact:** No change to the check itself; the `ClearTimeoutFlag` addition (V4) handles the downstream persistence.

#### V7: Checkbox `[x]` + continuing workflow is visually incoherent

- **Hypothesis:** Showing `[x]` (hard fail) while the TUI advances to the next step contradicts the existing meaning of `[x]` (which pairs with ModeError).
- **Investigation:** `docs/features/tui-display.md:104` documents `[✗]` as hard fail. Versioning.md confirms TUI chrome is NOT public API, so a new state is cheap. Plan originally picked "keep `[x]`" for simplicity.
- **Result:** Partially Refuted.
- **Impact:** Plan now introduces a distinct `StepTimedOutContinuing` state with its own glyph (`[!]` recommended) so operators can visually distinguish soft-timeout from hard-fail. Documentation updated across tui-display.md, reading-the-tui.md, recovering-from-step-failures.md.

#### V8: Race between `WasTerminated` (user skip) and `WasTimedOut` (timer)

- **Hypothesis:** Both flags could become true simultaneously, corrupting the orchestrate branch logic.
- **Investigation:** Read `workflow.go:195-210` (Terminate) and `:512-516` (timer goroutine). Both mutate under `processMu` with a first-flag-wins guard. WARN-002 invariant is enforced. No race.
- **Result:** Confirmed.
- **Impact:** Ordering of orchestrate branches (Terminated first, then TimedOut-with-continue, then default error) is correct as proposed.

#### V9: Version bump missing from plan

- **Hypothesis:** Adding `onTimeout` to the JSON schema requires a version bump.
- **Investigation:** versioning.md item 2 says config.json schema is public API; §"0.y.z" says backwards-compatible additions bump PATCH.
- **Result:** Partially Refuted.
- **Impact:** Plan now includes `src/internal/version/version.go` → `0.7.1` as an explicit change.

#### V10: Simpler alternative — just remove `timeoutSeconds`

- **Hypothesis:** Bare removal of the cap (no policy change) is sufficient.
- **Investigation:** Without a cap, a truly-hung Claude step would stall indefinitely in an unattended run. No run-level cap exists. Removing the cap trades "blocks on prompt" for "silently hangs forever" — strictly worse for unattended automation.
- **Result:** Confirmed (alternative is inferior).
- **Impact:** Plan's policy-plus-raised-cap approach is retained as strictly better.

#### V11: `stepStatus` mapping for the new state

- **Hypothesis:** Mapping `StepTimedOutContinuing` → `"failed"` in iteration.jsonl preserves truthful structured logging.
- **Investigation:** `stepStatus` at `run.go:220-234` currently maps `StepFailed` → `"failed"`. Extending to map `StepTimedOutContinuing` → `"failed"` means the setTimeoutNote call still attaches `"timed out after Ns"`. Operators grepping for timeouts continue to see today's behavior.
- **Result:** Confirmed.
- **Impact:** Plan explicitly specifies this mapping in the run.go change list.

#### V12: Cross-step integration test coverage

- **Hypothesis:** Original plan's test list covered only single-step scenarios and would have missed V4/V13.
- **Investigation:** Plan's original tests were orchestrate-layer unit tests with a `stubRunner`; none exercised the dispatcher seam where the bug lives.
- **Result:** Refuted (coverage gap).
- **Impact:** Plan now requires a cross-step integration test in `workflow/run_timeout_test.go` that exercises step A (claude, forced timeout, `OnTimeout=continue`) followed by step B (non-claude success) and asserts exact iteration.jsonl contents.

### Adjustments Made

- **V1:** Rewrote E11 to separate organic from retry durations; changed proposed `timeoutSeconds` from 3600 → 1800 with rationale tied to organic p95.
- **V3:** Explicitly enumerated `stepDispatcher.WasTimedOut()` delegation plus both `stubRunner` and `callbackStubRunner` updates.
- **V4:** Added `ClearTimeoutFlag()` on `workflow.Runner`, `stepDispatcher`, and `ui.StepRunner`. Orchestrate calls it inside the continue branch.
- **V5:** Added validator non-fatal warning + docs cross-link for `onTimeout:continue` preceding `resumePrevious:true`.
- **V7:** Introduced `StepTimedOutContinuing` state with its own glyph; updated three TUI-facing docs.
- **V9:** Added `version.Version` bump to 0.7.1 as an explicit change.
- **V11:** Specified `stepStatus(StepTimedOutContinuing) = "failed"` mapping.
- **V12:** Added cross-step integration test to the test list.

### Confidence Assessment

- **Confidence:** Medium-High (raised from Low after adjustments).
- **Remaining Risks:**
  1. **Run-summary accuracy on soft-timeout** (validator's untouched area): `stepDispatcher.RunStep` folds `LastStats()` into `runStats` after `RunSandboxedStep` returns, regardless of success. On a soft timeout, partial stats (tokens consumed up to the SIGTERM moment) are counted. Intended behavior, but the run-level summary should be spot-checked to confirm it still sums correctly when `onTimeout=continue` is in play.
  2. **Organic p95 basis is 8 samples** (E11); 1800s may need to be tuned up once more data accumulates. The policy-layer fix is resilient to mis-sizing — a timeout that fires with `onTimeout:continue` just logs-and-proceeds rather than blocking.
  3. **Downstream resume interaction (V5)** is a gotcha, not a regression. The validator warning + docs call it out, but an operator who ignores the warning and sets `resumePrevious:true` on a step after Test writing will silently lose resume on soft-timeout paths.
  4. **New `[!]` glyph semantics** need to be understood by users; the checkbox is no longer a binary done/fail state. Visual review during implementation is recommended.

## Final Summary

- **Root Cause:** `timeoutSeconds: 900` is smaller than the observed first-shot tail of the documented Test-writing prompt (organic p95 ≈ 733s, max 734s, right tail exceeds 900s), and `runStepWithErrorHandling` forces a blocking `c/r/q` prompt on any non-zero subprocess exit (E5) regardless of whether the failure was a timeout, defeating unattended runs.
- **Fix:** Raise Test writing's `timeoutSeconds` to 1800, add a new optional `onTimeout: "fail" | "continue"` per-step policy that makes a timeout non-fatal (new `StepTimedOutContinuing` state with `[!]` glyph, `ClearTimeoutFlag` called to prevent the dispatcher's WARN-001 from firing on the next step, iteration.jsonl still records `status=failed` with the `timed out after Ns` note), configure `"Test writing"` with `onTimeout: "continue"`, and bump version to 0.7.1.
- **Why Correct:** E5 + E6 establish the single policy site; E7 + E10 show the existing WARN-001 accounting already separates timeouts from failures at the flag level and can be routed on; E11 (revised) sizes the cap to organic tail; V4 closes the only blocking fix-induced defect; V7 resolves the visual-coherence concern.
- **Validation Outcome:** Refuted one blocking defect (V4) and corrected the distribution analysis (V1) that had inflated the proposed cap; plan updated with `ClearTimeoutFlag`, a distinct checkbox state, test-double enumeration, validator warning for the resume interaction, version bump, and a cross-step integration test.
- **Remaining Risks:** Medium-High confidence. Small-sample p95 basis, the known `resumePrevious:true` interaction gotcha, and the need to visually review the new checkbox glyph in context.
