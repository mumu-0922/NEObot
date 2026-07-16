from __future__ import annotations

from pathlib import Path

import pytest

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
    MINERU_JINA_POSTGRES_PROFILE,
    PROVIDER_RETRY_MAX_ATTEMPTS,
    ProviderProfileError,
    ProviderRuntimeProfile,
)
from mm_chat_rag.settings import Settings, SettingsError

SOURCE_GATEWAY_TOKEN = "unit-test-source-gateway-token"

def valid_profile() -> ProviderRuntimeProfile:
    return ProviderRuntimeProfile(
        profile_id=MINERU_JINA_POSTGRES_PROFILE,
        accepted_draft_wire_contracts=True,
    )


def test_disabled_provider_profile_is_safe_default() -> None:
    profile = ProviderRuntimeProfile()
    profile.validate_static_contract()
    profile.validate_for_job_stages((), mineru_configured=False, jina_configured=False)
    assert profile.profile_id == DISABLED_PROVIDER_PROFILE
    assert profile.enabled is False
    assert profile.retry_max_attempts == PROVIDER_RETRY_MAX_ATTEMPTS
    assert profile.initial_retry_seconds == DEFAULT_PROVIDER_INITIAL_RETRY_SECONDS
    assert profile.max_retry_seconds == DEFAULT_PROVIDER_MAX_RETRY_SECONDS
    assert profile.provider_concurrency == DEFAULT_PROVIDER_CONCURRENCY
    assert profile.mineru_requests_per_minute == DEFAULT_MINERU_REQUESTS_PER_MINUTE
    assert profile.jina_requests_per_minute == DEFAULT_JINA_REQUESTS_PER_MINUTE
    assert profile.jina_embedding_model == DEFAULT_JINA_EMBEDDING_MODEL
    assert profile.jina_rerank_model == DEFAULT_JINA_RERANK_MODEL
    assert profile.jina_embedding_dimensions == DEFAULT_JINA_EMBEDDING_DIMENSIONS


def test_mineru_jina_postgres_profile_requires_operator_draft_wire_acceptance() -> None:
    profile = ProviderRuntimeProfile(profile_id=MINERU_JINA_POSTGRES_PROFILE)
    with pytest.raises(ProviderProfileError, match="DRAFT_WIRE_ACCEPTED"):
        profile.validate_static_contract()

    valid_profile().validate_for_job_stages(
        ("parse", "passage_embedding"),
        mineru_configured=True,
        jina_configured=True,
    )


@pytest.mark.parametrize(
    ("profile", "match"),
    [
        (
            ProviderRuntimeProfile(
                profile_id="unknown", accepted_draft_wire_contracts=True
            ),
            "unsupported",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_JINA_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                retry_max_attempts=4,
            ),
            "must be 3",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_JINA_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                initial_retry_seconds=600,
                max_retry_seconds=300,
            ),
            "backoff",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_JINA_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                provider_concurrency=17,
            ),
            "concurrency",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_JINA_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                mineru_requests_per_minute=0,
            ),
            "MinerU rate",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_JINA_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                jina_requests_per_minute=0,
            ),
            "Jina rate",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_JINA_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                jina_embedding_model="wrong-model",
            ),
            "embedding model",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_JINA_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                jina_rerank_model="wrong-rerank",
            ),
            "rerank model",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_JINA_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                jina_embedding_dimensions=2048,
            ),
            "1024",
        ),
    ],
)
def test_provider_profile_rejects_unsupported_values(
    profile: ProviderRuntimeProfile, match: str
) -> None:
    with pytest.raises(ProviderProfileError, match=match):
        profile.validate_static_contract()


def test_provider_stage_gate_redacts_secret_values() -> None:
    fake_mineru_secret = "fake-mineru-token"
    fake_jina_secret = "fake-jina-key"
    with pytest.raises(SettingsError, match="RAG_PROVIDER_PROFILE") as missing_profile:
        Settings(
            database_url="postgresql://test",
            dispatch_enabled=True,
            job_stages=("parse", "passage_embedding"),
            mineru_api_key=fake_mineru_secret,
            jina_api_key=fake_jina_secret,
            source_gateway_url="http://backend:8080",
            source_gateway_token=SOURCE_GATEWAY_TOKEN,
        )
    rendered = str(missing_profile.value)
    assert fake_mineru_secret not in rendered
    assert fake_jina_secret not in rendered

    with pytest.raises(SettingsError, match="RAG_JINA_API_KEY") as missing_jina:
        Settings(
            database_url="postgresql://test",
            dispatch_enabled=True,
            job_stages=("passage_embedding",),
            provider_profile=valid_profile(),
        )
    assert fake_jina_secret not in str(missing_jina.value)


