# Canon

**Status:** draft
**Owner:** Oliver Fenton
**Last updated:** 2026-08-22
**Licence intent:** open source (Apache-2.0)

## Problem

Jira was the most criticised tool in the 2025 developer survey — attracting more complaints than
the next four combined. The complaints are real but they are symptoms. The disease is that
**configuration is per-project and unbounded**, so every team's setup diverges, and the
divergence destroys the one thing an org-wide tracker exists to provide: a question you can ask
across teams and get a true answer to.

Measured across hundreds of enterprise Jira Cloud instances:

| | Observed | Actually needed |
|---|---|---|
| Workflows per instance | 90–100 | a handful |
| Permission schemes | 40–100+ | 10–15 |
| Custom fields | 700–800+, **over 50% unused in 12 months** | tens |
| Workflow states | ballooned from 5 to 15 | 5 |
| Spellings of "completed" | 16 | 1 |
| Projects inactive 6+ months but fully configured | ~half | none |

The mechanism is always the same: *incremental decisions without visibility*. Each individual
request is reasonable — one more status, one more field, our team works differently. Nobody
ever sees the aggregate, so nobody ever says no. Fields duplicate under near-identical names
("Customer", "Client", "Account", "Customer Name"). Then reporting collapses, because you
cannot build a "completed work" filter when completed has sixteen spellings, and automation
becomes brittle because nobody is sure which status means ready to deploy.

The downstream symptoms follow: 3–5 second transitions, 47 status options none of which fit,
12 mandatory fields turning a bug report into a five-minute form. A dedicated administrator
becomes necessary, and the tool that was bought for visibility now needs its own specialist to
explain what it is telling you.

Cost compounds it. Jira Standard is $8.15/user/month list, but real all-in cost lands at
$20–30/user/month once Confluence, Guard and Marketplace apps are counted — over $250,000 a year
for a 500-person engineering organisation, before plugins. Atlassian's Maximum Quantity Billing,
introduced in late 2025, charges peak seat count per cycle with no refund for mid-cycle removals.

Linear's answer is to remove configurability: one canonical way to triage, prioritise and ship.
It works — about 30% of teams who try both switch — but only up to roughly 50 engineers and one
process. It has no answer for an organisation that genuinely contains a regulated team, a
support team and a product team, and its automation capability is gated behind higher-priced
plans. Open-source alternatives (Plane, Huly, Taiga, OpenProject) are mostly faithful
reimplementations of Jira or Linear at a lower price. None of them addresses configuration
divergence, and none of them is designed for the fact that a growing share of issues are now
opened, worked and closed by coding agents.

## Outcome

An organisation of any size can run one tracker where **every team shares one configuration,
divergence is impossible by construction rather than by policy**, and both humans and agents
work through the same interface at speed. Self-hosted, free, and small enough to understand.

## Users and jobs

| User | Job to be done | Today | With Canon |
|---|---|---|---|
| Engineer | Record and pick up work without leaving flow | 12 fields, 5 minutes, 3s page loads | Keyboard, one screen, sub-second |
| Coding agent | Open, claim, update and close issues with evidence | Bolted-on MCP over a verbose REST API | First-class actor; same API as the UI |
| Team lead | See real flow for my team | Velocity theatre from gamed estimates | Measured cycle time and throughput |
| Head of engineering | Ask one question across all teams | Impossible — 16 spellings of "done" | One schema, so one answer |
| Admin | Change how the org works | 90 workflows, no one understands them | One reviewed pull request |

## The central design decision

**Configuration is a versioned, org-owned artifact, not a per-project accretion.**

The whole organisation's schema — issue types, states, fields, transitions, permissions — lives
in one file, `canon.yaml`, versioned in git and changed by pull request. A team cannot
unilaterally add a status or a field, because there is nowhere local to add it. They open a PR
against the org schema, someone reviews it, and the change applies everywhere at once.

This is deliberately *not* Linear's answer. The organisation can be as complex as it genuinely
needs to be. It simply has to be complex **deliberately, visibly, in one place, with a reviewer**
— which is precisely the step Jira omits and the omission that produces 800 fields.

Two consequences follow, and they are the product:

- **Drift is structurally impossible**, not discouraged. There is no per-project override to
  drift from.
- **Complexity has a visible price.** The diff shows the aggregate. The 700th field is a line in
  a pull request that a human has to approve, which is the moment Jira never has.

## Requirements

EARS notation — `WHEN <trigger> THE SYSTEM SHALL <observable response>`.

### Must

**Configuration as code**
- **R1:** THE SYSTEM SHALL read the entire organisation's issue schema from a single `canon.yaml`
  at a configured path.
- **R2:** WHEN `canon.yaml` is invalid THE SYSTEM SHALL refuse to start and name the offending
  line.
- **R3:** WHEN an API caller sets a field or state not defined in `canon.yaml` THE SYSTEM SHALL
  reject the write with an error naming the valid values.
- **R4:** THE SYSTEM SHALL provide no runtime interface for adding fields, states or issue types.
- **R5:** WHEN a schema change would orphan existing issues THE SYSTEM SHALL refuse to apply it
  and list the affected issue ids.
- **R6:** WHEN `canon.yaml` changes THE SYSTEM SHALL apply the new schema without data migration
  or downtime for additive changes.

**One entity**
- **R7:** THE SYSTEM SHALL represent all work as a single `Issue` entity with an optional parent.
- **R8:** THE SYSTEM SHALL express epics, stories and sub-tasks purely as parent/child relations,
  with no separate types in the storage model.
