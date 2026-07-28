#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
preflight="${script_dir}/preflight-single-server.sh"
production_compose="${script_dir}/compose-single-server-production.sh"
restore_drill="${script_dir}/restore-minio-drill.sh"
postgres_backup="${script_dir}/backup-postgres.sh"
minio_backup="${script_dir}/backup-minio.sh"
provider_keyring_init="${script_dir}/init-provider-keyring.sh"
provider_keyring_rotate="${script_dir}/rotate-provider-keyring.sh"
example="${project_dir}/.env.single-server.example"
temp_dir="$(mktemp -d)"
trap 'rm -rf "${temp_dir}"' EXIT

assert_rejected() {
  local env_file="$1"
  local expected="$2"
  local output
  if output="$(${preflight} "${env_file}" 2>&1)"; then
    echo "preflight test: expected rejection" >&2
    exit 1
  fi
  if [[ "${output}" != *"${expected}"* ]]; then
    echo "preflight test: unexpected rejection reason" >&2
    exit 1
  fi
  if grep -Eqi \
    'test-(migrator|api|memory-worker|rag-worker|rag-replay|redis)-password|test-minio-(root|app)-password|test-provider-key|different-password' \
    <<<"${output}"; then
    echo "preflight test: rejection leaked a fixture secret" >&2
    exit 1
  fi
}

assert_rejected "${example}" "example env cannot be promoted"

for backup_script in "${postgres_backup}" "${minio_backup}"; do
  if ! grep -Fx 'umask 077' "${backup_script}" >/dev/null; then
    echo "preflight test: backup scripts must create owner-only artifacts" >&2
    exit 1
  fi
done

generated_keyring="${temp_dir}/generated/provider-keyring.json"
"${provider_keyring_init}" "${generated_keyring}" "test-generated-v1" >/dev/null
if [[ "$(stat -c '%a' "${generated_keyring}")" != "600" ]]; then
  echo "preflight test: generated provider keyring must use mode 600" >&2
  exit 1
fi
if [[ "$(stat -c '%a' "$(dirname "${generated_keyring}")")" != "700" ]]; then
  echo "preflight test: generated provider keyring parent must use mode 700" >&2
  exit 1
fi
python3 - "${generated_keyring}" <<'PY'
import base64
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
encoded = payload["keys"][0]["key"]
decoded = base64.urlsafe_b64decode(encoded + "=" * (-len(encoded) % 4))
if payload["v"] != 1 or payload["activeKid"] != "test-generated-v1" or len(decoded) != 32:
    raise SystemExit("preflight test: generated provider keyring is invalid")
PY
if "${provider_keyring_init}" "${generated_keyring}" "test-generated-v2" >/dev/null 2>&1; then
  echo "preflight test: provider keyring init overwrote an existing target" >&2
  exit 1
fi

mkdir "${temp_dir}/real-parent"
chmod 700 "${temp_dir}/real-parent"
ln -s "${temp_dir}/real-parent" "${temp_dir}/linked-parent"
if "${provider_keyring_init}" \
  "${temp_dir}/linked-parent/provider-keyring.json" \
  "test-generated-v2" >/dev/null 2>&1; then
  echo "preflight test: provider keyring init accepted a symlink parent" >&2
  exit 1
fi

prepared_keyring="${temp_dir}/generated/provider-keyring.next.json"
"${provider_keyring_rotate}" prepare \
  "${generated_keyring}" "${prepared_keyring}" "test-generated-v2" >/dev/null
pruned_keyring="${temp_dir}/generated/provider-keyring.final.json"
"${provider_keyring_rotate}" prune \
  "${prepared_keyring}" "${pruned_keyring}" >/dev/null
python3 - "${generated_keyring}" "${prepared_keyring}" "${pruned_keyring}" <<'PY'
import json
import sys
from pathlib import Path

source, prepared, pruned = [json.loads(Path(path).read_text()) for path in sys.argv[1:]]
if source["activeKid"] != "test-generated-v1" or len(source["keys"]) != 1:
    raise SystemExit("preflight test: rotation changed the source keyring")
if prepared["activeKid"] != "test-generated-v2" or len(prepared["keys"]) != 2:
    raise SystemExit("preflight test: prepared keyring did not retain the old key")
if pruned["activeKid"] != "test-generated-v2" or len(pruned["keys"]) != 1:
    raise SystemExit("preflight test: pruned keyring retained an old key")
if prepared["keys"][0] != pruned["keys"][0]:
    raise SystemExit("preflight test: prune changed the active key")
for path in sys.argv[1:]:
    if Path(path).stat().st_mode & 0o777 != 0o600:
        raise SystemExit("preflight test: rotated keyring mode is not 600")
PY
if "${provider_keyring_rotate}" prepare \
  "${generated_keyring}" "${prepared_keyring}" "test-generated-v3" >/dev/null 2>&1; then
  echo "preflight test: rotation overwrote an existing target" >&2
  exit 1
fi
if "${provider_keyring_rotate}" prepare \
  "${generated_keyring}" "${temp_dir}/generated/duplicate.json" \
  "test-generated-v1" >/dev/null 2>&1; then
  echo "preflight test: rotation accepted a duplicate key id" >&2
  exit 1
