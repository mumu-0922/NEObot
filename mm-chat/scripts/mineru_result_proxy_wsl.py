#!/usr/bin/env python3
"""Narrow WSL-to-Windows MinerU result downloader for Docker Desktop.

Some Docker Desktop/WSL networks can reach MinerU API and upload hosts but see
TLS EOFs from the result CDN. This optional localhost service accepts only the
already validated MinerU result capability and delegates the HTTPS GET to
Windows curl, whose Schannel path remains reachable. It never receives a
MinerU API Key or Authorization header.
"""

from __future__ import annotations

import argparse
import contextlib
import json
import os
import secrets
import subprocess
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Final
from urllib.parse import urlsplit

LISTEN_HOST: Final = "0.0.0.0"
LISTEN_PORT: Final = 18081
RESULT_PATH: Final = "/mineru-result"
RESULT_HOST: Final = "cdn-mineru.openxlab.org.cn"
RESULT_PATH_PREFIX: Final = "/pdf/"
RESULT_PATH_SUFFIX: Final = ".zip"
MAX_REQUEST_BYTES: Final = 8192
MAX_RESULT_BYTES: Final = 32 * 1024 * 1024
MAX_RESULT_URL_BYTES: Final = 4096
VISIBLE_ASCII_MIN: Final = 33
VISIBLE_ASCII_MAX: Final = 126
MIN_WSL_PATH_PARTS: Final = 4
MAX_TCP_PORT: Final = 65535
if os.name == "nt":
    _windows_curl_default = Path("C:/Windows/System32/curl.exe")
    _windows_temp_default = Path("C:/Windows/Temp")
else:
    _windows_curl_default = Path("/mnt/c/Windows/System32/curl.exe")
    _windows_temp_default = Path("/mnt/c/Windows/Temp")
WINDOWS_CURL_DEFAULT: Final = _windows_curl_default
WINDOWS_TEMP_DEFAULT: Final = _windows_temp_default


class RequestRejectedError(ValueError):
    """Raised for a closed request/URL contract violation."""


class FetchFailedError(RuntimeError):
    """Raised when the bounded Windows download fails."""


class DuplicateKeyError(ValueError):
    """Raised when untrusted JSON contains a duplicate key."""


class RequestHTTPError(ValueError):
    """Raised with the fixed HTTP status for an invalid local request."""

    def __init__(self, status: int) -> None:
        self.status = status
        super().__init__("request rejected")


def validate_result_url(value: object) -> str:
    """Admit only one exact HTTPS MinerU result-CDN capability."""
    if not isinstance(value, str) or not value or value != value.strip():
        raise RequestRejectedError
    if len(value.encode("utf-8")) > MAX_RESULT_URL_BYTES or any(
        ord(character) < VISIBLE_ASCII_MIN or ord(character) > VISIBLE_ASCII_MAX
        for character in value
    ):
        raise RequestRejectedError
    try:
        parsed = urlsplit(value)
        port = parsed.port
    except ValueError as error:
        raise RequestRejectedError from error
    if (
        parsed.scheme != "https"
        or parsed.hostname != RESULT_HOST
        or port not in {None, 443}
        or parsed.username is not None
        or parsed.password is not None
        or parsed.query
        or parsed.fragment
        or not parsed.path.startswith(RESULT_PATH_PREFIX)
        or not parsed.path.endswith(RESULT_PATH_SUFFIX)
        or not _safe_dynamic_path(parsed.path)
    ):
        raise RequestRejectedError
    return value


def decode_request(body: bytes) -> str:
    """Decode one exact, bounded JSON request without duplicate keys."""
    if not isinstance(body, bytes) or not 1 <= len(body) <= MAX_REQUEST_BYTES:
        raise RequestRejectedError
    try:
        payload = json.loads(
            body.decode("utf-8", errors="strict"),
            object_pairs_hook=_unique_object,
        )
    except (UnicodeError, json.JSONDecodeError, DuplicateKeyError) as error:
        raise RequestRejectedError from error
    if not isinstance(payload, dict) or set(payload) != {"resultUrl"}:
        raise RequestRejectedError
    return validate_result_url(payload["resultUrl"])


