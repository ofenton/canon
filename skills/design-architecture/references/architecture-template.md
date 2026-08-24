# The architecture document

The shape of `docs/architecture.md`, with what each section is for and the failure it prevents.
Sections that would be empty should be omitted, not filled with a placeholder — an empty heading
promises content and delivers none, which is worse than silence.

---

## Context

One paragraph: what the system does, who talks to it, what it talks to. Include a small diagram
only if it says something the paragraph cannot.

*Prevents:* a reader who has to infer the system's purpose from its package names.

## The one idea

If the design rests on a single decision, say it here in a sentence, then justify it.
"The event log is the system; everything else is a cache" tells a reader more than the component
table does, because every later question resolves against it.

Omit this if there genuinely isn't one. Manufacturing a grand idea for a CRUD service is worse
than admitting it is a CRUD service.

*Prevents:* a reader learning the shape of the system by surprise, one component at a time.

## Components

| Component | Responsibility | Depends on |
|---|---|---|

**State the layering as a rule.** "A component may only depend on components above it in this
table." That is checkable by a script and by a reviewer; a diagram with arrows is neither.

Include measured size (lines, files) for an existing system. It is cheap, and a component three
times the size of its description is telling you something.

*Prevents:* a dependency cycle nobody noticed, and the "where does this belong?" question having
no answer.

## Requirement dependencies

| Requirement | Needs first | Why |
|---|---|---|

Only for a system being designed, and it can be deleted once the plan exists — its job is to
feed `plan-increments`. A dependency is "cannot be built correctly before", not "would be nicer
after".

*Prevents:* work discovered mid-plan, which is the most expensive kind.

## Data

Stores, what lives in each, what the identifiers are, and **where personal data is**. If there is
none, say so explicitly: "no email addresses, names or profiles" is a claim somebody can check,
and an absent section is not.

*Prevents:* a subject-access request answered by reading the source.

## Runtime

How it starts, what it needs, how it is observed, what it refuses to do. Include what a failed
start looks like — that is what an operator meets first.

*Prevents:* an operator learning the deployment model from a Dockerfile.

## Cloud dependencies

| Service | Used for | Replaceable with | Cost to move |
|---|---|---|---|

Fill it in honestly, including when the answer is none — an empty table is a statement, and it
makes the next addition visible as a change. Filling this in is the cheapest insurance against a
lock-in nobody chose.

*Prevents:* discovering a dependency at renewal, or during an outage.

## Invariants

| Invariant | Asserted by |
|---|---|

**The most important section, and the only one a script can check.** Properties that hold across
components, each naming the test that asserts it. Prefer structural assertions — one test that
enumerates every route beats twenty that each check one, because the twenty-first will not have
a test.

An invariant with no assertion is a wish. `check-architecture.py` fails on it, and that failure
is the feature.

*Prevents:* the defect class that survives many increments because it belongs to everything and
therefore to nothing.

## Assumptions

For each: what would falsify it, what it costs to discover late, and the cheapest way to test it
early. Front-load the expensive ones.

*Prevents:* being wrong slowly and expensively rather than quickly and cheaply.

## Seams

Places designed to be replaced, and what replacing one would cost. A seam nobody has named is a
seam nobody will use.

*Prevents:* a rewrite where a substitution would have done.

## Known constraints

Scaling limits, integration contracts, licences, and the structural gaps the system currently
has. **State the gaps.** A document that lists only what works is marketing, and the reader finds
the gaps anyway — later, and with less goodwill.

*Prevents:* somebody adopting the system for a case it cannot serve.
