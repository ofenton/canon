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
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R13
- **Scope:** Expose every API operation over MCP, with an automated test asserting the MCP tool list covers the full API surface. No other changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL expose an MCP tool for every operation available in the HTTP API
  - [x] WHEN an API operation is added without a corresponding MCP tool THE SYSTEM SHALL fail its test suite
  - [x] WHEN an agent calls an MCP tool THE SYSTEM SHALL apply identical schema validation to the equivalent HTTP call
- **Test Strategy:**
  - Parity test enumerating API routes against MCP tools
  - Negative test: add a route in the test fixture, assert the parity test fails
  - End-to-end: drive a full issue lifecycle over MCP only
- **Dependencies:** feat-006
- **Rollback Plan:** Remove the MCP server; agents fall back to the HTTP API
- **Risk:** Low — mechanical once the API exists, and the parity test is the valuable part
- **Evidence:** see `specs/increments/feat-008-mcp-server-with-parity-test.md`

## feat-009: Queries and boards as saved queries

- **Type:** feature
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R9
- **Scope:** A query language over issues, saved queries, and boards expressed as a saved query plus a grouping key. Boards hold no state. No other changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL express a board as a saved query and a grouping key with no state of its own
  - [x] WHEN an issue stops matching a board's query THE SYSTEM SHALL cease to show it on that board with no separate update
  - [x] WHEN a query references a field not in canon.yaml THE SYSTEM SHALL reject it naming the valid fields
- **Test Strategy:**
  - Board test: change an issue field, assert board membership follows without a write to the board
  - Schema inspection asserting no board state table
  - Query parser table tests
- **Dependencies:** feat-006
- **Rollback Plan:** Ship fixed list views with no saved queries
- **Risk:** Medium — the query language is easy to over-build; keep it small
- **Evidence:** see `specs/increments/feat-009-queries-and-boards-as-saved-queries.md`

## feat-010: Flow metrics without estimation

- **Type:** feature
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R19, R20
- **Scope:** Cycle time and throughput computed from recorded state transitions. No estimate field anywhere. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN an operator requests flow metrics THE SYSTEM SHALL report cycle time and throughput derived from recorded state transitions
  - [x] THE SYSTEM SHALL provide no story point, velocity, estimate or burndown field in the schema, API or UI
  - [x] WHEN canon.yaml defines a field named as an estimate THE SYSTEM SHALL refuse to start
- **Test Strategy:**
  - Metrics test against a fixture with known transition timestamps
  - Assertion test scanning schema, API and UI for estimate-shaped fields
  - Startup-refusal test for an estimate field in canon.yaml
- **Dependencies:** feat-009
- **Rollback Plan:** Remove the metrics endpoint; transitions are still recorded so metrics can be added later
- **Risk:** Low — pure derivation from data already recorded
- **Evidence:** see `specs/increments/feat-010-flow-metrics-without-estimation.md`

## feat-011: Keyboard-first web UI

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R18
- **Scope:** List, detail and create views, embedded in the binary. Every action reachable by keyboard without pointer input. No other changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL make every action available in the UI reachable by keyboard without pointer input
  - [x] WHEN a user presses the create shortcut THE SYSTEM SHALL open a title-only create field focused and ready for input
  - [x] THE SYSTEM SHALL serve the UI from the binary with no separate asset deployment
- **Test Strategy:**
  - Playwright run driving a full issue lifecycle by keyboard only
  - Assert the binary serves the UI with no filesystem assets present
  - Screenshot comparison for the list and detail views
- **Dependencies:** feat-006
- **Rollback Plan:** Serve the API only; the CLI remains usable
- **Risk:** Medium — UI work is the easiest thing to overrun on a deadline
- **Evidence:** see `specs/increments/feat-011-keyboard-first-web-ui.md`

## feat-012: Meet the latency budget at 10,000 issues

- **Type:** perf
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R17
- **Scope:** Benchmark and tune reads against a seeded 10,000-issue dataset until the budget is met. Indexes and query plans only; no model changes. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN any read request is served against a 10,000-issue project THE SYSTEM SHALL respond in under 200ms at p95
  - [x] THE SYSTEM SHALL include a reproducible benchmark in the repository that fails CI if the budget regresses
- **Test Strategy:**
  - Seed 10k issues, measure p95 across every read endpoint
  - Wire the benchmark into CI as a failing gate
  - Profile before and after, record both
- **Dependencies:** feat-009
- **Rollback Plan:** Remove the CI latency gate; correctness is unaffected
- **Risk:** Medium — if the projection shape is wrong this reveals it late, which is why the benchmark is written early
- **Evidence:** see `specs/increments/feat-012-meet-the-latency-budget-at-10000-issues.md`

## feat-013: One-command self-host and single-file backup

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R21, R22
- **Scope:** `canon serve` starts a working instance with no external services. All data in one file that can be copied as a backup. README covering install, run and backup. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN an operator runs `canon serve` THE SYSTEM SHALL start a working instance with no external service dependencies
  - [x] THE SYSTEM SHALL store all data in a single file that can be copied while running to produce a valid backup
  - [x] WHEN a copied data file is restored THE SYSTEM SHALL start with identical state
- **Test Strategy:**
  - Fresh-container test: download the binary, run it, create an issue
  - Backup and restore test asserting state equality
  - Copy the file under concurrent writes and assert the restore is valid
- **Dependencies:** feat-011
- **Rollback Plan:** Document a manual multi-step setup instead
- **Risk:** Low — mostly packaging, but it is the whole self-host story
- **Evidence:** see `specs/increments/feat-013-one-command-self-host-and-single-file-backup.md`

## chore-003: Dogfood: run this project in Canon

- **Type:** chore
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** none
- **Scope:** Import this repository's increment ledger into Canon and track the remaining work there. No product code changes. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN the ledger is imported THE SYSTEM SHALL contain every increment with its status and history
  - [x] THE SYSTEM SHALL be the place this project's work is tracked from the import onward
- **Test Strategy:**
  - Import, then compare issue count and statuses against the ledger
  - Use it for one working day and record what broke
- **Dependencies:** feat-013
- **Rollback Plan:** Continue tracking in specs/increment-plan.md
- **Risk:** Low — read-only import, and the most convincing part of the demo
- **Evidence:** see `specs/increments/chore-003-dogfood-run-this-project-in-canon.md`


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
- **Evidence:** see `specs/increments/docs-001-project-readme.md`

