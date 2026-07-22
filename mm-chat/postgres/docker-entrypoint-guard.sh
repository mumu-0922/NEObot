#!/bin/sh
set -eu

case "${1:-}" in
  postgres|-*)
    pgdata="${PGDATA:-/var/lib/postgresql/data}"
    version_file="${pgdata}/PG_VERSION"
    if [ -f "${version_file}" ]; then
      data_major="$(tr -d '[:space:]' <"${version_file}")"
      if [ "${data_major}" != "17" ]; then
        printf '%s\n' \
          "mm-chat PostgreSQL 17 refused PGDATA major ${data_major:-unknown}." \
          "Restore a PostgreSQL 16 logical backup into a fresh PostgreSQL 17 data directory;" \
          "never mount PostgreSQL 16 PGDATA here." >&2
        exit 78
      fi
    fi
    ;;
esac

exec /usr/local/bin/docker-entrypoint.sh "$@"
