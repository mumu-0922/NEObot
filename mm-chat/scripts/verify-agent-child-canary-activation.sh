#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
evaluator="${script_dir}/evaluate-agent-production-activation.py"
policy="${project_dir}/config/agent-runner/production-policy.json"
schema_dir="${project_dir}/docs/contracts/schemas"
fixture_dir="${project_dir}/docs/contracts/fixtures/agent-runtime"
activation_schema="${schema_dir}/neo-agent-child-run-canary-activation.schema.json"
plan_schema="${schema_dir}/neo-agent-child-run-canary-plan.schema.json"
held_fixture="${fixture_dir}/neo-agent-child-run-canary-activation.valid.json"
invalid_fixture="${fixture_dir}/neo-agent-child-run-canary-activation.invalid.json"
plan_fixture="${fixture_dir}/neo-agent-child-run-canary-plan.valid.json"
invalid_plan="${fixture_dir}/neo-agent-child-run-canary-plan.invalid.json"
python_bin="${project_dir}/rag/.venv/bin/python"

for command in date go jq python3 sha256sum; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "G21.4 activation: ${command} is required" >&2
    exit 1
  }
done
for path in "${evaluator}" "${policy}" "${activation_schema}" "${plan_schema}" \
  "${held_fixture}" "${invalid_fixture}" "${plan_fixture}" "${invalid_plan}"; do
  [[ -s "${path}" ]] || { echo 'G21.4 activation: missing contract artifact' >&2; exit 1; }
done
if [[ ! -x "${python_bin}" ]]; then
  command -v uv >/dev/null 2>&1 || { echo 'G21.4 activation: uv is required' >&2; exit 1; }
  python_command=(uv run --project "${project_dir}/rag" --frozen --offline python)
else
  python_command=("${python_bin}")
fi

"${python_command[@]}" - "${activation_schema}" "${plan_schema}" "${held_fixture}" \
  "${invalid_fixture}" "${plan_fixture}" "${invalid_plan}" <<'PY'
import json,sys
from jsonschema import Draft202012Validator,FormatChecker
activation_schema,plan_schema,held,invalid,plan,bad_plan=(json.load(open(path,encoding="utf-8")) for path in sys.argv[1:])
for schema in (activation_schema,plan_schema):
    Draft202012Validator.check_schema(schema)
activation_validator=Draft202012Validator(activation_schema,format_checker=FormatChecker())
plan_validator=Draft202012Validator(plan_schema,format_checker=FormatChecker())
assert not list(activation_validator.iter_errors(held))
assert list(activation_validator.iter_errors(invalid))
assert not list(plan_validator.iter_errors(plan))
assert list(plan_validator.iter_errors(bad_plan))
assert activation_schema["properties"]["stage"]["const"] == "depth_one_child_canary"
assert activation_schema["properties"]["release"]["properties"]["migrationHead"]["const"] == 95
assert activation_schema["properties"]["authorization"]["properties"]["childAgents"]["const"] is True
assert activation_schema["properties"]["authorization"]["properties"]["brokerReadOnly"]["const"] is False
assert plan["toolCatalog"] == [{"identity":"delegate_task","capability":"delegate_task","actions":["create"],"classification":"mutable","idempotent":False}]
assert plan["parentRequestedTools"] == ["delegate_task"]
assert plan["childRequestedTools"] == ["delegate_task"]
assert all(plan["childBudget"][key] < plan["parentBudget"][key] for key in plan["parentBudget"])
PY

