#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"

(
  cd "${project_dir}/backend"
  go test -race ./internal/agentcontrol ./internal/agentbroker ./internal/agentcron ./internal/agentlearning ./internal/migration
  go vet ./internal/agentcontrol ./internal/agentbroker ./internal/agentcron ./internal/agentlearning
)

(
  cd "${project_dir}/frontend"
  corepack pnpm exec vitest run \
    src/__tests__/serverAgentCenterApi.test.ts \
    src/__tests__/chatPanelUrlState.test.ts \
    src/__tests__/legacySkillCutover.test.ts \
    src/__tests__/agentCenterComposition.test.ts
  corepack pnpm typecheck
)

python3 - "${project_dir}" <<'PY'
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
required = (
    "backend/internal/agentcontrol/service.go",
    "backend/internal/agentcontrol/repository_postgres.go",
    "backend/internal/agentcontrol/handler.go",
    "backend/migrations/090_agent_product_shadow.up.sql",
    "backend/migrations/090_agent_product_shadow.down.sql",
    "frontend/src/components/agent/AgentCenter.tsx",
    "frontend/src/lib/skills/legacyCutover.ts",
    "frontend/src/services/api/client/server/agentCenterApi.ts",
    "scripts/verify-agent-product-shadow-postgres17.sh",
)
for relative in required:
    path = root / relative
    assert path.is_file() and path.stat().st_size > 0, relative

migration = (root / "backend/migrations/090_agent_product_shadow.up.sql").read_text()
for signature in (
    "CREATE VIEW agent_product_runs",
    "CREATE FUNCTION agent_product_get_artifact(",
    "CREATE FUNCTION agent_product_cancel_run(",
    "CREATE FUNCTION agent_product_shadow_snapshot(",
    "CREATE FUNCTION agent_product_append_shadow_observation(",
    "mode IN ('synthetic','read_only')",
    "'ISOLATION_UNAVAILABLE'",
):
    assert signature in migration, signature
for forbidden in (
    "GRANT INSERT ON agent_shadow_",
    "GRANT UPDATE ON agent_shadow_",
    "GRANT DELETE ON agent_shadow_",
    "GRANT EXECUTE ON FUNCTION agent_effect_claim_commit",
    "GRANT EXECUTE ON FUNCTION agent_cron_claim_due",
    "GRANT EXECUTE ON FUNCTION agent_learning_claim_checks",
):
    assert forbidden.lower() not in migration.lower(), forbidden

service = (root / "backend/internal/agentcontrol/service.go").read_text()
assert "return ErrIsolationUnavailable" in service
assert "shadowAdapter.Observe" in service
assert "os/exec" not in service and "podman" not in service.lower()

legacy = (root / "frontend/src/lib/skills/legacyCutover.ts").read_text()
assert "dryRun: true" in legacy
assert "deleteStorageKeys: []" in legacy
assert "removeItem(" not in legacy and "clear(" not in legacy
print("Agent product/Shadow source verification: bounded facade, held Runtime, content-free inventory, and no-delete preparation passed")
PY

bash "${project_dir}/scripts/verify-agent-runtime-phase0.sh"
echo "Agent product/Shadow source verification: passed"
