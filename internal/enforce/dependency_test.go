package enforce

import (
	"strings"
	"testing"
)

func deps(t *testing.T, e *Enforcer, ids ...string) Principal {
	t.Helper()
	admin := adminPrincipal(t, e)
	for _, id := range ids {
		if err := e.CreateAs(admin, id, "task", map[string]string{"title": id}, "platform", at(1)); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	return admin
}

func mustDepend(t *testing.T, e *Enforcer, p Principal, id, on string) DependencyResult {
	t.Helper()
	res, err := e.AddDependency(p, id, on, at(2))
	if err != nil {
		t.Fatalf("%s depends on %s: %v", id, on, err)
	}
	return res
}

// AC: THE SYSTEM SHALL record that one issue depends on another, as a single
// directed relation with no other relation types.
func TestDependencyIsOneDirectedRelation(t *testing.T) {
	e, log := fixture(t)
	p := deps(t, e, "A", "B")
	mustDepend(t, e, p, "A", "B")

	got, err := e.DependenciesOf("A")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.DependsOn, ",") != "B" {
		t.Errorf("A depends on %v, want [B]", got.DependsOn)
	}
	if len(got.Dependents) != 0 {
		t.Errorf("A has dependents %v, want none", got.Dependents)
	}

	// One event type, one direction. No blocks/relates-to/duplicates vocabulary.
	events, err := log.All()
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, ev := range events {
		if strings.Contains(ev.Type, "depend") || strings.Contains(ev.Type, "block") ||
			strings.Contains(ev.Type, "relate") {
			kinds = append(kinds, ev.Type)
		}
	}
	if strings.Join(kinds, ",") != "issue.dependency_added" {
		t.Errorf("relation event types: %v — there must be exactly one", kinds)
	}
}

// AC: WHEN an operator requests an issue's dependencies THE SYSTEM SHALL return both
// what it depends on and what depends on it.
func TestReverseLookup(t *testing.T) {
	e, _ := fixture(t)
	p := deps(t, e, "API", "UI", "DOCS", "UNRELATED")
	mustDepend(t, e, p, "UI", "API")
	mustDepend(t, e, p, "DOCS", "API")

	api, err := e.DependenciesOf("API")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(api.Dependents, ",") != "DOCS,UI" {
		t.Errorf("what depends on API: %v, want [DOCS UI]", api.Dependents)
	}
	if len(api.DependsOn) != 0 {
		t.Errorf("API depends on %v, want none", api.DependsOn)
	}

	ui, _ := e.DependenciesOf("UI")
	if strings.Join(ui.DependsOn, ",") != "API" {
		t.Errorf("UI depends on %v", ui.DependsOn)
	}
}

// AC: WHEN a dependency would create a cycle THE SYSTEM SHALL record it and report a
// warning naming the cycle, rather than refusing the write.
func TestCyclesAreRecordedAndWarnedAbout(t *testing.T) {
	e, log := fixture(t)
	p := deps(t, e, "A", "B", "C")
	mustDepend(t, e, p, "A", "B")
	mustDepend(t, e, p, "B", "C")

	before, _ := log.Count()
	res := mustDepend(t, e, p, "C", "A") // closes the loop
	after, _ := log.Count()

	if after != before+1 {
		t.Error("a cycle-creating dependency must still be recorded")
	}
	if len(res.Cycle) == 0 {
		t.Fatal("creating a cycle must report it")
	}
	warning := res.Warning()
	for _, id := range []string{"A", "B", "C"} {
		if !strings.Contains(warning, id) {
			t.Errorf("the warning must name every member; %q missing from %q", id, warning)
		}
	}
	// The warning must explain the consequence, not just report the fact.
	if !strings.Contains(warning, "can start until") {
		t.Errorf("the warning should say what a cycle means for the work, got %q", warning)
	}

	// And it must keep being reported, not just at the moment of creation.
	for _, id := range []string{"A", "B", "C"} {
		got, err := e.DependenciesOf(id)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Cycles) == 0 {
			t.Errorf("%s is in a cycle but does not report one", id)
		}
	}

	// One loop, reported once, not once per member.
	cycles, err := e.Cycles()
	if err != nil {
		t.Fatal(err)
	}
	if len(cycles) != 1 {
		t.Errorf("got %d cycles, want 1: %v", len(cycles), cycles)
	}
}

func TestSelfDependencyIsRefused(t *testing.T) {
	e, _ := fixture(t)
	p := deps(t, e, "A")
	if _, err := e.AddDependency(p, "A", "A", at(2)); err == nil {
		t.Error("an issue depending on itself is meaningless and must be refused")
	}
}

