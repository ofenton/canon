package webhook

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/schema"
)

func quiet() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func sender(t *testing.T, hooks ...schema.Webhook) *Sender {
	t.Helper()
	s := New(&schema.Schema{Webhooks: hooks}, quiet())
	if s == nil {
		t.Fatal("expected a sender for a schema with webhooks")
	}
	t.Cleanup(func() { s.Close(5 * time.Second) })
	return s
}

func delivery(to string) Delivery {
	return Delivery{
		Event: "issue.transitioned", Issue: "CANON-1",
		From: "todo", To: to, Actor: "ollie", Kind: "human",
		At: time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC),
	}
}

// AC: WHEN an issue transitions THE SYSTEM SHALL deliver a webhook describing the
// transition.
func TestDeliversTheTransition(t *testing.T) {
	got := make(chan Delivery, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var d Delivery
		if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
			t.Errorf("decode: %v", err)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content-type = %q, want application/json", ct)
		}
		got <- d
	}))
	defer srv.Close()

	s := sender(t, schema.Webhook{URL: srv.URL})
	s.Send(delivery("in_progress"))

	select {
	case d := <-got:
		if d.Issue != "CANON-1" || d.From != "todo" || d.To != "in_progress" {
			t.Fatalf("delivery did not describe the transition: %+v", d)
		}
		if d.Actor != "ollie" || d.Kind != "human" {
			t.Fatalf("delivery lost the provenance: %+v", d)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no delivery arrived")
	}
}

// AC: WHEN delivery fails THE SYSTEM SHALL retry within a bound and never block the
// write.
func TestRetriesWithinABound(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := sender(t, schema.Webhook{URL: srv.URL, Retries: 2})
	s.Send(delivery("in_progress"))
	s.Close(10 * time.Second)

	// One attempt plus two retries, and then it stops. The bound is the point: an
	// unbounded retry against a decommissioned subscriber is a queue that grows for
	// ever.
	if got := attempts.Load(); got != 3 {
		t.Fatalf("made %d attempts, want 1 try plus 2 retries", got)
	}
}

// The write must never wait on somebody else's server. Send returns before the
// subscriber has even been contacted, let alone answered.
func TestSendDoesNotBlockOnASlowSubscriber(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	s := sender(t, schema.Webhook{URL: srv.URL})

	done := make(chan time.Duration, 1)
	go func() {
		start := time.Now()
		s.Send(delivery("in_progress"))
		done <- time.Since(start)
	}()

	select {
	case took := <-done:
		if took > 500*time.Millisecond {
			t.Fatalf("Send took %s; it must not wait on the subscriber", took)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Send blocked on a subscriber that never responds")
	}
}

// A subscriber that has gone away entirely must not be different from a slow one.
func TestAnUnreachableSubscriberIsSurvivable(t *testing.T) {
	// A port nothing is listening on.
	s := sender(t, schema.Webhook{URL: "http://127.0.0.1:1/hook", Retries: 0})
	s.Send(delivery("in_progress"))
	s.Close(5 * time.Second)
	// Reaching here without panic or hang is the assertion.
}

// A target that names states receives only those, so a noisy instance can notify on
// completion without notifying on everything.
func TestStatesNarrowDelivery(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	s := sender(t, schema.Webhook{URL: srv.URL, States: []string{"done"}})
	s.Send(delivery("in_progress"))
	s.Send(delivery("in_review"))
	s.Send(delivery("done"))
	s.Close(5 * time.Second)

	if got := hits.Load(); got != 1 {
		t.Fatalf("got %d deliveries, want only the one into done", got)
	}
}

// A schema with no webhooks produces a nil Sender, and a nil Sender must work.
// Otherwise every call site needs a check, and one of them will eventually not have
// it.
func TestNilSenderIsSafe(t *testing.T) {
	var s *Sender
	if got := New(&schema.Schema{}, quiet()); got != nil {
		t.Fatal("a schema with no webhooks should produce no sender")
	}
	s.Send(delivery("done"))
	s.Close(time.Second)
}
