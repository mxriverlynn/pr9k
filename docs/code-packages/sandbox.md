# Docker Sandbox

The `internal/sandbox` package constructs the `docker run` invocation that wraps every Claude step, manages the container ID file (cidfile) lifecycle, and provides a terminator closure that signals the running container on shutdown.

## Overview

pr9k invokes `claude` inside an ephemeral Docker container built from the `docker/sandbox-templates:claude-code` image. The sandbox bind-mounts only the target repo and the Claude profile directory, keeping everything else on the host — other repos, `~/.ssh`, `~/.aws`, arbitrary env vars — invisible to claude.

Non-claude steps (shell scripts, `git push`, `gh` calls) continue to run directly on the host; they need host credentials and are a different threat class.

## Constants

`image.go` defines the three path constants used by `BuildRunArgs`:

```go
const (
    ImageTag             = "docker/sandbox-templates:claude-code"
    ContainerRepoPath    = "/home/agent/workspace"
    ContainerProfilePath = "/home/agent/.claude"
)
```

- `ImageTag` — the Docker image pulled at setup time and passed to `docker run`.
- `ContainerRepoPath` — the container path where the target repo (`--project-dir`) is bind-mounted.
- `ContainerProfilePath` — the container path where the Claude profile directory is bind-mounted; also written as `CLAUDE_CONFIG_DIR` inside the container.

## BuiltinEnvAllowlist

```go
var BuiltinEnvAllowlist = []string{
    "ANTHROPIC_BASE_URL",
    "HTTPS_PROXY",
    "HTTP_PROXY",
    "NO_PROXY",
}
```

These four sandbox-plumbing names are always included when building the env passthrough. The caller passes the union of `BuiltinEnvAllowlist` and the per-workflow `env` array from `config.json` to `BuildRunArgs`.

`ANTHROPIC_API_KEY` is **not** in the builtin list. Users who want to authenticate claude steps via the API key env var must list `ANTHROPIC_API_KEY` in their workflow's `env` block; pr9k does not implicitly forward it.

## HostUIDGID

```go
func HostUIDGID() (int, int)
```

Returns the current process's UID and GID via `os.Getuid()` / `os.Getgid()`. Provided as a separate function so callers can pass explicit uid/gid values into `BuildRunArgs`, keeping that function pure (no syscalls) and fully testable.

## BuildRunArgs

```go
func BuildRunArgs(
    projectDir, profileDir string,
    uid, gid int,
    cidfile string,
    envAllowlist []string,
    containerEnv map[string]string,
    resumeSessionID string, // non-empty → appends --resume <id> before -p
    model, effort, prompt string, // non-empty effort → appends --effort <value> after --model
) []string
```

Constructs the complete `docker run ...` argv. The returned slice begins with `"docker"` and ends with the claude invocation flags. Key properties:

