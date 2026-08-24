package mcp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/api"
	"github.com/ofenton/canon/internal/enforce"
	"github.com/ofenton/canon/internal/event"
	"github.com/ofenton/canon/internal/schema"
)

var fixedTime = time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC)

func newFixture(t *testing.T, actor string) (*api.Server, *Server) {
	t.Helper()
	sch, err := schema.Load(filepath.Join("..", "schema", "testdata", "canon.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	log, err := event.Open(filepath.Join(t.TempDir(), "canon.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })

	e := enforce.New(sch, log)
	sys := event.Actor{ID: "bootstrap", Kind: event.ActorSystem}
	if err := e.RegisterActor("ollie", event.ActorHuman, "", fixedTime, sys); err != nil {
		t.Fatal(err)
	}
	if err := e.GrantRole("ollie", "admin", fixedTime, sys); err != nil {
		t.Fatal(err)
	}
	if err := e.AddToTeam("ollie", "platform", fixedTime, sys); err != nil {
		t.Fatal(err)
	}
	if err := e.RegisterActor("agent:one", event.ActorAgent, "claude-opus-5", fixedTime, sys); err != nil {
		t.Fatal(err)
	}
	if err := e.GrantRole("agent:one", "agent", fixedTime, sys); err != nil {
		t.Fatal(err)
	}
	if err := e.AddToTeam("agent:one", "platform", fixedTime, sys); err != nil {
		t.Fatal(err)
	}

	apiSrv := api.New(sch, log, e, func() time.Time { return fixedTime })
	return apiSrv, NewServer(apiSrv.Handler(), apiSrv.Routes(), actor)
}

// AC: THE SYSTEM SHALL expose an MCP tool for every operation available in the HTTP API.
// AC: WHEN an API operation is added without a corresponding MCP tool THE SYSTEM SHALL
// fail its test suite.
//
// Parity is structural here — tools are derived from the route table — so this test
// verifies the property rather than enumerating a hand-written list.
func TestToolParityWithTheAPI(t *testing.T) {
	apiSrv, mcpSrv := newFixture(t, "ollie")

	if got, want := len(mcpSrv.Tools()), len(apiSrv.Routes()); got != want {
		t.Fatalf("tools: got %d, routes: %d — every route must have a tool", got, want)
	}

	names := map[string]bool{}
	for _, tool := range mcpSrv.Tools() {
		if names[tool.Name] {
			t.Errorf("duplicate tool name %q — an agent could not address both", tool.Name)
		}
		names[tool.Name] = true
		if tool.Description == "" || strings.HasPrefix(tool.Description, "GET ") {
			t.Errorf("tool %q has no written description; an agent routes on this", tool.Name)
		}
		if tool.InputSchema["type"] != "object" {
			t.Errorf("tool %q has no object input schema", tool.Name)
		}
		verb, _, ok := strings.Cut(tool.Name, "_")
		if !ok {
			t.Errorf("tool %q is not verb_noun", tool.Name)
		}
		switch verb {
		case "list", "get", "create", "update", "set", "delete", "transition", "approve", "reject":
		default:
			t.Errorf("tool %q starts with %q, which is not an action an agent would look for", tool.Name, verb)
		}
		if verb != "list" && strings.HasSuffix(tool.Name, "s") && !strings.HasSuffix(tool.Name, "fields") {
			t.Errorf("tool %q is plural but acts on one thing", tool.Name)
		}
	}
}

// A route added without a description must be caught, not silently shipped with the
// pattern as its description.
func TestEveryRouteHasADescription(t *testing.T) {
	apiSrv, _ := newFixture(t, "ollie")
	for pattern := range apiSrv.Routes() {
		if _, ok := descriptions[pattern]; !ok {
			t.Errorf("route %q has no MCP description", pattern)
		}
	}
	for pattern := range descriptions {
		if _, ok := apiSrv.Routes()[pattern]; !ok {
			t.Errorf("description for %q describes no route", pattern)
		}
	}
}

func rpc(t *testing.T, s *Server, method string, params any) map[string]any {
	t.Helper()
	req := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		req["params"] = params
	}
	line, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := s.Serve(strings.NewReader(string(line)+"\n"), &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(out.String()), &resp); err != nil {
		t.Fatalf("decode %q: %v", out.String(), err)
	}
	return resp
}

func callText(t *testing.T, s *Server, name string, args map[string]any) (string, bool) {
	t.Helper()
	resp := rpc(t, s, "tools/call", map[string]any{"name": name, "arguments": args})
	if errObj, ok := resp["error"]; ok {
		t.Fatalf("tool %s: %v", name, errObj)
	}
	result := resp["result"].(map[string]any)
	content := result["content"].([]any)[0].(map[string]any)
	return content["text"].(string), result["isError"].(bool)
}

