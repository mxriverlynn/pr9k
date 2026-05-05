# Tini WARN at start of every claude step

## Problem Statement

**Symptom.** Every claude step in pr9k prints these stderr lines just before claude itself starts:
```
[stderr] [WARN  tini (7)] Tini is not running as PID 1 and isn't registered as a child subreaper.
[stderr] Zombie processes will not be re-parented to Tini, so zombie reaping won't work.
[stderr] To fix the problem, use the -s option or set the environment variable TINI_SUBREAPER to register Tini as a child subreaper, or run Tini as PID 1.
```

**Expected behavior.** No tini warning. The container's init handling is either correct (tini at PID 1) or silent (tini is gone).

**Conditions.** Every claude step (`BuildRunArgs`), every interactive sandbox session (`BuildInteractiveArgs`), and every sandbox shell (`BuildShellArgs`). The warning is in three command builders, but the underlying cause is shared.

**Impact.** Cosmetic — log pollution. Each claude step's `.jsonl` and TUI log gets three garbage stderr lines. Functionally harmless: the docker-injected tini at PID 1 *does* reap zombies correctly, the image's redundant tini at PID 7 does not but is also not the one that needs to.

## Evidence Summary

- **E1** — Image ENTRYPOINT is already tini. `docker inspect docker/sandbox-templates:claude-code` returns:
  ```json
  Entrypoint: ["tini","--"]
  Cmd: ["claude","--dangerously-skip-permissions"]
  ```
  So unmodified, the image runs `tini -- claude ...` as PID 1.

- **E2** — pr9k passes `--init` to every docker run invocation. `src/internal/sandbox/command.go:40` (`BuildRunArgs`), `:111` (`BuildInteractiveArgs`), `:139` (`BuildShellArgs`):
  ```go
  args := []string{
      "docker", "run",
      "--rm",
      "-i",
      "--init",
      ...
  }
  ```
  Docker's `--init` injects `/sbin/docker-init` (which is tini) as PID 1 inside the container, then execs the image's ENTRYPOINT as a child.

