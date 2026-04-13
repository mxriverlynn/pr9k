# Debugging a Run

When a workflow does something unexpected — a Claude step generated the wrong code, a capture bound the wrong value, a loop broke early when it shouldn't have — you need to reconstruct what happened. This guide walks through the three places ralph-tui leaves evidence, and how to use them together.

## The three sources of evidence

| Source | Location | What it tells you |
|--------|----------|-------------------|
| **Log file** | `<project-dir>/logs/ralph-YYYY-MM-DD-HHMMSS.log` | Every line of subprocess output, every chrome line (phase banners, step banners, capture logs), timestamped |
| **TUI log panel** | In-process | Same content as the log file, live, scrollable, but lost when ralph-tui exits |
| **Handoff files** | `<target-repo>/progress.txt`, `deferred.txt`, `test-plan.md`, `code-review.md` | What Claude steps wrote for the next step; what git thinks the state is |

If ralph-tui is still running, start with the log panel — scroll back with `↑`/`k`/`↓`/`j` in Normal or Done mode. If ralph-tui has exited, open the log file from the directory where you ran it.

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

For details on the logger format, see [File Logging](../features/file-logging.md).

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
awk '/── Iteration 3 ─/,/── Iteration 4 ─|^Finalizing$/' logs/ralph-2026-04-10-221950.log
```

Or to find every captured `ISSUE_ID`:

```bash
grep 'Captured ISSUE_ID' logs/ralph-2026-04-10-221950.log
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
ralph-tui -n 1
```

Combined with a specific `--project-dir` pointing at a scratch ralph-tui checkout with a custom `ralph-steps.json` that only includes the steps leading up to the failure, you can get a minimal repro in seconds.

If the bug is inside a specific step's prompt or script, you can also run that step directly:

```bash
# Run a helper script exactly the way ralph-tui would:
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
| `code-review.md` | Code review | Review fixes | Deleted by Review fixes |
| `progress.txt` | All Claude steps (append) | Update docs, Lessons learned | Cleared by Lessons learned |
| `deferred.txt` | All Claude steps (append) | Deferred work | Cleared by Deferred work |

These files live in the **target repo's working directory** and are not committed. If a run fails partway through, they may be left behind — a leftover `test-plan.md` means "test writing didn't run or didn't finish", not "here's your plan". Delete them (or let the next run overwrite them) before reproducing.

For the full file-passing model, see [Variable Output & Injection](variable-output-and-injection.md#file-based-data-passing-between-steps).

## Validator errors before a run

If ralph-tui refuses to start, it's the validator. `validator.Validate(workflowDir)` runs before the TUI and checks:

- `ralph-steps.json` exists and parses
- Every step has valid schema (`name`, `isClaude`, required fields per type)
- Every referenced `promptFile` exists in `prompts/`
- Every referenced script path exists in `scripts/` or on `PATH`
- Every `{{VAR}}` reference resolves against the built-in / captured variables in scope for that step's phase

Validation failures print structured errors to stderr before the TUI starts:

```
ralph-steps.json: step "Feature work": promptFile not found: prompts/feature-work.md
ralph-steps.json: step "Close issue": command[1] references unresolved variable {{ISSUE_ID}} in finalize phase
```

Fix the underlying config issue and re-run. See [Config Validation](../features/config-validation.md) for the full validation rules.

## Related documentation

- [Reading the TUI](reading-the-tui.md) — Log-panel layout and chrome rhythm (same content as the log file, live view)
- [Capturing Step Output](capturing-step-output.md) — How `captureAs` values get into the log and the VarTable
- [Variable Output & Injection](variable-output-and-injection.md) — Substitution rules and file-based data passing
- [Breaking Out of the Loop](breaking-out-of-the-loop.md) — `breakLoopIfEmpty` semantics and how to verify it fired
- [Recovering from Step Failures](recovering-from-step-failures.md) — Retry/continue decisions during a live run
- [File Logging](../features/file-logging.md) — Log file format, timestamp, context prefix
- [Config Validation](../features/config-validation.md) — Validator error format and rules
- [Workflow Orchestration](../features/workflow-orchestration.md) — Where build errors get logged
