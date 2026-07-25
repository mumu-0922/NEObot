"""Deterministic, token-aware structure-first Parent/Child planning.

The planner consumes validated source units and optional precomputed semantic
boundary hints. It never calls a provider and emits source-unit byte ranges
only; artifact mappers remain responsible for locators and provenance.
"""

from __future__ import annotations

import bisect
import hashlib
import json
from dataclasses import dataclass
from typing import Final, Literal, cast

from mm_chat_rag.frozen_tokenizer import (
    FROZEN_TOKENIZER,
    TOKENIZER_ARTIFACT_SHA256,
    TOKENIZER_NAME,
    TOKENIZER_NORMALIZATION,
    TOKENIZER_ORDINARY_TEXT_POLICY,
    TOKENIZER_REVISION,
    FrozenTokenizer,
)
from mm_chat_rag.semantic_profile import (
    SEMANTIC_BOUNDARY_PROFILE_HASH,
    SILICONFLOW_SEMANTIC_BOUNDARY_PROFILE_HASH,
)

CHILD_TARGET_MIN_TOKENS: Final = 300
CHILD_TARGET_TOKENS: Final = 400
CHILD_TARGET_MAX_TOKENS: Final = 500
CHILD_HARD_MAX_TOKENS: Final = 650
PARENT_TARGET_MIN_TOKENS: Final = 1200
PARENT_TARGET_MAX_TOKENS: Final = 1600
PARENT_HARD_MAX_TOKENS: Final = 2000
OVERLAP_TARGET_TOKENS: Final = 64
OVERLAP_MAX_TOKENS: Final = 100
DERIVED_CONTEXT_MAX_TOKENS: Final = 96

_PROFILE: Final = {
    "bounds": {
        "child": {
            "hardMaximum": CHILD_HARD_MAX_TOKENS,
            "target": CHILD_TARGET_TOKENS,
            "targetMaximum": CHILD_TARGET_MAX_TOKENS,
            "targetMinimum": CHILD_TARGET_MIN_TOKENS,
        },
        "overlap": {
            "maximum": OVERLAP_MAX_TOKENS,
            "target": OVERLAP_TARGET_TOKENS,
        },
        "parent": {
            "hardMaximum": PARENT_HARD_MAX_TOKENS,
            "targetMaximum": PARENT_TARGET_MAX_TOKENS,
            "targetMinimum": PARENT_TARGET_MIN_TOKENS,
        },
    },
    "derivedContext": {
        "citationAuthority": "original_source_span",
        "countedInOverlap": False,
        "maximumTokens": DERIVED_CONTEXT_MAX_TOKENS,
    },
    "nonIndexable": {
        "policy": "preserve_source_exclude_retrieval",
        "signals": ["repeated_text", "page_position", "frequency"],
    },
    "routes": {
        "code": "logical_lines_then_token",
        "formula": "atomic_then_token",
        "json": "subtree_path_then_token",
        "narrative": "semantic_hint_then_sentence_recursive",
        "slide": "slide_shape_then_token",
        "table": "header_row_group_then_token",
    },
    "schemaVersion": "mm-chat.structure-chunk-profile.v2",
    "semantic": {
        "admission": "long_unstructured_narrative_only",
        "failure": "deterministic_sentence_recursive_fallback",
        "hintAuthority": "content_and_embedding_profile_hash_bound",
        "profileHash": SEMANTIC_BOUNDARY_PROFILE_HASH,
    },
    "tokenizer": {
        "artifactSha256": TOKENIZER_ARTIFACT_SHA256,
        "name": TOKENIZER_NAME,
        "normalization": TOKENIZER_NORMALIZATION,
        "profileHash": FROZEN_TOKENIZER.profile_hash,
        "revision": TOKENIZER_REVISION,
        "specialTokenPolicy": TOKENIZER_ORDINARY_TEXT_POLICY,
        "vocabularySha256": FROZEN_TOKENIZER.vocabulary_sha256,
    },
}
STRUCTURE_CHUNK_PROFILE_HASH: Final = hashlib.sha256(
    json.dumps(
        _PROFILE,
        ensure_ascii=True,
        separators=(",", ":"),
        sort_keys=True,
    ).encode("utf-8")
).hexdigest()
_SILICONFLOW_PROFILE: Final = cast(
    "dict[str, object]",
    json.loads(json.dumps(_PROFILE)),
)
_SILICONFLOW_PROFILE["schemaVersion"] = "mm-chat.structure-chunk-profile.v3"
_siliconflow_semantic = cast("dict[str, object]", _SILICONFLOW_PROFILE["semantic"])
_siliconflow_semantic["profileHash"] = SILICONFLOW_SEMANTIC_BOUNDARY_PROFILE_HASH
SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH: Final = hashlib.sha256(
    json.dumps(
        _SILICONFLOW_PROFILE,
        ensure_ascii=True,
        separators=(",", ":"),
        sort_keys=True,
    ).encode("utf-8")
).hexdigest()
SUPPORTED_STRUCTURE_CHUNK_PROFILE_HASHES: Final = frozenset(
    {
        STRUCTURE_CHUNK_PROFILE_HASH,
        SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH,
    }
)

