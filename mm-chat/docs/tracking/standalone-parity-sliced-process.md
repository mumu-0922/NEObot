# Standalone Parity Sliced Process Log

This is the dedicated process log for the active standalone parity cutover plan:
[`../architecture/standalone-parity-sliced-cutover-plan.md`](../architecture/standalone-parity-sliced-cutover-plan.md).

Use this log for new cutover work instead of burying remaining migration notes
inside the large legacy `process.md`. Keep entries concise, evidence-backed, and
secret-free.

## Process Rule

Every migration group records:

```text
Date / Group / Objective
Changed surfaces
Verification commands and decisive results
Runtime or browser smoke evidence
Residual risks
Next group or rollback note
```

A group is not complete until its targeted tests and smoke are recorded here.
Full-suite gates are reserved for domain cutover, release candidates, and final
clean-copy closure.

Owner commit discipline recorded on 2026-07-15: after each migration group is
implemented, tested, and recorded, create a focused Git commit for that
completed group before starting the next group. Do not batch unrelated future
groups into the same commit.

## 2026-07-16 — G10.2b Build-based Compose Closure

Objective: switch the final operations proof back to the owner's preferred
source-build deployment flow and prove `mm-chat/` can build and run without
registry image publication.

Completed scope:

- recorded the owner decision: use `docker compose build` / `up --build` from
  `mm-chat/`; GHCR push and immutable digest env proof are optional hardening,
  not a required standalone/deletion gate;
- removed the Dockerfile frontend pin from the RAG image because the file does
  not require Dockerfile 1.7 features and the extra remote syntax fetch made
  local Compose builds less reliable;
- built backend, frontend, and RAG worker images from the standalone project
  tree through `compose.single-server.yml`;
- ran the migration container against the current local single-server stack;
- recreated backend, frontend, and RAG worker from the build outputs;
- verified frontend root, same-origin `/mm-api/ready`, backend `/ready`, and
  RAG worker health;
- kept `scripts/release-images.sh` as an optional future registry-promotion
  helper only.

Changed surfaces:

```text
mm-chat/rag/Dockerfile
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
mm-chat/docs/deployment/release-rollback.md
mm-chat/docs/deployment/former-root-delete-plan.md
mm-chat/docs/inventory/standalone-cutover-gap.md
```

Verification:

```text
cd mm-chat
C:\Program Files\Docker\Docker\resources\bin\docker.exe compose \
  --project-directory <mm-chat-win-path> \
  --env-file <mm-chat-win-path>\.env.single-server \
  -f <mm-chat-win-path>\compose.single-server.yml \
  --profile app --profile rag-worker \
  build backend frontend rag-worker
  # passed; built mm-chat/backend:local, mm-chat/frontend:local, mm-chat/rag:local

C:\Program Files\Docker\Docker\resources\bin\docker.exe compose \
  --project-directory <mm-chat-win-path> \
  --env-file <mm-chat-win-path>\.env.single-server \
  -f <mm-chat-win-path>\compose.single-server.yml \
  --profile ops run --rm migrate
  # passed; no migrations changed

C:\Program Files\Docker\Docker\resources\bin\docker.exe compose \
  --project-directory <mm-chat-win-path> \
  --env-file <mm-chat-win-path>\.env.single-server \
  -f <mm-chat-win-path>\compose.single-server.yml \
  --profile app --profile rag-worker \
  up -d backend frontend rag-worker
  # passed; backend and rag-worker healthy, frontend started on 127.0.0.1:18080

curl -fsS http://127.0.0.1:18080/                       # passed
curl -fsS http://127.0.0.1:18080/mm-api/ready           # status=ready; checks=database,redis,storage
curl -fsS http://127.0.0.1:8080/ready                   # status=ready; checks=database,redis,storage
rag-worker internal health                              # {"status":"alive"}
```

Residual blockers:

```text
G10.4 still requires the separate exact owner approval phrase before any
former-root deletion. Registry push/digest deployment remains optional and is
not blocking the current build-based standalone closure.
```

## 2026-07-16 — G10.2b.1 Release Image Script

Objective: unblock the production immutable-env gate by adding a standalone
`mm-chat` image release script that can build all three runtime images and emit
the digest env lines required by production preflight after a registry push.

Completed scope:

- added `scripts/release-images.sh`;
- builds the Go backend/admin/migrate image from `backend/Dockerfile`;
- builds the server-mode Next.js frontend image from `frontend/Dockerfile` with
  `/mm-api` and `http://backend:8080` build defaults;
- builds the Python RAG worker image from `rag/Dockerfile`;
- defaults to safe local `--load` mode for smoke builds;
- requires explicit `--push` before publishing registry images;
- writes local metadata under `.release/images/<tag>/`, now gitignored;
- in `--push` mode, reads Docker Buildx metadata and emits
  `MM_CHAT_VERSION`, `BACKEND_IMAGE`, `FRONTEND_IMAGE`, and `RAG_IMAGE` lines
  with immutable `@sha256:` references;
- made `verify-standalone.sh` fall back to Windows Docker CLI for Compose
  topology rendering when WSL Docker integration is unavailable, while
  normalizing returned UNC build-context paths back to the clean-copy root;
- documented the workflow in `docs/deployment/release-rollback.md`;
- recorded G10.2b as split between the completed script and the still-pending
  production digest env proof;
- after Docker Desktop WSL integration was restored, ran a real local `--load`
  smoke build for backend, frontend, and RAG images;
- attempted the production GHCR `--push` path and stopped at registry
  authentication/authorization, proving the next blocker is package write access
  rather than the local build chain.

Verification:

```text
bash -n mm-chat/scripts/release-images.sh                    # passed
mm-chat/scripts/release-images.sh --dry-run --tag smoke-test  # printed backend/frontend/rag buildx commands
mm-chat/scripts/release-images.sh --dry-run --push --tag smoke-test
  # printed backend/frontend/rag buildx push commands
cd mm-chat && ./scripts/release-images.sh --load --tag g10-release-smoke
  # initially blocked: Docker daemon was not reachable / WSL integration disabled
cd mm-chat && ./scripts/release-images.sh --load --tag smoke-local
  # passed; images loaded:
  # ghcr.io/mumu-0922/neobot-mm-chat:smoke-local
  # ghcr.io/mumu-0922/neobot-mm-chat-frontend:smoke-local
  # ghcr.io/mumu-0922/neobot-mm-chat-rag:smoke-local
cd mm-chat && ./scripts/release-images.sh \
  --push --image-namespace ghcr.io/mumu-0922 --tag g10-standalone-f24d95a
  # blocked at GHCR: 403 Forbidden fetching anonymous token for push
bash mm-chat/scripts/verify-standalone.sh                    # passed (structure)
```

Historical residual blockers before the 2026-07-16 build-based decision:

```text
Registry push/digest env proof was considered the next hardening step at this
point. It was superseded later the same day when the owner selected source-build
Compose deployment as the active standalone gate.
```

## 2026-07-16 — G10.3b Browser Screenshot/Interaction Smoke

Objective: complete the remaining browser-backed desktop/mobile visual smoke
using the Windows Chrome binary available from WSL.

Completed scope:

- used Windows Chrome at
  `C:\Program Files\Google\Chrome\Application\chrome.exe`;
- avoided WSL UNC profile paths after Chrome rejected them; used Windows Temp
  for the Chrome profile and screenshot output;
- drove Chrome through a short CDP script run by Windows Python because WSL
  could not connect to Chrome's Windows loopback CDP port;
- captured desktop `1365x768` and mobile `390x844` screenshots before and
  after interaction;
- verified the app shell title, `新建对话`/existing chat context, composer
  placeholder `随便问点什么…`, model/provider control visibility, and controls
  count;
- clicked model, Knowledge, and search controls in both desktop and mobile
  contexts;
- visually confirmed the desktop citation card (`知识引用（1 条）`, `STRICT`,
  model `GPT-5.5`), desktop Knowledge management page, mobile navigation drawer,
  and server-mode search fail-closed toast;
- removed the temporary Chrome profile after capture while keeping PNG/summary
  evidence in Windows Temp.

Verification:

```text
C:\Program Files\Google\Chrome\Application\chrome.exe --headless=new
  # probe screenshot passed after moving profile/output to Windows Temp

Windows Python CDP smoke:
- desktop viewport: 1365x768
- mobile viewport: 390x844 (deviceScaleFactor=2, output 780x1688)
- desktop base: title `Neo Chat - 本地优先的 AI 对话工作台`, bodyLength=885,
  hasNewChat=True, hasSendPlaceholder=True, controlCount=63,
  composerTag=TEXTAREA, composerPlaceholder=`随便问点什么…`
- desktop interaction: typed=True, clickedModel=True, clickedKnowledge=True,
  clickedSearch=True
- mobile base: title `Neo Chat - 本地优先的 AI 对话工作台`, bodyLength=415,
  hasNewChat=True, hasSendPlaceholder=True, controlCount=39,
  composerTag=TEXTAREA, composerPlaceholder=`随便问点什么…`
- mobile interaction: typed=True, clickedModel=True, clickedKnowledge=True,
  clickedSearch=True, dialogLikeCount=1
```

Evidence files:

```text
C:\Users\Administrator\AppData\Local\Temp\mm-chat-g10-browser-smoke\cdp-win\summary.json
C:\Users\Administrator\AppData\Local\Temp\mm-chat-g10-browser-smoke\cdp-win\desktop-initial.png
C:\Users\Administrator\AppData\Local\Temp\mm-chat-g10-browser-smoke\cdp-win\desktop-after-interaction.png
C:\Users\Administrator\AppData\Local\Temp\mm-chat-g10-browser-smoke\cdp-win\mobile-initial.png
C:\Users\Administrator\AppData\Local\Temp\mm-chat-g10-browser-smoke\cdp-win\mobile-after-interaction.png
```

Historical residual blockers before the 2026-07-16 build-based decision:

```text
G10.3 was complete. The old production immutable-env closure note was
superseded later the same day by build-based Compose closure. G10.4 separate
owner-confirmed former-root cleanup remains.
```

## 2026-07-16 — G10.3a Automated UI/Visual Contract Smoke

Objective: cover the visual/interaction contract with available automated
frontend tests and HTTP app-shell smoke while browser screenshot automation is
blocked by the missing Chrome binary.

Completed scope:

- attempted Playwright MCP browser smoke; it could not start because Chrome is
  missing at `/opt/google/chrome/chrome`;
- ran targeted frontend tests covering mobile app shell accessibility, mobile
  metadata tooltip behavior, citation styles/cards, markdown HTML rendering,
  server Knowledge Base composition, selected Knowledge collection composition,
  model resolution, and server defaults/model visibility;
- fetched the running frontend root over HTTP and verified it returns a valid
  Next HTML shell with body and static asset references.

Verification:

```text
Playwright MCP navigate http://127.0.0.1:18080/
  # blocked: Chromium distribution 'chrome' is not found at /opt/google/chrome/chrome

cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/chatShellA11y.test.ts \
  src/__tests__/messageMetaTooltip.test.ts \
  src/__tests__/darkThemeTokens.test.ts \
  src/__tests__/citations.test.ts \
  src/__tests__/markdownHtmlRendering.test.tsx \
  src/__tests__/serverKnowledgeBaseComposition.test.ts \
  src/__tests__/serverKnowledgeSelectionComposition.test.ts \
  src/__tests__/modelUtils.test.ts \
  src/__tests__/serverDefaults.test.ts
  # 9 files passed, 70 tests passed

curl -fsS http://127.0.0.1:18080/
  # has_next_root=True
  # has_static_next=True
  # has_body=True
  # bytes=96881
```

Residual blockers:

```text
G10.3b still needs real browser desktop/mobile screenshot and interaction smoke
after Chrome/Chromium is available. This slice does not claim screenshot
coverage.
```

## 2026-07-16 — G10.1 Former-root Delete-plan Dry Run

Objective: prepare the destructive cleanup gate without deleting anything.

Completed scope:

- added `scripts/plan-former-root-deletion.sh` as a non-destructive dry-run
  script;
- limited generated deletion candidates to known legacy former-root application
  artifacts;
- protected `mm-chat/`, `.git`, `.trellis`, `.agents`, `.codex`, and `.vscode`
  from the generated delete block;
- made env/secret-like and unclassified top-level paths stop in manual-review
  sections instead of entering the delete block;
- added `docs/deployment/former-root-delete-plan.md` with required gates,
  approval phrase, execution steps, post-delete verification, and rollback;
- linked the plan from `docs/deployment/README.md`;
- expanded G10 into explicit G10.1-G10.4 slices.

Changed surfaces:

```text
mm-chat/scripts/plan-former-root-deletion.sh
mm-chat/docs/deployment/former-root-delete-plan.md
mm-chat/docs/deployment/README.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
bash -n mm-chat/scripts/plan-former-root-deletion.sh               # passed
bash mm-chat/scripts/plan-former-root-deletion.sh                  # dry-run only; no deletion performed
```

Residual blockers:

```text
G10.2 operations/backup/restore closure, G10.3 visual smoke, and G10.4
separate owner-confirmed destructive cleanup remain. The current script prints
the rm command block but never runs it.
```

## 2026-07-16 — G10.2a Local Live-stack Backup/Restore Smoke

Objective: prove the current local single-server stack can be backed up,
restored into disposable targets, restarted, and health-checked without touching
production data destructively.

Completed scope:

- attempted the production wrapper first; it correctly rejected the current
  local env because `FRONTEND_IMAGE` is missing, so this slice is local-stack
  smoke and not final production deletion approval;
- ran lower-level Postgres and MinIO backups with a temporary `BACKUP_DIR`;
- verified both generated checksums;
- restored the Postgres dump into disposable database
  `neo_chat_restore_drill_g10`;
- verified the restored database is readable, has 24 migration rows, and
  contains core chat, file, and Knowledge tables;
- exported five restored file object keys, then dropped the disposable restore
  database and verified no restore DB remains;
- restored the MinIO archive into a temporary bucket, counted 17 restored
  objects, checked five sampled object keys with `mc stat`, and removed the
  temporary bucket;
- rendered Compose config with app/ops/RAG profiles, confirmed required
  services, `NEXT_PUBLIC_API_MODE=server`, and `/mm-api`;
- smoke-tested frontend root, `/mm-api/ready`, and `/mm-api/v1/version`;
- restarted `backend` and `frontend`, then verified HTTP readiness and Docker
  health returned.

Verification:

```text
cd mm-chat && ./scripts/backup-single-server-production.sh .env.single-server
  # blocked as expected for local env: FRONTEND_IMAGE is required

BACKUP_DIR=/tmp/mm-chat-g10-backup-20260716T144525Z \
  ./scripts/backup-postgres.sh                         # created dump
BACKUP_DIR=/tmp/mm-chat-g10-backup-20260716T144525Z \
  ./scripts/backup-minio.sh                            # 17 objects / 317.50 KiB
sha256sum -c postgres-20260716T144525Z.dump.sha256     # OK
sha256sum -c minio-20260716T144525Z.tar.gz.sha256      # OK

Postgres restore drill:
- restore database: neo_chat_restore_drill_g10
- database_readable: 1
- schema_migrations: 24
- core table acceptance block: passed
- sampled object keys: 5
- restore DB dropped and verified absent

MinIO restore drill:
- restored_object_count=17
- knowledge_document_version_objects_checked=5
- cleanup=drill_bucket_removed

Compose/runtime smoke:
- rendered services: admin, backend, frontend, migrate, minio, minio-client,
  minio-init, postgres, rag-replay, rag-worker, redis
- missing required services: none
- frontend_api_mode=server
- frontend_api_base=/mm-api
- GET /mm-api/ready: ready with database/redis/storage ready
- GET /mm-api/v1/version: single-server-dev
- frontend root bytes: 96881
- docker compose restart backend frontend: completed
- post-restart readiness: passed
- backend/frontend Docker health: healthy
```

Cleanup:

```text
/tmp/mm-chat-g10-backup-20260716T144525Z removed
/tmp/mm-chat-g10-restore removed
/tmp/mm-chat-g10-ops removed
neo_chat_restore_drill_g10 absent after drill
```

Historical residual blockers before the 2026-07-16 build-based decision:

```text
The old production immutable-image requirement was superseded later the same
day by source-build Compose closure. G10.3 visual smoke and G10.4
owner-confirmed cleanup remained open at this point.
```

## 2026-07-16 — G9.6 Clean-copy Preflight

Objective: prove `mm-chat/` is independently verifiable from an isolated copy
and no longer needs former-root files, symlinks, absolute paths, or escaped
Compose build contexts.

Completed scope:

- ran the structure-only standalone gate from an isolated temp copy;
- ran the full standalone gate from an isolated temp copy;
- verified required manifests, Dockerfiles, frontend assets, backend module, and
  RAG package metadata exist inside `mm-chat/`;
- verified the clean copy contains no symlinks, no former-root absolute path
  references, and no Compose build contexts outside the copied project root;
- verified Compose renders the required `frontend`, `backend`, `postgres`,
  `redis`, `minio`, `minio-init`, `migrate`, `admin`, `rag-worker`, and
  `rag-replay` services with the frontend in server mode behind `/mm-api`;
- fixed clean-copy test drift only: refreshed the `clearAppData` storage mock
  and RAG fake gateways so they match the current server-mode/RAG contracts;
- normalized RAG Python formatting with `ruff format`.

Changed surfaces:

