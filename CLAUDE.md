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
    workflow/
      workflow.go             # Runner: streams subprocess stdout/stderr through io.Pipe with mutex-protected writes and WaitGroup drain; Terminate: sends SIGTERM to current subprocess, SIGKILL after 3s timeout; WasTerminated: reports whether the last RunStep was user-terminated (reset at start of each RunStep); ResolveCommand: replaces {{ISSUE_ID}} template vars and resolves relative script paths against projectDir; WriteToLog: injects a line directly into the log pipe without running a subprocess; CaptureOutput: runs a command and returns trimmed stdout (used for get_next_issue, get_gh_user, git rev-parse HEAD)
      run.go                  # Run: main orchestration goroutine — displays banner, fetches GitHub username, runs N iterations, runs finalization, writes completion summary; StepExecutor/RunHeader interfaces; RunConfig struct; buildIterationSteps/buildFinalizeSteps helpers; iterHeader/finalHeader adapters
      workflow_test.go
      run_test.go
    ui/
      ui.go                   # KeyHandler: mode-based keyboard dispatch (Normal/Error/QuitConfirm); shortcutLine is mutex-protected; ShortcutLine() method is safe to call from any goroutine
      ui_test.go
      header.go               # StatusHeader: pointer-mutable TUI status display; StepState (Pending/Active/Done/Failed); SetIteration, SetStepState, SetFinalization, SetFinalizeStepState
      header_test.go
      log.go                  # StepSeparator / RetryStepSeparator: returns visual separator strings for step transitions in the output log
      log_test.go
      orchestrate.go          # Orchestrate: runs ResolvedSteps in sequence via StepRunner/StepHeader interfaces; on failure enters error mode and blocks on KeyHandler.Actions for continue/retry/quit
      orchestrate_test.go
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
- `Runner.Terminate()` sends SIGTERM to the active subprocess; if not exited within 3 seconds, sends SIGKILL — safe to call when idle (no-op)
- `Runner.WasTerminated()` reports whether the most recent `RunStep` was ended by a `Terminate()` call; the flag is reset at the start of each `RunStep` — used by `Orchestrate` to distinguish user-initiated termination from genuine failures
- `Runner.WriteToLog(line)` injects a line directly into the log pipe under the mutex; used to write step separator lines between subprocess outputs without spawning a command
- `Runner.CaptureOutput(command)` runs a command and returns trimmed stdout; stderr is discarded; used for single-value queries (get_next_issue, get_gh_user, git rev-parse HEAD)
- `StepSeparator(name)` / `RetryStepSeparator(name)` in `ui/log.go` produce the formatted separator strings passed to `WriteToLog`
- `Orchestrate(steps, runner, header, h)` in `ui/orchestrate.go` runs `ResolvedStep` slices in sequence; on non-terminated failure it sets the step to `StepFailed`, enters `ModeError`, and blocks on `h.Actions` — ActionContinue advances, ActionRetry re-runs the step with a separator, ActionQuit halts the loop
- `Run(executor, header, keyHandler, cfg)` in `workflow/run.go` is the top-level orchestration goroutine: displays banner, resolves GitHub username, loops over N iterations (each calling `Orchestrate` for iteration steps), runs finalization phase, writes completion summary, closes executor
- `StepExecutor` interface (in `run.go`) wraps `ui.StepRunner` + `CaptureOutput` + `Close`; `RunHeader` interface covers `SetIteration`, `SetStepState`, `SetFinalization`, `SetFinalizeStepState`
- `KeyHandler.ShortcutLine()` is a mutex-protected getter for the shortcut bar text; use it instead of field access when reading from a goroutine other than the key handler

## Project Discovery

- See [`docs/project-discovery.md`](docs/project-discovery.md) for full project discovery details including languages, frameworks, tooling, commands, test structure, documentation paths, and infrastructure for all projects in this repository.
- Default branch: main
- Docs: `docs/`
- Coding standards: `docs/coding-standards/` — Go error handling, testing, concurrency, API design, and Go-specific patterns

### ralph-bash

- Language: Bash
- No build, test, or lint commands

### ralph-tui

- Language: Go 1.23
- Test: `cd ralph-tui && go test ./...`
- Build: `cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui`
