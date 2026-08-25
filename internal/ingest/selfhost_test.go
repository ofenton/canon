package ingest

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// This repository follows the template, so it is the best fixture available: real
// markdown written by hand over 54 increments, with a real commit history.
func TestIngestThisRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	r, err := Repo("../..", time.Now)
	if err != nil {
		t.Skipf("not runnable outside a checkout: %v", err)
	}

	if len(r.Increments) < 40 {
		t.Fatalf("got %d increments from this repository's ledger, expected the full plan", len(r.Increments))
	}
	if r.Name == "" || r.Purpose == "" {
		t.Fatalf("spec gave name %q purpose %q", r.Name, r.Purpose)
	}
	if len(r.Requirements) < 50 {
		t.Fatalf("got %d requirements, expected the spec's full set", len(r.Requirements))
	}

	// Every increment past 'planned' must have a history, or the derivation is
	// silently returning nothing for real work.
	var withHistory int
	for _, inc := range r.Increments {
		if len(inc.Transitions) > 0 {
			withHistory++
		}
		for i := 1; i < len(inc.Transitions); i++ {
			if inc.Transitions[i].From != inc.Transitions[i-1].To {
				t.Errorf("%s: transition %d starts at %q but the previous ended at %q",
					inc.ID, i, inc.Transitions[i].From, inc.Transitions[i-1].To)
			}
		}
	}
	if withHistory < 40 {
		t.Fatalf("only %d of %d increments carry a history", withHistory, len(r.Increments))
	}

	// Cross-check one increment against git itself, so this is not the parser
	// agreeing with the parser.
	out, err := exec.Command("git", "-C", "../..", "log", "--format=%H",
		"--reverse", "--", LedgerPath).Output()
	if err != nil {
		t.Fatal(err)
	}
	commits := strings.Fields(string(out))
	for _, inc := range r.Increments {
		for _, tr := range inc.Transitions {
			if !contains(commits, tr.Commit) {
				t.Fatalf("%s cites commit %s, which never touched %s", inc.ID, tr.Commit, LedgerPath)
			}
		}
	}
	t.Logf("ingested %q: %d increments, %d with history, %d requirements, head %s",
		r.Name, len(r.Increments), withHistory, len(r.Requirements), r.Head[:7])
}

func contains(all []string, want string) bool {
	for _, a := range all {
		if a == want {
			return true
		}
	}
	return false
}
