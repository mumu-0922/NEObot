"""Fixed-budget HTTP capture operations and response-shape validation."""

from __future__ import annotations

import hashlib
from typing import Final, cast
from urllib.parse import urlsplit

import httpx

from tools.provider_capture_common import (
    HTTP_OK,
    JINA_EMBED_URL,
    JINA_MODEL,
    JINA_RERANK_MODEL,
    JINA_RERANK_URL,
    MAX_RESPONSE_BYTES,
    MINERU_BATCH_URL,
    RERANK_DOCUMENT_COUNT,
    SYNTHETIC_DOCUMENTS,
    SYNTHETIC_PASSAGE,
    SYNTHETIC_PDF_NAME,
    SYNTHETIC_QUERY,
    CaptureError,
    JsonObject,
    JsonValue,
    canonical_json_bytes,
    finite_number,
    json_list,
    json_object,
    nonnegative_int,
    request_hash,
    strict_json_object,
    validate_request_target,
)

TIMEOUT: Final = httpx.Timeout(connect=5.0, read=15.0, write=15.0, pool=5.0)
LIMITS: Final = httpx.Limits(max_connections=1, max_keepalive_connections=1)
_HTTP_REDIRECT_MIN: Final = 300
_HTTP_REDIRECT_MAX: Final = 400
_CONTENT_TYPE_PART_LIMIT: Final = 2


def capture_jina(client: httpx.Client, api_key: str) -> JsonObject:
    """Run the immutable three-call Jina plan."""
    operations: list[JsonValue] = [
        _capture_jina_embedding(client, api_key, dimensions)
        for dimensions in (1024, 2048)
    ]
    operations.append(_capture_jina_rerank(client, api_key))
    return {
        "operationCount": 3,
        "operations": operations,
        "provider": "jina",
        "state": "captured",
    }


def capture_mineru_submit(
    client: httpx.Client,
    api_key: str,
    pdf: bytes,
) -> JsonObject:
    """Submit once, never retry, upload, or poll."""
    request_body: JsonObject = {
        "enable_formula": True,
        "enable_table": True,
        "files": [{"name": SYNTHETIC_PDF_NAME}],
        "is_ocr": True,
        "model_version": "vlm",
    }
    body_hash = request_hash(request_body)
    try:
        response, metadata = send_json(
            client,
            "POST",
            MINERU_BATCH_URL,
            request_body,
            api_key,
        )
    except CaptureError as error:
        if str(error) != "PROVIDER_RESPONSE_LOST":
            raise
        return _unknown_mineru_submission(body_hash)
    shape = _validate_mineru_submit_response(response)
    operation: JsonObject = {
        "method": "POST",
        "operation": "local_upload_batch_submit",
        "path": "/api/v4/file-urls/batch",
        "requestBodySha256": body_hash,
        "response": shape,
        "state": "staged_after_submit",
        **metadata,
    }
    return {
        "operationCount": 1,
        "operations": [operation],
        "provider": "mineru",
        "state": "staged_after_submit",
        "syntheticPdfByteCount": len(pdf),
        "syntheticPdfSha256": hashlib.sha256(pdf).hexdigest(),
    }


def send_json(
    client: httpx.Client,
    method: str,
    url: str,
    body: JsonObject,
    api_key: str,
) -> tuple[JsonObject, JsonObject]:
    """Send one strict JSON request and consume bounded raw response bytes."""
    validate_request_target(method, url)
    content = canonical_json_bytes(body).removesuffix(b"\n")
    headers = {
        "Accept": "application/json",
        "Accept-Encoding": "identity",
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }
    client.cookies.clear()
    try:
        with client.stream(method, url, headers=headers, content=content) as response:
            validate_request_target(response.request.method, str(response.request.url))
            _validate_response_headers(response)
            raw = _read_bounded_raw_response(response)
    except CaptureError:
        raise
    except Exception:  # noqa: BLE001 - never retain transport implementation detail.
        raise CaptureError("PROVIDER_RESPONSE_LOST") from None
    return strict_json_object(raw), _response_metadata()


