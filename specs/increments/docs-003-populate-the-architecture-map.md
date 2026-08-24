# docs-003: Populate the architecture map

## Context

`docs/architecture.md` was the template's unfilled stub — 37 lines of empty headings — through all
41 increments, and the repository is now public. The template's own text warns that stale
architecture docs actively mislead agents; an empty one in a public repository is that, with a
heading promising content.

## What it says, and why that shape

**Packages in dependency order, and the claim that the order is the architecture.** A reader wanting
to know where something belongs gets an answer, and the answer is checkable.

**An invariants table naming the test that asserts each.** This is the part worth keeping. The three
worst defects in this project were cross-cutting invariants that no single increment owned and no
document stated:

- no read route authenticated — six increments
- any actor could grant itself any role — sixteen increments
- Go field names leaked into JSON — from the first HTTP route

Each survived because it belonged to everything and therefore to nothing. Writing the invariants
down does not prevent that on its own; naming the test next to each is what makes the list
falsifiable rather than aspirational.

**Known constraints stated as constraints.** The CLI writing the log directly, the memory-resident
projection, id collision after deletion, no repair path. Somebody deciding whether to use this
should find them in the architecture, not in an increment file from four days ago.

**An empty cloud-dependency table, deliberately.** The template asks for one. Canon has none, and
saying so with an empty table makes the next addition visible as a change.

## Evidence

**Verified by:** implementing session, `inc/docs-003-architecture`

### Claims checked rather than recalled

Every one of the seventeen tests named in the invariants table was confirmed to exist:

```
(nothing missing)
```

The layering claim — that no package imports one at or below it — was checked by walking real
imports rather than asserted:

```
layering holds: no package imports one at or below it
```

Package line counts, the event-type list, and the dependency list were all read from the source.

### Two claims I had over-stated, corrected

- **"27 keyboard checks"** — I had been repeating that number since feat-022. Running the suite
  reports **33**. It grew and I did not re-count.
- **The licence line** originally said "no third-party code under a copyleft licence" and named
  three modules. Three is what the code *imports*; they pull ten more. The line now says that, and
  notes that `go.mod` marks everything `// indirect` and wants a `go mod tidy` — which is a code
  change and not this increment.

### Not verified

**Nothing keeps this current.** It was accurate the moment it was written and will drift, because
no skill and no check updates it — which is the actual finding, recorded separately.

**The transitive licence set was spot-checked, not audited.** Five of the ten resolved to MIT
locally; the rest are BSD-3 by reputation, not by a licence scan in CI.
