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
	ID     string `json:"id"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Parent string `json:"parent,omitempty"`
	// Type is the issue type declared at creation, from canon.yaml.
	Type string `json:"type"`
	// Team owns the issue. Team-scoped roles resolve against it.
	Team      string            `json:"team,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	// LastActor is who most recently changed this issue. Provenance is projected,
	// not just stored, so "who last touched this" costs no replay to answer.
	LastActor event.Actor `json:"last_actor"`
	// Transitions is the ordered state history, which is where cycle time comes from.
	Transitions []Transition `json:"transitions,omitempty"`
	// DependsOn lists the issues this one waits on, sorted.
	DependsOn []string `json:"depends_on,omitempty"`
	// Multi holds multi-valued fields, each sorted.
	Multi map[string][]string `json:"multi,omitempty"`
	// Checklists holds checkable items per checklist field, in the order added.
	Checklists map[string][]ChecklistItem `json:"checklists,omitempty"`
	// Commits lists the commits linked to this issue, oldest first by author time.
	Commits []Commit `json:"commits,omitempty"`
}

// Commit is one commit linked to an issue.
//
// The link is a fact about work that was done, so it carries the commit's own
// author time rather than the moment somebody got round to recording it. That is
// what makes "this was built in March" survive being tracked in August.
type Commit struct {
	SHA        string    `json:"sha"`
	Message    string    `json:"message,omitempty"`
	Repository string    `json:"repository,omitempty"`
	Branch     string    `json:"branch,omitempty"`
	Author     string    `json:"author,omitempty"`
	At         time.Time `json:"at"`
	// LinkedBy is who recorded the link, which is not always the commit's author —
	// an operator sweeping a backlog is the common case.
	LinkedBy string `json:"linked_by,omitempty"`
}

// ChecklistItem is one acceptance criterion, and whether it has been met.
//
// Items are their own events rather than a value inside a field, so the log records
// who checked what and when. "Three of five met" is then a count over data, not a
// sentence someone wrote.
type ChecklistItem struct {
	Text      string    `json:"text"`
	Checked   bool      `json:"checked"`
	CheckedBy string    `json:"checked_by,omitempty"`
	CheckedAt time.Time `json:"checked_at,omitzero"`
}

// Transition records one state change.
type Transition struct {
	From  string      `json:"from,omitempty"`
	To    string      `json:"to"`
	At    time.Time   `json:"at"`
	Actor event.Actor `json:"actor"`
}

// Checkpoint is a projection's position in the log plus its materialised state.
// Restoring one avoids replaying the whole log.
type Checkpoint struct {
	Seq    int64
	Issues map[string]*Issue
}

// Actor is the projected identity of a human or agent: who they are, what roles
// they hold and which teams they belong to.
//
// Identity is state, not policy. Which roles exist is declared in canon.yaml and
// changed by pull request; who holds one changes weekly and belongs in the log.
type Actor struct {
	ID    string
	Kind  event.ActorKind
	Model string
	Roles []string
	Teams []string
}

// ProposalStatus is where a proposal has got to.
type ProposalStatus string

const (
	ProposalOpen     ProposalStatus = "open"
	ProposalApproved ProposalStatus = "approved"
	ProposalRejected ProposalStatus = "rejected"
)

// Proposal is an operation an agent was not permitted to perform, recorded for a
// human to decide on.
//
// It is a first-class record rather than a failed request, because the useful
// artifact is the attempt: what the agent wanted to do, on what, with what evidence.
type Proposal struct {
	ID         string
	Subject    string
	Operation  string
	Evidence   string
	ProposedBy string
	Model      string
	Role       string
	ProposedAt time.Time
	Status     ProposalStatus
	DecidedBy  string
	DecidedAt  time.Time
	Reason     string
	// From is the state the subject was in when the proposal was made. An approval
	// that no longer starts from here is stale.
	From string
	To   string
}

