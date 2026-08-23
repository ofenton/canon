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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/projection"
	"github.com/ofenton/canon/internal/query"
	"github.com/ofenton/canon/internal/schema"
)

// ActorHeader names the caller. v1 trusts it; see the package comment.
const ActorHeader = "X-Canon-Actor"

// Server exposes Canon over HTTP.
type Server struct {
	enforcer *enforce.Enforcer
	log      *event.Store
	schema   *schema.Schema
	now      func() time.Time
}

// New returns a Server. now is injectable so tests can assert on timestamps.
func New(s *schema.Schema, log *event.Store, e *enforce.Enforcer, now func() time.Time) *Server {
	if now == nil {
		now = time.Now
	}
	return &Server{enforcer: e, log: log, schema: s, now: now}
}

// Routes is the complete API surface, in one place.
//
// Kept as data rather than scattered across registration calls so the parity test
// can enumerate it, and so a reader can see the whole interface at once.
func (s *Server) Routes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"GET /api/schema":                      s.getSchema,
		"GET /api/events":                      s.listEvents,
		"GET /api/issues":                      s.listIssues,
		"POST /api/issues":                     s.createIssue,
		"GET /api/issues/{id}":                 s.getIssue,
		"DELETE /api/issues/{id}":              s.deleteIssue,
		"PATCH /api/issues/{id}/fields":        s.setFields,
		"POST /api/issues/{id}/transition":     s.transition,
		"PUT /api/issues/{id}/parent":          s.setParent,
		"GET /api/issues/{id}/children":        s.listChildren,
		"GET /api/proposals":                   s.listProposals,
		"GET /api/proposals/{id}":              s.getProposal,
		"POST /api/proposals/{id}/approve":     s.approveProposal,
		"POST /api/proposals/{id}/reject":      s.rejectProposal,
		"GET /api/boards":                      s.listBoards,
		"POST /api/boards":                     s.saveBoard,
		"GET /api/boards/{name}":               s.renderBoard,
		"DELETE /api/boards/{name}":            s.deleteBoard,
		"GET /api/actors":                      s.listActors,
		"POST /api/actors":                     s.registerActor,
		"GET /api/actors/{id}":                 s.getActor,
		"POST /api/actors/{id}/roles":          s.grantRole,
		"DELETE /api/actors/{id}/roles/{role}": s.revokeRole,
		"POST /api/actors/{id}/teams":          s.addToTeam,
		"DELETE /api/actors/{id}/teams/{team}": s.removeFromTeam,
	}
}

// Handler builds the router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for pattern, handler := range s.Routes() {
		mux.HandleFunc(pattern, handler)
	}
	return mux
}

// ---------------------------------------------------------------- reads

func (s *Server) getSchema(w http.ResponseWriter, r *http.Request) {
	type stateView struct {
		Name             string   `json:"name"`
		Category         string   `json:"category"`
		RequiresEvidence bool     `json:"requires_evidence,omitempty"`
		To               []string `json:"to"`
	}
	states := make([]stateView, 0, len(s.schema.States))
	for _, st := range s.schema.States {
		states = append(states, stateView{
			Name: st.Name, Category: string(st.Category),
			RequiresEvidence: st.RequiresEvidence, To: s.schema.PermittedFrom(st.Name),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":     s.schema.Version,
		"states":      states,
		"fields":      s.schema.Fields,
		"issue_types": s.schema.IssueTypes,
		"roles":       s.schema.RoleNames(),
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
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) listIssues(w http.ResponseWriter, r *http.Request) {
	view, err := s.view()
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
	writeJSON(w, http.StatusOK, map[string]any{"issues": q.Filter(view, s.schema)})
}

func (s *Server) getIssue(w http.ResponseWriter, r *http.Request) {
	view, err := s.view()
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
	view, err := s.view()
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
	view, err := s.view()
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
		if len(s.schema.IssueTypes) == 0 {
			writeError(w, http.StatusInternalServerError, errors.New("schema defines no issue types"))
			return
		}
		body.Type = s.schema.IssueTypes[0].Name
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
	if err := s.enforcer.CreateAs(p, id, body.Type, fields, body.Team, s.now()); err != nil {
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
	for field, value := range body {
		if err := s.enforcer.SetFieldAs(p, r.PathValue("id"), field, value, s.now()); err != nil {
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
	if err := s.enforcer.TransitionAs(p, r.PathValue("id"), body.To, body.Evidence, s.now()); err != nil {
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
	if err := s.enforcer.ReparentAs(p, r.PathValue("id"), body.Parent, s.now()); err != nil {
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
	if err := s.enforcer.DeleteAs(p, r.PathValue("id"), s.now()); err != nil {
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
	if err := s.enforcer.RemoveFromTeam(r.PathValue("id"), r.PathValue("team"), s.now(), p.Actor); err != nil {
		writeDomainError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- helpers

func (s *Server) view() (*projection.Projection, error) {
	p := projection.New(s.log)
	if err := p.Rebuild(); err != nil {
		return nil, err
	}
	return p, nil
}

// nextID allocates the next sequential issue id.
func (s *Server) nextID() (string, error) {
	view, err := s.view()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("CANON-%d", len(view.IssueIDs())+1), nil
}

func (s *Server) principal(w http.ResponseWriter, r *http.Request) (enforce.Principal, bool) {
	id := r.Header.Get(ActorHeader)
	if id == "" {
		writeError(w, http.StatusUnauthorized,
			fmt.Errorf("%s header is required", ActorHeader))
		return enforce.Principal{}, false
	}
	p, err := s.enforcer.Principal(id)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err)
		return enforce.Principal{}, false
	}
	return p, true
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
