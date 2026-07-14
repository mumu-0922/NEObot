"""Closed child-internal Native Artifact model for C1.3 parsers."""

from __future__ import annotations

import hashlib
import re
import unicodedata
from dataclasses import dataclass
from enum import StrEnum
from typing import Final, Self, cast

from mm_chat_rag.offline_parser.canonical import (
    CanonicalJsonError,
    JsonObject,
    JsonScalar,
    canonical_json_bytes,
    load_canonical_json_object,
)
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode

NATIVE_ARTIFACT_SCHEMA_VERSION: Final = "parser-native-artifact.v2"
_SHA256_RE: Final = re.compile(r"^[0-9a-f]{64}$")
_ATTRIBUTE_NAME_RE: Final = re.compile(r"^[a-z][a-zA-Z0-9]{0,63}$")
_SURROGATE_MIN: Final = 0xD800
_SURROGATE_MAX: Final = 0xDFFF
_TEXT_ENCODINGS: Final = frozenset({"utf-8", "utf-8-bom", "gb18030"})
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
_MIN_OOXML_SOURCE_UNITS: Final = 2
NATIVE_SUPPORTED_FORMATS: Final = _TEXT_FORMATS | _OOXML_FORMATS


class NativeNodeKind(StrEnum):
    """Closed structural kinds emitted before C1.4 canonicalization."""

    DOCUMENT = "document"
    HEADING = "heading"
    PARAGRAPH = "paragraph"
    LIST = "list"
    LIST_ITEM = "list_item"
    QUOTE = "quote"
    CODE = "code"
    TABLE = "table"
    TABLE_ROW = "table_row"
    TABLE_CELL = "table_cell"
    RAW_HTML = "raw_html"
    THEMATIC_BREAK = "thematic_break"
    LINE_BREAK = "line_break"
    ASSET_REF = "asset_ref"
    LINK = "link"
    FOOTNOTE = "footnote"
    ENDNOTE = "endnote"
    HEADER = "header"
    FOOTER = "footer"
    PAGE_BREAK = "page_break"
    SLIDE = "slide"
    SHAPE = "shape"
    NOTES = "notes"
    SHEET = "sheet"
    FORMULA = "formula"


class NativeTransformKind(StrEnum):
    """How a native text fragment relates to its exact source span."""

    IDENTITY = "identity"
    SYNTAX_DECODE = "syntax_decode"


class NativeFragmentRole(StrEnum):
    """Closed semantic role used before Canonical IR projection."""

    TEXT = "text"
    CELL_VALUE = "cell_value"
    CACHED_VALUE = "cached_value"
    FORMULA = "formula"
    EXTERNAL_TARGET = "external_target"


class NativeSourceUnitKind(StrEnum):
    """Closed source-unit kinds available to C1.3 Native Parsers."""

    RAW_FILE = "raw_file"
    OOXML_PART = "ooxml_part"


class NativeParseFailure(ValueError):  # noqa: N818
    """A native parser failed with one frozen stable error code."""

    def __init__(self, code: StableErrorCode) -> None:
        super().__init__(code.value)
        self.code = code


class NativeArtifactError(ValueError):
    """A child-produced internal Native Artifact is not closed or bound."""


