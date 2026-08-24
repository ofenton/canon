# feat-019: Checklist and multi-value fields

## Context

Prompted by the question of how to capture business value, KPIs and acceptance criteria. Most of
that already worked — an organisation adds fields to `canon.yaml` — but two things could not be
expressed: several values from a set, and criteria that are individually met or not.

## Design notes

**Acceptance criteria are not text.** A `text` field can hold them and nothing can count them,
filter on them or gate a transition behind them: the information is present and unusable. Items
are individually checkable, so "three of five met" is a count.

**Checklist items are events, not a value.** `checklist.item_added`, `item_checked`,
`item_unchecked`, `item_removed`. Encoding a list inside a string field would have been less code
and would have lost the thing that makes it worth having: the log records *who* met *which*
criterion and *when*. Unchecking is a fact too, not an erasure.

**`requires_checklist` turns criteria into a gate**, alongside `requires_evidence`. A state can
declare which checklists must be complete before anything enters it, and the refusal names what is
outstanding rather than just refusing.

**An empty checklist counts as complete.** Refusing on "no criteria yet" would make the gate
impossible to pass rather than merely strict, and would force people to invent a criterion to
satisfy the tool.

**`multi_enum` is a separate type, not a flag on `enum`.** A schema reader can then see at a
glance whether a field holds one thing or several, which matters more than saving a type name.

**A state may not require a checklist that is not one.** Validated at load, like every other
cross-reference in the schema: `requires_checklist: [priority]` is refused with the field's actual
type in the message.

**One route adds an item and marks one.** `PUT …/checklist/{field}` adds when `checked` is
omitted and marks when it is present. Splitting them would mean a caller has to know whether the
item already exists before choosing a URL.

## Evidence

**Verified by:** implementing session, `inc/feat-019-rich-fields`

### Individually checkable, countable criteria

```
acceptance: 1 of 3 met
  [x] WHEN a query contains a quote THE SYSTEM SHALL return matching rows  (ollie)
  [ ] THE SYSTEM SHALL return identical results for the fixture queries
  [ ] THE SYSTEM SHALL respond in under 200ms at p95
```

```
--- PASS: TestChecklistItemsAreCheckableAndCountable
--- PASS: TestChecklistOperationsAreRefusedOnNonChecklistFields
```

The test asserts the checker's identity is recorded, and that unchecking reduces the count rather
than removing the item.

### A state requiring a complete checklist refuses entry

```
$ curl -X POST /api/issues/S1/transition -d '{"to":"in_review","evidence":"tests pass"}'
refused: state "in_review" requires "acceptance" to be complete; 1 of 3 met,
outstanding: THE SYSTEM SHALL return identical results for the fixture queries; …

after meeting all three: 204
```

```
--- PASS: TestIncompleteChecklistBlocksTheTransition
--- PASS: TestEmptyChecklistDoesNotBlock
--- PASS: TestSchemaRefusesABadChecklistRequirement
```

### Several values from a declared set

```
kpi: conversion, p95_latency

refused: field "kpi" does not permit "vibes"; permitted values are conversion,
p95_latency, churn, cost_per_order
```

```
--- PASS: TestMultiValueFields
```

The test also asserts a refused write leaves the previous value untouched, and that a
single-value `enum` will not accept a multi-value write.

### Scope

`git diff --cached --stat main` — run. Two field types and `requires_checklist` in `schema`, the
projected items in `projection`, the rules in `enforce`, three routes in `api`, MCP descriptions,
and the sample schema extended.

### Not verified

**The UI does not render checklists or multi-value fields.** The detail view shows `Fields`, which
holds neither. A checklist is the one thing on this screen someone would want to tick, so this is
the most obvious next increment rather than a footnote.

Checklist items are matched by their text. Two items with the same wording cannot both exist —
refused on add — and renaming one means removing and re-adding, which loses who met it. An item id
would fix that and was not worth the weight yet.

I mis-patched the MCP maps twice while adding descriptions, once putting body hints into the
description map. Both were caught by the compiler and the parity test, but it is a sign these two
maps being keyed identically is easy to get wrong.

CI runs on the pull request.
