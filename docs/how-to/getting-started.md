# Getting Started

This guide walks you through installing pr9k, pointing it at a target repo, and interpreting your first run. If you want to adapt pr9k for a different workflow entirely, start here — then head to [Building Custom Workflows](building-custom-workflows.md).

## Prerequisites

- **[Go 1.26.2](https://go.dev/dl/)** — pr9k compiles to a single static binary
- **[Docker](https://docs.docker.com/get-docker/)** — Docker Desktop (macOS/Windows) or Docker Engine (Linux), running. pr9k runs every Claude step inside a Docker sandbox; Docker is a **required** runtime dependency, not optional
- **[GitHub CLI (`gh`)](https://cli.github.com/)** — authenticated against the repo you want to automate (`gh auth status`)
- **[Claude CLI (`claude`)](https://docs.anthropic.com/en/docs/claude-cli)** — installed and authenticated (`claude --version`). The CLI's credentials are used inside the sandbox container
- A **target repo** — a git working copy with at least one open GitHub issue labeled `ralph` assigned to your user (for the default workflow), or your own custom `config.json`
- A Unix-like terminal — pr9k uses `ioctl TIOCGWINSZ` for terminal sizing, so it runs on macOS and Linux but not Windows

## Installing

Clone this repo and build:

```bash
git clone https://github.com/mxriverlynn/pr9k.git
cd src
make build
```

`make build` produces:

- `bin/pr9k` — the orchestrator binary
- `bin/config.json` — the default workflow config
- `bin/prompts/` — the default Claude prompt files
- `bin/scripts/` — helper scripts (`get_next_issue`, `get_gh_user`, `close_gh_issue`, ...)
- `bin/ralph-art.txt` — ASCII art shown at the first init step

`bin/` is self-contained — you can copy it elsewhere or symlink `bin/pr9k` into your `PATH`.

If you just want to rebuild the Go binary without copying assets, run `cd src && go build -o ../bin/pr9k ./cmd/pr9k`. Don't use `go run`: the orchestrator resolves its project directory from the executable path (`os.Executable()` + `filepath.EvalSymlinks`), and `go run` uses a temp dir that doesn't contain the prompts or scripts.

## First run against the default workflow

Before the first run, add `logs/` to your target repo's `.gitignore`. Since pr9k 0.2.3, log files land under `<project-dir>/logs/` — that is, inside your target repo — and will appear as untracked changes if the directory is not ignored:

```bash
echo 'logs/' >> .gitignore
git add .gitignore && git commit -m "ignore pr9k log directory"
```

From the **target repo's working directory** (not pr9k's — pr9k runs subprocesses with the current working directory):

```bash
# Run until no more ralph-labeled issues remain:
/path/to/pr9k/bin/pr9k

# Or cap at 3 iterations for a dry run:
/path/to/pr9k/bin/pr9k -n 3
```

With `-n 0` (the default), pr9k runs until `scripts/get_next_issue` returns an empty string (no more open issues). With `-n N`, it caps the loop at N iterations regardless of remaining issues.

To check which version you are running without launching the workflow:

```bash
/path/to/pr9k/bin/pr9k --version
# pr9k version 0.4.1
```

`-v` is accepted as a short alias. See [Versioning](../coding-standards/versioning.md) for the repo's semver rules.

## Pointing at a different workflow bundle

If your pr9k install lives somewhere other than the current directory's resolved binary path — for example, if you're testing a feature branch of pr9k itself — pass `--workflow-dir` to override the workflow directory:

```bash
/path/to/pr9k/bin/pr9k --workflow-dir /path/to/pr9k/bin
```

The workflow directory is where pr9k looks for `config.json`, `prompts/`, and `scripts/`. It is *not* the target repo — the target repo is the current working directory when you launch pr9k (or can be overridden with `--project-dir`).

## What the TUI shows on first run

The TUI is a hand-built rounded frame with three inner regions. The current run state (phase, iteration number, issue ID, active step) is embedded directly into the top border as the window title, which renders with the app name `Power-Ralph.9000` in green and the iteration detail after the ` — ` in white (e.g., `╭── Power-Ralph.9000 — Iteration 1/3 — Issue #42 ──...──╮`):

1. **Checkbox grid** — one row per four steps (`[ ]` pending, `[▸]` active, `[✓]` done, `[✗]` failed, `[-]` skipped) immediately below the top border
2. **Log panel** — streams subprocess output in real time (rendered in white), interleaved with phase banners, per-step banners, and capture logs; supports keyboard arrow/vim keys and mouse-wheel/trackpad scrolling (to drag-select text, hold Option on macOS or Shift on Linux/Windows — see [Reading the TUI](reading-the-tui.md#selecting-log-text-to-copy))
3. **Footer** — shortcut bar for the current mode (`↑/k up  ↓/j down  n next step  q quit` in normal mode) on the left, with the `pr9k v<semver>` label pinned to the bottom-right. Mapped key tokens and the version label render in white; descriptions render in light gray

The two horizontal rules separating these regions use T-junction glyphs (`├`, `┤`) so they visually connect to the `│` side borders.

Every started step writes a banner into the log panel before its subprocess output:

```
Starting step: Feature work
───────────────────────────

[feature-work step output starts here]
```

At each phase transition you'll see a full-width underline:

```
Iterations
════════════════════════════════════════
```

And after any step with `captureAs`, the captured value is logged as `Captured VAR = "value"`. For the default workflow, that includes `GITHUB_USER`, `ISSUE_ID`, and `STARTING_SHA`.

For a detailed walk-through of the TUI layout and what each region means, see [Reading the TUI](reading-the-tui.md).

## Keyboard controls at a glance

| Mode | Keys | Effect |
|------|------|--------|
| Normal | `n` | Enter skip-confirm (`Skip current step? y/n, esc to cancel`) |
| Normal | `q` | Enter quit-confirm |
| NextConfirm | `y` | Confirm skip — terminate the current subprocess |
| NextConfirm | `n` or `Esc` | Cancel skip, return to normal mode |
| Error (step failed) | `c` | Accept the failure, advance to next step |
| Error (step failed) | `r` | Re-run the failed step |
| Error (step failed) | `q` | Enter quit-confirm |
| QuitConfirm | `y` | Confirm quit (footer flips to `Quitting...`) |
| QuitConfirm | `n` or `Esc` | Cancel quit, return to previous mode |
| Done (workflow complete) | `q` | Enter quit-confirm to exit |

See [Recovering from Step Failures](recovering-from-step-failures.md) and [Quitting Gracefully](quitting-gracefully.md) for the full interaction model.

## Where to go next

- **Setting up the Docker sandbox (first-time):** [Setting Up Docker Sandbox](setting-up-docker-sandbox.md)
- **Adapting the workflow for your project:** [Building Custom Workflows](building-custom-workflows.md)
- **Learning the variable substitution engine:** [Variable Output & Injection](variable-output-and-injection.md)
- **Capturing a step's output for later steps:** [Capturing Step Output](capturing-step-output.md)
- **Forwarding host env vars to the sandbox:** [Passing Environment Variables](passing-environment-variables.md)
- **Stopping the iteration loop dynamically:** [Breaking Out of the Loop](breaking-out-of-the-loop.md)
- **Reading the run's log file:** [Debugging a Run](debugging-a-run.md)
- **Understanding the architecture:** [Architecture Overview](../architecture.md)
