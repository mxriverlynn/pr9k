# Setting Up cmux for pr9k

This guide covers installing cmux, verifying that pr9k can reach it, running a real workflow in cmux mode, and understanding the dismissal gestures you will use to end a cmux-mode session.

> **Tested against:** cmux v0.64.7 (rolling-update policy: newer patch releases of the same minor are expected to work; breaking API changes are tracked under OI-1 in the implementation plan).

## Step 1: Install cmux

cmux is an AI-powered terminal multiplexer developed primarily for macOS.

**macOS (primary platform)**

From [cmux getting-started docs](https://cmux.com/docs/getting-started)
```
brew tap manaflow-ai/cmux
brew install --cask cmux
```

Verify the install:

```
cmux --version
```

**Linux / Windows**

Community ports exist but are not officially supported by the cmux project. Check the cmux project's releases page for the latest community builds. All five pr9k preflight checks run on every platform; if cmux is present, reachable, and responds to `system.identify`, pr9k will proceed.

## Step 2: Start cmux and launch a terminal session inside it

cmux must be running before you invoke pr9k with `--cmux`. Start it normally:

```
cmux
```

Open a terminal pane inside the cmux session and run `pr9k --cmux` **from that pane**. pr9k's `--cmux` flag requires that the launching terminal is a **descendant** of the cmux session (the default access mode); a pane inside cmux satisfies this and cmux exports `CMUX_SOCKET_PATH` into that pane's environment so pr9k connects automatically. pr9k does **not** start cmux for you — cmux is an interactive terminal multiplexer, and a pr9k-launched cmux would not be pr9k's ancestor, so the descendants-only check would still reject it.

Launching from outside a cmux session produces (the socket path shown is whichever path pr9k resolved — see Step 3):

```
cmuxctl: cmux mode must be launched from inside a cmux session (socket: ~/Library/Application Support/cmux/cmux.sock)
```

## Step 3: Verify cmux's access mode

cmux defaults to **descendants-only** mode: only processes that are children of the cmux session can reach the socket. This is the expected configuration for pr9k.

**Allow-all mode caveat (D-20):** If your cmux is configured with allow-all mode, the not-a-descendant error never fires. pr9k will connect regardless of whether the launching terminal is inside cmux. Whether to use allow-all mode is the operator's policy choice; pr9k does not enforce one mode over the other, but the default descendants-only mode is recommended for security.

You can check which socket path cmux is using:

```
echo $CMUX_SOCKET_PATH
```

pr9k resolves the cmux socket exactly the way cmux's own CLI does, in this order:

1. `CMUX_SOCKET_PATH` (cmux's canonical override) — used verbatim if set.
2. `CMUX_SOCKET` (cmux's deprecated alias) — used verbatim if set.
3. cmux's `last-socket-path` marker file — `~/Library/Application Support/cmux/last-socket-path` on macOS (`~/.config/cmux/last-socket-path` on Linux), then the `/tmp/cmux-last-socket-path` mirror. Its contents are the live socket path.
4. The stable default: `~/Library/Application Support/cmux/cmux.sock` on macOS, `~/.config/cmux/cmux.sock` on Linux.
5. The legacy default `/tmp/cmux.sock` (note: rejected on macOS by the world-writable-parent check, since `/tmp` is `0777` — set `CMUX_SOCKET_PATH` if you genuinely need a socket there).

When you run `pr9k --cmux` from inside a cmux pane (the supported flow), cmux has already exported `CMUX_SOCKET_PATH`, so step 1 resolves it. The remaining steps are why pr9k works without any manual configuration. Earlier pr9k builds hardcoded `/run/cmux.sock` (a Linux-only path that does not exist on macOS); if you saw `cmux ... not running` on every run, upgrade.

## Step 4: Run pr9k in cmux mode against a real workflow

From inside your cmux session, in the terminal pane where you want pr9k to run:

```
pr9k --cmux --project-dir /path/to/your/repo
```

pr9k runs its standard preflight checks (Docker, Claude profile) and then its cmux-specific preflight (binary, socket, descendant, capability). On success it prints the workspace label and a three-pane scaffold appears in cmux:

```
pr9k workspace: pr9k-myrepo-20260515T123456.000000000Z
```

The three panes (log, header, footer) start immediately and connect back to the orchestrator — the pr9k process you launched in this pane; there is no separate hidden orchestrator pane (Rework R / Architecture A) — over a Unix-socket interaction channel. Once all three have completed the readiness handshake (up to 10 seconds), the workflow begins.

### What each pane shows

| Pane | What you see |
|---|---|
| **Header** | Step-checkbox grid (one checkbox per step) and iteration counter, ticking over as steps complete |
| **Log** | Streaming subprocess output — the same output you would see in standard mode's log panel |
| **Footer** | Status-line output on its configured cadence, shortcut hints, and the pr9k version label |

### Navigating the panes

Focus the log pane in cmux to scroll back through output. Scrolling controls are your terminal emulator's standard scroll controls inside the focused pane. The log pane uses a viewport; all buffered lines are accessible.

### Expanding inline help

Press `?` in the footer pane to expand the shortcut reference inline above the footer row. The expansion is bounded to the available pane height. Press `Esc` to collapse it.

### Quitting cleanly

Press `q` in the footer pane. pr9k enters quit-confirmation mode; the footer shows a confirmation prompt. Press `y` to confirm. The orchestrator aborts the workflow cleanly, broadcasts a final state to each pane, and waits for acknowledgement. The workspace remains open in cmux showing the final state on each pane. Dismiss the workspace (Step 5) to return to your prior workspace.

Press `Esc` after `q` to cancel quit without effect.

> **Note:** Closing a pr9k pane (the header, log, or footer pane) while a run is in progress aborts the run. pr9k detects the display loss, terminates the running step, and broadcasts `run aborted` to the remaining panes. Each pane renders a final `run aborted` line and exits. The workspace stays open in cmux for inspection; dismiss it manually when done.

### Recovering from a failed step

When a workflow step exits non-zero and triggers pr9k's error mode, all three visible panes signal the failure near-simultaneously:

| Pane | What you see |
|---|---|
| **Header** | The failing step's checkbox changes to `[✗]` |
| **Log** | The step's error output appears as ordinary streamed content |
| **Footer** | The shortcut bar switches to the error-mode hints: continue / retry / quit |

Focus the footer pane and choose one of three actions:

- **`c` — Continue** — advance past the failed step and resume the workflow.
- **`r` — Retry** — re-run the same step. On retry, the log shows a separator line (`── <step name> (retry) ─────────────`) before any new output. A retry that fails again re-enters error mode; there is no retry-count cap and no auto-timeout — the run stays paused until you decide.
- **`q` — Quit** — enter the two-step quit confirmation. Press `y` to abort the run or `Esc`/`n` to cancel and return to the error prompt.

Control keys pressed while the header or log pane is focused are absorbed silently — those panes are read-only. If you are focused on a different pane or workspace when a step fails, the orchestrator blocks indefinitely and the footer shows the error prompt; pr9k fires an error-mode notification (see "Lifecycle notifications" below) that re-fires every 60 seconds to remind you. Switch to the footer pane when you are ready to respond.

### Monitoring from another workspace

You can switch to a different cmux workspace while pr9k is running and still track progress. In cmux's workspace list, the pr9k workspace row shows:

- **Status pill** — the current step name (e.g. `feature work`), updated on every step transition. The pill uses the key `pr9k.step`.
- **Progress bar** — if the run was launched with `-n M` (a bounded iteration count), the bar shows `<completed> / <total>` and updates on each iteration. The bar is not shown for unbounded runs.

When a step fails and pr9k enters error mode, the status pill changes to:

```
<step name> — awaiting input
```

(U+2014 em-dash). This signals that the run is paused and waiting for input in the footer pane.

When the workflow finishes cleanly, pr9k clears the status pill and progress bar from the workspace row.

## Step 5: Dismiss the workspace

When you are done inspecting the workspace, dismiss it using cmux's own controls. Two gestures both work:

**Workspace-close gesture** — closes the pr9k workspace directly. In cmux v0.64.7 this is typically the workspace-level close action in the command palette or the workspace tab's close control. The workspace disappears from cmux's workspace list; pr9k detects this and begins teardown.

**Close-each-pane gesture** — close each visible pane individually (header, log, footer) by exiting the shell in each one (e.g. press `Ctrl+D` in the pane or type `exit`). When any pane transitions to an exited state, pr9k detects this and begins teardown.

Either gesture triggers the same pr9k teardown sequence: `WorkspaceClose`, focus restore to the prior workspace, and exit.

After dismissal, your original workspace regains focus automatically.

## Step 6: Verify log artifacts (optional)

After a cmux-mode run, pr9k writes the same per-step JSONL artifacts as standard mode under:

```
<projectDir>/.pr9k/logs/<run-stamp>/
```

To confirm cmux mode produced equivalent output to a standard-mode run on the same workflow and inputs, compare the per-step `.jsonl` files. The content should match modulo run-specific fields (timestamps, run-stamp paths, width-dependent renders). This equivalence check is described in the Phase 2 implementation decision log at D-13.

## Orphan workspace recognition

If pr9k crashes before dismissal, the workspace remains in cmux. Orphan pr9k workspaces are identifiable by their `pr9k-` prefix in cmux's workspace list:

```
pr9k-myrepo-20260515T123456.000000000Z
```

Dismiss orphan workspaces manually using cmux's workspace-close gesture. pr9k does not auto-clean orphans.

pr9k also prints an advisory to stderr when `WorkspaceClose` fails during normal teardown:

```
pr9k: orphan workspace "pr9k-myrepo-..." could not be closed; dismiss it manually via cmux's controls
```

## Diagnostics: `PR9K_CMUX_DEBUG`

Set `PR9K_CMUX_DEBUG=1` to enable verbose cmux-integration diagnostics:

- The orchestrator prints `[pr9k-cmux-debug]` lines to stderr covering the
  `Serve` → `workspace.create` → `surface.split` → `hooks.Run` timeline and the
  dismissal observer's per-poll verdict (healthy / which arm fired / what cmux
  returned).
- Each display pane (`pr9k cmux-pane --role=…`) appends entry- and exit-event
  markers to `<projectDir>/.pr9k/pane-probe.log` (with timestamp, role, pid,
  socket path, args). The file outlives the cmux pane so it survives a pane
  that disappears unexpectedly.

Both are gated on the env var; with it unset the runtime is silent.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `cmux is not installed` | `cmux` binary not on PATH | Install cmux (Step 1) |
| `cmux socket not found at <path> (looked in: ...)` | No cmux socket at any resolved location — cmux is not running, or you are not in a cmux pane so `CMUX_SOCKET_PATH` is unset and no marker/default socket exists | Start `cmux` (Step 2), then run `pr9k --cmux` **from inside a cmux pane**; or set `CMUX_SOCKET_PATH` |
| `must be launched from inside a cmux session` | cmux is running but the launching terminal is not a cmux descendant (default descendants-only mode) | Open a terminal pane inside cmux and run pr9k there (Step 2) |
| `cmux socket is disabled` | cmux config has socket disabled | Re-enable the socket in cmux's settings |
| `cmux denied access — run pr9k from a terminal pane inside the cmux session` | cmux is in its default `cmuxOnly` mode and pr9k is not a cmux descendant | Launch pr9k from a terminal pane *inside* cmux, or set cmux's socket control mode to allow-all |
| `cmux socket requires authentication` | cmux socket password mode is enabled | Set `CMUX_SOCKET_PASSWORD` or configure the socket password in cmux Settings |
| `unexpected cmux identify response (no socket_path)` | cmux is an unsupported version | Use the pinned cmux v0.64.7 |
| `cmux socket parent directory ... is world-writable` | Socket parent dir is writable by all users (e.g. a socket placed directly in `/tmp`) | Correct the directory permissions, or point `CMUX_SOCKET_PATH` at a socket whose parent is not world-writable |
| `cmux build does not expose required notification method WorkspaceNotify` | Your cmux build predates the `notification.create_for_target` wire method (Phase 5 preflight probe) | Upgrade cmux; Phase 5 requires at least cmux 0.64.7 with commit `2f96c15c2` |
| `ready timeout: missing roles: ...` | A display pane sub-process failed to start or connect within 10 seconds | Check that Docker is running and the `pr9k cmux-pane` sub-command is on PATH; re-run |
| Pane shows "orchestrator unavailable — dismiss the workspace" | Orchestrator process exited before `WorkspaceDone` was sent | Check the `.pr9k/logs/` directory for the last run's error output; dismiss the workspace and re-run |

## Lifecycle notifications

When a step fails and pr9k enters error mode, cmux displays a notification:

> `<step name> failed in <repo> — Focus the footer pane to respond`

The notification re-fires every **60 seconds** until you respond, even if you dismiss it. This is intentional — pr9k keeps the interrupt signal active until you act.

**To stop the re-fire cadence**, focus the footer pane and press one of:
- `c` — continue (ignore the error, proceed to the next step)
- `r` — retry (re-run the failed step)
- `q` then `y` — quit the run

Pressing `q` stops the cadence immediately (even before pressing `y`). If you press `q` and then cancel with `n` or `esc`, the cadence restarts from 0.

When the workflow completes normally, a single completion notification appears:
> `pr9k run completed in <repo>`

If the run is aborted (quit), a single run-aborted notification appears:
> `pr9k run aborted in <repo>`

Both one-shot notifications fire before the workspace begins its close sequence.

## References

- [cmux mode feature doc](../features/cmux-mode.md)
- [cmuxctl code package doc](../code-packages/cmuxctl.md)
- [interactionchannel code package doc](../code-packages/interactionchannel.md)
- [Implementation decision log — D-20 (allow-all caveat)](../plans/cmux-rebuild/phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md)
- [Phase 2 implementation decision log — D-13 (log artifact equivalence)](../plans/cmux-rebuild/phase-2-real-workflow-runs/artifacts/implementation-decision-log.md)
