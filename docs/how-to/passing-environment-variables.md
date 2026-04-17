# Passing Environment Variables to the Sandbox

Claude steps run inside a Docker container with a scrubbed environment. By default, only five sandbox-plumbing variables are forwarded from the host. If your workflow needs additional host environment variables inside the container — API tokens, proxy settings, feature flags — you declare them in `ralph-steps.json`.

## The `env` field

Add a top-level `env` array to your `ralph-steps.json`:

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

At build time, ralph-tui merges the builtin allowlist with your `env` entries:

```
final allowlist = BuiltinEnvAllowlist + env (from ralph-steps.json)
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

Before running ralph-tui, set the variable on the host:

```bash
export GH_TOKEN=$(gh auth token)
/path/to/bin/ralph-tui
```

Inside the container, `echo $GH_TOKEN` will print the token value.

## Debugging: is my variable reaching the container?

If a claude step fails because it can't find an expected variable:

1. Verify the variable is set on the host: `echo $MY_VAR`
2. Verify it's listed in `ralph-steps.json`'s `env` array
3. Check that the validator didn't reject it: validation errors appear on stderr before the TUI starts
4. Check for typos — the name must match exactly (case-sensitive)

## Related documentation

- [Docker Sandbox](../features/docker-sandbox.md) — Mount layout, env allowlist behavior, and the full `docker run` command
- [sandbox Package](../code-packages/sandbox.md) — `BuildRunArgs`, `BuiltinEnvAllowlist`, and set-on-host filtering
- [Config Validation](../code-packages/validator.md) — Category 10 env validation rules
- [Building Custom Workflows](building-custom-workflows.md) — How to create custom step sequences
- [Step Definitions & Prompt Building](../code-packages/steps.md) — The `StepFile.Env` field in the JSON schema
- [Setting Up Docker Sandbox](setting-up-docker-sandbox.md) — First-time Docker setup and authentication
