"""Offline semantic proofs for normalization-map locator projection.

The packaged schemas close each artifact's shape.  This helper closes the
cross-artifact rules that a schema cannot express: document locators exactly
cover non-renderer text, projected views are derived only from normalization
map positions, and clipped child locators cannot expand their parent.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, replace
from typing import Final, Never, cast

from jsonschema.exceptions import ValidationError

from tests.support.parser_contracts import (
    JsonObject,
    JsonValue,
    PackagedSchemas,
    canonical_json_bytes,
    load_packaged_schemas,
    load_strict_json_bytes,
)
from tests.support.parser_normalization_semantics import (
    SOURCE_LOCATOR_AGGREGATE_DOMAIN,
    LocatorReferenceRegistry,
    NormalizationSemanticError,
    NormalizationSemanticFixture,
    aggregate_hash,
    validate_parser_normalization_semantics,
)

_FIXTURE_FIELDS: Final = frozenset(
    {
        "childLocators",
        "documentLocators",
        "knownReferences",
        "normalizationMap",
        "normalizationProfile",
        "sourceUnitResolver",
        "textBuffer",
    }
)
_REFERENCE_FIELDS: Final = frozenset(
    {
        "canonicalXPathPayloadRefs",
        "opaqueShapeIds",
        "opaqueSheetIds",
        "opaqueStructureIds",
        "ownerSeedIds",
    }
)
_DOCUMENT_LOCATOR_FIELDS: Final = frozenset({"locatorOrdinal", "locatorSet"})
_CHILD_LOCATOR_FIELDS: Final = frozenset(
    {"childOrdinal", "locatorSet", "parentLocatorOrdinal"}
)
_PROJECTED_VIEW_KINDS: Final = frozenset({"page_region", "source_text_position"})
_A1_CELL: Final = re.compile(r"^([A-Z]+)([1-9][0-9]*)$")


class ProjectionSemanticError(ValueError):
    """A schema-valid locator projection violates cross-artifact semantics."""


@dataclass(frozen=True, slots=True)
class DocumentProjectionLocator:
    """One ordered document-level locator set."""

    ordinal: int
    locator_set: JsonObject


@dataclass(frozen=True, slots=True)
class ChildProjectionLocator:
    """One clipped locator and the document locator that contains it."""

    ordinal: int
    parent_ordinal: int
    locator_set: JsonObject


@dataclass(frozen=True, slots=True)
class ProjectionSemanticFixture:
    """Closed fixture bundle for normalization-map projection proofs."""

    normalization: NormalizationSemanticFixture
    document_locators: tuple[DocumentProjectionLocator, ...]
    child_locators: tuple[ChildProjectionLocator, ...]


def parse_projection_semantic_fixture(content: bytes) -> ProjectionSemanticFixture:
    """Parse one exact-JCS projection fixture without semantic validation."""
    value = load_strict_json_bytes(content)
    document = _object(value, "$")
    _require_exact_fields(document, _FIXTURE_FIELDS, "$")
    references = _parse_references(document["knownReferences"])
    text = _string(document["textBuffer"], "$.textBuffer")
    normalization = NormalizationSemanticFixture(
        normalization_map=_object(document["normalizationMap"], "$.normalizationMap"),
        normalization_profile=_object(
            document["normalizationProfile"], "$.normalizationProfile"
        ),
        source_locator={},
        source_unit_resolver=_object(
            document["sourceUnitResolver"], "$.sourceUnitResolver"
        ),
        text_buffer=text.encode("utf-8"),
        references=references,
    )
    return ProjectionSemanticFixture(
        normalization=normalization,
        document_locators=_parse_document_locators(document["documentLocators"]),
        child_locators=_parse_child_locators(document["childLocators"]),
    )


def locator_aggregate_hash(locator_set: JsonObject) -> str:
    """Recompute a Source Locator v2 aggregate hash."""
    return aggregate_hash(locator_set, SOURCE_LOCATOR_AGGREGATE_DOMAIN)


def recompute_locator_hash(locator_set: JsonObject) -> None:
    """Replace one locator's aggregate hash after a test mutation."""
    locator_set["aggregateHash"] = locator_aggregate_hash(locator_set)


