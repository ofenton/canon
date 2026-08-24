---
name: design-architecture
description: Turns an agreed product spec into a structural design — components, the dependencies between requirements, and the invariants the system will claim — written to docs/architecture.md with decisions recorded as ADRs. Use after a spec is agreed and before plan-increments, or when asked to "design this", "work out the architecture", "what are the components", or when planning keeps discovering dependencies late. Produces structure for human approval; it does not write product code.
license: Apache-2.0
allowed-tools: Bash(python3:*) Bash(git:*) Bash(ls:*) Bash(grep:*) Read Edit Write Grep Glob
---

# Designing the architecture

You are turning *what we intend to build* into *how it fits together*, before anybody writes
an increment. You produce two things a plan cannot produce for itself: the dependencies
**between requirements**, and the properties that must hold **everywhere**.

Both exist because their absence has a signature. Requirements whose dependencies were never
mapped surface as work inserted mid-plan — "we discovered X needs Y first". Properties nobody
wrote down surface as defects that survive many increments, because each increment was correct
in isolation and the gap belonged to all of them.

## When this runs

The **Spec track only** — multi-increment work, anything user-facing and new, anything
regulated. Never for a Direct change, never for a single Increment. A component diagram for a
date-format fix is the ceremony this template exists to prevent.

```
write-product-spec  →  design-architecture  →  plan-increments  →  [GATE 1]
```

If an architecture document already exists, you are **updating it**, not replacing it. Read it
first and preserve what is still true.

## Workflow

```
Design progress:
- [ ] 1. Read the spec and the constitution
- [ ] 2. Identify the dependencies between requirements
- [ ] 3. Name the components and the direction of dependency
- [ ] 4. State the invariants, and how each will be asserted
- [ ] 5. Name the assumptions, and what would falsify each
- [ ] 6. Record the real choices as ADRs
- [ ] 7. Write docs/architecture.md and run the check
```

### 1. Read the spec and the constitution

`specs/product.md` for requirements, `docs/constitution.md` for rules you may not design
around. A constitution rule that makes a requirement impossible is a finding to raise now, not
a problem to discover in increment eleven.

### 2. Identify the dependencies between requirements

**This is the highest-value output.** `plan-increments` orders increments by dependency, but it
can only see dependencies between increments it has already written. Nothing else in the loop
ever asks whether requirement R15 is meaningless until R8 exists.

For each requirement, ask: *what must be true before this can be built at all?* Write the
answer as a table, not prose:

```markdown
| Requirement | Needs first | Why |
|---|---|---|
| R15 an agent lacking permission records a proposal | R14 roles | "permission" has nothing to resolve against |
| R26 create an issue from a repository | R27 commit links | the repository has nowhere to be recorded |
```

Two rules for this table:

- **A dependency is not "would be nicer after".** It is "cannot be built correctly before".
- **If two requirements need each other, say so.** That is a design problem, and the answer is
  usually a third thing both depend on.

### 3. Name the components and the direction of dependency

One table. For each component: what it is responsible for, and what it may depend on.

**State the layering as a rule, not a picture.** "A component may only depend on components
above it in this table" is checkable; a diagram with arrows is not. If the rule has an
exception, the exception is the interesting part of the design and belongs in an ADR.

Keep it to components that exist or will exist. An aspirational box is a lie with a border.

### 4. State the invariants, and how each will be asserted

An invariant is a property that must hold **across** components — the kind no single increment
owns:

- every route authenticates
- a refused write leaves no trace
- the schema cannot change at runtime
- no response exposes a credential

For each one, name what will assert it. **Prefer a structural assertion over a per-instance
one**: a test that enumerates every route and fails on the one that forgot is worth more than
twenty tests that each check one route, because the twenty-first route will not have a test.

```markdown
| Invariant | Asserted by |
|---|---|
| Every API route requires authentication | `TestEveryRouteAuthenticates` — enumerates the route table |
```

A test that does not exist yet is fine here — this is a design. It becomes an increment's job
to make it exist, and `check-architecture.py` will fail until it does. **That failure is the
feature.** An invariant with no assertion is a wish.

### 5. Name the assumptions, and what would falsify each

The point of designing before building is to be wrong cheaply. For each significant assumption,
write what evidence would show it false, and how expensive it would be to discover late.

```markdown
- **Assumption:** an in-memory projection is fast enough for the largest realistic instance.
  **Falsified by:** read latency exceeding budget at N records.
  **Cost if late:** high — the storage layer is what everything else is written against.
  **Cheapest early test:** a benchmark against synthetic data, before the API exists.
```

Front-load the ones that are expensive to reverse. If an assumption is cheap to change later,
say so and move on — that is a reason *not* to spend design time on it.

### 6. Record the real choices as ADRs

A choice belongs in `docs/decisions/` when a competent person could reasonably have chosen
differently. Storage model, language, whether to depend on a managed service: yes. Which
directory a helper lives in: no.

Use the existing ADR format. **The alternatives section is the valuable part** — an ADR that
does not say what was rejected and why is a decision with the reasoning removed.

### 7. Write docs/architecture.md and run the check

```bash
python3 .sdlc/bin/check-architecture.py
```

It verifies that the document is not still the stub, that every invariant names something, and
that every named test and component exists. It will fail on invariants whose tests are not built
yet — expected at design time; note it at Gate 1 rather than deleting the invariant to make the
check pass.

## Boundaries

- **You are not writing code.** Not a prototype, not an interface sketch in a real file. If a
  design cannot be explained without writing the implementation, the design is not finished.
- **You are not planning work.** No increments, no ordering, no estimates. `plan-increments`
  consumes what you produce.
- **You are not deciding scope.** If a requirement looks like it should be cut, say so as a
  finding for the human at Gate 1.
- **Descriptive beats aspirational.** For an existing system, describe what is there and mark
  intended changes explicitly. A document that mixes the two teaches an agent that the code is
  wrong.

## Failure modes

**The diagram nobody can act on.** Boxes and arrows with no dependency rule, no invariants and
no assumptions. It looks like architecture and decides nothing. The test: could someone use
this to reject a pull request? If not, it is decoration.

**Designing every component to the same depth.** Depth should follow risk. The storage model
deserves an ADR and a falsification test; the CLI argument parsing does not.

**Inventing structure for a small system.** Three components with one dependency each is a
complete architecture if that is what the system is. Padding it produces the bloat this template
argues against.

**Writing invariants nothing will ever assert.** Aspiration in a table is worse than aspiration
in prose, because a table looks checked.

## Reference

- [architecture-template.md](references/architecture-template.md) — the document's shape, with
  each section's purpose and the failure mode it prevents.
