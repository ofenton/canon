package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
	"github.com/ofenton/canon/internal/webhook"
)

var fixedTime = time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)

func newServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	s, err := schema.Load(filepath.Join("..", "schema", "testdata", "canon.yaml"))
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	log, err := event.Open(filepath.Join(t.TempDir(), "canon.db"))
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	t.Cleanup(func() { log.Close() })

	e := enforce.New(s, log)
	sys := event.Actor{ID: "bootstrap", Kind: event.ActorSystem}
	for _, a := range []struct{ id, role string }{{"ollie", "admin"}, {"sam", "member"}, {"jo", "reporter"}} {
		if err := e.RegisterActor(a.id, event.ActorHuman, "", fixedTime, sys); err != nil {
			t.Fatal(err)
		}
		if err := e.GrantRole(a.id, a.role, fixedTime, sys); err != nil {
			t.Fatal(err)
		}
		if err := e.AddToTeam(a.id, "platform", fixedTime, sys); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.RegisterActor("agent:one", event.ActorAgent, "claude-opus-5", fixedTime, sys); err != nil {
		t.Fatal(err)
	}
	if err := e.GrantRole("agent:one", "agent", fixedTime, sys); err != nil {
		t.Fatal(err)
	}
	if err := e.AddToTeam("agent:one", "platform", fixedTime, sys); err != nil {
		t.Fatal(err)
	}

	srv := New(s, log, e, func() time.Time { return fixedTime })
	return srv, srv.Handler()
}

