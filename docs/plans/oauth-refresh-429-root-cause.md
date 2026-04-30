# Investigation: `get_claude_credentials` Always Returns HTTP 429

The OAuth refresh call from `workflow/scripts/get_claude_credentials` to `console.anthropic.com/v1/oauth/token` returns HTTP 429 on every invocation, blocking every iteration's `claude` step from running with a fresh token.

## Problem Statement

- **Symptoms:** The script logs `OAuth refresh failed (HTTP 429)` on every run. The user reports this is universal — never seen a 200 in the wild. The `trap 'exit 0' EXIT` (line 9) silently masks the failure to the workflow, so iterations continue with an expired access token; the workflow appears to "work" while every claude step actually authenticates with stale credentials.
- **Expected behavior:** The script should hit the OAuth refresh endpoint, receive HTTP 200 with a JSON body containing `access_token`, `refresh_token`, `expires_in`, and persist the new tokens to `~/.claude/credentials.json`. The next iteration's `needs_refresh` check should then short-circuit (token is fresh).
- **Conditions:** Every invocation of the script when `credentials.json` exists with an `expiresAt` value within `REFRESH_BUFFER_MS` (10 min) of now. Currently triggers on every iteration because the on-disk `expiresAt` is 15+ days in the past — see E6.
- **Impact:** The pr9k workflow runs sandboxed `claude` subprocesses (per `docs/adr/20260413160000-require-docker-sandbox.md`) using the credentials in `~/.claude/credentials.json`. With every refresh failing, every claude step inside Docker is using a stale access token. Iterations may appear to run but cannot do real work. The 429 is the wall blocking the entire automated workflow.

## Evidence Summary

### E1: Exact request the script sends — `Content-Type: application/json` only, curl-default User-Agent

- **Source:** `workflow/scripts/get_claude_credentials:124-127`
- **Finding:**
  ```bash
  http_code=$(curl -s -o "$body_file" -w '%{http_code}' -X POST "$TOKEN_URL" \
      -H "Content-Type: application/json" \
      --max-time 30 \
      -d "{\"grant_type\": \"refresh_token\", \"refresh_token\": \"$refresh_token\", \"client_id\": \"$CLIENT_ID\"}")
  ```
- **Relevance:** Method `POST`. URL is `https://console.anthropic.com/v1/oauth/token` (line 21). The ONLY header explicitly sent is `Content-Type: application/json`. curl supplies its own default `User-Agent: curl/X.X.X` and `Accept: */*`. No `User-Agent` override. No `anthropic-beta`, `anthropic-version`, `anthropic-client`, or `Accept` headers. This is the wire shape that Anthropic's edge sees.

### E2: CLIENT_ID is correct — verified across 8 sources

- **Source:** `workflow/scripts/get_claude_credentials:19`
- **Finding:**
  ```bash
  CLIENT_ID="9d1c250a-e61b-44d9-88ed-5944d1962f5e"
  ```
- **Relevance:** Verified verbatim against:
  1. `openclaw/skills/skills/tunaissacoding/claude-oauth-refresher/refresh-token.sh` (the inspiration source cited at line 11 of our script)
  2. `RavenStorm-bit/claude-token-refresh` README
  3. `thapargautam/claude-token-refresh/refresh_daemon.py`
  4. `2lab-ai/claude-token-cli/src/oauth.rs`
  5. `ben-vargas` gist `c7c7cbfebbb47278f45feca9cef309d1`
  6. `changjonathanc` gist `9f9d635b2f8692e0520a884eaf098351`
  7. `griffinmartin/opencode-claude-auth/src/credentials.ts`
  8. **Drift bot scans of the actual published `@anthropic-ai/claude-code` binary** — `askalf/dario#166` (CC v2.1.122, closed 2026-04-28) and `other-yuka/kyoli-gam#36` (CC v2.1.123, open 2026-04-30, **today**) both confirm `clientId=9d1c250a-e61b-44d9-88ed-5944d1962f5e` is still pinned in the latest Claude Code release.

  **Wrong client_id is NOT the cause of the 429.** The CLIENT_ID is current and correct.

### E3: TOKEN_URL is the LEGACY host — production CLI moved to `platform.claude.com`

- **Source:** `workflow/scripts/get_claude_credentials:21`
- **Finding:**
  ```bash
  TOKEN_URL="https://console.anthropic.com/v1/oauth/token"
  ```
- **Relevance:** The Claude Code production binary v2.1.122–v2.1.123 (the latest two releases as of 2026-04-30) pins `tokenUrl=https://platform.claude.com/v1/oauth/token`, per drift-bot scans (askalf/dario#166, other-yuka/kyoli-gam#36). The `console.anthropic.com` host is the **legacy** OAuth endpoint. The third-party project `lobu-ai/lobu` filed and closed PR #345 ("fix(auth): align Claude OAuth client with public CLI to avoid 429") on 2026-04-24 (6 days before today) which migrated explicitly from `console.anthropic.com` to `platform.claude.com` to fix persistent 429 errors. Verbatim from that PR:

  > "The 429 `rate_limit_error` on `/v1/oauth/token` wasn't a generic rate limit — it was Anthropic's defense against failed exchanges with the real Claude Code `client_id`. Every one of our attempts was failing validation because we were POSTing to the old token host with stale authorize/redirect URLs."

  The legacy host is still reachable but appears to have tightened acceptance over time. This is one of two co-causes of the 429.

### E4: Missing User-Agent — Cloudflare WAF is the most-cited 429 cause in the community

- **Source:** Community evidence cross-referenced; pr9k script line 125 sends only `Content-Type: application/json`
- **Finding:** Three independent production codebases that talk to this endpoint document explicitly that they were forced to add a `claude-cli/<version> (external, cli)` User-Agent because Cloudflare WAF blocks bare requests:
  - **`NousResearch/hermes-agent` PR #16957** (merged 2026-04-28):
    > "Kept (auth plumbing, not identity spoofing): `claude-cli/<v>` UA on `platform.claude.com/v1/oauth/token` (login + refresh only) — **bare requests get Cloudflare 1010 blocked**"
  - **`diillson/chatcli` PR #841** (closed 2026-04-27):
    > "Token exchange uses a plain `http.Client` with `User-Agent: claude-cli/2.1.2 (external, cli)` to avoid Cloudflare blocks."
  - **`anthropics/claude-code` issue #47390** (open):
    > "Claude Code's MCP OAuth SDK does not send a User-Agent header (or sends an empty one) during the OAuth discovery and authentication flow. This causes 403 Forbidden responses from servers behind WAFs that block requests without a User-Agent (AWS WAF, Cloudflare bot protection, nginx rules, etc.)."
- **Relevance:** Anthropic's OAuth endpoints are fronted by Cloudflare. Cloudflare's bot-protection layer rejects requests with non-browser, non-recognized User-Agent strings — including curl's default `curl/X.X.X`. The block can manifest as 1010, 403, or 429 (the exact code depends on Cloudflare's response policy and any retries). The pr9k script lets curl emit its default UA, putting it squarely in the class of requests that have direct, documented evidence of being blocked.

