# Setting Step Timeouts

The `timeoutSeconds` field caps the wall-clock time a single step may run. When the deadline expires, pr9k sends `SIGTERM` to the Docker container (for claude steps) or the host process (for non-claude steps). If the process has not exited within 10 seconds, `SIGKILL` is sent.

← [Back to How-To Guides](README.md)

**Prerequisites**: a working install — see [Getting Started](getting-started.md) — and familiarity with the step schema in [Building Custom Workflows](building-custom-workflows.md). This page covers the `timeoutSeconds` and `onTimeout` fields.

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
  "timeoutSeconds": 1800
}
```

`1800` seconds (30 minutes) is the value the bundled "Test writing" step ships with, sized ~2.5× the observed organic p95 (~733s) with margin for occasional long runs. (`onTimeout` controls fail-vs-continue behaviour and is explained below.)

**Validator constraint:** `timeoutSeconds` must be a positive integer when set and must not exceed `86400` (24 hours). Omitting the field (or setting it to `0` via omitempty round-trip) means no timeout.

## What happens on timeout

1. The timeout goroutine sends `SIGTERM` after `timeoutSeconds` wall-clock seconds:
   - **Claude steps** — delivered via `docker kill --signal=SIGTERM` to the container.
   - **Non-claude steps** — delivered via `syscall.Kill(-pid, SIGTERM)` to the process group (the host child is started with `Setpgid: true`, so grandchildren are included).
2. If the process has not exited within 10 seconds, `SIGKILL` is sent to the same target.
3. The step's exit code is non-zero → the step is recorded as `status: "failed"` in `.pr9k/iteration.jsonl` with `notes: "timed out after Ns"`.
4. Next step depends on the `onTimeout` policy (see the next section).

## Soft-fail on timeout — `onTimeout: "continue"`

Per-step `onTimeout` selects what happens when a timeout fires:

- `""` or `"fail"` (default) — the workflow enters error mode and blocks on `c / r / q` input, exactly like any other non-zero exit. Use when a timeout on this step indicates a real problem that a human should see.
- `"continue"` — pr9k writes a one-line banner to the log ("timed out after Ns — continuing (onTimeout=continue)"), marks the step with the distinct `[!]` checkbox glyph (`StepTimedOutContinuing`), and advances to the next step without prompting. The iteration record still stores `status: "failed"` with the `timed out after Ns` note, so structured logs retain the signal.

`onTimeout: "continue"` is intended for unattended runs where a step's work is typically partial-but-valuable by the time the timer fires (the bundled "Test writing" step is the canonical example: tests and commits may already be in place before the cap). Without this policy, a single overnight timeout would stall the entire run until a human presses `c` or `r`.

Internally, the orchestrate layer calls `ClearTimeoutFlag()` on the runner after taking the continue branch, so the next step's dispatcher sees a clean slate and does not emit a spurious synthetic record.

### Gotcha: interaction with `resumePrevious`

If the step immediately after an `onTimeout: "continue"` step sets `resumePrevious: true`, a soft-timeout path will fall through the G2 gate (`prevState != StepDone`) and the dependent step starts a fresh Claude session instead of resuming. This is validator-warned at config time; add it to the review when combining the two.

## Partial session-ID blacklist

When a Claude step times out and the `claudestream` pipeline has already received a `session_id` from the model, that session ID is added to an in-memory blacklist (accessible via `Runner.SessionBlacklisted` / `Runner.BlacklistedSessions`). A future issue will wire a session-resume gate that consults this list to prevent resuming a timed-out session.

Session IDs are also written to `.pr9k/iteration.jsonl`. If session IDs are sensitive in your environment, add `.pr9k/` to `.gitignore` in the target repository.

## Advisory prompt budget

For claude steps, pair the enforced cap with an advisory budget in the prompt:

```
Budget: write all tests first, then run the suite ONCE. If >5 tests fail,
fix them in batch rather than one at a time. Do not exceed 8 minutes of
wall-clock test execution.
```

The 8-minute figure is an advisory model budget — the model is asked to self-regulate to that limit. `timeoutSeconds: 1800` (30 minutes) is the separate, enforced wall-clock cap that pr9k applies regardless of model behaviour. These are distinct: the advisory budget may be exceeded by a non-cooperative model, while the `timeoutSeconds` cap is always enforced by the runtime.

## Related documentation

- ← [Back to How-To Guides](README.md)
- [Building Custom Workflows](building-custom-workflows.md) — full step schema
- [Recovering from Step Failures](recovering-from-step-failures.md) — what error mode looks like when a default-policy timeout fires
- [Resuming Sessions](resuming-sessions.md) — important: a soft-timed-out step blocks `resumePrevious` on the next step (G2 gate)
- [Debugging a Run](debugging-a-run.md) — finding the `timed out after Ns` notes in `.pr9k/iteration.jsonl`
