// Package api is the single HTTP interface to Canon.
//
// The web UI, the CLI, agents and the MCP server all speak this API. There is no
// second path, no endpoint that exists only for the UI's convenience, and no
// operation the UI can do that an agent cannot. That parity is asserted by test:
// an agent-first tracker whose UI has private endpoints is not agent-first, it is
// a normal tracker with an API bolted on.
//
// v1 authorises but does not authenticate. The actor is taken from the X-Canon-Actor
// header and trusted; it must name a registered actor, whose roles come from the log.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ofenton/canon/internal/catalogue"
	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/metrics"
	"github.com/ofenton/canon/internal/projection"
	"github.com/ofenton/canon/internal/query"
	"github.com/ofenton/canon/internal/schema"
	"github.com/ofenton/canon/internal/ui"
)

// ActorHeader names the caller. v1 trusts it; see the package comment.
const ActorHeader = "X-Canon-Actor"

// Server exposes Canon over HTTP.
type Server struct {
	// products is the catalogue of ingested repositories. Reads answer from it and
	// never touch git, which is what R57 asks for: an aggregator that clones on
	// request is a proxy with worse latency.
	products *catalogue.Catalogue

	enforcer *enforce.Enforcer
	log      *event.Store
	schema   *schema.Schema
	now      func() time.Time

	// mu guards view. A single long-lived projection is caught up per request
	// rather than rebuilt: rebuilding is O(all events), catching up is O(new
	// events since the last read. At 30k events a rebuild costs ~40ms, which
	// meets the budget today and would breach it at 150k.
	mu   sync.Mutex
	view *projection.Projection
}

// New returns a Server. now is injectable so tests can assert on timestamps.
func New(s *schema.Schema, log *event.Store, e *enforce.Enforcer, now func() time.Time) *Server {
	if now == nil {
		now = time.Now
	}
	return &Server{enforcer: e, log: log, schema: s, now: now,
		view: projection.New(log), products: catalogue.New()}
}

// Catalogue returns the server's product catalogue, so a caller can refresh it.
func (s *Server) Catalogue() *catalogue.Catalogue { return s.products }

// Routes is the complete API surface, in one place.
//
// Kept as data rather than scattered across registration calls so the parity test
// can enumerate it, and so a reader can see the whole interface at once.
func (s *Server) Routes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"GET /api/products":                         s.listProducts,
		"GET /api/products/{name}":                  s.getProduct,
		"GET /api/schema":                           s.getSchema,
		"GET /api/schema/usage":                     s.schemaUsage,
		"GET /api/events":                           s.listEvents,
		"GET /api/issues":                           s.listIssues,
		"POST /api/issues":                          s.createIssue,
		"GET /api/issues/{id}":                      s.getIssue,
		"DELETE /api/issues/{id}":                   s.deleteIssue,
		"PATCH /api/issues/{id}/fields":             s.setFields,
		"PUT /api/issues/{id}/multi/{field}":        s.setMulti,
		"PUT /api/issues/{id}/checklist/{field}":    s.checklistItem,
		"DELETE /api/issues/{id}/checklist/{field}": s.removeChecklistItem,
		"POST /api/issues/{id}/transition":          s.transition,
		"PUT /api/issues/{id}/parent":               s.setParent,
		"GET /api/issues/{id}/children":             s.listChildren,
		"GET /api/issues/{id}/ancestors":            s.listAncestors,
		"GET /api/issues/{id}/tree":                 s.issueTree,
		"GET /api/issues/{id}/dependencies":         s.listDependencies,
		"PUT /api/issues/{id}/dependencies":         s.addDependency,
		"DELETE /api/issues/{id}/dependencies/{on}": s.removeDependency,
		"PUT /api/issues/{id}/commits":              s.linkCommit,
		"GET /api/issues/{id}/commits":              s.listCommits,
		"GET /api/cycles":                           s.listCycles,
		"GET /api/proposals":                        s.listProposals,
		"GET /api/proposals/{id}":                   s.getProposal,
		"POST /api/proposals/{id}/approve":          s.approveProposal,
		"POST /api/proposals/{id}/reject":           s.rejectProposal,
		"GET /api/metrics":                          s.metrics,
		"GET /api/boards":                           s.listBoards,
		"POST /api/boards":                          s.saveBoard,
		"GET /api/boards/{name}":                    s.renderBoard,
		"DELETE /api/boards/{name}":                 s.deleteBoard,
		"GET /api/actors":                           s.listActors,
		"POST /api/actors":                          s.registerActor,
		"GET /api/actors/{id}":                      s.getActor,
		"POST /api/actors/{id}/tokens":              s.issueToken,
		"DELETE /api/actors/{id}/tokens":            s.revokeTokens,
		"POST /api/actors/{id}/roles":               s.grantRole,
		"DELETE /api/actors/{id}/roles/{role}":      s.revokeRole,
		"POST /api/actors/{id}/teams":               s.addToTeam,
		"DELETE /api/actors/{id}/teams/{team}":      s.removeFromTeam,
	}
}