_ATOM_TARGET_TOKENS: Final = OVERLAP_TARGET_TOKENS
_MAX_UNITS: Final = 100_000
_MAX_TOTAL_BYTES: Final = 32 * 1024 * 1024
_MAX_UNIT_BYTES: Final = 4 * 1024 * 1024
_MAX_HEADING_BYTES: Final = 512
_NARRATIVE_KINDS: Final = frozenset(
    {"paragraph", "list", "list_item", "quote", "raw_html"}
)
_TABLE_KINDS: Final = frozenset({"table", "table_row", "table_cell", "sheet"})
_CODE_KINDS: Final = frozenset({"code"})
_JSON_KINDS: Final = frozenset({"json", "json_object", "json_array"})
_SLIDE_KINDS: Final = frozenset({"slide", "shape", "notes"})
_ALLOWED_KINDS: Final = frozenset(
    {
        "heading",
        "paragraph",
        "list",
        "list_item",
        "quote",
        "code",
        "table",
        "table_row",
        "table_cell",
        "raw_html",
        "formula",
        "footnote",
        "endnote",
        "header",
        "footer",
        "slide",
        "shape",
        "notes",
        "sheet",
        "json",
        "json_object",
        "json_array",
    }
)
_GENERAL_BOUNDARY_CHARACTERS: Final = frozenset(
    " \t\r\n,.;:!?\uff0c\u3002\uff1b\uff1a\uff01\uff1f\u3001)]}\uff09\u3011\u300b"
)
_SENTENCE_BOUNDARY_CHARACTERS: Final = frozenset("\n.!?\u3002\uff01\uff1f\uff1b;")
_CODE_BOUNDARY_CHARACTERS: Final = frozenset("\n;{}")
_TABLE_BOUNDARY_CHARACTERS: Final = frozenset("\n|")
_JSON_BOUNDARY_CHARACTERS: Final = frozenset("\n,]}")
_SLIDE_BOUNDARY_CHARACTERS: Final = frozenset("\n")

ChunkStrategy = Literal[
    "code_logical",
    "formula_atomic",
    "json_subtree",
    "non_indexable",
    "sentence_recursive",
    "sentence_semantic",
    "slide_shape",
    "structure_atomic",
    "table_row_group",
]
DerivedContextReason = Literal[
    "code_signature",
    "heading_context",
    "json_path",
    "sheet_header",
    "slide_title",
    "table_header",
]


class StructureChunkingError(ValueError):
    """Validated structural text cannot be planned within frozen bounds."""


@dataclass(frozen=True, slots=True)
class StructuredTextUnit:
    """One validated source-derived unit in canonical reading order."""

    ordinal: int
    kind: str
    text: str
    heading_path: tuple[str, ...] = ()
    indexable: bool = True


@dataclass(frozen=True, slots=True)
class SemanticBoundaryHints:
    """Provider-produced boundaries already bound to exact source content."""

    unit_ordinal: int
    content_sha256: str
    embedding_profile_hash: str
    boundary_bytes: tuple[int, ...]


@dataclass(frozen=True, slots=True)
class UnitChunkDiagnostic:
    """Read-only explanation of the automatic strategy selected per unit."""

    unit_ordinal: int
    strategy: ChunkStrategy
    source_token_count: int
    emitted_atom_count: int


@dataclass(frozen=True, slots=True)
class ChunkFragmentPlan:
    """One UTF-8-safe range into a StructuredTextUnit."""

    unit_ordinal: int
    start_byte: int
    end_byte: int
    token_count: int
    overlap: bool = False


