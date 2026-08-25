// Package api is Canon's read surface.
//
// Every route is a read. Canon derives what it shows from repositories that follow
// the template, so there is nothing here to write to: an aggregator that accepted
// writes would be a second source of truth for facts a repository already owns, which
// is the thing ADR-0009 exists to prevent.
//
// That absence is asserted rather than intended — see TestNoWriteRoutes.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/catalogue"
	"github.com/ofenton/canon/internal/conform"
	"github.com/ofenton/canon/internal/ingest"
	"github.com/ofenton/canon/internal/metrics"
	"github.com/ofenton/canon/internal/ui"
)

// defaultLimit bounds a list. Ten thousand increments in one response is slow for the
// server and useless to the caller.
const defaultLimit = 200

// Server serves the catalogue.
type Server struct {
	products *catalogue.Catalogue
	now      func() time.Time
}

// New builds a server over a catalogue.
func New(products *catalogue.Catalogue, now func() time.Time) *Server {
	if now == nil {
		now = time.Now
	}
	if products == nil {
		products = catalogue.New()
	}
	return &Server{products: products, now: now}
}

// Catalogue returns the catalogue, so a caller can refresh it.
func (s *Server) Catalogue() *catalogue.Catalogue { return s.products }

// Routes is the complete API surface, in one place.
//
// Kept as data rather than scattered across registration calls so the parity test can
// enumerate it, and so a reader can see the whole interface at once.
func (s *Server) Routes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"GET /api/products":        s.listProducts,
		"GET /api/products/{name}": s.getProduct,
		"GET /api/increments":      s.listIncrements,
		"GET /api/metrics":         s.metrics,
		"GET /api/conformance":     s.conformance,
		"GET /api/schema":          s.schema,
	}
}

// Handler builds the router: the API, plus the web UI at the root.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for pattern, handler := range s.Routes() {
		mux.HandleFunc(pattern, handler)
	}
	mux.Handle("/", ui.Handler())
	return mux
}

// APIHandler builds a router with no UI, for callers that want only the API.
func (s *Server) APIHandler() http.Handler {
	mux := http.NewServeMux()
	for pattern, handler := range s.Routes() {
		mux.HandleFunc(pattern, handler)
	}
	return mux
}

// listProducts returns the catalogue: every product Canon knows about.
//
// Answered from what was ingested. Nothing here reads git, so the response is as
// current as the last refresh and says so.
func (s *Server) listProducts(w http.ResponseWriter, r *http.Request) {
	entries := s.products.Entries()
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, summarise(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"products":     out,
		"refreshed_at": s.products.RefreshedAt(),
	})
}

// getProduct returns one product with its increments and conformance report.
func (s *Server) getProduct(w http.ResponseWriter, r *http.Request) {
	e, ok := s.products.Entry(r.PathValue("name"))
	if !ok {
		writeError(w, http.StatusNotFound,
			fmt.Errorf("no product called %q; GET /api/products lists them", r.PathValue("name")))
		return
	}
	body := map[string]any{
		"repository":   e.Repository,
		"conformance":  e.Report,
		"source":       e.Source,
		"refreshed_at": e.RefreshedAt,
	}
	if e.Err != "" {
		body["error"] = e.Err
	}
	if e.Repository != nil {
		body["blocked"] = e.Repository.Blocked()
		if cycles := e.Repository.Cycles(); len(cycles) > 0 {
			body["cycles"] = cycles
		}
	}
	writeJSON(w, http.StatusOK, body)
}

// listIncrements returns work across every product, which is the question no single
// repository can answer.
func (s *Server) listIncrements(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	product := r.URL.Query().Get("product")
	// blocked=true narrows to work that cannot start, which is the question worth
	// asking across products: everything else is a list somebody has to read.
	blockedOnly := r.URL.Query().Get("blocked") == "true"
	limit, offset := page(r)

	type row struct {
		Product string `json:"product"`
		ingest.Increment
		// BlockedBy is what this is waiting on that has not finished. Derived from
		// the ledger's own Dependencies field, not recorded here.
		BlockedBy []string `json:"blocked_by,omitempty"`
	}
	all := []row{}
	for _, e := range s.products.Entries() {
		if e.Repository == nil {
			continue
		}
		if product != "" && e.Name() != product {
			continue
		}
		blocked := e.Repository.Blocked()
		for _, inc := range e.Repository.Increments {
			if status != "" && inc.Status != status {
				continue
			}
			if blockedOnly && len(blocked[inc.ID]) == 0 {
				continue
			}
			all = append(all, row{Product: e.Name(), Increment: inc, BlockedBy: blocked[inc.ID]})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Product != all[j].Product {
			return all[i].Product < all[j].Product
		}
		return all[i].ID < all[j].ID
	})

	total := len(all)
	if offset > total {
		offset = total
	}
	end := min(offset+limit, total)
	writeJSON(w, http.StatusOK, map[string]any{
		"increments":   all[offset:end],
		"total":        total,
		"limit":        limit,
		"offset":       offset,
		"refreshed_at": s.products.RefreshedAt(),
	})
}

