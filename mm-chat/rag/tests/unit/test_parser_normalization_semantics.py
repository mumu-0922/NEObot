from __future__ import annotations

import copy
from collections.abc import Callable
from dataclasses import replace
from pathlib import Path

import pytest
from jsonschema.exceptions import ValidationError

from tests.support.parser_contracts import (
    ContractProfileError,
    canonical_json_bytes,
    load_packaged_schemas,
    load_strict_json_bytes,
)
from tests.support.parser_normalization_semantics import (
    NORMALIZATION_MAP_AGGREGATE_DOMAIN,
    SOURCE_LOCATOR_AGGREGATE_DOMAIN,
    SOURCE_UNIT_RESOLVER_AGGREGATE_DOMAIN,
    NormalizationSemanticError,
    NormalizationSemanticFixture,
    aggregate_hash,
    normalization_profile_hash,
    parse_normalization_semantic_fixture,
    payload_hash,
    validate_parser_normalization_semantics,
)

_FIXTURE_ROOT = (
    Path(__file__).parents[1]
    / "fixtures"
    / "parser_contracts"
    / "semantic_instances"
    / "normalization"
)
_POSITIVE_FIXTURES = frozenset(
    {
        "all-transform-and-view-variants.v1.json",
        "empty-text-structural-locator.v1.json",
    }
)
_NEGATIVE_FIXTURES = {
    "cycle-like-fragment-rewind.v1.json": "cycle-like source rewind",
    "gap-segment-cover.v1.json": "gapless non-overlapping exact cover",
    "locator-aggregate-hash.v1.json": "sourceLocator.aggregateHash",
    "missing-source-unit-ref.v1.json": "does not resolve",
    "ordering-source-units.v1.json": "sourceUnitOrdinal values must be contiguous",
}

type FixtureMutation = Callable[[NormalizationSemanticFixture], None]


def _fixture(path: Path) -> NormalizationSemanticFixture:
    return parse_normalization_semantic_fixture(path.read_bytes())


def _all_variants_fixture() -> NormalizationSemanticFixture:
    return copy.deepcopy(
        _fixture(_FIXTURE_ROOT / "positive/all-transform-and-view-variants.v1.json")
    )


def _rehash_map(fixture: NormalizationSemanticFixture) -> None:
    fixture.normalization_map["aggregateHash"] = aggregate_hash(
        fixture.normalization_map,
        NORMALIZATION_MAP_AGGREGATE_DOMAIN,
    )


def _rehash_resolver(fixture: NormalizationSemanticFixture) -> None:
    fixture.source_unit_resolver["aggregateHash"] = aggregate_hash(
        fixture.source_unit_resolver,
        SOURCE_UNIT_RESOLVER_AGGREGATE_DOMAIN,
    )


def _rehash_locator(fixture: NormalizationSemanticFixture) -> None:
    fixture.source_locator["aggregateHash"] = aggregate_hash(
        fixture.source_locator,
        SOURCE_LOCATOR_AGGREGATE_DOMAIN,
    )


def _rehash_profile_and_map(fixture: NormalizationSemanticFixture) -> None:
    fixture.normalization_map["normalizationProfileHash"] = normalization_profile_hash(
        fixture.normalization_profile
    )
    _rehash_map(fixture)


def _map_segments(fixture: NormalizationSemanticFixture) -> list[dict[str, object]]:
    return fixture.normalization_map["segments"]  # type: ignore[return-value]


def _resolver_entries(fixture: NormalizationSemanticFixture) -> list[dict[str, object]]:
    return fixture.source_unit_resolver["entries"]  # type: ignore[return-value]


def _text_anchors(fixture: NormalizationSemanticFixture) -> list[dict[str, object]]:
    return fixture.source_locator["textAnchors"]  # type: ignore[return-value]


def _mutate_segment_ordinal(fixture: NormalizationSemanticFixture) -> None:
    _map_segments(fixture)[1]["segmentOrdinal"] = 2
    _rehash_map(fixture)


