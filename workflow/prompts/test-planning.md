@progress.txt
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".
# Context
Issue #{{ISSUE_ID}}: {{ISSUE_BODY}}
Project card:
{{PROJECT_CARD}}

1. Run /test-planning against commits starting with {{STARTING_SHA}}, without the edge case testing agent, and write the test plan to test-plan.md
2. Add a comment to github issue {{ISSUE_ID}} with your progress
3. Append your progress to progress.txt
4. Append all deferred work to deferred.txt
Never commit test-plan.md
Never commit progress.txt
Never commit deferred.txt
