---
title: "pr9k Cmux Mode — Build Phase Outline"
source_artifact: "feature-specification.md"
audience: "mixed engineering, product, leadership"
generated: "2026-05-15"
generated_by: "han:plan-a-phased-build"
---

# pr9k Cmux Mode — Build Phase Outline

This document describes the order in which **pr9k cmux mode** will be built. The work is broken into a sequence of **phases**, where each phase is a thin end-to-end deliverable that can be demonstrated to a real person. Most phases depend on prior phases; the specific dependency for each phase is stated in its **Builds on** field. Cmux mode is an opt-in launch surface for pr9k that renders the workflow run as a multi-pane [cmux](https://cmux.com/) workspace instead of a single-window terminal display — the same workflow runs against the same target repository, but the chrome is owned by cmux (panes, sidebar, notifications, native selection) rather than drawn by pr9k.

This document is the companion to [feature-specification.md](feature-specification.md). The source artifact describes *the behavior pr9k cmux mode commits to delivering*. This document describes *the order in which the work will be built to ship that behavior*. Every phase below cites the source-artifact sections it covers, so anyone can trace a phase back to source.

## Table of Contents

- [Executive Summary](#executive-summary)
- [Build Phase Index](#build-phase-index)
- [Phase Kinds](#phase-kinds)
- [Build Phases](#build-phases)
  - [Phase 1: Cmux Mode Launch and Workspace Lifecycle](#phase-1)
  - [Phase 2: First Real Workflow Runs End-to-End in Cmux](#phase-2)
  - [Phase 3: Interactive Error Recovery](#phase-3)
  - [Phase 4: Sidebar Mirroring](#phase-4)
  - [Phase 5: Lifecycle Notifications](#phase-5)
  - [Phase 6: Robust Failure Handling](#phase-6)
  - [Phase 7: Orphan Workspace Startup Advisory](#phase-7)
- [Open Questions](#open-questions)

---

## Executive Summary {#executive-summary}

**The goal:** Ship `pr9k --cmux` as a real, opt-in launch mode that drives cmux to render the workflow run as a four-pane workspace (header, log, footer, and a hidden orchestrator pane) with sidebar status and notifications. The same workflows run, against the same target repositories, producing the same on-disk log artifacts — only the presentation surface changes.

**The shape of the build (5 bullets, plain language):**

- **Phase 1 lays the foundation:** the operator can launch cmux mode, watch a fresh, recognizably-named workspace appear with the four-pane scaffold in place, and dismiss it cleanly with their prior workspace restored — before any workflow content runs inside.
- **Phase 2 ships the first real run:** the same workflow that runs in the standard terminal display now runs inside the cmux workspace, with the header reflecting step progress, the log streaming live output, and the footer surfacing the status line, shortcut hints, and quit shortcut. The same on-disk log artifacts are produced.
- **Phase 3 makes the cmux run interactively recoverable** by surfacing pr9k's existing continue / retry / quit error-mode prompt inside the footer pane.
- **Phases 4 and 5 connect the workspace to cmux's broader chrome:** the sidebar shows live step and iteration progress from any workspace, and cmux notifications fire at completion, failure, and when an error-mode prompt is awaiting the operator's decision.
- **Phases 6 and 7 harden the experience:** display loss, orchestrator loss, hung cmux calls, and operator pane-close gestures all abort the run cleanly with a notification; a startup advisory tells the operator when orphan workspaces from prior crashed runs are still around.

**Sequencing rationale, in plain language:**

The first phase is a foundation because nothing else demoable can run until pr9k can stand up and dismantle a cmux workspace cleanly. From there, every subsequent phase is a feature slice — earlier ones cover the happy-path run, later ones add the auxiliary chrome (sidebar, notifications) and the resilience the operator needs when something goes wrong. The two polish-tier items — full failure-mode robustness and the orphan advisory — land at the end because they enrich a working core rather than make it work.

**Phases deliberately deferred:**

Four items the source artifact already names as deferred remain deferred here: routing pr9k's full status-line script into the cmux sidebar, automatic cleanup of orphan workspaces from prior crashes, distributing keyboard control across every pane, and a cmux pane for the resulting pull-request browser surface. Each carries a documented reopening trigger in the [feature specification's Deferred (YAGNI) section](feature-specification.md#deferred-yagni).

**Where to look next:** The [Build Phase Index](#build-phase-index) lists every phase in order. Detailed write-ups follow under [Build Phases](#build-phases). Decisions the team must resolve before Phase 1 can start are at [Open Questions](#open-questions).

---

## Build Phase Index {#build-phase-index}

> The scan view. One row per phase, in build order. Each "Outcome" cell is one short sentence (~15 words). Detailed write-ups follow under [Build Phases](#build-phases); use the link in the Phase column.

| # | Phase | Kind | Outcome (one sentence) |
|---|---|---|---|
| 1 | [Cmux Mode Launch and Workspace Lifecycle](#phase-1) | Foundation | The operator launches cmux mode and sees a fresh, recognizable workspace appear and tear down cleanly. |
| 2 | [First Real Workflow Runs End-to-End in Cmux](#phase-2) | Feature slice | A real workflow runs inside the cmux workspace with live header, log, and footer output. |
| 3 | [Interactive Error Recovery](#phase-3) | Feature slice | A failing step prompts the operator to continue, retry, or quit from inside the footer pane. |
| 4 | [Sidebar Mirroring](#phase-4) | Feature slice | The current step and iteration progress are visible in cmux's sidebar from any other workspace. |
| 5 | [Lifecycle Notifications](#phase-5) | Feature slice | The operator receives cmux notifications at completion, failure, and when an error-mode prompt is waiting. |
| 6 | [Robust Failure Handling](#phase-6) | Polish | Pane loss, hung cmux calls, and accidental pane closures all abort the run cleanly with diagnostics intact. |
| 7 | [Orphan Workspace Startup Advisory](#phase-7) | Polish | A startup line tells the operator how many crashed-run workspaces are still around. |

> Numbers are assigned in build order and are stable for the life of this outline. Cite them as `Phase N` in tickets, comments, and follow-up reports.

---

## Phase Kinds {#phase-kinds}

Every phase is tagged with one of four kinds. The taxonomy is used in the Build Phase Index and on each phase entry's `**Kind.**` line.

- **Foundation** — A capability that does not deliver new user-facing features on its own, but is required for later phases. Must still be demoable in its own right (e.g., "an operator can launch the new mode and see a recognizable workspace appear").
- **Feature slice** — A thin end-to-end strip of new behavior that a real user can experience.
- **Polish** — Refinement, resilience, or quality-of-life work that enriches a working core.
- **Deferred** — Listed for traceability; not built in the current plan. Slotted at the end of the index. (None in this build — deferred items live in the [feature specification's Deferred (YAGNI) section](feature-specification.md#deferred-yagni).)

---

## Build Phases {#build-phases}

### Phase 1: Cmux Mode Launch and Workspace Lifecycle {#phase-1}

**Kind.** Foundation.

**Builds on.** Nothing — this is the starting phase.

**What we build.** The operator can opt into cmux mode at launch and observe a recognizable, well-named cmux workspace appear, contain a four-pane scaffold, and tear down cleanly when they are done. No real workflow runs inside yet — the panes show placeholder content — but every piece of the workspace lifecycle is exercised end-to-end.

- A new opt-in launch surface is added so the operator can request cmux mode at run time.
- Before touching cmux, pr9k confirms the cmux runtime is reachable, that pr9k is permitted to drive it, and that the cmux build exposes every capability pr9k will use. If any check fails, pr9k aborts with a message tailored to which failure occurred — cmux is not installed, cmux is installed but not running, pr9k is not a descendant of a cmux session, the cmux socket is disabled by configuration, or the cmux build does not expose every capability pr9k needs.
- pr9k records which cmux workspace the operator was in before launch, so it can be brought back into focus afterwards.
- pr9k creates a fresh workspace whose name combines a fixed pr9k prefix, the target repository's basename, and a high-resolution timestamp — names are unique even across rapid concurrent launches and are scannable in cmux's workspace list.
- The new workspace is laid out with four panes: a small header pane on top, a tall log pane in the middle, a small footer pane on the bottom, and a fourth pane hosting the orchestrator process that is hidden by default. Each pane runs a placeholder process so the operator sees the shape clearly.
- The workspace stays open until the operator explicitly dismisses it. On dismissal, the operator's prior workspace is brought back into focus.

**Why this is Phase 1.** Every later phase puts content inside this workspace — step state, streaming logs, footer shortcuts, sidebar entries, notifications. None of that can be demoed until the workspace itself reliably appears, holds its shape, and tears down without stranding the operator. Phase 1 isolates the most platform-coupled work (cmux preflight, capability detection, workspace lifecycle, focus restore) so it can be validated on its own before any pr9k workflow content is added.

**Outcome to demonstrate.**

1. From inside a cmux session, the operator launches pr9k against a target repository in cmux mode.
2. A fresh cmux workspace appears named with the pr9k prefix, the target repo's basename, and a high-resolution timestamp. The workspace contains four panes; three are visible (header / log / footer) with placeholder labels, the fourth (orchestrator) is hidden.
3. The operator switches to another cmux workspace and back — the pr9k workspace persists with all panes intact.
4. The operator closes the workspace via cmux's own controls; cmux returns them to the workspace they were in before launching pr9k.
5. The operator launches pr9k cmux mode while cmux is stopped. pr9k aborts before creating any workspace and prints a message that names the specific failure (cmux is not running). Repeating the test with a cmux that is missing a required capability produces a different, specific message naming the missing capability.

**Source citations.**
- [Actors and Triggers](feature-specification.md#actors-and-triggers) — the `--cmux` launch flag and the preconditions the operator must satisfy.
- [Primary Flow](feature-specification.md#primary-flow) — steps 1–3, 5–7 (launch, preflight, capability check, workspace creation, pane layout, readiness handshake scaffolding).
- [Edge Cases and Failure Modes](feature-specification.md#edge-cases-and-failure-modes) — the four cmux-unreachable conditions and the cmux API capability-mismatch row.
- [Decision Log entries D2, D3, D4, D13, D18, D22, D25, D29](artifacts/decision-log.md) — opt-in launch, hard-precondition cmux availability, four-pane layout, hidden orchestrator pane, startup capability check, prior-workspace capture/restore, the `--cmux` launch flag, workspace name pattern.
- [Feature Technical Notes T1, T2, T3, T5](artifacts/feature-technical-notes.md) — cmux programmatic interface shape, access model, surfaces are processes, workspace persists after pane exit.

**Connects to.**
- [Phase 2](#phase-2) — the placeholder panes Phase 1 stands up become the real renderers Phase 2 brings to life.
- [Phase 6](#phase-6) — Phase 1's lifecycle teardown is what Phase 6 hardens against display loss, orchestrator loss, and hung cmux calls.
- [Phase 7](#phase-7) — Phase 1's workspace naming pattern is what Phase 7's orphan advisory recognizes.

**Preconditions to verify before starting.**
- Confirm which cmux versions the team commits to supporting at launch (the capability check is method-presence-based, not version-string-based; the supported versions still need to be named in the setup how-to). See [OQ-1](#oq-1).
- Confirm the team's plan for testing cmux mode in CI given that cmux is a graphical application — what level of integration testing is achievable without a live cmux process, and what's manual-only. See [OQ-2](#oq-2).

---

### Phase 2: First Real Workflow Runs End-to-End in Cmux {#phase-2}

**Kind.** Feature slice.

**Builds on.** [Phase 1](#phase-1) — the workspace, the four-pane scaffold, and the lifecycle teardown all exist; this phase puts real content inside the panes.

**What we build.** The same workflow that runs in the standard pr9k terminal display now runs inside the cmux workspace, with every visible pane reflecting live state. The placeholder content from Phase 1 is replaced by real renderers, the orchestrator (in the hidden pane) drives the workflow exactly as it would in the standard display, and the on-disk log artifacts continue to be produced unchanged.

- The hidden orchestrator pane runs the workflow engine — same phase semantics, same step definitions, same iteration counter, same exit codes as the standard display.
- The header pane shows step checkbox progress and the current iteration counter; both update live as the orchestrator advances.
- The log pane shows streaming output from the currently running step within the existing latency budget. Pane-local scrollback is available when the operator focuses this pane.
- The footer pane shows the configured status line (running the operator's status-line script unchanged), the shortcut hints, and supports the basic running-state shortcuts: quit (with the existing two-step `q` then `y` confirmation), help, and the next-iteration confirmation prompt the standard display uses today. The "skip current step" shortcut is also exercised.
- A readiness handshake gates the workflow's first step: the orchestrator waits until every visible pane has signaled it is rendering before pushing the first step's state, so the operator never misses the first events.
- The pr9k log artifacts written to the target repository's standard log directory are byte-for-byte the same content the standard display would produce in the same run.

**Why this is Phase 2.** Phase 1 proved the workspace can stand up; this phase is the smallest end-to-end demoable run that proves the multi-process architecture actually delivers the same pr9k experience an operator gets today. Without it, every subsequent phase (sidebar, notifications, error recovery, failure modes) is decorating an empty box. Once Phase 2 ships, the operator already has something genuinely useful — they can run a real workflow inside cmux and quit it cleanly — even if some of the chrome is still missing.

**Outcome to demonstrate.**

1. The operator launches pr9k in cmux mode against a target repository with a real workflow configured.
2. The cmux workspace appears (as in Phase 1) and the four panes start rendering. After the brief readiness handshake, the workflow begins.
3. The header pane shows checkboxes that tick over as each step completes, and the iteration counter advances.
4. The log pane streams the same output the operator would see in the standard terminal display today. The operator focuses the log pane and scrolls back through prior output.
5. The footer pane shows the status line on its existing cadence, the shortcut hints, and (if the workflow is configured to ask) the next-iteration confirmation prompt. The operator presses the help shortcut from the footer pane and sees the help modal.
6. The operator presses the quit shortcut from the footer pane and confirms; the orchestrator aborts cleanly, the workspace remains open showing the final state, and the operator dismisses it to return to their prior workspace.
7. After the run, the operator inspects the target repository's standard log directory and confirms the per-run log file and per-step artifacts match what the standard display would have produced.

**Source citations.**
- [Outcome](feature-specification.md#outcome) — same workflow, same artifacts, only the presentation surface changes.
- [Primary Flow](feature-specification.md#primary-flow) — steps 7–10, 12 (readiness handshake, workflow execution, state push to panes, log artifacts).
- [Alternate Flows and States](feature-specification.md#alternate-flows-and-states) — operator quits mid-run, skip current step, help is requested, operator focuses a non-control pane.
- [User Interactions](feature-specification.md#user-interactions) — header / log / footer affordances and feedback.
- [Coordinations](feature-specification.md#coordinations) — display panes inbound/outbound contract.
- [Decision Log entries D1, D4, D8, D13, D14, D16, D17, D20, D26](artifacts/decision-log.md) — alternate presentation only, four-pane layout, footer owns keyboard, hidden orchestrator pane, operator-initiated closure, readiness handshake, unchanged log artifacts, keyboard state machine in the footer, status-line script unchanged.
- [Feature Technical Notes T3, T4](artifacts/feature-technical-notes.md) — cmux surfaces are processes, display processes update independently.

**Connects to.**
- [Phase 3](#phase-3) — error-mode prompts will plug into the same footer-pane state machine Phase 2 establishes.
- [Phase 4](#phase-4) — the live step name and iteration counter the header pane displays will be mirrored to the cmux sidebar.
- [Phase 5](#phase-5) — completion-state detection from Phase 2's run loop is what fires the completion notification.
- [Phase 6](#phase-6) — Phase 2's display panes are exactly what Phase 6 hardens against accidental closure and process loss.

**Preconditions to verify before starting.**
- Settle the mechanism the orchestrator pane uses to push state to the three display panes and to receive operator intents from the footer pane (the spec calls this the "local interaction channel" and leaves the mechanism open). See [OQ-3](#oq-3).
- Confirm that the status-line script runs in the footer pane on its existing cadence, unchanged from how it runs in the standard display.
- Confirm the team's release plan accounts for the version-bump and matching feature-doc updates the new launch surface requires.

---

### Phase 3: Interactive Error Recovery {#phase-3}

**Kind.** Feature slice.

**Builds on.** [Phase 2](#phase-2) — the running workflow, the footer-pane keyboard state machine, and the orchestrator-to-footer intent channel all exist; this phase extends them to cover the error-mode prompt.

**What we build.** When a step fails in a way that triggers pr9k's existing continue / retry / quit error prompt, the operator sees the prompt inside the footer pane and can respond to it from there. The header pane reflects the failure, the log pane shows the error, and control keys pressed on non-control panes are absorbed silently rather than producing surprising behavior.

- The orchestrator's error-mode prompt — identical to the one the standard pr9k display produces today — is rendered inside the footer pane.
- The footer pane forwards the operator's continue / retry / quit choice to the orchestrator, which acts on it with the same semantics as the standard display.
- The header pane marks the failing step with the same failure indicator the standard display uses today.
- The log pane shows the error output as a normal part of the streaming log.
- If the operator presses one of the control keys (`q`, `c`, `r`, `?`, and so on) while the focused pane is the header or the log, the keystroke is absorbed without effect — no error, no notification.

**Why this is Phase 3.** Without an interactive recovery path, Phase 2's happy-path run cannot survive a single failing step — the operator's only option would be to quit and re-launch. Error recovery is the smallest follow-on slice that closes that gap, and it does so by reusing the keyboard state machine Phase 2 already put in the footer pane. It lands here rather than later because completion notifications (Phase 5) need the error-mode prompt to exist to demonstrate the persistent-notification path.

**Outcome to demonstrate.**

1. The operator launches pr9k in cmux mode against a target repository whose workflow has a step that is currently configured to fail (a deliberately broken step is the easiest demo).
2. The workflow runs as in Phase 2 until the failing step exits non-zero.
3. The footer pane switches into error-mode and shows the continue / retry / quit shortcut hints. The header pane shows the failing step's checkbox with a failure indicator. The log pane shows the step's error output.
4. The operator presses the retry shortcut. The orchestrator retries the step, the footer returns to its normal shortcut hints, and the run continues.
5. The operator repeats the run, deliberately presses a control key (`q`) while the log pane is focused — nothing happens. The operator focuses the footer pane and presses the same key — the standard quit confirmation appears.

**Source citations.**
- [Alternate Flows and States](feature-specification.md#alternate-flows-and-states) — "Step fails and prompts for operator decision" and "Operator focuses a non-control pane".
- [Edge Cases and Failure Modes](feature-specification.md#edge-cases-and-failure-modes) — the row covering control keys pressed on a non-control pane.
- [Decision Log entries D6, D8, D19, D20](artifacts/decision-log.md) — notification lifecycle hooks, footer owns keyboard input, error-mode notification directs to control pane, keyboard state machine details.

**Connects to.**
- [Phase 5](#phase-5) — the error-mode prompt established here is the trigger for Phase 5's persistent error-mode notification.

**Preconditions to verify before starting.**
- Confirm that all seven surviving keyboard modes — normal running, error prompt with continue/retry/quit, quit confirmation, next-iteration confirmation, done state, in-progress quitting, and help — can be hosted in the footer pane without behavioral regression. The one dropped mode is keyboard-driven log selection, which the spec replaces with cmux's native mouse selection.

---

### Phase 4: Sidebar Mirroring {#phase-4}

**Kind.** Feature slice.

**Builds on.** [Phase 2](#phase-2) — the live step state and iteration counter the header pane already tracks are the source of the sidebar entries this phase pushes.

**What we build.** While a workflow runs in pr9k's cmux workspace, cmux's persistent sidebar shows two live entries for that workspace: the current step name as a status line, and the iteration counter as a progress indicator in `N / M` form. Both entries update on the same cadence as the header pane, so an operator who is viewing a different cmux workspace can still see at a glance what pr9k is doing.

- The orchestrator pushes the current step name to the cmux sidebar as a status entry whenever the step changes.
- The orchestrator pushes the iteration counter as a sidebar progress entry whenever the counter advances.
- Both entries are scoped to the pr9k workspace — switching to another cmux workspace still shows them in the sidebar against the pr9k workspace's row.
- When the run completes (success, failure, or operator quit), both sidebar entries are cleared.

**Why this is Phase 4.** This is the smallest slice that delivers cmux's signature "monitor a workspace from elsewhere" benefit. It lands after Phase 3 because error recovery is more load-bearing than out-of-workspace visibility — an operator who cannot recover from a step failure inside the workspace will not benefit from sidebar visibility from outside it. The work itself is bounded: the data already exists in the orchestrator (step name, iteration counter), and the sidebar is a small, idiomatic part of cmux's API. The decision to include sidebar mirroring was made at the feature-specification level on platform-design grounds rather than on a named operator request; if scope needs to be cut, Phase 4 is the phase to defer, since it can be added later without changing any other phase.

**Outcome to demonstrate.**

1. The operator launches pr9k in cmux mode against a target repository with a multi-iteration workflow.
2. Once the run is underway, the operator switches to a different cmux workspace (their original workspace, or any other).
3. cmux's sidebar shows the pr9k workspace with two live entries: the current step name and an iteration progress indicator. Both update visibly as the workflow advances.
4. The run completes; the sidebar entries clear.

**Source citations.**
- [Outcome](feature-specification.md#outcome) — sidebar mirroring is a named outcome.
- [Primary Flow](feature-specification.md#primary-flow) — step 10.
- [User Interactions](feature-specification.md#user-interactions) — the cmux sidebar affordance.
- [Decision Log entries D5, D26](artifacts/decision-log.md) — mirror step and iteration only, status-line script stays in footer.

**Connects to.**
- [Phase 6](#phase-6) — when the run aborts, the failure-mode handling in Phase 6 is responsible for clearing the sidebar entries as part of cleanup.

**Preconditions to verify before starting.**
- Confirm that the team agrees the full status-line script should not also be mirrored into the sidebar — the spec defers this and it is worth one explicit acknowledgement before the work starts so the deferral is not silently revisited.

---

### Phase 5: Lifecycle Notifications {#phase-5}

**Kind.** Feature slice.

**Builds on.** [Phase 2](#phase-2) — the run-completion signal is the trigger for the completion notification. [Phase 3](#phase-3) — the error-mode prompt is the trigger for the error-mode notification.

**What we build.** cmux's notification surface fires at three named moments during a pr9k cmux run: when the full run completes successfully, when the run aborts (any reason that ends the run unsuccessfully), and when an error-mode prompt is waiting for the operator's decision. The error-mode notification is the only one that persists — it re-fires on a regular cadence until the operator answers the prompt, and its text explicitly directs the operator to focus the footer pane.

- A completion notification fires once when the full run finishes successfully.
- A "run aborted" notification fires once when the run ends unsuccessfully (including operator-initiated quit, when the operator dismisses the run via the footer pane's quit shortcut).
- An error-mode notification fires when a step has failed and is awaiting the operator's continue / retry / quit decision. This notification includes the text "Focus the pr9k control pane to respond" and re-fires on a regular cadence until the operator answers the prompt, so a dismissed notification does not silently strand the orchestrator.
- No per-step and no per-iteration notifications fire — those would be noise.

**Why this is Phase 5.** Notifications are the smallest piece of work that completes the "monitor from outside" story Phase 4 started — sidebar entries tell the operator passive state, notifications interrupt them when their attention is required. Phase 5 lands here because both of its trigger surfaces (completion-state detection in Phase 2 and error-mode prompt in Phase 3) are already built. The persistent re-firing behavior is what makes the multi-pane architecture safe in the failure case where the operator wandered away from the workspace.

**Outcome to demonstrate.**

1. The operator launches pr9k cmux mode against a target repository with a workflow that will complete successfully.
2. The workflow runs to completion; a completion notification appears in cmux. The operator clicks it and is taken to the pr9k workspace.
3. The operator launches a second run against a workflow with a step deliberately set to fail. The workflow reaches the failing step.
4. An error-mode notification appears in cmux with the text "Focus the pr9k control pane to respond." The operator dismisses it; some seconds later the notification re-appears.
5. The operator focuses the pr9k workspace's footer pane and presses the quit shortcut. The error-mode notification stops re-firing, the run aborts, and a "run aborted" notification appears once.

**Source citations.**
- [Primary Flow](feature-specification.md#primary-flow) — step 11.
- [Alternate Flows and States](feature-specification.md#alternate-flows-and-states) — "Step fails and prompts for operator decision".
- [User Interactions](feature-specification.md#user-interactions) — cmux notifications affordance.
- [Decision Log entries D6, D19](artifacts/decision-log.md) — notification lifecycle moments, error-mode notification directs to control pane.

**Connects to.**
- [Phase 6](#phase-6) — the "run aborted" notification path is what Phase 6 reuses for every failure-mode abort (display loss, orchestrator loss, hung cmux call, pane close).

**Preconditions to verify before starting.**
- Confirm the cadence at which the error-mode notification re-fires — the spec calls this an implementation-internal value but the team should agree on a number before the work starts so it is not litigated during review.

---

### Phase 6: Robust Failure Handling {#phase-6}

**Kind.** Polish.

**Builds on.** [Phase 2](#phase-2) — the running workflow and the channel between orchestrator and display panes are what this phase hardens. [Phase 4](#phase-4) — the sidebar entries this phase clears during failure handling are the entries Phase 4 pushes. [Phase 5](#phase-5) — the "run aborted" notification path is reused for every failure here.

**What we build.** Every way the multi-process architecture can fail — a visible pane dies, the orchestrator pane dies, the channel between them stalls, a cmux API call hangs, or the operator accidentally closes a pane with cmux's own close-pane gesture — produces the same clean outcome: the run aborts, a "run aborted" notification fires, the workspace transitions to a failed state, sidebar entries are cleared, and the workspace stays open so the operator can inspect what happened.

- A visible display pane dying mid-run aborts the run cleanly: the orchestrator finishes any cmux call already in flight (so cmux is left consistent), then aborts, fires the "run aborted" notification, clears sidebar entries, and exits with a non-zero status.
- The orchestrator pane dying while display panes are still alive: each display pane detects the lost channel within a stall threshold, renders a "run aborted" line on its own pane, and exits. The workspace stays open and the orchestrator pane (visible to the operator via cmux's pane-show controls) shows the exit status and last lines of orchestrator output.
- Every cmux API call has a fixed per-call timeout in the 5–10 second range, not operator-configurable in this release. A timeout is treated as fatal and triggers the same abort path as display loss.
- The same stall threshold is applied to the local interaction channel between orchestrator and display panes: a pane that has not received an expected message within the threshold treats the channel as lost.
- The operator using cmux's own close-pane gesture on any pr9k pane is treated identically to a process crash on that pane. This is called out explicitly in the in-run help modal and the cmux mode setup how-to as a known constraint of cmux mode.
- A forced kill of the orchestrator (one that bypasses normal shutdown) leaves the workspace as an orphan with no cleanup; the operator dismisses it manually. The per-run log file may be truncated short of the moment the forced kill happened, but prior content is intact.

**Why this is Phase 6.** Without robust failure handling, every later failure surfaces as a confusing partial state: the orchestrator running blind, the workspace in an inconsistent place, the operator unsure whether the run is alive or stuck. Each individual failure mode is small to handle on its own, but the value is in covering all of them together — leaving any one out means an operator can still hit a confusing state. It lands after Phases 4 and 5 because the cleanup work (clear sidebar entries, fire abort notification) reuses surfaces those phases built.

**Outcome to demonstrate.**

1. The operator launches pr9k cmux mode and starts a long-running workflow.
2. While the workflow is running, the operator closes the log pane using cmux's own pane-close gesture. Within the stall threshold, a "run aborted" notification fires, sidebar entries clear, and pr9k exits with a non-zero status. The workspace remains open; the operator dismisses it and is returned to their prior workspace.
3. The operator repeats the run and, this time, kills the orchestrator process directly. The display panes detect the loss, each renders a "run aborted" line, and exits. The workspace stays open; the operator reveals the orchestrator pane via cmux's pane-show controls and reads the orchestrator's final output.
4. The operator repeats the run with cmux deliberately wedged (a debug hook simulating a hung response). Within the per-call timeout, the same clean abort path fires.

**Source citations.**
- [Edge Cases and Failure Modes](feature-specification.md#edge-cases-and-failure-modes) — every row covering display loss, orchestrator loss, channel stall, API timeout, pane close, and forced-kill behavior.
- [Coordinations](feature-specification.md#coordinations) — the time-bounded contract on the local interaction channel.
- [Decision Log entries D10, D13, D15, D17, D22, D24, D27](artifacts/decision-log.md) — display loss aborts the run, hidden orchestrator pane, cmux timeout fatal, log artifacts unchanged, prior workspace restored, pane close treated as display loss, timeout value committed to 5–10 second range.
- [Feature Technical Notes T1, T4, T5](artifacts/feature-technical-notes.md) — cmux interface shape (calls can hang silently), display processes update independently, cmux workspace persists after pane exit.

**Connects to.**
- [Phase 7](#phase-7) — the orphan workspaces this phase tolerates are the inputs to Phase 7's startup advisory.

**Preconditions to verify before starting.**
- Confirm the team agrees with a fixed 5–10 second per-call cmux timeout, not operator-configurable in this release (the spec commits to this; reaffirming before the work starts avoids re-litigation in review).
- Confirm the team agrees with the stall threshold value for the local interaction channel (the spec leaves the exact number open). See [OQ-4](#oq-4).

---

### Phase 7: Orphan Workspace Startup Advisory {#phase-7}

**Kind.** Polish.

**Builds on.** [Phase 1](#phase-1) — the workspace naming pattern that lets pr9k recognize its own orphans. [Phase 6](#phase-6) — the failure modes that produce orphans.

**What we build.** At the start of every cmux-mode launch, pr9k counts how many existing cmux workspaces still carry pr9k's naming prefix from prior runs. If any exist, it prints a one-line advisory listing the orphan workspace names and continues the run without waiting for acknowledgement. The advisory is informational only — it does not interactively prompt and does not auto-clean.

- Just after the preflight checks succeed and before pr9k creates its own workspace, pr9k asks cmux for the list of existing workspaces and filters for those whose names match the pr9k prefix.
- If at least one orphan exists, pr9k prints a single line to the launching terminal listing the orphan workspace names. The new run starts immediately afterwards.
- If no orphans exist, the advisory is silent.

**Why this is Phase 7.** Orphan workspaces are a known consequence of the failure modes Phase 6 tolerates: any crash that bypasses cleanup leaves a workspace behind. The advisory bridges that gap with the smallest possible amount of work — operators get visibility into accumulation without pr9k taking on the risk of auto-cleaning workspaces the operator may want to inspect. It lands last because every prior phase can ship without it and because the value of the advisory is proportional to how often crashes happen, which the team will only know after the system runs in real use.

**Outcome to demonstrate.**

1. The operator deliberately produces an orphan workspace by forcing pr9k to stop mid-run without a chance to clean up (as demonstrated in Phase 6's failure-mode test).
2. The orphan workspace remains in cmux's workspace list with its pr9k naming pattern intact.
3. The operator launches pr9k in cmux mode again. After the preflight succeeds and before the new workspace appears, a one-line advisory prints to the launching terminal listing the orphan workspace name. The new run continues without waiting.
4. The operator dismisses the orphan workspace using cmux's controls; on the next launch, the advisory is silent.

**Source citations.**
- [Primary Flow](feature-specification.md#primary-flow) — step 4.
- [Edge Cases and Failure Modes](feature-specification.md#edge-cases-and-failure-modes) — the row covering pr9k cmux launch with an existing orphan.
- [Out of Scope](feature-specification.md#out-of-scope) — automatic cleanup is explicitly deferred.
- [Decision Log entries D11, D23, D28](artifacts/decision-log.md) — orphans are not auto-cleaned, startup advisory exists, advisory fires when any orphan exists.

**Connects to.**
- Nothing — this is the final phase.

**Preconditions to verify before starting.**
- Confirm the team has not received evidence that auto-cleanup of orphan workspaces is needed before the advisory ships. The spec defers auto-cleanup; this phase should not become the trojan horse for adding it.

---

## Open Questions {#open-questions}

> Decisions or verifications the team must resolve before the corresponding phase starts. Each question is presented with realistic options and a recommended answer where one is supportable. Cite open questions as `OQ-N` in follow-up.
>
> **Ordering:** open questions are listed by the lowest-numbered phase they block, ascending.

### OQ-1. Which cmux versions does pr9k commit to supporting? {#oq-1}

**Blocks phase(s).** Phase 1.

The spec commits to a method-presence capability check rather than a version-string match, so any cmux build that exposes the required methods will work. But the setup how-to and the release notes still need a named "minimum tested version" so operators have a concrete target to install and the team has a target for compatibility testing.

- **Option A — Pin to the current latest cmux release as the floor and re-evaluate per release.** Simple and honest: we tested against this version, we know it works, anything older is not supported. New cmux releases get a quick validation pass.
- **Option B — Test against a curated set (latest plus the previous 1–2 releases) and document the range.** Wider compatibility, more test burden.
- **Recommendation: Option A.** The capability check already covers the case where a future cmux removes a required method. Naming a single floor version is the lightest commitment that still answers the operator's "what should I install" question. Re-evaluate at each cmux release rather than maintaining a multi-version compatibility matrix.

### OQ-2. How does the team test cmux mode in CI? {#oq-2}

**Blocks phase(s).** Phase 1 (and every subsequent phase).

cmux is a graphical application that draws into its own OS window. The standard pr9k test suite cannot launch cmux in a headless CI environment, which means the multi-process architecture cannot be exercised end-to-end the way the standard display can.

- **Option A — Mock cmux's programmatic interface in tests; reserve live cmux integration testing for manual runs against a developer machine.** Fast, deterministic, but skips the most expensive failure modes (real timeouts, real pane lifecycle, real notification surface).
- **Option B — Run a live cmux process in CI on a platform that supports it (e.g., a macOS CI runner) for a smoke-test subset.** Slower, more brittle, but exercises the real surface.
- **Option C — Both — mock at the unit level and a live smoke test gated to a macOS CI runner.** Highest coverage, highest cost.
- **Recommendation: Option A for Phase 1 through 5, then revisit for Phase 6.** Mocked tests cover most of the multi-process logic. Phase 6 (failure handling) is the phase where mocked cmux is least convincing because the failures are timing-sensitive. The team should commit to a manual test plan for Phase 6 demos and revisit whether a live CI smoke test is worth adding when Phase 6 is ready to start.

### OQ-3. What is the mechanism for the local interaction channel between orchestrator and display panes? {#oq-3}

**Blocks phase(s).** Phase 2.

The spec calls this the "local interaction channel" and commits to its behavioral contract (time-bounded, carries state pushes and intents, detects loss via a stall threshold) but leaves the mechanism open. The implementation needs to settle this before Phase 2 work begins.

- **Option A — A Unix domain socket per workspace owned by the orchestrator, with each display pane process connecting on startup.** Mature, easy to test, works for both directions of traffic, easy to detect loss (broken connection).
- **Option B — Stdin / stdout pipes between orchestrator and each display pane.** Simpler initially but every pane must be launched by the orchestrator process, which complicates the spawn flow given that cmux is the one creating the panes.
- **Option C — A shared file system surface (named pipes or files) that each process polls.** Avoids socket setup but introduces a polling cadence and is hostile to the stall-threshold semantic.
- **Recommendation: Option A.** A Unix socket fits the multi-process topology cleanly: each display pane process is spawned by cmux but knows where to find the orchestrator's socket via an environment variable or command-line argument; loss detection is the socket's broken-connection signal; the same socket carries the readiness handshake. Option B's lifecycle coupling is wrong-shaped for cmux-spawned processes, and Option C's polling is the wrong loss model.

### OQ-4. What is the stall threshold for the local interaction channel? {#oq-4}

**Blocks phase(s).** Phase 6.

The spec commits to "the channel has a stall threshold" but does not name the value. The team should settle a value before Phase 6 ships so failure-mode tests are deterministic and the documentation is concrete.

- **Option A — Match the cmux per-call timeout (5–10 seconds).** Consistent with the cmux timeout, easy to reason about, possibly too short for the busiest moments of a Claude step.
- **Option B — Substantially longer (30–60 seconds).** More forgiving, but a stalled channel that takes that long to detect means the operator stares at a frozen display for that long before seeing any error.
- **Option C — Configurable.** Adds operator-facing surface area without evidence that any operator wants to change it; matches the rationale the spec used to keep the cmux timeout fixed.
- **Recommendation: Option A.** Same reasoning the spec applied to the cmux timeout: a healthy local channel should never take more than a few hundred milliseconds; aligning the two thresholds gives operators one mental model and one number to remember. Revisit only if operators report false-positive aborts in normal use.

---

*End of outline. If you need to cite a specific phase elsewhere, use its `Phase N` number — those numbers are stable for the life of this document. If you need to cite a specific open question, use its `OQ-N` ID.*
