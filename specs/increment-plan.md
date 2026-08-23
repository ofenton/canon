# Increment plan

The ledger. This file is the single source of truth for what is planned, being built, and shipped.
Edit it only through the conventions in `skills/track-increment-state/SKILL.md`, and run
`python3 .sdlc/bin/validate-plan.py` after every change.

At most one increment may be `in-progress` at a time.

---

## chore-001: Adopt the increment workflow

_Your first increment. Commit the scaffold with `Increment: chore-001` in the trailer, then
mark this done — that is the whole loop, run once, on the workflow itself._

- **Type:** chore
- **Status:** done
- **Traces:** none
- **Tier:** 2 (High)
- **Scope:** Add `AGENTS.md`, `skills/`, `.sdlc/` and `specs/` to the repository, and write the
  project constitution. No product code changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL provide an `AGENTS.md` naming the three planes, the two gates and the three tracks
  - [x] WHEN `validate-skills.py` runs THE SYSTEM SHALL report every skill valid
  - [x] WHEN `validate-plan.py` runs THE SYSTEM SHALL report the ledger well formed
  - [x] WHEN `check-traceability.py` runs THE SYSTEM SHALL report the ledger traces cleanly
  - [x] THE SYSTEM SHALL provide a `docs/constitution.md` agreed by a human
- **Test Strategy:**
  - Run both validators from a clean checkout
  - Start a fresh agent session and confirm it finds the ledger unprompted
- **Dependencies:** none
- **Rollback Plan:** Delete `.sdlc/`, `skills/`, `specs/` and `AGENTS.md`
- **Risk:** Low — additive, touches no product code
- **Evidence:** the scaffold is in use; all three validators pass on every commit and in CI

## chore-002: Set up the Go module, licence and CI

- **Type:** chore
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** none
- **Scope:** Create the Go module, `LICENSE` (Apache-2.0), `Makefile`, and a CI workflow running build, vet and test. No application code. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN `make build` runs THE SYSTEM SHALL produce a single static binary with no external runtime dependencies
  - [x] WHEN `make test` runs THE SYSTEM SHALL execute the test suite and exit non-zero on failure
  - [x] THE SYSTEM SHALL carry an Apache-2.0 LICENSE file at the repository root
- **Test Strategy:**
  - Build on a clean machine with no Go cache
  - Confirm `ldd` reports a static binary
- **Dependencies:** none
- **Rollback Plan:** Delete the Go module files; no product code depends on this yet
- **Risk:** Low — scaffolding only
- **Evidence:** PR #1 merged, CI run 32600923478 green; two scope violations recorded in `specs/increments/chore-002-set-up-the-go-module-licence-and-ci.md`

## feat-001: Append-only event log with actor provenance

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R12
- **Scope:** Define the event schema (CBOR-encoded, versioned) and an append-only store over SQLite. Every event carries id, timestamp, actor id, actor kind (human or agent), and model identifier where applicable. No projection, no API yet. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN an event is appended THE SYSTEM SHALL record actor id, actor kind and timestamp and SHALL NOT permit modification of any earlier event
  - [x] WHEN an event is appended with an unknown schema version THE SYSTEM SHALL reject it naming the supported versions
  - [x] THE SYSTEM SHALL append 10,000 events in under 2 seconds on commodity hardware
- **Test Strategy:**
  - Property test: appends never mutate prior events
  - Round-trip test: every event type encodes and decodes losslessly
  - Benchmark: 10k append throughput
- **Dependencies:** chore-002
- **Rollback Plan:** Drop the events table; nothing consumes it yet
- **Risk:** High — the event schema is the one thing federation depends on and the one thing that cannot be migrated cheaply
- **Evidence:** see `specs/increments/feat-001-append-only-event-log-with-actor-provenance.md`

## feat-002: Projection engine with snapshots

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R12
- **Scope:** Replay events into current-state projections, with periodic snapshots to bound replay cost, and a `canon rebuild` command that discards and rebuilds every projection from the log. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN `canon rebuild` runs THE SYSTEM SHALL discard all projections and reproduce identical state from the event log
  - [x] WHEN a snapshot exists THE SYSTEM SHALL replay only events after it
  - [x] THE SYSTEM SHALL rebuild projections for 10,000 events in under 5 seconds
- **Test Strategy:**
  - Determinism test: rebuild twice, assert byte-identical projections
  - Snapshot test: state with and without snapshots matches
  - Benchmark: rebuild at 10k events
