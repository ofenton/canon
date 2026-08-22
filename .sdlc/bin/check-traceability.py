#!/usr/bin/env python3
"""Check that the ledger still matches reality.

Spec drift — the plan says one thing, the repository says another — is the most
commonly reported failure of spec-driven workflows, and the one nothing else here
catches. This checks the cheap, mechanical half of it:

  * work claimed as done has commits carrying its trailer
  * requirements marked Must are covered by at least one increment
  * increments trace to requirements and findings that actually exist

It cannot tell you the code is correct. It can tell you the paperwork is fiction.
"""

from __future__ import annotations

import argparse
import pathlib
import re
import subprocess
import sys

HEADING_RE = re.compile(r"^##\s+([a-z]{2,6}-\d{3})\s*:\s*(.+?)\s*$")
FIELD_RE = re.compile(r"^-\s+\*\*(?P<key>[A-Za-z ]+):\*\*\s*(?P<value>.*)$")
REQ_RE = re.compile(r"^-\s+\*\*(R\d+):\*\*(?P<body>.*)$", re.M)
PLACEHOLDER = re.compile(r"<[^>]+>")
FINDING_RE = re.compile(r"^###\s+([A-Z]{2,8}-\d{3})\s*:", re.M)
TRACE_ID_RE = re.compile(r"\b(R\d+|[A-Z]{2,8}-\d{3})\b")

SHIPPED = {"in-review", "done"}


def load_increments(path: pathlib.Path) -> list[dict]:
    increments: list[dict] = []
    current: dict | None = None
    for line in path.read_text().splitlines():
        heading = HEADING_RE.match(line)
        if heading:
            current = {"id": heading.group(1), "title": heading.group(2), "fields": {}}
            increments.append(current)
            continue
        if current is None:
            continue
        field = FIELD_RE.match(line)
        if field:
            current["fields"][field.group("key").strip()] = field.group("value").strip()
    return increments


def untracked_commits() -> tuple[int, int] | None:
    """Return (commits with no Increment trailer, total commits), excluding merges.

    Work that legitimately needs no increment is normal — that is the Direct track.
    What is not normal is nobody knowing how much of it there is. Reporting the ratio
    makes the aggregate visible without forcing a junk increment for every typo, which
    is the pressure that produces NOJIRA-style placeholder references in the first place.
    """
    try:
        out = subprocess.run(
            ["git", "log", "--no-merges", "--format=%H%x1f%B%x1e"],
            capture_output=True, text=True, check=True, timeout=30,
        ).stdout
    except (subprocess.CalledProcessError, FileNotFoundError, subprocess.TimeoutExpired, OSError):
        return None

    total = untracked = 0
    for record in out.split("\x1e"):
        if not record.strip():
            continue
        _, _, message = record.partition("\x1f")
        total += 1
        if not re.search(r"^Increment:\s*[a-z]{2,6}-\d{3}\s*$", message, re.M):
            untracked += 1
    return untracked, total