## docs-002: Documentation catch-up

- **Type:** docs
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** none
- **Scope:** Bring the README up to date with queries, boards, metrics and MCP, and document the estimate refusal. Documentation only. No other changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL document every API route currently implemented
  - [x] THE SYSTEM SHALL list under "not built" only things that are genuinely not built
  - [x] WHEN a reader follows any documented example THE SYSTEM SHALL behave as shown
- **Test Strategy:**
  - Cross-check documented routes against `Routes()` mechanically
  - Run every example in the README against a running instance
- **Dependencies:** feat-006
- **Rollback Plan:** Restore the previous README from git history
- **Risk:** Low — documentation only
- **Evidence:** see `specs/increments/docs-002-documentation-catch-up.md`


## feat-017: Hierarchy API: ancestors, subtree and ancestor queries

- **Type:** feature
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R34, R35, R36
- **Scope:** Add `GET /api/issues/{id}/ancestors` and `GET /api/issues/{id}/tree`, and an `ancestor` query key. Read-only additions over the existing parent/child model. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN an operator requests an issue's ancestors THE SYSTEM SHALL return them from the issue to its root, in order
  - [x] WHEN an operator requests an issue's subtree THE SYSTEM SHALL return its descendants to a requested depth
  - [x] WHEN a query names an ancestor THE SYSTEM SHALL return every issue beneath it at any depth
  - [x] WHEN a requested subtree depth would return more than the list limit THE SYSTEM SHALL bound it and report the total
- **Test Strategy:**
  - Build a four-level tree and assert ancestors, subtree at each depth, and ancestor queries
  - Assert a deleted mid-tree node re-parents rather than orphaning, and the subtree reflects it
  - Benchmark the ancestor query against the 10,000-issue dataset
- **Dependencies:** feat-012
- **Rollback Plan:** Remove the two routes and the ancestor query key; the parent/child model is unchanged
- **Risk:** Low — read-only over a model that already holds the data
- **Evidence:** see `specs/increments/feat-017-hierarchy-api.md`

## feat-016: Dependencies with cycle warnings and reverse lookup

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R37, R38, R39, R40
- **Scope:** One directed `depends_on` relation recorded as events, projected with a reverse index. Cycles are recorded and warned about, never refused. Derive `blocked` from whether any dependency is not closed. No other changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL record that one issue depends on another, as a single directed relation with no other relation types
  - [x] WHEN a dependency would create a cycle THE SYSTEM SHALL record it and report a warning naming the cycle, rather than refusing the write
  - [x] WHEN an operator requests an issue's dependencies THE SYSTEM SHALL return both what it depends on and what depends on it
  - [x] THE SYSTEM SHALL derive whether an issue is blocked from whether any issue it depends on is not closed
  - [x] WHEN a query names blocked THE SYSTEM SHALL return issues whose dependencies are not all closed
- **Test Strategy:**
  - Table test over a dependency graph: direct, transitive and cyclic
  - Assert a cycle is stored, warned about by name, and does not refuse the write
  - Assert dependents are found in both directions and survive a projection rebuild
  - Assert blocked is derived, never stored as a field
- **Dependencies:** feat-017
- **Rollback Plan:** Remove the dependency events and routes; issues are unaffected
- **Risk:** Medium — a second graph over the same entities, and the cycle policy is the opposite of the hierarchy's
- **Evidence:** see `specs/increments/feat-016-dependencies-with-cycle-warnings.md`

## feat-018: Issue detail view showing hierarchy and dependencies

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R41
- **Scope:** A keyboard-reachable detail view in the UI showing an issue's fields, ancestors, children, dependencies and dependents, with warnings for cycles and blocked state. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN a user opens an issue in the UI THE SYSTEM SHALL show its fields, its place in the hierarchy and its dependencies, without leaving the keyboard
  - [x] WHEN an issue is blocked THE SYSTEM SHALL say so and name what is blocking it
  - [x] WHEN an issue is part of a dependency cycle THE SYSTEM SHALL show the cycle
  - [x] WHEN a user navigates to a related issue THE SYSTEM SHALL open it without a pointer
- **Test Strategy:**
  - Drive the detail view by keyboard only in the browser test
  - Assert every action in the detail view is in the action registry
  - Assert a blocked issue and a cyclic issue both display their warning
- **Dependencies:** feat-016
- **Rollback Plan:** Revert to the list view; the API still exposes everything
- **Risk:** Medium — the UI is where scope creeps, and this is the first view with real structure
- **Evidence:** see `specs/increments/feat-018-issue-detail-view.md`

## feat-019: Checklist and multi-value fields

- **Type:** feature
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R42, R43, R44
- **Scope:** Add `checklist` and `multi_enum` field types to canon.yaml, a `requires_checklist` state flag, and API support for checking individual items. No other changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL provide a checklist field whose items are individually checkable and countable
  - [x] WHEN a state is marked as requiring a complete checklist THE SYSTEM SHALL refuse entry to it while any item is unchecked
  - [x] THE SYSTEM SHALL provide a field type holding several values from a declared set
  - [x] WHEN a multi-value field is given a value outside its declared set THE SYSTEM SHALL reject the write naming the permitted values
- **Test Strategy:**
  - Table test over checklist operations: add, check, uncheck, count
  - Assert a requires_checklist state is refused with any item unchecked, and permitted when complete
  - Assert multi_enum rejects undeclared values and accepts several declared ones
- **Dependencies:** feat-018
- **Rollback Plan:** Remove the two field types and the state flag; existing schemas are unaffected
- **Risk:** Low — additive to the schema, and schemas without them keep working
- **Evidence:** see `specs/increments/feat-019-checklist-and-multi-value-fields.md`
## feat-020: Typed hierarchy levels

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R45, R46, R47, R48
- **Scope:** Declare permitted nesting as ordered levels of issue types in `canon.yaml`, enforce it on reparent and on delete, and remove the generic cycle check that becomes unreachable once ordering is enforced.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL declare the permitted nesting of issue types as ordered levels in `canon.yaml`, with several types allowed at one level
  - [x] WHEN `canon.yaml` declares a hierarchy THE SYSTEM SHALL require every issue type to appear in exactly one level
  - [x] WHEN a caller sets a parent whose type is not the level immediately above the child's THE SYSTEM SHALL reject the write and name the permitted parent types
  - [x] WHEN deleting an issue would lift a child to a parent the hierarchy does not permit THE SYSTEM SHALL refuse the delete and name the children in the way
  - [x] WHEN a schema declares no hierarchy THE SYSTEM SHALL refuse to set any parent, naming the missing declaration