def validate_parser_projection_semantics(
    fixture: ProjectionSemanticFixture,
    *,
    packaged_schemas: PackagedSchemas | None = None,
) -> None:
    """Prove coverage, projection uniqueness, and child non-expansion."""
    packaged = packaged_schemas or load_packaged_schemas()
    if not fixture.document_locators:
        _fail("documentLocators", "at least one document locator is required")

    _validate_locator_ordinals(fixture)
    for label, locator_set in _all_locator_sets(fixture):
        _validate_text_only_locator(locator_set, label)
        _validate_local_artifacts(fixture, locator_set, packaged, label)

    _validate_document_coverage(fixture)
    for locator in fixture.document_locators:
        _validate_locator_projection(
            fixture,
            locator.locator_set,
            f"documentLocators[{locator.ordinal}].locatorSet",
        )
    _validate_children(fixture)


def _parse_references(value: JsonValue) -> LocatorReferenceRegistry:
    document = _object(value, "$.knownReferences")
    _require_exact_fields(document, _REFERENCE_FIELDS, "$.knownReferences")
    return LocatorReferenceRegistry(
        canonical_xpath_payload_refs=_reference_set(
            document["canonicalXPathPayloadRefs"],
            "$.knownReferences.canonicalXPathPayloadRefs",
        ),
        opaque_shape_ids=_reference_set(
            document["opaqueShapeIds"], "$.knownReferences.opaqueShapeIds"
        ),
        opaque_sheet_ids=_reference_set(
            document["opaqueSheetIds"], "$.knownReferences.opaqueSheetIds"
        ),
        opaque_structure_ids=_reference_set(
            document["opaqueStructureIds"],
            "$.knownReferences.opaqueStructureIds",
        ),
        owner_seed_ids=_reference_set(
            document["ownerSeedIds"], "$.knownReferences.ownerSeedIds"
        ),
    )


def _parse_document_locators(
    value: JsonValue,
) -> tuple[DocumentProjectionLocator, ...]:
    records = _objects(value, "$.documentLocators")
    result: list[DocumentProjectionLocator] = []
    for index, record in enumerate(records):
        path = f"$.documentLocators[{index}]"
        _require_exact_fields(record, _DOCUMENT_LOCATOR_FIELDS, path)
        result.append(
            DocumentProjectionLocator(
                ordinal=_integer(record["locatorOrdinal"], f"{path}.locatorOrdinal"),
                locator_set=_object(record["locatorSet"], f"{path}.locatorSet"),
            )
        )
    return tuple(result)


def _parse_child_locators(value: JsonValue) -> tuple[ChildProjectionLocator, ...]:
    records = _objects(value, "$.childLocators")
    result: list[ChildProjectionLocator] = []
    for index, record in enumerate(records):
        path = f"$.childLocators[{index}]"
        _require_exact_fields(record, _CHILD_LOCATOR_FIELDS, path)
        result.append(
            ChildProjectionLocator(
                ordinal=_integer(record["childOrdinal"], f"{path}.childOrdinal"),
                parent_ordinal=_integer(
                    record["parentLocatorOrdinal"],
                    f"{path}.parentLocatorOrdinal",
                ),
                locator_set=_object(record["locatorSet"], f"{path}.locatorSet"),
            )
        )
    return tuple(result)


def _validate_locator_ordinals(fixture: ProjectionSemanticFixture) -> None:
    for expected, locator in enumerate(fixture.document_locators):
        if locator.ordinal != expected:
            _fail(
                f"documentLocators[{expected}].locatorOrdinal",
                "document locator ordinals must be contiguous from zero",
            )
    child_keys: list[tuple[int, int, int]] = []
    for expected, child in enumerate(fixture.child_locators):
        if child.ordinal != expected:
            _fail(
                f"childLocators[{expected}].childOrdinal",
                "child locator ordinals must be contiguous from zero",
            )
        anchors = _text_anchors(
            child.locator_set,
            f"childLocators[{expected}].locatorSet",
        )
        if not anchors:
            _fail(
                f"childLocators[{expected}].locatorSet.textAnchors",
                "a child locator requires canonical text",
            )
        child_keys.append(
            (
                child.parent_ordinal,
                _anchor_start(anchors[0]),
                _anchor_end(anchors[-1]),
            )
        )
    if child_keys != sorted(child_keys) or len(child_keys) != len(set(child_keys)):
        _fail(
            "childLocators",
            "child locators must be unique and ordered within each parent",
        )


