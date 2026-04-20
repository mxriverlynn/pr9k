# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**pr9k (Power-Ralph.9000)** is an automated development workflow orchestrator that drives the `claude` CLI through multi-step coding loops. It picks up GitHub issues labeled "ralph", implements features, writes tests, runs code reviews, and pushes — all unattended.

Based on [AI Hero's Getting Started with Ralph](https://www.aihero.dev/getting-started-with-ralph).

## Repository Structure

- `src/` — Go TUI orchestrator. See "pr9k" section below.
- `workflow/` — Default workflow bundle: `config.json` (step definitions), `prompts/` (Claude prompt files), and `scripts/` (helper scripts: `get_next_issue`, `close_gh_issue`, `get_gh_user`, `get_commit_sha`, `box-text`, `post_issue_summary`, `statusline`, `project_card`, `review_verdict`)
- `bin/` — Build output from `make build` (binary, prompts, scripts, configs)
- `docs/` — Architecture, feature docs, how-to guides, coding standards, and plans

## Workflow: The Ralph Loop

Each iteration:
1. Find next open issue assigned to user with label "ralph" (lowest number first)
2. Feature work (sonnet) → Test planning (opus) → Test writing (sonnet) → Summarize to issue → Close issue → Git push

After all iterations, finalization steps run (once per run, against the full set of branch changes):
1. Code review (opus) → Check review verdict → Fix review items (sonnet, skipped when the reviewer finds nothing) → Update docs (sonnet)
2. Deferred work — creates issues from `deferred.txt`
3. Lessons learned — codifies from `progress.txt`
4. Final git push

Intermediate files (`progress.txt`, `deferred.txt`, `test-plan.md`, `code-review.md`) are created in the **target repo's working directory**, never committed, and consumed/deleted by later steps.

## Key Design Decisions

- Two distinct directories, captured separately at startup:
  - **WorkflowDir (install dir)** — where pr9k's bundled `config.json`, `scripts/`, `prompts/`, and `ralph-art.txt` live. Resolved via two-candidate search: `<projectDir>/.pr9k/workflow/` first (in-repo override), then `<executableDir>/.pr9k/workflow/` (shipped bundle from `make build`). Overridden with `--workflow-dir`. Anchors `{{WORKFLOW_DIR}}` template substitution and relative script-path resolution.
  - **ProjectDir (target repo)** — the user's shell CWD captured via `os.Getwd()` at startup, or overridden with `--project-dir`. Governs subprocess `cmd.Dir` (so `gh`, `git`, `claude` operate against the target repo), the `.pr9k/logs/` output location, and `{{PROJECT_DIR}}` substitution.
- The `get_next_issue` script sorts open issues and picks the lowest number
- Claude steps run inside an ephemeral Docker container (image `docker/sandbox-templates:claude-code`) via `RunSandboxedStep`, with the target repo and Claude profile directory bind-mounted. Non-claude steps run directly on the host.
- Non-claude steps (`close_gh_issue`, `git push`) run as shell commands defined in JSON configs

## pr9k (Go/Bubble Tea)

The Go TUI orchestrator lives in `src/`, using [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) + [bubbles/viewport](https://github.com/charmbracelet/bubbles) for real-time streaming output. Full plan in `docs/plans/ralph-tui.md` (historical — describes the original TUI design).

### Build and run

```bash
# Using make (recommended):
make build
./bin/pr9k [-n <iterations>] [--workflow-dir <path>] [--project-dir <path>]

# Or build directly:
cd src && go build -o ../bin/pr9k ./cmd/pr9k
../bin/pr9k [-n <iterations>] [--workflow-dir <path>] [--project-dir <path>]
```

Use `go build` — `go run` won't work because `workflowDir` is resolved via `os.Executable()`.

See [`docs/architecture.md`](docs/architecture.md) for detailed architectural documentation including block diagrams, data flow, and links to feature-level docs.

## Architecture

- [`docs/architecture.md`](docs/architecture.md) — System-level architecture overview with block diagrams, data flow, keyboard state machine, and package dependency graph

## Feature Documentation

User-facing behavior, configuration, and cross-package integration. Each file lives in [`docs/features/`](docs/features/).

- [`docs/features/cli-configuration.md`](docs/features/cli-configuration.md) — CLI argument parsing with cobra flags (`--iterations`/`-n`, `--workflow-dir`, `--project-dir`, `--version`/`-v`), workflow directory resolution from the executable path, and project directory resolution from `os.Getwd()`
- [`docs/features/subprocess-execution.md`](docs/features/subprocess-execution.md) — Subprocess lifecycle management with real-time io.Pipe streaming and sendLine callback (SetSender), graceful SIGTERM/SIGKILL termination, and output capture
- [`docs/features/workflow-orchestration.md`](docs/features/workflow-orchestration.md) — The Run loop driving iterations and finalization, and the Orchestrate step sequencer with interactive error recovery
- [`docs/features/tui-display.md`](docs/features/tui-display.md) — Pointer-mutable status header with checkbox-based step progress, word-wrap log panel with scroll-position preservation, and text selection (`pos`, `selection`, `ModeSelect`, clipboard copy with OSC 52 fallback, mouse drag selection)
- [`docs/features/keyboard-input.md`](docs/features/keyboard-input.md) — Eight-mode keyboard state machine (Normal/Error/QuitConfirm/NextConfirm/Done/Select/Quitting/Help) and channel-based action dispatch
- [`docs/features/signal-handling.md`](docs/features/signal-handling.md) — OS signal handling (SIGINT/SIGTERM) triggering clean shutdown via ForceQuit
- [`docs/features/status-line.md`](docs/features/status-line.md) — Status-line feature: purpose, config schema, path resolution, script contract (stdin JSON payload, stdout rules, timeout), refresh triggers, cold-start fallback, help modal, concurrency model, lifecycle, observability, and out-of-scope list
- [`docs/features/docker-sandbox.md`](docs/features/docker-sandbox.md) — Docker sandbox architecture: mount layout (`<projectDir>` → `/home/agent/workspace`, `<profileDir>` → `/home/agent/.claude`), `BuildRunArgs` command shape, env allowlist behavior, UID/GID mapping, cidfile-driven termination, and residual risks
- [`docs/features/sandbox-subcommand.md`](docs/features/sandbox-subcommand.md) — `sandbox create` and `sandbox login` subcommands: Docker check, image pull, smoke test with ANSI sanitization for create; interactive claude REPL with auto-pull and profile-dir auto-create for login; shared helpers and dependency injection design

## Code Package Documentation

Per-Go-package API references (types, methods, synchronization, lifecycle) for contributors working on the internal packages. Each file lives in [`docs/code-packages/`](docs/code-packages/) and is named after the Go package it documents.

- [`docs/code-packages/steps.md`](docs/code-packages/steps.md) — `internal/steps`: JSON step configuration loading and prompt building with `{{VAR}}` substitution for iteration context
- [`docs/code-packages/logger.md`](docs/code-packages/logger.md) — `internal/logger`: Concurrent-safe timestamped file logger with millisecond-precision filenames and `RunStamp()` accessor for artifact directory naming
- [`docs/code-packages/vars.md`](docs/code-packages/vars.md) — `internal/vars`: `VarTable` with persistent and iteration scopes, built-in variables, and phase-based resolution
- [`docs/code-packages/validator.md`](docs/code-packages/validator.md) — `internal/validator`: D13 config validator for config.json: schema shape, file existence, variable scope resolution, env passthrough validation (Category 10), containerEnv validation, captureMode validation, sandbox isolation rules B and C, statusLine block validation, severity-based error collection (`IsFatal`, `FatalErrorCount`), and non-fatal warning/info handling
- [`docs/code-packages/statusline.md`](docs/code-packages/statusline.md) — `internal/statusline`: `Runner` goroutine lifecycle, `State` snapshot, `BuildPayload` stdin JSON, `Sanitize` ANSI output filtering, single-flight exec, 8 KB stdout limit, full-env trust model, and cold-start / shutdown ordering
- [`docs/code-packages/sandbox.md`](docs/code-packages/sandbox.md) — `internal/sandbox`: `BuildRunArgs` argv construction, `BuiltinEnvAllowlist`, cidfile lifecycle (`Path`/`Cleanup`), and `NewTerminator` closure for container signal delivery
- [`docs/code-packages/preflight.md`](docs/code-packages/preflight.md) — `internal/preflight`: `ResolveProfileDir`, `CheckProfileDir`, `CheckCredentials`, `Prober` interface, `RealProber`, `CheckDocker`, and `Run` (collect-all-errors startup validation)
- [`docs/code-packages/claudestream.md`](docs/code-packages/claudestream.md) — `internal/claudestream`: Parser (NDJSON line → typed Event), Renderer (events → TUI display lines, tool summary, Finalize), Aggregator (StepStats, captureAs result, is_error detection), RawWriter (per-step .jsonl persistence with O_TRUNC retry semantics), Slug (kebab filename generation), and Pipeline (single Observe entry point, atomic LastEventAt, crash-resilience sentinel)
- [`docs/code-packages/workflow.md`](docs/code-packages/workflow.md) — `internal/workflow`: `Runner` subprocess executor, `RunStep`/`RunStepFull` with `captureMode` (lastLine vs fullStdout with 32 KiB cap), `StepExecutor` interface, `stepDispatcher`, the `Run` loop, `IterationRecord`/`AppendIterationRecord` for the structured iteration log, and session resume gates (`evaluateResumeGates`, G1–G5) with per-phase trackers and session blacklist

## ADRs

- [`docs/adr/20260409135303-cobra-cli-framework.md`](docs/adr/20260409135303-cobra-cli-framework.md) — Decision to use spf13/cobra for CLI argument parsing (POSIX flags, subcommands). Apply when modifying CLI argument handling or adding new commands.
- [`docs/adr/20260410170952-narrow-reading-principle.md`](docs/adr/20260410170952-narrow-reading-principle.md) — Narrow-reading principle: pr9k is a generic step runner; workflow content lives in `config.json`, not Go code. Apply when evaluating any PR that adds Ralph-specific knowledge to Go code.
- [`docs/adr/20260411070907-bubble-tea-tui-framework.md`](docs/adr/20260411070907-bubble-tea-tui-framework.md) — Decision to migrate the TUI from Glyph to Bubble Tea + Lip Gloss + bubbles/viewport for dynamic window title, mouse-wheel scrolling, and long-term ecosystem stability. Apply when modifying any TUI rendering, keyboard dispatch, or subprocess-streaming code.
- [`docs/adr/20260413160000-require-docker-sandbox.md`](docs/adr/20260413160000-require-docker-sandbox.md) — Decision to make Docker an unconditional runtime requirement for claude steps. Apply when evaluating changes to the sandbox or any proposal to make Docker optional.
- [`docs/adr/20260413162428-workflow-project-dir-split.md`](docs/adr/20260413162428-workflow-project-dir-split.md) — Decision to split the single `--project-dir` flag into `--workflow-dir` (workflow bundle) and `--project-dir` (target repo). Apply when modifying CLI flags, `{{VAR}}` tokens, or any code that distinguishes the workflow bundle from the target repository.
- [`docs/adr/20260416-clipboard-and-selection.md`](docs/adr/20260416-clipboard-and-selection.md) — Decision to use `atotto/clipboard` for cross-platform clipboard writes, require `xclip`/`xsel` on Linux, implement a custom in-TUI selection layer (required by alt-screen + mouse-cell-motion), and ship OSC 52 stderr fallback alongside the primary path. Apply when modifying clipboard code, the selection layer, or evaluating proposals to change mouse mode.
- [`docs/adr/20260418175134-pr9k-rename-and-pr9k-layout.md`](docs/adr/20260418175134-pr9k-rename-and-pr9k-layout.md) — Records the `ralph-tui` → `pr9k` rename, `.pr9k/` umbrella for runtime state, two-candidate `resolveWorkflowDir`, and `ralph-steps.json` → `config.json` rename. Apply when modifying binary name, source-tree layout, runtime output paths, workflow discovery, or the workflow-config filename.

## Coding Standards

- [`docs/coding-standards/api-design.md`](docs/coding-standards/api-design.md) — API design patterns including unused parameter documentation, bounds guards, precondition validation, and adapter types. Apply when designing or modifying public interfaces and exported functions.
- [`docs/coding-standards/concurrency.md`](docs/coding-standards/concurrency.md) — Concurrency patterns including snapshot-then-unlock, WaitGroup drain, mutex-protected writes, and non-blocking channel sends. Apply when working with goroutines, mutexes, channels, or any shared state.
- [`docs/coding-standards/error-handling.md`](docs/coding-standards/error-handling.md) — Error handling conventions including package-prefixed messages, file paths in I/O errors, and scanner error checking. Apply to all error creation, wrapping, and propagation.
- [`docs/coding-standards/go-patterns.md`](docs/coding-standards/go-patterns.md) — Go-specific patterns including symlink-safe path resolution and 256KB scanner buffers. Apply when working with CLI args, file paths, or subprocess I/O.
- [`docs/coding-standards/lint-and-tooling.md`](docs/coding-standards/lint-and-tooling.md) — Lint suppressions are prohibited in any form (`//nolint`, `.golangci.yml` exclusions, disabled linters, etc.). Fix the root cause or escalate; never silence a finding. Apply to every commit and every PR review.
- [`docs/coding-standards/testing.md`](docs/coding-standards/testing.md) — Testing standards including race detector requirement, closeable idempotency tests, input immutability tests, and test helper path resolution. Apply when writing or modifying any test code.
- [`docs/coding-standards/versioning.md`](docs/coding-standards/versioning.md) — Semantic versioning standard: `version.Version` is the single source of truth, what counts as pr9k's public API (CLI flags, `config.json` schema, `{{VAR}}` language, `--version` output), `0.y.z` rules, and how to bump. Apply when changing any user-visible surface or preparing a release.
- [`docs/coding-standards/documentation.md`](docs/coding-standards/documentation.md) — Documentation standards: feature docs must ship with the feature (not as follow-ups), updating CLAUDE.md when adding new doc files, keeping doc code blocks consistent with production code, and documenting external tool dependencies. Apply to every PR that changes a user-visible surface.

## How-To Guides

Problem-focused guides for users running pr9k against their own projects. When adding a new how-to, keep each guide focused on solving one specific problem or using one specific feature.

- [`docs/how-to/getting-started.md`](docs/how-to/getting-started.md) — Install pr9k, point it at a target repo, and interpret the first run of the default workflow
- [`docs/how-to/setting-up-docker-sandbox.md`](docs/how-to/setting-up-docker-sandbox.md) — Install Docker, run `pr9k sandbox create`, authenticate the claude profile with `pr9k sandbox login`, and configure `CLAUDE_CONFIG_DIR` for multi-profile setups
- [`docs/how-to/reading-the-tui.md`](docs/how-to/reading-the-tui.md) — Tour of the four TUI regions (checkbox grid, iteration line, log panel, shortcut footer with version label) and the phase/step/capture chrome rhythm written into the log body
- [`docs/how-to/building-custom-workflows.md`](docs/how-to/building-custom-workflows.md) — How to create custom step sequences, add prompts, and mix Claude and shell steps
- [`docs/how-to/variable-output-and-injection.md`](docs/how-to/variable-output-and-injection.md) — How `{{VAR}}` tokens are resolved from the VarTable into prompts and commands, and how steps pass data via files
- [`docs/how-to/capturing-step-output.md`](docs/how-to/capturing-step-output.md) — How to use `captureAs` (and `captureMode: "fullStdout"` for multi-line output) to bind a step's stdout to a variable for use in later steps, including initialize-vs-iteration scoping
- [`docs/how-to/passing-environment-variables.md`](docs/how-to/passing-environment-variables.md) — How to forward host environment variables into the Docker sandbox via the `env` field, and how to inject literal values via `containerEnv`, in `config.json`
- [`docs/how-to/breaking-out-of-the-loop.md`](docs/how-to/breaking-out-of-the-loop.md) — Using `breakLoopIfEmpty` to exit the iteration loop dynamically when a capture step returns nothing
- [`docs/how-to/skipping-steps-conditionally.md`](docs/how-to/skipping-steps-conditionally.md) — Using `skipIfCaptureEmpty` to bypass a step when an earlier iteration-phase step produced empty output, with fail-safe defaults and validator constraints
- [`docs/how-to/setting-step-timeouts.md`](docs/how-to/setting-step-timeouts.md) — Using `timeoutSeconds` to cap the wall-clock time for a step, SIGTERM/SIGKILL escalation, and the advisory prompt budget pattern
- [`docs/how-to/recovering-from-step-failures.md`](docs/how-to/recovering-from-step-failures.md) — Error mode keyboard controls (`c` continue, `r` retry, `q` quit) and when to use each
- [`docs/how-to/quitting-gracefully.md`](docs/how-to/quitting-gracefully.md) — The `q`/`y` confirmation flow, Escape cancel, SIGINT/SIGTERM, `Quitting...` feedback, and exit codes
- [`docs/how-to/debugging-a-run.md`](docs/how-to/debugging-a-run.md) — Reading the persisted log file and iteration log (`.pr9k/iteration.jsonl`), navigating by chrome landmarks, tracing capture values, and reproducing failures with `-n 1`
- [`docs/how-to/copying-log-text.md`](docs/how-to/copying-log-text.md) — Mouse drag, keyboard single-line, and keyboard multi-line copy walkthroughs; OSC 52 SSH fallback; Linux `xclip`/`xsel` requirement; terminal native-selection override keys
- [`docs/how-to/configuring-a-status-line.md`](docs/how-to/configuring-a-status-line.md) — Add a `statusLine` block to `config.json`, use the sample script, read stdin JSON with `jq`, tune `refreshIntervalSeconds`, debug via log file, and recover the shortcut bar
- [`docs/how-to/caching-build-artifacts.md`](docs/how-to/caching-build-artifacts.md) — Use `containerEnv` to redirect Go/Node/Python/Rust cache dirs into `.ralph-cache/` for persistent, writable caches across iterations; language-to-env-var matrix and per-language example blocks
- [`docs/how-to/resuming-sessions.md`](docs/how-to/resuming-sessions.md) — Using `resumePrevious` to continue a prior Claude step's conversation via `--resume`, the five runtime gates (G1–G5), skip-chain interaction, and how to verify resume via `iteration.jsonl`

## Project Discovery

- See [`docs/project-discovery.md`](docs/project-discovery.md) for full project discovery details including languages, frameworks, tooling, commands, test structure, documentation paths, and infrastructure.
- Default branch: main
- Docs: `docs/`
- Coding standards: `docs/coding-standards/` — Go error handling, testing, concurrency, API design, and Go-specific patterns

### pr9k

- Language: Go 1.26.2
- Test: `cd src && go test -race ./...` or `make test`
- Build: `make build` or `cd src && go build -o ../bin/pr9k ./cmd/pr9k`
- Lint: `make lint` (requires golangci-lint)
- Vet: `make vet`
- CI: `make ci` (runs test, lint, format, vet, vulncheck, mod-tidy, build)
