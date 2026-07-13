from __future__ import annotations

import json
import stat
from collections.abc import Callable
from pathlib import Path
from typing import cast

import httpx
import pytest
from tools.provider_capture import CaptureError, canonical_json_bytes
from tools.provider_capture_evidence import validate_evidence_snapshot
from tools.provider_capture_mineru_lifecycle import (
    LifecycleRuntime,
    capture_lifecycle,
    dry_run_plan,
    main,
)
from tools.provider_capture_mineru_lifecycle_http import POLL_CALL_LIMIT

from tests.support import provider_lifecycle as lifecycle_fixture

_KEY = lifecycle_fixture.KEY
_OBSERVED_AT = lifecycle_fixture.OBSERVED_AT
_RESULT_URL = lifecycle_fixture.RESULT_URL
_UPLOAD_URL = lifecycle_fixture.UPLOAD_URL
_allocate_payload = lifecycle_fixture.allocate_payload
_capture = lifecycle_fixture.capture_snapshot
_json_response = lifecycle_fixture.json_response
_lifecycle_transport = lifecycle_fixture.lifecycle_transport
_poll_payload = lifecycle_fixture.poll_payload
_response = lifecycle_fixture.response


def _assert_no_sensitive_evidence(evidence: bytes) -> None:
    for forbidden in (
        _KEY,
        _UPLOAD_URL,
        _RESULT_URL,
        "sensitive-lifecycle-batch-id",
        "sensitive-lifecycle-trace-id",
        "sensitive-lifecycle-poll-trace-id",
        "mm-chat-synthetic-capture.pdf",
        "synthetic markdown",
        "OSSAccessKeyId",
    ):
        assert forbidden.encode() not in evidence


