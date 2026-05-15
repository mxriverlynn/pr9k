# Decision Log: Phase 1 — Cmux Mode Launch and Workspace Lifecycle

History, rationale, and rejected alternatives for every NEW decision settled while specifying Phase 1 of pr9k cmux mode. Parent-level decisions (D1–D29 in [../../artifacts/decision-log.md](../../artifacts/decision-log.md)) are inherited unchanged and are referenced from the spec via parent-decision links; only decisions that arise specifically in Phase 1's scope are recorded here.

Phase 1 contributes twelve new decisions: D1–D8 from the initial draft, and D9–D12 added during review-team finding resolution (Step 7). D6 and D8 were extended during review-team resolution; the rest of the initial-draft decisions stand unchanged.

Behavioral statements live in [../feature-specification.md](../feature-specification.md); this file is the rationale archive.

## Trivial decisions

_(none — every Phase 1-specific decision below carries rejected alternatives or load-bearing rationale beyond the user's framing)_

## Full decisions

### D1: Orchestrator pane runs a placeholder during Phase 1

- **Question:** During Phase 1, when no workflow runs, what process occupies the hidden orchestrator pane?
- **Decision:** A long-lived placeholder process (one that produces no visible output and waits for termination) occupies the orchestrator pane in Phase 1. The orchestrator pane is NOT omitted from the layout, and the pane is NOT spawned with a no-op short-lived process that exits immediately.
- **Rationale:** Parent decision [D13](../../artifacts/decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane) commits to four panes including the hidden orchestrator. Omitting the orchestrator pane in Phase 1 would mean Phase 2 has to re-derive the pane-layout work (where does the orchestrator pane come from, how does it slot into an existing three-pane workspace?). Spawning a short-lived no-op would cause the pane to immediately enter the "process exited" state per [parent T5](../../artifacts/feature-technical-notes.md#t5-cmux-workspace-persists-after-pane-exit), which would look like a crash to anyone inspecting the workspace and would defeat Phase 1's "the four-pane scaffold is intact" demoable. The placeholder must live until the workspace is dismissed.
- **Evidence:** [parent D13](../../artifacts/decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane); [parent T5](../../artifacts/feature-technical-notes.md#t5-cmux-workspace-persists-after-pane-exit); the build phase outline's Phase 1 "What we build" enumerates the orchestrator pane alongside the three visible panes.
- **Rejected alternatives:**
  - Omit the orchestrator pane in Phase 1 — rejected; Phase 2 would have to add it as a separate piece of work, defeating Phase 1's purpose as the scaffold-completing foundation.
  - Spawn a short-lived no-op (e.g., `true`) — rejected; the pane would immediately show "process exited" status and the workspace would look broken.
  - Run the real pr9k workflow runner with zero steps configured — rejected as premature integration of Phase 2 behavior into Phase 1's scope.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D7
- **Referenced in spec:** Outcome, Primary Flow

### D2: Launching terminal prints the workspace name

- **Question:** After cmux creates the new workspace, does the launching terminal print anything to indicate what happened?
- **Decision:** Yes — pr9k prints a single line to the launching terminal naming the new workspace, immediately after `workspace.create` succeeds and before any pane setup begins.
- **Rationale:** The operator's launching terminal would otherwise go silent for the duration of the launch flow, with no indication of whether (a) anything happened, (b) which workspace name was generated, (c) the launch is still alive vs. hung. A single line answers all three questions cheaply. The line is also the paper trail the operator needs to find the workspace in cmux's workspace list (the high-resolution timestamp in the name is hard to type from memory). This decision exists at Phase 1 because the orphan startup advisory (Phase 7) is not yet shipped — without the in-terminal line, the operator's only way to find the workspace name would be to scan cmux's workspace list manually.
- **Evidence:** [parent D29](../../artifacts/decision-log.md#d29-workspace-name-pattern) (nanosecond-precision timestamps are hard to recall); the build phase outline's Phase 1 Outcome step 5 implies operator-visible failure messages but does not commit to a success message; the design parallels how `pr9k sandbox create` already prints user-visible status to the launching terminal ([docs/features/sandbox-subcommand.md](../../../../features/sandbox-subcommand.md)).
- **Rejected alternatives:**
  - Silent on success, output only on failure — rejected; the operator's terminal would look hung for the duration of the workspace's lifetime.
  - Multi-line status log (workspace created, panes spawned, awaiting dismissal, etc.) — rejected as verbose for a single-purpose launching terminal; the workspace itself is the operator's main surface.
  - Print the name only at workspace dismissal — rejected; the operator wants to know the name while they may want to find the workspace, not after they've already dismissed it.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow, User Interactions

### D3: Visible placeholders identify pane role and signal placeholder state

- **Question:** What content do the three visible pane placeholders display?
- **Decision:** Each visible placeholder displays a static label that (a) names the pane's role (header / log / footer) so the operator can confirm the spatial layout is correct, and (b) explicitly signals the pane is a Phase 1 placeholder, not real content. Exact wording is implementation detail.
- **Rationale:** The Phase 1 demoable rests on the operator being able to visually confirm the four-pane scaffold. A blank pane would not confirm the layout (the operator could not tell whether the right pane is in the right place). A label that omits the placeholder signal would lead operators who don't read the Phase 1 docs to file false bug reports ("the log pane just says 'log' and doesn't stream"). The label must do both jobs.
- **Evidence:** the build phase outline's Phase 1 Outcome step 2 commits to "placeholder labels" on the three visible panes; standard usability practice for scaffold UIs is to identify both role and provisional state (Nielsen heuristic: visibility of system status).
- **Rejected alternatives:**
  - Blank placeholder panes — rejected; defeats the layout-confirmation demoable and signals nothing to the operator.
  - Real pr9k UI shells (the actual header / log / footer renderers wired up to no data) — rejected as premature integration of Phase 2 behavior; the labels are deliberately Phase-1-only so no one can confuse them with the real renderers.
  - Role-only labels without "placeholder" signal — rejected; operators who skip the docs would file bug reports against a working scaffold.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, User Interactions

### D4: Launching process blocks until workspace dismissed

- **Question:** Once pr9k has spawned the placeholder panes, does the launching pr9k process exit immediately, or does it stay alive and wait for the workspace to be dismissed?
- **Decision:** The launching pr9k process stays alive and waits for the workspace to be dismissed. The launching terminal session is blocked-on-pr9k for the duration. On dismissal, pr9k restores the prior workspace and exits with a zero status.
- **Rationale:** Three reasons all point the same way. (1) Parent decision [D22](../../artifacts/decision-log.md#d22-prior-workspace-is-captured-and-restored) commits to restoring the prior cmux workspace on dismissal; the call needs an alive process to make it. A detached background process could do it, but per [parent T2](../../artifacts/feature-technical-notes.md#t2-cmux-access-model), a double-fork-to-PID-1 background process would lose cmux ancestry and lose access to the cmux socket — same constraint that drove the orchestrator into a cmux pane in [parent D13](../../artifacts/decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane). (2) The standard pr9k run loop blocks the launching terminal today; cmux mode behaves consistently. (3) A blocked terminal means the operator's `pr9k` shell exit status correctly reports whether the run succeeded or failed — important for any operator wrapping `pr9k --cmux` in a shell script.
- **Evidence:** [parent D22](../../artifacts/decision-log.md#d22-prior-workspace-is-captured-and-restored), [parent T2](../../artifacts/feature-technical-notes.md#t2-cmux-access-model), [parent D13](../../artifacts/decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane); pr9k's existing run-loop blocking behavior per [docs/features/cli-configuration.md](../../../../features/cli-configuration.md).
- **Rejected alternatives:**
  - Exit immediately after pane setup — rejected; nothing would restore the prior workspace on dismissal.
  - Detach and run as a background process — rejected; would lose cmux ancestry and lose socket access.
  - Hand the restore responsibility to a placeholder pane process — rejected; the placeholder dies when the workspace is dismissed, so it cannot survive to make the restore call after the fact.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D6
- **Referenced in spec:** Outcome, Primary Flow

### D5: No in-workspace dismissal affordance in Phase 1

- **Question:** Can the operator dismiss the pr9k workspace using a keystroke inside the footer pane in Phase 1, or only via cmux's own workspace-close controls?
- **Decision:** Only via cmux's own workspace-close controls in Phase 1. The footer placeholder does not respond to keystrokes; the keyboard state machine ships in Phase 2 alongside the rest of the footer pane's real behavior.
- **Rationale:** Wiring even a single keystroke into the footer placeholder in Phase 1 would require shipping a partial keyboard state machine — at minimum the quit-confirmation flow, plus the orchestrator-footer interaction channel that Phase 2 actually owns. That's two phases of work compressed into Phase 1 for negligible benefit (cmux's own workspace-close gesture is always available and is a single interaction). Phase 1 is the foundation phase — it ships the smallest demoable that proves the workspace lifecycle. Adding dismissal shortcuts is Phase 2's job.
- **Evidence:** [parent D8](../../artifacts/decision-log.md#d8-footer-pane-owns-keyboard-input) and [parent D20](../../artifacts/decision-log.md#d20-keyboard-state-machine-is-owned-by-the-footer-pane) commit the keyboard state machine to Phase 2's footer pane; the build phase outline's Phase 2 "What we build" explicitly enumerates the footer-pane keyboard state machine including quit.
- **Rejected alternatives:**
  - Ship the quit shortcut in Phase 1 — rejected; would require the local interaction channel between footer and launching process, which is Phase 2's primary integration work.
  - Ship a single-key "dismiss" placeholder shortcut (no two-step confirmation) — rejected as a regression against the standard TUI's two-step quit, and as an inconsistency the operator would have to unlearn in Phase 2.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow, User Interactions

### D6: Double-signal leaves an orphan workspace; no cleanup attempted; SIGHUP covered

- **Question:** If the operator sends a second SIGINT, SIGTERM, or SIGHUP (or kills the launching pr9k process outright) before graceful shutdown completes, what happens to the partially-torn-down workspace? And does SIGHUP behave the same as SIGINT/SIGTERM in the first place?
- **Decision:** All three signal kinds (SIGINT, SIGTERM, SIGHUP) trigger the same graceful-shutdown path on the first delivery. On a second signal of any of the three kinds during shutdown, pr9k exits immediately. The workspace persists as an orphan with whatever state it had at the moment of forced exit; the prior workspace is not restored. The operator dismisses the orphan manually using cmux's own controls. SIGHUP delivery from the parent shell on terminal close is shell-configuration-dependent (`huponexit` in bash, default behavior in zsh); if the shell does not forward SIGHUP, the pr9k process becomes an orphaned process running until the operator dismisses the workspace through another route. No mid-shutdown cleanup, no second-pass teardown attempt, no cleanup signal handler that itself can be interrupted.
- **Rationale:** Once the operator has signaled "stop now" twice, the obligation to clean up gracefully is forfeited — attempting further cmux API calls in the second-signal path could itself hang, leaving the operator with a process that won't die. This matches pr9k's existing signal behavior ([docs/features/signal-handling.md](../../../../features/signal-handling.md)). SIGHUP is treated identically because semantically all three signals mean "stop now"; distinguishing them would surface a behavioral wrinkle for no operator benefit. The acknowledged limitation that SIGHUP may not be delivered when the parent shell does not forward it is recorded so reviewers do not assume universal SIGHUP coverage — it is a property of the operator's shell environment, not of pr9k. The orphan is annoying but recoverable; a hung pr9k is worse. Phase 7 ships the startup advisory that surfaces orphans on the next launch.
- **Evidence:** [docs/features/signal-handling.md](../../../../features/signal-handling.md); the parent specification's [SIGKILL edge case row](../../feature-specification.md#edge-cases-and-failure-modes) ("the prior workspace is not restored automatically. The operator dismisses the workspace manually"); standard POSIX shell behavior for SIGHUP on terminal close.
- **Rejected alternatives:**
  - Block until graceful cleanup completes regardless of further signals — rejected; the operator might be signaling because graceful shutdown itself is hanging, which is exactly when ignoring signals is unhelpful.
  - Spawn a background watchdog process to clean up after a forced exit — rejected as engineering for a rare case with no operator-actionable benefit (the advisory in Phase 7 handles the cleanup follow-up).
  - Make pr9k self-daemonize on receipt of SIGHUP to survive the parent shell — rejected; would lose cmux ancestry per [parent T2](../../artifacts/feature-technical-notes.md#t2-cmux-access-model), defeating the whole launch.
- **Linked technical notes:** —
- **Driven by findings:** F7
- **Dependent decisions:** —
- **Referenced in spec:** Alternate Flows and States

### D7: Phase 1 placeholder exit collapses to workspace dismissal

- **Question:** During Phase 1, if one of the four placeholder processes exits unexpectedly (the pane shows a non-zero exit status), what does the launching pr9k process do?
- **Decision:** Treat the unexpected placeholder exit identically to operator dismissal: close the workspace, restore the prior workspace (if applicable), exit with a non-zero status. Do NOT respawn the placeholder, do NOT try to keep the workspace alive without all four panes. The detection happens via the dismissal observation contract in [D9](#d9-dismissal-observation-contract), specifically through the placeholder-exit observation.
- **Rationale:** Phase 1's purpose is to prove the four-pane scaffold works end-to-end. A scaffold with three live panes and one dead one is not a working scaffold; continuing to wait for dismissal in that state would surface a broken-looking workspace and make the operator think dismissal itself was broken. Collapsing to dismissal is the simplest behavior that preserves the operator's ability to recover. Phase 6 generalizes display-loss handling with notifications and proper failure-mode separation; Phase 1's simpler subset is enough for the foundation phase.
- **Evidence:** [parent D10](../../artifacts/decision-log.md#d10-display-pane-loss-aborts-the-run) treats display-pane loss as fatal in the running-workflow context, which is the right precedent for Phase 1's placeholder-loss case; the build phase outline assigns Phase 6 the generalized failure-handling responsibility.
- **Rejected alternatives:**
  - Respawn the placeholder — rejected; introduces respawn complexity for a case Phase 6 will handle uniformly.
  - Keep the workspace alive with the remaining panes — rejected; the operator would see a partial scaffold and would not know whether it's intentional.
  - Exit immediately without restoring the prior workspace — rejected; the prior-workspace restore is cheap and useful even in the failure case.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D9
- **Referenced in spec:** Edge Cases and Failure Modes

### D8: Standard pr9k preflight runs before cmux preflight; "success" means zero fatal errors

- **Question:** When the operator launches with the cmux launch flag, does pr9k still run its standard preflight (load `config.json`, validate it, run the Docker / profile-dir checks if claude steps exist), and if so, in what order relative to the cmux preflight? And what counts as "preflight success" — zero output, or zero fatal errors?
- **Decision:** Yes — the standard pr9k preflight runs first, exactly as it does without the cmux flag, including the Docker check and the claude profile-directory check when `config.json` contains any claude steps. "Success" means zero fatal errors as defined by the existing severity-based validator surface ([`validator.FatalErrorCount`](../../../../code-packages/validator.md)); non-fatal warnings and infos are printed and the launch proceeds to cmux preflight. A fatal standard-preflight failure aborts the launch before cmux is contacted at all.
- **Rationale:** Phase 1 does not run a workflow, but the operator's intent is still "launch pr9k in cmux mode against this repo and run my workflow later phases" — they would not want to discover, after standing up a cmux workspace and exiting it, that their `config.json` is broken or Docker is not running. Running the standard preflight first surfaces those errors in the launching terminal where the operator expects them, before anything cmux-related happens. The reverse order (cmux first, standard second) would mean a broken `config.json` always produces a workspace creation + immediate teardown, wasting cmux state and confusing the operator. Skipping the standard preflight entirely is unsafe. The "zero fatal errors" semantic is the existing pr9k contract (warnings and infos are surfaced but non-blocking); diverging from it in cmux mode would be a surprise. The Docker and profile-directory checks remain gated on whether `config.json` contains claude steps — an operator whose config has no claude steps can demo Phase 1 on a machine without Docker, consistent with the existing pr9k non-cmux launch behavior.
- **Evidence:** [docs/code-packages/preflight.md](../../../../code-packages/preflight.md) (existing preflight ordering and the `hasClaudeSteps` gate); [docs/code-packages/validator.md](../../../../code-packages/validator.md) (severity-based validator surface with `FatalErrorCount`/`IsFatal`); [pr9k main entry point](../../../../../src/cmd/pr9k/main.go) showing the existing startup ordering (`startup()` prints non-fatal findings then proceeds).
- **Rejected alternatives:**
  - Cmux preflight first, standard preflight second — rejected; would create and tear down a cmux workspace just to discover a broken config or missing Docker.
  - Skip the standard preflight in cmux mode — rejected; the operator's workflow content is still validated, just later (when Phase 2 ships).
  - Skip the Docker preflight specifically in Phase 1 because no workflow runs — rejected; diverges from non-cmux behavior, sets up a Phase 2 surprise when the same operator discovers Docker is required after all.
  - Treat any non-fatal warning as fatal in cmux mode — rejected; diverges from the existing pr9k contract for no operator benefit.
  - Run them in parallel — rejected as premature concurrency; both preflights are fast and the sequential output is easier for the operator to read.
- **Linked technical notes:** —
- **Driven by findings:** F8, F9
- **Dependent decisions:** —
- **Referenced in spec:** Coordinations

### D9: Dismissal observation contract

- **Question:** How does the launching pr9k process know that the operator has dismissed the cmux workspace? The spec's "the launching process waits" commitment requires a concrete observable event.
- **Decision:** The launching pr9k process observes two events; either one is treated as workspace dismissal. (a) The pr9k workspace's name disappears from cmux's workspace list, indicating cmux closed the workspace (whether via the operator's workspace-close gesture, via a cmux-side cleanup, or because pr9k itself closed it during shutdown). (b) Any of the four spawned placeholder processes exits (which happens when the operator uses cmux's per-pane close gesture on any pane, or when a placeholder crashes on its own). The mechanism by which these events are observed — polling vs. async subscription, frequency, ordering — is implementation detail and is not specified here.
- **Rationale:** Both reviewers (junior-developer and edge-case-explorer) independently flagged that the spec committed to "wait for dismissal" without naming the observable event. The two events cover all three dismissal gestures cmux exposes — workspace-close, close-all-panes individually, and close-a-single-pane — without forcing pr9k to inspect cmux's UI gesture stream (which cmux does not expose). The workspace-list observation handles the case where cmux processes a workspace-close gesture but the placeholder panes have already exited; the placeholder-exit observation handles the case where the operator closes panes individually without explicitly closing the workspace ([parent T5](../../artifacts/feature-technical-notes.md#t5-cmux-workspace-persists-after-pane-exit) confirms a workspace persists if its panes exit without an explicit workspace-close). Reacting to either event is what makes [D7](#d7-phase-1-placeholder-exit-collapses-to-workspace-dismissal)'s "placeholder exit collapses to dismissal" rule actually work. Leaving the mechanism (polling vs. subscription) to implementation is correct because cmux's JSON-RPC interface ([parent T1](../../artifacts/feature-technical-notes.md#t1-cmux-programmatic-interface-shape)) admits either approach and the operator's experience is the same in both cases.
- **Evidence:** review-team findings (junior-developer F1, edge-case-explorer F4 and F13); [parent T1](../../artifacts/feature-technical-notes.md#t1-cmux-programmatic-interface-shape); [parent T5](../../artifacts/feature-technical-notes.md#t5-cmux-workspace-persists-after-pane-exit); [D7](#d7-phase-1-placeholder-exit-collapses-to-workspace-dismissal) (already commits to treating placeholder exit as dismissal).
- **Rejected alternatives:**
  - Observe only the workspace-list disappearance — rejected; the close-all-panes gesture would not trigger it (the workspace persists after all panes exit per parent T5), leaving pr9k blocked indefinitely.
  - Observe only the placeholder-process exits — rejected; if pr9k itself closes the workspace during the SIGINT path, the workspace disappears immediately while the placeholder exit notifications race the workspace-list update — relying on placeholder exits alone makes the shutdown ordering implementation-fragile.
  - Specify polling-vs-subscription in the spec — rejected as implementation detail; either approach satisfies the behavioral contract.
  - Specify a particular polling cadence — rejected; cadence is an implementation tuning parameter, not a behavioral commitment. The per-call cmux timeout from [parent D15](../../artifacts/decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) bounds the worst case in either approach.
- **Linked technical notes:** —
- **Driven by findings:** F1
- **Dependent decisions:** D7
- **Referenced in spec:** Primary Flow, Coordinations, Edge Cases and Failure Modes

### D10: "No prior workspace" state handled gracefully

- **Question:** What does pr9k do when the operator launches from a cmux session that contains no workspace other than the launching pane, or when the prior workspace it captured at startup has been closed by the time pr9k tries to restore focus?
- **Decision:** In both cases, pr9k proceeds with the launch (or the dismissal) without raising an error. At startup, if no prior workspace exists to capture, pr9k records that state and skips the explicit focus-restore call on dismissal, letting cmux apply its default focus behavior. At dismissal, if the captured prior workspace's name no longer resolves to an existing workspace, pr9k's workspace-select call may fail or no-op; pr9k ignores any error from the call, prints nothing extra to the launching terminal, and exits with the status it would otherwise have exited with.
- **Rationale:** Both situations are reachable by realistic operator workflows. The "no prior workspace" case is hit by any developer who opens a fresh cmux session and immediately runs `pr9k --cmux` (edge-case-explorer F1). The "prior workspace closed before restore" case is hit by an operator who manages cmux from a second terminal session during pr9k's lifetime (edge-case-explorer F3, junior-developer F6). The simplest correct behavior is "best-effort restore" — pr9k always tries, but never errors if the captured target is no longer valid. The operator never expects a precondition error for either case; they expect pr9k to do its best and exit cleanly. Aborting the launch in the "no prior workspace" case would be hostile to first-time operators in a fresh cmux session.
- **Evidence:** review-team findings (edge-case-explorer F1 and F3, junior-developer F6); [parent D22](../../artifacts/decision-log.md#d22-prior-workspace-is-captured-and-restored) ("explicitly selects that prior workspace so cmux returns the operator to the place they came from") — parent D22 commits to the restore intent but does not commit to erroring when the intent cannot be satisfied.
- **Rejected alternatives:**
  - Treat "no prior workspace" as a preflight failure — rejected; first-time operators in a fresh cmux session would be blocked from demoing Phase 1.
  - Print a warning to the launching terminal when the restore is skipped — rejected; adds noise for a case the operator already expects (cmux's default focus behavior takes over).
  - Try to predict which workspace cmux will focus and restore to that — rejected; cmux's default focus selection is its own concern, and pr9k should not race or duplicate it.
- **Linked technical notes:** —
- **Driven by findings:** F2, F6
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow, Edge Cases and Failure Modes

### D11: Repo basename sanitized before use in workspace name

- **Question:** What does pr9k do when the target repository's basename contains characters cmux rejects in workspace names, or is empty?
- **Decision:** pr9k sanitizes the basename before constructing the workspace name. The sanitization is: replace any character outside `[a-zA-Z0-9._-]` with a single hyphen, collapse runs of hyphens into a single hyphen, trim leading and trailing hyphens. If the sanitized result is empty, pr9k substitutes the literal string `repo`. The sanitized basename is then composed into the standard pattern `pr9k-<sanitized-basename>-<nanosecond-timestamp>`. The launching-terminal confirmation line shows the sanitized name so the operator sees exactly what cmux's workspace list will show.
- **Rationale:** Operators name repositories with dots, spaces, Unicode, and occasionally hyphens, and they pass arbitrary paths via `--project-dir`. Sending unsanitized text into cmux's workspace name surface risks either a cmux-side rejection (the partial-setup failure path would fire with a confusing cmux error message that does not name the basename as the cause), or a workspace name that renders as mojibake in cmux's workspace list. Sanitizing up-front gives a deterministic, scannable name regardless of input. The chosen character set (`[a-zA-Z0-9._-]`) is conservative — it admits every character that is portable across cmux's UI surface and avoids characters that have meaning in shell commands or display routines. The `repo` fallback for empty sanitized basenames (e.g., `--project-dir /` produces basename `/` which sanitizes to empty) keeps the workspace-name pattern stable rather than producing `pr9k--<timestamp>` with an empty middle segment.
- **Evidence:** review-team findings (edge-case-explorer F2 and F8); [parent D29](../../artifacts/decision-log.md#d29-workspace-name-pattern) (defines the name pattern but leaves the basename's character set unspecified); [docs/coding-standards/go-patterns.md](../../../../coding-standards/go-patterns.md) (timestamp conventions).
- **Rejected alternatives:**
  - Send the unsanitized basename and rely on cmux to reject — rejected; produces partial-setup failures with confusing diagnostics, no operator-actionable corrective.
  - Validate the basename before workspace creation and abort with a preflight message — rejected; would require the operator to choose a different `--project-dir` or rename their repo to use cmux mode, which is hostile when sanitization can succeed silently.
  - Use a more permissive character set (e.g., allow Unicode) — rejected; cmux's exact character acceptance varies across versions and platforms, and a conservative set is portable across the cmux versions [OI-1](#open-items) will eventually pin.
  - Use `unknown` instead of `repo` as the empty-sanitization fallback — rejected; `repo` is more accurate for the role and less alarming in cmux's workspace list.
- **Linked technical notes:** —
- **Driven by findings:** F3, F15
- **Dependent decisions:** D12
- **Referenced in spec:** Primary Flow, Edge Cases and Failure Modes, User Interactions

### D12: Workspace name collision retried once then fails

- **Question:** [Parent D29](../../artifacts/decision-log.md#d29-workspace-name-pattern) committed to nanosecond-precision timestamps to prevent same-second name collisions, but nanosecond precision reduces rather than strictly eliminates the collision probability. What does pr9k do when `workspace.create` is rejected because the generated name already exists?
- **Decision:** pr9k regenerates the timestamp once (producing a new nanosecond-precision value) and retries `workspace.create` exactly once. If the retry also fails with a duplicate-name error, pr9k treats this as a partial-setup failure: prints the failing call's diagnostic to the launching terminal and exits with a non-zero status. No further retries are attempted.
- **Rationale:** True same-nanosecond collisions are vanishingly rare in practice (two pr9k launches would have to hit the same nanosecond on the same host clock), but they are not strictly impossible — especially on hosts where the wall clock has lower-than-nanosecond hardware resolution, or in scripted-launch scenarios (CI/CD, shell loops). A single retry with a fresh timestamp handles the realistic case essentially always (a second collision would require a yet-rarer event). Retrying more than once would be defensive coding without evidence; the second collision suggests something else is wrong (e.g., the host clock is broken, or another process is creating workspaces with colliding names) and routing to the partial-setup failure path lets the operator see the diagnostic and act.
- **Evidence:** review-team finding edge-case-explorer F6; [parent D29](../../artifacts/decision-log.md#d29-workspace-name-pattern) (asserts the collision-prevention claim that this decision refines).
- **Rejected alternatives:**
  - No retry — rejected; would surface a same-nanosecond collision (real, albeit rare) as a confusing partial-setup failure with no operator corrective.
  - Unbounded retry until success — rejected as defensive coding; a persistent collision indicates a deeper problem that should be diagnosed, not papered over.
  - Add a random suffix to the name after first collision — rejected; would make the workspace name harder for the operator to recognize, defeating the scannability commitment of parent D29.
  - Retry with the same timestamp — rejected as obvious noise; the timestamp is the reason the call failed.
- **Linked technical notes:** —
- **Driven by findings:** F13
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow, Edge Cases and Failure Modes
