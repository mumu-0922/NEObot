# Former Root Delete Plan

This is the G10 destructive-cleanup plan for making `mm-chat/` the only
product project root. It is a plan and dry-run gate, not deletion approval.

## Boundary

Current former root:

```text
<former-root>
```

Standalone project root to preserve:

```text
<former-root>/mm-chat
```

Never delete automatically:

```text
<former-root>/mm-chat
<former-root>/.git
<former-root>/.trellis
<former-root>/.agents
<former-root>/.codex
<former-root>/.vscode
```

The protected metadata directories may be archived or moved later, but they are
not part of the product cleanup command block.

## Required gates before deletion

Run and record all gates before asking for destructive approval:

```bash
cd <former-root>
bash mm-chat/scripts/verify-standalone.sh
bash mm-chat/scripts/verify-standalone.sh --full
bash mm-chat/scripts/plan-former-root-deletion.sh
```

Production/runtime gates, when a live stack is in scope:

```bash
cd <former-root>/mm-chat
./scripts/backup-single-server-production.sh .env.single-server
# Verify the produced .sha256 files under backup/postgres and backup/minio.
# Run the Postgres temporary restore drill and MinIO restore drill from
# docs/deployment/backup-restore.md before deleting former-root artifacts.
```

Visual/interaction smoke must cover at least:

- app shell loads from `mm-chat` frontend;
- chat list/message send/stream path;
- provider/model selection visibility;
- Knowledge collection selection and citation-card rendering;
- Files/upload path if configured;
- mobile-width navigation smoke.

## Dry-run command

```bash
cd <former-root>
bash mm-chat/scripts/plan-former-root-deletion.sh
```

The dry-run script prints:

- protected paths that are not deletion candidates;
- legacy top-level application deletion candidates;
- env/secret-like top-level paths that require manual review and are not in the
  generated `rm` block;
- unclassified paths that require manual review and are not in the generated
  `rm` block;
- a one-path-per-line owner-confirmed `rm -rf -- ...` block.

It never deletes files.

## Current candidate manifest

The generated deletion candidates are limited to known legacy application
artifacts at the former root. The manifest intentionally excludes `mm-chat/`,
Git metadata, Trellis/task metadata, agent config, editor config, and any
unclassified path.

Current known candidate names:

```text
.dockerignore
.env.example
.github
.gitignore
.mypy_cache
.next
.prettierignore
.ruff_cache
.wrangler
AGENTS.md
CHANGELOG.md
CODE_OF_CONDUCT.md
CONTRIBUTING.md
Dockerfile
LICENSE
README.md
README.zh-CN.md
ROADMAP.md
SECURITY.md
docker-compose.yml
docs
eslint.config.mjs
next-env.d.ts
next.config.ts
node_modules
open-next.config.ts
package.json
pnpm-lock.yaml
pnpm-workspace.yaml
postcss.config.mjs
prettier.config.mjs
public
scripts
src
tailwind.config.ts
tsconfig.json
tsconfig.tsbuildinfo
wrangler.jsonc
```

If the dry-run prints any `Manual-review` or `Unclassified` path, stop and
update this document before deletion.

## Approval phrase

Deletion requires a separate owner message that includes all of this
information:

```text
I approve destructive former-root cleanup for <former-root>.
Preserve <former-root>/mm-chat and the protected metadata paths.
Gates passed: verify-standalone structure/full, backup checksums, restore drills,
visual smoke. Backup location: <path>. Commit: <sha>. Proceed.
```

Without that exact owner intent, do not run the generated `rm` commands.

## Execution procedure after approval

1. Save the dry-run output into the process log.
2. Re-run the full gate immediately before deletion:

   ```bash
   cd <former-root>
   bash mm-chat/scripts/verify-standalone.sh --full
   ```

3. Re-run the dry-run and ensure it has no manual-review or unclassified paths:

   ```bash
   bash mm-chat/scripts/plan-former-root-deletion.sh
   ```

4. Run only the generated `rm -rf -- ...` commands from the latest dry-run.
5. Do not delete `.git`, `.trellis`, `.agents`, `.codex`, `.vscode`, or
   `mm-chat/` in this cleanup.
6. Verify from the preserved standalone root:

   ```bash
   cd <former-root>/mm-chat
   bash scripts/verify-standalone.sh
   bash scripts/verify-standalone.sh --full
   docker compose -f compose.yml config >/tmp/mm-chat-compose-post-delete.yml
   ```

7. Commit only the deletion if Git metadata is preserved at the parent root.

## Rollback

If verification fails after deletion:

1. stop the stack;
2. restore deleted tracked paths from Git or from the pre-delete archive;
3. restore runtime data only from verified Postgres/MinIO backups, never from a
   partial working-tree cache;
4. rerun `bash mm-chat/scripts/verify-standalone.sh --full`;
5. record the failure and restored commit in the process log.

If `mm-chat/` itself is damaged, restore the standalone directory from the
pre-delete archive and do not retry cleanup until a new dry-run is reviewed.
