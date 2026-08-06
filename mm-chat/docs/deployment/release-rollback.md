# Release and Rollback Runbook

This runbook applies to the single-server `mm-chat` Compose stack. It assumes
real secrets live in `mm-chat/.env.single-server` and runtime data lives in
`mm-chat/data/`.

## Active Release Mode — Compose Source Build

The current owner-selected release path is source-build Compose deployment from
the standalone `mm-chat/` tree. Do not require GHCR, remote registry images, or
`@sha256:` digest env proof for the standalone cutover gate.

The Compose file already carries the required build contexts:

```text
backend     -> ./backend/Dockerfile
frontend    -> ./frontend/Dockerfile
rag-worker  -> ./rag/Dockerfile
```

Optional image publishing still exists through `scripts/release-images.sh`, but
it is a future hardening/promotion path, not the default deployment flow.

## Pre-Release Gate

Run from the standalone project root:

```bash
cd mm-chat
bash scripts/verify-standalone.sh
bash scripts/verify-standalone.sh --full

docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile app --profile rag-worker \
  build backend frontend rag-worker
```

Then run migrations and start the app/RAG services from the freshly built local
images:

```bash
docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile ops run --rm migrate

docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile app --profile rag-worker \
  up -d backend frontend rag-worker
```

Verify the runtime edge:

```bash
front_port="$(awk -F= '$1=="FRONTEND_PORT"{print $2}' .env.single-server)"
: "${front_port:=3000}"
curl -fsS "http://127.0.0.1:${front_port}/"
curl -fsS "http://127.0.0.1:${front_port}/mm-api/ready"
curl -fsS http://127.0.0.1:8080/ready
```

For the RAG worker, verify its container health or run an internal health probe:

```bash
docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile rag-worker exec -T rag-worker \
  python -c "import urllib.request; print(urllib.request.urlopen('http://127.0.0.1:8081/health', timeout=3).read().decode())"
```

## Backup Gate

Before migrations or service replacement, create and verify backups. The lower
level backup scripts work with the normal build-based Compose file and do not
require registry image variables:

```bash
cd mm-chat
COMPOSE_FILE=compose.single-server.yml \
ENV_FILE=.env.single-server \
bash scripts/backup-postgres.sh

COMPOSE_FILE=compose.single-server.yml \
ENV_FILE=.env.single-server \
bash scripts/backup-minio.sh
```

Keep the generated `.sha256` sidecars with the backup files. Restore drills are
required before destructive former-root cleanup; follow
[`backup-restore.md`](./backup-restore.md) for the temporary Postgres and MinIO
restore procedure.

## Deploy

For an ordinary single-server update:

```bash
cd mm-chat
git fetch --all --tags
git checkout <release-commit-or-tag>

docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile app --profile rag-worker \
  build backend frontend rag-worker

docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile ops run --rm migrate

docker compose --env-file .env.single-server \
  -f compose.single-server.yml \
  --profile app --profile rag-worker \
  up -d backend frontend rag-worker
```

Then run the smoke checks from the pre-release gate.

## Rollback Decision Tree

- **App build bad, schema compatible**: checkout the previous known-good commit
  or tag, rebuild `backend frontend rag-worker`, and recreate only the affected
  services.
- **Latest migration bad before user traffic**: stop `backend`, run the
  migration command with `down` only for the explicitly approved version, then
  redeploy the previous known-good commit. One invocation rolls back only one
  version.
- **Migration bad after user traffic**: prefer a forward fix. Down migration may
  destroy or orphan data.
- **Knowledge migrations after live writes**: forward-fix only unless a reviewed
  rollback plan proves no active leases, bound jobs, authority rows, namespace
  conflicts, projection state, or tombstones will be lost.
- **Object storage issue**: stop upload/import paths, verify MinIO backup, and
  restore into a temporary bucket before touching the live bucket.
- **Redis issue**: flush or recreate Redis only; Postgres/MinIO remain
  authoritative.

### Migration 053 / Memory v2 foundation rollback

