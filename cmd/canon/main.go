// Command canon is the Canon issue tracker.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/api"
	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/mcp"
	"github.com/ofenton/canon/internal/metrics"
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
  canon mcp [flags]             serve MCP over stdio, for agents
  canon rebuild [flags]         discard projections and replay the log
  canon backup -out <file>      write a consistent copy of the data, safe while running
  canon link [flags]            link commits to the issues they name

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

mcp flags:
  -actor string     actor to act as (required)
  -db string        path to the event log (default "canon.db")
  -schema string    path to canon.yaml (default "canon.yaml")

rebuild flags:
  -db string        path to the event log (default "canon.db")

backup flags:
  -out string       destination file (required, never overwritten)
  -db string        path to the event log (default "canon.db")

link flags:
  -actor string     who is recording the link (required)
  -range string     commit range to sweep, e.g. main..HEAD
  -commit string    a single commit to link (default HEAD)
  -issue string     issue to link to (default: read from each commit message)
  -repo string      path to the git repository (default ".")
  -dry-run          print what would be linked and write nothing
  -db string        path to the event log (default "canon.db")
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
	case "schema":
		return schemaCmd(args[1:])
	case "events":
		return events(args[1:])
	case "bootstrap":
		return bootstrap(args[1:])
	case "serve":
		return serve(args[1:])
	case "mcp":
		return mcpCmd(args[1:])
	case "rebuild":
		return rebuild(args[1:])
	case "backup":
		return backup(args[1:])
	case "link":
		return linkCmd(args[1:])
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
	if err := metrics.CheckNoEstimateFields(s); err != nil {
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
	roleName := fs.String("role", "", "role to grant (default: the most permissive role in canon.yaml)")
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

	// The role to grant is not hardcoded. This command previously assumed "admin",
	// which meant it failed on any schema that named its roles differently — found
	// the first time Canon was pointed at its own schema, where the equivalent role
	// is called "maintainer".
	role := *roleName
	switch {
	case sch.Unrestricted():
		role = ""
	case role != "":
		if _, ok := sch.Role(role); !ok {
			return fmt.Errorf("canon.yaml defines no %q role; defined roles are %s",
				role, strings.Join(sch.RoleNames(), ", "))
		}
	default:
		var err error
		if role, err = mostPermissiveRole(sch); err != nil {
			return err
		}
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

// mostPermissiveRole picks the role a first administrator should hold: the one that
// is granted outright everything any role is granted. If no single role dominates,
// the choice is the operator's to make rather than this command's to guess.
func mostPermissiveRole(sch *schema.Schema) (string, error) {
	names := sch.RoleNames()
	if len(names) == 0 {
		return "", fmt.Errorf("canon.yaml defines no roles")
	}

	// Compare what roles can actually do, not how their grants are spelled.
	// "transition:*" and "transition:approved->in_progress" are different strings
	// and the first subsumes the second, so the concrete operation space is what
	// matters — and the schema already knows how to decide against it.
	var operations []string
	operations = append(operations, schema.Verbs...)
	for _, field := range sch.FieldNames() {
		operations = append(operations, schema.FieldOp(field))
	}
	for _, t := range sch.Transitions {
		operations = append(operations, schema.TransitionOp(t.From, t.To))
	}

	var candidates []string
	for _, name := range names {
		role, _ := sch.Role(name)
		if role.Scope != schema.ScopeOrg {
			continue
		}
		complete := true
		for _, op := range operations {
			if role.Decide(op) != schema.Allow {
				complete = false
				break
			}
		}
		if complete {
			candidates = append(candidates, name)
		}
	}

	switch len(candidates) {
	case 1:
		return candidates[0], nil
	case 0:
		return "", fmt.Errorf("no single role in canon.yaml grants everything; choose one with -role (defined roles: %s)",
			strings.Join(names, ", "))
	default:
		sort.Strings(candidates)
		return candidates[0], nil
	}
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
	if err := metrics.CheckNoEstimateFields(sch); err != nil {
		return err
	}
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

// mcpCmd serves MCP over stdio.
//
// It dispatches through the same HTTP handler the network serves, so an agent and a
// human take an identical path through authorisation. -actor is required and not
// defaulted: an agent silently acting as whoever happens to be first in the registry
// would be worse than a clear error.
func mcpCmd(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	actor := fs.String("actor", "", "actor to act as")
	dbPath := fs.String("db", "canon.db", "path to the event log")
	schemaPath := fs.String("schema", "canon.yaml", "path to canon.yaml")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *actor == "" {
		return fmt.Errorf("-actor is required: every event records who caused it")
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
	if _, err := e.Principal(*actor); err != nil {
		return err
	}

	apiSrv := api.New(sch, store, e, time.Now)
	server := mcp.NewServer(apiSrv.Handler(), apiSrv.Routes(), *actor)
	return server.Serve(os.Stdin, os.Stdout)
}

// backup writes a consistent single-file copy, safely while the server runs.
//
// Copying canon.db by hand does not work: WAL mode keeps recent commits in a sidecar
// that has not been folded in yet, and on a young database that is most of the data.
// This is measured in internal/event: a plain copy of a 500-event log recovered zero.
func backup(args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	out := fs.String("out", "", "destination file")
	dbPath := fs.String("db", "canon.db", "path to the event log")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("-out is required")
	}

	store, err := event.Open(*dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	before, err := store.Count()
	if err != nil {
		return err
	}
	start := time.Now()
	if err := store.Backup(*out); err != nil {
		return err
	}
	info, err := os.Stat(*out)
	if err != nil {
		return err
	}

	fmt.Printf("wrote %s (%d events, %.1f KiB) in %s\n",
		*out, before, float64(info.Size())/1024, time.Since(start).Round(time.Millisecond))
	fmt.Printf("restore with: canon serve -db %s\n", *out)
	return nil
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
