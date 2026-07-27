# Session Auto-Commit

## Scenario: Record a developer journal without committing unrelated work

### 1. Scope / Trigger

Apply this contract when changing `.trellis/scripts/add_session.py`, its Git
helpers, journal rotation, `session_auto_commit`, or session-recording tests.
Session recording is a bookkeeping operation; it must never widen into active
tasks, archived tasks, generated Trellis state, or the developer's staged work.

### 2. Signatures

CLI entrypoint:

```bash
python3 ./.trellis/scripts/add_session.py \
  --title "<title>" \
  --commit "<hashes-or-dash>" \
  --summary "<summary>" \
  [--content-file <path> | --stdin] \
  [--branch <branch>] \
  [--no-commit]
```

Internal auto-commit boundary:

```python
def _auto_commit_workspace(
    repo_root: Path,
    journal_file: Path | None,
    index_file: Path,
) -> None: ...
```

Required Git operations when auto-commit is enabled:

```bash
git add -- <target-journal> <workspace-index>
git diff --cached --quiet -- <target-journal> <workspace-index>
git commit --only -m "<configured-message>" -- \
  <target-journal> <workspace-index>
```

### 3. Contracts

- The caller passes the journal file written by this invocation and its
  developer workspace `index.md`. No filesystem discovery of active tasks or
  the archive is allowed.
- A normal append targets the active journal. Rotation targets only the newly
  created `journal-N.md`; older journals are not restaged merely because they
  exist.
- `safe_git_add()` receives repo-relative explicit pathspecs and never retries
  with `-f`.
- `git commit --only` uses the same pathspecs. Files staged before session
  recording remain staged and do not enter the journal commit.
- `session_commit_message` remains the commit-message authority.
- `session_auto_commit: false` returns before any Git stage/diff/commit call;
  journal and index writes still remain on disk.
- If Git rejects an ignored Trellis path, print the established warning and
  skip the commit. Never override the user's ignore policy.
- Task archival is a separate lifecycle and commit boundary.

### 4. Validation and Error Matrix

| Condition                                   | Required result                                                    |
| ------------------------------------------- | ------------------------------------------------------------------ |
| Active task directory is untracked or dirty | Leave it unchanged and outside the journal commit.                 |
| Task archive contains unrelated changes     | Leave them unstaged/uncommitted by session recording.              |
| Unrelated file is already staged            | Exclude it from the journal commit and preserve its staged state.  |
| Journal exceeds `max_journal_lines`         | Create the next journal and commit only it plus `index.md`.        |
| `session_auto_commit` is false              | Write journal/index; execute no Git command.                       |
| Explicit `git add` reports an ignored path  | Warn and skip; do not use force.                                   |
| Journal/index have no staged difference     | Report no workspace changes and do not create an empty commit.     |
| Commit fails                                | Warn with Git's error; do not widen paths or amend another commit. |

### 5. Good / Base / Bad Cases

- **Good**: a pre-staged source file, dirty archive, and untracked active task
  all survive unchanged while a two-path journal commit is created.
- **Base**: append one session to the current journal, update the workspace
  index, and commit exactly those two files.
- **Bad**: discover every journal/task/archive path, run `git add` on the broad
  set, then use plain `git commit -m`, which can also consume unrelated staged
  work.

### 6. Tests Required

Use isolated temporary Git repositories; never run mutation tests against the
developer's actual checkout.

- Assert the normal commit's changed-path set is exactly the active journal and
  workspace index.
- Pre-stage an unrelated tracked file; assert it is absent from the journal
  commit and remains in `git diff --cached --name-only` afterward.
- Dirty an archived task and create an untracked active task; assert both retain
  their original status.
- Force journal rotation; assert the new journal and index are the only commit
  paths.
- Disable `session_auto_commit`; assert journal/index contents change while
  `HEAD` and the Git index do not.
- Keep a source scan or equivalent guard proving the removed broad
  `safe_trellis_paths_to_add` discovery path is not reintroduced.

### 7. Wrong vs Correct

#### Wrong

```python
paths = safe_trellis_paths_to_add(repo_root)  # includes tasks and archive
safe_git_add(paths, repo_root)
run_git(["commit", "-m", commit_msg], cwd=repo_root)
```

The stage set is broader than the write set, and the plain commit also consumes
anything the developer had staged earlier.

#### Correct

```python
paths = [
    journal_file.relative_to(repo_root).as_posix(),
    index_file.relative_to(repo_root).as_posix(),
]
safe_git_add(paths, repo_root)
run_git(
    ["commit", "--only", "-m", commit_msg, "--", *paths],
    cwd=repo_root,
)
```

The written paths, staged paths, diff check, and commit pathspec are identical.
