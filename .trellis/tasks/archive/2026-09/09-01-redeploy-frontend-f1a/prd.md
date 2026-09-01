# Redeploy Frontend with F1-A Logo

## Goal

Deploy the already committed NeoBot/F1-A frontend assets to the active local
single-server Compose runtime so `http://127.0.0.1:18080` serves the new product
name and logo instead of the 36-hour-old frontend image.

## Requirements

- Build only the `frontend` image from the current clean Git revision.
- Assign the candidate an explicit immutable local tag; do not move or overwrite
  the currently running image tag.
- Preserve the current frontend image ID and back up `.env.single-server` with
  mode `0600` before changing the selected `FRONTEND_IMAGE` reference.
- Render and validate the active Compose configuration before mutation.
- Recreate only the `frontend` service with `--no-build --no-deps`.
- Keep backend, PostgreSQL, Redis, MinIO, RAG, MCP Runner, and Memory Worker
  container IDs unchanged.
- Verify frontend health, the root HTML response, and live F1-A logo assets.

## Acceptance Criteria

- [x] The active frontend container uses the newly built immutable image.
- [x] `http://127.0.0.1:18080/` returns `200 text/html`.
- [x] `/logo.svg` and `/logo-512.png` return `200` from the live frontend.
- [x] Live frontend content exposes `NeoBot`; the old `Neo Chat` wordmark is no
      longer rendered after a hard refresh.
- [x] Every unrelated service container ID is unchanged and healthy/running.
- [x] The previous frontend image and environment backup remain available for
      rollback.

## Definition of Done

- Candidate frontend image built successfully.
- Targeted Compose recreation is healthy.
- Provider-free deployed smoke checks pass.
- Deployment evidence is recorded without committing runtime secrets.

## Technical Approach

Use the existing Compose `frontend` build definition with an explicit local tag
derived from the current Git revision. Record the old container/image IDs and
all unrelated service IDs, back up the active environment, update only
`FRONTEND_IMAGE`, validate Compose, then recreate only `frontend` with
`--no-build --no-deps --force-recreate`. Verify the compiled live edge rather
than source markers alone.

## Decision (ADR-lite)

**Context:** Source and focused tests are green, but the running frontend image
predates the F1-A commit.

**Decision:** Treat this as release propagation, not another source-code fix.
Build and pin one reviewed frontend candidate and perform a targeted recreation.

**Consequences:** There is a brief frontend-only interruption. The old image and
environment copy provide immediate rollback without touching persistent data.

## Out of Scope

- Backend, RAG, database, migration, Provider, Memory, Skill, or MCP changes.
- Publishing images to an external registry.
- Changing authentication or session data.

## Technical Notes

- Active frontend port: `127.0.0.1:18080 -> 3000`.
- Current runtime image predates commit `20b5d06a`.
- Deployment contract:
  `.trellis/spec/operations/runtime-recreate-image-pinning.md`.

## Deployment Evidence

- Deployed candidate: `mm-chat/frontend:f1a-0dfb94906b25`
  (`sha256:e726eb9f77ffd3bbd94a05ff79ac3a74576d6f6ee073b2f054874785e2f92b8e`).
- Active frontend container:
  `fd5e09880c063797e8a7aa0756cf262c58d35096ad99bb17d2852a83e784bd34`
  (`healthy`).
- Live edge: `/` returned `200 text/html`; `/logo.svg`, `/logo-192.png`, and
  `/logo-512.png` matched the committed source SHA-256 values.
- Live HTML contains `NeoBot` and does not contain the old `Neo Chat` wordmark.
- Backend, MCP Runner, Memory Worker, MinIO, PostgreSQL, RAG Worker, and Redis
  container IDs remained unchanged.
- Rollback image: `mm-chat/frontend:rollback-pre-f1a-20260901T2324`
  (`sha256:71661be87bbdc2432320417393e7727e619fd3324381e2bf924ecf4e8f289e50`).
- Runtime environment backup:
  `mm-chat/backup/runtime-env/predeploy-frontend-f1a-20260901T2324.env`
  (mode `0600`).
