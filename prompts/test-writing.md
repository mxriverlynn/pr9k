@test-plan.md
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".
# Context
Issue #{{ISSUE_ID}}: {{ISSUE_BODY}}
Project card:
{{PROJECT_CARD}}
Diff since iteration start:
{{PRE_REVIEW_DIFF}}

If the test-plan.md file is empty, non-existent, or otherwise says there is nothing to be done, skip to step 3.
1. Write all tests specified in test-plan.md
2. Run all tests, type checks, linting and formatting tools. Fix any issues.
3. Delete test-plan.md
4. Commit changes in a single commit.
5. Append your progress to progress.txt
6. Append all deferred work to deferred.txt
Never commit test-plan.md
Never commit progress.txt
Never commit deferred.txt

Budget: write all tests first, then run the suite ONCE. If >5 tests fail, fix them in batch rather than one at a time. Do not exceed 8 minutes of wall-clock test execution.