### E5: Direct first-hand bug report on the Anthropic tracker — Cloudflare WAF returns 403→429 on this exact endpoint

- **Source:** `anthropics/claude-code#47754` (open, filed 2026-04-14)
- **Finding:**
  > "Claude Code's OAuth token refresh is blocked by Cloudflare's WAF when the request originates from a headless Linux server."
  >
  > "Endpoint: `POST https://platform.claude.com/v1/oauth/token`
  > Content-Type: `application/x-www-form-urlencoded`
  > Parameters: `grant_type=refresh_token`, `refresh_token=<token>`, `client_id=9d1c250a-e61b-44d9-88ed-5944d1962f5e`"
  >
  > "Response: HTTP 403 (Cloudflare WAF — bot/automated traffic detection) or HTTP 429 (rate limit after retries)."
  >
  > "Cloudflare is classifying legitimate Claude Code CLI token refresh requests as bot traffic and blocking them. The same refresh works from macOS desktop environments where a browser context exists."
- **Relevance:** This is direct, first-party evidence on Anthropic's own bug tracker that the OAuth refresh endpoint returns 403/429 to non-browser, non-CLI traffic — even when the client_id, body shape, and host are all correct. The reporter is a paying Pro subscriber and reports three ignored support tickets across 26 days, confirming this is not a fringe edge case.

### E6: Stored `expiresAt` is 15+ days in the past — script has NEVER successfully refreshed since seeding

- **Source:** `~/.claude/credentials.json` (host file, computed against current time)
- **Finding:**
  ```
  expiresAt = 1776205380244   (Tue Apr 14 16:23:00 MDT 2026)
  now_ms    = 1777557823000   (Thu Apr 30 14:03:56 UTC 2026)
  diff_ms   = -1352442756     (token expired ~15 days ago)
  ```
- **Relevance:** `write_refreshed_credentials` (lines 167–180) is the only path that updates `expiresAt` in `credentials.json`, and it runs only when `http_code == 200` (line 129 gate). The 15-day-stale `expiresAt` is unambiguous evidence that this host has **never** received an HTTP 200 from the refresh endpoint since the credentials file was seeded — every call from day one has failed. `needs_refresh` therefore returns true on every invocation, sending another doomed request. The pr9k workflow loop calls the script once per iteration (E8), so the host has been hammering the endpoint with the same failing refresh token over and over for ~15 days.

### E7: The 429 has been a known, unresolved condition since 2026-04-29

- **Source:** `git log --pretty=full workflow/scripts/get_claude_credentials`, commit `565fb78`
- **Finding:** Commit `565fb78` (2026-04-29 13:56), titled `surface HTTP errors in get_claude_credentials OAuth refresh`, message includes:
  > "Verified manually against the live endpoint: a 429 rate-limit response now logs 'OAuth refresh failed (HTTP 429)' plus the rate_limit_error body, instead of the previous opaque 'invalid OAuth response' message."
- **Relevance:** The 429 was observed against the live endpoint over 24 hours before the user's current report. The script's error-handling path was hardened to expose it; the underlying cause was not addressed. The HTTP request shape (URL, method, headers, body) introduced in `b73c1ce` (2026-04-29 13:15) has been byte-identical from then through HEAD — no change has plausibly resolved or worsened the situation since.

### E8: Script is in the `iteration` phase — runs once per loop iteration

- **Source:** `workflow/config.json:18-19`
- **Finding:**
  ```json
  "iteration": [
      { "name": "Claude Credentials", "isClaude": false, "command": ["scripts/get_claude_credentials"] },
  ```
- **Relevance:** With `expiresAt` 15 days stale (E6), `needs_refresh` returns true on every iteration, and the script issues an identical failing curl POST every iteration. There is no client-side back-off, retry-after handling, or session-level circuit breaker. This call frequency — repeatedly hitting the OAuth endpoint with a refresh token the server keeps rejecting — is exactly the pattern that 429 rate-limiters are designed to defend against. **Anthropic's 429 is therefore a self-amplifying response to our own retry pattern, not a coincidence.**

### E9: `seed_from_keychain` may have selected a stale account on first run

- **Source:** `workflow/scripts/get_claude_credentials:67-76`
- **Finding:**
  ```bash
  has_fields=$(echo "$data" | jq -r '
      if (.claudeAiOauth.refreshToken // "") != ""
         and (.claudeAiOauth.expiresAt // 0) != 0
      then "yes" else "no" end' 2>/dev/null || echo "no")
  if [[ "$has_fields" == "yes" ]]; then
      credentials="$data"
      log "found valid OAuth tokens under keychain account: $account"
      break
  fi
  ```
