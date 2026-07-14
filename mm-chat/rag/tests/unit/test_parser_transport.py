"""Private UDS sidecar/controller and bounded-admission integration tests."""

from __future__ import annotations

import socket
import struct
import threading
import time
from pathlib import Path
from types import SimpleNamespace

import pytest

from mm_chat_rag.offline_parser import sidecar, smoke, transport
from mm_chat_rag.offline_parser.errors import StableErrorCode
from mm_chat_rag.offline_parser.protocol import (
    FrameType,
    build_request_header,
    encode_frame,
)
from mm_chat_rag.offline_parser.sandbox import SandboxRouteResult
from mm_chat_rag.offline_parser.sidecar import ParserSidecar, _AdmissionGate
from mm_chat_rag.offline_parser.transport import ParserController


def _start_sidecar(parent: Path) -> tuple[ParserSidecar, threading.Thread, Path]:
    parent.chmod(0o700)
    socket_path = parent / "parser.sock"
    sidecar = ParserSidecar(socket_path)
    thread = threading.Thread(target=sidecar.serve, daemon=True)
    thread.start()
    deadline = time.monotonic() + 5
    while not socket_path.exists() and time.monotonic() < deadline:
        time.sleep(0.01)
    assert socket_path.exists()
    return sidecar, thread, socket_path


def test_sidecar_uds_round_trip_stays_runtime_inert(tmp_path: Path) -> None:
    sidecar, thread, socket_path = _start_sidecar(tmp_path)
    try:
        controller = ParserController(socket_path)

        ambiguous = controller.invoke(b"plain", invocation_id="ambiguous")
        routed = controller.invoke(
            b"plain",
            invocation_id="routed",
            declared_extension=".txt",
        )

        assert ambiguous.response is not None
        assert ambiguous.response.stable_error_code is StableErrorCode.FORMAT_AMBIGUOUS
        assert ambiguous.local_error_code is None
        assert not ambiguous.stageable
        assert routed.response is not None
        assert routed.response.stable_error_code is StableErrorCode.FORMAT_UNSUPPORTED
        assert not routed.stageable
    finally:
        sidecar.stop()
        thread.join(timeout=5)
    assert not thread.is_alive()


def test_controller_cancel_fence_wins_before_connect(tmp_path: Path) -> None:
    outcome = ParserController(tmp_path / "missing.sock").invoke(
        b"source",
        invocation_id="cancelled",
        cancelled=lambda: True,
    )

    assert outcome.response is None
    assert outcome.local_error_code is StableErrorCode.PARSER_CANCELLED
    assert not outcome.stageable


def test_controller_maps_clean_eof_to_sandbox_unavailable(tmp_path: Path) -> None:
    socket_path = tmp_path / "eof.sock"
    listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    listener.bind(str(socket_path))
    socket_path.chmod(0o660)
    listener.listen(1)

    def close_once() -> None:
        connection, _address = listener.accept()
        connection.close()
        listener.close()

    thread = threading.Thread(target=close_once)
    thread.start()
    outcome = ParserController(socket_path).invoke(b"source", invocation_id="eof")
    thread.join(timeout=2)

    assert outcome.response is None
    assert outcome.local_error_code is StableErrorCode.PARSER_SANDBOX_UNAVAILABLE


def test_controller_rejects_regular_path_and_invalid_response(tmp_path: Path) -> None:
    regular = tmp_path / "regular"
    regular.write_text("not a socket", encoding="utf-8")
    unavailable = ParserController(regular).invoke(b"x", invocation_id="regular")
    assert unavailable.local_error_code is StableErrorCode.PARSER_SANDBOX_UNAVAILABLE

    socket_path = tmp_path / "invalid.sock"
    listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    listener.bind(str(socket_path))
    socket_path.chmod(0o660)
    listener.listen(1)

    def send_invalid() -> None:
        connection, _address = listener.accept()
        connection.recv(4096)
        connection.sendall(b"MMCP" + b"\x01\x02\x00\x00" + b"\x00" * 12)
        connection.close()
        listener.close()

    thread = threading.Thread(target=send_invalid)
    thread.start()
    invalid = ParserController(socket_path).invoke(b"x", invocation_id="invalid")
    thread.join(timeout=2)
    assert invalid.local_error_code is StableErrorCode.PROTOCOL_INVALID


