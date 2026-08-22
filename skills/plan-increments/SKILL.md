---
name: plan-increments
description: Turns a product spec or an assessment's findings into an ordered list of small, independently shippable increments in specs/increment-plan.md. Use after a spec is agreed or an assessment is complete, or when the user asks to "break this down", "plan the work", "create a backlog" or "turn these findings into tasks". Produces planned increments for human approval at Gate 1 — it does not write product code.
license: Apache-2.0
allowed-tools: Bash(python3:*) Bash(git:*) Read Edit Write Grep Glob
---

# Planning increments

You are converting intent into a queue of shippable units. You are not writing code, and you
are not deciding priorities that belong to a human — you are making the work legible enough
that a human can approve or reject it cheaply.

Read [track-increment-state](../track-increment-state/SKILL.md) first for the increment format
and status rules. The ledger is edited through that skill's conventions, not ad hoc.

## Workflow

```
Planning progress:
- [ ] 0. Confirm this work needs planning at all
- [ ] 1. Read the source of truth (spec or assessment)
- [ ] 2. Check for unresolved blocking questions
- [ ] 3. Draft the increment list
- [ ] 4. Order by dependency and tier
- [ ] 5. Write to the ledger and validate
- [ ] 6. Present for Gate 1 approval
```

### 0. Confirm planning is warranted

Check the track table in `AGENTS.md` first. Work you could describe in one sentence belongs on
the Direct track — do it and commit, do not plan it. Planning a one-line fix costs more to
review than the fix costs to make, and it teaches everyone that the process is theatre.

Planning is warranted when the work spans multiple changes, is user-facing, carries regulatory
or contractual weight, or will be picked up by someone who was not in the conversation.

### 1. Read the source

Planning reads from `specs/product.md`, `specs/assessments/*.md`, or both. Read the whole thing.
Also read `docs/constitution.md` — it may forbid approaches you would otherwise propose.

Do not plan from a conversation summary. If the requirement only exists in chat, stop and run
`write-product-spec` first; otherwise the plan encodes something nobody can review later.

### 2. Check blocking questions

If the spec has open questions marked as blocking the plan, list them and stop. Planning around
an unanswered question produces increments that get thrown away.

### 3. Draft the increments

Each increment must be:

- **Independently shippable.** Merged on its own, it leaves the product working. If it only makes
  sense alongside another increment, either merge them or make the dependency explicit.
- **Small enough to roll back in one line.** If you cannot write the Rollback Plan in one line,
  split it.
- **Scoped by naming what does not change.** End Scope with "No other changes." and mean it.
- **Verifiable without judgement.** Write every acceptance criterion in EARS:
  `WHEN <trigger> THE SYSTEM SHALL <observable response>`, or `THE SYSTEM SHALL <invariant>`
  for something unconditional. The form forces a named trigger and a named observation, which
  is what makes the criterion translate straight into a test. "Code is clean" is out;
  `WHEN a query contains a single quote THE SYSTEM SHALL return matching rows without error`
  is in.
- **Traceable.** Fill `Traces:` with the requirement ids (`R1`) or finding ids (`SEC-003`) the
  increment exists to satisfy. `check-traceability.py` uses this to prove no Must requirement
  was quietly dropped.

Sizing heuristic: one increment is roughly one PR one reviewer can hold in their head — often a
day or less of work. Twelve increments that each ship beat three that each stall.

**Split on:** a second user-visible behaviour, a schema migration, a new dependency, a change of
blast radius (one service vs many).
**Do not split on:** file boundaries, or "tests" as their own increment. Tests ship with the
change they test.

### 4. Order

Sort by dependency first, then Tier (1 Critical → 4 Low). State dependencies by id; the
validator will reject cycles and dangling references.

Prefer an order that front-loads risk. If an increment might invalidate the rest of the plan,
it goes first — finding that out on day one is cheap and on day twenty is not.

### 5. Write and validate

```bash
python3 .sdlc/bin/new-increment.py "Parameterize the search query" --type security --tier 2
```

Fill in Scope, Acceptance Criteria, Test Strategy, Rollback Plan and Risk. Then:

```bash
python3 .sdlc/bin/validate-plan.py
python3 .sdlc/bin/check-traceability.py
```

Fix every error and re-run until both pass. Commit the plan as one commit — the plan is a single
proposal, and Gate 1 approves or rejects it as one.

### 6. Present for Gate 1

Summarise for the human in this shape, then **stop and wait**:

- How many increments, and the total shape of the work
- The ordering, and why the first one is first
- Anything you deliberately left out of scope
- Any increment you are unsure about, and what would settle it

All increments stay `planned` until a human sets them to `approved`. Do not begin implementing,
and do not approve your own plan — that is the whole point of the gate.

## Anti-patterns

| Don't | Do |
|---|---|
| One increment called "implement the feature" | One per shippable behaviour |
| "Refactor first, then build" as increment 1 | Refactor inside the increment that needs it |
| Acceptance criteria that restate the scope | EARS criteria naming a trigger and an observation |
| Planning a change you could describe in one sentence | Direct track — just make the change |
| Deps left implicit because "it's obvious" | Deps named by id, so the validator can check them |
| Planning and implementing in one pass | Stop at Gate 1, every time |
