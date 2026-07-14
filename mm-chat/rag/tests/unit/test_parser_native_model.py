"""C1.3 child-internal Native Artifact and result-envelope contracts."""

from __future__ import annotations

import hashlib
import json
from dataclasses import fields, replace
from typing import Any, cast

import pytest

from mm_chat_rag.offline_parser.canonical import (
    JsonObject,
    JsonValue,
    canonical_json_bytes,
)
from mm_chat_rag.offline_parser.config import DEFAULT_CONFIG
from mm_chat_rag.offline_parser.errors import ParserFormat, StableErrorCode
from mm_chat_rag.offline_parser.native.decoding import decode_source
from mm_chat_rag.offline_parser.native.internal_result import (
    InternalResultError,
    NativeResultHeader,
)
from mm_chat_rag.offline_parser.native.model import (
    NATIVE_ARTIFACT_SCHEMA_VERSION,
    NativeArtifactError,
    NativeAttribute,
    NativeBytePosition,
    NativeDocument,
    NativeFragment,
    NativeFragmentRole,
    NativeNode,
    NativeNodeKind,
    NativeParseFailure,
    NativeSourcePosition,
    NativeSourceUnit,
    NativeSourceUnitKind,
    NativeTransformKind,
    attributes,
)
from mm_chat_rag.offline_parser.native.profile import native_parser_profile_manifest
from mm_chat_rag.offline_parser.native.txt import parse_txt


def _artifact_object() -> JsonObject:
    return parse_txt(decode_source(b"x")).to_object()


def _object_field(value: JsonObject, field: str) -> JsonObject:
    observed = value[field]
    assert isinstance(observed, dict)
    return observed


def _array_field(value: JsonObject, field: str) -> list[JsonValue]:
    observed = value[field]
    assert isinstance(observed, list)
    return observed


def _node_object(value: JsonObject, ordinal: int) -> JsonObject:
    observed = _array_field(value, "nodes")[ordinal]
    assert isinstance(observed, dict)
    return observed


def test_native_txt_artifact_is_closed_and_byte_deterministic() -> None:
    source = "café\r\n中文\n".encode()
    artifact = parse_txt(decode_source(source))

    first = artifact.canonical_bytes
    second = parse_txt(decode_source(source)).canonical_bytes
    value = json.loads(first)

    assert first == second
    assert artifact.artifact_sha256 == hashlib.sha256(first).hexdigest()
    assert value["schemaVersion"] == NATIVE_ARTIFACT_SCHEMA_VERSION
    assert value["source"] == {
        "bytes": len(source),
        "format": "txt",
        "sha256": hashlib.sha256(source).hexdigest(),
    }
    assert value["sourceUnits"] == [
        {
            "bytes": len(source),
            "canonicalUri": None,
            "decodedScalars": len("café\r\n中文\n"),
            "encoding": "utf-8",
            "kind": "raw_file",
            "ordinal": 0,
            "sha256": hashlib.sha256(source).hexdigest(),
        },
    ]
    assert [node["kind"] for node in value["nodes"]] == [
        "document",
        "paragraph",
    ]
    observed = NativeDocument.from_bytes(first)
    observed.validate_source_binding(source, expected_format=ParserFormat.TXT)


def test_internal_result_success_and_failure_discriminators_bind_body() -> None:
    body = parse_txt(decode_source(b"source")).canonical_bytes
    success = NativeResultHeader.success(ParserFormat.TXT, body)
    failure = NativeResultHeader.failure(StableErrorCode.INPUT_INVALID)

    observed = NativeResultHeader.from_bytes(success.canonical_bytes)
    observed.validate_body(body, body_limit=len(body))
    NativeResultHeader.from_bytes(failure.canonical_bytes).validate_body(
        b"",
        body_limit=len(body),
    )

    with pytest.raises(InternalResultError, match="hash"):
        observed.validate_body(body[:-1] + b"x", body_limit=len(body))
    with pytest.raises(InternalResultError):
        NativeResultHeader.from_bytes(b'{"outcome":"failure"}')
    with pytest.raises(InternalResultError, match="controller-only"):
        NativeResultHeader.failure(StableErrorCode.PARSER_CANCELLED)
    controller_only = failure.to_object()
    controller_only["stableErrorCode"] = "PARSER_SANDBOX_UNAVAILABLE"
    with pytest.raises(InternalResultError, match="field combination"):
        NativeResultHeader.from_bytes(canonical_json_bytes(controller_only))
    wrong_type = failure.to_object()
    wrong_type["format"] = 1
    with pytest.raises(InternalResultError, match="nullable text"):
        NativeResultHeader.from_bytes(canonical_json_bytes(wrong_type))
    with pytest.raises(NativeArtifactError):
        NativeDocument.from_bytes(b"{}")


