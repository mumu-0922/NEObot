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

### 3. Contracts

- Record each target container ID, image ID, Compose config-file label, and
  health state before stopping it.
- Resolve the exact image ID through an immutable digest or a protected retained
  tag before `--force-recreate`; a mutable fallback tag is not rollback state.
- Render the same Compose topology named by the live container labels.
- Compare the database's applied migration version with the selected binary's
  schema requirements. A flag-only restart must not run migrations to make an
  accidentally newer image start.
- Back up the active runtime environment outside Git with mode `0600` and make
  the candidate environment render successfully before downtime.
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

### 4. Validation and error matrix

| Condition | Required result |
| --- | --- |
| Running image ID has no retained digest/tag | Stop before recreation and create a protected reference while the container still exists. |
| Rendered image differs from the recorded live image during a flag-only change | Reject the candidate; pin the recorded image explicitly. |
| Selected binary requires an unapplied migration | Do not migrate implicitly; select a schema-compatible image or obtain separate migration authorization. |
| Target does not become healthy | Restore the protected environment and exact retained image, then verify health before further work. |
| Any unrelated container ID changes | Treat the operation as scope violation and investigate. |
| Persistent row count decreases | Stop, retain evidence, and restore from the protected data artifact if mutation is confirmed. |
| `compose run --no-build` is rejected by the installed CLI | Stop before credentials. Capability-detect, retain `--pull never`, omit positive `--build`, and verify the exact helper image. |
| Running admin binary lacks the required one-off command | Stop before credentials/Provider work and select an explicitly reviewed pinned helper image; do not recreate live backend. |

### 5. Good / base / bad cases

- **Good**: retain the running image by digest, render the candidate, snapshot
  persistence, recreate only named services, and prove flags, image IDs, health,
  and unrelated-container stability.
- **Base**: restart without recreation when no environment or image input
  changed; still verify the current container health.
- **Bad**: run `--force-recreate` against `backend:local`, discover afterward
  that the tag moved to a schema-incompatible image, and then run migrations to
  fit the accidental release.
- **Base helper**: the old live admin lacks the command; a pinned candidate
  image runs only the read-only helper with `--pull never`, while live service
  IDs stay unchanged.
- **Bad helper**: assume source capability exists in the running image, or call
  `compose build admin` and silently move `BACKEND_IMAGE` before export.

### 6. Tests required

- Assert the candidate Compose render names the intended pinned image and exact
  flag values.
- Assert target container IDs change while image IDs remain the reviewed IDs.
- Assert unrelated container IDs remain identical.
- Assert target health checks pass and recent startup logs contain no
  ERROR/FATAL/panic lines.
- Assert the database migration version is unchanged for a flag-only operation.
- Assert the protected environment and logical dump have mode `0600`, validate
  their hashes/catalog, and compare persistent row counts before and after.
- Assert one-off wrapper tests cover both Compose-run capability branches,
  positive `--build` is absent, `--pull never` is present, helper image identity
  is pinned, and pre-provider failures export no credential or artifact.

### 7. Wrong vs correct

#### Wrong

```bash
# backend:local may no longer be the image used by the live container.
docker compose up -d --no-build --force-recreate backend memory-worker
```

```bash
# Wrong: current source has the command, therefore the live admin must have it.
docker compose build admin
docker compose run admin new-read-only-helper
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
# Correct helper boundary: explicit reviewed image, no live recreation.
docker compose --env-file .env.reviewed run --rm --no-deps --pull never admin \
  new-read-only-helper
```
