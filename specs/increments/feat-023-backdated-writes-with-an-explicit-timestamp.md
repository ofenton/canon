# feat-023: Backdated writes with an explicit timestamp

## Context

`Event.At` has always been a supplied parameter — every enforcer method takes `at time.Time` — but
no caller could supply it. The API stamped `s.now()` at twenty write sites, so the one thing the
event model was built to allow was unreachable.

This is the enabler for feat-025 (linking a commit with its real timestamp) and for any import of
an existing tracker, where every issue's history predates Canon.

## Design notes

**Backdating is a permission of its own.** Being allowed to transition an issue says nothing about
whether you should be allowed to record that it happened last Tuesday: the first changes what is
true, the second changes what the record says was true. So `backdate` joins the closed verb list
next to `create`, `delete`, `reparent` and `depend`, rather than riding along with the write it
accompanies. No existing schema names the verb, so **every instance refuses backdating until
somebody grants it deliberately** — the safe default is the one you get by doing nothing.

**Present-dated writes cost nothing.** `AuthoriseBackdate` returns immediately when the supplied
time is not before now, so the ordinary path does not pay for this feature — no projection refresh,
no role lookup. Only a genuinely historical write is checked.

**A query parameter, not a body field.** `?at=` is one code path covering every write, including
the `DELETE` routes that carry no body. A body field would have meant a different mechanism for
`DELETE /api/issues/{id}` than for `POST /api/issues`, and two mechanisms is how one of them ends
up unimplemented.

**Two refusals, and both are about coherence rather than policy.** A future-dated write would
record something that has not happened; a write dated before its issue was created would describe
an issue that did not exist. Two minutes of clock skew is tolerated, because an unsynchronised
laptop is ordinary and being off by ninety seconds is not an attempt to write the future.

**Arrival order is untouched.** `Seq` is assigned on append and `At` is data. A backdated event
therefore replays *after* everything already in the log — it adds to history rather than rewriting
what earlier events produced. This is the property that makes the feature safe, and it has its own
test.

## Evidence

**Verified by:** implementing session, `inc/feat-023-backdated-writes`

### WHEN a caller supplies `at` THE SYSTEM SHALL record that instant

Live, against a real server bootstrapped with an admin actor:

```
$ curl -X POST "localhost:8096/api/issues?at=2026-05-26T14:03:00Z" ... -d '{"title":"Imported from the old tracker"}'
{"id":"CANON-1"}
```

The log, three months later, showing supplied time against arrival order:

```
  seq  event time            type                   subject
  4    2026-05-26T14:03:00Z  issue.created          CANON-1     ← backdated 90 days
  5    2026-05-26T14:03:00Z  issue.team_set         CANON-1
  9    2026-08-24T07:27:16Z  issue.created          CANON-2     ← ordinary write
  10   2026-08-24T07:27:16Z  issue.team_set         CANON-2
```

`seq` increases with arrival while `at` does not, which is the AC on ordering.

### WHEN `at` is in the future THE SYSTEM SHALL refuse and say so

```
{"error":"cannot write CANON-2 dated 2027-01-01T00:00:00Z: that is in the future (now is 2026-08-24T07:27:07Z)"}
```

### WHEN the caller lacks the permission THE SYSTEM SHALL refuse

```
{"error":"sam holds role(s) member, which do not permit \"backdate\" on CANON-2;
          roles that would permit it: admin"}
```

The same actor writes normally in the next call — `{"id":"CANON-2"}` — so the grant gates
backdating, not writing.

A malformed timestamp is a 400 that shows the format rather than naming a Go layout:

```
{"error":"at must be an RFC 3339 timestamp such as 2026-08-24T09:30:00Z, got \"yesterday\""}
```

### Tests

Thirteen new tests — six in `internal/enforce` covering the decision, seven in `internal/api`
covering the boundary, including backdating a transition on a backdated issue, which is the exact
shape an import produces. Full suite green:

```
ok  github.com/ofenton/canon/cmd/canon
ok  github.com/ofenton/canon/internal/api
ok  github.com/ofenton/canon/internal/enforce
ok  github.com/ofenton/canon/internal/event
ok  github.com/ofenton/canon/internal/mcp
ok  github.com/ofenton/canon/internal/metrics
ok  github.com/ofenton/canon/internal/projection
ok  github.com/ofenton/canon/internal/query
ok  github.com/ofenton/canon/internal/schema
ok  github.com/ofenton/canon/internal/ui
```

### Scope

`git diff --cached --stat main` — run. Ten issue-write handlers, the enforcer decision, one verb
added to the schema's closed list, two `canon.yaml` files granting it to their admin role, and the
README.

### Not verified

**Backdating covers the ten issue-write routes, not all twenty writes.** Board, actor, role and
team writes still stamp the server clock. Backdating "sam joined the platform team last March" is
a real thing an import might want, and this increment does not do it. The ten chosen are the issue
lifecycle — what feat-025 and an import actually replay. Extending it is the same helper at more
call sites, not a different design.

**No UI.** There is no way to backdate from the browser, and I do not think there should be a
casual one.

**The before-creation rule uses the projected `CreatedAt`**, so an issue whose creation event was
itself backdated moves its own floor. That is correct — a test covers exactly this case — but it
does mean the rule constrains coherence, not honesty: someone with the grant can build a
self-consistent false history. The defence against that is the grant and the recorded actor on
every event, not the timestamp check.
