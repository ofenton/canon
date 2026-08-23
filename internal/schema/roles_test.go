package schema

import (
	"strings"
	"testing"
)

func TestLoadsRoles(t *testing.T) {
	s, err := Load("testdata/canon.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(s.Roles) != 4 {
		t.Fatalf("roles: got %d want 4", len(s.Roles))
	}

	admin, ok := s.Role("admin")
	if !ok {
		t.Fatal("admin role missing")
	}
	if admin.Scope != "" {
		t.Errorf("admin scope: got %q, want unscoped", admin.Scope)
	}
	member, _ := s.Role("member")
	if member.Scope != ScopeTeam {
		t.Errorf("member scope: got %q want team", member.Scope)
	}
}

func TestRolePermissions(t *testing.T) {
	s, err := Load("testdata/canon.yaml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cases := []struct {
		role, op string
		want     Decision
	}{
		{"admin", "delete", Allow},
		{"admin", "transition:todo->in_progress", Allow},
		{"admin", "field:priority", Allow},
		{"member", "delete", Deny},
		{"member", "transition:in_review->done", Allow},
		{"agent", "transition:todo->in_progress", Allow},
		{"agent", "transition:in_review->done", Propose},
		{"agent", "delete", Propose},
		{"agent", "field:priority", Allow},
		{"reporter", "field:title", Allow},
		{"reporter", "field:priority", Deny},
		{"reporter", "transition:todo->in_progress", Deny},
		{"reporter", "delete", Deny},
	}
	for _, tc := range cases {
		r, ok := s.Role(tc.role)
		if !ok {
			t.Fatalf("role %q missing", tc.role)
		}
		if got := r.Decide(tc.op); got != tc.want {
			t.Errorf("%s on %q: got %v want %v", tc.role, tc.op, got, tc.want)
		}
	}
}

// AC: WHEN canon.yaml grants a role an operation that does not exist THE SYSTEM
// SHALL refuse to start and name it.
func TestRejectsUnknownOperations(t *testing.T) {
	cases := map[string]string{
		"unknown verb":         `roles: [{name: r, can: [teleport]}]`,
		"unknown field":        `roles: [{name: r, can: ["field:storyPoints"]}]`,
		"unknown state":        `roles: [{name: r, can: ["transition:todo->shipped"]}]`,
		"malformed transition": `roles: [{name: r, can: ["transition:todo"]}]`,
		"unknown scope":        `roles: [{name: r, scope: galaxy, can: [create]}]`,
	}
	base := `version: 1
states: [{name: todo, category: open}, {name: done, category: closed}]
transitions: [{from: todo, to: done}]
fields: [{name: title, type: string, required: true}]
issue_types: [{name: task, fields: [title]}]
`
	for name, extra := range cases {
		t.Run(name, func(t *testing.T) {
			path := write(t, base+extra+"\n")
			_, err := Load(path)
			if err == nil {
				t.Fatal("expected rejection")
			}
			// The offending token must appear, or the message is not actionable.
			for _, token := range []string{"teleport", "storyPoints", "shipped", "galaxy"} {
				if strings.Contains(extra, token) && !strings.Contains(err.Error(), token) {
					t.Errorf("error must name %q, got: %v", token, err)
				}
			}
		})
	}
}

func TestRejectsDuplicateAndEmptyRoles(t *testing.T) {
	base := `version: 1
states: [{name: todo, category: open}]
fields: [{name: title, type: string}]
issue_types: [{name: task, fields: [title]}]
`
	for name, extra := range map[string]string{
		"duplicate": `roles: [{name: r, can: [create]}, {name: r, can: [delete]}]`,
		"no name":   `roles: [{can: [create]}]`,
		"no grants": `roles: [{name: r}]`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(write(t, base+extra+"\n")); err == nil {
				t.Error("expected rejection")
			}
		})
	}
}

// Roles are optional: a schema without them still loads, so authorisation can be
// adopted incrementally rather than all at once.
func TestRolesAreOptional(t *testing.T) {
	s, err := Load(write(t, `version: 1
states: [{name: todo, category: open}]
fields: [{name: title, type: string}]
issue_types: [{name: task, fields: [title]}]
`))
	if err != nil {
		t.Fatalf("a schema without roles must load: %v", err)
	}
	if len(s.Roles) != 0 {
		t.Errorf("roles: got %d want 0", len(s.Roles))
	}
	if !s.Unrestricted() {
		t.Error("a schema with no roles must report itself unrestricted")
	}
}
