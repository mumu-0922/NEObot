"""Fixed C1.3A Native Parser dispatch behind the C1.2 Router gate."""

from __future__ import annotations

from dataclasses import dataclass

from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG, NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.decoding import DecodedSource, decode_source
from mm_chat_rag.offline_parser.native.model import (
    NativeDocument,
    NativeParseFailure,
    NativeTransformKind,
)
from mm_chat_rag.offline_parser.router import route_source

_SUPPORTED_FORMATS = frozenset(
    {ParserFormat.TXT, ParserFormat.MARKDOWN, ParserFormat.HTML}
)


@dataclass(frozen=True, slots=True)
class NativeParseOutcome:
    """Exactly one internal Native Artifact or stable failure."""

    parser_format: ParserFormat | None
    artifact: NativeDocument | None
    stable_error_code: StableErrorCode | None

    def __post_init__(self) -> None:
        success = (
            self.parser_format in _SUPPORTED_FORMATS
            and self.artifact is not None
            and self.stable_error_code is None
        )
        failure = (
            self.parser_format is None
            and self.artifact is None
            and self.stable_error_code is not None
        )
        if success == failure:
            raise ValueError("native parse outcome must contain exactly one outcome")
        if (
            success
            and self.artifact is not None
            and self.artifact.source_format is not self.parser_format
        ):
            raise ValueError("native artifact format does not match its dispatch")

    @property
    def artifact_bytes(self) -> bytes:
        """Return the internal artifact bytes or an empty failure body."""
        return self.artifact.canonical_bytes if self.artifact is not None else b""


def parse_native_source(
    source: bytes,
    *,
    declared_mime: str | None = None,
    declared_extension: str | None = None,
    limits: NativeParserLimits = DEFAULT_CONFIG.native,
) -> NativeParseOutcome:
    """Route first, then invoke one fixed Native Parser without fallback."""
    decision = route_source(
        source,
        declared_mime=declared_mime,
        declared_extension=declared_extension,
    )
    if decision.stable_error_code is not None:
        return _failure(decision.stable_error_code)
    parser_format = decision.parser_format
    if parser_format not in _SUPPORTED_FORMATS:
        return _failure(StableErrorCode.FORMAT_UNSUPPORTED)
    try:
        decoded = decode_source(source, limits=limits)
        artifact = _parse_selected(parser_format, decoded, limits)
        _validate_artifact_positions(artifact, decoded)
        _enforce_artifact_limits(artifact, limits)
    except NativeParseFailure as error:
        return _failure(error.code)
    return NativeParseOutcome(
        parser_format=parser_format,
        artifact=artifact,
        stable_error_code=None,
    )


def _parse_selected(
    parser_format: ParserFormat,
    decoded: DecodedSource,
    limits: NativeParserLimits,
) -> NativeDocument:
    if parser_format is ParserFormat.TXT:
        from mm_chat_rag.offline_parser.native.txt import parse_txt  # noqa: PLC0415

        return parse_txt(decoded)
    if parser_format is ParserFormat.MARKDOWN:
        from mm_chat_rag.offline_parser.native.markdown import (  # noqa: PLC0415
            parse_markdown,
        )

        return parse_markdown(decoded, limits)
    if parser_format is ParserFormat.HTML:
        from mm_chat_rag.offline_parser.native.html import parse_html  # noqa: PLC0415

        return parse_html(decoded, limits)
    raise NativeParseFailure(StableErrorCode.FORMAT_UNSUPPORTED)


def _enforce_artifact_limits(
    artifact: NativeDocument,
    limits: NativeParserLimits,
) -> None:
    nodes = artifact.nodes
    if (
        len(nodes) > limits.nodes
        or sum(len(node.fragments) for node in nodes) > limits.fragments
        or sum(len(node.attributes) for node in nodes) > limits.attributes
        or len(artifact.canonical_bytes) > limits.artifact_bytes
    ):
        raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)
    depths = [0] * len(nodes)
    for node in nodes[1:]:
        if node.parent_ordinal is None:
            raise NativeParseFailure(StableErrorCode.PARSER_SCHEMA_MISMATCH)
        depths[node.ordinal] = depths[node.parent_ordinal] + 1
        if depths[node.ordinal] > limits.nesting_depth:
            raise NativeParseFailure(StableErrorCode.RESULT_TOO_LARGE)


def _validate_artifact_positions(
    artifact: NativeDocument,
    decoded: DecodedSource,
) -> None:
    """Verify locator semantics while still inside the Seccomp child."""
    if (
        artifact.source_encoding != decoded.encoding
        or artifact.decoded_scalars != decoded.decoded_scalars
    ):
        raise NativeParseFailure(StableErrorCode.QUALITY_LOCATOR_FAILED)
    for node in artifact.nodes:
        expected_node = (
            decoded.document_position()
            if node.ordinal == 0
            else decoded.position(
                node.source_position.decoded_scalar_start,
                node.source_position.decoded_scalar_end,
            )
        )
        if node.source_position != expected_node:
            raise NativeParseFailure(StableErrorCode.QUALITY_LOCATOR_FAILED)
        for fragment in node.fragments:
            position = fragment.source_position
            expected_fragment = decoded.position(
                position.decoded_scalar_start,
                position.decoded_scalar_end,
            )
            if position != expected_fragment:
                raise NativeParseFailure(StableErrorCode.QUALITY_LOCATOR_FAILED)
            if (
                fragment.transform is NativeTransformKind.IDENTITY
                and fragment.text
                != decoded.text[
                    position.decoded_scalar_start : position.decoded_scalar_end
                ]
            ):
                raise NativeParseFailure(StableErrorCode.QUALITY_LOCATOR_FAILED)


def _failure(code: StableErrorCode) -> NativeParseOutcome:
    return NativeParseOutcome(
        parser_format=None,
        artifact=None,
        stable_error_code=code,
    )
