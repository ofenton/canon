# fix-003: One naming convention across the API

## Context

`GET /api/issues` returned `ID`, `Title`, `State`, `Commits`. Every hand-written response in the
same package returned `depends_on`, `linked_by`, `requires_checklist`. The projection's structs had
no JSON tags, so Go field names went straight to the wire wherever a domain type was serialised
directly and snake_case appeared wherever somebody built a map by hand.

That is the kind of inconsistency a client trips over exactly once and then works around forever.
Canon's whole argument is that one org-wide convention beats per-team divergence; an API that
cannot hold one convention across its own routes is not in a position to make it.

## Design notes

**Snake_case wins because most of the surface already used it**, and because it is what the MCP tool
descriptions and the README already documented.

**The test reads responses, not struct tags.** A field reaching the wire through a map, an embedded
type or a hand-built object is checked the same way as a tagged one. Tag inspection would have
missed every hand-written response, which is half the API.

**Keys the organisation chose are exempt**, decided by shape rather than by name — see below.

## The test's own false negative

The first version of the check skipped any key whose *path* contained `fields`, reasoning that a
field an org calls `storyPoints` is its business. That let `/api/schema` through, where `fields` is
a list of Canon's own field *definitions*:

```json
"fields": [{"Name": "title", "Type": "string", "Required": true}]
```

The test passed. The UI broke — `n` reported "this issue type has no checklist field", because it
now read `f.type` while the schema endpoint still said `Type`.

`userNamed` now decides by shape: `fields` as an **object** is user data, `fields` as a **list** is
Canon's own structure. A test that exempts by name exempts the thing it was written to catch.

## Evidence

**Verified by:** implementing session, `inc/fix-003-json-field-names`

### Before and after

```
before:  {"ID":"CANON-1","Title":"one","State":"todo","CreatedAt":"...","LastActor":{"ID":"ollie"}}
after:   {"id":"CANON-1","title":"one","state":"todo","created_at":"...","last_actor":{"id":"ollie"}}
```

### Every route, every key

`TestEveryJSONKeyIsSnakeCase` seeds an issue carrying fields, a checklist, a multi-value, a parent,
a dependency, a transition and a linked commit, then walks sixteen read routes and fails on any key
that is not snake_case. It found three leaks I had not spotted by reading — `issue_types`, `boards`,
and (after the shape fix) the schema's `fields`.

### The UI still works

The keyboard suite is what proves the rename did not quietly break the only client:

```
PASS  a met item shows who met it  — ci
PASS  Space unticks it again
PASS  multi-value fields show every value  — conversion, p95_latency
PASS  no uncaught exceptions

all keyboard checks passed
```

All 27 checks, no mouse events. Full Go suite green across all ten packages.

### Scope

`git diff --cached --stat main` — run. JSON tags on `projection.Issue`, `Commit`, `ChecklistItem`,
`Transition`, `Board`, `schema.Field`, `schema.IssueType` and `event.Actor`; 47 field references
rewritten in the UI; one MCP assertion; the new contract test.

### Not verified

**This is a breaking change for any client outside this repository.** There are none today, and the
version is pre-1.0, so it is the right moment — but nothing in the codebase records that the shape
changed. A changelog is the missing piece, and Canon does not have one.

**`event.Actor` gained JSON tags next to its CBOR tags.** The CBOR encoding is untouched and its
immutability tests still pass, so stored events are unaffected. Worth stating explicitly because
the two encodings now sit on the same struct and a future edit could plausibly assume changing one
changes both.
