# Resuming Sessions

pr9k can pass `--resume <session_id>` to a Claude step so it picks up directly from where the previous step left off — sharing the same conversation context rather than starting fresh. This is the `resumePrevious` pattern.

> **Note:** The engine is fully implemented but the **default workflow ships with this feature off** on all steps. Enabling it requires a field change in `config.json` and validation that it improves results for your workflow.

## When you want it

Use `resumePrevious` when two consecutive Claude steps are tightly coupled and the second step benefits from the first step's conversation history:

- **Test writing after test planning** — the test-writing step can continue the planning conversation rather than re-reading the plan from a file
- **Fix review items after code review** — the fix step can ask follow-up questions in the review conversation

If the steps are independent (different topics, different models, different phases), omit `resumePrevious` — each step starts with a clean context window.

## How it works

Mark a Claude step with `resumePrevious: true`:

```json
{
  "iteration": [
    {
      "name": "Test planning",
      "isClaude": true,
      "model": "opus",
      "promptFile": "test-planning.md"
    },
    {
      "name": "Test writing",
      "isClaude": true,
      "model": "sonnet",
      "promptFile": "test-writing.md",
      "resumePrevious": true
    }
  ]
}
```

At runtime, before "Test writing" starts, pr9k evaluates five gates. All five must pass for `--resume` to be injected; any single failure falls through to a fresh session without aborting the workflow:

| Gate | Condition | What triggers a fresh session |
|------|-----------|-------------------------------|
| G1 | Previous step produced a session ID | Previous step was a non-claude shell command |
| G2 | Previous step completed successfully | Previous step failed or was user-terminated |
| G3 | *(covered by G2)* | `is_error=true` sets the step failed, which blocks G2 |
| G4 | Previous step used fewer than 200 000 input tokens | Context window too large to resume safely |
| G5 | Previous step's session was not timed out | Timed-out sessions have unknown server-side state |

When a gate blocks, pr9k logs a message like:

```
resume gate blocked (G4: previous step context too large (217340 input tokens >= 200,000)) -- starting fresh session for step "Test writing"
```

The step then runs normally as a fresh session. No error is raised; no user action is required.

## Constraints

- `resumePrevious` is only valid on **Claude steps** (`isClaude: true`). Setting it on a non-claude step is a fatal validator error.
- The previous step must also be a **Claude step** for G1 to pass — non-claude steps produce no session ID. The validator emits a warning if the preceding step is non-claude.
- `resumePrevious` is evaluated per-step within a single phase. It **cannot bridge across phase or iteration boundaries** — the per-phase tracking is reset at the start of each iteration and at the start of the initialize and finalize phases.
- The first step of any phase always starts fresh (G1 fails on the zero-value session ID). The validator emits a warning if `resumePrevious` is set on the first step of a phase.

## Interaction with skipped steps

If a step is skipped via `skipIfCaptureEmpty`, the skip does **not** reset the resume chain. The next `resumePrevious` step evaluates gates against the step immediately before the skipped one:

```
Step A (claude)     →  completes, session ID captured
Step B (skipped)    →  skipped, tracking not updated
Step C (resumePrevious: true)  →  G1 checks Step A's session ID (Step B is invisible to the gates)
```

This means step C effectively continues step A's session across the skip.

## What you'll see in the TUI

When `--resume` is injected, no special indicator appears in the checkbox grid — the step runs like any other Claude step. The resume behavior is visible in the log body only when a gate blocks:

```
Starting step: Test writing
────────────────────────────────────────

resume gate blocked (G1: previous step has no session ID) -- starting fresh session for step "Test writing"

[claude output...]
```

When all gates pass, the log body shows no resume message — the step simply starts and claude picks up the prior conversation.

## Checking iteration.jsonl

The `.pr9k/iteration.jsonl` log records `session_id` for each Claude step. You can confirm a resume occurred by checking that the `session_id` of the resuming step matches the preceding step:

```bash
# Show step name and session_id for each claude step
jq -r 'select(.session_id != null) | "\(.step_name): \(.session_id)"' .pr9k/iteration.jsonl
```

If both steps show the same session ID, the resume worked. Different session IDs mean a gate blocked and a fresh session was started.

## Related documentation

- [Session Resume Gates](../features/workflow-orchestration.md#session-resume-gates-resumeprevious) — Implementation details: gate table, per-phase tracking, skip-chain behavior
- [Setting Step Timeouts](setting-step-timeouts.md) — How `timeoutSeconds` interacts with G5 (timed-out sessions are blacklisted)
- [Skipping Steps Conditionally](skipping-steps-conditionally.md) — How `skipIfCaptureEmpty` interacts with the resume chain
- [Building Custom Workflows](building-custom-workflows.md) — Full step schema reference
- [Debugging a Run](debugging-a-run.md) — Reading the iteration log to trace session IDs
