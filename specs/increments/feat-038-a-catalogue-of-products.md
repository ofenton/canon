# feat-038: A catalogue of products

## Context

feat-035 reads one repository. This makes a set of them: point Canon at a directory and it finds
every product under it, reads each, and answers from what it read.

## Design notes

**Discovery is by artifact, not by registration.** A product is anything with `specs/increment-plan.md`
and a git directory. Adopting Canon is committing a file — there is nowhere to register, no
per-repository configuration, and nothing to keep in step with reality. That is the adoption story
ADR-0009 wanted, and it falls out of searching for the convention rather than for a list.

**One level deep.** A parent directory of checkouts is the shape people have. Walking an entire
filesystem for ledgers would be slow and surprising.

**Reads never touch disk or git.** `Refresh` is a separate act with a recorded time; everything else
answers from memory. `TestReadsDoNotTouchTheSource` deletes the repository after refreshing and
asserts the catalogue still answers — which is the only honest way to prove a read is not secretly
a clone.

**A failing source is kept and reported, never dropped.** A product that silently vanishes from a
catalogue is worse than one that appears with a reason, because nobody notices an absence.

**Refresh replaces rather than accumulates**, or a product deleted from the organisation lingers for
ever.

## What real data found

Pointed at the directory this checkout lives in, Canon discovered **two** products without being
told about either:

```
$ canon catalogue ~/Developer/git/ofenton

2 product(s) under /Users/oliverfenton/Developer/git/ofenton

  Canon                    7 open ·  47 done   1 warning(s)   under an hour ago
                         Work happens in repositories. Agents plan it, build it...
  skynet_prototype       not yet readable
                         /Users/.../skynet_prototype has no commits
```

The second is a repository somebody ran `init.sh` in and never committed — the template's files
present, the history empty. A state I had not thought of, found immediately by pointing the thing at
a real directory rather than at fixtures. It is reported, named, and does not affect the product
that reads cleanly.

## Evidence

**Verified by:** implementing session, `inc/feat-038-catalogue`

### AC: discover repositories containing a ledger and list them as products

Above, and `TestDiscoverFindsProductsByArtifact` — two repositories with a ledger found, a `.git`
directory without one correctly not a product.

### AC: show each product's purpose from its own spec

```json
"name": "Canon",
"purpose": "Work happens in repositories. Agents plan it, build it and record it there — in a
            spec, a ledger and a commit history that is already precise about who changed what
            and when."
```

Taken from the first paragraph under the spec's first section heading, not from the file's opening,
which is a status block.

### AC: state when each repository was last ingested

`refreshed_at` on every product and on the catalogue itself. `TestRefreshTimeIsRecorded` asserts a
catalogue that has never refreshed says so rather than reporting a zero time as if it were real.

### AC: answer without cloning anything at request time

`TestReadsDoNotTouchTheSource` removes the repository from disk after refreshing and asserts both
listing and lookup still answer.

### Over HTTP

```
canon dev listening on :8091
  catalog 2 product(s) discovered under /Users/oliverfenton/Developer/git/ofenton

GET /api/products → both products, one with its error, and a refreshed_at
```

`GET /api/products/{name}` returns one product with its increments, derived histories and
conformance report. Both routes are in the contract test and have MCP descriptions, which the
parity test required before it would pass.

### Tests

Six in `internal/catalogue`, plus two route-contract entries and the naming check extended. Full
suite green across fourteen packages.

### Scope

`git diff --cached --stat main` — run. `internal/catalogue`, two API routes, `canon catalogue`,
`canon serve -products`, MCP descriptions, and tests.

### Not verified

**No remote discovery.** `Discover` reads a local directory. Pointing Canon at a GitHub organisation
— listing its repositories over the API and fetching them — is not built, and R52 says "given an
organisation". This satisfies it for a directory of checkouts and not for an org name, which is a
real shortfall against the requirement rather than a detail.

**No refresh loop.** The catalogue is filled once at startup. There is no schedule, no webhook, and
no way to refresh without restarting, so a long-running instance grows stale silently despite
carrying the timestamp that would reveal it.

**The UI shows none of this.** `/api/products` exists; no screen renders it. The web UI is still the
issue-centric one, and restructuring it belongs after `cut-001` removes what it is built around —
doing it now would mean writing that screen twice.

**The API still serves the old write surface too.** Both models are live on `main` simultaneously,
which is the cost of ingest landing before the deletion. `cut-001` resolves it.
