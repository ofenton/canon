package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/catalogue"
)

var fixedTime = time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

const ledger = `# Increment plan

## feat-001: Reindex on write

- **Type:** feature
- **Status:** done
- **Scope:** Reindex.
- **Test Strategy:**
  - Unit
- **Rollback Plan:** Revert
- **Risk:** Low
- **Evidence:** shipped

## feat-002: Cache the plan

- **Type:** feature
- **Status:** in-progress
- **Scope:** Cache.
- **Test Strategy:**
  - Unit
- **Rollback Plan:** Revert
- **Risk:** Low

## Sequencing
`

const spec = `# Widgets

## Problem

Widgets are hard to find.
`

// server builds a server over a real repository, because everything here is derived
// from git and a fixture of git's output would test the fixture.
func server(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "widgets")
	if err := os.MkdirAll(filepath.Join(repo, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "specs", "increment-plan.md"), []byte(ledger), 0o644)
	os.WriteFile(filepath.Join(repo, "specs", "product.md"), []byte(spec), 0o644)

	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "t@e.com"},
		{"config", "user.name", "T"},
		{"add", "."},
		{"commit", "-q", "-m", "plan"},
	} {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_DATE=2026-08-20T09:00:00Z", "GIT_COMMITTER_DATE=2026-08-20T09:00:00Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	c := catalogue.New()
	c.Refresh([]string{repo}, func() time.Time { return fixedTime })
	s := New(c, func() time.Time { return fixedTime })
	return s, s.Handler()
}

func get(t *testing.T, h http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec, body
}

// AC: THE SYSTEM SHALL expose no route that writes an issue.
//
// The guard for this whole increment. Structural rather than a list of things not to
// do: a new write route fails here without anybody remembering to add a case.
func TestNoWriteRoutes(t *testing.T) {
	s, _ := server(t)
	for pattern := range s.Routes() {
		method, path, ok := strings.Cut(pattern, " ")
		if !ok {
			t.Fatalf("route %q has no method", pattern)
		}
		if method != http.MethodGet {
			t.Errorf("route %q is a %s; Canon derives its data and accepts no writes", pattern, method)
		}
		if !strings.HasPrefix(path, "/api/") {
			t.Errorf("route %q is outside /api; the UI is mounted separately", pattern)
		}
	}
}

// Every route is exercised, so a route nobody calls cannot rot unnoticed.
func TestEveryRouteIsExercised(t *testing.T) {
	s, h := server(t)

	calls := map[string]string{
		"GET /api/products":        "/api/products",
		"GET /api/products/{name}": "/api/products/Widgets",
		"GET /api/increments":      "/api/increments",
		"GET /api/metrics":         "/api/metrics",
		"GET /api/conformance":     "/api/conformance",
		"GET /api/schema":          "/api/schema",
	}
	for route := range s.Routes() {
		path, ok := calls[route]
		if !ok {
			t.Errorf("route %q is never exercised by the contract test", route)
			continue
		}
		if rec, _ := get(t, h, path); rec.Code != http.StatusOK {
			t.Errorf("%s: status %d — %s", path, rec.Code, rec.Body)
		}
	}
}

// AC: THE SYSTEM SHALL serve every read to any member with no per-team rules.
//
// No header, no token, no actor. Identity existed to protect writes that no longer
// happen.
func TestReadsNeedNoIdentity(t *testing.T) {
	_, h := server(t)
	for _, path := range []string{"/api/products", "/api/increments", "/api/metrics", "/api/conformance"} {
		rec, _ := get(t, h, path)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s without any identity: %d, want 200", path, rec.Code)
		}
	}
}

func TestProductsCarryPurposeAndCounts(t *testing.T) {
	_, h := server(t)
	_, body := get(t, h, "/api/products")

	products, _ := body["products"].([]any)
	if len(products) != 1 {
		t.Fatalf("got %d products, want 1", len(products))
	}
	p, _ := products[0].(map[string]any)
	if p["name"] != "Widgets" {
		t.Fatalf("name = %v, want the spec's title", p["name"])
	}
	if !strings.Contains(p["purpose"].(string), "hard to find") {
		t.Fatalf("purpose = %v", p["purpose"])
	}
	if p["done"].(float64) != 1 || p["open"].(float64) != 1 {
		t.Fatalf("counts: open %v done %v, want 1 and 1", p["open"], p["done"])
	}
	if body["refreshed_at"] == nil {
		t.Fatal("a response must say when it was read; a stale view has to read as stale")
	}
}

// Work across products is the question no single repository can answer.
func TestIncrementsSpanProductsAndFilter(t *testing.T) {
	_, h := server(t)

	_, all := get(t, h, "/api/increments")
	if all["total"].(float64) != 2 {
		t.Fatalf("total = %v, want 2", all["total"])
	}
	_, done := get(t, h, "/api/increments?status=done")
	if done["total"].(float64) != 1 {
		t.Fatalf("filtered total = %v, want 1", done["total"])
	}
	rows, _ := done["increments"].([]any)
	first, _ := rows[0].(map[string]any)
	if first["product"] != "Widgets" {
		t.Fatalf("row has no product: %v", first)
	}
	if first["transitions"] == nil {
		t.Fatal("an increment must carry the history derived from its ledger")
	}
}

