# sandbox Subcommand

`ralph-tui sandbox` is the parent command for sandbox-management subcommands. It has two children:

- **`sandbox create`** — one-shot setup. Checks Docker, pulls the sandbox image, smoke-tests it under the current user.
- **`sandbox login`** — launches an interactive `claude` REPL inside the sandbox so the user can run `/login` and write `.credentials.json` to the bind-mounted profile directory.

- **Last Updated:** 2026-04-14
- **Authors:**
  - River Bailey

## Overview

- `sandbox` is a cobra parent command with no `RunE` — running bare `ralph-tui sandbox` prints the help text listing its children.
- Subcommands are registered via `cli.Execute(newSandboxCmd())` in `main.go`. Main-workflow flags (`--iterations`, `--workflow-dir`, `--project-dir`) are not available on either child.
- `--force` on `sandbox create` re-pulls the image even when it is already present.
- `sandbox login` mounts **only** the profile directory (never a project directory) — login is an auth-only operation; exposing host files to the container is accidental attack surface.
- If the sandbox image is missing when `sandbox login` runs, it auto-pulls with a verbose note pointing at `sandbox create` for future runs.

Key files:

- `ralph-tui/cmd/ralph-tui/sandbox.go` — `newSandboxCmd` (parent), shared helpers (`errSilentExit`, `dockerRunFunc`, `realDockerRun`, `dockerInteractiveFunc`, `realDockerInteractive`, `stripANSI`, `ansiEscapeRe`)
- `ralph-tui/cmd/ralph-tui/sandbox_create.go` — `newSandboxCreateCmd`, `newSandboxCreateCmdWith`, `runSandboxCreate`, `sandboxCreateDeps`, `semverRe`
- `ralph-tui/cmd/ralph-tui/sandbox_login.go` — `newSandboxLoginCmd`, `newSandboxLoginCmdWith`, `runSandboxLogin`, `sandboxLoginDeps`
- `ralph-tui/cmd/ralph-tui/sandbox_create_test.go` — test cases for create covering all branches via injected `sandboxCreateDeps`
- `ralph-tui/cmd/ralph-tui/sandbox_login_test.go` — test cases for login using injected `sandboxLoginDeps` with a `fakeInteractive` sibling to `fakeRun`
- `ralph-tui/cmd/ralph-tui/main.go` — registers the parent via `cli.Execute(newSandboxCmd())`
- `ralph-tui/internal/sandbox/command.go` — `BuildRunArgs` (workflow) and `BuildLoginArgs` (login)

## Usage

```
ralph-tui sandbox                   # prints help
ralph-tui sandbox create [--force]
ralph-tui sandbox login
```

| Command | Flag | Description |
|---------|------|-------------|
| `sandbox create` | `--force` | Re-pull the sandbox image even if already present (default: false) |
| `sandbox login` | — | (no flags) |

## Architecture — `sandbox create`

```
ralph-tui sandbox create
        │
        ▼
  runSandboxCreate(deps, force)
        │
        ├─ Step 1: Docker reachability
        │       deps.prober.DockerBinaryAvailable()
        │       deps.prober.DockerDaemonReachable()
        │
        ├─ Step 2: Image presence / pull
        │       deps.prober.SandboxImagePresent()
        │       if !present || force:
        │           deps.dockerRun(["docker", "pull", ImageTag], ...)
        │
        └─ Step 3: Smoke test
                deps.dockerRun(["docker", "run", "--rm", "-u", uid:gid, ImageTag, "claude", "--version"], ...)
                stripANSI(output) → semverRe match → "Sandbox verified:" or warning
```

## Architecture — `sandbox login`

```
ralph-tui sandbox login
        │
        ▼
  runSandboxLogin(deps)
        │
        ├─ Step 1: Docker reachability (same prober calls as create)
        │
        ├─ Step 2: Image presence; auto-pull on miss
        │       if !present:
        │           verbose note → deps.stdout
        │           deps.dockerRun(["docker", "pull", ImageTag], ...)
        │
        ├─ Step 3: Ensure profile dir exists
        │       os.MkdirAll(deps.profileDir, 0o700)
        │
        └─ Step 4: Interactive docker run
                deps.dockerInteractive(sandbox.BuildLoginArgs(profileDir, uid, gid),
                                       deps.stdin, deps.stdout, deps.stderr)
                (user types /login inside the claude REPL; /login writes .credentials.json
                 into profileDir via the rw bind-mount)
```

## Key Types

```go
// sandboxCreateDeps holds injected dependencies for unit-testable branches.
type sandboxCreateDeps struct {
    prober    preflight.Prober
    dockerRun dockerRunFunc
    uid, gid  int
    stdout    io.Writer
    stderr    io.Writer
}

// sandboxLoginDeps holds injected dependencies for the interactive login path.
type sandboxLoginDeps struct {
    prober            preflight.Prober
    dockerInteractive dockerInteractiveFunc
    dockerRun         dockerRunFunc   // used for the auto-pull fallback
    uid, gid          int
    profileDir        string
    stdin             io.Reader
    stdout            io.Writer
    stderr            io.Writer
}

// dockerRunFunc runs a docker command, directing stdout/stderr to writers.
type dockerRunFunc func(args []string, stdout, stderr io.Writer) (exitCode int, err error)

// dockerInteractiveFunc is like dockerRunFunc but also attaches stdin for
// `docker run -it` usage.
type dockerInteractiveFunc func(args []string, stdin io.Reader, stdout, stderr io.Writer) (exitCode int, err error)
```