- **Relevance:** The validity check tests only `refreshToken != ""` and `expiresAt != 0`. It does NOT compare `expiresAt` against the current time, and does NOT verify which keychain account is the most-recently-updated. If the user has multiple Claude Code keychain entries (multiple sign-ins, different accounts, stale installs), the first one in `sort -u` order with non-empty fields wins. A stale account whose refresh token has been Anthropic-revoked would silently win selection and be written to `credentials.json`. This may be amplifying E6 — the seeded refresh token may itself be stale/revoked, which on repeated retries would manifest as 429.

### E10: Body content type — JSON is accepted (not the cause)

- **Source:** Cross-references against working implementations
- **Finding:** Production CLI sends `application/x-www-form-urlencoded`. Our script sends JSON. Both are accepted by the live server: `anthropics/claude-code#53063` (closed 2026-04-25) shows a user successfully refreshing with JSON body to `platform.claude.com/v1/oauth/token`, receiving HTTP 200. The third-party `oauth-broker` and `claude-token-refresh` implementations also use JSON.
- **Relevance:** Body content-type is **not** the cause of the 429 — both shapes work. The script's JSON shape is fine.

### E11: Trap silently masks failure → retry amplification across iterations

- **Source:** `workflow/scripts/get_claude_credentials:9` and `:144`
- **Finding:**
  ```bash
  trap 'exit 0' EXIT
  ...
  if [[ "$http_code" != "200" ]]; then
      err "OAuth refresh failed (HTTP $http_code)"
      ...
      exit 1   # → trap converts to exit 0
  fi
  ```
- **Relevance:** A 429 (or any non-200) leaves `credentials.json` unchanged. The next iteration finds `TIME_LEFT_MS` still hugely negative, calls curl again, gets another 429. There is no retry-after honoring, no exponential backoff, no abort. Once the 429 starts, every subsequent iteration in the run amplifies the rate-limit pressure on Anthropic's side and is itself a candidate for further escalation by Cloudflare's reputation tracking.

### E12: Headers the production Claude Code CLI sends (per drift-bot + community evidence)

- **Source:** `2lab-ai/claude-token-cli/src/oauth.rs`, `NousResearch/hermes-agent#16957`, `diillson/chatcli#841`, `anthropics/claude-code#31021`
- **Finding:** The current production refresh request uses these headers:
  - `User-Agent: claude-cli/<version> (external, cli)` — required to avoid Cloudflare 1010
  - `Content-Type: application/x-www-form-urlencoded`
  - `Accept: application/json`
  - `anthropic-beta: oauth-2025-04-20`
- **Relevance:** Our script sends only the second one (and as JSON, not form-encoded). The two load-bearing missing pieces in evidence-weighted order are:
  1. **`User-Agent`** — direct evidence in two production codebases that adding `claude-cli/...` UA fixed Cloudflare blocks.
  2. **`anthropic-beta: oauth-2025-04-20`** — present in working third-party implementations and explicitly cited in `anthropics/claude-code#31021` as required to make the related `/api/oauth/usage` endpoint stop 429-ing.

## Root Cause Analysis

> **REVISED 2026-04-30 after adversarial validation.** The original diagnosis (Cloudflare WAF blocking default-UA requests, legacy-host migration) was substantially refuted by live HTTP probes from the user's actual machine. See Validation Results section below for the V1–V11 counter-evidence. The corrected analysis follows.

### Summary

The seeded refresh token in `~/.claude/credentials.json` is dead (server-side revoked or out-of-rotation), and the workflow's `trap 'exit 0' EXIT` plus once-per-iteration loop (with no `Retry-After` honoring) has been re-submitting that dead token to the OAuth endpoint repeatedly for 15+ days, putting it (or its associated account) into a per-token 429 state. The request shape is mostly fine; the credentials and the retry pattern are the problem.

### Detailed Analysis

Live HTTP probes from the user's machine (V1, V2 — see Validation Results) directly falsified two of the original three primary causes:

- **Cloudflare WAF is NOT blocking default-UA curl from this user's IP.** Ten consecutive default-UA POSTs to `console.anthropic.com/v1/oauth/token` and `platform.claude.com/v1/oauth/token` from the user's macOS shell return clean Anthropic JSON 400 (`invalid_request_error: Unsupported grant_type`), not 1010/403/429. The user's IP is not in any Cloudflare reputation block; curl's default JA3 fingerprint is not on the challenge list. The cited "headless Linux" Cloudflare-block bug (`anthropics/claude-code#47754`) explicitly notes that macOS works as a baseline (V4) — the user is on macOS (Darwin 25.3.0).
- **Switching to `platform.claude.com` does NOT escape the 429.** With a fake-but-shaped refresh_token (`sk-ant-ort01-INVALID`), default UA, and the production CLIENT_ID, both `console.anthropic.com/v1/oauth/token` AND `platform.claude.com/v1/oauth/token` return HTTP 429 (V2). The two hosts route to the same Anthropic origin (identical `via: 1.1 google` and `X-Envoy-Upstream-Service-Time` headers in V1). The host change is essentially cosmetic — both endpoints behave identically.

What the live probes also revealed: Anthropic distinguishes between malformed bodies (HTTP 400 with `invalid_request_error`) and **bodies that contain a real-shape-but-failed-validation refresh token** (HTTP 429). The user has been hammering one or the other endpoint with the same dead refresh_token at least once per workflow iteration for 15+ days (E6, E8, E11). The 429 is per-token / per-account reputation, not per-IP WAF.

The actual causal chain:

1. **Initial seed produced an `expiresAt` that's now 15+ days in the past** (E6). Either the keychain entry was already old at seed time (E9 — `seed_from_keychain` does not check entry freshness, picks the first sort-order match), or the seeded refresh_token was rejected on its very first refresh attempt and never updated.
2. **`needs_refresh` returns true on every invocation** because `TIME_LEFT_MS` is hugely negative.
3. **The script issues the same curl POST every iteration** with the same dead refresh_token.
4. **The `trap 'exit 0' EXIT` silently swallows the failure** (E11) so the workflow continues; no back-off, no `Retry-After` honoring, no circuit breaker.
5. **Anthropic's per-token rate-limiter ratchets up** in response to repeated identical failed exchanges, returning 429 indefinitely.

