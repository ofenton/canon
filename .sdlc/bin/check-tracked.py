#!/usr/bin/env python3
"""Fail if any tracked file matches the repository's own ignore rules.

Runtime state, build output and local config get committed by an incautious
`git add -A`, and nothing notices: once a file is tracked, git stops applying
ignore rules to it, so the .gitignore that should have prevented it silently
does nothing. This repository has now shipped a 2.4 MB binary and a set of
SQLite WAL files that way.

Exits 0 when the index is clean, 1 otherwise.
"""

from __future__ import annotations

import subprocess
import sys


def main() -> int:
    try:
        tracked = subprocess.run(
            ["git", "ls-files", "-z"],
            capture_output=True, check=True, timeout=30,
        ).stdout
    except (subprocess.CalledProcessError, FileNotFoundError, subprocess.TimeoutExpired, OSError) as err:
        print(f"error: could not list tracked files: {err}", file=sys.stderr)
        return 1

    if not tracked.strip(b"\0"):
        print("no tracked files")
        return 0

    # --no-index is essential: without it git stays silent about tracked files,
    # which are exactly the ones being looked for here.
    result = subprocess.run(
        ["git", "check-ignore", "--no-index", "--stdin", "-z", "-v"],
        input=tracked, capture_output=True, timeout=30,
    )

    # With -z, check-ignore -v emits four NUL-separated *fields* per record —
    # source, line, pattern, path — not colon-separated as it does without -z.
    fields = result.stdout.decode().split("\0")
    offenders = [
        (fields[i], fields[i + 1], fields[i + 2], fields[i + 3])
        for i in range(0, len(fields) - 3, 4)
        if fields[i + 3].strip()
    ]
    if not offenders:
        print("index clean — no tracked file matches an ignore rule")
        return 0

    print(f"{len(offenders)} tracked file(s) match an ignore rule:\n", file=sys.stderr)
    for source, line, pattern, path in offenders:
        where = f"{source}:{line}" if source else "an ignore rule"
        print(f"  ✗ {path}\n      matched by {where} ({pattern})", file=sys.stderr)
    print("\n  fix with: git rm --cached <path>", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
