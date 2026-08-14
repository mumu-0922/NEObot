#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
builder="${script_dir}/build-agent-runner-bundle.sh"
verifier="${script_dir}/verify-agent-runner-bundle.py"
schema="${project_dir}/docs/contracts/schemas/neo-agent-runner-bundle.schema.json"
release_commit=1111111111111111111111111111111111111111

for command in python3 cp chmod ln touch cmp; do
  command -v "${command}" >/dev/null 2>&1 || { echo "Agent Runner bundle verification: ${command} is required" >&2; exit 1; }
done
for path in "${builder}" "${verifier}" "${schema}"; do
  [[ -s "${path}" && ! -L "${path}" ]] || { echo 'Agent Runner bundle verification: artifact unavailable' >&2; exit 1; }
done

work_dir="$(mktemp -d)"
cleanup() {
  find "${work_dir}" -depth -mindepth 1 -delete 2>/dev/null || true
  rmdir "${work_dir}" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

python3 - "${schema}" <<'PY'
import json
import sys

schema = json.load(open(sys.argv[1], encoding="utf-8"))
assert schema["$schema"].endswith("2020-12/schema")
assert schema["additionalProperties"] is False
assert schema["properties"]["schemaVersion"]["const"] == "neo.agent-runner-bundle/v1"
assert schema["properties"]["files"]["minItems"] == 7
assert schema["properties"]["files"]["maxItems"] == 7
PY

bash "${builder}" --output "${work_dir}/bundle-a" --release-commit "${release_commit}" >/dev/null
bash "${builder}" --output "${work_dir}/bundle-b" --release-commit "${release_commit}" >/dev/null
python3 "${verifier}" --bundle "${work_dir}/bundle-a" --expected-class template >/dev/null
cmp "${work_dir}/bundle-a/neo-agent-runner-bundle.json" "${work_dir}/bundle-b/neo-agent-runner-bundle.json"
for relative in \
  README.md \
  bin/neo-runner-probe \
  bin/neo-runnerd \
  config/release-manifest.json \
  config/seccomp-agent-v1.json \
  systemd/neo-runnerd.env.example \
  systemd/neo-runnerd.service; do
  cmp "${work_dir}/bundle-a/${relative}" "${work_dir}/bundle-b/${relative}"
done

invalid_release_manifest="${work_dir}/invalid-release-manifest.json"
python3 - "${project_dir}/config/agent-runner/release-manifest.example.json" \
  "${invalid_release_manifest}" <<'PY'
import json
import pathlib
import sys

value = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))
value["networkMode"] = "bridge"
pathlib.Path(sys.argv[2]).write_text(json.dumps(value), encoding="utf-8")
PY
set +e
invalid_release_output="$(bash "${builder}" --output "${work_dir}/invalid-release-bundle" \
  --release-commit "${release_commit}" --release-manifest "${invalid_release_manifest}" 2>&1)"
invalid_release_status=$?
set -e
[[ "${invalid_release_status}" -eq 2 && ! -e "${work_dir}/invalid-release-bundle" && \
  "${invalid_release_output}" == *RELEASE_MANIFEST_INVALID* ]] || {
  echo 'Agent Runner bundle verification: invalid release manifest was accepted' >&2
  exit 1
}

assert_invalid() {
  local name="$1" expected="$2"
  local output status
  set +e
  output="$(python3 "${verifier}" --bundle "${work_dir}/${name}" 2>&1)"
  status=$?
  set -e
  [[ "${status}" -eq 2 && "${output}" == *"${expected}"* ]] || {
    echo "Agent Runner bundle verification: ${name} was not rejected as ${expected}" >&2
    exit 1
  }
}

cp -a "${work_dir}/bundle-a" "${work_dir}/size-drift"
printf x >>"${work_dir}/size-drift/config/seccomp-agent-v1.json"
assert_invalid size-drift BUNDLE_SIZE_DRIFT

cp -a "${work_dir}/bundle-a" "${work_dir}/hash-drift"
python3 - "${work_dir}/hash-drift/config/seccomp-agent-v1.json" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
raw = bytearray(path.read_bytes())
raw[-2] = ord(" ") if raw[-2] != ord(" ") else ord("\t")
path.write_bytes(raw)
PY
assert_invalid hash-drift BUNDLE_HASH_DRIFT

cp -a "${work_dir}/bundle-a" "${work_dir}/mode-drift"
chmod 0600 "${work_dir}/mode-drift/systemd/neo-runnerd.service"
assert_invalid mode-drift BUNDLE_MODE_DRIFT

cp -a "${work_dir}/bundle-a" "${work_dir}/extra-file"
touch "${work_dir}/extra-file/extra"
assert_invalid extra-file BUNDLE_FILE_SET_INVALID

cp -a "${work_dir}/bundle-a" "${work_dir}/missing-file"
find "${work_dir}/missing-file/systemd/neo-runnerd.env.example" -delete
assert_invalid missing-file BUNDLE_FILE_SET_INVALID

cp -a "${work_dir}/bundle-a" "${work_dir}/symlink"
find "${work_dir}/symlink/config/release-manifest.json" -delete
ln -s "${work_dir}/bundle-a/config/release-manifest.json" "${work_dir}/symlink/config/release-manifest.json"
assert_invalid symlink BUNDLE_SYMLINK_FORBIDDEN

set +e
false_production="$(bash "${builder}" --output "${work_dir}/false-production" \
  --release-commit "${release_commit}" --evidence-class production 2>&1)"
false_production_status=$?
set -e
[[ "${false_production_status}" -ne 0 && ! -e "${work_dir}/false-production" ]] || {
  echo 'Agent Runner bundle verification: false production promotion was accepted' >&2
  exit 1
}
[[ -n "${false_production}" ]] || {
  echo 'Agent Runner bundle verification: false production rejection was not diagnosable' >&2
  exit 1
}

printf '%s\n' 'Agent Runner exact-host bundle verification: passed (template only; production not implied).'
