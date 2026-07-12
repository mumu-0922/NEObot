#!/usr/bin/env bash
set -euo pipefail

if (( $# != 1 )); then
  echo "usage: backup-single-server-production.sh <env-file>" >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
env_file="$1"
if [[ "${env_file}" != /* ]]; then
  env_file="$(cd "$(dirname "${env_file}")" && pwd)/$(basename "${env_file}")"
fi

"${script_dir}/preflight-single-server.sh" "${env_file}" >&2

clean_env=(
  "PATH=${PATH}"
  "HOME=${HOME:-/tmp}"
  "COMPOSE_DISABLE_ENV_FILE=1"
  "COMPOSE_FILE=${project_dir}/compose.single-server.yml:${project_dir}/compose.production.yml"
  "ENV_FILE=${env_file}"
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

env -i "${clean_env[@]}" "${script_dir}/backup-postgres.sh"
env -i "${clean_env[@]}" "${script_dir}/backup-minio.sh"
