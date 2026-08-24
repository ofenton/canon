// Package event defines Canon's append-only event log.
//
// Canon stores events, not state. Current state is a projection produced by
// replaying the log (ADR-0003). This makes history inherent rather than a feature
// bolted on, and it is what allows a log to later live in a git ref without a
// rewrite — appends commute, so two clones merge by concatenation.
//
// The encoding is canonical CBOR (RFC 8949 §4.2.1). Canonical form matters because
// events carry provenance and may be signed: a signature is only verifiable if two
// encoders produce identical bytes for the same event. JSON has no canonical form,
// which is why it is not used here despite being easier to read.
package event

import (
	"errors"
	"fmt"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// SchemaVersion is the current event envelope version.
//
// Everything federates on this envelope, so it is the one part of Canon that cannot
// be changed cheaply. Readers must reject versions they do not understand rather
// than guess, because a misread event silently corrupts every projection built from it.
const SchemaVersion uint16 = 1

// MinSupportedVersion is the oldest envelope this build can decode.
const MinSupportedVersion uint16 = 1

// ActorKind distinguishes who caused an event. Provenance is not optional: an agent's
// write and a human's write are different facts, and telling them apart after the fact
// is impossible if it was not recorded at the time.
type ActorKind string

const (
	ActorHuman ActorKind = "human"
	ActorAgent ActorKind = "agent"
	// ActorSystem covers events the server originates, such as a schema migration.
	ActorSystem ActorKind = "system"
)

// Valid reports whether k is a known actor kind.
func (k ActorKind) Valid() bool {
	switch k {
	case ActorHuman, ActorAgent, ActorSystem:
		return true
	}
	return false
}

// Actor identifies who produced an event.
type Actor struct {
	// ID is stable across events for the same human or agent.
	ID string `cbor:"1,keyasint" json:"id"`
	// Kind is human, agent or system.
	Kind ActorKind `cbor:"2,keyasint" json:"kind"`
	// Model records which model acted, for agents. Empty for humans.
	Model string `cbor:"3,keyasint,omitempty" json:"model,omitempty"`
}

// Event is one immutable fact. Once appended it is never modified; a correction is
// a later event, not an edit.
type Event struct {
	// Version is the envelope schema version.
	Version uint16 `cbor:"1,keyasint"`
	// ID is a unique, sortable identifier assigned at append time.
	ID string `cbor:"2,keyasint"`
	// Type names the fact, such as "issue.created" or "issue.transitioned".
	Type string `cbor:"3,keyasint"`
	// Subject is the entity the fact is about, such as an issue id.
	Subject string `cbor:"4,keyasint"`
	// At is when the fact occurred, UTC. This may predate the append, so that a
	// commit can be linked retroactively without lying about when the work happened.
	At time.Time `cbor:"5,keyasint"`
	// Actor is who caused it.
	Actor Actor `cbor:"6,keyasint"`
	// Payload holds type-specific fields.
	Payload map[string]any `cbor:"7,keyasint,omitempty"`

	// Seq is the store-assigned append position. It is not part of the signed
	// envelope: the same event appended to two clones is the same fact at
	// different positions, so it must not affect the canonical bytes.
	Seq int64 `cbor:"-"`
}

// timeFormat is RFC 3339 with nanoseconds, always UTC.
const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(timeFormat, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing timestamp %q: %w", s, err)
	}
	return t.UTC(), nil
}

// New returns an event stamped with the current schema version.
//
// The store never defaults a missing version: an event whose version is zero is
// rejected rather than assumed to be current. Silently stamping a zero would let a
// decoder bug write events that claim to be valid, into a log that cannot be edited.
func New(typ, subject string, at time.Time, actor Actor, payload map[string]any) *Event {
	return &Event{
		Version: SchemaVersion,
		Type:    typ,
		Subject: subject,
		At:      at.UTC(),
		Actor:   actor,
		Payload: payload,
	}
}

var (
	ErrUnsupportedVersion = errors.New("unsupported event schema version")
	ErrInvalid            = errors.New("invalid event")
)

// encMode produces canonical CBOR so that identical events encode to identical bytes.
var encMode = func() cbor.EncMode {
	m, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		panic(fmt.Sprintf("event: building canonical CBOR encoder: %v", err))
	}
	return m
}()

// Validate reports why an event may not be appended, or nil if it may.
func (e *Event) Validate() error {
	switch {
	case e.Version < MinSupportedVersion || e.Version > SchemaVersion:
		return fmt.Errorf("%w: got %d, this build supports %d..%d",
			ErrUnsupportedVersion, e.Version, MinSupportedVersion, SchemaVersion)
	case e.Type == "":
		return fmt.Errorf("%w: type is required", ErrInvalid)
	case e.Subject == "":
		return fmt.Errorf("%w: subject is required", ErrInvalid)
	case e.Actor.ID == "":
		return fmt.Errorf("%w: actor id is required", ErrInvalid)
	case !e.Actor.Kind.Valid():
		return fmt.Errorf("%w: actor kind %q is not one of human, agent, system",
			ErrInvalid, e.Actor.Kind)
	case e.Actor.Kind == ActorAgent && e.Actor.Model == "":
		return fmt.Errorf("%w: agent actors must record a model identifier", ErrInvalid)
	case e.At.IsZero():
		return fmt.Errorf("%w: at is required", ErrInvalid)
	}
	return nil
}

// Marshal encodes e as canonical CBOR.
func (e *Event) Marshal() ([]byte, error) {
	return encMode.Marshal(e)
}

// Unmarshal decodes an event, rejecting envelope versions this build cannot read.
func Unmarshal(data []byte) (*Event, error) {
	var e Event
	if err := cbor.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("decoding event: %w", err)
	}
	if e.Version < MinSupportedVersion || e.Version > SchemaVersion {
		return nil, fmt.Errorf("%w: got %d, this build supports %d..%d",
			ErrUnsupportedVersion, e.Version, MinSupportedVersion, SchemaVersion)
	}
	// CBOR round-trips time as UTC-normalised; keep it explicit for comparisons.
	e.At = e.At.UTC()
	return &e, nil
}
