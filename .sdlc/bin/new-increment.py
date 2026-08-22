#!/usr/bin/env python3
"""Append a new increment to the ledger and create its detail file.

Scaffolding this rather than hand-writing it keeps ids sequential and the format
identical every time, which is what makes validate-plan.py meaningful.
"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys

PLAN = pathlib.Path("specs/increment-plan.md")
DETAIL_DIR = pathlib.Path("specs/increments")
TEMPLATE = pathlib.Path(".sdlc/templates/increment.md")

PREFIXES = {
    "feature": "feat", "fix": "fix", "security": "sec",
    "perf": "perf", "refactor": "ref", "chore": "chore", "docs": "docs",
}


def slugify(title: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", title.lower()).strip("-")
    return slug[:48].rstrip("-")


def next_id(text: str, prefix: str) -> str:
    used = [int(m) for m in re.findall(rf"^##\s+{re.escape(prefix)}-(\d{{3}})\s*:", text, re.M)]
    return f"{prefix}-{max(used, default=0) + 1:03d}"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("title", help="imperative title, e.g. 'Parameterize the search query'")
    parser.add_argument("--type", required=True, choices=sorted(PREFIXES), dest="inc_type")
    parser.add_argument("--tier", default="3", choices=["1", "2", "3", "4"])
    parser.add_argument("--depends-on", default="none", help="comma-separated increment ids")
    args = parser.parse_args()

    if not PLAN.exists():
        print(f"error: {PLAN} not found — run from the repository root", file=sys.stderr)
        return 1

    text = PLAN.read_text()
    inc_id = next_id(text, PREFIXES[args.inc_type])
    tier_label = {"1": "Critical", "2": "High", "3": "Medium", "4": "Low"}[args.tier]

    block = TEMPLATE.read_text()
    block = block.replace("## <id>: <Imperative title>", f"## {inc_id}: {args.title}")
    block = re.sub(r"^- \*\*Type:\*\* .*$", f"- **Type:** {args.inc_type}", block, flags=re.M)
    block = re.sub(r"^- \*\*Status:\*\* .*$", "- **Status:** planned", block, flags=re.M)
    block = re.sub(r"^- \*\*Tier:\*\* .*$", f"- **Tier:** {args.tier} ({tier_label})", block, flags=re.M)
    block = re.sub(r"^- \*\*Dependencies:\*\* .*$", f"- **Dependencies:** {args.depends_on}", block, flags=re.M)

    PLAN.write_text(text.rstrip() + "\n\n" + block.strip() + "\n")

    DETAIL_DIR.mkdir(parents=True, exist_ok=True)
    detail = DETAIL_DIR / f"{inc_id}-{slugify(args.title)}.md"
    if not detail.exists():
        detail.write_text(
            f"# {inc_id}: {args.title}\n\n"
            "## Context\n\n_Why this increment exists. Link the finding or requirement it comes from._\n\n"
            "## Design notes\n\n_Decisions taken while implementing, and what was rejected._\n\n"
            "## Evidence\n\n_Test output, PR link, commit sha. Filled in at verify._\n"
        )

    print(f"created {inc_id}")
    print(f"  ledger: {PLAN}")
    print(f"  detail: {detail}")
    print("\nFill in Scope, Acceptance Criteria, Test Strategy, Rollback Plan and Risk, then run:")
    print("  python3 .sdlc/bin/validate-plan.py")
    return 0


if __name__ == "__main__":
    sys.exit(main())