- **Test Strategy:**
  - Table test over legal and illegal nestings across four levels, including two types sharing a level
  - Assert a schema whose levels omit an issue type refuses to load
  - Assert delete refuses when lifting would break the hierarchy, and names the children
  - Assert the generic cycle path is unreachable and removed, not merely bypassed
- **Dependencies:** feat-016
- **Rollback Plan:** Remove the hierarchy block and restore the generic cycle check
- **Risk:** Medium — changes the meaning of an existing relation, and deletes a guard that has been in place since feat-005
- **Evidence:** see `specs/increments/feat-020-typed-hierarchy-levels.md`


## feat-021: Validate the hierarchy against an existing log

- **Type:** fix
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R49
- **Scope:** Extend `CheckMigration` so a tightened hierarchy is checked against existing nestings, as it already is for removed states. Refuse startup and name the offending issues.
- **Acceptance Criteria:**
  - [x] WHEN a schema change would leave existing issues nested in a way the hierarchy does not permit THE SYSTEM SHALL refuse to apply it and name the offending issues
  - [x] WHEN a schema removes the hierarchy entirely THE SYSTEM SHALL refuse to apply it while any issue has a parent
  - [x] WHEN a schema change is compatible with every existing nesting THE SYSTEM SHALL apply it
- **Test Strategy:**
  - Build a valid tree, then narrow the hierarchy and assert the refusal names the offending pairs
  - Assert removing the hierarchy is refused while parents exist, and permitted once they are cleared
  - Assert widening the hierarchy is always applicable
- **Dependencies:** feat-020
- **Rollback Plan:** Remove the nesting check from CheckMigration; state checking is unaffected
- **Risk:** Low — one more check in a function that already exists for exactly this purpose
- **Evidence:** see `specs/increments/feat-021-validate-the-hierarchy-against-an-existing-log.md`


## feat-022: Render checklists and multi-value fields

- **Type:** feature
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R50, R51
- **Scope:** Render checklists and multi-value fields in the issue detail view, with keyboard toggling and adding of checklist items. UI only. No other changes.
- **Acceptance Criteria:**
  - [x] WHEN an issue carries a checklist THE SYSTEM SHALL show every item, whether it is met, the count met, and allow a user to toggle one without leaving the keyboard
  - [x] WHEN an issue carries a multi-value field THE SYSTEM SHALL show every value
  - [x] WHEN a checklist blocks the state an issue is trying to reach THE SYSTEM SHALL make that visible on the issue
  - [x] WHEN a user adds a checklist item THE SYSTEM SHALL do so without leaving the keyboard
- **Test Strategy:**
  - Drive adding, toggling and untoggling a checklist item by keyboard in the browser test
  - Assert the progress count updates and the item shows who met it
  - Assert every new action appears in the action registry
- **Dependencies:** feat-019
- **Rollback Plan:** Remove the checklist and multi-value sections from the detail view
- **Risk:** Low — rendering over an API that already exists, in a view that already exists


---
- **Evidence:** see `specs/increments/feat-022-render-checklists-and-multi-value-fields.md`

## feat-023: Backdated writes with an explicit timestamp

- **Type:** feature
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R27
- **Scope:** Accept an optional `at` timestamp on write routes, authorised as its own permission, rejected if in the future or before the issue's creation. Records the supplied time as `Event.At` while `Seq` continues to record arrival order. No import tooling, no UI.
- **Acceptance Criteria:**
  - [x] WHEN a caller supplies `at` on a write THE SYSTEM SHALL record that instant as the event time
  - [x] WHEN a caller supplies `at` in the future THE SYSTEM SHALL refuse the write and say so
  - [x] WHEN a caller lacks the backdate permission THE SYSTEM SHALL refuse the write
  - [x] THE SYSTEM SHALL order the log by arrival, not by the supplied time
- **Test Strategy:**
  - Unit: accepted, future-dated, unauthorised, and before-creation cases
  - Replay: a backdated event rebuilds to the same projection
- **Dependencies:** none
- **Rollback Plan:** Ignore the `at` field in the API layer; events already written stay valid
- **Risk:** Medium — an unauthorised backdate would let history be rewritten, so the permission is the increment
- **Evidence:** see `specs/increments/feat-023-backdated-writes-with-an-explicit-timestamp.md`

## feat-024: Create an issue from a repository in one command

- **Type:** feature
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R26
- **Scope:** `canon new` takes a title and creates an issue from the current branch or commit, requiring nothing but a title, and prints the id and the trailer to paste. Records the repository, branch and commit as a link, using feat-025's mechanism rather than schema fields the org may not define. CLI only.
- **Acceptance Criteria:**
  - [x] WHEN a developer runs one command with only a title THE SYSTEM SHALL create an issue and print its id
  - [x] THE SYSTEM SHALL record the branch, repository and commit the command was run in
  - [x] WHEN the command is run outside a git repository THE SYSTEM SHALL still create the issue
- **Test Strategy:**
  - CLI test in a temporary git repository, and in a directory that is not one
- **Dependencies:** feat-025
- **Rollback Plan:** Remove the `new` subcommand; nothing else depends on it
- **Risk:** Low — additive subcommand
- **Evidence:** see `specs/increments/feat-024-create-an-issue-from-a-repository-in-one-command.md`

## feat-025: Link commits to issues, including after the fact

- **Type:** feature
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R27
- **Scope:** Record a commit against an issue with its original author timestamp, via API and `canon link`. Links are events, so a commit links once and the link is history. Reads `Increment:`-style trailers from a supplied range.
- **Acceptance Criteria:**
  - [x] WHEN a commit is supplied after the fact THE SYSTEM SHALL link it and record its original timestamp
  - [x] WHEN the same commit is linked twice THE SYSTEM SHALL record it once
  - [x] THE SYSTEM SHALL list the commits linked to an issue