def _mutate_source_unit_rank_order(fixture: NormalizationSemanticFixture) -> None:
    entries = _resolver_entries(fixture)
    layout_payload = entries[2]["payload"]
    middle_payload = entries[3]["payload"]
    assert isinstance(layout_payload, dict)
    assert isinstance(middle_payload, dict)
    layout_payload["role"] = "middle"
    middle_payload["role"] = "layout"
    entries[2]["payloadHash"] = payload_hash(layout_payload)
    entries[3]["payloadHash"] = payload_hash(middle_payload)
    _rehash_resolver(fixture)


def _mutate_resolver_order(fixture: NormalizationSemanticFixture) -> None:
    entries = _resolver_entries(fixture)
    entries[1], entries[2] = entries[2], entries[1]
    _rehash_resolver(fixture)


def _mutate_resolver_payload_ordinal(fixture: NormalizationSemanticFixture) -> None:
    _resolver_entries(fixture)[1]["payloadOrdinal"] = 1
    _rehash_resolver(fixture)


def _mutate_resolver_count(fixture: NormalizationSemanticFixture) -> None:
    fixture.source_unit_resolver["entryCount"] = 3
    _rehash_resolver(fixture)


def _mutate_payload_hash(fixture: NormalizationSemanticFixture) -> None:
    payload = _resolver_entries(fixture)[0]["payload"]
    assert isinstance(payload, dict)
    payload["displayName"] = "renamed.txt"
    _rehash_resolver(fixture)


def _mutate_source_position_bounds(fixture: NormalizationSemanticFixture) -> None:
    transform = _map_segments(fixture)[0]["transform"]
    assert isinstance(transform, dict)
    positions = transform["sourcePositions"]
    assert isinstance(positions, list)
    position = positions[0]
    assert isinstance(position, dict)
    position["rawByteEnd"] = 65
    _rehash_map(fixture)


def _mutate_text_position_order(fixture: NormalizationSemanticFixture) -> None:
    transform = _map_segments(fixture)[0]["transform"]
    assert isinstance(transform, dict)
    source_id = _resolver_entries(fixture)[0]["opaqueSourceUnitId"]
    transform["sourcePositions"] = [
        {
            "decodedScalarEnd": 3,
            "decodedScalarStart": 2,
            "endColumn": 3,
            "endLine": 0,
            "kind": "text_position",
            "opaqueSourceUnitId": source_id,
            "positionOrdinal": 0,
            "rawByteEnd": 3,
            "rawByteStart": 2,
            "startColumn": 2,
            "startLine": 0,
        },
        {
            "decodedScalarEnd": 1,
            "decodedScalarStart": 0,
            "endColumn": 1,
            "endLine": 0,
            "kind": "text_position",
            "opaqueSourceUnitId": source_id,
            "positionOrdinal": 1,
            "rawByteEnd": 1,
            "rawByteStart": 0,
            "startColumn": 0,
            "startLine": 0,
        },
    ]
    _rehash_map(fixture)


def _mutate_geometry_position_order(fixture: NormalizationSemanticFixture) -> None:
    transform = _map_segments(fixture)[5]["transform"]
    assert isinstance(transform, dict)
    positions = transform["sourcePositions"]
    assert isinstance(positions, list)
    positions.reverse()
    for ordinal, position in enumerate(positions):
        assert isinstance(position, dict)
        position["positionOrdinal"] = ordinal
    _rehash_map(fixture)


def _mutate_text_position_overlap(fixture: NormalizationSemanticFixture) -> None:
    transform = _map_segments(fixture)[0]["transform"]
    assert isinstance(transform, dict)
    source_id = _resolver_entries(fixture)[0]["opaqueSourceUnitId"]
    transform["sourcePositions"] = [
        {
            "decodedScalarEnd": 3,
            "decodedScalarStart": 0,
            "endColumn": 3,
            "endLine": 0,
            "kind": "text_position",
            "opaqueSourceUnitId": source_id,
            "positionOrdinal": 0,
            "rawByteEnd": 3,
            "rawByteStart": 0,
            "startColumn": 0,
            "startLine": 0,
        },
        {
            "decodedScalarEnd": 4,
            "decodedScalarStart": 2,
            "endColumn": 4,
            "endLine": 0,
            "kind": "text_position",
            "opaqueSourceUnitId": source_id,
            "positionOrdinal": 1,
            "rawByteEnd": 4,
            "rawByteStart": 2,
            "startColumn": 2,
            "startLine": 0,
        },
    ]
    _rehash_map(fixture)


