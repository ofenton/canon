# feat-015: Actor registry and team membership

## Context

feat-014 defined what roles may do. This decides who holds them — and closes the gap where a
caller supplied its own roles, which made authorisation a suggestion.

## Design notes

**Policy in config, membership in the log — the other half.** `canon.yaml` declares which roles
exist and what each may do: policy, rarely changed, reviewed as a diff. Who holds a role and who
is in which team changes weekly, so it is recorded as events and projected like anything else.
Putting membership in `canon.yaml` would make every joiner a pull request, and a process people
find intolerable is a process they route around.

**Membership changes are additions, never erasures.** A removal appends
`team.member_removed`; the original join stays where it was. An event written while someone was
a member therefore remains explicable years later, which is the entire reason for an append-only
log.

**Teams are not declared anywhere — a team exists because someone is in it.** Declaring them
would put a weekly-changing list into a policy file and make every reorganisation a schema review.

**`Principal` is now resolved from the log, not supplied by the caller.** Before this, a caller
passed its own roles. That was fine for testing feat-014's decision logic and useless as
authorisation.

**A grant naming an undefined role is refused at the point it is made.** Otherwise it silently
confers nothing and only surfaces later as a mysterious denial.

**Actors are part of the determinism digest.** Without that, a registry projection bug would be
invisible to the rebuild test that exists to catch exactly this class of problem.

## Evidence

**Verified by:** implementing session, `inc/feat-015-actor-registry`

```
  ollie       create PLAT-1                       allowed   roles=[admin] teams=[platform]
  sam         set priority (registered, no role)  DENIED    sam holds no role; roles that
                                                            permit it: admin, agent, member
  ghost       set priority (never registered)     REJECTED  actor "ghost" is not registered

  grant sam the member role and add to platform...
  sam         set priority (after grant)          allowed   roles=[member] teams=[platform]

  sam leaves the platform team...
  sam         set priority (after leaving)        DENIED    PLAT-1 is owned by team "platform"
                                                            and member is scoped to their own
                                                            team (member of: no teams)

  agent:one   todo -> in_progress                 allowed   roles=[agent] teams=[platform]
  agent:one   -> done                             PROPOSE   transition:in_progress->done

  events: 14 — identity, membership and work in one log
```

### Identities and membership recorded as events, not in canon.yaml

```
--- PASS: TestRegistryLivesInTheLog
```

Asserts exactly one `actor.registered`, `actor.role_granted` and `team.member_added` event, and
that the resolved principal carries the right roles, teams and actor kind.

### A role grant applies to subsequent writes without a restart

```
--- PASS: TestRoleGrantAppliesImmediately
--- PASS: TestRevokingARoleTakesEffect
```

Write refused, grant, write permitted — in one process. Revocation is checked in the same way.

### An unregistered actor is rejected, naming the actor

```
--- PASS: TestUnregisteredActorIsRejected
--- PASS: TestRejectsDuplicateRegistration
--- PASS: TestRejectsUnknownRoleGrant
--- PASS: TestAgentActorsMustDeclareAModel
```

### Prior membership is retained in the log

```
--- PASS: TestMembershipHistoryIsRetained
--- PASS: TestRegistryRebuildsDeterministically
```

Both the join and the leave remain in the log after a removal; a fresh projection over the same
log resolves identical roles and teams.

### Scope

`git diff --cached --stat main` — run. The registry in `enforce`, the projected `Actor` entity
and its five event types in `projection`. The projection change is named in this increment's
Scope ("project them").

### Not verified

Still no authentication: an actor id is claimed, not proven. The registry means a claimed id must
at least correspond to a registered actor with real roles, which is a meaningful narrowing, but
it is not identity. A minimal token scheme may land on Sunday if there is room.

CI runs on the pull request.
