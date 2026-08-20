# Verify Live Chat/Agent Mode Menu

## Goal

Make the already-implemented Chat/Agent multiline menu fix visible in the live
single-server frontend and prove the actively served runtime contains it.

## Confirmed Scope

- The user confirmed the menu still overlaps in the live page after the source
  fix.
- Preserve the existing source fix and deploy only the frontend unless live
  artifact inspection disproves it.
- Do not change Backend, database state, secrets, or user data.

## Requirements

- Trace the screenshot's live endpoint to the actual container and image.
- Build a fresh frontend image from the current committed source.
- Preserve a rollback record of the previous image and environment file.
- Recreate only the frontend service with the new immutable local tag.
- Verify readiness and prove the actively served CSS applies content-driven
  item height after the compact primitive height.

## Acceptance Criteria

- [x] The live frontend container runs a newly built image containing commit
      `1e3a532a`.
- [x] The live endpoint remains healthy and proxies Backend readiness.
- [x] Served production CSS places `h-auto` after `h-8` and `items-start` after
      `items-center`.
- [x] The previous frontend image reference and protected environment backup
      provide an exact rollback path.

## Definition of Done

- Frontend source quality gates from the prior fix remain valid.
- Live build, container health, edge checks, and served-asset inspection pass.
- Runtime evidence and rollback steps are recorded without exposing secrets.

## Technical Approach

Build `mm-chat/frontend` through `compose.single-server.yml` under a new local
tag, atomically update only `FRONTEND_IMAGE` in the live `0600` environment,
then recreate only `frontend` through the same single-server plus production
Compose topology used by the current container. Validate the container image
ID, HTTP readiness, and utility ordering in the CSS fetched from port `18080`.

## Out of Scope

- Additional menu redesign or Backend changes.
- Database migrations, persistent-data writes, registry push, or unrelated
  service recreation.

## Technical Notes

- Source fix commit: `1e3a532a`.
- Current live frontend image predates the fix and was built at 06:41 UTC.
- See `research/live-runtime.md` for decisive runtime evidence.
