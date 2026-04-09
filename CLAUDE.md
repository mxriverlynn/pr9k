# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**pr9k (Power-Ralph.9000)** is an automated development workflow orchestrator that drives the `claude` CLI through multi-step coding loops. It picks up GitHub issues labeled "ralph", implements features, writes tests, runs code reviews, and pushes — all unattended.

Based on [AI Hero's Getting Started with Ralph](https://www.aihero.dev/getting-started-with-ralph).

## Repository Structure

- `ralph-tui/` — Go TUI orchestrator. See "ralph-tui" section below.
- `scripts/` — Helper scripts (`get_next_issue`, `close_gh_issue`, `get_gh_user`, `get_commit_sha`, `box-text`)
- `prompts/` — Prompt files consumed by the orchestrator. Each prompt is passed to `claude -p`. Iteration prompts get `ISSUENUMBER=` and `STARTINGSHA=` prepended.
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

- Ralph is invoked **from the target repo** — all subprocesses inherit that cwd
- The project directory is resolved from the executable path via `os.Executable()` + `filepath.EvalSymlinks`
- The `get_next_issue` script sorts open issues and picks the lowest number
- Non-claude steps (`close_gh_issue`, `git push`) run as shell commands defined in JSON configs

## ralph-tui (Go/Glyph)

The Go TUI orchestrator lives in `ralph-tui/`, using [Glyph](https://useglyph.sh/) for real-time streaming output. Full plan in `docs/plans/ralph-tui.md`.

### Build and run

```bash
# Using make (recommended):
make build
./bin/ralph-tui <iterations>

# Or build directly:
cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui
./ralph-tui <iterations> [-project-dir <path>]
```

Use `go build` — `go run` won't work because `projectDir` is resolved via `os.Executable()`.

See [`docs/architecture.md`](docs/architecture.md) for detailed architectural documentation including block diagrams, data flow, and links to feature-level docs.

## Architecture & Feature Documentation

- [`docs/architecture.md`](docs/architecture.md) — System-level architecture overview with block diagrams, data flow, keyboard state machine, and package dependency graph
- [`docs/features/cli-configuration.md`](docs/features/cli-configuration.md) — CLI argument parsing, flag reordering, and project directory resolution from the executable path
- [`docs/features/step-definitions.md`](docs/features/step-definitions.md) — JSON step configuration loading and prompt building with variable prepending for iteration context
- [`docs/features/subprocess-execution.md`](docs/features/subprocess-execution.md) — Subprocess lifecycle management with real-time io.Pipe streaming, graceful SIGTERM/SIGKILL termination, and output capture
- [`docs/features/workflow-orchestration.md`](docs/features/workflow-orchestration.md) — The Run loop driving iterations and finalization, and the Orchestrate step sequencer with interactive error recovery
- [`docs/features/tui-display.md`](docs/features/tui-display.md) — Pointer-mutable status header with checkbox-based step progress and step separator formatting
- [`docs/features/keyboard-input.md`](docs/features/keyboard-input.md) — Three-mode keyboard state machine (Normal/Error/QuitConfirm) and channel-based action dispatch
- [`docs/features/signal-handling.md`](docs/features/signal-handling.md) — OS signal handling (SIGINT/SIGTERM) triggering clean shutdown via ForceQuit
- [`docs/features/file-logging.md`](docs/features/file-logging.md) — Concurrent-safe timestamped file logger with buffered I/O

## Coding Standards

- [`docs/coding-standards/api-design.md`](docs/coding-standards/api-design.md) — API design patterns including unused parameter documentation, bounds guards, precondition validation, and adapter types. Apply when designing or modifying public interfaces and exported functions.
- [`docs/coding-standards/concurrency.md`](docs/coding-standards/concurrency.md) — Concurrency patterns including snapshot-then-unlock, WaitGroup drain, mutex-protected writes, and non-blocking channel sends. Apply when working with goroutines, mutexes, channels, or any shared state.
- [`docs/coding-standards/error-handling.md`](docs/coding-standards/error-handling.md) — Error handling conventions including package-prefixed messages, file paths in I/O errors, and scanner error checking. Apply to all error creation, wrapping, and propagation.
- [`docs/coding-standards/go-patterns.md`](docs/coding-standards/go-patterns.md) — Go-specific patterns including flag reordering, symlink-safe path resolution, and 256KB scanner buffers. Apply when working with CLI args, file paths, or subprocess I/O.
- [`docs/coding-standards/testing.md`](docs/coding-standards/testing.md) — Testing standards including race detector requirement, closeable idempotency tests, input immutability tests, and test helper path resolution. Apply when writing or modifying any test code.

## How-To Guides

- [`docs/how-to/building-custom-workflows.md`](docs/how-to/building-custom-workflows.md) — How to create custom step sequences, add prompts, and mix Claude and shell steps
- [`docs/how-to/variable-output-and-injection.md`](docs/how-to/variable-output-and-injection.md) — How variables are injected into prompts and commands, and how steps pass data via files

## Project Discovery

- See [`docs/project-discovery.md`](docs/project-discovery.md) for full project discovery details including languages, frameworks, tooling, commands, test structure, documentation paths, and infrastructure.
- Default branch: main
- Docs: `docs/`
- Coding standards: `docs/coding-standards/` — Go error handling, testing, concurrency, API design, and Go-specific patterns

### ralph-tui

- Language: Go 1.23
- Test: `cd ralph-tui && go test -race ./...` or `make test`
- Build: `make build` or `cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui`
- Lint: `make lint` (requires golangci-lint)
- Vet: `make vet`
- CI: `make ci` (runs test, lint, format, vet, vulncheck, mod-tidy, build)
