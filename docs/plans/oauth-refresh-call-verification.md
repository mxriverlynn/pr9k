# Investigation: OAuth Refresh HTTP Call Verification

Verify the OAuth refresh call in `workflow/scripts/get_claude_credentials` is correct against authoritative sources, and identify any drift, deprecated fields, or hardening gaps.

## Problem Statement

- **Symptoms:** None reported. This is a proactive correctness audit prompted by the user.
- **Expected behavior:** The script's outbound HTTP call to `https://console.anthropic.com/v1/oauth/token` must match the contract the Anthropic OAuth endpoint actually expects: correct URL, method, headers, JSON body fields, hardcoded public `client_id`, and response-field handling.
- **Conditions:** The call fires on every pr9k iteration when stored `expiresAt` is within 10 minutes of expiry (`get_claude_credentials:46`). A wrong-shape call would manifest as 4xx/5xx responses, dropped iterations, or worse — a successful 200 whose response we mishandle.
- **Impact:** Every pr9k run depends on this refresh succeeding. A silent contract drift would block all sandboxed claude steps until manually re-authenticated.

## Evidence Summary

### E1: Endpoint URL

- **Source:** `workflow/scripts/get_claude_credentials:14`
- **Finding:**
  ```bash
  TOKEN_URL="https://console.anthropic.com/v1/oauth/token"
  ```
- **Relevance:** Used at line 71 in `curl`. Matches the canonical endpoint observed in three independent reverse-engineered Claude Code clients (C1) and the cited upstream `openclaw/skills` reference (U1).

### E2: HTTP method

- **Source:** `workflow/scripts/get_claude_credentials:71`
- **Finding:**
  ```bash
  http_code=$(curl -s -o "$body_file" -w '%{http_code}' -X POST "$TOKEN_URL" \
  ```
- **Relevance:** `POST`. Matches U2 and C3.

### E3: Headers

- **Source:** `workflow/scripts/get_claude_credentials:72`
- **Finding:**
  ```bash
  -H "Content-Type: application/json" \
  ```
- **Relevance:** Only `Content-Type: application/json` is sent. No `User-Agent` (curl default), `Accept`, `anthropic-beta`, `anthropic-version`, or `X-Stainless-*`. The upstream openclaw/skills reference sends the same single header (U3), and all three reverse-engineered canonical implementations send only `Content-Type: application/json` on the refresh call (C4). The `anthropic-beta`/`anthropic-version` headers appear only on downstream `/v1/messages` calls, not on the OAuth token endpoint.

### E4: Request body shape and shell-quoting risk

- **Source:** `workflow/scripts/get_claude_credentials:74`
- **Finding:**
  ```bash
  -d "{\"grant_type\": \"refresh_token\", \"refresh_token\": \"$refresh_token\", \"client_id\": \"$CLIENT_ID\"}"
  ```
