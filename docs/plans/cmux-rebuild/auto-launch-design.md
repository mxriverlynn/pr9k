# Design: `pr9k --cmux` auto-launch

**Status:** Design only — not scheduled for implementation. Produced as the "design a real auto-launch" half of the decision on the socket-path investigation ([socket-path-misdiagnosis-investigation.md](socket-path-misdiagnosis-investigation.md)).

## Problem

A user runs `pr9k --cmux` from an ordinary terminal and expects it to "just work." Today (after the socket-path fix) they instead get an accurate but still-terminal error: `cmux mode must be launched from inside a cmux session` (cmux is not running, or it is running but the terminal is not a cmux descendant). The user's original ask was "if cmux isn't running, pr9k should start it."

The socket-path fix ended the *misdiagnosis*. This design addresses the *remaining UX gap*: turning "here's an accurate error, now go do five manual steps" into "pr9k gets you into a working cmux session."

## The hard constraint that shapes every option

cmux defaults to **descendants-only** socket access: only processes descended from the cmux session may reach the control socket (see [setting-up-cmux.md](../../how-to/setting-up-cmux.md), decision-log D13). pr9k's pane model depends on this — `RunPhase1` asks cmux (over the socket) to `surface.spawn` the four panes, and those panes are descendants *because cmux spawned them*.

Implications, established in the investigation (E9, E10, V7):

- A pr9k process that **launches** cmux is cmux's *ancestor*, not its descendant. It still cannot reach the socket in descendants-only mode. Naive "fork cmux, then connect" **does not work**.
- The only processes that can drive the socket in the default security posture are ones cmux itself spawned, or ones running inside a cmux pane.
- Therefore any real auto-launch must arrange for **a pr9k process to come into existence *inside* cmux**, not for the current pr9k process to reach into cmux from outside.

Everything below is a variation on "how does pr9k get a copy of itself running inside cmux."

## Prerequisite (already shipped)

The socket resolver from the investigation fix is a hard dependency: once a pr9k process is running inside a cmux pane, it must be able to find the socket. cmux exports `CMUX_SOCKET_PATH` into spawned panes and/or publishes the `last-socket-path` marker file; the resolver consumes both. Auto-launch is only worth building on top of correct resolution.

## Candidate approaches

### Approach A — Re-exec through the cmux CLI (recommended for prototyping)

`cmux <path>` is documented to "open a directory in a new workspace (launches cmux if needed)." cmux's own CLI can also drive workspace/surface operations. The flow:

1. `pr9k --cmux` starts in a plain terminal. Preflight resolves the socket and the dial fails with `EACCES`/not-found → pr9k detects "cmux unreachable from here."
2. Instead of erroring out, pr9k shells out to the **cmux CLI** (not the socket) to (a) ensure cmux is running and a workspace for `<projectDir>` exists, and (b) spawn a pane inside that workspace whose command is `pr9k --cmux <args> --__cmux-inner`.
3. The cmux-spawned inner `pr9k` is a descendant of cmux → its socket access succeeds → it runs the normal `RunPhase1` lifecycle.
4. The outer `pr9k` either exits with a "handed off to cmux" message, or stays as a thin supervisor that waits for the inner run and relays its exit status.

**Why this can work where naive fork cannot:** the privileged actor is cmux's *own CLI* performing a launch + spawn, not pr9k's Go process dialing the socket. The inner pr9k satisfies descendants-only by construction (same mechanism `RunPhase1` already relies on for the four panes).

**Open questions (must be validated against cmux v0.64.6 before implementing):**

