#!/usr/bin/env bash
# Install the git hooks. Run once after cloning.
set -euo pipefail

project_root="$(cd "$(dirname "$0")/../.." && pwd)"
repo_root="$(git -C "$project_root" rev-parse --show-toplevel 2>/dev/null || true)"

if [[ -z "$repo_root" ]]; then
  echo "no git repository at $project_root — run 'git init' first" >&2
  exit 1
fi

# Installing into an ancestor repo would apply these validators to somebody else's project.
if [[ "$repo_root" != "$project_root" ]]; then
  echo "refusing: $project_root is inside the repository at $repo_root" >&2
  echo "run 'git init' here first so the hook gates this project only" >&2
  exit 1
fi

cd "$repo_root"
ln -sf ../../.sdlc/hooks/pre-commit .git/hooks/pre-commit
echo "installed .git/hooks/pre-commit -> .sdlc/hooks/pre-commit"
