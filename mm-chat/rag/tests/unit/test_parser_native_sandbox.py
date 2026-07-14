"""C1.3 Native Artifact isolation and MMCP fail-closed integration tests."""

from __future__ import annotations

import json
import socket
import subprocess
import sys
import time
from pathlib import Path

import pytest

from mm_chat_rag.offline_parser import sandbox
from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.decoding import decode_source
from mm_chat_rag.offline_parser.native.dispatch import parse_native_source
from mm_chat_rag.offline_parser.native.internal_result import NativeResultHeader
from mm_chat_rag.offline_parser.native.txt import parse_txt
from mm_chat_rag.offline_parser.protocol import (
    FrameType,
    build_request_header,
    decode_response,
    encode_frame,
)
from mm_chat_rag.offline_parser.sandbox import SandboxSupervisor
from mm_chat_rag.offline_parser.sidecar import ParserSidecar


@pytest.mark.parametrize(
    ("source", "extension", "expected_format"),
    [
        ("café\n中文".encode(), ".txt", ParserFormat.TXT),
        (b"# heading\n\nparagraph\n", ".md", ParserFormat.MARKDOWN),
        (
            b"<!doctype html><html><body><p>x</p></body></html>",
            ".html",
            ParserFormat.HTML,
        ),
    ],
)
def test_real_seccomp_child_returns_a_verified_native_artifact(
    source: bytes,
    extension: str,
    expected_format: ParserFormat,
) -> None:
    result = SandboxSupervisor().route(
        source,
        declared_extension=extension,
    )

    assert result.native_ready
    assert result.parser_format is expected_format
    assert result.stable_error_code is None
    artifact = json.loads(result.native_artifact)
    assert artifact["schemaVersion"] == "parser-native-artifact.v1"
    assert artifact["source"]["format"] == expected_format.value


def test_real_sidecar_keeps_native_artifact_off_mmcp_success_wire() -> None:
    source = b"internal only"
    request = build_request_header(
        invocation_id="native-fail-closed",
        source=source,
        parser_config_hash=DEFAULT_CONFIG.config_hash,
        deadline_unix_millis=int(time.time() * 1000) + 60_000,
        max_result_bytes=4096,
        declared_extension=".txt",
    )
    client, server = socket.socketpair()
    try:
        client.sendall(encode_frame(FrameType.REQUEST, request.to_object(), source))
        ParserSidecar()._handle_connection(server)
        content = client.recv(16_384)
    finally:
        client.close()
    response, body = decode_response(content, invocation_id="native-fail-closed")

    assert response.outcome == "failure"
    assert response.stable_error_code is StableErrorCode.FORMAT_UNSUPPORTED
    assert body == b""


def test_unimplemented_native_format_remains_closed() -> None:
    corpus = Path(__file__).parents[1] / "fixtures" / "parser_corpus"
    source = (corpus / "golden" / "docx" / "minimal.docx").read_bytes()

    outcome = parse_native_source(source, declared_extension=".docx")

    assert outcome.artifact is None
    assert outcome.stable_error_code is StableErrorCode.FORMAT_UNSUPPORTED


def test_supervisor_rejects_self_hashed_non_schema_and_wrong_source_artifacts() -> None:
    invalid_body = b"{}"
    invalid = sandbox._decode_native_result(
        NativeResultHeader.success(ParserFormat.TXT, invalid_body),
        invalid_body,
        source=b"x",
    )
    valid_body = parse_txt(decode_source(b"x")).canonical_bytes
    wrong_source = sandbox._decode_native_result(
        NativeResultHeader.success(ParserFormat.TXT, valid_body),
        valid_body,
        source=b"y",
    )

    assert invalid.stable_error_code is StableErrorCode.PARSER_SCHEMA_MISMATCH
    assert wrong_source.stable_error_code is StableErrorCode.PARSER_SCHEMA_MISMATCH
    assert not invalid.native_ready
    assert not wrong_source.native_ready


def test_sidecar_import_does_not_preload_native_parser_implementations() -> None:
    command = (
        "import sys; "
        "import mm_chat_rag.offline_parser.sidecar; "
        "assert 'markdown_it' not in sys.modules; "
        "assert 'mm_chat_rag.offline_parser.native.markdown' not in sys.modules; "
        "assert 'mm_chat_rag.offline_parser.native.html' not in sys.modules"
    )

    completed = subprocess.run(  # noqa: S603
        [sys.executable, "-I", "-B", "-c", command],
        check=False,
        capture_output=True,
        timeout=10,
    )

    assert completed.returncode == 0, completed.stderr.decode()


def test_real_child_handles_ten_megabyte_ascii_below_native_limits() -> None:
    source = b"a" * 10_000_000

    result = SandboxSupervisor().route(source, declared_extension=".txt")

    assert result.native_ready
    assert result.parser_format is ParserFormat.TXT
    assert len(result.native_artifact) > len(source)
