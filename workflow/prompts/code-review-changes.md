@.pr9k/artifacts/progress.txt
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".

1. run a /code-review for all changes made on the current branch
   - write the full review content to .pr9k/artifacts/code-review.md
   - If no changes need to be made, write EXACTLY the 14-byte sequence `NOTHING-TO-FIX` into code-review.md — with no heading, no code fences, no quotes, no indentation, and no content before or after. Any other content means code changes are required.
2. Append your progress to .pr9k/artifacts/progress.txt
3. Append all deferred work to .pr9k/artifacts/deferred.txt

Never commit any file in .pr9k/artifacts/
