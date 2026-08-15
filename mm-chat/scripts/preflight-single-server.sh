#!/usr/bin/env bash
set -euo pipefail

env_file="${1:-.env.single-server}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
project_dir="$(cd "${script_dir}/.." && pwd)"

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

python3 - "${env_file}" "${project_dir}" <<'PY'
import base64
import binascii
import hashlib
import json
import os
import re
import ssl
import stat
import subprocess
import sys
import ipaddress
from datetime import datetime, timezone
from pathlib import Path
from urllib.parse import unquote, urlsplit


def fail(message: str) -> None:
    print(f"single-server preflight: {message}", file=sys.stderr)
    raise SystemExit(1)


def parse_byok_private_key(value: str) -> str:
    if len(value) > 32768 or not (
        value.startswith("'") and value.endswith("'")
    ):
        fail("BYOK_PRIVATE_KEY_PEM must use a bounded single-quoted escaped PEM")
    inner = value[1:-1]
    if "'" in inner or "\\" in inner.replace(r"\n", ""):
        fail("BYOK_PRIVATE_KEY_PEM contains unsupported escaping")
    decoded = inner.replace(r"\n", "\n")
    match = re.fullmatch(
        r"-----BEGIN (?P<label>RSA PRIVATE KEY|PRIVATE KEY)-----\n"
        r"(?P<body>(?:[A-Za-z0-9+/]+={0,2}\n)+)"
        r"-----END (?P=label)-----\n?",
        decoded,
    )
    if match is None:
        fail("BYOK_PRIVATE_KEY_PEM must contain one escaped RSA private PEM")
    try:
        der = base64.b64decode("".join(match.group("body").splitlines()), validate=True)
    except (binascii.Error, ValueError):
        fail("BYOK_PRIVATE_KEY_PEM body must use canonical base64")
    if not der or len(der) > 16384:
        fail("BYOK_PRIVATE_KEY_PEM body size is invalid")
    return inner


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
        if key == "BYOK_PRIVATE_KEY_PEM" and value:
            value = parse_byok_private_key(value)
        elif any(
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


def parse_simple_duration(name: str, value: str) -> int:
    match = re.fullmatch(r"([1-9][0-9]*)(m|h)", value)
    if match is None:
        fail(f"{name} must use a positive whole-minute or whole-hour duration")
    amount = int(match.group(1))
    return amount * (60 if match.group(2) == "m" else 3600)


def parse_simple_duration_seconds(name: str, value: str) -> int:
    match = re.fullmatch(r"([1-9][0-9]*)(s|m|h)", value)
    if match is None:
        fail(f"{name} must use a positive whole-second, whole-minute, or whole-hour duration")
    amount = int(match.group(1))
    multiplier = {"s": 1, "m": 60, "h": 3600}[match.group(2)]
    return amount * multiplier


def validate_private_token(path_value: str, name: str) -> None:
    path = Path(path_value)
    if not path.is_absolute():
        path = Path(sys.argv[2]) / path
    try:
        metadata = path.lstat()
    except OSError:
        fail(f"{name} file is unavailable")
    if stat.S_ISLNK(metadata.st_mode) or not stat.S_ISREG(metadata.st_mode):
        fail(f"{name} must be a regular non-symlink file")
    if metadata.st_uid != os.getuid():
        fail(f"{name} must be owned by the invoking user")
    if stat.S_IMODE(metadata.st_mode) & 0o077:
        fail(f"{name} must use mode 600")
    if metadata.st_size < 32 or metadata.st_size > 4096:
        fail(f"{name} must contain 32 to 4096 bytes")
    try:
        raw = path.read_bytes()
        token = raw.decode("utf-8").strip()
    except (OSError, UnicodeError):
        fail(f"{name} is invalid")
    if (
        len(token) < 32
        or len(token) > 4096
        or "\r" in token
        or "\n" in token
        or raw.strip() != token.encode("utf-8")
    ):
        fail(f"{name} is invalid")


def resolve_secure_file(
    path_value: str,
    name: str,
    *,
    private: bool,
    minimum_size: int = 1,
    maximum_size: int = 1 << 20,
) -> Path:
    path = Path(path_value)
    if not path.is_absolute():
        path = Path(sys.argv[2]) / path
    try:
        metadata = path.lstat()
    except OSError:
        fail(f"{name} file is unavailable")
    if stat.S_ISLNK(metadata.st_mode) or not stat.S_ISREG(metadata.st_mode):
        fail(f"{name} must be a regular non-symlink file")
    if metadata.st_uid != os.getuid():
        fail(f"{name} must be owned by the invoking user")
    mode = stat.S_IMODE(metadata.st_mode)
    if mode & (0o077 if private else 0o022):
        expected = "use mode 600" if private else "not be group/world writable"
        fail(f"{name} must {expected}")
    if not minimum_size <= metadata.st_size <= maximum_size:
        fail(f"{name} has an invalid size")
    return path


def resolve_secure_directory(path_value: str, name: str) -> Path:
    path = Path(path_value)
    if not path.is_absolute():
        path = Path(sys.argv[2]) / path
    try:
        metadata = path.lstat()
    except OSError:
        fail(f"{name} directory is unavailable")
    if stat.S_ISLNK(metadata.st_mode) or not stat.S_ISDIR(metadata.st_mode):
        fail(f"{name} must be a non-symlink directory")
    if stat.S_IMODE(metadata.st_mode) & 0o022:
        fail(f"{name} must not be group/world writable")
    return path


values = parse_env(Path(sys.argv[1]))
for key, value in values.items():
    if "$" in value:
        fail(f"{key} uses forbidden env interpolation syntax")

retired_provider_env = (
    "RAG_MINERU_API_TOKEN",
    "DEFAULT_MINERU_API_TOKEN",
    "RAG_JINA_API_KEY",
    "DEFAULT_JINA_API_KEY",
    "RAG_QUERY_GATEWAY_URL",
    "RAG_RERANK_GATEWAY_URL",
    "DEFAULT_ELEVENLABS_API_KEY",
    "DEFAULT_ELEVENLABS_STT_MODEL",
    "DEFAULT_ELEVENLABS_TTS_MODEL",
    "DEFAULT_ELEVENLABS_TTS_VOICE_ID",
    "DEFAULT_MIMO_API_KEY",
    "DEFAULT_MIMO_STT_MODEL",
    "DEFAULT_MIMO_TTS_MODEL",
    "DEFAULT_MIMO_TTS_VOICE_ID",
)
for key in retired_provider_env:
    if key in values:
        fail(f"{key} is retired; configure providers through the server authority")

if (
    "MEMORY_LEXICAL_SHADOW_ENABLED" in values
    and values["MEMORY_LEXICAL_SHADOW_ENABLED"] not in {"true", "false"}
):
    fail("MEMORY_LEXICAL_SHADOW_ENABLED must be true or false")

if (
    "MEMORY_HYBRID_SHADOW_ENABLED" in values
    and values["MEMORY_HYBRID_SHADOW_ENABLED"] not in {"true", "false"}
):
    fail("MEMORY_HYBRID_SHADOW_ENABLED must be true or false")

if (
    "MEMORY_TOOL_LOOP_ENABLED" in values
    and values["MEMORY_TOOL_LOOP_ENABLED"] not in {"true", "false"}
):
    fail("MEMORY_TOOL_LOOP_ENABLED must be true or false")

if values.get("MEMORY_TOOL_LOOP_CANARY_USER_IDS", ""):
    uuid_pattern = re.compile(
        r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-"
        r"[0-9a-fA-F]{4}-[0-9a-fA-F]{12}"
    )
    canary_ids = [value.strip() for value in values["MEMORY_TOOL_LOOP_CANARY_USER_IDS"].split(",")]
    normalized_canary_ids = [value.lower() for value in canary_ids]
    if (
        any(not uuid_pattern.fullmatch(value) for value in canary_ids)
        or len(set(normalized_canary_ids)) != len(normalized_canary_ids)
    ):
        fail("MEMORY_TOOL_LOOP_CANARY_USER_IDS must be a unique comma-separated UUID list")

for key in (
    "MEMORY_L2_SCENE_SHADOW_ENABLED",
    "MEMORY_L2_SCENE_READER_ENABLED",
    "MEMORY_L3_PERSONA_SHADOW_ENABLED",
    "MEMORY_L3_PERSONA_READER_ENABLED",
):
    if key in values and values[key] not in {"true", "false"}:
        fail(f"{key} must be true or false")

for key in ("MCP_ENABLED", "MCP_REMOTE_ENABLED", "MCP_STDIO_ENABLED", "MCP_MARKETPLACE_ENABLED"):
    if key in values and values[key] not in {"true", "false"}:
        fail(f"{key} must be true or false")

if values.get("MCP_STDIO_ENABLED") == "true" and values.get("MCP_ENABLED") != "true":
    fail("MCP_STDIO_ENABLED requires MCP_ENABLED=true")
if values.get("MCP_MARKETPLACE_ENABLED") == "true" and values.get("MCP_ENABLED") != "true":
    fail("MCP_MARKETPLACE_ENABLED requires MCP_ENABLED=true")
if values.get("MCP_MARKETPLACE_ENABLED") == "true" and values.get("MCP_REMOTE_ENABLED") != "true":
    fail("MCP_MARKETPLACE_ENABLED requires MCP_REMOTE_ENABLED=true")

agent_flags = (
    "AGENT_RUNNER_CONTROL_ENABLED",
    "AGENT_ROOT_RUN_CANARY_ENABLED",
    "AGENT_BROKER_ARTIFACT_CANARY_ENABLED",
    "AGENT_PROJECT_MUTATION_CANARY_ENABLED",
    "AGENT_CHILD_CANARY_ENABLED",
    "AGENT_CRON_WORKER_ENABLED",
    "AGENT_DRAFT_LEARNING_WORKER_ENABLED",
    "AGENT_RUNTIME_ENABLED",
    "AGENT_SCHEDULER_ENABLED",
    "AGENT_SKILL_INSTALL_ENABLED",
    "AGENT_LEARNING_ENABLED",
    "AGENT_DELEGATION_ENABLED",
    "AGENT_BROKER_READ_ONLY_ENABLED",
    "AGENT_BROKER_MUTATION_ENABLED",
)
for key in agent_flags:
    if values.get(key) not in {"true", "false"}:
        fail(f"{key} must be true or false")
for key in (
    "AGENT_RUNTIME_ENABLED",
    "AGENT_SCHEDULER_ENABLED",
    "AGENT_SKILL_INSTALL_ENABLED",
    "AGENT_LEARNING_ENABLED",
    "AGENT_BROKER_READ_ONLY_ENABLED",
    "AGENT_BROKER_MUTATION_ENABLED",
):
    if values[key] != "false":
        fail(f"{key} must remain false through G21.5")
if values["AGENT_DELEGATION_ENABLED"] != values["AGENT_CHILD_CANARY_ENABLED"]:
    fail("AGENT_DELEGATION_ENABLED must match the dedicated G21.4 Child canary flag")

required = (
    "FRONTEND_IMAGE",
    "BACKEND_IMAGE",
    "RAG_IMAGE",
    "POSTGRES_IMAGE",
    "POSTGRES_DATA_DIR",
    "MM_CHAT_VERSION",
    "MM_CHAT_RUNTIME_UID",
    "MM_CHAT_RUNTIME_GID",
    "MIGRATION_DATABASE_URL",
    "DATABASE_URL",
    "MEMORY_WORKER_DATABASE_URL",
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
    "PROVIDER_SECRET_KEYRING_SOURCE",
    "MCP_ENABLED",
    "MCP_REMOTE_ENABLED",
    "MCP_STDIO_ENABLED",
    "MCP_AUDIT_RETENTION",
    "MCP_CLEANUP_INTERVAL",
    "MCP_MARKETPLACE_ENABLED",
    "MCP_MARKETPLACE_TIMEOUT",
    "MCP_MARKETPLACE_CACHE_TTL",
    *agent_flags,
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

runtime_ids: dict[str, int] = {}
for key in ("MM_CHAT_RUNTIME_UID", "MM_CHAT_RUNTIME_GID"):
    if not re.fullmatch(r"[1-9][0-9]{0,9}", values[key]):
        fail(f"{key} must be a positive numeric ID")
    runtime_ids[key] = int(values[key])
    if runtime_ids[key] > 2_147_483_647:
        fail(f"{key} must be a positive numeric ID")
if runtime_ids["MM_CHAT_RUNTIME_UID"] != os.getuid():
    fail("MM_CHAT_RUNTIME_UID must match the invoking user")
if runtime_ids["MM_CHAT_RUNTIME_GID"] != os.getgid():
    fail("MM_CHAT_RUNTIME_GID must match the invoking user's primary group")

keyring_path = Path(values["PROVIDER_SECRET_KEYRING_SOURCE"])
if not keyring_path.is_absolute():
    keyring_path = Path(sys.argv[2]) / keyring_path
try:
    keyring_lstat = keyring_path.lstat()
except OSError:
    fail("PROVIDER_SECRET_KEYRING_SOURCE file is unavailable")
if stat.S_ISLNK(keyring_lstat.st_mode) or not stat.S_ISREG(keyring_lstat.st_mode):
    fail("PROVIDER_SECRET_KEYRING_SOURCE must be a regular non-symlink file")
if keyring_lstat.st_uid != os.getuid():
    fail("PROVIDER_SECRET_KEYRING_SOURCE must be owned by the invoking user")
if stat.S_IMODE(keyring_lstat.st_mode) & 0o077:
    fail("PROVIDER_SECRET_KEYRING_SOURCE must use mode 600")
if keyring_lstat.st_size < 1 or keyring_lstat.st_size > 65536:
    fail("PROVIDER_SECRET_KEYRING_SOURCE has an invalid size")

class DuplicateKey(ValueError):
    pass

def unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise DuplicateKey
        result[key] = value
    return result

try:
    keyring = json.loads(
        keyring_path.read_text(encoding="utf-8"),
        object_pairs_hook=unique_object,
    )
except (OSError, UnicodeError, json.JSONDecodeError, DuplicateKey):
    fail("PROVIDER_SECRET_KEYRING_SOURCE is invalid")
if not isinstance(keyring, dict) or set(keyring) != {"v", "activeKid", "keys"}:
    fail("PROVIDER_SECRET_KEYRING_SOURCE is invalid")
active_kid = keyring.get("activeKid")
keys = keyring.get("keys")
if (
    keyring.get("v") != 1
    or not isinstance(active_kid, str)
    or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,63}", active_kid)
    or not isinstance(keys, list)
    or not 1 <= len(keys) <= 16
):
    fail("PROVIDER_SECRET_KEYRING_SOURCE is invalid")
key_ids: set[str] = set()
for item in keys:
    if not isinstance(item, dict) or set(item) != {"kid", "key"}:
        fail("PROVIDER_SECRET_KEYRING_SOURCE is invalid")
    kid = item.get("kid")
    encoded = item.get("key")
    if (
        not isinstance(kid, str)
        or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9._-]{0,63}", kid)
        or kid in key_ids
        or not isinstance(encoded, str)
    ):
        fail("PROVIDER_SECRET_KEYRING_SOURCE is invalid")
    try:
        decoded = base64.b64decode(
            encoded + "=" * (-len(encoded) % 4),
            altchars=b"-_",
            validate=True,
        )
    except (ValueError, binascii.Error):
        fail("PROVIDER_SECRET_KEYRING_SOURCE is invalid")
    if len(decoded) != 32 or base64.urlsafe_b64encode(decoded).decode().rstrip("=") != encoded:
        fail("PROVIDER_SECRET_KEYRING_SOURCE is invalid")
    key_ids.add(kid)
