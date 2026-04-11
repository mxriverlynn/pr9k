# Getting Started

This guide walks you through installing ralph-tui, pointing it at a target repo, and interpreting your first run. If you want to adapt ralph-tui for a different workflow entirely, start here — then head to [Building Custom Workflows](building-custom-workflows.md).

## Prerequisites

- **[Go 1.26.2](https://go.dev/dl/)** — ralph-tui compiles to a single static binary
- **[GitHub CLI (`gh`)](https://cli.github.com/)** — authenticated against the repo you want to automate (`gh auth status`)
- **[Claude CLI (`claude`)](https://docs.anthropic.com/en/docs/claude-cli)** — installed and authenticated (`claude --version`)
- A **target repo** — a git working copy with at least one open GitHub issue labeled `ralph` assigned to your user (for the default workflow), or your own custom `ralph-steps.json`
- A Unix-like terminal — ralph-tui uses `ioctl TIOCGWINSZ` for terminal sizing, so it runs on macOS and Linux but not Windows

## Installing

Clone this repo and build:

```bash
git clone https://github.com/mxriverlynn/pr9k.git
cd pr9k
make build
```

`make build` produces:

- `bin/ralph-tui` — the orchestrator binary
- `bin/ralph-steps.json` — the default workflow config
- `bin/prompts/` — the default Claude prompt files
- `bin/scripts/` — helper scripts (`get_next_issue`, `get_gh_user`, `close_gh_issue`, ...)
- `bin/ralph-art.txt` — ASCII art shown at the first init step

`bin/` is self-contained — you can copy it elsewhere or symlink `bin/ralph-tui` into your `PATH`.

If you just want to rebuild the Go binary without copying assets, run `cd ralph-tui && go build -o ../ralph-tui ./cmd/ralph-tui`. Don't use `go run`: the orchestrator resolves its project directory from the executable path (`os.Executable()` + `filepath.EvalSymlinks`), and `go run` uses a temp dir that doesn't contain the prompts or scripts.

## First run against the default workflow

From the **target repo's working directory** (not pr9k's — ralph-tui runs subprocesses with the current working directory):

```bash
# Run until no more ralph-labeled issues remain:
/path/to/pr9k/bin/ralph-tui

# Or cap at 3 iterations for a dry run:
/path/to/pr9k/bin/ralph-tui -n 3
```

With `-n 0` (the default), ralph-tui runs until `scripts/get_next_issue` returns an empty string (no more open issues). With `-n N`, it caps the loop at N iterations regardless of remaining issues.

To check which version you are running without launching the workflow:

```bash
/path/to/pr9k/bin/ralph-tui --version
# ralph-tui version 0.1.0
```

`-v` is accepted as a short alias. See [Versioning](../coding-standards/versioning.md) for the repo's semver rules.

## Pointing at a different project

If your pr9k install lives somewhere other than the current directory's resolved binary path — for example, if you're testing a feature branch of ralph-tui itself — pass `-p` to override the project directory:

```bash
/path/to/pr9k/bin/ralph-tui -p /path/to/pr9k
```

The project directory is where ralph-tui looks for `ralph-steps.json`, `prompts/`, and `scripts/`. It is *not* the target repo — the target repo is always the current working directory when you launch ralph-tui.

## What the TUI shows on first run

The TUI has four regions stacked top to bottom, inside a dynamic rounded border that embeds the current run state (e.g., `╭── ralph-tui — Iteration 1/3 ──...──╮`):

1. **Checkbox grid** — one row per four steps (`[ ]` pending, `[▸]` active, `[✓]` done, `[✗]` failed, `[-]` skipped) at the very top of the view
2. **Iteration line** — directly below the grid: `"Initializing 1/2: Splash"`, `"Iteration 1/3 — Issue #42"`, or `"Finalizing 1/3: Deferred work"` depending on the current phase
3. **Log panel** — streams subprocess output in real time, interleaved with phase banners, per-step banners, and capture logs
4. **Footer** — shortcut bar for the current mode (`↑/k up  ↓/j down  n next step  q quit` in normal mode) on the left, with the `ralph-tui v<semver>` label pinned to the bottom-right

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
| Normal | `n` | Terminate the current subprocess (skip the step) |
| Normal | `q` | Enter quit-confirm |
| Error (step failed) | `c` | Accept the failure, advance to next step |
| Error (step failed) | `r` | Re-run the failed step |
| Error (step failed) | `q` | Enter quit-confirm |
| QuitConfirm | `y` | Confirm quit (footer flips to `Quitting...`) |
| QuitConfirm | `n` or `Esc` | Cancel quit, return to previous mode |

See [Recovering from Step Failures](recovering-from-step-failures.md) and [Quitting Gracefully](quitting-gracefully.md) for the full interaction model.

## Where to go next

- **Adapting the workflow for your project:** [Building Custom Workflows](building-custom-workflows.md)
- **Learning the variable substitution engine:** [Variable Output & Injection](variable-output-and-injection.md)
- **Capturing a step's output for later steps:** [Capturing Step Output](capturing-step-output.md)
- **Stopping the iteration loop dynamically:** [Breaking Out of the Loop](breaking-out-of-the-loop.md)
- **Reading the run's log file:** [Debugging a Run](debugging-a-run.md)
- **Understanding the architecture:** [Architecture Overview](../architecture.md)