@dataclass(frozen=True, slots=True)
class NativeSourceUnit:
    """One raw file or decoded OOXML Part referenced by Native positions."""

    ordinal: int
    kind: NativeSourceUnitKind
    canonical_uri: str | None
    source_bytes: int
    source_sha256: str
    encoding: str | None
    decoded_scalars: int | None

    def __post_init__(self) -> None:
        if type(self.ordinal) is not int or self.ordinal < 0:
            raise ValueError("native source-unit ordinal must be non-negative")
        if type(self.source_bytes) is not int or self.source_bytes < 0:
            raise ValueError("native source-unit byte count is invalid")
        if not _SHA256_RE.fullmatch(self.source_sha256):
            raise ValueError("native source-unit hash is invalid")
        if self.kind is NativeSourceUnitKind.RAW_FILE:
            if self.ordinal != 0 or self.canonical_uri is not None:
                raise ValueError("native raw-file source unit must be ordinal zero")
        else:
            if self.ordinal == 0 or self.canonical_uri is None:
                raise ValueError("native OOXML Part requires a canonical URI")
            _validate_canonical_part_uri(self.canonical_uri)
        if self.encoding is None:
            if self.decoded_scalars is not None:
                raise ValueError("binary source unit cannot declare decoded scalars")
        elif (
            self.encoding not in _TEXT_ENCODINGS
            or type(self.decoded_scalars) is not int
            or self.decoded_scalars < 0
        ):
            raise ValueError("native text source-unit decoding metadata is invalid")

    def to_object(self) -> JsonObject:
        """Return the exact internal JSON shape."""
        return {
            "bytes": self.source_bytes,
            "canonicalUri": self.canonical_uri,
            "decodedScalars": self.decoded_scalars,
            "encoding": self.encoding,
            "kind": self.kind.value,
            "ordinal": self.ordinal,
            "sha256": self.source_sha256,
        }


@dataclass(frozen=True, slots=True)
class NativeBytePosition:
    """Zero-based half-open byte position for a binary source unit."""

    source_unit_ordinal: int
    raw_byte_start: int
    raw_byte_end: int

    def __post_init__(self) -> None:
        values = (self.source_unit_ordinal, self.raw_byte_start, self.raw_byte_end)
        if any(type(value) is not int or value < 0 for value in values):
            raise ValueError("native byte positions must be non-negative integers")
        if self.raw_byte_start > self.raw_byte_end:
            raise ValueError("native raw byte range is reversed")

    def contains(self, other: NativePosition) -> bool:
        """Return whether this byte position contains another position."""
        return (
            self.source_unit_ordinal == other.source_unit_ordinal
            and self.raw_byte_start <= other.raw_byte_start
            and other.raw_byte_end <= self.raw_byte_end
        )

    def to_object(self) -> JsonObject:
        """Return the exact internal JSON shape."""
        return {
            "kind": "byte",
            "rawByteEnd": self.raw_byte_end,
            "rawByteStart": self.raw_byte_start,
            "sourceUnitOrdinal": self.source_unit_ordinal,
        }


@dataclass(frozen=True, slots=True)
class NativeSourcePosition:
    """Zero-based half-open Raw Byte, Scalar, and Line/Column position."""

    raw_byte_start: int
    raw_byte_end: int
    decoded_scalar_start: int
    decoded_scalar_end: int
    start_line: int
    start_column: int
    end_line: int
    end_column: int
    source_unit_ordinal: int = 0

    def __post_init__(self) -> None:
        values = (
            self.source_unit_ordinal,
            self.raw_byte_start,
            self.raw_byte_end,
            self.decoded_scalar_start,
            self.decoded_scalar_end,
            self.start_line,
            self.start_column,
            self.end_line,
            self.end_column,
        )
        if any(type(value) is not int or value < 0 for value in values):
            raise ValueError("native source positions must be non-negative integers")
        if self.raw_byte_start > self.raw_byte_end:
            raise ValueError("native raw byte range is reversed")
        if self.decoded_scalar_start > self.decoded_scalar_end:
            raise ValueError("native decoded scalar range is reversed")
        if (self.start_line, self.start_column) > (
            self.end_line,
            self.end_column,
        ):
            raise ValueError("native line/column range is reversed")

    def contains(self, other: NativePosition) -> bool:
        """Return whether this source range fully contains another range."""
        return (
            isinstance(other, NativeSourcePosition)
            and self.source_unit_ordinal == other.source_unit_ordinal
            and self.raw_byte_start
            <= other.raw_byte_start
            <= other.raw_byte_end
            <= self.raw_byte_end
            and self.decoded_scalar_start
            <= other.decoded_scalar_start
            <= other.decoded_scalar_end
            <= self.decoded_scalar_end
            and (self.start_line, self.start_column)
            <= (other.start_line, other.start_column)
            <= (other.end_line, other.end_column)
            <= (self.end_line, self.end_column)
        )

    def to_object(self) -> JsonObject:
        """Return the exact internal JSON shape."""
        return {
            "decodedScalarEnd": self.decoded_scalar_end,
            "decodedScalarStart": self.decoded_scalar_start,
            "endColumn": self.end_column,
            "endLine": self.end_line,
            "kind": "text",
            "rawByteEnd": self.raw_byte_end,
            "rawByteStart": self.raw_byte_start,
            "sourceUnitOrdinal": self.source_unit_ordinal,
            "startColumn": self.start_column,
            "startLine": self.start_line,
        }