// Handler builds the router: the API, plus the web UI at the root.
//
// The UI is mounted here rather than in Routes() because Routes() is the contract
// agents get — the MCP tool list is derived from it, and a UI path in there would
// become a meaningless tool. Keeping them apart also lets the "every route is under
// /api" test stay strict instead of carving out an exception.
func (s *Server) Handler() http.Handler {
	api := http.NewServeMux()
	for pattern, handler := range s.Routes() {
		api.HandleFunc(pattern, handler)
	}

	mux := http.NewServeMux()
	// Every API route is behind authentication. The UI shell is not: it is a static
	// page that carries no data, and every call it makes comes back through here.
	mux.Handle("/api/", s.authenticate(api))
	mux.Handle("/", ui.Handler())
	return mux
}

// APIHandler builds a router with no UI, for callers that want only the API.
func (s *Server) APIHandler() http.Handler {
	mux := http.NewServeMux()
	for pattern, handler := range s.Routes() {
		mux.HandleFunc(pattern, handler)
	}
	return s.authenticate(mux)
}

// ---------------------------------------------------------------- reads

func (s *Server) getSchema(w http.ResponseWriter, r *http.Request) {
	type stateView struct {
		Name              string   `json:"name"`
		Category          string   `json:"category"`
		RequiresEvidence  bool     `json:"requires_evidence,omitempty"`
		RequiresChecklist []string `json:"requires_checklist,omitempty"`
		To                []string `json:"to"`
	}
	states := make([]stateView, 0, len(s.schema.States))
	for _, st := range s.schema.States {
		states = append(states, stateView{
			Name: st.Name, Category: string(st.Category),
			RequiresEvidence:  st.RequiresEvidence,
			RequiresChecklist: st.RequiresChecklist,
			To:                s.schema.PermittedFrom(st.Name),
		})
	}
	// The permitted parents and children per type, so a client can tell a user what
	// is allowed before they try it rather than after it is refused.
	parents := map[string][]string{}
	children := map[string][]string{}
	for _, it := range s.schema.IssueTypes {
		parents[it.Name] = s.schema.ParentTypesFor(it.Name)
		children[it.Name] = s.schema.ChildTypesFor(it.Name)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":      s.schema.Version,
		"states":       states,
		"fields":       s.schema.Fields,
		"issue_types":  s.schema.IssueTypes,
		"roles":        s.schema.RoleNames(),
		"teams":        s.schema.Teams,
		"hierarchy":    s.schema.Hierarchy.Levels,
		"parent_types": parents,
		"child_types":  children,
	})
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	since, err := intParam(r, "since")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var events []*event.Event
	if subject := r.URL.Query().Get("subject"); subject != "" {
		events, err = s.log.Subject(subject)
	} else {
		events, err = s.log.Since(since)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": redactSecrets(events)})
}

// redactSecrets removes stored credential material from an event stream.
//
// The hash is not the token and a 256-bit random secret is not recoverable from it,
// so this is defence in depth rather than a fix for a disclosure. It matters because
// the reasoning that makes the hash safe is a property of the token, not of the hash:
// anything that ever weakens token entropy would turn this route into the attack, and
// nobody would think to look here. Handing it out buys nothing either way.
func redactSecrets(events []*event.Event) []*event.Event {
	out := make([]*event.Event, 0, len(events))
	for _, e := range events {
		if _, sensitive := e.Payload["hash"]; !sensitive {
			out = append(out, e)
			continue
		}
		clone := *e
		clone.Payload = make(map[string]any, len(e.Payload))
		for k, v := range e.Payload {
			if k == "hash" {
				clone.Payload[k] = "[redacted]"
				continue
			}
			clone.Payload[k] = v
		}
		out = append(out, &clone)
	}
	return out
}

