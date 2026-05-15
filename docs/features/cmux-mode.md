# cmux Mode

Phase 1 of the cmux integration: an opt-in launch mode that stands up a recognisably-named cmux workspace with a four-pane scaffold, holds it open while the operator inspects it, and tears it down cleanly with the prior workspace restored — all without running workflow content.

## Activation

Pass `--cmux` on pr9k's run invocation:

```
pr9k --cmux [--project-dir <path>]
```

The flag is visible and experimental. When set, pr9k skips the logger, status-line, and Bubble Tea path entirely and branches into `cmuxctl.Preflight` then `cmuxctl.RunPhase1`.

## Workspace scaffold

Phase 1 creates one cmux workspace with four panes:

| Pane | Visible? | Content |
|---|---|---|
| orchestrator | No (hidden) | `sh -c 'tail -f /dev/null'` |
| header | Yes | `header — Phase 1 placeholder` |
| log | Yes | `log — Phase 1 placeholder` |
| footer | Yes | `footer — Phase 1 placeholder` |

The orchestrator pane is spawned first (D-3) and immediately hidden via `surface.hide`. The three visible panes display static placeholder labels; they carry no live data in Phase 1.

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
       → spawn four panes → wait for dismissal
       → teardown: WorkspaceClose + focus restore → exit
```

**Preflight** (five checks, run sequentially; first failure short-circuits):

1. cmux binary present on PATH
2. Socket path resolves and is a Unix socket (`CMUX_SOCKET_PATH` or `/run/cmux.sock`)
3. Socket is reachable from the launching terminal (descendants-only check)
4. Socket connection is not refused (socket-enabled check)
5. `system.identify` returns `name="cmux"` (capability check)

**Focus excursions** — the operator can switch away from the pr9k workspace and return; the workspace persists until explicitly dismissed via cmux's own controls.

**Dismissal** — the dismissal-observation goroutine polls `workspace.list` and `surface.list` every 500 ms. It fires on either:
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

## Dismissal-gesture coverage

Both dismissal gestures drive the same D-9 dismissal observation:

- **Workspace-close gesture** — closes the pr9k workspace directly; detected via `workspace.list` arm
- **Close-each-pane gesture** — exits individual panes until all are gone; detected via `surface.list` exited-pane arm

Either gesture reaches the same teardown path and results in exit code 0 on success.

## Fatal dismissal escalation

If the dismissal observer sees N=3 consecutive poll timeouts (each call exceeding the 5 s per-call deadline), it fires a fatal dismissal event. pr9k attempts teardown and exits non-zero with a message identifying the workspace name for manual cleanup.

## Out of scope (Phase 1)

- No real workflow execution (no step sequencing, no Claude invocations)
- No `.pr9k/logs/` log artifacts on the cmux path
- No sidebar entries or notifications in the cmux workspace
- No in-workspace dismissal shortcut (operator uses cmux's own controls)
- No automatic orphan cleanup after crash (orphan workspaces have the `pr9k-` prefix and must be dismissed manually via cmux)

## References

- [Feature specification](../plans/cmux-rebuild/phase-1-workspace-lifecycle/feature-specification.md)
- [Feature implementation plan](../plans/cmux-rebuild/phase-1-workspace-lifecycle/feature-implementation-plan.md)
- [cmuxctl code package doc](../code-packages/cmuxctl.md)
- [Setting up cmux how-to](../how-to/setting-up-cmux.md)
- [Implementation decision log](../plans/cmux-rebuild/phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md)