## `BuildLoginArgs` shape

```
docker run -it --rm --init
  -u <uid>:<gid>
  --mount type=bind,source=<profileDir>,target=/home/agent/.claude
  -e CLAUDE_CONFIG_DIR=/home/agent/.claude
  [-e TERM]
  <ImageTag>
  claude
```

Differences from `BuildRunArgs`:

- `-it` instead of `-i` (allocates a TTY for interactive use).
- No project-dir bind-mount. No `-w`. No `--cidfile` (terminator lifecycle isn't needed — user Ctrl-Cs or `/exit`s).
- No `-p <prompt>`, no `--permission-mode bypassPermissions`, no `--model` — the user drives the session directly.
- No `CLAUDE_CONFIG_DIR` env passthrough from `BuiltinEnvAllowlist` — it is set explicitly; `/login` doesn't need API keys.
- `TERM` is forwarded bare (`-e TERM`, name only) when the host has it set. Without this, Docker's pty defaults `TERM` inside the container to a bare `xterm` and the inner `claude` REPL can silently drop bracketed-paste sequences emitted by modern terminals (e.g. macOS Terminal.app) — making `Cmd+V` of the OAuth code appear to do nothing.

## Error Handling

### `sandbox create`

| Scenario | Output | Exit |
|----------|--------|------|
| Docker binary missing | `"Docker is not installed. Install Docker and try again."` to stderr | 1 |
| Docker daemon not running | `"Docker is installed but the daemon isn't running. Start Docker and try again."` | 1 |
| `SandboxImagePresent` error | `"Failed to check sandbox image: <err>"` | 1 |
| Pull exec error | `"Failed to pull sandbox image: <err>"` | 1 |
| Pull non-zero exit | `"Failed to pull sandbox image."` + captured stderr | 1 |
| Smoke test exec error | `"Sandbox smoke test failed: <err>"` | 1 |
| Smoke test non-zero exit | `"Sandbox smoke test failed — container exited with status <n>."` | 1 |
| Smoke test no output | `"Sandbox smoke test failed — image ran but produced no version output. ..."` | 1 |
| Smoke test unexpected output (warn) | Warning to stdout; still exits 0 | 0 |
| Success | `"Sandbox verified: <version> under UID <uid>:<gid>."` + `"Sandbox ready."` | 0 |

### `sandbox login`

| Scenario | Output | Exit |
|----------|--------|------|
| Docker binary missing | Same as create | 1 |
| Docker daemon not running | Same as create | 1 |
| `SandboxImagePresent` error | `"Failed to check sandbox image: <err>"` | 1 |
| Image missing | Verbose note to stdout, then pull; if pull succeeds, proceed to Step 3 | cont. |
| Pull failure | `"Failed to pull sandbox image."` + captured stderr | 1 |
| Profile dir prep fails (e.g. path is a file) | `"Failed to prepare profile directory <path>: <err>"` | 1 |
| Interactive exec error | `"Sandbox login failed: <err>"` | 1 |
| Interactive non-zero exit | (no extra message — claude's REPL output stands on its own) | 1 |
| Success | (user completes `/login` in the REPL, exits) | 0 |

## Dependency Injection Design

`newSandboxCreateCmd()` and `newSandboxLoginCmd()` wire the production deps (`preflight.RealProber`, `realDockerRun`, `realDockerInteractive`, `sandbox.HostUIDGID()`, `preflight.ResolveProfileDir()`, real `os.Stdin`/`Stdout`/`Stderr`). Tests call the `*With(deps)` variant with a `fakeProber`, `fakeRun`, and (for login) `fakeInteractive`, exercising every branch without a real Docker daemon or TTY.

`realDockerRun` and `realDockerInteractive` both include a bounds guard — they return an error immediately if `args` is empty, preventing a panic from `args[0]` on an empty slice.

## Security

**SEC-001 (OWASP A03 — Injection):** Smoke test output from `claude --version` inside the pulled Docker image is sanitized with `stripANSI` before being printed to the terminal. This prevents a malicious or compromised image from injecting terminal escape sequences that would be interpreted by the user's terminal.

`stripANSI` removes sequences matching:

- CSI sequences: `\x1b[` ... `[@-~]` (color, cursor movement, hidden text)
- OSC sequences: `\x1b]` ... `\x07` (window title manipulation)
- Fe sequences: `\x1b[@-Z\\-_]`

`sandbox login` does not sanitize its interactive stream — once the user is driving a TTY session, terminal control codes are expected and any filtering would break the REPL UX.

## Additional Information

- [Architecture Overview](../architecture.md)
- [Sandbox Package](sandbox.md) — `BuildRunArgs`, `BuildLoginArgs`, `HostUIDGID`, cidfile lifecycle, and `NewTerminator`
- [Preflight Package](preflight.md) — `Prober` interface, `RealProber`, `CheckDocker`, `ResolveProfileDir`
- [API Design Coding Standard](../coding-standards/api-design.md) — bounds guards and dependency injection patterns used here
