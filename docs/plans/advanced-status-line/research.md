# Advanced Status Line — Feasibility Research

**Status:** All open questions resolved; Option 1″ selected as the single-shippable goal
**Date:** 2026-04-19
**Question:** Can pr9k's custom status line expose the same rich data that Claude Code feeds to its own native `statusLine` scripts — model, session id, workspace, cost, tokens, rate limits, etc.?

---

## 1. Executive Summary

**Feasibility verdict:** Yes, fully, as a **single self-contained feature** — not a phased rollout. The goal is to expose Claude-only fields (`rate_limits.*`, `cost.total_lines_added/removed`, full `context_window`, etc.) on pr9k's status line. That goal is only reachable by capturing Claude's own statusLine payload and forwarding it to pr9k — there is no "derive-from-stream" approximation that carries those fields. Earlier drafts proposed shipping three incremental phases (Option 2a → Option 2 → Option 1′) culminating in capture-and-forward; this plan drops that framing and ships capture-and-forward directly.

Eight open questions (Q-OPEN-1 through Q-OPEN-8) blocked the original Option 1′ sketch. All eight are now resolved — six by Claude-docs research, two by empirical tests against the running CLI (see Section 14). The resolutions collapse several pieces of the original design:

- **No custom Docker image required.** The base image (`docker/sandbox-templates:claude-code`, Ubuntu 25.10) already contains `sh`, `cat`, `mv`, and `socat`, and runs statically-linked Go binaries. A three-line shell shim written into the project bind mount at run start suffices — no `docker build` step, no derived tag, no version-coupled rebuilds (resolves Q-OPEN-6).
- **No Unix-domain socket IPC.** Unix sockets on Docker Desktop macOS bind mounts are still unreliable in 2026, even on VirtioFS. The shim does an atomic-rename JSON write into `.pr9k/`, and pr9k watches it with `fsnotify` — the fallback the original draft proposed is now the primary design (resolves Q-OPEN-5).
- **No profile-dir replacement.** pr9k does NOT build a scratch `CLAUDE_CONFIG_DIR`. The user's real `~/.claude` stays bind-mounted as today (preserving `sessions/`, `plugins/`, `statsig/`, resume continuity, and every other feature). pr9k adds a single **file-level bind-mount overlay** that replaces just `settings.json` inside the container for the duration of the run. The user's real `settings.json` on disk is never touched (resolves Q-OPEN-1).
- **No parse-merge logic.** The overlaid file is pr9k-authored from scratch. It contains a single `statusLine` block plus a `pr9kOwned: true` marker; Claude silently tolerates unknown sibling keys (empirically verified, resolves Q-OPEN-2). A minimal settings.json containing only a `statusLine` block starts cleanly (empirically verified, Q8).

**Selected design — Option 1″ (double-prime):** file-level bind-mount overlay of a pr9k-authored `settings.json`, three-line shell shim written into `.pr9k/`, atomic-rename JSON file IPC with `fsnotify` watcher, raw Claude payload embedded as `claude.native` in pr9k's stdin payload. Opt-in behind `statusLine.captureClaudeStatusLine: true` in `config.json`. Estimated ~235 LoC of Go + ~100 lines of docs (see Section 6.6 for LoC breakdown by file).

**Options NOT being shipped:**

| # | Option | Reason dropped |
|---|--------|----------------|
| 2a | Surface `Renderer.Finalize` as a summary string | Covers only already-known fields. User's goal is Claude-only fields; Option 2a does not contribute to reaching that goal. Leave as an optional future addendum if a narrow scope emerges. |
| 2 | Derive-from-stream — parser extensions + typed fields | Does not surface any Claude-only field. Adds parser complexity and concurrency surface for zero progress toward the goal. |
| 1 (original) | Capture-and-forward, writing into the user's real `~/.claude/settings.json` | Superseded by 1″'s file-level overlay — no write to user's real profile needed. |
| 1′ | Capture-and-forward via scratch `CLAUDE_CONFIG_DIR` + derived Docker image + Unix-socket IPC | Superseded by 1″ — each of its three pillars (scratch dir, image ownership, Unix socket) turned out to be avoidable once the open questions resolved. |
| 3b | Claude Agent SDK | Python-only; pr9k is Go. No Go SDK with session-state accessors exists. |

Section 14 contains the full open-question resolution log with cited evidence. Section 6 describes the selected Option 1″ design in detail. Section 8 is the implementation plan.

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

## 3. Option 2a — Surface `Renderer.Finalize` as a summary string (not shipping)

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

**Not shipping as a deliverable on the path to the goal.** Option 2a surfaces only fields pr9k already computes from the NDJSON stream — none of which are Claude-only fields. The goal (Section 1) is Claude-only fields (`rate_limits.five_hour.used_percentage`, `cost.total_lines_added/removed`, etc.), which Option 2a does not contribute to. May be added as an optional extra `claude.last_summary` field alongside `claude.native` if a specific request arises; not required.

---

## 4. Option 2 — Derive-from-Stream (not shipping)

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

