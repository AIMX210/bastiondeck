#!/usr/bin/env python3
"""Count source lines of code for BastionDeck.

Scope (reported separately, never merged to hide a shortfall):
  - Go:       all .go under repo root and the nested agent module (tests incl.)
  - Web:      .ts/.tsx/.css under web/src (generated dist excluded)
  - SQL:      migrations
  - Docs:     markdown under docs/
Vendor, node_modules and build output are excluded.
"""
from __future__ import annotations
import os
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

BUCKETS = {
    "Go (server+cli+tui)": (".", [".go"], ["agent", "web", "bin"]),
    "Go (bd-agent module)": ("agent", [".go"], []),
    "Web TS/TSX/CSS": ("web/src", [".ts", ".tsx", ".css"], []),
    "SQL migrations": ("internal/migrations", [".sql"], []),
    "Docs": ("docs", [".md"], []),
}

SKIP_DIRS = {"node_modules", "dist", ".git", "vendor"}


def count(base: str, exts: list[str], extra_skip_roots: list[str]) -> tuple[int, int]:
    files = lines = 0
    skip_abs = [os.path.join(ROOT, p) for p in extra_skip_roots]
    for dirpath, dirnames, filenames in os.walk(os.path.join(ROOT, base)):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS]
        if any(os.path.abspath(dirpath).startswith(s) for s in skip_abs):
            continue
        for fn in filenames:
            if os.path.splitext(fn)[1] not in exts:
                continue
            files += 1
            with open(os.path.join(dirpath, fn), "r", encoding="utf-8", errors="replace") as fh:
                lines += sum(1 for _ in fh)
    return files, lines


def main() -> int:
    grand_files = grand_lines = 0
    source_lines = 0
    print(f"{'bucket':<24}{'files':>8}{'lines':>10}")
    print("-" * 42)
    for name, (base, exts, skip) in BUCKETS.items():
        f, l = count(base, exts, skip)
        grand_files += f
        grand_lines += l
        if not name.startswith("Docs"):
            source_lines += l
        print(f"{name:<24}{f:>8}{l:>10}")
    print("-" * 42)
    print(f"{'TOTAL (all)':<24}{grand_files:>8}{grand_lines:>10}")
    print(f"{'SOURCE (excl docs)':<24}{'':>8}{source_lines:>10}")
    if "--check" in sys.argv:
        target = 20_000
        ok = source_lines >= target
        print(f"\n[{'PASS' if ok else 'FAIL'}] source {source_lines} >= {target}")
        return 0 if ok else 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
