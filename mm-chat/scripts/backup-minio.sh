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

MINIO_BACKUP_DIR="$BACKUP_ROOT/minio"
mkdir -p "$MINIO_BACKUP_DIR"

if ! command -v docker >/dev/null 2>&1; then
  printf 'docker command not found\n' >&2
  exit 127
fi

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
staging_dir="$MINIO_BACKUP_DIR/.staging-${timestamp}"
archive_file="$MINIO_BACKUP_DIR/minio-${timestamp}.tar.gz"
tmp_archive="${archive_file}.tmp"

cleanup() {
  rm -rf "$staging_dir"
  rm -f "$tmp_archive"
}
trap cleanup EXIT

mkdir -p "$staging_dir"

profile_args=()
if [[ -n "${MINIO_CLIENT_PROFILE:-}" ]]; then
  profile_args+=(--profile "$MINIO_CLIENT_PROFILE")
elif [[ -z "${COMPOSE_PROFILES:-}" ]]; then
  profile_args+=(--profile ops)
fi

docker compose "${compose_args[@]}" "${profile_args[@]}" run --rm -T \
  --entrypoint /bin/sh \
  -v "$staging_dir:/backup-target" \
  minio-client -euc '
: "${S3_BUCKET:?S3_BUCKET is required in minio-client container env}"

alias_name="${MC_ALIAS:-minio}"
host_var="MC_HOST_${alias_name}"
host_value="$(printenv "$host_var" 2>/dev/null || true)"

if [ -z "$host_value" ]; then
  endpoint="${S3_ENDPOINT:-${MINIO_ENDPOINT:-http://minio:9000}}"
  access_key="${S3_ACCESS_KEY_ID:-${MINIO_ROOT_USER:-}}"
  secret_key="${S3_SECRET_ACCESS_KEY:-${MINIO_ROOT_PASSWORD:-}}"

  if [ -z "$access_key" ] || [ -z "$secret_key" ]; then
    printf "%s\n" \
      "S3_ACCESS_KEY_ID/S3_SECRET_ACCESS_KEY or MINIO_ROOT_USER/MINIO_ROOT_PASSWORD are required in minio-client container env" >&2
    exit 1
  fi

  mc alias set "$alias_name" "$endpoint" "$access_key" "$secret_key" >/dev/null
fi

bucket_dir="/backup-target/${S3_BUCKET}"
rm -rf "$bucket_dir"
mkdir -p "$bucket_dir"

mc mirror --overwrite --remove "${alias_name}/${S3_BUCKET}" "$bucket_dir"
'

tar -C "$staging_dir" -czf "$tmp_archive" .

if [[ ! -s "$tmp_archive" ]]; then
  printf 'MinIO backup archive is empty: %s\n' "$tmp_archive" >&2
  exit 1
fi

mv "$tmp_archive" "$archive_file"
(cd "$MINIO_BACKUP_DIR" && sha256sum "$(basename "$archive_file")" > "$(basename "$archive_file").sha256")
rm -rf "$staging_dir"
trap - EXIT

printf 'Created MinIO backup:\n  %s\n  %s\n' \
  "$archive_file" \
  "${archive_file}.sha256"
