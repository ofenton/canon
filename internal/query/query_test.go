package query

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/projection"
	"github.com/ofenton/canon/internal/schema"
)

func fixture(t *testing.T) (*schema.Schema, *enforce.Enforcer, *event.Store) {
	t.Helper()
	s, err := schema.Load(filepath.Join("..", "schema", "testdata", "canon.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	log, err := event.Open(filepath.Join(t.TempDir(), "canon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	e := enforce.New(s, log)
	sys := event.Actor{ID: "boot", Kind: event.ActorSystem}
	at := time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)
	if err := e.RegisterActor("ollie", event.ActorHuman, "", at, sys); err != nil {
		t.Fatal(err)
	}
	if err := e.GrantRole("ollie", "admin", at, sys); err != nil {
		t.Fatal(err)
	}
	if err := e.AddToTeam("ollie", "platform", at, sys); err != nil {
		t.Fatal(err)
	}
	return s, e, log
}

func at(min int) time.Time { return time.Date(2026, 8, 23, 10, min, 0, 0, time.UTC) }

// seed builds four issues across two teams, states and priorities.
func seed(t *testing.T, e *enforce.Enforcer) projection.Projection {
	t.Helper()
	p, err := e.Principal("ollie")
	if err != nil {
		t.Fatal(err)
	}
	mk := func(id, team, priority string) {
		if err := e.CreateAs(p, id, "bug",
			map[string]string{"title": "issue " + id, "priority": priority}, team, at(1)); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mk("CANON-1", "platform", "p1")
	mk("CANON-2", "platform", "p3")
	mk("CANON-3", "payments", "p1")
	mk("CANON-4", "payments", "p2")
	if err := e.TransitionAs(p, "CANON-1", "in_progress", "", at(2)); err != nil {
		t.Fatal(err)
	}
	if err := e.TransitionAs(p, "CANON-3", "abandoned", "", at(3)); err != nil {
		t.Fatal(err)
	}
	return projection.Projection{}
}

func view(t *testing.T, log *event.Store) *projection.Projection {
	t.Helper()
	p := projection.New(log)
	if err := p.Rebuild(); err != nil {
		t.Fatal(err)
	}
	return p
}

func ids(issues []*projection.Issue) []string {
	out := make([]string, len(issues))
	for i, issue := range issues {
		out[i] = issue.ID
	}
	return out
}

func TestFiltering(t *testing.T) {
	s, e, log := fixture(t)
	seed(t, e)
	v := view(t, log)

	cases := map[string][]string{
		"":                          {"CANON-1", "CANON-2", "CANON-3", "CANON-4"},
		"team=platform":             {"CANON-1", "CANON-2"},
		"state=todo":                {"CANON-2", "CANON-4"},
		"priority=p1":               {"CANON-1", "CANON-3"},
		"team=platform priority=p1": {"CANON-1"},
		"!team=platform":            {"CANON-3", "CANON-4"},
		"category=closed":           {"CANON-3"},
		"category=open":             {"CANON-2", "CANON-4"},
		"title~canon-2":             {"CANON-2"},
		"team=platform !state=todo": {"CANON-1"},
	}
	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			q, err := Parse(raw, s)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := ids(q.Filter(v, s))
			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("got %v want %v", got, want)
			}
		})
	}
}

// AC: WHEN a query references a field not in canon.yaml THE SYSTEM SHALL reject it
// naming the valid fields.
func TestRejectsUnknownKeysAndValues(t *testing.T) {
	s, _, _ := fixture(t)
	cases := map[string]string{
		"storyPoints=8":     "storyPoints",
		"sprint=4":          "sprint",
		"state=shipped":     "shipped",
		"category=sideways": "sideways",
		"priority=urgent":   "urgent",
		"team":              "no value",
		"=platform":         "no key",
	}
	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			_, err := Parse(raw, s)
			if err == nil {
				t.Fatal("expected rejection")
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error should mention %q, got: %v", want, err)
			}
		})
	}

	// The error must list what is valid, or the user has to guess.
	_, err := Parse("storyPoints=8", s)
	for _, valid := range []string{"priority", "state", "team", "category"} {
		if !strings.Contains(err.Error(), valid) {
			t.Errorf("error must list valid keys; %q missing from: %v", valid, err)
		}
	}
}

// AC: THE SYSTEM SHALL express a board as a saved query and a grouping key with no
// state of its own.
// AC: WHEN an issue stops matching a board's query THE SYSTEM SHALL cease to show it
// with no separate update.
func TestBoardMembershipFollowsTheData(t *testing.T) {
	s, e, log := fixture(t)
	seed(t, e)
	p, _ := e.Principal("ollie")

	q, err := Parse("team=platform", s)
	if err != nil {
		t.Fatal(err)
	}

	order, buckets := Group(q.Filter(view(t, log), s), "state", s)
	if strings.Join(order, ",") != "todo,in_progress" {
		t.Errorf("columns should follow the schema's state order, got %v", order)
	}
	if got := ids(buckets["in_progress"]); strings.Join(got, ",") != "CANON-1" {
		t.Errorf("in_progress: got %v", got)
	}

	// Move an issue out of the query's scope. Nothing updates the board.
	if err := e.SetFieldAs(p, "CANON-1", "component", "search", at(4)); err != nil {
		t.Fatal(err)
	}
	if err := e.TransitionAs(p, "CANON-1", "abandoned", "", at(5)); err != nil {
		t.Fatal(err)
	}

	order, buckets = Group(q.Filter(view(t, log), s), "state", s)
	if _, still := buckets["in_progress"]; still {
		t.Error("the issue must leave its column with no board update")
	}
	if got := ids(buckets["abandoned"]); strings.Join(got, ",") != "CANON-1" {
		t.Errorf("abandoned column: got %v", got)
	}
	_ = order

	// And out of the query entirely.
	teamQ, _ := Parse("team=platform category=active", s)
	if got := ids(teamQ.Filter(view(t, log), s)); len(got) != 0 {
		t.Errorf("no platform issue is active any more, got %v", got)
	}
}

