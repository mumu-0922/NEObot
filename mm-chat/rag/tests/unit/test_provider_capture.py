from __future__ import annotations

import json
import os
import shutil
import stat
import subprocess
import sys
from collections.abc import Callable, Iterator
from datetime import UTC, datetime
from pathlib import Path
from typing import Any, cast

import httpx
import pytest
from tools.provider_capture import (
    CaptureError,
    CaptureRuntime,
    canonical_json_bytes,
    capture,
    deterministic_synthetic_pdf,
    dry_run_plan,
    evidence_sha256,
    main,
    validate_capture_proxy_url,
    validate_evidence_snapshot,
    validate_request_target,
    write_evidence_snapshot,
)

_OBSERVED_AT = datetime(2026, 7, 13, 4, 5, 6, tzinfo=UTC)
_KEYS = {
    "MINERU_API_KEY": "unit-test-mineru-credential",
}
_PRIVATE_PROXY = "http://172.16.0.2:7890"


def _mineru_payload() -> dict[str, Any]:
    return {
        "code": 0,
        "data": {
            "batch_id": "sensitive-mineru-batch-id",
            "file_urls": [
                "https://signed-upload.invalid/object?signature=sensitive-value"
            ],
        },
        "msg": "ok",
        "trace_id": "sensitive-mineru-trace-id",
    }


def _json_response(payload: object, *, status: int = 200) -> httpx.Response:
    content = json.dumps(payload, separators=(",", ":")).encode()

    return _bytes_response(
        content,
        status=status,
        headers={
            "Content-Type": "application/json; charset=utf-8",
            "X-Request-Id": "sensitive-response-header-value",
        },
    )


def _bytes_response(
    content: bytes,
    *,
    headers: dict[str, str],
    status: int = 200,
) -> httpx.Response:
    class StaticStream(httpx.SyncByteStream):
        def __iter__(self) -> Iterator[bytes]:
            yield content

    return httpx.Response(
        status,
        headers=headers,
        stream=StaticStream(),
    )


def _success_transport(
    requests: list[httpx.Request] | None = None,
) -> httpx.MockTransport:
    def handler(request: httpx.Request) -> httpx.Response:
        if requests is not None:
            requests.append(request)
        if request.url.path == "/api/v4/file-urls/batch":
            return _json_response(_mineru_payload())
        raise AssertionError("unexpected request")

    return httpx.MockTransport(handler)


def _valid_snapshot() -> dict[str, Any]:
    return cast(
        "dict[str, Any]",
        capture(
            "mineru",
            observed_at=_OBSERVED_AT,
            runtime=CaptureRuntime(
                environ=_KEYS,
                transport=_success_transport(),
            ),
        ),
    )