// metrics reports flow, measured from transitions derived from commit history.
//
// There is nothing to configure and nothing to estimate: the numbers come from status
// changes that were committed anyway.
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest,
				fmt.Errorf("days must be a positive number, got %q", raw))
			return
		}
		days = n
	}
	to := s.now().UTC()
	from := to.AddDate(0, 0, -days)

	bucket := 24 * time.Hour
	if days > 90 {
		bucket = 7 * 24 * time.Hour
	}

	var repos []*ingest.Repository
	wanted := r.URL.Query().Get("product")
	for _, e := range s.products.Entries() {
		if e.Repository == nil || (wanted != "" && e.Name() != wanted) {
			continue
		}
		repos = append(repos, e.Repository)
	}
	writeJSON(w, http.StatusOK, metrics.Across(repos, from, to, bucket))
}

// conformance reports how faithfully each product follows the template.
//
// Reported, never enforced: an aggregator cannot decline a commit that has already
// happened. Refusing is the repository's own job, in its hook and its CI.
func (s *Server) conformance(w http.ResponseWriter, r *http.Request) {
	entries := s.products.Entries()
	out := make([]map[string]any, 0, len(entries))
	var errors, warnings int
	for _, e := range entries {
		counts := e.Report.Counts()
		errors += counts[conform.Error]
		warnings += counts[conform.Warning]
		out = append(out, map[string]any{
			"product":  e.Name(),
			"conforms": e.Err == "" && e.Report.Conforms(),
			"error":    e.Err,
			"findings": e.Report.Findings,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"products": out,
		"errors":   errors,
		"warnings": warnings,
	})
}

// schema returns what the template fixes.
//
// Not configuration — a statement of the convention. There is no per-organisation
// schema any more, which is a more opinionated position than Canon started with and
// the one ADR-0009 chose: a schema with no configuration cannot drift.
func (s *Server) schema(w http.ResponseWriter, r *http.Request) {
	names := []string{"planned", "approved", "in-progress", "in-review", "done", "abandoned", ingest.Removed}
	states := make([]map[string]string, 0, len(names))
	for _, name := range names {
		states = append(states, map[string]string{"name": name, "category": metrics.Category(name)})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"states": states,
		"types":  []string{"feature", "fix", "security", "perf", "refactor", "chore", "docs"},
		"ledger": ingest.LedgerPath,
		"spec":   ingest.SpecPath,
		"note":   "fixed by the agentic SDLC template, not configurable",
	})
}

// summarise reduces one product to the row a list shows.
func summarise(e *catalogue.Entry) map[string]any {
	row := map[string]any{
		"name":         e.Name(),
		"source":       e.Source,
		"refreshed_at": e.RefreshedAt,
	}
	if e.Err != "" {
		row["error"] = e.Err
		return row
	}
	var open, done int
	for _, inc := range e.Repository.Increments {
		if metrics.Category(inc.Status) == "closed" {
			done++
		} else {
			open++
		}
	}
	row["purpose"] = e.Repository.Purpose
	row["head"] = e.Repository.Head
	row["remote"] = e.Repository.Remote
	row["open"] = open
	row["done"] = done
	row["conforms"] = e.Report.Conforms()
	row["findings"] = len(e.Report.Findings)
	return row
}

// page reads limit and offset, bounded so one request cannot ask for everything.
func page(r *http.Request) (limit, offset int) {
	limit, offset = defaultLimit, 0
	if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = min(n, 1000)
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && n > 0 {
		offset = n
	}
	return limit, offset
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		// The status is already written; there is nowhere to report this but the
		// response, which is now malformed. Truncating loudly beats silence.
		fmt.Fprintf(w, "\n{\"error\":%q}\n", "encoding the response failed: "+err.Error())
	}
}

// writeError says what went wrong in the same shape as everything else, because a
// caller parsing two response shapes is being made to do work the server should do.
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": strings.TrimSpace(err.Error())})
}
