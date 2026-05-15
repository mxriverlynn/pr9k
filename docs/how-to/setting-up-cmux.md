# Setting Up cmux for pr9k

This guide covers installing cmux, verifying that pr9k can reach it, and understanding the dismissal gestures you will use to end a cmux-mode session.

> **Tested against:** cmux v0.64.6 (rolling-update policy: newer patch releases of the same minor are expected to work; breaking API changes are tracked under OI-1 in the implementation plan).

## Step 1: Install cmux

cmux is an AI-powered terminal multiplexer developed primarily for macOS.

**macOS (primary platform)**

```
brew install zed-industries/zed/cmux
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

Open a terminal pane inside the cmux session. pr9k's `--cmux` flag requires that the launching terminal is a **descendant** of the cmux session (the default access mode). Launching from outside a cmux session produces:

```
cmuxctl: cmux mode must be launched from inside a cmux session (socket: /run/cmux.sock)
```

## Step 3: Verify cmux's access mode

cmux defaults to **descendants-only** mode: only processes that are children of the cmux session can reach the socket. This is the expected configuration for pr9k.

**Allow-all mode caveat (D-20):** If your cmux is configured with allow-all mode, the not-a-descendant error never fires. pr9k will connect regardless of whether the launching terminal is inside cmux. Whether to use allow-all mode is the operator's policy choice; pr9k does not enforce one mode over the other, but the default descendants-only mode is recommended for security.

You can check which socket path cmux is using:

```
echo $CMUX_SOCKET_PATH
```

If `CMUX_SOCKET_PATH` is unset, pr9k uses the platform default `/run/cmux.sock`. Override it by setting `CMUX_SOCKET_PATH` in your shell environment before running pr9k.

## Step 4: Run pr9k in cmux mode

From inside your cmux session, in the terminal pane where you want pr9k to run:

```
pr9k --cmux --project-dir /path/to/your/repo
```

pr9k runs its standard preflight checks (Docker, Claude profile) and then its cmux-specific preflight (binary, socket, descendant, capability). On success it prints the workspace name and the four-pane scaffold appears in cmux:

```
pr9k workspace: pr9k-myrepo-20260515T123456.000000000Z
```

## Step 5: Dismiss the workspace

When you are done inspecting the workspace, dismiss it using cmux's own controls. Two gestures both work:

**Workspace-close gesture** — closes the pr9k workspace directly. In cmux v0.64.6 this is typically the workspace-level close action in the command palette or the workspace tab's close control. The workspace disappears from cmux's workspace list; pr9k detects this and begins teardown.

**Close-each-pane gesture** — close each visible pane individually (header, log, footer) by exiting the shell in each one (e.g. press `Ctrl+D` in the pane or type `exit`). When any pane transitions to an exited state, pr9k detects this and begins teardown.

Either gesture triggers the same pr9k teardown sequence: `WorkspaceClose`, focus restore to the prior workspace, and exit.

After dismissal, your original workspace regains focus automatically.

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

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `cmux is not installed` | `cmux` binary not on PATH | Install cmux (Step 1) |
| `cmux is installed but not running` | cmux daemon not started | Run `cmux` (Step 2) |
| `must be launched from inside a cmux session` | Launching terminal is not a cmux descendant | Open a terminal pane inside cmux (Step 2) |
| `cmux socket is disabled` | cmux config has socket disabled | Re-enable the socket in cmux's settings |
| `cmux version is incompatible` | `system.identify` returned an unexpected name or error | Update cmux to v0.64.6 or later |
| `CMUX_SOCKET_PATH parent directory ... is world-writable` | Socket parent dir is writable by all users | Correct the directory permissions |

## References

- [cmux mode feature doc](../features/cmux-mode.md)
- [cmuxctl code package doc](../code-packages/cmuxctl.md)
- [Implementation decision log — D-20 (allow-all caveat)](../plans/cmux-rebuild/phase-1-workspace-lifecycle/artifacts/implementation-decision-log.md)
