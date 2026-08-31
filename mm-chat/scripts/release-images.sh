#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
usage: release-images.sh [options]

Build the five standalone mm-chat images:
  - Go backend/admin/migrate image
  - Hardened MCP stdio runner image
  - Next.js frontend image
  - Python RAG worker image
  - PostgreSQL retrieval image

By default this builds local images with docker buildx --load. Use --push to
publish immutable registry images and print
FRONTEND_IMAGE/BACKEND_IMAGE/MCP_RUNNER_IMAGE/RAG_IMAGE/POSTGRES_IMAGE values
suitable for .env.single-server production preflight.

Options:
  --push                         Push images to the registry and emit digest refs.
  --load                         Load local images into Docker (default).
  --dry-run                      Print build commands without running Docker.
  --image-namespace <namespace>  Registry namespace, e.g. ghcr.io/mumu-0922.
                                 Default: IMAGE_NAMESPACE or ghcr.io/mumu-0922.
  --backend-repo <name>          Default: neobot-mm-chat.
  --mcp-runner-repo <name>       Default: neobot-mm-chat-mcp-runner.
  --frontend-repo <name>         Default: neobot-mm-chat-frontend.
  --rag-repo <name>              Default: neobot-mm-chat-rag.
  --postgres-repo <name>         Default: neobot-mm-chat-postgres.
  --tag <tag>                    Image tag and MM_CHAT_VERSION. Default: git-<sha>.
  --platform <platform>          Build platform. Default: linux/amd64.
  --metadata-dir <dir>           Output metadata/env dir. Default: .release/images/<tag>.
  --pull                         Always attempt to pull newer base images.
  --no-cache                     Disable Docker build cache.
  -h, --help                     Show this help.

Examples:
  ./scripts/release-images.sh --load --tag smoke-local
  docker login ghcr.io
  ./scripts/release-images.sh --push --image-namespace ghcr.io/mumu-0922 --tag v1.0.0
USAGE
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd "${script_dir}/.." && pwd -P)"

mode="load"
dry_run=false
image_namespace="${IMAGE_NAMESPACE:-ghcr.io/mumu-0922}"
backend_repo="${BACKEND_IMAGE_REPOSITORY:-neobot-mm-chat}"
mcp_runner_repo="${MCP_RUNNER_IMAGE_REPOSITORY:-neobot-mm-chat-mcp-runner}"
frontend_repo="${FRONTEND_IMAGE_REPOSITORY:-neobot-mm-chat-frontend}"
rag_repo="${RAG_IMAGE_REPOSITORY:-neobot-mm-chat-rag}"
postgres_repo="${POSTGRES_IMAGE_REPOSITORY:-neobot-mm-chat-postgres}"
tag="${MM_CHAT_RELEASE_TAG:-}"
platform="${PLATFORM:-linux/amd64}"
metadata_dir=""
pull=false
no_cache=false
docker_bin="${DOCKER_BIN:-docker}"

