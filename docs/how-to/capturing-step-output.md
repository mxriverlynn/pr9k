# Capturing Step Output

This guide shows how to use `captureAs` to bind a step's stdout to a variable so later steps can reference it via `{{VAR_NAME}}` substitution. This is the primary way to pass values — like a GitHub username, an issue ID, or a commit SHA — from a step that produces them to the steps that need them.

If you're looking for how `{{VAR}}` tokens get *resolved* against the VarTable, see [Variable Output & Injection](variable-output-and-injection.md). This guide is about the other direction: getting values *into* the VarTable.

## The basic shape

Every step in `ralph-steps.json` accepts an optional `captureAs` field. When set, ralph-tui calls `runner.LastCapture()` after the step completes and stores the result in the VarTable under that name.

```json
{
  "name": "Get next issue",
  "isClaude": false,
  "command": ["scripts/get_next_issue", "{{GITHUB_USER}}"],
  "captureAs": "ISSUE_ID"
}
```

After this step runs, `{{ISSUE_ID}}` is bound to the trimmed last non-empty stdout line — whatever `scripts/get_next_issue` printed last. All subsequent steps in the same iteration (and, for initialize steps, all subsequent phases) can reference it.

## What gets captured

`LastCapture()` returns the **last non-empty stdout line** from the most recent step, with leading/trailing whitespace trimmed. Specifically:

- Only `stdout` is captured — `stderr` is discarded for this purpose (but still streamed to the TUI log)
- Only the **last** non-empty line is kept — earlier output is visible in the log but not available as the capture value
- If every line is empty (or the step only wrote to stderr), the capture value is the empty string `""`
- If the step fails (non-zero exit or user-terminated), the capture value is reset to `""`

This means you should design capture scripts to print the captured value **last** — for example, a script that prints debug info to stderr and the result to stdout. Don't print the result first and then print progress messages.

### Good capture script

```bash
#!/bin/bash
# scripts/get_next_issue
set -euo pipefail
username="$1"
echo "fetching issues for $username..." >&2    # progress → stderr (visible in log)
issue=$(gh issue list --assignee "$username" --label ralph --json number --jq '.[0].number // ""')
echo "$issue"                                   # result → stdout (last line)
```

### Bad capture script

```bash
#!/bin/bash
# DON'T do this — the result is captured, but then "done." overwrites it
set -euo pipefail
gh issue list --assignee "$1" --label ralph --json number --jq '.[0].number // ""'
echo "done."   # ← this is now what gets captured, not the issue number
```

## Scoping: initialize vs iteration

The captured value is stored in the VarTable scope that matches the **active phase** at the time of capture:

| Captured during | Scope | Visible to |
|-----------------|-------|------------|
| Initialize phase | Persistent | Initialize, Iteration, and Finalize phases |
| Iteration phase | Iteration | Only the current iteration (cleared at the start of each new iteration) |
| Finalize phase | Not supported — `captureAs` in finalize steps is ignored by `workflow.Run` |

So if you capture `GITHUB_USER` during initialize, it's available forever. If you capture `ISSUE_ID` during iteration, it's cleared at the start of iteration 2 so the next iteration's `get_next_issue` can rebind it.

### Typical initialize-phase captures

```json
{
  "initialize": [
    {"name": "Splash", "isClaude": false, "command": ["scripts/box-text", "@ralph-art.txt"]},
    {"name": "Get GitHub user", "isClaude": false, "command": ["scripts/get_gh_user"], "captureAs": "GITHUB_USER"}
  ]
}
```

`GITHUB_USER` is now available in iteration and finalize steps as `{{GITHUB_USER}}`.

### Typical iteration-phase captures

```json
{
  "iteration": [
    {"name": "Get next issue", "isClaude": false, "command": ["scripts/get_next_issue", "{{GITHUB_USER}}"], "captureAs": "ISSUE_ID", "breakLoopIfEmpty": true},
    {"name": "Feature work", "model": "sonnet", "promptFile": "feature-work.md", "isClaude": true}
  ]
}
```

`{{ISSUE_ID}}` is bound after "Get next issue" and immediately available in `feature-work.md`'s prompt.

## Seeing the captured value in the log

Every `captureAs` step writes a `Captured VAR = "value"` line into the log body after the subprocess output. So the log for a successful capture looks like:

```
Starting step: Get next issue
─────────────────────────────

[get_next_issue step output — fetching, network timing, etc.]
42

Captured ISSUE_ID = "42"
```

The value is rendered with Go's `%q` formatter, which escapes newlines and control characters so multi-line or whitespace-heavy captures stay readable on a single log line. If the capture was empty, you'll see `Captured VAR = ""`.

This is a great debugging aid — if the wrong value is being substituted into a downstream step, scan the log for the capture line and see what was actually bound. See [Debugging a Run](debugging-a-run.md) for more on reading the log.

## Using the captured value

Once captured, reference the variable in any later step's command or prompt:

```json
{"name": "Close issue", "isClaude": false, "command": ["scripts/close_gh_issue", "{{ISSUE_ID}}"]}
```

Or inside a prompt file (`prompts/feature-work.md`):

```
Implement github issue {{ISSUE_ID}} on branch issue-{{ISSUE_ID}}.
When done, commit with message "Close #{{ISSUE_ID}}".
```

The substitution happens in `workflow.ResolveCommand` (for shell commands) and `steps.BuildPrompt` (for Claude prompts). Unresolved variables substitute to the empty string and log a warning.

## Interaction with `breakLoopIfEmpty`

A common pattern pairs `captureAs` with `breakLoopIfEmpty: true` so the loop exits cleanly when there's nothing left to process:

```json
{
  "name": "Get next issue",
  "isClaude": false,
  "command": ["scripts/get_next_issue", "{{GITHUB_USER}}"],
  "captureAs": "ISSUE_ID",
  "breakLoopIfEmpty": true
}
```

When `get_next_issue` prints an empty line (no more issues), `LastCapture()` is `""`, ralph-tui marks the remaining iteration steps as skipped (`[-]`), and exits the iteration loop. Finalization still runs. See [Breaking Out of the Loop](breaking-out-of-the-loop.md) for details.

## Related documentation

- [Variable Output & Injection](variable-output-and-injection.md) — How `{{VAR}}` tokens are resolved from the VarTable into prompts and commands
- [Breaking Out of the Loop](breaking-out-of-the-loop.md) — Using `breakLoopIfEmpty` to exit when the capture is empty
- [Building Custom Workflows](building-custom-workflows.md) — Full step schema and workflow structure
- [Debugging a Run](debugging-a-run.md) — Reading capture logs in the log file
- [Variable State Management](../features/variable-state.md) — VarTable internals: scopes, phase transitions, binding semantics
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — How `LastCapture()` extracts the last non-empty stdout line
- [Workflow Orchestration](../features/workflow-orchestration.md) — Where in the Run loop the bind happens
