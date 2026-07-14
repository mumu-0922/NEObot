"""Closed child-internal Native Artifact model for C1.3 parsers."""

from __future__ import annotations

import hashlib
import re
from dataclasses import dataclass
from enum import StrEnum
from typing import Final, Self, cast

from mm_chat_rag.offline_parser.canonical import (
    CanonicalJsonError,
    JsonObject,
    JsonScalar,
    JsonValue,
    canonical_json_bytes,
    load_canonical_json_object,
)
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode

NATIVE_ARTIFACT_SCHEMA_VERSION: Final = "parser-native-artifact.v1"
_SHA256_RE: Final = re.compile(r"^[0-9a-f]{64}$")
_ATTRIBUTE_NAME_RE: Final = re.compile(r"^[a-z][a-zA-Z0-9]{0,63}$")
_SURROGATE_MIN: Final = 0xD800
_SURROGATE_MAX: Final = 0xDFFF


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


class NativeTransformKind(StrEnum):
    """How a native text fragment relates to its exact source span."""

    IDENTITY = "identity"
    SYNTAX_DECODE = "syntax_decode"


class NativeParseFailure(ValueError):  # noqa: N818
    """A native parser failed with one frozen stable error code."""

    def __init__(self, code: StableErrorCode) -> None:
        super().__init__(code.value)
        self.code = code


