# feat-016: Dependencies with cycle warnings and reverse lookup

## Context

Jira offers blocks, is-blocked-by, relates-to, duplicates and clones, and teams spend meetings
deciding which one to use. One directed relation expresses all the ordering anyone acts on.

## Design notes

**Cycles are recorded, not refused — the opposite of the hierarchy's rule.** The difference is
deliberate and worth stating: a parent chain is a tree by definition, so a cycle there is
meaningless. Dependencies are a graph, and two pieces of work genuinely can be waiting on each
other. A tool that refuses to record that pushes the truth somewhere it cannot be seen — the same
failure as demanding a ticket reference for work that has no ticket, or letting configuration
sprawl because each addition is individually reasonable.

So the write succeeds, and the cycle is reported everywhere the issue appears: on the write, on
every member's dependency view, and in a project-wide `GET /api/cycles`. The warning says what a
cycle *means*, not just that one exists — "Nothing in this cycle can start until one of these
dependencies is removed."

**A cycle is reported once, not once per member.** `cycleKey` sorts the members so the same loop
entered from three different issues is recognised as one. Reporting it three times would train
people to ignore it.

**`blocked` is derived, never stored.** An issue is blocked when anything it depends on is not
in a state the schema calls closed. A stored field would be one more thing to keep in step, and
"closed" is asked of the schema so it means whatever the organisation says rather than a
hardcoded state name.

**Dependents are computed, not indexed.** A reverse index is a second structure to keep
consistent with the first, and the scan is fast enough at the scale the benchmark covers.

**Self-dependency is the one refusal.** It is meaningless rather than merely bad, so unlike a
genuine cycle there is nothing to record.

**A dependency on a deleted issue stops blocking but stays in the log.** The relation was a fact
when it was recorded; deleting the target does not unmake it.

**Empty lists, never nulls.** A client that has to handle both `null` and `[]` for "nothing here"
is doing work the server should have done.

## Evidence

**Verified by:** implementing session, `inc/feat-016-dependencies`

### One directed relation, both directions readable

```
UI:  depends on API · depended on by DEPLOY · blocked by API
API: depends on nothing · depended on by DOCS, UI · not blocked
```

```
--- PASS: TestDependencyIsOneDirectedRelation
--- PASS: TestReverseLookup
--- PASS: TestEmptyDependenciesAreEmptyNotNull
```

`TestDependencyIsOneDirectedRelation` scans the log and fails if any event type matching
`depend`, `block` or `relate` exists beyond the single `issue.dependency_added`.

### A cycle is recorded and warned about, not refused

```
$ curl -X PUT /api/issues/API/dependencies -d '{"on":"DEPLOY"}'
http 200 — the write succeeded
dependency cycle: API → DEPLOY → UI → API. Nothing in this cycle can start until
one of these dependencies is removed.

$ curl /api/cycles
1 cycle(s) in the project
```

```
--- PASS: TestCyclesAreRecordedAndWarnedAbout
--- PASS: TestRemoveDependencyClearsTheCycle
--- PASS: TestSelfDependencyIsRefused
--- PASS: TestDuplicateAndUnknownAreRefused
```

The test asserts the event count rose, the warning names every member, and every member keeps
reporting the cycle afterwards — not just the issue that closed it.

### Blocked derived from unfinished dependencies

```
blocked=true     API, DEPLOY, DOCS, UI
blocked=false    (none)
depends_on=API   DOCS, UI
```

```
--- PASS: TestBlockedIsDerived
--- PASS: TestBlockedAndDependsOnQueries   (6 cases)
--- PASS: TestDependencyOnADeletedIssue
--- PASS: TestDependenciesSurviveRebuild
```

`TestBlockedIsDerived` closes a blocker and asserts the dependent unblocks with nothing else
written. `TestBlockedAndDependsOnQueries` does the same through the query language.

### Scope

`git diff --cached --stat main` — run. The graph in `projection`, the rules in `enforce`, four
routes in `api`, two query keys in `query`, a `depend` verb in `schema`, and MCP descriptions.

The `depend` verb is a new operation in the permission vocabulary, which is a schema change —
but a role that could not be granted dependency permission would make the feature unusable under
any non-trivial schema.

### Not verified

**The UI shows none of this.** That is feat-018.

No transitive blocking: an issue is blocked only by its direct dependencies, not by what those
depend on. Reporting the whole chain would be more accurate and much noisier, and nobody has
asked for it yet.

CI runs on the pull request.
