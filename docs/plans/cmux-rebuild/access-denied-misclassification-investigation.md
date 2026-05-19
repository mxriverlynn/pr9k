# Investigation: `pr9k --cmux` reports "version is incompatible" when cmux actually denied access

When cmux is running but `pr9k --cmux` is launched from a terminal that is not a descendant of the cmux session, cmux replies with a plaintext `ERROR: Access denied …` line; pr9k cannot parse it as JSON and misreports it as `cmux version is incompatible with pr9k cmux mode`.

## Problem Statement

- **Symptoms:** Two sequential symptoms from the user:
  1. `cmux --cmux` with cmux not running → `cmuxctl: cmux socket not found at … (looked in: …)` — this is the *correct, fixed* behaviour from the prior socket-path fix.
  2. After opening the cmux app and retrying → `cmuxctl: cmux version is incompatible with pr9k cmux mode: cmuxctl: read system.identify: invalid character 'E' looking for beginning of value`.
- **Expected behavior:** When cmux denies access because the launching process is not inside a cmux pane, pr9k should say exactly that and tell the user how to fix it — not claim a version incompatibility.
- **Conditions:** cmux running in its **default** `cmuxOnly` socket-control mode; `pr9k --cmux` launched from a normal terminal (`~/dev/mxriverlynn/pr9k/bin`) that is not a child process of the cmux session.
- **Impact:** The user cannot tell what is actually wrong. The message points at a version problem (so the user suspects their cmux install / native-vs-brew), when the real issue is cmux's descendants-only security model plus a pr9k error-classification bug. This is the **same misdiagnosis anti-pattern** as the original `/run/cmux.sock` bug.
- **User's environment question (answered):** There is only **one** cmux. `brew install --cask cmux` *is* the native app (`/Applications/cmux.app`, v0.64.6, bundle `com.cmuxterm.app`); `/opt/homebrew/bin/cmux` is that app's CLI shim. "Native vs brew" is a non-issue — they are the same artifact. Two coexisting installs are **not** the cause (two same-bundle stable installs cannot run simultaneously on macOS; the symptom requires a *live* socket that accepts then rejects).

## Evidence Summary

### E1: cmux's default socket mode is `cmuxOnly`; the user is on that default

- **Source:** cmux `Sources/SocketControlSettings.swift` (`defaultMode → .cmuxOnly`); user's `~/.config/cmux/cmux.json` (only `$schema`, `schemaVersion` set; the `automation`/`socketControlMode` block is the commented default `"cmuxOnly"`); user machine confirms one install (`/Applications/cmux.app` 0.64.6, brew `--cask` = same app).
- **Finding:** `static var defaultMode: SocketControlMode { return .cmuxOnly }`; mode description: "Only processes started inside cmux terminals can send commands."
- **Relevance:** Every fresh cmux is `cmuxOnly`. The user never changed it, so cmux requires the connecting process to be a cmux descendant.

### E2: cmux writes a **plaintext** `ERROR: Access denied …\n` to non-descendants, before reading any request

- **Source:** cmux `Sources/TerminalController.swift` `handleClient()` (~lines 2178–2210) and `writeSocketResponse` (~1690)
- **Finding:**
  ```swift
  if accessMode == .cmuxOnly {
      let pid = peerPid ?? getPeerPid(socket)
      if let pid {
          guard isDescendant(pid) else {
              _ = writeSocketResponse(
                  "ERROR: Access denied — only processes started inside cmux can connect", to: socket)
              return
          }
      }
      if pid == nil {
          guard peerHasSameUID(socket) else {
              _ = writeSocketResponse("ERROR: Unable to verify client process", to: socket); return }
      }
  }
  // ... only AFTER this does it enter the read loop / dispatch system.identify
  private nonisolated func writeSocketResponse(_ response: String, to socket: Int32) -> Bool {
      let payload = response + "\n"; return Self.writeAllToSocket(Data(payload.utf8), to: socket) }
  ```
- **Relevance:** The connection is accepted at the socket layer, then cmux writes the bare UTF-8 line `ERROR: Access denied …\n` and closes — *before* `system.identify` is ever read. The first byte pr9k receives is `'E'`.

### E3: pr9k decodes the socket with a streaming `json.Decoder` and has no plaintext/ERROR handling

