# NeoBot standalone stack

`mm-chat/` is the self-contained product root for the server-backed NeoBot
runtime. It contains the complete Next.js frontend, Go API, private Python RAG
worker, migrations, Compose topology, deployment scripts, and operational
documentation. Commands below are run from this directory; nothing requires
the former repository-root application.

## Project Layout

```text
frontend/                  Next.js 16 / React 19 application
backend/                   Go API, migrations, and operator commands
rag/                       Private Python RAG worker and parser sidecar
postgres/                  PostgreSQL 17 BM25/pgvector retrieval image
mcp/                       Versioned MCP manifest and disabled token fixture
compose.yml                Canonical local Compose entrypoint
compose.single-server.yml  Complete single-server topology
compose.production.yml     Digest-only production override
scripts/                   Verification, migration, backup, and restore tools
docs/                      Architecture, contracts, deployment, and progress
```

The owner-approved standalone and single-server scope is complete through the
current database migration `109_memory_dead_letter_orphan_activity`.
Production builds use server mode: the browser talks to the same-origin Next.js
edge, the edge forwards `/mm-api` to Go, and durable authority stays in
PostgreSQL/MinIO. Legacy Next.js `/api/*` handlers and local adapters remain
only for explicit compatibility and rollback paths; they are not production
persistence authority.

The main product surfaces are multi-provider Chat, assistants and Agent runs,
Skills and MCP tools, web search, Knowledge RAG, governed Memory, voice and
generated media, rich artifacts, account security, and operator-managed
provider configuration. Default-off and canary features remain governed by the
committed environment schema and deployment contracts.

## Prerequisites

- Docker Engine with Compose v2
- Node.js 22, Corepack, and pnpm 10.30.3 for direct frontend development
- Go 1.25 for direct backend development
- Python 3.13 and `uv` for direct RAG development

## First Boot and Local Stack

Create a local environment file and replace every `change-me` value before
using real data or provider traffic:

```bash
cp .env.single-server.example .env.single-server
chmod 600 .env.single-server
mkdir -p data/agent-skills data/agent-workspace
chmod 700 data/agent-skills data/agent-workspace
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

Replace every placeholder, including the runtime UID/GID and independent
database role credentials. A fresh database requires migrations followed by
interactive creation of the API, Memory Worker, RAG Worker, and Replay logins,
then one-time Owner identity bootstrap. Follow
[`single-server-compose.md`](./docs/deployment/single-server-compose.md#local-development-first-boot)
and
[`postgres-single-server.md`](./docs/deployment/postgres-single-server.md#fresh-install-role-provisioning)
exactly; do not collapse those credential steps into command-line secrets.

After the first-boot procedure is complete, start the frontend and backend:

```bash
docker compose --env-file .env.single-server \
  --profile app up -d --build
```

Open <http://127.0.0.1:3000>. Browser API calls stay same-origin under
`/mm-api`; the Next.js server forwards them to the private `backend:8080`
Compose service. Postgres, Redis, MinIO, and RAG are never exposed to the
browser.

The user-facing **Tools** area is backed by MCP. Remote Streamable HTTP servers
run through the Go backend. Approved local stdio servers use one optional,
hardened Runner that starts child processes on demand rather than one resident
container per server. MCP is disabled by default; follow
[`docs/deployment/mcp-runner.md`](./docs/deployment/mcp-runner.md) before
enabling it.

The composer and Agent share a server-authorized Skill/MCP discovery and
install plane with exact revisions, approval, audit, and snapshot refresh. See
[`docs/contracts/resource-orchestration.md`](./docs/contracts/resource-orchestration.md).
Set `RESOURCE_ORCHESTRATION_ENABLED=false` to disable this mutation/recovery
plane without removing the existing Skill Store or Tools pages.

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
uv sync --frozen --all-groups
uv run ruff format --check .
uv run ruff check .
uv run mypy
uv run pytest
```

## Verification

Run the structural clean-copy gate from this project root:

```bash
./scripts/verify-standalone.sh
```

Use `./scripts/verify-standalone.sh --full` to install and verify the frontend,
run the Go suite, and create an isolated Python environment for all RAG quality
gates inside the clean copy.

Run the deterministic browser journeys from `frontend/` without Provider
credentials or billable model traffic:

```bash
corepack pnpm test:e2e:install
corepack pnpm test:e2e
```

The suite covers Auth and account security, per-Conversation model persistence,
the Agent Harness lifecycle, Memory governance and recall, Knowledge RAG, and
Skill/MCP resource flows. Failure traces, screenshots, and videos are written
below `frontend/test-results/e2e/`; see
[`frontend/e2e/README.md`](./frontend/e2e/README.md).

The current Chat Agent `local_direct` wiring and retired-control-plane boundary
have focused gates:

```bash
./scripts/verify-agent-local-runtime.sh
./scripts/verify-legacy-agent-cleanup-postgres17.sh
```

These validate the no-sudo Backend workspace, retained Skill Store and Chat
runtime, absence of legacy services, and fail-closed migration `098`. The local
runtime is deliberately not a Sandbox.

Detailed deployment, backup, and rollback instructions live in
[`docs/deployment/`](./docs/deployment/). Migration state is tracked in
[`docs/tracking/progress.md`](./docs/tracking/progress.md) and
[`docs/tracking/process.md`](./docs/tracking/process.md).
The encrypted provider boundary is defined in
[`docs/contracts/provider-secret-vault.md`](./docs/contracts/provider-secret-vault.md),
and the production SiliconFlow TTS boundary is defined in the
[`Voice provider production contract`](./docs/contracts/voice-provider-reservation.md).
The MCP boundary is defined in
[`docs/architecture/mcp-tools.md`](./docs/architecture/mcp-tools.md) and
[`docs/contracts/mcp-tools-api.md`](./docs/contracts/mcp-tools-api.md).
