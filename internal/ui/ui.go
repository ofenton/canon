// Package ui serves Canon's web interface.
//
// The UI is embedded in the binary, so there is nothing to deploy alongside it and
// no way for the assets to be a different version from the server.
//
// It is mounted outside the API's route table on purpose. Routes() is the contract
// agents get, and the MCP tool list is derived from it; a UI route in there would
// become a meaningless tool, and the "every route is under /api" test would have to
// be weakened. Keeping them separate lets that test stay strict.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets
var assets embed.FS

// Handler serves the UI at the root.
func Handler() http.Handler {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		// The embed directive guarantees this exists; a failure here is a build
		// problem, not a runtime one.
		panic("ui: embedded assets missing: " + err.Error())
	}
	return http.FileServer(http.FS(sub))
}

// Asset returns one embedded file, for tests that assert what is shipped.
func Asset(name string) ([]byte, error) { return assets.ReadFile("assets/" + name) }