- **Source:** `src/internal/cmuxctl/real.go:124–169`, `:266–276`
- **Finding:** `dec = json.NewDecoder(conn)`; `if err := capturedDec.Decode(&resp); err != nil { … fmt.Errorf("cmuxctl: read %s: %w", req.Method, err) }`. `SystemIdentify` calls `c.do(ctx, "system.identify", nil)`. No code anywhere reads or recognises a leading `ERROR:` line (exhaustive grep: no `password`, `auth`, `handshake`, `ERROR` handling).
- **Relevance:** `json.Decoder.Decode` on input whose first non-space byte is `E` returns exactly `invalid character 'E' looking for beginning of value`, wrapped as `cmuxctl: read system.identify: invalid character 'E' looking for beginning of value`.

### E4: `Preflight` condition 5 is a catch-all that mislabels any `SystemIdentify` error as a version incompatibility

- **Source:** `src/internal/cmuxctl/preflight.go:57–68`
- **Finding:**
  ```go
  id, err := client.SystemIdentify(ctx)
  if err != nil {
      safe := string(ansi.StripAll([]byte(err.Error())))
      return []error{fmt.Errorf("cmuxctl: cmux version is incompatible with pr9k cmux mode: %s", safe)}
  }
  ```
- **Relevance:** Transport/access errors and genuine version mismatches are conflated. The access-denied plaintext becomes `cmuxctl: cmux version is incompatible with pr9k cmux mode: cmuxctl: read system.identify: invalid character 'E' …` — the exact reported string. This is the misdiagnosis.

### E5: cmux's descendants check is post-accept (application layer), so pr9k's connect-time `EACCES` heuristic cannot catch it

- **Source:** `src/internal/cmuxctl/preflight.go:51–75` (`classifyDialError` keys off `syscall.EACCES`/`ECONNREFUSED` at `net.DialUnix`); cmux E2 (rejection happens *after* `accept`, via a written response)
- **Finding:** pr9k's preflight does `net.DialUnix` + immediate close (passes, because cmux accepts the connection), then calls `SystemIdentify` on a second dial. cmux's ancestry rejection is an application-level write, not a kernel `EACCES` on connect — so condition 3 ("must be launched from inside a cmux session") never fires; execution falls through to condition 5.
- **Relevance:** The honest "launch from inside a cmux pane" guidance pr9k *already has* (condition 3) is unreachable for the `cmuxOnly` rejection path, because that path is detectable only by reading the `ERROR:` body, which pr9k discards.

### E6: This is cmux's designed behaviour since v0.4.0 — not a v0.64.6 regression, not a protocol/version mismatch

