# Feature Implementation Plan: Phase 5 — Lifecycle Notifications

Phase 5 wires three cmux notification classes — completion (one-shot), run-aborted (one-shot), and error-mode (persistent, 60s cadence) — into the existing pr9k cmux orchestrator. The implementation introduces a new `cmuxNotifier` adapter in `src/cmd/pr9k/cmux_notifier.go` parallel to `cmuxSidebar`, one new RPC method on `cmuxctl.CmuxClient` (`WorkspaceNotify`) covering all three classes, a typed `*cmuxctl.TimeoutError` replacing the existing `fmt.Errorf` timeout string, two new `IntentType` values in `internal/interactionchannel` to make spec D5's first-keystroke timer-stop implementable in cmux mode, and three new firing sites inside `runCmuxWorkflowAdapted`. No new CLI flag, no new on-disk artifact, no schema change.

## Source Specification

- **Feature specification:** [feature-specification.md](feature-specification.md)
- **Specification decision log:** [artifacts/decision-log.md](artifacts/decision-log.md)
- **Specification team findings:** [artifacts/team-findings.md](artifacts/team-findings.md)
- **Specification decisions this plan inherits:** spec D1–D18 (full D1–D13; trivial D14–D18).
- **Specification open items this plan must respect or resolve:** None — spec declared no open items; this plan adds three pre-commit verification gates derived from R2's evidence-based resolutions (see [Open Items](#open-items)).
- **Parent feature specification:** [../feature-specification.md](../feature-specification.md)
- **Parent decision log:** [../artifacts/decision-log.md](../artifacts/decision-log.md)
- **Phase 4 implementation plan (format precedent + structural precedent for the parallel `cmuxNotifier` adapter):** [../phase-4-sidebar-mirroring/feature-implementation-plan.md](../phase-4-sidebar-mirroring/feature-implementation-plan.md)

