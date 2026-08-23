# feat-009: Queries and boards as saved queries

## Context

Constitution rule 9: boards hold no state of their own. This is where that becomes real rather
than aspirational.

## Design notes

**The language is deliberately small**: term, comparison, negation, implicit AND, substring. There
is no OR — two queries are two boards, and that has covered every case so far. JQL is what the far
end of this road looks like, and the increment itself warned that a query language is the easiest
thing here to over-build.

**Every key and value is checked against the schema.** A query for a field the organisation does
not have is refused, naming the valid keys, rather than returning an empty list. Silently matching
nothing is the least helpful possible response to a typo, because it looks exactly like "no work".

**`category` exists so queries survive a rename.** Filtering on `category=closed` keeps working
when an organisation renames its closing states; filtering on `state=done` does not. That is the
payoff for having a closed category set at all.

**Boards are state, not policy.** They are created and discarded as attention moves, so they live
in the log rather than `canon.yaml` — the same split as team membership. The query inside one is
still validated at save time, so a board cannot be created against a field that does not exist.

**A board that no longer parses returns an error, not an empty board.** The schema can move under
a saved board; an empty board reads as "no work", which is a lie worth avoiding.

**State columns follow the schema's declared order.** Alphabetical columns put `abandoned` before
`in_progress` and make a board unreadable.

## Bug found

**The projection was silently dropping every field set at creation time.** `issue.created` read
only `title` and `state` from the payload, so an issue created with a priority lost it. Nothing
noticed until a query tried to filter on one — the API accepted the write, the event contained the
data, and the projection quietly discarded it.

Fixed by copying every non-reserved payload key into `Fields`, with a regression test in
`projection` asserting fields survive and that the reserved keys do not leak in as duplicates.
`Type` is now projected too, for the same reason.

This is the sort of defect that only appears when a second feature reads what a first feature
wrote, and it is an argument for building the domain before the UI rather than after.

## Evidence

**Verified by:** implementing session, `inc/feat-009-queries`

### A board is a saved query and a grouping key, with no state of its own

```
platform  (team=platform  grouped by state)
  todo           CANON-2
  in_progress    CANON-1

→ move CANON-2 to in_progress; nothing writes to the board

platform  (team=platform  grouped by state)
  in_progress    CANON-1, CANON-2
```

```
--- PASS: TestBoardMembershipFollowsTheData
--- PASS: TestBoardsHaveNoStoredMembership   (rendering a board appends zero events)
--- PASS: TestBoardFollowsTheDataOverHTTP
--- PASS: TestGroupingKeys
```

### An issue stops appearing with no separate update

Covered above and by `TestBoardMembershipFollowsTheData`, which moves an issue out of a board's
query and asserts it leaves, then out of the query entirely.

### A query referencing a field not in canon.yaml is rejected, naming the valid fields

```
GET /api/issues?q=storyPoints=8
400  query key "storyPoints" is not a field in this organisation's schema;
     valid keys are actor, category, component, evidence, parent, priority, state, team, title
```

```
--- PASS: TestRejectsUnknownKeysAndValues   (7 cases)
--- PASS: TestQueryValidationAtTheBoundary
--- PASS: TestBoardQueryIsValidatedOnSave
--- PASS: TestFiltering                     (10 cases)
```

Values are checked too: `state=shipped` and `priority=urgent` are refused, not just unknown keys.

### Scope

`git diff --cached --stat main` — run:

```
 internal/api/api.go                    | 120 ++++   query support, four board routes
 internal/api/api_test.go               |  79 +++    contract and board tests
 internal/enforce/board.go              |  71 +++    saved boards
 internal/projection/projection.go      |  76 +++    Board entity, the fields fix
 internal/projection/projection_test.go |  37 +++    regression test for the fields bug
 internal/query/query.go                | 229 +++    the language
 internal/query/query_test.go           | 246 +++    tests
 specs/increment-plan.md                |   2 +-    status
```

The projection fix is outside the strict scope but is this increment's own blocker; recorded above.

**No README changes.** The intended documentation edit anchors on the `## Agents` section, which
exists only on feat-008's branch (PR #12) and not on main. The patch asserted its anchor and
failed rather than applying somewhere wrong — which is the behaviour I want, but it means the
README still says queries and boards are "Planned".

### Not verified

**The README is stale in two places**: queries and boards are still listed under "What is not
built", and the four board routes are undocumented. This needs a follow-up commit once PR #12 and
this one are both merged — the second time a cross-branch README edit has had to be deferred,
which is an argument for documenting in a single increment at the end of the week rather than
per feature.

No pagination. A query returning ten thousand issues returns ten thousand issues. Fine at the
current scale, wrong for `feat-012`'s dataset; noted rather than left to be discovered.

CI runs on the pull request.
