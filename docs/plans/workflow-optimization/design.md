# Ralph Workflow Optimization — Design

Evidence-based plan to cut per-iteration wall-clock time and token burn by
restructuring both the workflow definition (`ralph-steps.json`, `prompts/`) and
the `ralph-tui/` Go code that drives it.

## Iterative review summary

This plan went through three iterations of adversarial and evidence-based
review, with live CLI smoke tests for open ambiguities.

- **Iterations:** 3 (stopped early; plan reached stability).
- **Assumptions challenged:** 14 total (9 primary, 5 secondary). 5 primary
  assumptions were refuted and fixed in place:
  - A1 (capture-mode) → §3.1.b `captureMode: "fullStdout"` with 32 KiB cap.
  - A6 (timeout mechanism) → §3.9 routes through existing
    `currentTerminator`, not `exec.CommandContext`.
  - A7 (bare-sentinel matcher) → §3.3.a switches to empty-capture primitive
    via `scripts/review_verdict` (bash).
  - A8 (wrong deferred tool list) → §3.8 corrected to `TodoWrite` only.
  - (Q2 / 256 KiB cap) → lowered to 32 KiB after measuring real issue
    bodies (max observed 39 KB).
- **Consolidations:** §3.5's script now reads `.ralph-cache/iteration.jsonl`
  after §3.6 ships (not `progress.txt`). §3.3 dropped a
  `SkipCondition{Var,Equals}` struct in favour of reusing the
  `empty-capture` convention.
- **Ambiguities resolved with evidence:**
  - Q1 (`--model` on resume): **overrides**, verified via live smoke test
    against a real claude CLI session — memory preserved, model switched.
  - Q2 (capture cap): **32 KiB** — measured against 100+ real tool_result
    blobs in gearjot-v2 logs.
  - Q3 (shell vs Go): **shell** — matches all 6 existing scripts.
- **Agent validation — evidence-based investigator:** verified 6 of 7
  claims, flagged 2 overstatements (F2: `BreakLoopIfEmpty` reuse language;
  F3: 256 KiB scanner buffer is per-line not total), flagged 1 precondition
  (F1: `DisallowUnknownFields` coupling between `steps.go` and
  `validator.go`). All incorporated.
- **Agent validation — adversarial validator:** surfaced 11 failure
  modes; 10 drove plan edits (V1–V11), one (Phase-A "zero schema changes"
  labeling) produced a complete §6 rewrite. Confidence: Medium → High
  after edits.

See §7 for the full assumptions log, overlap findings, and resolved
ambiguities with their evidence.

## 1. Evidence summary

Analyzed across these production runs:

- `logs/ralph-2026-04-16-151704.311/` — pr9k, 4 iterations
- `logs/ralph-2026-04-16-125353.773/` — pr9k, multi-iteration
- `~/dev/gearjot/gearjot-v2/logs/ralph-2026-04-16-153328.268/` — 8 iterations
- `~/dev/gearjot/gearjot-v2/logs/ralph-2026-04-17-081916.744/`
- `~/dev/gearjot/gearjot-v2-web/logs/ralph-2026-04-17-123634.547/`
- `~/dev/gearjot/gearjot-v2-web/logs/ralph-2026-04-17-151230.064/`
- `~/dev/gearjot/gearjot-v2-web/logs/ralph-2026-04-16-162912.444/`

### 1.1 Baseline step durations (minutes)

| Step              | Avg | Min | Max  | Variance |
| ----------------- | --- | --- | ---- | -------- |
| Feature work      | 8.2 | 0.6 | 26.7 | **26.1** |
| Test planning     | 4.7 | —   | —    | 9.7      |
| Test writing      | 4.9 | 0.4 | 21.9 | **21.5** |
| Code review       | 3.5 | —   | —    | 6.3      |
| Fix review items  | 2.4 | —   | —    | 5.7      |
| Update docs       | 2.5 | —   | —    | 5.2      |

**Average iteration:** ~22.7 min (range 5.7 → 66.2 min). Feature work and
test writing together consume ~60% of iteration time.

### 1.2 Concrete waste, validated from `gearjot-v2/logs/ralph-2026-04-16-153328.268` (8 iterations)

| Pattern                                          | Raw count | Per iteration |
| ------------------------------------------------ | --------- | ------------- |
| `gh issue view …` invocations                    | 32        | 4×            |
| `git log` / `git diff` invocations               | 135       | ~17×          |
| `go env` probes                                  | 19        | ~2×           |
| `GOPATH`/`GOCACHE`/`GOMODCACHE` env prefixing    | 218       | ~27×          |
| `"permission denied"` error strings              | **88**    | ~11×          |
| `go test …` invocations across test+fix steps    | 77 (in 3 iter) | ~26× per iter |
| `gh issue comment` / `gh issue edit`             | ~5–6 per iteration | — |
| `git commit` per iteration                       | 3–5       | —             |
| Unique `session_id` per step (confirmed cold)    | 1         | —             |

Cross-run confirmation: in `pr9k/logs/ralph-2026-04-16-151704.311/` the
test-suite ran **77 times** and lint/vet/fmt **42 times** across 3 iterations —
one logical change set, ~120 re-verifications.

### 1.3 Structural root causes (code evidence)

1. **Fresh Claude session per step.** `ralph-tui/internal/sandbox/command.go:53-61`
   builds `claude … -p <prompt>` with no `--resume` / `--session-id`. Each step
   loads the repo cold. Validated by 1 unique `session_id` per `.jsonl`.

2. **No env var injection to kill Go permission errors.** `BuildRunArgs` in
   `ralph-tui/internal/sandbox/command.go:21-64` sets only `CLAUDE_CONFIG_DIR`.
   The container image has no `GOPATH`/`GOCACHE` pointing at a writable path,
   forcing Claude to prefix every `go` command with `GOPATH=/tmp/... GOCACHE=/tmp/...`
   and burn 2–3 retries on `permission denied` before finding the pattern.
   88 permission-denied hits in a single run.

3. **Every prompt re-asks Claude to re-learn.** `prompts/feature-work.md` and its
   five siblings total **67 lines**. They tell Claude the goal but hand it zero
   precomputed context — the issue body, the starting diff, the repo's lint
   command, the coding standards — so every step independently runs `gh issue
   view`, `git log`, `git diff`, `ls`, `find`.

4. **Two-step commit-ping-pong between file and memory.** Steps persist state
   via `progress.txt` / `test-plan.md` / `code-review.md` files in the workspace
   and re-read them with `@file` in the next prompt. This works, but Claude
   *also* re-reads all the source files each step touched, because the file is
   the only bridge and it's not authoritative.

5. **Redundant verification.** `test-writing.md:4` and `code-review-fixes.md:4`
   both say *"Run all tests, type checks, linting and formatting tools"*.
   Code review runs between them. Linters/tests run twice on effectively the
   same tree.

6. **Redundant GitHub updates.** Every prompt ends *"Update the github issue
   #{{ISSUE_ID}} with what was done"* → 5 comments per iteration. These are
   mostly identical summaries of work-in-progress that no human reads.

7. **ToolSearch preamble burn.** The container's Claude sees tools deferred
   behind `ToolSearch`; 21 `ToolSearch` calls per 8-iteration run means ~3 per
   step just to load `select:Bash,Read,Edit,Grep,Glob` before useful work.

---

## 2. Previous-analysis validation

