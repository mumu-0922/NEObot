"""Fail-closed semantic validation for normalization and locator fixtures.

The packaged JSON Schemas deliberately validate shape only.  This test helper
adds the cross-record invariants that JSON Schema cannot express.  It validates
only the canonical text bytes supplied by a fixture; source-unit bytes are not
available here, so the helper checks source offsets against declared bounds but
does not claim source-byte round trips.
"""

from __future__ import annotations

import hashlib
import re
import unicodedata
from dataclasses import dataclass
from typing import Final, Never, cast

from tests.support.parser_contracts import (
    JsonObject,
    JsonValue,
    PackagedSchemas,
    canonical_json_bytes,
    domain_separated_sha256,
    load_packaged_schemas,
    load_strict_json_bytes,
    validate_schema_instance,
)

NORMALIZATION_MAP_AGGREGATE_DOMAIN: Final = "mm-chat.normalization-map.v1\n"
SOURCE_UNIT_RESOLVER_AGGREGATE_DOMAIN: Final = "mm-chat.source-unit-resolver.v1\n"
SOURCE_LOCATOR_AGGREGATE_DOMAIN: Final = "mm-chat.source-locator.v2\n"

_FIXTURE_FIELDS: Final = frozenset(
    {
        "knownReferences",
        "normalizationMap",
        "normalizationProfile",
        "sourceLocator",
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
_VIEW_RANK: Final = {
    "source_text_position": 0,
    "page_region": 1,
    "slide_shape": 2,
    "sheet_range": 3,
    "ooxml_path": 4,
    "derived_structure": 5,
}
_STRUCTURE_KIND_RANK: Final = {
    kind: rank
    for rank, kind in enumerate(
        (
            "document",
            "page",
            "heading",
            "paragraph",
            "list",
            "list_item",
            "quote",
            "code",
            "table",
            "table_row",
            "table_cell",
            "formula",
            "caption",
            "footnote",
            "header",
            "footer",
            "page_break",
            "asset_ref",
            "slide",
            "sheet",
            "shape",
        )
    )
}
_A1_CELL: Final = re.compile(r"^([A-Z]+)([1-9][0-9]*)$")


class NormalizationSemanticError(ValueError):
    """A schema-valid normalization fixture violates semantic invariants."""


@dataclass(frozen=True, slots=True)
class LocatorReferenceRegistry:
    """References owned by fixture context rather than the three artifacts."""

    canonical_xpath_payload_refs: frozenset[str]
    opaque_shape_ids: frozenset[str]
    opaque_sheet_ids: frozenset[str]
    opaque_structure_ids: frozenset[str]
    owner_seed_ids: frozenset[str]


@dataclass(frozen=True, slots=True)
class NormalizationSemanticFixture:
    """One closed fixture bundle with exact canonical text bytes."""

    normalization_map: JsonObject
    normalization_profile: JsonObject
    source_locator: JsonObject
    source_unit_resolver: JsonObject
    text_buffer: bytes
    references: LocatorReferenceRegistry


@dataclass(frozen=True, slots=True)
class _ProfileIndex:
    kind_ranks: dict[str, int]
    synthetic_role_ranks: dict[str, int]
    newline_recipes: dict[str, JsonObject]
    syntax_recipes: dict[str, JsonObject]
    renderer_rules: dict[str, JsonObject]
    transform_kinds: frozenset[str]


@dataclass(frozen=True, slots=True)
class _SourceUnitIndex:
    by_id: dict[str, JsonObject]
    ordinal_by_id: dict[str, int]


def parse_normalization_semantic_fixture(
    content: bytes,
) -> NormalizationSemanticFixture:
    """Parse one exact-JCS fixture bundle without running semantic checks."""
    value = load_strict_json_bytes(content)
    document = _object(value, "$")
    _require_exact_fields(document, _FIXTURE_FIELDS, "$")
    text = _string(document["textBuffer"], "$.textBuffer")
    reference_value = _object(document["knownReferences"], "$.knownReferences")
    _require_exact_fields(reference_value, _REFERENCE_FIELDS, "$.knownReferences")
    references = LocatorReferenceRegistry(
        canonical_xpath_payload_refs=_reference_set(
            reference_value["canonicalXPathPayloadRefs"],
            "$.knownReferences.canonicalXPathPayloadRefs",
        ),
        opaque_shape_ids=_reference_set(
            reference_value["opaqueShapeIds"],
            "$.knownReferences.opaqueShapeIds",
        ),
        opaque_sheet_ids=_reference_set(
            reference_value["opaqueSheetIds"],
            "$.knownReferences.opaqueSheetIds",
        ),
        opaque_structure_ids=_reference_set(
            reference_value["opaqueStructureIds"],
            "$.knownReferences.opaqueStructureIds",
        ),
        owner_seed_ids=_reference_set(
            reference_value["ownerSeedIds"],
            "$.knownReferences.ownerSeedIds",
        ),
    )
    return NormalizationSemanticFixture(
        normalization_map=_object(document["normalizationMap"], "$.normalizationMap"),
        normalization_profile=_object(
            document["normalizationProfile"], "$.normalizationProfile"
        ),
        source_locator=_object(document["sourceLocator"], "$.sourceLocator"),
        source_unit_resolver=_object(
            document["sourceUnitResolver"], "$.sourceUnitResolver"
        ),
        text_buffer=text.encode("utf-8"),
        references=references,
    )


def validate_parser_normalization_semantics(
    fixture: NormalizationSemanticFixture,
    *,
    packaged_schemas: PackagedSchemas | None = None,
) -> None:
    """Validate schemas first, then every cross-record semantic invariant."""
    packaged = packaged_schemas or load_packaged_schemas()
    schema_instances = (
        (
            "normalization-profile.v1.schema.json",
            fixture.normalization_profile,
        ),
        ("normalization-map.v1.schema.json", fixture.normalization_map),
        (
            "source-unit-resolver.v1.schema.json",
            fixture.source_unit_resolver,
        ),
        ("source-locator.v2.schema.json", fixture.source_locator),
    )
    for schema_name, instance in schema_instances:
        validate_schema_instance(packaged, schema_name, instance)

    boundaries = _utf8_boundaries(fixture.text_buffer)
    profile = _validate_profile_semantics(fixture.normalization_profile)
    _validate_text_buffer(fixture.normalization_map, fixture.text_buffer)
    resolver_entries = _validate_resolver_local_semantics(fixture.source_unit_resolver)
    source_units = _validate_source_units(
        fixture.normalization_map,
        fixture.source_unit_resolver,
        resolver_entries,
        profile,
    )
    _validate_segments(
        fixture.normalization_map,
        fixture.text_buffer,
        boundaries,
        source_units,
        profile,
        fixture.references,
    )
    _validate_source_locator(
        fixture.source_locator,
        fixture.text_buffer,
        boundaries,
        source_units,
        fixture.references,
    )
    _validate_hashes(fixture)


def aggregate_hash(value: JsonObject, domain: str) -> str:
    """Recompute a domain-separated aggregate excluding ``aggregateHash``."""
    payload = value.copy()
    payload.pop("aggregateHash", None)
    return domain_separated_sha256(domain, payload)


def payload_hash(payload: JsonObject) -> str:
    """Hash one resolver display payload as exact RFC 8785 bytes."""
    return hashlib.sha256(canonical_json_bytes(payload)).hexdigest()


def normalization_profile_hash(profile: JsonObject) -> str:
    """Hash the complete normalization profile as exact RFC 8785 bytes."""
    return hashlib.sha256(canonical_json_bytes(profile)).hexdigest()


def _validate_profile_semantics(profile: JsonObject) -> _ProfileIndex:
    source_kinds = _object_list(profile["sourceUnitKinds"], "profile.sourceUnitKinds")
    kind_ranks: dict[str, int] = {}
    synthetic_role_ranks: dict[str, int] = {}
    for expected_rank, record in enumerate(source_kinds):
        path = f"profile.sourceUnitKinds[{expected_rank}]"
        rank = _integer(record["rank"], f"{path}.rank")
        kind = _string(record["kind"], f"{path}.kind")
        if rank != expected_rank:
            _fail(path, "source-unit ranks must be contiguous and array ordered")
        if kind in kind_ranks:
            _fail(path, f"duplicate source-unit kind {kind!r}")
        kind_ranks[kind] = rank
        role_values = record.get("roleRanks")
        if role_values is not None:
            roles = _object_list(role_values, f"{path}.roleRanks")
            for expected_role_rank, role_record in enumerate(roles):
                role_path = f"{path}.roleRanks[{expected_role_rank}]"
                role_rank = _integer(role_record["rank"], f"{role_path}.rank")
                role = _string(role_record["role"], f"{role_path}.role")
                if role_rank != expected_role_rank:
                    _fail(role_path, "role ranks must be contiguous and array ordered")
                if role in synthetic_role_ranks:
                    _fail(role_path, f"duplicate synthetic role {role!r}")
                synthetic_role_ranks[role] = role_rank

    newline_recipes = _unique_records(
        profile["newlineRecipes"],
        key_field="newlineRecipeId",
        label="profile.newlineRecipes",
    )
    syntax_recipes = _unique_records(
        profile["syntaxDecodeRecipes"],
        key_field="recipeId",
        label="profile.syntaxDecodeRecipes",
        ordinal_field="recipeOrdinal",
    )
    renderer_rules = _unique_records(
        profile["rendererRules"],
        key_field="rendererRuleId",
        label="profile.rendererRules",
        ordinal_field="rendererRuleOrdinal",
    )
    transform_values = _list(profile["transformKinds"], "profile.transformKinds")
    transform_kinds = tuple(
        _string(value, f"profile.transformKinds[{index}]")
        for index, value in enumerate(transform_values)
    )
    if len(transform_kinds) != len(set(transform_kinds)):
        _fail("profile.transformKinds", "transform kinds must be unique")
    return _ProfileIndex(
        kind_ranks=kind_ranks,
        synthetic_role_ranks=synthetic_role_ranks,
        newline_recipes=newline_recipes,
        syntax_recipes=syntax_recipes,
        renderer_rules=renderer_rules,
        transform_kinds=frozenset(transform_kinds),
    )


def _validate_text_buffer(normalization_map: JsonObject, text_buffer: bytes) -> None:
    expected_bytes = _integer(
        normalization_map["textBufferBytes"], "normalizationMap.textBufferBytes"
    )
    if expected_bytes != len(text_buffer):
        _fail("normalizationMap.textBufferBytes", "does not match fixture bytes")
    expected_hash = _string(
        normalization_map["textBufferSha256"],
        "normalizationMap.textBufferSha256",
    )
    if expected_hash != hashlib.sha256(text_buffer).hexdigest():
        _fail("normalizationMap.textBufferSha256", "does not match fixture bytes")
    text = text_buffer.decode("utf-8", errors="strict")
    if unicodedata.normalize("NFC", text) != text:
        _fail("textBuffer", "canonical text is not NFC")
    if "\r" in text:
        _fail("textBuffer", "canonical text contains a non-normalized CR")
    if "\x00" in text or "\ufffd" in text:
        _fail("textBuffer", "canonical text contains a forbidden scalar")


def _validate_resolver_local_semantics(
    resolver: JsonObject,
) -> tuple[JsonObject, ...]:
    entries = tuple(_object_list(resolver["entries"], "sourceUnitResolver.entries"))
    entry_count = _integer(resolver["entryCount"], "sourceUnitResolver.entryCount")
    if entry_count != len(entries):
        _fail("sourceUnitResolver.entryCount", "does not equal entries length")

    observed_keys: list[tuple[int, int, int]] = []
    ordinals_by_group: dict[tuple[int, int], list[int]] = {}
    payload_refs: set[str] = set()
    for index, entry in enumerate(entries):
        path = f"sourceUnitResolver.entries[{index}]"
        source_ordinal = _integer(
            entry["sourceUnitOrdinal"], f"{path}.sourceUnitOrdinal"
        )
        kind_rank = _integer(entry["payloadKindRank"], f"{path}.payloadKindRank")
        payload_ordinal = _integer(entry["payloadOrdinal"], f"{path}.payloadOrdinal")
        observed_keys.append((source_ordinal, kind_rank, payload_ordinal))
        ordinals_by_group.setdefault((source_ordinal, kind_rank), []).append(
            payload_ordinal
        )
        payload_ref = _string(
            entry["sourceUnitPayloadRef"], f"{path}.sourceUnitPayloadRef"
        )
        if payload_ref in payload_refs:
            _fail(path, "sourceUnitPayloadRef must be unique")
        payload_refs.add(payload_ref)
        payload = _object(entry["payload"], f"{path}.payload")
        if _string(entry["payloadHash"], f"{path}.payloadHash") != payload_hash(
            payload
        ):
            _fail(f"{path}.payloadHash", "does not match canonical payload")

    if observed_keys != sorted(observed_keys) or len(observed_keys) != len(
        set(observed_keys)
    ):
        _fail(
            "sourceUnitResolver.entries",
            "entries must be unique and ordered by sourceUnitOrdinal, "
            "payloadKindRank, payloadOrdinal",
        )
    for group, ordinals in ordinals_by_group.items():
        if ordinals != list(range(len(ordinals))):
            _fail(
                "sourceUnitResolver.entries",
                f"payload ordinals for group {group} are not contiguous from zero",
            )
    return entries


def _validate_source_units(  # noqa: PLR0915
    normalization_map: JsonObject,
    resolver: JsonObject,
    resolver_entries: tuple[JsonObject, ...],
    profile: _ProfileIndex,
) -> _SourceUnitIndex:
    units = _object_list(
        normalization_map["sourceUnits"], "normalizationMap.sourceUnits"
    )
    by_id: dict[str, JsonObject] = {}
    ordinal_by_id: dict[str, int] = {}
    display_refs: set[str] = set()
    canonical_order: list[tuple[int, bytes]] = []

    entries_by_ordinal: dict[int, list[JsonObject]] = {}
    for entry in resolver_entries:
        ordinal = _integer(
            entry["sourceUnitOrdinal"], "sourceUnitResolver.entry.sourceUnitOrdinal"
        )
        entries_by_ordinal.setdefault(ordinal, []).append(entry)

    for expected_ordinal, unit in enumerate(units):
        path = f"normalizationMap.sourceUnits[{expected_ordinal}]"
        ordinal = _integer(unit["sourceUnitOrdinal"], f"{path}.sourceUnitOrdinal")
        if ordinal != expected_ordinal:
            _fail(path, "sourceUnitOrdinal values must be contiguous from zero")
        opaque_id = _string(unit["opaqueSourceUnitId"], f"{path}.opaqueSourceUnitId")
        if opaque_id in by_id:
            _fail(path, "opaqueSourceUnitId must be unique")
        by_id[opaque_id] = unit
        ordinal_by_id[opaque_id] = ordinal

        display_ref = _string(
            unit["displayMetadataPayloadRef"], f"{path}.displayMetadataPayloadRef"
        )
        if display_ref in display_refs:
            _fail(path, "displayMetadataPayloadRef must be unique")
        display_refs.add(display_ref)
        matching_entries = [
            entry
            for entry in entries_by_ordinal.get(ordinal, [])
            if entry["sourceUnitPayloadRef"] == display_ref
        ]
        if len(matching_entries) != 1:
            _fail(path, "display metadata ref must resolve to exactly one entry")
        entry = matching_entries[0]
        kind = _string(unit["kind"], f"{path}.kind")
        if entry["opaqueSourceUnitId"] != opaque_id or entry["sourceUnitKind"] != kind:
            _fail(path, "resolver identity or source-unit kind does not match")
        try:
            kind_rank = profile.kind_ranks[kind]
        except KeyError as error:
            raise NormalizationSemanticError(
                f"{path}.kind: kind is absent from normalization profile"
            ) from error
        payload = _object(entry["payload"], "sourceUnitResolver.entry.payload")
        canonical_key = _canonical_source_unit_key(
            kind,
            payload,
            profile,
            path,
        )
        canonical_order.append((kind_rank, canonical_key))

    if not units:
        _fail(
            "normalizationMap.sourceUnits", "at least the raw source unit is required"
        )
    if canonical_order != sorted(canonical_order) or len(canonical_order) != len(
        set(canonical_order)
    ):
        _fail(
            "normalizationMap.sourceUnits",
            "source units are not uniquely ordered by profile rank and canonical key",
        )

    for index, entry in enumerate(resolver_entries):
        path = f"sourceUnitResolver.entries[{index}]"
        ordinal = _integer(entry["sourceUnitOrdinal"], f"{path}.sourceUnitOrdinal")
        if ordinal < 0 or ordinal >= len(units):
            _fail(path, "sourceUnitOrdinal is outside normalization-map bounds")
        unit = units[ordinal]
        if (
            entry["opaqueSourceUnitId"] != unit["opaqueSourceUnitId"]
            or entry["sourceUnitKind"] != unit["kind"]
        ):
            _fail(path, "entry does not resolve to the indexed source unit")

    raw_units = [unit for unit in units if unit["kind"] == "raw_file"]
    if len(raw_units) != 1:
        _fail("normalizationMap.sourceUnits", "exactly one raw_file unit is required")
    if resolver["sourceSha256"] != raw_units[0]["sourceSha256"]:
        _fail(
            "sourceUnitResolver.sourceSha256",
            "does not match the raw_file source unit",
        )
    return _SourceUnitIndex(by_id=by_id, ordinal_by_id=ordinal_by_id)


def _canonical_source_unit_key(
    kind: str,
    payload: JsonObject,
    profile: _ProfileIndex,
    path: str,
) -> bytes:
    if kind == "raw_file":
        return b""
    if kind == "ooxml_part":
        return _string(payload["canonicalPartUri"], f"{path}.canonicalPartUri").encode(
            "utf-8"
        )
    if kind == "synthetic_mineru_artifact":
        role = _string(payload["role"], f"{path}.role")
        try:
            role_rank = profile.synthetic_role_ranks[role]
        except KeyError as error:
            raise NormalizationSemanticError(
                f"{path}.role: role is absent from normalization profile"
            ) from error
        return role_rank.to_bytes(8, byteorder="big", signed=False)
    return _fail(path, f"unsupported source-unit kind {kind!r}")


def _validate_segments(
    normalization_map: JsonObject,
    text_buffer: bytes,
    boundaries: frozenset[int],
    source_units: _SourceUnitIndex,
    profile: _ProfileIndex,
    references: LocatorReferenceRegistry,
) -> None:
    segments = _object_list(normalization_map["segments"], "normalizationMap.segments")
    if not text_buffer:
        if segments:
            _fail("normalizationMap.segments", "empty text buffer requires no segments")
        return
    if not segments:
        _fail("normalizationMap.segments", "non-empty text buffer requires segments")

    expected_start = 0
    normalization_cluster_ids: set[str] = set()
    for expected_ordinal, segment in enumerate(segments):
        path = f"normalizationMap.segments[{expected_ordinal}]"
        ordinal = _integer(segment["segmentOrdinal"], f"{path}.segmentOrdinal")
        start = _integer(segment["canonicalStartByte"], f"{path}.canonicalStartByte")
        end = _integer(segment["canonicalEndByte"], f"{path}.canonicalEndByte")
        if ordinal != expected_ordinal:
            _fail(path, "segment ordinals must be contiguous from zero")
        if start != expected_start:
            _fail(path, "segments are not a gapless non-overlapping exact cover")
        if start >= end:
            _fail(path, "segments must be non-empty")
        _require_utf8_boundary(start, boundaries, f"{path}.canonicalStartByte")
        _require_utf8_boundary(end, boundaries, f"{path}.canonicalEndByte")
        transform = _object(segment["transform"], f"{path}.transform")
        _validate_transform(
            transform,
            text_buffer[start:end],
            start,
            boundaries,
            source_units,
            profile,
            references,
            normalization_cluster_ids,
            f"{path}.transform",
        )
        expected_start = end
    if expected_start != len(text_buffer):
        _fail("normalizationMap.segments", "segments do not exactly cover textBuffer")


def _validate_transform(
    transform: JsonObject,
    canonical_bytes: bytes,
    canonical_start: int,
    boundaries: frozenset[int],
    source_units: _SourceUnitIndex,
    profile: _ProfileIndex,
    references: LocatorReferenceRegistry,
    normalization_cluster_ids: set[str],
    path: str,
) -> None:
    kind = _string(transform["kind"], f"{path}.kind")
    if kind not in profile.transform_kinds:
        _fail(f"{path}.kind", "transform kind is absent from normalization profile")
    if kind == "renderer_insert":
        _validate_renderer_insert(
            transform,
            canonical_bytes,
            profile,
            references,
            path,
        )
        return

    parent_positions = _object_list(
        transform["sourcePositions"], f"{path}.sourcePositions"
    )
    parent_kind = _validate_position_list(
        parent_positions,
        source_units,
        f"{path}.sourcePositions",
    )
    if kind == "newline_fold":
        recipe_id = _string(transform["newlineRecipeId"], f"{path}.newlineRecipeId")
        recipe = profile.newline_recipes.get(recipe_id)
        if recipe is None:
            _fail(f"{path}.newlineRecipeId", "recipe is absent from profile")
        if recipe["legalSplitPolicy"] != "lf_boundaries_only":
            _fail(path, "newline recipe has no legal LF split marker")
        canonical_sequence = _string(
            recipe["canonicalSequence"], "normalizationProfile.newlineRecipe"
        ).encode("utf-8")
        if canonical_bytes != canonical_sequence:
            _fail(path, "newline segment does not equal its profile marker")
        if parent_kind != "text_position":
            _fail(path, "newline_fold requires text positions")
    elif kind == "nfc_compose":
        cluster_id = _string(
            transform["normalizationClusterId"],
            f"{path}.normalizationClusterId",
        )
        if cluster_id in normalization_cluster_ids:
            _fail(path, "normalizationClusterId must be unique")
        normalization_cluster_ids.add(cluster_id)
        canonical_text = canonical_bytes.decode("utf-8", errors="strict")
        if unicodedata.normalize("NFC", canonical_text) != canonical_text:
            _fail(path, "nfc_compose output is not NFC")
    elif kind == "syntax_decode":
        _validate_syntax_decode(
            transform,
            canonical_bytes,
            canonical_start,
            boundaries,
            source_units,
            profile,
            parent_positions,
            parent_kind,
            path,
        )
    elif kind != "identity":
        _fail(path, f"unsupported transform variant {kind!r}")


def _validate_syntax_decode(
    transform: JsonObject,
    canonical_bytes: bytes,
    canonical_start: int,
    boundaries: frozenset[int],
    source_units: _SourceUnitIndex,
    profile: _ProfileIndex,
    parent_positions: list[JsonObject],
    parent_kind: str,
    path: str,
) -> None:
    recipe_id = _string(transform["recipeId"], f"{path}.recipeId")
    recipe = profile.syntax_recipes.get(recipe_id)
    if recipe is None:
        _fail(f"{path}.recipeId", "recipe is absent from profile")
    if transform["recipeProfileHash"] != recipe["recipeProfileHash"]:
        _fail(f"{path}.recipeProfileHash", "does not match profile recipe")
    if recipe["legalSplitPolicy"] != "validated_subsegments_or_atomic":
        _fail(path, "syntax recipe does not represent legal split markers")
    raw_subsegments = transform.get("subsegments")
    if raw_subsegments is None:
        return
    subsegments = _object_list(raw_subsegments, f"{path}.subsegments")
    expected_start = 0
    flattened_positions: list[JsonObject] = []
    for expected_ordinal, subsegment in enumerate(subsegments):
        subpath = f"{path}.subsegments[{expected_ordinal}]"
        ordinal = _integer(subsegment["ordinal"], f"{subpath}.ordinal")
        start = _integer(
            subsegment["relativeCanonicalStart"],
            f"{subpath}.relativeCanonicalStart",
        )
        end = _integer(
            subsegment["relativeCanonicalEnd"],
            f"{subpath}.relativeCanonicalEnd",
        )
        if ordinal != expected_ordinal:
            _fail(subpath, "subsegment ordinals must be contiguous from zero")
        if start != expected_start or start >= end:
            _fail(subpath, "subsegments must be a gapless non-empty exact cover")
        _require_utf8_boundary(
            canonical_start + start,
            boundaries,
            f"{subpath}.relativeCanonicalStart",
        )
        _require_utf8_boundary(
            canonical_start + end,
            boundaries,
            f"{subpath}.relativeCanonicalEnd",
        )
        positions = _object_list(
            subsegment["sourcePositions"], f"{subpath}.sourcePositions"
        )
        position_kind = _validate_position_list(
            positions,
            source_units,
            f"{subpath}.sourcePositions",
        )
        if position_kind != parent_kind:
            _fail(subpath, "subsegments and parent must use one Position kind")
        flattened_positions.extend(positions)
        expected_start = end
    if expected_start != len(canonical_bytes):
        _fail(f"{path}.subsegments", "subsegments do not exactly cover parent")

    canonical_parent = _coalesce_positions(parent_positions)
    canonical_children = _coalesce_positions(flattened_positions)
    if canonical_json_bytes(
        cast("JsonValue", canonical_parent)
    ) != canonical_json_bytes(cast("JsonValue", canonical_children)):
        _fail(path, "parent positions differ from canonical subsegment coalesce")


def _validate_renderer_insert(
    transform: JsonObject,
    canonical_bytes: bytes,
    profile: _ProfileIndex,
    references: LocatorReferenceRegistry,
    path: str,
) -> None:
    rule_id = _string(transform["rendererRuleId"], f"{path}.rendererRuleId")
    rule = profile.renderer_rules.get(rule_id)
    if rule is None:
        _fail(f"{path}.rendererRuleId", "rule is absent from profile")
    if transform["rendererProfileHash"] != rule["rendererProfileHash"]:
        _fail(f"{path}.rendererProfileHash", "does not match profile rule")
    structure_ref = _object(transform["structureRef"], f"{path}.structureRef")
    if structure_ref["structureKind"] != rule["structureKind"]:
        _fail(f"{path}.structureRef", "structure kind does not match profile rule")
    owner_seed_id = _string(
        structure_ref["ownerSeedId"], f"{path}.structureRef.ownerSeedId"
    )
    _require_reference(
        owner_seed_id,
        references.owner_seed_ids,
        f"{path}.structureRef.ownerSeedId",
    )
    if rule["legalSplitPolicy"] != "declared_renderer_boundaries_only":
        _fail(path, "renderer rule does not represent legal split markers")
    inserted = _string(rule["insertedUtf8"], "normalizationProfile.rendererRule")
    if canonical_bytes != inserted.encode("utf-8"):
        _fail(path, "renderer segment does not equal its declared profile marker")


def _validate_position_list(
    positions: list[JsonObject],
    source_units: _SourceUnitIndex,
    path: str,
) -> str:
    kinds = {
        _string(position["kind"], f"{path}[{index}].kind")
        for index, position in enumerate(positions)
    }
    if len(kinds) != 1:
        _fail(path, "one Position kind is required per segment")
    kind = next(iter(kinds))
    observed_keys: list[tuple[object, ...]] = []
    previous_text: JsonObject | None = None
    for expected_ordinal, position in enumerate(positions):
        position_path = f"{path}[{expected_ordinal}]"
        ordinal = _integer(
            position["positionOrdinal"], f"{position_path}.positionOrdinal"
        )
        if ordinal != expected_ordinal:
            _fail(position_path, "position ordinals must be contiguous from zero")
        unit_id = _string(
            position["opaqueSourceUnitId"],
            f"{position_path}.opaqueSourceUnitId",
        )
        unit = _source_unit(
            source_units, unit_id, f"{position_path}.opaqueSourceUnitId"
        )
        unit_ordinal = source_units.ordinal_by_id[unit_id]
        key: tuple[object, ...]
        if kind == "text_position":
            _validate_text_position(position, unit, position_path)
            key = (
                unit_ordinal,
                _integer(position["rawByteStart"], f"{position_path}.rawByteStart"),
                _integer(position["rawByteEnd"], f"{position_path}.rawByteEnd"),
                _integer(
                    position["decodedScalarStart"],
                    f"{position_path}.decodedScalarStart",
                ),
                _integer(
                    position["decodedScalarEnd"],
                    f"{position_path}.decodedScalarEnd",
                ),
            )
            if previous_text is not None and _text_positions_mergeable(
                previous_text, position
            ):
                _fail(path, "adjacent text positions are not canonically coalesced")
            if previous_text is not None and _text_positions_overlap(
                previous_text, position
            ):
                _fail(path, "text positions overlap or rewind within a source unit")
            previous_text = position
        elif kind == "page_geometry_position":
            _validate_bbox(
                position["bboxMilliPoint"], f"{position_path}.bboxMilliPoint"
            )
            key = (
                unit_ordinal,
                _integer(position["pageIndex"], f"{position_path}.pageIndex"),
                _integer(
                    position["fragmentReadingOrdinal"],
                    f"{position_path}.fragmentReadingOrdinal",
                ),
                *_integer_tuple(
                    position["bboxMilliPoint"], f"{position_path}.bboxMilliPoint"
                ),
            )
        else:
            _fail(position_path, f"unsupported Position kind {kind!r}")
        observed_keys.append(key)
    if observed_keys != sorted(observed_keys) or len(observed_keys) != len(
        set(observed_keys)
    ):
        _fail(path, "positions are not unique and strictly source ordered")
    return kind


def _validate_text_position(
    position: JsonObject,
    source_unit: JsonObject,
    path: str,
) -> None:
    raw_start = _integer(position["rawByteStart"], f"{path}.rawByteStart")
    raw_end = _integer(position["rawByteEnd"], f"{path}.rawByteEnd")
    source_bytes = _integer(source_unit["sourceBytes"], "sourceUnit.sourceBytes")
    if raw_start >= raw_end or raw_end > source_bytes:
        _fail(path, "raw byte range is empty, reversed, or outside source-unit bounds")
    scalar_start = _integer(
        position["decodedScalarStart"], f"{path}.decodedScalarStart"
    )
    scalar_end = _integer(position["decodedScalarEnd"], f"{path}.decodedScalarEnd")
    if scalar_start >= scalar_end:
        _fail(path, "decoded scalar range must be non-empty and ordered")
    line_start = (
        _integer(position["startLine"], f"{path}.startLine"),
        _integer(position["startColumn"], f"{path}.startColumn"),
    )
    line_end = (
        _integer(position["endLine"], f"{path}.endLine"),
        _integer(position["endColumn"], f"{path}.endColumn"),
    )
    if line_start >= line_end:
        _fail(path, "line/column range must be non-empty and ordered")


def _coalesce_positions(positions: list[JsonObject]) -> list[JsonObject]:
    result: list[JsonObject] = []
    for position in positions:
        current = position.copy()
        current["positionOrdinal"] = len(result)
        if result and _text_positions_mergeable(result[-1], current):
            previous = result[-1]
            for field in (
                "rawByteEnd",
                "decodedScalarEnd",
                "endLine",
                "endColumn",
            ):
                previous[field] = current[field]
            continue
        result.append(current)
    for ordinal, position in enumerate(result):
        position["positionOrdinal"] = ordinal
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


def _text_positions_overlap(left: JsonObject, right: JsonObject) -> bool:
    if left.get("opaqueSourceUnitId") != right.get("opaqueSourceUnitId"):
        return False
    left_line_end = (
        _integer(left["endLine"], "position.endLine"),
        _integer(left["endColumn"], "position.endColumn"),
    )
    right_line_start = (
        _integer(right["startLine"], "position.startLine"),
        _integer(right["startColumn"], "position.startColumn"),
    )
    return (
        _integer(left["rawByteEnd"], "position.rawByteEnd")
        > _integer(right["rawByteStart"], "position.rawByteStart")
        or _integer(left["decodedScalarEnd"], "position.decodedScalarEnd")
        > _integer(right["decodedScalarStart"], "position.decodedScalarStart")
        or left_line_end > right_line_start
    )


def _validate_source_locator(
    locator: JsonObject,
    text_buffer: bytes,
    boundaries: frozenset[int],
    source_units: _SourceUnitIndex,
    references: LocatorReferenceRegistry,
) -> None:
    text_anchors = _object_list(locator["textAnchors"], "sourceLocator.textAnchors")
    previous_end = -1
    for expected_ordinal, anchor in enumerate(text_anchors):
        path = f"sourceLocator.textAnchors[{expected_ordinal}]"
        ordinal = _integer(anchor["anchorOrdinal"], f"{path}.anchorOrdinal")
        start = _integer(anchor["canonicalStartByte"], f"{path}.canonicalStartByte")
        end = _integer(anchor["canonicalEndByte"], f"{path}.canonicalEndByte")
        if ordinal != expected_ordinal:
            _fail(path, "text anchor ordinals must be contiguous from zero")
        if start >= end or end > len(text_buffer):
            _fail(path, "text anchor must be non-empty and inside textBuffer")
        if previous_end > start:
            _fail(path, "text anchor ranges overlap or are out of order")
        _require_utf8_boundary(start, boundaries, f"{path}.canonicalStartByte")
        _require_utf8_boundary(end, boundaries, f"{path}.canonicalEndByte")
        _validate_fragments(
            anchor["sourceFragments"],
            source_units,
            references,
            f"{path}.sourceFragments",
        )
        previous_end = end

    structural_anchors = _object_list(
        locator["structuralAnchors"], "sourceLocator.structuralAnchors"
    )
    observed_keys: list[tuple[int, int, str]] = []
    for expected_ordinal, anchor in enumerate(structural_anchors):
        path = f"sourceLocator.structuralAnchors[{expected_ordinal}]"
        ordinal = _integer(anchor["anchorOrdinal"], f"{path}.anchorOrdinal")
        if ordinal != expected_ordinal:
            _fail(path, "structural anchor ordinals must be contiguous from zero")
        node_kind = _string(anchor["nodeKind"], f"{path}.nodeKind")
        owner_seed_id = _string(anchor["ownerSeedId"], f"{path}.ownerSeedId")
        _require_reference(
            owner_seed_id, references.owner_seed_ids, f"{path}.ownerSeedId"
        )
        observed_keys.append(
            (
                _integer(anchor["structureOrdinal"], f"{path}.structureOrdinal"),
                _STRUCTURE_KIND_RANK[node_kind],
                owner_seed_id,
            )
        )
        _validate_fragments(
            anchor["sourceFragments"],
            source_units,
            references,
            f"{path}.sourceFragments",
        )
    if observed_keys != sorted(observed_keys) or len(observed_keys) != len(
        set(observed_keys)
    ):
        _fail(
            "sourceLocator.structuralAnchors",
            "structural anchors are not unique and canonically ordered",
        )


def _validate_fragments(
    raw_fragments: JsonValue,
    source_units: _SourceUnitIndex,
    references: LocatorReferenceRegistry,
    path: str,
) -> None:
    fragments = _object_list(raw_fragments, path)
    reading_keys: list[tuple[object, ...]] = []
    for expected_ordinal, fragment in enumerate(fragments):
        fragment_path = f"{path}[{expected_ordinal}]"
        ordinal = _integer(
            fragment["fragmentOrdinal"], f"{fragment_path}.fragmentOrdinal"
        )
        if ordinal != expected_ordinal:
            _fail(fragment_path, "fragment ordinals must be contiguous from zero")
        views = _object_list(fragment["views"], f"{fragment_path}.views")
        ranks: list[int] = []
        view_keys: list[tuple[object, ...]] = []
        for view_index, view in enumerate(views):
            view_path = f"{fragment_path}.views[{view_index}]"
            kind = _string(view["kind"], f"{view_path}.kind")
            rank = _VIEW_RANK[kind]
            ranks.append(rank)
            view_keys.append(
                _validate_locator_view(
                    view,
                    rank,
                    source_units,
                    references,
                    view_path,
                )
            )
        if ranks != sorted(ranks) or len(ranks) != len(set(ranks)):
            _fail(
                f"{fragment_path}.views",
                "view ranks must be unique and strictly increasing",
            )
        reading_keys.append(view_keys[0])
    if reading_keys != sorted(reading_keys) or len(reading_keys) != len(
        set(reading_keys)
    ):
        _fail(path, "fragments contain a duplicate or cycle-like source rewind")


def _validate_locator_view(  # noqa: PLR0911
    view: JsonObject,
    rank: int,
    source_units: _SourceUnitIndex,
    references: LocatorReferenceRegistry,
    path: str,
) -> tuple[object, ...]:
    kind = _string(view["kind"], f"{path}.kind")
    if kind == "source_text_position":
        unit_id = _string(view["opaqueSourceUnitId"], f"{path}.opaqueSourceUnitId")
        unit = _source_unit(source_units, unit_id, f"{path}.opaqueSourceUnitId")
        _validate_text_position(view, unit, path)
        return (
            rank,
            source_units.ordinal_by_id[unit_id],
            _integer(view["rawByteStart"], f"{path}.rawByteStart"),
            _integer(view["rawByteEnd"], f"{path}.rawByteEnd"),
        )
    if kind == "page_region":
        bbox = _validate_bbox(view["bboxMilliPoint"], f"{path}.bboxMilliPoint")
        return (
            rank,
            _integer(view["pageIndex"], f"{path}.pageIndex"),
            bbox[1],
            bbox[0],
        )
    if kind == "slide_shape":
        shape_id = _string(view["opaqueShapeId"], f"{path}.opaqueShapeId")
        _require_reference(
            shape_id, references.opaque_shape_ids, f"{path}.opaqueShapeId"
        )
        bbox_value = view.get("bboxMilliPoint")
        bbox = (
            (-1, -1, -1, -1)
            if bbox_value is None
            else _validate_bbox(bbox_value, f"{path}.bboxMilliPoint")
        )
        return (
            rank,
            _integer(view["slideIndex"], f"{path}.slideIndex"),
            shape_id,
            *bbox,
        )
    if kind == "sheet_range":
        sheet_id = _string(view["opaqueSheetId"], f"{path}.opaqueSheetId")
        _require_reference(
            sheet_id, references.opaque_sheet_ids, f"{path}.opaqueSheetId"
        )
        start = _a1_key(_string(view["startCell"], f"{path}.startCell"), path)
        end = _a1_key(_string(view["endCell"], f"{path}.endCell"), path)
        if start[0] > end[0] or start[1] > end[1]:
            _fail(path, "sheet range start must not follow its end")
        return (rank, sheet_id, *start, *end)
    if kind == "ooxml_path":
        unit_id = _string(view["opaqueSourceUnitId"], f"{path}.opaqueSourceUnitId")
        unit = _source_unit(source_units, unit_id, f"{path}.opaqueSourceUnitId")
        if unit["kind"] != "ooxml_part":
            _fail(path, "ooxml_path must reference an ooxml_part source unit")
        payload_ref = _string(
            view["canonicalXPathPayloadRef"], f"{path}.canonicalXPathPayloadRef"
        )
        _require_reference(
            payload_ref,
            references.canonical_xpath_payload_refs,
            f"{path}.canonicalXPathPayloadRef",
        )
        return (rank, source_units.ordinal_by_id[unit_id], payload_ref)
    if kind == "derived_structure":
        structure_id = _string(view["opaqueStructureId"], f"{path}.opaqueStructureId")
        _require_reference(
            structure_id,
            references.opaque_structure_ids,
            f"{path}.opaqueStructureId",
        )
        structure_kind = _string(view["structureKind"], f"{path}.structureKind")
        return (rank, _STRUCTURE_KIND_RANK[structure_kind], structure_id)
    return _fail(path, f"unsupported locator view {kind!r}")


def _validate_hashes(fixture: NormalizationSemanticFixture) -> None:
    expected_profile_hash = _string(
        fixture.normalization_map["normalizationProfileHash"],
        "normalizationMap.normalizationProfileHash",
    )
    if expected_profile_hash != normalization_profile_hash(
        fixture.normalization_profile
    ):
        _fail(
            "normalizationMap.normalizationProfileHash",
            "does not match canonical normalization profile",
        )
    map_domain = _string(
        fixture.normalization_profile["normalizationMapAggregateDomain"],
        "normalizationProfile.normalizationMapAggregateDomain",
    )
    if map_domain != NORMALIZATION_MAP_AGGREGATE_DOMAIN:
        _fail(
            "normalizationProfile.normalizationMapAggregateDomain", "unexpected domain"
        )
    hash_cases = (
        (
            fixture.normalization_map,
            map_domain,
            "normalizationMap.aggregateHash",
        ),
        (
            fixture.source_unit_resolver,
            SOURCE_UNIT_RESOLVER_AGGREGATE_DOMAIN,
            "sourceUnitResolver.aggregateHash",
        ),
        (
            fixture.source_locator,
            SOURCE_LOCATOR_AGGREGATE_DOMAIN,
            "sourceLocator.aggregateHash",
        ),
    )
    for value, domain, path in hash_cases:
        expected = _string(value["aggregateHash"], path)
        if expected != aggregate_hash(value, domain):
            _fail(path, "does not match canonical payload")


def _unique_records(
    raw_records: JsonValue,
    *,
    key_field: str,
    label: str,
    ordinal_field: str | None = None,
) -> dict[str, JsonObject]:
    records = _object_list(raw_records, label)
    result: dict[str, JsonObject] = {}
    for index, record in enumerate(records):
        path = f"{label}[{index}]"
        if ordinal_field is not None:
            ordinal = _integer(record[ordinal_field], f"{path}.{ordinal_field}")
            if ordinal != index:
                _fail(path, f"{ordinal_field} values must be contiguous from zero")
        key = _string(record[key_field], f"{path}.{key_field}")
        if key in result:
            _fail(path, f"duplicate {key_field} {key!r}")
        result[key] = record
    return result


def _utf8_boundaries(value: bytes) -> frozenset[int]:
    try:
        text = value.decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        raise NormalizationSemanticError("textBuffer is not strict UTF-8") from error
    boundaries = {0}
    offset = 0
    for character in text:
        offset += len(character.encode("utf-8"))
        boundaries.add(offset)
    return frozenset(boundaries)


def _require_utf8_boundary(
    value: int,
    boundaries: frozenset[int],
    path: str,
) -> None:
    if value not in boundaries:
        _fail(path, "offset is not a UTF-8 scalar boundary")


def _validate_bbox(value: JsonValue, path: str) -> tuple[int, int, int, int]:
    bbox = _integer_tuple(value, path)
    if len(bbox) != 4:
        _fail(path, "bbox must contain four integers")
    if bbox[0] >= bbox[2] or bbox[1] >= bbox[3]:
        _fail(path, "bbox must have positive half-open area")
    return bbox


def _a1_key(value: str, path: str) -> tuple[int, int]:
    match = _A1_CELL.fullmatch(value)
    if match is None:  # JSON Schema normally catches this first.
        _fail(path, "invalid A1 cell")
    column = 0
    for character in match.group(1):
        column = column * 26 + ord(character) - ord("A") + 1
    return (int(match.group(2)), column)


def _source_unit(
    source_units: _SourceUnitIndex,
    opaque_id: str,
    path: str,
) -> JsonObject:
    try:
        return source_units.by_id[opaque_id]
    except KeyError as error:
        raise NormalizationSemanticError(
            f"{path}: opaqueSourceUnitId does not resolve"
        ) from error


def _require_reference(value: str, known: frozenset[str], path: str) -> None:
    if value not in known:
        _fail(path, "reference does not resolve in fixture registry")


def _reference_set(value: JsonValue, path: str) -> frozenset[str]:
    items = _list(value, path)
    references = tuple(
        _string(item, f"{path}[{index}]") for index, item in enumerate(items)
    )
    if references != tuple(sorted(references)) or len(references) != len(
        set(references)
    ):
        _fail(path, "reference registry values must be unique and sorted")
    return frozenset(references)


def _require_exact_fields(
    value: JsonObject, expected: frozenset[str], path: str
) -> None:
    if set(value) != expected:
        _fail(path, f"fields must equal {sorted(expected)!r}")


def _object(value: JsonValue, path: str) -> JsonObject:
    if not isinstance(value, dict):
        _fail(path, "must be an object")
    return value


def _object_list(value: JsonValue, path: str) -> list[JsonObject]:
    return [
        _object(item, f"{path}[{index}]")
        for index, item in enumerate(_list(value, path))
    ]


def _list(value: JsonValue, path: str) -> list[JsonValue]:
    if not isinstance(value, list):
        _fail(path, "must be an array")
    return value


def _string(value: JsonValue, path: str) -> str:
    if not isinstance(value, str):
        _fail(path, "must be a string")
    return value


def _integer(value: JsonValue, path: str) -> int:
    if type(value) is not int:
        _fail(path, "must be an integer")
    return value


def _integer_tuple(value: JsonValue, path: str) -> tuple[int, ...]:
    return tuple(
        _integer(item, f"{path}[{index}]")
        for index, item in enumerate(_list(value, path))
    )


def _fail(path: str, message: str) -> Never:
    raise NormalizationSemanticError(f"{path}: {message}")
