@.pr9k/iteration.jsonl
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".

The goal here is not to document every single code pattern you run across. Instead, you're to identify and document patterns that help ensure we are preventing future problems, not repeating mistakes, and writing easily maintainable code through consistent coding techniques.
1. Read `.pr9k/iteration.jsonl` — one JSON record per step. Focus on `status: "failed"` entries and repeated patterns across iterations.
2. Analyze those findings together with the current branch changes, categorizing lessons learned.
3. Write or update /coding-standard documents from those findings. Commit changes.
4. After analyzing, truncate `.pr9k/iteration.jsonl` (empty the file, do not delete it).

Never commit .pr9k/artifacts/progress.txt
Never commit .pr9k/artifacts/deferred.txt
