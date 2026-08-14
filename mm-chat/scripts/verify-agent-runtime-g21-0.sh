#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
example_env="${project_dir}/.env.single-server.example"
preflight="${script_dir}/preflight-single-server.sh"
release_commit=1111111111111111111111111111111111111111

for command in bash python3 jq sha256sum openssl docker rg sed; do
  command -v "${command}" >/dev/null 2>&1 || { echo "G21.0 verification: ${command} is required" >&2; exit 1; }
done

work_dir="$(mktemp -d)"
cleanup() {
  find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

bash "${script_dir}/verify-agent-production-activation.sh"
bash "${script_dir}/verify-agent-runner-bundle.sh"

(
  # Existing Runner tests intentionally create mode-0711 broker directories;
  # keep the verification harness's private umask from changing that contract.
  umask 022
  cd "${backend_dir}"
  go test -race \
    ./cmd/agent-runtime-control \
    ./cmd/neo-runnerd \
    ./cmd/neo-runner-probe \
    ./internal/agentactivation \
    ./internal/agentruntimecontrol \
    ./internal/agentrunner
  go vet \
    ./cmd/agent-runtime-control \
    ./cmd/neo-runnerd \
    ./cmd/neo-runner-probe \
    ./internal/agentactivation \
    ./internal/agentruntimecontrol \
    ./internal/agentrunner
)

if rg -n 'agentactivation|agentruntimecontrol|NewRPCClient' "${backend_dir}/cmd/api" >/dev/null; then
  echo 'G21.0 verification: control-plane packages leaked into cmd/api' >&2
  exit 1
fi
if find "${backend_dir}/migrations" -maxdepth 1 -name '091_*' -print -quit | grep -q .; then
  echo 'G21.0 verification: migration 091 is forbidden in this slice' >&2
  exit 1
fi
for role_guard in pg_auth_members rolcanlogin rolsuper rolcreatedb rolcreaterole rolreplication rolbypassrls; do
  if ! rg -q "${role_guard}" "${backend_dir}/cmd/agent-runtime-control/main.go"; then
    echo "G21.0 verification: database role guard ${role_guard} is missing" >&2
    exit 1
  fi
done
for denied in launch heartbeat cancel prepare commit; do
  if rg -n "Method${denied^}|\"${denied}\"" "${backend_dir}/internal/agentruntimecontrol" >/dev/null; then
    echo "G21.0 verification: control worker contains denied ${denied} RPC" >&2
    exit 1
  fi
done

compose_json="${work_dir}/compose.json"
docker compose \
  --project-directory "${project_dir}" \
  --env-file "${example_env}" \
  -f "${project_dir}/compose.single-server.yml" \
  --profile agent-runtime-control \
  config --format json >"${compose_json}"
python3 - "${compose_json}" <<'PY'
import json
import sys

config = json.load(open(sys.argv[1], encoding="utf-8"))
service = config["services"]["agent-runtime-control"]
assert service["profiles"] == ["agent-runtime-control"]
assert "ports" not in service
assert service["read_only"] is True
assert service["cap_drop"] == ["ALL"]
assert "no-new-privileges:true" in service["security_opt"]
assert list(service["networks"]) == ["private"]
assert service.get("secrets", []) == []
assert service["environment"]["AGENT_RUNNER_CONTROL_ENABLED"] == "false"
for name in (
    "AGENT_RUNTIME_ENABLED",
    "AGENT_SCHEDULER_ENABLED",
    "AGENT_SKILL_INSTALL_ENABLED",
    "AGENT_LEARNING_ENABLED",
    "AGENT_DELEGATION_ENABLED",
    "AGENT_BROKER_READ_ONLY_ENABLED",
    "AGENT_BROKER_MUTATION_ENABLED",
):
    assert service["environment"][name] == "false", name
for forbidden in (
    "DATABASE_URL",
    "MIGRATION_DATABASE_URL",
    "PROVIDER_SECRET_KEYRING_FILE",
    "REDIS_URL",
    "S3_ACCESS_KEY_ID",
    "S3_SECRET_ACCESS_KEY",
    "MCP_RUNNER_TOKEN_FILE",
):
    assert forbidden not in service["environment"], forbidden
assert len(service["volumes"]) == 6
assert all(volume["read_only"] is True for volume in service["volumes"])
assert all(volume["bind"]["create_host_path"] is False for volume in service["volumes"])
PY

cat >"${work_dir}/client-cert.cnf" <<'EOF'
[req]
distinguished_name=dn
prompt=no
[dn]
CN=spiffe://neo-chat/agent-runtime-control
EOF
cat >"${work_dir}/server-ca.cnf" <<'EOF'
[req]
distinguished_name=dn
prompt=no
[dn]
CN=neo-runner-test-ca
EOF
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -sha256 \
  -config "${work_dir}/client-cert.cnf" \
  -keyout "${work_dir}/client.key" -out "${work_dir}/client.crt" >/dev/null 2>&1
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -sha256 \
  -config "${work_dir}/server-ca.cnf" \
  -keyout "${work_dir}/server-ca.key" -out "${work_dir}/server-ca.crt" >/dev/null 2>&1
chmod 600 "${work_dir}/client.key" "${work_dir}/client.crt" \
  "${work_dir}/server-ca.key" "${work_dir}/server-ca.crt"

zero='sha256:0000000000000000000000000000000000000000000000000000000000000000'
nonzero='sha256:1111111111111111111111111111111111111111111111111111111111111111'
sed "s|${zero}|${nonzero}|g" \
  "${project_dir}/config/agent-runner/release-manifest.example.json" |
  jq '.approved=true | .runnerVersion="g21.0-preflight-test"' >"${work_dir}/release-manifest.json"
chmod 600 "${work_dir}/release-manifest.json"

policy="${project_dir}/config/agent-runner/production-policy.json"
policy_sha="sha256:$(sha256sum "${policy}" | awk '{print $1}')"
manifest_sha="sha256:$(sha256sum "${work_dir}/release-manifest.json" | awk '{print $1}')"
client_sha="sha256:$(sha256sum "${work_dir}/client.crt" | awk '{print $1}')"
ca_sha="sha256:$(sha256sum "${work_dir}/server-ca.crt" | awk '{print $1}')"
runner_sha="sha256:$(printf synthetic-g21-runner | sha256sum | awk '{print $1}')"
evidence_sha="sha256:$(printf g21-live-check | sha256sum | awk '{print $1}')"
reviewer_sha="sha256:$(printf g21-reviewer | sha256sum | awk '{print $1}')"
deployment_sha="sha256:$(printf g21-deployment | sha256sum | awk '{print $1}')"
runner_url='https://10.0.0.8:9443/internal/neo-runner/v1/rpc'
endpoint_sha="$(python3 - "${runner_url}" <<'PY'
import hashlib
import sys
print("sha256:" + hashlib.sha256(b"neo-agent-runner-endpoint-v1\0" + sys.argv[1].encode()).hexdigest())
PY
)"
now="$(date -u +%s)"
format_epoch() { date -u -d "@$1" '+%Y-%m-%dT%H:%M:%SZ'; }
started="$(format_epoch "$((now - 300))")"
completed="$(format_epoch "$((now - 120))")"
observed="$(format_epoch "$((now - 180))")"
reviewed="$(format_epoch "$((now - 60))")"
expires="$(format_epoch "$((now + 3600))")"
jq \
  --arg commit "${release_commit}" --arg policy "${policy_sha}" \
  --arg manifest "${manifest_sha}" --arg runner "${runner_sha}" \
  --arg client "${client_sha}" --arg ca "${ca_sha}" --arg endpoint "${endpoint_sha}" \
  --arg evidence "${evidence_sha}" --arg reviewer "${reviewer_sha}" --arg deployment "${deployment_sha}" \
  --arg started "${started}" --arg completed "${completed}" --arg observed "${observed}" \
  --arg reviewed "${reviewed}" --arg expires "${expires}" '
  .evidenceClass="production" |
  .release.gitCommit=$commit |
  .release.operationsPolicySha256=$policy |
  .release.runnerManifestSha256=$manifest |
  .release.runnerBinarySha256=$runner |
  .target.deploymentFingerprint=$deployment |
  .wiring.endpointSha256=$endpoint |
  .wiring.clientCertificateSha256=$client |
  .wiring.serverCASha256=$ca |
  .window={startedAt:$started,completedAt:$completed,expiresAt:$expires} |
  .checks |= map(.result="passed" | .observedAt=$observed | .evidenceSha256=$evidence | .detailCode="PASS") |
  .review={decision:"approved",reviewedAt:$reviewed,reviewerFingerprint:$reviewer}