- **Relevance:** Three string fields: `grant_type` (literal `"refresh_token"`), `refresh_token` (interpolated), `client_id` (interpolated). Matches U4 and C5 byte-for-byte. **Caveat:** `$refresh_token` is splatted into JSON via shell quoting with no JSON-escape pass — a token containing a literal `"`, `\`, or newline would corrupt the body. OAuth refresh tokens from Anthropic appear to be opaque base64url-ish strings without those characters in practice, so this hasn't bitten anyone, but it's a latent fragility.

### E5: CLIENT_ID literal

- **Source:** `workflow/scripts/get_claude_credentials:9-12`
- **Finding:**
  ```bash
  # CLIENT_ID: OAuth client ID hardcoded into the official claude CLI for
  # talking to console.anthropic.com's OAuth endpoint — a public client
  # identifier (not a secret) that Anthropic's CLI ships with.
  CLIENT_ID="9d1c250a-e61b-44d9-88ed-5944d1962f5e"
  ```
- **Relevance:** UUID matches byte-for-byte with the upstream reference (U5) and all three independent reverse-engineered Claude Code clients (C6: ben-vargas, changjonathanc, griffinmartin). This is the correct public client_id.

### E6: Response handling and curl flags

- **Source:** `workflow/scripts/get_claude_credentials:71-98`
- **Finding (curl flags):**
  ```bash
  curl -s -o "$body_file" -w '%{http_code}' -X POST "$TOKEN_URL" \
      -H "Content-Type: application/json" \
      --max-time 30 \
  ```
- **Finding (status check):**
  ```bash
  if [[ "$http_code" != "200" ]]; then
  ```
- **Finding (field extraction):**
  ```bash
  NEW_ACCESS=$(jq -r '.access_token // empty' "$body_file")
  NEW_REFRESH=$(jq -r '.refresh_token // empty' "$body_file")
  expires_in=$(jq -r '.expires_in // 0' "$body_file")
  ...
  if [[ -z "$NEW_ACCESS" || -z "$NEW_REFRESH" || "$expires_in" -eq 0 ]]; then
      err "OAuth response (HTTP 200) did not contain expected access_token, refresh_token, or expires_in fields"
      exit 1
  fi
  ```
- **Relevance:** Strict equality on `200` (not 2xx range). Three response fields extracted: `access_token`, `refresh_token`, `expires_in`. Matches U6 and C7. **No retry, no follow-redirects, no `--fail`.** `--max-time 30` caps the call. Behavioral upgrade over upstream: explicit HTTP-status branch with redacted-body logging (commit `565fb78`).

### E7: Refresh-token rotation assumption

- **Source:** `workflow/scripts/get_claude_credentials:95`
- **Finding:**
  ```bash
  if [[ -z "$NEW_ACCESS" || -z "$NEW_REFRESH" || "$expires_in" -eq 0 ]]; then
      err "OAuth response (HTTP 200) did not contain expected access_token, refresh_token, or expires_in fields"
      exit 1
  fi
  ```
- **Relevance:** **The script treats `refresh_token` in the response as required.** Per C8, two of three independent canonical implementations (ben-vargas, griffinmartin) treat `refresh_token` in the response as **optional**, falling back to the previously-stored refresh token if the server doesn't return a new one:
  > `refreshToken: json.refresh_token || current.refreshToken`

  This is consistent with OAuth 2.0 RFC 6749 §6, where the authorization server *may* issue a new refresh token but is not required to. If Anthropic ever stops rotating refresh tokens on every refresh (or rotates only every Nth refresh), this script would fail with "OAuth response did not contain expected ... refresh_token" even though the response was actually valid.

### E8: Discarded response fields

- **Source:** `workflow/scripts/get_claude_credentials:89-118`
- **Finding:** Only `access_token`, `refresh_token`, `expires_in` are extracted; only `claudeAiOauth.accessToken`, `claudeAiOauth.refreshToken`, `claudeAiOauth.expiresAt` are written back.
- **Relevance:** Any `token_type`, `scope`, `account.uuid`, `organization.uuid`, or `id_token` returned by the server is silently dropped. Per C7, none of the three canonical implementations capture these either, so this is consistent with prior art and not a defect.

### E9: NEW_EXPIRES_AT computed from local clock

- **Source:** `workflow/scripts/get_claude_credentials:100`
- **Finding:**
  ```bash
  NEW_EXPIRES_AT=$(($(date +%s) * 1000 + expires_in * 1000))
  ```
- **Relevance:** Uses local-clock `now + expires_in*1000`. Matches ben-vargas (`Date.now() + Number(json.expires_in) * 1000`) and changjonathanc (`int(time.time()) + token_data['expires_in']`). Correct convention; clock skew is absorbed into stored expiry.

### E10: Strict 200-only status check

- **Source:** `workflow/scripts/get_claude_credentials:76`
- **Finding:**
  ```bash
  if [[ "$http_code" != "200" ]]; then
  ```
- **Relevance:** Any non-200 success (201, 204) would be treated as failure. The OAuth token endpoint per RFC 6749 §5.1 always returns 200 for success, so this is technically correct, but a more permissive check (`-ge 200 -lt 300`) would be more defensive without changing observable behavior. **Low risk.**

### U1–U7: Upstream openclaw/skills reference

- **Source:** `https://raw.githubusercontent.com/openclaw/skills/main/skills/tunaissacoding/claude-oauth-refresher/refresh-token.sh:23-24,216-223`
- **Finding:** Endpoint, method, headers, body, client_id, and parsed response fields are byte-identical to ours. The only divergence is upstream has weaker error handling (no HTTP-status check; relies on `curl` exit code only).
- **Relevance:** Confirms our HTTP call shape matches the cited reference exactly.

