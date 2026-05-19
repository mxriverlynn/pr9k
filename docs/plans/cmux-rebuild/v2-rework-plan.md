# Rework R: rebuild pr9k cmux mode against the real cmux v2 API

**Status:** Plan for review. Investigation that mandated this: [access-denied-misclassification-investigation.md](access-denied-misclassification-investigation.md). Every API fact below is verified against the user's pinned cmux commit `2f96c15c2` (cmux 0.64.6) by reading `Sources/TerminalController.swift` via `gh api … ?ref=2f96c15c2`.

## Why a rebuild (not a patch)

pr9k's `internal/cmuxctl` client + `RunPhase1` were written against an assumed JSON-RPC API. The real cmux v2 socket API differs in **every** dimension pr9k touches: envelope, identity, error codes, method names, params, result shapes, and the workspace identity model (UUIDs/refs, not names). No subset of small fixes makes `--cmux` work.

## Verified cmux v2 API (@ `2f96c15c2`)

**Transport:** newline-delimited JSON, one object per line.
- Request: `{"id":<any>,"method":"<string>","params":{...}}` — extra fields (e.g. `jsonrpc`) ignored; `id` echoed back unchanged.
- Success: `{"id":<id>,"ok":true,"result":{...}}`
- Error: `{"id":<id>,"ok":false,"error":{"code":"<string>","message":"<string>","data":<any?>}}`
- **Pre-request rejection (cmuxOnly default):** server writes a bare line `ERROR: Access denied — only processes started inside cmux can connect\n` (or `ERROR: Unable to verify client process\n`) and closes — before any request is read.
- **Handles:** every entity is returned as both a UUID id (`workspace_id`) and a short ref string (`workspace_ref` = `"workspace:N"`, also `pane:N`, `surface:N`, `window:N`). Inputs accept either the UUID or the ref.

**Methods pr9k needs:**

| pr9k need | cmux v2 method | key params | result |
|---|---|---|---|
| capability/identity | `system.identify` | — | `{socket_path, focused:{…refs/ids}, caller}` — **no name/version**; success+`socket_path` = "is cmux v2" |
| prior workspace (for restore) | `workspace.current` | — | `{workspace_id, workspace_ref, window_id, window_ref, workspace:{…}}` (err `not_found` if none) |
| create the pr9k workspace | `workspace.create` | `title`, `working_directory`, `initial_command`, `initial_env`(map), `description`, `focus` | `{workspace_id, workspace_ref, window_id, window_ref}` — first terminal surface runs `initial_command` |
| add a display surface | `surface.split` | **`direction`** (`left\|right\|up\|down`, required), `type`(default `terminal`), `working_directory`, `initial_command`, `surface_id`(split source; default focused), `workspace_id`/`_ref`(target; default current), `focus` | `{surface_id, surface_ref, pane_id, pane_ref, workspace_id/_ref, window_id/_ref, type}` |
| (alt) add surface as new tab | `surface.create` | `type`, `working_directory`, `initial_command`, workspace target | surface/pane refs |
| close a surface | `surface.close` | `surface_id`(default focused), workspace target | refs (err `invalid_state` if last surface) |
| restore focus | `workspace.select` | `workspace_id` (UUID/ref) | refs |
| dismiss the pr9k workspace | `workspace.close` | `workspace_id` (UUID/ref) | refs (err `protected` if pinned) |
| dismissal/poll | `workspace.list` / `surface.list` | — | `{workspaces:[…]}` / surface list |

**Gone / wrong in pr9k today:** `surface.spawn` (does not exist), `surface.hide` (does not exist), workspace-by-`name` create/close/select (cmux keys on `workspace_id`; there is no unique name — only a non-unique `title`), `workspace.list → []string` (real: object with `workspaces[]`), `Identity{Name,Version}`, `rpcErrorPayload.Code int`, JSON-only decode (misses the plaintext `ERROR:`), `surface.split` without `direction`/`initial_command`.

## R2 decision — orchestrator/pane architecture (CONSEQUENTIAL — needs sign-off)

cmux v2 has no `surface.hide` and no `surface.spawn`. The documented design ([decision-log D13 / D-3 / D-4]: "orchestrator runs in a hidden cmux pane; it spawns 4 panes and hides itself") **cannot be implemented in cmux v2.** It must change.

**Recommended architecture (A):** *the orchestrator is the in-pane pr9k process; there is no hidden 4th pane.*

