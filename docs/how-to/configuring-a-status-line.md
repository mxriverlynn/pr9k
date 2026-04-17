# Configuring a Status Line

ralph-tui can display live workflow state in the TUI footer by running a custom script on a schedule. This replaces the default shortcut bar in Normal mode with a single line of text from your script, plus a `? Help` shortcut for the keyboard shortcut modal.

## Prerequisites

- ralph-tui 0.6.0 or later
- A `ralph-steps.json` in your workflow directory
- [`jq`](https://jqlang.github.io/jq/) — required by the sample script to parse stdin JSON
- `git` (optional) — used by the sample script to display the current branch

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

The sample script at `scripts/statusline` in the ralph-tui distribution reads ralph-tui's JSON payload and prints the current phase, iteration, step name, and issue ID with ANSI color:

```bash
#!/usr/bin/env bash
# ralph-tui status line — demo script, adapt for your needs.
# Reads ralph-tui's JSON payload from stdin and prints a single status line.
# Requires: bash 3.1+, jq; git is used when available for branch display.

command -v jq >/dev/null || { printf 'statusline: jq is required\n' >&2; exit 1; }

input=$(cat)

PHASE=$(echo "$input" | jq -r '.phase // "unknown"')
ITER=$(echo "$input" | jq -r '.iteration // 0')
MAX=$(echo "$input" | jq -r '.maxIterations // 0')
STEP=$(echo "$input" | jq -r '.step.name // ""')
ISSUE=$(echo "$input" | jq -r '.captures.ISSUE_ID // ""')

CYAN='\033[36m'; YELLOW='\033[33m'; RESET='\033[0m'

BRANCH=""
git rev-parse --git-dir > /dev/null 2>&1 && BRANCH=" | 🌿 $(git branch --show-current 2>/dev/null)"

ITER_LABEL=""
if [ "$MAX" -gt 0 ] 2>/dev/null; then ITER_LABEL=" ${ITER}/${MAX}"
elif [ "$ITER" -gt 0 ] 2>/dev/null; then ITER_LABEL=" ${ITER}"; fi

EXTRAS=""
[ -n "$STEP" ] && EXTRAS="${EXTRAS} › ${STEP}"
[ -n "$ISSUE" ] && EXTRAS="${EXTRAS} | ${YELLOW}#${ISSUE}${RESET}"

line="${CYAN}${PHASE}${ITER_LABEL}${RESET}${EXTRAS}${BRANCH}"
printf '%b\n' "$line"
```

Copy it into your workflow's `scripts/` directory and make it executable:

```bash
cp /path/to/ralph-tui/scripts/statusline scripts/statusline
chmod +x scripts/statusline
```

The script uses `input=$(cat)` to drain stdin before processing, which is required — if the script exits without reading, ralph-tui's stdin write blocks until the 2-second command timeout fires.

## Step 3 — Available stdin fields

The script receives a JSON object on stdin with the following fields:

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

## Related documentation

- [Status Line Feature](../features/status-line.md) — Full feature reference: config schema, stdin JSON field table, stdout rules, refresh trigger matrix, help modal, concurrency model, and lifecycle
- [Status Line Package](../code-packages/statusline.md) — Package-level reference: `Runner` API, `State`, `BuildPayload`, `Sanitize`, and shutdown ordering
- [Reading the TUI](reading-the-tui.md) — Status-line footer display, `? Help` trigger, and help modal walkthrough
- [Config Validation](../code-packages/validator.md) — Validation rules applied to the `statusLine` block at startup
