---
name: assess-codebase
description: Produces an evidence-backed assessment of an existing codebase through a chosen lens — security, performance, quality or modernization — and writes findings to specs/assessments/. Use when the user asks to "review the codebase", "find security issues", "why is this slow", "assess technical debt", "what would it take to modernize this", or before planning work on unfamiliar code. Read-only: it produces findings, never fixes.
license: Apache-2.0
allowed-tools: Bash Read Grep Glob Write WebSearch WebFetch
---

# Assessing a codebase

An assessment is the input to planning. Its value is entirely in whether the findings are true,
so every finding carries the evidence that produced it and an honest confidence rating.

**This skill does not change code.** If you find something trivially fixable, still do not fix
it — record it and let `plan-increments` decide. Assessment that quietly edits leaves nobody
able to trust the report.

## Choose a lens

One lens per run. Read the lens file before you start:

| Lens | Read | Answers |
|---|---|---|
| Security | [references/security.md](references/security.md) | Where can this be attacked? |
| Performance | [references/performance.md](references/performance.md) | Where does time and money go? |
| Quality | [references/quality.md](references/quality.md) | What makes this hard to change safely? |
| Modernization | [references/modernization.md](references/modernization.md) | What would moving this cost? |

If the user has not named a lens, ask which one rather than doing a shallow pass of all four.
A mixed assessment produces a mixed backlog and nobody knows what has actually been covered.

## Workflow

```
Assessment progress:
- [ ] 1. Establish scope and say what is out of it
- [ ] 2. Map the ground — build, run, read
- [ ] 3. Sweep with tools, then read with judgement
- [ ] 4. Evidence every finding
- [ ] 5. Rate severity and confidence
- [ ] 6. Write the report
```

### 1. Scope

Name what you will review and what you will not, before you start. An assessment silently
scoped to whatever fitted in context is worse than a narrow one that says so.

### 2. Map the ground

Read `README`, `AGENTS.md`, `docs/constitution.md`, build and dependency files, CI config, and
the entry points. Get the thing building and its tests running if you can — a repo whose tests
do not pass is itself a finding.

```bash
git log --oneline -20
git log --format='%an' | sort | uniq -c | sort -rn | head    # who knows this code
find . -name '*.py' -o -name '*.ts' | head -50               # shape of the tree
```

### 3. Sweep, then read

Run whatever automated tooling exists for the lens (the lens file names the usual candidates),
then read the code the sweep points at. Tool output is a lead, not a finding — scanners produce
false positives, and confirming one by reading is the work.

The highest-value findings usually come from reading, not scanning: trust boundaries, the
difference between what a comment claims and what the code does, and the places where two
subsystems disagree about an invariant.

### 4. Evidence

Every finding needs something you actually observed: a snippet with `file:line`, a command and
its output, a timing, a query plan. If you cannot produce evidence, you have a hypothesis —
label it as one or drop it.

### 5. Severity and confidence

Rate both, separately. Severity is how bad it is if real; confidence is how sure you are that it
is real. A Critical/Low-confidence finding is a request to investigate, not a request to fix, and
conflating the two is how backlogs fill with phantoms.

Say what you checked and found clean. Absence of evidence is information the reader needs.

### 6. Write the report

Use [.sdlc/templates/assessment.md](../../.sdlc/templates/assessment.md). Write to
`specs/assessments/<YYYY-MM-DD>-<lens>.md`. Number findings `<LENS>-001` so increments can cite
them. Commit the report.

Then hand to `plan-increments` — do not plan the remediation here. Assessment and planning are
separate so that a human can disagree with the priorities without relitigating the findings.