- **OQ-1:** Exactly which `cmux` subcommands create/select a workspace and spawn a pane with a custom command from *outside* a session? The CLI contract lists `workspace`/`surface`/handle concepts, but the precise invocation and whether spawn-from-outside is permitted in descendants-only mode is unconfirmed. (`cmux --help`, `cmux docs api`, `cmux/cli-contract.md`.)
- **OQ-2:** Does the cmux CLI itself require descendant access for `surface.spawn`, or does the launch/`cmux <path>` path get a privileged channel? If the CLI is also descendants-gated, Approach A degrades to Approach B.
- **OQ-3:** Re-exec argument fidelity — `--project-dir`, `--workflow-dir`, env (`CLAUDE_CONFIG_DIR`, forwarded vars), and the Docker requirement must survive the hand-off into the cmux-spawned pane.
- **OQ-4:** Lifecycle/teardown ownership — if the outer process exits, who owns dismissal/teardown? Likely the inner pr9k (it already does, via `RunPhase1`), with the outer process detaching cleanly.
- **OQ-5:** Failure transparency — if the inner run fails before any pane renders, the outer process must surface a real error, not a silent exit.

### Approach B — Guided launch (instruct-and-exit, low-risk fallback)

If OQ-1/OQ-2 show the CLI cannot spawn-from-outside under descendants-only, pr9k does **not** auto-spawn. Instead it:

1. Detects "cmux installed but unreachable from here."
2. Ensures cmux is running (`cmux <projectDir>` is a pure launch, no socket control needed — this part is safe regardless of OQ-2).
3. Prints an exact, copy-pasteable next step ("a cmux window is now open; in a pane there, run: `pr9k --cmux --project-dir <abs>`") and exits 0.

This is strictly better UX than today (it starts cmux and tells the user the precise command) without depending on unconfirmed CLI capabilities. It is the safe floor and a good first increment even if Approach A is the eventual goal.

### Approach C — cmux hooks / native integration (out of scope, noted for completeness)

cmux ships a hooks/agents system (`cmux hooks`, `cmux docs agents`). A deeper integration could register pr9k as a cmux-aware agent so cmux launches it natively. This is a larger product bet, owns a cmux-version coupling surface, and is explicitly **out of scope** for this design — recorded only so it is not rediscovered later.

## Recommendation

Ship **Approach B first** (safe, immediately better UX, no unconfirmed dependencies), and treat **Approach A** as the target once OQ-1/OQ-2 are answered empirically against the pinned cmux v0.64.6. Approach A without those answers risks shipping a flow that fails in exactly the descendants-only mode that is cmux's default and pr9k's recommended posture.

Gate the whole behavior behind explicit opt-in initially (e.g. `--cmux=auto` or a config flag), defaulting to today's accurate-error behavior, so auto-launch is observable and reversible before it becomes default.

## Security & safety considerations

- **No silent process spawning by default.** Auto-launch changes pr9k from "connects to a thing you started" to "starts and re-execs itself." That is a meaningful trust change; it must be opt-in until proven, and must never run cmux with elevated privileges or alter cmux config.
- **Allow-all mode interaction.** In non-default allow-all mode the descendants constraint vanishes and even naive connect works; auto-launch logic must not assume descendants-only and must no-op cleanly when the socket is already reachable.
- **Re-exec argument injection.** The inner command line is constructed by pr9k and handed to cmux; it must be built from validated, escaped arguments (reuse the existing `SanitizeBasename`/argv-slice discipline from `cmuxctl`/`sandbox`, never a shell string).
- **Loop guard.** The inner pr9k must detect it is already inside cmux (reachable socket, or an explicit `--__cmux-inner` sentinel) and never attempt to auto-launch again — otherwise a spawn failure could fork-bomb.

## How this composes with the shipped fix

The socket-path fix is the foundation: it guarantees that *once a pr9k process is inside cmux*, it finds the socket on every platform without manual configuration (via `CMUX_SOCKET_PATH` or the marker file). Auto-launch is purely about *getting a pr9k process inside cmux*; it adds no new socket-resolution logic and inherits the resolver's correctness and the descendants-only honesty.

## Definition of done (when this is picked up)

- OQ-1..OQ-5 answered with evidence against cmux v0.64.6.
- Approach B implemented behind an opt-in flag, with the accurate-error behavior unchanged as the default.
- Loop guard + argument-escaping tests.
- `setting-up-cmux.md` updated to document the opt-in and exactly what it does/does not do (mirroring the honesty section in the investigation).
- A decision recorded (ADR) if auto-launch becomes the default, since it changes the trust model of `--cmux`.
