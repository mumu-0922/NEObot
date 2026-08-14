#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
example_env="${project_dir}/.env.single-server.example"
preflight="${script_dir}/preflight-single-server.sh"
work_dir="$(mktemp -d)"
cleanup(){ find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true; rmdir "${work_dir}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

for command in bash date jq openssl python3 sed sha256sum; do
  command -v "${command}" >/dev/null 2>&1 || { echo "G21.2 preflight: ${command} is required" >&2; exit 1; }
done

make_certificate(){
  local name="$1" common_name="$2"
  cat >"${work_dir}/${name}.cnf" <<EOF_CERT
[req]
distinguished_name=dn
prompt=no
[dn]
CN=${common_name}
EOF_CERT
  openssl req -x509 -newkey rsa:2048 -nodes -days 1 -sha256 \
    -config "${work_dir}/${name}.cnf" \
    -keyout "${work_dir}/${name}.key" -out "${work_dir}/${name}.crt" >/dev/null 2>&1
  chmod 600 "${work_dir}/${name}.key" "${work_dir}/${name}.crt"
}
make_certificate control-client spiffe://neo-chat/agent-runtime-control
make_certificate control-server-ca neo-runner-control-test-ca
make_certificate root-client spiffe://neo-chat/agent-runtime-root-canary
make_certificate root-server-ca neo-runner-root-test-ca
make_certificate broker-client spiffe://neo-chat/agent-runtime-broker-canary
make_certificate broker-server-ca neo-runner-broker-test-ca
make_certificate relay-server agent-broker-canary.internal
make_certificate relay-client-ca neo-runner-relay-client-test-ca

cp "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-root-run-canary-plan.valid.json" \
  "${work_dir}/root-plan.json"
cp "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-broker-artifact-canary-plan.valid.json" \
  "${work_dir}/broker-plan.json"
mkdir "${work_dir}/quarantine" "${work_dir}/project-read-root" "${work_dir}/workspace-read-root"
printf '%s' '0123456789abcdef0123456789abcdef0123456789abcdef' >"${work_dir}/mcp-runner-token"
printf '%s\n' '{"v":1,"activeKid":"test-v1","keys":[{"kid":"test-v1","key":"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY"}]}' \
  >"${work_dir}/provider-keyring.json"
chmod 600 "${work_dir}"/*.json "${work_dir}/mcp-runner-token"

zero='sha256:0000000000000000000000000000000000000000000000000000000000000000'
nonzero='sha256:1111111111111111111111111111111111111111111111111111111111111111'
sed "s|${zero}|${nonzero}|g" "${project_dir}/config/agent-runner/release-manifest.example.json" |
  jq '.approved=true | .runnerVersion="g21.2-preflight-test"' >"${work_dir}/release-manifest.json"
chmod 600 "${work_dir}/release-manifest.json"

python3 - "${project_dir}" "${work_dir}" <<'PY'
import base64, hashlib, json, os, sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

project, work = map(Path, sys.argv[1:])
now = datetime.now(timezone.utc).replace(microsecond=0)
fmt = lambda value: value.isoformat().replace("+00:00", "Z")
fp = lambda path: "sha256:" + hashlib.sha256(Path(path).read_bytes()).hexdigest()
label_fp = lambda label: "sha256:" + hashlib.sha256(label.encode()).hexdigest()
runner_endpoint = "https://10.0.0.8:9443/internal/neo-runner/v1/rpc"
relay_endpoint = "https://172.31.254.2:9444/internal/agent-broker/v1/relay"
endpoint_fp = "sha256:" + hashlib.sha256(b"neo-agent-runner-endpoint-v1\0" + runner_endpoint.encode()).hexdigest()
relay_fp = "sha256:" + hashlib.sha256(b"neo-agent-broker-relay-endpoint-v1\0" + relay_endpoint.encode()).hexdigest()
commit = "1" * 40
policy = project / "config/agent-runner/production-policy.json"
manifest = work / "release-manifest.json"
fixture_dir = project / "docs/contracts/fixtures/agent-runtime"

for prefix in ("root", "broker"):
    public = os.urandom(32)
    private = os.urandom(32) + public
    (work / f"{prefix}-authority-public-key").write_text(base64.urlsafe_b64encode(public).decode().rstrip("="))
    (work / f"{prefix}-authority-private-key").write_text(base64.urlsafe_b64encode(private).decode().rstrip("="))
    os.chmod(work / f"{prefix}-authority-public-key", 0o600)
    os.chmod(work / f"{prefix}-authority-private-key", 0o600)

common_release = {
    "gitCommit": commit,
    "migrationHead": 92,
    "runnerManifestSha256": fp(manifest),
    "runnerBinarySha256": label_fp("g21.2-preflight-runner"),
    "operationsPolicySha256": fp(policy),
}

def ready(name, client, ca, extra_wiring):
    record = json.loads((fixture_dir / f"neo-agent-{name}.valid.json").read_text())
    record["evidenceClass"] = "production"
    record["release"] = dict(common_release)
    record["target"]["deploymentFingerprint"] = label_fp(name + "-deployment")
    record["wiring"]["endpointSha256"] = endpoint_fp
    record["wiring"]["clientCertificateSha256"] = fp(work / client)
    record["wiring"]["serverCASha256"] = fp(work / ca)
    record["wiring"].update(extra_wiring)
    record["window"] = {
        "startedAt": fmt(now - timedelta(minutes=5)),
        "completedAt": fmt(now - timedelta(minutes=2)),
        "expiresAt": fmt(now + timedelta(hours=1)),
    }
    evidence = label_fp(name + "-evidence")
    for check in record["checks"]:
        check.update(result="passed", observedAt=fmt(now - timedelta(minutes=3)), evidenceSha256=evidence, detailCode="PASS")
    record["review"] = {
        "decision": "approved",
        "reviewedAt": fmt(now - timedelta(minutes=1)),
        "reviewerFingerprint": label_fp(name + "-reviewer"),
    }
    output = work / f"{name}-ready.json"
    output.write_text(json.dumps(record, separators=(",", ":")) + "\n")
    os.chmod(output, 0o600)

ready("production-activation", "control-client.crt", "control-server-ca.crt", {})
ready("root-run-canary-activation", "root-client.crt", "root-server-ca.crt", {
    "canaryPlanSha256": fp(work / "root-plan.json"),
    "authorityPublicKeySha256": fp(work / "root-authority-public-key"),
})
ready("broker-artifact-canary-activation", "broker-client.crt", "broker-server-ca.crt", {
    "canaryPlanSha256": fp(work / "broker-plan.json"),
    "authorityPublicKeySha256": fp(work / "broker-authority-public-key"),
    "relayEndpointSha256": relay_fp,
    "relayServerCertificateSha256": fp(work / "relay-server.crt"),
    "relayClientCASha256": fp(work / "relay-client-ca.crt"),
})
PY

valid_env="${work_dir}/enabled.env"
sed \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-postgres@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-postgres@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-mcp-runner@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-mcp-runner@sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee|' \
  -e 's|replace-with-release-id|git-deadbeef|' \
  -e "s|replace-with-host-uid|$(id -u)|" -e "s|replace-with-host-gid|$(id -g)|" \
  -e 's|change-me-migrator-postgres|test-migrator-password|g' \
  -e 's|change-me-api-postgres|test-api-password|g' \
  -e 's|change-me-memory-worker-postgres|test-memory-worker-password|g' \
  -e 's|change-me-rag-worker-postgres|test-rag-worker-password|g' \
  -e 's|change-me-rag-replay-postgres|test-rag-replay-password|g' \
  -e 's|change-me-agent-control-postgres|test-agent-control-password|g' \
  -e 's|change-me-agent-root-canary-postgres|test-agent-root-canary-password|g' \
  -e 's|change-me-agent-broker-canary-postgres|test-agent-broker-canary-password|g' \
  -e 's|change-me-redis|test-redis-password|g' \
  -e 's|change-me-minio-root-secret|test-minio-root-password|g' \
  -e 's|change-me-minio-user-secret|test-minio-app-password|g' \
  -e 's|change-me-base64-32-byte-random-key|MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=|g' \
  -e 's|https://change-me.example/invites/accept|https://chat.internal/invites/accept|' \
  -e 's|^AGENT_RUNNER_CONTROL_ENABLED=false$|AGENT_RUNNER_CONTROL_ENABLED=true|' \
  -e 's|^AGENT_ROOT_RUN_CANARY_ENABLED=false$|AGENT_ROOT_RUN_CANARY_ENABLED=true|' \
  -e 's|^AGENT_BROKER_ARTIFACT_CANARY_ENABLED=false$|AGENT_BROKER_ARTIFACT_CANARY_ENABLED=true|' \
  -e 's|^AGENT_RUNNER_URL=.*|AGENT_RUNNER_URL=https://10.0.0.8:9443/internal/neo-runner/v1/rpc|' \
  -e 's|^AGENT_ROOT_CANARY_RUNNER_URL=.*|AGENT_ROOT_CANARY_RUNNER_URL=https://10.0.0.8:9443/internal/neo-runner/v1/rpc|' \
  -e 's|^AGENT_BROKER_CANARY_RUNNER_URL=.*|AGENT_BROKER_CANARY_RUNNER_URL=https://10.0.0.8:9443/internal/neo-runner/v1/rpc|' \
  -e "s|^AGENT_RUNNER_CLIENT_CERT_SOURCE=.*|AGENT_RUNNER_CLIENT_CERT_SOURCE=${work_dir}/control-client.crt|" \
  -e "s|^AGENT_RUNNER_CLIENT_KEY_SOURCE=.*|AGENT_RUNNER_CLIENT_KEY_SOURCE=${work_dir}/control-client.key|" \
  -e "s|^AGENT_RUNNER_SERVER_CA_SOURCE=.*|AGENT_RUNNER_SERVER_CA_SOURCE=${work_dir}/control-server-ca.crt|" \
  -e "s|^AGENT_RUNNER_RELEASE_MANIFEST_SOURCE=.*|AGENT_RUNNER_RELEASE_MANIFEST_SOURCE=${work_dir}/release-manifest.json|" \
  -e "s|^AGENT_PRODUCTION_POLICY_SOURCE=.*|AGENT_PRODUCTION_POLICY_SOURCE=${project_dir}/config/agent-runner/production-policy.json|" \
  -e "s|^AGENT_PRODUCTION_ACTIVATION_SOURCE=.*|AGENT_PRODUCTION_ACTIVATION_SOURCE=${work_dir}/production-activation-ready.json|" \
  -e 's|^AGENT_RELEASE_GIT_COMMIT=.*|AGENT_RELEASE_GIT_COMMIT=1111111111111111111111111111111111111111|' \
  -e "s|^AGENT_ROOT_CANARY_CLIENT_CERT_SOURCE=.*|AGENT_ROOT_CANARY_CLIENT_CERT_SOURCE=${work_dir}/root-client.crt|" \
  -e "s|^AGENT_ROOT_CANARY_CLIENT_KEY_SOURCE=.*|AGENT_ROOT_CANARY_CLIENT_KEY_SOURCE=${work_dir}/root-client.key|" \
  -e "s|^AGENT_ROOT_CANARY_SERVER_CA_SOURCE=.*|AGENT_ROOT_CANARY_SERVER_CA_SOURCE=${work_dir}/root-server-ca.crt|" \
  -e "s|^AGENT_ROOT_CANARY_RELEASE_MANIFEST_SOURCE=.*|AGENT_ROOT_CANARY_RELEASE_MANIFEST_SOURCE=${work_dir}/release-manifest.json|" \
  -e "s|^AGENT_ROOT_CANARY_PRODUCTION_POLICY_SOURCE=.*|AGENT_ROOT_CANARY_PRODUCTION_POLICY_SOURCE=${project_dir}/config/agent-runner/production-policy.json|" \
  -e "s|^AGENT_ROOT_CANARY_ACTIVATION_SOURCE=.*|AGENT_ROOT_CANARY_ACTIVATION_SOURCE=${work_dir}/root-run-canary-activation-ready.json|" \
  -e "s|^AGENT_ROOT_CANARY_PLAN_SOURCE=.*|AGENT_ROOT_CANARY_PLAN_SOURCE=${work_dir}/root-plan.json|" \
  -e "s|^AGENT_ROOT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE=.*|AGENT_ROOT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE=${work_dir}/root-authority-private-key|" \
  -e "s|^AGENT_ROOT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE=.*|AGENT_ROOT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE=${work_dir}/root-authority-public-key|" \
  -e 's|^AGENT_ROOT_CANARY_RELEASE_GIT_COMMIT=.*|AGENT_ROOT_CANARY_RELEASE_GIT_COMMIT=1111111111111111111111111111111111111111|' \
  -e "s|^AGENT_BROKER_CANARY_CLIENT_CERT_SOURCE=.*|AGENT_BROKER_CANARY_CLIENT_CERT_SOURCE=${work_dir}/broker-client.crt|" \
  -e "s|^AGENT_BROKER_CANARY_CLIENT_KEY_SOURCE=.*|AGENT_BROKER_CANARY_CLIENT_KEY_SOURCE=${work_dir}/broker-client.key|" \
  -e "s|^AGENT_BROKER_CANARY_SERVER_CA_SOURCE=.*|AGENT_BROKER_CANARY_SERVER_CA_SOURCE=${work_dir}/broker-server-ca.crt|" \
  -e "s|^AGENT_BROKER_CANARY_RELEASE_MANIFEST_SOURCE=.*|AGENT_BROKER_CANARY_RELEASE_MANIFEST_SOURCE=${work_dir}/release-manifest.json|" \
  -e "s|^AGENT_BROKER_CANARY_PRODUCTION_POLICY_SOURCE=.*|AGENT_BROKER_CANARY_PRODUCTION_POLICY_SOURCE=${project_dir}/config/agent-runner/production-policy.json|" \
  -e "s|^AGENT_BROKER_CANARY_ACTIVATION_SOURCE=.*|AGENT_BROKER_CANARY_ACTIVATION_SOURCE=${work_dir}/broker-artifact-canary-activation-ready.json|" \
  -e "s|^AGENT_BROKER_CANARY_PLAN_SOURCE=.*|AGENT_BROKER_CANARY_PLAN_SOURCE=${work_dir}/broker-plan.json|" \
  -e "s|^AGENT_BROKER_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE=.*|AGENT_BROKER_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE=${work_dir}/broker-authority-private-key|" \
  -e "s|^AGENT_BROKER_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE=.*|AGENT_BROKER_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE=${work_dir}/broker-authority-public-key|" \
  -e 's|^AGENT_BROKER_CANARY_RELEASE_GIT_COMMIT=.*|AGENT_BROKER_CANARY_RELEASE_GIT_COMMIT=1111111111111111111111111111111111111111|' \
  -e "s|^AGENT_BROKER_CANARY_RELAY_TLS_CERT_SOURCE=.*|AGENT_BROKER_CANARY_RELAY_TLS_CERT_SOURCE=${work_dir}/relay-server.crt|" \
  -e "s|^AGENT_BROKER_CANARY_RELAY_TLS_KEY_SOURCE=.*|AGENT_BROKER_CANARY_RELAY_TLS_KEY_SOURCE=${work_dir}/relay-server.key|" \
  -e "s|^AGENT_BROKER_CANARY_RELAY_TLS_CLIENT_CA_SOURCE=.*|AGENT_BROKER_CANARY_RELAY_TLS_CLIENT_CA_SOURCE=${work_dir}/relay-client-ca.crt|" \
  -e "s|^AGENT_BROKER_CANARY_QUARANTINE_SOURCE=.*|AGENT_BROKER_CANARY_QUARANTINE_SOURCE=${work_dir}/quarantine|" \
  -e "s|^AGENT_BROKER_CANARY_PROJECT_ROOT_SOURCE=.*|AGENT_BROKER_CANARY_PROJECT_ROOT_SOURCE=${work_dir}/project-read-root|" \
  -e "s|^AGENT_BROKER_CANARY_WORKSPACE_ROOT_SOURCE=.*|AGENT_BROKER_CANARY_WORKSPACE_ROOT_SOURCE=${work_dir}/workspace-read-root|" \
  -e "s|^MCP_RUNNER_TOKEN_SOURCE=.*|MCP_RUNNER_TOKEN_SOURCE=${work_dir}/mcp-runner-token|" \
  -e "s|^PROVIDER_SECRET_KEYRING_SOURCE=.*|PROVIDER_SECRET_KEYRING_SOURCE=${work_dir}/provider-keyring.json|" \
  "${example_env}" >"${valid_env}"
chmod 600 "${valid_env}"

bash "${preflight}" "${valid_env}" >/dev/null

assert_rejected(){
  local name="$1" expected="$2" output env_file="${work_dir}/$1.env"
  if output="$(bash "${preflight}" "${env_file}" 2>&1)"; then
    echo "G21.2 preflight: accepted ${name}" >&2
    exit 1
  fi
  [[ "${output}" == *"${expected}"* ]] || { echo "G21.2 preflight: unexpected ${name} rejection" >&2; exit 1; }
}
sed 's|^AGENT_BROKER_CANARY_RUNNER_URL=.*|AGENT_BROKER_CANARY_RUNNER_URL=https://runner.internal:9443/internal/neo-runner/v1/rpc|' \
  "${valid_env}" >"${work_dir}/runner-hostname.env"
sed 's|^AGENT_BROKER_CANARY_RELAY_ENDPOINT=.*|AGENT_BROKER_CANARY_RELAY_ENDPOINT=https://relay.internal:9444/internal/agent-broker/v1/relay|' \
  "${valid_env}" >"${work_dir}/relay-hostname.env"
sed 's|^AGENT_BROKER_CANARY_RUNNER_RELAY_IDENTITY=.*|AGENT_BROKER_CANARY_RUNNER_RELAY_IDENTITY=spiffe://neo-chat/agent-runtime-root-canary|' \
  "${valid_env}" >"${work_dir}/reused-identity.env"
sed 's|^S3_BUCKET_AUTO_CREATE=false$|S3_BUCKET_AUTO_CREATE=true|' \
  "${valid_env}" >"${work_dir}/bucket-auto-create.env"
sed 's|^AGENT_BROKER_CANARY_RPC_TIMEOUT=.*|AGENT_BROKER_CANARY_RPC_TIMEOUT=11s|' \
  "${valid_env}" >"${work_dir}/rpc-timeout.env"
sed 's|^AGENT_BROKER_CANARY_AUTHORITY_TTL=.*|AGENT_BROKER_CANARY_AUTHORITY_TTL=9s|' \
  "${valid_env}" >"${work_dir}/authority-ttl.env"
sed 's|postgres://agent_broker_canary_app:test-agent-broker-canary-password@|postgres://agent_root_canary_app:test-agent-broker-canary-password@|' \
  "${valid_env}" >"${work_dir}/shared-principal.env"
jq '(.actions[] | select(.id=="project_read") | .classification)="mutable"' \
  "${work_dir}/broker-plan.json" >"${work_dir}/widened-broker-plan.json"
chmod 600 "${work_dir}/widened-broker-plan.json"
sed "s|^AGENT_BROKER_CANARY_PLAN_SOURCE=.*|AGENT_BROKER_CANARY_PLAN_SOURCE=${work_dir}/widened-broker-plan.json|" \
  "${valid_env}" >"${work_dir}/widened-plan.env"
chmod 600 "${work_dir}"/*.env

assert_rejected runner-hostname 'exact private HTTPS Runner RPC URL'
assert_rejected relay-hostname 'one exact private literal endpoint'
assert_rejected reused-identity 'identities must be exact and distinct'
assert_rejected bucket-auto-create 'existing S3-compatible bucket without auto-create'
assert_rejected rpc-timeout 'RPC_TIMEOUT must be between 1s and 10s'
assert_rejected authority-ttl 'AUTHORITY_TTL must be between 10s and 15s'
assert_rejected shared-principal 'must use distinct database principals'
assert_rejected widened-plan 'PLAN_SOURCE widens reviewed read or Artifact authority'

printf '%s\n' 'Agent Runtime G21.2 enabled preflight verification: passed.'
