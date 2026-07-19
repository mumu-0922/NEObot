from __future__ import annotations

import importlib.util
import threading
from http.client import HTTPConnection
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


def _load_proxy() -> ModuleType:
    path = Path(__file__).parents[3] / "scripts" / "mineru_result_proxy_wsl.py"
    spec = importlib.util.spec_from_file_location("mineru_result_proxy_wsl", path)
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


proxy = _load_proxy()


def test_result_proxy_accepts_only_exact_mineru_https_capability() -> None:
    valid = "https://cdn-mineru.openxlab.org.cn/pdf/f45/result.zip"
    assert proxy.validate_result_url(valid) == valid
    assert proxy.decode_request(b'{"resultUrl":"' + valid.encode() + b'"}') == valid

    invalid = (
        "http://cdn-mineru.openxlab.org.cn/pdf/f45/result.zip",
        "https://evil.example/pdf/f45/result.zip",
        "https://cdn-mineru.openxlab.org.cn/pdf/../result.zip",
        "https://cdn-mineru.openxlab.org.cn/pdf/f45/result.zip?token=value",
        "https://cdn-mineru.openxlab.org.cn/pdf/f45/result.txt",
    )
    for value in invalid:
        with pytest.raises(proxy.RequestRejectedError):
            proxy.validate_result_url(value)


def test_result_proxy_rejects_open_or_ambiguous_json() -> None:
    invalid = (
        b"{}",
        b'{"url":"https://cdn-mineru.openxlab.org.cn/pdf/f45/result.zip"}',
        b'{"resultUrl":"https://cdn-mineru.openxlab.org.cn/pdf/a.zip",'
        b'"resultUrl":"https://cdn-mineru.openxlab.org.cn/pdf/b.zip"}',
        b'{"resultUrl":"https://cdn-mineru.openxlab.org.cn/pdf/a.zip","headers":{}}',
    )
    for body in invalid:
        with pytest.raises(proxy.RequestRejectedError):
            proxy.decode_request(body)


def test_windows_path_conversion_is_closed() -> None:
    assert (
        proxy._windows_path(Path("/mnt/c/Windows/Temp/result.zip"))
        == "C:/Windows/Temp/result.zip"
    )
    with pytest.raises(proxy.FetchFailedError):
        proxy._windows_path(Path("/var/lib/mm-chat/result.zip"))


def test_result_fetch_keeps_signed_url_out_of_argv_and_removes_temp_file(
    monkeypatch: pytest.MonkeyPatch,
    tmp_path: Path,
) -> None:
    curl = tmp_path / "curl.exe"
    curl.write_bytes(b"fixture")
    temp_root = tmp_path / "temp"
    temp_root.mkdir()
    monkeypatch.setenv("MM_CHAT_WINDOWS_CURL", str(curl))
    monkeypatch.setenv("MM_CHAT_WINDOWS_TEMP", str(temp_root))
    monkeypatch.setenv("HTTPS_PROXY", "http://sensitive-proxy.test")
    monkeypatch.setattr(proxy, "_windows_path", lambda path: str(path))
    captured: dict[str, object] = {}

    def fake_run(args: list[str], **kwargs: object) -> SimpleNamespace:
        captured.update({"args": args, **kwargs})
        config = kwargs["input"]
        assert isinstance(config, str)
        output = next(
            line.removeprefix('output = "').removesuffix('"')
            for line in config.splitlines()
            if line.startswith("output = ")
        )
        Path(output).write_bytes(b"PK bounded zip fixture")
        return SimpleNamespace(returncode=0)

    monkeypatch.setattr(proxy.subprocess, "run", fake_run)
    result_url = "https://cdn-mineru.openxlab.org.cn/pdf/f45/result.zip"
    assert proxy.fetch_result(result_url) == b"PK bounded zip fixture"
    assert captured["args"] == [str(curl), "--config", "-"]
    assert result_url not in " ".join(captured["args"])
    assert result_url in str(captured["input"])
    assert "HTTPS_PROXY" not in captured["env"]
    assert not tuple(temp_root.iterdir())


def test_result_proxy_http_surface_is_closed(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    fetched: list[str] = []

    def fake_fetch(value: str) -> bytes:
        fetched.append(value)
        return b"PK proxy fixture"

    monkeypatch.setattr(proxy, "fetch_result", fake_fetch)
    server = proxy.ThreadingHTTPServer(
        ("127.0.0.1", 0),
        proxy.ResultProxyHandler,
    )
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    connection = HTTPConnection("127.0.0.1", server.server_port, timeout=3)
    valid = "https://cdn-mineru.openxlab.org.cn/pdf/f45/result.zip"
    try:
        connection.request(
            "POST",
            "/mineru-result",
            body=f'{{"resultUrl":"{valid}"}}',
            headers={"Content-Type": "application/json"},
        )
        response = connection.getresponse()
        assert response.status == 200
        assert response.read() == b"PK proxy fixture"

        connection.request(
            "POST",
            "/mineru-result",
            body=f'{{"resultUrl":"{valid}"}}',
            headers={
                "Authorization": "Bearer forbidden",
                "Content-Type": "application/json",
            },
        )
        response = connection.getresponse()
        assert response.status == 400
        assert response.read() == b'{"error":{"code":"RESULT_PROXY_FAILED"}}\n'

        connection.request("GET", "/mineru-result")
        response = connection.getresponse()
        assert response.status == 405
        response.read()
    finally:
        connection.close()
        server.shutdown()
        server.server_close()
        thread.join(timeout=3)
    assert fetched == [valid]
