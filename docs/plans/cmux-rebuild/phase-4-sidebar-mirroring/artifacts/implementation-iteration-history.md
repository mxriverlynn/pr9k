# Implementation Iteration History: Phase 4 — Sidebar Mirroring

Round-by-round record of the team conversation that produced [feature-implementation-plan.md](../feature-implementation-plan.md). Companion to [implementation-decision-log.md](implementation-decision-log.md).

**Size classification:** Small — single-subsystem extension (cmux client + orchestrator adapter), no auth/PII, no migration, no cross-service. Team cap: 3 (project-manager + junior-developer + 1 specialist). Round cap: 1.

**Team composition:** `project-manager` (coordinator, synthesis), `junior-developer` (always-on generalist stress-tester), `software-architect` (intra-codebase interface design — the heart of the work).

**Specialists explicitly NOT engaged (and why):**

- `user-experience-designer` — already engaged at the spec stage (D13 was the UX contribution); no new operator-facing surface in implementation beyond what the spec already settled.
- `adversarial-security-analyst` — Phase 4 adds no auth, no PII, no untrusted-input handling, no new secrets path. The new RPC calls use the same socket transport, the same per-call timeout policy (parent D15), and the same trust model as Phases 1–3.
- `devops-engineer` — no new infrastructure component, no SLO surface, no rollout machinery beyond the version bump. Operational posture inherits Phases 1–3.
- `data-engineer` — no schema change, no migration, no persistent data. The on-disk log artifacts are explicitly unchanged (D11).
- `structural-analyst` / `behavioral-analyst` / `concurrency-analyst` — the structural / behavioral / concurrency questions are intra-codebase and limited in scope; `software-architect` covers the SOLID, snapshot-then-unlock, and OnModeChange-composition analysis without needing an upstream-finding swarm.
- `risk-analyst` — RAID items are individually scoped to specific findings; no portfolio-level prioritization needed for a small phase.
- `test-engineer` / `edge-case-explorer` — the test surface is "extend `FakeClient` with four `Func` fields, extend the existing adapter test pattern, verify ordering via recorder slices" — the plan can specify this directly without dispatching a specialist for a small phase.
- `system-architect` — Phase 4 is entirely intra-process; no bounded-context split, no cross-service integration, no new ownership change.
- `gap-analyzer`, `content-auditor`, `adversarial-validator`, `information-architect` — none applicable.

## R1 — Round 1: Parallel specialist review

**Specialists engaged:** `software-architect`, `junior-developer` (parallel, single message). Both received the same domain-scoped briefs and the path to `artifacts/.discovery-notes.md` with the directive to read it first and not re-grep what is already there.

**New input provided to specialists:**

- Full feature specification at `docs/plans/cmux-rebuild/phase-4-sidebar-mirroring/feature-specification.md` (read-on-demand).
- Spec artifacts: `decision-log.md`, `feature-technical-notes.md`, `team-findings.md` (read-on-demand).
- Parent artifacts: parent spec, parent decision log.
- Discovery notes: `artifacts/.discovery-notes.md` (read-first directive).
- Phase 2 and Phase 3 implementation plans (format precedent).

### Claim Ledger

