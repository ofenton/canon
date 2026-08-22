# 0004 — Go, Apache-2.0, one binary

**Status:** accepted
**Date:** 2026-08-22

## Context

Canon must be self-hostable by one person in one command on a cheap VPS, respond in under 200ms
at p95 over 10,000 issues, manipulate git objects when the federated transport lands
([ADR-0003](0003-storage-history-and-federation.md)), and be open source from the first commit.

## Decision

**Go, Apache-2.0, distributed as a single static binary with the web UI embedded.**

- **Go** — a static binary with no runtime, no interpreter and no dependency install is what makes
  "one command, no external services" true rather than aspirational. `embed` puts the UI inside
  the binary. `go-git` gives the git plumbing federation will need. It is also what git-bug
  chose for the same problem.
- **Apache-2.0** — permissive, patent-granting, and the licence enterprises adopt without a legal
  review. This forecloses building on git-bug, whose GPLv3 would be viral
  ([ADR-0003](0003-storage-history-and-federation.md) assesses that trade).
- **One binary** — `canon serve` starts everything. No container required, no database to
  provision, no reverse proxy needed to try it.

## Alternatives considered

**Rust** — faster still and what `grite` chose. Rejected on delivery speed for a one-week build;
the performance headroom over Go is irrelevant at this scale.

**Python** — the author's strongest language, and comfortably fast enough for the stated latency
target. Rejected because it loses single-binary distribution, which is a large part of why a
self-hosted tracker gets tried at all. Revisit only if contributor familiarity outweighs that.

**GPLv3 to build on git-bug** — would inherit storage, web UI, GraphQL and GitHub/GitLab bridges.
Rejected: the licence is viral, and the fixed bug/comment/label model has no schema layer to hang
`canon.yaml` from.

## Consequences

**Good.** Trivial distribution and upgrade. Predictable performance without tuning. Cross
compilation to Linux, macOS and Windows from one machine. Git plumbing available in-process.

**Costs.** Go is more verbose than Python for the projection and query code. The author writes it
less often, which is a real cost on a one-week deadline. Frontend work needs a build step to
produce assets for `embed`.

**Constraint that follows.** Every dependency added must be justified in the increment that adds
it (constitution rule 13). A single binary is only an asset while the dependency tree stays small
enough to audit.
