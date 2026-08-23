# docs-002: Documentation catch-up

## Context

Two feature increments landed on branches whose README edits could not be written without
conflicting with each other. This is the catch-up, done as one increment rather than a patch per
feature — the approach I recommended after the second deferral, and the one that avoids the
problem entirely.

## Design notes

**One documentation increment, not one per feature.** A README edit written on a feature branch
anchors on text that may only exist on another feature branch. Twice now a patch has failed its
anchor assertion for exactly that reason. Deferring documentation to a single increment removes
the class of problem, at the cost of the README being briefly stale between merges — which is
visible and recoverable, unlike a patch that silently applies in the wrong place.

**Route coverage is checked mechanically**, not by eye. A route table maintained by hand drifts,
and the drift is invisible.

**The "not built" list is checked in both directions.** Removing MCP and boards from it matters as
much as adding things to it: a list that claims something is missing when it is built costs a
reader their trust in the whole document.

## Evidence

**Verified by:** implementing session, `inc/docs-002-readme-catchup`

### Every API route currently implemented is documented

```
routes on this branch: 25   undocumented: none
```

Cross-checked mechanically against `Routes()`.

### The "not built" list contains only things genuinely not built

Removed: the MCP server (built in feat-008), queries and boards (built in feat-009). The status
banner said "no MCP server", which had been false for two merges.

Remaining, and true: authentication, web UI, flow metrics (in review as feat-010), federated
repo-local storage, Jira import.

### Every documented example behaves as shown

Run against a live instance on this branch:

```
bad query:      400        (?q=storyPoints=8)
good query:     200        (?q=team=platform priority=p1)

platform  (team=platform  grouped by state)
  todo           CANON-2
  in_progress    CANON-1
→ after moving CANON-2:
platform  (team=platform  grouped by state)
  in_progress    CANON-1, CANON-2
```

The board example in the README is this output verbatim, including the "move an issue and the
board follows" claim.

### Scope

`README.md` and this evidence file. Documentation only.

### Not verified

**Flow metrics are not documented here.** feat-010 is in review on another branch, so its routes
do not exist on this one and documenting them would describe something absent. Its README section
rides in its own pull request, as feat-008's MCP section did — which is the exception to this
increment's own rule, and defensible only because that section anchors on text feat-010 itself
adds.

CI runs on the pull request.