if active_kid not in key_ids:
    fail("PROVIDER_SECRET_KEYRING_SOURCE is invalid")

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
if not valid_image_digest(values["POSTGRES_IMAGE"]):
    fail("POSTGRES_IMAGE must use a full immutable sha256 registry digest")

audit_retention = parse_simple_duration("MCP_AUDIT_RETENTION", values["MCP_AUDIT_RETENTION"])
if not 24 * 3600 <= audit_retention <= 365 * 24 * 3600:
    fail("MCP_AUDIT_RETENTION must be between 24h and 8760h")
cleanup_interval = parse_simple_duration("MCP_CLEANUP_INTERVAL", values["MCP_CLEANUP_INTERVAL"])
if not 60 <= cleanup_interval <= 24 * 3600:
    fail("MCP_CLEANUP_INTERVAL must be between 1m and 24h")

if values["MCP_STDIO_ENABLED"] == "true":
    for key in ("MCP_RUNNER_IMAGE", "MCP_RUNNER_URL", "MCP_RUNNER_TOKEN_SOURCE"):
        if not values.get(key, "").strip():
            fail(f"{key} is required when MCP stdio is enabled")
    if not valid_image_digest(values["MCP_RUNNER_IMAGE"]):
        fail("MCP_RUNNER_IMAGE must use a full immutable sha256 registry digest")
    try:
        runner_url = urlsplit(values["MCP_RUNNER_URL"])
        runner_port = runner_url.port
    except ValueError:
        fail("MCP_RUNNER_URL must be the private http://mcp-runner:8090 service URL")
    if (
        runner_url.scheme != "http"
        or runner_url.hostname != "mcp-runner"
        or runner_port != 8090
        or runner_url.username is not None
        or runner_url.password is not None
        or runner_url.path not in {"", "/"}
        or runner_url.query
        or runner_url.fragment
    ):
        fail("MCP_RUNNER_URL must be the private http://mcp-runner:8090 service URL")
    validate_private_token(values["MCP_RUNNER_TOKEN_SOURCE"], "MCP_RUNNER_TOKEN_SOURCE")

if values["MCP_MARKETPLACE_ENABLED"] == "true":
    for key in (
        "MCP_MARKETPLACE_BASE_URL",
        "MCP_MARKETPLACE_CLIENT_ID",
        "MCP_MARKETPLACE_CLIENT_SECRET_SOURCE",
    ):
        if not values.get(key, "").strip():
            fail(f"{key} is required when MCP Marketplace is enabled")
    try:
        marketplace_url = urlsplit(values["MCP_MARKETPLACE_BASE_URL"])
        marketplace_port = marketplace_url.port
    except ValueError:
        fail("MCP_MARKETPLACE_BASE_URL must be an HTTPS URL")
    if (
        marketplace_url.scheme != "https"
        or not marketplace_url.hostname
        or not valid_hostname(marketplace_url.hostname)
        or marketplace_url.username is not None
        or marketplace_url.password is not None
        or marketplace_url.query
        or marketplace_url.fragment
        or marketplace_port not in {None, 443}
        or any(part == ".." for part in marketplace_url.path.split("/"))
    ):
        fail("MCP_MARKETPLACE_BASE_URL must be an HTTPS URL")
    client_id = values["MCP_MARKETPLACE_CLIENT_ID"]
    if len(client_id) > 2048 or any(character.isspace() for character in client_id):
        fail("MCP_MARKETPLACE_CLIENT_ID is invalid")
    validate_private_token(
        values["MCP_MARKETPLACE_CLIENT_SECRET_SOURCE"],
        "MCP_MARKETPLACE_CLIENT_SECRET_SOURCE",
    )

agent_control_enabled = values["AGENT_RUNNER_CONTROL_ENABLED"] == "true"
agent_root_canary_enabled = values["AGENT_ROOT_RUN_CANARY_ENABLED"] == "true"
agent_broker_canary_enabled = (
    values["AGENT_BROKER_ARTIFACT_CANARY_ENABLED"] == "true"
)
agent_project_canary_enabled = (
    values["AGENT_PROJECT_MUTATION_CANARY_ENABLED"] == "true"
)
agent_child_canary_enabled = values["AGENT_CHILD_CANARY_ENABLED"] == "true"
agent_cron_worker_enabled = values["AGENT_CRON_WORKER_ENABLED"] == "true"
agent_draft_learning_worker_enabled = (
    values["AGENT_DRAFT_LEARNING_WORKER_ENABLED"] == "true"
)
private_networks = (
    ipaddress.ip_network("10.0.0.0/8"),
    ipaddress.ip_network("172.16.0.0/12"),
    ipaddress.ip_network("192.168.0.0/16"),
    ipaddress.ip_network("fc00::/7"),
)
identity_pattern = re.compile(r"[A-Za-z][A-Za-z0-9_.:@/-]{0,127}")
agent_control_keys = (
    "AGENT_RUNNER_DATABASE_URL",
    "AGENT_RUNNER_URL",
    "AGENT_RUNNER_ID",
    "AGENT_RUNNER_SERVER_NAME",
    "AGENT_RUNNER_CLIENT_IDENTITY",
    "AGENT_RUNNER_CLIENT_CERT_SOURCE",
    "AGENT_RUNNER_CLIENT_KEY_SOURCE",
    "AGENT_RUNNER_SERVER_CA_SOURCE",
    "AGENT_RUNNER_RELEASE_MANIFEST_SOURCE",
    "AGENT_PRODUCTION_POLICY_SOURCE",
    "AGENT_PRODUCTION_ACTIVATION_SOURCE",
    "AGENT_RELEASE_GIT_COMMIT",
    "AGENT_RUNNER_POLL_INTERVAL",
    "AGENT_RUNNER_RPC_TIMEOUT",
    "AGENT_RUNNER_RECONCILE_BATCH_SIZE",
)
if agent_control_enabled:
    for key in agent_control_keys:
        if not values.get(key, "").strip():
            fail(f"{key} is required when Agent control is enabled")
        if placeholder.search(values[key]):
            fail(f"{key} still contains a placeholder")
    try:
        agent_runner_url = urlsplit(values["AGENT_RUNNER_URL"])
        agent_runner_port = agent_runner_url.port
        agent_runner_ip = ipaddress.ip_address(agent_runner_url.hostname or "")
    except ValueError:
        fail("AGENT_RUNNER_URL must be the exact private HTTPS Runner RPC URL")
    agent_runner_private = any(
        agent_runner_ip in network
        for network in private_networks
        if agent_runner_ip.version == network.version
    )
    if (
        agent_runner_url.scheme != "https"
        or agent_runner_port is None
        or agent_runner_port < 1
        or not agent_runner_private
        or agent_runner_url.username is not None
        or agent_runner_url.password is not None
        or agent_runner_url.path != "/internal/neo-runner/v1/rpc"
        or agent_runner_url.query
        or agent_runner_url.fragment
    ):
        fail("AGENT_RUNNER_URL must be the exact private HTTPS Runner RPC URL")
    for key in ("AGENT_RUNNER_ID", "AGENT_RUNNER_SERVER_NAME"):
        if identity_pattern.fullmatch(values[key]) is None:
            fail(f"{key} is invalid")
    if values["AGENT_RUNNER_CLIENT_IDENTITY"] != "spiffe://neo-chat/agent-runtime-control":
        fail("AGENT_RUNNER_CLIENT_IDENTITY must be the dedicated control identity")
    if re.fullmatch(r"[0-9a-f]{40}", values["AGENT_RELEASE_GIT_COMMIT"]) is None or values["AGENT_RELEASE_GIT_COMMIT"] == "0" * 40:
        fail("AGENT_RELEASE_GIT_COMMIT must be a non-placeholder lowercase Git commit")
    poll_interval = parse_simple_duration_seconds(
        "AGENT_RUNNER_POLL_INTERVAL", values["AGENT_RUNNER_POLL_INTERVAL"]
    )
    rpc_timeout = parse_simple_duration_seconds(
        "AGENT_RUNNER_RPC_TIMEOUT", values["AGENT_RUNNER_RPC_TIMEOUT"]
    )
    if not 5 <= poll_interval <= 300:
        fail("AGENT_RUNNER_POLL_INTERVAL must be between 5s and 5m")
    if not 1 <= rpc_timeout <= 60:
        fail("AGENT_RUNNER_RPC_TIMEOUT must be between 1s and 1m")
    if re.fullmatch(r"[1-9][0-9]{0,3}", values["AGENT_RUNNER_RECONCILE_BATCH_SIZE"]) is None or not 1 <= int(values["AGENT_RUNNER_RECONCILE_BATCH_SIZE"]) <= 1000:
        fail("AGENT_RUNNER_RECONCILE_BATCH_SIZE must be between 1 and 1000")

    client_certificate = resolve_secure_file(
        values["AGENT_RUNNER_CLIENT_CERT_SOURCE"],
        "AGENT_RUNNER_CLIENT_CERT_SOURCE",
        private=True,
    )
    client_key = resolve_secure_file(
        values["AGENT_RUNNER_CLIENT_KEY_SOURCE"],
        "AGENT_RUNNER_CLIENT_KEY_SOURCE",
        private=True,
    )
    server_ca = resolve_secure_file(
        values["AGENT_RUNNER_SERVER_CA_SOURCE"],
        "AGENT_RUNNER_SERVER_CA_SOURCE",
        private=True,
    )
    release_manifest = resolve_secure_file(
        values["AGENT_RUNNER_RELEASE_MANIFEST_SOURCE"],
        "AGENT_RUNNER_RELEASE_MANIFEST_SOURCE",
        private=True,
    )
    production_policy = resolve_secure_file(
        values["AGENT_PRODUCTION_POLICY_SOURCE"],
        "AGENT_PRODUCTION_POLICY_SOURCE",
        private=False,
    )
    activation_record = resolve_secure_file(
        values["AGENT_PRODUCTION_ACTIVATION_SOURCE"],
        "AGENT_PRODUCTION_ACTIVATION_SOURCE",
        private=True,
    )
    try:
        tls_context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        tls_context.load_cert_chain(client_certificate, client_key)
        ssl.create_default_context(cafile=server_ca)
        decoded_certificate = ssl._ssl._test_decode_cert(str(client_certificate))
    except (OSError, ssl.SSLError, ValueError):
        fail("Agent Runner mTLS certificate/key material is invalid or mismatched")
    common_names = [
        value
        for relative_name in decoded_certificate.get("subject", ())
        for key, value in relative_name
        if key == "commonName"
    ]
    if common_names != [values["AGENT_RUNNER_CLIENT_IDENTITY"]]:
        fail("Agent Runner client certificate identity does not match configuration")

    evaluator = Path(sys.argv[2]) / "scripts/evaluate-agent-production-activation.py"
    try:
        decision = subprocess.run(
            [
                sys.executable,
                str(evaluator),
                "--record", str(activation_record),
                "--policy", str(production_policy),
                "--release-manifest", str(release_manifest),
                "--client-certificate", str(client_certificate),
                "--server-ca", str(server_ca),
                "--endpoint", values["AGENT_RUNNER_URL"],
                "--runner-id", values["AGENT_RUNNER_ID"],
                "--server-name", values["AGENT_RUNNER_SERVER_NAME"],
                "--caller-identity", values["AGENT_RUNNER_CLIENT_IDENTITY"],
                "--release-commit", values["AGENT_RELEASE_GIT_COMMIT"],
            ],
            check=False,
            capture_output=True,
            text=True,
            timeout=10,
        )
        decision_payload = json.loads(decision.stdout)
    except (OSError, subprocess.SubprocessError, json.JSONDecodeError):
        fail("Agent Runtime activation evaluator failed")
    if decision.returncode != 0 or decision_payload.get("verdict") != "ACTIVATION_READY":
        fail("AGENT_PRODUCTION_ACTIVATION_SOURCE is not READY for G21.0")