def test_lifecycle_default_is_no_network_dry_run(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("dry-run reached network")

    exit_code = main(
        [],
        runtime=LifecycleRuntime(
            environ={},
            transport=httpx.MockTransport(forbidden),
            output_base=tmp_path,
        ),
    )
    output = json.loads(capsys.readouterr().out)

    assert exit_code == 0
    assert calls == 0
    assert list(tmp_path.iterdir()) == []
    assert output == dry_run_plan()
    assert output["networkEnabled"] is False
    assert output["operations"][2]["count"] == POLL_CALL_LIMIT


def test_lifecycle_missing_key_fails_before_network_or_output(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("missing key reached network")

    exit_code = main(
        ["--execute", "--output-dir", "capture"],
        runtime=LifecycleRuntime(
            environ={},
            transport=httpx.MockTransport(forbidden),
            output_base=tmp_path,
            now=_OBSERVED_AT,
        ),
    )

    output = capsys.readouterr()
    assert exit_code == 2
    assert output.err.strip() == "CAPTURE_CREDENTIALS_MISSING_OR_INVALID"
    assert output.out == ""
    assert calls == 0
    assert list(tmp_path.iterdir()) == []


@pytest.mark.parametrize(
    ("environment", "expected_proxy"),
    [
        (
            {
                "ALL_PROXY": "http://10.0.0.3:7890",
                "HTTPS_PROXY": "http://10.0.0.3:7890",
                "HTTP_PROXY": "http://10.0.0.3:7890",
            },
            None,
        ),
        (
            {"PROVIDER_CAPTURE_PROXY_URL": "http://172.16.0.2:7890"},
            "http://172.16.0.2:7890",
        ),
    ],
)
def test_lifecycle_proxy_is_explicit_private_and_never_evidenced(
    monkeypatch: pytest.MonkeyPatch,
    environment: dict[str, str],
    expected_proxy: str | None,
) -> None:
    real_client = httpx.Client
    seen: dict[str, object] = {}

    def client_factory(**kwargs: object) -> httpx.Client:
        seen["proxy"] = kwargs.pop("proxy")
        seen["trust_env"] = kwargs["trust_env"]
        seen["follow_redirects"] = kwargs["follow_redirects"]
        kwargs["transport"] = _lifecycle_transport()
        constructor = cast("Callable[..., httpx.Client]", real_client)
        return constructor(**kwargs)

    monkeypatch.setattr(
        "tools.provider_capture_mineru_lifecycle.httpx.Client",
        client_factory,
    )
    snapshot = capture_lifecycle(
        observed_at=_OBSERVED_AT,
        runtime=LifecycleRuntime(
            environ={"MINERU_API_KEY": _KEY} | environment,
            sleeper=lambda _: None,
        ),
    )
    evidence = canonical_json_bytes(snapshot)

    assert seen == {
        "follow_redirects": False,
        "proxy": expected_proxy,
        "trust_env": False,
    }
    assert b"172.16.0.2" not in evidence
    assert b"10.0.0.3" not in evidence


def test_lifecycle_invalid_proxy_and_argv_never_echo_values(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    proxy = "http://user:secret@10.0.0.2:7890"
    exit_code = main(
        ["--execute", "--output-dir", "capture"],
        runtime=LifecycleRuntime(
            environ={
                "MINERU_API_KEY": _KEY,
                "PROVIDER_CAPTURE_PROXY_URL": proxy,
            },
            transport=_lifecycle_transport(),
            output_base=tmp_path,
            now=_OBSERVED_AT,
        ),
    )
    output = capsys.readouterr()
    assert exit_code == 2
    assert output.err.strip() == "CAPTURE_PROXY_INVALID"
    assert proxy not in output.err
    assert list(tmp_path.iterdir()) == []

    secret_argv = "--sensitive-stage-value"
    exit_code = main([secret_argv])
    output = capsys.readouterr()
    assert exit_code == 2
    assert output.err.strip() == "CLI_ARGUMENT_INVALID"
    assert secret_argv not in output.err


def test_lifecycle_invalid_output_is_rejected_before_network(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise AssertionError("invalid output reached network")

    exit_code = main(
        ["--execute", "--output-dir", "../escape"],
        runtime=LifecycleRuntime(
            environ={"MINERU_API_KEY": _KEY},
            transport=httpx.MockTransport(forbidden),
            output_base=tmp_path,
            now=_OBSERVED_AT,
        ),
    )
    output = capsys.readouterr()
    assert exit_code == 2
    assert output.err.strip() == "OUTPUT_DIRECTORY_INVALID"
    assert calls == 0
    assert list(tmp_path.iterdir()) == []


def test_complete_lifecycle_has_fixed_calls_headers_and_redacted_evidence() -> None:
    requests: list[httpx.Request] = []
    sleeps: list[float] = []
    snapshot = _capture(
        _lifecycle_transport(requests),
        sleeper=sleeps.append,
    )
    evidence = canonical_json_bytes(snapshot)

    assert [request.method for request in requests] == [
        "POST",
        "PUT",
        "GET",
        "GET",
        "GET",
    ]
    assert sleeps == [5.0]
    assert requests[1].content.startswith(b"%PDF-1.4")
    assert "authorization" not in requests[1].headers
    assert "content-type" not in requests[1].headers
    assert "cookie" not in requests[1].headers
    assert requests[2].headers["authorization"] == f"Bearer {_KEY}"
    assert "content-type" not in requests[2].headers
    assert "authorization" not in requests[-1].headers
    assert "cookie" not in requests[-1].headers
    assert all(request.headers["accept-encoding"] == "identity" for request in requests)
    assert snapshot["captureOutcome"] == "lifecycle_complete"
    assert snapshot["budgets"]["mineru"] == {
        "allowedAllocateCalls": 1,
        "allowedDownloadCalls": 1,
        "allowedPollCalls": POLL_CALL_LIMIT,
        "allowedUploadCalls": 1,
        "usedAllocateCalls": 1,
        "usedDownloadCalls": 1,
        "usedPollCalls": 2,
        "usedUploadCalls": 1,
    }
    _assert_no_sensitive_evidence(evidence)
    validate_evidence_snapshot(snapshot)


def test_complete_cli_writes_private_v2_evidence(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    exit_code = main(
        ["--execute", "--output-dir", "capture"],
        runtime=LifecycleRuntime(
            environ={"MINERU_API_KEY": _KEY},
            transport=_lifecycle_transport(),
            output_base=tmp_path,
            now=_OBSERVED_AT,
            sleeper=lambda _: None,
        ),
    )
    result = json.loads(capsys.readouterr().out)
    evidence_path = tmp_path / "capture" / result["evidenceFile"]
    evidence = json.loads(evidence_path.read_text(encoding="utf-8"))

    assert exit_code == 0
    assert result["captureOutcome"] == "lifecycle_complete"
    assert evidence["schemaVersion"] == "mm-chat.provider-capture-evidence.v2"
    assert stat.S_IMODE(evidence_path.parent.stat().st_mode) == 0o700
    assert stat.S_IMODE(evidence_path.stat().st_mode) == 0o600


def test_lifecycle_evidence_is_deterministic_for_fixed_responses() -> None:
    first = _capture(_lifecycle_transport())
    second = _capture(_lifecycle_transport())

    assert canonical_json_bytes(first) == canonical_json_bytes(second)


def test_rejected_upload_target_never_receives_pdf_and_writes_incomplete_evidence(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    requests: list[httpx.Request] = []
    exit_code = main(
        ["--execute", "--output-dir", "capture"],
        runtime=LifecycleRuntime(
            environ={"MINERU_API_KEY": _KEY},
            transport=_lifecycle_transport(
                requests,
                upload_url="https://evil.invalid/api-upload/a?signature=secret",
            ),
            output_base=tmp_path,
            now=_OBSERVED_AT,
            sleeper=lambda _: None,
        ),
    )
    result = json.loads(capsys.readouterr().out)
    evidence = (tmp_path / "capture" / result["evidenceFile"]).read_bytes()

    assert exit_code == 3
    assert len(requests) == 1
    assert result["captureOutcome"] == "upload_target_rejected"
    assert b"evil.invalid" not in evidence
    assert b"signature" not in evidence.lower()


def test_upload_response_loss_stops_without_poll_or_retry() -> None:
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        if request.method == "POST":
            return _json_response(_allocate_payload())
        raise httpx.ReadError("sensitive upload transport detail", request=request)

    snapshot = _capture(httpx.MockTransport(handler))
    evidence = canonical_json_bytes(snapshot)

    assert calls == 2
    assert snapshot["captureOutcome"] == "unknown_upload"
    assert snapshot["budgets"]["mineru"]["usedUploadCalls"] == 1
    assert snapshot["budgets"]["mineru"]["usedPollCalls"] == 0
    assert (
        snapshot["providers"][0]["operations"][1]["transportFailureClass"]
        == "read_error"
    )
    assert b"sensitive upload transport detail" not in evidence


def test_poll_exhaustion_is_bounded_and_never_downloads() -> None:
    requests: list[httpx.Request] = []
    sleeps: list[float] = []
    snapshot = _capture(
        _lifecycle_transport(
            requests,
            poll_states=["pending"] * POLL_CALL_LIMIT,
        ),
        sleeper=sleeps.append,
    )

    assert snapshot["captureOutcome"] == "poll_exhausted"
    assert len(requests) == 2 + POLL_CALL_LIMIT
    assert len(sleeps) == POLL_CALL_LIMIT - 1
    assert snapshot["budgets"]["mineru"]["usedPollCalls"] == POLL_CALL_LIMIT
    assert snapshot["budgets"]["mineru"]["usedDownloadCalls"] == 0


def test_poll_response_loss_records_one_unknown_call_without_retry() -> None:
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        if request.method == "POST":
            return _json_response(_allocate_payload())
        if request.method == "PUT":
            return _response(b"")
        raise httpx.ReadError("sensitive poll transport detail", request=request)

    snapshot = _capture(httpx.MockTransport(handler))
    evidence = canonical_json_bytes(snapshot)

    assert calls == 3
    assert snapshot["captureOutcome"] == "unknown_poll"
    assert snapshot["budgets"]["mineru"]["usedPollCalls"] == 1
    assert (
        snapshot["providers"][0]["operations"][2]["transportFailureClass"]
        == "read_error"
    )
    assert b"sensitive poll transport detail" not in evidence


@pytest.mark.parametrize(
    ("error_factory", "expected_class"),
    [
        (
            lambda request: httpx.ConnectTimeout(
                "sensitive connect timeout", request=request
            ),
            "connect_timeout",
        ),
        (
            lambda request: httpx.ReadTimeout(
                "sensitive read timeout", request=request
            ),
            "read_timeout",
        ),
        (
            lambda request: httpx.WriteTimeout(
                "sensitive write timeout", request=request
            ),
            "write_timeout",
        ),
        (
            lambda request: httpx.PoolTimeout(
                "sensitive pool timeout", request=request
            ),
            "pool_timeout",
        ),
        (
            lambda request: httpx.ConnectError(
                "sensitive connect error", request=request
            ),
            "connect_error",
        ),
        (
            lambda request: httpx.ReadError("sensitive read error", request=request),
            "read_error",
        ),
        (
            lambda request: httpx.WriteError("sensitive write error", request=request),
            "write_error",
        ),
        (
            lambda request: httpx.CloseError("sensitive close error", request=request),
            "close_error",
        ),
        (
            lambda request: httpx.LocalProtocolError(
                "sensitive local protocol", request=request
            ),
            "local_protocol_error",
        ),
        (
            lambda request: httpx.RemoteProtocolError(
                "sensitive protocol", request=request
            ),
            "remote_protocol_error",
        ),
        (
            lambda request: httpx.ProxyError("sensitive proxy", request=request),
            "proxy_error",
        ),
        (
            lambda request: httpx.UnsupportedProtocol(
                "sensitive unsupported protocol", request=request
            ),
            "unsupported_protocol",
        ),
        (
            lambda request: httpx.TransportError(
                "sensitive future transport", request=request
            ),
            "other_transport_error",
        ),
    ],
)
def test_download_response_loss_records_only_closed_transport_class(
    error_factory: Callable[[httpx.Request], httpx.TransportError],
    expected_class: str,
) -> None:
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        if request.method == "POST":
            return _json_response(_allocate_payload())
        if request.method == "PUT":
            return _response(b"")
        if request.url.host == "mineru.net":
            return _json_response(_poll_payload("done"))
        raise error_factory(request)

    snapshot = _capture(httpx.MockTransport(handler))
    evidence = canonical_json_bytes(snapshot)
    download = snapshot["providers"][0]["operations"][3]

    assert calls == 4
    assert snapshot["captureOutcome"] == "unknown_download"
    assert snapshot["budgets"]["mineru"]["usedDownloadCalls"] == 1
    assert download["transportFailureClass"] == expected_class
    assert b"sensitive" not in evidence
    validate_evidence_snapshot(snapshot)

    legacy = json.loads(evidence)
    legacy["providers"][0]["operations"][3].pop("transportFailureClass")
    validate_evidence_snapshot(legacy)

    for invalid_class in (None, "dynamic_transport_detail"):
        invalid = json.loads(evidence)
        invalid["providers"][0]["operations"][3]["transportFailureClass"] = (
            invalid_class
        )
        with pytest.raises(CaptureError, match="EVIDENCE_SCHEMA_INVALID"):
            validate_evidence_snapshot(invalid)


def test_non_transport_download_error_is_not_misclassified_or_evidenced() -> None:
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        if request.method == "POST":
            return _json_response(_allocate_payload())
        if request.method == "PUT":
            return _response(b"")
        if request.url.host == "mineru.net":
            return _json_response(_poll_payload("done"))
        raise RuntimeError("sensitive programming failure")

    with pytest.raises(RuntimeError, match="sensitive programming failure"):
        _capture(httpx.MockTransport(handler))
    assert calls == 4


def test_unknown_download_cli_is_terminal_and_existing_output_blocks_network(
    tmp_path: Path,
    capsys: pytest.CaptureFixture[str],
) -> None:
    requests: list[httpx.Request] = []

    def lost_download(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.method == "POST":
            return _json_response(_allocate_payload())
        if request.method == "PUT":
            return _response(b"")
        if request.url.host == "mineru.net":
            return _json_response(_poll_payload("done"))
        raise httpx.ReadError("sensitive download detail", request=request)

    exit_code = main(
        ["--execute", "--output-dir", "capture"],
        runtime=LifecycleRuntime(
            environ={"MINERU_API_KEY": _KEY},
            transport=httpx.MockTransport(lost_download),
            output_base=tmp_path,
            now=_OBSERVED_AT,
            sleeper=lambda _: None,
        ),
    )
    result = json.loads(capsys.readouterr().out)
    evidence_path = tmp_path / "capture" / result["evidenceFile"]
    evidence = json.loads(evidence_path.read_text(encoding="utf-8"))

    assert exit_code == 3
    assert len(requests) == 4
    assert result["captureOutcome"] == "unknown_download"
    assert (
        evidence["providers"][0]["operations"][3]["transportFailureClass"]
        == "read_error"
    )
    assert "sensitive download detail" not in evidence_path.read_text(encoding="utf-8")

    blocked_calls = 0

    def forbidden(_: httpx.Request) -> httpx.Response:
        nonlocal blocked_calls
        blocked_calls += 1
        raise AssertionError("existing output reached network")

    second_exit = main(
        ["--execute", "--output-dir", "capture"],
        runtime=LifecycleRuntime(
            environ={"MINERU_API_KEY": _KEY},
            transport=httpx.MockTransport(forbidden),
            output_base=tmp_path,
            now=_OBSERVED_AT,
        ),
    )
    output = capsys.readouterr()

    assert second_exit == 2
    assert output.out == ""
    assert output.err.strip() == "EVIDENCE_TARGET_EXISTS"
    assert blocked_calls == 0


@pytest.mark.parametrize(
    "mutation",
    ["wrong_batch", "wrong_file", "bad_data_id", "bad_start_time", "extra_field"],
)
def test_malformed_poll_identity_and_shape_fail_closed(mutation: str) -> None:
    calls = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        if request.method == "POST":
            return _json_response(_allocate_payload())
        if request.method == "PUT":
            return _response(b"")
        payload = _poll_payload("running")
        data = payload["data"]
        result = data["extract_result"][0]
        if mutation == "wrong_batch":
            data["batch_id"] = "different-batch"
        elif mutation == "wrong_file":
            result["file_name"] = "different.pdf"
        elif mutation == "bad_data_id":
            result["data_id"] = "../unsafe"
        elif mutation == "bad_start_time":
            result["extract_progress"]["start_time"] = "not-a-provider-time"
        else:
            result["unexpected"] = True
        return _json_response(payload)

    snapshot = _capture(httpx.MockTransport(handler))

    assert calls == 3
    assert snapshot["captureOutcome"] == "poll_failed"
    assert snapshot["budgets"]["mineru"]["usedPollCalls"] == 1


def test_result_redirect_is_not_followed_and_is_recorded_as_failed() -> None:
    requests: list[httpx.Request] = []

    def handler(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        if request.method == "POST":
            return _json_response(_allocate_payload())
        if request.method == "PUT":
            return _response(b"")
        if request.url.host == "mineru.net":
            return _json_response(_poll_payload("done"))
        return _response(
            b"",
            status=302,
            location="https://evil.invalid/steal",
        )

    snapshot = _capture(httpx.MockTransport(handler))

    assert snapshot["captureOutcome"] == "download_failed"
    assert len(requests) == 4
    assert all(request.url.host != "evil.invalid" for request in requests)


def test_parse_failure_redacts_provider_error_and_stops() -> None:
    snapshot = _capture(
        _lifecycle_transport(poll_states=["failed"]),
    )
    evidence = canonical_json_bytes(snapshot)

    assert snapshot["captureOutcome"] == "parse_failed"
    assert b"err_msg" not in evidence
    assert b"full_zip_url" not in evidence


def test_lifecycle_evidence_schema_rejects_dynamic_or_inconsistent_values() -> None:
    snapshot = _capture(_lifecycle_transport())
    dynamic = json.loads(canonical_json_bytes(snapshot))
    dynamic["providers"][0]["operations"][1]["target"] = _UPLOAD_URL
    with pytest.raises(CaptureError, match="EVIDENCE_SCHEMA_INVALID"):
        validate_evidence_snapshot(dynamic)

    inconsistent = json.loads(canonical_json_bytes(snapshot))
    inconsistent["budgets"]["mineru"]["usedPollCalls"] = 59
    with pytest.raises(CaptureError, match="EVIDENCE_SCHEMA_INVALID"):
        validate_evidence_snapshot(inconsistent)

    misplaced = json.loads(canonical_json_bytes(snapshot))
    misplaced["providers"][0]["operations"][3]["transportFailureClass"] = "read_error"
    with pytest.raises(CaptureError, match="EVIDENCE_SCHEMA_INVALID"):
        validate_evidence_snapshot(misplaced)