- **Source:** cmux `CHANGELOG.md` ([0.4.0] "cmux CLI with socket control modes"; [0.64.6] no socket changes); cmux `docs/v2-api-migration.md` (server routes any line starting with `{` to the v2 handler and ignores the extra `jsonrpc` field, so pr9k's JSON-RPC framing is accepted once past the ancestry gate)
- **Relevance:** Rules out "cmux version is incompatible" as literally true and rules out the framing mismatch and the two-install theory. The operative cause of the *screenshot symptom* is the `cmuxOnly` ancestry gate plus pr9k's misclassification — but adversarial validation found this is only the first of three defects (E7, E8).

### E7: (LATENT, CRITICAL) At the user's exact pinned cmux commit, `system.identify` returns **no `name`/`version`** — pr9k's capability check can never pass

- **Source:** cmux `Sources/TerminalController.swift` `v2Identify` at commit `2f96c15c2` (= the `cmux 0.64.6 (86) [2f96c15c2]` the user runs), fetched via `gh api …?ref=2f96c15c2`
- **Finding:**
  ```swift
  private func v2Identify(params: [String: Any]) -> [String: Any] {
      guard let tabManager = v2ResolveTabManager(params: params) else {
          return [ "socket_path": socketPath, "focused": NSNull(), "caller": NSNull() ] }
      // ... returns { socket_path, focused:{window/workspace/pane/surface refs}, caller }
  }
  // grep for "name"/"version" inside v2Identify at 2f96c15c2: (none)
  ```
  pr9k: `type Identity struct { Name string \`json:"name"\`; Version string \`json:"version"\` }` (`client.go`); `preflight.go:64`: `if id.Name != "cmux" { … "version is incompatible … name=%q" }`.
- **Relevance:** cmux v2 `system.identify` has **no `name` and no `version` field** at the user's exact build. So `json.Unmarshal` into `Identity` always yields `Name==""`, and `preflight.go:64`'s `id.Name != "cmux"` always fires → `cmuxctl: cmux version is incompatible with pr9k cmux mode: system.identify returned name=""`. **Even with cmux running and pr9k correctly launched inside a cmux pane, preflight cannot pass.** The access-denied fix alone is a phantom fix — the user would trade one "version incompatible" for another.

### E8: (LATENT, CRITICAL) cmux v2 error codes are **strings**; pr9k's `rpcErrorPayload.Code` is `int`, so every cmux error response fails to decode

- **Source:** cmux `TerminalController.swift@2f96c15c2`: `func v2Error(id: Any?, code: String, message: String …)` → `{"id":..,"ok":false,"error":{"code":"<string>","message":..}}`; code literals `"auth_required"`, `"invalid_params"`, `"auth_failed"`, `"method_not_found"`, `"not_found"`, … pr9k `real.go`: `type rpcErrorPayload struct { Code int \`json:"code"\`; Message string \`json:"message"\` }`
- **Relevance:** Any cmux v2 error response (including password-mode `auth_required`) makes pr9k's `json.Decoder.Decode(&resp)` fail with `json: cannot unmarshal string into … Code of type int` — *before* `resp.Error` is ever populated. So pr9k can never read a cmux error; they all collapse into the same condition-5 catch-all. The original fix plan's "detect `auth_required` from the JSON error" branch is unreachable until this type is corrected. (Success envelope `{"ok":true,"result":{…}}` *does* decode — Go ignores the unknown `ok` — so the only happy-path blocker is E7, not the envelope.)

### E9: (DECISIVE) `surface.spawn` and `surface.hide` — the methods `RunPhase1` uses to build the four panes — **do not exist in cmux v2**

- **Source:** cmux `TerminalController.swift@2f96c15c2` v2 dispatch (`processV2Command`); `internal/cmuxctl/runphase1.go` step 5; `internal/cmuxctl/real.go:333–341`
- **Finding:** At the user's pinned commit, the v2 command switch has **no `surface.spawn` and no `surface.hide` case** (grep: 0 hits each). The real cmux v2 surface/pane API is: `surface.split`, `surface.create`, `surface.close`, `surface.move`, `surface.focus`, `pane.create`, `pane.break`, `pane.join`, … pr9k's `RunPhase1` builds the orchestrator + header/log/footer panes via `client.SurfaceSpawn(...)` (→ `surface.spawn`) and hides the orchestrator via `client.SurfaceHide(...)` (→ `surface.hide`). Both calls return cmux v2 `method_not_found`.
- **Relevance:** Even with Fixes A+B+C and a correct in-pane launch, `RunPhase1` cannot create a single pane — it calls methods cmux does not implement. cmux mode is non-functional against the supported cmux, by construction, not just at preflight.

### E10: (DECISIVE) `surface.split` has a different contract than pr9k sends — cmux **requires** `direction` and runs the process via `initial_command`

- **Source:** cmux `v2SurfaceSplit` / `v2SurfaceCreate@2f96c15c2`; pr9k `SplitOpts` (`client.go:14`), `SurfaceSplit` (`real.go:319`)
- **Finding:** cmux `v2SurfaceSplit`: `guard let direction = parseSplitDirection(...) else { return .err(code:"invalid_params", message:"Missing or invalid direction (left|right|up|down)") }`; it accepts `type`, `working_directory`, `initial_command`, `url`, … i.e. the process to run is the `initial_command` param of `surface.split`/`surface.create`. pr9k sends `SplitOpts{ PaneID string \`json:"pane_id,omitempty"\` }` — **no `direction`** (→ cmux `invalid_params`) and **no `initial_command`** (so even a corrected split wouldn't run pr9k's pane process). pr9k's two-step "split, then spawn into the pane" model does not exist in cmux v2 — creation and command are one call.
- **Relevance:** The pane-creation design is mismodelled end to end: wrong method (`surface.spawn` absent, E9), wrong params (`surface.split` needs `direction`+`initial_command`, E10), and the "hidden orchestrator pane" (D-3/D-4) has no `surface.hide` primitive in cmux v2 at all. This is not patchable in `cmuxctl` alone — `RunPhase1`'s architecture must be re-mapped onto cmux v2's surface/pane model.

## Root Cause Analysis

### Summary

**pr9k's entire cmux integration was built against an assumed API that materially does not match cmux v2.** The screenshot symptom is only the first visible layer. (1) cmux's default `cmuxOnly` mode rejects non-descendant pr9k with a plaintext `ERROR: Access denied\n` that pr9k's JSON decoder can't parse and `Preflight` mislabels (E1–E5) — the screenshot symptom. (2) `system.identify` has no `name`/`version`, so the capability check can never pass even inside a pane (E7). (3) error `code` is a string not an int, so every cmux error response fails to decode (E8). (4) **`surface.spawn` and `surface.hide` do not exist in cmux v2** (E9) and `surface.split` requires `direction`+`initial_command` pr9k never sends (E10) — so `RunPhase1` cannot build a single pane even if 1–3 are fixed. Every failure prints the same misleading "version is incompatible". The honest conclusion: this is not a bug with a patch — **the `cmuxctl` client and the `RunPhase1` pane architecture must be reimplemented against the real cmux v2 API**, and the "hidden orchestrator pane" design (D-3/D-4) needs rethinking because cmux v2 has no `surface.hide`.

### Detailed Analysis

The user did the reasonable thing: saw "socket not found", opened the cmux app, retried. cmux was now running, pr9k's (correctly fixed) resolver found the socket and dialed it — but the user ran `./pr9k --cmux` from their **own terminal**, not a child of the cmux session. cmux in default `cmuxOnly` mode (confirmed in the user's config, E1) accepted the connection, found pr9k is not a descendant (`isDescendant` is a PID-ancestry walk, not TTY/Ghostty-based — V2), wrote `ERROR: Access denied — only processes started inside cmux can connect\n` and closed (E2). pr9k's `json.Decoder` saw `E` (E3); `Preflight`'s catch-all condition 5 wrapped it as a version incompatibility (E4); the accurate "inside a cmux session" message never fired because cmux's rejection is an application-layer write, not a connect-time `EACCES` (E5).

