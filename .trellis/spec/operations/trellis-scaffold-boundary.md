# Trellis Scaffold Boundary

## Scenario: Track shared Trellis tooling without committing local state

### 1. Scope / Trigger

Apply this contract after `trellis init`, when adding a platform adapter, or
when a fresh clone is missing scripts imported by tracked Trellis entrypoints.
The goal is a reproducible project workflow, not a snapshot of one developer's
session or machine.

### 2. Signatures

The shared scaffold may include these reviewed categories:

```text
.agents/skills/**
.codex/agents/**
.codex/config.toml
.codex/hooks.json
.codex/hooks/**
.trellis/config.yaml
.trellis/.version
.trellis/workflow.md
.trellis/scripts/**
.trellis/spec/**
.trellis/workspace/index.md
```

Local-only state includes:

```text
.trellis/.developer
.trellis/.current-task
.trellis/.runtime/**
.trellis/.template-hashes.json
.trellis/.cache/**
.trellis/worktrees/**
.trellis/.backup-*/**
**/__pycache__/**
**/*.pyc
```

### 3. Contracts

- Inventory generated files before staging. Classify each path as shared,
  local-only, or unrelated; unclassified paths do not enter the commit.
- Stage a reviewed file manifest with explicit pathspecs. Never use
  `git add .`, `git add .trellis/`, or `git add -f .trellis/` to capture a
  scaffold.
- `.trellis/.template-hashes.json` is local update-management state. Inspect it
  for template provenance when useful, but do not hand-edit or track it.
- `.trellis/.version` is the shared compatibility marker and may be tracked.
- Every tracked Python entrypoint must have its imported local modules in the
  shared manifest; partial script tracking is forbidden.
- Platform settings must reference repository-relative files that exist in the
  same tracked set. A generated compatibility hook may remain unregistered
  when the active platform event model does not use it, but its status must be
  documented during review.
- Scan candidate text for credential shapes and personal absolute paths without
  printing suspected secret values. Reject unresolved hits.
- Preserve unrelated editor preferences and all `mm-chat/` product/runtime
  paths unless the task explicitly expands scope.
- Do not rewrite `.trellis/.template-hashes.json` after local customization;
  hash drift is how `trellis update` detects user-modified templates.

### 4. Validation and Error Matrix

| Condition                                                | Required result                                                        |
| -------------------------------------------------------- | ---------------------------------------------------------------------- |
| Shared entrypoint imports an absent/untracked module     | Add the required shared module or reject the scaffold as incomplete.   |
| Candidate contains a credential or personal path hit     | Stop, redact/classify the hit, and rescan before staging.              |
| Hook/config target is absolute or missing                | Replace it with an existing repository-relative target.                |
| Local runtime/cache/update state appears in the manifest | Remove it and add/fix the narrow ignore rule.                          |
| Generated file is modified from the template hash        | Preserve the local version; do not silently overwrite or rewrite hash. |
| Unrelated dirty path exists                              | Leave it unstaged and report it separately.                            |
| Clean-copy command or hook smoke test fails              | Do not commit; return to the earliest missing dependency/reference.    |

### 5. Good / Base / Bad Cases

- **Good**: audit template provenance, add missing imported scripts, ignore
  local update/cache state, stage an explicit manifest, and pass clean-copy
  command and hook smoke tests.
- **Base**: add one project hook plus its registration and verify the
  repository-relative command from a clean copy.
- **Bad**: force-add `.trellis/`, which can absorb developer identity, runtime
  sessions, worktrees, caches, backups, and unrelated active tasks.

### 6. Tests Required

- Compile all shared `.trellis/scripts/**/*.py` and platform Python hooks.
- Run focused tests for customized scripts plus `task.py current`, `validate`,
  and context-loading smoke commands.
- Parse JSON and TOML configuration; parse YAML when a parser is available.
- Assert every configured hook command resolves inside the repository and
  exists in the reviewed tracked set.
- Check the reviewed candidate tree for secrets, personal absolute paths,
  symlinks, cache artifacts, and broken concrete documentation links.
- Build a temporary Git index or clean copy from the exact manifest and prove
  the Trellis commands import and run without relying on ignored local state.
- Confirm `git status --porcelain -- mm-chat` remains empty.

### 7. Wrong vs Correct

#### Wrong

```bash
git add -f .trellis/
```

This overrides the repository's local-state boundary and can stage update
metadata, developer identity, runtime sessions, caches, backups, and worktrees.

#### Correct

```bash
git add --pathspec-from-file=<reviewed-shared-files>
git diff --cached --name-only
```

The first command stages only the audited manifest; the second proves the
resulting index contains no unclassified path before commit.

### 8. Rollback

Revert only the scaffold commit. Do not recursively delete `.trellis/`: ignored
developer identity, runtime sessions, update metadata, caches, and worktrees
may coexist with tracked files and must survive rollback.
