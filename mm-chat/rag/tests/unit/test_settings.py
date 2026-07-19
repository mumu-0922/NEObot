from __future__ import annotations

import uuid

import pytest

from mm_chat_rag.provider_profile import (
    DEFAULT_JINA_EMBEDDING_DIMENSIONS,
    MINERU_JINA_POSTGRES_PROFILE,
)
from mm_chat_rag.settings import (
    ALLOWED_JOB_STAGES,
    Settings,
    SettingsError,
)


def base_env() -> dict[str, str]:
    return {"RAG_WORKER_DATABASE_URL": "postgresql://worker:secret@db/rag"}


def test_safe_dark_run_defaults() -> None:
    settings = Settings.from_env(base_env())
    assert settings.dispatch_enabled is False
    assert settings.job_stages == ()
    assert settings.poll_interval_seconds == 1
    assert settings.forced_rescan_seconds == 30
    assert settings.outbox_lease_seconds == 30
    assert settings.job_lease_seconds == 90
    assert settings.heartbeat_seconds == 30
    assert settings.redis_channel == "mm-chat:rag:outbox:v1"
    assert settings.source_gateway_url is None
    assert settings.source_gateway_token is None
    assert settings.jina_embedding_dimensions == DEFAULT_JINA_EMBEDDING_DIMENSIONS
    assert settings.provider_profile.enabled is False


def test_explicit_enabled_configuration() -> None:
    worker_id = uuid.uuid4()
    env = {
        **base_env(),
        "RAG_WORKER_DISPATCH_ENABLED": "true",
        "RAG_WORKER_JOB_STAGES": "parse,passage_embedding,purge",
        "RAG_WORKER_ID": str(worker_id),
        "RAG_WORKER_REDIS_URL": "redis://redis/0",
        "REDIS_KEY_PREFIX": "test:v2",
        "RAG_WORKER_LOG_LEVEL": "warning",
        "RAG_MINERU_RESULT_PROXY_URL": (
            " http://host.docker.internal:18081/mineru-result "
        ),
        "RAG_SOURCE_GATEWAY_URL": " http://backend:8080 ",
        "RAG_SOURCE_GATEWAY_TOKEN": " fake-source-token ",
        "RAG_PROVIDER_PROFILE": MINERU_JINA_POSTGRES_PROFILE,
        "RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED": "true",
    }
    settings = Settings.from_env(env)
    assert settings.worker_id == worker_id
    assert settings.job_stages == ("parse", "passage_embedding", "purge")
    assert settings.redis_channel == "test:v2:rag:outbox:v1"
    assert settings.log_level == "WARNING"
    assert (
        settings.mineru_result_proxy_url
        == "http://host.docker.internal:18081/mineru-result"
    )
    assert settings.source_gateway_url == "http://backend:8080"
    assert settings.source_gateway_token == "fake-source-token"
    assert settings.jina_embedding_dimensions == DEFAULT_JINA_EMBEDDING_DIMENSIONS
    assert settings.provider_profile.profile_id == MINERU_JINA_POSTGRES_PROFILE
    assert settings.provider_profile.accepted_draft_wire_contracts is True


def test_dispatch_parse_requires_provider_gateway() -> None:
    env = {
        **base_env(),
        "RAG_WORKER_DISPATCH_ENABLED": "true",
        "RAG_WORKER_JOB_STAGES": "parse",
    }
    with pytest.raises(SettingsError, match="RAG_SOURCE_GATEWAY_URL"):
        Settings.from_env(env)


def test_dispatch_parse_requires_source_gateway_url_and_token() -> None:
    env = {
        **base_env(),
        "RAG_WORKER_DISPATCH_ENABLED": "true",
        "RAG_WORKER_JOB_STAGES": "parse",
    }
    with pytest.raises(SettingsError, match="RAG_SOURCE_GATEWAY_URL"):
        Settings.from_env(env)

    with pytest.raises(SettingsError, match="RAG_SOURCE_GATEWAY_TOKEN"):
        Settings.from_env({**env, "RAG_SOURCE_GATEWAY_URL": "http://backend:8080"})


def test_dispatch_embedding_requires_provider_gateway() -> None:
    env = {
        **base_env(),
        "RAG_WORKER_DISPATCH_ENABLED": "true",
        "RAG_WORKER_JOB_STAGES": "passage_embedding",
    }
    with pytest.raises(SettingsError, match="RAG_SOURCE_GATEWAY_URL"):
        Settings.from_env(env)


