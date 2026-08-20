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
import json
import os
import re
import stat
import sys
import ipaddress
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

if (
    "AGENT_TIMELINE_ENABLED" in values
    and values["AGENT_TIMELINE_ENABLED"] not in {"true", "false"}
):
    fail("AGENT_TIMELINE_ENABLED must be true or false")

if values.get("AGENT_TIMELINE_CANARY_USER_IDS", ""):
    uuid_pattern = re.compile(
        r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-"
        r"[0-9a-fA-F]{4}-[0-9a-fA-F]{12}"
    )
    canary_ids = [value.strip() for value in values["AGENT_TIMELINE_CANARY_USER_IDS"].split(",")]
    normalized_canary_ids = [value.lower() for value in canary_ids]
    if (
        any(not uuid_pattern.fullmatch(value) for value in canary_ids)
        or len(set(normalized_canary_ids)) != len(normalized_canary_ids)
    ):
        fail("AGENT_TIMELINE_CANARY_USER_IDS must be a unique comma-separated UUID list")

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

agent_local_keys = (
    "AGENT_LOCAL_RUNTIME_ENABLED",
    "AGENT_LOCAL_RUNTIME_SOURCE",
    "AGENT_LOCAL_RUNTIME_ROOT",
    "AGENT_LOCAL_WORKSPACE_SOURCE",
    "AGENT_LOCAL_WORKSPACE_ROOT",
    "AGENT_LOCAL_SHELL",
    "AGENT_LOCAL_APPROVAL_MODE",
    "AGENT_LOCAL_CALL_TIMEOUT",
    "AGENT_LOCAL_RUN_TIMEOUT",
    "AGENT_LOCAL_MAX_OUTPUT_BYTES",
    "AGENT_LOCAL_MAX_CALLS_PER_RUN",
    "AGENT_LOCAL_MAX_ROUNDS_PER_RUN",
    "AGENT_LOCAL_MAX_CONCURRENT",
)
if values.get("AGENT_LOCAL_RUNTIME_ENABLED") not in {"true", "false"}:
    fail("AGENT_LOCAL_RUNTIME_ENABLED must be true or false")

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
    *agent_local_keys,
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

if values["AGENT_LOCAL_RUNTIME_ROOT"] != "/var/lib/mm-chat/agent-skills":
    fail("AGENT_LOCAL_RUNTIME_ROOT must be /var/lib/mm-chat/agent-skills")
if values["AGENT_LOCAL_WORKSPACE_ROOT"] != "/workspace":
    fail("AGENT_LOCAL_WORKSPACE_ROOT must be /workspace")
if values["AGENT_LOCAL_SHELL"] != "/bin/bash":
    fail("AGENT_LOCAL_SHELL must be /bin/bash")
if values["AGENT_LOCAL_APPROVAL_MODE"] not in {"smart", "off"}:
    fail("AGENT_LOCAL_APPROVAL_MODE must be smart or off")

local_call_timeout = parse_simple_duration_seconds(
    "AGENT_LOCAL_CALL_TIMEOUT", values["AGENT_LOCAL_CALL_TIMEOUT"]
)
local_run_timeout = parse_simple_duration_seconds(
    "AGENT_LOCAL_RUN_TIMEOUT", values["AGENT_LOCAL_RUN_TIMEOUT"]
)
if not 1 <= local_call_timeout <= 600:
    fail("AGENT_LOCAL_CALL_TIMEOUT must be between 1s and 10m")
if not local_call_timeout <= local_run_timeout <= 1800:
    fail("AGENT_LOCAL_RUN_TIMEOUT must be at least the call timeout and at most 30m")

for name, minimum, maximum in (
    ("AGENT_LOCAL_MAX_OUTPUT_BYTES", 1024, 8 << 20),
    ("AGENT_LOCAL_MAX_CALLS_PER_RUN", 1, 128),
    ("AGENT_LOCAL_MAX_ROUNDS_PER_RUN", 1, 32),
    ("AGENT_LOCAL_MAX_CONCURRENT", 1, 32),
):
    value = values[name]
    if not re.fullmatch(r"[1-9][0-9]*", value) or not minimum <= int(value) <= maximum:
        fail(f"{name} must be between {minimum} and {maximum}")

local_sources = []
data_root = (Path(sys.argv[2]) / "data").resolve()
for name in ("AGENT_LOCAL_RUNTIME_SOURCE", "AGENT_LOCAL_WORKSPACE_SOURCE"):
    configured = Path(values[name])
    if configured.is_absolute():
        fail(f"{name} must be a project-relative data path")
    source = (Path(sys.argv[2]) / configured).resolve()
    if data_root not in source.parents:
        fail(f"{name} must remain below the project data directory")
    try:
        source_lstat = source.lstat()
    except OSError:
        fail(f"{name} directory is unavailable")
    if stat.S_ISLNK(source_lstat.st_mode) or not stat.S_ISDIR(source_lstat.st_mode):
        fail(f"{name} must be a non-symlink directory")
    if source_lstat.st_uid != os.getuid() or source_lstat.st_gid != os.getgid():
        fail(f"{name} must be owned by the runtime UID/GID")
    if not os.access(source, os.R_OK | os.W_OK | os.X_OK):
        fail(f"{name} must be readable, writable and searchable")
    local_sources.append(source)
if len(set(local_sources)) != len(local_sources):
    fail("AGENT_LOCAL_RUNTIME_SOURCE and AGENT_LOCAL_WORKSPACE_SOURCE must be distinct")

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
