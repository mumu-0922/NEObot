# Former-root cleanup result

## Backup and rollback

- External backup:
  `/home/mumu/projects/neo-chat-former-root-backup-20260726-144734`
- Working-copy archive: `former-root.tar.gz` (`513 MiB`)
- Archive SHA-256:
  `73234bb8c172dd104dd2414be0f8f800d7d40092a3e0814abb36ae642dcf4b06`
- Archived top-level paths: `39`; manifest match passed.
- Local pre-delete tag: `former-root-predelete-20260726-144734` at
  `e7e9c4bf51afef78db0da6e0949c1eb5302d2f94`.
- The external `README.txt`, `deletion-report.txt`, `evidence.sha256`, Git
  metadata, and archive manifest contain the replay information.

## Runtime restore drill

- PostgreSQL dump:
  `runtime/postgres/postgres-20260726T065404Z.dump` (`8.8 MiB`), checksum
  passed.
- PostgreSQL temporary restore passed at migration
  `50:jina_runtime_retirement`; all 50 migration rows matched the live
  manifest.
- Restored Knowledge state: 14 collections, 82 documents, 82 document
  versions, 684 parent chunks, 699 child chunks, and 7,480 block spans.
- Document Version/File metadata mismatches: `0`.
- MinIO archive: `runtime/minio/minio-20260726T065405Z.tar.gz` (`23 MiB`),
  checksum passed.
- MinIO temporary-bucket restore: `109/109` objects; five PostgreSQL-sampled
  Knowledge object keys passed `mc stat`.
- Temporary database and bucket cleanup passed.
- Detailed evidence:
  `runtime/restore-drill-20260726T065830Z/` in the external backup.

## Source cleanup

- Removed all 28 physical former-root allowlist paths.
- Retained `.github/`, `.gitignore`, `.env.example`, `AGENTS.md`, community
  files, repository READMEs, and protected tooling metadata.
- Retargeted CI to standalone structure, frontend, backend, and RAG jobs.
- Retargeted container publishing to backend, frontend, RAG, and PostgreSQL
  images under the documented `neobot-mm-chat` repositories.
- Retargeted Dependabot, issue/PR templates, security links, and root docs.
- `git status --porcelain -- mm-chat` remained empty.

## Verification

- `bash mm-chat/scripts/verify-standalone.sh --full`: passed.
  - Frontend: 193 files / 936 tests passed; production build passed.
  - Backend: all Go tests passed.
  - RAG: 1,906 passed / 7 skipped; Ruff and mypy passed.
- `go vet ./...` from `mm-chat/backend`: passed.
- Compose config with live and example env files: passed.
- Root metadata Prettier check: passed.
- Workflow YAML parse and `actionlint`: passed.
- Live stack: backend, frontend, PostgreSQL, Redis, and RAG worker healthy;
  MinIO running.
- HTTP smoke: frontend `/`, backend `/ready`, frontend `/mm-api/health`, and
  private RAG `/health` all returned `200`.
