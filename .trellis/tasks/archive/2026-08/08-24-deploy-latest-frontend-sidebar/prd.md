# Deploy Latest Frontend Sidebar

## Goal

Deploy the already verified frontend from commit `d571bfdf` so the live UI
uses the unified workspace conversation tree and displays persisted response
duration metadata.

## Current evidence

- The running Frontend uses
  `mm-chat/frontend:web-url-reader-74519dea-20260823T173826Z`.
- That image predates commit `d571bfdf`.
- The Backend, database, object storage, RAG, and memory services are healthy
  and are outside this rollout.

## Requirements

- Build the current Frontend source into a new retained local image tag.
- Preserve the current Frontend image as an explicit rollback tag.
- Update only the runtime `FRONTEND_IMAGE` pointer.
- Recreate only the Frontend container with `--no-build --no-deps`.
- Do not mutate PostgreSQL, MinIO, Redis, knowledge, memory, or chat data.
- Verify the candidate image, Frontend health, root page, Backend readiness
  through the live edge, decisive compiled JavaScript, and unchanged unrelated
  container identities.
- Roll back to the retained image if any rollout proof fails.

## Acceptance criteria

- [x] The live Frontend container runs the candidate image built from the
      current source tree.
- [x] The root page and `/mm-api/ready` succeed through the Frontend edge.
- [x] Live compiled JavaScript contains the unified sidebar behavior delivered
      by `d571bfdf`.
- [x] Only the Frontend container identity changes.
- [x] All unrelated application and persistence services remain healthy.
- [x] The active runtime environment points at the candidate Frontend tag.

## Deployment evidence

- Candidate tag:
  `mm-chat/frontend:workspace-sidebar-d571bfdf-20260824T023312Z`
- Candidate image ID:
  `sha256:71ae2d1b9d49aef75c16b9f595f1ec30f2625c0d79cf425c968af83d30298d78`
- Rollback tag:
  `mm-chat/frontend:retained-before-sidebar-d571bfdf-20260824T023312Z`
- Frontend container changed from
  `7836930752c256efd67dd00c79b167162027cebf1e11787b729fb8c0f8ce09cd`
  to
  `6c9e93c5e5964ddd072bb449163c51e8ed6c9b09f1fa9d2022a4f623e0ea24ea`.
- The live root returned HTTP `200`; `/mm-api/ready` returned ready for
  database, Redis, and storage.
- Live asset `/_next/static/chunks/3e57u56js6aee.js` contains the Chinese
  temporary-chat and response-duration markers.
- Backend, MCP Runner, Memory Worker, MinIO, PostgreSQL, RAG Worker, and Redis
  retained their pre-deployment container IDs.

## Out of scope

- Product source changes.
- Database migrations or persistence changes.
- Backend, RAG, memory, MinIO, Redis, or PostgreSQL recreation.
