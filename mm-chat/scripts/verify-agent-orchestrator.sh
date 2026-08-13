#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"

for command in go bash rg; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    echo "Agent Orchestrator verification: ${command} is required" >&2
    exit 1
  fi
done

bash -n "${project_dir}/scripts/verify-agent-orchestrator.sh"
bash -n "${project_dir}/scripts/verify-agent-orchestrator-postgres17.sh"

(cd "${project_dir}/backend" && \
  go test -count=1 -race ./internal/agentorchestrator ./internal/migration ./internal/codejobs && \
  go vet ./internal/agentorchestrator ./internal/migration ./internal/codejobs)

bash "${project_dir}/scripts/verify-agent-runtime-phase0.sh"

if rg -n 'agentorchestrator|agent_orchestrator' \
  "${project_dir}/backend/cmd/api" "${project_dir}/backend/internal/httpserver" \
  "${project_dir}/frontend/src" >/dev/null; then
  echo "Agent Orchestrator verification: G20.2 gained forbidden API/frontend wiring" >&2
  exit 1
fi
if find "${project_dir}/backend/cmd" -maxdepth 1 -type d -name '*runner*' \
  | grep -Fq 'neo-runner'; then
  echo "Agent Orchestrator verification: neo-runnerd must not exist in G20.2" >&2
  exit 1
fi

printf '%s\n' \
  "Agent Orchestrator verification: passed (state machine, sanitized snapshot/events, opaque lease credentials, migration authority, Phase 0 contracts)" \
  "Agent Orchestrator verification: production Runtime remains unavailable; no Runner or Sandbox was launched"