| Claim                                              | Verdict | Notes |
| -------------------------------------------------- | ------- | ----- |
| Every phase is a fresh session                     | ✅       | Confirmed from code + `session_id` uniqueness. |
| "139× go env probes per 8-iteration run"           | ⚠️ Inflated | Actually 19 direct `go env`, but **218** `GO*=…` env-var prefix hacks — the *spirit* is right, the number is wrong. The real fix is still valid. |
| 32× gh issue view per run                          | ✅       | 32 exact. |
| 18× git log/diff per iteration                     | ✅       | 135 / 8 ≈ 17. |
| Feature-work & test-writing dominate               | ✅       | 8.2 min / 4.9 min averages; both >20 min variance. |
| Prompts are bare-bones                             | ✅       | 67 lines across 8 prompts. |
| Pre-compute iteration context bundle               | ✅ Adopt  | Highest ROI; see §3.1. |
| Set `GOPATH`/`GOMODCACHE`/`GOCACHE` in container   | ✅ Adopt, revise | Do it in `BuildRunArgs` rather than the image, so non-Go projects aren't affected. |
| Inject `test-plan.md`/`code-review.md` via prompt  | ⚠️ Partial | They're already injected via `@file` in the prompt preamble. Real win is in *also* injecting the diff + issue body so Claude doesn't re-run `gh`. |
| Drop "update the GitHub issue" from every prompt   | ✅ Adopt  | Move to one post-iteration step. |
| Cache coding-standards + project-discovery         | ⚠️ Project-specific | Valid for Go repos with stable standards; not universal. Make it a *captured var* pattern, not a hard-coded assumption. |
| Suppress ToolSearch preamble                       | ⚠️ Partial | Not a ralph-tui bug; can nudge Claude with a prompt hint listing the tools to preload. Low-ROI but cheap. |
| Emit structured summary.jsonl                      | ✅ Adopt  | Replace `progress.txt` append-soup with a per-step structured record that `lessons-learned` / `deferred-work` can consume. |

---

## 3. Proposed changes

Optimizations are ordered by expected ROI. Each names the files to change.

### 3.1 Precompute an iteration-context bundle (biggest win)

**Goal:** every Claude step starts with issue body, starting diff, and project
commands already in the prompt, so it doesn't re-run `gh issue view`, `git
log`, `git diff`, `find`, `ls` on every cold boot.

#### 3.1.a Blocker discovered during review — capture-mode limitation

The existing non-claude capture path (`workflow.go:495`) calls
`lastNonEmptyLine(capturedLines)` — it binds **only the last non-empty stdout
line** to `LastCapture`. Documented at `orchestrate.go:10`:
*`CaptureLastLine` … binds the last non-empty stdout line*. That's perfect for
`scripts/get_gh_user` (single-line `"mxriverlynn"`) and `scripts/get_next_issue`
(single-line `"106"`), which is what the current workflow uses it for.

It is **not** sufficient for the multi-line payloads §3.1 needs:
`gh issue view` prints the issue body (paragraphs), `git diff --stat` prints
several lines per file, and `scripts/project_card` emits ~40 lines.

Without addressing this, `{{ISSUE_BODY}}` in the prompt would expand to a
single line of trailing whitespace and defeat the whole optimization.

#### 3.1.b Fix — add a `captureMode` field on `Step`

Extend the step schema with an explicit capture mode (defaults to the
existing single-line behaviour so no existing workflow breaks):

```json
{ "name": "Get issue body", "isClaude": false,
  "command": ["gh", "issue", "view", "{{ISSUE_ID}}", "--json", "title,body", "-t", "{{.title}}\n\n{{.body}}"],
  "captureAs": "ISSUE_BODY",
  "captureMode": "fullStdout" }
```

**Go change — `ralph-tui/internal/steps/steps.go`:**

```go
type Step struct {
    ...
    CaptureAs   string `json:"captureAs,omitempty"`
    CaptureMode string `json:"captureMode,omitempty"` // "" → "lastLine" (default); "fullStdout"
}
```

**Go change — `ralph-tui/internal/workflow/workflow.go`:** the non-claude
`forwardPipe(stdout, true)` path (line ~456–499) retains all lines in
`capturedLines`. When the step's `CaptureMode == "fullStdout"`, set
`r.lastCapture = strings.Join(capturedLines, "\n")` instead of
`lastNonEmptyLine(capturedLines)`. The existing 256 KiB scanner buffer is
sufficient for the payloads in this plan; no buffer bump needed.

**Go change — `ralph-tui/internal/validator/`:** reject any
`CaptureMode` value outside `{"", "lastLine", "fullStdout"}`; reject
`captureMode` on claude steps (they go through the claudestream aggregator and
capture the `result` field, a separate path). Cap captured value size at
**32 KiB** post-join (resolved Q2 below — 1.6× the largest observed `gh issue
view` body in production logs; low enough to bound cumulative amplification
per V1). When a capture exceeds 32 KiB, keep the first 30 KiB verbatim and
append `"\n\n[…truncated, full body at github.com/<repo>/issues/{{ISSUE_ID}}]"`.

#### 3.1.c Workflow change — `ralph-tui/ralph-steps.json`

Per V2: the branch-diff capture has a temporal scope problem. Capturing once
at iteration start means feature-work sees an empty diff (no commits yet)
and everything after feature-work sees a stale diff (unless re-captured).
Solution: capture at two temporal points and only inject the relevant one
into each prompt.

Add these capture steps:

```json
{ "name": "Get issue body",      "isClaude": false,
  "command": ["gh", "issue", "view", "{{ISSUE_ID}}", "--json", "title,body", "-t", "{{.title}}\n\n{{.body}}"],
  "captureAs": "ISSUE_BODY",     "captureMode": "fullStdout" },
{ "name": "Get project card",    "isClaude": false,
  "command": ["scripts/project_card"],
  "captureAs": "PROJECT_CARD",   "captureMode": "fullStdout" }
```

— placed before `Feature work`. Then, between `Feature work` and
`Test planning`, add:

```json
{ "name": "Get post-feature diff", "isClaude": false,
  "command": ["git", "diff", "{{STARTING_SHA}}..HEAD", "--stat"],
  "captureAs": "PRE_REVIEW_DIFF",  "captureMode": "fullStdout" }