- **Test Strategy:**
  - Unit: link, duplicate link, unknown issue
  - CLI test over a temporary repository with real commit timestamps
- **Dependencies:** feat-023
- **Rollback Plan:** Remove the link routes; the events remain readable
- **Risk:** Low — new event type, no change to existing ones
- **Evidence:** see `specs/increments/feat-025-link-commits-to-issues-including-after-the-fact.md`

## feat-026: Untracked work as a counted category

- **Type:** feature
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R28, R29
- **Scope:** A traceability report over a commit range giving the proportion carrying no issue reference, with deliberately untracked work recorded as its own category rather than a placeholder id. Exposed as `canon trace`. The planned API route was dropped: the denominator is every commit in a range and the server has no repository to read, so a route could only ever report the tracked set against itself. See the increment record.
- **Acceptance Criteria:**
  - [x] WHEN an operator requests a report over a range THE SYSTEM SHALL give the proportion of commits carrying no issue reference
  - [x] THE SYSTEM SHALL count deliberately untracked commits separately from unexplained ones
  - [x] THE SYSTEM SHALL name the unexplained commits so they can be linked afterwards
- **Test Strategy:**
  - CLI test over a repository with tracked, deliberately untracked and unexplained commits
- **Dependencies:** feat-025
- **Rollback Plan:** Remove the report; it reads existing data and writes nothing
- **Risk:** Low — read-only
- **Evidence:** see `specs/increments/feat-026-untracked-work-as-a-counted-category.md`

## feat-027: Schema usage report

- **Type:** feature
- **Status:** done
- **Tier:** 3 (Medium)
- **Traces:** R25
- **Scope:** Report every field, state and issue type in `canon.yaml` with its usage count and last-used date, so unused configuration is visible. Read-only, over the projection.
- **Acceptance Criteria:**
  - [x] WHEN an admin requests a schema report THE SYSTEM SHALL list every field with its usage count and last-used date
  - [x] THE SYSTEM SHALL show configuration that has never been used
- **Test Strategy:**
  - Unit over a fixture log exercising some fields and not others
- **Dependencies:** none
- **Rollback Plan:** Remove the route and subcommand
- **Risk:** Low — read-only
- **Evidence:** see `specs/increments/feat-027-schema-usage-report.md`

## feat-028: Full-text search

- **Type:** feature
- **Status:** done
- **Tier:** 3 (Medium)
- **Traces:** R23
- **Scope:** Search titles and text fields, returning results within the latency budget at 10,000 issues, reachable from the existing `/` key in the UI.
- **Acceptance Criteria:**
  - [x] WHEN a user submits a query THE SYSTEM SHALL return matching results across titles and text fields
  - [x] THE SYSTEM SHALL return results in under 200ms at p95 with 10,000 issues
- **Test Strategy:**
  - Unit for matching; benchmark at 10,000 issues asserting the budget
- **Dependencies:** none
- **Rollback Plan:** Fall back to the existing field filter
- **Risk:** Low — additive query path
- **Evidence:** see `specs/increments/feat-028-full-text-search.md`

## feat-029: Webhook on every transition

- **Type:** feature
- **Status:** done
- **Tier:** 3 (Medium)
- **Traces:** R24
- **Scope:** Emit a webhook on every state transition, configured in `canon.yaml`, delivered asynchronously with a bounded retry, never blocking the write.
- **Acceptance Criteria:**
  - [x] WHEN an issue transitions THE SYSTEM SHALL deliver a webhook describing the transition
  - [x] WHEN delivery fails THE SYSTEM SHALL retry within a bound and never block the write
- **Test Strategy:**
  - Unit against a test server, including a failing endpoint
- **Dependencies:** none
- **Rollback Plan:** Remove the webhook block from the schema; no endpoint means no delivery
- **Risk:** Medium — an outbound call on the write path, so the async boundary is the increment
- **Evidence:** see `specs/increments/feat-029-webhook-on-every-transition.md`

## fix-001: Resolve issue references case-insensitively

- **Type:** fix
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R27, R28
- **Scope:** Make `canon link` and `canon trace` agree about which reference names which issue, by resolving a commit's reference against the ids Canon actually holds rather than assuming a casing. No change to how references are found.
- **Acceptance Criteria:**
  - [x] WHEN a commit names an issue in different case from the id THE SYSTEM SHALL link it to that issue
  - [x] THE SYSTEM SHALL classify a reference as tracked exactly when linking it would succeed
  - [x] WHEN a reference names no known issue THE SYSTEM SHALL report it as written
- **Test Strategy:**
  - CLI test over a repository whose commits name lower-case ids against upper-case issues and the reverse
  - The existing trace and link suites must still pass unchanged
- **Dependencies:** none
- **Rollback Plan:** Restore the unconditional upper-casing in `issueFrom`
- **Risk:** Low — one resolution step, no new surface
- **Evidence:** see `specs/increments/fix-001-resolve-issue-references-case-insensitively.md`

## fix-003: One naming convention across the API

- **Type:** fix
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R14
- **Scope:** Give the projection's exported types JSON tags so every response uses snake_case, matching the hand-written responses that already do. Update the UI, which is the only client. No change to what is returned, only to what the keys are called.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL name every JSON field in snake_case across every route
  - [x] THE SYSTEM SHALL serve the web UI unchanged in behaviour after the rename
- **Test Strategy:**
  - A test walking every route's response and failing on any key that is not snake_case
  - The keyboard suite must pass unchanged
- **Dependencies:** none
- **Rollback Plan:** Remove the struct tags; the Go field names return
- **Risk:** Medium — a breaking change to response shape, caught by a test that reads every route
- **Evidence:** see `specs/increments/fix-003-one-naming-convention-across-the-api.md`

## fix-002: Imported history carries real timestamps

- **Type:** fix
- **Status:** done
- **Tier:** 3 (Medium)
- **Traces:** R20
- **Scope:** Make `scripts/import-ledger.py` date each write from the increment's commits using feat-023's `?at=`, so imported flow metrics measure when work happened. Keep enough precision in durations for work that takes hours, and render it in units a reader can act on. The API keeps its `*_days` keys and numeric type.
- **Acceptance Criteria:**
  - [x] WHEN history is imported THE SYSTEM SHALL date each transition from the commits that carry the increment
  - [x] WHEN work completes in under a day THE SYSTEM SHALL report a duration greater than zero
  - [x] THE SYSTEM SHALL render a sub-day duration in hours or minutes rather than as 0d
  - [x] WHEN the importing actor may not backdate THE SYSTEM SHALL report that rather than silently landing history at import time