type NativePosition = NativeBytePosition | NativeSourcePosition


@dataclass(frozen=True, slots=True)
class NativeFragment:
    """One source-derived text fragment with an exact native position."""

    ordinal: int
    text: str
    transform: NativeTransformKind
    source_position: NativeSourcePosition
    role: NativeFragmentRole = NativeFragmentRole.TEXT

    def __post_init__(self) -> None:
        if type(self.ordinal) is not int or self.ordinal < 0:
            raise ValueError("native fragment ordinal must be non-negative")
        _validate_scalar_text(self.text, allow_empty=False)

    def to_object(self) -> JsonObject:
        """Return the exact internal JSON shape."""
        return {
            "ordinal": self.ordinal,
            "role": self.role.value,
            "sourcePosition": self.source_position.to_object(),
            "text": self.text,
            "transform": self.transform.value,
        }


@dataclass(frozen=True, slots=True)
class NativeAttribute:
    """One sorted, closed-name structural attribute."""

    name: str
    value: JsonScalar

    def __post_init__(self) -> None:
        if not _ATTRIBUTE_NAME_RE.fullmatch(self.name):
            raise ValueError("native attribute name is not a closed ASCII identifier")
        if isinstance(self.value, str):
            _validate_scalar_text(self.value, allow_empty=True)
        elif self.value is not None and type(self.value) not in {bool, int}:
            raise ValueError("native attribute value has an unsupported scalar type")

    def to_object(self) -> JsonObject:
        """Return the exact internal JSON shape."""
        return {"name": self.name, "value": self.value}


@dataclass(frozen=True, slots=True)
class NativeNode:
    """One deterministic structural node in source reading order."""

    ordinal: int
    kind: NativeNodeKind
    parent_ordinal: int | None
    source_position: NativePosition
    fragments: tuple[NativeFragment, ...] = ()
    attributes: tuple[NativeAttribute, ...] = ()

    def __post_init__(self) -> None:
        if type(self.ordinal) is not int or self.ordinal < 0:
            raise ValueError("native node ordinal must be non-negative")
        if self.parent_ordinal is not None and (
            type(self.parent_ordinal) is not int
            or self.parent_ordinal < 0
            or self.parent_ordinal >= self.ordinal
        ):
            raise ValueError("native parent ordinal must precede its child")
        if [item.ordinal for item in self.fragments] != list(
            range(len(self.fragments))
        ):
            raise ValueError("native fragment ordinals must be contiguous")
        previous_ends: dict[int, int] = {}
        for fragment in self.fragments:
            position = fragment.source_position
            if (
                position.source_unit_ordinal == self.source_position.source_unit_ordinal
                and not self.source_position.contains(position)
            ):
                raise ValueError("native fragment falls outside its node")
            if (
                position.source_unit_ordinal != self.source_position.source_unit_ordinal
                and fragment.transform is not NativeTransformKind.SYNTAX_DECODE
            ):
                raise ValueError("cross-unit native fragments must be syntax decoded")
            previous_end = previous_ends.get(position.source_unit_ordinal, -1)
            if position.raw_byte_start < previous_end:
                raise ValueError("native node fragments are not source ordered")
            previous_ends[position.source_unit_ordinal] = position.raw_byte_end
        names = [item.name for item in self.attributes]
        if names != sorted(names) or len(names) != len(set(names)):
            raise ValueError("native attributes must be unique and name-sorted")

    def to_object(self) -> JsonObject:
        """Return the exact internal JSON shape."""
        return {
            "attributes": [item.to_object() for item in self.attributes],
            "fragments": [item.to_object() for item in self.fragments],
            "kind": self.kind.value,
            "ordinal": self.ordinal,
            "parentOrdinal": self.parent_ordinal,
            "sourcePosition": self.source_position.to_object(),
        }


