// Command canon is the Canon issue tracker.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"time"

	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/projection"
)

// version is set at build time via -ldflags.
var version = "dev"

const usage = `canon — an issue tracker whose schema is versioned config

usage:
  canon version                 print the build version
  canon events [flags]          print the event log as JSON
  canon rebuild [flags]         discard projections and replay the log

events flags:
  -db string        path to the event log (default "canon.db")
  -subject string   only events about this subject
  -since int        only events after this sequence number

rebuild flags:
  -db string        path to the event log (default "canon.db")
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "canon:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch args[0] {
	case "version":
		fmt.Println(version)
		return nil
	case "events":
		return events(args[1:])
	case "rebuild":
		return rebuild(args[1:])
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

// rebuild discards the materialised state and replays the log from the beginning.
//
// The projection is a cache with no authority, so this is always safe: it is the
// recovery path for a projection bug, and the proof that the log is the source of truth.
func rebuild(args []string) error {
	fs := flag.NewFlagSet("rebuild", flag.ContinueOnError)
	path := fs.String("db", "canon.db", "path to the event log")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := event.Open(*path)
	if err != nil {
		return err
	}
	defer store.Close()

	total, err := store.Count()
	if err != nil {
		return err
	}

	start := time.Now()
	p := projection.New(store)
	if err := p.Rebuild(); err != nil {
		return fmt.Errorf("rebuild failed: %w", err)
	}
	fmt.Printf("replayed %d events in %s\ndigest %s\n",
		total, time.Since(start).Round(time.Millisecond), p.Snapshot()[:16])
	return nil
}

// events renders the log in the human-readable form. Canonical CBOR is what is
// stored, because a signature needs byte-stable encoding; this is how a person reads it.
func events(args []string) error {
	fs := flag.NewFlagSet("events", flag.ContinueOnError)
	path := fs.String("db", "canon.db", "path to the event log")
	subject := fs.String("subject", "", "only events about this subject")
	since := fs.Int64("since", 0, "only events after this sequence number")
	if err := fs.Parse(args); err != nil {
		return err
	}

	store, err := event.Open(*path)
	if err != nil {
		return err
	}
	defer store.Close()

	var found []*event.Event
	switch {
	case *subject != "":
		found, err = store.Subject(*subject)
	case *since > 0:
		found, err = store.Since(*since)
	default:
		found, err = store.All()
	}
	if err != nil {
		return err
	}

	// One JSON object per line: greppable, diffable, and streamable for large logs.
	enc := json.NewEncoder(os.Stdout)
	for _, e := range found {
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("encoding event %s: %w", e.ID, err)
		}
	}
	return nil
}
