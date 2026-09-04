# 0012 — The one thing Canon writes

**Status:** accepted
**Date:** 2026-09-04
**Amends:** [0009](0009-canon-as-aggregator.md), which said the centre originates nothing.

## Context

Adding a repository from the web interface is a write, and Canon has an asserted invariant that it
has none. `internal/api/api_test.go` enumerates the route table and fails on any method that is not
`GET`, with a comment saying why:

> An aggregator that accepted writes would be a second source of truth for facts a repository
> already owns, which is the thing ADR-0009 exists to prevent.

That reasoning is still correct, and it is about **work**. The list of sources is not work. It says
where Canon looks; it says nothing about what anybody built.

The alternative is worse than it sounds. Editing a file by hand on a deployed instance means either
shelling into something or redeploying to add a repository — which is the friction that stops an
estate being tracked, and R78 exists because of it.

## Decision

**Canon writes exactly one thing: where to look. It authors nothing about the work.**

The invariant is not deleted, it is **narrowed and re-asserted**. `TestNoWriteRoutes` becomes a test
that no route writes *work state* — increments, statuses, transitions, conformance findings — and
that the only write surface in the whole API is the source list. A route that accepts a status
change fails it, exactly as before.

That is a weaker guarantee than "no writes at all", and the honesty required is to say so: the
previous invariant could be checked by looking at HTTP methods, and this one cannot. It needs a test
that knows which routes are allowed to write, which is a list somebody maintains. **A guard with a
maintained allow-list is weaker than a guard without one.** It is accepted because the alternative —
no way to add a repository — fails a requirement.

## The list keeps its history

R79 requires that the list's history survives, for the same reason the ledger's does: a product that
vanishes from the catalogue should be explicable. The store is an S3 object with **versioning
enabled**, so every change is retained and recoverable without Canon implementing anything.

This is deliberately not a database. The list is a few dozen lines and the history is read perhaps
never; a versioned object answers it for a fraction of a cent, and ADR-0010's constraint stands
unchanged — a flat list, no schema, no parser beyond splitting lines.

## Consequences

- The API gains its first non-`GET` routes. They are confined to sources and nothing else.
- Ingest and serving become separate deployables ([ADR-0011](0011-how-canon-is-deployed.md)), so a
  source added through the UI is not visible until the next scheduled refresh. The interface has to
  say that rather than appearing to have done nothing.
- Authentication stops being optional. ADR-0009 concluded identity was minimal because everything
  was a read; a write surface, however narrow, means the Cognito pool in ADR-0011 is load-bearing
  rather than a nicety.

## Alternative considered

**Commit the list to a git repository through the GitHub API**, so the audit trail stays in git and
Canon still writes nothing of its own. Genuinely attractive — it would preserve the original
invariant intact rather than narrowing it.

Rejected for now on cost of machinery: it needs a repository to exist for the purpose, a token with
write scope, and a commit path with its own failure modes, to produce a history that S3 versioning
gives for nothing. **Worth revisiting if Canon is ever used by more than one person**, because at
that point "who added this repository, and why" becomes a question a commit message answers and an
object version does not.
