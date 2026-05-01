# Preflight Checks

The `internal/preflight` package performs startup validation before the main orchestration loop runs. It resolves and validates the Claude profile directory and checks that Docker is installed and reachable, with both checks gated on whether the loaded workflow contains any claude steps. All checks are collected before returning (collect-all-errors pattern), so the caller receives the full list of failures in one pass.

The package is fully injectable via `Prober` for unit testing. It is wired into `startup()` in `cmd/pr9k/main.go`, which collects both D13 validation errors and preflight errors before printing any output.

## Overview

At startup, pr9k must confirm two things before launching any Claude subprocess:
1. The Claude profile directory exists and is a directory.
2. Docker is installed, the daemon is running, and the sandbox image is available locally.

Both checks are **claude-only prerequisites**: when the loaded workflow contains zero claude steps, neither check runs, and pr9k can be used on a host with no claude profile dir and no docker installed.

`preflight.Run` orchestrates the checks and returns a `Result` containing every error regardless of intermediate failures. pr9k does not check or warn about the credentials file — authentication is the user's responsibility, and a missing or invalid credentials file surfaces at runtime when the in-container claude binary refuses to authenticate.

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
3. Daemon reachable, image missing → `"preflight: claude sandbox image is missing. Run: pr9k sandbox create"`
4. All green → nil or empty slice

At most one error is returned per `CheckDocker` call. Multiple failures across profile and docker are surfaced together by `Run`.

## Result and Run

```go
type Result struct {
    Warnings []string
    Errors   []error
}

func Run(projectDir, profileDir string, hasClaudeSteps bool, p Prober) Result
```

Orchestrates the full preflight sequence. All results are collected before returning regardless of failures:

1. `os.MkdirAll(projectDir+"/.pr9k", 0o755)` — creates the umbrella directory for `iteration.jsonl` and `.pr9k/logs/` on first run. Pre-created under the host UID before the container runs, to avoid a chmod fight when the container writes via the sandbox's UID mapping. If creation fails, a `"preflight: could not create .pr9k in <path>"` error is appended. The operation is idempotent — repeat calls on an existing dir are a no-op. **Always runs**, regardless of `hasClaudeSteps`.
2. **If `hasClaudeSteps` is false**, returns immediately with only the `.pr9k` result. The remaining checks are skipped.
3. `CheckProfileDir(profileDir)` — only when `hasClaudeSteps` is true.
4. `CheckDocker(p)` — only when `hasClaudeSteps` is true.

The caller (`startup()`) prints all D13 + preflight errors together before exiting. `hasClaudeSteps` is computed by `cmd/pr9k/main.go::hasClaudeSteps` from the loaded `steps.StepFile`.

## Testing

- `src/internal/preflight/profile_test.go` — `ResolveProfileDir`, `CheckProfileDir`:
  - `TestResolveProfileDir_WithCLAUDE_CONFIG_DIR` — verifies `$CLAUDE_CONFIG_DIR` is returned when set and non-empty
  - `TestResolveProfileDir_FallsBackToHomeClaud` — verifies fallback to `$HOME/.claude` when `$CLAUDE_CONFIG_DIR` is unset
  - `TestResolveProfileDir_TrailingWhitespace_Trimmed` — verifies trailing whitespace in `$CLAUDE_CONFIG_DIR` is trimmed
  - `TestResolveProfileDir_LeadingAndTrailingWhitespace_Trimmed` — verifies both leading and trailing whitespace are trimmed (not just trailing), guarding against `.env` parser artifacts
  - `TestResolveProfileDir_RelativePath_BecomeAbsolute` — verifies a relative path is made absolute via `filepath.Abs`
  - `TestResolveProfileDir_BothEnvVarsEmpty_FallsBackToCwdClaud` — verifies fallback when both `$CLAUDE_CONFIG_DIR` and `$HOME` are empty
  - `TestCheckProfileDir_NonexistentPath` — verifies "not found" error message when the path does not exist
  - `TestCheckProfileDir_FilePath` — verifies "not a directory" error message when the path points to a file
  - `TestCheckProfileDir_ValidDirectory` — verifies nil error for an existing directory
  - `TestCheckProfileDir_StatPermissionError_WrappedWithContext` — verifies non-ENOENT stat errors are propagated with context
- `src/internal/preflight/docker_test.go` — `CheckDocker`:
  - `TestCheckDocker_BinaryMissing` — verifies "docker is not installed" error when binary is absent
  - `TestCheckDocker_DaemonUnreachable` — verifies "docker daemon isn't running" error when binary present but daemon unreachable
  - `TestCheckDocker_ImageMissing` — verifies "claude sandbox image is missing" error when daemon is up but image absent
  - `TestCheckDocker_AllGreen` — verifies nil slice when binary, daemon, and image are all available
  - `TestCheckDocker_ImageNonExitError_WrappedWithContext` — verifies non-exit-error from `docker image inspect` propagates with context
  - `TestCheckDocker_BinaryMissing_ShortCircuits` — verifies daemon check is skipped when binary is absent
  - `TestCheckDocker_DaemonUnreachable_ShortCircuits` — verifies image check is skipped when daemon is unreachable
- `src/internal/preflight/run_test.go` — `Run`:
  - `TestRun_ProfileDirMissing` — verifies missing profile dir produces error in Result (with `hasClaudeSteps=true`)
  - `TestRun_ProfileDirIsFile` — verifies file-path profile dir produces error in Result
  - `TestRun_DockerBinaryMissing` — verifies docker binary missing produces error in Result
  - `TestRun_DockerDaemonUnreachable` — verifies docker daemon unreachable produces error in Result
  - `TestRun_ImageNotPresent` — verifies sandbox image missing produces error in Result
  - `TestRun_AllGreen` — verifies nil errors and no warnings when all checks pass
  - `TestRun_CollectsAllErrors_ProfileAndDocker` — verifies both profile and docker errors are collected even when profile check fails first
  - `TestRun_CollectsAllErrors_Pr9kProfileDocker` — verifies `.pr9k`, profile, and docker errors are all collected when projectDir is read-only
  - `TestRun_NoClaudeSteps_SkipsProfileAndDockerChecks` — verifies that when `hasClaudeSteps=false`, neither `CheckProfileDir` nor `CheckDocker` runs, so a missing profile dir and missing docker produce no errors
  - `TestRun_Pr9kDir_CreatedOnFirstRun` — verifies `.pr9k/` is created inside projectDir on first Run
  - `TestRun_Pr9kDir_IdempotentOnRepeatRun` — verifies calling Run twice does not error when `.pr9k/` already exists
  - `TestRun_Pr9kDir_ReadOnlyProjectDirSurfacesError` — verifies a read-only projectDir produces a `.pr9k` preflight error
  - `TestRun_Pr9kDir_FileClashSurfacesError` — verifies a file at the `.pr9k` path (instead of a dir) surfaces an error

## Package

**Package:** `internal/preflight/` (`profile.go`, `docker.go`, `run.go`)