agent_root_canary_keys = (
    "AGENT_ROOT_CANARY_DATABASE_URL",
    "AGENT_ROOT_CANARY_RUNNER_URL",
    "AGENT_ROOT_CANARY_RUNNER_ID",
    "AGENT_ROOT_CANARY_SERVER_NAME",
    "AGENT_ROOT_CANARY_CLIENT_IDENTITY",
    "AGENT_ROOT_CANARY_CLIENT_CERT_SOURCE",
    "AGENT_ROOT_CANARY_CLIENT_KEY_SOURCE",
    "AGENT_ROOT_CANARY_SERVER_CA_SOURCE",
    "AGENT_ROOT_CANARY_RELEASE_MANIFEST_SOURCE",
    "AGENT_ROOT_CANARY_PRODUCTION_POLICY_SOURCE",
    "AGENT_ROOT_CANARY_ACTIVATION_SOURCE",
    "AGENT_ROOT_CANARY_PLAN_SOURCE",
    "AGENT_ROOT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE",
    "AGENT_ROOT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE",
    "AGENT_ROOT_CANARY_RELEASE_GIT_COMMIT",
    "AGENT_ROOT_CANARY_POLL_INTERVAL",
    "AGENT_ROOT_CANARY_RPC_TIMEOUT",
    "AGENT_ROOT_CANARY_AUTHORITY_TTL",
    "AGENT_ROOT_CANARY_RECONCILE_BATCH_SIZE",
)
if agent_root_canary_enabled:
    if not agent_control_enabled:
        fail("AGENT_ROOT_RUN_CANARY_ENABLED requires G21.0 control enabled")
    for key in agent_root_canary_keys:
        if not values.get(key, "").strip():
            fail(f"{key} is required when the Root canary is enabled")
        if placeholder.search(values[key]):
            fail(f"{key} still contains a placeholder")
    try:
        canary_runner_url = urlsplit(values["AGENT_ROOT_CANARY_RUNNER_URL"])
        canary_runner_port = canary_runner_url.port
        canary_runner_ip = ipaddress.ip_address(canary_runner_url.hostname or "")
    except ValueError:
        fail("AGENT_ROOT_CANARY_RUNNER_URL must be the exact private HTTPS Runner RPC URL")
    canary_runner_private = any(
        canary_runner_ip in network
        for network in private_networks
        if canary_runner_ip.version == network.version
    )
    if (
        canary_runner_url.scheme != "https"
        or canary_runner_port is None
        or canary_runner_port < 1
        or not canary_runner_private
        or canary_runner_url.username is not None
        or canary_runner_url.password is not None
        or canary_runner_url.path != "/internal/neo-runner/v1/rpc"
        or canary_runner_url.query
        or canary_runner_url.fragment
    ):
        fail("AGENT_ROOT_CANARY_RUNNER_URL must be the exact private HTTPS Runner RPC URL")
    for key in ("AGENT_ROOT_CANARY_RUNNER_ID", "AGENT_ROOT_CANARY_SERVER_NAME"):
        if identity_pattern.fullmatch(values[key]) is None:
            fail(f"{key} is invalid")
    if values["AGENT_ROOT_CANARY_CLIENT_IDENTITY"] != "spiffe://neo-chat/agent-runtime-root-canary":
        fail("AGENT_ROOT_CANARY_CLIENT_IDENTITY must be the dedicated Root canary identity")
    if values["AGENT_ROOT_CANARY_CLIENT_IDENTITY"] == values["AGENT_RUNNER_CLIENT_IDENTITY"]:
        fail("Agent control and Root canary identities must be distinct")
    if (
        re.fullmatch(r"[0-9a-f]{40}", values["AGENT_ROOT_CANARY_RELEASE_GIT_COMMIT"]) is None
        or values["AGENT_ROOT_CANARY_RELEASE_GIT_COMMIT"] == "0" * 40
    ):
        fail("AGENT_ROOT_CANARY_RELEASE_GIT_COMMIT must be a non-placeholder lowercase Git commit")
    canary_poll = parse_simple_duration_seconds("AGENT_ROOT_CANARY_POLL_INTERVAL", values["AGENT_ROOT_CANARY_POLL_INTERVAL"])
    canary_rpc = parse_simple_duration_seconds("AGENT_ROOT_CANARY_RPC_TIMEOUT", values["AGENT_ROOT_CANARY_RPC_TIMEOUT"])
    canary_authority_ttl = parse_simple_duration_seconds("AGENT_ROOT_CANARY_AUTHORITY_TTL", values["AGENT_ROOT_CANARY_AUTHORITY_TTL"])
    if not 1 <= canary_poll <= 60:
        fail("AGENT_ROOT_CANARY_POLL_INTERVAL must be between 1s and 1m")
    if not 1 <= canary_rpc <= 60:
        fail("AGENT_ROOT_CANARY_RPC_TIMEOUT must be between 1s and 1m")
    if not 1 <= canary_authority_ttl <= 15:
        fail("AGENT_ROOT_CANARY_AUTHORITY_TTL must be between 1s and 15s")
    if re.fullmatch(r"[1-9][0-9]{0,3}", values["AGENT_ROOT_CANARY_RECONCILE_BATCH_SIZE"]) is None or not 1 <= int(values["AGENT_ROOT_CANARY_RECONCILE_BATCH_SIZE"]) <= 1000:
        fail("AGENT_ROOT_CANARY_RECONCILE_BATCH_SIZE must be between 1 and 1000")

    canary_files = {
        key: resolve_secure_file(values[key], key, private=(key != "AGENT_ROOT_CANARY_PRODUCTION_POLICY_SOURCE"))
        for key in (
            "AGENT_ROOT_CANARY_CLIENT_CERT_SOURCE",
            "AGENT_ROOT_CANARY_CLIENT_KEY_SOURCE",
            "AGENT_ROOT_CANARY_SERVER_CA_SOURCE",
            "AGENT_ROOT_CANARY_RELEASE_MANIFEST_SOURCE",
            "AGENT_ROOT_CANARY_PRODUCTION_POLICY_SOURCE",
            "AGENT_ROOT_CANARY_ACTIVATION_SOURCE",
            "AGENT_ROOT_CANARY_PLAN_SOURCE",
            "AGENT_ROOT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE",
            "AGENT_ROOT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE",
        )
    }
    try:
        tls_context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        tls_context.load_cert_chain(
            canary_files["AGENT_ROOT_CANARY_CLIENT_CERT_SOURCE"],
            canary_files["AGENT_ROOT_CANARY_CLIENT_KEY_SOURCE"],
        )
        ssl.create_default_context(cafile=canary_files["AGENT_ROOT_CANARY_SERVER_CA_SOURCE"])
        decoded_certificate = ssl._ssl._test_decode_cert(str(canary_files["AGENT_ROOT_CANARY_CLIENT_CERT_SOURCE"]))
    except (OSError, ssl.SSLError, ValueError):
        fail("Root canary mTLS certificate/key material is invalid or mismatched")
    common_names = [
        value
        for relative_name in decoded_certificate.get("subject", ())
        for key, value in relative_name
        if key == "commonName"
    ]
    if common_names != [values["AGENT_ROOT_CANARY_CLIENT_IDENTITY"]]:
        fail("Root canary client certificate identity does not match configuration")
    try:
        private_text = canary_files["AGENT_ROOT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE"].read_text().strip()
        public_text = canary_files["AGENT_ROOT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE"].read_text().strip()
        private_raw = base64.urlsafe_b64decode(private_text + "=" * (-len(private_text) % 4))
        public_raw = base64.urlsafe_b64decode(public_text + "=" * (-len(public_text) % 4))
    except (OSError, UnicodeError, ValueError, binascii.Error):
        fail("Root canary authority key material is invalid")
    if len(private_raw) != 64 or len(public_raw) != 32 or private_raw[32:] != public_raw:
        fail("Root canary authority private/public keys do not match")
    try:
        canary_plan = json.loads(
            canary_files["AGENT_ROOT_CANARY_PLAN_SOURCE"].read_text(),
            object_pairs_hook=unique_object,
        )
    except (OSError, UnicodeError, json.JSONDecodeError, DuplicateKey):
        fail("AGENT_ROOT_CANARY_PLAN_SOURCE is invalid")
    if (
        not isinstance(canary_plan, dict)
        or canary_plan.get("schemaVersion") != "neo.agent-root-run-canary-plan/v1"
        or canary_plan.get("synthetic") is not True
        or canary_plan.get("stepKind") != "root_canary"
        or canary_plan.get("toolRegistry", {}).get("tools") != []
        or canary_plan.get("sandbox", {}).get("networkMode") != "none"
        or canary_plan.get("snapshot", {}).get("noEgress") is not True
        or canary_plan.get("snapshot", {}).get("noSecrets") is not True
    ):
        fail("AGENT_ROOT_CANARY_PLAN_SOURCE is not a no-Egress/no-Secret synthetic plan")
    evaluator = Path(sys.argv[2]) / "scripts/evaluate-agent-production-activation.py"
    try:
        decision = subprocess.run(
            [
                sys.executable,
                str(evaluator),
                "--record", str(canary_files["AGENT_ROOT_CANARY_ACTIVATION_SOURCE"]),
                "--policy", str(canary_files["AGENT_ROOT_CANARY_PRODUCTION_POLICY_SOURCE"]),
                "--release-manifest", str(canary_files["AGENT_ROOT_CANARY_RELEASE_MANIFEST_SOURCE"]),
                "--client-certificate", str(canary_files["AGENT_ROOT_CANARY_CLIENT_CERT_SOURCE"]),
                "--server-ca", str(canary_files["AGENT_ROOT_CANARY_SERVER_CA_SOURCE"]),
                "--canary-plan", str(canary_files["AGENT_ROOT_CANARY_PLAN_SOURCE"]),
                "--authority-public-key", str(canary_files["AGENT_ROOT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE"]),
                "--endpoint", values["AGENT_ROOT_CANARY_RUNNER_URL"],
                "--runner-id", values["AGENT_ROOT_CANARY_RUNNER_ID"],
                "--server-name", values["AGENT_ROOT_CANARY_SERVER_NAME"],
                "--caller-identity", values["AGENT_ROOT_CANARY_CLIENT_IDENTITY"],
                "--release-commit", values["AGENT_ROOT_CANARY_RELEASE_GIT_COMMIT"],
            ],
            check=False,
            capture_output=True,
            text=True,
            timeout=10,
        )
        decision_payload = json.loads(decision.stdout)
    except (OSError, subprocess.SubprocessError, json.JSONDecodeError):
        fail("Root canary activation evaluator failed")
    if (
        decision.returncode != 0
        or decision_payload.get("verdict") != "ACTIVATION_READY"
        or decision_payload.get("reasonCode") != "ROOT_RUN_CANARY_GATES_PASSED"
    ):
        fail("AGENT_ROOT_CANARY_ACTIVATION_SOURCE is not READY for G21.1")

