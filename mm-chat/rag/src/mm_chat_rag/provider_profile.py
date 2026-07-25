"""G7 provider-backed RAG profile admission gates.

This module is intentionally config-only: it must not import HTTP clients,
provider SDKs, or parser handlers. G7.3 promotes a named profile gate before any
later slice can attach quota-consuming handlers.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Final

DISABLED_PROVIDER_PROFILE: Final = "disabled"
MINERU_SILICONFLOW_POSTGRES_PROFILE: Final = "mineru_siliconflow_postgres_v1"
SUPPORTED_PROVIDER_PROFILES: Final[frozenset[str]] = frozenset(
    {
        DISABLED_PROVIDER_PROFILE,
        MINERU_SILICONFLOW_POSTGRES_PROFILE,
    }
)
PROVIDER_JOB_STAGES: Final[frozenset[str]] = frozenset({"parse", "passage_embedding"})
PROVIDER_RETRY_MAX_ATTEMPTS: Final = 3
DEFAULT_PROVIDER_INITIAL_RETRY_SECONDS: Final = 30
DEFAULT_PROVIDER_MAX_RETRY_SECONDS: Final = 300
DEFAULT_PROVIDER_CONCURRENCY: Final = 2
DEFAULT_MINERU_REQUESTS_PER_MINUTE: Final = 60
DEFAULT_SILICONFLOW_EMBEDDING_MODEL: Final = "Pro/BAAI/bge-m3"
DEFAULT_SILICONFLOW_RERANK_MODEL: Final = "Pro/BAAI/bge-reranker-v2-m3"
DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS: Final = 1024
DEFAULT_SILICONFLOW_REQUESTS_PER_MINUTE: Final = 240
MAX_PROVIDER_RETRY_SECONDS: Final = 3600
MAX_PROVIDER_CONCURRENCY: Final = 16
MAX_PROVIDER_REQUESTS_PER_MINUTE: Final = 60_000


class ProviderProfileError(ValueError):
    """Raised when a provider runtime profile is incomplete or unsafe."""


@dataclass(frozen=True, slots=True)
class GenerationEmbeddingProfile:
    """Generation-bound embedding vector-space identity."""

    processor: str
    model_id: str
    dimensions: int

    def validate(self) -> None:
        if (
            self.processor == "siliconflow"
            and self.model_id == DEFAULT_SILICONFLOW_EMBEDDING_MODEL
            and self.dimensions == DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS
        ):
            return
        raise ProviderProfileError("RAG generation embedding profile is unsupported")


@dataclass(frozen=True, slots=True)
class ProviderRuntimeProfile:
    """A redacted provider profile contract selected by administrator config."""

    profile_id: str = DISABLED_PROVIDER_PROFILE
    accepted_draft_wire_contracts: bool = False
    retry_max_attempts: int = PROVIDER_RETRY_MAX_ATTEMPTS
    initial_retry_seconds: int = DEFAULT_PROVIDER_INITIAL_RETRY_SECONDS
    max_retry_seconds: int = DEFAULT_PROVIDER_MAX_RETRY_SECONDS
    provider_concurrency: int = DEFAULT_PROVIDER_CONCURRENCY
    mineru_requests_per_minute: int = DEFAULT_MINERU_REQUESTS_PER_MINUTE
    siliconflow_requests_per_minute: int = DEFAULT_SILICONFLOW_REQUESTS_PER_MINUTE
    siliconflow_embedding_model: str = DEFAULT_SILICONFLOW_EMBEDDING_MODEL
    siliconflow_rerank_model: str = DEFAULT_SILICONFLOW_RERANK_MODEL
    siliconflow_embedding_dimensions: int = DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS

    @property
    def enabled(self) -> bool:
        return self.profile_id != DISABLED_PROVIDER_PROFILE

    def validate_static_contract(self) -> None:
        """Reject unsupported or unsafe static profile values."""
        if self.profile_id not in SUPPORTED_PROVIDER_PROFILES:
            raise ProviderProfileError("RAG_PROVIDER_PROFILE is unsupported")
        if self.profile_id == DISABLED_PROVIDER_PROFILE:
            return
        if not self.accepted_draft_wire_contracts:
            raise ProviderProfileError(
                "RAG_PROVIDER_PROFILE_DRAFT_WIRE_ACCEPTED is required"
            )
        if self.retry_max_attempts != PROVIDER_RETRY_MAX_ATTEMPTS:
            raise ProviderProfileError("RAG_PROVIDER_RETRY_MAX_ATTEMPTS must be 3")
        if (
            not 1
            <= self.initial_retry_seconds
            <= self.max_retry_seconds
            <= MAX_PROVIDER_RETRY_SECONDS
        ):
            raise ProviderProfileError("RAG provider retry backoff is invalid")
        if not 1 <= self.provider_concurrency <= MAX_PROVIDER_CONCURRENCY:
            raise ProviderProfileError("RAG provider concurrency is invalid")
        if not 1 <= self.mineru_requests_per_minute <= MAX_PROVIDER_REQUESTS_PER_MINUTE:
            raise ProviderProfileError("RAG MinerU rate limit is invalid")
        if not (
            1
            <= self.siliconflow_requests_per_minute
            <= MAX_PROVIDER_REQUESTS_PER_MINUTE
        ):
            raise ProviderProfileError("RAG SiliconFlow rate limit is invalid")
        if self.siliconflow_embedding_model != DEFAULT_SILICONFLOW_EMBEDDING_MODEL:
            raise ProviderProfileError("RAG SiliconFlow embedding model is unsupported")
        if self.siliconflow_rerank_model != DEFAULT_SILICONFLOW_RERANK_MODEL:
            raise ProviderProfileError("RAG SiliconFlow rerank model is unsupported")
        if (
            self.siliconflow_embedding_dimensions
            != DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS
        ):
            raise ProviderProfileError(
                "RAG SiliconFlow embedding dimensions must be 1024"
            )

    def validate_for_job_stages(
        self,
        job_stages: tuple[str, ...],
        *,
        mineru_configured: bool,
        siliconflow_configured: bool,
    ) -> None:
        """Fail closed before provider-backed job handlers can be promoted."""
        enabled_provider_stages = set(job_stages) & PROVIDER_JOB_STAGES
        if not enabled_provider_stages:
            return
        self.validate_static_contract()
        if not self.enabled:
            raise ProviderProfileError(
                "RAG_PROVIDER_PROFILE is required for provider stages"
            )
        if "parse" in enabled_provider_stages and not mineru_configured:
            raise ProviderProfileError(
                "MinerU provider gateway is required for provider profile"
            )
        if (
            self.profile_id == MINERU_SILICONFLOW_POSTGRES_PROFILE
            and "passage_embedding" in enabled_provider_stages
            and not siliconflow_configured
        ):
            raise ProviderProfileError(
                "SiliconFlow provider gateway is required for provider profile"
            )

    def admits_embedding_profile(self, profile: GenerationEmbeddingProfile) -> bool:
        """Admit only vector spaces explicitly covered by the worker profile."""
        try:
            profile.validate()
        except ProviderProfileError:
            return False
        return self.profile_id == MINERU_SILICONFLOW_POSTGRES_PROFILE
