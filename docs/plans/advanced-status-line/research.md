# Advanced Status Line — Feasibility Research

**Status:** Draft complete, adversarially validated, Option 1 re-investigated (scratch-config-dir variant)
**Date:** 2026-04-19
**Question:** Can pr9k's custom status line expose the same rich data that Claude Code feeds to its own native `statusLine` scripts — model, session id, workspace, cost, tokens, rate limits, etc.?

---

## 1. Executive Summary

**Feasibility verdict:** Yes, fully. The original draft rated Option 1 as "defer — high risk" because it assumed pr9k would have to merge into the user's real `~/.claude/settings.json`. A follow-up investigation found that `CLAUDE_CONFIG_DIR` is already unconditionally overridden at container launch (`src/internal/sandbox/command.go:44`) and Claude's credentials live in a single file (`.credentials.json`). This opens a **scratch-config-dir design** — pr9k bind-mounts a scratch `CLAUDE_CONFIG_DIR` that it fully owns, and file-bind-mounts just `.credentials.json` from the user's real profile. pr9k never writes into the user's real `~/.claude`, the settings.json merge problem disappears, and Option 1 becomes shippable at **medium** cost rather than high.

**Options explored (four, plus a late-discovered 2a; Option 1 revised post-investigation):**

| # | Option | Fidelity | Complexity | Risk | Verdict |
|---|--------|----------|------------|------|---------|
| 2a | Surface `Renderer.Finalize` (already generated per step) as a summary string in pr9k's payload | ~60% of operator value (turns, tokens, cost, duration, session id) but **post-step only** | **Very low** | **Very low** — uses only existing public accessors | **Recommended first step** |
| 2 | Derive-from-stream — extend `Aggregator` to track Model and live SessionID, add concurrent-safe snapshot, expose via payload | ~80% (live tokens, plus fields added by parser extension; cost still post-step) | Medium | Medium — aggregator parser changes + concurrency discipline | Recommended second step |
| 1 (original) | Capture-and-forward, writing into the user's real `~/.claude/settings.json` | **100%** | **High** | **High** — parse-merge, atomic write, permissions, rollback | Superseded by 1′ |
| 1′ (revised) | Capture-and-forward using a pr9k-owned scratch `CLAUDE_CONFIG_DIR` + file-bind-mount of `.credentials.json` + shim baked into a pr9k-derived Docker image + Unix-socket IPC on the project bind-mount | **100%** | **Medium** | **Medium** — gated by Q-OPEN-1 (does Claude read adjacent state files?) and Q-OPEN-5 (Unix socket over Docker Desktop macOS bind-mount) | **Recommended third step, promoted from "defer"** |
| 3b | Claude Agent SDK | Matches Option 2 roughly | Very high | Very high — no Go Agent SDK; pr9k doesn't own the container image | **Do not pursue** |

**Recommendation — in priority order:**

1. **Ship Option 2a first.** Add `Runner.LastClaudeSummary() string` backed by the existing `Renderer.Finalize` output, surface it as a `claude.last_summary` string in the stdin payload, and update the sample script to display it. This is ~20 lines of Go plus docs, uses only code that already runs and is already tested, and needs zero concurrency work. Users immediately get *"turns · tokens (cache) · $cost · duration"* on their status line after each Claude step.
2. **Then ship Option 2 as an enhancement** once the cheap win is in. Extend `Aggregator.Observe` to capture `SessionID`/`Model` from `SystemEvent{subtype:"init"}` (today both are only observed from `ResultEvent`), add a `Model` field to `StepStats`, add a concurrent-safe `Snapshot()`, and surface typed fields (`claude.session_id`, `claude.model`, `claude.input_tokens`, ...). This is the point at which pr9k provides *live* in-flight numbers during a running step.
3. **Ship Option 1′ (scratch-config-dir variant) as an opt-in third phase.** Now viable at medium cost given the scratch-dir design — no parse-merge, no atomic-write, no user-profile rollback. Gate behind `statusLine.captureClaudeStatusLine: true` in `config.json`. Ship a pr9k-owned Dockerfile so the shim is baked in at `/usr/local/bin/pr9k-statusline-shim` with mode 0755, and use a Unix-domain socket on the project bind-mount (`<projectDir>/.pr9k/statusline.sock`) for IPC. Resolve Q-OPEN-1 (profile adjacent-file reads) and Q-OPEN-5 (Docker Desktop Unix-socket reliability) before committing.
4. **Do not pursue Option 3b (Agent SDK).** The Agent SDK is Python-only; pr9k is Go. Adopting it would either force a rewrite or require pr9k to own a derived container image — and once we own the image for Option 1′, the Agent SDK still has no Go surface, so the rewrite cost stays.

Sections 11 (adversarial validation) and 6 (Option 1 variants and optimal design) together contain the reasoning that moved Option 1 from "defer" to "third phase."

---

## 2. Background

### 2.1 pr9k's current status line

pr9k's `internal/statusline.Runner` (`/Users/mxriverlynn/dev/mxriverlynn/pr9k/src/internal/statusline/statusline.go:50-78`, **E1**) manages a user-supplied `command` that is invoked on a refresh tick (and on explicit `Trigger()` calls from the workflow). pr9k builds a JSON object, pipes it to the command's stdin, captures up to 8 KB of stdout, and displays whatever the script prints.

The current stdin JSON payload, per `statusline.BuildPayload` (`src/internal/statusline/payload.go:5-50`, **E4**):

```json
{
  "sessionId":     "<pr9k run timestamp>",
  "version":       "<pr9k version>",
  "phase":         "initialize|iteration|finalize",
  "iteration":     1,
  "maxIterations": 5,
  "step":          { "num": 3, "count": 9, "name": "feature-work" },
  "mode":          "normal|error|...",
  "workflowDir":   "/.../workflow",
  "projectDir":    "/.../target-repo",
  "captures":      { "ISSUE_NUMBER": "42", ... }
}
```

Notable: `sessionId` here is pr9k's own run stamp — **not** Claude's session id.

Per `docs/coding-standards/versioning.md:13-28`, pr9k's public API is: (1) CLI flags & exit codes, (2) `config.json` schema, (3) `{{VAR}}` substitution language, (4) `--version` output. The statusLine stdin payload is explicitly *not* in that list — it is documented (`docs/features/status-line.md:66-98` states "All fields are always present") but not versioned. This matters in Section 7: adding `claude` to the payload does not need a MAJOR bump under the project's own rules, but it does imply a documentation update and a schema-surface decision (see V7 below for the constraint to resolve).

### 2.2 Claude's native statusLine

Claude Code has its own `statusLine` feature, configured via `~/.claude/settings.json` (or project-local `.claude/settings.json`). The full stdin payload Claude feeds to a registered script, per https://code.claude.com/docs/en/statusline.md#available-data:

```json
{
  "cwd": "/current/working/directory",
  "session_id": "abc123...",
  "session_name": "my-session",
  "transcript_path": "/path/to/transcript.jsonl",
  "model": { "id": "claude-opus-4-7", "display_name": "Opus" },
  "workspace": {
    "current_dir": "/current/working/directory",
    "project_dir": "/original/project/directory",
    "added_dirs": [],
    "git_worktree": "feature-xyz"
  },
  "version": "2.1.90",
  "output_style": { "name": "default" },
  "cost": {
    "total_cost_usd": 0.01234,
    "total_duration_ms": 45000,
    "total_api_duration_ms": 2300,
    "total_lines_added": 156,
    "total_lines_removed": 23
  },
  "context_window": {
    "total_input_tokens": 15234,
    "total_output_tokens": 4521,
    "context_window_size": 200000,
    "used_percentage": 8,
    "remaining_percentage": 92,
    "current_usage": {
      "input_tokens": 8500,
      "output_tokens": 1200,
      "cache_creation_input_tokens": 5000,
      "cache_read_input_tokens": 2000
    }
  },
  "exceeds_200k_tokens": false,
  "rate_limits": {
    "five_hour":  { "used_percentage": 23.5, "resets_at": 1738425600 },
    "seven_day":  { "used_percentage": 41.2, "resets_at": 1738857600 }
  },
  "vim":      { "mode": "NORMAL" },
  "agent":    { "name": "security-reviewer" },
  "worktree": { "name": "my-feature", ... }
}
```

Claude invokes the script "after each new assistant message, when the permission mode changes, or when vim mode toggles. Updates are debounced at 300 ms." A user may also set `refreshInterval` (seconds, min 1) for a timer-driven refresh.

### 2.3 The gap pr9k wants to close

pr9k wants its own statusLine scripts to have access to rich Claude-native fields so a user script can render a meaningful "what is Claude doing right now" display instead of only workflow metadata.

---

## 3. Option 2a — Surface `Renderer.Finalize` as a summary string (Recommended first step)

