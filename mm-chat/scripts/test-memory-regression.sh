#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
compose_file="${project_dir}/compose.memory-regression.yml"
runner_script="${script_dir}/run-memory-regression.sh"
docker_bin="${DOCKER_BIN:-docker}"

if ! "${docker_bin}" compose version >/dev/null 2>&1; then
  echo "Memory regression topology: Docker Compose is required" >&2
  exit 1
fi

temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/mm-chat-memory-regression-test.XXXXXX")"
trap 'find "${temp_dir}" -depth -delete' EXIT
chmod 700 "${temp_dir}"
fixture_root="${temp_dir}/regression-fixtures"
cost_file="${temp_dir}/cost.json"
credential_file="${temp_dir}/credential"
render_output="${temp_dir}/render-output"
env_file="${temp_dir}/render.env"
rendered="${temp_dir}/compose.json"
mkdir -p "${fixture_root}" "${render_output}"
chmod 700 "${fixture_root}" "${render_output}"
printf '{"fixtures":[]}\n' >"${fixture_root}/fixtures.json"
printf '{"cases":[]}\n' >"${fixture_root}/corpus.json"
printf '{}\n' >"${fixture_root}/audit.json"
printf '{}\n' >"${fixture_root}/manifest.json"
printf '{"schemaVersion":"protocol-cost"}\n' >"${cost_file}"
printf 'fixture-live-credential-not-used\n' >"${credential_file}"
chmod 600 "${fixture_root}"/*.json "${cost_file}" "${credential_file}"

cat >"${env_file}" <<EOF
MEMORY_REGRESSION_DB_PASSWORD=fixture-render-password
MEMORY_REGRESSION_ROOT_PATH=${fixture_root}
MEMORY_REGRESSION_COST_BASIS_PATH=${cost_file}
MEMORY_REGRESSION_CREDENTIAL_PATH=${credential_file}
MEMORY_REGRESSION_OUTPUT_PATH=${render_output}
MEMORY_REGRESSION_RUN_ID=memory-regression-static
MEMORY_REGRESSION_LIVE_APPROVAL=I_UNDERSTAND_THIS_USES_REAL_SILICONFLOW_QUOTA
EOF
chmod 600 "${env_file}"

"${docker_bin}" compose \
  --project-name mmchat-memory-regression-static \
  --project-directory "${project_dir}" \
  --env-file "${env_file}" \
  -f "${compose_file}" \
  --profile memory-regression-fake \
  --profile memory-regression-live \
  config --format json >"${rendered}"

python3 - "${rendered}" "${fixture_root}" "${cost_file}" "${credential_file}" "${render_output}" <<'PY'
import json
import sys
from pathlib import Path

config = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
fixture_root, cost, credential, output = map(lambda value: Path(value).resolve(), sys.argv[2:6])
services = config.get("services", {})
expected = {
    "memory-regression-postgres",
    "memory-regression-migrate",
    "memory-regression-fake-runner",
    "memory-regression-live-runner",
}
if set(services) != expected:
    raise SystemExit(f"Memory regression topology: service drift: {sorted(services)}")

for name, service in services.items():
    if service.get("ports"):
        raise SystemExit(f"Memory regression topology: {name} publishes a host port")
    if not service.get("mem_limit") or not service.get("cpus") or not service.get("pids_limit"):
        raise SystemExit(f"Memory regression topology: {name} resource limit missing")
    if "no-new-privileges:true" not in service.get("security_opt", []):
        raise SystemExit(f"Memory regression topology: {name} no-new-privileges missing")

networks = config.get("networks", {})
if networks.get("memory-regression-private", {}).get("internal") is not True:
    raise SystemExit("Memory regression topology: private database network is not internal")
postgres = services["memory-regression-postgres"]
if set(postgres.get("networks", {})) != {"memory-regression-private"}:
    raise SystemExit("Memory regression topology: PostgreSQL network drift")
if postgres.get("image") != "mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5":
    raise SystemExit("Memory regression topology: PostgreSQL image drift")
if postgres.get("build", {}).get("context") != str((Path(sys.argv[1]).parent.parent / "postgres").resolve()):
    # Docker Compose may normalize to the actual product root; the target is
    # checked below by suffix to remain portable between Linux and WSL.
    if not str(postgres.get("build", {}).get("context", "")).replace("\\", "/").endswith("/mm-chat/postgres"):
        raise SystemExit("Memory regression topology: PostgreSQL build context drift")

migrate = services["memory-regression-migrate"]
if set(migrate.get("networks", {})) != {"memory-regression-private"}:
    raise SystemExit("Memory regression topology: migrate network drift")
if migrate.get("read_only") is not True or migrate.get("cap_drop") != ["ALL"]:
    raise SystemExit("Memory regression topology: migrate hardening drift")

def normalized_source(mount):
    return Path(str(mount["source"]).replace("\\", "/")).resolve()

for name in ("memory-regression-fake-runner", "memory-regression-live-runner"):
    runner = services[name]
    if runner.get("read_only") is not True or runner.get("cap_drop") != ["ALL"]:
        raise SystemExit(f"Memory regression topology: {name} hardening drift")
    if runner.get("build", {}).get("target") != "memory-regression-capture":
        raise SystemExit(f"Memory regression topology: {name} build target drift")
    mounts = {mount["target"]: mount for mount in runner.get("volumes", [])}
    expected_mounts = {
        "/fixtures/regression": fixture_root,
        "/inputs/cost-basis.json": cost,
        "/output": output,
    }
    if name == "memory-regression-live-runner":
        expected_mounts["/run/mm-chat-memory-regression/provider.key"] = credential
    if set(mounts) != set(expected_mounts):
        raise SystemExit(f"Memory regression topology: {name} mount target drift")
    for target, source in expected_mounts.items():
        mount = mounts[target]
        if mount.get("type") != "bind" or normalized_source(mount) != source:
            raise SystemExit(f"Memory regression topology: {name} mount source drift")
        if target != "/output" and mount.get("read_only") is not True:
            raise SystemExit(f"Memory regression topology: {name} input mount is writable")

fake = services["memory-regression-fake-runner"]
if fake.get("profiles") != ["memory-regression-fake"]:
    raise SystemExit("Memory regression topology: fake profile drift")
if set(fake.get("networks", {})) != {"memory-regression-private"}:
    raise SystemExit("Memory regression topology: fake runner gained external network")
if any("credential-file" in str(value) for value in fake.get("command", [])):
    raise SystemExit("Memory regression topology: fake runner accepted credential argument")

live = services["memory-regression-live-runner"]
if live.get("profiles") != ["memory-regression-live"]:
    raise SystemExit("Memory regression topology: live profile drift")
if set(live.get("networks", {})) != {"memory-regression-private", "memory-regression-egress"}:
    raise SystemExit("Memory regression topology: live egress topology drift")
if "/run/mm-chat-memory-regression/provider.key" not in live.get("command", []):
    raise SystemExit("Memory regression topology: live credential file boundary missing")
for key, value in live.get("environment", {}).items():
    if "KEY" in key or "TOKEN" in key or "SECRET" in key:
        raise SystemExit("Memory regression topology: Provider credential entered Docker environment")
    if "fixture-live-credential-not-used" in str(value):
        raise SystemExit("Memory regression topology: Provider credential leaked into Docker metadata")
PY

if rg -n '\.env\.single-server|mm-chat/(data|secrets|backup)|\.\./(data|secrets|backup)' "${compose_file}"; then
  echo "Memory regression topology: live runtime path referenced" >&2
  exit 1
fi
bash -n "${runner_script}"

fake_docker="${temp_dir}/fake-docker"
cat >"${fake_docker}" <<'PY'
#!/usr/bin/env python3
import hashlib
import json
import os
from pathlib import Path
import signal
import sys
import time

args = sys.argv[1:]
log_path = Path(os.environ.get("FAKE_DOCKER_LOG", "/dev/null"))
with log_path.open("a", encoding="utf-8") as log:
    log.write(" ".join(args) + "\n")

if args[:2] == ["compose", "version"]:
    raise SystemExit(0)
if args and args[0] == "compose":
    env_path = None
    if "--env-file" in args:
        env_path = Path(args[args.index("--env-file") + 1])
        with log_path.open("a", encoding="utf-8") as log:
            log.write(f"ENV_FILE={env_path}\n")
    if "config" in args or "build" in args or "up" in args or "down" in args:
        raise SystemExit(0)
    if "run" in args:
        service = next((value for value in args if value in {
            "memory-regression-migrate",
            "memory-regression-fake-runner",
            "memory-regression-live-runner",
        }), "")
        if service == "memory-regression-migrate":
            raise SystemExit(0)
        marker = os.environ.get("FAKE_RUNNER_MARKER")
        if marker:
            Path(marker).write_text("running\n", encoding="utf-8")
        sleep_seconds = float(os.environ.get("FAKE_RUNNER_SLEEP", "0"))
        if sleep_seconds:
            time.sleep(sleep_seconds)
        values = {}
        for line in env_path.read_text(encoding="utf-8").splitlines():
            key, value = line.split("=", 1)
            values[key] = value
        output = Path(values["MEMORY_REGRESSION_OUTPUT_PATH"])
        mode = "live_siliconflow" if "live" in service else "fake_protocol"
        candidate_prefix = "native-v2-hybrid" if mode == "live_siliconflow" else "native-v2-hybrid-fake-protocol"
        candidate_profile = "native_v2_hybrid" if mode == "live_siliconflow" else "native_v2_hybrid_fake_protocol"
        publish = os.environ.get("FAKE_PUBLISH", "full")
        observation = json.dumps({
            "schemaVersion": "neo-chat.memory-benchmark-regression-observations.v1",
            "cases": [{} for _ in range(500)],
        }, separators=(",", ":")).encode() + b"\n"
        report = json.dumps({
            "schemaVersion": "neo-chat.memory-benchmark-regression-report.v1",
            "corpusClass": "machine_reviewed_regression",
            "admissionMode": "regression_only",
            "promotionEligible": False,
            "passed": True,
        }, separators=(",", ":")).encode() + b"\n"
        bodies = {
            "native-v1-lexical.observations.json": observation,
            "native-v1-lexical.report.json": report,
            f"{candidate_prefix}.observations.json": observation,
            f"{candidate_prefix}.report.json": report,
        }
        if publish == "partial":
            name = "native-v1-lexical.observations.json"
            (output / name).write_bytes(bodies[name])
            (output / name).chmod(0o600)
        elif publish == "full":
            for name, body in bodies.items():
                (output / name).write_bytes(body)
                (output / name).chmod(0o600)
            manifest = {
                "schemaVersion": "neo-chat.memory-regression-native-run.v1",
                "runId": values["MEMORY_REGRESSION_RUN_ID"],
                "corpusClass": "machine_reviewed_regression",
                "admissionMode": "regression_only",
                "promotionEligible": False,
                "providerMode": mode,
                "profiles": [
                    {"role": "baseline", "profileId": "native_v1_lexical", "passed": True},
                    {"role": "candidate", "profileId": candidate_profile, "passed": True},
                ],
                "artifacts": [
                    {"name": name, "bytes": len(body), "sha256": hashlib.sha256(body).hexdigest()}
                    for name, body in sorted(bodies.items())
                ],
            }
            manifest_path = output / "run-manifest.json"
            manifest_path.write_text(json.dumps(manifest, separators=(",", ":")) + "\n", encoding="utf-8")
            manifest_path.chmod(0o600)
        print(json.dumps({"schemaVersion": "fake-docker-summary", "providerMode": mode}))
        raise SystemExit(int(os.environ.get("FAKE_RUNNER_STATUS", "0")))
if args[:2] == ["ps", "-aq"] or args[:3] == ["network", "ls", "-q"] or args[:3] == ["volume", "ls", "-q"]:
    raise SystemExit(0)
if args and args[0] == "inspect":
    print("[]")
    raise SystemExit(0)
raise SystemExit(0)
PY
chmod 700 "${fake_docker}"

assert_cleanup() {
  local log_file="$1"
  if ! grep -q 'compose .* down --volumes --remove-orphans' "${log_file}"; then
    echo "Memory regression protocol: teardown command missing" >&2
    exit 1
  fi
  local env_path
  env_path="$(sed -n 's/^ENV_FILE=//p' "${log_file}" | tail -n 1)"
  if [[ -n "${env_path}" && -e "$(dirname "${env_path}")" ]]; then
    echo "Memory regression protocol: temporary credential directory survived" >&2
    exit 1
  fi
}

success_output="${temp_dir}/success-output"
success_log="${temp_dir}/success-docker.log"
mkdir "${success_output}"
chmod 700 "${success_output}"
FAKE_DOCKER_LOG="${success_log}" FAKE_RUNNER_STATUS=0 FAKE_PUBLISH=full \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${success_output}" --provider-mode fake_protocol \
  >"${temp_dir}/success.stdout" 2>"${temp_dir}/success.stderr"
if [[ "$(find "${success_output}" -mindepth 2 -maxdepth 2 -type f | wc -l)" -ne 5 ]]; then
  echo "Memory regression protocol: success bundle was not retained" >&2
  exit 1
fi
assert_cleanup "${success_log}"

failure_output="${temp_dir}/failure-output"
failure_log="${temp_dir}/failure-docker.log"
mkdir "${failure_output}"
chmod 700 "${failure_output}"
set +e
FAKE_DOCKER_LOG="${failure_log}" FAKE_RUNNER_STATUS=7 FAKE_PUBLISH=partial \
  DOCKER_BIN="${fake_docker}" bash "${runner_script}" \
  --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
  --output-dir "${failure_output}" --provider-mode fake_protocol \
  >"${temp_dir}/failure.stdout" 2>"${temp_dir}/failure.stderr"
failure_status=$?
set -e
if [[ ${failure_status} -eq 0 || -n "$(find "${failure_output}" -mindepth 1 -print -quit)" ]]; then
  echo "Memory regression protocol: failure left partial output or returned success" >&2
  exit 1
fi
assert_cleanup "${failure_log}"

for signal_name in INT TERM HUP; do
  signal_output="${temp_dir}/signal-${signal_name}-output"
  signal_log="${temp_dir}/signal-${signal_name}-docker.log"
  marker="${temp_dir}/signal-${signal_name}.marker"
  mkdir "${signal_output}"
  chmod 700 "${signal_output}"
  setsid env \
    FAKE_DOCKER_LOG="${signal_log}" \
    FAKE_RUNNER_SLEEP=30 \
    FAKE_RUNNER_MARKER="${marker}" \
    FAKE_PUBLISH=none \
    DOCKER_BIN="${fake_docker}" \
    bash "${runner_script}" \
    --regression-root "${fixture_root}" --cost-basis "${cost_file}" \
    --output-dir "${signal_output}" --provider-mode fake_protocol \
    >"${temp_dir}/signal-${signal_name}.stdout" \
    2>"${temp_dir}/signal-${signal_name}.stderr" &
  wrapper_pid=$!
  for _ in $(seq 1 100); do
    [[ -f "${marker}" ]] && break
    sleep 0.05
  done
  if [[ ! -f "${marker}" ]]; then
    echo "Memory regression protocol: ${signal_name} runner did not start" >&2
    kill -TERM -- "-${wrapper_pid}" 2>/dev/null || true
    wait "${wrapper_pid}" || true
    exit 1
  fi
  kill -s "${signal_name}" -- "-${wrapper_pid}"
  set +e
  wait "${wrapper_pid}"
  signal_status=$?
  set -e
  if [[ ${signal_status} -eq 0 || -n "$(find "${signal_output}" -mindepth 1 -print -quit)" ]]; then
    echo "Memory regression protocol: ${signal_name} did not fail closed" >&2
    exit 1
  fi
  assert_cleanup "${signal_log}"
done

echo "Memory regression topology/lifecycle: passed"
