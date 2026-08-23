// Package mcp exposes Canon over the Model Context Protocol.
//
// The tools are *derived from the HTTP route table*, not hand-written alongside it.
// A hand-maintained tool list drifts from the API the first time someone is in a
// hurry, and the drift is invisible until an agent cannot do something a human can.
// Generating them makes parity structural; the test then verifies the property
// rather than enumerating it.
//
// Transport is JSON-RPC 2.0 over stdio, which is what MCP clients expect.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
)

// ProtocolVersion is the MCP revision this server implements.
const ProtocolVersion = "2025-06-18"

// Tool is one callable operation.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`

	method string
	path   string
}

var pathParam = regexp.MustCompile(`\{([a-zA-Z_]+)\}`)

// descriptions gives each route a sentence an agent can route on. Anything without
// one is still exposed — a missing description is a documentation gap, not a reason
// to hide an operation from agents.
var descriptions = map[string]string{
	"GET /api/schema":                      "Read the organisation's issue schema: states, permitted transitions, fields, issue types and roles. Call this first to learn what is allowed.",
	"GET /api/events":                      "Read the raw append-only event log, optionally filtered by subject or sequence.",
	"GET /api/issues":                      "List issues, optionally filtered by state or team.",
	"POST /api/issues":                     "Create an issue. Only a title is required.",
	"GET /api/issues/{id}":                 "Read one issue's current state, fields, parent and last actor.",
	"DELETE /api/issues/{id}":              "Delete an issue. Its children are lifted to its parent, never orphaned.",
	"PATCH /api/issues/{id}/fields":        "Set one or more fields. Fields not defined in the schema are refused.",
	"POST /api/issues/{id}/transition":     "Move an issue to a new state. Some states require evidence. If your role may only propose the transition, this returns a proposal for a human to approve.",
	"PUT /api/issues/{id}/parent":          "Set or clear an issue's parent. Cycles are refused.",
	"GET /api/issues/{id}/children":        "List an issue's direct children.",
	"GET /api/proposals":                   "List proposals awaiting a human decision. Pass status=all for the full history.",
	"GET /api/proposals/{id}":              "Read one proposal, including who proposed it and why.",
	"POST /api/proposals/{id}/approve":     "Approve a proposal and apply it. Humans only.",
	"POST /api/proposals/{id}/reject":      "Reject a proposal, with an optional reason. Humans only.",
	"GET /api/boards":                      "List saved boards and the keys a board may group by.",
	"POST /api/boards":                     "Save a board: a name, a query and a grouping key. A board holds no membership of its own.",
	"GET /api/boards/{name}":               "Render a saved board against current data, grouped into columns.",
	"DELETE /api/boards/{name}":            "Delete a saved board. The issues it showed are unaffected.",
	"GET /api/actors":                      "List registered actor ids.",
	"POST /api/actors":                     "Register a human or agent actor.",
	"GET /api/actors/{id}":                 "Read an actor's roles and team membership.",
	"POST /api/actors/{id}/roles":          "Grant an actor a role defined in canon.yaml.",
	"DELETE /api/actors/{id}/roles/{role}": "Revoke a role from an actor.",
	"POST /api/actors/{id}/teams":          "Add an actor to a team.",
	"DELETE /api/actors/{id}/teams/{team}": "Remove an actor from a team.",
}

// bodyHints describe the JSON body each write expects, so an agent does not have to
// guess field names from a URL.
var bodyHints = map[string]map[string]string{
	"POST /api/issues":                 {"title": "required", "type": "optional", "team": "optional", "id": "optional"},
	"POST /api/issues/{id}/transition": {"to": "required, target state", "evidence": "required for some states"},
	"PUT /api/issues/{id}/parent":      {"parent": "issue id, or empty to clear"},
	"POST /api/proposals/{id}/reject":  {"reason": "optional"},
	"POST /api/actors":                 {"id": "required", "kind": "human or agent", "model": "required for agents"},
	"POST /api/actors/{id}/roles":      {"role": "required"},
	"POST /api/actors/{id}/teams":      {"team": "required"},
	"POST /api/boards":                 {"name": "required", "query": "required, e.g. team=platform priority=p1", "group_by": "optional, defaults to state"},
	"PATCH /api/issues/{id}/fields":    {"<field name>": "value, for any field in the schema"},
}

// ToolsFrom derives the tool list from an HTTP route table.
func ToolsFrom(routes map[string]http.HandlerFunc) []Tool {
	patterns := make([]string, 0, len(routes))
	for pattern := range routes {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)

	tools := make([]Tool, 0, len(patterns))
	for _, pattern := range patterns {
		method, path, _ := strings.Cut(pattern, " ")
		tools = append(tools, Tool{
			Name:        toolName(method, path),
			Description: description(pattern),
			InputSchema: inputSchema(pattern, path),
			method:      method,
			path:        path,
		})
	}
	return tools
}