```

The rename `PRE_REVIEW_DIFF` makes the temporal scope explicit: this is the
diff state going INTO review, not an iteration-wide truth.

**New script — `scripts/project_card`:** bash (per Q3). Emits a short
(~40-line) card with build/test/lint commands, repo root, primary
languages. Reads Makefile / `package.json` / `pyproject.toml` / `Cargo.toml`
as found — no hard-coded language assumption.

**Prompt change — prompts that run BEFORE commits (feature-work.md,
test-planning.md):** inject only the stable context:

```md
# Context
Issue #{{ISSUE_ID}}: {{ISSUE_BODY}}
Project card:
{{PROJECT_CARD}}
```

**Prompt change — prompts that run AFTER at least one commit (code-review-changes.md,
code-review-fixes.md, update-docs.md):** also inject the review diff:

```md
# Context
Issue #{{ISSUE_ID}}: {{ISSUE_BODY}}
Project card:
{{PROJECT_CARD}}
Diff since iteration start:
{{PRE_REVIEW_DIFF}}
```

`test-writing.md` is intermediate — it runs after feature-work's commit but
before the review-diff capture would be most useful. Decision: inject
`{{PRE_REVIEW_DIFF}}` into test-writing as well; test-writing authors need
to know what code was written.

#### 3.1.d Guard — argv size

Multi-KiB env/args are fine in the shell, but the prompt is passed as a
Docker argv string (`sandbox/command.go:58` — `"-p", prompt`). A 100 KiB
issue body inside an already-long docker invocation nears `ARG_MAX`.
Mitigation: if the composed prompt exceeds **128 KiB**, write it to a file
inside the bind mount and pass `claude -p "$(cat /home/agent/workspace/.ralph-prompt)"`
instead — but doing that preserves the single invocation shape, not changes it.
Implement only if the 256 KiB capture cap isn't enough in practice; don't
pre-optimize.

**Expected impact:** eliminates ~4× `gh issue view` + ~6× `git log/diff` per
step × 6 steps = 60 tool calls saved per iteration; shaves ~30–60 s off each
step start.

---

### 3.2 Extend `env` schema so workflows can inject container env (generic)

**Principle:** ralph-tui must stay language-agnostic. The evidence for the
`GOPATH`/`GOCACHE`/`GOMODCACHE` / `"permission denied"` storm came entirely
from Go projects (gearjot-v2, ralph-tui itself). Hard-coding those names into
`internal/sandbox/command.go` would drag Go-specific knowledge into the
generic runner — the wrong place. Python projects want `PIP_CACHE_DIR`,
`POETRY_CACHE_DIR`; Node wants `NPM_CONFIG_CACHE`, `YARN_CACHE_FOLDER`; Rust
wants `CARGO_HOME`, `CARGO_TARGET_DIR`. None of these belong in ralph-tui.

**The right split:**
- `ralph-tui` exposes a mechanism — "set arbitrary env vars inside the sandbox".
- Each *workflow* (`ralph-steps.json`) decides *which* env vars to set for
  *its* projects.
- Language-specific caches are thus a per-workflow optimization, documented
  in a how-to guide, not wired into the Go code.

#### 3.2.a Schema change — two distinct env concepts

Today's `StepFile.Env` (`internal/steps/steps.go:37`) is a *name allowlist* —
forward these host env vars into the container. That stays unchanged.

Add a sibling concept for explicit key=value injection:

```json
{
  "env": ["GH_TOKEN"],
  "containerEnv": {
    "GOPATH":         "/home/agent/workspace/.ralph-cache/go",
    "GOCACHE":        "/home/agent/workspace/.ralph-cache/go-build",
    "GOMODCACHE":     "/home/agent/workspace/.ralph-cache/gomod",
    "XDG_CACHE_HOME": "/home/agent/workspace/.ralph-cache/xdg"
  }
}
```

`env` forwards *from the host*; `containerEnv` sets *literal values* that
don't depend on host state.

#### 3.2.b ralph-tui code changes (generic, language-free)

**`internal/steps/steps.go`** — add field:

```go
type StepFile struct {
    Env          []string          `json:"env,omitempty"`
    ContainerEnv map[string]string `json:"containerEnv,omitempty"`
    ...
}
```

**`internal/sandbox/command.go`** — extend `BuildRunArgs` signature:

```go
func BuildRunArgs(
    projectDir, profileDir string,
    uid, gid int,
    cidfile string,
    envAllowlist []string,
    containerEnv map[string]string, // NEW
    model, prompt string,
) []string
```

Append `-e KEY=VALUE` for each map entry in sorted order (deterministic argv
for tests). The function knows nothing about `GOPATH` or any language — it
just forwards whatever the workflow declared.

**`internal/validator/`** — validate `containerEnv`:
- Reject `CLAUDE_CONFIG_DIR` (already owned by sandbox).
- **Collision with `env` allowlist is allowed; `containerEnv` wins** (per V7).
  Emit an INFO-level validator notice, not an error — the author's explicit
  `containerEnv` literal is higher-intent than a transient host var. Docker's
  own argv parsing already resolves `-e KEY=VAL` wins over `-e KEY` at the
  last-wins level, so `BuildRunArgs` must emit `containerEnv` entries *after*
  the forwarded-from-host entries to get consistent behavior.
- Reject values containing newlines **or NUL bytes** (docker arg hygiene).
- Reject keys containing `=` (malformed).
- Warn if a key looks like a secret (`*_TOKEN`, `*_KEY`, `*_SECRET`) — literal
  values in `ralph-steps.json` will get committed; that's the user's call but
  worth a lint warning.

**`internal/workflow/workflow.go`** — thread `StepFile.ContainerEnv` from
config load down to `BuildRunArgs` call site.

**Tests — `internal/sandbox/command_test.go`:** assert the map is rendered
in sorted order with one `-e KEY=VALUE` per entry; empty/nil map is a no-op;
keys with `=` in the *value* pass through; keys with `=` in the *name* are
rejected in the validator.

None of the above mentions Go.

#### 3.2.c Workflow-level usage — the default Ralph workflow

The bundled `ralph-steps.json` drives Go-heavy projects today (pr9k itself,
gearjot-v2, gearjot-v2-events). Add the Go-cache entries shown above to
*that file* — it's a workflow config, not runner code. Users pointing
`--workflow-dir` at their own bundle can replace them with Python, Node, or
Rust equivalents.

Also seed `.ralph-cache/` into `.gitignore` in any repo using these paths
(workflow responsibility, not ralph-tui's).

**Preflight mkdir (per V8):** Docker bind-mounts the host source directory
as-is and will NOT auto-create subpaths. If `$PROJECT_DIR/.ralph-cache/`
does not exist on the host when the container starts, the first Go/Node/etc.
tool tries to write and the permission-denied storm returns. Add a
preflight check in `internal/preflight`:

```go
// In preflight.Run, before any iteration:
if err := os.MkdirAll(filepath.Join(projectDir, ".ralph-cache"), 0o755); err != nil {
    return fmt.Errorf("preflight: cannot create .ralph-cache in %s: %w", projectDir, err)
}
```

If the mkdir fails (read-only mount, wrong uid/gid), fail preflight — a
clear early error beats reproducing the original bug inside the container.
Document the host-writability requirement in
`docs/how-to/caching-build-artifacts.md`.

#### 3.2.d New how-to doc

Create `docs/how-to/caching-build-artifacts.md` with a matrix of language
→ env vars users commonly set:

| Language | Env vars |
| -------- | -------- |
| Go       | `GOPATH`, `GOCACHE`, `GOMODCACHE` |
| Node     | `NPM_CONFIG_CACHE`, `YARN_CACHE_FOLDER`, `PNPM_STORE_PATH` |
| Python   | `PIP_CACHE_DIR`, `POETRY_CACHE_DIR`, `UV_CACHE_DIR` |
| Rust     | `CARGO_HOME`, `CARGO_TARGET_DIR` |
| Generic  | `XDG_CACHE_HOME` |

Point all at paths under `/home/agent/workspace/.ralph-cache/...` so they
survive across iterations but stay inside the bind mount.

Add a ToC entry in `CLAUDE.md` under "How-To Guides".

**Expected impact (on Go workflows specifically):** eliminates 88
`"permission denied"` retries and ~218 inline `GO*=…` prefixes per
gearjot-v2 run. The *ralph-tui* change itself ships zero language-specific
bytes; the language benefit is a config edit away.

---

### 3.3 Collapse test-writing + fix-review-items double-verification

Current: `test-writing.md` runs tests, then `code-review-changes.md` runs,
then `code-review-fixes.md` runs tests again. On runs with few review items
this is pure waste.

**Prompt change — `prompts/test-writing.md`:** remove line 4 *"Run all tests,
type checks, linting and formatting tools. Fix any issues."* — leave
verification as the sole responsibility of `code-review-fixes.md`, which
runs right after. Claude writing tests without running them is fine; the
next step catches failures.

#### 3.3.a Skip the fix step entirely when the reviewer found nothing

Initial draft used a `skipIfCapture.equals` match on the Claude result text.
That is brittle — `claudestream.Aggregator.Result()` returns the final
assistant message (a paragraph), not a bare marker; an `equals` comparison
against a sentinel would almost never match, and a `contains` matcher would
false-fire on benign prose like *"there is nothing to fix beyond the
following …"*.

**Better: reuse the `BreakLoopIfEmpty` precedent.**
`run.go:371` already skips remaining steps when a non-claude capture is
empty. The code-review step already emits `code-review.md` as an artifact
file — a strict file-size check gives a signal that is objective and
impossible for Claude to accidentally spoof.

Add one non-claude step between `Code review` and `Fix review items`:

```json
{ "name": "Check review verdict", "isClaude": false,
  "command": ["scripts/review_verdict"],
  "captureAs": "REVIEW_HAS_FIXES",
  "captureMode": "lastLine" }