while (( $# > 0 )); do
  case "$1" in
    --push)
      mode="push"
      shift
      ;;
    --load)
      mode="load"
      shift
      ;;
    --dry-run)
      dry_run=true
      shift
      ;;
    --image-namespace)
      if (( $# < 2 )); then echo "release-images: --image-namespace requires a value" >&2; exit 2; fi
      image_namespace="$2"
      shift 2
      ;;
    --backend-repo)
      if (( $# < 2 )); then echo "release-images: --backend-repo requires a value" >&2; exit 2; fi
      backend_repo="$2"
      shift 2
      ;;
    --mcp-runner-repo)
      if (( $# < 2 )); then echo "release-images: --mcp-runner-repo requires a value" >&2; exit 2; fi
      mcp_runner_repo="$2"
      shift 2
      ;;
    --frontend-repo)
      if (( $# < 2 )); then echo "release-images: --frontend-repo requires a value" >&2; exit 2; fi
      frontend_repo="$2"
      shift 2
      ;;
    --rag-repo)
      if (( $# < 2 )); then echo "release-images: --rag-repo requires a value" >&2; exit 2; fi
      rag_repo="$2"
      shift 2
      ;;
    --postgres-repo)
      if (( $# < 2 )); then echo "release-images: --postgres-repo requires a value" >&2; exit 2; fi
      postgres_repo="$2"
      shift 2
      ;;
    --tag)
      if (( $# < 2 )); then echo "release-images: --tag requires a value" >&2; exit 2; fi
      tag="$2"
      shift 2
      ;;
    --platform)
      if (( $# < 2 )); then echo "release-images: --platform requires a value" >&2; exit 2; fi
      platform="$2"
      shift 2
      ;;
    --metadata-dir)
      if (( $# < 2 )); then echo "release-images: --metadata-dir requires a value" >&2; exit 2; fi
      metadata_dir="$2"
      shift 2
      ;;
    --pull)
      pull=true
      shift
      ;;
    --no-cache)
      no_cache=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "${tag}" ]]; then
  if git -C "${project_dir}" rev-parse --short=12 HEAD >/dev/null 2>&1; then
    tag="git-$(git -C "${project_dir}" rev-parse --short=12 HEAD)"
  else
    tag="manual-$(date -u +%Y%m%dT%H%M%SZ)"
  fi
fi

if [[ ! "${tag}" =~ ^[A-Za-z0-9_.-]+$ ]]; then
  echo "release-images: tag may contain only letters, digits, dot, underscore, and dash" >&2
  exit 2
fi

image_namespace="${image_namespace%/}"
if [[ -z "${image_namespace}" || "${image_namespace}" != */* ]]; then
  echo "release-images: --image-namespace must include registry and namespace, e.g. ghcr.io/mumu-0922" >&2
  exit 2
fi
registry="${image_namespace%%/*}"
namespace_path="${image_namespace#*/}"
if [[ "${registry}" != *.* && "${registry}" != *:* ]]; then
  echo "release-images: --image-namespace must begin with a registry host" >&2
  exit 2
fi
IFS='/' read -r -a namespace_segments <<<"${namespace_path}"
for segment in "${namespace_segments[@]}"; do
  if [[ ! "${segment}" =~ ^[a-z0-9]+([._-][a-z0-9]+)*$ ]]; then
    echo "release-images: namespace segment '${segment}' must be lowercase and registry-safe" >&2
    exit 2
  fi
done

for repo in \
  "${backend_repo}" \
  "${mcp_runner_repo}" \
  "${frontend_repo}" \
  "${rag_repo}" \
  "${postgres_repo}"; do
  if [[ ! "${repo}" =~ ^[a-z0-9]+([._-][a-z0-9]+)*$ ]]; then
    echo "release-images: repository '${repo}' must be lowercase and registry-safe" >&2
    exit 2
  fi
done

if [[ -z "${metadata_dir}" ]]; then
  metadata_dir="${project_dir}/.release/images/${tag}"
elif [[ "${metadata_dir}" != /* ]]; then
  metadata_dir="$(pwd -P)/${metadata_dir}"
fi

if [[ "${dry_run}" != true ]]; then
  if ! command -v "${docker_bin}" >/dev/null 2>&1; then
    echo "release-images: Docker command not found: ${docker_bin}" >&2
    exit 127
  fi
  if ! "${docker_bin}" version >/dev/null 2>&1; then
    echo "release-images: Docker daemon is not reachable from this shell" >&2
    echo "release-images: start Docker Desktop and enable WSL integration for this distro, then retry" >&2
    "${docker_bin}" version >&2 || true
    exit 1
  fi
  if ! "${docker_bin}" buildx version >/dev/null 2>&1; then
    echo "release-images: docker buildx is required" >&2
    exit 1
  fi
  mkdir -p "${metadata_dir}"
fi

quote_cmd() {
  local arg
  for arg in "$@"; do
    printf '%q ' "${arg}"
  done
  printf '\n'
}

extract_digest() {
  local metadata_file="$1"
  python3 - "$metadata_file" <<'PY'
import json
import re
import sys
from pathlib import Path

metadata = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
digest = metadata.get("containerimage.digest")
if not digest:
    descriptor = metadata.get("containerimage.descriptor")
    if isinstance(descriptor, dict):
        digest = descriptor.get("digest")
if not isinstance(digest, str) or re.fullmatch(r"sha256:[0-9a-f]{64}", digest) is None:
    raise SystemExit("metadata does not contain a canonical sha256 image digest")
print(digest)
PY
}

build_component() {
  local component="$1"
  local repo="$2"
  local context_dir="$3"
  local dockerfile="$4"
  shift 4
  local -a extra_args=("$@")
  local image_ref="${image_namespace}/${repo}:${tag}"
  local metadata_file="${metadata_dir}/${component}.metadata.json"
  local -a cmd=(
    "${docker_bin}" buildx build
    --platform "${platform}"
    --file "${dockerfile}"
    --tag "${image_ref}"
    --metadata-file "${metadata_file}"
  )
  if [[ "${pull}" == true ]]; then
    cmd+=(--pull)
  fi
  if [[ "${no_cache}" == true ]]; then
    cmd+=(--no-cache)
  fi
  cmd+=("${extra_args[@]}")
  if [[ "${mode}" == "push" ]]; then
    cmd+=(--push)
  else
    cmd+=(--load)
  fi
  cmd+=("${context_dir}")

  printf '\n==> Building %s image: %s\n' "${component}" "${image_ref}"
  quote_cmd "${cmd[@]}"
  if [[ "${dry_run}" == true ]]; then
    return 0
  fi

  "${cmd[@]}"
  if [[ "${mode}" == "push" ]]; then
    local digest
    local env_key
    digest="$(extract_digest "${metadata_file}")"
    env_key="$(printf '%s' "${component}" | tr '[:lower:]' '[:upper:]')"
    printf '%s_IMAGE=%s/%s@%s\n' \
      "${env_key}" "${image_namespace}" "${repo}" "${digest}" \
      >> "${metadata_dir}/release-images.env"
  else
    printf '%s_TAG=%s\n' "$(printf '%s' "${component}" | tr '[:lower:]' '[:upper:]')" "${image_ref}" \
      >> "${metadata_dir}/local-images.env"
  fi
}

if [[ "${dry_run}" != true ]]; then
  rm -f \
    "${metadata_dir}/release-images.env" \
    "${metadata_dir}/local-images.env" \
    "${metadata_dir}/production-images.env" \
    "${metadata_dir}/production-images.env.tmp"
fi

frontend_build_args=(
  --build-arg "NEXT_PUBLIC_API_MODE=${NEXT_PUBLIC_API_MODE:-server}"
  --build-arg "NEXT_PUBLIC_API_BASE_URL=${NEXT_PUBLIC_API_BASE_URL:-/mm-api}"
  --build-arg "MM_CHAT_BACKEND_INTERNAL_URL=${MM_CHAT_BACKEND_INTERNAL_URL:-http://backend:8080}"
)
if [[ -n "${NEXT_PUBLIC_SITE_URL:-}" ]]; then
  frontend_build_args+=(--build-arg "NEXT_PUBLIC_SITE_URL=${NEXT_PUBLIC_SITE_URL}")
fi

printf 'mm-chat image release\n'
printf '  project: %s\n' "${project_dir}"
printf '  mode: %s\n' "${mode}"
printf '  namespace: %s\n' "${image_namespace}"
printf '  tag: %s\n' "${tag}"
printf '  platform: %s\n' "${platform}"
printf '  metadata: %s\n' "${metadata_dir}"

build_component \
  backend \
  "${backend_repo}" \
  "${project_dir}/backend" \
  "${project_dir}/backend/Dockerfile" \
  --target runtime

build_component \
  mcp_runner \
  "${mcp_runner_repo}" \
  "${project_dir}/backend" \
  "${project_dir}/backend/Dockerfile" \
  --target mcp-runner

build_component \
  frontend \
  "${frontend_repo}" \
  "${project_dir}/frontend" \
  "${project_dir}/frontend/Dockerfile" \
  "${frontend_build_args[@]}"

build_component \
  rag \
  "${rag_repo}" \
  "${project_dir}/rag" \
  "${project_dir}/rag/Dockerfile"

build_component \
  postgres \
  "${postgres_repo}" \
  "${project_dir}/postgres" \
  "${project_dir}/postgres/Dockerfile"

if [[ "${dry_run}" == true ]]; then
  printf '\nDry run complete. No images were built.\n'
  exit 0
fi

if [[ "${mode}" == "push" ]]; then
  {
    printf 'MM_CHAT_VERSION=%s\n' "${tag}"
    cat "${metadata_dir}/release-images.env"
  } > "${metadata_dir}/production-images.env.tmp"
  mv "${metadata_dir}/production-images.env.tmp" "${metadata_dir}/production-images.env"
  printf '\nProduction image refs written to:\n  %s\n\n' "${metadata_dir}/production-images.env"
  cat "${metadata_dir}/production-images.env"
  printf '\nCopy these six lines into .env.single-server, then rerun production preflight/backup.\n'
else
  printf '\nLocal image tags written to:\n  %s\n\n' "${metadata_dir}/local-images.env"
  cat "${metadata_dir}/local-images.env"
  printf '\nLocal --load builds are useful for smoke tests but cannot pass production preflight.\n'
  printf 'Run again with --push after docker login to emit immutable @sha256 refs.\n'
fi
