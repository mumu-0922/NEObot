# mm-chat Refactor Workspace

`mm-chat/` is the isolated workspace for rebuilding Neo Chat into a server-backed deployment. Do not modify the existing app directly while planning or prototyping the refactor; put new design notes, scaffolds, experiments, and future backend code here unless a later task explicitly migrates a piece into the main app.

## Document Map

All refactor documentation lives under [`docs/`](./docs/):

- [`docs/architecture/server-refactor-design.md`](./docs/architecture/server-refactor-design.md) — target architecture, phases, APIs, data model, storage, deployment, and rollback plan.
- [`docs/inventory/`](./docs/inventory/) — current Neo Chat API, storage, chat stream, and provider-flow inventories.
- [`docs/tracking/progress.md`](./docs/tracking/progress.md) — living checklist. Mark completed work with `[x]` and link evidence.
- [`docs/tracking/process.md`](./docs/tracking/process.md) — chronological work log for decisions, commands, findings, and next steps.
- [`docs/contracts/`](./docs/contracts/) — future API/client contracts.
- [`docs/deployment/`](./docs/deployment/) — single-server Compose deployment, backup, restore, release, and rollback notes.
- [`frontend/`](./frontend/) — isolated Phase 11 frontend API-client scaffold
  kept outside the existing app until `src/` wiring is explicitly approved.

## Refactor Rules

1. Keep the current Next.js/React frontend working during every phase.
2. Build by strangler migration: add a new backend path, switch one capability at a time, keep rollback flags.
3. Start single-server first: Go backend + Postgres + Redis + private MinIO through Docker Compose under this workspace.
4. Store real files in object storage or local storage abstraction; store only metadata in Postgres.
5. Do not silently upload existing browser-local data. Any migration from IndexedDB/OPFS must be user-initiated.
6. Every completed phase must update both `docs/tracking/progress.md` and `docs/tracking/process.md`.

## First MVP

The first shippable target is not the full platform. It is:

```text
Next.js frontend -> Go API -> Postgres
                         -> model provider stream
```

MVP proves conversations, messages, and streaming model responses can survive refresh and server restart. Files, Redis hardening, MinIO, and RAG follow after the chat spine is stable.