' "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-production-activation.valid.json" \
  >"${work_dir}/production-activation.json"
chmod 600 "${work_dir}/production-activation.json"

provider_keyring="${work_dir}/provider-keyring.json"
printf '%s\n' '{"v":1,"activeKid":"test-v1","keys":[{"kid":"test-v1","key":"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY"}]}' >"${provider_keyring}"
chmod 600 "${provider_keyring}"
enabled_env="${work_dir}/enabled.env"
sed \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-postgres@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-postgres@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd|' \
  -e 's|replace-with-release-id|git-deadbeef|' \
  -e "s|replace-with-host-uid|$(id -u)|" -e "s|replace-with-host-gid|$(id -g)|" \
  -e 's|change-me-migrator-postgres|test-migrator-password|g' \
  -e 's|change-me-api-postgres|test-api-password|g' \
  -e 's|change-me-memory-worker-postgres|test-memory-worker-password|g' \
  -e 's|change-me-rag-worker-postgres|test-rag-worker-password|g' \
  -e 's|change-me-rag-replay-postgres|test-rag-replay-password|g' \
  -e 's|change-me-agent-control-postgres|test-agent-control-password|g' \
  -e 's|change-me-redis|test-redis-password|g' \
  -e 's|change-me-minio-root-secret|test-minio-root-password|g' \
  -e 's|change-me-minio-user-secret|test-minio-app-password|g' \
  -e 's|change-me-base64-32-byte-random-key|MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=|g' \
  -e 's|https://change-me.example/invites/accept|https://chat.internal/invites/accept|' \
  -e 's|^AGENT_RUNNER_CONTROL_ENABLED=false$|AGENT_RUNNER_CONTROL_ENABLED=true|' \
  -e "s|^AGENT_RUNNER_URL=.*|AGENT_RUNNER_URL=${runner_url}|" \
  -e "s|^AGENT_RUNNER_CLIENT_CERT_SOURCE=.*|AGENT_RUNNER_CLIENT_CERT_SOURCE=${work_dir}/client.crt|" \
  -e "s|^AGENT_RUNNER_CLIENT_KEY_SOURCE=.*|AGENT_RUNNER_CLIENT_KEY_SOURCE=${work_dir}/client.key|" \
  -e "s|^AGENT_RUNNER_SERVER_CA_SOURCE=.*|AGENT_RUNNER_SERVER_CA_SOURCE=${work_dir}/server-ca.crt|" \
  -e "s|^AGENT_RUNNER_RELEASE_MANIFEST_SOURCE=.*|AGENT_RUNNER_RELEASE_MANIFEST_SOURCE=${work_dir}/release-manifest.json|" \
  -e "s|^AGENT_PRODUCTION_POLICY_SOURCE=.*|AGENT_PRODUCTION_POLICY_SOURCE=${policy}|" \
  -e "s|^AGENT_PRODUCTION_ACTIVATION_SOURCE=.*|AGENT_PRODUCTION_ACTIVATION_SOURCE=${work_dir}/production-activation.json|" \
  -e "s|^AGENT_RELEASE_GIT_COMMIT=.*|AGENT_RELEASE_GIT_COMMIT=${release_commit}|" \
  -e "s|^PROVIDER_SECRET_KEYRING_SOURCE=.*|PROVIDER_SECRET_KEYRING_SOURCE=${provider_keyring}|" \
  "${example_env}" >"${enabled_env}"
