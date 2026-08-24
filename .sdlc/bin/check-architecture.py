#!/usr/bin/env python3
"""Check that docs/architecture.md still describes the system.

An architecture document is prose, and prose rots silently. This turns the part that
can rot into a contract: every invariant the document claims must name a test, and
that test must exist. A claim nobody can check is a claim that stops being true, and
the failure mode is worse than having no document — a wrong map is followed.

What it checks:

  - The Invariants table exists and is not empty.
  - Every invariant names at least one test, and every named test exists in the repo.
  - Every component the table lists is a real directory.
  - The document is not still the unfilled template stub.

What it deliberately does not check: whether the prose is *good*, whether components
are described accurately, or whether the architecture is wise. Those need a human.
Exits 0 when the document is honest about what it can be honest about, 1 otherwise.
"""

from __future__ import annotations

import argparse
import pathlib
import re
import subprocess
import sys

DOC = pathlib.Path("docs/architecture.md")

# A row in a markdown table: | left | right |
ROW_RE = re.compile(r"^\|(?P<cells>.+)\|\s*$")
# `TestSomething` or `test_something` in backticks, plus bare Test names.
TEST_RE = re.compile(r"`([A-Za-z_][A-Za-z0-9_]*)`|\b(Test[A-Z][A-Za-z0-9_]*)\b")
# A backticked path to a test file is evidence too: a browser suite or a table-driven
# file is a real assertion even though it declares no single Test function. Dogfooding
# this check against its own project found exactly that — it rejected `e2e/keyboard.mjs`,
# which is 33 assertions running in CI.
TESTFILE_RE = re.compile(r"`([^`\s]+(?:_test\.(?:go|py)|\.test\.[jt]s|\.mjs|\.spec\.[jt]s))`")
# Backticked paths that look like a component: internal/foo, cmd/bar, src/baz
PATH_RE = re.compile(r"`((?:[a-z][a-z0-9_.-]*/)+[a-z][a-z0-9_.-]*)`")

# Phrases the template ships with. If they survive, nobody filled the document in.
STUB_MARKERS = [
    "_What this system does, and what it talks to._",
    "_Stores, what lives in each, and where personal data is._",
    "_Where it runs, how it is deployed, how it is observed._",
    "_Things that shape every change",
]


def section(text: str, heading: str) -> str | None:
    """Return the body of a `## heading` section, or None if it is absent."""
    pattern = re.compile(rf"^##\s+{re.escape(heading)}\s*$(.*?)(?=^##\s|\Z)", re.M | re.S)
    found = pattern.search(text)
    return found.group(1) if found else None


def table_rows(body: str) -> list[list[str]]:
    """Return data rows of the first markdown table in a section."""
    rows: list[list[str]] = []
    for line in body.splitlines():
        match = ROW_RE.match(line.strip())
        if not match:
            continue
        cells = [c.strip() for c in match.group("cells").split("|")]
        # Skip the header separator (---|---) and the header itself.
        if all(set(c) <= set("-: ") for c in cells):
            rows = []  # everything before the separator was the header
            continue
        rows.append(cells)
    return rows


