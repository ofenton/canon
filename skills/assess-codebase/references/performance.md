# Performance lens

## Contents
- Measure before you look
- Where the time usually is
- Where the money usually is
- Tooling
- Rating severity

## Measure before you look

Do not report a performance finding without a number. Reading code and guessing which part is
slow is the single most reliable way to produce a backlog of pointless optimisations.

Establish, in order:
1. **What "slow" means here** — which operation, at what percentile, versus what target.
2. **A reproducible measurement** — a benchmark, a load test, a production trace.
3. **A profile** — where the time actually goes.

If no target exists, that is your first finding: nobody can tell whether the system is slow.

## Where the time usually is

**Database** — N+1 queries; missing indexes; full scans; queries returning far more rows than
used; transactions held open across network calls; connection pool exhaustion; lock contention.
Check query plans, not just query text.

**Network** — Sequential calls that could be concurrent; chatty service-to-service protocols;
missing timeouts (a slow dependency becomes your outage); absent connection reuse; payloads
carrying fields nobody reads.

**Compute** — Accidentally quadratic loops over collections that grew; work repeated per
request that could be done once; serialization of large objects; regex backtracking.

**Caching** — Absent where it would be trivial; present but with a hit rate nobody measures;
stampedes on expiry; caching the wrong layer.

**Frontend** — Bundle size and what is in it; render-blocking resources; layout thrash;
re-renders from unstable references; images served unsized and uncompressed.

**Concurrency** — Blocking calls on event loops; thread pool starvation; unbounded queues that
turn a spike into a memory failure; back-pressure that does not exist.

## Where the money usually is

Cost is a performance dimension and is often the easier win. Look at: over-provisioned
instances and idle capacity; data egress; log and metric volume; retention on hot storage;
per-request costs of paid APIs (including LLM tokens); anything running on a schedule that
nobody has questioned since it was written.

## Tooling

Use the platform's profiler rather than reasoning from source: `py-spy` / `cProfile`, Node
`--cpu-prof` and `clinic`, JFR, `pprof`, Chrome DevTools and Lighthouse. For databases, use
`EXPLAIN ANALYZE` and the slow query log. For distributed paths, use whatever tracing exists —
and if none exists, say so.

## Rating severity

Rate by user impact and cost, not by how inefficient the code looks:

| Severity | Test |
|---|---|
| Critical | Breaches an SLO now, or scales in a way that will fail imminently |
| High | Materially degrades a common user journey, or a large ongoing cost |
| Medium | Noticeable, bounded, or affects an uncommon path |
| Low | Measurable inefficiency with no current impact |

For each finding state the measured cost, the expected gain, and roughly what it would take.
A 40% improvement to something taking 3ms is not a finding.
