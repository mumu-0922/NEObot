#!/usr/bin/env bash
set -euo pipefail

if (( $# < 2 )); then
  echo "usage: compose-single-server-production.sh <env-file> <compose-args...>" >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
env_file="$1"
shift

for argument in "$@"; do
  case "${argument}" in
    -f | -f* | --file | --file=* | --env-file | --env-file=* | \
      --project-directory | --project-directory=* | --build | --build=* | \
      -e* | --env | --env=* | --env-from-file | --env-from-file=* | build)
      echo "production compose: file/env/build overrides are forbidden" >&2
      exit 2
      ;;
  esac
done

if [[ "${env_file}" != /* ]]; then
  env_file="$(cd "$(dirname "${env_file}")" && pwd)/$(basename "${env_file}")"
fi

"${script_dir}/preflight-single-server.sh" "${env_file}" >&2

clean_env=(
  "PATH=${PATH}"
  "HOME=${HOME:-/tmp}"
  "COMPOSE_DISABLE_ENV_FILE=1"
)
for name in \
  DOCKER_HOST \
  DOCKER_CONTEXT \
  DOCKER_TLS_VERIFY \
  DOCKER_CERT_PATH \
  DOCKER_CONFIG \
  XDG_RUNTIME_DIR; do
  if [[ -n "${!name:-}" ]]; then
    clean_env+=("${name}=${!name}")
  fi
done

exec env -i "${clean_env[@]}" docker compose \
  --project-directory "${project_dir}" \
  --env-file "${env_file}" \
  -f "${project_dir}/compose.single-server.yml" \
  -f "${project_dir}/compose.production.yml" \
  "$@"