agent_broker_canary_keys = (
    "AGENT_BROKER_CANARY_DATABASE_URL",
    "AGENT_BROKER_CANARY_RUNNER_URL",
    "AGENT_BROKER_CANARY_RUNNER_ID",
    "AGENT_BROKER_CANARY_RUNNER_SERVER_NAME",
    "AGENT_BROKER_CANARY_CLIENT_IDENTITY",
    "AGENT_BROKER_CANARY_CLIENT_CERT_SOURCE",
    "AGENT_BROKER_CANARY_CLIENT_KEY_SOURCE",
    "AGENT_BROKER_CANARY_SERVER_CA_SOURCE",
    "AGENT_BROKER_CANARY_RELEASE_MANIFEST_SOURCE",
    "AGENT_BROKER_CANARY_PRODUCTION_POLICY_SOURCE",
    "AGENT_BROKER_CANARY_ACTIVATION_SOURCE",
    "AGENT_BROKER_CANARY_PLAN_SOURCE",
    "AGENT_BROKER_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE",
    "AGENT_BROKER_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE",
    "AGENT_BROKER_CANARY_RELEASE_GIT_COMMIT",
    "AGENT_BROKER_CANARY_RELAY_SUBNET",
    "AGENT_BROKER_CANARY_RELAY_IP",
    "AGENT_BROKER_CANARY_RELAY_LISTEN_ADDR",
    "AGENT_BROKER_CANARY_RELAY_ENDPOINT",
    "AGENT_BROKER_CANARY_RELAY_TLS_CERT_SOURCE",
    "AGENT_BROKER_CANARY_RELAY_TLS_KEY_SOURCE",
    "AGENT_BROKER_CANARY_RELAY_TLS_CLIENT_CA_SOURCE",
    "AGENT_BROKER_CANARY_RUNNER_RELAY_IDENTITY",
    "AGENT_BROKER_CANARY_QUARANTINE_SOURCE",
    "AGENT_BROKER_CANARY_PROJECT_ROOT_SOURCE",
    "AGENT_BROKER_CANARY_WORKSPACE_ROOT_SOURCE",
    "AGENT_BROKER_CANARY_MCP_RUNNER_URL",
    "AGENT_BROKER_CANARY_POLL_INTERVAL",
    "AGENT_BROKER_CANARY_RPC_TIMEOUT",
    "AGENT_BROKER_CANARY_AUTHORITY_TTL",
    "AGENT_BROKER_CANARY_RECONCILE_BATCH_SIZE",
    "MCP_RUNNER_TOKEN_SOURCE",
    "STORAGE_BACKEND",
    "S3_ENDPOINT",
    "S3_BUCKET",
    "S3_REGION",
    "S3_USE_SSL",
    "S3_FORCE_PATH_STYLE",
    "S3_BUCKET_AUTO_CREATE",
)
if agent_broker_canary_enabled:
    if not agent_control_enabled or not agent_root_canary_enabled:
        fail("AGENT_BROKER_ARTIFACT_CANARY_ENABLED requires G21.0 and G21.1 enabled")
    for key in agent_broker_canary_keys:
        if not values.get(key, "").strip():
            fail(f"{key} is required when the Broker canary is enabled")
        if placeholder.search(values[key]):
            fail(f"{key} still contains a placeholder")

    try:
        broker_runner_url = urlsplit(values["AGENT_BROKER_CANARY_RUNNER_URL"])
        broker_runner_ip = ipaddress.ip_address(broker_runner_url.hostname or "")
        broker_runner_port = broker_runner_url.port
    except ValueError:
        fail("AGENT_BROKER_CANARY_RUNNER_URL must be the exact private HTTPS Runner RPC URL")
    if (
        broker_runner_url.scheme != "https"
        or broker_runner_port is None
        or not any(
            broker_runner_ip in network
            for network in private_networks
            if broker_runner_ip.version == network.version
        )
        or broker_runner_url.username is not None
        or broker_runner_url.password is not None
        or broker_runner_url.path != "/internal/neo-runner/v1/rpc"
        or broker_runner_url.query
        or broker_runner_url.fragment
    ):
        fail("AGENT_BROKER_CANARY_RUNNER_URL must be the exact private HTTPS Runner RPC URL")
    for key in ("AGENT_BROKER_CANARY_RUNNER_ID", "AGENT_BROKER_CANARY_RUNNER_SERVER_NAME"):
        if identity_pattern.fullmatch(values[key]) is None:
            fail(f"{key} is invalid")
    required_identities = {
        values["AGENT_RUNNER_CLIENT_IDENTITY"],
        values["AGENT_ROOT_CANARY_CLIENT_IDENTITY"],
        values["AGENT_BROKER_CANARY_CLIENT_IDENTITY"],
        values["AGENT_BROKER_CANARY_RUNNER_RELAY_IDENTITY"],
    }
    if (
        values["AGENT_BROKER_CANARY_CLIENT_IDENTITY"]
        != "spiffe://neo-chat/agent-runtime-broker-canary"
        or values["AGENT_BROKER_CANARY_RUNNER_RELAY_IDENTITY"]
        != "spiffe://neo-chat/neo-runner-broker-relay"
        or len(required_identities) != 4
    ):
        fail("Agent control, Root canary, Broker canary and relay identities must be exact and distinct")

    try:
        relay_subnet = ipaddress.ip_network(values["AGENT_BROKER_CANARY_RELAY_SUBNET"], strict=True)
        relay_ip = ipaddress.ip_address(values["AGENT_BROKER_CANARY_RELAY_IP"])
        relay_listen = urlsplit("//" + values["AGENT_BROKER_CANARY_RELAY_LISTEN_ADDR"])
        relay_listen_ip = ipaddress.ip_address(relay_listen.hostname or "")
        relay_endpoint = urlsplit(values["AGENT_BROKER_CANARY_RELAY_ENDPOINT"])
    except ValueError:
        fail("Agent Broker relay must use one exact private literal endpoint")
    if (
        not relay_subnet.is_private
        or relay_ip not in relay_subnet
        or relay_ip in {relay_subnet.network_address, relay_subnet.broadcast_address}
        or relay_listen_ip != relay_ip
        or relay_listen.port is None
        or relay_endpoint.scheme != "https"
        or relay_endpoint.hostname != str(relay_ip)
        or relay_endpoint.port != relay_listen.port
        or relay_endpoint.path != "/internal/agent-broker/v1/relay"
        or relay_endpoint.username is not None
        or relay_endpoint.password is not None
        or relay_endpoint.query
        or relay_endpoint.fragment
    ):
        fail("Agent Broker relay must use one exact private literal endpoint")

    if (
        re.fullmatch(r"[0-9a-f]{40}", values["AGENT_BROKER_CANARY_RELEASE_GIT_COMMIT"])
        is None
        or values["AGENT_BROKER_CANARY_RELEASE_GIT_COMMIT"] == "0" * 40
    ):
        fail("AGENT_BROKER_CANARY_RELEASE_GIT_COMMIT must be a non-placeholder lowercase Git commit")
    broker_poll = parse_simple_duration_seconds("AGENT_BROKER_CANARY_POLL_INTERVAL", values["AGENT_BROKER_CANARY_POLL_INTERVAL"])
    broker_rpc = parse_simple_duration_seconds("AGENT_BROKER_CANARY_RPC_TIMEOUT", values["AGENT_BROKER_CANARY_RPC_TIMEOUT"])
    broker_authority_ttl = parse_simple_duration_seconds("AGENT_BROKER_CANARY_AUTHORITY_TTL", values["AGENT_BROKER_CANARY_AUTHORITY_TTL"])
    if not 1 <= broker_poll <= 60:
        fail("AGENT_BROKER_CANARY_POLL_INTERVAL must be between 1s and 1m")
    if not 1 <= broker_rpc <= 10:
        fail("AGENT_BROKER_CANARY_RPC_TIMEOUT must be between 1s and 10s")
    if not 10 <= broker_authority_ttl <= 15:
        fail("AGENT_BROKER_CANARY_AUTHORITY_TTL must be between 10s and 15s")
    if (
        re.fullmatch(r"[1-9][0-9]{0,3}", values["AGENT_BROKER_CANARY_RECONCILE_BATCH_SIZE"])
        is None
        or not 1 <= int(values["AGENT_BROKER_CANARY_RECONCILE_BATCH_SIZE"]) <= 1000
    ):
        fail("AGENT_BROKER_CANARY_RECONCILE_BATCH_SIZE must be between 1 and 1000")
    if (
        values["STORAGE_BACKEND"] not in {"minio", "s3"}
        or values["S3_BUCKET_AUTO_CREATE"] != "false"
        or values["S3_USE_SSL"] not in {"true", "false"}
        or values["S3_FORCE_PATH_STYLE"] not in {"true", "false"}
    ):
        fail("Broker canary requires the existing S3-compatible bucket without auto-create")
    try:
        broker_mcp_url = urlsplit(values["AGENT_BROKER_CANARY_MCP_RUNNER_URL"])
    except ValueError:
        fail("AGENT_BROKER_CANARY_MCP_RUNNER_URL is invalid")
    if (
        broker_mcp_url.scheme != "http"
        or broker_mcp_url.hostname != "mcp-runner"
        or broker_mcp_url.port != 8090
        or broker_mcp_url.path not in {"", "/"}
        or broker_mcp_url.username is not None
        or broker_mcp_url.password is not None
        or broker_mcp_url.query
        or broker_mcp_url.fragment
    ):
        fail("AGENT_BROKER_CANARY_MCP_RUNNER_URL must be the private MCP Runner URL")
    validate_private_token(values["MCP_RUNNER_TOKEN_SOURCE"], "MCP_RUNNER_TOKEN_SOURCE")

    broker_files = {
        key: resolve_secure_file(
            values[key], key,
            private=(key != "AGENT_BROKER_CANARY_PRODUCTION_POLICY_SOURCE"),
        )
        for key in (
            "AGENT_BROKER_CANARY_CLIENT_CERT_SOURCE",
            "AGENT_BROKER_CANARY_CLIENT_KEY_SOURCE",
            "AGENT_BROKER_CANARY_SERVER_CA_SOURCE",
            "AGENT_BROKER_CANARY_RELEASE_MANIFEST_SOURCE",
            "AGENT_BROKER_CANARY_PRODUCTION_POLICY_SOURCE",
            "AGENT_BROKER_CANARY_ACTIVATION_SOURCE",
            "AGENT_BROKER_CANARY_PLAN_SOURCE",
            "AGENT_BROKER_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE",
            "AGENT_BROKER_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE",
            "AGENT_BROKER_CANARY_RELAY_TLS_CERT_SOURCE",
            "AGENT_BROKER_CANARY_RELAY_TLS_KEY_SOURCE",
            "AGENT_BROKER_CANARY_RELAY_TLS_CLIENT_CA_SOURCE",
        )
    }
    for key in (
        "AGENT_BROKER_CANARY_QUARANTINE_SOURCE",
        "AGENT_BROKER_CANARY_PROJECT_ROOT_SOURCE",
        "AGENT_BROKER_CANARY_WORKSPACE_ROOT_SOURCE",
    ):
        resolve_secure_directory(values[key], key)
    try:
        tls_context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        tls_context.load_cert_chain(
            broker_files["AGENT_BROKER_CANARY_CLIENT_CERT_SOURCE"],
            broker_files["AGENT_BROKER_CANARY_CLIENT_KEY_SOURCE"],
        )
        ssl.create_default_context(cafile=broker_files["AGENT_BROKER_CANARY_SERVER_CA_SOURCE"])
        decoded_certificate = ssl._ssl._test_decode_cert(str(broker_files["AGENT_BROKER_CANARY_CLIENT_CERT_SOURCE"]))
        relay_context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        relay_context.load_cert_chain(
            broker_files["AGENT_BROKER_CANARY_RELAY_TLS_CERT_SOURCE"],
            broker_files["AGENT_BROKER_CANARY_RELAY_TLS_KEY_SOURCE"],
        )
        ssl.create_default_context(cafile=broker_files["AGENT_BROKER_CANARY_RELAY_TLS_CLIENT_CA_SOURCE"])
    except (OSError, ssl.SSLError, ValueError):
        fail("Broker canary mTLS certificate/key material is invalid or mismatched")
    common_names = [
        value
        for relative_name in decoded_certificate.get("subject", ())
        for key, value in relative_name
        if key == "commonName"
    ]
    if common_names != [values["AGENT_BROKER_CANARY_CLIENT_IDENTITY"]]:
        fail("Broker canary client certificate identity does not match configuration")
    try:
        private_text = broker_files["AGENT_BROKER_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE"].read_text().strip()
        public_text = broker_files["AGENT_BROKER_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE"].read_text().strip()
        private_raw = base64.urlsafe_b64decode(private_text + "=" * (-len(private_text) % 4))
        public_raw = base64.urlsafe_b64decode(public_text + "=" * (-len(public_text) % 4))
    except (OSError, UnicodeError, ValueError, binascii.Error):
        fail("Broker canary authority key material is invalid")
    if len(private_raw) != 64 or len(public_raw) != 32 or private_raw[32:] != public_raw:
        fail("Broker canary authority private/public keys do not match")

    try:
        broker_plan = json.loads(
            broker_files["AGENT_BROKER_CANARY_PLAN_SOURCE"].read_text(),
            object_pairs_hook=unique_object,
        )
    except (OSError, UnicodeError, json.JSONDecodeError, DuplicateKey):
        fail("AGENT_BROKER_CANARY_PLAN_SOURCE is invalid")
    if not isinstance(broker_plan, dict):
        fail("AGENT_BROKER_CANARY_PLAN_SOURCE is invalid")
    actions = broker_plan.get("actions")
    expected_actions = {"project_read", "workspace_read", "mcp_read", "artifact_publish", "possible_send"}
    if (
        broker_plan.get("schemaVersion") != "neo.agent-broker-artifact-canary-plan/v1"
        or broker_plan.get("synthetic") is not True
        or broker_plan.get("sandbox", {}).get("networkMode") != "none"
        or broker_plan.get("sandbox", {}).get("rootfsReadOnly") is not True
        or broker_plan.get("sandbox", {}).get("noNewPrivileges") is not True
        or broker_plan.get("sandbox", {}).get("capabilities") != []
        or not isinstance(actions, list)
        or len(actions) != 5
        or {item.get("id") for item in actions if isinstance(item, dict)} != expected_actions
    ):
        fail("AGENT_BROKER_CANARY_PLAN_SOURCE is not the strict synthetic five-action plan")
    by_id = {item["id"]: item for item in actions}
    mcp_plan = by_id["mcp_read"].get("mcp", {})
    command_argv = mcp_plan.get("commandArgv")
    if (
        by_id["project_read"].get("file", {}).get("root") != "/run/agent-runtime-broker-canary/project"
        or by_id["workspace_read"].get("file", {}).get("root") != "/run/agent-runtime-broker-canary/workspace"
        or mcp_plan.get("toolPolicy") != {mcp_plan.get("toolName"): "read"}
        or not isinstance(command_argv, list)
        or not command_argv
        or not isinstance(command_argv[0], str)
        or not command_argv[0].startswith("/opt/mcp/")
        or by_id["artifact_publish"].get("classification") != "mutable"
        or by_id["artifact_publish"].get("idempotent") is not False
        or any(
            by_id[action_id].get("classification") != "read"
            or by_id[action_id].get("idempotent") is not True
            for action_id in ("project_read", "workspace_read", "mcp_read", "possible_send")
        )
    ):
        fail("AGENT_BROKER_CANARY_PLAN_SOURCE widens reviewed read or Artifact authority")

    evaluator = Path(sys.argv[2]) / "scripts/evaluate-agent-production-activation.py"
    try:
        decision = subprocess.run(
            [
                sys.executable, str(evaluator),
                "--record", str(broker_files["AGENT_BROKER_CANARY_ACTIVATION_SOURCE"]),
                "--policy", str(broker_files["AGENT_BROKER_CANARY_PRODUCTION_POLICY_SOURCE"]),
                "--release-manifest", str(broker_files["AGENT_BROKER_CANARY_RELEASE_MANIFEST_SOURCE"]),
                "--client-certificate", str(broker_files["AGENT_BROKER_CANARY_CLIENT_CERT_SOURCE"]),
                "--server-ca", str(broker_files["AGENT_BROKER_CANARY_SERVER_CA_SOURCE"]),
                "--canary-plan", str(broker_files["AGENT_BROKER_CANARY_PLAN_SOURCE"]),
                "--authority-public-key", str(broker_files["AGENT_BROKER_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE"]),
                "--relay-endpoint", values["AGENT_BROKER_CANARY_RELAY_ENDPOINT"],
                "--relay-server-certificate", str(broker_files["AGENT_BROKER_CANARY_RELAY_TLS_CERT_SOURCE"]),
                "--relay-client-ca", str(broker_files["AGENT_BROKER_CANARY_RELAY_TLS_CLIENT_CA_SOURCE"]),
                "--runner-relay-identity", values["AGENT_BROKER_CANARY_RUNNER_RELAY_IDENTITY"],
                "--endpoint", values["AGENT_BROKER_CANARY_RUNNER_URL"],
                "--runner-id", values["AGENT_BROKER_CANARY_RUNNER_ID"],
                "--server-name", values["AGENT_BROKER_CANARY_RUNNER_SERVER_NAME"],
                "--caller-identity", values["AGENT_BROKER_CANARY_CLIENT_IDENTITY"],
                "--release-commit", values["AGENT_BROKER_CANARY_RELEASE_GIT_COMMIT"],
            ],
            check=False, capture_output=True, text=True, timeout=10,
        )
        decision_payload = json.loads(decision.stdout)
    except (OSError, subprocess.SubprocessError, json.JSONDecodeError):
        fail("Broker canary activation evaluator failed")
    if (
        decision.returncode != 0
        or decision_payload.get("verdict") != "ACTIVATION_READY"
        or decision_payload.get("reasonCode") != "BROKER_ARTIFACT_CANARY_GATES_PASSED"
    ):
        fail("AGENT_BROKER_CANARY_ACTIVATION_SOURCE is not READY for G21.2")

