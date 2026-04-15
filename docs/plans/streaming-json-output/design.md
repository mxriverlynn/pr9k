# Streaming JSON Output from Claude — Design Plan

**Status:** Reviewed (iterations 1–3 + agent validation complete)
**Author:** River + Claude
**Date:** 2026-04-14

## Goal

Replace the current `-p <prompt>` plain-text output mode with `-p <prompt> --output-format stream-json --verbose` so that we can:

1. Capture structured data (turn-by-turn assistant messages, tool calls, token usage, cost).
2. Build a foundation for future analytics (token use, cost reporting, per-step performance).
3. Continue presenting a human-readable view in the TUI by extracting only the relevant text, not raw JSON.

## Evidence base

This plan draws on:

- **Official Claude Code docs**
  - `cli-reference` confirms `--output-format` accepts `text | json | stream-json` and `--verbose` "shows full turn-by-turn output."
  - `headless` confirms `stream-json` is "newline-delimited JSON for real-time streaming" and that `-p --output-format stream-json --verbose` is the supported invocation.
  - `agent-sdk/streaming-output` documents the message flow and requires `--include-partial-messages` for token-level deltas; without it, only complete `AssistantMessage` and `ResultMessage` objects are emitted.
  - `agent-sdk/python` provides the dataclass schema for `SystemMessage`, `AssistantMessage`, `UserMessage`, `ResultMessage`, `TextBlock`.
- **Repository code**
  - `ralph-tui/internal/sandbox/command.go:21-62` — `BuildRunArgs` constructs the docker+claude argv (lines 53-59 are where `-p` is appended).
  - `ralph-tui/internal/workflow/workflow.go:208-309` — `runCommand` and `forwardPipe` stream stdout/stderr line-by-line via `sendLine`.
  - `ralph-tui/internal/workflow/run.go:31-41` — `stepDispatcher` already routes claude vs non-claude steps via `IsClaude`.
  - `ralph-tui/cmd/ralph-tui/main.go:162-199` — buffered `lineCh`, drain goroutine, `LogLinesMsg`.
  - `ralph-tui/internal/ui/log_panel.go:65-94` — TUI viewport ring buffer (500 lines).
- **Repository docs**
  - `docs/features/subprocess-execution.md` — sendLine architecture, 256KB scanner, dual-pipe goroutines, stdout-only LastCapture.
  - `docs/features/variable-state.md` — `captureAs` semantics (last non-empty stdout line, persistent vs iteration scope).
  - `docs/features/file-logging.md` — `[timestamp] [iteration] [step] line` format.
  - `docs/features/tui-display.md` — log body chrome (phase banners, step separators, capture logs).
  - `docs/adr/20260410170952-narrow-reading-principle.md` — ralph-tui is a generic step runner; workflow content lives in JSON, not Go.

## Stream-json schema reference

Each line emitted by `claude -p --output-format stream-json --verbose` is one JSON object. The relevant message types:

### `system` (init)

Emitted once at start. Contains session metadata. Shape:
```json
{ "type": "system", "subtype": "init", "session_id": "...", ... }
```

May also be emitted later with `subtype: "api_retry"` (documented in `headless`):
```json
{ "type": "system", "subtype": "api_retry",
  "attempt": 1, "max_retries": 5, "retry_delay_ms": 2000,
  "error_status": 429, "error": "rate_limit", "uuid": "...", "session_id": "..." }
```

### `assistant`

One per assistant turn. Contains an array of content blocks (text, thinking, tool_use):
```json
{ "type": "assistant",
  "message": { "id": "...", "model": "...", "content": [
      { "type": "text",      "text": "..." },
      { "type": "tool_use",  "name": "Bash", "input": {...} }
  ], "usage": { "input_tokens": 1234, "output_tokens": 56,
                 "cache_creation_input_tokens": 0, "cache_read_input_tokens": 0 } } }
```

### `user`

Tool results being fed back to the model:
```json
{ "type": "user",
  "message": { "content": [
    { "type": "tool_result", "tool_use_id": "...", "content": "..." }
  ] } }
```

### `result`

Emitted last. The authoritative final answer:
```json
{ "type": "result",
  "subtype": "success",
  "is_error": false,
  "duration_ms": 12345,
  "duration_api_ms": 6789,
  "num_turns": 4,
  "session_id": "...",
  "total_cost_usd": 0.0123,
  "usage": { "input_tokens": ..., "output_tokens": ...,
             "cache_creation_input_tokens": ..., "cache_read_input_tokens": ... },
  "result": "<final assistant text>",
  "stop_reason": "end_turn" }
```

## Decisions (high confidence — driven by evidence)

### D1. Append `--output-format stream-json --verbose` to the claude invocation
- **Where:** `ralph-tui/internal/sandbox/command.go:53-59` (`BuildRunArgs`).
- **Why:** Required by claude CLI for stream-json mode in `-p` (confirmed in cli-reference). User requested.
- **Risk:** None — both flags are stable, documented.

### D2. Do NOT use `--include-partial-messages`
- **Why:** Token-level deltas would create more parsing complexity without benefit. We display per-turn text, not per-token. Docs note known limitations (e.g., disables under explicit thinking budgets). Simpler model: parse one full assistant message at a time.
- **Reversible:** Trivial to add later if we want a typing-style UI.

### D3. Parse NDJSON line-by-line; one JSON object per line — and raise the line cap
- **Why:** Documented behavior. Existing scanner already line-splits.
- **Buffer size change:** The current 256KB `bufio.Scanner` buffer in `forwardPipe` (`internal/workflow/workflow.go:267-269`) is **not** sufficient for stream-json lines. A single event can carry multi-MB payloads — e.g., a `user` `tool_result` echoing a large file that `Read` or `Grep` returned, a `tool_use` whose `input.content` is an entire file being written by `Write`/`Edit`, or a `Bash` `tool_result` containing full test output. `bufio.Scanner` returns `bufio.ErrTooLong` when a line exceeds its buffer; the current code only logs the scanner error and exits the scan loop — silently dropping the remainder of the stream. Under stream-json that drop would also mean the `result` event never arrives, and D15's "no result" synthesis would wrongly report the step as failed.
- **Replacement approach:** For claude steps, replace the `bufio.Scanner` with a `bufio.Reader.ReadString('\n')` loop (or `bufio.Scanner` with a ~16MB cap plus a visible truncation marker). Non-claude steps keep the 256KB `bufio.Scanner` — plain-text logs truly are line-bounded. Implementation detail: the claude-aware wrapper in `RunSandboxedStep` provides its own pipe-reader that feeds both `RawWriter` (for verbatim bytes, see D14) and the parser/renderer chain. If a line exceeds a hard safety cap (say 64MB to guard memory), the wrapper writes a sentinel line `{"type":"ralph_truncation_marker","reason":"line_too_long","bytes":<n>}` to RawWriter, logs a user-visible warning, and continues.
- **Testing:** Add a case that feeds a 2MB single-line `tool_result` through the pipeline end-to-end.

