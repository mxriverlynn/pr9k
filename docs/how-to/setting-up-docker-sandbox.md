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

Run `sandbox create` from your pr9k install:

```bash
/path/to/pr9k/bin/ralph-tui sandbox create
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
/path/to/pr9k/bin/ralph-tui sandbox create --force
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

ralph-tui bind-mounts your Claude profile directory (`$CLAUDE_CONFIG_DIR` if set, otherwise `~/.claude`) into every sandbox container. The claude CLI inside the container uses that mount for authentication — so if your profile is not authenticated, every claude step will fail.

The sandbox profile lives on disk at `<profileDir>/.credentials.json`. On macOS the host `claude` CLI stores its OAuth token in the Keychain rather than on disk, so you cannot simply `claude login` on the host and expect the sandbox to pick it up — you need to authenticate **inside** the sandbox so the OAuth flow writes `.credentials.json` to the bind-mounted profile directory.

### Preferred: `ralph-tui sandbox login`

```bash
/path/to/pr9k/bin/ralph-tui sandbox login
```

This launches a one-shot interactive container with `claude` running. Inside the REPL, type `/login` and complete the OAuth flow in your browser. When you exit, `.credentials.json` exists in your profile directory and every subsequent ralph-tui run picks it up.

If the sandbox image hasn't been pulled yet, `sandbox login` auto-pulls it and prints a note:

```
Sandbox image not found; pulling it first — run 'ralph-tui sandbox create' next time to separate this step.
```

The profile directory is created automatically if it doesn't exist (mode `0700`).

### Debugging fallback: manual `docker run`

If something about `sandbox login` isn't working and you want to isolate the problem, you can launch the same container by hand:

```bash
docker run -it --rm --init \
  -u $(id -u):$(id -g) \
  -v ~/.claude:/home/agent/.claude \
  -e CLAUDE_CONFIG_DIR=/home/agent/.claude \
  docker/sandbox-templates:claude-code \
  claude
```

Type `/login` inside the REPL; type `/exit` when done. This is the same argv `sandbox login` builds via `sandbox.BuildLoginArgs`.

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

### Warning: credentials file is empty or missing

ralph-tui's startup preflight inspects `<profileDir>/.credentials.json` and emits a warning if it's not usable. The warning text differs based on cause:

**Empty file:**

```
Warning: /home/you/.claude/.credentials.json is empty. Claude will likely fail authentication.
Re-authenticate with 'claude login' inside the sandbox.
```

An empty credentials file is typically caused by a SIGKILL mid-OAuth-refresh — the file was truncated before the new token could be written. Re-run `ralph-tui sandbox login` to refresh it.

**Missing file:**

```
Warning: /home/you/.claude/.credentials.json does not exist. The sandboxed claude has no credentials
to authenticate with. Run 'ralph-tui sandbox login' to authenticate, or set ANTHROPIC_API_KEY in the
host environment.
```

A missing credentials file usually means the profile directory is new and you haven't authenticated yet, or `CLAUDE_CONFIG_DIR` points at a different profile directory than the one that was authenticated. Run `ralph-tui sandbox login` to populate it.

**Alternative:** setting `ANTHROPIC_API_KEY` in the host shell satisfies the sandbox without a credentials file — `BuiltinEnvAllowlist` passes that variable into every claude container, so API-key auth works with no on-disk credentials. When `ANTHROPIC_API_KEY` is set, both warnings above are suppressed.

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

### "Claude sandbox image is missing. Run: ralph-tui sandbox create"

The image was not pulled yet, or was deleted from the local image store. Run:

```bash
/path/to/pr9k/bin/ralph-tui sandbox create
```

### "Claude profile directory not found: /home/you/.claude"

The profile directory does not exist yet. Run `ralph-tui sandbox login` to create it and authenticate, or set `CLAUDE_CONFIG_DIR` to an existing profile.

### "Docker is installed but the daemon isn't running"

Start Docker Desktop, or on Linux:

```bash
systemctl start docker
```

### Claude step fails with authentication error inside the sandbox

The bind-mounted credentials may be invalid. Re-authenticate:

```bash
/path/to/pr9k/bin/ralph-tui sandbox login
```

Then retry the failed step (press `r` in ralph-tui's error mode).

### Files written by claude are owned by root

This happens when the `-u` flag is not taking effect (e.g., an older Docker image or a non-standard UID setup). Re-pull the image:

```bash
/path/to/pr9k/bin/ralph-tui sandbox create --force
```

## Related Documentation

- [Getting Started](getting-started.md) — First-run walkthrough and TUI orientation
- [Docker Sandbox Feature Doc](../features/docker-sandbox.md) — Architecture, mount layout, env allowlist, and residual risks
- [sandbox Subcommand Feature Doc](../features/sandbox-subcommand.md) — Implementation details of the `sandbox create` and `sandbox login` subcommands
- [Preflight Feature Doc](../features/preflight.md) — Startup checks that enforce sandbox readiness
- [ADR: Require Docker Sandbox](../adr/20260413160000-require-docker-sandbox.md) — Decision rationale for making Docker a runtime requirement
- [Recovering from Step Failures](recovering-from-step-failures.md) — Retry/continue decisions when a step fails inside the sandbox
