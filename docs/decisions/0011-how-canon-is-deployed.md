# 0011 — How Canon is deployed

**Status:** accepted
**Date:** 2026-09-04

## Context

Canon has only ever run on a laptop. Deploying it for one person raises a constraint that shapes the
architecture rather than following it: **it must cost almost nothing.** A tracker for one operator
that costs $25 a month in load balancers is a tracker nobody keeps.

What Canon is makes this tractable. It is a **read-only derivation over git**: it fetches mirrors,
parses ledgers, and answers from what it derived. `internal/catalogue` already says so —

> Reads never touch the network or the filesystem. Refreshing is a separate act with a recorded time.

Every response already carries `refreshed_at`. The system is *already* snapshot-shaped; it has
simply been keeping the snapshot in memory.

## Options

Costs are for one user, a handful of repositories, and a few hundred requests a month.

### A. An always-on container

Lightsail container, or ECS Fargate behind an ALB.

- **For:** the smallest possible change — Canon is already one binary serving HTTP.
- **Against:** you pay for readiness, not use. Lightsail's smallest container is ~$7/month; Fargate
  plus an ALB is ~$25 before any traffic, and the load balancer alone is $16 of it.
- **Cost:** $7–25/month.

### B. Lambda serving live, cache in ephemeral storage

Put the existing binary behind a Function URL and let it fetch mirrors on demand.

- **For:** no idle cost, and almost no code change.
- **Against:** **the cache is the problem.** Lambda's `/tmp` does not survive an execution
  environment, so a cold start re-clones every mirror while somebody waits. Three repositories is
  perhaps ten seconds; thirty is unusable. EFS fixes it and reintroduces a monthly bill.
- **Cost:** ~$0 plus latency nobody will tolerate.

### C. Split ingest from serving, and put the snapshot in S3

A scheduled job fetches mirrors and writes a **catalogue snapshot** to S3. The read path loads that
snapshot and answers from it — which is exactly what the in-memory catalogue does today, with S3
where the process memory used to be.

- **For:** no idle cost, no cold-start clone, and the expensive part (git) runs on a schedule where
  latency does not matter. It is also the architecture Canon already documents.
- **Against:** two deployables instead of one, and freshness is bounded by the schedule.
- **Cost:** pennies. S3 and EventBridge are rounding errors; Lambda's free tier covers a personal
  request volume many times over.

## Decision

**C, with the API preserved.**

There is a cheaper variant of C — write one JSON file per route and filter entirely in the browser,
deleting the server. It was rejected for one reason: **it would fork the UI's data access from the
API's, and Canon asserts that agents over MCP and humans over HTTP see the same surface.**
`internal/mcp/parity_test.go` enumerates the API's own route table and fails when the two diverge.
Saving a Lambda invocation is not worth trading a tested invariant for an untested duplicate.

### Shape

```
EventBridge (schedule) ─► ingest Lambda ─► fetches mirrors ─► writes snapshot ─► S3
                                                                                 │
browser ─► CloudFront ─► API Gateway (HTTP API) ─► read Lambda ◄─────────────────┘
                              │
                        Cognito JWT authoriser
```

**Authentication** is a Cognito user pool with one user and a JWT authoriser native to the HTTP API,
which means no custom authoriser code and nothing to get wrong. The page itself is served
unauthenticated — it is an empty shell containing no data — and every `/api/` route requires the
token. This is deliberate: a browser navigating to a page cannot set an `Authorization` header, so
gating the shell would need Lambda@Edge and a cookie scheme. Gating the data instead is simpler,
cheaper and gives away nothing.

**Credentials** live in SSM Parameter Store as `SecureString`. Parameter Store's standard tier is
free; Secrets Manager is $0.40 per secret per month, which is real money at this scale and buys
rotation nobody is going to use.

## Consequences

- Freshness becomes a schedule rather than a timer inside a process. R75 exists because of this: a
  failed refresh must serve the previous snapshot and say when it was taken, rather than serving
  nothing.
- The ingest path and the read path stop sharing a process, so `catalogue.Refresh` and the API
  can no longer assume the same memory. The snapshot format becomes an interface between two
  deployables and needs a version.
- The cache moves from a laptop's disk to the ingest Lambda's ephemeral storage, which is fine
  because a mirror is derived: ADR-0010 already requires that deleting it loses nothing.
- Canon acquires infrastructure, and therefore a way to be broken by a change nobody tested. The
  infrastructure is code in this repository for that reason.

## Alternatives not taken

**A hosted tracker.** The point of Canon is that it reads repositories nobody registered anywhere;
paying somebody else to do that reintroduces the registration.

**A cron job on an existing machine.** Cheapest of all, and it makes the catalogue reachable only
from that machine. The requirement is to see the estate from anywhere with an identity, which is
what the Cognito pool is for.
