#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"

for command in go bash jq; do
  if ! command -v "${command}" >/dev/null 2>&1; then
    echo "Skill supply verification: ${command} is required" >&2
    exit 1
  fi
done

bash -n "${project_dir}/scripts/verify-skill-supply-chain.sh"
bash -n "${project_dir}/scripts/verify-skill-supply-chain-postgres17.sh"
jq empty \
  "${project_dir}/docs/contracts/schemas/neo-skill-runtime-manifest.schema.json" \
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-skill-runtime-manifest.valid.json" \
  "${project_dir}/docs/contracts/fixtures/agent-runtime/neo-skill-runtime-manifest.invalid.json"

(cd "${project_dir}/backend" && \
  go test -count=1 -race ./internal/skillsupply ./internal/mcpclient ./internal/migration ./internal/codejobs && \
  go vet ./internal/skillsupply ./internal/mcpclient ./internal/migration ./internal/codejobs)

bash "${project_dir}/scripts/verify-agent-local-runtime.sh"

printf '%s\n' \
  "Skill supply verification: passed (malicious archives, deterministic identities/SBOM, exact sources, strict API, local_direct runtime)" \
  "Skill supply verification: installed Skills execute only through the bounded local workspace"
