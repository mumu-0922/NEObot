#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"

for command in bash docker go jq python3 rg; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "G21.6 verification: ${command} is required" >&2
    exit 1
  }
done

for path in \
  "${backend_dir}/cmd/agent-runtime-product-canary/main.go" \
  "${backend_dir}/internal/agentactivation/product_canary.go" \
  "${backend_dir}/internal/agentproductcanary/plan.go" \
  "${backend_dir}/internal/agentproductcanary/service.go" \
  "${backend_dir}/internal/agentproductcanary/repository_postgres.go" \
  "${backend_dir}/migrations/095_agent_product_canary_activation.up.sql" \
  "${backend_dir}/migrations/095_agent_product_canary_activation.down.sql" \
  "${project_dir}/docs/contracts/schemas/neo-agent-product-canary-activation.schema.json" \
  "${project_dir}/docs/contracts/schemas/neo-agent-product-canary-plan.schema.json" \
  "${script_dir}/verify-agent-product-canary-activation.sh" \
  "${script_dir}/verify-agent-product-canary-postgres17.sh" \
  "${script_dir}/verify-agent-runtime-g21-6-preflight.sh"; do
  [[ -s "${path}" ]] || {
    echo "G21.6 verification: missing ${path}" >&2
    exit 1
  }
done

if rg -n 'agentproductcanary|agent-runtime-product-canary' "${backend_dir}/cmd/api" >/dev/null; then
  echo 'G21.6 verification: product worker execution authority leaked into cmd/api' >&2
  exit 1
fi
if [[ "$(find "${backend_dir}/migrations" -maxdepth 1 -name '095_*.up.sql' -printf '%f\n')" != \
  '095_agent_product_canary_activation.up.sql' ]]; then
  echo 'G21.6 verification: migration 095 tail is missing or widened' >&2
  exit 1
fi

for signature in \
  'CREATE ROLE agent_product_canary_worker NOLOGIN NOSUPERUSER' \
  'CREATE TABLE agent_product_canary_activations(' \
  'CREATE TABLE agent_product_canary_requests(' \
  'CREATE TABLE agent_product_canary_receipts(' \
  'CREATE TABLE agent_product_canary_promotions(' \
  'CREATE FUNCTION agent_product_canary_enqueue(' \
  'CREATE FUNCTION agent_product_canary_worker_health(' \
  'CREATE FUNCTION agent_product_canary_worker_claim_requests(' \
  'CREATE FUNCTION agent_product_canary_worker_complete_request(' \
  'CREATE FUNCTION agent_product_canary_record_promotion(' \
  'TO agent_product_canary_worker;'; do
  rg -Fq "${signature}" "${backend_dir}/migrations/095_agent_product_canary_activation.up.sql" || {
    echo "G21.6 verification: migration authority signature missing: ${signature}" >&2
    exit 1
  }
done
for forbidden in \
  'GRANT SELECT ON agent_product_canary_' \
  'GRANT INSERT ON agent_product_canary_' \
  'GRANT UPDATE ON agent_product_canary_' \
  'GRANT agent_orchestrator_runtime TO agent_product_canary_worker' \
  'GRANT agent_runner_control TO agent_product_canary_worker'; do
  if rg -Fiq "${forbidden}" "${backend_dir}/migrations/095_agent_product_canary_activation.up.sql"; then
    echo "G21.6 verification: product authority widened: ${forbidden}" >&2
    exit 1
  fi
done
if sed -n '/GRANT EXECUTE ON FUNCTION agent_product_canary_status/,/TO agent_product_canary_worker;/p' \
  "${backend_dir}/migrations/095_agent_product_canary_activation.up.sql" | \
  rg -Fq 'agent_product_canary_record_promotion'; then
  echo 'G21.6 verification: API or worker received final-promotion authority' >&2
  exit 1
fi
rg -Fq 'AGENT_PRODUCT_CANARY_DOWN_REQUIRES_EMPTY' \
  "${backend_dir}/migrations/095_agent_product_canary_activation.down.sql"

for signature in \
  'productCanaryCallerIdentity = "spiffe://neo-chat/agent-runtime-product-canary"' \
  'agentrunner.MethodLaunch, agentrunner.MethodHeartbeat, agentrunner.MethodCancel'; do
  rg -Fq "${signature}" "${backend_dir}/cmd/neo-runnerd/main.go" || {
    echo "G21.6 verification: Runner caller policy drifted: ${signature}" >&2
    exit 1
  }
done
if sed -n '/productIdentity :=/,/handler, err :=/p' "${backend_dir}/cmd/neo-runnerd/main.go" | \
  rg -q 'Method(Prepare|Commit|Result)'; then
  echo 'G21.6 verification: product caller received Broker or result authority' >&2
  exit 1
fi

for signature in \
  'ExpectedPolicyRevision int64 `json:"expectedPolicyRevision"`' \
  'ExpectedGeneration     int64 `json:"expectedGeneration"`' \
  'service.repository.EnqueueProductCanary' \
  'neo.agent-product-canary-request/v1'; do
  rg -Fq "${signature}" "${backend_dir}/internal/agentcontrol" || {
    echo "G21.6 verification: authenticated fixed-request boundary missing: ${signature}" >&2
    exit 1
  }
done
if rg -n 'agent(orchestrator|runner)|Sandbox|ToolRegistry|Argv' \
  "${backend_dir}/internal/agentcontrol/service.go" \
  "${backend_dir}/internal/agentcontrol/handler.go" >/dev/null; then
  echo 'G21.6 verification: API request boundary constructs execution authority' >&2
  exit 1
fi

(cd "${backend_dir}" && go test -race \
  ./internal/agentactivation ./internal/agentproductcanary \
  ./internal/agentcontrol ./internal/agentrootcanary \
  ./cmd/agent-runtime-product-canary ./cmd/neo-runnerd)
(cd "${backend_dir}" && go vet \
  ./internal/agentactivation ./internal/agentproductcanary \
  ./internal/agentcontrol ./internal/agentrootcanary \
  ./cmd/agent-runtime-product-canary ./cmd/neo-runnerd)

bash "${script_dir}/verify-agent-product-canary-activation.sh"
bash "${script_dir}/verify-agent-production-closure.sh"
bash "${script_dir}/verify-agent-runtime-g21-6-preflight.sh"
bash "${script_dir}/verify-agent-product-canary-postgres17.sh"

# G21.6 adds only the bounded product request/worker/final-closure bridge to the
# complete G21.0-G21.5 exact-host chain.
bash "${script_dir}/verify-agent-runtime-g21-5.sh"

set +e
host_output="$(bash "${script_dir}/verify-agent-runner-host.sh" 2>&1)"
host_status=$?
set -e
if [[ "${host_status}" -eq 0 || "${host_output}" != *ISOLATION_UNAVAILABLE* ]]; then
  echo 'G21.6 verification: current host unexpectedly became production-eligible' >&2
  exit 1
fi

echo 'Agent Runtime G21.6 verification: passed; current host remains ISOLATION_UNAVAILABLE.'