```text
mm-chat/frontend/src/__tests__/clearAppData.test.ts
mm-chat/rag/src/mm_chat_rag/*.py
mm-chat/rag/tests/integration/*.py
mm-chat/rag/tests/unit/*.py
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/inventory/standalone-cutover-gap.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
bash mm-chat/scripts/verify-standalone.sh                 # passed (structure)
bash mm-chat/scripts/verify-standalone.sh --full          # passed (full)

full gate details:
- frontend: pnpm install --frozen-lockfile, format:check, lint, typecheck,
  test, and server-mode build passed; Vitest reported 840 tests across
  180 files; build route table still shows 11 `/api/*` handlers.
- backend: gofmt -l ., go vet ./..., and go test ./... passed.
- rag: ruff check ., ruff format --check ., mypy src, and pytest passed;
  pytest reported 1699 passed and 7 skipped.

cd mm-chat/backend && go test ./...                       # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/clearAppData.test.ts \
  src/__tests__/opfsAuthority.test.ts                     # passed
cd mm-chat/rag && uv run ruff format --check .            # passed
cd mm-chat/rag && uv run ruff check .                     # passed
cd mm-chat/rag && uv run mypy src                         # passed
cd mm-chat/rag && uv run pytest \
  tests/unit/test_jina_gateway.py::test_jina_dependency_bundle_runs_admitted_embedding_handler \
  tests/unit/test_replay_worker.py::test_worker_auto_promotes_parse_stage_from_settings
                                                             # passed
```

Residual blockers:

```text
G9 is complete. G10 now owns the final operations/backup/restore/visual smoke
and exact former-root delete plan. No former-root deletion is authorized or
performed by G9.6.
```

## 2026-07-16 — G9.5c Direct IndexedDB Write Authority Fence

Objective: finish the G9.5 local write-authority slice by removing direct
non-import `appDb.setItem/removeItem` writes from chat runtime paths.

Completed scope:

- added `BrowserLocalIndexedDBAuthorityError` with code
  `BROWSER_LOCAL_INDEXEDDB_IMPORT_ONLY`;
- added `setRuntimeAppDbItem` and `removeRuntimeAppDbItem` as the only
  non-import IndexedDB write/delete helpers;
- made those helpers throw in `NEXT_PUBLIC_API_MODE=server`;
- replaced direct chat message `appDb.setItem/removeItem` calls in
  `chatStore.ts` with the runtime helpers;
- kept direct `appDb.getItem/keys` reads available for explicit local
  export/import compatibility;
- verified production source no longer has direct `appDb.setItem/removeItem`
  outside `storageConfig.ts`.

Changed surfaces:

```text
mm-chat/frontend/src/store/storage/storageConfig.ts
mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/__tests__/browserLocalAuthority.test.ts
mm-chat/frontend/src/__tests__/chatStore.test.ts
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/inventory/standalone-cutover-gap.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/browserLocalAuthority.test.ts \
  src/__tests__/chatStore.test.ts \
  src/__tests__/chatStoreServerRead.test.ts              # passed, 62 tests
cd mm-chat/frontend && corepack pnpm typecheck            # passed
cd mm-chat/frontend && corepack pnpm lint                 # passed
cd mm-chat/frontend && corepack pnpm format:check         # passed
cd mm-chat/frontend && corepack pnpm build                # passed; build route table still shows 11 `/api/*` handlers
rg 'appDb\\.(setItem|removeItem)' mm-chat/frontend/src \
  -g '!**/__tests__/**'                                   # only storageConfig authority helpers/adapters remain
git diff --check -- mm-chat                               # passed
```

Residual blockers:

```text
G9.6 clean-copy preflight remains. Browser-local direct reads for explicit
export/import are still allowed by design and are not write authority.
```

## 2026-07-16 — G9.5b OPFS Write/Delete Authority Fence

Objective: continue local production-authority removal by preventing OPFS
write/delete helpers from mutating browser-local file state when the frontend is
in server mode.

Completed scope:

- added `BrowserLocalOPFSAuthorityError` with code
  `BROWSER_LOCAL_OPFS_IMPORT_ONLY`;
- made `saveToOPFS`, `writeToOPFS`, `deleteFromOPFS`, and
  `deleteOPFSDirectory` throw the authority error when
  `NEXT_PUBLIC_API_MODE=server`;
- kept OPFS `listOPFSDirectory` and `readBlobFromOPFSUrl` available so explicit
  browser import can still enumerate and package local files;
- preserved explicit local mode OPFS write/delete behavior and covered it with
  regression tests;
- updated G9.5 planning docs to keep direct `appDb` write authority as G9.5c
  rather than claiming full local-authority removal.

Changed surfaces:

```text
mm-chat/frontend/src/utils/opfs.ts
mm-chat/frontend/src/__tests__/opfsAuthority.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/inventory/standalone-cutover-gap.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/opfsAuthority.test.ts \
  src/__tests__/opfs.test.ts                              # passed, 6 tests
cd mm-chat/frontend && corepack pnpm typecheck             # passed
cd mm-chat/frontend && corepack pnpm lint                  # passed
cd mm-chat/frontend && corepack pnpm format:check          # passed
cd mm-chat/frontend && corepack pnpm build                 # passed; build route table still shows 11 `/api/*` handlers
git diff --check -- mm-chat                                # passed
```

Residual blockers:

```text
G9.5c must sweep direct `appDb` IndexedDB writes that bypass Zustand persistence
storage. This slice does not claim full production local-authority removal.
```

## 2026-07-16 — G9.5a Zustand Persistence Authority Fence

Objective: start local production-authority removal by preventing persisted
Zustand stores from hydrating from or writing to browser-local IndexedDB and
`localStorage` when the frontend is built/run in server mode.

Completed scope:

- added `BrowserLocalPersistenceAuthority` in `storageConfig.ts`;
- made `NEXT_PUBLIC_API_MODE=server` resolve browser-local persistence authority
  to `import-only`;
- made `getAppDbStorage()` and `getBrowserLocalStorage()` return `noopStorage`
  in server mode, so Zustand persist adapters do not trigger legacy migrations,
  `appDb.getItem/setItem/removeItem`, or `window.localStorage` reads/writes;
- kept explicit browser-import direct reads available through `appDb` and OPFS
  for the one-time import flow;
- documented G9.5 as partially complete: direct `appDb` calls and OPFS
  write/delete helpers remain G9.5b scope.

Changed surfaces:

```text
mm-chat/frontend/src/store/storage/storageConfig.ts
mm-chat/frontend/src/__tests__/browserLocalAuthority.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/inventory/standalone-cutover-gap.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/browserLocalAuthority.test.ts \
  src/__tests__/appExport.test.ts \
  src/__tests__/browserImportPackage.test.ts \
  src/__tests__/chatStoreServerRead.test.ts                   # passed, 35 tests
cd mm-chat/frontend && corepack pnpm typecheck                 # passed
cd mm-chat/frontend && corepack pnpm lint                      # passed
cd mm-chat/frontend && corepack pnpm format:check              # passed
cd mm-chat/frontend && corepack pnpm build                     # passed; build route table still shows 11 `/api/*` handlers
git diff --check -- mm-chat                                    # passed
```

Residual blockers:

```text
G9.5b must sweep direct `appDb` and OPFS write/delete call sites that bypass
Zustand persistence storage. This slice does not claim full production
local-authority removal.
```

## 2026-07-16 — G9.4 Plugin/Agent Legacy Route Removal

Objective: delete the transitional Next plugin/agent route handlers after the
server-mode services already route list/install/execute/catalog calls through Go
`/v1/*`.

Completed scope:

- deleted `/api/agents`, `/api/agents/[identifier]`,
  `/api/plugins/list`, `/api/plugins/install`, and `/api/plugins/execute`
  route handlers from `src/app/api`;
- removed the legacy `pluginExecutionHttp` helper and the old route-handler
  tests that targeted the deleted Next implementations;
- changed local plugin and agent API adapters to fail closed with typed
  unsupported-feature errors instead of calling deleted Next routes;
- preserved server adapters for `/v1/agents*`, `/v1/plugins`,
  `/v1/plugins/install`, and `/v1/plugins/execute`;
- updated route inventory guard from 16 to 11 active transitional Next
  handlers and added explicit negative assertions for the G9.4-retired paths;
- refreshed frontend call-site, frontend API client, route inventory, gap,
  plan, and progress docs so plugin/agent compatibility is not documented as an
  active local Next route path.

Changed surfaces:

```text
mm-chat/frontend/src/app/api/agents/route.ts
mm-chat/frontend/src/app/api/agents/[identifier]/route.ts
mm-chat/frontend/src/app/api/plugins/list/route.ts
mm-chat/frontend/src/app/api/plugins/install/route.ts
mm-chat/frontend/src/app/api/plugins/execute/route.ts
mm-chat/frontend/src/services/api/client/local/agentApi.ts
mm-chat/frontend/src/services/api/client/local/pluginApi.ts
mm-chat/frontend/src/services/api/client/pluginExecutionHttp.ts
mm-chat/frontend/src/config/api.ts
mm-chat/frontend/src/lib/security/requestGuards.ts
mm-chat/frontend/src/__tests__/legacyRouteInventory.test.ts
mm-chat/frontend/src/__tests__/legacyPluginAgentRouteRemoval.test.ts
mm-chat/frontend/src/__tests__/agentService.test.ts
mm-chat/frontend/src/__tests__/pluginService.test.ts
mm-chat/frontend/src/__tests__/pluginUtils.test.ts
mm-chat/frontend/src/__tests__/accessControl.test.ts
mm-chat/frontend/src/__tests__/agentListRoute.test.ts
mm-chat/frontend/src/__tests__/pluginListRoute.test.ts
mm-chat/frontend/src/__tests__/pluginExecuteRoute.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/api-routes.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/inventory/standalone-cutover-gap.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/legacyRouteInventory.test.ts \
  src/__tests__/legacyPluginAgentRouteRemoval.test.ts \
  src/__tests__/agentService.test.ts \
  src/__tests__/pluginService.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/accessControl.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts              # passed, 119 tests
cd mm-chat/frontend && corepack pnpm typecheck                 # passed
cd mm-chat/frontend && corepack pnpm lint                      # passed
cd mm-chat/frontend && corepack pnpm format:check              # passed
cd mm-chat/frontend && corepack pnpm build                     # passed; build route table shows 11 `/api/*` handlers
git diff --check -- mm-chat                                    # passed
rg '(/api/agents|/api/plugins)' mm-chat/frontend/src \
  -g '!**/__tests__/**' -g '!node_modules'                     # no production references
```

Residual blockers:

```text
G9.5 owns browser-local production authority removal next:
IndexedDB/localforage/OPFS must become dev/import-only rather than a production
write authority.
No live browser screenshot is claimed for this static/API-client deletion slice.
```

## 2026-07-16 — G9.3 Config/Provider/BYOK Legacy Route Removal

Objective: delete the transitional Next config/provider/BYOK bootstrap routes
after the typed API client already routes server mode to Go `/v1/*`.

Completed scope:

- deleted `/api/config`, `/api/providers/models`, and
  `/api/byok/public-key` route handlers from `src/app/api`;
- updated local `settings`, `providers`, and `byok` API adapters to fail closed
  without calling deleted Next routes;
- preserved server adapters for `/v1/config`, `/v1/providers/models`, and
  `/v1/byok/public-key`;
- updated BYOK client tests to load public keys through the server-mode
  `/mm-api/v1/byok/public-key` path;
- updated route inventory guard from 19 to 16 active transitional Next
  handlers and added explicit negative assertions for the G9.3-retired paths;
- refreshed plan, inventory, provider-flow, and frontend API client contracts
  so local config/provider/BYOK compatibility is no longer documented as an
  active route path.

Changed surfaces:

```text
mm-chat/frontend/src/app/api/config/route.ts
mm-chat/frontend/src/app/api/providers/models/route.ts
mm-chat/frontend/src/app/api/byok/public-key/route.ts
mm-chat/frontend/src/services/api/client/local/settingsApi.ts
mm-chat/frontend/src/services/api/client/local/providerApi.ts
mm-chat/frontend/src/services/api/client/local/byokApi.ts
mm-chat/frontend/src/lib/security/requestGuards.ts
mm-chat/frontend/src/__tests__/legacyRouteInventory.test.ts
mm-chat/frontend/src/__tests__/legacyConfigProviderByokRouteRemoval.test.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/byok.test.ts
mm-chat/frontend/src/__tests__/byokRoutes.test.ts
mm-chat/frontend/src/__tests__/byokServices.test.ts
mm-chat/frontend/src/__tests__/serverDefaults.test.ts
mm-chat/frontend/src/__tests__/accessControl.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/api-routes.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/inventory/provider-flow.md
mm-chat/docs/inventory/standalone-cutover-gap.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/legacyRouteInventory.test.ts \
  src/__tests__/legacyConfigProviderByokRouteRemoval.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/byok.test.ts \
  src/__tests__/byokRoutes.test.ts \
  src/__tests__/byokServices.test.ts \
  src/__tests__/serverDefaults.test.ts \
  src/__tests__/accessControl.test.ts                         # passed, 125 tests
cd mm-chat/frontend && corepack pnpm typecheck                 # passed
cd mm-chat/frontend && corepack pnpm lint                      # passed
cd mm-chat/frontend && corepack pnpm format:check              # passed
cd mm-chat/frontend && corepack pnpm build                     # passed; build route table shows 16 `/api/*` handlers
git diff --check -- mm-chat                                    # passed
```

Residual blockers:

```text
G9.4 owns plugin/agent route retirement next. Remaining transitional Next API
handler count is 16.
No live browser screenshot is claimed for this static/API-client deletion slice.
```

## 2026-07-16 — G9.2 RAG/Doc-Parse Legacy Route Removal

Objective: delete the replaced transitional Next RAG/doc-parse handlers and
make old local service entrypoints fail closed instead of calling missing
routes.

Completed scope:

- deleted `/api/rag/query`, `/api/rag/upsert`, `/api/rag/delete`,
  `/api/doc-parse`, `/api/doc-parse/jobs/[id]`, and
  `/api/chat/rag-queries` route handlers from `src/app/api`;
- updated the route inventory guard from 25 to 19 active transitional Next
  handlers and added explicit negative assertions for the six G9.2-retired
  paths;
- changed `ragService.ts` and `docParseService.ts` into fail-closed
  compatibility shims so local-mode leftovers do not silently hit 404 routes;
- changed RAG query rewrite to use the original prompt instead of the retired
  `/api/chat/rag-queries` helper;
- stopped `clearBrowserAppData` from calling the retired `/api/rag/delete`
  endpoint; local reset now only clears local OPFS/localforage/app DB state;
- updated service/inventory docs so future work does not resurrect the retired
  handlers.

Changed surfaces:

```text
mm-chat/frontend/src/app/api/rag/**/route.ts
mm-chat/frontend/src/app/api/doc-parse/**/route.ts
mm-chat/frontend/src/app/api/chat/rag-queries/route.ts
mm-chat/frontend/src/services/api/ragService.ts
mm-chat/frontend/src/services/api/docParseService.ts
mm-chat/frontend/src/services/api/chatService.ts
mm-chat/frontend/src/lib/data/clearAppData.ts
mm-chat/frontend/src/lib/security/requestGuards.ts
mm-chat/frontend/src/config/api.ts
mm-chat/frontend/src/__tests__/legacyRouteInventory.test.ts
mm-chat/frontend/src/__tests__/legacyRagDocRouteRemoval.test.ts
mm-chat/frontend/src/__tests__/ragService.test.ts
mm-chat/frontend/src/__tests__/clearAppData.test.ts
mm-chat/frontend/src/__tests__/byokServices.test.ts
mm-chat/frontend/src/__tests__/byokRoutes.test.ts
mm-chat/frontend/src/__tests__/serverDefaults.test.ts
mm-chat/frontend/src/__tests__/docParseJobs.test.ts
mm-chat/frontend/src/services/README.md
mm-chat/frontend/src/components/knowledge/README.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/inventory/api-routes.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/inventory/standalone-cutover-gap.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
rm -rf mm-chat/frontend/.next                                  # cleared stale Next route type cache
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/legacyRouteInventory.test.ts \
  src/__tests__/legacyRagDocRouteRemoval.test.ts \
  src/__tests__/ragService.test.ts \
  src/__tests__/clearAppData.test.ts \
  src/__tests__/byokServices.test.ts \
  src/__tests__/byokRoutes.test.ts \
  src/__tests__/serverDefaults.test.ts                         # passed, 42 tests