func (s *Server) listIssues(w http.ResponseWriter, r *http.Request) {
	view, err := s.currentView()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// ?q= is the query language; ?state= and ?team= remain as shorthands for the
	// two filters people reach for most, and compose with it.
	raw := r.URL.Query().Get("q")
	for _, shorthand := range []string{"state", "team"} {
		if v := r.URL.Query().Get(shorthand); v != "" {
			raw = strings.TrimSpace(raw + " " + shorthand + "=" + v)
		}
	}
	q, err := query.Parse(raw, s.schema)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Lists are bounded. Ten thousand issues in one response is slow to produce and
	// useless to read; total is returned so a caller knows what it is not seeing.
	limit, offset, err := page(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	issues, total := q.FilterPage(view, s.schema, limit, offset)
	writeJSON(w, http.StatusOK, map[string]any{
		"issues": issues,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (s *Server) getIssue(w http.ResponseWriter, r *http.Request) {
	view, err := s.currentView()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	issue, ok := view.Issue(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown issue %s", r.PathValue("id")))
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

func (s *Server) listChildren(w http.ResponseWriter, r *http.Request) {
	view, err := s.currentView()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"children": view.Children(r.PathValue("id"))})
}

// listProposals returns open proposals by default; ?status=all for the history.
func (s *Server) listProposals(w http.ResponseWriter, r *http.Request) {
	var (
		proposals []*enforce.Proposal
		err       error
	)
	if r.URL.Query().Get("status") == "all" {
		proposals, err = s.enforcer.AllProposals()
	} else {
		proposals, err = s.enforcer.Proposals()
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proposals": proposals})
}

func (s *Server) getProposal(w http.ResponseWriter, r *http.Request) {
	all, err := s.enforcer.AllProposals()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	for _, p := range all {
		if p.ID == r.PathValue("id") {
			writeJSON(w, http.StatusOK, p)
			return
		}
	}
	writeError(w, http.StatusNotFound, fmt.Errorf("unknown proposal %s", r.PathValue("id")))
}

func (s *Server) approveProposal(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	if err := s.enforcer.ApproveProposal(p, r.PathValue("id"), s.now()); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) rejectProposal(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength > 0 && !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	if err := s.enforcer.RejectProposal(p, r.PathValue("id"), body.Reason, s.now()); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listBoards returns the saved boards.
// metrics reports measured flow. There is nothing to configure and nothing to
// estimate: the numbers come from transitions that were recorded anyway.
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	view, err := s.currentView()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	q, err := query.Parse(r.URL.Query().Get("q"), s.schema)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	to := s.now().UTC()
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("days must be a positive number, got %q", raw))
			return
		}
		days = n
	}
	from := to.AddDate(0, 0, -days)

	bucket := 24 * time.Hour
	if days > 90 {
		bucket = 7 * 24 * time.Hour
	}
	writeJSON(w, http.StatusOK,
		metrics.Compute(q.Filter(view, s.schema), s.schema, from, to, bucket))
}

func (s *Server) listBoards(w http.ResponseWriter, r *http.Request) {
	boards, err := s.enforcer.Boards()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"boards": boards, "group_keys": query.GroupKeys(s.schema),
	})
}

