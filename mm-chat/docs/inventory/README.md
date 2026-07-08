# Inventory Docs

These documents capture the current Neo Chat behavior before migration.

- [`api-routes.md`](./api-routes.md) — current Next.js API route inventory and future Go ownership.
- [`storage.md`](./storage.md) — current localStorage/IndexedDB/OPFS usage and target server storage.
- [`chat-flow.md`](./chat-flow.md) — current chat streaming path and target Go streaming spine.
- [`provider-flow.md`](./provider-flow.md) — current provider resolution and target server-side provider boundary.
- [`frontend-call-sites.md`](./frontend-call-sites.md) — frontend fetch/storage/OPFS call sites that must move behind the API client boundary.
- [`browser-data-export.md`](./browser-data-export.md) — current full-app/session export surfaces, local chat shapes, OPFS references, and Phase 8 normalization risks.
