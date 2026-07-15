#!/usr/bin/env bash
set -euo pipefail

env_file="${1:-.env.single-server}"

if [[ -L "${env_file}" ]]; then
  echo "single-server preflight: env file must not be a symbolic link" >&2
  exit 1
fi

if [[ ! -f "${env_file}" ]]; then
  echo "single-server preflight: env file not found" >&2
  exit 1
fi

if [[ "$(basename "${env_file}")" == ".env.single-server.example" ]]; then
  echo "single-server preflight: example env cannot be promoted" >&2
  exit 1
fi

mode="$(stat -c '%a' "${env_file}")"
if (( (8#${mode}) & 077 )); then
  echo "single-server preflight: env file must not be group/world accessible (use chmod 600)" >&2
  exit 1
fi

owner="$(stat -c '%u' "${env_file}")"
if [[ "${owner}" != "$(id -u)" ]]; then
  echo "single-server preflight: env file must be owned by the invoking user" >&2
  exit 1
fi

python3 - "${env_file}" <<'PY'
import re
import sys
import ipaddress
from pathlib import Path
from urllib.parse import unquote, urlsplit


def fail(message: str) -> None:
    print(f"single-server preflight: {message}", file=sys.stderr)
    raise SystemExit(1)


def parse_env(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for number, raw_line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        line = raw_line.strip()
        if not line or line.startswith("#"):
            continue
        if line != raw_line or line.startswith("export "):
            fail(f"unsupported env syntax at line {number}")
        if "=" not in line:
            fail(f"invalid env assignment at line {number}")
        key, value = line.split("=", 1)
        if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key):
            fail(f"invalid env name at line {number}")
        if key.startswith(("COMPOSE_", "DOCKER_")):
            fail(f"reserved env name at line {number}")
        if key in values:
            fail(f"duplicate env name at line {number}")
        if any(
            character.isspace()
            or ord(character) < 0x20
            or ord(character) == 0x7F
            or character in "\"'\\#$"
            for character in value
        ):
            fail(f"{key} uses unsupported quoting, escaping, comment, or interpolation syntax")
        values[key] = value
    return values


def valid_hostname(value: str) -> bool:
    try:
        ipaddress.ip_address(value)
        return True
    except ValueError:
        pass
    if len(value) > 253:
        return False
    labels = value.rstrip(".").split(".")
    return bool(labels) and all(
        re.fullmatch(r"[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?", label)
        for label in labels
    )


def valid_image_digest(value: str) -> bool:
    if "@sha256:" not in value:
        return False
    reference, digest = value.rsplit("@sha256:", 1)
    if not re.fullmatch(r"[0-9a-f]{64}", digest):
        return False
    parts = reference.split("/")
    if len(parts) < 2 or any(not part for part in parts):
        return False
    registry = parts[0]
    if registry.endswith(":"):
        return False
    if "." not in registry and ":" not in registry:
        return False
    try:
        parsed_registry = urlsplit("//" + registry)
        _ = parsed_registry.port
    except ValueError:
        return False
    if not parsed_registry.hostname or not valid_hostname(parsed_registry.hostname):
        return False
    if parsed_registry.username is not None or parsed_registry.password is not None:
        return False
    repository_segment = re.compile(r"[a-z0-9]+(?:[._-][a-z0-9]+)*")
    return all(repository_segment.fullmatch(part) for part in parts[1:])


values = parse_env(Path(sys.argv[1]))
for key, value in values.items():
    if "$" in value:
        fail(f"{key} uses forbidden env interpolation syntax")

required = (
    "FRONTEND_IMAGE",
    "BACKEND_IMAGE",
    "RAG_IMAGE",
    "MM_CHAT_VERSION",
    "MIGRATION_DATABASE_URL",
    "DATABASE_URL",
    "RAG_WORKER_DATABASE_URL",
    "RAG_REPLAY_DATABASE_URL",
    "POSTGRES_DB",
    "POSTGRES_USER",
    "POSTGRES_PASSWORD",
    "REDIS_URL",
    "REDIS_PASSWORD",
    "MINIO_ROOT_USER",
    "MINIO_ROOT_PASSWORD",
    "S3_ACCESS_KEY_ID",
    "S3_SECRET_ACCESS_KEY",
    "TEAM_CURSOR_ACTIVE_KEY_ID",
    "TEAM_CURSOR_KEYRING",
    "TEAM_MAIL_ACTIVE_KEY_ID",
    "TEAM_MAIL_KEYRING",
    "TEAM_INVITE_ACCEPT_URL_BASE",
    "PROVIDER_BASE_URL",
    "PROVIDER_MODEL",
    "PROVIDER_API_KEY",
)
for key in required:
    if not values.get(key, "").strip():
        fail(f"{key} is required")

placeholder = re.compile(
    r"change-me|replace-with|your-|\.example(?:[/:]|$)|example\.(?:com|net|org)",
    re.IGNORECASE,
)
for key in required:
    if placeholder.search(values[key]):
        fail(f"{key} still contains a placeholder")

if values.get("AUTH_MODE") != "required":
    fail("AUTH_MODE must be required for promotion")
if values.get("RAG_WORKER_DISPATCH_ENABLED") != "false":
    fail("RAG_WORKER_DISPATCH_ENABLED must remain false in Phase 15.2B")
if values.get("RAG_WORKER_JOB_STAGES", ""):
    fail("RAG_WORKER_JOB_STAGES must remain empty in Phase 15.2B")

image = values["BACKEND_IMAGE"]
if not valid_image_digest(image):
    fail("BACKEND_IMAGE must use a full immutable sha256 registry digest")
if not valid_image_digest(values["FRONTEND_IMAGE"]):
    fail("FRONTEND_IMAGE must use a full immutable sha256 registry digest")
if not valid_image_digest(values["RAG_IMAGE"]):
    fail("RAG_IMAGE must use a full immutable sha256 registry digest")

if values["MM_CHAT_VERSION"].lower() in {"dev", "local", "single-server-dev"}:
    fail("MM_CHAT_VERSION must identify the release")

database_urls = {}
for key in (
    "MIGRATION_DATABASE_URL",
    "DATABASE_URL",
    "RAG_WORKER_DATABASE_URL",
    "RAG_REPLAY_DATABASE_URL",
):
    try:
        parsed = urlsplit(values[key])
        _ = parsed.port
    except ValueError:
        fail(f"{key} must be a PostgreSQL URL")
    if (
        parsed.scheme not in {"postgres", "postgresql"}
        or not parsed.hostname
        or not parsed.username
        or not parsed.password
    ):
        fail(f"{key} must be a PostgreSQL URL with user and password")
    database_urls[key] = parsed

migration_database = database_urls["MIGRATION_DATABASE_URL"]
if unquote(migration_database.username or "") != values["POSTGRES_USER"]:
    fail("MIGRATION_DATABASE_URL user does not match POSTGRES_USER")
if unquote(migration_database.password or "") != values["POSTGRES_PASSWORD"]:
    fail("MIGRATION_DATABASE_URL password does not match POSTGRES_PASSWORD")
if unquote(migration_database.path.lstrip("/")) != values["POSTGRES_DB"]:
    fail("MIGRATION_DATABASE_URL database does not match POSTGRES_DB")

for key, parsed in database_urls.items():
    if parsed.hostname != migration_database.hostname:
        fail(f"{key} host must match MIGRATION_DATABASE_URL")
    if unquote(parsed.path.lstrip("/")) != values["POSTGRES_DB"]:
        fail(f"{key} database does not match POSTGRES_DB")

database_users = [unquote(parsed.username or "") for parsed in database_urls.values()]
if len(set(database_users)) != len(database_users):
    fail("migration, API, RAG worker, and RAG replay must use distinct database principals")

database_passwords = [
    unquote(parsed.password or "") for parsed in database_urls.values()
]
if len(set(database_passwords)) != len(database_passwords):
    fail("database principals must use distinct passwords")

redis = urlsplit(values["REDIS_URL"])
if redis.scheme not in {"redis", "rediss"} or not redis.hostname:
    fail("REDIS_URL must be a Redis URL")
if unquote(redis.password or "") != values["REDIS_PASSWORD"]:
    fail("REDIS_URL password does not match REDIS_PASSWORD")

try:
    invite_url = urlsplit(values["TEAM_INVITE_ACCEPT_URL_BASE"])
    _ = invite_url.port
except ValueError:
    fail("TEAM_INVITE_ACCEPT_URL_BASE must be a valid HTTPS URL")
if (
    invite_url.scheme != "https"
    or not invite_url.hostname
    or not valid_hostname(invite_url.hostname)
):
    fail("TEAM_INVITE_ACCEPT_URL_BASE must be a valid HTTPS URL")
if invite_url.username is not None or invite_url.password is not None:
    fail("TEAM_INVITE_ACCEPT_URL_BASE must not contain user info")
if invite_url.fragment:
    fail("TEAM_INVITE_ACCEPT_URL_BASE must not contain a fragment")
for pair in invite_url.query.split("&"):
    key = unquote(pair.split("=", 1)[0]).strip()
    if key.casefold() == "token":
        fail("TEAM_INVITE_ACCEPT_URL_BASE must not contain a token query parameter")

try:
    provider_url = urlsplit(values["PROVIDER_BASE_URL"])
    _ = provider_url.port
except ValueError:
    fail("PROVIDER_BASE_URL must be a valid HTTPS URL")
if (
    provider_url.scheme != "https"
    or not provider_url.hostname
    or not valid_hostname(provider_url.hostname)
):
    fail("PROVIDER_BASE_URL must be an HTTPS URL")
if provider_url.username is not None or provider_url.password is not None:
    fail("PROVIDER_BASE_URL must not contain user info")
if provider_url.fragment:
    fail("PROVIDER_BASE_URL must not contain a fragment")

print("single-server preflight: passed")
PY
