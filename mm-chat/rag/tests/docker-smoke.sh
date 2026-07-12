#!/usr/bin/env bash
set -euo pipefail

image="${1:-mm-chat-rag:phase-15.2b-smoke}"
postgres_image="${2:-postgres:16-alpine}"
suffix="$$"
network="rag-smoke-${suffix}"
postgres_container="rag-smoke-postgres-${suffix}"
worker_container="rag-smoke-worker-${suffix}"

cleanup() {
  docker rm -f "${worker_container}" "${postgres_container}" >/dev/null 2>&1 || true
  docker network rm "${network}" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

docker build --tag "${image}" .
docker network create "${network}" >/dev/null
docker run --detach --name "${postgres_container}" \
  --network "${network}" --network-alias postgres \
  --env POSTGRES_HOST_AUTH_METHOD=trust \
  --env POSTGRES_DB=rag_smoke \
  "${postgres_image}" >/dev/null

for _ in $(seq 1 30); do
  if docker exec "${postgres_container}" \
    pg_isready --username postgres --dbname rag_smoke >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
docker exec "${postgres_container}" \
  pg_isready --username postgres --dbname rag_smoke >/dev/null

docker exec --interactive "${postgres_container}" \
  psql --username postgres --dbname rag_smoke --set ON_ERROR_STOP=1 >/dev/null <<'SQL'
CREATE FUNCTION knowledge_rag_worker_readiness()
RETURNS TABLE(
  consumer_ready BOOLEAN,
  projection_ready BOOLEAN,
  active_index_generation_id UUID,
  detail JSONB
)
LANGUAGE sql
STABLE
AS $function$
  SELECT true, false, NULL::UUID,
    '{"consumer":"ready","projection":"not_ready"}'::JSONB
$function$;
SQL

docker run --detach --name "${worker_container}" \
  --network "${network}" \
  --read-only --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  --cap-drop ALL --security-opt no-new-privileges \
  --pids-limit 64 --memory 448m \
  --env RAG_WORKER_DATABASE_URL=postgresql://postgres@postgres/rag_smoke \
  "${image}" >/dev/null

for _ in $(seq 1 30); do
  status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "${worker_container}")"
  if [[ "${status}" == "healthy" ]]; then
    break
  fi
  if [[ "$(docker inspect --format '{{.State.Running}}' "${worker_container}")" != "true" ]]; then
    docker logs "${worker_container}" >&2
    exit 1
  fi
  sleep 1
done

test "$(docker inspect --format '{{.State.Health.Status}}' "${worker_container}")" = "healthy"
test "$(docker exec "${worker_container}" id -u)" = "10001"

docker run --rm --read-only --cap-drop ALL \
  --security-opt no-new-privileges \
  --entrypoint rag-replay "${image}" --help >/dev/null

printf 'Docker smoke passed: rag-worker healthy; rag-replay --help executable\n'
