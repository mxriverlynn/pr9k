# Implementation Iteration History: Phase 5 — Lifecycle Notifications

Round-by-round record of the team's discussion on the way to a committed implementation plan. Each `R#` entry captures the specialists engaged, what they raised, how it was resolved, and which decisions and plan sections changed as a result. The decision log ([implementation-decision-log.md](implementation-decision-log.md)) cross-references back via `Driven by rounds:` fields; the plan ([../feature-implementation-plan.md](../feature-implementation-plan.md)) cross-references via `Changed in plan:`.

## Team composition

Medium classification (two subsystems: `internal/cmuxctl` adds an RPC method; orchestrator gains a re-fire timer and three new firing sites). Team cap 5, round cap 2.

- `project-manager` — coordinator and final synthesizer. Called once in Step 8 (synthesis); not per round.
- `junior-developer` — generalist stress-tester and reframer.
- `concurrency-analyst` — re-fire timer goroutine, D12 ordering, in-flight RPC race, timer stop/restart, cleanup on panic.
- `behavioral-analyst` — completion vs. abort branching, timeout classification (D7 / D10 / D16), state transitions, abort-path call definition.
- `software-architect` — module boundaries (`cmuxNotifier` vs. extending `cmuxSidebar`), `CmuxClient` extension, capability-check shape, firing-site composition, Phase 6 forward compatibility.

Not engaged at R1, with rationale recorded for traceability:

- `user-experience-designer` and `edge-case-explorer`: reviewed at spec stage; findings already baked in to D2, D3, D4, D5, D12, etc.
- `adversarial-security-analyst`: no auth, PII, secrets, or untrusted-input surface (notification text is repo basename + step name only).
- `devops-engineer`: no new operational machinery beyond the existing per-run log file; no new SLO / observability / rollout surface.
- `system-architect`: pr9k↔cmux contract is settled at parent [D-R2](../../artifacts/decision-log.md#d-r2-cmux-v2-protocol-is-the-contract-of-record); no new bounded-context split, no cross-service integration change.
- `test-engineer`: test planning is a downstream skill (`han:test-planning`) run after the implementation plan is committed.
- `data-engineer` / `risk-analyst`: not applicable to Phase 5's scope.

---

## R1 — Round 1: parallel specialist review

**Specialists engaged:** `junior-developer`, `concurrency-analyst`, `behavioral-analyst`, `software-architect`. Launched in parallel with domain-scoped briefs and the [`.discovery-notes.md`](.discovery-notes.md) as required reading.

**New input provided:** the [feature-specification](../feature-specification.md), spec [`decision-log.md`](decision-log.md), spec [`team-findings.md`](team-findings.md), and `.discovery-notes.md` (which enumerated six implementation gaps the team would settle). No code was changed; this round is analysis-only.

### Claim ledger

Findings grouped by category. Each row names the supporting specialist(s), the state (Evidenced / Anecdotal / Disputed), and the spec-maturity tag (plan-level / spec-level / T#-contradiction).

#### Structural / module-boundary

| Claim | Raised by | State | Spec-maturity |
|-------|-----------|-------|---------------|
| `cmuxNotifier` is a new struct in `src/cmd/pr9k/cmux_notifier.go`, parallel to `cmuxSidebar` — SRP, different state machines, different failure policy at the abort-path boundary | `software-architect` (A1), `junior-developer` (JD-006) | Evidenced | plan-level |
| One new `CmuxClient` method (`WorkspaceNotify`) — not three per notification class; class semantics live in `cmuxNotifier` | `software-architect` (A2) | Evidenced | plan-level |
| Phase 6 forward compatibility = `cmuxNotifier.FireRunAborted(ctx)` as a concrete method (no new interface, no `NotifierPort`) — same pattern Phase 4 used for `sidebar.ClearAll` | `software-architect` (A7) | Evidenced | plan-level |
| The composition pattern (new struct, constructed in `runCmuxOrchestratorWith`, injected into `runCmuxWorkflowAdapted`) follows Phase 4's precedent | `software-architect` (A1, A6), `junior-developer` (JD-006) | Evidenced | plan-level |
| `cmuxSidebar` needs a package-private `LastStepName() string` (or `lastStepNameLocked()`) accessor so the notifier can snapshot the step name at error-mode entry per D13 | `software-architect` (A6), `behavioral-analyst` (B9) | Evidenced | plan-level |

#### Cmux v2 wire contract

| Claim | Raised by | State | Spec-maturity |
|-------|-----------|-------|---------------|
| The exact `notification.*` wire method name and param shape MUST be verified against the cmux source at commit `2f96c15c2` before the wire call is committed (parent D-R2 precedent) | `junior-developer` (JD-001), `software-architect` (A2 spec-contradiction note) | Evidenced | plan-level — but blocks decision; pre-commit gate |
| `notification.create_for_target` with the workspace UUID/ref as target is the authoritative match for spec D9, with `notification.create_for_caller` as the fallback if `create_for_target` does not accept a workspace target | `software-architect` (A2) | Evidenced | plan-level |
| `dismiss` and `list` are not needed by Phase 5 — YAGNI defer with reopen trigger "Phase 5 or later adds a dismiss/history call site" | `software-architect` (A2, A7 YAGNI list) | Evidenced | plan-level |

#### Typed timeout error

| Claim | Raised by | State | Spec-maturity |
|-------|-----------|-------|---------------|
| `cmuxctl` must introduce `*TimeoutError` (typed sentinel) to replace the current `fmt.Errorf("cmuxctl: %s timed out after %s", ...)` — required by D7 / D10 / D16 classification, and **also fixes a pre-existing latent defect in `cmuxSidebar`'s `errors.Is(err, context.DeadlineExceeded)` check** (which does not catch queue-level timeouts) | `software-architect` (A4), `concurrency-analyst` (C4), `behavioral-analyst` (B3), `junior-developer` (JD-003) | Evidenced (four specialists converged) | plan-level |
| The error classifier must distinguish four cases: nil, `*TimeoutError`, `context.Canceled` / `context.DeadlineExceeded`, other; `context.Canceled` from in-flight calls during shutdown is treated as non-fatal (caller context is already cancelled) | `concurrency-analyst` (C5) | Evidenced | plan-level |

#### Concurrency & timer lifecycle

| Claim | Raised by | State | Spec-maturity |
|-------|-----------|-------|---------------|
| The re-fire timer lives on `cmuxNotifier`, started via context-derived `context.WithCancel(ctx)`, stopped via `cancelRefire()` — not as an inline closure local | `software-architect` (A5), `concurrency-analyst` (C8) | Evidenced | plan-level |
| `cmuxNotifier` carries its own `sync.Mutex` covering `errorActive bool`, `snapshotName string`, `cancelRefire context.CancelFunc` — required because the key-adapter goroutine and orchestrator goroutine both touch these fields | `concurrency-analyst` (C1, C3) | Evidenced | plan-level |
| The initial notification call in `EnterErrorMode` should fire **synchronously inside the OnModeChange callback body** (not spawned in a goroutine before the state bit is set) — D12 ordering is enforced by sequential statements in the same goroutine | `concurrency-analyst` (C2) | Evidenced | plan-level |
| `defer notifier.ExitErrorMode()` (or `Stop()`) at the construction site provides the panic/abort safety net, mirroring `defer sidebar.ClearAll(...)` at `cmux_pane.go:200` | `concurrency-analyst` (C8), `software-architect` (A5) | Evidenced | plan-level |
| Apply the `ctx.Err()` guard after `ticker.C` fires (per `concurrency.md`) to avoid one spurious RPC call on race with cancellation | `concurrency-analyst` (C7) | Evidenced | plan-level |
| `FakeClient`'s new notification method follows the existing mu-guarded recorder + `Func`-override pattern verbatim | `concurrency-analyst` (C11) | Evidenced | plan-level |
| `RealClient.Stop()` already drains the I/O goroutine via `<-ioDone` before returning — no additional work for Phase 5 | `concurrency-analyst` (C9) | Evidenced (negative result) | plan-level |
| The 60-second ticker cadence cannot be tightened by fast error returns — `time.NewTicker` constructed once at error-mode entry, never recreated on each tick; D17's degraded-but-safe acceptance is mechanically sound | `concurrency-analyst` (C10) | Evidenced (negative result) | plan-level |

#### Behavioral wiring (completion vs. abort, timeout fork)

| Claim | Raised by | State | Spec-maturity |
|-------|-----------|-------|---------------|
| `ExitReason` mapping: `Completed → FireCompletion`, `LoopBroken → FireCompletion`, `UserQuit → FireRunAborted` — make this table explicit at the `runCmuxWorkflowAdapted` return site | `behavioral-analyst` (B1) | Evidenced | plan-level |
| The terminal (completion / run-aborted) notification fires **inside `runCmuxWorkflowAdapted` between `workflow.Run` returning and `sidebar.ClearAll(ctx)`** — before `WorkspaceDone` broadcast, satisfying spec D11 / EC5 | `behavioral-analyst` (B2), `software-architect` (A6) | Evidenced | plan-level |
| D14 ordering in the OnModeChange closure: footer broadcast first → sidebar mutation (existing) → notifier mutation (new); both sidebar and notifier are "step 3" relative to D12 but with sidebar called first to preserve Phase 4's behavior | `behavioral-analyst` (B4), `software-architect` (A6) | Evidenced | plan-level |
| D10's "abort-path call" reads narrowly — only `FireRunAborted` itself; all other cmux calls (including sidebar `ClearAll`) keep their normal D7/D17 routing | `behavioral-analyst` (B7), `software-architect` (A7 commentary) | Evidenced | plan-level |
| D16's "in-flight call after answer" requires a `resolved chan struct{}` (closed on first-keystroke resolution) the goroutine checks via `select { case <-resolved: ... default: ... }` after the call returns | `behavioral-analyst` (B8) | Evidenced | plan-level |
| The re-fire timer must be **stopped before the abort path issues `FireRunAborted`** — not after. The discovery-notes-flagged stopRefire `sync.Once`/channel pattern; abort calls `notifier.ExitErrorMode()` before `FireRunAborted` | `behavioral-analyst` (B6) | Evidenced | plan-level |
| `runCmuxOrchestratorWith` continues to return `nil` even on a fatal cmux timeout — the timeout drives the abort path inside `runCmuxWorkflowAdapted`, surfaces via the exit code in `WorkspaceDone{ExitCode:1}`, and `RunPhase1`'s normal path keeps the workspace open for operator dismissal. No Phase 5 change to the function signature | `behavioral-analyst` (B10) | Evidenced (negative result) | plan-level |

#### Capability check (spec D8)

| Claim | Raised by | State | Spec-maturity |
|-------|-----------|-------|---------------|
| `Preflight` is extended with one probe call to `WorkspaceNotify(Workspace{}, "")` and inspects the error: `*CmuxError` with `Code == "method_not_found"` (or `"unknown_method"` — exact code must be verified against cmux `2f96c15c2`) → fail with a method-named error per spec D8; any other error → method exists, probe passes | `software-architect` (A3), `behavioral-analyst` (B5) | Evidenced (two specialists converged on the same mechanic) | plan-level |
| Probe only the **one new method** (`WorkspaceNotify`) — do not probe sidebar methods or other existing methods. Per-method enumeration of all 13 methods is YAGNI; Phase 4's deferral stands | `software-architect` (A3) | Evidenced | plan-level |
| The "method-named" requirement in D8 is satisfiable; the alternative "version-pin only" reading does not satisfy D8 | `junior-developer` (JD-002), `software-architect` (A3) | Evidenced | plan-level |

#### Documentation

| Claim | Raised by | State | Spec-maturity |
|-------|-----------|-------|---------------|
| The spec's "Out of Scope" parking lot for the how-to documentation update is a violation of `docs/coding-standards/documentation.md` ("feature docs ship with the feature, not as follow-ups"). The plan MUST include an explicit documentation work-unit covering: persistent re-fire behavior in `docs/how-to/setting-up-cmux.md`, the new notification RPC entry in `docs/code-packages/cmuxctl.md`, the feature doc section in `docs/features/cmux-mode.md`, and any version-pin bump | `junior-developer` (JD-007) | Evidenced | plan-level |

#### Definition of Done

| Claim | Raised by | State | Spec-maturity |
|-------|-----------|-------|---------------|
| The plan should include an explicit DoD checklist (Phase 4 precedent: ~11 items) — particularly for: (a) timer-stop on the right resolution signal (see Disputed item below), (b) cancelled-quit handling (see Disputed item below), (c) abort-path notification timeout non-fatal | `junior-developer` (JD-008) | Evidenced | plan-level |

#### Disputed claims

| Claim | Raised by | State | Spec-maturity |
|-------|-----------|-------|---------------|
| **In cmux mode, the spec D5 "stops on first quit keystroke `q`" semantic is not implementable as written.** The footer pane (`cmux_footer_machine.go`) handles `q`/`y` quit-confirmation locally and only sends `IntentQuit` AFTER the operator confirms with `y`. The orchestrator never sees the `q` keystroke and never sees the `ModeError → ModeQuitConfirm → ModeError` cancelled-quit transition. The simpler-and-correct mechanic in cmux mode is: timer stops when `IntentQuit` arrives at the key-adapter loop (i.e., after `y` confirms), and the cancelled-quit timer restart is **impossible by construction** — the cancellation never reaches the orchestrator | `concurrency-analyst` (C6 — T1-contradiction); contradicted by `behavioral-analyst` (B11 — assumed ModeQuitConfirm/ModeError transitions are visible to OnModeChange) | **Disputed → settled by evidence below** | **T#-contradiction (spec contradiction)** |

**Evidence for resolving the disputed claim (read during deterministic aggregation, not yet in the iteration entry pre-resolution):** `src/cmd/pr9k/cmux_footer_machine.go` lines 94–113 show the footer state machine routes `q` from `ModeError` to `ModeQuitConfirm` locally, and only emits `RecordIntent(IntentQuit)` on `y` (line 107–109) or restores the previous mode on `n`/`esc` (line 110–111). The orchestrator's `keyAdapterLoop` (`cmux_workflow.go:215–253`) only receives `IntentQuit`, never `q` directly. **C6's claim is verified by the code; B11's assumption was wrong.**

#### YAGNI candidates (raised by R1)

- **Hook-list refactor of the `OnModeChange` closure** — `junior-developer` JD-009 flagged growing closure complexity; `software-architect` A6's YAGNI commentary deferred it. Evidence test fails: no measured friction; Phase 4 R3 trigger ("a fourth independent effect that does not compose cleanly") is not met by Phase 5's additions (which are two new branches in the same closure, not a fourth effect with different state).
- **`NotifierPort` interface** — Rule of Three not met (`*cmuxNotifier` is the only implementation; Phase 6 calls the concrete type).
- **`notification.dismiss` / `notification.list` methods on `CmuxClient`** — no Phase 5 call site.
- **Per-method capability enumeration of all 13 methods** — Phase 4 YAGNI deferral stands; Phase 5 probes only the one new method.
- **Version-pin bump (cmux 0.64.7 → newer)** — JD-010 noted this is conditional on whether notification methods exist in the current pin. If they do (verified at `2f96c15c2`, build outline line 269 confirms the dispatch shape), no bump is needed; only the one new probe is added.

### Open questions raised (OQ-N)

These are the questions deterministic aggregation cannot settle without evidence search, junior-developer reframing, or user input. Routed through Step 6.

- **OQ-1: D5 "first keystroke" in cmux mode → resolve via evidence** (already settled below as part of the disputed-claim resolution). The plan adopts the simpler-correct mechanic: timer stops on `IntentQuit` arrival (after `y` confirms). No cancelled-quit restart in cmux mode — the cancellation is invisible to the orchestrator. This is a **plan-stage YAGNI substitution** for an unimplementable spec mechanic. **Recommend escalating to the user for confirmation** because it narrows a spec D5 commitment.
- **OQ-2: Pin the exact `notification.*` wire method and param shape against cmux source at `2f96c15c2`.** Two candidates per software-architect A2: `notification.create_for_target` (preferred if the target field accepts a workspace UUID/ref) or `notification.create_for_caller` (fallback — orchestrator is a process inside the pr9k workspace). **Recommend resolving via evidence** by reading the cmux source. If reading is not available in the planning context, the plan ships with both candidates documented and a pre-commit verification gate.
- **OQ-3: Pin the exact `*CmuxError` code string cmux v2 returns for missing methods.** Candidates: `"method_not_found"`, `"unknown_method"`. Required for `Preflight`'s D8 method-name check. **Recommend resolving via evidence** by reading cmux source; otherwise check at runtime with logging and adjust during the manual cmux gate (parent D-R4 precedent).

### Spec-maturity tagging summary

- **Plan-level findings:** ~30 (most of the ledger).
- **Spec-level findings:** 0.
- **`T#`-contradictions:** **1** (the cmux-mode D5 "first keystroke" semantic, raised by `concurrency-analyst`; settled by evidence on the footer state machine code).

**Spec-maturity gate calculation:** Gate trips when (≥ 2 `T#`-contradictions raised by ≥ 2 distinct specialists) OR (≥ 5 `spec-level` findings raised by ≥ 3 distinct specialists). Phase 5 R1 has 1 contradiction by 1 specialist; **gate does NOT trip**. The disputed claim is settled by evidence (footer state machine code) at aggregation time; the OQ-1 escalation to the user is for confirmation of the YAGNI substitution, not for spec-stage rework.

### Resolution source per question

- **OQ-1:** Evidence (footer state machine code) settles the mechanic; user input requested to confirm the spec D5 reframing.
- **OQ-2:** Evidence (cmux source verification) — to be attempted at Step 6.
- **OQ-3:** Evidence (cmux source verification) — to be attempted at Step 6; otherwise documented as a pre-commit gate.

### Next-step recommendation (deterministic)

**Continue iterating (Step 6).** Reasons:

1. OQ-1 needs user confirmation that the D5 "first keystroke" reframing is acceptable (an evidence-grounded YAGNI substitution).
2. OQ-2 and OQ-3 need an evidence pass against the cmux source.
3. No specialist named a handoff to another specialist that is not already on the R1 team — `risk-analyst` and `structural-analyst` were named by junior-developer in his "specialist handoff" sections, but their concerns (closure complexity, structural separation) were already covered by `software-architect` A1 and A6 with concrete recommendations. No fresh handoff is needed.

### Decisions produced (backfilled in Step 8)

—

### Changed in plan (backfilled in Step 8)

—

---

## R2 — Round 2 (to be written if needed)

The round cap for the Medium classification is 2. R2 will document any additional round needed after Step 6's resolution loop produces new specialist work. If Step 6 closes all open questions by evidence and user input without re-engaging specialists, R2 will be skipped and we proceed directly to Step 7.5 (YAGNI sweep) and Step 8 (PM synthesis).
