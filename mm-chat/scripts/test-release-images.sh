#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
release_script="${script_dir}/release-images.sh"
temp_dir="$(mktemp -d)"
trap 'rm -rf "${temp_dir}"' EXIT

fake_docker="${temp_dir}/docker"
cat >"${fake_docker}" <<'FAKE_DOCKER'
#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "version" ]]; then
  exit 0
fi
if [[ "${1:-}" == "buildx" && "${2:-}" == "version" ]]; then
  exit 0
fi
if [[ "${1:-}" != "buildx" || "${2:-}" != "build" ]]; then
  echo "fake docker: unsupported command" >&2
  exit 2
fi
shift 2

metadata_file=""
image_ref=""
while (( $# > 0 )); do
  case "$1" in
    --metadata-file)
      metadata_file="$2"
      shift 2
      ;;
    --tag)
      image_ref="$2"
      shift 2
      ;;
    *)
      shift
      ;;
  esac
done

case "${image_ref}" in
  */neobot-mm-chat-mcp-runner:*) component="mcp_runner"; hex="e" ;;
  */neobot-mm-chat-frontend:*) component="frontend"; hex="c" ;;
  */neobot-mm-chat-rag:*) component="rag"; hex="b" ;;
  */neobot-mm-chat-postgres:*) component="postgres"; hex="d" ;;
  */neobot-mm-chat:*) component="backend"; hex="a" ;;
  *) echo "fake docker: unexpected image ${image_ref}" >&2; exit 2 ;;
esac

if [[ "${FAKE_DOCKER_FAIL_COMPONENT:-}" == "${component}" ]]; then
  echo "fake docker: forced ${component} failure" >&2
  exit 42
fi

if [[ "${FAKE_DOCKER_INVALID_DIGEST_COMPONENT:-}" == "${component}" ]]; then
  printf '%s\n' '{"containerimage.digest":"sha256:not-canonical"}' >"${metadata_file}"
  exit 0
fi

digest="$(printf '%064s' '' | tr ' ' "${hex}")"
printf '{"containerimage.digest":"sha256:%s"}\n' "${digest}" >"${metadata_file}"
FAKE_DOCKER
chmod 700 "${fake_docker}"

assert_contains() {
  local haystack="$1"
  local needle="$2"
  if [[ "${haystack}" != *"${needle}"* ]]; then
    printf 'release image test: missing output: %s\n' "${needle}" >&2
    exit 1
  fi
}

namespace="registry.example/team"
tag="release-test"

dry_run_output="$(${release_script} \
  --dry-run \
  --push \
  --image-namespace "${namespace}" \
  --tag "${tag}")"
if [[ "$(grep -c '^==> Building ' <<<"${dry_run_output}")" != "5" ]]; then
  echo "release image test: dry run did not select five images" >&2
  exit 1
fi
assert_contains "${dry_run_output}" "${namespace}/neobot-mm-chat-postgres:${tag}"
assert_contains "${dry_run_output}" "${script_dir%/scripts}/postgres/Dockerfile"

push_dir="${temp_dir}/push"
DOCKER_BIN="${fake_docker}" "${release_script}" \
  --push \
  --image-namespace "${namespace}" \
  --tag "${tag}" \
  --metadata-dir "${push_dir}" >/dev/null

expected="${temp_dir}/expected.env"
backend_digest="$(printf '%064s' '' | tr ' ' a)"
mcp_runner_digest="$(printf '%064s' '' | tr ' ' e)"
frontend_digest="$(printf '%064s' '' | tr ' ' c)"
rag_digest="$(printf '%064s' '' | tr ' ' b)"
postgres_digest="$(printf '%064s' '' | tr ' ' d)"
cat >"${expected}" <<EOF
MM_CHAT_VERSION=${tag}
BACKEND_IMAGE=${namespace}/neobot-mm-chat@sha256:${backend_digest}
MCP_RUNNER_IMAGE=${namespace}/neobot-mm-chat-mcp-runner@sha256:${mcp_runner_digest}
FRONTEND_IMAGE=${namespace}/neobot-mm-chat-frontend@sha256:${frontend_digest}
RAG_IMAGE=${namespace}/neobot-mm-chat-rag@sha256:${rag_digest}
POSTGRES_IMAGE=${namespace}/neobot-mm-chat-postgres@sha256:${postgres_digest}
EOF
if ! cmp -s "${expected}" "${push_dir}/production-images.env"; then
  echo "release image test: production bundle is incomplete or unordered" >&2
  diff -u "${expected}" "${push_dir}/production-images.env" >&2 || true
  exit 1
fi

if FAKE_DOCKER_FAIL_COMPONENT=postgres DOCKER_BIN="${fake_docker}" \
  "${release_script}" \
    --push \
    --image-namespace "${namespace}" \
    --tag "${tag}" \
    --metadata-dir "${push_dir}" >/dev/null 2>&1; then
  echo "release image test: forced partial build unexpectedly passed" >&2
  exit 1
fi
if [[ -e "${push_dir}/production-images.env" || -e "${push_dir}/production-images.env.tmp" ]]; then
  echo "release image test: failed build retained a promotable bundle" >&2
  exit 1
fi

if FAKE_DOCKER_INVALID_DIGEST_COMPONENT=backend DOCKER_BIN="${fake_docker}" \
  "${release_script}" \
    --push \
    --image-namespace "${namespace}" \
    --tag "${tag}" \
    --metadata-dir "${push_dir}" >/dev/null 2>&1; then
  echo "release image test: non-canonical digest unexpectedly passed" >&2
  exit 1
fi
if [[ -e "${push_dir}/production-images.env" || -e "${push_dir}/production-images.env.tmp" ]]; then
  echo "release image test: invalid digest retained a promotable bundle" >&2
  exit 1
fi

DOCKER_BIN="${fake_docker}" "${release_script}" \
  --load \
  --image-namespace "${namespace}" \
  --tag "${tag}" \
  --metadata-dir "${push_dir}" >/dev/null
if [[ -e "${push_dir}/production-images.env" ]]; then
  echo "release image test: local build retained a production bundle" >&2
  exit 1
fi
if [[ "$(wc -l <"${push_dir}/local-images.env")" != "5" ]]; then
  echo "release image test: local bundle does not contain five image tags" >&2
  exit 1
fi

if "${release_script}" --dry-run --postgres-repo Invalid --tag "${tag}" >/dev/null 2>&1; then
  echo "release image test: invalid PostgreSQL repository was accepted" >&2
  exit 1
fi
if "${release_script}" --dry-run --image-namespace team/project --tag "${tag}" >/dev/null 2>&1; then
  echo "release image test: namespace without a registry host was accepted" >&2
  exit 1
fi

echo "release image test: passed"
