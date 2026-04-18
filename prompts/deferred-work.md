@.ralph-cache/iteration.jsonl
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".
1. Read `.ralph-cache/iteration.jsonl` for step status context (failed records with `notes` carry prep error details). Also read `deferred.txt` and the current branch changes — these are the primary sources for deferred work content.
2. Create a new issue with a `deferred` label and the full context of everything that was deferred, where, and why.
3. Delete all content from deferred.txt, leaving an empty file in place.
Never commit deferred.txt
