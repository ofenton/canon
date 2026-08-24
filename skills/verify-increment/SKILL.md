---
name: verify-increment
description: Proves an in-progress increment meets its acceptance criteria, writes a human-reviewable walkthrough of the evidence, and moves it to in-review for Gate 2 sign-off. Use when implementation is finished, or when the user asks to "verify", "check it's done", "run the acceptance criteria" or "is this ready to ship". Never marks work done — that is a human decision.
license: Apache-2.0
allowed-tools: Bash Read Edit Write Grep Glob
---

# Verifying an increment

Verification only means something if the verifier is not the author. **Run this in a fresh
context** — a new session, or a subagent that sees only the diff and the acceptance criteria,
not the reasoning that produced the change. An agent grading its own work grades the intention
rather than the result.

```
Use a subagent to verify inc/<id> against its acceptance criteria in
specs/increment-plan.md. It sees the diff and the criteria only. Report which
criteria hold with evidence, and any change outside Scope.
```

## Calibration

Check what the increment promised. Do not go looking for work it never claimed to do.

A reviewer told to find problems will find some, whether or not they exist, and acting on all
of them produces defensive code, speculative abstraction and tests for cases that cannot happen.
Report only what affects correctness, the stated criteria, or the constitution. Everything else
is at most a note, and more often nothing.

## Workflow

```
Verification progress:
- [ ] 1. Re-read the increment as written, before reading the diff
- [ ] 2. Check each acceptance criterion against real output
- [ ] 3. Check the scope boundary held
- [ ] 4. Run the regression suite
- [ ] 5. Check the constitution
- [ ] 6. Ask what this increment now claims system-wide
- [ ] 7. Write the walkthrough, move to in-review
```

### 1. Re-read first

Read the increment from `specs/increment-plan.md` **before** looking at the diff. Anchoring on
the implementation first makes you verify what was built rather than what was asked for.

### 2. Check each criterion

Criteria are written in EARS (`WHEN … THE SYSTEM SHALL …`), so each one names a trigger and an
observable response. Produce that observation: a command and its actual output, a measurement,
a screenshot. Paste the real output, not a summary of it.

A criterion is met only if you can point at something. "The tests cover this" is not evidence;
the test name and its passing output is. If a criterion cannot be checked, it stays unticked and
you say why — an unverifiable criterion is a planning defect worth reporting back.

### 3. Check the scope boundary

**Run this command and paste its real output.** Writing the file list from memory is not a scope
check — it is a guess that looks like one, and it is how a stray binary or another increment's
work reaches main unnoticed.

```bash
git diff --stat main...HEAD
```

Every changed file should be explicable from the increment's Scope. Anything else is scope creep,
even if it is an improvement. Report it, and propose it as its own increment. Then check the
reverse: does the diff do everything the Scope promised?

### 4. Regression

Run the full suite named in the Test Strategy, not just the new tests. Record the command, the
counts and the duration. If anything fails, verification fails — report it and hand back to
`implement-increment`. Do not fix it yourself; a verifier that patches what it is verifying is
no longer verifying anything.

### 5. Constitution

Check the change against `docs/constitution.md`. Constitution breaches are blocking regardless of
whether the acceptance criteria pass.

### 6. Did this establish something system-wide?

Before writing the walkthrough, ask one question: **does this increment create or change a
property that must now hold everywhere?**

Not "did it work" — that is the acceptance criteria. This is "does the system now claim something
new". A new route, a new write path, a new response shape, a new kind of stored value: each either
obeys an existing invariant or introduces one.

If it introduces one, add a row to the Invariants table in `docs/architecture.md` naming the test
that asserts it, and say so in the evidence. If it *should* obey an existing invariant, check that
it does — the assertion is usually already there and will tell you.

This exists because the worst defects are the ones no increment owns. Every increment can be
correct in isolation while the system as a whole is not: a read route that forgot to authenticate
is nobody's bug until somebody asks whether *every* route authenticates. Verification is the
cheapest place to ask, because it is the moment somebody is already looking at what changed.

**Prefer a structural assertion.** One test that enumerates every route and fails on the one that
forgot is worth more than twenty that each check one route, because the twenty-first route will
not have a test.

If the answer is no, write nothing. Most increments establish nothing system-wide, and inventing
an invariant to have something to say is how the table stops meaning anything.

### 7. Write the walkthrough

Raw logs are not reviewable. Write a **walkthrough** into `specs/increments/<id>-<slug>.md` — a
short account a human can check at a glance, in this shape:

```markdown
## Evidence

**Verified by:** <fresh session / subagent> on <branch>

### WHEN a search query contains a single quote THE SYSTEM SHALL return matching rows without error
`pytest tests/test_search.py::test_quote_in_query -q` → `1 passed in 0.4s`
Query `O'Brien` now returns 3 rows; previously raised `OperationalError`.

### THE SYSTEM SHALL return identical results for the queries in tests/fixtures/queries.json
`pytest tests/test_search.py -q` → `40 passed in 2.1s`

### Regression
`pytest -q` → `312 passed, 2 skipped in 41s`

### Scope
`git diff --stat main...HEAD` → 2 files, +18 −11. Both named in Scope.

### Not verified
None.
```

Screenshots, before/after images and recordings belong here too — for anything user-facing they
are the evidence, and they are faster for a human to check than any amount of prose.

Then fill the ledger's `Evidence:` field with a pointer, set `Status: in-review`, run
`python3 .sdlc/bin/validate-plan.py` and `python3 .sdlc/bin/check-traceability.py`, and commit.

### Present for Gate 2

- Each criterion, with the evidence that it holds
- Anything you could not verify, and why
- Scope deviations found
- What you would watch after release, and the rollback plan as written

Say plainly whether you recommend shipping. Then stop — only a human sets `done`.

## Reporting failures

Report failures with the output that shows them, state what is blocked, and stop. Do not soften
a failing result into "mostly working", and do not fix and re-verify in the same pass.
