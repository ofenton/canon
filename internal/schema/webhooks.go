package schema

import (
	"net/url"
	"strings"
)

// Webhooks.
//
// Where a notification goes is org policy, so it lives in canon.yaml like everything
// else. It is not a per-project setting, and there is no runtime API to add one: a
// tracker that lets any team point a firehose of state changes at any URL is how an
// integration nobody remembers configuring ends up leaking work items to a defunct
// vendor.

// Webhook is one subscriber to state changes.
type Webhook struct {
	URL string `yaml:"url" json:"url"`
	// States narrows delivery to transitions into these states. Empty means all of
	// them, which is what somebody reaching for this first expects.
	States []string `yaml:"states,omitempty" json:"states,omitempty"`
	// Retries is how many times to try again after the first failure. Zero means one
	// attempt and no retry, which is a legitimate choice for a noisy subscriber.
	Retries int `yaml:"retries,omitempty" json:"retries,omitempty"`

	line int
}

// maxRetries bounds what a schema may ask for. A subscriber that has been down for
// nine retries is down, and the tenth is not information.
const maxRetries = 5

func (s *Schema) validateWebhooks(add func(string, ...any)) {
	for _, w := range s.Webhooks {
		switch {
		case strings.TrimSpace(w.URL) == "":
			add("line %d: a webhook has no url", w.line)
			continue
		default:
			u, err := url.Parse(w.URL)
			if err != nil || u.Scheme == "" || u.Host == "" {
				add("line %d: webhook url %q is not an absolute http or https url", w.line, w.URL)
				continue
			}
			if u.Scheme != "http" && u.Scheme != "https" {
				add("line %d: webhook url %q uses scheme %q; only http and https are delivered",
					w.line, w.URL, u.Scheme)
			}
		}

		// A webhook naming a state that does not exist fires never, and does so
		// silently — the same failure mode as a role granting a misspelled field.
		for _, name := range w.States {
			if !s.HasState(name) {
				add("line %d: webhook %s watches state %q, which is not defined", w.line, w.URL, name)
			}
		}
		if w.Retries < 0 {
			add("line %d: webhook %s has negative retries", w.line, w.URL)
		}
		if w.Retries > maxRetries {
			add("line %d: webhook %s asks for %d retries; the most is %d",
				w.line, w.URL, w.Retries, maxRetries)
		}
	}
}