- **Test Strategy:**
  - Unit: durations of minutes and hours survive the rounding
  - Re-import Canon's own ledger and compare the resulting cycle times against the real commit history
- **Dependencies:** none
- **Rollback Plan:** Restore the two-decimal rounding in `days()`
- **Risk:** Low — precision and presentation only
- **Evidence:** see `specs/increments/fix-002-imported-history-carries-real-timestamps.md`

## fix-004: One render wins

- **Type:** fix
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R21
- **Scope:** Make a superseded render stop writing. Every view renders asynchronously, so two navigations in quick succession leave two renders in flight and whichever fetch returns last paints the screen. Guard each write with a generation token. No change to any view's content.
- **Acceptance Criteria:**
  - [x] WHEN a render is superseded before its data arrives THE SYSTEM SHALL discard it rather than paint it
  - [x] WHEN a view is navigated to THE SYSTEM SHALL show that view regardless of how slowly the previous one loads
- **Test Strategy:**
  - Browser test navigating away mid-fetch against a deliberately slow response, repeated so a race would surface
  - The keyboard suite must pass unchanged
- **Dependencies:** none
- **Rollback Plan:** Remove the generation check; the races return
- **Risk:** Low — one guard, no change to what is rendered
- **Evidence:** see `specs/increments/fix-004-one-render-wins.md`

## feat-030: Teams are declared, not invented

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R1, R3
- **Scope:** Declare the organisation's teams in `canon.yaml` and refuse any team not declared, on issues and on membership alike. Membership stays in the event log. A schema change that would orphan issues owned by a removed team is refused, as with states and types.
- **Acceptance Criteria:**
  - [x] WHEN a caller names a team not declared in canon.yaml THE SYSTEM SHALL refuse the write and list the teams that exist
  - [x] WHEN a schema removes a team that issues still belong to THE SYSTEM SHALL refuse to apply it
  - [x] WHERE a schema declares no teams THE SYSTEM SHALL accept any team, so existing instances keep working
- **Test Strategy:**
  - Unit: undeclared team on create, on reparent-by-team, on membership; migration check; the undeclared-schema case
  - The existing suites must pass against a schema that now declares its teams
- **Dependencies:** none
- **Rollback Plan:** Remove the `teams:` block from canon.yaml; validation becomes a no-op again
- **Risk:** Medium — tightens an input that was previously free text, so an existing instance with typo'd teams would fail its migration check, which is the point
- **Evidence:** see `specs/increments/feat-030-teams-are-declared-not-invented.md`

## feat-031: Authenticate the actor

- **Type:** security
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R14, R15
- **Scope:** Prove who is calling. A bearer token per actor, generated by Canon, shown once, stored only as a hash, revocable, and recorded in the log. Verification is a seam so an external identity provider can replace it without touching authorisation. No password login, no sessions, no external service.
- **Acceptance Criteria:**
  - [x] WHEN a caller presents no token THE SYSTEM SHALL refuse the request
  - [x] WHEN a caller presents a token THE SYSTEM SHALL act as the actor that token belongs to, ignoring any claimed identity
  - [x] WHEN a token is revoked THE SYSTEM SHALL refuse it thereafter
  - [x] THE SYSTEM SHALL store no token it could disclose, only a hash
  - [x] WHERE no actor has a token THE SYSTEM SHALL keep working as before, so an existing instance is not locked out by an upgrade
- **Test Strategy:**
  - Unit: issue, verify, revoke, wrong token, unknown token, and that the log holds no token
  - Boundary: every write route refuses an unauthenticated caller
  - The keyboard suite and the MCP suite must pass with authentication on
- **Dependencies:** none
- **Rollback Plan:** Issue no tokens; verification falls back to the trusted-header behaviour
- **Risk:** High — this is the security boundary, and a mistake either locks everyone out or lets everyone in
- **Evidence:** see `specs/increments/feat-031-authenticate-the-actor.md`

## chore-004: Open the repository

- **Type:** chore
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R30
- **Scope:** Make the repository public and give an arriving stranger what they need: a changelog recording the breaking change already shipped, how to contribute, how to report a vulnerability, and branch protection now that a public repository allows it. No code changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL record every change that would break an existing client, including the ones already shipped
  - [x] THE SYSTEM SHALL tell a contributor how the increment workflow works before they open a pull request
  - [x] THE SYSTEM SHALL give a security researcher a private way to report a vulnerability
  - [x] WHEN a pull request targets main THE SYSTEM SHALL require its checks to pass before merging
- **Test Strategy:**
  - Attempt a direct push to main and confirm it is refused
  - Read the repository as an outsider would: clone, build, run, in that order
- **Dependencies:** none
- **Rollback Plan:** Make the repository private again; the added files are harmless either way
- **Risk:** Medium — publishing is irreversible in practice, since anything public may already have been copied
- **Evidence:** see `specs/increments/chore-004-open-the-repository.md`

## docs-003: Populate the architecture map

- **Type:** docs
- **Status:** in-review
- **Tier:** 3 (Medium)
- **Traces:** R31
- **Scope:** Replace the unfilled template stub at `docs/architecture.md` with the system as built, measured from the code rather than recalled. Include the cross-cutting invariants and which test asserts each. No code changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL describe every package, its responsibility and what it may import
  - [x] THE SYSTEM SHALL name, for each architectural invariant, the test that asserts it
  - [x] THE SYSTEM SHALL state the structural gaps it has rather than only what works
- **Test Strategy:**
  - Verify every named test exists, rather than trusting the list
  - Verify the claimed import layering by walking real imports
- **Dependencies:** none
- **Rollback Plan:** Restore the stub; nothing reads this file mechanically
- **Risk:** Low — documentation, though a wrong architecture doc misleads worse than none
- **Evidence:** see `specs/increments/docs-003-populate-the-architecture-map.md`

## feat-032: A familiar visual language

