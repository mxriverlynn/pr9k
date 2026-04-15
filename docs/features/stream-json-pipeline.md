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

## Wiring (planned)

The claudestream package is complete and tested, but not yet wired into the
workflow layer. The planned integration points are:

- `internal/sandbox/command.go` — append `--output-format stream-json --verbose` to claude invocations
- `internal/workflow/` — route `IsClaude == true` stdout through `Pipeline.Observe`; use `Pipeline.Aggregator().Result()` for `captureAs`; call `Pipeline.Close()` and check `Pipeline.WriteErr()` after each step
- `internal/logger/` — millisecond-precision `RunStamp` for artifact directory naming
- Artifact directory: `logs/<run-timestamp>/<phase-prefix>-<slug>.jsonl`

See `docs/plans/streaming-json-output/design.md` for the full integration design.
