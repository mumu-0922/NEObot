#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
evaluator="${script_dir}/evaluate-agent-production-activation.py"
policy="${project_dir}/config/agent-runner/production-policy.json"
schema_dir="${project_dir}/docs/contracts/schemas"
fixture_dir="${project_dir}/docs/contracts/fixtures/agent-runtime"
python_bin="${project_dir}/rag/.venv/bin/python"

for command in date go jq python3 sha256sum; do
  command -v "${command}" >/dev/null 2>&1 || { echo "G21.3 activation: ${command} is required" >&2; exit 1; }
done
if [[ ! -x "${python_bin}" ]]; then
  command -v uv >/dev/null 2>&1 || { echo 'G21.3 activation: uv is required' >&2; exit 1; }
  python_command=(uv run --project "${project_dir}/rag" --frozen --offline python)
else
  python_command=("${python_bin}")
fi

contracts=(
  neo-agent-project-mutation-canary-activation
  neo-agent-project-mutation-canary-plan
  neo-agent-project-mutation-approval
)
for name in "${contracts[@]}"; do
  [[ -s "${schema_dir}/${name}.schema.json" ]] || { echo "G21.3 activation: missing ${name} schema" >&2; exit 1; }
  [[ -s "${fixture_dir}/${name}.valid.json" && -s "${fixture_dir}/${name}.invalid.json" ]] || {
    echo "G21.3 activation: missing ${name} fixtures" >&2; exit 1;
  }
done

"${python_command[@]}" - "${schema_dir}" "${fixture_dir}" "${contracts[@]}" <<'PY'
import json,sys
from pathlib import Path
from jsonschema import Draft202012Validator,FormatChecker
schemas,fixtures=map(Path,sys.argv[1:3])
for name in sys.argv[3:]:
    schema=json.loads((schemas/f"{name}.schema.json").read_text())
    valid=json.loads((fixtures/f"{name}.valid.json").read_text())
    invalid=json.loads((fixtures/f"{name}.invalid.json").read_text())
    Draft202012Validator.check_schema(schema)
    validator=Draft202012Validator(schema,format_checker=FormatChecker())
    assert not list(validator.iter_errors(valid)), name
    assert list(validator.iter_errors(invalid)), name
    widened=dict(valid); widened["unexpectedG21Field"]=True
    assert list(validator.iter_errors(widened)), name
activation=json.loads((schemas/"neo-agent-project-mutation-canary-activation.schema.json").read_text())
plan=json.loads((schemas/"neo-agent-project-mutation-canary-plan.schema.json").read_text())
approval=json.loads((schemas/"neo-agent-project-mutation-approval.schema.json").read_text())
assert activation["properties"]["stage"]["const"]=="project_mutation_canary"
assert activation["properties"]["release"]["properties"]["migrationHead"]["const"]==94
assert plan["properties"]["action"]["properties"]["approval"]["const"]=="per_commit"
assert approval["properties"]["payload"]["properties"]["release"]["properties"]["migrationHead"]["const"]==94
PY

(cd "${project_dir}/backend" && go test ./internal/agentactivation ./internal/agentprojectcanary \
  -run 'TestVerifyProjectCanaryRequiresMigration093ApprovalAndNarrowMutation|TestSignedApprovalBindsReleaseActivationPlanAndAction|TestActivationBindingFingerprintHasNoApprovalRecordHashCycle')