@dataclass(frozen=True, slots=True)
class ParentChunkPlan:
    """One section-bounded Parent chunk containing unique source ranges."""

    ordinal: int
    heading_path: tuple[str, ...]
    fragments: tuple[ChunkFragmentPlan, ...]
    token_count: int


@dataclass(frozen=True, slots=True)
class ChildChunkPlan:
    """One retrieval Child window and its exact adjacent overlap."""

    ordinal: int
    parent_ordinal: int
    ordinal_in_parent: int
    fragments: tuple[ChunkFragmentPlan, ...]
    token_count: int
    overlap_before_tokens: int
    derived_contexts: tuple[DerivedContextPlan, ...] = ()


@dataclass(frozen=True, slots=True)
class DerivedContextPlan:
    """One bounded source-backed prefix that is not citation quote authority."""

    fragment: ChunkFragmentPlan
    reason: DerivedContextReason


@dataclass(frozen=True, slots=True)
class StructureChunkPlan:
    """Complete deterministic plan for one validated document."""

    parents: tuple[ParentChunkPlan, ...]
    children: tuple[ChildChunkPlan, ...]
    diagnostics: tuple[UnitChunkDiagnostic, ...]


@dataclass(frozen=True, slots=True)
class _Atom:
    unit_ordinal: int
    start_byte: int
    end_byte: int
    token_count: int
    heading_path: tuple[str, ...]


def plan_structure_chunks(
    units: tuple[StructuredTextUnit, ...],
    *,
    semantic_hints: tuple[SemanticBoundaryHints, ...] = (),
    semantic_profile_hash: str = SEMANTIC_BOUNDARY_PROFILE_HASH,
    tokenizer: FrozenTokenizer = FROZEN_TOKENIZER,
) -> StructureChunkPlan:
    """Plan bounded Parent/Child windows without changing source authority."""
    _validate_units(units)
    hints_by_unit = _validate_semantic_hints(
        units,
        semantic_hints,
        semantic_profile_hash,
    )
    atoms: list[_Atom] = []
    diagnostics: list[UnitChunkDiagnostic] = []
    for unit in units:
        if not unit.indexable:
            diagnostics.append(UnitChunkDiagnostic(unit.ordinal, "non_indexable", 0, 0))
            continue
        unit_atoms, strategy, token_count = _unit_atoms(
            unit,
            hints_by_unit.get(unit.ordinal),
            tokenizer,
        )
        atoms.extend(unit_atoms)
        diagnostics.append(
            UnitChunkDiagnostic(
                unit.ordinal,
                strategy,
                token_count,
                len(unit_atoms),
            )
        )
    if not atoms:
        raise StructureChunkingError("document has no indexable structural text")
    atom_tuple = tuple(atoms)
    parents = _plan_parents(units, atom_tuple, tokenizer)
    children: list[ChildChunkPlan] = []
    for parent in parents:
        parent_atoms = tuple(
            _Atom(
                unit_ordinal=fragment.unit_ordinal,
                start_byte=fragment.start_byte,
                end_byte=fragment.end_byte,
                token_count=fragment.token_count,
                heading_path=parent.heading_path,
            )
            for fragment in parent.fragments
        )
        children.extend(
            _plan_children(
                units,
                parent.ordinal,
                parent_atoms,
                len(children),
                tokenizer,
            )
        )
    return StructureChunkPlan(
        parents=parents,
        children=tuple(children),
        diagnostics=tuple(diagnostics),
    )


def structure_chunk_profile(
    chunk_profile_hash: str = STRUCTURE_CHUNK_PROFILE_HASH,
) -> dict[str, object]:
    """Return a detached diagnostic view of one frozen structure profile."""
    if chunk_profile_hash == STRUCTURE_CHUNK_PROFILE_HASH:
        profile = _PROFILE
    elif chunk_profile_hash == SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH:
        profile = _SILICONFLOW_PROFILE
    else:
        raise StructureChunkingError("structure chunk profile is unsupported")
    return cast("dict[str, object]", json.loads(json.dumps(profile)))


def structure_semantic_profile_hash(chunk_profile_hash: str) -> str:
    """Resolve the only semantic profile admitted by a structure profile."""
    if chunk_profile_hash == STRUCTURE_CHUNK_PROFILE_HASH:
        return SEMANTIC_BOUNDARY_PROFILE_HASH
    if chunk_profile_hash == SILICONFLOW_STRUCTURE_CHUNK_PROFILE_HASH:
        return SILICONFLOW_SEMANTIC_BOUNDARY_PROFILE_HASH
    raise StructureChunkingError("structure chunk profile is unsupported")