func do(t *testing.T, h http.Handler, actor, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if actor != "" {
		req.Header.Set(ActorHeader, actor)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// AC: WHEN a caller creates an issue supplying only a title THE SYSTEM SHALL create
// it successfully.
func TestCreateNeedsOnlyATitle(t *testing.T) {
	_, h := newServer(t)
	rec := do(t, h, "ollie", "POST", "/api/issues", map[string]string{"title": "Search is slow"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var created struct{ ID string }
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("an id must be allocated when none is supplied")
	}

	rec = do(t, h, "ollie", "GET", "/api/issues/"+created.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	var issue struct {
		Title, State string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &issue); err != nil {
		t.Fatal(err)
	}
	if issue.Title != "Search is slow" {
		t.Errorf("title: got %q", issue.Title)
	}
	if issue.State != "todo" {
		t.Errorf("state: got %q, want the schema's first open state", issue.State)
	}
}

// AC: THE SYSTEM SHALL expose every read and write operation over one HTTP API.
// Every route is exercised, so the contract is proven rather than asserted.
func TestEveryRouteIsExercised(t *testing.T) {
	srv, h := newServer(t)

	type call struct {
		route, method, path string
		actor               string
		body                any
		want                int
	}
	calls := []call{
		{"GET /api/schema", "GET", "/api/schema", "ollie", nil, 200},
		{"GET /api/schema/usage", "GET", "/api/schema/usage", "ollie", nil, 200},
		{"POST /api/issues", "POST", "/api/issues", "ollie", map[string]string{"id": "CANON-1", "title": "one", "team": "platform", "type": "story"}, 201},
		{"GET /api/issues", "GET", "/api/issues", "ollie", nil, 200},
		{"GET /api/issues/{id}", "GET", "/api/issues/CANON-1", "ollie", nil, 200},
		{"PATCH /api/issues/{id}/fields", "PATCH", "/api/issues/CANON-1/fields", "ollie", map[string]string{"priority": "p1"}, 204},
		{"PUT /api/issues/{id}/multi/{field}", "PUT", "/api/issues/CANON-1/multi/kpi", "ollie", map[string]any{"values": []string{"conversion"}}, 204},
		{"PUT /api/issues/{id}/checklist/{field}", "PUT", "/api/issues/CANON-1/checklist/acceptance", "ollie", map[string]any{"text": "a criterion"}, 201},
		{"DELETE /api/issues/{id}/checklist/{field}", "DELETE", "/api/issues/CANON-1/checklist/acceptance", "ollie", map[string]any{"text": "a criterion"}, 204},
		{"POST /api/issues/{id}/transition", "POST", "/api/issues/CANON-1/transition", "ollie", map[string]string{"to": "in_progress"}, 204},
		{"POST /api/issues", "POST", "/api/issues", "ollie", map[string]string{"id": "CANON-2", "title": "two", "team": "platform", "type": "task"}, 201},
		{"PUT /api/issues/{id}/parent", "PUT", "/api/issues/CANON-2/parent", "ollie", map[string]string{"parent": "CANON-1"}, 204},
		{"GET /api/issues/{id}/children", "GET", "/api/issues/CANON-1/children", "ollie", nil, 200},
		{"GET /api/issues/{id}/ancestors", "GET", "/api/issues/CANON-2/ancestors", "ollie", nil, 200},
		{"GET /api/issues/{id}/tree", "GET", "/api/issues/CANON-1/tree", "ollie", nil, 200},
		{"PUT /api/issues/{id}/dependencies", "PUT", "/api/issues/CANON-2/dependencies", "ollie", map[string]string{"on": "CANON-1"}, 200},
		{"GET /api/issues/{id}/dependencies", "GET", "/api/issues/CANON-2/dependencies", "ollie", nil, 200},
		{"GET /api/cycles", "GET", "/api/cycles", "ollie", nil, 200},
		{"PUT /api/issues/{id}/commits", "PUT", "/api/issues/CANON-1/commits", "ollie", map[string]string{"sha": "a1b2c3d", "message": "Reindex on write"}, 204},
		{"GET /api/issues/{id}/commits", "GET", "/api/issues/CANON-1/commits", "ollie", nil, 200},
		{"DELETE /api/issues/{id}/dependencies/{on}", "DELETE", "/api/issues/CANON-2/dependencies/CANON-1", "ollie", nil, 204},
		{"DELETE /api/issues/{id}", "DELETE", "/api/issues/CANON-2", "ollie", nil, 204},
		{"POST /api/issues", "POST", "/api/issues", "ollie", map[string]string{"id": "CANON-3", "title": "three", "team": "platform", "type": "task"}, 201},
		{"POST /api/issues/{id}/transition", "POST", "/api/issues/CANON-3/transition", "agent:one", map[string]string{"to": "in_progress"}, 204},
		{"GET /api/events", "GET", "/api/events", "ollie", nil, 200},
		{"GET /api/actors", "GET", "/api/actors", "ollie", nil, 200},
		{"POST /api/actors", "POST", "/api/actors", "ollie", map[string]string{"id": "kim"}, 201},
		{"GET /api/actors/{id}", "GET", "/api/actors/kim", "ollie", nil, 200},
		{"POST /api/actors/{id}/tokens", "POST", "/api/actors/kim/tokens", "ollie", nil, 201},
		{"DELETE /api/actors/{id}/tokens", "DELETE", "/api/actors/kim/tokens", "ollie", nil, 204},
		{"POST /api/actors/{id}/roles", "POST", "/api/actors/kim/roles", "ollie", map[string]string{"role": "member"}, 204},
		{"DELETE /api/actors/{id}/roles/{role}", "DELETE", "/api/actors/kim/roles/member", "ollie", nil, 204},
		{"POST /api/actors/{id}/teams", "POST", "/api/actors/kim/teams", "ollie", map[string]string{"team": "platform"}, 204},
		{"DELETE /api/actors/{id}/teams/{team}", "DELETE", "/api/actors/kim/teams/platform", "ollie", nil, 204},

		// An agent works CANON-1 to in_review, then proposes completing it. 202 is
		// the proposal, not a failure.
		{"POST /api/issues/{id}/transition", "POST", "/api/issues/CANON-1/transition", "agent:one", map[string]string{"to": "in_review", "evidence": "312 passed"}, 204},
		{"POST /api/issues/{id}/transition", "POST", "/api/issues/CANON-1/transition", "agent:one", map[string]string{"to": "done"}, 202},
		{"GET /api/proposals", "GET", "/api/proposals", "ollie", nil, 200},
		{"GET /api/proposals/{id}", "GET", "/api/proposals/PROP-1", "ollie", nil, 200},
		{"POST /api/proposals/{id}/approve", "POST", "/api/proposals/PROP-1/approve", "ollie", nil, 204},
		{"POST /api/issues/{id}/transition", "POST", "/api/issues/CANON-3/transition", "agent:one", map[string]string{"to": "done"}, 202},
		{"POST /api/proposals/{id}/reject", "POST", "/api/proposals/PROP-2/reject", "ollie", map[string]string{"reason": "not ready"}, 204},

		{"POST /api/boards", "POST", "/api/boards", "ollie", map[string]string{"name": "platform", "query": "team=platform", "group_by": "state"}, 201},
		{"GET /api/metrics", "GET", "/api/metrics", "ollie", nil, 200},
		{"GET /api/boards", "GET", "/api/boards", "ollie", nil, 200},
		{"GET /api/boards/{name}", "GET", "/api/boards/platform", "ollie", nil, 200},
		{"DELETE /api/boards/{name}", "DELETE", "/api/boards/platform", "ollie", nil, 204},
	}

	exercised := map[string]bool{}
	for _, c := range calls {
		rec := do(t, h, c.actor, c.method, c.path, c.body)
		if rec.Code != c.want {
			t.Errorf("%s %s: status %d want %d — %s", c.method, c.path, rec.Code, c.want, rec.Body)
		}
		exercised[c.route] = true
	}

	var missing []string
	for route := range srv.Routes() {
		if !exercised[route] {
			missing = append(missing, route)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("routes never exercised by the contract test: %s", strings.Join(missing, ", "))
	}
}

// AC: THE SYSTEM SHALL contain no endpoint reachable only by the web UI.
//
// Everything an agent can reach is under /api. A route outside it, or one whose
// name marks it as UI-only, would break the parity the product claims.
func TestNoUIOnlyRoutes(t *testing.T) {
	srv, _ := newServer(t)
	for route := range srv.Routes() {
		_, path, ok := strings.Cut(route, " ")
		if !ok {
			t.Errorf("route %q has no method", route)
			continue
		}
		if !strings.HasPrefix(path, "/api/") {
			t.Errorf("route %q is outside /api: every operation must be reachable by an agent", route)
		}
		for _, marker := range []string{"/ui/", "/internal/", "/_", "/web/"} {
			if strings.Contains(path, marker) {
				t.Errorf("route %q looks UI-only", route)
			}
		}
	}
}

// Authorisation must hold at the HTTP boundary, not only in the domain.
func TestAuthorisationAtTheBoundary(t *testing.T) {
	_, h := newServer(t)
	if rec := do(t, h, "ollie", "POST", "/api/issues",
		map[string]string{"id": "CANON-1", "title": "x", "team": "platform"}); rec.Code != 201 {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body)
	}

	cases := []struct {
		name, actor, method, path string
		body                      any
		want                      int
	}{
		{"no actor header", "", "POST", "/api/issues", map[string]string{"title": "x"}, 401},
		// Reads used to be open to anyone — this expectation was 200 until feat-031,
		// when authentication moved into middleware covering every route.
		{"unregistered actor", "ghost", "GET", "/api/issues", nil, 401},
		{"unregistered actor writing", "ghost", "POST", "/api/issues", map[string]string{"title": "x"}, 401},
		{"reporter may not set priority", "jo", "PATCH", "/api/issues/CANON-1/fields", map[string]string{"priority": "p1"}, 422},
		{"member may set priority", "sam", "PATCH", "/api/issues/CANON-1/fields", map[string]string{"priority": "p1"}, 204},
		{"unknown field", "ollie", "PATCH", "/api/issues/CANON-1/fields", map[string]string{"storyPoints": "8"}, 422},
		{"illegal transition", "ollie", "POST", "/api/issues/CANON-1/transition", map[string]string{"to": "done"}, 422},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, h, c.actor, c.method, c.path, c.body)
			if rec.Code != c.want {
				t.Errorf("status %d want %d — %s", rec.Code, c.want, rec.Body)
			}
		})
	}
}

// An agent's refusal is 202 with a proposal, not 403. The request was understood and
// recorded for a human, which is a different outcome from being refused.
func TestAgentProposalIsAccepted(t *testing.T) {
	_, h := newServer(t)
	do(t, h, "ollie", "POST", "/api/issues", map[string]string{"id": "CANON-1", "title": "x", "team": "platform"})
	do(t, h, "agent:one", "POST", "/api/issues/CANON-1/transition", map[string]string{"to": "in_progress"})
	do(t, h, "agent:one", "POST", "/api/issues/CANON-1/transition",
		map[string]string{"to": "in_review", "evidence": "312 passed"})

	rec := do(t, h, "agent:one", "POST", "/api/issues/CANON-1/transition", map[string]string{"to": "done"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status %d want 202 — %s", rec.Code, rec.Body)
	}
	var body struct {
		Status, Operation string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "proposal_required" {
		t.Errorf("status: got %q", body.Status)
	}
	if body.Operation != "transition:in_review->done" {
		t.Errorf("operation: got %q", body.Operation)
	}
}

// Errors must survive the HTTP boundary intact — they are what tells an agent what
// to do next, and rewording them at the edge would discard that.
func TestErrorsReachTheCaller(t *testing.T) {
	_, h := newServer(t)
	do(t, h, "ollie", "POST", "/api/issues", map[string]string{"id": "CANON-1", "title": "x", "team": "platform"})
	rec := do(t, h, "ollie", "POST", "/api/issues/CANON-1/transition", map[string]string{"to": "done"})
	var body struct{ Error string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"in_progress", "abandoned"} {
		if !strings.Contains(body.Error, want) {
			t.Errorf("error should name the permitted transitions, got: %s", body.Error)
		}
	}
}

func TestListFilters(t *testing.T) {
	_, h := newServer(t)
	for i, team := range []string{"platform", "platform", "payments"} {
		do(t, h, "ollie", "POST", "/api/issues",
			map[string]string{"id": fmt.Sprintf("CANON-%d", i+1), "title": "x", "team": team})
	}
	rec := do(t, h, "ollie", "GET", "/api/issues?team=platform", nil)
	var body struct{ Issues []struct{ ID string } }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Issues) != 2 {
		t.Errorf("team filter: got %d issues want 2", len(body.Issues))
	}
}

// A query naming something the schema does not have must be refused at the boundary,
// not silently return an empty list that reads as "no work".
func TestQueryValidationAtTheBoundary(t *testing.T) {
	_, h := newServer(t)
	do(t, h, "ollie", "POST", "/api/issues",
		map[string]string{"id": "CANON-1", "title": "x", "team": "platform", "fields": ""})

	for _, q := range []string{"storyPoints=8", "sprint=4", "state=shipped"} {
		rec := do(t, h, "ollie", "GET", "/api/issues?q="+url.QueryEscape(q), nil)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("query %q: status %d want 400 — %s", q, rec.Code, rec.Body)
		}
	}

	rec := do(t, h, "ollie", "GET", "/api/issues?q="+url.QueryEscape("team=platform"), nil)
	if rec.Code != http.StatusOK {
		t.Errorf("a valid query must succeed: %d %s", rec.Code, rec.Body)
	}
}

// Board membership must follow the data with no board update.
func TestBoardFollowsTheDataOverHTTP(t *testing.T) {
	_, h := newServer(t)
	do(t, h, "ollie", "POST", "/api/issues", map[string]string{"id": "CANON-1", "title": "one", "team": "platform", "type": "story"})
	do(t, h, "ollie", "POST", "/api/issues", map[string]string{"id": "CANON-2", "title": "two", "team": "payments"})
	if rec := do(t, h, "ollie", "POST", "/api/boards",
		map[string]string{"name": "plat", "query": "team=platform", "group_by": "state"}); rec.Code != 201 {
		t.Fatalf("save board: %d %s", rec.Code, rec.Body)
	}

	count := func() int {
		rec := do(t, h, "ollie", "GET", "/api/boards/plat", nil)
		var body struct {
			Columns []struct {
				Name  string
				Count int
			}
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		total := 0
		for _, c := range body.Columns {
			total += c.Count
		}
		return total
	}

	if got := count(); got != 1 {
		t.Fatalf("board should hold one platform issue, got %d", got)
	}
	// Moving an issue into the team must add it, with no write to the board.
	do(t, h, "ollie", "POST", "/api/issues", map[string]string{"id": "CANON-3", "title": "three", "team": "platform", "type": "task"})
	if got := count(); got != 2 {
		t.Errorf("board should follow the data, got %d", got)
	}
}

// Saving a board whose query does not parse must be refused at save time.
func TestBoardQueryIsValidatedOnSave(t *testing.T) {
	_, h := newServer(t)
	rec := do(t, h, "ollie", "POST", "/api/boards",
		map[string]string{"name": "bad", "query": "storyPoints=8"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status %d want 422 — %s", rec.Code, rec.Body)
	}
	rec = do(t, h, "ollie", "POST", "/api/boards",
		map[string]string{"name": "bad", "query": "team=platform", "group_by": "sprint"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("bad group key: status %d want 422 — %s", rec.Code, rec.Body)
	}
}

// AC: WHEN an issue transitions THE SYSTEM SHALL deliver a webhook describing the
// transition.
//
// End to end over HTTP rather than against the sender directly, because the thing
// worth proving is that a real transition through the real API reaches a real
// subscriber — the wiring is where this would silently not happen.
func TestTransitionOverHTTPFiresAWebhook(t *testing.T) {
	got := make(chan map[string]any, 4)
	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var d map[string]any
		_ = json.NewDecoder(r.Body).Decode(&d)
		got <- d
	}))
	defer subscriber.Close()

	sch, err := schema.Load(filepath.Join("..", "schema", "testdata", "canon.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	sch.Webhooks = []schema.Webhook{{URL: subscriber.URL}}

	log, err := event.Open(filepath.Join(t.TempDir(), "canon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	e := enforce.New(sch, log)
	sender := webhook.New(sch, slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.OnTransition(sender)
	defer sender.Close(5 * time.Second)

	sys := event.Actor{ID: "bootstrap", Kind: event.ActorSystem}
	for _, step := range []func() error{
		func() error { return e.RegisterActor("ollie", event.ActorHuman, "", fixedTime, sys) },
		func() error { return e.GrantRole("ollie", "admin", fixedTime, sys) },
		func() error { return e.AddToTeam("ollie", "platform", fixedTime, sys) },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	h := New(sch, log, e, func() time.Time { return fixedTime }).Handler()

	if rec := do(t, h, "ollie", "POST", "/api/issues",
		map[string]any{"id": "CANON-1", "title": "Search is slow", "type": "story", "team": "platform"}); rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	if rec := do(t, h, "ollie", "POST", "/api/issues/CANON-1/transition",
		map[string]any{"to": "in_progress"}); rec.Code >= 300 {
		t.Fatalf("transition: %d %s", rec.Code, rec.Body)
	}

	select {
	case d := <-got:
		if d["issue"] != "CANON-1" || d["to"] != "in_progress" || d["from"] != "todo" {
			t.Fatalf("webhook did not describe the transition: %v", d)
		}
		if d["actor"] != "ollie" {
			t.Fatalf("webhook lost the provenance: %v", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a transition over HTTP delivered no webhook")
	}
}

// A subscriber that never answers must not slow a transition down. This is the
// property the whole asynchronous design exists for, so it is asserted end to end.
func TestASlowSubscriberDoesNotSlowTheWrite(t *testing.T) {
	release := make(chan struct{})
	subscriber := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer subscriber.Close()
	defer close(release)

	sch, err := schema.Load(filepath.Join("..", "schema", "testdata", "canon.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	sch.Webhooks = []schema.Webhook{{URL: subscriber.URL}}

	log, err := event.Open(filepath.Join(t.TempDir(), "canon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	e := enforce.New(sch, log)
	sender := webhook.New(sch, slog.New(slog.NewTextHandler(io.Discard, nil)))
	e.OnTransition(sender)
	defer sender.Close(time.Second)

	sys := event.Actor{ID: "bootstrap", Kind: event.ActorSystem}
	_ = e.RegisterActor("ollie", event.ActorHuman, "", fixedTime, sys)
	_ = e.GrantRole("ollie", "admin", fixedTime, sys)
	_ = e.AddToTeam("ollie", "platform", fixedTime, sys)
	h := New(sch, log, e, func() time.Time { return fixedTime }).Handler()

	do(t, h, "ollie", "POST", "/api/issues",
		map[string]any{"id": "CANON-1", "title": "t", "type": "story", "team": "platform"})

	start := time.Now()
	rec := do(t, h, "ollie", "POST", "/api/issues/CANON-1/transition", map[string]any{"to": "in_progress"})
	took := time.Since(start)

	if rec.Code >= 300 {
		t.Fatalf("transition: %d %s", rec.Code, rec.Body)
	}
	if took > 500*time.Millisecond {
		t.Fatalf("the transition took %s waiting on a subscriber that never answers", took)
	}
}
