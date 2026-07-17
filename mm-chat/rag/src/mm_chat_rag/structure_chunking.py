"""Deterministic structure-aware Parent/Child chunk planning.

The planner is deliberately pure. It references validated structural units by
ordinal and UTF-8 byte range, but does not read files, mint source locators,
write projections, call providers, or switch an Index Generation.
"""

from __future__ import annotations

import bisect
import math
from dataclasses import dataclass
from typing import Final

CHILD_TARGET_MIN_TOKENS: Final = 300
CHILD_TARGET_TOKENS: Final = 400
CHILD_TARGET_MAX_TOKENS: Final = 500
CHILD_HARD_MAX_TOKENS: Final = 650
PARENT_TARGET_MIN_TOKENS: Final = 1200
PARENT_TARGET_MAX_TOKENS: Final = 1600
PARENT_HARD_MAX_TOKENS: Final = 2000
OVERLAP_TARGET_TOKENS: Final = 64
OVERLAP_MAX_TOKENS: Final = 100

_TOKEN_BYTES: Final = 4
_ATOM_TARGET_TOKENS: Final = OVERLAP_TARGET_TOKENS
_MAX_UNITS: Final = 100_000
_MAX_TOTAL_BYTES: Final = 32 * 1024 * 1024
_MAX_UNIT_BYTES: Final = 4 * 1024 * 1024
_MAX_HEADING_BYTES: Final = 512
_SPLITTABLE_KINDS: Final = frozenset(
    {"paragraph", "list", "list_item", "quote", "raw_html"}
)
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
    }
)
_BOUNDARY_CHARACTERS: Final = frozenset(
    " \t\r\n,.;:!?\uff0c\u3002\uff1b\uff1a\uff01\uff1f\u3001)]}\uff09\u3011\u300b"
)


class StructureChunkingError(ValueError):
    """Validated structural text cannot be planned within frozen bounds."""


@dataclass(frozen=True, slots=True)
class StructuredTextUnit:
    """One validated source-derived unit in canonical reading order."""

    ordinal: int
    kind: str
    text: str
    heading_path: tuple[str, ...] = ()


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


@dataclass(frozen=True, slots=True)
class StructureChunkPlan:
    """Complete deterministic plan for one validated document."""

    parents: tuple[ParentChunkPlan, ...]
    children: tuple[ChildChunkPlan, ...]


@dataclass(frozen=True, slots=True)
class _Atom:
    unit_ordinal: int
    start_byte: int
    end_byte: int
    token_count: int
    heading_path: tuple[str, ...]


def plan_structure_chunks(
    units: tuple[StructuredTextUnit, ...],
) -> StructureChunkPlan:
    """Plan bounded Parent/Child windows without changing source authority."""
    _validate_units(units)
    atoms = tuple(atom for unit in units for atom in _unit_atoms(unit))
    parents = _plan_parents(atoms)
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
        children.extend(_plan_children(parent.ordinal, parent_atoms, len(children)))
    return StructureChunkPlan(parents=parents, children=tuple(children))


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


def _unit_atoms(unit: StructuredTextUnit) -> tuple[_Atom, ...]:
    token_count = _estimated_tokens(len(unit.text.encode("utf-8")))
    ranges: tuple[tuple[int, int], ...]
    if unit.kind not in _SPLITTABLE_KINDS and token_count <= CHILD_TARGET_MAX_TOKENS:
        ranges = ((0, len(unit.text.encode("utf-8"))),)
    else:
        target = (
            _ATOM_TARGET_TOKENS
            if unit.kind in _SPLITTABLE_KINDS
            else CHILD_TARGET_MAX_TOKENS
        )
        ranges = _split_utf8_ranges(unit.text, target)
    return tuple(
        _Atom(
            unit_ordinal=unit.ordinal,
            start_byte=start,
            end_byte=end,
            token_count=_estimated_tokens(end - start),
            heading_path=unit.heading_path,
        )
        for start, end in ranges
    )


def _split_utf8_ranges(text: str, target_tokens: int) -> tuple[tuple[int, int], ...]:
    offsets = [0]
    boundaries: list[int] = []
    for character in text:
        offsets.append(offsets[-1] + len(character.encode("utf-8")))
        if character in _BOUNDARY_CHARACTERS:
            boundaries.append(offsets[-1])
    total_bytes = offsets[-1]
    target_bytes = target_tokens * _TOKEN_BYTES
    minimum_bytes = max(_TOKEN_BYTES, int(target_bytes * 0.75))
    ranges: list[tuple[int, int]] = []
    start = 0
    while total_bytes - start > target_bytes:
        target_end = start + target_bytes
        boundary_index = bisect.bisect_right(boundaries, target_end) - 1
        boundary = boundaries[boundary_index] if boundary_index >= 0 else -1
        if boundary < start + minimum_bytes:
            offset_index = bisect.bisect_right(offsets, target_end) - 1
            boundary = offsets[offset_index]
        if boundary <= start:
            raise StructureChunkingError("unable to find a UTF-8-safe boundary")
        ranges.append((start, boundary))
        start = boundary
    if start < total_bytes:
        ranges.append((start, total_bytes))
    if not ranges:
        raise StructureChunkingError("structural unit produced no chunk ranges")
    return tuple(ranges)


