# mm-chat Frontend Adapter Scaffold

This directory holds the Phase 11.1 TypeScript scaffold for the future
`local|server` frontend API boundary. It is intentionally isolated under
`mm-chat/` until the owner approves wiring it into the existing Next.js app
under `src/`.

See [`DESIGN.md`](./DESIGN.md) for design decisions, tradeoffs, and security boundaries.

Current scope:

- resolve `NEXT_PUBLIC_API_MODE=local|server` with `local` as safe fallback;
- resolve `NEXT_PUBLIC_API_BASE_URL` without performing network calls;
- classify the browser network edge as same-origin proxy or direct-CORS;
- centralize server HTTP URL building and JSON error normalization;
- parse Go SSE frames for later Phase 11.3 streaming work;
- expose compile-safe local/server adapter shells with unsupported operations.

Out of scope for this slice:

- wiring existing React components or Zustand stores;
- persisting conversations/messages through Go;
- streaming assistant responses from the UI;
- file upload/download UI integration;
- auth, import/export, RAG, plugins, voice, documents, image generation.

Run targeted checks from the repository root. If project dependencies are not
installed locally, these `corepack pnpm dlx` commands reproduce the verification
used for Phase 11.1A:

```bash
corepack pnpm dlx vitest@4.1.9 run mm-chat/frontend/__tests__/api-client.test.ts
corepack pnpm --package=typescript@5.9.3 dlx tsc --noEmit --target ES2020 --module ESNext --moduleResolution Bundler --lib DOM,ESNext --strict --skipLibCheck mm-chat/frontend/src/api-client/index.ts
corepack pnpm dlx prettier@3.9.4 --check 'mm-chat/frontend/**/*.ts' mm-chat/frontend/README.md mm-chat/frontend/DESIGN.md
git diff --check -- mm-chat
```
