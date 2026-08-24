package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// last returns the most recent event in the log, which is how these tests read the
// timestamp that was actually recorded rather than the one the caller asked for.
func last(t *testing.T, s *Server) map[string]any {
	t.Helper()
	events, err := s.log.All()
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("log is empty")
	}
	e := events[len(events)-1]
	raw, err := json.Marshal(map[string]any{"type": e.Type, "at": e.At, "subject": e.Subject})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// AC: WHEN a caller supplies at on a write THE SYSTEM SHALL record that instant as
// the event time.
func TestBackdatedCreateOverHTTP(t *testing.T) {
	s, h := newServer(t)
	when := fixedTime.Add(-72 * time.Hour)

	rec := do(t, h, "ollie", "POST",
		"/api/issues?at="+when.Format(time.RFC3339),
		map[string]any{"title": "Recorded late", "team": "platform"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, body %s", rec.Code, rec.Body)
	}

	got := last(t, s)["at"].(string)
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("parse recorded time %q: %v", got, err)
	}
	if !parsed.Equal(when) {
		t.Fatalf("recorded at %s, want the supplied %s", parsed, when)
	}
}

// AC: WHEN a caller supplies at in the future THE SYSTEM SHALL refuse the write and
// say so.
func TestFutureDatedWriteIsRefusedOverHTTP(t *testing.T) {
	_, h := newServer(t)
	when := fixedTime.Add(24 * time.Hour)

	rec := do(t, h, "ollie", "POST",
		"/api/issues?at="+when.Format(time.RFC3339),
		map[string]any{"title": "Tomorrow", "team": "platform"})
	if rec.Code == http.StatusCreated {
		t.Fatal("a future-dated create should be refused")
	}
	if !strings.Contains(rec.Body.String(), "future") {
		t.Fatalf("the refusal should say the time is in the future, got: %s", rec.Body)
	}
}

// AC: WHEN a caller lacks the backdate permission THE SYSTEM SHALL refuse the write.
func TestBackdateWithoutPermissionIsRefusedOverHTTP(t *testing.T) {
	_, h := newServer(t)
	when := fixedTime.Add(-72 * time.Hour)

	rec := do(t, h, "sam", "POST",
		"/api/issues?at="+when.Format(time.RFC3339),
		map[string]any{"title": "Not mine to date", "team": "platform"})
	if rec.Code == http.StatusCreated {
		t.Fatal("a member holds no backdate grant and should be refused")
	}
	if !strings.Contains(rec.Body.String(), "backdate") {
		t.Fatalf("the refusal should name the operation, got: %s", rec.Body)
	}
}

// A member must still be able to write normally: the permission gates backdating,
// not writing.
func TestOrdinaryWriteIsUnaffected(t *testing.T) {
	s, h := newServer(t)

	rec := do(t, h, "sam", "POST", "/api/issues",
		map[string]any{"title": "Ordinary", "team": "platform"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, body %s", rec.Code, rec.Body)
	}
	if got := last(t, s)["at"].(string); !strings.HasPrefix(got, "2026-08-23T09:00:00") {
		t.Fatalf("an ordinary write should carry the server's clock, got %s", got)
	}
}

// A malformed timestamp is a client error, and the message has to be enough to fix
// it without reading the source.
func TestMalformedTimestampIsRejected(t *testing.T) {
	_, h := newServer(t)

	rec := do(t, h, "ollie", "POST", "/api/issues?at=last%20tuesday",
		map[string]any{"title": "Vague", "team": "platform"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "RFC 3339") {
		t.Fatalf("the error should show the expected format, got: %s", rec.Body)
	}
}

// Backdating has to reach the writes an import actually replays, not just create.
func TestBackdatingReachesTransitions(t *testing.T) {
	s, h := newServer(t)

	// The issue is itself backdated, so the transition can sit between its creation
	// and now — which is the shape every imported issue has.
	born := fixedTime.Add(-3 * time.Hour)
	rec := do(t, h, "ollie", "POST", "/api/issues?at="+born.Format(time.RFC3339),
		map[string]any{"title": "Moves", "team": "platform"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d, body %s", rec.Code, rec.Body)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	when := fixedTime.Add(-time.Hour)
	rec = do(t, h, "ollie", "POST",
		"/api/issues/"+created.ID+"/transition?at="+when.Format(time.RFC3339),
		map[string]any{"to": "in_progress"})
	if rec.Code >= 300 {
		t.Fatalf("transition = %d, body %s", rec.Code, rec.Body)
	}

	got := last(t, s)["at"].(string)
	parsed, _ := time.Parse(time.RFC3339, got)
	if !parsed.Equal(when) {
		t.Fatalf("transition recorded at %s, want %s", parsed, when)
	}
}

// A write dated before the issue existed would describe an issue that did not exist.
func TestBackdatingBeforeCreationIsRefusedOverHTTP(t *testing.T) {
	_, h := newServer(t)

	rec := do(t, h, "ollie", "POST", "/api/issues",
		map[string]any{"title": "Exists from now", "team": "platform"})
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	when := fixedTime.Add(-48 * time.Hour)
	rec = do(t, h, "ollie", "POST",
		"/api/issues/"+created.ID+"/transition?at="+when.Format(time.RFC3339),
		map[string]any{"to": "in_progress"})
	if rec.Code < 300 {
		t.Fatal("a transition dated before the issue was created should be refused")
	}
	if !strings.Contains(rec.Body.String(), "created") {
		t.Fatalf("the refusal should say when the issue was created, got: %s", rec.Body)
	}
}
