from __future__ import annotations

from pathlib import Path

import pytest

from mm_chat_rag.provider_profile import (
    DEFAULT_MINERU_REQUESTS_PER_MINUTE,
    DEFAULT_PROVIDER_CONCURRENCY,
    DEFAULT_PROVIDER_INITIAL_RETRY_SECONDS,
    DEFAULT_PROVIDER_MAX_RETRY_SECONDS,
    DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS,
    DEFAULT_SILICONFLOW_EMBEDDING_MODEL,
    DEFAULT_SILICONFLOW_REQUESTS_PER_MINUTE,
    DEFAULT_SILICONFLOW_RERANK_MODEL,
    DISABLED_PROVIDER_PROFILE,
    MINERU_SILICONFLOW_POSTGRES_PROFILE,
    PROVIDER_RETRY_MAX_ATTEMPTS,
    GenerationEmbeddingProfile,
    ProviderProfileError,
    ProviderRuntimeProfile,
)
from mm_chat_rag.settings import Settings, SettingsError

SOURCE_GATEWAY_TOKEN = "unit-test-source-gateway-token"


def valid_profile() -> ProviderRuntimeProfile:
    return ProviderRuntimeProfile(
        profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
        accepted_draft_wire_contracts=True,
    )


def test_disabled_provider_profile_is_safe_default() -> None:
    profile = ProviderRuntimeProfile()
    profile.validate_static_contract()
    profile.validate_for_job_stages(
        (), mineru_configured=False, siliconflow_configured=False
    )
    assert profile.profile_id == DISABLED_PROVIDER_PROFILE
    assert profile.enabled is False
    assert profile.retry_max_attempts == PROVIDER_RETRY_MAX_ATTEMPTS
    assert profile.initial_retry_seconds == DEFAULT_PROVIDER_INITIAL_RETRY_SECONDS
    assert profile.max_retry_seconds == DEFAULT_PROVIDER_MAX_RETRY_SECONDS
    assert profile.provider_concurrency == DEFAULT_PROVIDER_CONCURRENCY
    assert profile.mineru_requests_per_minute == DEFAULT_MINERU_REQUESTS_PER_MINUTE
    assert (
        profile.siliconflow_requests_per_minute
        == DEFAULT_SILICONFLOW_REQUESTS_PER_MINUTE
    )
    assert profile.siliconflow_embedding_model == DEFAULT_SILICONFLOW_EMBEDDING_MODEL
    assert profile.siliconflow_rerank_model == DEFAULT_SILICONFLOW_RERANK_MODEL
    assert (
        profile.siliconflow_embedding_dimensions
        == DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS
    )


def test_mineru_siliconflow_profile_requires_operator_draft_wire_acceptance() -> None:
    profile = ProviderRuntimeProfile(profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE)
    with pytest.raises(ProviderProfileError, match="DRAFT_WIRE_ACCEPTED"):
        profile.validate_static_contract()

    valid_profile().validate_for_job_stages(
        ("parse", "passage_embedding"),
        mineru_configured=True,
        siliconflow_configured=True,
    )


def test_hybrid_worker_profile_admits_only_pinned_siliconflow_bge_space() -> None:
    profile = ProviderRuntimeProfile(
        profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
        accepted_draft_wire_contracts=True,
    )
    profile.validate_for_job_stages(
        ("parse", "passage_embedding"),
        mineru_configured=True,
        siliconflow_configured=True,
    )
    assert profile.admits_embedding_profile(
        GenerationEmbeddingProfile(
            processor="siliconflow",
            model_id=DEFAULT_SILICONFLOW_EMBEDDING_MODEL,
            dimensions=DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS,
        )
    )
    assert profile.siliconflow_rerank_model == DEFAULT_SILICONFLOW_RERANK_MODEL
    assert not profile.admits_embedding_profile(
        GenerationEmbeddingProfile(
            processor="jina",
            model_id="jina-embeddings-v4",
            dimensions=DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS,
        )
    )