chmod 600 "${enabled_env}"
bash "${preflight}" "${enabled_env}" >/dev/null

assert_preflight_rejected() {
  local env_file="$1" expected="$2" output
  if output="$(bash "${preflight}" "${env_file}" 2>&1)"; then
    echo "G21.0 verification: preflight accepted ${env_file}" >&2
    exit 1
  fi
  [[ "${output}" == *"${expected}"* ]] || {
    echo "G21.0 verification: unexpected preflight rejection for ${env_file}" >&2
    exit 1
  }
}

sed 's|^AGENT_RUNTIME_ENABLED=false$|AGENT_RUNTIME_ENABLED=true|' "${enabled_env}" >"${work_dir}/execution.env"
chmod 600 "${work_dir}/execution.env"
assert_preflight_rejected "${work_dir}/execution.env" 'AGENT_RUNTIME_ENABLED must remain false in G21.0'

sed 's|^AGENT_RUNNER_URL=.*|AGENT_RUNNER_URL=https://8.8.8.8:9443/internal/neo-runner/v1/rpc|' \
  "${enabled_env}" >"${work_dir}/public-url.env"
chmod 600 "${work_dir}/public-url.env"
assert_preflight_rejected "${work_dir}/public-url.env" 'exact private HTTPS Runner RPC URL'

