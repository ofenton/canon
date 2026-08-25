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

Lines are non-test Go only, `find internal/<pkg> -name '*.go' ! -name '*_test.go'`. Every figure here
had drifted by `feat-040` — `ingest` read 754 against 684, `api` 476 against 319 — because nothing
checks them. They are measured again below; they will drift again until something does.

| Package | Lines | Responsibility | Imports |
|---|---:|---|---|
| `internal/ingest` | 684 | Reads a repository: parses the ledger and spec, derives status history from the ledger's commit history | — |
| `internal/ui` | 33 | Serves one embedded HTML file | — |
| `internal/metrics` | 322 | Flow measured from derived transitions | `ingest` |
| `internal/conform` | 247 | Runs the template's rules and reports what fails | `ingest` |
| `internal/source` | 360 | Says where Canon looks: parses the list of sources and resolves each to repositories | `ingest` |
| `internal/catalogue` | 156 | Holds what was read from each repository, and when | `ingest`, `conform`, `source` |
| `internal/api` | 325 | The read surface, and pagination | `catalogue`, `conform`, `ingest`, `metrics`, `ui` |
| `internal/mcp` | 325 | MCP over stdio, **derived from the API's route table** | — (takes routes as data) |
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
canon serve -source ~/code -addr :8080 -refresh 5m
```

One process, one port, no database, no configuration file. Canon does write a **cache** of mirrored
repositories, which is not a contradiction of that and is worth stating precisely: a cache is
discardable and reproducible, a store is authoritative. Nothing is read from the cache that could not
be read from origin, and `TestDeletingTheCacheLosesNothing` deletes it and compares the ingest.

Reads never touch git: the catalogue is
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
| Every MCP tool matches an API route | `TestToolsCoverEveryRealRoute` — against `api.Routes()` itself, not a copy |
| Every route has an MCP description | `TestEveryRealRouteHasADescription` |
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
| A blocked increment names what it waits on | `TestBlockedReportsUnfinishedDependencies` |
| Finished work is never blocked | `TestFinishedWorkIsNotBlocked` |
| A dependency outside the ledger is not a block | `TestADanglingDependencyIsNotABlock` |
| A dependency cycle is found and reported once | `TestCyclesAreFound`, `TestACycleIsReportedOnce` |
| Every action works by keyboard and by pointer | `e2e/keyboard.mjs` — two runs, one sending no clicks and one sending no keys |
| Deleting the cache loses nothing | `internal/source.TestDeletingTheCacheLosesNothing` — deletes it, rebuilds, compares ingest fingerprints |
| An unreachable remote is served stale with its reason | `internal/catalogue.TestAStaleSourceIsServedWithItsReason` — `Stale`, not `Err`: there is something to show |
| One unreachable source never empties the catalogue | `internal/catalogue.TestAFailedSourceAppearsRatherThanVanishing` — a failed source survives as an entry |
| The list of sources has no schema | `internal/source.TestTheListHasNoSchema` — a nested key must parse as two opaque lines, and the package may not import a decoder |
| The repository holds no state and no configuration | `cmd/canon.TestTheRepositoryHoldsNoStateOrConfiguration` — reads `git ls-files`, so a claim in the README is not the only guard |
| Every view has a URL that reproduces it | `e2e/urls.mjs` — a copied URL is opened in a fresh page and compared |
| Every piece of view state reaches the URL | `internal/ui.TestEveryPieceOfViewStateIsInTheURL` — parses the state object, so new state fails by default |

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
- **Blocking is direct only.** A waits on B is reported; A waits on B waits on C is not.
  Transitive chains are technically true and rarely act on, and the direct answer is the one
  somebody can do something about.
- **The rules exist twice**, in `internal/conform` and in the template's `validate-plan.py`, with
  nothing keeping them in step. [ADR-0006](decisions/0006-distributing-the-template.md) is the
  answer and is not built.
- **Apache-2.0**, with no third-party Go modules at all after `cut-001`. Playwright is a dev
  dependency used by the browser suite.