Migration `053_memory_project_scope_settings` is additive but replaces the v1
Global active-content unique index with three exact-scope indexes. The
post-`053` backend repeats the Global predicate in its Memory `ON CONFLICT`;
the pre-`053` binary does not. Never run those writer versions together after
the migration.

Forward deployment also uses a bounded outage: stop every pre-`053` backend,
apply `053`, and only then start the post-`053` backend. Do not apply `053`
under a live pre-`053` Memory writer.

Before v2 use, rollback requires a full backend outage: stop every post-`053`
backend, run one `migrate down`, then deploy the pre-`053` backend. Down fails
atomically if any Project exists, any Memory is non-Global, Conversation Memory
policy/generation changed, or Sensitive/L2/L3 settings differ from the
migration defaults. Do not delete user data to bypass a guard.

Once any v2 authority is in use, retain `053` and roll back only later feature
flags/readers. Use a forward fix for repository defects.

### Migration 054 / Memory capture worker rollback

Stop every `memory-worker` instance before rolling back `054`. The down
migration fails closed with `MEMORY_WORKER_ROLLBACK_REQUIRES_EMPTY_QUEUE` while
any outbox event or job exists, including completed/dead-letter history. Do not
delete queued user work to bypass that guard. The safe operational rollback is
to stop the worker and automatic capture while retaining `054`; chat, v1
Memory CRUD, and v1 Recall remain available. Only a pre-traffic or explicitly
reconciled empty queue may run one `migrate down`, after every post-`054` API
writer has stopped. Redis requires no queue migration because it stores only
best-effort `event_id` wake signals.

### Migrations 055-056 / Memory provenance and Review rollback

Stop every Memory worker before attempting either down migration. Migration
`056` refuses rollback while candidate batches, Review suggestions/targets/
evidence, expiry jobs, or non-default canonical temporal/conflict metadata
exist. Migration `055` refuses rollback while evidence, revisions, tombstones,
deletion manifests, non-default epochs, AI canonical rows, or purge work exist.

Do not delete proposal, provenance, or deletion history to force these guards.
After traffic, stop automatic capture and use a forward fix while keeping the
schema. Clean down/re-up is a disposable/pre-traffic proof only.

### Migration 057 / Memory action, Activity, and Usage rollback

Stop post-`057` API writers before a pre-traffic down. The migration fails with
`MEMORY_ACTION_ROLLBACK_REQUIRES_EMPTY_HISTORY` while any direct action,
normalized target, Activity, Usage, or typed prior revision snapshot exists,
and with `MEMORY_ACTION_ROLLBACK_REQUIRES_V1_SOURCE` while a `direct_user`
canonical row exists.

Do not delete user Memory/history to bypass either guard. After any PR6 action
or answer Usage has committed, retain `057` and roll back application behavior
with a forward-compatible build. A valid clean drill is one-step
`057 -> 056 -> 057` after all disposable fixture users are removed.

### Migration 058 / Memory lexical projection and shadow rollback

Set `MEMORY_LEXICAL_SHADOW_ENABLED=false` before rollback and stop post-`058`
API writers. The flag stops new comparisons only; canonical transactions keep
projection correct while `058` exists. Down fails with
`MEMORY_LEXICAL_ROLLBACK_REQUIRES_V1_READER` if any Memory reader profile has
been promoted, and with `MEMORY_LEXICAL_ROLLBACK_REQUIRES_EMPTY_OBSERVATIONS`
after any shadow observation exists.

Never delete observation history to force rollback. After observation begins,
retain `058` and roll back behavior with the default-off flag or a compatible
application build. Only a clean pre-observation database may run
`058 -> 057 -> 058`; the derived projection is safely discarded on down and
rebuilt from current canonical Memory on re-up.

### Migration 059 / Memory vector and hybrid shadow rollback

Set `MEMORY_TOOL_LOOP_ENABLED=false` in the API and
`MEMORY_HYBRID_SHADOW_ENABLED=false` in both the API and Memory Worker, then
stop post-`059` processes. The switches stop first-round Memory Tool reads, new
embedding claims, and hybrid comparisons; they do not mutate the active reader
pointer. Down fails
with `MEMORY_HYBRID_ROLLBACK_REQUIRES_V1_READER` for a non-v1 reader and with
`MEMORY_HYBRID_ROLLBACK_REQUIRES_EMPTY_OBSERVATIONS` after any hybrid
observation exists.