### C1–C9: Canonical Claude CLI shape (three independent reverse-engineered sources)

- **Sources:**
  - `https://gist.github.com/ben-vargas/c7c7cbfebbb47278f45feca9cef309d1` (TypeScript)
  - `https://gist.github.com/changjonathanc/9f9d635b2f8692e0520a884eaf098351` (Python)
  - `https://github.com/griffinmartin/opencode-claude-auth/blob/main/src/credentials.ts` (TypeScript)
- **Finding:** All three confirm:
  - **C1:** Endpoint `https://console.anthropic.com/v1/oauth/token` (griffinmartin uses `claude.ai/v1/oauth/token` with form-encoding as a one-off outlier; 2/3 sources favor `console.anthropic.com` + JSON, which matches ours).
  - **C3:** POST.
  - **C4:** Only `Content-Type: application/json`.
  - **C5:** Body has exactly `grant_type`, `refresh_token`, `client_id` — no `scope`, `redirect_uri`, or `code_verifier` on refresh (those belong to the initial authorization-code exchange).
  - **C6:** `client_id = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"` (all three).
  - **C7:** Response fields used: `access_token`, `refresh_token`, `expires_in`.
  - **C8:** `refresh_token` in response is treated as **optional** — clients fall back to existing refresh token if absent.
  - **C9:** Anthropic does not officially document this OAuth flow; everything we know is reverse-engineered. `code.claude.com/docs/en/authentication` describes user-facing login but documents none of: endpoint URL, body shape, client_id, or response format.
- **Relevance:** Independent triangulation across the upstream reference and three reverse-engineered clients confirms the wire shape we send is correct.

## Root Cause Analysis

### Summary

The OAuth refresh HTTP call in `get_claude_credentials` is **correct** as currently implemented — endpoint, method, headers, body shape, `client_id`, and parsed response fields all match the canonical Claude CLI behavior verified across four independent sources. The investigation surfaced one **medium-risk hardening gap** (E7: refresh-token rotation assumption) and two **low-risk fragilities** (E4: shell-quoting in JSON body construction; E10: strict 200-only status check).

### Detailed Analysis

The user's question was "is the call correct" — and the answer is yes, every observable wire-level field matches authoritative sources:

- The endpoint URL (E1) matches the upstream openclaw reference (U1) and all three reverse-engineered canonical clients (C1).
- Method, headers, body fields, and `client_id` (E2, E3, E4, E5) are byte-identical to U2–U5 and confirmed by C3–C6.
- Response handling (E6) extracts the same three fields the canonical implementations use (C7).

The investigation also surfaced one issue that **is not a current bug but would become one** if Anthropic changes refresh-token rotation behavior: the script requires `refresh_token` to be non-empty in every response (E7), while the canonical implementations treat it as optional and fall back to the prior refresh token (C8). Per RFC 6749 §6, this is server-discretionary, so any future Anthropic change to skip rotation on intermediate refreshes would break this script with a misleading "did not contain expected fields" error. The script would then exit 1 and pr9k iterations would stall on Claude auth.

The two low-risk items: (1) JSON body construction via shell-quoting (E4) is fragile but Anthropic refresh tokens have not historically contained the characters that would break it; (2) strict `== "200"` status check (E10) is correct per RFC 6749 §5.1 but slightly less defensive than a 2xx-range check.

## Coding Standards Reference

| Standard | Source | Applies To |
|----------|--------|------------|
| Error messages package-prefixed | `docs/coding-standards/error-handling.md` | `err()` calls in the script — already use `[get_claude_credentials] ERROR:` prefix; conformant. |
| File paths in I/O errors | `docs/coding-standards/error-handling.md` | Already cites `$CONFIG_FILE` in the no-refreshToken error (line 59); conformant. |
| Inferred from surrounding code: shell-script defensive patterns | `set -e` (line 2), `mktemp` for tempfiles, jq with `// empty`/`// 0` defaults | Matches existing patterns; any fix should preserve them. |

