# PR9K: Power-Ralph.9000

**pr9k (Power-Ralph.9000)** is an automated development workflow orchestrator that drives the `claude` CLI through multi-step coding loops. It picks up GitHub issues labeled "ralph", implements features, writes tests, runs code reviews, and pushes — all unattended.

Based on [AI Hero's Getting Started with Ralph](https://www.aihero.dev/getting-started-with-ralph).

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

# Build the TUI orchestrator
cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui
```

### Quick Start

From the **target repo** (the repo where you want Ralph to work):

```bash
# Run 3 iterations using the bash orchestrator
path/to/pr9k/ralph-loop 3

# Or use the TUI orchestrator
path/to/pr9k/ralph-tui 3
```

Ralph will find the next open issue labeled `ralph`, implement the feature, write tests, run a code review, fix review findings, close the issue, update docs, and push — then repeat for the next issue.

## How To

### Run the bash orchestrator

```bash
# From your target repo:
path/to/pr9k/ralph-loop <iterations>
```

### Run the TUI orchestrator

```bash
# From your target repo:
path/to/pr9k/ralph-tui <iterations>

# Or specify the project directory explicitly:
path/to/pr9k/ralph-tui <iterations> -project-dir path/to/pr9k
```

Use `go build` — `go run` won't work because the project directory is resolved from the executable path.

### Run a single prompt interactively (human-in-the-loop)

```bash
path/to/pr9k/ralph-hitl [prompt-name]
```

### Keyboard controls (TUI)

During a run:
- **n** — skip the current step
- **q** → **y** — quit Ralph

When a step fails:
- **c** — continue to the next step
- **r** — retry the failed step
- **q** → **y** — quit Ralph

## Documentation

- [Architecture Overview](docs/architecture.md) — System-level architecture with block diagrams, data flow, and package dependencies
- **Feature Documentation** (in [`docs/features/`](docs/features/)):
  - [CLI & Configuration](docs/features/cli-configuration.md) — Argument parsing and project directory resolution
  - [Step Definitions & Prompt Building](docs/features/step-definitions.md) — JSON step configs and prompt construction
  - [Subprocess Execution & Streaming](docs/features/subprocess-execution.md) — Real-time subprocess output streaming
  - [Workflow Orchestration](docs/features/workflow-orchestration.md) — Iteration loop and step sequencing with error recovery
  - [TUI Status Header](docs/features/tui-display.md) — Checkbox-based progress display
  - [Keyboard Input & Error Recovery](docs/features/keyboard-input.md) — Keyboard state machine
  - [Signal Handling & Shutdown](docs/features/signal-handling.md) — Clean shutdown on SIGINT/SIGTERM
  - [File Logging](docs/features/file-logging.md) — Timestamped log file output
- [Coding Standards](docs/coding-standards/) — Go error handling, testing, concurrency, API design, and Go-specific patterns
- [ralph-tui Plan](docs/plans/ralph-tui.md) — Original specification and design decisions

## Copyright & License

Copyright River Bailey. Licensed under the Apache License 2.0. See [LICENSE](./LICENSE) for details.