**Premise:** pr9k already formats and emits a per-step summary line via `internal/claudestream.Renderer.Finalize`. Expose that line as a string in pr9k's stdin payload. Zero parser changes, zero concurrency work.

### 3.1 The free data

`src/internal/claudestream/render.go:51-64` — `Renderer.Finalize(stats StepStats)` returns a single formatted line: `"<turns> turns · <in>/<out> tokens (cache: <c>/<r>) · $<cost> · <duration>"`. Today it is written to the log at `src/internal/workflow/workflow.go:379-385` after each Claude step. `Renderer.FinalizeRun` at `render.go:74-103` produces the run-level cumulative summary in the same format.

`src/internal/workflow/workflow.go:702-706` (**E28**) already exposes a post-step `LastStats()` accessor that holds the `StepStats` for the most recent Claude step. Adding a peer `LastClaudeSummary() string` that returns the formatted line (or the caller can call `renderer.Finalize(runner.LastStats())`) is trivial.

### 3.2 What it delivers

The summary line covers: `num_turns`, `input_tokens`, `output_tokens`, `cache_creation_tokens`, `cache_read_tokens`, `total_cost_usd`, `duration_ms`. All the numeric fields that appear in the native payload's `cost` and `context_window` sub-objects except for Claude-computed percentages and lines added/removed.

This is the post-step, cumulative-for-the-step view. It does not tell the user anything during a running step, but it does tell them everything about the *last completed* Claude step — which is what a typical status-line use case (after-action snapshot) actually wants.

### 3.3 Integration into pr9k

One interface method added to the `StatusRunner` interface or passed via a new getter on the statusline runner. One field added to the payload JSON:

```json
"claude": { "last_summary": "3 turns · 12,456/3,211 tokens (cache: 900/45,678) · $0.0421 · 00:37.2" }
```

Changes:
- `internal/workflow/run.go` — extend `StatusRunner` interface with `PushClaudeSummary(string)` (or add a getter `SetClaudeSummaryGetter(...)`).
- `internal/statusline/state.go` — add `ClaudeLastSummary string` field.
- `internal/statusline/payload.go` — marshal it as `"claude": { "last_summary": ... }` when non-empty.
- Call site: after `workflow.RunSandboxedStep` returns successfully, assemble `renderer.Finalize(lastStats)` and push it.
- `workflow/scripts/statusline` — display the line.
- `docs/features/status-line.md` — document the new field.

### 3.4 Fidelity and limitations

- Fidelity: ~60% of operator-facing value, 100% of what pr9k currently computes.
- Limitation: post-step only. User does not see anything change during a running step — the status line remains on the *previous* step's summary until the current step completes.
- Limitation: opaque string. A user script cannot easily break out individual numbers without parsing the format.

### 3.5 Why this is the first step

- **Zero risk.** Uses only existing, tested code paths.
- **Immediate user value.** Users get a useful "last Claude step" readout within hours of merge.
- **Forward-compatible.** If Option 2 ships later, the `claude` sub-object gains typed fields alongside `last_summary`; old scripts keep working.
- **Validates the pipe.** Shipping Option 2a first proves the end-to-end mechanism (workflow → runner → payload → script) works before the more invasive Option 2 extensions land.

### 3.6 Verdict

**Ship this first.** It is the cheapest useful thing to do and it puts all the plumbing in place that Option 2 would need anyway.

---

## 4. Option 2 — Derive-from-Stream (Recommended second step)

**Premise:** extend `internal/claudestream.Aggregator` to capture the live fields a status line needs (most importantly `SessionID` and `Model` — which are *not* captured today), add a concurrent-safe snapshot accessor, and expose typed fields via pr9k's payload.

### 4.1 What the stream *actually* provides today

Claude's NDJSON stream is launched by default on every step: `src/internal/sandbox/command.go:77-90` (**E24**) passes `--output-format stream-json --verbose`. pr9k's `internal/claudestream.Pipeline` consumes it. Five event types are recognized (`src/internal/claudestream/parser.go:46-79`, **E11**): `system`, `assistant`, `user`, `result`, `rate_limit_event`.

**Critical correction from adversarial validation (V1, V2):** the current `Aggregator.Observe` (`src/internal/claudestream/aggregate.go:23-49`, **E17**) *does not update `SessionID` from `SystemEvent{subtype:"init"}`*. There is no `*SystemEvent` branch in the switch. `SessionID` is assigned only inside the `*ResultEvent` branch (line 39). Similarly `TotalCostUSD` is assigned *only* at `ResultEvent` (line 37), and `Model` is not in `StepStats` at all (`src/internal/claudestream/event.go:132-143`, **E16**).

Test `TestAggregator_ObserveIgnoresSystemAndUser` at `src/internal/claudestream/aggregate_test.go:184-197` pins this behavior: after a `SystemEvent{Type:"system", Subtype:"init", SessionID:"s1"}` is fed in, `stats.SessionID` is asserted to still be `""`. So we cannot claim today that pr9k gets live `session_id` and `model` from the stream — it does not.

### 4.2 Mapping pr9k-available fields to Claude's native statusLine payload

| Claude native field | pr9k source | Liveness | Fidelity |
|---------------------|-------------|----------|----------|
| `session_id` | `StepStats.SessionID` (only from `*ResultEvent` today; would need `*SystemEvent{init}` support added to `Aggregator.Observe`) | Post-step today; can be made in-flight by adding init-event handling | ⚠️ **requires parser extension** |
| `model.id` | Not in `StepStats` today. Available on wire as `SystemEvent.Model` (**E12**) and `AssistantMsg.Model` (**E13**). Would require a new `StepStats.Model` field. | Requires new field | ⚠️ **requires parser extension** |
| `model.display_name` | Derivable from id via a pr9k-maintained mapping table. | Static | ⚠️ computed |
| `cwd`, `workspace.current_dir`, `workspace.project_dir` | pr9k knows `ProjectDir` (**E30**) | Always | ✅ 100% |
| `workspace.added_dirs` | pr9k does not use `--add-dir` | N/A | ⚠️ empty |
| `workspace.git_worktree` | Computable via `git` in `projectDir` | On demand | ⚠️ computed |
| `version` (Claude Code) | Available on wire in `SystemEvent{init}.claude_code_version`, not currently parsed. | Static once captured | ⚠️ **requires parser extension** |
| `cost.total_cost_usd` | `StepStats.TotalCostUSD` — assigned only at `*ResultEvent` (V2). **Not live during a running step.** | Post-step only | ⚠️ post-step only |
| `cost.total_duration_ms` | `StepStats.DurationMS` (populated from `ResultEvent`). Not live. | Post-step only | ⚠️ post-step only |
| `cost.total_api_duration_ms` | `ResultEvent.DurationAPIMS` (**E15**). Not on `StepStats`; would need a new field. | Post-step only | ⚠️ **requires field** |
| `cost.total_lines_added`, `total_lines_removed` | Not in the stream. Claude-internal. | Never | ❌ requires Option 1 or local git-diff |
| `context_window.total_input_tokens` | `StepStats.InputTokens` — cumulative per-turn (lines 26-29). **Live but imprecise** (V3): during multi-turn steps, per-turn token totals are added, which double-counts the cached prefix on repeated turns. At step end the `ResultEvent` branch *overwrites* (not augments) the running tally, so users will see the running total suddenly correct to a much smaller number. | Live, but noisy | ⚠️ live with caveat |
| `context_window.total_output_tokens` | `StepStats.OutputTokens` — same caveat as above | Live, but noisy | ⚠️ live with caveat |
| `context_window.current_usage.*` | Would require tracking "last turn usage" — `Aggregator` does not retain it today; only the running sum | Not available | ⚠️ **requires new field** |
| `context_window.context_window_size` | Static per model (200 000 for current Opus/Sonnet; subject to change). Lookup table. | Static | ⚠️ computed |
| `context_window.used_percentage`, etc. | Derivable from input tokens and window size — but the input-tokens caveat above applies | Derived | ⚠️ computed from imprecise inputs |
| `rate_limits.five_hour.*`, `seven_day.*` | Not on the stream as aggregates. `RateLimitEvent` carries `LastRateLimitInfo` — transient burst-window snapshot only (**E17**). | Event-driven only | ❌ requires Option 1 for truth |
| `output_style.name`, `vim.mode`, `agent.name`, `worktree.*` | Not emitted by claude-in-container | N/A | ⚠️ omit |
| `transcript_path` | **Not stable by derivation alone** (V12). Claude's NDJSON init event actually publishes the path via `memory_paths.auto` (strip `/memory/` suffix). `claudestream.SystemEvent` does not parse this field today. | Requires parser extension | ⚠️ **requires parser extension** |