fi

mkdir "${temp_dir}/insecure-parent"
chmod 755 "${temp_dir}/insecure-parent"
if "${provider_keyring_init}" \
  "${temp_dir}/insecure-parent/provider-keyring.json" \
  "test-generated-v2" >/dev/null 2>&1; then
  echo "preflight test: provider keyring init accepted an insecure parent" >&2
  exit 1
fi

if grep -F -- '--ignore-existing' "${restore_drill}" >/dev/null; then
  echo "preflight test: restore drill must fail on bucket-name collision" >&2
  exit 1
fi

valid="${temp_dir}/valid.env"
provider_keyring="${temp_dir}/provider-keyring.json"
printf '%s\n' '{"v":1,"activeKid":"test-v1","keys":[{"kid":"test-v1","key":"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY"}]}' >"${provider_keyring}"
chmod 600 "${provider_keyring}"
sed \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-postgres@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-postgres@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd|' \
  -e 's|replace-with-release-id|git-deadbeef|' \
  -e "s|replace-with-host-uid|$(id -u)|" \
  -e "s|replace-with-host-gid|$(id -g)|" \
  -e 's|change-me-migrator-postgres|test-migrator-password|g' \
  -e 's|change-me-api-postgres|test-api-password|g' \
  -e 's|change-me-memory-worker-postgres|test-memory-worker-password|g' \
  -e 's|change-me-rag-worker-postgres|test-rag-worker-password|g' \
  -e 's|change-me-rag-replay-postgres|test-rag-replay-password|g' \
  -e 's|change-me-redis|test-redis-password|g' \
  -e 's|change-me-minio-root-secret|test-minio-root-password|g' \
  -e 's|change-me-minio-user-secret|test-minio-app-password|g' \
  -e 's|change-me-base64-32-byte-random-key|MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=|g' \
  -e 's|https://change-me.example/invites/accept|https://chat.internal/invites/accept|' \
  -e "s|^PROVIDER_SECRET_KEYRING_SOURCE=.*|PROVIDER_SECRET_KEYRING_SOURCE=${provider_keyring}|" \
  "${example}" >"${valid}"
chmod 600 "${valid}"
"${preflight}" "${valid}" >/dev/null

for retired_provider_env in \
  RAG_MINERU_API_TOKEN \
  DEFAULT_MINERU_API_TOKEN \
  RAG_JINA_API_KEY \
  DEFAULT_JINA_API_KEY \
  RAG_QUERY_GATEWAY_URL \
  RAG_RERANK_GATEWAY_URL \
  DEFAULT_ELEVENLABS_API_KEY \
  DEFAULT_ELEVENLABS_STT_MODEL \
  DEFAULT_ELEVENLABS_TTS_MODEL \
  DEFAULT_ELEVENLABS_TTS_VOICE_ID \
  DEFAULT_MIMO_API_KEY \
  DEFAULT_MIMO_STT_MODEL \
  DEFAULT_MIMO_TTS_MODEL \
  DEFAULT_MIMO_TTS_VOICE_ID; do
  retired_env_file="${temp_dir}/retired-${retired_provider_env}.env"
  cp "${valid}" "${retired_env_file}"
  printf '%s=%s\n' "${retired_provider_env}" "retired-fixture" >>"${retired_env_file}"
  chmod 600 "${retired_env_file}"
  assert_rejected "${retired_env_file}" "${retired_provider_env} is retired"
done

quoted_byok_pem="${temp_dir}/quoted-byok-pem.env"
sed "s|^BYOK_PRIVATE_KEY_PEM=.*|BYOK_PRIVATE_KEY_PEM='-----BEGIN PRIVATE KEY-----\\\\nYWJj\\\\n-----END PRIVATE KEY-----'|" \
  "${valid}" >"${quoted_byok_pem}"
chmod 600 "${quoted_byok_pem}"
"${preflight}" "${quoted_byok_pem}" >/dev/null

invalid_byok_pem="${temp_dir}/invalid-byok-pem.env"
sed "s|^BYOK_PRIVATE_KEY_PEM=.*|BYOK_PRIVATE_KEY_PEM='not-a-private-key'|" \
  "${valid}" >"${invalid_byok_pem}"
chmod 600 "${invalid_byok_pem}"
assert_rejected "${invalid_byok_pem}" "BYOK_PRIVATE_KEY_PEM"

runtime_uid_mismatch="${temp_dir}/runtime-uid-mismatch.env"
sed "s|^MM_CHAT_RUNTIME_UID=.*|MM_CHAT_RUNTIME_UID=$(( $(id -u) + 1 ))|" \
  "${valid}" >"${runtime_uid_mismatch}"
chmod 600 "${runtime_uid_mismatch}"
assert_rejected \
  "${runtime_uid_mismatch}" \
  "MM_CHAT_RUNTIME_UID must match the invoking user"

runtime_gid_mismatch="${temp_dir}/runtime-gid-mismatch.env"
sed "s|^MM_CHAT_RUNTIME_GID=.*|MM_CHAT_RUNTIME_GID=$(( $(id -g) + 1 ))|" \
  "${valid}" >"${runtime_gid_mismatch}"
