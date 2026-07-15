"""G7 provider-backed RAG profile admission gates.

This module is intentionally config-only: it must not import HTTP clients,
provider SDKs, or parser handlers. G7.3 promotes a named profile gate before any
later slice can attach quota-consuming handlers.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Final

DISABLED_PROVIDER_PROFILE: Final = "disabled"
MINERU_JINA_POSTGRES_PROFILE: Final = "mineru_jina_postgres_v1"
SUPPORTED_PROVIDER_PROFILES: Final[frozenset[str]] = frozenset(
    {DISABLED_PROVIDER_PROFILE, MINERU_JINA_POSTGRES_PROFILE}
)
DEFAULT_JINA_EMBEDDING_DIMENSIONS: Final = 1024
PROVIDER_JOB_STAGES: Final[frozenset[str]] = frozenset({"parse", "passage_embedding"})
PROVIDER_RETRY_MAX_ATTEMPTS: Final = 3
DEFAULT_PROVIDER_INITIAL_RETRY_SECONDS: Final = 30
DEFAULT_PROVIDER_MAX_RETRY_SECONDS: Final = 300
DEFAULT_PROVIDER_CONCURRENCY: Final = 2
DEFAULT_MINERU_REQUESTS_PER_MINUTE: Final = 60
DEFAULT_JINA_REQUESTS_PER_MINUTE: Final = 240
DEFAULT_JINA_EMBEDDING_MODEL: Final = "jina-embeddings-v4"
DEFAULT_JINA_RERANK_MODEL: Final = "jina-reranker-v3"
MAX_PROVIDER_RETRY_SECONDS: Final = 3600
MAX_PROVIDER_CONCURRENCY: Final = 16
MAX_PROVIDER_REQUESTS_PER_MINUTE: Final = 60_000


class ProviderProfileError(ValueError):
    """Raised when a provider runtime profile is incomplete or unsafe."""


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
    jina_requests_per_minute: int = DEFAULT_JINA_REQUESTS_PER_MINUTE
    jina_embedding_model: str = DEFAULT_JINA_EMBEDDING_MODEL
    jina_rerank_model: str = DEFAULT_JINA_RERANK_MODEL
    jina_embedding_dimensions: int = DEFAULT_JINA_EMBEDDING_DIMENSIONS

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
        if not 1 <= self.jina_requests_per_minute <= MAX_PROVIDER_REQUESTS_PER_MINUTE:
            raise ProviderProfileError("RAG Jina rate limit is invalid")
        if self.jina_embedding_model != DEFAULT_JINA_EMBEDDING_MODEL:
            raise ProviderProfileError("RAG Jina embedding model is unsupported")
        if self.jina_rerank_model != DEFAULT_JINA_RERANK_MODEL:
            raise ProviderProfileError("RAG Jina rerank model is unsupported")
        if self.jina_embedding_dimensions != DEFAULT_JINA_EMBEDDING_DIMENSIONS:
            raise ProviderProfileError("RAG Jina embedding dimensions must be 1024")

    def validate_for_job_stages(
        self,
        job_stages: tuple[str, ...],
        *,
        mineru_configured: bool,
        jina_configured: bool,
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
                "RAG_MINERU_API_TOKEN is required for provider profile"
            )
        if "passage_embedding" in enabled_provider_stages and not jina_configured:
            raise ProviderProfileError(
                "RAG_JINA_API_KEY is required for provider profile"
            )
