"""Bounded retry classification for durable orchestration."""

from __future__ import annotations

from dataclasses import dataclass

from mm_chat_rag.models import stable_error_code

_MAX_RETRY_SECONDS = 3600


@dataclass(frozen=True, slots=True)
class RetryDecision:
    """A stable database transition, never raw exception text."""

    outcome: str
    error_code: str
    retry_after_seconds: int


class RetryableJobError(RuntimeError):
    """Handler failure that can be retried by the durable queue."""

    def __init__(self, error_code: str, retry_after_seconds: int = 30) -> None:
        super().__init__(stable_error_code(error_code))
        if not 1 <= retry_after_seconds <= _MAX_RETRY_SECONDS:
            raise ValueError("retry_after_seconds must be between 1 and 3600")
        self.error_code = error_code
        self.retry_after_seconds = retry_after_seconds


class PermanentJobError(RuntimeError):
    """Handler failure that must enter the Postgres DLQ immediately."""

    def __init__(self, error_code: str) -> None:
        super().__init__(stable_error_code(error_code))
        self.error_code = error_code


def retry_or_dlq(
    *, attempt_count: int, max_attempts: int, error_code: str, retry_after_seconds: int
) -> RetryDecision:
    """Choose retry or terminal failed state using the leased attempt snapshot."""
    stable_error_code(error_code)
    if attempt_count >= max_attempts:
        return RetryDecision("failed", error_code, 0)
    return RetryDecision("retry", error_code, retry_after_seconds)