### D4. Only claude steps (`IsClaude == true`) get JSON parsing
- **Why:** Non-claude steps emit plain text. Branching already exists at `ralph-tui/internal/workflow/run.go:36-40`. Narrow-reading principle (ADR-2): the runner stays generic; the JSON-awareness lives behind the `IsClaude` boundary.

### D5. New package `internal/claudestream` houses the parser + extractor
- **Why:** Keeps `internal/workflow/workflow.go` generic (no JSON knowledge in the subprocess layer). The package exposes:
  - A `Parser` that consumes raw NDJSON lines and emits typed events.
  - A `Renderer` that converts typed events into display lines (the strings sent to `sendLine`).
  - An `Aggregator` that accumulates the final `result.result`, total tokens, total cost.
- **Reversible:** Package-level abstraction; can be reshaped without touching subprocess code.

### D6. `captureAs` for claude steps binds to `result.result` (not "last stdout line")
- **Why:** With JSON output, "last non-empty stdout line" would be the JSON `{"type":"result",...}` blob, which is meaningless to bind to `{{VAR}}`. The `result` message has an explicit `result` field documented as the final assistant text. Aligns with the existing semantic intent ("the step's answer").
- **Non-breaking on current workflow:** Verified against `ralph-tui/ralph-steps.json` — no `isClaude: true` step currently declares `captureAs`. The behavior change alters nothing observable in the default workflow today.
- **Validator change required:** `internal/validator/validator.go:281-290` Rule A currently **rejects** `captureAs` on any claude step (`"captureAs on a claude step is not allowed: after sandboxing, captured stdout is docker's output, not claude's"`). That rule was correct under plain-text `-p` where the "last line" would have been docker's log output. Under stream-json, the `Aggregator.Result()` returns the parsed `result.result` field, not docker's stdout — so the rule's rationale no longer applies to claude steps. **Action:** remove Rule A and the associated tests in `internal/validator/validator_test.go`. Leave schema rules 271 ("captureAs must not be empty when set") and 273 (reserved-name guard) and 276 (duplicate-in-phase guard) untouched — they remain correct.
- **Reversible:** Yes — the `Aggregator` is the single source of truth for the captured value.

### D7. Malformed JSON lines are logged and skipped (do not abort the step)
- **Why:** Defensive parsing. If claude emits an unparsable line (version drift, partial flush), we log it raw to the file and continue. The step still has `is_error` and exit code as authoritative success signals.

### D8. Unknown JSON fields are tolerated
- **Why:** Schema may evolve. Use `json.Decoder` with structs that ignore unknown fields (Go default). Only known fields drive behavior.

### D9. Non-claude steps and the file-logging format are unchanged
- **Why:** Narrow scope. Non-claude steps already work correctly with plaintext. Existing file-logging chrome (`[timestamp] [iter] [step] line`) wraps whatever we emit; the rendered display lines flow through it untouched.

### D10. The TUI ring buffer (500 lines), drain batching, and viewport behavior are unchanged
- **Why:** The Renderer emits the same `string` lines via `sendLine`. Downstream is agnostic to source.

## Decisions resolved during grilling

| ID | Topic | Decision |
|---|---|---|
| D11 | TUI display granularity | Assistant text + one-line tool-use indicators (no thinking, no tool results) |
| D12 | Tool-indicator format | `→ <Tool> <smart-summary>` truncated to 80 chars; per-tool field table with JSON fallback |
| D13 | Token / cost handling | Per-step summary line + cumulative run total; no auto `{{VAR}}` exposure |
| D14 | Raw JSONL persistence | Per-step `.jsonl` files under `logs/<run-timestamp>/<phase-prefix>-<slug>.jsonl` |
| D15 | `is_error == true` handling | Treated as step failure; routes through existing `c`/`r`/`q` recovery |
| D16 | Session ID as variable | Not bound; preserved in JSONL only |
| D17 | Per-step opt-in/out | Uniform — all `isClaude: true` steps use stream-json |
| D18 | Rollout strategy | Hard switch in one PR; no fallback; patch version bump |
| D19 | Multi-block formatting | Inline with natural newline splits; blank line between assistant turns |

### D11. TUI shows assistant text + one-line tool-use indicators (Option C)
- **Why:** Preserves "feels alive" UX. Tool-result content is excluded to avoid flooding the 500-line viewport.
- **Renderer rules:**
  - `assistant.message.content[].type == "text"` → emit each non-empty `text` as one or more lines (split on `\n`).
  - `assistant.message.content[].type == "tool_use"` → emit a single `→ <Tool>` indicator line (format pending Q1b).
  - `assistant.message.content[].type == "thinking"` → not displayed (low value to humans, would also flood).
  - `user` (tool_result) messages → not displayed.
  - `system` init → emit one banner line (e.g., `[claude session <id> started, model <name>]`).
  - `system` api_retry → emit a visible warning line (`⚠ retry <attempt>/<max> in <ms>ms — <error>`).

### D12. Tool-indicator format: name + smart per-tool summary, truncated to 80 chars
- **Per-tool field used for the summary:**
  | Tool | Field rendered |
  |---|---|
  | `Bash` | `command` |
  | `Read`, `Edit`, `Write`, `NotebookEdit` | `file_path` |
  | `Glob`, `Grep` | `pattern` |
  | `Task`, `Agent` | `description` |
  | `WebFetch` | `url` |
  | (any other / MCP / future) | compact JSON of `input`, truncated |
