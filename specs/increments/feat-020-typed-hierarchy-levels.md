# feat-020: Typed hierarchy levels

## Context

Review found that nothing constrained the hierarchy by type. `Reparent` checked both issues
existed and that no cycle formed — so a task could be the parent of an epic. The hierarchy had
been a generic parent/child tree since feat-005, and a generic tree is not what anyone means by
a hierarchy.

## Design notes

**Nesting is a property of types, not of issues.** The schema declares an ordering — epic,
feature, story, then task or bug — and a child's type must sit at the level below its parent's.
Several types share a level naturally, which is what "a story contains tasks and bugs" needs.

**Cycles become impossible by construction, so the cycle check was deleted.** A child's level is
strictly greater than its parent's, so walking parents upward strictly decreases the level and
must terminate. `wouldCycle` was guarding a case the type rules make unreachable, and leaving it
would have been a check nobody could ever trip — worse than no check, because it implies a risk
that is not there. The read paths in `projection` keep their seen-sets, which guard a different
case: a log written by an older build.

**A schema with no hierarchy permits no nesting at all.** The alternative — treating an
undeclared hierarchy as "anything goes" — invents policy the organisation has not agreed, and
does it silently. Refusing with "add a hierarchy block to permit it" is a better failure.

**Every issue type must be placed in exactly one level.** A type left out would be silently
unnestable: a rule nobody wrote and nobody could find.

**`allow_skipping` is the organisation's decision, not ours.** Some orgs want an epic to hold a
story directly; some do not. Default is strict.

**Delete refuses rather than producing an invalid tree.** Lifting a story's tasks to a feature
breaks the hierarchy. Cascading would destroy work nobody asked to destroy; lifting anyway would
hide the problem. The refusal names the children in the way, so the fix is obvious.

**A title-only create now defaults to the most granular type.** Taking the first declared type
would default to `epic`, which is almost never what someone typing a title and pressing enter
wants. Found by running it, not by reading it.

## Evidence

**Verified by:** implementing session, `inc/feat-020-typed-hierarchy`

### Nesting declared as ordered levels, several types per level

```yaml
hierarchy:
  levels: [[epic], [feature], [story], [task, bug]]
  allow_skipping: false
```

```
F under E   204        E under F   422  an epic is at the top of the hierarchy and cannot have a parent
S under F   204        T under E   422  a task cannot sit under an epic; a task may only sit under story
T under S   204        B under T   422  a bug cannot sit under a task; a bug may only sit under story
B under S   204        S under E   422  a story cannot sit under an epic; a story may only sit under feature
```

```
--- PASS: TestNestingRules                     (10 legal and illegal pairs)
--- PASS: TestRefusalNamesThePermittedParents
--- PASS: TestSkippingWhenAllowed
```

### Every issue type must appear in exactly one level

```
--- PASS: TestEveryTypeMustBePlaced/a_type_left_out       (names the missing type)
--- PASS: TestEveryTypeMustBePlaced/a_type_placed_twice
--- PASS: TestEveryTypeMustBePlaced/an_undefined_type
--- PASS: TestEveryTypeMustBePlaced/an_empty_level
--- PASS: TestNoHierarchyMeansNoNesting
```

### Delete refuses when lifting would break the hierarchy

```
$ curl -X DELETE /api/issues/S
cannot delete S: its children would be lifted under F (feature), which the hierarchy
does not permit for B (bug), T (task). Move or delete them first
```

```
--- PASS: TestDeleteRefusesAnIllegalLift
```

The test then moves the children out and asserts the delete succeeds, and that the children
survive it.

### The cycle check is gone, not bypassed

```
--- PASS: TestHierarchyCyclesAreImpossibleByConstruction
```

Every inversion that would once have formed a cycle — including same-level and self-parenting —
is now refused on type grounds, with the refusal naming types rather than paths. `wouldCycle` is
deleted.

### Scope

`git diff --cached --stat main` — run. The declaration and rules in `schema`, enforcement in
`enforce`, the default-type change in `api`, hierarchies added to both shipped schemas, and the
tests updated to use real types.

The default-type change is outside the stated scope. It is here because adding a hierarchy made
`epic` the first declared type, so a title-only create silently started producing epics — a
regression this increment caused and should therefore fix.

### Not verified

**The UI still shows none of this.** feat-018 now carries three increments of invisible
behaviour: hierarchy, dependencies and typed nesting.

Existing logs written before this change may contain nestings the new rules forbid. Nothing
validates a log against a tightened hierarchy at startup, the way `CheckMigration` does for
states. That is a real gap and a candidate for the next increment.

CI runs on the pull request.
