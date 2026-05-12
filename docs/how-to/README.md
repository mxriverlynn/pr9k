# pr9k How-To Guides

This is the canonical index of every user-facing how-to for pr9k. The pages are grouped in the order a new user would actually need them — start at the top and work down, or jump straight to the section that matches your task.

If you are looking for the project front door, return to [the main README](../../README.md).

If you are modifying pr9k itself (not using it against your own repo), see the [Contributor Reference](../../README.md#contributor-reference) section in the main README instead.

---

## 1. Getting Started

Install pr9k and run the bundled "Ralph" workflow against a real GitHub repo.

1. [Getting Started](getting-started.md) — prerequisites, install, the one-time GitHub setup (creating the `ralph` label and assigning issues), `.gitignore`, and your first run
2. [Setting Up the Docker Sandbox](setting-up-docker-sandbox.md) — install Docker, pull the sandbox image, authenticate the bundled Claude profile, and verify the smoke test passes
3. [Reading the TUI](reading-the-tui.md) — the four screen regions (status header, log panel, footer, optional status line), the keyboard map, and the chrome rhythm pr9k writes into the log

## 2. Operating a Run

What to do while pr9k is running.

4. [Recovering from Step Failures](recovering-from-step-failures.md) — the `c`/`r`/`q` decision when a step fails
5. [Quitting Gracefully](quitting-gracefully.md) — the `q` → `y` confirmation flow, SIGINT/SIGTERM, and exit codes
6. [Copying Log Text](copying-log-text.md) — mouse drag, keyboard Select mode, OSC 52 over SSH, and Linux `xclip`/`xsel`

## 3. Customizing Your Workflow

Write your own steps and prompts once the bundled workflow runs end-to-end.

7. [Building Custom Workflows](building-custom-workflows.md) — start from a minimal `config.json`, learn the step schema, and mix Claude and shell steps
8. [Workflow Variables](workflow-variables.md) — `{{VAR}}` token substitution, the built-in variable list, iteration vs persistent scope, and file-based handoffs between steps
9. [Capturing Step Output](capturing-step-output.md) — `captureAs` to bind a step's stdout to a variable for later steps
10. [Using the Workflow Builder](using-the-workflow-builder.md) — the interactive `pr9k workflow` TUI for editing `config.json` and companion files
11. [Configuring an External Editor for the Workflow Builder](configuring-external-editor-for-workflow-builder.md) — point `Ctrl+E` at `code`, `nvim`, `nano`, or your editor of choice

## 4. Advanced Step Configuration

Opt-in features you reach for when you outgrow the basics.

12. [Breaking Out of the Loop](breaking-out-of-the-loop.md) — `breakLoopIfEmpty` to exit the iteration loop dynamically when there is no more work
13. [Skipping Steps Conditionally](skipping-steps-conditionally.md) — `skipIfCaptureEmpty` to bypass a step when an earlier step produced no output
14. [Setting Step Timeouts](setting-step-timeouts.md) — `timeoutSeconds` and `onTimeout` to cap a step's wall-clock time and choose fail-vs-continue
15. [Passing Environment Variables](passing-environment-variables.md) — `env` to forward host variables (including `ANTHROPIC_API_KEY`) into the sandbox; `containerEnv` to inject literals
16. [Configuring a Status Line](configuring-a-status-line.md) — add a custom status line to the TUI footer
17. [Resuming Sessions](resuming-sessions.md) — `resumePrevious` for tightly-coupled Claude steps that share context (off by default)
18. [Setting Claude Effort](setting-claude-effort.md) — `effort` per step and `defaults.effort` workflow-wide, forwarded to the Claude CLI as `--effort`
19. [Configuring Workflow Defaults](configuring-defaults.md) — the top-level `defaults` block, override hierarchy, and the supported keys (`effort`, `model`)

## 5. Debugging

Reconstruct what happened after the fact.

20. [Debugging a Run](debugging-a-run.md) — read the log file, the iteration JSONL, the per-step JSONL artifacts, and reproduce a failure with `-n 1`

---

## When to use what

- **First time using pr9k?** Start at section 1.
- **Run is on screen and something happened?** Section 2.
- **Default workflow runs but you want your own?** Section 3.
- **You wrote a workflow and want to harden it?** Section 4.
- **Something went wrong?** Section 5 (and section 2 if it's still running).

For a higher-altitude view of how the pieces fit together, see the [architecture overview](../architecture.md).
