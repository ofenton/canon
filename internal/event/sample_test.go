package event

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// TestSampleEvent prints a representative event in both encodings, so the schema
// can be reviewed as data rather than as a struct definition.
func TestSampleEvent(t *testing.T) {
	e := &Event{
		Version: SchemaVersion,
		ID:      "01K3QG7X8N4YV2H0ZB6M9TDPWA",
		Type:    "issue.transitioned",
		Subject: "CANON-14",
		At:      time.Date(2026, 8, 22, 21, 14, 3, 0, time.UTC),
		Actor: Actor{
			ID:    "agent:claude-code-01",
			Kind:  ActorAgent,
			Model: "claude-opus-5",
		},
		Payload: map[string]any{
			"from":     "in_progress",
			"to":       "in_review",
			"evidence": "pytest -q → 312 passed in 41s",
		},
	}
	if err := e.Validate(); err != nil {
		t.Fatalf("sample event must be valid: %v", err)
	}
	raw, err := e.Marshal()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	pretty, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	fmt.Printf("\n--- canonical CBOR (%d bytes) ---\n%s\n\n--- decoded ---\n%s\n\n",
		len(raw), hex.EncodeToString(raw), pretty)
}
