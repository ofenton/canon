package ingest

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"time"
)

// Repository is everything Canon knows about one product, all of it derived.
type Repository struct {
	// Path is where it was read from; Remote is its origin URL if it has one.
	Path   string `json:"path"`
	Remote string `json:"remote,omitempty"`
	// Head is the commit this reflects. Two ingests of the same head are identical.
	Head string `json:"head"`

	Name         string        `json:"name"`
	Purpose      string        `json:"purpose,omitempty"`
	Requirements []Requirement `json:"requirements,omitempty"`
	Increments   []Increment   `json:"increments"`

	// IngestedAt is when this was read, not when anything happened. It exists so a
	// stale view reads as stale rather than as current (R58).
	IngestedAt time.Time `json:"ingested_at"`
}

// Repo reads a repository and derives its state.
//
// Deterministic for a given commit: the same head produces the same result, which is
// what makes re-ingesting safe and makes a fingerprint meaningful.
func Repo(path string, now func() time.Time) (*Repository, error) {
	ledger, ok, err := currentFile(path, LedgerPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("%s has no %s, so it does not follow the template", path, LedgerPath)
	}

	r := &Repository{
		Path:       path,
		Remote:     remote(path),
		Head:       head(path),
		Increments: parseLedger(ledger),
		IngestedAt: now().UTC(),
	}

	// The spec is optional. A repository with a ledger and no spec is a repository
	// mid-adoption, and reporting on it is more useful than refusing it.
	if spec, ok, err := currentFile(path, SpecPath); err == nil && ok {
		r.Name, r.Purpose, r.Requirements = parseSpec(spec)
	}
	if r.Name == "" {
		r.Name = path
	}

	history, err := Transitions(path)
	if err != nil {
		return nil, err
	}
	for i := range r.Increments {
		r.Increments[i].Transitions = history[r.Increments[i].ID]
	}
	return r, nil
}

// Removed marks an increment that left the ledger. Parenthesised so it cannot
// collide with a status a schema might define.
const Removed = "(removed)"

// Transitions derives every increment's status history from the ledger's own commit
// history.
//
// This is exact, not approximate, and that distinction cost something to learn. The
// template requires that every status change is a commit, so the ledger's history *is*
// the transition log. The mechanism this replaces spread each increment's route across
// the commits carrying its trailer and was measured at roughly thirty times out: a p50
// of nine minutes against a real four hours.
//
// The one honest limit: two status changes in one commit share a timestamp. That is
// exact about what git recorded, which is the most any reader can claim.
func Transitions(repo string) (map[string][]Transition, error) {
	commits, err := touching(repo, LedgerPath)
	if err != nil {
		return nil, err
	}

	out := map[string][]Transition{}
	// lastKnown always holds the status most recently reported for an id, including
	// Removed. One map rather than comparing consecutive parses: the first version
	// mutated the freshly parsed state to carry removals forward, which produced a
	// duplicate (removed) -> (removed) and then lost the id entirely, so a
	// reappearance read as a creation. Found by ingesting this repository.
	lastKnown := map[string]string{}

	emit := func(id, from, to string, c commit) {
		out[id] = append(out[id], Transition{
			From: from, To: to,
			At: c.At.Format(time.RFC3339), Commit: c.SHA,
		})
		lastKnown[id] = to
	}

	for _, c := range commits {
		text, ok, err := fileAt(repo, c.SHA, LedgerPath)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		current := statuses(text)

		// An increment can leave the ledger — a revert, or a plan withdrawn. That is
		// a real event with a real commit, and recording it keeps the history
		// continuous: without it, an increment that vanished and returned reads as
		// having been created twice.
		for id, was := range lastKnown {
			if was == Removed {
				continue
			}
			if _, still := current[id]; !still {
				emit(id, was, Removed, c)
			}
		}
		for id, status := range current {
			if was, seen := lastKnown[id]; !seen || was != status {
				emit(id, was, status, c)
			}
		}
	}
	return out, nil
}

// currentFile reads a file at HEAD.
func currentFile(repo, path string) (string, bool, error) {
	h := head(repo)
	if h == "" {
		return "", false, fmt.Errorf("%s has no commits", repo)
	}
	return fileAt(repo, h, path)
}

// Fingerprint is a stable summary of what was ingested, for asserting that two
// ingests of the same commit agree.
func (r *Repository) Fingerprint() string {
	ids := make([]string, 0, len(r.Increments))
	byID := map[string]Increment{}
	for _, inc := range r.Increments {
		ids = append(ids, inc.ID)
		byID[inc.ID] = inc
	}
	sort.Strings(ids)

	var b []byte
	for _, id := range ids {
		inc := byID[id]
		b = append(b, fmt.Sprintf("%s\x1f%s\x1f%s\x1f%d\x1e",
			inc.ID, inc.Status, inc.Type, len(inc.Transitions))...)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
