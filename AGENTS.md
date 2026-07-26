# Repository Guidelines

## Product root and structure

The repository root is a thin Git/GitHub/Trellis shell. The only product
application is `mm-chat/`:

- `mm-chat/frontend/`: Next.js/React TypeScript UI and Vitest tests.
- `mm-chat/backend/`: Go API, migrations, and operator commands.
- `mm-chat/rag/`: Python 3.13 parsing and RAG worker.
- `mm-chat/postgres/`: PostgreSQL 17 retrieval image.
- `mm-chat/docs/`: architecture, contracts, deployment, and tracking docs.
- `mm-chat/scripts/`: verification, release, backup, and restore tools.

Do not recreate product source, package manifests, or Docker entrypoints at the
repository root.

## Build, test, and development commands

Use Node.js 22 with pnpm 10.30.3, Go 1.25, Python 3.13, and Docker Compose v2.

- Full standalone gate: `bash mm-chat/scripts/verify-standalone.sh --full`.
- Frontend: run `corepack pnpm install --frozen-lockfile`,
  `corepack pnpm format:check`, `corepack pnpm lint`,
  `corepack pnpm typecheck`, `corepack pnpm test`, and
  `corepack pnpm build` from `mm-chat/frontend/`.
- Backend: run `go vet ./...` and `go test ./...` from `mm-chat/backend/`.
- RAG: run `uv sync --frozen --all-groups`, `uv run ruff format --check .`,
  `uv run ruff check .`, `uv run mypy`, and `uv run pytest` from
  `mm-chat/rag/`.
- Compose: run commands from `mm-chat/` with
  `--env-file .env.single-server`.

## Coding and testing conventions

Frontend code uses strict TypeScript, `@/*` imports, two-space indentation,
double quotes, semicolons, and Prettier. Components use PascalCase, hooks start
with `use`, and utilities use camelCase. Add Vitest coverage under
`mm-chat/frontend/src/__tests__/` for UI, state, route, and security changes.

Backend changes require focused Go tests beside the owning package and migration
replay/schema coverage when persistence changes. RAG changes require typed
Python, Ruff, mypy, and pytest coverage. Cross-layer changes must trace the full
request, storage, and failure path.

## Commits and pull requests

Use focused conventional prefixes such as `feat:`, `fix:`, `docs:`, `refactor:`,
and `chore:`. Pull requests must summarize user-facing impact and list the
component checks actually run. Update `mm-chat/docs/` when configuration,
deployment, security, storage, or operational behavior changes.

## Security and runtime state

Never commit real API keys, provider secrets, private chat logs, or user files.
Use `mm-chat/.env.single-server.example` as the configuration template. Never
delete or rewrite `mm-chat/data/`, `mm-chat/secrets/`, `mm-chat/backup/`, or a
live `mm-chat/.env.single-server` during source cleanup. Treat those paths as
runtime state, not source.
