# Preflight Checks

The `internal/preflight` package performs startup validation before the main orchestration loop runs. It resolves and validates the Claude profile directory, checks that Docker is installed and reachable, and verifies the sandbox image is present locally. All checks are collected before returning (collect-all-errors pattern), so the caller receives the full list of failures in one pass.

The package is fully injectable via `Prober` for unit testing. It is wired into `startup()` in `cmd/ralph-tui/main.go`, which collects both D13 validation errors and preflight errors before printing any output.

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

Returns `$CLAUDE_CONFIG_DIR` if set and non-empty (trailing whitespace is trimmed to guard against `.env` parser artifacts), otherwise `$HOME/.claude`. The path is passed through `filepath.Abs` but symlinks are not resolved — realpath is not material for the stat check.

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

Runs the three-step docker check, short-circuiting on the first failure and returning a nil or empty slice on success:
1. Binary missing → `"preflight: docker is not installed. Install Docker and try again"`
2. Binary present, daemon unreachable → `"preflight: docker daemon isn't running. Start Docker and try again"`
3. Daemon reachable, image missing → `"preflight: claude sandbox image is missing. Run: ralph-tui create-sandbox"`
4. All green → nil or empty slice

At most one error is returned per `CheckDocker` call. Multiple failures across profile and docker are surfaced together by `Run`.

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

The caller (`startup()`) prints all D13 + preflight errors together before exiting.

## Testing

- `ralph-tui/internal/preflight/profile_test.go` — `ResolveProfileDir`, `CheckProfileDir`, `CheckCredentials`:
  - `TestResolveProfileDir_WithCLAUDE_CONFIG_DIR` — verifies `$CLAUDE_CONFIG_DIR` is returned when set and non-empty
  - `TestResolveProfileDir_FallsBackToHomeClaud` — verifies fallback to `$HOME/.claude` when `$CLAUDE_CONFIG_DIR` is unset
  - `TestResolveProfileDir_TrailingWhitespace_Trimmed` — verifies trailing whitespace in `$CLAUDE_CONFIG_DIR` is trimmed
  - `TestResolveProfileDir_LeadingAndTrailingWhitespace_Trimmed` (SUGG-003) — verifies both leading and trailing whitespace are trimmed (not just trailing), guarding against `.env` parser artifacts
  - `TestResolveProfileDir_RelativePath_BecomeAbsolute` — verifies a relative path is made absolute via `filepath.Abs`
  - `TestResolveProfileDir_BothEnvVarsEmpty_FallsBackToCwdClaud` — verifies fallback when both `$CLAUDE_CONFIG_DIR` and `$HOME` are empty
  - `TestCheckProfileDir_NonexistentPath` — verifies "not found" error message when the path does not exist
  - `TestCheckProfileDir_FilePath` — verifies "not a directory" error message when the path points to a file
  - `TestCheckProfileDir_ValidDirectory` — verifies nil error for an existing directory
  - `TestCheckProfileDir_StatPermissionError_WrappedWithContext` — verifies non-ENOENT stat errors are propagated with context
  - `TestCheckCredentials_NoCredentialsFile` — verifies missing `.credentials.json` returns empty warning and nil error
  - `TestCheckCredentials_ZeroByteCredentials` — verifies a zero-byte credentials file returns a non-empty warning
  - `TestCheckCredentials_NonEmptyCredentials` — verifies a non-empty credentials file returns no warning
  - `TestCheckCredentials_StatPermissionError_PropagatedWrapped` — verifies permission errors are propagated as errors (not warnings)
- `ralph-tui/internal/preflight/docker_test.go` — `CheckDocker`:
  - `TestCheckDocker_BinaryMissing` — verifies "docker is not installed" error when binary is absent
  - `TestCheckDocker_DaemonUnreachable` — verifies "docker daemon isn't running" error when binary present but daemon unreachable
  - `TestCheckDocker_ImageMissing` — verifies "claude sandbox image is missing" error when daemon is up but image absent
  - `TestCheckDocker_AllGreen` — verifies nil slice when binary, daemon, and image are all available
  - `TestCheckDocker_ImageNonExitError_WrappedWithContext` — verifies non-exit-error from `docker image inspect` propagates with context
  - `TestCheckDocker_BinaryMissing_ShortCircuits` — verifies daemon check is skipped when binary is absent
  - `TestCheckDocker_DaemonUnreachable_ShortCircuits` — verifies image check is skipped when daemon is unreachable
- `ralph-tui/internal/preflight/run_test.go` — `Run`:
  - `TestRun_ProfileDirMissing` — verifies missing profile dir produces error in Result
  - `TestRun_ProfileDirIsFile` — verifies file-path profile dir produces error in Result
  - `TestRun_DockerBinaryMissing` — verifies docker binary missing produces error in Result
  - `TestRun_DockerDaemonUnreachable` — verifies docker daemon unreachable produces error in Result
  - `TestRun_ImageNotPresent` — verifies sandbox image missing produces error in Result
  - `TestRun_ZeroByteCredentials_WarningNotFatal` — verifies zero-byte credentials produces a warning but no error (non-fatal)
  - `TestRun_CredentialsPermissionError_CollectedAsError` — verifies credentials stat permission error is collected as an error
  - `TestRun_AllGreen` — verifies nil errors and no warnings when all checks pass with non-empty credentials
  - `TestRun_CollectsAllErrors_ProfileAndDocker` — verifies both profile and docker errors are collected even when profile check fails first

## Package

**Package:** `internal/preflight/` (`profile.go`, `docker.go`, `run.go`)
