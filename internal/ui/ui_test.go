package ui

import (
	"strings"
	"testing"
)

func page(t *testing.T) string {
	t.Helper()
	body, err := assets.ReadFile("assets/index.html")
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// One embedded file, no external requests. A CSS or font dependency would end a
// property the product claims and would break in any network Canon cannot reach.
func TestNoExternalRequests(t *testing.T) {
	src := page(t)
	for _, banned := range []string{"http://", "https://", "@import", "<link", "cdn."} {
		if strings.Contains(src, banned) {
			t.Errorf("the UI references %q; it must be self-contained", banned)
		}
	}
}

// Every asynchronous renderer must check its ticket before writing, or a superseded
// render paints over the current one — which is how pressing one nav item then another
// could land on the wrong screen under a highlighted tab.
//
// Structural rather than a browser test, because the failure is a race: navigating and
// looking passes most of the time on a broken build.
func TestEveryRendererChecksItsTicket(t *testing.T) {
	src := page(t)
	for _, fn := range []string{"renderProducts", "renderWork", "renderFlow", "renderConformance", "renderProduct"} {
		body := functionBody(src, "async function "+fn+"(")
		if body == "" {
			t.Errorf("%s is missing", fn)
			continue
		}
		if !strings.Contains(body, "await") {
			continue
		}
		guard := strings.Index(body, "current(seq)")
		if guard < 0 {
			t.Errorf("%s awaits but never checks current(seq); a superseded render would paint over the current one", fn)
			continue
		}
		if write := strings.Index(body, "main.innerHTML"); write >= 0 && guard > write {
			t.Errorf("%s writes to main before checking current(seq)", fn)
		}
	}
	if !strings.Contains(src, "let renderSeq = 0") {
		t.Error("the render generation counter is gone; the guards check nothing")
	}
}

// Every action is in one registry, and both input paths dispatch through it. Two
// implementations of the same action diverge; one registry cannot.
func TestActionsAreDeclaredOnce(t *testing.T) {
	src := page(t)
	if !strings.Contains(src, "const ACTIONS = [") {
		t.Fatal("the action registry is gone")
	}
	// The keyboard reference is generated from it, so it cannot drift from the keys.
	if !strings.Contains(src, "ACTIONS.map(") {
		t.Error("the keyboard help is not generated from the registry, so it can drift")
	}
	if !strings.Contains(src, "ACTIONS.find(") {
		t.Error("keyboard dispatch does not read the registry")
	}
}

// Canon accepts no writes, and the UI must not imply otherwise. A create form or a
// non-GET fetch here would be a screen offering something the server refuses.
func TestTheUIOffersNoWrites(t *testing.T) {
	src := page(t)
	for _, banned := range []string{`method: "POST"`, `method: "PUT"`, `method: "DELETE"`, `method:"POST"`} {
		if strings.Contains(src, banned) {
			t.Errorf("the UI issues %s; Canon accepts no writes", banned)
		}
	}
	if strings.Contains(src, "<form") {
		t.Error("the UI has a form; there is nothing to submit")
	}
}

// Every screen must say when what it shows was read. An aggregator that presents a
// four-hour-old view as current is worse than one that is honest about lag.
func TestScreensSayWhenTheyWereRead(t *testing.T) {
	if !strings.Contains(page(t), "refreshed_at") {
		t.Error("no screen reads refreshed_at, so a stale view cannot say it is stale")
	}
}

// functionBody returns one function's text, matched by brace depth so an inserted
// function between two others cannot silently widen the region checked.
func functionBody(page, signature string) string {
	start := strings.Index(page, signature)
	if start < 0 {
		return ""
	}
	open := strings.Index(page[start:], "{")
	if open < 0 {
		return ""
	}
	depth, i := 0, start+open
	for ; i < len(page); i++ {
		switch page[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return page[start : i+1]
			}
		}
	}
	return page[start:]
}

// Every piece of view state reaches the URL, so a view can be sent to somebody.
//
// Structural, and deliberately the shape that fails when the *next* increment adds
// state: a search query or a new filter that render() reads but the URL does not carry
// breaks this without anybody having to remember the requirement. That is the whole
// reason ui-001 comes before the three increments that add state.
//
// Two keys are exempt and named here rather than inferred. cursor is which row is
// highlighted — restoring a highlight in a fresh tab would be odd, and it is reset on
// every navigation anyway. limit is fixed, not chosen.
func TestEveryPieceOfViewStateIsInTheURL(t *testing.T) {
	src := page(t)

	_, rest, ok := strings.Cut(src, "const state = {")
	if !ok {
		t.Fatal("the state object is gone; this test no longer guards anything")
	}
	body, _, _ := strings.Cut(rest, "}")

	notShareable := map[string]bool{"cursor": true, "limit": true}
	var checked int
	for _, field := range strings.Split(body, ",") {
		key, _, ok := strings.Cut(strings.TrimSpace(field), ":")
		if !ok || notShareable[key] {
			continue
		}
		checked++
		// Both directions: writing it without reading it back produces a URL that
		// looks shareable and restores nothing, which is worse than no URL at all.
		for fn, half := range map[string]string{
			"stateToParams": after(src, "function stateToParams()"),
			"applyURL":      after(src, "function applyURL()"),
		} {
			if !strings.Contains(half, key) {
				t.Errorf("state.%s never appears in %s, so it cannot survive a copied URL", key, fn)
			}
		}
	}
	if checked < 4 {
		t.Fatalf("only %d shareable field(s) examined; the state object was not parsed", checked)
	}
}

// The UI navigates within the page, never by reloading it. A full load would discard
// the history this increment depends on, and back would leave Canon entirely.
func TestNavigationNeverReloadsThePage(t *testing.T) {
	src := page(t)
	for _, banned := range []string{"location.href =", "location.assign", "location.replace", "window.open"} {
		if strings.Contains(src, banned) {
			t.Errorf("the UI uses %q; navigation goes through history so back stays inside Canon", banned)
		}
	}
	for _, required := range []string{"history.pushState", "history.replaceState", `"popstate"`} {
		if !strings.Contains(src, required) {
			t.Errorf("%s is gone; the URL and the screen can now disagree", required)
		}
	}
}

// after returns the source following a marker, bounded to the function that follows it.
func after(src, marker string) string {
	_, rest, ok := strings.Cut(src, marker)
	if !ok {
		return ""
	}
	if end := strings.Index(rest, "\n}"); end > 0 {
		return rest[:end]
	}
	return rest
}
