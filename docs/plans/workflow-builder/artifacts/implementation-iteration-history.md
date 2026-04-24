# Implementation Iteration History: Workflow Builder

<!--
This file records how the implementation plan for the workflow builder evolved
across discussion rounds. Committed decisions live in
[implementation-decision-log.md](implementation-decision-log.md) and the primary
plan lives in [../feature-implementation-plan.md](../feature-implementation-plan.md).

Cross-referencing invariants:
- `Decisions produced:` — D# IDs from implementation-decision-log.md that this
  round added or changed.
- `Changed in plan:` — sections of ../feature-implementation-plan.md that this
  round updated.
-->

## R1: Parallel specialist review

- **Specialists engaged:** user-experience-designer, adversarial-security-analyst, devops-engineer, software-architect, concurrency-analyst, test-engineer, edge-case-explorer, junior-developer, project-manager (facilitation).
- **New input provided:** Initial feature specification (`feature-specification.md`), committed technical notes (T1 atomic save, T2 terminal handoff, T3 in-memory validation), project-discovery context, existing codebase patterns (`cmd/pr9k/sandbox.go`, `src/internal/ui/ui.go`, `src/internal/validator/validator.go`, `src/internal/logger/`), and the coding-standards set.
- **Questions raised (by theme):**
  - **T1 (atomic save):** package / API shape (who owns it?); write-failure injection seam; symlink-through-save test; EXDEV cross-device rename; temp-file naming convention and crash-era scan glob; companion-file atomicity and orphan-on-crash behavior. Arch-R4, DevOps DOR-001/002/004/007, Edge PH-1/AS-1/AS-4, Test T1-matrix.
  - **T2 (terminal handoff):** `tea.ExecProcess` ordering with SIGINT; RestoreTerminal failure distinction from editor non-zero exit; statusline `program.Send` during released window; input-queuing semantics; "Opening editor…" message timing. Concurrency C1/C3, DevOps DOR-003/009, Jr JrF-4.
  - **T3 (in-memory validation):** approach (a) vs (b) feasibility given the validator's disk-based helpers; scaffold-workflow case (prompt file doesn't exist yet); async-validation race with user input. Arch-R3, Concurrency C4, Jr JrF-1.
  - **Security:** `$VISUAL` word-splitting function; path-picker filesystem surface; pre-copy integrity path traversal; `safePromptPath` symlink escape + editor write; recovery-view ANSI injection; logger PII/secret filter contract. Sec-1 through Sec-9.
  - **UX architecture:** focus stack vs single `prevMode`; dialog composition with overlapping surfaces; shortcut-footer ownership; independent outline + detail-pane viewports; global-key intercept ordering. UX IP-001 through IP-007.
  - **Package decomposition:** where the builder's code lives; SOLID alignment; interface shapes. Arch-R1 through R8.
  - **Concurrency:** double-save race + D41 snapshot refresh; mtime comparison precision; goroutine lifecycle in a cobra-subcommand process. C1 through C8.
  - **Test plan:** package-by-package test inventory; T1/T2/T3 test matrices; 28 TUI mode coverage entries; doc integrity tests; production-config integration. Test plan, 7 sections.
  - **Edge cases:** 4 P0 (EXDEV, orphaned companions, `$VISUAL`-with-space, negative viewport); 22 P1; 4 P2. Edge PH/AS/EE/TUI/VI families.
  - **Spec-level gaps:** env/containerEnv editing; statusLine add/remove; step-removal affordance; placeholder prompt path; findings-panel scrollability; D69 stale D54 reference. Jr JrF-7/11/12/14/15, Edge VI-3, Jr JrF-10.
