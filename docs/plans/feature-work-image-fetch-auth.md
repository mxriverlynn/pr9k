# Feature Work step "Could not process image" — root cause

## Problem Statement

**Symptom.** The "Feature work" step aborts with:

```
API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"Could not process image"},"request_id":"req_011Cajn3AeuDEM4PdBbhr313"}
```

**Reproducer.** GitHub issue `GearJot/gearjot-v2-web#263`. Issue body contains exactly one image, referenced as a markdown link:

```
https://github.com/GearJot/gearjot-v2-planning/raw/main/share-feature/phase-1-company-settings/ui-designs/detail-manager-save-error.png
```

**Conditions.** The image lives in a *different* repository (`GearJot/gearjot-v2-planning`) than the issue (`GearJot/gearjot-v2-web`), and that planning repo is private.

**Impact.** Feature work aborts on the first vision-API call after the "preprocessing" branch in `feature-work.md`. The fix added in commit `fbc79ff` does not catch this case.

## Evidence Summary

**E1 — The image URL returns HTML, not a PNG.** `curl -sLI` against the URL in issue #263:

```
HTTP/2 404
content-type: text/html; charset=utf-8
content-length: 304916
```

A `curl -sL` of the same URL writes a 304,916-byte HTML document to disk:

```
$ file test-image.png
test-image.png: HTML document text, Unicode text, UTF-8 text, with very long lines
```

The first bytes are `<!DOCTYPE html>...`. There is no PNG signature anywhere in the response.

**E2 — `gearjot-v2-planning` is private, `raw/.../main/...` is cookie-auth only.** The 404 is what GitHub serves for a private repo to an unauthenticated request. The `https://github.com/OWNER/REPO/raw/...` URL form is the *web UI* raw path; it uses session cookies for auth. It does **not** accept `Authorization: Bearer <token>`. Token-authenticated raw access has to go through `raw.githubusercontent.com` with a header, or through `gh api repos/.../contents/...`.

**E3 — `feature-work.md:15` instructs claude to download images, but specifies no auth.** From `workflow/prompts/feature-work.md`:

```
15: Download all images attached to the github issue, save them to `.pr9k/artifacts/ui-designs/`, then prepare a processed set the vision API will accept:
```

There is no instruction to use `gh`, `GH_TOKEN`, or any authenticated path. Claude defaults to `curl`/`WebFetch`, which sends no GitHub credentials.

**E4 — The preprocessing branch trusts the file extension, not the bytes.** `feature-work.md:19`:

```
- If the file is already PNG or JPEG and is at most 1568 px on its longest edge and at most ~1 MB on disk, copy it as-is.
```

The downloaded file is named `detail-manager-save-error.png` and is ~300 KB. It passes the "PNG, ≤1 MB, ≤1568 px on longest edge" gate trivially, because nothing actually decodes it to verify it's an image. Both `sips` and `ffmpeg` would fail on it, but the prompt only invokes them on the *re-encode* branch — the "copy as-is" branch never runs a decoder.

**E5 — The "skip on vision-API failure" tolerance does not save the run.** `feature-work.md:22` says to skip an image if the vision API rejects it. But the very `claude` invocation that the prompt runs *inside* is the one that hits the vision API. When the vision API returns 400 on an attachment passed at prompt-context time, the entire `claude` process exits non-zero before the prompt body can decide to "skip" anything. The tolerance clause assumes claude can recover from a per-image 400, which it cannot for an attached input.

**E6 — `GH_TOKEN` is already forwarded into the sandbox.** `workflow/config.json:1`:

```json
{
  "env": ["GH_TOKEN"],
```

So an authenticated path is available — the prompt just doesn't tell claude to use it.

**E7 — The issue body's image is a markdown reference, not a GitHub-attached upload.** The issue body uses a raw markdown image link to a third-party repo URL. It is not a `user-images.githubusercontent.com` upload (which would be public and `gh issue view` would give us an `assets` array). The prompt's wording "images attached to the github issue" is ambiguous in a way that hides this distinction.

## Root Cause Analysis

**Root cause.** The Feature work prompt tells claude to "download all images attached to the github issue" without specifying an authenticated fetch path. For issue #263, the only image is hosted in a *separate, private* GitHub repo via the cookie-auth `github.com/.../raw/main/...` URL form. An unauthenticated `curl` of that URL returns a 404 HTML page, which gets saved verbatim as `detail-manager-save-error.png`. The pre-processing logic added in `fbc79ff` checks the *extension* and the *file size* but never decodes the bytes, so HTML-masquerading-as-PNG slides through the "copy as-is" branch (E1, E3, E4). The vision API then rejects the HTML payload with `Could not process image` (E1).

A secondary failure mode amplifies this: even when an individual image fails at the vision-API layer, the prompt's "log and skip" tolerance (E5) cannot intercept the error, because the failing image is part of the same `claude` invocation's input — the process exits 400 before the prompt logic runs.

## Coding Standards Reference

- **Narrow-reading principle** (`docs/adr/20260410170952-narrow-reading-principle.md`): pr9k is a generic step runner; workflow content lives in prompts, not Go. The fix belongs in `workflow/prompts/feature-work.md`, not in pr9k Go code.
- **Documentation standards** (`docs/coding-standards/documentation.md`): documentation must ship with the change. If we change image-fetch semantics, update any how-to that describes Feature Work behavior.
- No Go code changes, so Go-specific standards (testing, concurrency, atomicwrite, etc.) do not apply here.

## Planned Fix

