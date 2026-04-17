---
name: plan-to-issues
description: Split the contents of a design doc or plan into issues that are posted to the github repo issue list
---

## Context

Before splitting, confirm with the user:
1. The plan/design source (file path, pasted text, or conversation context). If none was provided, ask what issues to create and what source to use.
2. The target repository (defaults to the current repo's `origin`).
3. The `[tag]` prefix to use for visual grouping.

## Discovery

Before splitting, inspect:
- The plan/design document itself.
- Existing labels in the target repo (`gh label list`) — you'll need a `ready` label; create it if missing (`gh label create ready --color 0E8A16 --description 'Ready to be picked up'`).
- The repo's existing issue conventions if the plan references prior work.

## Splitting Work

Split the plan into small slices, each representing one deliverable. Every issue must include:

- A `[tag]` prefix in the title for visual grouping.
- Full description of work to be done.
- Implementation hints — design choices and expected files to work within.
- Plan for tests to write.
- Acceptance criteria to know when the work is done.
- A list of dependency issues (by their sequential number in this split) that block this work.

If any of these are missing for an issue, the split is probably wrong — consolidate or rework.

## Show the Work Split

Show a table (pre-post preview so the user can approve):

- Sequential issue number (starting at 1) and title.
- Short description — 1 or 2 sentences max.
- Dependency sequential numbers.

**Stop and ask the user to confirm the split before posting.** Do not proceed without an explicit yes.

## Post the Issues to Github

Post in **topological order** — blockers first — so dependencies can be linked as they're created.

For each issue:
- Title: `[tag] ...`
- Body: full description, implementation hints, test plan, acceptance criteria.
- Assignee: the current github user (obtain via `scripts/get_gh_user` or `gh api user --jq .login`).
- Label: `ready` (and any other labels the user specified).

Capture each created issue's `number` and node `id` from `gh issue create ... --json number,id,url`. You need both: `number` for display, node `id` for the dependencies API.

After all issues are created, link dependencies in a second pass. For each issue with blockers, call the issue dependencies REST endpoint once per blocker:

```
gh api \
  --method POST \
  -H 'Accept: application/vnd.github+json' \
  /repos/{owner}/{repo}/issues/{issue_number}/dependencies/blocked_by \
  -f issue_id=<blocker_node_id>
```

Notes:
- `issue_number` is the dependent issue's number; `issue_id` is the blocker's node `id` (not its number).
- If the endpoint path differs in the current API version, check `gh api /repos/{owner}/{repo}/issues/1/dependencies` or the GitHub REST docs and adjust.
- Same-repo dependencies only.

## Final Issue List

Show a table (post-creation confirmation with live issue numbers and URLs):

- GitHub issue number, title, and URL.
- Short description — 1 or 2 sentences max.
- Dependency GitHub issue numbers.
