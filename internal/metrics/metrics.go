// Package metrics derives flow measurements from recorded state transitions.
//
// Everything here is measured, never estimated. Canon has no story point field and
// will not get one: estimates get inflated under pressure to make velocity rise,
// they are inconsistent between people, and they measure the guess rather than the
// work. Cycle time and throughput come from timestamps that were recorded anyway.
//
// The cost is that these numbers only exist after work has flowed. That is the
// honest trade — a forecast you cannot make yet beats a number you made up.
package metrics

import (
	"fmt"
	"sort"
	"time"

	"github.com/ofenton/canon/internal/ingest"
)

// Flow is the measured behaviour of a set of issues.
type Flow struct {
	// Completed counts issues that reached a closed state in the window.
	Completed int `json:"completed"`
	// Started counts issues that first entered an active state in the window.
	Started int `json:"started"`
	// InProgress counts issues currently in an active state.
	InProgress int `json:"in_progress"`
	// CycleTime is active → closed, for issues completed in the window.
	CycleTime Summary `json:"cycle_time"`
	// LeadTime is created → closed, which is what a requester actually waits.
	LeadTime Summary `json:"lead_time"`
	// Ageing is how long the oldest unfinished work has been in progress. Watching
	// this beats watching cycle time, because it moves before the damage lands.
	Ageing []Ageing `json:"ageing"`
	// Throughput is completions per bucket, oldest first.
	Throughput []Bucket `json:"throughput"`
	// Window describes what was measured.
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// Summary describes a distribution. There is no mean: cycle times are long-tailed,
// and an average hides the tail that people actually complain about.
type Summary struct {
	Count  int     `json:"count"`
	P50    float64 `json:"p50_days"`
	P85    float64 `json:"p85_days"`
	P95    float64 `json:"p95_days"`
	Max    float64 `json:"max_days"`
	Sample []Item  `json:"slowest,omitempty"`
}

// Item is one measured issue.
type Item struct {
	ID   string  `json:"id"`
	Days float64 `json:"days"`
}

// Ageing is unfinished work and how long it has been active.
type Ageing struct {
	ID    string  `json:"id"`
	State string  `json:"state"`
	Days  float64 `json:"days"`
}

// Bucket is one period of throughput.
type Bucket struct {
	Start     time.Time `json:"start"`
	Completed int       `json:"completed"`
}

// Compute measures flow over issues, between from and to.
func Compute(increments []ingest.Increment, from, to time.Time, bucket time.Duration) Flow {
	flow := Flow{From: from, To: to}

	var cycles, leads []Item
	for _, issue := range increments {
		startedAt, closedAt := milestones(issue)

		if closedAt.IsZero() {
			if isActive(issue.Status) {
				flow.InProgress++
				if !startedAt.IsZero() {
					flow.Ageing = append(flow.Ageing, Ageing{
						ID: issue.ID, State: issue.Status, Days: days(to.Sub(startedAt)),
					})
				}
			}
		}
		if !startedAt.IsZero() && within(startedAt, from, to) {
			flow.Started++
		}
		if closedAt.IsZero() || !within(closedAt, from, to) {
			continue
		}

		flow.Completed++
		if !startedAt.IsZero() {
			cycles = append(cycles, Item{ID: issue.ID, Days: days(closedAt.Sub(startedAt))})
		}
		if first := firstSeen(issue); !first.IsZero() {
			leads = append(leads, Item{ID: issue.ID, Days: days(closedAt.Sub(first))})
		}
	}

	flow.CycleTime = summarise(cycles)
	flow.LeadTime = summarise(leads)
	sort.Slice(flow.Ageing, func(i, j int) bool { return flow.Ageing[i].Days > flow.Ageing[j].Days })
	flow.Throughput = throughput(increments, from, to, bucket)
	return flow
}

// milestones returns when an issue first became active and first became closed.
//
// "First" matters: work that is reopened and closed again should not report a cycle
// time measured from the second attempt, because the calendar time a requester waited
// includes the first.
func milestones(inc ingest.Increment) (started, closed time.Time) {
	for _, t := range inc.Transitions {
		at, err := time.Parse(time.RFC3339, t.At)
		if err != nil {
			continue
		}
		switch Category(t.To) {
		case "active":
			if started.IsZero() {
				started = at
			}
		case "closed":
			if closed.IsZero() {
				closed = at
			}
		}
	}
	// An issue closed without ever being active has no cycle time, only a lead time.
	return started, closed
}

func throughput(increments []ingest.Increment, from, to time.Time, bucket time.Duration) []Bucket {
	if bucket <= 0 || !to.After(from) {
		return nil
	}
	counts := map[time.Time]int{}
	for start := from.Truncate(bucket); start.Before(to); start = start.Add(bucket) {
		counts[start] = 0
	}
	for _, issue := range increments {
		_, closed := milestones(issue)
		if closed.IsZero() || !within(closed, from, to) {
			continue
		}
		counts[closed.Truncate(bucket)]++
	}

	out := make([]Bucket, 0, len(counts))
	for start, n := range counts {
		out = append(out, Bucket{Start: start, Completed: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out
}

func summarise(items []Item) Summary {
	if len(items) == 0 {
		return Summary{}
	}
	sorted := append([]Item(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Days < sorted[j].Days })

	sample := make([]Item, 0, 3)
	for i := len(sorted) - 1; i >= 0 && len(sample) < 3; i-- {
		sample = append(sample, sorted[i])
	}
	return Summary{
		Count:  len(sorted),
		P50:    percentile(sorted, 0.50),
		P85:    percentile(sorted, 0.85),
		P95:    percentile(sorted, 0.95),
		Max:    sorted[len(sorted)-1].Days,
		Sample: sample,
	}
}

// percentile uses nearest-rank, which needs no interpolation and gives an answer
// that is always a real measurement rather than a number between two of them.
func percentile(sorted []Item, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(p*float64(len(sorted)) + 0.5)
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1].Days
}

// Category groups the template's fixed statuses. There is no configurable schema to
// read any more: under ADR-0009 the template is the schema, so a status not listed
// here is not one this system understands.
//
// (removed) is deliberately not closed. An increment removed from the ledger did not
// finish; it stopped existing, and counting it as completed would flatter every
// repository that reverted a plan.
func Category(status string) string {
	switch status {
	case "planned", "approved", ingest.Removed:
		return "open"
	case "in-progress", "in-review":
		return "active"
	case "done", "abandoned":
		return "closed"
	}
	return ""
}

func isActive(status string) bool { return Category(status) == "active" }

func within(t, from, to time.Time) bool {
	return !t.Before(from) && !t.After(to)
}

// days converts a duration to days, keeping enough precision for work that took
// hours rather than days.
//
// Two decimal places meant anything under about fifteen minutes rounded to zero, and
// a two-day project reported p50, p85 and p95 all as 0d — which reads as a broken
// metric rather than as fast work. Four places resolve to about nine seconds, which
// is finer than any transition anyone records deliberately.
//
// The unit stays days because it is in the API's field names and a client reading
// p50_days should not have to know which release changed what it meant. Turning the
// number into something a human reads is the caller's job.
func days(d time.Duration) float64 {
	return float64(int(d.Hours()/24*10000+0.5)) / 10000
}

// EstimateFieldNames are field names Canon refuses to load.
//
// Rule 10 of the constitution says no estimation, and a rule enforced only by
// convention lasts until the first person who wants a story point field. Refusing
// them by name makes the position structural: the way to reintroduce estimation is
// to change this list in a pull request, which is the conversation worth having.
var EstimateFieldNames = []string{
	"storypoints", "story_points", "points", "estimate", "estimated",
	"velocity", "burndown", "effort", "tshirt", "t_shirt_size",
}

// CheckNoEstimateFields reports an error if an increment carries an estimate-shaped
// field.
//
// The rule is unchanged by the reframe: no estimation, refused by name. What changed
// is where the field could come from — an organisation's schema before, a repository's
// ledger now.
func CheckNoEstimateFields(fields map[string]string) error {
	for name := range fields {
		normalised := normalise(name)
		for _, banned := range EstimateFieldNames {
			if normalised == banned {
				return fmt.Errorf("field %q is an estimate; Canon measures flow from recorded transitions and has no estimation. Remove it, or use cycle time and throughput instead", name)
			}
		}
	}
	return nil
}

func normalise(name string) string {
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			out = append(out, r+32)
		case r == '-' || r == ' ':
			out = append(out, '_')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

// firstSeen is when an increment first appeared in the ledger. Lead time measures
// from there: a ledger records no separate creation, so the first transition is it.
func firstSeen(inc ingest.Increment) time.Time {
	for _, t := range inc.Transitions {
		if at, err := time.Parse(time.RFC3339, t.At); err == nil {
			return at
		}
	}
	return time.Time{}
}
