#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
schema="${project_dir}/docs/contracts/schemas/neo-agent-runner-local-test-report.schema.json"
valid="${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-runner-local-test-report.valid.json"
invalid="${project_dir}/docs/contracts/fixtures/agent-runtime/neo-agent-runner-local-test-report.invalid.json"
temporary="$(mktemp -d)"
trap 'rm -rf -- "${temporary}"' EXIT

PYTHONPATH="${script_dir}" python3 "${script_dir}/agent_runner_local_test_test.py"
python3 -m py_compile "${script_dir}/agent_runner_local_test.py" "${script_dir}/manage-agent-runner-local-test.py"
python3 - "${schema}" "${valid}" "${invalid}" <<'PY'
import json
import sys

schema = json.load(open(sys.argv[1], encoding="utf-8"))
valid = json.load(open(sys.argv[2], encoding="utf-8"))
invalid = json.load(open(sys.argv[3], encoding="utf-8"))
required = set(schema["required"])
assert schema["$schema"].endswith("2020-12/schema")
assert schema["additionalProperties"] is False
assert set(valid) == required
assert valid["schemaVersion"] == schema["properties"]["schemaVersion"]["const"]
assert valid["evidenceClass"] == schema["properties"]["evidenceClass"]["const"]
assert valid["productionEligible"] is False
assert valid["outcome"] == "LOCAL_SKILL_SMOKE_PASSED"
assert len(valid["checks"]) == 8 and len(set(valid["checks"])) == 8
assert set(valid["checks"]) == set(schema["properties"]["checks"]["items"]["enum"])
assert invalid["evidenceClass"] != "local_test" or invalid["productionEligible"] is not False
PY

set +e
production_output="$(python3 "${script_dir}/evaluate-agent-production-closure.py" --record "${valid}" 2>&1)"
production_status=$?
set -e
[[ "${production_status}" -eq 2 && "${production_output}" == *PROMOTION_EVIDENCE_INVALID* && \
  "${production_output}" != *'"verdict":"PROMOTION_READY"'* ]] || {
  echo 'Agent Runner local-test verification: production evaluator accepted local evidence' >&2
  exit 1
}

(
  cd "${project_dir}/backend"
  go test ./internal/agentrunner ./cmd/agent-runtime-local-test-smoke ./cmd/neo-skill-local-test-workload
  CGO_ENABLED=0 go build -buildvcs=false -o "${temporary}/neo-skill-local-test-workload" ./cmd/neo-skill-local-test-workload
)
python3 - "${temporary}/neo-skill-local-test-workload" <<'PY'
import struct
import sys

raw = open(sys.argv[1], "rb").read(64)
assert raw[:4] == b"\x7fELF" and raw[4] == 2 and raw[5] == 1
program_offset = struct.unpack_from("<Q", raw, 32)[0]
program_entry_size = struct.unpack_from("<H", raw, 54)[0]
program_count = struct.unpack_from("<H", raw, 56)[0]
with open(sys.argv[1], "rb") as executable:
    executable.seek(program_offset)
    headers = executable.read(program_entry_size * program_count)
assert all(
    struct.unpack_from("<I", headers, index * program_entry_size)[0] != 3
    for index in range(program_count)
), "local-test workload unexpectedly requires a host dynamic linker"
PY

set +e
host_output="$(bash "${script_dir}/verify-agent-runner-host.sh" 2>&1)"
host_status=$?
set -e
[[ "${host_status}" -ne 0 && "${host_output}" == *ISOLATION_UNAVAILABLE* ]] || {
  echo 'Agent Runner local-test verification: production host gate changed state' >&2
  exit 1
}

status_output="$(python3 "${script_dir}/manage-agent-runner-local-test.py" status --json)"
[[ "${status_output}" == *'"evidenceClass": "local_test"'* && \
  "${status_output}" == *'"productionEligible": false'* ]] || {
  echo 'Agent Runner local-test verification: local status lost its evidence boundary' >&2
  exit 1
}

printf '%s\n' 'Agent Runner local-test verification: passed (offline controls; no host mutation or production promotion).'
