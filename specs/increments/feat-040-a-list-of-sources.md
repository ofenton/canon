# feat-040: A list of the repositories to track

**Traces:** R70, R71

## What changed

`Discover(root)` was the entry point: one local directory, one level deep. It is now one
source kind among several, and Canon reads a **list**.

`internal/source` parses the list and resolves each line. A source is a place, not a
repository to register:

```
~/code                            a local directory, scanned one level deep
/srv/checkouts/orders             one repository
git@github.com:ofenton/orders     fetched — feat-041
github:ofenton                    expanded — feat-042
```

`-sources <file>`, or `-source <line>` repeatedly, or neither — in which case Canon reads
`canon.sources` if it is there and the working directory if it is not. `canon catalogue
~/code` still works, because that is what anyone tries first.

## Decisions worth defending

**Classification is syntactic.** `classify` decides what a line names from its shape
alone, never by asking the filesystem or the network. A line whose meaning depends on when
it was read — a repository on Monday, a directory on Tuesday — is not something anyone can
reason about, and the failure would appear as a product silently changing shape.

The one thing settled by looking is whether a local path is a repository or a directory of
them, and the result is reported back on the `Result`, so a listing can explain itself and
a line never has to declare its own kind (or declare it wrongly).

**`Parse` cannot fail on content.** A line it does not understand becomes a source whose
resolution reports the problem. Refusing to start over one bad line would let a typo in a
list of forty hide the other thirty-nine — the opposite of R71.

**A failed source is an entry, not an absence.** `Catalogue.RefreshFrom` keeps a source
that resolved to nothing, carrying its error, which is the same shape an unreadable
repository already takes. A source that vanishes cannot be reported, and R71 is entirely
about being told.

**An empty directory is an error, quietly.** A directory holding no products is not a
failure of Canon's — it may hold one tomorrow — but silence there reads as "working", so
it says so.

## The one that constrains the future

ADR-0010 says the list must never become configuration, and the operational form of that is
that a nested key must never parse. `TestTheListHasNoSchema` asserts it directly: given

```
sources:
  - ~/code
```

the result must be **two opaque lines**, not a structure. It also fails if `source.go` ever
references `encoding/json`, `yaml`, `toml` or `encoding/xml`. That is the line a future
increment can be held to, rather than a sentence in an ADR nobody re-reads.

## A test that passed for the wrong reason

`TestATildeIsExpanded` first asserted that the error message for `~` mentioned the expanded
path. It does not, deliberately — a report quotes back what was written — so the test was
asserting a property of the error text rather than of the expansion, and it passed while
proving nothing. It now sets `HOME` to a temporary directory, plants a repository under it,
and resolves `~`.

## The package table had drifted

`docs/architecture.md` gives a line count per package. Every one was wrong: `ingest` read
754 against 684, `api` 476 against 319, `ui` 111 against 33. They have been unchecked since
`docs-003` wrote them, and four increments of deletion moved all of them at once.

Measured again, with the command recorded above the table so the next person measures the
same thing. **Nothing checks them, so they will drift again** — a candidate for
`check-architecture.py`, which already parses this table for the invariants, and not
something to bolt on inside a feature increment.

## Not done

Fetching and organisation expansion. Those lines parse and classify correctly today and
report what to do instead, which is why `feat-041` and `feat-042` are additions to
`resolve` rather than changes to anything above it.

## Evidence

- `internal/source` — 8 tests: parsing, classification, resolution, one bad source among
  good ones, the schema constraint
- `internal/catalogue.TestAFailedSourceAppearsRatherThanVanishing` — the requirement at the
  level a person sees it
- `go test ./...` — all nine packages pass; both browser suites pass against `-source .`
- `docs/architecture.md` — 31 invariants, every named test exists
