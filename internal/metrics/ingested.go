package metrics

import (
	"time"

	"github.com/ofenton/canon/internal/ingest"
)

// Ingested measures flow for one repository over a window.
//
// The measurement code was never wrong; its input was. `scripts/import-ledger.py`
// spread each increment's route across the commits carrying its trailer and reported
// a p50 of nine minutes against a real four hours. This reads transitions derived
// exactly from the ledger's own history.
func Ingested(r *ingest.Repository, from, to time.Time, bucket time.Duration) Flow {
	return Compute(r.Increments, from, to, bucket)
}

// Across measures flow over several repositories at once, which is the question only
// an aggregator can answer.
func Across(repos []*ingest.Repository, from, to time.Time, bucket time.Duration) Flow {
	var all []ingest.Increment
	for _, r := range repos {
		if r != nil {
			all = append(all, r.Increments...)
		}
	}
	return Compute(all, from, to, bucket)
}