def _mutate_position_kind(fixture: NormalizationSemanticFixture) -> None:
    transform = _map_segments(fixture)[3]["transform"]
    assert isinstance(transform, dict)
    subsegments = transform["subsegments"]
    assert isinstance(subsegments, list)
    subsegment = subsegments[0]
    assert isinstance(subsegment, dict)
    source_id = _resolver_entries(fixture)[2]["opaqueSourceUnitId"]
    subsegment["sourcePositions"] = [
        {
            "bboxMilliPoint": [0, 0, 100, 100],
            "fragmentReadingOrdinal": 0,
            "kind": "page_geometry_position",
            "opaqueSourceUnitId": source_id,
            "pageIndex": 0,
            "positionOrdinal": 0,
        }
    ]
    _rehash_map(fixture)


def _mutate_subsegment_gap(fixture: NormalizationSemanticFixture) -> None:
    transform = _map_segments(fixture)[3]["transform"]
    assert isinstance(transform, dict)
    subsegments = transform["subsegments"]
    assert isinstance(subsegments, list)
    second = subsegments[1]
    assert isinstance(second, dict)
    second["relativeCanonicalStart"] = 0
    _rehash_map(fixture)


def _mutate_parent_coalesce(fixture: NormalizationSemanticFixture) -> None:
    transform = _map_segments(fixture)[3]["transform"]
    assert isinstance(transform, dict)
    subsegments = transform["subsegments"]
    assert isinstance(subsegments, list)
    second = subsegments[1]
    assert isinstance(second, dict)
    positions = second["sourcePositions"]
    assert isinstance(positions, list)
    position = positions[0]
    assert isinstance(position, dict)
    position.update(
        {
            "decodedScalarStart": 9,
            "rawByteStart": 11,
            "startColumn": 7,
        }
    )
    _rehash_map(fixture)


def _mutate_newline_recipe_ref(fixture: NormalizationSemanticFixture) -> None:
    transform = _map_segments(fixture)[1]["transform"]
    assert isinstance(transform, dict)
    transform["newlineRecipeId"] = "missing"
    _rehash_map(fixture)


def _mutate_renderer_profile_marker(fixture: NormalizationSemanticFixture) -> None:
    transform = _map_segments(fixture)[4]["transform"]
    assert isinstance(transform, dict)
    transform["rendererProfileHash"] = "1" * 64
    _rehash_map(fixture)


def _mutate_utf8_boundary(fixture: NormalizationSemanticFixture) -> None:
    _map_segments(fixture)[2]["canonicalEndByte"] = 3
    _rehash_map(fixture)


def _mutate_anchor_overlap(fixture: NormalizationSemanticFixture) -> None:
    _text_anchors(fixture)[1]["canonicalStartByte"] = 2
    _rehash_locator(fixture)


def _mutate_anchor_empty(fixture: NormalizationSemanticFixture) -> None:
    _text_anchors(fixture)[1]["canonicalStartByte"] = 9
    _rehash_locator(fixture)


def _mutate_fragment_ordinal(fixture: NormalizationSemanticFixture) -> None:
    fragments = _text_anchors(fixture)[0]["sourceFragments"]
    assert isinstance(fragments, list)
    second = fragments[1]
    assert isinstance(second, dict)
    second["fragmentOrdinal"] = 2
    _rehash_locator(fixture)


def _mutate_view_rank(fixture: NormalizationSemanticFixture) -> None:
    fragments = _text_anchors(fixture)[0]["sourceFragments"]
    assert isinstance(fragments, list)
    fragment = fragments[0]
    assert isinstance(fragment, dict)
    views = fragment["views"]
    assert isinstance(views, list)
    views.reverse()
    _rehash_locator(fixture)


def _mutate_structural_anchor_order(fixture: NormalizationSemanticFixture) -> None:
    anchors = fixture.source_locator["structuralAnchors"]
    assert isinstance(anchors, list)
    anchors.reverse()
    for ordinal, anchor in enumerate(anchors):
        assert isinstance(anchor, dict)
        anchor["anchorOrdinal"] = ordinal
    _rehash_locator(fixture)


