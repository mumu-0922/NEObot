"""C1.2 private UDS sidecar with bounded admission and child isolation."""

from __future__ import annotations

import argparse
import os
import signal
import socket
import stat
import sys
import threading
import time
from pathlib import Path
from typing import Final

from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG
from mm_chat_rag.offline_parser.errors import CONTROLLER_ONLY_ERRORS, StableErrorCode
from mm_chat_rag.offline_parser.protocol import (
    FrameType,
    ProtocolError,
    RequestHeader,
    ResponseHeader,
    decode_request,
    encode_frame,
)
from mm_chat_rag.offline_parser.sandbox import SandboxSupervisor
from mm_chat_rag.offline_parser.transport import (
    DEFAULT_SOCKET_PATH,
    receive_request_frame,
)

_SOCKET_MODE: Final = 0o660


class _AdmissionGate:
    """Exactly one active and one waiting invocation."""

    def __init__(self) -> None:
        self._condition = threading.Condition()
        self._active = False
        self._waiting = 0

    def enter(self) -> bool:
        with self._condition:
            if not self._active:
                self._active = True
                return True
            if self._waiting >= 1:
                return False
            self._waiting += 1
            try:
                self._condition.wait_for(self._slot_available)
                self._active = True
                return True
            finally:
                self._waiting -= 1

    def leave(self) -> None:
        with self._condition:
            self._active = False
            self._condition.notify(1)

    def _slot_available(self) -> bool:
        return not self._active


class ParserSidecar:
    """Serve MMCP while keeping C1.3 Native Artifacts off the external wire."""

    def __init__(self, socket_path: Path = DEFAULT_SOCKET_PATH) -> None:
        self._socket_path = socket_path
        self._supervisor = SandboxSupervisor()
        self._admission = _AdmissionGate()
        self._stop = threading.Event()
        self._fatal = threading.Event()
        self._threads: set[threading.Thread] = set()

    def serve(self) -> int:
        """Bind the private socket and serve until signalled or restart-fenced."""
        _prepare_socket_parent(self._socket_path.parent)
        try:
            self._socket_path.unlink(missing_ok=True)
        except OSError:
            return 70
        listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        try:
            listener.bind(str(self._socket_path))
            self._socket_path.chmod(_SOCKET_MODE, follow_symlinks=False)
            listener.listen(2)
            listener.settimeout(0.2)
            while not self._stop.is_set() and not self._fatal.is_set():
                try:
                    connection, _address = listener.accept()
                except TimeoutError:
                    continue
                thread = threading.Thread(
                    target=self._handle_connection,
                    args=(connection,),
                    name="parser-invocation",
                    daemon=False,
                )
                self._threads.add(thread)
                thread.start()
                self._threads = {item for item in self._threads if item.is_alive()}
        finally:
            listener.close()
            self._stop.set()
            for thread in tuple(self._threads):
                thread.join(timeout=2)
            self._socket_path.unlink(missing_ok=True)
        return 75 if self._fatal.is_set() else 0

    def stop(self) -> None:
        self._stop.set()

    def _handle_connection(self, connection: socket.socket) -> None:
        invocation_id: str | None = None
        entered = False
        try:
            deadline = (
                time.monotonic() + DEFAULT_CONFIG.sandbox.wall_timeout_millis / 1000
            )
            content = receive_request_frame(connection, deadline=deadline)
            request, source = decode_request(
                content,
                expected_config_hash=DEFAULT_CONFIG.config_hash,
            )
            invocation_id = request.invocation_id
            entered = self._admission.enter()
            if not entered:
                self._send_failure(connection, request, StableErrorCode.PARSER_BUSY)
                return
            result = self._supervisor.route(
                source,
                declared_mime=request.declared_mime,
                declared_extension=request.declared_extension,
                cancelled=lambda: _peer_closed(connection),
                deadline_monotonic=deadline,
            )
            if result.requires_restart:
                self._fatal.set()
                return
            if result.stable_error_code in CONTROLLER_ONLY_ERRORS:
                if (
                    result.stable_error_code
                    is StableErrorCode.PARSER_SANDBOX_UNAVAILABLE
                ):
                    self._fatal.set()
                return
            if result.stable_error_code is not None:
                self._send_failure(connection, request, result.stable_error_code)
                return
            # C1.3 Native Artifacts are child-internal inputs to the future C1.4
            # canonicalizer. MMCP success is frozen to canonical-ir.v2, so even a
            # verified Native Artifact must remain zero-body and non-stageable.
            self._send_failure(connection, request, StableErrorCode.FORMAT_UNSUPPORTED)
        except (OSError, ProtocolError, ValueError):
            if invocation_id is not None:
                try:
                    response = ResponseHeader.failure(
                        invocation_id,
                        StableErrorCode.PROTOCOL_INVALID,
                    )
                    connection.sendall(
                        encode_frame(FrameType.RESPONSE, response.to_object(), b"")
                    )
                except OSError:
                    pass
        finally:
            if entered:
                self._admission.leave()
            connection.close()

    @staticmethod
    def _send_failure(
        connection: socket.socket,
        request: RequestHeader,
        code: StableErrorCode,
    ) -> None:
        response = ResponseHeader.failure(request.invocation_id, code)
        connection.sendall(encode_frame(FrameType.RESPONSE, response.to_object(), b""))


def main() -> int:
    """Run the sidecar as container PID 1 unless explicitly test-overridden."""
    parser = argparse.ArgumentParser(description="MM Chat offline parser sidecar")
    parser.add_argument("--socket", type=Path, default=DEFAULT_SOCKET_PATH)
    parser.add_argument("--allow-non-pid1", action="store_true", help=argparse.SUPPRESS)
    arguments = parser.parse_args()
    if os.getpid() != 1 and not arguments.allow_non_pid1:
        sys.stderr.write("parser-sidecar must be container PID 1\n")
        return 64
    if os.geteuid() != DEFAULT_CONFIG.sidecar_uid and not arguments.allow_non_pid1:
        sys.stderr.write("parser-sidecar must run as UID 10002\n")
        return 64
    sidecar = ParserSidecar(arguments.socket)
    signal.signal(signal.SIGTERM, lambda _signal, _frame: sidecar.stop())
    signal.signal(signal.SIGINT, lambda _signal, _frame: sidecar.stop())
    return sidecar.serve()


def _prepare_socket_parent(parent: Path) -> None:
    observed = parent.lstat()
    if not stat.S_ISDIR(observed.st_mode):
        raise OSError("parser IPC parent is not a directory")
    if stat.S_IMODE(observed.st_mode) & 0o007:
        raise OSError("parser IPC parent is accessible to other users")


def _peer_closed(connection: socket.socket) -> bool:
    try:
        return connection.recv(1, socket.MSG_PEEK | socket.MSG_DONTWAIT) == b""
    except (BlockingIOError, TimeoutError):
        return False
    except OSError:
        return True


if __name__ == "__main__":
    raise SystemExit(main())
