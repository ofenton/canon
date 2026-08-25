# feat-036: Flow measured from real transitions

## Context

The measurement code was never the problem; its input was. `scripts/import-ledger.py` spread each
increment's route across the commits carrying its trailer, and squashed merges collapsed those to
near-simultaneous timestamps — a p50 of nine minutes against a real four hours, roughly thirty times
out. feat-035 made exact transitions available. This feeds them in and retires the heuristic.

## Design notes

**A conversion, not a rewrite.** `Compute` is 400 tested lines producing percentiles, ageing and
throughput. `FromIngest` converts ingested increments into the shape it reads, and nothing about
the measurement changed.

**The template's statuses are hardcoded.** Under ADR-0009 the template *is* the schema, so there is
no configuration to read. A status not in `TemplateStates` is not one this system understands.

**`(removed)` is open, not closed.** An increment removed from the ledger did not finish; it stopped
existing. Counting it as completed would flatter every repository that ever reverted a plan.

## What the real numbers say about this repository

```
$ canon flow .

Canon — last 30 days

  completed      45
  started        46
  in progress    2

  cycle time     p50 14m · p85 1.2h · p95 9.1h  (n=44)
  lead time      p50 3.1h · p85 15.1h · p95 17.2h  (n=45)
```

**The cycle time is honest and close to meaningless, and that is a finding about how this project
was run rather than about the metric.**

Cycle time is first-active to first-closed — `in-progress` to `done`. Measured across this ledger:

```
increments with both in-progress and in-review: 26
of those, in-review within 2 minutes of in-progress: 6
median gap: 3.4 minutes
```

Three and a half minutes is not how long the work took. It is the gap between two commits made at
the *end* of the work, because `in-progress` was recorded alongside the finished code rather than
before it was started.

`implement-increment` is explicit about this — step 2 of its workflow is *"Open the branch, set
status in-progress"*, before step 3 writes the tests and step 4 implements. **The template said to
do it and the agent running it, me, did not.** Lead time at p50 3.1h is the truthful figure for this
repository; cycle time will only mean something once the loop is followed in the order it states.

This is exactly the class of thing `feat-037` should report: an aggregator that says *"your cycle
times are meaningless because in-progress lands three minutes before in-review"* is doing work no
repository-local check does.

## The latent defect this found

`TemplateSchema()` first returned a struct literal. That compiles, and `category()` — which scans
the `States` slice — worked, so the flow numbers were right. But `HasState`, `HasField` and `Role`
read unexported maps built only by `index()`, so on that schema they returned false for **every**
status while `category()` returned the correct one.

Two ways to answer the same question, disagreeing silently. `schema.NewFixed` is now the only
supported way to construct a schema outside `Load`, and it indexes.

## Evidence

**Verified by:** implementing session, `inc/feat-036-real-flow`

### AC: cycle and lead time from derived transitions

`TestFlowFromIngestedTransitions` builds an increment approved at 09:00, started at 10:00 and done
at 14:00, and asserts cycle 4h and lead 5h. Both within a tenth of an hour.

### AC: no estimate of any kind

The existing `TestEstimateFieldsAreRefused` and `TestNoEstimationAnywhereInTheSource` pass
unchanged — the second parses the source rather than trusting the claim.

### AC: two status changes in one commit report the same instant

Asserted in `internal/ingest` by `TestTwoChangesInOneCommitShareAnInstant`; nothing here invents an
interval, and the 3.4-minute median above is a real measurement of two real commits.

### Tests

Five new in `internal/metrics`: flow from ingested transitions, `(removed)` not counted as
completed, unfinished work ageing, every template status categorised, and creation being the first
transition. Full suite green across twelve packages.

### Scope

`git diff --cached --stat main` — run. `internal/metrics/ingested.go` and its tests,
`schema.NewFixed`, `canon flow`, and the deletion of `scripts/import-ledger.py`.

### Not verified

**The HTTP `/api/metrics` route still reads the projection**, not ingested repositories. Wiring the
server to ingest is `feat-038`; until then the API and the CLI answer from different sources and can
disagree.

**One repository at a time.** `canon flow <path>` takes a single directory. Aggregating across an
organisation is `feat-038`.

**Nothing warns that cycle time is unreliable.** This increment found the condition and reports it
in prose here; detecting it automatically is `feat-037`'s job and is not built.
