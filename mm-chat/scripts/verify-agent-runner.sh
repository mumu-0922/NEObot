#!/usr/bin/env bash
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$PROJECT_DIR/backend"

cd "$BACKEND_DIR"
go test -race ./internal/agentrunner ./cmd/neo-runnerd ./cmd/neo-runner-probe ./internal/migration
go vet ./internal/agentrunner ./cmd/neo-runnerd ./cmd/neo-runner-probe

python3 - "$PROJECT_DIR" <<'PY'
import hashlib
import json
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
manifest = json.loads((root / "config/agent-runner/release-manifest.example.json").read_text())
assert manifest["schemaVersion"] == "neo.agent-runner-release/v1"
assert manifest["approved"] is False
assert manifest["networkMode"] == "none"
assert manifest["requiredControllers"] == ["cpu", "memory", "pids"]
assert manifest["isolationAcceptance"]["path"].startswith("/")
assert manifest["isolationAcceptance"]["sha256"].startswith("sha256:")
names = {item["name"] for item in manifest["binaries"]}
assert {"podman", "crun", "conmon", "newuidmap", "newgidmap"} <= names
assert all(item["path"].startswith("/") and item["sha256"].startswith("sha256:") for item in manifest["binaries"])

seccomp = json.loads((root / "config/agent-runner/seccomp-agent-v1.json").read_text())
assert seccomp["defaultAction"] == "SCMP_ACT_ERRNO"
allowed = {name for group in seccomp["syscalls"] for name in group["names"]}
for forbidden in ("bpf", "mount", "pivot_root", "ptrace", "reboot", "setns", "unshare"):
    assert forbidden not in allowed

service = (root / "deploy/agent-runner/neo-runnerd.service").read_text()
for anchor in ("User=neo-runner", "Delegate=yes", "NoNewPrivileges=no", "CapabilityBoundingSet=CAP_SETUID CAP_SETGID", "AmbientCapabilities=", "ProtectSystem=strict", "ProtectControlGroups=no", "RestrictNamespaces=cgroup ipc net mnt pid user uts"):
    assert anchor in service
for forbidden in ("User=root", "sudo", "docker.sock", "--privileged"):
    assert forbidden not in service

schema = json.loads((root / "docs/contracts/schemas/neo-runner-rpc.schema.json").read_text())
refs = {item["$ref"] for item in schema["oneOf"]}
assert "#/$defs/listRequest" in refs and "#/$defs/listResponse" in refs
assert "#/$defs/reconcileRequest" in refs and "#/$defs/reconcileResponse" in refs
probe_features = schema["$defs"]["probeRequest"]["allOf"][1]["properties"]["body"]["properties"]["requiredFeatures"]["items"]["enum"]
for feature in ("snapshot_workspace", "network_none", "subordinate_ids", "cgroup_reap"):
    assert feature in probe_features
assert "error" in schema["$defs"]["probeResponse"]["allOf"][1]["properties"]["body"]["properties"]
print("Agent Runner source verification: release/protocol/seccomp/systemd contracts passed")
PY

bash "$PROJECT_DIR/scripts/verify-agent-runtime-phase0.sh"
echo "Agent Runner source verification: passed (production readiness not implied)"
