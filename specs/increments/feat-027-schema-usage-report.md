# feat-027: Schema usage report

## Context

Jira instances reach 700–800 custom fields with **over half unused in twelve months**, because
nobody is ever shown the aggregate. Every individual request was reasonable; no one saw the total,
so no one ever said no.

Canon's answer so far has been that the schema is one reviewed file. That is only half an argument:
a reviewer approving the 40th field has exactly the Jira admin's problem unless they can see what
the existing 39 are doing. The diff shows what is being added; this shows what is already there and
dead.

## Design notes

**It counts issues, not events.** "Twelve issues use this field" is the question a reviewer is
actually asking. "This field was written 400 times" flatters a field that one issue churned on.

**Everything declared appears, used or not.** The unused rows are the point of the report, so they
sort first — the rows needing a decision should not be at the bottom of a long list of healthy ones.

**Enums carry their distribution.** A four-value priority enum where everything is `p2` is a field
pretending to be a decision, and that is invisible from a count. Free-text fields get no
distribution, because the distinct values of a string field are just its contents.

**Teams and roles are reported too.** They are declared configuration like anything else, and a
role nobody holds is as worth seeing as a field nobody sets — especially now that feat-030 makes
teams declared rather than invented.

**Dates read as "4 months ago", not as dates.** Somebody deciding whether to delete a field wants
the answer, not arithmetic.

## The defect the first run found

Run against Canon's own instance, the first version reported:

```
  title                  unused   nothing uses this
  evidence               unused   nothing uses this
```

Every issue has a title. `title` and `evidence` are schema fields that the projection **promotes
out of the `Fields` map** — title onto `Issue`, evidence onto the transition — so counting `Fields`
alone missed them entirely.

On a schema where `title` is the one required field, that is not a small inaccuracy. It is the
report confidently advising somebody to delete the field everything depends on. A tool whose whole
purpose is telling you what is safe to remove has to be right about that.

Fixing it also revealed that **evidence was recorded on the event and never projected**, so nothing
could see it at all. `Transition.Evidence` now exists, which the detail view will want too.

```
  title                      39   last used under an hour ago
  evidence                   35   last used under an hour ago
```

## Evidence

**Verified by:** implementing session, `inc/feat-027-schema-usage`

### Against Canon's own instance

```
$ canon usage

field
  title                      39   last used under an hour ago
  tier                       38   last used under an hour ago  (1 17, 2 17, 3 4)
  kind                       38   last used under an hour ago  (feature 27, fix 5, chore 3, docs 2, perf 1)
  scope                      38   last used under an hour ago
  rollback                   38   last used under an hour ago
  risk                       38   last used under an hour ago
  evidence                   35   last used under an hour ago
  traces                     33   last used under an hour ago

state
  done                       35   last used under an hour ago
  approved                    3   last used under an hour ago
  planned                     1   last used under an hour ago
  in_progress            unused   nothing uses this
  in_review              unused   nothing uses this
  abandoned              unused   nothing uses this

issue type
  increment                  39   last used under an hour ago

team
  canon                      38   last used under an hour ago

role
  maintainer                  1
  agent                  unused   nothing uses this

18 declared, 6 unused — every unused row is a line somebody could delete from canon.yaml
```

The three unused states are honest and correct: this instance's history was imported and every
increment is sitting at rest, so nothing is currently in flight. `abandoned` has genuinely never
been used. The `agent` role is declared and unheld, which is a true statement about a project where
an agent has been acting as `ollie` throughout — arguably the most useful line in the report.

### Tests

Five in `internal/enforce`: per-field counts, unused configuration sorting first with no last-used
time, the promoted-field case above, enum distribution (including a string field carrying none), and
teams and roles. Route contract and naming tests extended to `/api/schema/usage`; MCP description
added, which the parity test required before it would pass.

Full suite green across all ten packages.

### Scope

`git diff --cached --stat main` — run. `internal/enforce/usage.go` and its tests, `Transition.Evidence`
in the projection, `GET /api/schema/usage`, the MCP description, `canon usage`, and the usage text.

### Not verified

**Counts are over live issues, so a deleted issue's usage disappears.** A field used only by work
that was later deleted reads as unused. That is arguably right — nothing live needs it — but it is
a judgement, not an obvious truth, and the log could answer differently.

**"Last used" is the issue's `UpdatedAt`, not when that field was last written.** An issue touched
today makes every field on it look touched today. Per-field timestamps would need the projection to
carry them, which is real memory on a large instance for a report nobody runs hourly.

**Nothing surfaces this in the UI.** It is a CLI command and an API route; a reviewer looking at a
`canon.yaml` pull request would most want it as a comment on that PR, which is a CI integration
nobody has written.
