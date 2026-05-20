# cmux Mode

An opt-in launch mode (`--cmux`) that runs a real pr9k workflow inside a recognisably-named cmux workspace, streaming live state to three visible panes (header, log, footer) over a Unix-socket interaction channel. Phase 1 introduced the four-pane workspace scaffold; Phase 2 added the real workflow execution path; Phase 3 added interactive error recovery — a failing step is now recoverable from inside the footer pane with the same continue / retry / quit semantics as the standard display.

## Activation

Pass `--cmux` on pr9k's run invocation:

```
pr9k --cmux [--project-dir <path>]
```

> **Reworked against real cmux v2 (Rework R / Architecture A — decision-log D-R1/D-R2).** Verified at cmux 0.64.7 / commit `4d04459dd`. cmux v2 has no `surface.spawn`/`surface.hide`; there is **no hidden orchestrator pane**. See [../plans/cmux-rebuild/access-denied-misclassification-investigation.md](../plans/cmux-rebuild/access-denied-misclassification-investigation.md) and [../plans/cmux-rebuild/v2-rework-plan.md](../plans/cmux-rebuild/v2-rework-plan.md). Sections below describing a hidden 4th pane / `surface.spawn` / `name=="cmux"` describe the superseded design.

The flag is visible and experimental. When set, pr9k runs `cmuxctl.Preflight` then `cmuxctl.RunPhase1`. The pr9k process the operator launched inside a cmux pane **is** the orchestrator; it creates the workspace and its three display surfaces, then runs the workflow, streaming state to the panes over the interaction channel.

## Workspace scaffold

The orchestrator (the in-pane pr9k process) creates one cmux workspace with **three** surfaces — there is no hidden orchestrator pane:

| Surface | Created by | Content |
|---|---|---|
| log | `workspace.create` (first surface) | `pr9k cmux-pane --role=log` — streaming subprocess output |
| header | `surface.split` (`up`) | `pr9k cmux-pane --role=header` — live step checkboxes + iteration counter |
| footer | `surface.split` (`down`) | `pr9k cmux-pane --role=footer` — status-line output + shortcut hints + version label |

Each surface's process command embeds `PR9K_CMUX_SOCKET`/`PR9K_PROJECT_DIR` (cmux v2 `surface.split` has no `initial_env`). The three panes connect back to the orchestrator over the interaction channel; once all three send `Ready`, the workflow begins.

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
       → workspace.create (log) + 2 surface.split (header, footer) → readiness handshake → workflow run
       → WorkspaceDone broadcast → DoneAck wait → socket teardown
       → wait for operator dismissal
       → WorkspaceClose + focus restore → exit
```

**Preflight** (five checks, run sequentially; first failure short-circuits):

1. cmux binary present on PATH
2. Socket path resolves and is a Unix socket (cmux's discovery contract: `CMUX_SOCKET_PATH` → `CMUX_SOCKET` → `last-socket-path` marker file → `os.UserConfigDir()/cmux/cmux.sock` → `/tmp/cmux.sock`)
3. Socket is reachable from the launching terminal (descendants-only check)
4. Socket connection is not refused (socket-enabled check)
5. `system.identify` succeeds and returns a non-empty `socket_path` (capability check; cmux v2 has no name/version — D-R2)

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
| Socket not found | `cmuxctl: cmux socket not found at <path> (looked in: <locations>); start cmux, then launch pr9k from inside a cmux pane, or set CMUX_SOCKET_PATH` |
| Not a descendant | `cmuxctl: cmux mode must be launched from inside a cmux session (socket: <path>)` |
| Socket disabled | `cmuxctl: cmux socket is disabled in cmux configuration; re-enable it and try again` |
| Access denied (cmuxOnly) | `cmuxctl: cmux denied access — run pr9k from a terminal pane inside the cmux session … or set cmux's socket control mode to allow-all` |
| Auth required | `cmuxctl: cmux socket requires authentication (auth_required) — set CMUX_SOCKET_PASSWORD …` |
| Unexpected identify | `cmuxctl: unexpected cmux identify response (no socket_path) …` |

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

