# Debugging a Run

← [Back to How-To Guides](README.md)

When a workflow does something unexpected — a Claude step generated the wrong code, a capture bound the wrong value, a loop broke early when it shouldn't have — you need to reconstruct what happened. This guide walks through the four places pr9k leaves evidence, and how to use them together.

**Prerequisites**: a working install ([Getting Started](getting-started.md)). Most examples use `jq`; if you want to query the JSONL artifacts you'll want it on your `$PATH`.

For the keyboard map and what the live TUI shows, see [Reading the TUI](reading-the-tui.md). For copying log text out of the TUI to paste into a bug report, see [Copying Log Text](copying-log-text.md).

## The four sources of evidence

| Source | Location | What it tells you |
|--------|----------|-------------------|
| **Log file** | `<project-dir>/.pr9k/logs/ralph-YYYY-MM-DD-HHMMSS.mmm.log` | Every line of subprocess output, every chrome line (phase banners, step banners, capture logs), timestamped |
| **TUI log panel** | In-process | Same content as the log file, live, scrollable, but lost when pr9k exits |
| **JSONL artifacts** | `<project-dir>/.pr9k/logs/<runstamp>/<phase>-<NN>-<slug>.jsonl` | Verbatim NDJSON stream from every claude step — raw turn-by-turn events, token usage, cost, the `result.result` text, and whether `is_error` was set |
| **Iteration log** | `<project-dir>/.pr9k/iteration.jsonl` | One structured record per step: name, status, duration, token counts, and prep-error notes |
| **Handoff files** | `<target-repo>/progress.txt`, `deferred.txt`, `test-plan.md`, `code-review.md` | What Claude steps wrote for the next step; what git thinks the state is |

If pr9k is still running, start with the log panel — scroll back with `↑`/`k`/`↓`/`j` in Normal or Done mode. If pr9k has exited, open the log file from the directory where you ran it.

> **Tip:** Logs land under `<project-dir>/.pr9k/logs/` — that is, inside your **target repo's working directory**. Add `.pr9k/` to the target repo's `.gitignore` before your first run to prevent log files from appearing as untracked changes:
> ```
> echo '.pr9k/' >> .gitignore
> ```

## Reading the log file

The log file is plain text, UTF-8, one line per logical log event. You can use standard tools like `less`, `grep`, `tail -f` (during a run), or `rg` — nothing is binary-encoded.

Each line is timestamp-prefixed by `logger.Log()` and includes the current step name when known:

```
2026-04-10T22:22:31.739 [Get GitHub user] marxriverlynn
2026-04-10T22:22:31.739 [Get GitHub user]
2026-04-10T22:22:31.740 [Get GitHub user] Captured GITHUB_USER = "marxriverlynn"
2026-04-10T22:22:31.740 [Get next issue] 
2026-04-10T22:22:31.740 [Get next issue] Starting step: Get next issue
2026-04-10T22:22:31.740 [Get next issue] ─────────────────────────────
```

For details on the logger format, see [File Logging](../code-packages/logger.md).

## JSONL artifacts for claude steps

Every `isClaude: true` step writes a per-step `.jsonl` file containing the verbatim NDJSON stream emitted by `claude -p --output-format stream-json --verbose`. These files live in a per-run subdirectory alongside the `.log` file:

```
.pr9k/logs/
  ralph-2026-04-14-173022.123.log        # human-readable log (unchanged)
  ralph-2026-04-14-173022.123/           # JSONL artifacts for this run
    initialize-02-get-gh-user.jsonl
    iter01-03-feature-work.jsonl
    iter01-04-test-planning.jsonl
    iter02-03-feature-work.jsonl
    finalize-02-lessons-learned.jsonl
```

Filename format: `<phase-prefix><NN>-<step-slug>.jsonl`. Phase prefixes are `initialize-`, `iter<NN>-` (1-indexed, zero-padded to 2 digits), and `finalize-`. `<NN>` is the step's position within the phase. Only claude steps have `.jsonl` files; non-claude steps write nothing here.

