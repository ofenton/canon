package ingest

import (
	"strings"
	"testing"
)

func withDeps(id, status string, deps ...string) Increment {
	return Increment{ID: id, Title: id, Status: status, Type: "feature", DependsOn: deps}
}

// AC: WHERE a ledger declares dependencies THE SYSTEM SHALL show what is blocked and
// by what.
func TestBlockedReportsUnfinishedDependencies(t *testing.T) {
	r := &Repository{Increments: []Increment{
		withDeps("feat-001", "done"),
		withDeps("feat-002", "in-progress"),
		withDeps("feat-003", "approved", "feat-001", "feat-002"),
	}}

	blocked := r.Blocked()
	if got := blocked["feat-003"]; len(got) != 1 || got[0] != "feat-002" {
		t.Fatalf("feat-003 waits on %v; feat-001 is done and should not block", got)
	}
	if _, ok := blocked["feat-002"]; ok {
		t.Fatal("feat-002 depends on nothing and cannot be blocked")
	}
}

// Finished work is not blocked, whatever it once waited on. Reporting it would put
// completed increments on a list of things somebody has to unblock.
func TestFinishedWorkIsNotBlocked(t *testing.T) {
	r := &Repository{Increments: []Increment{
		withDeps("feat-001", "in-progress"),
		withDeps("feat-002", "done", "feat-001"),
		withDeps("feat-003", "abandoned", "feat-001"),
	}}
	if blocked := r.Blocked(); len(blocked) != 0 {
		t.Fatalf("finished work reported as blocked: %v", blocked)
	}
}

// A dependency on something not in this ledger is a conformance problem, not a block:
// nothing can unblock it, so reporting it as waiting sends somebody looking for work
// that does not exist.
func TestADanglingDependencyIsNotABlock(t *testing.T) {
	r := &Repository{Increments: []Increment{
		withDeps("feat-001", "approved", "feat-999"),
	}}
	if blocked := r.Blocked(); len(blocked) != 0 {
		t.Fatalf("a dependency on a non-existent increment was reported as a block: %v", blocked)
	}
}

func TestCyclesAreFound(t *testing.T) {
	r := &Repository{Increments: []Increment{
		withDeps("feat-001", "approved", "feat-002"),
		withDeps("feat-002", "approved", "feat-003"),
		withDeps("feat-003", "approved", "feat-001"),
		withDeps("feat-004", "approved"),
	}}

	cycles := r.Cycles()
	if len(cycles) != 1 {
		t.Fatalf("got %d cycles, want 1: %v", len(cycles), cycles)
	}
	if len(cycles[0]) != 3 {
		t.Fatalf("cycle = %v, want three members", cycles[0])
	}
	if cycles[0][0] != "feat-001" {
		t.Fatalf("cycle starts at %s; it should start at its lowest-sorting member so the "+
			"same cycle always reads the same way", cycles[0][0])
	}
	if strings.Contains(strings.Join(cycles[0], " "), "feat-004") {
		t.Fatal("feat-004 is not in the cycle")
	}
}

// The same cycle found from different entry points must be reported once.
func TestACycleIsReportedOnce(t *testing.T) {
	r := &Repository{Increments: []Increment{
		withDeps("feat-001", "approved", "feat-002"),
		withDeps("feat-002", "approved", "feat-001"),
	}}
	if cycles := r.Cycles(); len(cycles) != 1 {
		t.Fatalf("got %d cycles for one mutual pair: %v", len(cycles), cycles)
	}
}

// An acyclic ledger reports nothing, or the check means nothing.
func TestNoCyclesInAnOrdinaryLedger(t *testing.T) {
	r := &Repository{Increments: []Increment{
		withDeps("feat-001", "done"),
		withDeps("feat-002", "approved", "feat-001"),
		withDeps("feat-003", "approved", "feat-001", "feat-002"),
	}}
	if cycles := r.Cycles(); len(cycles) != 0 {
		t.Fatalf("a ledger with no cycle reported %v", cycles)
	}
}

// An increment depending on itself is the smallest cycle and the easiest to miss.
func TestSelfDependencyIsACycle(t *testing.T) {
	r := &Repository{Increments: []Increment{withDeps("feat-001", "approved", "feat-001")}}
	if cycles := r.Cycles(); len(cycles) != 1 {
		t.Fatalf("a self-dependency should be a cycle, got %v", cycles)
	}
}

// This repository's own ledger has dependencies and must have no cycles — the
// template refuses them locally, so finding one here would mean the two
// implementations disagree.
func TestThisLedgerHasNoCycles(t *testing.T) {
	r, err := Repo("../..", now)
	if err != nil {
		t.Skipf("not runnable outside a checkout: %v", err)
	}
	if cycles := r.Cycles(); len(cycles) != 0 {
		t.Fatalf("this ledger has a cycle that validate-plan.py did not refuse: %v", cycles)
	}
	blocked := r.Blocked()
	t.Logf("%d increments declare a dependency that has not finished", len(blocked))
}
