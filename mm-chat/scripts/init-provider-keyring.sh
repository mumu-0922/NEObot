#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"
target="${1:-${project_dir}/secrets/provider-keyring.json}"
key_id="${2:-provider-$(date -u +%Y%m%dT%H%M%SZ)}"

if [[ "${target}" != /* ]]; then
  target="${project_dir}/${target#./}"
fi
if [[ -e "${target}" || -L "${target}" ]]; then
  echo "provider keyring init: target already exists" >&2
  exit 1
fi

parent="$(dirname "${target}")"
if [[ -L "${parent}" ]]; then
  echo "provider keyring init: parent must not be a symbolic link" >&2
  exit 1
fi
if [[ -e "${parent}" ]]; then
  if [[ ! -d "${parent}" || "$(stat -c '%u' "${parent}")" != "$(id -u)" ]]; then
    echo "provider keyring init: parent must be a user-owned directory" >&2
    exit 1
  fi
  parent_mode="$(stat -c '%a' "${parent}")"
  if (( (8#${parent_mode}) & 077 )); then
    echo "provider keyring init: parent must use mode 700" >&2
    exit 1
  fi
else
  mkdir -p "${parent}"
  chmod 700 "${parent}"
fi

python3 - "${target}" "${key_id}" <<'PY'
import base64
import json
import os
import re
import secrets
import sys
from pathlib import Path

target = Path(sys.argv[1])
key_id = sys.argv[2]
if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,63}", key_id):
    print("provider keyring init: invalid key id", file=sys.stderr)
    raise SystemExit(1)

payload = {
    "v": 1,
    "activeKid": key_id,
    "keys": [
        {
            "kid": key_id,
            "key": base64.urlsafe_b64encode(secrets.token_bytes(32))
            .decode("ascii")
            .rstrip("="),
        }
    ],
}
encoded = (json.dumps(payload, separators=(",", ":")) + "\n").encode("utf-8")
flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0)
descriptor = os.open(target, flags, 0o600)
try:
    with os.fdopen(descriptor, "wb") as handle:
        handle.write(encoded)
        handle.flush()
        os.fsync(handle.fileno())
except BaseException:
    target.unlink(missing_ok=True)
    raise
PY

echo "provider keyring init: created ${target}"
