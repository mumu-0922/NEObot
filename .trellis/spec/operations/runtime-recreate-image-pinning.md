# Runtime Recreate Image Pinning

## Scenario: Recreate live Compose services without image or schema drift

### 1. Scope / trigger

This contract applies before `docker compose up --force-recreate`, especially
when a service uses a fallback tag such as `mm-chat/backend:local`, when the
database is intentionally behind the repository's newest migration, or when a
runtime-only flag change must not become an implicit application release.

### 2. Signatures

Capture the live image ID and require an explicit resolvable image reference:

```bash
container_id="$(docker compose ps -q backend)"
live_image_id="$(docker inspect --format '{{.Image}}' "$container_id")"
docker image inspect "$live_image_id"

BACKEND_IMAGE=registry.example/mm-chat/backend@sha256:<digest>
docker compose --env-file .env.single-server config --quiet
docker compose --env-file .env.single-server \
  up -d --no-build --no-deps --force-recreate backend
```

The Backend Dockerfile is multi-target and ends in the distinct `mcp-runner`
stage. A manual immutable Backend-only build must select `runtime` explicitly
and inspect its command before recreation:

```bash
docker build --pull=false --target runtime \
  --tag mm-chat/backend:<immutable-tag> backend
docker image inspect mm-chat/backend:<immutable-tag> \
  --format 'user={{.Config.User}} cmd={{json .Config.Cmd}}'
```

One-off helper commands must also pin the reviewed image. Compose versions may
omit `run --no-build`; capability inspection is allowed only before credentials
or Provider work:

```bash
docker compose run --help
# Always: --no-deps --pull never
# Add --no-build only when this exact subcommand supports it.
# Never add positive --build.
```

For a local-only recovery, an explicit retained tag may replace a registry
digest, but it must resolve to the reviewed image ID before any container is
removed.

Pair pre-deploy PostgreSQL and MinIO backups under one bounded identifier, then
compare the restored database with the live migration authority dynamically:

```bash
backup_id="predeploy-<commit>-<UTC>"
COMPOSE_FILE=compose.single-server.yml ENV_FILE=.env.single-server \
  BACKUP_SET_ID="$backup_id" bash scripts/backup-postgres.sh
COMPOSE_FILE=compose.single-server.yml ENV_FILE=.env.single-server \
  BACKUP_SET_ID="$backup_id" bash scripts/backup-minio.sh

psql_live ... -At -F '|' \
  -c 'SELECT version, name, checksum FROM schema_migrations ORDER BY version' \
  > live-migrations.tsv
psql_restored ... -At -F '|' \
  -c 'SELECT version, name, checksum FROM schema_migrations ORDER BY version' \
  > restored-migrations.tsv
cmp --silent live-migrations.tsv restored-migrations.tsv
```

### 3. Contracts

- Record each target container ID, image ID, Compose config-file label, and
  health state before stopping it.
- Resolve the exact image ID through an immutable digest or a protected retained
  tag before `--force-recreate`; a mutable fallback tag is not rollback state.
- Never build `backend/Dockerfile` without an explicit target for a manual
  Backend tag. Its final stage is `mcp-runner`; Backend requires
  `--target runtime`, `USER mmchat:mmchat`, and
  `CMD ["/usr/local/bin/mm-chat-api"]`. Prefer the target-aware Compose build
  or `scripts/release-images.sh` over a handwritten Docker build.
- Render the same Compose topology named by the live container labels.
- Compose `config --images` lists only services admitted by the active profile
  set. Include `--profile app` when asserting the Backend candidate; output
  produced without that profile cannot prove which Backend image will run.
- Under `set -o pipefail`, do not validate a long producer with
  `producer | grep -q`: an early successful grep can close the pipe, make the
  producer exit on `SIGPIPE`, and falsely trigger rollback. Capture the bounded
  output first, then grep the captured string.
- Compare the database's applied migration version with the selected binary's
  schema requirements. A flag-only restart must not run migrations to make an
  accidentally newer image start.
- Treat an unexpectedly empty live database as a recovery blocker, not a clean
  compatibility state. Preserve it as an exclusive rollback artifact and
  require separately authorized, isolated same-major restore rehearsal before
  selecting any recovered database or applying an explicit migration range.
- Back up the active runtime environment outside Git with mode `0600` and make
  the candidate environment render successfully before downtime.
- Use one explicit backup-set ID for the pre-deploy PostgreSQL dump and MinIO
  archive. Verify both checksum sidecars, restore only to a uniquely named
  temporary database and bucket, compare the complete live/restored migration
  manifests, validate bounded row/object samples, and prove all drill targets
  were removed before recreation.
