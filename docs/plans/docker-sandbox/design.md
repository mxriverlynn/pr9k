# Docker Sandbox for Claude — Design Plan

Status: **Design — not implemented**.
Target ralph-tui version: **0.3.0** (breaking change — `y` bump from `0.2.2` per `docs/coding-standards/versioning.md`).

## 1. Overview

Today, ralph-tui invokes `claude` as a direct subprocess of the host
(`ralph-tui/internal/workflow/run.go:243`), sharing the host's filesystem,
environment, and process namespace. Because ralph-tui runs claude with
`--permission-mode bypassPermissions` inside an unattended loop, any
hallucinated destructive action has the full blast radius of the invoking user.

This plan moves every claude invocation into an ephemeral Docker container
built from the `docker/sandbox-templates:claude-code` image, bind-mounting only
the target repo and the Claude profile directory into the container.
Everything else on the host — other repos, `~/.ssh`, `~/.aws`, cached
credentials, arbitrary env vars — becomes invisible to claude.

A new `ralph-tui create-sandbox` subcommand pulls the image and verifies the
host can launch containers under its own UID. At startup, ralph-tui refuses to
run until the sandbox is present, Docker is reachable, and the Claude profile
directory exists.

## 2. Goal & Non-Goals

### Goal
Contain claude's blast radius to **(a) the bind-mounted target repo** and
**(c) a scrubbed process environment** — filesystem isolation plus host
isolation. The user owns the repo; if claude corrupts files inside it, that is
recoverable through git. Files, secrets, and other repos outside the mount are
invisible to claude.

### Non-goals
- **Network isolation.** Claude needs outbound HTTPS to the Anthropic API,
  and prompts may reach public registries or GitHub. Restricting egress is
  a separate, larger project and is deferred.
- **Parallelism.** Running multiple ralph loops concurrently is out of scope.
  The sandbox design neither enables it nor blocks it; conflicts over the
  target repo remain the user's problem.
- **Reproducibility of the claude-code runtime.** We pull by tag, not by
  digest (see §4.10). Users accept Docker's upstream as the source of truth
  for claude-code versions.
- **Sandboxing non-claude steps.** Shell steps (`scripts/close_gh_issue`,
  `scripts/get_commit_sha`, `git push`, etc.) continue to run directly on the
  host. They need host `gh`/`git` credentials and are a different threat
  class.

## 3. Threat Model

### In scope
| Attack                                            | Mitigated by                                     |
|---------------------------------------------------|--------------------------------------------------|
| Claude `rm -rf`s outside the repo                 | No bind mount outside `<project-dir>`            |
| Claude reads `~/.ssh`, `~/.aws`, other repos      | Not bind-mounted; container sees empty `$HOME`   |
| Claude exfiltrates host env vars                  | `-e` allowlist; no `-e $(env)`                   |
| Upstream tool on host is subverted by claude work | Claude cannot invoke host binaries               |

### Accepted residual risk
| Risk                                              | Why accepted                                     |
|---------------------------------------------------|--------------------------------------------------|
| Claude corrupts `~/.claude/.credentials.json`     | Profile is mounted read-write; user already runs `bypassPermissions` against far more sensitive files in the repo itself. Re-authenticating is cheap. |
| Malicious upstream `docker/sandbox-templates:claude-code` push | Same trust assumption as `npm i -g @anthropic-ai/claude-code` today. Tag-based pulls deliberately trade pinning for upgrade ergonomics. |
| Prompts tell claude to shell out to `gh`, `curl`, etc. | Image bundles a minimal tool set. Calls fail inside the container. **Note:** claude-code's agent loop may observe `command not found` and fall back to other approaches (e.g., hand-rolled HTTPS if `curl` is present, or simply writing something plausible to the repo). The sandbox does not guarantee a visible step error — the guarantee is "no host tool access", not "no semantic regression in the commit." The repo mount is read-write, so semantic corruption inside the repo remains a risk; git history is the backstop. |

## 4. Architectural Decisions

Each decision was resolved one-by-one during design review. Numbered for
reference.

1. **Goal shape: filesystem + host isolation (a+c).** Network isolation and
   parallelism deferred (§2).
2. **Approach: raw `docker run` against the official image.** Not the `sbx`
   CLI (adds an install dependency with humans-first defaults); not a custom
   Dockerfile (added maintenance burden for no current benefit). Raw
   `docker run` gives us full flag control with zero new dependencies.
3. **Profile directory mount: `$CLAUDE_CONFIG_DIR` or `$HOME/.claude`.**
   Resolved at ralph-tui startup to an absolute path. Bind-mounted to
   `/home/agent/.claude` inside the container. Read-write (see §4.4).
4. **Profile mount mode: read-write (`:rw`).** Claude needs to refresh OAuth
   tokens mid-run; read-only would cause silent auth failures in long
   unattended loops. Credential corruption is an accepted residual risk — the
   user is already running bypassPermissions against unmerged work in the
   repo, which is usually more sensitive than a refreshable credentials file.
5. **Sandbox is unconditional.** No `--no-sandbox` flag, no per-step opt-out,
   no auto-detect. Safety requirements that are opt-out accidentally become
   opt-out the day you need them on.
6. **`ralph-tui create-sandbox` subcommand.** Explicit setup action that
   produces the artifacts ralph-tui requires at startup. Without this command
   a user would have to run `docker pull` manually, which is worse UX.
7. **`create-sandbox` produces: a local copy of the tagged image, smoke-tested
   under the invoking user's UID.** Not a long-lived named container (that
   accumulates cross-iteration state and defeats the fresh-environment
   property of sandboxing). Not a custom-built image (§4.2).