sed 's|postgres://agent_runner_control_app:test-agent-control-password@|postgres://neo_chat_api:test-agent-control-password@|' \
  "${enabled_env}" >"${work_dir}/shared-principal.env"
chmod 600 "${work_dir}/shared-principal.env"
assert_preflight_rejected "${work_dir}/shared-principal.env" 'must use distinct database principals'

jq '.evidenceClass="template"' "${work_dir}/production-activation.json" >"${work_dir}/held-activation.json"
chmod 600 "${work_dir}/held-activation.json"
sed "s|^AGENT_PRODUCTION_ACTIVATION_SOURCE=.*|AGENT_PRODUCTION_ACTIVATION_SOURCE=${work_dir}/held-activation.json|" \
  "${enabled_env}" >"${work_dir}/held.env"
chmod 600 "${work_dir}/held.env"
assert_preflight_rejected "${work_dir}/held.env" 'not READY for G21.0'

cp "${work_dir}/client.crt" "${work_dir}/insecure-client.crt"
chmod 644 "${work_dir}/insecure-client.crt"
sed "s|^AGENT_RUNNER_CLIENT_CERT_SOURCE=.*|AGENT_RUNNER_CLIENT_CERT_SOURCE=${work_dir}/insecure-client.crt|" \
  "${enabled_env}" >"${work_dir}/insecure.env"
chmod 600 "${work_dir}/insecure.env"
assert_preflight_rejected "${work_dir}/insecure.env" 'must use mode 600'

sed "s|^AGENT_RUNNER_CLIENT_KEY_SOURCE=.*|AGENT_RUNNER_CLIENT_KEY_SOURCE=${work_dir}/server-ca.key|" \
  "${enabled_env}" >"${work_dir}/mismatched-key.env"
chmod 600 "${work_dir}/mismatched-key.env"
assert_preflight_rejected "${work_dir}/mismatched-key.env" 'certificate/key material is invalid or mismatched'

ln -s "${work_dir}/production-activation.json" "${work_dir}/activation-link.json"
sed "s|^AGENT_PRODUCTION_ACTIVATION_SOURCE=.*|AGENT_PRODUCTION_ACTIVATION_SOURCE=${work_dir}/activation-link.json|" \
  "${enabled_env}" >"${work_dir}/symlink.env"
chmod 600 "${work_dir}/symlink.env"
assert_preflight_rejected "${work_dir}/symlink.env" 'must be a regular non-symlink file'

set +e
host_output="$(bash "${script_dir}/verify-agent-runner-host.sh" 2>&1)"
host_status=$?
set -e
[[ "${host_status}" -ne 0 && "${host_output}" == *ISOLATION_UNAVAILABLE* ]] || {
  echo 'G21.0 verification: current host unexpectedly became production-eligible' >&2
  exit 1
}

printf '%s\n' 'Agent Runtime G21.0 verification: passed; current host remains ISOLATION_UNAVAILABLE.'
