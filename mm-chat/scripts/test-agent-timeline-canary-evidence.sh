#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
VERIFIER="${SCRIPT_DIR}/verify-agent-timeline-canary-evidence.py"
TEMPLATE="${SCRIPT_DIR}/../docs/deployment/agent-timeline-canary-evidence.example.json"
WORK_DIR="$(mktemp -d)"
trap 'rm -rf "${WORK_DIR}"' EXIT

if output="$(python3 "${VERIFIER}" "${TEMPLATE}" 2>&1)"; then
  printf 'committed canary template must not be eligible\n' >&2
  exit 1
fi
if [[ "${output}" != "agent timeline canary evidence invalid: templateOnly" ]]; then
  printf 'unexpected template rejection: %s\n' "${output}" >&2
  exit 1
fi

VALID="${WORK_DIR}/valid.json"
python3 - "${TEMPLATE}" "${VALID}" <<'PY'
import json
import sys

source, destination = sys.argv[1:]
value = json.load(open(source, encoding="utf-8"))
value["templateOnly"] = False
value["candidate"]["commit"] = "0123456789abcdef0123456789abcdef01234567"
value["candidate"]["backendImage"] = (
    "registry.example/mm-chat/backend@sha256:" + "0123456789abcdef" * 4
)
value["candidate"]["frontendImage"] = (
    "registry.example/mm-chat/frontend@sha256:" + "fedcba9876543210" * 4
)
json.dump(value, open(destination, "w", encoding="utf-8"))
PY

REPORT="${WORK_DIR}/report.json"
python3 "${VERIFIER}" "${VALID}" --report "${REPORT}"
python3 - "${REPORT}" <<'PY'
import json
import os
import stat
import sys

path = sys.argv[1]
report = json.load(open(path, encoding="utf-8"))
assert report["agentTurns"] == 5
assert report["canaryUserCount"] == 1
assert report["candidateCommit"] == "0123456789abcdef0123456789abcdef01234567"
assert report["controlUserCount"] == 1
assert report["durableReloadRatio"] == 1.2
assert report["schemaVersion"] == 1
assert report["securityLeakCount"] == 0
assert report["status"] == "eligible"
assert report["toolCalls"] == 8
assert report["visibleUpdateP95Ms"] == 200.0
assert report["windowDurationSeconds"] == 1800
assert report["windowStartedAt"] == "2026-08-20T11:30:00Z"
assert report["windowEndedAt"] == "2026-08-20T12:00:00Z"
assert report["backendImage"].endswith("@sha256:" + "0123456789abcdef" * 4)
assert report["frontendImage"].endswith("@sha256:" + "fedcba9876543210" * 4)
assert len(report["evidenceSha256"]) == 64
int(report["evidenceSha256"], 16)
assert stat.S_IMODE(os.stat(path).st_mode) == 0o600
PY

mutate_and_reject() {
  local name="$1"
  local mutation="$2"
  local expected="$3"
  local candidate="${WORK_DIR}/${name}.json"
  python3 - "${VALID}" "${candidate}" "${mutation}" <<'PY'
import json
import sys

source, destination, mutation = sys.argv[1:]
value = json.load(open(source, encoding="utf-8"))
if mutation == "invalid_window":
    value["window"]["startedAt"] = value["window"]["endedAt"]
elif mutation == "control_exposure":
    value["rollout"]["controlAgentEventsExposed"] = True
elif mutation == "visible_latency":
    value["performance"]["visibleUpdateP95Ms"] = 301
elif mutation == "reload_ratio":
    value["performance"]["durableReloadP95Ms"] = 120.01
elif mutation == "missing_probe":
    value["security"]["probeTypes"].remove("artifact_authorization")
elif mutation == "raw_content":
    value["content"] = "must-not-be-accepted"
else:
    raise SystemExit("unknown test mutation")
json.dump(value, open(destination, "w", encoding="utf-8"))
PY
  local output
  if output="$(python3 "${VERIFIER}" "${candidate}" 2>&1)"; then
    printf 'expected verifier failure for %s\n' "${name}" >&2
    exit 1
  fi
  if [[ "${output}" != "agent timeline canary evidence invalid: ${expected}" ]]; then
    printf 'unexpected verifier error for %s: %s\n' "${name}" "${output}" >&2
    exit 1
  fi
}

mutate_and_reject invalid-window invalid_window window.duration
mutate_and_reject control-exposure control_exposure rollout.controlAgentEventsExposed
mutate_and_reject visible-latency visible_latency performance.visibleUpdateP95Ms.limit
mutate_and_reject reload-ratio reload_ratio performance.durableReloadRatio
mutate_and_reject missing-probe missing_probe security.probeTypes.required
mutate_and_reject raw-content raw_content evidence.shape

printf 'agent timeline canary evidence verifier: ok\n'
