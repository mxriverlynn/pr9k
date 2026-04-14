# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**pr9k (Power-Ralph.9000)** is an automated development workflow orchestrator that drives the `claude` CLI through multi-step coding loops. It picks up GitHub issues labeled "ralph", implements features, writes tests, runs code reviews, and pushes — all unattended.

Based on [AI Hero's Getting Started with Ralph](https://www.aihero.dev/getting-started-with-ralph).

## Repository Structure

- `ralph-tui/` — Go TUI orchestrator. See "ralph-tui" section below.
- `scripts/` — Helper scripts (`get_next_issue`, `close_gh_issue`, `get_gh_user`, `get_commit_sha`, `box-text`)
- `prompts/` — Prompt files consumed by the orchestrator. Each prompt is passed to `claude -p`. Prompts use `{{ISSUE_ID}}`, `{{STARTING_SHA}}`, and other `{{VAR}}` tokens that are substituted at runtime.
- `bin/` — Build output from `make build` (binary, prompts, scripts, configs)
- `docs/` — Architecture, feature docs, how-to guides, coding standards, and plans

## Workflow: The Ralph Loop

Each iteration:
1. Find next open issue assigned to user with label "ralph" (lowest number first)
2. Feature work (sonnet) → Test planning (opus) → Test writing (sonnet) → Code review (opus) → Review fixes (sonnet) → Close issue → Update docs (sonnet) → Git push

After all iterations, three finalization steps run:
1. Deferred work — creates issues from `deferred.txt`
2. Lessons learned — codifies from `progress.txt`
3. Final git push

Intermediate files (`progress.txt`, `deferred.txt`, `test-plan.md`, `code-review.md`) are created in the **target repo's working directory**, never committed, and consumed/deleted by later steps.

## Key Design Decisions

- Two distinct directories, captured separately at startup:
  - **WorkflowDir (install dir)** — where ralph-tui's bundled `ralph-steps.json`, `scripts/`, `prompts/`, and `ralph-art.txt` live. Resolved from the executable path via `os.Executable()` + `filepath.EvalSymlinks`, or overridden with `--workflow-dir`. Anchors `{{WORKFLOW_DIR}}` template substitution and relative script-path resolution.
  - **ProjectDir (target repo)** — the user's shell CWD captured via `os.Getwd()` at startup, or overridden with `--project-dir`. Governs subprocess `cmd.Dir` (so `gh`, `git`, `claude` operate against the target repo), the `logs/` output location, and `{{PROJECT_DIR}}` substitution.
- The `get_next_issue` script sorts open issues and picks the lowest number
- Claude steps run inside an ephemeral Docker container (image `docker/sandbox-templates:claude-code`) via `RunSandboxedStep`, with the target repo and Claude profile directory bind-mounted. Non-claude steps run directly on the host.
- Non-claude steps (`close_gh_issue`, `git push`) run as shell commands defined in JSON configs

## ralph-tui (Go/Bubble Tea)

The Go TUI orchestrator lives in `ralph-tui/`, using [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) + [bubbles/viewport](https://github.com/charmbracelet/bubbles) for real-time streaming output. Full plan in `docs/plans/ralph-tui.md`.

### Build and run

```bash
# Using make (recommended):
make build
./bin/ralph-tui [-n <iterations>] [--workflow-dir <path>] [--project-dir <path>]

# Or build directly:
cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui
./ralph-tui [-n <iterations>] [--workflow-dir <path>] [--project-dir <path>]
```

Use `go build` — `go run` won't work because `workflowDir` is resolved via `os.Executable()`.

See [`docs/architecture.md`](docs/architecture.md) for detailed architectural documentation including block diagrams, data flow, and links to feature-level docs.

## Architecture & Feature Documentation

- [`docs/architecture.md`](docs/architecture.md) — System-level architecture overview with block diagrams, data flow, keyboard state machine, and package dependency graph
- [`docs/features/cli-configuration.md`](docs/features/cli-configuration.md) — CLI argument parsing with cobra flags (`--iterations`/`-n`, `--workflow-dir`, `--project-dir`, `--version`/`-v`), workflow directory resolution from the executable path, and project directory resolution from `os.Getwd()`
- [`docs/features/step-definitions.md`](docs/features/step-definitions.md) — JSON step configuration loading and prompt building with `{{VAR}}` substitution for iteration context
- [`docs/features/subprocess-execution.md`](docs/features/subprocess-execution.md) — Subprocess lifecycle management with real-time io.Pipe streaming and sendLine callback (SetSender), graceful SIGTERM/SIGKILL termination, and output capture
- [`docs/features/workflow-orchestration.md`](docs/features/workflow-orchestration.md) — The Run loop driving iterations and finalization, and the Orchestrate step sequencer with interactive error recovery
- [`docs/features/tui-display.md`](docs/features/tui-display.md) — Pointer-mutable status header with checkbox-based step progress and step separator formatting
- [`docs/features/keyboard-input.md`](docs/features/keyboard-input.md) — Four-mode keyboard state machine (Normal/Error/QuitConfirm/Quitting) and channel-based action dispatch
- [`docs/features/signal-handling.md`](docs/features/signal-handling.md) — OS signal handling (SIGINT/SIGTERM) triggering clean shutdown via ForceQuit
- [`docs/features/file-logging.md`](docs/features/file-logging.md) — Concurrent-safe timestamped file logger with buffered I/O
- [`docs/features/variable-state.md`](docs/features/variable-state.md) — `VarTable` with persistent and iteration scopes, built-in variables, and phase-based resolution
- [`docs/features/config-validation.md`](docs/features/config-validation.md) — D13 config validator for ralph-steps.json: schema shape, file existence, variable scope resolution, env passthrough validation (Category 10), sandbox isolation rules A/B/C, and structured error collection
- [`docs/features/docker-sandbox.md`](docs/features/docker-sandbox.md) — Docker sandbox architecture: mount layout (`<projectDir>` → `/home/agent/workspace`, `<profileDir>` → `/home/agent/.claude`), `BuildRunArgs` command shape, env allowlist behavior, UID/GID mapping, cidfile-driven termination, and residual risks
- [`docs/features/sandbox.md`](docs/features/sandbox.md) — Docker sandbox package: `BuildRunArgs` argv construction, `BuiltinEnvAllowlist`, cidfile lifecycle (`Path`/`Cleanup`), and `NewTerminator` closure for container signal delivery
- [`docs/features/preflight.md`](docs/features/preflight.md) — Preflight package: `ResolveProfileDir`, `CheckProfileDir`, `CheckCredentials`, `Prober` interface, `RealProber`, `CheckDocker`, and `Run` (collect-all-errors startup validation)
- [`docs/features/sandbox-subcommand.md`](docs/features/sandbox-subcommand.md) — `sandbox create` and `sandbox login` subcommands: Docker check, image pull, smoke test with ANSI sanitization for create; interactive claude REPL with auto-pull and profile-dir auto-create for login; shared helpers and dependency injection design

## ADRs

- [`docs/adr/20260409135303-cobra-cli-framework.md`](docs/adr/20260409135303-cobra-cli-framework.md) — Decision to use spf13/cobra for CLI argument parsing (POSIX flags, subcommands). Apply when modifying CLI argument handling or adding new commands.
- [`docs/adr/20260410170952-narrow-reading-principle.md`](docs/adr/20260410170952-narrow-reading-principle.md) — Narrow-reading principle: ralph-tui is a generic step runner; workflow content lives in `ralph-steps.json`, not Go code. Apply when evaluating any PR that adds Ralph-specific knowledge to Go code.
- [`docs/adr/20260411070907-bubble-tea-tui-framework.md`](docs/adr/20260411070907-bubble-tea-tui-framework.md) — Decision to migrate the TUI from Glyph to Bubble Tea + Lip Gloss + bubbles/viewport for dynamic window title, mouse-wheel scrolling, and long-term ecosystem stability. Apply when modifying any TUI rendering, keyboard dispatch, or subprocess-streaming code.
- [`docs/adr/20260413160000-require-docker-sandbox.md`](docs/adr/20260413160000-require-docker-sandbox.md) — Decision to make Docker an unconditional runtime requirement for claude steps. Apply when evaluating changes to the sandbox or any proposal to make Docker optional.
- [`docs/adr/20260413162428-workflow-project-dir-split.md`](docs/adr/20260413162428-workflow-project-dir-split.md) — Decision to split the single `--project-dir` flag into `--workflow-dir` (workflow bundle) and `--project-dir` (target repo). Apply when modifying CLI flags, `{{VAR}}` tokens, or any code that distinguishes the workflow bundle from the target repository.

## Coding Standards

- [`docs/coding-standards/api-design.md`](docs/coding-standards/api-design.md) — API design patterns including unused parameter documentation, bounds guards, precondition validation, and adapter types. Apply when designing or modifying public interfaces and exported functions.
- [`docs/coding-standards/concurrency.md`](docs/coding-standards/concurrency.md) — Concurrency patterns including snapshot-then-unlock, WaitGroup drain, mutex-protected writes, and non-blocking channel sends. Apply when working with goroutines, mutexes, channels, or any shared state.
- [`docs/coding-standards/error-handling.md`](docs/coding-standards/error-handling.md) — Error handling conventions including package-prefixed messages, file paths in I/O errors, and scanner error checking. Apply to all error creation, wrapping, and propagation.
- [`docs/coding-standards/go-patterns.md`](docs/coding-standards/go-patterns.md) — Go-specific patterns including symlink-safe path resolution and 256KB scanner buffers. Apply when working with CLI args, file paths, or subprocess I/O.
- [`docs/coding-standards/lint-and-tooling.md`](docs/coding-standards/lint-and-tooling.md) — Lint suppressions are prohibited in any form (`//nolint`, `.golangci.yml` exclusions, disabled linters, etc.). Fix the root cause or escalate; never silence a finding. Apply to every commit and every PR review.
- [`docs/coding-standards/testing.md`](docs/coding-standards/testing.md) — Testing standards including race detector requirement, closeable idempotency tests, input immutability tests, and test helper path resolution. Apply when writing or modifying any test code.
- [`docs/coding-standards/versioning.md`](docs/coding-standards/versioning.md) — Semantic versioning standard: `version.Version` is the single source of truth, what counts as ralph-tui's public API (CLI flags, `ralph-steps.json` schema, `{{VAR}}` language, `--version` output), `0.y.z` rules, and how to bump. Apply when changing any user-visible surface or preparing a release.

## How-To Guides

Problem-focused guides for users running ralph-tui against their own projects. When adding a new how-to, keep each guide focused on solving one specific problem or using one specific feature.

- [`docs/how-to/getting-started.md`](docs/how-to/getting-started.md) — Install ralph-tui, point it at a target repo, and interpret the first run of the default workflow
- [`docs/how-to/setting-up-docker-sandbox.md`](docs/how-to/setting-up-docker-sandbox.md) — Install Docker, run `ralph-tui sandbox create`, authenticate the claude profile with `ralph-tui sandbox login`, and configure `CLAUDE_CONFIG_DIR` for multi-profile setups
- [`docs/how-to/reading-the-tui.md`](docs/how-to/reading-the-tui.md) — Tour of the four TUI regions (checkbox grid, iteration line, log panel, shortcut footer with version label) and the phase/step/capture chrome rhythm written into the log body
- [`docs/how-to/building-custom-workflows.md`](docs/how-to/building-custom-workflows.md) — How to create custom step sequences, add prompts, and mix Claude and shell steps
- [`docs/how-to/variable-output-and-injection.md`](docs/how-to/variable-output-and-injection.md) — How `{{VAR}}` tokens are resolved from the VarTable into prompts and commands, and how steps pass data via files
- [`docs/how-to/capturing-step-output.md`](docs/how-to/capturing-step-output.md) — How to use `captureAs` to bind a step's stdout to a variable for use in later steps, including initialize-vs-iteration scoping
- [`docs/how-to/breaking-out-of-the-loop.md`](docs/how-to/breaking-out-of-the-loop.md) — Using `breakLoopIfEmpty` to exit the iteration loop dynamically when a capture step returns nothing
- [`docs/how-to/recovering-from-step-failures.md`](docs/how-to/recovering-from-step-failures.md) — Error mode keyboard controls (`c` continue, `r` retry, `q` quit) and when to use each
- [`docs/how-to/quitting-gracefully.md`](docs/how-to/quitting-gracefully.md) — The `q`/`y` confirmation flow, Escape cancel, SIGINT/SIGTERM, `Quitting...` feedback, and exit codes
- [`docs/how-to/debugging-a-run.md`](docs/how-to/debugging-a-run.md) — Reading the persisted log file, navigating by chrome landmarks, tracing capture values, and reproducing failures with `-n 1`

## Project Discovery

- See [`docs/project-discovery.md`](docs/project-discovery.md) for full project discovery details including languages, frameworks, tooling, commands, test structure, documentation paths, and infrastructure.
- Default branch: main
- Docs: `docs/`
- Coding standards: `docs/coding-standards/` — Go error handling, testing, concurrency, API design, and Go-specific patterns

### ralph-tui

- Language: Go 1.26.2
- Test: `cd ralph-tui && go test -race ./...` or `make test`
- Build: `make build` or `cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui`
- Lint: `make lint` (requires golangci-lint)
- Vet: `make vet`
- CI: `make ci` (runs test, lint, format, vet, vulncheck, mod-tidy, build)
