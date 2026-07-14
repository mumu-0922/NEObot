"""Fixed C1.3B Native Parser dispatch behind the Router admission gate."""

from __future__ import annotations

from dataclasses import dataclass
from typing import TYPE_CHECKING, Final, Never

from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG, NativeParserLimits
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.decoding import (
    DecodedSource,
    decode_source,
    source_unit_from_decoded,
)
from mm_chat_rag.offline_parser.native.model import (
    NATIVE_SUPPORTED_FORMATS,
    NativeBytePosition,
    NativeDocument,
    NativeParseFailure,
    NativeSourcePosition,
    NativeSourceUnitKind,
    NativeTransformKind,
)
from mm_chat_rag.offline_parser.router import route_source

if TYPE_CHECKING:
    from mm_chat_rag.offline_parser.native.opc import ValidatedOpcPackage

_TEXT_FORMATS: Final = frozenset(
    {
        ParserFormat.TXT,
        ParserFormat.MARKDOWN,
        ParserFormat.HTML,
        ParserFormat.CSV,
    }
)
_OOXML_FORMATS: Final = frozenset(
    {ParserFormat.DOCX, ParserFormat.PPTX, ParserFormat.XLSX}
)


@dataclass(frozen=True, slots=True)
class NativeParseOutcome:
    """Exactly one internal Native Artifact or stable failure."""

    parser_format: ParserFormat | None
    artifact: NativeDocument | None
    stable_error_code: StableErrorCode | None

    def __post_init__(self) -> None:
        success = (
            self.parser_format in NATIVE_SUPPORTED_FORMATS
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
        native_limits=limits,
    )
    if decision.stable_error_code is not None:
        return _failure(decision.stable_error_code)
    parser_format = decision.parser_format
    if parser_format not in NATIVE_SUPPORTED_FORMATS:
        return _failure(StableErrorCode.FORMAT_UNSUPPORTED)
    try:
        if parser_format in _TEXT_FORMATS:
            if decision.opc_package is not None:
                _schema_failure()
            parser_source: DecodedSource | ValidatedOpcPackage = decode_source(
                source,
                limits=limits,
            )
        else:
            if decision.opc_package is None:
                _schema_failure()
            parser_source = decision.opc_package
        artifact = _parse_selected(parser_format, parser_source, limits)
        _validate_artifact_positions(artifact, parser_source)
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
    source: DecodedSource | ValidatedOpcPackage,
    limits: NativeParserLimits,
) -> NativeDocument:
    if parser_format in _TEXT_FORMATS:
        return _parse_text_selected(parser_format, _decoded_source(source), limits)
    if parser_format in _OOXML_FORMATS:
        return _parse_ooxml_selected(parser_format, _opc_package(source), limits)
    raise NativeParseFailure(StableErrorCode.FORMAT_UNSUPPORTED)


def _parse_text_selected(
    parser_format: ParserFormat,
    source: DecodedSource,
    limits: NativeParserLimits,
) -> NativeDocument:
    if parser_format is ParserFormat.TXT:
        from mm_chat_rag.offline_parser.native.txt import parse_txt  # noqa: PLC0415

        return parse_txt(source)
    if parser_format is ParserFormat.MARKDOWN:
        from mm_chat_rag.offline_parser.native.markdown import (  # noqa: PLC0415
            parse_markdown,
        )

        return parse_markdown(source, limits)
    if parser_format is ParserFormat.HTML:
        from mm_chat_rag.offline_parser.native.html import parse_html  # noqa: PLC0415

        return parse_html(source, limits)
    if parser_format is ParserFormat.CSV:
        from mm_chat_rag.offline_parser.native.csv import parse_csv  # noqa: PLC0415

        return parse_csv(source, limits)
    raise NativeParseFailure(StableErrorCode.FORMAT_UNSUPPORTED)


def _parse_ooxml_selected(
    parser_format: ParserFormat,
    source: ValidatedOpcPackage,
    limits: NativeParserLimits,
) -> NativeDocument:
    if parser_format is ParserFormat.DOCX:
        from mm_chat_rag.offline_parser.native.docx import parse_docx  # noqa: PLC0415

        return parse_docx(source, limits)
    if parser_format is ParserFormat.PPTX:
        from mm_chat_rag.offline_parser.native.pptx import parse_pptx  # noqa: PLC0415

        return parse_pptx(source, limits)
    if parser_format is ParserFormat.XLSX:
        from mm_chat_rag.offline_parser.native.xlsx import parse_xlsx  # noqa: PLC0415

        return parse_xlsx(source, limits)
    raise NativeParseFailure(StableErrorCode.FORMAT_UNSUPPORTED)


