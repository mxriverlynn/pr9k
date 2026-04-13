# create-sandbox Subcommand

Pulls the Docker sandbox image and verifies it can run `claude --version` under the current user's UID/GID before the first workflow run.

- **Last Updated:** 2026-04-13
- **Authors:**
  - River Bailey

## Overview

- `ralph-tui create-sandbox` is a one-shot setup command that checks Docker availability, pulls the sandbox image, and runs a smoke test to confirm the image works correctly under the current user
- It is a subcommand registered via the variadic `Execute(extra ...*cobra.Command)` pattern — the main workflow flags (`--iterations`, `--workflow-dir`, `--project-dir`) are not available when running `create-sandbox`
- The `--force` flag re-pulls the image even when it is already present
- All user-facing messages are written to stdout; error messages go to stderr
- Subprocess output (Docker pull progress, smoke test output) is never reflected verbatim to the terminal without ANSI sanitization (see SEC-001 fix)

Key files:
- `ralph-tui/cmd/ralph-tui/create_sandbox.go` — `newCreateSandboxCmd`, `newCreateSandboxCmdWith`, `runCreateSandbox`, `createSandboxDeps`, `realDockerRun`, `stripANSI`
- `ralph-tui/cmd/ralph-tui/create_sandbox_test.go` — 16 test cases covering all branches via injected `createSandboxDeps`
- `ralph-tui/cmd/ralph-tui/main.go` — registers the subcommand via `cli.Execute(newCreateSandboxCmd())`

## Usage

```
ralph-tui create-sandbox [--force]
```

| Flag | Description | Default |
|------|-------------|---------|
| `--force` | Re-pull the sandbox image even if already present | `false` |

## Architecture

```
ralph-tui create-sandbox
        │
        ▼
  runCreateSandbox(deps, force)
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

## Key Types

```go
// createSandboxDeps holds injected dependencies for unit-testable branches.
type createSandboxDeps struct {
    prober    preflight.Prober  // Docker availability and image checks
    dockerRun dockerRunFunc     // exec seam for pull and smoke test
    uid       int               // host UID passed via -u uid:gid
    gid       int               // host GID passed via -u uid:gid
    stdout    io.Writer
    stderr    io.Writer
}