def _all_locator_sets(
    fixture: ProjectionSemanticFixture,
) -> tuple[tuple[str, JsonObject], ...]:
    documents = tuple(
        (
            f"documentLocators[{locator.ordinal}].locatorSet",
            locator.locator_set,
        )
        for locator in fixture.document_locators
    )
    children = tuple(
        (f"childLocators[{locator.ordinal}].locatorSet", locator.locator_set)
        for locator in fixture.child_locators
    )
    return documents + children


def _validate_text_only_locator(locator_set: JsonObject, path: str) -> None:
    structural = _objects(
        locator_set.get("structuralAnchors"), f"{path}.structuralAnchors"
    )
    if structural:
        _fail(
            f"{path}.structuralAnchors",
            "structural anchors must not cover or clip canonical text",
        )
    if not _text_anchors(locator_set, path):
        _fail(f"{path}.textAnchors", "projection locators require text anchors")


def _validate_local_artifacts(
    fixture: ProjectionSemanticFixture,
    locator_set: JsonObject,
    packaged: PackagedSchemas,
    path: str,
) -> None:
    local_fixture = replace(fixture.normalization, source_locator=locator_set)
    try:
        validate_parser_normalization_semantics(
            local_fixture,
            packaged_schemas=packaged,
        )
    except (NormalizationSemanticError, ValidationError) as error:
        raise ProjectionSemanticError(f"{path}: {error}") from error


def _validate_document_coverage(fixture: ProjectionSemanticFixture) -> None:
    observed: list[tuple[int, int]] = []
    previous_end = -1
    for locator in fixture.document_locators:
        path = f"documentLocators[{locator.ordinal}].locatorSet"
        for anchor in _text_anchors(locator.locator_set, path):
            start = _anchor_start(anchor)
            end = _anchor_end(anchor)
            if start < previous_end:
                _fail(
                    "documentLocators",
                    "document locator ranges overlap or are reordered",
                )
            observed.append((start, end))
            previous_end = end

    expected = _non_separator_ranges(fixture.normalization.normalization_map)
    if _merge_adjacent(observed) != expected:
        _fail(
            "documentLocators",
            "ordered locators leave a gap or cover a renderer separator",
        )


def _non_separator_ranges(normalization_map: JsonObject) -> list[tuple[int, int]]:
    ranges: list[tuple[int, int]] = []
    for segment in _segments(normalization_map):
        transform = _object(segment["transform"], "normalizationMap.segment.transform")
        if transform.get("kind") == "renderer_insert":
            continue
        ranges.append((_segment_start(segment), _segment_end(segment)))
    return _merge_adjacent(ranges)


def _merge_adjacent(ranges: list[tuple[int, int]]) -> list[tuple[int, int]]:
    merged: list[tuple[int, int]] = []
    for start, end in ranges:
        if merged and merged[-1][1] == start:
            merged[-1] = (merged[-1][0], end)
        else:
            merged.append((start, end))
    return merged