def test_hybrid_worker_profile_requires_siliconflow_gateway() -> None:
    profile = ProviderRuntimeProfile(
        profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
        accepted_draft_wire_contracts=True,
    )
    with pytest.raises(ProviderProfileError, match="SiliconFlow"):
        profile.validate_for_job_stages(
            ("passage_embedding",),
            mineru_configured=True,
            siliconflow_configured=False,
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
                profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                retry_max_attempts=4,
            ),
            "must be 3",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                initial_retry_seconds=600,
                max_retry_seconds=300,
            ),
            "backoff",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                provider_concurrency=17,
            ),
            "concurrency",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                mineru_requests_per_minute=0,
            ),
            "MinerU rate",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                siliconflow_requests_per_minute=0,
            ),
            "SiliconFlow rate",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                siliconflow_embedding_model="wrong-model",
            ),
            "embedding model",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                siliconflow_rerank_model="wrong-rerank",
            ),
            "rerank model",
        ),
        (
            ProviderRuntimeProfile(
                profile_id=MINERU_SILICONFLOW_POSTGRES_PROFILE,
                accepted_draft_wire_contracts=True,
                siliconflow_embedding_dimensions=2048,
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


def test_provider_stage_gate_requires_profile_and_go_gateway() -> None:
    with pytest.raises(SettingsError, match="RAG_PROVIDER_PROFILE") as missing_profile:
        Settings(
            database_url="postgresql://test",
            dispatch_enabled=True,
            job_stages=("parse", "passage_embedding"),
            source_gateway_url="http://backend:8080",
            source_gateway_token=SOURCE_GATEWAY_TOKEN,
        )
    rendered = str(missing_profile.value)
    assert SOURCE_GATEWAY_TOKEN not in rendered

    with pytest.raises(
        SettingsError, match="RAG_SOURCE_GATEWAY_URL"
    ) as missing_gateway:
        Settings(
            database_url="postgresql://test",
            dispatch_enabled=True,
            job_stages=("passage_embedding",),
            provider_profile=valid_profile(),
        )
    assert SOURCE_GATEWAY_TOKEN not in str(missing_gateway.value)


def test_provider_profile_env_loader_accepts_locked_defaults() -> None:
    settings = Settings.from_env(
        {
            "RAG_WORKER_DATABASE_URL": "postgresql://worker:secret@db/rag",
            "RAG_WORKER_DISPATCH_ENABLED": "true",
            "RAG_WORKER_JOB_STAGES": "parse,passage_embedding",
            "RAG_SOURCE_GATEWAY_URL": "http://backend:8080",
            "RAG_SOURCE_GATEWAY_TOKEN": "fake-source-token",
            "RAG_PROVIDER_PROFILE": MINERU_SILICONFLOW_POSTGRES_PROFILE,
            "RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED": "true",
            "RAG_PROVIDER_RETRY_MAX_ATTEMPTS": "3",
            "RAG_PROVIDER_INITIAL_RETRY_SECONDS": "30",
            "RAG_PROVIDER_MAX_RETRY_SECONDS": "300",
            "RAG_PROVIDER_CONCURRENCY": "2",
            "RAG_MINERU_REQUESTS_PER_MINUTE": "60",
            "RAG_SILICONFLOW_REQUESTS_PER_MINUTE": "240",
            "RAG_SILICONFLOW_EMBEDDING_MODEL": DEFAULT_SILICONFLOW_EMBEDDING_MODEL,
            "RAG_SILICONFLOW_RERANK_MODEL": DEFAULT_SILICONFLOW_RERANK_MODEL,
        }
    )

    assert settings.provider_profile.profile_id == MINERU_SILICONFLOW_POSTGRES_PROFILE
    assert settings.provider_profile.enabled is True
    assert settings.provider_profile.accepted_draft_wire_contracts is True
    assert settings.provider_profile.retry_max_attempts == 3
    assert settings.provider_profile.provider_concurrency == 2
    assert settings.provider_profile.siliconflow_embedding_dimensions == 1024


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
        ({"RAG_SILICONFLOW_REQUESTS_PER_MINUTE": "0"}, "between"),
        ({"RAG_SILICONFLOW_EMBEDDING_MODEL": "wrong-model"}, "embedding model"),
        ({"RAG_SILICONFLOW_RERANK_MODEL": "wrong-rerank"}, "rerank model"),
    ],
)
def test_provider_profile_env_loader_fails_closed(
    update: dict[str, str], match: str
) -> None:
    env = {
        "RAG_WORKER_DATABASE_URL": "postgresql://worker:secret@db/rag",
        "RAG_PROVIDER_PROFILE": MINERU_SILICONFLOW_POSTGRES_PROFILE,
        "RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED": "true",
        **update,
    }
    with pytest.raises(SettingsError, match=match):
        Settings.from_env(env)


def test_provider_profile_module_is_config_only() -> None:
    source = Path("src/mm_chat_rag/provider_profile.py").read_text(encoding="utf-8")
    forbidden = ("import httpx", "import requests", "urllib.request", "openai")
    assert not any(item in source for item in forbidden)