8. **Docker command location: hardcoded in Go at the same site as today's
   claude invocation.** The narrow-reading-principle ADR
   ([docs/adr/20260410170952-narrow-reading-principle.md](../../adr/20260410170952-narrow-reading-principle.md))
   already tolerates the existing hardcoded claude command here; we extend
   that slice rather than refactoring the step abstraction in the same PR.
   Moving claude-step resolution into `ralph-steps.json` proper is a
   legitimate later refactor that should be done separately.
9. **UID/GID mapping: `-u $(id -u):$(id -g)`.** Files claude writes into the
   bind-mounted repo are owned by the host user, so subsequent shell steps
   (git, gh) and cleanup behave the same whether claude ran directly or in a
   container. Verified working for this image via smoke test (§11).
10. **Image reference: tag only.** `docker/sandbox-templates:claude-code`, not
    pinned by digest. Users get upstream updates by re-running
    `ralph-tui create-sandbox --force`. Trade-off: less reproducibility, more
    trust in the upstream image — the user chose this cadence.
11. **Environment passthrough: layered allowlist.** Ralph-tui always attempts
    to pass five "sandbox-plumbing" variables (see §5). A new top-level `env`
    field in `ralph-steps.json` lets workflows extend the list with
    workflow-specific variables (e.g., `GITHUB_TOKEN`, `AWS_ACCESS_KEY_ID`).
    Exact names only — no glob or prefix wildcards in v1.
12. **Lifecycle: `--rm --init --cidfile <tmp>` with `-i`, no `-t`.** `--rm`
    auto-cleans the container. `--init` installs tini as PID 1 so SIGTERM is
    forwarded and reaped correctly. `--cidfile` captures the container ID so
    we can `docker kill` on quit. No `-t` because a TTY corrupts line-buffered
    stdout that the capture layer depends on.
13. **Termination: `docker kill` via cidfile, SIGTERM → 3s grace → SIGKILL.**
    Ralph-tui's existing `Runner.Terminate()` logic is preserved but driven
    through a new `terminator` closure stored on the Runner, so
    sandbox-aware termination does not leak into the orchestration layer.
    Escalation ownership is precise: the closure performs the TERM step
    *and* the KILL step (both via `docker kill --signal=TERM|KILL <cid>`);
    `Runner.Terminate()` keeps the existing 3-second grace timer and
    decides *when* to escalate by invoking the closure twice — once for
    TERM, once for KILL after the grace expires. This matters because
    signaling the host `docker` CLI with `proc.Kill()` (today's fallback
    path in `workflow.go:101`) would only kill the CLI process and orphan
    the running container; escalation must go through `docker kill` to
    hit the container itself. See §9 for the `Runner` struct changes.
14. **Testing: unit-test command builder + unit-test preflight with injected
    prober + manual smoke checklist.** No CI integration tests against real
    Docker.

## 5. The Runtime Docker Command

The template below is the source of truth for the command ralph-tui
constructs for every claude step. Substituted values in `<ANGLE_BRACKETS>`.

```
docker run                                              \
  --rm                                                  \
  -i                                                    \
  --init                                                \
  --cidfile <TMP>/ralph-<UNIQUE>.cid                    \
  -u <UID>:<GID>                                        \
  -v <PROJECT_DIR>:/home/agent/workspace                \
  -v <PROFILE_DIR>:/home/agent/.claude                  \
  -w /home/agent/workspace                              \
  -e CLAUDE_CONFIG_DIR=/home/agent/.claude              \
  [-e ANTHROPIC_API_KEY]                                \
  [-e ANTHROPIC_BASE_URL]                               \
  [-e HTTPS_PROXY]                                      \
  [-e HTTP_PROXY]                                       \
  [-e NO_PROXY]                                         \
  [-e <EACH_ENTRY_FROM_RALPH_STEPS_JSON_ENV>]           \
  docker/sandbox-templates:claude-code                  \
  claude --permission-mode bypassPermissions            \
         --model <MODEL>                                \
         -p <PROMPT>
```

### Flag rationale (flag-by-flag)

- `--rm`: container is ephemeral — one per claude step, deleted on exit.
- `-i`: attach stdin. Only safe when ralph-tui explicitly hands docker an
  empty stdin — otherwise docker inherits the parent's stdin FD. Bubble
  Tea already reads `os.Stdin` in raw mode for keyboard handling, and a
  second reader on the same TTY causes lost keystrokes and a wedged TUI.
  The sandbox caller MUST set `cmd.Stdin = bytes.NewReader(nil)` (or
  `os.DevNull`) before `cmd.Start()`. Code-change inventory calls this
  out in `run.go` / `workflow.go`. If there is no concrete evidence that
  omitting `-i` causes stdout truncation in this codepath (adversarial
  review noted the prior claim was not citation-backed), the Plan B is
  to drop `-i` entirely — both options are acceptable; the important
  invariant is "docker never shares a TTY with Bubble Tea." The plan
  defaults to `-i` + explicit empty stdin for symmetry with Docker's
  common usage, but implementers MAY drop `-i` if the no-stdin path
  proves cleaner.
- `--init`: install tini as PID 1 inside the container. Ensures SIGTERM
  actually gets to claude and zombies get reaped. Zero downside.
