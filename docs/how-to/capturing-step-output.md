# Capturing Step Output

This guide shows how to use `captureAs` to bind a step's stdout to a variable so later steps can reference it via `{{VAR_NAME}}` substitution. This is the primary way to pass values — like a GitHub username, an issue ID, or a commit SHA — from a step that produces them to the steps that need them.

If you're looking for how `{{VAR}}` tokens get *resolved* against the VarTable, see [Workflow Variables](workflow-variables.md). This guide is about the other direction: getting values *into* the VarTable.

## The basic shape

Every step in `config.json` accepts an optional `captureAs` field. When set, pr9k calls `runner.LastCapture()` after the step completes and stores the result in the VarTable under that name.

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

The captured value depends on the step type.

### Non-claude steps (`isClaude: false`)

By default (and when `captureMode` is `"lastLine"` or absent), `LastCapture()` returns the **last non-empty stdout line** from the most recent step, with leading/trailing whitespace trimmed. Specifically:

- Only `stdout` is captured — `stderr` is discarded for this purpose (but still streamed to the TUI log)
- Only the **last** non-empty line is kept — earlier output is visible in the log but not available as the capture value
- If every line is empty (or the step only wrote to stderr), the capture value is the empty string `""`
- If the step fails (non-zero exit or user-terminated), the capture value is reset to `""`

This means you should design capture scripts to print the captured value **last** — for example, a script that prints debug info to stderr and the result to stdout. Don't print the result first and then print progress messages.

#### Capturing multi-line output with `captureMode: "fullStdout"`

When a step emits a multi-line payload — such as a GitHub issue body or a git diff — set `captureMode` to `"fullStdout"`:

```json
{
  "name": "Get issue body",
  "isClaude": false,
  "command": ["gh", "issue", "view", "{{ISSUE_ID}}", "--json", "title,body", "-t", "{{{{.title}}}}\n\n{{{{.body}}}}"],
  "captureAs": "ISSUE_BODY",
  "captureMode": "fullStdout"
}
```

Note the `{{{{.title}}}}` escape: ralph's variable substitution runs first on command arguments, so any literal `{{` that should reach the subprocess must be written as `{{{{` (ralph's escape for a literal `{{`). Also note that the `\n` sequences in the JSON string are real newlines by the time `gh` receives the argument — JSON parsing occurs before ralph sees the value. This step captures the full issue title and body into `{{ISSUE_BODY}}` for later steps.

The default workflow also captures a project card and post-feature diff the same way:

```json
{ "name": "Get project card", "isClaude": false,
  "command": ["scripts/project_card"],
  "captureAs": "PROJECT_CARD", "captureMode": "fullStdout" },
{ "name": "Get post-feature diff", "isClaude": false,
  "command": ["git", "diff", "{{STARTING_SHA}}..HEAD", "--stat"],
  "captureAs": "PRE_REVIEW_DIFF", "captureMode": "fullStdout" }
```

> **Note:** `Get post-feature diff` compares `{{STARTING_SHA}}` against `HEAD`, so it is only meaningful after the Feature work step has committed its changes. If Feature work only stages or modifies files without committing, `HEAD` still points to `STARTING_SHA` and the diff will be empty.

With `captureMode: "fullStdout"`, all stdout lines are joined with `"\n"` and bound to the variable. A hard cap of **32 KiB** applies: if the joined content exceeds 32 KiB, the first 30 KiB are kept verbatim and the following marker is appended:

```
[...truncated, full content exceeds 32 KiB]
```

Valid `captureMode` values: `""` (or absent), `"lastLine"`, `"fullStdout"`. Any other value is rejected at config-load time by the validator. Setting `captureMode` on a claude step is also rejected — claude steps always use the `claudestream` aggregator path.

### Claude steps (`isClaude: true`)

For claude steps, `captureAs` binds to **`result.result`** — the authoritative final-answer text from the `ResultEvent` in the `claude -p --output-format stream-json --verbose` output stream. The raw JSON on stdout is never meaningful to bind; the `claudestream.Aggregator` parses the stream and extracts `result.result` for you.

This is the value that appears in the TUI and log as the last substantive assistant message (after the per-step summary line). If the step fails (`is_error: true` or no result event emitted), the capture value is reset to `""`.

Design your prompts so that `result.result` contains the value you want to capture. If claude emits multi-paragraph prose, the full text is captured — use downstream steps to extract the piece you need.

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

When `get_next_issue` prints an empty line (no more issues), `LastCapture()` is `""`, pr9k marks the remaining iteration steps as skipped (`[-]`), and exits the iteration loop. Finalization still runs. See [Breaking Out of the Loop](breaking-out-of-the-loop.md) for details.

## Related documentation

- [Workflow Variables](workflow-variables.md) — How `{{VAR}}` tokens are resolved from the VarTable into prompts and commands
- [Breaking Out of the Loop](breaking-out-of-the-loop.md) — Using `breakLoopIfEmpty` to exit when the capture is empty
- [Building Custom Workflows](building-custom-workflows.md) — Full step schema and workflow structure
- [Debugging a Run](debugging-a-run.md) — Reading capture logs in the log file
- [Variable State Management](../code-packages/vars.md) — VarTable internals: scopes, phase transitions, binding semantics, and the difference between non-claude and claude captureAs binding
- [Subprocess Execution & Streaming](../features/subprocess-execution.md) — How `LastCapture()` extracts the last non-empty stdout line (non-claude) or `result.result` (claude)
- [Workflow Orchestration](../features/workflow-orchestration.md) — Where in the Run loop the bind happens
