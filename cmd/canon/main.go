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
)

// version is set at build time via -ldflags.
var version = "dev"

const usage = `canon — point it at your repositories and see what every team is building

Canon derives everything from repositories that follow the agentic SDLC template.
It authors nothing: a repository owns its own work, and this reads it.

usage:
  canon version                 print the build version
  canon catalogue <root>        list every product found under a directory
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
  -products string  directory to discover products under (default ".")
  -refresh duration how often to re-read the repositories (default 5m, 0 disables)

mcp flags:
  -products string  directory to discover products under (default ".")

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
func load(root string) (*api.Server, []string, error) {
	srv := api.New(catalogue.New(), time.Now)
	sources, err := catalogue.Discover(root)
	if err != nil {
		return nil, nil, err
	}
	srv.Catalogue().Refresh(sources, time.Now)
	return srv, sources, nil
}

// serve runs the read surface over HTTP.
func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "address to listen on")
	root := fs.String("products", ".", "directory to discover products under")
	every := fs.Duration("refresh", 5*time.Minute, "how often to re-read the repositories")
	if err := fs.Parse(args); err != nil {
		return err
	}

	srv, sources, err := load(*root)
	if err != nil {
		return err
	}

	fmt.Printf("canon %s listening on %s\n", version, *addr)
	fmt.Printf("  products %d discovered under %s\n", len(sources), *root)
	if len(sources) == 0 {
		fmt.Printf("           nothing found — a product is a repository with %s\n",
			"specs/increment-plan.md")
	}

	// Re-reading on a timer is what stops a long-running instance quietly showing
	// yesterday. Every response carries refreshed_at regardless, so a stale view can
	// be recognised as stale rather than trusted as current.
	if *every > 0 {
		fmt.Printf("  refresh  every %s\n", *every)
		go func() {
			for range time.Tick(*every) {
				if found, err := catalogue.Discover(*root); err == nil {
					srv.Catalogue().Refresh(found, time.Now)
				}
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
	root := fs.String("products", ".", "directory to discover products under")
	if err := fs.Parse(args); err != nil {
		return err
	}

	srv, _, err := load(*root)
	if err != nil {
		return err
	}
	return mcp.NewServer(srv.APIHandler(), srv.Routes(), "").Serve(os.Stdin, os.Stdout)
}
