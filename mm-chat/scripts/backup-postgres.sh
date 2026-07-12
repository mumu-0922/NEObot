#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
MM_CHAT_DIR="$(cd -- "$SCRIPT_DIR/.." && pwd)"
CALLER_PWD="$(pwd)"

resolve_path() {
  local path="$1"
  case "$path" in
    /*) printf '%s\n' "$path" ;;
    *) printf '%s\n' "$CALLER_PWD/$path" ;;
  esac
}

normalize_compose_file() {
  local input="$1"
  local old_ifs="$IFS"
  local parts=()
  local normalized=()
  local part

  IFS=':' read -r -a parts <<< "$input"
  IFS="$old_ifs"

  for part in "${parts[@]}"; do
    if [[ -z "$part" ]]; then
      continue
    fi
    normalized+=("$(resolve_path "$part")")
  done

  local joined=""
  for part in "${normalized[@]}"; do
    if [[ -n "$joined" ]]; then
      joined+=":"
    fi
    joined+="$part"
  done

  printf '%s\n' "$joined"
}

validate_compose_files() {
  local input="$1"
  local old_ifs="$IFS"
  local parts=()
  local part

  IFS=':' read -r -a parts <<< "$input"
  IFS="$old_ifs"

  for part in "${parts[@]}"; do
    if [[ ! -f "$part" ]]; then
      printf 'Compose file not found: %s\n' "$part" >&2
      exit 1
    fi
  done
}

if [[ -n "${COMPOSE_FILE:-}" ]]; then
  COMPOSE_FILE="$(normalize_compose_file "$COMPOSE_FILE")"
else
  COMPOSE_FILE="$MM_CHAT_DIR/compose.single-server.yml"
fi

validate_compose_files "$COMPOSE_FILE"

compose_args=(--project-directory "$MM_CHAT_DIR")
if [[ -n "${PROJECT_NAME:-}" ]]; then
  compose_args+=(--project-name "$PROJECT_NAME")
elif [[ -n "${COMPOSE_PROJECT_NAME:-}" ]]; then
  compose_args+=(--project-name "$COMPOSE_PROJECT_NAME")
fi
if [[ -n "${ENV_FILE:-}" ]]; then
  compose_args+=(--env-file "$(resolve_path "$ENV_FILE")")
elif [[ -n "${COMPOSE_ENV_FILE:-}" ]]; then
  compose_args+=(--env-file "$(resolve_path "$COMPOSE_ENV_FILE")")
elif [[ -f "$MM_CHAT_DIR/.env.single-server" ]]; then
  compose_args+=(--env-file "$MM_CHAT_DIR/.env.single-server")
elif [[ -f "$MM_CHAT_DIR/.env.single-server.example" ]]; then
  compose_args+=(--env-file "$MM_CHAT_DIR/.env.single-server.example")
fi

old_ifs="$IFS"
IFS=':' read -r -a compose_files <<< "$COMPOSE_FILE"
IFS="$old_ifs"
for compose_file in "${compose_files[@]}"; do
  [[ -n "$compose_file" ]] && compose_args+=(-f "$compose_file")
done

if [[ -n "${BACKUP_DIR:-}" ]]; then
  BACKUP_ROOT="$(resolve_path "$BACKUP_DIR")"
else
  BACKUP_ROOT="$MM_CHAT_DIR/backup"
fi

POSTGRES_BACKUP_DIR="$BACKUP_ROOT/postgres"
mkdir -p "$POSTGRES_BACKUP_DIR"

if ! command -v docker >/dev/null 2>&1; then
  printf 'docker command not found\n' >&2
  exit 127
fi

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_file="$POSTGRES_BACKUP_DIR/postgres-${timestamp}.dump"
tmp_file="${backup_file}.tmp"

cleanup() {
  rm -f "$tmp_file"
}
trap cleanup EXIT

docker compose "${compose_args[@]}" exec -T postgres sh -ceu '
: "${POSTGRES_USER:?POSTGRES_USER is required in postgres container env}"
db="${POSTGRES_DB:-$POSTGRES_USER}"

if [ -n "${POSTGRES_PASSWORD:-}" ]; then
  export PGPASSWORD="$POSTGRES_PASSWORD"
fi

export PGHOST="${PGHOST:-127.0.0.1}"
export PGPORT="${PGPORT:-5432}"

exec pg_dump \
  --format=custom \
  --no-owner \
  --no-acl \
  --username="$POSTGRES_USER" \
  "$db"
' > "$tmp_file"

if [[ ! -s "$tmp_file" ]]; then
  printf 'Postgres backup is empty: %s\n' "$tmp_file" >&2
  exit 1
fi

mv "$tmp_file" "$backup_file"
(cd "$POSTGRES_BACKUP_DIR" && sha256sum "$(basename "$backup_file")" > "$(basename "$backup_file").sha256")
trap - EXIT

printf 'Created Postgres backup:\n  %s\n  %s\n' \
  "$backup_file" \
  "${backup_file}.sha256"