Each line in a `.jsonl` file is one JSON object. The relevant types:

| `type` field | What it contains |
|---|---|
| `system` | Session init (model name, session ID) and API retry notifications |
| `rate_limit_event` | Rate-limit status (usually `"allowed"`) |
| `assistant` | One complete turn: text blocks, tool-use indicators, token counts |
| `user` | Tool results fed back to the model |
| `result` | Final answer: `result` text, `is_error` flag, session ID, total cost, token counts |
| `ralph_end` | Sentinel written by pr9k after the `result` event — its absence means the run was truncated |

### Useful queries

**Find the final captured value for a step:**

```bash
jq 'select(.type == "result") | .result' .pr9k/logs/ralph-2026-04-14-173022.123/iter01-03-feature-work.jsonl
```

**Check whether a step ended in error:**

```bash
jq 'select(.type == "result") | {is_error, result}' .pr9k/logs/ralph-2026-04-14-173022.123/iter01-03-feature-work.jsonl
```

**Check token spend for a step:**

```bash
jq 'select(.type == "result") | {total_cost_usd, usage, num_turns}' .pr9k/logs/ralph-2026-04-14-173022.123/iter01-03-feature-work.jsonl
```

**Verify the step's artifact was written completely (sentinel present):**

```bash
tail -1 .pr9k/logs/ralph-2026-04-14-173022.123/iter01-03-feature-work.jsonl | jq .type
# "ralph_end" → complete; any other output or an empty tail → truncated
```

**Read all assistant turns in order:**

```bash
jq -r 'select(.type == "assistant") | .message.content[] | select(.type == "text") | .text' \
  .pr9k/logs/ralph-2026-04-14-173022.123/iter01-03-feature-work.jsonl
```

> **Retry behavior:** When you press `r` (retry) in error mode, the next attempt overwrites the `.jsonl` file from the beginning. The prior attempt's raw events are lost from the artifact (but the rendered lines remain in the `.log` file, separated by a `(retry)` separator). Token spend from discarded retry attempts is still included in the per-step and run-level summary lines.

## Iteration log (.pr9k/iteration.jsonl)

pr9k writes one JSON record to `<project-dir>/.pr9k/iteration.jsonl` after every step completes, including prep failures. Each record has the form:

```json
{"schema_version":1,"issue_id":"42","iteration_num":1,"step_name":"feature-work","status":"done","duration_s":12.34,"input_tokens":1500,"output_tokens":800,"session_id":"abc-123"}
```

**Fields:**

| Field | Meaning |
|-------|---------|
| `schema_version` | Always `1`. Third-party parsers should reject unknown versions. |
| `issue_id` | The value of `ISSUE_ID` at the time the record was written (empty in initialize/finalize phases). |
| `iteration_num` | Loop iteration index (1-based). `0` for initialize and finalize phases. |
| `step_name` | Step name from `config.json`. |
| `status` | `"done"`, `"failed"`, `"skipped"`, or `"unknown"` (step never started). |
| `duration_s` | Wall-clock seconds from step start to finish. For steps that enter error mode, this includes user idle time. |
| `notes` | Only present on prep failures — contains the `buildStep` error string. |

**Lifecycle:** the file accumulates records across the entire run. The `lessons-learned.md` finalize step truncates it at the end of each run.