def test_admission_gate_allows_one_active_one_waiter_and_rejects_third() -> None:
    gate = _AdmissionGate()
    assert gate.enter()
    waiting_started = threading.Event()
    waiting_entered = threading.Event()

    def wait_for_slot() -> None:
        waiting_started.set()
        if gate.enter():
            waiting_entered.set()
            gate.leave()

    waiter = threading.Thread(target=wait_for_slot)
    waiter.start()
    assert waiting_started.wait(timeout=1)
    deadline = time.monotonic() + 1
    while gate._waiting != 1 and time.monotonic() < deadline:
        time.sleep(0.01)

    assert not gate.enter()
    gate.leave()
    waiter.join(timeout=1)
    assert waiting_entered.is_set()


def test_transport_stream_prefix_trailing_cancel_and_deadline_gates() -> None:
    left, right = socket.socketpair()
    try:
        left.sendall(b"FAIL" + b"\x00" * 16)
        with pytest.raises(transport.ProtocolError):
            transport.receive_request_frame(right, deadline=time.monotonic() + 1)
    finally:
        left.close()
        right.close()

    left, right = socket.socketpair()
    try:
        left.close()
        with pytest.raises(transport.ProtocolError, match="closed"):
            transport.receive_request_frame(right, deadline=time.monotonic() + 1)
    finally:
        right.close()


def test_transport_stream_remainder_cancel_short_and_trailing_paths() -> None:
    prefix = struct.pack(">4sBBHIQ", b"MMCP", 1, 1, 0, 2, 1)

    left, right = socket.socketpair()
    try:
        left.sendall(prefix)
        calls = 0

        def cancel_after_prefix() -> bool:
            nonlocal calls
            calls += 1
            return calls > 1

        assert (
            transport._receive_frame(
                right,
                expected_type=FrameType.REQUEST,
                body_limit=10,
                deadline=time.monotonic() + 1,
                cancelled=cancel_after_prefix,
            )
            is None
        )
    finally:
        left.close()
        right.close()

    left, right = socket.socketpair()
    try:
        left.sendall(prefix + b"{}")
        left.close()
        assert (
            transport._receive_frame(
                right,
                expected_type=FrameType.REQUEST,
                body_limit=10,
                deadline=time.monotonic() + 1,
                cancelled=None,
            )
            == b""
        )
    finally:
        right.close()

    left, right = socket.socketpair()
    try:
        left.sendall(prefix + b"{}x" + b"trailing")
        with pytest.raises(transport.ProtocolError, match="trailing"):
            transport._receive_frame(
                right,
                expected_type=FrameType.REQUEST,
                body_limit=10,
                deadline=time.monotonic() + 1,
                cancelled=None,
            )
    finally:
        left.close()
        right.close()

    left, right = socket.socketpair()
    try:
        assert (
            transport._receive_exact(
                right,
                1,
                deadline=time.monotonic() + 1,
                cancelled=lambda: True,
            )
            is None
        )
        with pytest.raises(TimeoutError):
            transport._receive_exact(
                right,
                1,
                deadline=time.monotonic() - 1,
                cancelled=None,
            )
    finally:
        left.close()
        right.close()


def test_transport_rejects_world_accessible_socket_and_result_overflow(
    tmp_path: Path,
) -> None:
    path = tmp_path / "socket"
    listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    listener.bind(str(path))
    path.chmod(0o666)
    try:
        with pytest.raises(OSError, match="other users"):
            transport._validate_socket_path(path)
        with pytest.raises(transport.ProtocolError, match="maxResultBytes"):
            transport._validate_result_limit(b"xx", 1)
    finally:
        listener.close()


