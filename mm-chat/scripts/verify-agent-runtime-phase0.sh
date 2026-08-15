#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
python_bin="${project_dir}/rag/.venv/bin/python"

if [[ ! -x "${python_bin}" ]]; then
  if ! command -v uv >/dev/null 2>&1; then
    echo "Agent Runtime Phase 0 verification: uv is required when rag/.venv is absent" >&2
    exit 1
  fi
  python_command=(
    uv run --project "${project_dir}/rag" --frozen --offline python
  )
else
  python_command=("${python_bin}")
fi

for command in jq bash; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    echo "Agent Runtime Phase 0 verification: ${command} is required" >&2
    exit 1
  fi
done

required_agent_paths=(
  "${project_dir}/backend/internal/agentdelegation/service.go"
  "${project_dir}/backend/internal/agentdelegation/repository_postgres.go"
  "${project_dir}/backend/migrations/087_agent_child_delegation.up.sql"
  "${project_dir}/backend/migrations/087_agent_child_delegation.down.sql"
  "${project_dir}/scripts/verify-agent-delegation.sh"
  "${project_dir}/scripts/verify-agent-delegation-postgres17.sh"
  "${project_dir}/backend/internal/agentcron/service.go"
  "${project_dir}/backend/internal/agentcron/repository_postgres.go"
  "${project_dir}/backend/migrations/088_agent_cron_foundation.up.sql"
  "${project_dir}/backend/migrations/088_agent_cron_foundation.down.sql"
  "${project_dir}/scripts/verify-agent-cron.sh"
  "${project_dir}/scripts/verify-agent-cron-postgres17.sh"
  "${project_dir}/docs/contracts/schemas/neo-cron-template.schema.json"
  "${project_dir}/backend/internal/agentlearning/service.go"
  "${project_dir}/backend/internal/agentlearning/repository_postgres.go"
  "${project_dir}/backend/migrations/089_agent_draft_learning.up.sql"
  "${project_dir}/backend/migrations/089_agent_draft_learning.down.sql"
  "${project_dir}/scripts/verify-agent-learning.sh"
  "${project_dir}/scripts/verify-agent-learning-postgres17.sh"
  "${project_dir}/docs/contracts/schemas/neo-skill-draft.schema.json"
  "${project_dir}/backend/internal/agentcontrol/service.go"
  "${project_dir}/backend/internal/agentcontrol/repository_postgres.go"
  "${project_dir}/backend/internal/agentcontrol/handler.go"
  "${project_dir}/backend/migrations/090_agent_product_shadow.up.sql"
  "${project_dir}/backend/migrations/090_agent_product_shadow.down.sql"
  "${project_dir}/frontend/src/components/agent/AgentCenter.tsx"
  "${project_dir}/frontend/src/store/storage/legacySkillRetirement.ts"
  "${project_dir}/frontend/src/services/api/client/server/agentCenterApi.ts"
  "${project_dir}/scripts/cutover-legacy-skills.sql"
  "${project_dir}/scripts/verify-agent-legacy-cutover.sh"
  "${project_dir}/scripts/verify-agent-legacy-cutover-postgres17.sh"
  "${project_dir}/scripts/verify-agent-product-shadow.sh"
  "${project_dir}/scripts/verify-agent-product-shadow-postgres17.sh"
  "${project_dir}/config/agent-runner/production-policy.json"
  "${project_dir}/docs/contracts/schemas/neo-agent-production-policy.schema.json"
  "${project_dir}/docs/contracts/schemas/neo-agent-production-closure.schema.json"
  "${project_dir}/scripts/evaluate-agent-production-closure.py"
  "${project_dir}/scripts/verify-agent-production-closure.sh"
  "${project_dir}/backend/cmd/agent-runtime-control/main.go"
  "${project_dir}/backend/internal/agentactivation/activation.go"
  "${project_dir}/backend/internal/agentruntimecontrol/control.go"
  "${project_dir}/docs/contracts/schemas/neo-agent-production-activation.schema.json"
  "${project_dir}/docs/contracts/schemas/neo-agent-runner-bundle.schema.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-runner-bundle.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-runner-bundle.invalid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-production-activation.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-production-activation.invalid.json"
  "${project_dir}/scripts/evaluate-agent-production-activation.py"
  "${project_dir}/scripts/build-agent-runner-bundle.sh"
  "${project_dir}/scripts/verify-agent-runner-bundle.py"
  "${project_dir}/scripts/verify-agent-runner-bundle.sh"
  "${project_dir}/scripts/verify-agent-runtime-g21-0.sh"
  "${project_dir}/backend/cmd/agent-runtime-root-canary/main.go"
  "${project_dir}/backend/internal/agentactivation/root_canary.go"
  "${project_dir}/backend/internal/agentrootcanary/service.go"
  "${project_dir}/backend/internal/agentrootcanary/repository_postgres.go"
  "${project_dir}/docs/contracts/schemas/neo-agent-root-run-canary-activation.schema.json"
  "${project_dir}/docs/contracts/schemas/neo-agent-root-run-canary-plan.schema.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-root-run-canary-activation.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-root-run-canary-activation.invalid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-root-run-canary-plan.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-root-run-canary-plan.invalid.json"
  "${project_dir}/scripts/verify-agent-root-canary-activation.sh"
  "${project_dir}/scripts/verify-agent-root-canary-postgres17.sh"
  "${project_dir}/scripts/verify-agent-runtime-g21-1.sh"
  "${project_dir}/backend/cmd/agent-runtime-broker-canary/main.go"
  "${project_dir}/backend/internal/agentactivation/broker_canary.go"
  "${project_dir}/backend/internal/agentbrokercanary/service.go"
  "${project_dir}/backend/internal/agentbrokerrelay/handler.go"
  "${project_dir}/backend/internal/agentbroker/artifact_postgres.go"
  "${project_dir}/backend/migrations/091_agent_artifact_publication.up.sql"
  "${project_dir}/backend/migrations/091_agent_artifact_publication.down.sql"
  "${project_dir}/docs/contracts/schemas/neo-agent-broker-artifact-canary-activation.schema.json"
  "${project_dir}/docs/contracts/schemas/neo-agent-broker-artifact-canary-plan.schema.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-broker-artifact-canary-activation.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-broker-artifact-canary-activation.invalid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-broker-artifact-canary-plan.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-broker-artifact-canary-plan.invalid.json"
  "${project_dir}/scripts/verify-agent-artifact-publication-postgres17.sh"
  "${project_dir}/scripts/verify-agent-broker-canary-activation.sh"
  "${project_dir}/scripts/verify-agent-broker-canary-preflight.sh"
  "${project_dir}/scripts/verify-agent-runtime-g21-2.sh"
  "${project_dir}/backend/cmd/agent-runtime-project-canary/main.go"
  "${project_dir}/backend/internal/agentactivation/project_canary.go"
  "${project_dir}/backend/internal/agentprojectcanary/service.go"
  "${project_dir}/backend/internal/agentbroker/project_mutation_postgres.go"
  "${project_dir}/backend/migrations/092_agent_project_mutation_canary.up.sql"
  "${project_dir}/backend/migrations/092_agent_project_mutation_canary.down.sql"
  "${project_dir}/docs/contracts/schemas/neo-agent-project-mutation-canary-activation.schema.json"
  "${project_dir}/docs/contracts/schemas/neo-agent-project-mutation-canary-plan.schema.json"
  "${project_dir}/docs/contracts/schemas/neo-agent-project-mutation-approval.schema.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-project-mutation-canary-activation.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-project-mutation-canary-activation.invalid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-project-mutation-canary-plan.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-project-mutation-canary-plan.invalid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-project-mutation-approval.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-project-mutation-approval.invalid.json"
  "${project_dir}/scripts/verify-agent-project-canary-activation.sh"
  "${project_dir}/scripts/verify-agent-project-canary-preflight.sh"
  "${project_dir}/scripts/verify-agent-project-mutation-postgres17.sh"
  "${project_dir}/scripts/verify-agent-runtime-g21-3.sh"
  "${project_dir}/backend/cmd/agent-runtime-child-canary/main.go"
  "${project_dir}/backend/internal/agentactivation/child_canary.go"
  "${project_dir}/backend/internal/agentchildcanary/plan.go"
  "${project_dir}/backend/internal/agentchildcanary/reaper.go"
  "${project_dir}/backend/internal/agentchildcanary/service.go"
  "${project_dir}/backend/migrations/093_agent_child_canary_reap_transport.up.sql"
  "${project_dir}/backend/migrations/093_agent_child_canary_reap_transport.down.sql"
  "${project_dir}/docs/contracts/schemas/neo-agent-child-run-canary-activation.schema.json"
  "${project_dir}/docs/contracts/schemas/neo-agent-child-run-canary-plan.schema.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-child-run-canary-activation.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-child-run-canary-activation.invalid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-child-run-canary-plan.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-child-run-canary-plan.invalid.json"
  "${project_dir}/scripts/verify-agent-child-canary-activation.sh"
  "${project_dir}/scripts/verify-agent-child-canary-preflight.sh"
  "${project_dir}/scripts/verify-agent-child-canary-postgres17.sh"
  "${project_dir}/scripts/verify-agent-runtime-g21-4.sh"
  "${project_dir}/backend/cmd/agent-runtime-cron-worker/main.go"
  "${project_dir}/backend/cmd/agent-runtime-draft-learning-worker/main.go"
  "${project_dir}/backend/internal/agentactivation/cron_learning_workers.go"
  "${project_dir}/backend/internal/agentcronworker/plan.go"
  "${project_dir}/backend/internal/agentcronworker/service.go"
  "${project_dir}/backend/internal/agentcronworker/target_postgres.go"
  "${project_dir}/backend/internal/agentlearningworker/checker.go"
  "${project_dir}/backend/internal/agentlearningworker/plan.go"
  "${project_dir}/backend/internal/agentlearningworker/repository_postgres.go"
  "${project_dir}/backend/internal/agentlearningworker/service.go"
  "${project_dir}/backend/migrations/094_agent_cron_learning_activation.up.sql"
  "${project_dir}/backend/migrations/094_agent_cron_learning_activation.down.sql"
  "${project_dir}/docs/contracts/schemas/neo-agent-cron-worker-activation.schema.json"
  "${project_dir}/docs/contracts/schemas/neo-agent-cron-worker-plan.schema.json"
  "${project_dir}/docs/contracts/schemas/neo-agent-draft-learning-worker-activation.schema.json"
  "${project_dir}/docs/contracts/schemas/neo-agent-draft-learning-worker-plan.schema.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-cron-worker-activation.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-cron-worker-activation.invalid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-cron-worker-plan.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-cron-worker-plan.invalid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-draft-learning-worker-activation.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-draft-learning-worker-activation.invalid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-draft-learning-worker-plan.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-draft-learning-worker-plan.invalid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-runner-rpc.result.valid.json"
  "${project_dir}/scripts/verify-agent-cron-worker.sh"
  "${project_dir}/scripts/verify-agent-cron-worker-postgres17.sh"
  "${project_dir}/scripts/verify-agent-draft-learning-worker.sh"
  "${project_dir}/scripts/verify-agent-draft-learning-worker-postgres17.sh"
  "${project_dir}/scripts/verify-agent-runtime-g21-5-preflight.sh"
  "${project_dir}/scripts/verify-agent-runtime-g21-5.sh"
  "${project_dir}/backend/cmd/agent-runtime-product-canary/main.go"
  "${project_dir}/backend/internal/agentactivation/product_canary.go"
  "${project_dir}/backend/internal/agentproductcanary/plan.go"
  "${project_dir}/backend/internal/agentproductcanary/service.go"
  "${project_dir}/backend/internal/agentproductcanary/repository_postgres.go"
  "${project_dir}/backend/migrations/095_agent_product_canary_activation.up.sql"
  "${project_dir}/backend/migrations/095_agent_product_canary_activation.down.sql"
  "${project_dir}/docs/contracts/schemas/neo-agent-product-canary-activation.schema.json"
  "${project_dir}/docs/contracts/schemas/neo-agent-product-canary-plan.schema.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-product-canary-activation.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-product-canary-activation.invalid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-product-canary-plan.valid.json"
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-product-canary-plan.invalid.json"
  "${project_dir}/scripts/verify-agent-product-canary-activation.sh"
  "${project_dir}/scripts/verify-agent-product-canary-postgres17.sh"
  "${project_dir}/scripts/verify-agent-runtime-g21-6-preflight.sh"
  "${project_dir}/scripts/verify-agent-runtime-g21-6.sh"
)
for path in "${required_agent_paths[@]}"; do
  if [[ ! -s "${path}" ]]; then
    echo "Agent Runtime Phase 0 verification: missing Agent artifact ${path}" >&2
    exit 1
  fi
