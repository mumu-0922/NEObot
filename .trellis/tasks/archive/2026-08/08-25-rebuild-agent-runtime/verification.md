# Verification

## Deployment evidence

- Previous Backend container:
  `c9a5b3a71f301ad05a37eb0846901f00c4dfbab80f3229141eedcb46c1503e54`.
- Previous Backend image:
  `sha256:4da645d1602bca0d789233d354087bea2a678a103b7e1b45dc04bf0e5011d0e6`.
- Protected rollback tag:
  `mm-chat/backend:rollback-before-completion-driven-20260825`.
- Candidate tag: `mm-chat/backend:agent-completion-driven-37341994`.
- Candidate image:
  `sha256:dba10d51235bf5fd1adbe3cc26daf1f6ca78b407f4904e54869b5fa259606221`.
- Candidate identity: `USER mmchat:mmchat`, command
  `/usr/local/bin/mm-chat-api`.
- New Backend container:
  `a4ab0e2ad6185a9c57630a9634563772c6b6e4ebf107d8934705e3993aeb4d1c`.
- The Frontend, MCP Runner, Memory worker, MinIO, PostgreSQL, RAG worker, and
  Redis container IDs remained unchanged.
- Live environment backup:
  `mm-chat/backup/.env.single-server.before-agent-completion-37341994`, mode
  `0600`.

## Runtime probes

- Backend Compose health: `healthy`.
- `GET /health`: `status=healthy`.
- `GET /ready`: `status=ready`; database, Redis, and storage all `ready`.
- Agent Host: healthy with the existing pinned Runner.
- Recent Backend startup logs: zero panic, fatal, or error matches.

## Test evidence

- Focused completion-driven `internal/chat` regression selection: passed.
- `bash mm-chat/scripts/verify-agent-local-runtime.sh`: passed.
- `GOCACHE=/tmp/neo-chat-go-cache go vet ./...`: passed.
- `GOCACHE=/tmp/neo-chat-go-cache go test ./...`: passed.

## Scope proof

- Only the Backend container was recreated.
- No migration, data-service restart, paid Provider call, application source
  edit, or runtime-data deletion was performed.