work_dir="$(mktemp -d)"
cleanup(){ find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true; rmdir "${work_dir}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM
cp "${plan_fixture}" "${work_dir}/plan.json"
printf '%s' 'synthetic-g21.4-client-certificate' >"${work_dir}/client.crt"
printf '%s' 'synthetic-g21.4-server-ca' >"${work_dir}/server-ca.crt"
printf '%s' 'synthetic-g21.4-ed25519-public-key' >"${work_dir}/authority-public-key"
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
started="$(stamp "$((now_epoch-300))")"
completed="$(stamp "$((now_epoch-120))")"
observed="$(stamp "$((now_epoch-180))")"
reviewed="$(stamp "$((now_epoch-60))")"
expires="$(stamp "$((now_epoch+3600))")"
commit='1111111111111111111111111111111111111111'
nonzero='sha256:1111111111111111111111111111111111111111111111111111111111111111'
jq --arg commit "${commit}" --arg policy "$(sha "${policy}")" --arg endpoint "${endpoint_sha}" \
  --arg client "$(sha "${work_dir}/client.crt")" --arg ca "$(sha "${work_dir}/server-ca.crt")" \
  --arg plan "$(sha "${work_dir}/plan.json")" --arg authority "$(sha "${work_dir}/authority-public-key")" \
  --arg started "${started}" --arg completed "${completed}" --arg observed "${observed}" \
  --arg reviewed "${reviewed}" --arg expires "${expires}" --arg nonzero "${nonzero}" '
  .evidenceClass="production" |
  .release.gitCommit=$commit |
  .release.runnerManifestSha256=$nonzero |
  .release.runnerBinarySha256=$nonzero |
  .release.operationsPolicySha256=$policy |
  .target.deploymentFingerprint=$nonzero |
  .wiring.endpointSha256=$endpoint |
  .wiring.clientCertificateSha256=$client |
  .wiring.serverCASha256=$ca |
  .wiring.canaryPlanSha256=$plan |
  .wiring.authorityPublicKeySha256=$authority |
  .window={startedAt:$started,completedAt:$completed,expiresAt:$expires} |
  .checks |= map(.result="passed" | .observedAt=$observed | .evidenceSha256=$nonzero | .detailCode="PASS") |
  .review={decision:"approved",reviewedAt:$reviewed,reviewerFingerprint:$nonzero}
' "${held_fixture}" >"${work_dir}/ready.json"
chmod 600 "${work_dir}/ready.json"

base_args=(
  --policy "${policy}" --client-certificate "${work_dir}/client.crt" --server-ca "${work_dir}/server-ca.crt"
  --canary-plan "${work_dir}/plan.json" --authority-public-key "${work_dir}/authority-public-key"
  --endpoint "${endpoint}" --runner-id neo-runner-primary --server-name neo-runner.internal
  --caller-identity spiffe://neo-chat/agent-runtime-child-canary --release-commit "${commit}"
)
run_case(){
  local expected_status="$1" verdict="$2" reason="$3" record="$4"
  local output status
  set +e
  output="$(python3 "${evaluator}" --record "${record}" "${base_args[@]}")"; status=$?
  set -e
  [[ "${status}" -eq "${expected_status}" ]] || { echo "${output}" >&2; exit 1; }
  jq -e --arg verdict "${verdict}" --arg reason "${reason}" \
    '.verdict==$verdict and .reasonCode==$reason' <<<"${output}" >/dev/null
}
run_case 0 ACTIVATION_READY DEPTH_ONE_CHILD_CANARY_GATES_PASSED "${work_dir}/ready.json"

jq '.authorization.childAgents=false' "${work_dir}/ready.json" >"${work_dir}/widened.json"
chmod 600 "${work_dir}/widened.json"
run_case 2 ACTIVATION_EVIDENCE_INVALID AUTHORIZATION_WIDENED "${work_dir}/widened.json"

printf '%s' 'drifted-plan' >"${work_dir}/plan.json"
run_case 2 ACTIVATION_EVIDENCE_INVALID CANARY_PLAN_DRIFT "${work_dir}/ready.json"

set +e
held_output="$(python3 "${evaluator}" --record "${held_fixture}")"; held_status=$?
set -e
[[ "${held_status}" -eq 3 ]]
jq -e '.verdict=="ACTIVATION_HELD" and .reasonCode=="ISOLATION_UNAVAILABLE"' <<<"${held_output}" >/dev/null

(cd "${project_dir}/backend" && go test \
  ./internal/agentactivation ./internal/agentchildcanary \
  ./cmd/agent-runtime-child-canary ./cmd/neo-runnerd)

echo 'Agent Runtime G21.4 activation contracts passed; checked-in evidence remains ISOLATION_UNAVAILABLE.'
