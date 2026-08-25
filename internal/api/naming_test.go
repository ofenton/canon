package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// snakeCase is what every hand-written response in this package already uses:
// depends_on, linked_by, requires_checklist. The projection's structs carried Go
// field names into JSON, so GET /api/issues returned ID, Title and State while every
// other route returned snake_case — an inconsistency any client trips over exactly
// once, then works around forever.
var snakeCase = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// AC: THE SYSTEM SHALL name every JSON field in snake_case across every route.
//
// This walks real responses rather than inspecting struct tags, so a field reaching
// the wire through a map, an embedded type or a hand-built object is covered the same
// way as a tagged one.
func TestEveryJSONKeyIsSnakeCase(t *testing.T) {
	_, h := newServer(t)

	// Build something with every shape on it: fields, a checklist, a multi-value, a
	// parent, a dependency, a transition and a linked commit.
	seed := []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/issues", map[string]any{"id": "CANON-1", "title": "one", "team": "platform", "type": "story"}},
		{"POST", "/api/issues", map[string]any{"id": "CANON-2", "title": "two", "team": "platform", "type": "task"}},
		{"PATCH", "/api/issues/CANON-1/fields", map[string]any{"priority": "p1"}},
		{"PUT", "/api/issues/CANON-1/multi/kpi", map[string]any{"values": []string{"conversion"}}},
		{"PUT", "/api/issues/CANON-1/checklist/acceptance", map[string]any{"text": "a criterion"}},
		{"PUT", "/api/issues/CANON-2/parent", map[string]any{"parent": "CANON-1"}},
		{"PUT", "/api/issues/CANON-1/dependencies", map[string]any{"on": "CANON-2"}},
		{"POST", "/api/issues/CANON-1/transition", map[string]any{"to": "in_progress"}},
		{"PUT", "/api/issues/CANON-1/commits", map[string]any{"sha": "a1b2c3d", "message": "Reindex"}},
		{"POST", "/api/boards", map[string]any{"name": "platform", "query": "team=platform", "group_by": "state"}},
	}
	for _, s := range seed {
		if rec := do(t, h, "ollie", s.method, s.path, s.body); rec.Code >= 300 {
			t.Fatalf("seeding %s %s: %d — %s", s.method, s.path, rec.Code, rec.Body)
		}
	}

	reads := []string{
		"/api/schema", "/api/schema/usage", "/api/products", "/api/events", "/api/issues", "/api/issues/CANON-1",
		"/api/issues/CANON-1/children", "/api/issues/CANON-2/ancestors",
		"/api/issues/CANON-1/tree", "/api/issues/CANON-1/dependencies",
		"/api/issues/CANON-1/commits", "/api/cycles", "/api/metrics",
		"/api/boards", "/api/boards/platform", "/api/actors", "/api/actors/ollie",
		"/api/proposals",
	}
	for _, path := range reads {
		rec := do(t, h, "ollie", "GET", path, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: %d — %s", path, rec.Code, rec.Body)
		}
		var body any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		for _, key := range badKeys(body, nil, false) {
			t.Errorf("GET %s returns %q, which is not snake_case", path, key)
		}
	}
}

// badKeys walks a decoded response and returns every key that is not snake_case.
//
// Keys an organisation chose are skipped: a field somebody calls "storyPoints" is
// their business, and this test is about the shape Canon picks, not the shape it is
// handed. Which keys those are is decided by shape, not by name — see userNamed.
func badKeys(v any, path []string, skip bool) []string {
	var bad []string
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if !skip && !snakeCase.MatchString(k) {
				bad = append(bad, strings.Join(append(path, k), "."))
			}
			bad = append(bad, badKeys(child, append(path, k), userNamed(k, child))...)
		}
	case []any:
		for _, child := range t {
			bad = append(bad, badKeys(child, path, skip)...)
		}
	}
	return bad
}

// userNamed reports whether a value's keys were chosen by the organisation.
//
// The name alone is not enough: at /api/schema, "fields" is a list of Canon's own
// field *definitions*, and at an issue it is a map keyed by whatever the org called
// its fields. Only the map form is user data. Testing the name alone let
// {"Name":..., "Type":...} through on the schema endpoint, which is exactly the leak
// this test exists to catch — found by the UI breaking after the rename.
func userNamed(key string, value any) bool {
	if _, isObject := value.(map[string]any); !isObject {
		return false
	}
	switch key {
	case "fields", "multi", "checklists", "payload", "counts", "by_state":
		return true
	}
	return false
}