def _validate_locator_projection(
    fixture: ProjectionSemanticFixture,
    locator_set: JsonObject,
    path: str,
) -> None:
    for index, anchor in enumerate(_text_anchors(locator_set, path)):
        anchor_path = f"{path}.textAnchors[{index}]"
        expected = _project_range(
            fixture.normalization.normalization_map,
            fixture.normalization.text_buffer,
            _anchor_start(anchor),
            _anchor_end(anchor),
            anchor_path,
        )
        fragments = _objects(
            anchor["sourceFragments"], f"{anchor_path}.sourceFragments"
        )
        if len(fragments) != len(expected):
            _fail(
                f"{anchor_path}.sourceFragments",
                "fragment count differs from the unique normalization-map projection",
            )
        for fragment_index, (fragment, expected_view) in enumerate(
            zip(fragments, expected, strict=True)
        ):
            fragment_path = f"{anchor_path}.sourceFragments[{fragment_index}]"
            views = _objects(fragment["views"], f"{fragment_path}.views")
            projected = [
                view for view in views if view.get("kind") in _PROJECTED_VIEW_KINDS
            ]
            if len(projected) != 1:
                _fail(
                    f"{fragment_path}.views",
                    "each fragment requires exactly one map-projected text/page view",
                )
            observed_bytes = canonical_json_bytes(cast("JsonValue", projected[0]))
            expected_bytes = canonical_json_bytes(cast("JsonValue", expected_view))
            if observed_bytes != expected_bytes:
                _fail(
                    f"{fragment_path}.views",
                    "text/page view differs from the unique map projection",
                )


def _project_range(
    normalization_map: JsonObject,
    text_buffer: bytes,
    start: int,
    end: int,
    path: str,
) -> list[JsonObject]:
    positions: list[JsonObject] = []
    covered = start
    for segment in _segments(normalization_map):
        segment_start = _segment_start(segment)
        segment_end = _segment_end(segment)
        clip_start = max(start, segment_start)
        clip_end = min(end, segment_end)
        if clip_start >= clip_end:
            continue
        if clip_start != covered:
            _fail(path, "projection range crosses an uncovered canonical gap")
        transform = _object(segment["transform"], "normalizationMap.segment.transform")
        kind = _string(transform["kind"], "normalizationMap.segment.transform.kind")
        if kind == "renderer_insert":
            _fail(path, "text locators must not project renderer separators")
        positions.extend(
            _project_segment(
                transform,
                text_buffer[segment_start:segment_end],
                segment_start,
                segment_end,
                clip_start,
                clip_end,
                path,
            )
        )
        covered = clip_end
    if covered != end:
        _fail(path, "projection range is outside normalization-map coverage")
    return [_position_to_view(position) for position in _coalesce_positions(positions)]


def _project_segment(
    transform: JsonObject,
    canonical_bytes: bytes,
    segment_start: int,
    segment_end: int,
    clip_start: int,
    clip_end: int,
    path: str,
) -> list[JsonObject]:
    kind = _string(transform["kind"], "normalizationMap.segment.transform.kind")
    whole_segment = clip_start == segment_start and clip_end == segment_end
    positions = _objects(
        transform.get("sourcePositions"),
        "normalizationMap.segment.transform.sourcePositions",
    )
    if kind == "identity":
        return _project_identity(
            positions,
            canonical_bytes,
            segment_start,
            clip_start,
            clip_end,
            path,
        )
    if kind in {"newline_fold", "nfc_compose"}:
        if not whole_segment:
            _fail(path, f"locator split is not legal inside {kind}")
        return [position.copy() for position in positions]
    if kind == "syntax_decode":
        if whole_segment:
            return [position.copy() for position in positions]
        return _project_syntax_subsegments(
            transform,
            segment_start,
            clip_start,
            clip_end,
            path,
        )
    return _fail(path, f"unsupported projected transform {kind!r}")


def _project_identity(
    positions: list[JsonObject],
    canonical_bytes: bytes,
    segment_start: int,
    clip_start: int,
    clip_end: int,
    path: str,
) -> list[JsonObject]:
    kinds = {position.get("kind") for position in positions}
    if kinds == {"page_geometry_position"}:
        if clip_start != segment_start or clip_end != segment_start + len(
            canonical_bytes
        ):
            _fail(path, "page geometry identity segments are atomic")
        return [position.copy() for position in positions]
    if kinds != {"text_position"}:
        return _fail(path, "identity projection requires one position kind")

    text = canonical_bytes.decode("utf-8", errors="strict")
    scalar_lengths = [
        _integer(position["decodedScalarEnd"], "position.decodedScalarEnd")
        - _integer(position["decodedScalarStart"], "position.decodedScalarStart")
        for position in positions
    ]
    if sum(scalar_lengths) != len(text):
        _fail(path, "identity positions do not uniquely partition canonical scalars")

    result: list[JsonObject] = []
    scalar_cursor = 0
    for position, scalar_length in zip(positions, scalar_lengths, strict=True):
        piece = text[scalar_cursor : scalar_cursor + scalar_length]
        piece_start = segment_start + len(text[:scalar_cursor].encode("utf-8"))
        piece_end = piece_start + len(piece.encode("utf-8"))
        selected_start = max(clip_start, piece_start)
        selected_end = min(clip_end, piece_end)
        if selected_start < selected_end:
            result.append(
                _clip_identity_text_position(
                    position,
                    piece,
                    piece_start,
                    selected_start,
                    selected_end,
                    path,
                )
            )
        scalar_cursor += scalar_length
    return result