- Migration authority comes from the live database's ordered
  `(version, name, checksum)` rows. Never duplicate a migration list in a
  restore runbook; it becomes stale as soon as a new migration lands.
- A healthy source-build stack may predate newer registry-promotion preflight
  keys. For a narrowly authorized local defect rollout, preserve and render the
  exact live Compose topology instead of inventing missing production values.
  Record the hardening gap separately. Production promotion must still use the
  strict wrapper and immutable registry digests.
- Use `--no-build --no-deps` and name only the affected services. Record
  unrelated container IDs and require them to remain unchanged.
- If a local development image was lost, rebuild only from an exact Git commit
  known to match the current schema, in a tracked-only external build context.
  Production rollback still requires a retained published digest; never rebuild
  a production rollback image.
- A running old admin image may not contain a newly added one-off export
  command. Build and pin the reviewed candidate explicitly for that helper;
  never move the mutable live tag and never infer helper capability from the
  current source tree.
- Readiness must be checked by dependency, not only by container health. A
  retained object-storage bucket does not prove that the configured application
  IAM identity still exists after a runtime restart. If storage alone is not
  ready, rerun only the existing byte/config-matched idempotent initializer and
  require an independent application-credential bucket-access proof; do not
  recreate or restart unrelated storage services.
- Validate every post-recreate probe path against the registered running
  routes before classifying its response as service health. An unknown verifier
  route is still a failed rollout proof: execute the prepared behavior rollback,
  preserve the pinned image/schema/data, correct the probe, and retry only the
  previously authorized Provider-free recreation.
- Authenticated deployment smokes may create a short-lived marked Session, but
  cleanup must use an exact tested SQL path and prove the marker count returns
  to zero. `psql -v name=value -c "... :'name' ..."` does not perform psql
  variable interpolation in this execution mode; use stdin/heredoc SQL for
  psql variables, or a validated fixed marker for cleanup.
- Keep every rollback phase marker read by an `EXIT` trap in the parent shell.
  Assigning it in a pipeline body such as `producer | while read ...` mutates a
  subshell copy and can make the trap skip a required disable/recreate action.
  Use process substitution, a parent-shell loop, or an explicit state file, and
  exercise the trap before live mutation.
- Bind verifier assertions to the documented, observed response contract.
  Optional fields may be absent; a health proof must use authoritative fields
  such as `status`, `readyCount`, `pendingCount`, and `failedCount` rather than
  inventing a mandatory `reason` value.
- Verify packaged frontend behavior against its compiled representation.
  Tailwind source class names may be escaped, transformed, or absent from
  minified assets; assert emitted CSS declarations or rendered browser behavior
  instead of treating a source-marker miss as a defective candidate image.
- A user-reported live UI fix is not complete when only source tests and a local
  production build pass. Prove the active Frontend container runs an image
  built from or after the fix commit, then inspect the CSS/JavaScript fetched
  from the live edge (or rendered computed style). If the container predates
  the fix, classify the failure as release propagation and deploy the reviewed
  Frontend image before changing the source again.
- Parenthesize every PostgreSQL set-operation branch that owns
  `ORDER BY`/`LIMIT`, or move that selection into an explicit scalar subquery.
  Never let verifier SQL syntax remain the first execution of a rollback path
  after live mutation.

### 4. Validation and error matrix

