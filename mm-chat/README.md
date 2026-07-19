# mm-chat

`mm-chat/` is the self-contained project root for the server-backed Neo Chat
runtime. It contains the complete Next.js frontend, Go API, private Python RAG
worker, migrations, Compose topology, deployment scripts, and operational
documentation. Commands below are run from this directory; nothing requires
the former repository-root application.

## Project Layout

```text
frontend/                  Next.js 16 / React 19 application
backend/                   Go API, migrations, and operator commands
rag/                       Private Python RAG worker and parser sidecar
compose.yml                Canonical local Compose entrypoint
compose.single-server.yml  Complete single-server topology
compose.production.yml     Digest-only production override
scripts/                   Verification, migration, backup, and restore tools
docs/                      Architecture, contracts, deployment, and progress
```

The frontend preserves the existing Neo Chat interface. Chat, files, browser
import, Auth, Teams, and Knowledge server contracts are being cut over to Go;
legacy Next.js `/api/*` handlers remain only where parity work is unfinished.

## Prerequisites

- Docker Engine with Compose v2
- Node.js 22 and Corepack for direct frontend development
- Go 1.25 for direct backend development
- Python 3.13 for direct RAG development

## Start the Complete Local Stack

Create a local environment file and replace every `change-me` value before
using real data or provider traffic:

```bash
cp .env.single-server.example .env.single-server
chmod 600 .env.single-server
./scripts/init-provider-keyring.sh
```

The keyring command creates the gitignored `secrets/provider-keyring.json` with
mode `600` under a mode-`700` user-owned directory and never prints key
material. Compose mounts it read-only only into
the Go `backend` and one-shot `admin` service. Set `MM_CHAT_RUNTIME_UID` and
`MM_CHAT_RUNTIME_GID` in the env file to `id -u` and `id -g`; Compose
file-backed secrets preserve host ownership, so those non-root services must
run as the protected file's owner.

For an existing deployment, never edit that keyring in place. Use
`scripts/rotate-provider-keyring.sh` plus the dry-run/backup/exact-plan
administrator workflow in `docs/deployment/secret-rotation.md`.

Initialize the database, then start the frontend and backend together:

```bash
docker compose --env-file .env.single-server \
  --profile ops run --rm migrate
docker compose --env-file .env.single-server \
  --profile app up -d --build
```

Open <http://127.0.0.1:3000>. Browser API calls stay same-origin under
`/mm-api`; the Next.js server forwards them to the private `backend:8080`
Compose service. Postgres, Redis, MinIO, and RAG are never exposed to the
browser.

Stop the stack without deleting data:

```bash
docker compose --env-file .env.single-server --profile app down
```

## Direct Development

Frontend:

```bash
cd frontend
corepack pnpm install --frozen-lockfile
NEXT_PUBLIC_API_MODE=server \
NEXT_PUBLIC_API_BASE_URL=/mm-api \
MM_CHAT_BACKEND_INTERNAL_URL=http://127.0.0.1:8080 \
corepack pnpm dev
```

Backend:

```bash
cd backend
go test ./...
go run ./cmd/api
```

RAG:

```bash
cd rag
python3.13 -m venv .venv
.venv/bin/pip install -e . --group dev
.venv/bin/pytest
```

## Verification

Run the structural clean-copy gate from this project root:

```bash
./scripts/verify-standalone.sh
```

Use `./scripts/verify-standalone.sh --full` to install and verify the frontend
and run the Go test suite inside the isolated copy. The final deletion of the
former root application remains a separate owner-confirmed destructive gate.

Detailed deployment, backup, and rollback instructions live in
[`docs/deployment/`](./docs/deployment/). Migration state is tracked in
[`docs/tracking/progress.md`](./docs/tracking/progress.md) and
[`docs/tracking/process.md`](./docs/tracking/process.md).
The encrypted provider boundary is defined in
[`docs/contracts/provider-secret-vault.md`](./docs/contracts/provider-secret-vault.md);
future hosted Voice work must begin from the fail-closed
[`Voice reservation contract`](./docs/contracts/voice-provider-reservation.md).
