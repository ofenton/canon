# feat-017: Hierarchy API — ancestors, subtree and ancestor queries

## Context

feat-005 built the hierarchy and enforced it — cycles refused, delete lifts children — and then
nothing surfaced it. The API offered direct children and nothing else; the UI mentioned parents
zero times. This is the read side of a model that has existed since Wednesday.

## Design notes

**Subtrees come back depth-first, not sorted by id.** Sorted order groups siblings but separates
children from their parents, which makes the one thing a caller wants to do with a subtree —
render it indented — unnecessarily hard. Depth-first with siblings in id order is both directly
renderable and deterministic.

This was found by rendering the output: the ASCII tree drew `SUB-1` and `SUB-2` under the wrong
parent because the flat list was alphabetical. The data was right and the shape was unusable,
which is the kind of thing only looking at it catches.

**Ancestors and descendants both carry a seen-set** even though `Reparent` refuses cycles. A log
written by an older build could contain one, and an infinite loop in a read path is worse than a
wrong answer. `TestCyclicParentsDoNotSpin` writes a cycle directly to the log, bypassing the
enforcer, and asserts both walks terminate.

**`ancestor=` is resolved in `Filter`, not `Match`.** An issue does not know its own lineage, so
matching it per-issue would mean walking parents for every candidate. Resolving the subtree once
against the projection is both simpler and cheaper.

**Subtrees are paginated like any other list.** A subtree can exceed a page, and truncating
without saying so is the same lie feat-012 removed from the issue list.

## Evidence

**Verified by:** implementing session, `inc/feat-017-hierarchy-api`

### Ancestors returned from the issue to its root, in order

```
$ curl /api/issues/SUB-1/ancestors
STORY-A → EPIC   depth 2
```

```
--- PASS: TestAncestors                  (5 cases including roots and orphans)
--- PASS: TestCyclicParentsDoNotSpin
```

### A subtree returned to a requested depth

```
$ curl /api/issues/EPIC/tree              $ curl /api/issues/EPIC/tree?depth=1
EPIC                                      EPIC
   └─ STORY-A                                └─ STORY-A
      └─ SUB-1                                └─ STORY-B
      └─ SUB-2
   └─ STORY-B
```

Rendered directly from the API's order, with no client-side sorting.

```
--- PASS: TestDescendantsToDepth         (8 cases)
--- PASS: TestSubtreeIsDepthFirst
--- PASS: TestSubtreeFollowsADelete
```

`TestSubtreeFollowsADelete` deletes a mid-tree node and asserts the subtree and the ancestor
chain both reflect the lift immediately.

### A query naming an ancestor returns everything beneath it at any depth

```
q=ancestor=STORY-A   →  SUB-1, SUB-2
q=ancestor=EPIC      →  STORY-A, STORY-B, SUB-1, SUB-2
```

```
--- PASS: TestAncestorQuery              (6 cases, including negation and combination)
```

`!ancestor=EPIC` returns everything outside the subtree, and `ancestor=EPIC state=todo` combines
with ordinary terms.

### Latency

Measured against the 10,000-issue dataset, alongside the existing budget:

```
  ancestors of one issue     p50     0.0ms   p95     0.0ms   ok
  subtree of one issue       p50     0.1ms   p95     0.1ms   ok
  query by ancestor          p50     1.0ms   p95     1.0ms   ok
```

### Scope

`git diff --cached --stat main` — run. Two routes in `api`, the walks in `projection`, the query
key in `query`, MCP descriptions, and benchmark entries.

### Not verified

**The UI still shows no hierarchy at all.** That is feat-018, and until it lands this increment is
only reachable through the API and MCP.

`ancestor=` scopes to a subtree but there is no `descendant=` for "everything above this". Nobody
has asked for it, and adding query keys speculatively is how a query language becomes JQL.

CI runs on the pull request.
