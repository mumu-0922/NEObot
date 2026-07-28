#!/usr/bin/env bash
set -euo pipefail

if (( $# < 1 || $# > 2 )); then
  echo "usage: backup-single-server-production.sh <env-file> [daily|weekly|pre-deploy]" >&2
  exit 2
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
env_file="$1"
backup_class="${2:-daily}"
case "$backup_class" in
  daily|weekly|pre-deploy) ;;
  *)
    echo "backup class must be daily, weekly, or pre-deploy" >&2
    exit 2
    ;;
esac
if [[ "${env_file}" != /* ]]; then
  env_file="$(cd "$(dirname "${env_file}")" && pwd)/$(basename "${env_file}")"
fi

"${script_dir}/preflight-single-server.sh" "${env_file}" >&2

umask 077
backup_root="${project_dir}/backup"
sets_dir="${backup_root}/sets"
mkdir -p "${sets_dir}"
created_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
random_suffix="$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')"
backup_set_id="${timestamp}-${random_suffix}"

postgres_rel="postgres/postgres-${backup_set_id}.dump"
postgres_checksum_rel="${postgres_rel}.sha256"
minio_rel="minio/minio-${backup_set_id}.tar.gz"
minio_checksum_rel="${minio_rel}.sha256"
manifest_rel="sets/${backup_set_id}.json"
manifest_checksum_rel="${manifest_rel}.sha256"

cleanup_set() {
  rm -f \
    "${backup_root}/${postgres_rel}" \
    "${backup_root}/${postgres_checksum_rel}" \
    "${backup_root}/${minio_rel}" \
    "${backup_root}/${minio_checksum_rel}" \
    "${backup_root}/${manifest_rel}" \
    "${backup_root}/${manifest_checksum_rel}" \
    "${backup_root}/${manifest_rel}.tmp"
}
trap cleanup_set EXIT

clean_env=(
  "PATH=${PATH}"
  "HOME=${HOME:-/tmp}"
  "COMPOSE_DISABLE_ENV_FILE=1"
  "COMPOSE_FILE=${project_dir}/compose.single-server.yml:${project_dir}/compose.production.yml"
  "ENV_FILE=${env_file}"
  "BACKUP_SET_ID=${backup_set_id}"
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

for path in \
  "${backup_root}/${postgres_rel}" \
  "${backup_root}/${postgres_checksum_rel}" \
  "${backup_root}/${minio_rel}" \
  "${backup_root}/${minio_checksum_rel}"; do
  if [[ ! -f "$path" || -L "$path" ]]; then
    printf 'backup set artifact is missing or unsafe: %s\n' "$path" >&2
    exit 1
  fi
done

(cd "${backup_root}/postgres" && sha256sum -c "$(basename "${postgres_checksum_rel}")")
(cd "${backup_root}/minio" && sha256sum -c "$(basename "${minio_checksum_rel}")")

postgres_sha256="$(sha256sum "${backup_root}/${postgres_rel}" | awk '{print $1}')"
minio_sha256="$(sha256sum "${backup_root}/${minio_rel}" | awk '{print $1}')"

BACKUP_SET_ID="$backup_set_id" \
BACKUP_CLASS="$backup_class" \
BACKUP_CREATED_AT="$created_at" \
POSTGRES_REL="$postgres_rel" \
POSTGRES_CHECKSUM_REL="$postgres_checksum_rel" \
POSTGRES_SHA256="$postgres_sha256" \
MINIO_REL="$minio_rel" \
MINIO_CHECKSUM_REL="$minio_checksum_rel" \
MINIO_SHA256="$minio_sha256" \
python3 - <<'PY' > "${backup_root}/${manifest_rel}.tmp"
import json
import os

manifest = {
    "version": 1,
    "setId": os.environ["BACKUP_SET_ID"],
    "class": os.environ["BACKUP_CLASS"],
    "createdAt": os.environ["BACKUP_CREATED_AT"],
    "containsMemoryPlaintext": True,
    "artifacts": [
        {
            "kind": "postgres",
            "path": os.environ["POSTGRES_REL"],
            "checksumPath": os.environ["POSTGRES_CHECKSUM_REL"],
            "sha256": os.environ["POSTGRES_SHA256"],
        },
        {
            "kind": "minio",
            "path": os.environ["MINIO_REL"],
            "checksumPath": os.environ["MINIO_CHECKSUM_REL"],
            "sha256": os.environ["MINIO_SHA256"],
        },
    ],
}
print(json.dumps(manifest, ensure_ascii=False, separators=(",", ":")))
PY

mv "${backup_root}/${manifest_rel}.tmp" "${backup_root}/${manifest_rel}"
(
  cd "$sets_dir"
  sha256sum "${backup_set_id}.json" > "${backup_set_id}.json.sha256"
)
trap - EXIT

printf 'Created verified backup set:\n  %s\n  %s\n' \
  "${backup_root}/${manifest_rel}" \
  "${backup_root}/${manifest_checksum_rel}"
