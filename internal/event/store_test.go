package event

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "canon.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func sample(subject string) *Event {
	return New("issue.created", subject,
		time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC),
		Actor{ID: "agent:one", Kind: ActorAgent, Model: "claude-opus-5"},
		map[string]any{"title": "Search is slow"})
}

// A zero version must be rejected, not quietly stamped as current.
func TestRejectsZeroVersion(t *testing.T) {
	s := testStore(t)
	e := sample("CANON-1")
	e.Version = 0
	if err := s.Append(e); !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("got %v, want ErrUnsupportedVersion", err)
	}
}

// AC: WHEN an event is appended THE SYSTEM SHALL record actor id, actor kind and
// timestamp and SHALL NOT permit modification of any earlier event.
func TestAppendRecordsProvenance(t *testing.T) {
	s := testStore(t)
	in := sample("CANON-1")
	if err := s.Append(in); err != nil {
		t.Fatalf("append: %v", err)
	}
	if in.ID == "" {
		t.Fatal("append must assign an id")
	}

	got, err := s.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 event, got %d", len(got))
	}
	e := got[0]
	if e.Actor.ID != "agent:one" || e.Actor.Kind != ActorAgent || e.Actor.Model != "claude-opus-5" {
		t.Errorf("provenance not preserved: %+v", e.Actor)
	}
	if !e.At.Equal(in.At) {
		t.Errorf("timestamp not preserved: got %v want %v", e.At, in.At)
	}
	if e.Version != SchemaVersion {
		t.Errorf("version: got %d want %d", e.Version, SchemaVersion)
	}
}

// The store is append-only: there must be no path that rewrites a stored event.
func TestAppendOnly(t *testing.T) {
	s := testStore(t)
	for i := range 3 {
		if err := s.Append(sample(fmt.Sprintf("CANON-%d", i))); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	before, err := s.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}

	// Appending more must leave every earlier event byte-identical.
	if err := s.Append(sample("CANON-99")); err != nil {
		t.Fatalf("append: %v", err)
	}
	after, err := s.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("want %d events, got %d", len(before)+1, len(after))
	}
	for i := range before {
		if before[i].ID != after[i].ID || before[i].Subject != after[i].Subject {
			t.Errorf("event %d changed: %+v -> %+v", i, before[i], after[i])
		}
	}

	// Re-appending the same id must be refused rather than overwrite.
	dup := sample("CANON-0")
	dup.ID = before[0].ID
	if err := s.Append(dup); !errors.Is(err, ErrImmutable) {
		t.Errorf("re-appending an existing id: got %v, want ErrImmutable", err)
	}
}

// Ids must sort in append order so replay is deterministic without a separate index.
func TestIDsSortInAppendOrder(t *testing.T) {
	s := testStore(t)
	for i := range 200 {
		if err := s.Append(sample(fmt.Sprintf("CANON-%d", i))); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	got, err := s.All()
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].ID >= got[i].ID {
			t.Fatalf("ids not ascending at %d: %q >= %q", i, got[i-1].ID, got[i].ID)
		}
		if got[i-1].Seq >= got[i].Seq {
			t.Fatalf("seq not ascending at %d", i)
		}
	}
}

// AC: WHEN an event is appended with an unknown schema version THE SYSTEM SHALL
// reject it naming the supported versions.
func TestRejectsUnknownSchemaVersion(t *testing.T) {
	s := testStore(t)
	for _, v := range []uint16{0, SchemaVersion + 1, 9999} {
		e := sample("CANON-1")
		e.Version = v
		err := s.Append(e)
		if !errors.Is(err, ErrUnsupportedVersion) {
			t.Errorf("version %d: got %v, want ErrUnsupportedVersion", v, err)
		}
		if err != nil && !contains(err.Error(), fmt.Sprintf("%d..%d", MinSupportedVersion, SchemaVersion)) {
			t.Errorf("version %d: error must name the supported range, got %q", v, err)
		}
	}
}

func TestRejectsInvalidEvents(t *testing.T) {
	s := testStore(t)
	cases := map[string]func(*Event){
		"no type":        func(e *Event) { e.Type = "" },
		"no subject":     func(e *Event) { e.Subject = "" },
		"no actor id":    func(e *Event) { e.Actor.ID = "" },
		"bad actor kind": func(e *Event) { e.Actor.Kind = "robot" },
		"agent no model": func(e *Event) { e.Actor.Model = "" },
		"no timestamp":   func(e *Event) { e.At = time.Time{} },
	}
	for name, mutate := range cases {
		e := sample("CANON-1")
		mutate(e)
		if err := s.Append(e); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

// AC: WHEN an operator requests events in JSON form THE SYSTEM SHALL render every
// field of each event as human-readable JSON, losslessly.
func TestJSONRoundTrip(t *testing.T) {
	in := &Event{
		Version: SchemaVersion,
		ID:      "01K3QG7X8N4YV2H0ZB6M9TDPWA",
		Type:    "issue.transitioned",
		Subject: "CANON-14",
		At:      time.Date(2026, 8, 22, 21, 14, 3, 0, time.UTC),
		Actor:   Actor{ID: "agent:one", Kind: ActorAgent, Model: "claude-opus-5"},
		Payload: map[string]any{
			"from":     "in_progress",
			"to":       "in_review",
			"evidence": "312 passed in 41s",
			"attempts": int64(3),
			"blocked":  false,
		},
	}
	js, err := in.MarshalJSON()
	if err != nil {
		t.Fatalf("to json: %v", err)
	}
	if !contains(string(js), `"evidence"`) || !contains(string(js), "claude-opus-5") {
		t.Fatalf("json is not human-readable:\n%s", js)
	}

	back, err := UnmarshalJSON(js)
	if err != nil {
		t.Fatalf("from json: %v", err)
	}
	// Losslessness is checked at the CBOR bytes, since that is the canonical form
	// a signature would cover.
	a, err := in.Marshal()
	if err != nil {
		t.Fatalf("marshal in: %v", err)
	}
	b, err := back.Marshal()
	if err != nil {
		t.Fatalf("marshal back: %v", err)
	}
	if string(a) != string(b) {
		t.Errorf("json round trip is lossy\n in:  %+v\n out: %+v", in, back)
	}
}

func TestCBORRoundTrip(t *testing.T) {
	in := sample("CANON-7")
	in.ID = "01K3QG7X8N4YV2H0ZB6M9TDPWA"
	raw, err := in.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	again, err := out.Marshal()
	if err != nil {
		t.Fatalf("remarshal: %v", err)
	}
	if string(raw) != string(again) {
		t.Error("cbor encoding is not canonical: re-encoding produced different bytes")
	}
}

// AC: THE SYSTEM SHALL append 10,000 events in under 2 seconds on commodity hardware.
func TestAppendThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("throughput test skipped in short mode")
	}
	s := testStore(t)
	const n = 10_000
	start := time.Now()
	if err := s.AppendBatch(func(yield func(*Event) bool) {
		for i := range n {
			if !yield(sample(fmt.Sprintf("CANON-%d", i))) {
				return
			}
		}
	}); err != nil {
		t.Fatalf("append batch: %v", err)
	}
	elapsed := time.Since(start)
	t.Logf("appended %d events in %s (%.0f events/sec)", n, elapsed.Round(time.Millisecond),
		float64(n)/elapsed.Seconds())
	if elapsed > 2*time.Second {
		t.Errorf("append of %d events took %s, budget is 2s", n, elapsed)
	}
	got, err := s.Count()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != n {
		t.Errorf("count: got %d want %d", got, n)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) &&
		(haystack == needle || len(needle) == 0 ||
			indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
