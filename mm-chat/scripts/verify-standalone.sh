#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
usage: verify-standalone.sh [--full]

Without arguments, verify an isolated mm-chat copy, its manifests, symlink
boundary, absolute-path boundary, and rendered Compose topology.

--full additionally installs and verifies the frontend, runs Go tests, and
creates an isolated Python environment for the RAG quality gates.
EOF
}

full=false
case "${1:-}" in
  "") ;;
  --full) full=true ;;
  -h | --help)
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
temp_dir="$(mktemp -d)"
copy_dir="${temp_dir}/mm-chat"
trap 'rm -rf "${temp_dir}"' EXIT

docker_bin="${DOCKER_BIN:-docker}"
docker_uses_windows_paths=false
if ! "${docker_bin}" compose version >/dev/null 2>&1; then
  windows_docker="/mnt/c/Program Files/Docker/Docker/resources/bin/docker.exe"
  if [[ -x "${windows_docker}" ]] && "${windows_docker}" compose version >/dev/null 2>&1; then
    docker_bin="${windows_docker}"
    docker_uses_windows_paths=true
  else
    echo "standalone verification: docker compose is required for Compose topology rendering" >&2
    echo "standalone verification: start Docker Desktop and enable WSL integration, or set DOCKER_BIN" >&2
    exit 1
  fi
fi

docker_path() {
  local path="$1"
  if [[ "${docker_uses_windows_paths}" == true ]]; then
    wslpath -w "${path}"
  else
    printf '%s\n' "${path}"
  fi
}

mkdir -p "${copy_dir}"
tar \
  --exclude='./.env.single-server' \
  --exclude='./backup' \
  --exclude='./data' \
  --exclude='./frontend/.next' \
  --exclude='./frontend/.open-next' \
  --exclude='./frontend/node_modules' \
  --exclude='./frontend/tsconfig.tsbuildinfo' \
  --exclude='./rag/.mypy_cache' \
  --exclude='./rag/.pytest_cache' \
  --exclude='./rag/.ruff_cache' \
  --exclude='./rag/.venv' \
  -C "${project_dir}" -cf - . | tar -C "${copy_dir}" -xf -

required_paths=(
  README.md
  compose.yml
  compose.single-server.yml
  compose.hindsight-fixture.yml
  compose.memory-regression.yml
  compose.production.yml
  frontend/package.json
  frontend/pnpm-lock.yaml
  frontend/next.config.ts
  frontend/Dockerfile
  frontend/src/app
  frontend/public
  backend/go.mod
  backend/Dockerfile
  scripts/run-memory-hindsight-fixture.sh
  scripts/test-memory-hindsight-fixture.sh
  scripts/run-memory-regression.sh
  scripts/test-memory-regression.sh
  rag/pyproject.toml
  rag/Dockerfile
)
for path in "${required_paths[@]}"; do
  if [[ ! -e "${copy_dir}/${path}" ]]; then
    echo "standalone verification: missing ${path}" >&2
    exit 1
  fi
done

if symlink="$(find "${copy_dir}" -type l -print -quit)" && [[ -n "${symlink}" ]]; then
  echo "standalone verification: symbolic link is not allowed: ${symlink#${copy_dir}/}" >&2
  exit 1
fi

if rg -n --hidden \
  --glob '!**/docs/tracking/process.md' \
  --glob '!**/scripts/verify-standalone.sh' \
  '/home/mumu/projects/neo-chat|\\\\wsl\.localhost\\Ubuntu\\home\\mumu\\projects\\neo-chat' \
  "${copy_dir}" >"${temp_dir}/outer-paths.txt"; then
  echo "standalone verification: outer-project absolute path found" >&2
  cat "${temp_dir}/outer-paths.txt" >&2
  exit 1
fi

compose_json="${temp_dir}/compose.json"
"${docker_bin}" compose \
  --project-directory "$(docker_path "${copy_dir}")" \
  -f "$(docker_path "${copy_dir}/compose.yml")" \
  --profile app --profile ops --profile memory-worker \
  --profile rag-worker --profile rag-ops \
  config --format json >"${compose_json}"

python3 - "${compose_json}" "${copy_dir}" <<'PY'
import json
import sys
from pathlib import Path

config = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
root = Path(sys.argv[2]).resolve()


def normalize_path(value: str) -> Path:
    # Windows docker.exe may render WSL paths as UNC paths such as
    # \\wsl.localhost\Ubuntu\tmp\...; convert those back before comparing
    # against the Linux-side clean-copy root.
    normalized = value.replace("\\", "/")
    for prefix in ("//wsl.localhost/Ubuntu", "//wsl$/Ubuntu"):
        if normalized.startswith(prefix + "/"):
            normalized = normalized[len(prefix) :]
            break
    return Path(normalized).resolve()


services = config["services"]
required = {
    "frontend",
    "backend",
    "memory-worker",
    "postgres",
    "redis",
    "minio",
    "minio-init",
    "migrate",
    "admin",
    "rag-worker",
    "rag-replay",
}
missing = sorted(required - services.keys())
if missing:
    raise SystemExit(f"standalone verification: missing services: {missing}")

