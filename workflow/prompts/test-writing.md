@.pr9k/artifacts/test-plan.md
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".
# Context
Issue #{{ISSUE_ID}}: {{ISSUE_BODY}}
Project card:
{{PROJECT_CARD}}
Diff since iteration start:
{{PRE_REVIEW_DIFF}}

If the .pr9k/artifacts/test-plan.md file is empty, non-existent, or otherwise says there is nothing to be done, skip to step 3.
1. Write all tests specified in .pr9k/artifacts/test-plan.md
2. Run all tests, type checks, linting and formatting tools. Fix any issues.
3. Delete .pr9k/artifacts/test-plan.md
4. Commit changes in a single commit.
5. Add a comment to github issue {{ISSUE_ID}} with your progress
6. Append your progress to .pr9k/artifacts/progress.txt
7. Append all deferred work to .pr9k/artifacts/deferred.txt

Budget: write all tests first, then run the suite ONCE. If >5 tests fail, fix them in batch rather than one at a time. Do not exceed 8 minutes of wall-clock test execution.

Never commit any file in .pr9k/artifacts/
