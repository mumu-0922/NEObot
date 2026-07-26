# NEObot / MM Chat

<p align="center">
  <img src="mm-chat/frontend/public/logo.png" width="96" alt="MM Chat logo" />
</p>

<p align="center">
  <strong>A self-hosted AI chat stack with a Next.js frontend, Go API,
  PostgreSQL, private object storage, and a Python RAG worker.</strong>
</p>

<p align="center">
  <a href="README.zh-CN.md">简体中文</a>
  ·
  <a href="https://github.com/mumu-0922/NEObot/actions/workflows/ci.yml">CI</a>
  ·
  <a href="https://github.com/mumu-0922/NEObot/actions/workflows/docker.yml">Docker</a>
</p>

The product source lives entirely in [`mm-chat/`](./mm-chat/). The repository
root is intentionally a thin GitHub and development-tooling shell; it no longer
contains a second application.

## Repository layout

```text
mm-chat/frontend/  Next.js 16 / React 19 frontend
mm-chat/backend/   Go API, migrations, and operator commands
mm-chat/rag/       Python document parsing and RAG worker
mm-chat/postgres/  PostgreSQL 17 retrieval image
mm-chat/docs/      Architecture, contracts, deployment, and runbooks
mm-chat/scripts/   Verification, release, backup, and restore tools
```

See [`mm-chat/README.md`](./mm-chat/README.md) for architecture, direct
development, and operational details.

## Quick start

Requirements: Docker Engine with Compose v2. Direct component development also
uses Node.js 22, pnpm 10.30.3, Go 1.25, and Python 3.13.

```bash
cd mm-chat
cp .env.single-server.example .env.single-server
chmod 600 .env.single-server
./scripts/init-provider-keyring.sh

docker compose --env-file .env.single-server \
  --profile ops run --rm migrate
docker compose --env-file .env.single-server \
  --profile app up -d --build
```

Replace every placeholder before using real data or provider traffic. Open
<http://127.0.0.1:3000> unless `FRONTEND_PORT` overrides the default.

## Verification

```bash
bash mm-chat/scripts/verify-standalone.sh --full

cd mm-chat/frontend
corepack pnpm format:check
corepack pnpm lint
corepack pnpm typecheck
corepack pnpm test
corepack pnpm build

cd ../backend
go vet ./...
go test ./...

cd ../rag
uv sync --frozen --all-groups
uv run ruff format --check .
uv run ruff check .
uv run mypy
uv run pytest
```

## Security and operations

- Copy configuration from
  [`mm-chat/.env.single-server.example`](./mm-chat/.env.single-server.example).
- Never commit `mm-chat/.env.single-server`, `mm-chat/data/`,
  `mm-chat/secrets/`, or `mm-chat/backup/`.
- Follow the reviewed deployment and recovery procedures in
  [`mm-chat/docs/deployment/`](./mm-chat/docs/deployment/).
- Report vulnerabilities through
  [GitHub Security Advisories](https://github.com/mumu-0922/NEObot/security/advisories/new).

## License

[MIT](./LICENSE)