That is only the first layer. Adversarial validation, confirmed against the user's exact pinned cmux commit `2f96c15c2`, found the original cmux-mode client was built against an **assumed protocol that does not match cmux v2**: `system.identify` carries no `name`/`version` (E7), and error codes are strings not ints (E8). So even after the user gets inside a cmux pane and the access-denied message is fixed, `preflight.go:64`'s `id.Name != "cmux"` fires (`name=""`) and any cmux error response fails to decode — both funnelling back into the same "version is incompatible" catch-all. This is the same project anti-pattern as the `/run/cmux.sock` bug — **pr9k reports a guessed catch-all instead of the real cause** — now compounded by genuine protocol-modelling errors. It is cmux's designed security model plus pr9k protocol bugs, not a version or install problem (E6); the user's native-vs-brew question is moot (one install — `/Applications/cmux.app` == the brew cask).

## Coding Standards Reference

| Standard | Source | Applies To |
|----------|--------|------------|
| Package-prefixed error messages; include actionable context | `docs/coding-standards/error-handling.md` | New typed access-denied/auth errors must keep the `cmuxctl:` prefix and tell the operator what to do |
| Explicit precondition validation; don't rely on platform-implicit behaviour | `docs/coding-standards/error-handling.md` | Detect cmux's `ERROR:` sentinel explicitly rather than letting it fall through the JSON decoder into a catch-all |
| Test doubles: stubs for queries, no untested behaviours | `docs/coding-standards/testing.md`; `docs/code-packages/cmuxctl.md` | Add a fake cmux server that emits `ERROR: Access denied\n` (E9-class coverage gap from prior analysis) |
| Documentation ships with the change | `docs/coding-standards/documentation.md` | `setting-up-cmux.md` troubleshooting + `cmux-mode.md` preflight table updated for the new message |
| No version bump on this branch | user instruction (this session) | `version.go` untouched |

## Planned Fix

### Summary

There are **two distinct deliverables**, and they must not be conflated:

- **Patch P (truthful errors) — necessary, NOT sufficient, ~½ day:** Fixes A (Identity by `socket_path`), B (`code`→string + `ok` envelope), C (detect plaintext `ERROR:` → accurate access-denied/auth guidance). After P, pr9k stops lying ("version incompatible" → the real reason) and `preflight` can pass from inside a pane. **P does not make `--cmux` work** — `RunPhase1` still calls non-existent methods (E9, E10).
- **Rework R (the actual working solution) — feature-sized:** Reimplement `internal/cmuxctl` + `RunPhase1` against the **real cmux v2 surface/pane API**, and re-architect the orchestrator/pane layout because cmux v2 has no `surface.hide` (E9, E10). This is where "a real working solution" actually lives. It is not a patch; it is a rebuild of the cmux mode against verified cmux v2 behaviour.