def _clip_identity_text_position(
    position: JsonObject,
    canonical_piece: str,
    piece_start: int,
    clip_start: int,
    clip_end: int,
    path: str,
) -> JsonObject:
    boundaries = _scalar_byte_boundaries(canonical_piece)
    relative_start = clip_start - piece_start
    relative_end = clip_end - piece_start
    if relative_start not in boundaries or relative_end not in boundaries:
        _fail(path, "identity split is not on a UTF-8 scalar boundary")
    scalar_start = boundaries.index(relative_start)
    scalar_end = boundaries.index(relative_end)
    raw_start = _integer(position["rawByteStart"], "position.rawByteStart")
    raw_end = _integer(position["rawByteEnd"], "position.rawByteEnd")
    if raw_end - raw_start != len(canonical_piece.encode("utf-8")):
        _fail(path, "identity raw bytes cannot be linearly clipped")

    source_start = (
        _integer(position["startLine"], "position.startLine"),
        _integer(position["startColumn"], "position.startColumn"),
    )
    expected_source_end = _advance_text_position(source_start, canonical_piece)
    observed_source_end = (
        _integer(position["endLine"], "position.endLine"),
        _integer(position["endColumn"], "position.endColumn"),
    )
    if observed_source_end != expected_source_end:
        _fail(path, "identity line/column range cannot be linearly clipped")

    prefix = canonical_piece[:scalar_start]
    selected = canonical_piece[scalar_start:scalar_end]
    clipped = position.copy()
    clipped_raw_start = raw_start + len(prefix.encode("utf-8"))
    clipped["rawByteStart"] = clipped_raw_start
    clipped["rawByteEnd"] = clipped_raw_start + len(selected.encode("utf-8"))
    decoded_start = _integer(
        position["decodedScalarStart"], "position.decodedScalarStart"
    )
    clipped["decodedScalarStart"] = decoded_start + scalar_start
    clipped["decodedScalarEnd"] = decoded_start + scalar_end
    start_line, start_column = _advance_text_position(source_start, prefix)
    end_line, end_column = _advance_text_position((start_line, start_column), selected)
    clipped["startLine"] = start_line
    clipped["startColumn"] = start_column
    clipped["endLine"] = end_line
    clipped["endColumn"] = end_column
    return clipped


def _project_syntax_subsegments(
    transform: JsonObject,
    segment_start: int,
    clip_start: int,
    clip_end: int,
    path: str,
) -> list[JsonObject]:
    raw_subsegments = transform.get("subsegments")
    if raw_subsegments is None:
        return _fail(path, "locator split is not legal inside atomic syntax_decode")
    subsegments = _objects(raw_subsegments, "normalizationMap.syntax.subsegments")
    boundaries = {segment_start}
    for subsegment in subsegments:
        boundaries.add(
            segment_start
            + _integer(
                subsegment["relativeCanonicalEnd"],
                "normalizationMap.syntax.subsegment.relativeCanonicalEnd",
            )
        )
    if clip_start not in boundaries or clip_end not in boundaries:
        _fail(path, "syntax_decode split is not on a validated subsegment boundary")
    result: list[JsonObject] = []
    covered = clip_start
    for subsegment in subsegments:
        sub_start = segment_start + _integer(
            subsegment["relativeCanonicalStart"],
            "normalizationMap.syntax.subsegment.relativeCanonicalStart",
        )
        sub_end = segment_start + _integer(
            subsegment["relativeCanonicalEnd"],
            "normalizationMap.syntax.subsegment.relativeCanonicalEnd",
        )
        if sub_start >= clip_start and sub_end <= clip_end:
            if sub_start != covered:
                _fail(path, "selected syntax subsegments are not an exact cover")
            result.extend(
                position.copy()
                for position in _objects(
                    subsegment["sourcePositions"],
                    "normalizationMap.syntax.subsegment.sourcePositions",
                )
            )
            covered = sub_end
    if covered != clip_end:
        _fail(path, "selected syntax subsegments are not an exact cover")
    return result


