"""Frozen semantic-boundary profile shared by routing and execution."""

from __future__ import annotations

import hashlib
import json
from typing import Final, cast

from mm_chat_rag.provider_profile import (
    DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS,
    DEFAULT_SILICONFLOW_EMBEDDING_MODEL,
    GenerationEmbeddingProfile,
    ProviderProfileError,
)

SEMANTIC_MIN_SOURCE_TOKENS: Final = 1200
SEMANTIC_EMBEDDING_DIMENSIONS: Final = DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS
SEMANTIC_MIN_SENTENCES: Final = 4
SEMANTIC_MAX_SENTENCES: Final = 4096
SEMANTIC_MAX_SENTENCE_BYTES: Final = 64 * 1024
SEMANTIC_EMBED_BATCH_SIZE: Final = 256
SEMANTIC_CACHE_MAX_ENTRIES: Final = 1024
SEMANTIC_BREAK_PERCENTILE: Final = 0.85
SEMANTIC_MIN_DISTANCE: Final = 0.15
SEMANTIC_MAX_BOUNDARIES: Final = 128


def _profile(model_id: str, dimensions: int) -> dict[str, object]:
    return {
        "breakPercentile": SEMANTIC_BREAK_PERCENTILE,
        "embeddingDimensions": dimensions,
        "embeddingModel": model_id,
        "maximumBoundaries": SEMANTIC_MAX_BOUNDARIES,
        "maximumSentenceBytes": SEMANTIC_MAX_SENTENCE_BYTES,
        "maximumSentences": SEMANTIC_MAX_SENTENCES,
        "minimumDistance": SEMANTIC_MIN_DISTANCE,
        "minimumSentences": SEMANTIC_MIN_SENTENCES,
        "minimumSourceTokens": SEMANTIC_MIN_SOURCE_TOKENS,
        "schemaVersion": "mm-chat.semantic-boundary-profile.v1",
    }


def _profile_hash(profile: dict[str, object]) -> str:
    return hashlib.sha256(
        json.dumps(profile, separators=(",", ":"), sort_keys=True).encode()
    ).hexdigest()


_PROFILE: Final = _profile(
    DEFAULT_SILICONFLOW_EMBEDDING_MODEL,
    DEFAULT_SILICONFLOW_EMBEDDING_DIMENSIONS,
)
SEMANTIC_BOUNDARY_PROFILE_HASH: Final = _profile_hash(_PROFILE)
SILICONFLOW_SEMANTIC_BOUNDARY_PROFILE_HASH: Final = SEMANTIC_BOUNDARY_PROFILE_HASH
SUPPORTED_SEMANTIC_BOUNDARY_PROFILE_HASHES: Final = frozenset(
    {
        SEMANTIC_BOUNDARY_PROFILE_HASH,
    }
)


def semantic_boundary_profile() -> dict[str, object]:
    """Return a detached diagnostic view of the frozen semantic profile."""
    return cast("dict[str, object]", json.loads(json.dumps(_PROFILE)))


def semantic_boundary_profile_hash(
    embedding_profile: GenerationEmbeddingProfile | None,
) -> str:
    """Bind semantic cache and hints to the exact generation vector space."""
    if embedding_profile is None:
        return SEMANTIC_BOUNDARY_PROFILE_HASH
    try:
        embedding_profile.validate()
    except ProviderProfileError as error:
        raise ValueError("semantic embedding profile is unsupported") from error
    return SEMANTIC_BOUNDARY_PROFILE_HASH


def siliconflow_semantic_boundary_profile() -> dict[str, object]:
    """Return the frozen Pro BGE semantic-boundary profile descriptor."""
    return cast("dict[str, object]", json.loads(json.dumps(_PROFILE)))
