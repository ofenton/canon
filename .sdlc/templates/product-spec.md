# <Product / feature name>

**Status:** draft | in review | agreed
**Owner:** <human>
**Last updated:** <YYYY-MM-DD>

## Problem

<Who has what problem today, and how you know. Evidence, not assertion.>

## Outcome

<What is true when this works. Stated so you could tell whether it happened.>

## Users and jobs

| User | Job to be done | Today | With this |
|---|---|---|---|

## Requirements

Write requirements in EARS so they translate directly into acceptance criteria and tests:
`WHEN <trigger> THE SYSTEM SHALL <observable response>`, `WHILE <state> …`, `IF <condition>
THEN …`, or a bare `THE SYSTEM SHALL <invariant>`. Increments cite these ids in their `Traces:`
field, and `check-traceability.py` will flag any Must requirement no increment covers.

### Must
- **R1:** WHEN <trigger> THE SYSTEM SHALL <observable response>

### Should
- **R2:** WHEN <trigger> THE SYSTEM SHALL <observable response>

### Out of scope
- <Named explicitly, so it does not creep back in>

## Constraints

<Regulatory, platform, integration, data residency, timeline. Anything that removes options.>

## Open questions

| # | Question | Blocks | Owner | Resolution |
|---|---|---|---|---|
| Q1 | | | | |

Unresolved questions marked "blocks: plan" must be closed before `plan-increments` runs.

## Success measures

| Measure | Baseline | Target | How measured |
|---|---|---|---|
