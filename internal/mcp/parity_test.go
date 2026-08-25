package mcp_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ofenton/canon/internal/api"
	"github.com/ofenton/canon/internal/catalogue"
	"github.com/ofenton/canon/internal/mcp"
)

// AC: THE SYSTEM SHALL offer agents over MCP exactly the routes it offers humans.
//
// Against the real route table, not a copy of it. The previous version of this test
// listed the routes again in its own file, which is precisely the drift it exists to
// prevent: a route added to the API and forgotten here would have passed.
//
// This lives in package mcp_test rather than mcp so it can import api without a cycle.
func TestToolsCoverEveryRealRoute(t *testing.T) {
	srv := api.New(catalogue.New(), time.Now)
	routes := srv.Routes()

	tools := mcp.ToolsFrom(routes)
	if len(tools) != len(routes) {
		t.Fatalf("%d tools for %d routes — an agent sees a different surface from a human",
			len(tools), len(routes))
	}
	for _, tool := range tools {
		if tool.Description == "" {
			t.Errorf("tool %q has no description; an agent routes on this", tool.Name)
		}
		if strings.HasPrefix(tool.Name, "create_") || strings.HasPrefix(tool.Name, "update_") ||
			strings.HasPrefix(tool.Name, "delete_") {
			t.Errorf("tool %q looks like a write; Canon accepts none", tool.Name)
		}
	}
}

// A route added without a description would give agents a tool they cannot choose
// between. Checked against the real table for the same reason as above.
func TestEveryRealRouteHasADescription(t *testing.T) {
	srv := api.New(catalogue.New(), time.Now)
	for _, tool := range mcp.ToolsFrom(srv.Routes()) {
		if tool.Description == "" {
			t.Errorf("a route has no MCP description: %s", tool.Name)
		}
	}
}