- **Type:** feature
- **Status:** done
- **Tier:** 3 (Medium)
- **Traces:** R21
- **Scope:** Restyle the web UI to the visual conventions people already know from GitHub — its neutral palette, type scale, 6px radii, bordered surfaces and state pills — keeping the existing single-file, no-build, no-dependency structure. Presentation only: no change to what any screen shows or to any keyboard behaviour.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL render in light and dark following the reader's system preference
  - [x] THE SYSTEM SHALL remain one embedded file with no external requests
  - [x] THE SYSTEM SHALL pass every existing keyboard check unchanged
- **Test Strategy:**
  - The keyboard suite must pass with no edits, since nothing about behaviour changes
  - Screenshots of every view in both colour schemes
- **Dependencies:** none
- **Rollback Plan:** Restore the previous style block; the markup is unchanged
- **Risk:** Low — presentation only, and the keyboard suite is the guard
- **Evidence:** see `specs/increments/feat-032-a-familiar-visual-language.md`

## feat-033: Usable with a mouse

- **Type:** feature
- **Status:** abandoned
- **Superseded by:** ui-001 to ui-004. Planned against the issue-tracker UI, which `cut-001` replaced. Its second and third criteria were delivered incidentally — a click selects and a double-click opens, and the browser suite drives both paths — but its first was not: `?`, `r` and `Escape` still have no pointer affordance. That is now `ui-002`.
- **Tier:** 2 (High)
- **Traces:** R21
- **Scope:** Make every action reachable by pointer as well as by keyboard: clickable rows and relationships, visible buttons for the actions currently bound only to keys, and hit targets a person can hit. Keyboard parity is preserved and asserted, and the two paths call the same action registry rather than duplicating logic.
- **Acceptance Criteria:**
  - [ ] THE SYSTEM SHALL perform every action in the action registry from a pointer as well as from the keyboard
  - [ ] WHEN a row is clicked THE SYSTEM SHALL select it, and open it on a second click or on Enter
  - [ ] THE SYSTEM SHALL continue to pass every keyboard check with no mouse events
- **Test Strategy:**
  - A structural test asserting every registry action has a pointer affordance
  - A browser test driving the UI by mouse only, alongside the existing keyboard-only suite
- **Dependencies:** feat-032
- **Rollback Plan:** Remove the pointer handlers; the keyboard paths are untouched
- **Risk:** Medium — two input paths diverging is the failure mode, so both must call one registry
- **Evidence:** superseded before implementation; see the increments that replace it

## feat-034: Search and pagination people can see

- **Type:** feature
- **Status:** abandoned
- **Superseded by:** ui-001 to ui-004. Pagination landed with `cut-001`; the visible search box did not, because `internal/query` was removed and there is no text search to expose. That is now `ui-003`.
- **Tier:** 1 (Critical)
- **Traces:** R23, R21
- **Scope:** A permanent search box rather than a keyboard-only prompt, and pagination controls so a list longer than one page is reachable. One input for both search and filter, because the query language already does both. No per-field filter controls.
- **Acceptance Criteria:**
  - [ ] WHEN a list has more results than one page THE SYSTEM SHALL provide a way to reach the rest
  - [ ] THE SYSTEM SHALL show the search input without requiring a key press to reveal it
  - [ ] WHEN a query is refined THE SYSTEM SHALL return to the first page rather than an empty one
- **Test Strategy:**
  - Browser test paging through a seeded list larger than one page, by mouse and by keyboard
  - Assert the offset resets when the query changes
- **Dependencies:** feat-033
- **Rollback Plan:** Hide the controls; `/` and the API's limit and offset are unchanged
- **Risk:** Medium — an off-by-one in paging shows the wrong rows, which is worse than showing none
- **Evidence:** superseded before implementation; see the increments that replace it

## chore-005: Take the template's own updates, and record the reconciliation

- **Type:** chore
- **Status:** done
- **Tier:** 3 (Medium)
- **Traces:** R31
- **Scope:** Record ADR-0007 (a repository holds one component; a release composes component versions), and bring this repository up to the template's current state — the `design-architecture` skill, `check-architecture.py`, the updated `verify-increment` — and backport `lint-workflows.py`, which was written here and never reached the template. Add ADR-0005 and ADR-0006 recording where work should live and how the template should be distributed. No product code changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL run the same skills and checks as the template it came from
  - [x] THE SYSTEM SHALL record the reconciliation as decisions, with the alternatives that were rejected
  - [x] THE SYSTEM SHALL measure the drift rather than assert it
- **Test Strategy:**
  - Diff every shared file against the template and show the result
  - Every validator passes, including the new one
- **Dependencies:** none
- **Rollback Plan:** Remove the added skill, check and ADRs; nothing depends on them yet
- **Risk:** Low — process and documentation only
- **Evidence:** see `specs/increments/chore-005-sync-the-template.md`

## docs-004: ADR-0009, Canon as aggregator

- **Type:** docs
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R31
- **Scope:** Record the decision to make Canon read-only over repositories that follow the template: what it enforces, what identity remains, what the approval gate is, and what gets deleted. Measured, not estimated. No code changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL state what enforcement means when writes happen elsewhere
  - [x] THE SYSTEM SHALL measure the code affected rather than describing it
  - [x] THE SYSTEM SHALL state what the product gives up, not only what it gains
- **Test Strategy:**
  - Derive increment time from the ledger's git history and compare it against the mechanism in use
  - Count non-test lines per package to size the deletion
- **Dependencies:** none
- **Rollback Plan:** Mark the ADR withdrawn; no code depends on it
- **Risk:** Low — a decision document, though it proposes deleting roughly half the codebase
- **Evidence:** see `specs/increments/docs-004-adr-0009-canon-as-aggregator.md`

## feat-035: Ingest a repository

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R52, R53
- **Scope:** Read a repository that follows the template — clone or fetch, parse `specs/product.md` and `specs/increment-plan.md`, and derive each increment's status history from the commit history of the ledger file. Additive: nothing existing is removed, so main keeps working.
- **Acceptance Criteria:**
  - [x] WHEN given a repository containing `specs/increment-plan.md` THE SYSTEM SHALL ingest every increment without per-repository configuration
  - [x] THE SYSTEM SHALL derive each status transition and its timestamp from the ledger file's commit history rather than approximating it
  - [x] WHEN a repository is ingested twice THE SYSTEM SHALL produce the same result
