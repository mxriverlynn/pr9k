# Skipping Steps Conditionally

pr9k can skip a step when a preceding step produced an empty capture — without treating the skip as a failure. This is the `skipIfCaptureEmpty` pattern. The default workflow uses it to bypass the "Fix review items" step when the code reviewer reports nothing to fix.

## When you want it

Use `skipIfCaptureEmpty` when a step is only meaningful if an earlier step produced non-empty output. Common scenarios:

- **Conditional fix step** — "Run the fix step only when the review found something to fix"
- **Conditional upload** — "Only upload a report if one was generated"
- **Conditional notify** — "Only send a notification if there is new content"

If the step should always run regardless of prior output, omit `skipIfCaptureEmpty`.

## How it works

Mark a step with `skipIfCaptureEmpty` naming a capture variable bound by an earlier iteration step:

```json
{
  "iteration": [
    {
      "name": "Check review verdict",
      "isClaude": false,
      "command": ["scripts/review_verdict"],
      "captureAs": "VERDICT"
    },
    {
      "name": "Fix review items",
      "isClaude": true,
      "model": "sonnet",
      "promptFile": "fix-review-items.md",
      "skipIfCaptureEmpty": "VERDICT"
    }
  ]
}
```

At runtime, before "Fix review items" starts, pr9k checks three conditions:

1. `skipIfCaptureEmpty` is set on the step (`"VERDICT"` in the example)
2. The capture named by `skipIfCaptureEmpty` is empty (the trimmed last non-empty stdout line is `""`)
3. The step that produced the capture completed as `StepDone` — it did **not** return a non-zero exit code

If all three are true, pr9k:

- Marks the step as `[-]` (skipped) in the TUI header
- Logs `Step skipped (capture "VERDICT" is empty)`
- Writes a `"skipped"` record to `.pr9k/iteration.jsonl`
- Moves on to the next step without entering error mode

If the source step **failed** (non-zero exit), the skip check is suppressed and the dependent step runs normally. A failing verdict script should surface an error, not silently skip the fix.

## Producing an empty capture

Any non-claude step can act as the source. Print non-empty output when the skip should NOT fire and print nothing (or exit without output) when it should:

```bash
#!/usr/bin/env bash
set -euo pipefail

# If there's work to do, print "yes". Otherwise print nothing.
if grep -q "NOTHING-TO-FIX" code-review.md 2>/dev/null; then
  exit 0   # empty stdout → skip fires
fi
echo "yes"  # non-empty stdout → skip does not fire
```

## Constraints

- `skipIfCaptureEmpty` is valid in the **iteration** and **finalize** phases — not in initialize.
- It must reference a `captureAs` name bound by an **earlier step in the same phase**. Initialize-phase captures and iteration-phase captures are not allowed in a finalize step: per-phase `captureStates` maps mean cross-phase references would silently never fire.
- The field value must be non-empty when set (omit the field entirely to disable the check).

The validator enforces all three constraints at config-load time and reports an error if any is violated.

## What you'll see in the TUI

When the skip fires, the checkbox grid shows the source step as `[✓]` and the skipped step as `[-]`:

```
Iteration 1/0 — Issue #42
[✓] Check review verdict    [-] Fix review items
[✓] Summarize to issue      [✓] Close issue
```

The log body shows the source step's output, the skip message, and then continues with the next step:

```
── Iteration 1 ──────────────────────────

Starting step: Check review verdict
────────────────────────────────────────

[script output]

Captured VERDICT = ""

Step skipped (capture "VERDICT" is empty)

Starting step: Summarize to issue
────────────────────────────────────────
```

## Interaction with captureStates isolation

The skip check uses a `captureStates` map that is re-initialised at the start of each iteration. If iteration 1 captures an empty `VERDICT` (skip fires), iteration 2 starts with a fresh map — a non-empty `VERDICT` in iteration 2 will not be silently suppressed by state left over from iteration 1.

## Related documentation

- [Breaking Out of the Loop](breaking-out-of-the-loop.md) — The sibling `breakLoopIfEmpty` pattern for ending the iteration loop when a step produces empty output
- [Capturing Step Output](capturing-step-output.md) — How `captureAs` binds values that `skipIfCaptureEmpty` reads
- [Building Custom Workflows](building-custom-workflows.md) — Full step schema reference
- [Recovering from Step Failures](recovering-from-step-failures.md) — How error mode interacts with the skip guard
- [Workflow Orchestration](../features/workflow-orchestration.md) — Implementation: `captureStates` map, per-iteration reset, fail-safe defaults