chmod 600 "${runtime_gid_mismatch}"
assert_rejected \
  "${runtime_gid_mismatch}" \
  "MM_CHAT_RUNTIME_GID must match the invoking user's primary group"

invalid_runtime_uid="${temp_dir}/invalid-runtime-uid.env"
sed 's|^MM_CHAT_RUNTIME_UID=.*|MM_CHAT_RUNTIME_UID=root|' \
  "${valid}" >"${invalid_runtime_uid}"
chmod 600 "${invalid_runtime_uid}"
assert_rejected \
  "${invalid_runtime_uid}" \
  "MM_CHAT_RUNTIME_UID must be a positive numeric ID"

missing_migration="${temp_dir}/missing-migration.env"
grep -v '^MIGRATION_DATABASE_URL=' "${valid}" >"${missing_migration}"
chmod 600 "${missing_migration}"
assert_rejected "${missing_migration}" "MIGRATION_DATABASE_URL is required"

insecure="${temp_dir}/insecure.env"
cp "${valid}" "${insecure}"
chmod 644 "${insecure}"
assert_rejected "${insecure}" "must not be group/world accessible"

symlinked="${temp_dir}/symlinked.env"
ln -s "${valid}" "${symlinked}"
assert_rejected "${symlinked}" "must not be a symbolic link"

missing_provider_keyring="${temp_dir}/missing-provider-keyring.env"
sed 's|^PROVIDER_SECRET_KEYRING_SOURCE=.*|PROVIDER_SECRET_KEYRING_SOURCE=/missing/provider-keyring.json|' \
  "${valid}" >"${missing_provider_keyring}"
chmod 600 "${missing_provider_keyring}"
assert_rejected \
  "${missing_provider_keyring}" \
  "PROVIDER_SECRET_KEYRING_SOURCE file is unavailable"

insecure_keyring="${temp_dir}/insecure-provider-keyring.json"
cp "${provider_keyring}" "${insecure_keyring}"
chmod 644 "${insecure_keyring}"
insecure_keyring_env="${temp_dir}/insecure-provider-keyring.env"
sed "s|^PROVIDER_SECRET_KEYRING_SOURCE=.*|PROVIDER_SECRET_KEYRING_SOURCE=${insecure_keyring}|" \
  "${valid}" >"${insecure_keyring_env}"
chmod 600 "${insecure_keyring_env}"
assert_rejected \
  "${insecure_keyring_env}" \
  "PROVIDER_SECRET_KEYRING_SOURCE must use mode 600"

malformed_keyring="${temp_dir}/malformed-provider-keyring.json"
printf '%s\n' '{"v":1,"activeKid":"missing","keys":[]}' >"${malformed_keyring}"
chmod 600 "${malformed_keyring}"
malformed_keyring_env="${temp_dir}/malformed-provider-keyring.env"
sed "s|^PROVIDER_SECRET_KEYRING_SOURCE=.*|PROVIDER_SECRET_KEYRING_SOURCE=${malformed_keyring}|" \
  "${valid}" >"${malformed_keyring_env}"
chmod 600 "${malformed_keyring_env}"
assert_rejected \
  "${malformed_keyring_env}" \
  "PROVIDER_SECRET_KEYRING_SOURCE is invalid"

migration_user_mismatch="${temp_dir}/migration-user-mismatch.env"
sed 's|^POSTGRES_USER=.*|POSTGRES_USER=different_migrator|' \
  "${valid}" >"${migration_user_mismatch}"
chmod 600 "${migration_user_mismatch}"
assert_rejected \
  "${migration_user_mismatch}" \
  "MIGRATION_DATABASE_URL user does not match POSTGRES_USER"

invalid_memory_lexical_shadow="${temp_dir}/invalid-memory-lexical-shadow.env"
sed 's|^MEMORY_LEXICAL_SHADOW_ENABLED=false$|MEMORY_LEXICAL_SHADOW_ENABLED=maybe|' \
  "${valid}" >"${invalid_memory_lexical_shadow}"
chmod 600 "${invalid_memory_lexical_shadow}"
assert_rejected \
  "${invalid_memory_lexical_shadow}" \
  "MEMORY_LEXICAL_SHADOW_ENABLED must be true or false"

invalid_memory_hybrid_shadow="${temp_dir}/invalid-memory-hybrid-shadow.env"
sed 's|^MEMORY_HYBRID_SHADOW_ENABLED=false$|MEMORY_HYBRID_SHADOW_ENABLED=maybe|' \
  "${valid}" >"${invalid_memory_hybrid_shadow}"
chmod 600 "${invalid_memory_hybrid_shadow}"
assert_rejected \
  "${invalid_memory_hybrid_shadow}" \
  "MEMORY_HYBRID_SHADOW_ENABLED must be true or false"

migration_password_mismatch="${temp_dir}/migration-password-mismatch.env"
sed 's|test-migrator-password@postgres|different-password@postgres|' \
  "${valid}" >"${migration_password_mismatch}"
