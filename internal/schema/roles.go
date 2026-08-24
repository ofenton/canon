package schema

import (
	"fmt"
	"sort"
	"strings"
)

// Decision is what a role permits for one operation.
//
// Propose exists because an agent refused outright either stops or retries blindly,
// and neither is useful. Recording the attempt for a human to approve is the same
// two-gate shape the rest of this system uses: the agent proposes, a person decides.
type Decision int

const (
	Deny Decision = iota
	Allow
	Propose
)

func (d Decision) String() string {
	switch d {
	case Allow:
		return "allow"
	case Propose:
		return "propose"
	default:
		return "deny"
	}
}

// Scope narrows where a role's grants apply.
type Scope string

const (
	// ScopeOrg is the default: the role applies to every issue.
	ScopeOrg Scope = ""
	// ScopeTeam limits the role to issues owned by a team the actor belongs to.
	ScopeTeam Scope = "team"
)

// Verbs are the operations a role may be granted, beyond the parameterised
// field: and transition: forms. Keeping this list short and closed is the point:
// a permission vocabulary that grows per project is how 40 schemes become 100.
var Verbs = []string{"create", "delete", "reparent", "depend", "backdate", "link", "administer"}

// DependOp is the operation of adding or removing a dependency.
const DependOp = "depend"

// AdministerOp is the operation of changing who exists and what they may do:
// registering actors, granting and revoking roles, team membership, and tokens.
//
// It is separate from every other verb because it is the only one that can change the
// answer to "what may I do". Without it, any registered actor could grant itself any
// role — which it could, until feat-031 — and every other permission in the schema
// was decoration.
const AdministerOp = "administer"

// Role is one named set of grants. Roles are org-wide policy and live in canon.yaml;
// who holds a role is state and lives in the event log.
type Role struct {
	Name    string   `yaml:"name"`
	Scope   Scope    `yaml:"scope"`
	Can     []string `yaml:"can"`
	Propose []string `yaml:"propose"`

	line int
}

// Decide reports what this role permits for an operation such as "delete",
// "field:priority" or "transition:todo->in_progress".
//
// An explicit grant beats a wildcard, and allow beats propose, so a role can be
// given a broad propose rule and a narrow allow carved out of it.
func (r Role) Decide(op string) Decision {
	if matchesAny(r.Can, op) {
		return Allow
	}
	if matchesAny(r.Propose, op) {
		return Propose
	}
	return Deny
}

// matchesAny reports whether op is covered by any grant, honouring the family
// wildcards "field:*" and "transition:*".
func matchesAny(grants []string, op string) bool {
	family, _, hasFamily := strings.Cut(op, ":")
	for _, grant := range grants {
		if grant == op || grant == "*" {
			return true
		}
		if hasFamily && grant == family+":*" {
			return true
		}
	}
	return false
}

// Role returns a role by name.
func (s *Schema) Role(name string) (Role, bool) {
	r, ok := s.roles[name]
	return r, ok
}

// RoleNames lists every defined role, sorted.
func (s *Schema) RoleNames() []string {
	out := make([]string, 0, len(s.roles))
	for name := range s.roles {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Unrestricted reports whether the schema defines no roles at all.
//
// Authorisation is opt-in so it can be adopted incrementally. A schema with no roles
// permits everything, which is the correct behaviour for a single-user instance and
// an obvious one to notice, rather than a subtle half-enforced state.
func (s *Schema) Unrestricted() bool { return len(s.roles) == 0 }

// RolesPermitting lists the roles that would allow or propose an operation, sorted.
// Enforcement uses it to tell a caller which role they would need.
func (s *Schema) RolesPermitting(op string) []string {
	var out []string
	for name, role := range s.roles {
		if role.Decide(op) != Deny {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// validateRoles checks every grant against the rest of the schema.
//
// A grant naming a field or state that does not exist is almost always a typo, and a
// typo in a permission silently grants or withholds nothing at all. Refusing at load
// is the only point at which it is cheap to catch.
func (s *Schema) validateRoles(add func(string, ...any)) {
	seen := map[string]int{}
	for _, r := range s.Roles {
		switch {
		case r.Name == "":
			add("line %d: role has no name", r.line)
			continue
		case seen[r.Name] != 0:
			add("line %d: duplicate role %q, first defined at line %d", r.line, r.Name, seen[r.Name])
			continue
		default:
			seen[r.Name] = r.line
		}

		if r.Scope != ScopeOrg && r.Scope != ScopeTeam {
			add("line %d: role %q has scope %q; valid scopes are team, or omit for org-wide",
				r.line, r.Name, r.Scope)
		}
		if len(r.Can) == 0 && len(r.Propose) == 0 {
			add("line %d: role %q grants nothing; remove it or give it can or propose",
				r.line, r.Name)
		}
		for _, grant := range append(append([]string{}, r.Can...), r.Propose...) {
			if err := s.checkGrant(grant); err != nil {
				add("line %d: role %q: %v", r.line, r.Name, err)
			}
		}
	}
}

// checkGrant reports why a grant is not a valid operation.
func (s *Schema) checkGrant(grant string) error {
	if grant == "*" {
		return nil
	}
	family, rest, hasFamily := strings.Cut(grant, ":")
	if !hasFamily {
		for _, verb := range Verbs {
			if grant == verb {
				return nil
			}
		}
		return fmt.Errorf("unknown operation %q; valid operations are %s, field:<name>, transition:<from>-><to>, or the wildcards field:* and transition:*",
			grant, strings.Join(Verbs, ", "))
	}

	switch family {
	case "field":
		if rest == "*" {
			return nil
		}
		if !s.HasField(rest) {
			return fmt.Errorf("grant %q names undefined field %q; defined fields are %s",
				grant, rest, strings.Join(s.FieldNames(), ", "))
		}
	case "transition":
		if rest == "*" {
			return nil
		}
		from, to, ok := strings.Cut(rest, "->")
		if !ok {
			return fmt.Errorf("grant %q is malformed; a transition grant is transition:<from>-><to> or transition:*", grant)
		}
		if !s.HasState(from) {
			return fmt.Errorf("grant %q names undefined state %q", grant, from)
		}
		if !s.HasState(to) {
			return fmt.Errorf("grant %q names undefined state %q", grant, to)
		}
		if !s.CanTransition(from, to) {
			return fmt.Errorf("grant %q names a transition the schema does not permit", grant)
		}
	default:
		return fmt.Errorf("unknown operation family %q in %q; valid families are field and transition", family, grant)
	}
	return nil
}

// Operation builders, so callers cannot drift from the grant syntax.

// FieldOp returns the operation string for setting a field.
func FieldOp(field string) string { return "field:" + field }

// TransitionOp returns the operation string for one transition.
func TransitionOp(from, to string) string { return "transition:" + from + "->" + to }
