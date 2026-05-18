# cmux Mode

An opt-in launch mode (`--cmux`) that runs a real pr9k workflow inside a recognisably-named cmux workspace, streaming live state to three visible panes (header, log, footer) over a Unix-socket interaction channel. Phase 1 introduced the four-pane workspace scaffold; Phase 2 added the real workflow execution path.

## Activation

Pass `--cmux` on pr9k's run invocation:

```
pr9k --cmux [--project-dir <path>]
```

The flag is visible and experimental. When set, pr9k runs `cmuxctl.Preflight` then `cmuxctl.RunPhase1`, which creates the workspace, spawns the four pane sub-processes, and hands control to `runCmuxOrchestrator` for the workflow run.

## Workspace scaffold

Phase 2 creates one cmux workspace with four panes:

| Pane | Visible? | Content (Phase 2) |
|---|---|---|
| orchestrator | No (hidden) | `pr9k cmux-pane --role=orchestrator` |
| header | Yes | `pr9k cmux-pane --role=header` — live step checkboxes + iteration counter |
| log | Yes | `pr9k cmux-pane --role=log` — streaming subprocess output |
| footer | Yes | `pr9k cmux-pane --role=footer` — status-line output + shortcut hints + version label |

The orchestrator pane is spawned first (D-3) and immediately hidden via `surface.hide`. The three visible panes connect to the orchestrator over the interaction channel; once all three have sent a `Ready` message (readiness handshake), the workflow begins.

## Workspace name

The workspace name is composed as:

```
pr9k-<sanitized>-<timestamp>
```

where `<sanitized>` is the sanitized form of `filepath.Base(projectDir)` and `<timestamp>` is a nanosecond-precision UTC timestamp (`20060102T150405.000000000Z`). The `pr9k-` prefix makes pr9k workspaces immediately identifiable in cmux's workspace list.

Sanitization rules (D-11): characters outside `[a-zA-Z0-9._-]` become `-`; consecutive hyphens are collapsed to one; leading and trailing hyphens are trimmed; an empty result falls back to `repo`. The pre-sanitized basename is never printed to operator-visible output (D-23).

On a workspace-name collision, `RunPhase1` retries once with a fresh timestamp. A second collision is a fatal error.

pr9k prints the final workspace name to standard output immediately after creation:

```
pr9k workspace: pr9k-myrepo-20260515T123456.000000000Z
```

## Lifecycle

```
launch → preflight → capture prior workspace → create workspace
       → spawn four panes → readiness handshake → workflow run
       → WorkspaceDone broadcast → DoneAck wait → socket teardown
       → wait for operator dismissal
       → WorkspaceClose + focus restore → exit
```

**Preflight** (five checks, run sequentially; first failure short-circuits):

1. cmux binary present on PATH
2. Socket path resolves and is a Unix socket (`CMUX_SOCKET_PATH` or `/run/cmux.sock`)
3. Socket is reachable from the launching terminal (descendants-only check)
4. Socket connection is not refused (socket-enabled check)
5. `system.identify` returns `name="cmux"` (capability check)

**Readiness handshake** — before the first workflow step runs, `Channel.AwaitReady` blocks until all three display roles (`header`, `log`, `footer`) have sent a `Ready` message. The deadline is 10 seconds (`ReadyHandshakeTimeout`). On success, the orchestrator pre-populates the initial header state (D-8) and begins step execution. On timeout, a named-roles error identifies which roles did not check in (e.g., `"ready timeout: missing roles: log, footer"`), and the orchestrator exits non-zero.

**Focus excursions** — the operator can switch away from the pr9k workspace and return; the workspace persists until explicitly dismissed.