- The operator runs `pr9k --cmux` from a terminal pane inside cmux. **That process is the orchestrator** — it already runs `RunPhase1` and the workflow; it does not need to be a separate cmux surface.
- It calls `workspace.create` (title `pr9k-<sanitized>-<ts>`, `working_directory=projectDir`, `initial_command="<exe> cmux-pane --role=log"`, `initial_env=PR9K_*`) → the workspace's first surface is the **log** pane.
- Then `surface.split` ×2 (`direction:"up"` then `"down"`, `initial_command="<exe> cmux-pane --role=header"` / `--role=footer"`) → **header** and **footer** panes.
- The orchestrator streams state to the 3 panes over the existing interaction-channel Unix socket (unchanged). Its own stdout is the operator's original launching pane (acceptable — it's transient bootstrap output, then it blocks on the dismissal observer).
- Teardown: `workspace.close{workspace_id}` then `workspace.select{prior workspace_id}`.

Why (A): eliminates the two nonexistent primitives entirely; 3 surfaces not 4; matches cmux v2's "create-with-command" model; minimal new concepts. Rejected (B) "keep 4, fake-hide via layout/close" — fights the API, fragile.

**This revises cmux-rebuild D-3/D-4 and the feature spec.** Implementation must update the decision log + feature doc (an ADR-style amendment), not just code. **This is the decision that needs explicit approval before the rebuild, because it changes a documented core architecture of the cmux-rebuild feature.**

## Implementation sequence (after R2 sign-off)

1. **`internal/cmuxctl` envelope+errors:** rewrite `rpcResponse` to `{id, ok *bool, result json.RawMessage, error}`; `rpcErrorPayload.Code` → `string`; success = `ok==true`/no error; typed `CmuxError{Code,Message}`. Add `bufio.Reader` peek in `dial()`; classify pre-request plaintext `ERROR:` → typed `ErrAccessDenied`/`ErrPlaintext`.
2. **Identity:** `Identity{ SocketPath string \`json:"socket_path"\` }`; `Preflight` capability = successful `system.identify` with non-empty `socket_path`; classify `ErrAccessDenied` → accurate "run inside a cmux pane / allow-all" message; auth `error.code=="auth_required"` → password message; else generic.
3. **Handle model:** introduce a `Workspace` handle (`ID`, `Ref`) and `Surface` handle; methods take/return handles, not names. `CmuxClient` interface changes accordingly (`WorkspaceCreate(opts) (Workspace,error)`, `WorkspaceClose(Workspace)`, `WorkspaceSelect(Workspace)`, `WorkspaceCurrent() (Workspace,error)`, `SurfaceSplit(SplitOpts) (Surface,error)`, `SurfaceClose(Surface)`, …). Drop `SurfaceSpawn`/`SurfaceHide`.
4. **`RunPhase1` rewrite** to architecture (A): create workspace (log surface) → 2 splits (header/footer) → dismissal observer on `workspace.list` by `workspace_id` → teardown by id. Remove orchestrator-pane + hide steps. Keep interaction-channel + role sentinels unchanged.
5. **Fakes/tests:** rewrite `FakeClient` + cmuxctl/cmd tests to the v2 envelope/handles; add servers for: v2 success, v2 error (string code), pre-request plaintext `ERROR:`, `surface.split` missing-direction guard. Short-socket-path helper. `-race`.
6. **Docs:** `cmux-mode.md`, `setting-up-cmux.md`, `cmuxctl.md`, decision-log/feature-spec amendment for R2.
7. **`make ci` green.**
8. **R3 end-to-end gate (human):** `pr9k --cmux` from inside a cmux 0.64.6 pane creates the 3 panes and runs a workflow. Preflight passing is **not** acceptance.

## Risks / open items

- **OI-R1:** `workspace.create`'s first surface uses `initial_command` — confirm the surface is a terminal that runs it as a shell command with `initial_env` (params imply yes; verify in R3).
- **OI-R2:** `surface.split` `direction` semantics + `surface_id` targeting across the just-created workspace (split must target the new workspace's surface, via `workspace_id` + `surface_id`/focused) — verify pane layout in R3.
- **OI-R3:** other RPCs (`surface.list` shape for the dismissal observer) still need their result shapes traced before step 4.
- **OI-R4:** no version bump (branch policy this session).
- This is feature-sized; estimate after R2 sign-off and OI-R3 tracing.
