# Docker Sandbox

Executes every Claude CLI step inside an ephemeral Docker container, limiting blast radius to the bind-mounted target repository and a scrubbed process environment.

- **Last Updated:** 2026-04-18
- **Authors:**
  - River Bailey

## Overview

- Every `isClaude: true` step runs via `docker run` against the `docker/sandbox-templates:claude-code` image; shell command steps continue to run on the host
- The target repo (`--project-dir`) is bind-mounted at `/home/agent/workspace`; the Claude profile directory is bind-mounted at `/home/agent/.claude`
- The workflow bundle (`--workflow-dir`) is NOT mounted — prompts and scripts are interpolated on the host before the container starts
- Environment variables are passed through two mechanisms: a host-forwarding allowlist (`env` field) and a literal key=value injection map (`containerEnv` field). Five sandbox-plumbing builtins are always attempted. Unlisted host vars and vars not in `containerEnv` are invisible to claude
- Each step runs as the invoking host user (`-u $(id -u):$(id -g)`) so files written to the bind-mounted repo are host-owned
- `--cidfile` captures the container ID so `Terminate()` can issue a real `docker kill` rather than signaling the host docker CLI (which would orphan the container)

Key files:
- `src/internal/sandbox/command.go` — `BuildRunArgs`
- `src/internal/sandbox/image.go` — `BuiltinEnvAllowlist`, `ImageTag`, container path constants
- `src/internal/sandbox/cidfile.go` — `Path()` (cidfile reservation), `Cleanup()` (ENOENT-tolerant removal)
- `src/internal/sandbox/terminator.go` — `NewTerminator` closure for SIGTERM/SIGKILL via `docker kill`
- `src/internal/sandbox/command_test.go` — Unit tests for `BuildRunArgs` and env allowlist merging
- `src/internal/sandbox/cidfile_test.go` — Unit tests for `Cleanup`
- `src/internal/workflow/run.go` — `buildStep` dispatches claude steps to `RunSandboxedStep`

## Architecture

```
Host
├── pr9k (orchestrator)
│   ├── builds docker run argv via sandbox.BuildRunArgs
│   ├── generates cidfile path via sandbox.Cidfile.Path()
│   └── calls Runner.RunSandboxedStep(stepName, argv, SandboxOptions{...})
│
└── Docker daemon
    └── container (docker/sandbox-templates:claude-code)
        ├── /home/agent/workspace  ← bind-mount of <PROJECT_DIR> (rw)
        ├── /home/agent/.claude    ← bind-mount of <PROFILE_DIR> (rw)
        ├── runs as host UID:GID
        └── claude --permission-mode bypassPermissions --model <MODEL> -p <PROMPT>
```

After `docker run` exits, `RunSandboxedStep` removes the cidfile (ENOENT-tolerant via `sandbox.Cleanup`).

## The Runtime Docker Command

pr9k constructs the following command for every claude step. Values in `<ANGLE_BRACKETS>` are substituted at runtime:

```
docker run                                              \
  --rm                                                  \
  -i                                                    \
  --init                                                \
  --cidfile <TMP>/ralph-<UNIQUE>.cid                    \
  -u <UID>:<GID>                                        \
  --mount type=bind,source=<PROJECT_DIR>,target=/home/agent/workspace  \
  --mount type=bind,source=<PROFILE_DIR>,target=/home/agent/.claude   \
  -w /home/agent/workspace                              \
  -e CLAUDE_CONFIG_DIR=/home/agent/.claude              \
  [-e ANTHROPIC_API_KEY]                                \
  [-e ANTHROPIC_BASE_URL]                               \
  [-e HTTPS_PROXY]                                      \
  [-e HTTP_PROXY]                                       \
  [-e NO_PROXY]                                         \
  [-e <EACH_ENTRY_FROM_RALPH_STEPS_JSON_ENV>]           \
  [-e <KEY>=<VALUE_FROM_RALPH_STEPS_JSON_CONTAINER_ENV>] \
  docker/sandbox-templates:claude-code                  \
  claude --permission-mode bypassPermissions            \
         --model <MODEL>                                \
         [--resume <SESSION_ID>]                        \
         -p <PROMPT>                                    \
         --output-format stream-json                    \
         --verbose
```

