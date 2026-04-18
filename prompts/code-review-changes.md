@progress.txt
You will likely need TodoWrite for tracking multi-step progress on this task. Preload once via ToolSearch query "select:TodoWrite".
# Context
Issue #{{ISSUE_ID}}: {{ISSUE_BODY}}
Project card:
{{PROJECT_CARD}}
Diff since iteration start:
{{PRE_REVIEW_DIFF}}

1. run a /code-review for the changes made since commit sha {{STARTING_SHA}}, and write the full review content to code-review.md
2. Append your progress to progress.txt
3. Append all deferred work to deferred.txt
Never commit code-review.md
Never commit progress.txt
Never commit deffered.txt