**Net coverage for a status-line use case:**
- Live, post-extension: `session_id`, `model`, `input_tokens`, `output_tokens` (noisy), token-percentage-derivatives (noisy).
- Post-step: `total_cost_usd`, `total_duration_ms`, `num_turns`.
- Never without Option 1: `rate_limits.five_hour/seven_day`, `cost.total_lines_added/removed`.

### 4.3 Integration into pr9k

The work splits into three layers:

**Layer A — parser extension (new):**
- `src/internal/claudestream/aggregate.go`: add a `*SystemEvent` case that captures `SessionID`, `Model`, and `claude_code_version` (requires the latter to be added to the `SystemEvent` struct in `event.go` first). Also capture `memory_paths.auto` if transcript_path is wanted.
- `src/internal/claudestream/event.go`: extend `StepStats` with `Model string`, optionally `ClaudeCodeVersion string`, optionally `TranscriptPath string`, and a `LastTurnUsage Usage` field if live `current_usage` is desired.
- Tests: update `TestAggregator_ObserveIgnoresSystemAndUser` (rename and invert assertions).

**Layer B — concurrent-safe snapshot (V8):**
- Decision required: mutex vs `atomic.Pointer[StepStats]`. A mutex adds ~tens of nanoseconds per `Observe` call (hundreds per step); `atomic.Pointer` requires a fresh allocation per event. Recommend a mutex — `Observe` allocations already happen on the hot path during NDJSON parsing, the mutex overhead is negligible, and a mutex is simpler to reason about.
- Concern: `StepStats.LastRateLimitInfo *RateLimitInfo` aliases an interior struct. Either deep-copy in `Snapshot()` or change the field to a value. Deep copy is safer and one line.
- Concern: other Aggregator fields (`result`, `hasResult`, `isError`, `subtype`, `stopReason`) could tear. For Option 2 only `StepStats` needs to be coherent, and all `StepStats` writes are guarded by the same mutex, so intra-`StepStats` tearing is eliminated. Cross-field tearing with `hasResult` is out-of-scope for this work.
- Add a `go test -race` stress test that drives the aggregator with a concurrent reader.

**Layer C — wiring (analogous to Option 2a plus typed fields):**
- `src/internal/claudestream/pipeline.go`: add `Stats() StepStats` (wraps `aggregator.Snapshot()`).
- `src/internal/workflow/workflow.go`: add `ActiveClaudeStats() (claudestream.StepStats, bool, time.Time)` following the `HeartbeatSilence` template (**E27**). Return a `startedAt` so the status script can distinguish *cold-start warming up* from *idle between steps* (V9). Fall back to `LastStats()` when no pipeline is active, so the status line does not flicker to empty at step boundaries.
- `src/internal/statusline/state.go` + `payload.go`: add typed fields under `claude.{session_id, model, input_tokens, output_tokens, ...}`. Keep `claude.last_summary` from Option 2a alongside.
- `src/internal/statusline/statusline.go`: `SetClaudeStatsGetter(fn func() (claudestream.StepStats, bool, time.Time))`, used in `execScript` (**E8**).
- `src/cmd/pr9k/main.go`: slot the new injection between `SetModeGetter` and `Start` (**E50**).
- `workflow/scripts/statusline`: demonstrate consuming typed fields.

### 4.4 Fidelity summary

- **Live (post-extension, with caveats):** session_id, model.id, input_tokens, output_tokens, running cache tokens.
- **Post-step authoritative:** total_cost_usd, total_duration_ms, num_turns, and the *corrected* token counts (ResultEvent replaces the running tally).
- **Computed from pr9k knowledge:** model.display_name, context_window_size, used_percentage, cwd, git_worktree, transcript_path.
- **Not available without Option 1:** cost.total_lines_added/removed, precise rate_limits.five_hour/seven_day aggregates, workspace.added_dirs (N/A), output_style/vim/agent/worktree (N/A).

### 4.5 Failure modes

- **Aggregator concurrency bugs** — V8. Mitigate with `-race` stress test and `LastRateLimitInfo` deep-copy.
- **Noisy live token counts** — V3. Status-line users who compute a context-window percentage from live tokens will see inflated numbers during multi-turn cache-heavy steps. Document this, or clamp the reported percentage to never exceed 100%.
- **Cold-start vs idle ambiguity** — V9. Return a `startedAt` from `ActiveClaudeStats()`; treat post-step display with `LastStats()` fallback as the default between steps.
- **`--resume` behavior is unvalidated** — V4. Capture a real NDJSON stream from `claude --resume` and confirm the `SystemEvent{init}` fires (carrying the resumed SessionID and Model) before committing. If it does not, Option 2's live SessionID/Model only works for fresh sessions; pr9k would need to keep the first assistant event's `message.model` as a fallback.
- **Aggregator struct invariants** — the `Aggregator` comment at `aggregate.go:8-19` says it is single-goroutine owned. Changing that is a change to its contract; update the comment.

### 4.6 Verdict

**Ship this after Option 2a, not instead of it.** The work is medium-complexity and requires real parser changes — none of which are blockers, but all of which are more than the "wiring" the initial draft claimed.

---

## 5. Option 1 — Capture-and-Forward

**Premise:** install a pr9k-controlled shim script inside the Claude configuration as Claude's own `statusLine.command`. When Claude invokes the shim, the shim has Claude's full native payload on stdin. The shim then forwards that payload back to the host pr9k process, which stores it and passes it through to pr9k's own user-facing statusLine script on the next refresh.

### 5.1 Foothold

- **Profile dir is a read-write bind mount** (`src/internal/sandbox/command.go:41-42`, **E41**; confirmed rw by `docs/features/docker-sandbox.md:87`, **E48**). Writing files into that directory before launching is mechanically possible.
- **`CLAUDE_CONFIG_DIR` always points at the mount** (`src/internal/sandbox/command.go:44`, **E43**). Claude inside the container reads `/home/agent/.claude/settings.json` — which IS the host's `<profileDir>/settings.json`.
- **Container uid/gid matches host.** `src/internal/sandbox/command.go` passes `-u <UID>:<GID>` derived from the host user; no permission flip.

### 5.2 Major blocker, not a mitigable risk: profile-dir contamination (V5)

The "profile dir" is the user's **real** `~/.claude`. On a typical host it already contains `settings.json` with user-defined permissions, plugins, and marketplace config — data that pr9k would destroy if it simply wrote `settings.json` from scratch. The research draft originally framed this as "Mitigation: detect and preserve an existing statusLine." That framing is too weak for what is actually required.

Before Option 1 can ship, all of the following must be designed:

1. **Parse-then-merge** — read existing `settings.json`, add a `statusLine` block only if the user has not defined one, leave every other field untouched.
2. **Atomic write** — temp file + rename to prevent partial-write corruption on pr9k crash.
3. **Preserve permissions** — existing `settings.json` is typically `rw-------`; do not widen.
4. **Shim executable permissions** — `os.WriteFile` default of 0600 will not execute inside the container. The shim must be written with mode 0755.
5. **Opt-in by default** — do not write into the real `~/.claude` unless the user has explicitly enabled the feature in `config.json`.
6. **Rollback** — if pr9k crashes mid-run, the injected `statusLine` entry is left in the user's settings. A cleanup hook on shutdown is needed, with an "orphan settings" detection fallback on next start.

Treat this as the critical path. If any of these are not in the design, Option 1 is not shippable.

### 5.3 IPC back to the host

The shim runs inside the container and must hand Claude's payload to pr9k on the host. Practical options:

| IPC approach | Cost |
|--------------|------|
| Write to a file in the profile mount, poll from host | Polling adds latency; pr9k-owned files mingle with Claude's own profile. |
| Write to a file in the project mount | Visible to `git status`; collides with pr9k's rule that intermediate artifacts live in `.pr9k/` (ADR `docs/adr/20260418175134-pr9k-rename-and-pr9k-layout.md`). |
| TCP socket to the host (`host.docker.internal`) | Requires a new host listener; requires allowlisting a new env var in `BuildRunArgs` (**E44**); new attack surface. |
| Named FIFO in the profile mount | macOS Docker Desktop bind-mount FIFO is flaky. |
| Shim stderr | Not routed anywhere pr9k sees — Claude consumes statusLine output itself. Not viable. |

**Mount constraint** — `rg 'workflowDir\|--add-dir' src/internal/sandbox` returns nothing (**E49**). pr9k does not mount the workflow dir. Shim files must be *written*, not mounted, into the profile mount.

### 5.4 Integration into pr9k

- `internal/sandbox` — new pre-spawn hook (first of its kind — **E47** confirms no pre-start file prep today) that writes the merged `settings.json` + shim script. Plus the rollback hook.
- `internal/sandbox/image.go` — possibly extend `BuiltinEnvAllowlist` (**E44**) with a pr9k shim address env var.
- `internal/statusline` — new storage for the latest captured payload (mutex-guarded), new goroutine/poller, new `claude` field in `State` and `BuildPayload`.
- `internal/steps`, `internal/validator` — new `statusLine.captureClaudeStatusLine: true` flag (strict `DisallowUnknownFields`, **E38**).
- Host-side IPC listener + permission/port choice.
- Container base image — confirm `sh` + file-write primitives exist; `nc` may need to be replaced with a stdin-redirect write or a small Go binary depending on base image contents.