@dataclass(frozen=True, slots=True)
class NativeDocument:
    """Closed internal Native Artifact; never an MMCP success body in C1.3."""

    source_format: ParserFormat
    source_bytes: int
    source_sha256: str
    source_units: tuple[NativeSourceUnit, ...]
    nodes: tuple[NativeNode, ...]
    schema_version: str = NATIVE_ARTIFACT_SCHEMA_VERSION

    def __post_init__(self) -> None:
        if self.schema_version != NATIVE_ARTIFACT_SCHEMA_VERSION:
            raise ValueError("unknown native artifact schema version")
        if self.source_format not in NATIVE_SUPPORTED_FORMATS:
            raise ValueError("native artifact has an unsupported format")
        if type(self.source_bytes) is not int or self.source_bytes < 0:
            raise ValueError("native artifact source byte count is invalid")
        if not _SHA256_RE.fullmatch(self.source_sha256):
            raise ValueError("native artifact source hash is invalid")
        if not self.source_units or [
            item.ordinal for item in self.source_units
        ] != list(range(len(self.source_units))):
            raise ValueError("native source-unit ordinals must be contiguous")
        raw_unit = self.source_units[0]
        if (
            raw_unit.kind is not NativeSourceUnitKind.RAW_FILE
            or raw_unit.source_bytes != self.source_bytes
            or raw_unit.source_sha256 != self.source_sha256
        ):
            raise ValueError("native raw source unit is not bound to the request")
        if self.source_format in _TEXT_FORMATS and (
            raw_unit.encoding is None or len(self.source_units) != 1
        ):
            raise ValueError("native text format requires exactly one decoded unit")
        if self.source_format in _OOXML_FORMATS and (
            raw_unit.encoding is not None
            or len(self.source_units) < _MIN_OOXML_SOURCE_UNITS
        ):
            raise ValueError("native OOXML format requires a binary raw unit and Parts")
        part_uris = [
            item.canonical_uri
            for item in self.source_units[1:]
            if item.kind is NativeSourceUnitKind.OOXML_PART
        ]
        folded_uris = [cast("str", value).casefold() for value in part_uris]
        if (
            len(part_uris) != len(self.source_units) - 1
            or len(part_uris) != len(set(part_uris))
            or len(folded_uris) != len(set(folded_uris))
            or part_uris
            != sorted(
                part_uris,
                key=lambda value: cast("str", value).encode("utf-8"),
            )
        ):
            raise ValueError("native OOXML source units are not URI ordered")
        if not self.nodes or [item.ordinal for item in self.nodes] != list(
            range(len(self.nodes))
        ):
            raise ValueError("native node ordinals must be non-empty and contiguous")
        root = self.nodes[0]
        if root.kind is not NativeNodeKind.DOCUMENT or root.parent_ordinal is not None:
            raise ValueError("native artifact node zero must be the document root")
        self._validate_root(root.source_position, raw_unit)
        for node in self.nodes:
            self._validate_position(node.source_position)
            for fragment in node.fragments:
                self._validate_position(fragment.source_position)
            if node.parent_ordinal not in {None, 0} and not self.nodes[
                cast("int", node.parent_ordinal)
            ].source_position.contains(node.source_position):
                raise ValueError("native child node falls outside its parent")

    @property
    def source_encoding(self) -> str:
        """Return the raw text encoding or the closed binary marker."""
        return self.source_units[0].encoding or "binary"

    @property
    def decoded_scalars(self) -> int:
        """Return raw-file decoded scalars; binary packages have zero."""
        return self.source_units[0].decoded_scalars or 0

    @property
    def canonical_bytes(self) -> bytes:
        """Return deterministic internal artifact bytes."""
        return canonical_json_bytes(self.to_object())

    @property
    def artifact_sha256(self) -> str:
        """Return the digest used by the child/supervisor internal envelope."""
        return hashlib.sha256(self.canonical_bytes).hexdigest()

    def to_object(self) -> JsonObject:
        """Return the exact internal artifact shape."""
        source: JsonObject = {
            "bytes": self.source_bytes,
            "format": self.source_format.value,
            "sha256": self.source_sha256,
        }
        return {
            "nodes": [item.to_object() for item in self.nodes],
            "schemaVersion": self.schema_version,
            "source": source,
            "sourceUnits": [item.to_object() for item in self.source_units],
        }

    @classmethod
    def from_bytes(cls, content: bytes) -> Self:
        """Decode canonical bytes and reconstruct every closed DTO invariant."""
        try:
            value = load_canonical_json_object(content)
            _require_fields(
                value,
                {"nodes", "schemaVersion", "source", "sourceUnits"},
                "native artifact",
            )
            source = _object(value["source"], "native source")
            _require_fields(source, {"bytes", "format", "sha256"}, "native source")
            return cls(
                schema_version=_text(value["schemaVersion"], "schemaVersion"),
                source_format=ParserFormat(_text(source["format"], "source.format")),
                source_bytes=_integer(source["bytes"], "source.bytes"),
                source_sha256=_text(source["sha256"], "source.sha256"),
                source_units=tuple(
                    _source_unit(item)
                    for item in _list(value["sourceUnits"], "sourceUnits")
                ),
                nodes=tuple(_node(item) for item in _list(value["nodes"], "nodes")),
            )
        except NativeArtifactError:
            raise
        except (CanonicalJsonError, KeyError, TypeError, ValueError) as error:
            raise NativeArtifactError("native artifact is invalid") from error

    def validate_source_binding(
        self,
        source: bytes,
        *,
        expected_format: ParserFormat,
    ) -> None:
        """Bind child output metadata to request bytes without parsing Source."""
        digest = hashlib.sha256(source).hexdigest()
        raw_unit = self.source_units[0]
        if (
            self.source_format is not expected_format
            or self.source_bytes != len(source)
            or self.source_sha256 != digest
            or raw_unit.source_bytes != len(source)
            or raw_unit.source_sha256 != digest
        ):
            raise NativeArtifactError("native artifact source binding does not match")

    def _validate_root(
        self,
        position: NativePosition,
        raw_unit: NativeSourceUnit,
    ) -> None:
        if (
            position.source_unit_ordinal != 0
            or position.raw_byte_start != 0
            or position.raw_byte_end != self.source_bytes
        ):
            raise ValueError("native document root must cover the complete source")
        if raw_unit.encoding is None:
            if not isinstance(position, NativeBytePosition):
                raise ValueError("binary native root requires a byte position")
        elif not (
            isinstance(position, NativeSourcePosition)
            and position.decoded_scalar_start == 0
            and position.decoded_scalar_end == raw_unit.decoded_scalars
        ):
            raise ValueError("text native root must cover all decoded scalars")

    def _validate_position(self, position: NativePosition) -> None:
        if position.source_unit_ordinal >= len(self.source_units):
            raise ValueError("native position references an unknown source unit")
        unit = self.source_units[position.source_unit_ordinal]
        if position.raw_byte_end > unit.source_bytes:
            raise ValueError("native position exceeds source-unit byte bounds")
        if isinstance(position, NativeBytePosition) and unit.encoding is not None:
            raise ValueError("native byte position requires a binary source unit")
        if isinstance(position, NativeSourcePosition) and (
            unit.encoding is None
            or unit.decoded_scalars is None
            or position.decoded_scalar_end > unit.decoded_scalars
        ):
            raise ValueError("native text position exceeds source-unit bounds")


