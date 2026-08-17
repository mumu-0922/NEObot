#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
project_dir="$(cd -- "${script_dir}/.." && pwd -P)"
backend_dir="${project_dir}/backend"
postgres_dir="${project_dir}/postgres"
postgres_image="${POSTGRES_IMAGE:-mm-chat/postgres:17.10-pg_textsearch1.3.1-pgvector0.8.5}"
container_name="neo-chat-artifacts-pg17-$RANDOM-$$"
database_name="neo_chat_artifacts_drill"
database_user="postgres"
database_password="artifact-drill-$(openssl rand -hex 16)"

cleanup() {
  docker rm -f "${container_name}" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
log() { printf 'Chat artifact PostgreSQL 17 drill: %s\n' "$*"; }

for command in docker go openssl; do
  command -v "${command}" >/dev/null 2>&1 || {
    echo "Chat artifact PostgreSQL 17 drill: ${command} is required" >&2
    exit 1
  }
done
docker version >/dev/null 2>&1 || {
  echo "Chat artifact PostgreSQL 17 drill: Docker is required" >&2
  exit 1
}
if ! docker image inspect "${postgres_image}" >/dev/null 2>&1; then
  docker build --pull=false --tag "${postgres_image}" "${postgres_dir}" >/dev/null
fi

docker run -d --rm --name "${container_name}" \
  -e "POSTGRES_DB=${database_name}" -e "POSTGRES_USER=${database_user}" \
  -e "POSTGRES_PASSWORD=${database_password}" -p 127.0.0.1::5432 \
  "${postgres_image}" >/dev/null
for _ in $(seq 1 60); do
  docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" \
    >/dev/null 2>&1 && break
  sleep 1
done
docker exec "${container_name}" pg_isready -U "${database_user}" -d "${database_name}" >/dev/null

host_port="$(docker port "${container_name}" 5432/tcp | sed -n '1s/.*://p')"
database_url="postgres://${database_user}:${database_password}@127.0.0.1:${host_port}/${database_name}?sslmode=disable"

log "running output attachment round-trip, ownership and deletion proofs"
(
  cd "${backend_dir}"
  MM_CHAT_TEST_DATABASE_URL="${database_url}" go test -count=1 -race \
    -run '^TestPostgres(CreateMessagePersistsAttachmentOnlyMessages|RepositoryEnforcesTwoUserIsolation|CreateMessageRejectsMissingOrDeletedAttachment)$' \
    ./internal/chat
)
log "passed"
