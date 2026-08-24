package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

// seedLarge builds a project of n issues with a realistic event tail: created, a
// field set, and most of them transitioned once or twice.
func seedLarge(tb testing.TB, n int) (http.Handler, *event.Store) {
	tb.Helper()
	sch, err := schema.Load(filepath.Join("..", "schema", "testdata", "canon.yaml"))
	if err != nil {
		tb.Fatal(err)
	}
	log, err := event.Open(filepath.Join(tb.TempDir(), "canon.db"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { log.Close() })

	at := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	sys := event.Actor{ID: "boot", Kind: event.ActorSystem}
	human := event.Actor{ID: "ollie", Kind: event.ActorHuman}

	// Write through the log directly: the point is to measure reads, and going
	// through the enforcer would spend the whole budget on setup.
	bootstrap := []*event.Event{
		event.New("actor.registered", "ollie", at, sys, map[string]any{"kind": "human"}),
		event.New("actor.role_granted", "ollie", at, sys, map[string]any{"role": "admin"}),
		event.New("team.member_added", "ollie", at, sys, map[string]any{"team": "platform"}),
	}
	if err := log.AppendBatch(func(yield func(*event.Event) bool) {
		for _, e := range bootstrap {
			if !yield(e) {
				return
			}
		}
		for i := range n {
			id := fmt.Sprintf("CANON-%d", i+1)
			team := "platform"
			if i%3 == 0 {
				team = "payments"
			}
			when := at.Add(time.Duration(i) * time.Minute)
			if !yield(event.New("issue.created", id, when, human, map[string]any{
				"title": fmt.Sprintf("Issue number %d needs attention", i+1),
				"state": "todo", "type": "bug",
				"priority": []string{"p1", "p2", "p3", "p4"}[i%4],
			})) {
				return
			}
			if !yield(event.New("issue.team_set", id, when, human, map[string]any{"team": team})) {
				return
			}
			if i%4 != 0 {
				if !yield(event.New("issue.transitioned", id, when.Add(time.Hour), human,
					map[string]any{"from": "todo", "to": "in_progress"})) {
					return
				}
			}
			if i%4 == 2 {
				if !yield(event.New("issue.transitioned", id, when.Add(2*time.Hour), human,
					map[string]any{"from": "in_progress", "to": "in_review", "evidence": "ok"})) {
					return
				}
			}
		}
		return
	}); err != nil {
		tb.Fatal(err)
	}

	e := enforce.New(sch, log)
	srv := New(sch, log, e, time.Now)
	return srv.Handler(), log
}

// measure returns the p95 of n requests, in milliseconds.
func measure(tb testing.TB, h http.Handler, method, path string, reps int) (p50, p95 float64) {
	tb.Helper()
	samples := make([]float64, 0, reps)
	for range reps {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set(ActorHeader, "ollie")
		rec := httptest.NewRecorder()
		start := time.Now()
		h.ServeHTTP(rec, req)
		samples = append(samples, float64(time.Since(start).Microseconds())/1000)
		if rec.Code >= 400 {
			tb.Fatalf("%s %s: status %d — %s", method, path, rec.Code, rec.Body.String())
		}
	}
	sort.Float64s(samples)
	at := func(p float64) float64 {
		i := int(p*float64(len(samples))+0.5) - 1
		if i < 0 {
			i = 0
		}
		if i >= len(samples) {
			i = len(samples) - 1
		}
		return samples[i]
	}
	return at(0.50), at(0.95)
}

// AC: WHEN any read request is served against a 10,000-issue project THE SYSTEM SHALL
// respond in under 200ms at p95.
// AC: THE SYSTEM SHALL include a reproducible benchmark that fails CI if the budget regresses.
func TestReadLatencyBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("latency budget skipped in short mode")
	}
	const (
		issues = 10_000
		budget = 200.0
		reps   = 20
	)
	h, log := seedLarge(t, issues)
	count, err := log.Count()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("dataset: %d issues, %d events", issues, count)

	reads := []struct{ name, method, path string }{
		{"list all issues", "GET", "/api/issues"},
		{"list, filtered by team", "GET", "/api/issues?q=team%3Dplatform"},
		{"list, two terms", "GET", "/api/issues?q=team%3Dplatform+priority%3Dp1"},
		{"one issue", "GET", "/api/issues/CANON-5000"},
		{"children of one issue", "GET", "/api/issues/CANON-5000/children"},
		{"schema", "GET", "/api/schema"},
		{"metrics, 30 days", "GET", "/api/metrics?days=30"},
		{"ancestors of one issue", "GET", "/api/issues/CANON-5000/ancestors"},
		{"subtree of one issue", "GET", "/api/issues/CANON-5000/tree"},
		{"query by ancestor", "GET", "/api/issues?q=ancestor%3DCANON-1"},
		{"proposals", "GET", "/api/proposals"},
		{"actors", "GET", "/api/actors"},
	}

	var breached []string
	for _, r := range reads {
		p50, p95 := measure(t, h, r.method, r.path, reps)
		status := "ok"
		if p95 > budget {
			status = "OVER BUDGET"
			breached = append(breached, r.name)
		}
		t.Logf("  %-26s p50 %7.1fms   p95 %7.1fms   %s", r.name, p50, p95, status)
	}
	if len(breached) > 0 {
		t.Errorf("%d read(s) over the %.0fms budget: %v", len(breached), budget, breached)
	}
}

