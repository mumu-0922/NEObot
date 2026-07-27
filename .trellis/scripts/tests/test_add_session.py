from __future__ import annotations

import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


SCRIPTS_DIR = Path(__file__).resolve().parents[1]
if str(SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPTS_DIR))

import add_session as add_session_module


class AddSessionGitIsolationTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp_dir.cleanup)
        self.repo = Path(self.temp_dir.name)

        self.git("init", "-q")
        self.git("config", "user.name", "Trellis Test")
        self.git("config", "user.email", "trellis-test@example.invalid")

        trellis_dir = self.repo / ".trellis"
        self.workspace_dir = trellis_dir / "workspace" / "tester"
        self.archive_file = trellis_dir / "tasks" / "archive" / "old" / "result.md"
        self.journal_file = self.workspace_dir / "journal-1.md"
        self.index_file = self.workspace_dir / "index.md"
        self.config_file = trellis_dir / "config.yaml"
        self.unrelated_file = self.repo / "unrelated-staged.txt"

        self.workspace_dir.mkdir(parents=True)
        self.archive_file.parent.mkdir(parents=True)
        (trellis_dir / ".developer").write_text("name=tester\n", encoding="utf-8")
        self.config_file.write_text(
            'session_commit_message: "chore: record journal"\n'
            "max_journal_lines: 2000\n"
            "session_auto_commit: true\n",
            encoding="utf-8",
        )
        self.journal_file.write_text(
            "# Journal - tester (Part 1)\n\n> Started: 2026-07-27\n\n---\n",
            encoding="utf-8",
        )
        self.index_file.write_text(
            """# Workspace Index - tester

<!-- @@@auto:current-status -->
- **Active File**: `journal-1.md`
- **Total Sessions**: 0
- **Last Active**: -
<!-- @@@/auto:current-status -->

<!-- @@@auto:active-documents -->
| File | Lines | Status |
|------|-------|--------|
| `journal-1.md` | ~5 | Active |
<!-- @@@/auto:active-documents -->

<!-- @@@auto:session-history -->
| # | Date | Title | Commits | Branch |
|---|------|-------|---------|--------|
<!-- @@@/auto:session-history -->
""",
            encoding="utf-8",
        )
        self.archive_file.write_text("baseline\n", encoding="utf-8")
        self.unrelated_file.write_text("baseline\n", encoding="utf-8")

        self.git("add", "--", ".trellis", "unrelated-staged.txt")
        self.git("commit", "-q", "-m", "test: baseline")

    def git(self, *args: str) -> str:
        result = subprocess.run(
            ["git", *args],
            cwd=self.repo,
            check=True,
            capture_output=True,
            text=True,
            encoding="utf-8",
        )
        return result.stdout

    def committed_paths(self) -> set[str]:
        changed_paths = self.git(
            "show", "--format=", "--name-only", "HEAD"
        ).splitlines()
        return {
            line
            for line in changed_paths
            if line
        }

    def run_add_session(self, title: str = "Test session") -> int:
        with mock.patch.object(
            add_session_module, "get_repo_root", return_value=self.repo
        ):
            return add_session_module.add_session(
                title=title,
                summary="Regression coverage",
                extra_content="Verified isolated journal commit.",
            )

    def test_normal_append_commits_only_journal_and_index(self) -> None:
        self.unrelated_file.write_text("developer staged work\n", encoding="utf-8")
        self.git("add", "--", "unrelated-staged.txt")
        self.archive_file.write_text("dirty archive work\n", encoding="utf-8")
        active_task_file = (
            self.repo / ".trellis" / "tasks" / "active-task" / "prd.md"
        )
        active_task_file.parent.mkdir(parents=True)
        active_task_file.write_text("untracked task\n", encoding="utf-8")

        self.assertEqual(self.run_add_session(), 0)

        self.assertEqual(
            self.committed_paths(),
            {
                ".trellis/workspace/tester/index.md",
                ".trellis/workspace/tester/journal-1.md",
            },
        )
        self.assertEqual(
            self.git("log", "-1", "--pretty=%s").strip(),
            "chore: record journal",
        )
        self.assertEqual(
            self.git("diff", "--cached", "--name-only").splitlines(),
            ["unrelated-staged.txt"],
        )
        self.assertIn(
            ".trellis/tasks/archive/old/result.md",
            self.git("diff", "--name-only").splitlines(),
        )
        self.assertIn(
            "?? .trellis/tasks/active-task/",
            self.git("status", "--porcelain").splitlines(),
        )

    def test_rotation_commits_new_journal_and_index_only(self) -> None:
        self.config_file.write_text(
            'session_commit_message: "chore: record journal"\n'
            "max_journal_lines: 1\n"
            "session_auto_commit: true\n",
            encoding="utf-8",
        )
        self.git("add", "--", ".trellis/config.yaml")
        self.git("commit", "-q", "-m", "test: force journal rotation")

        self.assertEqual(self.run_add_session("Rotated session"), 0)

        self.assertTrue((self.workspace_dir / "journal-2.md").is_file())
        self.assertEqual(
            self.committed_paths(),
            {
                ".trellis/workspace/tester/index.md",
                ".trellis/workspace/tester/journal-2.md",
            },
        )

    def test_disabled_auto_commit_writes_files_without_touching_git(self) -> None:
        self.config_file.write_text(
            'session_commit_message: "chore: record journal"\n'
            "max_journal_lines: 2000\n"
            "session_auto_commit: false\n",
            encoding="utf-8",
        )
        head_before = self.git("rev-parse", "HEAD").strip()
        journal_before = self.journal_file.read_text(encoding="utf-8")

        with (
            mock.patch.object(
                add_session_module, "get_repo_root", return_value=self.repo
            ),
            mock.patch.object(
                add_session_module,
                "safe_git_add",
                side_effect=AssertionError("safe_git_add must not run"),
            ),
            mock.patch.object(
                add_session_module,
                "run_git",
                side_effect=AssertionError("run_git must not run"),
            ),
        ):
            result = add_session_module.add_session(
                title="No automatic commit",
                summary="Git must remain untouched",
                extra_content="Journal data is still written.",
            )

        self.assertEqual(result, 0)
        self.assertNotEqual(
            self.journal_file.read_text(encoding="utf-8"),
            journal_before,
        )
        self.assertEqual(self.git("rev-parse", "HEAD").strip(), head_before)
        self.assertEqual(self.git("diff", "--cached", "--name-only"), "")
        self.assertEqual(
            set(self.git("diff", "--name-only").splitlines()),
            {
                ".trellis/config.yaml",
                ".trellis/workspace/tester/index.md",
                ".trellis/workspace/tester/journal-1.md",
            },
        )


if __name__ == "__main__":
    unittest.main()
