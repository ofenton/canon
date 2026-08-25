# docs-007: Where Canon looks

**Traces:** R70

## What was wrong

`Discover(root)` reads one local directory, one level deep, and takes anything with a
ledger and a `.git`. That has been the entry point since `feat-035`, and it works only
because every repository is checked out side by side on one machine. It is the dogfood
standing in for the product.

R52 has said the right thing since the reframe — *WHEN Canon is given an organisation THE
SYSTEM SHALL discover repositories that contain `specs/increment-plan.md`* — and I have
been recording it as a "known shortfall" for five increments rather than planning it. A
shortfall that is never scheduled is a decision made by omission.

## The decision

[ADR-0010](../../docs/decisions/0010-the-list-of-tracked-repositories.md). Canon reads a
list of **sources**, where a source is a place to look:

```
~/code                            a local directory, scanned one level deep
git@github.com:ofenton/orders     one repository, fetched
github:ofenton                    every repository in an organisation that has the ledger
```

The third kind is why this is a list of places rather than a registry. Inside an
organisation nothing is registered — a product appears because it committed a ledger,
which is what ADR-0009 wanted. Individual repositories are named only when they fall
outside one.

## The distinction that had to be drawn

`chore-006` deleted the last tracked YAML two commits ago and added a test that fails if
one comes back. Adding a sources file immediately afterwards needs more than "this one is
fine".

`canon.yaml` configured **how work behaves** — states, roles, required fields. Its absence
is the product: the template fixes those, so there is nothing to choose and nothing to
drift. A list of sources configures nothing. Point Canon at a different list and every
rule, state and field is identical; only what it reads changes. It is an argument that
lives in a file because arguments that change rarely and matter to everyone belong under
review.

The operational form of that distinction: **a flat list, no schema, no parser beyond
splitting lines.** The moment it acquires a nested key it has become configuration and the
ADR has been violated. That is a line a future increment can be held to.

## What changes about holding nothing

Remote sources must be fetched, so Canon will write bare clones to a cache. That is a real
change to a claim made in the README and enforced by `chore-006`'s test, and the ADR says
so rather than absorbing it quietly.

**A cache is discardable and reproducible; a store is authoritative.** Deleting Canon's
cache loses nothing. R72 states this as a requirement and `feat-041` tests it by deleting
the cache and comparing the catalogue — which is the only version of that claim worth
having.

## Sequencing, and what it displaces

`feat-040`, `feat-041`, `feat-042` go before `ui-002` to `ui-004`. `ui-003` is "search
across every product" and `ui-004` is "what changed recently across every product" — both
are close to meaningless against a catalogue of one, which is what the local scan
produces. The interface increments get their value from there being several products to
aggregate, so the aggregation comes first.

`ui-002` is unaffected either way and simply waits.

## Not decided here

Credentials beyond "use what git already has". `feat-041` will lean on the SSH agent and
git's credential helper, and `feat-042` will read a token from the environment; neither
stores anything. Whether that is enough for a hosted Canon is a question for whoever
deploys one, and it is not answered by this ADR.

## Evidence

- `docs/decisions/0010-the-list-of-tracked-repositories.md` — accepted, amends 0009
- `specs/product.md` — R70, R71, R72
- `specs/increment-plan.md` — feat-040, feat-041, feat-042 planned and sequenced before the
  interface work
