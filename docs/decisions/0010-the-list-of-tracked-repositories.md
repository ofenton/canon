# 0010 — The list of tracked repositories

**Status:** accepted
**Date:** 2026-08-25
**Amends:** [0009](0009-canon-as-aggregator.md), which chose discovery by artifact and left no place to record where to look.

## Context

ADR-0009 decided that a product is any repository containing `specs/increment-plan.md`, and that
adopting Canon is committing that file rather than registering anywhere. That is still right, and it
is about **what makes a repository a product**. It says nothing about **where Canon looks**, and the
implementation quietly answered that question with the cheapest thing that worked: `Discover(root)`
reads one local directory, one level deep.

That only works because every repository is checked out side by side on one machine. It is the
dogfood, and it has been standing in for the product. Canon is meant to be a single pane of glass
across many product repositories — which requires knowing which repositories those are, and being
able to reach ones that are not on this disk.

R52 has said this since the reframe: *WHEN Canon is given an organisation THE SYSTEM SHALL discover
repositories that contain `specs/increment-plan.md`.* The requirement was never the problem. It has
simply been unmet, and the local scan made it easy not to notice.

## Decision

**Canon reads a list of sources. A source is a place to look, not a repository to register.**

One line per source, `#` for comments, three kinds of entry:

```
# canon.sources
~/code                            a local directory, scanned one level deep
git@github.com:ofenton/orders     one repository, fetched
github:ofenton                    every repository in an organisation that has the artifact
```

The third kind is the one that matters, and it is why this is not a registry. Within an organisation
there is still nothing to register: a new product appears in Canon because it committed a ledger,
exactly as ADR-0009 intended. The list names **hosts and organisations**, and falls back to naming
individual repositories only for those outside them.

### This is input, not configuration

`chore-006` removed the last tracked YAML and added a test that fails if one returns. That test
stands, and this does not contradict it, but the distinction has to be stated precisely or it will
be eroded by the next plausible file.

What `canon.yaml` configured was **how work behaves** — which states exist, who may move between
them, which fields are required. The absence of that is the product: the template fixes it, so there
is nothing to choose and nothing to drift.

A list of sources configures nothing. It changes *what Canon reads*, not *how Canon behaves*: point
it at a different list and every rule, state and field is identical. It is an argument, and it lives
in a file only because arguments that change rarely and matter to everyone belong under review.

So: a flat list, no schema, no parser beyond splitting lines. The moment it acquires a nested key it
has become configuration and this ADR has been violated.

### Canon gains a cache, not a store

A remote source has to be fetched before it can be read, so Canon will write bare clones to a cache
directory. This is a real change to "Canon holds nothing" and should not be waved past.

The distinction that keeps it honest: **a cache is discardable and reproducible; a store is
authoritative.** Deleting Canon's cache loses nothing — the next refresh rebuilds it from the same
repositories. Nothing is ever read from the cache that could not be read from origin, and nothing is
ever written to it that did not come from origin. If a future change makes the cache the only place
some fact exists, that fact has become state and this decision no longer covers it.

## Consequences

- `Discover(root)` stops being the entry point and becomes one source kind among three.
- Canon acquires a network dependency and, with it, per-source failure. One unreachable host must not
  stop the others — the same rule the catalogue already applies to one unparseable ledger.
- Credentials enter the picture for private repositories. Canon should use what git already has
  (SSH agent, credential helper) and read a token from the environment for organisation listing,
  rather than storing anything itself.
- The dogfood keeps working: a local directory is a valid source, so `-products .` becomes
  `-sources` with one line in it.

## Alternatives

**A registry Canon owns, written through its API.** Rejected: Canon accepts no writes, and a mutable
list of products held centrally is exactly the second source of truth ADR-0009 exists to prevent.

**Flags only — `canon serve -repo a -repo b`.** Rejected: not reviewable, not diffable, and
unusable past a handful. The list is the sort of thing that should change by pull request.

**Organisation discovery only, with no file.** Attractive — nothing to maintain at all — but it
cannot span two hosts or reach a repository outside the organisation, and it makes the API token
mandatory for the local case. The file subsumes it: an organisation is one line.
