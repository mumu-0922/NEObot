# Review and Track Shared Trellis Scaffold - Result

## Outcome

The partial Trellis installation is converted into a reviewed shared scaffold:
the exact 80-file manifest reproduces the Codex/Trellis workflow from a clean
copy, while developer identity, runtime sessions, update metadata, caches,
backups, worktrees, Python caches, and unrelated editor state remain outside
the tracked boundary.

## Inventory and Changes

- Classified `.agents/`, `.codex/`, Trellis scripts/config/workflow/version,
  shared specs, and the workspace index as repository-shared.
- Kept `.trellis/.developer`, `.current-task`, `.runtime/`,
  `.template-hashes.json`, `.cache/`, `worktrees/`, backups, temporary files,
  and Python caches local-only.
- Added the missing local-state ignore rules for template update metadata,
  cache data, and task worktrees.
- Preserved `.vscode/settings.json` unchanged and unstaged; its pre-task diff is
  line-ending-only.
- Fixed two stale paths in `.trellis/workflow.md` to name the actual local
  Codex hook, active-task resolver, task store, and status-writer entrypoint.
- Indexed the generated code-reuse guide.
- Added the executable Trellis scaffold boundary contract and linked it from
  the operations index.
- Recorded the reviewed staging set in `research/scaffold-manifest.txt`: 80
  shared paths, producing a 74-path Git delta (72 additions and two spec-index
  modifications); six manifest paths are already tracked and unchanged.

## Verification

- Trellis 0.5.19 template audit before edits: 72/72 recorded paths present;
  67 baseline matches and five intentional local modifications; zero missing.
- Redacted credential and personal absolute-path scan: zero unresolved hits.
- Candidate symlink and generated-cache scan: zero leaked paths.
- Python 3.13 compile: all `.trellis/scripts/**/*.py` and
  `.codex/hooks/*.py` passed.
- `test_add_session.py`: three regression tests passed, including exact
  journal/index commits and preservation of unrelated staged state.
- Task/context smoke: `task.py current`, `task.py validate`, package discovery,
  and the Codex inline Phase 2.1 loader passed.
- Configuration parse: JSON, TOML, and Trellis YAML passed.
- Hook smoke: registered `UserPromptSubmit` hook and dormant compatibility
  `SessionStart` hook both accepted stdin and returned valid event JSON.
- Clean-copy test: a temporary Git index built only from the 80-path manifest
  compiled and ran task/context/hook commands without `.developer`, runtime,
  template hashes, cache, or worktree state.
- Authored/modified spec and task Markdown passed the repository's local
  Prettier binary.
- `git status --porcelain -- mm-chat` remained empty.

The product-wide standalone gate was not run because no file under `mm-chat/`
changed; the task-specific Python, configuration, hook, Git-index, and
fresh-copy gates exercise the affected surface directly.

## Compatibility and Risk

- `.codex/hooks/session-start.py` remains unregistered in `hooks.json`; this is
  deliberate compatibility retention, not an active hook path.
- `.trellis/.template-hashes.json` remains available locally for
  `trellis update` but is ignored and absent from the shared manifest. Its
  contents were not modified.
- Existing project customizations in the three Codex agent TOMLs,
  `add_session.py`, and `common/safe_commit.py` are preserved verbatim.
- Remaining dirty state after the scaffold work should be only the active task
  before archival plus the excluded `.vscode/settings.json` line-ending diff.

## Rollback

Revert only the scaffold work commit. Do not recursively remove `.trellis/` or
force-add it: ignored developer/runtime/update/cache/worktree state may coexist
with tracked scaffold files and must survive both rollback and future updates.
