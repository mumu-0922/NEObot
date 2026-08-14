#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
backend_dir="${project_dir}/backend"

cd "${backend_dir}"
go test -race ./internal/agentdelegation ./internal/agentbroker ./internal/agentrunner ./internal/agentorchestrator ./internal/migration
go vet ./internal/agentdelegation ./internal/agentbroker ./internal/agentrunner ./internal/agentorchestrator

python3 - "${project_dir}" <<'PY'
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
schema = json.loads((root / "docs/contracts/schemas/neo-runner-rpc.schema.json").read_text())
lineage = schema["$defs"]["runLineage"]
assert lineage["properties"]["depth"]["maximum"] == 1
for fixture in ("neo-runner-rpc.valid.json", "neo-runner-rpc.invalid.json"):
    body = json.loads((root / "docs/contracts/fixtures/agent-runtime" / fixture).read_text())["body"]
    assert body["lineage"]["depth"] == 1
for path in (
    root / "backend/internal/agentdelegation/README.md",
    root / "backend/internal/agentdelegation/DESIGN.md",
    root / "backend/migrations/087_agent_child_delegation.up.sql",
    root / "backend/migrations/087_agent_child_delegation.down.sql",
):
    assert path.is_file() and path.stat().st_size > 0
migration = (root / "backend/migrations/087_agent_child_delegation.up.sql").read_text()
assert "CREATE FUNCTION agent_delegation_reconcile(" in migration
assert "CREATE FUNCTION agent_delegation_registry_subset(" in migration
assert "starts_with(child_resource,parent_resource)" in migration
assert "p_subject->>'userId' IS DISTINCT FROM p_user_id::TEXT" in migration
assert "SETTLEMENT_INVALID" in migration
assert "PARENT_AUTHORITY_STALE" in migration
print("Agent delegation source verification: depth/subset/settlement/recovery/held boundaries passed")
PY

echo "Agent delegation source verification: passed (production promotion not implied)"