def fetch_result(url: str) -> bytes:
    """Download through Windows Schannel without exposing the URL in argv."""
    admitted = validate_result_url(url)
    curl_path = Path(os.environ.get("MM_CHAT_WINDOWS_CURL", str(WINDOWS_CURL_DEFAULT)))
    temp_root = Path(os.environ.get("MM_CHAT_WINDOWS_TEMP", str(WINDOWS_TEMP_DEFAULT)))
    if not curl_path.is_file() or not temp_root.is_dir():
        raise FetchFailedError
    filename = f"mm-chat-mineru-{secrets.token_hex(16)}.zip"
    wsl_target = temp_root / filename
    windows_target = _windows_path(wsl_target)
    config = (
        f'url = "{admitted}"\n'
        f'output = "{windows_target}"\n'
        'proto = "=https"\n'
        "fail\n"
        "silent\n"
        "show-error\n"
        "tlsv1.2\n"
        "max-time = 90\n"
    )
    try:
        return _run_windows_curl(curl_path, wsl_target, config)
    finally:
        with contextlib.suppress(OSError):
            wsl_target.unlink(missing_ok=True)


def _run_windows_curl(curl_path: Path, target: Path, config: str) -> bytes:
    """Run one bounded Schannel fetch with proxy inheritance disabled."""
    clean_env = dict(os.environ)
    for name in (
        "HTTP_PROXY",
        "HTTPS_PROXY",
        "ALL_PROXY",
        "http_proxy",
        "https_proxy",
        "all_proxy",
    ):
        clean_env.pop(name, None)
    try:
        completed = subprocess.run(  # noqa: S603 - fixed operator-owned curl path
            [str(curl_path), "--config", "-"],
            input=config,
            text=True,
            capture_output=True,
            timeout=100,
            check=False,
            env=clean_env,
        )
        if completed.returncode != 0:
            raise FetchFailedError
        info = target.stat()
        if not info.st_mode or not 1 <= info.st_size <= MAX_RESULT_BYTES:
            raise FetchFailedError
        content = target.read_bytes()
        if len(content) != info.st_size or not content.startswith(b"PK"):
            raise FetchFailedError
    except (OSError, subprocess.SubprocessError) as error:
        raise FetchFailedError from error
    else:
        return content


class ResultProxyHandler(BaseHTTPRequestHandler):
    """Serve one closed result-download operation with fixed errors."""

    server_version = "mm-chat-mineru-result-proxy/1"
    sys_version = ""

    def do_POST(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler contract
        try:
            result_url = self._read_result_url()
            content = fetch_result(result_url)
        except RequestHTTPError as error:
            self._error(error.status)
            return
        except RequestRejectedError:
            self._error(400)
            return
        except FetchFailedError:
            self._error(502)
            return
        self.send_response(200)
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Encoding", "identity")
        self.send_header("Content-Length", str(len(content)))
        self.send_header("Content-Type", "application/zip")
        self.end_headers()
        self.wfile.write(content)

    def _read_result_url(self) -> str:
        if self.path != RESULT_PATH:
            raise RequestHTTPError(404)
        if self.headers.get("Authorization") or self.headers.get("Cookie"):
            raise RequestHTTPError(400)
        if self.headers.get("Transfer-Encoding"):
            raise RequestHTTPError(400)
        content_type = self.headers.get("Content-Type", "").split(";", 1)[0]
        if content_type.strip().lower() != "application/json":
            raise RequestHTTPError(415)
        raw_length = self.headers.get("Content-Length", "")
        if not raw_length.isascii() or not raw_length.isdecimal():
            raise RequestHTTPError(400)
        length = int(raw_length)
        if not 1 <= length <= MAX_REQUEST_BYTES:
            raise RequestHTTPError(413)
        return decode_request(self.rfile.read(length))

    def do_GET(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler contract
        self._error(405)

    def log_message(self, format: str, *args: object) -> None:  # noqa: A002
        _ = format, args

    def _error(self, status: int) -> None:
        body = b'{"error":{"code":"RESULT_PROXY_FAILED"}}\n'
        self.send_response(status)
        self.send_header("Cache-Control", "no-store")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(body)


def _unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise DuplicateKeyError
        result[key] = value
    return result


def _safe_dynamic_path(path: str) -> bool:
    return (
        bool(path)
        and "%" not in path
        and "\\" not in path
        and all(segment not in {"", ".", ".."} for segment in path.split("/")[1:])
    )


def _windows_path(path: Path) -> str:
    if path.drive:
        return path.as_posix()
    parts = path.parts
    if (
        len(parts) < MIN_WSL_PATH_PARTS
        or parts[:2] != ("/", "mnt")
        or len(parts[2]) != 1
    ):
        raise FetchFailedError
    drive = parts[2].upper()
    return f"{drive}:/{'/'.join(parts[3:])}"


def main() -> None:
    """Run the optional closed Windows result downloader."""
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default=LISTEN_HOST)
    parser.add_argument("--port", default=LISTEN_PORT, type=int)
    args = parser.parse_args()
    if not 1 <= args.port <= MAX_TCP_PORT:
        raise SystemExit("invalid port")
    server = ThreadingHTTPServer((args.host, args.port), ResultProxyHandler)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
