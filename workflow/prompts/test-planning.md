@.pr9k/artifacts/progress.txt

You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".

# Context
Issue #{{ISSUE_ID}}: {{ISSUE_BODY}}
Project card:
{{PROJECT_CARD}}

# Work Steps
1. Run /test-planning against commits starting with {{STARTING_SHA}}, with the following constraints:
   - Do not use edge case explorer agent
   - Focus on the minimum number of tests to consider these changes stable
2. Write the test plan to .pr9k/artifacts/test-plan.md
3. Add a comment to github issue {{ISSUE_ID}} with your progress
4. Append your progress to .pr9k/artifacts/progress.txt
5. Append all deferred work to .pr9k/artifacts/deferred.txt

Never commit any file in .pr9k/artifacts/