The changes below specify Patch P in detail. Rework R is scoped (not specified line-by-line) because it requires a design pass against the real cmux v2 surface/pane model and a rethink of D-3/D-4 — that is its own planning effort, recommended next.

### Changes

#### `src/internal/cmuxctl/client.go` — Identity shape (Fix A, E7)

- **Change:** Redefine `Identity` to the cmux v2 `system.identify` result: `SocketPath string \`json:"socket_path"\`` (plus optionally `Focused json.RawMessage`, `Caller json.RawMessage`). Remove `Name`/`Version` (they do not exist in cmux v2).
- **Evidence:** (E7)
- **Standards:** api-design (model the real contract)
- **Details:** Capability is proven by a well-formed v2 identify result, not a product-name string.

#### `src/internal/cmuxctl/real.go` — v2 envelope + error code type (Fix B, E8) and plaintext detection (Fix C, E2/E3)

- **Change:**
  - `rpcErrorPayload.Code`: `int` → `string` (cmux v2 codes are strings: `auth_required`, `invalid_params`, …).
  - Add `OK *bool \`json:"ok"\`` to `rpcResponse`; treat `ok==false` (or `error!=nil`) as failure and surface `error.code`/`error.message`. Keep success = `ok==true`/no error → `result`.
  - In `dial()`, wrap `conn` in a `*bufio.Reader` and build `json.NewDecoder(bufReader)` from **that** reader (so peeked bytes are not lost). Before decoding a response, `Peek(1)`; if the first non-space byte is not `{`/`[`, `ReadString('\n')` and return a typed `errCmuxPlaintext{raw}` (ANSI-stripped) instead of a JSON error. The existing per-call timeout still covers the peek; `disconnect()` closing `conn` unblocks it.
- **Evidence:** (E2), (E3), (E8); validation V4, V6
- **Standards:** error-handling (typed, prefixed); concurrency (peek/decode on the captured reader, dial-scoped)
- **Details:** `bufio.Reader` is created in `dial()` and stored alongside `conn`; `disconnect()` still closes `conn`. Decoder reads via the buffered reader so a successful JSON response after a peek is intact.

#### `src/internal/cmuxctl/preflight.go` — accurate classification (Fix C, E4/E5)

- **Change:** Replace the condition-5 catch-all. Order: (a) if the `SystemIdentify` error is `errCmuxPlaintext` whose text contains "Access denied" / "only processes started inside cmux" / "Unable to verify client process" → emit: `cmuxctl: cmux denied access — run pr9k from a terminal pane inside the cmux session (cmux defaults to cmuxOnly mode), or set cmux's socket control mode to allow-all. cmux said: <verbatim>`; (b) if it is a v2 `error` with code `auth_required`/`auth_*` → a distinct password-auth message; (c) if `system.identify` succeeded but the v2 result has no `socket_path` → a genuine "unexpected cmux identify response" message; (d) otherwise keep "version incompatible". Delete the `id.Name != "cmux"` check; replace with "successful v2 identify (socket_path present)".
- **Evidence:** (E4), (E5), (E7); validation V1 (match both denial strings)
- **Standards:** error-handling; api-design
- **Details:** Mirrors the existing condition-3 EACCES guidance so both rejection mechanisms read consistently.

#### `src/internal/cmuxctl/preflight_test.go`, `cmuxctl_test.go`, `cmuxctl_internal_test.go`

- **Change:** Fake cmux servers for: (1) pre-request `ERROR: Access denied …\n` then close → assert the new descendants message, **not** "version incompatible"; (2) v2 success `{"id":1,"ok":true,"result":{"socket_path":"/x"}}` → preflight passes; (3) v2 error `{"id":1,"ok":false,"error":{"code":"auth_required","message":"…"}}` → distinct auth message and decodes cleanly (regression for E8); (4) genuine non-JSON non-`ERROR:` → still "version incompatible". Short-socket-path helper from the prior fix; `-race`.
- **Evidence:** (E2), (E7), (E8); prior-analysis coverage gap
- **Standards:** testing.md