def _validate_units(units: tuple[StructuredTextUnit, ...]) -> None:
    if not isinstance(units, tuple) or not units or len(units) > _MAX_UNITS:
        raise StructureChunkingError("structural units are missing or unbounded")
    total_bytes = 0
    for expected_ordinal, unit in enumerate(units):
        if (
            not isinstance(unit, StructuredTextUnit)
            or type(unit.ordinal) is not int
            or unit.ordinal != expected_ordinal
        ):
            raise StructureChunkingError("structural unit ordinals are not contiguous")
        if not isinstance(unit.kind, str) or unit.kind not in _ALLOWED_KINDS:
            raise StructureChunkingError("structural unit kind is unsupported")
        if type(unit.indexable) is not bool:
            raise StructureChunkingError("structural unit indexability is invalid")
        if (
            not isinstance(unit.text, str)
            or not unit.text.strip()
            or "\x00" in unit.text
        ):
            raise StructureChunkingError("structural unit text is invalid")
        try:
            encoded = unit.text.encode("utf-8", errors="strict")
        except UnicodeError as error:
            raise StructureChunkingError("structural unit text is invalid") from error
        if len(encoded) > _MAX_UNIT_BYTES:
            raise StructureChunkingError("structural unit exceeds the byte limit")
        total_bytes += len(encoded)
        if total_bytes > _MAX_TOTAL_BYTES:
            raise StructureChunkingError("document exceeds the chunk planner limit")
        _validate_heading_path(unit.heading_path)


def _validate_heading_path(heading_path: tuple[str, ...]) -> None:
    if not isinstance(heading_path, tuple):
        raise StructureChunkingError("heading path is invalid")
    for value in heading_path:
        if not isinstance(value, str) or not value.strip() or "\x00" in value:
            raise StructureChunkingError("heading path is invalid")
        try:
            encoded = value.encode("utf-8", errors="strict")
        except UnicodeError as error:
            raise StructureChunkingError("heading path is invalid") from error
        if len(encoded) > _MAX_HEADING_BYTES:
            raise StructureChunkingError("heading path is invalid")


def _validate_semantic_hints(
    units: tuple[StructuredTextUnit, ...],
    hints: tuple[SemanticBoundaryHints, ...],
    semantic_profile_hash: str,
) -> dict[int, SemanticBoundaryHints]:
    if not isinstance(hints, tuple):
        raise StructureChunkingError("semantic boundary hints are invalid")
    result: dict[int, SemanticBoundaryHints] = {}
    for hint in hints:
        if (
            not isinstance(hint, SemanticBoundaryHints)
            or type(hint.unit_ordinal) is not int
            or hint.unit_ordinal < 0
            or hint.unit_ordinal >= len(units)
            or hint.unit_ordinal in result
        ):
            raise StructureChunkingError("semantic boundary hints are invalid")
        unit = units[hint.unit_ordinal]
        encoded = unit.text.encode("utf-8")
        if (
            unit.kind not in _NARRATIVE_KINDS
            or not unit.indexable
            or hint.content_sha256 != hashlib.sha256(encoded).hexdigest()
            or hint.embedding_profile_hash != semantic_profile_hash
            or not isinstance(hint.boundary_bytes, tuple)
            or not hint.boundary_bytes
        ):
            raise StructureChunkingError("semantic boundary hints are invalid")
        scalar_offsets = set(_utf8_scalar_offsets(unit.text))
        previous = 0
        for boundary in hint.boundary_bytes:
            if (
                type(boundary) is not int
                or boundary <= previous
                or boundary >= len(encoded)
                or boundary not in scalar_offsets
            ):
                raise StructureChunkingError("semantic boundary hints are invalid")
            previous = boundary
        result[hint.unit_ordinal] = hint
    return result