agent_project_canary_keys = (
    "AGENT_PROJECT_CANARY_DATABASE_URL",
    "AGENT_PROJECT_CANARY_RUNNER_URL",
    "AGENT_PROJECT_CANARY_RUNNER_ID",
    "AGENT_PROJECT_CANARY_RUNNER_SERVER_NAME",
    "AGENT_PROJECT_CANARY_CLIENT_IDENTITY",
    "AGENT_PROJECT_CANARY_CLIENT_CERT_SOURCE",
    "AGENT_PROJECT_CANARY_CLIENT_KEY_SOURCE",
    "AGENT_PROJECT_CANARY_SERVER_CA_SOURCE",
    "AGENT_PROJECT_CANARY_RELEASE_MANIFEST_SOURCE",
    "AGENT_PROJECT_CANARY_PRODUCTION_POLICY_SOURCE",
    "AGENT_PROJECT_CANARY_ACTIVATION_SOURCE",
    "AGENT_PROJECT_CANARY_PLAN_SOURCE",
    "AGENT_PROJECT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE",
    "AGENT_PROJECT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE",
    "AGENT_PROJECT_CANARY_APPROVAL_DOCUMENT_SOURCE",
    "AGENT_PROJECT_CANARY_APPROVAL_PUBLIC_KEY_SOURCE",
    "AGENT_PROJECT_CANARY_RELEASE_GIT_COMMIT",
    "AGENT_PROJECT_CANARY_RELAY_SUBNET",
    "AGENT_PROJECT_CANARY_RELAY_IP",
    "AGENT_PROJECT_CANARY_RELAY_LISTEN_ADDR",
    "AGENT_PROJECT_CANARY_RELAY_ENDPOINT",
    "AGENT_PROJECT_CANARY_RELAY_TLS_CERT_SOURCE",
    "AGENT_PROJECT_CANARY_RELAY_TLS_KEY_SOURCE",
    "AGENT_PROJECT_CANARY_RELAY_TLS_CLIENT_CA_SOURCE",
    "AGENT_PROJECT_CANARY_RUNNER_RELAY_IDENTITY",
    "AGENT_PROJECT_CANARY_POLL_INTERVAL",
    "AGENT_PROJECT_CANARY_RPC_TIMEOUT",
    "AGENT_PROJECT_CANARY_AUTHORITY_TTL",
    "AGENT_PROJECT_CANARY_RECONCILE_BATCH_SIZE",
)
if agent_project_canary_enabled:
    if not agent_control_enabled or not agent_root_canary_enabled or not agent_broker_canary_enabled:
        fail("AGENT_PROJECT_MUTATION_CANARY_ENABLED requires G21.0, G21.1 and G21.2 enabled")
    for key in agent_project_canary_keys:
        if not values.get(key, "").strip():
            fail(f"{key} is required when the Project mutation canary is enabled")
        if placeholder.search(values[key]):
            fail(f"{key} still contains a placeholder")

    try:
        project_runner_url = urlsplit(values["AGENT_PROJECT_CANARY_RUNNER_URL"])
        project_runner_ip = ipaddress.ip_address(project_runner_url.hostname or "")
        project_runner_port = project_runner_url.port
    except ValueError:
        fail("AGENT_PROJECT_CANARY_RUNNER_URL must be the exact private HTTPS Runner RPC URL")
    if (
        project_runner_url.scheme != "https"
        or project_runner_port is None
        or not any(
            project_runner_ip in network
            for network in private_networks
            if project_runner_ip.version == network.version
        )
        or project_runner_url.username is not None
        or project_runner_url.password is not None
        or project_runner_url.path != "/internal/neo-runner/v1/rpc"
        or project_runner_url.query
        or project_runner_url.fragment
    ):
        fail("AGENT_PROJECT_CANARY_RUNNER_URL must be the exact private HTTPS Runner RPC URL")
    for key in ("AGENT_PROJECT_CANARY_RUNNER_ID", "AGENT_PROJECT_CANARY_RUNNER_SERVER_NAME"):
        if identity_pattern.fullmatch(values[key]) is None:
            fail(f"{key} is invalid")
    project_identities = {
        values["AGENT_RUNNER_CLIENT_IDENTITY"],
        values["AGENT_ROOT_CANARY_CLIENT_IDENTITY"],
        values["AGENT_BROKER_CANARY_CLIENT_IDENTITY"],
        values["AGENT_BROKER_CANARY_RUNNER_RELAY_IDENTITY"],
        values["AGENT_PROJECT_CANARY_CLIENT_IDENTITY"],
        values["AGENT_PROJECT_CANARY_RUNNER_RELAY_IDENTITY"],
    }
    if (
        values["AGENT_PROJECT_CANARY_CLIENT_IDENTITY"]
        != "spiffe://neo-chat/agent-runtime-project-canary"
        or values["AGENT_PROJECT_CANARY_RUNNER_RELAY_IDENTITY"]
        != "spiffe://neo-chat/neo-runner-project-relay"
        or len(project_identities) != 6
    ):
        fail("Agent control, Root, Broker, Project and relay identities must be exact and distinct")

    try:
        project_relay_subnet = ipaddress.ip_network(values["AGENT_PROJECT_CANARY_RELAY_SUBNET"], strict=True)
        project_relay_ip = ipaddress.ip_address(values["AGENT_PROJECT_CANARY_RELAY_IP"])
        project_relay_listen = urlsplit("//" + values["AGENT_PROJECT_CANARY_RELAY_LISTEN_ADDR"])
        project_relay_listen_ip = ipaddress.ip_address(project_relay_listen.hostname or "")
        project_relay_endpoint = urlsplit(values["AGENT_PROJECT_CANARY_RELAY_ENDPOINT"])
    except ValueError:
        fail("Agent Project relay must use one exact private literal endpoint")
    if (
        not project_relay_subnet.is_private
        or project_relay_subnet.overlaps(relay_subnet)
        or project_relay_ip not in project_relay_subnet
        or project_relay_ip in {project_relay_subnet.network_address, project_relay_subnet.broadcast_address}
        or project_relay_listen_ip != project_relay_ip
        or project_relay_listen.port is None
        or project_relay_endpoint.scheme != "https"
        or project_relay_endpoint.hostname != str(project_relay_ip)
        or project_relay_endpoint.port != project_relay_listen.port
        or project_relay_endpoint.path != "/internal/agent-broker/v1/relay"
        or project_relay_endpoint.username is not None
        or project_relay_endpoint.password is not None
        or project_relay_endpoint.query
        or project_relay_endpoint.fragment
    ):
        fail("Agent Project relay must use one exact private literal endpoint")

    if (
        re.fullmatch(r"[0-9a-f]{40}", values["AGENT_PROJECT_CANARY_RELEASE_GIT_COMMIT"])
        is None
        or values["AGENT_PROJECT_CANARY_RELEASE_GIT_COMMIT"] == "0" * 40
    ):
        fail("AGENT_PROJECT_CANARY_RELEASE_GIT_COMMIT must be a non-placeholder lowercase Git commit")
    project_poll = parse_simple_duration_seconds("AGENT_PROJECT_CANARY_POLL_INTERVAL", values["AGENT_PROJECT_CANARY_POLL_INTERVAL"])
    project_rpc = parse_simple_duration_seconds("AGENT_PROJECT_CANARY_RPC_TIMEOUT", values["AGENT_PROJECT_CANARY_RPC_TIMEOUT"])
    project_authority_ttl = parse_simple_duration_seconds("AGENT_PROJECT_CANARY_AUTHORITY_TTL", values["AGENT_PROJECT_CANARY_AUTHORITY_TTL"])
    if not 1 <= project_poll <= 60:
        fail("AGENT_PROJECT_CANARY_POLL_INTERVAL must be between 1s and 1m")
    if not 1 <= project_rpc <= 10:
        fail("AGENT_PROJECT_CANARY_RPC_TIMEOUT must be between 1s and 10s")
    if not 10 <= project_authority_ttl <= 15:
        fail("AGENT_PROJECT_CANARY_AUTHORITY_TTL must be between 10s and 15s")
    if (
        re.fullmatch(r"[1-9][0-9]{0,3}", values["AGENT_PROJECT_CANARY_RECONCILE_BATCH_SIZE"])
        is None
        or not 1 <= int(values["AGENT_PROJECT_CANARY_RECONCILE_BATCH_SIZE"]) <= 1000
    ):
        fail("AGENT_PROJECT_CANARY_RECONCILE_BATCH_SIZE must be between 1 and 1000")

    project_files = {
        key: resolve_secure_file(
            values[key], key,
            private=(key != "AGENT_PROJECT_CANARY_PRODUCTION_POLICY_SOURCE"),
        )
        for key in (
            "AGENT_PROJECT_CANARY_CLIENT_CERT_SOURCE",
            "AGENT_PROJECT_CANARY_CLIENT_KEY_SOURCE",
            "AGENT_PROJECT_CANARY_SERVER_CA_SOURCE",
            "AGENT_PROJECT_CANARY_RELEASE_MANIFEST_SOURCE",
            "AGENT_PROJECT_CANARY_PRODUCTION_POLICY_SOURCE",
            "AGENT_PROJECT_CANARY_ACTIVATION_SOURCE",
            "AGENT_PROJECT_CANARY_PLAN_SOURCE",
            "AGENT_PROJECT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE",
            "AGENT_PROJECT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE",
            "AGENT_PROJECT_CANARY_APPROVAL_DOCUMENT_SOURCE",
            "AGENT_PROJECT_CANARY_APPROVAL_PUBLIC_KEY_SOURCE",
            "AGENT_PROJECT_CANARY_RELAY_TLS_CERT_SOURCE",
            "AGENT_PROJECT_CANARY_RELAY_TLS_KEY_SOURCE",
            "AGENT_PROJECT_CANARY_RELAY_TLS_CLIENT_CA_SOURCE",
        )
    }
    if len(set(project_files.values())) != len(project_files):
        fail("Project canary approval, authority and TLS files must be distinct")
    project_sensitive_files = {
        project_files[key]
        for key in project_files
        if key not in {
            "AGENT_PROJECT_CANARY_RELEASE_MANIFEST_SOURCE",
            "AGENT_PROJECT_CANARY_PRODUCTION_POLICY_SOURCE",
        }
    }
    broker_sensitive_files = {
        broker_files[key]
        for key in broker_files
        if key not in {
            "AGENT_BROKER_CANARY_RELEASE_MANIFEST_SOURCE",
            "AGENT_BROKER_CANARY_PRODUCTION_POLICY_SOURCE",
        }
    }
    if project_sensitive_files & broker_sensitive_files:
        fail("Project canary files must not reuse Broker canary authority or TLS material")
    try:
        tls_context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        tls_context.load_cert_chain(
            project_files["AGENT_PROJECT_CANARY_CLIENT_CERT_SOURCE"],
            project_files["AGENT_PROJECT_CANARY_CLIENT_KEY_SOURCE"],
        )
        ssl.create_default_context(cafile=project_files["AGENT_PROJECT_CANARY_SERVER_CA_SOURCE"])
        decoded_certificate = ssl._ssl._test_decode_cert(
            str(project_files["AGENT_PROJECT_CANARY_CLIENT_CERT_SOURCE"])
        )
        relay_context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        relay_context.load_cert_chain(
            project_files["AGENT_PROJECT_CANARY_RELAY_TLS_CERT_SOURCE"],
            project_files["AGENT_PROJECT_CANARY_RELAY_TLS_KEY_SOURCE"],
        )
        ssl.create_default_context(cafile=project_files["AGENT_PROJECT_CANARY_RELAY_TLS_CLIENT_CA_SOURCE"])
    except (OSError, ssl.SSLError, ValueError):
        fail("Project canary mTLS certificate/key material is invalid or mismatched")
    common_names = [
        value
        for relative_name in decoded_certificate.get("subject", ())
        for key, value in relative_name
        if key == "commonName"
    ]
    if common_names != [values["AGENT_PROJECT_CANARY_CLIENT_IDENTITY"]]:
        fail("Project canary client certificate identity does not match configuration")
    try:
        authority_private_text = project_files["AGENT_PROJECT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE"].read_text().strip()
        authority_public_text = project_files["AGENT_PROJECT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE"].read_text().strip()
        approval_public_text = project_files["AGENT_PROJECT_CANARY_APPROVAL_PUBLIC_KEY_SOURCE"].read_text().strip()
        authority_private_raw = base64.urlsafe_b64decode(authority_private_text + "=" * (-len(authority_private_text) % 4))
        authority_public_raw = base64.urlsafe_b64decode(authority_public_text + "=" * (-len(authority_public_text) % 4))
        approval_public_raw = base64.urlsafe_b64decode(approval_public_text + "=" * (-len(approval_public_text) % 4))
    except (OSError, UnicodeError, ValueError, binascii.Error):
        fail("Project canary authority or approval key material is invalid")
    if (
        len(authority_private_raw) != 64
        or len(authority_public_raw) != 32
        or authority_private_raw[32:] != authority_public_raw
        or len(approval_public_raw) != 32
        or authority_public_raw == approval_public_raw
    ):
        fail("Project canary approval authority must be separate from the Runner authority")

    try:
        project_plan_raw = project_files["AGENT_PROJECT_CANARY_PLAN_SOURCE"].read_bytes()
        project_plan = json.loads(project_plan_raw, object_pairs_hook=unique_object)
    except (OSError, UnicodeError, json.JSONDecodeError, DuplicateKey):
        fail("AGENT_PROJECT_CANARY_PLAN_SOURCE is invalid")
    action = project_plan.get("action", {}) if isinstance(project_plan, dict) else {}
    project_policy = action.get("project", {}) if isinstance(action, dict) else {}
    arguments = action.get("arguments", {}) if isinstance(action, dict) else {}
    content = project_policy.get("content")
    if not isinstance(content, str):
        fail("AGENT_PROJECT_CANARY_PLAN_SOURCE is not the strict one-action synthetic Project plan")
    content_raw = content.encode("utf-8")
    try:
        plan_issued = datetime.fromisoformat(str(project_plan.get("issuedAt", "")).replace("Z", "+00:00"))
        plan_expires = datetime.fromisoformat(str(project_plan.get("expiresAt", "")).replace("Z", "+00:00"))
    except ValueError:
        fail("AGENT_PROJECT_CANARY_PLAN_SOURCE has an invalid activation window")
    current_time = datetime.now(timezone.utc)
    content_fingerprint = "sha256:" + hashlib.sha256(
        b"neo-project-file-v1\0" + content_raw
    ).hexdigest()
    mutation_material = "\n".join((
        str(action.get("baseRevision", "")),
        str(action.get("resource", "")),
        str(project_policy.get("path", "")),
        content_fingerprint,
    )).encode()
    mutation_fingerprint = "sha256:" + hashlib.sha256(
        b"neo-project-canary-mutation-v1\0" + mutation_material
    ).hexdigest()
    if (
        project_plan.get("schemaVersion") != "neo.agent-project-mutation-canary-plan/v1"
        or project_plan.get("synthetic") is not True
        or plan_issued.tzinfo is None
        or plan_expires.tzinfo is None
        or plan_issued > current_time
        or current_time >= plan_expires
        or plan_expires <= plan_issued
        or project_plan.get("sandbox", {}).get("networkMode") != "none"
        or project_plan.get("sandbox", {}).get("rootfsReadOnly") is not True
        or project_plan.get("sandbox", {}).get("noNewPrivileges") is not True
        or project_plan.get("sandbox", {}).get("capabilities") != []
        or action.get("id") != "project_mutation"
        or action.get("toolIdentity") != "project.patch"
        or action.get("capability") != "project.write"
        or action.get("action") != "apply_patch"
        or action.get("classification") != "mutable"
        or action.get("idempotent") is not False
        or action.get("approval") != "per_commit"
        or action.get("resource") != project_policy.get("resource")
        or action.get("baseRevision") != project_policy.get("baseRevision")
        or not isinstance(project_policy.get("path"), str)
        or "/" in project_policy.get("path", "")
        or project_policy.get("path") in {"", ".", ".."}
        or len(content_raw) < 1
        or len(content_raw) > 4096
        or "\x00" in content
        or not isinstance(project_policy.get("maxBytes"), int)
        or not len(content_raw) <= project_policy["maxBytes"] <= 4096
        or arguments != {
            "contentFingerprint": content_fingerprint,
            "mutationFingerprint": mutation_fingerprint,
            "path": project_policy.get("path"),
            "sizeBytes": len(content_raw),
        }
    ):
        fail("AGENT_PROJECT_CANARY_PLAN_SOURCE is not the strict one-action synthetic Project plan")

    try:
        approval_document = json.loads(
            project_files["AGENT_PROJECT_CANARY_APPROVAL_DOCUMENT_SOURCE"].read_text(),
            object_pairs_hook=unique_object,
        )
        approval_payload = approval_document["payload"]
        approval_signature = base64.urlsafe_b64decode(
            approval_document["signature"] + "=" * (-len(approval_document["signature"]) % 4)
        )
        approval_window = approval_payload["window"]
        approval_issued = datetime.fromisoformat(approval_window["issuedAt"].replace("Z", "+00:00"))
        approval_not_before = datetime.fromisoformat(approval_window["notBefore"].replace("Z", "+00:00"))
        approval_expires = datetime.fromisoformat(approval_window["expiresAt"].replace("Z", "+00:00"))
    except (OSError, UnicodeError, json.JSONDecodeError, DuplicateKey, KeyError, TypeError, ValueError, binascii.Error):
        fail("AGENT_PROJECT_CANARY_APPROVAL_DOCUMENT_SOURCE is invalid")
    approval_action = approval_payload.get("action", {}) if isinstance(approval_payload, dict) else {}
    approval_request = approval_payload.get("request", {}) if isinstance(approval_payload, dict) else {}
    plan_fingerprint = "sha256:" + hashlib.sha256(project_plan_raw).hexdigest()
    activation_material = "\n".join((
        "project_mutation_canary",
        values["AGENT_PROJECT_CANARY_RELEASE_GIT_COMMIT"],
        str(project_plan.get("targetFingerprint", "")),
        values["AGENT_PROJECT_CANARY_RUNNER_ID"],
        plan_fingerprint,
        values["AGENT_PROJECT_CANARY_CLIENT_IDENTITY"],
        values["AGENT_PROJECT_CANARY_RELAY_ENDPOINT"],
    )).encode()
    activation_fingerprint = "sha256:" + hashlib.sha256(
        b"neo-agent-project-canary-activation-binding-v1\0" + activation_material
    ).hexdigest()
    if (
        not isinstance(approval_document, dict)
        or set(approval_document) != {"payload", "signature"}
        or not isinstance(approval_payload, dict)
        or approval_payload.get("schemaVersion") != "neo.agent-project-mutation-approval/v1"
        or re.fullmatch(r"approval_[A-Za-z0-9_-]{16,128}", str(approval_payload.get("approvalId", ""))) is None
        or approval_payload.get("decision") != "approved"
        or approval_payload.get("release") != {
            "gitCommit": values["AGENT_PROJECT_CANARY_RELEASE_GIT_COMMIT"],
            "migrationHead": 94,
        }
        or approval_payload.get("target") != {
            "deploymentFingerprint": project_plan.get("targetFingerprint"),
            "runnerId": values["AGENT_PROJECT_CANARY_RUNNER_ID"],
        }
        or approval_payload.get("activation") != {
            "stage": "project_mutation_canary",
            "activationFingerprint": activation_fingerprint,
            "planFingerprint": plan_fingerprint,
        }
        or approval_request != {
            "callerIdentity": values["AGENT_PROJECT_CANARY_CLIENT_IDENTITY"],
            "requestIdentity": action.get("requestIdentity"),
            "idempotencyKey": action.get("idempotencyKey"),
        }
        or approval_action != {
            "toolIdentity": action.get("toolIdentity"),
            "capability": action.get("capability"),
            "action": action.get("action"),
            "resource": action.get("resource"),
            "baseRevision": action.get("baseRevision"),
            "path": project_policy.get("path"),
            "contentFingerprint": content_fingerprint,
            "mutationFingerprint": mutation_fingerprint,
        }
        or not isinstance(approval_payload.get("actor"), dict)
        or approval_payload["actor"].get("type") != "operator"
        or re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.:@/-]{0,127}", str(approval_payload["actor"].get("id", ""))) is None
        or re.fullmatch(r"[A-Z][A-Z0-9_]{0,63}", str(approval_payload["actor"].get("reasonCode", ""))) is None
        or set(approval_window) != {"issuedAt", "notBefore", "expiresAt"}
        or approval_issued.tzinfo is None
        or approval_not_before.tzinfo is None
        or approval_expires.tzinfo is None
        or approval_issued > approval_not_before
        or approval_not_before > current_time
        or current_time >= approval_expires
        or approval_expires <= approval_not_before
        or (approval_expires - approval_not_before).total_seconds() > 900
        or len(approval_signature) != 64
        or approval_signature == b"\0" * 64
    ):
        fail("AGENT_PROJECT_CANARY_APPROVAL_DOCUMENT_SOURCE is not the exact reviewed approval")

    evaluator = Path(sys.argv[2]) / "scripts/evaluate-agent-production-activation.py"
    try:
        decision = subprocess.run(
            [
                sys.executable, str(evaluator),
                "--record", str(project_files["AGENT_PROJECT_CANARY_ACTIVATION_SOURCE"]),
                "--policy", str(project_files["AGENT_PROJECT_CANARY_PRODUCTION_POLICY_SOURCE"]),
                "--release-manifest", str(project_files["AGENT_PROJECT_CANARY_RELEASE_MANIFEST_SOURCE"]),
                "--client-certificate", str(project_files["AGENT_PROJECT_CANARY_CLIENT_CERT_SOURCE"]),
                "--server-ca", str(project_files["AGENT_PROJECT_CANARY_SERVER_CA_SOURCE"]),
                "--canary-plan", str(project_files["AGENT_PROJECT_CANARY_PLAN_SOURCE"]),
                "--authority-public-key", str(project_files["AGENT_PROJECT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE"]),
                "--approval-document", str(project_files["AGENT_PROJECT_CANARY_APPROVAL_DOCUMENT_SOURCE"]),
                "--approval-public-key", str(project_files["AGENT_PROJECT_CANARY_APPROVAL_PUBLIC_KEY_SOURCE"]),
                "--relay-endpoint", values["AGENT_PROJECT_CANARY_RELAY_ENDPOINT"],
                "--relay-server-certificate", str(project_files["AGENT_PROJECT_CANARY_RELAY_TLS_CERT_SOURCE"]),
                "--relay-client-ca", str(project_files["AGENT_PROJECT_CANARY_RELAY_TLS_CLIENT_CA_SOURCE"]),
                "--runner-relay-identity", values["AGENT_PROJECT_CANARY_RUNNER_RELAY_IDENTITY"],
                "--endpoint", values["AGENT_PROJECT_CANARY_RUNNER_URL"],
                "--runner-id", values["AGENT_PROJECT_CANARY_RUNNER_ID"],
                "--target-fingerprint", project_plan["targetFingerprint"],
                "--server-name", values["AGENT_PROJECT_CANARY_RUNNER_SERVER_NAME"],
                "--caller-identity", values["AGENT_PROJECT_CANARY_CLIENT_IDENTITY"],
                "--release-commit", values["AGENT_PROJECT_CANARY_RELEASE_GIT_COMMIT"],
            ],
            check=False, capture_output=True, text=True, timeout=10,
        )
        decision_payload = json.loads(decision.stdout)
    except (OSError, subprocess.SubprocessError, json.JSONDecodeError):
        fail("Project canary activation evaluator failed")
    if (
        decision.returncode != 0
        or decision_payload.get("verdict") != "ACTIVATION_READY"
        or decision_payload.get("reasonCode") != "PROJECT_MUTATION_CANARY_GATES_PASSED"
    ):
        fail("AGENT_PROJECT_CANARY_ACTIVATION_SOURCE is not READY for G21.3")