def _decoded_source(source: DecodedSource | ValidatedOpcPackage) -> DecodedSource:
    if not isinstance(source, DecodedSource):
        raise NativeParseFailure(StableErrorCode.PARSER_SCHEMA_MISMATCH)
    return source


def _opc_package(
    source: DecodedSource | ValidatedOpcPackage,
) -> ValidatedOpcPackage:
    from mm_chat_rag.offline_parser.native.opc import (  # noqa: PLC0415
        ValidatedOpcPackage,
    )

    if not isinstance(source, ValidatedOpcPackage):
        raise NativeParseFailure(StableErrorCode.PARSER_SCHEMA_MISMATCH)
    return source


def _enforce_artifact_limits(
    artifact: NativeDocument,
    limits: NativeParserLimits,
) -> None:
    nodes = artifact.nodes
    if (
        len(nodes) > limits.nodes
        or len(artifact.source_units) > limits.source_units
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
    source: DecodedSource | ValidatedOpcPackage,
) -> None:
    """Recompute all native Locators while still inside the Seccomp child."""
    decoded_units = _decoded_units_for_artifact(artifact, source)
    for node in artifact.nodes:
        position = node.source_position
        if isinstance(position, NativeBytePosition):
            if (
                node.ordinal != 0
                or artifact.source_format not in _OOXML_FORMATS
                or position
                != NativeBytePosition(
                    source_unit_ordinal=0,
                    raw_byte_start=0,
                    raw_byte_end=artifact.source_bytes,
                )
            ):
                _locator_failure()
        else:
            decoded = decoded_units.get(position.source_unit_ordinal)
            if decoded is None:
                _locator_failure()
            expected_node = (
                decoded.document_position()
                if node.ordinal == 0 and position.source_unit_ordinal == 0
                else decoded.position(
                    position.decoded_scalar_start,
                    position.decoded_scalar_end,
                )
            )
            if position != expected_node:
                _locator_failure()
        for fragment in node.fragments:
            fragment_position = fragment.source_position
            decoded = decoded_units.get(fragment_position.source_unit_ordinal)
            if decoded is None:
                _locator_failure()
            expected_fragment = decoded.position(
                fragment_position.decoded_scalar_start,
                fragment_position.decoded_scalar_end,
            )
            if fragment_position != expected_fragment:
                _locator_failure()
            if (
                fragment.transform is NativeTransformKind.IDENTITY
                and fragment.text
                != decoded.text[
                    fragment_position.decoded_scalar_start : (
                        fragment_position.decoded_scalar_end
                    )
                ]
            ):
                _locator_failure()


def _decoded_units_for_artifact(
    artifact: NativeDocument,
    source: DecodedSource | ValidatedOpcPackage,
) -> dict[int, DecodedSource]:
    if isinstance(source, DecodedSource):
        expected_unit = source_unit_from_decoded(
            source,
            kind=NativeSourceUnitKind.RAW_FILE,
            canonical_uri=None,
        )
        if artifact.source_format not in _TEXT_FORMATS or artifact.source_units != (
            expected_unit,
        ):
            _locator_failure()
        return {0: source}

    package = _opc_package(source)
    if (
        artifact.source_format not in _OOXML_FORMATS
        or artifact.source_format is not package.parser_format
        or artifact.source_units != package.source_units
    ):
        _locator_failure()
    used_ordinals: set[int] = set()
    for node in artifact.nodes:
        if isinstance(node.source_position, NativeSourcePosition):
            used_ordinals.add(node.source_position.source_unit_ordinal)
        used_ordinals.update(
            fragment.source_position.source_unit_ordinal for fragment in node.fragments
        )
    decoded_units: dict[int, DecodedSource] = {}
    for ordinal in used_ordinals:
        if ordinal == 0:
            _locator_failure()
        unit = artifact.source_units[ordinal]
        if unit.canonical_uri is None:
            _locator_failure()
        parsed = package.parse_xml_part(unit.canonical_uri)
        if (
            parsed.decoded.encoding != unit.encoding
            or parsed.decoded.decoded_scalars != unit.decoded_scalars
        ):
            _locator_failure()
        decoded_units[ordinal] = parsed.decoded
    return decoded_units


def _locator_failure() -> Never:
    raise NativeParseFailure(StableErrorCode.QUALITY_LOCATOR_FAILED)


def _schema_failure() -> Never:
    raise NativeParseFailure(StableErrorCode.PARSER_SCHEMA_MISMATCH)


def _failure(code: StableErrorCode) -> NativeParseOutcome:
    return NativeParseOutcome(
        parser_format=None,
        artifact=None,
        stable_error_code=code,
    )
