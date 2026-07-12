#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${MM_CHAT_TEST_DATABASE_URL:-}" ]]; then
  echo "MM_CHAT_TEST_DATABASE_URL is required; use a disposable PostgreSQL 16 database" >&2
  exit 1
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
backend_dir="$(cd "${script_dir}/../backend" && pwd)"

export MM_CHAT_REQUIRE_POSTGRES_TESTS=true
export GOCACHE="${GOCACHE:-/tmp/mm-chat-go-cache}"

cd "${backend_dir}"
go test -count=1 -race ./internal/knowledge ./internal/migration
