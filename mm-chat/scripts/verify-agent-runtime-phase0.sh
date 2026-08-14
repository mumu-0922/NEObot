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

required_g20_5_paths=(
  "${project_dir}/backend/internal/agentdelegation/service.go"
  "${project_dir}/backend/internal/agentdelegation/repository_postgres.go"
  "${project_dir}/backend/migrations/087_agent_child_delegation.up.sql"
  "${project_dir}/backend/migrations/087_agent_child_delegation.down.sql"
  "${project_dir}/scripts/verify-agent-delegation.sh"
  "${project_dir}/scripts/verify-agent-delegation-postgres17.sh"
)
for path in "${required_g20_5_paths[@]}"; do
  if [[ ! -s "${path}" ]]; then
    echo "Agent Runtime Phase 0 verification: missing G20.5 artifact ${path}" >&2
    exit 1
  fi
done
grep -Fq "type RunLineage struct" "${project_dir}/backend/internal/agentrunner/types.go"
grep -Fq "CREATE FUNCTION agent_delegation_reconcile(" \
  "${project_dir}/backend/migrations/087_agent_child_delegation.up.sql"
grep -Fq "DROP FUNCTION agent_delegation_reconcile(INTEGER)" \
  "${project_dir}/backend/migrations/087_agent_child_delegation.down.sql"

schema_dir="${project_dir}/docs/contracts/schemas"
fixture_dir="${project_dir}/docs/contracts/fixtures/agent-runtime"

for path in "${schema_dir}"/*.json "${fixture_dir}"/*.json; do
  jq empty "${path}" >/dev/null
done

bash -n "${project_dir}/scripts/verify-agent-runtime-phase0.sh"

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
  "${isolated_root}/mm-chat/docs" \
  "${isolated_root}/mm-chat/scripts"

cp "${project_dir}/backend/internal/codejobs/handler.go" \
  "${project_dir}/backend/internal/codejobs/service.go" \
  "${isolated_root}/mm-chat/backend/internal/codejobs/"
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

cd "${isolated_root}"
"${python_command[@]}" \
  "${isolated_root}/mm-chat/scripts/verify-agent-runtime-phase0.py"