func (s *Server) saveBoard(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name    string `json:"name"`
		Query   string `json:"query"`
		GroupBy string `json:"group_by"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	validate := func(raw string) error {
		_, err := query.Parse(raw, s.schema)
		return err
	}
	if body.GroupBy != "" && !slices.Contains(query.GroupKeys(s.schema), body.GroupBy) {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Errorf("cannot group by %q; valid keys are %s",
				body.GroupBy, strings.Join(query.GroupKeys(s.schema), ", ")))
		return
	}
	if err := s.enforcer.SaveBoard(p, body.Name, body.Query, body.GroupBy, validate, s.now()); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"name": body.Name})
}

// renderBoard resolves a saved board against the current data.
//
// Nothing is stored per board: the columns are computed on every read, which is why
// an issue leaves a board the moment it stops matching.
func (s *Server) renderBoard(w http.ResponseWriter, r *http.Request) {
	board, err := s.enforcer.BoardByName(r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	view, err := s.currentView()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	q, err := query.Parse(board.Query, s.schema)
	if err != nil {
		// The schema may have moved under a saved board. Say so rather than
		// returning an empty board that looks like "no work".
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Errorf("board %q no longer parses against the current schema: %w", board.Name, err))
		return
	}
	order, buckets := query.Group(q.Filter(view, s.schema), board.GroupBy, s.schema)
	columns := make([]map[string]any, 0, len(order))
	for _, name := range order {
		columns = append(columns, map[string]any{
			"name": name, "count": len(buckets[name]), "issues": buckets[name],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name": board.Name, "query": board.Query,
		"group_by": board.GroupBy, "columns": columns,
	})
}

func (s *Server) deleteBoard(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	if err := s.enforcer.DeleteBoard(p, r.PathValue("name"), s.now()); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// defaultIssueType is what a title-only create produces.
//
// With a hierarchy declared this is the most granular type — the bottom level —
// because someone typing a title and pressing enter is capturing a piece of work,
// not opening an epic. Taking the first declared type would default to the
// outermost, which is almost always wrong.
func (s *Server) defaultIssueType() (string, error) { return s.enforcer.DefaultIssueType() }

// listAncestors returns the chain from an issue up to its root, nearest first.
// setMulti replaces the values of a multi-valued field.
func (s *Server) setMulti(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Values []string `json:"values"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	when, ok := s.at(w, r, p, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.enforcer.SetMulti(p, r.PathValue("id"), r.PathValue("field"), body.Values, when); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// checklistItem adds a criterion, or marks one met or unmet.
//
// One route rather than three: adding an item and checking it are the same shape of
// request, and splitting them would mean a caller has to know whether the item
// already exists before choosing a URL.
func (s *Server) checklistItem(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text    string `json:"text"`
		Checked *bool  `json:"checked"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	when, ok := s.at(w, r, p, r.PathValue("id"))
	if !ok {
		return
	}
	id, field := r.PathValue("id"), r.PathValue("field")

	if body.Checked == nil {
		if err := s.enforcer.AddChecklistItem(p, id, field, body.Text, when); err != nil {
			writeDomainError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"field": field, "text": body.Text})
		return
	}
	if err := s.enforcer.SetChecklistItem(p, id, field, body.Text, *body.Checked, when); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeChecklistItem(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	when, ok := s.at(w, r, p, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.enforcer.RemoveChecklistItem(p, r.PathValue("id"), r.PathValue("field"), body.Text, when); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listAncestors(w http.ResponseWriter, r *http.Request) {
	view, err := s.currentView()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	id := r.PathValue("id")
	if _, ok := view.Issue(id); !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown issue %s", id))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ancestors": view.Ancestors(id),
		"depth":     view.Depth(id),
	})
}

// issueTree returns an issue's descendants, each with its depth so a caller can
// render the shape without walking parents itself.
func (s *Server) issueTree(w http.ResponseWriter, r *http.Request) {
	view, err := s.currentView()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	id := r.PathValue("id")
	root, ok := view.Issue(id)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown issue %s", id))
		return
	}

	depth := 0 // unlimited
	if raw := r.URL.Query().Get("depth"); raw != "" {
		depth, err = strconv.Atoi(raw)
		if err != nil || depth < 0 {
			writeError(w, http.StatusBadRequest,
				fmt.Errorf("depth must be zero or more, got %q", raw))
			return
		}
	}

	all := view.Descendants(id, depth)
	limit, offset, err := page(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// A subtree can be larger than a page, so it is bounded like any other list and
	// reports the total rather than silently truncating.
	start := min(offset, len(all))
	end := min(start+limit, len(all))

	rootDepth := view.Depth(id)
	nodes := make([]map[string]any, 0, end-start)
	for _, childID := range all[start:end] {
		issue, _ := view.Issue(childID)
		nodes = append(nodes, map[string]any{
			"issue": issue,
			"depth": view.Depth(childID) - rootDepth,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"root":   root,
		"nodes":  nodes,
		"total":  len(all),
		"limit":  limit,
		"offset": offset,
	})
}

// listDependencies returns both directions plus why the issue is blocked.
func (s *Server) listDependencies(w http.ResponseWriter, r *http.Request) {
	got, err := s.enforcer.DependenciesOf(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, got)
}

// addDependency records a dependency. A cycle is a 200 with a warning, not an error:
// the write succeeded, and the caller needs to know what it just created.
func (s *Server) addDependency(w http.ResponseWriter, r *http.Request) {
	var body struct {
		On string `json:"on"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	when, ok := s.at(w, r, p, r.PathValue("id"))
	if !ok {
		return
	}
	res, err := s.enforcer.AddDependency(p, r.PathValue("id"), body.On, when)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	out := map[string]any{"id": r.PathValue("id"), "on": body.On}
	if warning := res.Warning(); warning != "" {
		out["warning"] = warning
		out["cycle"] = res.Cycle
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) removeDependency(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	when, ok := s.at(w, r, p, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.enforcer.RemoveDependency(p, r.PathValue("id"), r.PathValue("on"), when); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listCycles reports every dependency cycle in the project.
//
// A cycle is recorded rather than refused, so it needs somewhere to be seen. This is
// the aggregate view — the same reasoning as showing unused fields or untracked
// commits: the individual decision is reasonable, and only the total is alarming.
func (s *Server) listCycles(w http.ResponseWriter, r *http.Request) {
	cycles, err := s.enforcer.Cycles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]map[string]any, 0, len(cycles))
	for _, cycle := range cycles {
		out = append(out, map[string]any{
			"cycle": cycle,
			"warning": fmt.Sprintf("%s → %s. Nothing in this cycle can start until one of these dependencies is removed.",
				strings.Join(cycle, " → "), cycle[0]),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"cycles": out, "total": len(out)})
}

func (s *Server) listActors(w http.ResponseWriter, r *http.Request) {
	ids, err := s.enforcer.Actors()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"actors": ids})
}

func (s *Server) getActor(w http.ResponseWriter, r *http.Request) {
	p, err := s.enforcer.Principal(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": p.Actor.ID, "kind": p.Actor.Kind, "model": p.Actor.Model,
		"roles": p.Roles, "teams": p.Teams,
	})
}

// ---------------------------------------------------------------- writes

func (s *Server) createIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID     string            `json:"id"`
		Title  string            `json:"title"`
		Type   string            `json:"type"`
		Team   string            `json:"team"`
		Fields map[string]string `json:"fields"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Title == "" {
		writeError(w, http.StatusBadRequest, errors.New("title is required"))
		return
	}
	// Everything except the title has a defensible default, because a create that
	// demands twelve fields is the thing this product exists to remove.
	if body.Type == "" {
		var err error
		if body.Type, err = s.defaultIssueType(); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	fields := map[string]string{"title": body.Title}
	for k, v := range body.Fields {
		fields[k] = v
	}
	id := body.ID
	if id == "" {
		var err error
		if id, err = s.nextID(); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	when, ok := s.at(w, r, p, id)
	if !ok {
		return
	}
	if err := s.enforcer.CreateAs(p, id, body.Type, fields, body.Team, when); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) setFields(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	when, ok := s.at(w, r, p, r.PathValue("id"))
	if !ok {
		return
	}
	for field, value := range body {
		if err := s.enforcer.SetFieldAs(p, r.PathValue("id"), field, value, when); err != nil {
			writeDomainError(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) transition(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To       string `json:"to"`
		Evidence string `json:"evidence"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	when, ok := s.at(w, r, p, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.enforcer.TransitionAs(p, r.PathValue("id"), body.To, body.Evidence, when); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setParent(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Parent string `json:"parent"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	when, ok := s.at(w, r, p, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.enforcer.ReparentAs(p, r.PathValue("id"), body.Parent, when); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteIssue(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	when, ok := s.at(w, r, p, r.PathValue("id"))
	if !ok {
		return
	}
	if err := s.enforcer.DeleteAs(p, r.PathValue("id"), when); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) registerActor(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Kind  string `json:"kind"`
		Model string `json:"model"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	if err := s.enforcer.AuthoriseAdmin(p, body.ID); err != nil {
		writeDomainError(w, err)
		return
	}
	kind := event.ActorKind(body.Kind)
	if body.Kind == "" {
		kind = event.ActorHuman
	}
	if err := s.enforcer.RegisterActor(body.ID, kind, body.Model, s.now(), p.Actor); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": body.ID})
}

func (s *Server) grantRole(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Role string `json:"role"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	if err := s.enforcer.AuthoriseAdmin(p, r.PathValue("id")); err != nil {
		writeDomainError(w, err)
		return
	}
	if err := s.enforcer.GrantRole(r.PathValue("id"), body.Role, s.now(), p.Actor); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) revokeRole(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	if err := s.enforcer.AuthoriseAdmin(p, r.PathValue("id")); err != nil {
		writeDomainError(w, err)
		return
	}
	if err := s.enforcer.RevokeRole(r.PathValue("id"), r.PathValue("role"), s.now(), p.Actor); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addToTeam(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Team string `json:"team"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	if err := s.enforcer.AuthoriseAdmin(p, r.PathValue("id")); err != nil {
		writeDomainError(w, err)
		return
	}
	if err := s.enforcer.AddToTeam(r.PathValue("id"), body.Team, s.now(), p.Actor); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeFromTeam(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	if err := s.enforcer.AuthoriseAdmin(p, r.PathValue("id")); err != nil {
		writeDomainError(w, err)
		return
	}
	if err := s.enforcer.RemoveFromTeam(r.PathValue("id"), r.PathValue("team"), s.now(), p.Actor); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- helpers

// currentView returns the projection, caught up to the end of the log.
//
// Callers must not hold it across requests: it is shared, and a later catchup will
// mutate it. Every handler reads what it needs and returns.
func (s *Server) currentView() (*projection.Projection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.view.Catchup(); err != nil {
		// A projection that failed to apply an event is not trustworthy, and
		// serving stale state as if it were current would be worse than an error.
		// Rebuild from scratch so the next request starts clean.
		s.view = projection.New(s.log)
		return nil, fmt.Errorf("projection is out of date and could not catch up: %w", err)
	}
	return s.view, nil
}

// nextID allocates the next sequential issue id.
func (s *Server) nextID() (string, error) { return s.enforcer.NextIssueID() }

// linkCommit records a commit against an issue.
//
// PUT rather than POST because linking the same commit twice is the same request
// twice: the enforcer treats a repeat as a no-op, so the verb should say so.
func (s *Server) linkCommit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SHA        string `json:"sha"`
		Message    string `json:"message"`
		Repository string `json:"repository"`
		Branch     string `json:"branch"`
		Author     string `json:"author"`
		At         string `json:"at"`
	}
	if !decode(w, r, &body) {
		return
	}
	p, ok := s.principal(w, r)
	if !ok {
		return
	}

	c := enforce.Commit{
		SHA:        body.SHA,
		Message:    body.Message,
		Repository: body.Repository,
		Branch:     body.Branch,
		Author:     body.Author,
	}
	// The commit's own author time travels in the body, not in the ?at= parameter:
	// ?at= says when to record the write, and for a link those are different things
	// only by accident. One field, one meaning.
	if body.At != "" {
		when, err := time.Parse(time.RFC3339, body.At)
		if err != nil {
			writeError(w, http.StatusBadRequest,
				fmt.Errorf("at must be an RFC 3339 timestamp such as 2026-08-24T09:30:00Z, got %q", body.At))
			return
		}
		c.At = when.UTC()
	}

	if _, err := s.enforcer.LinkCommit(p, r.PathValue("id"), c, s.now().UTC()); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listCommits returns the commits linked to an issue.
func (s *Server) listCommits(w http.ResponseWriter, r *http.Request) {
	commits, err := s.enforcer.CommitsOf(r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(commits))
	for _, c := range commits {
		out = append(out, map[string]any{
			"sha":        c.SHA,
			"message":    c.Message,
			"repository": c.Repository,
			"branch":     c.Branch,
			"author":     c.Author,
			"at":         c.At,
			"linked_by":  c.LinkedBy,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"commits": out})
}

// schemaUsage reports what each declared thing is actually doing.
func (s *Server) schemaUsage(w http.ResponseWriter, r *http.Request) {
	usage, err := s.enforcer.SchemaUsage()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var unused int
	for _, u := range usage {
		if !u.Used() {
			unused++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"usage": usage, "declared": len(usage), "unused": unused,
	})
}

// issueToken generates a token for an actor. The token is in the response and nowhere
// else, ever again.
func (s *Server) issueToken(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	target := r.PathValue("id")
	// Issuing somebody else a token is granting them access, so it needs the same
	// authority as granting a role. Issuing your own is always allowed: that is how
	// an actor rotates a token they believe has leaked, and needing to ask somebody
	// else first is how a leaked token stays live over a weekend.
	if target != p.Actor.ID {
		if err := s.enforcer.AuthoriseAdmin(p, target); err != nil {
			writeDomainError(w, err)
			return
		}
	}

	token, err := s.enforcer.IssueToken(target, s.now(), p.Actor)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"actor": target,
		"token": token,
		"note":  "store this now; it cannot be shown again",
	})
}

// revokeTokens withdraws every token an actor holds.
func (s *Server) revokeTokens(w http.ResponseWriter, r *http.Request) {
	p, ok := s.principal(w, r)
	if !ok {
		return
	}
	target := r.PathValue("id")
	if target != p.Actor.ID {
		if err := s.enforcer.AuthoriseAdmin(p, target); err != nil {
			writeDomainError(w, err)
			return
		}
	}
	if err := s.enforcer.RevokeToken(target, s.now(), p.Actor); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listProducts returns the catalogue: every product Canon knows about.
//
// Answered entirely from what was ingested. Nothing here reads git, so the response
// is as current as the last refresh and says so.
func (s *Server) listProducts(w http.ResponseWriter, r *http.Request) {
	entries := s.products.Entries()
	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		row := map[string]any{
			"name":         e.Name(),
			"source":       e.Source,
			"refreshed_at": e.RefreshedAt,
		}
		if e.Err != "" {
			row["error"] = e.Err
			out = append(out, row)
			continue
		}
		var open, done int
		for _, inc := range e.Repository.Increments {
			if inc.Status == "done" || inc.Status == "abandoned" {
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
		out = append(out, row)
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
	writeJSON(w, http.StatusOK, e)
}

// at resolves the instant a write should be recorded at.
//
// Every write is stamped with the current time unless the caller supplies "at",
// which is how an import replays history that happened before Canon existed, and how
// a commit is linked with the timestamp it actually carried. Backdating is a
// permission of its own — see enforce.BackdateOp — so the ordinary path costs
// nothing and the historical one is deliberate.
//
// It is a query parameter rather than a body field so that one code path covers
// every write, including the DELETE routes that carry no body.
func (s *Server) at(w http.ResponseWriter, r *http.Request, p enforce.Principal, subject string) (time.Time, bool) {
	now := s.now().UTC()
	raw := r.URL.Query().Get("at")
	if raw == "" {
		return now, true
	}
	when, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		writeError(w, http.StatusBadRequest,
			fmt.Errorf("at must be an RFC 3339 timestamp such as 2026-08-24T09:30:00Z, got %q", raw))
		return time.Time{}, false
	}
	if err := s.enforcer.AuthoriseBackdate(p, subject, when.UTC(), now); err != nil {
		writeDomainError(w, err)
		return time.Time{}, false
	}
	// An issue's own history cannot start before the issue does. Commit links are
	// exempt and do not come through here — see enforce.CheckNotBeforeCreation.
	if err := s.enforcer.CheckNotBeforeCreation(subject, when.UTC()); err != nil {
		writeDomainError(w, err)
		return time.Time{}, false
	}
	return when.UTC(), true
}

// principalKey is where authenticate stores the caller for the request.
type principalKey struct{}

// authenticate resolves the caller once, for every route.
//
// Middleware rather than a call in each handler, because a per-handler check is a
// check somebody forgets: before this, every write authenticated and **no read did**,
// so an unauthenticated caller could read the entire tracker. Wrapping the router
// means a new route is behind this by construction rather than by remembering.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := s.identify(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		if id == "" {
			writeError(w, http.StatusUnauthorized,
				fmt.Errorf("%s header or Authorization: Bearer <token> is required", ActorHeader))
			return
		}
		p, err := s.enforcer.Principal(id)
		if err != nil {
			writeError(w, http.StatusUnauthorized, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, p)))
	})
}

