#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
evaluator="${script_dir}/evaluate-agent-production-activation.py"
policy="${project_dir}/config/agent-runner/production-policy.json"
schema_dir="${project_dir}/docs/contracts/schemas"
fixture_dir="${project_dir}/docs/contracts/fixtures/agent-runtime"
activation_schema="${schema_dir}/neo-agent-broker-artifact-canary-activation.schema.json"
plan_schema="${schema_dir}/neo-agent-broker-artifact-canary-plan.schema.json"
held_fixture="${fixture_dir}/neo-agent-broker-artifact-canary-activation.valid.json"
invalid_fixture="${fixture_dir}/neo-agent-broker-artifact-canary-activation.invalid.json"
plan_fixture="${fixture_dir}/neo-agent-broker-artifact-canary-plan.valid.json"
invalid_plan="${fixture_dir}/neo-agent-broker-artifact-canary-plan.invalid.json"
python_bin="${project_dir}/rag/.venv/bin/python"

for command in python3 jq sha256sum date; do
  command -v "${command}" >/dev/null 2>&1 || { echo "G21.2 activation: ${command} is required" >&2; exit 1; }
done
if [[ ! -x "${python_bin}" ]]; then
  command -v uv >/dev/null 2>&1 || { echo 'G21.2 activation: uv is required' >&2; exit 1; }
  python_command=(uv run --project "${project_dir}/rag" --frozen --offline python)
else
  python_command=("${python_bin}")
fi
for path in "${evaluator}" "${policy}" "${activation_schema}" "${plan_schema}" \
  "${held_fixture}" "${invalid_fixture}" "${plan_fixture}" "${invalid_plan}"; do
  [[ -s "${path}" ]] || { echo 'G21.2 activation: missing contract artifact' >&2; exit 1; }
done

"${python_command[@]}" - "${activation_schema}" "${plan_schema}" "${held_fixture}" \
  "${invalid_fixture}" "${plan_fixture}" "${invalid_plan}" <<'PY'
import json,sys
from jsonschema import Draft202012Validator,FormatChecker
for schema_path, valid_path, invalid_path in ((sys.argv[1],sys.argv[3],sys.argv[4]),(sys.argv[2],sys.argv[5],sys.argv[6])):
    schema=json.load(open(schema_path,encoding="utf-8"))
    valid=json.load(open(valid_path,encoding="utf-8"))
    invalid=json.load(open(invalid_path,encoding="utf-8"))
    Draft202012Validator.check_schema(schema)
    validator=Draft202012Validator(schema,format_checker=FormatChecker())
    assert not list(validator.iter_errors(valid))
    assert list(validator.iter_errors(invalid))
    widened=dict(valid); widened["unexpectedG21Field"]=True
    assert list(validator.iter_errors(widened))
activation=json.load(open(sys.argv[1],encoding="utf-8"))
plan=json.load(open(sys.argv[2],encoding="utf-8"))
assert activation["properties"]["stage"]["const"]=="broker_artifact_canary"
assert activation["properties"]["release"]["properties"]["migrationHead"]["const"]==92
assert plan["properties"]["actions"]["minItems"]==plan["properties"]["actions"]["maxItems"]==5
PY

work_dir="$(mktemp -d)"
cleanup(){ find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true; rmdir "${work_dir}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM
cp "${plan_fixture}" "${work_dir}/plan.json"
for file in client.crt server-ca.crt authority-public-key relay-server.crt relay-client-ca.crt; do
  printf 'synthetic-g21.2-%s' "${file}" >"${work_dir}/${file}"
done
chmod 600 "${work_dir}"/*
sha(){ printf 'sha256:%s' "$(sha256sum "$1" | awk '{print $1}')"; }
endpoint='https://10.0.0.8:9443/internal/neo-runner/v1/rpc'
relay_endpoint='https://172.31.254.2:9444/internal/agent-broker/v1/relay'
read -r endpoint_sha relay_sha < <(python3 - "${endpoint}" "${relay_endpoint}" <<'PY'
import hashlib,sys
runner='sha256:'+hashlib.sha256(b'neo-agent-runner-endpoint-v1\0'+sys.argv[1].encode()).hexdigest()
relay='sha256:'+hashlib.sha256(b'neo-agent-broker-relay-endpoint-v1\0'+sys.argv[2].encode()).hexdigest()
print(runner,relay)
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
  .wiring.relayEndpointSha256=$relay |
  .wiring.relayServerCertificateSha256=$relaycert |
  .wiring.relayClientCASha256=$relayca |
  .window={startedAt:$started,completedAt:$completed,expiresAt:$expires} |
  .checks |= map(.result="passed" | .observedAt=$observed | .evidenceSha256=$nonzero | .detailCode="PASS") |
  .review={decision:"approved",reviewedAt:$reviewed,reviewerFingerprint:$nonzero}
' "${held_fixture}" >"${work_dir}/ready.json"
chmod 600 "${work_dir}/ready.json"

base_args=(
  --policy "${policy}" --client-certificate "${work_dir}/client.crt" --server-ca "${work_dir}/server-ca.crt"
  --canary-plan "${work_dir}/plan.json" --authority-public-key "${work_dir}/authority-public-key"
  --relay-endpoint "${relay_endpoint}" --relay-server-certificate "${work_dir}/relay-server.crt"
  --relay-client-ca "${work_dir}/relay-client-ca.crt"
  --runner-relay-identity spiffe://neo-chat/neo-runner-broker-relay
  --endpoint "${endpoint}" --runner-id neo-runner-primary --server-name neo-runner.internal
  --caller-identity spiffe://neo-chat/agent-runtime-broker-canary --release-commit "${commit}"
)
run_case(){
  local expected_status="$1" verdict="$2" reason="$3" record="$4"; shift 4
  local output status
  set +e; output="$(python3 "${evaluator}" --record "${record}" "${base_args[@]}" "$@")"; status=$?; set -e
  [[ "${status}" -eq "${expected_status}" ]] || { echo "${output}" >&2; exit 1; }
  jq -e --arg verdict "${verdict}" --arg reason "${reason}" \
    '.verdict==$verdict and .reasonCode==$reason' <<<"${output}" >/dev/null
}
run_case 0 ACTIVATION_READY BROKER_ARTIFACT_CANARY_GATES_PASSED "${work_dir}/ready.json"
jq '.authorization.brokerMutable=true' "${work_dir}/ready.json" >"${work_dir}/widened.json"; chmod 600 "${work_dir}/widened.json"
run_case 2 ACTIVATION_EVIDENCE_INVALID AUTHORIZATION_WIDENED "${work_dir}/widened.json"
run_case 2 ACTIVATION_EVIDENCE_INVALID RELAY_ENDPOINT_DRIFT "${work_dir}/ready.json" \
  --relay-endpoint https://172.31.254.3:9444/internal/agent-broker/v1/relay
set +e; held="$(python3 "${evaluator}" --policy "${policy}" --record "${held_fixture}")"; held_status=$?; set -e
[[ "${held_status}" -eq 3 ]] && grep -Fq '"reasonCode":"ISOLATION_UNAVAILABLE"' <<<"${held}"

echo 'Agent Runtime G21.2 activation contracts passed; checked-in evidence remains ISOLATION_UNAVAILABLE.'
