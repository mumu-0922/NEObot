#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
evaluator="${script_dir}/evaluate-agent-production-activation.py"
policy="${project_dir}/config/agent-runner/production-policy.json"
schema_dir="${project_dir}/docs/contracts/schemas"
fixture_dir="${project_dir}/docs/contracts/fixtures/agent-runtime"
activation_schema="${schema_dir}/neo-agent-product-canary-activation.schema.json"
plan_schema="${schema_dir}/neo-agent-product-canary-plan.schema.json"
held_fixture="${fixture_dir}/neo-agent-product-canary-activation.valid.json"
invalid_fixture="${fixture_dir}/neo-agent-product-canary-activation.invalid.json"
plan_fixture="${fixture_dir}/neo-agent-product-canary-plan.valid.json"
python_bin="${project_dir}/rag/.venv/bin/python"

for command in date go jq python3 sha256sum; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "G21.6 activation: ${command} is required" >&2
    exit 1
  }
done
for path in "${evaluator}" "${policy}" "${activation_schema}" "${plan_schema}" \
  "${held_fixture}" "${invalid_fixture}" "${plan_fixture}"; do
  [[ -s "${path}" ]] || { echo 'G21.6 activation: missing contract artifact' >&2; exit 1; }
done
if [[ ! -x "${python_bin}" ]]; then
  command -v uv >/dev/null 2>&1 || { echo 'G21.6 activation: uv is required' >&2; exit 1; }
  python_command=(uv run --project "${project_dir}/rag" --frozen --offline python)
else
  python_command=("${python_bin}")
fi

"${python_command[@]}" - "${activation_schema}" "${plan_schema}" "${held_fixture}" \
  "${invalid_fixture}" "${plan_fixture}" <<'PY'
import json,sys
from jsonschema import Draft202012Validator,FormatChecker
activation_schema,plan_schema,held,invalid,plan=(json.load(open(path,encoding="utf-8")) for path in sys.argv[1:])
for schema in (activation_schema,plan_schema):
    Draft202012Validator.check_schema(schema)
activation_validator=Draft202012Validator(activation_schema,format_checker=FormatChecker())
plan_validator=Draft202012Validator(plan_schema,format_checker=FormatChecker())
assert not list(activation_validator.iter_errors(held))
assert list(activation_validator.iter_errors(invalid))
assert not list(plan_validator.iter_errors(plan))
bad_plan=dict(plan); bad_plan["argv"]=["/bin/sh"]
assert list(plan_validator.iter_errors(bad_plan))
assert activation_schema["properties"]["release"]["properties"]["migrationHead"]["const"] == 95
assert activation_schema["properties"]["wiring"]["properties"]["callerIdentity"]["$ref"] == "#/$defs/identity"
assert plan["toolRegistry"]["tools"] == []
assert plan["toolRegistry"]["depth"] == 0
assert plan["sandbox"]["networkMode"] == "none"
assert plan["sandbox"]["rootfsReadOnly"] is True
assert plan["argv"] == ["/opt/neo/bin/product-canary", "--bounded-smoke"]
PY

