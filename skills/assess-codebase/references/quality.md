# Quality lens

## Contents
- The question this lens answers
- Change safety
- Tests
- Structure
- Operability
- Tooling
- Rating severity

## The question this lens answers

Not "is this code beautiful" but **"what makes this codebase expensive or dangerous to change?"**
Style opinions are not findings. Anything you report must connect to a concrete risk: a change
that will break something silently, a bug class that keeps recurring, onboarding that takes
months, a deploy nobody dares do on a Friday.

## Change safety

- Where would a change break something with no test to catch it?
- What invariants are held only in someone's head or in a comment?
- Which modules does everything depend on, so every change is a blast-radius question?
- Where does the same rule exist in more than one place, so they can drift?
- What is duplicated because the abstraction is wrong versus genuinely coincidental?

Use history as evidence: files that change most often, and files that appear repeatedly in
bug-fix commits, are where quality problems actually cost money.

```bash
git log --format=format: --name-only | sort | uniq -c | sort -rn | head -20
git log --oneline --grep='fix' -i | wc -l
```

## Tests

- Do they run, and how long do they take? (A suite nobody runs is not a suite.)
- Do they test behaviour or implementation? Tests that break on every refactor are a tax.
- What is untested that matters — error paths, boundaries, concurrency, migrations?
- Are they deterministic? Find the flaky ones and name them.
- Is there anything between unit tests and manual clicking?

Coverage percentage is weak evidence. Coverage of the paths that carry risk is the finding.

## Structure

- Are boundaries real or nominal? Can any module reach into any other?
- Does the layering hold, or does business logic live in handlers and SQL in templates?
- Is there dead code, and how would anyone know?
- Are errors handled or swallowed? A bare `except: pass` is a finding with a location.
- Is configuration separated from code, and validated at startup rather than at first use?

## Operability

- Can you tell what the system is doing in production? Logs, metrics, traces.
- Do errors carry enough context to debug from, without leaking secrets?
- How long from commit to production, and how much of it is manual?
- Can a deploy be rolled back, and has anyone tried?

## Tooling

Linters and complexity tools give leads, not findings: `ruff` / `eslint` / `golangci-lint`,
`radon` or `lizard` for complexity, `jscpd` for duplication, coverage reports, and the
dependency graph. Read what they point at before reporting it.

## Rating severity

| Severity | Test |
|---|---|
| Critical | Actively causing incidents, or blocks all safe change in a core area |
| High | Materially slows delivery or lets a known bug class recur |
| Medium | Localised friction with a clear cost |
| Low | Worth doing when nearby |

Tie every finding to something observed. "God object" is an opinion; "this 2,000-line class
appears in 40% of bug-fix commits" is a finding.