def _mutate_sheet_range(fixture: NormalizationSemanticFixture) -> None:
    fragments = _text_anchors(fixture)[0]["sourceFragments"]
    assert isinstance(fragments, list)
    fragment = fragments[0]
    assert isinstance(fragment, dict)
    views = fragment["views"]
    assert isinstance(views, list)
    sheet_view = views[3]
    assert isinstance(sheet_view, dict)
    sheet_view["startCell"] = "C3"
    sheet_view["endCell"] = "B2"
    _rehash_locator(fixture)


def _mutate_bbox(fixture: NormalizationSemanticFixture) -> None:
    transform = _map_segments(fixture)[5]["transform"]
    assert isinstance(transform, dict)
    positions = transform["sourcePositions"]
    assert isinstance(positions, list)
    first = positions[0]
    assert isinstance(first, dict)
    first["bboxMilliPoint"] = [0, 0, 0, 100]
    _rehash_map(fixture)


def _mutate_profile_ordinal(fixture: NormalizationSemanticFixture) -> None:
    recipes = fixture.normalization_profile["syntaxDecodeRecipes"]
    assert isinstance(recipes, list)
    recipe = recipes[0]
    assert isinstance(recipe, dict)
    recipe["recipeOrdinal"] = 1
    _rehash_profile_and_map(fixture)


_SEMANTIC_MUTATIONS: tuple[tuple[FixtureMutation, str], ...] = (
    (_mutate_segment_ordinal, "segment ordinals"),
    (_mutate_source_unit_rank_order, "profile rank and canonical key"),
    (_mutate_resolver_order, "entries must be unique and ordered"),
    (_mutate_resolver_payload_ordinal, "payload ordinals"),
    (_mutate_resolver_count, "entryCount"),
    (_mutate_payload_hash, "payloadHash"),
    (_mutate_source_position_bounds, "source-unit bounds"),
    (_mutate_text_position_order, "overlap or rewind"),
    (_mutate_geometry_position_order, "strictly source ordered"),
    (_mutate_text_position_overlap, "overlap or rewind"),
    (_mutate_position_kind, "one Position kind"),
    (_mutate_subsegment_gap, "gapless non-empty exact cover"),
    (_mutate_parent_coalesce, "canonical subsegment coalesce"),
    (_mutate_newline_recipe_ref, "absent from profile"),
    (_mutate_renderer_profile_marker, "does not match profile rule"),
    (_mutate_utf8_boundary, "UTF-8 scalar boundary"),
    (_mutate_anchor_overlap, "overlap"),
    (_mutate_anchor_empty, "non-empty"),
    (_mutate_fragment_ordinal, "fragment ordinals"),
    (_mutate_view_rank, "view ranks"),
    (_mutate_structural_anchor_order, "canonically ordered"),
    (_mutate_sheet_range, "sheet range start"),
    (_mutate_bbox, "positive half-open area"),
    (_mutate_profile_ordinal, "recipeOrdinal"),
)


def test_semantic_fixture_inventory_is_explicit_and_exact_jcs() -> None:
    positive_paths = tuple(sorted((_FIXTURE_ROOT / "positive").glob("*.json")))
    negative_paths = tuple(sorted((_FIXTURE_ROOT / "negative").glob("*.json")))
    assert {path.name for path in positive_paths} == _POSITIVE_FIXTURES
    assert {path.name for path in negative_paths} == set(_NEGATIVE_FIXTURES)
    for path in (*positive_paths, *negative_paths):
        content = path.read_bytes()
        assert canonical_json_bytes(load_strict_json_bytes(content)) == content


@pytest.mark.parametrize("fixture_name", sorted(_POSITIVE_FIXTURES))
def test_positive_semantic_fixtures_validate_after_offline_schema_gate(
    fixture_name: str,
) -> None:
    fixture = _fixture(_FIXTURE_ROOT / "positive" / fixture_name)
    validate_parser_normalization_semantics(
        fixture,
        packaged_schemas=load_packaged_schemas(),
    )