The Tool switch is also the production fixed-Judge rollback boundary. Turning
it false removes `search_memory` exposure and stops fixed BGE/Luna reader/Judge
requests on the next backend composition; it does not delete canonical Memory,
hybrid observations, or immutable answer Usage. Do not substitute v1 results
for a failed product Tool read. If the current stored `SERVER_DEFAULT` /
OpenAI-Compatible / attested Base-URL hash / `gpt-5.6-luna` tuple drifts, leave
the Tool switch false until the exact authority is restored and reviewed.
Clearing `MEMORY_TOOL_LOOP_CANARY_USER_IDS` is the narrower immediate canary
rollback. Keep the global switch false after any failed Validation; never add a
UUID merely because aggregate metrics passed when a required slice failed.

Before a flag-only `--force-recreate`, record each running container's exact
image ID and pin `BACKEND_IMAGE` to an immutable digest or retained tag. The
default `mm-chat/backend:local` tag is mutable and may no longer name the image
used by the running container; recreating from a drifted tag is an implicit
release and may select a binary that requires unapplied migrations. Render the
same Compose topology shown by the live container labels, use `--no-build` plus
`--no-deps`, and reject any image or schema drift. Never run a migration merely
to accommodate an accidentally selected image.

Never delete observation evidence to force rollback. After shadow collection
begins, retain `059` and roll back behavior with the default-off flag. Only a
clean pre-observation database may run `059 -> 058 -> 059`; its HNSW vectors
and embedding jobs are derived and rebuild from current canonical projection.

Migration `065` adds only the read-only exact-final hydration capability. After
the Tool flag is false and backend processes are stopped, clean rollback may
run `065 -> 064`; re-up must replay `064 -> 065`. Down removes the function
only and does not delete observations or canonical Memory. Do not attempt to
keep an enabled Tool Loop on a pre-`065` backend; final content authority would
be unavailable and must fail closed.

### Migration 070 / Memory health rollback

Stop every Memory Worker before attempting schema rollback. Each clean Worker
shutdown calls `memory_worker_retire`; an unclean shutdown becomes inactive
after its bounded heartbeat TTL. Migration `070` down fails with
`MEMORY_HEALTH_ROLLBACK_REQUIRES_STOPPED_WORKERS` while any heartbeat remains
live. Runtime roles must not delete heartbeat rows directly. After the guard
passes, down removes only derived heartbeat/user-health capabilities and
restores the prior worker-readiness function; canonical Memory, projections,
capture jobs, and Usage remain intact. Clean disposable replay is
`069 -> 070 -> 069 -> 070`.

### Migration 069 / Memory compatible Tool-profile rollback

Disable automatic recording and stop every Memory Worker before rolling back
application behavior. This prevents new extraction, proposal, promotion, and
embedding claims while retaining existing canonical Memory and audit history.
Migration `069` is a forward compatibility correction over byte-immutable
`068`. Its down path restores extraction profile v4 authority and is a pre-
promotion drill only: it fails with
`MEMORY_AUTO_CAPTURE_COMPATIBLE_PROFILE_ROLLBACK_REQUIRES_NO_PROMOTIONS` once
any `auto_accept`/`AUTO_CAPTURED` audit exists. Clean disposable replay is
`068 -> 069 -> 068 -> 069`.

### Migration 068 / Memory Tool evidence-profile rollback

Migration `068` is a forward profile correction over byte-immutable `067`. Its
down path restores extraction profile v3 authority and is a pre-promotion drill
only: it fails with
`MEMORY_AUTO_CAPTURE_TOOL_PROFILE_ROLLBACK_REQUIRES_NO_PROMOTIONS` once any
`auto_accept`/`AUTO_CAPTURED` audit exists. Clean disposable replay is
`067 -> 068 -> 067 -> 068`.

### Migration 067 / Memory auto-capture authority rollback

