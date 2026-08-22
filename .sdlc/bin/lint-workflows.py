#!/usr/bin/env python3
"""Parse every GitHub Actions workflow.

A workflow that does not parse fails remotely as a "workflow file issue" with no
jobs and no logs, which is slow and confusing to diagnose. Catching it locally
costs milliseconds.
"""

import pathlib
import sys

try:
    import yaml
except ImportError:
    print("pyyaml not installed — skipping workflow lint", file=sys.stderr)
    sys.exit(0)


def main() -> int:
    root = pathlib.Path(".github/workflows")
    if not root.is_dir():
        print("no .github/workflows directory")
        return 0

    failed = 0
    for path in sorted(root.glob("*.y*ml")):
        try:
            yaml.safe_load(path.read_text())
        except yaml.YAMLError as err:
            print(f"  ✗ {path}\n     {err}", file=sys.stderr)
            failed += 1
        else:
            print(f"  ✓ {path}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
