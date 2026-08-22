# Constitution

Non-negotiable rules for this repository. Agents read this before planning and before
implementing, and verification checks changes against it.

This file changes rarely and only by human decision. If a rule here is blocking good work,
the answer is to change the rule deliberately — not to work around it.

Keep it short. A constitution nobody can hold in their head is not a constitution.

---

## 1. Scope discipline

Work happens in increments recorded in `specs/increment-plan.md`. Nothing gets built that is
not in an approved increment's Scope. Discoveries become new increments, not silent extras.

## 2. Two gates, both human

A human approves the plan before code is written, and approves the ship before release.
Agents may move work to `in-review`; only a human marks it `done`.

## 3. Tests before implementation

Tests come from the increment's Test Strategy and are written before the code, and must be
seen to fail first. Every acceptance criterion maps to a test, or to an explicitly recorded
manual check.

## 4. Evidence over assertion

"It works" is not a result. Test output, measurements and diffs are. Findings, verifications
and status changes carry the evidence that produced them.

## 5. Reversibility

Every increment has a one-line rollback plan. If it cannot be written in one line, the
increment is too big.

## 6. State lives in git

`specs/increment-plan.md` is the single source of truth for work state, and every status change
is a commit ([ADR-0002](decisions/0002-repository-is-the-source-of-truth.md)). Nothing important
lives only in a chat session. If work is also tracked on a board, the board is a projection —
the repository is written first, and where they disagree the repository wins.

## 7. Portability over vendor convenience

Agent configuration is authored once, in `AGENTS.md` and `skills/`, on open standards.
Vendor-specific paths are generated symlinks, never copies — see
[ADR-0001](decisions/0001-vendor-neutral-agent-tooling.md). Enforcement lives in git hooks, not
in any one agent's settings.

Apply the same test to the product itself: prefer a managed service where it saves real work,
but know what replacing it would cost, and record that in `docs/architecture.md`. A dependency
you could not move off in a quarter is an architectural decision and needs an ADR.

---

## Project-specific rules

These exist because this product is a reaction to a specific failure. Breaking them turns Canon
into the thing it was built to replace.

### 8. One schema, no local overrides

The organisation's entire issue schema lives in one `canon.yaml`. There is no per-project,
per-team or per-user override, and no runtime interface for adding a field, state or issue type.
Any change to what is expressible is a change to that file, reviewed as a pull request.

A feature request of the form "let this team configure X for themselves" is a request to
reintroduce configuration drift, and the answer is no.

### 9. One entity

All work is an `Issue` with an optional parent. Epics, stories and sub-tasks are parent/child
relations, not types in the storage model. Boards and sprints are saved queries and time-boxes
over issues; they hold no state of their own.

Adding a second top-level entity requires an ADR arguing why it cannot be an issue.

### 10. No estimation

There is no story point, velocity, burndown or estimate field, and none will be added. Flow is
measured from recorded state transitions — cycle time and throughput — not guessed in advance.

### 11. One API

The web UI, the CLI and agents use the same API. No operation exists in the UI that an agent
cannot perform, and no endpoint exists solely for the UI's convenience. A test asserts parity.

### 12. Provenance on every mutation

Every write records who did it, whether they were human or an agent, and which model if an agent.
State transitions that a schema marks `requires_evidence` are rejected without it.

### 13. Small enough to read

If a new contributor cannot understand the data model in an hour, this project has already lost.
Every dependency added must be justified in the increment that adds it.

### 14. Open source from the first commit

Apache-2.0. No open-core crippleware, no features held back for a paid tier, no telemetry.