- `--cidfile <tmp>/ralph-<unique>.cid`: write the container ID to this file
  immediately. `Runner.Terminate()` reads it to issue a real `docker kill`
  rather than signaling the docker CLI (which would orphan the container).
  The cidfile path must not already exist when `docker run` starts; ralph-tui
  generates a unique path per step under `os.TempDir()` and removes it after
  `cmd.Wait()` returns. Cleanup runs in the same `defer` that clears
  `currentProc` / `currentTerminator`, so it also fires on panic, on
  Terminate()-driven exits, and when `docker run` fails before the
  container starts (cidfile may or may not exist — `os.Remove` + ignore
  ENOENT).
- `-u <UID>:<GID>`: run as the host user. Files written to the bind-mounted
  repo come out owned by you.
- `-v <PROJECT_DIR>:/home/agent/workspace`: bind-mount the target repo.
  Matches the image's default `WORKDIR`, so relative paths claude resolves
  inside the container correspond to real host paths.
- `-v <PROFILE_DIR>:/home/agent/.claude`: bind-mount the Claude profile.
  `<PROFILE_DIR>` is `$CLAUDE_CONFIG_DIR` if set, else `$HOME/.claude`.
- `-w /home/agent/workspace`: explicit working directory (redundant with
  image default but defensive against future image changes).
- `-e CLAUDE_CONFIG_DIR=/home/agent/.claude`: tell claude-code inside the
  container where its profile lives. Set inside the container regardless of
  whether the host had `CLAUDE_CONFIG_DIR` set — we do not passthrough this
  variable, we overwrite it to the mount point.
- `-e <NAME>` (no value): pass through host env var `NAME` if set, skip
  otherwise. Implemented as `if v, ok := os.LookupEnv(name); ok { args = append(args, "-e", name) }`.
  Built-in set: `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL`, `HTTPS_PROXY`,
  `HTTP_PROXY`, `NO_PROXY`. User-extended from `ralph-steps.json`.
- `docker/sandbox-templates:claude-code`: tag-only image reference.
- `claude --permission-mode bypassPermissions --model <MODEL> -p <PROMPT>`:
  since we replace the image's default CMD (`claude --dangerously-skip-permissions`),
  we re-add the permission flag explicitly.

### What the command does NOT include
- `-t` (TTY): deliberately omitted — breaks line-buffered capture.
- `--network`: left as default bridge. See §2 non-goals.
- `-e HOME=...`: leave as the container's default (`/home/agent`).
  The image creates the agent user with a writable home.
- Resource limits (`--cpus`, `--memory`): not set in v1.

## 6. `create-sandbox` Subcommand Spec

### Invocation
```
ralph-tui create-sandbox [--force]
```

### Behavior
1. **Docker reachability check**
   - If `docker` is not on `PATH` (detected via `exec.LookPath`) →
     print: `Docker is not installed. Install Docker and try again.` → exit 1.
   - Run `docker version`. Non-zero exit →
     print: `Docker is installed but the daemon isn't running. Start Docker and try again.` → exit 1.
2. **Image pull**
   - If `docker image inspect docker/sandbox-templates:claude-code` exits 0
     AND `--force` was not passed →
     print: `Image docker/sandbox-templates:claude-code already present; skipping pull (use --force to re-pull).`
   - Otherwise → `docker pull docker/sandbox-templates:claude-code`, streaming
     progress. Non-zero exit →
     print: `Failed to pull sandbox image.` + stderr → exit 1.
3. **Smoke test**
   - Run: `docker run --rm -u $(id -u):$(id -g) docker/sandbox-templates:claude-code claude --version`.
   - Capture both stdout and stderr; accept a version line from either
     (some node CLIs emit `--version` to stderr).
   - Failure cases with distinct messages so diagnostics match the cause:
     - Non-zero exit → `Sandbox smoke test failed — container exited with status <N>.` + captured stderr → exit 1.
     - Exit 0 but no output on either stream → `Sandbox smoke test failed — image ran but produced no version output. Image may be corrupted or a locally-tagged stub. Re-pull with --force.` → exit 1.
     - Exit 0 with output that does not match a semver-shaped pattern →
       `Sandbox smoke test warning — unexpected version output: <line>. Proceeding, but this image may not be the expected claude-code.` (warning, not failure — upstream output format is outside our control).
   - On success, print: `Sandbox verified: claude <version> under UID <UID>:<GID>.`
   - Note: this smoke test does NOT exercise writes with bind mounts, so
     it will not catch the "`agent` user can't write to `/home/agent/.cache`
     under host UID" failure mode — §10(d) manual checklist catches it.
4. **Done**
   - Print: `Sandbox ready.` → exit 0.

### Output shape
Structured step-by-step:
```
Checking Docker... ✓
Image present... skipping pull (use --force to re-pull)
Verifying sandbox... ✓ claude 2.1.101 under UID 501:20
Sandbox ready.
```

### Exit codes
- `0` — success.
- `1` — any failure. Remediation is printed above the exit.

## 7. Startup Preflight Spec

Runs once, at ralph-tui invocation (any command other than `create-sandbox`),
before the main orchestration loop begins.

### Sequence
1. Parse CLI flags and load `ralph-steps.json`. Existing config validation
   (D13) runs first; any config errors exit with existing behavior.
2. **Resolve profile dir**: `$CLAUDE_CONFIG_DIR` if set, else `$HOME/.claude`.
   Expand to absolute.
3. **Profile dir check**: `os.Stat(profileDir)`. On miss:
   `Claude profile directory not found: <path>. Set CLAUDE_CONFIG_DIR or create ~/.claude.` → exit 1.
   If the stat succeeds but `fi.IsDir()` is false (e.g., the path
   resolves to a regular file), emit:
   `Claude profile path is not a directory: <path>. Point CLAUDE_CONFIG_DIR at a directory.` → exit 1.
   This matches the explicit-precondition-validation standard
   (`docs/coding-standards/error-handling.md:26`) and the `os.Stat`
   behavior that succeeds on files.
