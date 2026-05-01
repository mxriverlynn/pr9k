# Setting Up the Docker Sandbox

← [Back to How-To Guides](README.md)

pr9k runs every Claude step inside an ephemeral Docker container to limit blast radius. Docker is required — there is no fallback to direct invocation. This guide covers installing Docker, pulling the sandbox image, authenticating the Claude profile, and troubleshooting the most common setup problems.

You only do this once per machine. After the first successful smoke test and `/login`, subsequent runs reuse the cached image and credentials.

## Prerequisites

Before running `pr9k` you need:

1. **Docker Desktop** (macOS/Windows) or **Docker Engine** (Linux) installed and running
2. A Claude profile authenticated with your Anthropic account

## Step 1 — Install Docker

### macOS

Install Docker Desktop from [docs.docker.com/desktop/mac](https://docs.docker.com/desktop/mac/install/) or via Homebrew:

```bash
brew install --cask docker
```

After installing, start Docker Desktop from your Applications folder. Wait for the whale icon in the menu bar to stop animating (daemon is ready).

Verify Docker is reachable:

```bash
docker version
```

### Linux

Install Docker Engine following the official guide for your distribution:
[docs.docker.com/engine/install](https://docs.docker.com/engine/install/)

After installing, add your user to the `docker` group so you can run `docker` without `sudo`:

```bash
sudo usermod -aG docker $USER
# Log out and back in for the group change to take effect
```

Verify:

```bash
docker version
```

## Step 2 — Pull and verify the sandbox image

Run `sandbox create` from your pr9k install:

```bash
/path/to/pr9k/bin/pr9k sandbox create
```

This command:

1. Checks that Docker is installed and the daemon is running
2. Pulls `docker/sandbox-templates:claude-code` (streams pull progress)
3. Runs a smoke test (`claude --version` inside the container under your UID/GID) to confirm the image works for your user
4. Prints `Sandbox ready.` on success

Expected output:

```
Checking Docker... ✓
Pulling docker/sandbox-templates:claude-code...
[pull progress output]
Sandbox verified: claude 2.1.101 under UID 501:20.
Sandbox ready.
```

If the image is already present from a previous run, `sandbox create` skips the pull:

```
Checking Docker... ✓
Image docker/sandbox-templates:claude-code already present; skipping pull (use --force to re-pull).
Sandbox verified: claude 2.1.101 under UID 501:20.
Sandbox ready.
```

### Forcing a re-pull

To update the image to the latest upstream tag:

```bash
/path/to/pr9k/bin/pr9k sandbox create --force
```

`--force` skips the "already present" check and runs a fresh `docker pull`.

### `sandbox create` failure cases

| Message | Cause | Fix |
|---------|-------|-----|
| `Docker is not installed...` | `docker` not on `PATH` | Install Docker per Step 1 |
| `Docker is installed but the daemon isn't running...` | Daemon stopped | Start Docker Desktop / `systemctl start docker` |
| `Failed to pull sandbox image.` | Network error or auth issue | Check internet access; `docker login` if using a private registry |
| `Sandbox smoke test failed — container exited with status N.` | Image broken or incompatible UID | Re-pull with `--force`; check `docker run` permissions |
| `Sandbox smoke test failed — image ran but produced no version output.` | Tag squatted or local stub image | Re-pull with `--force`; verify image digest |

## Step 3 — Authenticate the Claude profile

pr9k bind-mounts your Claude profile directory (`$CLAUDE_CONFIG_DIR` if set, otherwise `~/.claude`) into every sandbox container. The claude CLI inside the container uses that mount for authentication — so if your profile is not authenticated, every claude step will fail.

The sandbox profile lives on disk at `<profileDir>/.credentials.json`. On macOS the host `claude` CLI stores its OAuth token in the Keychain rather than on disk, so you cannot simply `claude login` on the host and expect the sandbox to pick it up — you need to authenticate **inside** the sandbox so the OAuth flow writes `.credentials.json` to the bind-mounted profile directory.

### Preferred: `pr9k sandbox --interactive`

```bash
/path/to/pr9k/bin/pr9k sandbox --interactive
```

This launches a one-shot interactive container with `claude` running. Inside the REPL, type `/login` and complete the OAuth flow in your browser. When you exit, `.credentials.json` exists in your profile directory and every subsequent pr9k run picks it up.

If the sandbox image hasn't been pulled yet, `sandbox --interactive` auto-pulls it and prints a note:

```
Sandbox image not found; pulling it first — run 'pr9k sandbox create' next time to separate this step.
```

The profile directory is created automatically if it doesn't exist (mode `0700`).

### Debugging fallback: manual `docker run`

If something about `sandbox --interactive` isn't working and you want to isolate the problem, you can launch the same container by hand:

```bash
docker run -it --rm --init \
  -u $(id -u):$(id -g) \
  -v ~/.claude:/home/agent/.claude \
  -e CLAUDE_CONFIG_DIR=/home/agent/.claude \
  -e TERM \
  docker/sandbox-templates:claude-code \
  claude
```

Type `/login` inside the REPL; type `/exit` when done. This is the same argv `sandbox --interactive` builds via `sandbox.BuildInteractiveArgs` — `-e TERM` is forwarded bare so the container pty inherits the host's `TERM` value and the REPL can read bracketed-paste sequences from modern terminals (without it, pasting the OAuth code into the REPL may silently fail).

### Verifying the profile is authenticated

```bash
docker run --rm \
  -u $(id -u):$(id -g) \
  -v ~/.claude:/home/agent/.claude \
  -e CLAUDE_CONFIG_DIR=/home/agent/.claude \
  docker/sandbox-templates:claude-code \
  claude --version
```

If this prints a version number without an authentication error, the profile is ready.

### Two ways to authenticate claude steps

pr9k does not check or warn about your credentials file at startup. Authentication is your call, and a missing or invalid credentials file surfaces at runtime when the in-container claude binary refuses to authenticate. You can authenticate via either of:

1. **OAuth via `.credentials.json`** (recommended) — run `pr9k sandbox --interactive` and `/login` as above. The bundled workflow ships with a `Claude Credentials` step that refreshes the token from your macOS keychain on each iteration.
2. **API key via `ANTHROPIC_API_KEY`** — set the variable on the host **and** add `ANTHROPIC_API_KEY` to your workflow's `env` array. pr9k does not auto-forward this variable; opting in via `env` is required:

   ```json
   {
     "env": ["ANTHROPIC_API_KEY"],
     "iteration": [ ... ]
   }
   ```

   ```bash
   export ANTHROPIC_API_KEY=sk-ant-...
   /path/to/bin/pr9k
   ```

If neither is set up, claude steps will fail with an authentication error when they run, and the step error message will be visible in the TUI and the log file.

If `.credentials.json` exists but is zero bytes (typically caused by SIGKILL mid-OAuth-refresh), re-run `pr9k sandbox --interactive` to refresh it.

## Step 4 — Configure `CLAUDE_CONFIG_DIR` (optional, for multiple profiles)

If you need to run pr9k with a specific Claude profile — for example, to separate work and personal accounts — set `CLAUDE_CONFIG_DIR` to the profile directory before running pr9k:

```bash
CLAUDE_CONFIG_DIR=/path/to/profile /path/to/pr9k/bin/pr9k
```

pr9k resolves the profile directory at startup from `$CLAUDE_CONFIG_DIR` if set, then falls back to `$HOME/.claude`. The resolved path is bind-mounted into every container at `/home/agent/.claude`.

To persist the variable for all pr9k invocations, add it to your shell profile (`~/.zshrc`, `~/.bashrc`, etc.):

```bash
export CLAUDE_CONFIG_DIR=/path/to/profile
```

## Step 5 — First run

From your target repo's working directory:

```bash
/path/to/pr9k/bin/pr9k -n 1
```

The `-n 1` flag caps the run at one iteration, which is useful for verifying the sandbox works end-to-end before a longer unattended run.

On the first run, pr9k's startup preflight checks:

1. `config.json` parses and validates
2. Claude profile directory exists
3. Docker is installed and the daemon is running
4. Sandbox image is present locally

If any check fails, pr9k prints a structured error and exits before the TUI starts. Fix the reported issue and re-run.

## Troubleshooting

### "Claude sandbox image is missing. Run: pr9k sandbox create"

The image was not pulled yet, or was deleted from the local image store. Run:

```bash
/path/to/pr9k/bin/pr9k sandbox create
```

### "Claude profile directory not found: /home/you/.claude"

The profile directory does not exist yet. Run `pr9k sandbox --interactive` to create it and authenticate, or set `CLAUDE_CONFIG_DIR` to an existing profile.

### "Docker is installed but the daemon isn't running"

Start Docker Desktop, or on Linux:

```bash
systemctl start docker
```

### Claude step fails with authentication error inside the sandbox

The bind-mounted credentials may be invalid. Re-authenticate:

```bash
/path/to/pr9k/bin/pr9k sandbox --interactive
```

Then retry the failed step (press `r` in pr9k's error mode).

### Files written by claude are owned by root

This happens when the `-u` flag is not taking effect (e.g., an older Docker image or a non-standard UID setup). Re-pull the image:

```bash
/path/to/pr9k/bin/pr9k sandbox create --force
```

### Opening a shell inside the sandbox

For ad-hoc debugging — running tooling, inspecting filesystem state, or executing one-off `claude` invocations against the same mounts the workflow runner uses — `pr9k sandbox shell` drops you into an interactive bash session inside the sandbox container:

```bash
/path/to/pr9k/bin/pr9k sandbox shell
```

The current working directory is bind-mounted at `/home/agent/workspace` and your Claude profile is bind-mounted at `/home/agent/.claude`. The container is removed (`docker run --rm`) when you `exit` the shell. If the sandbox image hasn't been pulled yet, `sandbox shell` auto-pulls it before launching, the same way `sandbox --interactive` does.

## Related Documentation

- ← [Back to How-To Guides](README.md)
- [Getting Started](getting-started.md) — first-run walkthrough and TUI orientation
- [Reading the TUI](reading-the-tui.md) — what to expect on screen once the sandbox is set up and you launch a real run
- [Passing Environment Variables](passing-environment-variables.md) — forward host env vars (API tokens, proxy settings) into the sandbox
- [Docker Sandbox Feature Doc](../features/docker-sandbox.md) — architecture, mount layout, env allowlist, and residual risks (contributor reference)
- [sandbox Subcommand Feature Doc](../features/sandbox-subcommand.md) — implementation of `sandbox create`, `sandbox --interactive`, and `sandbox shell`
- [Preflight Feature Doc](../code-packages/preflight.md) — startup checks that enforce sandbox readiness
- [ADR: Require Docker Sandbox](../adr/20260413160000-require-docker-sandbox.md) — decision rationale
- [Recovering from Step Failures](recovering-from-step-failures.md) — Retry/continue decisions when a step fails inside the sandbox
