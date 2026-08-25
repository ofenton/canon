package metrics

import (
	"time"

	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/ingest"
	"github.com/ofenton/canon/internal/projection"
	"github.com/ofenton/canon/internal/schema"
)

// Flow measured from an ingested repository.
//
// The measurement code is unchanged and stays unchanged: percentiles, ageing and
// throughput were right, and what was wrong was their input. `import-ledger.py`
// spread each increment's route across the commits carrying its trailer and reported
// a p50 of nine minutes against a real four hours — roughly thirty times out. This
// feeds the same code from transitions derived exactly from the ledger's own history.

// TemplateStates is the status vocabulary the agentic SDLC template fixes, and its
// grouping into the three categories flow is measured against.
//
// Hardcoded, deliberately. Under [ADR-0009] the template *is* the schema: there is no
// per-organisation configuration to read, so a status that is not here is not a status
// this system understands. That is a more opinionated position than Canon started
// with and it is the one the reframe chose.
var TemplateStates = []schema.State{
	{Name: "planned", Category: schema.Open},
	{Name: "approved", Category: schema.Open},
	{Name: "in-progress", Category: schema.Active},
	{Name: "in-review", Category: schema.Active},
	{Name: "done", Category: schema.Closed},
	{Name: "abandoned", Category: schema.Closed},
	// An increment removed from the ledger is deliberately *not* closed. It did not
	// finish; it stopped existing. Counting it as completed would flatter every
	// repository that reverted a plan.
	{Name: ingest.Removed, Category: schema.Open},
}

// TemplateSchema returns a schema describing only what the template fixes.
//
// It exists so the measurement code, which takes a schema, can be fed without a
// configuration file. When `cut-001` removes the configurable schema this is what
// remains.
func TemplateSchema() *schema.Schema {
	return schema.NewFixed(TemplateStates)
}

// FromIngest converts ingested increments into the shape the measurement code reads.
//
// A conversion rather than a rewrite: `Compute` is 400 tested lines whose logic was
// never the problem.
func FromIngest(r *ingest.Repository) []*projection.Issue {
	out := make([]*projection.Issue, 0, len(r.Increments))
	for _, inc := range r.Increments {
		issue := &projection.Issue{
			ID:     inc.ID,
			Title:  inc.Title,
			State:  inc.Status,
			Type:   inc.Type,
			Team:   r.Name,
			Fields: inc.Fields,
		}
		for _, t := range inc.Transitions {
			at, err := time.Parse(time.RFC3339, t.At)
			if err != nil {
				continue
			}
			// CreatedAt is the first thing that happened to it, which is what lead
			// time measures from. The ledger has no separate creation time — the
			// first transition is the creation.
			if issue.CreatedAt.IsZero() {
				issue.CreatedAt = at
			}
			issue.UpdatedAt = at
			issue.Transitions = append(issue.Transitions, projection.Transition{
				From: t.From, To: t.To, At: at,
				Actor: event.Actor{ID: t.Commit, Kind: event.ActorSystem},
			})
		}
		out = append(out, issue)
	}
	return out
}

// Ingested measures flow for one repository over a window.
func Ingested(r *ingest.Repository, from, to time.Time, bucket time.Duration) Flow {
	return Compute(FromIngest(r), TemplateSchema(), from, to, bucket)
}
