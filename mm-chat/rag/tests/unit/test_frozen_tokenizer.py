"""Frozen tokenizer corruption and reconstruction boundary tests."""

from __future__ import annotations

import hashlib
from pathlib import Path
from typing import cast

import pytest
import tiktoken

import mm_chat_rag.frozen_tokenizer as tokenizer_module
from mm_chat_rag.frozen_tokenizer import FrozenTokenizer, FrozenTokenizerError


class _MismatchedEncoding:
    def encode_ordinary(self, _: str) -> list[int]:
        return [1]

    def decode_single_token_bytes(self, _: int) -> bytes:
        return b"x"


class _ResourceRoot:
    def __init__(self, artifact: bytes) -> None:
        self._artifact = artifact

    def joinpath(self, _: str) -> _ResourceRoot:
        return self

    def read_bytes(self) -> bytes:
        return self._artifact


def _install_artifact(
    monkeypatch: pytest.MonkeyPatch,
    artifact: bytes,
) -> None:
    monkeypatch.setattr(
        tokenizer_module,
        "files",
        lambda _: cast("Path", _ResourceRoot(artifact)),
    )
    monkeypatch.setattr(
        tokenizer_module,
        "TOKENIZER_ARTIFACT_SHA256",
        hashlib.sha256(artifact).hexdigest(),
    )


def test_frozen_tokenizer_rejects_non_text_and_byte_reconstruction_drift() -> None:
    tokenizer = FrozenTokenizer(
        encoding=cast("tiktoken.Encoding", _MismatchedEncoding()),
        artifact_sha256="a" * 64,
        profile_hash="b" * 64,
        vocabulary_sha256="c" * 64,
    )

    with pytest.raises(TypeError, match="must be text"):
        tokenizer.count(cast("str", 1))
    with pytest.raises(FrozenTokenizerError, match="reconstruction mismatch"):
        tokenizer.token_end_bytes("ab")


def test_frozen_tokenizer_rejects_artifact_hash_drift(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(
        tokenizer_module,
        "files",
        lambda _: cast("Path", _ResourceRoot(b"tampered")),
    )

    with pytest.raises(FrozenTokenizerError, match="artifact hash mismatch"):
        tokenizer_module._load_frozen_tokenizer()


def test_frozen_tokenizer_rejects_malformed_artifact(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _install_artifact(monkeypatch, b"invalid-line")

    with pytest.raises(FrozenTokenizerError, match="artifact is invalid"):
        tokenizer_module._load_frozen_tokenizer()


def test_frozen_tokenizer_rejects_incomplete_vocabulary(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _install_artifact(monkeypatch, b"YQ== 0\n")

    with pytest.raises(FrozenTokenizerError, match="vocabulary is incomplete"):
        tokenizer_module._load_frozen_tokenizer()
