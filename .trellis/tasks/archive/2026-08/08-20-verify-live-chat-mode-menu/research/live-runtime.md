# Live Runtime Evidence

## Decisive finding

The screenshot is served by `127.0.0.1:18080`, which maps to
`mm-chat-frontend-1`. At inspection time the container ran:

```text
mm-chat/frontend:live-tools-e0ce997005a9-20260820T064039Z
sha256:2e076bfe0ee35031df53d9dfc90b48538b3ac5344c56efd010d8208745bc5c24
created 2026-08-20T06:41:49Z
```

The UI source fix was committed later as `1e3a532a`, so the live container
cannot contain it. This explains why local tests and `next build` passed while
the user's second screenshot remained unchanged.

## Runtime topology

- Frontend edge: `127.0.0.1:18080 -> frontend:3000`
- Backend edge: `127.0.0.1:8080 -> backend:8080`
- Compose project: `mm-chat`
- Compose files recorded on the live container:
  `compose.single-server.yml,compose.production.yml`
- Environment file: `.env.single-server` (contents remain protected)

## Rollout constraint

`compose.production.yml` removes local build definitions, so build the new
frontend image with `compose.single-server.yml` only. Recreate the service with
both live Compose files and the updated `FRONTEND_IMAGE` tag.

## Rollback

Retain the previous image tag and a mode-`0600` environment backup. Rollback is
an atomic `FRONTEND_IMAGE` restore followed by frontend-only recreation.

## Rollout result

- Protected rollback directory:
  `mm-chat/backup/chat-mode-menu-20260820T075214Z`
- Retained old image:
  `mm-chat/frontend:retained-before-chat-mode-menu-20260820T075214Z`
- New live image:
  `mm-chat/frontend:chat-mode-menu-1e3a532a-20260820T075223Z`
- New live image ID:
  `sha256:3c680bcef59379cbdccf36a1122bc8348c673523e02c65ce502231b6f2e4d7e3`
- Only `mm-chat-frontend-1` changed container ID; every unrelated live
  container retained its ID.
- Frontend health, direct Backend readiness, and same-origin Backend readiness
  all passed.

The actively served HTML now references
`/_next/static/chunks/36mxv-z57h45r.css`. Its SHA-256 is
`31998fd923498ba9ece65a2b081941fdd5bb0b834feb372b0a021094a512c6e5`, and
the emitted `h-auto` / `items-start` rules occur after the compact primitive's
`h-8` / `items-center` rules. The previous image served a different CSS chunk,
so a normal page reload selects the new immutable asset; an already-open tab
cannot hot-swap production JavaScript and CSS without a reload.

## Exact rollback action

Restore
`backup/chat-mode-menu-20260820T075214Z/.env.single-server.before` over the
live environment atomically, then run:

```bash
docker compose --env-file .env.single-server \
  -f compose.single-server.yml -f compose.production.yml --profile app \
  up -d --no-build --no-deps --pull never --force-recreate frontend
```

Require Frontend health plus direct and same-origin Backend readiness before
declaring rollback complete.

## Final verification

- `bash mm-chat/scripts/verify-standalone.sh --full`: passed.
- Frontend: format, lint, typecheck, production build, and `922` Vitest tests
  passed.
- Backend: full Go test suite passed.
- RAG: Ruff, mypy, and pytest passed (`1906` passed, `7` skipped).
- Post-gate live checks still reported the candidate Frontend healthy and both
  direct and same-origin Backend readiness as `status=ready`.
