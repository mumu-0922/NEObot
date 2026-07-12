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
    'test-(postgres|redis)-password|test-minio-(root|app)-password|test-provider-key' \
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
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|' \
  -e 's|replace-with-release-id|git-deadbeef|' \
  -e 's|change-me-postgres|test-postgres-password|g' \
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

insecure="${temp_dir}/insecure.env"
cp "${valid}" "${insecure}"
chmod 644 "${insecure}"
assert_rejected "${insecure}" "must not be group/world accessible"

placeholder="${temp_dir}/placeholder.env"
sed 's|^PROVIDER_API_KEY=.*|PROVIDER_API_KEY=change-me-provider-key|' \
  "${valid}" >"${placeholder}"
chmod 600 "${placeholder}"
assert_rejected "${placeholder}" "PROVIDER_API_KEY still contains a placeholder"

mismatch="${temp_dir}/mismatch.env"
sed 's|test-postgres-password@postgres|different-password@postgres|' \
  "${valid}" >"${mismatch}"
chmod 600 "${mismatch}"
assert_rejected "${mismatch}" "DATABASE_URL password does not match POSTGRES_PASSWORD"

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
  BACKEND_IMAGE=mm-chat/backend:host-override \
  POSTGRES_PASSWORD=host-override-password \
    "${production_compose}" "${valid}" \
      --profile app --profile ops config --format json
} 2>"${temp_dir}/production-compose.stderr")"
python3 - "${rendered}" <<'PY'
import json
import sys

config = json.loads(sys.argv[1])
services = config["services"]
want_image = (
    "ghcr.io/mumu-0922/neobot-mm-chat@sha256:"
    + "a" * 64
)
for name in ("backend", "migrate", "admin"):
    service = services[name]
    assert service["image"] == want_image, (name, service["image"])
    assert "build" not in service, name
assert services["postgres"]["environment"]["POSTGRES_PASSWORD"] == "test-postgres-password"
assert "test-postgres-password@postgres" in services["backend"]["environment"]["DATABASE_URL"]
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
