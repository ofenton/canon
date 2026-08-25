package mcp

import (
	"net/http"
	"strings"
	"testing"
)

// routes mirrors the API's read surface. Kept as data here so the parity test can run
// without importing api, which imports ui and would make this a much heavier test.
func routes() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"GET /api/products":        nil,
		"GET /api/products/{name}": nil,
		"GET /api/increments":      nil,
		"GET /api/metrics":         nil,
		"GET /api/conformance":     nil,
		"GET /api/schema":          nil,
	}
}

// AC: THE SYSTEM SHALL offer agents over MCP exactly the routes it offers humans.
//
// Tools are derived from the route table rather than written twice, so this asserts
// the derivation rather than a hand-maintained list.
func TestToolParityWithTheAPI(t *testing.T) {
	tools := ToolsFrom(routes())
	if len(tools) != len(routes()) {
		t.Fatalf("got %d tools for %d routes", len(tools), len(routes()))
	}
	for _, tool := range tools {
		if tool.Description == "" {
			t.Errorf("tool %q has no description; an agent routes on this", tool.Name)
		}
		if tool.Name == "" {
			t.Error("a tool with no name cannot be called")
		}
	}
}

// A route added without a description would give agents a tool they cannot choose.
func TestEveryRouteHasADescription(t *testing.T) {
	for pattern := range routes() {
		if description(pattern) == "" {
			t.Errorf("route %q has no MCP description", pattern)
		}
	}
}

// Nothing writes, so no tool should describe a body. This is the MCP half of
// TestNoWriteRoutes.
func TestNoToolTakesAWriteBody(t *testing.T) {
	if len(bodyHints) != 0 {
		t.Fatalf("bodyHints is not empty: %v — Canon accepts no writes", bodyHints)
	}
	for _, tool := range ToolsFrom(routes()) {
		if strings.HasPrefix(tool.Name, "create_") || strings.HasPrefix(tool.Name, "update_") ||
			strings.HasPrefix(tool.Name, "delete_") {
			t.Errorf("tool %q looks like a write", tool.Name)
		}
	}
}
