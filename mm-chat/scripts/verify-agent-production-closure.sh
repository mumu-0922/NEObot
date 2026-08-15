#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
evaluator="${script_dir}/evaluate-agent-production-closure.py"
policy="${project_dir}/config/agent-runner/production-policy.json"
held_fixture="${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-production-closure.valid.json"
invalid_fixture="${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-production-closure.invalid.json"
record=""

usage() {
  cat >&2 <<'USAGE'
usage: verify-agent-production-closure.sh [--record <closure.json>] [--policy <policy.json>]

Without --record, run only the offline fail-closed contract self-test. With a
record, run the self-test first and then return the read-only production verdict
(0 ready, 3 held, 2 invalid). This command never enables Runtime or mutates live
state.
USAGE
}

while (($# > 0)); do
  case "$1" in
    --record)
      (($# >= 2)) || { usage; exit 2; }
      record="$2"
      shift 2
      ;;
    --policy)
      (($# >= 2)) || { usage; exit 2; }
      policy="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage
      exit 2
      ;;
  esac
done

for command in python3 jq sha256sum date; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    printf 'Agent production closure verification: %s is required\n' "${command}" >&2
    exit 1
  fi
done
for path in "${evaluator}" "${policy}" "${held_fixture}" "${invalid_fixture}"; do
  if [[ ! -s "${path}" ]]; then
    printf 'Agent production closure verification: missing artifact %s\n' "${path}" >&2
    exit 1
  fi
done

work_dir="$(mktemp -d)"
cleanup() {
  find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

run_case() {
  local name="$1"
  local expected_status="$2"
  local expected_verdict="$3"
  local expected_reason="$4"
  local input="$5"
  local output_file="${work_dir}/decision-${name}.json"
  local status

  set +e
  python3 "${evaluator}" --policy "${policy}" --record "${input}" \
    >"${output_file}"
  status=$?
  set -e
  if [[ "${status}" -ne "${expected_status}" ]]; then
    printf 'Agent production closure verification: %s status=%s expected=%s\n' \
      "${name}" "${status}" "${expected_status}" >&2
    cat "${output_file}" >&2
    exit 1
  fi
  jq -e \
    --arg verdict "${expected_verdict}" \
    --arg reason "${expected_reason}" \
    '.schemaVersion == "neo.agent-production-decision/v1" and
     .verdict == $verdict and .reasonCode == $reason and
     (.policyFingerprint | test("^sha256:[0-9a-f]{64}$")) and
     (.evidenceFingerprint | test("^sha256:[0-9a-f]{64}$"))' \
    "${output_file}" >/dev/null
}

digest_one="sha256:$(printf 'production-closure-one' | sha256sum | awk '{print $1}')"
digest_two="sha256:$(printf 'production-closure-two' | sha256sum | awk '{print $1}')"
fingerprint_of() {
  printf 'sha256:%s' "$(printf '%s' "$1" | sha256sum | awk '{print $1}')"
}
chain_control="$(fingerprint_of chain-control)"
chain_root="$(fingerprint_of chain-root)"
chain_broker="$(fingerprint_of chain-broker)"
chain_project="$(fingerprint_of chain-project)"
chain_child="$(fingerprint_of chain-child)"
chain_cron="$(fingerprint_of chain-cron)"
chain_learning="$(fingerprint_of chain-learning)"
product_activation="$(fingerprint_of product-activation)"
product_request="$(fingerprint_of product-request)"
product_run="$(fingerprint_of product-run)"
product_plan="$(fingerprint_of product-plan)"
product_receipt="$(fingerprint_of product-receipt)"
policy_digest="sha256:$(sha256sum "${policy}" | awk '{print $1}')"
now_epoch="$(date -u +%s)"
format_epoch() {
  date -u -d "@$1" '+%Y-%m-%dT%H:%M:%SZ'
}
started_at="$(format_epoch "$((now_epoch - 180))")"
observed_at="$(format_epoch "$((now_epoch - 120))")"
completed_at="$(format_epoch "$((now_epoch - 60))")"
reviewed_at="$(format_epoch "$((now_epoch - 30))")"
expires_at="$(format_epoch "$((now_epoch + 3600))")"
stale_started_at="$(format_epoch "$((now_epoch - 7200))")"
stale_observed_at="$(format_epoch "$((now_epoch - 7100))")"
stale_completed_at="$(format_epoch "$((now_epoch - 7000))")"
stale_reviewed_at="$(format_epoch "$((now_epoch - 6900))")"
stale_expires_at="$(format_epoch "$((now_epoch - 1))")"

jq \
  --arg started_at "${started_at}" \
  --arg observed_at "${observed_at}" \
  --arg completed_at "${completed_at}" \
  --arg reviewed_at "${reviewed_at}" \
  --arg expires_at "${expires_at}" '
  .window.startedAt = $started_at |
  .window.completedAt = $completed_at |
  .window.expiresAt = $expires_at |
  .checks |= map(.observedAt = $observed_at) |
  .review.reviewedAt = $reviewed_at
' "${held_fixture}" >"${work_dir}/current-held.json"

jq \
  --arg digest_one "${digest_one}" \
  --arg digest_two "${digest_two}" \
  --arg policy_digest "${policy_digest}" \
  --arg chain_control "${chain_control}" \
  --arg chain_root "${chain_root}" \
  --arg chain_broker "${chain_broker}" \
  --arg chain_project "${chain_project}" \
  --arg chain_child "${chain_child}" \
  --arg chain_cron "${chain_cron}" \
  --arg chain_learning "${chain_learning}" \
  --arg product_activation "${product_activation}" \
  --arg product_request "${product_request}" \
  --arg product_run "${product_run}" \
  --arg product_plan "${product_plan}" \
  --arg product_receipt "${product_receipt}" '
  .evidenceClass = "production" |
  .release.gitCommit = "1111111111111111111111111111111111111111" |
  .release.runnerManifestSha256 = $digest_one |
  .release.runnerBinarySha256 = $digest_two |
  .release.runtimeBundleFingerprint = $digest_one |
  .release.operationsPolicySha256 = $policy_digest |
  .target.deploymentFingerprint = $digest_two |
  .activationChain = {
    controlPlane: $chain_control,
    rootRun: $chain_root,
    brokerArtifact: $chain_broker,
    projectMutation: $chain_project,
    depthOneChild: $chain_child,
    cronWorker: $chain_cron,
    draftLearning: $chain_learning
  } |
  .productCanary = {
    activationFingerprint: $product_activation,
    requestFingerprint: $product_request,
    runSnapshotFingerprint: $product_run,
    planFingerprint: $product_plan,
    receiptFingerprint: $product_receipt
  } |
  .checks |= map(
    .result = "passed" |
    .evidenceSha256 = $digest_one |
    .detailCode = "PASS"
  ) |
  .review.decision = "approved" |
  .review.reviewerFingerprint = $digest_two
' "${work_dir}/current-held.json" >"${work_dir}/ready.json"

jq '.evidenceClass = "template"' \
  "${work_dir}/ready.json" >"${work_dir}/template-ready.json"
jq '.cleanup.temporaryArtifacts = 1' \
  "${work_dir}/ready.json" >"${work_dir}/cleanup-residue.json"
jq '.cleanup.temporaryProductRequests = 1' \
  "${work_dir}/ready.json" >"${work_dir}/product-residue.json"
jq '.activationChain.draftLearning = .activationChain.cronWorker' \
  "${work_dir}/ready.json" >"${work_dir}/activation-chain-drift.json"
jq '.productCanary.receiptFingerprint = .productCanary.requestFingerprint' \
  "${work_dir}/ready.json" >"${work_dir}/product-receipt-mismatch.json"
jq \
  --arg started_at "${stale_started_at}" \
  --arg observed_at "${stale_observed_at}" \
  --arg completed_at "${stale_completed_at}" \
  --arg reviewed_at "${stale_reviewed_at}" \
  --arg expires_at "${stale_expires_at}" '
  .window.startedAt = $started_at |
  .window.completedAt = $completed_at |
  .window.expiresAt = $expires_at |
  .checks |= map(.observedAt = $observed_at) |
  .review.reviewedAt = $reviewed_at
' "${work_dir}/ready.json" >"${work_dir}/stale.json"
jq '.release.operationsPolicySha256 = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"' \
  "${work_dir}/ready.json" >"${work_dir}/policy-drift.json"
jq 'del(.checks[-1])' \
  "${work_dir}/ready.json" >"${work_dir}/incomplete.json"
jq '.checks[-1] = .checks[0]' \
  "${work_dir}/ready.json" >"${work_dir}/duplicate.json"
cp "${work_dir}/ready.json" "${work_dir}/writable.json"
chmod 0666 "${work_dir}/writable.json"
ln -s "${held_fixture}" "${work_dir}/symlink.json"

run_case held-isolation 3 PROMOTION_HELD ISOLATION_UNAVAILABLE \
  "${work_dir}/current-held.json"
run_case synthetic-ready 0 PROMOTION_READY ALL_PRODUCTION_GATES_PASSED \
  "${work_dir}/ready.json"
run_case template-ready 3 PROMOTION_HELD NON_PRODUCTION_EVIDENCE \
  "${work_dir}/template-ready.json"
run_case stale 3 PROMOTION_HELD EVIDENCE_STALE \
  "${work_dir}/stale.json"
run_case cleanup-residue 3 PROMOTION_HELD TEMPORARY_EVIDENCE_REMAINS \
  "${work_dir}/cleanup-residue.json"
run_case product-residue 3 PROMOTION_HELD TEMPORARY_EVIDENCE_REMAINS \
  "${work_dir}/product-residue.json"
run_case activation-chain-drift 2 PROMOTION_EVIDENCE_INVALID ACTIVATION_CHAIN_INVALID \
  "${work_dir}/activation-chain-drift.json"
run_case product-receipt-mismatch 2 PROMOTION_EVIDENCE_INVALID PRODUCT_CANARY_BINDING_INVALID \
  "${work_dir}/product-receipt-mismatch.json"
run_case policy-drift 2 PROMOTION_EVIDENCE_INVALID POLICY_FINGERPRINT_MISMATCH \
  "${work_dir}/policy-drift.json"
run_case incomplete 2 PROMOTION_EVIDENCE_INVALID CHECK_SET_INCOMPLETE \
  "${work_dir}/incomplete.json"
run_case duplicate 2 PROMOTION_EVIDENCE_INVALID CHECK_SET_INVALID \
  "${work_dir}/duplicate.json"
run_case malformed 2 PROMOTION_EVIDENCE_INVALID EVIDENCE_ROOT_INVALID \
  "${invalid_fixture}"
run_case writable 2 PROMOTION_EVIDENCE_INVALID DOCUMENT_FILE_UNSAFE \
  "${work_dir}/writable.json"
run_case symlink 2 PROMOTION_EVIDENCE_INVALID DOCUMENT_UNREADABLE \
  "${work_dir}/symlink.json"

printf '%s\n' \
  'Agent Runtime G21.6 offline closure contract passed; production promotion remains evidence-gated.'

if [[ -n "${record}" ]]; then
  production_args=(--policy "${policy}" --record "${record}")
  exec python3 "${evaluator}" "${production_args[@]}"
fi