- **E3** — Layering produces "tini at PID 7". With `--init` + image ENTRYPOINT `["tini", "--"]`, the process tree inside the container is:
  - PID 1: `docker-init` (Docker's tini)
  - PID 7: `tini --` (image's tini)
  - PID 8+: `claude ...`

  The image's inner tini detects it is not PID 1 and is not registered as a subreaper via `prctl(PR_SET_CHILD_SUBREAPER)` — hence the WARN at startup.

- **E4** — Documentation explicitly justifies `--init` as zombie reaping. `docs/features/docker-sandbox.md:87`:
  > `--init` — install tini as PID 1 so SIGTERM is forwarded to claude and zombie processes are reaped.

  This rationale is correct for the *outer* tini, but redundant given the image already has its own tini ENTRYPOINT (E1). The doc was written without knowledge of the image's ENTRYPOINT.

- **E5** — Three places hard-code `--init` and three tests assert it. Tests:
  - `src/internal/sandbox/command_test.go:51` `assertContainsFlag(t, args, "--init")`
  - `src/internal/sandbox/command_test.go:185` `expectedPrefixOrder := []string{"docker", "run", "--rm", "-i", "--init", "--cidfile"}`
  - `src/internal/sandbox/command_test.go:479` `wantFlags := []string{"-it", "--rm", "--init"}` (interactive)
  - `src/internal/sandbox/command_test.go:637` `wantFlags := []string{"-it", "--rm", "--init"}` (shell)
  - `src/cmd/pr9k/sandbox_interactive_test.go:310` and `src/cmd/pr9k/sandbox_shell_test.go:219` assert `--init` in command shape

  All tests will need to be updated to assert `--init` is **absent** (or just stop asserting it).

- **E6** — Termination wiring is PID-1-agnostic on the host side. `src/internal/sandbox/terminator.go` calls `docker kill --signal=<SIG> <cid>` against the container ID; the daemon delivers the signal to whichever process is PID 1 inside the container. Both docker-init and the image's tini are tini-class init processes with equivalent SIGTERM-forwarding-and-zombie-reaping semantics, so removing `--init` does not regress termination behavior.

- **E7** — Sibling docs `docs/features/docker-sandbox.md`, `docs/features/sandbox-subcommand.md`, `docs/how-to/setting-up-docker-sandbox.md`, `docs/code-packages/sandbox.md` all reference `--init` in the docker run shape. They will need touch-ups consistent with the fix.

## Root Cause Analysis

The image `docker/sandbox-templates:claude-code` already has `tini --` as its ENTRYPOINT (E1). pr9k additionally passes `--init` to `docker run` (E2), which injects Docker's own tini at PID 1. The result is two tinis in series: docker-init at PID 1, the image's tini at PID 7 (E3). The inner tini correctly detects it is neither PID 1 nor registered as a subreaper and emits the WARN. The outer tini fully covers signal forwarding and zombie reaping, so the inner tini is redundant; nothing relies on it being present beyond its own existence in the image.

## Coding Standards Reference

- **`docs/coding-standards/documentation.md`** — Feature docs ship with the feature; updating CLAUDE.md when adding new doc files; keeping doc code blocks consistent with production code. Applies to docs touch-ups for `--init` removal across `docs/features/docker-sandbox.md`, `docs/features/sandbox-subcommand.md`, `docs/how-to/setting-up-docker-sandbox.md`, `docs/code-packages/sandbox.md`, `docs/audits/new-user-stress-test.md`.
- **`docs/coding-standards/testing.md`** — Race detector required (already in `make test`). Test changes here are signature-level argv assertions — no concurrency concerns.
- **ADR `docs/adr/20260413160000-require-docker-sandbox.md`** — Docker is a required runtime for claude steps. The fix preserves this; no ADR change needed.
- **Inferred from code** — `BuildRunArgs` is a pure argv builder with no side effects; tests assert argv shape verbatim. The fix follows the same shape.

## Planned Fix

**One-sentence summary.** Remove `--init` from all three docker run argv builders so the image's existing `tini --` ENTRYPOINT runs as PID 1, eliminating the redundant inner tini and its WARN.

### File 1: `src/internal/sandbox/command.go`

**Change.** Remove the `"--init",` line from `BuildRunArgs` (line 40), `BuildInteractiveArgs` (line 111), and `BuildShellArgs` (line 139). Update the godoc on `BuildRunArgs` if it mentions `--init` (it does not currently).

- Justified by **E1, E2, E3, E6**.

### File 2: `src/internal/sandbox/command_test.go`

**Change.** Remove `--init` from the four assertions cited in E5:
- Line 51: drop the `assertContainsFlag(t, args, "--init")` call (or change to assertNotContainsFlag).
- Line 185: remove `"--init"` from `expectedPrefixOrder`.
- Lines 479, 637: remove `"--init"` from `wantFlags` slices.

Add positive regression tests asserting `--init` is **absent** for ALL THREE builders (mandatory, not optional — V7) so re-introduction in any one site is caught:

```go
func TestBuildRunArgs_NoDoubleInit(t *testing.T)         { /* assert no "--init" in argv */ }
func TestBuildInteractiveArgs_NoDoubleInit(t *testing.T) { /* same */ }
func TestBuildShellArgs_NoDoubleInit(t *testing.T)       { /* same */ }
```

### File 3: `src/cmd/pr9k/sandbox_interactive_test.go` and `src/cmd/pr9k/sandbox_shell_test.go`

**Change.** Remove `"--init",` from the expected argv slices at lines 310 and 219 respectively.

### File 4: Documentation

**Change.** Remove `--init` references from:
- `docs/features/docker-sandbox.md:54` (argv example), `:87` (`--init` justification bullet — replace with a one-liner explaining the image's ENTRYPOINT handles tini).
- `docs/features/sandbox-subcommand.md:165, 186` (example commands).
- `docs/how-to/setting-up-docker-sandbox.md:133` (example command — but this is a user-facing doc, see Open Question 1 below).
- `docs/code-packages/sandbox.md:68, 148` (package contract).
- `docs/audits/new-user-stress-test.md:135` — leave alone (audit log of a past observation, not a current claim).

### What this fix does NOT change

- The image. We're not modifying the image's ENTRYPOINT — we're stopping the duplicate.
- Termination behavior. Docker still forwards SIGTERM to PID 1, which is now the image's tini, which forwards to claude. Same behavior as before.
- Any other docker run flags.

## Alternatives Considered (and Rejected)

- **Set `TINI_SUBREAPER=1` env var** — silences the WARN but leaves the redundant inner tini and the wrong subreaper semantics. Treats the symptom, not the cause.
- **Pass `-s` to inner tini** — would require overriding ENTRYPOINT, which has the same effect as just removing `--init` (the cleaner option) and is more invasive.
- **Remove tini from the image** — image is upstream (`docker/sandbox-templates`), not under our control. Even if it were, the image is reasonable on its own; the redundancy is on our side.

## Validation

Will dispatch adversarial-validator before implementation. Expected challenges:

1. **Does removing `--init` regress signal forwarding?** No — image's tini handles SIGTERM identically (same binary, same default behavior).
2. **Could the image's ENTRYPOINT change in a future image update, breaking us?** Possible. The fix is robust if ENTRYPOINT remains tini-or-equivalent. Worth noting as remaining risk.
3. **Does any sandbox flow rely on docker-init's specific behavior?** Not based on E6 — `terminator.go` interacts with the docker daemon via cidfile, not with PID 1 directly.

## Adjustments After Validation

- **V3:** Rephrased E6 from "it's the same binary" to "tini-class init processes with equivalent semantics" — same conclusion, more accurate phrasing.
- **V7:** Made the `NoDoubleInit` regression test mandatory for all three builders (was parenthetical).
- **V9:** Image is referenced by floating tag `docker/sandbox-templates:claude-code` with no SHA pin. If a future image push changes ENTRYPOINT away from tini, claude/bash would run as PID 1 with no signal forwarder and the fix would silently regress. Documented as the top remaining risk; mitigation (digest pinning or a startup ENTRYPOINT check) is out of scope for this fix but worth a follow-up.

## Final Summary

- **Root cause:** The image `docker/sandbox-templates:claude-code` already has `tini --` as its ENTRYPOINT, and pr9k passes `--init` to `docker run`, so Docker injects its own tini at PID 1 and the image's tini ends up as a child at PID 7, where it correctly detects it isn't PID 1 and emits the WARN.
- **Fix:** Remove `--init` from `BuildRunArgs`, `BuildInteractiveArgs`, and `BuildShellArgs`; the image's tini becomes PID 1 with no behavior change. Update tests and docs to match.
- **Why correct:** `docker inspect` confirms the image's ENTRYPOINT is tini (E1); `terminator.go` uses `docker kill` against the container ID and is PID-1-agnostic on the host side (E6, V1); ENTRYPOINT semantics mean trailing `bash`/`claude` argv override CMD, not ENTRYPOINT (V2).
- **Validation outcome:** Validator confirmed termination, ENTRYPOINT/CMD semantics, test enumeration, doc enumeration, and absence of hidden docker-init features. Two minor plan-text tightenings applied; image-pin remains the top open risk.
- **Remaining risks:** Floating image tag (no SHA pin) means a future image change could leave the container with no init; no CI guard exists. Plan does not pin the image (out of scope).

## Open Questions

1. The `setting-up-docker-sandbox.md` how-to is a user-facing copy-paste recipe for `pr9k sandbox --interactive`. The fix updates this to drop `--init` from the example so it matches the binary's actual argv.