`--resume <SESSION_ID>` is injected only when the step has `resumePrevious: true` **and** all five resume gates (G1–G5) pass. See [Session Resume Gates](workflow-orchestration.md#session-resume-gates-resumeprevious) for gate details. The default workflow ships with `resumePrevious` unset on all steps.

### Flag rationale

- `--rm` — container is ephemeral; deleted on exit.
- `-i` — attach stdin. Only safe because `RunSandboxedStep` provides explicit empty stdin (`bytes.NewReader(nil)`) to prevent Bubble Tea's raw-mode keyboard reader from being inherited by docker.
- `--init` — install tini as PID 1 so SIGTERM is forwarded to claude and zombie processes are reaped.
- `--cidfile <tmp>/ralph-<unique>.cid` — capture the container ID so `Terminate()` can call `docker kill <cid>` rather than signaling the host docker CLI process (which would orphan the container). The cidfile path is unique per step under `os.TempDir()`.
- `-u <UID>:<GID>` — run as the invoking host user. Files written to the bind-mounted repo are owned by the host user, so subsequent shell steps (`git`, `gh`) work without permission errors.
- `--mount type=bind,source=<PROJECT_DIR>,target=/home/agent/workspace` — bind-mount the target repo. `<PROJECT_DIR>` is the `--project-dir` value (default: `os.Getwd()` + `filepath.EvalSymlinks`). The `--mount` syntax is used instead of `-v` to avoid argument injection via colons in path names. The workflow bundle (`<WORKFLOW_DIR>`) is NOT mounted.
- `--mount type=bind,source=<PROFILE_DIR>,target=/home/agent/.claude` — bind-mount the Claude profile (read-write so OAuth token refresh works).
- `-w /home/agent/workspace` — explicit working directory; matches the bind-mount so relative paths inside the container correspond to real host paths.
- `-e CLAUDE_CONFIG_DIR=/home/agent/.claude` — set inside the container regardless of whether the host had `CLAUDE_CONFIG_DIR`; points to the mount point.
- `--output-format stream-json` — instructs claude to emit newline-delimited JSON (NDJSON) on stdout. Required for the `claudestream` pipeline to parse typed events.
- `--verbose` — includes all event types in the NDJSON stream (assistant turns, tool calls, result). Without this flag, many event types are suppressed and the pipeline cannot render step progress or extract the result.
- No `-t` (TTY) — deliberately omitted; a TTY corrupts line-buffered stdout that the capture layer depends on.
- No `--network` — left as default bridge; network isolation is a non-goal in v1.

## Key Files

| File | Purpose |
|------|---------|
| `src/internal/sandbox/command.go` | `BuildRunArgs` (constructs the `docker run` argv) |
| `src/internal/sandbox/image.go` | `BuiltinEnvAllowlist` (the five sandbox-plumbing vars), `ImageTag`, container path constants |
| `src/internal/sandbox/cidfile.go` | `Path()` (cidfile reservation), `Cleanup()` (ENOENT-tolerant removal) |
| `src/internal/sandbox/terminator.go` | `NewTerminator` — returns a closure that calls `docker kill --signal=TERM|KILL <cid>` |
| `src/internal/sandbox/command_test.go` | Tests for `BuildRunArgs`, env allowlist merging |
| `src/internal/sandbox/cidfile_test.go` | Tests for `Cleanup` (ENOENT tolerance) |
| `src/internal/workflow/run.go` | `buildStep` reads `stepFile.Env`, calls `sandbox.BuildRunArgs`, returns `ResolvedStep` with `CidfilePath` |
| `src/internal/workflow/workflow.go` | `RunSandboxedStep` — installs terminator, provides empty stdin, delegates to `runCommand` |

## Core Types

```go
// sandbox package

// BuiltinEnvAllowlist is the fixed set of host environment variable names
// pr9k always attempts to pass into the sandbox. Each name is passed
// with -e <NAME> (no value) so Docker reads it from the host env at
// container start; if the variable is unset on the host it is silently
// skipped (os.LookupEnv skip-if-unset behavior).
var BuiltinEnvAllowlist = []string{
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_BASE_URL",
    "HTTPS_PROXY",
    "HTTP_PROXY",
    "NO_PROXY",
}

// cidfile package-level functions (cidfile.go)

// Path reserves a unique, non-existent path for `docker run --cidfile`.
// Creates a temp file, captures its name, then removes it so the path
// is reserved but does not exist (docker requires --cidfile to be absent).
func Path() (string, error) { ... }

// Cleanup removes the cidfile at path. ENOENT is tolerated (the file
// may not exist if docker run failed before the container started).
func Cleanup(path string) error { ... }
```

```go
// workflow package

// SandboxOptions carries sandbox-specific parameters for RunSandboxedStep.
type SandboxOptions struct {
    // Terminator, when non-nil, is called by Runner.Terminate() instead of
    // signaling the host process. Receives SIGTERM first; if the process
    // does not exit within the grace period, receives SIGKILL.
    Terminator  func(syscall.Signal) error
    // CidfilePath is the --cidfile path to clean up after the step exits.
    // May be empty. Cleanup is ENOENT-tolerant.
    CidfilePath string
    // ArtifactPath is the path for the per-step .jsonl file (D14). When
    // non-empty and CaptureMode == CaptureResult, a RawWriter is opened here.
    ArtifactPath string
    // CaptureMode selects the capture semantics for the step. CaptureResult
    // activates the claudestream pipeline. Zero value (CaptureLastLine)
    // preserves current non-pipeline behaviour.
    CaptureMode ui.CaptureMode
}
```

## Environment Injection Behavior

Three sources of env vars are combined for every claude step:

1. **Builtin set** (`sandbox.BuiltinEnvAllowlist`): `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL`, `HTTPS_PROXY`, `HTTP_PROXY`, `NO_PROXY`. Always attempted; silently skipped if unset on host.
2. **User `env` field** (`StepFile.Env`): names listed in the top-level `env` array in `config.json`. Merged at build time via `append(sandbox.BuiltinEnvAllowlist, stepFile.Env...)`. Each name is passed as `-e NAME` (no `=VALUE`) so Docker reads the value from the host at container start; unset names are silently skipped.
3. **User `containerEnv` field** (`StepFile.ContainerEnv`): a key→value map in the top-level `containerEnv` object of `config.json`. Each entry is injected as `-e KEY=VALUE` with a literal value — the host environment is not consulted. Entries are emitted in **sorted key order** (deterministic argv) **after** the allowlist entries, so Docker's last-wins rule means `containerEnv` beats a same-named host passthrough.

```json
{
  "env": ["ANTHROPIC_API_KEY"],
  "containerEnv": {
    "GOPATH":     "/home/agent/workspace/.ralph-cache/go",
    "GOCACHE":    "/home/agent/workspace/.ralph-cache/gocache",
    "GOMODCACHE": "/home/agent/workspace/.ralph-cache/gomodcache"
  }
}
```

`containerEnv` values are committed to the repo — do not store secrets in this field. Use the `env` passthrough for secrets that live on the host. The validator warns when a key ends in `_TOKEN`, `_KEY`, or `_SECRET`.

Variables NOT in either allowlist are invisible inside the container: `PATH`, `HOME`, `USER`, and all other host env vars are excluded by default. `CLAUDE_CONFIG_DIR` is always set to the container mount point and cannot be overridden via `containerEnv` (the validator rejects it).

## Cidfile-Driven Termination

When the user presses `q` or a SIGINT/SIGTERM arrives during a claude step, `Runner.Terminate()` is called. For sandboxed steps, it dispatches through the `currentTerminator` closure (installed by `RunSandboxedStep`) instead of signaling the host docker CLI process:

```
Runner.Terminate()
  └── reads currentTerminator (not nil for sandboxed steps)
  └── calls terminator(SIGTERM)
        └── docker kill --signal=TERM <container-id>   ← reads cidfile
  └── waits up to 3 seconds
  └── if still running: calls terminator(SIGKILL)
        └── docker kill --signal=KILL <container-id>
```

The terminator is constructed by `sandbox.NewTerminator(cmd, cidfilePath)`. It polls the cidfile for up to 2 seconds at 50ms intervals (the file is written by the Docker daemon shortly after the container starts). If the cidfile never appears, it falls back to signaling the host `docker run` CLI process directly.

`currentTerminator` is cleared before `procDone` is closed, preventing stale signal dispatch if `Terminate()` races with natural step completion.

## What the Sandbox Does NOT Protect Against

The following residual risks are accepted:

| Risk | Why accepted |
|------|-------------|
| Claude corrupts `~/.claude/.credentials.json` | Profile is mounted read-write (required for OAuth token refresh). Re-authenticating is cheap. |
| Malicious upstream `docker/sandbox-templates:claude-code` push | Same trust assumption as installing `@anthropic-ai/claude-code` globally. Tag-based pulls deliberately trade pinning for upgrade ergonomics. |
| Prompts instruct claude to write malicious content inside the repo | The repo mount is read-write; semantic corruption inside the repo remains a risk. Git history is the backstop. |
| claude-code's agent loop hallucinates alternatives when host tools are absent | The guarantee is "no host tool access", not "no semantic regression in the commit." |
| Network exfiltration | Network isolation is a non-goal in v1; claude needs outbound HTTPS to the Anthropic API. |

## Error Handling

| Scenario | Behavior |
|----------|----------|
| `docker` not on `PATH` | Preflight exits 1 before the TUI starts |
| Docker daemon not running | Preflight exits 1 before the TUI starts |
| Sandbox image missing | Preflight exits 1 with message `Run: pr9k sandbox create` |
| `docker run` exits non-zero | `RunSandboxedStep` returns the error; orchestrator enters error mode |
| Cidfile never appears (container crashed instantly) | Terminator falls back to signaling host docker CLI |
| Cidfile cleanup fails (ENOENT) | Silently ignored — file may not exist if docker run failed before start |

## Additional Information

- [Setting Up Docker Sandbox](../how-to/setting-up-docker-sandbox.md) — User-facing setup guide: install Docker, run `sandbox create`, authenticate via `sandbox login`
- [sandbox Subcommand](sandbox-subcommand.md) — `pr9k sandbox create` and `pr9k sandbox login` implementations: Docker check, image pull, smoke test, interactive login flow
- [Preflight](../code-packages/preflight.md) — Startup checks that reject a missing Docker daemon or sandbox image before the TUI starts
- [Subprocess Execution & Streaming](subprocess-execution.md) — `RunSandboxedStep`, `SandboxOptions`, terminator lifecycle, cidfile cleanup
- [Config Validation](../code-packages/validator.md) — Sandbox rules B and C (prompt-token ban, captureAs+tokens-in-command; Rule A removed in issue #91)
- [Step Definitions & Prompt Building](../code-packages/steps.md) — `StepFile.Env` field and `BuildRunArgs` call site in `buildStep`
- [Passing Environment Variables](../how-to/passing-environment-variables.md) — User-facing guide for declaring env vars in `config.json`
- [Variable Output & Injection](../how-to/variable-output-and-injection.md) — Why `{{WORKFLOW_DIR}}` and `{{PROJECT_DIR}}` are banned in prompt files
- [ADR: Require Docker Sandbox](../adr/20260413160000-require-docker-sandbox.md) — Decision to make Docker a runtime requirement
- [ADR: workflow-dir / project-dir split](../adr/20260413162428-workflow-project-dir-split.md) — Why `PROJECT_DIR` means the target repo (not the workflow bundle)
- [Architecture Overview](../architecture.md) — System-level view including the sandbox layer