| ID | Claim | Supporting Specialists | State | Spec-maturity |
|----|-------|------------------------|-------|---------------|
| C1 | `cmuxSidebar` lives as a separate struct in `src/cmd/pr9k/cmux_workflow.go` (or a new `cmux_sidebar.go` in the same package). Not on the `RunHeader` interface, not a decorator of `cmuxHeader` — composed alongside it in the orchestrator's wiring. Reason: SRP — `cmuxHeader`'s job is the interaction-channel header pane; the sidebar surface has different transport, different error semantics, different lifecycle events. | software-architect (S1) | Evidenced (cites `docs/coding-standards/concurrency.md`, existing `cmuxHeader` shape) | plan-level |
| C2 | The error-mode marker fires by augmenting the **existing** `SetOnModeChange` closure in `runCmuxWorkflowAdapted` — one closure that pushes `StateFooter` AND calls `sidebar.EnterErrorMode(ctx)` when `mode == ui.ModeError`. The sidebar caches `lastStepName` set by each step-status push; the closure reads that cache. No change to the `OnModeChange` callback signature. | software-architect (S2), junior-developer (F3 — clarified single-field constraint) | Evidenced (`src/internal/ui/ui.go:86` shows a single `onModeChange` field; composition into one closure is the only safe extension) | plan-level |
| C3 | The iteration→finalize transition is detected by a `sidebarAwareHeader` wrapper that implements `RunHeader` and delegates to `cmuxHeader`. The wrapper observes the first `RenderFinalizeLine` call and issues `sidebar.ClearProgress(ctx)` once; on every `RenderInitializeLine` / `RenderFinalizeLine` it issues `sidebar.PushStep`; on `SetStepState(idx, StepActive)` it issues `sidebar.PushStep(nameAt(idx))`; on `RenderIterationLine(iter, maxIter, _)` it issues `sidebar.PushProgress`. No new method on `RunHeader`. Reason: LSP — adding a `BeginFinalize()` to `RunHeader` would force every implementation (including `StatusHeader` in standard mode) to provide a do-nothing method. | software-architect (S3) | Evidenced (cites `src/internal/workflow/run.go` call sequence) | plan-level |
| C4 | Four new methods on the existing `CmuxClient` interface: `WorkspaceSetStatus(ctx, ws, key, value)`, `WorkspaceClearStatus(ctx, ws, key)`, `WorkspaceSetProgress(ctx, ws, fraction, label)`, `WorkspaceClearProgress(ctx, ws)`. No `opts` parameter for optional pill icon/color/priority (D12 defers them). No separate `CmuxSidebar` sub-interface — Rule of Three not met, single concrete caller today. | software-architect (S4) | Evidenced (cites ISP, existing `CmuxClient` pattern, D12) | plan-level |
| C5 | Capability check: bump the minimum cmux version pin from 0.64.6 to 0.64.7 in three locations (`src/internal/cmuxctl/client.go` package docstring, `src/internal/cmuxctl/preflight.go:65` error string, `docs/how-to/setting-up-cmux.md:5` Tested-against line). Leave preflight as identify-only. **Do not** add per-method `system.capabilities` enumeration — Phases 1–3 shipped without it and the literal reading of parent D18 / spec D15 fails the YAGNI simpler-version test against the version-pin alternative. Document explicitly that the capability check verifies cmux v2 is reachable; per-method probing remains deferred. | software-architect (S5), junior-developer (F4, F5) | Evidenced (cites YAGNI rule simpler-version test, three existing version-string locations, parent D18 text vs. current code) | plan-level |
| C6 | Thread `cmuxctl.CmuxClient` and `cmuxctl.Workspace` through the orchestrator composition root: `cmuxOrchestratorHooks` captures both (currently captures neither — the `Run` closure ignores `ws` with `_`); `runCmuxOrchestratorWith` gains two parameters; `runCmuxWorkflowAdapted` gains two parameters. Every call site (including tests) updates. This is the structural prerequisite that unblocks C1, C7, C8. | software-architect (S6), junior-developer (F1, F2, F11) | Evidenced (cites `src/cmd/pr9k/cmux_pane.go:46` `_` discard, `src/cmd/pr9k/cmux_workflow.go:124` current signature) | plan-level |
| C7 | Cleanup ordering on graceful run-end: `sidebar.ClearAll(ctx)` is called from `runCmuxWorkflowAdapted` immediately after `workflow.Run` returns and `keyHandler.SetMode(ui.ModeDone)`, before `keyCancel()` and `wg.Wait()`, and before the outer `runCmuxOrchestratorWith` broadcasts `WorkspaceDone`. The clears use the parent `ctx`, not `keyCtx`, so the post-cancel context is not used. | software-architect (S7) | Evidenced (cites `docs/coding-standards/concurrency.md` snapshot-then-unlock, existing `runCmuxWorkflowAdapted` teardown sequence) | plan-level |
| C8 | Hoist `cmuxSidebar` construction up to `runCmuxOrchestratorWith` (not inside `runCmuxWorkflowAdapted`) and register `defer sidebar.ClearAll(context.Background())` immediately after construction. This is a best-effort defense for paths that bypass `workflow.Run`'s normal return (e.g., a panic). Phase 6 will harden non-graceful paths further; the hoist is what allows Phase 6 to do that work without modifying `runCmuxWorkflowAdapted`'s internals. | software-architect (S8) | Evidenced (cites parent D17, OCP) | plan-level |
| C9 | `KeyHandler.SetOnModeChange` is a single-field assignment (`src/internal/ui/ui.go:131-135`); a second call overwrites the first. Phase 3's footer-state push hook lives there today. Phase 4 must **augment the existing closure** rather than register a second one — concretely, the closure becomes `func(mode, line) { ch.SendStateFooter(...); if mode == ui.ModeError { sidebar.EnterErrorMode(ctx) } }`. | junior-developer (F3) | Evidenced (cites the single-field assignment in `ui.go`) | plan-level |
| C10 | The sidebar status pill key string is pinned to literal `"pr9k.step"`. No documented cmux reserved prefix for `pr9k.`. Pinned in a typed const (`sidebarStatusKey`) in the same package as `cmuxSidebar` so the value is a single source of truth used by both push and clear paths. | junior-developer (F6) | Evidenced (cmux CLI contract doc on `main` lists no reserved prefixes; cmux's own examples use bare keys `claude_code`, `codex`, `build`, `deploy` — a dot-prefixed key is novel but not invalid). Caveat: if cmux 0.64.7 rejects dot-separated keys in practice, the implementation discovers it at first push and substitutes `pr9k_step` with a corresponding doc update. | plan-level |
| C11 | Version bump: 0.12.0 → 0.13.0 (MINOR), matching the established Phase 2 (0.10.0→0.11.0) and Phase 3 (0.11.0→0.12.0) phase-cadence precedent. The written standard `docs/coding-standards/versioning.md:37` says PATCH for backwards-compatible additions during 0.y.z; Phase 4 follows the de-facto precedent of phase-per-MINOR rather than the strict reading. **Surfaced as RAID assumption A1 in the plan**: if the user prefers strict-standard PATCH (0.12.1), the version constant is one line to change. | junior-developer (F7), discovery notes | Anecdotal vs. Evidenced (precedent vs. written standard) | plan-level — surfaced as RAID assumption for user verification |
| C12 | Error-mode marker uses U+2014 em dash: `" — awaiting input"`. Precedent: `src/internal/ui/header.go:63` `IterationIssueSep = " — Issue #"` and `src/internal/ui/log.go:57` `" — continuing (onTimeout=continue)"`. UTF-8 literal in the Go source. | junior-developer (F8) | Evidenced (existing precedent in `internal/ui`) | plan-level (closed) |
| C13 | Documentation updates: extend `docs/features/cmux-mode.md` (new section on sidebar mirroring), `docs/how-to/setting-up-cmux.md` (subsection covering what an operator monitoring from another workspace sees), `docs/code-packages/cmuxctl.md` (four new RPC methods documented). No new how-to file. Matches Phase 2 / Phase 3 precedent. | junior-developer (F9) | Evidenced (`docs/coding-standards/documentation.md` requires feature docs ship with the feature; precedent in Phase 2/3 plans) | plan-level |
| C14 | `FakeClient` extension: four new `Func` fields (`WorkspaceSetStatusFunc`, etc.) plus four recorder slices (`SetStatusCalls`, `ClearStatusCalls`, `SetProgressCalls`, `ClearProgressCalls`) so tests can verify call ordering (push-then-clear, error-mode marker timing). The compile-time interface assertion `var _ CmuxClient = (*FakeClient)(nil)` enforces this as a gate. | software-architect (S4), junior-developer (F10) | Evidenced (existing `FakeClient` pattern uses Func fields + recorder slices) | plan-level |
| C15 | YAGNI clarification on D7's "terminal iteration progress value": the iteration counter's natural push cadence already leaves `M / M` (or the last-completed iteration's value) in cmux's state when the loop ends. The "terminal push then clear" at the finalization transition is realized by **doing nothing extra at the push level and issuing `ClearProgress` once at the first `RenderFinalizeLine` event**. No special two-call sequence is required. The simpler-version test favors this reading. | junior-developer (F12) | Evidenced (re-reading spec primary flow + alternate flow; the orchestrator pushes iteration progress on every counter advance, and the last advance during iteration K pushes K/M) | plan-level |

### Open Questions (resolved this round)

- **OQ-1 (cmux version pin):** Did cmux 0.64.6 also have the sidebar surface, or is 0.64.7 the real minimum? **Resolved by evidence.** The Phase 4 spec D1 evidence section explicitly cites cmux 0.64.7 as the pinned version (user confirmation on 2026-05-19). The cmux CLI contract doc on `main` (fetched during round 1) documents the four sidebar verbs but does not specify a first-available version. Conclusion: trust the spec author's pinned version. Plan pins to 0.64.7. (Source: software-architect S5; junior-developer F4.)
- **OQ-2 (key prefix reserved?):** Does cmux reserve any namespace prefix that would conflict with `pr9k.`? **Resolved by evidence.** No reserved prefixes documented in the cmux CLI contract; cmux's own examples (`claude_code`, `codex`, `build`, `deploy`) are bare. The plan pins `pr9k.step` and the implementation discovers any cmux-side rejection at first push. (Source: junior-developer F6, web-fetched cmux CLI contract.)
- **OQ-3 (versioning PATCH vs. MINOR):** **Resolved by precedent.** Phase 2 and Phase 3 both bumped MINOR. Plan continues that pattern (0.13.0) and surfaces the standards-vs-precedent question as RAID assumption A1 for the user's review.
- **OQ-4 (OnModeChange composition):** Resolved by reframing — software-architect's S2 pseudocode already shows the single closure that does both pushes, addressing junior-developer's F3 concern.
- **OQ-5 (doc location):** Resolved by standards + precedent — three existing docs extended; no new how-to.

### Open Questions (carried to user)

None blocking. The version-bump question (C11 / OQ-3) is surfaced as a RAID assumption (A1) so the user can overrule MINOR → PATCH if they prefer strict-standard alignment.

### YAGNI candidates surfaced in Round 1 (carried to YAGNI sweep)

- **`CmuxSidebar` sub-interface** — software-architect (S4) ruled it out under ISP / Rule-of-Three / single-implementation-interface anti-pattern. **Disposition:** rejected as an abstraction; the four methods live on `CmuxClient` directly. Recorded as YAGNI in the plan.
- **Per-method `system.capabilities` enumeration in preflight** — software-architect (S5) ruled it out under simpler-version test (version pin satisfies D15 at a fraction of the scope). **Disposition:** deferred to `## Deferred (YAGNI)` in the plan; reopen trigger: "a cmux minor release removes or renames a method pr9k calls and a runtime failure surfaces that the version pin did not prevent."
- **`set-status` icon / color / priority parameters** — D12 already defers these at the spec level. **Disposition:** plan documents the parameter omission.
- **A separate "terminal progress push" sequence (C15)** — junior-developer F12 flagged this; resolved by re-reading the spec. **Disposition:** simpler-version recorded — the natural per-iteration push cadence already leaves the terminal value visible; one `ClearProgress` call at finalize-transition is sufficient.
- **Two-step push-then-clear orchestration** — not actually committed by the spec; collapsed into a single `ClearProgress` call by C15.

### Spec-maturity gate

**Not tripped.** Threshold: ≥ 2 `T#`-contradictions by ≥ 2 distinct specialists, or ≥ 5 `spec-level` findings by ≥ 3 distinct specialists. Round 1 produced **zero `T#`-contradictions** (T1 — cmux sidebar surface shape — is honored by every recommendation) and **zero `spec-level` findings** (every finding was `plan-level`: resolvable by evidence, reframing, or implementation choice within the plan's authority). The PM facilitation pass is therefore **NOT** triggered.

### Next-step recommendation (deterministic)

**Go to synthesis.** Round cap reached (small phase, 1 round); the deterministic stop rule's second sub-clause (≤ 2 new findings AND zero major findings) is also satisfied by the round's output, but the binding rule is the round cap. All open questions are either resolved by evidence or carried as a single RAID assumption (A1) for user review.

### Decisions produced

- **Full ([D-1](implementation-decision-log.md#d-1-sidebar-adapter-is-a-separate-cmuxsidebar-struct-plus-a-sidebarawareheader-wrapper-composed-alongside-cmuxheader)):** sidebar adapter is `cmuxSidebar` + `sidebarAwareHeader` composed alongside `cmuxHeader`.
- **Full ([D-2](implementation-decision-log.md#d-2-error-mode-marker-fires-by-augmenting-the-existing-onmodechange-closure-in-place-reading-from-a-cached-step-name)):** error-mode marker fires by augmenting the existing `OnModeChange` closure in place, reading from a cached step name.
- **Full ([D-3](implementation-decision-log.md#d-3-cmuxclient-gains-four-new-methods-directly-no-cmuxsidebar-sub-interface)):** four new methods directly on `CmuxClient`; no sub-interface.
- **Full ([D-4](implementation-decision-log.md#d-4-capability-check-stays-identify-only-the-cmux-version-pin-moves-0646--0647)):** capability check stays identify-only; cmux version pin bumps 0.64.6 → 0.64.7.
- **Full ([D-5](implementation-decision-log.md#d-5-cmuxctlcmuxclient-and-cmuxctlworkspace-are-threaded-through-the-orchestrator-composition-root-from-cmuxorchestratorhooks--runcmuxorchestratorwith--runcmuxworkflowadapted)):** thread `CmuxClient` + `Workspace` through `cmuxOrchestratorHooks → runCmuxOrchestratorWith → runCmuxWorkflowAdapted`.
- **Full ([D-6](implementation-decision-log.md#d-6-cleanup-ordering--sidebarclearallctx-runs-in-runcmuxworkflowadapted-immediately-after-workflowrun-returns-using-the-parent-context)):** cleanup ordering — `sidebar.ClearAll(ctx)` runs in `runCmuxWorkflowAdapted` immediately after `workflow.Run` returns, using parent context.
- **Full ([D-7](implementation-decision-log.md#d-7-cmuxsidebar-construction-is-hoisted-to-runcmuxorchestratorwith-a-defer-sidebarclearallcontextbackground-runs-on-every-exit-path)):** `cmuxSidebar` construction hoisted to `runCmuxOrchestratorWith`; `defer sidebar.ClearAll(context.Background())` runs on every exit path.
- **Trivial [D-8](implementation-decision-log.md#trivial-decisions):** `pr9k.step` status key as a typed const.
- **Trivial [D-9](implementation-decision-log.md#trivial-decisions):** error-mode marker literal `" — awaiting input"` (U+2014).
- **Trivial [D-10](implementation-decision-log.md#trivial-decisions):** version bump 0.12.0 → 0.13.0 (MINOR by precedent); surfaced as RAID A1.
- **Trivial [D-11](implementation-decision-log.md#trivial-decisions):** doc updates extend `cmux-mode.md`, `setting-up-cmux.md`, `code-packages/cmuxctl.md`; no new how-to.
- **Trivial [D-12](implementation-decision-log.md#trivial-decisions):** `FakeClient` extension with 4 Func fields + 4 recorder slices.
- **Trivial [D-13](implementation-decision-log.md#trivial-decisions):** D7 simpler-version — single `ClearProgress` at finalize transition; no separate terminal push.
- **Trivial [D-14](implementation-decision-log.md#trivial-decisions):** `nameAt(idx)` accessor on `cmuxHeader`.

### Changed in plan

- Outcome — sidebar adapter shape, version pin bump, version bump.
- Context — Phase 6 forward-compatibility statement; out-of-scope boundary.
- Team Composition and Participation — `software-architect`, `junior-developer` engaged; others explicitly not.
- Implementation Approach (Architecture and Integration Points) — new file, three existing modifications, four `CmuxClient` additions, preflight unchanged.
- Implementation Approach (Runtime Behavior) — per-event push cadence, completion sequence, error paths.
- Implementation Approach (External Interfaces) — four new JSON-RPC methods, operator-visible sidebar surfaces.
- Decomposition and Sequencing — W1 version pin, W2 client extension, W3 sidebar adapter + threading + closure augmentation + cleanup ordering + hoisted construction, W4 test cascade, W5 docs, W6 version bump.
- RAID Log (Risks, Assumptions, Issues, Dependencies) — six risks, five assumptions, two resolved issues, four phase-boundary dependencies.
- Testing Strategy — `cmuxctl.RealClient` round-trip, `FakeClient` recorder tests, `cmuxSidebar` unit tests, `sidebarAwareHeader` wrapper tests, race detector, single round-trip integration test.
- Operational Readiness — release-notes scope, rollback path.
- Definition of Done — full checklist.
- Deferred (YAGNI) — five deferrals with reopen triggers.
- Open Items — OI-1 (versioning A1).
- Summary — outcome, decisions counted, deferrals counted, open items.