- **Format:** `→ <ToolName> <summary>` — single line, summary truncated to 80 chars with `…` suffix when longer.
- **Why:** Mirrors what users currently see from claude in plain `-p` mode. The per-tool table is short and lives in one place in `internal/claudestream/render.go`. Unknown tools degrade gracefully without code changes.

### D13. Token usage and cost: per-step summary line + cumulative run total
- **Per-step (2a):** Renderer emits a closing line at step completion containing `<turns> turns · <in>/<out> tokens (cache: <creation>/<read>) · $<cost> · <duration>`. Visible in TUI; persisted in file logger via the existing prefix.
- **Variables (2b):** No auto-populated `{{VAR}}` for token/cost. Defer until a real consumer exists (per ADR-2 narrow-reading).
- **Run total (2c):** Orchestrator's finalize phase emits a closing line summing cost, in/out tokens, and turn count across every claude step in the run.
- **Where the totals live:** A small accumulator in the workflow runner (e.g., `RunStats`) collects each step's `Aggregator` snapshot. The accumulator is reset at run start.
- **Why:** Immediate feedback now; analytics-shaped data is preserved (per Q3) for later programmatic consumption; no speculative variable surface.

### D14. Raw JSONL persisted per claude step under a per-run directory
- **Layout (matches the actual `ralph-YYYY-MM-DD-HHMMSS` convention — see `internal/logger/logger.go:31`):**
  ```
  logs/
    ralph-2026-04-14-173022.log           # existing human-readable log (unchanged)
    ralph-2026-04-14-173022/              # NEW per-run directory, same basename as the .log file
      initialize-02-get-gh-user.jsonl     # only present for claude steps
      iter01-03-feature-work.jsonl
      iter01-04-test-planning.jsonl
      iter02-03-feature-work.jsonl
      finalize-02-lessons-learned.jsonl
  ```
- **File contents:** Verbatim NDJSON output from `claude -p --output-format stream-json --verbose`. No wrapper. No re-encoding. One file per claude step invocation.
- **Filename shape:** `<phase-prefix><NN>-<step-slug>.jsonl`
  - **Step slug:** kebab-case of the step name (e.g., `Feature work` → `feature-work`).
  - **Phase prefix:** `initialize-`, `iterNN-` (zero-padded to 2 digits), or `finalize-`.
  - **Step index `NN`:** zero-padded 2-digit position within the phase. Needed because step names can repeat (e.g., if a workflow ever has two steps with the same name) and to preserve execution order on disk listings.
- **Non-claude steps:** No `.jsonl` file is created.
- **Writer:** Owned by `internal/claudestream` (a `RawWriter` opened per claude-step invocation, closed via `defer` in `RunSandboxedStep`'s wrapper so it closes on natural exit, terminator path, and the `cmd.Start()` failure path).
- **Retry behavior:** When the user selects **retry** in error mode (`docs/features/keyboard-input.md`), `RunSandboxedStep` is invoked a second time for the same step. The `.jsonl` file is **opened in truncate-append mode on each invocation** — the retry overwrites the prior attempt's bytes. Rationale: the file is a snapshot of "what claude emitted for this step invocation"; preserving the failed attempt would change the file's semantic from "this step's output" to "all attempts, interleaved." Debuggers wanting both attempts can read the plain-text log (which contains rendered lines for every attempt, separated by `RetryStepSeparator`).
- **Per-run timestamp source:** `NewLogger` captures its timestamp at construction. To avoid coordinating two independent timestamps, add a `Logger.RunStamp() string` accessor (returns the `"ralph-2006-01-02-150405"` basename without the `.log` extension). `main.go` reads this after `NewLogger` and passes it into `RunConfig.RunStamp`. `claudestream.Pipeline` constructs the artifact directory by joining `projectDir/logs/<runstamp>/`.
- **Why:** Preserves every byte claude emits — required for the user's stated future analytics goal. Per-step granularity means a downstream tool can read one file at a time without parsing run-level boundaries. Mirrors the existing per-run timestamp convention from `docs/features/file-logging.md`.

### D15. `result.is_error == true` is treated as step failure
- **Behavior:** When a claude step's `result` message has `is_error: true`, the `RunSandboxedStep` call returns a non-nil error (synthesized by the `Aggregator`) even if the docker subprocess exited 0. This triggers the existing error-mode interactive recovery (`c`/`r`/`q`) per `docs/features/keyboard-input.md`.
- **Error message:** Includes `subtype` and `stop_reason` from the result message (e.g., `claude step ended with is_error=true: subtype=error_max_turns, stop_reason=max_turns`).
- **Edge case — no `result` message ever arrives** (claude crashed before emitting one): treated as failure too. The `Aggregator` returns an error if it never observed a `result` event and the subprocess exited 0.
- **Edge case — user-initiated terminate (`s`-skip):** If `Runner.Terminate()` was called, `runStepWithErrorHandling` checks `WasTerminated()` before consulting the error and treats the step as done-skip (see `internal/ui/orchestrate.go:64`). The `Aggregator.Err()` check must **not** short-circuit that: the wrapper should still return the aggregator error so that the caller can distinguish, but `WasTerminated()` takes precedence in `runStepWithErrorHandling`. This matches existing behavior for subprocess `err != nil && WasTerminated()`.
- **Edge case — retry after is_error:** On retry, a fresh `Aggregator` and `RawWriter` are constructed for the new `RunSandboxedStep` invocation. The prior attempt's `StepStats` **are** folded into `RunStats` per D21 — failed attempts consumed real tokens and the cumulative total must reflect that so the summary reconciles against the Anthropic invoice.
- **Why:** Aligns "step success" with the user's intent (the workflow can't progress after a failed claude turn). Reuses the existing recovery UX rather than inventing a new failure category.

### D16. `session_id` is not auto-bound to a built-in variable
- **Behavior:** Session IDs are preserved in the per-step JSONL artifact (D14) and surfaced in the TUI banner (D11). No `{{LAST_SESSION_ID}}` is added to the `VarTable` built-ins.
- **Why:** No prompt or script in `prompts/` or `scripts/` currently consumes a session ID. ADR-2 narrow-reading discourages speculative built-ins. Per `docs/coding-standards/versioning.md` the `{{VAR}}` language is part of ralph-tui's public API, so additions should be justified by a real consumer.
- **Reversible:** Trivial to add a built-in later when a workflow needs it.

