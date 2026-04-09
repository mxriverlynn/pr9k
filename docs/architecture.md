# ralph-tui Architecture

ralph-tui is a Go TUI application that replaces the original `ralph-loop` bash script with a real-time, interactive orchestrator. It drives the `claude` CLI through multi-step coding loops — picking up GitHub issues, implementing features, writing tests, running code reviews, and pushing — all with live streaming output and keyboard-driven error recovery.

Built with [Glyph](https://useglyph.sh/) for TUI rendering, ralph-tui streams subprocess output in real time through an `io.Pipe`, displays workflow progress via a checkbox-based status header, and supports interactive error handling (retry, continue, quit) when steps fail.

## System Block Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                           main.go                                   │
│                                                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────────────────┐  │
│  │  CLI Parsing │  │ Step Loading │  │    OS Signal Handling     │  │
│  │  (cli.Parse  │  │ (steps.Load  │  │  SIGINT/SIGTERM → chan    │  │
│  │   Args)      │  │  Steps)      │  │  → KeyHandler.ForceQuit   │  │
│  └──────┬───────┘  └──────┬───────┘  └───────────┬───────────────┘  │
│         │                 │                      │                  │
│         ▼                 ▼                      ▼                  │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │                    workflow.Run (goroutine)                     ││
│  │                                                                 ││
│  │  ┌─────────────────────────────────────────────────────────┐    ││
│  │  │              Iteration Loop (1..N)                      │    ││
│  │  │                                                         │    ││
│  │  │  get_next_issue → git rev-parse HEAD → build steps      │    ││
│  │  │       │                                                 │    ││
│  │  │       ▼                                                 │    ││
│  │  │  ┌──────────────────────────────────────────────────┐   │    ││
│  │  │  │         ui.Orchestrate (step sequencer)          │   │    ││
│  │  │  │                                                  │   │    ││
│  │  │  │  for each step:                                  │   │    ││
│  │  │  │    drain Actions channel (check for quit)        │   │    ││
│  │  │  │    set step → Active                             │   │    ││
│  │  │  │    runner.RunStep(name, command)                 │   │    ││
│  │  │  │      ├─ success → step → Done                    │   │    ││
│  │  │  │      ├─ terminated → step → Done (skip)          │   │    ││
│  │  │  │      └─ failure → step → Failed                  │   │    ││
│  │  │  │           enter ModeError                        │   │    ││
│  │  │  │           wait on Actions:                       │   │    ││
│  │  │  │             c → continue   r → retry   q → quit  │   │    ││
│  │  │  └──────────────────────────────────────────────────┘   │    ││
│  │  └─────────────────────────────────────────────────────────┘    ││
│  │                                                                 ││
│  │  ┌─────────────────────────────────────────────────────────┐    ││
│  │  │           Finalization Phase                            │    ││
│  │  │  Deferred work → Lessons learned → Final git push       │    ││
│  │  │  (also runs through ui.Orchestrate)                     │    ││
│  │  └─────────────────────────────────────────────────────────┘    ││
│  └─────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────┘
```

## Data Flow Diagram

```
┌──────────────┐    ┌──────────────────┐    ┌─────────────────────┐
│  JSON Config │    │   Prompt Files   │    │   Helper Scripts    │
│  (configs/)  │    │   (prompts/)     │    │   (scripts/)        │
└──────┬───────┘    └────────┬─────────┘    └──────────┬──────────┘
       │                     │                         │
       ▼                     ▼                         ▼
┌──────────────┐    ┌──────────────────┐    ┌─────────────────────┐
│ steps.Load   │    │ steps.BuildPrompt│    │ runner.CaptureOutput│
│ Steps()      │    │ (prepend vars)   │    │ (issue ID, user,    │
│              │    │                  │    │  HEAD SHA)          │
└──────┬───────┘    └────────┬─────────┘    └──────────┬──────────┘
       │                     │                         │
       └─────────┬───────────┘                         │
                 ▼                                     │
       ┌──────────────────┐                            │
       │ buildIteration   │◄───────────────────────────┘
       │ Steps()          │
       │ → ResolvedStep[] │
       └────────┬─────────┘
                │
                ▼
       ┌──────────────────┐     ┌────────────────┐
       │ runner.RunStep() │────▶│  Subprocess    │
       │                  │     │  (claude/git/  │
       │                  │     │   scripts)     │
       │                  │     └───────┬────────┘
       │                  │             │ stdout/stderr
       │                  │             ▼
       │                  │     ┌────────────────┐
       │                  │     │ scanner        │
       │                  │     │ goroutines (2) │
       │                  │     └───┬────────┬───┘
       │                  │         │        │
       └──────────────────┘         │        │
                                    ▼        ▼
                            ┌────────┐  ┌─────────┐
                            │io.Pipe │  │ Logger  │
                            │(→ TUI) │  │(→ file) │
                            └────────┘  └─────────┘
```

## Keyboard & Mode State Machine

```
                  ┌─────────────┐
                  │ ModeNormal  │
                  │             │
                  │ n → skip    │
                  │ q ──────────┼──────┐
                  └──────┬──────┘      │
                         │             │
                   step fails          │
                         │             ▼
                  ┌──────▼──────┐  ┌───────────────┐
                  │ ModeError   │  │ModeQuitConfirm│
                  │             │  │               │
                  │ c → continue│  │ y → ActionQuit│
                  │ r → retry   │  │ n → previous  │
                  │ q ──────────┼─▶│    mode       │
                  └─────────────┘  └───────────────┘

  OS Signal (SIGINT/SIGTERM):
    → KeyHandler.ForceQuit()
    → cancel subprocess + inject ActionQuit
```

## Features

Each feature is documented in detail in its own file under [`docs/features/`](features/).

### [CLI & Configuration](features/cli-configuration.md)

Parses command-line arguments (`<iterations>` and optional `-project-dir` flag) and resolves the project directory. Uses a `reorderArgs` workaround to allow flags in any position despite Go's `flag` package stopping at the first positional argument. Resolves the project directory from the executable path via `os.Executable()` + `filepath.EvalSymlinks`.

**Package:** `internal/cli/`

### [Step Definitions & Prompt Building](features/step-definitions.md)

Loads workflow step definitions from JSON configuration files (`configs/ralph-steps.json`, `configs/ralph-finalize-steps.json`). Each step defines a name, model, prompt file, and whether it's a Claude step or a shell command. `BuildPrompt` reads prompt files and optionally prepends `ISSUENUMBER=` and `STARTINGSHA=` variables for iteration context.

**Package:** `internal/steps/`

### [Subprocess Execution & Streaming](features/subprocess-execution.md)

The `Runner` executes workflow steps as subprocesses, streaming stdout/stderr in real time through an `io.Pipe` to the TUI and a file logger simultaneously. Uses mutex-protected writes, `sync.WaitGroup` for pipe draining, and a 256KB scanner buffer. Supports graceful termination (SIGTERM with 3-second SIGKILL fallback) and single-value output capture for helper scripts.

**Package:** `internal/workflow/` (`workflow.go`)

### [Workflow Orchestration](features/workflow-orchestration.md)

The top-level `Run` function drives the entire workflow: displays a startup banner, fetches the GitHub username, loops over N iterations (each fetching an issue and running 8 steps through the step sequencer), then runs the finalization phase (deferred work, lessons learned, final push). The `Orchestrate` function sequences resolved steps, manages step state transitions, and handles error recovery by blocking on user input.

**Packages:** `internal/workflow/` (`run.go`), `internal/ui/` (`orchestrate.go`)

### [TUI Status Header](features/tui-display.md)

A pointer-mutable status display that Glyph reads on each render cycle. Shows the current iteration/issue on one line and step progress as two rows of 4 checkboxes each (8 steps total). Each step displays as `[ ]` (pending), `[▸]` (active), `[✓]` (done), or `[✗]` (failed). Switches to finalization mode with its own step names when the iteration loop completes.

**Package:** `internal/ui/` (`header.go`, `log.go`)

### [Keyboard Input & Error Recovery](features/keyboard-input.md)

A three-mode state machine (`ModeNormal`, `ModeError`, `ModeQuitConfirm`) that routes keypresses and communicates user decisions to the orchestration goroutine via a buffered `Actions` channel. In normal mode, `n` skips the current step and `q` enters quit confirmation. In error mode (entered when a step fails), `c` continues, `r` retries, and `q` enters quit confirmation. Each mode displays its own shortcut bar text.

**Package:** `internal/ui/` (`ui.go`)

### [Signal Handling & Shutdown](features/signal-handling.md)

Listens for SIGINT and SIGTERM via `os/signal.Notify`. On receipt, calls `KeyHandler.ForceQuit()` which terminates the current subprocess and injects `ActionQuit` into the actions channel using a non-blocking send. The orchestration loop picks up the quit action before the next step starts, enabling clean shutdown. The main goroutine tracks whether a signal was received to choose the exit code (0 for normal, 1 for signaled).

**Package:** `cmd/ralph-tui/` (`main.go`)

### [File Logging](features/file-logging.md)

A concurrent-safe file logger that writes timestamped, context-prefixed lines to `logs/ralph-YYYY-MM-DD-HHMMSS.log`. Each line includes a timestamp, optional iteration context (e.g., "Iteration 1/3"), and step name. Protected by `sync.Mutex` for concurrent writes from multiple scanner goroutines. Uses `bufio.Writer` with explicit flush on close.

**Package:** `internal/logger/`

## Package Dependency Graph

```
cmd/ralph-tui/main.go
    ├── internal/cli        (argument parsing)
    ├── internal/logger     (file logging)
    ├── internal/steps      (step loading)
    ├── internal/ui         (key handling, header, orchestration)
    └── internal/workflow   (subprocess execution, run loop)
            ├── internal/logger
            ├── internal/steps
            └── internal/ui
```

## Key Design Principles

- **Streaming over buffering**: Subprocess output streams through `io.Pipe` in real time — no buffered collection and dump.
- **Pointer-mutable state**: The `StatusHeader` uses exported string fields that Glyph reads by pointer on each render; callers mutate in place.
- **Channel-based coordination**: The `Actions` channel is the sole communication path from keyboard/signal handlers to the orchestration goroutine.
- **Non-blocking sends for signal safety**: `ForceQuit` uses `select`/`default` to inject `ActionQuit` without blocking, making it safe to call from a signal handler goroutine.
- **Interface-driven testability**: `StepRunner`, `StepHeader`, `StepExecutor`, and `RunHeader` interfaces decouple orchestration from concrete implementations.

## Additional Information

- **How-To Guides:**
  - [Building Custom Workflows](how-to/building-custom-workflows.md) — Creating custom step sequences, adding prompts, mixing Claude and shell steps
  - [Variable Output & Injection](how-to/variable-output-and-injection.md) — Variable injection into prompts/commands and file-based data passing between steps
- [ralph-tui Plan](plans/ralph-tui.md) — Original specification with acceptance criteria, verification checklist, and design rationale
- [Project Discovery](project-discovery.md) — Repository-level attributes: languages, frameworks, tooling, commands, and configuration
- **Coding Standards** — Conventions that govern ralph-tui implementation:
  - [Concurrency](coding-standards/concurrency.md) — Mutex patterns, WaitGroup drain, channel dispatch, non-blocking sends
  - [Error Handling](coding-standards/error-handling.md) — Package-prefixed errors, file paths in I/O errors, scanner error checking
  - [API Design](coding-standards/api-design.md) — Bounds guards, precondition validation, adapter types, platform assumptions
  - [Go Patterns](coding-standards/go-patterns.md) — Flag reordering, symlink-safe paths, slice immutability, scanner buffers
  - [Testing](coding-standards/testing.md) — Race detector, idempotent close, bounds testing, test doubles with mutexes
