package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// hexish matches a SHA-256 hash as it would appear in a response.
var hexish = regexp.MustCompile(`\b[0-9a-f]{64}\b`)

// withToken issues a token for an actor through the API and returns it.
func withToken(t *testing.T, h http.Handler, admin, target string) string {
	t.Helper()
	rec := do(t, h, admin, "POST", "/api/actors/"+target+"/tokens", nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("issue token for %s: %d %s", target, rec.Code, rec.Body)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Token == "" {
		t.Fatal("no token in the response")
	}
	return body.Token
}

// asToken sends a request authenticated by bearer token rather than by claim.
func asToken(t *testing.T, h http.Handler, token, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	rec := do(t, h, "", method, path, body)
	_ = rec // the helper above cannot set headers; build the request directly
	req := httptest.NewRequest(method, path, strings.NewReader(encode(t, body)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	h.ServeHTTP(out, req)
	return out
}

func encode(t *testing.T, body any) string {
	t.Helper()
	if body == nil {
		return ""
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// AC: WHEN a caller presents no token THE SYSTEM SHALL refuse the request.
func TestOnceATokenExistsClaimsAreRefused(t *testing.T) {
	_, h := newServer(t)

	// Before any token exists, the previous behaviour holds.
	if rec := do(t, h, "ollie", "GET", "/api/issues", nil); rec.Code != http.StatusOK {
		t.Fatalf("an instance with no tokens should still work: %d %s", rec.Code, rec.Body)
	}

	withToken(t, h, "ollie", "sam")

	// sam now holds a token, so claiming to be sam is no longer enough — for reads
	// as well as writes, which is the half that was missing entirely.
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/issues"},
		{"POST", "/api/issues"},
	} {
		rec := do(t, h, "sam", c.method, c.path, map[string]any{"title": "x", "team": "platform"})
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with only a claimed identity: %d, want 401", c.method, c.path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "Bearer") {
			t.Errorf("the refusal should say how to authenticate, got: %s", rec.Body)
		}
	}

	// ollie holds no token and is still trusted, so issuing sam a token did not lock
	// the administrator out of their own instance.
	if rec := do(t, h, "ollie", "GET", "/api/issues", nil); rec.Code != http.StatusOK {
		t.Fatalf("an actor with no token should still work: %d %s", rec.Code, rec.Body)
	}
}

// AC: WHEN a caller presents a token THE SYSTEM SHALL act as the actor that token
// belongs to, ignoring any claimed identity.
func TestTheTokenDecidesWhoYouAre(t *testing.T) {
	_, h := newServer(t)
	samToken := withToken(t, h, "ollie", "sam")

	// Present sam's token while claiming to be ollie. The token wins.
	req := httptest.NewRequest("POST", "/api/issues", strings.NewReader(
		`{"id":"CANON-1","title":"who made this","type":"story","team":"platform"}`))
	req.Header.Set("Authorization", "Bearer "+samToken)
	req.Header.Set(ActorHeader, "ollie")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}

	got := asToken(t, h, samToken, "GET", "/api/issues/CANON-1", nil)
	var issue map[string]any
	if err := json.Unmarshal(got.Body.Bytes(), &issue); err != nil {
		t.Fatal(err)
	}
	last, _ := issue["last_actor"].(map[string]any)
	if last["id"] != "sam" {
		t.Fatalf("the write was attributed to %v; the token must decide, not the header", last["id"])
	}
}

// AC: WHEN a token is revoked THE SYSTEM SHALL refuse it thereafter.
func TestRevokedTokenIsRefusedOverHTTP(t *testing.T) {
	_, h := newServer(t)
	samToken := withToken(t, h, "ollie", "sam")

	if rec := asToken(t, h, samToken, "GET", "/api/issues", nil); rec.Code != http.StatusOK {
		t.Fatalf("the token should work before revocation: %d", rec.Code)
	}
	if rec := do(t, h, "ollie", "DELETE", "/api/actors/sam/tokens", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body)
	}
	if rec := asToken(t, h, samToken, "GET", "/api/issues", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a revoked token returned %d, want 401", rec.Code)
	}
}

// Verified live before this existed: a "reporter" granted itself "admin" and the API
// returned 204. Every registry write is now gated.
func TestAnActorCannotEscalateItsOwnRole(t *testing.T) {
	_, h := newServer(t)

	rec := do(t, h, "jo", "POST", "/api/actors/jo/roles", map[string]any{"role": "admin"})
	// 422 is how this API reports a refused operation; what matters is that it is
	// refused and says which role would permit it.
	if rec.Code < 400 {
		t.Fatalf("a reporter granted itself admin: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "administer") {
		t.Fatalf("the refusal should name the operation, got: %s", rec.Body)
	}

	check := do(t, h, "ollie", "GET", "/api/actors/jo", nil)
	if strings.Contains(check.Body.String(), "admin") {
		t.Fatalf("jo holds admin: %s", check.Body)
	}
}

// The whole registry is administrative, not just role granting.
func TestEveryRegistryWriteIsGated(t *testing.T) {
	_, h := newServer(t)

	writes := []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/actors", map[string]any{"id": "intruder"}},
		{"POST", "/api/actors/jo/roles", map[string]any{"role": "admin"}},
		{"DELETE", "/api/actors/ollie/roles/admin", nil},
		{"POST", "/api/actors/jo/teams", map[string]any{"team": "platform"}},
		{"DELETE", "/api/actors/ollie/teams/platform", nil},
		{"POST", "/api/actors/ollie/tokens", nil},
		{"DELETE", "/api/actors/ollie/tokens", nil},
	}
	for _, w := range writes {
		rec := do(t, h, "jo", w.method, w.path, w.body)
		if rec.Code < 400 {
			t.Errorf("%s %s as a reporter returned %d; it should be refused", w.method, w.path, rec.Code)
		}
	}
}

// Rotating your own token must not need somebody else, or a leaked token stays live
// over a weekend while its owner waits for an administrator.
func TestAnActorMayRotateItsOwnToken(t *testing.T) {
	_, h := newServer(t)
	samToken := withToken(t, h, "ollie", "sam")

	req := httptest.NewRequest("POST", "/api/actors/sam/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+samToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("an actor should be able to issue itself a token: %d %s", rec.Code, rec.Body)
	}
}

// A token must never come back from a read route.
func TestNoRouteDisclosesATokenOrItsHash(t *testing.T) {
	s, h := newServer(t)
	token := withToken(t, h, "ollie", "sam")

	for _, path := range []string{"/api/actors", "/api/actors/sam", "/api/events"} {
		rec := asToken(t, h, token, "GET", path, nil)
		body := rec.Body.String()
		if strings.Contains(body, token) {
			t.Errorf("GET %s discloses the token", path)
		}
		if strings.Contains(body, "token_hashes") || strings.Contains(body, "TokenHashes") {
			t.Errorf("GET %s discloses token hashes", path)
		}
	}

	// The hash is in the log by design — that is where it is stored — but the events
	// route must not be a way to read it out. The key survives; the value must not,
	// so this looks for hex rather than for the field name.
	rec := asToken(t, h, token, "GET", "/api/events", nil)
	if hexish.FindString(rec.Body.String()) != "" {
		t.Errorf("the events route exposes a stored hash: %s", hexish.FindString(rec.Body.String()))
	}
	if !strings.Contains(rec.Body.String(), "[redacted]") {
		t.Error("the token event should still be visible, with its secret removed")
	}
	_ = s
}
