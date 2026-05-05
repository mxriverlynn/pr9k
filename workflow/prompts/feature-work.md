@.pr9k/artifacts/progress.txt
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".

# Context
Issue #{{ISSUE_ID}}: {{ISSUE_BODY}}
Project card:
{{PROJECT_CARD}}

Implement github issue #{{ISSUE_ID}} in the current branch (do not switch branches) using strict TDD self-healing. ONLY WORK ON A SINGLE TASK.

## UI Design References

Build the processed image set BEFORE you start sending images to the vision API. Anything that fails fetch, MIME check, or decode must be dropped here — once the vision API returns "Could not process image", the whole step aborts and the per-image skip clause cannot recover.

### 1. Enumerate image candidates

Scan the issue body and `gh issue view {{ISSUE_ID}}` output for image URLs. Treat all of these as candidates:

- `user-images.githubusercontent.com/...` and `private-user-images.githubusercontent.com/...` (real GitHub uploads on this issue)
- Inline markdown image links to `github.com/OWNER/REPO/raw/REF/PATH` (web-UI raw URL — cookie-auth only, must be rewritten)
- Inline markdown image links to `raw.githubusercontent.com/OWNER/REPO/REF/PATH`
- Other direct image URLs in the body

If there are zero candidates, skip to the TDD self-healing loop.

### 2. Fetch each candidate with auth

`GH_TOKEN` is forwarded into the sandbox. Use it. Do NOT use plain unauthenticated `curl` against `github.com/.../raw/...` — that path is cookie-auth only and will silently return a 404 HTML page that looks like a small PNG to a naive extension check.

For each candidate URL, save the raw bytes under `.pr9k/artifacts/ui-designs/raw/` and treat any non-2xx response as a hard skip (do not save a body on failure):

- `github.com/OWNER/REPO/raw/REF/PATH` → fetch via `gh api -H 'Accept: application/vnd.github.raw' "repos/OWNER/REPO/contents/PATH?ref=REF"`. Check the exit code; non-zero is a hard skip.
- `raw.githubusercontent.com/...` → `curl -fsSL -H "Authorization: Bearer $GH_TOKEN" -H "Accept: application/vnd.github.raw" -o <path> <url>`. The `-f` flag turns 4xx/5xx into a non-zero exit so the body is not written.
- `user-images.githubusercontent.com/...` and `private-user-images.githubusercontent.com/...` → `curl -fsSL -H "Authorization: Bearer $GH_TOKEN" -o <path> <url>`.
- Anything else → `curl -fsSL -o <path> <url>`.

### 3. Validate bytes before trusting the extension

For each saved file, run BOTH checks. If either fails, drop the file and continue:

- **MIME check.** `file --mime-type -b <path>` must return one of `image/png`, `image/jpeg`, `image/webp`, `image/gif`. Reject anything beginning with `<!DOCTYPE`, `<html`, or anything `file` reports as `text/html` / `text/plain`.
- **Decode probe.** `sips --getProperty pixelWidth <path>` (macOS) or `ffprobe -v error -show_entries stream=width -of csv=p=0 <path>` (Linux) must succeed and return a positive integer.

### 4. Build the processed set

Skip animated GIFs and non-raster formats (`.svg`, `.heic`, `.bmp`, etc.).

For each image that passed steps 2 and 3, write a processed copy under `.pr9k/artifacts/ui-designs/processed/`:

- If the file is already PNG or JPEG and is at most 1568 px on its longest edge and at most ~1 MB on disk, copy it as-is.
- Otherwise, re-encode/downscale to JPEG or PNG, capped at 1568 px on the longest edge and roughly 1 MB on disk. `sips` (macOS) or `ffmpeg` is available in the sandbox.

### 5. Log and reference

Append one line per candidate to `.pr9k/artifacts/ui-designs/fetch-log.txt` with the URL, the outcome (`fetched` / `http-<code>` / `not-an-image:<mime>` / `decode-failed` / `processed`), and the reason. This is the audit trail when the user asks why an image was missing.

Reference only the files under `.pr9k/artifacts/ui-designs/processed/` as visual references. If that directory is empty after step 4, proceed to the TDD loop without any UI references and note in `.pr9k/artifacts/progress.txt` that no design images could be loaded for this issue (with the URLs and reasons from the fetch log).

## TDD self-healing loop

1. Write acceptance tests derived from the issue's acceptance criteria. Run the test suite and confirm the new tests FAIL for the right reason. If a new test passes before any production code is written, rewrite it so it actually exercises the new behavior.

2. Enter the loop. MAXIMUM 20 iterations. Each iteration:
   - Run the project's full verification command (tests + lint + vet / type-check). For this repo's Go code that is `make ci`; for other stacks use the equivalent (e.g. `npm run check`, `pytest && ruff check`).
   - Parse the output, pick ONE failing test, and make the smallest production-code edit that advances it.
   - Rerun verification.
   - If a previously-passing test regresses, REVERT the last edit and try a different approach.
   - Append one JSON line per iteration to `.pr9k/artifacts/tdd-log.txt` in the target repo's working directory: `{"n": N, "duration_s": S, "outcome": "red|green|reverted", "note": "..."}`. Do NOT write to `iteration.jsonl` — that file is owned by pr9k itself and writing to it will corrupt the run.

3. Exit only when all tests pass AND lint/vet/type-check are clean. If the 20-iteration cap is hit first, stop, record the blocked state, and continue with the obligations below — do not keep looping past the cap.

4. Write a short summary after the loop: total iterations used, time-to-green (or time-to-blocked), and any abandoned approaches. Include this summary in the github issue comment and the .pr9k/artifacts/progress.txt entry.

## After the loop

5. Check off each completed Acceptance Criterion in github issue #{{ISSUE_ID}}.
6. Add a comment to github issue {{ISSUE_ID}} with your progress and the TDD summary.
7. Append your progress to .pr9k/artifacts/progress.txt.
8. Append all deferred work to .pr9k/artifacts/deferred.txt.
9. Commit changes in a single commit.

Never commit any file in .pr9k/artifacts/
