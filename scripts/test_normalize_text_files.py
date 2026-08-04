#!/usr/bin/env python3
"""Exercise the pre-commit normalizer in an isolated Git repository."""

from __future__ import annotations

import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent


def run(cwd: Path, *args: str) -> bytes:
    return subprocess.run([*args], cwd=cwd, check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE).stdout


class PreCommitNormalizationTest(unittest.TestCase):
    def test_commit_normalizes_index_only(self) -> None:
        scratch_root = ROOT / "tmp"
        scratch_root.mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(prefix="alaa-mcp-hook-", dir=scratch_root) as directory:
            repo = Path(directory)
            (repo / "scripts").mkdir()
            (repo / ".githooks").mkdir()
            shutil.copy2(ROOT / "scripts" / "normalize_text_files.py", repo / "scripts")
            shutil.copy2(ROOT / ".githooks" / "pre-commit", repo / ".githooks")
            run(repo, "git", "init", "-q")
            run(repo, "git", "config", "user.email", "test@example.invalid")
            run(repo, "git", "config", "user.name", "Hook Test")
            run(repo, "git", "config", "core.hooksPath", ".githooks")

            staged = b"\xef\xbb\xbffirst\r\nsecond\rthird\n"
            (repo / "notes.txt").write_bytes(staged)
            (repo / "caf\u00e9 file.md").write_bytes(b"\xef\xbb\xbfhello\r\n")
            binary = b"\x00\xff\r\n"
            (repo / "asset.bin").write_bytes(binary)
            run(repo, "git", "add", "notes.txt", "caf\u00e9 file.md", "asset.bin")

            # This is deliberately not staged. The hook must leave it untouched.
            working_copy = b"local edit\r\n"
            (repo / "notes.txt").write_bytes(working_copy)
            run(repo, "git", "commit", "-qm", "fixture")

            self.assertEqual(run(repo, "git", "show", "HEAD:notes.txt"), b"first\nsecond\nthird\n")
            self.assertEqual(run(repo, "git", "show", "HEAD:caf\u00e9 file.md"), b"hello\n")
            self.assertEqual(run(repo, "git", "show", "HEAD:asset.bin"), binary)
            self.assertEqual((repo / "notes.txt").read_bytes(), working_copy)


if __name__ == "__main__":
    unittest.main()
