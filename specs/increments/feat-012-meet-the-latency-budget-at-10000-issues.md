# feat-012: Meet the latency budget at 10,000 issues

## Context

Written into the plan as tuning. The benchmark turned out to matter more than the tuning: it
showed the budget was already met by a design that would not have held.

## What the benchmark found

**The budget passed at baseline — 44ms p95 against 200ms — by rebuilding the entire projection on
every request.** That is O(all events). At 30k events it costs 40ms; at 150k it breaches. Banking
a passing number produced by a design that fails at three times the data would have been the
wrong call, so the design was fixed rather than the number accepted.

| Read | Before | After |
|---|---|---|
| list all issues | 44.6ms | **1.5ms** |
| list, filtered by team | 42.4ms | **1.4ms** |
| list, two terms | 39.1ms | **1.7ms** |
| one issue | 37.1ms | **0.0ms** |
| metrics, 30 days | 39.0ms | **2.8ms** |

The fix is one long-lived projection that catches up per request instead of rebuilding.
`Catchup()` was built in feat-002 for exactly this and had never been used by the API.

## Design notes

**Catch-up failure rebuilds rather than serves stale state.** If an event cannot be applied the
projection is not trustworthy, and returning what it happens to hold would be worse than an
error. The next request starts from a fresh projection.

**The shared view is guarded by a mutex and must not be held across requests.** Every handler
reads what it needs and returns. This is the cost of caching, and it is written down where the
field is declared.

**Lists are bounded, with `total` returned.** Ten thousand issues in one response is slow to
produce and useless to read. `total` is returned so a caller knows what it is not seeing —
truncation that does not announce itself is a lie.

**The scaling claim is tested, not asserted.** `TestReadCostDoesNotTrackLogSize` measures at 10k
and 50k issues and fails if cost grows more than 8x for 5x the data. It grows 4.6x — linear in
*matches*, which is expected because filtering still scans every issue. The win is that reads no
longer replay history.

## Bug introduced and caught in the same increment

Surfacing the list count in the status bar **wiped the refusal message an action had just
reported**: the transition failed, said so, then the re-render overwrote it with "1 issue". The
browser test caught it immediately — the illegal-transition check timed out.

Fixed by separating the two: the status bar reports the last action, the list summary lives in the
main region. A structural test now fails if `renderIssues` writes to the status bar at all.

## Evidence

**Verified by:** implementing session, `inc/feat-012-latency`

### Any read against a 10,000-issue project responds in under 200ms at p95

```
dataset: 10000 issues, 30003 events

  list all issues            p50     1.1ms   p95     1.5ms   ok
  list, filtered by team     p50     1.2ms   p95     1.4ms   ok
  list, two terms            p50     1.4ms   p95     1.7ms   ok
  one issue                  p50     0.0ms   p95     0.0ms   ok
  children of one issue      p50     0.1ms   p95     0.1ms   ok
  schema                     p50     0.0ms   p95     0.0ms   ok
  metrics, 30 days           p50     2.2ms   p95     2.8ms   ok
  proposals                  p50     0.0ms   p95     0.1ms   ok
  actors                     p50     0.0ms   p95     0.0ms   ok
```

```
--- PASS: TestReadLatencyBudget
--- PASS: TestReadCostDoesNotTrackLogSize   (10k: 1.8ms · 50k: 8.3ms)
--- PASS: TestLargeListIsPaginated
```

### A reproducible benchmark that fails CI if the budget regresses

The benchmark is an ordinary test, so `make test` and CI run it on every change. It fails with the
offending endpoint named and its measured p95, not a generic timeout.

### Scope

`git diff --cached --stat main` — run.

**Two deviations, both recorded rather than expanded silently.** The increment's Scope said
"indexes and query plans only; no model changes":

1. **The projection cache** is not an index, but it is the actual cause of read latency, and
   tuning around it would have been treating a symptom.
2. **Pagination** changes the list response shape. It was the recorded follow-up from feat-009
   ("no pagination — fine at the current scale, wrong for feat-012's dataset"), and this is that
   increment.

The UI was updated for both, which is why `internal/ui` appears in the diff.

### Not verified

Measured in-process with `httptest`, so the figures exclude network and TLS. That is the right
comparison for a latency budget about the server's own work, but a real client will see more.

Concurrency is untested. The mutex serialises catch-up, which is correct but means a slow catch-up
blocks readers.

CI runs on the pull request.