cd mm-chat/frontend && corepack pnpm typecheck                 # passed
cd mm-chat/frontend && corepack pnpm lint                      # passed
cd mm-chat/frontend && corepack pnpm format:check              # passed
cd mm-chat/frontend && corepack pnpm build                     # passed; build route table shows 19 `/api/*` handlers
git diff --check -- mm-chat                                    # passed
```

Residual blockers:

```text
`src/lib/api/docParseJobs.ts` is now dead compatibility code and remains for
later local-authority cleanup. Server-mode Knowledge/RAG continues through the
Go API client and Python RAG path proven in G7/G8.
G9.3 owns config/provider/BYOK route retirement next.
No live browser screenshot is claimed for this static/service deletion slice.
```

## 2026-07-16 — G9.1 Route Inventory Freeze

Objective: freeze the current transitional Next API handler surface before G9
starts deleting handlers one domain at a time.

Completed scope:

- added a static inventory smoke test for all current
  `mm-chat/frontend/src/app/api/**/route.ts` handlers;
- locked the current 25 transitional route paths so accidental route drift
  fails before route-deletion slices silently change the standalone boundary;
- expanded the G9 sub-plan into deletion groups for RAG/doc-parse,
  config/provider/BYOK, plugin/agent, local authority removal, and clean-copy
  preflight.

Changed surfaces:

```text
mm-chat/frontend/src/__tests__/legacyRouteInventory.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/legacyRouteInventory.test.ts                    # passed, 1 test
cd mm-chat/frontend && corepack pnpm typecheck                  # passed
cd mm-chat/frontend && corepack pnpm lint                       # passed
cd mm-chat/frontend && corepack pnpm format:check               # passed
git diff --check -- mm-chat                                     # passed
```

Residual blockers:

```text
No route was deleted in G9.1. G9.2 owns deletion of replaced RAG/doc-parse
Next handlers and the nearby local-mode/service references.
No live browser screenshot is claimed for this static guard slice.
```

## 2026-07-15 — G0 Plan Freeze and Guardrails Completed

Objective: collect all remaining unfinished migration work into a new active
plan and establish the rule that each migrated group is tested immediately.

Owner directive:

- organize all remaining unfinished work into a new plan document;
- migrate one group at a time;
- test each group after migration;
- stop treating every small slice as one giant full-suite migration;
- create a new process document and keep the new plan/process as the active
  memory anchors.

Evidence inspected:

```text
mm-chat/docs/inventory/standalone-cutover-gap.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/app/api/**/route.ts
.trellis/tasks/07-07-mm-chat-server-refactor-design/prd.md
```

Remaining blockers captured:

- server-mode UI blockers in `ChatApp.tsx`: regeneration, message version
  switching, assistant presets, message editing, edit branches, message
  deletion, chat deletion, chat duplication, message retraction, smart rename,
  chat renaming, pinning, system instruction editing, and search toggle;
- 25 transitional frontend `/api/*` handlers still registered;
- unfinished domains: Auth UI lifecycle, Teams UI, Knowledge/RAG UI, Plugin
  final ownership, Provider Settings/BYOK, Agent catalogs, Voice, Image, Code
  Execution, Search, parser/RAG/citations, production local-mode removal,
  visual regression, backup/restore, clean-copy, and delete-plan gates.

Documents created or updated:

```text
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
mm-chat/docs/README.md
mm-chat/docs/architecture/README.md
mm-chat/docs/tracking/README.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/process.md
```

Verification completed for this documentation slice:

```text
git diff --check -- mm-chat/docs/...                         # passed
python3 link/path existence checks for the new docs           # passed
rg headings in new plan/process docs                          # passed
```

No runtime code was changed in G0, so frontend/backend tests were intentionally
not run for this documentation-only slice.

Next group: G1 Conversation and Message Operations.

## 2026-07-15 — G1.1 Conversation Metadata Operations Completed

Objective: remove the first server-mode blockers in G1 without changing the
frontend visual baseline.

Completed scope:

- server-backed chat deletion;
- server-backed chat renaming;
- server-backed pin/unpin;
- server-backed system instruction edit/delete.

Changed surfaces:

```text
mm-chat/backend/internal/chat/types.go
mm-chat/backend/internal/chat/service.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/chat/repository_postgres_test.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/chatApi.ts
mm-chat/frontend/src/services/api/client/server/chatApi.ts
mm-chat/frontend/src/services/api/chatCrudService.ts
mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/features/chat/hooks/useChatShellState.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/frontend/src/__tests__/chatAppServerModeComposition.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
```

Runtime contract:

```text
PATCH  /v1/chat/conversations/{id}
DELETE /v1/chat/conversations/{id}
```

`PATCH` accepts title, modelRef, systemInstruction, config/metadata merge, and
pinned. `DELETE` soft-deletes the conversation. Frontend server mode now calls
these contracts through typed client/service/store actions and no longer shows
unsupported toasts for chat deletion, chat renaming, pinning, or system
instruction editing.

Verification:

```text
cd mm-chat/backend && go test ./internal/chat                         # passed
cd mm-chat/backend && go test ./...                                   # passed
cd mm-chat/backend && go vet ./...                                    # passed
cd mm-chat/frontend && corepack pnpm typecheck                        # passed
cd mm-chat/frontend && corepack pnpm format:check                     # passed
cd mm-chat/frontend && corepack pnpm lint                             # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStoreServerRead.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts                  # 4 files / 61 tests passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend          # passed
curl -fsS http://127.0.0.1:8080/ready                                 # ready
curl -fsS http://127.0.0.1:18080                                      # frontend ok
HTTP smoke: create -> PATCH title/systemInstruction/pinned/config ->
  list -> DELETE -> list                                               # passed; delete_status=204, listed_after=0
```

Operational note: Compose commands for this stack must include
`--env-file .env.single-server`; running without it falls back to empty Team
cursor keyring defaults and the backend correctly refuses `AUTH_MODE=required`.
No secret values were recorded.

Residual G1 blockers:

```text
regeneration
message version switching
assistant presets
message editing
message edit branches
message deletion
chat duplication
message retraction
smart rename
```

Next slice: G1.2 Message deletion and retraction.

## 2026-07-15 — G1.2 Message Deletion and Retraction Completed

Objective: remove the server-mode blockers for deleting a single message and
retracting from a selected message onward, without changing the frontend visual
baseline.

Completed scope:

- server-backed single message deletion;
- server-backed message retraction using `scope=subsequent`;
- frontend server-mode actions now call Go through the typed client/service/
  store path instead of showing unsupported toasts for these two controls.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/chat/errors.go
mm-chat/backend/internal/chat/types.go
mm-chat/backend/internal/chat/service.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/chat/repository_postgres_test.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/chatApi.ts
mm-chat/frontend/src/services/api/client/server/chatApi.ts
mm-chat/frontend/src/services/api/chatCrudService.ts
mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/features/chat/hooks/useChatShellState.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/frontend/src/__tests__/chatAppServerModeComposition.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
```

Runtime contract:

```text
DELETE /v1/chat/conversations/{conversationId}/messages/{messageId}
DELETE /v1/chat/conversations/{conversationId}/messages/{messageId}?scope=subsequent
```

`scope` is optional. Empty or `message` soft-deletes only the selected message.
`subsequent` soft-deletes the selected message and all later messages by
conversation `sequence_no`. Unknown scopes fail closed with
`INVALID_DELETE_SCOPE`; missing or already-deleted messages map to
`MESSAGE_NOT_FOUND`.

Verification:

```text
cd mm-chat/backend && go test ./...                                   # passed
cd mm-chat/backend && go vet ./...                                    # passed
cd mm-chat/frontend && corepack pnpm typecheck                        # passed
cd mm-chat/frontend && corepack pnpm format:check                     # passed
cd mm-chat/frontend && corepack pnpm lint                             # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStoreServerRead.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts                  # 4 files / 63 tests passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend          # passed
curl -fsS http://127.0.0.1:8080/ready                                 # ready
curl -fsS http://127.0.0.1:18080                                      # frontend ok
HTTP smoke: create conversation -> append 3 messages ->
  DELETE second message -> list first+third ->
  DELETE first message with scope=subsequent -> list empty             # passed; 204/204
```

Smoke identifiers:

```text
conversation=8d517d55-7a7e-42dd-bd0e-612c77643b9f
single delete target=6b57d286-f579-4480-bec1-e51d31c80828
after single delete=7192a3be-760d-4433-a669-1f3c7a43980e,c5cdeabf-8115-4669-92ba-9bdeed9fc47a
after subsequent delete=<empty>
```

Residual G1 blockers:

```text
regeneration
message version switching
assistant presets
message editing
message edit branches
chat duplication
smart rename
```

Next slice: G1.3 Message editing and edit branches.

## 2026-07-15 — G1.3 Message Editing and Edit Branches Completed

Objective: remove the server-mode blockers for editing rendered model-message
content and creating edited user-message branches, while keeping the existing
chat UI controls and visual baseline unchanged.

Completed scope:

- server-backed `PATCH` for message content edits;
- frontend server-mode model-message edit now calls Go through the typed
  client/service/store path;
- frontend server-mode user-message edit creates a new persisted branch user
  message instead of mutating the original message;
- server message listing now carries enough parent metadata for the frontend
  server snapshot to reconstruct active branch state.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/chat/types.go
mm-chat/backend/internal/chat/service.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/chat/repository_postgres_test.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/chatApi.ts
mm-chat/frontend/src/services/api/client/server/chatApi.ts
mm-chat/frontend/src/services/api/chatCrudService.ts
mm-chat/frontend/src/lib/chat/types.ts
mm-chat/frontend/src/lib/chat/messageTree.ts
mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/features/chat/hooks/useChatShellState.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/frontend/src/__tests__/chatAppServerModeComposition.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
```

Runtime contract:

```text
PATCH /v1/chat/conversations/{conversationId}/messages/{messageId}
```

The request accepts `{"content":"..."}` only. Empty content fails closed with
`EMPTY_CONTENT`; absent editable fields fail with `NO_MESSAGE_UPDATES`; missing
or deleted messages map to `MESSAGE_NOT_FOUND`. The update clears stale
`output_blocks` so edited model text is rendered from the new canonical
content. Edited user-message branches continue to use
`POST /v1/chat/conversations/{id}/messages` with:

```json
{
  "parentMessageId": "previous-active-parent-when-present",
  "metadata": {
    "branchSourceMessageId": "original-user-message-id",
    "treeParentMessageId": null
  }
}
```

For root-level user branches, `treeParentMessageId:null` is intentional and is
used by the frontend server snapshot to keep sibling branches reconstructable.

Verification:

```text
cd mm-chat/backend && go test ./internal/chat                         # passed
cd mm-chat/backend && go test ./...                                   # passed
cd mm-chat/backend && go vet ./...                                    # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStoreServerRead.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts                                   # 6 files / 75 tests passed
cd mm-chat/frontend && corepack pnpm typecheck                        # passed
cd mm-chat/frontend && corepack pnpm format:check                     # passed
cd mm-chat/frontend && corepack pnpm lint                             # passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend          # passed
curl -fsS http://127.0.0.1:8080/ready                                 # ready
curl -fsS http://127.0.0.1:18080                                      # frontend ok
HTTP smoke: create conversation -> append root user message ->
  PATCH message content -> append edited root branch with
  branchSourceMessageId/treeParentMessageId metadata -> list messages  # passed
```

Smoke identifiers:

```text
conversation=0a272093-8e18-499f-a489-9245b0220c0b
patched message=2953f2de-7bdf-4945-8a32-948ea3384de0
branch message=bf8e73be-1923-4fd5-950f-cc23d22406bd
patched_content=patched root
patched_outputBlocks=0
branch_parent=<empty>
branch_treeParent=null
listed_ids=2953f2de-7bdf-4945-8a32-948ea3384de0,bf8e73be-1923-4fd5-950f-cc23d22406bd
listed_contents=patched root|edited branch root
```

Residual G1 blockers:

```text
regeneration
message version switching
assistant presets
chat duplication
smart rename
```

Next slice: G1.4 Regeneration and message version switching.

## 2026-07-15 — G1.4 Regeneration and Message Version Switching Completed

Objective: remove the server-mode blockers for regenerating an assistant answer
and switching between assistant-message versions, without changing the existing
chat UI layout or interaction language.

Completed scope:

- server-mode regeneration now reuses the selected assistant message's parent
  user message and streams through the existing Go SSE contract instead of
  appending a duplicate user prompt;
- repeated regeneration creates sibling assistant branches under the same user
  message;
- server-mode message version switching updates only the active server read tree
  in memory and does not write browser-local IndexedDB/OPFS state;
- `ChatApp.tsx` no longer shows unsupported toasts for regeneration or message
  version switching.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/chat/handler_test.go

mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/frontend/src/__tests__/chatAppServerModeComposition.test.ts
```

Runtime contract:

```text
POST /v1/chat/conversations/{conversationId}/stream
```

No new public backend route was added for this slice. The frontend calls the
existing stream contract with the original parent user message id; the Go chat
runtime already persists the newly streamed assistant message as a later sibling
branch. This keeps regeneration semantics aligned with normal server streaming
and avoids a second mutation path.

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/chat # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...           # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go vet ./...            # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/chatStoreServerRead.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts                                      # passed
cd mm-chat/frontend && corepack pnpm typecheck                               # passed
cd mm-chat/frontend && corepack pnpm format:check                            # passed
cd mm-chat/frontend && corepack pnpm lint                                    # passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend                 # passed
curl -fsS http://127.0.0.1:8080/ready                                        # ready
curl -fsS http://127.0.0.1:18080                                             # frontend ok
```

Provider-cost note: this slice did not run a live external-provider HTTP stream
smoke, because the current Compose stack is configured with real provider
settings and a full regeneration smoke may spend external model quota. The
branching contract is covered by Go mock stream tests, frontend store/component
tests, and Compose readiness/frontend HTTP smoke.

Residual G1 blockers:

```text
assistant presets
chat duplication
smart rename
```

Cross-group blocker still paused by owner directive:

```text
search toggle
```

Next slice: G1.5 Chat duplication and assistant presets.

## 2026-07-15 — G1.5 Chat Duplication and Assistant Presets Completed

Objective: remove the server-mode blockers for duplicating chats and applying
assistant presets, while keeping the existing frontend visual baseline and
leaving Agent Catalog server ownership to G2.

Completed scope:

- added server-backed conversation duplication through Go;
- duplicated conversations copy metadata, system instruction, model ref,
  visible messages, message parent links, output blocks, and server attachment
  references;
- duplicated conversations are unpinned by default, and copied assistant
  messages strip operational `runId` metadata so old runs cannot be cancelled
  through the copy;
- server-mode chat duplication now calls the typed API/service/store path and
  loads the copied server snapshot instead of mutating IndexedDB;
- server-mode assistant preset selection now applies the resolved instruction
  to the current empty server conversation or creates a new server conversation;
- `ChatApp.tsx` no longer shows unsupported toasts for assistant presets or
  chat duplication.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/chat/types.go
mm-chat/backend/internal/chat/service.go
mm-chat/backend/internal/chat/repository_postgres.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/chat/repository_postgres_test.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/chatApi.ts
mm-chat/frontend/src/services/api/client/server/chatApi.ts
mm-chat/frontend/src/services/api/chatCrudService.ts
mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/features/chat/hooks/useChatShellState.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/frontend/src/__tests__/chatAppServerModeComposition.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts

mm-chat/docs/contracts/chat-crud-api.md
mm-chat/docs/contracts/frontend-api-client.md
```

Runtime contract:

```text
POST /v1/chat/conversations/{conversationId}/duplicate
```

Request body accepts optional `title` and `idempotencyKey`. If `title` is
omitted, Go uses `<source title> (Copy)`. Message IDs are regenerated and
parent-message links are remapped inside the copied conversation. Server file
attachment records are linked to the copied messages; file bytes are not
re-uploaded.

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/chat # passed with httptest socket permission
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...           # passed with httptest socket permission
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go vet ./...            # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/chatStoreServerRead.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts                                          # 6 files / 78 tests passed
cd mm-chat/frontend && corepack pnpm typecheck                               # passed
cd mm-chat/frontend && corepack pnpm format:check                            # passed
cd mm-chat/frontend && corepack pnpm lint                                    # passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend                 # passed
curl -fsS http://127.0.0.1:8080/ready                                        # ready
curl -fsS http://127.0.0.1:18080                                             # frontend ok
```

HTTP smoke identifiers:

```text
source=d27fd74d-198c-4c59-9df8-47a9bd3e068d
duplicate=0489beee-1ed7-4677-96f2-a7a07693a2c6
sourceMessage=f28e0ff0-9fd6-46ef-8e8f-ce404b36cab3
duplicateTitle=G1.5 duplicate smoke (Copy)
duplicatePinned=false
duplicateMessageCount=1
listedMessageCount=1
listedContent=copy me
frontendBytes=96436
```

Residual G1 blockers:

```text
smart rename
```

Cross-group blocker still paused by owner directive:

```text
search toggle
```

Next slice: G1.6 Smart rename / title generation through server-owned route.

## 2026-07-15 — G1.6 Smart Rename and Server-Owned Title Generation Completed

Objective: remove the final G1 server-mode blocker by moving smart rename/title
generation behind a Go-owned route that reads server conversation history.

Completed scope:

- added `POST /v1/chat/conversations/{conversationId}/title` in Go;
- the title route reads messages from Postgres and builds the title prompt on
  the server side;
- if no `modelRef` or provider is available, the route returns a normalized
  first-user-message fallback without spending external model quota;
- frontend server API/client/service/store now exposes server title generation;
- server-mode smart rename calls the Go route and updates the conversation title
  through the existing `PATCH /v1/chat/conversations/{id}` path;
- server-mode auto-title after the first streamed message uses the same Go route
  and only applies the title while the conversation title is still `New Chat`;
- `ChatApp.tsx` no longer shows an unsupported toast for smart rename.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/chatApi.ts
mm-chat/frontend/src/services/api/client/server/chatApi.ts
mm-chat/frontend/src/services/api/chatCrudService.ts
mm-chat/frontend/src/store/core/chatStore.ts
mm-chat/frontend/src/features/chat/hooks/useChatShellState.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStoreServerRead.test.ts
mm-chat/frontend/src/__tests__/chatAppServerModeComposition.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts

mm-chat/docs/contracts/chat-crud-api.md
mm-chat/docs/contracts/frontend-api-client.md
```

Runtime contract:

```text
POST /v1/chat/conversations/{conversationId}/title
```

Request body accepts optional `modelRef`. With `modelRef`, Go may call the
configured provider through the existing provider abstraction. Without
`modelRef`, Go returns the deterministic first-user-message fallback; the smoke
used this path to avoid external model cost.

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/chat # passed with httptest socket permission
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...           # passed with httptest socket permission
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go vet ./...            # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/chatStoreServerRead.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts                                          # 6 files / 79 tests passed
cd mm-chat/frontend && corepack pnpm typecheck                               # passed
cd mm-chat/frontend && corepack pnpm format:check                            # passed
cd mm-chat/frontend && corepack pnpm lint                                    # passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend                 # passed
curl -fsS http://127.0.0.1:8080/ready                                        # ready
curl -fsS http://127.0.0.1:18080                                             # frontend ok
```

HTTP smoke identifiers:

```text
conversation=6d2b8d8d-3954-4092-a77a-025140e832bb
message=1fbb866d-8b64-4802-9d8b-de8e3e0e32c8
title=Server title fallback smoke
updatedTitle=Server title fallback smoke
frontendBytes=96436
```

Residual G1 blockers:

```text
<none>
```

Cross-group blocker still paused by owner directive:

```text
search toggle
```

Next slice: G2 Title, Related Questions, and Agent/Assistant Catalogs.

## 2026-07-15 — G2 Related Questions and Agent/Assistant Catalogs Completed

Objective: replace the remaining G2 helper-generation and catalog dependencies
with server-owned contracts while keeping the existing UI surface unchanged.

Completed scope:

- added `POST /v1/chat/conversations/{conversationId}/related-questions` in
  Go;
- related-question generation now reads the conversation messages from Postgres
  and uses the latest user/assistant pair, instead of accepting browser-owned
  `history` or provider config;
- the related-question route returns `{ "questions": [] }` without external
  model cost when no `modelRef`, provider, or usable message pair is present;
- added a Go-owned Agent catalog service and routes:
  - `GET /v1/agents?locale=en|zh|ja`;
  - `GET /v1/agents/{identifier}?locale=en|zh|ja`;
- catalog list/detail responses are normalized server-side and invalid agent
  identifiers fail before any upstream registry request;
- frontend API client now exposes `agents` and server `relatedQuestions`
  methods;
- frontend `agentService` routes server-mode catalog list/detail through Go
  `/v1/*` routes and keeps the legacy Next `/api/agents*` path only for local
  mode rollback;
- server-mode post-generation related prompts now call the Go conversation
  route with the server conversation ID and no browser history payload;
- server mode fails closed with an empty related-question list when no
  conversation ID is available, instead of falling back to the transitional
  Next route.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/agents/types.go
mm-chat/backend/internal/agents/service.go
mm-chat/backend/internal/agents/handler.go
mm-chat/backend/internal/agents/handler_test.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/metrics.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/index.ts
mm-chat/frontend/src/services/api/client/mode.ts
mm-chat/frontend/src/services/api/client/local/agentApi.ts
mm-chat/frontend/src/services/api/client/local/chatApi.ts
mm-chat/frontend/src/services/api/client/server/agentApi.ts
mm-chat/frontend/src/services/api/client/server/chatApi.ts
mm-chat/frontend/src/services/api/agentService.ts
mm-chat/frontend/src/services/api/chatService.ts
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/__tests__/agentService.test.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/byokServices.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts

mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/chat-crud-api.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Runtime contracts:

```text
POST /v1/chat/conversations/{conversationId}/related-questions
GET  /v1/agents?locale=en|zh|ja
GET  /v1/agents/{identifier}?locale=en|zh|ja
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build \
  go test ./internal/agents ./internal/chat ./internal/httpserver            # passed with httptest socket permission
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...           # passed with httptest socket permission
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go vet ./...            # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/agentService.test.ts \
  src/__tests__/byokServices.test.ts \
  src/__tests__/chatAppServerModeComposition.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts                                         # 7 files / 69 tests passed
cd mm-chat/frontend && corepack pnpm typecheck                              # passed
cd mm-chat/frontend && corepack pnpm format:check                           # passed
cd mm-chat/frontend && corepack pnpm lint                                   # passed

cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend                # passed
curl -fsS http://127.0.0.1:8080/ready                                       # ready
curl -fsS http://127.0.0.1:18080 | wc -c                                    # 96436
```

HTTP smoke identifiers:

```text
conversation=d5b90cc2-f74e-46e3-8698-6ad37d10532d
message=39dee7c2-2f54-4396-a161-3735b86766df
relatedQuestions=[]
agentsStatus=200
agentsUnavailable=false
agentsCount=500
frontendBytes=96436
```

Residual G2 blockers:

```text
<none>
```

Cross-group blocker still paused by owner directive:

```text
search toggle
```

Next slice: G3 Auth, Runtime Config, Provider Settings, and BYOK.

## 2026-07-15 — G3.1 Runtime Config, Provider Model, BYOK Route Boundary Completed

Objective: open the first G3 server-owned runtime boundary before wiring UI:
Go owns public runtime config, server-default provider model listing, BYOK
public-key publication, and the frontend API client exposes the corresponding
local/server adapters.

Completed scope:

- added Go `runtimeconfig` service and handler;
- registered public `GET /v1/config` and `GET /v1/byok/public-key` routes;
- registered protected `POST /v1/providers/models` route;
- `GET /v1/config` publishes browser-safe provider/default deployment facts
  without serializing provider API keys;
- `POST /v1/providers/models` supports only `source:"server-default"` model
  lists from server config in this slice;
- plaintext provider secrets are rejected and custom BYOK provider model refresh
  remains fail-closed for a later G3 slice;
- frontend API client now has `auth`, `settings`, `providers`, and `byok`
  subclients with local rollback and server `/v1/*` shells;
- server Auth shell routes login/logout/me/invite/recovery/session-revoke to Go
  auth endpoints and sends Bearer only through explicit token input.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/config/config.go
mm-chat/backend/internal/runtimeconfig/types.go
mm-chat/backend/internal/runtimeconfig/service.go
mm-chat/backend/internal/runtimeconfig/handler.go
mm-chat/backend/internal/runtimeconfig/service_test.go
mm-chat/backend/internal/runtimeconfig/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/httpserver/metrics.go

mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/index.ts
mm-chat/frontend/src/services/api/client/local/http.ts
mm-chat/frontend/src/services/api/client/local/authApi.ts
mm-chat/frontend/src/services/api/client/local/settingsApi.ts
mm-chat/frontend/src/services/api/client/local/providerApi.ts
mm-chat/frontend/src/services/api/client/local/byokApi.ts
mm-chat/frontend/src/services/api/client/server/authApi.ts
mm-chat/frontend/src/services/api/client/server/settingsApi.ts
mm-chat/frontend/src/services/api/client/server/providerApi.ts
mm-chat/frontend/src/services/api/client/server/byokApi.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts

mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Runtime contracts opened in this slice:

```text
GET  /v1/config
POST /v1/providers/models          # server-default source only
GET  /v1/byok/public-key
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build \
  go test ./internal/runtimeconfig ./internal/config ./internal/httpserver  # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...          # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go vet ./...           # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts \
  src/__tests__/envExample.test.ts \
  src/__tests__/serverDefaults.test.ts                                     # 6 files / 80 tests passed
cd mm-chat/frontend && corepack pnpm typecheck                             # passed
cd mm-chat/frontend && corepack pnpm format:check                          # passed
cd mm-chat/frontend && corepack pnpm lint                                  # passed
```

Residual G3 blockers:

```text
G3.2 frontend Auth lifecycle UI still calls the legacy local access-password route.
G3.3 ChatApp and ProviderSettings still call transitional /api/config,
     /api/providers/models, and /api/byok/public-key directly outside the new
     API client boundary.
G3 custom provider BYOK decrypt/model refresh is fail-closed in Go until the UI
   adapter and secret-envelope handling slice.
```

Next slice: G3.2 Frontend Auth lifecycle wired to Go login/logout/me.

## 2026-07-15 — G3.2 Frontend Auth Lifecycle Gate Completed

Objective: wire frontend server-mode Auth lifecycle to Go login/logout/me while
preserving the local access-password rollback path.

Completed scope:

- added a browser `sessionStorage`-backed server auth session helper for the Go
  Bearer token returned by `POST /v1/auth/login`;
- server-mode HTTP client now injects `Authorization: Bearer <token>` from that
  runtime session into `/v1/*` requests;
- added `ServerAuthGate`, which verifies an existing token with `GET /v1/me`
  before mounting `ChatApp` and clears stale sessions on auth failure;
- `app/page.tsx` routes to `ServerAuthGate` only when
  `NEXT_PUBLIC_API_MODE=server` and `AUTH_MODE=required`;
- `AccessPasswordPage` retains the existing local `/api/access/verify` flow and
  adds a server-auth mode that sends `{ email, password }` to Go login;
- frontend Compose/env examples now expose `AUTH_MODE` to the frontend runtime
  so the SSR page can select the correct gate.

Changed surfaces for this slice:

```text
mm-chat/compose.single-server.yml
mm-chat/frontend/.env.example
mm-chat/frontend/src/app/page.tsx
mm-chat/frontend/src/components/app/AccessPasswordPage.tsx
mm-chat/frontend/src/components/app/ServerAuthGate.tsx
mm-chat/frontend/src/i18n/locales/en/AccessPassword.json
mm-chat/frontend/src/i18n/locales/ja/AccessPassword.json
mm-chat/frontend/src/i18n/locales/zh/AccessPassword.json
mm-chat/frontend/src/lib/security/serverAuthMode.ts
mm-chat/frontend/src/services/api/client/authSession.ts
mm-chat/frontend/src/services/api/client/server/httpClient.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/envExample.test.ts

mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Runtime flow:

```text
server mode + AUTH_MODE=required -> ServerAuthGate
ServerAuthGate existing token -> GET /v1/me -> ChatApp or clear session
ServerAuthGate login form -> POST /v1/auth/login -> sessionStorage token -> ChatApp
server API calls -> Authorization: Bearer <token>
local mode/access-password -> unchanged /api/access/verify + httpOnly cookie
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/envExample.test.ts \
  src/__tests__/accessControl.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts                                    # passed, 6 files / 85 tests
cd mm-chat/frontend && corepack pnpm typecheck                         # passed
cd mm-chat/frontend && corepack pnpm format:check                      # passed
cd mm-chat/frontend && corepack pnpm lint                              # passed
```

Residual G3 blockers:

```text
G3.3 ChatApp and ProviderSettings still call transitional /api/config,
     /api/providers/models, and /api/byok/public-key directly outside the new
     API client boundary.
G3.4 hosted/dev auth behavior and same-origin Compose smoke still pending.
```

Next slice: G3.3 Provider Settings/BYOK UI adapters through the API client.

## 2026-07-15 — G3.3 Provider Settings and BYOK API-Client Wiring Completed

Objective: remove direct server-mode UI calls to transitional Next runtime
config/provider/BYOK routes and route those flows through the API client
boundary.

Completed scope:

- `ChatApp` runtime config bootstrap now calls
  `createNeoChatApiClient().settings.getRuntimeConfig()`;
- `ChatApp` default server-provider model bootstrap now calls
  `createNeoChatApiClient().providers.listModels()`;
- `ProviderSettings` model refresh now calls the provider API client instead of
  posting directly to `/api/providers/models`;
- BYOK public-key loading now calls `createNeoChatApiClient().byok.getPublicKey()`;
- local adapter implementations remain the only code paths that call
  `/api/config`, `/api/providers/models`, or `/api/byok/public-key` for local
  rollback.

Changed surfaces for this slice:

```text
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/components/settings/ProviderSettings.tsx
mm-chat/frontend/src/lib/byok/client.ts

mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/byokServices.test.ts \
  src/__tests__/byok.test.ts \
  src/__tests__/serverDefaults.test.ts \
  src/__tests__/envExample.test.ts                                  # passed, 5 files / 84 tests
cd mm-chat/frontend && corepack pnpm typecheck                      # passed
cd mm-chat/frontend && corepack pnpm format:check                   # passed
cd mm-chat/frontend && corepack pnpm lint                           # passed
rg direct transitional config/provider/BYOK calls                   # only local adapters remain
```

Residual G3 blockers:

```text
G3.4 hosted/dev auth behavior and same-origin Compose smoke still pending.
```

Next slice: G3.4 Hosted/dev auth behavior and same-origin smoke.

## 2026-07-15 — G3.4 Hosted/Dev Auth Smoke Completed

Objective: verify hosted/dev auth behavior and same-origin `/mm-api` runtime
routing after the G3 auth/config/provider/BYOK wiring.

Completed scope:

- ran Compose build/start for backend and frontend with `.env.single-server`;
- verified current dev auth mode from `.env.single-server` exposes runtime config
  and allows unauthenticated chat reads as expected for development mode;
- attempted `AUTH_MODE=required` smoke and found backend startup failed because
  required auth also requires Team cursor keyring settings;
- added explicit `local-dev` Team cursor keyring defaults to the single-server
  Compose/backend env examples so required-mode local smoke can start;
- reran `AUTH_MODE=required` Compose smoke and verified same-origin `/mm-api`
  config stays public while chat routes return `401 UNAUTHENTICATED` without a
  Bearer token;
- verified the frontend home page renders the client AuthGate shell in required
  mode before `ChatApp` mounts;
- restored the original `.env.single-server` stack after the required-mode smoke.

Changed surfaces for this slice:

```text
mm-chat/compose.single-server.yml
mm-chat/backend/.env.example
mm-chat/docs/deployment/single-server-compose.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat && docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --build backend frontend             # passed
GET http://127.0.0.1:8080/ready                                          # 200
GET http://127.0.0.1:18080/                                              # 200, bytes=96504
GET http://127.0.0.1:18080/mm-api/v1/config                              # 200, deployment.mode=local
GET http://127.0.0.1:18080/mm-api/v1/chat/conversations                  # 200 in dev mode

AUTH_MODE=required docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d --force-recreate backend frontend    # passed after local-dev cursor keyring default
GET http://127.0.0.1:18080/                                              # 200, contains "Checking session"
GET http://127.0.0.1:18080/mm-api/v1/config                              # 200, deployment.mode=hosted
GET http://127.0.0.1:18080/mm-api/v1/chat/conversations                  # 401 UNAUTHENTICATED

docker compose --env-file .env.single-server \
  -f compose.single-server.yml up -d backend frontend                     # restored original stack
GET http://127.0.0.1:18080/mm-api/v1/config                              # 200, deployment.mode=local
GET http://127.0.0.1:18080/mm-api/v1/chat/conversations                  # 200 after restore

cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/envExample.test.ts \
  src/__tests__/apiClientScaffold.test.ts                                # passed, 2 files / 46 tests
cd mm-chat/frontend && corepack pnpm typecheck                            # passed
cd mm-chat/frontend && corepack pnpm format:check                         # passed
cd mm-chat/frontend && corepack pnpm lint                                 # passed
```

Residual G3 blockers:

```text
<none>
```

Next slice: G4 Plugin Registry, Install, and Execution Final Ownership.

## 2026-07-15 — G4.1 Server Plugin Tool Planning Boundary Completed

Objective: land the first plugin slice without taking all plugin ownership in
one bite: provider-side tool planning goes through Go, while browser plugin
execution remains bounded and explicitly transitional.

Completed scope:

- added `orchestrateServerPlugins` as the frontend server-mode bridge from
  active plugin selections to Go `/v1/chat/tools/plan`;
- offered only installed, active, enabled plugin functions to Go and failed
  closed on duplicate function names or unoffered planned calls;
- kept plugin auth values out of the Go planning request;
- executed planned calls sequentially through the existing hardened plugin
  execution helper and retained `success|error` status per call;
- appended plugin results as explicitly untrusted context capped at 64 KiB before
  the final Go chat stream;
- covered URL/body mapping and malformed successful response handling for the
  API-client `/v1/chat/tools/plan` adapter;
- split G4 into smaller remaining slices so registry, install, execute final
  ownership, and live smoke can each be migrated and tested separately.

Changed surfaces for this slice:

```text
mm-chat/frontend/src/services/api/serverPluginOrchestration.ts
mm-chat/frontend/src/__tests__/serverPluginOrchestration.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/serverPluginOrchestration.test.ts  # passed, 1 file / 9 tests
cd mm-chat/frontend && corepack pnpm typecheck     # passed
cd mm-chat/frontend && corepack pnpm format:check  # passed
cd mm-chat/frontend && corepack pnpm lint          # passed
```

Residual G4 blockers:

```text
G4.2 plugin registry/list adapter
G4.3 plugin install/custom-manifest adapter
G4.4 plugin execute final ownership
G4.5 live browser smoke
```

Next slice: G4.2 Plugin registry/list adapter.

## 2026-07-15 — G4.2 Plugin Registry/List Adapter Completed

Objective: migrate the plugin marketplace list read as its own bounded slice,
without mixing install or execution final ownership into the same change.

Completed scope:

- added `PluginApi.listAvailable()` to the frontend API-client contract;
- added a local adapter that preserves the existing `/api/plugins/list` rollback
  path only inside `client/local/pluginApi.ts`;
- added a server adapter that targets Go `/v1/plugins` and treats a missing or
  unavailable registry route as explicit `{ plugins: [], unavailable: true }`;
- routed `fetchApiGuruList()` through `createNeoChatApiClient().plugins` instead
  of direct component/service fetches;
- covered default local cache behavior, server-mode URL routing, unavailable
  registry degradation, and malformed successful server responses;
- confirmed direct `/api/plugins/list` usage is now limited to the local adapter,
  route tests, and static route constants.

Changed surfaces for this slice:

```text
mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/index.ts
mm-chat/frontend/src/services/api/client/local/pluginApi.ts
mm-chat/frontend/src/services/api/client/server/pluginApi.ts
mm-chat/frontend/src/services/api/pluginService.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/pluginService.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/pluginService.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts          # passed, 5 files / 67 tests
cd mm-chat/frontend && corepack pnpm typecheck # passed
cd mm-chat/frontend && corepack pnpm format:check # passed
cd mm-chat/frontend && corepack pnpm lint # passed
rg '"/api/plugins/list"|/api/plugins/list' mm-chat/frontend/src -n
# remaining direct route references: local adapter, static route constant, tests
```

Residual G4 blockers:

```text
G4.3 plugin install/custom-manifest adapter
G4.4 plugin execute final ownership
G4.5 live browser smoke
```

Next slice: G4.3 Plugin install/custom-manifest adapter.

## 2026-07-15 — G4.3 Plugin Install Adapter Completed

Objective: migrate plugin install and custom manifest install calls as a separate
slice, without claiming final plugin execution ownership.

Completed scope:

- extended `PluginApi` with `install({ plugin | customInput })`;
- kept the legacy `/api/plugins/install` route reachable only through the local
  API-client adapter for rollback/local mode;
- added a server adapter that targets Go `/v1/plugins/install` and converts a
  missing/unavailable route into recoverable `PLUGIN_INSTALL_UNAVAILABLE`;
- routed `installPlugin()` and `installCustomPlugin()` through the API client;
- verified server mode does not fall back to the Next install route when Go has
  no plugin install endpoint yet.

Changed surfaces for this slice:

```text
mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/pluginApi.ts
mm-chat/frontend/src/services/api/client/server/pluginApi.ts
mm-chat/frontend/src/services/api/pluginService.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/pluginService.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/pluginService.test.ts \
  src/__tests__/apiClientScaffold.test.ts  # passed, 2 files / 56 tests
cd mm-chat/frontend && corepack pnpm typecheck # passed
cd mm-chat/frontend && corepack pnpm format:check # passed
cd mm-chat/frontend && corepack pnpm lint # passed
rg '"/api/plugins/install"|/api/plugins/install' mm-chat/frontend/src -n
# remaining direct route references: local adapter, static route constant, tests
```

Residual G4 blockers:

```text
G4.4 plugin execute final ownership
G4.5 live browser smoke
```

Next slice: G4.4 Plugin execute final ownership.

## 2026-07-15 — G4.4 Plugin Execute API-Client Boundary Completed

Objective: centralize plugin execution behind the API client without breaking the
G4.1 server planning flow that still depends on the hardened transitional Next
execution route.

Completed scope:

- extended `PluginApi` with `execute({ payload })`;
- moved direct `/api/plugins/execute` fetch construction into
  `client/pluginExecutionHttp.ts`;
- routed `executePluginFunction()` through `createNeoChatApiClient().plugins`
  while preserving BYOK retry semantics by rebuilding the encrypted payload for
  each retry;
- kept both local and server adapters on the same bounded transitional execution
  route for this slice, matching the current Server Plugin Orchestration
  contract;
- added API-client coverage proving server-mode plugin execution uses the
  isolated transitional adapter rather than the Go `/mm-api` prefix;
- reclassified the remaining work so final route retirement is a separate G4.5
  slice instead of being hidden inside this adapter-boundary slice.

Changed surfaces for this slice:

```text
mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/local/pluginApi.ts
mm-chat/frontend/src/services/api/client/server/pluginApi.ts
mm-chat/frontend/src/services/api/client/pluginExecutionHttp.ts
mm-chat/frontend/src/utils/pluginUtils.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts # passed, 3 files / 61 tests
cd mm-chat/frontend && corepack pnpm typecheck # passed
cd mm-chat/frontend && corepack pnpm format:check # passed
cd mm-chat/frontend && corepack pnpm lint # passed
```

Residual G4 blockers:

```text
G4.5 plugin execute final ownership and transitional route retirement
G4.6 live browser smoke
```

Next slice: G4.5 Plugin execute final ownership and transitional route retirement.

## 2026-07-15 — G4.5a Go Plugin Execution Fail-Closed Admission Completed

Objective: remove production server-mode fallback to the transitional Next plugin
execution route without pretending the full Go plugin sandbox exists yet.

Completed scope:

- added Go `internal/plugins` handler with explicit routes:
  - `GET /v1/plugins` returns an empty unavailable registry response;
  - `POST /v1/plugins/install` fails closed with `PLUGIN_INSTALL_UNAVAILABLE`;
  - `POST /v1/plugins/execute` fails closed with
    `PLUGIN_EXECUTION_UNAVAILABLE`;
- registered `/v1/plugins` and `/v1/plugins/*` in the Go HTTP server and metrics
  path normalizer;
- kept `GET /v1/plugins` public for marketplace visibility, while install and
  execute stay behind normal auth middleware in required mode;
- changed the frontend server plugin adapter so server-mode execution posts to
  Go `/v1/plugins/execute` and no longer falls back to `/api/plugins/execute`;
- kept `/api/plugins/execute` only in the local adapter rollback path;
- updated the API-client contract and inventory to reflect the Go fail-closed
  execution boundary.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/httpserver/metrics.go
mm-chat/backend/internal/httpserver/metrics_test.go
mm-chat/frontend/src/services/api/client/server/pluginApi.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/pluginUtils.test.ts
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/plugins ./internal/httpserver                         # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts                  # passed, 3 files / 62 tests
cd mm-chat/frontend && corepack pnpm typecheck                     # passed
cd mm-chat/frontend && corepack pnpm format:check                  # passed
cd mm-chat/frontend && corepack pnpm lint                          # passed
```

Residual G4 blockers:

```text
G4.5b plugin execute sandbox implementation
G4.6 live browser smoke with a real plugin result
```

Next slice: G4.5b Plugin execute sandbox implementation.

## 2026-07-15 — G4.5b Minimal Go Plugin Execution Sandbox Completed

Objective: turn the Go plugin execution gate from pure fail-closed admission into
a minimal safe executor while preserving one-slice-at-a-time migration. This
slice intentionally does not claim persistent registry ownership yet.

Completed scope:

- changed Go BYOK public-key algorithm metadata to the frontend envelope contract
  `RSA-OAEP-256+A256GCM`;
- added Go BYOK `DecryptOptionalSecret` / `DecryptSecretEnvelope` support and
  wired plugin execution to the same runtime config service instance so
  ephemeral development keys do not drift;
- implemented Go `/v1/plugins/execute` for full manifest payloads:
  - validates the selected function is declared by the supplied plugin;
  - substitutes path parameters and appends GET query args;
  - rejects plaintext plugin auth;
  - decrypts BYOK `valueSecret` using `plugin:{pluginId}:auth` context;
  - applies bearer/oauth2/apiKey auth to header/query/body according to config;
  - blocks localhost/private/link-local outbound URLs and redirects by default;
  - enforces HTTP method allowlist, timeout, 2 MiB response cap, and generic JSON/text result normalization;
- kept id-only plugin execution fail-closed with `PLUGIN_REGISTRY_REQUIRED` until
  the registry-backed finalization slice;
- changed server-mode frontend plugin execution to send the full plugin manifest
  payload to Go; local mode still keeps `/api/plugins/execute` as rollback only.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/runtimeconfig/service.go
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/frontend/src/services/api/client/server/pluginApi.ts
mm-chat/frontend/src/utils/pluginUtils.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/pluginUtils.test.ts
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...          # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/runtimeconfig ./internal/plugins ./internal/httpserver       # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts                         # passed, 3 files / 62 tests
cd mm-chat/frontend && corepack pnpm typecheck                            # passed
cd mm-chat/frontend && corepack pnpm format:check                         # passed
cd mm-chat/frontend && corepack pnpm lint                                 # passed
git diff --check -- mm-chat                                               # passed
```

Residual G4 blockers:

```text
G4.5c registry-backed plugin execute finalization
G4.6 live browser smoke with a real plugin result
```

Next slice: G4.5c Registry-backed plugin execute finalization, or G4.6 live smoke
if the owner accepts full-manifest execution as the smoke baseline before
registry persistence.

## 2026-07-15 — G4.5c.1 Go Registry Id-only Bridge Completed

Objective: move server-mode plugin execution off full-manifest production
payloads without swallowing the full durable-registry scope in one slice.

Completed scope:

- added a Go plugin registry interface with an in-memory implementation seeded
  by the current built-in plugin definitions;
- changed `POST /v1/plugins/install` from pure fail-closed to register supplied
  plugin payloads in the Go registry and return the installed plugin;
- kept custom OpenAPI manifest install fail-closed with
  `PLUGIN_CUSTOM_INSTALL_UNAVAILABLE` until the durable conversion slice;
- changed id-only `POST /v1/plugins/execute` to resolve
  `pluginId/functionName` from the Go registry, while preserving full-manifest
  execution as compatibility;
- changed server-mode frontend plugin execution to send id-only payloads to Go
  and never fall back to `/api/plugins/execute`;
- preserved G4.5b sandbox controls: BYOK encrypted auth, private URL/redirect
  blocking, timeout, response cap, and generic result normalization.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/plugins/builtins.go
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/handler_test.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/frontend/src/utils/pluginUtils.ts
mm-chat/frontend/src/__tests__/pluginUtils.test.ts
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...       # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/plugins ./internal/httpserver                              # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts                       # passed, 3 files / 62 tests
cd mm-chat/frontend && corepack pnpm typecheck                          # passed
cd mm-chat/frontend && corepack pnpm format:check                       # passed
cd mm-chat/frontend && corepack pnpm lint                               # passed
git diff --check -- mm-chat                                             # passed
```

Residual G4.5c blockers:

```text
G4.5c.2 Postgres-backed plugin registry persistence
G4.5c.2 Custom OpenAPI manifest conversion in Go
G4.5c.2 Plugin audit metadata and built-in result normalizers
G4.6 live browser smoke with a real plugin result
```

Next slice: G4.5c.2 durable registry completion, kept separate so the migration
continues one tested group at a time.

## 2026-07-15 — G4.5c.2a Postgres Plugin Registry Persistence Completed

Objective: make the Go plugin registry durable for installed plugin payloads
without mixing in custom OpenAPI manifest conversion or built-in result
normalizers.

Completed scope:

- added migration `011_plugin_registry` with `plugin_registry` JSONB payload
  storage, installing-user audit reference, built-in flag, timestamps, and
  rollback SQL;
- added a Go `PostgresRegistry` implementing save/get/list over the durable
  table while overlaying built-in plugin definitions as authoritative entries;
- wired `cmd/api` to use the Postgres registry whenever `DATABASE_URL` provides
  a SQL DB, while local/dev without DB keeps the memory registry fallback;
- changed `GET /v1/plugins` to list Go registry plugins instead of returning an
  unavailable empty registry;
- rejected installed plugin attempts that reuse built-in ids with
  `PLUGIN_ID_RESERVED`;
- updated runtime public deployment health so a database-backed plugin registry
  reports as a shared store.

Changed surfaces for this slice:

```text
mm-chat/backend/migrations/011_plugin_registry.up.sql
mm-chat/backend/migrations/011_plugin_registry.down.sql
mm-chat/backend/internal/migration/plugin_registry_schema_test.go
mm-chat/backend/internal/plugins/repository_postgres.go
mm-chat/backend/internal/plugins/repository_postgres_test.go
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/runtimeconfig/service.go
mm-chat/backend/internal/runtimeconfig/service_test.go
mm-chat/backend/cmd/api/main.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...       # passed with escalated httptest port permission
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts \
  src/__tests__/serverDefaults.test.ts                                  # passed, 4 files / 85 tests
cd mm-chat/frontend && corepack pnpm typecheck                          # passed
cd mm-chat/frontend && corepack pnpm format:check                       # passed
cd mm-chat/frontend && corepack pnpm lint                               # passed
git diff --check -- mm-chat                                             # passed
```

Residual G4.5c blockers:

```text
G4.5c.2d Plugin audit metadata beyond installing-user persistence
G4.6 live browser smoke with a real plugin result
```

Next slice: G4.5c.2c built-in result normalizers or, if the owner wants runtime
confidence first, G4.6 smoke against a converted plugin.

## 2026-07-15 — G4.5c.2b Go OpenAPI Plugin Install Conversion Completed

Objective: move custom OpenAPI manifest conversion into the Go backend as its
own tested slice, without mixing in audit metadata cleanup or built-in result
normalizers.

Completed scope:

- added a Go OpenAPI/Swagger converter that reads `servers` or
  `host/schemes/basePath`, maps supported HTTP operations into plugin
  functions, carries path/query parameter schemas, and preserves apiKey/bearer
  auth declarations;
- changed `POST /v1/plugins/install` to accept raw custom OpenAPI JSON,
  bounded manifest URL fetches, and marketplace payloads with `manifestUrl` plus
  empty `functions`;
- routed manifest URL fetches through the Go outbound URL policy and redirect
  checks, with a 3 MiB manifest response cap and explicit install error codes;
- kept supplied full plugin payload registration intact, then registered
  converted plugins into the same memory/Postgres registry so server-mode
  execution can continue with `pluginId/functionName` only;
- updated route tests so custom manifest install is no longer fail-closed.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/plugins/openapi.go
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/handler_test.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/plugins ./internal/httpserver                              # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/plugins ./internal/httpserver ./internal/runtimeconfig \
  ./internal/migration ./cmd/api                                        # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...       # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts \
  src/__tests__/serverDefaults.test.ts                                  # passed, 4 files / 85 tests
cd mm-chat/frontend && corepack pnpm typecheck                          # passed
cd mm-chat/frontend && corepack pnpm format:check                       # passed
cd mm-chat/frontend && corepack pnpm lint                               # passed
git diff --check -- mm-chat                                             # passed
```

Residual G4.5c blockers:

```text
G4.5c.2d Plugin audit metadata beyond installing-user persistence
G4.6 live browser smoke with a real plugin result
```

Next slice: G4.5c.2c built-in result normalizers or G4.6 live smoke against a
converted plugin.

## 2026-07-15 — G4.5c.2c Go Built-in Plugin Result Normalizers Completed

Objective: move built-in plugin result normalization into Go as a narrow slice,
without touching audit metadata or live-smoke wiring.

Completed scope:

- normalized Jina Web Reader `{code:200,data.content}` payloads into readable
  markdown strings inside Go `/v1/plugins/execute`;
- normalized Agnes image responses into `{imageUrl,imageBase64,revisedPrompt,raw}`
  envelopes;
- normalized Agnes video status/result payloads into stable task/video/status,
  generation status, progress, media URL, error, and raw fields;
- normalized Unsplash `results[]` payloads into the compact image result array
  shape already expected by the frontend fallback path;
- kept local Next plugin execution normalizers unchanged for rollback/local mode.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/normalizers.go
mm-chat/backend/internal/plugins/normalizers_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/plugins # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...              # passed
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/pluginResponseNormalizers.test.ts \
  src/__tests__/pluginUtils.test.ts \
  src/__tests__/serverPluginOrchestration.test.ts                              # passed, 3 files / 17 tests
cd mm-chat/frontend && corepack pnpm typecheck                                  # passed
cd mm-chat/frontend && corepack pnpm format:check                               # passed
cd mm-chat/frontend && corepack pnpm lint                                       # passed
```

Residual G4.5c blockers:

```text
G4.5c.2d Plugin audit metadata beyond installing-user persistence
G4.6 live browser smoke with a real plugin result
```

Next slice: decide whether to add the remaining audit metadata or run G4.6 live
smoke first.

## 2026-07-15 — G4.6a Zero-cost Plugin Orchestration Smoke Harness Completed

Objective: add a reproducible smoke harness for the plugin final-ownership path
without spending external provider quota or relying on public plugin services.
This is intentionally not marked as the final browser/provider live smoke.

Completed scope:

- added an in-process backend smoke test that mounts real Go chat and plugin HTTP
  handlers on one mux;
- installs a custom OpenAPI weather plugin through `POST /v1/plugins/install`;
- plans a plugin call through `POST /v1/chat/tools/plan` using a fake provider;
- executes the installed plugin through id-only `POST /v1/plugins/execute` using
  a fake plugin HTTP transport;
- builds bounded untrusted plugin context and sends it through
  `POST /v1/chat/conversations/{id}/stream`;
- verifies the Go SSE stream completes and the assistant message is persisted
  with the plugin-derived answer.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/chat/plugin_orchestration_smoke_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/chat
# passed with escalated httptest loopback permission
```

Residual G4 blockers:

```text
G4.5c.2d Plugin audit metadata beyond installing-user persistence
G4.6b live browser/provider smoke with a real plugin result
```

Next slice: either define the remaining plugin audit metadata contract, or run
G4.6b once approved credentials/runtime are available.

## 2026-07-15 — G6.1 Server-mode Media Job Fail-closed Gates Completed

Objective: start G6 with a narrow frontend/server boundary slice. Do not enable
real voice, image-generation, or code-execution jobs yet; only prevent
server-mode fallthrough to transitional Next routes.

Completed scope:

- added disabled `voice`, `imageGeneration`, and `codeExecution` capability
  flags to the frontend API-client capability map;
- gated `chatService.executeCode()` in server mode so it returns an explicit
  unsupported error string instead of calling `/api/chat/execute-code`;
- gated `chatService.generateImage()` in server mode so it throws an explicit
  unsupported feature error instead of calling `/api/chat/generate-image`;
- gated `voiceService.transcribeAudio()` and non-browser
  `voiceService.synthesizeSpeech()` in server mode so they throw explicit
  unsupported feature errors instead of calling `/api/voice/*`;
- left browser-native speech recognition/synthesis behavior local-only and
  unchanged.

Changed surfaces for this slice:

```text
mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/mode.ts
mm-chat/frontend/src/services/api/chatService.ts
mm-chat/frontend/src/services/api/voiceService.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/byokServices.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/byokServices.test.ts \
  src/__tests__/chatCrudService.test.ts \
  src/__tests__/chatStreamService.test.ts \
  src/__tests__/fileService.test.ts                                       # passed, 5 files / 71 tests
cd mm-chat/frontend && corepack pnpm typecheck                            # passed
cd mm-chat/frontend && corepack pnpm format:check                         # passed
cd mm-chat/frontend && corepack pnpm lint                                 # passed
git diff --check -- mm-chat                                               # passed
```

Residual G6 blockers:

```text
G6.2 Voice synthesis/transcription Go job admission
G6.3 Image generation Go job admission
G6.4 Code execution Go job admission
G6.5 Job audit/rate-limit/cancel metadata and provider smoke
```

Next slice: G6.2 voice job admission contract, unless the owner chooses image
or code admission first.

## 2026-07-15 — G6.2 Voice Job Admission Routes Completed

Objective: add Go-owned voice job admission endpoints without enabling real
speech-to-text or text-to-speech execution yet. The goal is a typed,
fail-closed server boundary that can later receive executors, storage, audit,
rate-limit, and cancellation logic.

Completed scope:

- added `internal/voicejobs` with request/response DTOs, a fail-closed service,
  and a handler for `POST /v1/voice/transcribe` and
  `POST /v1/voice/synthesize`;
- `transcribe` validates multipart admission shape, required audio part, and
  supported provider identifiers before returning `VOICE_JOBS_UNAVAILABLE`;
- `synthesize` validates strict JSON, required text, and supported provider
  identifiers before returning `VOICE_JOBS_UNAVAILABLE`;
- registered both routes in the Go HTTP server and added metric-path
  normalization so the endpoints do not collapse into `__unknown__`;
- kept frontend `voice` capability disabled, so server-mode UI still fails
  closed from G6.1 until real execution is implemented.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/voicejobs/types.go
mm-chat/backend/internal/voicejobs/service.go
mm-chat/backend/internal/voicejobs/handler.go
mm-chat/backend/internal/voicejobs/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/httpserver/metrics.go
mm-chat/backend/internal/httpserver/metrics_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/voicejobs ./internal/httpserver                         # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...    # passed
git diff --check -- mm-chat                                          # passed
```

Residual G6 blockers:

```text
G6.3 Image generation Go job admission
G6.4 Code execution Go job admission
G6.5 Real voice executors, output storage, audit/rate-limit/cancel metadata,
and provider smoke
```

Next slice: G6.3 image-generation Go job admission, keeping the same
fail-closed pattern.

## 2026-07-15 — G6.3 Image Generation Admission Route Completed

Objective: add a Go-owned image-generation admission endpoint without enabling
real image generation, provider calls, object storage writes, or billing/audit
side effects.

Completed scope:

- added `internal/imagejobs` with server-only `modelRef + prompt` request DTOs,
  response DTOs, a fail-closed service, and a handler for
  `POST /v1/images/generations`;
- rejected legacy-style plaintext provider objects via strict JSON decoding;
- validated required `modelRef.providerId`, `modelRef.modelId`, prompt, prompt
  length, and image count before returning `IMAGE_JOBS_UNAVAILABLE`;
- registered `/v1/images/generations` in the Go HTTP server and metric-path
  normalizer;
- kept frontend `imageGeneration` capability disabled until real execution,
  storage, and audit controls are added.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/imagejobs/types.go
mm-chat/backend/internal/imagejobs/service.go
mm-chat/backend/internal/imagejobs/handler.go
mm-chat/backend/internal/imagejobs/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/httpserver/metrics.go
mm-chat/backend/internal/httpserver/metrics_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/imagejobs ./internal/httpserver                         # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...    # passed
git diff --check -- mm-chat                                          # passed
```

Residual G6 blockers:

```text
G6.4 Code execution Go job admission
G6.5 Real voice/image executors, output storage, audit/rate-limit/cancel
metadata, and provider smoke
```

Next slice: G6.4 code-execution admission, still fail-closed and sandbox-first.

## 2026-07-15 — G6.4 Code Execution Admission Route Completed

Objective: add a Go-owned code-execution admission endpoint without enabling
model-simulated execution, local sandbox execution, filesystem access, network
access, or billing/audit side effects.

Completed scope:

- added `internal/codejobs` with server-only `modelRef + language + code`
  request DTOs, response DTOs, a fail-closed service, and a handler for
  `POST /v1/code/executions`;
- rejected legacy-style plaintext provider objects via strict JSON decoding;
- validated required `modelRef.providerId`, `modelRef.modelId`, non-empty code,
  maximum code length, and supported language before returning
  `CODE_EXECUTION_UNAVAILABLE`;
- preserved original code text after validation so a future sandbox receives the
  exact submitted program rather than a trimmed copy;
- registered `/v1/code/executions` in the Go HTTP server and metric-path
  normalizer;
- kept frontend `codeExecution` capability disabled until a real sandbox,
  storage/audit, rate-limit, cancellation, and smoke path exist.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/codejobs/types.go
mm-chat/backend/internal/codejobs/service.go
mm-chat/backend/internal/codejobs/handler.go
mm-chat/backend/internal/codejobs/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/httpserver/metrics.go
mm-chat/backend/internal/httpserver/metrics_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/codejobs ./internal/httpserver                          # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...    # passed
git diff --check -- mm-chat                                          # passed
```

Residual G6 blockers:

```text
G6.5 Real voice/image/code executors, output storage, audit/rate-limit/cancel
metadata, sandbox policy, and provider smoke
```

Next slice: G6.5 executor/storage/audit design split; do not enable real code
execution without an explicit sandbox contract.

## 2026-07-15 — G6.5a Sanitized Job Admission Audit Completed

Objective: add a shared audit metadata seam for voice, image-generation, and
code-execution job admission without enabling real execution, storage writes,
rate limits, cancellation, or provider smoke.

Completed scope:

- added `internal/jobaudit` with job kind/status constants, sanitized event DTOs,
  recorder interface, recorder function adapter, user-id attachment from auth
  context, and recorder-failure wrapping;
- wired voice, image, and code fail-closed services to emit unavailable audit
  events before returning their existing unavailable errors;
- audit events include only `kind`, `status`, `userId`, `providerId`, `modelId`,
  `language`, and `reason`;
- audit events intentionally do not contain prompt text, submitted source code,
  synthesis text, or audio bytes;
- audit sink failure maps to `503 JOB_AUDIT_UNAVAILABLE`, preserving fail-closed
  behavior for future enabled executors.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/jobaudit/jobaudit.go
mm-chat/backend/internal/jobaudit/jobaudit_test.go
mm-chat/backend/internal/voicejobs/service.go
mm-chat/backend/internal/voicejobs/service_test.go
mm-chat/backend/internal/voicejobs/handler.go
mm-chat/backend/internal/voicejobs/handler_test.go
mm-chat/backend/internal/imagejobs/service.go
mm-chat/backend/internal/imagejobs/service_test.go
mm-chat/backend/internal/imagejobs/handler.go
mm-chat/backend/internal/imagejobs/handler_test.go
mm-chat/backend/internal/codejobs/service.go
mm-chat/backend/internal/codejobs/service_test.go
mm-chat/backend/internal/codejobs/handler.go
mm-chat/backend/internal/codejobs/handler_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/jobaudit ./internal/codejobs ./internal/imagejobs \
  ./internal/voicejobs ./internal/httpserver                         # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...    # passed
git diff --check -- mm-chat                                          # passed
```

Residual G6 blockers:

```text
G6.5b Shared job rate-limit and cancellation gates
G6.5c Real voice/image executors with output storage and provider smoke
G6.5d Code execution sandbox contract before any real executor is enabled
```

Next slice: G6.5b shared job rate-limit/cancel gate, still without enabling
real executors.

## 2026-07-15 — G6.5b Job Cancellation and Rate-limit Gate Completed

Objective: add the shared job-control boundary needed by future async
voice/image/code executors without enabling any real executor or cancellation
state mutation yet.

Completed scope:

- added `internal/jobcontrol` with `POST /v1/jobs/{jobId}/cancel` route parsing,
  job-id validation, and fail-closed service behavior;
- invalid job ids and unknown job-control subroutes return `404 NOT_FOUND`
  without echoing the raw identifier;
- valid cancellation requests return `501 JOB_CANCELLATION_UNAVAILABLE` until
  a durable job registry/cancellation store exists;
- registered `/v1/jobs/{jobId}/cancel` in the Go HTTP server and metric-path
  normalizer;
- added an HTTP-server regression proving job-control routes are covered by the
  existing global rate-limit middleware and return `429 RATE_LIMITED` when over
  limit.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/jobcontrol/service.go
mm-chat/backend/internal/jobcontrol/handler.go
mm-chat/backend/internal/jobcontrol/handler_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/httpserver/metrics.go
mm-chat/backend/internal/httpserver/metrics_test.go
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test \
  ./internal/jobcontrol ./internal/httpserver                         # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...    # passed
git diff --check -- mm-chat                                          # passed
```

Residual G6 blockers:

```text
G6.5c Real voice/image executors with output storage and provider smoke
G6.5d Code execution sandbox contract before any real executor is enabled
```

Next slice: G6.5c should start with real voice/image executor design or a
storage-only result artifact contract; code execution remains blocked until the
sandbox contract is explicit.

## 2026-07-15 — G6.5d Code Execution Sandbox Contract Completed

Objective: define the hard gate for real code execution before any runtime
executor is enabled. This is a contract-only slice; the server route remains
fail-closed and `codeExecution` capability remains disabled.

Completed scope:

- added `docs/contracts/code-execution-sandbox-contract.md` with the required
  seven-section code-spec structure;
- defined request/response signatures, sandbox boundaries, allowed audit fields,
  validation/error matrix, good/base/bad cases, required tests, and wrong vs
  correct execution flow;
- documented that model-simulated execution is not equivalent to sandboxed code
  execution;
- updated contract index and G6 progress ledgers.

Changed surfaces for this slice:

```text
mm-chat/docs/contracts/code-execution-sandbox-contract.md
mm-chat/docs/contracts/README.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
git diff --check -- mm-chat                                          # passed
```

Residual G6 blockers:

```text
G6.5c Real voice/image executors with output storage and provider smoke
```

Next slice: either implement storage-first voice/image result artifacts or defer
real provider execution until credentials and smoke target are available.

## 2026-07-15 — G6.5c.1 Storage-only Voice/Image Artifact Boundary Completed

Objective: create the backend storage seam that future real voice/image
executors must use for generated outputs, without enabling any provider call,
credential use, or quota-consuming live smoke.

Completed scope:

- added `internal/jobartifacts`, a small Go service that accepts future job
  result streams and stores them through the existing `files.Service.Upload`
  boundary;
- mapped image results to file purpose `image` and audio results to purpose
  `audio`;
- validated artifact kind, positive declared size, non-nil body, and matching
  `image/*` or `audio/*` content type before upload;
- sanitized display filenames and client job identifiers so executor outputs do
  not pass path fragments into file metadata;
- returned only compact artifact metadata (`fileId`, `purpose`, `contentType`,
  `size`) and kept generated bytes behind backend file/object storage;
- left `voice` and `imageGeneration` capabilities disabled and did not touch any
  real STT/TTS/image provider configuration or quota.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/jobartifacts/artifacts.go
mm-chat/backend/internal/jobartifacts/artifacts_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/jobartifacts # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                  # passed
git diff --check -- mm-chat                                                         # passed
```

Residual G6 blockers:

```text
G6.5c.2 Real voice executor with stored audio artifacts and configured-provider smoke
G6.5c.3 Real image executor with stored image artifacts and configured-provider smoke
```

Next slice: wire one real executor at a time behind this artifact boundary only
after an explicit live-provider smoke target and quota/credential approval.

## 2026-07-15 — G6.5c.2a Voice Executor Opt-in Seam Completed

Objective: add the Go service seam needed by future voice transcription and
speech-synthesis executors while keeping the default runtime fail-closed and
avoiding any real provider call, credential use, or quota-consuming smoke.

Completed scope:

- added a `voicejobs.Executor` interface with `Transcribe` and `Synthesize`
  methods;
- passed validated multipart audio metadata and stream handles from
  `/v1/voice/transcribe` into the service only after admission validation;
- added `WithExecutor` as the explicit opt-in gate, so the default service still
  returns `VOICE_JOBS_UNAVAILABLE`;
- added a sanitized `admitted` audit status and fail-closed audit gate requiring
  an explicit audit recorder before any configured voice executor can run;
- required an artifact store before any synthesis executor can run, returning
  `VOICE_ARTIFACT_STORE_UNAVAILABLE` before executor invocation when storage is
  absent;
- stored synthesized audio executor output through the G6.5c.1 artifact
  boundary and returned only compact artifact metadata;
- covered the seam with fake in-process executors/stores only. No live STT/TTS
  provider was called.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/voicejobs/types.go
mm-chat/backend/internal/jobaudit/jobaudit.go
mm-chat/backend/internal/voicejobs/service.go
mm-chat/backend/internal/voicejobs/service_test.go
mm-chat/backend/internal/voicejobs/handler.go
mm-chat/backend/internal/voicejobs/handler_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/jobaudit ./internal/voicejobs # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                # passed
git diff --check -- mm-chat                                                       # passed
```

Residual G6 blockers:

```text
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke
G6.5c.3 Real image executor with stored image artifacts and configured-provider smoke
```

Next slice: either implement the image executor opt-in seam or add the real
voice provider behind an explicit quota/credential approval gate.

## 2026-07-15 — G6.5c.3a Image Executor Opt-in Seam Completed

Objective: add the Go service seam needed by future image-generation executors
while keeping the default runtime fail-closed and avoiding any real image
provider call, credential use, or quota-consuming smoke.

Completed scope:

- added an `imagejobs.Executor` interface and explicit `WithExecutor` opt-in
  gate;
- required a configured image artifact store before any image executor can run,
  returning `IMAGE_ARTIFACT_STORE_UNAVAILABLE` before executor invocation when
  storage is absent;
- required an explicitly configured sanitized `admitted` audit recorder before
  executor invocation, so audit absence/failure prevents provider calls;
- stored generated image executor streams through the G6.5c.1 artifact boundary
  as `image` purpose files and returned only compact artifact metadata;
- added `docs/contracts/media-job-executor-seams.md` as the seven-section
  executable contract for voice/image executor gates;
- covered the seam with fake in-process executors/stores only. No live image
  provider was called.

Changed surfaces for this slice:

```text
mm-chat/backend/internal/imagejobs/types.go
mm-chat/backend/internal/imagejobs/service.go
mm-chat/backend/internal/imagejobs/service_test.go
mm-chat/backend/internal/imagejobs/handler.go
mm-chat/backend/internal/imagejobs/handler_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/README.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/contracts/media-job-executor-seams.md
mm-chat/docs/inventory/frontend-call-sites.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/imagejobs # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                # passed
git diff --check -- mm-chat                                                       # passed
```

Residual G6 blockers:

```text
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke
G6.5c.3b Real provider-backed image executor and authorized configured-provider smoke
```

Next slice: either add real provider code behind explicit quota/credential
approval or move to the next non-provider G6 hardening slice.

## 2026-07-15 — G6.5e Live Provider Smoke Authorization Gate Completed

Objective: add a reusable default-deny authorization gate for any future live
voice/image provider smoke, so executor seams cannot accidentally consume
supplier quota just because provider credentials exist.

Completed scope:

- added `internal/providersmoke`, a provider-free Go package that authorizes
  live provider smoke only when all required env values are present;
- required `MM_CHAT_PROVIDER_LIVE_SMOKE_ENABLED=true`, the exact approval text
  `I_UNDERSTAND_THIS_USES_REAL_PROVIDER_QUOTA`, a sanitized run id, and an exact
  `kind:providerId:modelId` target match;
- limited live-smoke target kinds to `voice.transcribe`, `voice.synthesize`, and
  `image.generate`;
- made authorization errors wrap a stable `ErrNotAuthorized` and expose only
  codes, not provider/model/prompt/credential values;
- documented the env keys in backend and single-server example env files only;
- added `docs/contracts/provider-live-smoke-authorization.md` as the
  seven-section executable contract for quota-consuming smoke gates.

Changed surfaces for this slice:

```text
mm-chat/backend/.env.example
mm-chat/.env.single-server.example
mm-chat/backend/internal/providersmoke/gate.go
mm-chat/backend/internal/providersmoke/gate_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/README.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/contracts/media-job-executor-seams.md
mm-chat/docs/contracts/provider-live-smoke-authorization.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/providersmoke # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                  # passed
git diff --check -- mm-chat                                                         # passed
```

Residual G6 blockers:

```text
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke
G6.5c.3b Real provider-backed image executor and authorized configured-provider smoke
```

Next slice: wire a real provider only after the owner chooses a provider target
and explicitly authorizes quota-consuming live smoke.

## 2026-07-15 — G6.5c.3b.1 OpenAI-compatible Image Executor Added, Live Smoke Blocked by Current Provider

Objective: after owner approval to use provider quota, add the real
OpenAI-compatible image executor and run an authorized live image smoke without
exposing provider secrets.

Completed scope:

- added `imagejobs.OpenAICompatibleExecutor`, posting to
  `/images/generations` with `model`, `prompt`, `n`, and optional `size`;
- accepted both `b64_json` provider responses and provider-hosted image URLs;
- converted provider image responses into `GeneratedImageResult` streams for
  the existing image artifact boundary;
- added fake-transport tests for request shape, Authorization header use,
  base64 decoding, URL image fetch, unsupported provider IDs, and non-leaky
  provider-status errors;
- added `TestLiveOpenAICompatibleImageGenerationSmoke`, which is skipped by
  default and only runs after the G6.5e live-smoke authorization gate passes;
- attempted live smoke with the owner-approved quota path.

Live smoke evidence:

```text
Normal sandbox smoke:
  result: blocked before provider by local proxy/socket sandbox

Escalated configured relay smoke:
  endpoint class: configured OpenAI-compatible relay
  result: provider reached, /v1/images/generations returned HTTP 404

Escalated direct OpenAI smoke with the same configured key:
  endpoint class: https://api.openai.com/v1
  result: provider reached, returned HTTP 401
```

No provider key, prompt body, response body, or `.env.single-server` content was
printed. No image artifact was produced because no image-capable endpoint/key
completed successfully.

Changed surfaces for this slice:

```text
mm-chat/.env.single-server.example
mm-chat/backend/.env.example
mm-chat/backend/internal/imagejobs/openai_compatible_executor.go
mm-chat/backend/internal/imagejobs/openai_compatible_executor_test.go
mm-chat/backend/internal/imagejobs/openai_compatible_live_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/contracts/media-job-executor-seams.md
mm-chat/docs/contracts/provider-live-smoke-authorization.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/imagejobs # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                # passed
git diff --check -- mm-chat                                                       # passed
```

Residual G6 blockers:

```text
G6.5c.3b.2 Authorized configured-provider image smoke passes against an image-capable key/endpoint
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke
```

Next slice: provide or configure an image-capable endpoint/key, or add a
provider-specific executor such as Agnes/Gemini if that is the intended image
supplier.

## 2026-07-15 — G6.5c.3b.2 Authorized OpenAI-Compatible Image Smoke Passed

Objective: verify the real OpenAI-compatible image executor against an
owner-approved image-capable provider without persisting or printing provider
credentials.

Official API contract checked:

- direct image generation uses `POST /v1/images/generations`;
- the request body is the OpenAI Images API shape: `model`, `prompt`, optional
  `n`, and optional `size`;
- `gpt-image-2` is an OpenAI image model target;
- GPT image generation responses provide generated image bytes via
  `data[].b64_json` by default.

Live smoke evidence:

```text
Configured endpoint class: OpenAI-compatible relay
Target: image.generate:openai:gpt-image-2
Result: passed
Stored artifact: /tmp/mm-chat-provider-smoke/1-generated-1.png
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/imagejobs -run TestLiveOpenAICompatibleImageGenerationSmoke -count=1 -v # passed
```

No provider key, `.env.single-server` content, prompt body beyond the existing
smoke-test prompt, provider response body, or generated image bytes were added
to the repository. The generated image artifact remains in `/tmp` only.

Completed G6 image-executor blockers:

```text
G6.5c.3b.2 Authorized configured-provider image smoke passes against an image-capable key/endpoint
```

Residual G6 blockers:

```text
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke
Route-wiring/capability-reopen slice for imageGeneration remains separate from this smoke.
```

## 2026-07-15 — G6.5c.3c Go Image Route Wired to Executor/Storage/Audit

Objective: connect the already verified OpenAI-compatible image executor to the
real Go HTTP route without enabling any browser-side fallback or exposing
provider secrets.

Completed scope:

- added `httpserver.WithImageJobService` so `/v1/images/generations` can be
  wired to a configured service instead of the default fail-closed handler;
- updated `cmd/api` to build an image job service with:
  - sanitized structured `job_audit` events;
  - OpenAI-compatible executor opt-in when `PROVIDER_TYPE` is OpenAI-compatible
    and server-only `PROVIDER_BASE_URL` plus `PROVIDER_API_KEY` are present;
  - `jobartifacts` storage through the existing backend `files.Service` when
    file repository and object store dependencies are both present;
- preserved fail-closed behavior when provider credentials, file metadata DB,
  or object storage are absent;
- mapped provider-side image failures to `502 IMAGE_PROVIDER_ERROR` without
  leaking prompts, provider bodies, or credentials.

Changed surfaces:

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/imagejobs/handler.go
mm-chat/backend/internal/imagejobs/handler_test.go
mm-chat/backend/internal/imagejobs/openai_compatible_executor.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/contracts/media-job-executor-seams.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/imagejobs ./internal/httpserver ./cmd/api # passed
git diff --check -- mm-chat                                                                                         # passed
```

Residual blockers:

```text
Frontend imageGeneration adapter/capability reopen remains separate.
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke remains open.
```

## 2026-07-15 — G6.5c.3d Frontend Server-Mode Image Adapter and Capability Reopen

Objective: reopen the server-mode image generation UI path only after the Go
route, real executor, artifact storage, and live smoke gates were proven.

Completed scope:

- added local/server `ImageGenerationApi` adapters under the frontend API-client
  boundary;
- enabled `imageGeneration` only when `createNeoChatApiClient()` is configured
  for server mode, while keeping `voice` and `codeExecution` disabled;
- changed `generateImage()` so server mode posts to Go
  `/v1/images/generations` instead of falling through to
  `/api/chat/generate-image`;
- mapped returned image artifact metadata to server-backed image attachments
  with bytes fetched through `/v1/files/{fileId}/content`;
- kept local mode on the transitional Next `/api/chat/generate-image` route.

Changed surfaces:

```text
mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/index.ts
mm-chat/frontend/src/services/api/client/local/imageApi.ts
mm-chat/frontend/src/services/api/client/server/imageApi.ts
mm-chat/frontend/src/services/api/chatService.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/byokServices.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
mm-chat/frontend/src/__tests__/skillInvocationWiring.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run src/__tests__/apiClientScaffold.test.ts src/__tests__/byokServices.test.ts # passed
cd mm-chat/frontend && corepack pnpm format:check # passed
cd mm-chat/frontend && corepack pnpm lint # passed
cd mm-chat/frontend && corepack pnpm typecheck # passed
cd mm-chat/frontend && corepack pnpm vitest run src/__tests__/apiClientScaffold.test.ts src/__tests__/byokServices.test.ts src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts src/__tests__/fileService.test.ts src/__tests__/skillInvocationWiring.test.ts # passed
cd mm-chat/frontend && corepack pnpm test # passed with sandbox escalation; ordinary sandbox blocks byok script child process with EPERM
```

Residual blockers:

```text
G6.5c.2b Real provider-backed voice executor and authorized configured-provider smoke remains open.
Search/RAG, code execution, final local-mode removal, and clean-copy deletion gates remain separate slices.
```

## 2026-07-15 — G6.5c.2b.1 OpenAI-Compatible Voice Executor, Route Wiring, and Smoke Harness

Objective: add a real voice-provider executor behind the existing Go voice job
admission/storage/audit seam without reopening frontend voice controls or
requiring live provider quota during normal tests.

Completed scope:

- added `voicejobs.OpenAICompatibleExecutor` for OpenAI-compatible voice APIs:
  - STT: multipart `POST /audio/transcriptions` with `file`, `model`, and
    optional provider language;
  - TTS: JSON `POST /audio/speech` with `model`, `input`, and `voice`;
- mapped provider non-2xx responses to sanitized `502 VOICE_PROVIDER_ERROR`
  without echoing provider bodies, synthesis text, audio bytes, or credentials;
- wired `cmd/api` to construct a voice job service from server-only
  `PROVIDER_BASE_URL` and `PROVIDER_API_KEY` when configured;
- required backend artifact storage before TTS executor calls, preserving the
  existing no-storage fail-closed behavior before quota consumption;
- added `httpserver.WithVoiceJobService` and route-level coverage for configured
  voice synthesis artifacts;
- added a gated live voice smoke harness under the existing
  `providersmoke` authorization contract. Normal test runs skip it unless the
  exact quota approval, run id, and voice target are configured.

Changed surfaces:

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/cmd/api/main_test.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/httpserver/server_test.go
mm-chat/backend/internal/voicejobs/handler.go
mm-chat/backend/internal/voicejobs/handler_test.go
mm-chat/backend/internal/voicejobs/openai_compatible_executor.go
mm-chat/backend/internal/voicejobs/openai_compatible_executor_test.go
mm-chat/backend/internal/voicejobs/openai_compatible_live_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/contracts/media-job-executor-seams.md
mm-chat/docs/contracts/provider-live-smoke-authorization.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/voicejobs ./internal/httpserver ./cmd/api # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./... # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/voicejobs -run TestLiveOpenAICompatibleVoiceSmoke -count=1 -v # skipped: provider live smoke disabled
```

Residual blockers:

```text
G6.5c.2b.2 Authorized configured-provider voice smoke remains open.
Frontend voice capability/adapter reopen remains separate; `voice` stays disabled in server mode.
```

## 2026-07-15 — G4.5c.2d Plugin Audit Metadata Beyond Installing-User Persistence

Owner direction before this slice: skip the voice real-provider smoke/key path
for now. `G6.5c.2b.2` remains open/deferred; work resumed on the smallest
remaining plugin finalization item.

Objective: add server-side plugin install/execute audit metadata without
recording secrets, argument values, plugin responses, or full outbound URLs.

Completed scope:

- added a Go plugin audit seam with sanitized `plugin.install` and
  `plugin.execute` admission events;
- recorded only bounded metadata: action/status, actor user id, plugin id,
  function name/count, call id, install/execute source, built-in/auth presence,
  argument count, request id/user-agent/IP when available, and host-only URL
  metadata;
- wired plugin install audit before registry mutation and plugin execute audit
  before outbound plugin HTTP execution;
- mapped configured audit sink failures to `503 PLUGIN_AUDIT_UNAVAILABLE`,
  fail-closing before registry writes or outbound plugin calls;
- added a Postgres audit recorder that writes configured-server events to
  `audit_logs` without a new migration; local/dev without `DATABASE_URL` keeps
  the existing memory registry behavior without a mandatory audit sink;
- wired `cmd/api` and `httpserver` so configured Postgres deployments use the
  new plugin audit recorder.

Changed surfaces:

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/plugins/audit.go
mm-chat/backend/internal/plugins/audit_postgres.go
mm-chat/backend/internal/plugins/handler.go
mm-chat/backend/internal/plugins/handler_test.go
mm-chat/backend/internal/plugins/repository_postgres_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/contracts/frontend-api-client.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/plugins ./internal/httpserver ./cmd/api # passed
```

Residual blockers:

```text
G4.6b Live browser/provider smoke remains open.
G6.5c.2b.2 Authorized configured-provider voice smoke remains deferred/skipped for now.
G9 still owns final removal of local-only transitional Next plugin routes.
```

## 2026-07-15 — G4.6b Live Deployed-Frontend Weather Plugin Smoke Completed

Objective: close the remaining plugin final-ownership smoke by proving one real
installed plugin can be planned by the configured provider, executed through Go,
passed as bounded untrusted context, streamed through Go, persisted, reloaded,
and cleaned up.

Runtime repair before smoke:

- the running backend image was stale and returned `404 NOT_FOUND` for
  `/v1/plugins`;
- rebuilt and restarted only `backend` and `frontend` with the existing
  `.env.single-server` file without reading or printing secrets;
- the rebuilt backend exposed `/v1/plugins`, but `plugin_registry` was missing
  until migration `011_plugin_registry` was applied;
- ran the migration by mapping the container's existing `DATABASE_URL` to the
  migrator-required `MIGRATION_DATABASE_URL`, without printing the connection
  string.

Smoke path:

```text
GET    http://127.0.0.1:18080/mm-api/v1/plugins
POST   http://127.0.0.1:18080/mm-api/v1/chat/conversations
POST   http://127.0.0.1:18080/mm-api/v1/chat/conversations/{id}/messages
POST   http://127.0.0.1:18080/mm-api/v1/chat/tools/plan
POST   http://127.0.0.1:18080/mm-api/v1/plugins/execute
POST   http://127.0.0.1:18080/mm-api/v1/chat/conversations/{id}/stream
GET    http://127.0.0.1:18080/mm-api/v1/chat/conversations/{id}/messages
DELETE http://127.0.0.1:18080/mm-api/v1/chat/conversations/{id}
```

Evidence:

```text
Docker Compose backend/frontend rebuild                        passed
backend/frontend health                                        healthy / healthy
migrate up                                                     up 011_plugin_registry
GET /mm-api/v1/plugins                                         200, built-ins include weather-gpt
Direct weather plugin service                                  200, Shanghai weather returned
Go tool plan via deployed frontend                             getCurrentWeather({location: Shanghai})
Go plugin execute via deployed frontend                        weather-gpt/getCurrentWeather, Shanghai result
Go final SSE via deployed frontend                             message.completed
Persisted message reload                                       completed assistant message present
Smoke conversation cleanup                                     DELETE success
Postgres plugin audit probe                                    plugin.execute|weather-gpt|getCurrentWeather|weathergpt.vercel.app
Artifact                                                       /tmp/mm-chat-g46b-live-smoke-20260715-190934-11ed9e5d.json
```

Final answer preview:

```text
上海当前天气为多云，气温 35.4°C，体感温度约 48.8°C，湿度 56%，降雨概率较低。
```

Playwright note:

```text
MCP browser runtime could not launch Chrome: /opt/google/chrome/chrome missing.
This slice therefore uses the accepted "deployed frontend" path through the
same-origin `/mm-api` proxy rather than a fresh visible screenshot. No claim is
made for visual-regression coverage.
```

Residual blockers:

```text
G4 is complete for server-owned plugin registry/install/execute and live smoke.
G9 still owns removal of replaced transitional Next `/api/plugins/*` routes.
```

## 2026-07-15 — Voice Provider Preference Captured: Free/Simple TTS First

Owner directive: keep the Voice real-provider gap remembered for later, but
prefer a free or effectively-free TTS path where "it just speaks" over a
premium voice provider.

Decision recorded:

- keep the existing Go `voicejobs.Executor` seam and `/v1/voice/*` routes;
- do not remove the OpenAI-compatible voice executor, but do not block on the
  current relay because its `/audio/speech` and `/audio/transcriptions` probes
  returned `404 page not found` earlier;
- future TTS enablement priority:
  1. local/internal Piper-style TTS service or executor, no external quota;
  2. official cloud free-tier API adapter if the owner accepts account/API key
     setup;
  3. OpenAI-compatible `/audio/*` only if the configured relay later supports
     voice endpoints;
- browser `speechSynthesis` may be used as a local playback fallback, but it is
  not enough to close server-owned stored-audio parity because it does not
  produce backend artifacts.

Residual blocker remains:

```text
G6.5c.2b.2 Authorized configured-provider voice smoke
G6.5c.2b.3 Free/simple TTS provider selection and smoke
```

## 2026-07-15 — Voice VPS Constraint Captured: Keep API Seam, Defer Free Hosted TTS

Owner refinement: storing or synthesizing voice on the VPS is not acceptable for
this deployment because storage is awkward and VPS performance is insufficient.
The backend voice seam should still stay in place because a free hosted API can
be connected later.

Decision update:

- do not prioritize a local/internal Piper-style TTS executor for the current
  VPS deployment;
- keep the existing Go `voicejobs.Executor` seam and `/v1/voice/*` routes as
  the intended future integration point for a free hosted TTS API;
- browser `speechSynthesis` remains a temporary local fallback/test guard
  because it is free, local to the browser, needs no API key, and consumes no
  backend storage or VPS CPU;
- server-owned stored-audio parity remains open/deferred until the free hosted
  API adapter is selected, wired, and smoked.

Code/test guard added:

```text
mm-chat/frontend/src/services/api/voiceService.ts
mm-chat/frontend/src/__tests__/byokServices.test.ts
```

Validation target:

```text
cd mm-chat/frontend && corepack pnpm vitest run src/__tests__/byokServices.test.ts
```

## 2026-07-15 — G7 Dedicated Plan and Process Started

G7 now uses a dedicated plan and process log so the RAG/citation cutover can be
executed slice-by-slice without mixing with prior G1-G6 migration history.

New authoritative G7 files:

```text
docs/architecture/g7-rag-citation-cutover-plan.md
docs/tracking/g7-rag-citation-process.md
```

Owner-locked headline: real MinerU + Jina + Postgres provider loop, Jina 1024,
all PDF classes, admin env/Docker-secret provider keys for automatic indexing,
selected-chat Knowledge query scope, strict refusal for unknowns, basic citation
cards first, and G9-owned legacy Next route deletion.

## 2026-07-16 — G8.1 Teams/Knowledge API Client Adapter Seam

Objective: start G8 with a narrow, non-visual adapter slice before touching the
current frontend screens.

Completed scope:

- extended `NeoChatApiClient` and `ApiCapabilities` with `teams` and
  `knowledge`;
- added typed Team DTOs and methods for create/list/get/update, members,
  membership leave, invites, and revoke;
- added typed Knowledge DTOs and methods for collection CRUD, document bind/list
  /get/content/version/reprocess/delete, collection consents, and query
  consents;
- kept local mode fail-closed for these two domains, avoiding accidental
  fallback to legacy root routes;
- wired server mode to Go `/v1/teams/*`, `/v1/knowledge/*`, and
  `/v1/me/knowledge/query-consents/*`;
- added route-shape and capability tests, plus updated existing service tests
  to include the expanded capability/client surface.

Changed surfaces:

```text
mm-chat/frontend/src/services/api/client/types.ts
mm-chat/frontend/src/services/api/client/index.ts
mm-chat/frontend/src/services/api/client/mode.ts
mm-chat/frontend/src/services/api/client/local/teamApi.ts
mm-chat/frontend/src/services/api/client/local/knowledgeApi.ts
mm-chat/frontend/src/services/api/client/server/teamApi.ts
mm-chat/frontend/src/services/api/client/server/knowledgeApi.ts
mm-chat/frontend/src/__tests__/apiClientScaffold.test.ts
mm-chat/frontend/src/__tests__/chatCrudService.test.ts
mm-chat/frontend/src/__tests__/chatStreamService.test.ts
mm-chat/frontend/src/__tests__/fileService.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run src/__tests__/apiClientScaffold.test.ts # passed, 55 tests
cd mm-chat/frontend && corepack pnpm vitest run src/__tests__/apiClientScaffold.test.ts src/__tests__/chatCrudService.test.ts src/__tests__/chatStreamService.test.ts src/__tests__/fileService.test.ts # passed, 70 tests
cd mm-chat/frontend && corepack pnpm lint # passed
cd mm-chat/frontend && corepack pnpm typecheck # passed
cd mm-chat/frontend && corepack pnpm format:check # passed
git diff --check -- mm-chat # passed
changed-file secret scan # passed
```

Residual blockers:

```text
G8.2 visible Teams UI shell/actions remains next.
G8.3-G8.5 Knowledge UI, consent UX, and browser isolation smoke remain open.
G9 still owns legacy local authority and replaced route deletion after visible
G8 flows pass.
```

## 2026-07-16 — G8.2 Teams Settings UI Shell and Actions

Objective: expose the existing Go Team control plane through the current
frontend theme without introducing a new route shell or redesign.

Completed scope:

- added a deep-linkable `settingsTab=teams` Settings tab;
- added `TeamSettings` with server-mode capability gating and local-mode
  fail-closed copy;
- wired visible actions through the frontend API client only: Team list, create,
  select/detail, rename, member list, member role update, invite list, create
  invite, revoke invite, and leave team;
- kept caller identity and authorization out of browser payloads; UI copy
  explicitly records that Go resolves authority from the authenticated session;
- added English, Simplified Chinese, and Japanese Team copy;
- added composition tests for the tab, i18n, API-client-only action wiring, and
  absence of caller identity fields.

Changed surfaces:

```text
mm-chat/frontend/src/components/settings/TeamSettings.tsx
mm-chat/frontend/src/components/settings/SettingsPage.tsx
mm-chat/frontend/src/lib/chat/panelUrlState.ts
mm-chat/frontend/src/i18n/locales/en.ts
mm-chat/frontend/src/i18n/locales/zh.ts
mm-chat/frontend/src/i18n/locales/ja.ts
mm-chat/frontend/src/i18n/locales/en/SettingsPage.json
mm-chat/frontend/src/i18n/locales/zh/SettingsPage.json
mm-chat/frontend/src/i18n/locales/ja/SettingsPage.json
mm-chat/frontend/src/i18n/locales/en/Team.json
mm-chat/frontend/src/i18n/locales/zh/Team.json
mm-chat/frontend/src/i18n/locales/ja/Team.json
mm-chat/frontend/src/__tests__/teamSettingsComposition.test.ts
mm-chat/frontend/src/__tests__/chatPanelUrlState.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run src/__tests__/teamSettingsComposition.test.ts src/__tests__/chatPanelUrlState.test.ts src/__tests__/settingsUiComposition.test.ts src/__tests__/settingsHealthPanel.test.ts # passed, 13 tests
cd mm-chat/frontend && corepack pnpm typecheck # passed
cd mm-chat/frontend && corepack pnpm lint # passed
cd mm-chat/frontend && corepack pnpm format:check # passed
cd mm-chat/frontend && corepack pnpm build # passed
git diff --check -- mm-chat # passed
```

Residual blockers:

```text
G8.3 Knowledge collection/document UI shell remains next.
G8.4 Consent UX and G8.5 browser isolation smoke remain open.
A live browser screenshot is still not claimed because this environment has no
working Chromium runtime.
```

## 2026-07-16 — G8.3 Knowledge Collection/Document UI Shell

Objective: expose the existing Go Knowledge collection/document control plane
through the current Knowledge Base surface while preserving the local-mode
rollback UI.

Completed scope:

- added a server-mode branch in `KnowledgeBase` that renders the Go-backed
  `ServerKnowledgeBase` only when both `knowledge` and `files` API
  capabilities are enabled;
- kept the existing local OPFS/IndexedDB Knowledge UI as the local-mode
  fallback path;
- added server collection list/create/update/delete controls with personal/team
  scope selection;
- added document upload -> server file upload -> Knowledge document bind flow;
- added document list/status, reprocess, and delete controls;
- made document delete immediately invisible in the UI with rollback on API
  failure;
- added fail-closed unsupported copy for missing server/files/knowledge
  capability and localized English, Simplified Chinese, and Japanese copy;
- added composition tests that prevent direct `/v1/knowledge`, `/api/rag`, or
  `/api/doc-parse` calls from the visible UI and prevent browser identity/ACL
  spoofing fields.

Changed surfaces:

```text
mm-chat/frontend/src/components/knowledge/KnowledgeBase.tsx
mm-chat/frontend/src/components/knowledge/ServerKnowledgeBase.tsx
mm-chat/frontend/src/i18n/locales/en/Knowledge.json
mm-chat/frontend/src/i18n/locales/zh/Knowledge.json
mm-chat/frontend/src/i18n/locales/ja/Knowledge.json
mm-chat/frontend/src/__tests__/serverKnowledgeBaseComposition.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/serverKnowledgeBaseComposition.test.ts \
  src/__tests__/knowledgeCitations.test.ts \
  src/__tests__/apiClientScaffold.test.ts                         # passed, 63 tests
cd mm-chat/frontend && corepack pnpm typecheck                    # passed
cd mm-chat/frontend && corepack pnpm lint                         # passed
cd mm-chat/frontend && corepack pnpm format:check                 # passed
cd mm-chat/frontend && corepack pnpm build                        # passed
git diff --check -- mm-chat                                      # passed
```

Residual blockers:

```text
G8.4 Consent UX remains open.
G8.5 personal/team visibility and selected-chat Knowledge browser smoke remains
open.
Legacy Next /api/rag/* and /api/doc-parse/* route deletion remains deferred to
G9.
No live browser screenshot is claimed in this environment.
```

## 2026-07-16 — G8.5 Frontend Knowledge Isolation Smoke

Objective: prove the visible frontend no longer uses browser-local Knowledge
selection to decide server-mode RAG scope.

Completed scope:

- changed `KnowledgeSelectionModal` to detect server mode through
  `createNeoChatApiClient`;
- in server mode, the modal lists Go-visible Knowledge collections via
  `apiClient.knowledge.listCollections({ limit: 100 })`, so Personal vs Team
  visibility is derived from backend auth/ACL state instead of local OPFS or
  IndexedDB state;
- server-mode selection emits collection-level Knowledge attachments only; file
  drilldown remains local-mode-only because Go strict RAG consumes selected
  collection IDs;
- preserved the existing local Knowledge selection UX for rollback/local mode;
- added a composition smoke proving selected Knowledge attachments flow into
  `selectedKnowledgeCollectionIds`, `ragStrict`, and `knowledgeStrict` config
  /metadata on the ChatApp server send/regeneration paths;
- asserted the visible selection UI does not hard-code `/v1/knowledge`,
  transitional `/api/rag`, or browser-supplied actor/owner/ACL fields.

Changed surfaces:

```text
mm-chat/frontend/src/components/knowledge/KnowledgeSelectionModal.tsx
mm-chat/frontend/src/__tests__/serverKnowledgeSelectionComposition.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/serverKnowledgeSelectionComposition.test.ts \
  src/__tests__/serverKnowledgeBaseComposition.test.ts \
  src/__tests__/knowledgeCitations.test.ts                         # passed, 12 tests
cd mm-chat/frontend && corepack pnpm typecheck                    # passed
cd mm-chat/frontend && corepack pnpm lint                         # passed
cd mm-chat/frontend && corepack pnpm format:check                 # passed
```

Residual blockers:

```text
G8 UI/control-plane wiring is complete. G9 owns production local-mode authority
removal and transitional Next /api route deletion.
No live browser screenshot is claimed in this environment.
```

## 2026-07-16 — G8.4 Knowledge Consent UX

Objective: expose collection and query processing-consent controls through the
current server-mode Knowledge UI without collecting provider keys in the
frontend.

Completed scope:

- added collection consent listing, grant, and revoke controls under the
  selected server Knowledge collection;
- gated collection consent writes on `permissions.manageConsent`; read-only
  users see fail-closed copy instead of spoofable authority controls;
- added current-user query consent listing, grant, and revoke controls in the
  server Knowledge surface;
- added MinerU/Jina presets for the locked admin-env-backed profile while
  keeping processor/endpoint/model/purpose/data-type fields editable for exact
  governance identities;
- kept provider credentials server-owned: the UI grants/revokes consent only
  and does not capture API keys, BYOK keys, or backend secret values;
- added fail-closed copy explaining that missing governance profile, provider
  config, or consent stops Go before any processor call;
- extended composition tests to require API-client-only consent wiring, no
  direct transitional route calls, no caller identity/ACL spoof fields, and no
  key/token inputs in the consent UI.

Changed surfaces:

```text
mm-chat/frontend/src/components/knowledge/ServerKnowledgeBase.tsx
mm-chat/frontend/src/i18n/locales/en/Knowledge.json
mm-chat/frontend/src/i18n/locales/zh/Knowledge.json
mm-chat/frontend/src/i18n/locales/ja/Knowledge.json
mm-chat/frontend/src/__tests__/serverKnowledgeBaseComposition.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/serverKnowledgeBaseComposition.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/messagesParity.test.ts                              # passed, 66 tests
cd mm-chat/frontend && corepack pnpm typecheck                    # passed
cd mm-chat/frontend && corepack pnpm lint                         # passed
cd mm-chat/frontend && corepack pnpm format:check                 # passed
cd mm-chat/frontend && corepack pnpm build                        # passed
git diff --check -- mm-chat                                      # passed
```

Residual blockers:

```text
G8.5 browser/server-mode isolation smoke remains open: personal vs team
visibility and selected-chat Knowledge scope through the visible frontend.
Legacy Next /api/rag/* and /api/doc-parse/* route deletion remains deferred to
G9.
No live browser screenshot is claimed in this environment.
```

## 2026-07-17 — G11.1 Owner parity: chat image understanding

Objective: restore the original project behavior where uploaded chat images are
visible to the selected model during server-mode streaming.

Completed scope:

- added a Go chat provider attachment contract and a resolver seam for
  server-backed message attachments;
- wired the HTTP server to resolve `image/*` message attachments from the
  existing file service/object store before provider streaming;
- forwarded resolved image bytes through normal and strict-RAG provider request
  paths without accepting attachments in the stream request body;
- encoded OpenAI-compatible chat-completions requests as multimodal user content
  with text plus `image_url` data URL parts when image attachments exist;
- added handler coverage proving the provider receives the image attachment and
  provider coverage proving the outbound OpenAI-compatible payload shape.

Changed surfaces:

```text
mm-chat/backend/internal/chat/provider.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/provider_openai_compatible.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/chat/provider_openai_compatible_test.go
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/chat ./internal/httpserver  # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                                  # passed
```

Residual blockers:

```text
G11.2 must delete the Team frontend path for true original single-user parity.
G11.3 must restore browser-configured provider/model-fetch parity before final
owner cleanup can resume.
```

## 2026-07-17 — G11.2 Owner parity: single-user Team removal

Objective: restore the original single-user product surface instead of leaving a
nonfunctional Team management page in the standalone frontend.

Completed scope:

- removed the Settings Team tab, `settingsTab=teams` URL value, and Team
  settings component from the visible frontend;
- deleted the Team locale bundle and Team-settings composition test that kept
  the multi-user UI alive;
- updated settings URL-state coverage so old Team deep links normalize away as
  invalid settings tabs;
- made server Knowledge collection creation Personal-only in the standalone UI
  so users are not asked for Team IDs;
- kept lower-level backend/API-client Team code untouched for this slice to
  avoid unsafe migration/schema deletion during an owner-facing UI fix.

Changed surfaces:

```text
mm-chat/frontend/src/components/settings/SettingsPage.tsx
mm-chat/frontend/src/components/settings/TeamSettings.tsx
mm-chat/frontend/src/components/knowledge/ServerKnowledgeBase.tsx
mm-chat/frontend/src/lib/chat/panelUrlState.ts
mm-chat/frontend/src/i18n/locales/{en,zh,ja}.ts
mm-chat/frontend/src/i18n/locales/{en,zh,ja}/SettingsPage.json
mm-chat/frontend/src/i18n/locales/{en,zh,ja}/Knowledge.json
mm-chat/frontend/src/i18n/locales/{en,zh,ja}/Team.json
mm-chat/frontend/src/__tests__/settingsUiComposition.test.ts
mm-chat/frontend/src/__tests__/chatPanelUrlState.test.ts
mm-chat/frontend/src/__tests__/serverKnowledgeBaseComposition.test.ts
mm-chat/frontend/src/__tests__/serverKnowledgeSelectionComposition.test.ts
mm-chat/frontend/src/__tests__/teamSettingsComposition.test.ts
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/settingsUiComposition.test.ts \
  src/__tests__/chatPanelUrlState.test.ts \
  src/__tests__/serverKnowledgeBaseComposition.test.ts \
  src/__tests__/serverKnowledgeSelectionComposition.test.ts             # passed, 17 tests
cd mm-chat/frontend && corepack pnpm typecheck                        # passed
cd mm-chat/frontend && corepack pnpm lint                             # passed
cd mm-chat/frontend && corepack pnpm format:check                     # passed
```

Residual blockers:

```text
G11.3 must restore original-style browser provider settings/model-fetch parity.
Backend Team schema/routes are intentionally deferred; they are no longer visible
through the standalone frontend after this slice.
```

## 2026-07-17 — G11.3 Owner parity: browser provider runtime flow

Objective: restore the original local single-user behavior where providers are
configured in the web UI and the selected provider is actually used for model
listing and chat streaming.

Completed scope:

- extended provider runtime DTOs with the browser provider ID so Go can validate
  selected custom provider model refs without collapsing them into the env
  default provider;
- added a chat runtime-provider resolver path: stream requests may include a
  BYOK-encrypted provider config, Go decrypts the API key in memory, constructs a
  temporary OpenAI-compatible provider, and streams with that provider;
- kept plaintext provider secrets rejected by Go; browser keys remain in local
  secret storage and are sent to Go only as BYOK envelopes for the request;
- changed `/v1/providers/models` from env-only to real OpenAI-compatible
  `/models` fetching for BYOK browser providers;
- made new web providers default to `OpenAI Compatible` with `/v1` base URL and
  auto-enabled fetched models when the provider had no selected model list yet;
- wired new message, edited-user-message resend, and assistant regeneration
  paths to pass the selected provider runtime config into the Go stream.

Changed surfaces:

```text
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/chat/provider_openai_compatible.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/runtimeconfig/{types.go,service.go,handler.go}
mm-chat/backend/internal/chat/handler_test.go
mm-chat/backend/internal/runtimeconfig/service_test.go
mm-chat/frontend/src/components/app/ChatApp.tsx
mm-chat/frontend/src/components/settings/ProviderSettings.tsx
mm-chat/frontend/src/lib/byok/client.ts
mm-chat/frontend/src/services/api/client/{types.ts,server/chatApi.ts}
mm-chat/frontend/src/store/core/{chatStore.ts,coreSettingsStore.ts}
mm-chat/frontend/src/__tests__/{apiClientScaffold.test.ts,byok.test.ts}
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/chat ./internal/runtimeconfig ./internal/httpserver  # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                                                # passed
cd mm-chat/frontend && corepack pnpm typecheck                                                                  # passed
cd mm-chat/frontend && corepack pnpm vitest run src/__tests__/apiClientScaffold.test.ts src/__tests__/byok.test.ts # passed, 64 tests
cd mm-chat/frontend && corepack pnpm lint                                                                       # passed
cd mm-chat/frontend && corepack pnpm format:check                                                               # passed
```

Residual blockers:

```text
Gemini browser-provider chat is still not reimplemented in Go; current runtime
provider streaming supports OpenAI/OpenAI-compatible endpoints, matching the
owner-provided `/v1` supplier path. Former-root deletion should wait for owner
browser smoke on image input, single-user settings, and provider model fetch/chat.
```

## 2026-07-17 — G11.3a BYOK public JWK WebCrypto compatibility

Objective: fix the owner-visible provider model-fetch failure where the browser
rejected the Go BYOK public key with `The JWK "alg" member was inconsistent
with that specified by the Web Crypto call`.

Completed scope:

- kept the application envelope response algorithm as
  `RSA-OAEP-256+A256GCM`;
- changed the embedded RSA public JWK `alg` to the WebCrypto-compatible
  `RSA-OAEP-256`;
- normalized legacy/cached BYOK public-key responses in the frontend before
  `crypto.subtle.importKey`;
- added backend and frontend regression coverage so old combined JWK alg values
  no longer break browser provider encryption.

Changed surfaces:

```text
mm-chat/backend/internal/runtimeconfig/service.go
mm-chat/backend/internal/runtimeconfig/service_test.go
mm-chat/frontend/src/lib/byok/client.ts
mm-chat/frontend/src/__tests__/byok.test.ts
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./internal/runtimeconfig  # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./...                     # passed
cd mm-chat/frontend && corepack pnpm vitest run src/__tests__/byok.test.ts             # passed, 10 tests
cd mm-chat/frontend && corepack pnpm typecheck                                         # passed
cd mm-chat/frontend && corepack pnpm lint                                              # passed
cd mm-chat/frontend && corepack pnpm format:check                                      # passed
git diff --check -- mm-chat                                                            # passed
```

Residual blockers:

```text
Requires rebuilding/restarting the local standalone stack so the browser loads
the fixed frontend bundle and backend BYOK public-key response. If a tab keeps
the old in-memory public-key promise, hard-refresh or reopen the tab.
```

## 2026-07-17 — G11.3b Browser provider preference persistence

Objective: fix the owner-visible regression where a custom browser-configured
provider and its fetched model list disappeared after refreshing the standalone
server-mode page.

Root cause:

- G9.5 correctly made browser-local IndexedDB/OPFS non-authoritative in server
  mode for chats, files, memory, and Knowledge state;
- the same fence accidentally made the core settings `localStorage` adapter a
  no-op in server mode;
- Provider Settings stores custom provider shells, selected/fetched models,
  theme, and language in `neo-chat-core-settings`, so those values survived
  only in memory until refresh.

Completed scope:

- added a dedicated browser preference storage adapter that remains available
  in server mode;
- switched `coreSettingsStore` to use that preference storage for
  `neo-chat-core-settings`;
- kept `getAppDbStorage()` and the generic browser-local runtime storage fence
  unchanged, so server-mode chat/Knowledge/local data authority remains blocked;
- added regression coverage proving server mode blocks app DB writes while
  still persisting `neo-chat-core-settings` through localStorage.

Changed surfaces:

```text
mm-chat/frontend/src/store/storage/storageConfig.ts
mm-chat/frontend/src/store/core/coreSettingsStore.ts
mm-chat/frontend/src/__tests__/browserLocalAuthority.test.ts
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/browserLocalAuthority.test.ts \
  src/__tests__/serverDefaultStores.test.ts \
  src/__tests__/byok.test.ts                                    # passed, 19 tests
cd mm-chat/frontend && corepack pnpm typecheck                  # passed
cd mm-chat/frontend && corepack pnpm lint                       # passed
cd mm-chat/frontend && corepack pnpm format:check               # passed
git diff --check -- mm-chat                                     # passed
```

Residual blockers:

```text
Requires rebuilding/restarting the frontend container. Existing custom provider
entries already lost by a prior refresh must be re-added once; after this fix
new entries persist across refreshes.
```

## 2026-07-17 — G11.3c Server Default admin provider persistence

Objective: correct the owner-selected provider-configuration authority: the
production provider is the backend Server Default, seeded by env/secret, editable
from the web settings page, and persisted in backend storage rather than treated
as browser-local authoritative state.

Owner correction:

```text
Use backend env/secret Server Default plus administrator web configuration.
Provider Settings may edit it, but the saved authority must be backend-owned.
```

Completed scope:

- added Go admin runtime-config endpoints for `GET/PUT /v1/admin/provider-config`;
- added a Postgres-backed `provider_configs` repository for `SERVER_DEFAULT`;
- kept env/secret values as the fallback Server Default when no DB override
  exists;
- persisted provider name, type, base URL, enabled model list, and encrypted
  secret envelope in backend storage;
- refused plaintext provider secrets and verified BYOK envelopes before storing
  them;
- made `/v1/config`, `/v1/providers/models`, and chat streaming resolve the
  current backend Server Default config instead of only the process env snapshot;
- wired frontend Provider Settings so Server Default edits load/save via the Go
  admin endpoint, API key save uses BYOK `encryptSecret`, and API key presence is
  shown without returning the secret to the browser;
- hid new browser-local provider creation in server mode while leaving the prior
  browser BYOK path as a fallback/tested compatibility surface.

Changed surfaces:

```text
mm-chat/backend/cmd/api/main.go
mm-chat/backend/internal/chat/handler.go
mm-chat/backend/internal/httpserver/server.go
mm-chat/backend/internal/runtimeconfig/{types.go,service.go,handler.go,repository_postgres.go}
mm-chat/backend/internal/runtimeconfig/{service_test.go,handler_test.go}
mm-chat/frontend/src/components/settings/ProviderSettings.tsx
mm-chat/frontend/src/services/api/client/{types.ts,server/providerApi.ts,local/providerApi.ts}
mm-chat/frontend/src/__tests__/{apiClientScaffold.test.ts,settingsUiComposition.test.ts}
mm-chat/frontend/src/i18n/locales/{zh,en,ja}/Providers.json
mm-chat/docs/architecture/standalone-parity-sliced-cutover-plan.md
mm-chat/docs/tracking/progress.md
mm-chat/docs/tracking/standalone-parity-sliced-process.md
```

Verification:

```text
cd mm-chat/frontend && corepack pnpm vitest run \
  src/__tests__/settingsUiComposition.test.ts \
  src/__tests__/apiClientScaffold.test.ts \
  src/__tests__/browserLocalAuthority.test.ts \
  src/__tests__/byok.test.ts                                      # passed, 71 tests
cd mm-chat/frontend && corepack pnpm typecheck                    # passed
cd mm-chat/frontend && corepack pnpm lint                         # passed
cd mm-chat/frontend && corepack pnpm format:check                 # passed
cd mm-chat/backend && GOCACHE=/tmp/neo-chat-go-build go test ./... # passed
```

Residual risks:

```text
Requires rebuilding/restarting backend and frontend containers before browser
manual smoke. The server stores encrypted BYOK envelopes, so stable BYOK private
key configuration remains required for durable restart-safe secret decryption;
without it, env/secret fallback still works but DB-stored envelopes encrypted by
an ephemeral key cannot survive backend key rotation.
```

## 2026-07-17 — G11.3d Multi-provider backend authority and model-list repair

Objective: correct the owner-visible server-mode regressions and make the web
provider editor a backend-authoritative multi-provider administrator surface.

Root cause:

- G11.3c explicitly hid Add whenever the API client was in server mode, so only
  `SERVER_DEFAULT` could be edited;
- the real `/models` request succeeded and returned seven models, but the
  follow-up config save copied the selected one-model list into `modelsList`,
  collapsing the visible fetched catalog back to `gpt-5.5`;
- the deployed backend allowed an ephemeral BYOK key, which is unsuitable for
  decrypting persisted provider envelopes after restart.

Completed scope:

- added backend collection/item admin routes:
  `GET /v1/admin/providers`, `PUT /v1/admin/providers/{id}`, and
  `DELETE /v1/admin/providers/{id}`;
- extended the Postgres provider repository with list and soft-delete while
  retaining the existing locked upsert path;
- added `server-stored` runtime references so model listing and chat resolve a
  custom provider by backend ID and never require the browser to receive its
  secret;
- restored the Add button in server mode, creates a backend draft immediately,
  saves all provider fields/keys to the backend, loads all backend providers on
  settings entry, and deletes custom providers from backend storage;
- retained browser provider metadata only as a non-authoritative UI cache;
- preserved fetched `modelsList` when saving the selected `models` subset;
- generated a stable RSA BYOK key and key ID in ignored
  `.env.single-server`, disabled ephemeral keys, and rebuilt/recreated the
  backend and frontend through the owner-selected Compose source-build flow.

Security boundary:

```text
The web UI does not rewrite the host .env file. .env/secret remains the startup
fallback; Postgres is the live backend configuration authority. API keys cross
the browser boundary only as BYOK envelopes, are stored encrypted, and are
reported back only as hasApiKey=true/false.
```

Verification:

```text
frontend ESLint / typecheck / format                    passed / passed / passed
frontend full Vitest                                   841 passed (179 files)
Go full test suite                                      passed
Docker Compose backend/frontend source build            passed
frontend/backend container health                       healthy / healthy
GET /mm-api/v1/admin/providers                          200, SERVER_DEFAULT listed
POST /mm-api/v1/providers/models                        200, 7 real models
temporary custom provider PUT/list/DELETE               passed; proof row removed
temporary custom BYOK provider model fetch              passed; 7 real models, row purged
runtime BYOK flags                                      stable=true, ephemeral=false
```

Live model proof:

```text
gpt-5.6-sol
gpt-5.6-terra
gpt-5.6-luna
gpt-5.5
gpt-5.4
gpt-5.4-mini
gpt-image-2
```

Residual risk:

```text
An encrypted SERVER_DEFAULT envelope created under the former ephemeral key can
no longer be decrypted by that old key after restart. The configured env API
key remains the working fallback; the next key save from the web UI replaces
the stored envelope using the new stable key.
```

## 2026-07-17 — G11.3e Provider editor autosave

Objective: fix the owner-visible case where fetched models appeared and could
be checked, but switching Settings tabs discarded those checks because no
backend update was issued.

Runtime evidence before the fix:

```text
browser POST /v1/providers/models                         200
browser PUT /v1/admin/provider-config after model checks absent
Postgres SERVER_DEFAULT models                            ["gpt-5.5"]
```

Completed scope:

- model checkbox changes now queue an immediate backend save;
- enable/type changes save automatically;
- provider name and base URL save on blur, including tab switches;
- backend saves are serialized so rapid checkbox changes cannot arrive out of
  order;
- save responses update only server-management metadata and do not overwrite
  newer optimistic model selections;
- the explicit Save to Backend button remains as a manual flush action.

Verification:

```text
targeted provider/settings/BYOK tests                    passed, 70 tests
frontend typecheck / ESLint / format                     passed / passed / passed
```

## 2026-07-17 — G11.4 Image-model chat dispatch

Objective: make selecting `gpt-image-2` in the ordinary chat composer produce
and render an image instead of sending the model to the text chat endpoint.

Root cause and runtime evidence:

```text
10:00 POST /v1/chat/conversations/{id}/stream             502 in 225 ms
persisted assistant model                                 gpt-image-2
persisted assistant status/metadata                       failed / PROVIDER_ERROR
direct /v1/images/generations                             200, image/png
```

The image executor already worked, but `handleSendServerMessage` always used
the chat stream. The Go chat handler resolved `gpt-image-2` as a text provider
and called the incompatible chat-completions endpoint.

Completed scope:

- detect `gpt-image-*`, `dall-e-*`, and `imagen-*` before resolving a text chat
  provider;
- keep the existing user-message and SSE contract, emitting
  `message.started` immediately;
- call the configured Go image job service, store the result through the
  existing artifact service, attach the resulting file to the assistant
  message, finalize it, and return the attachment in `message.completed`;
- extend assistant create/finalize repository inputs to validate and persist
  server file attachments using the same ownership checks as user messages;
- preserve cancellation and fail-closed image error events without exposing
  upstream response bodies;
- set Next `experimental.proxyTimeout` to 300 seconds because rewrite proxies
  otherwise abort streaming backend requests after 30 seconds.

Verification:

```text
Go full tests / vet                                      passed / passed
frontend proxy/config targeted tests                    57 passed
frontend typecheck / ESLint / format                    passed / passed / passed
Docker backend/frontend source builds                   passed / passed
live chat SSE through /mm-api                           23 seconds
live SSE events                                          message.started, message.completed
live assistant model                                     openai_compatible:gpt-image-2
live attachment                                          1 image/png, 941812 bytes
persisted message roles                                  user, assistant
persisted assistant attachments                          1 after reload
temporary conversation/file                              deleted; active rows 0/0
backend/frontend health                                  healthy / healthy
```

## 2026-07-17 — G11.5 Uniform single-user authority

Objective: make Knowledge and every other standalone feature follow the same
single-user ownership model as the original project, without exposing
multi-user consent administration in the product UI.

Root cause:

```text
AUTH_MODE=development                                    configured
Knowledge/query-consent request without Bearer           401
cause                                                     identity middleware forced Bearer for Team/Knowledge/RAG routes
secondary gate                                            Knowledge services require an explicit context User
```

Completed scope:

- development mode now bypasses Session resolution for all non-public routes
  and explicitly injects the fixed Development Owner for strict services and
  repository fallbacks alike;
- stale or valid browser Bearer headers are ignored in development mode, so
  they cannot switch identity or make the single-user app fail authentication;
- required mode remains fail-closed and continues validating Bearer Sessions;
- removed collection/query processing-consent loading, grant, revoke, refresh,
  forms, and status cards from the server Knowledge interface;
- each new collection now receives server-owned MinerU PDF parse and Jina
  passage-embedding consent automatically, so removing the UI cannot strand
  uploads before indexing;
- retained server-side governance and existing owner consents as internal RAG
  safety state rather than user-facing multi-user controls.

Verification:

```text
backend full tests / vet                                  passed / passed
frontend format / lint / typecheck                        passed / passed / passed
frontend tests / production build                         854 passed / passed
Docker backend/frontend source builds                     passed / passed
live query-consents without Bearer                        200
live collections with stale Bearer                        200, fixed Personal owner
temporary collection auto-consents                       MinerU parse + Jina passage_embedding granted
temporary collection cleanup                             deleted; subsequent GET 404
backend/frontend health                                   healthy / healthy
```

## 2026-07-17 — G11.6 Original Knowledge layout parity

Objective: restore the original frontend interaction and visual structure for
Knowledge without reverting its Go/Postgres/MinIO/RAG backend ownership.

Root cause:

```text
original frontend                                        search + card grid + modal editing
migrated ServerKnowledgeBase                             left collection rail + inline admin forms
reason                                                    G8 server adapter shipped a replacement UI instead of reusing product chrome
```

Completed scope:

- restored the original collection search bar, responsive card grid, dashed
  create card, icon/color create/edit modal, breadcrumb detail transition,
  upload drop zone, and compact document rows;
- removed the migration-only two-column admin rail and inline create/edit
  forms;
- kept only server-required behavior differences: backend persistence,
  automatic indexing status, explicit refresh, retry/reprocess, and deletion;
- collection create/update continues to use typed Go API adapters and now also
  persists the original icon/color choices.

Verification:

```text
frontend format / lint / typecheck                        passed / passed / passed
frontend Knowledge composition tests                     7 passed
frontend full tests / production build                    855 passed / passed
Docker frontend source build                              passed
backend/frontend health                                   healthy / healthy
```

## 2026-07-17 — G11.7 Native document indexing repair

Objective: repair the owner-visible `上传并绑定文档失败` error for DOCX while
keeping PDF on the real MinerU path and preserving the existing Jina/Postgres
publication boundary.

Runtime evidence before the fix:

```text
POST /v1/files                                           201
POST /v1/knowledge/collections/{id}/documents            503
error                                                    KNOWLEDGE_PROCESSOR_UNAVAILABLE
failed file MIME                                          application/vnd.openxmlformats-officedocument.wordprocessingml.document
```

Root cause:

- Go bound every Knowledge document to processor `mineru`;
- the server-owned MinerU profile/consent correctly allowed only
  `application/pdf`;
- the Python worker had complete TXT/Markdown/HTML/CSV/DOCX/PPTX/XLSX Native
  Parsers, but production parse composition still installed only the MinerU
  gateway.

Completed scope:

- Go now resolves processor authority from the stored file MIME: PDF uses
  MinerU; non-PDF supported formats use `native/local/native-parser-v1`;
- server startup idempotently applies the credential-free Native governance
  profile and grants its parse consent to all existing Personal collections;
- new collections automatically receive Native consent alongside MinerU parse
  and Jina passage-embedding consent;
- the RAG worker routes by the exact processor authority pinned on the job,
  executes Native parsing through the existing one-exec child, resource-limit,
  process-group, and seccomp sandbox, then emits a projection-ready basic text
  baseline into the existing Jina/Postgres pipeline;
- failed upload-to-bind attempts now delete only the just-uploaded unbound file,
  preventing future orphan objects while preserving already bound files.
- reprocess is now a failed-Version retry only: the UI exposes it only when the
  pending Version is `failed`, and Go rejects rebuilding an already published
  active Version in the same index generation before creating another Job or
  Materialization.

Verification:

```text
Go full vet / tests                                       passed / passed
RAG Ruff / format / strict Mypy                          passed / passed / passed
RAG full tests                                           1701 passed / 7 skipped
frontend format / lint / typecheck                       passed / passed / passed
frontend tests / production build                        855 passed / passed
Docker backend/rag/frontend source build                 passed
backend/rag/frontend health                              healthy / healthy / healthy
```

Live replay reused the failed orphan instead of uploading a duplicate:

```text
file                                                      6824b279-4316-433c-9f85-7d8f85d8110d
document                                                  a6a20583-949d-4b18-b12c-84f45a3224f2
bind                                                       201 processing
parse job                                                  native/native-parser-v1, succeeded, attempt 1
embedding job                                              jina/jina-embeddings-v4, succeeded, attempt 1
search projection                                          1 ready row, 1024 dimensions, vector present
final document/version                                     active / active
```

The source-built runtime also closed the published-Version retry regression:

```text
POST active document /reprocess                           400 INVALID_DOCUMENT_PAYLOAD
message                                                    only failed document versions can be reprocessed
jobs/materializations/reprocess outbox after request       2 / 1 / 0 (unchanged)
```

Residual boundary: this first production Native path intentionally projects a
basic derived text baseline. The existing Native Artifact retains structural
and exact Part positions, but richer table/heading/source-anchor Canonical IR
mapping remains a later quality slice; it is not required for the current basic
citation card and strict no-evidence behavior.

## 2026-07-17 — G11.8 Multilingual Knowledge candidate recall

Objective: repair strict Knowledge refusal for Chinese queries whose exact
evidence existed in an active document but could not pass the English-oriented
Postgres `simple` lexical candidate gate.

Root cause and scope:

- the active candidate function used `plainto_tsquery('simple', ...)` plus
  whitespace/punctuation exact terms; PostgreSQL did not segment CJK phrases;
- migration `025_rag_multilingual_candidate_recall` preserves the existing
  lexical path and adds bounded exact-phrase plus at-most-64 overlapping
  alphanumeric bigram signals over authorized active child content;
- the function still emits references only, keeps selected-collection/current
  generation/published projection fences, and requires Go reauthorization
  before source hydration;
- overlap needs two signals for queries longer than one bigram, preventing the
  unrelated Chinese smoke from becoming evidence merely because a common
  character occurs.

Live proof against the owner's active `test` collection:

```text
query                         before 025   after 025
研究方向是什么                  0            1
西北工业大学                    0            1
今天天气如何                    0            0
lindo咋申请                     0            0
Go API runtime role execute       denied-risk  1 candidate
```

The final `lindo咋申请` refusal is correct for the uploaded document: its
extracted source is a personal introduction/application essay and contains no
Lindo application procedure, requirements, or steps. Strict mode must not turn
filename/topic resemblance into an unsupported answer.

## 2026-07-17 — G11.9A Auto Knowledge answer closure

The development-only RAG break was closed without weakening hydration ACLs.
Startup now supplies a database-valid fixed-owner internal Session, provisions
server-owned answer governance, and keeps answer-only consent changes from
invalidating parse/embedding projections. Chat selected-Knowledge handling is
now Auto augmentation rather than strict refusal; `[K]` citations persist only
when evidence was injected, a normal miss silently uses the model, and only a
real dependency failure surfaces a lightweight frontend notice.

Migration `026` normalizes punctuation before locale-independent CJK bigram
recall. Source-built live proof against `linux do作文.docx` answered
`研究方向是什么？` with “推荐系统” and `[K1]`; `今天天气如何` completed as ordinary
chat with `outcome=no_evidence` and no refusal card. Detailed identifiers,
projection repair evidence, checks, and rollback are in
`docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-17 — G11.9B Conversation-persistent Knowledge binding

Knowledge selection moved from per-message stream config/metadata to the
Postgres-backed conversation config key
`selectedKnowledgeCollectionIds`. Go enforces UUID, dedupe, maximum eight, and
explicit-empty semantics; conversation state wins over stale request metadata,
while a non-empty legacy selection is migrated exactly once when the canonical
key is absent.

The server composer now has a dedicated Knowledge button, a seeded save modal,
and persistent removable chips. Send, regeneration, and edited-message branch
streams no longer serialize Knowledge IDs. Local mode retains attachment
compatibility without becoming a second server authority.

Checks passed: Go all-package compile and focused binding tests; frontend
format/lint/typecheck; 47 focused frontend tests; backend/frontend production
source builds; recreated container health. Full frontend Vitest passed 855 of
856, with only the unchanged restricted-sandbox `spawnSync /usr/bin/node EPERM`
case. Detailed contract, rollback, and the blocked host curl note are recorded
in `docs/tracking/g11-knowledge-auto-rag-process.md`.

## 2026-07-17 — G11.9C.1 Contextual rewrite and dual-query RRF

The first hybrid-quality slice now rewrites only deictic follow-ups using at
most six prior messages. Original and standalone queries both execute against
the existing reference-only keyword/CJK candidate function; exact references
are fused with deterministic global RRF, broad candidate limit 20, and final
hydration limit five. Rewrite failure leaves the original lane intact, and
diagnostics persist only a boolean rather than private query text.

Go all-package compile, vet, focused rewrite/RRF tests, an end-to-end handler
follow-up test, backend production source build, and recreated health passed.
Dense Jina query embedding and reranking remain explicitly open as
G11.9C.2/C.3; details and rollback are in
`docs/tracking/g11-knowledge-auto-rag-process.md`.
