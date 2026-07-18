#!/usr/bin/env bash
set -euo pipefail

if (( $# < 3 || $# > 4 )); then
  echo "usage: rotate-provider-keyring.sh prepare <source> <target> [new-key-id] | rotate-provider-keyring.sh prune <source> <target>" >&2
  exit 2
fi

action="$1"
source_file="$2"
target_file="$3"
key_id="${4:-}"
if [[ "${action}" != "prepare" && "${action}" != "prune" ]]; then
  echo "provider keyring rotation: action must be prepare or prune" >&2
  exit 2
fi
if [[ "${action}" == "prune" && -n "${key_id}" ]]; then
  echo "provider keyring rotation: prune does not accept a key id" >&2
  exit 2
fi
if [[ "${action}" == "prepare" && -z "${key_id}" ]]; then
  key_id="provider-$(date -u +%Y%m%dT%H%M%SZ)"
fi

python3 - "${action}" "${source_file}" "${target_file}" "${key_id}" <<'PY'
import base64
import binascii
import json
import os
import re
import secrets
import stat
import sys
from pathlib import Path


def fail(message: str) -> None:
    print(f"provider keyring rotation: {message}", file=sys.stderr)
    raise SystemExit(1)


class DuplicateKey(ValueError):
    pass


def unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise DuplicateKey
        result[key] = value
    return result


def validate_keyring(value: object) -> dict[str, object]:
    if not isinstance(value, dict) or set(value) != {"v", "activeKid", "keys"}:
        fail("source keyring is invalid")
    active_kid = value.get("activeKid")
    keys = value.get("keys")
    key_id_pattern = re.compile(r"[A-Za-z0-9][A-Za-z0-9._-]{0,63}")
    if (
        value.get("v") != 1
        or not isinstance(active_kid, str)
        or not key_id_pattern.fullmatch(active_kid)
        or not isinstance(keys, list)
        or not 1 <= len(keys) <= 16
    ):
        fail("source keyring is invalid")
    seen: set[str] = set()
    for item in keys:
        if not isinstance(item, dict) or set(item) != {"kid", "key"}:
            fail("source keyring is invalid")
        kid = item.get("kid")
        encoded = item.get("key")
        if (
            not isinstance(kid, str)
            or not key_id_pattern.fullmatch(kid)
            or kid in seen
            or not isinstance(encoded, str)
        ):
            fail("source keyring is invalid")
        try:
            decoded = base64.b64decode(
                encoded + "=" * (-len(encoded) % 4),
                altchars=b"-_",
                validate=True,
            )
        except (ValueError, binascii.Error):
            fail("source keyring is invalid")
        if (
            len(decoded) != 32
            or base64.urlsafe_b64encode(decoded).decode().rstrip("=") != encoded
        ):
            fail("source keyring is invalid")
        seen.add(kid)
    if active_kid not in seen:
        fail("source keyring is invalid")
    return value


action, source_raw, target_raw, new_kid = sys.argv[1:]
source = Path(source_raw).resolve(strict=False)
target = Path(target_raw).resolve(strict=False)
try:
    source_info = Path(source_raw).lstat()
except OSError:
    fail("source keyring is unavailable")
if stat.S_ISLNK(source_info.st_mode) or not stat.S_ISREG(source_info.st_mode):
    fail("source keyring must be a regular non-symlink file")
if source_info.st_uid != os.getuid() or stat.S_IMODE(source_info.st_mode) != 0o600:
    fail("source keyring must be user-owned with mode 600")
if source_info.st_size < 1 or source_info.st_size > 65536:
    fail("source keyring is invalid")

parent_raw = Path(target_raw).parent
try:
    parent_info = parent_raw.lstat()
except OSError:
    fail("target parent is unavailable")
if (
    stat.S_ISLNK(parent_info.st_mode)
    or not stat.S_ISDIR(parent_info.st_mode)
    or parent_info.st_uid != os.getuid()
    or stat.S_IMODE(parent_info.st_mode) != 0o700
):
    fail("target parent must be a user-owned mode-700 non-symlink directory")
if source == target or Path(target_raw).exists() or Path(target_raw).is_symlink():
    fail("target already exists")

try:
    payload = validate_keyring(
        json.loads(
            Path(source_raw).read_text(encoding="utf-8"),
            object_pairs_hook=unique_object,
        )
    )
except (OSError, UnicodeError, json.JSONDecodeError, DuplicateKey):
    fail("source keyring is invalid")

keys = payload["keys"]
if action == "prepare":
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,63}", new_kid):
        fail("new key id is invalid")
    if len(keys) >= 16 or any(item["kid"] == new_kid for item in keys):
        fail("new key id is invalid")
    next_payload = {
        "v": 1,
        "activeKid": new_kid,
        "keys": [
            {
                "kid": new_kid,
                "key": base64.urlsafe_b64encode(secrets.token_bytes(32))
                .decode("ascii")
                .rstrip("="),
            },
            *keys,
        ],
    }
else:
    active = next(item for item in keys if item["kid"] == payload["activeKid"])
    next_payload = {"v": 1, "activeKid": payload["activeKid"], "keys": [active]}

encoded = (json.dumps(next_payload, separators=(",", ":")) + "\n").encode("utf-8")
flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0)
descriptor = os.open(target_raw, flags, 0o600)
try:
    with os.fdopen(descriptor, "wb") as handle:
        handle.write(encoded)
        handle.flush()
        os.fsync(handle.fileno())
except BaseException:
    Path(target_raw).unlink(missing_ok=True)
    raise
PY

echo "provider keyring rotation: created ${target_file}"