done
grep -Fq "type RunLineage struct" "${project_dir}/backend/internal/agentrunner/types.go"
grep -Fq "CREATE FUNCTION agent_delegation_reconcile(" \
  "${project_dir}/backend/migrations/087_agent_child_delegation.up.sql"
grep -Fq "DROP FUNCTION agent_delegation_reconcile(INTEGER)" \
  "${project_dir}/backend/migrations/087_agent_child_delegation.down.sql"
grep -Fq "CREATE FUNCTION agent_cron_enqueue_trigger(" \
  "${project_dir}/backend/migrations/088_agent_cron_foundation.up.sql"
grep -Fq "CREATE FUNCTION agent_cron_reconcile(" \
  "${project_dir}/backend/migrations/088_agent_cron_foundation.up.sql"
grep -Fq "DROP FUNCTION agent_cron_enqueue_trigger(" \
  "${project_dir}/backend/migrations/088_agent_cron_foundation.down.sql"
grep -Fq "DROP FUNCTION agent_cron_reconcile(TIMESTAMPTZ,INTEGER)" \
  "${project_dir}/backend/migrations/088_agent_cron_foundation.down.sql"
grep -Fq "CREATE FUNCTION agent_learning_create_draft(" \
  "${project_dir}/backend/migrations/089_agent_draft_learning.up.sql"
