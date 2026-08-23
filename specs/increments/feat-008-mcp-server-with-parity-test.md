# feat-008: MCP server with parity test

## Context

The agent-first claim is marketing unless an agent can do everything a human can. This makes that
structural rather than aspirational.

## Design notes

**Tools are derived from the HTTP route table, not written alongside it.** A hand-maintained tool
list drifts the first time someone is in a hurry, and the drift is invisible until an agent cannot
do something a human can. Deriving them makes parity a property of the code; the test then
verifies the property rather than enumerating a list that would need maintaining too.

**Calls dispatch through the same `http.Handler` the network serves.** Reimplementing dispatch
for MCP would create exactly the second code path this design exists to prevent — and it is the
path where authorisation would eventually differ.

**A 202 proposal is not `isError`.** From the agent's point of view the attempt succeeded: it was
recorded for a human. Marking it an error would teach agents to treat proposals as failures and
stop, which defeats the mechanism.

**Tool names are singular except for listings** — `create_issue`, `list_issues`. They are the
surface an agent reasons about before reading any description, so they are worth getting right
even though they are generated. `singular()` is deliberately naive and covers this API's actual
nouns; a pluralisation library would be a dependency earning nothing.

**`-actor` is required, never defaulted.** An agent silently acting as whoever happens to be
first in the registry would be worse than a clear error, because every event records who caused it.

**Descriptions are checked in both directions.** A route without a description fails the suite,
and a description for a route that no longer exists fails too — otherwise stale guidance
accumulates and misleads agents.

## Evidence

**Verified by:** implementing session, `inc/feat-008-mcp`

### An MCP tool for every API operation, enforced

```
--- PASS: TestToolParityWithTheAPI
--- PASS: TestEveryRouteHasADescription
```

Parity is asserted on counts and names, and every tool must have a written description, a unique
name in `verb_noun` form, and an object input schema.

```
$ canon mcp -actor ollie   ← tools/list
21 tools for 21 routes

  create_issue         Create an issue. Only a title is required.
  transition_issue     Move an issue to a new state. Some states require evidence…
  approve_proposal     Approve a proposal and apply it. Humans only.
  list_proposals       List proposals awaiting a human decision…
  …
```

### Adding a route without a tool fails the suite

Parity holds by construction — a new route produces a tool automatically — but a route with no
description fails `TestEveryRouteHasADescription`, so a new endpoint cannot ship with the raw
pattern as its agent-facing guidance.

### Identical schema validation over MCP

```
$ canon mcp -actor ollie
call create_issue        → isError=false  {"id":"CANON-1"}
call update_issue_fields → isError=true   field "storyPoints" is not defined in the schema;
                                          defined fields are component, evidence, priority, title
```

```
--- PASS: TestSchemaValidationIsIdenticalOverMCP
--- PASS: TestFullLifecycleOverMCPOnly
--- PASS: TestInitializeAndList
--- PASS: TestMissingArgumentIsReported
--- PASS: TestUnknownToolAndMethod
```

`TestFullLifecycleOverMCPOnly` drives create → in_progress → in_review with evidence → proposal →
human approval entirely through MCP, with the agent and the admin as separate MCP sessions.

### Scope

`git diff --cached --stat main` — run. The server in `internal/mcp`, the `mcp` command in
`cmd/canon`, and a README section. Documenting a feature in the same increment that builds it is
a deliberate small deviation: the alternative is a README that is wrong between merges.

### Not verified

Not yet run against a real MCP client — only against the protocol as specified. The handshake,
`tools/list` and `tools/call` are covered; resources, prompts and notifications are not
implemented, as Canon has no use for them.

CI runs on the pull request.
