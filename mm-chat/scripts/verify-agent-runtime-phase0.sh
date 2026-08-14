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

schema_dir="${project_dir}/docs/contracts/schemas"
fixture_dir="${project_dir}/docs/contracts/fixtures/agent-runtime"

for path in "${schema_dir}"/*.json "${fixture_dir}"/*.json; do
  jq empty "${path}" >/dev/null
done

bash -n "${project_dir}/scripts/verify-agent-runtime-phase0.sh"
bash -n "${project_dir}/scripts/verify-agent-production-closure.sh"
bash -n "${project_dir}/scripts/verify-agent-runtime-g21-0.sh"

bash "${project_dir}/scripts/verify-agent-runtime-g21-0.sh"

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
