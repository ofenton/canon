// Command canon is the Canon issue tracker.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/api"
	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/projection"
	"github.com/ofenton/canon/internal/schema"
)

// version is set at build time via -ldflags.
var version = "dev"

const usage = `canon — an issue tracker whose schema is versioned config

usage:
  canon version                 print the build version
  canon schema [flags]          validate canon.yaml and print a summary
  canon events [flags]          print the event log as JSON
  canon bootstrap [flags]       create the first admin on an empty log
  canon serve [flags]           run the HTTP API
  canon rebuild [flags]         discard projections and replay the log

schema flags:
  -schema string    path to canon.yaml (default "canon.yaml")

events flags:
  -db string        path to the event log (default "canon.db")
  -subject string   only events about this subject
  -since int        only events after this sequence number

bootstrap flags:
  -actor string     id of the first admin (required)
  -team string      team to add them to
  -db string        path to the event log (default "canon.db")
  -schema string    path to canon.yaml (default "canon.yaml")

serve flags:
  -db string        path to the event log (default "canon.db")
  -schema string    path to canon.yaml (default "canon.yaml")
  -addr string      address to listen on (default ":8080")

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
	case "schema":
		return schemaCmd(args[1:])
	case "events":
		return events(args[1:])
	case "bootstrap":
		return bootstrap(args[1:])
	case "serve":
		return serve(args[1:])
	case "rebuild":
		return rebuild(args[1:])
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

// bootstrap creates the first administrator.
//
// Registering an actor requires an actor, so an empty log cannot admit anyone over
// HTTP. The obvious fix — let the first HTTP caller register themselves — would put
// a privileged unauthenticated path on the network, which is a bad trade even for a
// self-hosted tool that does not yet authenticate.
//
// This is a local command instead. It needs filesystem access to the log, which is a
// real authorisation boundary and one the operator already crossed to install Canon.
func bootstrap(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	actorID := fs.String("actor", "", "id of the first admin")
	team := fs.String("team", "", "team to add them to")
	dbPath := fs.String("db", "canon.db", "path to the event log")
	schemaPath := fs.String("schema", "canon.yaml", "path to canon.yaml")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *actorID == "" {
		return fmt.Errorf("-actor is required")
	}

	sch, err := schema.Load(*schemaPath)
	if err != nil {
		return err
	}
	store, err := event.Open(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	e := enforce.New(sch, store)
	existing, err := e.Actors()
	if err != nil {
		return err
	}
	// Only ever on an empty registry: after that, admins are made by admins.
	if len(existing) > 0 {
		return fmt.Errorf("this log already has %d actor(s) (%s); bootstrap only runs on an empty registry, use the API to add more",
			len(existing), strings.Join(existing, ", "))
	}

	role := "admin"
	if sch.Unrestricted() {
		role = ""
	} else if _, ok := sch.Role(role); !ok {
		return fmt.Errorf("canon.yaml defines no %q role; defined roles are %s",
			role, strings.Join(sch.RoleNames(), ", "))
	}

	by := event.Actor{ID: "bootstrap", Kind: event.ActorSystem}
	now := time.Now().UTC()
	if err := e.RegisterActor(*actorID, event.ActorHuman, "", now, by); err != nil {
		return err
	}
	if role != "" {
		if err := e.GrantRole(*actorID, role, now, by); err != nil {
			return err
		}
	}
	if *team != "" {
		if err := e.AddToTeam(*actorID, *team, now, by); err != nil {
			return err
		}
	}

	fmt.Printf("registered %s", *actorID)
	if role != "" {
		fmt.Printf(" as %s", role)
	}
	if *team != "" {
		fmt.Printf(" in team %s", *team)
	}
	fmt.Printf("\n\nstart the server and act as them:\n  canon serve -db %s -schema %s\n  curl -H 'X-Canon-Actor: %s' http://localhost:8080/api/issues\n",
		*dbPath, *schemaPath, *actorID)
	return nil
}

// serve runs the HTTP API.
//
// One process, one file, no external services — the self-host story depends on
// there being nothing else to install (ADR-0004).
func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	dbPath := fs.String("db", "canon.db", "path to the event log")
	schemaPath := fs.String("schema", "canon.yaml", "path to canon.yaml")
	addr := fs.String("addr", ":8080", "address to listen on")
	if err := fs.Parse(args); err != nil {
		return err
	}

	sch, err := schema.Load(*schemaPath)
	if err != nil {
		return err
	}
	store, err := event.Open(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	// Refuse to start against a log the schema cannot describe, rather than
	// discovering it one failed write at a time.
	if err := enforce.CheckMigration(store, sch); err != nil {
		return fmt.Errorf("schema does not fit the existing log: %w", err)
	}

	srv := api.New(sch, store, enforce.New(sch, store), time.Now)
	fmt.Printf("canon %s listening on %s\n  schema %s (%d states, %d fields, %d roles)\n  log    %s\n",
		version, *addr, *schemaPath,
		len(sch.States), len(sch.Fields), len(sch.RoleNames()), *dbPath)

	server := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return server.ListenAndServe()
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
