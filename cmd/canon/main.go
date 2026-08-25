// Command canon reads repositories that follow the agentic SDLC template and shows
// what every team is building.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ofenton/canon/internal/api"
	"github.com/ofenton/canon/internal/catalogue"
	"github.com/ofenton/canon/internal/mcp"
	"github.com/ofenton/canon/internal/source"
)

// version is set at build time via -ldflags.
var version = "dev"

const usage = `canon — point it at your repositories and see what every team is building

Canon derives everything from repositories that follow the agentic SDLC template.
It authors nothing: a repository owns its own work, and this reads it.

usage:
  canon version                 print the build version
  canon catalogue               list every product Canon can find
  canon ingest <path>           read one repository and print what was derived
  canon flow <path>             report how long work actually took
  canon conform <path>          report how faithfully a repository follows the template
  canon serve [flags]           serve the catalogue over HTTP, with the web UI
  canon mcp [flags]             serve the same reads to agents over stdio

catalogue flags:
  -json             print the catalogue as JSON

ingest flags:
  -json             print the ingested repository as JSON

flow flags:
  -days int         window in days (default 30)

conform flags:
  -json             print the report as JSON
  -strict           exit non-zero on warnings as well as errors

serve flags:
  -addr string      address to listen on (default ":8080")
  -refresh duration how often to re-read the repositories (default 5m, 0 disables)

where to look (catalogue, serve and mcp):
  -source string    a place to look; repeatable
  -sources string   a file listing places to look (default canon.sources if present)
  -cache string     where fetched repositories are kept (deleting it loses nothing)

A source is a place, not a repository to register: a local directory scanned one level
deep, a local repository, or — once built — a repository to fetch and an organisation to
expand. With no source given, Canon reads the working directory.

A product is any repository containing specs/increment-plan.md. There is nothing to
register and nothing to configure: adopting Canon is committing that file.
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
	case "catalogue", "catalog":
		return catalogueCmd(args[1:])
	case "ingest":
		return ingestCmd(args[1:])
	case "flow":
		return flowCmd(args[1:])
	case "conform":
		return conformCmd(args[1:])
	case "serve":
		return serve(args[1:])
	case "mcp":
		return mcpCmd(args[1:])
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

// load builds a server with its catalogue already filled.
func load(sources []source.Source) (*api.Server, []source.Result) {
	srv := api.New(catalogue.New(), time.Now)
	results := source.Resolve(sources, cacheDir)
	srv.Catalogue().RefreshFrom(results, time.Now)
	return srv, results
}

// serve runs the read surface over HTTP.
func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "address to listen on")
	every := fs.Duration("refresh", 5*time.Minute, "how often to re-read the repositories")
	sources := sourceFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	list, err := sources()
	if err != nil {
		return err
	}

	srv, results := load(list)

	fmt.Printf("canon %s listening on %s\n", version, *addr)
	report(results)
	if len(source.Paths(results)) == 0 {
		fmt.Printf("  nothing found — a product is a repository with %s\n", "specs/increment-plan.md")
	}

	// Re-reading on a timer is what stops a long-running instance quietly showing
	// yesterday. Every response carries refreshed_at regardless, so a stale view can
	// be recognised as stale rather than trusted as current.
	if *every > 0 {
		fmt.Printf("  refresh  every %s\n", *every)
		go func() {
			for range time.Tick(*every) {
				srv.Catalogue().RefreshFrom(source.Resolve(list, cacheDir), time.Now)
			}
		}()
	}
	fmt.Printf("  reads    open to anyone who can reach the port; Canon accepts no writes\n")

	server := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return server.ListenAndServe()
}

// mcpCmd serves the same read surface to agents over stdio.
//
// No actor is required any more. Every event used to record who caused it; nothing is
// caused here, so there is nobody to record.
func mcpCmd(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ContinueOnError)
	sources := sourceFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	list, err := sources()
	if err != nil {
		return err
	}

	srv, _ := load(list)
	return mcp.NewServer(srv.APIHandler(), srv.Routes(), "").Serve(os.Stdin, os.Stdout)
}