```

`scripts/review_verdict`: if `code-review.md` is missing, empty, or contains
only whitespace/the string `NOTHING-TO-FIX`, print nothing (empty capture);
otherwise print `"yes"`. This mirrors the existing `get_next_issue` pattern
of "empty stdout = false".

**Workflow change — `ralph-tui/ralph-steps.json`:** add `skipIfCaptureEmpty`
to the `Fix review items` step, naming the capture it reads:

```json
{ "name": "Fix review items", "isClaude": true, "model": "sonnet",
  "promptFile": "code-review-fixes.md",
  "skipIfCaptureEmpty": "REVIEW_HAS_FIXES" }
```

This reuses the *exact same empty-capture primitive* `BreakLoopIfEmpty`
already uses — lower cognitive surface area than introducing a second
comparison operator.

**Go change — `ralph-tui/internal/steps/steps.go`:** one string field, not a
struct:

```go
type Step struct {
    ...
    SkipIfCaptureEmpty string `json:"skipIfCaptureEmpty,omitempty"`
}
```

**Go change — `ralph-tui/internal/workflow/workflow.go`:** in the step loop,
after var resolution and before `Orchestrate`, check if
`s.SkipIfCaptureEmpty != ""`; read the named var from `vt` with the current
phase's resolution; if empty (or missing), mark the step skipped via
`header.SetStepState(j, ui.StepSkipped)`, emit `Step skipped (<var> empty)`
to the log, and `continue`.

**Go change — `ralph-tui/internal/validator/`:** verify
`skipIfCaptureEmpty` names a capture bound by a *strictly earlier* step in
the same phase.

**Failure-vs-empty distinction (per V11):** `LastCapture()` returns `""` on
both successful-empty and script-failure (`workflow.go:496-497`). A crashing
`review_verdict` script would look identical to "no fixes needed" and
silently skip `Fix review items`. Runtime check must verify the source
capture step ended with `StepDone`, not `StepFailed`; if the source step
failed, propagate to ModeError instead of skipping. The validator cannot
prove this at config-load time (it's runtime-only), so the workflow engine
tracks the source step's final state alongside its captured value.

**Matcher specification for `scripts/review_verdict` (per V3):** bash
implementation must (1) read `code-review.md`; (2) strip leading `#`
markdown headings and blank lines; (3) trim trailing whitespace; (4)
compare the remainder byte-for-byte to the literal 14-byte sequence
`NOTHING-TO-FIX`. Any other content — including trailing prose, multi-line
output, or an empty file — prints `yes` (= has fixes). Empty file counts
as "has fixes" to avoid false-positive skip when the prior step crashed
before writing the file. Prompt update to `code-review-changes.md`:
*"If no changes need to be made, write EXACTLY the 14-byte sequence
`NOTHING-TO-FIX` into code-review.md, with no heading and no content
before or after. Any other content means code changes are required."*
Test matrix: exact-match, trailing newline, leading `#`, quoted-in-prose,
empty file, whitespace-only, multi-line review.

**Dependency (per V4):** this section must ship *after* §3.6, because the
skipped step must record `status: "skipped"` in `.ralph-iteration.jsonl`
for downstream `Update docs` and finalize phases to reason about the gap.
Phase B ordering (§6) already enforces this; stated explicitly here.

**Prompt change — `prompts/code-review-changes.md`:** add an explicit line:
*"If no changes need to be made, write the literal string `NOTHING-TO-FIX`
into code-review.md and nothing else."*

**Expected impact:** saves ~4–6 min on iterations where code-review finds
nothing substantial (estimated ~30% of runs based on observed variance).

---

### 3.4 Resume the previous step's Claude session when it's the same model

Fresh sessions per step are the #1 cause of re-discovery overhead. Reusing
a session across same-model steps halves cold-boot cost.

Steps that can chain by model:
- `feature-work` (sonnet) → no direct successor in same model (test-planning
  is opus)
- `test-writing` (sonnet) → `fix-review-items` (sonnet) is two steps away —
  not contiguous
- `code-review` (opus) → next is `fix-review-items` (sonnet) — different model

So the *current* workflow doesn't benefit much from naïve chaining. But a
small reorder helps:

**Workflow reorder — `ralph-tui/ralph-steps.json`:**

```json
{ "name": "Feature work",    "model": "sonnet", ... },
{ "name": "Test writing",    "model": "sonnet", ..., "resumePrevious": true },
{ "name": "Test planning",   "model": "opus",   ... },
{ "name": "Code review",     "model": "opus",   ..., "resumePrevious": true },
{ "name": "Fix review items","model": "sonnet", ... },
{ "name": "Update docs",     "model": "sonnet", ..., "resumePrevious": true }
```

Keep test-planning before test-writing semantically? The original order
(plan → write) is correct for *new code*, but for the ralph loop where
feature-work already committed the implementation, letting the sonnet
session that wrote the feature also write the tests is faster and contextually
more accurate. Validate with a single iteration A/B run before committing.

**Go change — `ralph-tui/internal/steps/steps.go`:** add
`ResumePrevious bool` to `Step`.

**Go change — `ralph-tui/internal/sandbox/command.go`:** extend
`BuildRunArgs` to accept an optional `resumeSessionID string`; when non-empty,
append `--resume <id>` instead of starting a new session. Caller in
`workflow.go` reads the session id from the previous step's
`claudestream.StepStats` (parser already extracts it from the `ResultEvent`
— see `claudestream/aggregate.go:39`).

**Hard gates on resume (per V6, V10 evidence):**
- Only emit `--resume <sid>` when all four hold:
  1. `StepStats.SessionID` is a non-empty UUID (confirmed failure mode:
     `claude -p --resume ""` errors with *"--resume requires a valid session
     ID or session title when used with --print."*).
  2. The previous step completed with `StepDone` (not `StepFailed`, not
     timeout-terminated).
  3. The previous step's `Aggregator.Err()` was `nil` (no `is_error=true`,
     `ResultEvent` was observed).
  4. The cumulative resumed-session `InputTokens` reported by the last
     `ResultEvent` is below 200 KiB (if exceeded, force a fresh session —
     sessions grow unbounded).
- If any gate fails, fall through to a fresh session. Log a single line
  stating which gate blocked resume so operators can diagnose.

**Tests — `ralph-tui/internal/sandbox/command_test.go`:** add a resume-path
case.

**Expected impact:** 40–60% reduction in re-read overhead on resumed steps
(no re-reading of issue body, file tree, coding standards). For two resumed
steps per iteration at ~30 s saved each, ~1 min/iteration; compounds as
more steps chain.

**Risks / validation:** sessions grow unbounded — the parent prompt for step
N+1 must be tight. Add a hard cap: if resumed session has >200 k input tokens,
force a fresh session. Track via `claudestream.StepStats`.

**Model-override behaviour on resume — resolved by smoke test.** `--model`
**does** override on resume (see §7.2 Q1). A sonnet session resumed with
`--model opus` answers as opus and retains conversation memory. This means
§3.4's same-model chains remain the conservative first cut, but cross-model
resumption is available if later experimentation proves it useful. Not
blocking for Phase C.

