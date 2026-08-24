# feat-022: Render checklists and multi-value fields

## Context

feat-019 made acceptance criteria data. The detail view showed `Fields`, which holds neither
checklists nor multi-values, so the one thing on that screen someone would want to tick was
invisible.

## Design notes

**Checklists come before the relationships.** On a story with acceptance criteria they are the
work itself, and someone opening the issue is usually there to see or change them. Parent,
children and dependencies sit below.

**The heading carries the count and what it blocks.** `acceptance 1 of 3 met · blocks in_review`
tells you the state of the work and the consequence in one line, without opening anything. The
count turns amber only when the checklist is both incomplete and gating a state — an incomplete
checklist that gates nothing is not a problem.

**Space toggles, and that is the whole interaction.** A criterion is met or it is not; saying so
should cost one keystroke. Items are focusable and sit in the same `j`/`k` sequence as the
relationships, so nothing needs a different way of moving.

**An empty checklist still shows.** An issue whose type declares a checklist but has no items yet
has no key in `Checklists`, so the field is resolved from the schema and rendered with "no criteria
yet — n to add one". Otherwise the feature is invisible exactly when someone needs to start using
it.

**Multi-values render as tags, not a comma-joined string.** They are a set, and a set of one long
value is unreadable run together.

## Evidence

**Verified by:** implementing session, `inc/feat-022-render-checklists`

### Every item, whether it is met, the count, and toggling by keyboard

```
S1
Reindex on write
Blocked — waiting on T1

DETAIL       Type story · State todo · Team platform · priority p1

ACCEPTANCE 1 OF 3 MET · BLOCKS IN_REVIEW
  [x] WHEN a query contains a quote THE SYSTEM SHALL return matching rows   ollie
  [ ] THE SYSTEM SHALL return identical results for the fixture queries
  [ ] THE SYSTEM SHALL respond in under 200ms at p95

KPI          conversion  p95_latency
PARENT       no parent
CHILDREN     T1
WAITS ON     T1

Esc back · Space tick · n new criterion · p parent · d dependency · t transition · Enter follow
```

Driven by Playwright with no mouse events:

```
PASS  n adds a checklist item by keyboard
PASS  the checklist shows how many are met   — 0 of 2 met · blocks in_review
PASS  the checklist says what it blocks      — 0 of 2 met · blocks in_review
PASS  Space ticks the selected item          — 1 of 2 met · blocks in_review
PASS  a met item shows who met it            — you
PASS  Space unticks it again
PASS  multi-value fields show every value    — conversion, p95_latency
```

All 27 keyboard checks pass, including the 20 from earlier increments.

### Scope

`git diff --cached --stat main` — run. The rendering and two actions in `internal/ui`,
`requires_checklist` added to the schema endpoint so the UI can say what a checklist gates, and
the browser test extended.

### Not verified

Removing a checklist item has an API route and no key binding. Adding and ticking are the common
operations; removing is rare enough that leaving it to the API for now seemed better than another
key on an already busy screen.

A single checklist field per issue type is assumed by the `n` shortcut, which adds to the first
one it finds. A type with two checklists would need the prompt to ask which — noted rather than
built, since no schema has two.

My first browser run failed because I seeded the story before the test rather than inside it,
which changed the row order and broke the `j`/`k` assertions from feat-011. The fixture is now
created at the point it is needed.

CI first failed on this increment: the new assertion compared the "met by" name against the
literal `you` while every other assertion used the actor passed on the command line, so it
passed locally and failed under CI's `ci` actor. This is the same mistake feat-011 already had
fixed once, in the same file. Reproduced by running the test against a local instance
bootstrapped with `-actor ci`, which now passes; CI is green on the pull request.
