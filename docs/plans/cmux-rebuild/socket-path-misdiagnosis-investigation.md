# Investigation: `pr9k --cmux` always reports "cmux is installed but not running" on macOS

`pr9k --cmux` hardcodes a Linux-only socket path (`/run/cmux.sock`) that cannot exist on macOS, so it misdiagnoses every run as "cmux is not running" regardless of whether cmux is actually running.

## Problem Statement

- **Symptoms:** Running `pr9k --cmux` consistently aborts with `cmuxctl: cmux is installed but not running; start cmux and try again`. The user reports this is *consistent* — it happens every time.
- **Expected behavior:** When cmux is installed and running, `pr9k --cmux` should connect to it and start the workspace. The user's stated wish: if cmux is not running, pr9k should start it.
- **Conditions:** macOS host (the user is on Darwin 25.3.0; macOS is cmux's primary platform). `CMUX_SOCKET_PATH` is unset. cmux v0.64.6 is installed at `/opt/homebrew/bin/cmux` (the exact pinned version).
- **Impact:** `--cmux` mode is completely unusable on macOS out of the box. Preflight is fatal (`os.Exit(1)`), so the workspace lifecycle never starts. This blocks the entire cmux feature on its primary platform.

## Evidence Summary

### E1: The error string is produced by three code paths, all rooted in socket-path resolution

- **Source:** `src/internal/cmuxctl/preflight.go:90`, `:99`, `:134`
- **Finding:**
  ```go
  // preflight.go:89-91 (resolveSocketPath, EvalSymlinks branch)
  if errors.Is(err, fs.ErrNotExist) {
      return "", errors.New("cmuxctl: cmux is installed but not running; start cmux and try again")
  }
  // preflight.go:98-100 (resolveSocketPath, os.Stat branch)
  // preflight.go:133-134 (classifyDialError fallback for ENOENT / transient)
  return errors.New("cmuxctl: cmux is installed but not running; start cmux and try again")
  ```
- **Relevance:** The user-facing symptom string matches exactly. The dominant trigger is the `filepath.EvalSymlinks` → `fs.ErrNotExist` branch at line 90 — it fires when the socket path does not exist on disk.

### E2: The five-condition `Preflight`, and how the dial is reached

- **Source:** `src/internal/cmuxctl/preflight.go:40-75`
- **Finding:**
  ```go
  func Preflight(ctx context.Context, prober CmuxProber, client CmuxClient) []error {
      if !prober.CmuxBinaryAvailable() { /* Condition 1 */ }
      socketPath, err := resolveSocketPath()   // D-15 validation
      if err != nil { return []error{err} }    // <-- aborts here on macOS
      conn, dialErr := net.DialUnix("unix", nil, &net.UnixAddr{Name: socketPath, Net: "unix"})
      if dialErr != nil { return []error{classifyDialError(dialErr, socketPath)} }
      conn.Close()
      id, err := client.SystemIdentify(ctx)    // Condition 5
      ...
  }
  ```
- **Relevance:** Condition 1 (binary on PATH) passes — cmux is installed (E7). Execution reaches `resolveSocketPath()`, which fails before `net.DialUnix` is ever attempted. The failure is in path resolution, not connectivity.

### E3: The default socket path is hardcoded to `/run/cmux.sock`, explicitly flagged as unverified

- **Source:** `src/internal/cmuxctl/preflight.go:18-20`, `:80-93`
- **Finding:**
  ```go
  // defaultSocketPath is the cmux Unix socket path used when CMUX_SOCKET_PATH is unset.
  // TODO(OI-1): verify against pinned cmux version; update if the actual default differs.
  const defaultSocketPath = "/run/cmux.sock"

  func resolveSocketPath() (string, error) {
      raw := strings.TrimSpace(os.Getenv("CMUX_SOCKET_PATH"))
      if raw == "" {
          raw = defaultSocketPath          // "/run/cmux.sock"
      }
      resolved, err := filepath.EvalSymlinks(raw)
      if err != nil {
          if errors.Is(err, fs.ErrNotExist) {
              return "", errors.New("cmuxctl: cmux is installed but not running; start cmux and try again")
          }
          ...
  ```
- **Relevance:** This is the root defect. The `TODO(OI-1)` comment admits the default was a guess and was never verified against the pinned cmux version. When `CMUX_SOCKET_PATH` is unset (the user's case), every run resolves `/run/cmux.sock`.

### E4: The duplicate fallback in the client constructor uses the same bad default

- **Source:** `src/internal/cmuxctl/real.go:73-80`
- **Finding:**
  ```go
  // NewProductionClient creates a RealClient using CMUX_SOCKET_PATH (or the
  // platform default) ...
  func NewProductionClient() *RealClient {
      socketPath := strings.TrimSpace(os.Getenv("CMUX_SOCKET_PATH"))
      if socketPath == "" {
          socketPath = defaultSocketPath   // same "/run/cmux.sock"
      }
      return NewRealClient(socketPath, DefaultTimeout)
  }
  ```
- **Relevance:** Socket-path resolution is duplicated in two places (`resolveSocketPath` and `NewProductionClient`) and both read only `CMUX_SOCKET_PATH` + the broken constant. Any fix must change both, ideally by consolidating into one resolver. Neither reads the `CMUX_SOCKET` deprecated alias (E8).

### E5: The error is fatal — the run aborts with exit code 1

- **Source:** `src/cmd/pr9k/main.go:136-148`, `:205-210`
- **Finding:**
  ```go
  if errs := cmuxctl.Preflight(ctx, cmuxProber, client); len(errs) > 0 {
      for _, e := range errs { _, _ = fmt.Fprintln(errOut, e) }
      return false
  }
  // caller: if !ok { os.Exit(1) }
  ```
- **Relevance:** There is no recovery path. `RunPhase1` is never reached; no workspace, no panes. The single misdiagnosed preflight error kills the whole feature.

### E6: On the user's macOS host, `/run` does not exist at all; `CMUX_SOCKET_PATH` is unset; cmux is installed and is the pinned version

- **Source:** Live commands on the user's machine (Darwin 25.3.0)
- **Finding:**
  ```
  $ which cmux           → /opt/homebrew/bin/cmux
  $ cmux --version       → cmux 0.64.6 (86) [2f96c15c2]
  $ echo "[$CMUX_SOCKET_PATH]" → []                 (unset)
  $ ls -la /run/cmux.sock → ls: /run/cmux.sock: No such file or directory
  $ ls -la /run           → ls: /run: No such file or directory
  ```
- **Relevance:** `/run` is a Linux-ism. It does not exist on macOS (no `/run` directory, let alone `/run/cmux.sock`). With `CMUX_SOCKET_PATH` unset, `resolveSocketPath()` *can never* succeed on this machine — the result is deterministic and independent of whether cmux is running. This is exactly why the failure is "consistent."

### E7: cmux's own source defines the real defaults — `~/.config/cmux/cmux.sock` (current) and `/tmp/cmux.sock` (legacy). `/run/cmux.sock` appears nowhere in cmux.

- **Source:** `manaflow-ai/cmux` repository (authoritative, via `gh search code`)
- **Finding:**
  ```
  CLI/CLISocketPathResolver.swift:  private static let stableSocketFileName = "cmux.sock"
  CLI/CLISocketPathResolver.swift:  static let legacyDefaultSocketPath = "/tmp/cmux.sock"
  Sources/SocketControlSettings.swift: legacyStableDefaultSocketPath = "/tmp/cmux.sock"
  Resources/opencode-plugin.js:     const DEFAULT_SOCKET = `${os.homedir()}/.config/cmux/cmux.sock`;
  docs/feed.md:  Verify `$CMUX_SOCKET_PATH` matches the running app (default is `~/.config/cmux/cmux.sock`).
  docs/feed.md:  | `~/.config/cmux/cmux.sock` | V2 socket the hooks/plugin talk to. |
  web/.../docs/api/page.tsx:        SOCK="${CMUX_SOCKET_PATH:-/tmp/cmux.sock}"
  ```
  A repo-wide search for `cmux.sock` returns **zero** occurrences of `/run/cmux.sock`.
- **Relevance:** The authoritative default on macOS is `~/.config/cmux/cmux.sock` (the "V2" socket), with `/tmp/cmux.sock` as the documented legacy/compatibility default. `/run/cmux.sock` is a fabrication in pr9k. `~/.config/cmux/` already exists on the user's machine (`cmux config paths` → `primary: ~/.config/cmux/cmux.json`), confirming this is the live config root.

### E8: `CMUX_SOCKET_PATH` is canonical; `CMUX_SOCKET` is a supported (deprecated) alias

- **Source:** cmux `docs/cli-contract.md` (fetched from cmux repo)
- **Finding:** cli-contract lists `CMUX_SOCKET_PATH` as "Canonical socket path override" and `CMUX_SOCKET` as a "Deprecated compatibility alias."
- **Relevance:** A correct resolver should honor both env vars (canonical first, alias second) before falling back to the on-disk defaults. pr9k currently reads only `CMUX_SOCKET_PATH`.

### E9: cmux can self-launch (`cmux <path>` "launches cmux if needed"), but its default access mode is descendants-only

- **Source:** `cmux --help` (live); `docs/how-to/setting-up-cmux.md:43-47`
- **Finding:**
  ```
  cmux <path>   Open a directory in a new workspace (launches cmux if needed)
  ```
  > cmux defaults to **descendants-only** mode: only processes that are children of the cmux session can reach the socket. … **Allow-all mode caveat (D-20):** If your cmux is configured with allow-all mode, the not-a-descendant error never fires.
- **Relevance:** Bears directly on the user's requested fix ("pr9k should start cmux"). cmux *can* be launched programmatically, but see E10 — auto-start alone does not produce a connection pr9k can use under the default security mode.

### E10: Panes are spawned by cmux via RPC, not by pr9k; the intended flow is pr9k launched from inside a cmux pane

- **Source:** `src/internal/cmuxctl/runphase1.go:195-223`; `docs/how-to/setting-up-cmux.md:30-41`; `docs/plans/cmux-rebuild/artifacts/decision-log.md:179`
- **Finding:**
  ```go
  // RunPhase1 asks cmux to spawn the pr9k panes (they become cmux descendants):
  client.SurfaceSpawn(ctx, orchPaneID, []string{exe, "cmux-pane", "--role=orchestrator"}, spawnEnv)
  ```
  Decision-log D13: "cmux mode requires pr9k to be invoked from inside a cmux session (so the launch is itself a child of cmux)." Setup how-to: outside-cmux launches produce `cmux mode must be launched from inside a cmux session`.
- **Relevance:** If pr9k *spawned* cmux, pr9k would be cmux's **parent**, not a descendant — so in default descendants-only mode pr9k still could not reach the socket (it would hit the EACCES path, `classifyDialError` → "must be launched from inside a cmux session"). Pure auto-start does **not** yield a usable session under the default security posture. The designed flow is: cmux already running → user launches `pr9k --cmux` from a terminal pane inside cmux (where cmux itself exports `CMUX_SOCKET_PATH`) → pr9k connects and asks cmux to spawn panes via RPC.

### E11: pr9k never spawns cmux today — no auto-start machinery exists

- **Source:** Repo-wide search of `src/**/*.go` for `exec.Command(...cmux...)`
- **Finding:** The only cmux exec reference is `exec.LookPath("cmux")` in the binary-presence prober. There is no `exec.Command("cmux", ...)` anywhere.
- **Relevance:** Auto-start would be net-new behavior, and per E10 it does not address the actual root cause (E3/E6/E7). The setup how-to (`docs/how-to/setting-up-cmux.md:53`) also repeats the wrong default ("pr9k uses the platform default `/run/cmux.sock`"), so the documentation carries the same defect.

### E12: cmux's *actual* socket on macOS is `~/Library/Application Support/cmux/cmux.sock`, NOT `~/.config/cmux/cmux.sock`

- **Source:** Live commands on the user's machine
- **Finding:**
  ```
  $ cat /tmp/cmux-last-socket-path
  /Users/mxriverlynn/Library/Application Support/cmux/cmux.sock
  $ cat "$HOME/Library/Application Support/cmux/last-socket-path"
  /Users/mxriverlynn/Library/Application Support/cmux/cmux.sock
  $ ls ~/.config/cmux/            → cmux.json only (NO socket)
  $ ls -ld ~/Library/Application\ Support/cmux  → drwx------ (0700, passes D-15)
  $ ls -ld /tmp /private/tmp      → /tmp -> private/tmp ; drwxrwxrwt (0777)
  ```
- **Relevance:** Corrects E7. cmux as a native macOS `.app` uses `~/Library/Application Support/cmux/` (the macOS data-dir convention), not `~/.config/cmux/` (XDG/Linux). `~/.config/cmux/` holds only the JSON config. Two consequences: (a) any resolver that guesses `~/.config/cmux/cmux.sock` on macOS is still wrong; (b) the legacy `/tmp/cmux.sock` candidate is rejected anyway by pr9k's own D-15 world-writable-parent check (`preflight.go:113`), since `/tmp` is `0777` on macOS.

### E13: cmux publishes a `last-socket-path` **marker file** — its official socket autodiscovery contract

- **Source:** cmux source `Packages/CMUXSocketPathDomain/Sources/CMUXSocketPathDomain/SocketPathMarkerFiles.swift`; `CLI/CLISocketPathResolver.swift`; `tests/test_cli_socket_autodiscovery.py` (via `gh api`/`gh search`)
- **Finding:**
  ```swift
  public static let stableAppSupportFileName = "last-socket-path"   // in the app-support/config dir
  public static let stableTmpPath            = "/tmp/cmux-last-socket-path"  // mirror
  private static let stableSocketFileName    = "cmux.sock"
  static  let legacyDefaultSocketPath        = "/tmp/cmux.sock"
  // CLISocketPathResolver.stableDefaultSocketPath = <stableSocketDirectory>/cmux.sock
  //   else legacyDefaultSocketPath
  // tests/test_cli_socket_autodiscovery.py: write_marker(home, "last-socket-path", stable_socket)
  ```
  cmux writes the live socket path into a marker file at `<app-support>/cmux/last-socket-path` and mirrors it to `/tmp/cmux-last-socket-path`. cmux's own CLI resolves the socket by: env override (`CMUX_SOCKET_PATH`/`CMUX_SOCKET`) → marker file → `<stableSocketDir>/cmux.sock` → `/tmp/cmux.sock`. For the **stable** build (what the user runs: `cmux 0.64.6 (86)`), `<stableSocketDir>` is `~/Library/Application Support/cmux` on macOS and `~/.config/cmux` on Linux — exactly what Go's `os.UserConfigDir()` returns on each platform.
- **Relevance:** This is the authoritative, version-stable discovery mechanism. Mirroring it (env → marker file → `os.UserConfigDir()/cmux/cmux.sock` → `/tmp/cmux.sock`) makes pr9k correct on both platforms without per-instance guessing and without depending on cmux exporting `CMUX_SOCKET_PATH`.

## Root Cause Analysis

### Summary

pr9k hardcodes `defaultSocketPath = "/run/cmux.sock"` — a path that exists on no cmux platform and is *impossible* on macOS (where `/run` itself does not exist) — so with `CMUX_SOCKET_PATH` unset, `resolveSocketPath()` deterministically fails with `fs.ErrNotExist` and pr9k misreports "cmux is installed but not running," regardless of cmux's real state.

### Detailed Analysis

The causal chain: `pr9k --cmux` → `runCmuxMode` → `cmuxctl.Preflight` (E2). Condition 1 passes because cmux is installed (E6). Execution reaches `resolveSocketPath()` (E3). `CMUX_SOCKET_PATH` is unset on the user's machine (E6), so `raw` falls back to the constant `/run/cmux.sock` (E3). `filepath.EvalSymlinks("/run/cmux.sock")` returns `fs.ErrNotExist` because `/run` does not exist on macOS at all (E6), producing `cmuxctl: cmux is installed but not running; start cmux and try again` (E1). The error is fatal — `os.Exit(1)` — so the run dies before any connection attempt (E5).

The defect is that `/run/cmux.sock` is simply wrong. cmux's own source proves it discovers the socket via the `last-socket-path` marker file and a platform-correct default — `~/Library/Application Support/cmux/cmux.sock` on macOS, `~/.config/cmux/cmux.sock` on Linux — with `/tmp/cmux.sock` as the documented legacy fallback; `/run/cmux.sock` appears nowhere in cmux (E12, E13). The `TODO(OI-1)` comment (E3) is an admission that this value was a placeholder guess.

This means the user's reported wish — "if cmux isn't running, pr9k should start it" — is a reasonable inference from a **misleading error message**, not the actual fix. The error claims cmux is not running; in reality pr9k is looking at a path cmux never uses. Even when the user *does* have cmux running, pr9k will still print the same error. Auto-starting cmux would not fix this (and, per E9/E10/V7, pr9k launching cmux would make pr9k *not a descendant* of cmux, failing the default descendants-only socket-access check anyway). The correct fix is to resolve the socket path the way cmux itself does (E13).

## Coding Standards Reference

| Standard | Source | Applies To |
|----------|--------|------------|
| Package-prefixed error messages, include the offending path in I/O errors | `docs/coding-standards/error-handling.md` | The improved "not running" / resolution-failure messages must keep the `cmuxctl:` prefix and name the resolved path tried |
| Explicit, cross-platform precondition validation; do not rely on platform-implicit behavior | `docs/coding-standards/error-handling.md` | The new resolver must not assume a Linux-only filesystem layout |
| Resolve paths through `filepath.EvalSymlinks`; derive user paths explicitly | `docs/coding-standards/go-patterns.md` | The new resolver keeps the existing D-15 `EvalSymlinks`/socket-type/world-writable validation once a candidate is chosen; uses `os.UserConfigDir()` (platform-correct: `~/Library/Application Support` on macOS, `~/.config` on Linux) for the stable default |
| Narrow-reading principle (pr9k is a generic runner) | `docs/adr/20260410170952-narrow-reading-principle.md` | The resolver encodes cmux's *socket discovery contract*, not Ralph workflow knowledge — acceptable, but keep it confined to `cmuxctl` and mirror cmux's documented contract rather than inventing behavior |
| Versioning: `--cmux` socket-discovery is user-visible behavior | `docs/coding-standards/versioning.md` | Fixing the default changes observable behavior; bump version and note in release notes/feature doc |
| Feature docs ship with the change | `docs/coding-standards/documentation.md` | `docs/how-to/setting-up-cmux.md` and `docs/features/cmux-mode.md` must be corrected in the same change (they currently document `/run/cmux.sock`) |

## Planned Fix

### Summary

Replace the single bogus `/run/cmux.sock` default with one shared resolver that mirrors cmux's authoritative socket-discovery contract — env override (`CMUX_SOCKET_PATH` → `CMUX_SOCKET`) → cmux's `last-socket-path` marker file → `os.UserConfigDir()/cmux/cmux.sock` → legacy `/tmp/cmux.sock` (E12, E13) — make the failure message accurate and actionable, and correct the docs; do **not** implement blind auto-start, because it does not address the root cause and is incompatible with cmux's default descendants-only access mode.

### Changes

#### `src/internal/cmuxctl/socketpath.go` (new file)

- **Change:** Add an exported resolver — `ResolveSocketPath(deps) (chosen string, tried []string)` — used by both `resolveSocketPath` (preflight) and `NewProductionClient`. It mirrors cmux's **stable-variant** discovery contract (E13), in order:
  1. `CMUX_SOCKET_PATH` if non-empty → use verbatim (canonical, E8).
  2. `CMUX_SOCKET` if non-empty → use verbatim (deprecated alias, E8).
  3. **Marker file:** read the first existing of `filepath.Join(os.UserConfigDir(), "cmux", "last-socket-path")` then `/tmp/cmux-last-socket-path`; the file's trimmed first line *is* the socket path (E12, E13).
  4. Stable default: `filepath.Join(os.UserConfigDir(), "cmux", "cmux.sock")` — Go's `os.UserConfigDir()` returns `~/Library/Application Support` on macOS and `~/.config` (or `$XDG_CONFIG_HOME`) on Linux, matching cmux's `<stableSocketDir>` exactly (E12, E13).
  5. Legacy: `/tmp/cmux.sock` (E13) — included for contract fidelity; note it will be rejected by the existing D-15 world-writable-parent check on macOS (E12), which is acceptable because (3)/(4) resolve first in every realistic case.

  Selection rule for (3)–(5): take the first candidate whose target exists on disk; if none exist, return the stable default (4) so the error message names the cmux-correct path. Scope is deliberately limited to the **stable** cmux variant (the pinned, supported build); nightly/staging/dev variant sockets are dev-only and reachable via the `CMUX_SOCKET_PATH` escape hatch — encoding the full variant matrix would over-fit and violates the narrow-reading ADR.
- **Evidence:** (E3), (E4), (E8), (E12), (E13)
- **Standards:** error-handling (explicit/cross-platform), go-patterns (`os.UserConfigDir`, `EvalSymlinks`), narrow-reading ADR (confined to `cmuxctl`, mirrors only the supported stable contract)
- **Details:** Pure except for injected `os.Getenv`/`os.UserConfigDir`/stat/readfile shims, so it is hermetically unit-testable across both platforms. The chosen path still flows through the existing D-15 validation (`EvalSymlinks`, socket-type, world-writable-parent) in `resolveSocketPath`.

#### `src/internal/cmuxctl/preflight.go`

- **Change:** Delete `const defaultSocketPath = "/run/cmux.sock"` and both `TODO(OI-1)` lines tied to it. In `resolveSocketPath`, replace the `CMUX_SOCKET_PATH`-or-constant block with a call to the shared resolver. Rewrite the "not running" message to name the resolved path and give accurate next steps (start cmux, then run `pr9k --cmux` *from inside a cmux pane*; or set `CMUX_SOCKET_PATH`).
- **Evidence:** (E1), (E3), (E6), (E7), (E10)
- **Standards:** error-handling (package prefix + include path)
- **Details:** New message shape, e.g. `cmuxctl: cmux socket not found at %s (tried: %s); start cmux and launch pr9k from inside a cmux pane, or set CMUX_SOCKET_PATH`. Preserve the distinct EACCES/ECONNREFUSED branches in `classifyDialError` unchanged.

#### `src/internal/cmuxctl/real.go`

- **Change:** Replace the duplicated `CMUX_SOCKET_PATH`-or-`defaultSocketPath` block in `NewProductionClient` with the shared resolver so the client and preflight can never disagree on the socket path.
- **Evidence:** (E4)
- **Standards:** error-handling; DRY (single source of truth for socket discovery)
- **Details:** `NewProductionClient` calls the shared resolver and passes the chosen path to `NewRealClient`.

#### `src/internal/cmuxctl/preflight_test.go`, `socketpath_test.go` (new), `real_test.go`

- **Change:** Add table tests for the resolver: env precedence (`CMUX_SOCKET_PATH` beats `CMUX_SOCKET` beats marker beats defaults); marker-file resolution (app-support marker preferred over `/tmp` mirror; trimmed content used as the path); **macOS path** assertion that `os.UserConfigDir()/cmux/cmux.sock` resolves under `Library/Application Support` (E12); Linux path assertion under `.config`; and the "name the stable default when nothing exists" behavior. Update/keep existing preflight tests (none assert the literal `/run/cmux.sock`, per V5, but `TestPreflight_SocketPath_Empty_FallsBackToDefault` should assert the new message shape, not just the `cmuxctl:` prefix).
- **Evidence:** (E3), (E4), (E8), (E12), (E13)
- **Standards:** `docs/coding-standards/testing.md` (race detector; helper path resolution)
- **Details:** Inject env + a fake `UserConfigDir`/stat/readfile so tests are hermetic and exercise both the macOS and Linux directory shapes regardless of the host OS.

#### `docs/how-to/setting-up-cmux.md`, `docs/features/cmux-mode.md`, `docs/code-packages/cmuxctl.md`

- **Change:** Correct every statement that the default socket is `/run/cmux.sock`; document the real resolution order (env → marker file → `os.UserConfigDir()/cmux/cmux.sock` → `/tmp/cmux.sock`) and the `CMUX_SOCKET` alias. Clarify that `pr9k --cmux` must be launched from inside a cmux pane under the default descendants-only mode (so after this fix a plain-terminal launch produces the *accurate* "must be launched from inside a cmux session" message, not the misleading "not running" — see V4) and that pr9k does not auto-start cmux (rationale from E10/V7). **Additionally** (per the project "no pre-existing errors" rule and V6): fix `cmuxctl.md`'s stale `SurfaceSpawn` signatures (lines ~18 and ~77 are missing the `env map[string]string` parameter present in `client.go:35`/`fake.go:32`) and the stale Phase-1 placeholder spawn-command description (~155–156) that no longer matches `runphase1.go:204`.
- **Evidence:** (E9), (E10), (E11), (E12), (E13); V4, V6
- **Standards:** `docs/coding-standards/documentation.md` (docs ship with the change); `~/.claude/CLAUDE.md` (no pre-existing errors)
- **Details:** Update the troubleshooting table, the "If `CMUX_SOCKET_PATH` is unset…" sentence, and the two `SurfaceSpawn` signature blocks + spawn-command prose in `cmuxctl.md`.

#### `src/internal/version/version.go` (version bump)

- **Change:** Bump the patch/minor version; this changes user-visible `--cmux` behavior.
- **Evidence:** (E5), (E6)
- **Standards:** `docs/coding-standards/versioning.md`
- **Details:** Per the 0.y.z rules in the versioning standard.

### What the fix does and does NOT do (honesty about "after the fix" — V4)

This fix makes pr9k **find the right socket** and **report the truth**. Concretely, after the fix, with cmux running:

- Launched **from inside a cmux pane** (the designed flow, default descendants-only mode): cmux exports `CMUX_SOCKET_PATH` into the pane environment, OR the marker file resolves the live socket; pr9k connects and `--cmux` works. This is the path the fix unblocks.
- Launched **from a plain terminal** (outside cmux, default descendants-only mode): the resolver now finds the real socket, the dial returns `EACCES`, and pr9k prints the *accurate* `cmux mode must be launched from inside a cmux session` instead of the *misleading* `cmux is installed but not running`. The user still does not get a working session — **by cmux's security design**, not by a pr9k bug. The win is an honest, actionable error.
- cmux **not running** at all: accurate "socket not found at <path> (tried: …); start cmux" message naming the cmux-correct path.

The investigation does not claim the fix yields a working `pr9k --cmux` from any terminal — it claims the fix ends the *misdiagnosis* and makes every outcome's message correct and actionable.

### Auto-start (the user's literal request): why it is deferred, not adopted

The user asked for "if cmux isn't running, pr9k should start it." This is **out of scope for the root-cause fix** and is recommended *against* as the primary remedy, because:

1. It does not address the bug. The user's failure happens whether or not cmux is running (E6) — the path is simply wrong (E12, E13).
2. Under cmux's default descendants-only access mode (E9), pr9k must be a **descendant** of the cmux session to reach the socket (E10). If pr9k launches cmux, pr9k is *not* a descendant of it (it is an ancestor/unrelated process, regardless of whether cmux daemonizes — V7); the socket access check still fails with `EACCES`. Auto-start only "works" under non-default allow-all mode.
3. The designed flow is pr9k launched from inside a cmux pane, where cmux exports `CMUX_SOCKET_PATH` itself (E8, E10); the correct fix makes that flow work.

A genuinely useful auto-start would be a larger, separate design (e.g. `cmux <projectDir>` to open a workspace, then re-exec pr9k *inside* a cmux-spawned pane so it is a descendant) and should be its own feature decision, not bundled into this defect fix.

## Validation Results

A dedicated `adversarial-validator` pass challenged the evidence, the root cause, and the fix. It **confirmed the root cause** but **broke the original fix design** (which had proposed `~/.config/cmux/cmux.sock` as the macOS default). The fix above is the post-validation, corrected design.

### Counter-Evidence Investigated

#### V1: Earlier check masks the root cause / EACCES instead of missing socket

- **Hypothesis:** A prior check (`startupValidate`, binary prober) fails first with a different error, or the real failure is the EACCES descendants-only path — making the path-resolution root cause wrong.
- **Investigation:** `main.go:136-154` — `startupValidate` runs first but its errors carry the `preflight:` prefix; the user's string carries `cmuxctl:`, proving `startupValidate` passed and execution reached `cmuxctl.Preflight`. `/run` does not exist on macOS (E6), so `resolveSocketPath` fails at `filepath.EvalSymlinks` (preflight.go:87-90) **before** `net.DialUnix`, so EACCES cannot occur. Confirmed by `TestRunCmuxMode_StandardPreflightBeforeCmuxPreflight`.
- **Result:** Confirmed (root cause holds).
- **Impact:** None — the causal chain is structurally correct.

#### V2: (CRITICAL) The proposed macOS default `~/.config/cmux/cmux.sock` is WRONG

- **Hypothesis:** E7's `~/.config/cmux/cmux.sock` reflects a Linux/XDG view, not the macOS runtime; the proposed resolver would still fail on macOS.
- **Investigation:** Live machine: `/tmp/cmux-last-socket-path` and `~/Library/Application Support/cmux/last-socket-path` both contain `/Users/mxriverlynn/Library/Application Support/cmux/cmux.sock`. `~/.config/cmux/` holds only `cmux.json` — no socket. cmux as a macOS `.app` uses `~/Library/Application Support/cmux/`, not `~/.config/`. This was independently re-verified directly (E12) and against cmux source `SocketPathMarkerFiles.swift` / `CLISocketPathResolver.swift` (E13).
- **Result:** **Refuted** — the original fix's macOS candidate was wrong.
- **Impact:** **Plan changed.** The resolver was rebuilt around cmux's `last-socket-path` marker file (E13) and `os.UserConfigDir()` (which yields the correct `~/Library/Application Support` on macOS and `~/.config` on Linux). See Adjustments A1.

#### V3: `/tmp/cmux.sock` legacy fallback is unreachable on macOS (D-15)

- **Hypothesis:** `/tmp` is `0777` on macOS, so pr9k's own world-writable-parent check (preflight.go:113, `Perm()&0o002`) rejects any `/tmp/cmux.sock` before dialing.
- **Investigation:** Confirmed `/tmp -> /private/tmp`, `drwxrwxrwt` (`0777`); `0o777 & 0o002 != 0` → D-15 rejects. The sticky bit is not consulted.
- **Result:** Confirmed.
- **Impact:** **Plan changed.** The fix now explicitly documents that the legacy `/tmp/cmux.sock` candidate is D-15-rejected on macOS and is retained only for contract fidelity, since the marker file (3) and stable default (4) resolve first in every realistic case. See Adjustments A2.

#### V4: "After the fix, does `pr9k --cmux` actually work from a plain terminal?"

- **Hypothesis:** The investigation implies the fix makes `--cmux` work; really, a plain-terminal launch in descendants-only mode still fails (EACCES).
- **Investigation:** Correct. With the resolver fixed, the dial from outside cmux returns `EACCES` → `classifyDialError` → "must be launched from inside a cmux session." The fix converts a *misleading* error into an *accurate* one; it does not bypass cmux's security model.
- **Result:** Partially refuted (the framing was over-optimistic, not the fix).
- **Impact:** **Plan changed.** Added the "What the fix does and does NOT do" section to represent every post-fix outcome honestly. See Adjustments A3.

#### V5: Existing tests' coverage of the default path

- **Hypothesis:** Tests assert the literal `/run/cmux.sock` and will break; or coverage is missing.
- **Investigation:** No test asserts `/run/cmux.sock`. `TestPreflight_SocketPath_Empty_FallsBackToDefault` only checks the `cmuxctl:` prefix — too loose to catch resolver regressions, and gives zero coverage of the macOS path.
- **Result:** Confirmed (no breakage; coverage gap real).
- **Impact:** **Plan changed.** The test item now requires explicit macOS (`Library/Application Support`) and marker-file assertions, and tightening `TestPreflight_SocketPath_Empty_FallsBackToDefault` to the new message shape. See Adjustments A4.

#### V6: Pre-existing stale `SurfaceSpawn` signatures in `cmuxctl.md`

- **Hypothesis:** The doc-fix scope ("correct socket-path statements") leaves other lies in `cmuxctl.md`.
- **Investigation:** `cmuxctl.md:18` and `:77` show `SurfaceSpawn(... argv []string) error` — missing `env map[string]string`, which exists in `client.go:35` and `fake.go:32`. Spawn-command prose (~155-156) describes `tail -f /dev/null` one-liners; `runphase1.go:204` spawns `[exe, "cmux-pane", "--role=..."]`.
- **Result:** Confirmed.
- **Impact:** **Plan changed.** Per the project "no pre-existing errors" rule, the doc-fix item now also corrects these signatures and the spawn-command prose. See Adjustments A5.

#### V7: "pr9k becomes cmux's parent" reasoning for deferring auto-start

- **Hypothesis:** If `cmux <path>` daemonizes, pr9k would not be cmux's parent, so the deferral reasoning is unsound.
- **Investigation:** Decision-log D13 / technical notes T2 confirm the constraint is *descendant-of-cmux ancestry*, not the parent/child direction specifically. Whether cmux daemonizes or not, a pr9k that launches cmux is never a descendant of it → EACCES persists.
- **Result:** Confirmed (conclusion sound; wording imprecise).
- **Impact:** **Plan changed (wording).** The deferral now says pr9k would be "not a descendant" rather than "the parent." See Adjustments A6.

### Adjustments Made

- **A1 (from V2):** Replaced the broken `~/.config/cmux/cmux.sock` macOS candidate with cmux's authoritative discovery: env (`CMUX_SOCKET_PATH`/`CMUX_SOCKET`) → `last-socket-path` marker file (`<UserConfigDir>/cmux/last-socket-path`, then `/tmp/cmux-last-socket-path`) → `os.UserConfigDir()/cmux/cmux.sock` → `/tmp/cmux.sock`. Added E12, E13.
- **A2 (from V3):** Documented that `/tmp/cmux.sock` is D-15-rejected on macOS and is retained only for contract fidelity.
- **A3 (from V4):** Added "What the fix does and does NOT do" — the fix ends the misdiagnosis and makes every outcome's message accurate; it does not bypass descendants-only security.
- **A4 (from V5):** Test item now mandates macOS/marker-file assertions and a tightened message-shape assertion.
- **A5 (from V6):** Doc-fix item extended to repair `cmuxctl.md`'s stale `SurfaceSpawn` signatures and spawn-command prose.
- **A6 (from V7):** Sharpened the auto-start deferral wording to "not a descendant."

### Confidence Assessment

- **Confidence:** High for the **root cause** (deterministically reproduced from the user's environment E6, self-admitted in code via `TODO(OI-1)` E3). High for the **revised fix** (now grounded in cmux's authoritative marker-file contract E13 and the platform-correct `os.UserConfigDir()`, with the wrong-path failure mode that broke the first design eliminated by V2/V3 and directly re-verified on the user's machine, E12).
- **Remaining Risks:**
  - **Unverified:** whether cmux exports `CMUX_SOCKET_PATH` into spawned-pane environments. The marker-file step (3) makes the fix correct *regardless* of whether it does, so this is now low-impact, but it should be confirmed during implementation by inspecting a real cmux pane's env.
  - The fix only restores honest error messages and the inside-a-pane happy path; a plain-terminal launch under default descendants-only mode still cannot connect — that is cmux's security design, not a residual bug (V4).
  - cmux's rolling-update policy (pinned v0.64.6) could change the marker/default contract; mirroring the contract + the `CMUX_SOCKET_PATH` escape hatch + the existing `OI-1` rolling-pin process is the hedge.
  - Auto-start remains an unmet user expectation; deliberately scoped out with rationale (V7), pending the user's explicit decision.

## Final Summary

- **Root Cause:** pr9k hardcodes the cmux socket default to `/run/cmux.sock` (preflight.go:20, real.go:78), a path that cannot exist on macOS (`/run` itself is absent), so with `CMUX_SOCKET_PATH` unset every `pr9k --cmux` run on macOS fails `resolveSocketPath` and misreports cmux as "not running," regardless of cmux's real state (E3, E6, E12, E13).
- **Fix:** Replace the bogus constant with one shared resolver mirroring cmux's authoritative discovery — env (`CMUX_SOCKET_PATH`→`CMUX_SOCKET`) → `last-socket-path` marker file → `os.UserConfigDir()/cmux/cmux.sock` → legacy `/tmp/cmux.sock` — make the error message accurate, and correct the docs (E4, E8, E12, E13).
- **Why Correct:** cmux's own source (`SocketPathMarkerFiles.swift`, `CLISocketPathResolver.swift`) and the live marker files on the user's machine prove the discovery contract and that `/run/cmux.sock` is used nowhere in cmux (E12, E13); the failure reproduces deterministically (E6).
- **Validation Outcome:** Adversarial validation **confirmed the root cause** but **broke the original fix** (V2: `~/.config/cmux/cmux.sock` is wrong on macOS; V3: `/tmp/cmux.sock` is D-15-rejected) — the fix was rebuilt around the marker-file contract and `os.UserConfigDir()`, and scope was widened to honest post-fix messaging (V4) and pre-existing doc lies (V6).
- **Remaining Risks:** Low for the fix's correctness; the main caveat is honesty rather than risk — a plain-terminal launch in default descendants-only mode still cannot connect by cmux's design (V4), and true auto-start is intentionally deferred for the user's decision (V7).