work_dir="$(mktemp -d)"
cleanup(){ find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true; rmdir "${work_dir}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM
sha(){ printf 'sha256:%s' "$(sha256sum "$1" | awk '{print $1}')"; }
fingerprint(){ printf 'sha256:%s' "$(printf '%s' "$1" | sha256sum | awk '{print $1}')"; }
nonzero="$(fingerprint product-canary-nonzero)"

jq --arg fp "${nonzero}" '
  walk(if type=="string" and .==("sha256:"+("0"*64)) then $fp else . end) |
  .sandbox.image="registry.invalid/neo/product-canary@"+($fp|sub("^sha256:";"sha256:"))
' "${plan_fixture}" >"${work_dir}/plan.json"
printf '%s' 'synthetic-g21.6-client-certificate' >"${work_dir}/client.crt"
printf '%s' 'synthetic-g21.6-server-ca' >"${work_dir}/server-ca.crt"
printf '%s' 'synthetic-g21.6-ed25519-public-key' >"${work_dir}/authority-public-key"

cat >"${work_dir}/release-manifest.json" <<JSON
{
  "schemaVersion":"neo.agent-runner-release/v1",
  "approved":true,
  "runnerId":"neo-runner-primary",
  "runnerVersion":"g21.6-test",
  "protocolVersion":"neo.runner-rpc/v1",
  "binaries":[
    {"name":"podman","path":"/opt/neo/bin/podman","version":"test","sha256":"${nonzero}"},
    {"name":"crun","path":"/opt/neo/bin/crun","version":"test","sha256":"${nonzero}"},
    {"name":"conmon","path":"/opt/neo/bin/conmon","version":"test","sha256":"${nonzero}"},
    {"name":"newuidmap","path":"/opt/neo/bin/newuidmap","version":"test","sha256":"${nonzero}"},
    {"name":"newgidmap","path":"/opt/neo/bin/newgidmap","version":"test","sha256":"${nonzero}"}
  ],
  "storageDriver":"overlay",
  "networkMode":"none",
  "userNamespaceSize":65536,
  "requiredControllers":["cpu","memory","pids"],
  "seccompProfile":{"path":"/etc/neo-runner/seccomp.json","sha256":"${nonzero}"},
  "probeSuiteFingerprint":"${nonzero}",
  "isolationAcceptance":{"path":"/etc/neo-runner/isolation.json","sha256":"${nonzero}"}
}
JSON
chmod 600 "${work_dir}"/*

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
activation_id='activation_0123456789abcdef'

jq --arg commit "${commit}" --arg policy "$(sha "${policy}")" \
  --arg manifest "$(sha "${work_dir}/release-manifest.json")" --arg endpoint "${endpoint_sha}" \
  --arg client "$(sha "${work_dir}/client.crt")" --arg ca "$(sha "${work_dir}/server-ca.crt")" \
  --arg plan "$(sha "${work_dir}/plan.json")" --arg authority "$(sha "${work_dir}/authority-public-key")" \
  --arg activation_id "${activation_id}" --arg started "${started}" --arg completed "${completed}" \
  --arg observed "${observed}" --arg reviewed "${reviewed}" --arg expires "${expires}" \
  --arg nonzero "${nonzero}" \
  --arg control "$(fingerprint prerequisite-control)" --arg root "$(fingerprint prerequisite-root)" \
  --arg broker "$(fingerprint prerequisite-broker)" --arg project "$(fingerprint prerequisite-project)" \
  --arg child "$(fingerprint prerequisite-child)" --arg cron "$(fingerprint prerequisite-cron)" \
  --arg learning "$(fingerprint prerequisite-learning)" '
  .evidenceClass="production" |
  .release.gitCommit=$commit |
  .release.runnerManifestSha256=$manifest |
  .release.runnerBinarySha256=$nonzero |
  .release.operationsPolicySha256=$policy |
  .target.deploymentFingerprint=$nonzero |
  .wiring.activationId=$activation_id |
  .wiring.endpointSha256=$endpoint |
  .wiring.clientCertificateSha256=$client |
  .wiring.serverCASha256=$ca |
  .wiring.canaryPlanSha256=$plan |
  .wiring.authorityPublicKeySha256=$authority |
  .prerequisites={
    controlPlane:$control,rootRun:$root,
    brokerArtifact:$broker,projectMutation:$project,
    depthOneChild:$child,cronWorker:$cron,
    draftLearning:$learning
  } |
  .window={startedAt:$started,completedAt:$completed,expiresAt:$expires} |
  .checks |= map(.result="passed" | .observedAt=$observed | .evidenceSha256=$nonzero | .detailCode="PASS") |
  .review={decision:"approved",reviewedAt:$reviewed,reviewerFingerprint:$nonzero}
' "${held_fixture}" >"${work_dir}/ready.json"
chmod 600 "${work_dir}/ready.json"

base_args=(
  --policy "${policy}" --release-manifest "${work_dir}/release-manifest.json"
  --client-certificate "${work_dir}/client.crt" --server-ca "${work_dir}/server-ca.crt"
  --canary-plan "${work_dir}/plan.json" --authority-public-key "${work_dir}/authority-public-key"
  --endpoint "${endpoint}" --runner-id neo-runner-primary --server-name neo-runner.internal
  --caller-identity spiffe://neo-chat/agent-runtime-product-canary --release-commit "${commit}"
  --activation-id "${activation_id}"
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
run_case 0 ACTIVATION_READY PRODUCT_CANARY_GATES_PASSED "${work_dir}/ready.json"

jq '.authorization.genericRuntime=true' "${work_dir}/ready.json" >"${work_dir}/widened.json"
jq '.prerequisites.draftLearning=.prerequisites.cronWorker' "${work_dir}/ready.json" >"${work_dir}/duplicate.json"
chmod 600 "${work_dir}/widened.json" "${work_dir}/duplicate.json"
run_case 2 ACTIVATION_EVIDENCE_INVALID AUTHORIZATION_WIDENED "${work_dir}/widened.json"
run_case 2 ACTIVATION_EVIDENCE_INVALID PREREQUISITE_INVALID "${work_dir}/duplicate.json"

printf '%s' 'drifted-plan' >"${work_dir}/plan.json"
run_case 2 ACTIVATION_EVIDENCE_INVALID CANARY_PLAN_DRIFT "${work_dir}/ready.json"

set +e
held_output="$(python3 "${evaluator}" --record "${held_fixture}")"; held_status=$?
set -e
[[ "${held_status}" -eq 3 ]]
jq -e '.verdict=="ACTIVATION_HELD" and .reasonCode=="ISOLATION_UNAVAILABLE"' <<<"${held_output}" >/dev/null

(cd "${project_dir}/backend" && go test \
  ./internal/agentactivation ./internal/agentproductcanary \
  ./cmd/agent-runtime-product-canary ./cmd/neo-runnerd)

echo 'Agent Runtime G21.6 activation contracts passed; checked-in evidence remains ISOLATION_UNAVAILABLE.'