def _scalar_byte_boundaries(value: str) -> list[int]:
    boundaries = [0]
    cursor = 0
    for character in value:
        cursor += len(character.encode("utf-8"))
        boundaries.append(cursor)
    return boundaries


def _advance_text_position(start: tuple[int, int], value: str) -> tuple[int, int]:
    line, column = start
    for character in value:
        if character == "\n":
            line += 1
            column = 0
        else:
            column += 1
    return line, column


def _coalesce_positions(positions: list[JsonObject]) -> list[JsonObject]:
    result: list[JsonObject] = []
    for position in positions:
        current = position.copy()
        if result and _text_positions_mergeable(result[-1], current):
            previous = result[-1]
            for field in ("rawByteEnd", "decodedScalarEnd", "endLine", "endColumn"):
                previous[field] = current[field]
            continue
        result.append(current)
    return result


def _text_positions_mergeable(left: JsonObject, right: JsonObject) -> bool:
    return (
        left.get("kind") == "text_position"
        and right.get("kind") == "text_position"
        and left.get("opaqueSourceUnitId") == right.get("opaqueSourceUnitId")
        and left.get("rawByteEnd") == right.get("rawByteStart")
        and left.get("decodedScalarEnd") == right.get("decodedScalarStart")
        and left.get("endLine") == right.get("startLine")
        and left.get("endColumn") == right.get("startColumn")
    )


def _position_to_view(position: JsonObject) -> JsonObject:
    kind = position.get("kind")
    if kind == "text_position":
        view = {
            key: value
            for key, value in position.items()
            if key not in {"kind", "positionOrdinal"}
        }
        view["kind"] = "source_text_position"
        return view
    if kind == "page_geometry_position":
        return {
            "bboxMilliPoint": position["bboxMilliPoint"],
            "kind": "page_region",
            "pageIndex": position["pageIndex"],
        }
    return _fail("normalizationMap", f"unsupported source position {kind!r}")


def _validate_children(fixture: ProjectionSemanticFixture) -> None:
    parents = {locator.ordinal: locator for locator in fixture.document_locators}
    for child in fixture.child_locators:
        path = f"childLocators[{child.ordinal}]"
        parent = parents.get(child.parent_ordinal)
        if parent is None:
            _fail(
                f"{path}.parentLocatorOrdinal",
                "child references an unknown parent locator",
            )
        _validate_child_canonical_subset(child, parent, path)
        _validate_locator_projection(
            fixture,
            child.locator_set,
            f"{path}.locatorSet",
        )
        _validate_child_fragment_subset(child, parent, path)


def _validate_child_canonical_subset(
    child: ChildProjectionLocator,
    parent: DocumentProjectionLocator,
    path: str,
) -> None:
    parent_anchors = _text_anchors(parent.locator_set, "parent.locatorSet")
    for index, child_anchor in enumerate(
        _text_anchors(child.locator_set, f"{path}.locatorSet")
    ):
        child_start = _anchor_start(child_anchor)
        child_end = _anchor_end(child_anchor)
        containing = [
            anchor
            for anchor in parent_anchors
            if _anchor_start(anchor) <= child_start and child_end <= _anchor_end(anchor)
        ]
        if len(containing) != 1:
            _fail(
                f"{path}.locatorSet.textAnchors[{index}]",
                "child canonical range expands or crosses its parent",
            )


