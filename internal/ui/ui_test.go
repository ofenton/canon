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