grep -Fq "CREATE FUNCTION agent_learning_promote(" \
  "${project_dir}/backend/migrations/089_agent_draft_learning.up.sql"
grep -Fq "DROP FUNCTION agent_learning_promote(" \
  "${project_dir}/backend/migrations/089_agent_draft_learning.down.sql"
grep -Fq "DROP FUNCTION agent_learning_reconcile(TIMESTAMPTZ,INTEGER)" \
  "${project_dir}/backend/migrations/089_agent_draft_learning.down.sql"
grep -Fq "CREATE FUNCTION agent_product_get_artifact(" \
  "${project_dir}/backend/migrations/090_agent_product_shadow.up.sql"
grep -Fq "CREATE FUNCTION agent_product_cancel_run(" \
  "${project_dir}/backend/migrations/090_agent_product_shadow.up.sql"
grep -Fq "CREATE FUNCTION agent_product_shadow_snapshot(" \
  "${project_dir}/backend/migrations/090_agent_product_shadow.up.sql"
grep -Fq "CREATE FUNCTION agent_product_append_shadow_observation(" \
  "${project_dir}/backend/migrations/090_agent_product_shadow.up.sql"
grep -Fq "DROP FUNCTION agent_product_append_shadow_observation(" \
  "${project_dir}/backend/migrations/090_agent_product_shadow.down.sql"
