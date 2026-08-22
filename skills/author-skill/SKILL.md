---
name: author-skill
description: Creates or revises a skill in this repository following the Agent Skills open standard, including frontmatter, progressive disclosure and validation. Use when the user asks to "add a skill", "create a skill for X", "fix this skill", "the skill isn't triggering", or when a workflow has been repeated enough times to be worth encoding. Produces a SKILL.md plus any reference files and scripts.
license: Apache-2.0
allowed-tools: Bash Read Edit Write Grep Glob WebFetch
---

# Authoring a skill

A skill encodes something an agent would otherwise get wrong or have to be told every time.
Before writing one, check it is actually needed: if the agent already does the task well
without it, the skill is pure context cost.

## When a skill is the right answer

| You want | Use |
|---|---|
| Knowledge loaded on demand when a task matches | **Skill** |
| A rule that must apply to every action | `AGENTS.md` or `docs/constitution.md` |
| Something enforced mechanically | A script, hook or CI check |
| A separate context for a big search or a long job | A subagent |

Pick the smallest of these that works. Most things that feel like they need a skill need three
lines in `AGENTS.md`.

## Workflow

```
Authoring progress:
- [ ] 1. Establish the gap with a real failure
- [ ] 2. Write the description as a routing rule
- [ ] 3. Draft the body, minimally
- [ ] 4. Split anything long into references
- [ ] 5. Validate the format
- [ ] 6. Test on a fresh session
```

### 1. Establish the gap

Run the task without a skill and record where it went wrong. That failure is the specification.
Skills written from imagined needs document things nobody was going to get wrong anyway.

Keep at least three real scenarios you can retest against later.

### 2. Description as a routing rule

The description is the only part loaded at startup, and it is the whole basis for whether the
skill fires. Write it to answer "when should this activate?", not "what is this about?".

Include: what it does, the trigger phrases a user actually types, what it produces, and any
hard boundary ("read-only", "never marks work done"). Third person, always.

```yaml
# Good — routable
description: Turns a product spec into ordered, shippable increments in specs/increment-plan.md. Use after a spec is agreed, or when the user asks to "break this down" or "plan the work". Produces planned increments for approval — it does not write code.

# Bad — a summary, not a rule
description: Helps with planning work.
```

Frontmatter fields: `name` (required, lowercase/hyphens, max 64 chars, must match the directory
name), `description` (required, max 1024 chars), and optionally `license`, `compatibility`,
`metadata`, `allowed-tools`.

### 3. Draft the body, minimally

Assume the agent is capable. Only write what it does not already know: your conventions, your
paths, your gotchas, the order things must happen in. Delete any paragraph explaining a concept
a competent engineer would know.

Match specificity to fragility. Where several approaches are valid, give direction and let the
agent choose. Where the operation is fragile or must be consistent, give the exact command and
say not to vary it.

For multi-step work, give a checklist the agent can copy and tick — it is what stops steps
being skipped. Where quality matters, build in a loop: run the validator, fix, repeat.

### 4. Split anything long

Keep `SKILL.md` under 500 lines. Push depth into `references/`, executables into `scripts/`,
templates into `assets/`.

Keep references **one level deep** from `SKILL.md` — an agent following a chain of links tends
to skim rather than read, and gets partial information. Give reference files over 100 lines a
contents list at the top for the same reason.

### 5. Validate

```bash
python3 .sdlc/bin/validate-skills.py
```

Then check by eye:

- [ ] `name` matches the directory name
- [ ] Description states what *and* when, in third person
- [ ] Body under 500 lines
- [ ] References one level deep, forward slashes throughout
- [ ] No dates or "as of now" — use an "old patterns" section instead
- [ ] Consistent terminology (pick one word per concept and keep it)
- [ ] Examples are concrete, not abstract

### 6. Test on a fresh session

Load the skill in a new session and run your three scenarios. Watch specifically for: whether it
triggers unprompted, whether it reads the reference files you expected, and whether it skips a
step. If it did not trigger, the description is wrong — fix that before touching the body.

A reference file the agent never opens is either unnecessary or badly signposted from
`SKILL.md`. A file it opens every time belongs in `SKILL.md`.

After adding a skill, run `bash .sdlc/bin/link-agents.sh` so every agent surface sees it.
Never author a skill at a vendor path such as `.claude/skills/` — those are generated
symlinks, and a real file there forks the definition invisibly.
