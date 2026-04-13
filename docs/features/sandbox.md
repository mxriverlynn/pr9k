# Docker Sandbox

The `internal/sandbox` package constructs the `docker run` invocation that wraps every Claude step, manages the container ID file (cidfile) lifecycle, and provides a terminator closure that signals the running container on shutdown.

## Overview

Ralph-tui invokes `claude` inside an ephemeral Docker container built from the `docker/sandbox-templates:claude-code` image. The sandbox bind-mounts only the target repo and the Claude profile directory, keeping everything else on the host — other repos, `~/.ssh`, `~/.aws`, arbitrary env vars — invisible to claude.

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
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_BASE_URL",
    "HTTPS_PROXY",
    "HTTP_PROXY",
    "NO_PROXY",
}
```

These five names are always included when building the env passthrough. The caller passes the union of `BuiltinEnvAllowlist` and the per-workflow `env` array from `ralph-steps.json` to `BuildRunArgs`.

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
    model, prompt string,
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
- **`--permission-mode bypassPermissions`** — required for the unattended loop.

## Cidfile lifecycle

`cidfile.go` owns the two operations on the cidfile path:

### Path

```go
func Path() (string, error)
```

Reserves a unique, non-existent path for `--cidfile`. Creates a temp file under the system temp dir (pattern `ralph-*.cid`), then removes it — reserving the name without leaving the file present (`docker run --cidfile` requires the target path to not exist on entry).

There is a small TOCTOU race between `os.Remove` and docker's `O_CREAT|O_EXCL`, but ralph-tui does not run multiple concurrent loops (design §2), so the accepted failure mode is a loud "container ID file found" error from docker on the rare collision.

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

## Package

**Package:** `internal/sandbox/` (`image.go`, `command.go`, `cidfile.go`, `terminator.go`)
