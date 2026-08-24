# fix-004: One render wins

## Context

A screenshot of the running instance showed the Flow tab highlighted with the **issue list** in the
body. I first read that as a missing loading state — the nav updates synchronously while the body
waits on a fetch — probed it, saw the correct view, and said so.

That was wrong, and the probe was what misled me: it passed because the race went the other way
that time.

Every view fetches before it paints. Two navigations in quick succession leave two renders in
flight, and the screen shows whichever request happens to return last. Escape then `g m` clears
`main`, starts the metrics fetch, and then the *issue* fetch — started by the Escape — returns and
paints the list into a screen whose tab says Flow. Not a stale frame. A wrong one, and
non-deterministically so, which is why it looked fine when I went back to check.

## Design notes

**Each render takes a ticket.** `renderSeq` increments on entry; a renderer checks `current(seq)`
after every await and before every write. A render that has been superseded discards its result.

This is the same discipline the status bar needed, arrived at the same way: by watching it lose.
The status bar was fixed three times because each fix was applied to one function rather than to
the shape of the problem. This applies the guard to all five renderers at once, and asserts it.

**Asserted structurally, not by navigating and looking.** A browser test that navigates and checks
would pass most of the time on a broken build — which is precisely what my probe did. The test
parses the page, finds every `async function render*`, and fails if one awaits without checking its
ticket, or writes to `main` before checking. I verified the test fails by deleting one guard.

## Evidence

**Verified by:** implementing session, `inc/fix-004-render-race`

### The race, reproduced and then gone

Playwright with `/api/issues` deliberately slowed by 700ms, navigating away mid-fetch, eight times:

```
BEFORE the fix:  8/8 runs painted the wrong view
AFTER  the fix:  8/8 runs showed the view that was asked for
```

Same script, same server, same delay — only `index.html` differs.

### The test catches it

Deleting the guard from `renderMetrics`:

```
--- FAIL: TestEveryRendererChecksItsTicket
    ui_test.go:237: renderMetrics awaits but never checks current(seq);
                    a superseded render would paint over the current one
```

Restored, it passes. A structural test nobody has watched fail is a test that might assert nothing.

### Everything else still works

Full Go suite green across all ten packages, and the 27-check keyboard suite passes unchanged.

### Scope

`git diff --cached --stat main` — run. `renderSeq` and `current()`, six guards across five
renderers, and one test.

### Not verified

**Only `main` is guarded.** `say()` writes the status bar and `ask()` opens the prompt, both from
handlers that can also be superseded. Neither showed a defect under this test, and both were
tightened in earlier increments, but they are not covered by the new assertion.

**I reported this as "not a race" earlier in the session** on the strength of one probe. The probe
ran the same three sequences three times and metrics won each time; eight runs against a slowed
endpoint show it losing every time. One passing observation of a race is not evidence, and I should
have slowed the endpoint before drawing a conclusion rather than after.