def _capture_jina_embedding(
    client: httpx.Client,
    api_key: str,
    dimensions: int,
) -> JsonObject:
    request_body: JsonObject = {
        "dimensions": dimensions,
        "embedding_type": "float",
        "input": [{"text": SYNTHETIC_PASSAGE}],
        "late_chunking": False,
        "model": JINA_MODEL,
        "return_multivector": False,
        "return_tokenized_input": False,
        "task": "retrieval.passage",
        "truncate": False,
    }
    response, metadata = send_json(
        client,
        "POST",
        JINA_EMBED_URL,
        request_body,
        api_key,
    )
    return {
        "method": "POST",
        "operation": f"embedding_{dimensions}",
        "path": "/v1/embeddings",
        "requestBodySha256": request_hash(request_body),
        "response": _validate_embedding_response(response, dimensions),
        **metadata,
    }


def _capture_jina_rerank(client: httpx.Client, api_key: str) -> JsonObject:
    request_body: JsonObject = {
        "documents": list(SYNTHETIC_DOCUMENTS),
        "model": JINA_RERANK_MODEL,
        "query": SYNTHETIC_QUERY,
        "return_documents": False,
        "return_embeddings": False,
        "top_n": 2,
    }
    response, metadata = send_json(
        client,
        "POST",
        JINA_RERANK_URL,
        request_body,
        api_key,
    )
    return {
        "method": "POST",
        "operation": "rerank",
        "path": "/v1/rerank",
        "requestBodySha256": request_hash(request_body),
        "response": _validate_rerank_response(response),
        **metadata,
    }


def _unknown_mineru_submission(body_hash: str) -> JsonObject:
    return {
        "operationCount": 1,
        "operations": [
            {
                "method": "POST",
                "operation": "local_upload_batch_submit",
                "path": "/api/v4/file-urls/batch",
                "requestBodySha256": body_hash,
                "state": "unknown_submission",
            }
        ],
        "provider": "mineru",
        "state": "unknown_submission",
    }


def _validate_response_headers(response: httpx.Response) -> None:
    status_code = response.status_code
    if _HTTP_REDIRECT_MIN <= status_code < _HTTP_REDIRECT_MAX:
        raise CaptureError("REDIRECT_FORBIDDEN")
    if status_code != HTTP_OK:
        raise CaptureError("PROVIDER_STATUS_INVALID")
    _normalized_json_content_type(response.headers.get("content-type"))
    encoding = response.headers.get("content-encoding")
    if encoding is not None and encoding.strip().lower() != "identity":
        raise CaptureError("PROVIDER_CONTENT_ENCODING_INVALID")
    length = response.headers.get("content-length")
    if length is not None:
        _validate_content_length(length)


def _validate_content_length(value: str) -> None:
    if not value.isascii() or not value.isdecimal():
        raise CaptureError("PROVIDER_CONTENT_LENGTH_INVALID")
    if int(value) > MAX_RESPONSE_BYTES:
        raise CaptureError("PROVIDER_RESPONSE_TOO_LARGE")


def _read_bounded_raw_response(response: httpx.Response) -> bytes:
    raw = bytearray()
    for chunk in response.iter_raw():
        if len(raw) + len(chunk) > MAX_RESPONSE_BYTES:
            raise CaptureError("PROVIDER_RESPONSE_TOO_LARGE")
        raw.extend(chunk)
    return bytes(raw)


def _normalized_json_content_type(value: str | None) -> str:
    if value is None:
        raise CaptureError("PROVIDER_CONTENT_TYPE_INVALID")
    parts = [part.strip().lower() for part in value.split(";")]
    if parts[0] != "application/json" or len(parts) > _CONTENT_TYPE_PART_LIMIT:
        raise CaptureError("PROVIDER_CONTENT_TYPE_INVALID")
    if len(parts) == _CONTENT_TYPE_PART_LIMIT and parts[1] not in {
        "charset=utf-8",
        'charset="utf-8"',
    }:
        raise CaptureError("PROVIDER_CONTENT_TYPE_INVALID")
    return "application/json"


def _response_metadata() -> JsonObject:
    return {
        "httpStatus": HTTP_OK,
        "responseContentType": "application/json",
        "responseHeaderNames": ["content-type"],
    }


