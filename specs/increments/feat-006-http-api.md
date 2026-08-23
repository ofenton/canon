# feat-006: HTTP API

## Context

The single interface. The UI, CLI, agents and the MCP server all speak this and nothing else.

## Design notes

**`Routes()` returns the surface as data**, not scattered registration calls. That is what lets
the parity test enumerate every route and fail if one is never exercised — and it lets a reader
see the whole interface at once.

**A proposal is 202 Accepted, not 403.** The request was understood and recorded for a human,
which is a materially different outcome from being refused. An agent that gets 403 should stop;
an agent that gets 202 should tell someone.

**Domain errors reach the caller verbatim.** They already name what the caller should have done —
`permitted transitions from "todo" are abandoned, in_progress` — and rewording at the edge would
discard the thing that makes them useful to an agent.

**Everything except the title has a defensible default on create.** Issue type defaults to the
first in `canon.yaml`, state to the first `open` state, id to the next sequential. A create that
demands twelve fields is precisely what this product exists to remove.

**Scope deviation: `canon bootstrap` was added.** Registering an actor requires an actor, so an
empty log could admit nobody over HTTP — the API was unusable on a fresh install. The tests did
not catch it because they call the enforcer directly; **only running the binary did**.

The obvious fix, letting the first HTTP caller register themselves, puts a privileged
unauthenticated path on the network. That is a bad trade even for a tool that does not yet
authenticate, and it is the kind of thing that survives into v2. So bootstrap is a local command:
it needs filesystem access to the log, which is a real authorisation boundary and one the
operator already crossed to install Canon. It refuses to run on a non-empty registry.

## Evidence

**Verified by:** implementing session, `inc/feat-006-http-api`

### WHEN a caller creates an issue supplying only a title THE SYSTEM SHALL create it successfully

```
$ curl -X POST /api/issues -H 'X-Canon-Actor: ollie' -d '{"title":"Search is slow","team":"platform"}'
{"id":"CANON-1"}

$ curl /api/issues/CANON-1 -H 'X-Canon-Actor: ollie'
{'ID': 'CANON-1', 'Title': 'Search is slow', 'State': 'todo', 'Team': 'platform',
 'LastActor': {'ID': 'ollie', 'Kind': 'human'}}
```

```
--- PASS: TestCreateNeedsOnlyATitle
```

### THE SYSTEM SHALL expose every read and write operation over one HTTP API

```
--- PASS: TestEveryRouteIsExercised
```

All 17 routes are called by the contract test, and the test fails if `Routes()` ever contains one
the test does not exercise — so a new endpoint cannot be added without proving it works.

### THE SYSTEM SHALL contain no endpoint reachable only by the web UI

```
--- PASS: TestNoUIOnlyRoutes
```

Every route must sit under `/api/` and must not contain `/ui/`, `/internal/`, `/_` or `/web/`.

### Authorisation holds at the boundary, not only in the domain

```
storyPoints  422  field "storyPoints" is not defined in the schema; defined fields are component…
todo->done   422  CANON-1 cannot move from "todo" to "done"; permitted transitions are abandoned…
no header    401
agent->done  202  proposal_required  transition:in_review->done
```

```
--- PASS: TestAuthorisationAtTheBoundary (7 cases)
--- PASS: TestAgentProposalIsAccepted
--- PASS: TestErrorsReachTheCaller
--- PASS: TestListFilters
```

### Bootstrap

```
$ canon bootstrap -actor ollie -team platform
registered ollie as admin in team platform

$ canon bootstrap -actor mallory
canon: this log already has 1 actor(s) (ollie); bootstrap only runs on an empty
registry, use the API to add more
```

### Scope

`git diff --cached --stat main` — run. The API in `internal/api`, `serve` and `bootstrap` in
`cmd/canon`. `bootstrap` is the deviation described above.

### Not verified

Still no authentication: `X-Canon-Actor` is trusted. It must now name a registered actor with
real roles, which is a meaningful narrowing, but anyone who can reach the port can claim any
registered identity. Do not expose this instance to a network you do not control.

CI runs on the pull request.