- **Resolution source (by question cluster):**
  - T1 package / API — evidence (code shape + DOR-001 recommendation) → plan-level commitment.
  - T2 mechanics — evidence (Bubble Tea source + DOR-003/009 recommendation) → plan-level commitment.
  - T3 approach — specialist handoff (architect Round 2).
  - Security path-traversal — evidence (existing `EvalSymlinks` precedent in `args.go`) → plan-level commitment; handoff 1 to security for verification.
  - UX architecture — specialist handoff (UX Round 2, given architect's R6 proposal).
  - Spec-level gaps (env/containerEnv/statusLine/step-removal/placeholder/findings-panel) — auto-mode default per project-manager's recommended answers (routine defaults grounded in D46/D48/D29/D52 precedent).
  - D69 stale D54 reference — evidence (D54 is marked superseded by D7 in R3) → noted as Open Item for spec author; plan cites D7 as authoritative.
- **Decisions produced:** D-1, D-2, D-4, D-5, D-11, D-12, D-13, D-14, D-15, D-16, D-17, D-18, D-19, D-20, D-22, D-23, D-24, D-25, D-26, D-27, D-28, D-29, D-30, D-31, D-32, D-33, D-34, D-35, D-36, D-38, D-39, D-40, D-41, D-42, D-43, D-44 (36 decisions seeded from R1 evidence and R1 resolutions; D-3, D-6, D-7, D-8, D-9, D-10, D-21, D-37 were pointed to Round 2 for specialist verification and emerged there in their final form).
- **Changed in plan:** Every section of `../feature-implementation-plan.md` was seeded in R1 — Source Specification, Outcome, Context, Team Composition, Implementation Approach (Architecture, Data Model, Runtime Behavior, External Interfaces), Decomposition and Sequencing (WU-1 through WU-12), RAID Log (all R#, A#, I#, Dep# entries), Testing Strategy, Security Posture (items 1–4, 6, 7), Operational Readiness, Definition of Done, Open Items (OI-1 scheduled into PR, OI-2 D69 stale-ref), Summary.
- **Project-manager next-step recommendation:** "Continue iterating" — three specialist handoffs (security, UX, architect) with plan commitments to verify; plan-level commitments also made for spec-level OQs per PM recommendations.

## R2: Specialist handoffs on architecture, security, and UX

- **Specialists engaged:** adversarial-security-analyst, user-experience-designer, software-architect.
- **New input provided:** R1 facilitation report; OQ resolutions accepted by coordinator (env/containerEnv editing; statusLine add/remove; step-removal affordance; placeholder prompt path; findings-panel scrollability; D69 stale-ref note); architect's R6 dialog-composition proposal for UX to validate; code-level validator surface for architect to inspect.
- **Questions raised:**
  - Security: do the plan commitments (EvalSymlinks+containment in all four new code paths; `stripANSI` reuse; `os.CreateTemp` with explicit 0o600; exclude secrets from logger) close the exploit paths from R1?
  - UX: does R6 `dialogState` + depth-1 `prevFocus` + `helpOpen bool` satisfy IP-001? What `Del`-key scoping prevents in-field destructive misfires? What is the exact `Update` ordering with overlay-active vs. global-shortcut?
  - Architect: given the actual validator code, is approach (a) feasible? What's the final signature? How many internal functions need `companionFiles` threaded? Is production-test preservation possible?
- **Resolution source:**
  - Security → evidence (codebase inspection). 2 findings still open (Sec-1 `$VISUAL` word-split function name; Sec-7 SIGINT exit-code branching), 1 new HIGH finding (NEW-1: OQ-4 placeholder prompt file creation path must EvalSymlinks+contain). All plan-level, the plan commits directly.
  - UX → evidence (trace of three focus-scenarios + Update ordering). R6 + depth-1 `prevFocus` sufficient; 3 commitments needed (Del-key scoping; unnamed-entry placeholder; refreshIntervalSeconds=0 hint); Update ordering clarified (overlay-check before global-key intercept).
  - Architect → evidence (validator code shape). Approach (a) confirmed feasible; exact signature given; 6 new packages (not 7 — `internal/ansi` rejected, reuse `statusline.Sanitize`); EditorRunner stays single-method with resolution internal.
- **Decisions produced:** D-3 (final `ValidateDoc` signature settled by architect code audit), D-6 (EditorRunner one-method confirmed), D-7 (ExecCallback SIGINT branch named), D-8 (dialogState + helpOpen + depth-1 prevFocus confirmed sufficient), D-9 (Update routing order corrected — overlay-first, not global-first), D-10 (Del scoping — outline-focus only), D-21 (create-on-editor-open EvalSymlinks + containment), D-37 (production integration test invariant confirmed). Also refined: D-1 (package count amended from 7 to 6 + atomicwrite; `internal/ansi` rejected), D-22 (`google/shlex` named), D-23 (stripANSI reuse via `statusline.Sanitize`, not new package; symlink-banner ordering committed), D-27 (logger contract finalized), D-28 (`(unnamed)` placeholder for env entries), D-29 (refresh-interval-0 hint for statusLine), D-38 (OI-1 in same PR confirmed), D-39 (per-open TOCTOU guard committed).
- **Changed in plan:** Implementation Approach — Architecture and Integration Points (package decomposition amended to 6 packages, `internal/ansi` explicitly rejected); Runtime Behavior (Update routing order corrected; `Del`-key scoping clarified; OQ-4 create-on-editor-open path specified); External Interfaces (`ValidateDoc` signature settled; `google/shlex` named); Security Posture (NEW-1 mitigation added as item 2; per-open TOCTOU note added to item 1; items 5–7 finalized); RAID Log (R3 severity recalibrated post-OI-1; R9 added for NEW-1); Definition of Done (OI-1 same-PR invariant confirmed).
- **Project-manager next-step recommendation:** "Go to synthesis." All Round 2 findings are direct plan-level commitments; no further specialist cycle needed.
