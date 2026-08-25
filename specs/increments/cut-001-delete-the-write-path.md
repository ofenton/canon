# cut-001: Delete the write path

## Context

ADR-0009 decided Canon derives and does not author. feat-035 through feat-038 built the read path
alongside the old one, so `main` kept working. This removes what the old model needed.

## What the scope understated

The increment said: *"authorisation, authentication, the actor registry, proposals, boards,
backdating, checklists, dependency and commit-link writes, and the write half of the API and CLI.
Roughly 5,000 lines."*

That was the write path as a list of features. Following it through, the whole model goes: **you
cannot keep a read path over a store nothing writes to.** The event log has no writer, so the
projection over it has no data, so the query language over that has nothing to query, so the API
routes reading it return empty sets.

Removed: `internal/enforce`, `internal/event`, `internal/projection`, `internal/query`,
`internal/schema`, `internal/webhook`, and every write command.

```
14,783 non-test lines  →  3,045
74 files changed, 1,139 insertions, 16,575 deletions
go.mod: 14 modules → 0
```

**Eleven thousand seven hundred lines, not five thousand.** The estimate in ADR-0009 was made by
counting packages that defended writes and assuming the rest survived. It did not.

`go.mod` is now three lines. Canon has no third-party dependencies at all — a claim I wrote into
`docs/architecture.md` before verifying, then checked with `go mod tidy` and found fourteen modules
still listed. The claim is true now because it was tested, not because it was asserted.

## Design notes

**Metrics was rewritten, not deleted.** The percentile, ageing and throughput logic was never wrong;
only its input was. It now reads `ingest.Increment` directly, which is what freed `projection` and
`schema` to go.

**Category moved into metrics as a fixed function.** There is no configurable status vocabulary any
more, so `Category(status)` is a switch over what the template fixes.

**Identity is gone entirely.** Actors, roles, teams, tokens, the `administer` verb, proposals — all
of it existed to protect writes that no longer happen. Reads are open to anyone who can reach the
port, and `TestReadsNeedNoIdentity` asserts it rather than leaving it as an absence.

**The UI was rewritten around products.** The old one was built on issues, boards and a create form,
and calls seventeen routes that no longer exist. Four screens now: products, work across products,
flow, conformance.

## Evidence

**Verified by:** implementing session, `inc/cut-001-delete-write-path`

### AC: no route writes an issue

```go
func TestNoWriteRoutes(t *testing.T) {
	for pattern := range s.Routes() {
		method, path, _ := strings.Cut(pattern, " ")
		if method != http.MethodGet { ... }
```

Structural, over the route table. A write route added later fails this without anybody remembering
to add a case. `TestTheUIOffersNoWrites` and `TestNoToolTakesAWriteBody` hold the other two
surfaces.

### AC: every read served with no per-team rules

`TestReadsNeedNoIdentity` calls four routes with no header, no token and no actor.

### AC: the read path passes unchanged

```
ok  cmd/canon · internal/api · internal/catalogue · internal/conform
ok  internal/ingest · internal/mcp · internal/metrics · internal/ui
```

Twenty-two invariants named in `docs/architecture.md`, every one asserted by a test that exists —
`check-architecture.py` verifies that mechanically.

### The browser suite, both input paths

```
PASS  ? opens keyboard help
PASS  j and k move the selection
PASS  Enter opens the product
PASS  the product shows its increments  — 54 increment(s)
PASS  g w / g f / g c navigate
PASS  no uncaught exceptions (keyboard)

PASS  clicking a nav item navigates
PASS  the status filter narrows the list  — 48 row(s)
PASS  pagination controls are present
PASS  clicking a row selects it
PASS  double-clicking opens the product
PASS  no uncaught exceptions (mouse)
```

Two runs against one server: the first sends no clicks, the second sends no keys. That is the only
way to prove both paths work rather than that one works and the other is present.

The first run failed on `the product shows its increments — 0 increment(s)`, because `j` selected
the second product, which is the one with no commits. The fixture was wrong, not the code — but it
did show that an unreadable product renders its error rather than an empty table.

### Scope

`git diff --cached --stat main` — run. 74 files.

### Not verified

**`internal/query` was deleted although ADR-0009 said to keep it.** The reframed spec requires no
search — R52 to R63 contain no such requirement — and 752 lines nothing calls is what this increment
exists to remove. It is recoverable from git if a requirement appears, but this is a deliberate
departure from the ADR rather than an oversight.

**`internal/webhook` was deleted for the same reason.** ADR-0009 said "kept — notify on ingest
rather than on write". Nothing calls it under the new model, and notification on ingest is unbuilt.

**No performance budget.** `TestReadLatencyBudget` measured reads against 10,000 issues and went
with the API it tested. Nothing now asserts that a catalogue of fifty repositories serves quickly,
and the old budget was one of the more valuable tests in the project.

**The e2e suite is smaller than the one it replaces** — thirteen checks against thirty-three. The
old UI had more screens and more actions; this is not a regression in coverage per feature, but it
is fewer assertions than yesterday and worth saying.
