# Issue tracker: GitHub

Issues and specs live in GitHub Issues for
`princeflaco/golang-policy-inference-case`. Use the `gh` CLI.

## Conventions

Run commands inside this clone, or pass
`--repo princeflaco/golang-policy-inference-case`.

- Create: `gh issue create --title "..." --body "..."`
- Read: `gh issue view <number> --json number,title,body,labels,comments`
- List: `gh issue list --state open --json number,title,body,labels,assignees`
- Comment: `gh issue comment <number> --body "..."`
- Apply/remove labels: `gh issue edit <number> --add-label "..."` /
  `--remove-label "..."`
- Close: `gh issue close <number> --comment "..."`

Use a heredoc or `--body-file` for multiline bodies.

When a skill says "publish to the issue tracker", create a GitHub issue.
When it says "fetch the relevant ticket", read the issue and its comments.

## Pull requests as a triage surface

**PRs as a request surface: no.**

If enabled later, use `gh pr view`, `gh pr diff`, `gh pr comment`,
`gh pr edit`, and `gh pr close` for the corresponding operations.
For external PRs, exclude authors whose association is OWNER, MEMBER,
or COLLABORATOR.

GitHub shares issue and PR numbers. When the surface is unclear, try
`gh pr view <number>`, then `gh issue view <number>`.

## Wayfinding operations

- Map: one issue labelled `wayfinder:map`, containing Notes,
  Decisions-so-far, and Fog.
- Child: link each ticket as a GitHub sub-issue where supported.
  Otherwise, add it to the map's task list and put `Part of #<map>`
  at the top of the child body.
- Type: use `wayfinder:research`, `wayfinder:prototype`,
  `wayfinder:grilling`, or `wayfinder:task`.
- Blocking: prefer native issue dependencies. Add one with
  `gh api --method POST repos/{owner}/{repo}/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>`.
  Obtain the database ID with
  `gh api repos/{owner}/{repo}/issues/<blocker> --jq .id`.
  If unavailable, record `Blocked by: #<number>` in the child body.
- Frontier: select the first open child in map order with no assignee
  and no open blockers.
- Claim: `gh issue edit <number> --add-assignee @me` before working.
- Resolve: comment with the answer, close the child, and append a
  concise finding and link to the map's Decisions-so-far.