4. **Docker reachability**:
   - `exec.LookPath("docker")` fails → `Docker is not installed. Install Docker and try again.` → exit 1.
   - `docker version` exits non-zero → `Docker is installed but the daemon isn't running. Start Docker and try again.` → exit 1.
5. **Sandbox image present**: `docker image inspect docker/sandbox-templates:claude-code`.
   Non-zero exit → `Claude sandbox image is missing. Run: ralph-tui create-sandbox` → exit 1.
6. **Credentials sanity (best-effort)**: if `<profileDir>/.credentials.json`
   exists and has size 0, emit a warning:
   `Warning: <path>/.credentials.json is empty. Claude will likely fail authentication. Re-authenticate with 'claude login' inside the sandbox.`
   Do NOT fail — only warn. This catches the SIGKILL-during-OAuth-refresh
   failure mode (see §13 adversarial notes). Absence of the file is not
   a warning condition (fresh profile is valid).
7. Enter main loop.

### Explicitly not done at startup
- **No smoke test at startup.** `create-sandbox` already verified the specific
  image + UID combination. Re-verifying costs a container start per
  ralph-tui invocation for marginal value.
- **No registry/network check.** If pull-time network is down, the preflight
  doesn't care — we already have the image locally.
- **No mid-run re-check.** Docker state shouldn't change during an
  iteration loop; if the daemon dies mid-run, the next `docker run` fails
  naturally and the user sees it in the normal error path.

## 8. Config Schema Change

### New top-level field: `env`

`ralph-steps.json` gains one new optional top-level entry: an array of host
environment variable names to pass through into the sandbox.

```json
{
  "env": ["GITHUB_TOKEN", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"],
  "initialize": [ ... ],
  "iteration": [
    { "name": "Feature work", "isClaude": true, "model": "sonnet", "promptFile": "feature-work.md" },
    ...
  ],
  "finalize": [ ... ]
}
```

Note: the existing top-level schema is `initialize` / `iteration` / `finalize`
(see `ralph-tui/internal/steps/steps.go:27-31` and `ralph-tui/ralph-steps.json`).
`env` is a new sibling alongside those three phase arrays — not a wrapper
around them.

### Rules
- Optional. Absent `env` → equivalent to `"env": []`.
- Type: array of strings. Each string is a host env var name.
- Exact names only (no globs, no prefixes) in v1.
- Case-sensitive (env var names are case-sensitive).
- Duplicates and overlap with the built-in set are allowed and harmless
  (docker idempotently passes a var once).
- Unknown vars (not set on the host) are silently skipped — not an error.
  This matches `-e NAME` (no value) semantics.
- `CLAUDE_CONFIG_DIR` and `HOME` must NOT appear in the user `env` list.
  Both are set/managed by the sandbox itself (`CLAUDE_CONFIG_DIR` is
  overwritten to the mount point; `HOME` is whatever the image sets).
  The validator rejects either name with a clear reason. This keeps the
  sandbox's own invariants out of reach of workflow configs.

### Validation (extensions of D13)

**`env` field:**
- `env` must be an array if present.
- Each element must be a non-empty string.
- Each element must be a valid env var name (regex `^[A-Za-z_][A-Za-z0-9_]*$`).
- Elements must not be `CLAUDE_CONFIG_DIR` or `HOME` (sandbox reserves
  these).
- Use `Category: "env"`, `Phase: "config"`, `StepName: ""`.

**`{{PROJECT_DIR}}` ban in prompts:**
- Every prompt file referenced by a claude step is scanned for the
  literal token `{{PROJECT_DIR}}`. If found, emit a validator error
  (see §13 resolved decision for rationale and message).
- Non-claude `command` steps are not scanned.

These additions bring D13's category count to ten.
- Reject anything else via the existing `validator.Error` surface
  (`ralph-tui/internal/validator/validator.go:23-37`). Use a new
  `Category: "env"` value; `Phase: "config"`; `StepName: ""` (this is a
  file-level field, not step-level).

## 9. Code Change Inventory

New and modified files:

### New packages

- **`ralph-tui/internal/sandbox/`** — sandbox-specific logic. Keeps docker
  knowledge localized.
  - `sandbox/image.go` — constants (`ImageTag = "docker/sandbox-templates:claude-code"`,
    mount paths `ContainerRepoPath`, `ContainerProfilePath`, built-in env
    allowlist).
  - `sandbox/command.go` — `BuildRunArgs(projectDir, profileDir string, uid, gid int, cidfile string, envAllowlist []string, model, prompt string) []string`.
    Pure function producing the full `docker run ...` argv. Unit-tested.
  - `sandbox/terminator.go` — given a cidfile path *and* the `*os.Process`
    for the `docker run` CLI invocation, returns a closure of the shape
    `func(signal syscall.Signal) error` that:
    1. Polls the cidfile for up to `cidfileWait` (default 2s) to allow
       the race between `docker run` start and cidfile-write to settle.
    2. If the cidfile appears and contains a 64-char hex string, runs
       `docker kill --signal=<SIGNAL> <cid>`.
    3. If the cidfile is still missing after the poll window, OR
       contains a partial write, falls back to `proc.Signal(signal)` on
       the host docker CLI process. This is the correct recovery path
       *because before the container is running there is nothing to
       orphan* — signaling the CLI aborts the launch cleanly.
    This design addresses the cidfile race: user hits `q` during image
    pull or cold-start, cidfile may not yet exist, and the closure must
    NOT no-op. The prior "nothing to kill" sentinel would orphan a
    container that starts moments after termination.
    The closure is stateless (no captured buffers) so it is safe to
    invoke from `Runner.Terminate()` without additional locking.
  - `sandbox/cidfile.go` — unique cidfile path generation (via
    `os.CreateTemp("", "ralph-*.cid")` then `os.Remove` so the path is
    available to `docker run --cidfile`; this guarantees uniqueness
    even under concurrent ralph-tui invocations and produces a loud,
    specific error if collision ever occurs) and ENOENT-tolerant
    cleanup.