#### `docs/how-to/setting-up-cmux.md`, `docs/features/cmux-mode.md`, `docs/code-packages/cmuxctl.md`

- **Change:** Document the v2 identify/error envelope and the new messages; troubleshooting rows for access-denied and auth-required; state the two real ways to run (inside a cmux pane; or allow-all mode) and that `brew --cask cmux` == the native app. Update cmuxctl.md's Identity/`rpcErrorPayload` description.
- **Evidence:** (E1), (E2), (E6), (E7), (E8)
- **Standards:** documentation.md

### Rework R — the actual working solution (scoped, not line-specified)

R is a feature-sized rebuild of pr9k's cmux mode against verified cmux v2 behaviour. Scope:

- **R1 — v2 client conformance pass:** audit every RPC pr9k uses against cmux v2 at the pinned commit. Replace `surface.spawn`/`surface.hide` (nonexistent, E9). Send `surface.split`/`surface.create` with the real params (`direction`, `type`, `working_directory`, `initial_command`) (E10). Verify result shapes for `workspace.current` (pr9k expects `{name}`), `workspace.list`, `surface.split` (`pane_id`) against cmux v2's actual `ref`/`*_id` shapes (the v2Identify shape proves these differ). Adopt the v2 `{ok,result,error}` envelope (Patch P/B) throughout.
- **R2 — pane-architecture rethink (D-3/D-4):** cmux v2 has no `surface.hide`. The "hidden orchestrator pane + 3 visible panes" design must be re-expressed in v2 primitives (`surface.split`/`surface.create` with `initial_command`, `surface.close`, `surface.move`, `pane.*`), or the orchestrator-as-hidden-pane decision revisited. This is a design decision, not a code tweak — it touches the cmux-rebuild feature spec and decision log.
- **R3 — end-to-end verification gate:** the only acceptance signal is a real `pr9k --cmux` run from inside a cmux pane (cmux 0.64.6) that creates the panes and runs a workflow. Preflight passing is necessary but not sufficient (the prior `/run` fix taught this).
- **Effort:** R is comparable to the original cmux-mode build, not a patch. Recommend a dedicated planning pass (treat like a new phase of the cmux-rebuild plan) before implementation.

### Operational reality for the user (no sugar-coating)

