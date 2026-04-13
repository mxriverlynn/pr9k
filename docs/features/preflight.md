# Preflight Checks

The `internal/preflight` package performs startup validation before the main orchestration loop runs. It resolves and validates the Claude profile directory, checks that Docker is installed and reachable, and verifies the sandbox image is present locally. All checks are collected before returning (collect-all-errors pattern), so the caller receives the full list of failures in one pass.

No wiring into `main()` is present — that happens in ticket 8. The package is fully injectable via `Prober` for unit testing.

## Overview

At startup, ralph-tui must confirm three things before launching any Claude subprocess:
1. The Claude profile directory exists and is a directory.
2. Docker is installed, the daemon is running, and the sandbox image is available locally.
3. The credentials file in the profile dir is not empty (warning only, non-fatal).

`preflight.Run` orchestrates all three checks and returns a `Result` containing every warning and every error regardless of intermediate failures.

## ResolveProfileDir

```go
func ResolveProfileDir() string
```

Returns `$CLAUDE_CONFIG_DIR` if set and non-empty, otherwise `$HOME/.claude`. The path is passed through `filepath.Abs` but symlinks are not resolved — realpath is not material for the stat check.

## CheckProfileDir

```go
func CheckProfileDir(path string) error
```

Stats `path`:
- Not exist → error: `claude profile directory not found: <path>. Set CLAUDE_CONFIG_DIR or create ~/.claude`
- Stat succeeds but `fi.IsDir() == false` → error: `claude profile path is not a directory: <path>. Point CLAUDE_CONFIG_DIR at a directory`
- Directory present → nil

## CheckCredentials

```go
func CheckCredentials(profileDir string) (warning string, _ error)
```

Checks `<profileDir>/.credentials.json`:
- Missing file → empty warning, nil error (fresh profile is valid)
- Zero-byte file → non-empty warning containing "will likely fail authentication"
- Non-empty file → empty warning, nil error
- Any stat error other than `os.ErrNotExist` → propagated as an error (not a warning)

## Prober interface

```go
type Prober interface {
    DockerBinaryAvailable() bool
    DockerDaemonReachable() error
    SandboxImagePresent() (bool, error)
}
```

Abstracts docker binary and daemon probes so unit tests can drive every failure mode without shelling out.

## RealProber

```go
type RealProber struct{}
```

The production `Prober` implementation:
- `DockerBinaryAvailable` — `exec.LookPath("docker")`
- `DockerDaemonReachable` — `exec.Command("docker", "version").Run()`
- `SandboxImagePresent` — `exec.Command("docker", "image", "inspect", sandbox.ImageTag).Run()`

An `*exec.ExitError` from `docker image inspect` is treated as "image absent" (returns `false, nil`). Other errors propagate.

## CheckDocker

```go
func CheckDocker(p Prober) []error
```

Runs the three-step docker check and collects errors before returning:
1. Binary missing → `"docker is not installed. Install Docker and try again"`
2. Binary present, daemon unreachable → `"docker daemon isn't running. Start Docker and try again"`
3. Daemon reachable, image missing → `"claude sandbox image is missing. Run: ralph-tui create-sandbox"`
4. All green → nil or empty slice

Each failure stops further checks in the sequence (a missing binary makes the daemon check meaningless), but multiple failures across profile and docker are surfaced together by `Run`.

## Result and Run

```go
type Result struct {
    Warnings []string
    Errors   []error
}

func Run(profileDir string, p Prober) Result
```

Orchestrates the full preflight sequence. All results are collected before returning regardless of failures:
1. `CheckProfileDir(profileDir)`
2. `CheckDocker(p)`
3. `CheckCredentials(profileDir)` — warnings only

The caller (ticket 8) prints all D13 + preflight errors together before exiting.

## Package

**Package:** `internal/preflight/` (`profile.go`, `docker.go`, `run.go`)