- **`ralph-tui/internal/preflight/`** — startup checks.
  - `preflight/profile.go` — resolves and validates the profile dir.
  - `preflight/docker.go` — docker-binary, daemon-reachability, and
    image-presence checks. Exposes a `Prober` interface for tests.
  - `preflight/run.go` — orchestrates the sequence and prints messages.

### Modified files

- **`ralph-tui/cmd/ralph-tui/main.go`** — wire in preflight before the main
  `Run()`. Register new `create-sandbox` cobra command.
- **`ralph-tui/cmd/ralph-tui/create_sandbox.go`** (new) — cobra subcommand
  implementation calling into `sandbox` + `preflight` packages.
- **`ralph-tui/internal/workflow/run.go`** — `buildStep` for `IsClaude: true`
  now calls `sandbox.BuildRunArgs(...)` instead of constructing the literal
  `["claude", ...]` slice. The resolved step also carries the cidfile
  path (so the Runner can install the terminator closure) and a flag
  telling the Runner to set `cmd.Stdin = bytes.NewReader(nil)` before
  `cmd.Start()` — required to prevent docker's `-i` from sharing the
  host TTY with Bubble Tea (§5 flag rationale).
- **`ralph-tui/internal/workflow/workflow.go`** — `Runner` gains:
  - `currentTerminator func(syscall.Signal) error` field alongside
    `currentProc`, guarded by the same `processMu` mutex that already
    protects `currentProc`, `procDone`, and `terminated`
    (`workflow.go:27-31`). Read/write sites must acquire `processMu`.
  - `SetTerminator(func(syscall.Signal) error)` called by sandboxed
    steps before `cmd.Start()`. Cleared in the same `defer` that
    clears `currentProc` (`workflow.go:144-149`), so terminator and
    process lifetimes stay matched.
  - `Terminate()` snapshots `currentTerminator` under `processMu`. If
    non-nil, the TERM and KILL paths call `terminator(syscall.SIGTERM)`
    and `terminator(syscall.SIGKILL)` instead of `proc.Signal`/`proc.Kill`.
    The existing 3-second grace timer is preserved; escalation stays
    in `Runner` and the closure remains stateless (§4.13).
- **`ralph-tui/internal/steps/steps.go`** — parse new top-level `env` field
  on `StepFile` (alongside `Initialize`/`Iteration`/`Finalize`); plumb it
  into `buildStep`'s caller chain so `sandbox.BuildRunArgs` can see the
  list. `BuildPrompt` itself does not need to change.
- **`ralph-tui/internal/validator/validator.go`** — D13 lives here
  (`Error` type at lines 23-37, `vFile` at 56-60). Extend `vFile` with
  `Env *[]string` and add a validation category for env names. Errors
  use the existing `validator.Error` type (not `ConfigError` — that name
  was shorthand; correct the spec here).
- **`ralph-tui/internal/version/version.go`** — bump `0.2.2` → `0.3.0`.

### New test files
- `ralph-tui/internal/sandbox/command_test.go` — golden argv tests (§10a).
- `ralph-tui/internal/sandbox/cidfile_test.go` — unique path generation
  and ENOENT-tolerant cleanup.
- `ralph-tui/internal/preflight/run_test.go` — preflight matrix with
  injected `Prober` (§10b).
- `ralph-tui/internal/validator/validator_test.go` — extend existing
  file with cases covering the new `env` field (valid names, invalid
  regex, non-string elements, non-array top-level value).

### Files deliberately untouched
- `ralph-tui/ralph-steps.json` — the existing default workflow gets no `env`
  entries. If a migration to add `GITHUB_TOKEN` etc. is needed for the
  default loop, that's a separate content change.
- `prompts/*.md` — unchanged.
- `scripts/*` — unchanged. These run on the host, not in the sandbox.

## 10. Testing Plan

### (a) Unit-test the command builder
`sandbox/command_test.go`:
- Golden-test the full argv slice for a representative input (verifies
  flag ordering, mount paths, env expansion, literal flag strings).
- Covers: built-in env vars present on host, absent on host, user
  allowlist overlap with built-ins, duplicate entries, empty user list.
- Boundary: prompts containing shell metacharacters, newlines, embedded
  quotes — confirming they pass through as a single argv element.

### (b) Unit-test the preflight with injected prober
`preflight/run_test.go`:
- Inject a fake `Prober` that returns configurable results for each check.
- Test matrix:
  - Profile dir missing.
  - Profile dir points to a file, not a directory.
  - Docker binary not on PATH.
  - Docker binary present but daemon unreachable.
  - Image not present locally.
  - All green.
- Verify exact user-facing message strings (locking in copy). Verify
  exit code (non-zero) signaled via returned error or sentinel.

