package enforce

import (
	"testing"

	"github.com/ofenton/canon/internal/event"
)

func find(t *testing.T, rows []Usage, kind, name string) Usage {
	t.Helper()
	for _, r := range rows {
		if r.Kind == kind && r.Name == name {
			return r
		}
	}
	t.Fatalf("no %s named %q in the report", kind, name)
	return Usage{}
}

// AC: WHEN an admin requests a schema report THE SYSTEM SHALL list every field with
// its usage count and last-used date.
func TestUsageCountsIssuesPerField(t *testing.T) {
	e, _ := fixture(t)
	admin := actor("ollie", "admin", "platform")
	for _, id := range []string{"CANON-1", "CANON-2", "CANON-3"} {
		if err := e.CreateAs(admin, id, "story", map[string]string{"title": "t", "priority": "p1"}, "platform", at(0)); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	// One more issue touching a different field, to prove counts are per field.
	if err := e.CreateAs(admin, "CANON-4", "bug", map[string]string{"title": "t", "component": "search"}, "platform", at(0)); err != nil {
		t.Fatalf("create: %v", err)
	}

	rows, err := e.SchemaUsage()
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if got := find(t, rows, "field", "priority").Count; got != 3 {
		t.Errorf("priority used by %d issues, want 3", got)
	}
	if got := find(t, rows, "field", "component").Count; got != 1 {
		t.Errorf("component used by %d issues, want 1", got)
	}
	if find(t, rows, "field", "priority").LastUsed.IsZero() {
		t.Error("a used field must carry a last-used time")
	}
}

// AC: THE SYSTEM SHALL show configuration that has never been used.
func TestUsageShowsUnusedConfiguration(t *testing.T) {
	e, _ := fixture(t)
	admin := actor("ollie", "admin", "platform")
	if err := e.CreateAs(admin, "CANON-1", "story", map[string]string{"title": "t"}, "platform", at(0)); err != nil {
		t.Fatalf("create: %v", err)
	}

	rows, err := e.SchemaUsage()
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	unused := find(t, rows, "field", "component")
	if unused.Used() {
		t.Fatalf("component is used by nothing, got count %d", unused.Count)
	}
	if !unused.LastUsed.IsZero() {
		t.Error("something nothing uses cannot have a last-used time")
	}
	// Unused rows sort first: they are the ones needing a decision.
	if rows[0].Used() {
		t.Error("unused configuration should be listed first")
	}
}

// title and evidence are schema fields the projection promotes out of the Fields map.
// Counting only Fields reported both as unused, which on a schema where every issue
// has a title is the report advising somebody to delete the one required field.
func TestUsageCountsFieldsThePropertyModelPromotes(t *testing.T) {
	e, _ := fixture(t)
	admin := actor("ollie", "admin", "platform")
	if err := e.CreateAs(admin, "CANON-1", "story", map[string]string{"title": "Search is slow"}, "platform", at(0)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := e.TransitionAs(admin, "CANON-1", "in_progress", "", at(1)); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if err := e.TransitionAs(admin, "CANON-1", "in_review", "312 passed in 41s", at(2)); err != nil {
		t.Fatalf("transition: %v", err)
	}

	rows, err := e.SchemaUsage()
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	if got := find(t, rows, "field", "title").Count; got != 1 {
		t.Errorf("title is on every issue but reported %d uses", got)
	}
	if got := find(t, rows, "field", "evidence").Count; got != 1 {
		t.Errorf("evidence was supplied on a transition but reported %d uses", got)
	}
}

// An enum where everything takes one value is a field pretending to be a decision,
// and that is invisible from a count alone.
func TestUsageShowsAnEnumsDistribution(t *testing.T) {
	e, _ := fixture(t)
	admin := actor("ollie", "admin", "platform")
	for i, p := range []string{"p1", "p1", "p2"} {
		id := []string{"CANON-1", "CANON-2", "CANON-3"}[i]
		if err := e.CreateAs(admin, id, "story", map[string]string{"title": "t", "priority": p}, "platform", at(0)); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	rows, _ := e.SchemaUsage()
	detail := find(t, rows, "field", "priority").Detail
	if detail["p1"] != 2 || detail["p2"] != 1 {
		t.Fatalf("expected p1 twice and p2 once, got %v", detail)
	}
	// A free-text field's distinct values are just its contents, so no distribution.
	if d := find(t, rows, "field", "title").Detail; len(d) != 0 {
		t.Fatalf("a string field should carry no distribution, got %v", d)
	}
}

// Teams and roles are declared too, and dead ones are as worth seeing as dead fields.
func TestUsageCoversTeamsAndRoles(t *testing.T) {
	e, _ := fixture(t)
	sys := event.Actor{ID: "bootstrap", Kind: event.ActorSystem}
	if err := e.RegisterActor("sam", event.ActorHuman, "", at(0), sys); err != nil {
		t.Fatal(err)
	}
	if err := e.GrantRole("sam", "member", at(0), sys); err != nil {
		t.Fatal(err)
	}
	admin := actor("ollie", "admin", "platform")
	if err := e.CreateAs(admin, "CANON-1", "story", map[string]string{"title": "t"}, "platform", at(0)); err != nil {
		t.Fatalf("create: %v", err)
	}

	rows, _ := e.SchemaUsage()
	if find(t, rows, "team", "platform").Count != 1 {
		t.Error("the platform team owns an issue and should be counted")
	}
	if find(t, rows, "team", "growth").Used() {
		t.Error("the growth team owns nothing and should read as unused")
	}
	if find(t, rows, "role", "member").Count != 1 {
		t.Error("sam holds member, so the role is in use")
	}
	if find(t, rows, "role", "reporter").Used() {
		t.Error("nobody holds reporter, so it should read as unused")
	}
}
