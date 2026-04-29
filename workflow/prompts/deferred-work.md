@.pr9k/iteration.jsonl
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".

1. Read `.pr9k/iteration.jsonl` for step status context (failed records with `notes` carry prep error details)
2. Also read `.pr9k/artifacts/deferred.txt` and the current branch changes — these are the primary sources for deferred work content
3. Create a new issue with a `deferred` label and the full context of everything that was deferred, where, and why
4. Delete .pr9k/artifacts/deferred.txt

Never commit any file in .pr9k/artifacts/
