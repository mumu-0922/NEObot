"""Deterministic structure-aware Parent/Child chunk planner tests."""

from __future__ import annotations

from itertools import pairwise

import pytest

from mm_chat_rag.structure_chunking import (
    CHILD_HARD_MAX_TOKENS,
    CHILD_TARGET_MAX_TOKENS,
    CHILD_TARGET_MIN_TOKENS,
    OVERLAP_MAX_TOKENS,
    PARENT_HARD_MAX_TOKENS,
    PARENT_TARGET_MIN_TOKENS,
    ChildChunkPlan,
    StructureChunkingError,
    StructuredTextUnit,
    plan_structure_chunks,
)


def _paragraph(seed: str, words: int = 720) -> str:
    return " ".join(f"{seed}{index}" for index in range(words))


def test_planner_builds_section_bounded_parent_child_windows() -> None:
    units = (
        StructuredTextUnit(0, "heading", "Recommendation", ("Recommendation",)),
        StructuredTextUnit(1, "paragraph", _paragraph("alpha"), ("Recommendation",)),
        StructuredTextUnit(2, "paragraph", _paragraph("beta"), ("Recommendation",)),
        StructuredTextUnit(3, "heading", "Evaluation", ("Evaluation",)),
        StructuredTextUnit(4, "list_item", _paragraph("gamma"), ("Evaluation",)),
        StructuredTextUnit(5, "paragraph", _paragraph("delta"), ("Evaluation",)),
    )

    plan = plan_structure_chunks(units)

    assert len(plan.parents) >= 4
    assert len(plan.children) > len(plan.parents)
    for parent in plan.parents:
        assert parent.token_count <= PARENT_HARD_MAX_TOKENS
        assert {
            units[fragment.unit_ordinal].heading_path for fragment in parent.fragments
        } == {parent.heading_path}
    assert any(
        parent.token_count >= PARENT_TARGET_MIN_TOKENS for parent in plan.parents
    )

    children_by_parent: dict[int, list[ChildChunkPlan]] = {}
    for child in plan.children:
        children_by_parent.setdefault(child.parent_ordinal, []).append(child)
        assert child.token_count <= CHILD_HARD_MAX_TOKENS
        parent_fragments = plan.parents[child.parent_ordinal].fragments
        assert all(
            any(
                parent_fragment.unit_ordinal == fragment.unit_ordinal
                and parent_fragment.start_byte <= fragment.start_byte
                and fragment.end_byte <= parent_fragment.end_byte
                for parent_fragment in parent_fragments
            )
            for fragment in child.fragments
        )
        sibling_count = sum(
            item.parent_ordinal == child.parent_ordinal for item in plan.children
        )
        if child.ordinal_in_parent < sibling_count - 1:
            assert child.token_count >= CHILD_TARGET_MIN_TOKENS
        assert child.overlap_before_tokens <= OVERLAP_MAX_TOKENS
    assert any(child.overlap_before_tokens > 0 for child in plan.children)

    for siblings in children_by_parent.values():
        for previous, current in pairwise(siblings):
            overlap = [fragment for fragment in current.fragments if fragment.overlap]
            assert overlap
            previous_ranges = {
                (fragment.unit_ordinal, fragment.start_byte, fragment.end_byte)
                for fragment in previous.fragments
            }
            assert all(
                (fragment.unit_ordinal, fragment.start_byte, fragment.end_byte)
                in previous_ranges
                for fragment in overlap
            )


def test_planner_preserves_atomic_table_row_and_utf8_boundaries() -> None:
    table_row = "| 用户 | 偏好 | 序列决策 |" * 20
    multilingual = "用户偏好建模与序列决策。" * 240
    units = (
        StructuredTextUnit(0, "heading", "实验", ("实验",)),
        StructuredTextUnit(1, "table_row", table_row, ("实验",)),
        StructuredTextUnit(2, "paragraph", multilingual, ("实验",)),
    )

    plan = plan_structure_chunks(units)

    table_fragments = [
        fragment
        for parent in plan.parents
        for fragment in parent.fragments
        if fragment.unit_ordinal == 1
    ]
    assert len(table_fragments) == 1
    assert (table_fragments[0].start_byte, table_fragments[0].end_byte) == (
        0,
        len(table_row.encode()),
    )
    for parent in plan.parents:
        for fragment in parent.fragments:
            encoded = units[fragment.unit_ordinal].text.encode()
            encoded[fragment.start_byte : fragment.end_byte].decode("utf-8")
    assert plan == plan_structure_chunks(units)


def test_planner_keeps_typical_children_within_target_maximum() -> None:
    units = tuple(
        StructuredTextUnit(index, "paragraph", _paragraph(f"p{index}", 240), ())
        for index in range(12)
    )

    plan = plan_structure_chunks(units)

    sibling_counts = {
        parent.ordinal: sum(
            child.parent_ordinal == parent.ordinal for child in plan.children
        )
        for parent in plan.parents
    }
    non_tail = [
        child
        for child in plan.children
        if child.ordinal_in_parent < sibling_counts[child.parent_ordinal] - 1
    ]
    assert non_tail
    assert all(
        CHILD_TARGET_MIN_TOKENS <= child.token_count <= CHILD_TARGET_MAX_TOKENS
        for child in non_tail
    )


@pytest.mark.parametrize(
    "units",
    [
        (),
        (StructuredTextUnit(1, "paragraph", "text"),),
        (StructuredTextUnit(0, "unknown", "text"),),
        (StructuredTextUnit(0, "paragraph", "   "),),
        (StructuredTextUnit(0, "paragraph", "text", ("",)),),
        (StructuredTextUnit(False, "paragraph", "text"),),
        (StructuredTextUnit(0, "paragraph", "text", ("\ud800",)),),
    ],
)
def test_planner_rejects_invalid_structural_units(
    units: tuple[StructuredTextUnit, ...],
) -> None:
    with pytest.raises(StructureChunkingError):
        plan_structure_chunks(units)