func TestPaginationBoundsAList(t *testing.T) {
	_, h := server(t)

	_, page := get(t, h, "/api/increments?limit=1&offset=0")
	if len(page["increments"].([]any)) != 1 || page["total"].(float64) != 2 {
		t.Fatalf("limit ignored: %v of %v", len(page["increments"].([]any)), page["total"])
	}
	// An offset past the end returns nothing rather than erroring or wrapping.
	_, past := get(t, h, "/api/increments?limit=10&offset=99")
	if n := len(past["increments"].([]any)); n != 0 {
		t.Fatalf("offset past the end returned %d rows", n)
	}
}

func TestUnknownProductSaysHowToFindOne(t *testing.T) {
	_, h := server(t)
	rec, body := get(t, h, "/api/products/nothing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(body["error"].(string), "/api/products") {
		t.Fatalf("the error should say where to look: %v", body["error"])
	}
}

// The schema route states the convention rather than offering configuration.
func TestSchemaIsAStatementNotAnOption(t *testing.T) {
	_, h := server(t)
	_, body := get(t, h, "/api/schema")

	states, _ := body["states"].([]any)
	if len(states) < 6 {
		t.Fatalf("got %d states, want the template's full set", len(states))
	}
	if !strings.Contains(body["note"].(string), "not configurable") {
		t.Fatalf("the schema route should say it is fixed: %v", body["note"])
	}
}

// Conformance is reported, never enforced: a repository with findings still appears,
// and the response is a 200 describing them rather than an error.
func TestConformanceReportsRatherThanRefuses(t *testing.T) {
	_, h := server(t)
	rec, body := get(t, h, "/api/conformance")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; conformance describes, it does not refuse", rec.Code)
	}
	products, _ := body["products"].([]any)
	if len(products) != 1 {
		t.Fatalf("got %d products", len(products))
	}
}

var snakeCase = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// Every JSON key is snake_case, on every route. Walks real responses rather than
// struct tags, so a key reaching the wire through a map is covered too.
func TestEveryJSONKeyIsSnakeCase(t *testing.T) {
	_, h := server(t)
	for _, path := range []string{
		"/api/products", "/api/products/Widgets", "/api/increments",
		"/api/metrics", "/api/conformance", "/api/schema",
	} {
		rec, _ := get(t, h, path)
		var body any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		for _, key := range badKeys(body, nil) {
			t.Errorf("GET %s returns %q, which is not snake_case", path, key)
		}
	}
}

// badKeys walks a decoded response, skipping values whose keys an organisation chose.
func badKeys(v any, path []string) []string {
	var bad []string
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if !userNamed(path) && !snakeCase.MatchString(k) {
				bad = append(bad, strings.Join(append(path, k), "."))
			}
			bad = append(bad, badKeys(child, append(path, k))...)
		}
	case []any:
		for _, child := range t {
			bad = append(bad, badKeys(child, path)...)
		}
	}
	sort.Strings(bad)
	return bad
}

// userNamed reports whether these keys came from a repository's own ledger rather
// than from Canon.
func userNamed(path []string) bool {
	for _, p := range path {
		if p == "fields" {
			return true
		}
	}
	return false
}

// AC: WHEN a person submits a word THE SYSTEM SHALL return matching increments from every
// product.
//
// AC: THE SYSTEM SHALL match without regard to case.
func TestSearchReadsEveryFieldNotJustTheTitle(t *testing.T) {
	_, h := server(t)

	for _, tc := range []struct {
		query, why string
		want       int
	}{
		{"reindex", "a word in a title", 1},
		{"REINDEX", "the same word shouted; nobody types things the way they are written", 1},
		{"feat-002", "an id", 1},
		{"cache", "a word in one title and one scope", 1},
		{"widgets", "the product's name", 2},
		{"in-progress", "a status", 1},
		{"revert", "a rollback plan — a field the template does not fix", 2},
		{"nothing-matches-this", "a word nobody wrote", 0},
	} {
		_, body := get(t, h, "/api/increments?q="+tc.query)
		if got := int(body["total"].(float64)); got != tc.want {
			t.Errorf("q=%q matched %d, want %d (%s)", tc.query, got, tc.want, tc.why)
		}
	}
}

// AC: WHEN a search is refined THE SYSTEM SHALL return to the first page rather than an
// empty one.
//
// The server's half: a narrowed search must report the total it actually found, or a UI
// holding an old offset pages into nothing.
func TestSearchReportsItsOwnTotal(t *testing.T) {
	_, h := server(t)

	_, all := get(t, h, "/api/increments")
	_, narrowed := get(t, h, "/api/increments?q=reindex")
	if narrowed["total"].(float64) >= all["total"].(float64) {
		t.Fatalf("a search did not narrow anything: %v of %v", narrowed["total"], all["total"])
	}
	_, past := get(t, h, "/api/increments?q=reindex&offset=50")
	if n := len(past["increments"].([]any)); n != 0 {
		t.Errorf("offset past a narrowed result returned %d rows", n)
	}
	if past["total"].(float64) != narrowed["total"].(float64) {
		t.Errorf("total changed with offset: %v then %v", narrowed["total"], past["total"])
	}
}

// Search spans products, which is the question no single repository can answer.
func TestSearchSpansEveryProduct(t *testing.T) {
	_, h := server(t)
	_, body := get(t, h, "/api/increments?q=e")
	rows, _ := body["increments"].([]any)
	if len(rows) == 0 {
		t.Fatal("nothing matched a letter that appears everywhere")
	}
	for _, r := range rows {
		if r.(map[string]any)["product"] == nil {
			t.Fatal("a result does not say which product it came from")
		}
	}
}
