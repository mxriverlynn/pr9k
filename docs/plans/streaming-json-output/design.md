# Streaming JSON Output from Claude — Design Plan

**Status:** Ready for implementation (all questions resolved)
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

### D3. Parse NDJSON line-by-line; one JSON object per line
- **Why:** Documented behavior. Existing scanner already line-splits with a 256KB buffer (subprocess-execution.md:182) — sufficient for any single JSON message.

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
- **Layout:**
  ```
  logs/
    2026-04-14T17-30-22.log               # existing human-readable log (unchanged)
    2026-04-14T17-30-22/                  # NEW per-run directory for raw artifacts
      initialize-get-issue.jsonl          # only present for claude steps
      iter01-feature-work.jsonl
      iter01-test-planning.jsonl
      iter02-feature-work.jsonl
      finalize-lessons-learned.jsonl
  ```
- **File contents:** Verbatim NDJSON output from `claude -p --output-format stream-json --verbose`. No wrapper. No re-encoding. One file per claude step invocation.
- **Step slug:** kebab-case of the step name (e.g., `Feature work` → `feature-work`).
- **Iteration prefix:** `iterNN-` (zero-padded to 2 digits) for iteration-phase steps. `initialize-` and `finalize-` prefixes for those phases.
- **Non-claude steps:** No `.jsonl` file is created.
- **Writer:** Owned by `internal/claudestream` (a `RawWriter` opened per claude-step invocation, closed when the step exits — including on terminator/SIGTERM paths).
- **Why:** Preserves every byte claude emits — required for the user's stated future analytics goal. Per-step granularity means a downstream tool can read one file at a time without parsing run-level boundaries. Mirrors the existing per-run timestamp convention from `docs/features/file-logging.md`.

### D15. `result.is_error == true` is treated as step failure
- **Behavior:** When a claude step's `result` message has `is_error: true`, the `RunSandboxedStep` call returns a non-nil error (synthesized by the `Aggregator`) even if the docker subprocess exited 0. This triggers the existing error-mode interactive recovery (`c`/`r`/`q`) per `docs/features/keyboard-input.md`.
- **Error message:** Includes `subtype` and `stop_reason` from the result message (e.g., `claude step ended with is_error=true: subtype=error_max_turns, stop_reason=max_turns`).
- **Edge case — no `result` message ever arrives** (claude crashed before emitting one): treated as failure too. The `Aggregator` returns an error if it never observed a `result` event and the subprocess exited 0.
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

3. **`ralph-tui/internal/sandbox/command.go:53-59`** — Append `"--output-format", "stream-json", "--verbose"` to the claude argv. (D1)

4. **`ralph-tui/internal/workflow/workflow.go`** — `RunSandboxedStep` becomes claude-aware:
   - Construct a per-step `claudestream.Pipeline` (containing `Parser`, `Renderer`, `Aggregator`, `RawWriter`).
   - Wrap the existing `sendLine` callback: each raw line is fed first to `RawWriter.Write`, then to `Parser.Parse`. Successful events go to `Aggregator.Observe` and then `Renderer.Render`; the resulting display lines are forwarded to the original `sendLine`. Malformed lines log + skip (D7).
   - On step completion: close `RawWriter`, then call `Aggregator.Err()`. If non-nil, return it (D15). Otherwise let `Aggregator.Result()` populate `LastCapture` (D6).
   - Token/cost summary line (D13) is rendered by `Renderer.Finalize(stats)` after the result event.
   - `runCommand`'s `forwardPipe` is unchanged — the JSON-awareness lives entirely in the wrapper, so non-claude steps (`RunStep`) are untouched (D9).

5. **`ralph-tui/internal/workflow/run.go`** — `Run` adds a `RunStats` accumulator that sums each step's `claudestream.StepStats` and prints the cumulative summary line in the finalize phase (D13, 2c).

6. **`ralph-tui/cmd/ralph-tui/main.go`** — No structural change; the per-step JSONL file's parent directory (`logs/<run-timestamp>/`) is created at run start alongside the existing log file. Path is passed down to `claudestream.Pipeline` via the existing dependency chain.

### Existing behavior unchanged (per scope decisions)

- Non-claude steps (`RunStep`): plain-text streaming, unchanged. (D9)
- File logger format: unchanged. (D9)
- TUI ring buffer, drain batching, viewport: unchanged. (D10)
- ralph-steps.json schema: unchanged. (D17)
- CLI flags, `{{VAR}}` language, `--version` output: unchanged. (D18)

### Test plan

- **`parser_test.go`** — Golden inputs covering: `system` init, `system` api_retry, `assistant` with each content-block type, `user` tool_result, `result` success, `result` is_error. Plus malformed-line cases (truncated JSON, unknown `type`, empty line) verifying `MalformedLineError`.
- **`render_test.go`** — Pure-function tests: each event type produces the expected display lines per D11/D12/D19. Parameterized table for the per-tool summary fallback (D12). Snapshot test for a multi-turn assistant message verifying the inter-turn blank line.
- **`aggregate_test.go`** — Sequence-driven tests: feed an event stream, assert `Result()`, `Stats()`, `Err()`. Specific cases: `is_error: true` → `Err()` returns; no `result` event → `Err()` returns the missing-result sentinel; success path → `Err()` returns nil and `Result()` returns the `.result` field.
- **`rawwriter_test.go`** — Verifies file is written verbatim (including malformed lines) and is properly closed on the SIGTERM/cancellation path.
- **End-to-end** in `internal/workflow/`: a fake-claude harness (script that writes a canned NDJSON sequence to stdout and exits) drives `RunSandboxedStep` and asserts the captured value, the rendered display lines, the JSONL file contents, and the error path for `is_error: true`.
- All tests run under `-race` per `docs/coding-standards/testing.md`.

### Documentation updates

- **New:** `docs/features/stream-json-pipeline.md` — Describes the `claudestream` package, the event flow, the renderer rules, and the JSONL artifact layout.
- **Update:** `docs/features/subprocess-execution.md` — Note that `RunSandboxedStep` now wraps `sendLine` with the claude pipeline.
- **Update:** `docs/features/variable-state.md` — Note that for claude steps, `captureAs` binds to `result.result` (not "last stdout line").
- **Update:** `docs/features/file-logging.md` — Cross-reference to the new per-run `<timestamp>/` directory holding `.jsonl` artifacts.
- **Update:** `docs/architecture.md` — Add `claudestream` to the package dependency graph and update the data-flow section for claude steps.
- **Update:** `CLAUDE.md` — Add `docs/features/stream-json-pipeline.md` to the feature list.
- **Update:** `docs/how-to/debugging-a-run.md` — Mention the JSONL artifacts and how to consume them.
- **No new ADR required** — this change does not establish a new principle; it implements a tactical capability whose rationale is captured in this design doc.

## Out of scope

- Switching non-claude steps to JSON.
- Removing the file logger or changing its format.
- Persisting analytics to a database. (We may write JSONL files; consumption is future work.)
- The `--include-partial-messages` token-streaming UX.
- Multi-session resumption / `--continue` / `--resume` integration.
