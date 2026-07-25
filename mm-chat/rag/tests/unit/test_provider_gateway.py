"""Closed transport boundary tests for the Go provider gateway."""

from __future__ import annotations

from collections.abc import Callable

import httpx
import pytest

import mm_chat_rag.provider_gateway as gateway_module
from mm_chat_rag.provider_gateway import (
    GO_PROVIDER_GATEWAY_CONFIG_INVALID,
    GO_PROVIDER_GATEWAY_REQUEST_FAILED,
    GO_PROVIDER_GATEWAY_RESPONSE_INVALID,
    GO_PROVIDER_GATEWAY_RESPONSE_TOO_LARGE,
    GO_PROVIDER_GATEWAY_STATUS_INVALID,
    GoProviderGateway,
)
from mm_chat_rag.retry import PermanentJobError, RetryableJobError

BASE_URL = "http://backend:8080"
INTERNAL_HEADER_VALUE = "unit-test-provider-gateway-value"
PATH = "/internal/rag/providers/siliconflow/embeddings"


def _response(
    *,
    status: int = 200,
    body: bytes = b"{}",
    content_type: str = "application/json",
    content_encoding: str = "identity",
) -> httpx.Response:
    return httpx.Response(
        status,
        headers={
            "Content-Type": content_type,
            "Content-Encoding": content_encoding,
        },
        stream=httpx.ByteStream(body),
    )


async def _post_with_handler(
    handler: Callable[[httpx.Request], httpx.Response],
    *,
    max_response_bytes: int = 1024,
) -> dict[str, object]:
    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        return await GoProviderGateway(
            base_url=BASE_URL,
            internal_token=INTERNAL_HEADER_VALUE,
            client=client,
        ).post_json(PATH, {"input": "bounded"}, max_response_bytes=max_response_bytes)


@pytest.mark.parametrize(
    ("base_url", "token"),
    [
        ("", INTERNAL_HEADER_VALUE),
        (" http://backend:8080", INTERNAL_HEADER_VALUE),
        ("ftp://backend:8080", INTERNAL_HEADER_VALUE),
        ("http://user@backend:8080", INTERNAL_HEADER_VALUE),
        ("http://backend:8080?secret=x", INTERNAL_HEADER_VALUE),
        ("http://backend:8080#fragment", INTERNAL_HEADER_VALUE),
        ("http://[", INTERNAL_HEADER_VALUE),
        (BASE_URL, ""),
        (BASE_URL, " spaced "),
        (BASE_URL, "contains\nnewline"),
        (BASE_URL, "x" * 4097),
    ],
)
def test_provider_gateway_rejects_unsafe_configuration(
    base_url: str,
    token: str,
) -> None:
    with pytest.raises(PermanentJobError) as raised:
        GoProviderGateway(base_url=base_url, internal_token=token)
    assert raised.value.error_code == GO_PROVIDER_GATEWAY_CONFIG_INVALID


@pytest.mark.parametrize(
    "path",
    [
        "/v1/embeddings",
        "internal/rag/providers/siliconflow/embeddings",
        "/internal/rag/providers/siliconflow/embeddings?override=true",
        "/internal/rag/providers/siliconflow/embeddings#fragment",
        "/internal/rag/providers/siliconflow\\embeddings",
    ],
)
async def test_provider_gateway_rejects_non_owned_operation_paths(path: str) -> None:
    gateway = GoProviderGateway(
        base_url=BASE_URL,
        internal_token=INTERNAL_HEADER_VALUE,
    )
    with pytest.raises(PermanentJobError) as raised:
        await gateway.post_json(path, {}, max_response_bytes=1024)
    assert raised.value.error_code == GO_PROVIDER_GATEWAY_CONFIG_INVALID