// toolName turns "POST /api/issues/{id}/transition" into "transition_issue"-style
// snake case that reads as an action.
func toolName(method, path string) string {
	trimmed := strings.TrimPrefix(path, "/api/")
	parts := []string{}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == "" || strings.HasPrefix(seg, "{") {
			continue
		}
		parts = append(parts, seg)
	}
	verb := map[string]string{
		"GET": "get", "POST": "create", "PATCH": "update", "PUT": "set", "DELETE": "delete",
	}[method]

	// A GET with no path parameter lists; with one, it reads a single thing.
	listing := method == "GET" && !strings.Contains(path, "{")
	if listing {
		verb = "list"
	}

	// Trailing verbs already name the action for sub-resources.
	if last := parts[len(parts)-1]; method == "POST" && len(parts) > 1 {
		switch last {
		case "transition", "approve", "reject":
			return last + "_" + singular(parts[0])
		}
	}

	// Only a listing reads naturally in the plural: list_issues, but create_issue.
	// Tool names are the surface an agent reasons about, so they are worth getting
	// right even though they are derived.
	nouns := make([]string, len(parts))
	copy(nouns, parts)
	if !listing {
		for i := range nouns {
			nouns[i] = singular(nouns[i])
		}
	}
	return verb + "_" + strings.Join(nouns, "_")
}

// singular is deliberately naive: it covers the resource names this API actually
// uses. A pluralisation library would be a dependency earning nothing.
func singular(s string) string {
	switch s {
	case "issues":
		return "issue"
	case "actors":
		return "actor"
	case "proposals":
		return "proposal"
	case "roles":
		return "role"
	case "teams":
		return "team"
	case "fields":
		return "fields" // a field set, not one field
	}
	return strings.TrimSuffix(s, "s")
}

func description(pattern string) string {
	if d, ok := descriptions[pattern]; ok {
		return d
	}
	return pattern
}

func inputSchema(pattern, path string) map[string]any {
	props := map[string]any{}
	var required []string
	for _, match := range pathParam.FindAllStringSubmatch(path, -1) {
		props[match[1]] = map[string]any{"type": "string"}
		required = append(required, match[1])
	}
	for field, hint := range bodyHints[pattern] {
		props[field] = map[string]any{"type": "string", "description": hint}
		if strings.HasPrefix(hint, "required") && !strings.Contains(hint, "for") {
			required = append(required, field)
		}
	}
	sort.Strings(required)
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// Server speaks JSON-RPC 2.0 over a reader and writer.
type Server struct {
	handler http.Handler
	tools   []Tool
	actor   string
}

// NewServer returns an MCP server over an HTTP handler.
//
// Calls are dispatched through the same handler the network serves, so an agent and
// a human take an identical path through authorisation. Reimplementing dispatch here
// would create the second code path this design exists to avoid.
func NewServer(handler http.Handler, routes map[string]http.HandlerFunc, actor string) *Server {
	return &Server{handler: handler, tools: ToolsFrom(routes), actor: actor}
}

// Tools returns the derived tool list.
func (s *Server) Tools() []Tool { return s.tools }

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve reads JSON-RPC messages until the input closes.
func (s *Server) Serve(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	encoder := json.NewEncoder(out)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = encoder.Encode(response{JSONRPC: "2.0",
				Error: &rpcError{Code: -32700, Message: "parse error: " + err.Error()}})
			continue
		}
		resp := s.dispatch(req)
		// A notification has no id and takes no reply.
		if len(req.ID) == 0 {
			continue
		}
		if err := encoder.Encode(resp); err != nil {
			return fmt.Errorf("writing response: %w", err)
		}
	}
	return scanner.Err()
}

func (s *Server) dispatch(req request) response {
	resp := response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "canon", "version": "dev"},
		}
	case "tools/list":
		resp.Result = map[string]any{"tools": s.tools}
	case "tools/call":
		result, err := s.call(req.Params)
		if err != nil {
			resp.Error = &rpcError{Code: -32602, Message: err.Error()}
			return resp
		}
		resp.Result = result
	case "ping":
		resp.Result = map[string]any{}
	default:
		resp.Error = &rpcError{Code: -32601, Message: "unknown method " + req.Method}
	}
	return resp
}

// call routes a tool invocation through the HTTP handler.
func (s *Server) call(raw json.RawMessage) (any, error) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	var tool *Tool
	for i := range s.tools {
		if s.tools[i].Name == params.Name {
			tool = &s.tools[i]
			break
		}
	}
	if tool == nil {
		return nil, fmt.Errorf("unknown tool %q", params.Name)
	}

	path := tool.path
	body := map[string]any{}
	for key, value := range params.Arguments {
		placeholder := "{" + key + "}"
		if strings.Contains(path, placeholder) {
			path = strings.ReplaceAll(path, placeholder, fmt.Sprint(value))
			continue
		}
		body[key] = value
	}
	if remaining := pathParam.FindStringSubmatch(path); remaining != nil {
		return nil, fmt.Errorf("tool %q needs argument %q", tool.Name, remaining[1])
	}

	var payload io.Reader
	if len(body) > 0 {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = strings.NewReader(string(encoded))
	}

	req := httptest.NewRequest(tool.method, path, payload)
	req.Header.Set("X-Canon-Actor", s.actor)
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)

	text := strings.TrimSpace(rec.Body.String())
	if text == "" {
		text = fmt.Sprintf("%d %s", rec.Code, http.StatusText(rec.Code))
	}
	// isError marks refusals so an agent stops rather than treating a 422 body as
	// a successful result. A 202 proposal is deliberately not an error: the attempt
	// succeeded in being recorded.
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": rec.Code >= 400,
	}, nil
}
