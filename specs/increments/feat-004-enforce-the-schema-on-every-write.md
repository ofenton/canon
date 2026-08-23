# feat-004: Enforce the schema on every write

## Context

The product's central claim. feat-003 loads the schema; this makes it binding.

## Design notes

**`enforce` is the only path to the log.** No other package appends. Validation that could be
bypassed is documentation, not enforcement.

**A rejected write appends nothing.** Verified by counting the log before and after three
rejections. Validation that still writes would be worse than none: the log would contain states
the schema forbids, and every projection built from it would be wrong.

**Errors name what the caller should have done.** `permitted transitions from "todo" are
abandoned, in_progress` rather than "invalid transition". An agent can act on the first and only
retry blindly on the second, and this is the surface agents will hit constantly.

**No runtime schema mutation is asserted against the source**, not by inspection. A test parses
every non-test file in `enforce`, `schema` and `cmd/canon` and fails if a function named
`AddField`, `AddState`, `AddIssueType` or similar ever appears. Otherwise the guarantee erodes the
first time someone adds a convenience helper.

**Migration is checked against projected state, not the raw log.** An issue that once passed
through a since-removed state is fine; one that currently sits in it is not. Checking the raw log
would refuse changes that are actually safe, which would teach people to skip the check.

**`UseSchema` does not re-validate.** Refusing there would leave the process enforcing an older
schema than the file on disk — a silent divergence. Callers run `CheckMigration` first, and the
schema is consulted per write rather than compiled into anything, so additive changes need no
restart and no migration.

**The initial state comes from the schema**, being the first state in the `open` category, rather
than a constant named `todo`. The schema decides what "the beginning" means.

## Evidence

**Verified by:** implementing session, `inc/feat-004-enforce-schema`

### WHEN a caller sets a field not defined in canon.yaml THE SYSTEM SHALL reject the write and name the valid fields

```
set storyPoints = "8"                      REJECTED
    field "storyPoints" is not defined in the schema;
    defined fields are component, evidence, priority, title
```

Note what this rejects for free: `storyPoints` cannot be added by any team, which is constitution
rule 10 enforced by the same mechanism as everything else rather than as a special case.

### WHEN a caller transitions to a state not permitted from the current state THE SYSTEM SHALL reject the write and name the permitted transitions

```
transition todo -> done                    REJECTED
    CANON-1 cannot move from "todo" to "done";
    permitted transitions from "todo" are abandoned, in_progress

agent: in_progress -> in_review, no evidence   REJECTED
    state "in_review" requires evidence; supply it with the transition
```

### THE SYSTEM SHALL expose no API or UI operation that adds a field, state or issue type at runtime

```
--- PASS: TestNoRuntimeSchemaMutation (0.00s)
```

Parses the source of three packages and fails on any function matching a schema-mutating name.

### WHEN a schema change would leave existing issues in an undefined state THE SYSTEM SHALL refuse to apply it and list the affected issue ids

```
--- PASS: TestRefusesOrphaningSchemaChange (0.00s)
```

Asserts the error lists the stranded issue, names the removed state, and does **not** list issues
that are unaffected.

### WHEN a schema change is purely additive THE SYSTEM SHALL apply it without restart or data migration

```
--- PASS: TestAdditiveSchemaChangeApplies (0.00s)
```

Adds a field to the live schema and writes it immediately, in-process.

### Full suite

```
--- PASS: TestRejectsUndefinedField          --- PASS: TestRejectsUnknownIssueType
--- PASS: TestRejectsValueOutsideEnum        --- PASS: TestRejectsFieldNotOnIssueType
--- PASS: TestRejectsIllegalTransition       --- PASS: TestRequiredFieldsAreEnforced
--- PASS: TestRejectsUndefinedState          --- PASS: TestRejectedWritesAppendNothing
--- PASS: TestRefusesOrphaningSchemaChange   --- PASS: TestAdditiveSchemaChangeApplies
--- PASS: TestNoRuntimeSchemaMutation
```

End to end, only accepted writes reached the log: **4 events from 8 attempts**.

### Scope

`git diff --cached --stat main` — run:

```
 internal/enforce/enforce.go        | 300 +++   validation and the only append path
 internal/enforce/enforce_test.go   | 280 +++   tests
 internal/projection/projection.go  |  11 ++    IssueIDs(), needed by CheckMigration
 specs/increment-plan.md            |   2 +-    status
```

`IssueIDs()` is a one-method addition to `projection`, outside this increment's package but
required by `CheckMigration` to enumerate affected issues. Noted rather than hidden.

### Not verified

Nothing outstanding. CI runs on the pull request.
