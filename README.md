# Canon

An issue tracker where the organisation's schema is versioned configuration, not per-project
accretion — and where coding agents are first-class users rather than an API afterthought.

Apache-2.0. Self-hosted. One static binary, one file of data, no external services.

> **Status: in development.** The domain, authorisation and HTTP API work. There is no web UI,
> no MCP server and no authentication yet. See [What is not built](#what-is-not-built).

## The problem

Jira was the most criticised developer tool of 2025 — more complaints than the next four
combined. The usual explanations (it's slow, there are too many fields) are symptoms. The disease
is that **configuration is per-project and unbounded**, so every team's setup diverges and the
divergence destroys the one thing an org-wide tracker exists to provide: a question you can ask
across teams and get a true answer to.

Measured across hundreds of enterprise Jira Cloud instances:

| | Observed | Actually needed |
|---|---|---|
| Workflows per instance | 90–100 | a handful |
| Permission schemes | 40–100+ | 10–15 |
| Custom fields | 700–800+, **over half unused in 12 months** | tens |
| Spellings of "completed" | 16 | 1 |
| Projects inactive 6+ months but fully configured | ~half | none |

The mechanism is always *incremental decisions without visibility*. Each request is individually
reasonable — one more status, our team works differently — and nobody ever sees the aggregate, so
nobody ever says no. Then reporting collapses, because you cannot filter "completed work" when
completed has sixteen spellings.

Linear's answer is to remove configurability. That works to roughly fifty engineers and one
process, and has nothing to say to an organisation that genuinely contains a regulated team, a
support team and a product team.

## The idea

**Configuration is a versioned, org-owned artifact.**

The whole organisation's schema — states, transitions, fields, issue types and roles — lives in
one `canon.yaml`, in git, changed by pull request. A team cannot unilaterally add a status,
because there is nowhere local to add one. They open a PR against the shared schema, someone
reviews it, and it applies everywhere at once.

Two consequences, and they are the product:

- **Drift is structurally impossible**, not discouraged. There is no per-project override to
  drift from.
- **Complexity has a visible price.** The 700th field is a line in a diff that a human has to
  approve — the moment Jira never has.

The organisation can be as complex as it genuinely needs. It just has to be complex
**deliberately, visibly, in one place, with a reviewer**.

### Other opinions Canon holds

- **One entity.** All work is an `Issue` with an optional parent. Epics, stories and sub-tasks are
  parent/child relations, not types. Boards are saved queries.
- **No estimation.** There is no story point, velocity or burndown field, and none will be added.
  Flow is measured from recorded state transitions, not guessed in advance.
- **Agents are first-class.** One API serves the UI, the CLI and agents, with a test asserting
  parity. Every write records whether a human or an agent made it, and which model.
- **Agents propose, humans decide.** An operation an agent may not perform outright returns
  `202 proposal_required` rather than a refusal, so the attempt is recorded for a human.
- **History is the storage model.** Canon stores an append-only event log; current state is a
  projection you can discard and rebuild at any time.

## Quick start

Requires Go 1.26+ to build. No database to provision, nothing else to install.

```bash
git clone https://github.com/ofenton/canon.git && cd canon
make build

cp internal/schema/testdata/canon.yaml .            # a realistic starting schema
./bin/canon bootstrap -actor you -team platform     # create the first admin, once
./bin/canon serve                                   # listens on :8080
```

In another terminal:

```bash
A=http://localhost:8080/api
H='X-Canon-Actor: you'

curl -X POST $A/issues -H "$H" -d '{"title":"Search is slow","team":"platform"}'
# {"id":"CANON-1"}

curl $A/issues/CANON-1 -H "$H"
curl $A/schema -H "$H"        # what this organisation permits
```

### Try to break it

This is the part worth five minutes:

```bash
curl -X PATCH $A/issues/CANON-1/fields -H "$H" -d '{"storyPoints":"8"}'
# 422  field "storyPoints" is not defined in the schema; defined fields are component,
#      evidence, priority, title

curl -X POST $A/issues/CANON-1/transition -H "$H" -d '{"to":"done"}'
# 422  CANON-1 cannot move from "todo" to "done"; permitted transitions from "todo"
#      are abandoned, in_progress

curl -X POST $A/issues -d '{"title":"x"}'
# 401  X-Canon-Actor header is required
```

Errors name what you should have done, not just that you were wrong. That matters most for
agents, which can act on the first and only retry blindly on the second.

Now edit `canon.yaml` — add a `sprints:` key, misspell a state in a transition, remove a state
that issues are sitting in — and restart. It refuses to start and tells you the line.

### The agent path

```bash
curl -X POST $A/actors -H "$H" -d '{"id":"agent:one","kind":"agent","model":"claude-opus-5"}'
curl -X POST $A/actors/agent:one/roles -H "$H" -d '{"role":"agent"}'
curl -X POST $A/actors/agent:one/teams -H "$H" -d '{"team":"platform"}'

AH='X-Canon-Actor: agent:one'
curl -X POST $A/issues/CANON-1/transition -H "$AH" -d '{"to":"in_progress"}'
# 204

curl -X POST $A/issues/CANON-1/transition -H "$AH" -d '{"to":"in_review"}'
# 422  state "in_review" requires evidence; supply it with the transition

curl -X POST $A/issues/CANON-1/transition -H "$AH" \
     -d '{"to":"in_review","evidence":"312 passed in 41s"}'
# 204

curl -X POST $A/issues/CANON-1/transition -H "$AH" -d '{"to":"done"}'
# 202  {"status":"proposal_required","operation":"transition:in_review->done"}
```

The agent may start work and move it to review with evidence, but completing it is a proposal for
a human. That is declared in `canon.yaml`, not in code.

## canon.yaml

```yaml
version: 1

states:
  - {name: todo, category: open}
  - {name: in_progress, category: active}
  - {name: in_review, category: active, requires_evidence: true}
  - {name: done, category: closed}

transitions:
  - {from: todo, to: in_progress}
  - {from: in_progress, to: in_review}
  - {from: in_review, to: done}

fields:
  - {name: title, type: string, required: true}
  - {name: priority, type: enum, values: [p1, p2, p3, p4]}

issue_types:
  - {name: bug, fields: [title, priority]}

roles:
  - name: admin
    can: [create, delete, reparent, "field:*", "transition:*"]
  - name: member
    scope: team                       # only issues owned by a team you belong to
    can: [create, reparent, "field:*", "transition:*"]
  - name: agent
    scope: team
    can: [create, "field:*", "transition:todo->in_progress"]
    propose: [delete, "transition:*"] # anything else awaits a human
```

`category` is a closed set — open, active, closed. That is the direct answer to "completed has
sixteen spellings": without a fixed grouping, no cross-team question has a true answer.

Every grant is validated against the rest of the schema at load. `field:storyPoints` or
`transition:todo->shipped` is refused by name, because a typo in a permission grants nothing and
is invisible at runtime.

**Roles are policy; membership is state.** Which roles exist lives in `canon.yaml` and changes by
pull request. Who holds one, and which team they are in, lives in the event log and changes by
API call — making every joiner a pull request would teach people to route around the system.

## API

One API. The CLI, agents and (eventually) the web UI all use it; a test fails if any route exists
outside `/api`, or if the contract test does not exercise every route.

Every request needs an `X-Canon-Actor` header naming a registered actor.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/schema` | The organisation's schema, with permitted transitions per state |
| `GET` | `/api/events` | The raw event log (`?subject=`, `?since=`) |
| `GET` | `/api/issues` | List issues (`?state=`, `?team=`) |
| `POST` | `/api/issues` | Create — only `title` is required |
| `GET` | `/api/issues/{id}` | One issue's projected state |
| `DELETE` | `/api/issues/{id}` | Delete; children are lifted to the grandparent |
| `PATCH` | `/api/issues/{id}/fields` | Set one or more fields |
| `POST` | `/api/issues/{id}/transition` | `{"to": …, "evidence": …}` |
| `PUT` | `/api/issues/{id}/parent` | Set or clear the parent |
| `GET` | `/api/issues/{id}/children` | Direct children |
| `GET` | `/api/proposals` | Open proposals awaiting a human (`?status=all` for history) |
| `GET` | `/api/proposals/{id}` | One proposal |
| `POST` | `/api/proposals/{id}/approve` | Apply it, on the approver's authority |
| `POST` | `/api/proposals/{id}/reject` | Decline it, with an optional `reason` |
| `GET` | `/api/metrics` | Measured flow (`?days=`, `?q=` to scope it) |
| `GET` | `/api/actors` | Registered actor ids |
| `POST` | `/api/actors` | Register a human or agent |
| `GET` | `/api/actors/{id}` | An actor's roles and teams |
| `POST` | `/api/actors/{id}/roles` | Grant a role |
| `DELETE` | `/api/actors/{id}/roles/{role}` | Revoke a role |
| `POST` | `/api/actors/{id}/teams` | Add to a team |
| `DELETE` | `/api/actors/{id}/teams/{team}` | Remove from a team |

**Status codes.** `422` means the schema or your role refused it. `401` means the actor header is
missing or names nobody. `202` with `proposal_required` means an agent's attempt was recorded for
a human — a different outcome from a refusal, and worth handling differently.

## Commands

```
canon bootstrap -actor <id> [-team <t>]   create the first admin on an empty log
canon serve [-addr :8080]                 run the HTTP API
canon mcp -actor <id>                     serve MCP over stdio, for agents
canon schema                              validate canon.yaml and summarise it
canon events [-subject <id>] [-since <n>] print the event log as JSON
canon rebuild                             discard projections and replay the log
canon version
```

All accept `-db` (default `canon.db`) and, where relevant, `-schema` (default `canon.yaml`).

## Agents

Canon speaks MCP over stdio. Point an agent at it:

```json
{ "mcpServers": { "canon": {
    "command": "/path/to/canon",
    "args": ["mcp", "-actor", "agent:one", "-db", "/path/to/canon.db",
             "-schema", "/path/to/canon.yaml"]
} } }
```

The tools are **derived from the HTTP route table**, so an agent can do everything a human can —
21 routes, 21 tools, verified by a test rather than by discipline. Calls dispatch through the same
handler the network serves, so an agent and a human take an identical path through authorisation.

A refusal comes back as an error an agent can act on; a proposal does not:

```
create_issue         {"title":"Search is slow"}          → {"id":"CANON-1"}
update_issue_fields  {"id":"CANON-1","storyPoints":"8"}  → isError, field "storyPoints" is not
                                                            defined in the schema
transition_issue     {"id":"CANON-1","to":"done"}        → proposal_required, PROP-1
```

The last one is not an error. The attempt was recorded for a human, which from the agent's point
of view succeeded.

## Measurement, not estimation

Canon has no story point field and will not get one. Estimates get inflated under pressure to make
velocity rise, they are inconsistent between people, and they measure the guess rather than the
work. A field named like an estimate is refused at startup:

```
$ canon schema
canon: field "storyPoints" is an estimate; Canon measures flow from recorded transitions
and has no estimation. Remove it, or use cycle time and throughput instead
```

What you get instead comes from timestamps that were recorded anyway:

```
$ curl "$A/metrics?days=30" -H "$H"

completed 9   started 11   in progress 2
cycle time (active→closed)   p50   2d   p85   7d   p95  11d   max  11d   (n=9)
lead time (created→closed)   p50   2d   p85   7d   p95  11d   max  11d   (n=9)
slowest: CANON-9 11d, CANON-6 7d, CANON-8 3d
ageing (unfinished, oldest first):
  WIP-2    in_progress  18d
  WIP-1    in_progress  10d
```

Three deliberate choices:

- **No mean.** Cycle times are long-tailed and an average hides the tail people actually complain
  about. Here p50 is 2 days and p85 is 7.
- **Ageing is reported for unfinished work.** Cycle time only moves once something finishes. The
  oldest thing still in progress moves *before* the damage lands, and is the number to watch.
- **Lead time alongside cycle time.** Cycle time is what the team controls; lead time is what the
  requester waits. Reporting only the first is how a team convinces itself things are fine while
  the queue grows.

`?q=` accepts the query language, so flow can be measured per team or per component without a
separate reporting concept.

## How it stores things

Canon stores **events, not state**: `issue.created`, `field.set`, `issue.transitioned`,
`actor.role_granted`. Current state is a projection produced by replaying them, and the projection
is a cache with no authority — `canon rebuild` discards and reproduces it, which turns a
projection bug into a five-minute fix rather than a data repair script.

Events are canonical CBOR in a SQLite table with triggers that reject `UPDATE` and `DELETE`, so
the log is append-only as a property of the database rather than a habit of its callers.
`canon events` renders any of it as human-readable JSON.

This buys three things at once: history is inherent rather than bolted on, backup is copying one
file, and a second log home becomes a transport rather than a rewrite — appends commute, so two
clones merge by concatenation. That last point is why the storage layer is shaped this way; see
[ADR-0003](docs/decisions/0003-storage-history-and-federation.md).

## What is not built

Honest list, so nobody is surprised:

- **Authentication.** `X-Canon-Actor` is trusted. It must name a registered actor with real roles,
  which is a meaningful narrowing, but anyone who can reach the port can claim any registered
  identity. **Do not expose an instance to a network you do not control.**
- **Web UI.** API and CLI only so far.

- **Queries, boards and flow metrics.** Planned.
- **Federated repo-local storage.** The event model is designed for it; the transport is not built.
- **Jira import.** Wanted, not started.

Deliberately excluded and not coming: story points, velocity, burndown, per-project workflow
customisation, a plugin marketplace, and bundled documents/chat/video.

## Development

```bash
make check     # vet, workflow lint, tests
make build     # static binary into bin/
make bench     # benchmarks
```

The repository is developed under a spec-anchored increment workflow — see
[`AGENTS.md`](AGENTS.md). Work is planned in [`specs/increment-plan.md`](specs/increment-plan.md),
each increment carries acceptance criteria in EARS notation, and evidence for every completed one
is in [`specs/increments/`](specs/increments/). Decisions are recorded as ADRs in
[`docs/decisions/`](docs/decisions/).

Four validators run in the pre-commit hook and in CI: the ledger is well formed, skills match the
Agent Skills spec, the ledger matches git history, and no ignored file is tracked.

## Licence

Apache-2.0. See [LICENSE](LICENSE).