agent_child_canary_keys = (
    "AGENT_CHILD_CANARY_DATABASE_URL",
    "AGENT_CHILD_CANARY_RUNNER_URL",
    "AGENT_CHILD_CANARY_RUNNER_ID",
    "AGENT_CHILD_CANARY_SERVER_NAME",
    "AGENT_CHILD_CANARY_CLIENT_IDENTITY",
    "AGENT_CHILD_CANARY_CLIENT_CERT_SOURCE",
    "AGENT_CHILD_CANARY_CLIENT_KEY_SOURCE",
    "AGENT_CHILD_CANARY_SERVER_CA_SOURCE",
    "AGENT_CHILD_CANARY_RELEASE_MANIFEST_SOURCE",
    "AGENT_CHILD_CANARY_PRODUCTION_POLICY_SOURCE",
    "AGENT_CHILD_CANARY_ACTIVATION_SOURCE",
    "AGENT_CHILD_CANARY_PLAN_SOURCE",
    "AGENT_CHILD_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE",
    "AGENT_CHILD_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE",
    "AGENT_CHILD_CANARY_RELEASE_GIT_COMMIT",
    "AGENT_CHILD_CANARY_POLL_INTERVAL",
    "AGENT_CHILD_CANARY_RPC_TIMEOUT",
    "AGENT_CHILD_CANARY_AUTHORITY_TTL",
    "AGENT_CHILD_CANARY_RECONCILE_BATCH_SIZE",
)
if agent_child_canary_enabled:
    if not (
        agent_control_enabled
        and agent_root_canary_enabled
        and agent_broker_canary_enabled
        and agent_project_canary_enabled
    ):
        fail("AGENT_CHILD_CANARY_ENABLED requires G21.0-G21.3 enabled")
    for key in agent_child_canary_keys:
        if not values.get(key, "").strip():
            fail(f"{key} is required when the Child canary is enabled")
        if placeholder.search(values[key]):
            fail(f"{key} still contains a placeholder")
    for key in values:
        if key.startswith("AGENT_CHILD_CANARY_") and any(
            marker in key
            for marker in ("S3", "MCP", "PROVIDER", "VAULT", "REDIS", "RELAY", "EGRESS", "SECRET")
        ):
            fail("Child canary must not receive Broker, relay, Egress, Secret, MCP, Provider, vault or Redis configuration")

    try:
        child_runner_url = urlsplit(values["AGENT_CHILD_CANARY_RUNNER_URL"])
        child_runner_ip = ipaddress.ip_address(child_runner_url.hostname or "")
        child_runner_port = child_runner_url.port
    except ValueError:
        fail("AGENT_CHILD_CANARY_RUNNER_URL must be the exact private HTTPS Runner RPC URL")
    if (
        child_runner_url.scheme != "https"
        or child_runner_port is None
        or not any(
            child_runner_ip in network
            for network in private_networks
            if child_runner_ip.version == network.version
        )
        or child_runner_url.username is not None
        or child_runner_url.password is not None
        or child_runner_url.path != "/internal/neo-runner/v1/rpc"
        or child_runner_url.query
        or child_runner_url.fragment
    ):
        fail("AGENT_CHILD_CANARY_RUNNER_URL must be the exact private HTTPS Runner RPC URL")
    for key in ("AGENT_CHILD_CANARY_RUNNER_ID", "AGENT_CHILD_CANARY_SERVER_NAME"):
        if identity_pattern.fullmatch(values[key]) is None:
            fail(f"{key} is invalid")
    prior_identities = {
        values["AGENT_RUNNER_CLIENT_IDENTITY"],
        values["AGENT_ROOT_CANARY_CLIENT_IDENTITY"],
        values["AGENT_BROKER_CANARY_CLIENT_IDENTITY"],
        values["AGENT_BROKER_CANARY_RUNNER_RELAY_IDENTITY"],
        values["AGENT_PROJECT_CANARY_CLIENT_IDENTITY"],
        values["AGENT_PROJECT_CANARY_RUNNER_RELAY_IDENTITY"],
        values["AGENT_CHILD_CANARY_CLIENT_IDENTITY"],
    }
    if (
        values["AGENT_CHILD_CANARY_CLIENT_IDENTITY"]
        != "spiffe://neo-chat/agent-runtime-child-canary"
        or len(prior_identities) != 7
    ):
        fail("G21.0-G21.4 caller and relay identities must be exact and distinct")
    if (
        re.fullmatch(r"[0-9a-f]{40}", values["AGENT_CHILD_CANARY_RELEASE_GIT_COMMIT"])
        is None
        or values["AGENT_CHILD_CANARY_RELEASE_GIT_COMMIT"] == "0" * 40
    ):
        fail("AGENT_CHILD_CANARY_RELEASE_GIT_COMMIT must be a non-placeholder lowercase Git commit")
    child_poll = parse_simple_duration_seconds(
        "AGENT_CHILD_CANARY_POLL_INTERVAL", values["AGENT_CHILD_CANARY_POLL_INTERVAL"]
    )
    child_rpc = parse_simple_duration_seconds(
        "AGENT_CHILD_CANARY_RPC_TIMEOUT", values["AGENT_CHILD_CANARY_RPC_TIMEOUT"]
    )
    child_authority_ttl = parse_simple_duration_seconds(
        "AGENT_CHILD_CANARY_AUTHORITY_TTL", values["AGENT_CHILD_CANARY_AUTHORITY_TTL"]
    )
    if not 1 <= child_poll <= 60:
        fail("AGENT_CHILD_CANARY_POLL_INTERVAL must be between 1s and 1m")
    if not 1 <= child_rpc <= 10:
        fail("AGENT_CHILD_CANARY_RPC_TIMEOUT must be between 1s and 10s")
    if not 10 <= child_authority_ttl <= 15:
        fail("AGENT_CHILD_CANARY_AUTHORITY_TTL must be between 10s and 15s")
    if (
        re.fullmatch(r"[1-9][0-9]{0,3}", values["AGENT_CHILD_CANARY_RECONCILE_BATCH_SIZE"])
        is None
        or not 1 <= int(values["AGENT_CHILD_CANARY_RECONCILE_BATCH_SIZE"]) <= 1000
    ):
        fail("AGENT_CHILD_CANARY_RECONCILE_BATCH_SIZE must be between 1 and 1000")

    child_files = {
        key: resolve_secure_file(
            values[key], key,
            private=(key != "AGENT_CHILD_CANARY_PRODUCTION_POLICY_SOURCE"),
        )
        for key in (
            "AGENT_CHILD_CANARY_CLIENT_CERT_SOURCE",
            "AGENT_CHILD_CANARY_CLIENT_KEY_SOURCE",
            "AGENT_CHILD_CANARY_SERVER_CA_SOURCE",
            "AGENT_CHILD_CANARY_RELEASE_MANIFEST_SOURCE",
            "AGENT_CHILD_CANARY_PRODUCTION_POLICY_SOURCE",
            "AGENT_CHILD_CANARY_ACTIVATION_SOURCE",
            "AGENT_CHILD_CANARY_PLAN_SOURCE",
            "AGENT_CHILD_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE",
            "AGENT_CHILD_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE",
        )
    }
    if len(set(child_files.values())) != len(child_files):
        fail("Child canary TLS, evidence, plan and authority files must be distinct")
    child_sensitive_files = {
        child_files[key]
        for key in child_files
        if key != "AGENT_CHILD_CANARY_PRODUCTION_POLICY_SOURCE"
    }
    prior_sensitive_files = {
        *(
            canary_files[key]
            for key in canary_files
            if key != "AGENT_ROOT_CANARY_PRODUCTION_POLICY_SOURCE"
        ),
        *(
            broker_files[key]
            for key in broker_files
            if key != "AGENT_BROKER_CANARY_PRODUCTION_POLICY_SOURCE"
        ),
        *(
            project_files[key]
            for key in project_files
            if key != "AGENT_PROJECT_CANARY_PRODUCTION_POLICY_SOURCE"
        ),
        client_certificate,
        client_key,
        server_ca,
        release_manifest,
        activation_record,
    }
    if child_sensitive_files & prior_sensitive_files:
        fail("Child canary files must not reuse prior activation, authority or TLS material")
    try:
        tls_context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        tls_context.load_cert_chain(
            child_files["AGENT_CHILD_CANARY_CLIENT_CERT_SOURCE"],
            child_files["AGENT_CHILD_CANARY_CLIENT_KEY_SOURCE"],
        )
        ssl.create_default_context(cafile=child_files["AGENT_CHILD_CANARY_SERVER_CA_SOURCE"])
        decoded_certificate = ssl._ssl._test_decode_cert(
            str(child_files["AGENT_CHILD_CANARY_CLIENT_CERT_SOURCE"])
        )
    except (OSError, ssl.SSLError, ValueError):
        fail("Child canary mTLS certificate/key material is invalid or mismatched")
    common_names = [
        value
        for relative_name in decoded_certificate.get("subject", ())
        for key, value in relative_name
        if key == "commonName"
    ]
    if common_names != [values["AGENT_CHILD_CANARY_CLIENT_IDENTITY"]]:
        fail("Child canary client certificate identity does not match configuration")
    try:
        authority_private_text = child_files["AGENT_CHILD_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE"].read_text().strip()
        authority_public_text = child_files["AGENT_CHILD_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE"].read_text().strip()
        authority_private_raw = base64.urlsafe_b64decode(
            authority_private_text + "=" * (-len(authority_private_text) % 4)
        )
        authority_public_raw = base64.urlsafe_b64decode(
            authority_public_text + "=" * (-len(authority_public_text) % 4)
        )
    except (OSError, UnicodeError, ValueError, binascii.Error):
        fail("Child canary authority key material is invalid")
    if (
        len(authority_private_raw) != 64
        or len(authority_public_raw) != 32
        or authority_private_raw[32:] != authority_public_raw
    ):
        fail("Child canary authority private/public keys do not match")

    try:
        child_plan = json.loads(
            child_files["AGENT_CHILD_CANARY_PLAN_SOURCE"].read_text(),
            object_pairs_hook=unique_object,
        )
    except (OSError, UnicodeError, json.JSONDecodeError, DuplicateKey):
        fail("AGENT_CHILD_CANARY_PLAN_SOURCE is invalid")
    plan_keys = {
        "schemaVersion", "synthetic", "userId", "projectId", "assistantId", "model",
        "packageFingerprint", "runtimeBundleFingerprint", "parentIdempotencyKey",
        "childIdempotencyKey", "parentStepKind", "childStepKind", "parentGrantId",
        "delegationResource", "grantWindowSeconds", "childExpirySeconds", "parentBudget",
        "childBudget", "toolCatalog", "parentRequestedTools", "childRequestedTools",
        "parentSandbox", "childSandbox", "parentArgv", "childArgv", "parentLeaseSeconds",
        "childLeaseSeconds",
    }
    budget_keys = {"maxWallSeconds", "maxModelTokens", "maxToolCalls", "maxArtifactBytes"}
    sandbox_keys = {
        "runtimeBundleFingerprint", "packageFingerprint", "image", "uid", "gid",
        "rootfsReadOnly", "noNewPrivileges", "capabilities", "seccompProfileFingerprint",
        "networkMode", "workspaceSnapshotId", "workspaceFingerprint", "resources",
    }
    resource_keys = {"cpuMillis", "memoryMiB", "pids", "wallSeconds", "outputBytes", "scratchBytes"}
    parent_budget = child_plan.get("parentBudget", {}) if isinstance(child_plan, dict) else {}
    child_budget = child_plan.get("childBudget", {}) if isinstance(child_plan, dict) else {}
    parent_sandbox = child_plan.get("parentSandbox", {}) if isinstance(child_plan, dict) else {}
    child_sandbox = child_plan.get("childSandbox", {}) if isinstance(child_plan, dict) else {}
    parent_resources = parent_sandbox.get("resources", {}) if isinstance(parent_sandbox, dict) else {}
    child_resources = child_sandbox.get("resources", {}) if isinstance(child_sandbox, dict) else {}
    exact_tool = [{
        "identity": "delegate_task", "capability": "delegate_task", "actions": ["create"],
        "classification": "mutable", "idempotent": False,
    }]
    if (
        not isinstance(child_plan, dict)
        or set(child_plan) != plan_keys
        or child_plan.get("schemaVersion") != "neo.agent-child-run-canary-plan/v1"
        or child_plan.get("synthetic") is not True
        or child_plan.get("parentStepKind") != "child_canary_parent"
        or child_plan.get("childStepKind") != "child_canary_work"
        or child_plan.get("toolCatalog") != exact_tool
        or child_plan.get("parentRequestedTools") != ["delegate_task"]
        or child_plan.get("childRequestedTools") != ["delegate_task"]
        or child_plan.get("parentArgv") != ["/opt/neo/bin/child-canary-parent", "--wait-for-cancel"]
        or child_plan.get("childArgv") != ["/opt/neo/bin/child-canary-child", "--wait-for-cancel"]
        or not isinstance(parent_budget, dict)
        or not isinstance(child_budget, dict)
        or set(parent_budget) != budget_keys
        or set(child_budget) != budget_keys
        or any(
            not isinstance(parent_budget[key], int)
            or not isinstance(child_budget[key], int)
            or not 0 < child_budget[key] < parent_budget[key]
            for key in budget_keys
        )
        or not isinstance(parent_sandbox, dict)
        or not isinstance(child_sandbox, dict)
        or set(parent_sandbox) != sandbox_keys
        or set(child_sandbox) != sandbox_keys
        or any(
            sandbox.get("runtimeBundleFingerprint") != child_plan.get("runtimeBundleFingerprint")
            or sandbox.get("packageFingerprint") != child_plan.get("packageFingerprint")
            or sandbox.get("rootfsReadOnly") is not True
            or sandbox.get("noNewPrivileges") is not True
            or sandbox.get("capabilities") != []
            or sandbox.get("networkMode") != "none"
            for sandbox in (parent_sandbox, child_sandbox)
        )
        or not isinstance(parent_resources, dict)
        or not isinstance(child_resources, dict)
        or set(parent_resources) != resource_keys
        or set(child_resources) != resource_keys
        or any(
            not isinstance(parent_resources[key], int)
            or not isinstance(child_resources[key], int)
            or child_resources[key] > parent_resources[key]
            for key in resource_keys
        )
        or child_resources.get("wallSeconds") != child_budget.get("maxWallSeconds")
        or parent_resources.get("wallSeconds") != parent_budget.get("maxWallSeconds")
    ):
        fail("AGENT_CHILD_CANARY_PLAN_SOURCE is not the strict one-Parent/one-Child plan")

    evaluator = Path(sys.argv[2]) / "scripts/evaluate-agent-production-activation.py"
    try:
        decision = subprocess.run(
            [
                sys.executable, str(evaluator),
                "--record", str(child_files["AGENT_CHILD_CANARY_ACTIVATION_SOURCE"]),
                "--policy", str(child_files["AGENT_CHILD_CANARY_PRODUCTION_POLICY_SOURCE"]),
                "--release-manifest", str(child_files["AGENT_CHILD_CANARY_RELEASE_MANIFEST_SOURCE"]),
                "--client-certificate", str(child_files["AGENT_CHILD_CANARY_CLIENT_CERT_SOURCE"]),
                "--server-ca", str(child_files["AGENT_CHILD_CANARY_SERVER_CA_SOURCE"]),
                "--canary-plan", str(child_files["AGENT_CHILD_CANARY_PLAN_SOURCE"]),
                "--authority-public-key", str(child_files["AGENT_CHILD_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE"]),
                "--endpoint", values["AGENT_CHILD_CANARY_RUNNER_URL"],
                "--runner-id", values["AGENT_CHILD_CANARY_RUNNER_ID"],
                "--server-name", values["AGENT_CHILD_CANARY_SERVER_NAME"],
                "--caller-identity", values["AGENT_CHILD_CANARY_CLIENT_IDENTITY"],
                "--release-commit", values["AGENT_CHILD_CANARY_RELEASE_GIT_COMMIT"],
            ],
            check=False, capture_output=True, text=True, timeout=10,
        )
        decision_payload = json.loads(decision.stdout)
    except (OSError, subprocess.SubprocessError, json.JSONDecodeError):
        fail("Child canary activation evaluator failed")
    if (
        decision.returncode != 0
        or decision_payload.get("verdict") != "ACTIVATION_READY"
        or decision_payload.get("reasonCode") != "DEPTH_ONE_CHILD_CANARY_GATES_PASSED"
    ):
        fail("AGENT_CHILD_CANARY_ACTIVATION_SOURCE is not READY for G21.4")