work_dir="$(mktemp -d)"
cleanup(){ find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true; rmdir "${work_dir}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM

cp "${fixture_dir}/neo-agent-project-mutation-canary-plan.valid.json" "${work_dir}/plan.json"
cp "${fixture_dir}/neo-agent-project-mutation-approval.valid.json" "${work_dir}/approval.json"
for file in client.crt server-ca.crt relay-server.crt relay-client-ca.crt; do
  printf 'synthetic-g21.3-%s' "${file}" >"${work_dir}/${file}"
done
python3 - "${work_dir}" <<'PY'
import base64,os,sys
from pathlib import Path
root=Path(sys.argv[1])
for name in ("authority-public-key","approval-public-key"):
    root.joinpath(name).write_text(base64.urlsafe_b64encode(os.urandom(32)).decode().rstrip("="))
PY
chmod 600 "${work_dir}"/*
sha(){ printf 'sha256:%s' "$(sha256sum "$1" | awk '{print $1}')"; }
endpoint='https://10.0.0.8:9443/internal/neo-runner/v1/rpc'
relay_endpoint='https://172.31.254.10:9445/internal/agent-broker/v1/relay'
read -r endpoint_sha relay_sha < <(python3 - "${endpoint}" "${relay_endpoint}" <<'PY'
import hashlib,sys
print('sha256:'+hashlib.sha256(b'neo-agent-runner-endpoint-v1\0'+sys.argv[1].encode()).hexdigest(),
      'sha256:'+hashlib.sha256(b'neo-agent-project-relay-endpoint-v1\0'+sys.argv[2].encode()).hexdigest())
PY
)

now_epoch="$(date -u +%s)"; stamp(){ date -u -d "@$1" '+%Y-%m-%dT%H:%M:%SZ'; }
started="$(stamp "$((now_epoch-300))")"; completed="$(stamp "$((now_epoch-120))")"
observed="$(stamp "$((now_epoch-180))")"; reviewed="$(stamp "$((now_epoch-60))")"
expires="$(stamp "$((now_epoch+3600))")"; commit='1111111111111111111111111111111111111111'
nonzero='sha256:1111111111111111111111111111111111111111111111111111111111111111'
jq --arg commit "${commit}" --arg policy "$(sha "${policy}")" --arg endpoint "${endpoint_sha}" \
  --arg relay "${relay_sha}" --arg client "$(sha "${work_dir}/client.crt")" \
  --arg ca "$(sha "${work_dir}/server-ca.crt")" --arg plan "$(sha "${work_dir}/plan.json")" \
  --arg authority "$(sha "${work_dir}/authority-public-key")" \
  --arg approval "$(sha "${work_dir}/approval.json")" \
  --arg approvalkey "$(sha "${work_dir}/approval-public-key")" \
  --arg relaycert "$(sha "${work_dir}/relay-server.crt")" \
  --arg relayca "$(sha "${work_dir}/relay-client-ca.crt")" \
  --arg started "${started}" --arg completed "${completed}" --arg observed "${observed}" \
  --arg reviewed "${reviewed}" --arg expires "${expires}" --arg nonzero "${nonzero}" '
  .evidenceClass="production" |
  .release.gitCommit=$commit |
  .release.operationsPolicySha256=$policy |
  .release.runnerManifestSha256=$nonzero |
  .release.runnerBinarySha256=$nonzero |
  .target.deploymentFingerprint=$nonzero |
  .wiring.endpointSha256=$endpoint |
  .wiring.clientCertificateSha256=$client |
  .wiring.serverCASha256=$ca |
  .wiring.canaryPlanSha256=$plan |
  .wiring.authorityPublicKeySha256=$authority |
  .wiring.approvalDocumentSha256=$approval |
  .wiring.approvalPublicKeySha256=$approvalkey |
  .wiring.relayEndpointSha256=$relay |
  .wiring.relayServerCertificateSha256=$relaycert |
  .wiring.relayClientCASha256=$relayca |
  .window={startedAt:$started,completedAt:$completed,expiresAt:$expires} |
  .checks |= map(.result="passed" | .observedAt=$observed | .evidenceSha256=$nonzero | .detailCode="PASS") |
  .review={decision:"approved",reviewedAt:$reviewed,reviewerFingerprint:$nonzero}
' "${fixture_dir}/neo-agent-project-mutation-canary-activation.valid.json" >"${work_dir}/ready.json"
chmod 600 "${work_dir}/ready.json"

base_args=(
  --policy "${policy}" --client-certificate "${work_dir}/client.crt" --server-ca "${work_dir}/server-ca.crt"
  --canary-plan "${work_dir}/plan.json" --authority-public-key "${work_dir}/authority-public-key"
  --approval-document "${work_dir}/approval.json" --approval-public-key "${work_dir}/approval-public-key"
  --relay-endpoint "${relay_endpoint}" --relay-server-certificate "${work_dir}/relay-server.crt"
  --relay-client-ca "${work_dir}/relay-client-ca.crt"
  --runner-relay-identity spiffe://neo-chat/neo-runner-project-relay
  --endpoint "${endpoint}" --runner-id neo-runner-primary --target-fingerprint "${nonzero}"
  --server-name neo-runner.internal --caller-identity spiffe://neo-chat/agent-runtime-project-canary
  --release-commit "${commit}"
)
run_case(){
  local expected_status="$1" verdict="$2" reason="$3" record="$4"; shift 4
  local output status
  set +e; output="$(python3 "${evaluator}" --record "${record}" "${base_args[@]}" "$@")"; status=$?; set -e
  [[ "${status}" -eq "${expected_status}" ]] || { echo "${output}" >&2; exit 1; }
  jq -e --arg verdict "${verdict}" --arg reason "${reason}" \
    '.verdict==$verdict and .reasonCode==$reason' <<<"${output}" >/dev/null
}
run_case 0 ACTIVATION_READY PROJECT_MUTATION_CANARY_GATES_PASSED "${work_dir}/ready.json"
jq '.authorization.brokerMutable=true' "${work_dir}/ready.json" >"${work_dir}/widened.json"; chmod 600 "${work_dir}/widened.json"
run_case 2 ACTIVATION_EVIDENCE_INVALID AUTHORIZATION_WIDENED "${work_dir}/widened.json"
printf 'drift' >>"${work_dir}/approval.json"
run_case 2 ACTIVATION_EVIDENCE_INVALID APPROVAL_DOCUMENT_DRIFT "${work_dir}/ready.json"

echo 'Agent Runtime G21.3 activation contracts passed; checked-in evidence remains non-promotional.'