class NativeArtifactError(ValueError):
    """A child-produced internal Native Artifact is not closed or bound."""


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

    def __post_init__(self) -> None:
        values = (
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

    def contains(self, other: NativeSourcePosition) -> bool:
        """Return whether this source range fully contains another range."""
        return (
            self.raw_byte_start
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
            "rawByteEnd": self.raw_byte_end,
            "rawByteStart": self.raw_byte_start,
            "startColumn": self.start_column,
            "startLine": self.start_line,
        }


@dataclass(frozen=True, slots=True)
class NativeFragment:
    """One source-derived text fragment with an exact native position."""

    ordinal: int
    text: str
    transform: NativeTransformKind
    source_position: NativeSourcePosition

    def __post_init__(self) -> None:
        if type(self.ordinal) is not int or self.ordinal < 0:
            raise ValueError("native fragment ordinal must be non-negative")
        _validate_scalar_text(self.text, allow_empty=False)

    def to_object(self) -> JsonObject:
        """Return the exact internal JSON shape."""
        return {
            "ordinal": self.ordinal,
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
    source_position: NativeSourcePosition
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
        previous_end = -1
        for fragment in self.fragments:
            if not self.source_position.contains(fragment.source_position):
                raise ValueError("native fragment falls outside its node")
            if fragment.source_position.raw_byte_start < previous_end:
                raise ValueError("native node fragments are not source ordered")
            previous_end = fragment.source_position.raw_byte_end
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
    source_encoding: str
    source_bytes: int
    source_sha256: str
    decoded_scalars: int
    nodes: tuple[NativeNode, ...]
    schema_version: str = NATIVE_ARTIFACT_SCHEMA_VERSION

    def __post_init__(self) -> None:
        if self.schema_version != NATIVE_ARTIFACT_SCHEMA_VERSION:
            raise ValueError("unknown native artifact schema version")
        if self.source_format not in {
            ParserFormat.TXT,
            ParserFormat.MARKDOWN,
            ParserFormat.HTML,
        }:
            raise ValueError("C1.3A native artifact has an unsupported format")
        if self.source_encoding not in {"utf-8", "utf-8-bom", "gb18030"}:
            raise ValueError("native artifact has an unsupported source encoding")
        if type(self.source_bytes) is not int or self.source_bytes < 0:
            raise ValueError("native artifact source byte count is invalid")
        if type(self.decoded_scalars) is not int or self.decoded_scalars < 0:
            raise ValueError("native artifact scalar count is invalid")
        if not _SHA256_RE.fullmatch(self.source_sha256):
            raise ValueError("native artifact source hash is invalid")
        if not self.nodes or [item.ordinal for item in self.nodes] != list(
            range(len(self.nodes))
        ):
            raise ValueError("native node ordinals must be non-empty and contiguous")
        root = self.nodes[0]
        if root.kind is not NativeNodeKind.DOCUMENT or root.parent_ordinal is not None:
            raise ValueError("native artifact node zero must be the document root")
        if (
            root.source_position.raw_byte_start != 0
            or root.source_position.raw_byte_end != self.source_bytes
            or root.source_position.decoded_scalar_start != 0
            or root.source_position.decoded_scalar_end != self.decoded_scalars
        ):
            raise ValueError("native document root must cover the complete source")
        for node in self.nodes:
            position = node.source_position
            if (
                position.raw_byte_end > self.source_bytes
                or position.decoded_scalar_end > self.decoded_scalars
            ):
                raise ValueError("native node position exceeds the source bounds")
            if node.parent_ordinal is not None and not self.nodes[
                node.parent_ordinal
            ].source_position.contains(position):
                raise ValueError("native child node falls outside its parent")

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
            "decodedScalars": self.decoded_scalars,
            "encoding": self.source_encoding,
            "format": self.source_format.value,
            "sha256": self.source_sha256,
        }
        value: dict[str, JsonValue] = {
            "nodes": [item.to_object() for item in self.nodes],
            "schemaVersion": self.schema_version,
            "source": source,
        }
        return value

    @classmethod
    def from_bytes(cls, content: bytes) -> Self:
        """Decode canonical bytes and reconstruct every closed DTO invariant."""
        try:
            value = load_canonical_json_object(content)
            _require_fields(
                value,
                {"nodes", "schemaVersion", "source"},
                "native artifact",
            )
            source = _object(value["source"], "native source")
            _require_fields(
                source,
                {
                    "bytes",
                    "decodedScalars",
                    "encoding",
                    "format",
                    "sha256",
                },
                "native source",
            )
            nodes = tuple(_node(item) for item in _list(value["nodes"], "nodes"))
            return cls(
                schema_version=_text(value["schemaVersion"], "schemaVersion"),
                source_format=ParserFormat(_text(source["format"], "source.format")),
                source_encoding=_text(source["encoding"], "source.encoding"),
                source_bytes=_integer(source["bytes"], "source.bytes"),
                source_sha256=_text(source["sha256"], "source.sha256"),
                decoded_scalars=_integer(
                    source["decodedScalars"],
                    "source.decodedScalars",
                ),
                nodes=nodes,
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
        if (
            self.source_format is not expected_format
            or self.source_bytes != len(source)
            or self.source_sha256 != hashlib.sha256(source).hexdigest()
        ):
            raise NativeArtifactError("native artifact source binding does not match")


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


def _node(value: object) -> NativeNode:
    item = _object(value, "native node")
    if set(item) != {
        "attributes",
        "fragments",
        "kind",
        "ordinal",
        "parentOrdinal",
        "sourcePosition",
    }:
        raise NativeArtifactError("native node fields are not closed")
    parent_value = item["parentOrdinal"]
    parent = (
        None if parent_value is None else _integer(parent_value, "node.parentOrdinal")
    )
    return NativeNode(
        ordinal=_integer(item["ordinal"], "node.ordinal"),
        kind=NativeNodeKind(_text(item["kind"], "node.kind")),
        parent_ordinal=parent,
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
    if set(item) != {"ordinal", "sourcePosition", "text", "transform"}:
        raise NativeArtifactError("native fragment fields are not closed")
    return NativeFragment(
        ordinal=_integer(item["ordinal"], "fragment.ordinal"),
        text=_text(item["text"], "fragment.text"),
        transform=NativeTransformKind(_text(item["transform"], "fragment.transform")),
        source_position=_position(item["sourcePosition"]),
    )


def _attribute(value: object) -> NativeAttribute:
    item = _object(value, "native attribute")
    if set(item) != {"name", "value"}:
        raise NativeArtifactError("native attribute fields are not closed")
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


def _position(value: object) -> NativeSourcePosition:
    item = _object(value, "native source position")
    fields = {
        "decodedScalarEnd",
        "decodedScalarStart",
        "endColumn",
        "endLine",
        "rawByteEnd",
        "rawByteStart",
        "startColumn",
        "startLine",
    }
    if set(item) != fields:
        raise NativeArtifactError("native source-position fields are not closed")
    return NativeSourcePosition(
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
