# Stream JSON Pipeline

The `internal/claudestream` package parses, renders, aggregates, and persists
the newline-delimited JSON (NDJSON) stream emitted by `claude -p --output-format
stream-json --verbose`. It is designed as a self-contained unit that sits
between the raw subprocess stdout and the TUI display layer.

The package is composed of six types — Parser, Renderer, Aggregator, RawWriter,
Slug, and Pipeline — plus the event types they operate on. All components are
single-goroutine (the stdout-forwarding goroutine owns them); they are not safe
for concurrent use by multiple writers.

**Package:** `internal/claudestream/`

## Event types (`event.go`)

The stream produces five known top-level event types, each unmarshalled into
its own struct:

| Go type | JSON `type` field | Purpose |
|---|---|---|
| `*SystemEvent` | `"system"` | Session init (`subtype: "init"`) and API retry notifications (`subtype: "api_retry"`) |
| `*AssistantEvent` | `"assistant"` | One complete assistant turn, including `content` blocks (text, tool_use, thinking) and token usage |
| `*UserEvent` | `"user"` | Tool results fed back to the model; not rendered |
| `*ResultEvent` | `"result"` | Final event; carries the step result text, `is_error`, session ID, cost, and cumulative usage |
| `*RateLimitEvent` | `"rate_limit_event"` | Rate-limit status; rendered only when `status != "allowed"` |

All five implement the `Event` interface (single unexported method `eventType()`).

`ContentBlock` within an `AssistantEvent` is a discriminated union on the `"type"` field. Populated fields vary by subtype: `Text` for `"text"` blocks, `Name`/`Input` for `"tool_use"` blocks, `ToolUseID`/`Content` for `"tool_result"` blocks.

`RateLimitInfo` uses camelCase JSON field names (matching upstream claude CLI output), unlike all other types in the package which use snake_case.

`StepStats` accumulates timing and usage across a single step:

```go
type StepStats struct {
    NumTurns, InputTokens, OutputTokens int
    CacheCreationTokens, CacheReadTokens int
    TotalCostUSD  float64
    DurationMS    int64
    SessionID     string
    LastRateLimitInfo *RateLimitInfo
}
```

## Parser (`parser.go`)

`Parser.Parse(line []byte) (Event, error)` converts one raw NDJSON line to a typed `Event`.

Dispatch:
1. Empty line → `*MalformedLineError` with `Msg: "empty line"`
2. Invalid JSON → `*MalformedLineError` with `Msg: "invalid JSON"`
3. Missing `"type"` field → `*MalformedLineError` with `Msg: "missing type field"`
4. Known type → fully unmarshals into the appropriate struct
5. Unknown type → `*MalformedLineError` with `Msg: "unknown type <value>"`

`MalformedLineError` preserves the raw bytes in its `Raw` field so callers can
log them without re-reading. Unknown sibling fields on known event types are
silently ignored (standard `encoding/json` behaviour).

## Renderer (`render.go`)

`Renderer.Render(ev Event) []string` converts one typed event to zero or more
human-readable display lines for the TUI log panel. Rules:

| Event | Output |
|---|---|
| `*SystemEvent` subtype `"init"` | `[claude session <id> started, model <model>]` |
| `*SystemEvent` subtype `"api_retry"` | `⚠ retry N/M in Xms — <error>` |
| `*SystemEvent` other subtypes | nothing |
| `*AssistantEvent` | blank separator line before 2nd+ turns; then per content block: text split on `\n`, tool_use as `→ <Name> <summary>`, thinking dropped |
| `*UserEvent` | nothing |
| `*ResultEvent` | nothing |
| `*RateLimitEvent` status `"allowed"` | nothing |
| `*RateLimitEvent` other status | `⚠ rate limit <type>: <status> (resets HH:MM:SS)` |

`Renderer.Finalize(stats StepStats) []string` returns the single closing
summary line after a step completes:

```
<turns> turns · <in>/<out> tokens (cache: <creation>/<read>) · $<cost> · <duration>
```

`Renderer.FinalizeRun(invocations, retries int, total StepStats) []string`
returns the run-level cumulative summary line (D13 2c). Returns nil when
`invocations == 0` (no claude steps ran):

```
total claude spend across N step invocations[ (including R retries)]: <turns> turns · <in>/<out> tokens (cache: <creation>/<read>) · $<cost> · <duration>
```

The retries parenthetical is omitted when `retries == 0`. `FinalizeRun` is a
value-receiver method that uses no `Renderer` state; it can be called on a
zero-value `Renderer{}` at the `Run()` call site.