### D17. Stream-json + parsing applies uniformly to every `isClaude: true` step
- **Behavior:** No new field in ralph-steps.json. `BuildRunArgs` always appends `--output-format stream-json --verbose`. The JSON pipeline activates whenever `IsClaude == true`.
- **Why:** No current or planned workflow needs a non-streaming claude call. ADR-2 narrow-reading: format is part of "what a claude step is," not a per-step knob. Avoids speculative schema growth in `internal/validator/validator.go`.
- **Reversible:** A future need can add the field then.

### D18. Hard switch — no fallback, no feature flag
- **Behavior:** One PR removes plain-text `-p` output handling. After merge, all claude steps use stream-json. No env var, no CLI flag, no config knob to disable.
- **Why:** Repo has no external users; backward compatibility is not a goal. A flag would be a permanent maintenance cost for zero benefit. `git revert` is the rollback if the change breaks.
- **Version bump:** Patch-level per `docs/coding-standards/versioning.md` — no change to CLI flags, `ralph-steps.json` schema, `{{VAR}}` language, or `--version` output. Internal "how we read claude" is implementation detail.

### D19. Renderer spacing — inline blocks with natural newline splits + blank line between assistant turns
- **Inner spacing within a single assistant message:**
  - Text blocks: split the block's `text` field on `\n` and emit each as a separate log line; empty lines preserved.
  - Tool-use blocks: emit one `→ <Tool> <summary>` line per block (per D12).
  - Thinking blocks: skipped (per D11).
  - No blank lines inserted between blocks within the same assistant message.
- **Between assistant turns:** When a new `type: "assistant"` message is encountered (after the first one of the step), the renderer emits a single blank line first.
- **Between system / user / result events:** No turn separator — those are either banner-style lines (system init, retries, step-end summary) or invisible (user/tool_result).
- **Why:** Respects claude's own paragraph structure inside text blocks; gives a faint structural landmark between turns for log-scrollback comprehension; keeps total line count small to preserve the 500-line ring buffer.

## Decisions resolved during adversarial review

### D20. stdout feeds the parser; stderr bypasses it
- **Behavior:** The claude pipeline wrapper in `RunSandboxedStep` routes only the **stdout** stream through `RawWriter` + `Parser` + `Renderer`. The **stderr** stream keeps its current behavior: each line goes directly to the file logger (`logger.Log`) and to `sendLine` with a visible `[stderr] ` prefix prepended by the wrapper. Stderr lines are **not** parsed as JSON; they are **not** written to the `.jsonl` artifact.
- **Why:** Without this split, docker-layer diagnostics (`Cannot connect to Docker daemon`, image-pull progress), claude-CLI error text, and Go runtime panics would all hit `Parser.Parse`, each would return `MalformedLineError`, and D7's "log and skip" rule would quietly swallow them. That is a regression from today's behavior where stderr is interleaved into the TUI/log verbatim.
- **Artifact:** If stderr volume becomes a debug concern, a future improvement can add `<slug>.stderr.log` siblings — out of scope for this PR.

### D21. All `RunSandboxedStep` returns fold into `RunStats`; `c`/`r` differ only in JSONL retention
- **Token accounting (applies to every claude step return, regardless of outcome or user recovery choice):** When `RunSandboxedStep` returns, its `StepStats` is unconditionally added to `RunStats`. Successful, continued-past-failure, and discarded-on-retry attempts all count. Rationale: the cumulative total must match real Anthropic spend so a user can reconcile the run against their invoice. Computing "successful-workflow-only" spend would be a synthetic number with no external referent.
- **Behavior on `c` (continue):** The failed step's JSONL file is preserved as-is (it contains whatever events arrived before the failure). Tokens fold in per the rule above.
- **Behavior on `r` (retry):** The JSONL file is truncated (D14) because the new attempt is the definitive record for that step slot. Tokens from the prior (discarded) attempt still fold into `RunStats` per the rule above — the JSONL is discarded but the spend was real.
- **Summary-line semantics:** The finalize summary line is labeled `total claude spend across <N> step invocations (including <R> retries)` so the number is self-describing. `N` is the total count of `RunSandboxedStep` returns; `R` is the subset that were followed by a user `r` retry.