def test_dispatch_purge_does_not_require_provider_secrets() -> None:
    settings = Settings.from_env(
        {
            **base_env(),
            "RAG_WORKER_DISPATCH_ENABLED": "true",
            "RAG_WORKER_JOB_STAGES": "purge",
        }
    )
    assert settings.job_stages == ("purge",)
    assert settings.source_gateway_url is None
    assert settings.source_gateway_token is None


def test_provider_key_environment_names_are_not_parsed() -> None:
    settings = Settings.from_env(
        {
            **base_env(),
            "RAG_WORKER_DISPATCH_ENABLED": "true",
            "RAG_WORKER_JOB_STAGES": "parse,passage_embedding",
            "RAG_MINERU_API_TOKEN": "must-not-enter-settings",
            "RAG_JINA_API_KEY": "must-not-enter-settings",
            "DEFAULT_MINERU_API_TOKEN": "must-not-enter-settings",
            "DEFAULT_JINA_API_KEY": "must-not-enter-settings",
            "RAG_SOURCE_GATEWAY_URL": "http://backend:8080",
            "RAG_SOURCE_GATEWAY_TOKEN": "provider-gateway-token",
            "RAG_PROVIDER_PROFILE": MINERU_JINA_POSTGRES_PROFILE,
            "RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED": "true",
        }
    )
    assert not hasattr(settings, "mineru_api_key")
    assert not hasattr(settings, "jina_api_key")
    assert settings.source_gateway_url == "http://backend:8080"
    assert settings.source_gateway_token == "provider-gateway-token"


@pytest.mark.parametrize(
    ("update", "match"),
    [
        ({"RAG_WORKER_DATABASE_URL": ""}, "required"),
        ({"RAG_WORKER_DISPATCH_ENABLED": "1"}, "exactly"),
        ({"RAG_WORKER_JOB_STAGES": "parse"}, "must be empty"),
        (
            {
                "RAG_WORKER_DISPATCH_ENABLED": "true",
                "RAG_WORKER_JOB_STAGES": "parse,parse",
            },
            "duplicates",
        ),
        (
            {
                "RAG_WORKER_DISPATCH_ENABLED": "true",
                "RAG_WORKER_JOB_STAGES": "Bad Stage",
            },
            "unknown stage",
        ),
        (
            {
                "RAG_WORKER_DISPATCH_ENABLED": "true",
                "RAG_WORKER_JOB_STAGES": "synthetic",
            },
            "unknown stage",
        ),
        ({"REDIS_KEY_PREFIX": "bad prefix"}, "PREFIX"),
        ({"RAG_WORKER_CONSUMER": "UPPER"}, "CONSUMER"),
        ({"RAG_WORKER_ID": "nope"}, "UUID"),
        ({"RAG_WORKER_LOG_LEVEL": "TRACE"}, "LOG_LEVEL"),
        ({"RAG_WORKER_DATABASE_URL": "https://db/rag"}, "service URL"),
        ({"RAG_WORKER_REDIS_URL": "http://redis"}, "service URL"),
        ({"RAG_SOURCE_GATEWAY_URL": "ftp://backend"}, "service URL"),
        ({"RAG_MINERU_RESULT_PROXY_URL": "ftp://host/proxy"}, "service URL"),
        ({"RAG_SOURCE_GATEWAY_TOKEN": "bad token"}, "invalid"),
        ({"RAG_WORKER_HEALTH_HOST": ""}, "HEALTH_HOST"),
        ({"RAG_WORKER_TYPO": "true"}, "unknown worker setting"),
        ({"RAG_WORKER_POLL_SECONDS": "0"}, "between"),
        ({"RAG_WORKER_POLL_SECONDS": "wat"}, "integer"),
        (
            {
                "RAG_WORKER_DISPATCH_ENABLED": "true",
                "RAG_WORKER_JOB_LEASE_SECONDS": "30",
                "RAG_WORKER_HEARTBEAT_SECONDS": "20",
            },
            "twice heartbeat",
        ),
        ({"RAG_WORKER_RESCAN_SECONDS": "1"}, "RESCAN"),
    ],
)
def test_invalid_environment_fails_closed(update: dict[str, str], match: str) -> None:
    with pytest.raises(SettingsError, match=match):
        Settings.from_env({**base_env(), **update})


def test_job_stage_allowlist_matches_sql_contract() -> None:
    expected = frozenset({"parse", "passage_embedding", "purge"})
    assert expected == ALLOWED_JOB_STAGES
    with pytest.raises(SettingsError, match="unknown stage"):
        Settings(
            database_url="postgresql://test",
            dispatch_enabled=True,
            job_stages=("synthetic",),
        )