def test_native_model_rejects_reversed_unsorted_and_out_of_bounds_shapes() -> None:
    with pytest.raises(ValueError, match="reversed"):
        NativeSourcePosition(2, 1, 0, 0, 0, 0, 0, 0)

    position = NativeSourcePosition(0, 1, 0, 1, 0, 0, 0, 1)
    with pytest.raises(ValueError, match="attributes"):
        NativeNode(
            ordinal=1,
            kind=NativeNodeKind.PARAGRAPH,
            parent_ordinal=0,
            source_position=position,
            attributes=(
                NativeAttribute("z", 1),
                NativeAttribute("a", 2),
            ),
        )
    with pytest.raises(ValueError, match="ordinals"):
        NativeNode(
            ordinal=1,
            kind=NativeNodeKind.PARAGRAPH,
            parent_ordinal=0,
            source_position=position,
            fragments=(
                NativeFragment(
                    ordinal=1,
                    text="x",
                    transform=NativeTransformKind.IDENTITY,
                    source_position=position,
                ),
            ),
        )
    artifact = parse_txt(decode_source(b"x"))
    root = NativeNode(0, NativeNodeKind.DOCUMENT, None, position)
    with pytest.raises(ValueError, match="bound|complete source|bounds"):
        replace(artifact, source_bytes=0, source_sha256="0" * 64, nodes=(root,))


def test_native_source_positions_reject_invalid_scalar_and_line_ranges() -> None:
    with pytest.raises(ValueError, match="non-negative integers"):
        NativeSourcePosition(False, 0, 0, 0, 0, 0, 0, 0)
    with pytest.raises(ValueError, match="decoded scalar range is reversed"):
        NativeSourcePosition(0, 0, 2, 1, 0, 0, 0, 0)
    with pytest.raises(ValueError, match="line/column range is reversed"):
        NativeSourcePosition(0, 0, 0, 0, 1, 0, 0, 1)


def test_native_v2_ooxml_source_units_and_byte_root_round_trip() -> None:
    source = b"package"
    part = b"<x>value</x>"
    source_sha256 = hashlib.sha256(source).hexdigest()
    part_sha256 = hashlib.sha256(part).hexdigest()
    root = NativeNode(
        0,
        NativeNodeKind.DOCUMENT,
        None,
        NativeBytePosition(0, 0, len(source)),
    )
    child_position = NativeSourcePosition(
        3,
        8,
        3,
        8,
        0,
        3,
        0,
        8,
        source_unit_ordinal=1,
    )
    child = NativeNode(
        1,
        NativeNodeKind.PARAGRAPH,
        0,
        child_position,
        fragments=(
            NativeFragment(
                0,
                "value",
                NativeTransformKind.IDENTITY,
                child_position,
                NativeFragmentRole.TEXT,
            ),
        ),
    )
    document = NativeDocument(
        source_format=ParserFormat.DOCX,
        source_bytes=len(source),
        source_sha256=source_sha256,
        source_units=(
            NativeSourceUnit(
                0,
                NativeSourceUnitKind.RAW_FILE,
                None,
                len(source),
                source_sha256,
                None,
                None,
            ),
            NativeSourceUnit(
                1,
                NativeSourceUnitKind.OOXML_PART,
                "/word/document.xml",
                len(part),
                part_sha256,
                "utf-8",
                len(part.decode()),
            ),
        ),
        nodes=(root, child),
    )

    observed = NativeDocument.from_bytes(document.canonical_bytes)

    assert observed == document
    observed.validate_source_binding(source, expected_format=ParserFormat.DOCX)
    assert observed.source_encoding == "binary"
    assert observed.decoded_scalars == 0


def test_native_v2_rejects_invalid_source_units_and_cross_unit_identity() -> None:
    source = b"x"
    digest = hashlib.sha256(source).hexdigest()
    with pytest.raises(ValueError, match="ordinal zero"):
        NativeSourceUnit(
            1,
            NativeSourceUnitKind.RAW_FILE,
            None,
            1,
            digest,
            "utf-8",
            1,
        )
    with pytest.raises(ValueError, match="not canonical"):
        NativeSourceUnit(
            1,
            NativeSourceUnitKind.OOXML_PART,
            "word/../document.xml",
            1,
            digest,
            "utf-8",
            1,
        )
    with pytest.raises(ValueError, match="decoding metadata"):
        NativeSourceUnit(
            0,
            NativeSourceUnitKind.RAW_FILE,
            None,
            1,
            digest,
            "utf-16",
            1,
        )

    node_position = NativeSourcePosition(0, 1, 0, 1, 0, 0, 0, 1)
    other_position = replace(node_position, source_unit_ordinal=1)
    with pytest.raises(ValueError, match="cross-unit"):
        NativeNode(
            1,
            NativeNodeKind.TABLE_CELL,
            0,
            node_position,
            fragments=(
                NativeFragment(
                    0,
                    "x",
                    NativeTransformKind.IDENTITY,
                    other_position,
                ),
            ),
        )


