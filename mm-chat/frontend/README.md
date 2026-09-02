# mm-chat Frontend

This directory is the standalone Next.js/React frontend for NeoBot. It carries
the complete UI, assets, tests, and build configuration for the server-backed
product.

## Current Runtime State

- Production Compose builds the frontend in server mode and sends `/mm-api`
  through the same-origin Next.js edge to the private Go API.
- Chat/SSE, Files, Browser Import, Auth, provider settings, Teams, Knowledge,
  Memory, Skills, MCP, Agent, voice, image, and workspace surfaces have typed Go
  server adapters.
- Server Memory settings expose Project/Conversation governance, scoped Memory/
  Review/detail/delete progress, assistant Activity, and PR10 encrypted
  `.mm-memory` Export/Import with dry-run before confirm. Local Memory remains
  hidden rather than deleted in server mode.
- Legacy Next.js `/api/*` handlers and local adapters remain only for explicit
  compatibility/rollback paths. They must not become silent fallback authority
  in server mode.
- New features must reuse the existing theme, layout, components, responsive
  rules, accessibility behavior, and concise UI copy discipline.

## Commands

Use Node.js 22 and pnpm 10.30.3:

```bash
corepack pnpm install --frozen-lockfile
corepack pnpm dev
corepack pnpm format:check
corepack pnpm lint
corepack pnpm typecheck
corepack pnpm test
corepack pnpm build
corepack pnpm test:e2e
corepack pnpm logo:generate
```

`public/logo.svg` is the canonical NeoBot logo asset. `logo:generate`
deterministically renders the 192px/512px PNGs and multi-size favicon from that
vector master. Keep the inline `Logo` component geometry aligned with the SVG;
do not hand-edit generated PNG or ICO derivatives.

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

The Memory UI is selected by API authority, not a user-facing mode switch:
`MemorySettings` renders `ServerMemoryGovernance` only when the server Memory
capability is active, and otherwise retains `LocalMemorySettings` for rollback.
The server screen never reads or imports the local Zustand Memory store.
Portability passphrases, selected packages, and dry-run plans remain transient
component state; they are not written to browser persistence. Imported settings
are displayed as suggestions and are never applied by the frontend.

## UI Copy Discipline

- Keep visible helper copy only when it enables an action, explains an error,
  or resolves genuine ambiguity.
- Do not add labels or status sentences that merely repeat a state already
  communicated by the component title, icon, color, count, or content.
- Prefer concise labels and existing tooltips over explanatory annotations;
  remove translation keys when their visible copy is removed.

## Standalone Invariants

The frontend remains release-safe only while:

1. production builds stay in server mode with the `/mm-api` same-origin edge;
2. server mode never silently reads browser-local durable authority;
3. unsupported local-mode adapter methods fail closed;
4. `mm-chat/` builds and runs from an isolated clean copy;
5. unit, build, and deterministic Playwright journeys preserve the interface
   and server contracts.

See [`DESIGN.md`](./DESIGN.md) and the authoritative
[`standalone progress ledger`](../docs/tracking/progress.md).
