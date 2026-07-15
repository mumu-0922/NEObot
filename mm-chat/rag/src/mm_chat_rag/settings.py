"""Strict environment-only worker configuration.

This module intentionally does not load dotenv files. Secrets remain in process
environment or mounted secret providers and are never represented in logs.
"""

from __future__ import annotations

import os
import re
import uuid
from collections.abc import Mapping
from dataclasses import dataclass, field
from typing import Final, Self
from urllib.parse import urlsplit

from mm_chat_rag.provider_profile import (
    DEFAULT_JINA_EMBEDDING_DIMENSIONS,
    DEFAULT_JINA_EMBEDDING_MODEL,
    DEFAULT_JINA_REQUESTS_PER_MINUTE,
    DEFAULT_JINA_RERANK_MODEL,
    DEFAULT_MINERU_REQUESTS_PER_MINUTE,
    DEFAULT_PROVIDER_CONCURRENCY,
    DEFAULT_PROVIDER_INITIAL_RETRY_SECONDS,
    DEFAULT_PROVIDER_MAX_RETRY_SECONDS,
    DISABLED_PROVIDER_PROFILE,
    PROVIDER_RETRY_MAX_ATTEMPTS,
    ProviderProfileError,
    ProviderRuntimeProfile,
)

_NAME_RE: Final = re.compile(r"^[a-z][a-z0-9_.-]{0,62}$")
_PREFIX_RE: Final = re.compile(r"^[A-Za-z0-9:_-]{1,64}$")
ALLOWED_JOB_STAGES: Final[frozenset[str]] = frozenset(
    {"parse", "passage_embedding", "purge"}
)
_KNOWN_ENV: Final = {
    "RAG_WORKER_DATABASE_URL",
    "RAG_WORKER_REDIS_URL",
    "RAG_WORKER_CONSUMER",
    "RAG_WORKER_ID",
    "RAG_WORKER_DISPATCH_ENABLED",
    "RAG_WORKER_JOB_STAGES",
    "RAG_WORKER_POLL_SECONDS",
    "RAG_WORKER_RESCAN_SECONDS",
    "RAG_WORKER_OUTBOX_LEASE_SECONDS",
    "RAG_WORKER_JOB_LEASE_SECONDS",
    "RAG_WORKER_HEARTBEAT_SECONDS",
    "RAG_WORKER_SHUTDOWN_GRACE_SECONDS",
    "RAG_WORKER_STATEMENT_TIMEOUT_MS",
    "RAG_WORKER_LOCK_TIMEOUT_MS",
    "RAG_WORKER_IDLE_TRANSACTION_TIMEOUT_MS",
    "RAG_WORKER_HEALTH_HOST",
    "RAG_WORKER_HEALTH_PORT",
    "RAG_WORKER_LOG_LEVEL",
    "RAG_WORKER_MAX_RESCAN_CLAIMS",
    "RAG_MINERU_API_TOKEN",
    "DEFAULT_MINERU_API_TOKEN",
    "RAG_JINA_API_KEY",
    "DEFAULT_JINA_API_KEY",
    "RAG_PROVIDER_PROFILE",
    "RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED",
    "RAG_PROVIDER_RETRY_MAX_ATTEMPTS",
    "RAG_PROVIDER_INITIAL_RETRY_SECONDS",
    "RAG_PROVIDER_MAX_RETRY_SECONDS",
    "RAG_PROVIDER_CONCURRENCY",
    "RAG_MINERU_REQUESTS_PER_MINUTE",
    "RAG_JINA_REQUESTS_PER_MINUTE",
    "RAG_JINA_EMBEDDING_MODEL",
    "RAG_JINA_RERANK_MODEL",
}


class SettingsError(ValueError):
    """Raised when environment configuration is missing or unsafe."""


def _required(env: Mapping[str, str], name: str) -> str:
    value = env.get(name, "").strip()
    if not value:
        raise SettingsError(f"{name} is required")
    return value


def _optional_alias(env: Mapping[str, str], *names: str) -> str | None:
    for name in names:
        value = env.get(name, "").strip()
        if value:
            return value
    return None


