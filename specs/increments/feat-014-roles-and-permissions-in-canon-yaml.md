# feat-014: Roles and permissions in canon.yaml

## Context

Added Wednesday after review found R15 — an agent lacking permission records a proposal — had
nothing to define "permission" against. The research that motivated Canon found 40–100+ permission
schemes per Jira instance where 10–15 would do, so permissions defined per project would
reintroduce the exact disease the product exists to prevent.

## Design notes

**Policy in config, membership in the log.** Which roles exist and what each may do is policy: it
belongs in `canon.yaml`, changes rarely, and is reviewed as a diff. Who is in which team is state
and belongs in the event log (feat-015). Making every joiner a pull request would be intolerable
and would teach people to route around the system.

**The operation vocabulary is closed and short.** Three verbs — create, delete, reparent — plus
the parameterised `field:<name>` and `transition:<from>-><to>`, with family wildcards. A
permission vocabulary that grows per project is how 40 schemes become 100.

**Grants are validated against the rest of the schema at load.** `field:storyPoints` or
`transition:todo->shipped` is refused by name. A typo in a permission silently grants nothing and
is invisible at runtime, so load time is the only cheap moment to catch it.

**`propose:` is a first-class outcome, not a denial.** An agent refused outright either stops or
retries blindly; neither is useful. `ProposalRequired` is a distinct error type so callers can
tell the two apart — feat-007 turns it into a stored proposal, but the decision is made here.
This is the two-gate pattern from the development process appearing inside the product.

**Allow beats propose beats deny, across all roles an actor holds.** A role can be given a broad
`propose: [transition:*]` with a narrow `can: [transition:todo->in_progress]` carved out of it,
which is exactly how the `agent` role is written.

**Authorisation is opt-in.** A schema with no roles reports itself unrestricted and permits
everything. Half-enforced authorisation is worse than none, because it looks like a guarantee;
absent authorisation is at least obvious.

**An issue with no owning team is reachable by any scoped role.** Refusing it would leave unowned
issues editable by nobody, which is a worse failure than being slightly permissive.

## Evidence

**Verified by:** implementing session, `inc/feat-014-roles-and-permissions`

```
  admin     create PLAT-1 (team platform)        allowed
  admin     create PAY-1 (team payments)         allowed

  member    priority on own team's issue         allowed
  member    priority on another team's           DENIED
            sam may field:priority, but PAY-1 is owned by team "payments" and
            role(s) member are scoped to their own team (member of: platform)
  member    delete PLAT-1                        DENIED
            sam holds role(s) member, which do not permit "delete" on PLAT-1;
            roles that would permit it: admin, agent

  reporter  set title                            allowed
  reporter  set priority                         DENIED
            roles that would permit it: admin, agent, member

  agent     todo -> in_progress                  allowed
  agent     -> in_review (with evidence)         allowed
  agent     -> done                              PROPOSE   transition:in_review->done

  admin     -> done                              allowed
```

### Every role and its permissions defined in canon.yaml, no per-project override

```
--- PASS: TestLoadsRoles              --- PASS: TestRolePermissions (13 cases)
--- PASS: TestRolesAreOptional        --- PASS: TestUnrestrictedSchemaPermitsEverything
```

### A denied operation names the roles that would permit it

```
--- PASS: TestDeniedOperationNamesPermittingRoles
--- PASS: TestPermittedOperationsSucceed
--- PASS: TestDeniedWritesAppendNothing
```

Denied writes append nothing, exactly as schema rejections do not.

### scope: team permits only issues owned by a team the actor belongs to

```
--- PASS: TestTeamScopeIsEnforced
```

Same role, two teams, one issue: permitted for the owner, refused for the other, and an
org-scoped role crosses freely.

### A grant naming an operation that does not exist refuses startup

```
--- PASS: TestRejectsUnknownOperations/unknown_verb        (teleport)
--- PASS: TestRejectsUnknownOperations/unknown_field       (field:storyPoints)
--- PASS: TestRejectsUnknownOperations/unknown_state       (transition:todo->shipped)
--- PASS: TestRejectsUnknownOperations/malformed_transition
--- PASS: TestRejectsUnknownOperations/unknown_scope       (scope: galaxy)
--- PASS: TestRejectsDuplicateAndEmptyRoles (3 cases)
```

### No runtime interface for creating or altering a role

```
--- PASS: TestNoRuntimeRoleMutation
```

Parses three packages, fails on `AddRole`, `GrantPermission`, `CreateRole`, `SetRole` or `Grant`.

### Scope

`git diff --cached --stat main` — run. Roles in `schema`, enforcement in `enforce`, an
`issue.team_set` event and a `Team` field in `projection` so team scope has something to resolve
against. That projection change is named in this increment's Scope.

### Not verified

Authorisation is enforced against a **claimed** identity — v1 authorises but does not
authenticate, per the spec's Out of scope. `Principal` is constructed by the caller, so adding
verification later changes how one is built, not how it is used.

CI runs on the pull request.
