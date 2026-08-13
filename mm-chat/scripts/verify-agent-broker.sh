#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
backend_dir="${project_dir}/backend"

cd "${backend_dir}"
go test -race ./internal/agentbroker ./internal/safenet ./internal/mcpclient ./internal/agentrunner ./internal/migration
go vet ./internal/agentbroker ./internal/safenet ./internal/mcpclient ./internal/agentrunner

python3 - "${project_dir}" <<'PY'
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
schema = json.loads((root / "docs/contracts/schemas/neo-runner-rpc.schema.json").read_text())
refs = {entry["$ref"] for entry in schema["oneOf"]}
for name in ("prepareRequest", "prepareResponse", "commitRequest", "commitResponse"):
    assert f"#/$defs/{name}" in refs
methods = set(schema["$defs"]["authority"]["properties"]["method"]["enum"])
assert {"prepare", "commit"} <= methods
for fixture in (
    "neo-runner-rpc.prepare.valid.json",
    "neo-runner-rpc.commit.valid.json",
):
    json.loads((root / "docs/contracts/fixtures/agent-runtime" / fixture).read_text())

for path in (
    root / "backend/internal/agentbroker/README.md",
    root / "backend/internal/agentbroker/DESIGN.md",
    root / "backend/migrations/086_agent_broker_foundation.up.sql",
    root / "backend/migrations/086_agent_broker_foundation.down.sql",
):
    assert path.is_file() and path.stat().st_size > 0
print("Agent Broker source verification: contracts and held boundaries passed")
PY

echo "Agent Broker source verification: passed (production promotion not implied)"
