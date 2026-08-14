#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
evaluator="${script_dir}/evaluate-agent-production-activation.py"
policy="${project_dir}/config/agent-runner/production-policy.json"
schema="${project_dir}/docs/contracts/schemas/neo-agent-production-activation.schema.json"
held_fixture="${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-production-activation.valid.json"
invalid_fixture="${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-production-activation.invalid.json"

for command in python3 jq sha256sum date; do
  command -v "${command}" >/dev/null 2>&1 || {
    printf 'Agent production activation verification: %s is required\n' "${command}" >&2
    exit 1
  }
done
for path in "${evaluator}" "${policy}" "${schema}" "${held_fixture}" "${invalid_fixture}"; do
  [[ -s "${path}" ]] || { printf 'Agent production activation verification: missing artifact\n' >&2; exit 1; }
done

work_dir="$(mktemp -d)"
cleanup(){ find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true; rmdir "${work_dir}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

python3 - "${schema}" "${held_fixture}" "${invalid_fixture}" <<'PY'
import json,sys
schema=json.load(open(sys.argv[1],encoding="utf-8"))
valid=json.load(open(sys.argv[2],encoding="utf-8"))
invalid=json.load(open(sys.argv[3],encoding="utf-8"))
assert schema["$schema"].endswith("2020-12/schema")
assert schema["additionalProperties"] is False
assert set(valid) == set(schema["required"])
assert valid["schemaVersion"] == schema["properties"]["schemaVersion"]["const"]
assert valid["stage"] == schema["properties"]["stage"]["const"]
assert invalid.get("stage") != schema["properties"]["stage"]["const"]
PY

manifest="${work_dir}/release-manifest.json"
runner_binary="${work_dir}/neo-runnerd"
client_certificate="${work_dir}/client.crt"
server_ca="${work_dir}/server-ca.crt"
cp "${project_dir}/config/agent-runner/release-manifest.example.json" "${manifest}"
sed -i \
  -e 's/"approved": false/"approved": true/' \
  -e 's/sha256:0000000000000000000000000000000000000000000000000000000000000000/sha256:1111111111111111111111111111111111111111111111111111111111111111/g' \
  "${manifest}"
printf '%s' 'synthetic-runner-binary' >"${runner_binary}"
printf '%s' 'synthetic-client-certificate' >"${client_certificate}"
printf '%s' 'synthetic-server-ca' >"${server_ca}"
chmod 600 "${manifest}" "${runner_binary}" "${client_certificate}" "${server_ca}"
endpoint='https://10.0.0.8:9443/internal/neo-runner/v1/rpc'
endpoint_sha="$(python3 - "${endpoint}" <<'PY'
import hashlib,sys
print('sha256:'+hashlib.sha256(b'neo-agent-runner-endpoint-v1\0'+sys.argv[1].encode()).hexdigest())
PY
)"
policy_sha="sha256:$(sha256sum "${policy}" | awk '{print $1}')"
manifest_sha="sha256:$(sha256sum "${manifest}" | awk '{print $1}')"
runner_sha="sha256:$(sha256sum "${runner_binary}" | awk '{print $1}')"
client_sha="sha256:$(sha256sum "${client_certificate}" | awk '{print $1}')"
ca_sha="sha256:$(sha256sum "${server_ca}" | awk '{print $1}')"
evidence_sha="sha256:$(printf activation-evidence | sha256sum | awk '{print $1}')"
reviewer_sha="sha256:$(printf activation-reviewer | sha256sum | awk '{print $1}')"
deployment_sha="sha256:$(printf activation-deployment | sha256sum | awk '{print $1}')"
release_commit='1111111111111111111111111111111111111111'
now_epoch="$(date -u +%s)"
format_epoch(){ date -u -d "@$1" '+%Y-%m-%dT%H:%M:%SZ'; }
started_at="$(format_epoch "$((now_epoch - 300))")"
completed_at="$(format_epoch "$((now_epoch - 120))")"
observed_at="$(format_epoch "$((now_epoch - 180))")"
reviewed_at="$(format_epoch "$((now_epoch - 60))")"
expires_at="$(format_epoch "$((now_epoch + 3600))")"
future_reviewed_at="$(format_epoch "$((now_epoch + 600))")"
stale_started_at="$(format_epoch "$((now_epoch - 7200))")"
stale_completed_at="$(format_epoch "$((now_epoch - 7000))")"
stale_observed_at="$(format_epoch "$((now_epoch - 7100))")"
stale_reviewed_at="$(format_epoch "$((now_epoch - 6900))")"
stale_expires_at="$(format_epoch "$((now_epoch - 3600))")"