- **Dependencies:** feat-001
- **Rollback Plan:** Revert to replaying the full log on every read
- **Risk:** Medium — projection bugs are recoverable by rebuild, which is the point
- **Evidence:** see `specs/increments/feat-002-projection-engine-with-snapshots.md`

## feat-003: Load and validate canon.yaml

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R1, R2
- **Scope:** Read the organisation schema — issue types, states, transitions, fields, permissions — from a single `canon.yaml`. Validate it on startup. No enforcement on writes yet. No other changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL read the entire issue schema from one `canon.yaml` at a configured path
  - [x] WHEN `canon.yaml` is syntactically invalid THE SYSTEM SHALL refuse to start and name the offending line number
  - [x] WHEN `canon.yaml` references an undefined state in a transition THE SYSTEM SHALL refuse to start and name the transition
- **Test Strategy:**
  - Table test over malformed schemas asserting the reported line number
  - Golden test: a realistic org schema loads and round-trips
- **Dependencies:** chore-002
- **Rollback Plan:** Fall back to a hardcoded default schema
- **Risk:** Low — startup-time validation, fails loudly
- **Evidence:** see `specs/increments/feat-003-load-and-validate-canon-yaml.md`

## feat-004: Enforce the schema on every write

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R3, R4, R5, R6
- **Scope:** Reject writes that use fields, states or transitions not in `canon.yaml`. Provide no runtime interface for adding them. Refuse schema changes that would orphan existing issues. Apply additive changes without downtime. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN a caller sets a field not defined in `canon.yaml` THE SYSTEM SHALL reject the write and name the valid fields
  - [x] WHEN a caller transitions to a state not permitted from the current state THE SYSTEM SHALL reject the write and name the permitted transitions
  - [x] THE SYSTEM SHALL expose no API or UI operation that adds a field, state or issue type at runtime
  - [x] WHEN a schema change would leave existing issues in an undefined state THE SYSTEM SHALL refuse to apply it and list the affected issue ids
  - [x] WHEN a schema change is purely additive THE SYSTEM SHALL apply it without restart or data migration
- **Test Strategy:**
  - Fuzz unknown field and state names against the write path
  - Orphan test: remove a state in use, assert refusal and the id list
  - Assert no route or command mutates the schema
- **Dependencies:** feat-003, feat-001
- **Rollback Plan:** Disable enforcement, accepting any field — reverts Canon to a normal tracker
- **Risk:** Medium — this is the product's central claim, so the tests matter more than the code
- **Evidence:** see `specs/increments/feat-004-enforce-the-schema-on-every-write.md`

## feat-005: Issue entity with parent/child hierarchy

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R7, R8, R10
- **Scope:** One `Issue` entity with an optional parent, expressed as events. Epics, stories and sub-tasks are parent/child relations, not storage types. Deleting an issue re-parents its children. No other changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL store all work as a single Issue entity with an optional parent reference
  - [x] THE SYSTEM SHALL contain no storage-level distinction between epic, story and sub-task
  - [x] WHEN an issue with children is deleted THE SYSTEM SHALL re-parent its children to that issue's parent
  - [x] WHEN a parent reference would create a cycle THE SYSTEM SHALL reject the write
- **Test Strategy:**
  - Cycle detection test across a deep hierarchy
  - Re-parent test: delete a mid-tree node, assert grandchildren survive
  - Schema inspection test asserting one issue table
- **Dependencies:** feat-002, feat-004
- **Rollback Plan:** Revert to a flat issue list with no parent field
- **Risk:** Low — small model, well understood
- **Evidence:** see `specs/increments/feat-005-issue-entity-with-parent-child-hierarchy.md`

## feat-006: HTTP API

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R11, R16
- **Scope:** The single API used by the UI, CLI and agents. Create, read, update, transition, list. Creating an issue requires only a title. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN a caller creates an issue supplying only a title THE SYSTEM SHALL create it successfully
  - [x] THE SYSTEM SHALL expose every read and write operation over one HTTP API
  - [x] THE SYSTEM SHALL contain no endpoint reachable only by the web UI
- **Test Strategy:**
  - Contract test covering every documented endpoint
  - Assertion test: the route table contains no UI-only route
  - Title-only creation test
- **Dependencies:** feat-005
- **Rollback Plan:** Revert to the CLI as the only interface
- **Risk:** Low — thin layer over the event log
- **Evidence:** see `specs/increments/feat-006-http-api.md`