### Tool summary (`toolSummary`)

For `tool_use` content blocks, `toolSummary(name, input)` extracts the most
useful field from the tool's JSON input and truncates to 80 runes (appending
`"…"` if clipped). Per-tool field selection:

| Tool | Field |
|---|---|
| `Bash` | `command` |
| `Read`, `Edit`, `Write`, `NotebookEdit` | `file_path` |
| `Glob`, `Grep` | `pattern` |
| `Task`, `Agent` | `description` |
| `WebFetch` | `url` |
| other | compact JSON of full input |

If the selected field is absent, falls back to compact JSON. If the field
value is not a JSON string, `strings.Trim` is used before truncation.

## Aggregator (`aggregate.go`)

`Aggregator.Observe(ev Event)` folds one event into running state. `nil`
events fall through the type switch silently.

- `*AssistantEvent`: adds per-turn token counts to the running tally.
- `*ResultEvent`: overwrites all fields (result text, stats, error flags) with
  the authoritative cumulative values from the result event. Token tallies from
  individual assistant events are discarded in favour of the result event's
  `usage` field.
- `*RateLimitEvent`: stores `RateLimitInfo` pointer in `StepStats.LastRateLimitInfo`.
- Other events: no-op.

Post-step inspection:

```go
agg.Result()  // string — result.result field (captureAs semantics)
agg.Stats()   // StepStats
agg.Err()     // nil, or error if is_error==true or no result event seen
```

`Err()` returns non-nil in two cases:
1. `result.is_error == true` — message includes a ≤200-rune snippet of the
   result text, session ID, subtype, and stop reason for log correlation.
2. No result event was observed (stream truncated) — returns `"claude step produced no result event"`.

## RawWriter (`rawwriter.go`)

`NewRawWriter(path string) (*RawWriter, error)` opens `path` with
`O_CREATE|O_TRUNC|O_WRONLY` and mode `0o600`. A retry invocation therefore
overwrites the prior attempt's bytes. Returns a wrapped error containing the
path on failure.

`RawWriter.WriteLine(b []byte)` appends the verbatim bytes followed by `'\n'`.
Uses a `bufio.Writer` for throughput; does not fsync per line (crash resilience
is provided by the sentinel line written by `Pipeline`).

`RawWriter.Close()` flushes buffered data and closes the file. Idempotent:
subsequent calls return `nil`. All Write/Flush/Close errors include the file
path per the error-handling coding standard.

## Slug (`slug.go`)

`Slug(name string) string` converts a step name to a kebab-case identifier
suitable for use in `.jsonl` filenames:

- Lowercased.
- Runs of non-alphanumeric characters (spaces, punctuation, unicode
  non-letters/non-digits) replaced by a single `"-"`.
- Leading and trailing `"-"` trimmed.

Examples: `"Feature work"` → `"feature-work"`, `"Fix review items"` →
`"fix-review-items"`.

## Pipeline (`pipeline.go`)

`Pipeline` composes Parser + Renderer + Aggregator + RawWriter behind a single
entry point. It is the main integration surface for the workflow layer.

```go
p := claudestream.NewPipeline(rawWriter) // rawWriter may be nil
```

`NewPipeline(rawWriter *RawWriter)` — `rawWriter` may be `nil` to disable
persistence (useful in tests that do not want to touch disk).

### Observe

`Pipeline.Observe(line []byte) []string` processes one raw NDJSON line:

1. Writes verbatim bytes to `RawWriter` (if non-nil); stores first write error via `WriteErr()`.
2. Stamps `lastEventAt` atomically (even for malformed lines — any activity counts).
3. Parses the line; returns `nil` on `MalformedLineError` (caller is responsible for logging).
4. Folds the event into the `Aggregator`.
5. On `ResultEvent`: writes the sentinel line `{"type":"ralph_end","ok":true,"schema":"v1"}` to the `RawWriter` for crash-resilience.
6. Returns `Renderer.Render` output (zero or more display lines).

### Other methods

| Method | Returns | Purpose |
|---|---|---|
| `LastEventAt()` | `time.Time` | Wall-clock time of most recent line; zero value if none observed. Read concurrently by heartbeat goroutine (atomic). |
| `Aggregator()` | `*Aggregator` | Post-step result inspection and `captureAs` binding |
| `Renderer()` | `*Renderer` | Access for `Finalize` calls after the step ends |
| `Close()` | `error` | Flush/close `RawWriter`; idempotent; no-op if `rawWriter` is nil |
| `WriteErr()` | `error` | First `RawWriter` write error, or nil if all writes succeeded |