jq \
  --arg started "${started_at}" --arg completed "${completed_at}" \
  --arg observed "${observed_at}" --arg reviewed "${reviewed_at}" --arg expires "${expires_at}" \
  --arg policy "${policy_sha}" --arg manifest "${manifest_sha}" --arg runner "${runner_sha}" \
  --arg client "${client_sha}" --arg ca "${ca_sha}" --arg endpoint "${endpoint_sha}" \
  --arg evidence "${evidence_sha}" --arg reviewer "${reviewer_sha}" \
  --arg deployment "${deployment_sha}" --arg commit "${release_commit}" '
  .evidenceClass="production" |
  .release.gitCommit=$commit |
  .release.runnerManifestSha256=$manifest |
  .release.runnerBinarySha256=$runner |
  .release.operationsPolicySha256=$policy |
  .target.deploymentFingerprint=$deployment |
  .wiring.endpointSha256=$endpoint |
  .wiring.clientCertificateSha256=$client |
  .wiring.serverCASha256=$ca |
  .window.startedAt=$started |
  .window.completedAt=$completed |
  .window.expiresAt=$expires |
  .checks |= map(.result="passed" | .observedAt=$observed | .evidenceSha256=$evidence | .detailCode="PASS") |
  .review.decision="approved" |
  .review.reviewedAt=$reviewed |
  .review.reviewerFingerprint=$reviewer
' "${held_fixture}" >"${work_dir}/ready.json"
chmod 600 "${work_dir}/ready.json"

base_args=(
  --policy "${policy}" --release-manifest "${manifest}" --runner-binary "${runner_binary}"
  --client-certificate "${client_certificate}" --server-ca "${server_ca}" --endpoint "${endpoint}"
  --runner-id neo-runner-primary --server-name neo-runner.internal
  --caller-identity spiffe://neo-chat/agent-runtime-control --release-commit "${release_commit}"
)
run_case(){
  local name="$1" expected_status="$2" expected_verdict="$3" expected_reason="$4" record="$5"; shift 5
  local output="${work_dir}/${name}.decision.json" status
  set +e
  python3 "${evaluator}" --record "${record}" "${base_args[@]}" "$@" >"${output}"
  status=$?
  set -e
  [[ "${status}" -eq "${expected_status}" ]] || { cat "${output}" >&2; exit 1; }
  jq -e --arg verdict "${expected_verdict}" --arg reason "${expected_reason}" \
    '.verdict==$verdict and .reasonCode==$reason' "${output}" >/dev/null
}

run_case ready 0 ACTIVATION_READY CONTROL_PLANE_GATES_PASSED "${work_dir}/ready.json"
jq '.evidenceClass="template"' "${work_dir}/ready.json" >"${work_dir}/template.json"; chmod 600 "${work_dir}/template.json"
run_case template 3 ACTIVATION_HELD NON_PRODUCTION_EVIDENCE "${work_dir}/template.json"
jq '.authorization.rootRuns=true' "${work_dir}/ready.json" >"${work_dir}/widened.json"; chmod 600 "${work_dir}/widened.json"
run_case widened 2 ACTIVATION_EVIDENCE_INVALID AUTHORIZATION_WIDENED "${work_dir}/widened.json"
jq '.cleanup.orphanSandboxes=1' "${work_dir}/ready.json" >"${work_dir}/residue.json"; chmod 600 "${work_dir}/residue.json"
run_case residue 3 ACTIVATION_HELD RUNTIME_RESIDUE_REMAINS "${work_dir}/residue.json"
jq --arg reviewed "${future_reviewed_at}" '.review.reviewedAt=$reviewed' \
  "${work_dir}/ready.json" >"${work_dir}/future-review.json"; chmod 600 "${work_dir}/future-review.json"
