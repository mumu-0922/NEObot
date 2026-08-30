# Production Promotion Gap

## Scope inspected

- `mm-chat/scripts/preflight-single-server.sh`
- `mm-chat/scripts/compose-single-server-production.sh`
- `mm-chat/scripts/release-images.sh`
- `mm-chat/scripts/backup-single-server-production.sh`
- `mm-chat/.env.single-server.example`
- `mm-chat/compose.production.yml`
- `mm-chat/docs/deployment/release-rollback.md`
- `mm-chat/docs/deployment/single-server-compose.md`
- `.trellis/spec/operations/runtime-recreate-image-pinning.md`

No secret value was printed. The active env was inspected only for key
presence, policy classification, and image-reference shape.

## Observed live boundary

- `.env.single-server` exists, is owned by the operator, and uses mode `0600`.
- Strict preflight exits first with `POSTGRES_DATA_DIR is required`.
- Of 49 unconditional production-preflight keys, 19 are missing or empty:
  - `POSTGRES_DATA_DIR`
  - `TEAM_CURSOR_ACTIVE_KEY_ID`
  - `TEAM_CURSOR_KEYRING`
  - `TEAM_MAIL_ACTIVE_KEY_ID`
  - `TEAM_MAIL_KEYRING`
  - `TEAM_INVITE_ACCEPT_URL_BASE`
  - `MCP_AUDIT_RETENTION`
  - `MCP_CLEANUP_INTERVAL`
  - `AGENT_LOCAL_RUNTIME_SOURCE`
  - `AGENT_LOCAL_RUNTIME_ROOT`
  - `AGENT_LOCAL_WORKSPACE_ROOT`
  - `AGENT_LOCAL_SHELL`
  - `AGENT_LOCAL_APPROVAL_MODE`
  - `AGENT_LOCAL_CALL_TIMEOUT`
  - `AGENT_LOCAL_RUN_TIMEOUT`
  - `AGENT_LOCAL_MAX_OUTPUT_BYTES`
  - `AGENT_LOCAL_MAX_CALLS_PER_RUN`
  - `AGENT_LOCAL_MAX_ROUNDS_PER_RUN`
  - `AGENT_LOCAL_MAX_CONCURRENT`
- `RAG_WORKER_DISPATCH_ENABLED=false` and an explicitly empty
  `RAG_WORKER_JOB_STAGES` are also required policy states but are absent or do
  not currently satisfy the strict gate.
- The active Backend, Frontend, MCP Runner, RAG, and PostgreSQL references are
  local/mutable from the strict preflight's perspective. Docker reports local
  content digests, but their `mm-chat/...@sha256:` names have no registry host
  and intentionally fail `valid_image_digest`.
- The live Backend and Memory Worker currently run different retained Backend
  image IDs. A production promotion must converge them deliberately on the
  reviewed release's single `BACKEND_IMAGE`; copying the current env is not a
  coherent production release.
- Docker client configuration exists, but its presence does not prove package
  write permission. Historical project evidence records a GHCR push attempt
  failing with HTTP 403.

## Executable-contract gap

`release-images.sh` currently builds and emits immutable references for four
application images: Backend, MCP Runner, Frontend, and RAG. Strict preflight
also requires an immutable `POSTGRES_IMAGE`, but the release script does not
build or emit the repository's `postgres/Dockerfile` image. Therefore its
`production-images.env` output is insufficient by itself to satisfy the
production gate.

There is no focused release-script test. `test-preflight-single-server.sh`
covers digest syntax and production Compose rendering, but does not assert that
the release bundle contains every image required by preflight.

## Documentation drift

`release-rollback.md` declares source-build Compose as the active release mode
and says digest promotion is optional. `single-server-compose.md` separately
states that production must use registry digests and the strict wrapper. This
is understandable historical layering, but operators currently have two
different answers to “what is production promotion.”

## Recommended bounded change

1. Extend `release-images.sh` to build/publish the PostgreSQL retrieval image,
   accept a configurable PostgreSQL repository, and emit `POSTGRES_IMAGE`.
2. Add a focused hermetic test using a fake Docker/buildx command to prove:
   all five images are selected, local mode stays non-publishable, push mode
   emits five immutable refs plus the release version, and partial/failing
   builds never produce a complete promotion bundle.
3. Add the focused test to the standalone structural gate.
4. Reconcile the deployment docs: source-build remains the current live/local
   path; registry digest promotion becomes the explicit hardened production
   path and requires operator-authorized registry publication.
5. Produce a sanitized operator gap report for the current env, but do not
   synthesize missing Team secrets, invite URLs, registry credentials, or edit
   the live env.

## Authority boundary

No image push or live recreation is authorized by the current request. A real
digest candidate and rollback env can only be completed after the operator
explicitly authorizes a registry namespace/push and supplies or reviews the
missing production configuration. Until then, implementation can make the
release path complete and testable without changing live state.