grep -Fq "AGENT_PRODUCT_DOWN_DATA_EXISTS" \
  "${project_dir}/backend/migrations/090_agent_product_shadow.down.sql"
grep -Fq "CREATE FUNCTION agent_artifact_authorize(" \
  "${project_dir}/backend/migrations/091_agent_artifact_publication.up.sql"
grep -Fq "CREATE FUNCTION agent_artifact_attach(" \
  "${project_dir}/backend/migrations/091_agent_artifact_publication.up.sql"
grep -Fq "DROP FUNCTION agent_artifact_attach(" \
  "${project_dir}/backend/migrations/091_agent_artifact_publication.down.sql"
grep -Fq "CREATE FUNCTION agent_project_mutation_commit(" \
  "${project_dir}/backend/migrations/092_agent_project_mutation_canary.up.sql"
grep -Fq "CREATE FUNCTION agent_project_mutation_status(" \
  "${project_dir}/backend/migrations/092_agent_project_mutation_canary.up.sql"
grep -Fq "CREATE FUNCTION agent_project_mutation_cleanup(" \
  "${project_dir}/backend/migrations/092_agent_project_mutation_canary.up.sql"
grep -Fq "AGENT_PROJECT_MUTATION_DOWN_REQUIRES_EMPTY" \
  "${project_dir}/backend/migrations/092_agent_project_mutation_canary.down.sql"
grep -Fq "CREATE FUNCTION agent_delegation_reap_inventory(" \
  "${project_dir}/backend/migrations/093_agent_child_canary_reap_transport.up.sql"
grep -Fq "AGENT_REAP_AUTHORITY_ACTIVE" \
  "${project_dir}/backend/migrations/093_agent_child_canary_reap_transport.up.sql"
grep -Fq "AGENT_CHILD_REAP_TRANSPORT_DOWN_REQUIRES_CLEAN" \
  "${project_dir}/backend/migrations/093_agent_child_canary_reap_transport.down.sql"
