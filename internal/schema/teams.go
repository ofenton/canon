package schema

import (
	"fmt"
	"sort"
	"strings"
)

// Teams.
//
// Which teams exist is policy and belongs in canon.yaml, changed by pull request.
// Who is in one is state and belongs in the event log, changed by API call. That is
// the same split the roles already use, and teams shipped with only its second half:
// membership was recorded faithfully against a team name nothing had ever agreed to.
//
// The consequence was the exact failure this product argues against. "platform",
// "Platform" and "platfrom" were three teams, every one of them accepted, and a
// team-scoped role silently reached none of the others' issues. An org-wide tracker
// whose team names are free text cannot answer a cross-team question, which is the
// only reason to have one.

// Team is one team in the organisation.
type Team struct {
	Name string `yaml:"name" json:"name"`
	// Description is for humans reading the schema. It changes nothing.
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	line int
}

// TeamsDeclared reports whether the schema names its teams.
//
// A schema that does not is left alone: every instance created before this existed
// has teams in its log and none in its config, and refusing to start would break them
// all to enforce a rule they never agreed to. Declaring one team turns the check on.
func (s *Schema) TeamsDeclared() bool { return len(s.Teams) > 0 }

// HasTeam reports whether a team is declared.
func (s *Schema) HasTeam(name string) bool {
	if !s.TeamsDeclared() {
		return true
	}
	_, ok := s.teams[name]
	return ok
}

// TeamNames lists every declared team, sorted.
func (s *Schema) TeamNames() []string {
	out := make([]string, 0, len(s.Teams))
	for _, t := range s.Teams {
		out = append(out, t.Name)
	}
	sort.Strings(out)
	return out
}

// CheckTeam refuses a team the organisation has not declared.
//
// The error lists what does exist, because the overwhelmingly likely cause is a typo
// or a casing difference and the fix is then obvious without opening canon.yaml.
func (s *Schema) CheckTeam(name string) error {
	if name == "" || s.HasTeam(name) {
		return nil
	}
	return fmt.Errorf("team %q is not declared in canon.yaml; declared teams are %s",
		name, strings.Join(s.TeamNames(), ", "))
}

// validateTeams checks the teams block against itself.
func (s *Schema) validateTeams(add func(string, ...any)) {
	seen := map[string]int{}
	for _, t := range s.Teams {
		switch {
		case strings.TrimSpace(t.Name) == "":
			add("line %d: a team has no name", t.line)
		case t.Name != strings.TrimSpace(t.Name):
			add("line %d: team %q has leading or trailing space", t.line, t.Name)
		}
		if first, dup := seen[t.Name]; dup {
			add("line %d: team %q is already declared on line %d", t.line, t.Name, first)
			continue
		}
		seen[t.Name] = t.line
	}

	// A role scoped to a team is meaningless if no team exists to scope it to.
	if !s.TeamsDeclared() {
		for _, r := range s.Roles {
			if r.Scope == ScopeTeam {
				add("role %q is scoped to a team, but no teams are declared", r.Name)
			}
		}
	}
}