def _unit_atoms(
    unit: StructuredTextUnit,
    semantic_hint: SemanticBoundaryHints | None,
    tokenizer: FrozenTokenizer,
) -> tuple[tuple[_Atom, ...], ChunkStrategy, int]:
    token_count = tokenizer.count(unit.text)
    strategy = _strategy(unit, semantic_hint, token_count)
    total_bytes = len(unit.text.encode("utf-8"))
    ranges: tuple[tuple[int, int], ...]
    if strategy in {"formula_atomic", "structure_atomic"} and (
        token_count <= CHILD_TARGET_MAX_TOKENS
    ):
        ranges = ((0, total_bytes),)
    elif unit.kind in _NARRATIVE_KINDS:
        ranges = _split_utf8_ranges(
            unit.text,
            _ATOM_TARGET_TOKENS,
            tokenizer,
            preferred_boundaries=(
                semantic_hint.boundary_bytes if semantic_hint is not None else ()
            ),
            kind_boundaries=_SENTENCE_BOUNDARY_CHARACTERS,
        )
    else:
        ranges = _split_utf8_ranges(
            unit.text,
            CHILD_TARGET_MAX_TOKENS,
            tokenizer,
            kind_boundaries=_kind_boundaries(unit.kind),
        )
    atoms = tuple(
        _Atom(
            unit_ordinal=unit.ordinal,
            start_byte=start,
            end_byte=end,
            token_count=tokenizer.count(
                unit.text.encode("utf-8")[start:end].decode("utf-8")
            ),
            heading_path=unit.heading_path,
        )
        for start, end in ranges
    )
    if any(atom.token_count > CHILD_TARGET_MAX_TOKENS for atom in atoms):
        raise StructureChunkingError("structural atom exceeds the token limit")
    return atoms, strategy, token_count


def _strategy(
    unit: StructuredTextUnit,
    semantic_hint: SemanticBoundaryHints | None,
    token_count: int,
) -> ChunkStrategy:
    if unit.kind in _NARRATIVE_KINDS:
        if semantic_hint is not None and token_count > CHILD_TARGET_MAX_TOKENS:
            return "sentence_semantic"
        return "sentence_recursive"
    kind_strategies: tuple[tuple[frozenset[str], ChunkStrategy], ...] = (
        (_TABLE_KINDS, "table_row_group"),
        (_CODE_KINDS, "code_logical"),
        (_JSON_KINDS, "json_subtree"),
        (_SLIDE_KINDS, "slide_shape"),
        (frozenset({"formula"}), "formula_atomic"),
    )
    return next(
        (strategy for kinds, strategy in kind_strategies if unit.kind in kinds),
        "structure_atomic",
    )


def _kind_boundaries(kind: str) -> frozenset[str]:
    if kind in _CODE_KINDS:
        return _CODE_BOUNDARY_CHARACTERS
    if kind in _TABLE_KINDS:
        return _TABLE_BOUNDARY_CHARACTERS
    if kind in _JSON_KINDS:
        return _JSON_BOUNDARY_CHARACTERS
    if kind in _SLIDE_KINDS:
        return _SLIDE_BOUNDARY_CHARACTERS
    return _GENERAL_BOUNDARY_CHARACTERS


