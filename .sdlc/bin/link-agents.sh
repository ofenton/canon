#!/usr/bin/env bash
# Expose this repository's agent configuration to every coding agent, without
# duplicating any of it.
#
# There is exactly one copy of everything: AGENTS.md and skills/. Each agent looks
# for its own conventional path, so we symlink those paths at the single source.
# Nothing here is authored per-agent, so no agent can drift from the others, and
# dropping a vendor costs one line in the tables below.
#
# Run after cloning, and after adding a skill.
set -euo pipefail

cd "$(dirname "$0")/../.."

# path-that-agents-look-for : what-it-points-at
SKILL_TARGETS=(
  ".claude/skills"          # Claude Code
  ".github/skills"          # GitHub Copilot
)

ENTRYPOINT_TARGETS=(
  "CLAUDE.md"                          # Claude Code
  ".github/copilot-instructions.md"    # GitHub Copilot
  "GEMINI.md"                          # Gemini CLI
)
# Codex, Cursor and Antigravity read AGENTS.md directly and need no symlink.
# To support another agent, add its path above — nothing else changes.

link() {
  local target="$1" source="$2"
  mkdir -p "$(dirname "$target")"
  if [[ -e "$target" && ! -L "$target" ]]; then
    echo "skip $target — exists and is not a symlink" >&2
    return
  fi
  local rel
  rel="$(python3 -c 'import os,sys; print(os.path.relpath(sys.argv[1], os.path.dirname(sys.argv[2]) or "."))' "$source" "$target")"
  ln -sfn "$rel" "$target"
  echo "linked $target -> $rel"
}

for target in "${SKILL_TARGETS[@]}"; do
  link "$target" "skills"
done

for target in "${ENTRYPOINT_TARGETS[@]}"; do
  link "$target" "AGENTS.md"
done