def _request_frame(invocation_id: str = "sidecar-branch") -> bytes:
    source = b"x"
    header = build_request_header(
        invocation_id=invocation_id,
        source=source,
        parser_config_hash=sidecar.DEFAULT_CONFIG.config_hash,
        deadline_unix_millis=int(time.time() * 1000) + 60_000,
        max_result_bytes=1024,
    )
    return encode_frame(FrameType.REQUEST, header.to_object(), source)


@pytest.mark.parametrize(
    ("result", "fatal"),
    [
        (SandboxRouteResult(None, StableErrorCode.PARSER_CANCELLED), False),
        (SandboxRouteResult(None, StableErrorCode.PARSER_SANDBOX_UNAVAILABLE), True),
        (SandboxRouteResult(None, StableErrorCode.INPUT_INVALID), False),
        (SandboxRouteResult(None, None, requires_restart=True), True),
    ],
)
def test_sidecar_controller_only_and_restart_branches(
    tmp_path: Path,
    result: SandboxRouteResult,
    fatal: bool,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    parser_sidecar = ParserSidecar(tmp_path / "unused.sock")
    monkeypatch.setattr(
        parser_sidecar._supervisor, "route", lambda *_args, **_kwargs: result
    )
    client, server = socket.socketpair()
    client.sendall(_request_frame())

    parser_sidecar._handle_connection(server)

    assert parser_sidecar._fatal.is_set() is fatal
    client.settimeout(0.1)
    if result.stable_error_code is StableErrorCode.INPUT_INVALID:
        assert client.recv(4096)
    else:
        assert client.recv(1) == b""
    client.close()


def test_sidecar_busy_and_protocol_error_responses(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    parser_sidecar = ParserSidecar(tmp_path / "unused.sock")
    monkeypatch.setattr(parser_sidecar._admission, "enter", lambda: False)
    client, server = socket.socketpair()
    client.sendall(_request_frame("busy"))
    parser_sidecar._handle_connection(server)
    assert b"PARSER_BUSY" in client.recv(4096)
    client.close()

    parser_sidecar = ParserSidecar(tmp_path / "unused-2.sock")
    monkeypatch.setattr(
        parser_sidecar._supervisor,
        "route",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(ValueError()),
    )
    client, server = socket.socketpair()
    client.sendall(_request_frame("protocol"))
    parser_sidecar._handle_connection(server)
    assert b"PROTOCOL_INVALID" in client.recv(4096)
    client.close()


def test_sidecar_parent_and_main_identity_gates(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    public = tmp_path / "public"
    public.mkdir(mode=0o755)
    with pytest.raises(OSError, match="other users"):
        sidecar._prepare_socket_parent(public)
    regular = tmp_path / "file"
    regular.write_text("x", encoding="utf-8")
    with pytest.raises(OSError, match="directory"):
        sidecar._prepare_socket_parent(regular)

    monkeypatch.setattr(sidecar.os, "getpid", lambda: 2)
    monkeypatch.setattr(sidecar.sys, "argv", ["parser-sidecar"])
    assert sidecar.main() == 64


def test_smoke_main_success_and_failure_branches(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    good_response = SimpleNamespace(
        stable_error_code=StableErrorCode.FORMAT_AMBIGUOUS,
    )
    good = SimpleNamespace(
        response=good_response,
        body=b"",
        local_error_code=None,
        stageable=False,
    )

    class FakeRoot:
        def __enter__(self) -> FakeRoot:
            return self

        def __exit__(self, *_args: object) -> None:
            return None

        def write_artifact(self, _path: str, _content: bytes) -> None:
            return None

    monkeypatch.setattr(
        smoke.ParserController, "invoke", lambda *_args, **_kwargs: good
    )
    monkeypatch.setattr(smoke.OwnedOutputRoot, "create", lambda: FakeRoot())
    assert smoke.main() == 0

    bad = SimpleNamespace(
        response=None, body=b"", local_error_code=None, stageable=False
    )
    monkeypatch.setattr(smoke.ParserController, "invoke", lambda *_args, **_kwargs: bad)
    assert smoke.main() == 1