def _split_utf8_ranges(
    text: str,
    target_tokens: int,
    tokenizer: FrozenTokenizer,
    *,
    preferred_boundaries: tuple[int, ...] = (),
    kind_boundaries: frozenset[str] = _GENERAL_BOUNDARY_CHARACTERS,
) -> tuple[tuple[int, int], ...]:
    encoded = text.encode("utf-8")
    total_bytes = len(encoded)
    scalar_offsets = _utf8_scalar_offsets(text)
    token_ends = tokenizer.token_end_bytes(text)
    general: list[int] = []
    structural: list[int] = []
    byte_offset = 0
    for character in text:
        byte_offset += len(character.encode("utf-8"))
        if character in _GENERAL_BOUNDARY_CHARACTERS:
            general.append(byte_offset)
        if character in kind_boundaries:
            structural.append(byte_offset)

    ranges: list[tuple[int, int]] = []
    start = 0
    while start < total_bytes:
        consumed_tokens = bisect.bisect_right(token_ends, start)
        approximate_remaining = len(token_ends) - consumed_tokens
        if approximate_remaining <= target_tokens and (
            tokenizer.count(encoded[start:].decode("utf-8")) <= target_tokens
        ):
            ranges.append((start, total_bytes))
            break
        target_index = min(consumed_tokens + target_tokens - 1, len(token_ends) - 1)
        approximate_end = token_ends[target_index]
        safe_index = bisect.bisect_right(scalar_offsets, approximate_end) - 1
        maximum_end = scalar_offsets[safe_index]
        if maximum_end <= start:
            raise StructureChunkingError("unable to find a UTF-8-safe boundary")
        minimum_end = start + max(1, (maximum_end - start) // 2)
        boundary = _preferred_boundary(
            preferred_boundaries,
            structural,
            general,
            minimum_end=minimum_end,
            maximum_end=maximum_end,
        )
        if boundary <= start:
            boundary = maximum_end
        boundary = _fit_token_limit(
            encoded,
            scalar_offsets,
            start,
            boundary,
            target_tokens,
            tokenizer,
        )
        if boundary <= start:
            raise StructureChunkingError("unable to find a token-safe boundary")
        ranges.append((start, boundary))
        start = boundary
    if not ranges:
        raise StructureChunkingError("structural unit produced no chunk ranges")
    return tuple(ranges)


def _preferred_boundary(
    preferred: tuple[int, ...],
    structural: list[int],
    general: list[int],
    *,
    minimum_end: int,
    maximum_end: int,
) -> int:
    for boundaries in (preferred, structural, general):
        index = bisect.bisect_right(boundaries, maximum_end) - 1
        if index >= 0 and boundaries[index] >= minimum_end:
            return boundaries[index]
    return maximum_end


def _fit_token_limit(
    encoded: bytes,
    scalar_offsets: tuple[int, ...],
    start: int,
    candidate: int,
    token_limit: int,
    tokenizer: FrozenTokenizer,
) -> int:
    if tokenizer.count(encoded[start:candidate].decode("utf-8")) <= token_limit:
        return candidate
    low = bisect.bisect_right(scalar_offsets, start)
    high = bisect.bisect_left(scalar_offsets, candidate)
    best = start
    while low <= high:
        middle = (low + high) // 2
        boundary = scalar_offsets[middle]
        count = tokenizer.count(encoded[start:boundary].decode("utf-8"))
        if count <= token_limit:
            best = boundary
            low = middle + 1
        else:
            high = middle - 1
    return best


def _utf8_scalar_offsets(text: str) -> tuple[int, ...]:
    offsets = [0]
    for character in text:
        offsets.append(offsets[-1] + len(character.encode("utf-8")))
    return tuple(offsets)


def _plan_parents(
    units: tuple[StructuredTextUnit, ...],
    atoms: tuple[_Atom, ...],
    tokenizer: FrozenTokenizer,
) -> tuple[ParentChunkPlan, ...]:
    parents: list[ParentChunkPlan] = []
    current: list[_Atom] = []
    current_heading: tuple[str, ...] | None = None
    for atom in atoms:
        heading_changed = (
            current_heading is not None and atom.heading_path != current_heading
        )
        candidate = (*current, atom)
        candidate_tokens = _chunk_token_count(units, candidate, tokenizer)
        current_tokens = _chunk_token_count(units, current, tokenizer) if current else 0
        target_reached = (
            current_tokens >= PARENT_TARGET_MIN_TOKENS
            and candidate_tokens > PARENT_TARGET_MAX_TOKENS
        )
        hard_limit = candidate_tokens > PARENT_HARD_MAX_TOKENS
        if current and (heading_changed or target_reached or hard_limit):
            parents.append(
                _parent_plan(
                    units,
                    len(parents),
                    current_heading or (),
                    current,
                    tokenizer,
                )
            )
            current = []
            current_heading = None
        current_heading = (
            atom.heading_path if current_heading is None else current_heading
        )
        current.append(atom)
    if current:
        parents.append(
            _parent_plan(
                units,
                len(parents),
                current_heading or (),
                current,
                tokenizer,
            )
        )
    return tuple(parents)


def _parent_plan(
    units: tuple[StructuredTextUnit, ...],
    ordinal: int,
    heading_path: tuple[str, ...],
    atoms: list[_Atom],
    tokenizer: FrozenTokenizer,
) -> ParentChunkPlan:
    token_count = _chunk_token_count(units, atoms, tokenizer)
    if token_count > PARENT_HARD_MAX_TOKENS:
        raise StructureChunkingError("parent chunk exceeds the hard token limit")
    return ParentChunkPlan(
        ordinal=ordinal,
        heading_path=heading_path,
        fragments=tuple(_fragment(atom) for atom in atoms),
        token_count=token_count,
    )


def _plan_children(
    units: tuple[StructuredTextUnit, ...],
    parent_ordinal: int,
    atoms: tuple[_Atom, ...],
    first_global_ordinal: int,
    tokenizer: FrozenTokenizer,
) -> tuple[ChildChunkPlan, ...]:
    children: list[ChildChunkPlan] = []
    cursor = 0
    previous_primary: tuple[_Atom, ...] = ()
    while cursor < len(atoms):
        overlap = _overlap_suffix(units, previous_primary, tokenizer)
        overlap_tokens = _chunk_token_count(units, overlap, tokenizer) if overlap else 0
        primary: list[_Atom] = []
        while cursor < len(atoms):
            atom = atoms[cursor]
            candidate = (*overlap, *primary, atom)
            candidate_tokens = _chunk_token_count(units, candidate, tokenizer)
            if primary and candidate_tokens > CHILD_TARGET_MAX_TOKENS:
                break
            if not primary and candidate_tokens > CHILD_HARD_MAX_TOKENS and overlap:
                overlap = ()
                overlap_tokens = 0
                candidate = (atom,)
                candidate_tokens = _chunk_token_count(units, candidate, tokenizer)
            if candidate_tokens > CHILD_HARD_MAX_TOKENS:
                raise StructureChunkingError("child chunk exceeds the hard token limit")
            primary.append(atom)
            cursor += 1
            if candidate_tokens >= CHILD_TARGET_TOKENS:
                break
            if cursor < len(atoms) and candidate_tokens >= CHILD_TARGET_MIN_TOKENS:
                next_tokens = _chunk_token_count(
                    units,
                    (*overlap, *primary, atoms[cursor]),
                    tokenizer,
                )
                if next_tokens > CHILD_TARGET_TOKENS:
                    break
        if not primary:
            raise StructureChunkingError("child chunk made no forward progress")
        source_atoms = (*overlap, *primary)
        derived_contexts = _derived_contexts(
            units,
            source_atoms,
            tokenizer,
        )
        derived_atoms = tuple(
            _atom_from_fragment(units, item.fragment) for item in derived_contexts
        )
        token_count = _chunk_token_count(
            units,
            (*derived_atoms, *source_atoms),
            tokenizer,
        )
        if token_count > CHILD_HARD_MAX_TOKENS:
            derived_contexts = ()
            token_count = _chunk_token_count(units, source_atoms, tokenizer)
        ordinal_in_parent = len(children)
        fragments = tuple(_fragment(atom, overlap=True) for atom in overlap) + tuple(
            _fragment(atom) for atom in primary
        )
        children.append(
            ChildChunkPlan(
                ordinal=first_global_ordinal + ordinal_in_parent,
                parent_ordinal=parent_ordinal,
                ordinal_in_parent=ordinal_in_parent,
                fragments=fragments,
                token_count=token_count,
                overlap_before_tokens=overlap_tokens,
                derived_contexts=derived_contexts,
            )
        )
        previous_primary = tuple(primary)
    return tuple(children)


def _derived_contexts(
    units: tuple[StructuredTextUnit, ...],
    source_atoms: tuple[_Atom, ...],
    tokenizer: FrozenTokenizer,
) -> tuple[DerivedContextPlan, ...]:
    if not source_atoms:
        return ()
    source_ranges = {
        (atom.unit_ordinal, atom.start_byte, atom.end_byte) for atom in source_atoms
    }
    candidates: list[DerivedContextPlan] = []
    first = source_atoms[0]
    heading_context = _heading_context(units, first.heading_path, tokenizer)
    if heading_context is not None:
        candidates.append(heading_context)
    type_context = _type_context(units[first.unit_ordinal], first, tokenizer)
    if type_context is not None:
        candidates.append(type_context)

    selected: list[DerivedContextPlan] = []
    for candidate in candidates:
        fragment = candidate.fragment
        identity = (
            fragment.unit_ordinal,
            fragment.start_byte,
            fragment.end_byte,
        )
        if identity in source_ranges:
            continue
        proposed = (*selected, candidate)
        proposed_atoms = tuple(
            _atom_from_fragment(units, item.fragment) for item in proposed
        )
        if _chunk_token_count(units, proposed_atoms, tokenizer) > (
            DERIVED_CONTEXT_MAX_TOKENS
        ):
            continue
        selected.append(candidate)
    return tuple(selected)


def _heading_context(
    units: tuple[StructuredTextUnit, ...],
    heading_path: tuple[str, ...],
    tokenizer: FrozenTokenizer,
) -> DerivedContextPlan | None:
    if not heading_path:
        return None
    heading_id = heading_path[-1]
    for unit in reversed(units):
        if (
            unit.kind == "heading"
            and unit.heading_path
            and unit.heading_path[-1] == heading_id
        ):
            fragment = _prefix_fragment(unit, tokenizer)
            return DerivedContextPlan(fragment, "heading_context")
    return None


def _type_context(
    unit: StructuredTextUnit,
    first: _Atom,
    tokenizer: FrozenTokenizer,
) -> DerivedContextPlan | None:
    if first.start_byte == 0:
        return None
    reason: DerivedContextReason | None = None
    if unit.kind in {"table", "table_row", "table_cell"}:
        reason = "table_header"
    elif unit.kind == "sheet":
        reason = "sheet_header"
    elif unit.kind == "code":
        reason = "code_signature"
    elif unit.kind in _JSON_KINDS:
        reason = "json_path"
    elif unit.kind in _SLIDE_KINDS:
        reason = "slide_title"
    if reason is None:
        return None
    return DerivedContextPlan(_prefix_fragment(unit, tokenizer), reason)


def _prefix_fragment(
    unit: StructuredTextUnit,
    tokenizer: FrozenTokenizer,
) -> ChunkFragmentPlan:
    encoded = unit.text.encode("utf-8")
    first_line_end = encoded.find(b"\n")
    preferred_end = len(encoded) if first_line_end < 0 else first_line_end
    if preferred_end <= 0:
        preferred_end = len(encoded)
    prefix = encoded[:preferred_end].decode("utf-8")
    if tokenizer.count(prefix) > DERIVED_CONTEXT_MAX_TOKENS:
        preferred_end = _split_utf8_ranges(
            prefix,
            DERIVED_CONTEXT_MAX_TOKENS,
            tokenizer,
        )[0][1]
        prefix = encoded[:preferred_end].decode("utf-8")
    return ChunkFragmentPlan(
        unit_ordinal=unit.ordinal,
        start_byte=0,
        end_byte=preferred_end,
        token_count=tokenizer.count(prefix),
    )


def _atom_from_fragment(
    units: tuple[StructuredTextUnit, ...],
    fragment: ChunkFragmentPlan,
) -> _Atom:
    return _Atom(
        unit_ordinal=fragment.unit_ordinal,
        start_byte=fragment.start_byte,
        end_byte=fragment.end_byte,
        token_count=fragment.token_count,
        heading_path=units[fragment.unit_ordinal].heading_path,
    )


def _overlap_suffix(
    units: tuple[StructuredTextUnit, ...],
    previous_primary: tuple[_Atom, ...],
    tokenizer: FrozenTokenizer,
) -> tuple[_Atom, ...]:
    selected: list[_Atom] = []
    for atom in reversed(previous_primary):
        candidate = (atom, *selected)
        tokens = _chunk_token_count(units, candidate, tokenizer)
        if tokens > OVERLAP_MAX_TOKENS:
            break
        selected.insert(0, atom)
        if tokens >= OVERLAP_TARGET_TOKENS:
            break
    return tuple(selected)


def _chunk_token_count(
    units: tuple[StructuredTextUnit, ...],
    atoms: tuple[_Atom, ...] | list[_Atom],
    tokenizer: FrozenTokenizer,
) -> int:
    return tokenizer.count(_chunk_text(units, atoms))


def _chunk_text(
    units: tuple[StructuredTextUnit, ...],
    atoms: tuple[_Atom, ...] | list[_Atom],
) -> str:
    parts: list[str] = []
    previous: _Atom | None = None
    for atom in atoms:
        if previous is not None:
            adjacent = (
                previous.unit_ordinal == atom.unit_ordinal
                and previous.end_byte == atom.start_byte
            )
            if not adjacent:
                parts.append("\n\n")
        encoded = units[atom.unit_ordinal].text.encode("utf-8")
        parts.append(encoded[atom.start_byte : atom.end_byte].decode("utf-8"))
        previous = atom
    return "".join(parts)


def _fragment(atom: _Atom, *, overlap: bool = False) -> ChunkFragmentPlan:
    return ChunkFragmentPlan(
        unit_ordinal=atom.unit_ordinal,
        start_byte=atom.start_byte,
        end_byte=atom.end_byte,
        token_count=atom.token_count,
        overlap=overlap,
    )