// A large list must not be unbounded. Returning ten thousand issues in one response
// is slow for the server and useless to the caller.
func TestLargeListIsPaginated(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode")
	}
	h, _ := seedLarge(t, 10_000)
	req := httptest.NewRequest(http.MethodGet, "/api/issues", nil)
	req.Header.Set(ActorHeader, "ollie")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var body struct {
		Issues []json.RawMessage `json:"issues"`
		Total  int               `json:"total"`
		Limit  int               `json:"limit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Issues) > 500 {
		t.Errorf("an unfiltered list returned %d issues; it must be bounded", len(body.Issues))
	}
	if body.Total != 10_000 {
		t.Errorf("total: got %d want 10000 — the caller must know what it is missing", body.Total)
	}
}

// The point of catching up rather than rebuilding, and of paginating during the scan
// rather than after it, is that a read's cost stops tracking the size of the log.
//
// This is asserted on allocations, not on wall-clock time. Two earlier versions
// compared timings between dataset sizes and both flaked in CI while measuring flat
// locally — a shared runner's contention swamps a two-millisecond baseline. A
// profile confirmed the read path was never the problem: the allocations the
// benchmark attributed to it were the fixture's. Allocations per read are
// deterministic and machine-independent, which is what an algorithmic claim needs.
func TestReadCostDoesNotTrackLogSize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode")
	}
	measureAllocs := func(n int) float64 {
		h, _ := seedLarge(t, n)
		request := func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/api/issues?q=team%3Dplatform", nil)
			r.Header.Set(ActorHeader, "ollie")
			return r
		}
		// Warm the projection so catch-up is not counted as steady-state cost.
		h.ServeHTTP(httptest.NewRecorder(), request())
		return testing.AllocsPerRun(10, func() {
			h.ServeHTTP(httptest.NewRecorder(), request())
		})
	}

	small := measureAllocs(10_000)
	large := measureAllocs(50_000)
	t.Logf("  10k issues: %.0f allocs/read", small)
	t.Logf("  50k issues: %.0f allocs/read", large)

	// A read returns one page whatever the dataset, so its allocations should be
	// near-constant. A little slack absorbs map iteration order and encoder reuse.
	if large > small*1.5 {
		t.Errorf("allocations grew from %.0f to %.0f for a 5x larger log; a read should cost the page, not the log",
			small, large)
	}

	// The absolute budget is the requirement, and is checked on time.
	_, p95 := measure(t, mustSeed(t, 50_000), "GET", "/api/issues?q=team%3Dplatform", 20)
	t.Logf("  50k issues: p95 %.2fms", p95)
	if p95 > 200 {
		t.Errorf("p95 at 50k issues is %.1fms, over the 200ms budget", p95)
	}
}

func mustSeed(t *testing.T, n int) http.Handler {
	t.Helper()
	h, _ := seedLarge(t, n)
	return h
}
