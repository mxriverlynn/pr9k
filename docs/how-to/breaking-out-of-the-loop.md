# Breaking Out of the Loop

pr9k's iteration loop runs until one of two conditions is true: it hits the `--iterations N` cap, or a step with `breakLoopIfEmpty: true` returns an empty capture. This guide explains the `breakLoopIfEmpty` pattern — when to use it, how it interacts with the rest of the workflow, and what it looks like at runtime.

## When you want it

Use `breakLoopIfEmpty` when the iteration loop should keep running **as long as there's more work to pick up**. The default workflow uses it on the "Get next issue" step so ralph keeps processing GitHub issues until there are none left — without you having to guess how many to cap at.

Common scenarios:

- **Issue queue drain** — "Keep implementing ralph-labeled issues until there are none left"
- **Task list drain** — "Keep picking items off a work queue until the queue is empty"
- **File-driven batch** — "Keep processing files from an input directory until the directory is empty"

If you know exactly how many iterations you want, pass `-n N` and skip `breakLoopIfEmpty` entirely.

## How it works

Mark a step with both `captureAs` and `breakLoopIfEmpty: true`:

```json
{
  "name": "Get next issue",
  "isClaude": false,
  "command": ["scripts/get_next_issue", "{{GITHUB_USER}}"],
  "captureAs": "ISSUE_ID",
  "breakLoopIfEmpty": true
}
```

At runtime, after the step completes, pr9k checks three conditions:

1. The step has `breakLoopIfEmpty: true`
2. `executor.LastCapture()` is empty (the trimmed last non-empty stdout line is `""`)
3. The step finished as `StepDone` — it did **not** return a non-zero exit code

If all three are true, pr9k:

- Marks every remaining step in the current iteration as `[-]` (skipped) in the TUI header
- Exits the iteration loop
- **Still runs the finalization phase** — finalize steps always run, even on an early break

If the step **failed** (non-zero exit), the break check is skipped so the normal error-mode recovery takes effect instead. This matters because a network hiccup calling `gh` shouldn't look like "no more issues" — the user should see the error and decide whether to retry.

## Typical placement

Put the break step **first** in the iteration array, so an empty result short-circuits all the other work:

```json
{
  "iteration": [
    {"name": "Get next issue", "isClaude": false, "command": ["scripts/get_next_issue", "{{GITHUB_USER}}"], "captureAs": "ISSUE_ID", "breakLoopIfEmpty": true},
    {"name": "Feature work", "model": "sonnet", "promptFile": "feature-work.md", "isClaude": true},
    {"name": "Run tests",    "isClaude": false, "command": ["scripts/run_tests"]},
    {"name": "Git push",     "isClaude": false, "command": ["git", "push"]}
  ]
}
```

On the iteration where there are no more issues:

1. "Get next issue" runs, captures `""`
2. `breakLoopIfEmpty` fires → "Feature work", "Run tests", "Git push" are all marked `[-]` skipped
3. Loop exits
4. Finalization still runs

You **can** place the break step later in the iteration, but then every preceding step runs on the final "no more work" iteration before the break fires. That's usually wasted work.

## Interaction with `--iterations N`

The cap and `breakLoopIfEmpty` are both ceilings — whichever fires first wins:

| `--iterations` | `breakLoopIfEmpty` fires on iter... | Actual iterations run |
|----------------|-------------------------------------|------------------------|
| `0` (unbounded) | Iteration 5 | 5 |
| `10` | Iteration 5 | 5 |
| `10` | Never (always has work) | 10 |
| `3` | Iteration 5 | 3 (cap hit first) |
| `0` (unbounded) | Never (always has work) | **infinite** — don't do this |

If `--iterations 0` and no step ever breaks the loop, pr9k will run forever. Always pair unbounded mode with at least one `breakLoopIfEmpty` step, or expect to kill it with `Ctrl+C` / `q`.

## What you'll see in the TUI

When the break fires, the iteration checkbox grid shows the break step as `[✓]` (it completed successfully — empty output is not a failure) and every subsequent step as `[-]`:

```
Iteration 5/0 — Issue #
[✓] Get next issue    [-] Feature work
[-] Run tests         [-] Git push
```

The log body shows the break step's normal output, a `Captured ISSUE_ID = ""` line, and then jumps straight to the finalization phase banner:

```
── Iteration 5 ─────────────

Starting step: Get next issue
─────────────────────────────

[no output — or a script-specific "no issues found" message]

Captured ISSUE_ID = ""

Finalizing
════════════════════════════════════════

Starting step: Deferred work
────────────────────────────

[finalize output]
```

The `RunResult.IterationsRun` value returned by `workflow.Run` reflects the **index of the iteration that triggered the break** — so if the break fires on iteration 5, `IterationsRun` is 5 (not 4). The "Ralph completed after N iteration(s)" summary line uses this value.

## Multiple break steps

You can set `breakLoopIfEmpty` on more than one step if any of them should be able to end the loop. For example, a workflow that drains a queue *and* checks a kill-switch file:

```json
{
  "iteration": [
    {"name": "Check killswitch", "isClaude": false, "command": ["scripts/check_killswitch"], "captureAs": "KILL_SWITCH", "breakLoopIfEmpty": true},
    {"name": "Get next task",    "isClaude": false, "command": ["scripts/get_next_task"],    "captureAs": "TASK_ID",     "breakLoopIfEmpty": true},
    {"name": "Process task",     "model": "sonnet", "promptFile": "process-task.md",         "isClaude": true}
  ]
}
```

Either the killswitch going absent, or the task queue going empty, exits the loop.

## Not a failure signal

**`breakLoopIfEmpty` is for "nothing left to do", not for error reporting.** If your step fails (non-zero exit, network error, missing file), let it fail — pr9k will enter error mode and let the user decide whether to retry or continue. Don't use a "print an empty string on error" pattern to smuggle failures through the break; you'll lose visibility into what broke.

For handling step failures, see [Recovering from Step Failures](recovering-from-step-failures.md).

## Related documentation

- [Capturing Step Output](capturing-step-output.md) — How `captureAs` binds values, which `breakLoopIfEmpty` reads
- [Building Custom Workflows](building-custom-workflows.md) — Full step schema including `breakLoopIfEmpty`
- [Recovering from Step Failures](recovering-from-step-failures.md) — The distinction between "nothing to do" and "something broke"
- [Workflow Orchestration](../features/workflow-orchestration.md) — Implementation: the `lastState == StepDone` guard, remaining-step skipping
- [Step Definitions & Prompt Building](../code-packages/steps.md) — Step schema reference
