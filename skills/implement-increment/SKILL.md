---
name: implement-increment
description: Builds exactly one approved increment from specs/increment-plan.md — opens the branch, writes tests first, implements the scope, and stops. Use when the user says "implement sec-001", "build the next increment", "start the work", or after a plan is approved at Gate 1. Refuses to work on increments that are not approved, and refuses to change anything outside the increment's Scope.
license: Apache-2.0
allowed-tools: Bash Read Edit Write Grep Glob
---

# Implementing an increment

You build one increment. Not the next one as well, not the tidy-up you noticed on the way.
Scope discipline is the entire value of this workflow — an agent that quietly does more than
it was asked destroys the reviewability that made the plan worth writing.

## Preconditions

Stop and say so if any of these fail:

- The increment exists in `specs/increment-plan.md`
- Its `Status:` is `approved` (not `planned` — that means Gate 1 has not happened)
- All its `Dependencies:` are `done`
- No other increment is `in-progress`
- The working tree is clean

## Workflow

```
Implementation progress:
- [ ] 1. Read the increment and its detail file
- [ ] 2. Open the branch, set status in-progress
- [ ] 3. Write the failing tests
- [ ] 4. Implement the scope
- [ ] 5. Tick acceptance criteria as they pass
- [ ] 6. Hand to verify-increment
```

### 1. Read

Read the increment in the ledger and `specs/increments/<id>-<slug>.md`. Read
`docs/constitution.md` for the rules you must not break. Read the code you are about to change
before changing it.

Restate the Scope and Acceptance Criteria back in one short paragraph before you start. If your
restatement is longer than the increment, the increment is underspecified — go back to
`plan-increments` rather than inventing the missing detail.

### 2. Branch and claim

```bash
git checkout -b inc/<id>-<slug>
```

Then set `Status: in-progress` in the ledger, validate, and commit — see
[track-increment-state](../track-increment-state/SKILL.md). Claiming the increment in git is
what stops a second agent or session picking up the same work.

### 3. Tests first

Write the tests from the increment's Test Strategy **before** the implementation, and run them
to confirm they fail for the right reason. A test that passes before you have written any code
is testing nothing.

Each acceptance criterion should map to at least one test. If a criterion cannot be expressed
as a test, say so explicitly and record how it will be checked instead — by hand, by screenshot,
by measurement. Do not silently leave it unverified.

### 4. Implement

Match the surrounding code: its naming, its error handling, its comment density, its idioms.
A change that reads as if it were always there is easier to review than a change that is
individually elegant.

Commit in small steps with the trailer:

```bash
git commit -m "sec-001: parameterize search query" -m "Increment: sec-001"
```

**When you find something outside scope** — a second bug, a missing test, an ugly abstraction —
do not fix it. Note it, finish the increment, and raise it as a new increment afterwards. The
one exception is a change without which the increment cannot work at all; make it, and call it
out explicitly in the increment's detail file under Design notes.

### 5. Tick criteria

As each acceptance criterion starts passing, tick it in the ledger. Record decisions and
rejected approaches in the detail file's Design notes while they are fresh — that is the
context a reviewer needs and the thing that is always lost otherwise.

### 6. Hand over

When every criterion is ticked and the tests pass, run `verify-increment`. Do not set the
status to `done` yourself; verification is a separate step for the same reason you do not
review your own PR.

## Scope pressure

The user may ask mid-increment for something not in scope. That is fine and normal — the answer
is not "no", it is "yes, as its own increment". Say what you will finish first, capture the new
ask, and carry on. If they want it now instead, abandon or pause the current increment
explicitly in the ledger rather than blending the two.
