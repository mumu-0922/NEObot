# Production deployment evidence

Date: 2026-07-27

## Release

- Feature commits: `ccad2a3`, `39672e1`, `baf0c03`
- Migration-manifest repair: `8bc0a56`
- Runtime-role grant repair: `5debc62`
- Deployment mode: source-build Docker Compose
- Built services: `backend`, `frontend`, `rag-worker`

## Backup and migration

- PostgreSQL: `backup/postgres/postgres-20260727T054132Z.dump` plus SHA-256 sidecar
- MinIO: `backup/minio/minio-20260727T054133Z.tar.gz` plus SHA-256 sidecar
- Forward-fix PostgreSQL backup:
  `backup/postgres/postgres-20260727T063025Z.dump` plus SHA-256 sidecar
- Forward-fix MinIO backup:
  `backup/minio/minio-20260727T063026Z.tar.gz` plus SHA-256 sidecar
- Migration `050_jina_runtime_retirement` was restored to its exact applied
  byte manifest; its verified checksum is
  `87302c2cf0dee5ce11388795891db4e64dfba4a7086a9906bcbaeab5397519e6`.
- Migration `051_siliconflow_tts_cache` applied successfully.
- Migration `052_tts_runtime_role_grants` applied successfully after the first
  production read-aloud exposed the missing runtime DML grant. It grants
  `SELECT, INSERT, UPDATE, DELETE` on only the two TTS tables to
  `go_api_runtime`; ownership and `TRUNCATE` remain denied.
- `tts_audio_cache` and `tts_audio_cleanup_queue` exist and both started empty.

## Runtime verification

- Backend, frontend, PostgreSQL, Redis, and RAG Worker: healthy
- Frontend `/`: `200`
- Frontend `/mm-api/ready`: `200`
- Backend `/ready`: `200`
- Backend `/health`: `200`
- RAG Worker `/health`: `{"status":"alive"}`
- Voice administration and synthesis routes are active through both Backend
  and the same-origin frontend proxy.
- A fresh dedicated Key was saved through encrypted administrator ingress. A
  bounded retest returned HTTP `200`, `audio/mpeg`, and non-empty bytes; exact
  activation returned HTTP `200`.
- Runtime config now reports `defaultTtsAvailable=true`,
  `defaultSttAvailable=false`, provider `siliconflow`, model
  `FunAudioLLM/CosyVoice2-0.5B`, and voice
  `FunAudioLLM/CosyVoice2-0.5B:claire`.
- The live API role has all four required DML privileges on both TTS tables and
  no `TRUNCATE`; migration head is `052`.
- The active standalone deployment leaves `AUTH_REQUIRE_LOGIN` unset, which is
  the fixed Development Owner mode. Therefore a request without a Bearer token
  is still actor-authorized by middleware; it is not an anonymous-access proof.
- The final authorized production replay created one isolated conversation and
  one exact source message. The first synthesis returned HTTP `200`,
  `cached=false`, `audio/mpeg`, and 65,023 bytes. The File content route returned
  the same MIME and exact byte count; independent `file` detection also
  classified it as audio.
- The identical second request returned HTTP `200`, `cached=true`, and the same
  file ID, content type, and size. Database evidence showed exactly one cache
  row and one active File artifact for the fixture.
- Conversation deletion followed by the startup cleanup pass reduced the cache
  and cleanup queue to zero, soft-deleted the File metadata, removed the object,
  and made the original content URL return `404`. Temporary request/audio files
  and the temporary bearer-session fixture used during mode discovery were
  removed. The formal `VOICE:SILICONFLOW` configuration remains active.
- Backend startup logs contain no TTS permission-denied or cleanup failure after
  migration `052`.

## Final source verification

- `bash mm-chat/scripts/verify-standalone.sh --full` passed from an isolated
  copy: Frontend format/lint/typecheck/build plus 193 files/943 tests, all Go
  packages, and RAG Ruff/mypy plus 1,906 passed/7 skipped.
- If the deployment later enables `AUTH_REQUIRE_LOGIN=true`, repeat only the
  offline/session-bound ownership matrix (missing token and cross-user File
  access); do not repeat a paid synthesis merely to prove middleware behavior.

No data rollback was required. The missing runtime grant was repaired forward
with migration `052`, and every live fixture was reclaimed through the product
cleanup boundary. If a later rollback becomes necessary, use the recorded
Postgres/MinIO backups and the migration `052`/`051` rollback contract; after
generated audio exists, prefer another forward fix.