worker_prerequisite_sources = {
    "control_plane": "AGENT_PRODUCTION_ACTIVATION_SOURCE",
    "root_run_canary": "AGENT_ROOT_CANARY_ACTIVATION_SOURCE",
    "broker_artifact_canary": "AGENT_BROKER_CANARY_ACTIVATION_SOURCE",
    "project_mutation_canary": "AGENT_PROJECT_CANARY_ACTIVATION_SOURCE",
    "child_run_canary": "AGENT_CHILD_CANARY_ACTIVATION_SOURCE",
}


def resolve_worker_prerequisites() -> dict[str, Path]:
    result: dict[str, Path] = {}
    for stage, key in worker_prerequisite_sources.items():
        if not values.get(key, "").strip() or placeholder.search(values[key]):
            fail(f"{key} is required as fresh G21 prerequisite evidence")
        result[stage] = resolve_secure_file(values[key], key, private=True)
    if len(set(result.values())) != len(result):
        fail("G21.0-G21.4 prerequisite evidence files must be distinct")
    return result


def evaluate_exact_worker(
    *,
    record: Path,
    policy: Path,
    plan: Path,
    release_commit: str,
    prerequisites: dict[str, Path],
    expected_reason: str,
    extra_args: list[str] | None = None,
) -> None:
    command = [
        sys.executable,
        str(Path(sys.argv[2]) / "scripts/evaluate-agent-production-activation.py"),
        "--record", str(record),
        "--policy", str(policy),
        "--worker-plan", str(plan),
        "--release-commit", release_commit,
    ]
    for stage, path in sorted(prerequisites.items()):
        command.extend(("--prerequisite", f"{stage}={path}"))
    command.extend(extra_args or [])
    try:
        decision = subprocess.run(
            command, check=False, capture_output=True, text=True, timeout=10
        )
        payload = json.loads(decision.stdout)
    except (OSError, subprocess.SubprocessError, json.JSONDecodeError):
        fail("G21.5 activation evaluator failed")
    if (
        decision.returncode != 0
        or payload.get("verdict") != "ACTIVATION_READY"
        or payload.get("reasonCode") != expected_reason
    ):
        fail(f"G21.5 activation is not READY: {expected_reason}")


agent_cron_worker_keys = (
    "AGENT_CRON_WORKER_DATABASE_URL",
    "AGENT_CRON_WORKER_PLAN_SOURCE",
    "AGENT_CRON_WORKER_PRODUCTION_POLICY_SOURCE",
    "AGENT_CRON_WORKER_ACTIVATION_SOURCE",
    "AGENT_CRON_WORKER_RELEASE_GIT_COMMIT",
)
if agent_cron_worker_enabled:
    for key in agent_cron_worker_keys:
        if not values.get(key, "").strip():
            fail(f"{key} is required when the exact Cron worker is enabled")
        if placeholder.search(values[key]):
            fail(f"{key} still contains a placeholder")
    for key in values:
        if key.startswith("AGENT_CRON_WORKER_") and any(
            marker in key
            for marker in (
                "RUNNER", "CLIENT_CERT", "CLIENT_KEY", "AUTHORITY", "S3", "OBJECT",
                "MCP", "PROVIDER", "VAULT", "REDIS", "ADMIN", "PROMOTE", "SECRET",
            )
        ):
            fail("Cron worker must not receive Runner, object-store or administrator credentials")
    if (
        re.fullmatch(r"[0-9a-f]{40}", values["AGENT_CRON_WORKER_RELEASE_GIT_COMMIT"])
        is None
        or values["AGENT_CRON_WORKER_RELEASE_GIT_COMMIT"] == "0" * 40
    ):
        fail("AGENT_CRON_WORKER_RELEASE_GIT_COMMIT must be a non-placeholder lowercase Git commit")
    cron_plan = resolve_secure_file(
        values["AGENT_CRON_WORKER_PLAN_SOURCE"],
        "AGENT_CRON_WORKER_PLAN_SOURCE",
        private=True,
        maximum_size=64 << 10,
    )
    cron_policy = resolve_secure_file(
        values["AGENT_CRON_WORKER_PRODUCTION_POLICY_SOURCE"],
        "AGENT_CRON_WORKER_PRODUCTION_POLICY_SOURCE",
        private=False,
    )
    cron_activation = resolve_secure_file(
        values["AGENT_CRON_WORKER_ACTIVATION_SOURCE"],
        "AGENT_CRON_WORKER_ACTIVATION_SOURCE",
        private=True,
    )
    try:
        cron_plan_payload = json.loads(
            cron_plan.read_text(), object_pairs_hook=unique_object
        )
    except (OSError, UnicodeError, json.JSONDecodeError, DuplicateKey):
        fail("AGENT_CRON_WORKER_PLAN_SOURCE is invalid")
    cron_plan_keys = {
        "schemaVersion", "synthetic", "activationId", "templateId", "userId",
        "revision", "revisionFingerprint", "validFrom", "validUntil", "owner",
        "pollMillis", "leaseSeconds", "batchSize", "retentionHours",
        "maintenanceEvery",
    }
    if (
        not isinstance(cron_plan_payload, dict)
        or set(cron_plan_payload) != cron_plan_keys
        or cron_plan_payload.get("schemaVersion") != "neo.agent-cron-worker-plan/v1"
        or cron_plan_payload.get("synthetic") is not True
        or not isinstance(cron_plan_payload.get("revision"), int)
        or cron_plan_payload.get("revision", 0) < 1
    ):
        fail("AGENT_CRON_WORKER_PLAN_SOURCE is not one exact synthetic Template plan")
    cron_prerequisites = resolve_worker_prerequisites()
    evaluate_exact_worker(
        record=cron_activation,
        policy=cron_policy,
        plan=cron_plan,
        release_commit=values["AGENT_CRON_WORKER_RELEASE_GIT_COMMIT"],
        prerequisites=cron_prerequisites,
        expected_reason="CRON_WORKER_GATES_PASSED",
    )


