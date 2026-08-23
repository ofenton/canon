# docs-001: Project README

## Context

The repository still carried the agentic-SDLC template's README, which described the scaffold
rather than the product. It is the first thing anyone sees.

## Design notes

**The problem section leads with measured numbers, not adjectives.** "Jira is bloated" is an
opinion; 90–100 workflows, 700+ fields with over half unused, and sixteen spellings of
"completed" is an argument. The whole product only makes sense once a reader believes the
divergence is real.

**"Try to break it" is given its own section.** The refusals are more convincing than the happy
path — anyone can demo a create form. Being told `field "storyPoints" is not defined in the
schema; defined fields are …` is the product working.

**"What is not built" is explicit and near the top of the reader's path.** A README that implies a
web UI exists costs a contributor an hour and their goodwill. The authentication warning is stated
as an instruction — *do not expose an instance to a network you do not control* — rather than a
caveat, because a caveat gets skimmed.

**Deliberate exclusions are listed alongside the unbuilt work**, so nobody opens an issue asking
for story points. Refusing them is the product, and a README that only lists what is missing
invites the wrong contributions.

## Evidence

**Verified by:** implementing session, `inc/docs-001-readme`

### Describes what Canon is and the problem it addresses, with evidence

The problem table cites the measured Jira Cloud figures; the idea section states the wedge and
the four opinions that follow from it.

### The quick start takes a reader from clone to a working instance

Run verbatim in a fresh clone at `/tmp`, not in the working tree:

```
### make build                     ok
### cp schema + bootstrap          registered you as admin in team platform
### create                         {"id":"CANON-1"}

### the three refusals from the README
  storyPoints: field "storyPoints" is not defined in the schema; defined fields are c…
  todo->done:  CANON-1 cannot move from "todo" to "done"; permitted transitions from …
  no header:   401

### the agent path
  in_progress:        204
  in_review no evid:  422
  in_review + evid:   204
  done:               202 proposal_required transition:in_review->done

### a bad canon.yaml refuses to start
  canon: parsing schema canon.yaml: line 72: mapping key "version" already defined at line 3
```

Every command in the README was executed as written. None needed adjusting.

### Documents every API route currently implemented

Cross-checked mechanically against `Routes()`:

```
routes in code: 17
undocumented: none
```

### States plainly what is not built

Six items under "What is not built", including authentication with an explicit instruction not to
expose an instance, plus the deliberate exclusions.

### Scope

`README.md` and this evidence file. Documentation only, as scoped.

### Not verified

The GitHub clone URL in the quick start is written for a public repository; the repo is currently
private, so `git clone https://…` will fail for anyone without access until it is made public.
Correct for the intended state, wrong for today. Recorded rather than quietly using an SSH URL
that would need changing at launch.