def _boolean(env: Mapping[str, str], name: str, default: bool) -> bool:
    raw = env.get(name)
    if raw is None:
        return default
    if raw == "true":
        return True
    if raw == "false":
        return False
    raise SettingsError(f"{name} must be exactly 'true' or 'false'")


def _integer(
    env: Mapping[str, str], name: str, default: int, minimum: int, maximum: int
) -> int:
    raw = env.get(name, str(default))
    try:
        value = int(raw)
    except ValueError as error:
        raise SettingsError(f"{name} must be an integer") from error
    if not minimum <= value <= maximum:
        raise SettingsError(f"{name} must be between {minimum} and {maximum}")
    return value


def _stages(raw: str) -> tuple[str, ...]:
    if not raw.strip():
        return ()
    values = tuple(item.strip() for item in raw.split(","))
    _validate_job_stages(values)
    return values


def _validate_job_stages(values: tuple[str, ...]) -> None:
    if any(
        not isinstance(value, str) or value not in ALLOWED_JOB_STAGES
        for value in values
    ):
        raise SettingsError("RAG_WORKER_JOB_STAGES contains an unknown stage")
    if len(values) != len(set(values)):
        raise SettingsError("RAG_WORKER_JOB_STAGES contains duplicates")


def _service_url(value: str, name: str, schemes: set[str]) -> str:
    try:
        parsed = urlsplit(value)
        valid = parsed.scheme in schemes and bool(parsed.hostname)
    except ValueError:
        valid = False
    if not valid:
        raise SettingsError(f"{name} must be a valid service URL")
    return value