### Sentinel line

After the `ResultEvent` is written, the Pipeline appends:

```json
{"type":"ralph_end","ok":true,"schema":"v1"}
```

Downstream tooling can check for this line to detect whether an artifact was
written completely (i.e., the process did not crash mid-step).

## Wiring

The claudestream package is wired into the workflow layer at the following
integration points (completed in issues #89–#91):

### sandbox/command.go (D1)

`BuildRunArgs` appends `--output-format stream-json --verbose` to every claude
CLI invocation. This ensures all claude subprocess output is NDJSON.

### workflow/workflow.go (D6, D14, D15)

`RunSandboxedStep` constructs a `Pipeline` for every step where
`opts.CaptureMode == ui.CaptureResult`:

1. Opens a `RawWriter` at `opts.ArtifactPath` (if non-empty) for per-step JSONL
   persistence. Open failure is logged as `[artifact] open failed: …` but does
   not abort the step (persistence is best-effort).
2. Constructs `claudestream.NewPipeline(rw)` — `rw` may be `nil`.
3. Passes the pipeline to `runCommand`. The claude-aware stdout path (activated
   when pipeline is non-nil) uses `bufio.Reader.ReadString('\n')` with no 256 KB
   line cap, feeding each raw line to `pipeline.Observe`. Stderr uses a 256 KB
   scanner with `[stderr] ` prefix.
4. After the subprocess exits, folds `pipeline.Aggregator().Stats()` into
   `lastStats` so callers can retrieve them via `LastStats()`.
5. Checks `pipeline.Aggregator().Err()` (D15) before the subprocess exit code —
   if the aggregator reports `is_error == true` or no result event was seen, the
   step fails with the aggregator's error.
6. On success, emits `pipeline.Renderer().Finalize(stats)` summary line through
   `sendLine` and the file logger (D13 2a).
7. Binds `pipeline.Aggregator().Result()` into `lastCapture` (D6) for `captureAs`
   downstream.
8. Defers `pipeline.Close()` and logs any `pipeline.WriteErr()` as
   `[artifact] write error: …` (M1).

Non-pipeline invocations (`CaptureMode != CaptureResult`) take the existing
256 KB dual-scanner path unchanged.

### workflow/run.go (D14, D21)

`artifactPath` computes the per-step `.jsonl` path as:

```
<projectDir>/.pr9k/logs/<runStamp>/<phasePrefix><stepIdx02d>-<slug>.jsonl
```

Phase prefixes: `initialize-`, `iter<NN>-` (1-indexed), `finalize-`. Returns `""`
when `cfg.RunStamp == ""` (persistence disabled) or the step is not a claude step.

For every `IsClaude == true` step, `Run` sets:
- `resolved.ArtifactPath = artifactPath(…)`
- `resolved.CaptureMode = ui.CaptureResult`

These flow through `stepDispatcher` into `SandboxOptions` for each
`RunSandboxedStep` call.

`runStats` accumulates `StepStats` across all claude step invocations in a run
(D21). `stepDispatcher.RunStep` calls `executor.LastStats()` after every
`RunSandboxedStep` return (including retries and error paths) and folds the
stats via `rs.add(stats, isRetry)`. `prevFailed` on the dispatcher tracks
whether the prior call ended in error so retry invocations are counted
separately.

After the finalize phase, `Run` calls `Renderer.FinalizeRun(rs.invocations,
rs.retries, rs.total)` and writes each returned line via
`executor.WriteRunSummary` (D13 2c). `WriteRunSummary` sends the line to both
the TUI (via `sendLine`) and the file logger so the cumulative total is
persisted to disk; if the logger write fails, a `[log] run summary write
failed: …` error line is sent to the TUI so disk failures are visible to the
operator. No summary is emitted when `rs.invocations == 0`.

### logger/logger.go (D24)

`Logger.RunStamp()` returns the log basename minus `.log` (e.g.
`"ralph-2026-04-14-173022.123"`). `main.go` passes `log.RunStamp()` as
`workflow.RunConfig.RunStamp`. Millisecond precision prevents two rapid
successive runs from sharing an artifact directory.

### cmd/src/main.go

`startup()` creates the per-run artifact directory
(`<projectDir>/.pr9k/logs/<runStamp>/`) via `os.MkdirAll(0o700)` immediately after
`NewLogger` succeeds. Directory creation failure logs to stderr and aborts
startup, consistent with the existing logger-failure path.

See `docs/plans/streaming-json-output/design.md` for the original integration
design doc.