- **Test Strategy:**
  - Ingest this repository and compare the derived transitions against `git log -p specs/increment-plan.md`
  - Ingest twice, assert the projection fingerprint matches
- **Dependencies:** none
- **Rollback Plan:** Remove the ingest command; nothing else depends on it yet
- **Risk:** Medium — the parser meets other people's markdown, and being wrong quietly is the failure mode
- **Evidence:** see `specs/increments/feat-035-ingest-a-repository.md`

## feat-036: Flow measured from real transitions

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R56
- **Scope:** Feed the existing metrics from ingested transitions instead of authored events, and retire `scripts/import-ledger.py`, whose approximation was measured at roughly thirty times out.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL report cycle and lead time from transitions derived from commit history
  - [x] THE SYSTEM SHALL report no estimate of any kind
  - [x] WHEN two status changes share a commit THE SYSTEM SHALL report them at the same instant rather than inventing an interval
- **Test Strategy:**
  - Compare reported percentiles against the same figures computed directly from `git log`
  - The existing estimation-refusal tests must still pass
- **Dependencies:** feat-035
- **Rollback Plan:** Point metrics back at the projection's authored transitions
- **Risk:** Low — the metrics code is unchanged; only its input changes
- **Evidence:** see `specs/increments/feat-036-flow-measured-from-real-transitions.md`

## feat-037: Conformance, reported not enforced

- **Type:** feature
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R54, R61, R62
- **Scope:** Run the template's own rules across every ingested repository and report what fails, per repository, without refusing anything. A repository that does not conform is reported and skipped, never fatal to the rest.
- **Acceptance Criteria:**
  - [x] WHEN a repository fails a rule THE SYSTEM SHALL name the rule and the increment, and continue with the others
  - [x] WHEN an increment traces to a requirement that does not exist THE SYSTEM SHALL report it against that repository
  - [x] THE SYSTEM SHALL report the proportion of commits carrying no increment reference, per repository
- **Test Strategy:**
  - Fixtures: a conforming repository, one with an illegal status, one with a dangling trace, one with no ledger at all
- **Dependencies:** feat-035
- **Rollback Plan:** Stop reporting conformance; ingest is unaffected
- **Risk:** Low — read-only
- **Evidence:** see `specs/increments/feat-037-conformance-reported-not-enforced.md`

## feat-038: A catalogue of products

- **Type:** feature
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R55, R57, R58
- **Scope:** Discover repositories across an organisation, present each as a product with its purpose taken from `specs/product.md`, and state when each was last ingested so a stale view reads as stale.
- **Acceptance Criteria:**
  - [x] WHEN given an organisation THE SYSTEM SHALL discover repositories containing a ledger and list them as products
  - [x] THE SYSTEM SHALL show each product's purpose from its own spec
  - [x] THE SYSTEM SHALL state when each repository was last ingested
  - [x] THE SYSTEM SHALL answer without cloning anything at request time
- **Test Strategy:**
  - Browser test over several ingested fixtures, by mouse and by keyboard
  - Assert no network call happens during a read
- **Dependencies:** feat-035
- **Rollback Plan:** Serve the single-repository view; ingest is unaffected
- **Risk:** Low — presentation over ingested state
- **Evidence:** see `specs/increments/feat-038-a-catalogue-of-products.md`

## cut-001: Delete the write path

- **Type:** refactor
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R59
- **Scope:** Remove everything that exists to defend writes Canon no longer accepts: authorisation, authentication, the actor registry, proposals, boards, backdating, checklists, dependency and commit-link writes, and the write half of the API and CLI. Reads become open to any authenticated member. Roughly 5,000 lines.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL serve every read to any member of the organisation with no per-team visibility rules
  - [x] THE SYSTEM SHALL expose no route that writes an issue
  - [x] THE SYSTEM SHALL continue to pass every read-path test unchanged
- **Test Strategy:**
  - A structural test asserting no write route exists in the route table
  - The read surface, keyboard suite and MCP parity tests must pass unchanged
- **Dependencies:** feat-035, feat-036, feat-037, feat-038
- **Rollback Plan:** Revert the commit; the deletion is one change and touches nothing that ingest depends on
- **Risk:** High — the largest single change in the project, and the risk is deleting something a read path quietly needed
- **Evidence:** see `specs/increments/cut-001-delete-the-write-path.md`

## feat-039: Read-only agent surface

- **Type:** feature
- **Status:** done
- **Tier:** 2 (High)
- **Traces:** R60, R63
- **Scope:** Restore MCP parity against the reduced route table, and show what is blocked and why from dependencies declared in ingested ledgers.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL offer agents over MCP exactly the routes it offers humans over HTTP
  - [x] WHERE a ledger declares dependencies THE SYSTEM SHALL show what is blocked and by what
- **Test Strategy:**
  - The existing parity test, against the reduced surface
  - Fixture with a declared dependency chain, including a cycle
- **Dependencies:** cut-001
- **Rollback Plan:** Leave the MCP surface as it is
- **Risk:** Low — the parity test is the guard
- **Evidence:** see `specs/increments/feat-039-read-only-agent-surface.md`

## docs-005: Reframe the product spec

- **Type:** docs
- **Status:** done
- **Tier:** 1 (Critical)
- **Traces:** R52
- **Scope:** Rewrite `specs/product.md` for Canon as an aggregator, and plan the increments that deliver it. The requirements delivered under the previous framing are kept, marked superseded, because 43 increments trace to them. No code changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL state the new framing and why it changed, with the evidence that changed it
  - [x] THE SYSTEM SHALL preserve every requirement that a delivered increment traces to
  - [x] THE SYSTEM SHALL state what the reframe gives up, not only what it gains
- **Test Strategy:**
  - `check-traceability.py` must still resolve every trace from every delivered increment
  - Every new Must requirement must be claimed by a planned increment
- **Dependencies:** none
- **Rollback Plan:** Restore the previous `specs/product.md` from git
- **Risk:** Medium — a spec nobody rereads is how a project drifts back to what it was
- **Evidence:** see `specs/increments/docs-005-reframe-the-product-spec.md`

## ui-001: Every view has a URL