func TestInitializeAndList(t *testing.T) {
	_, s := newFixture(t, "ollie")

	resp := rpc(t, s, "initialize", nil)
	result := resp["result"].(map[string]any)
	if result["protocolVersion"] != ProtocolVersion {
		t.Errorf("protocolVersion: got %v", result["protocolVersion"])
	}

	resp = rpc(t, s, "tools/list", nil)
	tools := resp["result"].(map[string]any)["tools"].([]any)
	if len(tools) == 0 {
		t.Fatal("no tools listed")
	}
}

// AC: WHEN an agent calls an MCP tool THE SYSTEM SHALL apply identical schema
// validation to the equivalent HTTP call.
func TestSchemaValidationIsIdenticalOverMCP(t *testing.T) {
	_, s := newFixture(t, "ollie")

	text, isErr := callText(t, s, "create_issue", map[string]any{"title": "Search is slow", "team": "platform"})
	if isErr {
		t.Fatalf("create failed: %s", text)
	}

	text, isErr = callText(t, s, "update_issue_fields", map[string]any{"id": "CANON-1", "storyPoints": "8"})
	if !isErr {
		t.Error("an undefined field must be refused over MCP too")
	}
	if !strings.Contains(text, "storyPoints") || !strings.Contains(text, "not defined in the schema") {
		t.Errorf("the HTTP error must reach the agent verbatim, got: %s", text)
	}

	text, isErr = callText(t, s, "transition_issue", map[string]any{"id": "CANON-1", "to": "done"})
	if !isErr {
		t.Error("an illegal transition must be refused over MCP too")
	}
	if !strings.Contains(text, "in_progress") {
		t.Errorf("the error must name the permitted transitions, got: %s", text)
	}
}

// An agent driving the full lifecycle over MCP alone, ending in a proposal.
func TestFullLifecycleOverMCPOnly(t *testing.T) {
	apiSrv, admin := newFixture(t, "ollie")
	agent := NewServer(apiSrv.Handler(), apiSrv.Routes(), "agent:one")

	if text, isErr := callText(t, admin, "create_issue",
		map[string]any{"title": "Search is slow", "team": "platform"}); isErr {
		t.Fatalf("create: %s", text)
	}
	if text, isErr := callText(t, agent, "transition_issue",
		map[string]any{"id": "CANON-1", "to": "in_progress"}); isErr {
		t.Fatalf("start: %s", text)
	}
	if text, isErr := callText(t, agent, "transition_issue",
		map[string]any{"id": "CANON-1", "to": "in_review", "evidence": "312 passed"}); isErr {
		t.Fatalf("review: %s", text)
	}

	// The agent may only propose completion. 202 is not an error: the attempt was
	// recorded, which is a success from the agent's point of view.
	text, isErr := callText(t, agent, "transition_issue", map[string]any{"id": "CANON-1", "to": "done"})
	if isErr {
		t.Errorf("a recorded proposal must not be reported as an error: %s", text)
	}
	if !strings.Contains(text, "proposal_required") || !strings.Contains(text, "PROP-1") {
		t.Errorf("the proposal id must reach the agent, got: %s", text)
	}

	// A human approves, over MCP as well.
	if text, isErr := callText(t, admin, "approve_proposal", map[string]any{"id": "PROP-1"}); isErr {
		t.Fatalf("approve: %s", text)
	}
	text, _ = callText(t, admin, "get_issue", map[string]any{"id": "CANON-1"})
	if !strings.Contains(text, `"state":"done"`) {
		t.Errorf("issue should be done after approval, got: %s", text)
	}
}

func TestMissingArgumentIsReported(t *testing.T) {
	_, s := newFixture(t, "ollie")
	resp := rpc(t, s, "tools/call", map[string]any{"name": "get_issue", "arguments": map[string]any{}})
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected an error, got %v", resp)
	}
	if !strings.Contains(errObj["message"].(string), "id") {
		t.Errorf("the error must name the missing argument, got: %v", errObj["message"])
	}
}

func TestUnknownToolAndMethod(t *testing.T) {
	_, s := newFixture(t, "ollie")
	if resp := rpc(t, s, "tools/call", map[string]any{"name": "teleport"}); resp["error"] == nil {
		t.Error("an unknown tool must error")
	}
	if resp := rpc(t, s, "wat", nil); resp["error"] == nil {
		t.Error("an unknown method must error")
	}
}