### 5.5 Fidelity

**100% — by construction.** The shim sees exactly what Claude sends. No field synthesis, no interpolation, no staleness beyond the 300 ms Claude debounce. This is the only path to `rate_limits.five_hour.used_percentage`, `cost.total_lines_added/removed`, and precise `context_window.used_percentage`.

### 5.6 Verdict (original draft)

**Viable, but deferred.** Only worth the cost if users explicitly ask for the Claude-only fields. Most operator use cases are satisfied by Option 2a + Option 2.

*Superseded by Section 6 below. The scratch-config-dir variant (Option 1′) makes Option 1 shippable at medium cost.*

---

## 6. Option 1′ — Capture-and-Forward, Revised (scratch config dir + baked-in shim + opt-in)

**Re-investigation prompt:** "Would Option 1 be easier if we had a full Dockerfile configuration? Or as an optional plugin that the end user explicitly enables? What is the most optimal capture-and-forward design?"

**Top-line answer:** The Dockerfile helps with two narrow concerns; explicit opt-in helps with two more; but the biggest unlock comes from a third dimension the original draft missed — redirecting `CLAUDE_CONFIG_DIR` at the container boundary to a pr9k-owned scratch directory, so pr9k never touches the user's real `~/.claude/settings.json` at all.

### 6.1 Dimension 1 — Does a pr9k-owned Dockerfile help? (Partial yes)

**What pr9k ships today:**
- No Dockerfile exists in the repo (verified by `Glob("**/Dockerfile*")` — matches are only vendored Go toolchain files).
- `src/internal/sandbox/image.go:1-7` hard-codes `ImageTag = "docker/sandbox-templates:claude-code"` — a tag pulled as-is from Anthropic.
- `src/cmd/pr9k/sandbox_create.go:88-112` does `docker pull` + smoke-test; there is no `docker build` plumbing.
- ADR `docs/adr/20260413160000-require-docker-sandbox.md:115-118` constrains the *presence* of a sandbox, not the *origin* of the image; the tag-only pin is described as a "trust trade-off" for upgrade ergonomics, not a hard architectural constraint.

**What a pr9k-owned Dockerfile (extending `docker/sandbox-templates:claude-code`) buys:**

| V5 blocker | Closed by Dockerfile? | Reasoning |
|---|---|---|
| Item 1 (parse-then-merge) | ❌ | Orthogonal — settings.json still needs to declare `statusLine.command`. |
| Item 2 (atomic write) | ❌ | Orthogonal — same reason. |
| Item 3 (preserve permissions) | ❌ | Orthogonal — same reason. |
| Item 4 (shim mode 0755) | ✅ | Dockerfile can `COPY pr9k-statusline-shim /usr/local/bin/ && chmod 0755` at build time; no `os.WriteFile` needed. |
| Item 5 (opt-in default) | ❌ | Policy decision, not image concern. |
| Item 6 (rollback) | ❌ | Same — applies to the user's settings.json, not the image. |
| 5.3 base-image utility uncertainty | ✅ | A derived image can ship a statically-linked Go shim binary; no need to rely on `sh`/`nc` being in the base image. |

**Costs of owning a Dockerfile:**
- New `docker/Dockerfile` + build-context directory.
- Extend `sandbox_create` from a 14-line `docker pull` into a `docker build` (or `docker pull` + `docker build`) flow — roughly 50–100 extra lines in `sandbox_create.go`.
- Derived-tag choice: `pr9k/claude-sandbox:<version.Version>` ties image cache validity to `src/internal/version/version.go`, already the single source of truth (`docs/coding-standards/versioning.md:5-11`).
- `BuildRunArgs` and `BuildLoginArgs` both reference `ImageTag` (`src/internal/sandbox/image.go:4` + `src/internal/sandbox/command.go:118`); the derived tag must flow through both to keep `sandbox login` consistent with `pr9k` runs.
- Upstream tracking: when Anthropic updates `docker/sandbox-templates:claude-code`, users must re-run `pr9k sandbox create --force` to rebuild. Already the model today, just amplified.
- Tension with the narrow-reading ADR (`docs/adr/20260410170952-narrow-reading-principle.md`): a Dockerfile that exists only to enable Option 1′ is a workflow-ish concern baked into pr9k. Justified if it solves enough distinct problems; keeping the shim strictly inside pr9k's image-ownership story is defensible.

**Verdict on Dockerfile alone:** necessary-but-not-sufficient. It closes 2 of 6 V5 items and eliminates one base-image uncertainty. Not a standalone solution for Option 1.

### 6.2 Dimension 2 — Does explicit user opt-in help? (Partial yes)

**Opt-in precedent in pr9k today:**
- `containerEnv` allows literal-value env injection that could be secrets; validator warns on `_TOKEN`/`_KEY`/`_SECRET` suffixes but permits the write (`src/internal/validator/validator.go:235-244`).
- `resumePrevious` is a claude-specific behavior gate defaulting to off (`src/internal/steps/steps.go:42-49`).
- Validator schema already strict via `DisallowUnknownFields` (`src/internal/validator/validator.go:164-168`); adding `CaptureClaudeStatusLine *bool` to `vStatusLine` (lines 93-97) and `StatusLineConfig` (`src/internal/steps/steps.go:51-57`) is ~10 lines.

**What opt-in closes on V5:**

| V5 blocker | Closed by opt-in alone? | Reasoning |
|---|---|---|
| Item 1 (parse-then-merge) | ❌ (or only via "refuse and guide") | Consent doesn't make merging safe; the alternative is refuse-if-incompatible, which shifts complexity to the user. |
| Item 2 (atomic write) | ❌ | Atomicity is a correctness property; user consent doesn't waive it. |
| Item 3 (preserve permissions) | ❌ | Same. |
| Item 4 (shim mode 0755) | ❌ | Orthogonal — see Dimension 1. |
| Item 5 (opt-in default) | ✅ | Collapses to "check the flag." |
| Item 6 (rollback) | ✅ (via detectable-marker + startup self-heal) | If pr9k's startup always removes its own marked `statusLine` block when the flag is off, crash rollback becomes idempotent self-healing at next start. Gated by Q-OPEN-2 (does Claude accept unknown sibling keys under `statusLine`?). |

**"Refuse and guide" variant:** pr9k refuses to start if the user's `settings.json` already has a `statusLine` block it doesn't own, printing an actionable message. This collapses V5 items 1–2 (no merge; just read and compare) at the cost of user-onboarding friction. Precedent: the existing "preflight: claude profile directory not found" error at `src/internal/preflight/profile.go:37-44` follows the same error-with-actionable-message pattern.

**Reality check on the user's real `settings.json`:** on a typical host, this file is 0600-mode and contains user-chosen permission allowlists, plugin configuration, and marketplace settings (verified on the current host — file is 734 bytes with `permissions.allow`, `enabledPlugins`, `extraKnownMarketplaces`, `skipAutoPermissionPrompt` keys). "Refuse and guide" will bounce many real users on first run.

**Verdict on opt-in alone:** closes 2 of 6 V5 items cleanly (5, 6) and reduces 2 more to a user-friction trade (1, 2 via refuse-and-guide). Items 3 and 4 remain. Not a standalone solution; complements Dimension 1.

### 6.3 Dimension 3 — The unlock: a pr9k-owned scratch `CLAUDE_CONFIG_DIR`

**Key facts from re-investigation (all verified in the current tree):**

- `CLAUDE_CONFIG_DIR` is already unconditionally overridden at the container boundary. `src/internal/sandbox/command.go:44`:
  ```go
  "-e", "CLAUDE_CONFIG_DIR=" + ContainerProfilePath,
  ```
  Whatever the host had, claude-in-container reads from `/home/agent/.claude`.
- Credentials are a **single file**. `src/internal/preflight/profile.go:58` stats only `.credentials.json`, and the `BuildLoginArgs` comment at `src/internal/sandbox/command.go:97-99` confirms `/login` writes a single `.credentials.json`. Host verification: `ls -la ~/.claude/.credentials.json` returns `-rw-------@ 1 … 471 bytes` — one small file.
- Docker bind-mounts support both directory-level *and* file-level sources. `--mount type=bind,source=<host>/.credentials.json,target=/home/agent/.claude/.credentials.json` is valid on both Docker Desktop (macOS) and native Linux Docker.
- The `.pr9k/` umbrella is already gitignored (recent commits `a606c6e` "updating gitignore" and `671f54a` "realign .gitignore tests" per the startup status); a scratch `CLAUDE_CONFIG_DIR` parked at `<projectDir>/.pr9k/sandbox-claude-config/` stays out of git.

