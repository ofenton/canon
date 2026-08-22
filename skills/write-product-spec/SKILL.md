---
name: write-product-spec
description: Turns an idea, brief or conversation into a reviewable product spec at specs/product.md, and refines or validates an existing one. Use when the user describes something they want built, says "write a spec", "write a PRD", "capture the requirements", or when planning is blocked because a requirement only exists in chat. Produces requirements and open questions for human agreement — it does not design the implementation.
license: Apache-2.0
allowed-tools: Read Edit Write Grep Glob WebSearch WebFetch
---

# Writing a product spec

The spec says **what and why**. The plan says how much and in what order. Implementation says
how. Keep them separate: a spec that names a database table has decided something a human has
not agreed to yet.

Use [.sdlc/templates/product-spec.md](../../.sdlc/templates/product-spec.md).

## Workflow

```
Spec progress:
- [ ] 1. Gather what is already known
- [ ] 2. Draft problem and outcome
- [ ] 3. Write requirements as testable statements
- [ ] 4. Name what is out of scope
- [ ] 5. Log open questions honestly
- [ ] 6. Validate, then hand to the human
```

### 1. Gather

Read anything that exists: tickets, notes, `docs/constitution.md`, prior specs, and the code if
the feature touches something built. Then ask the user for what only they know — usually the
problem's evidence, the constraints, and who decides.

Ask about four things specifically, because they are the ones that are always missing:
**who has the problem**, **how you would know it was solved**, **what must not change**, and
**what has already been ruled out**.

### 2. Problem and outcome

State the problem as something happening to someone, with evidence. "Users find search slow"
is an assertion; "search p95 is 4.2s and 30% of sessions abandon at the results page" is a
problem. If there is no evidence, write "no evidence yet" rather than inventing plausible
framing — a spec that reads as confident when it is not is worse than an obviously thin one.

The outcome is what is true afterwards, stated so you could tell whether it happened.

### 3. Requirements

Each requirement gets an id (`R1`, `R2`) so increments can cite it. Write them so a test could
exist for each. Split Must from Should honestly — if everything is a Must, priorities have not
been decided and you should say so.

Write what the user needs, not how to deliver it. "Results return in under 500ms at p95" is a
requirement. "Add a Redis cache" is a solution, and belongs in the plan.

### 4. Out of scope

Name explicitly what this is not doing, particularly the adjacent things a reader would assume
were included. This section prevents more rework than any other.

### 5. Open questions

Log every question you could not answer, with who can answer it and what it blocks. Mark the
ones blocking the plan — `plan-increments` will refuse to run until those are closed.

Do not resolve a question by guessing and moving on. An assumption recorded as a question is
recoverable; an assumption buried in a requirement is not. Where you must assume something to
keep drafting, write it in the requirement as "**Assumed:** ..." so it is visible.

### 6. Validate

Check before handing over:

- [ ] Every requirement is testable and has an id
- [ ] Problem has evidence, or says it does not
- [ ] Success measures have a baseline and a target
- [ ] Out of scope is populated
- [ ] No implementation decisions have leaked in
- [ ] Open questions have owners
- [ ] Nothing contradicts the constitution

Then present the spec and **stop**. Ask the user to mark it `agreed` — planning reads only
agreed specs. Refining an existing spec follows the same loop; keep the id numbering stable so
existing increments keep pointing at the right thing.
