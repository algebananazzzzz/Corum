#!/usr/bin/env python3
"""Check course-page source markers; semantic coverage still needs review."""

import argparse
from pathlib import Path
import re


def check(path):
    errors = []
    count = 0
    fence = None
    for number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        fence_match = re.match(r"^\s*(?:>\s*)*(`{3,}|~{3,})", line)
        if fence_match:
            marker = fence_match.group(1)
            if fence is None:
                fence = marker
            elif marker[0] == fence[0] and len(marker) >= len(fence):
                fence = None
            continue
        if fence or "%%" not in line:
            continue
        count += 1
        match = re.fullmatch(r"\s*%%\s+([^%]+?)\s+p(\d+)(?:-(\d+))?\s+%%\s*", line)
        if not match:
            errors.append(f"{path}:{number}: expected %% LABEL p6-7 %% or %% LABEL p6 %%")
        elif "," in match[1] or re.search(r"\bp\d", match[1]):
            errors.append(f"{path}:{number}: use a separate marker for each source range")
        elif int(match[2]) < 1 or (match[3] and int(match[3]) < int(match[2])):
            errors.append(f"{path}:{number}: page range must be positive and ascending")
    if not count:
        errors.append(f"{path}: expected at least one course-source marker")
    return count, errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("pages", nargs="+", type=Path)
    args = parser.parse_args()
    failed = False
    for page in args.pages:
        count, errors = check(page)
        if errors:
            failed = True
            print("\n".join(errors))
        else:
            print(f"OK {page}: {count} source markers")
    return int(failed)


if __name__ == "__main__":
    raise SystemExit(main())