@pytest.mark.parametrize("ordinal", [-1, True])
def test_native_fragment_rejects_invalid_ordinal(ordinal: int) -> None:
    position = NativeSourcePosition(0, 1, 0, 1, 0, 0, 0, 1)

    with pytest.raises(ValueError, match="ordinal"):
        NativeFragment(
            ordinal=ordinal,
            text="x",
            transform=NativeTransformKind.IDENTITY,
            source_position=position,
        )


@pytest.mark.parametrize("text", ["", "nul\x00", "\ud800"])
def test_native_fragment_rejects_empty_or_non_scalar_text(text: str) -> None:
    position = NativeSourcePosition(0, 1, 0, 1, 0, 0, 0, 1)

    with pytest.raises(ValueError, match="Unicode scalar|non-scalar"):
        NativeFragment(
            ordinal=0,
            text=text,
            transform=NativeTransformKind.IDENTITY,
            source_position=position,
        )


@pytest.mark.parametrize("name", ["", "Upper", "has-hyphen", "a" * 65])
def test_native_attribute_rejects_names_outside_closed_ascii_profile(
    name: str,
) -> None:
    with pytest.raises(ValueError, match="closed ASCII identifier"):
        NativeAttribute(name, None)


def test_native_attribute_rejects_non_scalar_values_and_text_nul() -> None:
    with pytest.raises(ValueError, match="unsupported scalar"):
        NativeAttribute("value", 1.5)  # type: ignore[arg-type]
    with pytest.raises(ValueError, match="non-scalar"):
        NativeAttribute("value", "nul\x00")

    assert attributes(z=None, a="", enabled=True, count=1) == (
        NativeAttribute("a", ""),
        NativeAttribute("count", 1),
        NativeAttribute("enabled", True),
        NativeAttribute("z", None),
    )


def test_native_node_rejects_invalid_identity_parent_and_fragment_bounds() -> None:
    node_position = NativeSourcePosition(0, 4, 0, 4, 0, 0, 0, 4)

    with pytest.raises(ValueError, match="node ordinal"):
        NativeNode(-1, NativeNodeKind.PARAGRAPH, None, node_position)
    with pytest.raises(ValueError, match="parent ordinal"):
        NativeNode(1, NativeNodeKind.PARAGRAPH, True, node_position)

    outside = NativeSourcePosition(3, 5, 3, 5, 0, 3, 0, 5)
    with pytest.raises(ValueError, match="outside its node"):
        NativeNode(
            1,
            NativeNodeKind.PARAGRAPH,
            0,
            node_position,
            fragments=(
                NativeFragment(
                    0,
                    "xx",
                    NativeTransformKind.IDENTITY,
                    outside,
                ),
            ),
        )


def test_native_node_rejects_overlapping_fragments_and_duplicate_attributes() -> None:
    node_position = NativeSourcePosition(0, 4, 0, 4, 0, 0, 0, 4)
    first = NativeFragment(
        0,
        "abc",
        NativeTransformKind.IDENTITY,
        NativeSourcePosition(0, 3, 0, 3, 0, 0, 0, 3),
    )
    overlapping = NativeFragment(
        1,
        "bc",
        NativeTransformKind.IDENTITY,
        NativeSourcePosition(2, 4, 2, 4, 0, 2, 0, 4),
    )

    with pytest.raises(ValueError, match="source ordered"):
        NativeNode(
            1,
            NativeNodeKind.PARAGRAPH,
            0,
            node_position,
            fragments=(first, overlapping),
        )
    with pytest.raises(ValueError, match="attributes"):
        NativeNode(
            1,
            NativeNodeKind.PARAGRAPH,
            0,
            node_position,
            attributes=(NativeAttribute("a", 1), NativeAttribute("a", 2)),
        )


