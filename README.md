# PR9K: Power-Ralph.9000

**pr9k (Power-Ralph.9000)** is an automated development workflow orchestrator that drives the `claude` CLI through multi-step coding loops. It picks up GitHub issues labeled "ralph", implements features, writes tests, runs code reviews, and pushes — all unattended.

Based on [AI Hero's Getting Started with Ralph](https://www.aihero.dev/getting-started-with-ralph).

<img src="images/power-ralph-9000.jpg">

## Getting Started

### Prerequisites

- [Go 1.23+](https://go.dev/dl/) (for ralph-tui)
- [GitHub CLI (`gh`)](https://cli.github.com/) — authenticated with access to your target repo
- [Claude CLI (`claude`)](https://docs.anthropic.com/en/docs/claude-cli) — installed and authenticated
- A GitHub repo with issues labeled `ralph` assigned to your user

### Installation

```bash
git clone https://github.com/mxriverlynn/pr9k.git
cd pr9k

# Build the orchestrator
make build
```

### Quick Start

From the **target repo** (the repo where you want Ralph to work):

```bash
# Run until no issues remain (until-done mode)
path/to/pr9k/bin/ralph-tui

# Or cap at 3 iterations
path/to/pr9k/bin/ralph-tui -n 3
```

Ralph will find the next open issue labeled `ralph`, implement the feature, write tests, run a code review, fix review findings, close the issue, update docs, and push — then repeat for the next issue. When run without `-n`, Ralph keeps going until `get_next_issue` finds no more issues.

## How To

### Run the orchestrator

```bash
# From your target repo — run until no issues remain:
path/to/pr9k/bin/ralph-tui

# Cap at N iterations:
path/to/pr9k/bin/ralph-tui -n <iterations>

# Specify the project directory explicitly:
path/to/pr9k/bin/ralph-tui -p path/to/pr9k

# Build and run directly (without make):
cd path/to/pr9k/ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui
path/to/pr9k/ralph-tui -n <iterations>
```

Omitting `-n` (or passing `-n 0`) runs Ralph in until-done mode: it keeps picking up issues until `get_next_issue` finds none. Passing `-n N` caps the run at N iterations.

Use `go build` — `go run` won't work because the project directory is resolved from the executable path.

### Keyboard controls (TUI)

During a run:
- **↑/k** / **↓/j** — scroll the log panel
- **n** — skip the current step (SIGTERMs the subprocess)
- **q** → **y** — quit Ralph (or **n** / **Esc** to cancel the quit)

When a step fails:
- **c** — continue to the next step
- **r** — retry the failed step
- **q** → **y** — quit Ralph

When the workflow completes:
- any key — exit

See [Recovering from Step Failures](docs/how-to/recovering-from-step-failures.md) and [Quitting Gracefully](docs/how-to/quitting-gracefully.md) for the full interaction model.

## Documentation

- [Architecture Overview](docs/architecture.md) — System-level architecture with block diagrams, data flow, and package dependencies
- **How-To Guides** (in [`docs/how-to/`](docs/how-to/)) — problem-focused guides for using ralph-tui on your own projects:
  - [Getting Started](docs/how-to/getting-started.md) — Install, first run against your own repo, quick tour of the TUI
  - [Reading the TUI](docs/how-to/reading-the-tui.md) — The four regions of the screen: header, checkbox grid, log panel, footer
  - [Building Custom Workflows](docs/how-to/building-custom-workflows.md) — Creating custom step sequences and prompt files
  - [Variable Output & Injection](docs/how-to/variable-output-and-injection.md) — How `{{VAR}}` tokens are resolved and how steps pass data via files
  - [Capturing Step Output](docs/how-to/capturing-step-output.md) — Using `captureAs` to bind step stdout to a variable
  - [Breaking Out of the Loop](docs/how-to/breaking-out-of-the-loop.md) — Using `breakLoopIfEmpty` to exit the iteration loop dynamically
  - [Recovering from Step Failures](docs/how-to/recovering-from-step-failures.md) — Error mode decisions: continue, retry, or quit
  - [Quitting Gracefully](docs/how-to/quitting-gracefully.md) — The `q`/`y` flow, Escape cancel, SIGINT, exit codes
  - [Debugging a Run](docs/how-to/debugging-a-run.md) — Reading the log file, finding captures, reproducing failures
- **Feature Documentation** (in [`docs/features/`](docs/features/)) — implementation details for each ralph-tui package:
  - [CLI & Configuration](docs/features/cli-configuration.md) — Argument parsing and project directory resolution
  - [Step Definitions & Prompt Building](docs/features/step-definitions.md) — JSON step configs and prompt construction
  - [Config Validation](docs/features/config-validation.md) — D13 validator: schema, scopes, referenced-file existence
  - [Subprocess Execution & Streaming](docs/features/subprocess-execution.md) — Real-time subprocess output streaming
  - [Workflow Orchestration](docs/features/workflow-orchestration.md) — Iteration loop, phase banners, capture logs, completion summary
  - [TUI Status Header & Log Display](docs/features/tui-display.md) — Checkbox grid plus phase/step banner rhythm
  - [Keyboard Input & Error Recovery](docs/features/keyboard-input.md) — Five-mode keyboard state machine
  - [Signal Handling & Shutdown](docs/features/signal-handling.md) — Clean shutdown on SIGINT/SIGTERM, unified with quit-confirm
  - [File Logging](docs/features/file-logging.md) — Timestamped, context-prefixed log file output
  - [Variable State Management](docs/features/variable-state.md) — VarTable scopes and phase transitions
- [Coding Standards](docs/coding-standards/) — Go error handling, testing, concurrency, API design, and Go-specific patterns
- [Architectural Decision Records (ADRs)](docs/adr/) — Historical decisions including the narrow-reading principle and cobra CLI choice
- [ralph-tui Plan](docs/plans/ralph-tui.md) — Original specification and design decisions

## Copyright & License

Copyright River Bailey. Licensed under the Apache License 2.0. See [LICENSE](./LICENSE) for details.