func TestDuplicateAndUnknownAreRefused(t *testing.T) {
	e, _ := fixture(t)
	p := deps(t, e, "A", "B")
	mustDepend(t, e, p, "A", "B")

	if _, err := e.AddDependency(p, "A", "B", at(3)); err == nil {
		t.Error("adding the same dependency twice must be refused")
	}
	if _, err := e.AddDependency(p, "A", "GHOST", at(3)); err == nil {
		t.Error("depending on an unknown issue must be refused")
	}
	if err := e.RemoveDependency(p, "A", "GHOST", at(3)); err == nil {
		t.Error("removing a dependency that does not exist must be refused")
	}
}

// AC: THE SYSTEM SHALL derive whether an issue is blocked from whether any issue it
// depends on is not closed.
func TestBlockedIsDerived(t *testing.T) {
	e, _ := fixture(t)
	p := deps(t, e, "API", "UI")
	mustDepend(t, e, p, "UI", "API")

	blocked, by := e.IsBlocked("UI")
	if !blocked || strings.Join(by, ",") != "API" {
		t.Fatalf("UI should be blocked by API, got %v %v", blocked, by)
	}
	if blocked, _ := e.IsBlocked("API"); blocked {
		t.Error("API depends on nothing and must not be blocked")
	}

	// Closing the dependency unblocks it, with nothing else written.
	for _, to := range []string{"in_progress", "in_review", "done"} {
		evidence := ""
		if to == "in_review" {
			evidence = "ok"
		}
		if err := e.TransitionAs(p, "API", to, evidence, at(4)); err != nil {
			t.Fatalf("API -> %s: %v", to, err)
		}
	}
	if blocked, by := e.IsBlocked("UI"); blocked {
		t.Errorf("UI should be unblocked once API closed, still blocked by %v", by)
	}
}

// A dependency on a deleted issue stops blocking, but the relation stays in the log.
func TestDependencyOnADeletedIssue(t *testing.T) {
	e, _ := fixture(t)
	p := deps(t, e, "A", "B")
	mustDepend(t, e, p, "A", "B")
	if err := e.DeleteAs(p, "B", at(5)); err != nil {
		t.Fatal(err)
	}
	if blocked, by := e.IsBlocked("A"); blocked {
		t.Errorf("a deleted dependency must not block, still blocked by %v", by)
	}
	got, _ := e.DependenciesOf("A")
	if strings.Join(got.DependsOn, ",") != "B" {
		t.Errorf("the relation should remain recorded, got %v", got.DependsOn)
	}
}

func TestRemoveDependencyClearsTheCycle(t *testing.T) {
	e, _ := fixture(t)
	p := deps(t, e, "A", "B")
	mustDepend(t, e, p, "A", "B")
	res := mustDepend(t, e, p, "B", "A")
	if len(res.Cycle) == 0 {
		t.Fatal("expected a cycle")
	}
	if err := e.RemoveDependency(p, "B", "A", at(6)); err != nil {
		t.Fatal(err)
	}
	cycles, _ := e.Cycles()
	if len(cycles) != 0 {
		t.Errorf("removing the edge should clear the cycle, got %v", cycles)
	}
}

// Dependencies must survive a rebuild like anything else.
func TestDependenciesSurviveRebuild(t *testing.T) {
	e, log := fixture(t)
	p := deps(t, e, "A", "B", "C")
	mustDepend(t, e, p, "A", "B")
	mustDepend(t, e, p, "B", "C")
	mustDepend(t, e, p, "C", "A")

	first, err := e.Projection()
	if err != nil {
		t.Fatal(err)
	}
	digest := first.Snapshot()

	fresh := New(e.Schema(), log)
	second, err := fresh.Projection()
	if err != nil {
		t.Fatal(err)
	}
	if second.Snapshot() != digest {
		t.Error("dependencies are not part of the deterministic projection")
	}
	if len(second.DependencyCycles()) != 1 {
		t.Error("the cycle did not survive a rebuild")
	}
}

// An issue with no dependencies must return empty lists, not nulls. A client that
// has to handle both null and [] for "nothing here" is doing the server's work.
func TestEmptyDependenciesAreEmptyNotNull(t *testing.T) {
	e, _ := fixture(t)
	deps(t, e, "LONE")
	got, err := e.DependenciesOf("LONE")
	if err != nil {
		t.Fatal(err)
	}
	if got.DependsOn == nil || got.Dependents == nil || got.BlockedBy == nil {
		t.Errorf("nil slice in %+v; every list must be empty rather than null", got)
	}
}