// identify decides who is calling: the token if there is one, the claim if the actor
// being claimed has no token, and an error if they have one and it was not presented.
func (s *Server) identify(r *http.Request) (string, error) {
	if token := bearer(r); token != "" {
		// The token decides. A header claiming somebody else is ignored rather than
		// rejected: an old client sending both keeps working and cannot escalate.
		return s.enforcer.Verify(token)
	}

	claimed := r.Header.Get(ActorHeader)
	if claimed == "" {
		return "", nil
	}
	needsToken, err := s.enforcer.ActorRequiresToken(claimed)
	if err != nil {
		return "", err
	}
	if needsToken {
		return "", fmt.Errorf("%s holds a token, so a claimed identity is not accepted; send Authorization: Bearer <token>", claimed)
	}
	return claimed, nil
}

// principal returns the caller resolved by authenticate.
//
// Once any actor holds a token, a bearer token is the only accepted identity and the
// X-Canon-Actor header stops being trusted — a caller cannot present a valid token and
// then claim to be somebody else, which is the whole point of the change. An instance
// where nobody holds a token keeps the previous behaviour, so an upgrade cannot lock
// an existing deployment out of itself.
func (s *Server) principal(w http.ResponseWriter, r *http.Request) (enforce.Principal, bool) {
	p, ok := r.Context().Value(principalKey{}).(enforce.Principal)
	if !ok {
		// Only reachable if a route is mounted outside authenticate, which the
		// contract test forbids.
		writeError(w, http.StatusUnauthorized, fmt.Errorf("not authenticated"))
		return enforce.Principal{}, false
	}
	return p, true
}

