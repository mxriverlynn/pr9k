@.pr9k/artifacts/progress.txt
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".

# Context
Issue #{{ISSUE_ID}}: {{ISSUE_BODY}}
Project card:
{{PROJECT_CARD}}

Implement github issue #{{ISSUE_ID}} in the current branch (do not switch branches) using strict TDD self-healing. ONLY WORK ON A SINGLE TASK.

## UI Design References

If there are no image attachments on github issue #{{ISSUE_ID}}, skip to the TDD self-healing loop.

Download all images attached to the github issue
- Save images to .pr9k/artifacts/ui-designs/
- Use appropriate images as visual reference for UI development

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
