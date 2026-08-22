#!/usr/bin/env bash
# Initialise a new product repository from this template.
#
# Does every mechanical step, checks its own work, and prints the two things that
# genuinely need a human. Safe to re-run: it changes nothing that is already correct.
set -euo pipefail

cd "$(dirname "$0")/../.."
ROOT="$(pwd)"

PROJECT_NAME="${1:-$(basename "$ROOT")}"
KEEP_EXAMPLE=0
ALLOW_NESTED=0
for arg in "${@:2}"; do
  case "$arg" in
    --keep-example) KEEP_EXAMPLE=1 ;;
    --nested)       ALLOW_NESTED=1 ;;
    *) echo "unknown option: $arg" >&2; exit 2 ;;
  esac
done

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
ok()  { printf '  ✓ %s\n' "$*"; }
note(){ printf '  · %s\n' "$*"; }

say "Initialising $PROJECT_NAME"

# ---------------------------------------------------------------- git
# `git rev-parse` succeeds from anywhere inside a repository, so a plain check would
# silently adopt an ancestor repo — and install our pre-commit hook into it. Compare
# roots, and refuse unless the nesting is deliberate.
if existing_root="$(git rev-parse --show-toplevel 2>/dev/null)"; then
  if [[ "$existing_root" == "$ROOT" ]]; then
    ok "git repository already present"
  elif [[ $ALLOW_NESTED -eq 1 ]]; then
    git init -q
    ok "initialised a nested git repository inside $existing_root"
  else
    cat >&2 <<NESTED

  Refusing to initialise: $ROOT is inside an existing repository.

    this project : $ROOT
    repo root    : $existing_root

  Continuing would install the pre-commit hook into $existing_root/.git/hooks and apply
  these validators to that whole repository. Either move this project outside it, or
  re-run with --nested to create a repository here deliberately.

NESTED
    exit 1
  fi
else
  git init -q
  ok "initialised a git repository"
fi

# ---------------------------------------------------------------- agent surfaces
say "Agent surfaces"
bash .sdlc/bin/link-agents.sh | sed 's/^/  ✓ /'

# ---------------------------------------------------------------- hooks
say "Enforcement"
bash .sdlc/hooks/install.sh | sed 's/^/  ✓ /'

# ---------------------------------------------------------------- worked example
say "Worked example"
EXAMPLE_DETAIL="specs/increments/sec-001-parameterize-the-search-query.md"
if [[ $KEEP_EXAMPLE -eq 1 ]]; then
  note "kept sec-001 (--keep-example)"
elif ! grep -q '^## sec-001:' specs/increment-plan.md 2>/dev/null; then
  note "already removed"
else
  # Only remove it if nobody has started using it as real work.
  status=$(awk '/^## sec-001:/{f=1} f&&/^- \*\*Status:\*\*/{print $3; exit}' specs/increment-plan.md)
  if [[ "$status" != "planned" ]]; then
    note "sec-001 is '$status', not 'planned' — leaving it alone, remove it by hand if it is not real"
  else
    python3 - <<'PY'
import pathlib, re
p = pathlib.Path("specs/increment-plan.md")
t = re.sub(r"\n## sec-001:.*?(?=\n## |\Z)", "\n", p.read_text(), flags=re.S)
p.write_text(t.rstrip() + "\n")
PY
    rm -f "$EXAMPLE_DETAIL"
    ok "removed the sec-001 worked example"
  fi
fi

# ---------------------------------------------------------------- naming
say "Project name"
for f in docs/constitution.md docs/architecture.md specs/product.md; do
  if grep -q '<Product / feature name>' "$f" 2>/dev/null; then
    python3 - "$f" "$PROJECT_NAME" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
p.write_text(p.read_text().replace("<Product / feature name>", sys.argv[2]))
PY
    ok "named $f"
  fi
done
note "constitution and architecture still carry placeholder sections — that is expected"

# ---------------------------------------------------------------- verify
say "Checks"
status=0
python3 .sdlc/bin/validate-skills.py     | sed 's/^/  ✓ /' || status=1
python3 .sdlc/bin/validate-plan.py       | sed 's/^/  ✓ /' || status=1
python3 .sdlc/bin/check-traceability.py  | sed 's/^/  ✓ /' || status=1
if [[ $status -ne 0 ]]; then
  printf '\n  Initialisation left the repository in a failing state. Fix the above before continuing.\n' >&2
  exit 1
fi

# ---------------------------------------------------------------- what humans must do
cat <<'NEXT'

Two things need you, not an agent:

  1. Edit docs/constitution.md
     Replace the "Project-specific rules" section. Delete any rule you would not
     actually enforce — an aspirational constitution teaches agents the rules are optional.

  2. Work chore-001
     It is already 'approved' in the ledger. Commit this scaffold with the trailer,
     then verify and mark it done:

       git add -A
       git commit -m "chore-001: adopt the increment workflow" -m "Increment: chore-001"

     That runs the whole loop once, on the workflow itself, and proves the hooks fire.

Then start the loop. Open your agent in this directory and say one of:

  Existing code:  "Assess this repo through the security lens, then plan increments from
                   the findings."
  New product:    "Interview me about what we are building, then write the product spec."

The agent reads AGENTS.md and takes it from there. It will stop at Gate 1 and wait for you.
NEXT