| Condition                                                                                        | Required result                                                                                                                                                                                          |
| ------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Running image ID has no retained digest/tag                                                      | Stop before recreation and create a protected reference while the container still exists.                                                                                                                |
| Rendered image differs from the recorded live image during a flag-only change                    | Reject the candidate; pin the recorded image explicitly.                                                                                                                                                 |
| Selected binary requires an unapplied migration                                                  | Do not migrate implicitly; select a schema-compatible image or obtain separate migration authorization.                                                                                                  |
| Candidate Backend image has the MCP Runner entrypoint or no API command                          | Do not wait out health retries. Restore the retained Backend image immediately, rebuild with `--target runtime`, inspect image config, then perform a fresh targeted recreation.                         |
| Target does not become healthy                                                                   | Restore the protected environment and exact retained image, then verify health before further work.                                                                                                      |
| Any unrelated container ID changes                                                               | Treat the operation as scope violation and investigate.                                                                                                                                                  |
| Persistent row count decreases                                                                   | Stop, retain evidence, and restore from the protected data artifact if mutation is confirmed.                                                                                                            |
| Live database contains no expected public schema                                                 | Preserve the empty state, stop recreation, and obtain separate authority for an isolated same-major restore rehearsal plus an exact migration range.                                                     |
| Target container is healthy but readiness reports storage not ready                              | Verify bucket and IAM separately; rerun only the existing attested initializer, then prove application-key access without restarting storage.                                                            |
| `compose run --no-build` is rejected by the installed CLI                                        | Stop before credentials. Capability-detect, retain `--pull never`, omit positive `--build`, and verify the exact helper image.                                                                           |
| Running admin binary lacks the required one-off command                                          | Stop before credentials/Provider work and select an explicitly reviewed pinned helper image; do not recreate live backend.                                                                               |
| A post-recreate identity/readiness probe returns route-level `404`                               | Restore the protected behavior environment, keep the pinned image and schema, verify the route from current source/runtime, and retry only the Provider-free recreation. Do not replay a consumed smoke. |
| `config --images` omits Backend because profile `app` was not active                             | Re-render with the exact live profile set before deciding the candidate reference is absent. |
| `producer \| grep -q` fails only under `pipefail` after a visible match                          | Treat it as a verifier defect; capture producer output before matching and do not mutate live state until it passes. |
| temporary authenticated Session cleanup leaves its marker row                                   | Keep rollback active, delete by the exact validated marker, prove zero rows, then retry with stdin-based psql variable substitution. |
| An `EXIT` trap reads a phase marker assigned inside a pipeline subshell                          | Treat automatic rollback as unproven. Execute the prepared corrective rollback directly, verify the disabled state, then replace the pipeline or persist state explicitly before retry.                  |
| A healthy response omits an optional field assumed by the verifier                               | Roll back behavior, inspect the current API contract and captured response, then assert only authoritative required fields on a fresh Provider-free retry.                                               |
| A `UNION` arm contains unparenthesized `ORDER BY`/`LIMIT`                                        | Reject the verifier before live use; parenthesize the arm or use a scalar subquery and execute it against the rehearsed schema.                                                                          |
| A packaged frontend lacks a literal Tailwind source class but emits the required CSS declaration | Treat the literal-marker assertion as invalid, retain the prepared rollback result, and retry with compiled-CSS or rendered-behavior verification.                                                       |
| Source checks pass but the live Frontend image predates the fix commit                           | Stop editing the source; retain the old image, deploy a reviewed candidate, and prove the live edge serves the candidate asset.                                                                          |
| Restore acceptance embeds a fixed migration list that differs from the live manifest            | Reject the verifier before deployment. Export ordered live and restored `(version, name, checksum)` rows and require a byte-identical comparison.                                                        |
| Strict promotion preflight rejects an older but healthy source-build environment                 | Do not synthesize production values inside the defect rollout. Preserve the exact live topology, use the reviewed local-drill boundary, and schedule promotion hardening separately.                     |
| Temporary restore database, bucket, staging, or object-key sample remains after the drill        | Stop before recreation, remove only the uniquely named drill residue, prove zero matches, and rerun cleanup verification.                                                                               |

### 5. Good / base / bad cases

- **Good**: retain the running image by digest, render the candidate, snapshot
  persistence, recreate only named services, and prove flags, image IDs, health,
  and unrelated-container stability.
- **Base**: restart without recreation when no environment or image input
  changed; still verify the current container health.
- **Bad**: run `--force-recreate` against `backend:local`, discover afterward
  that the tag moved to a schema-incompatible image, and then run migrations to
  fit the accidental release.
- **Good Backend-only build**: select Docker target `runtime`, inspect the image
  user and API command, preserve the old environment/image, then recreate only
  Backend.
- **Bad Backend-only build**: build the Dockerfile's final `mcp-runner` stage as
  a Backend tag and wait for an API health check that can never open port 8080.
- **Base helper**: the old live admin lacks the command; a pinned candidate
  image runs only the read-only helper with `--pull never`, while live service
  IDs stay unchanged.
- **Bad helper**: assume source capability exists in the running image, or call
  `compose build admin` and silently move `BACKEND_IMAGE` before export.
- **Good dependency recovery**: backend health exposes storage-only failure;
  bucket existence and IAM are inspected separately, the existing initializer
  is rerun without recreation, and application-key access plus full readiness
  pass while unrelated container IDs remain unchanged.
- **Bad dependency recovery**: restart MinIO because its bucket exists but the
  backend is not ready, rotate credentials opportunistically, or treat
  container health as proof that the application identity is usable.
- **Good verifier recovery**: an unknown probe path triggers the prepared
  behavior rollback; source confirms the correct route, and a fresh
  Provider-free recreation proves health without changing schema or replaying
  the behavioral smoke.
- **Bad verifier recovery**: leave flags enabled after an unproven route, change
  live auth mode to make the probe pass, or rerun a consumed Provider smoke.
