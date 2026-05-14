# Feature Technical Notes: pr9k Cmux Mode

Load-bearing mechanics that the behavioral specification of pr9k cmux mode relies on. Each note exists because the spec's behavioral commitment is only correct if the named mechanic is honored, and the mechanic is *not* discoverable from pr9k's current code repo — it is a property of cmux that pr9k must respect.

**Scope note:** these are implementation-context notes that explain *why* a behavioral commitment in the spec is correct. They are not themselves behavioral commitments — the behavior is in [../feature-specification.md](../feature-specification.md), and the implementation plan is free to choose any mechanism that satisfies the spec's behavioral guarantees, provided the mechanic respects the constraints described here.

## T1: cmux programmatic interface shape

- **Context:** the spec's [Coordinations](../feature-specification.md#coordinations) row for cmux commits pr9k to "outbound" calls that create workspaces, split panes, spawn processes, push sidebar entries, fire notifications, focus workspaces, and close workspaces. That commitment, and the per-call timeout in [D15](decision-log.md#d15-cmux-api-per-call-timeout-is-fatal), are only correct because cmux exposes a specific programmatic surface.
- **Technical detail:** cmux exposes a control-plane API only — there is no facility to read pane contents, subscribe to pane output, capture pane state, or render cmux in a headless mode. The methods pr9k uses fall in six categories: workspace lifecycle, surface (pane) splitting/focus, input forwarding into a surface's stdin, notifications, sidebar status / progress / log, and system identity (used by [D18](decision-log.md#d18-startup-capability-check) to detect API skew). The protocol is newline-delimited JSON-RPC 2.0 over a Unix domain socket. Calls are individually addressable by request id; responses indicate success or a failure with diagnostic text. Calls can hang silently — the socket may accept connections and complete handshakes but never deliver a response if cmux's event loop is saturated or deadlocked, which is why [D15](decision-log.md#d15-cmux-api-per-call-timeout-is-fatal) commits to a per-call timeout.
- **Supports decisions:** D5, D6, D15, D18
- **Driven by findings:** —
- **Referenced in spec:** Preconditions, Coordinations, Edge Cases and Failure Modes

## T2: cmux access model

- **Context:** the spec's [Preconditions](../feature-specification.md#actors-and-triggers) require pr9k to be invoked from inside a cmux session, [Edge Cases and Failure Modes](../feature-specification.md#edge-cases-and-failure-modes) commits to a clean abort if cmux is unreachable for any of four distinguishable reasons, and [D13](decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane) chooses the hidden-pane orchestrator location partly to preserve cmux ancestry. All three commitments are dictated by cmux's specific access model — not by a pr9k policy.
- **Technical detail:** cmux's socket has three access modes. The default mode permits only processes that are descendants of cmux itself to connect; an "allow all" mode is opt-in, and a "disabled" mode shuts the socket off entirely. pr9k cannot widen cmux's policy from outside, so the spec's launch flow is constrained: pr9k must be a descendant of cmux for the default policy to admit it. A detached background orchestrator that double-forks to PID 1 would lose ancestry; this is why [D13](decision-log.md#d13-orchestrator-runs-in-a-hidden-cmux-pane) chose the hidden-pane location, which preserves ancestry by construction. There is no credential exchange — admission is purely ancestry-based on Unix socket permissions.
- **Supports decisions:** D3, D13
- **Driven by findings:** —
- **Referenced in spec:** Preconditions, Edge Cases and Failure Modes

## T3: cmux surfaces are processes, not paintable canvases

- **Context:** the spec's [Primary Flow](../feature-specification.md#primary-flow) commits pr9k to spawning *one process per pane* and pushing state to those processes over an interaction channel. That commitment is only correct because cmux's surfaces cannot be painted from outside cmux.
- **Technical detail:** each cmux surface is a terminal pane that hosts a process; the surface's visible content is whatever that process writes. cmux's API does not expose a "draw text at coordinates" operation; the only ways to influence what a surface shows are (a) spawn a process in it whose output becomes the visible content, or (b) forward keystrokes into the surface's stdin. Consequently, pr9k must run a real renderer process inside each pane — it cannot stream rendered frames into cmux's panes from the outside. This constraint extends to the orchestrator pane: pr9k cannot "hide content" from outside; the pane is hidden via cmux's pane-show-hide mechanism, not by suppressing what its process writes.
- **Supports decisions:** D4, D13
- **Driven by findings:** —
- **Referenced in spec:** Primary Flow, Coordinations

## T4: Display processes update independently

- **Context:** the spec's [Coordinations](../feature-specification.md#coordinations) row for display panes commits pr9k to *eventual* per-pane consistency rather than atomic cross-pane redraws; [D10](decision-log.md#d10-display-pane-loss-aborts-the-run) treats display loss as fatal; and [D16](decision-log.md#d16-per-launch-readiness-handshake-before-workflow-starts) requires a readiness handshake before the workflow starts. All three commitments rest on the fact that the three display panes are independent processes.
- **Technical detail:** because each pane runs its own renderer process and receives updates over an out-of-process channel, an event that semantically affects all three panes (e.g., "iteration ticks over") will reach the three renderers at slightly different times — on the order of single-digit milliseconds. Operators familiar with today's single-process TUI experience atomic cross-region redraws; cmux mode is observably looser. Independence also means the loss of any one display does not stall the others — they continue rendering whatever they last received until the orchestrator notices and aborts. The "fatal on display loss" rule depends on this independence: respawning a display means catching it up to current state across the channel, which is not specified. The readiness handshake is needed because process startup order across cmux's pane-spawn calls is not guaranteed; the orchestrator must not push events until every display has signaled readiness, or the operator misses the first events.
- **Supports decisions:** D10, D16
- **Driven by findings:** —
- **Referenced in spec:** Primary Flow, Coordinations, Edge Cases and Failure Modes

## T5: cmux workspace persists after pane exit

- **Context:** the spec's [Edge Cases and Failure Modes](../feature-specification.md#edge-cases-and-failure-modes) row for "orchestrator pane dies but display panes are still alive" commits to the workspace remaining open so the operator can read the residual state; and [D14](decision-log.md#d14-workspace-closure-is-operator-initiated) commits to operator-initiated workspace closure. Both rely on cmux's specific lifecycle: a workspace persists even after its panes' processes exit. If cmux instead auto-closed workspaces when their last pane exited, these commitments would be wrong.
- **Technical detail:** cmux models workspaces and panes as independent objects with independent lifecycles. A pane whose process exits remains in the workspace (showing the exit status to the operator) until explicitly closed; a workspace whose panes have all exited remains in cmux's workspace list until explicitly closed via `workspace.close` or via the operator's workspace-close gesture. pr9k's behavioral commitments rely on this lifecycle: a crashed orchestrator pane leaves its exit status visible for diagnosis; a completed run leaves its final pane state available until the operator dismisses it. If a future cmux release auto-closes empty workspaces, the spec's "orchestrator pane dies" row and [D14](decision-log.md#d14-workspace-closure-is-operator-initiated) would need to change to a different visible-state strategy.
- **Supports decisions:** D13, D14
- **Driven by findings:** —
- **Referenced in spec:** Edge Cases and Failure Modes