## feat-007: Agent identity, provenance and proposals

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R12, R14, R15
- **Scope:** Per-actor identity records with an id, kind and optional signing key. Transitions marked `requires_evidence` in the schema are rejected without evidence. An agent lacking permission creates a proposal for human approval rather than failing. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN an agent performs any mutation THE SYSTEM SHALL record its actor id and model identifier on the resulting event
  - [x] WHEN an agent transitions an issue to a state marked requires_evidence without supplying evidence THE SYSTEM SHALL reject the transition
  - [x] WHEN an agent attempts a transition it lacks permission for THE SYSTEM SHALL record a proposal awaiting human approval and return the proposal id
  - [x] WHEN a human approves a proposal THE SYSTEM SHALL apply the original transition with both actors recorded
- **Test Strategy:**
  - Evidence test across every requires_evidence transition in the test schema
  - Proposal lifecycle test: create, approve, reject
  - Provenance test asserting model id survives a projection rebuild
- **Dependencies:** feat-006, feat-014, feat-015
- **Rollback Plan:** Treat agents as ordinary actors with no proposal path
- **Risk:** Medium — the proposal flow is new behaviour with no direct prior art to copy
- **Evidence:** see `specs/increments/feat-007-agent-identity-provenance-and-proposals.md`

## feat-008: MCP server with parity test

- **Type:** feature
- **Status:** in-progress
- **Tier:** 1 (Critical)
- **Traces:** R13
- **Scope:** Expose every API operation over MCP, with an automated test asserting the MCP tool list covers the full API surface. No other changes.
- **Acceptance Criteria:**
  - [ ] THE SYSTEM SHALL expose an MCP tool for every operation available in the HTTP API
  - [ ] WHEN an API operation is added without a corresponding MCP tool THE SYSTEM SHALL fail its test suite
  - [ ] WHEN an agent calls an MCP tool THE SYSTEM SHALL apply identical schema validation to the equivalent HTTP call
- **Test Strategy:**
  - Parity test enumerating API routes against MCP tools
  - Negative test: add a route in the test fixture, assert the parity test fails
  - End-to-end: drive a full issue lifecycle over MCP only
- **Dependencies:** feat-006
- **Rollback Plan:** Remove the MCP server; agents fall back to the HTTP API
- **Risk:** Low — mechanical once the API exists, and the parity test is the valuable part
- **Evidence:** _(filled in at verify)_

## feat-009: Queries and boards as saved queries

- **Type:** feature
- **Status:** approved
- **Tier:** 2 (High)
- **Traces:** R9
- **Scope:** A query language over issues, saved queries, and boards expressed as a saved query plus a grouping key. Boards hold no state. No other changes.
- **Acceptance Criteria:**
  - [ ] THE SYSTEM SHALL express a board as a saved query and a grouping key with no state of its own
  - [ ] WHEN an issue stops matching a board's query THE SYSTEM SHALL cease to show it on that board with no separate update
  - [ ] WHEN a query references a field not in canon.yaml THE SYSTEM SHALL reject it naming the valid fields
- **Test Strategy:**
  - Board test: change an issue field, assert board membership follows without a write to the board
  - Schema inspection asserting no board state table
  - Query parser table tests
- **Dependencies:** feat-006
- **Rollback Plan:** Ship fixed list views with no saved queries
- **Risk:** Medium — the query language is easy to over-build; keep it small
- **Evidence:** _(filled in at verify)_

## feat-010: Flow metrics without estimation

- **Type:** feature
- **Status:** approved
- **Tier:** 2 (High)
- **Traces:** R19, R20
- **Scope:** Cycle time and throughput computed from recorded state transitions. No estimate field anywhere. No other changes.
- **Acceptance Criteria:**
  - [ ] WHEN an operator requests flow metrics THE SYSTEM SHALL report cycle time and throughput derived from recorded state transitions
  - [ ] THE SYSTEM SHALL provide no story point, velocity, estimate or burndown field in the schema, API or UI
  - [ ] WHEN canon.yaml defines a field named as an estimate THE SYSTEM SHALL refuse to start
- **Test Strategy:**
  - Metrics test against a fixture with known transition timestamps
  - Assertion test scanning schema, API and UI for estimate-shaped fields
  - Startup-refusal test for an estimate field in canon.yaml
- **Dependencies:** feat-009
- **Rollback Plan:** Remove the metrics endpoint; transitions are still recorded so metrics can be added later
- **Risk:** Low — pure derivation from data already recorded
- **Evidence:** _(filled in at verify)_

## feat-011: Keyboard-first web UI