def _plan_parents(atoms: tuple[_Atom, ...]) -> tuple[ParentChunkPlan, ...]:
    parents: list[ParentChunkPlan] = []
    current: list[_Atom] = []
    current_tokens = 0
    current_heading: tuple[str, ...] | None = None
    for atom in atoms:
        heading_changed = (
            current_heading is not None and atom.heading_path != current_heading
        )
        target_reached = (
            current_tokens >= PARENT_TARGET_MIN_TOKENS
            and current_tokens + atom.token_count > PARENT_TARGET_MAX_TOKENS
        )
        hard_limit = current_tokens + atom.token_count > PARENT_HARD_MAX_TOKENS
        if current and (heading_changed or target_reached or hard_limit):
            parents.append(_parent_plan(len(parents), current_heading or (), current))
            current = []
            current_tokens = 0
            current_heading = None
        current_heading = (
            atom.heading_path if current_heading is None else current_heading
        )
        current.append(atom)
        current_tokens += atom.token_count
    if current:
        parents.append(_parent_plan(len(parents), current_heading or (), current))
    return tuple(parents)


def _parent_plan(
    ordinal: int,
    heading_path: tuple[str, ...],
    atoms: list[_Atom],
) -> ParentChunkPlan:
    token_count = sum(atom.token_count for atom in atoms)
    if token_count > PARENT_HARD_MAX_TOKENS:
        raise StructureChunkingError("parent chunk exceeds the hard token limit")
    return ParentChunkPlan(
        ordinal=ordinal,
        heading_path=heading_path,
        fragments=tuple(_fragment(atom) for atom in atoms),
        token_count=token_count,
    )


def _plan_children(
    parent_ordinal: int,
    atoms: tuple[_Atom, ...],
    first_global_ordinal: int,
) -> tuple[ChildChunkPlan, ...]:
    children: list[ChildChunkPlan] = []
    cursor = 0
    previous_primary: tuple[_Atom, ...] = ()
    while cursor < len(atoms):
        overlap = _overlap_suffix(previous_primary)
        overlap_tokens = sum(atom.token_count for atom in overlap)
        primary: list[_Atom] = []
        total_tokens = overlap_tokens
        while cursor < len(atoms):
            atom = atoms[cursor]
            if primary and total_tokens + atom.token_count > CHILD_TARGET_MAX_TOKENS:
                break
            if not primary and total_tokens + atom.token_count > CHILD_HARD_MAX_TOKENS:
                overlap = ()
                overlap_tokens = 0
                total_tokens = 0
            primary.append(atom)
            cursor += 1
            total_tokens += atom.token_count
            if total_tokens >= CHILD_TARGET_TOKENS:
                break
            if (
                total_tokens >= CHILD_TARGET_MIN_TOKENS
                and cursor < len(atoms)
                and total_tokens + atoms[cursor].token_count > CHILD_TARGET_TOKENS
            ):
                break
        if not primary:
            raise StructureChunkingError("child chunk made no forward progress")
        if total_tokens > CHILD_HARD_MAX_TOKENS:
            raise StructureChunkingError("child chunk exceeds the hard token limit")
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
                token_count=total_tokens,
                overlap_before_tokens=overlap_tokens,
            )
        )
        previous_primary = tuple(primary)
    return tuple(children)


def _overlap_suffix(previous_primary: tuple[_Atom, ...]) -> tuple[_Atom, ...]:
    selected: list[_Atom] = []
    tokens = 0
    for atom in reversed(previous_primary):
        if (
            atom.token_count > OVERLAP_MAX_TOKENS
            or tokens + atom.token_count > OVERLAP_MAX_TOKENS
        ):
            break
        selected.append(atom)
        tokens += atom.token_count
        if tokens >= OVERLAP_TARGET_TOKENS:
            break
    selected.reverse()
    return tuple(selected)


def _fragment(atom: _Atom, *, overlap: bool = False) -> ChunkFragmentPlan:
    return ChunkFragmentPlan(
        unit_ordinal=atom.unit_ordinal,
        start_byte=atom.start_byte,
        end_byte=atom.end_byte,
        token_count=atom.token_count,
        overlap=overlap,
    )


def _estimated_tokens(byte_count: int) -> int:
    return max(1, math.ceil(byte_count / _TOKEN_BYTES))
