# Repository Root Boundary

## Scenario: Maintain the standalone product root

### 1. Scope / trigger

This contract applies when changing repository layout, GitHub automation,
dependency update configuration, release images, root documentation, or any
cleanup that could reach product or runtime paths.

The Git root is a thin metadata shell. `mm-chat/` is the only product root.
Product source, package manifests, Dockerfiles, Compose files, operational
scripts, and product documentation must not be recreated at the Git root.

### 2. Signatures

Required entrypoints:

```bash
bash mm-chat/scripts/verify-standalone.sh
bash mm-chat/scripts/verify-standalone.sh --full

docker compose --project-directory mm-chat \
  --env-file mm-chat/.env.single-server.example \
  -f mm-chat/compose.yml config --quiet
```

Component roots are `mm-chat/frontend`, `mm-chat/backend`, `mm-chat/rag`, and
`mm-chat/postgres`. CI and release contexts must name those paths explicitly.

### 3. Contracts

- Retained Git-root entrypoints are `.github/`, `.gitignore`, `.env.example`,
  `AGENTS.md`, community files, repository READMEs, and Trellis/editor metadata.
- `.env.example` points to `mm-chat/.env.single-server.example`; it is not a
  second configuration authority.
- Root documentation sends all build, test, deployment, backup, and recovery
  commands into `mm-chat/`.
- Generated frontend output such as `frontend/.next/`,
  `frontend/.open-next/`, and `frontend/node_modules/` is not standalone source
  and must be excluded before the isolated copy is inspected for symlinks or
  absolute paths. Source symlinks remain forbidden.
- `mm-chat/data/`, `mm-chat/secrets/`, `mm-chat/backup/`, and
  `mm-chat/.env.single-server` are runtime state. Source cleanup must not delete,
  rewrite, archive into Git, or relocate them.
- A destructive cleanup requires an external working-copy archive, SHA-256,
  archive manifest, Git state, restore instructions, and successful temporary
  PostgreSQL/MinIO restore drills before deletion.
- Deletion uses a reviewed fixed allowlist and verifies path parents before
  removing anything. Broad repository-root deletion is forbidden.

### 4. Validation and error matrix

| Condition                                                                                   | Required result                                                         |
| ------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Backup checksum or archive listing fails                                                    | Stop before deletion.                                                   |
| Restore drill cannot recreate the temporary database or bucket                              | Stop before deletion.                                                   |
| Candidate path is outside the fixed allowlist                                               | Reject it.                                                              |
| Candidate resolves beneath `.git`, `.agents`, `.codex`, `.trellis`, `.vscode`, or `mm-chat` | Reject it.                                                              |
| `git status --porcelain -- mm-chat` changes during root cleanup                             | Stop and inspect before commit.                                         |
| A preceding Next/OpenNext build leaves generated symlinks                                   | Exclude the generated output tree; do not weaken source symlink checks. |
| Standalone, component, Compose, or live health gate fails                                   | Do not publish the cleanup.                                             |

### 5. Good / base / bad cases

- **Good**: archive exact dirty root paths externally, verify both runtime
  restore paths, delete only the allowlist, retarget automation, and pass clean
  copy plus live smoke tests.
- **Base**: change only root README or GitHub metadata, but still run structure,
  formatting, workflow syntax, and Compose-render checks.
- **Bad**: run recursive deletion against the repository root, move `mm-chat/`
  without migrating bind mounts, or use a Git tag as the only backup for dirty
  working-tree content.

### 6. Tests required

- Assert every deletion candidate exists in the external archive manifest.
- Assert no allowlisted former-root path remains after cleanup.
- Assert protected paths remain present and `mm-chat/` tracked state is clean.
- Run `verify-standalone.sh --full`, frontend format/lint/typecheck/test/build,
  backend vet/test, and RAG Ruff/mypy/pytest.
- Run the structural gate after both an ordinary Next build and an OpenNext
  Worker build when their copy boundaries change.
- Render Compose with example and active env files.
- Validate workflow YAML/action expressions.
- Require HTTP 200 from the frontend, backend readiness, same-origin health,
  and private RAG health endpoints when a live stack exists.

### 7. Wrong vs correct

#### Wrong

```bash
rm -rf src public docs node_modules mm-chat
git tag pre-cleanup
```

The tag cannot recover uncommitted content, and the command crosses the product
boundary.

#### Correct

```text
verified external archive + runtime backup pair + restore drills
  -> fixed lexical allowlist deletion
  -> thin root entrypoints targeting mm-chat
  -> clean-copy/component/Compose/live verification
  -> reviewed commit
```