def _validate_child_fragment_subset(
    child: ChildProjectionLocator,
    parent: DocumentProjectionLocator,
    path: str,
) -> None:
    parent_anchors = _text_anchors(parent.locator_set, "parent.locatorSet")
    for child_anchor in _text_anchors(child.locator_set, f"{path}.locatorSet"):
        parent_anchor = next(
            anchor
            for anchor in parent_anchors
            if _anchor_start(anchor) <= _anchor_start(child_anchor)
            and _anchor_end(child_anchor) <= _anchor_end(anchor)
        )
        parent_fragments = _objects(
            parent_anchor["sourceFragments"], "parent.sourceFragments"
        )
        child_fragments = _objects(
            child_anchor["sourceFragments"], "child.sourceFragments"
        )
        parent_cursor = 0
        for fragment_index, child_fragment in enumerate(child_fragments):
            match = _next_containing_fragment(
                parent_fragments,
                child_fragment,
                parent_cursor,
            )
            if match is None:
                _fail(
                    f"{path}.locatorSet.sourceFragments[{fragment_index}]",
                    "child fragments/views expand or reorder the parent projection",
                )
            parent_cursor = match + 1


def _next_containing_fragment(
    parents: list[JsonObject],
    child: JsonObject,
    start: int,
) -> int | None:
    for index in range(start, len(parents)):
        if _fragment_contains(parents[index], child):
            return index
    return None


def _fragment_contains(parent: JsonObject, child: JsonObject) -> bool:
    parent_views = {
        _string(view["kind"], "parent.view.kind"): view
        for view in _objects(parent["views"], "parent.views")
    }
    for child_view in _objects(child["views"], "child.views"):
        kind = _string(child_view["kind"], "child.view.kind")
        parent_view = parent_views.get(kind)
        if parent_view is None or not _view_contains(parent_view, child_view):
            return False
    return True


def _view_contains(parent: JsonObject, child: JsonObject) -> bool:  # noqa: PLR0911
    kind = parent.get("kind")
    if kind != child.get("kind"):
        return False
    if kind == "source_text_position":
        return (
            parent.get("opaqueSourceUnitId") == child.get("opaqueSourceUnitId")
            and _range_contains(parent, child, "rawByteStart", "rawByteEnd")
            and _range_contains(
                parent,
                child,
                "decodedScalarStart",
                "decodedScalarEnd",
            )
            and (
                _integer(parent["startLine"], "parent.startLine"),
                _integer(parent["startColumn"], "parent.startColumn"),
            )
            <= (
                _integer(child["startLine"], "child.startLine"),
                _integer(child["startColumn"], "child.startColumn"),
            )
            and (
                _integer(child["endLine"], "child.endLine"),
                _integer(child["endColumn"], "child.endColumn"),
            )
            <= (
                _integer(parent["endLine"], "parent.endLine"),
                _integer(parent["endColumn"], "parent.endColumn"),
            )
        )
    if kind == "page_region":
        return parent.get("pageIndex") == child.get("pageIndex") and _bbox_contains(
            parent["bboxMilliPoint"], child["bboxMilliPoint"]
        )
    if kind == "slide_shape":
        if parent.get("slideIndex") != child.get("slideIndex") or parent.get(
            "opaqueShapeId"
        ) != child.get("opaqueShapeId"):
            return False
        parent_bbox = parent.get("bboxMilliPoint")
        child_bbox = child.get("bboxMilliPoint")
        return child_bbox is None or (
            parent_bbox is not None and _bbox_contains(parent_bbox, child_bbox)
        )
    if kind == "sheet_range":
        return _sheet_range_contains(parent, child)
    return canonical_json_bytes(cast("JsonValue", parent)) == canonical_json_bytes(
        cast("JsonValue", child)
    )


def _range_contains(
    parent: JsonObject,
    child: JsonObject,
    start_field: str,
    end_field: str,
) -> bool:
    return (
        _integer(parent[start_field], f"parent.{start_field}")
        <= _integer(child[start_field], f"child.{start_field}")
        < _integer(child[end_field], f"child.{end_field}")
        <= _integer(parent[end_field], f"parent.{end_field}")
    )