chmod 600 "${migration_password_mismatch}"
assert_rejected \
  "${migration_password_mismatch}" \
  "MIGRATION_DATABASE_URL password does not match POSTGRES_PASSWORD"

for database_key in \
  MIGRATION_DATABASE_URL \
  DATABASE_URL \
  MEMORY_WORKER_DATABASE_URL \
  RAG_WORKER_DATABASE_URL \
  RAG_REPLAY_DATABASE_URL; do
  invalid_database_url="${temp_dir}/invalid-${database_key}.env"
  sed "s|^${database_key}=.*|${database_key}=not-a-postgres-url|" \
    "${valid}" >"${invalid_database_url}"
  chmod 600 "${invalid_database_url}"
  assert_rejected "${invalid_database_url}" "${database_key} must be a PostgreSQL URL"

  empty_database_user="${temp_dir}/empty-user-${database_key}.env"
  sed "/^${database_key}=/ s|postgres://[^:]*:|postgres://:|" \
    "${valid}" >"${empty_database_user}"
  chmod 600 "${empty_database_user}"
  assert_rejected \
    "${empty_database_user}" \
    "${database_key} must be a PostgreSQL URL with user and password"

  empty_database_password="${temp_dir}/empty-password-${database_key}.env"
  sed "/^${database_key}=/ s|:[^:@]*@|:@|" \
    "${valid}" >"${empty_database_password}"
  chmod 600 "${empty_database_password}"
  assert_rejected \
    "${empty_database_password}" \
    "${database_key} must be a PostgreSQL URL with user and password"
done

mismatched_migration_host="${temp_dir}/host-MIGRATION_DATABASE_URL.env"
sed '/^MIGRATION_DATABASE_URL=/ s|@postgres:|@other-postgres:|' \
  "${valid}" >"${mismatched_migration_host}"
chmod 600 "${mismatched_migration_host}"
assert_rejected \
  "${mismatched_migration_host}" \
  "DATABASE_URL host must match MIGRATION_DATABASE_URL"

for database_key in \
  DATABASE_URL \
  MEMORY_WORKER_DATABASE_URL \
  RAG_WORKER_DATABASE_URL \
  RAG_REPLAY_DATABASE_URL; do
  mismatched_host="${temp_dir}/host-${database_key}.env"
  sed "/^${database_key}=/ s|@postgres:|@other-postgres:|" \
    "${valid}" >"${mismatched_host}"
  chmod 600 "${mismatched_host}"
  assert_rejected \
    "${mismatched_host}" \
    "${database_key} host must match MIGRATION_DATABASE_URL"
done

for database_key in \
  MIGRATION_DATABASE_URL \
  DATABASE_URL \
  MEMORY_WORKER_DATABASE_URL \
  RAG_WORKER_DATABASE_URL \
  RAG_REPLAY_DATABASE_URL; do
  mismatched_database="${temp_dir}/database-${database_key}.env"
  sed "/^${database_key}=/ s|/neo_chat?|/other_database?|" \
    "${valid}" >"${mismatched_database}"
  chmod 600 "${mismatched_database}"
  assert_rejected \
    "${mismatched_database}" \
    "${database_key} database does not match POSTGRES_DB"
done

for interpolation_value in \
  '${UNSET}' \
  '${UNSET:-fallback}' \
  '${UNSET-fallback}' \
  '$$'; do
  interpolation="${temp_dir}/interpolation-$RANDOM.env"
  grep -v '^S3_SECRET_ACCESS_KEY=' "${valid}" >"${interpolation}"
  printf 'S3_SECRET_ACCESS_KEY=%s\n' "${interpolation_value}" >>"${interpolation}"
  chmod 600 "${interpolation}"
  assert_rejected \
    "${interpolation}" \
    "uses unsupported quoting, escaping, comment, or interpolation syntax"
done

for invalid_rag_image in \
  'mm-chat/rag:release-1' \
  'rag@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' \
  'ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:abc'; do
  image_env="${temp_dir}/rag-image-$RANDOM.env"
  sed "s|^RAG_IMAGE=.*|RAG_IMAGE=${invalid_rag_image}|" "${valid}" >"${image_env}"
  chmod 600 "${image_env}"
  assert_rejected "${image_env}" "RAG_IMAGE must use a full immutable sha256 registry digest"
done

for invalid_frontend_image in \
  'mm-chat/frontend:release-1' \
  'frontend@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc' \
  'ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:abc'; do
  image_env="${temp_dir}/frontend-image-$RANDOM.env"
  sed "s|^FRONTEND_IMAGE=.*|FRONTEND_IMAGE=${invalid_frontend_image}|" "${valid}" >"${image_env}"
  chmod 600 "${image_env}"
  assert_rejected "${image_env}" "FRONTEND_IMAGE must use a full immutable sha256 registry digest"
done

for invalid_postgres_image in \
  'mm-chat/postgres:17' \
  'postgres@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd' \
  'ghcr.io/mumu-0922/neobot-mm-chat-postgres@sha256:abc'; do
  image_env="${temp_dir}/postgres-image-$RANDOM.env"
  sed "s|^POSTGRES_IMAGE=.*|POSTGRES_IMAGE=${invalid_postgres_image}|" \
    "${valid}" >"${image_env}"
  chmod 600 "${image_env}"
  assert_rejected \
    "${image_env}" \
    "POSTGRES_IMAGE must use a full immutable sha256 registry digest"