// Board is a saved query and a grouping key.
//
// It holds no membership. An issue appears on a board because it matches, and leaves
// because it stops matching — there is nothing to update and nothing to go stale.
// That is the whole difference between a board and a second copy of the truth.
type Board struct {
	Name      string    `json:"name"`
	Query     string    `json:"query"`
	GroupBy   string    `json:"group_by,omitempty"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Projection is the materialised view over an event log.
type Projection struct {
	log       *event.Store
	issues    map[string]*Issue
	actors    map[string]*Actor
	proposals map[string]*Proposal
	boards    map[string]*Board

	// sortedIDs caches the issue id ordering. Every list read needs it, and
	// re-sorting per request is O(n log n) work repeated for a set that only
	// changes when an issue is created or deleted.
	sortedIDs []string

	seq  int64
	read int64
}

// New returns an empty projection over log. Call Rebuild or Restore before reading.
func New(log *event.Store) *Projection {
	return &Projection{log: log, issues: map[string]*Issue{},
		actors: map[string]*Actor{}, proposals: map[string]*Proposal{},
		boards: map[string]*Board{}}
}

// Rebuild discards all state and replays the log from the beginning.
func (p *Projection) Rebuild() error {
	p.issues = map[string]*Issue{}
	p.sortedIDs = nil
	p.actors = map[string]*Actor{}
	p.proposals = map[string]*Proposal{}
	p.boards = map[string]*Board{}
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
	p.sortedIDs = nil
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

// Dependents returns the issues that depend on id, sorted.
//
// The reverse direction is the question people actually ask — "what am I holding
// up?" — and it is the one Jira makes hardest. It is computed rather than stored:
// a second index would be another thing to keep in step with the first.
func (p *Projection) Dependents(id string) []string {
	var out []string
	for otherID, issue := range p.issues {
		for _, on := range issue.DependsOn {
			if on == id {
				out = append(out, otherID)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// Blocked reports whether any issue this one depends on is not closed, and names
// them. Derived, never stored: a "blocked" field would be one more thing to get
// wrong, and it is already implied by the data.
func (p *Projection) Blocked(id string, closed func(state string) bool) (bool, []string) {
	issue, ok := p.issues[id]
	if !ok {
		return false, nil
	}
	var blockers []string
	for _, on := range issue.DependsOn {
		other, exists := p.issues[on]
		if !exists {
			// A dependency on something deleted no longer blocks anything, but it
			// is not silently dropped either — the relation stays in the log.
			continue
		}
		if !closed(other.State) {
			blockers = append(blockers, on)
		}
	}
	sort.Strings(blockers)
	return len(blockers) > 0, blockers
}

// DependencyCycles returns every cycle in the dependency graph, each as the path
// that closes it, deduplicated and in a stable order.
//
// Cycles are permitted here, unlike in the hierarchy: a parent chain is a tree by
// definition, so a cycle is meaningless, but two pieces of work genuinely can wait
// on each other. Refusing to record that forces people to lie or to track it
// somewhere else. Recording it and reporting it loudly is the honest option — a
// cycle means nothing in it can start.
func (p *Projection) DependencyCycles() [][]string {
	var cycles [][]string
	seenCycle := map[string]bool{}

	const (
		white = 0
		grey  = 1
		black = 2
	)
	colour := make(map[string]int, len(p.issues))

	var visit func(id string, path []string)
	visit = func(id string, path []string) {
		colour[id] = grey
		path = append(path, id)
		issue := p.issues[id]
		if issue != nil {
			for _, on := range issue.DependsOn {
				if _, exists := p.issues[on]; !exists {
					continue
				}
				switch colour[on] {
				case grey:
					cycle := append([]string(nil), path[indexOf(path, on):]...)
					if key := cycleKey(cycle); !seenCycle[key] {
						seenCycle[key] = true
						cycles = append(cycles, cycle)
					}
				case white:
					visit(on, path)
				}
			}
		}
		colour[id] = black
	}

	for _, id := range p.IssueIDs() {
		if colour[id] == white {
			visit(id, nil)
		}
	}
	sort.Slice(cycles, func(i, j int) bool {
		return strings.Join(cycles[i], ",") < strings.Join(cycles[j], ",")
	})
	return cycles
}

func indexOf(path []string, id string) int {
	for i, v := range path {
		if v == id {
			return i
		}
	}
	return 0
}

// cycleKey identifies a cycle regardless of which member it was entered from, so
// the same loop is not reported once per participant.
func cycleKey(cycle []string) string {
	sorted := append([]string(nil), cycle...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// Ancestors returns the chain from an issue's parent up to its root, nearest first.
//
// The walk carries a seen-set even though Reparent refuses cycles: a projection
// replaying a log written by an older build must not spin, and an infinite loop in
// a read path is worse than a wrong answer.
func (p *Projection) Ancestors(id string) []string {
	var out []string
	seen := map[string]bool{id: true}
	issue, ok := p.issues[id]
	for ok && issue.Parent != "" {
		if seen[issue.Parent] {
			break
		}
		seen[issue.Parent] = true
		out = append(out, issue.Parent)
		issue, ok = p.issues[issue.Parent]
	}
	return out
}

// Descendants returns every issue beneath id, to maxDepth levels. A maxDepth of
// zero or less means unlimited.
//
// The order is depth-first, so a child immediately follows its parent and a caller
// can render a tree by indenting each entry by its depth. Returning them sorted by
// id instead would put siblings together but separate them from their parents, which
// makes the one thing callers want to do with a subtree unnecessarily hard.
// Siblings are visited in id order, so the result is still deterministic.
func (p *Projection) Descendants(id string, maxDepth int) []string {
	var out []string
	seen := map[string]bool{id: true}

	var walk func(parent string, depth int)
	walk = func(parent string, depth int) {
		if maxDepth > 0 && depth >= maxDepth {
			return
		}
		for _, child := range p.Children(parent) {
			if seen[child] {
				continue
			}
			seen[child] = true
			out = append(out, child)
			walk(child, depth+1)
		}
	}
	walk(id, 0)
	return out
}

// Depth returns how far an issue sits below its root.
func (p *Projection) Depth(id string) int { return len(p.Ancestors(id)) }

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
//
// The result is cached and invalidated only when the set of issues changes, so a
// read costs a slice copy rather than a sort. Callers get a copy because the cache
// must not be mutable from outside.
func (p *Projection) IssueIDs() []string {
	if p.sortedIDs == nil {
		ids := make([]string, 0, len(p.issues))
		for id := range p.issues {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		p.sortedIDs = ids
	}
	out := make([]string, len(p.sortedIDs))
	copy(out, p.sortedIDs)
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
		fmt.Fprintf(h, "%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%d\x1f%d\x1f%s\x1f%s\x1f%s\n",
			issue.ID, issue.Title, issue.State, issue.Type, issue.Parent, issue.Team,
			issue.CreatedAt.UnixNano(), issue.UpdatedAt.UnixNano(),
			issue.LastActor.ID, issue.LastActor.Kind, issue.LastActor.Model)
		for _, k := range sortedKeys(issue.Fields) {
			fmt.Fprintf(h, "  %s=%s\n", k, issue.Fields[k])
		}
		for _, t := range issue.Transitions {
			fmt.Fprintf(h, "  %s>%s@%d\n", t.From, t.To, t.At.UnixNano())
		}
		for _, on := range issue.DependsOn {
			fmt.Fprintf(h, "  depends>%s\n", on)
		}
		for _, k := range sortedMultiKeys(issue.Multi) {
			fmt.Fprintf(h, "  multi %s=%s\n", k, strings.Join(issue.Multi[k], ","))
		}
		for _, k := range sortedChecklistKeys(issue.Checklists) {
			for _, item := range issue.Checklists[k] {
				fmt.Fprintf(h, "  check %s[%s]=%t@%s\n", k, item.Text, item.Checked, item.CheckedBy)
			}
		}
	}
	for _, proposal := range p.Proposals("") {
		fmt.Fprintf(h, "proposal\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s\n",
			proposal.ID, proposal.Subject, proposal.Operation,
			proposal.Status, proposal.ProposedBy, proposal.DecidedBy)
	}
	for _, board := range p.Boards() {
		fmt.Fprintf(h, "board\x1f%s\x1f%s\x1f%s\n", board.Name, board.Query, board.GroupBy)
	}
	for _, id := range p.ActorIDs() {
		actor := p.actors[id]
		fmt.Fprintf(h, "actor\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s\n",
			actor.ID, actor.Kind, actor.Model,
			strings.Join(actor.Roles, ","), strings.Join(actor.Teams, ","))
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
			ID:         e.Subject,
			Title:      str(e.Payload["title"]),
			State:      str(e.Payload["state"]),
			Type:       str(e.Payload["type"]),
			Fields:     map[string]string{},
			Multi:      map[string][]string{},
			Checklists: map[string][]ChecklistItem{},
			CreatedAt:  e.At,
		}
		// Everything else in the payload is a schema field. Reading only the two
		// named above silently dropped fields supplied at creation, which surfaced
		// only when a query tried to filter on one.
		for key, value := range e.Payload {
			switch key {
			case "title", "state", "type":
				continue
			}
			issue.Fields[key] = str(value)
		}
		p.issues[e.Subject] = issue
		p.sortedIDs = nil
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

	case "issue.team_set":
		issue, err := p.require(e)
		if err != nil {
			return err
		}
		issue.Team = str(e.Payload["team"])
		p.touch(issue, e)

	case "board.created":
		if _, exists := p.boards[e.Subject]; exists {
			return fmt.Errorf("event %s creates board %q, which already exists", e.ID, e.Subject)
		}
		p.boards[e.Subject] = &Board{
			Name:      e.Subject,
			Query:     str(e.Payload["query"]),
			GroupBy:   str(e.Payload["group_by"]),
			CreatedBy: e.Actor.ID,
			CreatedAt: e.At,
		}

	case "board.deleted":
		if _, ok := p.boards[e.Subject]; !ok {
			return fmt.Errorf("event %s deletes unknown board %q", e.ID, e.Subject)
		}
		delete(p.boards, e.Subject)

	case "proposal.created":
		if _, exists := p.proposals[e.Subject]; exists {
			return fmt.Errorf("event %s creates proposal %q, which already exists", e.ID, e.Subject)
		}
		p.proposals[e.Subject] = &Proposal{
			ID:         e.Subject,
			Subject:    str(e.Payload["issue"]),
			Operation:  str(e.Payload["operation"]),
			Evidence:   str(e.Payload["evidence"]),
			ProposedBy: e.Actor.ID,
			Model:      e.Actor.Model,
			Role:       str(e.Payload["role"]),
			ProposedAt: e.At,
			Status:     ProposalOpen,
			From:       str(e.Payload["from"]),
			To:         str(e.Payload["to"]),
		}

	case "proposal.approved", "proposal.rejected":
		proposal, ok := p.proposals[e.Subject]
		if !ok {
			return fmt.Errorf("event %s decides unknown proposal %q", e.ID, e.Subject)
		}
		proposal.Status = ProposalApproved
		if e.Type == "proposal.rejected" {
			proposal.Status = ProposalRejected
		}
		proposal.DecidedBy = e.Actor.ID
		proposal.DecidedAt = e.At
		proposal.Reason = str(e.Payload["reason"])

	case "actor.registered":
		if _, exists := p.actors[e.Subject]; exists {
			return fmt.Errorf("event %s registers %q, which already exists", e.ID, e.Subject)
		}
		p.actors[e.Subject] = &Actor{
			ID:    e.Subject,
			Kind:  event.ActorKind(str(e.Payload["kind"])),
			Model: str(e.Payload["model"]),
		}

	case "actor.role_granted":
		actor, err := p.requireActor(e)
		if err != nil {
			return err
		}
		actor.Roles = addOnce(actor.Roles, str(e.Payload["role"]))

	case "actor.role_revoked":
		actor, err := p.requireActor(e)
		if err != nil {
			return err
		}
		actor.Roles = remove(actor.Roles, str(e.Payload["role"]))

	case "team.member_added":
		actor, err := p.requireActor(e)
		if err != nil {
			return err
		}
		actor.Teams = addOnce(actor.Teams, str(e.Payload["team"]))

	case "team.member_removed":
		// Removal is an added fact, not an erased one: the join stays in the log so
		// events made while a member remain explicable.
		actor, err := p.requireActor(e)
		if err != nil {
			return err
		}
		actor.Teams = remove(actor.Teams, str(e.Payload["team"]))

	case "field.multi_set":
		issue, err := p.require(e)
		if err != nil {
			return err
		}
		field := str(e.Payload["field"])
		if field == "" {
			return fmt.Errorf("event %s: field.multi_set with no field name", e.ID)
		}
		issue.Multi[field] = toStrings(e.Payload["values"])
		p.touch(issue, e)

	case "checklist.item_added":
		issue, err := p.require(e)
		if err != nil {
			return err
		}
		field := str(e.Payload["field"])
		issue.Checklists[field] = append(issue.Checklists[field],
			ChecklistItem{Text: str(e.Payload["text"])})
		p.touch(issue, e)

	case "checklist.item_checked", "checklist.item_unchecked":
		issue, err := p.require(e)
		if err != nil {
			return err
		}
		field, text := str(e.Payload["field"]), str(e.Payload["text"])
		items := issue.Checklists[field]
		found := false
		for i := range items {
			if items[i].Text != text {
				continue
			}
			found = true
			items[i].Checked = e.Type == "checklist.item_checked"
			if items[i].Checked {
				items[i].CheckedBy, items[i].CheckedAt = e.Actor.ID, e.At
			} else {
				items[i].CheckedBy, items[i].CheckedAt = "", time.Time{}
			}
		}
		if !found {
			return fmt.Errorf("event %s: no checklist item %q in %q on %s",
				e.ID, text, field, e.Subject)
		}
		p.touch(issue, e)

	case "checklist.item_removed":
		issue, err := p.require(e)
		if err != nil {
			return err
		}
		field, text := str(e.Payload["field"]), str(e.Payload["text"])
		kept := issue.Checklists[field][:0]
		for _, item := range issue.Checklists[field] {
			if item.Text != text {
				kept = append(kept, item)
			}
		}
		issue.Checklists[field] = kept
		p.touch(issue, e)

	case "issue.dependency_added":
		issue, err := p.require(e)
		if err != nil {
			return err
		}
		on := str(e.Payload["on"])
		if on == "" {
			return fmt.Errorf("event %s: dependency with no target", e.ID)
		}
		issue.DependsOn = addOnce(issue.DependsOn, on)
		p.touch(issue, e)

	case "issue.dependency_removed":
		issue, err := p.require(e)
		if err != nil {
			return err
		}
		issue.DependsOn = remove(issue.DependsOn, str(e.Payload["on"]))
		p.touch(issue, e)

	case "issue.commit_linked":
		issue, err := p.require(e)
		if err != nil {
			return err
		}
		sha := str(e.Payload["sha"])
		if sha == "" {
			return fmt.Errorf("event %s: commit link with no sha", e.ID)
		}
		// The event's own At is the commit's author time, not when the link was
		// made: a commit linked six weeks late still happened when it happened.
		// Kept sorted by author time as they arrive. `git log` hands commits over
		// newest first and a sweep may link any range in any order, so append order
		// says nothing useful; author order is the one a reader can follow.
		c := Commit{
			SHA:        sha,
			Message:    str(e.Payload["message"]),
			Repository: str(e.Payload["repository"]),
			Branch:     str(e.Payload["branch"]),
			Author:     str(e.Payload["author"]),
			At:         e.At,
			LinkedBy:   e.Actor.ID,
		}
		i := sort.Search(len(issue.Commits), func(i int) bool {
			return issue.Commits[i].At.After(c.At)
		})
		issue.Commits = append(issue.Commits, Commit{})
		copy(issue.Commits[i+1:], issue.Commits[i:])
		issue.Commits[i] = c
		p.touch(issue, e)

	case "issue.deleted":
		// Deletion is a tombstone. The events stay in the log — history is not
		// rewritten — but the issue leaves the projected present.
		if _, ok := p.issues[e.Subject]; !ok {
			return fmt.Errorf("event %s deletes unknown issue %q", e.ID, e.Subject)
		}
		delete(p.issues, e.Subject)
		p.sortedIDs = nil

	default:
		return fmt.Errorf("event %s at seq %d: unknown type %q — "+
			"the projection must be taught this type before it can replay this log",
			e.ID, e.Seq, e.Type)
	}
	return nil
}

// Proposal returns one proposal.
func (p *Projection) Proposal(id string) (*Proposal, bool) {
	proposal, ok := p.proposals[id]
	return proposal, ok
}

// Proposals returns proposals in id order, optionally filtered by status.
func (p *Projection) Proposals(status ProposalStatus) []*Proposal {
	ids := make([]string, 0, len(p.proposals))
	for id := range p.proposals {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]*Proposal, 0, len(ids))
	for _, id := range ids {
		if status == "" || p.proposals[id].Status == status {
			out = append(out, p.proposals[id])
		}
	}
	return out
}

// Board returns one saved board.
func (p *Projection) Board(name string) (*Board, bool) {
	board, ok := p.boards[name]
	return board, ok
}

// Boards returns every saved board, in name order.
func (p *Projection) Boards() []*Board {
	names := make([]string, 0, len(p.boards))
	for name := range p.boards {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]*Board, 0, len(names))
	for _, name := range names {
		out = append(out, p.boards[name])
	}
	return out
}

// ChecklistProgress returns how many items in a checklist are met, and how many
// there are. A field that is not a checklist reports zero of zero.
func (i *Issue) ChecklistProgress(field string) (done, total int) {
	for _, item := range i.Checklists[field] {
		total++
		if item.Checked {
			done++
		}
	}
	return done, total
}

// ChecklistComplete reports whether every item is met. An empty checklist counts as
// complete: there is nothing outstanding, and refusing on "no criteria yet" would
// make the gate impossible to pass rather than merely strict.
func (i *Issue) ChecklistComplete(field string) bool {
	done, total := i.ChecklistProgress(field)
	return done == total
}

func toStrings(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, str(item))
	}
	sort.Strings(out)
	return out
}

func sortedMultiKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedChecklistKeys(m map[string][]ChecklistItem) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Actor returns the projected identity of one actor.
func (p *Projection) Actor(id string) (*Actor, bool) {
	actor, ok := p.actors[id]
	return actor, ok
}

// ActorIDs returns every registered actor id, sorted.
func (p *Projection) ActorIDs() []string {
	out := make([]string, 0, len(p.actors))
	for id := range p.actors {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (p *Projection) requireActor(e *event.Event) (*Actor, error) {
	actor, ok := p.actors[e.Subject]
	if !ok {
		return nil, fmt.Errorf("event %s (%s) refers to unregistered actor %q",
			e.ID, e.Type, e.Subject)
	}
	return actor, nil
}

// addOnce appends a value if absent, keeping the slice sorted so the digest is stable.
func addOnce(values []string, v string) []string {
	if v == "" {
		return values
	}
	for _, existing := range values {
		if existing == v {
			return values
		}
	}
	values = append(values, v)
	sort.Strings(values)
	return values
}

func remove(values []string, v string) []string {
	out := values[:0]
	for _, existing := range values {
		if existing != v {
			out = append(out, existing)
		}
	}
	return out
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