**Summary.** Rewrite the "UI Design References" section of `feature-work.md` so that (a) images are always fetched with `GH_TOKEN`-authenticated requests via paths that work for private cross-repo assets, (b) downloaded files are validated as real image bytes before they enter the processed set, and (c) any image we can't authenticate, decode, or convert is dropped *before* claude attaches it to its own context.

### File: `workflow/prompts/feature-work.md`

Replace the current "UI Design References" section (lines 11–22) with prose that instructs claude to:

1. **Enumerate image references from the issue.** Treat both genuine `user-images.githubusercontent.com` uploads and inline markdown image links to `github.com/.../raw/...` or `raw.githubusercontent.com/...` as candidates. (Justified by E7.)

2. **Fetch with auth, against an endpoint that accepts `Authorization`.** For each `github.com/OWNER/REPO/raw/REF/PATH` URL, rewrite to `gh api -H 'Accept: application/vnd.github.raw' repos/OWNER/REPO/contents/PATH?ref=REF` (or use `gh api` with the equivalent URL). For `raw.githubusercontent.com`, use `curl -fsSL -H "Authorization: Bearer $GH_TOKEN" -H "Accept: application/vnd.github.raw"`. Do not use plain unauthenticated `curl` against `github.com/.../raw/...` — that path is cookie-auth only and silently returns 404 HTML for private repos. (Justified by E2, E3.)

3. **Treat any non-2xx status as a hard skip.** `curl -f` (or checking the `gh api` exit code) ensures a 404 is not silently captured to disk as a fake image. (Justified by E1.)

4. **Verify each downloaded file is actually a decodable image *before* including it.** Use `file --mime-type` and confirm the result is `image/png`, `image/jpeg`, `image/webp`, or `image/gif`; reject anything starting with `<!DOCTYPE`, `<html`, or that `file` reports as `text/html`. Then run a decode probe (`sips --getProperty pixelWidth` on macOS, `ffprobe -v error -show_entries stream=width` otherwise). Only files that pass *both* checks are eligible for the "copy as-is" or "re-encode" branches. (Justified by E4.)

5. **Build the processed set first, then reference only those files.** If the processed set is empty after validation, do not pass *any* image to the vision API for this issue — proceed to the TDD loop without UI references and note the missing designs in the progress log. This eliminates the "process exits 400 before our skip clause runs" failure (E5).

6. **Log every rejection** with the URL, HTTP status (or decode error), and reason, into `.pr9k/artifacts/ui-designs/fetch-log.txt`, so future investigation does not require curl-by-hand.

### Why this fixes E1–E5

- E1/E2 fail because the URL form requires cookie auth. Switching to `gh api` / `raw.githubusercontent.com` + bearer token uses the already-forwarded `GH_TOKEN` (E6) on a transport that accepts it.
- E3 is fixed by the prompt explicitly naming the auth path.
- E4 is fixed by mandating MIME + decode verification before the "copy as-is" branch.
- E5 is fixed by moving the rejection decision to *before* the claude invocation that consumes the images, instead of relying on intra-call recovery.

### Out of scope (deferred)

- Adding a `scripts/fetch_issue_images` helper in `workflow/scripts/`. That would be cleaner than embedding shell logic in a prompt, but the narrow-reading ADR pulls toward keeping workflow content out of Go and out of new scripts unless duplicated across multiple prompts. Revisit only if a second prompt needs the same logic.
- Changing pr9k Go code. None of E1–E7 implicate the orchestrator.

## Adjustments Made

None — initial investigation produced direct, byte-level evidence (E1: the URL returns `text/html` 404 with `<!DOCTYPE html>` content). No conflicting hypotheses required adversarial validation rounds. The fix is surgical and limited to one prompt file.

## Confidence Assessment

**High** for the root cause: the failing URL was fetched in real time and returned HTML, not PNG bytes; the prompt explicitly says "download" with no auth; the preprocessing logic provably trusts extensions over bytes.

## Remaining Risks

- **Claude may still misuse `gh api` for cross-repo fetches** if the user running pr9k lacks read access to the planning repo. In that case the fetch correctly fails with 404, the image is correctly skipped, and the issue is now a *user permissions* problem rather than a vision-API crash. The prompt should make this distinction explicit in its log message.
- **Other private cross-repo URL patterns** (`objects.githubusercontent.com` signed URLs, S3-hosted attachments) are not covered by the rewrite rules above. The MIME/decode gate (step 4) is the safety net: anything that isn't a real image gets dropped before reaching the vision API.
- **The 1568 px / ~1 MB heuristic** in the existing prompt is a rough Anthropic vision recommendation, not a hard API limit. Out of scope for this fix; flag if future "Could not process image" errors recur with valid PNG bytes.

## Final Summary

- **Root cause.** The Feature Work prompt downloads issue images without GitHub auth; for issue #263 the image lives in a private cross-repo planning bucket, so `curl` saves a 404 HTML page as `.png`, and the preprocessing logic copies it through unchecked because it only inspects the file *extension* (E1, E3, E4).
- **Fix.** Rewrite `workflow/prompts/feature-work.md`'s "UI Design References" section to fetch via `gh api` / bearer-token `raw.githubusercontent.com`, treat non-2xx as hard skip, and require MIME + decode verification before any image reaches the vision API.
- **Why correct.** E1 directly demonstrates the corrupt download; E4 directly demonstrates that the existing extension/size gate cannot detect it; the rewrite addresses both.
- **Validation outcome.** No counter-evidence found; root cause is reproducible by `curl`-ing the URL and inspecting the bytes.
- **Remaining risks.** Cross-repo permissions and other URL families are addressed by the MIME/decode gate as a backstop, but won't be auth-fetchable if the user lacks access.
