# mm-chat Frontend

This directory is the standalone Next.js/React frontend for `mm-chat`. It
preserves the existing Neo Chat interface and currently carries the complete UI,
assets, tests, and build configuration that previously ran only from the
repository root.

## Current Migration State

- The complete frontend source and static assets now live here.
- Chat CRUD/SSE, Files, and Browser Import already have Go server adapters.
- Legacy Next.js `/api/*` handlers remain temporarily for feature-parity work.
- `local|server` remains a transition mechanism only. The frozen final runtime
  is server-only with explicit browser-data import.
- Do not redesign the interface during backend cutover. New features must reuse
  the existing theme, layout, components, responsive rules, and accessibility
  behavior.

## Commands

Use Node.js 22 and pnpm 10.30.3:

```bash
corepack pnpm install --frozen-lockfile
corepack pnpm dev
corepack pnpm typecheck
corepack pnpm lint
corepack pnpm test
corepack pnpm build
```

Server-mode development uses the Go backend:

```bash
NEXT_PUBLIC_API_MODE=server \
NEXT_PUBLIC_API_BASE_URL=/mm-api \
MM_CHAT_BACKEND_INTERNAL_URL=http://127.0.0.1:8080 \
corepack pnpm dev
```

The root `compose.single-server.yml` builds this frontend in server mode and
provides the persistent `/mm-api` same-origin edge to the private Go service.
Run the complete stack from `mm-chat/` with the `app` profile.

## Final Standalone Gate

The frontend is not considered fully migrated until:

1. every legacy `/api/*` capability has a Go/RAG replacement;
2. production local-mode and browser-local authority are removed;
3. `mm-chat/` builds and runs from a clean copy without the original root app;
4. visual and interaction regression checks preserve the existing interface;
5. the owner approves the separate original-project deletion plan.

See [`DESIGN.md`](./DESIGN.md) and
[`../docs/inventory/standalone-cutover-gap.md`](../docs/inventory/standalone-cutover-gap.md).