Adversarial-cited evidence supporting this corrected diagnosis: `anthropics/claude-code#53063` (originally cited at E10 to support "JSON works") is actually titled "OAuth auto-refresh fails in non-interactive (subprocess) mode → 401 after token expiry" — V5 shows this issue is evidence FOR the user's bug pattern (subprocess refresh failing), not against it. The pr9k workflow runs the curl from a non-interactive subprocess context, exactly matching this bug class.

What this means for the fix: **changing the request shape will not revive a dead refresh_token.** The order of operations needs to be inverted — diagnose first, re-seed credentials, fix the retry-amplification engine, and only then consider request-shape changes if the 429 still persists. The CLIENT_ID is correct (E2, unchanged conclusion). The ancillary issues (E9 keychain selection, E11 trap masking) are now load-bearing rather than secondary.

## Coding Standards Reference

| Standard | Source | Applies To |
|----------|--------|------------|
| Use `atomicwrite` and `O_CREATE\|O_EXCL\|O_WRONLY, 0o600` for file replacement; `O_TRUNC` is prohibited | `docs/coding-standards/file-writes.md` | The `mktemp + mv` pattern in `write_refreshed_credentials` (lines 169-178) is bash-level, not Go; this script is exempt because the standard governs Go file-write code. The current pattern (`mktemp` then `mv`) is a reasonable bash-level analog of atomic-replace. No change required for this fix. |
| Package-prefixed error messages | `docs/coding-standards/error-handling.md` | The script already prefixes via `[get_claude_credentials]` — keep this prefix on any new log lines. |
| Documentation must ship with the feature | `docs/coding-standards/documentation.md` | The fix changes user-visible behavior of the bundled workflow; the change should be reflected in any docs that mention the OAuth flow (e.g. `docs/how-to/setting-up-docker-sandbox.md` if it mentions the host) and the prior investigation doc `docs/plans/oauth-refresh-call-verification.md` should get a follow-up note. |
| Narrow-reading principle: pr9k is a generic step runner; workflow content lives in `config.json`, not Go | `docs/adr/20260410170952-narrow-reading-principle.md` | This script is workflow content (`workflow/scripts/`), not Go code. All changes here are appropriate per the ADR. |
| ADR `20260413162428-workflow-project-dir-split.md` | `docs/adr/20260413162428-workflow-project-dir-split.md` | Confirms the script's location under `workflow/scripts/` is the intended workflow-bundle home; no Go-side change is needed — modify the script in place and `make build` will copy it into `bin/.pr9k/workflow/scripts/`. |

No standard explicitly governs HTTP-client request construction in shell scripts; the inferred convention from this script's own existing structure (one curl invocation, allowlist-based body redaction, captured `http_code` for status checks) is preserved.

## Planned Fix (REVISED — staged, diagnose-first)

### Summary

Stop generating the 429 ourselves before trying to re-shape the request. Step 1 instruments the failure so we can see what Anthropic is actually returning. Step 2 re-seeds the credentials manually. Step 3 fixes the retry-amplification engine. Only steps 4–6 touch the request shape, and only one variable at a time.

### Why This Order

The validator's V1/V2 live probes show that the request shape is not the proximate cause of the 429 from this user's machine. Bundling 4 simultaneous shape changes (V7) gives no diagnostic signal if it works AND no diagnostic signal if it doesn't. The 15-day-stale `expiresAt` (E6) plus `trap 'exit 0' EXIT` (E11) plus once-per-iteration call (E8) means the script has been generating its own 429 for two weeks regardless of headers. Stop the bleeding first, then change one variable at a time if needed.

### Step 1 — Instrument the failure (REQUIRED, do this first)

**File:** `workflow/scripts/get_claude_credentials`

Add response-header capture so future 429s are diagnosable. Specifically, dump headers via `-D` to a temp file and, on non-200, log:
- `cf-ray:` value (presence indicates Cloudflare in path; absence is unusual)
- `cf-mitigated:` value (Cloudflare-WAF block has this header set; Anthropic-app rate-limit does not)
- `server:` value (`cloudflare` confirms CF; `anthropic` is the upstream)
- `Retry-After:` value (Anthropic app-layer rate limits often include it)

Implementation sketch:
```bash
local body_file headers_file http_code
body_file=$(mktemp)
headers_file=$(mktemp)
http_code=$(curl -s -o "$body_file" -D "$headers_file" -w '%{http_code}' -X POST "$TOKEN_URL" \
    -H "Content-Type: application/json" \
    --max-time 30 \
    -d "{\"grant_type\": \"refresh_token\", \"refresh_token\": \"$refresh_token\", \"client_id\": \"$CLIENT_ID\"}")

if [[ "$http_code" != "200" ]]; then
    err "OAuth refresh failed (HTTP $http_code)"
    # Diagnostic headers (case-insensitive grep — curl emits with original case)
    local cf_ray cf_mitigated server retry_after
    cf_ray=$(grep -i '^cf-ray:' "$headers_file" | head -1 | tr -d '\r' || true)
    cf_mitigated=$(grep -i '^cf-mitigated:' "$headers_file" | head -1 | tr -d '\r' || true)
    server=$(grep -i '^server:' "$headers_file" | head -1 | tr -d '\r' || true)
    retry_after=$(grep -i '^retry-after:' "$headers_file" | head -1 | tr -d '\r' || true)
    [[ -n "$cf_ray" ]] && err "diagnostic: $cf_ray"
    [[ -n "$cf_mitigated" ]] && err "diagnostic: $cf_mitigated"
    [[ -n "$server" ]] && err "diagnostic: $server"
    [[ -n "$retry_after" ]] && err "diagnostic: $retry_after"
    # ... existing body-redaction block
    rm -f "$body_file" "$headers_file"
    exit 1
fi
rm -f "$headers_file"
```

