package enforce

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

func actor(id, role, team string) Principal {
	kind := event.ActorHuman
	model := ""
	if strings.HasPrefix(id, "agent:") {
		kind, model = event.ActorAgent, "claude-opus-5"
	}
	return Principal{
		Actor: event.Actor{ID: id, Kind: kind, Model: model},
		Roles: []string{role},
		Teams: []string{team},
	}
}

// AC: WHEN an actor attempts an operation their role does not permit THE SYSTEM
// SHALL reject it and name the roles that would permit it.
func TestDeniedOperationNamesPermittingRoles(t *testing.T) {
	e, _ := fixture(t)
	admin := actor("ollie", "admin", "platform")
	if err := e.CreateAs(admin, "CANON-1", "task", map[string]string{"title": "x"}, "platform", at(0)); err != nil {
		t.Fatalf("admin create: %v", err)
	}

	reporter := actor("sam", "reporter", "platform")
	err := e.DeleteAs(reporter, "CANON-1", at(1))
	if err == nil {
		t.Fatal("a reporter must not delete")
	}
	msg := err.Error()
	if !strings.Contains(msg, "reporter") {
		t.Errorf("error must name the actor's role, got: %v", err)
	}
	if !strings.Contains(msg, "admin") {
		t.Errorf("error must name a role that would permit it, got: %v", err)
	}
}

func TestPermittedOperationsSucceed(t *testing.T) {
	e, _ := fixture(t)
	admin := actor("ollie", "admin", "platform")
	if err := e.CreateAs(admin, "CANON-1", "task", map[string]string{"title": "x"}, "platform", at(0)); err != nil {
		t.Fatalf("create: %v", err)
	}
	member := actor("sam", "member", "platform")
	if err := e.SetFieldAs(member, "CANON-1", "priority", "p1", at(1)); err != nil {
		t.Errorf("member may set fields on their team's issue: %v", err)
	}
	if err := e.TransitionAs(member, "CANON-1", "in_progress", "", at(2)); err != nil {
		t.Errorf("member may transition their team's issue: %v", err)
	}
}

// AC: WHEN a role is declared scope: team THE SYSTEM SHALL permit its operations
// only on issues owned by a team that actor belongs to.
func TestTeamScopeIsEnforced(t *testing.T) {
	e, _ := fixture(t)
	admin := actor("ollie", "admin", "platform")
	if err := e.CreateAs(admin, "PLAT-1", "task", map[string]string{"title": "ours"}, "platform", at(0)); err != nil {
		t.Fatal(err)
	}
	if err := e.CreateAs(admin, "PAY-1", "task", map[string]string{"title": "theirs"}, "payments", at(1)); err != nil {
		t.Fatal(err)
	}

	member := actor("sam", "member", "platform")
	if err := e.SetFieldAs(member, "PLAT-1", "priority", "p1", at(2)); err != nil {
		t.Errorf("own team's issue must be permitted: %v", err)
	}
	err := e.SetFieldAs(member, "PAY-1", "priority", "p1", at(3))
	if err == nil {
		t.Fatal("another team's issue must be refused")
	}
	if !strings.Contains(err.Error(), "payments") {
		t.Errorf("error must name the owning team, got: %v", err)
	}

	// An unscoped role crosses teams freely.
	if err := e.SetFieldAs(admin, "PAY-1", "priority", "p2", at(4)); err != nil {
		t.Errorf("an org-scoped role must cross teams: %v", err)
	}
}

// Denied writes must not reach the log, exactly as schema rejections do not.
func TestDeniedWritesAppendNothing(t *testing.T) {
	e, log := fixture(t)
	admin := actor("ollie", "admin", "platform")
	if err := e.CreateAs(admin, "CANON-1", "task", map[string]string{"title": "x"}, "platform", at(0)); err != nil {
		t.Fatal(err)
	}
	before, _ := log.Count()

	reporter := actor("sam", "reporter", "platform")
	_ = e.DeleteAs(reporter, "CANON-1", at(1))
	_ = e.TransitionAs(reporter, "CANON-1", "in_progress", "", at(2))
	_ = e.SetFieldAs(reporter, "CANON-1", "priority", "p1", at(3))

	after, _ := log.Count()
	if after != before {
		t.Errorf("denied writes appended %d events", after-before)
	}
}

// An agent's refusals are proposals, not denials — feat-007 acts on them, but the
// decision is made here and must be distinguishable now.
func TestAgentRefusalIsAProposal(t *testing.T) {
	e, _ := fixture(t)
	admin := actor("ollie", "admin", "platform")
	if err := e.CreateAs(admin, "CANON-1", "task", map[string]string{"title": "x"}, "platform", at(0)); err != nil {
		t.Fatal(err)
	}
	agent := actor("agent:one", "agent", "platform")

	// Granted outright.
	if err := e.TransitionAs(agent, "CANON-1", "in_progress", "", at(1)); err != nil {
		t.Fatalf("agent may start work: %v", err)
	}
	// in_review requires evidence and is granted; supply it.
	if err := e.TransitionAs(agent, "CANON-1", "in_review", "312 passed", at(2)); err != nil {
		t.Fatalf("agent may move to review with evidence: %v", err)
	}
	// in_review -> done is only in the agent's propose list.
	err := e.TransitionAs(agent, "CANON-1", "done", "", at(3))
	if err == nil {
		t.Fatal("agent must not complete work outright")
	}
	var proposal *ProposalRequired
	if !AsProposalRequired(err, &proposal) {
		t.Fatalf("agent refusal must be a proposal, got a plain denial: %v", err)
	}
	if proposal.Operation != schema.TransitionOp("in_review", "done") {
		t.Errorf("proposal operation: got %q", proposal.Operation)
	}
}

// AC: THE SYSTEM SHALL expose no runtime interface for creating or altering a role.
func TestNoRuntimeRoleMutation(t *testing.T) {
	forbidden := []string{"AddRole", "GrantPermission", "CreateRole", "SetRole", "Grant"}
	fset := token.NewFileSet()
	for _, root := range []string{".", filepath.Join("..", "schema"), filepath.Join("..", "..", "cmd", "canon")} {
		pkgs, err := parser.ParseDir(fset, root, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", root, err)
		}
		for _, pkg := range pkgs {
			for name, file := range pkg.Files {
				if strings.HasSuffix(name, "_test.go") {
					continue
				}
				ast.Inspect(file, func(n ast.Node) bool {
					fn, ok := n.(*ast.FuncDecl)
					if !ok {
						return true
					}
					for _, bad := range forbidden {
						if fn.Name.Name == bad {
							t.Errorf("%s defines %s: roles must not be mutable at runtime", name, bad)
						}
					}
					return true
				})
			}
		}
	}
}

// A schema with no roles must keep working, so authorisation can be adopted later.
func TestUnrestrictedSchemaPermitsEverything(t *testing.T) {
	s, err := schema.Load(filepath.Join("..", "schema", "testdata", "canon.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_ = s
	e, _ := fixtureWithoutRoles(t)
	anyone := Principal{Actor: event.Actor{ID: "nobody", Kind: event.ActorHuman}}
	if err := e.CreateAs(anyone, "CANON-1", "task", map[string]string{"title": "x"}, "", at(0)); err != nil {
		t.Errorf("an unrestricted schema must permit anyone: %v", err)
	}
	if err := e.DeleteAs(anyone, "CANON-1", at(1)); err != nil {
		t.Errorf("an unrestricted schema must permit delete: %v", err)
	}
}
