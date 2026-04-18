@.ralph-cache/iteration.jsonl
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".
1. Read `.ralph-cache/iteration.jsonl` — one JSON record per step. Records with `notes` mentioning deferred work are the primary source; also check the current branch changes for full context.
2. Create a new issue with a `deferred` label and the full context of everything that was deferred, where, and why.
3. Delete all content from deferred.txt, leaving an empty file in place.
Never commit deferred.txt
