#!/usr/bin/env bash
set -euo pipefail
umask 022

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
verifier="${script_dir}/verify-agent-runner-bundle.py"

usage() {
  cat >&2 <<'EOF'
Usage: build-agent-runner-bundle.sh --output DIR --release-commit HEX40
       [--evidence-class template|production] [--target-arch amd64|arm64]
       [--release-manifest FILE]

Builds one offline, content-addressed deployment artifact. It never installs,
starts, or configures the target host. The output path must not already exist.
EOF
}

output=""
release_commit=""
evidence_class="template"
target_arch="amd64"
release_manifest="${project_dir}/config/agent-runner/release-manifest.example.json"
while (($#)); do
  case "$1" in
    --output) [[ $# -ge 2 ]] || { usage; exit 2; }; output="$2"; shift 2 ;;
    --release-commit) [[ $# -ge 2 ]] || { usage; exit 2; }; release_commit="$2"; shift 2 ;;
    --evidence-class) [[ $# -ge 2 ]] || { usage; exit 2; }; evidence_class="$2"; shift 2 ;;
    --target-arch) [[ $# -ge 2 ]] || { usage; exit 2; }; target_arch="$2"; shift 2 ;;
    --release-manifest) [[ $# -ge 2 ]] || { usage; exit 2; }; release_manifest="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
done

[[ -n "${output}" && -n "${release_commit}" ]] || { usage; exit 2; }
[[ "${release_commit}" =~ ^[0-9a-f]{40}$ ]] || { echo 'Runner bundle: release commit must be lowercase HEX40' >&2; exit 2; }
[[ "${evidence_class}" == template || "${evidence_class}" == production ]] || { echo 'Runner bundle: invalid evidence class' >&2; exit 2; }
[[ "${target_arch}" == amd64 || "${target_arch}" == arm64 ]] || { echo 'Runner bundle: invalid target architecture' >&2; exit 2; }
[[ -f "${release_manifest}" && ! -L "${release_manifest}" ]] || { echo 'Runner bundle: release manifest must be a regular non-symlink file' >&2; exit 2; }
for command in go python3 cp chmod mkdir mv mktemp dirname; do
  command -v "${command}" >/dev/null 2>&1 || { echo "Runner bundle: ${command} is required" >&2; exit 1; }
done
[[ -f "${verifier}" && ! -L "${verifier}" ]] || { echo 'Runner bundle: verifier is unavailable' >&2; exit 1; }
go_version="$(cd "${backend_dir}" && GOTOOLCHAIN=auto GOPROXY=off go env GOVERSION)"
[[ "${go_version}" =~ ^go1\.25(\.[0-9]+)?$ ]] || { echo 'Runner bundle: Go 1.25 is required' >&2; exit 1; }
if [[ "${evidence_class}" == production ]]; then
  command -v git >/dev/null 2>&1 || { echo 'Runner bundle: git is required for production' >&2; exit 1; }
  [[ "$(git -C "${project_dir}" rev-parse HEAD)" == "${release_commit}" ]] || { echo 'Runner bundle: production commit does not match HEAD' >&2; exit 2; }
  [[ -z "$(git -C "${project_dir}" status --porcelain -- .)" ]] || { echo 'Runner bundle: production source tree must be clean' >&2; exit 2; }
fi

output_parent="$(dirname -- "${output}")"
output_name="$(basename -- "${output}")"
[[ "${output_name}" != . && "${output_name}" != .. && -n "${output_name}" ]] || { echo 'Runner bundle: unsafe output path' >&2; exit 2; }
mkdir -p -- "${output_parent}"
output_parent="$(cd -- "${output_parent}" && pwd -P)"
output="${output_parent}/${output_name}"
[[ ! -e "${output}" && ! -L "${output}" ]] || { echo 'Runner bundle: output path already exists' >&2; exit 2; }

work_dir="$(mktemp -d "${output_parent}/.neo-agent-runner-bundle.XXXXXX")"
cleanup() {
  if [[ -d "${work_dir}" ]]; then
    find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true
    rmdir "${work_dir}" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

mkdir -p "${work_dir}/bin" "${work_dir}/config" "${work_dir}/systemd"
chmod 0755 "${work_dir}/bin" "${work_dir}/config" "${work_dir}/systemd"
(
  cd "${backend_dir}"
  CGO_ENABLED=0 GOOS=linux GOARCH="${target_arch}" GOTOOLCHAIN=auto GOPROXY=off GOFLAGS=-mod=readonly \
    go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' \
    -o "${work_dir}/bin/neo-runnerd" ./cmd/neo-runnerd
  CGO_ENABLED=0 GOOS=linux GOARCH="${target_arch}" GOTOOLCHAIN=auto GOPROXY=off GOFLAGS=-mod=readonly \
    go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' \
    -o "${work_dir}/bin/neo-runner-probe" ./cmd/neo-runner-probe
)
cp -- "${release_manifest}" "${work_dir}/config/release-manifest.json"
cp -- "${project_dir}/config/agent-runner/seccomp-agent-v1.json" "${work_dir}/config/seccomp-agent-v1.json"
cp -- "${project_dir}/deploy/agent-runner/neo-runnerd.env.example" "${work_dir}/systemd/neo-runnerd.env.example"
cp -- "${project_dir}/deploy/agent-runner/neo-runnerd.service" "${work_dir}/systemd/neo-runnerd.service"
cp -- "${project_dir}/deploy/agent-runner/README.md" "${work_dir}/README.md"
chmod 0755 "${work_dir}/bin/neo-runnerd" "${work_dir}/bin/neo-runner-probe"
chmod 0644 "${work_dir}/config/release-manifest.json" "${work_dir}/config/seccomp-agent-v1.json" \
  "${work_dir}/systemd/neo-runnerd.env.example" "${work_dir}/systemd/neo-runnerd.service"
chmod 0644 "${work_dir}/README.md"

python3 - "${work_dir}" "${evidence_class}" "${release_commit}" "${go_version}" "${target_arch}" <<'PY'
import hashlib
import json
import pathlib
import stat
import sys

root = pathlib.Path(sys.argv[1])
evidence_class, release_commit, go_version, target_arch = sys.argv[2:]
paths = (
    "README.md",
    "bin/neo-runner-probe",
    "bin/neo-runnerd",
    "config/release-manifest.json",
    "config/seccomp-agent-v1.json",
    "systemd/neo-runnerd.env.example",
    "systemd/neo-runnerd.service",
)
files = []
for relative in paths:
    path = root / relative
    raw = path.read_bytes()
    files.append({
        "path": relative,
        "mode": f"{stat.S_IMODE(path.stat().st_mode):04o}",
        "size": len(raw),
        "sha256": "sha256:" + hashlib.sha256(raw).hexdigest(),
    })
manifest = {
    "schemaVersion": "neo.agent-runner-bundle/v1",
    "evidenceClass": evidence_class,
    "release": {
        "gitCommit": release_commit,
        "migrationHead": 90,
        "goVersion": go_version,
        "targetOS": "linux",
        "targetArch": target_arch,
    },
    "files": files,
}
canonical = json.dumps(manifest, ensure_ascii=True, separators=(",", ":"), sort_keys=True).encode()
manifest["inventorySha256"] = "sha256:" + hashlib.sha256(
    b"neo-agent-runner-bundle-v1\0" + canonical
).hexdigest()
(root / "neo-agent-runner-bundle.json").write_text(
    json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8"
)
PY
chmod 0644 "${work_dir}/neo-agent-runner-bundle.json"

set +e
verification="$(python3 "${verifier}" --bundle "${work_dir}" --expected-class "${evidence_class}")"
verification_status=$?
set -e
if [[ "${verification_status}" -ne 0 ]]; then
  printf '%s\n' "${verification}" >&2
  exit "${verification_status}"
fi
mv -- "${work_dir}" "${output}"
trap - EXIT INT TERM
printf 'Runner bundle built: %s\n' "${output}"
