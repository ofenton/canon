# feat-030: Teams are declared, not invented

## Context

Raised by review while looking at the running instance: teams were half-built. Membership was real —
`team.member_added` and `team.member_removed` are events, `Issue.Team` is projected, and team-scoped
roles resolve against it. But **which teams exist was recorded nowhere**, so nothing validated the
name:

```
team 'platform'            -> 204
team 'Platform'            -> 204        three teams
team 'platfrom'            -> 204
issue in team 'Marketting' -> 201
```

The README already states the correct rule for roles — *roles are policy, membership is state* —
and teams shipped with only the second half. The consequence was exactly the failure this product
exists to argue against, sitting inside it: an org-wide tracker whose team names are free text
cannot answer a cross-team question, which is the only reason to have one. Worse than a typo, a
team-scoped role silently reached none of the mis-spelled team's issues.

## Design notes

**Teams go in `canon.yaml`, membership stays in the log.** Same split as roles, for the same reason:
creating a team is a decision worth a pull request; adding a person to one is not, and making every
joiner a PR teaches people to route around the system.

**An undeclared schema accepts anything.** Every instance created before this has teams in its log
and none in its config. Refusing to start would break all of them to enforce a rule they never
agreed to. Declaring one team turns the check on — the migration is opt-in and its cost is visible.

**The error lists the declared teams.** The overwhelmingly likely cause is a typo or a casing
difference, and naming what exists makes the fix obvious without opening the schema.

**Removing a team that owns work is refused**, like removing a state or narrowing the hierarchy.
A team ceasing to exist does not make its work cease to exist; the reassignment should be a
decision, not a silent orphaning.

**A team-scoped role with no teams declared is refused at load.** It cannot mean anything, and a
permission that grants nothing is invisible at runtime — the same reasoning that already rejects
`field:storyPoints` by name.

**No team at all is still allowed.** "Unowned" is a real state, not a typo.

## Evidence

**Verified by:** implementing session, `inc/feat-030-declared-teams`

### The same probes, after

```
team 'platform'   -> 204
team 'Platform'   -> {"error":"team \"Platform\" is not declared in canon.yaml;
                       declared teams are growth, payments, platform"}   [422]
team 'platfrom'   -> same
issue in 'Marketting' -> same
```

And the teams are visible to any client, including an agent:

```
GET /api/schema → "teams": [{"name":"platform","description":"The core product"},
                            {"name":"growth"},{"name":"payments"}]
```

### Tests

Six in `internal/schema` — declared and undeclared teams, the no-teams-declared escape hatch, the
empty team, duplicates, padded names, and the team-scoped-role check. Three in `internal/enforce` —
refusal on create (asserting the refused write appends nothing), refusal on membership, and the
migration check naming both the team and the stranded issue.

Two existing tests failed on the first run, both using a `payments` team the schema did not declare.
That is the feature working: the fix was to declare `payments`, not to weaken the check.

Full suite green across all ten packages.

### Scope

`git diff --cached --stat main` — run. `internal/schema/teams.go` and its tests, the `teams` key
added to the top-level allowlist, `CheckTeam` called from `CreateAs` and `AddToTeam`,
`strandedByTeam` in the migration check, `teams` on `/api/schema`, and both shipped schemas.

### Not verified

**`RemoveFromTeam` is not checked**, deliberately: removing somebody from a team that should not
exist is the correct way out of a mess, and refusing it would trap them there.

**Nothing migrates an existing instance.** An operator whose log contains `platfrom` will have their
next schema load refused by the migration check with a clear message, and must then either declare
the typo or reassign the issues. There is no `canon rename-team`, and that is the tool the message
tells them they need.

**Teams have no metadata beyond a description** — no lead, no parent team, no archived flag. Adding
them is easy and none of it was asked for; the whole point of this increment is that the list is
short and reviewed.
