# Setting Up the Docker Sandbox

ralph-tui runs every Claude step inside an ephemeral Docker container to limit blast radius. Docker is required — there is no fallback to direct invocation. This guide covers installing Docker, pulling the sandbox image, authenticating the Claude profile, and troubleshooting the most common setup problems.

## Prerequisites

Before running `ralph-tui` you need:

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

Run the `create-sandbox` subcommand from your pr9k install:

```bash
/path/to/pr9k/bin/ralph-tui create-sandbox
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
Verifying sandbox... ✓ claude 2.1.101 under UID 501:20
Sandbox ready.
```

If the image is already present from a previous run, `create-sandbox` skips the pull:

```
Checking Docker... ✓
Image docker/sandbox-templates:claude-code already present; skipping pull (use --force to re-pull)
Verifying sandbox... ✓ claude 2.1.101 under UID 501:20
Sandbox ready.
```

### Forcing a re-pull

To update the image to the latest upstream tag:

```bash
/path/to/pr9k/bin/ralph-tui create-sandbox --force
```

`--force` skips the "already present" check and runs a fresh `docker pull`.

### create-sandbox failure cases

| Message | Cause | Fix |
|---------|-------|-----|
| `Docker is not installed...` | `docker` not on `PATH` | Install Docker per Step 1 |
| `Docker is installed but the daemon isn't running...` | Daemon stopped | Start Docker Desktop / `systemctl start docker` |
| `Failed to pull sandbox image.` | Network error or auth issue | Check internet access; `docker login` if using a private registry |
| `Sandbox smoke test failed — container exited with status N.` | Image broken or incompatible UID | Re-pull with `--force`; check `docker run` permissions |
| `Sandbox smoke test failed — image ran but produced no version output.` | Tag squatted or local stub image | Re-pull with `--force`; verify image digest |

## Step 3 — Authenticate the Claude profile

ralph-tui bind-mounts your Claude profile directory (`$CLAUDE_CONFIG_DIR` if set, otherwise `~/.claude`) into every sandbox container. The claude CLI inside the container uses that mount for authentication — so if your profile is not authenticated, every claude step will fail.

Authenticate your default profile with:

```bash
claude login
```

This stores OAuth credentials in `~/.claude/.credentials.json`. The sandbox container reads this file from the bind-mount on every step.

To verify authentication works inside the sandbox:

```bash
docker run --rm \
  -u $(id -u):$(id -g) \
  -v ~/.claude:/home/agent/.claude \
  -e CLAUDE_CONFIG_DIR=/home/agent/.claude \
  docker/sandbox-templates:claude-code \
  claude --version
```

If this prints a version number without an authentication error, the profile is ready.

### Warning: empty credentials file

If ralph-tui's startup preflight prints:

```
Warning: /home/you/.claude/.credentials.json is empty. Claude will likely fail authentication.
Re-authenticate with 'claude login'.
```

Your credentials file was likely corrupted by a SIGKILL mid-OAuth-refresh. Re-run `claude login` to refresh it.

## Step 4 — Configure `CLAUDE_CONFIG_DIR` (optional, for multiple profiles)

If you need to run ralph-tui with a specific Claude profile — for example, to separate work and personal accounts — set `CLAUDE_CONFIG_DIR` to the profile directory before running ralph-tui:

```bash
CLAUDE_CONFIG_DIR=/path/to/profile /path/to/pr9k/bin/ralph-tui
```

ralph-tui resolves the profile directory at startup from `$CLAUDE_CONFIG_DIR` if set, then falls back to `$HOME/.claude`. The resolved path is bind-mounted into every container at `/home/agent/.claude`.

To persist the variable for all ralph-tui invocations, add it to your shell profile (`~/.zshrc`, `~/.bashrc`, etc.):

```bash
export CLAUDE_CONFIG_DIR=/path/to/profile
```

## Step 5 — First run

From your target repo's working directory:

```bash
/path/to/pr9k/bin/ralph-tui -n 1
```

The `-n 1` flag caps the run at one iteration, which is useful for verifying the sandbox works end-to-end before a longer unattended run.

On the first run, ralph-tui's startup preflight checks:

1. `ralph-steps.json` parses and validates
2. Claude profile directory exists
3. Docker is installed and the daemon is running
4. Sandbox image is present locally

If any check fails, ralph-tui prints a structured error and exits before the TUI starts. Fix the reported issue and re-run.

## Troubleshooting

### "Claude sandbox image is missing. Run: ralph-tui create-sandbox"

The image was not pulled yet, or was deleted from the local image store. Run:

```bash
/path/to/pr9k/bin/ralph-tui create-sandbox
```

### "Claude profile directory not found: /home/you/.claude"

The profile directory does not exist yet. Run `claude login` to create and populate it, or set `CLAUDE_CONFIG_DIR` to an existing profile.

### "Docker is installed but the daemon isn't running"

Start Docker Desktop, or on Linux:

```bash
systemctl start docker
```

### Claude step fails with authentication error inside the sandbox

The bind-mounted credentials may be invalid. Re-authenticate:

```bash
claude login
```

Then retry the failed step (press `r` in ralph-tui's error mode).

### Files written by claude are owned by root

This happens when the `-u` flag is not taking effect (e.g., an older Docker image or a non-standard UID setup). Re-pull the image:

```bash
/path/to/pr9k/bin/ralph-tui create-sandbox --force
```

## Related Documentation

- [Getting Started](getting-started.md) — First-run walkthrough and TUI orientation
- [Docker Sandbox Feature Doc](../features/docker-sandbox.md) — Architecture, mount layout, env allowlist, and residual risks
- [Create Sandbox Feature Doc](../features/create-sandbox.md) — Implementation details of the `create-sandbox` subcommand
- [Preflight Feature Doc](../features/preflight.md) — Startup checks that enforce sandbox readiness
- [ADR: Require Docker Sandbox](../adr/20260413160000-require-docker-sandbox.md) — Decision rationale for making Docker a runtime requirement
- [Recovering from Step Failures](recovering-from-step-failures.md) — Retry/continue decisions when a step fails inside the sandbox