### D22. TUI ring buffer raised from 500 to 2000 lines
- **Change:** `internal/ui/log_panel.go:65-94` — raise the ring buffer cap from 500 to 2000 lines.
- **Why:** Under stream-json, each claude step renders multiple assistant turns (each turn's text may span several lines after `\n` split) plus per-tool-use indicators plus system/retry banners plus a step summary. A single feature-work step can legitimately emit 200–800 lines; at 500, phase banners scroll off before the step finishes, breaking the "navigate by chrome landmarks" affordance documented in `docs/how-to/reading-the-tui.md`. 2000 lines keeps an iteration's worth of chrome visible. Cost: ~4× the memory footprint of the string slice (negligible).
- **Alternative considered:** Compressing assistant turns into a one-line summary. Rejected — loses the "feels alive" UX that D11 explicitly preserves.
- **Reversible:** Single constant; trivial to tune.

### D23. Heartbeat indicator for long silent turns
- **Behavior:** When no stream-json event arrives for N seconds (default 15s) during a claude step, the TUI renders a transient `⋯ thinking (Ns)` line in the iteration line area (not the log body — it must not be appended to the ring buffer). The line updates in place each tick; it is cleared as soon as the next event arrives.
- **Why:** Without `--include-partial-messages` (D2), there is no visible activity between assistant turns. Plain-text `-p` mode today streams tokens progressively, so the user sees continuous output. Stream-json can be silent for 30+ seconds during explicit-thinking-budget turns or while claude waits on a slow tool. A passive heartbeat replaces token-level streaming's "feels alive" contribution without the parsing/scope cost of `--include-partial-messages`.
- **Implementation:** Tick driven by a `tea.Tick` every 1s; the wrapper records `lastEventAt` on every observed event; the status header reads it and renders the heartbeat when `time.Since(lastEventAt) > threshold`.
- **Reversible:** Isolated to one rendering path in the status header.

### D24. RunStamp gains subsecond precision to prevent same-second collisions
- **Change:** `Logger.RunStamp()` and the log filename use `ralph-2006-01-02-150405.000` (milliseconds). Existing `ralph-YYYY-MM-DD-HHMMSS.log` pattern becomes `ralph-YYYY-MM-DD-HHMMSS-mmm.log`.
- **Why:** Two runs started in the same wall-clock second (common in CI with `-n 1` restarts, or when a user types `./bin/ralph-tui` twice quickly after a failed run) would otherwise produce the same RunStamp. `MkdirAll` is a no-op on the second run, and `O_TRUNC` on retry would overwrite the first run's JSONL artifacts.
- **Migration:** This changes an observed user-facing filename. Callers / docs that reference the old format (see `docs/features/file-logging.md:11`) must be updated. The `--version` output and any compatibility contracts are untouched per the versioning ADR.
- **Alternative considered:** Retry `MkdirAll` with an `-N` suffix on collision. Rejected — more complex and the subsecond stamp is already standard.

### D25. RunStats is orchestrator-goroutine-only
- **Rule:** `RunStats` is written only by the orchestrator after each `RunSandboxedStep` returns. No other goroutine reads or writes it. If a future requirement wants a live "running total" line in the TUI during a step, the accumulator gains a mutex and the current-step partial stats are exposed via snapshot-then-Send per `docs/coding-standards/concurrency.md`.
- **Enforcement:** The `RunStats` struct lives in `internal/workflow/run.go`, is not exported from the package, and carries a single-line comment stating the constraint. No mutex is added prophylactically.

### D26. Crash-mid-step resilience
- **Behavior:** After each parsed event, `RawWriter` calls `f.Sync()` is **not** done (cost too high). Instead, after observing the `result` event, `RawWriter` writes a trailing sentinel line: `{"type":"ralph_end","ok":true,"schema":"v1"}`. Files without a trailing `ralph_end` line indicate a crashed or terminated run; downstream analytics can reject them.
- **Why:** Balances write throughput (no per-line fsync) against being able to detect truncated artifacts. A host-level crash (OOM, power loss, SIGKILL) bypasses `defer`, so no amount of `defer`-based cleanup is a guarantee; a sentinel line is the cheapest file-level integrity signal.

## Implementation sketch

### New code

1. **`ralph-tui/internal/claudestream/`** (new package, all logic for stream-json handling)
   - `event.go` — Typed event structs: `SystemEvent`, `AssistantEvent`, `UserEvent`, `ResultEvent`. Each is a Go struct with `json:"..."` tags ignoring unknown fields. A `ContentBlock` discriminated union with `text`, `tool_use`, `thinking`, `tool_result` shapes.
   - `parser.go` — `Parser.Parse(line []byte) (Event, error)`. Returns one of the typed events. Malformed lines surface a `MalformedLineError` carrying the raw bytes; callers (the wiring in step 3) log and continue per D7.
   - `render.go` — `Renderer.Render(ev Event) []string`. Pure function: given an event, returns zero or more display lines per D11/D12/D19. Holds the per-tool summary table from D12. Handles the inter-turn blank line by tracking whether it's seen a prior `assistant` event.
   - `aggregate.go` — `Aggregator` accumulates state across a single step: final `result.result` text, total usage struct, total cost, num_turns, duration_ms, observed `is_error`, observed `subtype`, observed `session_id`. Exposes `Result() string` (for D6 captureAs), `Stats() StepStats` (for D13), `Err() error` (for D15: returns non-nil if `is_error` true or no `result` ever observed), and a sentinel for D15's "result never arrived" case.
   - `rawwriter.go` — `RawWriter` opens the per-step `.jsonl` file and appends every received line verbatim (before parsing). `io.Closer`. Lifecycle owned by the step-runner wrapper (step 3 below).
   - `slug.go` — Tiny helper for D14 filename generation (kebab-case slug from step name; phase-prefix builder).

2. **`ralph-tui/internal/claudestream/<file>_test.go`** — Unit tests for each component (see Test plan below).

### Wiring (modifications to existing files)

3. **`ralph-tui/internal/sandbox/command.go:53-59`** — Append `"--output-format", "stream-json", "--verbose"` to the claude argv, after the existing `-p`, `prompt` pair. (D1)

4. **`ralph-tui/internal/workflow/workflow.go`** — `RunSandboxedStep` becomes claude-aware. The JSON pipeline context is carried in an extended `SandboxOptions`:
   ```go
   type SandboxOptions struct {
       Terminator  func(syscall.Signal) error
       CidfilePath string
       // NEW: fields carrying the claudestream pipeline context.
       // Empty ArtifactPath disables JSONL persistence (used in tests that
       // don't want to touch disk). When ArtifactPath is non-empty, the
       // RawWriter opens it with O_CREATE|O_TRUNC|O_WRONLY (retry overwrite).
       ArtifactPath string
       // CaptureMode selects post-step capture semantics:
       //   CaptureLastLine  — current behavior (non-claude steps)
       //   CaptureResult    — bind Aggregator.Result() to LastCapture (D6)
       CaptureMode CaptureMode
   }
   ```
   - `RunSandboxedStep` constructs a per-step `claudestream.Pipeline` (`Parser` + `Renderer` + `Aggregator` + `RawWriter`) when `CaptureMode == CaptureResult`. For claude steps, stdout and stderr are handled **separately** (D20):
     - **Stdout** is read by a custom pipe-reader (D3 replacement for the 256KB scanner): a `bufio.Reader.ReadString('\n')` loop that (a) writes each line verbatim to `RawWriter` before any parsing, (b) feeds the same bytes to `Parser.Parse`. Parsed events go to `Aggregator.Observe` and `Renderer.Render`; rendered display lines flow out through the original `sendLine` (so the file logger and TUI both see rendered output). Malformed lines are logged via `logger.Log` with the raw bytes prefixed `[malformed-json]` and skipped (D7).
     - **Stderr** keeps its current `bufio.Scanner` (256KB buffer) and forwards each line to `sendLine` with a `[stderr] ` prefix and to `logger.Log`. Stderr is **not** fed to `Parser.Parse` and **not** written to `RawWriter`.
   - A `defer pipeline.Close()` inside the wrapper guarantees `RawWriter` is flushed and closed on natural exit, terminator/SIGTERM path, and `cmd.Start()` failure path. Close is idempotent.
   - On step completion: call `Aggregator.Err()` — if non-nil, return it (D15). Otherwise call `Renderer.Finalize(Aggregator.Stats())` to emit the summary line (D13 2a) through the sendLine path, and set `lastCapture = Aggregator.Result()` (D6).
   - `runCommand`'s `forwardPipe` is unchanged — the JSON-awareness lives entirely in the wrapper above `runCommand`. Non-claude steps (`RunStep`, and claude-less sandbox callers like `sandbox create` smoke test) never construct the pipeline (D9).

5. **`ralph-tui/internal/workflow/run.go`** — Two changes in `Run`:
   - `buildStep` fills `SandboxOptions.ArtifactPath` for every claude step using the shape from D14 (`<projectDir>/logs/<runstamp>/<phase-prefix><NN>-<slug>.jsonl`). The phase prefix and the 2-digit step index come from the enclosing phase loop (`initialize-`, `iterNN-`, `finalize-`), and the slug is produced by `claudestream.Slug(step.Name)`. Sets `CaptureMode = CaptureResult` for every claude step.
   - Introduce a `RunStats` accumulator (sum of `claudestream.StepStats`). After each claude step returns successfully, the orchestrator reads `executor.LastStats()` (a new method that returns the `StepStats` from the most recent `RunSandboxedStep`, mirroring `LastCapture()`) and adds it into `RunStats`. At the end of the finalize phase, a single cumulative summary line is emitted via `executor.WriteToLog` (D13 2c). Because `WriteToLog` bypasses the file logger (see `docs/features/subprocess-execution.md`), the orchestrator **also** writes the cumulative summary through `log.Log("Run summary", ...)` so the cumulative total is persisted to disk.

6. **`ralph-tui/cmd/ralph-tui/main.go`** — Two changes:
   - Add `RunStamp` to `workflow.RunConfig` and populate it from `svc.log.RunStamp()`.
   - Create the per-run artifact directory (`projectDir/logs/<runstamp>/`) eagerly after `NewLogger` succeeds so later per-step file opens cannot race on directory creation. `MkdirAll` with mode 0o700 — same mode as the existing `logs/` directory.
7. **`ralph-tui/internal/logger/logger.go`** — Add `Logger.RunStamp() string` returning the basename captured in `NewLogger` (the `ralph-YYYY-MM-DD-HHMMSS` portion). Backed by a new unexported `runStamp` field set in `NewLogger` alongside the file creation. No behavior change to existing log-line formatting.

### Existing behavior unchanged (per scope decisions)

- Non-claude steps (`RunStep`): plain-text streaming, unchanged. (D9)
- File logger format: log-line prefix shape unchanged. (D9) — note: the log **filename** gains a millisecond component (D24), which is an unrelated precision fix.
- TUI drain batching and viewport scrolling behavior: unchanged. (D10) — ring buffer cap changes from 500 → 2000 (D22), which does not alter batching or viewport semantics.
- ralph-steps.json schema: unchanged. (D17)
- CLI flags, `{{VAR}}` language, `--version` output: unchanged. (D18)

### Test plan

- **`parser_test.go`** — Golden inputs covering: `system` init, `system` api_retry, `assistant` with each content-block type, `user` tool_result, `result` success, `result` is_error. Plus malformed-line cases (truncated JSON, unknown `type`, empty line) verifying `MalformedLineError`.
- **`render_test.go`** — Pure-function tests: each event type produces the expected display lines per D11/D12/D19. Parameterized table for the per-tool summary fallback (D12). Snapshot test for a multi-turn assistant message verifying the inter-turn blank line.
- **`aggregate_test.go`** — Sequence-driven tests: feed an event stream, assert `Result()`, `Stats()`, `Err()`. Specific cases: `is_error: true` → `Err()` returns; no `result` event → `Err()` returns the missing-result sentinel; success path → `Err()` returns nil and `Result()` returns the `.result` field.
- **`rawwriter_test.go`** — Verifies file is written verbatim (including malformed lines), file is opened with `O_TRUNC` on each open (retry overwrite), and is properly closed on the SIGTERM/cancellation path and on the `cmd.Start()` failure path (before any line is received).
- **`slug_test.go`** — Kebab-case conversion for representative step names (`Feature work`, `Fix review items`, `Close issue`, names with punctuation). Path-shape test for the `initialize-/iterNN-/finalize-` prefix assembler.
- **End-to-end** in `internal/workflow/`: a fake-claude harness (script that writes a canned NDJSON sequence to stdout and exits) drives `RunSandboxedStep` and asserts:
  - captured value bound to `LastCapture` matches `result.result` (D6);
  - rendered display lines match the expected text (D11/D12/D19);
  - JSONL file contents exactly match the script's stdout bytes;
  - error path for `is_error: true` surfaces through `RunSandboxedStep` (D15);
  - retry path overwrites the JSONL file (D14 retry behavior).
- **Logger test** — `Logger.RunStamp()` returns a value that, when used as a directory basename, produces a valid on-disk directory on macOS/Linux. Verify the millisecond component (D24) and that two `NewLogger` calls within the same second produce distinct RunStamps.
- **main.go smoke** — Extend existing startup test (if any) to assert the per-run artifact directory is created during startup.
- **Large-line stress test** (V1 regression guard) — Feed a 2MB single-line `tool_result` through the stdout pipe reader; assert `RawWriter` captures all bytes verbatim and `Parser.Parse` decodes successfully.
- **Stderr passthrough test** (D20) — Emit a mix of NDJSON stdout and plain-text stderr; assert stderr appears with `[stderr]` prefix in the log body and does NOT appear in the `.jsonl` artifact.
- **Continue vs retry test** (D21) — Drive an `is_error:true` result, then (a) choose `c`: assert JSONL preserved, `StepStats` folded into RunStats; (b) re-run and choose `r`: assert JSONL truncated on the second attempt, first attempt's `StepStats` discarded.
- **Crash-marker test** (D26) — Assert the `ralph_end` sentinel line is the last line of a successfully-completed step's `.jsonl`, and is absent when the step was terminated mid-stream.
- **Validator test delta** — The test for "captureAs on claude step is rejected" is deleted; a new test asserts a claude step with `captureAs: "RESULT"` passes validation.
- **Ring buffer capacity test** (D22) — Assert the viewport cap is 2000 after the change; feed 2500 lines and assert the first 500 scroll out while chrome banners emitted at line 400 remain visible.
- All tests run under `-race` per `docs/coding-standards/testing.md`.

### Documentation updates

- **New:** `docs/features/stream-json-pipeline.md` — Describes the `claudestream` package, the event flow, the renderer rules, and the JSONL artifact layout.
- **Update:** `docs/features/subprocess-execution.md` — Note that `RunSandboxedStep` now wraps `sendLine` with the claude pipeline.
- **Update:** `docs/features/variable-state.md` — Note that for claude steps, `captureAs` binds to `result.result` (not "last stdout line").
- **Update:** `docs/features/file-logging.md` — Update the filename format to include the millisecond component (D24); cross-reference to the new per-run `<timestamp>/` directory holding `.jsonl` artifacts.
- **Update:** `docs/features/config-validation.md` — Remove documentation of Rule A (captureAs-on-claude rejection) per D6.
- **Update:** `docs/features/tui-display.md` — Document the 2000-line ring buffer (D22) and the heartbeat indicator (D23).
- **Update:** `docs/architecture.md` — Add `claudestream` to the package dependency graph and update the data-flow section for claude steps.
- **Update:** `CLAUDE.md` — Add `docs/features/stream-json-pipeline.md` to the feature list.
- **Update:** `docs/how-to/debugging-a-run.md` — Mention the JSONL artifacts and how to consume them.
- **No new ADR required** — this change does not establish a new principle; it implements a tactical capability whose rationale is captured in this design doc.

## Open questions (not blocking implementation; re-review during PR)

- **O-1. Does `--permission-mode bypassPermissions` coexist cleanly with `--output-format stream-json --verbose`?** Evidence: both flags are documented in claude CLI's `cli-reference` as independent flags. No documented interaction. Plan assumes independence. If `bypassPermissions` suppresses some `system` init field (e.g., tool list), the parser already ignores unknown-typed lines (D7/D8). **Resolution (closed pre-implementation):** during development, run a manual smoke test inside the sandbox container (`docker run ... claude --permission-mode bypassPermissions --model <m> -p "say hello" --output-format stream-json --verbose`). Capture the NDJSON output and commit it as a golden fixture consumed by `parser_test.go`. This validates the flag interaction and replaces hand-written parser fixtures with real claude output, so future schema drift is caught by the same fixture when it is refreshed.
- **O-2. (Closed.)** Resolved by the D21 rewrite: every `RunSandboxedStep` return folds its `StepStats` into `RunStats` so the cumulative total matches real Anthropic spend. The finalize summary line labels the total with invocation and retry counts.
- **O-3. (Closed.)** Non-issue. `LoadSteps` is called exactly once at startup (`ralph-tui/cmd/ralph-tui/main.go:46`); the resulting step slice is the authoritative source of names, slugs, and indices for the run's entire duration. Mid-run edits to `ralph-steps.json` are not observed. Cross-run collisions are prevented by D24's millisecond `RunStamp`. Within-run collisions are prevented by the `<phase-prefix><NN>-<slug>.jsonl` shape — `NN` is the position within the phase, so even duplicate step names produce distinct filenames.
- **O-4. Per-step tolerance for `error_max_turns`.** D15 treats all `is_error: true` as failure. In practice, `error_max_turns` may carry a usable partial answer in `result.result` that some workflows would prefer to accept. **Deferred indefinitely.** Trigger to revisit: recurring `error_max_turns` on any claude step in the default or a user workflow (today no step even passes `--max-turns`, and claude's default ceiling is high). Implementation cost, when the time comes, is two changes — not one: (a) a `tolerateMaxTurns: true` per-step schema field in `ralph-steps.json`, and (b) aggregator logic so that when tolerated, `is_error` does **not** raise `Aggregator.Err()` and `result.result` is still bound via `captureAs`. The `c` continue-past-failure path (D21) does **not** cover this today because D15 returns before the capture is bound, so a downstream step after `c` would see an empty `{{CAPTURE}}`, not the partial answer.
- **O-5. `session_id` threading for multi-step claude workflows.** Deferred indefinitely. **Rationale is positive, not passive:** the default workflow deliberately hands off between claude steps via files (`test-plan.md`, `code-review.md`, `progress.txt`, `deferred.txt`) rather than in-memory session state. This gives the workflow three properties that in-memory threading would trade away — each step is individually restartable, human-inspectable, and loosely coupled. A future session-threading request must be evaluated against those properties; adopting `--resume` should be a conscious tradeoff, not a reflex. When the tradeoff is worth it, the touchpoints are: (a) expose the captured `session_id` via a new optional `captureSessionAs` field on the step, (b) teach `BuildRunArgs` to append `--resume <id>` when `{{SESSION_ID_FROM_PREV}}` is substituted into a prompt.

## Iteration review log

**Scope:** 3 iterations + agent validation performed 2026-04-14.

### Iteration 1 — assumption surfacing

| Assumption | Classification | Evaluation | Evidence / Action |
|---|---|---|---|
| `claude -p --output-format stream-json --verbose` produces NDJSON | Primary | Verified (from cited docs) | No code change |
| `BuildRunArgs` at `internal/sandbox/command.go:53-59` is the right injection point | Primary | Verified | `args = append(args, ImageTag, "claude", "--permission-mode", ..., "-p", prompt)` at those exact lines |
| `IsClaude` already gates dispatch | Primary | Verified | `stepDispatcher.RunStep` at `internal/workflow/run.go:36-41` |
| Per-run timestamp is shared with `<timestamp>.log` | Primary | **Refuted** — actual format is `ralph-YYYY-MM-DD-HHMMSS.log` (`internal/logger/logger.go:31`), not the ISO-ish example in the original D14 | **D14 layout rewritten** + new `Logger.RunStamp()` accessor + `RunConfig.RunStamp` field |
| RunSandboxedStep has enough context to pick the JSONL filename | Primary | **Refuted** — it only receives `stepName`; phase + iteration + index live in the orchestrator | **Wiring section rewritten**: `SandboxOptions` extended with `ArtifactPath` and `CaptureMode`; `buildStep` in `run.go` now populates them |
| `captureAs` for claude steps is currently used | Secondary | **Refuted** (verified against `ralph-tui/ralph-steps.json`) — no claude step sets `captureAs`, so D6 is a no-op for the default workflow | D6 annotated as non-breaking; no validator change required |
| `json.Decoder` ignores unknown fields by default | Primary | Verified (Go stdlib default) | D8 confirmed |
| Non-claude steps unchanged | Secondary | Verified | `stepDispatcher` dispatch is type-preserving |
| `WriteToLog` writes to the file logger | Primary | **Refuted** — per `docs/features/subprocess-execution.md`, `WriteToLog` bypasses the file logger. The cumulative summary line (D13 2c) must **also** go through `log.Log` to persist | **Step 5 rewritten** to dual-emit the run summary via both `WriteToLog` and `log.Log` |

### Iteration 2 — edge cases and failure modes

| Finding | Evaluation | Action |
|---|---|---|
| Retry behavior on the per-step JSONL file was unspecified (D14) | Gap | Added retry semantics: `O_TRUNC` on each open; prior attempt overwritten. Rationale recorded in D14 |
| `RunSandboxedStep` error path interacts with `WasTerminated()` (user-skip) — could synthesize a false positive is_error when user skips | Risk | D15 edge-case note added; wrapper returns aggregator error, but `runStepWithErrorHandling` checks `WasTerminated()` first (`internal/ui/orchestrate.go:64`) |
| `cmd.Start()` failure path — RawWriter must not panic if no lines ever arrive | Risk | Added test (`rawwriter_test.go`) covering start-failure path; `defer Close()` in wrapper already handles it |
| Aggregator tokens from failed retry attempt | Deferred | Recorded as O-2 |

### Iteration 4 — agent validation (evidence-based-investigator + adversarial-validator)

| Finding | Source | Disposition | Plan change |
|---|---|---|---|
| E10: `validator.go:281-290` already rejects `captureAs` on claude steps (plan's D6 wrongly said no validator change needed) | evidence-investigator | Accepted | **D6 rewritten** to require removal of Rule A in validator + associated test |
| V1: 256KB `bufio.Scanner` buffer is insufficient for single NDJSON lines carrying large `tool_result`/`Write`/`Edit` payloads; `ErrTooLong` would silently truncate the stream and trick D15 into "no result arrived" | adversarial-validator | Accepted | **D3 rewritten**: switch claude stdout to `bufio.Reader.ReadString('\n')` with 64MB hard cap + truncation sentinel; non-claude stays 256KB scanner |
| V2: RawWriter fed from scanner output would inherit the truncation, breaking "verbatim" guarantee | adversarial-validator | Accepted | Covered by D3 replacement — `RawWriter` now writes raw bytes from the new pipe-reader before parsing; verbatim guarantee holds up to the 64MB cap |
| V3: stderr lines piped through the parser would all become `MalformedLineError` and be silently swallowed, hiding docker/claude diagnostics | adversarial-validator | Accepted | **New D20** added: stdout feeds the parser; stderr bypasses it and is tagged `[stderr]` in the log body |
| V4: `c` (continue past failure) semantics for JSONL retention and `RunStats` inclusion were unspecified | adversarial-validator | Accepted | **New D21** added: `c` preserves JSONL + folds tokens into cumulative; `r` truncates JSONL + excludes failed-attempt tokens. Summary line labels the total |
| V5: `error_max_turns` with a partial answer is unconditionally treated as failure (no per-step opt-in) | adversarial-validator | Acknowledged, deferred | Added to Open Questions as O-4 (per-step tolerance flag if this bites in practice) |
| V6: `RunStats` concurrency is unspecified | adversarial-validator | Accepted | **New D25** added: single-goroutine invariant documented + comment at the struct declaration |
| V7: 500-line ring buffer will overflow with multi-turn claude output, hiding phase banners | adversarial-validator | Accepted | **New D22** added: raise ring buffer to 2000. Cost is a small constant memory increase |
| V8: Silent "feels alive" gap during long turns without `--include-partial-messages` | adversarial-validator | Accepted | **New D23** added: passive `⋯ thinking (Ns)` heartbeat in the status header after 15s of silence |
| V9: Host crash mid-step leaves truncated JSONL with no way to detect | adversarial-validator | Accepted | **New D26** added: trailing `{"type":"ralph_end","ok":true,"schema":"v1"}` sentinel line after the `result` event |
| V10: Same-second run collisions overwrite prior artifacts; `MkdirAll` mode claim is not consistent for pre-existing `logs/` | adversarial-validator | Accepted | **New D24** added: RunStamp gains millisecond precision; `docs/features/file-logging.md` update added to the docs list |
| V11: session_id threading is reversible but the exact touchpoints weren't documented | adversarial-validator | Accepted | D16's "Reversible" clause — noted via O-1 path (captureAs target + `--resume` in `BuildRunArgs`). Added to Open Questions as O-5 |

### Iteration 3 — consolidation and test-plan completeness

| Finding | Evaluation | Action |
|---|---|---|
| Internal overlap: Renderer and Aggregator both observe events | Intentional | Separation preserves single-responsibility: Renderer emits display lines, Aggregator owns terminal state + captureAs result |
| External overlap: no existing NDJSON parser in ralph-tui | Verified (grepped `encoding/json` usage: only `steps.LoadSteps` uses it for static config) | No consolidation available |
| Test plan missed: retry overwrite, start-failure close, slug assembly, RunStamp format | Gap | Added `slug_test.go`, expanded `rawwriter_test.go`, added logger RunStamp test and e2e retry assertion |
| Stability assessment | Low structural churn expected in iteration 4 | **Stop iterating**; proceed to agent validation |

## Out of scope

- Switching non-claude steps to JSON.
- Removing the file logger or changing its format.
- Persisting analytics to a database. (We may write JSONL files; consumption is future work.)
- The `--include-partial-messages` token-streaming UX.
- Multi-session resumption / `--continue` / `--resume` integration.
