# Architecture

Canon as built, after `cut-001`. Descriptive, not aspirational: if something here is not true, this
document is wrong and should be fixed.

Rules live in [`docs/constitution.md`](constitution.md). Decisions and their rationale live in
[`docs/decisions/`](decisions/). Reasoning about a particular change lives in that increment's file
under `specs/increments/`. This file is the map.

## Context

Canon reads repositories that follow the agentic SDLC template and shows what every team is
building. Point it at a directory; it finds every repository containing `specs/increment-plan.md`,
derives everything from the ledger and its commit history, and serves it.

```
  repositories ──► ingest ──► catalogue (in memory) ──┬──► HTTP API ──► web UI
   (git, on disk)                │                    └──► MCP (stdio) ──► agents
                                 └──► conformance
```

## The one idea

**Canon derives; it does not author.**

Every fact it shows is reconstructible from a repository at any moment. That is not a limitation
worked around — it is the property that makes one source of truth possible. If Canon and a
repository disagree, the repository is right and Canon is stale, and there is never a question
about which.

The measurement that produced this design: **96% of the data Canon held while it was a tracker was
reconstructible from the repositories it tracked.** A thing you can rebuild from git is a cache of
git. See [ADR-0009](decisions/0009-canon-as-aggregator.md).

## Components

Measured, smallest dependency first. A package may only import packages above it in this table;
that ordering is the architecture.

| Package | Lines | Responsibility | Imports |
|---|---:|---|---|
| `internal/ingest` | 754 | Reads a repository: parses the ledger and spec, derives status history from the ledger's commit history | — |
| `internal/ui` | 111 | Serves one embedded HTML file | — |
| `internal/metrics` | 348 | Flow measured from derived transitions | `ingest` |
| `internal/conform` | 322 | Runs the template's rules and reports what fails | `ingest` |
| `internal/catalogue` | 290 | Discovers repositories and holds what was read | `ingest`, `conform` |
| `internal/api` | 476 | The read surface, and pagination | `catalogue`, `conform`, `ingest`, `metrics`, `ui` |
| `internal/mcp` | 298 | MCP over stdio, **derived from the API's route table** | — (takes routes as data) |
| `cmd/canon` | 446 | `catalogue`, `ingest`, `flow`, `conform`, `serve`, `mcp` | all |

**3,045 non-test lines, down from 14,783.** The event log, the projection, schema enforcement,
authorisation, authentication, the actor registry, proposals, boards, the query language and every
write path were removed by `cut-001`, because they existed to defend writes Canon no longer accepts.

## Data

**There is no store.** The catalogue is in memory, rebuilt by reading repositories. Nothing is
persisted, so there is nothing to back up, migrate or corrupt — restarting re-derives everything.

**Transitions come from the ledger's own file history.** The template requires that every status
change is a commit, so `git log -p specs/increment-plan.md` *is* the transition log. Canon reads the
whole file at each commit and compares parsed states rather than parsing diffs: a diff shows a
changed `Status:` line without reliably showing which increment it belongs to.

The mechanism this replaced approximated, and was measured at roughly **thirty times out** — a p50
of nine minutes against a real four hours.

**Personal data** is whatever a repository puts in its own ledger, plus the commit ids Canon cites.
Canon stores none of it.

## Runtime

```bash
canon serve -products ~/code -addr :8080 -refresh 5m
```

One process, one port, no database, no configuration file. Reads never touch git: the catalogue is
filled at startup and on a timer, and every response carries `refreshed_at` so a stale view reads as
stale rather than presenting itself as current.

Reads are open to anyone who can reach the port. Identity existed to protect writes that no longer
happen.

## Cloud dependencies

**None.** Canon runs anywhere it can read a directory of git repositories.

| Service | Used for | Replaceable with | Cost to move |
|---|---|---|---|
| — | — | — | — |

Empty on purpose: adding a row is a decision worth an ADR.

## Invariants

Properties the system claims, each asserted by a test. A property nobody checks is a property that
stops being true. Structural assertions — parsing source, enumerating routes — are noted, because
those are the ones that survive a careless change.

| Invariant | Asserted by |
|---|---|
| No route writes anything | `TestNoWriteRoutes` — enumerates the route table (structural) |
| The UI offers no writes | `TestTheUIOffersNoWrites` (structural) |
| No MCP tool takes a write body | `TestNoToolTakesAWriteBody` |
| Every route is exercised | `TestEveryRouteIsExercised` (structural) |
| Every MCP tool matches an API route | `TestToolParityWithTheAPI` — tools are derived, not written twice |
| Every route has an MCP description | `TestEveryRouteHasADescription` |
| Every JSON key is snake_case | `TestEveryJSONKeyIsSnakeCase` — walks real responses |
| Reads require no identity | `TestReadsNeedNoIdentity` |
| Transitions come from commit history | `TestTransitionsComeFromCommitHistory` |
| An unrelated commit invents no transition | `TestUnrelatedCommitsProduceNoTransitions` |
| Ingesting twice gives the same result | `TestIngestIsDeterministic` |
| A malformed entry does not lose the others | `TestAMalformedEntryDoesNotLoseTheOthers` |
| An unreadable source is reported, not dropped | `TestAnUnreadableSourceIsReportedNotDropped` |
| Reads never touch the source | `TestReadsDoNotTouchTheSource` — deletes the repository, then reads |
| Conformance reports, never refuses | `TestConformanceReportsRatherThanRefuses` |
| No estimation field can be introduced | `TestEstimateFieldsAreRefused`, `TestNoEstimationAnywhereInTheSource` (parses source) |
| Every template status has a category | `TestEveryTemplateStatusIsCategorised` |
| A superseded render never paints | `TestEveryRendererChecksItsTicket` (structural) |
| Every action is declared once | `TestActionsAreDeclaredOnce` (structural) |
| The UI makes no external request | `TestNoExternalRequests` (structural) |
| Every screen says when it was read | `TestScreensSayWhenTheyWereRead` |
| Every action works by keyboard and by pointer | `e2e/keyboard.mjs` — two runs, one sending no clicks and one sending no keys |

**This table is the part of this document most worth keeping current.** The worst defects in this
project were cross-cutting invariants that no single increment owned and no document stated.

## Seams

**`ingest.Repo`** is the only thing that knows what a repository looks like. Supporting a different
convention is that function.

**The route table** in `api.Routes()` is data. MCP derives its tools from it, so adding a route
without a description fails a test rather than silently producing an agent surface that lags the
HTTP one.

**`catalogue.Discover`** is how repositories are found. Remote discovery — a GitHub organisation
over its API — replaces this function and nothing else.

## Known constraints

- **Local paths only.** `Discover` reads a directory. Pointing Canon at a GitHub organisation is not
  built, which is a real shortfall against R52.
- **One level deep.** A parent directory of checkouts is the shape assumed; nested layouts are not
  found.
- **Everything in memory.** Instance size is bounded by RAM, and nothing has measured where.
- **Refresh is a timer.** There is no webhook and no incremental update; each refresh re-reads every
  repository from scratch.
- **The rules exist twice**, in `internal/conform` and in the template's `validate-plan.py`, with
  nothing keeping them in step. [ADR-0006](decisions/0006-distributing-the-template.md) is the
  answer and is not built.
- **Apache-2.0**, with no third-party Go modules at all after `cut-001`. Playwright is a dev
  dependency used by the browser suite.
