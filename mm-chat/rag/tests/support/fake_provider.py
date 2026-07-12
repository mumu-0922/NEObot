"""In-memory, no-network provider fake driven by validated fixtures."""

from __future__ import annotations

import hashlib
import json
import math
from collections.abc import Awaitable, Callable, Mapping
from dataclasses import dataclass
from typing import Final

from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse, Response
from starlette.routing import Route

from tests.support.provider_contracts import (
    ContractValidationError,
    Operation,
    ProviderContract,
    ResponseCase,
)

_CASE_HEADER: Final = "X-MM-Chat-Fixture-Case"
_MAX_SAFE_INTEGER: Final = (1 << 53) - 1


@dataclass(frozen=True, slots=True)
class RecordedCall:
    """Sanitized request evidence; header values and body bytes are never retained."""

    operation_id: str
    method: str
    path: str
    header_names: tuple[str, ...]
    body_sha256: str
    body_bytes: int


class FakeProvider:
    """Build a Starlette app without listeners, DNS, sockets, or global state."""

    def __init__(
        self,
        contract: ProviderContract,
        selected_cases: dict[str, str],
    ) -> None:
        self.contract = contract
        self.calls: list[RecordedCall] = []
        routes: list[Route] = []
        route_keys: set[tuple[str, str]] = set()
        for operation_id, case_id in selected_cases.items():
            operation = contract.operation(operation_id)
            if operation.support_state != "observed":
                raise ContractValidationError("fake cannot route an unknown operation")
            if operation.method is None or operation.path_template is None:
                raise ContractValidationError("observed operation is incomplete")
            response_case = _case(operation, case_id)
            key = (operation.method, operation.path_template)
            if key in route_keys:
                raise ContractValidationError("selected fake routes are ambiguous")
            route_keys.add(key)
            routes.append(
                Route(
                    operation.path_template,
                    self._endpoint(operation, response_case),
                    methods=[operation.method],
                )
            )
        self.app = Starlette(routes=routes)

    def _endpoint(
        self, operation: Operation, response_case: ResponseCase
    ) -> Callable[[Request], Awaitable[Response]]:
        path_template = operation.path_template
        if path_template is None:
            raise ContractValidationError("observed operation lacks a path template")

        async def endpoint(request: Request) -> Response:  # noqa: PLR0911
            body = await request.body()
            request_contract = operation.request
            if request_contract is None:
                return JSONResponse(
                    {"error": "FIXTURE_REQUEST_UNDEFINED"}, status_code=500
                )
            maximum_bytes = request_contract["maximumBytes"]
            if not isinstance(maximum_bytes, int) or len(body) > maximum_bytes:
                return JSONResponse(
                    {"error": "FIXTURE_REQUEST_TOO_LARGE"}, status_code=413
                )
            required = {
                str(name).lower() for name in request_contract["requiredHeaderNames"]
            }
            actual_names = {name.lower() for name in request.headers}
            if not required <= actual_names:
                return JSONResponse(
                    {"error": "FIXTURE_HEADERS_MISMATCH"}, status_code=400
                )
            if "content-type" in required or body:
                expected_content_type = str(request_contract["contentType"]).lower()
                actual_content_type = request.headers.get("content-type", "")
                actual_media_type = actual_content_type.split(";", 1)[0].strip().lower()
                if actual_media_type != expected_content_type:
                    return JSONResponse(
                        {"error": "FIXTURE_CONTENT_TYPE_MISMATCH"}, status_code=400
                    )
            if not _matches_body(body, request_contract["body"]):
                return JSONResponse({"error": "FIXTURE_BODY_MISMATCH"}, status_code=400)
            self.calls.append(
                RecordedCall(
                    operation_id=operation.operation_id,
                    method=request.method,
                    path=path_template,
                    header_names=tuple(sorted(actual_names)),
                    body_sha256=hashlib.sha256(body).hexdigest(),
                    body_bytes=len(body),
                )
            )
            response_body = response_case.body
            headers = dict(response_case.headers)
            headers[_CASE_HEADER] = response_case.case_id
            if "json" in response_body:
                return JSONResponse(
                    _mutable_json(response_body["json"]),
                    status_code=response_case.status,
                    headers=headers,
                )
            return Response(
                str(response_body["rawBodyUtf8"]).encode(),
                status_code=response_case.status,
                headers=headers,
            )

        return endpoint


def _case(operation: Operation, case_id: str) -> ResponseCase:
    for response_case in operation.response_cases:
        if response_case.case_id == case_id:
            return response_case
    raise ContractValidationError("selected response case does not exist")


def _matches_body(body: bytes, expected: object) -> bool:
    if not isinstance(expected, Mapping):
        return False
    if "rawBodyUtf8" in expected:
        return body == str(expected["rawBodyUtf8"]).encode()
    if "json" not in expected:
        return False
    try:
        actual = json.loads(
            body,
            object_pairs_hook=_unique_object,
            parse_constant=_reject_constant,
            parse_float=_finite_float,
            parse_int=_safe_int,
        )
    except (UnicodeDecodeError, json.JSONDecodeError, ValueError):
        return False
    return bool(actual == _mutable_json(expected["json"]))


def _reject_constant(value: str) -> None:
    raise ValueError(f"non-finite JSON constant: {value}")


def _unique_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


def _finite_float(value: str) -> float:
    parsed = float(value)
    if not math.isfinite(parsed):
        raise ValueError("non-finite JSON number")
    return parsed


def _safe_int(value: str) -> int:
    parsed = int(value)
    if abs(parsed) > _MAX_SAFE_INTEGER:
        raise ValueError("JSON integer exceeds safe range")
    return parsed


def _mutable_json(value: object) -> object:
    if isinstance(value, Mapping):
        return {str(key): _mutable_json(item) for key, item in value.items()}
    if isinstance(value, tuple):
        return [_mutable_json(item) for item in value]
    return value