**Parent specification decisions Phase 5 inherits unchanged:** [parent D6](../artifacts/decision-log.md#d6-fire-cmux-notifications-at-named-lifecycle-moments) (three notification moments), [parent D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) (per-call timeout fatal), [parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode) (log artifacts unchanged), [parent D18](../artifacts/decision-log.md#d18-startup-capability-check) (preflight gate), [parent D19](../artifacts/decision-log.md#d19-error-mode-notifications-direct-the-operator-to-the-control-pane) (error notification names the pane to focus — Phase 5 supersedes the "control pane" phrasing in favor of "footer pane"), [parent D29](../artifacts/decision-log.md#d29-workspace-name-pattern) (workspace-name pattern → repo basename), [parent D-R1](../artifacts/decision-log.md#d-r1-orchestrator-is-the-in-pane-pr9k-process-no-hidden-orchestrator-pane-supersedes-d13d-3d-4-hidden-pane-parts), [parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record), [parent D-R4](../artifacts/decision-log.md#d-r4-manual-cmux-integration-gate) (the manual cmux integration gate Phase 5's pre-commit verifications align with).

## Outcome

When this plan executes, the codebase contains:

- A new `cmuxNotifier` struct (in a new `src/cmd/pr9k/cmux_notifier.go`) that owns a `cmuxctl.CmuxClient`, a `cmuxctl.Workspace`, a `*logger.Logger`, a `sync.Mutex` covering `errorActive bool`, `snapshotName string`, `cancelRefire context.CancelFunc`, and a `resolved chan struct{}` per active error-mode session ([PD-1](artifacts/implementation-decision-log.md#pd-1-cmuxnotifier-is-a-new-struct-parallel-to-cmuxsidebar)). Its method set: `FireCompletion(ctx) error`, `FireRunAborted(ctx) error`, `EnterErrorMode(ctx, stepName string) error`, `ExitErrorMode() error`, `RestartErrorModeTimer(ctx)`, and an internal `firePersistent(ctx)`.
- One new method on `cmuxctl.CmuxClient`: `WorkspaceNotify(ctx context.Context, ws Workspace, class NotificationClass, body string) error` ([PD-2](artifacts/implementation-decision-log.md#pd-2-one-new-workspacenotify-method-covers-all-three-notification-classes)). `NotificationClass` is a typed string constant with three values (`NotificationCompletion`, `NotificationRunAborted`, `NotificationErrorMode`). Matching wrapper in `cmuxctl.RealClient` (which translates the call into cmux's verified `notification.*` wire method); matching `Func` field + recorder slice in `cmuxctl.FakeClient`.
- A typed `*cmuxctl.TimeoutError` exported from `internal/cmuxctl`, replacing the current `fmt.Errorf("cmuxctl: %s timed out after %s", ...)` at `src/internal/cmuxctl/real.go` ([PD-4](artifacts/implementation-decision-log.md#pd-4-typed-timeouterror-replaces-the-fmterrorf-timeout-string)). Existing `cmuxSidebar` timeout-detection (`errors.Is(err, context.DeadlineExceeded)`) is corrected at the same time — a pre-existing latent defect that does not catch queue-level timeouts.
- `Preflight` extended with one probe call to `WorkspaceNotify` on a zero `Workspace{}` and an `isMethodNotFound(err)` helper that checks `*CmuxError` for `Code == "method_not_found"` OR `Code == "unknown_method"` ([PD-3](artifacts/implementation-decision-log.md#pd-3-preflight-is-extended-with-one-probe-call-to-workspacenotify)). Any other error from the probe is treated as method-exists. The probe is the only Phase 5 preflight extension; per-method enumeration of all 13 `CmuxClient` methods stays deferred.
- Two new `IntentType` values in `internal/interactionchannel`: `IntentErrorQuitInitiated` (footer pane → orchestrator on the first `q` keystroke in `ModeError`) and `IntentErrorQuitCancelled` (footer pane → orchestrator on `n`/`esc` from `ModeQuitConfirm` when the previous mode was `ModeError`) ([PD-7](artifacts/implementation-decision-log.md#pd-7-two-new-intenttype-values-make-spec-d5-implementable-in-cmux-mode)). The footer state machine (`cmux_footer_machine.go`) emits these intents at the matching state transitions; the orchestrator's `keyAdapterLoop` (`cmux_workflow.go`) gains two new branches that call `notifier.ExitErrorMode()` and `notifier.RestartErrorModeTimer(ctx)` respectively. The IPC contract change is mechanically additive (new intent types; existing intents unchanged) and therefore does not require a versioning bump beyond the patch level.
- The existing `keyHandler.SetOnModeChange` closure inside `runCmuxWorkflowAdapted` is augmented in place to call `notifier.EnterErrorMode(ctx, sidebar.LastStepName())` after the existing `sidebar.EnterErrorMode(ctx)` call when `mode == ui.ModeError` ([PD-6](artifacts/implementation-decision-log.md#pd-6-three-step-ordering-of-error-mode-effects-is-enforced-by-sequential-statements-in-the-onmodechange-callback)). The closure does not gain a slice-of-hooks refactor — Phase 4's R3 reopen trigger is not met by Phase 5's two-branch addition.
- A new `cmuxSidebar.LastStepName() string` accessor (or unexported equivalent the notifier package can read) exposing the existing `lastStepName` field under the sidebar's mutex ([PD-13](artifacts/implementation-decision-log.md#pd-13-cmuxsidebar-gains-a-laststepname-accessor)).
- Terminal-notification firing sites inside `runCmuxWorkflowAdapted`, between `workflow.Run` returning and `sidebar.ClearAll(ctx)` running ([PD-9](artifacts/implementation-decision-log.md#pd-9-terminal-notifications-fire-inside-runcmuxworkflowadapted-between-workflowrun-returning-and-sidebarclearall)). The mapping from `workflow.RunResult.ExitReason` to notification class is explicit at the return site: `Completed → FireCompletion`, `LoopBroken → FireCompletion`, `UserQuit → FireRunAborted` ([PD-8](artifacts/implementation-decision-log.md#trivial-decisions)). `runCmuxOrchestratorWith` continues to return `nil` even on a fatal cmux timeout — the exit code propagates through `WorkspaceDone{ExitCode}` ([PD-11](artifacts/implementation-decision-log.md#trivial-decisions)).
- The re-fire timer is started inside `cmuxNotifier.EnterErrorMode` via `context.WithCancel(ctx)`, stopped via `cancelRefire()` inside `ExitErrorMode`, and protected by `defer notifier.ExitErrorMode()` at the construction site mirroring `defer sidebar.ClearAll(...)` ([PD-5](artifacts/implementation-decision-log.md#pd-5-the-re-fire-timer-lives-on-cmuxnotifier-context-cancellation-stops-it)). The 60-second cadence uses `time.NewTicker(60 * time.Second)`. The timer goroutine applies `ctx.Err()` after `ticker.C` fires to avoid one spurious RPC on a race with cancellation. The abort path stops the timer **before** issuing `FireRunAborted` so the timer cannot fire during shutdown ([PD-10](artifacts/implementation-decision-log.md#pd-10-the-re-fire-timer-is-stopped-before-the-abort-path-issues-firerunaborted)).
- In-flight call tolerance via a `resolved chan struct{}` on each error-mode session, closed by `ExitErrorMode` on the first resolution intent. The per-call goroutine checks `select { case <-resolved: ... default: ... }` after the RPC returns to satisfy spec D16's "post-answer outcome is non-fatal regardless of result" ([PD-12](artifacts/implementation-decision-log.md#pd-12-in-flight-call-after-resolution-is-treated-as-non-fatal-via-a-resolved-channel)).
- Documentation work-unit covering `docs/features/cmux-mode.md` (new Phase 5 section on the three notification classes + persistent re-fire), `docs/how-to/setting-up-cmux.md` (operator-facing description of the 60-second re-fire and the dismissal-resistance behavior), and `docs/code-packages/cmuxctl.md` (one new RPC wrapper, the typed `*TimeoutError`, the new `IntentType` constants, and the extended `Preflight` probe) ([PD-14](artifacts/implementation-decision-log.md#trivial-decisions)). The CLAUDE.md feature/code-package lists do not gain new entries — existing files are extended.

When operators run `pr9k --cmux --project-dir <repo>` against a project with a real workflow, they observe the Phase 5 demo: the workspace appears (Phases 1–2); the workflow runs to completion and exactly one completion notification (`pr9k run completed in <repo>`) appears in cmux's notification chrome; activating it brings the operator back to the pr9k workspace. If a step fails, an error-mode notification (`<step name> failed in <repo> — Focus the footer pane to respond`) appears within the cadence's first tick; if the operator dismisses it, it re-appears 60 seconds later; when the operator presses `q` in the footer pane, the timer stops at the first keystroke (carried by `IntentErrorQuitInitiated`), and one run-aborted notification (`pr9k run aborted in <repo>`) fires after the two-step confirmation completes. If the operator cancels the quit confirmation, the timer restarts from 0 (carried by `IntentErrorQuitCancelled`) and the re-fire cadence resumes until the operator answers.

## Context

- **Driving constraint:** without Phase 5, the operator who switched away from the pr9k workspace mid-run has no interrupt signal when the run reaches a terminal state or pauses awaiting a continue/retry/quit decision. Phase 4 gave the operator passive sidebar signal; Phase 5 closes the attention-gap with active notifications and is the smallest demoable slice of "interrupt me when the run needs me." Phase 6 then generalizes failure-mode handling (display loss, pane close, channel stall) and reuses Phase 5's `FireRunAborted` for every abort path.
- **Stakeholders:**
  - **Operators on macOS (cmux's primary platform)** — care that completion and abort notifications fire exactly once, that the error-mode notification re-fires reliably every 60 seconds, that activating a notification brings the pr9k workspace into focus, and that the run-aborted notification fires before the workspace closes.
  - **pr9k maintainers** — care that `cmuxNotifier` stays parallel to `cmuxSidebar` (single-responsibility), that the typed `*TimeoutError` is reusable by `cmuxSidebar`'s pre-existing latent-defect fix, that the new `IntentType` values do not break the existing `interactionchannel` test doubles, and that Phase 6's abort path can call `FireRunAborted` without modifying Phase 5 internals.
- **Future-state concern:** Phase 6 (failure-mode hardening) calls `cmuxNotifier.FireRunAborted` from every non-graceful path it adds — display loss, pane close, channel stall ([PD-1](artifacts/implementation-decision-log.md#pd-1-cmuxnotifier-is-a-new-struct-parallel-to-cmuxsidebar)). The construction-at-`runCmuxOrchestratorWith` pattern plus the outer `defer notifier.ExitErrorMode()` ([PD-5](artifacts/implementation-decision-log.md#pd-5-the-re-fire-timer-lives-on-cmuxnotifier-context-cancellation-stops-it)) is the OCP extension point Phase 6 reuses. The 60-second cadence is fixed in this release per spec [D3](artifacts/decision-log.md#d3-error-mode-notification-fires-on-entry-then-re-fires-every-60-seconds); the reopen trigger ("operators report 60s is too spammy / too slow") is recorded for the next planner.
- **Out-of-scope boundary:** per-pane click-to-focus targeting ([spec Deferred (YAGNI)](feature-specification.md#deferred-yagni)); operator-configurable cadence (spec D3); operator-configurable notification icon/color/urgency/sound (spec Out of Scope); distinct text per abort sub-reason (spec Deferred (YAGNI)); same-repo concurrent-run disambiguation in notification text (spec Deferred (YAGNI)); abort-completion urgency differentiation (spec Deferred (YAGNI)); notification history pane inside pr9k (spec Out of Scope); failure paths that bypass orchestrator shutdown (display loss, pane close, forced kill — Phase 6 scope per spec [D15](artifacts/decision-log.md#trivial-decisions)); per-method enumeration of the 12 existing `CmuxClient` methods (Phase 4 YAGNI deferral stands); `NotifierPort` interface (deferred under YAGNI — single implementation today).

## Team Composition and Participation

| Specialist | Status | Key Input |
|------------|--------|-----------|
| `project-manager` | Coordinator | Aggregated R1 deterministically (spec-maturity gate did not trip); routed OQ-1 to the user, resolved OQ-2/OQ-3 via evidence in R2; applied the YAGNI rule at Step 7.5; synthesized this plan and the decision log. |
| `software-architect` | Active (R1) | Eight findings (A1–A8) covering the parallel `cmuxNotifier` struct, one-new-method extension, capability-check shape, typed timeout error, timer lifecycle, terminal-notification firing site, abort-path narrow reading, and Phase 6 forward compatibility. |
| `concurrency-analyst` | Active (R1) | Eleven findings (C1–C11) covering mutex coverage on `cmuxNotifier`, synchronous OnModeChange callback ordering, mutex coverage on shared fields, typed-timeout-error necessity, four-way error classifier, two-step quit reframing (C6 — settled by code reading), the `ctx.Err()` guard after `ticker.C`, `defer notifier.ExitErrorMode()` safety net, `RealClient.Stop()` already drains (negative result), the 60-second cadence is not tightenable (negative result), and the FakeClient pattern. |
| `behavioral-analyst` | Active (R1) | Eleven findings (B1–B11) covering the `ExitReason → FireX` mapping table, terminal-notification firing site, the typed timeout error, OnModeChange ordering, capability-check mechanism, timer-stop-before-`FireRunAborted` ordering, narrow reading of D10's abort-path scope, the `resolved chan struct{}` for D16, the `cmuxSidebar.LastStepName()` accessor, `runCmuxOrchestratorWith` returns `nil` on fatal timeout (negative result), and the cmux-mode D5 disagreement with `concurrency-analyst` C6 (resolved by code reading in favor of C6). |
| `junior-developer` | Active (R1) | Ten findings (JD-001–JD-010) covering the cmux v2 wire-method verification gate, capability-check method-name precedent, typed timeout error necessity, sustained-failure case (already settled by spec D17), notification text examples, hook-list refactor (deferred — Phase 4 trigger not met), documentation work-unit (standards-required), DoD checklist scope, hook-list complexity threshold, and the cmux version pin question (resolved — no bump). |
| `user-experience-designer` | Not engaged | UX contributions already landed at spec stage (D2 footer-pane wording, D3 60s cadence, D4 dismissal-resistance, D5 first-keystroke timer stop). No new operator-facing surface in implementation beyond what the spec settled. |
| `adversarial-security-analyst` | Not engaged | No new auth / PII / untrusted-input / secrets path. Notification text contains repo basename (operator-supplied via `--project-dir`) and step name (operator-supplied via `config.json`) — both already trusted inputs. Same socket transport, same trust model as Phases 1–4. |
| `devops-engineer` | Not engaged | No new SLO / observability surface / rollout machinery. The per-run log file is the single diagnostic sink per [parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode). |
| `data-engineer` | Not engaged | No schema change, no migration, no persistent data. |
| `system-architect` | Not engaged | The pr9k↔cmux contract is settled at [parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record); no new bounded-context split. |
| `test-engineer` | Not engaged | Downstream skill — extends the existing `FakeClient` + `FakeInteractionChannel` adapter test patterns; W4 specifies the test surface directly. |
| `risk-analyst` / `structural-analyst` / `edge-case-explorer` / `system-architect` / `gap-analyzer` / `content-auditor` / `adversarial-validator` / `information-architect` | Not engaged | RAID items are individually scoped to specific findings; no portfolio-level prioritization needed. Edge cases were exhaustively surfaced at spec stage (EC1–EC9, F1–F21). |

## Implementation Approach

### Architecture and Integration Points

**New file: `src/cmd/pr9k/cmux_notifier.go`** ([PD-1](artifacts/implementation-decision-log.md#pd-1-cmuxnotifier-is-a-new-struct-parallel-to-cmuxsidebar)). Introduces one type:

- `cmuxNotifier` — owns the cmux RPC client, the workspace handle, a `*logger.Logger`, a sidebar pointer (read-only — for `LastStepName()`), and per-error-mode-session state (`errorActive bool`, `snapshotName string`, `cancelRefire context.CancelFunc`, `resolved chan struct{}`). The struct is single-purpose: it knows nothing about header / footer / sidebar concerns beyond the snapshot of `lastStepName` it captures at error-mode entry. Method set: `FireCompletion(ctx) error`, `FireRunAborted(ctx) error`, `EnterErrorMode(ctx, stepName string) error`, `ExitErrorMode() error`, `RestartErrorModeTimer(ctx)`. A `sync.Mutex` covers the four mutable fields ([PD-5](artifacts/implementation-decision-log.md#pd-5-the-re-fire-timer-lives-on-cmuxnotifier-context-cancellation-stops-it)); reads are short and snapshot-then-unlock per `docs/coding-standards/concurrency.md`. Non-timeout RPC errors are logged via `*logger.Logger` and swallowed (spec D7); timeouts propagate fatal **except** when the call is part of the abort path or fires after the resolved channel has been closed (spec D10, D16).

**Modification: `src/internal/cmuxctl/client.go`** ([PD-2](artifacts/implementation-decision-log.md#pd-2-one-new-workspacenotify-method-covers-all-three-notification-classes)). Add one method signature to `CmuxClient`:

```go
WorkspaceNotify(ctx context.Context, ws Workspace, class NotificationClass, body string) error
```

`NotificationClass` is a typed string constant in the same package with three values: `NotificationCompletion`, `NotificationRunAborted`, `NotificationErrorMode`. Body text is composed by `cmuxNotifier` per spec [D2](artifacts/decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-footer-pane-directive)'s canonical wording; the `RealClient` wrapper translates the class + body into cmux's verified `notification.*` wire method and the JSON param shape. The class is internal to pr9k — cmux receives a single text body per call.

**Modification: `src/internal/cmuxctl/real.go`** ([PD-4](artifacts/implementation-decision-log.md#pd-4-typed-timeouterror-replaces-the-fmterrorf-timeout-string)). Two changes:

1. Add a `WorkspaceNotify` wrapper over `c.do(ctx, method, params)`. The wire-level method name is `notification.create_for_target` with `notification.create_for_caller` as the fallback (verified against cmux source at commit `2f96c15c2` before commit — see the pre-commit gate in [Open Items](#open-items)). The `target` parameter is the workspace UUID/ref; param shape matches the existing `WorkspaceClose` / `WorkspaceSelect` conventions (`workspace_id` field). The class is encoded into the wire method or body per cmux's contract (verified at the gate).
2. Replace the existing `fmt.Errorf("cmuxctl: %s timed out after %s", method, duration)` at the timeout site with a new typed `*TimeoutError`:

```go
type TimeoutError struct { Method string; Duration time.Duration }
func (e *TimeoutError) Error() string { return fmt.Sprintf("cmuxctl: %s timed out after %s", e.Method, e.Duration) }
```

The error is exported so `cmuxNotifier`, `cmuxSidebar`, and any future caller can do `var te *cmuxctl.TimeoutError; errors.As(err, &te)`. As part of the same change, `cmuxSidebar`'s existing `errors.Is(err, context.DeadlineExceeded)` timeout-detection (a latent pre-existing defect that does not catch queue-level timeouts) is corrected to `errors.As(err, &te)`.

**Modification: `src/internal/cmuxctl/fake.go`** ([PD-2](artifacts/implementation-decision-log.md#pd-2-one-new-workspacenotify-method-covers-all-three-notification-classes)). Add one new `Func` field and one recorder slice:

```go
WorkspaceNotifyFunc func(ctx context.Context, ws Workspace, class NotificationClass, body string) error
NotifyCalls []NotifyCall // ordered record for test ordering assertions
```

The recorder slice is protected by the existing `FakeClient` mutex. The compile-time assertion `var _ CmuxClient = (*FakeClient)(nil)` enforces the implementation.

**Modification: `src/internal/cmuxctl/preflight.go`** ([PD-3](artifacts/implementation-decision-log.md#pd-3-preflight-is-extended-with-one-probe-call-to-workspacenotify)). After the existing `SystemIdentify` step succeeds, issue one probe call: `client.WorkspaceNotify(ctx, Workspace{}, NotificationCompletion, "")`. If the returned error matches `isMethodNotFound(err)` (which checks `*CmuxError` for `Code == "method_not_found"` OR `Code == "unknown_method"`), Preflight fails with a method-named error: `"cmuxctl: cmux build does not expose required notification method WorkspaceNotify (notification.create_for_target); upgrade cmux"`. Any other error (zero-workspace rejection, network error, anything else) is treated as method-exists and the probe passes. The probe is fire-and-forget for the test response (the response body is discarded); only the error code is consulted. No other existing method is probed (Phase 4's per-method enumeration deferral stands).

**Modification: `src/internal/interactionchannel/`** ([PD-7](artifacts/implementation-decision-log.md#pd-7-two-new-intenttype-values-make-spec-d5-implementable-in-cmux-mode)). Add two new `IntentType` constants:

- `IntentErrorQuitInitiated` — emitted by the footer pane when `q` is pressed in `ModeError`, before `enterMode(ui.ModeQuitConfirm)`. Carries no additional payload.
- `IntentErrorQuitCancelled` — emitted by the footer pane when `n` or `esc` is pressed in `ModeQuitConfirm` and the previous mode was `ModeError`. Carries no additional payload.

Both intents are mechanically additive: existing intent types and their wire shapes are unchanged. The JSON `"type"` discriminator gains two new string values. Per-role buffered-channel routing follows the existing pattern. Per `docs/coding-standards/versioning.md`, the IPC protocol is not part of pr9k's public API surface (internal to one process tree), so the contract change does not require a CLI / config schema bump.

**Modification: `src/cmd/pr9k/cmux_footer_machine.go`** ([PD-7](artifacts/implementation-decision-log.md#pd-7-two-new-intenttype-values-make-spec-d5-implementable-in-cmux-mode)). Two new emit sites:

1. In `handleError`, when `q` is pressed and the state transition to `ModeQuitConfirm` is about to occur, emit `RecordIntent(IntentErrorQuitInitiated)` **before** calling `enterMode(ui.ModeQuitConfirm)`. The emit ordering matters for spec D5 — the orchestrator must see the timer-stop signal at the first keystroke moment, not after `y` confirms.
2. In `handleQuitConfirm`, when `n` or `esc` is processed and the prior mode was `ModeError`, emit `RecordIntent(IntentErrorQuitCancelled)` before restoring the previous mode.

**Modification: `src/cmd/pr9k/cmux_workflow.go`** ([PD-6](artifacts/implementation-decision-log.md#pd-6-three-step-ordering-of-error-mode-effects-is-enforced-by-sequential-statements-in-the-onmodechange-callback), [PD-9](artifacts/implementation-decision-log.md#pd-9-terminal-notifications-fire-inside-runcmuxworkflowadapted-between-workflowrun-returning-and-sidebarclearall), [PD-10](artifacts/implementation-decision-log.md#pd-10-the-re-fire-timer-is-stopped-before-the-abort-path-issues-firerunaborted)). Five changes:

1. `runCmuxWorkflowAdapted` gains a `notifier *cmuxNotifier` parameter, passed from the outer caller alongside the existing `sidebar *cmuxSidebar`.
2. The existing `keyHandler.SetOnModeChange` closure is augmented in place. The order inside the closure when `mode == ui.ModeError`:
   1. **Footer broadcast** (already first in the closure — Phase 3 behavior, unchanged).
   2. **Sidebar mutation** (already second — `sidebar.EnterErrorMode(ctx)`, Phase 4 behavior, unchanged).
   3. **Notifier mutation** (new — `notifier.EnterErrorMode(ctx, sidebar.LastStepName())`).
   The three statements run sequentially in the same goroutine, so spec [D12](artifacts/decision-log.md#d12-error-mode-effects-are-ordered-footer-broadcast-state-bit-and-timer-notification-call)'s ordering is enforced structurally. The notifier's `EnterErrorMode` itself fires the initial notification synchronously inside the callback body — not in a spawned goroutine — so the state bit is set before the call begins ([PD-6](artifacts/implementation-decision-log.md#pd-6-three-step-ordering-of-error-mode-effects-is-enforced-by-sequential-statements-in-the-onmodechange-callback)).
3. The `keyAdapterLoop` (existing in `cmux_workflow.go`) gains four new branches for intent routing:
   - `IntentErrorQuitInitiated` → `notifier.ExitErrorMode()`. This is the spec D5 first-keystroke stop.
   - `IntentErrorQuitCancelled` → `notifier.RestartErrorModeTimer(ctx)`. This is the spec D5 cancelled-quit timer restart.
   - `IntentContinue` → `notifier.ExitErrorMode()`. The footer already emits this at the first keystroke for continue (no two-step confirmation).
   - `IntentRetry` → `notifier.ExitErrorMode()`. Same — emitted at the first keystroke.
4. Terminal-notification firing site — immediately after `workflow.Run` returns (carrying `RunResult.ExitReason`) and before `sidebar.ClearAll(ctx)`:

```go
switch result.ExitReason {
case workflow.ExitReasonCompleted, workflow.ExitReasonLoopBroken:
    _ = notifier.FireCompletion(ctx)
case workflow.ExitReasonUserQuit:
    notifier.ExitErrorMode() // stop the re-fire timer if it was running
    _ = notifier.FireRunAborted(ctx)
}
sidebar.ClearAll(ctx)
```

Both `FireCompletion` and `FireRunAborted` are best-effort from the caller's perspective — the function bodies handle the abort-path timeout exception internally per [PD-10](artifacts/implementation-decision-log.md#pd-10-the-re-fire-timer-is-stopped-before-the-abort-path-issues-firerunaborted). The `notifier.ExitErrorMode()` call before `FireRunAborted` is required even when error mode was not active — `ExitErrorMode` is idempotent on inactive state.
5. `runCmuxOrchestratorWith` (in `src/cmd/pr9k/cmux_pane.go`) gains a parallel construction of `*cmuxNotifier` next to the existing `*cmuxSidebar`, with `defer notifier.ExitErrorMode()` immediately after construction (best-effort safety net for panic / abort paths, mirroring `defer sidebar.ClearAll(...)`). The notifier pointer is passed into `runCmuxWorkflowAdapted`.

**Modification: `src/cmd/pr9k/cmux_sidebar.go`** ([PD-13](artifacts/implementation-decision-log.md#pd-13-cmuxsidebar-gains-a-laststepname-accessor)). Add one new method:

```go
func (s *cmuxSidebar) LastStepName() string {
    s.mu.Lock(); defer s.mu.Unlock()
    return s.lastStepName
}
```

Snapshot-then-unlock per `docs/coding-standards/concurrency.md`. The accessor is read by `cmuxNotifier.EnterErrorMode` at error-mode entry to satisfy spec [D13](artifacts/decision-log.md#d13-step-name-is-snapshotted-at-error-mode-entry-not-recomputed-at-each-re-fire)'s "snapshot at entry" commitment — the snapshot is captured into `cmuxNotifier.snapshotName` and reused across every 60-second re-fire of that error-mode session.

### Data Model and Persistence

Phase 5 introduces no new persistent state, no schema change, and no on-disk artifact. The notifier's in-memory state (`errorActive`, `snapshotName`, `cancelRefire`, `resolved`) is per-error-mode-session, allocated at error-mode entry and released at exit. The on-disk log artifacts at `<projectDir>/.pr9k/logs/<run-stamp>/` remain unchanged from Phase 4 ([parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode)) — non-timeout notification errors are logged into the existing per-run log file alongside the existing sidebar-failure logs.

### Runtime Behavior

**Launch sequence (extending Phase 4 lifecycle):**

1. `pr9k --cmux --project-dir <repo>` proceeds through Phase 1 preflight. The preflight now includes one additional probe call to `WorkspaceNotify` ([PD-3](artifacts/implementation-decision-log.md#pd-3-preflight-is-extended-with-one-probe-call-to-workspacenotify)). If cmux returns `method_not_found` or `unknown_method`, the launch aborts with a method-named error per spec [D8](artifacts/decision-log.md#d8-the-phase-1-capability-check-is-extended-to-cover-phase-5s-notification-methods).
2. `RunPhase1` creates the workspace and spawns the four pane processes (unchanged from Phase 4).
3. The orchestrator pane's `runCmuxOrchestratorWith` constructs `*cmuxSidebar` (Phase 4) and `*cmuxNotifier` (Phase 5) in sequence, with `defer sidebar.ClearAll(context.Background())` and `defer notifier.ExitErrorMode()` registered in order. The notifier pointer is passed into `runCmuxWorkflowAdapted`.
4. `runCmuxWorkflowAdapted` constructs `sidebarAwareHeader` (Phase 4 — unchanged) and registers the augmented `OnModeChange` closure containing footer-broadcast + sidebar-mutation + notifier-mutation. The `keyAdapterLoop` is started with the four new intent branches.
5. `workflow.Run` runs; no notifications fire during normal step progress.

**Per-event notification firing:**

- **Successful completion** (`workflow.Run` returns with `ExitReason ∈ {Completed, LoopBroken}`):
  1. `notifier.FireCompletion(ctx)` fires the `pr9k run completed in <repo>` notification (one shot).
  2. `sidebar.ClearAll(ctx)` runs.
  3. `WorkspaceDone{ExitCode: 0}` is broadcast to display panes.
  4. The workspace remains open per [parent D14](../artifacts/decision-log.md#d14-workspace-closure-is-operator-initiated); the operator dismisses it.

- **Operator-initiated quit at any point in the run** (`workflow.Run` returns with `ExitReason == UserQuit`):
  1. `notifier.ExitErrorMode()` stops the re-fire timer if it was active (idempotent on inactive state) ([PD-10](artifacts/implementation-decision-log.md#pd-10-the-re-fire-timer-is-stopped-before-the-abort-path-issues-firerunaborted)).
  2. `notifier.FireRunAborted(ctx)` fires the `pr9k run aborted in <repo>` notification. Abort-path semantics apply: timeout is treated as non-fatal per spec [D10](artifacts/decision-log.md#d10-abort-path-notification-calls-treat-every-failure-including-timeout-as-non-fatal-explicit-exception-to-parent-d15).
  3. `sidebar.ClearAll(ctx)`.
  4. `WorkspaceDone{ExitCode: 1}`.

- **Step fails → error mode** (`keyHandler.SetMode(ui.ModeError)` fires):
  1. Footer broadcast (Phase 3 unchanged).
  2. Sidebar `EnterErrorMode` (Phase 4 unchanged — appends `" — awaiting input"`).
  3. `notifier.EnterErrorMode(ctx, sidebar.LastStepName())`: snapshot the step name; set `errorActive = true`; allocate `resolved chan struct{}`; spawn the re-fire timer goroutine on a child context derived from `context.WithCancel(ctx)`; fire the initial notification call synchronously.
  4. The re-fire timer goroutine: `time.NewTicker(60 * time.Second)`; on each tick, check `ctx.Err()` (return if cancelled), then issue a fresh `WorkspaceNotify` with `NotificationErrorMode` class. After the call returns, check `select { case <-resolved: log-and-continue; default: classify the error }` to satisfy spec [D16](artifacts/decision-log.md#trivial-decisions).

- **Operator answers continue or retry** (footer emits `IntentContinue` or `IntentRetry` at first keystroke):
  - `keyAdapterLoop` calls `notifier.ExitErrorMode()`: close `resolved`; call `cancelRefire()`; clear `errorActive`. Any in-flight call's outcome (success / non-timeout error / timeout) is treated as non-fatal because `resolved` was closed before the call returned.

- **Operator presses `q` in error mode** (footer emits `IntentErrorQuitInitiated` at the first keystroke):
  - `keyAdapterLoop` calls `notifier.ExitErrorMode()`. The two-step `q`/`y` confirmation proceeds in the footer pane.
  - If the operator confirms with `y`: footer emits `IntentQuit`; `workflow.Run` returns with `ExitReason == UserQuit`; the abort path above fires `FireRunAborted`.
  - If the operator cancels with `n`/`esc`: footer emits `IntentErrorQuitCancelled`; `keyAdapterLoop` calls `notifier.RestartErrorModeTimer(ctx)` which allocates a fresh `resolved` channel and respawns the re-fire goroutine on a fresh context derived from `ctx`. The operator continues to receive the periodic prompt until they answer.

**Error paths:**

- **Non-timeout notification error** (workspace handle rejected, malformed value, transient cmux error, method-disappeared mid-run per spec [F11 edge case](artifacts/team-findings.md#f11-notification-text-examples-read-like-authoritative-strings-but-were-marked-for-example)): logged to the per-run log file; workflow continues. The next 60-second re-fire is attempted normally. Per spec [D7](artifacts/decision-log.md#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15).
- **Sustained non-timeout failures** (every re-fire fails): per spec [D17](artifacts/decision-log.md#trivial-decisions), this is an acceptable degraded state. The sidebar's `" — awaiting input"` pill is the persistent backstop signal. No threshold, no escalation.
- **Notification call timeout — not abort-path**: classified by `errors.As(err, &te)` where `te *cmuxctl.TimeoutError`; treated as fatal per [parent D15](../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal); triggers the abort path. Phase 5 surfaces this through the abort path's `FireRunAborted` call (operator receives one run-aborted notification instead of one completion notification — a faithful signal).
- **Notification call timeout — abort-path** (`FireRunAborted`'s own call): treated as non-fatal per spec [D10](artifacts/decision-log.md#d10-abort-path-notification-calls-treat-every-failure-including-timeout-as-non-fatal-explicit-exception-to-parent-d15); the abort sequence continues to completion. The narrow reading of "abort-path" is: only `FireRunAborted` itself ([PD-9](artifacts/implementation-decision-log.md#pd-9-terminal-notifications-fire-inside-runcmuxworkflowadapted-between-workflowrun-returning-and-sidebarclearall)). All other cmux calls (including `sidebar.ClearAll`) keep their normal D7/D17 routing.
- **In-flight call after answer**: outcome (success / non-timeout error / timeout) is treated as non-fatal per spec [D16](artifacts/decision-log.md#trivial-decisions) via the `resolved chan struct{}` check ([PD-12](artifacts/implementation-decision-log.md#pd-12-in-flight-call-after-resolution-is-treated-as-non-fatal-via-a-resolved-channel)).

### External Interfaces

**CLI.** No new flag, no new sub-command, no change to existing flag behavior.

**Cmux JSON-RPC 2.0.** One new method called by pr9k against cmux v0.64.7's socket: `notification.create_for_target` (primary; verified against cmux source at commit `2f96c15c2` per [parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record) precedent), with `notification.create_for_caller` as fallback if the target field shape differs. The exact wire method name + JSON param shape is verified at the pre-commit gate ([OI-1](#open-items)). Method body carries the text composed by `cmuxNotifier` per spec [D2](artifacts/decision-log.md#d2-notification-text-is-repo-name-plus-lifecycle-verb-error-text-incorporates-the-footer-pane-directive).

**Cmux notification surface (operator-visible).** Three notification classes appear in cmux's chrome targeting the pr9k workspace:

- Completion: `pr9k run completed in <repo-basename>` — one-shot, fires before workspace close per spec [D11](artifacts/decision-log.md#d11-run-aborted-and-completion-notifications-fire-before-workspaceclose-while-the-workspace-handle-is-still-valid).
- Run aborted: `pr9k run aborted in <repo-basename>` — one-shot, fires before workspace close.
- Error mode: `<step name> failed in <repo-basename> — Focus the footer pane to respond` — persistent at 60-second cadence; survives dismissal per spec [D4](artifacts/decision-log.md#d4-dismissal-does-not-suppress-re-fire-pr9k-owns-the-cadence); first keystroke (`q`, `c`, or `r`) stops the cadence per spec [D5](artifacts/decision-log.md#d5-resolution-of-the-error-prompt-stops-re-fire-immediately).

**Internal IPC contract change (`interactionchannel`).** Two new `IntentType` discriminator values (`IntentErrorQuitInitiated`, `IntentErrorQuitCancelled`) emitted by the footer pane and consumed by the orchestrator. Mechanically additive — existing intents unchanged ([PD-7](artifacts/implementation-decision-log.md#pd-7-two-new-intenttype-values-make-spec-d5-implementable-in-cmux-mode)).

**Operator-visible terminal output (inside the pr9k workspace).** Unchanged from Phase 4. The header / log / footer panes render the same content; the notification surface is parallel.

## Decomposition and Sequencing

| # | Work Unit | Delivers | Depends On | Verification |
|---|-----------|----------|------------|--------------|
| W1 | Typed `*cmuxctl.TimeoutError` exported from `internal/cmuxctl`; `RealClient` timeout site updated to return the typed error; `cmuxSidebar`'s existing `errors.Is(err, context.DeadlineExceeded)` corrected to `errors.As(err, &te)` ([PD-4](artifacts/implementation-decision-log.md#pd-4-typed-timeouterror-replaces-the-fmterrorf-timeout-string)) | A single exported timeout sentinel reusable across `cmuxNotifier`, `cmuxSidebar`, and any future caller; a pre-existing latent defect in sidebar timeout detection is fixed | — | Unit tests: a forced `time.Sleep` longer than `DefaultTimeout` against a `runphase1_test.go`-style fake socket returns `*cmuxctl.TimeoutError`; `cmuxSidebar`'s timeout path triggers fatal-abort under the corrected classifier |
| W2 | `CmuxClient.WorkspaceNotify` method signature; `NotificationClass` typed constants; `RealClient` wrapper translating to `notification.create_for_target` (or fallback `notification.create_for_caller`); `FakeClient.WorkspaceNotifyFunc` + `NotifyCalls` recorder slice ([PD-2](artifacts/implementation-decision-log.md#pd-2-one-new-workspacenotify-method-covers-all-three-notification-classes)); `Preflight` extended with one probe + `isMethodNotFound` helper checking both `"method_not_found"` and `"unknown_method"` ([PD-3](artifacts/implementation-decision-log.md#pd-3-preflight-is-extended-with-one-probe-call-to-workspacenotify)) | Production and test clients implement the one new RPC; preflight fails fast with a method-named error on cmux builds that lack notification methods | W1 | Unit tests: `RealClient.WorkspaceNotify` round-trip against a fake cmux socket (verifies the wire method name and param shape match the cmux source at commit `2f96c15c2` — see [OI-1](#open-items) pre-commit gate); `FakeClient.NotifyCalls` records calls in order; `Preflight` returns the method-named error when the fake cmux returns `*CmuxError{Code: "method_not_found"}` AND when it returns `Code: "unknown_method"`; `Preflight` passes when the fake returns any other error (zero-workspace rejection, etc.); `var _ CmuxClient = (*FakeClient)(nil)` and `var _ CmuxClient = (*RealClient)(nil)` compile |
| W3 | Two new `IntentType` constants in `internal/interactionchannel`; JSON `"type"` discriminator values; existing test doubles (`FakeInteractionChannel`, `FakeDisplayPane`, `FakeFooterKeySource`) extended; emit sites added in `cmux_footer_machine.go` (`handleError` and `handleQuitConfirm`) ([PD-7](artifacts/implementation-decision-log.md#pd-7-two-new-intenttype-values-make-spec-d5-implementable-in-cmux-mode)) | The footer state machine emits a timer-stop signal at the first `q` keystroke and a timer-restart signal on cancelled confirmation, making spec D5 implementable in cmux mode without spec narrowing | — | Unit tests: pressing `q` in `ModeError` emits `IntentErrorQuitInitiated` before `enterMode(ModeQuitConfirm)`; pressing `n`/`esc` in `ModeQuitConfirm` (prior mode `ModeError`) emits `IntentErrorQuitCancelled`; pressing `n`/`esc` in `ModeQuitConfirm` (prior mode `ModeNormal`) does NOT emit `IntentErrorQuitCancelled`; race detector clean |
| W4 | `cmuxNotifier` struct in a new `src/cmd/pr9k/cmux_notifier.go`; `cmuxSidebar.LastStepName()` accessor ([PD-13](artifacts/implementation-decision-log.md#pd-13-cmuxsidebar-gains-a-laststepname-accessor)); `cmuxNotifier` construction hoisted to `runCmuxOrchestratorWith` with `defer notifier.ExitErrorMode()`; `runCmuxWorkflowAdapted` gains a `notifier` parameter; the augmented `OnModeChange` closure calls `notifier.EnterErrorMode(ctx, sidebar.LastStepName())` after the existing sidebar mutation ([PD-1](artifacts/implementation-decision-log.md#pd-1-cmuxnotifier-is-a-new-struct-parallel-to-cmuxsidebar), [PD-5](artifacts/implementation-decision-log.md#pd-5-the-re-fire-timer-lives-on-cmuxnotifier-context-cancellation-stops-it), [PD-6](artifacts/implementation-decision-log.md#pd-6-three-step-ordering-of-error-mode-effects-is-enforced-by-sequential-statements-in-the-onmodechange-callback)); `keyAdapterLoop` gains four intent branches; terminal-notification firing sites added between `workflow.Run` return and `sidebar.ClearAll(ctx)` ([PD-9](artifacts/implementation-decision-log.md#pd-9-terminal-notifications-fire-inside-runcmuxworkflowadapted-between-workflowrun-returning-and-sidebarclearall), [PD-10](artifacts/implementation-decision-log.md#pd-10-the-re-fire-timer-is-stopped-before-the-abort-path-issues-firerunaborted)); `resolved chan struct{}` per error-mode session ([PD-12](artifacts/implementation-decision-log.md#pd-12-in-flight-call-after-resolution-is-treated-as-non-fatal-via-a-resolved-channel)) | The notifier adapter compiles, integrates with the orchestrator, fires at the right events, stops the timer on the right signals, and tolerates post-resolution in-flight calls | W1, W2, W3 | Unit tests using `FakeClient`: `EnterErrorMode` records exactly one `NotifyCalls` entry on entry with class `NotificationErrorMode`; an injected clock/ticker advance produces one re-fire call per simulated 60-second tick; `IntentErrorQuitInitiated` arriving on the key-adapter loop closes the `resolved` channel and the next ticker fire does not produce another call; `IntentErrorQuitCancelled` produces a fresh `resolved` channel and restarts the cadence; `IntentContinue` / `IntentRetry` close `resolved`; `FireCompletion` / `FireRunAborted` are called exactly once at the matching `ExitReason`; `FireRunAborted` returns nil even when the underlying RPC returns `*cmuxctl.TimeoutError` (abort-path non-fatal); a `WorkspaceNotify` call that returns after `resolved` is closed is logged-not-fatal regardless of outcome |
| W5 | Test-suite cascade: update `cmux_workflow_test.go`, `cmux_pane_test.go`, `cmux_error_recovery_test.go`, `cmux_footer_*_test.go` for the new signatures, the new intent types, and the new firing assertions | Phase 1–4 test suite continues to pass under the new signatures; race detector clean | W4 | `cd src && go test -race ./cmd/pr9k/... ./internal/cmuxctl/... ./internal/interactionchannel/...` passes |
| W6 | Documentation: extend `docs/features/cmux-mode.md` (new Phase 5 section on three notification classes + persistent re-fire + dismissal-resistance); extend `docs/how-to/setting-up-cmux.md` (operator-facing description of the 60-second re-fire and how to silence it by answering the prompt — addresses spec F2); extend `docs/code-packages/cmuxctl.md` (one new RPC wrapper, the typed `*TimeoutError`, the new `IntentType` values, the extended `Preflight` probe) ([PD-14](artifacts/implementation-decision-log.md#trivial-decisions)) | Operators understand the Phase 5 demo and the persistent-re-fire behavior; maintainers can navigate the new RPC wrapper and the IPC additions | W2, W4 | `make build` passes; doc code blocks match production code; CLAUDE.md links still resolve |
| W7 | Version bump in `src/internal/version/version.go` (PATCH bump appropriate for the IPC additions and the new RPC method) | `pr9k --version` reflects the Phase 5 ship; release-readiness signal | W1–W6 | `make ci` passes; `pr9k --version` output matches |

## RAID Log

### Risks

| ID | Risk | Likelihood | Severity | Blast Radius | Reversibility | Owner | Mitigation |
|----|------|------------|----------|--------------|---------------|-------|------------|
| R1 | The wire-level `notification.*` method name or param shape differs from `notification.create_for_target` against the cmux source at commit `2f96c15c2`; the probe in `Preflight` may pass for the wrong reason or fail on a method that does exist under a different name | Medium (six candidate methods documented in the cmux dispatch shape; only `parent D-R4` manual gate verifies the exact match) | Medium (notifications fail silently per spec D7; the sidebar pill is the operator's remaining signal per spec D17) | Cmux mode operators | Reversible (single-line change in `RealClient.WorkspaceNotify`) | `software-architect` | Pre-commit verification gate ([OI-1](#open-items)): read cmux source at `2f96c15c2` before the wire call is committed; fallback to `notification.create_for_caller` if `create_for_target` rejects the workspace UUID/ref shape |
| R2 | The `*CmuxError.Code` string for missing methods is neither `"method_not_found"` nor `"unknown_method"` — `Preflight` would treat a missing method as "method exists" and the runtime falls through to spec D7's log-and-continue | Medium (no documented enumeration in the discovery notes; Phase 4's `WorkspaceList` workaround precedent shows cmux's actual codes have surprised the team before) | Low–Medium (silent runtime degradation per spec D17; the sidebar pill is the backstop) | Cmux mode operators | Reversible (one-line addition to `isMethodNotFound`) | `software-architect` | Pre-commit verification gate ([OI-2](#open-items)) at the manual cmux integration gate ([parent D-R4](../artifacts/decision-log.md#d-r4-manual-cmux-integration-gate) precedent); the helper already checks both candidates so the gate just confirms which one fires |
| R3 | The 60-second re-fire goroutine leaks if `cancelRefire()` is not called on a panic path — the goroutine survives `runCmuxOrchestratorWith`'s return and continues issuing notifications against a closed workspace handle | Low (the outer `defer notifier.ExitErrorMode()` mirrors `defer sidebar.ClearAll(...)`; both run on every exit path including panic) | Medium (orphan goroutine + invalid-handle errors flooding the log) | `cmuxNotifier` internals | Reversible (the defer is one line) | `software-architect` (Phase 6) | The defer at construction time is the structural mitigation; race detector exercises the panic path during W5 tests; Phase 6's failure-mode work tests cover non-graceful paths explicitly |
| R4 | Two `OnModeChange` mutations (sidebar then notifier) execute sequentially and the second one is missed if the first panics — the closure does not run the third statement | Low (sidebar mutation has been Phase 4 production for one phase already; no panic-class bugs observed) | Low (one missed notification per error-mode entry; the next 60-second re-fire would still occur if the panic was inside the *call*, not inside `EnterErrorMode` before the timer starts) | Cmux mode operators | Reversible (wrap the sidebar call in `defer recover()` if reports surface) | `software-architect` | Code review of the closure; spec D12 ordering is enforced by sequential statements per [PD-6](artifacts/implementation-decision-log.md#pd-6-three-step-ordering-of-error-mode-effects-is-enforced-by-sequential-statements-in-the-onmodechange-callback); the panic surface is not new (Phase 4 already runs sidebar mutation in this closure) |
| R5 | A re-fire call that lands after `IntentContinue` was processed (operator chose continue, workflow resumed, ran another step that itself enters error mode) collides with the new session's `resolved` channel — the in-flight call from the OLD session checks the NEW session's `resolved` and is misclassified | Low (the per-session `resolved` channel is allocated fresh in `EnterErrorMode` and read by the timer goroutine; the goroutine is cancelled before the new session starts) | Medium (one misclassified call per back-to-back error-mode session) | `cmuxNotifier` internals | Reversible (capture `resolved` into the timer goroutine's local variable at session start) | `concurrency-analyst` | The timer goroutine captures `resolved` and `snapshotName` as local variables at session entry; mutex-protected reads of the notifier state are short and snapshot-then-unlock |
| R6 | The two new `IntentType` values are ignored by Phase 1–4 test doubles that hard-coded the existing intent list | Medium (`FakeInteractionChannel` may assert on a closed switch over `IntentType`) | Low (test compilation failure caught at W5; no runtime impact) | Test suite | Reversible (add the two new cases) | `concurrency-analyst` | W5 cascade explicitly addresses the test-double extension; compile-time enum switches will surface the gap |

### Assumptions

| ID | Assumption | What Changes If Wrong | Verifier | Status |
|----|------------|-----------------------|----------|--------|
| A1 | cmux v0.64.7 at commit `2f96c15c2` exposes `notification.create_for_target` accepting a workspace UUID/ref as the `target` field | If it accepts only surface IDs or another shape, the fallback `notification.create_for_caller` is used; the orchestrator pane's "caller" surface is inside the pr9k workspace, so cmux's activate-notification-focuses-caller behavior should still bring the workspace into focus | `software-architect` at the pre-commit gate ([OI-1](#open-items)) | Unverified — verifiable against cmux source |
| A2 | The cmux v2 `*CmuxError.Code` for a missing method is either `"method_not_found"` or `"unknown_method"` | If it is a different string, the `isMethodNotFound` helper does not match and `Preflight` treats the missing method as method-exists; the runtime falls through to spec D7's log-and-continue path until corrected | `software-architect` at the manual cmux integration gate ([OI-2](#open-items)) | Unverified — verifiable at runtime against a cmux build with the method removed |
| A3 | The orchestrator's `keyHandler.SetOnModeChange` callback is the right place to wire `notifier.EnterErrorMode` — the callback fires synchronously on the orchestrator's mode-change event, before `workflow.Run` proceeds past the failing step | If `SetOnModeChange` is asynchronous or fires after the step state machine has moved on, the snapshotted step name may be stale by the time `EnterErrorMode` reads it | `concurrency-analyst` at W4 (verified against `src/internal/ui/ui.go:131-135` — single-field assignment, synchronous fire) | Verified during R1 — the callback is synchronous |
| A4 | The PATCH version bump is appropriate (one new RPC method, two new internal intent types, no CLI / config schema change) | If the IPC additions are deemed externally visible, a MINOR bump is appropriate (matching Phase 2/3/4 phase-per-MINOR precedent vs. the strict-standard reading of `docs/coding-standards/versioning.md:37`) | River at PR review | Unverified — user judgment |
| A5 | The 60-second cadence does not collide with `cmuxctl.RealClient.DefaultTimeout` (8 seconds) under normal conditions because the ticker constructed at error-mode entry is never recreated on fast call returns | If the call returns in <1ms repeatedly (e.g., cmux is up but rejects every call quickly), the cadence stays at 60 seconds — `time.NewTicker` ignores fast returns | `concurrency-analyst` at R1 (verified by C10 — negative result) | Verified during R1 |

### Issues (Currently Active)

| ID | Issue | Owner | Next Step |
|----|-------|-------|-----------|
| I1 | Spec D5's "first quit keystroke" was unimplementable in cmux mode because the footer pane handles `q`/`y` confirmation locally and does not surface `q` to the orchestrator | `concurrency-analyst` (raised C6); user (resolved OQ-1) | Addressed in W3 — two new `IntentType` values bridge the gap; spec D5 is preserved verbatim |
| I2 | The exact `notification.*` wire method, param shape, and `*CmuxError` code for missing methods are not pinned at plan time | `software-architect` at the pre-commit gate ([OI-1](#open-items), [OI-2](#open-items)) | Verified at the manual cmux integration gate (parent D-R4 precedent) before the wire calls are committed |

### Dependencies (External, Phase-Boundary)

| ID | Dependency | Phase | Notes |
|----|------------|-------|-------|
| Dep1 | Phase 2's `interactionchannel.Channel`, `cmuxHeader`, `runCmuxWorkflowAdapted` are landed and stable | Phase 2 | Phase 5 extends `interactionchannel`'s intent types and `runCmuxWorkflowAdapted`'s signature — both are mechanically additive |
| Dep2 | Phase 3's `KeyHandler.SetOnModeChange` callback registers a closure that is augmented (not replaced) by Phase 4 and Phase 5 | Phase 3 | Phase 5 chains a third effect into the same closure; the closure does not need a slice-of-hooks refactor (Phase 4 R3 trigger not met) |
| Dep3 | Phase 4's `cmuxSidebar`, `sidebarAwareHeader`, `cmuxctl.CmuxClient` interface (12 methods), `cmuxctl.Workspace` threading through the composition root are landed | Phase 4 | Phase 5 reads `sidebar.lastStepName` via the new `LastStepName()` accessor; the workspace-handle threading is reused for `cmuxNotifier` |
| Dep4 | Cmux v0.64.7 at commit `2f96c15c2` exposes the `notification.*` method dispatch shape documented in the build outline | Operator / cmux upstream | Pre-commit gate ([OI-1](#open-items)) verifies the exact method name before the wire call is committed |
| Dep5 | Phase 6 (failure-mode hardening) will call `cmuxNotifier.FireRunAborted` from its abort path | Phase 6 (future) | The hoisted construction + outer defer ([PD-5](artifacts/implementation-decision-log.md#pd-5-the-re-fire-timer-lives-on-cmuxnotifier-context-cancellation-stops-it)) is the integration point; no new interface is required from Phase 5 — `FireRunAborted` is a concrete method on `*cmuxNotifier` ([PD-15](artifacts/implementation-decision-log.md#trivial-decisions)) |
| Dep6 | Manual cmux integration gate ([parent D-R4](../artifacts/decision-log.md#d-r4-manual-cmux-integration-gate)) — Phase 5's pre-commit verifications align with the same gate Phase 4 used | Manual gate | The gate verifies the wire method name and the `*CmuxError` code at the same time |

## Testing Strategy

Test surface follows Phase 4's precedent: extend the existing `FakeClient`, `FakeInteractionChannel`, and adapter-test patterns; no new test harness; no live-cmux integration tests in this plan.

- **`cmuxctl.TimeoutError` unit tests (W1):** a forced timeout returns `*cmuxctl.TimeoutError`; `errors.As(err, &te)` succeeds; the `cmuxSidebar` timeout-detection path triggers fatal-abort under the corrected classifier.
- **`cmuxctl.RealClient.WorkspaceNotify` round-trip test (W2):** exercises a request/response cycle against a fake cmux socket using the existing `runphase1_test.go` pattern. Verifies the wire method name and the parameter shape against cmux 0.64.7 (the pre-commit gate at [OI-1](#open-items) confirms the actual value).
- **`cmuxctl.FakeClient` recorder tests (W2):** `WorkspaceNotifyFunc` + `NotifyCalls` slice exercised with focused unit tests; calls recorded in order with class + body.
- **`cmuxctl.Preflight` unit tests (W2):** `Preflight` returns the method-named error when the fake cmux returns `*CmuxError{Code: "method_not_found"}` AND when `Code: "unknown_method"`; passes when the fake returns any other error.
- **`interactionchannel` unit tests (W3):** new intent types serialize / deserialize correctly; existing test doubles tolerate the new types; `IntentErrorQuitInitiated` and `IntentErrorQuitCancelled` route through the per-role channels.
- **`cmux_footer_machine.go` unit tests (W3):** pressing `q` in `ModeError` emits `IntentErrorQuitInitiated` before `enterMode(ModeQuitConfirm)`; pressing `n`/`esc` in `ModeQuitConfirm` (prior mode `ModeError`) emits `IntentErrorQuitCancelled`; pressing `n`/`esc` in `ModeQuitConfirm` (prior mode `ModeNormal`) does NOT emit `IntentErrorQuitCancelled`.
- **`cmuxNotifier` unit tests (W4) using `FakeClient`:**
  - `EnterErrorMode` records one `NotifyCalls` entry with class `NotificationErrorMode`, body matching `<step> failed in <repo> — Focus the footer pane to respond`.
  - An injected clock/ticker advance produces exactly one re-fire call per simulated 60-second tick.
  - `ExitErrorMode` after a re-fire call has been issued but not returned: the call's outcome is logged-not-fatal regardless of return value (timeout, error, or success).
  - `RestartErrorModeTimer(ctx)` allocates a fresh `resolved` channel and produces a fresh tick cadence from 0.
  - `FireCompletion` records one `NotifyCalls` entry with class `NotificationCompletion`, body matching `pr9k run completed in <repo>`.
  - `FireRunAborted` records one entry, returns nil even when the underlying RPC returns `*cmuxctl.TimeoutError` (abort-path non-fatal per [PD-10](artifacts/implementation-decision-log.md#pd-10-the-re-fire-timer-is-stopped-before-the-abort-path-issues-firerunaborted)).
- **`cmux_workflow.go` integration test (W4) using `FakeClient` + `FakeInteractionChannel`:** drive a synthetic workflow through `runCmuxWorkflowAdapted` with three scenarios:
  1. Success path — one `NotificationCompletion` call before `sidebar.ClearAll`.
  2. Error mode → continue — one `NotificationErrorMode` call on entry; no additional calls after `IntentContinue` (assert against an injected clock advance equivalent to 120 simulated seconds).
  3. Error mode → quit (with intervening cancel) — `IntentErrorQuitInitiated` stops the timer; `IntentErrorQuitCancelled` restarts; second `IntentErrorQuitInitiated`; then `IntentQuit`; final `NotificationRunAborted` call.
- **Race detector (W4, W5):** `cd src && go test -race ./cmd/pr9k/... ./internal/cmuxctl/... ./internal/interactionchannel/...` is the required gate per `docs/coding-standards/testing.md`.
- **Test cascade (W5):** existing tests in `src/cmd/pr9k/` updated for the new `runCmuxOrchestratorWith` and `runCmuxWorkflowAdapted` signatures.

**No load-test, no fuzz, no concurrency-stress test required** — the cmux call surface is sequential per the existing `RealClient` queue, and `cmuxNotifier`'s state is short-lived per error-mode session.

## Security Posture

No new attack surface. The new `WorkspaceNotify` RPC method uses the same socket transport, the same filesystem-permission-based access control, the same `CMUX_SOCKET_PATH` validation, and the same per-call timeout policy as Phases 1–4. Notification body text contains the repo basename (operator-supplied via `--project-dir`, already a trusted input) and the step name (operator-supplied via `config.json`, already a trusted input); both are already free-form short strings with no parsing semantics on cmux's side. The two new `IntentType` values in `internal/interactionchannel` flow over the same Unix socket as existing intents, with the same UID/process-tree trust model. No new secret, no new auth path, no untrusted-input parsing.

## Operational Readiness

- **No new SLO** — notifications are best-effort per spec [D7](artifacts/decision-log.md#d7-non-timeout-notification-errors-are-logged-and-the-run-continues-timeouts-remain-fatal-per-parent-d15) / [D17](artifacts/decision-log.md#trivial-decisions); sustained non-timeout failures are an explicitly acceptable degraded state with the sidebar pill as the backstop.
- **No new infrastructure component** — uses the existing cmux socket connection.
- **No new feature flag** — Phase 5 ships as a behavioral extension of `--cmux` mode.
- **No new observability surface** — the existing per-run log file at `<projectDir>/.pr9k/logs/<run-stamp>/` is the diagnostic surface for notification failures, per [parent D17](../artifacts/decision-log.md#d17-log-file-artifacts-are-unchanged-in-cmux-mode).
- **Release notes** name the scope: cmux mode now fires three classes of notification (one-shot completion, one-shot run-aborted, persistent error-mode); error-mode notification re-fires every 60 seconds until the operator answers; preflight now requires cmux to expose `notification.*` methods. No cmux version-pin bump.
- **Rollback path:** revert the Phase 5 commit; `--cmux` mode reverts to Phase 4 behavior (no notifications). No state migration; no data loss; no operator-visible change beyond the absence of notifications.

## Definition of Done

- [ ] `cmuxctl.CmuxClient` exposes `WorkspaceNotify`; `NotificationClass` typed constants exist; `RealClient` and `FakeClient` implement; compile-time interface assertions pass.
- [ ] `*cmuxctl.TimeoutError` is exported; `RealClient`'s timeout site returns the typed error; `cmuxSidebar`'s existing `errors.Is(err, context.DeadlineExceeded)` is corrected to `errors.As(err, &te)`.
- [ ] `Preflight` issues one probe call to `WorkspaceNotify` and fails the launch with a method-named error when the response indicates `method_not_found` OR `unknown_method`.
- [ ] `cmuxNotifier` exists in `src/cmd/pr9k/cmux_notifier.go` with `FireCompletion`, `FireRunAborted`, `EnterErrorMode`, `ExitErrorMode`, `RestartErrorModeTimer`; `sync.Mutex` guards mutable fields; `defer notifier.ExitErrorMode()` runs at the construction site in `runCmuxOrchestratorWith`.
- [ ] `cmuxSidebar.LastStepName() string` accessor exists, snapshot-then-unlock under the existing sidebar mutex.
- [ ] `IntentErrorQuitInitiated` and `IntentErrorQuitCancelled` exist in `internal/interactionchannel`; the footer state machine emits them at the matching transitions; the orchestrator's `keyAdapterLoop` routes them to `notifier.ExitErrorMode()` and `notifier.RestartErrorModeTimer(ctx)` respectively.
- [ ] **The error-mode re-fire timer stops on the first resolution intent** (`IntentErrorQuitInitiated`, `IntentContinue`, `IntentRetry`) — verified by a test that advances an injected clock past 60 seconds after the intent and asserts no additional `NotifyCalls` entry is produced.
- [ ] **The cancelled-quit timer restart works** — verified by a test that emits `IntentErrorQuitInitiated`, then `IntentErrorQuitCancelled`, then advances the injected clock past 60 seconds and asserts exactly one fresh `NotifyCalls` entry.
- [ ] **The abort-path notification timeout is non-fatal** — verified by a test that returns `*cmuxctl.TimeoutError` from `FireRunAborted`'s underlying RPC and asserts `runCmuxWorkflowAdapted` returns `ExitReasonUserQuit` with no panic and no second abort cycle.
- [ ] Terminal notifications fire inside `runCmuxWorkflowAdapted` between `workflow.Run` returning and `sidebar.ClearAll(ctx)`; the `ExitReason → FireX` mapping is explicit at the return site.
- [ ] Step name is snapshotted at `EnterErrorMode` and reused across re-fires for the same error-mode session; a fresh session captures a fresh snapshot.
- [ ] `docs/features/cmux-mode.md`, `docs/how-to/setting-up-cmux.md`, `docs/code-packages/cmuxctl.md` updated.
- [ ] `version.Version` PATCH-bumped (or MINOR-bumped if A4 is overruled).
- [ ] `cd src && go test -race ./...` passes.
- [ ] `make ci` passes.
- [ ] **Pre-commit verification gates ([OI-1](#open-items), [OI-2](#open-items)) closed** before the wire call is merged — the exact `notification.*` method name and param shape, and the `*CmuxError` code string for missing methods, are confirmed against cmux source at commit `2f96c15c2` (or against runtime behavior at the manual cmux integration gate per [parent D-R4](../artifacts/decision-log.md#d-r4-manual-cmux-integration-gate)).
- [ ] Manual smoke: launch `pr9k --cmux` against a small real workflow, observe a completion notification; relaunch against a failing workflow, observe an error-mode notification; dismiss it, observe re-fire 60 seconds later; press `q` to confirm timer stops; cancel confirmation, observe re-fire restart; confirm quit, observe one run-aborted notification.

## Specialist Handoffs for Implementation

- **`software-architect`** — already engaged at planning (R1). Re-engage at the pre-commit gate ([OI-1](#open-items)) to verify the `notification.*` wire method and param shape against cmux source at commit `2f96c15c2`.
- **`test-engineer`** — dispatch when W4 begins. Owns the integration tests under `src/cmd/pr9k/cmux_notifier_test.go` and the timer-injection test harness; reuses the existing `FakeClient` + `FakeInteractionChannel` recorder pattern; does not author a new test framework.
- **`concurrency-analyst`** — already engaged at planning. Re-engage if race-detector output during W4/W5 surfaces a `cmuxNotifier` state machine bug (R5 in the RAID).
- **`behavioral-analyst`** — already engaged at planning. Re-engage at the manual cmux integration gate ([OI-2](#open-items)) if the `*CmuxError` code string differs from both candidates.
- **`adversarial-security-analyst`** — not engaged for Phase 5 (no new trust boundary).
- **`devops-engineer`** — not engaged for Phase 5 (no new infrastructure).
- **`user-experience-designer`** — re-engage only if operators report the 60-second cadence is spammy or too slow (spec D3 reopen trigger).

## Deferred (YAGNI)

### `NotifierPort` interface

- **Why deferred:** Single-implementation interface anti-pattern (YAGNI rule). `*cmuxNotifier` is the only implementation today; Phase 6's abort path calls the concrete type, not an interface. Rule of Three not met.
- **Reopen when:** a second concrete implementation of run-aborted firing appears (e.g., a non-cmux notification backend, an in-process notification sink for tests beyond the existing `FakeClient`, or a remote-call adapter), AND the second implementation cannot share `*cmuxNotifier`'s state via the existing struct.
- **Source:** R1 — `software-architect` A7 YAGNI list.

### `notification.dismiss` / `notification.list` methods on `CmuxClient`

- **Why deferred:** No Phase 5 call site. The persistent re-fire cadence (spec D3) plus pr9k-owned cadence (spec D4) does not need explicit dismiss or list calls — pr9k re-fires unconditionally regardless of cmux's dismissal state, and the operator's notification history is owned by cmux itself.
- **Reopen when:** Phase 5 or later adds a dismiss/history call site (for example, a "snooze all error-mode notifications for this run" affordance) — that would also reopen the spec-stage "snooze controls inside pr9k" Out of Scope decision.
- **Source:** R1 — `software-architect` A2, A7.

### `notification.create_for_surface` for per-pane targeting

- **Why deferred:** spec-stage Deferred (YAGNI) already covers per-pane click-to-focus targeting. Phase 5's notifications target the pr9k workspace handle; cmux's normal focus rules pick the active pane inside the workspace. The error-mode notification text already directs the operator to the footer pane.
- **Reopen when:** operators report that the inter-pane focus hop after activating the error-mode notification is friction worth an extra cmux dependency, AND the cmux v2 method for per-surface targeting is verified against cmux source per [parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record) precedent (the same reopen trigger the spec recorded).
- **Source:** spec-stage Deferred (YAGNI) — carried forward unchanged.

### Hook-list refactor of the `SetOnModeChange` closure

- **Why deferred:** Phase 4 RAID R3 set the reopen trigger as "a fourth independent effect that does not compose cleanly into the closure." Phase 5 adds two branches that fit cleanly: one new `EnterErrorMode` call inside the existing `ModeError` block, and four new branches in the separate `keyAdapterLoop` (which is not the `OnModeChange` closure). The closure remains three statements long when `mode == ui.ModeError`. No measured friction; the trigger is not met.
- **Reopen when:** a fourth independent effect with different state semantics arrives — for example, a Phase 6 channel-stall handler that also wants `OnModeChange` notification but must coordinate with `cmuxNotifier` state.
- **Source:** R1 — `junior-developer` JD-009; `software-architect` A6 commentary.

### Per-method capability enumeration of all 13 `CmuxClient` methods

- **Why deferred:** Phase 4's YAGNI deferral stands ([Phase 4 D-4](../phase-4-sidebar-mirroring/artifacts/implementation-decision-log.md#d-4-capability-check-stays-identify-only-the-cmux-version-pin-moves-0646--0647)). Phase 5 probes only the one new method (`WorkspaceNotify`) because that is the only one not already covered by Phase 4's version-pin + identify-only check. Enumerating the other 12 methods is a symmetry/completeness anti-pattern with no evidence — Phases 1–4 ship without it and the version pin satisfies the same evidence at a fraction of the surface.
- **Reopen when:** the Phase 4 reopen trigger fires ("a cmux minor release removes or renames a method pr9k calls and a runtime failure surfaces that the version pin did not prevent") — at that point per-method enumeration across all 13 methods becomes evidence-justified.
- **Source:** R1 — `software-architect` A3 plus Phase 4 YAGNI deferral.

## Open Items

- **OI-1 (Pre-commit verification gate — wire method shape):** verify the exact `notification.*` wire method name and JSON param shape against cmux source at commit `2f96c15c2` before the wire call is committed. Candidates: `notification.create_for_target` (primary) and `notification.create_for_caller` (fallback). The `RealClient.WorkspaceNotify` wrapper is written against the primary candidate; if the cmux source rejects the workspace-handle target, the fallback is used.
  - **Resolves when:** `software-architect` reads cmux source at the pinned commit and confirms the method name + param shape; the `RealClient` wrapper is updated to match if needed.
  - **Blocks implementation:** No — the plan is committable today. This is a pre-merge verification gate following [parent D-R2](../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record) precedent.

- **OI-2 (Pre-commit verification gate — `*CmuxError` code for missing methods):** verify the exact `*CmuxError.Code` string cmux v2 returns for a missing method. Candidates: `"method_not_found"` (primary) and `"unknown_method"` (fallback). The `isMethodNotFound` helper checks both; if neither matches, the helper is adjusted at the manual cmux integration gate.
  - **Resolves when:** `behavioral-analyst` confirms the code string at the manual cmux integration gate ([parent D-R4](../artifacts/decision-log.md#d-r4-manual-cmux-integration-gate) precedent), symmetric to how Phase 4's `WorkspaceList` workaround surfaced the `"id"/"ref"` vs `"workspace_id"/"workspace_ref"` JSON-tag discrepancy.
  - **Blocks implementation:** No — the plan is committable today. The helper already checks both candidates so the gate just confirms which one fires.

- **OI-3 (Version-bump magnitude — A4):** PATCH (by the strict reading of `docs/coding-standards/versioning.md:37`: backwards-compatible additions during 0.y.z) vs. MINOR (by Phase 2/3/4 phase-per-MINOR precedent). The plan ships at PATCH unless the user overrules at PR review. The constant is one line to change either way.
  - **Resolves when:** River decides at PR review.
  - **Blocks implementation:** No.

No other open items. Every R1 specialist finding was resolved by evidence (the cmux-mode D5 disputed claim, OQ-2, OQ-3), by user input (OQ-1 — the `IntentType` extension preserving spec D5 verbatim), or by implementation choice within the plan's authority.

## Summary

- **Outcome delivered:** three notification classes (completion, run-aborted, error-mode persistent at 60s) fire at the right lifecycle moments; the abort-path timeout is non-fatal per spec D10; the spec D5 first-keystroke timer-stop is implementable in cmux mode via two additive `IntentType` values; the sidebar pill remains the persistent backstop when notifications degrade.
- **Team size:** 5 specialists (4 in R1: `junior-developer`, `concurrency-analyst`, `behavioral-analyst`, `software-architect`; `project-manager` synthesis) — see [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md)
- **Rounds of facilitation:** 2 (R1 parallel specialist review; R2 resolution-loop closing OQ-1/OQ-2/OQ-3) — see [artifacts/implementation-iteration-history.md](artifacts/implementation-iteration-history.md)
- **Decisions committed:** 15 — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md)
- **Decisions settled by evidence:** 12 (PD-1, PD-2, PD-3, PD-4, PD-5, PD-6, PD-8, PD-9, PD-10, PD-11, PD-12, PD-13)
- **Decisions settled by user input:** 1 (PD-7 — the new `IntentType` values; OQ-1 user resolution)
- **Decisions settled by trivial agreement (no rejected alternatives):** 5 trivial (PD-8, PD-11, PD-14, PD-15, plus PD-16 documentation-cross-reference)
- **Rejected alternatives recorded:** 18 across the 10 full decisions — see [artifacts/implementation-decision-log.md](artifacts/implementation-decision-log.md)
- **Open items remaining:** 3 — two pre-commit verification gates (OI-1, OI-2) and one version-bump magnitude question (OI-3); none block implementation start.
- **YAGNI deferrals:** 5 — `NotifierPort` interface, `notification.dismiss`/`list` methods, per-surface targeting, hook-list refactor, per-method capability enumeration.
- **Recommendation:** ship as planned. The two pre-commit gates resolve at the manual cmux integration gate following parent D-R4 precedent; the version-bump question is resolvable at PR review without blocking implementation.
