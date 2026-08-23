package enforce

import (
	"strings"
	"testing"

	"github.com/ofenton/canon/internal/event"
)

func sys() event.Actor { return event.Actor{ID: "bootstrap", Kind: event.ActorSystem} }

// AC: THE SYSTEM SHALL record actor identities and team membership as events in the
// log, not in canon.yaml.
func TestRegistryLivesInTheLog(t *testing.T) {
	e, log := fixture(t)

	if err := e.RegisterActor("ollie", event.ActorHuman, "", at(0), sys()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := e.GrantRole("ollie", "admin", at(1), sys()); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := e.AddToTeam("ollie", "platform", at(2), sys()); err != nil {
		t.Fatalf("add to team: %v", err)
	}

	types := map[string]int{}
	events, err := log.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range events {
		types[ev.Type]++
	}
	for _, want := range []string{"actor.registered", "actor.role_granted", "team.member_added"} {
		if types[want] != 1 {
			t.Errorf("expected one %s event, got %d", want, types[want])
		}
	}

	p, err := e.Principal("ollie")
	if err != nil {
		t.Fatalf("resolve principal: %v", err)
	}
	if len(p.Roles) != 1 || p.Roles[0] != "admin" {
		t.Errorf("roles: got %v want [admin]", p.Roles)
	}
	if len(p.Teams) != 1 || p.Teams[0] != "platform" {
		t.Errorf("teams: got %v want [platform]", p.Teams)
	}
	if p.Actor.Kind != event.ActorHuman {
		t.Errorf("kind: got %q want human", p.Actor.Kind)
	}
}

// AC: WHEN an actor is granted a role THE SYSTEM SHALL apply it to subsequent writes
// without a restart.
func TestRoleGrantAppliesImmediately(t *testing.T) {
	e, _ := fixture(t)
	register(t, e, "ollie", "admin", "platform")

	if err := e.RegisterActor("sam", event.ActorHuman, "", at(3), sys()); err != nil {
		t.Fatal(err)
	}
	sam, err := e.Principal("sam")
	if err != nil {
		t.Fatal(err)
	}
	// No role yet, so a write must be refused.
	if err := e.CreateAs(sam, "CANON-2", "task", map[string]string{"title": "x"}, "platform", at(4)); err == nil {
		t.Fatal("an actor with no role must not write")
	}

	if err := e.GrantRole("sam", "member", at(5), sys()); err != nil {
		t.Fatal(err)
	}
	if err := e.AddToTeam("sam", "platform", at(6), sys()); err != nil {
		t.Fatal(err)
	}
	sam, err = e.Principal("sam")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.CreateAs(sam, "CANON-2", "task", map[string]string{"title": "x"}, "platform", at(7)); err != nil {
		t.Errorf("the grant must apply without a restart: %v", err)
	}
}

// AC: WHEN an unregistered actor attempts a write THE SYSTEM SHALL reject it naming
// the actor.
func TestUnregisteredActorIsRejected(t *testing.T) {
	e, _ := fixture(t)
	register(t, e, "ollie", "admin", "platform")

	_, err := e.Principal("ghost")
	if err == nil {
		t.Fatal("resolving an unregistered actor must fail")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error must name the actor, got: %v", err)
	}
}

// AC: WHEN membership changes THE SYSTEM SHALL retain the prior membership in the
// log, so past events remain explicable.
func TestMembershipHistoryIsRetained(t *testing.T) {
	e, log := fixture(t)
	register(t, e, "sam", "member", "platform")

	if err := e.RemoveFromTeam("sam", "platform", at(8), sys()); err != nil {
		t.Fatalf("remove: %v", err)
	}
	sam, err := e.Principal("sam")
	if err != nil {
		t.Fatal(err)
	}
	if len(sam.Teams) != 0 {
		t.Errorf("teams after removal: got %v want none", sam.Teams)
	}

	// The removal must be an added fact, not an erased one.
	events, err := log.All()
	if err != nil {
		t.Fatal(err)
	}
	var added, removed bool
	for _, ev := range events {
		switch ev.Type {
		case "team.member_added":
			added = true
		case "team.member_removed":
			removed = true
		}
	}
	if !added || !removed {
		t.Errorf("both the join and the leave must remain in the log (added=%v removed=%v)", added, removed)
	}
}

func TestRevokingARoleTakesEffect(t *testing.T) {
	e, _ := fixture(t)
	register(t, e, "ollie", "admin", "platform")
	register(t, e, "sam", "member", "platform")

	ollie, _ := e.Principal("ollie")
	if err := e.CreateAs(ollie, "CANON-1", "task", map[string]string{"title": "x"}, "platform", at(9)); err != nil {
		t.Fatal(err)
	}

	sam, _ := e.Principal("sam")
	if err := e.SetFieldAs(sam, "CANON-1", "priority", "p1", at(10)); err != nil {
		t.Fatalf("member may set a field: %v", err)
	}
	if err := e.RevokeRole("sam", "member", at(11), sys()); err != nil {
		t.Fatal(err)
	}
	sam, _ = e.Principal("sam")
	if err := e.SetFieldAs(sam, "CANON-1", "priority", "p2", at(12)); err == nil {
		t.Error("a revoked role must stop permitting writes")
	}
}

func TestRejectsUnknownRoleGrant(t *testing.T) {
	e, _ := fixture(t)
	if err := e.RegisterActor("sam", event.ActorHuman, "", at(0), sys()); err != nil {
		t.Fatal(err)
	}
	err := e.GrantRole("sam", "wizard", at(1), sys())
	if err == nil || !strings.Contains(err.Error(), "wizard") {
		t.Errorf("granting an undefined role must be refused naming it, got: %v", err)
	}
}

func TestRejectsDuplicateRegistration(t *testing.T) {
	e, _ := fixture(t)
	if err := e.RegisterActor("sam", event.ActorHuman, "", at(0), sys()); err != nil {
		t.Fatal(err)
	}
	if err := e.RegisterActor("sam", event.ActorHuman, "", at(1), sys()); err == nil {
		t.Error("registering the same actor twice must be refused")
	}
}

func TestAgentActorsMustDeclareAModel(t *testing.T) {
	e, _ := fixture(t)
	if err := e.RegisterActor("agent:one", event.ActorAgent, "", at(0), sys()); err == nil {
		t.Error("an agent actor must declare a model")
	}
	if err := e.RegisterActor("agent:one", event.ActorAgent, "claude-opus-5", at(1), sys()); err != nil {
		t.Errorf("a model-bearing agent must register: %v", err)
	}
	p, err := e.Principal("agent:one")
	if err != nil {
		t.Fatal(err)
	}
	if p.Actor.Model != "claude-opus-5" {
		t.Errorf("model: got %q", p.Actor.Model)
	}
}

// Rebuilding must reproduce identical registry state, exactly as issues do.
func TestRegistryRebuildsDeterministically(t *testing.T) {
	e, log := fixture(t)
	register(t, e, "ollie", "admin", "platform")
	register(t, e, "sam", "member", "payments")
	if err := e.RemoveFromTeam("sam", "payments", at(20), sys()); err != nil {
		t.Fatal(err)
	}

	first, err := e.Principal("sam")
	if err != nil {
		t.Fatal(err)
	}
	fresh := New(e.Schema(), log)
	second, err := fresh.Principal("sam")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(first.Roles, ",") != strings.Join(second.Roles, ",") ||
		strings.Join(first.Teams, ",") != strings.Join(second.Teams, ",") {
		t.Errorf("registry not deterministic: %+v vs %+v", first, second)
	}
}

func register(t *testing.T, e *Enforcer, id, role, team string) {
	t.Helper()
	kind, model := event.ActorHuman, ""
	if strings.HasPrefix(id, "agent:") {
		kind, model = event.ActorAgent, "claude-opus-5"
	}
	if err := e.RegisterActor(id, kind, model, at(0), sys()); err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
	if err := e.GrantRole(id, role, at(0), sys()); err != nil {
		t.Fatalf("grant %s: %v", id, err)
	}
	if team != "" {
		if err := e.AddToTeam(id, team, at(0), sys()); err != nil {
			t.Fatalf("team %s: %v", id, err)
		}
	}
}
