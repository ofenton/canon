# docs-009: A program of work for running Canon

**Traces:** R73, R80

## What was asked

Deploy Canon to AWS for one person, cheaply. Add GitHub repositories through the interface. Report,
per repository, whether the agent loop is configured, and show the spec alongside increment status.

Three requirement groups (R73–R83) and twelve increments.

## The architecture, and why it is cheap

[ADR-0011](../../docs/decisions/0011-how-canon-is-deployed.md). The constraint shaped the design
rather than following it: a tracker for one operator that costs $25/month in load balancers is a
tracker nobody keeps.

What made it tractable is that **Canon is already snapshot-shaped.** `internal/catalogue` says so
today — *"reads never touch the network or the filesystem; refreshing is a separate act with a
recorded time"* — and every response already carries `refreshed_at`. It has simply been keeping the
snapshot in process memory. Moving it to S3 is not a redesign, it is the same architecture with a
different address.

| | Cost | Why not |
|---|---:|---|
| Always-on container | $7–25/mo | You pay for readiness. Fargate's ALB alone is $16 before traffic |
| Lambda serving live | ~$0 | **The cache.** `/tmp` dies with the execution environment, so a cold start re-clones every mirror while somebody waits |
| **Snapshot in S3** | **pennies** | Chosen. The expensive part — git — runs on a schedule where latency does not matter |

There is a cheaper variant still: write one JSON per route and filter in the browser, deleting the
server entirely. **Rejected**, because it would fork the UI's data access from the API's, and
`internal/mcp/parity_test.go` asserts that agents and humans see the same surface. Saving a Lambda
invocation is not worth trading a tested invariant for an untested duplicate.

Authentication is a Cognito pool with a JWT authoriser native to the HTTP API — no custom authoriser
code. The page shell is served unauthenticated because it contains no data; every `/api/` route is
gated. Gating the shell instead would need Lambda@Edge and a cookie scheme to solve the fact that a
browser navigation cannot set an `Authorization` header. Credentials go in Parameter Store, whose
standard tier is free; Secrets Manager is $0.40 per secret per month for rotation nobody will use.

## The thing this breaks, stated plainly

Adding a repository from the interface is a **write**, and Canon has an asserted invariant that it
has none — `TestNoWriteRoutes` enumerates the route table and fails on any method that is not `GET`.

[ADR-0012](../../docs/decisions/0012-the-one-thing-canon-writes.md) narrows it rather than deleting
it: **Canon writes exactly one thing, where to look, and authors nothing about the work.** The test
becomes "no route writes work state", and a route accepting a status change still fails it.

The honesty this requires: the old invariant could be checked by looking at HTTP methods. The new
one cannot — it needs a list of which routes may write, which somebody maintains. **A guard with a
maintained allow-list is weaker than a guard without one.** Accepted because the alternative is
shelling into a deployed instance to add a repository, which is the friction that stops an estate
being tracked.

## Sequencing, and what it means

```
feat-043  serve from a snapshot        ← everything depends on this, and it needs no AWS
feat-044  package for Lambda
feat-045  infrastructure as code       ← the first thing that costs money if it is wrong
feat-046  sign in
feat-047  refresh on a schedule
feat-048  a source list with history
feat-049  add and remove a repository  ← the first write Canon has ever had
feat-050  credentials from Parameter Store
feat-051  report whether the loop is set up
feat-052  show the product spec
feat-053  template version across the estate
```

**`feat-043` is first and is the only genuinely architectural one.** Splitting deriving from serving
is the whole deployment, and it can be built and tested on a laptop with no AWS account involved. If
it is right, everything after it is packaging.

**`feat-051`, `feat-052` and `feat-053` need no AWS either** and could be pulled forward at any
point. They are sequenced last because that was the order asked for, not because anything blocks
them — worth knowing if the deployment stalls on something.

## What is deliberately not here

**Driving the agent loop from Canon.** Named as the later goal and left out entirely. Everything
above keeps Canon a reader with one narrow exception; starting work from Canon would make it an
originator, which reverses ADR-0009 and deserves its own decision rather than arriving as a feature.

**Anything for a second user.** The pool has one user, the token is one account's, and ADR-0012
records what would need revisiting if that changes — at more than one person, "who added this
repository and why" is a question a commit message answers and an object version does not.

## Not verified

No AWS resource has been created and no cost has been measured. Every figure here is list price
arithmetic. `feat-045` requires destroying and reapplying the stack once and **recording what it
actually cost**, because a deployment plan whose central claim is "this is cheap" should not rest on
my arithmetic.

## Evidence

- `docs/decisions/0011-how-canon-is-deployed.md` — accepted, three options costed
- `docs/decisions/0012-the-one-thing-canon-writes.md` — accepted, amends 0009
- `specs/product.md` — R73–R83 across running it, choosing what is tracked, and reporting on adoption
- `specs/increment-plan.md` — 12 increments, every new requirement claimed