**Not shipping.** Option 2 does not surface any Claude-only field (rate limits, lines added/removed, context-window truth from Claude's own bookkeeping). It only provides live approximations of fields Claude itself computes more accurately. Parser + concurrency cost is not justified by the incremental value when Option 1″ delivers 100% fidelity via capture-and-forward. May become useful in the future if pr9k wants to *also* surface between-trigger live approximations alongside the (more authoritative) `claude.native` payload — strictly optional polish.

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

*Superseded by Option 1″ (Section 6 below). The file-level settings-overlay variant makes capture-and-forward shippable at low cost, with no scratch directory, no derived image, and no Unix-socket IPC.*

---

## 6. Option 1″ — Capture-and-Forward (selected design)

**Premise (unchanged from Option 1):** install a pr9k-controlled shim as Claude's `statusLine.command`. Claude invokes the shim with its full native payload on stdin. The shim forwards the payload back to pr9k. pr9k embeds the payload in its own statusLine script's stdin JSON.

**What changed from Option 1′:** each of 1′'s three pillars collapsed once the open questions resolved. This section is the new selected design; the Option 1′ sketch that preceded it is preserved in the subsections below it as the reasoning trail.

### 6.0 Option 1″ — architecture at a glance

Three new artifacts, five integration points, one config flag. No Dockerfile, no Unix socket, no scratch profile dir.

**Artifacts written at container spawn (when `captureClaudeStatusLine: true`):**
1. `<projectDir>/.pr9k/sandbox-settings.json` — pr9k-authored Claude settings (single statusLine block + `pr9kOwned: true` marker).
2. `<projectDir>/.pr9k/statusline-shim.sh` — three-line shell script, mode 0755.
3. `<projectDir>/.pr9k/statusline-current.json` (runtime) — the latest Claude payload, atomically rewritten by the shim per refresh.

**Docker invocation gains one bind-mount when the flag is on:** `-v <projectDir>/.pr9k/sandbox-settings.json:/home/agent/.claude/settings.json` (file-level). This is layered on top of the existing profile directory mount, so Claude sees pr9k's `settings.json` inside the container while the user's real `~/.claude/settings.json` on disk stays untouched.

**Pr9k host-side additions:** an `fsnotify` watcher on `<projectDir>/.pr9k/` that updates `claudePayload atomic.Pointer[json.RawMessage]` on `statusline.Runner`; `BuildPayload` emits the payload as `claude.native` when non-empty.

**Gated entirely behind `statusLine.captureClaudeStatusLine: bool` in `config.json`**, default `false`. When the flag is off, pr9k behaves exactly as today.

### 6.1 The settings overlay

A file-level bind mount layered on top of a directory-level bind mount is supported on both Docker Desktop macOS and native Linux Docker; it is the same mechanism Docker Desktop uses to plumb through `docker.sock` (`https://docs.docker.com/desktop/release-notes/` release 4.39.0). Inside the container, `/home/agent/.claude/` still resolves to the user's real profile directory for every file EXCEPT `settings.json`, which is pr9k's authored file for the duration of the run.

pr9k's `settings.json` overlay content:

```json
{
  "statusLine": {
    "type": "command",
    "command": "/home/agent/workspace/.pr9k/statusline-shim.sh",
    "pr9kOwned": true
  }
}
```

The `pr9kOwned: true` sibling is tolerated by Claude — empirically verified against `claude` 2.1.114 in the sandbox image: the CLI's debug log emitted zero warnings when started against this file, and `claude auth status` / `claude -p …` both succeeded (Section 14, Q-OPEN-2). It serves as a self-signaling marker the shim's IPC consumer can assert on to reject a payload received via an unexpected code path.

**Why overlay instead of write-to-disk:** Claude tolerates a minimal settings.json (empirically verified, Section 14, Q8), so the overlay does not need to merge anything. Because the overlay lives only inside the container's mount namespace, pr9k never mutates the user's real `~/.claude/settings.json` — no parse-merge, no atomic rename on the host profile, no rollback hook, no "preserve user permissions" concern. Crash safety collapses to "on next startup, remove stale `.pr9k/sandbox-settings.json` if present"; there is no partial-write state to reconcile.

**Does the user's real profile stay fully functional?** Yes. The real profile directory is still bind-mounted at `/home/agent/.claude`, so `sessions/`, `plugins/`, `statsig/`, `.credentials.json`, and every other directory Claude reads are available unchanged (Section 14, Q-OPEN-1). `--resume` across steps still finds session transcripts where Claude wrote them, because the profile mount is rw and pr9k didn't replace it.

### 6.2 The shim

A three-line shell script is enough — no custom Docker image needed, because the base image (Ubuntu 25.10) already ships `sh`, `cat`, and `mv` (Section 14, Q-OPEN-6). pr9k writes the shim at container spawn:

```sh
#!/bin/sh
cat > /home/agent/workspace/.pr9k/statusline-current.json.tmp
mv /home/agent/workspace/.pr9k/statusline-current.json.tmp \
   /home/agent/workspace/.pr9k/statusline-current.json
```

**Why not Unix socket IPC:** Docker Desktop macOS still cannot reliably expose an AF_UNIX socket created on a host bind mount to a container process (Section 14, Q-OPEN-5). The forum-documented failure mode — `EINVAL` on `connect(2)` and corrupted mode bits — persists on VirtioFS in 2026 despite the 4.39+ file-level socket passthrough that addressed a different use case (mounting an existing socket file like `docker.sock`, not creating a socket in a shared directory). Atomic-rename file drops on a VirtioFS bind mount work correctly; the shim's `mv` is the reliable primitive.

**Why not a custom Dockerfile:** the entire purpose of owning the image would have been to bake in a shim binary with executable permissions. pr9k can set mode 0755 on the shim file when it writes it to `.pr9k/` on the host — Docker Desktop preserves file mode bits across VirtioFS for bind-mounted regular files. The container user's UID/GID is already mapped to the host user (`src/internal/sandbox/command.go` `-u <UID>:<GID>`), so the shim is executable inside the container without further handling.

**Static Go shim as a fallback:** if the shell shim proves flaky (e.g., if Claude's invocation shell is restricted or PATH doesn't find `sh`/`cat`), a statically-linked Go binary compiled with `CGO_ENABLED=0` runs cleanly in the base image (Section 14, Q-OPEN-6). Swap the three-line shell for a ~30-line Go binary; pr9k would still write it to `.pr9k/` rather than bake it into an image. This fallback is a drop-in replacement for the shim file and does not affect any other layer of the design.

### 6.3 The IPC (atomic-rename + fsnotify)

pr9k's host-side receiver:

1. On `Runner.Start` (when the feature is enabled), create an `fsnotify` watcher on `<projectDir>/.pr9k/`.
2. On `Create`/`Rename` events for `statusline-current.json`, read the file (~few KB), validate it parses as JSON, store it in `claudePayload atomic.Pointer[json.RawMessage]`.
3. On any change, call `Trigger()` so the next refresh tick picks up the new data. (The existing `Trigger()` drop-on-full channel is non-blocking; see E6.)
4. On shutdown, close the watcher, delete `statusline-current.json`, `sandbox-settings.json`, and the shim.

**Why atomic rename + fsnotify, not polling:** fsnotify is already the Go-ecosystem standard for file-change notifications; it maps to inotify on Linux and FSEvents on macOS, and works on Docker Desktop bind-mounted host directories (the pr9k-side watcher is running on the host, watching a host directory — not inside the container). A 300 ms debounce on refresh is well within human-perceptible latency; file-write latency on VirtioFS for a few-KB JSON file is in the low-ms range.

**Concurrency model:** `claudePayload atomic.Pointer[json.RawMessage]` is the single shared cell. Writer: fsnotify-loop goroutine. Readers: `execScript` goroutine (when building stdin payload for the user's statusLine script). No mutex needed; `atomic.Pointer` provides the release-acquire ordering this pattern requires.

**Failure modes:**
- Shim write races with fsnotify poll: atomic rename is atomic at the filesystem layer; the reader always sees either the old file or the new file, never a partial write. (Verified by the POSIX `rename(2)` contract; macOS VirtioFS preserves it.)
- Multiple claude step launches in sequence: each invocation overwrites the same `statusline-current.json`. The watcher sees successive `Rename` events and updates `claudePayload` each time. No cross-step payload leakage.
- pr9k crashes mid-step: stale `statusline-current.json` + `sandbox-settings.json` remain in `.pr9k/`. On next startup pr9k removes them before starting fresh. No user-visible artifact.

### 6.4 Host-side wiring summary

| Component | File | Change |
|---|---|---|
| Config schema | `src/internal/steps/steps.go:51-66` | Add `CaptureClaudeStatusLine *bool` to `StatusLineConfig`. |
| Validator | `src/internal/validator/validator.go:93-97` | Accept `captureClaudeStatusLine` field; no cross-field constraints beyond "bool or absent." |
| Sandbox invocation | `src/internal/sandbox/command.go` (`BuildRunArgs`) | When flag is on, write shim + settings overlay, add file-level bind mount, return the extra args. |
| Shim writer | New `src/internal/sandbox/statusline_overlay.go` | ~40 LoC: WriteFile shim (mode 0755) + settings.json (mode 0644), idempotent cleanup on startup. |
| fsnotify watcher | New `src/internal/statusline/claude_watcher.go` | ~80 LoC: watcher goroutine, close-on-shutdown, atomic.Pointer update. |
| Payload | `src/internal/statusline/payload.go:5-50` | Add `claude` sub-object with `native: json.RawMessage` (and keep room for future typed fields). |
| Runner | `src/internal/statusline/statusline.go` | Hold `claudePayload atomic.Pointer[json.RawMessage]`; wire watcher start/stop into `Start`/`Close`. |
| main.go | `src/cmd/pr9k/main.go:141-197` | Slot watcher start in `Runner.Start` after `SetModeGetter` (E50). |
| Default workflow script | `workflow/scripts/statusline` | Surface one or two Claude-only fields (e.g. `.claude.native.rate_limits.five_hour.used_percentage`) as a demo. |
| Documentation | `docs/features/status-line.md`, `docs/how-to/configuring-a-status-line.md` | New subsection: opt-in flag, `claude.native` payload shape, a small `jq` example. |

**No changes required to:**
- `internal/claudestream/` — parser unchanged; StepStats unchanged.
- `internal/workflow/` — Runner/Orchestrate unchanged.
- `internal/preflight/` — no new preflight check (credentials handling unchanged).
- Docker image selection — still `docker/sandbox-templates:claude-code`.
- `BuiltinEnvAllowlist` — no new env var crossing the container boundary.

### 6.5 Concrete payload surface

pr9k's stdin JSON to the user's statusLine script, with the flag on:

```json
{
  "sessionId":     "...",          // pr9k run stamp (existing)
  "version":       "...",
  "phase":         "iteration",
  "iteration":     1,
  "maxIterations": 5,
  "step":          { "num": 3, "count": 9, "name": "feature-work" },
  "mode":          "normal",
  "workflowDir":   "/.../workflow",
  "projectDir":    "/.../target-repo",
  "captures":      { "ISSUE_NUMBER": "42" },
  "claude": {
    "native": { ...Claude's full statusLine payload, verbatim... }
  }
}
```

The `claude.native` sub-object is absent entirely when (a) the flag is off, or (b) the flag is on but Claude has not invoked the shim yet (cold-start before the first assistant message). User scripts test for presence: `if .claude.native then ... else empty end`.

**Versioning treatment (resolves V7):** the pr9k statusLine payload is not part of the formal public-API list enumerated in `docs/coding-standards/versioning.md:13-28` today. Before shipping this feature, amend the versioning doc to either (a) add "statusLine stdin payload schema" to the versioned surface list, or (b) explicitly declare it documented-but-not-versioned. Recommendation: (b), with a stability note — "pr9k-authored keys are stable; `claude.native` is a pass-through of Claude's own schema, which pr9k does not control and which may change under Claude's own versioning independent of pr9k." This framing matches the precedent set for the captures map (also pass-through, also not versioned).

### 6.6 LoC and timeline estimate

| Layer | LoC | Notes |
|---|---|---|
| Validator field | 10 | `vStatusLine` addition + test |
| `StatusLineConfig` field | 5 | Pointer-bool field in struct + JSON tag |
| Sandbox overlay writer | 40 | Write shim + settings.json, chmod, cleanup |
| BuildRunArgs bind-mount extension | 15 | Conditional on flag |
| fsnotify watcher | 80 | Watcher goroutine, teardown, atomic.Pointer |
| statusline.Runner integration | 30 | Hold claudePayload, wire start/stop |
| Payload marshaling | 15 | Add `claude` field to JSON output |
| Default workflow script | 10 | Demo consumption of one Claude field |
| Tests (race detector, overlay, cleanup) | 100 | Integration tests |
| Documentation | ~100 (prose) | Feature doc + how-to + CLAUDE.md index entry |

**Total: ~305 LoC new Go + ~100 lines of prose.** Smaller than the ~580 LoC estimate for Option 1′ (no Dockerfile, no Unix-socket listener).

**Timeline estimate:** 1–2 days of focused implementation for the Go code, plus 0.5 day for docs. Test development adds another 0.5–1 day. Feasible as a single landable PR without pre-requisite work.

### 6.7 Remaining design decisions (non-blocking)

- **Shim language.** Shell (3 lines, requires `sh` + `cat` + `mv` in base image — confirmed present) vs statically-linked Go binary (~30 LoC, carries its own runtime). Shell is simpler; Go is more robust if the base image ever drops coreutils. Recommendation: ship shell; keep Go binary as a drop-in if needed.
- **Payload-size guard.** Claude's native payload is small (~1–2 KB), but pr9k should cap the stored size to e.g. 16 KB to avoid pathological cases. 8 KB is already the cap on statusLine script OUTPUT (E10); this is a separate cap on INPUT.
- **`claude.native` vs typed fields.** Shipping `native` as a raw pass-through (`json.RawMessage`) means user scripts read Claude's schema directly. Alternative: pr9k could also surface typed convenience fields like `claude.session_id`, `claude.model`, `claude.cost_usd`. Typed fields are optional — they duplicate what's already in `native`, but give users a less brittle surface if Claude changes its schema. Recommendation: ship `claude.native` only in the first cut; add typed fields on demand.
- **Refresh cadence interaction.** Claude invokes the shim on its own triggers (assistant message, permission mode change, vim toggle, and optional `refreshInterval` timer; Section 14, Q-OPEN-7). pr9k's own refresh cadence is independent. This is expected: `claude.native` updates when Claude says so, pr9k's other fields update when pr9k says so. Document this clearly so users understand why the Claude-side fields may lag.

### 6.8 Historical subsections (Option 1′ sketch — preserved as reasoning trail)

The subsections below describe the earlier Option 1′ sketch. They are preserved for context on why Option 1″'s simpler design was chosen, but they do not describe the implementation.

**Re-investigation prompt:** "Would Option 1 be easier if we had a full Dockerfile configuration? Or as an optional plugin that the end user explicitly enables? What is the most optimal capture-and-forward design?"

**Top-line answer:** The Dockerfile helps with two narrow concerns; explicit opt-in helps with two more; but the biggest unlock comes from a third dimension the original draft missed — redirecting `CLAUDE_CONFIG_DIR` at the container boundary to a pr9k-owned scratch directory, so pr9k never touches the user's real `~/.claude/settings.json` at all.

### 6.8.1 Dimension 1 — Does a pr9k-owned Dockerfile help? (Partial yes)

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

### 6.8.2 Dimension 2 — Does explicit user opt-in help? (Partial yes)

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

### 6.8.3 Dimension 3 — The unlock: a pr9k-owned scratch `CLAUDE_CONFIG_DIR`

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

### 6.8.4 The IPC question — how does the shim hand Claude's payload back to pr9k?

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

### 6.8.5 Recommended optimal design for Option 1′ (historical)

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

### 6.8.6 Verdict on Option 1′ (historical)

Superseded by Option 1″ (Sections 6.0–6.7). Option 1′ proposed a scratch `CLAUDE_CONFIG_DIR` + derived Docker image + Unix-socket IPC; all three pillars turned out to be unnecessary once the open questions resolved. See Section 14 for the evidence trail.

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

## 8. Implementation Plan — Single Deliverable (Option 1″)

No phases. A single landable PR that adds the opt-in `captureClaudeStatusLine` feature end-to-end. Work is sequenced for easy review, not split into separate releases.

### 8.1 Order of operations

1. **Settings-overlay + shim writer (sandbox layer).**
   - Add `CaptureClaudeStatusLine *bool` to `StatusLineConfig` (`src/internal/steps/steps.go:51-66`); default is `nil` meaning off.
   - Extend validator (`src/internal/validator/validator.go:93-97`) to accept the new field.
   - Create `src/internal/sandbox/statusline_overlay.go` — two small functions:
     - `WriteOverlay(projectDir string) (settingsPath string, shimPath string, err error)` — writes `settings.json` and `statusline-shim.sh` under `<projectDir>/.pr9k/`, mode 0644 and 0755 respectively. Caller passes the shim's container-absolute target path.
     - `CleanOverlay(projectDir string)` — removes all three files at startup (stale `sandbox-settings.json`, `statusline-shim.sh`, `statusline-current.json`) idempotently.
   - Extend `BuildRunArgs` to, when the flag is true, append `-v <settingsPath>:/home/agent/.claude/settings.json` to the docker argv.
   - Unit tests: overlay file contents round-trip, idempotent cleanup, flag-off is a no-op.

2. **Host-side watcher (statusline layer).**
   - Add `github.com/fsnotify/fsnotify` to `go.mod` (already a well-established Go module).
   - Create `src/internal/statusline/claude_watcher.go`:
     - `type claudeWatcher struct { watcher *fsnotify.Watcher; payload *atomic.Pointer[json.RawMessage]; done chan struct{} }`
     - `(*Runner).startClaudeWatcher(dir string) error` — starts fsnotify on `<projectDir>/.pr9k/`, filters for `statusline-current.json`, reads+validates+stores on change.
     - `(*Runner).stopClaudeWatcher()` — signals `done`, waits, closes watcher.
   - Wire `Start`/`Close` to the watcher lifecycle when the flag is on.
   - Tests: watcher starts and stops cleanly; atomic-rename write is observed; malformed JSON does not poison the pointer; concurrent reads/writes pass `-race`.

3. **Payload marshaling.**
   - Add `Claude *ClaudePayload` (with `Native json.RawMessage`) to the stdin JSON struct in `src/internal/statusline/payload.go:5-50`. Omit-empty so it's absent when no payload has arrived.
   - Tests: payload round-trip; absent field when flag is off; present field when pointer is set.

4. **Default workflow script update.**
   - `workflow/scripts/statusline` — add a small `jq` snippet that surfaces one or two Claude fields (e.g. `rate_limits.five_hour.used_percentage`) as a demonstration.
   - Test: script runs with the expanded payload without errors.

5. **Documentation + ADR note.**
   - New section in `docs/features/status-line.md` describing `captureClaudeStatusLine`, the overlay mechanism, and the `claude.native` pass-through.
   - New how-to: `docs/how-to/capturing-claude-statusline-data.md` with end-to-end example.
   - Add CLAUDE.md index entry for the new how-to.
   - Amend `docs/coding-standards/versioning.md:13-28` to declare `claude.native` a pass-through, documented-but-not-versioned.
   - No new ADR required (the feature is additive, gated, reversible, and contained within the existing sandbox contract).

6. **Release notes, version bump.**
   - Minor bump under 0.y.z rules (`docs/coding-standards/versioning.md:13-28`); this is additive public-API growth (new `config.json` schema field + payload extension).

### 8.2 Tradeoffs and known limitations

- **Claude-only fields update only when Claude invokes the shim.** Claude's refresh triggers are: after each assistant message, permission mode change, vim toggle, and optional `refreshInterval` timer. Between triggers, the `claude.native` payload in pr9k's output reflects the most recent trigger. During silent step work (thinking, tool execution with no assistant message yet), the payload is stale. This is inherent to capture-and-forward and cannot be improved without deriving state from the NDJSON stream (which would still not cover rate_limits / lines_added / lines_removed).
- **Cold-start gap.** Before Claude's first assistant message in a step, `claude.native` is absent. User scripts must tolerate the field being missing — document this explicitly.
- **Opt-in only.** When the flag is off, nothing in pr9k's behavior changes; the overlay bind-mount is not added, no shim files are written, no watcher runs. Opt-in is the right default because the feature ties pr9k's output to a Claude-schema shape that Claude controls.
- **Payload surface versioning.** `claude.native` is a pass-through of Claude's own payload. pr9k does NOT promise compatibility across Claude versions for the contents of that object; user scripts that consume specific Claude fields take on the coupling. This matches the treatment of `captures` (also pass-through).
- **Silent JSON tolerance.** Empirical testing revealed Claude silently tolerates malformed settings.json (Section 14, Q8 bonus control). pr9k should round-trip-read its own `sandbox-settings.json` after writing to detect corruption — a 10-LoC defensive check. Recommended.
- **No Claude-on-host support.** This plan assumes Claude runs inside the Docker sandbox (the current pr9k model; ADR `docs/adr/20260413160000-require-docker-sandbox.md`). If Claude is ever run directly on the host, the overlay mechanism would need to be rethought (likely as project-local `.claude/settings.json` instead — see Section 14, Q-OPEN-3 findings). Out of scope for now.

### 8.3 Deferred / future work

These items are NOT shipping in the same PR but are enabled by it:
- **Typed convenience fields** (`claude.session_id`, `claude.model`, `claude.cost_usd`, etc.) marshaled alongside `claude.native`. Add on user demand.
- **Option 2a (Renderer.Finalize summary string)** as `claude.last_summary`. Cheap addition if users want a compact post-step line alongside the raw `claude.native`.
- **Option 2 (derive-from-stream)** typed fields. Not useful once `claude.native` is in place, except for the one narrow case of providing *between-trigger* live approximations — a second-order polish.

None of the above are on the critical path; each is a future enhancement if the base feature surfaces real user need.

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

**Confidence:** High. All eight open questions (Q-OPEN-1 through Q-OPEN-8) that blocked the original Option 1′ sketch are now resolved — six via Claude docs research, two via empirical tests against the CLI in the sandbox image (see Section 14). Each resolution *simplifies* the design rather than forcing a fallback; the plan in Section 8 therefore has no "if X, then Y" branches contingent on further investigation.

**Remaining risks for Option 1″ implementation:**

1. **Silent settings-parse failures** — empirical bonus finding: Claude silently tolerates malformed `settings.json`. If pr9k writes a corrupted overlay, Claude may start but the statusLine shim won't be registered. Mitigation: pr9k round-trip-reads its own `sandbox-settings.json` after writing (10 LoC defensive check). Cheap, recommended.
2. **Docker Desktop macOS bind-mount regressions** — Docker Desktop ships bind-mount changes release-to-release; the general "file overlay onto a bind-mounted directory" mechanism has been stable, but any new regression would surface as "Claude doesn't see the overlay." Integration test on both macOS and Linux CI agents mitigates. Low risk.
3. **Claude payload schema evolution** — `claude.native` is a verbatim pass-through. If Claude's statusLine schema changes, user scripts that read specific nested fields break. pr9k cannot prevent this. Mitigation: version-note in `status-line.md` naming `claude.native` as "pass-through; coupling is the user script's responsibility." Documented in Section 8.2.
4. **Versioning doc amendment (V7)** — before shipping, amend `docs/coding-standards/versioning.md` to classify `claude.native` as a pass-through surface. Blocker for the PR, not for the design. Small.
5. **Opt-in friction** — some users won't realize the flag exists and will wonder why they don't see Claude-only fields. Mitigation: mention the flag prominently in `docs/how-to/configuring-a-status-line.md` and in the default workflow's `statusLine` block as a commented-out example. Documentation concern, not implementation.
6. **Cold-start gap** — `claude.native` is absent until Claude's first shim invocation (~after the first assistant message). User scripts must tolerate the field being missing. Document this explicitly and test with a `jq` expression that handles both cases.
7. **Shell-shim portability** — if Claude ever runs the statusLine command through a restricted shell that omits coreutils, the three-line `/bin/sh` shim fails. Fallback: swap in the static Go binary variant (Section 6.2). Drop-in replacement; no other layer changes.

All other earlier risks (V1–V13 and the old Q-OPEN set) either no longer apply to Option 1″ (they concerned Options 2/2a) or are resolved (see Section 14). The full reasoning trail is preserved in Sections 11 and 12 for future readers.

---

## 14. Investigation Results — Open Question Resolutions

All eight open questions from the original Option 1′ sketch are resolved. Two were answered empirically by running the Claude CLI in the sandbox image (Q-OPEN-2, and Q-OPEN-7 / V4); six were answered from Claude Code, Docker Desktop, and Docker Hub documentation. Each entry below cites the evidence source and records the resulting design decision.

### Q-OPEN-1 — Does Claude read profile-dir state files beyond `.credentials.json`?

**Answer:** Yes, but only a bounded set.

**Evidence (from https://code.claude.com/docs/en/claude-directory.md):**
- **Required at startup:** `.credentials.json` (auth), `settings.json` / `settings.local.json` (policy).
- **Required for `--resume`:** `sessions/` directory contains session transcripts; Claude reads from this when resuming by session ID.
- **Optional (read if present):** `plugins/` (plugin configuration), `.claude.json`, CLAUDE.md (context file).
- **Output-only / cache:** `projects/`, `teams/`, `debug/`, `file-history/`, `backups/`, `shell-snapshots/`, `statsig/`, `telemetry/`, `tasks/`, `todos/`, `rule-violations.md`, `stats-cache.json`, `mcp-needs-auth-cache.json`, `history.jsonl`.

**Impact on design:** a scratch `CLAUDE_CONFIG_DIR` design would need to recreate `sessions/`, `plugins/`, and possibly `.claude.json` — cost and correctness risk. A **file-level overlay that masks only `settings.json`** keeps everything else intact and is therefore the chosen approach. See Section 6.1.

### Q-OPEN-2 — Does Claude accept unknown sibling keys under `statusLine`?

**Answer:** Yes. Silently tolerated.

**Evidence (empirical, Claude 2.1.114 in `docker/sandbox-templates:claude-code`):** wrote a scratch `settings.json` containing `{"statusLine": {"type":"command", "command":"/bin/echo test", "pr9kOwned":true, "pr9kMarker":"safe-to-remove"}}`. Ran `claude auth status` (exit 0), `claude -p "hi" --output-format stream-json --verbose` (emitted normal `system/init` event), and inspected the `--debug-file` output: zero lines mentioning `pr9k`, `unknown`, or `schema`. The settings watcher registered the file cleanly: `[DEBUG] Watching for changes in setting files /home/agent/.claude-scratch/settings.json...`.

**Impact on design:** pr9k safely adds a `pr9kOwned: true` marker to the overlay to self-signal ownership. See Section 6.1.

**Bonus empirical finding:** Claude silently tolerates *malformed* JSON in settings.json as well — a deliberately corrupted file produced no error and no warning in the debug log. pr9k should round-trip-read its own written overlay as a defensive check (listed in Section 13 as remaining risk #1).

### Q-OPEN-3 — Does Claude accept `statusLine.command` from a project-local `.claude/settings.json`?

**Answer:** Yes. Precedence: Managed > project-local `settings.local.json` > project-local `settings.json` > user-global `~/.claude/settings.json`.

**Evidence (https://code.claude.com/docs/en/settings.md, "Settings precedence"):**
> "Settings apply in order of precedence. From highest to lowest: 1. Managed settings, 2. Local settings (`.claude/settings.local.json`), 3. Project settings (`.claude/settings.json`), 4. User settings (`~/.claude/settings.json`)."

**Impact on design:** a project-local overlay variant is technically possible, but it would require pr9k to write a `.claude/` directory into the user's project root — either polluting the repo or requiring a git-ignore setup. The **file-level bind-mount overlay over `/home/agent/.claude/settings.json`** is cleaner because it leaves zero on-disk trace in the user's project. See Section 6.1.

### Q-OPEN-4 — Is there a `CLAUDE_STATUSLINE_COMMAND` env var or equivalent?

**Answer:** No.

**Evidence (https://code.claude.com/docs/en/env-vars.md):** the env-vars reference enumerates all Claude Code env vars; there is no `CLAUDE_STATUSLINE*` or equivalent. Status-line configuration is exclusively via `settings.json`.

**Impact on design:** the overlay approach is mandatory; there is no env-var shortcut to bypass settings.json writing.

### Q-OPEN-5 — Is Unix-domain socket over a bind-mount reliable on Docker Desktop macOS?

**Answer:** No, still unreliable in 2026.

**Evidence:**
- Docker Community Forums, July 2024, [Unix socket on bind mount](https://forums.docker.com/t/unix-socket-on-bind-mount/142653): creating an AF_UNIX socket on a bind-mounted host directory yields `EINVAL` and corrupted `stat(2)` mode bits. Community guidance: create sockets inside the container FS or a named volume.
- Docker Desktop 4.39.0 release notes: added Unix-socket support, but only for mounting an *already-existing socket file* by its full path (e.g. `-v /host.sock:/container.sock`) — not for sockets created in a shared directory.
- [docker/for-mac #4995](https://github.com/docker/for-mac/issues/4995) (gRPC-FUSE socket blocked); [for-mac #6243](https://github.com/docker/for-mac/issues/6243), [#6614](https://github.com/docker/for-mac/issues/6614) (VirtioFS permissions): even on the current default VirtioFS backend, AF_UNIX socket in bind-mounted dir is not a first-class feature.

**Impact on design:** **atomic-rename JSON file + fsnotify** is the primary IPC. It works reliably on VirtioFS bind mounts. See Section 6.3. The original Option 1′ Unix-socket plan is dropped entirely.

### Q-OPEN-6 — Does `docker/sandbox-templates:claude-code` support statically-linked Go binaries and have shell tooling?

**Answer:** Yes to Go, and yes to shell tooling — `sh`, `cat`, `mv` are present. `socat` present. `nc`/`netcat` absent.

**Evidence (empirical, via `docker inspect` and `docker run`):**
- Base OS: Ubuntu 25.10 (Questing Quokka). Confirmed by `/etc/os-release` inside the container and by the image labels `com.docker.sandboxes.base=ubuntu:questing`, `org.opencontainers.image.version=25.10`.
- Static Go binary test: compiled `hello.go` with `CGO_ENABLED=0 GOOS=linux GOARCH=arm64`, bind-mounted and executed inside the image; output `ok`, plus `unix-listen: ok` from `net.Listen("unix", ...)` — Unix socket syscalls work inside the container.
- `sh` at `/usr/bin/sh`, `bash` at `/usr/bin/bash`, `socat` at `/usr/bin/socat`. `nc`/`ncat`/`netcat` all absent from `/usr/bin`; not in dpkg list.
- Image config: `WorkingDir=/home/agent/workspace`, `User=agent` (non-root), `Cmd=["claude","--dangerously-skip-permissions"]`, no Entrypoint.

**Impact on design:** **no custom Dockerfile needed.** The three-line `/bin/sh` shim suffices. If coreutils ever disappear from the base image, swap in a ~30-LoC static Go binary written by pr9k — no image changes required. See Section 6.2.

**Note:** Dockerfile source for `docker/sandbox-templates:claude-code` is NOT publicly available (no GitHub repo; Docker Hub overview is empty). Users extend via `FROM docker/sandbox-templates:claude-code` — reinforces the "derive nothing, overlay everything" approach.

### Q-OPEN-7 — Refresh cadence and payload shape per invocation

**Answer:** Claude invokes the shim on: (1) each new assistant message, (2) permission mode change, (3) vim mode toggle, (4) optional `refreshInterval` timer. 300 ms debounce. If a new trigger fires while the shim is still running, Claude cancels the in-flight execution. The stdin payload schema is identical every call.

**Evidence (https://code.claude.com/docs/en/statusline.md, "When it updates"):**
> "Your script runs after each new assistant message, when the permission mode changes, or when vim mode toggles. Updates are debounced at 300ms... If a new update triggers while your script is still running, the in-flight execution is cancelled."

**Impact on design:** the atomic-rename + fsnotify IPC is comfortably within tolerance — ~few-KB JSON writes take low-ms on VirtioFS. No bandwidth concern. Because Claude cancels in-flight shims, the shim must be idempotent in its file write (the rename is atomic; either the new file lands or nothing changes).

### Q-OPEN-8 / V13 — Does the Claude CLI have a non-interactive subcommand for session state?

**Answer:** No. Only `claude auth status` is JSON-printable and it returns auth state only.

**Evidence (https://code.claude.com/docs/en/cli-reference.md):** lists `claude auth status`, `claude agents`, `claude auto-mode defaults`, `claude update`. No `claude status`, `claude session info`, or similar for live session metrics.

**Impact on design:** the capture-and-forward shim is the only path to Claude-side session state. There is no CLI-invocation shortcut that could replace it.

### V4 — Does `claude --resume` emit an `init` event with the resumed `session_id` and `model`?

**Answer:** Yes. Verified empirically.

**Evidence (empirical, Claude 2.1.114 in `docker/sandbox-templates:claude-code`):**
- Stream 1 (fresh session): first event `{"type":"system","subtype":"init","session_id":"2144ce37-51e8-4c4c-beaf-44b3a44edc5a","model":"claude-opus-4-7[1m]","claude_code_version":"2.1.114",...}`. Final `result` event: matching `session_id`.
- Stream 2 (resumed with `--resume 2144ce37-51e8-4c4c-beaf-44b3a44edc5a`): first event is again a `system/init` with `session_id=2144ce37-...` (matches the resumed ID) and `model=claude-opus-4-7[1m]`.
- Schemas are identical between fresh and resumed streams; resumed streams additionally carry an expanded `tools` / `mcp_servers` list and a new `uuid` (environmental, not session-identifying).

**Impact on design:** **not blocking for Option 1″** (the shim captures whatever Claude sends, regardless of whether it's a fresh or resumed session). This finding matters mainly for future "derive from NDJSON stream" features (Option 2), confirming that init-event parsing is a viable signal for both fresh and resumed sessions.

### Summary of resolved questions

| ID | Question | Answer | Design impact |
|---|----------|--------|---------------|
| Q-OPEN-1 | Claude reads profile state beyond `.credentials.json`? | Yes — `sessions/`, `plugins/`, etc. | Use file-level overlay, NOT scratch dir |
| Q-OPEN-2 | Unknown keys under `statusLine` tolerated? | Yes (empirical) | Safe to add `pr9kOwned` marker |
| Q-OPEN-3 | Project-local settings.json works? | Yes — but overlay is cleaner | Overlay chosen; project-local noted as alternative |
| Q-OPEN-4 | Env var bypasses settings.json? | No | Overlay is mandatory path |
| Q-OPEN-5 | Unix socket over bind-mount reliable? | No (macOS Docker Desktop) | Use atomic-rename + fsnotify |
| Q-OPEN-6 | Base image runs static Go / has sh? | Yes to both | No custom Dockerfile |
| Q-OPEN-7 | Refresh cadence + payload shape | 300ms debounce, 4 triggers, identical schema | No bandwidth concern |
| Q-OPEN-8 | Non-interactive CLI subcommand for state? | No | Capture-and-forward is only path |
| V4 | `--resume` emits init event? | Yes (empirical) | Not blocking; useful for future work |
| V7 | Payload versioning treatment | Decision pending in PR | Amend `versioning.md` before ship |
| V13 | `claude --help` audit | Done (same as Q-OPEN-8) | No subcommand exists |
