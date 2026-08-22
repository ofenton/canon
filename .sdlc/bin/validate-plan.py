#!/usr/bin/env python3
"""Validate specs/increment-plan.md.

The ledger is the single source of truth for work state, so it is worth checking
mechanically rather than trusting an agent to have kept it consistent. Exits 0 when
the ledger is well formed, 1 otherwise, printing one line per problem.
"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys

ID_RE = re.compile(r"^[a-z]{2,6}-\d{3}$")
HEADING_RE = re.compile(r"^##\s+([a-z]{2,6}-\d{3})\s*:\s*(.+?)\s*$")
FIELD_RE = re.compile(r"^-\s+\*\*(?P<key>[A-Za-z ]+):\*\*\s*(?P<value>.*)$")
CRITERION_RE = re.compile(r"^\s+-\s+\[(?P<mark>[ xX])\]\s+(?P<text>.+)$")

STATUSES = ["planned", "approved", "in-progress", "in-review", "done", "abandoned"]
TERMINAL = {"done", "abandoned"}
TYPES = {"feature", "fix", "security", "perf", "refactor", "chore", "docs"}
REQUIRED = [
    "Type", "Status", "Tier", "Scope",
    "Acceptance Criteria", "Test Strategy",
    "Dependencies", "Rollback Plan", "Risk",
]
# Evidence is only required once an increment leaves implementation.
EVIDENCE_REQUIRED_FROM = {"in-review", "done"}
UNFILLED = re.compile(r"^\s*(_?\(?filled in at verify\)?_?|tbd|todo|n/?a)?\s*$", re.I)
# Angle brackets and unresolved "a | b | c" choices mean the template was copied but not filled.
PLACEHOLDER = re.compile(r"<[^>]+>")
UNCHOSEN = re.compile(r"^[^|]{1,40}(\s*\|\s*[^|]{1,40}){2,}$")


class Increment:
    def __init__(self, inc_id: str, title: str, line: int):
        self.id = inc_id
        self.title = title
        self.line = line
        self.fields: dict[str, str] = {}
        self.criteria: list[tuple[bool, str]] = []
        self.detail_lines: list[str] = []

    @property
    def status(self) -> str:
        return self.fields.get("Status", "").strip().lower()

    @property
    def deps(self) -> list[str]:
        raw = self.fields.get("Dependencies", "").strip().lower()
        if raw in ("", "none", "-"):
            return []
        return [d.strip() for d in raw.split(",") if d.strip()]


def parse(path: pathlib.Path) -> tuple[list[Increment], list[str]]:
    errors: list[str] = []
    increments: list[Increment] = []
    current: Increment | None = None
    current_field: str | None = None

    in_comment = False
    for lineno, raw in enumerate(path.read_text().splitlines(), start=1):
        # Commented-out optional fields are hints, not content.
        if in_comment:
            in_comment = "-->" not in raw
            continue
        if "<!--" in raw and "-->" not in raw:
            in_comment = True
            continue
        if "<!--" in raw and "-->" in raw:
            continue

        heading = HEADING_RE.match(raw)
        if heading:
            current = Increment(heading.group(1), heading.group(2), lineno)
            increments.append(current)
            current_field = None
            continue
        if raw.startswith("## ") and current is not None:
            # A heading that did not parse as an increment ends the previous one.
            current = None
            current_field = None
            continue
        if current is None:
            continue

        field = FIELD_RE.match(raw)
        if field:
            current_field = field.group("key").strip()
            current.fields[current_field] = field.group("value").strip()
            continue

        criterion = CRITERION_RE.match(raw)
        if criterion and current_field == "Acceptance Criteria":
            current.criteria.append(
                (criterion.group("mark").lower() == "x", criterion.group("text").strip())
            )
        elif raw.strip().startswith("-") and raw.startswith((" ", "\t")):
            current.detail_lines.append(raw.strip().lstrip("- ").strip())

    if not increments:
        errors.append(f"{path}: no increments found — expected headings like '## abc-001: Title'")
    return increments, errors


def validate(increments: list[Increment]) -> list[str]:
    errors: list[str] = []
    by_id: dict[str, Increment] = {}

    for inc in increments:
        where = f"{inc.id} (line {inc.line})"

        if inc.id in by_id:
            errors.append(f"{where}: duplicate id, first seen at line {by_id[inc.id].line}")
        by_id[inc.id] = inc

        for key in REQUIRED:
            if key not in inc.fields:
                errors.append(f"{where}: missing required field '{key}'")

        status = inc.status
        if status and status not in STATUSES:
            errors.append(f"{where}: status '{status}' is not one of {', '.join(STATUSES)}")

        inc_type = inc.fields.get("Type", "").strip().lower()
        if inc_type and inc_type not in TYPES:
            errors.append(f"{where}: type '{inc_type}' is not one of {', '.join(sorted(TYPES))}")

        tier = inc.fields.get("Tier", "")
        if tier and not re.match(r"^[1-4]\b", tier.strip()):
            errors.append(f"{where}: tier must start with 1-4, got '{tier}'")

        if "Acceptance Criteria" in inc.fields and not inc.criteria:
            errors.append(f"{where}: acceptance criteria list is empty — nothing to verify against")

        if status == "done":
            unmet = [text for done, text in inc.criteria if not done]
            for text in unmet:
                errors.append(f"{where}: status is done but criterion is unticked — {text}")

        evidence = inc.fields.get("Evidence", "")
        if status in EVIDENCE_REQUIRED_FROM and UNFILLED.match(evidence):
            errors.append(f"{where}: status is {status} but Evidence is empty")

        rollback = inc.fields.get("Rollback Plan", "")
        if status not in TERMINAL and UNFILLED.match(rollback):
            errors.append(f"{where}: Rollback Plan is empty")

        for key, value in inc.fields.items():
            if PLACEHOLDER.search(value) or UNCHOSEN.match(value.strip()):
                errors.append(f"{where}: '{key}' still holds template placeholder text — {value.strip()[:60]}")
        for _, text in inc.criteria:
            if PLACEHOLDER.search(text):
                errors.append(f"{where}: acceptance criterion is template placeholder text — {text[:60]}")
        for text in inc.detail_lines:
            if PLACEHOLDER.search(text):
                errors.append(f"{where}: list item still holds template placeholder text — {text[:60]}")

    in_progress = [i.id for i in increments if i.status == "in-progress"]
    if len(in_progress) > 1:
        errors.append(
            f"WIP limit breached: {len(in_progress)} increments in-progress "
            f"({', '.join(in_progress)}) — only one at a time"
        )

    for inc in increments:
        for dep in inc.deps:
            if not ID_RE.match(dep):
                errors.append(f"{inc.id}: dependency '{dep}' is not a valid increment id")
            elif dep not in by_id:
                errors.append(f"{inc.id}: depends on '{dep}' which is not in the ledger")
            elif dep == inc.id:
                errors.append(f"{inc.id}: depends on itself")

    errors.extend(find_cycles(by_id))

    # An increment cannot be further along than the work it is built on.
    for inc in increments:
        if inc.status not in ("in-progress", "in-review", "done"):
            continue
        for dep in inc.deps:
            target = by_id.get(dep)
            if target and target.status != "done":
                errors.append(
                    f"{inc.id}: status is {inc.status} but dependency {dep} is {target.status or 'unset'}"
                )

    return errors


def find_cycles(by_id: dict[str, Increment]) -> list[str]:
    """Report each dependency cycle once, as the path that closes it."""
    errors: list[str] = []
    seen_cycles: set[frozenset[str]] = set()
    WHITE, GREY, BLACK = 0, 1, 2
    colour = {k: WHITE for k in by_id}

    def visit(node: str, path: list[str]) -> None:
        colour[node] = GREY
        path.append(node)
        for dep in by_id[node].deps:
            if dep not in by_id:
                continue
            if colour[dep] == GREY:
                cycle = path[path.index(dep):] + [dep]
                key = frozenset(cycle)
                if key not in seen_cycles:
                    seen_cycles.add(key)
                    errors.append("dependency cycle: " + " → ".join(cycle))
            elif colour[dep] == WHITE:
                visit(dep, path)
        path.pop()
        colour[node] = BLACK

    for node in by_id:
        if colour[node] == WHITE:
            visit(node, [])
    return errors


def summarise(increments: list[Increment]) -> str:
    counts = {s: 0 for s in STATUSES}
    for inc in increments:
        if inc.status in counts:
            counts[inc.status] += 1
    parts = [f"{counts[s]} {s}" for s in STATUSES if counts[s]]
    return f"{len(increments)} increments: " + (", ".join(parts) or "none with a status")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "plan", nargs="?", default="specs/increment-plan.md",
        help="path to the ledger (default: specs/increment-plan.md)",
    )
    args = parser.parse_args()

    path = pathlib.Path(args.plan)
    if not path.exists():
        print(f"error: {path} not found — run from the repository root", file=sys.stderr)
        return 1

    increments, errors = parse(path)
    errors += validate(increments)

    if errors:
        print(f"{path}: {len(errors)} problem(s)\n", file=sys.stderr)
        for err in errors:
            print(f"  ✗ {err}", file=sys.stderr)
        print(f"\n{summarise(increments)}", file=sys.stderr)
        return 1

    print(f"{path}: ok — {summarise(increments)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
