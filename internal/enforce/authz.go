package enforce

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

// Principal is who is acting: the recorded actor plus the roles and teams they hold.
//
// v1 authorises but does not authenticate. The identity here is supplied by the
// caller and taken at face value, which is honest for a single-tenant instance that
// is not exposed to the internet and dishonest for anything else. Enforcement is
// written so that adding verification later changes how a Principal is constructed,
// not how it is used.
type Principal struct {
	Actor event.Actor
	Roles []string
	Teams []string
}

// ProposalRequired reports that an operation was refused outright but the actor's
// role would allow it to be proposed for human approval.
//
// It is distinct from a denial because the correct response differs: an agent that
// is denied should stop, and an agent that may propose should record the attempt.
// feat-007 turns this into a stored proposal; this increment decides it.
type ProposalRequired struct {
	Actor     string
	Operation string
	Subject   string
	Role      string
}

func (p *ProposalRequired) Error() string {
	return fmt.Sprintf("%s may not %s on %s directly, but may propose it for human approval (role %q)",
		p.Actor, p.Operation, p.Subject, p.Role)
}

// AsProposalRequired reports whether err is a ProposalRequired, assigning it to target.
func AsProposalRequired(err error, target **ProposalRequired) bool {
	return errors.As(err, target)
}

// authorise decides whether principal may perform op on an issue owned by ownerTeam.
//
// The rules, in order: an unrestricted schema permits everything; an unknown role is
// refused rather than ignored; a team-scoped role only reaches its own team's issues;
// and allow beats propose beats deny across all the roles an actor holds.
func (e *Enforcer) authorise(p Principal, op, subject, ownerTeam string) error {
	if e.schema.Unrestricted() {
		return nil
	}
	if len(p.Roles) == 0 {
		return fmt.Errorf("%s holds no role; operation %q on %s refused (roles that permit it: %s)",
			p.Actor.ID, op, subject, e.permittingRoles(op))
	}

	best := schema.Deny
	var bestRole string
	var outOfScope []string

	for _, name := range p.Roles {
		role, ok := e.schema.Role(name)
		if !ok {
			return fmt.Errorf("%s holds role %q, which is not defined in the schema; defined roles are %s",
				p.Actor.ID, name, strings.Join(e.schema.RoleNames(), ", "))
		}
		decision := role.Decide(op)
		if decision == schema.Deny {
			continue
		}
		// A team-scoped role reaches only its own team's issues. An issue with no
		// owning team is reachable by any scoped role: refusing it would make
		// unowned issues editable by nobody.
		if role.Scope == schema.ScopeTeam && ownerTeam != "" && !contains(p.Teams, ownerTeam) {
			outOfScope = append(outOfScope, name)
			continue
		}
		if decision > best {
			best, bestRole = decision, name
		}
	}

	switch best {
	case schema.Allow:
		return nil
	case schema.Propose:
		return &ProposalRequired{
			Actor: p.Actor.ID, Operation: op, Subject: subject, Role: bestRole,
		}
	}

	if len(outOfScope) > 0 {
		sort.Strings(outOfScope)
		return fmt.Errorf("%s may %s, but %s is owned by team %q and role(s) %s are scoped to their own team (member of: %s)",
			p.Actor.ID, op, subject, ownerTeam, strings.Join(outOfScope, ", "),
			strings.Join(orNone(p.Teams), ", "))
	}
	return fmt.Errorf("%s holds role(s) %s, which do not permit %q on %s; roles that would permit it: %s",
		p.Actor.ID, strings.Join(p.Roles, ", "), op, subject, e.permittingRoles(op))
}

func (e *Enforcer) permittingRoles(op string) string {
	roles := e.schema.RolesPermitting(op)
	if len(roles) == 0 {
		return "none — no role in canon.yaml grants this"
	}
	return strings.Join(roles, ", ")
}

// CreateAs records a new issue on behalf of a principal, owned by ownerTeam.
func (e *Enforcer) CreateAs(p Principal, id, issueType string, fields map[string]string, ownerTeam string, at time.Time) error {
	// A creator must be able to reach the team they are creating into, or a scoped
	// role could seed issues it may not subsequently touch.
	if err := e.authorise(p, "create", id, ownerTeam); err != nil {
		return err
	}
	for name := range fields {
		if err := e.authorise(p, schema.FieldOp(name), id, ownerTeam); err != nil {
			return err
		}
	}
	if err := e.Create(id, issueType, fields, at, p.Actor); err != nil {
		return err
	}
	if ownerTeam == "" {
		return nil
	}
	return e.append("issue.team_set", id, at, p.Actor, map[string]any{"team": ownerTeam})
}

// SetFieldAs records a field value on behalf of a principal.
func (e *Enforcer) SetFieldAs(p Principal, id, field, value string, at time.Time) error {
	team, err := e.ownerTeam(id)
	if err != nil {
		return err
	}
	if err := e.authorise(p, schema.FieldOp(field), id, team); err != nil {
		return err
	}
	return e.SetField(id, field, value, at, p.Actor)
}

// TransitionAs moves an issue on behalf of a principal.
func (e *Enforcer) TransitionAs(p Principal, id, to, evidence string, at time.Time) error {
	if err := e.refresh(); err != nil {
		return err
	}
	issue, ok := e.view.Issue(id)
	if !ok {
		return fmt.Errorf("unknown issue %s", id)
	}
	if err := e.authorise(p, schema.TransitionOp(issue.State, to), id, issue.Team); err != nil {
		return err
	}
	return e.Transition(id, to, evidence, at, p.Actor)
}

// DeleteAs deletes an issue on behalf of a principal.
func (e *Enforcer) DeleteAs(p Principal, id string, at time.Time) error {
	team, err := e.ownerTeam(id)
	if err != nil {
		return err
	}
	if err := e.authorise(p, "delete", id, team); err != nil {
		return err
	}
	return e.Delete(id, at, p.Actor)
}

// ReparentAs sets an issue's parent on behalf of a principal.
func (e *Enforcer) ReparentAs(p Principal, id, parent string, at time.Time) error {
	team, err := e.ownerTeam(id)
	if err != nil {
		return err
	}
	if err := e.authorise(p, "reparent", id, team); err != nil {
		return err
	}
	return e.Reparent(id, parent, at, p.Actor)
}

func (e *Enforcer) ownerTeam(id string) (string, error) {
	if err := e.refresh(); err != nil {
		return "", err
	}
	issue, ok := e.view.Issue(id)
	if !ok {
		return "", fmt.Errorf("unknown issue %s", id)
	}
	return issue.Team, nil
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

func orNone(teams []string) []string {
	if len(teams) == 0 {
		return []string{"no teams"}
	}
	return teams
}