1. **There is no working configuration of `pr9k --cmux` against cmux 0.64.6 today** — not from inside a pane, not with allow-all, not native vs brew. The feature is non-functional against the cmux it pins (E7–E10). Patch P makes it tell the *truth* instead of "version incompatible"; it does **not** make it work.
2. To actually get a working `--cmux`, Rework R must ship. Until then, run pr9k **without** `--cmux` (standard TUI) — that path is unaffected and fully working.
3. Your native-vs-brew question: **one install, not the cause.** `brew install --cask cmux` *is* `/Applications/cmux.app` (v0.64.6); `/opt/homebrew/bin/cmux` is its CLI shim. Switching between them changes nothing.
4. When R lands, the supported flow will be: launch `pr9k --cmux` from a terminal pane *inside* cmux (cmux's default `cmuxOnly` security requires a descendant; `isDescendant` is process-ancestry, V2), or set cmux to allow-all (D-20). Fully automatic "any terminal" remains the deferred [auto-launch-design.md](auto-launch-design.md).

## Validation Results

A dedicated `adversarial-validator` pass challenged the evidence, root cause, and fix; findings re-verified against the user's exact pinned commit `2f96c15c2`.

- **V1 — both denial strings (Confirmed):** cmux emits `ERROR: Access denied — only processes started inside cmux can connect` (non-descendant) *and* `ERROR: Unable to verify client process` (pid-unreadable). Impact: Fix C matches both phrases. No root-cause change.
- **V2 — Ghostty red herring (Confirmed):** `isDescendant` is a `sysctl` PID-ancestry walk, not TTY/session/Ghostty-based. Using Ghostty as your terminal does not make you a cmux descendant. Confirms the root cause; only "launched by cmux" counts.
- **V3 — `system.identify` has no `name`/`version` (Refuted the original fix → CRITICAL):** verified at commit `2f96c15c2` that `v2Identify` returns `{socket_path,focused,caller}`. The original single-part fix was a phantom fix. Impact: added **Fix A**; deleted the `id.Name != "cmux"` check.
- **V4 — error `code` is string not int (Refuted the original fix → CRITICAL):** verified `v2Error(code: String)` at `2f96c15c2`. pr9k's `int` Code makes every cmux error response fail to decode, so the planned "detect auth_required from JSON" was unreachable. Impact: added **Fix B** (Code→string, parse `ok`).
- **V5 — operational advice sufficiency (Confirmed):** the single pr9k orchestrator process (inside a pane) dials once and reuses the connection; Docker containers don't dial the socket; cmux-spawned panes are descendants. So "inside a cmux pane" is sufficient for the access layer — once A+B land.
- **V6 — bufio/peek integration (Confirmed feasible):** sound iff the `bufio.Reader` is created at `dial()` scope and the `json.Decoder` wraps it; per-call timeout covers the peek; `disconnect()` closing `conn` unblocks it. Impact: Fix B details specify dial-scope.

### Adjustments Made

- **A1 (V3):** Added Fix A — Identity modelled as cmux v2 (`socket_path`); capability check = successful v2 identify, not `name=="cmux"`.
- **A2 (V4):** Added Fix B — `rpcErrorPayload.Code` → string; parse v2 `ok`/`error` envelope.
- **A3 (V1):** Fix C matches both denial strings.
- **A4 (V6):** Fix B specifies dial-scoped `bufio.Reader` feeding the decoder.
- **A5 (V3/V4):** Root cause reframed from one defect to a fundamental API mismatch.
- **A6 (post-validation deep check, E9/E10):** Verified the other RPCs against the same pinned commit and found `surface.spawn`/`surface.hide` **do not exist** and `surface.split` has a different contract. Escalated from "three-part patch" to "Patch P (truthful errors) + Rework R (v2 rebuild)"; the patch alone cannot make `--cmux` work.

### Confidence Assessment

- **Confidence:** High. Every claim — `cmuxOnly` default, plaintext `ERROR:`, `v2Identify` shape, string error codes, **absent `surface.spawn`/`surface.hide`**, `surface.split` requiring `direction`+`initial_command` — is verified against the user's exact cmux commit `2f96c15c2` and the user's own config.
- **Remaining Risks:**
  - **Rework R is unscoped in detail by design.** R1/R2 require a design pass against cmux v2's surface/pane model and a D-3/D-4 rethink; estimating it precisely needs that pass. The risk is treating R as small — it is not.
  - Result-shape conformance of `workspace.current`/`workspace.list`/`surface.split` is asserted-mismatched-by-analogy (the v2Identify shape proves cmux uses `ref`/`*_id`), not each individually traced — R1 must trace every one.
  - cmux's `cmuxOnly` security is by design and unchanged by P or R; the supported flow is always "inside a cmux pane" or allow-all.
  - Patch P's plaintext string-match is fragile to cmux wording changes (falls back to the still-improved generic path; acceptable).

## Final Summary

- **Root Cause:** pr9k's cmux mode was built against an assumed API that materially does not match cmux v2 (verified at the user's pinned commit `2f96c15c2`): default `cmuxOnly` rejects non-descendant pr9k with an unparsed plaintext `ERROR:` that is mislabeled "version incompatible" (E1–E5); `system.identify` has no `name`/`version` (E7); error `code` is a string not int (E8); and `surface.spawn`/`surface.hide` don't exist while `surface.split` needs `direction`+`initial_command` pr9k never sends (E9, E10).
- **Fix:** Two deliverables — **Patch P** (Fixes A+B+C: truthful errors, v2 envelope, accurate access-denied/auth messages) makes pr9k stop lying and pass preflight; **Rework R** (reimplement `cmuxctl` + `RunPhase1` against the real cmux v2 surface/pane API and rethink the hidden-orchestrator design) is the actual working solution and is feature-sized.
- **Why Correct:** cmux source at the exact pinned commit proves the identify shape, string error codes, and the absence of `surface.spawn`/`surface.hide`; the user's config confirms default `cmuxOnly`; adversarial validation refuted the original one-line fix and the deep RPC check refuted even the three-part patch as sufficient.
- **Operational answer:** There is **no working `pr9k --cmux` against cmux 0.64.6 today**; use pr9k without `--cmux` until Rework R ships. Patch P only makes the errors honest. Native app == brew cask; one install; not the cause.
- **Remaining Risks:** Rework R must be planned as its own effort and gated on an end-to-end in-pane run, not on preflight passing.