- **Good rollback control**: the parent shell sets a phase marker, a forced
  verifier failure exercises the `EXIT` trap, and post-state proves the disable
  event plus targeted service recreation occurred.
- **Bad rollback control**: set the marker inside `... | tee ... | while read`
  and assume the parent-shell trap can observe it.
- **Good contract verification**: assert required health status and counts,
  while treating an absent optional reason as valid.
- **Bad contract verification**: hard-code a convenient reason string that the
  endpoint contract does not require.
- **Good SQL verification**: execute the exact set-operation query during the
  disposable rehearsal with every ordered/limited arm parenthesized.
- **Bad SQL verification**: first execute an untested set-operation verifier
  only after live flags have been enabled.
- **Good packaged-UI verification**: assert the exact emitted `max-height` and
  scrollbar declarations or measure the rendered scroll region.
- **Bad packaged-UI verification**: grep minified chunks for an unescaped source
  class and classify its absence as a broken image.
- **Good recovery rehearsal**: one ID names both backup artifacts; the dump is
  restored into a unique database, the archive into a unique bucket, the full
  live migration manifest and bounded object samples match, and every drill
  target is removed before Backend recreation.
- **Bad recovery rehearsal**: trust checksums without restoring, compare the
  database against a migration list frozen at an old release, or leave the
  temporary bucket available after the test.
- **Base source-build rollout**: strict promotion keys are absent, so the exact
  running Compose topology is rendered and only release identity plus the
  reviewed image changes; registry-promotion hardening remains separate.

### 6. Tests required

- Assert the candidate Compose render names the intended pinned image and exact
  flag values.
- Assert image checks activate the target service profile and that the verifier
  cannot false-fail from `grep -q` closing a `pipefail` pipeline.
- Assert target container IDs change while image IDs remain the reviewed IDs.
- Assert unrelated container IDs remain identical.
- Assert target health checks pass and recent startup logs contain no
  ERROR/FATAL/panic lines.
- For a manual Backend image, assert target `runtime`, `mmchat:mmchat`, and
  `/usr/local/bin/mm-chat-api` before changing the live environment.
- Assert identity/readiness probe paths are registered by the selected image;
  a route-level failure must exercise behavior rollback before corrected retry.
- For a temporary authenticated Session, prove insert, request authorization,
  exact cleanup, and zero remaining marker rows. Execute psql-variable SQL from
  stdin rather than assuming `-c` interpolation.
- Assert the database migration version is unchanged for a flag-only operation.
- Assert the protected environment and logical dump have mode `0600`, validate
  their hashes/catalog, and compare persistent row counts before and after.
- If the live database is unexpectedly empty, assert an exclusive rollback,
  same-major schema-first rehearsal, exact foreign-key orphan proof, explicit
  migration-range authorization, and post-selection counts before recreation.
- Assert backend readiness components separately; for storage IAM repair, pin
  the initializer image/config, keep storage and unrelated IDs unchanged, and
  prove bucket access with the exact configured application identity.
- Assert one-off wrapper tests cover both Compose-run capability branches,
  positive `--build` is absent, `--pull never` is present, helper image identity
  is pinned, and pre-provider failures export no credential or artifact.
- Inject a failure after each live phase marker and assert the parent-shell trap
  performs the expected append-only disable and targeted recreation exactly
  once; pipeline logging must not own rollback state.
- Replay captured health payloads with and without optional fields, asserting
  required status/count behavior rather than undocumented text.
- Run the exact verifier SQL against the disposable PostgreSQL rehearsal,
  including every `UNION` arm with `ORDER BY`/`LIMIT`.
- For Tailwind UI changes, inspect the packaged CSS declarations or rendered
  computed style; do not use literal source class names as runtime evidence.
- For fixes reported against a live page, record the pre/post Frontend image
  IDs, require only the Frontend container ID to change, and fetch the decisive
  immutable asset through the live HTTP edge after recreation.
- For every pre-deploy recovery set, assert both SHA-256 sidecars, exact
  live/restored migration-manifest equality, representative table-count
  equality, sampled Knowledge/MCP/Skill object `stat` success, and zero
  temporary database/bucket/staging/sample residue.
- If strict promotion preflight is intentionally not the active topology,
  assert the rendered candidate differs from the running service only in the
  explicitly authorized release/image inputs and that unrelated container IDs
  remain byte-identical.

### 7. Wrong vs correct

#### Wrong

```bash
# backend:local may no longer be the image used by the live container.
docker compose up -d --no-build --force-recreate backend memory-worker
```

```bash
# Wrong: the Dockerfile's final stage is MCP Runner, not Backend API.
docker build --tag mm-chat/backend:candidate backend
```