done

old_postgres_data="${temp_dir}/old-postgres-data.env"
sed 's|^POSTGRES_DATA_DIR=.*|POSTGRES_DATA_DIR=./data/postgres|' \
  "${valid}" >"${old_postgres_data}"
chmod 600 "${old_postgres_data}"
assert_rejected "${old_postgres_data}" "POSTGRES_DATA_DIR must be ./data/postgres17"

same_memory_principal="${temp_dir}/same-memory-principal.env"
sed 's|postgres://memory_worker:test-memory-worker-password@|postgres://neo_chat_api:test-memory-worker-password@|' \
  "${valid}" >"${same_memory_principal}"
chmod 600 "${same_memory_principal}"
assert_rejected "${same_memory_principal}" "must use distinct database principals"

same_worker_principal="${temp_dir}/same-worker-principal.env"
sed 's|postgres://rag_worker:test-rag-worker-password@|postgres://neo_chat_api:test-rag-worker-password@|' \
  "${valid}" >"${same_worker_principal}"
chmod 600 "${same_worker_principal}"
assert_rejected "${same_worker_principal}" "must use distinct database principals"

same_replay_principal="${temp_dir}/same-replay-principal.env"
sed 's|postgres://rag_replay:test-rag-replay-password@|postgres://rag_worker:test-rag-replay-password@|' \
  "${valid}" >"${same_replay_principal}"
chmod 600 "${same_replay_principal}"
assert_rejected "${same_replay_principal}" "must use distinct database principals"

same_api_principal="${temp_dir}/same-api-principal.env"
sed 's|postgres://neo_chat_api:test-api-password@|postgres://neo_chat_migrator:test-api-password@|' \
  "${valid}" >"${same_api_principal}"
chmod 600 "${same_api_principal}"
assert_rejected "${same_api_principal}" "must use distinct database principals"

same_api_password="${temp_dir}/same-api-password.env"
sed 's|test-api-password@postgres|test-migrator-password@postgres|' \
  "${valid}" >"${same_api_password}"
chmod 600 "${same_api_password}"
assert_rejected "${same_api_password}" "database principals must use distinct passwords"

same_memory_password="${temp_dir}/same-memory-password.env"
sed 's|test-memory-worker-password@postgres|test-api-password@postgres|' \
  "${valid}" >"${same_memory_password}"
chmod 600 "${same_memory_password}"
assert_rejected "${same_memory_password}" "database principals must use distinct passwords"

same_rag_password="${temp_dir}/same-rag-password.env"
sed 's|test-rag-replay-password@postgres|test-rag-worker-password@postgres|' \
  "${valid}" >"${same_rag_password}"
chmod 600 "${same_rag_password}"
assert_rejected "${same_rag_password}" "database principals must use distinct passwords"

dispatch_enabled="${temp_dir}/dispatch-enabled.env"
sed 's|^RAG_WORKER_DISPATCH_ENABLED=false$|RAG_WORKER_DISPATCH_ENABLED=true|' \
  "${valid}" >"${dispatch_enabled}"
chmod 600 "${dispatch_enabled}"
assert_rejected "${dispatch_enabled}" "must remain false in Phase 15.2B"

stages_enabled="${temp_dir}/stages-enabled.env"
sed 's|^RAG_WORKER_JOB_STAGES=$|RAG_WORKER_JOB_STAGES=parse|' \
  "${valid}" >"${stages_enabled}"
chmod 600 "${stages_enabled}"
assert_rejected "${stages_enabled}" "must remain empty in Phase 15.2B"

for invalid_image in \
  'mm-chat/backend:release-1' \
  'backend@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' \
  'ghcr.io//backend@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' \
  'ghcr.io:/backend@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' \
  'ghcr.io/mumu-0922/neobot-mm-chat@sha256:abc' \
  'ghcr.io/mumu-0922/neobot-mm-chat@sha256:gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg'; do
  image_env="${temp_dir}/image-$RANDOM.env"
  sed "s|^BACKEND_IMAGE=.*|BACKEND_IMAGE=${invalid_image}|" "${valid}" >"${image_env}"
  chmod 600 "${image_env}"
  assert_rejected "${image_env}" "must use a full immutable sha256 registry digest"
done

for invalid_invite in \
  'https://user:password@chat.internal/invites/accept' \
  'https://chat.internal/invites/accept#fragment' \
  'https://chat.internal/invites/accept?ToKeN=value' \
  'https://bad host/invites/accept' \
  'https://chat.internal:invalid/invites/accept'; do
  invite_env="${temp_dir}/invite-$RANDOM.env"
  sed "s|^TEAM_INVITE_ACCEPT_URL_BASE=.*|TEAM_INVITE_ACCEPT_URL_BASE=${invalid_invite}|" \
    "${valid}" >"${invite_env}"
  chmod 600 "${invite_env}"
  assert_rejected "${invite_env}" "TEAM_INVITE_ACCEPT_URL_BASE"
