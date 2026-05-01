# Passing Environment Variables to the Sandbox

← [Back to How-To Guides](README.md)

Claude steps run inside a Docker container with a scrubbed environment. By default, only four sandbox-plumbing variables are forwarded from the host. If your workflow needs additional host environment variables inside the container — API tokens, proxy settings, feature flags — you declare them in `config.json`.

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

Four variables are always attempted, regardless of the `env` field:

| Variable | Purpose |
|----------|---------|
| `ANTHROPIC_BASE_URL` | Custom API endpoint |
| `HTTPS_PROXY` | HTTPS proxy for outbound requests |
| `HTTP_PROXY` | HTTP proxy for outbound requests |
| `NO_PROXY` | Proxy exclusion list |

These are defined in `sandbox.BuiltinEnvAllowlist`. You do not need to repeat them in `env`.

`ANTHROPIC_API_KEY` is **not** in the builtin allowlist. If you want to authenticate claude steps via the API key env var instead of the OAuth credentials file, list `ANTHROPIC_API_KEY` in your `env` array — see ["Authenticating claude steps"](#authenticating-claude-steps) below.

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

## Authenticating claude steps

Claude steps inside the sandbox need credentials to talk to Anthropic. There are two supported paths and you can use either:

1. **OAuth via `.credentials.json`** — the default. Run `pr9k sandbox --interactive` once, log in with `/login`, and pr9k writes `.credentials.json` into the bind-mounted profile dir. The bundled workflow ships with a `Claude Credentials` step that refreshes the token from your macOS keychain on each iteration. See [Setting Up the Docker Sandbox](setting-up-docker-sandbox.md#authenticate-the-bundled-claude-profile).
2. **API key via `ANTHROPIC_API_KEY`** — opt-in. Set the variable on the host and add it to your workflow's `env` array:

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

   pr9k does **not** auto-forward `ANTHROPIC_API_KEY`. Listing it in `env` is required.

If you have neither a valid `.credentials.json` nor `ANTHROPIC_API_KEY` set, pr9k will still start; the in-container claude binary will fail with an authentication error when the first claude step runs, and the step will fail with a clear message.

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

`env` forwards values from the host at container start. Use `containerEnv` when the value does not exist on the host or when you want to pin a specific literal value regardless of the host environment:

```json
{
  "env": ["GH_TOKEN"],
  "containerEnv": {
    "FEATURE_FLAG_X": "true",
    "DATABASE_URL": "postgres://container-only-host/db"
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
| When to use | Secrets, per-machine config | Feature flags, fixed constants, container-only paths |
| Precedence | Applied first | Applied after `env` — Docker last-wins, so containerEnv beats host passthrough for the same key |

### Constraints

- Keys must not be `CLAUDE_CONFIG_DIR` (reserved for the sandbox mount point) — the validator rejects this with a fatal error.
- Keys must not contain `=`; values must not contain newlines or NUL — both are fatal errors.
- `containerEnv` values are committed to `config.json`. **Do not store secrets here.** The validator emits a warning when a key ends with `_TOKEN`, `_KEY`, `_SECRET`, `_PASSWORD`, `_PASSPHRASE`, `_CREDENTIAL`, or `_APIKEY`.

## Related documentation

- ← [Back to How-To Guides](README.md)
- [Setting Up the Docker Sandbox](setting-up-docker-sandbox.md) — first-time Docker setup, mounts, and auth
- [Building Custom Workflows](building-custom-workflows.md) — full step schema
- [Docker Sandbox](../features/docker-sandbox.md) — mount layout, env allowlist, full `docker run` command (contributor reference)
- [Config Validation](../code-packages/validator.md) — env validation rules
- [Preflight](../code-packages/preflight.md) — startup checks for the profile dir and Docker