def attributes(**values: JsonScalar) -> tuple[NativeAttribute, ...]:
    """Build one name-sorted attribute tuple for parser implementations."""
    return tuple(
        NativeAttribute(name=name, value=value)
        for name, value in sorted(values.items())
    )


def _validate_scalar_text(value: str, *, allow_empty: bool) -> None:
    if not isinstance(value, str) or (not allow_empty and not value):
        raise ValueError("native text must be a Unicode scalar string")
    if "\x00" in value or any(
        _SURROGATE_MIN <= ord(character) <= _SURROGATE_MAX for character in value
    ):
        raise ValueError("native text contains a non-scalar value")


def _validate_canonical_part_uri(value: str) -> None:
    _validate_scalar_text(value, allow_empty=False)
    if (
        not value.startswith("/")
        or value != unicodedata.normalize("NFC", value)
        or "\\" in value
        or any(segment in {"", ".", ".."} for segment in value[1:].split("/"))
    ):
        raise ValueError("native OOXML Part URI is not canonical")


def _source_unit(value: object) -> NativeSourceUnit:
    item = _object(value, "native source unit")
    _require_fields(
        item,
        {
            "bytes",
            "canonicalUri",
            "decodedScalars",
            "encoding",
            "kind",
            "ordinal",
            "sha256",
        },
        "native source unit",
    )
    canonical_uri = item["canonicalUri"]
    encoding = item["encoding"]
    decoded_scalars = item["decodedScalars"]
    return NativeSourceUnit(
        ordinal=_integer(item["ordinal"], "sourceUnit.ordinal"),
        kind=NativeSourceUnitKind(_text(item["kind"], "sourceUnit.kind")),
        canonical_uri=(
            None
            if canonical_uri is None
            else _text(canonical_uri, "sourceUnit.canonicalUri")
        ),
        source_bytes=_integer(item["bytes"], "sourceUnit.bytes"),
        source_sha256=_text(item["sha256"], "sourceUnit.sha256"),
        encoding=None if encoding is None else _text(encoding, "sourceUnit.encoding"),
        decoded_scalars=(
            None
            if decoded_scalars is None
            else _integer(decoded_scalars, "sourceUnit.decodedScalars")
        ),
    )