agent_draft_learning_worker_keys = (
    "AGENT_DRAFT_LEARNING_WORKER_DATABASE_URL",
    "AGENT_DRAFT_LEARNING_WORKER_RUNNER_URL",
    "AGENT_DRAFT_LEARNING_WORKER_RUNNER_ID",
    "AGENT_DRAFT_LEARNING_WORKER_SERVER_NAME",
    "AGENT_DRAFT_LEARNING_WORKER_CLIENT_IDENTITY",
    "AGENT_DRAFT_LEARNING_WORKER_PLAN_SOURCE",
    "AGENT_DRAFT_LEARNING_WORKER_CLIENT_CERT_SOURCE",
    "AGENT_DRAFT_LEARNING_WORKER_CLIENT_KEY_SOURCE",
    "AGENT_DRAFT_LEARNING_WORKER_SERVER_CA_SOURCE",
    "AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PRIVATE_KEY_SOURCE",
    "AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PUBLIC_KEY_SOURCE",
    "AGENT_DRAFT_LEARNING_WORKER_RELEASE_MANIFEST_SOURCE",
    "AGENT_DRAFT_LEARNING_WORKER_PRODUCTION_POLICY_SOURCE",
    "AGENT_DRAFT_LEARNING_WORKER_ACTIVATION_SOURCE",
    "AGENT_DRAFT_LEARNING_WORKER_OBJECT_CREDENTIAL_META_SOURCE",
    "AGENT_DRAFT_LEARNING_WORKER_RELEASE_GIT_COMMIT",
    "AGENT_DRAFT_LEARNING_WORKER_OWNER",
    "AGENT_DRAFT_LEARNING_WORKER_POLL_INTERVAL",
    "AGENT_DRAFT_LEARNING_WORKER_RPC_TIMEOUT",
    "AGENT_DRAFT_LEARNING_WORKER_LEASE_DURATION",
    "AGENT_DRAFT_LEARNING_WORKER_BATCH_SIZE",
    "AGENT_DRAFT_LEARNING_WORKER_RETENTION",
    "AGENT_DRAFT_LEARNING_WORKER_MAINTENANCE_EVERY",
    "AGENT_DRAFT_LEARNING_WORKER_S3_ACCESS_KEY_ID",
    "AGENT_DRAFT_LEARNING_WORKER_S3_SECRET_ACCESS_KEY",
)
if agent_draft_learning_worker_enabled:
    for key in agent_draft_learning_worker_keys:
        if not values.get(key, "").strip():
            fail(f"{key} is required when the Draft-learning worker is enabled")
        if placeholder.search(values[key]):
            fail(f"{key} still contains a placeholder")
    for key in values:
        if key.startswith("AGENT_DRAFT_LEARNING_WORKER_") and any(
            marker in key for marker in ("MCP", "PROVIDER", "VAULT", "REDIS", "ADMIN", "PROMOTE", "EGRESS")
        ):
            fail("Draft-learning worker must not receive Promote, Broker or product credentials")
    try:
        draft_runner_url = urlsplit(values["AGENT_DRAFT_LEARNING_WORKER_RUNNER_URL"])
        draft_runner_ip = ipaddress.ip_address(draft_runner_url.hostname or "")
        draft_runner_port = draft_runner_url.port
    except ValueError:
        fail("AGENT_DRAFT_LEARNING_WORKER_RUNNER_URL must be the exact private HTTPS Runner RPC URL")
    if (
        draft_runner_url.scheme != "https"
        or draft_runner_port is None
        or not any(
            draft_runner_ip in network
            for network in private_networks
            if draft_runner_ip.version == network.version
        )
        or draft_runner_url.username is not None
        or draft_runner_url.password is not None
        or draft_runner_url.path != "/internal/neo-runner/v1/rpc"
        or draft_runner_url.query
        or draft_runner_url.fragment
    ):
        fail("AGENT_DRAFT_LEARNING_WORKER_RUNNER_URL must be the exact private HTTPS Runner RPC URL")
    for key in (
        "AGENT_DRAFT_LEARNING_WORKER_RUNNER_ID",
        "AGENT_DRAFT_LEARNING_WORKER_SERVER_NAME",
    ):
        if identity_pattern.fullmatch(values[key]) is None:
            fail(f"{key} is invalid")
    if values["AGENT_DRAFT_LEARNING_WORKER_CLIENT_IDENTITY"] != (
        "spiffe://neo-chat/agent-runtime-draft-learning"
    ):
        fail("AGENT_DRAFT_LEARNING_WORKER_CLIENT_IDENTITY must be the dedicated Draft identity")
    prior_identity_keys = (
        "AGENT_RUNNER_CLIENT_IDENTITY",
        "AGENT_ROOT_CANARY_CLIENT_IDENTITY",
        "AGENT_BROKER_CANARY_CLIENT_IDENTITY",
        "AGENT_BROKER_CANARY_RUNNER_RELAY_IDENTITY",
        "AGENT_PROJECT_CANARY_CLIENT_IDENTITY",
        "AGENT_PROJECT_CANARY_RUNNER_RELAY_IDENTITY",
        "AGENT_CHILD_CANARY_CLIENT_IDENTITY",
        "AGENT_DRAFT_LEARNING_WORKER_CLIENT_IDENTITY",
    )
    if len({values.get(key, "") for key in prior_identity_keys}) != len(prior_identity_keys):
        fail("G21.0-G21.5 caller and relay identities must be exact and distinct")
    if (
        re.fullmatch(r"[0-9a-f]{40}", values["AGENT_DRAFT_LEARNING_WORKER_RELEASE_GIT_COMMIT"])
        is None
        or values["AGENT_DRAFT_LEARNING_WORKER_RELEASE_GIT_COMMIT"] == "0" * 40
    ):
        fail("AGENT_DRAFT_LEARNING_WORKER_RELEASE_GIT_COMMIT must be a non-placeholder lowercase Git commit")
    draft_poll = parse_simple_duration_seconds(
        "AGENT_DRAFT_LEARNING_WORKER_POLL_INTERVAL",
        values["AGENT_DRAFT_LEARNING_WORKER_POLL_INTERVAL"],
    )
    draft_rpc = parse_simple_duration_seconds(
        "AGENT_DRAFT_LEARNING_WORKER_RPC_TIMEOUT",
        values["AGENT_DRAFT_LEARNING_WORKER_RPC_TIMEOUT"],
    )
    draft_lease = parse_simple_duration_seconds(
        "AGENT_DRAFT_LEARNING_WORKER_LEASE_DURATION",
        values["AGENT_DRAFT_LEARNING_WORKER_LEASE_DURATION"],
    )
    draft_retention = parse_simple_duration_seconds(
        "AGENT_DRAFT_LEARNING_WORKER_RETENTION",
        values["AGENT_DRAFT_LEARNING_WORKER_RETENTION"],
    )
    if not 1 <= draft_poll <= 60 or not 1 <= draft_rpc <= 60 or not 30 <= draft_lease <= 300:
        fail("Draft-learning poll, RPC or lease duration is outside the reviewed bound")
    if not 3600 <= draft_retention <= 365 * 24 * 3600:
        fail("AGENT_DRAFT_LEARNING_WORKER_RETENTION is outside the reviewed bound")
    for key, minimum, maximum in (
        ("AGENT_DRAFT_LEARNING_WORKER_BATCH_SIZE", 1, 1000),
        ("AGENT_DRAFT_LEARNING_WORKER_MAINTENANCE_EVERY", 1, 10000),
    ):
        if re.fullmatch(r"[1-9][0-9]*", values[key]) is None or not minimum <= int(values[key]) <= maximum:
            fail(f"{key} is outside the reviewed bound")
    if (
        values.get("STORAGE_BACKEND") not in {"minio", "s3"}
        or values.get("S3_BUCKET_AUTO_CREATE") != "false"
        or values.get("S3_USE_SSL") not in {"true", "false"}
        or values.get("S3_FORCE_PATH_STYLE") not in {"true", "false"}
        or values["AGENT_DRAFT_LEARNING_WORKER_S3_ACCESS_KEY_ID"]
        in {values.get("S3_ACCESS_KEY_ID"), values.get("MINIO_ROOT_USER")}
        or values["AGENT_DRAFT_LEARNING_WORKER_S3_SECRET_ACCESS_KEY"]
        in {values.get("S3_SECRET_ACCESS_KEY"), values.get("MINIO_ROOT_PASSWORD")}
    ):
        fail("Draft-learning worker requires a distinct existing S3-compatible bucket credential")
    draft_file_keys = (
        "AGENT_DRAFT_LEARNING_WORKER_PLAN_SOURCE",
        "AGENT_DRAFT_LEARNING_WORKER_CLIENT_CERT_SOURCE",
        "AGENT_DRAFT_LEARNING_WORKER_CLIENT_KEY_SOURCE",
        "AGENT_DRAFT_LEARNING_WORKER_SERVER_CA_SOURCE",
        "AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PRIVATE_KEY_SOURCE",
        "AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PUBLIC_KEY_SOURCE",
        "AGENT_DRAFT_LEARNING_WORKER_RELEASE_MANIFEST_SOURCE",
        "AGENT_DRAFT_LEARNING_WORKER_ACTIVATION_SOURCE",
        "AGENT_DRAFT_LEARNING_WORKER_OBJECT_CREDENTIAL_META_SOURCE",
    )
    draft_files = {
        key: resolve_secure_file(values[key], key, private=True)
        for key in draft_file_keys
    }
    draft_policy = resolve_secure_file(
        values["AGENT_DRAFT_LEARNING_WORKER_PRODUCTION_POLICY_SOURCE"],
        "AGENT_DRAFT_LEARNING_WORKER_PRODUCTION_POLICY_SOURCE",
        private=False,
    )
    if len(set(draft_files.values())) != len(draft_files):
        fail("Draft-learning TLS, evidence, plan, authority and credential metadata files must be distinct")
    prior_source_values = {
        values.get(key, "")
        for key in (
            "AGENT_RUNNER_CLIENT_CERT_SOURCE", "AGENT_RUNNER_CLIENT_KEY_SOURCE",
            "AGENT_RUNNER_SERVER_CA_SOURCE", "AGENT_RUNNER_RELEASE_MANIFEST_SOURCE",
            "AGENT_ROOT_CANARY_CLIENT_CERT_SOURCE", "AGENT_ROOT_CANARY_CLIENT_KEY_SOURCE",
            "AGENT_ROOT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE", "AGENT_ROOT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE",
            "AGENT_BROKER_CANARY_CLIENT_CERT_SOURCE", "AGENT_BROKER_CANARY_CLIENT_KEY_SOURCE",
            "AGENT_BROKER_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE", "AGENT_BROKER_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE",
            "AGENT_PROJECT_CANARY_CLIENT_CERT_SOURCE", "AGENT_PROJECT_CANARY_CLIENT_KEY_SOURCE",
            "AGENT_PROJECT_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE", "AGENT_PROJECT_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE",
            "AGENT_CHILD_CANARY_CLIENT_CERT_SOURCE", "AGENT_CHILD_CANARY_CLIENT_KEY_SOURCE",
            "AGENT_CHILD_CANARY_AUTHORITY_PRIVATE_KEY_SOURCE", "AGENT_CHILD_CANARY_AUTHORITY_PUBLIC_KEY_SOURCE",
        )
        if values.get(key, "")
    }
    if {values[key] for key in draft_file_keys} & prior_source_values:
        fail("Draft-learning files must not reuse prior Runner or canary key material")
    try:
        tls_context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        tls_context.load_cert_chain(
            draft_files["AGENT_DRAFT_LEARNING_WORKER_CLIENT_CERT_SOURCE"],
            draft_files["AGENT_DRAFT_LEARNING_WORKER_CLIENT_KEY_SOURCE"],
        )
        ssl.create_default_context(
            cafile=draft_files["AGENT_DRAFT_LEARNING_WORKER_SERVER_CA_SOURCE"]
        )
        decoded_certificate = ssl._ssl._test_decode_cert(
            str(draft_files["AGENT_DRAFT_LEARNING_WORKER_CLIENT_CERT_SOURCE"])
        )
    except (OSError, ssl.SSLError, ValueError):
        fail("Draft-learning mTLS certificate/key material is invalid or mismatched")
    common_names = [
        value
        for relative_name in decoded_certificate.get("subject", ())
        for key, value in relative_name
        if key == "commonName"
    ]
    if common_names != [values["AGENT_DRAFT_LEARNING_WORKER_CLIENT_IDENTITY"]]:
        fail("Draft-learning client certificate identity does not match configuration")
    try:
        authority_private_text = draft_files[
            "AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PRIVATE_KEY_SOURCE"
        ].read_text().strip()
        authority_public_text = draft_files[
            "AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PUBLIC_KEY_SOURCE"
        ].read_text().strip()
        authority_private_raw = base64.urlsafe_b64decode(
            authority_private_text + "=" * (-len(authority_private_text) % 4)
        )
        authority_public_raw = base64.urlsafe_b64decode(
            authority_public_text + "=" * (-len(authority_public_text) % 4)
        )
    except (OSError, UnicodeError, ValueError, binascii.Error):
        fail("Draft-learning authority key material is invalid")
    if (
        len(authority_private_raw) != 64
        or len(authority_public_raw) != 32
        or authority_private_raw[32:] != authority_public_raw
    ):
        fail("Draft-learning authority private/public keys do not match")
    try:
        draft_plan_payload = json.loads(
            draft_files["AGENT_DRAFT_LEARNING_WORKER_PLAN_SOURCE"].read_text(),
            object_pairs_hook=unique_object,
        )
    except (OSError, UnicodeError, json.JSONDecodeError, DuplicateKey):
        fail("AGENT_DRAFT_LEARNING_WORKER_PLAN_SOURCE is invalid")
    draft_sandbox = draft_plan_payload.get("sandbox", {}) if isinstance(draft_plan_payload, dict) else {}
    draft_checks = draft_plan_payload.get("checks", []) if isinstance(draft_plan_payload, dict) else []
    if (
        not isinstance(draft_plan_payload, dict)
        or draft_plan_payload.get("schemaVersion") != "neo.agent-draft-learning-worker-plan/v1"
        or draft_plan_payload.get("synthetic") is not True
        or draft_plan_payload.get("runnerId") != values["AGENT_DRAFT_LEARNING_WORKER_RUNNER_ID"]
        or draft_plan_payload.get("callerIdentity") != values["AGENT_DRAFT_LEARNING_WORKER_CLIENT_IDENTITY"]
        or draft_plan_payload.get("toolRegistry", {}).get("tools") != []
        or draft_sandbox.get("networkMode") != "none"
        or draft_sandbox.get("capabilities") != []
        or draft_sandbox.get("rootfsReadOnly") is not True
        or draft_sandbox.get("noNewPrivileges") is not True
        or [item.get("kind") for item in draft_checks if isinstance(item, dict)] != ["isolation", "evaluation"]
        or [item.get("argv", [None])[0] for item in draft_checks if isinstance(item, dict)]
        != ["/opt/neo/bin/draft-isolation-check", "/opt/neo/bin/draft-evaluation-check"]
    ):
        fail("AGENT_DRAFT_LEARNING_WORKER_PLAN_SOURCE is not the strict rootless Draft-check plan")
    draft_prerequisites = resolve_worker_prerequisites()
    evaluate_exact_worker(
        record=draft_files["AGENT_DRAFT_LEARNING_WORKER_ACTIVATION_SOURCE"],
        policy=draft_policy,
        plan=draft_files["AGENT_DRAFT_LEARNING_WORKER_PLAN_SOURCE"],
        release_commit=values["AGENT_DRAFT_LEARNING_WORKER_RELEASE_GIT_COMMIT"],
        prerequisites=draft_prerequisites,
        expected_reason="DRAFT_LEARNING_WORKER_GATES_PASSED",
        extra_args=[
            "--release-manifest", str(draft_files["AGENT_DRAFT_LEARNING_WORKER_RELEASE_MANIFEST_SOURCE"]),
            "--client-certificate", str(draft_files["AGENT_DRAFT_LEARNING_WORKER_CLIENT_CERT_SOURCE"]),
            "--server-ca", str(draft_files["AGENT_DRAFT_LEARNING_WORKER_SERVER_CA_SOURCE"]),
            "--authority-public-key", str(draft_files["AGENT_DRAFT_LEARNING_WORKER_AUTHORITY_PUBLIC_KEY_SOURCE"]),
            "--object-credential-meta", str(draft_files["AGENT_DRAFT_LEARNING_WORKER_OBJECT_CREDENTIAL_META_SOURCE"]),
            "--endpoint", values["AGENT_DRAFT_LEARNING_WORKER_RUNNER_URL"],
            "--runner-id", values["AGENT_DRAFT_LEARNING_WORKER_RUNNER_ID"],
            "--server-name", values["AGENT_DRAFT_LEARNING_WORKER_SERVER_NAME"],
            "--caller-identity", values["AGENT_DRAFT_LEARNING_WORKER_CLIENT_IDENTITY"],
        ],
    )


marketplace_timeout = parse_simple_duration_seconds(
    "MCP_MARKETPLACE_TIMEOUT", values["MCP_MARKETPLACE_TIMEOUT"]
)
if not 1 <= marketplace_timeout <= 30:
    fail("MCP_MARKETPLACE_TIMEOUT must be between 1s and 30s")
marketplace_cache_ttl = parse_simple_duration_seconds(
    "MCP_MARKETPLACE_CACHE_TTL", values["MCP_MARKETPLACE_CACHE_TTL"]
)
if not 10 <= marketplace_cache_ttl <= 3600:
    fail("MCP_MARKETPLACE_CACHE_TTL must be between 10s and 1h")
if values["POSTGRES_DATA_DIR"] != "./data/postgres17":
    fail("POSTGRES_DATA_DIR must be ./data/postgres17")

if values["MM_CHAT_VERSION"].lower() in {"dev", "local", "single-server-dev"}:
    fail("MM_CHAT_VERSION must identify the release")

database_urls = {}
for key in (
    "MIGRATION_DATABASE_URL",
    "DATABASE_URL",
    "MEMORY_WORKER_DATABASE_URL",
    "RAG_WORKER_DATABASE_URL",
    "RAG_REPLAY_DATABASE_URL",
    *(("AGENT_RUNNER_DATABASE_URL",) if agent_control_enabled else ()),
    *(("AGENT_ROOT_CANARY_DATABASE_URL",) if agent_root_canary_enabled else ()),
    *(("AGENT_BROKER_CANARY_DATABASE_URL",) if agent_broker_canary_enabled else ()),
    *(("AGENT_PROJECT_CANARY_DATABASE_URL",) if agent_project_canary_enabled else ()),
    *(("AGENT_CHILD_CANARY_DATABASE_URL",) if agent_child_canary_enabled else ()),
    *(("AGENT_CRON_WORKER_DATABASE_URL",) if agent_cron_worker_enabled else ()),
    *(("AGENT_DRAFT_LEARNING_WORKER_DATABASE_URL",) if agent_draft_learning_worker_enabled else ()),
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
    fail("migration, API, workers, Agent stages and canaries must use distinct database principals")

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

print("single-server preflight: passed")
PY
