# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**pr9k (Power-Ralph.9000)** is an automated development workflow orchestrator that drives the `claude` CLI through multi-step coding loops. It picks up GitHub issues labeled "ralph", implements features, writes tests, runs code reviews, and pushes — all unattended.

Based on [AI Hero's Getting Started with Ralph](https://www.aihero.dev/getting-started-with-ralph).

## Repository Structure

- `ralph-bash/` — Original bash implementation
  - `ralph-loop` — Main orchestrator script. Run from the **target repo**, not from this repo: `path/to/ralph-loop <iterations>`
  - `ralph-hitl` — Human-in-the-loop single-prompt runner: `path/to/ralph-hitl [prompt-name]`
  - `scripts/` — Helper scripts (`get_next_issue`, `close_gh_issue`, `get_gh_user`, `get_commit_sha`, `box-text`)
- `ralph-tui/` — Go TUI replacement (in progress). See "In Progress: ralph-tui" section below.
- `prompts/` — Prompt files consumed by both bash and TUI orchestrators. Each prompt is passed to `claude -p`. Iteration prompts get `ISSUENUMBER=` and `STARTINGSHA=` prepended.
- `docs/plans/` — Implementation plans (e.g., `ralph-tui.md` for the Go/Glyph TUI replacement)

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
- `ralph-loop` resolves its own directory via `$(dirname "$0")` to find prompts and scripts regardless of where it's called from
- The `get_next_issue` script sorts open issues and picks the lowest number
- Non-claude steps (`close_gh_issue`, `git push`) capture stderr with `2>&1`

## In Progress: ralph-tui (Go/Glyph)

A Go TUI replacement for `ralph-loop` is being built in `ralph-tui/`, using [Glyph](https://useglyph.sh/) for real-time streaming output. Full plan in `docs/plans/ralph-tui.md`.

### Current directory structure

```
ralph-tui/
  cmd/ralph-tui/
    main.go                   # entry point: parses CLI args, prints parsed values
  internal/
    cli/
      args.go                 # ParseArgs: iterations + optional -project-dir flag; reorderArgs allows flags in any position
      args_test.go
    workflow/                 # (not yet implemented)
    ui/
      ui.go                   # KeyHandler: mode-based keyboard dispatch (Normal/Error/QuitConfirm)
      ui_test.go
    steps/
      steps.go                # LoadSteps / LoadFinalizeSteps: parse Step structs from JSON configs; BuildPrompt with empty PromptFile validation
      steps_test.go
    logger/
      logger.go               # Logger: concurrent-safe file logger with timestamped, prefixed lines; SetContext/Log/Close
      logger_test.go
  configs/
    ralph-steps.json          # 8 iteration step definitions
    ralph-finalize-steps.json # 3 finalization step definitions
    configs_test.go           # validates JSON structure of both config files
  go.mod                      # module: github.com/mxriverlynn/pr9k/ralph-tui
```

### Build and run

```bash
cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui
./ralph-tui <iterations> [-project-dir <path>]
```

Use `go build` — `go run` won't work because `projectDir` is resolved via `os.Executable()`.

### Key architectural notes
- Uses `io.Pipe` for subprocess streaming (Glyph's `Log` component takes an `io.Reader`)
- Step definitions loaded from `configs/ralph-steps.json` at runtime, resolved relative to `projectDir`
- Logs to `logs/ralph-YYYY-MM-DD-HHMMSS.log`
- `projectDir` resolved via `os.Executable()` with `filepath.EvalSymlinks` (symlink-safe)
- `-project-dir` flag can appear before or after `<iterations>` — `reorderArgs` in `args.go` reorders args before parsing to work around Go's `flag` package stopping at the first positional
- `BuildPrompt` validates that `PromptFile` is non-empty before attempting file I/O

## Project Discovery

- See [`docs/project-discovery.md`](docs/project-discovery.md) for full project discovery details including languages, frameworks, tooling, commands, test structure, documentation paths, and infrastructure for all projects in this repository.
- Default branch: main
- Docs: `docs/`

### ralph-bash

- Language: Bash
- No build, test, or lint commands

### ralph-tui

- Language: Go 1.23
- Test: `cd ralph-tui && go test ./...`
- Build: `cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui`
