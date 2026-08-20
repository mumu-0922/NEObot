#!/usr/bin/env bash

# Emit SQL that marks a reviewed migration tail as already applied so a
# disposable drill can exercise and roll back an older schema boundary. The
# caller must delete these synthetic ledger rows before its first down step.
migration_drill_deferred_tail_sql() {
  local backend_dir="$1"
  shift

  command -v python3 >/dev/null 2>&1 || {
    echo "migration drill tail: python3 is required" >&2
    return 1
  }

  python3 - "${backend_dir}" "$@" <<'PY'
import hashlib
import re
import sys
from pathlib import Path

backend_dir = Path(sys.argv[1])
migration_ids = sys.argv[2:]
if not migration_ids:
    raise SystemExit("at least one migration id is required")

print("""CREATE TABLE schema_migrations (
  version BIGINT PRIMARY KEY,
  name TEXT NOT NULL,
  checksum TEXT,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);""")
for migration_id in migration_ids:
    match = re.fullmatch(r"([0-9]+)_([A-Za-z0-9][A-Za-z0-9_-]*)", migration_id)
    if match is None:
        raise SystemExit(f"invalid migration id: {migration_id}")
    version, name = match.groups()
    digest = hashlib.sha256()
    digest.update(migration_id.encode("ascii"))
    digest.update(b"\0")
    digest.update((backend_dir / "migrations" / f"{migration_id}.up.sql").read_bytes())
    digest.update(b"\0")
    digest.update((backend_dir / "migrations" / f"{migration_id}.down.sql").read_bytes())
    print(
        "INSERT INTO schema_migrations(version,name,checksum) "
        f"VALUES ({int(version)},'{name}','{digest.hexdigest()}');"
    )
PY
}