def test_provider_profile_env_loader_accepts_locked_defaults() -> None:
    settings = Settings.from_env(
        {
            "RAG_WORKER_DATABASE_URL": "postgresql://worker:secret@db/rag",
            "RAG_WORKER_DISPATCH_ENABLED": "true",
            "RAG_WORKER_JOB_STAGES": "parse,passage_embedding",
            "RAG_MINERU_API_TOKEN": " fake-mineru-token ",
            "RAG_JINA_API_KEY": " fake-jina-key ",
            "RAG_SOURCE_GATEWAY_URL": "http://backend:8080",
            "RAG_SOURCE_GATEWAY_TOKEN": "fake-source-token",
            "RAG_PROVIDER_PROFILE": MINERU_JINA_POSTGRES_PROFILE,
            "RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED": "true",
            "RAG_PROVIDER_RETRY_MAX_ATTEMPTS": "3",
            "RAG_PROVIDER_INITIAL_RETRY_SECONDS": "30",
            "RAG_PROVIDER_MAX_RETRY_SECONDS": "300",
            "RAG_PROVIDER_CONCURRENCY": "2",
            "RAG_MINERU_REQUESTS_PER_MINUTE": "60",
            "RAG_JINA_REQUESTS_PER_MINUTE": "240",
            "RAG_JINA_EMBEDDING_MODEL": DEFAULT_JINA_EMBEDDING_MODEL,
            "RAG_JINA_RERANK_MODEL": DEFAULT_JINA_RERANK_MODEL,
        }
    )

    assert settings.provider_profile.profile_id == MINERU_JINA_POSTGRES_PROFILE
    assert settings.provider_profile.enabled is True
    assert settings.provider_profile.accepted_draft_wire_contracts is True
    assert settings.provider_profile.retry_max_attempts == 3
    assert settings.provider_profile.provider_concurrency == 2
    assert settings.provider_profile.jina_embedding_dimensions == 1024


@pytest.mark.parametrize(
    ("update", "match"),
    [
        ({"RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED": "false"}, "DRAFT_WIRE_ACCEPTED"),
        ({"RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED": "yes"}, "exactly"),
        ({"RAG_PROVIDER_RETRY_MAX_ATTEMPTS": "4"}, "between"),
        ({"RAG_PROVIDER_INITIAL_RETRY_SECONDS": "0"}, "between"),
        ({"RAG_PROVIDER_MAX_RETRY_SECONDS": "0"}, "between"),
        ({"RAG_PROVIDER_CONCURRENCY": "0"}, "between"),
        ({"RAG_MINERU_REQUESTS_PER_MINUTE": "0"}, "between"),
        ({"RAG_JINA_REQUESTS_PER_MINUTE": "0"}, "between"),
        ({"RAG_JINA_EMBEDDING_MODEL": "jina-embeddings-v3"}, "embedding model"),
        ({"RAG_JINA_RERANK_MODEL": "jina-reranker-v2"}, "rerank model"),
    ],
)
def test_provider_profile_env_loader_fails_closed(
    update: dict[str, str], match: str
) -> None:
    env = {
        "RAG_WORKER_DATABASE_URL": "postgresql://worker:secret@db/rag",
        "RAG_PROVIDER_PROFILE": MINERU_JINA_POSTGRES_PROFILE,
        "RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED": "true",
        **update,
    }
    with pytest.raises(SettingsError, match=match):
        Settings.from_env(env)


def test_provider_profile_module_is_config_only() -> None:
    source = Path("src/mm_chat_rag/provider_profile.py").read_text(encoding="utf-8")
    forbidden = ("import httpx", "import requests", "urllib.request", "openai")
    assert not any(item in source for item in forbidden)