grep -Fq "CREATE FUNCTION agent_cron_worker_claim_due(" \
  "${project_dir}/backend/migrations/094_agent_cron_learning_activation.up.sql"
grep -Fq "CREATE FUNCTION agent_learning_worker_issue_runner_authority(" \
  "${project_dir}/backend/migrations/094_agent_cron_learning_activation.up.sql"
grep -Fq "CREATE FUNCTION agent_learning_worker_record_runner_result(" \
  "${project_dir}/backend/migrations/094_agent_cron_learning_activation.up.sql"
grep -Fq "TO agent_cron_worker;" \
  "${project_dir}/backend/migrations/094_agent_cron_learning_activation.up.sql"
grep -Fq "TO agent_learning_worker;" \
  "${project_dir}/backend/migrations/094_agent_cron_learning_activation.up.sql"
grep -Fq "AGENT_WORKER_DOWN_RETAINED_ACTIVATION_FACTS" \
  "${project_dir}/backend/migrations/094_agent_cron_learning_activation.down.sql"
grep -Fq "CREATE FUNCTION agent_product_canary_enqueue(" \
  "${project_dir}/backend/migrations/095_agent_product_canary_activation.up.sql"
grep -Fq "CREATE FUNCTION agent_product_canary_record_promotion(" \
  "${project_dir}/backend/migrations/095_agent_product_canary_activation.up.sql"
grep -Fq "AGENT_PRODUCT_CANARY_DOWN_REQUIRES_EMPTY" \
  "${project_dir}/backend/migrations/095_agent_product_canary_activation.down.sql"

schema_dir="${project_dir}/docs/contracts/schemas"
fixture_dir="${project_dir}/docs/contracts/fixtures/agent-runtime"

