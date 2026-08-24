// Package webhook delivers a notification when an issue changes state.
//
// The design constraint is that a write must never wait on somebody else's server.
// A tracker whose transitions get slower because an integration is having a bad day
// is a tracker people stop using, and an outbound call on the write path is the most
// common way that happens. So delivery is asynchronous, bounded, and droppable: the
// event is already in the log, and the log is the record. A webhook is a courtesy.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ofenton/canon/internal/schema"
)

// Delivery is what a subscriber receives.
type Delivery struct {
	Event    string    `json:"event"`
	Issue    string    `json:"issue"`
	From     string    `json:"from,omitempty"`
	To       string    `json:"to"`
	Actor    string    `json:"actor"`
	Kind     string    `json:"actor_kind"`
	Model    string    `json:"model,omitempty"`
	Team     string    `json:"team,omitempty"`
	Evidence string    `json:"evidence,omitempty"`
	At       time.Time `json:"at"`
}

// Sender delivers notifications. A nil Sender is a working Sender that does nothing,
// so a schema with no webhooks needs no special case at the call site.
type Sender struct {
	targets []schema.Webhook
	client  *http.Client
	log     *slog.Logger

	// wg tracks in-flight deliveries so Close can wait for them. Without it a
	// short-lived process exits with its notifications half-sent, which looks
	// exactly like a bug in the subscriber.
	wg sync.WaitGroup

	// inFlight bounds concurrency. An instance transitioning a thousand issues in a
	// migration must not open a thousand sockets to somebody else's server.
	inFlight chan struct{}
}

// maxConcurrent is how many deliveries may be open at once.
const maxConcurrent = 8

// New builds a Sender for a schema's webhooks, or nil if it declares none.
func New(s *schema.Schema, log *slog.Logger) *Sender {
	if len(s.Webhooks) == 0 {
		return nil
	}
	return &Sender{
		targets:  s.Webhooks,
		client:   &http.Client{Timeout: 10 * time.Second},
		log:      log,
		inFlight: make(chan struct{}, maxConcurrent),
	}
}

// Send delivers a notification to every interested target, and returns immediately.
//
// Nothing about the caller's write depends on the outcome. A failure is logged, not
// returned: there is no answer the transition could give to "the subscriber is down"
// except to carry on.
func (s *Sender) Send(d Delivery) {
	if s == nil {
		return
	}
	body, err := json.Marshal(d)
	if err != nil {
		s.log.Error("webhook payload could not be encoded", "issue", d.Issue, "err", err)
		return
	}

	for _, target := range s.targets {
		if !wants(target, d.To) {
			continue
		}
		s.wg.Add(1)
		go func(t schema.Webhook) {
			defer s.wg.Done()
			s.inFlight <- struct{}{}
			defer func() { <-s.inFlight }()
			s.deliver(t, body, d)
		}(target)
	}
}

// wants reports whether a target cares about a transition into this state.
//
// No states listed means every transition, which is the setting somebody reaches for
// first and the one they should get without reading anything.
func wants(t schema.Webhook, to string) bool {
	if len(t.States) == 0 {
		return true
	}
	for _, s := range t.States {
		if s == to {
			return true
		}
	}
	return false
}

// deliver posts once and retries a bounded number of times.
//
// The bound is the point. An unbounded retry against a subscriber that has been
// decommissioned is a queue that grows for ever, and the usual outcome is somebody
// discovering it months later having filled a disk.
func (s *Sender) deliver(t schema.Webhook, body []byte, d Delivery) {
	attempts := t.Retries + 1
	backoff := 250 * time.Millisecond

	for attempt := 1; attempt <= attempts; attempt++ {
		err := s.post(t, body)
		if err == nil {
			return
		}
		if attempt == attempts {
			// Logged with everything needed to replay it by hand, because the
			// delivery is gone and the transition is not.
			s.log.Error("webhook delivery failed, giving up",
				"url", t.URL, "issue", d.Issue, "to", d.To,
				"attempts", attempts, "err", err)
			return
		}
		s.log.Warn("webhook delivery failed, retrying",
			"url", t.URL, "issue", d.Issue, "attempt", attempt, "err", err)
		time.Sleep(backoff)
		backoff *= 2
	}
}

func (s *Sender) post(t schema.Webhook, body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "canon-webhook")

	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s returned %d", t.URL, res.StatusCode)
	}
	return nil
}

// Close waits for in-flight deliveries, bounded so shutdown cannot hang on somebody
// else's slow server.
func (s *Sender) Close(within time.Duration) {
	if s == nil {
		return
	}
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(within):
		s.log.Warn("shutting down with webhook deliveries still in flight")
	}
}