done

for unsupported_assignment in \
  'REDIS_PASSWORD="quoted-value"' \
  'REDIS_PASSWORD=value\\twith-escape' \
  'REDIS_PASSWORD=value#inline-comment' \
  'export REDIS_PASSWORD=value'; do
  syntax_env="${temp_dir}/syntax-$RANDOM.env"
  grep -v '^REDIS_PASSWORD=' "${valid}" >"${syntax_env}"
  printf '%s\n' "${unsupported_assignment}" >>"${syntax_env}"
  chmod 600 "${syntax_env}"
  assert_rejected "${syntax_env}" "unsupported"
done

duplicate="${temp_dir}/duplicate.env"
cp "${valid}" "${duplicate}"
printf '\nREDIS_PASSWORD=second-value\n' >>"${duplicate}"
chmod 600 "${duplicate}"
assert_rejected "${duplicate}" "duplicate env name"

reserved="${temp_dir}/reserved.env"
cp "${valid}" "${reserved}"
printf '\nCOMPOSE_FILE=alternate.yml\n' >>"${reserved}"
chmod 600 "${reserved}"
assert_rejected "${reserved}" "reserved env name"

rendered="$({
  FRONTEND_IMAGE=mm-chat/frontend:host-override \
  BACKEND_IMAGE=mm-chat/backend:host-override \
  RAG_IMAGE=mm-chat/rag:host-override \
  POSTGRES_PASSWORD=host-override-password \
  MIGRATION_DATABASE_URL=postgres://override:override@override:5432/override \
  DATABASE_URL=postgres://override:override@override:5432/override \
    "${production_compose}" "${valid}" \
      --profile app --profile ops --profile memory-worker --profile rag-worker --profile rag-ops \
      config --format json
} 2>"${temp_dir}/production-compose.stderr")"
python3 - "${rendered}" "$(id -u):$(id -g)" <<'PY'
import json
import sys

config = json.loads(sys.argv[1])
runtime_user = sys.argv[2]
services = config["services"]
want_postgres_image = (
    "ghcr.io/mumu-0922/neobot-mm-chat-postgres@sha256:"
    + "d" * 64
)
postgres = services["postgres"]
assert postgres["image"] == want_postgres_image
assert "build" not in postgres
assert len(postgres["volumes"]) == 1
assert postgres["volumes"][0]["type"] == "bind"
assert postgres["volumes"][0]["source"].endswith("/mm-chat/data/postgres17")
assert postgres["volumes"][0]["target"] == "/var/lib/postgresql/data"
assert int(postgres["mem_limit"]) == 1024 * 1024 * 1024
assert float(postgres["cpus"]) == 2
postgres_command = " ".join(postgres["command"])
for setting in (
    "shared_preload_libraries=pg_textsearch",
    "shared_buffers=128MB",
    "work_mem=4MB",
    "maintenance_work_mem=64MB",
    "max_connections=30",
):
    assert setting in postgres_command
want_frontend_image = (
    "ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:"
    + "c" * 64
)
frontend = services["frontend"]
assert frontend["image"] == want_frontend_image
assert "build" not in frontend
assert frontend["profiles"] == ["app"]
assert frontend["read_only"] is True
assert frontend["init"] is True
assert frontend["cap_drop"] == ["ALL"]
assert "no-new-privileges:true" in frontend["security_opt"]
assert frontend["depends_on"] == {
    "backend": {
        "condition": "service_healthy",
        "required": True,
    }
}
assert frontend["environment"]["NEXT_PUBLIC_API_MODE"] == "server"
assert frontend["environment"]["NEXT_PUBLIC_API_BASE_URL"] == "/mm-api"
assert frontend["environment"]["MM_CHAT_BACKEND_INTERNAL_URL"] == "http://backend:8080"
assert list(frontend["networks"]) == ["private"]
want_image = (
    "ghcr.io/mumu-0922/neobot-mm-chat@sha256:"
    + "a" * 64
)
for name in ("backend", "memory-worker", "migrate", "admin"):
    service = services[name]
    assert service["image"] == want_image, (name, service["image"])
    assert "build" not in service, name
assert list(services["backend"]["networks"]) == ["private", "rag-private"]
assert services["backend"]["user"] == runtime_user
assert services["memory-worker"]["user"] == runtime_user
assert services["admin"]["user"] == runtime_user
for name in ("backend", "memory-worker", "admin"):
    assert services[name]["secrets"] == [
        {
            "source": "mm_chat_provider_keyring",
            "target": "mm_chat_provider_keyring",
        }
    ]
assert services["postgres"]["environment"] == {
    "POSTGRES_DB": "neo_chat",
    "POSTGRES_PASSWORD": "test-migrator-password",
    "POSTGRES_USER": "neo_chat_migrator",
}
backend_environment = services["backend"]["environment"]
assert "neo_chat_api:test-api-password@postgres" in backend_environment["DATABASE_URL"]
assert "MIGRATION_DATABASE_URL" not in backend_environment
assert backend_environment["MEMORY_LEXICAL_SHADOW_ENABLED"] == "false"
assert backend_environment["MEMORY_HYBRID_SHADOW_ENABLED"] == "false"

