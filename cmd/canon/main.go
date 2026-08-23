// Command canon is the Canon issue tracker.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"strings"

	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

// version is set at build time via -ldflags.
var version = "dev"

const usage = `canon — an issue tracker whose schema is versioned config

usage:
  canon version                 print the build version
  canon events [flags]          print the event log as JSON
  canon schema [flags]          validate canon.yaml and print a summary

events flags:
  -db string        path to the event log (default "canon.db")
  -subject string   only events about this subject
  -since int        only events after this sequence number

schema flags:
  -schema string    path to canon.yaml (default "canon.yaml")
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
	case "schema":
		return schemaCmd(args[1:])
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

// schemaCmd validates canon.yaml and summarises it.
//
// The summary exists so the aggregate is visible. Jira instances reach 800 fields
// because nobody ever sees the total; printing it is the cheapest possible defence.
func schemaCmd(args []string) error {
	fs := flag.NewFlagSet("schema", flag.ContinueOnError)
	path := fs.String("schema", "canon.yaml", "path to canon.yaml")
	if err := fs.Parse(args); err != nil {
		return err
	}

	s, err := schema.Load(*path)
	if err != nil {
		return err
	}

	fmt.Printf("%s is valid\n\n", *path)
	fmt.Printf("  %d states, %d transitions, %d fields, %d issue types\n\n",
		len(s.States), len(s.Transitions), len(s.Fields), len(s.IssueTypes))
	for _, st := range s.States {
		evidence := ""
		if st.RequiresEvidence {
			evidence = "  (requires evidence)"
		}
		fmt.Printf("  %-14s %-8s -> %s%s\n", st.Name, st.Category,
			strings.Join(s.PermittedFrom(st.Name), ", "), evidence)
	}
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