// dockerRunFunc runs a docker command, directing stdout/stderr to writers.
// Returns the process exit code and any exec-level error (distinct from non-zero exit).
type dockerRunFunc func(args []string, stdout, stderr io.Writer) (exitCode int, err error)
```

## Implementation Details

### Step 1: Docker Reachability

`runCreateSandbox` uses the `preflight.Prober` interface (the same used by the preflight package) to check:

1. `DockerBinaryAvailable()` — confirms `docker` is on PATH
2. `DockerDaemonReachable()` — confirms the daemon is running

Both failures write a human-readable message to stderr and return `errSilentExit` (which tells `main` to exit 1 without printing an additional error).

### Step 2: Image Presence and Pull

`SandboxImagePresent()` runs `docker image inspect` to determine whether the image is already cached locally. If present and `--force` is not set, the pull step is skipped with an informational message.

When a pull runs, Docker's progress output streams directly to `deps.stdout` so the user sees real-time progress. Pull stderr is captured separately and forwarded only on failure.

The image tag is the compile-time constant `sandbox.ImageTag` — it is never accepted as user input, preventing tag injection.

### Step 3: Smoke Test

The smoke test runs:
```
docker run --rm -u <uid>:<gid> <ImageTag> claude --version
```

The `-u uid:gid` values come from `sandbox.HostUIDGID()` in the production path, injected via `createSandboxDeps` in tests. This ensures the container runs as the current user, not root.

Smoke test output (stdout then stderr) is sanitized with `stripANSI` before being matched against a semver pattern or printed. This prevents terminal injection if a malicious or compromised image emits ANSI escape sequences in its `--version` output (SEC-001).

Outcomes:
- **Semver match** — prints `"Sandbox verified: <version> under UID <uid>:<gid>."` and exits 0
- **Non-semver non-empty output** — prints a warning (image may be a stub or tag squat) and still exits 0
- **Empty output** — fatal error, directs user to re-pull with `--force`

### Error Handling

| Scenario | Output | Exit |
|----------|--------|------|
| Docker binary missing | `"Docker is not installed. Install Docker and try again."` to stderr | 1 |
| Docker daemon not running | `"Docker is installed but the daemon isn't running. Start Docker and try again."` to stderr | 1 |
| `SandboxImagePresent` error | `"Failed to check sandbox image: <err>"` to stderr | 1 |
| Pull exec error | `"Failed to pull sandbox image: <err>"` to stderr | 1 |
| Pull non-zero exit | `"Failed to pull sandbox image."` + captured pull stderr | 1 |
| Smoke test exec error | `"Sandbox smoke test failed: <err>"` to stderr | 1 |
| Smoke test non-zero exit | `"Sandbox smoke test failed — container exited with status <n>."` + stderr | 1 |
| Smoke test no output | `"Sandbox smoke test failed — image ran but produced no version output. ..."` | 1 |
| Smoke test unexpected output (warn) | Warning to stdout; still exits 0 | 0 |
| Success | `"Sandbox verified: <version> under UID <uid>:<gid>."` + `"Sandbox ready."` | 0 |

### Dependency Injection Design

`newCreateSandboxCmd()` wires the production `createSandboxDeps` (real `preflight.RealProber`, `realDockerRun`, `sandbox.HostUIDGID()`, `os.Stdout`/`os.Stderr`). Tests call `newCreateSandboxCmdWith(deps)` with a `fakeProber` and `fakeRun`, exercising every branch without shelling out to a real Docker daemon.

`realDockerRun` includes a bounds guard — it returns an error immediately if `args` is empty, preventing a panic from `args[0]` on an empty slice.

## Testing

- `ralph-tui/cmd/ralph-tui/create_sandbox_test.go` — 16 test cases

### Test Cases

| Test | What It Validates |
|------|-------------------|
| `TestCreateSandbox_DockerBinaryMissing` | Binary not on PATH → `errSilentExit` + install message, no exec calls |
| `TestCreateSandbox_DaemonUnreachable` | Daemon not running → `errSilentExit` + start message |
| `TestCreateSandbox_ImagePresentErr` | `SandboxImagePresent` error → `errSilentExit` + error message, no exec calls |
| `TestCreateSandbox_ImagePresent_NoForce` | Image present, no `--force` → pull skipped, smoke test runs, `"Sandbox ready."` |
| `TestCreateSandbox_ImagePresent_Force` | Image present, `--force` → pull runs, smoke test runs, `"Sandbox ready."` |
| `TestCreateSandbox_PullExecError` | Pull exec error → `errSilentExit` + error message, smoke test not run |
| `TestCreateSandbox_PullFails_SmokeNotRun` | Pull exit non-zero → `errSilentExit` + failure message, smoke test not run |
| `TestCreateSandbox_PullFails_StderrForwarded` | Pull exit non-zero with stderr → stderr forwarded to user |
| `TestCreateSandbox_PullFails_EmptyStderr` | Pull exit non-zero, no stderr → only `"Failed to pull sandbox image.\n"` in stderr |
| `TestCreateSandbox_SmokeExecError` | Smoke exec error → `errSilentExit` + error message |
| `TestCreateSandbox_SmokeTest_NonZeroExit` | Smoke exit non-zero → `errSilentExit` + exit-status message + forwarded stderr |
| `TestCreateSandbox_SmokeTest_EmptyOutput` | Smoke exit 0, no output → `errSilentExit` + no-version-output message |
| `TestCreateSandbox_SmokeTest_UnexpectedOutput` | Smoke exit 0, non-semver output → warning to stdout, exits 0, `"Sandbox ready."` |
| `TestCreateSandbox_SmokeTest_Success` | Smoke exit 0, valid semver → `"Sandbox verified: ... under UID 501:20."`, exits 0 |
| `TestCreateSandbox_SmokeTest_VersionFromStderr` | Version on stderr (not stdout) → accepted and printed to stdout |
| `TestCreateSandbox_SmokeTest_ArgsIncludeUID` | Smoke test argv contains `-u <uid>:<gid>` from injected uid/gid |

## Security

**SEC-001 (OWASP A03 — Injection):** Smoke test output from `claude --version` inside the pulled Docker image is sanitized with `stripANSI` before being printed to the terminal. This prevents a malicious or compromised image from injecting terminal escape sequences (e.g., `\x1b]2;malicious-title\x07`, `\x1b[8m` hidden text) that would be interpreted by the user's terminal.

`stripANSI` removes sequences matching:
- CSI sequences: `\x1b[` ... `[@-~]` (color, cursor movement, hidden text)
- OSC sequences: `\x1b]` ... `\x07` (window title manipulation)
- Fe sequences: `\x1b[@-Z\\-_]`

## Additional Information

- [Architecture Overview](../architecture.md)
- [Sandbox Package](sandbox.md) — `BuildRunArgs`, `HostUIDGID`, cidfile lifecycle, and `NewTerminator`
- [Preflight Package](preflight.md) — `Prober` interface, `RealProber`, `CheckDocker`
- [API Design Coding Standard](../coding-standards/api-design.md) — Bounds guards and dependency injection patterns used here
