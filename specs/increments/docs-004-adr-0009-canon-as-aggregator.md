# docs-004: ADR-0009, Canon as aggregator

## Context

ADR-0005 established that 96% of Canon's data is reconstructible from git and left four questions
open. This settles them by going further: the centre originates nothing.

## The finding that changed the design

`AGENTS.md` rule 4 requires every status change to be a commit. So the ledger's own file history is
the transition log — exactly, with no heuristic. Comparing it against the mechanism actually in use:

```
from the ledger's git history — 35 increments
  p50 4.3h · p85 15.4h · p95 17.6h · max 22.3h

scripts/import-ledger.py's approximation
  p50 9m  · p85 1.2h  · p95 9h
```

**Roughly thirty times out.** `import-ledger.py` spread each increment's route across the commits
carrying its trailer, and squashed merges collapsed those to near-simultaneous timestamps. fix-002
already corrected this once — from "every cycle time reads 0d" to an approximation — and the
approximation is still wrong by more than an order of magnitude. The exact answer was in one file's
history the whole time.

This is the third time a measurement in this project has overturned something asserted, and the
second time on this exact metric.

## Evidence

**Verified by:** implementing session, `inc/docs-004-adr-0009`

### Transitions read straight from git

```
46 increments with a transition history in git

feat-025
  09:18:37  (new)     -> approved
  10:26:42  approved  -> in-review
  10:26:48  in-review -> done
```

The limit is stated in the ADR rather than hidden: resolution is bounded by commit granularity, and
`in-progress` collapsed above because two status changes shared a commit. That is exact about what
git recorded.

### The deletion, sized by measurement

12,890 non-test lines today, counted per package and per file. Roughly 5,000–6,000 removable:
`internal/enforce` mostly goes (authz 173, auth 103, proposal 115, registry 92, board 54,
backdate 38, checklist 106, dependency 136, link 79 — all defending writes that no longer happen),
most of `internal/schema` goes because the template is the schema, and `internal/api` and
`cmd/canon` roughly halve.

### Scope

`git diff --cached --stat main` — run. One ADR and this record. No code.

## Not verified

**The ADR is proposed and carries four open questions**, the first of which asks whether the
resulting product is the one wanted at all — a fixed convention aggregated, rather than an
organisation's own schema centrally owned. That is a narrowing, and the ADR argues both sides
rather than assuming the answer.

**The deletion estimate is the author's.** It was measured, but which lines survive is a judgement
made by the person who wrote them, and worth a second opinion before anybody acts on it.

**Nothing has been deleted.** No code changed in this increment.