### (c) Extend validator tests for the `env` field
`validator_test.go` gains cases covering:
- Missing `env` key → equivalent to empty list (no error).
- `env: []` → no error.
- `env: ["GITHUB_TOKEN"]` → no error.
- `env: [""]` → error (empty name).
- `env: ["1BAD"]` → error (regex violation).
- `env: [123]` → error (non-string element, reported via JSON decode).
- `env: "GITHUB_TOKEN"` → error (top-level value must be an array).
- `env: ["CLAUDE_CONFIG_DIR"]` → error (sandbox-reserved).
- `env: ["HOME"]` → error (sandbox-reserved).
- Prompt file containing `{{PROJECT_DIR}}` referenced by a claude step
  → error with exact `Problem` string from §13.
- Prompt file with no `{{PROJECT_DIR}}` → clean.
- `command` step containing `{{PROJECT_DIR}}` → clean (ban is prompt-
  only).
Include both the error category/phase/stepName shape and the exact
`Problem` string in assertions, matching the existing test style.

### (d) Manual validation checklist (in the plan, not automated)
Before merging, a human runs:
- [ ] `ralph-tui create-sandbox` on a fresh image cache succeeds and prints
      structured output.
- [ ] `ralph-tui create-sandbox` on a present image skips pull, still
      smoke-tests.
- [ ] `ralph-tui create-sandbox --force` re-pulls even if present.
- [ ] `ralph-tui -n 1` against a real issue runs claude successfully and
      writes repo files owned by host UID.
- [ ] `q` + `y` mid-claude-step terminates cleanly — `docker ps` shows no
      orphan containers; `/tmp/ralph-*.cid` files are cleaned up.
- [ ] Ctrl+C (SIGINT) produces the same clean termination.
- [ ] Running with `CLAUDE_CONFIG_DIR=~/.claude-testdouble` mounts the
      correct profile (confirmed by claude using that session).
- [ ] Running with an `env: ["GITHUB_TOKEN"]` entry and `GITHUB_TOKEN` set
      on the host — claude inside the sandbox reports the var is set
      (verified via a scratch prompt like `echo $GITHUB_TOKEN`).
- [ ] Running with `env: ["GITHUB_TOKEN"]` and `GITHUB_TOKEN` unset — no
      error, claude just doesn't see the var.
- [ ] Removing the image (`docker rmi`) and running `ralph-tui` produces
      the "sandbox missing, run create-sandbox" error and exits 1.
- [ ] Stopping the Docker daemon produces the "daemon not running" error.
- [ ] Deleting `~/.claude` produces the "profile not found" error.

### (e) Not doing
No automated integration test that hits real Docker. Value over (a)+(b)+(d)
is small; adds a CI requirement for docker-in-docker or a host daemon;
and brings network/registry flakiness into the suite. The manual checklist
catches realism gaps.

## 11. Migration & Rollout

### Breaking change
This release removes the ability to run claude without Docker. Existing
users will see a preflight error on the first run after upgrading until
they run `ralph-tui create-sandbox`.

Per `docs/coding-standards/versioning.md`, this is a `y` bump under the
`0.y.z` scheme: `0.2.2` → `0.3.0`.

**Characterization of the change**: This is schema-additive (new `env`
field is backwards-compatible for existing `ralph-steps.json`) *plus*
environment-breaking (Docker becomes a hard runtime dependency, so a
previously-valid config no longer produces the same workflow — it fails
preflight). Under the versioning standard's §2 rule ("Any existing
user's ralph-steps.json that was valid before must still be valid and
still produce the same workflow"), environment-breaking would be MAJOR
in a `1.y.z` regime. It is MINOR here *only* under the `0.y.z` escape
hatch. The first `1.0.0` release must not silently absorb this kind of
change — future breaking environment dependencies will be 2.0.0.

New user-visible surface:
- `create-sandbox` subcommand
- `env` top-level field in `ralph-steps.json`
- Three new startup error conditions with exit code 1
- Docker required as a runtime dependency

### Upgrade path
1. User updates ralph-tui to `0.3.0`.
2. First run prints: `Claude sandbox image is missing. Run: ralph-tui create-sandbox` and exits 1.
3. User runs `ralph-tui create-sandbox` — image pulls, smoke test passes.
4. Normal runs resume.

### Rollback
If sandboxed claude turns out to break a specific workflow in production,
the rollback is: pin to `ralph-tui 0.2.x` until a fix ships. No data
migration is needed — ralph-tui state lives only in the target repo.

## 12. Docs That Need Updating on Implementation

These are *followup* deliverables, not part of this plan. Each should be
authored as part of the implementation PR or a follow-up doc PR.

- **New**: `docs/features/docker-sandbox.md` — feature doc describing the
  sandbox architecture, mount layout, env allowlist, and command shape.
- **New**: `docs/adr/<timestamp>-require-docker-sandbox.md` — records the
  decision to make Docker a runtime requirement (sibling to the
  narrow-reading-principle ADR).
- **New**: `docs/how-to/setting-up-docker-sandbox.md` — user-facing setup
  guide: install Docker, run `create-sandbox`, authenticate claude profile,
  configure `CLAUDE_CONFIG_DIR` for multi-profile setups.
- **Update**: `docs/features/subprocess-execution.md` — document the new
  `Runner.currentTerminator` closure and the docker-kill-via-cidfile
  termination path.
- **Update**: `docs/features/step-definitions.md` — document the new
  top-level `env` field and the layered allowlist behavior.
- **Update**: `docs/features/config-validation.md` — document the new
  validation rules for the `env` field and the `{{PROJECT_DIR}}`-in-
  prompts ban.
