# feat-032: A familiar visual language

## Context

The UI worked and looked like nothing in particular: a custom palette, flat rows, monospace state
names. Asked to make it feel like GitHub, which is the right instinct — a tracker is not the place
to teach somebody a new visual language. Every developer already knows what a grey bordered surface
with a 6px radius and a coloured state pill means.

## Design notes

**Primer copied by eye, not imported.** GitHub's palette, type scale, radii and surface conventions,
written into the existing token block. Importing Primer would have been faster and would have ended
a property the product claims — one embedded file, no external requests, no build step. That claim
is asserted by a test, and it is worth more than the hours saved.

**Presentation only.** No markup restructuring beyond wrapping the table in a surface and turning
the state cell into a pill. Nothing about behaviour, keyboard handling or data changed, which is why
the keyboard suite could be the guard rather than needing edits of its own.

**State pills carry the category, not the name.** `open` green, `active` amber, `closed` purple —
the schema's three categories, which is what the colour is for. A schema with sixteen states still
reads at a glance, which is the argument categories exist to make.

**Metrics became cards.** A single definition list rendered `31` and
`p50 11m · p85 1.2h · p95 9h` at the same size, which is the wrong emphasis: one is a number and one
is a distribution. Counts are now large, distributions are monospace at body size.

**An empty list says something.** Column headers over nothing is a bug report waiting to happen;
"Nothing matches **backdate**" is an answer.

## Evidence

**Verified by:** implementing session, `inc/feat-032-github-look`

### Light and dark, following the system preference

Both captured from the running instance holding Canon's own 38 increments. The screenshots are the
evidence for this increment and are attached to the pull request.

### AC: every existing keyboard check passes, unchanged

```
PASS  multi-value fields show every value  — conversion, p95_latency
PASS  no uncaught exceptions

all keyboard checks passed
```

**33 checks, no edits to `e2e/keyboard.mjs`.** That is the whole claim of this increment: if a
restyle needed the behaviour suite changed, it was not a restyle.

Full Go suite green across eleven packages.

### AC: one embedded file, no external requests

No `@import`, no `<link>`, no font URL, no CDN. `internal/ui` still embeds a single file, and the
existing test that asserts it passes.

### Scope

`git diff --cached --stat main` — run. One file: `internal/ui/assets/index.html`, the style block
and four render functions.

### Not verified

**No mobile layout.** The table does not collapse, the toolbar does not wrap usefully below about
40rem, and nothing has been tested on a phone. `feat-033` and `feat-034` touch the same screens and
this should be resolved once rather than three times.

**Colour contrast has not been measured.** The palette is GitHub's, which is accessible, but the
combinations here are mine and no contrast checker has been run over them.

**Dark mode follows the system only.** There is no toggle, so somebody who wants dark on a light
system cannot have it.
