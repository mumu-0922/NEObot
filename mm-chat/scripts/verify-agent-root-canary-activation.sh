#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
evaluator="${script_dir}/evaluate-agent-production-activation.py"
policy="${project_dir}/config/agent-runner/production-policy.json"
activation_schema="${project_dir}/docs/contracts/schemas/neo-agent-root-run-canary-activation.schema.json"
plan_schema="${project_dir}/docs/contracts/schemas/neo-agent-root-run-canary-plan.schema.json"
held_fixture="${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-root-run-canary-activation.valid.json"
invalid_fixture="${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-root-run-canary-activation.invalid.json"
plan_fixture="${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-root-run-canary-plan.valid.json"
invalid_plan="${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-root-run-canary-plan.invalid.json"

for command in python3 jq sha256sum date; do
  command -v "${command}" >/dev/null 2>&1 || { printf 'Root canary activation: %s is required\n' "${command}" >&2; exit 1; }
done
for path in "${evaluator}" "${policy}" "${activation_schema}" "${plan_schema}" "${held_fixture}" "${invalid_fixture}" "${plan_fixture}" "${invalid_plan}"; do
  [[ -s "${path}" ]] || { printf '%s\n' 'Root canary activation: missing contract artifact' >&2; exit 1; }
done

python3 - "${activation_schema}" "${plan_schema}" "${held_fixture}" "${invalid_fixture}" "${plan_fixture}" "${invalid_plan}" <<'PY'
import json,sys
activation,plan,held,invalid,valid_plan,bad_plan=(json.load(open(path,encoding="utf-8")) for path in sys.argv[1:])
assert activation["$schema"].endswith("2020-12/schema") and activation["additionalProperties"] is False
assert activation["properties"]["stage"]["const"] == "root_run_canary"
assert activation["properties"]["authorization"]["properties"]["controlPlane"]["const"] is False
assert activation["properties"]["authorization"]["properties"]["rootRuns"]["const"] is True
assert held["stage"] == "root_run_canary" and len(held["checks"]) == 8
assert invalid.get("stage") != "root_run_canary"
assert plan["additionalProperties"] is False and valid_plan["toolRegistry"]["tools"] == []
assert bad_plan["toolRegistry"]["tools"] == ["delegate_task"]
PY

work_dir="$(mktemp -d)"
cleanup(){ find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true; rmdir "${work_dir}" 2>/dev/null || true; }
trap cleanup EXIT INT TERM
cp "${plan_fixture}" "${work_dir}/plan.json"
printf '%s' 'synthetic-client-certificate' >"${work_dir}/client.crt"
printf '%s' 'synthetic-server-ca' >"${work_dir}/server-ca.crt"
printf '%s' 'synthetic-ed25519-public-key-material' >"${work_dir}/authority-public-key"
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
observed="$(stamp "$((now_epoch-180))")"; reviewed="$(stamp "$((now_epoch-60))")"; expires="$(stamp "$((now_epoch+3600))")"
commit='1111111111111111111111111111111111111111'
nonzero='sha256:1111111111111111111111111111111111111111111111111111111111111111'
jq \
  --arg commit "${commit}" --arg policy "$(sha "${policy}")" --arg endpoint "${endpoint_sha}" \
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
  --caller-identity spiffe://neo-chat/agent-runtime-root-canary --release-commit "${commit}"
)
run_case(){
  local name="$1" status_expected="$2" verdict="$3" reason="$4" record="$5" status output
  set +e
  output="$(python3 "${evaluator}" --record "${record}" "${base_args[@]}")"; status=$?
  set -e
  [[ "${status}" -eq "${status_expected}" ]] || { printf '%s\n' "${output}" >&2; exit 1; }
  jq -e --arg verdict "${verdict}" --arg reason "${reason}" '.verdict==$verdict and .reasonCode==$reason' <<<"${output}" >/dev/null
}
run_case ready 0 ACTIVATION_READY ROOT_RUN_CANARY_GATES_PASSED "${work_dir}/ready.json"

run_missing_binding_case(){
  local omitted="$1" output status skip argument
  local -a args=()
  skip=false
  for argument in "${base_args[@]}"; do
    if [[ "${skip}" == true ]]; then
      skip=false
      continue
    fi
    if [[ "${argument}" == "${omitted}" ]]; then
      skip=true
      continue
    fi
    args+=("${argument}")
  done
  set +e
  output="$(python3 "${evaluator}" --record "${work_dir}/ready.json" "${args[@]}")"; status=$?
  set -e
  [[ "${status}" -eq 2 ]] || { printf '%s\n' "${output}" >&2; exit 1; }
  jq -e '.verdict=="ACTIVATION_EVIDENCE_INVALID" and .reasonCode=="WIRING_INVALID"' <<<"${output}" >/dev/null
}
run_missing_binding_case --canary-plan
run_missing_binding_case --authority-public-key

jq '.authorization.brokerReadOnly=true' "${work_dir}/ready.json" >"${work_dir}/widened.json"; chmod 600 "${work_dir}/widened.json"
run_case widened 2 ACTIVATION_EVIDENCE_INVALID AUTHORIZATION_WIDENED "${work_dir}/widened.json"
printf '%s' 'drifted-plan' >"${work_dir}/plan.json"
run_case plan-drift 2 ACTIVATION_EVIDENCE_INVALID CANARY_PLAN_DRIFT "${work_dir}/ready.json"
cp "${plan_fixture}" "${work_dir}/plan.json"; chmod 600 "${work_dir}/plan.json"

set +e
held_output="$(python3 "${evaluator}" --policy "${policy}" --record "${held_fixture}")"; held_status=$?
set -e
[[ "${held_status}" -eq 3 ]] && grep -Fq '"reasonCode":"ISOLATION_UNAVAILABLE"' <<<"${held_output}"

(
  # Runner filesystem fixtures intentionally include mode-0711 broker paths.
  umask 022
  cd "${project_dir}/backend"
  go test ./internal/agentactivation ./internal/agentrootcanary ./internal/agentrunner \
    ./cmd/agent-runtime-root-canary ./cmd/neo-runnerd
)
printf '%s\n' 'Agent Runtime G21.1 activation and Root canary contracts passed; checked-in evidence remains ISOLATION_UNAVAILABLE.'