**Contamination risk (V10) — hypothesis, not claimed win.** The reorder
(feature-work → test-writing as same-model chain) is presented in this
section as a plausible improvement. It has NOT been proven faster or more
accurate in practice. The session JSONL persists every turn including
`thinking`, `tool_use`, and `tool_result`, so test-writing's resumed
context includes every failed exploration feature-work had. Phase C must
A/B `resume=true` vs `resume=false` and measure:
- first-try test-pass rate (did the tests written work without needing
  `Fix review items` rescue?)
- median wall-clock on `Test writing`
- median total iteration time

Roll back §3.4 if either of the first two regresses, even if wall-clock
improves.

---

### 3.5 Drop 4 of 5 per-step GitHub issue comments

**Prompt change — delete these lines from all prompts:**
- `feature-work.md:6`
- `test-planning.md:5`
- `test-writing.md:9`
- `code-review-changes.md:5`
- `code-review-fixes.md:9`

Keep only a new step that posts one combined summary after `Update docs`:

**Workflow change — `ralph-tui/ralph-steps.json`:** add a non-Claude step
right before `Close issue`:

```json
{ "name": "Summarize to issue", "isClaude": false,
  "command": ["scripts/post_issue_summary", "{{ISSUE_ID}}"] }
```

**New script — `scripts/post_issue_summary`:** reads
`.ralph-iteration.jsonl` (see §3.6 — progress.txt is retired by that section),
filters records for the current issue, squashes into one comment, and posts
with `gh issue comment`. Ordering constraint: ships after §3.6. If Phase A
ships before §3.6, fall back to reading `progress.txt`; document the fallback
in the script itself so it's self-explanatory.

**Expected impact:** 4× `gh issue` API calls saved per iteration × 32/run
→ 4–8 fewer per iteration. Minor wall-clock (~10 s / iteration) but
eliminates GitHub rate-limit pressure.

---

### 3.6 Replace `progress.txt` append-soup with structured iteration log

**Problem:** Every step appends free-form text to `progress.txt`. The
finalize `lessons-learned.md` then re-reads it all and tries to re-parse.
Unstructured, lossy, and the file grows.

**Design:** `{{PROJECT_DIR}}/.ralph-cache/iteration.jsonl` (per V9 — NOT the
workspace root; same parent as the build caches in §3.2.c so a single
`.ralph-cache/` gitignore entry covers everything). One JSON record per
step with known fields (issue_id, step_name, model, duration_s,
files_touched, commit_sha, status, notes). Replaces `progress.txt` and
`deferred.txt`. `status` is one of `"done"`, `"skipped"`, `"failed"` —
downstream prompts MUST tolerate all three.

**Go change — `ralph-tui/internal/workflow/workflow.go`:** after each
successful step, append one line to `.ralph-iteration.jsonl` with
`claudestream.StepStats` data already being collected.

**Prompt change — lessons-learned.md, deferred-work.md:** replace
`@progress.txt` / `@deferred.txt` with `@.ralph-iteration.jsonl` and give
Claude field-level instructions instead of "analyze the full contents".

**Schema — `ralph-tui/internal/workflow/iterationlog.go` (new file):**

```go
type IterationRecord struct {
    SchemaVersion int     `json:"schema_version"` // start at 1; bump on incompatible changes
    IssueID       string  `json:"issue_id"`
    IterationNum  int     `json:"iteration_num"`
    StepName      string  `json:"step_name"`
    Model         string  `json:"model,omitempty"`
    Status        string  `json:"status"` // "done" | "skipped" | "failed"
    DurationS     float64 `json:"duration_s"`
    InputTokens   int     `json:"input_tokens,omitempty"`
    OutputTokens  int     `json:"output_tokens,omitempty"`
    SessionID     string  `json:"session_id,omitempty"`
    CommitSHA     string  `json:"commit_sha,omitempty"`
    Notes         string  `json:"notes,omitempty"`
}

func AppendIterationRecord(projectDir string, rec IterationRecord) error { ... }
```

`SchemaVersion` is part of the record for forward-compat: prompts like
`lessons-learned.md` that parse this file can fail fast when the schema
evolves. Bump on any incompatible change.

**Expected impact:** finalize phases become structured queries instead of
free-text parses. Lessons-learned step should drop from ~2–3 min to <1 min.
Also gives Ralph operators real telemetry they can eyeball (currently the
only iteration metrics are hidden in `.jsonl` files).

---

### 3.7 Tighten commit cadence

Current: up to 4 commits per iteration (feature-work, test-writing, fix-review,
update-docs). Squashes on push but inflates the iteration log.

**Prompt change — `prompts/test-writing.md`, `prompts/code-review-fixes.md`,
`prompts/update-docs.md`:** change "Commit changes in a single commit." to
"Use `git commit --amend --no-edit` if the previous commit is by the same
step within this iteration; otherwise commit normally."

