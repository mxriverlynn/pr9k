# Setting Step Timeouts

The `timeoutSeconds` field caps the wall-clock time a single step may run. When the deadline expires, ralph-tui sends `SIGTERM` to the Docker container (for claude steps) or the host process (for non-claude steps). If the process has not exited within 10 seconds, `SIGKILL` is sent.

## When to use it

Use `timeoutSeconds` on steps where runaway behaviour has been observed in practice. Typical examples:

- **Test writing** — an LLM that runs each test fix individually and re-executes the full suite between each can consume 30–60+ minutes per iteration.
- **Feature work** — complex refactors can occasionally loop indefinitely.

A timeout is not a substitute for writing efficient prompts; it is a last-resort guard.

## Configuration

Add `timeoutSeconds` to any step in `ralph-steps.json`:

```json
{
  "name": "Test writing",
  "isClaude": true,
  "model": "sonnet",
  "promptFile": "test-writing.md",
  "timeoutSeconds": 900
}
```

`900` seconds (15 minutes) is the default applied to the bundled "Test writing" step — roughly 3× the observed median duration.

**Validator constraint:** `timeoutSeconds` must be a positive integer when set. Omitting the field (or setting it to `0` via omitempty round-trip) means no timeout.

## What happens on timeout

1. The timeout goroutine sends `SIGTERM` (via `docker kill` for sandboxed steps) after `timeoutSeconds` wall-clock seconds.
2. If the process has not exited within 10 seconds, `SIGKILL` is sent.
3. The step's exit code is non-zero → the step is recorded as `status: "failed"` in `.ralph-cache/iteration.jsonl` with `notes: "timed out after Ns"`.
4. The workflow enters error mode (same as any other non-zero exit), and the user can choose to continue, retry, or quit.

## Partial session-ID blacklist

When a Claude step times out and the `claudestream` pipeline has already received a `session_id` from the model, that session ID is added to an in-memory `Runner.SessionBlacklist`. A future issue will wire a session-resume gate that consults this list to prevent resuming a timed-out session.

## Advisory prompt budget

For claude steps, pair the enforced cap with an advisory budget in the prompt:

```
Budget: write all tests first, then run the suite ONCE. If >5 tests fail,
fix them in batch rather than one at a time. Do not exceed 8 minutes of
wall-clock test execution.
```

The budget guides the model; `timeoutSeconds` enforces the cap regardless of model behaviour.