**Dismissal** — the dismissal-observation goroutine polls `workspace.list` and `surface.list` every 500 ms (Phase 1's `DismissalObserver`, reused unchanged per D-16). It fires on either:
- The pr9k workspace is no longer present in `workspace.list` (workspace-close gesture)
- Any pane in the workspace transitions to exited state (close-each-pane gesture)

After a dismissal event, teardown runs:
1. Best-effort `WorkspaceClose` (if it fails, an orphan-workspace diagnostic is printed to stderr and the process exits non-zero)
2. Silent `WorkspaceSelect` to the prior workspace (errors ignored; prior workspace may be stale)
3. Observer goroutine joined via WaitGroup

**Signal handling** — SIGINT, SIGTERM, and SIGHUP all trigger the teardown path. A second signal (`os.Exit(1)`) is delivered immediately by the watchdog goroutine without waiting for teardown.

## Preflight failures

All five conditions produce operator-readable error messages:

| Condition | Message |
|---|---|
| Binary not installed | `cmuxctl: cmux is not installed; see the cmux setup how-to` |
| Not running | `cmuxctl: cmux is installed but not running; start cmux and try again` |
| Not a descendant | `cmuxctl: cmux mode must be launched from inside a cmux session (socket: <path>)` |
| Socket disabled | `cmuxctl: cmux socket is disabled in cmux configuration; re-enable it and try again` |
| Version incompatible | `cmuxctl: cmux version is incompatible with pr9k cmux mode: ...` |

## Per-pane rendering

### Header pane

The header pane renders the same step-checkbox grid and iteration counter as the standard-mode status header. State is pushed from the orchestrator via `StateHeader` messages containing:
- `IterationLine` — the current iteration display string
- `StepNames[]` — ordered list of step names
- `StepStates[]` — per-step checked/unchecked/active state

The pane re-renders on every incoming `StateHeader` message.

### Log pane

The log pane streams pre-rendered log lines from the orchestrator. Each `StateLog` message carries a batch of `Lines [][]byte`; the pane appends them to its viewport. The operator can focus the pane and scroll back through the full output. The buffer uses drop-oldest semantics when the orchestrator-side channel is full (D-2).

### Footer pane

The footer pane renders three rows:
1. Status-line output — produced by `statusline.Runner` running inside the footer pane process with a `nil` logger (D-9). Script stderr is visible in the pane at runtime; it is not persisted to the on-disk log.
2. Shortcut hints — delivered as `StateFooter.ShortcutLine` from the orchestrator.
3. Version label — `pr9k v<version>` shown at the trailing edge (D-18).

**Keyboard input** — the footer pane owns the operator's keystrokes. A `KeyHandler` adapter goroutine inside the orchestrator process reads from the `Channel.Recv()` stream, maps incoming `Intent` messages to `KeyHandler.Actions` channel entries, and calls `SetMode` to synchronise the footer's modal state (D-5).

**Help expansion** — pressing `?` expands the help modal inline above the footer row (D-12). The expansion is bounded to the available pane height. Pressing `Esc` collapses it. No separate overlay is needed; the expansion is rendered directly in the footer pane.

**Quit flow** — pressing `q` then `y` produces one `ActionQuit` intent that reaches the orchestrator cleanly. Pressing `q` then `Esc` cancels without effect.

## Completion behavior

When the workflow finishes, the orchestrator:

1. Broadcasts `WorkspaceDone{ExitCode: N}` to all connected display panes.
2. Waits up to 5 seconds for a `DoneAck` from each role (`DoneAckTimeout`). Panes that do not ack within the deadline do not hang the orchestrator.
3. Closes the interaction-channel socket.
4. Leaves the workspace visible in cmux, showing each pane's final state, so the operator can inspect the output.
5. The `DismissalObserver` continues polling; the operator dismisses the workspace to return to the prior workspace.

## Minimum-size advisory

When a display pane's terminal falls below 60 columns × 16 rows (`uichrome.MinTerminalWidth` / `uichrome.MinTerminalHeight`), the pane replaces its normal render with an advisory message asking the operator to widen the window. Normal rendering resumes when the pane is resized above the threshold (D-11).

## Failure paths

| Failure | Operator-visible effect |
|---|---|
| Handshake timeout | Orchestrator prints named-roles error, exits non-zero; remaining panes show "orchestrator unavailable — dismiss the workspace" |
| Display pane exits before `WorkspaceDone` | Orchestrator treats the disconnection as `ActionQuit` and aborts cleanly; the other two panes continue to show their final state |
| Orchestrator crash mid-run | All three display panes lose their socket connection and render "orchestrator unavailable — dismiss the workspace" |
| Fatal poll timeout (3 consecutive timeouts) | `DismissalObserver` fires a fatal dismissal event; pr9k attempts teardown and exits non-zero with the workspace name for manual cleanup |

## Log artifacts

The orchestrator creates the standard per-run `.pr9k/logs/<run-stamp>/` directory and writes per-step JSONL artifacts, identical in structure to standard mode (D-7). The artifacts are equivalent in content to a standard-mode run against the same workflow and inputs, modulo run-specifics (RunStamp, wall-clock timestamps, width-dependent renders) — see D-13 for the equivalence definition.

Status-line runner diagnostics are not persisted in cmux mode (D-9 / YAGNI-7).

## Dismissal-gesture coverage

Both dismissal gestures drive the same Phase 1 `DismissalObserver`:

- **Workspace-close gesture** — closes the pr9k workspace directly; detected via `workspace.list` arm
- **Close-each-pane gesture** — exits individual panes until all are gone; detected via `surface.list` exited-pane arm

Either gesture reaches the same teardown path and results in exit code 0 on success.

## Fatal dismissal escalation

If the dismissal observer sees N=3 consecutive poll timeouts (each call exceeding the 5 s per-call deadline), it fires a fatal dismissal event. pr9k attempts teardown and exits non-zero with a message identifying the workspace name for manual cleanup.

## Out of scope (Phase 2)

- No sidebar entries or notifications in the cmux workspace (planned for Phase 4)
- No in-workspace error-recovery prompts (planned for Phase 3)
- No completion notification to the launching terminal (planned for a later phase)
- No automatic orphan cleanup after crash (orphan workspaces have the `pr9k-` prefix and must be dismissed manually via cmux)
- No live-cmux integration test in CI (deferred to Phase 6 per YAGNI-5)
- No heartbeat indicator forwarding across the process boundary (dropped per D-10 / YAGNI-1)
- No configurable handshake-timeout value (deferred per YAGNI-4)
- No per-pane scrollback persistence to disk (deferred per YAGNI-3; `.pr9k/logs/` artifacts satisfy the need)

## References

- [Feature specification — Phase 1](../plans/cmux-rebuild/phase-1-workspace-lifecycle/feature-specification.md)
- [Feature implementation plan — Phase 2](../plans/cmux-rebuild/phase-2-real-workflow-runs/feature-implementation-plan.md)
- [cmuxctl code package doc](../code-packages/cmuxctl.md)
- [interactionchannel code package doc](../code-packages/interactionchannel.md)
- [Setting up cmux how-to](../how-to/setting-up-cmux.md)
- [Phase 1 implementation decision log](../plans/cmux-rebuild/phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md)
- [Phase 2 implementation decision log](../plans/cmux-rebuild/phase-2-real-workflow-runs/artifacts/implementation-decision-log.md)