- **Update**: `docs/how-to/variable-output-and-injection.md` — note
  that `{{PROJECT_DIR}}` is valid only in `command` steps, not in
  prompt files (since sandboxed claude cannot see the host path).
- **Update**: `CLAUDE.md` — add pointers to the new feature doc, ADR, and
  how-to guide under the appropriate sections.

## 13. Open Questions & Known Risks

### `{{PROJECT_DIR}}` inside claude prompts — resolved

**Decision:** Reject `{{PROJECT_DIR}}` when it appears in any prompt
file referenced by a claude step. The built-in variable remains
available for non-claude `command` steps (which continue to run on the
host and see host paths).

**Why:** The VarTable seeds `PROJECT_DIR` to the *host* absolute path
resolved from `os.Executable()` (`ralph-tui/internal/vars/vars.go:63`;
the `{{VAR}}` language is public API per
`docs/coding-standards/versioning.md:19`). Once claude runs inside a
container with the repo mounted at `/home/agent/workspace`, a prompt
containing `{{PROJECT_DIR}}/foo` would hand claude a host path that
does not exist in the container — claude then either fails visibly,
wastes tokens searching for the file, or hallucinates its contents.
Today no prompt file uses `{{PROJECT_DIR}}` (only
`ralph-tui/ralph-steps.json:3` uses it in a non-claude command, which
still works on the host), so banning it in prompts is zero-churn.
Keeps the substitution language uniform and phase-only (no
context-dependent resolution), aligned with the narrow-reading
principle ADR.

**Implementation sketch (extends §8 validation):**
- D13 validator already reads every prompt file referenced by a claude
  step (file-existence check in `validator.go`). Add a scan pass that
  rejects any occurrence of the literal token `{{PROJECT_DIR}}` in
  prompt bodies.
- New `Category: "prompt"` (or reuse existing prompt-related category)
  with `Phase: <step's phase>`, `StepName: <step name>`, and
  `Problem`: `prompt %s: {{PROJECT_DIR}} is not valid inside prompt files — it expands to a host path that does not exist inside the sandbox. Use paths relative to the workspace root (claude's cwd is the repo root inside the container).`
- Non-claude command steps are unaffected — `ResolveCommand` in
  `run.go:260-282` still substitutes the host path and commands still
  run on the host.
- Add validator test cases: prompt containing `{{PROJECT_DIR}}` →
  error; prompt without it → clean; command containing
  `{{PROJECT_DIR}}` → clean.