- **Type:** feature
- **Status:** approved
- **Tier:** 1 (Critical)
- **Traces:** R64
- **Scope:** Put the view, the selected product, the filters and the page into the URL, and read them back on load. Browser back and forward move between views. A reporting tool whose findings cannot be sent to somebody is much less useful than one whose can.
- **Acceptance Criteria:**
  - [ ] WHEN a view is reached THE SYSTEM SHALL update the URL so that opening it reproduces the view
  - [ ] WHEN the browser back button is used THE SYSTEM SHALL return to the previous view
  - [ ] WHEN a URL naming a product that does not exist is opened THE SYSTEM SHALL say so rather than showing an empty screen
- **Test Strategy:**
  - Browser test: navigate, copy the URL, open it in a fresh page, assert the same view
  - Browser test: back and forward across three views
- **Dependencies:** none
- **Rollback Plan:** Stop writing the URL; the in-memory state still drives every view
- **Risk:** Low — additive to state that already exists
- **Evidence:** _(filled in at verify)_

## ui-002: Pointer parity, and a narrow screen

- **Type:** feature
- **Status:** approved
- **Tier:** 2 (High)
- **Traces:** R65, R67
- **Scope:** Give every registry action a pointer affordance, and make the layout work below 40rem. A structural test asserts the parity so a new action cannot be keyboard-only by omission.
- **Acceptance Criteria:**
  - [ ] THE SYSTEM SHALL perform every action in the registry from a pointer as well as from the keyboard
  - [ ] WHEN the viewport is 400px wide THE SYSTEM SHALL show every column's content without the page scrolling sideways
  - [ ] THE SYSTEM SHALL continue to pass the keyboard-only run with no pointer events
- **Test Strategy:**
  - A structural test pairing each registry action with its affordance
  - Browser test at 400px, asserting `document.body.scrollWidth` does not exceed the viewport
- **Dependencies:** ui-001
- **Rollback Plan:** Remove the added controls; the keyboard paths are untouched
- **Risk:** Low — presentation and affordances only
- **Evidence:** _(filled in at verify)_

## ui-003: Search across every product

- **Type:** feature
- **Status:** approved
- **Tier:** 2 (High)
- **Traces:** R66
- **Scope:** Search increment titles, ids and field values across every ingested product, served from `/api/increments?q=`. One input, no per-field controls: a filter bar with a control per field is the accretion this product refuses.
- **Acceptance Criteria:**
  - [ ] WHEN a person submits a word THE SYSTEM SHALL return matching increments from every product
  - [ ] WHEN a search is refined THE SYSTEM SHALL return to the first page rather than an empty one
  - [ ] THE SYSTEM SHALL match without regard to case
- **Test Strategy:**
  - Unit over ingested fixtures, including matches in a non-title field
  - Browser test: type, assert the list narrows and the URL carries the query
- **Dependencies:** ui-001
- **Rollback Plan:** Remove the parameter; the status and blocked filters are unaffected
- **Risk:** Low — a read filter over data already in memory
- **Evidence:** _(filled in at verify)_

## ui-004: What changed recently

- **Type:** feature
- **Status:** approved
- **Tier:** 2 (High)
- **Traces:** R68, R69
- **Scope:** A view of status changes across every product, most recent first, each naming the commit it came from. This is the screen only an aggregator can show, and it is built from transitions that are already exact.
- **Acceptance Criteria:**
  - [ ] THE SYSTEM SHALL list recent status changes across every product, most recent first
  - [ ] THE SYSTEM SHALL name the commit each change came from
  - [ ] THE SYSTEM SHALL state when the data behind the view was read
- **Test Strategy:**
  - Unit: ordering across two products with interleaved timestamps
  - Browser test asserting the view renders and carries a commit reference
- **Dependencies:** ui-001
- **Rollback Plan:** Remove the route and the screen; nothing else reads them
- **Risk:** Low — a projection over transitions already derived
- **Evidence:** _(filled in at verify)_

## docs-006: Requirements for the interface

- **Type:** docs
- **Status:** in-review
- **Tier:** 2 (High)
- **Traces:** R64
- **Scope:** Give the web interface requirements. The reframed spec has none, so the UI exists and nothing asks it for anything. Replan the two UI increments written against the old product, marking what of them already landed. No code changes.
- **Acceptance Criteria:**
  - [x] THE SYSTEM SHALL state what the interface is for, in requirements an increment can trace to
  - [x] THE SYSTEM SHALL record what the superseded increments delivered rather than deleting them
  - [x] THE SYSTEM SHALL audit the current interface rather than assuming what it does
- **Test Strategy:**
  - Audit the interface: enumerate its registry actions, its pointer affordances, its URL handling and its responsive rules
  - Every new requirement claimed by a planned increment
- **Dependencies:** none
- **Rollback Plan:** Restore the previous spec and plan from git
- **Risk:** Low — documentation and planning
- **Evidence:** see `specs/increments/docs-006-requirements-for-the-interface.md`

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
| ~~Mon~~ | ~~feat-014 – feat-022~~ | ✅ Roles, hierarchy, dependencies, detail view |
| Tue | feat-023, feat-025, feat-024 | Backdating, commits linked after the fact, one-command create |
| Wed | feat-026, feat-027 | Untracked work counted; unused configuration visible |
| Thu | feat-028, feat-029 | Search and webhooks — the first two to cut |

The remaining work is the `NOJIRA` group first (feat-023 – feat-026), because it is the part that
argues something. Search and webhooks are ordinary features and are sequenced last deliberately:
if the week runs out, losing them costs the demo nothing.

Authorisation was added on Wednesday after review found that R15 — an agent lacking permission
records a proposal — had nothing to define "permission" against. The domain is built before the
API so that `feat-006` exposes a finished model once rather than being revised twice.

Risk is front-loaded deliberately. `feat-001` is first because the event schema is what federation
depends on and the only thing that cannot be changed cheaply later. If it is wrong, Monday is when
that should hurt, not Saturday.

`feat-024` was planned before `feat-025` and is built after it. An issue created from a repository
has to record where it came from, and the only place to put a repository and branch was schema
fields the org may not have defined — which R3 rightly refuses. `feat-025`'s commit link is that
place, so the order inverted rather than the design bending.

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