for path in "${schema_dir}"/*.json "${fixture_dir}"/*.json; do
  jq empty "${path}" >/dev/null
done

bash -n "${project_dir}/scripts/verify-agent-runtime-phase0.sh"
bash -n "${project_dir}/scripts/verify-agent-production-closure.sh"
bash -n "${project_dir}/scripts/verify-agent-runtime-g21-0.sh"
bash -n "${project_dir}/scripts/verify-agent-root-canary-activation.sh"
bash -n "${project_dir}/scripts/verify-agent-root-canary-postgres17.sh"
bash -n "${project_dir}/scripts/verify-agent-runtime-g21-1.sh"
bash -n "${project_dir}/scripts/verify-agent-artifact-publication-postgres17.sh"
bash -n "${project_dir}/scripts/verify-agent-broker-canary-activation.sh"
bash -n "${project_dir}/scripts/verify-agent-broker-canary-preflight.sh"
bash -n "${project_dir}/scripts/verify-agent-runtime-g21-2.sh"
bash -n "${project_dir}/scripts/verify-agent-project-canary-activation.sh"
bash -n "${project_dir}/scripts/verify-agent-project-canary-preflight.sh"
bash -n "${project_dir}/scripts/verify-agent-project-mutation-postgres17.sh"
bash -n "${project_dir}/scripts/verify-agent-runtime-g21-3.sh"
bash -n "${project_dir}/scripts/verify-agent-child-canary-activation.sh"
bash -n "${project_dir}/scripts/verify-agent-child-canary-preflight.sh"
bash -n "${project_dir}/scripts/verify-agent-child-canary-postgres17.sh"
bash -n "${project_dir}/scripts/verify-agent-runtime-g21-4.sh"
bash -n "${project_dir}/scripts/verify-agent-cron-worker.sh"
bash -n "${project_dir}/scripts/verify-agent-cron-worker-postgres17.sh"
bash -n "${project_dir}/scripts/verify-agent-draft-learning-worker.sh"
bash -n "${project_dir}/scripts/verify-agent-draft-learning-worker-postgres17.sh"
bash -n "${project_dir}/scripts/verify-agent-runtime-g21-5-preflight.sh"
bash -n "${project_dir}/scripts/verify-agent-runtime-g21-5.sh"
bash -n "${project_dir}/scripts/verify-agent-product-canary-activation.sh"
bash -n "${project_dir}/scripts/verify-agent-product-canary-postgres17.sh"
bash -n "${project_dir}/scripts/verify-agent-runtime-g21-6-preflight.sh"
bash -n "${project_dir}/scripts/verify-agent-runtime-g21-6.sh"

bash "${project_dir}/scripts/verify-agent-runtime-g21-2.sh"
bash "${project_dir}/scripts/verify-agent-project-canary-activation.sh"
bash "${project_dir}/scripts/verify-agent-child-canary-activation.sh"
bash "${project_dir}/scripts/verify-agent-cron-worker.sh"
bash "${project_dir}/scripts/verify-agent-draft-learning-worker.sh"
bash "${project_dir}/scripts/verify-agent-runtime-g21-5-preflight.sh"
bash "${project_dir}/scripts/verify-agent-product-canary-activation.sh"
bash "${project_dir}/scripts/verify-agent-runtime-g21-6-preflight.sh"

isolated_root="$(mktemp -d)"
cleanup() {
  if [[ -d "${isolated_root}" ]]; then
    find "${isolated_root}" -depth -mindepth 1 -delete
    rmdir "${isolated_root}" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

mkdir -p \
  "${isolated_root}/mm-chat/backend/internal/codejobs" \
  "${isolated_root}/mm-chat/backend/internal/agentcontrol" \
  "${isolated_root}/mm-chat/backend/migrations" \
  "${isolated_root}/mm-chat/frontend/src/store/storage" \
  "${isolated_root}/mm-chat/config/agent-runner" \
  "${isolated_root}/mm-chat/docs" \
  "${isolated_root}/mm-chat/scripts"

cp "${project_dir}/backend/internal/codejobs/handler.go" \
  "${project_dir}/backend/internal/codejobs/service.go" \
  "${isolated_root}/mm-chat/backend/internal/codejobs/"
cp "${project_dir}/backend/internal/agentcontrol/service.go" \
  "${isolated_root}/mm-chat/backend/internal/agentcontrol/"
cp "${project_dir}/backend/migrations/090_agent_product_shadow.up.sql" \
  "${project_dir}/backend/migrations/090_agent_product_shadow.down.sql" \
  "${project_dir}/backend/migrations/091_agent_artifact_publication.up.sql" \
  "${project_dir}/backend/migrations/091_agent_artifact_publication.down.sql" \
  "${project_dir}/backend/migrations/092_agent_project_mutation_canary.up.sql" \
  "${project_dir}/backend/migrations/092_agent_project_mutation_canary.down.sql" \
  "${project_dir}/backend/migrations/093_agent_child_canary_reap_transport.up.sql" \
  "${project_dir}/backend/migrations/093_agent_child_canary_reap_transport.down.sql" \
  "${project_dir}/backend/migrations/094_agent_cron_learning_activation.up.sql" \
  "${project_dir}/backend/migrations/094_agent_cron_learning_activation.down.sql" \
  "${project_dir}/backend/migrations/095_agent_product_canary_activation.up.sql" \
  "${project_dir}/backend/migrations/095_agent_product_canary_activation.down.sql" \
  "${isolated_root}/mm-chat/backend/migrations/"
cp "${project_dir}/frontend/src/store/storage/legacySkillRetirement.ts" \
  "${isolated_root}/mm-chat/frontend/src/store/storage/"
cp "${project_dir}/scripts/cutover-legacy-skills.sql" \
  "${isolated_root}/mm-chat/scripts/"
cp "${project_dir}/scripts/verify-agent-legacy-cutover.sh" \
  "${isolated_root}/mm-chat/scripts/"
(
  cd "${project_dir}"
  find docs -type f \( -name '*.md' -o -name '*.json' \) -print0 |
    while IFS= read -r -d '' relative_path; do
      mkdir -p "${isolated_root}/mm-chat/$(dirname -- "${relative_path}")"
      cp "${relative_path}" "${isolated_root}/mm-chat/${relative_path}"
    done
)
cp "${project_dir}/scripts/verify-agent-runtime-phase0.py" \
  "${isolated_root}/mm-chat/scripts/"
cp "${project_dir}/config/agent-runner/production-policy.json" \
  "${isolated_root}/mm-chat/config/agent-runner/"
cp "${project_dir}/scripts/evaluate-agent-production-closure.py" \
  "${project_dir}/scripts/verify-agent-production-closure.sh" \
  "${isolated_root}/mm-chat/scripts/"

cd "${isolated_root}"
"${python_command[@]}" \
  "${isolated_root}/mm-chat/scripts/verify-agent-runtime-phase0.py"
bash "${isolated_root}/mm-chat/scripts/verify-agent-production-closure.sh"
