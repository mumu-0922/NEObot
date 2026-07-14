from __future__ import annotations

import copy
from pathlib import Path

import pytest

from tests.support.parser_contracts import (
    JsonObject,
    JsonValue,
    load_packaged_schemas,
    load_strict_json_bytes,
)
from tests.support.parser_projection_semantics import (
    ProjectionSemanticError,
    ProjectionSemanticFixture,
    locator_aggregate_hash,
    parse_projection_semantic_fixture,
    recompute_locator_hash,
    validate_parser_projection_semantics,
)

_FIXTURE_ROOT = (
    Path(__file__).parents[1]
    / "fixtures"
    / "parser_contracts"
    / "semantic_instances"
    / "projection"
)
_POSITIVE_FIXTURE = "exact-cover-projection-and-child-clipping.v1.json"
_NEGATIVE_FIXTURES = {
    "child-view-expansion.v1.json": "expand or reorder the parent projection",
    "gap-document-cover.v1.json": "leave a gap",
    "overlap-document-cover.v1.json": "ranges overlap or are reordered",
    "projection-page-view.v1.json": "unique map projection",
    "projection-source-view.v1.json": "unique map projection",
    "reorder-source-fragments.v1.json": "cycle-like source rewind",
    "split-atomic-syntax.v1.json": "not legal inside atomic syntax_decode",
    "stale-locator-hash.v1.json": "sourceLocator.aggregateHash",
    "structural-anchor-clipping.v1.json": "structural anchors must not cover",
}


@pytest.fixture(scope="module")
def projection_fixture() -> ProjectionSemanticFixture:
    path = _FIXTURE_ROOT / "positive" / _POSITIVE_FIXTURE
    return parse_projection_semantic_fixture(path.read_bytes())


def _all_locator_sets(fixture: ProjectionSemanticFixture) -> list[JsonObject]:
    documents = [locator.locator_set for locator in fixture.document_locators]
    children = [locator.locator_set for locator in fixture.child_locators]
    return documents + children


def _objects(value: JsonValue) -> list[JsonObject]:
    assert isinstance(value, list)
    result: list[JsonObject] = []
    for item in value:
        assert isinstance(item, dict)
        result.append(item)
    return result


def _integer(value: JsonValue) -> int:
    assert type(value) is int
    return value


def _anchors(locator_set: JsonObject) -> list[JsonObject]:
    return _objects(locator_set["textAnchors"])


def _fragments(anchor: JsonObject) -> list[JsonObject]:
    return _objects(anchor["sourceFragments"])


def _views(fragment: JsonObject) -> list[JsonObject]:
    return _objects(fragment["views"])


def test_projection_fixtures_are_exact_jcs() -> None:
    paths = sorted(_FIXTURE_ROOT.rglob("*.json"))
    assert {path.name for path in paths} == {
        _POSITIVE_FIXTURE,
        *_NEGATIVE_FIXTURES,
    }
    for path in paths:
        load_strict_json_bytes(path.read_bytes())


def test_non_separator_text_has_ordered_exact_locator_cover(
    projection_fixture: ProjectionSemanticFixture,
) -> None:
    validate_parser_projection_semantics(
        projection_fixture,
        packaged_schemas=load_packaged_schemas(),
    )

    document_ranges = [
        (
            _integer(anchor["canonicalStartByte"]),
            _integer(anchor["canonicalEndByte"]),
        )
        for locator in projection_fixture.document_locators
        for anchor in _anchors(locator.locator_set)
    ]
    assert document_ranges == [(0, 4), (6, 8)]


def test_text_and_page_views_are_unique_map_projections(
    projection_fixture: ProjectionSemanticFixture,
) -> None:
    text_anchors = _anchors(projection_fixture.document_locators[0].locator_set)
    page_anchors = _anchors(projection_fixture.document_locators[1].locator_set)
    text_fragments = _fragments(text_anchors[0])
    assert _views(text_fragments[0])[0]["kind"] == "source_text_position"
    assert [
        _views(fragment)[0]["kind"] for fragment in _fragments(page_anchors[0])
    ] == ["page_region", "page_region"]


def test_child_clips_are_ordered_parent_subsets(
    projection_fixture: ProjectionSemanticFixture,
) -> None:
    child_ranges = [
        (
            child.parent_ordinal,
            _integer(_anchors(child.locator_set)[0]["canonicalStartByte"]),
            _integer(_anchors(child.locator_set)[0]["canonicalEndByte"]),
        )
        for child in projection_fixture.child_locators
    ]
    assert child_ranges == [(0, 1, 2), (0, 3, 4), (1, 7, 8)]
    validate_parser_projection_semantics(projection_fixture)


def test_all_positive_locator_hashes_are_recomputed(
    projection_fixture: ProjectionSemanticFixture,
) -> None:
    for locator_set in _all_locator_sets(projection_fixture):
        assert locator_set["aggregateHash"] == locator_aggregate_hash(locator_set)

    mutated = copy.deepcopy(projection_fixture.document_locators[0].locator_set)
    _anchors(mutated)[0]["canonicalEndByte"] = 3
    assert mutated["aggregateHash"] != locator_aggregate_hash(mutated)
    recompute_locator_hash(mutated)
    assert mutated["aggregateHash"] == locator_aggregate_hash(mutated)


@pytest.mark.parametrize(
    ("filename", "expected_message"),
    sorted(_NEGATIVE_FIXTURES.items()),
)
def test_projection_semantics_fail_closed(
    filename: str,
    expected_message: str,
) -> None:
    path = _FIXTURE_ROOT / "negative" / filename
    fixture = parse_projection_semantic_fixture(path.read_bytes())
    with pytest.raises(ProjectionSemanticError, match=expected_message):
        validate_parser_projection_semantics(fixture)


@pytest.mark.parametrize(
    "filename",
    sorted(set(_NEGATIVE_FIXTURES) - {"stale-locator-hash.v1.json"}),
)
def test_semantic_negative_locators_have_fresh_hashes(filename: str) -> None:
    path = _FIXTURE_ROOT / "negative" / filename
    fixture = parse_projection_semantic_fixture(path.read_bytes())
    for locator_set in _all_locator_sets(fixture):
        assert locator_set["aggregateHash"] == locator_aggregate_hash(locator_set)