@pytest.mark.parametrize(
    ("changes", "message"),
    [
        ({"schema_version": "parser-native-artifact.v1"}, "schema version"),
        ({"source_format": ParserFormat.PDF}, "unsupported format"),
        ({"source_bytes": True}, "source byte count"),
        ({"source_sha256": "A" * 64}, "source hash"),
        ({"source_units": ()}, "source-unit ordinals"),
        ({"nodes": ()}, "node ordinals"),
    ],
)
def test_native_document_rejects_invalid_metadata(
    changes: dict[str, object],
    message: str,
) -> None:
    artifact = parse_txt(decode_source(b"x"))

    with pytest.raises(ValueError, match=message):
        replace(artifact, **cast("dict[str, Any]", changes))


def test_native_document_rejects_invalid_root_bounds_and_parent_containment() -> None:
    source = b"abcd"
    artifact = parse_txt(decode_source(source))
    root_position = NativeSourcePosition(0, 4, 0, 4, 0, 0, 0, 4)

    wrong_root = NativeNode(0, NativeNodeKind.PARAGRAPH, None, root_position)
    with pytest.raises(ValueError, match="document root"):
        replace(artifact, nodes=(wrong_root,))

    root = NativeNode(0, NativeNodeKind.DOCUMENT, None, root_position)
    beyond_source = NativeNode(
        1,
        NativeNodeKind.PARAGRAPH,
        0,
        NativeSourcePosition(0, 5, 0, 4, 0, 0, 0, 4),
    )
    with pytest.raises(ValueError, match="source-unit byte bounds"):
        replace(artifact, nodes=(root, beyond_source))

    narrow_parent = NativeNode(
        1,
        NativeNodeKind.QUOTE,
        0,
        NativeSourcePosition(0, 1, 0, 1, 0, 0, 0, 1),
    )
    escaped_child = NativeNode(
        2,
        NativeNodeKind.PARAGRAPH,
        1,
        NativeSourcePosition(2, 3, 2, 3, 0, 2, 0, 3),
    )
    with pytest.raises(ValueError, match="outside its parent"):
        replace(artifact, nodes=(root, narrow_parent, escaped_child))


def test_native_document_rejects_each_source_binding_mismatch() -> None:
    artifact = parse_txt(decode_source(b"x"))

    for source, expected_format in (
        (b"x", ParserFormat.HTML),
        (b"xx", ParserFormat.TXT),
        (b"y", ParserFormat.TXT),
    ):
        with pytest.raises(NativeArtifactError, match="source binding"):
            artifact.validate_source_binding(source, expected_format=expected_format)


def test_native_parse_failure_preserves_frozen_stable_code() -> None:
    observed = NativeParseFailure(StableErrorCode.INPUT_INVALID)

    assert str(observed) == "INPUT_INVALID"
    assert observed.code is StableErrorCode.INPUT_INVALID


def test_native_from_bytes_rejects_open_or_mistyped_container_shapes() -> None:
    with pytest.raises(NativeArtifactError, match="invalid"):
        NativeDocument.from_bytes(b"not-json")

    value = _artifact_object()
    value["extra"] = None
    with pytest.raises(NativeArtifactError, match="fields are not closed"):
        NativeDocument.from_bytes(canonical_json_bytes(value))

    value = _artifact_object()
    value["source"] = []
    with pytest.raises(NativeArtifactError, match="native source must be an object"):
        NativeDocument.from_bytes(canonical_json_bytes(value))

    value = _artifact_object()
    _object_field(value, "source")["extra"] = None
    with pytest.raises(NativeArtifactError, match="fields are not closed"):
        NativeDocument.from_bytes(canonical_json_bytes(value))

    value = _artifact_object()
    value["nodes"] = {}
    with pytest.raises(NativeArtifactError, match="nodes must be an array"):
        NativeDocument.from_bytes(canonical_json_bytes(value))


def test_native_from_bytes_rejects_open_nested_shapes() -> None:
    value = _artifact_object()
    _node_object(value, 0)["extra"] = None
    with pytest.raises(NativeArtifactError, match="node fields are not closed"):
        NativeDocument.from_bytes(canonical_json_bytes(value))

    value = _artifact_object()
    paragraph = _node_object(value, 1)
    fragment = _array_field(paragraph, "fragments")[0]
    assert isinstance(fragment, dict)
    fragment["extra"] = None
    with pytest.raises(NativeArtifactError, match="fragment fields are not closed"):
        NativeDocument.from_bytes(canonical_json_bytes(value))

    value = _artifact_object()
    paragraph = _node_object(value, 1)
    _array_field(paragraph, "attributes").append(
        {"extra": None, "name": "level", "value": 1},
    )
    with pytest.raises(NativeArtifactError, match="attribute fields are not closed"):
        NativeDocument.from_bytes(canonical_json_bytes(value))

    value = _artifact_object()
    _object_field(_node_object(value, 0), "sourcePosition").pop("endColumn")
    with pytest.raises(NativeArtifactError, match="text position fields"):
        NativeDocument.from_bytes(canonical_json_bytes(value))