- **`--rm`** — container is destroyed when it exits; no residue on the host.
- **`-i`** — stdin stays attached (claude reads from it).
- **`--init`** — PID 1 is a minimal init; zombie reaping is handled correctly.
- **`--cidfile <path>`** — docker writes the 64-char container ID to `<path>` as soon as the container starts. The terminator reads this file to address `docker kill` at the right container.
- **`-u <uid>:<gid>`** — container runs as the host user; files written to the bind mount are owned by the invoking user.
- **`--mount type=bind,...`** — bind mounts use key=value syntax (`--mount type=bind,source=...,target=...`) rather than the shorthand `-v dir:target`; this avoids argument injection via colons in path names.
- **`CLAUDE_CONFIG_DIR`** — always set to `ContainerProfilePath`; callers must not include it in `envAllowlist`.
- **Env passthrough** — each name in `envAllowlist` is deduplicated (first-seen order), then emitted as `-e NAME` only if `os.LookupEnv(name)` returns `ok=true`. Unset host vars are silently skipped. Names are passed as bare `-e NAME` (not `-e NAME=value`) so the secret never appears in the docker CLI invocation.
- **containerEnv injection** — after the passthrough entries, each key in `containerEnv` is emitted as `-e KEY=VALUE` in sorted key order. This injects literal values into the container that are not present on the host. Docker's last-wins rule means containerEnv beats any same-named host passthrough. `CLAUDE_CONFIG_DIR` is silently skipped even if present as a defense-in-depth guard (the validator already rejects it, but this prevents any future call path that bypasses validation from overwriting the sandbox mount point).
- **`--permission-mode bypassPermissions`** — required for the unattended loop.
- **`--effort <value>`** — appended only when `effort` is non-empty. Valid values are enforced upstream by `steps.IsValidEffort` and the validator; this function does no further checking. Empty `effort` omits the flag entirely (the CLI's default applies). Callers compute the effective value (per-step `Step.Effort` overriding `Defaults.Effort`) at load time in `steps.LoadSteps`, so `buildStep` simply forwards `s.Effort`.
- **`--output-format stream-json`** — instructs claude to emit NDJSON on stdout so the `claudestream` pipeline can parse typed events.
- **`--verbose`** — includes all event types in the stream (assistant turns, tool calls, result); without this flag many event types are suppressed.

## Cidfile lifecycle

`cidfile.go` owns the two operations on the cidfile path:

### Path

```go
func Path() (string, error)
```

Reserves a unique, non-existent path for `--cidfile`. Creates a temp file under the system temp dir (pattern `ralph-*.cid`), then removes it — reserving the name without leaving the file present (`docker run --cidfile` requires the target path to not exist on entry).

There is a small TOCTOU race between `os.Remove` and docker's `O_CREAT|O_EXCL`, but pr9k does not run multiple concurrent loops (design §2), so the accepted failure mode is a loud "container ID file found" error from docker on the rare collision.

### Cleanup

```go
func Cleanup(path string) error
```

Removes the cidfile. `ENOENT` is tolerated — the file may not exist if `docker run` failed before writing it.

## NewTerminator

```go
func NewTerminator(cmd *exec.Cmd, cidfile string) func(syscall.Signal) error
```

Returns a closure that, when invoked with a signal, delivers it to the running container (via `docker kill --signal`) rather than to the docker CLI process. The closure is installed as the terminator on the `Runner` in place of the default `cmd.Process.Signal` call.

Behavior per signal invocation:

1. **Already exited guard** — if `cmd.ProcessState != nil`, return nil. This guards against PID-recycling hazards: if the container has exited, the docker CLI process has also exited, and the PID may have been recycled by the OS.
2. **Poll cidfile** — poll `cidfile` for up to 2 seconds (50ms interval). The cidfile is written by docker shortly after the container starts; a brief poll handles the startup race.
3. **Docker kill** — if a valid 64-char lowercase-hex container ID is found, run `docker kill --signal=<N> <cid>` (signal by number). This delivers the signal to the container's PID 1, not to the docker CLI.
4. **Fallback** — if the cidfile is still missing after 2 seconds (container never reached running state), fall back to `cmd.Process.Signal(sig)` on the docker CLI process. Signaling the CLI at this point aborts the `docker run` launch cleanly; no orphan container can exist because the container never started.

The closure captures `*exec.Cmd` (not bare `*os.Process`) so `cmd.ProcessState` is accessible — `os.Process` does not expose exit state.

### isValidCID (internal)

`pollCidfile` validates candidate container IDs with `isValidCID`, which requires exactly 64 lowercase-hex characters. This rejects partial writes (truncated IDs), whitespace-only content, and any non-hex string docker might write on error.

## BuildInteractiveArgs

```go
func BuildInteractiveArgs(profileDir string, uid, gid int) []string
```

Constructs the `docker run -it ...` argv for an interactive `claude` REPL used by `pr9k sandbox --interactive`. The user runs `/login` inside the REPL to write `.credentials.json` to the bind-mounted profile directory. Key differences from `BuildRunArgs`:

- `-it` instead of `-i` — allocates a TTY for interactive use.
- Only the profile directory is bind-mounted (no project dir, no `-w`) — login is auth-only and host project files are accidental attack surface.
- No `--cidfile` and no terminator plumbing — the user exits via Ctrl-C or `/exit`.
- No `--permission-mode`, `--model`, or `-p <prompt>` — the user drives the REPL directly.
- `CLAUDE_CONFIG_DIR` is the only unconditional env entry; `BuiltinEnvAllowlist` is not consulted here.
- `TERM` is forwarded bare (`-e TERM`, name only) when the host has it set, so the inner pty reports the host's real terminal capabilities. Without this, Docker defaults `TERM` to `xterm` and modern terminals' bracketed-paste sequences can be silently dropped by the `claude` REPL, making paste of the OAuth code appear to do nothing.

## BuildShellArgs

```go
func BuildShellArgs(projectDir, profileDir string, uid, gid int) []string
```

Constructs the `docker run -it ...` argv for an interactive `bash` shell used by `pr9k sandbox shell`. Both the project directory and the profile directory are bind-mounted, the working directory is set to the project mount, and the entrypoint is `bash`. `--rm` removes the container when the shell exits. Key differences from `BuildInteractiveArgs`:

- The host project directory is bind-mounted at `ContainerRepoPath` (read-write) and is the working directory, so the user lands in the project tree.
- Entrypoint is `bash` instead of `claude` (the `claude` binary is still on `PATH` inside the shell).
- Otherwise the shape matches `BuildInteractiveArgs`: `-it`, `--rm`, `--init`, profile mount, `CLAUDE_CONFIG_DIR`, optional `-e TERM` passthrough.

## Package

**Package:** `internal/sandbox/` (`image.go`, `command.go`, `cidfile.go`, `terminator.go`)
