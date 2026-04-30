# Passing Environment Variables to the Sandbox

← [Back to How-To Guides](README.md)

Claude steps run inside a Docker container with a scrubbed environment. By default, only five sandbox-plumbing variables are forwarded from the host. If your workflow needs additional host environment variables inside the container — API tokens, proxy settings, feature flags — you declare them in `config.json`.

**Prerequisites**: a working install with the sandbox set up — see [Setting Up the Docker Sandbox](setting-up-docker-sandbox.md) — and familiarity with the step schema in [Building Custom Workflows](building-custom-workflows.md).

## The `env` field

Add a top-level `env` array to your `config.json`:

```json
{
  "env": ["GH_TOKEN", "MY_CUSTOM_VAR"],
  "initialize": [ ... ],
  "iteration": [ ... ],
  "finalize": [ ... ]
}
```

Each entry is the **name** of a host environment variable (not a `KEY=VALUE` pair). Docker reads the value from the host environment at container start. If the variable is not set on the host, it is silently skipped — no error, no empty string injected.

The `env` array applies to **all** `isClaude: true` steps. Shell command steps run directly on the host and inherit the full host environment, so they do not need `env` entries.

## What gets forwarded automatically

Five variables are always attempted, regardless of the `env` field:

| Variable | Purpose |
|----------|---------|
| `ANTHROPIC_API_KEY` | Direct API authentication (bypasses OAuth) |
| `ANTHROPIC_BASE_URL` | Custom API endpoint |
| `HTTPS_PROXY` | HTTPS proxy for outbound requests |
| `HTTP_PROXY` | HTTP proxy for outbound requests |
| `NO_PROXY` | Proxy exclusion list |

These are defined in `sandbox.BuiltinEnvAllowlist`. You do not need to repeat them in `env`.

Additionally, `CLAUDE_CONFIG_DIR=/home/agent/.claude` is always set inside the container with an explicit value (the mount point), not a passthrough.

## How merging works

At build time, pr9k merges the builtin allowlist with your `env` entries:

```
final allowlist = BuiltinEnvAllowlist + env (from config.json)
```

Duplicates are de-duplicated by name (first-seen wins). Each name is passed to Docker as `-e NAME` (no `=VALUE`), so Docker reads the value from the host. If `os.LookupEnv(name)` returns false on the host, the `-e` flag is still added — Docker itself silently omits unset variables.

## Validation rules

The D13 config validator (Category 10) checks every entry in `env` at startup. A validation error exits 1 before the TUI starts:

| Rule | Example violation |
|------|-------------------|
| Empty string | `""` |
| Invalid identifier | `"MY-VAR"` (hyphens not allowed), `"123ABC"` (starts with digit) |
| Reserved sandbox name | `"CLAUDE_CONFIG_DIR"`, `"HOME"` |
| Denied for safety | `"PATH"`, `"USER"`, `"SSH_AUTH_SOCK"`, `"LD_PRELOAD"` |

Valid names match the regex `^[A-Za-z_][A-Za-z0-9_]*$`.

## Example: forwarding a GitHub token

The default workflow forwards `GH_TOKEN` so that Claude can use the GitHub CLI inside the container:

```json
{
  "env": ["GH_TOKEN"],
  "initialize": [ ... ],
  "iteration": [ ... ],
  "finalize": [ ... ]
}
```

Before running pr9k, set the variable on the host:

```bash
export GH_TOKEN=$(gh auth token)
/path/to/bin/pr9k
```

Inside the container, `echo $GH_TOKEN` will print the token value.

## Debugging: is my variable reaching the container?

If a claude step fails because it can't find an expected variable:

1. Verify the variable is set on the host: `echo $MY_VAR`
2. Verify it's listed in `config.json`'s `env` array
3. Check that the validator didn't reject it: validation errors appear on stderr before the TUI starts
4. Check for typos — the name must match exactly (case-sensitive)

## The `containerEnv` field

`env` forwards values from the host at container start. Use `containerEnv` when the value does not exist on the host or when you want to pin a specific literal value regardless of the host environment — for example, redirecting build caches into the bind-mounted workspace so they persist across runs:

```json
{
  "env": ["GH_TOKEN"],
  "containerEnv": {
    "GOCACHE": "/home/agent/workspace/.ralph-cache/go",
    "GOMODCACHE": "/home/agent/workspace/.ralph-cache/gomod",
    "GOPATH": "/home/agent/workspace/.ralph-cache/gopath"
  },
  "initialize": [ ... ],
  "iteration": [ ... ],
  "finalize": [ ... ]
}
```

Key differences from `env`:

| | `env` | `containerEnv` |
|--|-------|----------------|
| Value source | Host environment at container start | Literal value in `config.json` |
| Stored in repo | Name only (safe) | Name **and value** (committed to repo) |
| When to use | Secrets, per-machine config | Build paths, feature flags, fixed constants |
| Precedence | Applied first | Applied after `env` — Docker last-wins, so containerEnv beats host passthrough for the same key |

### Constraints

- Keys must not be `CLAUDE_CONFIG_DIR` (reserved for the sandbox mount point) — the validator rejects this with a fatal error.
- Keys must not contain `=`; values must not contain newlines or NUL — both are fatal errors.
- `containerEnv` values are committed to `config.json`. **Do not store secrets here.** The validator emits a warning when a key ends with `_TOKEN`, `_KEY`, `_SECRET`, `_PASSWORD`, `_PASSPHRASE`, `_CREDENTIAL`, or `_APIKEY`.

### The `.ralph-cache` directory

pr9k creates `<projectDir>/.ralph-cache/` at startup via `preflight.Run` so that Docker bind-mount subpaths (e.g., `GOCACHE=/home/agent/workspace/.ralph-cache/go`) are present before any Claude step runs. Add `.ralph-cache/` to `.gitignore` in your target repo to keep the build artifact cache out of commits, alongside the pr9k runtime-state entries from [Getting Started](getting-started.md). Do **not** ignore the entire `.pr9k/` folder — `.pr9k/workflow/` is a tracked source directory for committed per-repo workflow overrides. See [Caching Build Artifacts](caching-build-artifacts.md#target-project-gitignore) for the combined block.

## Related documentation

- ← [Back to How-To Guides](README.md)
- [Setting Up the Docker Sandbox](setting-up-docker-sandbox.md) — first-time Docker setup, mounts, and auth
- [Caching Build Artifacts](caching-build-artifacts.md) — using `containerEnv` to point Go/Node/Python/Rust caches at `.ralph-cache/`
- [Building Custom Workflows](building-custom-workflows.md) — full step schema
- [Docker Sandbox](../features/docker-sandbox.md) — mount layout, env allowlist, full `docker run` command (contributor reference)
- [Config Validation](../code-packages/validator.md) — env validation rules
- [Preflight](../code-packages/preflight.md) — startup checks that create `.ralph-cache` before Claude steps run
