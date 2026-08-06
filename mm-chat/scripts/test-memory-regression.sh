#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
compose_file="${project_dir}/compose.memory-regression.yml"
runner_script="${script_dir}/run-memory-regression.sh"
docker_bin="${DOCKER_BIN:-docker}"

if ! "${docker_bin}" compose version >/dev/null 2>&1; then
  echo "Memory regression topology: Docker Compose is required" >&2
  exit 1
fi

temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/mm-chat-memory-regression-test.XXXXXX")"
trap 'find "${temp_dir}" -depth -delete' EXIT
chmod 700 "${temp_dir}"
fixture_root="${temp_dir}/regression-fixtures"
cost_file="${temp_dir}/cost.json"
credential_file="${temp_dir}/credential"
memory_tool_route_credential_file="${temp_dir}/memory-tool-route-credential"
configured_judge_credential_file="${temp_dir}/configured-judge-credential"
identical_judge_credential_file="${temp_dir}/identical-judge-credential"
render_output="${temp_dir}/render-output"
env_file="${temp_dir}/render.env"
rendered="${temp_dir}/compose.json"
mkdir -p "${fixture_root}" "${render_output}"
chmod 700 "${fixture_root}" "${render_output}"
printf '{"fixtures":[]}\n' >"${fixture_root}/fixtures.json"
printf '{"cases":[]}\n' >"${fixture_root}/corpus.json"
printf '{}\n' >"${fixture_root}/audit.json"
printf '{}\n' >"${fixture_root}/manifest.json"
printf '{"schemaVersion":"protocol-cost"}\n' >"${cost_file}"
printf 'fixture-live-credential-not-used\n' >"${credential_file}"
printf 'fixture-route-credential-not-used\n' >"${memory_tool_route_credential_file}"
printf 'fixture-judge-credential-not-used\n' >"${configured_judge_credential_file}"
cp "${credential_file}" "${identical_judge_credential_file}"
chmod 600 "${fixture_root}"/*.json "${cost_file}" "${credential_file}" \
  "${memory_tool_route_credential_file}" "${configured_judge_credential_file}" \
  "${identical_judge_credential_file}"

cat >"${env_file}" <<EOF
MEMORY_REGRESSION_DB_PASSWORD=fixture-render-password
MEMORY_REGRESSION_ROOT_PATH=${fixture_root}
MEMORY_REGRESSION_COST_BASIS_PATH=${cost_file}
MEMORY_REGRESSION_CREDENTIAL_PATH=${credential_file}
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_CREDENTIAL_PATH=${memory_tool_route_credential_file}
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_CREDENTIAL_TARGET=
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_CREDENTIAL_PATH=${configured_judge_credential_file}
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_CREDENTIAL_TARGET=
MEMORY_REGRESSION_OUTPUT_PATH=${render_output}
MEMORY_REGRESSION_RUN_ID=memory-regression-static
MEMORY_REGRESSION_CAPTURE_MODE=full_regression
MEMORY_REGRESSION_LIVE_APPROVAL=I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_PROVIDER_ID=
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_PROVIDER_TYPE=
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_BASE_URL=
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_BASE_URL_SHA256=
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_MODEL=
MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_APPROVAL=NOT_AUTHORIZED
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_PROVIDER_ID=
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_PROVIDER_TYPE=
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_BASE_URL=
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_BASE_URL_SHA256=
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_MODEL=
MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_APPROVAL=NOT_AUTHORIZED
MEMORY_REGRESSION_PRODUCTION_MEMORY_JUDGE_VALIDATION_APPROVAL=NOT_AUTHORIZED
EOF
chmod 600 "${env_file}"

"${docker_bin}" compose \
  --project-name mmchat-memory-regression-static \
  --project-directory "${project_dir}" \
  --env-file "${env_file}" \
  -f "${compose_file}" \
  --profile memory-regression-fake \
  --profile memory-regression-live \
  config --format json >"${rendered}"

python3 - "${rendered}" "${fixture_root}" "${cost_file}" "${credential_file}" \
  "${memory_tool_route_credential_file}" "${configured_judge_credential_file}" \
  "${render_output}" <<'PY'
import json
import sys
from pathlib import Path

config = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
fixture_root, cost, credential, route_credential, judge_credential, output = map(
    lambda value: Path(value).resolve(), sys.argv[2:8]
)
services = config.get("services", {})
expected = {
    "memory-regression-postgres",
    "memory-regression-migrate",
    "memory-regression-fake-runner",
    "memory-regression-live-runner",
}
if set(services) != expected:
    raise SystemExit(f"Memory regression topology: service drift: {sorted(services)}")

for name, service in services.items():
    if service.get("ports"):
        raise SystemExit(f"Memory regression topology: {name} publishes a host port")
    if not service.get("mem_limit") or not service.get("cpus") or not service.get("pids_limit"):
        raise SystemExit(f"Memory regression topology: {name} resource limit missing")
    if "no-new-privileges:true" not in service.get("security_opt", []):
        raise SystemExit(f"Memory regression topology: {name} no-new-privileges missing")

networks = config.get("networks", {})
if networks.get("memory-regression-private", {}).get("internal") is not True:
    raise SystemExit("Memory regression topology: private database network is not internal")
postgres = services["memory-regression-postgres"]
if set(postgres.get("networks", {})) != {"memory-regression-private"}:
    raise SystemExit("Memory regression topology: PostgreSQL network drift")
if postgres.get("image") != "mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5":
    raise SystemExit("Memory regression topology: PostgreSQL image drift")
if postgres.get("build", {}).get("context") != str((Path(sys.argv[1]).parent.parent / "postgres").resolve()):
    # Docker Compose may normalize to the actual product root; the target is
    # checked below by suffix to remain portable between Linux and WSL.
    if not str(postgres.get("build", {}).get("context", "")).replace("\\", "/").endswith("/mm-chat/postgres"):
        raise SystemExit("Memory regression topology: PostgreSQL build context drift")

migrate = services["memory-regression-migrate"]
if set(migrate.get("networks", {})) != {"memory-regression-private"}:
    raise SystemExit("Memory regression topology: migrate network drift")
if migrate.get("read_only") is not True or migrate.get("cap_drop") != ["ALL"]:
    raise SystemExit("Memory regression topology: migrate hardening drift")

def normalized_source(mount):
    return Path(str(mount["source"]).replace("\\", "/")).resolve()

for name in ("memory-regression-fake-runner", "memory-regression-live-runner"):
    runner = services[name]
    if runner.get("read_only") is not True or runner.get("cap_drop") != ["ALL"]:
        raise SystemExit(f"Memory regression topology: {name} hardening drift")
    if runner.get("build", {}).get("target") != "memory-regression-capture":
        raise SystemExit(f"Memory regression topology: {name} build target drift")
    environment = runner.get("environment", {})
    if "role=go_api_runtime" not in environment.get(
        "MM_CHAT_MEMORY_REGRESSION_RUNTIME_DATABASE_URL", ""
    ) or "role=go_api_runtime" in environment.get(
        "MM_CHAT_MEMORY_REGRESSION_ADMIN_DATABASE_URL", ""
    ):
        raise SystemExit(f"Memory regression topology: {name} runtime role boundary drift")
    mounts = {mount["target"]: mount for mount in runner.get("volumes", [])}
    expected_mounts = {
        "/fixtures/regression": fixture_root,
        "/inputs/cost-basis.json": cost,
        "/output": output,
    }
    if name == "memory-regression-live-runner":
        expected_mounts["/run/mm-chat-memory-regression/provider.key"] = credential
        expected_mounts["/run/mm-chat-memory-regression/memory-tool-route-provider.key"] = route_credential
        expected_mounts["/run/mm-chat-memory-regression/configured-candidate-judge-provider.key"] = judge_credential
    if set(mounts) != set(expected_mounts):
        raise SystemExit(f"Memory regression topology: {name} mount target drift")
    for target, source in expected_mounts.items():
        mount = mounts[target]
        if mount.get("type") != "bind" or normalized_source(mount) != source:
            raise SystemExit(f"Memory regression topology: {name} mount source drift")
        if target != "/output" and mount.get("read_only") is not True:
            raise SystemExit(f"Memory regression topology: {name} input mount is writable")

fake = services["memory-regression-fake-runner"]
if fake.get("profiles") != ["memory-regression-fake"]:
    raise SystemExit("Memory regression topology: fake profile drift")
if set(fake.get("networks", {})) != {"memory-regression-private"}:
    raise SystemExit("Memory regression topology: fake runner gained external network")
if any("credential-file" in str(value) for value in fake.get("command", [])):
    raise SystemExit("Memory regression topology: fake runner accepted credential argument")

live = services["memory-regression-live-runner"]
if live.get("profiles") != ["memory-regression-live"]:
    raise SystemExit("Memory regression topology: live profile drift")
if set(live.get("networks", {})) != {"memory-regression-private", "memory-regression-egress"}:
    raise SystemExit("Memory regression topology: live egress topology drift")
if "/run/mm-chat-memory-regression/provider.key" not in live.get("command", []):
    raise SystemExit("Memory regression topology: live credential file boundary missing")
if "-memory-tool-route-credential-file" not in live.get("command", []):
    raise SystemExit("Memory regression topology: Memory Tool-route credential boundary missing")
if "-configured-candidate-judge-credential-file" not in live.get("command", []):
    raise SystemExit("Memory regression topology: configured candidate-judge credential boundary missing")
if live.get("environment", {}).get(
    "MM_CHAT_MEMORY_REGRESSION_LIVE_PRODUCTION_MEMORY_JUDGE_VALIDATION_APPROVAL"
) != "NOT_AUTHORIZED":
    raise SystemExit("Memory regression topology: production Validation approval default drift")
for key, value in live.get("environment", {}).items():
    if "KEY" in key or "TOKEN" in key or "SECRET" in key:
        raise SystemExit("Memory regression topology: Provider credential entered Docker environment")
    if "fixture-live-credential-not-used" in str(value):
        raise SystemExit("Memory regression topology: Provider credential leaked into Docker metadata")
    if "fixture-route-credential-not-used" in str(value):
        raise SystemExit("Memory regression topology: Memory Tool-route credential leaked into Docker metadata")
    if "fixture-judge-credential-not-used" in str(value):
        raise SystemExit("Memory regression topology: configured candidate-judge credential leaked into Docker metadata")
PY

if rg -n '\.env\.single-server|mm-chat/(data|secrets|backup)|\.\./(data|secrets|backup)' "${compose_file}"; then
  echo "Memory regression topology: live runtime path referenced" >&2
  exit 1
fi
bash -n "${runner_script}"

fake_docker="${temp_dir}/fake-docker"
cat >"${fake_docker}" <<'PY'
#!/usr/bin/env python3
import hashlib
import json
import os
from pathlib import Path
import signal
import sys
import time

args = sys.argv[1:]
log_path = Path(os.environ.get("FAKE_DOCKER_LOG", "/dev/null"))
with log_path.open("a", encoding="utf-8") as log:
    log.write(" ".join(args) + "\n")

if args[:2] == ["compose", "version"]:
    raise SystemExit(0)
if args and args[0] == "compose":
    env_path = None
    if "--env-file" in args:
        env_path = Path(args[args.index("--env-file") + 1])
        with log_path.open("a", encoding="utf-8") as log:
            log.write(f"ENV_FILE={env_path}\n")
    if "config" in args or "build" in args or "up" in args or "down" in args:
        raise SystemExit(0)
    if "run" in args:
        service = next((value for value in args if value in {
            "memory-regression-migrate",
            "memory-regression-fake-runner",
            "memory-regression-live-runner",
        }), "")
        if service == "memory-regression-migrate":
            raise SystemExit(0)
        marker = os.environ.get("FAKE_RUNNER_MARKER")
        if marker:
            Path(marker).write_text("running\n", encoding="utf-8")
        sleep_seconds = float(os.environ.get("FAKE_RUNNER_SLEEP", "0"))
        if sleep_seconds:
            time.sleep(sleep_seconds)
        values = {}
        for line in env_path.read_text(encoding="utf-8").splitlines():
            key, value = line.split("=", 1)
            values[key] = value
        output = Path(values["MEMORY_REGRESSION_OUTPUT_PATH"])
        mode = "live_siliconflow" if "live" in service else "fake_protocol"
        capture_mode = values["MEMORY_REGRESSION_CAPTURE_MODE"]
        candidate_prefix = "native-v2-hybrid" if mode == "live_siliconflow" else "native-v2-hybrid-fake-protocol"
        candidate_profile = "native_v2_hybrid" if mode == "live_siliconflow" else "native_v2_hybrid_fake_protocol"
        publish = os.environ.get("FAKE_PUBLISH", "full")
        observation = json.dumps({
            "schemaVersion": "neo-chat.memory-benchmark-regression-observations.v1",
            "cases": [{} for _ in range(500)],
        }, separators=(",", ":")).encode() + b"\n"
        report = json.dumps({
            "schemaVersion": "neo-chat.memory-benchmark-regression-report.v1",
            "corpusClass": "machine_reviewed_regression",
            "admissionMode": "regression_only",
            "promotionEligible": False,
            "passed": True,
        }, separators=(",", ":")).encode() + b"\n"
        if capture_mode == "full_regression":
            bodies = {
                "native-v1-lexical.observations.json": observation,
                "native-v1-lexical.report.json": report,
                f"{candidate_prefix}.observations.json": observation,
                f"{candidate_prefix}.report.json": report,
            }
            manifest_schema = "neo-chat.memory-regression-native-run.v1"
            admission_mode = "regression_only"
        elif capture_mode == "development_calibration":
            def threshold_curve(minimum, maximum):
                count = maximum - minimum + 1
                return {
                    "minimumBasisPoints": minimum,
                    "maximumBasisPoints": maximum,
                    "stepBasisPoints": 1,
                    "relevantEligibleCaseCount": 240,
                    "relevantMissingCaseCount": 0,
                    "unrelatedNegativeEligibleCaseCount": 30,
                    "unrelatedNegativeMissingCaseCount": 0,
                    "relevantPassingCaseCounts": [240] * count,
                    "unrelatedNegativePassingCaseCounts": [0] * count,
                }
            calibration = json.dumps({
                "schemaVersion": "neo-chat.memory-regression-relevance-calibration.v3",
                "corpusClass": "machine_reviewed_regression",
                "admissionMode": "development_calibration_only",
                "promotionEligible": False,
                "split": "development",
                "caseCount": 300,
                "selected": {"providerSimilarityBasisPoints": 20, "finalRelevanceBasisPoints": 30},
                "intentSelectionAlgorithm": "zero-egress_max-recall_highest-intent-margin_v1",
                "intentEvaluatedThresholdCount": 201,
                "intentFeasibleThresholdCount": 1,
                "intentSelected": {"minimumMemoryIntentMarginBasisPoints": 10, "evaluation": {"passed": True}},
                "diagnostics": {
                    "version": "aggregate-threshold-curves-intent-and-attempts-v2",
                    "otherCaseCount": 30,
                    "failurePairCounts": {},
                    "intentFailureThresholdCounts": {},
                    "bestSafetyAttempt": {
                        "providerSimilarityBasisPoints": 20,
                        "finalRelevanceBasisPoints": 30,
                        "evaluation": {},
                    },
                    "bestRecallAttempt": {
                        "providerSimilarityBasisPoints": 20,
                        "finalRelevanceBasisPoints": 30,
                        "evaluation": {},
                    },
                    "bestIntentSafetyAttempt": {
                        "minimumMemoryIntentMarginBasisPoints": 10,
                        "evaluation": {},
                    },
                    "bestIntentRecallAttempt": {
                        "minimumMemoryIntentMarginBasisPoints": 10,
                        "evaluation": {},
                    },
                    "memoryIntentMarginCurve": threshold_curve(-100, 100),
                    "admissionSimilarityCurve": threshold_curve(-100, 100),
                    "maximumRerankScoreCurve": threshold_curve(0, 100),
                    "topTwoRerankMarginCurve": threshold_curve(0, 100),
                },
            }, separators=(",", ":")).encode() + b"\n"
            bodies = {"relevance-calibration.json": calibration}
            manifest_schema = "neo-chat.memory-regression-relevance-run.v1"
            admission_mode = "development_calibration_only"
        elif capture_mode == "development_cloud_judge":
            cloud = json.dumps({
                "schemaVersion": "neo-chat.memory-regression-relevance-calibration.v5",
                "corpusClass": "machine_reviewed_regression",
                "admissionMode": "development_cloud_judge_only",
                "promotionEligible": False,
                "split": "development",
                "caseCount": 300,
                "providerEgressPolicy": "owner_authorized_normal_candidates_v1",
                "providerCostPolicy": "owner_authorized_absolute_cap_v1",
                "providerCostAuthorized": True,
                "judgeModelId": values["MEMORY_REGRESSION_CLOUD_JUDGE_MODEL"],
                "passed": True,
                "evaluation": {"passed": True, "providerCostRatio": 0.487716},
                "diagnostics": {
                    "emptyCandidateCaseCount": 105,
                    "judgeCompletedCaseCount": 195,
                    "judgeAbstainedCaseCount": 30,
                    "failedCaseCount": 0,
                    "failureCodeCounts": {},
                },
                "costAuthority": {
                    "unit": "cny_microunits",
                    "authorizedRequestCount": 300,
                    "actualRequestCount": 195,
                    "authorizedMaximumInputTokens": 300000,
                    "actualInputTokenUpperBound": 258647,
                    "authorizedMaximumOutputTokens": 38400,
                    "actualOutputTokenUpperBound": 24960,
                    "maximumJudgeCostMicrounits": 753600,
                    "maximumMemoryProviderCostMicrounits": 864516,
                },
            }, separators=(",", ":")).encode() + b"\n"
            bodies = {"cloud-judge-development.json": cloud}
            manifest_schema = "neo-chat.memory-regression-relevance-run.v1"
            admission_mode = "development_cloud_judge_only"
        elif capture_mode in {
            "development_fixed_memory_judge_accuracy",
            "development_fixed_memory_judge_failure_diagnostic",
            "development_fixed_memory_judge_transport_stable",
            "development_fixed_memory_judge_negative_guard",
            "development_fixed_memory_judge_negative_guard_buffered",
        }:
            negative_guard = capture_mode in {
                "development_fixed_memory_judge_negative_guard",
                "development_fixed_memory_judge_negative_guard_buffered",
            }
            buffered = capture_mode == "development_fixed_memory_judge_negative_guard_buffered"
            diagnostic = capture_mode in {
                "development_fixed_memory_judge_failure_diagnostic",
                "development_fixed_memory_judge_transport_stable",
                "development_fixed_memory_judge_negative_guard",
                "development_fixed_memory_judge_negative_guard_buffered",
            }
            transport_stable = capture_mode in {
                "development_fixed_memory_judge_transport_stable",
                "development_fixed_memory_judge_negative_guard",
                "development_fixed_memory_judge_negative_guard_buffered",
            }
            guarded_case_count = 10 if negative_guard else 0
            judge_completed_case_count = (
                185
                if negative_guard
                else 193
                if diagnostic and not transport_stable
                else 195
            )
            judge_attempt_count = (
                judge_completed_case_count + 2
                if transport_stable
                else judge_completed_case_count + 3
                if diagnostic
                else judge_completed_case_count + 1
            )
            judge_retry_count = 2 if transport_stable else 1
            judge_input_token_upper_bound = 245893 if negative_guard else 258893 if transport_stable else 258770
            cooldown_elapsed = 299000 if mode == "live_siliconflow" else 0
            cooldown_clock = "wall_clock_v1" if mode == "live_siliconflow" else "virtual_protocol_v1"
            def latency(count):
                return {
                    "sampleCount": count,
                    "totalMilliseconds": count,
                    "p95LatencyMilliseconds": 1 if count else 0,
                    "p99LatencyMilliseconds": 1 if count else 0,
                    "maximumLatencyMilliseconds": 1 if count else 0,
                }
            accuracy = json.dumps({
                "schemaVersion": (
                    "neo-chat.memory-regression-relevance-calibration.v17"
                    if buffered
                    else "neo-chat.memory-regression-relevance-calibration.v16"
                    if negative_guard
                    else "neo-chat.memory-regression-relevance-calibration.v14"
                    if transport_stable
                    else "neo-chat.memory-regression-relevance-calibration.v13"
                    if diagnostic
                    else "neo-chat.memory-regression-relevance-calibration.v12"
                ),
                "corpusClass": "machine_reviewed_regression",
                "admissionMode": (
                    "development_fixed_memory_judge_negative_guard_buffered_only"
                    if buffered
                    else "development_fixed_memory_judge_negative_guard_only"
                    if negative_guard
                    else "development_fixed_memory_judge_transport_stable_only"
                    if transport_stable
                    else "development_fixed_memory_judge_failure_diagnostic_only"
                    if diagnostic
                    else "development_fixed_memory_judge_accuracy_only"
                ),
                "promotionEligible": False,
                **({
                    "policySelected": False,
                    "diagnosticComplete": True,
                    "failureTaxonomyVersion": "memory-candidate-judge-failure-taxonomy-v1",
                    "failureTaxonomySha256": "c22cb137da8b5fda87526237446519dd9abe2c8d221ad703c5445358d9059f8d",
                    "diagnosticCompleteness": "attempt_terminal_reconciled_fail_closed_v1",
                } if diagnostic else {}),
                "split": "development",
                "caseCount": 300,
                "policyId": (
                    "memory_hybrid_fixed_cloud_candidate_judge_negative_guard_development_v1"
                    if negative_guard
                    else "memory_hybrid_fixed_cloud_candidate_judge_accuracy_development_v2"
                ),
                **({
                    "negativePolicyQueryGuardRequired": True,
                    "negativePolicyQueryGuardVersion": "memory-negative-policy-query-guard-v1",
                    "negativePolicyQueryGuardSha256": "8fe79b55a0f136392081a81e471abae98d0db7b8e3bece74adcc590b9d2c8f39",
                    "relevancePolicyDescriptorSha256": "82341542e46b091521b9f4b8c4eb637d6e732683d9902e0d2e3832a14cb50f9b",
                } if negative_guard else {}),
                "providerEgressPolicy": "owner_authorized_normal_candidates_v1",
                "providerCostPolicy": "owner_authorized_absolute_cap_v1",
                "providerCostAuthorized": True,
                "judgeProviderId": values["MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_PROVIDER_ID"],
                "judgeProviderType": values["MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_PROVIDER_TYPE"],
                "judgeBaseUrlSha256": values["MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_BASE_URL_SHA256"],
                "judgeModelId": values["MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_MODEL"],
                "judgeAdapter": (
                    "chat-configured-candidate-judge-buffered-v1"
                    if buffered
                    else "chat-configured-candidate-judge-v1"
                ),
                "evaluationCriteriaVersion": "neo-chat.memory-benchmark-criteria.v3",
                "evaluationCriteria": {
                    "minimumCandidateRecallAt20": 0.95,
                    "minimumFinalRecallAt5": 0.90,
                    "minimumCurrentFactAccuracy": 0.95,
                    "maximumFalseInjectionRate": 0.02,
                    "maximumAveragePromptMemoryTokens": 600,
                    "maximumPromptMemoryTokens": 900,
                    "maximumProviderCostRatio": 0.15,
                    "latencyEvaluationMode": "diagnostic_only_v1",
                    "applicationDeadlineMode": "none_v1",
                },
                "executionPolicy": {
                    "sequenceVersion": (
                        "bge_query_admission_bge_rerank_luna_judge_buffered_json_record_serial_judge_retry_v1"
                        if buffered
                        else "bge_query_admission_bge_rerank_luna_judge_record_serial_judge_retry_v2"
                        if transport_stable
                        else "bge_query_admission_bge_rerank_luna_judge_record_serial_v1"
                    ),
                    "globalProviderRequestConcurrency": 1,
                    "applicationDeadlineMode": "none_v1",
                    "providerElapsedTimeoutMode": "none_v1",
                    "latencyEvaluationMode": "diagnostic_only_v1",
                    "interCaseCooldownMilliseconds": 1000,
                    "interCaseCooldownClock": cooldown_clock,
                    "retryPolicyVersion": (
                        "transient_408_429_5xx_transport_read_judge_twice_v2"
                        if transport_stable
                        else "transient_408_429_5xx_transport_read_once_v1"
                    ),
                    "maximumRetriesPerProviderRequest": 1,
                    "retryFallbackDelayMilliseconds": 5000,
                    **({
                        "maximumJudgeRetriesPerRequest": 2,
                        "secondJudgeRetryDelayMilliseconds": 10000,
                    } if transport_stable else {}),
                },
                "passed": (not diagnostic) or transport_stable,
                "evaluation": {
                    "passed": (not diagnostic) or transport_stable,
                    "budgets": {
                        "p95LatencyMilliseconds": 120000,
                        "p99LatencyMilliseconds": 150000,
                        "averagePromptMemoryTokens": 100,
                        "maximumPromptMemoryTokens": 200,
                        "promptTokenPassed": True,
                    },
                    "slices": {},
                    "failures": [],
                },
                "diagnostics": {
                    "emptyCandidateCaseCount": 105,
                    "judgeCompletedCaseCount": judge_completed_case_count,
                    "judgeAbstainedCaseCount": 30,
                    **({
                        "negativePolicyQueryAbstainedCaseCount": guarded_case_count,
                    } if negative_guard else {}),
                    "failedCaseCount": 2 if diagnostic and not transport_stable else 0,
                    "failureCodeCounts": (
                        {"CANDIDATE_JUDGE_FAILED": 2}
                        if diagnostic and not transport_stable
                        else {}
                    ),
                    **({
                        "judgeTerminalFailureCategoryCounts": {} if transport_stable else {
                            "CANDIDATE_JUDGE_OUTPUT_JSON_INVALID": 1,
                            "PROVIDER_RATE_LIMITED": 1,
                        },
                    } if diagnostic else {}),
                },
                "providerAttempts": {
                    "passageEmbeddingAttempts": 1,
                    "passageEmbeddingRetries": 0,
                    "queryEmbeddingAttempts": 300,
                    "queryEmbeddingRetries": 0,
                    "rerankAttempts": judge_completed_case_count,
                    "rerankRetries": 0,
                    "judgeAttempts": judge_attempt_count,
                    "judgeRetries": judge_retry_count,
                    "judgeInputTokenUpperBound": judge_input_token_upper_bound,
                    "judgeRetryInputTokenUpperBound": 246 if transport_stable else 123,
                    "interCaseCooldownCount": 299,
                    "interCaseCooldownMilliseconds": 299000,
                    "interCaseCooldownElapsedMilliseconds": cooldown_elapsed,
                    "passageEmbeddingLatency": latency(1),
                    "queryEmbeddingLatency": latency(300),
                    "rerankLatency": latency(judge_completed_case_count),
                    "judgeLatency": latency(judge_attempt_count),
                    **({
                        "judgeAttemptFailureCategoryCounts": {
                            "PROVIDER_TRANSPORT_FAILED": 2,
                        } if transport_stable else {
                            "CANDIDATE_JUDGE_OUTPUT_JSON_INVALID": 1,
                            "PROVIDER_RATE_LIMITED": 2,
                        },
                    } if diagnostic else {}),
                },
                "costAuthority": {
                    "unit": "cny_microunits",
                    "authorizedRequestCount": 900 if transport_stable else 600,
                    "actualRequestCount": judge_attempt_count,
                    "authorizedMaximumInputTokens": 1500000 if transport_stable else 600000,
                    "actualInputTokenUpperBound": judge_input_token_upper_bound,
                    "authorizedMaximumOutputTokens": 115200 if transport_stable else 76800,
                    "actualOutputTokenUpperBound": judge_attempt_count * 128,
                    "maximumJudgeCostMicrounits": 565200 if transport_stable else 376800,
                    "maximumMemoryProviderCostMicrounits": 565200 if transport_stable else 487716,
                },
            }, separators=(",", ":")).encode() + b"\n"
            report_name = (
                "fixed-memory-judge-negative-guard-buffered-development.json"
                if buffered
                else "fixed-memory-judge-negative-guard-development.json"
                if negative_guard
                else "fixed-memory-judge-transport-stable-development.json"
                if transport_stable
                else "fixed-memory-judge-failure-diagnostic-development.json"
                if diagnostic
                else "fixed-memory-judge-accuracy-development.json"
            )
            bodies = {report_name: accuracy}
            manifest_schema = "neo-chat.memory-regression-relevance-run.v1"
            admission_mode = (
                "development_fixed_memory_judge_negative_guard_buffered_only"
                if buffered
                else "development_fixed_memory_judge_negative_guard_only"
                if negative_guard
                else "development_fixed_memory_judge_transport_stable_only"
                if transport_stable
                else "development_fixed_memory_judge_failure_diagnostic_only"
                if diagnostic
                else "development_fixed_memory_judge_accuracy_only"
            )
        elif capture_mode in {
            "development_configured_candidate_judge",
            "development_fixed_memory_judge",
        }:
            fixed_memory_judge = capture_mode == "development_fixed_memory_judge"
            configured_judge = json.dumps({
                "schemaVersion": (
                    "neo-chat.memory-regression-relevance-calibration.v11"
                    if fixed_memory_judge
                    else "neo-chat.memory-regression-relevance-calibration.v10"
                ),
                "corpusClass": "machine_reviewed_regression",
                "admissionMode": (
                    "development_fixed_memory_judge_only"
                    if fixed_memory_judge
                    else "development_configured_candidate_judge_only"
                ),
                "promotionEligible": False,
                "split": "development",
                "caseCount": 300,
                "providerEgressPolicy": "owner_authorized_normal_candidates_v1",
                "providerCostPolicy": "owner_authorized_absolute_cap_v1",
                "providerCostAuthorized": True,
                "judgeProviderId": values["MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_PROVIDER_ID"],
                "judgeProviderType": values["MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_PROVIDER_TYPE"],
                "judgeBaseUrlSha256": values["MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_BASE_URL_SHA256"],
                "judgeModelId": values["MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_MODEL"],
                "judgeAdapter": "chat-configured-candidate-judge-v1",
                **({
                    "policyId": "memory_hybrid_fixed_cloud_candidate_judge_development_v1",
                    "evaluationCriteriaVersion": "neo-chat.memory-benchmark-criteria.v2",
                    "evaluationCriteria": {
                        "maximumP95LatencyMilliseconds": 1500,
                        "maximumP99LatencyMilliseconds": 2500,
                        "hardCutoffMilliseconds": 3000,
                    },
                } if fixed_memory_judge else {}),
                "passed": True,
                "evaluation": {"passed": True, "providerCostRatio": 0.5},
                "diagnostics": {
                    "emptyCandidateCaseCount": 105,
                    "judgeCompletedCaseCount": 195,
                    "judgeAbstainedCaseCount": 30,
                    "failedCaseCount": 0,
                    "failureCodeCounts": {},
                },
                "costAuthority": {
                    "unit": "cny_microunits",
                    "authorizedRequestCount": 300,
                    "actualRequestCount": 195,
                    "authorizedMaximumInputTokens": 300000,
                    "actualInputTokenUpperBound": 258647,
                    "authorizedMaximumOutputTokens": 38400,
                    "actualOutputTokenUpperBound": 24960,
                    "maximumJudgeCostMicrounits": 376800,
                    "maximumMemoryProviderCostMicrounits": 487716,
                },
            }, separators=(",", ":")).encode() + b"\n"
            report_name = (
                "fixed-memory-judge-development.json"
                if fixed_memory_judge
                else "configured-candidate-judge-development.json"
            )
            bodies = {report_name: configured_judge}
            manifest_schema = "neo-chat.memory-regression-relevance-run.v1"
            admission_mode = (
                "development_fixed_memory_judge_only"
                if fixed_memory_judge
                else "development_configured_candidate_judge_only"
            )
        elif capture_mode in {
            "development_memory_tool_route",
            "development_memory_tool_route_diagnostic",
        }:
            diagnostic = capture_mode == "development_memory_tool_route_diagnostic"
            memory_tool_route = json.dumps({
                "schemaVersion": (
                    "neo-chat.memory-regression-relevance-calibration.v9"
                    if diagnostic
                    else "neo-chat.memory-regression-relevance-calibration.v7"
                ),
                "corpusClass": "machine_reviewed_regression",
                "admissionMode": (
                    "development_main_model_first_tool_round_route_failure_diagnostic_only"
                    if diagnostic
                    else "development_main_model_first_tool_round_only"
                ),
                "promotionEligible": False,
                "split": "development",
                "caseCount": 300,
                "policyId": "memory_hybrid_main_model_first_tool_round_calibration_v1",
                "profileId": candidate_profile,
                "configurationSha256": "a" * 64,
                "providerEgressPolicy": "owner_authorized_normal_candidates_v1",
                "providerCostPolicy": "owner_authorized_absolute_cap_v1",
                "providerCostAuthorized": True,
                "routeProviderId": values["MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_PROVIDER_ID"],
                "routeProviderType": values["MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_PROVIDER_TYPE"],
                "routeBaseUrlSha256": values["MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_BASE_URL_SHA256"],
                "routeModelId": values["MEMORY_REGRESSION_MEMORY_TOOL_ROUTE_MODEL"],
                "toolName": "search_memory",
                "toolContractVersion": "memory-search-tool-v1",
                "toolContractSha256": "f8f404df0ae3a3938081b813c8750d59ba252adbcb8dc755e075e5c738e20ca6",
                "toolAdapterVersion": "chat-first-tool-round-memory-decision-v1",
                "selectionAlgorithm": "first-tool-round-call_then-bge-order_top5-token-budget_v1",
                "passed": True,
                "evaluation": {"passed": True, "providerCostRatio": 0.5},
                "diagnostics": {
                    "emptyCandidateCaseCount": 105,
                    "routeCompletedCaseCount": 300,
                    "routeUsedCaseCount": 165,
                    "routeAbstainedCaseCount": 135,
                    "failedCaseCount": 0,
                    "failureCodeCounts": {},
                    **({"routeFailureCategoryCounts": {}} if diagnostic else {}),
                    **({
                        "retrievalIncompleteCaseCount": 0,
                        "retrievalFailureCodeCounts": (
                            {"RELEVANCE_ADMISSION_UNAVAILABLE": "invalid"}
                            if os.environ.get("FAKE_MALFORMED_RETRIEVAL_COUNTS") == "1"
                            else {}
                        ),
                    } if diagnostic else {}),
                },
                "costAuthority": {
                    "unit": "cny_microunits",
                    "authorizedRequestCount": 300,
                    "actualRequestCount": 300,
                    "authorizedMaximumInputTokens": 300000,
                    "actualInputTokenUpperBound": 280000,
                    "authorizedMaximumOutputTokens": 2457600,
                    "actualOutputTokenUpperBound": 19200,
                    "maximumRouteCostMicrounits": 400000,
                    "maximumMemoryProviderCostMicrounits": 500000,
                },
                **({
                    "failureTaxonomyVersion": "memory-tool-route-failure-taxonomy-v1",
                    "failureTaxonomySha256": "66f11e91edc0cf5a6a9dbf5dd30336e58a52860adee968fb4658d6ccd70d52a0",
                    "diagnosticCompleteness": "route_complete_retrieval_fail_closed_v1",
                } if diagnostic else {}),
            }, separators=(",", ":")).encode() + b"\n"
            report_name = (
                "memory-first-tool-round-route-diagnostic-development.json"
                if diagnostic
                else "memory-first-tool-round-development.json"
            )
            bodies = {report_name: memory_tool_route}
            manifest_schema = "neo-chat.memory-regression-relevance-run.v1"
            admission_mode = (
                "development_main_model_first_tool_round_route_failure_diagnostic_only"
                if diagnostic
                else "development_main_model_first_tool_round_only"
            )
        elif capture_mode == "production_fixed_memory_judge_validation":
            zero_safety = {
                "crossUserLeakCount": 0,
                "deletedMemoryLeakCount": 0,
                "secretLeakCount": 0,
                "untrustedSourceLeakCount": 0,
                "unauthorizedProviderEgressCount": 0,
                "passed": True,
            }
            validation_hashes = {
                "configurationSha256": "1" * 64,
                "validationCaseOrderSha256": "2" * 64,
                "corpusRawSha256": "3" * 64,
                "productionRelevancePolicySha256": "4" * 64,
                "memoryReadIntentPolicySha256": "538d9ccff34fb976cedfca0d9e153078cb3ce36f1baff0691f1d2124d182119c",
                "evaluationCriteriaSha256": "5" * 64,
            }
            def validation_latency(count):
                return {
                    "sampleCount": count,
                    "totalMilliseconds": count,
                    "p95LatencyMilliseconds": 1 if count else 0,
                    "p99LatencyMilliseconds": 1 if count else 0,
                    "maximumLatencyMilliseconds": 1 if count else 0,
                }
            validation_payload = {
                "schemaVersion": "neo-chat.memory-regression-relevance-validation.v15",
                "corpusClass": "machine_reviewed_regression",
                "admissionMode": "frozen_production_fixed_memory_judge_validation_only",
                "promotionEligible": False,
                "releaseEligible": False,
                "policySelected": False,
                "executionComplete": True,
                "evidenceClass": (
                    "live_validation"
                    if mode == "live_siliconflow"
                    else "fake_protocol_lifecycle_only"
                ),
                "split": "validation",
                "caseCount": 100,
                "policyId": "memory_hybrid_fixed_cloud_candidate_judge_production_v1",
                "profileId": candidate_profile,
                **validation_hashes,
                "memoryReadIntentPolicyVersion": "memory-explicit-read-intent-v1",
                "providerEgressPolicy": "owner_authorized_normal_candidates_v1",
                "providerCostPolicy": "owner_authorized_absolute_cap_v1",
                "providerCostAuthorized": True,
                "judgeProviderId": values["MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_PROVIDER_ID"],
                "judgeProviderType": values["MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_PROVIDER_TYPE"],
                "judgeBaseUrlSha256": values["MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_BASE_URL_SHA256"],
                "judgeModelId": values["MEMORY_REGRESSION_CONFIGURED_CANDIDATE_JUDGE_MODEL"],
                "judgeAdapter": "chat-configured-candidate-judge-v1",
                "judgePromptVersion": "memory-cloud-candidate-judge-prompt-v1",
                "judgePromptSha256": "c004e834f2db572fc8393f088f47750d420379664f972357f987a09d8647f9c8",
                "judgeDecodingProfile": "temperature-0_max-output-128_no-thinking_v1",
                "failureTaxonomyVersion": "memory-candidate-judge-failure-taxonomy-v1",
                "failureTaxonomySha256": "c22cb137da8b5fda87526237446519dd9abe2c8d221ad703c5445358d9059f8d",
                "diagnosticCompleteness": "attempt_terminal_reconciled_fail_closed_v1",
                "selectionAlgorithm": "strict-ordinal_intersect-bge-order_top5-token-budget_v1",
                "evaluationCriteriaVersion": "neo-chat.memory-benchmark-criteria.v3",
                "evaluationCriteria": {
                    "minimumCandidateRecallAt20": 0.95,
                    "minimumFinalRecallAt5": 0.90,
                    "minimumCurrentFactAccuracy": 0.95,
                    "maximumFalseInjectionRate": 0.02,
                    "maximumAveragePromptMemoryTokens": 600,
                    "maximumPromptMemoryTokens": 900,
                    "maximumProviderCostRatio": 0.15,
                    "latencyEvaluationMode": "diagnostic_only_v1",
                    "applicationDeadlineMode": "none_v1",
                },
                "executionPolicy": {
                    "sequenceVersion": "production_bge_m3_rerank_fixed_luna_judge_record_serial_v1",
                    "globalProviderRequestConcurrency": 1,
                    "applicationDeadlineMode": "none_v1",
                    "providerElapsedTimeoutMode": "none_v1",
                    "latencyEvaluationMode": "diagnostic_only_v1",
                    "interCaseCooldownMilliseconds": 1000,
                    "interCaseCooldownClock": (
                        "wall_clock_v1"
                        if mode == "live_siliconflow"
                        else "virtual_protocol_v1"
                    ),
                    "retryPolicyVersion": "transient_408_429_5xx_transport_read_judge_twice_v2",
                    "maximumRetriesPerProviderRequest": 1,
                    "retryFallbackDelayMilliseconds": 5000,
                    "maximumJudgeRetriesPerRequest": 2,
                    "secondJudgeRetryDelayMilliseconds": 10000,
                },
                "passed": mode == "live_siliconflow",
                "outcome": (
                    {
                        "severity": "none",
                        "requiredAction": "owner_review_no_automatic_release",
                        "reasons": [],
                    }
                    if mode == "live_siliconflow"
                    else {
                        "severity": "yellow",
                        "requiredAction": "retain_beta",
                        "reasons": ["FAKE_PROTOCOL_NON_EVIDENCE"],
                    }
                ),
                "evaluation": {
                    "passed": True,
                    "metrics": {
                        "candidateRecallAt20": 1.0,
                        "finalRecallAt5": 1.0,
                        "currentFactAccuracy": 1.0,
                        "falseInjectionRate": 0.0,
                        "relevantCaseCount": 65,
                        "negativeCaseCount": 35,
                        "currentFactCaseCount": 33,
                        "falseInjectionCases": 0,
                    },
                    "rankingDiagnostics": {"ndcgAt5": 1.0, "mrrAt5": 1.0},
                    "budgets": {
                        "p95LatencyMilliseconds": 1,
                        "p99LatencyMilliseconds": 1,
                        "averagePromptMemoryTokens": 100,
                        "maximumPromptMemoryTokens": 200,
                        "promptTokenPassed": True,
                    },
                    "safety": zero_safety,
                    "slices": {},
                    "failures": [],
                },
                "diagnostics": {
                    "emptyCandidateCaseCount": 35,
                    "judgeCompletedCaseCount": 65,
                    "judgeAbstainedCaseCount": 10,
                    "failedCaseCount": 0,
                    "failureCodeCounts": {},
                    "judgeTerminalFailureCategoryCounts": {},
                },
                "providerAttempts": {
                    "passageEmbeddingAttempts": 1,
                    "passageEmbeddingRetries": 0,
                    "queryEmbeddingAttempts": 100,
                    "queryEmbeddingRetries": 0,
                    "rerankAttempts": 65,
                    "rerankRetries": 0,
                    "judgeAttempts": 65,
                    "judgeRetries": 0,
                    "judgeInputTokenUpperBound": 65000,
                    "judgeRetryInputTokenUpperBound": 0,
                    "interCaseCooldownCount": 99,
                    "interCaseCooldownMilliseconds": 99000,
                    "interCaseCooldownElapsedMilliseconds": (
                        99000 if mode == "live_siliconflow" else 0
                    ),
                    "passageEmbeddingLatency": validation_latency(1),
                    "queryEmbeddingLatency": validation_latency(100),
                    "rerankLatency": validation_latency(65),
                    "judgeLatency": validation_latency(65),
                    "judgeAttemptFailureCategoryCounts": {},
                },
                "costAuthority": {
                    "unit": "cny_microunits",
                    "authorizedRequestCount": 300,
                    "actualRequestCount": 65,
                    "authorizedMaximumInputTokens": 300000,
                    "actualInputTokenUpperBound": 65000,
                    "authorizedMaximumOutputTokens": 38400,
                    "actualOutputTokenUpperBound": 8320,
                    "maximumJudgeCostMicrounits": 2,
                    "maximumMemoryProviderCostMicrounits": 50,
                },
            }
            if os.environ.get("FAKE_PRODUCTION_FORBIDDEN") == "1":
                validation_payload["query"] = "forbidden-case-level-payload"
            validation = json.dumps(
                validation_payload, separators=(",", ":")
            ).encode() + b"\n"
            bodies = {"fixed-memory-judge-production-validation.json": validation}
            manifest_schema = "neo-chat.memory-regression-relevance-validation-run.v15"
            admission_mode = "frozen_production_fixed_memory_judge_validation_only"
        elif capture_mode == "frozen_validation":
            validation = json.dumps({
                "schemaVersion": "neo-chat.memory-regression-relevance-validation.v1",
                "corpusClass": "machine_reviewed_regression",
                "admissionMode": "frozen_validation_only",
                "promotionEligible": False,
                "split": "validation",
                "caseCount": 100,
                "passed": True,
            }, separators=(",", ":")).encode() + b"\n"
            bodies = {"relevance-validation.json": validation}
            manifest_schema = "neo-chat.memory-regression-relevance-run.v1"
            admission_mode = "frozen_validation_only"
        else:
            raise SystemExit(2)
        if publish == "partial":
            name = "native-v1-lexical.observations.json"
            (output / name).write_bytes(bodies[name])
            (output / name).chmod(0o600)
        elif publish == "full":
            for name, body in bodies.items():
                (output / name).write_bytes(body)
                (output / name).chmod(0o600)
            manifest = {
                "schemaVersion": manifest_schema,
                "runId": values["MEMORY_REGRESSION_RUN_ID"],
                "corpusClass": "machine_reviewed_regression",
                "admissionMode": admission_mode,
                "promotionEligible": False,
                "providerMode": mode,
                "artifacts": [
                    {"name": name, "bytes": len(body), "sha256": hashlib.sha256(body).hexdigest()}
                    for name, body in sorted(bodies.items())
                ],
            }
            if capture_mode == "full_regression":
                manifest["profiles"] = [
                    {"role": "baseline", "profileId": "native_v1_lexical", "passed": True},
                    {"role": "candidate", "profileId": candidate_profile, "passed": True},
                ]
            else:
                manifest.update({
                    "captureMode": capture_mode,
                    "split": "development" if capture_mode in {
                        "development_calibration",
                        "development_cloud_judge",
                        "development_memory_tool_route",
                        "development_memory_tool_route_diagnostic",
                        "development_configured_candidate_judge",
                        "development_fixed_memory_judge",
                        "development_fixed_memory_judge_accuracy",
                        "development_fixed_memory_judge_failure_diagnostic",
                        "development_fixed_memory_judge_transport_stable",
                        "development_fixed_memory_judge_negative_guard",
                        "development_fixed_memory_judge_negative_guard_buffered",
                    } else "validation",
                    "profileId": candidate_profile,
                })
                if capture_mode in {
                    "development_cloud_judge",
                    "development_memory_tool_route",
                    "development_memory_tool_route_diagnostic",
                    "development_configured_candidate_judge",
                    "development_fixed_memory_judge",
                    "development_fixed_memory_judge_accuracy",
                    "development_fixed_memory_judge_failure_diagnostic",
                    "development_fixed_memory_judge_transport_stable",
                    "development_fixed_memory_judge_negative_guard",
                    "development_fixed_memory_judge_negative_guard_buffered",
                    "production_fixed_memory_judge_validation",
                }:
                    manifest["providerCostPolicy"] = "owner_authorized_absolute_cap_v1"
                if capture_mode == "development_fixed_memory_judge_failure_diagnostic":
                    manifest["passed"] = False
                elif capture_mode == "development_fixed_memory_judge_transport_stable":
                    manifest["passed"] = True
                elif capture_mode in {
                    "development_fixed_memory_judge_negative_guard",
                    "development_fixed_memory_judge_negative_guard_buffered",
                }:
                    manifest.update({
                        "passed": True,
                        "negativePolicyQueryGuardVersion": "memory-negative-policy-query-guard-v1",
                        "negativePolicyQueryGuardSha256": "8fe79b55a0f136392081a81e471abae98d0db7b8e3bece74adcc590b9d2c8f39",
                        "relevancePolicyDescriptorSha256": "82341542e46b091521b9f4b8c4eb637d6e732683d9902e0d2e3832a14cb50f9b",
                    })
                elif capture_mode == "production_fixed_memory_judge_validation":
                    validation_outcome = (
                        {
                            "severity": "none",
                            "requiredAction": "owner_review_no_automatic_release",
                            "reasons": [],
                        }
                        if mode == "live_siliconflow"
                        else {
                            "severity": "yellow",
                            "requiredAction": "retain_beta",
                            "reasons": ["FAKE_PROTOCOL_NON_EVIDENCE"],
                        }
                    )
                    manifest.update({
                        "captureId": "00000000-0000-4000-8000-000000000001",
                        "releaseEligible": False,
                        "evidenceClass": (
                            "live_validation"
                            if mode == "live_siliconflow"
                            else "fake_protocol_lifecycle_only"
                        ),
                        "policyId": "memory_hybrid_fixed_cloud_candidate_judge_production_v1",
                        "configurationSha256": validation_hashes["configurationSha256"],
                        "validationCaseOrderSha256": validation_hashes["validationCaseOrderSha256"],
                        "productionRelevancePolicySha256": validation_hashes["productionRelevancePolicySha256"],
                        "memoryReadIntentPolicySha256": validation_hashes["memoryReadIntentPolicySha256"],
                        "evaluationCriteriaSha256": validation_hashes["evaluationCriteriaSha256"],
                        "passed": mode == "live_siliconflow",
                        "outcome": validation_outcome,
                        "startedAt": "2026-08-05T00:00:00Z",
                        "completedAt": "2026-08-05T00:01:39Z",
                        "costBasisSha256": "6" * 64,
                        "inputs": {
                            "fixtureRawSha256": "7" * 64,
                            "corpusRawSha256": validation_hashes["corpusRawSha256"],
                            "auditRawSha256": "8" * 64,
                            "manifestRawSha256": "9" * 64,
                        },
                    })
            manifest_path = output / "run-manifest.json"
            manifest_path.write_text(json.dumps(manifest, separators=(",", ":")) + "\n", encoding="utf-8")
            manifest_path.chmod(0o600)
        print(json.dumps({"schemaVersion": "fake-docker-summary", "providerMode": mode}))
        raise SystemExit(int(os.environ.get("FAKE_RUNNER_STATUS", "0")))
if args[:2] == ["ps", "-aq"] or args[:3] == ["network", "ls", "-q"] or args[:3] == ["volume", "ls", "-q"]:
    raise SystemExit(0)
if args and args[0] == "inspect":
    print("[]")
    raise SystemExit(0)
raise SystemExit(0)
PY
chmod 700 "${fake_docker}"

assert_cleanup() {
  local log_file="$1"
  if ! grep -q 'compose .* down --volumes --remove-orphans' "${log_file}"; then
    echo "Memory regression protocol: teardown command missing" >&2
    exit 1
  fi
  local env_path
  env_path="$(sed -n 's/^ENV_FILE=//p' "${log_file}" | tail -n 1)"
  if [[ -n "${env_path}" && -e "$(dirname "${env_path}")" ]]; then
    echo "Memory regression protocol: temporary credential directory survived" >&2
    exit 1
  fi
}

success_output="${temp_dir}/success-output"
success_log="${temp_dir}/success-docker.log"
mkdir "${success_output}"
chmod 700 "${success_output}"
FAKE_DOCKER_LOG="${success_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${success_output}" --provider-mode fake_protocol \
  >"${temp_dir}/success.stdout" 2>"${temp_dir}/success.stderr"
if [[ "$(find "${success_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 5 ]]; then
  echo "Memory regression protocol: success bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${success_log}"

calibration_output="${temp_dir}/calibration-output"
calibration_log="${temp_dir}/calibration-docker.log"
mkdir "${calibration_output}"
chmod 700 "${calibration_output}"
FAKE_DOCKER_LOG="${calibration_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${calibration_output}" --provider-mode fake_protocol \
  --capture-mode development_calibration \
  >"${temp_dir}/calibration.stdout" 2>"${temp_dir}/calibration.stderr"
if [[ "$(find "${calibration_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: calibration bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${calibration_log}"

cloud_output="${temp_dir}/cloud-output"
cloud_log="${temp_dir}/cloud-docker.log"
mkdir "${cloud_output}"
chmod 700 "${cloud_output}"
FAKE_DOCKER_LOG="${cloud_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${cloud_output}" --provider-mode fake_protocol \
  --capture-mode development_cloud_judge \
  --cloud-judge-model deepseek-ai/DeepSeek-V4-Flash \
  >"${temp_dir}/cloud.stdout" 2>"${temp_dir}/cloud.stderr"
if [[ "$(find "${cloud_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: cloud-judge bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${cloud_log}"

configured_judge_output="${temp_dir}/configured-judge-output"
configured_judge_log="${temp_dir}/configured-judge-docker.log"
mkdir "${configured_judge_output}"
chmod 700 "${configured_judge_output}"
FAKE_DOCKER_LOG="${configured_judge_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${configured_judge_output}" --provider-mode fake_protocol \
  --capture-mode development_configured_candidate_judge \
  --configured-candidate-judge-provider-id configured-gpt \
  --configured-candidate-judge-provider-type openai \
  --configured-candidate-judge-base-url https://api.openai.example/v1 \
  --configured-candidate-judge-model gpt-test \
  >"${temp_dir}/configured-judge.stdout" \
  2>"${temp_dir}/configured-judge.stderr"
if [[ "$(find "${configured_judge_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: configured candidate-judge bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${configured_judge_log}"

fixed_judge_output="${temp_dir}/fixed-judge-output"
fixed_judge_log="${temp_dir}/fixed-judge-docker.log"
mkdir "${fixed_judge_output}"
chmod 700 "${fixed_judge_output}"
FAKE_DOCKER_LOG="${fixed_judge_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${fixed_judge_output}" --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  >"${temp_dir}/fixed-judge.stdout" \
  2>"${temp_dir}/fixed-judge.stderr"
if [[ "$(find "${fixed_judge_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: fixed Memory Judge bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${fixed_judge_log}"

accuracy_first_output="${temp_dir}/accuracy-first-output"
accuracy_first_log="${temp_dir}/accuracy-first-docker.log"
mkdir "${accuracy_first_output}"
chmod 700 "${accuracy_first_output}"
FAKE_DOCKER_LOG="${accuracy_first_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${accuracy_first_output}" --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_accuracy \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  >"${temp_dir}/accuracy-first.stdout" \
  2>"${temp_dir}/accuracy-first.stderr"
if [[ "$(find "${accuracy_first_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: accuracy-first Memory Judge bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${accuracy_first_log}"

judge_failure_diagnostic_output="${temp_dir}/judge-failure-diagnostic-output"
judge_failure_diagnostic_log="${temp_dir}/judge-failure-diagnostic-docker.log"
mkdir "${judge_failure_diagnostic_output}"
chmod 700 "${judge_failure_diagnostic_output}"
FAKE_DOCKER_LOG="${judge_failure_diagnostic_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${judge_failure_diagnostic_output}" --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_failure_diagnostic \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  >"${temp_dir}/judge-failure-diagnostic.stdout" \
  2>"${temp_dir}/judge-failure-diagnostic.stderr"
if [[ "$(find "${judge_failure_diagnostic_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: Judge failure diagnostic bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${judge_failure_diagnostic_log}"

transport_stable_output="${temp_dir}/transport-stable-output"
transport_stable_log="${temp_dir}/transport-stable-docker.log"
mkdir "${transport_stable_output}"
chmod 700 "${transport_stable_output}"
FAKE_DOCKER_LOG="${transport_stable_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${transport_stable_output}" --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_transport_stable \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  >"${temp_dir}/transport-stable.stdout" \
  2>"${temp_dir}/transport-stable.stderr"
if [[ "$(find "${transport_stable_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: transport-stable Memory Judge bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${transport_stable_log}"

negative_guard_output="${temp_dir}/negative-guard-output"
negative_guard_log="${temp_dir}/negative-guard-docker.log"
mkdir "${negative_guard_output}"
chmod 700 "${negative_guard_output}"
FAKE_DOCKER_LOG="${negative_guard_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${negative_guard_output}" --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_negative_guard \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  >"${temp_dir}/negative-guard.stdout" \
  2>"${temp_dir}/negative-guard.stderr"
if [[ "$(find "${negative_guard_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 || \
  "$(find "${negative_guard_output}" -mindepth 2 -maxdepth 2 -type f ! -perm 0600 | wc -l)" -ne 0 ]]; then
  echo "Memory regression protocol: negative-guard Memory Judge bundle was not retained privately" >&2
  exit 1
fi
assert_cleanup "${negative_guard_log}"

buffered_output="${temp_dir}/buffered-output"
buffered_log="${temp_dir}/buffered-docker.log"
mkdir "${buffered_output}"
chmod 700 "${buffered_output}"
FAKE_DOCKER_LOG="${buffered_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${buffered_output}" --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge_negative_guard_buffered \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  >"${temp_dir}/buffered.stdout" \
  2>"${temp_dir}/buffered.stderr"
if [[ "$(find "${buffered_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 || \
  "$(find "${buffered_output}" -mindepth 2 -maxdepth 2 -type f ! -perm 0600 | wc -l)" -ne 0 ]]; then
  echo "Memory regression protocol: buffered Memory Judge bundle was not retained privately" >&2
  exit 1
fi
assert_cleanup "${buffered_log}"

production_validation_output="${temp_dir}/production-validation-output"
production_validation_log="${temp_dir}/production-validation-docker.log"
mkdir "${production_validation_output}"
chmod 700 "${production_validation_output}"
set +e
FAKE_DOCKER_LOG="${production_validation_log}" FAKE_RUNNER_STATUS=7 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${production_validation_output}" --provider-mode fake_protocol \
  --capture-mode production_fixed_memory_judge_validation \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  >"${temp_dir}/production-validation.stdout" \
  2>"${temp_dir}/production-validation.stderr"
production_validation_status=$?
set -e
if [[ ${production_validation_status} -eq 0 || \
  "$(find "${production_validation_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 || \
  "$(find "${production_validation_output}" -mindepth 2 -maxdepth 2 -type f ! -perm 0600 | wc -l)" -ne 0 ]]; then
  echo "Memory regression protocol: Fake production Validation did not retain its failed aggregate bundle" >&2
  exit 1
fi
assert_cleanup "${production_validation_log}"

forbidden_validation_output="${temp_dir}/forbidden-production-validation-output"
forbidden_validation_log="${temp_dir}/forbidden-production-validation-docker.log"
mkdir "${forbidden_validation_output}"
chmod 700 "${forbidden_validation_output}"
set +e
FAKE_DOCKER_LOG="${forbidden_validation_log}" FAKE_RUNNER_STATUS=7 \
  FAKE_PUBLISH=full FAKE_PRODUCTION_FORBIDDEN=1 \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${forbidden_validation_output}" --provider-mode fake_protocol \
  --capture-mode production_fixed_memory_judge_validation \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  >"${temp_dir}/forbidden-production-validation.stdout" \
  2>"${temp_dir}/forbidden-production-validation.stderr"
forbidden_validation_status=$?
set -e
if [[ ${forbidden_validation_status} -eq 0 || \
  -n "$(find "${forbidden_validation_output}" -mindepth 1 -print -quit)" ]]; then
  echo "Memory regression protocol: production Validation retained forbidden payload" >&2
  exit 1
fi
assert_cleanup "${forbidden_validation_log}"

fixed_drift_output="${temp_dir}/fixed-drift-output"
mkdir "${fixed_drift_output}"
chmod 700 "${fixed_drift_output}"
set +e
DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${fixed_drift_output}" --provider-mode fake_protocol \
  --capture-mode development_fixed_memory_judge \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model other \
  >"${temp_dir}/fixed-drift.stdout" \
  2>"${temp_dir}/fixed-drift.stderr"
fixed_drift_status=$?
set -e
if [[ ${fixed_drift_status} -eq 0 || \
  -n "$(find "${fixed_drift_output}" -mindepth 1 -print -quit)" ]]; then
  echo "Memory regression protocol: fixed Memory Judge authority drift did not fail closed" >&2
  exit 1
fi

memory_tool_route_output="${temp_dir}/memory-tool-route-output"
memory_tool_route_log="${temp_dir}/memory-tool-route-docker.log"
mkdir "${memory_tool_route_output}"
chmod 700 "${memory_tool_route_output}"
FAKE_DOCKER_LOG="${memory_tool_route_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${memory_tool_route_output}" --provider-mode fake_protocol \
  --capture-mode development_memory_tool_route \
  --memory-tool-route-provider-id configured-deepseek \
  --memory-tool-route-provider-type openai_compatible \
  --memory-tool-route-base-url https://api.deepseek.example/ \
  --memory-tool-route-model deepseek-chat \
  >"${temp_dir}/memory-tool-route.stdout" \
  2>"${temp_dir}/memory-tool-route.stderr"
if [[ "$(find "${memory_tool_route_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: Memory Tool-route bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${memory_tool_route_log}"

memory_tool_route_diagnostic_output="${temp_dir}/memory-tool-route-diagnostic-output"
memory_tool_route_diagnostic_log="${temp_dir}/memory-tool-route-diagnostic-docker.log"
mkdir "${memory_tool_route_diagnostic_output}"
chmod 700 "${memory_tool_route_diagnostic_output}"
FAKE_DOCKER_LOG="${memory_tool_route_diagnostic_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${memory_tool_route_diagnostic_output}" --provider-mode fake_protocol \
  --capture-mode development_memory_tool_route_diagnostic \
  --memory-tool-route-provider-id configured-deepseek \
  --memory-tool-route-provider-type openai_compatible \
  --memory-tool-route-base-url https://api.deepseek.example/ \
  --memory-tool-route-model deepseek-chat \
  >"${temp_dir}/memory-tool-route-diagnostic.stdout" \
  2>"${temp_dir}/memory-tool-route-diagnostic.stderr"
if [[ "$(find "${memory_tool_route_diagnostic_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: Memory Tool-route diagnostic bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${memory_tool_route_diagnostic_log}"

malformed_diagnostic_output="${temp_dir}/malformed-memory-tool-route-diagnostic-output"
malformed_diagnostic_log="${temp_dir}/malformed-memory-tool-route-diagnostic-docker.log"
mkdir "${malformed_diagnostic_output}"
chmod 700 "${malformed_diagnostic_output}"
set +e
FAKE_DOCKER_LOG="${malformed_diagnostic_log}" \
  FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full FAKE_MALFORMED_RETRIEVAL_COUNTS=1 \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${malformed_diagnostic_output}" --provider-mode fake_protocol \
  --capture-mode development_memory_tool_route_diagnostic \
  --memory-tool-route-provider-id configured-deepseek \
  --memory-tool-route-provider-type openai_compatible \
  --memory-tool-route-base-url https://api.deepseek.example/ \
  --memory-tool-route-model deepseek-chat \
  >"${temp_dir}/malformed-memory-tool-route-diagnostic.stdout" \
  2>"${temp_dir}/malformed-memory-tool-route-diagnostic.stderr"
malformed_diagnostic_status=$?
set -e
if [[ ${malformed_diagnostic_status} -eq 0 || \
  -n "$(find "${malformed_diagnostic_output}" -mindepth 1 -print -quit)" || \
  ! -s "${temp_dir}/malformed-memory-tool-route-diagnostic.stderr" || \
  "$(cat "${temp_dir}/malformed-memory-tool-route-diagnostic.stderr")" == *"TypeError"* ]]; then
  echo "Memory regression protocol: malformed retrieval counts did not fail closed cleanly" >&2
  exit 1
fi
assert_cleanup "${malformed_diagnostic_log}"

live_memory_tool_route_output="${temp_dir}/live-memory-tool-route-output"
live_memory_tool_route_log="${temp_dir}/live-memory-tool-route-docker.log"
mkdir "${live_memory_tool_route_output}"
chmod 700 "${live_memory_tool_route_output}"
FAKE_DOCKER_LOG="${live_memory_tool_route_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${live_memory_tool_route_output}" \
  --provider-mode live_siliconflow \
  --capture-mode development_memory_tool_route \
  --credential-file "${credential_file}" \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --memory-tool-route-credential-file "${memory_tool_route_credential_file}" \
  --memory-tool-route-provider-id configured-gpt \
  --memory-tool-route-provider-type openai \
  --memory-tool-route-base-url https://api.openai.example/v1 \
  --memory-tool-route-model gpt-test \
  --memory-tool-route-approval I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA \
  >"${temp_dir}/live-memory-tool-route.stdout" \
  2>"${temp_dir}/live-memory-tool-route.stderr"
if [[ "$(find "${live_memory_tool_route_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: live Memory Tool-route bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${live_memory_tool_route_log}"

live_configured_judge_output="${temp_dir}/live-configured-judge-output"
live_configured_judge_log="${temp_dir}/live-configured-judge-docker.log"
mkdir "${live_configured_judge_output}"
chmod 700 "${live_configured_judge_output}"
FAKE_DOCKER_LOG="${live_configured_judge_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${live_configured_judge_output}" \
  --provider-mode live_siliconflow \
  --capture-mode development_configured_candidate_judge \
  --credential-file "${credential_file}" \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --configured-candidate-judge-credential-file "${configured_judge_credential_file}" \
  --configured-candidate-judge-provider-id configured-gpt \
  --configured-candidate-judge-provider-type openai \
  --configured-candidate-judge-base-url https://api.openai.example/v1 \
  --configured-candidate-judge-model gpt-test \
  --configured-candidate-judge-approval I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA \
  >"${temp_dir}/live-configured-judge.stdout" \
  2>"${temp_dir}/live-configured-judge.stderr"
if [[ "$(find "${live_configured_judge_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: live configured candidate-judge bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${live_configured_judge_log}"

live_fixed_judge_output="${temp_dir}/live-fixed-judge-output"
live_fixed_judge_log="${temp_dir}/live-fixed-judge-docker.log"
mkdir "${live_fixed_judge_output}"
chmod 700 "${live_fixed_judge_output}"
FAKE_DOCKER_LOG="${live_fixed_judge_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${live_fixed_judge_output}" \
  --provider-mode live_siliconflow \
  --capture-mode development_fixed_memory_judge \
  --credential-file "${credential_file}" \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --configured-candidate-judge-credential-file "${configured_judge_credential_file}" \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --configured-candidate-judge-approval I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA \
  >"${temp_dir}/live-fixed-judge.stdout" \
  2>"${temp_dir}/live-fixed-judge.stderr"
if [[ "$(find "${live_fixed_judge_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: live fixed Memory Judge bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${live_fixed_judge_log}"

live_accuracy_first_output="${temp_dir}/live-accuracy-first-output"
live_accuracy_first_log="${temp_dir}/live-accuracy-first-docker.log"
mkdir "${live_accuracy_first_output}"
chmod 700 "${live_accuracy_first_output}"
FAKE_DOCKER_LOG="${live_accuracy_first_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${live_accuracy_first_output}" \
  --provider-mode live_siliconflow \
  --capture-mode development_fixed_memory_judge_accuracy \
  --credential-file "${credential_file}" \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --configured-candidate-judge-credential-file "${configured_judge_credential_file}" \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --configured-candidate-judge-approval I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA \
  >"${temp_dir}/live-accuracy-first.stdout" \
  2>"${temp_dir}/live-accuracy-first.stderr"
if [[ "$(find "${live_accuracy_first_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: live accuracy-first bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${live_accuracy_first_log}"

live_production_validation_output="${temp_dir}/live-production-validation-output"
live_production_validation_log="${temp_dir}/live-production-validation-docker.log"
mkdir "${live_production_validation_output}"
chmod 700 "${live_production_validation_output}"
FAKE_DOCKER_LOG="${live_production_validation_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${live_production_validation_output}" \
  --provider-mode live_siliconflow \
  --capture-mode production_fixed_memory_judge_validation \
  --credential-file "${credential_file}" \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --configured-candidate-judge-credential-file "${configured_judge_credential_file}" \
  --configured-candidate-judge-provider-id SERVER_DEFAULT \
  --configured-candidate-judge-provider-type openai_compatible \
  --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
  --configured-candidate-judge-model gpt-5.6-luna \
  --production-memory-judge-validation-approval I_UNDERSTAND_THIS_USES_REAL_FROZEN_MEMORY_VALIDATION_QUOTA \
  >"${temp_dir}/live-production-validation.stdout" \
  2>"${temp_dir}/live-production-validation.stderr"
if [[ "$(find "${live_production_validation_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 2 ]]; then
  echo "Memory regression protocol: live production Validation wrapper bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${live_production_validation_log}"

for denial in missing-production-approval legacy-development-approval; do
  denial_output="${temp_dir}/${denial}-output"
  mkdir "${denial_output}"
  chmod 700 "${denial_output}"
  denial_args=()
  if [[ "${denial}" == "legacy-development-approval" ]]; then
    denial_args=(
      --configured-candidate-judge-approval
      I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA
    )
  fi
  set +e
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
    --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
    --output-dir "${denial_output}" \
    --provider-mode live_siliconflow \
    --capture-mode production_fixed_memory_judge_validation \
    --credential-file "${credential_file}" \
    --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
    --configured-candidate-judge-credential-file "${configured_judge_credential_file}" \
    --configured-candidate-judge-provider-id SERVER_DEFAULT \
    --configured-candidate-judge-provider-type openai_compatible \
    --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
    --configured-candidate-judge-model gpt-5.6-luna \
    "${denial_args[@]}" \
    >"${temp_dir}/${denial}.stdout" 2>"${temp_dir}/${denial}.stderr"
  denial_status=$?
  set -e
  if [[ ${denial_status} -eq 0 || -n "$(find "${denial_output}" -mindepth 1 -print -quit)" ]]; then
    echo "Memory regression protocol: ${denial} did not fail closed before output" >&2
    exit 1
  fi
done

for credential_denial in shared-file byte-identical; do
  credential_denial_output="${temp_dir}/production-${credential_denial}-output"
  mkdir "${credential_denial_output}"
  chmod 700 "${credential_denial_output}"
  denied_judge_credential="${credential_file}"
  if [[ "${credential_denial}" == "byte-identical" ]]; then
    denied_judge_credential="${identical_judge_credential_file}"
  fi
  set +e
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
    --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
    --output-dir "${credential_denial_output}" \
    --provider-mode live_siliconflow \
    --capture-mode production_fixed_memory_judge_validation \
    --credential-file "${credential_file}" \
    --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
    --configured-candidate-judge-credential-file "${denied_judge_credential}" \
    --configured-candidate-judge-provider-id SERVER_DEFAULT \
    --configured-candidate-judge-provider-type openai_compatible \
    --configured-candidate-judge-base-url https://sub.mumubuku.top/v1 \
    --configured-candidate-judge-model gpt-5.6-luna \
    --production-memory-judge-validation-approval I_UNDERSTAND_THIS_USES_REAL_FROZEN_MEMORY_VALIDATION_QUOTA \
    >"${temp_dir}/production-${credential_denial}.stdout" \
    2>"${temp_dir}/production-${credential_denial}.stderr"
  credential_denial_status=$?
  set -e
  if [[ ${credential_denial_status} -eq 0 || \
    -n "$(find "${credential_denial_output}" -mindepth 1 -print -quit)" ]]; then
    echo "Memory regression protocol: production ${credential_denial} credentials did not fail closed" >&2
    exit 1
  fi
done

same_credential_output="${temp_dir}/same-credential-output"
mkdir "${same_credential_output}"
chmod 700 "${same_credential_output}"
set +e
DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${same_credential_output}" \
  --provider-mode live_siliconflow \
  --capture-mode development_memory_tool_route \
  --credential-file "${credential_file}" \
  --live-approval I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA \
  --memory-tool-route-credential-file "${credential_file}" \
  --memory-tool-route-provider-id configured-gpt \
  --memory-tool-route-provider-type openai \
  --memory-tool-route-base-url https://api.openai.example/v1 \
  --memory-tool-route-model gpt-test \
  --memory-tool-route-approval I_UNDERSTAND_THIS_USES_REAL_CONFIGURED_CHAT_PROVIDER_QUOTA \
  >"${temp_dir}/same-credential.stdout" \
  2>"${temp_dir}/same-credential.stderr"
same_credential_status=$?
set -e
if [[ ${same_credential_status} -eq 0 || \
  -n "$(find "${same_credential_output}" -mindepth 1 -print -quit)" ]]; then
  echo "Memory regression protocol: shared Provider credential did not fail closed" >&2
  exit 1
fi

failure_output="${temp_dir}/failure-output"
failure_log="${temp_dir}/failure-docker.log"
mkdir "${failure_output}"
chmod 700 "${failure_output}"
set +e
FAKE_DOCKER_LOG="${failure_log}" FAKE_RUNNER_STATUS=7 FAKE_PUBLISH=partial \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${failure_output}" --provider-mode fake_protocol \
  >"${temp_dir}/failure.stdout" 2>"${temp_dir}/failure.stderr"
failure_status=$?
set -e
if [[ ${failure_status} -eq 0 || -n "$(find "${failure_output}" -mindepth 1 -print -quit)" ]]; then
  echo "Memory regression protocol: failure left partial output or returned success" >&2
  exit 1
fi
assert_cleanup "${failure_log}"

for signal_name in INT TERM HUP; do
  signal_output="${temp_dir}/signal-${signal_name}-output"
  signal_log="${temp_dir}/signal-${signal_name}-docker.log"
  marker="${temp_dir}/signal-${signal_name}.marker"
  mkdir "${signal_output}"
  chmod 700 "${signal_output}"
  setsid env \
    FAKE_DOCKER_LOG="${signal_log}" \
    FAKE_RUNNER_SLEEP=30 \
    FAKE_RUNNER_MARKER="${marker}" \
    FAKE_PUBLISH=none \
    DOCKER_BIN="${fake_docker}" \
    bash "${runner_script}" \
    --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
    --output-dir "${signal_output}" --provider-mode fake_protocol \
    >"${temp_dir}/signal-${signal_name}.stdout" \
    2>"${temp_dir}/signal-${signal_name}.stderr" &
  wrapper_pid=$!
  for _ in $(seq 1 100); do
    [[ -f "${marker}" ]] && break
    sleep 0.05
  done
  if [[ ! -f "${marker}" ]]; then
    echo "Memory regression protocol: ${signal_name} runner did not start" >&2
    kill -TERM -- "-${wrapper_pid}" 2>/dev/null || true
    wait "${wrapper_pid}" || true
    exit 1
  fi
  kill -s "${signal_name}" -- "-${wrapper_pid}"
  set +e
  wait "${wrapper_pid}"
  signal_status=$?
  set -e
  if [[ ${signal_status} -eq 0 || -n "$(find "${signal_output}" -mindepth 1 -print -quit)" ]]; then
    echo "Memory regression protocol: ${signal_name} did not fail closed" >&2
    exit 1
  fi
  assert_cleanup "${signal_log}"
done

echo "Memory regression topology/lifecycle: passed"