@dataclass(frozen=True, slots=True)
class Settings:
    """Validated worker settings with production-safe defaults."""

    database_url: str
    redis_url: str | None = None
    redis_key_prefix: str = "mm-chat"
    consumer_name: str = "mm-chat-rag-v1"
    worker_id: uuid.UUID = field(default_factory=uuid.uuid4)
    dispatch_enabled: bool = False
    job_stages: tuple[str, ...] = ()
    poll_interval_seconds: int = 1
    forced_rescan_seconds: int = 30
    outbox_lease_seconds: int = 30
    job_lease_seconds: int = 90
    heartbeat_seconds: int = 30
    shutdown_grace_seconds: int = 120
    statement_timeout_ms: int = 10_000
    lock_timeout_ms: int = 2_000
    idle_transaction_timeout_ms: int = 10_000
    health_host: str = "0.0.0.0"
    health_port: int = 8081
    log_level: str = "INFO"
    advisory_lock_key: int = 5_567_946_413_527_621_955
    max_rescan_claims: int = 100
    mineru_api_key: str | None = None
    jina_api_key: str | None = None
    jina_embedding_dimensions: int = DEFAULT_JINA_EMBEDDING_DIMENSIONS
    provider_profile: ProviderRuntimeProfile = field(
        default_factory=ProviderRuntimeProfile
    )

    def __post_init__(self) -> None:
        """Reject stages outside the SQL claim-function contract."""
        _validate_job_stages(self.job_stages)
        if self.jina_embedding_dimensions != DEFAULT_JINA_EMBEDDING_DIMENSIONS:
            raise SettingsError("Jina embedding dimensions must be 1024")
        try:
            self.provider_profile.validate_static_contract()
        except ProviderProfileError as error:
            raise SettingsError(str(error)) from error
        if (
            self.dispatch_enabled
            and "parse" in self.job_stages
            and self.mineru_api_key is None
        ):
            raise SettingsError(
                "RAG_MINERU_API_TOKEN is required when parse dispatch is enabled"
            )
        if (
            self.dispatch_enabled
            and "passage_embedding" in self.job_stages
            and self.jina_api_key is None
        ):
            raise SettingsError(
                "RAG_JINA_API_KEY is required when embedding dispatch is enabled"
            )
        try:
            self.provider_profile.validate_for_job_stages(
                self.job_stages,
                mineru_configured=self.mineru_api_key is not None,
                jina_configured=self.jina_api_key is not None,
            )
        except ProviderProfileError as error:
            raise SettingsError(str(error)) from error

    @property
    def redis_channel(self) -> str:
        """Return the bounded, deployment-namespaced wake channel."""
        return f"{self.redis_key_prefix}:rag:outbox:v1"

    @classmethod
    def from_env(cls, environ: Mapping[str, str] | None = None) -> Self:
        """Build settings from an explicit mapping or ``os.environ`` only."""
        env = os.environ if environ is None else environ
        unknown = sorted(
            key
            for key in env
            if key.startswith("RAG_WORKER_") and key not in _KNOWN_ENV
        )
        if unknown:
            raise SettingsError(f"unknown worker setting: {unknown[0]}")
        database_url = _service_url(
            _required(env, "RAG_WORKER_DATABASE_URL"),
            "RAG_WORKER_DATABASE_URL",
            {"postgres", "postgresql"},
        )
        redis_url = env.get("RAG_WORKER_REDIS_URL", "").strip() or None
        if redis_url is not None:
            redis_url = _service_url(
                redis_url, "RAG_WORKER_REDIS_URL", {"redis", "rediss"}
            )
        prefix = env.get("REDIS_KEY_PREFIX", "mm-chat").strip()
        if not _PREFIX_RE.fullmatch(prefix):
            raise SettingsError("REDIS_KEY_PREFIX is invalid")
        consumer = env.get("RAG_WORKER_CONSUMER", "mm-chat-rag-v1").strip()
        if not _NAME_RE.fullmatch(consumer):
            raise SettingsError("RAG_WORKER_CONSUMER is invalid")
        raw_worker_id = env.get("RAG_WORKER_ID")
        try:
            worker_id = uuid.UUID(raw_worker_id) if raw_worker_id else uuid.uuid4()
        except ValueError as error:
            raise SettingsError("RAG_WORKER_ID must be a UUID") from error

        dispatch_enabled = _boolean(env, "RAG_WORKER_DISPATCH_ENABLED", default=False)
        job_stages = _stages(env.get("RAG_WORKER_JOB_STAGES", ""))
        if not dispatch_enabled and job_stages:
            raise SettingsError(
                "RAG_WORKER_JOB_STAGES must be empty while dispatch is disabled"
            )
        mineru_api_key = _optional_alias(
            env, "RAG_MINERU_API_TOKEN", "DEFAULT_MINERU_API_TOKEN"
        )
        jina_api_key = _optional_alias(env, "RAG_JINA_API_KEY", "DEFAULT_JINA_API_KEY")
        provider_profile = ProviderRuntimeProfile(
            profile_id=env.get(
                "RAG_PROVIDER_PROFILE", DISABLED_PROVIDER_PROFILE
            ).strip()
            or DISABLED_PROVIDER_PROFILE,
            accepted_draft_wire_contracts=_boolean(
                env, "RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED", default=False
            ),
            retry_max_attempts=_integer(
                env,
                "RAG_PROVIDER_RETRY_MAX_ATTEMPTS",
                PROVIDER_RETRY_MAX_ATTEMPTS,
                PROVIDER_RETRY_MAX_ATTEMPTS,
                PROVIDER_RETRY_MAX_ATTEMPTS,
            ),
            initial_retry_seconds=_integer(
                env,
                "RAG_PROVIDER_INITIAL_RETRY_SECONDS",
                DEFAULT_PROVIDER_INITIAL_RETRY_SECONDS,
                1,
                3600,
            ),
            max_retry_seconds=_integer(
                env,
                "RAG_PROVIDER_MAX_RETRY_SECONDS",
                DEFAULT_PROVIDER_MAX_RETRY_SECONDS,
                1,
                3600,
            ),
            provider_concurrency=_integer(
                env, "RAG_PROVIDER_CONCURRENCY", DEFAULT_PROVIDER_CONCURRENCY, 1, 16
            ),
            mineru_requests_per_minute=_integer(
                env,
                "RAG_MINERU_REQUESTS_PER_MINUTE",
                DEFAULT_MINERU_REQUESTS_PER_MINUTE,
                1,
                60_000,
            ),
            jina_requests_per_minute=_integer(
                env,
                "RAG_JINA_REQUESTS_PER_MINUTE",
                DEFAULT_JINA_REQUESTS_PER_MINUTE,
                1,
                60_000,
            ),
            jina_embedding_model=env.get(
                "RAG_JINA_EMBEDDING_MODEL", DEFAULT_JINA_EMBEDDING_MODEL
            ).strip()
            or DEFAULT_JINA_EMBEDDING_MODEL,
            jina_rerank_model=env.get(
                "RAG_JINA_RERANK_MODEL", DEFAULT_JINA_RERANK_MODEL
            ).strip()
            or DEFAULT_JINA_RERANK_MODEL,
            jina_embedding_dimensions=DEFAULT_JINA_EMBEDDING_DIMENSIONS,
        )
        if dispatch_enabled and "parse" in job_stages and mineru_api_key is None:
            raise SettingsError(
                "RAG_MINERU_API_TOKEN is required when parse dispatch is enabled"
            )
        if (
            dispatch_enabled
            and "passage_embedding" in job_stages
            and jina_api_key is None
        ):
            raise SettingsError(
                "RAG_JINA_API_KEY is required when embedding dispatch is enabled"
            )

        log_level = env.get("RAG_WORKER_LOG_LEVEL", "INFO").strip().upper()
        if log_level not in {"DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"}:
            raise SettingsError("RAG_WORKER_LOG_LEVEL is invalid")

        health_host = env.get("RAG_WORKER_HEALTH_HOST", "0.0.0.0").strip()
        if not health_host:
            raise SettingsError("RAG_WORKER_HEALTH_HOST must not be empty")
        settings = cls(
            database_url=database_url,
            redis_url=redis_url,
            redis_key_prefix=prefix,
            consumer_name=consumer,
            worker_id=worker_id,
            dispatch_enabled=dispatch_enabled,
            job_stages=job_stages,
            poll_interval_seconds=_integer(env, "RAG_WORKER_POLL_SECONDS", 1, 1, 60),
            forced_rescan_seconds=_integer(
                env, "RAG_WORKER_RESCAN_SECONDS", 30, 5, 3600
            ),
            outbox_lease_seconds=_integer(
                env, "RAG_WORKER_OUTBOX_LEASE_SECONDS", 30, 5, 3600
            ),
            job_lease_seconds=_integer(
                env, "RAG_WORKER_JOB_LEASE_SECONDS", 90, 30, 7200
            ),
            heartbeat_seconds=_integer(
                env, "RAG_WORKER_HEARTBEAT_SECONDS", 30, 5, 1800
            ),
            shutdown_grace_seconds=_integer(
                env, "RAG_WORKER_SHUTDOWN_GRACE_SECONDS", 120, 5, 7200
            ),
            statement_timeout_ms=_integer(
                env, "RAG_WORKER_STATEMENT_TIMEOUT_MS", 10_000, 100, 120_000
            ),
            lock_timeout_ms=_integer(
                env, "RAG_WORKER_LOCK_TIMEOUT_MS", 2_000, 100, 30_000
            ),
            idle_transaction_timeout_ms=_integer(
                env,
                "RAG_WORKER_IDLE_TRANSACTION_TIMEOUT_MS",
                10_000,
                100,
                120_000,
            ),
            health_host=health_host,
            health_port=_integer(env, "RAG_WORKER_HEALTH_PORT", 8081, 1, 65535),
            log_level=log_level,
            max_rescan_claims=_integer(
                env, "RAG_WORKER_MAX_RESCAN_CLAIMS", 100, 1, 10_000
            ),
            mineru_api_key=mineru_api_key,
            jina_api_key=jina_api_key,
            jina_embedding_dimensions=DEFAULT_JINA_EMBEDDING_DIMENSIONS,
            provider_profile=provider_profile,
        )
        if settings.heartbeat_seconds * 2 > settings.job_lease_seconds:
            raise SettingsError(
                "RAG_WORKER_JOB_LEASE_SECONDS must be at least twice heartbeat"
            )
        if settings.forced_rescan_seconds <= settings.poll_interval_seconds:
            raise SettingsError("forced rescan must be slower than normal polling")
        return settings