- **Type:** feature
- **Status:** approved
- **Tier:** 1 (Critical)
- **Traces:** R18
- **Scope:** List, detail and create views, embedded in the binary. Every action reachable by keyboard without pointer input. No other changes.
- **Acceptance Criteria:**
  - [ ] THE SYSTEM SHALL make every action available in the UI reachable by keyboard without pointer input
  - [ ] WHEN a user presses the create shortcut THE SYSTEM SHALL open a title-only create field focused and ready for input
  - [ ] THE SYSTEM SHALL serve the UI from the binary with no separate asset deployment
- **Test Strategy:**
  - Playwright run driving a full issue lifecycle by keyboard only
  - Assert the binary serves the UI with no filesystem assets present
  - Screenshot comparison for the list and detail views
- **Dependencies:** feat-006
- **Rollback Plan:** Serve the API only; the CLI remains usable
- **Risk:** Medium — UI work is the easiest thing to overrun on a deadline
- **Evidence:** _(filled in at verify)_

## feat-012: Meet the latency budget at 10,000 issues

- **Type:** perf
- **Status:** approved
- **Tier:** 2 (High)
- **Traces:** R17
- **Scope:** Benchmark and tune reads against a seeded 10,000-issue dataset until the budget is met. Indexes and query plans only; no model changes. No other changes.
- **Acceptance Criteria:**
  - [ ] WHEN any read request is served against a 10,000-issue project THE SYSTEM SHALL respond in under 200ms at p95
  - [ ] THE SYSTEM SHALL include a reproducible benchmark in the repository that fails CI if the budget regresses
- **Test Strategy:**
  - Seed 10k issues, measure p95 across every read endpoint
  - Wire the benchmark into CI as a failing gate
  - Profile before and after, record both
- **Dependencies:** feat-009
- **Rollback Plan:** Remove the CI latency gate; correctness is unaffected
- **Risk:** Medium — if the projection shape is wrong this reveals it late, which is why the benchmark is written early
- **Evidence:** _(filled in at verify)_

## feat-013: One-command self-host and single-file backup

- **Type:** feature
- **Status:** approved
- **Tier:** 1 (Critical)
- **Traces:** R21, R22
- **Scope:** `canon serve` starts a working instance with no external services. All data in one file that can be copied as a backup. README covering install, run and backup. No other changes.
- **Acceptance Criteria:**
  - [ ] WHEN an operator runs `canon serve` THE SYSTEM SHALL start a working instance with no external service dependencies
  - [ ] THE SYSTEM SHALL store all data in a single file that can be copied while running to produce a valid backup
  - [ ] WHEN a copied data file is restored THE SYSTEM SHALL start with identical state
- **Test Strategy:**
  - Fresh-container test: download the binary, run it, create an issue
  - Backup and restore test asserting state equality
  - Copy the file under concurrent writes and assert the restore is valid
- **Dependencies:** feat-011
- **Rollback Plan:** Document a manual multi-step setup instead
- **Risk:** Low — mostly packaging, but it is the whole self-host story
- **Evidence:** _(filled in at verify)_

## chore-003: Dogfood: run this project in Canon

- **Type:** chore
- **Status:** approved
- **Tier:** 2 (High)
- **Traces:** none
- **Scope:** Import this repository's increment ledger into Canon and track the remaining work there. No product code changes. No other changes.
- **Acceptance Criteria:**
  - [ ] WHEN the ledger is imported THE SYSTEM SHALL contain every increment with its status and history
  - [ ] THE SYSTEM SHALL be the place this project's work is tracked from the import onward
- **Test Strategy:**
  - Import, then compare issue count and statuses against the ledger
  - Use it for one working day and record what broke
- **Dependencies:** feat-013
- **Rollback Plan:** Continue tracking in specs/increment-plan.md
- **Risk:** Low — read-only import, and the most convincing part of the demo
- **Evidence:** _(filled in at verify)_


## feat-014: Roles and permissions in canon.yaml

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R30, R31, R32
- **Scope:** Add a `roles:` section to `canon.yaml` defining each role, the operations it permits, and an optional `scope: team`. Enforce it on every write in `enforce`. Add an `owner_team` field concept to issues so team scope has something to resolve against. No other changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL define every role and the operations it permits in `canon.yaml`, with no per-project override
  - [x] WHEN an actor attempts an operation their role does not permit THE SYSTEM SHALL reject it and name the roles that would permit it
  - [x] WHEN a role is declared `scope: team` THE SYSTEM SHALL permit its operations only on issues owned by a team that actor belongs to
  - [x] WHEN `canon.yaml` grants a role an operation that does not exist THE SYSTEM SHALL refuse to start and name it
  - [x] THE SYSTEM SHALL expose no runtime interface for creating or altering a role