func TestGroupingKeys(t *testing.T) {
	s, e, log := fixture(t)
	seed(t, e)
	v := view(t, log)
	all, _ := Parse("", s)

	order, buckets := Group(all.Filter(v, s), "team", s)
	if strings.Join(order, ",") != "payments,platform" {
		t.Errorf("non-state grouping should sort alphabetically, got %v", order)
	}
	if len(buckets["platform"]) != 2 {
		t.Errorf("platform bucket: got %d", len(buckets["platform"]))
	}

	// A field with no value must not silently vanish.
	_, buckets = Group(all.Filter(v, s), "component", s)
	if len(buckets["(none)"]) != 4 {
		t.Errorf("issues with no component should bucket under (none), got %d", len(buckets["(none)"]))
	}

	keys := GroupKeys(s)
	for _, want := range []string{"state", "team", "priority", "component"} {
		var found bool
		for _, k := range keys {
			if k == want {
				found = true
			}
		}
		if !found {
			t.Errorf("group key %q missing from %v", want, keys)
		}
	}
}

// A board holds no membership, so there is nothing to store and nothing to go stale.
func TestBoardsHaveNoStoredMembership(t *testing.T) {
	s, e, log := fixture(t)
	seed(t, e)
	before, err := log.Count()
	if err != nil {
		t.Fatal(err)
	}
	q, _ := Parse("team=platform", s)
	Group(q.Filter(view(t, log), s), "state", s)
	after, _ := log.Count()
	if after != before {
		t.Errorf("rendering a board wrote %d events; a board must be a pure view", after-before)
	}
	_ = e
}

// AC: WHEN a query names an ancestor THE SYSTEM SHALL return every issue beneath it
// at any depth.
func TestAncestorQuery(t *testing.T) {
	s, e, log := fixture(t)
	p, err := e.Principal("ollie")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"EPIC", "STORY", "SUB", "OTHER"} {
		if err := e.CreateAs(p, id, "task", map[string]string{"title": id}, "platform", at(1)); err != nil {
			t.Fatal(err)
		}
	}
	for _, link := range [][2]string{{"STORY", "EPIC"}, {"SUB", "STORY"}} {
		if err := e.Reparent(link[0], link[1], at(2), event.Actor{ID: "ollie", Kind: event.ActorHuman}); err != nil {
			t.Fatal(err)
		}
	}
	v := view(t, log)

	cases := map[string]string{
		"ancestor=EPIC":            "STORY,SUB",
		"ancestor=STORY":           "SUB",
		"ancestor=SUB":             "",
		"ancestor=NOPE":            "",
		"!ancestor=EPIC":           "EPIC,OTHER",
		"ancestor=EPIC state=todo": "STORY,SUB",
	}
	for raw, want := range cases {
		t.Run(raw, func(t *testing.T) {
			q, err := Parse(raw, s)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			got := ids(q.Filter(v, s))
			if strings.Join(got, ",") != want {
				t.Errorf("got %v want %q", got, want)
			}
		})
	}
}

// AC: WHEN a query names blocked THE SYSTEM SHALL return issues whose dependencies
// are not all closed.
func TestBlockedAndDependsOnQueries(t *testing.T) {
	s, e, log := fixture(t)
	p, err := e.Principal("ollie")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"API", "UI", "DOCS", "FREE"} {
		if err := e.CreateAs(p, id, "task", map[string]string{"title": id}, "platform", at(1)); err != nil {
			t.Fatal(err)
		}
	}
	for _, dep := range [][2]string{{"UI", "API"}, {"DOCS", "API"}} {
		if _, err := e.AddDependency(p, dep[0], dep[1], at(2)); err != nil {
			t.Fatal(err)
		}
	}

	check := func(raw, want string) {
		t.Helper()
		q, err := Parse(raw, s)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		got := ids(q.Filter(view(t, log), s))
		if strings.Join(got, ",") != want {
			t.Errorf("%s = %v want %q", raw, got, want)
		}
	}

	check("blocked=true", "DOCS,UI")
	check("blocked=false", "API,FREE")
	check("depends_on=API", "DOCS,UI")
	check("!depends_on=API", "API,FREE")
	check("blocked=true depends_on=API", "DOCS,UI")

	// Closing the blocker changes the answer with nothing else written.
	for _, to := range []string{"in_progress", "in_review", "done"} {
		evidence := ""
		if to == "in_review" {
			evidence = "ok"
		}
		if err := e.TransitionAs(p, "API", to, evidence, at(3)); err != nil {
			t.Fatal(err)
		}
	}
	check("blocked=true", "")
	check("blocked=false", "API,DOCS,FREE,UI")

	if _, err := Parse("blocked=maybe", s); err == nil {
		t.Error("blocked must take true or false")
	}
}
