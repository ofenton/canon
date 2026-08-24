# feat-021: Validate the hierarchy against an existing log

## Context

feat-020 introduced typed nesting and left a hole its own evidence named: nothing checked a
tightened hierarchy against nestings that already existed. `CheckMigration` had done exactly this
job for states since feat-004 and simply did not know about types.

## Design notes

**The two checks report together.** An operator who fixes the states, restarts, and is then told
about the nestings has been made to do the work twice. One run says everything.

**Nestings are reported per offending shape, not per issue.** "A task under a feature" is the
decision to make; the same illegal shape repeated fifty times is one decision, not fifty. The
offending pairs are listed under it, capped at five with a count, because a long list helps
nobody find the shape.

**Removing the hierarchy is a narrowing, not a widening.** A schema with no hierarchy permits no
nesting at all (feat-020), so dropping the block while issues are nested is refused — which is
consistent, but easy to get backwards if you think of an absent hierarchy as "unconstrained".

**Widening is always applicable.** Adding a level, or turning on `allow_skipping`, cannot
invalidate an existing tree. The tests assert that rather than leaving it to reasoning.

## Evidence

**Verified by:** implementing session, `inc/feat-021-hierarchy-migration`

### A narrowed hierarchy is refused, naming the offending issues

Built a real tree, edited `canon.yaml` to move `feature` below `task`, and restarted:

```
$ canon serve
canon: schema does not fit the existing log: this schema change would leave existing issues invalid:
  the new hierarchy does not permit feature under epic: 1 nesting(s) — F→E
  the new hierarchy does not permit story under feature: 1 nesting(s) — S→F

move the issues, or keep the schema as it is
```

The server refuses to start rather than running with data its own rules forbid.

```
--- PASS: TestMigrationRefusesANarrowedHierarchy
```

The test also asserts that still-legal nestings — `task under story`, `bug under story` — are
**not** reported. A check that flags correct data alongside incorrect data trains people to skim it.

### Removing the hierarchy is refused while anything is nested

```
--- PASS: TestMigrationRefusesRemovingTheHierarchy
```

Clears every parent and asserts the same change then applies.

### A compatible change still applies

```
--- PASS: TestMigrationAllowsAWiderHierarchy
```

An unchanged schema, and one that turns on `allow_skipping`, are both applicable.

### States and nestings are reported in one run

```
--- PASS: TestMigrationReportsStatesAndNestingsTogether
```

### Scope

`git diff --cached --stat main` — run. `CheckMigration` split into two focused helpers plus the
new nesting check, and four tests.

### Not verified

The check runs at startup and when a schema is explicitly applied. Nothing watches `canon.yaml`
for edits while the server is running, so a change made to a live instance takes effect on the
next restart — where this check will catch it.

Dependencies are not validated against schema changes, because no schema rule constrains them.
If a future increment restricts which types may depend on which, this is where that belongs.

CI runs on the pull request.
