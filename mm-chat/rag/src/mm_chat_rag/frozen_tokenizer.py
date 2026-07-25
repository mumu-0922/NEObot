"""Offline, hash-bound tokenizer authority for persisted RAG chunks."""

from __future__ import annotations

import base64
import hashlib
import json
from dataclasses import dataclass
from importlib.resources import files
from typing import Final

import tiktoken

TOKENIZER_NAME: Final = "cl100k_base"
TOKENIZER_REVISION: Final = "openai-public-2022-12-14"
TOKENIZER_ARTIFACT_SHA256: Final = (
    "223921b76ee99bde995b7ff738513eef100fb51d18c93597a113bcffe865b2a7"
)
TOKENIZER_NORMALIZATION: Final = "none"
TOKENIZER_ORDINARY_TEXT_POLICY: Final = "encode_ordinary"

_TOKENIZER_RESOURCE: Final = "resources/tokenizers/cl100k_base.tiktoken"
_VOCABULARY_SIZE: Final = 100256
_PATTERN: Final = (
    r"'(?i:[sdmt]|ll|ve|re)|[^\r\n\p{L}\p{N}]?+\p{L}++|"
    r"\p{N}{1,3}+| ?[^\s\p{L}\p{N}]++[\r\n]*+|\s++$|"
    r"\s*[\r\n]|\s+(?!\S)|\s"
)
_SPECIAL_TOKENS: Final = {
    "<|endoftext|>": 100257,
    "<|fim_prefix|>": 100258,
    "<|fim_middle|>": 100259,
    "<|fim_suffix|>": 100260,
    "<|endofprompt|>": 100276,
}


class FrozenTokenizerError(RuntimeError):
    """The packaged tokenizer authority is missing, corrupt, or invalid."""


@dataclass(frozen=True, slots=True)
class FrozenTokenizer:
    """Read-only tokenizer plus the identity persisted in Chunk Profiles."""

    encoding: tiktoken.Encoding
    artifact_sha256: str
    profile_hash: str
    vocabulary_sha256: str

    def count(self, text: str) -> int:
        """Count ordinary source tokens without recognizing special strings."""
        if not isinstance(text, str):
            raise TypeError("tokenizer input must be text")
        return len(self.encoding.encode_ordinary(text))

    def token_end_bytes(self, text: str) -> tuple[int, ...]:
        """Return cumulative byte ends for one exact encoding of ``text``."""
        total = 0
        ends: list[int] = []
        for token in self.encoding.encode_ordinary(text):
            total += len(self.encoding.decode_single_token_bytes(token))
            ends.append(total)
        if total != len(text.encode("utf-8")):
            raise FrozenTokenizerError("tokenizer byte reconstruction mismatch")
        return tuple(ends)


def _load_frozen_tokenizer() -> FrozenTokenizer:
    artifact = files("mm_chat_rag").joinpath(_TOKENIZER_RESOURCE).read_bytes()
    artifact_sha256 = hashlib.sha256(artifact).hexdigest()
    if artifact_sha256 != TOKENIZER_ARTIFACT_SHA256:
        raise FrozenTokenizerError("packaged tokenizer artifact hash mismatch")

    mergeable_ranks: dict[bytes, int] = {}
    try:
        for line in artifact.splitlines():
            encoded_token, encoded_rank = line.split()
            mergeable_ranks[base64.b64decode(encoded_token, validate=True)] = int(
                encoded_rank
            )
    except (TypeError, ValueError) as error:
        raise FrozenTokenizerError("packaged tokenizer artifact is invalid") from error
    if len(mergeable_ranks) != _VOCABULARY_SIZE:
        raise FrozenTokenizerError("packaged tokenizer vocabulary is incomplete")

    vocabulary_digest = hashlib.sha256()
    for token, rank in sorted(mergeable_ranks.items(), key=lambda item: item[1]):
        vocabulary_digest.update(len(token).to_bytes(4, "big"))
        vocabulary_digest.update(token)
        vocabulary_digest.update(rank.to_bytes(4, "big"))
    vocabulary_sha256 = vocabulary_digest.hexdigest()
    profile = {
        "artifactSha256": artifact_sha256,
        "name": TOKENIZER_NAME,
        "normalization": TOKENIZER_NORMALIZATION,
        "revision": TOKENIZER_REVISION,
        "specialTokenPolicy": TOKENIZER_ORDINARY_TEXT_POLICY,
        "vocabularySha256": vocabulary_sha256,
    }
    profile_hash = hashlib.sha256(
        json.dumps(
            profile,
            ensure_ascii=True,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("utf-8")
    ).hexdigest()
    encoding = tiktoken.Encoding(
        name=f"mm_chat_{TOKENIZER_NAME}_{TOKENIZER_REVISION}",
        pat_str=_PATTERN,
        mergeable_ranks=mergeable_ranks,
        special_tokens=_SPECIAL_TOKENS,
    )
    return FrozenTokenizer(
        encoding=encoding,
        artifact_sha256=artifact_sha256,
        profile_hash=profile_hash,
        vocabulary_sha256=vocabulary_sha256,
    )


FROZEN_TOKENIZER: Final = _load_frozen_tokenizer()