Migration `067` is a forward fix over the already-applied, byte-immutable
`066`; never edit `066` or rewrite its recorded checksum. Its down path restores
the original `066` promotion function and is a pre-promotion schema drill only:
it fails with
`MEMORY_AUTO_CAPTURE_AUTHORITY_ROLLBACK_REQUIRES_NO_PROMOTIONS` once any
`auto_accept`/`AUTO_CAPTURED` audit exists. Clean disposable replay is
`066 -> 067 -> 066 -> 067`.

### Migration 066 / Memory auto-capture promotion rollback

Migration `066` down is likewise a pre-promotion schema drill only: it fails with
`MEMORY_AUTO_CAPTURE_ROLLBACK_REQUIRES_NO_PROMOTIONS` once any
`auto_accept`/`AUTO_CAPTURED` audit exists. Never delete a canonical row,
suggestion, or decision audit to bypass that guard. After the first promotion,
keep `066`–`069` applied and use another forward-compatible fix. Clean disposable
replay for the initial capability is `065 -> 066 -> 065 -> 066`.

### Migration 060 / Memory governance rollback

Deploy migration `060` and its matching backend together. The migration
revokes `go_api_runtime` EXECUTE on the old Global manual upsert/update
functions, and the post-`060` repository calls classification-aware governance
wrappers. Do not run a pre-`060` writer against the new grant set.

For a pre-traffic down, stop post-`060` API writers first. Rollback fails with
`MEMORY_GOVERNANCE_ROLLBACK_REQUIRES_NO_DECISIONS` when the Review decision
audit is non-empty, `MEMORY_GOVERNANCE_ROLLBACK_REQUIRES_LEGACY_REVIEWS` when a
legacy Review has been decided, or
`MEMORY_GOVERNANCE_ROLLBACK_REQUIRES_NO_MOVE_REVISIONS` after a scoped move.
Clean down restores the old v1 function grants.

Never delete governance/revision history to force rollback. After any PR9
decision or move exists, retain `060` and deploy a forward fix. Reader rollback
remains independent because PR9 did not promote lexical/hybrid/scoped Memory
into prompts or Usage.

### Migrations 051-052 / SiliconFlow TTS rollback

Migration `051_siliconflow_tts_cache` adds the exact Voice provider identity
constraint plus `tts_audio_cache` and `tts_audio_cleanup_queue`. Its down
migration drops those tables and therefore loses cleanup identity. Do not run
it while generated objects remain live.

Migration `052_tts_runtime_role_grants` grants the API capability role the
minimum DML required for synthesis lookup/commit and cleanup claims. Its down
migration revokes those grants and must run before 051 down. A missing 052 is a
forward-migration defect: do not hot-grant production or edit applied 051.

For a pre-traffic rollback, deactivate `VOICE:SILICONFLOW`, stop the backend
and its cleanup worker, drain or explicitly reconcile the cleanup queue through
the normal File/object deletion path, verify both TTS tables are empty, and
take verified Postgres and MinIO backups. Run one `migrate down` to revoke 052,
then a separately approved second down to drop 051, deploy the previous images,
and verify Voice is fail-closed. Keep the provider keyring: the Voice vault row
and backups are unreadable without it.

After user traffic or when cached files cannot be reconciled, use a forward fix
instead of schema rollback. Dropping 051 before 052 or before cleanup can leave untracked audio
objects; deleting MinIO objects directly first can leave authenticated File and
cache metadata pointing at missing bytes.

## Optional Registry Image Promotion

If a future deployment wants registry-published immutable artifacts, use:

```bash
cd mm-chat
docker login ghcr.io
./scripts/release-images.sh \
  --push \
  --image-namespace ghcr.io/mumu-0922 \
  --tag <release-id>
```

The script writes `.release/images/<release-id>/production-images.env` with
`MM_CHAT_VERSION`, `BACKEND_IMAGE`, `FRONTEND_IMAGE`, and `RAG_IMAGE` digest
lines. This optional path may be paired with
`compose-single-server-production.sh` and `preflight-single-server.sh`, but it
is not required for the current build-based standalone cutover.

## Post-Release Notes

Record release commit, migration output, backup filenames, smoke-test results,
and rollback decision in `mm-chat/docs/tracking/standalone-parity-sliced-process.md`.
