package event

import (
	"encoding/json"
	"fmt"
)

// Canonical CBOR is compact and byte-stable, which is what a signature needs, but it
// is unreadable by eye. These render the same event as JSON so a log can be inspected,
// diffed and pasted into a bug report without special tooling.
//
// The JSON form is a view, not a second source of truth: it round-trips back to
// identical CBOR bytes, which the tests assert.

// jsonEvent mirrors Event with names instead of the integer CBOR keys.
type jsonEvent struct {
	Version uint16         `json:"version"`
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Subject string         `json:"subject"`
	At      string         `json:"at"`
	Actor   jsonActor      `json:"actor"`
	Payload map[string]any `json:"payload,omitempty"`
	Seq     int64          `json:"seq,omitempty"`
}

type jsonActor struct {
	ID    string    `json:"id"`
	Kind  ActorKind `json:"kind"`
	Model string    `json:"model,omitempty"`
}

// MarshalJSON renders the event as human-readable JSON.
func (e *Event) MarshalJSON() ([]byte, error) {
	return json.Marshal(jsonEvent{
		Version: e.Version,
		ID:      e.ID,
		Type:    e.Type,
		Subject: e.Subject,
		At:      e.At.UTC().Format(timeFormat),
		Actor:   jsonActor{ID: e.Actor.ID, Kind: e.Actor.Kind, Model: e.Actor.Model},
		Payload: e.Payload,
		Seq:     e.Seq,
	})
}

// UnmarshalJSON parses the human-readable form back into an event.
func UnmarshalJSON(data []byte) (*Event, error) {
	var j jsonEvent
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("decoding event json: %w", err)
	}
	at, err := parseTime(j.At)
	if err != nil {
		return nil, fmt.Errorf("decoding event json: %w", err)
	}
	e := &Event{
		Version: j.Version,
		ID:      j.ID,
		Type:    j.Type,
		Subject: j.Subject,
		At:      at,
		Actor:   Actor{ID: j.Actor.ID, Kind: j.Actor.Kind, Model: j.Actor.Model},
		Payload: normaliseJSONNumbers(j.Payload),
		Seq:     j.Seq,
	}
	return e, nil
}

// normaliseJSONNumbers converts JSON's float64-for-everything back to integers where
// the value is integral. Without this a payload that arrived as int64 returns as
// float64 and re-encodes to different CBOR bytes, which would make the round trip lossy.
func normaliseJSONNumbers(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		switch n := v.(type) {
		case float64:
			if n == float64(int64(n)) {
				out[k] = int64(n)
				continue
			}
			out[k] = n
		case map[string]any:
			out[k] = normaliseJSONNumbers(n)
		default:
			out[k] = v
		}
	}
	return out
}