- **Test Strategy:**
  - Table test over each role against each operation, permitted and refused
  - Team-scope test: same role, two teams, one issue — permitted for the owner, refused for the other
  - Source-assertion test that no AddRole or GrantPermission function exists
  - Regression: the existing enforcement suite still passes
- **Dependencies:** feat-004
- **Rollback Plan:** Remove the roles section and the permission check; enforcement returns to schema-only
- **Risk:** Medium — new schema surface, and the operation vocabulary must be right before roles reference it
- **Evidence:** see `specs/increments/feat-014-roles-and-permissions-in-canon-yaml.md`

## feat-015: Actor registry and team membership

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R33
- **Scope:** Record actor identities and team membership as events (`actor.registered`, `team.member_added`, `team.member_removed`) and project them. Resolve an actor's roles and teams at write time. No authentication. No other changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL record actor identities and team membership as events in the log, not in `canon.yaml`
  - [x] WHEN an actor is granted a role THE SYSTEM SHALL apply it to subsequent writes without a restart
  - [x] WHEN an unregistered actor attempts a write THE SYSTEM SHALL reject it naming the actor
  - [x] WHEN membership changes THE SYSTEM SHALL retain the prior membership in the log, so past events remain explicable
- **Test Strategy:**
  - Membership lifecycle test: add, act, remove, act — permitted then refused
  - Projection test: rebuilding reproduces identical membership
  - Test that a role granted mid-log applies only to events after it
- **Dependencies:** feat-014
- **Rollback Plan:** Treat every actor as holding a single default role
- **Risk:** Medium — introduces a second projected entity alongside issues
- **Evidence:** see `specs/increments/feat-015-actor-registry-and-team-membership.md`
## docs-001: Project README

- **Type:** docs
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** none
- **Scope:** Replace the inherited template README with one describing Canon: the problem, the wedge, how to run it, and the API. Documentation only. No other changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL describe what Canon is and the problem it addresses, with the evidence behind it
  - [x] WHEN a reader follows the quick start THE SYSTEM SHALL take them from clone to a working instance
  - [x] THE SYSTEM SHALL document every API route currently implemented
  - [x] THE SYSTEM SHALL state plainly what is not built yet, including the absence of authentication
- **Test Strategy:**
  - Follow the quick start verbatim on a clean checkout and confirm each command works
  - Cross-check the documented routes against `Routes()`
- **Dependencies:** feat-006
- **Rollback Plan:** Restore the previous README from git history
- **Risk:** Low — documentation only, but it is the first thing anyone sees


---
- **Evidence:** see `specs/increments/docs-001-project-readme.md`

## Sequencing

| Day | Increments | Milestone |
|---|---|---|
| ~~Mon~~ | ~~chore-002, feat-001~~ | ✅ Event schema settled |
| ~~Tue~~ | ~~feat-002, feat-003~~ | ✅ State rebuilds from the log; org schema loads |
| ~~Wed~~ | ~~feat-004, feat-005~~ | ✅ Schema enforced on writes; issue model exists |
| Thu | feat-014, feat-015 | Authorisation: roles in config, membership in the log |
| Fri | feat-006, feat-007 | One API; agents have identity, provenance and proposals |
| Sat | feat-008, feat-009, feat-010 | MCP at parity; boards are queries; metrics without estimates |
| Sun | feat-011, feat-012, feat-013, chore-003 | UI, latency, one-command self-host, dogfooded |

Authorisation was added on Wednesday after review found that R15 — an agent lacking permission
records a proposal — had nothing to define "permission" against. The domain is built before the
API so that `feat-006` exposes a finished model once rather than being revised twice.

Risk is front-loaded deliberately. `feat-001` is first because the event schema is what federation
depends on and the only thing that cannot be changed cheaply later. If it is wrong, Monday is when
that should hurt, not Saturday.

## What gets cut first

A real week slips. Cut in this order, and stop rather than compress everything:

1. **feat-012** (latency tuning) — keep the benchmark, drop the tuning. A demo at 1,000 issues
   proves the product; 10,000 proves the engineering.
2. **feat-010** (flow metrics) — transitions are recorded regardless, so metrics can be derived
   any time afterwards.
3. **feat-009** (query language) — ship fixed list views instead. Boards are the demo-friendly
   part, not the load-bearing part.

Do not cut **feat-004** (schema enforcement), **feat-008** (MCP parity) or **chore-003**
(dogfooding). Those three are the argument. Without enforcement it is another tracker; without
MCP parity the agent claim is marketing; without dogfooding the demo is a slideshow.
