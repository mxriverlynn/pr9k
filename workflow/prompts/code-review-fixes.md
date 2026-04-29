@.pr9k/artifacts/code-review.md
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".

If the .pr9k/artifacts/code-review.md file is empty, non-existant, or otherwise says nothing needs to be done, skip all to step 3.
1. Implement all identified items in .pr9k/artifacts/code-review.md
2. Run all CI checks, including tests, type checks, linting and formatting tools. Fix any issues.
3. Delete .pr9k/artifacts/code-review.md
4. Commit all changes in a single commit.
5. Append your progress to .pr9k/artifacts/progress.txt
6. Append all deferred work to .pr9k/artifacts/deferred.txt

Never commit any file in .pr9k/artifacts/