def _bbox_contains(parent_value: JsonValue, child_value: JsonValue) -> bool:
    parent = _integer_list(parent_value, "parent.bboxMilliPoint", length=4)
    child = _integer_list(child_value, "child.bboxMilliPoint", length=4)
    return (
        parent[0] <= child[0]
        and parent[1] <= child[1]
        and child[2] <= parent[2]
        and child[3] <= parent[3]
    )


def _sheet_range_contains(parent: JsonObject, child: JsonObject) -> bool:
    if parent.get("opaqueSheetId") != child.get("opaqueSheetId"):
        return False
    parent_start = _a1_key(_string(parent["startCell"], "parent.startCell"))
    parent_end = _a1_key(_string(parent["endCell"], "parent.endCell"))
    child_start = _a1_key(_string(child["startCell"], "child.startCell"))
    child_end = _a1_key(_string(child["endCell"], "child.endCell"))
    return (
        parent_start[0] <= child_start[0]
        and parent_start[1] <= child_start[1]
        and child_end[0] <= parent_end[0]
        and child_end[1] <= parent_end[1]
    )


def _a1_key(value: str) -> tuple[int, int]:
    match = _A1_CELL.fullmatch(value)
    if match is None:
        return _fail("sheet_range", "invalid A1 cell")
    column = 0
    for character in match.group(1):
        column = column * 26 + ord(character) - ord("A") + 1
    return column, int(match.group(2))


def _segments(normalization_map: JsonObject) -> list[JsonObject]:
    return _objects(normalization_map["segments"], "normalizationMap.segments")


def _segment_start(segment: JsonObject) -> int:
    return _integer(segment["canonicalStartByte"], "segment.canonicalStartByte")


def _segment_end(segment: JsonObject) -> int:
    return _integer(segment["canonicalEndByte"], "segment.canonicalEndByte")


def _text_anchors(locator_set: JsonObject, path: str) -> list[JsonObject]:
    return _objects(locator_set.get("textAnchors"), f"{path}.textAnchors")


def _anchor_start(anchor: JsonObject) -> int:
    return _integer(anchor["canonicalStartByte"], "anchor.canonicalStartByte")


def _anchor_end(anchor: JsonObject) -> int:
    return _integer(anchor["canonicalEndByte"], "anchor.canonicalEndByte")


def _require_exact_fields(
    value: JsonObject,
    expected: frozenset[str],
    path: str,
) -> None:
    observed = frozenset(value)
    if observed != expected:
        missing = sorted(expected - observed)
        extra = sorted(observed - expected)
        _fail(path, f"closed fields differ; missing={missing}, extra={extra}")


def _reference_set(value: JsonValue, path: str) -> frozenset[str]:
    values = _list(value, path)
    result = tuple(_string(item, f"{path}[]") for item in values)
    if len(result) != len(set(result)) or list(result) != sorted(result):
        _fail(path, "references must be unique and sorted")
    return frozenset(result)


def _object(value: JsonValue, path: str) -> JsonObject:
    if not isinstance(value, dict):
        return _fail(path, "expected object")
    return value


def _objects(value: JsonValue | None, path: str) -> list[JsonObject]:
    return [_object(item, f"{path}[]") for item in _list(value, path)]


def _list(value: JsonValue | None, path: str) -> list[JsonValue]:
    if not isinstance(value, list):
        return _fail(path, "expected array")
    return value


def _integer_list(value: JsonValue, path: str, *, length: int) -> list[int]:
    values = [_integer(item, f"{path}[]") for item in _list(value, path)]
    if len(values) != length:
        _fail(path, f"expected {length} integers")
    return values


def _string(value: JsonValue, path: str) -> str:
    if not isinstance(value, str):
        return _fail(path, "expected string")
    return value


def _integer(value: JsonValue, path: str) -> int:
    if type(value) is not int:
        return _fail(path, "expected integer")
    return value


def _fail(path: str, message: str) -> Never:
    raise ProjectionSemanticError(f"{path}: {message}")