```bash
# Wrong: current source has the command, therefore the live admin must have it.
docker compose build admin
docker compose run admin new-read-only-helper
```

```bash
# Wrong: an assumed route is treated as readiness authority while flags stay on.
curl --fail http://127.0.0.1:8080/v1/auth/me
```

```bash
# Wrong: Backend is profile-gated and may be absent from this render.
docker compose config --images | grep -q 'mm-chat/backend:candidate'

# Wrong: psql variables remain literal in this -c execution mode.
psql -v sid="$session_id" -c "DELETE FROM sessions WHERE id = :'sid'::uuid"
```

```bash
# Wrong: rollout_phase is changed only in a pipeline subshell; EXIT sees "pre".
rollout_phase=pre
produce | while read -r line; do rollout_phase=enabled; done
trap '[[ "$rollout_phase" == enabled ]] && disable_preview' EXIT
```

```sql
-- Wrong: ORDER BY/LIMIT belongs to an unparenthesized UNION arm.
SELECT id FROM preview_events ORDER BY created_at DESC LIMIT 1
UNION ALL
SELECT id FROM preview_events WHERE enabled = false;
```

```bash
# Wrong: "reason" is optional in the actual health response contract.
jq -e '.status == "ready" and .reason == "memory_ready"' health.json
```

```bash
# Wrong: Tailwind may escape or transform the source class in packaged assets.
grep -R -q 'max-h-\[39.75rem\]' /app/.next/static
```

```sql
-- Wrong: this copied list stops being restore authority after the next release.
WITH expected(version) AS (VALUES (1), (2), (3), (4), (5))
SELECT * FROM expected EXCEPT SELECT version FROM schema_migrations;
```

#### Correct

```bash
live_image_id="$(docker inspect --format '{{.Image}}' "$(docker compose ps -q backend)")"
docker tag "$live_image_id" mm-chat/backend:retained-before-flag-change
printf '%s\n' 'BACKEND_IMAGE=mm-chat/backend:retained-before-flag-change' >> .env.candidate
docker compose --env-file .env.candidate config --quiet
docker compose --env-file .env.candidate \
  up -d --no-build --no-deps --force-recreate backend memory-worker
```

```bash
# Correct: select and inspect the Backend API runtime before recreation.
docker build --pull=false --target runtime \
  --tag mm-chat/backend:candidate backend
docker image inspect mm-chat/backend:candidate \
  --format 'user={{.Config.User}} cmd={{json .Config.Cmd}}'
```

```bash
# Correct helper boundary: explicit reviewed image, no live recreation.
docker compose --env-file .env.reviewed run --rm --no-deps --pull never admin \
  new-read-only-helper
```

```bash
# Correct: restore the protected behavior environment first, then verify the
# route registered by the selected image before a Provider-free retry.
docker compose --env-file .env.rollback \
  up -d --no-build --no-deps --force-recreate backend memory-worker
curl --fail http://127.0.0.1:8080/v1/me
```

```bash
# Correct: activate the exact profile, capture bounded output, then match it.
images="$(docker compose --profile app config --images)"
grep -Fx 'mm-chat/backend:candidate' <<<"$images"

# Correct: psql variables are expanded while processing stdin.
psql -v sid="$session_id" <<'SQL'
DELETE FROM sessions WHERE id = :'sid'::uuid;
SQL
```

```bash
# Correct: the loop and phase assignment execute in the parent shell.
rollout_phase=pre
while read -r line; do rollout_phase=enabled; done < <(produce)
trap '[[ "$rollout_phase" == enabled ]] && disable_preview' EXIT
```

```sql
-- Correct: each ordered/limited set-operation arm owns its clauses.
(SELECT id FROM preview_events ORDER BY created_at DESC LIMIT 1)
UNION ALL
(SELECT id FROM preview_events WHERE enabled = false);
```

```bash
# Correct: bind the verifier to required health fields.
jq -e '.status == "ready" and .readyCount == 2 and
       .pendingCount == 0 and .failedCount == 0' health.json
```

```bash
# Correct: verify the compiled runtime declaration.
grep -R -F -q 'max-height:39.75rem' /app/.next/static
```

```bash
# Correct: compare the complete authority observed at drill time.
psql_live ... -At -F '|' \
  -c 'SELECT version, name, checksum FROM schema_migrations ORDER BY version' \
  > live-migrations.tsv
psql_restored ... -At -F '|' \
  -c 'SELECT version, name, checksum FROM schema_migrations ORDER BY version' \
  > restored-migrations.tsv
cmp --silent live-migrations.tsv restored-migrations.tsv
```
