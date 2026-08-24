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

## What CI found, again

`TestReadCostDoesNotTrackLogSize` failed a second time — 3.3x per-issue growth where local runs
measured flat. The first CI failure (feat-017) prompted a real fix; this one turned out to be the
instrument.

**A profile settled it.** The allocations the benchmark attributed to a read were the *fixture's*:
`seedLarge`, CBOR encoding and SQLite binding accounted for half the profile. Measured properly,
the read path allocates **1,273 per read at 10,000 issues and 1,271 at 50,000** — flat, as the
design intends.

So the test was asserting an algorithmic property using wall-clock timing on a shared runner,
where contention swamps a two-millisecond baseline. It now asserts on **allocations per read**,
which are deterministic and machine-independent, and keeps the absolute latency budget — the
actual requirement — checked on time.

**One real improvement came out of it.** `Filter` materialised every match before the handler
sliced out a page: at 50,000 issues that is a slice of roughly 33,000 pointers, ~264 KiB per read,
to return 200. `FilterPage` counts as it scans and keeps only the page, so a read allocates the
page rather than the result set. The total is still exact, so the scan is not short-circuited.

### Scope

`git diff --cached --stat main` — run. `CheckMigration` split into two focused helpers plus the
new nesting check, four tests, and the benchmark and `FilterPage` work described above.

The benchmark and pagination changes are outside the stated scope. They are here because CI went
red while verifying this increment, and leaving main broken across two increments to keep a scope
boundary tidy would be the wrong trade.

### Not verified

The check runs at startup and when a schema is explicitly applied. Nothing watches `canon.yaml`
for edits while the server is running, so a change made to a live instance takes effect on the
next restart — where this check will catch it.

Dependencies are not validated against schema changes, because no schema rule constrains them.
If a future increment restricts which types may depend on which, this is where that belongs.

CI runs on the pull request.
