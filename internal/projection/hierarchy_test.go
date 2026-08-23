package projection

import (
	"strings"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/event"
)

// deepTree builds root → a → b → c, plus a sibling of b.
func deepTree(t *testing.T, log *event.Store) *Projection {
	t.Helper()
	link := func(child, parent string, min int) *event.Event {
		return event.New("issue.reparented", child, at(min), human("ollie"),
			map[string]any{"parent": parent})
	}
	events := []*event.Event{
		event.New("issue.created", "root", at(0), human("ollie"), map[string]any{"title": "root", "state": "todo"}),
		event.New("issue.created", "a", at(1), human("ollie"), map[string]any{"title": "a", "state": "todo"}),
		event.New("issue.created", "b", at(2), human("ollie"), map[string]any{"title": "b", "state": "todo"}),
		event.New("issue.created", "c", at(3), human("ollie"), map[string]any{"title": "c", "state": "todo"}),
		event.New("issue.created", "b2", at(4), human("ollie"), map[string]any{"title": "b2", "state": "todo"}),
		event.New("issue.created", "loner", at(5), human("ollie"), map[string]any{"title": "loner", "state": "todo"}),
		link("a", "root", 6), link("b", "a", 7), link("c", "b", 8), link("b2", "a", 9),
	}
	for _, e := range events {
		if err := log.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	p := New(log)
	if err := p.Rebuild(); err != nil {
		t.Fatal(err)
	}
	return p
}

// AC: WHEN an operator requests an issue's ancestors THE SYSTEM SHALL return them
// from the issue to its root, in order.
func TestAncestors(t *testing.T) {
	p := deepTree(t, newLog(t))
	cases := map[string]string{
		"c":     "b,a,root",
		"b":     "a,root",
		"a":     "root",
		"root":  "",
		"loner": "",
	}
	for id, want := range cases {
		got := strings.Join(p.Ancestors(id), ",")
		if got != want {
			t.Errorf("Ancestors(%s) = %q want %q", id, got, want)
		}
	}
	if got := p.Depth("c"); got != 3 {
		t.Errorf("Depth(c) = %d want 3", got)
	}
}

// AC: WHEN an operator requests an issue's subtree THE SYSTEM SHALL return its
// descendants to a requested depth.
func TestDescendantsToDepth(t *testing.T) {
	p := deepTree(t, newLog(t))
	// Depth-first: a child follows its parent, so a caller can indent by depth.
	cases := []struct {
		id    string
		depth int
		want  string
	}{
		{"root", 0, "a,b,c,b2"},
		{"root", 1, "a"},
		{"root", 2, "a,b,b2"},
		{"root", 3, "a,b,c,b2"},
		{"a", 0, "b,c,b2"},
		{"c", 0, ""},
		{"loner", 0, ""},
		{"absent", 0, ""},
	}
	for _, c := range cases {
		got := strings.Join(p.Descendants(c.id, c.depth), ",")
		if got != c.want {
			t.Errorf("Descendants(%s, %d) = %q want %q", c.id, c.depth, got, c.want)
		}
	}
}

// Deleting a mid-tree node lifts its children, and the subtree must reflect that
// immediately rather than reporting a shape that no longer exists.
func TestSubtreeFollowsADelete(t *testing.T) {
	log := newLog(t)
	p := deepTree(t, log)
	if got := strings.Join(p.Descendants("a", 1), ","); got != "b,b2" {
		t.Fatalf("before: %q", got)
	}

	// Delete b: c should lift to a.
	if err := log.Append(event.New("issue.reparented", "c", at(10), human("ollie"),
		map[string]any{"parent": "a", "because": "parent b was deleted"})); err != nil {
		t.Fatal(err)
	}
	if err := log.Append(event.New("issue.deleted", "b", at(10), human("ollie"), nil)); err != nil {
		t.Fatal(err)
	}
	if err := p.Catchup(); err != nil {
		t.Fatal(err)
	}
	// Both are now siblings at depth 1, so id order applies: b2 before c.
	if got := strings.Join(p.Descendants("a", 1), ","); got != "b2,c" {
		t.Errorf("after delete: %q want b2,c", got)
	}
	if got := strings.Join(p.Ancestors("c"), ","); got != "a,root" {
		t.Errorf("ancestors of c after delete: %q want a,root", got)
	}
}

// A log written by an older build could hold a parent cycle. A read path must not
// spin on it — an infinite loop is worse than a wrong answer.
func TestCyclicParentsDoNotSpin(t *testing.T) {
	log := newLog(t)
	for _, id := range []string{"x", "y"} {
		if err := log.Append(event.New("issue.created", id, at(0), human("ollie"),
			map[string]any{"title": id, "state": "todo"})); err != nil {
			t.Fatal(err)
		}
	}
	// Written directly to the log, bypassing the enforcer that would refuse this.
	for _, pair := range [][2]string{{"x", "y"}, {"y", "x"}} {
		if err := log.Append(event.New("issue.reparented", pair[0], at(1), human("ollie"),
			map[string]any{"parent": pair[1]})); err != nil {
			t.Fatal(err)
		}
	}
	p := New(log)
	if err := p.Rebuild(); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = p.Ancestors("x")
		_ = p.Descendants("x", 0)
	}()
	select {
	case <-done:
	case <-timeout():
		t.Fatal("a cyclic parent chain made a read path spin")
	}
}

func timeout() <-chan time.Time { return time.After(2 * time.Second) }

// A subtree must come back in an order a caller can indent directly: each child
// immediately after its parent. Sorted-by-id separates children from parents, which
// is the one thing a tree renderer cannot work around.
func TestSubtreeIsDepthFirst(t *testing.T) {
	p := deepTree(t, newLog(t))
	got := p.Descendants("root", 0)

	position := map[string]int{}
	for i, id := range got {
		position[id] = i
	}
	for _, child := range got {
		issue, _ := p.Issue(child)
		if issue.Parent == "root" {
			continue
		}
		if position[issue.Parent] > position[child] {
			t.Errorf("%s appears before its parent %s: %v", child, issue.Parent, got)
		}
	}
	if strings.Join(got, ",") != "a,b,c,b2" {
		t.Errorf("got %v, want depth-first a,b,c,b2", got)
	}
}
