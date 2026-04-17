# Configuring a Status Line

ralph-tui can display live workflow state in the TUI footer by running a custom script on a schedule. This replaces the default shortcut bar in Normal mode with a single line of text from your script, plus a `? Help` shortcut for the keyboard shortcut modal.

## Prerequisites

- ralph-tui 0.6.0 or later
- A `ralph-steps.json` in your workflow directory

## Step 1 — Add a `statusLine` block to `ralph-steps.json`

Open your `ralph-steps.json` and add a top-level `statusLine` object:

```json
{
  "statusLine": {
    "command": "scripts/statusline"
  },
  "initialize": [ ... ],
  "iteration": [ ... ],
  "finalize": [ ... ]
}
```

The `command` field is required. `type` and `refreshIntervalSeconds` are optional.

## Step 2 — Copy or write a script

The sample script at `scripts/statusline` in the ralph-tui distribution is a minimal starting point:

```bash
#!/usr/bin/env bash
# ralph-tui status line.
# Claude Code-compatible: reads a single JSON object from stdin.
# For MVP this script ignores the input and prints a static line.
cat >/dev/null
echo "testing status line"
```

Copy it into your workflow's `scripts/` directory and make it executable:

```bash
cp /path/to/ralph-tui/scripts/statusline scripts/statusline
chmod +x scripts/statusline
```

The `cat >/dev/null` line is important: it drains stdin so the pipe closes cleanly. Without it ralph-tui's stdin write blocks until the 2-second command timeout fires.

## Step 3 — Read workflow state with `jq`

The script receives a JSON object on stdin. Here is a script that shows the current phase, iteration, and issue ID:

```bash
#!/usr/bin/env bash
input=$(cat)
phase=$(echo "$input" | jq -r '.phase')
iter=$(echo "$input" | jq -r '.iteration')
issue=$(echo "$input" | jq -r '.captures.ISSUE_ID // ""')

if [ -n "$issue" ]; then
    echo "$phase  iter $iter  #$issue"
else
    echo "$phase  iter $iter"
fi
```

Available fields from stdin:

| Field | Example |
|-------|---------|
| `.phase` | `"iteration"` |
| `.iteration` | `2` |
| `.maxIterations` | `5` |
| `.step.name` | `"Feature work"` |
| `.step.num` | `4` |
| `.mode` | `"normal"` |
| `.captures.ISSUE_ID` | `"42"` |
| `.version` | `"0.6.0"` |
| `.workflowDir` | `"/home/user/.local/bin"` |
| `.projectDir` | `"/home/user/myrepo"` |

## Tuning `refreshIntervalSeconds`

By default the script runs every **5 seconds**. Adjust this with the `refreshIntervalSeconds` field:

```json
"statusLine": {
  "command": "scripts/statusline",
  "refreshIntervalSeconds": 10
}
```

Set to `0` to disable the timer and only refresh on workflow events (phase change, step change, mode change):

```json
"statusLine": {
  "command": "scripts/statusline",
  "refreshIntervalSeconds": 0
}
```

## Debugging

ralph-tui logs all status-line activity to the session log file (under `logs/` in your project directory). Lines are prefixed with `[statusline]`:

```
tail -f logs/<timestamp>.log | grep '\[statusline\]'
```

Common entries:

| Log line | Meaning |
|----------|---------|
| `[statusline] stderr: <text>` | Script wrote to stderr |
| `[statusline] stdout truncated at 8 KB` | Output exceeded the 8 KB limit |
| `[statusline] error: exit status 1` | Script exited non-zero |
| `[statusline] error: signal: killed` | Script timed out (2 s) and was killed |

If the footer shows the shortcut bar instead of your script's output, the runner may still be on cold-start (first run hasn't completed yet) or the command path could not be resolved. Check the log for `[statusline]` errors.

## Recovering the shortcut bar

Two ways to see the full keyboard shortcut reference while the status line is active:

1. **Press `?`** — opens the help modal with a per-mode shortcut grid. Press `esc` to close.
2. **Remove `statusLine`** from `ralph-steps.json` and restart — the footer returns to the default shortcut bar permanently.

## Command path resolution

`command` resolves by these rules (same as non-claude step commands):

1. **Contains `/`** — joined with the workflow bundle directory (relative) or used as-is (absolute). The resolved path must exist on disk.
2. **Bare name** — looked up via `PATH` using `exec.LookPath`.

Examples:

```json
"command": "scripts/statusline"      // relative to workflowDir
"command": "/usr/local/bin/mystatus" // absolute
"command": "mystatus"                // resolved via PATH
```

## Security note

The script inherits the full host environment, including `ANTHROPIC_API_KEY`, `GITHUB_TOKEN`, and other secrets present at startup. Treat the script with the same level of trust as the workflow binary itself. The script is **not** sandboxed inside Docker.