// bearer reads a token from the Authorization header, or from the query string.
//
// The query string is there for the web UI, which cannot set a header on a page load.
// It is a real trade: tokens in URLs end up in server logs and browser history. It is
// accepted here because the alternative is a cookie and a session layer, and neither
// belongs in a tracker that is meant to be one binary — but it is the first thing to
// revisit if Canon ever faces a hostile network.
func bearer(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if token, ok := strings.CutPrefix(h, "Bearer "); ok {
			return strings.TrimSpace(token)
		}
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

// writeDomainError maps a domain error to a status.
//
// A proposal is 202 Accepted rather than 403: the request was understood and
// recorded for a human, which is a different outcome from being refused.
func writeDomainError(w http.ResponseWriter, err error) {
	var proposal *enforce.ProposalRequired
	if enforce.AsProposalRequired(err, &proposal) {
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":      "proposal_required",
			"proposal_id": proposal.ProposalID,
			"operation":   proposal.Operation,
			"subject":     proposal.Subject,
			"role":        proposal.Role,
			"message":     err.Error(),
		})
		return
	}
	writeError(w, http.StatusUnprocessableEntity, err)
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(r.Body).Decode(into); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON body: %w", err))
		return false
	}
	return true
}

// defaultLimit bounds an unqualified list. It is large enough that a normal project
// fits in one response and small enough that a pathological one cannot stall a read.
const defaultLimit = 200

// maxLimit caps what a caller may ask for.
const maxLimit = 1000

func page(r *http.Request) (limit, offset int, err error) {
	limit, offset = defaultLimit, 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			return 0, 0, fmt.Errorf("limit must be a positive number, got %q", raw)
		}
		if limit > maxLimit {
			return 0, 0, fmt.Errorf("limit may not exceed %d, got %d", maxLimit, limit)
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return 0, 0, fmt.Errorf("offset must be zero or more, got %q", raw)
		}
	}
	return limit, offset, nil
}

func intParam(r *http.Request, name string) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q", name, raw)
	}
	return n, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError returns the message verbatim. The domain's errors already name what
// the caller should have done, and rewording them here would discard that.
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}
