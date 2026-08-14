#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
backend_dir="${project_dir}/backend"

cd "${backend_dir}"
go test -race ./internal/agentlearning ./internal/skillsupply ./internal/agentorchestrator ./internal/migration
go vet ./internal/agentlearning ./internal/skillsupply ./internal/agentorchestrator

bash "${project_dir}/scripts/verify-agent-runtime-phase0.sh"

python3 - "${project_dir}" <<'PY'
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
schema = json.loads(
    (root / "docs/contracts/schemas/neo-skill-draft.schema.json").read_text()
)
assert schema["properties"]["schemaVersion"]["const"] == "neo.skill-draft/v1"
assert schema["properties"]["tests"]["minItems"] == 1
assert schema["properties"]["evidence"]["minItems"] == 2
assert schema["properties"]["changedPaths"]["maxItems"] == 2048

for path in (
    root / "backend/internal/agentlearning/README.md",
    root / "backend/internal/agentlearning/DESIGN.md",
    root / "backend/migrations/089_agent_draft_learning.up.sql",
    root / "backend/migrations/089_agent_draft_learning.down.sql",
    root / "scripts/verify-agent-learning-postgres17.sh",
    root / "docs/contracts/fixtures/agent-runtime/neo-skill-draft.valid.json",
    root / "docs/contracts/fixtures/agent-runtime/neo-skill-draft.invalid.json",
):
    assert path.is_file() and path.stat().st_size > 0

migration = (root / "backend/migrations/089_agent_draft_learning.up.sql").read_text()
for signature in (
    "CREATE FUNCTION agent_learning_create_draft(",
    "CREATE FUNCTION agent_learning_claim_checks(",
    "CREATE FUNCTION agent_learning_complete_checks(",
    "CREATE FUNCTION agent_learning_promote(",
    "CREATE FUNCTION agent_learning_claim_cleanup(",
    "CREATE FUNCTION agent_learning_reconcile(",
    "CREATE FUNCTION agent_learning_prune(",
):
    assert signature in migration
assert "kind IN ('static','isolation','evaluation')" in migration
assert "TO agent_learning_control" in migration

allowed_importers = {
    "backend/cmd/api/main.go",
    "backend/internal/agentcontrol/handler.go",
    "backend/internal/agentcontrol/handler_test.go",
    "backend/internal/agentcontrol/repository_postgres.go",
    "backend/internal/agentcontrol/service.go",
    "backend/internal/agentcontrol/service_test.go",
    "backend/internal/agentcontrol/types.go",
}
unexpected_importers = []
for path in (root / "backend").rglob("*.go"):
    relative = path.relative_to(root).as_posix()
    if relative.startswith("backend/internal/agentlearning/"):
        continue
    if 'backend/internal/agentlearning"' in path.read_text() and relative not in allowed_importers:
        unexpected_importers.append(relative)
assert unexpected_importers == []

api_main = (root / "backend/cmd/api/main.go").read_text()
assert "agentlearning.WithLearningEnabled(false)" in api_main
assert "agentLearningService.Claim" not in api_main
assert "agentLearningService.Reconcile" not in api_main
assert "agentLearningService.Cleanup" not in api_main
assert "agentLearningService.Prune" not in api_main
print("Agent Learning source verification: quarantine, exact checks, human Promote, cleanup, and held wiring passed")
PY

echo "Agent Learning source verification: passed (production Learning and exact-host execution remain disabled)"
