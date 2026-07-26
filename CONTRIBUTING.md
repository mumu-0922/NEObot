# Contributing

Thanks for helping improve MM Chat. Product changes belong under `mm-chat/`;
the repository root only contains GitHub and development-tooling metadata.

## Development setup

Requirements:

- Docker Engine with Compose v2
- Node.js 22 and pnpm 10.30.3
- Go 1.25
- Python 3.13 and uv

Start the complete stack by following [`mm-chat/README.md`](mm-chat/README.md).
Never reuse production secrets or runtime data for development fixtures.

## Quality checks

Run the checks for every component you changed:

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

Also render Compose after deployment changes:

```bash
cd mm-chat
docker compose --env-file .env.single-server.example config --quiet
```

## Pull request guidelines

- Keep changes focused and explain user-facing and operational impact.
- Add tests for bug fixes, migrations, API routes, state changes, and
  security-sensitive behavior.
- Update `mm-chat/docs/` for configuration, deployment, privacy, storage, or
  workflow changes.
- Include screenshots for visible UI changes.
- Do not include API keys, passwords, provider secrets, private logs, database
  dumps, object-storage archives, or user files.
- Preserve `mm-chat/data/`, `mm-chat/secrets/`, `mm-chat/backup/`, and the live
  `mm-chat/.env.single-server`.

## Reporting security issues

Do not open public issues for vulnerabilities. Use
[GitHub Security Advisories](https://github.com/mumu-0922/NEObot/security/advisories/new).