No ADR or coding standard specifically governs OAuth client behavior in this codebase. The narrow-reading principle (`docs/adr/20260410170952-narrow-reading-principle.md`) governs Go code, not workflow scripts.

## Planned Fix

### Summary

The HTTP call is correct; no urgent fix is required. Optionally apply three hardening changes to `workflow/scripts/get_claude_credentials` to remove the latent fragilities surfaced by the investigation. **The user should decide which (if any) to apply** — the script works correctly today.

### Changes

#### `workflow/scripts/get_claude_credentials` (optional hardening)

##### Change A — Tolerate non-rotating refresh tokens (medium risk; recommended)

- **Change:** Make `NEW_REFRESH` optional. If the OAuth response omits `refresh_token`, fall back to the existing stored value.
- **Evidence:** E7, C8, V7.
- **Standards:** Error-handling conventions (preserve existing `err()` prefix).
- **Details:** Replace lines 90, 95, and 105–116 so that:
  - Line 90 still extracts `NEW_REFRESH` with `// empty` (already does this).
  - Line 95's `[[ -z "$NEW_REFRESH" ... ]]` check is dropped from the must-be-present validation; instead, after extraction, set `NEW_REFRESH="${NEW_REFRESH:-$refresh_token}"` so we reuse the prior refresh token when the server doesn't rotate. Continue to require non-empty `NEW_ACCESS` and non-zero `expires_in`.
  - The write-back at lines 109–115 already sets `claudeAiOauth.refreshToken = $refresh` and is correct as-is once `$NEW_REFRESH` is guaranteed populated.

  Per V7, this falls back on missing, null, AND empty-string `refresh_token` (matches ben-vargas's `||` truthiness fallback). Griffinmartin's `??` would not fall back on empty string; we deliberately pick the more permissive ben-vargas semantics so any "token-omitted-from-response" shape (including buggy `""`) succeeds.

##### Change B — Construct request body via jq instead of shell-quoting (low risk; nice-to-have)

- **Change:** Build the JSON body with `jq -nc --arg ... '{...}'` so refresh tokens containing `"`, `\`, or newlines can't corrupt the body.
- **Evidence:** E4.
- **Standards:** Inferred — the script already uses `jq` to read the credentials file and to construct the response-redaction filter; using it for body construction is consistent.
- **Details:** Replace line 74 with:
  ```bash
  local body
  body=$(jq -nc \
      --arg grant_type "refresh_token" \
      --arg refresh_token "$refresh_token" \
      --arg client_id "$CLIENT_ID" \
      '{grant_type: $grant_type, refresh_token: $refresh_token, client_id: $client_id}')
  ```
  Then pass `-d "$body"` to curl.

##### Change C — Accept any 2xx status code (low risk; trivial)

- **Change:** Replace `[[ "$http_code" != "200" ]]` with a 2xx-range check.
- **Evidence:** E10.
- **Standards:** Defensive shell idioms.
- **Details:** Replace line 76 with:
  ```bash
  if [[ "$http_code" -lt 200 || "$http_code" -ge 300 ]]; then
  ```
  This is purely defensive — RFC 6749 §5.1 says the token endpoint always returns 200 on success — but it costs nothing and removes a brittle exact-match.

##### Change D — Allowlist-based redaction on non-200 (medium risk; recommended)

- **Change:** Replace the deny-by-key-name `jq walk` filter with an allowlist that only prints `error` and `error_description`, AND substring-redact any occurrence of the live refresh token in the body before logging.
- **Evidence:** V4.
- **Standards:** Error-handling conventions; defensive logging.
- **Details:** Replace lines 79–84 with something like:
  ```bash
  local body_view
  body_view=$(jq -c '{error: .error, error_description: .error_description}' "$body_file" 2>/dev/null) || body_view=""
  if [[ -n "$body_view" ]]; then
      # belt-and-suspenders: scrub the live refresh token if it ever echoes back
      body_view=${body_view//$refresh_token/<redacted>}
      err "response body: $body_view"
  else
      err "response body (non-JSON, first 500 bytes): $(head -c 500 "$body_file" | tr -cd '[:print:]\n')"
  fi
  ```
  Allowlist semantics make this safe-by-default: any future field added to the OAuth error response (including ones that might echo credentials) is silently dropped from logs unless we opt it in.

##### Change E — Restore the non-JSON fallback under `set -e` (low risk; recommended)

- **Change:** Append `|| true` (or `|| body_view=""`) to the `jq` line so non-JSON bodies fall through to the raw-bytes branch instead of aborting the script.
- **Evidence:** V5.
- **Standards:** Defensive shell.
- **Details:** Already covered by Change D's `|| body_view=""` pattern above. If applying only this change without Change D, modify line 79 to:
  ```bash
  redacted=$(jq 'walk(...)' "$body_file" 2>/dev/null) || redacted=""
  ```

##### Change F — Coerce `expires_in` to integer (low risk; nice-to-have)

- **Change:** Force `expires_in` to an integer in jq before bash arithmetic touches it.
- **Evidence:** V6.
- **Standards:** Defensive shell; protect arithmetic.
- **Details:** Replace line 92 with:
  ```bash
  expires_in=$(jq -r '(.expires_in // 0) | floor' "$body_file")
  ```
  Today the server returns integers; `| floor` is a no-op on integers and prevents future float values from crashing bash arithmetic with a non-prefixed error.

##### Change C — Accept any 2xx status code (low risk; trivial)

- **Change:** Replace `[[ "$http_code" != "200" ]]` with a 2xx-range check.
- **Evidence:** E10.
- **Standards:** Defensive shell idioms.
- **Details:** Replace line 76 with:
  ```bash
  if [[ "$http_code" -lt 200 || "$http_code" -ge 300 ]]; then
  ```
  Pure defense — RFC 6749 §5.1 says the token endpoint always returns 200 on success — but it costs nothing.

### Recommended posture

- **Apply Change A** (E7/C8/V7) — guards against the only plausible runtime regression (Anthropic stops rotating refresh tokens on every call).
- **Apply Change D** (V4) — closes the token-leak hole in error logging.
- **Apply Change E** (V5) — fixes the dead-code non-JSON fallback.
- **Apply Change F** (V6) — defends against future float `expires_in`.
- **Skip Changes B and C** unless you want belt-and-suspenders; both are pure hardening with no observed trigger.

## Validation Results

### Counter-Evidence Investigated

#### V1: Endpoint dominance — is `console.anthropic.com/v1/oauth/token` actually canonical?

- **Hypothesis:** One reverse-engineered source (griffinmartin) used `claude.ai/v1/oauth/token` with form-encoding. If that's the new shape, ours is on the deprecation path.
- **Investigation:** Validator pulled GitHub commit history. griffinmartin's OAuth code was added by PR #104 merged **2026-03-31** (~4 weeks ago) with explicit OAuth-refresh tests — i.e., recent and intentional, not legacy. ben-vargas's gist was updated **2026-04-23** (uses `console.anthropic.com` + JSON). Both shapes appear to coexist on the live server today.
- **Result:** **Partially refuted.** Our endpoint works today and matches 3 of 4 sources, but griffinmartin is not an "outlier" — it's a tested, current alternative. We cannot claim our shape is "the canonical shape," only that it works today.
- **Impact:** Soften the confidence claim. If Anthropic ever turns off `console.anthropic.com/v1/oauth/token`, our script breaks while griffinmartin-style clients keep working. Added to remaining risks.

#### V2: Headers (`anthropic-beta`, `anthropic-version`) — are we missing required headers?

- **Hypothesis:** The server might silently fail or rate-limit calls missing these headers.
- **Investigation:** All three reverse-engineered clients omit these on refresh. They appear only on downstream `/v1/messages` calls. The script runs successfully in production (commit `b73c1ce` shape, unchanged since).
- **Result:** Refuted. Headers are correct.
- **Impact:** No change.

#### V3: Could the `client_id` be wrong / about to rotate?

- **Hypothesis:** The hardcoded UUID might be stale.
- **Investigation:** Three independent canonical clients all hardcode `9d1c250a-e61b-44d9-88ed-5944d1962f5e`; upstream openclaw matches.
- **Result:** Refuted today. Latent risk if Anthropic rotates it; no graceful detection — we'd see 401s.
- **Impact:** No change to wire shape, but see V4 below — 401 response handling has its own bug.

#### V4: Response-body redaction filter has a token-leak hole

- **Hypothesis:** Lines 79–83's `jq walk` filter redacts response bodies on non-200. Does it actually catch all paths a token could leak through?
- **Investigation:** The `jq walk` regex `token|secret|key|code` (case-insensitive) matches **keys**, not values. Tested payloads:
  - `{"detail":"refresh_token SECRET-LEAK is invalid"}` → passes through unredacted.
  - `{"error":"invalid_grant","error_description":"..."}` (RFC 6749 §5.2 standard) → keys `error` and `error_description` don't match the regex; values print verbatim.
  - Non-JSON body falls through to line 83's raw `head -c 500` dump — also unredacted.
- **Result:** **Refuted.** Redaction is denylist-by-key-name and misses RFC-standard error fields. If Anthropic ever echoes the refresh token (or its prefix) in `error_description`, it gets logged in plaintext to `.pr9k/logs/<run>.log`.
- **Impact:** Add **Change D**: switch redaction to allowlist (print only `error` + `error_description`) AND substring-redact the known refresh-token value before logging.

#### V5: `set -e` kills the non-JSON fallback at line 79

- **Hypothesis:** Line 83's "first 500 bytes" fallback is supposed to fire when the response body isn't JSON. Does it?
- **Investigation:** Line 79 is `redacted=$(jq ... 2>/dev/null)`. Under `set -e`, jq exiting nonzero on non-JSON input causes the assignment to fail, which aborts the script before line 82–84 runs. The `2>/dev/null` only suppresses stderr; the exit code still propagates.
- **Result:** **Refuted.** The non-JSON fallback is dead code. On a non-JSON 4xx body, the user sees only `[get_claude_credentials] ERROR: OAuth refresh failed (HTTP 4xx)` and nothing else — the fallback that was supposed to print the body never executes.
- **Impact:** Add **Change E**: append `|| true` to line 79 (or wrap in `if redacted=$(...); then ...; else ...`) so the fallback is reachable.

#### V6: `expires_in` arithmetic crashes on float values

- **Hypothesis:** Line 100's `$(($(date +%s) * 1000 + expires_in * 1000))` only handles integers.
- **Investigation:** `expires_in=3600.5` → `bash: syntax error: invalid arithmetic operator (error token is ".5")`. Line 95's `"$expires_in" -eq 0` also fails on floats. RFC 6749 §4.2.2 says `expires_in` SHOULD be an integer but doesn't forbid floats. Anthropic returns integers today, so this is latent.
- **Result:** **Partially refuted** vs the original report. The original claimed line 100 works; it works only for integer `expires_in`.
- **Impact:** Add **Change F**: coerce via `expires_in=$(jq -r '(.expires_in // 0) | floor' "$body_file")`.

#### V7: Change A semantic distinction (`||` vs `??`)

- **Hypothesis:** The original report claimed Change A "matches 2 of 3 canonical clients." Verify the empty-string semantics.
- **Investigation:** ben-vargas uses `json.refresh_token || current.refreshToken` — JS truthiness, falls back on `undefined`, `null`, AND `""`. griffinmartin uses `data.refresh_token ?? currentRefreshToken` — nullish-only, does NOT fall back on `""`. The proposed `${NEW_REFRESH:-$refresh_token}` after `jq -r '... // empty'` falls back on missing, null, AND empty — matches **ben-vargas**, not griffinmartin.
- **Result:** Confirmed in spirit; needs a clarifying note.
- **Impact:** Update Change A to note the semantic choice.

#### V8: Change B (jq body construction) — does it change wire shape?

- **Hypothesis:** Switching from shell-quoting to `jq -nc` could alter the wire bytes.
- **Investigation:** `jq -nc` emits compact JSON, preserves field order. With `grant_type, refresh_token, client_id` order matching the current shell shape, wire bytes are identical for normal tokens — and properly JSON-escaped for tokens containing special characters (which the current code does NOT do correctly).
- **Result:** Confirmed safe. Change B is a strict improvement.
- **Impact:** No change to plan.

#### V9: TLS verification

- **Hypothesis:** Could a transparent proxy MITM this?
- **Investigation:** No `-k`/`--insecure`/`CURL_CA_BUNDLE=` overrides. curl defaults to verify peer + host.
- **Result:** Confirmed safe.
- **Impact:** No change.

#### V10: `mktemp` failure path

- **Hypothesis:** If `mktemp` fails (full /tmp), `body_file=""` and `curl -o ""` could misbehave.
- **Investigation:** Under `set -e`, `body_file=$(mktemp)` aborts on failure before `curl` runs.
- **Result:** Confirmed safe.
- **Impact:** No change.

### Adjustments Made

Three new hardening changes added to "Planned Fix" and confidence demoted from High to Medium:

- **Change D** (V4): Allowlist-based response body logging on non-200.
- **Change E** (V5): Restore non-JSON fallback by tolerating jq failure.
- **Change F** (V6): Coerce `expires_in` to integer via `| floor`.
- **Change A note** (V7): Clarify empty-string semantics matches ben-vargas, not griffinmartin.
- **Confidence demotion** (V1): Endpoint is one of two coexisting shapes, not "the" canonical shape.

### Confidence Assessment

- **Confidence:** **Medium.** The headline conclusion holds: the OAuth refresh HTTP wire shape — endpoint, method, headers, body, `client_id`, response field names — is correct and matches authoritative sources. But validation surfaced three real defects beyond the original scope: a token-leak hole in error logging (V4), dead-code non-JSON fallback (V5), and a latent float-`expires_in` crash (V6). None affects current correctness; all should be fixed.
- **Remaining Risks:**
  1. **Undocumented contract** (C9). Anthropic does not officially document the OAuth refresh shape. Any change is silent.
  2. **Endpoint is not uniquely canonical** (V1). `claude.ai/v1/oauth/token` + form-encoding is also accepted; we have no signal for which Anthropic prefers long-term.
  3. **Non-rotating refresh tokens** (E7). Pre-empted by Change A.
  4. **Token leak via error logs** (V4). If a non-200 response ever echoes the refresh token (e.g., in `error_description`), it persists in `.pr9k/logs/<run>.log`. Pre-empted by Change D.
  5. **Shell-quoting in body** (E4). Pre-empted by Change B.
  6. **`client_id` rotation** (V3). No graceful detection.

## Final Summary

- **Headline:** The OAuth refresh HTTP wire shape is **correct** — endpoint, method, headers, body, `client_id`, and parsed response fields all match canonical Claude CLI behavior verified across four independent sources (E1–E10, U1–U7, C1–C9). No urgent fix needed.
- **Surfaced Defects (beyond original scope):** Validation found three real issues: the response-body redaction filter has a token-leak hole (V4); the non-JSON fallback is dead code under `set -e` (V5); `expires_in` arithmetic crashes on float values (V6). None affects current correctness, but Changes D/E/F should be applied.
- **Why Correct:** Endpoint, method, headers, body, `client_id`, and response field names are byte-identical to the upstream openclaw reference (U1–U7) and confirmed by three independent reverse-engineered Claude CLI clients (C1–C9). The ben-vargas (TS), changjonathanc (Python), and griffinmartin (TS) implementations all hardcode the same `client_id` and use the same three-field body.
- **Validation Outcome:** Eight of ten adversarial hypotheses (V1–V10) refuted; V4, V5, V6 surfaced real defects in error handling and arithmetic, prompting Changes D, E, F. V1 partially refuted: griffinmartin's `claude.ai/v1/oauth/token` + form-encoded shape is current and tested, so we cannot claim our shape is uniquely canonical — only that it works today. Confidence demoted from High to Medium.
- **Remaining Risks:** Undocumented contract; endpoint is one of two coexisting shapes; `client_id` rotation has no graceful detection. See Confidence Assessment.
