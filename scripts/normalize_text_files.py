#!/usr/bin/env python3
"""Normalize staged UTF-8 text without touching unstaged working-tree edits."""

from __future__ import annotations

import argparse
import subprocess
import sys
from pathlib import Path


UTF8_BOM = b"\xef\xbb\xbf"


def git(root: Path, *args: str, data: bytes | None = None) -> bytes:
    return subprocess.run(
        ["git", *args], cwd=root, input=data, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True
    ).stdout


def staged_paths(root: Path) -> list[str]:
    names = git(root, "diff", "--cached", "--name-only", "-z", "--diff-filter=ACMR")
    return [item.decode("utf-8", "surrogateescape") for item in names.split(b"\0") if item]


def index_mode(root: Path, path: str) -> str | None:
    entry = git(root, "ls-files", "-s", "-z", "--", path).split(b"\0", 1)[0]
    return entry.split(maxsplit=1)[0].decode("ascii") if entry else None


def normalized(data: bytes) -> bytes | None:
    # Binary and UTF-16 files are intentionally outside this UTF-8 text policy.
    if b"\0" in data:
        return None
    return data.removeprefix(UTF8_BOM).replace(b"\r\n", b"\n").replace(b"\r", b"\n")


def update_index(root: Path, mode: str, path: str, data: bytes) -> None:
    blob = git(root, "hash-object", "-w", "--stdin", data=data).strip().decode("ascii")
    record = f"{mode} {blob}\t{path}".encode("utf-8", "surrogateescape") + b"\0"
    git(root, "update-index", "-z", "--add", "--index-info", data=record)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--staged", action="store_true", help="normalize only staged additions and modifications")
    args = parser.parse_args()
    if not args.staged:
        parser.error("--staged is required")

    root = Path(git(Path.cwd(), "rev-parse", "--show-toplevel").strip().decode("utf-8")).resolve()
    changed = 0
    for path in staged_paths(root):
        mode = index_mode(root, path)
        if mode is None or mode == "120000":
            continue
        data = git(root, "show", f":{path}")
        replacement = normalized(data)
        if replacement is not None and replacement != data:
            update_index(root, mode, path, replacement)
            changed += 1
    if changed:
        print(f"normalized {changed} staged UTF-8 text file(s) to LF without a BOM")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.CalledProcessError as error:
        sys.stderr.buffer.write(error.stderr)
        raise SystemExit(error.returncode)
