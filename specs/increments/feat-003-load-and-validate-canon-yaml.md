# feat-003: Load and validate canon.yaml

## Context

The organisation's entire issue schema in one versioned file. This increment reads and validates
it; feat-004 enforces it on writes.

## Design notes

**All problems are reported at once, sorted by line.** Fixing a schema one error per run is the
friction that stops people improving it — and a schema people avoid touching is how a workflow
ossifies at fifteen states nobody understands.

**Unknown top-level keys are refused, not ignored.** A silently-ignored key means a typo disables
a rule without telling anyone, and it is exactly how `sprints:` would quietly become one team's
local extension. Refusing it is the wedge, enforced at load time.

**Syntax errors print the offending region, not just a line number.** yaml.v3 attributes an
indentation error to the line the *block* began on: a bad indent on line 4 is reported against
line 2. A bare number therefore sends a reader two lines above the fault. The error now shows the
region with a marker, so it is findable regardless of the parser's precision. The test asserts
the region and the marker rather than an exact line, because asserting a number the library
cannot deliver would be testing the wrong thing.

**Categories are a closed set** — open, active, closed. This is the direct answer to "completed
has sixteen spellings". Without a fixed grouping, no cross-team question has a true answer.

**`PermittedFrom` exists for error messages.** Enforcement should tell a caller what it should
have done, not only that it was wrong.

**Issue types are a view over fields, not a storage type.** Consistent with constitution rule 9:
epics and sub-tasks are parent/child relations, not entities.

## Evidence

**Verified by:** implementing session, `inc/feat-003-canon-yaml-loader`

### THE SYSTEM SHALL read the entire issue schema from one canon.yaml at a configured path

```
$ canon schema -schema internal/schema/testdata/canon.yaml
internal/schema/testdata/canon.yaml is valid

  5 states, 6 transitions, 4 fields, 3 issue types

  todo           open     -> abandoned, in_progress
  in_progress    active   -> abandoned, in_review
  in_review      active   -> done, in_progress  (requires evidence)
  done           closed   ->
  abandoned      closed   ->
```

```
--- PASS: TestLoadsTheWholeSchema
```

Asserts the loader answers what enforcement will ask: `HasState`, `CanTransition`,
`RequiresEvidence`, `PermittedFrom`.

### WHEN canon.yaml is syntactically invalid THE SYSTEM SHALL refuse to start and name the offending line number

```
$ canon schema -schema /tmp/bad.yaml
canon: parsing schema /tmp/bad.yaml: yaml: line 2: did not find expected '-' indicator

  the parser stopped at line 2; the problem is at or just after it:
        1 | version: 1
  ->    2 | states:
        3 |   - name: todo
        4 |    category: open
```

```
--- PASS: TestInvalidSyntaxNamesTheLine
```

Non-zero exit, so it refuses to start rather than continuing on a partial schema.

### WHEN canon.yaml references an undefined state in a transition THE SYSTEM SHALL refuse to start and name the transition

```
$ canon schema -schema /tmp/bad2.yaml
canon: schema /tmp/bad2.yaml has 4 problem(s):
  - line 2: state "todo" has category "sideways"; valid categories are open, active, closed
  - line 3: transition todo -> nowhere refers to undefined state "nowhere"
  - line 4: enum field "priority" must list its values
  - line 5: issue type "task" references undefined field "missing"
```

```
--- PASS: TestUndefinedTransitionStateNamesTheTransition
--- PASS: TestReportsAllProblemsAtOnce
--- PASS: TestRejectsStructurallyInvalidSchemas (8 subtests)
--- PASS: TestMissingFileIsClear
```

### Scope

`git diff --cached --stat main` — run:

```
 cmd/canon/main.go                    |  40 ++++   schema subcommand
 go.mod / go.sum                      |   4 ++    yaml.v3
 internal/schema/schema.go            | 400 +++   loader and validator
 internal/schema/schema_test.go       | 210 +++   tests
 internal/schema/testdata/canon.yaml  |  50 +++   a realistic org schema
 specs/increment-plan.md              |   2 +-    status
```

No files outside Scope.

### Dependency added

- `gopkg.in/yaml.v3` — decoding via `yaml.Node` is what preserves line numbers. An error that
  cannot be located in the file is barely better than no error.

### Not verified

Nothing outstanding. CI runs on the pull request.
