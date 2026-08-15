#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
preflight="${script_dir}/preflight-single-server.sh"
example_env="${project_dir}/.env.single-server.example"
fixtures="${project_dir}/docs/contracts/fixtures/agent-runtime"
policy="${project_dir}/config/agent-runner/production-policy.json"
work_dir="$(mktemp -d)"
cleanup(){ find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true; rmdir "${work_dir}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

for command in date jq openssl python3 sed sha256sum; do
  command -v "${command}" >/dev/null 2>&1 || { echo "G21.5 preflight: ${command} is required" >&2; exit 1; }
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
    -config "${work_dir}/${name}.cnf" -keyout "${work_dir}/${name}.key" \
    -out "${work_dir}/${name}.crt" >/dev/null 2>&1
}
make_certificate draft-client spiffe://neo-chat/agent-runtime-draft-learning
make_certificate draft-ca neo-runner-draft-learning-test-ca

openssl genpkey -algorithm Ed25519 -out "${work_dir}/authority.pem" >/dev/null 2>&1
openssl pkey -in "${work_dir}/authority.pem" -outform DER -out "${work_dir}/authority.der"
openssl pkey -in "${work_dir}/authority.pem" -pubout -outform DER -out "${work_dir}/authority-public.der"
python3 - "${work_dir}/authority.der" "${work_dir}/authority-public.der" \
  "${work_dir}/authority-private-key" "${work_dir}/authority-public-key" <<'PY'
import base64,sys
private=open(sys.argv[1],'rb').read()[-32:]
public=open(sys.argv[2],'rb').read()[-32:]
open(sys.argv[3],'w').write(base64.urlsafe_b64encode(private+public).decode().rstrip('=')+'\n')
open(sys.argv[4],'w').write(base64.urlsafe_b64encode(public).decode().rstrip('=')+'\n')
PY

cp "${fixtures}/neo-agent-cron-worker-plan.valid.json" "${work_dir}/cron-plan.json"
cp "${fixtures}/neo-agent-draft-learning-worker-plan.valid.json" "${work_dir}/draft-plan.json"
printf '%s\n' '{"credentialId":"draft-learning-cleanup-v1","scope":"exact-object-read-delete"}' \
  >"${work_dir}/object-credential.json"
for stage in control root broker project child; do
  printf '{"stage":"%s","ready":true}\n' "${stage}" >"${work_dir}/prerequisite-${stage}.json"
done
nonzero='sha256:1111111111111111111111111111111111111111111111111111111111111111'
jq --arg nonzero "${nonzero}" '
  .approved=true | .runnerVersion="g21.5-test" |
  .binaries |= map(.sha256=$nonzero) |
  .seccompProfile.sha256=$nonzero | .probeSuiteFingerprint=$nonzero |
  .isolationAcceptance.sha256=$nonzero
