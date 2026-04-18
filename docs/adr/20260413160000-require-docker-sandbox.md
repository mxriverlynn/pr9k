# Require Docker as a Runtime Dependency for Claude Steps

- **Status:** accepted
- **Date Created:** 2026-04-13 16:00
- **Last Updated:** 2026-04-13
- **Authors:**
  - River Bailey (mxriverlynn, river.bailey@testdouble.com)
- **Reviewers:**

## Context

pr9k invokes `claude` with `--permission-mode bypassPermissions` in an
unattended loop. When claude runs directly on the host, any hallucinated
destructive action — `rm -rf`, overwriting credentials, reading `~/.ssh` —
has the full blast radius of the invoking user across the entire host
filesystem.

The Docker sandbox plan (`docs/plans/docker-sandbox/design.md`) was designed
to contain that blast radius to two namespaces: the bind-mounted target
repository and a scrubbed process environment. Reaching that goal requires
Docker — there is no equivalent container runtime that fits the existing
toolchain without new install dependencies.

Docker is already in widespread use by the target audience (developers who
use GitHub, Anthropic APIs, and CLI tooling), and the `docker/sandbox-templates:claude-code`
image is the image the Claude CLI's own sandboxing infrastructure uses, which
aligns the runtime with the upstream trust model.

## Decision Drivers

- **Blast radius containment.** The primary reason for the Docker requirement
  is safety: claude runs with `bypassPermissions` and without Docker there is
  no practical isolation boundary between claude's filesystem access and the
  host user's home directory.
- **Tight coupling between the feature and the requirement.** Sandboxing and
  the Docker requirement are the same feature. An opt-out (`--no-sandbox`)
  would cause the safety property to evaporate the first time a user or
  operator exercises it under pressure.
- **Existing toolchain alignment.** `docker/sandbox-templates:claude-code` is
  the image used by the Claude CLI's own sandbox infrastructure. pr9k
  extends rather than replaces that trust model.
- **No equivalent alternative without new dependencies.** Alternatives
  considered (OCI runtimes via Podman, nsjail, process namespaces via
  `unshare`, macOS Sandbox.framework) all require either additional install
  steps or platform-specific kernel capabilities — a worse tradeoff than
  requiring Docker, which the target audience already has.

## Considered Options

1. **Require Docker unconditionally.** Every claude step runs in a container.
   No opt-out flag, no per-step bypass.

   - Pros: the safety guarantee is binary and unconditional; no code path
     where the invariant is accidentally bypassed.
   - Cons: users who do not have Docker installed must install it before
     pr9k works at all; adds a startup preflight check; `sandbox create`
     subcommand is required to pull and verify the image before first use.

2. **Make Docker optional; fall back to direct invocation.**

   - Pros: pr9k works without Docker.
   - Cons: the opt-out becomes the default failure mode — users who forget to
     install Docker or whose daemon has stopped silently lose the isolation
     boundary. The safety requirement is effectively opt-in, which is worse
     than opt-out.

3. **Keep direct invocation; document the risk.**

   - Pros: zero new dependencies.
   - Cons: no isolation. The entire motivation for this ADR is that running
     claude directly on the host with `bypassPermissions` is not acceptable
     for an unattended loop. Documentation does not enforce isolation.

## Decision

Adopt **Option 1**: Docker is an unconditional runtime requirement. No
`--no-sandbox` flag, no per-step opt-out, no fallback. Rationale:

- Safety requirements that are opt-out accidentally become opt-out the first
  time they are inconvenient. An unconditional requirement keeps the invariant
  binary.
- The startup preflight (see `docs/code-packages/preflight.md`) checks for Docker
  reachability and the sandbox image before the TUI starts, so users get a
  clear actionable error rather than a mid-run failure.
- The `pr9k sandbox create` subcommand (see `docs/features/sandbox-subcommand.md`)
  provides a guided setup path that pulls the image and runs a smoke test,
  minimizing the friction of the new dependency.
- The target audience (developers running an AI-driven coding loop against a
  real git repo) already has Docker installed in the vast majority of cases.

## Consequences

**Positive:**

- Claude's blast radius is contained to the bind-mounted target repo and a
  scrubbed environment on every step, unconditionally.
- The safety guarantee is verifiable: if pr9k is running, the sandbox is
  active.
- Startup preflight catches a missing or unreachable Docker daemon before any
  workflow step runs.

**Negative:**

- Users without Docker must install it before pr9k works. `sandbox create`
  provides a guided path, but the install itself is outside pr9k's control.
- A stopped or crashed Docker daemon after startup causes the next claude step
  to fail visibly — recoverable, but disruptive to an unattended run.

**Neutral:**

- Shell command steps (`close_gh_issue`, `git push`, etc.) continue to run
  directly on the host. The Docker requirement applies only to `isClaude: true`
  steps. Shell steps need host `gh`/`git` credentials and are a different
  threat class.
- The image reference is tag-only (`docker/sandbox-templates:claude-code`, not
  pinned by digest). Users get upstream updates by re-running
  `pr9k sandbox create --force`. This trades reproducibility for upgrade
  ergonomics — the same trust model as `npm i -g @anthropic-ai/claude-code`.

## Notes

### Key Files

| File | Purpose |
|------|---------|
| `src/internal/sandbox/command.go` | `BuildRunArgs` constructs the `docker run` argv for every claude step |
| `src/internal/sandbox/terminator.go` | `NewTerminator` — `docker kill` via cidfile for clean termination |
| `src/internal/preflight/docker.go` | `CheckDocker` — startup check that Docker CLI and daemon are reachable |
| `src/internal/preflight/run.go` | `Run` — orchestrates all preflight checks and returns structured results |
| `src/cmd/src/sandbox_create.go` | `sandbox create` subcommand — guided image pull and smoke test |

### Related Docs

- [Docker Sandbox Feature Doc](../features/docker-sandbox.md) — architecture,
  mount layout, env allowlist, cidfile termination, and residual risks
- [Setting Up Docker Sandbox](../how-to/setting-up-docker-sandbox.md) — user-facing
  guide: install Docker, run `sandbox create`, authenticate profile
- [Preflight Package Doc](../code-packages/preflight.md) — startup validation
  including Docker reachability and sandbox image presence
- [Sandbox Subcommand Feature Doc](../features/sandbox-subcommand.md) — `sandbox create`
  subcommand implementation
- [Sandbox Design Plan](../plans/docker-sandbox/design.md) — full design
  rationale, threat model (§3), and architectural decisions (§4)
- [Narrow-Reading Principle ADR](20260410170952-narrow-reading-principle.md) —
  motivates keeping the Docker command hardcoded in Go rather than in
  `ralph-steps.json`
