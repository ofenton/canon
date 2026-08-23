package ui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

// AC: THE SYSTEM SHALL serve the UI from the binary with no separate asset deployment.
func TestServedFromTheBinary(t *testing.T) {
	// Serve from a directory that contains nothing. If the UI still loads, it came
	// from the binary and not the filesystem.
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d serving from an empty directory", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<title>Canon</title>") {
		t.Error("the served page is not the UI")
	}
}

// The UI must not pull anything from a network the operator does not control. A
// self-hosted tool that phones out to a CDN is not self-hosted.
func TestNoExternalAssets(t *testing.T) {
	page := mustAsset(t)
	for _, marker := range []string{"http://", "https://", "//cdn", "googleapis", "unpkg", "jsdelivr"} {
		if strings.Contains(page, marker) {
			t.Errorf("the UI references %q; everything must be embedded", marker)
		}
	}
	if strings.Contains(page, "<script src") || strings.Contains(page, "<link rel=\"stylesheet\"") {
		t.Error("the UI loads a separate asset; it must be one self-contained file")
	}
}

var actionRe = regexp.MustCompile(`\{\s*key:\s*"([^"]+)",\s*name:\s*"([^"]+)"`)

// AC: THE SYSTEM SHALL make every action available in the UI reachable by keyboard
// without pointer input.
//
// Checked structurally: every action lives in one registry with a binding, and the
// only click handlers permitted are on nav buttons that duplicate a "g" shortcut.
func TestEveryActionHasAKeyboardBinding(t *testing.T) {
	page := mustAsset(t)

	actions := actionRe.FindAllStringSubmatch(page, -1)
	if len(actions) < 10 {
		t.Fatalf("found %d actions in the registry; the UI should not have shrunk that far", len(actions))
	}
	seen := map[string]string{}
	for _, a := range actions {
		key, name := a[1], a[2]
		if key == "" {
			t.Errorf("action %q has no key binding", name)
		}
		if prev, dup := seen[key]; dup {
			t.Errorf("key %q is bound to both %q and %q", key, prev, name)
		}
		seen[key] = name
	}

	for _, required := range []string{"c", "j", "k", "/", "?", "t"} {
		if _, ok := seen[required]; !ok {
			t.Errorf("no binding for %q", required)
		}
	}

	// Any click handler must be on a nav button, which exists to make the shortcut
	// discoverable rather than to be the only way in.
	for _, snippet := range clickHandlers(page) {
		if !strings.Contains(snippet, "nav button") {
			t.Errorf("pointer-only affordance: %s", snippet)
		}
	}
}

// Selection must be scoped to the main region. An unscoped "tbody tr" also matches
// the help dialog's rows, so j/k would walk through the keyboard reference — which
// is exactly what happened before the browser test caught it.
func TestSelectionIsScopedToMain(t *testing.T) {
	page := mustAsset(t)
	for _, selector := range []string{`querySelectorAll("tbody tr`, `querySelector('[aria-selected`} {
		if strings.Contains(page, selector) {
			t.Errorf("unscoped selector %q will match the help dialog too", selector)
		}
	}
	if !strings.Contains(page, `querySelectorAll("#main tbody tr`) {
		t.Error("row selection is not scoped to #main")
	}
}

// The help dialog must be generated from the registry, so it cannot drift from it.
func TestHelpIsGeneratedFromTheRegistry(t *testing.T) {
	page := mustAsset(t)
	if !strings.Contains(page, "ACTIONS\n  .map") && !strings.Contains(page, "ACTIONS.map") {
		t.Error("the help table is not generated from ACTIONS; it will drift")
	}
}

// AC: WHEN a user presses the create shortcut THE SYSTEM SHALL open a title-only
// create field focused and ready for input.
func TestCreateDialogIsTitleOnlyAndFocused(t *testing.T) {
	page := mustAsset(t)
	form := between(page, `<dialog id="create">`, "</dialog>")
	if form == "" {
		t.Fatal("no create dialog")
	}
	inputs := regexp.MustCompile(`<input`).FindAllString(form, -1)
	if len(inputs) != 1 {
		t.Errorf("the create dialog has %d inputs; it must ask for a title and nothing else", len(inputs))
	}
	if !strings.Contains(form, `id="title"`) {
		t.Error("the single input is not the title")
	}
	if !strings.Contains(page, `el("title").focus()`) {
		t.Error("the create dialog does not focus its input; the shortcut would only move the work")
	}
}

func mustAsset(t *testing.T) string {
	t.Helper()
	b, err := Asset("index.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// clickHandlers returns each click registration with the two lines before it, since
// the element it is bound to is usually named on a preceding line.
func clickHandlers(page string) []string {
	lines := strings.Split(page, "\n")
	var out []string
	for i, line := range lines {
		if !strings.Contains(line, `"click"`) && !strings.Contains(line, "onclick") {
			continue
		}
		start := max(i-2, 0)
		out = append(out, strings.TrimSpace(strings.Join(lines[start:i+1], " ")))
	}
	return out
}