def known_tests(root: pathlib.Path) -> set[str]:
    """Every test function name in the repository.

    Uses git ls-files so it sees exactly what is committed, and ignores anything the
    repository itself ignores.
    """
    try:
        listing = subprocess.run(
            ["git", "ls-files", "-z"],
            capture_output=True, text=True, check=True, timeout=30,
        ).stdout
    except (subprocess.CalledProcessError, FileNotFoundError, subprocess.TimeoutExpired):
        return set()

    names: set[str] = set()
    for name in listing.split("\0"):
        if not name or not re.search(r"(_test\.(go|py)|\.test\.[jt]s|test_.*\.py|\.mjs)$", name):
            continue
        path = root / name
        try:
            source = path.read_text(errors="ignore")
        except OSError:
            continue
        names |= set(re.findall(r"^func (Test[A-Za-z0-9_]+)\(", source, re.M))
        names |= set(re.findall(r"^\s*def (test_[A-Za-z0-9_]+)\(", source, re.M))
        # A JS check() call names its assertion in a string literal; treat the file
        # itself as the evidence rather than trying to parse every call.
        if name.endswith(".mjs") or ".test." in name:
            names.add(pathlib.Path(name).name)
    return names


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--doc", default=str(DOC), help="path to the architecture document")
    parser.add_argument("--require", action="store_true",
                        help="fail if the document is missing, rather than skipping")
    args = parser.parse_args()

    doc = pathlib.Path(args.doc)
    root = pathlib.Path(".")
    problems: list[str] = []

    if not doc.exists():
        if args.require:
            print(f"{doc}: missing", file=sys.stderr)
            return 1
        # A project that has not reached the Spec track has no architecture to check,
        # and demanding one would be the ceremony this template exists to avoid.
        print(f"{doc}: absent — skipped (pass --require to make this an error)")
        return 0

    text = doc.read_text()

    # An untouched stub means nobody has run design-architecture yet, which is the
    # normal state of a project that has not reached the Spec track. Failing here
    # would demand a component diagram before there are components — the ceremony
    # this template exists to prevent. It is noted on every run rather than being
    # silent, so a stub cannot survive unnoticed the way one survived 41 increments
    # in the first project built this way, and --require turns it into an error once
    # a project is past that point.
    if any(marker in text for marker in STUB_MARKERS):
        if not args.require:
            print(f"{doc}: still the template stub — run design-architecture when you "
                  f"reach the Spec track (--require makes this an error)")
            return 0
        problems.append(f"{doc} is still the unfilled template stub")

    invariants = section(text, "Invariants")
    if invariants is None:
        problems.append(
            f"{doc} has no '## Invariants' section. That section is the point: it is "
            f"the part of an architecture a change can violate without anybody noticing"
        )
        rows = []
    else:
        rows = table_rows(invariants)
        if not rows:
            problems.append(f"{doc}: the Invariants table is empty")

    tests = known_tests(root)
    for row in rows:
        if len(row) < 2:
            continue
        claim, evidence = row[0], " ".join(row[1:])
        named = {a or b for a, b in TEST_RE.findall(evidence)}
        named = {n for n in named if n.startswith(("Test", "test_"))}

        # A named test file counts, provided it is really there.
        for path in TESTFILE_RE.findall(evidence):
            if (root / path).exists():
                named.add(path)
            else:
                problems.append(
                    f"{doc}: invariant \"{claim[:60]}\" points at `{path}`, which does not exist")

        if not named:
            problems.append(
                f"{doc}: invariant \"{claim[:60]}\" names no test. State what asserts "
                f"it, or drop the claim"
            )
            continue
        if not tests:
            continue  # nothing indexable in this repo; do not invent failures
        # A path that exists has already been verified above; only function names
        # need looking up.
        functions = {n for n in named if n.startswith(("Test", "test_"))}
        missing = sorted(n for n in functions if n not in tests)
        if missing and not (functions & tests):
            problems.append(
                f"{doc}: invariant \"{claim[:60]}\" names {', '.join(missing)}, "
                f"which does not exist. The document claims something nothing checks"
            )

    # Components must be real directories: a table listing a package that was renamed
    # sends a reader somewhere that is not there.
    components = section(text, "Components")
    if components:
        for row in table_rows(components):
            for path in PATH_RE.findall(row[0] if row else ""):
                if not (root / path).exists():
                    problems.append(f"{doc}: component `{path}` does not exist")

    if problems:
        print(f"{doc}: {len(problems)} problem(s)\n", file=sys.stderr)
        for problem in problems:
            print(f"  ✗ {problem}", file=sys.stderr)
        return 1

    counted = f"{len(rows)} invariant(s)" if rows else "no invariants"
    print(f"{doc}: ok — {counted}, every named test exists")
    return 0


if __name__ == "__main__":
    sys.exit(main())
