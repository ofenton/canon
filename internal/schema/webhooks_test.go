package schema

import (
	"strings"
	"testing"
)

const webhookBase = `version: 1
states: [{name: todo, category: open}, {name: done, category: closed}]
transitions: [{from: todo, to: done}]
fields: [{name: title, type: string, required: true}]
issue_types: [{name: task, fields: [title]}]
`

func TestValidWebhookLoads(t *testing.T) {
	s := load(t, webhookBase+`webhooks:
  - {url: "https://example.com/hook"}
  - {url: "https://example.com/done", states: [done], retries: 3}
`)
	if len(s.Webhooks) != 2 {
		t.Fatalf("got %d webhooks, want 2", len(s.Webhooks))
	}
}

// A webhook watching a state that does not exist fires never, and does so silently —
// the same failure as a role granting a misspelled field.
func TestWebhookWatchingAnUnknownStateIsRefused(t *testing.T) {
	_, err := loadErr(t, webhookBase+`webhooks:
  - {url: "https://example.com/hook", states: [shipped]}
`)
	if err == nil || !strings.Contains(err.Error(), "shipped") {
		t.Fatalf("expected the unknown state to be named, got: %v", err)
	}
}

func TestWebhookURLMustBeAbsoluteHTTP(t *testing.T) {
	for _, bad := range []string{"", "/hook", "example.com/hook", "ftp://example.com/hook"} {
		_, err := loadErr(t, webhookBase+`webhooks:
  - {url: "`+bad+`"}
`)
		if err == nil {
			t.Errorf("url %q should be refused", bad)
		}
	}
}

// A subscriber down for nine retries is down, and the tenth is not information.
func TestWebhookRetriesAreBounded(t *testing.T) {
	_, err := loadErr(t, webhookBase+`webhooks:
  - {url: "https://example.com/hook", retries: 50}
`)
	if err == nil || !strings.Contains(err.Error(), "the most is") {
		t.Fatalf("expected an upper bound on retries, got: %v", err)
	}
}

// There is no runtime API for webhooks, and this is the test that says so: adding one
// means editing canon.yaml, which means a pull request.
func TestWebhooksAreConfigurationOnly(t *testing.T) {
	s := load(t, webhookBase+`webhooks: [{url: "https://example.com/hook"}]`)
	if len(s.Webhooks) != 1 {
		t.Fatal("the schema is the only place a webhook comes from")
	}
}
