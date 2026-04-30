# Caching Build Artifacts Across Iterations

← [Back to How-To Guides](README.md)

**Audience**: custom-workflow authors whose unattended runs hit `permission denied` cache errors during compile/test steps, or who want their Go/Node/Python/Rust caches to survive across iterations.

**Prerequisites**: a working install ([Getting Started](getting-started.md)) and familiarity with the `containerEnv` mechanism — read [Passing Environment Variables](passing-environment-variables.md) first if you haven't.

**Skip this if** you only run Claude steps and never build/test inside the sandbox; the bundled Go-project workflow already includes these settings, so users running pr9k against a Go repo with the bundled `config.json` get this behaviour automatically.

## The Problem

By default, each Claude step runs inside an ephemeral Docker container with no persistent build cache. On Go projects this produces a cascade of `permission denied` errors: the Go toolchain falls back to `$HOME/.cache/go-build` and `$HOME/go`. Because the container runs with the host user's UID via `-u` and that UID has no `/etc/passwd` entry inside the container, `$HOME` resolves to `/` (or is unset), and the toolchain hits a permissions wall.

The fix: redirect cache directories to a subdirectory of the bind-mounted project directory via `containerEnv`, so the cache persists across iterations and is always writable.

## How It Works

The `containerEnv` block in `config.json` sets environment variables inside the Docker container before any Claude step runs. Point cache env vars at `/home/agent/workspace/.ralph-cache/<subdir>` — inside the bind-mounted project directory — and the build toolchain will use paths that are both writable and persistent across iterations.

The parent `.ralph-cache/` directory is created by `preflight.Run` before the first step executes. Subdirectories (`go/`, `go-build/`, `gomod/`, `xdg/`) are created on first use by the respective toolchain.

Add `.ralph-cache/` to your target project's `.gitignore` so the cache is never committed.

## Language-to-Env-Var Matrix

| Language | Env vars |
| --- | --- |
| Go | `GOPATH`, `GOCACHE`, `GOMODCACHE`, `GOTMPDIR` |
| Node | `NPM_CONFIG_CACHE`, `YARN_CACHE_FOLDER`, `PNPM_STORE_PATH` |
| Python | `PIP_CACHE_DIR`, `POETRY_CACHE_DIR`, `UV_CACHE_DIR` |
| Rust | `CARGO_HOME`, `CARGO_TARGET_DIR` |
| Generic fallback | `XDG_CACHE_HOME` |

## Example `containerEnv` Blocks

### Go

If you are running Ralph against a Go project, the default bundled `config.json` already includes these settings — you do not need to add them manually unless you are writing a custom workflow.

```json
{
  "containerEnv": {
    "GOPATH": "/home/agent/workspace/.ralph-cache/go",
    "GOCACHE": "/home/agent/workspace/.ralph-cache/go-build",
    "GOMODCACHE": "/home/agent/workspace/.ralph-cache/gomod",
    "XDG_CACHE_HOME": "/home/agent/workspace/.ralph-cache/xdg"
  }
}
```

`XDG_CACHE_HOME` is included as a defense-in-depth fallback for any auxiliary tool that lands in the container (e.g. linters, formatters, `ko`) that respects XDG paths.

`GOTMPDIR` controls where `go build` stages intermediates. Add it if `/tmp` is read-only or quota-constrained in your container:

```json
"GOTMPDIR": "/home/agent/workspace/.ralph-cache/gotmp"
```

### Node (npm / yarn / pnpm)

```json
{
  "containerEnv": {
    "NPM_CONFIG_CACHE":  "/home/agent/workspace/.ralph-cache/npm",
    "YARN_CACHE_FOLDER": "/home/agent/workspace/.ralph-cache/yarn",
    "PNPM_STORE_PATH":   "/home/agent/workspace/.ralph-cache/pnpm"
  }
}
```

### Python (pip / Poetry / uv)

```json
{
  "containerEnv": {
    "PIP_CACHE_DIR":    "/home/agent/workspace/.ralph-cache/pip",
    "POETRY_CACHE_DIR": "/home/agent/workspace/.ralph-cache/poetry",
    "UV_CACHE_DIR":     "/home/agent/workspace/.ralph-cache/uv"
  }
}
```

### Rust

```json
{
  "containerEnv": {
    "CARGO_HOME":       "/home/agent/workspace/.ralph-cache/cargo",
    "CARGO_TARGET_DIR": "/home/agent/workspace/.ralph-cache/cargo-target"
  }
}
```

## Host-Writability Precondition

The bind-mount source (your project directory on the host) must be writable by the UID/GID passed to Docker's `-u` flag. Ralph's Docker sandbox uses the current host user's UID/GID for the `-u` flag, so as long as you can write files in your project directory the cache subdirectories will be writable inside the container too.

If you run Docker in rootless mode or with a custom UID mapping, verify that the container's effective UID can write to the project directory.

## Target Project `.gitignore`

Add the following to the `.gitignore` of every project you run Ralph against:

```
# pr9k runtime state (session logs, iteration.jsonl, optional in-repo workflow override)
.pr9k/
# Ralph build artifact cache
.ralph-cache/
```

This prevents runtime state and the build cache from being accidentally committed. The pr9k repo itself already has both entries.

`.pr9k/` is the primary runtime output directory (logs, `iteration.jsonl`). `.ralph-cache/` is the bind-mounted artifact cache created separately by the Docker sandbox preflight; it lands at the project root alongside `.pr9k/`.

## Related documentation

- ← [Back to How-To Guides](README.md)
- [Passing Environment Variables](passing-environment-variables.md) — the `env` and `containerEnv` mechanism this page builds on
- [Setting Up the Docker Sandbox](setting-up-docker-sandbox.md) — how the bind-mount is wired and the UID precondition
- [Building Custom Workflows](building-custom-workflows.md) — where to put `containerEnv` in your `config.json`
- [Debugging a Run](debugging-a-run.md) — diagnosing `permission denied` errors in step logs
