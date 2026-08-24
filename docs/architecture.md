# Architecture

Canon as built, at 41 increments. Descriptive, not aspirational: if something here is not true, this
document is wrong and should be fixed.

Rules live in [`docs/constitution.md`](constitution.md). Decisions and their rationale live in
[`docs/decisions/`](decisions/). Reasoning about a particular change lives in that increment's file
under `specs/increments/`. This file is the map.

## Context

Canon is an issue tracker for an organisation that wants one schema rather than one per team. It is
a single Go binary with an embedded web UI, an HTTP API, an MCP server for agents, and a SQLite
file. It talks to nothing else. Optionally it posts webhooks outward on state changes; nothing is
required to be listening.

```
  humans  ──► web UI ─┐
  scripts ──► CLI ────┼──► HTTP API ──► enforce ──► event log (SQLite)
  agents  ──► MCP ────┘                    │              │
                                           └──► webhooks  └──► projection (in memory)
```

## The one idea

**The event log is the system. Everything else is a cache or a view.**

Every change is an append-only event. Current state is a projection rebuilt by replaying the log,
and can be discarded at any time (`canon rebuild`). This is why history, provenance, backup and the
future federation story are one mechanism rather than four.

The log is immutable at the database level, not by convention:

```sql
CREATE TRIGGER events_no_update BEFORE UPDATE ON events
  BEGIN SELECT RAISE(ABORT, 'events are immutable: update is not permitted'); END;
```

Anything holding a connection to the file is bound by that, including a future bug.

## Components

Measured, smallest dependency first. A package may only import packages above it in this table;
that ordering is the architecture.

| Package | Lines | Responsibility | Imports |
|---|---:|---|---|
| `internal/event` | 838 | Append-only log: CBOR encoding, ULID ids, immutability triggers, backup | — |
| `internal/schema` | 1363 | Loads and validates `canon.yaml`: states, transitions, fields, types, hierarchy, roles, teams, webhooks | — |
| `internal/ui` | 211 | Serves one embedded HTML file | — |
| `internal/projection` | 1170 | Replays events into current state; hierarchy and dependency graph queries | `event` |
| `internal/webhook` | 250 | Asynchronous, bounded outbound delivery | `schema` |
| `internal/metrics` | 401 | Flow measured from recorded transitions | `projection`, `schema` |
| `internal/enforce` | 3578 | **The domain.** Every write goes through here: schema enforcement, authorisation, authentication, proposals | `event`, `projection`, `schema`, `webhook` |
| `internal/query` | 752 | The query language, and search | `enforce`, `event`, `projection`, `schema` |
| `internal/api` | 2130 | HTTP surface, authentication middleware, pagination | most of the above, `ui` |
| `internal/mcp` | 511 | MCP over stdio, **derived from the API's route table** | `api`, `enforce`, `event`, `schema` |
| `cmd/canon` | 1686 | `serve`, `mcp`, `bootstrap`, `token`, `new`, `link`, `trace`, `usage`, `schema`, `events`, `rebuild`, `backup` | all |

`enforce` is the largest package and deliberately so: it is where every rule lives, and the
alternative is rules spread across handlers where nobody can find them.

## Data

**One store**: a SQLite file, WAL mode, default `canon.db`. There is no second database, no cache
server, and no queue. Backup is `VACUUM INTO` while running — a plain file copy of a WAL database
recovers nothing, learned the hard way.

**Events** carry `{Version, ID, Type, Subject, At, Actor, Payload, Seq}`. `Seq` is assigned on
append and is arrival order; `At` is when the thing happened and may be earlier, which is what makes
backdating and commit linking possible. Encoded as canonical CBOR (RFC 8949 §4.2.1) so the bytes are
stable — a prerequisite for signing them later, which nothing does yet.

Twenty event types, and the list is meant to stay short:

```
issue.created  issue.transitioned  issue.reparented  issue.deleted  issue.team_set
issue.dependency_added  issue.dependency_removed  issue.commit_linked
actor.registered  actor.role_granted  actor.role_revoked
actor.token_issued  actor.tokens_revoked
team.member_added  team.member_removed
board.created  board.deleted
proposal.created  proposal.approved  proposal.rejected
```

**The projection is in memory**, rebuilt at startup and kept current incrementally. Read cost does
not track log size — asserted by `TestReadCostDoesNotTrackLogSize`, which measures allocations
rather than wall-clock after the first version of that test measured its own fixture.

**Configuration is a file, not data.** `canon.yaml` holds states, transitions, fields, issue types,
the type hierarchy, roles, teams and webhooks. There is no runtime interface for changing any of it,
and `TestNoRuntimeSchemaMutation` asserts that by parsing the source rather than by trusting the
claim. Membership — who holds a role, who is in a team — *is* data and lives in the log.

**Personal data** is limited to actor ids, whatever an organisation puts in issue text, and token
hashes. There are no email addresses, names or profiles: an actor id is an opaque string the
operator chooses. The log is not encrypted at rest.

## Runtime

One process, one port, no orchestration required:

```bash
canon serve -db canon.db -schema canon.yaml -addr :8080
```

Startup validates `canon.yaml` against the existing log and **refuses to start** if a schema change
would strand issues — a removed state that issues sit in, a narrowed hierarchy, a deleted team that
still owns work. That is deliberate and has a cost: see the repair gap below.

Startup also reports whether identity is proved, and who can still be impersonated:

```
  auth   PARTIAL — still claimable without a token: mallory
```

Observability is stdout: a structured `slog` logger for webhook delivery, and the startup banner.
There are no metrics endpoints, no tracing, and no health check beyond the API answering.

## Cloud dependencies

**None, and that is the product.** Canon runs on a laptop, a VM, or a container with a volume. The
only external calls it ever makes are outbound webhooks the operator configured.

| Service | Used for | Replaceable with | Cost to move |
|---|---|---|---|
| — | — | — | — |

This table is empty on purpose. Adding a row is a decision worth an ADR, because "one static binary,
one file of data, no external services" is what makes self-hosting cheaper than the thing Canon
argues against. The nearest call was authentication: delegating to Cognito was considered and
rejected in `feat-031`, because a tracker that cannot start without an AWS account is not
self-hosted and an open-source tool requiring one vendor's identity service is not vendor-neutral.

## Invariants

These are properties the system claims. Each is asserted by a test, because a property nobody checks
is a property that stops being true. Where the assertion is structural — parsing source, enumerating
routes — it is noted, since those are the ones that survive a careless change.

| Invariant | Asserted by |
|---|---|
| Every API route requires authentication | `TestOnceATokenExistsClaimsAreRefused`, plus middleware wrapping `/api/` so a new route is covered by construction |
| Every registry write requires `administer` | `TestEveryRegistryWriteIsGated`, `TestAnActorCannotEscalateItsOwnRole` |
| No route discloses a token or its hash | `TestNoRouteDisclosesATokenOrItsHash`, `TestTheLogNeverHoldsTheToken` |
| Every MCP tool matches an API route | `TestToolParityWithTheAPI` — tools are *derived* from the route table, not written twice |
| No endpoint is reachable only by the UI | `TestNoUIOnlyRoutes` (structural) |
| Every route is exercised by the contract test | `TestEveryRouteIsExercised` (structural) |
| Every JSON key is snake_case | `TestEveryJSONKeyIsSnakeCase` — walks real responses, not struct tags |
| The schema cannot change at runtime | `TestNoRuntimeSchemaMutation` (parses source) |
| No estimation field can be introduced | `TestEstimateFieldsAreRefused`, and `TestNoEstimationAnywhereInTheSource` which parses the source |
| A refused write appends nothing | `TestDeniedWritesAppendNothing`, `TestRejectedWritesAppendNothing` |
| Events are immutable | SQLite triggers, plus `internal/event/immutability_test.go` |
| A rebuild is deterministic | `TestRegistryRebuildsDeterministically` |
| Every UI action is reachable by keyboard | `e2e/keyboard.mjs` — 33 checks, no mouse events, in CI |
| A superseded render never paints | `TestEveryRendererChecksItsTicket` (structural) |
| A write never waits on a webhook | `TestASlowSubscriberDoesNotSlowTheWrite` |
| Reads stay under 200ms at 10,000 issues | `TestReadLatencyBudget`, fails CI on regression |

**This table is the part of this document most worth keeping current.** The three worst defects
found in this project were cross-cutting invariants that no single increment owned and no document
stated: no read route authenticated, any actor could grant itself any role, and Go field names
leaked into JSON. Each survived several increments precisely because it belonged to everything and
therefore to nothing.

## Seams

Places designed to be replaced, and what replacing them would cost.

**`enforce.Verify`** turns a token into an actor id. Replacing it with one that trusts a signed OIDC
claim from Cognito, Entra, Okta or Keycloak changes that function and nothing above it —
authorisation takes a `Principal` and does not care how one was constructed.

**`event.Store`** is the only thing that knows about SQLite. A different backend is that interface.
The CBOR encoding is deliberately not SQLite-specific.

**The route table** in `api.Routes()` is data. MCP derives its tools from it, which is why adding a
route without an MCP description fails a test rather than silently producing an agent surface that
lags the HTTP one — a real failure, caught this way when `feat-008` and `feat-009` merged cleanly
and still broke `main`.

**`canon.yaml`** is the seam for the organisation. Everything an org can vary lives there, and the
absence of a per-project override is the product.

## Known constraints

- **The CLI writes the log directly.** `canon new`, `canon link`, `canon trace` and `canon token`
  open the SQLite file rather than talking to a server, so they cannot be used against a remote
  instance and must not run while a server holds the file for writing. This is the largest gap.
- **The projection is memory-resident**, so instance size is bounded by RAM. Nothing has measured
  where that bound is beyond the 10,000-issue budget.
- **Webhook deliveries are in memory and unsigned**, so a restart loses undelivered ones and a
  subscriber cannot verify a delivery came from Canon.
- **Issue ids are `CANON-<n>` over the count of live issues**, so deleting an issue can make the
  next id collide with one already used.
- **No repair path.** If a schema change would strand issues the server refuses to start, which is
  correct, and means the data cannot then be fixed through it.
- **One writer.** SQLite in WAL mode allows concurrent readers, but Canon assumes a single serving
  process. Running two against one file is untested.
- **Apache-2.0.** The code imports exactly three third-party modules — `modernc.org/sqlite` (pure
  Go, no cgo, BSD-3), `github.com/fxamacker/cbor/v2` (MIT) and `gopkg.in/yaml.v3` (MIT/Apache-2.0) —
  which pull ten transitive modules, all permissive. `go.mod` marks everything `// indirect` and
  wants a `go mod tidy`. Playwright is a dev dependency only, used by the browser suite.
