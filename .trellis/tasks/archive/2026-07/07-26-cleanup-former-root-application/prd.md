# Clean up former root application

## Goal

Make `mm-chat/` the only product application in the repository while retaining
the parent Git/workflow metadata required to develop and operate it. Preserve
an exact recoverable archive of every former-root path that will be deleted or
rewritten before performing any destructive action.

## What I already know

- The live Backend, Frontend, and RAG Worker all use
  `/home/mumu/projects/neo-chat/mm-chat/compose.yml` and build only from
  `mm-chat/`.
- `mm-chat/` is independently buildable and has its own frontend, Go backend,
  Python RAG worker, PostgreSQL image, Compose manifests, docs, scripts, env
  example, data, secrets, and backup paths.
- All 695 current dirty paths are outside `mm-chat/`; none has been pushed as
  working-tree content.
- The owner approved the recommended cleanup and requested the backup under
  the WSL path exposed as `\\wsl.localhost\Ubuntu\home\mumu\projects`, which is
  `/home/mumu/projects` inside WSL.
- `mm-chat/docs/deployment/former-root-delete-plan.md` and
  `mm-chat/scripts/plan-former-root-deletion.sh` define the existing protected
  boundary and destructive gate.

## Requirements

- Create a timestamped backup directory under `/home/mumu/projects` before
  deletion.
- Archive the exact working copies of every top-level former-root path that
  will be deleted or rewritten, including tracked modifications and untracked
  files, while excluding protected paths.
- Store a SHA-256 checksum, archive manifest, Git HEAD, Git status, and recovery
  instructions beside the archive; verify the checksum and archive integrity.
- Never delete or rewrite `.git/`, `.agents/`, `.codex/`, `.trellis/`,
  `.vscode/`, or `mm-chat/` as part of product cleanup.
- Never alter `mm-chat/data/`, `mm-chat/secrets/`, `mm-chat/backup/`, or the
  live `.env.single-server`.
- Run standalone structure/full verification and the non-destructive former-
  root deletion planner before deletion.
- Preserve root repository metadata that GitHub and development tooling require:
  `.github/`, `.gitignore`, `.env.example`, `AGENTS.md`, `LICENSE`, repository
  README files, `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, and
  `CHANGELOG.md`.
- Rewrite retained root entrypoints and GitHub automation to target `mm-chat/`
  rather than the deleted legacy Next.js root application.
- Delete legacy root source, public data, old root docs/scripts, old root build
  manifests/configuration, reproducible caches/build output, obsolete roadmap,
  and the stray `NUL` file according to a fixed allowlist.
- Verify the standalone project, Compose rendering, frontend, backend, RAG
  tests, live container health, and same-origin proxy after deletion.
- Commit and push only the reviewed cleanup; leave unrelated protected Trellis,
  agent, Codex, and editor changes untouched.

## Deletion Allowlist

```text
.dockerignore
.mypy_cache/
.next/
.prettierignore
.ruff_cache/
.wrangler/
Dockerfile
NUL
ROADMAP.md
docker-compose.yml
docs/
eslint.config.mjs
next-env.d.ts
next.config.ts
node_modules/
open-next.config.ts
package.json
pnpm-lock.yaml
pnpm-workspace.yaml
postcss.config.mjs
prettier.config.mjs
public/
scripts/
src/
tailwind.config.ts
tsconfig.json
tsconfig.tsbuildinfo
wrangler.jsonc
```

## Retained / Rewritten Root Paths

```text
.github/
.gitignore
.env.example
AGENTS.md
CHANGELOG.md
CODE_OF_CONDUCT.md
CONTRIBUTING.md
LICENSE
README.md
README.zh-CN.md
SECURITY.md
```

## Acceptance Criteria

- [x] Backup exists under `/home/mumu/projects`, passes SHA-256 verification,
      and `tar` can list the complete archive.
- [x] Backup contains all deletion-allowlist and retained/rewrite paths that
      existed before cleanup, plus Git/status/manifest recovery metadata.
- [x] Protected paths and `mm-chat/` are byte-present after cleanup; runtime
      data/secrets/backups remain untouched.
- [x] No deletion-allowlist path remains at the former root.
- [x] Root README and automation invoke only `mm-chat` build/test/deploy paths.
- [x] `bash mm-chat/scripts/verify-standalone.sh --full` passes after cleanup.
- [x] Frontend format/lint/typecheck/tests/build, backend vet/tests, and RAG
      tests pass from `mm-chat/`.
- [x] Backend, Frontend, and RAG Worker remain healthy and the frontend root,
      backend readiness, and `/mm-api/health` return HTTP 200.
- [ ] The cleanup is committed and pushed without including protected-path
      dirty changes unrelated to this task.

## Definition of Done

- Pre-delete archive and checksum are independently verifiable.
- Deletion is fixed-allowlist and occurs only after pre-delete gates pass.
- GitHub automation and repository entry docs are coherent with `mm-chat/`.
- Full standalone and live smoke gates pass after deletion.
- Rollback instructions and backup path are reported to the owner.

## Technical Approach

1. Snapshot status and create an external timestamped tar archive containing
   all paths in the deletion and rewrite sets.
2. Verify archive checksum/listing, then run the existing standalone/full and
   deletion dry-run gates.
3. Remove only the fixed allowlist with path-boundary checks.
4. Replace retained root README/env/ignore/GitHub automation with thin
   `mm-chat` entrypoints.
5. Run post-delete standalone, language, Compose, and live health gates.
6. Review the exact deletion/rewrite diff, commit, push, then archive this task.

## Decision (ADR-lite)

**Context**: The legacy root Next.js application is no longer in any live
Compose/build path, but repository-level GitHub and agent metadata must remain
at the parent Git root. Moving or renaming `mm-chat/` would also move relative
runtime data paths and increase rollback risk.

**Decision**: Keep `mm-chat/` at its existing path, remove the former product
application through an explicit allowlist, and retain a thin parent repository
shell for GitHub and development metadata.

**Consequences**: The repository remains nested by one directory, but runtime
paths and data mounts remain stable. A verified external archive preserves all
uncommitted former-root work for recovery.

## Out of Scope

- Renaming `mm-chat/` to `mumu-chat/`.
- Moving `mm-chat/` contents to the parent Git root.
- Deleting or compacting Git history.
- Deleting any live database, object, Redis, secret, or backup data.
- Reworking product behavior while cleaning repository layout.

## Technical Notes

- Existing plan: `mm-chat/docs/deployment/former-root-delete-plan.md`
- Dry-run: `bash mm-chat/scripts/plan-former-root-deletion.sh`
- Standalone gate: `bash mm-chat/scripts/verify-standalone.sh --full`
- Backup destination: `/home/mumu/projects`