def git_trailers() -> dict[str, int] | None:
    """Count commits per increment id. None when git history cannot be read at all."""
    try:
        head = subprocess.run(
            ["git", "rev-parse", "--verify", "HEAD"],
            capture_output=True, text=True, timeout=10,
        )
        if head.returncode != 0:
            # A repository before its first commit has no trailers, which is a fact
            # rather than a failure — an increment claimed done here is genuinely wrong.
            return {}
        out = subprocess.run(
            ["git", "log", "--format=%B%x00"],
            capture_output=True, text=True, check=True, timeout=30,
        ).stdout
    except (subprocess.CalledProcessError, FileNotFoundError, subprocess.TimeoutExpired, OSError):
        return None

    counts: dict[str, int] = {}
    for message in out.split("\0"):
        for match in re.finditer(r"^Increment:\s*([a-z]{2,6}-\d{3})\s*$", message, re.M):
            counts[match.group(1)] = counts.get(match.group(1), 0) + 1
    return counts


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--plan", default="specs/increment-plan.md")
    parser.add_argument("--spec", default="specs/product.md")
    parser.add_argument("--assessments", default="specs/assessments")
    parser.add_argument(
        "--max-untracked-pct", type=float, default=None,
        help="fail if more than this percentage of commits carry no Increment trailer "
             "(default: report the ratio without failing)",
    )
    args = parser.parse_args()

    plan_path = pathlib.Path(args.plan)
    if not plan_path.exists():
        print(f"error: {plan_path} not found — run from the repository root", file=sys.stderr)
        return 1

    increments = load_increments(plan_path)
    problems: list[str] = []
    notes: list[str] = []

    # Known ids that an increment is allowed to trace to.
    known: set[str] = set()
    spec_path = pathlib.Path(args.spec)
    must_requirements: set[str] = set()
    if spec_path.exists():
        spec_text = spec_path.read_text()
        known.update(m.group(1) for m in REQ_RE.finditer(spec_text))
        # A draft spec is a proposal. Its requirements only become commitments to trace
        # once a human marks the spec agreed — demanding coverage before then would force
        # planning to happen before the spec is settled, which is backwards.
        spec_status = ""
        status_match = re.search(r"^\*\*Status:\*\*\s*(.+?)\s*$", spec_text, re.M)
        if status_match:
            spec_status = status_match.group(1).strip().lower()
        if spec_status == "agreed":
            must_section = re.search(r"^###\s+Must\s*$(.*?)(?=^###\s|\Z)", spec_text, re.M | re.S)
            if must_section:
                # An unfilled template requirement is not yet a commitment to trace.
                must_requirements.update(
                    m.group(1) for m in REQ_RE.finditer(must_section.group(1))
                    if not PLACEHOLDER.search(m.group("body"))
                )
        elif spec_status:
            notes.append(f"{spec_path} is '{spec_status}', not 'agreed' — skipped requirement coverage")
    assessments = pathlib.Path(args.assessments)
    if assessments.is_dir():
        for report in assessments.glob("*.md"):
            known.update(FINDING_RE.findall(report.read_text()))

    traced: set[str] = set()
    for inc in increments:
        status = inc["fields"].get("Status", "").lower()
        if status == "abandoned":
            continue
        raw_trace = inc["fields"].get("Traces", "").strip()
        ids = set(TRACE_ID_RE.findall(raw_trace))
        traced.update(ids)
        for trace_id in sorted(ids):
            if known and trace_id not in known:
                problems.append(
                    f"{inc['id']}: traces to '{trace_id}', which is not a requirement in "
                    f"{spec_path} or a finding in {assessments}/"
                )

    for req in sorted(must_requirements - traced):
        problems.append(f"{req} is a Must requirement but no increment traces to it")

    counts = git_trailers()
    if counts is None:
        notes.append("git history unavailable — skipped the commit-trailer check")
    else:
        for inc in increments:
            status = inc["fields"].get("Status", "").lower()
            if status in SHIPPED and not counts.get(inc["id"]):
                problems.append(
                    f"{inc['id']}: status is {status} but no commit carries "
                    f"'Increment: {inc['id']}' — the work is claimed, not recorded"
                )
        orphans = sorted(set(counts) - {i["id"] for i in increments})
        for orphan in orphans:
            problems.append(
                f"commits carry 'Increment: {orphan}' but that increment is not in the ledger"
            )

        tally = untracked_commits()
        if tally is not None and tally[1]:
            untracked, total = tally
            pct = 100.0 * untracked / total
            summary = f"{untracked} of {total} commits ({pct:.0f}%) carry no Increment trailer"
            if args.max_untracked_pct is not None and pct > args.max_untracked_pct:
                problems.append(f"{summary} — above the {args.max_untracked_pct:.0f}% limit")
            else:
                notes.append(summary)

    for note in notes:
        print(f"  note: {note}")

    if problems:
        print(f"\n{plan_path}: {len(problems)} traceability problem(s)\n", file=sys.stderr)
        for problem in problems:
            print(f"  ✗ {problem}", file=sys.stderr)
        return 1

    print(f"{plan_path}: ok — {len(increments)} increments trace cleanly")
    return 0


if __name__ == "__main__":
    sys.exit(main())