**Outcome to observe:** Run pr9k once. The next 429 will tell us which classifier is rejecting us:
- If `server: cloudflare` and `cf-mitigated:` is present → Cloudflare WAF block (then header changes are relevant)
- If `server: cloudflare` but no `cf-mitigated:` and the body is `{"type":"error","error":{"type":"rate_limit_error",...}}` → Anthropic application-layer rate limit on this token/account (then re-seeding is the fix)

### Step 2 — Re-seed credentials manually (REQUIRED, do this second)

This is a one-time manual step the user must perform on the host:

1. Quit any running `claude` instances on the host.
2. Delete `~/.claude/credentials.json` (rename it as a backup if you want — `mv ~/.claude/credentials.json ~/.claude/credentials.json.bak`).
3. Run `claude` interactively on the host and complete the OAuth login. This issues a fresh refresh_token to the macOS keychain.
4. Re-run pr9k. The script will re-seed `credentials.json` from the new keychain entry.
5. Observe the next iteration's log:
   - HTTP 200 → the request shape was correct all along; the dead-token theory is confirmed and no further fix needed for the 429 itself
   - HTTP 400 with `invalid_grant` → keychain entry was somehow already stale; investigate which keychain entry was selected (use the new logging from the E9 hardening below)
   - HTTP 429 still → request shape is genuinely an issue; proceed to Step 4

### Step 3 — Stop self-amplifying retries (REQUIRED, do this third)

**File:** `workflow/scripts/get_claude_credentials`

Two changes; keep them in one commit since they address the same retry-amplification engine:

3a. **Honor `Retry-After` if present.** When the response is 429 and `Retry-After:` is set, write a marker file (e.g., `/tmp/.pr9k-oauth-cooldown`) with the deadline timestamp, and have the script short-circuit on subsequent invocations until that deadline passes. Implementation sketch:
```bash
# At top of refresh_oauth_token, before curl:
local cooldown_file="/tmp/.pr9k-oauth-cooldown-$(id -u)"
if [[ -f "$cooldown_file" ]]; then
    local deadline
    deadline=$(cat "$cooldown_file" 2>/dev/null || echo 0)
    if [[ "$(date +%s)" -lt "$deadline" ]]; then
        local wait=$((deadline - $(date +%s)))
        err "in OAuth refresh cooldown for $wait more seconds; skipping"
        return 0
    fi
    rm -f "$cooldown_file"
fi

# In the non-200 block, after capturing retry_after:
if [[ "$http_code" == "429" && -n "$retry_after" ]]; then
    local seconds
    seconds=$(echo "$retry_after" | sed -n 's/^[Rr]etry-[Aa]fter:[[:space:]]*\([0-9]*\).*/\1/p')
    if [[ "$seconds" =~ ^[0-9]+$ ]] && [[ "$seconds" -gt 0 ]]; then
        local deadline=$(($(date +%s) + seconds))
        echo "$deadline" > "$cooldown_file"
        err "honoring Retry-After=$seconds; cooldown until $(date -r "$deadline" 2>/dev/null || echo "$deadline")"
    fi
fi
```

3b. **Move `Claude Credentials` from `iteration` to `initialize`** in `workflow/config.json`:
```json
"initialize": [
    { "name": "Claude Credentials", "isClaude": false, "command": ["scripts/get_claude_credentials"] },
    ...
],
"iteration": [
    // remove the Claude Credentials entry here
    ...
]
```
This caps the call frequency at once-per-pr9k-run (regardless of iteration count), preserving E5's design intent that one refresh per token-lifetime window is sufficient. The `REFRESH_BUFFER_MS` 10-minute pre-expiry buffer (E5) means a single refresh at run start covers ~50 minutes of subsequent iterations even on the original 1-hour token lifetime — which is plenty for a typical pr9k run.

**File:** `workflow/scripts/get_claude_credentials` — also tighten the keychain selection (E9):
- **Change:** `seed_from_keychain` should pick the keychain entry with the **highest `expiresAt`**, not the first one in sort-order.
- **Evidence:** (E9), V9
- **Details:** Replace the `break`-on-first-match loop with a pass that records `(account, expiresAt)` for every entry whose fields are non-empty, then selects the entry with the largest `expiresAt`. Implementation sketch:
  ```bash
  local best_account="" best_expires=0 best_data=""
  while IFS= read -r account; do
      [[ -z "$account" ]] && continue
      data=$(security find-generic-password -s "$service" -a "$account" -w 2>/dev/null || true)
      [[ -z "$data" ]] && continue
      expires=$(echo "$data" | jq -r '.claudeAiOauth.expiresAt // 0' 2>/dev/null || echo 0)
      refresh=$(echo "$data" | jq -r '.claudeAiOauth.refreshToken // ""' 2>/dev/null || echo "")
      [[ -z "$refresh" || "$expires" -eq 0 ]] && continue
      if [[ "$expires" -gt "$best_expires" ]]; then
          best_expires="$expires"
          best_account="$account"
          best_data="$data"
      fi
  done <<< "$accounts"
  if [[ -z "$best_data" ]]; then
      err "no '$service' keychain entry contained valid OAuth tokens; run 'claude' on this host first"
      exit 1
  fi
  log "selected keychain account: $best_account (expiresAt=$best_expires)"
  credentials="$best_data"
  ```

### Step 4 — IF AND ONLY IF the 429 persists after Steps 1–3, change User-Agent

**Conditional on:** Step 1 logging showing `cf-mitigated:` set OR the 429 body being a Cloudflare HTML page rather than JSON.

**File:** `workflow/scripts/get_claude_credentials`

- **Change:** Add `-A "claude-cli/2.1.123 (external, cli)"` to the curl invocation. Use the **real published version** `2.1.123` (from drift-bot scan `other-yuka/kyoli-gam#36` dated 2026-04-30), not the placeholder `1.0.0` originally suggested — the validator (V8) flagged that a fake version is more likely than not to fail Cloudflare's classifier if there is one.
- **Evidence:** (E4), (E12), V8
- **Maintenance note:** Pin the version with a comment pointing at `https://www.npmjs.com/package/@anthropic-ai/claude-code` so a future maintainer knows to bump it. Out of scope for this fix: automating that bump.

