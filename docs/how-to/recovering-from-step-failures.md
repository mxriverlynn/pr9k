# Recovering from Step Failures

← [Back to How-To Guides](README.md)

Steps fail. A `git push` conflicts, a Claude step hits a rate limit, a test script returns 1 because of a flake. pr9k pauses when this happens and hands control back to you: **continue**, **retry**, or **quit**. This guide walks through the error-mode flow so you can make the right call when a step fails.

**Prerequisites**: you've launched a run and you're staring at a `[✗]` in the checkbox grid. If you haven't yet, see [Getting Started](getting-started.md) first; the keyboard map and screen layout are documented in [Reading the TUI](reading-the-tui.md). While the TUI is up you can also press `?` to open the live keyboard reference modal.

## What counts as a failure

A step is treated as failed when:

1. The subprocess returns a **non-zero exit code**, AND
2. The process was **not user-terminated** (you didn't press `n` to skip it)

If you pressed `n`, the orchestrator marks the step as done (`[✓]`) and moves on — `n`-initiated skips are not failures, they're intentional user actions.

If the process exited 0, it's done (`[✓]`) regardless of whether stdout was empty or stderr was loud.

Only non-zero-and-not-user-terminated triggers **Error mode**.

## What happens when a step fails

As soon as a step fails, four things happen:

1. The checkbox for the step flips to `[✗]`
2. The keyboard handler switches to `ModeError` (`KeyHandler.SetMode(ModeError)`)
3. The shortcut footer changes to `c continue  r retry  q quit`
4. The orchestration goroutine blocks on `<-h.Actions`, waiting for your decision

The workflow is paused until you press a key. You can scroll the log to inspect what went wrong — the streaming stdout and stderr from the failed step are still in the log panel — but no subprocess is running.

## Your three choices

### `c` — Continue

Accept the failure and move on.

- The step stays marked `[✗]` — the failure is visible in the log and the checkbox grid
- The next step in the iteration starts immediately
- The workflow goroutine returns `ActionContinue` from `runStepWithErrorHandling`

Use `c` when:

- The failure is expected/benign for this iteration (e.g., `git push` fails because there are no commits to push — your feature step had nothing to do)
- The downstream steps can handle the failed state
- You want to see how far the workflow gets before giving up

### `r` — Retry

Re-run the same step.

- The step stays marked `[✗]` from the previous attempt until the retry begins
- A `── <step name> (retry) ─────────────` separator is written to the log
- The same subprocess command runs again; success clears the `[✗]` to `[✓]`
- If the retry also fails, you land back in Error mode with the same three choices

Retries use the **same resolved command and prompt** as the original attempt. Captured variables from earlier steps in the iteration are not re-fetched — for example, a retry of "Feature work" uses the same `STARTING_SHA` that was captured at the start of the iteration, not a fresh one.

Use `r` when:

- The failure looks transient (network hiccup, rate limit, race condition)
- You just fixed something out-of-band (edited a file, rebooted a service)
- You want a second try before deciding whether to give up

### `q` — Quit

Start the quit confirmation flow.

- Mode switches to `ModeQuitConfirm`
- Footer changes to `Quit Power-Ralph.9000? (y/n, esc to cancel)`
- Pressing `y` confirms and begins shutdown (footer → `Quitting...`)
- Pressing `n` or `Esc` cancels and returns to `ModeError` so you can still pick `c` or `r`

See [Quitting Gracefully](quitting-gracefully.md) for the full quit flow.

Use `q` when:

- The failure is structural — continuing won't recover
- You need to investigate in another terminal before doing anything else
- The workflow has gone off the rails and you want to stop cleanly

## The orchestrator stays responsive during the pause

While you're sitting in error mode, `workflow.Run` is blocked on the `Actions` channel — but the TUI is not frozen. The log panel still streams anything the previous step hadn't flushed yet, scrolling works, the header and footer render on every frame, and SIGINT still triggers the signal handler (which calls `ForceQuit` and shuts down cleanly).

If you leave the workflow paused and walk away, nothing bad happens — no timeout, no auto-retry, no auto-continue. Ralph waits.

## After you choose: what happens next

| Choice | Next step | Checkbox state | Mode |
|--------|-----------|----------------|------|
| `c` | Advance to next step in the iteration | Failed step stays `[✗]` | Back to `ModeNormal` |
| `r` (success) | This step succeeds, advance | `[✗]` → `[✓]` | Back to `ModeNormal` |
| `r` (fails again) | Block in error mode again | Stays `[✗]` | Still `ModeError` |
| `q` → `y` | Shutdown (ForceQuit) | Stays `[✗]` | `ModeQuitting` then process exits |
| `q` → `n`/`Esc` | Return to error mode pause | Stays `[✗]` | `ModeError` |

## Edge cases

### The step terminated itself

If you pressed `n` **during** the step to skip it, a confirmation prompt appears (`Skip current step? y/n, esc to cancel`). Pressing `y` confirms the skip: the subprocess gets a SIGTERM (then SIGKILL after 3 seconds). The non-zero exit is treated as a **successful termination** (`WasTerminated() == true`) and the step is marked `[✓]` — no error mode, no pause, just advance. Pressing `n` or `Esc` cancels the skip and returns to normal mode. This is the mechanism behind "skip this step".

### `buildStep` failed before the subprocess could start

If prompt-file reading or variable substitution fails (e.g., a referenced `promptFile` doesn't exist), the error happens in `buildStep` before `Orchestrate` is called. The Run loop logs `Error preparing initialize step: ...` / `Error preparing steps: ...` / `Error preparing finalize step: ...` to the log body and skips that step, moving to the next. You do **not** enter error mode for build failures — they're treated as "skip this step" at the orchestrator level.

In practice this only happens when the config references a missing prompt file. The validator (which runs before the TUI starts) catches most of these cases, but file deletions between validation and execution can sneak through.

### A later iteration step re-fails after you chose `c`

Every step is independent. Choosing `c` on iteration 3's "Test writing" doesn't affect iteration 3's "Git push" or iteration 4's "Feature work" — both are still capable of failing and dropping you back into error mode.

### The step timed out but was configured with `onTimeout: "continue"`

If a step's config sets `onTimeout: "continue"` AND the per-step `timeoutSeconds` cap fires, pr9k skips the error-mode pause entirely. You'll see:

1. A one-line banner in the log: `── <step name> timed out after Ns — continuing (onTimeout=continue) ─────────────`
2. The step's checkbox flips to `[!]` (not `[✗]`) so soft-timeouts are visually distinct from hard failures
3. The workflow advances to the next step without prompting

The iteration log still records `status: "failed"` with `notes: "timed out after Ns"` for the affected step, so after-the-fact debugging sees the timeout. The bundled "Test writing" step ships with this policy enabled; see [Setting Step Timeouts](setting-step-timeouts.md) for details.

## Deciding: continue, retry, or quit?

A rough decision tree:

1. **Is the error transient?** (network, rate limit, file-lock race)
   - Yes → `r` (retry once or twice)
   - No → keep reading
2. **Does the next step depend on this step having succeeded?**
   - Yes → `q` (don't corrupt downstream state)
   - No → keep reading
3. **Is there something you need to fix before retrying?**
   - Yes, and it's quick → fix it, then `r`
   - Yes, and it's not quick → `q` (investigate without holding the workflow)
   - No → `c` (accept the failure and see how the rest goes)

## Related documentation

- ← [Back to How-To Guides](README.md)
- [Reading the TUI](reading-the-tui.md) — what error mode looks like (footer, `[✗]` checkbox); press `?` while running for the live keyboard map
- [Quitting Gracefully](quitting-gracefully.md) — the `q` path in detail
- [Setting Step Timeouts](setting-step-timeouts.md) — `timeoutSeconds` and `onTimeout: "continue"` for steps where soft-fails are preferable
- [Debugging a Run](debugging-a-run.md) — using the log file to investigate after the fact
- [Keyboard Input & Error Recovery](../features/keyboard-input.md) — the `ModeError` / `ModeQuitConfirm` state machine (contributor reference)
- [Workflow Orchestration](../features/workflow-orchestration.md) — error-handling loop and the retry separator
- [Signal Handling & Shutdown](../features/signal-handling.md) — SIGINT during error mode
