#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
preflight="${script_dir}/preflight-single-server.sh"
production_compose="${script_dir}/compose-single-server-production.sh"
restore_drill="${script_dir}/restore-minio-drill.sh"
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
    'test-(migrator|api|rag-worker|rag-replay|redis)-password|test-minio-(root|app)-password|test-provider-key|different-password' \
    <<<"${output}"; then
    echo "preflight test: rejection leaked a fixture secret" >&2
    exit 1
  fi
}

assert_rejected "${example}" "example env cannot be promoted"
if grep -F -- '--ignore-existing' "${restore_drill}" >/dev/null; then
  echo "preflight test: restore drill must fail on bucket-name collision" >&2
  exit 1
fi

valid="${temp_dir}/valid.env"
sed \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb|' \
  -e 's|replace-with-release-id|git-deadbeef|' \
  -e 's|change-me-migrator-postgres|test-migrator-password|g' \
  -e 's|change-me-api-postgres|test-api-password|g' \
  -e 's|change-me-rag-worker-postgres|test-rag-worker-password|g' \
  -e 's|change-me-rag-replay-postgres|test-rag-replay-password|g' \
  -e 's|change-me-redis|test-redis-password|g' \
  -e 's|change-me-minio-root-secret|test-minio-root-password|g' \
  -e 's|change-me-minio-user-secret|test-minio-app-password|g' \
  -e 's|change-me-base64-32-byte-random-key|MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=|g' \
  -e 's|https://change-me.example/invites/accept|https://chat.internal/invites/accept|' \
  -e 's|https://your-openai-compatible-relay.example/v1|https://relay.internal/v1|' \
  -e 's|change-me-provider-key|test-provider-key-1234567890|' \
  "${example}" >"${valid}"
chmod 600 "${valid}"
"${preflight}" "${valid}" >/dev/null

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

placeholder="${temp_dir}/placeholder.env"
sed 's|^PROVIDER_API_KEY=.*|PROVIDER_API_KEY=change-me-provider-key|' \
  "${valid}" >"${placeholder}"
chmod 600 "${placeholder}"
assert_rejected "${placeholder}" "PROVIDER_API_KEY still contains a placeholder"

migration_user_mismatch="${temp_dir}/migration-user-mismatch.env"
sed 's|^POSTGRES_USER=.*|POSTGRES_USER=different_migrator|' \
  "${valid}" >"${migration_user_mismatch}"
chmod 600 "${migration_user_mismatch}"
assert_rejected \
  "${migration_user_mismatch}" \
  "MIGRATION_DATABASE_URL user does not match POSTGRES_USER"

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
  grep -v '^PROVIDER_API_KEY=' "${valid}" >"${interpolation}"
  printf 'PROVIDER_API_KEY=%s\n' "${interpolation_value}" >>"${interpolation}"
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

for invalid_provider in \
  'https://user:password@relay.internal/v1' \
  'https://relay.internal/v1#fragment' \
  'https://bad host/v1' \
  'https://relay.internal:invalid/v1'; do
  provider_env="${temp_dir}/provider-$RANDOM.env"
  sed "s|^PROVIDER_BASE_URL=.*|PROVIDER_BASE_URL=${invalid_provider}|" \
    "${valid}" >"${provider_env}"
  chmod 600 "${provider_env}"
  assert_rejected "${provider_env}" "PROVIDER_BASE_URL"
done

for unsupported_assignment in \
  'PROVIDER_API_KEY="quoted-value"' \
  'PROVIDER_API_KEY=value\\twith-escape' \
  'PROVIDER_API_KEY=value#inline-comment' \
  'export PROVIDER_API_KEY=value'; do
  syntax_env="${temp_dir}/syntax-$RANDOM.env"
  grep -v '^PROVIDER_API_KEY=' "${valid}" >"${syntax_env}"
  printf '%s\n' "${unsupported_assignment}" >>"${syntax_env}"
  chmod 600 "${syntax_env}"
  assert_rejected "${syntax_env}" "unsupported"
done

duplicate="${temp_dir}/duplicate.env"
cp "${valid}" "${duplicate}"
printf '\nPROVIDER_API_KEY=second-value\n' >>"${duplicate}"
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
      --profile app --profile ops --profile rag-worker --profile rag-ops \
      config --format json
} 2>"${temp_dir}/production-compose.stderr")"
python3 - "${rendered}" <<'PY'
import json
import sys

config = json.loads(sys.argv[1])
services = config["services"]
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
for name in ("backend", "migrate", "admin"):
    service = services[name]
    assert service["image"] == want_image, (name, service["image"])
    assert "build" not in service, name
assert list(services["backend"]["networks"]) == ["private", "rag-private"]
assert services["postgres"]["environment"] == {
    "POSTGRES_DB": "neo_chat",
    "POSTGRES_PASSWORD": "test-migrator-password",
    "POSTGRES_USER": "neo_chat_migrator",
}
backend_environment = services["backend"]["environment"]
assert "neo_chat_api:test-api-password@postgres" in backend_environment["DATABASE_URL"]
assert "MIGRATION_DATABASE_URL" not in backend_environment

migrate_environment = services["migrate"]["environment"]
assert "neo_chat_migrator:test-migrator-password@postgres" in migrate_environment["MIGRATION_DATABASE_URL"]
assert "DATABASE_URL" not in migrate_environment

admin_environment = services["admin"]["environment"]
assert "neo_chat_api:test-api-password@postgres" in admin_environment["DATABASE_URL"]
assert "MIGRATION_DATABASE_URL" not in admin_environment

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
  --profile app --profile ops --profile rag-worker --profile rag-ops \
  config --format json)"
python3 - "${development_rendered}" <<'PY'
import json
import sys

config = json.loads(sys.argv[1])
services = config["services"]
for name in ("frontend", "backend", "migrate", "admin", "rag-worker", "rag-replay"):
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
