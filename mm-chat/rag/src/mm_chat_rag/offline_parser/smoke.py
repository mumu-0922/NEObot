"""Credential-free C1.2 MMCP/UDS smoke used by the isolated Compose profile."""

from __future__ import annotations

from mm_chat_rag.offline_parser.errors import StableErrorCode
from mm_chat_rag.offline_parser.output_root import OwnedOutputRoot
from mm_chat_rag.offline_parser.transport import ParserController


def main() -> int:
    """Prove a closed ambiguous-text failure without enabling parsing."""
    outcome = ParserController().invoke(b"plain text", invocation_id="c1.2-smoke")
    if (
        outcome.response is None
        or outcome.response.stable_error_code is not StableErrorCode.FORMAT_AMBIGUOUS
        or outcome.body
        or outcome.local_error_code is not None
        or outcome.stageable
    ):
        return 1
    with OwnedOutputRoot.create() as output:
        output.write_artifact("summary/status.txt", b"c1.2-smoke-passed\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