def _node(value: object) -> NativeNode:
    item = _object(value, "native node")
    _require_fields(
        item,
        {
            "attributes",
            "fragments",
            "kind",
            "ordinal",
            "parentOrdinal",
            "sourcePosition",
        },
        "native node",
    )
    parent_value = item["parentOrdinal"]
    return NativeNode(
        ordinal=_integer(item["ordinal"], "node.ordinal"),
        kind=NativeNodeKind(_text(item["kind"], "node.kind")),
        parent_ordinal=(
            None
            if parent_value is None
            else _integer(parent_value, "node.parentOrdinal")
        ),
        source_position=_position(item["sourcePosition"]),
        fragments=tuple(
            _fragment(fragment)
            for fragment in _list(item["fragments"], "node.fragments")
        ),
        attributes=tuple(
            _attribute(attribute)
            for attribute in _list(item["attributes"], "node.attributes")
        ),
    )


def _fragment(value: object) -> NativeFragment:
    item = _object(value, "native fragment")
    _require_fields(
        item,
        {"ordinal", "role", "sourcePosition", "text", "transform"},
        "native fragment",
    )
    position = _position(item["sourcePosition"])
    if not isinstance(position, NativeSourcePosition):
        raise NativeArtifactError("native fragment position must be text")
    return NativeFragment(
        ordinal=_integer(item["ordinal"], "fragment.ordinal"),
        role=NativeFragmentRole(_text(item["role"], "fragment.role")),
        text=_text(item["text"], "fragment.text"),
        transform=NativeTransformKind(_text(item["transform"], "fragment.transform")),
        source_position=position,
    )