**Useful queries** (requires `jq` — see [Prerequisites](#prerequisites) below):

```bash
# Show all step names and statuses for the last run:
jq -r '"- \(.step_name) [\(.status)]"' .pr9k/iteration.jsonl

# Find any failed steps:
jq 'select(.status == "failed")' .pr9k/iteration.jsonl

# Show prep failures with their error details:
jq 'select(.status == "failed" and .notes != null) | {step_name, notes}' .pr9k/iteration.jsonl

# Total token spend across all claude steps:
jq -s '[.[].input_tokens // 0] | add' .pr9k/iteration.jsonl
```

### Prerequisites

The iteration log queries above require `jq`. Install it before running pr9k if you intend to query `.pr9k/iteration.jsonl` directly. The `post_issue_summary` script also uses `jq` to build its GitHub comment body — a missing `jq` binary will cause the "Summarize to issue" step to fail with a bare shell error.

Install on macOS: `brew install jq`. Install on Debian/Ubuntu: `apt-get install jq`.

## Navigating with chrome landmarks

The chrome written by `workflow.Run` and `ui.Orchestrate` gives you navigation landmarks. Use them to jump to the part of the run you care about:

| Search pattern | Finds |
|----------------|-------|
| `Initializing` (with a `═` line right after) | Top of the run |
| `Iterations` (with a `═` line right after) | Top of the iteration loop |
| `── Iteration N ─────────────` | Start of iteration N |
| `Starting step: <name>` | Start of a specific step (in any phase) |
| `Captured VAR = ` | Every capture binding |
| `Finalizing` (with a `═` line right after) | Top of the finalize phase |
| `Ralph completed after` | End of the run |
| `(retry)` | Every retry separator |
| `Error preparing ` | Build-error skips |

For example, to see everything that happened in iteration 3:

```bash
# Range from the "Iteration 3" separator to the next Iteration separator or Finalizing
awk '/── Iteration 3 ─/,/── Iteration 4 ─|^Finalizing$/' .pr9k/logs/ralph-2026-04-10-221950.log
```

Or to find every captured `ISSUE_ID`:

```bash
grep 'Captured ISSUE_ID' .pr9k/logs/ralph-2026-04-10-221950.log
```

For the full log-body rhythm (what all the chrome looks like interleaved with real output), see [Reading the TUI](reading-the-tui.md#the-chrome-rhythm).

## "Why did my step get the wrong value?"

If a step ran with the wrong `{{VAR}}` substitution, the capture log tells you what was bound:

```
Captured ISSUE_ID = "42"
```

Work backwards from the substitution:

1. Find the `Starting step: <downstream step>` that used the wrong value
2. Scroll up from there to find the most recent `Captured <VAR> = ` line for the variable in question
3. That `Captured` line is what the substitution engine saw — if it's not what you expected, the bug is in the capturing step (`scripts/get_next_issue`, `scripts/get_gh_user`, etc.), not the substitution
4. If there's **no** `Captured <VAR> = ` line before the downstream step, the variable was never bound — substitution returns `""` and logs a warning

For the capture semantics (last non-empty stdout line, phase-scoped), see [Capturing Step Output](capturing-step-output.md).

## "Why did the loop break early?"

If `breakLoopIfEmpty` fired when you didn't expect it:

1. Find the last `Starting step: <break-step-name>` before the `Finalizing` banner
2. Check its output in the log — was the last non-empty stdout line actually empty?
3. Look at the `Captured <VAR> = ""` line right after — if the captured value is `""`, the break did what it was supposed to
4. If the step failed (non-zero exit), the break check is **skipped** and you should see an error-mode entry, not a clean break — this is deliberate (see [Breaking Out of the Loop](breaking-out-of-the-loop.md))

## "Why did a step fail?"

When a step fails, the log contains all the output leading up to the failure (stderr is streamed too), the `[✗]` checkbox transition, and — if you retried — a `── <step name> (retry) ─────────────` separator before the next attempt.

To look at a failed step in isolation:

1. Find `Starting step: <step-name>`
2. Read forward until the next `Starting step:` line or `(retry)` separator
3. Everything in between is that attempt's subprocess output
4. If there are multiple retries, each one starts with a new `(retry)` separator

For decision-making guidance on retry vs continue vs quit, see [Recovering from Step Failures](recovering-from-step-failures.md).

## Reproducing a single iteration

If you want to reproduce a bug without running the whole workflow, narrow the scope:

```bash
# Cap at 1 iteration so you only hit the bug once
pr9k -n 1
```

Combined with `--workflow-dir` pointing at an alternate workflow bundle (a scratch directory with a custom `config.json` that only includes the steps leading up to the failure) and `--project-dir` pointing at the target repo you want to reproduce against, you can get a minimal repro in seconds. `--workflow-dir` controls where pr9k looks for `config.json`, `prompts/`, and `scripts/`; `--project-dir` controls the target repository cwd for all subprocesses.

If the bug is inside a specific step's prompt or script, you can also run that step directly:

```bash
# Run a helper script exactly the way pr9k would:
GITHUB_USER=$(scripts/get_gh_user)
scripts/get_next_issue "$GITHUB_USER"
```

Or invoke `claude` with the same command the orchestrator would construct:

```bash
claude --permission-mode bypassPermissions --model sonnet -p "$(cat prompts/feature-work.md | sed 's/{{ISSUE_ID}}/42/g')"
```

## Handoff files

Several Claude steps communicate by writing files into the target repo and having the next step read them with `@filename` syntax:

| File | Written by | Read by | Lifecycle |
|------|-----------|---------|-----------|
| `test-plan.md` | Test planning | Test writing | Deleted by Test writing |
| `code-review.md` | Code review | Fix review items | Deleted by Fix review items |
| `progress.txt` | All Claude steps (append) | Update docs, Lessons learned | Cleared by Lessons learned |
| `deferred.txt` | All Claude steps (append) | Deferred work | Cleared by Deferred work |

These files live in the **target repo's working directory** and are not committed. If a run fails partway through, they may be left behind — a leftover `test-plan.md` means "test writing didn't run or didn't finish", not "here's your plan". Delete them (or let the next run overwrite them) before reproducing.

For the full file-passing model, see [Workflow Variables](workflow-variables.md#file-based-data-passing-between-steps).

## Validator errors before a run

If pr9k refuses to start, it's the validator. `validator.Validate(workflowDir)` runs before the TUI and checks:

- `config.json` exists and parses
- Every step has valid schema (`name`, `isClaude`, required fields per type)
- Every referenced `promptFile` exists in `prompts/`
- Every referenced script path exists in `scripts/` or on `PATH`
- Every `{{VAR}}` reference resolves against the built-in / captured variables in scope for that step's phase

Validation failures print structured errors to stderr before the TUI starts:

```
config.json: step "Feature work": promptFile not found: prompts/feature-work.md
config.json: step "Close issue": command[1] references unresolved variable {{ISSUE_ID}} in finalize phase
```

Fix the underlying config issue and re-run. See [Config Validation](../code-packages/validator.md) for the full validation rules.

## Related documentation

- ← [Back to How-To Guides](README.md)
- [Reading the TUI](reading-the-tui.md) — log-panel layout and chrome rhythm (same content as the log file, live view)
- [Copying Log Text](copying-log-text.md) — grab a failing region for a bug report without leaving the TUI
- [Recovering from Step Failures](recovering-from-step-failures.md) — retry/continue decisions during a live run
- [Capturing Step Output](capturing-step-output.md) — how `captureAs` values get into the log and the variable table
- [Workflow Variables](workflow-variables.md) — substitution rules and file-based data passing
- [Breaking Out of the Loop](breaking-out-of-the-loop.md) — `breakLoopIfEmpty` semantics and how to verify it fired
- [Setting Step Timeouts](setting-step-timeouts.md) — `timed out after Ns` notes in `iteration.jsonl`
- [Resuming Sessions](resuming-sessions.md) — using session IDs in `iteration.jsonl` to verify a resume worked
- [File Logging](../code-packages/logger.md) — log file format, timestamp, context prefix (contributor reference)
- [Stream JSON Pipeline](../code-packages/claudestream.md) — the package that produces the JSONL artifacts
- [Config Validation](../code-packages/validator.md) — validator error format and rules
