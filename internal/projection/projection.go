// Package projection rebuilds current state by replaying the event log.
//
// The projection is a cache with no authority (ADR-0003). It can be discarded and
// rebuilt at any time, which is what turns a projection bug into a five-minute fix
// rather than a data repair script. Nothing here may write to the log.
package projection

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/event"
)

// Issue is the projected current state of one issue.
type Issue struct {
	ID        string
	Title     string
	State     string
	Parent    string
	Fields    map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
	// LastActor is who most recently changed this issue. Provenance is projected,
	// not just stored, so "who last touched this" costs no replay to answer.
	LastActor event.Actor
	// Transitions is the ordered state history, which is where cycle time comes from.
	Transitions []Transition
}

// Transition records one state change.
type Transition struct {
	From, To string
	At       time.Time
	Actor    event.Actor
}

// Checkpoint is a projection's position in the log plus its materialised state.
// Restoring one avoids replaying the whole log.
type Checkpoint struct {
	Seq    int64
	Issues map[string]*Issue
}

// Projection is the materialised view over an event log.
type Projection struct {
	log    *event.Store
	issues map[string]*Issue
	seq    int64
	read   int64
}

// New returns an empty projection over log. Call Rebuild or Restore before reading.
func New(log *event.Store) *Projection {
	return &Projection{log: log, issues: map[string]*Issue{}}
}

// Rebuild discards all state and replays the log from the beginning.
func (p *Projection) Rebuild() error {
	p.issues = map[string]*Issue{}
	p.seq = 0
	return p.Catchup()
}

// Catchup applies every event after the current position.
func (p *Projection) Catchup() error {
	events, err := p.log.Since(p.seq)
	if err != nil {
		return fmt.Errorf("reading log from seq %d: %w", p.seq, err)
	}
	for _, e := range events {
		if err := p.apply(e); err != nil {
			return err
		}
		p.seq = e.Seq
		p.read++
	}
	return nil
}

// Restore adopts a checkpoint, so replay resumes from that position.
func (p *Projection) Restore(cp Checkpoint) error {
	p.issues = make(map[string]*Issue, len(cp.Issues))
	for id, issue := range cp.Issues {
		clone := *issue
		clone.Fields = maps(issue.Fields)
		clone.Transitions = append([]Transition(nil), issue.Transitions...)
		p.issues[id] = &clone
	}
	p.seq = cp.Seq
	return nil
}

// Checkpoint captures the current position and state.
func (p *Projection) Checkpoint() Checkpoint {
	issues := make(map[string]*Issue, len(p.issues))
	for id, issue := range p.issues {
		clone := *issue
		clone.Fields = maps(issue.Fields)
		clone.Transitions = append([]Transition(nil), issue.Transitions...)
		issues[id] = &clone
	}
	return Checkpoint{Seq: p.seq, Issues: issues}
}

// EventsRead reports how many events this projection has applied, so tests can
// prove a checkpoint actually bounded the replay rather than merely appearing to.
func (p *Projection) EventsRead() int64 { return p.read }

// Issue returns the projected state of one issue.
func (p *Projection) Issue(id string) (*Issue, bool) {
	issue, ok := p.issues[id]
	return issue, ok
}

// Children returns the ids of issues whose parent is id, in sorted order.
func (p *Projection) Children(id string) []string {
	var out []string
	for childID, issue := range p.issues {
		if issue.Parent == id {
			out = append(out, childID)
		}
	}
	sort.Strings(out)
	return out
}

// IssueIDs returns every projected issue id, sorted.
func (p *Projection) IssueIDs() []string {
	out := make([]string, 0, len(p.issues))
	for id := range p.issues {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Snapshot returns a stable digest of the whole projection.
//
// Determinism is the property that makes a rebuildable cache trustworthy: if two
// replays of the same log disagree, the projection has a bug and every downstream
// answer is suspect. A digest makes that comparable in a test.
func (p *Projection) Snapshot() string {
	ids := make([]string, 0, len(p.issues))
	for id := range p.issues {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	h := sha256.New()
	for _, id := range ids {
		issue := p.issues[id]
		fmt.Fprintf(h, "%s\x1f%s\x1f%s\x1f%s\x1f%d\x1f%d\x1f%s\x1f%s\x1f%s\n",
			issue.ID, issue.Title, issue.State, issue.Parent,
			issue.CreatedAt.UnixNano(), issue.UpdatedAt.UnixNano(),
			issue.LastActor.ID, issue.LastActor.Kind, issue.LastActor.Model)
		for _, k := range sortedKeys(issue.Fields) {
			fmt.Fprintf(h, "  %s=%s\n", k, issue.Fields[k])
		}
		for _, t := range issue.Transitions {
			fmt.Fprintf(h, "  %s>%s@%d\n", t.From, t.To, t.At.UnixNano())
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// apply folds one event into the current state.
//
// An unrecognised type is an error, never a skip. Silently ignoring an event means
// the projection quietly disagrees with the log, which is the one failure mode a
// rebuildable cache cannot detect on its own.
func (p *Projection) apply(e *event.Event) error {
	switch e.Type {
	case "issue.created":
		issue := &Issue{
			ID:        e.Subject,
			Title:     str(e.Payload["title"]),
			State:     str(e.Payload["state"]),
			Fields:    map[string]string{},
			CreatedAt: e.At,
		}
		p.issues[e.Subject] = issue
		p.touch(issue, e)

	case "field.set":
		issue, err := p.require(e)
		if err != nil {
			return err
		}
		field, value := str(e.Payload["field"]), str(e.Payload["value"])
		switch field {
		case "title":
			issue.Title = value
		case "":
			return fmt.Errorf("event %s: field.set with no field name", e.ID)
		default:
			issue.Fields[field] = value
		}
		p.touch(issue, e)

	case "issue.transitioned":
		issue, err := p.require(e)
		if err != nil {
			return err
		}
		from, to := str(e.Payload["from"]), str(e.Payload["to"])
		if to == "" {
			return fmt.Errorf("event %s: issue.transitioned with no target state", e.ID)
		}
		issue.State = to
		issue.Transitions = append(issue.Transitions, Transition{
			From: from, To: to, At: e.At, Actor: e.Actor,
		})
		p.touch(issue, e)

	case "issue.reparented":
		issue, err := p.require(e)
		if err != nil {
			return err
		}
		issue.Parent = str(e.Payload["parent"])
		p.touch(issue, e)

	case "issue.deleted":
		// Deletion is a tombstone. The events stay in the log — history is not
		// rewritten — but the issue leaves the projected present.
		if _, ok := p.issues[e.Subject]; !ok {
			return fmt.Errorf("event %s deletes unknown issue %q", e.ID, e.Subject)
		}
		delete(p.issues, e.Subject)

	default:
		return fmt.Errorf("event %s at seq %d: unknown type %q — "+
			"the projection must be taught this type before it can replay this log",
			e.ID, e.Seq, e.Type)
	}
	return nil
}

// require returns the issue an event is about, or an error naming the gap. An event
// about an issue that was never created means the log is inconsistent, not that the
// event should be dropped.
func (p *Projection) require(e *event.Event) (*Issue, error) {
	issue, ok := p.issues[e.Subject]
	if !ok {
		return nil, fmt.Errorf("event %s (%s) refers to unknown issue %q",
			e.ID, e.Type, e.Subject)
	}
	return issue, nil
}

// touch records when and by whom an issue last changed.
func (p *Projection) touch(issue *Issue, e *event.Event) {
	issue.UpdatedAt = e.At
	issue.LastActor = e.Actor
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func maps(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
