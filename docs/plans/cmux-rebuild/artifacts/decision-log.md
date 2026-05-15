# Decision Log: pr9k Cmux Mode

History, rationale, and rejected alternatives for every decision settled while specifying pr9k cmux mode. Behavioral statements live in [../feature-specification.md](../feature-specification.md); this file is the rationale archive.

## Trivial decisions

_(none — every decision settled so far has either rejected alternatives or load-bearing evidence beyond the user's framing)_

## Full decisions

### D1: Cmux mode is an alternate presentation, not a new workflow

- **Question:** Does cmux mode change WHAT pr9k does, or only HOW it presents the run?
- **Decision:** Cmux mode presents the same run pr9k runs today — same phase semantics (initialize / iteration / finalize), same step definitions, same iteration bounds, same error handling, same exit codes. Only the rendering surface changes.
- **Rationale:** The narrow-reading principle ADR commits pr9k to a clean split between runtime mechanics (Go) and workflow content (`config.json`). Cmux mode is purely runtime mechanics — it touches presentation, not what runs. Keeping workflow semantics identical means `config.json` files written for the standard TUI work unchanged in cmux mode and vice versa.
- **Evidence:** [docs/adr/20260410170952-narrow-reading-principle.md](../../../adr/20260410170952-narrow-reading-principle.md); [investigation.md](../investigation.md) Round 2 ("The workflow engine, claude streaming parser, Docker sandbox, validator, variable substitution — all unchanged").
- **Rejected alternatives:**
  - Make cmux mode a "richer" run with extra steps or extra outputs — rejected because it would couple workflow content to presentation and violate the narrow-reading principle.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** D2, D4, D5, D6, D9, D13, D17
- **Referenced in spec:** Outcome

### D2: Cmux mode is opt-in at launch

- **Question:** Is cmux mode the default, opt-in, or auto-detected?
- **Decision:** Cmux mode is opt-in — the operator explicitly requests it at launch. The exact invocation form (flag vs. subcommand) is open ([OI-1](../feature-specification.md#open-items)).
- **Rationale:** cmux is platform-bound (mac primarily, community Linux/Windows ports). The standard TUI runs anywhere with a terminal. Making cmux mode default would break the standard launch path on any host without cmux. Auto-detection (e.g., "use cmux if we appear to be running inside it") feels clever but couples a heavy presentation choice to environment inference and surprises operators who launch from inside cmux but want the standard TUI.
- **Evidence:** [investigation.md](../investigation.md) Round 2 ("Platform portability ... cmux mode would only work where cmux runs"); cmux community-port observations.
- **Rejected alternatives:**
  - Default to cmux when pr9k detects it is running inside cmux — rejected because it surprises operators who want a single-pane TUI inside a cmux session.
  - Replace the standard TUI entirely — rejected because of platform portability.
- **Linked technical notes:** —
- **Driven by findings:** F28
- **Dependent decisions:** D3, D25
- **Referenced in spec:** Outcome, Actors and Triggers, Out of Scope

### D3: Cmux availability is a hard precondition, not a fallback

- **Question:** If the operator launches in cmux mode but cmux is unreachable, should pr9k fall back to the standard TUI or abort?
- **Decision:** pr9k aborts with an error that names the specific failure condition. No automatic fallback. The four distinguishable failure conditions get distinct error messages ([F1](team-findings.md#f1-distinguishable-cmux-failure-conditions); see also [Edge Cases](../feature-specification.md#edge-cases-and-failure-modes)).
- **Rationale:** Fallback would mask configuration mistakes (e.g., the operator wanted cmux but forgot to start it) and produce a different experience than the operator asked for without telling them. An explicit error keeps the operator's mental model intact. The four distinguishable conditions (not installed, not running, not a cmux child, socket disabled) each have a different corrective action, so collapsing them into a single "cmux unreachable" error would not help the operator.
- **Evidence:** project precedent on subprocess preflight in [docs/code-packages/preflight.md](../../../code-packages/preflight.md) — pr9k already prefers fail-fast preflight errors over silent fallback for Docker and the Claude profile directory.
- **Rejected alternatives:**
  - Fall back to standard TUI on missing cmux — rejected as silent surprise.
  - Prompt the operator to choose — rejected as needless interactive ceremony.
  - Collapse all four cmux-unreachable conditions into a single error message — rejected after [F1](team-findings.md#f1-distinguishable-cmux-failure-conditions) showed operators need different remediation per condition.
- **Linked technical notes:** T2
- **Driven by findings:** F1
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow, Edge Cases and Failure Modes, Out of Scope

### D4: Three-pane vertical layout (header / log / footer) plus a hidden orchestrator pane

- **Question:** What is the initial pane layout in the cmux workspace?
- **Decision:** Four panes total: three vertically stacked visible panes (header on top, tall log in the middle, footer at the bottom) and one hidden orchestrator pane (see [D13](#d13-orchestrator-runs-in-a-hidden-cmux-pane)). The visible layout mirrors the existing TUI's spatial composition so a user moving between modes sees the same regions in the same places.
- **Rationale:** The existing TUI is already laid out vertically in this order; operators have built muscle memory and the existing rendering code can be lifted into per-pane renderers with minimal reshape. A horizontal split or different ordering would require redesigning information density without evidence anyone has asked for it. The hidden orchestrator pane preserves the visible three-region UX while satisfying [D13](#d13-orchestrator-runs-in-a-hidden-cmux-pane)'s ancestry and diagnosability requirements.
- **Evidence:** existing TUI composition in [docs/features/tui-display.md](../../../features/tui-display.md); [investigation.md](../investigation.md) Round 2 sketches the three-visible-pane layout explicitly.
- **Rejected alternatives:**
  - Two visible panes — rejected because conflating any two of header / log / footer would be a visual regression.
  - Four-plus visible panes (e.g., dedicated iteration-log or test-output panes) — rejected as speculative.
  - Horizontal split — rejected because the log pane needs vertical line length for streamed Claude output.
- **Linked technical notes:** T3
- **Driven by findings:** F8
- **Dependent decisions:** D8, D13
- **Referenced in spec:** Outcome, Primary Flow

### D5: Mirror key state into cmux sidebar

- **Question:** Should cmux's native sidebar (status / progress / log entries) be used?
- **Decision:** Yes, for two specific entries: current step name (sidebar status) and iteration counter (sidebar progress, `N / M` form). The full status-line script is *not* mirrored to the sidebar in the initial release ([Deferred (YAGNI)](../feature-specification.md#deferred-yagni)).
- **Rationale:** cmux's sidebar exists precisely so a workspace's state is visible when its panes are not in focus. Step name and iteration progress are the two pieces of state operators most often want to glance at; routing them through the sidebar is the canonical idiomatic use of cmux's API. Restricting to those two avoids duplicating the existing footer-resident status-line script without observable benefit. The sidebar's visual form (a labeled line per workspace, a progress bar) is documented in the spec's User Interactions section so operators understand what they will see.
- **Evidence:** cmux sidebar API (`set_status`, `set_progress`, `log`) documented in [investigation.md](../investigation.md) Round 1.
- **Rejected alternatives:**
  - Mirror the full status line — deferred per YAGNI simpler-version test.
  - No sidebar usage at all — rejected because it would waste cmux's main differentiator without reason.
- **Linked technical notes:** T1
- **Driven by findings:** F2
- **Dependent decisions:** D6, D26
- **Referenced in spec:** Outcome, Primary Flow, User Interactions

### D6: Fire cmux notifications at named lifecycle moments

- **Question:** When should cmux's notification API be invoked?
- **Decision:** At exactly three moments: (1) the full run completes successfully; (2) the run aborts due to error, display loss, or operator pane-close; (3) an error-mode prompt is awaiting the operator's decision. No per-step or per-iteration notifications. The error-mode notification is persistent and re-fires on a regular cadence until answered, and its text directs the operator to the control pane ([D19](#d19-error-mode-notifications-direct-the-operator-to-the-control-pane)).
- **Rationale:** Notifications are interruptive; the operator opts into cmux mode to be informed without staying focused, but a notification per step would become noise. The three chosen moments are the ones where the operator's attention is *required* (terminal state, or a blocking prompt). A one-shot notification for an error prompt can be missed; persistence ensures the blocking state does not silently strand the run.
- **Evidence:** cmux notification API (`notification.create`) documented in [investigation.md](../investigation.md) Round 1; analogous discipline in pr9k's existing error-mode design.
- **Rejected alternatives:**
  - Per-step notifications — rejected as noise.
  - No notifications, sidebar-only — rejected because completion and error events are exactly when the operator most wants a foreground alert.
  - One-shot error-mode notification — rejected because a dismissed notification combined with focus elsewhere strands the orchestrator silently.
- **Linked technical notes:** T1
- **Driven by findings:** F3
- **Dependent decisions:** D19
- **Referenced in spec:** Primary Flow, Alternate Flows and States, Edge Cases and Failure Modes

### D7: Workspace cleanup on completion *(superseded by D14)*

- **Status:** superseded. Originally specified a "brief grace period" auto-close; replaced by [D14](#d14-workspace-closure-is-operator-initiated) after review showed "brief" was not a testable behavioral commitment and regressed the existing TUI's `ModeDone` behavior.
- **Referenced in spec:** —

### D8: Footer pane owns keyboard input

- **Question:** Which pane processes operator keystrokes — only one, all of them, or some combination?
- **Decision:** Only the footer pane processes control keys (quit, continue, retry, skip, help). Header and log panes accept pane-local keys (scrollback in the log) but do not forward intents to the orchestrator. The full mapping of which keyboard modes survive and where they live is recorded in [D20](#d20-keyboard-state-machine-is-owned-by-the-footer-pane).
- **Rationale:** A single control pane keeps the keyboard state machine intact — pr9k already has a multi-mode state machine (see [docs/features/keyboard-input.md](../../../features/keyboard-input.md)); distributing it across three processes would require a distributed-protocol redesign for no observable benefit. The footer is the natural control surface because it already displays the shortcut hints in the standard TUI. The trade-off is that the operator must focus the footer pane to drive pr9k — surfaced as a visible hint, and addressed by the persistent error-mode notification per [D19](#d19-error-mode-notifications-direct-the-operator-to-the-control-pane).
- **Evidence:** [docs/features/keyboard-input.md](../../../features/keyboard-input.md); [investigation.md](../investigation.md) Round 2.
- **Rejected alternatives:**
  - Every pane forwards its own intents to the orchestrator — deferred per YAGNI; see [Deferred (YAGNI)](../feature-specification.md#deferred-yagni).
  - Header is the control pane — rejected because the existing TUI places shortcut hints in the footer; moving them would break muscle memory.
- **Linked technical notes:** —
- **Driven by findings:** F4
- **Dependent decisions:** D20
- **Referenced in spec:** Alternate Flows and States, User Interactions

### D9: Cmux mode applies to the run loop only, not the workflow builder

- **Question:** Does `pr9k workflow` (the workflow builder) also operate in cmux mode?
- **Decision:** No. The workflow builder retains its existing single-window TUI. Only the run loop has a cmux mode.
- **Rationale:** The workflow builder is interactive editing — menus, dialogs, form fields, external editor invocation. Splitting it across cmux panes would require redesigning the editing model without evidence anyone wants it. The run loop is the surface where multi-pane composition pays off (header / log / footer are already conceptually separate); the workflow builder is not.
- **Evidence:** [docs/features/workflow-builder.md](../../../features/workflow-builder.md).
- **Rejected alternatives:**
  - Apply cmux mode to both surfaces — rejected as scope expansion without evidence.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Out of Scope

### D10: Display pane loss aborts the run

- **Question:** If one of the visible panes dies mid-run, what does pr9k do?
- **Decision:** Treat the loss as fatal — abort the run, fire a "run aborted" notification, mark the workspace as failed, exit non-zero. Do not attempt to respawn the display. The trigger condition is broadened by [D24](#d24-operator-pane-close-is-treated-as-display-loss) to cover the operator-initiated close gesture, and by [D15](#d15-cmux-api-per-call-timeout-is-fatal) to cover IPC stalls and cmux timeouts.
- **Rationale:** Display loss usually indicates a crash in pr9k's own code, a cmux failure, or the operator closing a pane manually. In all cases, continuing the run "blind" (orchestrator runs but operator sees nothing) is worse than aborting. Respawning a display introduces stateful recovery logic without evidence any of the failure modes is recoverable in practice.
- **Evidence:** [docs/coding-standards/error-handling.md](../../../coding-standards/error-handling.md); [docs/features/signal-handling.md](../../../features/signal-handling.md).
- **Rejected alternatives:**
  - Respawn the display and continue — rejected because state replay across IPC boundaries is non-trivial and there's no demonstrated need.
  - Continue without the display ("run in the dark") — rejected because the operator loses visibility into a still-running workflow that affects their git history and Docker state.
- **Linked technical notes:** T4
- **Driven by findings:** F5, F11, F23, F26
- **Dependent decisions:** D15, D24
- **Referenced in spec:** Edge Cases and Failure Modes

### D11: Orphan workspaces from crashes are not auto-cleaned

- **Question:** If a prior run crashed and left a cmux workspace open, does the next pr9k launch reuse or clean it?
- **Decision:** Neither. Each new launch creates a fresh, uniquely-named workspace ([D21](#d21-workspace-name-format)). Orphan workspaces from prior runs remain for the operator to inspect and dismiss manually. A startup advisory ([D23](#d23-orphan-workspace-startup-advisory)) surfaces accumulation so operators are not blind to it.
- **Rationale:** Reusing a workspace would require validating its state matches what pr9k expects (panes still present, processes still healthy, no extra panes the operator added), which is fragile. Auto-cleaning would discard forensic evidence the operator might want to read. The simplest, safest behavior is "fresh workspace per run, dead ones stay until you handle them" — with visibility into accumulation.
- **Evidence:** investigation.md Round 2.
- **Rejected alternatives:**
  - Adopt the prior workspace — rejected as fragile.
  - Auto-close orphan workspaces on launch — deferred per YAGNI ([Deferred (YAGNI)](../feature-specification.md#deferred-yagni)).
- **Linked technical notes:** —
- **Driven by findings:** F6
- **Dependent decisions:** D21, D23
- **Referenced in spec:** Edge Cases and Failure Modes, Out of Scope

### D12: Drop cross-pane and keyboard-driven selection

- **Question:** Should pr9k preserve the custom mouse-drag and keyboard-driven selection layers when running in cmux mode?
- **Decision:** No. Cmux mode delegates selection to cmux's native per-pane mouse selection. The custom selection layer (built into the standard TUI to work around alt-screen + mouse-cell-motion conflicts) is unused in cmux mode, and the keyboard `v` select mode is dropped (operators select with the mouse in the pane).
- **Rationale:** cmux's underlying terminal handles native selection per-pane; the operator can select within the header, log, or footer independently. Cross-pane drag is not supported by any terminal multiplexer the operator might be familiar with — so the loss is not a regression against the user's mental model. Keyboard `v` select mode existed in the standard TUI specifically because alt-screen + mouse-cell-motion mode breaks native drag selection; in cmux mode that conflict is gone, so the keyboard alternative is redundant.
- **Evidence:** [docs/adr/20260416-clipboard-and-selection.md](../../../adr/20260416-clipboard-and-selection.md).
- **Rejected alternatives:**
  - Implement cross-pane selection in cmux mode — rejected as engineering not requested.
  - Preserve keyboard `v` select mode in the log pane — rejected because the rationale for its existence no longer applies; cmux mode adds it to Out of Scope and to the [D20](#d20-keyboard-state-machine-is-owned-by-the-footer-pane) "dropped modes" list.
- **Linked technical notes:** —
- **Driven by findings:** F7
- **Dependent decisions:** D20
- **Referenced in spec:** User Interactions, Out of Scope

### D13: Orchestrator runs in a hidden cmux pane

- **Question:** Where does the workflow orchestrator process run — in a detached background process, in the foreground launching terminal, or as a hidden cmux pane in the same workspace as the visible panes?
- **Decision:** The orchestrator runs in a hidden fourth cmux pane in the same workspace. The pane is created at workspace setup time, hosts the orchestrator process directly, and is kept off-screen by default. Operators can reveal the pane via cmux's own pane-show controls if they want to inspect orchestrator state.
- **Rationale:** Three options were on the table — detached background process, foreground launching terminal, hidden pane — and the hidden-pane choice wins on every dimension considered.
  - **Ancestry / socket access.** cmux's default access policy restricts socket connections to descendants of cmux itself ([T2](feature-technical-notes.md#t2-cmux-access-model)). A detached process that double-forks to PID 1 loses that ancestry and cannot reach cmux. A hidden pane is a cmux child by construction.
  - **Diagnosability on crash.** A detached process crash leaves nothing visible; stderr goes to /dev/null unless captured. A hidden-pane crash leaves the pane in cmux with the exit status visible; the operator can show the pane to inspect the final orchestrator output. The foreground-terminal option also has decent crash visibility but couples the workspace lifetime to the launching terminal.
  - **Lifecycle.** cmux owns the pane's lifetime; a hidden pane is alive until cmux closes it. The orchestrator's lifetime is naturally tied to the workspace's lifetime.
  - **Closes original OI-1 and OI-5.** The earlier spec drafts left both as open items. The hidden-pane choice answers both: the orchestrator is launched from inside cmux as a pane (so ancestry is preserved); and cmux mode requires pr9k to be invoked from inside a cmux session (so the launch is itself a child of cmux).
- **Evidence:** [T2](feature-technical-notes.md#t2-cmux-access-model); investigation.md Round 2; review-team findings F8 (diagnosability comparison), F9 (ancestry).
- **Rejected alternatives:**
  - **Detached background process** — rejected because it loses cmux ancestry on double-fork and has the worst crash diagnosability of the three options.
  - **Foreground launching terminal** — rejected because workspace lifetime would be coupled to the launching shell session; closing the shell would orphan the workspace, and crash output would intermix with the operator's shell history.
- **Linked technical notes:** T2, T3, T5
- **Driven by findings:** F8, F9
- **Dependent decisions:** D4, D14, D22
- **Referenced in spec:** Outcome, Primary Flow, Edge Cases and Failure Modes, User Interactions

### D14: Workspace closure is operator-initiated

- **Question:** When the run completes (or aborts), what happens to the cmux workspace?
- **Decision:** The workspace stays open until the operator explicitly dismisses it — either via the footer pane's quit/close shortcut or via cmux's own workspace-close controls. There is no auto-close timer. This matches the standard TUI's existing `ModeDone` behavior, where the TUI remains visible until the operator quits.
- **Rationale:** The initial draft specified "a brief grace period" before auto-close, but review showed "brief" is not a testable behavioral commitment and silently regresses the existing TUI. Operators reading the final completion screen, the failure context, or a Docker image identifier value need an unbounded amount of time; an auto-close timer either runs too short (operator misses content) or so long it provides no value. The simplest answer is no timer — operator owns dismissal.
- **Evidence:** existing `ModeDone` behavior in [docs/features/keyboard-input.md](../../../features/keyboard-input.md); review-team findings F10.
- **Rejected alternatives:**
  - Auto-close after a fixed grace period — rejected as untestable and as a UX regression against the standard TUI.
  - Close instantly on completion — rejected; the final screen is often the most informative.
- **Linked technical notes:** T5
- **Driven by findings:** F10
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, Alternate Flows and States

### D15: cmux API per-call timeout is fatal

- **Question:** What happens when a cmux API call neither succeeds nor fails — when the socket is alive but cmux is hung?
- **Decision:** Every cmux API call has a configured per-call wall-clock timeout. A timeout is treated as fatal: pr9k aborts the run via the same path as display-process loss ([D10](#d10-display-pane-loss-aborts-the-run)). The same stall-detection rule applies to the local interaction channel between orchestrator and display panes.
- **Rationale:** Without a timeout, a hung cmux socket would leave the orchestrator blocked indefinitely while the workflow continues to run against the target repository (Docker containers, git commits, claude calls) with no operator visibility — the named "running visually blind" failure mode that [D10](#d10-display-pane-loss-aborts-the-run) is supposed to prevent. The exact timeout value is left to the implementation but the existence of the timeout is a behavioral commitment.
- **Evidence:** review-team finding F11; [docs/coding-standards/error-handling.md](../../../coding-standards/error-handling.md).
- **Rejected alternatives:**
  - No timeout, rely on the socket eventually returning an error — rejected because a hung remote will never return an error.
  - Timeout that prints a warning but continues — rejected because subsequent calls will hit the same hang and the run state diverges from the operator's view.
- **Linked technical notes:** T1
- **Driven by findings:** F11, F26
- **Dependent decisions:** D10, D27
- **Referenced in spec:** Edge Cases and Failure Modes, Coordinations

### D16: Per-launch readiness handshake before workflow starts

- **Question:** Does the orchestrator wait for all three display panes to be ready before starting the workflow, or does it begin immediately and risk dropping early events?
- **Decision:** The orchestrator waits until all three display panes signal readiness on the local interaction channel before starting the workflow. If any display fails to signal within the channel's stall threshold, the run aborts with the same teardown as display loss.
- **Rationale:** Without a handshake, the first step's header update and first log lines could reach displays that have not yet started rendering — the operator would miss the first events. The handshake adds at most a small startup latency and ensures the operator sees the first step from its first byte.
- **Evidence:** review-team finding F12; [docs/features/tui-display.md](../../../features/tui-display.md).
- **Rejected alternatives:**
  - Start immediately and accept dropped early events — rejected; the operator loses visibility into step 1.
  - Buffer events forever — rejected because slow display startup would buffer unboundedly.
- **Linked technical notes:** T4
- **Driven by findings:** F12
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow

### D17: Log file artifacts are unchanged in cmux mode

- **Question:** Does the existing on-disk logging (per-run `.pr9k/logs/<run>.log`, per-step `.jsonl` artifacts, iteration log) carry over in cmux mode?
- **Decision:** Yes, unchanged. The orchestrator's working directory is the target repository, the file paths are identical to the standard TUI's, and the same content gets written. This is the primary post-mortem surface for cmux-mode failures and is called out explicitly in the spec.
- **Rationale:** A change to logging would be a behavioral regression; preserving it is the obviously correct default. The investigation's "what is lost" list mentioned "logs and crash diagnostics spread out" as a cost of multi-process architecture, which would only be true if logging weren't preserved. Stating this explicitly closes that gap.
- **Evidence:** [docs/code-packages/logger.md](../../../code-packages/logger.md); review-team finding F13.
- **Rejected alternatives:**
  - Move log files into a cmux-specific path — rejected as needless divergence.
  - Drop the per-step `.jsonl` artifacts in cmux mode — rejected because they are downstream tools' input (debugging, the iteration log).
- **Linked technical notes:** —
- **Driven by findings:** F13
- **Dependent decisions:** —
- **Referenced in spec:** Outcome, Primary Flow, Coordinations, Edge Cases and Failure Modes

### D18: Startup capability check

- **Question:** How does pr9k detect a cmux that has shipped a breaking change to its API?
- **Decision:** At startup, pr9k calls cmux's identity / capabilities API to confirm cmux exposes every method pr9k will call. If a required method is absent, pr9k aborts with an error naming the missing methods so the operator can update cmux. pr9k does *not* pin to a specific cmux version string — the contract is method presence, not a version match.
- **Rationale:** A breaking cmux API change without this check would produce cryptic JSON-RPC failures at the first call that hits a renamed or removed method. Checking up-front gives the operator an actionable error before any workspace is created.
- **Evidence:** cmux `system.capabilities` / `system.identify` methods documented in [investigation.md](../investigation.md) Round 1; review-team finding F14.
- **Rejected alternatives:**
  - Pin to a specific cmux version string — rejected because cmux's version surface is not stable enough to bind to.
  - No check; rely on individual call failures — rejected; produces cryptic errors with no remediation guidance.
- **Linked technical notes:** T1
- **Driven by findings:** F14
- **Dependent decisions:** —
- **Referenced in spec:** Actors and Triggers, Primary Flow, Edge Cases and Failure Modes

### D19: Error-mode notifications direct the operator to the control pane

- **Question:** When an error-mode prompt fires and the operator's focus is on a non-control pane, how does the operator know to switch focus?
- **Decision:** The error-mode notification text includes the explicit directive "Focus the pr9k control pane to respond." The notification persists and re-fires on a regular cadence until the operator answers the prompt, so a dismissed notification does not strand the orchestrator.
- **Rationale:** Without the directive, an operator who has never seen a multi-pane pr9k workspace might not know where to go. Persistence ensures the operator can resume the run even after dismissing a notification. The cadence is a behavioral commitment; the exact value is implementation-internal.
- **Evidence:** review-team finding F15.
- **Rejected alternatives:**
  - One-shot notification — rejected; dismissed notifications strand the orchestrator silently.
  - Auto-focus the control pane on error — rejected because forcing focus changes is hostile to operators inspecting the log pane when the error appears.
- **Linked technical notes:** —
- **Driven by findings:** F3, F15
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow, Alternate Flows and States, User Interactions

### D20: Keyboard state machine is owned by the footer pane

- **Question:** Which of the standard TUI's keyboard modes survive in cmux mode, and where do they live?
- **Decision:** Seven of the standard TUI's eight modes survive (Normal, Error, QuitConfirm, NextConfirm, Done, Quitting, Help). The `Select` mode (keyboard-driven log selection via `v`) is dropped per [D12](#d12-drop-cross-pane-and-keyboard-driven-selection). All seven surviving modes are owned by the footer pane — its keyboard state machine is the local state machine. When the operator presses a control key on a non-control pane, the keystroke is absorbed without effect. When the operator triggers `?`, the help modal renders in the footer pane regardless of whether a status-line script is configured (the standard TUI's `StatusLineActive` gate on help is removed in cmux mode).
- **Rationale:** Owning the state machine in the footer keeps it consistent with [D8](#d8-footer-pane-owns-keyboard-input) and avoids a distributed-protocol redesign. The footer process buffers keystrokes locally and forwards intents in arrival order; the orchestrator processes intents serially and ignores conflicting follow-ups once an intent has begun. Dropping `v` select mode follows from [D12](#d12-drop-cross-pane-and-keyboard-driven-selection) — the rationale for its existence no longer applies in cmux mode. The `StatusLineActive` gate on help was a coupling artifact; in cmux mode, help is always available because the operator may need it precisely when the status-line script is absent or broken.
- **Evidence:** [docs/features/keyboard-input.md](../../../features/keyboard-input.md); review-team findings F16, F17, F18.
- **Rejected alternatives:**
  - Move the state machine to the orchestrator — rejected; would require every keystroke to round-trip the local interaction channel and would make the footer pane visibly laggy.
  - Distribute modes across panes (e.g., scroll mode in the log) — deferred per YAGNI; see [Deferred (YAGNI)](../feature-specification.md#deferred-yagni).
- **Linked technical notes:** —
- **Driven by findings:** F4, F7, F16, F17, F18
- **Dependent decisions:** D12
- **Referenced in spec:** Alternate Flows and States, Edge Cases and Failure Modes, User Interactions, Out of Scope

### D21: Workspace name format

- **Question:** What names does pr9k use for the cmux workspaces it creates?
- **Decision:** Workspaces are named with a fixed prefix (so pr9k can detect orphans without false positives) followed by the target repository's basename and a high-resolution timestamp. The exact pattern is recommended in [OI-5](../feature-specification.md#open-items); the pattern uses precision sufficient that two launches within the same wall-clock second do not collide.
- **Rationale:** The prefix lets the startup advisory ([D23](#d23-orphan-workspace-startup-advisory)) detect pr9k workspaces without scanning content. The repo basename lets the operator distinguish concurrent runs against different repos. The timestamp prevents collisions between concurrent or rapid launches.
- **Evidence:** review-team findings F19, F20.
- **Rejected alternatives:**
  - Timestamp-only names — rejected; same-second launches would collide.
  - Random IDs — rejected; operators cannot scan a workspace list and find the run they care about.
  - Repo-only names — rejected; concurrent runs against the same repo would collide.
- **Linked technical notes:** —
- **Driven by findings:** F19, F20, F22
- **Dependent decisions:** D23, D29
- **Referenced in spec:** Primary Flow

### D22: Prior workspace is captured and restored

- **Question:** When the operator dismisses pr9k's workspace, where does focus go?
- **Decision:** At startup (before creating the pr9k workspace), pr9k records which cmux workspace the operator was in. On workspace dismissal — success, abort, or quit — pr9k explicitly selects that prior workspace so cmux returns the operator to the place they came from.
- **Rationale:** Letting cmux choose the next workspace at close time is unpredictable; the operator wants to land back where they were. Capturing and restoring is a small extra step and produces a familiar experience.
- **Evidence:** cmux `workspace.current` and `workspace.select` methods documented in [investigation.md](../investigation.md) Round 1; review-team finding F21.
- **Rejected alternatives:**
  - Let cmux pick — rejected as unpredictable.
- **Linked technical notes:** —
- **Driven by findings:** F21
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow, Alternate Flows and States

### D23: Orphan workspace startup advisory

- **Question:** Should pr9k tell the operator about orphan workspaces when it starts?
- **Decision:** Yes. At startup, pr9k counts how many existing cmux workspaces carry pr9k's naming prefix ([D21](#d21-workspace-name-format)). If any exist (or above a small threshold; see [OI-4](../feature-specification.md#open-items)), pr9k prints a one-line advisory listing the orphan workspace names. The run proceeds without waiting; the advisory is informational only.
- **Rationale:** The crash failure modes ([D10](#d10-display-pane-loss-aborts-the-run), [D15](#d15-cmux-api-per-call-timeout-is-fatal), [D24](#d24-operator-pane-close-is-treated-as-display-loss), SIGKILL edge case) guarantee orphans will accumulate. Awareness is essential; auto-cleanup is deferred. The advisory bridges the gap with minimal cost.
- **Evidence:** review-team finding F22.
- **Rejected alternatives:**
  - Silent — rejected because operators would discover orphans only by listing workspaces manually.
  - Block until acknowledged — rejected as needless ceremony.
- **Linked technical notes:** —
- **Driven by findings:** F6, F22
- **Dependent decisions:** D28
- **Referenced in spec:** Primary Flow, Edge Cases and Failure Modes, Out of Scope

### D24: Operator pane close is treated as display loss

- **Question:** What happens when the operator closes one of pr9k's panes using cmux's own close-pane gesture?
- **Decision:** Treated identically to display-process loss per [D10](#d10-display-pane-loss-aborts-the-run): the run aborts, a "run aborted" notification fires, and pr9k exits non-zero. The cmux setup how-to and the in-run help modal both call this out as a known constraint: closing a pane closes the run.
- **Rationale:** Operators trained on tmux or iTerm splits have pane-close in muscle memory; treating accidental closure as a fatal abort is unsafe by default. However, distinguishing "operator close" from "process crash" would require either a confirmation prompt cmux does not natively support, or a recovery flow that respawns the display — neither is justified by current evidence. The safer choice is to make the behavior visible: operators are told up front that closing a pane closes the run. Future iteration can add a confirmation flow if the constraint proves too punishing in practice.
- **Evidence:** review-team finding F23; investigation.md Round 2 ("Closing a pane is a normal cmux interaction").
- **Rejected alternatives:**
  - Confirmation prompt before allowing the close — rejected because cmux does not natively expose a pane-close hook; pr9k cannot intercept the gesture.
  - Respawn the display and continue — rejected for the same reasons as [D10](#d10-display-pane-loss-aborts-the-run).
- **Linked technical notes:** —
- **Driven by findings:** F23
- **Dependent decisions:** D10
- **Referenced in spec:** Edge Cases and Failure Modes

### D25: Launch surface is the `--cmux` flag

- **Question:** What is the exact invocation form for cmux mode — a flag on `pr9k`, a new subcommand, or a separate binary?
- **Decision:** A `--cmux` flag on the existing run invocation. No new subcommand surface; no separate binary.
- **Rationale:** Consistent with the existing cobra-flag convention established in [docs/adr/20260409135303-cobra-cli-framework.md](../../../adr/20260409135303-cobra-cli-framework.md). A subcommand would imply cmux mode is a separate command tree (it is not — same run loop, same `config.json`, same target repository, same exit codes); a separate binary would fragment installation and packaging. A flag is the lightest-weight addition that honors how operators already type `pr9k`. As [F28](team-findings.md#f28-versioning--documentation-impact-of-the-new-cli-surface-was-unflagged) called out, this is a new public CLI surface and requires a MINOR version bump plus matching feature-doc updates in the same PR per the versioning and documentation standards.
- **Evidence:** [docs/adr/20260409135303-cobra-cli-framework.md](../../../adr/20260409135303-cobra-cli-framework.md); [docs/coding-standards/versioning.md](../../../coding-standards/versioning.md); [docs/coding-standards/documentation.md](../../../coding-standards/documentation.md); operator confirmation accepting the spec's recommended provisional answer.
- **Rejected alternatives:**
  - `pr9k cmux run` subcommand — rejected because cmux mode is not a separate command tree; it is an alternate presentation of the same run.
  - Separate `pr9k-cmux` binary — rejected because it would fragment installation and break shared workflow config discovery.
  - Environment variable (e.g., `PR9K_CMUX=1`) — rejected because hidden launch modes are surprising; explicit flags are more honest.
- **Linked technical notes:** —
- **Driven by findings:** F28
- **Dependent decisions:** —
- **Referenced in spec:** Actors and Triggers

### D26: Status-line script runs in the footer pane unchanged

- **Question:** Does cmux mode honor the existing status-line script, or is the status line routed to cmux's sidebar?
- **Decision:** The existing status-line script runs in the footer pane unchanged. Sidebar mirroring of the status line is deferred per YAGNI ([Deferred (YAGNI)](../feature-specification.md#deferred-yagni)).
- **Rationale:** The status-line script is already a working surface in the standard TUI ([docs/features/status-line.md](../../../features/status-line.md)); reusing it as-is in the footer pane is the simplest path and matches the principle of "cmux mode is an alternate presentation, not a new workflow" ([D1](#d1-cmux-mode-is-an-alternate-presentation-not-a-new-workflow)). Routing the status line into cmux's sidebar would duplicate the information without observable benefit until the footer pane is itself dropped. The sidebar entries pr9k does push ([D5](#d5-mirror-key-state-into-cmux-sidebar)) are limited to step name and iteration progress for precisely this reason: minimum sidebar surface, maximum reuse of existing code.
- **Evidence:** [docs/features/status-line.md](../../../features/status-line.md); [D1](#d1-cmux-mode-is-an-alternate-presentation-not-a-new-workflow); operator confirmation.
- **Rejected alternatives:**
  - Route the status line to the cmux sidebar — deferred per YAGNI simpler-version test.
  - Drop the status line in cmux mode — rejected; the status line is operator-configured behavior and dropping it would be a regression.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** User Interactions

### D27: cmux per-call timeout value

- **Question:** What is the cmux-API per-call timeout value committed in [D15](#d15-cmux-api-per-call-timeout-is-fatal), and is it operator-configurable?
- **Decision:** A fixed timeout in the 5–10 second range (the exact value chosen by the implementation), not operator-configurable in the initial release.
- **Rationale:** 5–10 seconds is long enough that healthy cmux responses always fit (cmux's API methods are control-plane operations that should return in well under a second on a healthy host) and short enough that a hung socket aborts before the operator's Docker / git / claude state diverges materially from the operator's view. Not configurable in the initial release because adding a configuration surface for a value most operators will never need to change is YAGNI; operators on unusually slow hosts can request configuration in a follow-up if real evidence emerges.
- **Evidence:** [D15](#d15-cmux-api-per-call-timeout-is-fatal); operator confirmation.
- **Rejected alternatives:**
  - Sub-second timeout — rejected; healthy cmux on a loaded host could exceed sub-second response times.
  - Configurable from the start — rejected per YAGNI; no operator has reported needing it.
  - No fixed value (let the implementation choose without a stated range) — rejected; a spec-level commitment to "5–10 seconds" keeps the implementation honest and gives reviewers a target to push back against.
- **Linked technical notes:** T1
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Edge Cases and Failure Modes

### D28: Orphan advisory fires when any orphan exists

- **Question:** How many orphan pr9k workspaces trigger the startup advisory ([D23](#d23-orphan-workspace-startup-advisory)), and what does the advisory contain?
- **Decision:** Any time more than zero orphan workspaces exist, pr9k prints a one-line advisory listing the orphan workspace names and continues without waiting for acknowledgement. No threshold above zero; no interactive prompt.
- **Rationale:** Orphans accumulate from crash failure modes ([D10](#d10-display-pane-loss-aborts-the-run), [D15](#d15-cmux-api-per-call-timeout-is-fatal), [D24](#d24-operator-pane-close-is-treated-as-display-loss), SIGKILL edge case). Operator visibility into accumulation matters from the first orphan, not after some arbitrary threshold; thresholds are surface area without value. A single-line listing is enough to give the operator the workspace name(s) needed to dismiss them in cmux. Not waiting for acknowledgement honors the principle that startup advisories should be informational, not interactive — operators who launched pr9k want to run pr9k, not answer prompts.
- **Evidence:** [D23](#d23-orphan-workspace-startup-advisory); operator confirmation.
- **Rejected alternatives:**
  - Threshold > 0 (e.g., advise only when 5+ orphans exist) — rejected as arbitrary; the cost of advising on one orphan is negligible.
  - Block on acknowledgement — rejected as needless ceremony.
  - Don't advise (silent) — rejected per [D23](#d23-orphan-workspace-startup-advisory).
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow

### D29: Workspace name pattern

- **Question:** What is the exact pattern for cmux workspace names committed in [D21](#d21-workspace-name-format)?
- **Decision:** `pr9k-<repo-basename>-<nanosecond-timestamp>`, where `<repo-basename>` is the target repository directory's basename and `<nanosecond-timestamp>` is a high-resolution UTC timestamp with nanosecond precision (e.g., `pr9k-myrepo-20260515T103045.123456789Z`).
- **Rationale:** Three pieces of information, each carrying its own weight: the `pr9k-` prefix lets [D23](#d23-orphan-workspace-startup-advisory) detect orphans without scanning content; the repo basename lets the operator distinguish concurrent runs against different repos in cmux's workspace list; nanosecond precision prevents collisions between rapid or concurrent launches (a same-second collision would silently fail at workspace creation). Timestamps in UTC avoid surprises when operators run pr9k across timezone changes (e.g., when traveling).
- **Evidence:** [D21](#d21-workspace-name-format); operator confirmation; [docs/coding-standards/go-patterns.md](../../../coding-standards/go-patterns.md) timestamp conventions.
- **Rejected alternatives:**
  - Second-precision timestamps — rejected per [F19](team-findings.md#f19-workspace-uniqueness-mechanism-was-unspecified); same-second launches would collide.
  - Local-time timestamps — rejected; UTC is unambiguous across hosts and timezones.
  - Random IDs (e.g., UUID) — rejected; operators cannot scan a workspace list and recognize the run they care about.
  - Repo-only names — rejected; concurrent runs against the same repo would collide.
- **Linked technical notes:** —
- **Driven by findings:** —
- **Dependent decisions:** —
- **Referenced in spec:** Primary Flow