def _validate_embedding_response(payload: JsonObject, dimensions: int) -> JsonObject:
    if payload.get("model") != JINA_MODEL:
        raise CaptureError("JINA_EMBEDDING_SHAPE_INVALID")
    data = json_list(payload.get("data"), "JINA_EMBEDDING_SHAPE_INVALID")
    if len(data) != 1:
        raise CaptureError("JINA_EMBEDDING_SHAPE_INVALID")
    item = json_object(data[0], "JINA_EMBEDDING_SHAPE_INVALID")
    index = nonnegative_int(item.get("index"), "JINA_EMBEDDING_SHAPE_INVALID")
    vector = json_list(item.get("embedding"), "JINA_EMBEDDING_SHAPE_INVALID")
    if index != 0 or len(vector) != dimensions:
        raise CaptureError("JINA_EMBEDDING_SHAPE_INVALID")
    if any(not finite_number(value) for value in vector):
        raise CaptureError("JINA_EMBEDDING_SHAPE_INVALID")
    usage = json_object(payload.get("usage"), "JINA_EMBEDDING_SHAPE_INVALID")
    total_tokens = nonnegative_int(
        usage.get("total_tokens"), "JINA_EMBEDDING_SHAPE_INVALID"
    )
    prompt_value = usage.get("prompt_tokens")
    prompt_tokens = (
        nonnegative_int(prompt_value, "JINA_EMBEDDING_SHAPE_INVALID")
        if prompt_value is not None
        else None
    )
    return {
        "indexes": [index],
        "itemCount": 1,
        "model": JINA_MODEL,
        "usage": {"promptTokens": prompt_tokens, "totalTokens": total_tokens},
        "vectorDimension": dimensions,
    }


def _validate_rerank_response(payload: JsonObject) -> JsonObject:
    if payload.get("model") != JINA_RERANK_MODEL:
        raise CaptureError("JINA_RERANK_SHAPE_INVALID")
    results = json_list(payload.get("results"), "JINA_RERANK_SHAPE_INVALID")
    if len(results) != RERANK_DOCUMENT_COUNT:
        raise CaptureError("JINA_RERANK_SHAPE_INVALID")
    indexes: list[JsonValue] = []
    scores: list[JsonValue] = []
    for raw_result in results:
        result = json_object(raw_result, "JINA_RERANK_SHAPE_INVALID")
        index = nonnegative_int(result.get("index"), "JINA_RERANK_SHAPE_INVALID")
        score = result.get("relevance_score")
        if index not in {0, 1} or not finite_number(score):
            raise CaptureError("JINA_RERANK_SHAPE_INVALID")
        indexes.append(index)
        scores.append(cast("int | float", score))
    if set(cast("list[int]", indexes)) != {0, 1}:
        raise CaptureError("JINA_RERANK_SHAPE_INVALID")
    usage = json_object(payload.get("usage"), "JINA_RERANK_SHAPE_INVALID")
    total_tokens = nonnegative_int(
        usage.get("total_tokens"), "JINA_RERANK_SHAPE_INVALID"
    )
    return {
        "indexes": indexes,
        "model": JINA_RERANK_MODEL,
        "resultCount": RERANK_DOCUMENT_COUNT,
        "scores": scores,
        "usage": {"totalTokens": total_tokens},
    }


def _validate_mineru_submit_response(payload: JsonObject) -> JsonObject:
    if payload.get("code") != 0:
        raise CaptureError("MINERU_SUBMIT_SHAPE_INVALID")
    data = json_object(payload.get("data"), "MINERU_SUBMIT_SHAPE_INVALID")
    batch_id = data.get("batch_id")
    file_urls = json_list(data.get("file_urls"), "MINERU_SUBMIT_SHAPE_INVALID")
    if not isinstance(batch_id, str) or not batch_id or len(file_urls) != 1:
        raise CaptureError("MINERU_SUBMIT_SHAPE_INVALID")
    for raw_url in file_urls:
        if not isinstance(raw_url, str):
            raise CaptureError("MINERU_SUBMIT_SHAPE_INVALID")
        _validate_signed_upload_url(raw_url)
    return {"batchIdPresent": True, "signedUploadUrlCount": len(file_urls)}


def _validate_signed_upload_url(url: str) -> None:
    try:
        parsed = urlsplit(url)
        _ = parsed.port
    except ValueError:
        raise CaptureError("MINERU_SUBMIT_SHAPE_INVALID") from None
    if (
        parsed.scheme != "https"
        or not parsed.hostname
        or parsed.username is not None
        or parsed.password is not None
        or parsed.fragment
    ):
        raise CaptureError("MINERU_SUBMIT_SHAPE_INVALID")