def test_positive_fixture_covers_every_transform_position_and_view_variant() -> None:
    fixture = _all_variants_fixture()
    segments = _map_segments(fixture)
    transform_kinds = {
        transform["kind"]
        for segment in segments
        if isinstance((transform := segment["transform"]), dict)
    }
    position_kinds = {
        position["kind"]
        for segment in segments
        if isinstance((transform := segment["transform"]), dict)
        for position in transform.get("sourcePositions", [])
        if isinstance(position, dict)
    }
    fragments = _text_anchors(fixture)[0]["sourceFragments"]
    assert isinstance(fragments, list)
    first_fragment = fragments[0]
    assert isinstance(first_fragment, dict)
    views = first_fragment["views"]
    assert isinstance(views, list)
    view_kinds = {view["kind"] for view in views if isinstance(view, dict)}
    assert transform_kinds == {
        "identity",
        "newline_fold",
        "nfc_compose",
        "renderer_insert",
        "syntax_decode",
    }
    assert position_kinds == {"page_geometry_position", "text_position"}
    assert view_kinds == {
        "derived_structure",
        "ooxml_path",
        "page_region",
        "sheet_range",
        "slide_shape",
        "source_text_position",
    }


@pytest.mark.parametrize(
    ("fixture_name", "message"),
    sorted(_NEGATIVE_FIXTURES.items()),
)
def test_checked_in_negative_semantic_cases_fail_closed(
    fixture_name: str,
    message: str,
) -> None:
    fixture = _fixture(_FIXTURE_ROOT / "negative" / fixture_name)
    with pytest.raises(NormalizationSemanticError, match=message):
        validate_parser_normalization_semantics(fixture)


@pytest.mark.parametrize(
    ("mutation", "message"),
    _SEMANTIC_MUTATIONS,
    ids=[
        mutation.__name__.removeprefix("_mutate_")
        for mutation, _ in _SEMANTIC_MUTATIONS
    ],
)
def test_schema_valid_cross_record_mutations_fail_closed(
    mutation: FixtureMutation,
    message: str,
) -> None:
    fixture = _all_variants_fixture()
    mutation(fixture)
    with pytest.raises(NormalizationSemanticError, match=message):
        validate_parser_normalization_semantics(fixture)


@pytest.mark.parametrize(
    ("artifact", "domain", "message"),
    [
        ("normalization_map", NORMALIZATION_MAP_AGGREGATE_DOMAIN, "normalizationMap"),
        (
            "source_unit_resolver",
            SOURCE_UNIT_RESOLVER_AGGREGATE_DOMAIN,
            "sourceUnitResolver",
        ),
        ("source_locator", SOURCE_LOCATOR_AGGREGATE_DOMAIN, "sourceLocator"),
    ],
)
def test_each_aggregate_hash_is_recomputed(
    artifact: str,
    domain: str,
    message: str,
) -> None:
    fixture = _all_variants_fixture()
    value = getattr(fixture, artifact)
    assert value["aggregateHash"] == aggregate_hash(value, domain)
    value["aggregateHash"] = "0" * 64
    with pytest.raises(NormalizationSemanticError, match=message):
        validate_parser_normalization_semantics(fixture)


def test_schema_validation_precedes_semantic_validation() -> None:
    fixture = _all_variants_fixture()
    fixture.normalization_map["unexpected"] = True
    fixture.normalization_map["segments"] = []
    with pytest.raises(ValidationError):
        validate_parser_normalization_semantics(fixture)


def test_non_contract_numbers_fail_before_schema_and_semantics() -> None:
    fixture = _all_variants_fixture()
    fixture.normalization_map["textBufferBytes"] = 0.5  # type: ignore[assignment]
    with pytest.raises(ContractProfileError, match="float"):
        validate_parser_normalization_semantics(fixture)


def test_invalid_fixture_text_bytes_fail_strict_utf8_without_source_claims() -> None:
    fixture = replace(_all_variants_fixture(), text_buffer=b"\xc3")
    with pytest.raises(NormalizationSemanticError, match="strict UTF-8"):
        validate_parser_normalization_semantics(fixture)
