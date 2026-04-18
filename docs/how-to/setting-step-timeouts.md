# Setting Step Timeouts

The `timeoutSeconds` field caps the wall-clock time a single step may run. When the deadline expires, pr9k sends `SIGTERM` to the Docker container (for claude steps) or the host process (for non-claude steps). If the process has not exited within 10 seconds, `SIGKILL` is sent.

## When to use it

Use `timeoutSeconds` on steps where runaway behaviour has been observed in practice. Typical examples:

- **Test writing** — an LLM that runs each test fix individually and re-executes the full suite between each can consume 30–60+ minutes per iteration.
- **Feature work** — complex refactors can occasionally loop indefinitely.

A timeout is not a substitute for writing efficient prompts; it is a last-resort guard.

## Configuration

Add `timeoutSeconds` to any step in `config.json`:

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

**Validator constraint:** `timeoutSeconds` must be a positive integer when set and must not exceed `86400` (24 hours). Omitting the field (or setting it to `0` via omitempty round-trip) means no timeout.

## What happens on timeout

1. The timeout goroutine sends `SIGTERM` after `timeoutSeconds` wall-clock seconds:
   - **Claude steps** — delivered via `docker kill --signal=SIGTERM` to the container.
   - **Non-claude steps** — delivered via `syscall.Kill(-pid, SIGTERM)` to the process group (the host child is started with `Setpgid: true`, so grandchildren are included).
2. If the process has not exited within 10 seconds, `SIGKILL` is sent to the same target.
3. The step's exit code is non-zero → the step is recorded as `status: "failed"` in `.ralph-cache/iteration.jsonl` with `notes: "timed out after Ns"`.
4. The workflow enters error mode (same as any other non-zero exit), and the user can choose to continue, retry, or quit.

## Partial session-ID blacklist

When a Claude step times out and the `claudestream` pipeline has already received a `session_id` from the model, that session ID is added to an in-memory blacklist (accessible via `Runner.SessionBlacklisted` / `Runner.BlacklistedSessions`). A future issue will wire a session-resume gate that consults this list to prevent resuming a timed-out session.

Session IDs are also written to `.ralph-cache/iteration.jsonl`. If session IDs are sensitive in your environment, add `.ralph-cache/` to `.gitignore` in the target repository.

## Advisory prompt budget

For claude steps, pair the enforced cap with an advisory budget in the prompt:

```
Budget: write all tests first, then run the suite ONCE. If >5 tests fail,
fix them in batch rather than one at a time. Do not exceed 8 minutes of
wall-clock test execution.
```

The 8-minute figure is an advisory model budget — the model is asked to self-regulate to that limit. `timeoutSeconds: 900` (15 minutes) is the separate, enforced wall-clock cap that pr9k applies regardless of model behaviour. These are distinct: the advisory budget may be exceeded by a non-cooperative model, while the `timeoutSeconds` cap is always enforced by the runtime.