def test_native_from_bytes_rejects_mistyped_closed_fields() -> None:
    value = _artifact_object()
    value["schemaVersion"] = 1
    with pytest.raises(NativeArtifactError, match="schemaVersion must be text"):
        NativeDocument.from_bytes(canonical_json_bytes(value))

    value = _artifact_object()
    _object_field(value, "source")["bytes"] = True
    with pytest.raises(NativeArtifactError, match="source.bytes must be an integer"):
        NativeDocument.from_bytes(canonical_json_bytes(value))

    value = _artifact_object()
    _node_object(value, 1)["parentOrdinal"] = True
    with pytest.raises(NativeArtifactError, match="parentOrdinal must be an integer"):
        NativeDocument.from_bytes(canonical_json_bytes(value))

    value = _artifact_object()
    paragraph = _node_object(value, 1)
    _array_field(paragraph, "attributes").append(
        {"name": "level", "value": []},
    )
    with pytest.raises(NativeArtifactError, match="value is not scalar"):
        NativeDocument.from_bytes(canonical_json_bytes(value))

    value = _artifact_object()
    position = _object_field(_node_object(value, 0), "sourcePosition")
    position["rawByteStart"] = False
    with pytest.raises(NativeArtifactError, match="rawByteStart must be an integer"):
        NativeDocument.from_bytes(canonical_json_bytes(value))


def test_native_from_bytes_round_trips_closed_attributes() -> None:
    value = _artifact_object()
    paragraph = _node_object(value, 1)
    _array_field(paragraph, "attributes").append(
        {"name": "level", "value": 1},
    )
    content = canonical_json_bytes(value)

    observed = NativeDocument.from_bytes(content)

    assert observed.nodes[1].attributes == (NativeAttribute("level", 1),)
    assert observed.canonical_bytes == content


def test_native_parser_profile_pins_components_dependencies_and_options() -> None:
    profile = native_parser_profile_manifest()

    formats = ["txt", "markdown", "html", "docx", "pptx", "xlsx", "csv"]
    assert profile["schemaVersion"] == "native-parser-profile.internal.v2"
    assert profile["artifactSchemaVersion"] == "parser-native-artifact.v2"
    assert profile["supportedFormats"] == formats
    assert profile["markdownOptions"] == {
        "breaks": False,
        "html": True,
        "linkify": False,
        "plugins": ["table"],
        "preset": "commonmark",
        "runtimePluginDiscovery": False,
        "typographer": False,
    }
    dependencies = profile["dependencies"]
    assert isinstance(dependencies, list)
    dependency_pairs: list[tuple[JsonValue, JsonValue]] = []
    for item in dependencies:
        assert isinstance(item, dict)
        dependency_pairs.append((item["name"], item["license"]))
    assert dependency_pairs == [
        ("markdown-it-py", "MIT"),
        ("mdurl", "MIT"),
    ]
    components = profile["components"]
    assert isinstance(components, list)
    component_formats: list[JsonValue] = []
    for component in components:
        assert isinstance(component, dict)
        component_formats.append(component["format"])
        digest = component["implementationArtifactSha256"]
        assert isinstance(digest, str)
        assert len(digest) == 64
        assert digest == digest.casefold()
        assert component["implementationVersion"] == "0.2.0"
        source_files = component["sourceFiles"]
        assert isinstance(source_files, list)
        assert "router.py" in source_files
        assert "canonical.py" in source_files
        assert "config.py" in source_files
        assert "errors.py" in source_files
        assert "native/model.py" in source_files
        assert "native/decoding.py" in source_files
        assert "native/dispatch.py" in source_files
        assert "native/internal_result.py" in source_files
        assert "native/profile.py" in source_files
        assert f"native/{component['format']}.py" in source_files
        if component["format"] in {"docx", "pptx", "xlsx"}:
            assert "native/opc.py" in source_files
            assert "native/xml_source.py" in source_files
    assert component_formats == formats
    assert DEFAULT_CONFIG.canonical_object()["nativeParserProfile"] == profile


def test_every_native_limit_is_bound_into_the_parser_config_hash() -> None:
    baseline = DEFAULT_CONFIG.config_hash

    for field in fields(DEFAULT_CONFIG.native):
        current = getattr(DEFAULT_CONFIG.native, field.name)
        changed_native = replace(
            DEFAULT_CONFIG.native,
            **{field.name: current + 1},
        )
        changed_config = replace(DEFAULT_CONFIG, native=changed_native)

        assert changed_config.config_hash != baseline, field.name