def test_default_cli_is_no_network_dry_run_and_creates_no_evidence(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    calls = 0

    def forbidden_network(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("dry-run made a network request")

    exit_code = main(
        [],
        runtime=CaptureRuntime(
            environ={},
            transport=httpx.MockTransport(forbidden_network),
            output_base=tmp_path,
        ),
    )

    assert exit_code == 0
    assert calls == 0
    assert list(tmp_path.iterdir()) == []
    output = json.loads(capsys.readouterr().out)
    assert output == dry_run_plan("all")
    assert output["networkEnabled"] is False
    assert output["evidenceFilesCreated"] is False
    assert output["proxy"] == {
        "automaticEnvironmentProxyLoaded": False,
        "explicitEnvironmentName": "PROVIDER_CAPTURE_PROXY_URL",
        "privateAddressOnly": True,
    }


def test_dry_run_ignores_even_invalid_explicit_proxy(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    exit_code = main(
        [],
        runtime=CaptureRuntime(
            environ={"PROVIDER_CAPTURE_PROXY_URL": "unit-test-invalid"},
            output_base=tmp_path,
        ),
    )

    assert exit_code == 0
    assert json.loads(capsys.readouterr().out)["networkEnabled"] is False
    assert list(tmp_path.iterdir()) == []


def test_default_cli_does_not_create_bytecode_files(tmp_path: Path) -> None:
    source_tools = Path(__file__).parents[2] / "tools"
    copied_tools = tmp_path / "tools"
    copied_tools.mkdir()
    for source in source_tools.glob("provider_capture*.py"):
        shutil.copyfile(source, copied_tools / source.name)
    environment = dict(os.environ)
    environment.pop("PYTHONDONTWRITEBYTECODE", None)
    for name in tuple(environment):
        if name.startswith(("COVERAGE_", "COV_CORE_")):
            environment.pop(name)
    environment["PYTHONPATH"] = str(tmp_path)

    result = subprocess.run(  # noqa: S603
        [sys.executable, "-B", "-m", "tools.provider_capture"],
        cwd=tmp_path,
        env=environment,
        check=False,
        capture_output=True,
        text=True,
    )

    assert result.returncode == 0
    assert json.loads(result.stdout)["networkEnabled"] is False
    assert not list(tmp_path.rglob("*.pyc"))
    assert not list(tmp_path.rglob("__pycache__"))


def test_execute_missing_key_fails_closed_before_network_or_directory(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    calls = 0

    def forbidden_network(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("missing credentials reached the network")

    exit_code = main(
        ["--execute", "--provider", "mineru", "--output-dir", "capture"],
        runtime=CaptureRuntime(
            environ={},
            transport=httpx.MockTransport(forbidden_network),
            output_base=tmp_path,
            now=_OBSERVED_AT,
        ),
    )

    output = capsys.readouterr()
    assert exit_code == 2
    assert output.out == ""
    assert output.err.strip() == "CAPTURE_CREDENTIALS_MISSING_OR_INVALID"
    assert calls == 0
    assert list(tmp_path.iterdir()) == []


def test_invalid_cli_argument_never_echoes_its_value(
    capsys: pytest.CaptureFixture[str],
) -> None:
    secret = "unit-test-secret-must-not-appear"

    exit_code = main(["--api-key", secret])

    output = capsys.readouterr()
    assert exit_code == 2
    assert output.out == ""
    assert output.err.strip() == "CLI_ARGUMENT_INVALID"
    assert secret not in output.err


def test_custom_transport_capture_error_is_sanitized(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    secret = "unit-test-secret-transport-detail"

    def handler(_: httpx.Request) -> httpx.Response:
        raise CaptureError(secret)

    exit_code = main(
        ["--execute", "--provider", "mineru", "--output-dir", "capture"],
        runtime=CaptureRuntime(
            environ=_KEYS,
            transport=httpx.MockTransport(handler),
            output_base=tmp_path,
            now=_OBSERVED_AT,
        ),
    )

    output = capsys.readouterr()
    assert exit_code == 2
    assert output.err.strip() == "CAPTURE_FAILED"
    assert secret not in output.err
    assert list(tmp_path.iterdir()) == []


def test_retired_jina_selection_is_rejected_before_network() -> None:
    calls = 0

    def forbidden_network(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("retired Jina selection reached the network")

    with pytest.raises(CaptureError, match="PROVIDER_SELECTION_INVALID"):
        capture(
            "jina",
            observed_at=_OBSERVED_AT,
            runtime=CaptureRuntime(
                environ={"JINA_API_KEY": "must-not-be-read"},
                transport=httpx.MockTransport(forbidden_network),
            ),
        )
    assert calls == 0


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        ("http://127.0.0.1:7890", "http://127.0.0.1:7890"),
        ("http://10.0.0.2:7890/", "http://10.0.0.2:7890"),
        ("http://172.16.0.2:7890", "http://172.16.0.2:7890"),
        ("http://192.168.1.2:7890", "http://192.168.1.2:7890"),
        ("http://[::1]:7890", "http://[::1]:7890"),
        ("http://[fd00::2]:7890", "http://[fd00::2]:7890"),
        (None, None),
        ("", None),
    ],
)
def test_explicit_private_proxy_is_strictly_canonicalized(
    value: str | None,
    expected: str | None,
) -> None:
    assert validate_capture_proxy_url(value) == expected


@pytest.mark.parametrize(
    "value",
    [
        " https://172.16.0.2:7890",
        "https://172.16.0.2:7890",
        "socks5://172.16.0.2:7890",
        "http://proxy.local:7890",
        "http://8.8.8.8:7890",
        "http://169.254.1.1:7890",
        "http://0.0.0.0:7890",
        "http://172.16.0.2",
        "http://172.16.0.2:0",
        "http://user:pass@172.16.0.2:7890",
        "http://172.16.0.2:7890/path",
        "http://172.16.0.2:7890/?token=secret",
        "http://172.16.0.2:7890/#fragment",
        "http://[fe80::1]:7890",
        "http://[2001:4860:4860::8888]:7890",
    ],
)
def test_explicit_proxy_rejects_non_private_or_credentialed_values(value: str) -> None:
    with pytest.raises(CaptureError, match="CAPTURE_PROXY_INVALID"):
        validate_capture_proxy_url(value)


def test_generic_proxy_environment_is_ignored(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    real_client = httpx.Client
    seen: dict[str, object] = {}

    client_constructor = cast("Callable[..., httpx.Client]", real_client)

    def client_factory(**kwargs: object) -> httpx.Client:
        seen["proxy"] = kwargs.pop("proxy")
        kwargs["transport"] = _success_transport()
        return client_constructor(**kwargs)

    monkeypatch.setattr("tools.provider_capture.httpx.Client", client_factory)
    capture(
        "mineru",
        observed_at=_OBSERVED_AT,
        runtime=CaptureRuntime(
            environ=_KEYS
            | {
                "ALL_PROXY": "http://10.0.0.3:7890",
                "HTTPS_PROXY": "http://10.0.0.3:7890",
                "HTTP_PROXY": "http://10.0.0.3:7890",
            },
        ),
    )

    assert seen["proxy"] is None


def test_explicit_private_proxy_is_injected_but_never_recorded(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    real_client = httpx.Client
    seen: dict[str, object] = {}

    client_constructor = cast("Callable[..., httpx.Client]", real_client)

    def client_factory(**kwargs: object) -> httpx.Client:
        seen["proxy"] = kwargs.pop("proxy")
        kwargs["transport"] = _success_transport()
        return client_constructor(**kwargs)

    monkeypatch.setattr("tools.provider_capture.httpx.Client", client_factory)
    snapshot = capture(
        "mineru",
        observed_at=_OBSERVED_AT,
        runtime=CaptureRuntime(
            environ=_KEYS | {"PROVIDER_CAPTURE_PROXY_URL": _PRIVATE_PROXY},
        ),
    )

    evidence = canonical_json_bytes(snapshot)
    assert seen["proxy"] == _PRIVATE_PROXY
    assert _PRIVATE_PROXY.encode() not in evidence
    assert b"172.16.0.2" not in evidence


def test_invalid_explicit_proxy_fails_before_network_and_never_echoes_value(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    calls = 0
    forbidden = "http://unit-test-user:unit-test-secret@10.0.0.2:7890"

    def handler(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("invalid proxy reached network")

    exit_code = main(
        ["--execute", "--provider", "mineru", "--output-dir", "capture"],
        runtime=CaptureRuntime(
            environ=_KEYS | {"PROVIDER_CAPTURE_PROXY_URL": forbidden},
            transport=httpx.MockTransport(handler),
            output_base=tmp_path,
            now=_OBSERVED_AT,
        ),
    )

    output = capsys.readouterr()
    assert exit_code == 2
    assert output.out == ""
    assert output.err.strip() == "CAPTURE_PROXY_INVALID"
    assert forbidden not in output.err
    assert calls == 0
    assert list(tmp_path.iterdir()) == []


def test_http_requests_disable_compression_and_do_not_replay_cookies() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        response = _json_response(_mineru_payload())
        response.headers["Set-Cookie"] = "provider-session=sensitive"
        return response

    capture(
        "mineru",
        observed_at=_OBSERVED_AT,
        runtime=CaptureRuntime(
            environ=_KEYS,
            transport=httpx.MockTransport(handler),
        ),
    )

    assert len(requests) == 1
    assert all(request.headers["accept-encoding"] == "identity" for request in requests)
    assert all("cookie" not in request.headers for request in requests)


@pytest.mark.parametrize(
    ("method", "url"),
    [
        ("POST", "http://api.jina.ai/v1/embeddings"),
        ("POST", "https://evil.invalid/v1/embeddings"),
        ("POST", "https://api.jina.ai:444/v1/embeddings"),
        ("POST", "https://api.jina.ai/v1/other"),
        ("GET", "https://api.jina.ai/v1/embeddings"),
        ("POST", "https://api.jina.ai/v1/embeddings?x=1"),
    ],
)
def test_wrong_scheme_host_port_path_method_and_query_are_rejected(
    method: str,
    url: str,
) -> None:
    with pytest.raises(CaptureError, match="TARGET_NOT_ALLOWLISTED"):
        validate_request_target(method, url)


def test_redirect_is_rejected_without_following_location() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(
            307,
            headers={"Location": "https://evil.invalid/steal"},
        )

    with pytest.raises(CaptureError, match="REDIRECT_FORBIDDEN"):
        capture(
            "mineru",
            observed_at=_OBSERVED_AT,
            runtime=CaptureRuntime(
                environ=_KEYS,
                transport=httpx.MockTransport(handler),
            ),
        )
    assert len(requests) == 1


@pytest.mark.parametrize(
    ("headers", "content", "error_code"),
    [
        ({"Content-Type": "text/plain"}, b"{}", "PROVIDER_CONTENT_TYPE_INVALID"),
        (
            {"Content-Type": "application/json"},
            b'{"duplicate":1,"duplicate":2}',
            "PROVIDER_JSON_INVALID",
        ),
        (
            {"Content-Type": "application/json"},
            b'{"invalid":\xff}',
            "PROVIDER_JSON_INVALID",
        ),
        (
            {"Content-Type": "application/json"},
            b'{"invalid":NaN}',
            "PROVIDER_JSON_INVALID",
        ),
    ],
)
def test_response_content_type_json_and_utf8_are_strict(
    headers: dict[str, str],
    content: bytes,
    error_code: str,
) -> None:
    transport = httpx.MockTransport(lambda _: _bytes_response(content, headers=headers))
    with pytest.raises(CaptureError, match=error_code):
        capture(
            "mineru",
            observed_at=_OBSERVED_AT,
            runtime=CaptureRuntime(environ=_KEYS, transport=transport),
        )


@pytest.mark.parametrize(
    ("headers", "error_code"),
    [
        (
            {
                "Content-Encoding": "gzip",
                "Content-Type": "application/json",
            },
            "PROVIDER_CONTENT_ENCODING_INVALID",
        ),
        (
            {
                "Content-Length": "1048577",
                "Content-Type": "application/json",
            },
            "PROVIDER_RESPONSE_TOO_LARGE",
        ),
    ],
)
def test_response_encoding_and_declared_length_are_fail_closed(
    headers: dict[str, str],
    error_code: str,
) -> None:
    transport = httpx.MockTransport(lambda _: _bytes_response(b"{}", headers=headers))
    with pytest.raises(CaptureError, match=error_code):
        capture(
            "mineru",
            observed_at=_OBSERVED_AT,
            runtime=CaptureRuntime(environ=_KEYS, transport=transport),
        )


def test_oversize_response_is_stopped_and_rejected() -> None:
    chunks_read = 0

    class OversizeStream(httpx.SyncByteStream):
        def __iter__(self) -> Iterator[bytes]:
            nonlocal chunks_read
            for chunk in (b"{" + b" " * 1_048_576, b"should-not-be-read"):
                chunks_read += 1
                yield chunk

    transport = httpx.MockTransport(
        lambda _: httpx.Response(
            200,
            headers={"Content-Type": "application/json"},
            stream=OversizeStream(),
        )
    )
    with pytest.raises(CaptureError, match="PROVIDER_RESPONSE_TOO_LARGE"):
        capture(
            "mineru",
            observed_at=_OBSERVED_AT,
            runtime=CaptureRuntime(environ=_KEYS, transport=transport),
        )
    assert chunks_read == 1


def test_evidence_omits_keys_dynamic_ids_urls_headers_vectors_and_input_text() -> None:
    snapshot = capture(
        "all",
        observed_at=_OBSERVED_AT,
        runtime=CaptureRuntime(environ=_KEYS, transport=_success_transport()),
    )
    evidence = canonical_json_bytes(snapshot)

    for forbidden in (
        *_KEYS.values(),
        "sensitive-jina-request-id",
        "sensitive-jina-rerank-request-id",
        "sensitive-mineru-batch-id",
        "sensitive-mineru-trace-id",
        "signed-upload.invalid",
        "sensitive-response-header-value",
        "MM Chat synthetic capture passage",
        "Deterministic capture uses synthetic provider inputs",
        "0.125,0.125",
    ):
        assert forbidden.encode() not in evidence
    assert b'"responseHeaderNames":["content-type"]' in evidence
    assert b'"signedUploadUrlCount":1' in evidence
    assert b'"batchIdPresent":true' in evidence


def test_mineru_submit_response_loss_is_single_call_unknown_submission() -> None:
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise httpx.ReadError("sensitive transport detail", request=request)

    snapshot = capture(
        "mineru",
        observed_at=_OBSERVED_AT,
        runtime=CaptureRuntime(
            environ=_KEYS,
            transport=httpx.MockTransport(handler),
        ),
    )
    evidence = canonical_json_bytes(snapshot)
    providers = cast("list[dict[str, Any]]", snapshot["providers"])
    budgets = cast("dict[str, dict[str, int]]", snapshot["budgets"])

    assert calls == 1
    assert providers[0]["state"] == "unknown_submission"
    assert providers[0]["operations"][0]["state"] == "unknown_submission"
    assert b"sensitive transport detail" not in evidence
    assert budgets["mineru"]["usedSubmitCalls"] == 1
    assert budgets["mineru"]["usedSignedUploadCalls"] == 0
    assert budgets["mineru"]["usedPollCalls"] == 0


def test_mineru_unknown_submission_writes_evidence_but_returns_nonzero(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise httpx.ReadError("sensitive detail", request=request)

    exit_code = main(
        ["--execute", "--provider", "mineru", "--output-dir", "capture"],
        runtime=CaptureRuntime(
            environ=_KEYS,
            transport=httpx.MockTransport(handler),
            output_base=tmp_path,
            now=_OBSERVED_AT,
        ),
    )

    output = capsys.readouterr()
    result = json.loads(output.out)
    evidence = json.loads((tmp_path / "capture" / result["evidenceFile"]).read_text())
    assert exit_code == 3
    assert calls == 1
    assert output.err == ""
    assert result["status"] == "evidence_written"
    assert result["captureOutcome"] == "unknown_submission"
    assert evidence["captureOutcome"] == "unknown_submission"


def test_mineru_is_staged_and_never_uploads_or_polls() -> None:
    requests: list[httpx.Request] = []
    snapshot = capture(
        "mineru",
        observed_at=_OBSERVED_AT,
        runtime=CaptureRuntime(
            environ=_KEYS,
            transport=_success_transport(requests),
        ),
    )
    providers = cast("list[dict[str, Any]]", snapshot["providers"])

    assert len(requests) == 1
    assert requests[0].method == "POST"
    assert requests[0].url == httpx.URL("https://mineru.net/api/v4/file-urls/batch")
    assert providers[0]["state"] == "staged_after_submit"
    assert deterministic_synthetic_pdf().startswith(b"%PDF-1.4")
    assert deterministic_synthetic_pdf() == deterministic_synthetic_pdf()


def test_canonical_evidence_bytes_and_hash_are_deterministic() -> None:
    first = capture(
        "all",
        observed_at=_OBSERVED_AT,
        runtime=CaptureRuntime(environ=_KEYS, transport=_success_transport()),
    )
    second = capture(
        "all",
        observed_at=_OBSERVED_AT,
        runtime=CaptureRuntime(environ=_KEYS, transport=_success_transport()),
    )
    first_bytes = canonical_json_bytes(first)
    second_bytes = canonical_json_bytes(second)

    assert first_bytes == second_bytes
    assert evidence_sha256(first_bytes) == evidence_sha256(second_bytes)
    assert len(evidence_sha256(first_bytes)) == 64


def test_evidence_schema_is_closed_before_write() -> None:
    snapshot = _valid_snapshot()
    snapshot["unexpected"] = "must-not-be-written"
    with pytest.raises(CaptureError, match="EVIDENCE_SCHEMA_INVALID"):
        validate_evidence_snapshot(snapshot)


def test_evidence_writer_creates_private_directory_and_file(tmp_path: Path) -> None:
    output_dir = tmp_path / "capture"
    target = write_evidence_snapshot(output_dir, _valid_snapshot())

    assert target.read_bytes().endswith(b"\n")
    assert stat.S_IMODE(output_dir.stat().st_mode) == 0o700
    assert stat.S_IMODE(target.stat().st_mode) == 0o600


def test_evidence_writer_refuses_existing_directory_without_overwrite(
    tmp_path: Path,
) -> None:
    output_dir = tmp_path / "capture"
    output_dir.mkdir()
    sentinel = output_dir / "sentinel"
    sentinel.write_text("unchanged", encoding="utf-8")

    with pytest.raises(CaptureError, match="EVIDENCE_TARGET_EXISTS"):
        write_evidence_snapshot(output_dir, _valid_snapshot())
    assert sentinel.read_text(encoding="utf-8") == "unchanged"


def test_evidence_writer_refuses_symlink_and_path_escape(tmp_path: Path) -> None:
    destination = tmp_path / "destination"
    destination.mkdir()
    symlink = tmp_path / "capture"
    symlink.symlink_to(destination, target_is_directory=True)

    with pytest.raises(CaptureError, match="EVIDENCE_TARGET_EXISTS"):
        write_evidence_snapshot(symlink, _valid_snapshot())
    with pytest.raises(CaptureError, match="OUTPUT_PARENT_INVALID"):
        write_evidence_snapshot(
            tmp_path / "safe" / ".." / "escape",
            _valid_snapshot(),
        )
    assert list(destination.iterdir()) == []


def test_evidence_writer_refuses_symlink_in_parent_chain(tmp_path: Path) -> None:
    destination = tmp_path / "destination"
    nested = destination / "nested"
    nested.mkdir(parents=True)
    parent_link = tmp_path / "parent-link"
    parent_link.symlink_to(destination, target_is_directory=True)

    with pytest.raises(CaptureError, match="OUTPUT_PARENT_INVALID"):
        write_evidence_snapshot(parent_link / "nested" / "capture", _valid_snapshot())
    assert list(nested.iterdir()) == []


def test_evidence_writer_no_overwrite_race_preserves_foreign_target(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    real_link = os.link
    sentinel = b"same-uid-race-sentinel"

    def racing_link(
        source: str,
        destination: str,
        *,
        src_dir_fd: int,
        dst_dir_fd: int,
        follow_symlinks: bool,
    ) -> None:
        target_fd = os.open(
            destination,
            os.O_WRONLY | os.O_CREAT | os.O_EXCL,
            0o600,
            dir_fd=dst_dir_fd,
        )
        with os.fdopen(target_fd, "wb") as target:
            target.write(sentinel)
        real_link(
            source,
            destination,
            src_dir_fd=src_dir_fd,
            dst_dir_fd=dst_dir_fd,
            follow_symlinks=follow_symlinks,
        )

    monkeypatch.setattr(os, "link", racing_link)
    output_dir = tmp_path / "capture"

    with pytest.raises(CaptureError, match="EVIDENCE_TARGET_EXISTS"):
        write_evidence_snapshot(output_dir, _valid_snapshot())

    assert (output_dir / "provider-capture-evidence.json").read_bytes() == sentinel
    assert not list(output_dir.glob("*.tmp"))


def test_execute_rejects_escaping_output_before_network(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    calls = 0

    def forbidden_network(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("invalid destination reached network")

    exit_code = main(
        ["--execute", "--provider", "mineru", "--output-dir", "../escape"],
        runtime=CaptureRuntime(
            environ=_KEYS,
            transport=httpx.MockTransport(forbidden_network),
            output_base=tmp_path,
            now=_OBSERVED_AT,
        ),
    )

    output = capsys.readouterr()
    assert exit_code == 2
    assert output.err.strip() == "OUTPUT_DIRECTORY_INVALID"
    assert calls == 0
    assert list(tmp_path.iterdir()) == []


def test_production_dispatch_remains_disabled_and_registries_empty() -> None:
    rag_root = Path(__file__).parents[2]
    handlers = (rag_root / "src/mm_chat_rag/handlers.py").read_text(encoding="utf-8")
    settings = (rag_root / "src/mm_chat_rag/settings.py").read_text(encoding="utf-8")
    assert "DISPATCH_REGISTRY: Final[Mapping[str, DispatchPlanner]] = {}" in handlers
    assert "JOB_HANDLER_REGISTRY: Final[Mapping[str, JobHandler]] = {}" in handlers
    assert '_boolean(env, "RAG_WORKER_DISPATCH_ENABLED", default=False)' in settings
