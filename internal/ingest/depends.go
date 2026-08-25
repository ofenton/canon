package ingest

import "sort"

// Dependencies between increments.
//
// One relation, one direction: A waits on B. The template's ledger says so with a
// `Dependencies:` field, and everything here is derived from it — Canon does not
// record dependencies, it reads them.
//
// Cycles are found and reported rather than refused. Refusing is the repository's own
// job, in `validate-plan.py`; by the time Canon sees a ledger the cycle is already
// committed, and a reader that hides it is less useful than one that names it.

// finished reports whether a status means the work is over.
func finished(status string) bool { return status == "done" || status == "abandoned" }

// Blocked returns, for each increment, the increments it waits on that are not
// finished.
//
// Only direct dependencies. Transitive blocking — A waits on B which waits on C — is
// deliberately not computed: it produces long chains that are technically true and
// rarely act on, and the direct answer is the one somebody can do something about.
func (r *Repository) Blocked() map[string][]string {
	status := make(map[string]string, len(r.Increments))
	for _, inc := range r.Increments {
		status[inc.ID] = inc.Status
	}

	out := map[string][]string{}
	for _, inc := range r.Increments {
		if finished(inc.Status) {
			continue
		}
		var waiting []string
		for _, dep := range inc.DependsOn {
			// A dependency on something not in this ledger is a conformance problem,
			// not a block: nothing can unblock it, so reporting it as waiting would
			// send somebody looking for work that does not exist.
			if s, known := status[dep]; known && !finished(s) {
				waiting = append(waiting, dep)
			}
		}
		if len(waiting) > 0 {
			sort.Strings(waiting)
			out[inc.ID] = waiting
		}
	}
	return out
}

// Cycles returns every dependency cycle in the ledger, each starting at its
// lowest-sorting member so the same cycle is always reported the same way.
func (r *Repository) Cycles() [][]string {
	edges := make(map[string][]string, len(r.Increments))
	for _, inc := range r.Increments {
		edges[inc.ID] = inc.DependsOn
	}

	var cycles [][]string
	seen := map[string]bool{}
	var path []string
	onPath := map[string]bool{}

	var walk func(string)
	walk = func(id string) {
		if onPath[id] {
			// Found one: take the path from where this id first appears.
			for i, p := range path {
				if p == id {
					cycles = append(cycles, normalise(path[i:]))
					return
				}
			}
			return
		}
		if seen[id] {
			return
		}
		seen[id] = true
		onPath[id] = true
		path = append(path, id)
		for _, next := range edges[id] {
			if _, known := edges[next]; known {
				walk(next)
			}
		}
		path = path[:len(path)-1]
		onPath[id] = false
	}

	ids := make([]string, 0, len(edges))
	for id := range edges {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		walk(id)
	}
	return dedupe(cycles)
}

// normalise rotates a cycle to start at its lowest-sorting member, so the same cycle
// discovered from different entry points reads identically.
func normalise(cycle []string) []string {
	if len(cycle) == 0 {
		return nil
	}
	lowest := 0
	for i, id := range cycle {
		if id < cycle[lowest] {
			lowest = i
		}
	}
	out := make([]string, 0, len(cycle))
	out = append(out, cycle[lowest:]...)
	out = append(out, cycle[:lowest]...)
	return out
}

func dedupe(cycles [][]string) [][]string {
	seen := map[string]bool{}
	var out [][]string
	for _, c := range cycles {
		key := ""
		for _, id := range c {
			key += id + "\x1f"
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}