## Interactive error recovery

When a workflow step exits non-zero in a way that triggers pr9k's existing error mode, the cmux workspace becomes interactively recoverable. The same condition that triggers error mode in the standard display triggers it here — the semantics are inherited unchanged:

- **Header pane** — the failing step's checkbox shows `[✗]`. A successful retry advances it to `[✓]`; a retry that fails again marks it `[✗]` again.
- **Log pane** — the error output appears as ordinary streamed content. On retry, a separator line (`── <step name> (retry) ─────────────`) is written to the log **before** any retried-step output.
- **Footer pane** — switches to the error-mode shortcut hints (continue / retry / quit).

The operator focuses the footer pane and resolves the error:

- **Continue (`c`)** — advances past the failed step and resumes the workflow.
- **Retry (`r`)** — re-runs the step with the same resolved command and prompt. A retry that fails again re-enters error mode with the separator written first; there is no retry-count cap and no auto-timeout.
- **Quit (`q`)** — runs the two-step confirmation (`q` then `y`). Pressing `Esc` or `n` cancels and returns the footer to the error prompt. Any non-confirmation key during the quit confirmation is ignored and not buffered for replay.

Control keys pressed while the header or log pane is focused are absorbed silently — no error, no bell, no notification. The footer pane is the only control surface. If a step fails while the operator's focus is on a different pane or workspace, the orchestrator blocks indefinitely; the run stays paused until the footer pane is focused and the prompt is answered. A directing cmux notification ships in Phase 5.

The three in-workspace failure signals (header `[✗]`, error output in the log, error-mode hints in the footer) converge near-simultaneously, subject to the same small bounded cross-pane desynchronization Phase 2 accepts — each pane is an independent renderer. A keystroke delivered in the brief window between the prompt appearing and the orchestrator entering error mode is not dropped: the buffered `Actions` channel and the synchronous `onModeChange` hook ensure it is acted on once the orchestrator is ready.

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

## Out of scope (Phase 3)

- No cmux notification on step failure directing the operator to the footer pane (planned for Phase 5)
- No sidebar failure label on step failure (planned for Phase 4)
- No sidebar entries or notifications in the cmux workspace (planned for Phase 4)
- No completion notification to the launching terminal (planned for a later phase)
- No generalized failure handling for display-pane loss, orchestrator loss, or interaction-channel stalls during error mode (planned for Phase 6; behavior is unchanged from Phase 2)
- No automatic orphan cleanup after crash (orphan workspaces have the `pr9k-` prefix and must be dismissed manually via cmux)
- No live-cmux integration test in CI (deferred to Phase 6 per YAGNI-5)
- No heartbeat indicator forwarding across the process boundary (dropped per D-10 / YAGNI-1)
- No configurable handshake-timeout value (deferred per YAGNI-4)
- No per-pane scrollback persistence to disk (deferred per YAGNI-3; `.pr9k/logs/` artifacts satisfy the need)

## References

- [Feature specification — Phase 1](../plans/cmux-rebuild/phase-1-workspace-lifecycle/feature-specification.md)
- [Feature implementation plan — Phase 2](../plans/cmux-rebuild/phase-2-real-workflow-runs/feature-implementation-plan.md)
- [Feature specification — Phase 3](../plans/cmux-rebuild/phase-3-interactive-error-recovery/feature-specification.md)
- [Feature implementation plan — Phase 3](../plans/cmux-rebuild/phase-3-interactive-error-recovery/feature-implementation-plan.md)
- [cmuxctl code package doc](../code-packages/cmuxctl.md)
- [interactionchannel code package doc](../code-packages/interactionchannel.md)
- [Setting up cmux how-to](../how-to/setting-up-cmux.md)
- [Phase 1 implementation decision log](../plans/cmux-rebuild/phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md)
- [Phase 2 implementation decision log](../plans/cmux-rebuild/phase-2-real-workflow-runs/artifacts/implementation-decision-log.md)
- [Phase 3 implementation decision log](../plans/cmux-rebuild/phase-3-interactive-error-recovery/artifacts/implementation-decision-log.md)