def _attribute(value: object) -> NativeAttribute:
    item = _object(value, "native attribute")
    _require_fields(item, {"name", "value"}, "native attribute")
    raw_value = item["value"]
    if not (
        raw_value is None
        or type(raw_value) in {bool, int}
        or isinstance(raw_value, str)
    ):
        raise NativeArtifactError("native attribute value is not scalar")
    return NativeAttribute(
        name=_text(item["name"], "attribute.name"),
        value=cast("JsonScalar", raw_value),
    )


def _position(value: object) -> NativePosition:
    item = _object(value, "native source position")
    kind = _text(item.get("kind"), "position.kind")
    if kind == "byte":
        _require_fields(
            item,
            {"kind", "rawByteEnd", "rawByteStart", "sourceUnitOrdinal"},
            "native byte position",
        )
        return NativeBytePosition(
            source_unit_ordinal=_integer(
                item["sourceUnitOrdinal"],
                "position.sourceUnitOrdinal",
            ),
            raw_byte_start=_integer(item["rawByteStart"], "position.rawByteStart"),
            raw_byte_end=_integer(item["rawByteEnd"], "position.rawByteEnd"),
        )
    if kind != "text":
        raise NativeArtifactError("native source-position kind is unknown")
    _require_fields(
        item,
        {
            "decodedScalarEnd",
            "decodedScalarStart",
            "endColumn",
            "endLine",
            "kind",
            "rawByteEnd",
            "rawByteStart",
            "sourceUnitOrdinal",
            "startColumn",
            "startLine",
        },
        "native text position",
    )
    return NativeSourcePosition(
        source_unit_ordinal=_integer(
            item["sourceUnitOrdinal"],
            "position.sourceUnitOrdinal",
        ),
        raw_byte_start=_integer(item["rawByteStart"], "position.rawByteStart"),
        raw_byte_end=_integer(item["rawByteEnd"], "position.rawByteEnd"),
        decoded_scalar_start=_integer(
            item["decodedScalarStart"],
            "position.decodedScalarStart",
        ),
        decoded_scalar_end=_integer(
            item["decodedScalarEnd"],
            "position.decodedScalarEnd",
        ),
        start_line=_integer(item["startLine"], "position.startLine"),
        start_column=_integer(item["startColumn"], "position.startColumn"),
        end_line=_integer(item["endLine"], "position.endLine"),
        end_column=_integer(item["endColumn"], "position.endColumn"),
    )


def _object(value: object, path: str) -> dict[str, object]:
    if not isinstance(value, dict) or not all(isinstance(key, str) for key in value):
        raise NativeArtifactError(f"{path} must be an object")
    return cast("dict[str, object]", value)


def _list(value: object, path: str) -> list[object]:
    if not isinstance(value, list):
        raise NativeArtifactError(f"{path} must be an array")
    return cast("list[object]", value)


def _text(value: object, path: str) -> str:
    if not isinstance(value, str):
        raise NativeArtifactError(f"{path} must be text")
    return value


def _integer(value: object, path: str) -> int:
    if type(value) is not int:
        raise NativeArtifactError(f"{path} must be an integer")
    return value


def _require_fields(
    value: dict[str, object] | JsonObject,
    fields: set[str],
    path: str,
) -> None:
    if set(value) != fields:
        raise NativeArtifactError(f"{path} fields are not closed")
