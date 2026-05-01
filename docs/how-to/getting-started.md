# Getting Started

This guide walks you through installing pr9k, doing the one-time setup, and getting a successful first run on a real GitHub repo using the bundled "Ralph" workflow. Plan on roughly 15 minutes from clone to first iteration, plus however long Docker takes to pull the sandbox image the first time.

← [Back to How-To Guides](README.md)

## Prerequisites

You will need:

- **[Go 1.26.2](https://go.dev/dl/)** — pr9k compiles to a single binary
- **[Docker](https://docs.docker.com/get-docker/)** — Docker Desktop on macOS/Windows, Docker Engine on Linux. The daemon must be running while pr9k is running. Every Claude step runs inside a sandbox container; this is a hard requirement, not optional. Setup is covered in [Setting Up the Docker Sandbox](setting-up-docker-sandbox.md)
- **[GitHub CLI (`gh`)](https://cli.github.com/)** — authenticated against the repo you want to automate. Verify with `gh auth status`. Note: pr9k uses whichever account `gh` is logged in as, so make sure that account has push access to your target repo
- **[Claude CLI (`claude`)](https://docs.anthropic.com/en/docs/claude-cli)** — installed and authenticated (`claude --version`). Its credentials are reused inside the sandbox container
- **[`jq`](https://jqlang.github.io/jq/)** — used by the bundled `post_issue_summary` script and by many of the [debugging](debugging-a-run.md) examples
- **A target repo** — a git working copy with at least one open GitHub issue labeled `ralph` and assigned to you (the one-time setup is below)
- **A Unix-like terminal** — pr9k uses POSIX terminal sizing, so it runs on macOS and Linux but not Windows

## Step 1 — Install pr9k

Clone the repo and build:

```bash
git clone https://github.com/mxriverlynn/pr9k.git
cd pr9k
make build
```

`make build` produces a self-contained `bin/` directory with this layout:

```
bin/
├── pr9k                          # the orchestrator binary
└── .pr9k/
    └── workflow/
        ├── config.json           # bundled workflow definition
        ├── ralph-art.txt         # ASCII banner shown by the splash step
        ├── prompts/              # bundled Claude prompt files
        └── scripts/              # bundled helpers (get_next_issue, get_gh_user, …)
```

You can copy `bin/` anywhere or symlink `bin/pr9k` into your `$PATH`. Whichever path you pick, the binary always finds its bundled workflow next to itself.

If you only want to rebuild the Go binary without copying assets, run `cd src && go build -o ../bin/pr9k ./cmd/pr9k`. Don't use `go run` — pr9k locates its bundled workflow via the executable path, and `go run` puts the binary in a temp directory that doesn't have the prompts or scripts.

To check the version once the build is in place:

```bash
/path/to/pr9k/bin/pr9k --version
# pr9k version 0.7.5
```

`-v` is accepted as a short alias for `--version`.

## Step 2 — Set up the Docker sandbox

Every Claude step runs inside a Docker container, so before the first run you need to pull the sandbox image and authenticate the bundled Claude profile. The full walkthrough is in [Setting Up the Docker Sandbox](setting-up-docker-sandbox.md). The short version:

```bash
/path/to/pr9k/bin/pr9k sandbox create        # pull the image, run the smoke test
/path/to/pr9k/bin/pr9k sandbox --interactive # opens a Claude REPL inside the sandbox so you can /login once
```

When the smoke test prints `Sandbox verified: claude <version> under UID <uid>:<gid>.` and the interactive `/login` flow completes successfully, you're ready.

## Step 3 — Set up your target repo

In the GitHub repo you want pr9k to work on:

1. **Create the `ralph` label.** From a checkout of the repo (or via the GitHub UI):

    ```bash
    gh label create ralph --description "Picked up by pr9k" --color 8B5CF6
    ```

2. **Open one or more issues** that describe small, well-scoped pieces of work. Each issue body is the prompt context Claude will see, so write the issue the way you would write a ticket for a teammate.

3. **Apply the `ralph` label and assign each issue to yourself.** pr9k's `get_next_issue` script picks the lowest-numbered open issue that is both labeled `ralph` *and* assigned to the current `gh` user, so both conditions must hold.

4. **Ignore pr9k's runtime state in `.gitignore`.** pr9k writes its logs, iteration log, and per-step JSONL artifacts under `<target-repo>/.pr9k/`. Without these entries, every run leaves untracked files in your tree:

    ```
    # pr9k temp files and logs
    .pr9k/logs/
    .pr9k/iteration.jsonl
    .pr9k/artifacts/
    ```

    Do **not** ignore the entire `.pr9k/` folder — `.pr9k/workflow/` is a tracked source directory when you commit a per-repo workflow override (see [Building Custom Workflows](building-custom-workflows.md)).

## Step 4 — First run

From the **target repo's working directory** (not pr9k's — pr9k runs subprocesses with the current working directory):

```bash
# Run until no more ralph-labeled issues remain (until-done mode):
/path/to/pr9k/bin/pr9k

# Or cap at 3 iterations for a dry run:
/path/to/pr9k/bin/pr9k -n 3
```

Without `-n` (or with `-n 0`), pr9k keeps picking up issues until `get_next_issue` returns nothing. With `-n N`, it caps the loop at N iterations regardless of remaining issues.

### What a successful run looks like

The first thing the TUI shows is the splash step (the `Power-Ralph.9000` ASCII banner), then it captures the GitHub username and starting commit SHA. From there it loops once per issue:

1. **Get next issue** — picks the lowest-numbered open `ralph`-labeled issue assigned to you and binds it to `ISSUE_ID`
2. **Feature work** — Claude (sonnet by default) reads the issue body and implements the change
3. **Test planning** (opus) → **Test writing** (sonnet) — drafts and writes tests
4. **Summarize to issue** — posts a comment summarizing what changed
5. **Close issue** — `gh issue close`
6. **Git push** — pushes the branch upstream

After every iteration, finalization runs once:

7. **Code review** (opus) → **Fix review items** (sonnet, skipped if the reviewer found nothing)
8. **Update docs** (sonnet)
9. **Deferred work** — files any new issues from `deferred.txt`
10. **Lessons learned** — codifies entries from `progress.txt`
11. **Final git push**

A typical iteration takes 5–15 minutes depending on issue size and which model is in use. If you see no output for more than ~30 seconds during a Claude step, the TUI's iteration title appends `⋯ thinking (Ns)` so you know pr9k is still waiting on a stream event — it isn't hung.

When the workflow finishes you'll see a completion summary like:

```
total claude spend across 4 step invocations (including 1 retry): 42 turns · 18432/6144 tokens (cache: 512/2048) · $0.0420000 · 3m22s

Ralph completed after 2 iteration(s) and 2 finalizing tasks.
```

The TUI does **not** auto-exit when the workflow finishes — press `q` then `y` to exit so you have time to review the final output.

### What if there are no `ralph` issues open?

That's fine. `get_next_issue` returns an empty string, the iteration step's `breakLoopIfEmpty` triggers, the rest of the iteration phase is skipped, and pr9k drops straight into finalization. You'll see `Captured ISSUE_ID = ""` in the log, the iteration grid will mark its remaining steps as skipped (`[-]`), and the run will end normally. Add an issue, give it the `ralph` label, and run again.

### What does it cost?

Every Claude step prints a per-step `$cost` line in the log, and the run ends with a `total claude spend` line summing them. There is no enforced spending cap; if cost matters, run with `-n 1` first to measure your workflow's per-iteration spend before turning it loose unattended. Sonnet is materially cheaper than Opus for the same wall-clock work.

## Cross-cutting flags

A few flags you'll occasionally need:

| Flag | What it does |
|------|--------------|
| `-n` / `--iterations` | Cap the iteration loop at N. `0` (the default) means run until `get_next_issue` is empty. |
| `--workflow-dir` | Point pr9k at a workflow bundle other than the one shipped with the binary. Most users never touch this. |
| `--project-dir` | Run pr9k against a target repo other than the current directory. Most users never touch this — they just `cd` into the repo first. |
| `--version` / `-v` | Print the version and exit. Short for `--version`; not "verbose". |

pr9k resolves the workflow bundle in two steps: first it looks for `<target-repo>/.pr9k/workflow/` (per-repo override — useful when one repo needs a custom workflow); if that's missing it falls back to `<binary-dir>/.pr9k/workflow/` (the bundle from `make build`). To author a custom workflow, see [Building Custom Workflows](building-custom-workflows.md).

## Where to go next

**Recommended next:**

- [Reading the TUI](reading-the-tui.md) — full tour of the screen, keyboard map, status line, and the chrome rhythm in the log

**While a run is on screen:**

- [Recovering from Step Failures](recovering-from-step-failures.md) — what to do when a step exits non-zero
- [Quitting Gracefully](quitting-gracefully.md) — `q` → `y`, SIGINT, exit codes
- [Copying Log Text](copying-log-text.md) — mouse drag and keyboard Select mode

**Customize your workflow:**

- [Building Custom Workflows](building-custom-workflows.md) — write your own `config.json` and prompt files
- [Workflow Variables](workflow-variables.md) — `{{VAR}}` substitution and file-based handoffs
- [Capturing Step Output](capturing-step-output.md) — bind a step's stdout to a variable

**When something goes wrong:**

- [Debugging a Run](debugging-a-run.md) — log file, iteration JSONL, per-step JSONL artifacts, and reproducing a failure with `-n 1`
