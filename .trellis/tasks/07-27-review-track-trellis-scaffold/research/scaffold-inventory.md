# Research: Trellis Scaffold Inventory

- Query: Which Trellis/Codex initialization artifacts are repository-shared,
  which are local-only, and what must change before tracking them?
- Scope: internal
- Date: 2026-07-27

## Findings

### Shared scaffold

The following paths are generated project tooling and contain no detected
credential material or personal absolute path:

- `.agents/skills/**`: 35 Codex/shared Trellis skill and reference files.
- `.codex/config.toml`, `.codex/hooks.json`, `.codex/hooks/**`: project Codex
  defaults and hook adapters. `session-start.py` is a generated compatibility
  adapter but is not registered by the current `hooks.json`; the live Codex
  path is `inject-workflow-state.py` on `UserPromptSubmit`.
- `.trellis/config.yaml`, `.trellis/.version`, `.trellis/workflow.md`: shared
  workflow configuration and version contract.
- `.trellis/scripts/**`: shared CLI/runtime modules. The existing tracked
  `add_session.py` and `common/safe_commit.py` import currently untracked
  modules, so partial tracking does not reproduce from a fresh clone.
- `.trellis/.gitignore`: the local-state exclusion boundary.
- `.trellis/workspace/index.md`: shared workspace instructions.
- `.trellis/spec/guides/code-reuse-thinking-guide.md`: shared guide; it needs
  an entry in `.trellis/spec/guides/index.md` to remain discoverable.

`.trellis/.template-hashes.json` is not shared source. It is Trellis update
management state, and `common/safe_commit.py` explicitly lists it with local
runtime/cache paths that must never be auto-staged. It must remain ignored and
untracked. Its audit still provides provenance: all 72 recorded template paths
exist, 67 match the Trellis 0.5.19 baseline, and five are intentional local
modifications (`.codex/agents/trellis-{research,implement,check}.toml`,
`add_session.py`, and `common/safe_commit.py`).

### Local-only and unrelated state

- Local-only: `.trellis/.developer`, `.trellis/.current-task`,
  `.trellis/.runtime/`, `.trellis/.template-hashes.json`,
  `.trellis/.cache/`, `.trellis/worktrees/`, `.trellis/.backup-*`, temporary
  files, and Python caches.
- Unrelated: `.vscode/settings.json`. Its only diff is LF to CRLF; normalized
  content is unchanged, and the task must preserve it without staging.
- Product/runtime: no path under `mm-chat/` is dirty or in this task's scope.

### Audit evidence

- Secret scanner categories: private-key blocks, GitHub/OpenAI/AWS token
  shapes, JWTs, and non-placeholder credential assignments. Result: zero
  candidate hits.
- Personal-path scanner: POSIX `/home/<user>` and `/Users/<user>` plus Windows
  `C:\\Users\\<user>`. Result: zero candidate hits. Generic Windows path
  conversion examples in `.codex/hooks/session-start.py` are implementation
  patterns, not machine identity.
- Symlink inventory: zero links under candidate roots.
- Template hash inventory: 72 present, zero missing, five locally modified as
  listed above.
- Broken concrete paths in `.trellis/workflow.md`: the final "Full contract"
  section points to two non-existent upstream-style paths. Replace them with
  the repository's actual Codex hook and shared resolver/status-writer paths.
- Ignore gap: `.trellis/.cache/`, `.trellis/worktrees/`, and
  `.trellis/.template-hashes.json` are identified as local-only by the runtime
  but are not currently ignored by `.trellis/.gitignore`.

## Exact Boundary

Track the shared allowlist above plus this task's PRD/research/result and the
operations boundary documentation. Use explicit file pathspecs generated from
the reviewed manifest; do not stage `.trellis/` recursively and never use
`git add .` or `git add -f .trellis/`.

Rollback is a focused revert of the shared scaffold commit. It must not remove
or rewrite ignored runtime state, editor state, or any `mm-chat/` path.

## Caveats / Not Found

- `.codex/hooks/session-start.py` is dormant under the current
  `.codex/hooks.json`, but is a current Trellis 0.5.19 template artifact rather
  than an unexplained local file, so it is retained for compatibility.
- `.trellis/.template-hashes.json` is useful for the local `trellis update`
  installation but is not required by the checked-in task/context commands.