- **R9:** THE SYSTEM SHALL express boards as saved queries with a grouping key, holding no state
  of their own.
- **R10:** WHEN an issue is deleted THE SYSTEM SHALL re-parent its children rather than orphan
  or cascade-delete them.

**Agents as first-class actors**
- **R11:** THE SYSTEM SHALL serve one API used identically by the web UI, the CLI and agents.
- **R12:** THE SYSTEM SHALL record on every mutation the actor, whether that actor is a human or
  an agent, and the agent's model identifier where applicable.
- **R13:** THE SYSTEM SHALL expose an MCP server covering every operation the UI can perform.
- **R14:** WHEN an agent transitions an issue to a state marked `requires_evidence` THE SYSTEM
  SHALL reject the transition unless evidence is supplied.
- **R15:** WHEN an agent lacks permission for a transition THE SYSTEM SHALL record the attempt as
  a proposal for human approval rather than silently failing.

**Speed**
- **R16:** WHEN a user creates an issue THE SYSTEM SHALL require no more than a title.
- **R17:** THE SYSTEM SHALL respond to any read request for a project of 10,000 issues in under
  200ms at p95 on commodity hardware.
- **R18:** THE SYSTEM SHALL make every action reachable by keyboard without pointer input.

**Measurement without estimation**
- **R19:** THE SYSTEM SHALL report cycle time and throughput from recorded state transitions.
- **R20:** THE SYSTEM SHALL provide no story point, velocity or estimate field.

**Operability**
- **R21:** WHEN an operator runs one documented command THE SYSTEM SHALL start a working instance
  with no external service dependencies.
- **R22:** THE SYSTEM SHALL store all data in a single file that can be copied as a backup.

### Should

- **R23:** WHEN a user submits a query THE SYSTEM SHALL return full-text results across titles and
  bodies in under 200ms at p95.
- **R24:** THE SYSTEM SHALL emit a webhook on every state transition.
- **R25:** WHEN an admin requests a schema report THE SYSTEM SHALL list every field with its usage
  count and last-used date, so unused configuration is visible.

### Out of scope

Stated explicitly, because refusing these *is* the product:

- **Story points, velocity, burndown.** Estimates get gamed under pressure, are inconsistent
  between people, and distract from outcomes. Cycle time is measured rather than guessed.
- **Per-project workflow customisation.** This is the disease.
- **Documents, chat, video, whiteboards.** Huly's play. Not ours.
- **Gantt charts, time tracking, budgets, resource management.** OpenProject's play. Not ours.
- **A plugin marketplace.** The mechanism by which trackers become unmaintainable.
- **SSO/SAML, multi-org, audit exports.** Needed for enterprise adoption, not for v1.
- **Jira import.** Wanted eventually; not in the first week.
- **Mobile app.** Responsive web only.

## Constraints

- **One week.** Real calendar week, ending Sunday 30 August 2026.
- **Open source from the first commit.** Apache-2.0. No open-core crippleware.
- **Company agnostic.** No IAG-specific concepts anywhere.
- **Self-hostable by one person in one command**, on a $10/month VPS.
- **Small enough to read.** If a contributor cannot understand the data model in an hour, it will
  accrete configuration the same way Jira did.

## Success measures

| Measure | Baseline | Target | How measured |
|---|---|---|---|
| Issue creation, keystroke to saved | Jira ≈5 min (12 fields) | Under 10 seconds | Timed, recorded |
| Read latency, 10k issues | Jira 3–5s transitions | <200ms p95 | Benchmark in repo |
| Ways to define a status | Jira: per project, unbounded | Exactly one, org-wide | Schema inspection |
| Cost for 500 engineers | $250k+/year | Server cost only | Arithmetic |
| Dogfooding | — | This project tracked in Canon by day 6 | The demo itself |
| Agent parity | MCP wraps a subset | 100% of UI operations in MCP | Test asserting coverage |

## Open questions

| # | Question | Blocks | Owner | Resolution |
|---|---|---|---|---|
| Q1 | Name — "Canon" (one canonical config) vs alternatives | naming, repo, domain | Ollie | |
| Q2 | Stack: Python/FastAPI + SQLite, or TypeScript full-stack | plan | Ollie | |
| Q3 | Are issues stored in git too, or only the schema? | plan | Ollie | See note |
| Q4 | Apache-2.0 or AGPL-3.0 | — | Ollie | |
| Q5 | Does an agent need its own identity record, or is a token enough for v1? | — | Ollie | |

**On Q3.** Storing issues as files in git is architecturally attractive and dogfoods ADR-0002,
but it brings merge conflicts on concurrent edits, poor query performance past a few thousand
issues, and no realtime. **Assumed:** schema in git (reviewed, versioned), issues in SQLite
(fast, queryable), with a git-backed export for auditability. Worth arguing with at Gate 1.

## Why this beats the alternatives

| | Jira | Linear | Plane / Huly | Canon |
|---|---|---|---|---|
| Config divergence | The disease | Avoided by removing config | Inherited from whoever they cloned | Structurally impossible |
| Org-wide consistency | Policy, unenforced | Enforced by having no options | Policy, unenforced | Enforced by construction |
| Process diversity | Unlimited, uncontrolled | Not supported | Limited | Supported, but reviewed |
| Agents | MCP over verbose REST | MCP over GraphQL, gated by plan | Minimal | First-class, full parity |
| Estimation theatre | Central | Present | Present | Absent by design |
| Cost at 500 seats | $250k+/yr | Significant | Server only | Server only |

The gap nobody occupies: **an organisation that needs more than one process, but needs all of
them to be the same shape.**
