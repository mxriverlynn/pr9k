# ralph-tui Architecture

ralph-tui is a Go TUI application that replaces the original `ralph-loop` bash script with a real-time, interactive orchestrator. It drives the `claude` CLI through multi-step coding loops — picking up GitHub issues, implementing features, writing tests, running code reviews, and pushing — all with live streaming output and keyboard-driven error recovery.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) + [bubbles/viewport](https://github.com/charmbracelet/bubbles) for TUI rendering, ralph-tui streams subprocess output in real time via a `sendLine` callback through a buffered channel, displays workflow progress via a checkbox-based status header, and supports interactive error handling (retry, continue, quit) when steps fail.

## System Block Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                           main.go                                   │
│                                                                     │
│  ┌──────────────┐  ┌──────────────┐  ┌───────────────────────────┐  │
│  │  CLI Parsing │  │ Step Loading │  │    OS Signal Handling     │  │
│  │  (cli.       │  │ (steps.Load  │  │  SIGINT/SIGTERM → chan    │  │
│  │   Execute)   │  │  Steps)      │  │  → KeyHandler.ForceQuit   │  │
│  └──────┬───────┘  └──────┬───────┘  └───────────┬───────────────┘  │
│         │                 │                      │                  │
│         ▼                 ▼                      ▼                  │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │                    workflow.Run (goroutine)                     ││
│  │                                                                 ││
│  │  ┌─────────────────────────────────────────────────────────┐    ││
│  │  │  Initialize Phase (once, before loop)                   │    ││
│  │  │  buildStep → ui.Orchestrate → LastCapture → VarTable    │    ││
│  │  │  (noopHeader: no TUI checkbox updates during init)      │    ││
│  │  └─────────────────────────────────────────────────────────┘    ││
│  │                                                                 ││
│  │  ┌─────────────────────────────────────────────────────────┐    ││
│  │  │     Iteration Loop (1..N, or until BreakLoopIfEmpty)    │    ││
│  │  │                                                         │    ││
│  │  │  VarTable.ResetIteration → buildStep → Orchestrate      │    ││
│  │  │       │                                                 │    ││
│  │  │       ▼                                                 │    ││
│  │  │  ┌──────────────────────────────────────────────────┐   │    ││
│  │  │  │         ui.Orchestrate (step sequencer)          │   │    ││
│  │  │  │                                                  │   │    ││
│  │  │  │  for each step:                                  │   │    ││
│  │  │  │    drain Actions channel (check for quit)        │   │    ││
│  │  │  │    set step → Active                             │   │    ││
│  │  │  │    stepDispatcher → RunStep / RunSandboxedStep   │   │    ││
│  │  │  │      (IsClaude: docker sandbox; else: direct)    │   │    ││
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
       ▼                     ▼                         │
┌──────────────┐    ┌──────────────────┐               │
│ steps.Load   │    │ steps.BuildPrompt│               │ (run as
│ Steps()      │    │ ({{VAR}} subst.) │               │  initialize
│              │    │                  │               │  steps via
└──────┬───────┘    └────────┬─────────┘               │  RunStep +
       │                     │                         │  LastCapture)
       └─────────┬───────────┘                         │
                 ▼                                     │
       ┌──────────────────┐    ┌──────────────────┐    │
       │   buildStep()    │    │    VarTable       │◄───┘
       │ (per phase, per  │◄───│  (persistent +   │
       │  step)           │    │   iteration       │
       │ → ResolvedStep   │    │   scopes)         │
       └────────┬─────────┘    └──────────────────┘
                │
                ▼
       ┌──────────────────┐     ┌────────────────────────────┐
       │  stepDispatcher  │────▶│  IsClaude?                 │
       │  (per step)      │     │  yes: RunSandboxedStep     │
       │                  │     │    → docker run (sandbox)  │
       │                  │     │  no: RunStep               │
       │                  │     │    → direct subprocess     │
       │                  │     └───────┬────────────────────┘
       │                  │             │ stdout/stderr
       │                  │             ▼
       │                  │     ┌────────────────┐
       │                  │     │ scanner        │
       │                  │     │ goroutines (2) │
       │                  │     │ stdout: capture│
       │                  │     │ stderr: forward│
       │                  │     └───┬────────┬───┘
       │                  │         │        │
       └──────────────────┘         │        │
              │                     ▼        ▼
              │             sendLine(line)  Logger
              │             (snapshot-then-  (file)
              │              unlock; via
              │              SetSender)
              │                  │
              │             buffered lineCh
              │             → drain goroutine
              │             → program.Send(LogLinesMsg)
              │             → Bubble Tea TUI
              │
              ▼ LastCapture()
       ┌──────────────────┐
       │ VarTable.Bind    │
       │ (CaptureAs vars) │
       └──────────────────┘
```

## Keyboard & Mode State Machine

```
                  ┌─────────────┐
                  │ ModeNormal  │
                  │             │
                  │ n ──────────┼──────────────────────┐
                  │ q ──────────┼──────┐               │
                  └──────┬──────┘      │               │
                         │             │               │
                   step fails          │               │
                         │             │               ▼
                         ▼             ▼       ┌────────────────────┐
                  ┌─────────────┐  ┌──────────────────┐│ ModeNextConfirm  │
                  │ ModeError   │  │ ModeQuitConfirm  ││                  │
                  │             │  │                  ││ y → cancel step  │
                  │ c → continue│  │ y → ModeQuitting ││     + prevMode   │
                  │ r → retry   │  │     + ForceQuit  ││ n, Esc → prevMode│
                  │ q ──────────┼─▶│ n, Esc → prevMode│└────────────────────┘
                  └─────────────┘  └────────┬─────────┘
                                            │ y
                                            ▼
                                   ┌─────────────────┐
                                   │  ModeQuitting   │
                                   │                 │
                                   │ footer shows    │
                                   │ "Quitting..."   │
                                   │ (terminal)      │
                                   └─────────────────┘

  Normal completion:
    → Run returns after writing the completion summary
    → workflow goroutine enters ModeDone ("q quit" footer)
    → user reviews output, then presses q → y to exit

                  ┌─────────────┐
                  │  ModeDone   │
                  │             │
                  │ q ──────────┼──▶ ModeQuitConfirm
                  │ (only key)  │    → y → tea.QuitMsg
                  └─────────────┘

  OS Signal (SIGINT/SIGTERM):
    → KeyHandler.ForceQuit()
    → cancel subprocess + inject ActionQuit
    (unified with the QuitConfirm 'y' path;
     active in all modes including ModeDone)
```

## Features

Each feature is documented in detail in its own file under [`docs/features/`](features/).

### [CLI & Configuration](features/cli-configuration.md)

Parses command-line flags (`--iterations`/`-n`, `--workflow-dir`, `--project-dir`, and `--version`/`-v`) using [spf13/cobra](https://github.com/spf13/cobra). `--workflow-dir` resolves the install directory (where `ralph-steps.json`, `prompts/`, and `scripts/` live) from the executable path via `os.Executable()` + `filepath.EvalSymlinks` when not given explicitly. `--project-dir` resolves the target repo from `os.Getwd()` + `filepath.EvalSymlinks` when not given explicitly. Neither dir flag has a short form. Iterations defaults to 0 (run until done). The `--version` flag is wired through cobra's built-in `cmd.Version` field, which reads from `internal/version.Version` (the single source of truth for the app version — see the [Versioning](coding-standards/versioning.md) standard).

**Packages:** `internal/cli/`, `internal/version/`

### [Step Definitions & Prompt Building](features/step-definitions.md)

Loads workflow step definitions from `ralph-steps.json`, which contains initialize, iteration, and finalization step groups. Each step defines a name, model, prompt file, and whether it's a Claude step or a shell command. `BuildPrompt` reads prompt files and applies `{{VAR}}` substitution using the active `VarTable` and phase.

**Package:** `internal/steps/`

### [Subprocess Execution & Streaming](features/subprocess-execution.md)

The `Runner` executes workflow steps as subprocesses, streaming stdout/stderr in real time via a `sendLine` callback (installed via `SetSender`) to a buffered channel in `main.go`; a drain goroutine coalesces lines into batched `LogLinesMsg` values sent to the Bubble Tea program. Scanner output is also written to the file logger. Uses mutex-protected writes with snapshot-then-unlock for the callback, `sync.WaitGroup` for pipe draining, and a 256KB scanner buffer. Supports graceful termination (SIGTERM with 3-second SIGKILL fallback). After each successful `RunStep`, the last non-empty stdout line is stored and retrievable via `LastCapture()`, which the orchestrator uses to bind `CaptureAs` variables into the `VarTable`. `ResolveCommand` (in `run.go`) applies `{{VAR}}` substitution and resolves relative script paths.

**Package:** `internal/workflow/` (`workflow.go`, `run.go`)

### [Workflow Orchestration](features/workflow-orchestration.md)

The top-level `Run` function drives the entire workflow in three config-defined phases: initialize (runs once before the loop, binding `CaptureAs` values such as GitHub username and issue ID into the persistent VarTable), iteration loop (bounded to N when `--iterations N > 0`, or until `BreakLoopIfEmpty` fires when `--iterations 0`), and finalization (deferred work, lessons learned, final push). All step resolution goes through `buildStep` and `{{VAR}}` substitution via `VarTable`. The `Orchestrate` function sequences resolved steps, manages step state transitions, and handles error recovery by blocking on user input.

**Packages:** `internal/workflow/` (`run.go`), `internal/ui/` (`orchestrate.go`)

### [TUI Status Header & Log Display](features/tui-display.md)

A Bubble Tea `Model` assembled row-by-row in `Model.View()` as a hand-built rounded frame (no `lipgloss.Border` wrapper, so the two internal horizontal rules can use `├─┤` T-junction glyphs that visually connect to the `│` side borders). The current iteration/issue is embedded into the top-border title — `Power-Ralph.9000 — Iteration N/M — Issue #<id>` in bounded mode, or `Power-Ralph.9000 — Iteration N — Issue #<id>` when running unbounded (`--iterations 0`); the same string is set as the OS window title via `tea.SetWindowTitle`. The app name `Power-Ralph.9000` (from the `AppTitle` constant) renders green and the iteration detail after the ` — ` separator renders white. Step progress displays as a dynamic grid of rows, each holding `HeaderCols` (4) checkboxes, sized at startup to fit the largest phase. Each step shows as `[ ]` (pending), `[▸]` (active), `[✓]` (done), `[✗]` (failed), or `[-]` (skipped). `SetPhaseSteps` swaps the header to a new phase's step names at the start of each phase (initialize, iteration, finalize). State updates are sent as typed messages via `HeaderProxy` (which calls `program.Send`) so header mutations never race with the Bubble Tea Update goroutine. The log body is rendered in white and is also structured: `log.go` helpers produce full-width `PhaseBanner` headings, per-iteration `StepSeparator` lines, per-step `StepStartBanner` headings, `CaptureLog` lines for `captureAs` bindings, and the final `CompletionSummary` — all sized via `ui.TerminalWidth()` with an 80-column fallback. D23 heartbeat: when no stream-json event arrives for ≥15 s during an active claude step, the iteration title appends `  ⋯ thinking (Ns)`; the suffix updates in-place each second and is cleared as soon as the next event arrives. The heartbeat reader is installed via `StatusHeader.SetHeartbeatReader(runner)` in `main.go` before the model is constructed; a separate 1-second ticker goroutine in `main.go` dispatches `HeartbeatTickMsg` via `program.Send` — `Model.Init()` returns nil and the ticker is not owned by the Bubble Tea event loop. `Model.Update()` delegates `HeartbeatTickMsg` to `StatusHeader.HandleHeartbeatTick()`. The `Runner` in `workflow.go` implements `HeartbeatReader` by exposing `HeartbeatSilence() (time.Duration, bool)` under `processMu`.

**Package:** `internal/ui/` (`header.go`, `keys.go`, `log.go`, `log_panel.go`, `messages.go`, `model.go`, `orchestrate.go`, `terminal.go`)

### [Keyboard Input & Error Recovery](features/keyboard-input.md)

A six-mode state machine (`ModeNormal`, `ModeError`, `ModeQuitConfirm`, `ModeNextConfirm`, `ModeDone`, `ModeQuitting`) that routes keypresses and communicates user decisions to the orchestration goroutine via a buffered `Actions` channel. In normal mode, `n` enters a skip-confirmation prompt (`ModeNextConfirm`) and `q` enters quit confirmation. In error mode (entered when a step fails), `c` continues, `r` retries, and `q` enters quit confirmation. In quit-confirm mode, `y` flips to `ModeQuitting` (footer shows `Quitting...`) and calls `ForceQuit`; `n` or `<Escape>` cancel. In next-confirm mode, `y` cancels the subprocess and returns to the previous mode; `n` or `<Escape>` cancel the skip. When the workflow finishes normally, the TUI enters `ModeDone` (footer shows `q quit`) so the user can review output before quitting via `q` → `y`. Each mode displays its own shortcut bar text.

**Package:** `internal/ui/` (`ui.go`, `keys.go`)

### [Signal Handling & Shutdown](features/signal-handling.md)

Listens for SIGINT and SIGTERM via `os/signal.Notify`. On receipt, calls `KeyHandler.ForceQuit()` which first flips mode to `ModeQuitting` and updates the shortcut bar (so the footer shows `"Quitting..."` immediately), then terminates the current subprocess and injects `ActionQuit` into the actions channel using a non-blocking send. The orchestration loop picks up the quit action before the next step starts, enabling clean shutdown. The main goroutine tracks whether a signal was received to choose the exit code (0 for normal, 1 for signaled).

**Package:** `cmd/ralph-tui/` (`main.go`)

### [File Logging](features/file-logging.md)

A concurrent-safe file logger that writes timestamped, context-prefixed lines to `logs/ralph-YYYY-MM-DD-HHMMSS.mmm.log` (millisecond precision). Each line includes a timestamp, optional iteration context (e.g., "Iteration 1/3"), and step name. Protected by `sync.Mutex` for concurrent writes from multiple scanner goroutines. Uses `bufio.Writer` with explicit flush on close. Exposes `RunStamp()` — the log basename without `.log` — which `main.go` passes into `RunConfig.RunStamp` for artifact directory naming by `claudestream.Pipeline`.

**Package:** `internal/logger/`

### [Variable State Management](features/variable-state.md)

`VarTable` owns all runtime variable state for a single run. It maintains two scoped tables — persistent (survives the whole run) and iteration (cleared at the start of each iteration) — plus seven built-in variables seeded from CLI flags and updated by the orchestrator (`WORKFLOW_DIR`, `PROJECT_DIR`, `MAX_ITER`, `ITER`, `STEP_NUM`, `STEP_COUNT`, `STEP_NAME`). Resolution order during an iteration step is iteration table → persistent table; during initialize or finalize, only the persistent table is consulted. `captureAs` bindings from step output are routed to the correct scope based on the active workflow phase.

**Package:** `internal/vars/`

### [Config Validation](features/config-validation.md)

Validates `ralph-steps.json` against all ten D13 categories in a single pass, collecting every error before returning. Checks file presence and parseability, per-step schema shape (including `isClaude`, `captureAs`, `breakLoopIfEmpty`), phase size, referenced file existence, and variable scope resolution. Also validates the top-level `env` array (Category 10) and enforces sandbox isolation rules B and C (host-path tokens in prompts, and captureAs+host-path in commands; Rule A was removed in issue #91 — captureAs on claude steps is now valid and binds via the Aggregator). Returns a slice of structured `Error` values; an empty slice means valid. Wired into `main.go` immediately after `steps.LoadSteps`; validation failures exit 1 with structured errors on stderr before the TUI starts.

**Package:** `internal/validator/`

### [Docker Sandbox](features/sandbox.md)

The `internal/sandbox` package constructs the `docker run` argv that wraps every Claude step, manages the container ID file (cidfile) lifecycle, and provides a terminator closure that signals the running container on shutdown. `BuildRunArgs` is a pure function (uid/gid as parameters) that emits `--mount type=bind,...` mounts for the target repo and Claude profile directory, an env passthrough with deduplication and set-on-host filtering, and the claude invocation flags. `BuiltinEnvAllowlist` names five env vars always included in the passthrough. `Path()`/`Cleanup()` reserve and clean up the cidfile path. `NewTerminator` returns a closure that polls the cidfile for the container ID, delivers `docker kill --signal` to the container, and falls back to signaling the docker CLI process if the container never started.

**Package:** `internal/sandbox/`

### [Preflight Checks](features/preflight.md)

Startup validation that runs before the main orchestration loop. Resolves and validates the Claude profile directory (`ResolveProfileDir` / `CheckProfileDir`), checks for Docker binary availability, daemon reachability, and sandbox image presence via the injectable `Prober` interface, and verifies the credentials file is non-empty (`CheckCredentials`). All checks are collected before returning (collect-all-errors via `Run`) so the caller sees the full list of failures in one pass. `RealProber` uses `exec.CommandContext` with a 10-second timeout for each Docker probe to guard against a frozen daemon.

**Package:** `internal/preflight/`

### [Stream JSON Pipeline](features/stream-json-pipeline.md)

Parses, renders, aggregates, and persists the NDJSON stream emitted by `claude -p --output-format stream-json --verbose`. `Parser` dispatches raw lines to typed event structs (`SystemEvent`, `AssistantEvent`, `UserEvent`, `ResultEvent`, `RateLimitEvent`); malformed lines return a `*MalformedLineError` carrying the raw bytes. `Renderer` converts events to human-readable display lines for the TUI (assistant text split on newlines, tool_use as `→ Name summary` indicators, nothing for thinking/user/result events) and produces a per-step closing summary via `Finalize`. `Aggregator` folds events into `StepStats` (token counts, cost, duration, session ID) and exposes `Result()` for `captureAs` binding and `Err()` for `is_error` detection. `RawWriter` persists verbatim bytes to a per-step `.jsonl` file (`O_TRUNC` on open so retries overwrite). `Slug` converts step names to kebab-case identifiers for filenames. `Pipeline` composes all four behind a single `Observe(line []byte) []string` entry point, tracks the first write error via `WriteErr()`, stamps `LastEventAt` atomically for the heartbeat goroutine, and appends a sentinel line after the result event for crash-resilience.

**Package:** `internal/claudestream/`

## Package Dependency Graph

```
cmd/ralph-tui/main.go
    ├── internal/cli           (argument parsing)
    │       └── internal/version
    ├── internal/logger        (file logging)
    ├── internal/preflight     (startup validation)
    │       └── internal/sandbox
    ├── internal/sandbox       (docker run argv, cidfile, terminator)
    ├── internal/steps         (step loading)
    ├── internal/ui            (key handling, header, orchestration)
    ├── internal/validator     (config validation)
    │       └── internal/vars
    ├── internal/vars          (runtime variable state)
    ├── internal/version       (compile-time Version constant)
    └── internal/workflow      (subprocess execution, run loop)
            ├── internal/claudestream  (stream-json pipeline)
            ├── internal/logger
            ├── internal/preflight
            ├── internal/sandbox
            ├── internal/steps
            ├── internal/ui
            └── internal/vars

internal/claudestream          (stream-json parsing, rendering, aggregation)
    (no internal dependencies)
```

## Key Design Principles

- **Narrow-reading principle**: Ralph-tui facilitates the workflow; it does not define it. Workflow content (steps, commands, prompts) lives in `ralph-steps.json`. Go code owns only runtime mechanics — phase sequencing, loop bounds, variable substitution, and TUI chrome. Any PR that adds Ralph-specific knowledge to Go code must justify the exception against [ADR: Narrow-Reading Principle](adr/20260410170952-narrow-reading-principle.md).
- **Streaming over buffering**: Subprocess output is forwarded line-by-line via the `sendLine` callback into a buffered channel; the drain goroutine coalesces lines before sending `LogLinesMsg` to the Bubble Tea program — no bulk buffering and dump.
- **Message-passing state**: `StatusHeader` mutations are never applied directly by the orchestration goroutine. They are wrapped as typed messages by `HeaderProxy` and sent via `program.Send`, received on the Bubble Tea Update goroutine, and applied there — eliminating header data races. The completion summary is *not* a header method — it is written to the log body via `ui.CompletionSummary` so it scrolls with the rest of the run transcript.
- **Channel-based coordination**: The `Actions` channel is the sole communication path from keyboard/signal handlers to the orchestration goroutine.
- **Non-blocking sends for signal safety**: `ForceQuit` uses `select`/`default` to inject `ActionQuit` without blocking, making it safe to call from a signal handler goroutine.
- **Interface-driven testability**: `StepRunner`, `StepHeader`, `StepExecutor`, and `RunHeader` interfaces decouple orchestration from concrete implementations.

## Additional Information

- **How-To Guides:**
  - [Building Custom Workflows](how-to/building-custom-workflows.md) — Creating custom step sequences, adding prompts, mixing Claude and shell steps
  - [Variable Output & Injection](how-to/variable-output-and-injection.md) — Variable injection into prompts/commands and file-based data passing between steps
  - [Passing Environment Variables](how-to/passing-environment-variables.md) — Forwarding host env vars into the Docker sandbox via the `env` field
- [ralph-tui Plan](plans/ralph-tui.md) — Original specification with acceptance criteria, verification checklist, and design rationale
- [Project Discovery](project-discovery.md) — Repository-level attributes: languages, frameworks, tooling, commands, and configuration
- **Coding Standards** — Conventions that govern ralph-tui implementation:
  - [Concurrency](coding-standards/concurrency.md) — Mutex patterns, WaitGroup drain, channel dispatch, non-blocking sends
  - [Error Handling](coding-standards/error-handling.md) — Package-prefixed errors, file paths in I/O errors, scanner error checking
  - [API Design](coding-standards/api-design.md) — Bounds guards, precondition validation, adapter types, platform assumptions
  - [Go Patterns](coding-standards/go-patterns.md) — Symlink-safe paths, slice immutability, scanner buffers
  - [Testing](coding-standards/testing.md) — Race detector, idempotent close, bounds testing, test doubles with mutexes
  - [Lint and Tooling](coding-standards/lint-and-tooling.md) — Lint suppressions are prohibited; fix the root cause or escalate
  - [Versioning](coding-standards/versioning.md) — Semver rules specific to ralph-tui, the `version.Version` single source of truth, and what counts as the app's public API