for name, service in services.items():
    build = service.get("build")
    if not build:
        continue
    context = normalize_path(str(build["context"]))
    if context != root and root not in context.parents:
        raise SystemExit(
            f"standalone verification: {name} build context escapes project: {context}"
        )

frontend = services["frontend"]
if frontend["environment"]["NEXT_PUBLIC_API_MODE"] != "server":
    raise SystemExit("standalone verification: frontend is not in server mode")
if frontend["environment"]["NEXT_PUBLIC_API_BASE_URL"] != "/mm-api":
    raise SystemExit("standalone verification: frontend API edge is not /mm-api")
if "backend" not in frontend.get("depends_on", {}):
    raise SystemExit("standalone verification: frontend does not depend on backend")

backend = services["backend"]
memory_worker = services["memory-worker"]
if memory_worker.get("profiles") != ["memory-worker"]:
    raise SystemExit("standalone verification: Memory Worker profile drifted")
if memory_worker.get("ports"):
    raise SystemExit("standalone verification: Memory Worker exposes a host port")
if set(memory_worker.get("networks", {})) != {"private"}:
    raise SystemExit("standalone verification: Memory Worker is not private-only")
if (
    backend["environment"]["MEMORY_HYBRID_SHADOW_ENABLED"]
    != memory_worker["environment"]["MEMORY_HYBRID_SHADOW_ENABLED"]
):
    raise SystemExit("standalone verification: Memory hybrid flags disagree")
if backend["environment"]["MEMORY_TOOL_LOOP_ENABLED"] != "false":
    raise SystemExit("standalone verification: Memory Tool Loop must default false")
if "MEMORY_TOOL_LOOP_ENABLED" in memory_worker["environment"]:
    raise SystemExit("standalone verification: Memory Worker received the Tool Loop flag")
if (
    backend["environment"]["MEMORY_L2_SCENE_SHADOW_ENABLED"]
    != memory_worker["environment"]["MEMORY_L2_SCENE_SHADOW_ENABLED"]
):
    raise SystemExit("standalone verification: Memory L2 Scene shadow flags disagree")
if backend["environment"]["MEMORY_L2_SCENE_READER_ENABLED"] != "false":
    raise SystemExit("standalone verification: Memory L2 Scene reader must default false")
if "MEMORY_L2_SCENE_READER_ENABLED" in memory_worker["environment"]:
    raise SystemExit("standalone verification: Memory Worker received the L2 reader flag")
if (
    backend["environment"]["MEMORY_L3_PERSONA_SHADOW_ENABLED"]
    != memory_worker["environment"]["MEMORY_L3_PERSONA_SHADOW_ENABLED"]
):
    raise SystemExit("standalone verification: Memory L3 Persona shadow flags disagree")
if backend["environment"]["MEMORY_L3_PERSONA_READER_ENABLED"] != "false":
    raise SystemExit("standalone verification: Memory L3 Persona reader must default false")
if "MEMORY_L3_PERSONA_READER_ENABLED" in memory_worker["environment"]:
    raise SystemExit("standalone verification: Memory Worker received the L3 reader flag")
PY

DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-hindsight-fixture.sh"
DOCKER_BIN="${docker_bin}" bash "${copy_dir}/scripts/test-memory-regression.sh"

if [[ "${full}" == true ]]; then
  rag_python="${RAG_PYTHON:-python3.13}"
  if ! command -v "${rag_python}" >/dev/null 2>&1; then
    echo "standalone verification: Python 3.13 is required for RAG checks" >&2
    exit 1
  fi
  if ! "${rag_python}" -c \
    'import sys; raise SystemExit(sys.version_info[:2] != (3, 13))'; then
    echo "standalone verification: RAG checks require Python 3.13" >&2
    exit 1
  fi

  (
    cd "${copy_dir}/frontend"
    corepack pnpm install --frozen-lockfile
    corepack pnpm format:check
    corepack pnpm lint
    corepack pnpm typecheck
    corepack pnpm test
    NEXT_PUBLIC_API_MODE=server \
      NEXT_PUBLIC_API_BASE_URL=/mm-api \
      MM_CHAT_BACKEND_INTERNAL_URL=http://backend:8080 \
      corepack pnpm build
  )
  (
    cd "${copy_dir}/backend"
    test -z "$(gofmt -l .)"
    go vet ./...
    go test ./...
  )
  (
    cd "${copy_dir}/rag"
    "${rag_python}" -m venv .venv
    .venv/bin/pip install -e . --group dev
    .venv/bin/ruff check .
    .venv/bin/ruff format --check .
    .venv/bin/mypy src
    .venv/bin/pytest
  )
fi

if [[ "${full}" == true ]]; then
  echo "standalone verification: passed (full)"
else
  echo "standalone verification: passed (structure)"
fi