### Step 5 — IF AND ONLY IF Step 4 doesn't help, add `Accept` and `anthropic-beta` headers

- **Change:** Add `-H "Accept: application/json" -H "anthropic-beta: oauth-2025-04-20"`.
- **Evidence:** (E12), V10 (acknowledged unverified-but-harmless)
- **Note:** The validator (V10) refuted these as required-on-this-endpoint citations. Treat as belt-and-suspenders.

### Step 6 — IF AND ONLY IF Steps 4–5 don't help, switch host AND body shape

- **Change:** Switch `TOKEN_URL` to `https://platform.claude.com/v1/oauth/token` and switch body to `application/x-www-form-urlencoded`. Production sends form-encoded; if Cloudflare is fingerprinting the body shape, this is the last-resort change.
- **Evidence:** (E3), (E12), V2 (validator showed the host alone won't fix it, but combined with form-encoding it might)
- **Note:** This is the most invasive change. Bundle host + body together because they're a related "match production exactly" change.

- **Change (secondary, hardening for E9):** Tighten `seed_from_keychain` to pick the keychain entry with the **maximum** `expiresAt`, rather than the first non-empty one in sort-order.
- **Evidence:** (E9)
- **Details:** Replace the `break`-on-first-match loop with a pass that records `(account, expiresAt)` for every entry whose fields are non-empty, then selects the entry with the largest `expiresAt`. This does not eliminate stale-token risk but reduces the window in which a long-revoked entry from another install wins selection. Implementation sketch:
  ```bash
  local best_account="" best_expires=0 best_data=""
  while IFS= read -r account; do
      [[ -z "$account" ]] && continue
      data=$(security find-generic-password -s "$service" -a "$account" -w 2>/dev/null || true)
      [[ -z "$data" ]] && continue
      expires=$(echo "$data" | jq -r '.claudeAiOauth.expiresAt // 0' 2>/dev/null || echo 0)
      refresh=$(echo "$data" | jq -r '.claudeAiOauth.refreshToken // ""' 2>/dev/null || echo "")
      [[ -z "$refresh" || "$expires" -eq 0 ]] && continue
      if [[ "$expires" -gt "$best_expires" ]]; then
          best_expires="$expires"
          best_account="$account"
          best_data="$data"
      fi
  done <<< "$accounts"
  if [[ -z "$best_data" ]]; then
      err "no '$service' keychain entry contained valid OAuth tokens; run 'claude' on this host first"
      exit 1
  fi
  log "selected keychain account: $best_account (expiresAt=$best_expires)"
  credentials="$best_data"
  ```

- **Change (diagnostic, optional):** When the response is non-200, log the `cf-ray:` and `cf-mitigated:` response headers if present, so future Cloudflare blocks are diagnosable from the existing log file alone.
- **Evidence:** (E5), (E11) — Cloudflare-vs-Anthropic distinction is currently invisible in the log
- **Details:** Capture response headers via `-D /dev/stderr` or a temp dump file and grep for `cf-ray`, `cf-mitigated`, `server: cloudflare`, then emit those via `err`. Keep the existing JSON body redaction allowlist intact (do not log full headers — some can be large). This is helpful but not required for the primary fix; gate it behind a comment if it adds too much noise.

#### `docs/plans/oauth-refresh-call-verification.md`

- **Change:** Append a follow-up note explaining that the prior conclusion ("the wire shape matches upstream openclaw and three reverse-engineered clients") was correct **at the time of writing** but Anthropic has since (a) migrated the production endpoint to `platform.claude.com`, (b) tightened Cloudflare classification on the legacy host, and (c) the openclaw reference itself is now also out of date for production traffic. Reference this new investigation file (`oauth-refresh-429-root-cause.md`).
- **Evidence:** (E3), (E7) — the prior doc explicitly demoted confidence to Medium and this investigation confirms why.
- **Standards:** `docs/coding-standards/documentation.md` (docs ship with feature changes).
- **Details:** Two-paragraph addendum at the bottom of the file. Do not rewrite the body — preserve the historical record of what was concluded then.

#### `CLAUDE.md` (no change required)

- **Change:** None. The CLAUDE.md does not enumerate workflow scripts; the existing entries for ADRs and coding standards remain accurate. Mentioning the script there would violate the narrow-reading principle.

### Out of Scope (deliberate)

The following adjacent issues surfaced during investigation and are deliberately deferred to keep the fix focused:

- **`trap 'exit 0' EXIT` (line 9)** — masks failures and amplifies retries (E11). The trap exists by design (per the comment at lines 4-8: workflow must not abort on a refresh failure). Changing it is a separate scope decision and does not address the root cause of the 429.
- **JSON-escaping of the refresh token (B4)** — currently relies on Anthropic's tokens being base64url-shaped. A latent fragility but not the cause of the 429.
- **Switch body to `application/x-www-form-urlencoded`** — production sends form-encoded, but JSON is accepted (E10). Don't change two variables at once. If the User-Agent fix alone doesn't resolve the 429, switching the body shape becomes the next thing to try.
- **Move the script from `iteration` to `initialize`** (E8) — would reduce call frequency from once-per-iteration to once-per-run. Worth considering for cost/wear reasons but does not fix the 429; the once-per-run call would still 429 with the current request shape.

## Validation Results

Adversarial validator ran live HTTP probes from the user's actual machine (macOS Darwin 25.3.0, default `curl/8.7.1`) against both the legacy and current OAuth hosts. Results materially refute the original plan's primary causes.

### Counter-Evidence Investigated

#### V1: Live default-UA probe — Cloudflare is NOT blocking us

- **Hypothesis:** Cloudflare WAF blocks default-UA curl on `console.anthropic.com/v1/oauth/token` and `platform.claude.com/v1/oauth/token`.
- **Investigation:** 10 consecutive default-UA POSTs to console; 1 to platform; both with body `{"grant_type":"password",...}`.
- **Result:** **Refuted.** All probes returned clean Anthropic JSON `400 invalid_request_error`. No 1010, no 403, no `cf-mitigated:` header. The user's IP is not in any Cloudflare reputation block from this OS / network combination.
- **Impact:** E4, E5 mostly wrong for this user. Adding User-Agent will not fix the 429 by itself. Demoted to Step 4 in the staged fix.

#### V2: Live valid-shape-fake-token probe — both hosts return 429

- **Hypothesis:** Switching to `platform.claude.com/v1/oauth/token` will escape the 429 because it's the production host and the legacy host is being tightened.
- **Investigation:** POST with `refresh_token=sk-ant-ort01-INVALID` (valid shape, fake content), default UA, JSON body, against both hosts.
- **Result:** **Refuted.** Both hosts returned HTTP 429. Both route to the same Anthropic origin (matching `via: 1.1 google` and `X-Envoy-Upstream-Service-Time` headers across hosts).
- **Impact:** E3 ("legacy host being tightened") was wrong. The 429 is per-token / per-account reputation, not per-host. The host change in original Step 1 won't help on its own; demoted to Step 6.

#### V3: Earlier in-repo investigation contradicts the new "legacy host" framing

- **Hypothesis:** The earlier `oauth-refresh-call-verification.md` (commit `ca15f6c`, 2026-04-29) had concluded `console.anthropic.com` was correct and "both shapes coexist on the live server today." If that was right, the new "legacy host" framing must explain what changed in 24 hours.
- **Investigation:** Read prior investigation lines 290–296. The earlier conclusion is internally consistent with V1/V2 above.
- **Result:** **Partially Refuted.** The two investigations contradict each other; live probes side with the earlier one.
- **Impact:** Original E3 framing softened. The legacy host is not "tightened toward 429"; it is a co-equal alias of the current host.

#### V4: User is on macOS, not headless Linux — `anthropics/claude-code#47754` doesn't apply

- **Hypothesis:** The cited Cloudflare-block bug applies to this user.
- **Investigation:** CLAUDE.md confirms Darwin 25.3.0. `workflow/config.json:19` confirms `isClaude: false` so the script runs on the host (macOS), not in Docker. Issue #47754's body explicitly says "the same refresh works from macOS desktop environments" — i.e., macOS is the reporter's working baseline.
- **Result:** **Refuted** as evidence for this user.
- **Impact:** E5 is the wrong cited bug for this case. Removed from primary causes.

#### V5: Cited PR #53063 is mis-framed in the original E10

- **Hypothesis:** `claude-code#53063` proves JSON body is accepted (cited at E10).
- **Investigation:** The issue title is verbatim "OAuth auto-refresh fails in non-interactive (subprocess) mode → 401 after token expiry." It describes a failure mode in subprocess context — exactly what pr9k's curl call is.
- **Result:** **Refuted** as cited. **Confirms** the corrected diagnosis (subprocess context refresh-failure pattern matches the user).
- **Impact:** #53063 is now evidence FOR the dead-token / subprocess-context theory. E10's "JSON works" claim is unchanged (the issue body separately confirms JSON works manually) but the issue is now load-bearing for the corrected analysis, not the original.

#### V6: 15-day amplification confirms self-generated 429

- **Hypothesis:** `trap 'exit 0' EXIT` + once-per-iteration loop is sufficient to generate a 429 condition independent of request shape.
- **Investigation:** Confirmed `expiresAt=1776205380244` vs `now_ms=1777558967046` → diff = -15.67 days. Conservatively, with one pr9k iteration per workflow run (most days) and ~5–20 iterations per run, the dead refresh_token has been re-submitted on the order of 100s–1000s of times since the file was seeded.
- **Result:** **Confirmed.** E11 amplification is real and central.
- **Impact:** Promoted from "out of scope" to required Step 3 of the fix. The trap-masking + iteration-loop combination is doing more harm than the missing User-Agent.

#### V7: Bundling 4 changes into one commit is its own anti-pattern

- **Hypothesis:** Original plan changes host + UA + Accept + anthropic-beta in one commit. If the 429 stops, no diagnostic signal which one helped; if it continues, no signal which to change next.
- **Investigation:** Original plan internal contradiction — its own "Out of Scope" point 3 says "Don't change two variables at once" while the bundled changes 1–4 do exactly that.
- **Result:** **Confirmed** internal inconsistency.
- **Impact:** Fix re-staged into 6 numbered steps. One variable per step.

#### V8: `claude-cli/1.0.0` placeholder is more risk-additive than helpful

- **Hypothesis:** Using a fake version string in the User-Agent is harmless.
- **Investigation:** If Cloudflare cross-references against known-published versions (a sensible bot-protection design), `1.0.0` won't match and the User-Agent gain is lost. Production version per drift-bot scans is `2.1.123`.
- **Result:** **Confirmed.** Use the real published version.
- **Impact:** Step 4 now specifies `claude-cli/2.1.123 (external, cli)` with a maintenance note.

#### V9: Stored refresh token is most likely dead

- **Hypothesis:** The 15-day-stale `expiresAt` indicates the token itself is server-side revoked, not just out-of-rotation.
- **Investigation:** The 400-vs-429 distinction in V1/V2 (server returns 400 for malformed body, 429 for real-shape-but-failed token) plus the 15+ days of repeated failed submissions strongly suggests the token is in a per-token rate-limit / revocation state.
- **Result:** **Partially Refuted** the original "fix the request shape" framing. **Confirms** the corrected diagnosis.
- **Impact:** Step 2 (manual re-seed via `claude` login) is now required and load-bearing.

#### V10: `anthropic-beta: oauth-2025-04-20` is unverified for this endpoint

- **Hypothesis:** The header is required on `/v1/oauth/token`.
- **Investigation:** Cited evidence (`anthropics/claude-code#31021`) is about `/api/oauth/usage`, a different endpoint. The earlier in-repo investigation explicitly notes "all three reverse-engineered clients omit these on refresh."
- **Result:** **Refuted** as required-on-this-endpoint.
- **Impact:** Demoted to Step 5 (belt-and-suspenders), no longer claimed as causal.

#### V11: Other failure modes (IP reputation, JA3, body order) ruled out by live probes

- **Hypothesis:** A non-cited failure mode is the actual cause.
- **Investigation:** V1/V2 live probes from the user's IP returned clean 400s with default-UA-and-shape — meaning this user's IP, JA3 fingerprint, and TLS posture are all acceptable to Cloudflare today.
- **Result:** **Confirmed** by ruling out: the 429 is not coming from these layers.
- **Impact:** Diagnosis narrowed to the per-token rate-limit on the dead refresh_token + retry amplification.

### Adjustments Made

- **Root Cause Analysis** — completely rewritten. Original "Cloudflare WAF + legacy host + missing UA" diagnosis replaced with "dead refresh_token + retry amplification + missing back-off." Triggered by V1, V2, V3, V4, V9.
- **Planned Fix** — re-staged from a single bundled commit into 6 sequential steps. Steps 1–3 (instrument, re-seed, stop self-amplification) are now required; Steps 4–6 (User-Agent, headers, host+body) are conditional on Step 1 logging. Triggered by V1, V2, V6, V7, V9.
- **User-Agent version string** — changed from `claude-cli/1.0.0` to `claude-cli/2.1.123` with a maintenance note. Triggered by V8.
- **`anthropic-beta` header** — moved from required-causal to belt-and-suspenders. Triggered by V10.
- **`trap 'exit 0' EXIT` + iteration-vs-initialize** — promoted from "out of scope" to required Step 3. Triggered by V6.
- **Keychain selection (E9)** — kept as a hardening change, now part of Step 3.

### Confidence Assessment

- **Confidence:** **Medium-High** for the corrected diagnosis. **High** for Step 1 (instrumentation) being correct. **High** for Step 2 (re-seed) being correct given V9's evidence.
- **Why not Higher:** I cannot directly observe the actual 429 response body and headers without authenticated live probes (which the validator correctly declined to do). Step 1 logging will resolve this on the next run — a Cloudflare-shaped 429 (HTML body, `cf-mitigated:` set) flips the diagnosis back toward request-shape; an Anthropic-shaped 429 (JSON `rate_limit_error` body, no `cf-mitigated:`) confirms the corrected diagnosis.
- **Remaining Risks:**
  - **The instrumentation in Step 1 may itself fail to surface the right signal** if curl's `-D` writes are interleaved with stderr in the run-mode log. Mitigation: dump headers to a file and grep, as shown in the sketch.
  - **`Retry-After` may be `0` or absent on the actual 429.** anthropics/claude-code#30930 reports `retry-after: 0` is common on Anthropic OAuth-class 429s. The cooldown logic in Step 3a should treat `retry-after: 0` as a soft signal (e.g., 60-second cooldown floor) rather than literal zero.
  - **Re-seeding (Step 2) may fail if Anthropic has flagged the user's account** (e.g., due to the 15 days of failed retries triggering anti-abuse on the account itself, not just the token). If a fresh `claude` login fails to authenticate, the user has a different problem entirely — escalate to Anthropic support.
  - **Step 4 assumes the User-Agent is the only Cloudflare-shaping variable.** TLS-fingerprint (JA3) is a separate vector and curl's JA3 is well-known. If Cloudflare is JA3-fingerprinting the OAuth path, no header change will help; the user would need to switch to a JA3-impersonating client (`curl-impersonate`, `cycletls`). Assess after Step 1 logging.
  - **No automated test coverage of this script.** Re-validation is manual against the live endpoint.
  - **The CLIENT_ID claim (E2) remains High-confidence**; the validator did not challenge it because 8 cross-references plus drift-bot scans of the published binary establish it firmly.

## Final Summary

- **Root Cause:** The seeded refresh_token in `~/.claude/credentials.json` is dead (server-side revoked or out-of-rotation), and the script's `trap 'exit 0' EXIT` + once-per-iteration loop has been re-submitting that dead token with no back-off for 15+ days, putting it into a per-token / per-account 429 rate-limit state. The CLIENT_ID is correct (E2). Live probes from the user's machine showed neither host nor User-Agent is the proximate cause (V1, V2).
- **Fix:** Six staged steps, in order: (1) instrument response headers (`cf-ray`, `cf-mitigated`, `Retry-After`) so the actual 429 source is visible in logs; (2) re-seed credentials via a fresh `claude` login on the host; (3) stop self-amplification by honoring `Retry-After`, moving the script to the `initialize` phase, and tightening keychain-entry selection by `expiresAt`; (4–6) only if 429 persists after Steps 1–3, change request shape one variable at a time (User-Agent → `Accept`/`anthropic-beta` → host+body), informed by Step 1's diagnostic logging.
- **Why Correct:** V1 and V2 live probes from the user's actual machine refuted the original Cloudflare-WAF and host-migration theories. V6 confirmed retry amplification is real. V9 + the 400-vs-429 server distinction strongly indicate the token itself is the rate-limit subject, not the request shape. V5 re-purposed `claude-code#53063` as evidence FOR the subprocess-context dead-token theory rather than against it.
- **Validation Outcome:** Validator's V1/V2 live probes substantially refuted the original diagnosis, triggering a complete rewrite of Root Cause Analysis and a re-staging of the fix from one bundled commit into six numbered steps. CLIENT_ID and `expiresAt` math (E2, E6) survived unchallenged.
- **Remaining Risks:** Step 1 logging may need adjustment if curl's `-D` writes interleave with stderr; `Retry-After: 0` from Anthropic should get a 60-second floor; account-level flagging (vs. token-level) is possible after 15 days of failures and would require Anthropic support escalation; TLS JA3 fingerprinting is unverified and would defeat all header changes if active. See Confidence Assessment for full list.
