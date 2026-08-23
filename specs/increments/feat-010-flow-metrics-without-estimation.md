# feat-010: Flow metrics without estimation

## Context

Constitution rule 10: no estimation. This is the positive half — what Canon offers *instead* of
story points — and the enforcement half, which makes the rule structural rather than cultural.

## Design notes

**Everything is measured, nothing is estimated.** Cycle time and throughput come from timestamps
that were recorded anyway. The honest cost is that these numbers only exist after work has flowed;
a forecast you cannot make yet beats a number you made up.

**There is no mean.** Cycle times are long-tailed, and an average hides the tail people actually
complain about. p50/p85/p95 and the three slowest issues by name are more useful than one number,
and harder to game. The seeded demo shows why: p50 is 2 days and p85 is 7.

**Percentiles use nearest-rank, not interpolation**, so every reported figure is a real
measurement of a real issue rather than a number between two of them.

**Ageing is reported for unfinished work.** Cycle time is a lagging indicator — it only moves once
something finishes. The oldest thing still in progress moves *before* the damage lands, and it is
the number a lead should actually watch. In the demo it surfaces `WIP-2` at 18 days.

**Reopened work measures from the first time it became active.** Measuring from the second attempt
would flatter the numbers and misrepresent what a requester waited.

**Lead time is reported alongside cycle time.** Cycle time is what the team controls; lead time is
what the requester experiences. Reporting only the first is how a team convinces itself things are
fine while the queue grows.

**Estimate fields are refused by name at startup.** A rule enforced only by convention lasts until
the first person who wants a story point field. `EstimateFieldNames` covers the obvious spellings,
and the way to reintroduce estimation is now a pull request that edits that list — which is the
conversation worth having.

## Evidence

**Verified by:** implementing session, `inc/feat-010-metrics`

### Cycle time and throughput derived from recorded state transitions

```
$ curl "/api/metrics?days=30"
completed 9   started 11   in progress 2
cycle time (active→closed)   p50   2d   p85   7d   p95  11d   max  11d   (n=9)
lead time (created→closed)   p50   2d   p85   7d   p95  11d   max  11d   (n=9)
slowest: CANON-9 11d, CANON-6 7d, CANON-8 3d
ageing (unfinished, oldest first):
  WIP-2    in_progress  18d
  WIP-1    in_progress  10d
```

```
--- PASS: TestCycleTimeAndThroughputFromTransitions
--- PASS: TestAgeingSurfacesUnfinishedWork
--- PASS: TestReopenedWorkMeasuresFromFirstActive
--- PASS: TestWindowExcludesWorkOutsideIt
--- PASS: TestEmptyIsNotAnError
```

Metrics accept the feat-009 query language, so flow can be measured per team or per component
without a separate reporting concept.

### canon.yaml defining an estimate field refuses to start

```
$ canon schema -schema /tmp/est.yaml
canon: field "storyPoints" is an estimate; Canon measures flow from recorded
transitions and has no estimation. Remove it, or use cycle time and throughput instead
```

```
--- PASS: TestEstimateFieldsAreRefused   (7 spellings, plus a false-positive check)
```

Checked in `canon serve` and `canon schema`, so neither running nor validating can accept one.

### No story point, velocity, estimate or burndown anywhere

```
--- PASS: TestNoEstimationAnywhereInTheSource
```

Parses seven packages and fails on any function, type or field named `StoryPoints`, `Velocity`,
`Burndown`, `Estimate` or `Estimated` — so the guarantee survives a future convenience helper.

### Scope

`git diff --cached --stat main` — run. Metrics in `internal/metrics`, one route in `api`, the
startup check in `cmd/canon`, an MCP description. No README changes: the documentation catch-up is
`docs-002`, deliberately one increment rather than a patch per feature.

### Not verified

Throughput buckets by day up to 90 days and by week beyond, with no configuration. Fine for now;
someone will eventually want a fortnight.

CI runs on the pull request.