run_case future-review 2 ACTIVATION_EVIDENCE_INVALID REVIEW_INVALID "${work_dir}/future-review.json"
jq \
  --arg started "${stale_started_at}" --arg completed "${stale_completed_at}" \
  --arg observed "${stale_observed_at}" --arg reviewed "${stale_reviewed_at}" \
  --arg expires "${stale_expires_at}" '
  .window={startedAt:$started,completedAt:$completed,expiresAt:$expires} |
  .checks |= map(.observedAt=$observed) |
  .review.reviewedAt=$reviewed
' "${work_dir}/ready.json" >"${work_dir}/stale.json"; chmod 600 "${work_dir}/stale.json"
run_case stale 3 ACTIVATION_HELD EVIDENCE_STALE "${work_dir}/stale.json"
jq '.checks[-1]=.checks[0]' "${work_dir}/ready.json" >"${work_dir}/duplicate.json"; chmod 600 "${work_dir}/duplicate.json"
run_case duplicate 2 ACTIVATION_EVIDENCE_INVALID CHECK_SET_INVALID "${work_dir}/duplicate.json"
jq '.checks=(.checks[0:4])' "${work_dir}/ready.json" >"${work_dir}/incomplete.json"; chmod 600 "${work_dir}/incomplete.json"
run_case incomplete 2 ACTIVATION_EVIDENCE_INVALID CHECK_SET_INCOMPLETE "${work_dir}/incomplete.json"
run_case endpoint-drift 2 ACTIVATION_EVIDENCE_INVALID RUNNER_ENDPOINT_DRIFT "${work_dir}/ready.json" --endpoint https://10.0.0.9:9443/internal/neo-runner/v1/rpc
unapproved_manifest="${work_dir}/unapproved-release-manifest.json"
cp "${project_dir}/config/agent-runner/release-manifest.example.json" "${unapproved_manifest}"
chmod 600 "${unapproved_manifest}"
unapproved_sha="sha256:$(sha256sum "${unapproved_manifest}" | awk '{print $1}')"
jq --arg manifest "${unapproved_sha}" '.release.runnerManifestSha256=$manifest' \
  "${work_dir}/ready.json" >"${work_dir}/unapproved.json"
chmod 600 "${work_dir}/unapproved.json"
run_case unapproved 2 ACTIVATION_EVIDENCE_INVALID RUNNER_MANIFEST_INVALID \
  "${work_dir}/unapproved.json" --release-manifest "${unapproved_manifest}"
cp "${invalid_fixture}" "${work_dir}/invalid.json"; chmod 600 "${work_dir}/invalid.json"
run_case malformed 2 ACTIVATION_EVIDENCE_INVALID ACTIVATION_ROOT_INVALID "${work_dir}/invalid.json"

ln -s "${work_dir}/ready.json" "${work_dir}/ready-link.json"
run_case symlink 2 ACTIVATION_EVIDENCE_INVALID DOCUMENT_UNREADABLE "${work_dir}/ready-link.json"
cp "${work_dir}/ready.json" "${work_dir}/writable.json"
chmod 666 "${work_dir}/writable.json"
run_case writable 2 ACTIVATION_EVIDENCE_INVALID DOCUMENT_UNREADABLE "${work_dir}/writable.json"

set +e
held_output="$(python3 "${evaluator}" --policy "${policy}" --record "${held_fixture}")"
held_status=$?
set -e
[[ "${held_status}" -eq 3 ]] && grep -Fq '"reasonCode":"ISOLATION_UNAVAILABLE"' <<<"${held_output}"

printf '%s\n' 'Agent Runtime G21.0 activation contract passed; checked-in evidence remains ISOLATION_UNAVAILABLE.'