**The design:**

Instead of bind-mounting the user's real `~/.claude` and then *writing into it*, pr9k:

1. Creates a scratch directory at `<projectDir>/.pr9k/sandbox-claude-config/` per run.
2. Writes `settings.json` into the scratch dir — a pr9k-fully-owned file — with the single block:
   ```json
   {
     "statusLine": {
       "type": "command",
       "command": "/usr/local/bin/pr9k-statusline-shim"
     }
   }
   ```
3. Bind-mounts the scratch dir at `/home/agent/.claude` (replacing or supplementing today's real-profile mount).
4. Bind-mounts the real profile's `.credentials.json` as a *file-level* overlay at `/home/agent/.claude/.credentials.json`, so Claude can authenticate.
5. Keeps the shim baked into a pr9k-owned Docker image (Dimension 1).

**What collapses:**

| V5 blocker | Closed by Dimension 3? | Why |
|---|---|---|
| Item 1 (parse-then-merge) | ✅ | pr9k owns the whole settings.json — nothing to merge. |
| Item 2 (atomic write) | ✅ | pr9k writes its own scratch file; no risk of corrupting user state. |
| Item 3 (preserve permissions) | ✅ | pr9k picks the mode (0600) from the outset. |
| Item 4 (shim mode 0755) | ✅ (via Dimension 1) | Baked into image. |
| Item 5 (opt-in default) | ✅ (via Dimension 2) | Flag check. |
| Item 6 (rollback) | ✅ | Scratch dir can be deleted on successful exit; on crash, next `pr9k` invocation cleans stale scratch dirs trivially (no merge state to reconcile). |
| 5.3 base-image utility uncertainty | ✅ (via Dimension 1) | Baked-in Go binary. |

All six V5 blockers + the base-image concern fall. Option 1 moves from "high risk, defer" to "medium risk, shippable with opt-in gate."

**Caveats and open questions this design depends on:**

- **Q-OPEN-1 (critical):** does the Claude CLI read state files from the profile directory *beyond* `.credentials.json`? On the current host, `~/.claude/` contains (live listing) `backups/`, `debug/`, `file-history/`, `plans/`, `plugins/`, `projects/`, `sessions/`, `shell-snapshots/`, `statsig/`, `tasks/`, `teams/`, `telemetry/`, `todos/`, plus files `.claude.json`, `CLAUDE.md`, `history.jsonl`, `mcp-needs-auth-cache.json`, `rule-violations.md`, `stats-cache.json`. If any of these are read (not just written) by Claude CLI at startup in a way that operator workflows depend on (e.g. resuming a past session by UUID), the pure scratch-dir design fails and we need the fallback below.
- **Fallback design if Q-OPEN-1 goes the wrong way:** bind-mount the *real* profile dir as today, and layer a file-level bind-mount of pr9k's scratch `settings.json` *on top of* the real one. Docker supports file-over-file bind-mounts on both macOS Desktop and native Linux. This avoids merging the user's settings.json by shadowing it — the user's real file on disk is untouched; only the view inside the container is pr9k-owned for the duration of the run.
- **Q-OPEN-3:** Claude Code may also accept `statusLine.command` from a project-local `.claude/settings.json` inside the project mount. If so, an even simpler variant is possible: write `.pr9k/claude-settings/.claude/settings.json` inside the project dir, then point `CLAUDE_CONFIG_DIR` at it. Needs confirmation at `https://code.claude.com/docs/en/statusline.md`.
- **Q-OPEN-4:** if there is a `CLAUDE_STATUSLINE_COMMAND` env var or equivalent, the entire settings.json write step disappears — pr9k just sets the env var in `BuildRunArgs`. Grep of `src/` returns no matches; not in the pr9k codebase, unknown from Claude docs.

### 6.4 The IPC question — how does the shim hand Claude's payload back to pr9k?

**Options the original draft enumerated (`research.md:281-292`):**

| Transport | Issues |
|---|---|
| Write to file in profile mount, poll from host | Latency; mingles with Claude's own state. |
| Write to file in project mount | Visible to `git status` if not under `.pr9k/`. |
| TCP to `host.docker.internal` | New env allowlist entries, new host listener, new attack surface. |
| Named FIFO in profile mount | Flaky on Docker Desktop macOS. |
| Shim stderr | Not viable — Claude consumes it. |

**New option surfaced by this investigation:** a **Unix-domain socket on the project bind-mount**.

- pr9k creates `<projectDir>/.pr9k/statusline.sock` on the host, listens on it before spawning the container.
- The shim inside the container connects to `/home/agent/workspace/.pr9k/statusline.sock` (the project is already bind-mounted at `/home/agent/workspace`) and writes the received stdin payload per Claude invocation.
- `.pr9k/` is already in `.gitignore`; the socket node is invisible to `git status`.
- No new env allowlist entries (nothing cross-host-network).
- Lower attack surface than TCP: no port open outside the container.

**Viability:** Unix-domain sockets over bind-mounts have been *historically unreliable* on Docker Desktop macOS (the same concern that sinks named FIFOs in V5.3). Current state needs confirmation against Docker Desktop release notes (Q-OPEN-5) or a direct test on the target host. On native Linux Docker, Unix sockets over bind-mounts are reliable and well-exercised.

**Fallback if Unix-socket is flaky:** file-write with atomic rename under `<projectDir>/.pr9k/statusline-current.json`, pr9k polls via `fsnotify` (or a 300 ms timer matching Claude's own debounce). Slightly higher latency; no new risk surface.

### 6.5 Recommended optimal design for Option 1

Combining all three dimensions, in order of what pr9k ships:

1. **`docker/Dockerfile`** in the repo root, extending `docker/sandbox-templates:claude-code`, `COPY`ing a statically-linked Go shim binary to `/usr/local/bin/pr9k-statusline-shim` with mode 0755. Derived tag `pr9k/claude-sandbox:<version.Version>`. `sandbox_create` grows a `docker build` step.

2. **`config.json` opt-in:** new `statusLine.captureClaudeStatusLine: bool` field, default `false`. Validator additions (~10 lines in `vStatusLine` + validator section). Feature-gate all subsequent behavior on this flag.

3. **Scratch `CLAUDE_CONFIG_DIR`:** on container spawn (when flag is true), pr9k creates `<projectDir>/.pr9k/sandbox-claude-config/`, writes `settings.json` declaring `statusLine.command = /usr/local/bin/pr9k-statusline-shim`, bind-mounts the scratch dir at `/home/agent/.claude`, and file-bind-mounts the real `.credentials.json` at `/home/agent/.claude/.credentials.json`.
   - Fallback (if Q-OPEN-1 fails): keep the real-profile bind-mount and layer a file-level overlay of pr9k's settings.json on top.

4. **IPC via Unix-domain socket** at `<projectDir>/.pr9k/statusline.sock`:
   - pr9k `net.Listen("unix", ...)` before container spawn; close after teardown.
   - Shim `net.Dial("unix", "/home/agent/workspace/.pr9k/statusline.sock")`; on each invocation, reads its stdin, pipes to the socket, exits.
   - Fallback (if Q-OPEN-5 rules out Unix sockets on macOS Docker Desktop): atomic file-write + `fsnotify` poll under the same `.pr9k/` prefix.

5. **pr9k-side payload handling:** a new `claudePayload atomic.Pointer[json.RawMessage]` on `statusline.Runner`, updated from the socket listener goroutine. `BuildPayload` (`src/internal/statusline/payload.go:5-50`) embeds it as `claude.native: <Claude's full stdin payload>` so user scripts can read every field Claude sends.

6. **Self-healing cleanup:** on every pr9k startup, remove stale `<projectDir>/.pr9k/sandbox-claude-config/` and `<projectDir>/.pr9k/statusline.sock` before creating fresh ones. No rollback-hook-in-signal-handler is needed because pr9k never touched the user's real `~/.claude`.

7. **Versioning decision before payload expansion:** resolve V7 (the payload is not currently part of the formal public-API list) before embedding Claude's raw payload as `claude.native`. Either amend `docs/coding-standards/versioning.md:13-20` or explicitly record the payload as documented-but-not-versioned.

**Rough LOC estimate:**
- `docker/Dockerfile` + shim Go source: ~150 LOC.
- `sandbox_create` Docker-build path: ~80 LOC.
- `BuildRunArgs` scratch-dir + credentials file-mount wiring: ~40 LOC.
- `statusline.Runner` socket listener + `claudePayload` storage: ~100 LOC.
- Validator schema: ~10 LOC.
- Tests (`-race` stress test on the listener; integration test for the scratch-dir layout): ~200 LOC.
- **Total: ~580 LOC** of new code, plus ~50 LOC of documentation updates.

Compare to original Option 1 estimate (which was deferred as "too large to size without solving profile-dir merge first"): this is tractable.

### 6.6 Verdict on Option 1′

**Promoted from "defer" to "third phase."** With the scratch-config-dir design, the profile-dir contamination blocker (V5) collapses entirely. The remaining risks — Q-OPEN-1 (adjacent state file reads) and Q-OPEN-5 (Unix socket on Docker Desktop macOS) — are resolvable before implementation begins, each with a well-defined fallback design. Ship after Option 2, gated behind `captureClaudeStatusLine: true`.

---

## 7. Option 3b — Claude Agent SDK

**Premise:** rewrite pr9k's step-launch path to use the **Claude Agent SDK** and call structured state accessors like `get_context_usage()`.

### 7.1 Why it fails for pr9k

- **No Go Agent SDK.** The Agent SDK is Python-only (`https://github.com/anthropics/claude-agent-sdk-python`). The official Go SDK is the Messages API SDK (beta "Managed Agents"), not the Agent SDK — no equivalent state accessors.
- **No statusLine callback equivalent.** The SDK has hooks (`PreToolUse`, `PostToolUse`, `UserPromptSubmit`) for action interception. No periodic state-snapshot callback.
- **SDK bundles the Claude CLI and spawns it as a subprocess anyway.** It does not replace the subprocess layer.
- **pr9k does not own the container image** (V11) *today*. Section 6 proposes owning one for Option 1′ — but even with image ownership, there is no Go SDK surface that would expose the statusLine-relevant state. A Python sidecar inside the image would need an additional process lifecycle and a pr9k-owned IPC channel to it; that is strictly more work than Option 1′'s shim, for the same fidelity.

### 7.2 Verdict

**Do not pursue.** Re-evaluate only if Anthropic ships a Go Agent SDK or adds a statusLine-callback method to the Messages API. Owning a derived image (for Option 1′) does not change this; the SDK surface is the gating constraint.

---

## 8. Final Recommendation

**Phased plan (revised to include Option 1′):**

1. **Phase 1 — Option 2a (ship first):** surface `Renderer.Finalize` as a `claude.last_summary` string in pr9k's stdin payload. ~20 lines of Go plus docs plus a sample-script update. No parser changes, no concurrency, no container work. Users get immediate post-step Claude stats in their status line.

2. **Phase 2 — Option 2 (enhancement):**
   - Parser extensions in `claudestream`: capture `SessionID`, `Model`, `ClaudeCodeVersion`, and optionally `transcript_path` via `memory_paths.auto` from `SystemEvent{subtype:"init"}`.
   - Add `Model`, `LastTurnUsage`, `DurationAPIMS` fields to `StepStats`.
   - Add concurrent-safe `Snapshot()` to `Aggregator` with `LastRateLimitInfo` deep-copy.
   - Add `Pipeline.Stats()` wrapper.
   - Add `workflow.Runner.ActiveClaudeStats() (StepStats, bool, time.Time)` returning `LastStats()` as a between-steps fallback; include `startedAt` for cold-start signaling.
   - Add `statusline.Runner.SetClaudeStatsGetter(...)`.
   - Extend `statusline.State` / `BuildPayload` with typed `claude.{session_id, model, input_tokens, output_tokens, ...}` fields alongside `claude.last_summary`.
   - Update `workflow/scripts/statusline` and `docs/features/status-line.md`.
   - Add a `-race` stress test for the new concurrent path.
   - Before cutover: capture a real `claude --resume` NDJSON stream and confirm `SystemEvent{init}` fires with the resumed SessionID and Model.

3. **Phase 3 — Option 1′ (scratch-config-dir, opt-in):**
   - Resolve Q-OPEN-1 (does Claude read adjacent state files from the profile dir?) and Q-OPEN-5 (Unix-socket-over-bind-mount reliability on current Docker Desktop macOS) *before* starting. Each has a documented fallback (file-over-file settings.json overlay; atomic-write + `fsnotify` polling).
   - Add `statusLine.captureClaudeStatusLine: bool` to the config schema (~10 lines in `vStatusLine` + validator).
   - Ship a pr9k-owned `docker/Dockerfile` that extends `docker/sandbox-templates:claude-code`, bakes in a statically-linked Go shim at `/usr/local/bin/pr9k-statusline-shim`, and tags the result `pr9k/claude-sandbox:<version.Version>`. Extend `sandbox_create` with a `docker build` step.
   - Wire the scratch `CLAUDE_CONFIG_DIR`: on container spawn (when the flag is true), create `<projectDir>/.pr9k/sandbox-claude-config/`, write a pr9k-owned `settings.json`, bind-mount the scratch dir at `/home/agent/.claude`, file-bind-mount the real `.credentials.json` at `/home/agent/.claude/.credentials.json`.
   - Wire the Unix-domain socket IPC at `<projectDir>/.pr9k/statusline.sock`; the shim dials it from `/home/agent/workspace/.pr9k/statusline.sock`.
   - Extend `statusline.Runner` with a `claudePayload atomic.Pointer[json.RawMessage]`; `BuildPayload` emits it as `claude.native` when non-empty. Keep `claude.last_summary` from Phase 1 alongside.
   - Self-healing startup cleanup: remove stale scratch dirs and sockets at every pr9k start.
   - Do *not* promise this phase until users explicitly ask for the Claude-only fields (`rate_limits.five_hour.used_percentage`, `cost.total_lines_added/removed`).

**Tradeoffs:**

- Option 2a alone gives useful information *only after a step completes*. Users who want in-flight feedback during a running step do not get it from Option 2a; they must wait for Option 2.
- Option 2's "live" tokens are noisy during multi-turn cache-heavy steps (V3); users who compute context-window percentages from them will see inflated numbers that correct downward at step end. This should be documented, not fixed in the aggregator (the aggregator's "add per turn then overwrite at result" is intentional).
- Option 2 requires parser extensions beyond what the initial draft claimed; "it's already there" was wrong for SessionID, Model, and transcript_path.
- Option 1′ remains the only path to Claude-only truth for rate limits and lines-added/removed. The scratch-config-dir design eliminates the profile-dir merge problem at the cost of owning a Dockerfile and introducing a Unix-socket listener — both manageable.
- Owning a Dockerfile for Option 1′ adds a maintenance obligation: rebuild on base-image updates and on every `version.Version` bump. The ADR on Docker requirement (`docs/adr/20260413160000-require-docker-sandbox.md`) does not forbid it.

**Open questions to resolve before the relevant phase starts:**

*Phase 2 blockers:*
- How should `context_window.used_percentage` cope with inflated live token counts? Clamp to 100%? Surface both the cumulative and a per-turn derivative? Default recommendation: surface raw tokens only (no percentages) from pr9k, and let the user script compute a percentage if desired.
- Should `claude` be emitted always or only when `statusLine` is configured? Recommend always; the cost is a few extra bytes in a JSON payload only a configured script would even read.
- Resolve the V7 tension: does pr9k want the statusLine payload to become part of its versioned public API (document in `docs/coding-standards/versioning.md`), or explicitly keep it as a documented-but-not-versioned surface? Decide *before* enlarging it with a `claude` sub-object.
- V13 — `claude --help` audit: does a `claude status` / `claude session info` CLI subcommand exist? If yes, it is a simpler way to reach some of this data than aggregator extensions. Check in an authenticated environment.

*Phase 3 (Option 1′) blockers (all from the Section 6 re-investigation):*
- **Q-OPEN-1 (critical):** does the Claude CLI read state files from the profile dir beyond `.credentials.json`? If yes, switch from a scratch-dir mount to a file-level overlay of `settings.json` atop the real profile mount. Confirm against `https://code.claude.com/docs/en/settings.md` and `https://code.claude.com/docs/en/authentication.md`.
- **Q-OPEN-2:** does Claude accept unknown sibling keys under `statusLine` (e.g. a pr9k ownership marker)? If no, use a side-car marker file in the scratch config dir instead.
- **Q-OPEN-3:** does Claude accept `statusLine.command` from a project-local `.claude/settings.json` inside the project mount? If yes, a simpler variant becomes possible (no scratch `CLAUDE_CONFIG_DIR` needed).
- **Q-OPEN-4:** is there a `CLAUDE_STATUSLINE_COMMAND` (or similar) env var that bypasses settings.json entirely? If yes, the entire settings.json write step disappears.
- **Q-OPEN-5:** is Unix-domain socket over a bind-mount reliable on current Docker Desktop for macOS? Confirm against `https://docs.docker.com/desktop/release-notes/` or test on the target host. Fallback: atomic-write JSON file + `fsnotify` polling.
- **Q-OPEN-6:** does `docker/sandbox-templates:claude-code` (or the intended derived image) support running a statically-linked Go binary as the shim? If the base is distroless, compile with `CGO_ENABLED=0`.
- **Q-OPEN-7:** does Claude's `statusLine.command` receive stdin on every refresh or only on debounced assistant-message events? Documentation says 300 ms debounce; re-confirm before sizing IPC bandwidth.
- **Q-OPEN-8:** V13 audit (see above) may reveal a simpler CLI-side data-fetch that could be invoked from Option 2 code instead of building Option 1′ at all.

---

## 9. Appendix A — Claude's native statusLine payload (verbatim from docs)

Source: https://code.claude.com/docs/en/statusline.md#available-data

```json
{
  "cwd": "/current/working/directory",
  "session_id": "abc123...",
  "session_name": "my-session",
  "transcript_path": "/path/to/transcript.jsonl",
  "model": { "id": "claude-opus-4-7", "display_name": "Opus" },
  "workspace": {
    "current_dir": "/current/working/directory",
    "project_dir": "/original/project/directory",
    "added_dirs": [],
    "git_worktree": "feature-xyz"
  },
  "version": "2.1.90",
  "output_style": { "name": "default" },
  "cost": {
    "total_cost_usd": 0.01234,
    "total_duration_ms": 45000,
    "total_api_duration_ms": 2300,
    "total_lines_added": 156,
    "total_lines_removed": 23
  },
  "context_window": {
    "total_input_tokens": 15234,
    "total_output_tokens": 4521,
    "context_window_size": 200000,
    "used_percentage": 8,
    "remaining_percentage": 92,
    "current_usage": {
      "input_tokens": 8500,
      "output_tokens": 1200,
      "cache_creation_input_tokens": 5000,
      "cache_read_input_tokens": 2000
    }
  },
  "exceeds_200k_tokens": false,
  "rate_limits": {
    "five_hour": { "used_percentage": 23.5, "resets_at": 1738425600 },
    "seven_day": { "used_percentage": 41.2, "resets_at": 1738857600 }
  },
  "vim":      { "mode": "NORMAL" },
  "agent":    { "name": "security-reviewer" },
  "worktree": { "name": "my-feature", "path": "/path/to/.claude/worktrees/my-feature", "branch": "worktree-my-feature", "original_cwd": "/path/to/project", "original_branch": "main" }
}
```

Refresh semantics (verbatim from docs): "Your script runs after each new assistant message, when the permission mode changes, or when vim mode toggles. Updates are debounced at 300 ms." Optional `refreshInterval` (seconds, min 1).

---

## 10. Appendix B — Evidence Log

### pr9k's statusline package

**E1: `Runner` struct** — `src/internal/statusline/statusline.go:50-78`.
**E2: `Config` struct** — `src/internal/statusline/statusline.go:24-30`. Fields: `Command`, `RefreshIntervalSeconds`.
**E3: `State` struct** — `src/internal/statusline/state.go:8-22`. `SessionID` is pr9k's own `RunStamp`.
**E4: `BuildPayload` (stdin JSON)** — `src/internal/statusline/payload.go:5-50`.
**E5: `PushState(s State)`** — `src/internal/statusline/statusline.go:149-156`. Only state setter.
**E6: `Trigger()`** — `src/internal/statusline/statusline.go:159-167`. Drop-on-full channel, capacity 4.
**E7: Extension points** — `src/internal/statusline/statusline.go:126-145`. `SetSender`, `SetModeGetter`.
**E8: `execScript`** — `src/internal/statusline/statusline.go:263-278`. Payload assembly site.
**E9: Single-flight guard** — `src/internal/statusline/statusline.go:257-261`.
**E10: 2 s timeout, 8 KB stdout cap** — `src/internal/statusline/statusline.go:281-287, 33`.

### claudestream package

**E11: Five recognized event types** — `src/internal/claudestream/parser.go:46-79`. `system`, `assistant`, `user`, `result`, `rate_limit_event`.
**E12: `SystemEvent` fields** — `src/internal/claudestream/event.go:13-29`. `SessionID`, `Model` from `subtype:"init"`.
**E13: `AssistantMsg.Usage`** — `src/internal/claudestream/event.go:46-59`.
**E14: `ContentBlock` types** — `src/internal/claudestream/event.go:62-75`. `text`, `tool_use`, `tool_result`, `thinking`.
**E15: `ResultEvent`** — `src/internal/claudestream/event.go:92-107`. Final numbers.
**E16: `StepStats`** — `src/internal/claudestream/event.go:132-143`. **No `Model` field.**
**E17: `Aggregator.Observe`** — `src/internal/claudestream/aggregate.go:23-49`. **No `*SystemEvent` branch; `SessionID` and `TotalCostUSD` assigned only at `*ResultEvent`.**
**E18: Aggregator not concurrent-safe** — `src/internal/claudestream/aggregate.go:8-19`.
**E19: `Pipeline.Observe`** — `src/internal/claudestream/pipeline.go:50-81`.
**E20: `Pipeline.Aggregator()`** — `src/internal/claudestream/pipeline.go:93-101`.
**E21: `lastEventAt atomic.Int64`** — `src/internal/claudestream/pipeline.go:23-25, 59`.
**E22: Renderer output separation** — `src/internal/claudestream/render.go:32-47`.
**E23: No wiring claudestream → statusline today** — verified empty from `rg`.

### workflow package

**E24: `BuildRunArgs`** — `src/internal/sandbox/command.go:77-90`. `--output-format stream-json --verbose` hard-wired.
**E25: Stdout → `pipeline.Observe`** — `src/internal/workflow/workflow.go:560-603`.
**E26: `activePipeline`** — `src/internal/workflow/workflow.go:76-82, 332-342`.
**E27: `HeartbeatSilence()`** — `src/internal/workflow/workflow.go:718-733`. Template for `ActiveClaudeStats()`.
**E28: `LastStats()`** — `src/internal/workflow/workflow.go:702-706`. Post-step accessor (critical for Option 2a).
**E29: `StatusRunner` interface** — `src/internal/workflow/run.go:20-26, 203-207`.
**E30: `buildState`** — `src/internal/workflow/run.go:32-62`.
**E31: `push` closure** — `src/internal/workflow/run.go:319-325`.
**E32: Env into sandbox** — `src/internal/workflow/run.go:729-731`.

### Hooks & layering

**E33: No listener pattern on Pipeline today.**
**E34: `StepExecutor` interface surfaces `LastStats`** — `src/internal/workflow/run.go:66-85`.
**E35: Statusline refresh after step end** — `src/internal/workflow/run.go:494-495`.

### Config schema

**E36: `StatusLineConfig`** — `src/internal/steps/steps.go:51-66`. `Type` reserved for future growth.
**E37: Validator enforces `type == "command"` or empty** — `src/internal/validator/validator.go:257-273`.
**E38: `DisallowUnknownFields`** — `src/internal/validator/validator.go:164-168`.
**E39: Command path resolution** — `src/internal/statusline/statusline.go:404-416`.
**E40: Default workflow statusLine block** — `workflow/config.json:9-13`.

### Docker sandbox

**E41: Two bind mounts — project + profile** — `src/internal/sandbox/command.go:41-42`.
**E42: Container paths** — `src/internal/sandbox/image.go:1-7`.
**E43: `CLAUDE_CONFIG_DIR` hard-coded** — `src/internal/sandbox/command.go:44`.
**E44: `BuiltinEnvAllowlist`** — `src/internal/sandbox/image.go:9-17`.
**E45: `envDenylist`** — `src/internal/validator/validator.go:126-135`.
**E46: `ResolveProfileDir`** — `src/internal/preflight/profile.go:16-30`.
**E47: No pre-start file injection** — verified.
**E48: Profile mount is rw** — `docs/features/docker-sandbox.md:87`.
**E49: Workflow dir not mounted** — verified empty.

### Wiring

**E50: main.go wiring order** — `src/cmd/pr9k/main.go:141-197`.
**E51: `statusline.New` takes workflowDir + projectDir** — `src/internal/statusline/statusline.go:84`.
**E52: Claude statusLine stdout/stderr routing** — `src/internal/workflow/workflow.go:606-628`.

---

## 11. Appendix C — Claude Agent SDK notes

- **Python-only.** `https://github.com/anthropics/claude-agent-sdk-python` v0.1.63. Bundles Claude CLI; spawns it as subprocess.
- **No Go Agent SDK.** `https://github.com/anthropics/anthropic-sdk-go` v1.37.0 is the Messages API SDK; "Claude Managed Agents" (beta) but not the session-state accessors.
- **No statusLine callback equivalent.** Hooks are action interceptors, not periodic state snapshots. You would poll `get_context_usage()` yourself.
- **Hooks do NOT expose token or cost data.** Per https://code.claude.com/docs/en/hooks.md.
- **pr9k does not own the container image.** Adding a Python runtime requires pr9k to take ownership of a derived image — a major architectural change.

---

## 12. Adversarial Validation Findings

The following findings materially reshaped the recommendation. The initial draft proposed Option 2 as the single-step winner; validation found that several of Option 2's "live fidelity" claims were wrong, and surfaced a simpler alternative (Option 2a) that is now the Phase 1 recommendation.

**V1 — Aggregator does NOT capture SessionID or Model from `SystemEvent{init}`.**
- `src/internal/claudestream/aggregate.go:23-49` has no `*SystemEvent` branch. `StepStats.SessionID` is assigned only inside `*ResultEvent`. `Model` is not a field of `StepStats` at all (`event.go:132-143`).
- Test `TestAggregator_ObserveIgnoresSystemAndUser` at `aggregate_test.go:184-197` explicitly asserts `stats.SessionID == ""` after an init event.
- **Impact:** Option 2 requires an `Aggregator.Observe` extension and a new `StepStats.Model` field. "It's already there" was wrong.

**V2 — `TotalCostUSD` is NOT updated live per event.**
- `aggregate.go:30-45`: `TotalCostUSD` is assigned only in `*ResultEvent`. `DurationMS` and `SessionID` share that branch.
- **Impact:** During a running step, `ActiveClaudeStats().TotalCostUSD` is zero. The "live cost" claim was wrong. Recommendation now documents cost as post-step only.

**V3 — Live tokens are cumulative per turn, but cache tokens double-count.**
- `aggregate.go:26-29`: `*AssistantEvent` adds `Usage` to the stats. `*ResultEvent` replaces the running tally with the authoritative cumulative totals (lines 42-45, with comment "The result event carries cumulative usage totals; prefer them over the running tally").
- Test `TestAggregator_MultiAssistantThenResult` at `aggregate_test.go:203-245` verifies: three assistant events summing 60 input tokens get overwritten by a ResultEvent's `InputTokens: 7`.
- **Impact:** Live token display will inflate during multi-turn cache-heavy steps and then correct downward at step end. Surface as caveat in docs; do not compute percentages on the pr9k side.

**V4 — `--resume` behavior is unvalidated.**
- No `--resume` fixture in `docs/plans/streaming-json-output/fixtures/`. The iteration-log workflow at `docs/how-to/resuming-sessions.md:92-101` relies on `StepStats.SessionID` matching across resumed sessions — which comes from `ResultEvent.SessionID` — so resumed steps' ResultEvent *does* carry the matching session_id, but the *init* event is not pinned.
- **Impact:** Blocker for Option 2 cutover: capture a real resumed-session NDJSON stream and confirm init-event presence.

**V5 — Profile-dir contamination is a blocker for Option 1, not a mitigable risk.**
- `/Users/mxriverlynn/.claude/settings.json` is a real user-owned file with user-configured data. pr9k's bind-mount at `src/internal/sandbox/command.go:41-42` is the actual host directory — no layering.
- `os.WriteFile` defaults to 0600; shim scripts must be 0755 to execute.
- Container uid/gid matches host (`src/internal/sandbox/command.go` `-u <UID>:<GID>`) — this specific permissions concern is handled.
- **Impact:** Option 1 now has a mandatory merge-preserve-permissions design list; it is not "write settings.json."

**V6 — `Sanitize` affects script output, not payload.**
- `src/internal/statusline/sanitize.go:11` strips ANSI from script stdout; it does not touch the stdin JSON pr9k writes.
- **Impact:** No concern for the payload; 8 KB cap on the output side still applies.

**V7 — The statusLine payload is NOT part of pr9k's formal public API.**
- `docs/coding-standards/versioning.md:13-28` enumerates public surfaces; the stdin payload is not listed.
- `docs/features/status-line.md:66-98` documents the payload as "all fields always present" — de-facto documented, not versioned.
- **Impact:** The initial draft's "minor bump under 0.y.z" framing was incorrect. Resolve before enlarging the payload: either add payload to the versioning doc's public-surface list, or explicitly keep it as documented-but-not-versioned.

**V8 — Concurrent snapshot on Aggregator is medium-risk, not low-risk.**
- `StepStats.LastRateLimitInfo *RateLimitInfo` aliases — snapshots must deep-copy or change the field to value.
- Mutex vs `atomic.Pointer[StepStats]` decision: recommend mutex for simplicity (hot path already allocates).
- Existing Aggregator fields (`result`, `hasResult`, `isError`, `subtype`, `stopReason`) can tear vs `stats` if a future feature reads them concurrently; not in scope for Option 2 but worth noting.
- `aggregate_test.go` has no `-race` stress test today — a new test is required.
- **Impact:** Added concrete decisions and test work to Option 2's plan.

**V9 — `ActiveClaudeStats` has a cold-start ambiguity.**
- `src/internal/workflow/workflow.go:330-342`: `activePipeline` is set before streaming starts; during the Docker startup window `StepStats` is zero.
- `HeartbeatSilence` (`workflow.go:728-731`) handles a similar case via `startedAt`.
- **Impact:** `ActiveClaudeStats()` signature includes `startedAt time.Time` and falls back to `LastStats()` between steps.

**V10 — Missing alternative: `Renderer.Finalize` already builds the summary line.**
- `src/internal/claudestream/render.go:51-64` already produces `"<turns> turns · <in>/<out> tokens (cache: <c>/<r>) · $<cost> · <duration>"`. `workflow.go:379-385` writes it to the log after each step.
- **Impact:** This is now Option 2a. Makes Phase 1 a one-day ship instead of a week's worth of parser and concurrency work. The initial recommendation was overkill.

**V11 — Agent SDK dismissal right answer, different reason.**
- pr9k doesn't own the container image (`docker/sandbox-templates:claude-code`); any Python sidecar would require pr9k to own a derived image.
- **Impact:** Section 6.1 now cites the image-ownership constraint, not just "no Go SDK."

**V12 — Transcript path is not a stable convention.**
- Host `~/.claude/projects/` directory names use `/ → -` slugging of the absolute path. For Claude-in-container, the slug is `-home-agent-workspace`.
- The init event actually publishes the authoritative path at `memory_paths.auto` (stripping the `/memory/` suffix yields the transcript dir). `SystemEvent` struct does not parse this field.
- **Impact:** If `transcript_path` is wanted in Option 2, add `memory_paths.auto` to the `SystemEvent` parser; do not try to derive it by slug convention.

**V13 — Claude CLI subcommand audit is unverified.**
- `claude --help` in this environment shows only an interactive REPL; no top-level subcommand list is printable non-interactively from this sandbox.
- The init NDJSON carries slash commands (`cost`, `context`, etc.) but those are REPL slash commands, not host-invokable CLI subcommands.
- **Impact:** An authenticated user should run `claude --help` once to confirm no `claude status` / `claude session info` subcommand exists. If one does, Option 2's parser extensions may be partly redundant.

---

## 13. Confidence Assessment & Remaining Risks

**Confidence:** Medium–high. The phased direction (Option 2a → Option 2 → Option 1′, not Option 3b) is robust. The scratch-config-dir design for Option 1′ is new and depends on two factual open questions (Q-OPEN-1 and Q-OPEN-5) that each have a defined fallback, so neither can cause Phase 3 to be unshippable — only to pick a different sub-design.

**Remaining risks to call out before starting implementation:**

1. `--resume` NDJSON shape is unverified (V4). Block Phase 2 on a fixture capture.
2. Concurrency primitive choice for Aggregator (V8) — mutex recommended, but the decision should be re-validated with a microbenchmark if it becomes a concern on the hot path.
3. Live-token inflation (V3) — documentation decision, not an implementation one. Decide up front whether pr9k surfaces raw tokens only or also a percentage (recommend raw only).
4. Versioning treatment of the statusLine payload (V7) — decide whether to enlarge scope of public API before enlarging the payload. Especially acute if `claude.native` lands in Phase 3, because it means embedding Claude's own payload shape into pr9k's de-facto surface.
5. `claude --help` audit (V13 / Q-OPEN-8) — cheap verification that could change scope. Recommend running once before Phase 2 starts.
6. Transcript-path parser extension (V12) — minor, but do not claim `transcript_path` unless `memory_paths.auto` is parsed.
7. Q-OPEN-1 (profile adjacent-file reads) — blocks choosing between the scratch-dir mount and the file-level overlay fallback for Phase 3. Must be resolved before Option 1′ implementation starts.
8. Q-OPEN-5 (Docker Desktop macOS Unix-socket-over-bind-mount reliability) — blocks choosing between the Unix-socket IPC and the atomic-write+fsnotify fallback for Phase 3.
9. Maintenance cost of owning a Dockerfile — must be accepted as part of committing to Phase 3. Rebuild on base-image updates and on every `version.Version` bump; `sandbox_create` grows a `docker build` step that is new surface area for CI.
