# docs-006: Requirements for the interface

## Context

`cut-001` rewrote the web interface around products and deleted the one it replaced. Two increments
planned before the reframe — `feat-033` (pointer parity) and `feat-034` (search and pagination) —
described a product that no longer exists.

Replanning them turned up something worse: **the reframed spec has no interface requirements at
all.** R52 to R63 cover ingest, conformance, the catalogue, metrics, identity and MCP. The UI exists,
is tested, and nothing asks it for anything. It could be deleted tomorrow and no requirement would
notice.

## The audit, before the plan

Enumerated rather than recalled:

```
10 actions in the registry:
  ?  j  k  Enter  Escape  r  g p  g w  g f  g c

pointer affordances: 8 — four nav buttons, row click, double-click, two filters, pagination
URL state:           none — no view can be linked
text search:         none — internal/query went with cut-001
responsive rules:    1 @media block, and it is the dark-mode one
```

So of `feat-033`'s three criteria, two were delivered incidentally by the rewrite and one was not:
`?`, `r` and `Escape` have no pointer affordance. Of `feat-034`'s, pagination landed and search did
not — because the query language was deleted and there is nothing to expose.

Both are marked **abandoned** with that recorded, rather than deleted. An increment that was planned,
partly delivered by something else, and then superseded is a more useful record than a gap in the
numbering.

## What the interface is for, now

Canon is agent-first, and the interface is where that claim is tested: whatever an agent can ask
for, a person should be able to see. It is a **reading** tool — Canon authors nothing, so there is
no form, no editing and no undo anywhere in it.

Six requirements, R64 to R69. Two of them are new capabilities rather than tidying:

**R64, every view has a URL.** A reporting tool whose findings cannot be sent to somebody is much
less useful than one whose can, and this is the largest single gap in the current interface. Nothing
in it touches `location` or `history`.

**R68, what changed recently across every product.** The screen only an aggregator can show. Canon
already derives exact transitions with the commit each came from; nothing displays them.

## Evidence

**Verified by:** implementing session, `inc/docs-006-ui-requirements`

### The plan

```
specs/increment-plan.md: ok — 58 increments: 4 approved, 1 in-review, 51 done, 2 abandoned
specs/increment-plan.md: ok — 58 increments trace cleanly
```

| | |
|---|---|
| `ui-001` | Every view has a URL — the gap that makes the rest shareable |
| `ui-002` | Pointer parity, and a narrow screen |
| `ui-003` | Search across every product |
| `ui-004` | What changed recently |

`ui-001` is first because the other three each add state — a filter, a query, a view — that should be
in the URL from the moment it exists rather than retrofitted three times.

### Scope

`git diff --cached --stat main` — run. `specs/product.md`, the ledger, and this record. No code.

### Not verified

**`ui-002`'s parity criterion needs a definition of "action".** `j` and `k` move a selection, which a
pointer does by clicking the row it wants — so a literal reading demands a button nobody would use.
The increment says a structural test pairs each action with an affordance, and what counts as an
affordance for a movement key is a judgement that increment has to make and defend.

**`ui-004` assumes transitions are enough.** A useful activity feed might want the commit's message
or author, which ingest reads but does not keep. That may turn into a change to `ingest.Transition`
rather than a UI increment.

**Nothing is built.** This increment is a spec and a plan.