Actually simpler and lower-risk: **leave commits alone**. The rebase on
`git push` compresses them anyway, and separate commits give a useful
timeline for post-mortem. Defer this optimization unless evidence shows the
commit phase itself is slow (it isn't — git is fast).

**Decision:** do NOT change; keep 3–4 commits as observable markers.

---

### 3.8 Preload deferred tools via prompt nudge

Initial draft listed `Bash, Read, Edit, Grep, Glob` — that's wrong. Those
tools are present in the container's Claude at session init (see the
`system/init` event's `tools` array in any `.jsonl` file). What is actually
deferred and causing `ToolSearch` calls mid-step are tools like `TodoWrite`,
`WebFetch`, `AskUserQuestion`. Confirmed from
`gearjot-v2/logs/ralph-2026-04-16-153328.268/*.jsonl` — every observed
`ToolSearch` query is one of `select:TodoWrite`, `select:WebFetch`,
`select:AskUserQuestion`, or combinations thereof.

**Prompt change — top of every prompt:** add the line

```md
You will likely need TodoWrite for tracking multi-step progress on this task.
(Preload once via ToolSearch query "select:TodoWrite".)
```

Only `TodoWrite` is worth preloading in the typical Ralph flow; `WebFetch`
and `AskUserQuestion` appear incidental. Even this change is low-ROI —
`ToolSearch` calls are cheap (one `tool_use` + `tool_result` round-trip per
selection). Gate rollout on a before/after `ToolSearch`-count diff; if the
change doesn't cut calls by >50%, drop the nudge and keep the prompts short.

---

### 3.9 Reduce test-writing variance by budget

Observed: test-writing ranged 0.4 → 21.9 min. The 21-minute case is Claude
generating many tests one at a time, re-running the suite between each.

**Prompt change — `prompts/test-writing.md`:** add

```md
Budget: write all tests first, then run the suite ONCE. If >5 tests fail,
fix them in batch rather than one at a time. Do not exceed 8 minutes of
wall-clock test execution in this step.
```

**Go change — `ralph-tui/internal/workflow/workflow.go`:** add an optional
`timeoutSeconds` field to `Step`. When set, spawn a goroutine that waits
`timeoutSeconds`; on fire, call the existing `currentTerminator(syscall.SIGTERM)`
closure (followed by `syscall.SIGKILL` after a 10 s grace window). Do **not**
switch to `exec.CommandContext` — the default Go context cancellation sends
SIGKILL directly to the `docker run` parent, which leaves the bind-mounted
container running (the cidfile-driven `docker kill` flow is the only clean
termination path; `workflow.go:350-353` already wires `sandbox.NewTerminator`
for exactly this reason). Cancel the timeout goroutine via a `done` channel
on successful step completion. Default behavior (no timeout) is unchanged.

**Interaction with §3.4 session resume (per V6/V10):** a timeout-killed
step has `hasResult=false` in the aggregator, so `StepStats.SessionID` is
empty and the §3.4 gate already blocks resume for the next step. But the
SIGKILL may leave the session file (`~/.claude/projects/<...>/sessions/<sid>.jsonl`)
mid-write with a truncated final line. Even if a later step *could*
resume by knowing the session ID (e.g., read from the JSONL artifact
file), it must not — record timed-out session IDs in an in-memory
blacklist for the remainder of the run and skip `--resume` for any step
attempting to chain from one.

**Config — `ralph-steps.json`:**

```json
{ "name": "Test writing", "model": "sonnet", "promptFile": "test-writing.md",
  "timeoutSeconds": 900 }
```

**Tests — `ralph-tui/internal/workflow/run_test.go`:** add a timeout-kills-step
case using the existing fake exec harness.

**Expected impact:** caps the long tail without hurting the median. Worst-case
iteration drops from 66 min to ~30 min.

---

## 4. Summary of ralph-tui code changes

All changes are **language-agnostic** and carry zero Go-specific (or any
other language-specific) knowledge. Language-specific wins come from the
*workflow config* in §5, not from the runner.

| File                                                   | Change |
| ------------------------------------------------------ | ------ |
| `ralph-tui/internal/sandbox/command.go`                | Add `containerEnv map[string]string` parameter to `BuildRunArgs`; render as sorted `-e KEY=VALUE` args. Accept optional `resumeSessionID` and append `--resume`. |
| `ralph-tui/internal/sandbox/command_test.go`           | Cover arbitrary containerEnv map + resume path. |
| `ralph-tui/internal/steps/steps.go`                    | Add `ContainerEnv map[string]string` to `StepFile`; add `ResumePrevious`, `SkipIfCapture`, `TimeoutSeconds` to `Step`. |
| `ralph-tui/internal/steps/steps_test.go`               | Parse-through tests for the new fields. |
| `ralph-tui/internal/validator/`                        | Validate `containerEnv` (no `CLAUDE_CONFIG_DIR`, no collision with `env` allowlist, no newlines in values, lint-warn on secret-looking names); validate `SkipIfCapture` references a real earlier capture; validate `TimeoutSeconds > 0`; cap captured-var size at 64 KiB. |
| `ralph-tui/internal/workflow/workflow.go`              | Thread `StepFile.ContainerEnv` into `BuildRunArgs`; honor `SkipIfCapture`; wire `--resume` from previous `StepStats.SessionID`; enforce `TimeoutSeconds` via `exec.CommandContext`; append to `.ralph-iteration.jsonl`. |
| `ralph-tui/internal/workflow/iterationlog.go` (new)    | `IterationRecord` + `AppendIterationRecord`. |
| `ralph-tui/internal/workflow/run.go`                   | Pipe iteration log writes through existing stats plumbing. |
| `ralph-tui/internal/claudestream/` (no change)         | Parser already surfaces `session_id`; re-use. |
| `docs/features/workflow-orchestration.md`              | Document `ContainerEnv`, `ResumePrevious`, `SkipIfCapture`, `TimeoutSeconds`. |
| `docs/features/docker-sandbox.md`                      | Add the `containerEnv` key=value injection next to the existing `env` allowlist. |
| `docs/how-to/building-custom-workflows.md`             | Update with the new fields and `{{ISSUE_BODY}}` / `{{BRANCH_DIFF_STAT}}` / `{{PROJECT_CARD}}` conventions. |
| `docs/how-to/caching-build-artifacts.md` (new)         | Language-to-env-var matrix (Go, Node, Python, Rust); example `containerEnv` blocks. Referenced from `CLAUDE.md`. |
| `CLAUDE.md`                                            | Add entry for `caching-build-artifacts.md` under How-To Guides. |

## 5. Summary of workflow / prompt / script changes

| File                                                   | Change |
| ------------------------------------------------------ | ------ |
| `ralph-steps.json`                                     | Add top-level `containerEnv` block with the Go cache vars (this workflow targets Go repos today); add `Get issue body`, `Get branch diff`, `Get project card` capture steps; reorder iteration to allow session-resume chaining; add `skipIfCapture` on `Fix review items`; add `Summarize to issue` before `Close issue`; add `timeoutSeconds: 900` on `Test writing`. |
| `prompts/*.md`                                         | Insert `# Context` preamble with precomputed vars; remove per-step "Update the github issue" lines; add ToolSearch preload hint; tighten `test-writing.md` budget. |
| `prompts/lessons-learned.md`, `prompts/deferred-work.md` | Consume `.ralph-iteration.jsonl` instead of `progress.txt`/`deferred.txt`. |
| `scripts/project_card` (new)                           | One-shot project-metadata emitter (reads Makefile/package.json/pyproject.toml/Cargo.toml as found — no hard-coded language assumption). |
| `scripts/post_issue_summary` (new)                     | Reads iteration log, posts one gh comment. |

### 5.1 Design principle reinforced

Whenever the question arises *"should ralph-tui know about language X?"*, the
answer is **no** unless the feature is truly universal. The right path is
almost always:

1. Give `ralph-steps.json` a generic knob (`containerEnv`, `captureAs`, etc.).
2. Put the language-specific value in the bundled workflow config.
3. Document the pattern in `docs/how-to/` so other workflows can copy it.

This keeps the generic orchestrator small and the per-project optimizations
where they belong — next to the prompts and scripts that already encode
that project's opinions.

## 6. Rollout & validation

Phase A originally claimed "zero schema changes". That was wrong (per V5):
§3.1 adds `captureMode` and §3.2 adds `containerEnv` — both are new JSON
keys on the `ralph-steps.json` schema. Both are backward-compatible
additive changes (optional fields, defaults preserve current behaviour),
but both require a minor version bump per
`docs/coding-standards/versioning.md`. Relabeled accordingly.

**Phase A — minimal additive schema + prompt edits:**
1. §3.2 `containerEnv` in `BuildRunArgs`. New optional top-level key on
   `StepFile`. Preflight `mkdir -p .ralph-cache` (per V8). Language-specific
   cache vars go in the default workflow's `ralph-steps.json`, not the runner.
2. §3.1 precomputed context vars. Adds `captureMode` optional field on
   `Step`. Adds `scripts/project_card` (bash, per Q3). Prompt updates per
   the pre-commit / post-commit split in §3.1.c (per V2).
3. §3.5 collapse GitHub issue comments. Prompt-only edits + new
   `scripts/post_issue_summary`. Falls back to `progress.txt` until §3.6
   ships (see §3.5).
4. §3.8 TodoWrite preload nudge. Prompt-only, gated on empirical reduction.

Expected: 30–40% median iteration time reduction. Schema is backward
compatible but not identical — minor version bump.

**Phase B — workflow engine additions:**
5. §3.6 `.ralph-cache/iteration.jsonl`. Structured iteration log with
   `SchemaVersion`. Ship *before* §3.3 so skipped-step records exist.
6. §3.3 `skipIfCaptureEmpty`. Requires §3.6 to exist (per V4). New
   `scripts/review_verdict` bash matcher per the V3 spec.
7. §3.9 `timeoutSeconds`. Routed through existing `currentTerminator`
   (per A6 evidence), not `exec.CommandContext`.

Expected: caps worst-case iteration (66 min → ~30 min), restructures
finalize phases.

**Phase C — session resume (speculative, validate carefully):**
8. §3.4 session resume. Gated on four runtime preconditions (per V6/V10):
   non-empty SessionID, `StepDone`, `is_error=false`, input-tokens < 200 KiB.
   Timed-out sessions blacklisted for the run. Model-override confirmed
   working on resume (Q1).

**Phase C validation gate — explicit A/B requirement:** before declaring
Phase C successful, run 5 iterations with `resume=true` and 5 with
`resume=false` on the same issue queue. Record for each:
- first-try test-pass rate (did the session-resumed test-writing produce
  tests that didn't need `Fix review items` rescue?)
- median wall-clock on `Test writing`
- median total iteration time
- median `input_tokens` per iteration

Roll back §3.4 if **any** of the first two metrics regresses, even if
wall-clock improves. Token cost is a hard gate too — a cumulative
context-bloat regression (V1) kills Phase C.

**Per-phase regression gates:**
- Wall-clock: each phase must cut median iteration time by ≥15% on
  `pr9k` itself; roll back otherwise.
- Tokens: each phase must not *raise* median `input_tokens` per iteration
  (V1 — cumulative context bloat can exceed tool-call savings).
- Tool-use counts: phase must cut `gh issue view` + `git log/diff` + `go test`
  invocations by ≥50% per iteration (grep `.jsonl`).
- Error rate: `is_error=true` count per iteration must not rise.

## 7. Assumptions log (from iterative review)

| # | Assumption | Class | Evaluation | Evidence / Action |
|---|---|---|---|---|
| A1 | `captureAs` on non-claude steps binds full stdout | Primary | **Refuted** | `workflow.go:495` uses `lastNonEmptyLine`; `orchestrate.go:10` documents `CaptureLastLine` as zero-value. → Fixed in §3.1.b by adding `captureMode: "fullStdout"`. |
| A2 | `claude -p --resume <id>` is supported | Primary | **Verified** | `claude --help` confirms `-r/--resume`; `--no-session-persistence` note implies `-p` persists by default. |
| A3 | Session state survives across Docker container instances via bind-mounted profile dir | Primary | **Verified** | Host `~/.claude/projects/-home-agent-workspace/sessions/<id>.jsonl` exists; `command.go:36-38` bind-mounts profile dir. |
| A4 | Relative `scripts/foo` resolves against workflow dir | Primary | **Verified** | `run.go:491-492` joins `workflowDir + exe` when relative w/ separator. |
| A5 | `{{VAR}}` substitution works in every `command[]` arg | Primary | **Verified** | `run.go:485`; covered by `workflow_test.go:545`. |
| A6 | `exec.CommandContext` context-deadline is a safe timeout path | Primary | **Refuted** | Go SIGKILLs the `docker run` parent; cidfile-driven `docker kill` is the only clean path. → Fixed in §3.9 to route through existing `currentTerminator`. |
| A7 | Claude's `Aggregator.Result()` returns a bare sentinel usable for `equals` match | Primary | **Refuted** | `aggregate.go:54` returns the full final assistant text (a paragraph). → Fixed in §3.3.a by switching to empty-capture primitive via a non-claude `review_verdict` script. |
| A8 | `Bash/Read/Edit/Grep/Glob` are deferred tools in the sandbox Claude | Primary | **Refuted** | `system/init` event in every `.jsonl` lists them all as initially available; actual deferred tools observed are `TodoWrite`, `WebFetch`, `AskUserQuestion`. → Fixed in §3.8 with corrected preload list. |
| A9 | `StepStats.SessionID` is populated by the time a step ends | Primary | **Verified** | `aggregate.go:39` sets it on `ResultEvent`, which is the terminal event. |
| B1 | `captureAs: ISSUE_BODY` via `gh issue view` works as-is | Secondary (A1) | Invalidated → fixed with §3.1.b. |
| B2 | `captureAs: BRANCH_DIFF_STAT` via `git diff --stat` works as-is | Secondary (A1) | Invalidated → fixed with §3.1.b. |
| B3 | `captureAs: PROJECT_CARD` via `scripts/project_card` works as-is | Secondary (A1) | Invalidated → fixed with §3.1.b. |
| B4 | `--model` override on resume switches model mid-session | Secondary (A2) | **Uncertain** — gated behind Phase C smoke test. |
| B5 | Timeout enforcement cleanly kills the container | Secondary (A6) | Invalidated → fixed with §3.9 revision. |

### 7.1 Overlap findings

- **Internal (fixed):** §3.5 `scripts/post_issue_summary` originally read
  `progress.txt`, but §3.6 retires that file — tightened cross-reference so
  the script reads `.ralph-iteration.jsonl` with a `progress.txt` fallback
  only until §3.6 ships.
- **External (inspired-by):** §3.3's skip mechanism is **inspired by** the
  `BreakLoopIfEmpty` convention (`run.go:371`) — both use empty stdout as
  a "false" signal. But they diverge meaningfully (per F2): `BreakLoopIfEmpty`
  reads the *current* step's fresh `LastCapture()` post-execution and ends
  the iteration loop; `skipIfCaptureEmpty` reads a *named, earlier-bound*
  VarTable value pre-execution and skips only THIS step. What's shared is
  the convention; what's different is the plumbing. That keeps the
  comparison-operator surface area at zero without overclaiming reuse.
- **External (deliberate):** §3.1's `CaptureMode` field intentionally
  shadows the concept of `ui.CaptureMode` (which today switches between
  `CaptureLastLine` and `CaptureResult` for claude steps). The JSON field
  name is `captureMode` (string); the Go-level mapping lives in
  `internal/steps`. Named deliberately to align with the existing concept,
  not to replace it.

### 7.2 Resolved ambiguities (evidence-based)

All three open questions plus the uncertainties surfaced by adversarial review
were resolved via a mix of live CLI smoke tests and log-corpus measurement.

#### Q1 / B4 — `claude --resume <id> --model <other>` behaviour

**Answer: `--model` overrides the session's original model. Conversation
history is preserved verbatim across the model switch.**

Method: seeded a session with `command claude -p --model sonnet "Remember this
token: GOLDFISH-7419. Reply with just OK."` → captured `session_id
83b20959-057b-4c2e-91ec-1f73a76064d6`. Then ran `command claude -p --resume
83b20959-... --model opus "What token did I ask you to remember, and which
model are you?"`. Stream-json output showed `"model":"claude-opus-4-7"` at
init, same session ID retained, and `"result":"Token: GOLDFISH-7419. I'm
Claude Opus 4.7."`.

Consequence: §3.4 *could* in principle chain across models, but that's
orthogonal to the plan's goal (same-model chains have the clearest
contextual-benefit story). Keep the same-model constraint for the initial
rollout; document in §3.4 that cross-model chaining is technically possible
but out of scope until measured.

#### Q2 — capture cap sizing

**Answer: 32 KiB per-capture hard limit, not the originally proposed 256 KiB.**

Method: measured every `gh issue view` tool_result in
`gearjot-v2/logs/ralph-2026-04-16-153328.268/iter*-03-feature-work.jsonl` via
Python json scan. Distribution: p50 ≈ 14 KB, p90 ≈ 22 KB, max 39 KB. The
39 KB outlier included the full comment thread in the default
`gh issue view` output.

When §3.1.c captures `gh issue view --json title,body -t "{{.title}}\n\n{{.body}}"`
(body only, no comments), observed sizes will be substantially smaller than
these totals — but 32 KiB gives ~1.6× margin over the largest real body we've
seen. A 256 KiB ceiling invites cumulative amplification (V1 below).

Truncation rule: when the captured value exceeds 32 KiB, keep the first
30 KiB verbatim and append `"\n\n[…truncated, full body at github.com/<repo>/issues/{{ISSUE_ID}}]"`.
Validator caps at 32 KiB; runtime enforces the truncation.

#### Q3 — `scripts/review_verdict` implementation language

**Answer: bash, matching the other six existing scripts.**

All six files in `scripts/` are `#!/bin/bash` (`box-text`, `close_gh_issue`,
`get_commit_sha`, `get_gh_user`, `get_next_issue`, `statusline`). Adding a Go
binary would require a second build artifact, a second release channel, and
a second language-review surface. Shell-script file-exists + sentinel-match
is a ~10-line solution; the Go version has no advantage.

### 7.3 Adversarial findings applied

The `adversarial-validator` agent surfaced 11 failure modes. Each either
drove a plan edit (listed below) or was recorded as a known risk.

**V1 — cumulative context amplification:** the 32 KiB cap (Q2 above) bounds
per-capture payload. Add to §6 validation gates: *"If Phase A raises
median `input_tokens` per iteration (from `.ralph-iteration.jsonl`), roll
back regardless of wall-clock savings."* Tokens are a cost, not just a
latency knob.

**V2 — `BRANCH_DIFF_STAT` goes stale after Feature work commits:** the
capture runs before feature-work, so steps 4–8 see a pre-commit (empty)
diff. Either (a) re-capture between steps (restores the tool-call cost) or
(b) inject only into prompts that run after commits. Chose (b): delete
`{{BRANCH_DIFF_STAT}}` from `feature-work.md` and `test-planning.md`
prompts; keep it only in `code-review-changes.md`, `code-review-fixes.md`,
`update-docs.md`. Rename capture to `PRE_FEATURE_DIFF` to make its
temporal scope obvious. Add a second capture `PRE_REVIEW_DIFF` right
before code-review to get the post-feature-work tree state.

**V3 — `NOTHING-TO-FIX` matcher brittleness:** tighten `scripts/review_verdict`
to: (1) strip leading `#` markdown headings, blank lines, and trailing
whitespace; (2) compare the remainder byte-for-byte to the literal
`NOTHING-TO-FIX`; (3) any other content counts as "has fixes". Prompt
update in `code-review-changes.md`: *"If no changes need to be made, write
EXACTLY the 14-byte sequence `NOTHING-TO-FIX` into code-review.md and
nothing else (no heading, no trailing newline beyond the single file
terminator)."* Add unit tests for the matcher: exact-match, trailing
newline, leading `#`, quoted-in-prose, empty file, whitespace-only.

**V4 — skipped step downstream state:** when `skipIfCaptureEmpty` fires,
emit a one-line entry to `.ralph-iteration.jsonl` (`status: "skipped"`) so
§3.6-consuming prompts can see the gap. This requires §3.3 to ship *after*
§3.6, as Phase B's ordering already implies. State the dependency
explicitly in §6.

**V5 — Phase A "zero schema changes" is false:** §3.1 adds `captureMode`,
§3.2 adds `containerEnv`. Both are backward-compatible (new optional
fields, defaults preserve existing behaviour), but they *are* schema
additions — the `docs/coding-standards/versioning.md` definition of
ralph-tui's public surface includes `ralph-steps.json`. Rewrite §6
Phase A label accordingly.

**V6 — session resume with empty SessionID:** confirmed failure mode.
`command claude -p --resume "" --model sonnet "hi"` errors out with
*"--resume requires a valid session ID or session title when used with
--print."* An invalid UUID produces a similar error. §3.4 must gate:
*"Only emit `--resume <sid>` when the previous step's `StepStats.SessionID`
is a non-empty UUID AND that step ended with `StepDone` (not `StepFailed`,
not timeout, not `is_error=true`)."*

**V7 — strict `containerEnv` vs `env` collision:** the initial rule
*"reject name collisions"* defeats the valid fallback pattern
(*"use host GOPATH if set, else a workspace default"*). Revise to:
`containerEnv` literal wins, but emit an INFO-level validator notice when
a key appears in both. Rationale: `containerEnv` is the author's explicit
intent; silently ignoring it would be a worse surprise than overriding a
transient host var.

**V8 — `.ralph-cache/` subpath creation on bind mount:** host-side
pre-creation is required because Docker mounts the source as-is. Add a
preflight check in `internal/preflight`: before any iteration runs,
`os.MkdirAll(projectDir + "/.ralph-cache", 0o755)`. If the cache dir
cannot be created (permission, read-only mount), fail preflight rather
than re-introducing the permission-denied storm inside the container.
Document in `docs/how-to/caching-build-artifacts.md`.

**V9 — `.ralph-iteration.jsonl` location:** move from the workspace root
to `{{PROJECT_DIR}}/.ralph-cache/iteration.jsonl`. Same parent dir as the
build caches (§3.2.c), so a single `.ralph-cache/` gitignore entry covers
everything. Update every prompt that currently says *"Never commit
progress.txt"* to *"Never commit anything under `.ralph-cache/`"*.

**V10 — resumed session inherits prior errors/explorations:** the
smoke-test showed the session JSONL accumulates every turn including
thinking, tool_use, and tool_result — so a failed feature-work's
exploration and error messages would prime the test-writing step toward
the same failures. §3.4 gains a new gate: *"Do NOT resume a session
whose terminal `ResultEvent` had `is_error=true` or `subtype !=
'success'`. Treat the next step as a fresh session."* Also reframe the
feature-work → test-writing reorder at the top of §3.4 as a **hypothesis
to validate**, not a claimed win; add an explicit A/B requirement to the
Phase C validation gate: first-try pass rate with resume vs fresh.

**V11 — failed `review_verdict` script silently skips fixes:** the
existing `LastCapture()` path zeros on non-zero exit (`workflow.go:496-497`),
so a crashing verdict script looks identical to "no fixes needed". Gate
`skipIfCaptureEmpty` on the source step having completed with `StepDone`
(not `StepFailed`). Record in the validator: the source capture step must
be permitted to error out to ModeError; it must NOT be swallowed as
"skip me". Implementation: extend the `skipIfCaptureEmpty` check to read
both `vt.GetInPhase(...)` (for the value) *and* the most recent step's
state (to reject `StepFailed`).

### 7.4 Additional findings from evidence-based investigator

**F1 — `DisallowUnknownFields` coupling (validator.go:125):** the validator
uses a strict JSON decoder, meaning any new `ralph-steps.json` key will be
rejected at parse time unless `validator/validator.go`'s `vStep`/`vFile`
structs are extended in the *same* commit as `internal/steps/steps.go`.
§4's file list already includes both, but add a note: *"These files must
land together; landing `steps.go` changes independently breaks every
workflow that uses the new keys."*

**F2 — `BreakLoopIfEmpty` reuse language overstated:** §7.1 originally
said §3.3 reuses "the exact same empty-capture primitive". On closer
inspection the two mechanisms share only the *convention* that empty
stdout means "false"; the data source, firing time, and scope of effect
all differ. Reframe §7.1 as "inspired by" rather than "exact same".

**F3 — scanner buffer is per-line, not total (`workflow.go:461-462`):** the
256 KiB buffer caps a single *line*, not total stdout. Fine for
`gh issue view --json title,body -t "{{.title}}\n\n{{.body}}"` (newlines
between paragraphs), but a single-line JSON blob from `--json` *without*
a `-t` template could exceed 256 KiB on a pathological issue. The
template-based capture in §3.1.c avoids this, but document it as a
precondition.

## 8. Explicitly out of scope

- Parallel step execution (feature-work and test-planning cannot run concurrently;
  test-planning needs the feature-work commit to exist).
- Dropping Docker sandboxing — ADR `20260413160000-require-docker-sandbox.md`
  makes it unconditional; these optimizations work *within* the sandbox.
- Rewriting `claudestream` — the parser already extracts what we need.
- Switching from `claude -p` to SDK/API calls — large surface-area change,
  separate design doc.
