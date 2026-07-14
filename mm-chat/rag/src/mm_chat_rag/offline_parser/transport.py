"""Bounded Unix-socket MMCP controller transport and synthesized outcomes."""

from __future__ import annotations

import socket
import stat
import struct
import time
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import Final

from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG, ParserHarnessConfig
from mm_chat_rag.offline_parser.errors import StableErrorCode
from mm_chat_rag.offline_parser.protocol import (
    FrameType,
    ProtocolError,
    ResponseHeader,
    build_request_header,
    decode_response,
    encode_frame,
)

DEFAULT_SOCKET_PATH: Final = Path("/run/mm-chat-parser-ipc/parser.sock")
_PREFIX: Final = struct.Struct(">4sBBHIQ")
_READ_POLL_SECONDS: Final = 0.05


@dataclass(frozen=True, slots=True)
class ControllerOutcome:
    """Validated wire response or a controller-synthesized local failure."""

    response: ResponseHeader | None
    body: bytes
    local_error_code: StableErrorCode | None

    @property
    def stageable(self) -> bool:
        """Only a validated non-empty Canonical IR success can be staged."""
        return (
            self.response is not None
            and self.response.outcome == "success"
            and bool(self.body)
            and self.local_error_code is None
        )


class ParserController:
    """Connect to a private UDS and independently verify every MMCP field."""

    def __init__(
        self,
        socket_path: Path = DEFAULT_SOCKET_PATH,
        *,
        config: ParserHarnessConfig = DEFAULT_CONFIG,
    ) -> None:
        self._socket_path = socket_path
        self._config = config

    def invoke(  # noqa: PLR0911
        self,
        source: bytes,
        *,
        invocation_id: str,
        declared_mime: str | None = None,
        declared_extension: str | None = None,
        cancelled: Callable[[], bool] | None = None,
    ) -> ControllerOutcome:
        """Send one request; cancellation always wins over transport failure."""
        if cancelled is not None and cancelled():
            return _local_failure(StableErrorCode.PARSER_CANCELLED)
        now = int(time.time() * 1000)
        deadline_millis = now + self._config.sandbox.wall_timeout_millis
        try:
            _validate_socket_path(self._socket_path)
            header = build_request_header(
                invocation_id=invocation_id,
                source=source,
                parser_config_hash=self._config.config_hash,
                deadline_unix_millis=deadline_millis,
                max_result_bytes=self._config.max_result_bytes,
                declared_mime=declared_mime,
                declared_extension=declared_extension,
            )
            frame = encode_frame(FrameType.REQUEST, header.to_object(), source)
        except (OSError, ProtocolError):
            return _local_failure(StableErrorCode.PARSER_SANDBOX_UNAVAILABLE)

        connection = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        deadline = time.monotonic() + self._config.sandbox.wall_timeout_millis / 1000
        try:
            connection.settimeout(max(0.1, deadline - time.monotonic()))
            connection.connect(str(self._socket_path))
            connection.sendall(frame)
            content = _receive_frame(
                connection,
                expected_type=FrameType.RESPONSE,
                body_limit=self._config.max_result_bytes,
                deadline=deadline,
                cancelled=cancelled,
            )
            if content is None:
                return _local_failure(StableErrorCode.PARSER_CANCELLED)
            if content == b"":
                return _local_failure(StableErrorCode.PARSER_SANDBOX_UNAVAILABLE)
            response, body = decode_response(content, invocation_id=invocation_id)
            _validate_result_limit(body, header.max_result_bytes)
            return ControllerOutcome(
                response=response, body=body, local_error_code=None
            )
        except ProtocolError:
            if cancelled is not None and cancelled():
                return _local_failure(StableErrorCode.PARSER_CANCELLED)
            return _local_failure(StableErrorCode.PROTOCOL_INVALID)
        except (ConnectionError, OSError, TimeoutError):
            if cancelled is not None and cancelled():
                return _local_failure(StableErrorCode.PARSER_CANCELLED)
            return _local_failure(StableErrorCode.PARSER_SANDBOX_UNAVAILABLE)
        finally:
            connection.close()


def receive_request_frame(connection: socket.socket, *, deadline: float) -> bytes:
    """Receive one bounded request frame for the sidecar server."""
    content = _receive_frame(
        connection,
        expected_type=FrameType.REQUEST,
        body_limit=DEFAULT_CONFIG.max_request_bytes,
        deadline=deadline,
        cancelled=None,
    )
    if content is None or content == b"":
        raise ProtocolError("request connection closed before one complete frame")
    return content


def _receive_frame(
    connection: socket.socket,
    *,
    expected_type: FrameType,
    body_limit: int,
    deadline: float,
    cancelled: Callable[[], bool] | None,
) -> bytes | None:
    prefix = _receive_exact(
        connection,
        _PREFIX.size,
        deadline=deadline,
        cancelled=cancelled,
    )
    if prefix is None or prefix == b"":
        return prefix
    magic, major, raw_type, flags, header_length, body_length = _PREFIX.unpack(prefix)
    if (
        magic != b"MMCP"
        or major != 1
        or raw_type != expected_type.value
        or flags != 0
        or header_length < 1
        or header_length > DEFAULT_CONFIG.max_header_bytes
        or body_length > body_limit
    ):
        raise ProtocolError("invalid MMCP stream prefix")
    remainder = _receive_exact(
        connection,
        header_length + body_length,
        deadline=deadline,
        cancelled=cancelled,
    )
    if remainder is None:
        return None
    if len(remainder) != header_length + body_length:
        return b""
    try:
        trailing = connection.recv(1, socket.MSG_PEEK | socket.MSG_DONTWAIT)
    except (BlockingIOError, TimeoutError):
        trailing = b""
    if trailing:
        raise ProtocolError("MMCP stream contains trailing bytes")
    return prefix + remainder


def _receive_exact(
    connection: socket.socket,
    length: int,
    *,
    deadline: float,
    cancelled: Callable[[], bool] | None,
) -> bytes | None:
    result = bytearray()
    while len(result) < length:
        if cancelled is not None and cancelled():
            return None
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise TimeoutError
        connection.settimeout(min(remaining, _READ_POLL_SECONDS))
        try:
            chunk = connection.recv(length - len(result))
        except TimeoutError:
            continue
        if not chunk:
            break
        result.extend(chunk)
    return bytes(result)


def _validate_socket_path(path: Path) -> None:
    observed = path.lstat()
    if not stat.S_ISSOCK(observed.st_mode):
        raise OSError("parser IPC path is not a Unix socket")
    if stat.S_IMODE(observed.st_mode) & 0o007:
        raise OSError("parser IPC socket is accessible to other users")


def _local_failure(code: StableErrorCode) -> ControllerOutcome:
    return ControllerOutcome(response=None, body=b"", local_error_code=code)


def _validate_result_limit(body: bytes, max_result_bytes: int) -> None:
    if len(body) > max_result_bytes:
        raise ProtocolError("response exceeds caller maxResultBytes")