memory = services["memory-worker"]
assert memory["profiles"] == ["memory-worker"]
assert "ports" not in memory
assert memory["read_only"] is True
assert memory["init"] is True
assert memory["cap_drop"] == ["ALL"]
assert "no-new-privileges:true" in memory["security_opt"]
assert float(memory["cpus"]) <= 0.5
assert int(memory["pids_limit"]) == 64
assert int(memory["mem_limit"]) == 192 * 1024 * 1024
assert list(memory["networks"]) == ["private"]
assert memory["depends_on"] == {
    "postgres": {"condition": "service_healthy", "required": True}
}
memory_environment = memory["environment"]
assert "memory_worker:test-memory-worker-password@postgres" in memory_environment["MEMORY_WORKER_DATABASE_URL"]
assert memory_environment["MEMORY_HYBRID_SHADOW_ENABLED"] == "false"
assert memory_environment["PROVIDER_TIMEOUT"] == "45s"
assert memory_environment["REDIS_URL"].startswith("redis://")
assert "DATABASE_URL" not in memory_environment
assert "MIGRATION_DATABASE_URL" not in memory_environment
assert "/usr/local/bin/mm-chat-memory-worker healthcheck" in " ".join(memory["healthcheck"]["test"])

migrate_environment = services["migrate"]["environment"]
assert "neo_chat_migrator:test-migrator-password@postgres" in migrate_environment["MIGRATION_DATABASE_URL"]
assert "DATABASE_URL" not in migrate_environment

admin_environment = services["admin"]["environment"]
assert "neo_chat_api:test-api-password@postgres" in admin_environment["DATABASE_URL"]
assert "MIGRATION_DATABASE_URL" not in admin_environment
assert admin_environment["BYOK_ALLOW_EPHEMERAL_KEY"] == "false"
for service_name in ("frontend", "backend", "admin"):
    for provider_env in (
        "PROVIDER_TYPE",
        "DEFAULT_PROVIDER_NAME",
        "PROVIDER_BASE_URL",
        "PROVIDER_MODEL",
        "PROVIDER_API_KEY",
        "DEFAULT_PROVIDER_TYPE",
        "DEFAULT_PROVIDER_MODELS",
    ):
        assert provider_env not in services[service_name]["environment"]

retired_provider_env = (
    "RAG_MINERU_API_TOKEN",
    "DEFAULT_MINERU_API_TOKEN",
    "RAG_JINA_API_KEY",
    "DEFAULT_JINA_API_KEY",
    "RAG_QUERY_GATEWAY_URL",
    "RAG_RERANK_GATEWAY_URL",
    "DEFAULT_ELEVENLABS_API_KEY",
    "DEFAULT_ELEVENLABS_STT_MODEL",
    "DEFAULT_ELEVENLABS_TTS_MODEL",
    "DEFAULT_ELEVENLABS_TTS_VOICE_ID",
    "DEFAULT_MIMO_API_KEY",
    "DEFAULT_MIMO_STT_MODEL",
    "DEFAULT_MIMO_TTS_MODEL",
    "DEFAULT_MIMO_TTS_VOICE_ID",
)
for service_name, service in services.items():
    environment = service.get("environment", {})
    for retired_env in retired_provider_env:
        assert retired_env not in environment, (service_name, retired_env)

rag = services["rag-worker"]
want_rag_image = (
    "ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:"
    + "b" * 64
)
assert rag["image"] == want_rag_image
assert "build" not in rag
assert "ports" not in rag
assert rag["read_only"] is True
assert rag["init"] is True
assert rag["cap_drop"] == ["ALL"]
assert "no-new-privileges:true" in rag["security_opt"]
assert float(rag["cpus"]) <= 1
assert int(rag["pids_limit"]) == 64
assert int(rag["mem_limit"]) == 448 * 1024 * 1024
assert rag["depends_on"] == {
    "postgres": {
        "condition": "service_healthy",
        "required": True,
    }
}
environment = rag["environment"]
assert "test-rag-worker-password@postgres" in environment["RAG_WORKER_DATABASE_URL"]
assert environment["RAG_WORKER_REDIS_URL"].startswith("redis://")
assert environment["RAG_WORKER_DISPATCH_ENABLED"] == "false"
assert environment["RAG_WORKER_JOB_STAGES"] == ""
assert "RAG_REPLAY_DATABASE_URL" not in environment
assert "DATABASE_URL" not in environment
assert "MIGRATION_DATABASE_URL" not in environment
for forbidden in (
    "MINIO_ROOT_USER",
    "MINIO_ROOT_PASSWORD",
    "S3_ACCESS_KEY_ID",
    "S3_SECRET_ACCESS_KEY",
    "PROVIDER_API_KEY",
    "RAG_MINERU_API_TOKEN",
    "DEFAULT_MINERU_API_TOKEN",
    "RAG_JINA_API_KEY",
    "DEFAULT_JINA_API_KEY",
    "RAG_QUERY_GATEWAY_URL",
    "RAG_RERANK_GATEWAY_URL",
):
    assert forbidden not in environment