' "${project_dir}/config/agent-runner/release-manifest.example.json" >"${work_dir}/release-manifest.json"
chmod 600 "${work_dir}"/*
sha(){ printf 'sha256:%s' "$(sha256sum "$1" | awk '{print $1}')"; }
endpoint='https://10.0.0.8:9443/internal/neo-runner/v1/rpc'
endpoint_sha="$(python3 - "${endpoint}" <<'PY'
import hashlib,sys
print('sha256:'+hashlib.sha256(b'neo-agent-runner-endpoint-v1\0'+sys.argv[1].encode()).hexdigest())
PY
)"
now_epoch="$(date -u +%s)"
stamp(){ date -u -d "@$1" '+%Y-%m-%dT%H:%M:%SZ'; }
started="$(stamp "$((now_epoch-300))")"; completed="$(stamp "$((now_epoch-120))")"
observed="$(stamp "$((now_epoch-180))")"; reviewed="$(stamp "$((now_epoch-60))")"
expires="$(stamp "$((now_epoch+3600))")"; commit='1111111111111111111111111111111111111111'

prerequisite_jq='map(if .stage=="control_plane" then .evidenceSha256=$control
  elif .stage=="root_run_canary" then .evidenceSha256=$root
  elif .stage=="broker_artifact_canary" then .evidenceSha256=$broker
  elif .stage=="project_mutation_canary" then .evidenceSha256=$project
  elif .stage=="child_run_canary" then .evidenceSha256=$child else . end)'
common_jq_args=(
  --arg commit "${commit}" --arg policy "$(sha "${policy}")" --arg nonzero "${nonzero}"
  --arg started "${started}" --arg completed "${completed}" --arg observed "${observed}"
  --arg reviewed "${reviewed}" --arg expires "${expires}"
  --arg control "$(sha "${work_dir}/prerequisite-control.json")"
  --arg root "$(sha "${work_dir}/prerequisite-root.json")"
  --arg broker "$(sha "${work_dir}/prerequisite-broker.json")"
  --arg project "$(sha "${work_dir}/prerequisite-project.json")"
  --arg child "$(sha "${work_dir}/prerequisite-child.json")"
)

jq "${common_jq_args[@]}" --arg plan "$(sha "${work_dir}/cron-plan.json")" "
  .evidenceClass=\"production\" | .release.gitCommit=\$commit |
  .release.runnerManifestSha256=\$nonzero | .release.runnerBinarySha256=\$nonzero |
  .release.operationsPolicySha256=\$policy | .target.planSha256=\$plan |
  .prerequisites |= ${prerequisite_jq} |
  .window={startedAt:\$started,completedAt:\$completed,expiresAt:\$expires} |
  .checks |= map(.result=\"passed\" | .observedAt=\$observed | .evidenceSha256=\$nonzero | .detailCode=\"PASS\") |
  .review={decision:\"approved\",reviewedAt:\$reviewed,reviewerFingerprint:\$nonzero}
" "${fixtures}/neo-agent-cron-worker-activation.valid.json" >"${work_dir}/cron-activation.json"

jq "${common_jq_args[@]}" --arg plan "$(sha "${work_dir}/draft-plan.json")" \
  --arg endpoint "${endpoint_sha}" --arg manifest "$(sha "${work_dir}/release-manifest.json")" \
  --arg client "$(sha "${work_dir}/draft-client.crt")" --arg ca "$(sha "${work_dir}/draft-ca.crt")" \
  --arg authority "$(sha "${work_dir}/authority-public-key")" \
  --arg object "$(sha "${work_dir}/object-credential.json")" "
  .evidenceClass=\"production\" | .release.gitCommit=\$commit |
  .release.runnerManifestSha256=\$manifest | .release.runnerBinarySha256=\$nonzero |
  .release.operationsPolicySha256=\$policy | .target.planSha256=\$plan |
  .prerequisites |= ${prerequisite_jq} |
  .wiring.endpointSha256=\$endpoint | .wiring.clientCertificateSha256=\$client |
  .wiring.serverCASha256=\$ca | .wiring.authorityPublicKeySha256=\$authority |
  .wiring.objectCredentialMetaSha256=\$object |
  .window={startedAt:\$started,completedAt:\$completed,expiresAt:\$expires} |
  .checks |= map(.result=\"passed\" | .observedAt=\$observed | .evidenceSha256=\$nonzero | .detailCode=\"PASS\") |
  .review={decision:\"approved\",reviewedAt:\$reviewed,reviewerFingerprint:\$nonzero}
" "${fixtures}/neo-agent-draft-learning-worker-activation.valid.json" >"${work_dir}/draft-activation.json"
chmod 600 "${work_dir}"/*.json

printf '%s\n' '{"v":1,"activeKid":"test-v1","keys":[{"kid":"test-v1","key":"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY"}]}' \
  >"${work_dir}/provider-keyring.json"
base_env="${work_dir}/base.env"
sed \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-frontend@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-rag@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-postgres@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-postgres@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd|' \
  -e 's|ghcr.io/mumu-0922/neobot-mm-chat-mcp-runner@sha256:replace-with-64-lowercase-hex|ghcr.io/mumu-0922/neobot-mm-chat-mcp-runner@sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee|' \
  -e 's|replace-with-release-id|git-g215-test|' \
  -e "s|replace-with-host-uid|$(id -u)|" -e "s|replace-with-host-gid|$(id -g)|" \
  -e 's|change-me-migrator-postgres|test-migrator-password|g' \
  -e 's|change-me-api-postgres|test-api-password|g' \
  -e 's|change-me-memory-worker-postgres|test-memory-worker-password|g' \
  -e 's|change-me-rag-worker-postgres|test-rag-worker-password|g' \
  -e 's|change-me-rag-replay-postgres|test-rag-replay-password|g' \
  -e 's|change-me-agent-cron-worker-postgres|test-agent-cron-worker-password|g' \
  -e 's|change-me-agent-learning-worker-postgres|test-agent-learning-worker-password|g' \
  -e 's|change-me-redis|test-redis-password|g' \
  -e 's|change-me-minio-root-secret|test-minio-root-password|g' \
  -e 's|change-me-minio-user-secret|test-minio-app-password|g' \
  -e 's|change-me-base64-32-byte-random-key|MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=|g' \
  -e 's|https://change-me.example/invites/accept|https://chat.internal/invites/accept|' \
  -e "s|^PROVIDER_SECRET_KEYRING_SOURCE=.*|PROVIDER_SECRET_KEYRING_SOURCE=${work_dir}/provider-keyring.json|" \
  -e "s|^AGENT_PRODUCTION_ACTIVATION_SOURCE=.*|AGENT_PRODUCTION_ACTIVATION_SOURCE=${work_dir}/prerequisite-control.json|" \
  -e "s|^AGENT_ROOT_CANARY_ACTIVATION_SOURCE=.*|AGENT_ROOT_CANARY_ACTIVATION_SOURCE=${work_dir}/prerequisite-root.json|" \
  -e "s|^AGENT_BROKER_CANARY_ACTIVATION_SOURCE=.*|AGENT_BROKER_CANARY_ACTIVATION_SOURCE=${work_dir}/prerequisite-broker.json|" \
  -e "s|^AGENT_PROJECT_CANARY_ACTIVATION_SOURCE=.*|AGENT_PROJECT_CANARY_ACTIVATION_SOURCE=${work_dir}/prerequisite-project.json|" \
  -e "s|^AGENT_CHILD_CANARY_ACTIVATION_SOURCE=.*|AGENT_CHILD_CANARY_ACTIVATION_SOURCE=${work_dir}/prerequisite-child.json|" \
  -e "s|^AGENT_CRON_WORKER_PLAN_SOURCE=.*|AGENT_CRON_WORKER_PLAN_SOURCE=${work_dir}/cron-plan.json|" \
  -e "s|^AGENT_CRON_WORKER_PRODUCTION_POLICY_SOURCE=.*|AGENT_CRON_WORKER_PRODUCTION_POLICY_SOURCE=${policy}|" \
  -e "s|^AGENT_CRON_WORKER_ACTIVATION_SOURCE=.*|AGENT_CRON_WORKER_ACTIVATION_SOURCE=${work_dir}/cron-activation.json|" \
  -e "s|^AGENT_CRON_WORKER_RELEASE_GIT_COMMIT=.*|AGENT_CRON_WORKER_RELEASE_GIT_COMMIT=${commit}|" \
  -e "s|^AGENT_DRAFT_LEARNING_WORKER_RUNNER_URL=.*|AGENT_DRAFT_LEARNING_WORKER_RUNNER_URL=${endpoint}|" \
  -e "s|^AGENT_DRAFT_LEARNING_WORKER_PLAN_SOURCE=.*|AGENT_DRAFT_LEARNING_WORKER_PLAN_SOURCE=${work_dir}/draft-plan.json|" \
  -e "s|^AGENT_DRAFT_LEARNING_WORKER_CLIENT_CERT_SOURCE=.*|AGENT_DRAFT_LEARNING_WORKER_CLIENT_CERT_SOURCE=${work_dir}/draft-client.crt|" \
  -e "s|^AGENT_DRAFT_LEARNING_WORKER_CLIENT_KEY_SOURCE=.*|AGENT_DRAFT_LEARNING_WORKER_CLIENT_KEY_SOURCE=${work_dir}/draft-client.key|" \
  -e "s|^AGENT_DRAFT_LEARNING_WORKER_SERVER_CA_SOURCE=.*|AGENT_DRAFT_LEARNING_WORKER_SERVER_CA_SOURCE=${work_dir}/draft-ca.crt|" \
  -e "s|^AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PRIVATE_KEY_SOURCE=.*|AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PRIVATE_KEY_SOURCE=${work_dir}/authority-private-key|" \
  -e "s|^AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PUBLIC_KEY_SOURCE=.*|AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PUBLIC_KEY_SOURCE=${work_dir}/authority-public-key|" \
  -e "s|^AGENT_DRAFT_LEARNING_WORKER_RELEASE_MANIFEST_SOURCE=.*|AGENT_DRAFT_LEARNING_WORKER_RELEASE_MANIFEST_SOURCE=${work_dir}/release-manifest.json|" \
  -e "s|^AGENT_DRAFT_LEARNING_WORKER_PRODUCTION_POLICY_SOURCE=.*|AGENT_DRAFT_LEARNING_WORKER_PRODUCTION_POLICY_SOURCE=${policy}|" \
  -e "s|^AGENT_DRAFT_LEARNING_WORKER_ACTIVATION_SOURCE=.*|AGENT_DRAFT_LEARNING_WORKER_ACTIVATION_SOURCE=${work_dir}/draft-activation.json|" \
  -e "s|^AGENT_DRAFT_LEARNING_WORKER_OBJECT_CREDENTIAL_META_SOURCE=.*|AGENT_DRAFT_LEARNING_WORKER_OBJECT_CREDENTIAL_META_SOURCE=${work_dir}/object-credential.json|" \
  -e "s|^AGENT_DRAFT_LEARNING_WORKER_RELEASE_GIT_COMMIT=.*|AGENT_DRAFT_LEARNING_WORKER_RELEASE_GIT_COMMIT=${commit}|" \
  -e 's|^AGENT_DRAFT_LEARNING_WORKER_S3_ACCESS_KEY_ID=.*|AGENT_DRAFT_LEARNING_WORKER_S3_ACCESS_KEY_ID=draft-learning-test-access|' \
  -e 's|^AGENT_DRAFT_LEARNING_WORKER_S3_SECRET_ACCESS_KEY=.*|AGENT_DRAFT_LEARNING_WORKER_S3_SECRET_ACCESS_KEY=draft-learning-test-secret|' \
  "${example_env}" >"${base_env}"
chmod 600 "${base_env}"

run_enabled(){
  local cron="$1" draft="$2" output
  output="${work_dir}/enabled-${cron}-${draft}.env"
  sed -e "s|^AGENT_CRON_WORKER_ENABLED=false$|AGENT_CRON_WORKER_ENABLED=${cron}|" \
    -e "s|^AGENT_DRAFT_LEARNING_WORKER_ENABLED=false$|AGENT_DRAFT_LEARNING_WORKER_ENABLED=${draft}|" \
    "${base_env}" >"${output}"
  chmod 600 "${output}"
  bash "${preflight}" "${output}" >/dev/null
}
run_enabled true false
run_enabled false true
run_enabled true true

assert_rejected(){
  local file="$1" expected="$2" output
  if output="$(bash "${preflight}" "${file}" 2>&1)"; then
    echo "G21.5 preflight: accepted negative ${file}" >&2; exit 1
  fi
  [[ "${output}" == *"${expected}"* ]] || { printf '%s\n' "${output}" >&2; exit 1; }
}
cron_bad="${work_dir}/cron-credential.env"
cp "${work_dir}/enabled-true-false.env" "${cron_bad}"
printf '%s\n' 'AGENT_CRON_WORKER_RUNNER_TOKEN=forbidden-value' >>"${cron_bad}"
chmod 600 "${cron_bad}"
assert_rejected "${cron_bad}" 'Cron worker must not receive Runner, object-store or administrator credentials'
draft_bad="${work_dir}/draft-promote.env"
cp "${work_dir}/enabled-false-true.env" "${draft_bad}"
printf '%s\n' 'AGENT_DRAFT_LEARNING_WORKER_PROMOTE_TOKEN=forbidden-value' >>"${draft_bad}"
chmod 600 "${draft_bad}"
assert_rejected "${draft_bad}" 'must not receive Promote, Broker or product credentials'
held="${work_dir}/cron-held.env"
cp "${fixtures}/neo-agent-cron-worker-activation.valid.json" "${work_dir}/cron-held-activation.json"
chmod 600 "${work_dir}/cron-held-activation.json"
sed "s|^AGENT_CRON_WORKER_ACTIVATION_SOURCE=.*|AGENT_CRON_WORKER_ACTIVATION_SOURCE=${work_dir}/cron-held-activation.json|" \
  "${work_dir}/enabled-true-false.env" >"${held}"
chmod 600 "${held}"
assert_rejected "${held}" 'G21.5 activation is not READY: CRON_WORKER_GATES_PASSED'

printf '%s\n' 'Agent Runtime G21.5 independent enabled preflight verification: passed.'
