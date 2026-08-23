package enforce

import (
	"fmt"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/event"
)

// The actor registry.
//
// canon.yaml declares which roles exist and what each may do; that is policy, it
// changes rarely, and it is reviewed as a diff. Who holds a role and who is in which
// team is state: it changes weekly, and making every joiner a pull request would be
// intolerable and would teach people to route around the system. So membership is
// recorded as events, projected like anything else, and rebuildable from the log.
//
// Membership changes are additions, never erasures. A removal appends a
// team.member_removed event and the original join stays where it was, so an event
// written while someone was a member remains explicable years later.

// RegisterActor records a new human or agent identity.
func (e *Enforcer) RegisterActor(id string, kind event.ActorKind, model string, at time.Time, by event.Actor) error {
	if err := e.refresh(); err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("actor id is required")
	}
	if _, exists := e.view.Actor(id); exists {
		return fmt.Errorf("actor %q is already registered", id)
	}
	if !kind.Valid() {
		return fmt.Errorf("actor kind %q is not one of human, agent, system", kind)
	}
	if kind == event.ActorAgent && model == "" {
		return fmt.Errorf("agent %q must declare a model identifier", id)
	}
	if kind != event.ActorAgent && model != "" {
		return fmt.Errorf("only agent actors carry a model identifier, %q is %s", id, kind)
	}
	return e.append("actor.registered", id, at, by,
		map[string]any{"kind": string(kind), "model": model})
}

// GrantRole gives an actor a role defined in canon.yaml.
func (e *Enforcer) GrantRole(id, role string, at time.Time, by event.Actor) error {
	if err := e.requireActor(id); err != nil {
		return err
	}
	// A grant naming a role that does not exist would silently confer nothing, so
	// refuse at the point it is made rather than at the point it fails to work.
	if _, ok := e.schema.Role(role); !ok {
		return fmt.Errorf("role %q is not defined in canon.yaml; defined roles are %s",
			role, strings.Join(e.schema.RoleNames(), ", "))
	}
	return e.append("actor.role_granted", id, at, by, map[string]any{"role": role})
}

// RevokeRole removes a role from an actor.
func (e *Enforcer) RevokeRole(id, role string, at time.Time, by event.Actor) error {
	if err := e.requireActor(id); err != nil {
		return err
	}
	return e.append("actor.role_revoked", id, at, by, map[string]any{"role": role})
}

// AddToTeam records that an actor joined a team.
//
// Teams are not declared anywhere: a team exists because someone is in it. Declaring
// them in canon.yaml would put a weekly-changing list into a file meant for policy,
// and every reorganisation would become a schema review.
func (e *Enforcer) AddToTeam(id, team string, at time.Time, by event.Actor) error {
	if err := e.requireActor(id); err != nil {
		return err
	}
	if team == "" {
		return fmt.Errorf("team name is required")
	}
	return e.append("team.member_added", id, at, by, map[string]any{"team": team})
}

// RemoveFromTeam records that an actor left a team.
func (e *Enforcer) RemoveFromTeam(id, team string, at time.Time, by event.Actor) error {
	if err := e.requireActor(id); err != nil {
		return err
	}
	return e.append("team.member_removed", id, at, by, map[string]any{"team": team})
}

// Principal resolves an actor id into the identity, roles and teams to authorise with.
//
// Callers pass an id; the roles come from the log. Before this, a caller supplied its
// own roles, which made authorisation a suggestion.
func (e *Enforcer) Principal(id string) (Principal, error) {
	if err := e.refresh(); err != nil {
		return Principal{}, err
	}
	actor, ok := e.view.Actor(id)
	if !ok {
		return Principal{}, fmt.Errorf("actor %q is not registered; register it before it can act", id)
	}
	return Principal{
		Actor: event.Actor{ID: actor.ID, Kind: actor.Kind, Model: actor.Model},
		Roles: append([]string(nil), actor.Roles...),
		Teams: append([]string(nil), actor.Teams...),
	}, nil
}

// Actors lists every registered actor id, sorted.
func (e *Enforcer) Actors() ([]string, error) {
	if err := e.refresh(); err != nil {
		return nil, err
	}
	return e.view.ActorIDs(), nil
}

func (e *Enforcer) requireActor(id string) error {
	if err := e.refresh(); err != nil {
		return err
	}
	if _, ok := e.view.Actor(id); !ok {
		return fmt.Errorf("actor %q is not registered", id)
	}
	return nil
}