assert "/health" in " ".join(rag["healthcheck"]["test"])
assert list(rag["networks"]) == ["private", "rag-private"]
assert config["networks"]["rag-private"]["internal"] is True

replay = services["rag-replay"]
assert replay["image"] == want_rag_image
assert "build" not in replay
assert "ports" not in replay
assert replay["entrypoint"] == ["rag-replay"]
assert replay["environment"] == {
    "RAG_REPLAY_DATABASE_URL": (
        "postgres://rag_replay:test-rag-replay-password@"
        "postgres:5432/neo_chat?sslmode=disable"
    )
}
assert "RAG_WORKER_DATABASE_URL" not in replay["environment"]
assert "DATABASE_URL" not in replay["environment"]
assert "MIGRATION_DATABASE_URL" not in replay["environment"]
assert list(replay["networks"]) == ["rag-private"]
PY

development_rendered="$(docker compose \
  --project-directory "${project_dir}" \
  --env-file "${example}" \
  -f "${project_dir}/compose.single-server.yml" \
    --profile app --profile ops --profile memory-worker --profile rag-worker --profile rag-ops \
  config --format json)"
python3 - "${development_rendered}" <<'PY'
import json
import sys

config = json.loads(sys.argv[1])
services = config["services"]
for name in ("postgres", "frontend", "backend", "memory-worker", "migrate", "admin", "rag-worker", "rag-replay"):
    assert "build" in services[name], name
assert "MIGRATION_DATABASE_URL" not in services["backend"]["environment"]
assert "DATABASE_URL" not in services["migrate"]["environment"]
assert "MIGRATION_DATABASE_URL" in services["migrate"]["environment"]
assert "MIGRATION_DATABASE_URL" not in services["admin"]["environment"]
frontend = services["frontend"]
assert frontend["image"] == "ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:replace-with-64-lowercase-hex"
assert frontend["build"]["context"].endswith("/mm-chat/frontend")
assert frontend["build"]["args"]["NEXT_PUBLIC_API_MODE"] == "server"
assert frontend["build"]["args"]["NEXT_PUBLIC_API_BASE_URL"] == "/mm-api"
assert frontend["profiles"] == ["app"]
postgres = services["postgres"]
assert postgres["image"] == "ghcr.io/mumu-0922/neobot-mm-chat-postgres@sha256:replace-with-64-lowercase-hex"
assert postgres["build"]["context"].endswith("/mm-chat/postgres")
assert postgres["volumes"][0]["source"].endswith("/mm-chat/data/postgres17")
assert postgres["volumes"][0]["target"] == "/var/lib/postgresql/data"
assert int(postgres["mem_limit"]) == 1024 * 1024 * 1024
assert float(postgres["cpus"]) == 2
assert services["backend"]["user"] == "replace-with-host-uid:replace-with-host-gid"
assert services["memory-worker"]["user"] == "replace-with-host-uid:replace-with-host-gid"
assert services["admin"]["user"] == "replace-with-host-uid:replace-with-host-gid"
memory = services["memory-worker"]
assert memory["profiles"] == ["memory-worker"]
assert memory["build"]["context"].endswith("/mm-chat/backend")
assert "ports" not in memory
rag = services["rag-worker"]
assert rag["image"] == "ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:replace-with-64-lowercase-hex"
assert rag["build"]["context"].endswith("/mm-chat/rag")
assert rag["profiles"] == ["rag-worker"]
assert "ports" not in rag
replay = services["rag-replay"]
assert replay["profiles"] == ["rag-ops"]
assert replay["build"]["context"].endswith("/mm-chat/rag")
assert config["networks"]["rag-private"]["internal"] is True
PY

restore_rendered="$({
  "${production_compose}" "${valid}" \
    --profile restore config --format json
} 2>"${temp_dir}/restore-compose.stderr")"
python3 - "${restore_rendered}" <<'PY'
import json
import sys

service = json.loads(sys.argv[1])["services"]["minio-restore"]
assert service["image"] == "quay.io/minio/mc:RELEASE.2025-07-21T05-28-08Z"
assert service["entrypoint"] == ["/bin/sh", "/usr/local/libexec/restore-minio-drill.sh"]
assert "build" not in service
assert service["environment"]["MINIO_ROOT_PASSWORD"] == "test-minio-root-password"
PY

for forbidden_args in \
  '-f compose.single-server.yml config' \
  '--env-file alternate.env config' \
  'run -e MIGRATION_DATABASE_URL=override migrate' \
  'run -e DATABASE_URL=override migrate' \
  'run -eDATABASE_URL=override migrate' \
  'build backend' \
  'up --build backend'; do
  read -r -a args <<<"${forbidden_args}"
  if "${production_compose}" "${valid}" "${args[@]}" \
    >"${temp_dir}/forbidden.stdout" 2>"${temp_dir}/forbidden.stderr"; then
    echo "preflight test: production wrapper accepted forbidden arguments" >&2
    exit 1
  fi
  grep -F "file/env/build overrides are forbidden" \
    "${temp_dir}/forbidden.stderr" >/dev/null
done

echo "single-server preflight tests: passed"