**Docs follow-up** (goes in §12's "docs to update" list):
- `docs/how-to/variable-output-and-injection.md` — note that
  `{{PROJECT_DIR}}` is valid only in `command` steps, not in prompt
  files.

### To verify during implementation
- **Prompt size vs. ARG_MAX.** macOS `getconf ARG_MAX` is 1,048,576 bytes
  (~1MB) as of Sonoma, but in practice argv+environ share that budget and
  long prompts plus env can push against it. Current direct-claude
  invocation already passes prompts as `-p <arg>`, so this isn't *new*, but
  adding docker wrapping + env-e flags slightly tightens the budget.
  If a prompt overflows, the failure mode is a loud `argument list too
  long` — easy to diagnose, acceptable for v1. Revisit via stdin piping if
  it bites.
- **Smoke-test realism.** §10(d)'s manual checklist includes a `-n 1` real
  run; the §6 automated `claude --version` smoke test in `create-sandbox`
  does not exercise writes. If `-u $(id -u):$(id -g)` causes filesystem write
  problems inside the image (e.g., the `agent` user can't write to
  `/home/agent/.cache`), `create-sandbox` will greenlight a sandbox that
  then fails on the first real claude step. The manual checklist catches
  this; consider promoting a lightweight write-smoke (e.g., `claude -p "echo hello > /home/agent/workspace/smoke.txt"`
  with a temp mount) into `create-sandbox` if problems recur.
- **Tini+claude-code interaction.** `--init` installs tini as PID 1. Tini
  has sensible defaults but some Node applications have been observed to
  delay SIGTERM handling; if the 3-second grace period proves too short in
  practice, widening it is a one-line change in `workflow.go`.

### Adversarial-review notes (incorporated above)
- **Stdin/TTY collision with Bubble Tea** (F1): addressed in §5 and §9
  by requiring `cmd.Stdin = bytes.NewReader(nil)` at the invocation site.
- **Cidfile race during kill window** (F2): addressed in §9
  `sandbox/terminator.go` spec — poll + CLI-signal fallback.
- **Smoke-test realism** (F3, F8): addressed in §6 with distinct error
  messages per failure cause and a non-failing warning when output
  doesn't match semver.
- **`gh`/`curl` fallback silent corruption** (F4): threat-model table
  §3 rephrased — the guarantee is "no host tool access," not "no
  semantic regression."
- **Profile rw + SIGKILL mid-refresh** (F6): §7 preflight now warns on
  zero-byte `.credentials.json`.
- **Env regex stricter than docker** (F7): accepted; §8 validation is
  intentionally stricter than the kernel accepts.
- **`bypassPermissions` still lets claude wipe its own profile** (F9):
  acknowledged here — the sandbox does not reduce credentials-file
  risk versus today's direct invocation. §3 threat table lists this as
  accepted residual.
- **Versioning characterization** (F10): §11 now distinguishes
  schema-additive from environment-breaking and calls out the
  implication for a future `1.0.0`.
- **Tini + node SIGTERM behavior** (F6 remaining risk, §13 original
  note): unchanged — the 3s grace may need widening if observed.
- **ARG_MAX under docker wrapping** (original §13 note): unchanged.
- **Digest drift between `create-sandbox` and first run** (§4.10
  tradeoff, adversarial F3 variant): accepted by §4.10's tag-only
  design; users opt into upstream cadence.
- **Image's `agent` user UID vs. host UID** (F3-5 remaining risk): not
  caught by §6 smoke (no mounts); §10(d) manual checklist catches it.

### Deferred
- **Network egress restriction** (goal b): would require custom network,
  DNS allowlist, or an egress proxy. Significant complexity for marginal
  additional safety given claude needs Anthropic API + GitHub + public
  registries. Revisit if a compelling threat emerges.
- **Refactoring claude-step resolution into `ralph-steps.json`**: the
  narrow-reading-principle ADR favors this, but doing it simultaneously
  with the sandbox change doubles the PR size. Tracked as future work.
- **Per-step sandbox customization** (different images, different env
  allowlists per step): all claude steps today share the same threat
  profile. If workflows diverge (e.g., some steps need AWS creds and
  others don't), we can add per-step `env` overrides then. Not v1.
- **Supply-chain pinning (digest-based image references)**: user chose
  tag-based pulls for upgrade ergonomics. If a supply-chain incident
  makes reproducibility more important than ease, §4.10 can be revisited.

## 14. Plan Review Summary

This plan was iteratively reviewed against the codebase plus full agent
validation (evidence-based-investigator + adversarial-validator).

### Iterations
- **Iteration 1 (correctness)** — fixed an outright schema error (§8
  example used a non-existent `"steps"` key instead of `initialize` /
  `iteration` / `finalize`); clarified termination escalation ownership
  between `Runner.Terminate()` and the new terminator closure (§4.13,
  §9); pinned the closure signature to `func(syscall.Signal) error`;
  documented mutex coverage for `currentTerminator`; surfaced
  `{{PROJECT_DIR}}` substitution semantics inside sandboxed prompts as
  an unresolved decision needing user input (§13).
- **Iteration 2 (consistency)** — corrected references to the D13
  validator file path (`internal/validator/validator.go`, not
  `internal/steps/validate.go`) and to the type name (`validator.Error`,
  not `ConfigError`); added missing test files to the §9 inventory; added
  §10(c) extending validator tests; tightened §7 profile dir check to
  also reject non-directory paths; fixed cross-references (§13 to
  §10(d) and §6, not §11 and "Q7"); renamed §10's "Not doing" to (e)
  to avoid label collision.
- **Iteration 3 (completeness)** — added prohibition on `CLAUDE_CONFIG_DIR`
  and `HOME` in user env list with validator enforcement; added the
  `env` validator category to D13's category count.
- **Stopped at iteration 3** because remaining open questions
  (image-internal assumptions, runtime behavior) were better tested by
  agent validation than by another self-review pass.

### Agent validation outcomes
- **evidence-based-investigator**: 9 of 10 plan claims verified against
  file:line evidence; 1 claim (no prompt uses `{{PROJECT_DIR}}`)
  confirmed for `prompts/` but flagged that `ralph-steps.json:3` does
  use it in a non-claude command (runs on host — out of sandbox scope).
  Confirmed there is no existing UID/GID lookup code in ralph-tui.
- **adversarial-validator**: surfaced 10 findings, of which 7 became
  plan changes:
  - F1 (HIGH): docker `-i` could share TTY with Bubble Tea — fixed in
    §5 and §9 by mandating `cmd.Stdin = bytes.NewReader(nil)`.
  - F2 (HIGH): cidfile race during the kill window could orphan
    containers — fixed in §9 by replacing the "missing cidfile = no-op"
    sentinel with poll-then-CLI-signal fallback.
  - F3, F8 (MED): smoke-test could greenlight corrupt or stub images —
    fixed in §6 with cause-specific error messages and a non-failing
    semver-shape warning.
  - F4 (MED): `gh`/`curl` fallback could silently corrupt commits —
    rephrased threat-model row in §3.
  - F6 (MED): SIGKILL during OAuth refresh could brick auth silently —
    added zero-byte `.credentials.json` warning to §7 preflight.
  - F10 (HIGH on framing): versioning characterization muddled —
    clarified in §11 that the `0.3.0` choice is correct only under the
    `0.y.z` escape hatch; equivalent change in a `1.y.z` regime would
    be MAJOR.
  - F5, F7, F9: accepted with explicit framing, not silent acceptance.

### Items resolved during review
1. **`{{PROJECT_DIR}}` semantics inside sandboxed prompts** — resolved
   by banning the token in prompt files via validator extension (see
   §8 and §13). Non-claude `command` steps continue to use the host
   path. Chosen over context-dependent remapping because no prompt
   currently uses `{{PROJECT_DIR}}`, keeping the substitution language
   pure and phase-only matches the narrow-reading-principle ADR, and
   the error surfaces at preflight rather than mid-run.

### Counts
- 3 review iterations completed.
- 10 plan-claim assumptions challenged via evidence-based agent;
  10 plan-claim assumptions challenged via adversarial agent.
- 7 adversarial findings incorporated as plan changes; 3 accepted with
  documentation update.
- 1 outright correctness bug found and fixed (§8 schema example).
- 4 cross-reference and naming inconsistencies fixed.
- 1 decision surfaced and resolved during review
  (`{{PROJECT_DIR}}` ban in prompts).
- 0 consolidations made — the plan was already well-decomposed; each
  numbered section addresses a distinct concern with low overlap.
