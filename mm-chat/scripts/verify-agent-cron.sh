#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
backend_dir="${project_dir}/backend"

cd "${backend_dir}"
go test -race ./internal/agentcron ./internal/agentbroker ./internal/agentorchestrator ./internal/migration
go vet ./internal/agentcron ./internal/agentbroker ./internal/agentorchestrator

bash "${project_dir}/scripts/verify-agent-runtime-phase0.sh"

python3 - "${project_dir}" <<'PY'
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
schema = json.loads(
    (root / "docs/contracts/schemas/neo-cron-template.schema.json").read_text()
)
schedule = schema["$defs"]["schedule"]["properties"]
policies = schema["$defs"]["policies"]["properties"]
assert schedule["expression"]["pattern"]
assert schedule["calculator"]["const"] == "robfig-cron/v3.0.1+go-tzdata"
assert policies["maxCatchupRuns"]["maximum"] == 100
assert policies["catchupWindowSeconds"]["maximum"] == 86400

for path in (
    root / "backend/internal/agentcron/README.md",
    root / "backend/internal/agentcron/DESIGN.md",
    root / "backend/migrations/088_agent_cron_foundation.up.sql",
    root / "backend/migrations/088_agent_cron_foundation.down.sql",
    root / "scripts/verify-agent-cron-postgres17.sh",
):
    assert path.is_file() and path.stat().st_size > 0

migration = (root / "backend/migrations/088_agent_cron_foundation.up.sql").read_text()
for signature in (
    "CREATE FUNCTION agent_cron_create_revision(",
    "CREATE FUNCTION agent_cron_claim_due(",
    "CREATE FUNCTION agent_cron_advance_cursor(",
    "CREATE FUNCTION agent_cron_claim_triggers(",
    "CREATE FUNCTION agent_cron_enqueue_trigger(",
    "CREATE FUNCTION agent_cron_reconcile(",
    "CREATE FUNCTION agent_cron_prune(",
):
    assert signature in migration
assert "GRANT EXECUTE ON FUNCTION agent_cron_create_revision" in migration
assert "TO agent_cron_control" in migration
print("Agent Cron source verification: strict schedule, durable claims, authority, and held boundaries passed")
PY

echo "Agent Cron source verification: passed (production Scheduler promotion not implied)"
