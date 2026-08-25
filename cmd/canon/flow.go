package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/ofenton/canon/internal/ingest"
	"github.com/ofenton/canon/internal/metrics"
)

// flowCmd reports how long work actually took, for one ingested repository.
//
// Measured from transitions derived from the ledger's own commit history. There is
// nothing to configure and nothing to estimate: the numbers come from status changes
// that were committed anyway.
func flowCmd(args []string) error {
	fs := flag.NewFlagSet("flow", flag.ContinueOnError)
	days := fs.Int("days", 30, "window in days")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path := "."
	if fs.NArg() > 0 {
		path = fs.Arg(0)
	}

	r, err := ingest.Repo(path, time.Now)
	if err != nil {
		return err
	}
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -*days)
	f := metrics.Ingested(r, from, to, 24*time.Hour)

	fmt.Printf("%s — last %d days\n\n", r.Name, *days)
	fmt.Printf("  %-14s %d\n", "completed", f.Completed)
	fmt.Printf("  %-14s %d\n", "started", f.Started)
	fmt.Printf("  %-14s %d\n\n", "in progress", f.InProgress)
	fmt.Printf("  cycle time     %s\n", spreadOf(f.CycleTime))
	fmt.Printf("  lead time      %s\n", spreadOf(f.LeadTime))
	if len(f.Ageing) > 0 {
		fmt.Printf("\n  ageing\n")
		for _, a := range f.Ageing {
			fmt.Printf("    %-12s %-12s %s\n", a.ID, a.State, duration(a.Days))
		}
	}
	fmt.Printf("\n  Derived from %s. Canon has no estimates.\n", ingest.LedgerPath)
	return nil
}

func spreadOf(s metrics.Summary) string {
	if s.Count == 0 {
		return "nothing completed in the window"
	}
	return fmt.Sprintf("p50 %s · p85 %s · p95 %s  (n=%d)",
		duration(s.P50), duration(s.P85), duration(s.P95), s.Count)
}

// duration renders days the way somebody reads them. A team shipping in an afternoon
// should not be told its cycle time is 0d.
func duration(d float64) string {
	if d >= 1 {
		return fmt.Sprintf("%.1fd", d)
	}
	if hours := d * 24; hours >= 1 {
		return fmt.Sprintf("%.1fh", hours)
	}
	if mins := int(d * 24 * 60); mins > 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return "under a minute"
}
