# PR9K: Power-Ralph.9000

<img src="images/pr9k-banner.png">

**pr9k (Power-Ralph.9000)** is an automated development workflow orchestrator that drives the `claude` CLI through multi-step coding loops. It picks up GitHub issues labeled `ralph`, implements features, writes tests, runs code reviews, and pushes — all unattended.

Based on [AI Hero's Getting Started with Ralph](https://www.aihero.dev/getting-started-with-ralph).

## When to use pr9k

**Reach for pr9k when** you have a backlog of small, well-defined GitHub issues you want a Claude-driven loop to work through unattended — implement, test, review, push, repeat.

**Don't reach for it when** the work needs human judgment per change, when you don't want every iteration billed against your Anthropic account, or when your project is on Windows (pr9k requires a Unix-like terminal).

**A successful run looks like** one closed GitHub issue and one pushed commit per iteration, plus a finalizing code-review pass and updated docs at the end of the run.

## Getting Started

### Prerequisites

- [Go 1.26.2](https://go.dev/dl/) (to build pr9k)
- [Docker](https://docs.docker.com/get-docker/) (Docker Desktop on macOS/Windows, Docker Engine on Linux) — every Claude step runs inside a sandbox container; Docker is a required runtime dependency, not optional. See [Setting Up the Docker Sandbox](docs/how-to/setting-up-docker-sandbox.md)
- [GitHub CLI (`gh`)](https://cli.github.com/) — authenticated against the target repo (`gh auth status`)
- [Claude CLI (`claude`)](https://docs.anthropic.com/en/docs/claude-cli) — installed and authenticated; its credentials are reused inside the sandbox
- A target GitHub repo with at least one open issue labeled `ralph` and assigned to your user
- A Unix-like terminal — pr9k uses POSIX terminal sizing, so it runs on macOS and Linux but not Windows

### Installation

```bash
git clone https://github.com/mxriverlynn/pr9k.git
cd pr9k
make build
```

`make build` produces a self-contained `bin/` directory containing the `pr9k` binary plus the bundled workflow, prompts, and helper scripts. You can copy `bin/` elsewhere or symlink `bin/pr9k` into your `PATH`.

### Quick Start

From the **target repo** (the repo where you want pr9k to work).

Add the following to your `.gitignore` file:
```
# pr9k temp files and logs
.pr9k/logs/
.pr9k/iteration.jsonl
.pr9k/artifacts/
```

Then run the following:
```bash
# Run until no ralph-labeled issues remain (until-done mode):
/path/to/pr9k/bin/pr9k

# Or cap at 3 iterations for a dry run:
/path/to/pr9k/bin/pr9k -n 3
```

pr9k finds the next open issue labeled `ralph` (lowest number first), implements the feature, writes tests, runs a code review, fixes review findings, closes the issue, updates docs, and pushes — then repeats for the next issue. Without `-n`, it keeps going until no more matching issues remain.

For a guided first run, see [Getting Started](docs/how-to/getting-started.md). For the full keyboard map and a tour of the screen, see [Reading the TUI](docs/how-to/reading-the-tui.md). If a step fails mid-run, see [Recovering from Step Failures](docs/how-to/recovering-from-step-failures.md).

<img src="images/ralph-tui-screenshot.png">

## User Guides

If you are using pr9k against your own repo, start here.

- **[How-To Guides](docs/how-to/README.md)** — the canonical index of every user-facing how-to, grouped by task: Getting Started → Operating a Run → Customizing Your Workflow → Advanced Step Configuration → Debugging.

The first three pages most readers need:

- [Getting Started](docs/how-to/getting-started.md) — install, the one-time GitHub setup, `.gitignore`, first run
- [Setting Up the Docker Sandbox](docs/how-to/setting-up-docker-sandbox.md) — install Docker, pull the sandbox image, authenticate the Claude profile
- [Reading the TUI](docs/how-to/reading-the-tui.md) — screen regions, keyboard map, and the chrome rhythm

## Contributor Reference

If you are modifying pr9k itself, start here.

- [Architecture Overview](docs/architecture.md) — system-level architecture with block diagrams, data flow, and package dependencies
- **Feature Documentation** ([`docs/features/`](docs/features/)) — user-facing behavior and cross-package integration
- **Code Package Documentation** ([`docs/code-packages/`](docs/code-packages/)) — per-Go-package API references
- [Coding Standards](docs/coding-standards/) — Go error handling, testing, concurrency, API design, and Go-specific patterns
- [Architectural Decision Records](docs/adr/) — historical decisions including the narrow-reading principle and the cobra CLI choice
- [Plans](docs/plans/) — original specifications and design documents

## Copyright & License

Copyright River Bailey. Licensed under the Apache License 2.0. See [LICENSE](./LICENSE) for details.