@pytest.mark.parametrize("maximum", [0, 16 * 1024 * 1024 + 1])
async def test_provider_gateway_rejects_unbounded_response_limits(maximum: int) -> None:
    with pytest.raises(PermanentJobError) as raised:
        await _post_with_handler(lambda _: _response(), max_response_bytes=maximum)
    assert raised.value.error_code == GO_PROVIDER_GATEWAY_CONFIG_INVALID


@pytest.mark.parametrize("status", [404, 409, 429, 502, 503, 504])
async def test_provider_gateway_retries_only_transient_statuses(status: int) -> None:
    with pytest.raises(RetryableJobError) as raised:
        await _post_with_handler(lambda _: _response(status=status))
    assert raised.value.error_code == GO_PROVIDER_GATEWAY_STATUS_INVALID
    assert raised.value.retry_after_seconds == 30


async def test_provider_gateway_rejects_non_retryable_status() -> None:
    with pytest.raises(PermanentJobError) as raised:
        await _post_with_handler(lambda _: _response(status=401))
    assert raised.value.error_code == GO_PROVIDER_GATEWAY_STATUS_INVALID


@pytest.mark.parametrize(
    ("body", "content_type", "content_encoding"),
    [
        (b"{}", "text/plain", "identity"),
        (b"{}", "application/json", "gzip"),
        (b"{", "application/json", "identity"),
        (b"\xff", "application/json", "identity"),
        (b"[]", "application/json", "identity"),
    ],
)
async def test_provider_gateway_rejects_malformed_response_envelopes(
    body: bytes,
    content_type: str,
    content_encoding: str,
) -> None:
    with pytest.raises(PermanentJobError) as raised:
        await _post_with_handler(
            lambda _: _response(
                body=body,
                content_type=content_type,
                content_encoding=content_encoding,
            )
        )
    assert raised.value.error_code == GO_PROVIDER_GATEWAY_RESPONSE_INVALID


async def test_provider_gateway_rejects_streamed_and_consumed_oversize_bodies() -> None:
    with pytest.raises(PermanentJobError) as streamed:
        await _post_with_handler(
            lambda _: _response(body=b'{"tooLarge":true}'),
            max_response_bytes=2,
        )
    assert streamed.value.error_code == GO_PROVIDER_GATEWAY_RESPONSE_TOO_LARGE

    response = httpx.Response(
        200,
        headers={"Content-Type": "application/json"},
        content=b"oversized",
    )
    assert response.is_stream_consumed
    with pytest.raises(PermanentJobError) as consumed:
        await gateway_module._read_bounded_response(response, 2)
    assert consumed.value.error_code == GO_PROVIDER_GATEWAY_RESPONSE_TOO_LARGE


async def test_provider_gateway_maps_transport_errors_without_detail_leakage() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        raise httpx.ReadError(
            f"sensitive {INTERNAL_HEADER_VALUE}",
            request=request,
        )

    with pytest.raises(RetryableJobError) as raised:
        await _post_with_handler(handler)
    assert raised.value.error_code == GO_PROVIDER_GATEWAY_REQUEST_FAILED
    assert INTERNAL_HEADER_VALUE not in str(raised.value)


async def test_provider_gateway_constructs_closed_default_client(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    requests: list[httpx.Request] = []
    real_client = httpx.AsyncClient(
        transport=httpx.MockTransport(
            lambda request: requests.append(request) or _response(body=b'{"ok":true}')
        )
    )

    def fake_client(**kwargs: object) -> httpx.AsyncClient:
        assert kwargs["follow_redirects"] is False
        assert kwargs["trust_env"] is False
        return real_client

    monkeypatch.setattr(gateway_module.httpx, "AsyncClient", fake_client)
    result = await GoProviderGateway(
        base_url=f"{BASE_URL}/",
        internal_token=INTERNAL_HEADER_VALUE,
    ).post_json(PATH, {}, max_response_bytes=1024)

    assert result == {"ok": True}
    assert requests[0].url == httpx.URL(f"{BASE_URL}{PATH}")
