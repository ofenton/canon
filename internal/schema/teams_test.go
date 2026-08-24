package schema

import (
	"strings"
	"testing"
)

// AC: WHEN a caller names a team not declared in canon.yaml THE SYSTEM SHALL refuse
// the write and list the teams that exist.
func TestUndeclaredTeamIsRefusedAndDeclaredOnesListed(t *testing.T) {
	s := load(t, `version: 1
states: [{name: todo, category: open}]
transitions: []
fields: [{name: title, type: string, required: true}]
issue_types: [{name: task, fields: [title]}]
teams:
  - {name: platform}
  - {name: growth}
`)
	if err := s.CheckTeam("platform"); err != nil {
		t.Fatalf("a declared team must be accepted: %v", err)
	}
	// Casing is not a nicety here: a team-scoped role would silently reach nothing.
	for _, bad := range []string{"Platform", "platfrom", "marketing"} {
		err := s.CheckTeam(bad)
		if err == nil {
			t.Fatalf("team %q is not declared and should be refused", bad)
		}
		if !strings.Contains(err.Error(), "growth") || !strings.Contains(err.Error(), "platform") {
			t.Fatalf("the error should list the declared teams, got: %v", err)
		}
	}
}

// AC: WHERE a schema declares no teams THE SYSTEM SHALL accept any team.
//
// Every instance created before this existed has teams in its log and none in its
// config. Refusing to start would break them all to enforce a rule they never agreed
// to; declaring one team turns the check on.
func TestASchemaWithNoTeamsAcceptsAny(t *testing.T) {
	s := load(t, `version: 1
states: [{name: todo, category: open}]
transitions: []
fields: [{name: title, type: string, required: true}]
issue_types: [{name: task, fields: [title]}]
`)
	if s.TeamsDeclared() {
		t.Fatal("no teams block means no teams declared")
	}
	for _, any := range []string{"platform", "anything at all", "Platform"} {
		if err := s.CheckTeam(any); err != nil {
			t.Fatalf("an undeclared schema must accept %q: %v", any, err)
		}
	}
}

// An empty team is "unowned", which is a real state and not a typo.
func TestNoTeamIsAlwaysAllowed(t *testing.T) {
	s := load(t, `version: 1
states: [{name: todo, category: open}]
transitions: []
fields: [{name: title, type: string, required: true}]
issue_types: [{name: task, fields: [title]}]
teams: [{name: platform}]
`)
	if err := s.CheckTeam(""); err != nil {
		t.Fatalf("an issue with no team must be allowed: %v", err)
	}
}

func TestDuplicateTeamIsRefused(t *testing.T) {
	_, err := loadErr(t, `version: 1
states: [{name: todo, category: open}]
transitions: []
fields: [{name: title, type: string, required: true}]
issue_types: [{name: task, fields: [title]}]
teams:
  - {name: platform}
  - {name: platform}
`)
	if err == nil || !strings.Contains(err.Error(), "already declared") {
		t.Fatalf("a duplicate team should be refused, got: %v", err)
	}
}

func TestTeamWithSurroundingSpaceIsRefused(t *testing.T) {
	_, err := loadErr(t, `version: 1
states: [{name: todo, category: open}]
transitions: []
fields: [{name: title, type: string, required: true}]
issue_types: [{name: task, fields: [title]}]
teams: [{name: "platform "}]
`)
	if err == nil || !strings.Contains(err.Error(), "trailing space") {
		t.Fatalf("a padded team name should be refused, got: %v", err)
	}
}

// A role scoped to a team means nothing if no team exists to scope it to.
func TestTeamScopedRoleWithoutTeamsIsRefused(t *testing.T) {
	_, err := loadErr(t, `version: 1
states: [{name: todo, category: open}]
transitions: []
fields: [{name: title, type: string, required: true}]
issue_types: [{name: task, fields: [title]}]
roles:
  - {name: member, scope: team, can: [create]}
`)
	if err == nil || !strings.Contains(err.Error(), "no teams are declared") {
		t.Fatalf("a team-scoped role without teams should be refused, got: %v", err)
	}
}

// loadErr loads a schema expecting it to be rejected, returning the error to inspect.
func loadErr(t *testing.T, body string) (*Schema, error) {
	t.Helper()
	return Load(write(t, body))
}
