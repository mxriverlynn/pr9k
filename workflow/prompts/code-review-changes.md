@progress.txt
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".

1. run a /code-review for all changes made on the current branch (since it diverged from the default branch), and write the full review content to code-review.md. If no changes need to be made, write EXACTLY the 14-byte sequence `NOTHING-TO-FIX` into code-review.md — with no heading, no code fences, no quotes, no indentation, and no content before or after. Any other content means code changes are required.
2. Append your progress to progress.txt
3. Append all deferred work to deferred.txt
Never commit code-review.md
Never commit progress.txt
Never commit deffered.txt
