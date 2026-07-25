#!/usr/bin/env python3
"""Build the deterministic draft-only promotion curation queue."""

from __future__ import annotations

import argparse
import hashlib
import json
import uuid
from collections import Counter
from pathlib import Path
from typing import Any

SOURCE_SCHEMA = "neo-chat.rag-evaluation-source-corpus.v1"
IMPORT_SCHEMA = "neo-chat.rag-evaluation-source-import.v1"
PROMOTION_SCHEMA = "neo-chat.rag-promotion-golden.v1"
CURATION_SCHEMA = "neo-chat.rag-promotion-curation-queue.v1"
SOURCE_COUNT = 50
FACT_COUNT = 10
SHA256_HEX_LENGTH = 64
DEVELOPMENT_FACTS_PER_SOURCE = 6
VALIDATION_FACTS_PER_SOURCE = 2
MINIMUM_SLICE_CASES = 50
EXPECTED_CASES = SOURCE_COUNT * FACT_COUNT
EXPECTED_SPLITS = Counter({"development": 300, "validation": 100, "holdout": 100})
CRITICAL_SLICES = (
    "pdf",
    "text_markdown_docx",
    "pptx",
    "xlsx_table",
    "json_code",
    "chinese",
    "english",
    "short_fact",
    "cross_section",
    "exact_numeric",
)
LANES = ("pdf", "docx", "pptx", "xlsx", "code")
FACT_ANCHORS = tuple(f"F{ordinal:02d}" for ordinal in range(1, FACT_COUNT + 1))
TABLE_EXACT_ANCHORS = frozenset({"F03", "F04", "F08", "F09", "F10"})
EXPECTED_FACT_SLICES = {
    "F01": ("short_fact",),
    "F02": ("short_fact",),
    "F03": ("exact_numeric",),
    "F04": ("exact_numeric",),
    "F05": ("short_fact",),
    "F06": ("short_fact",),
    "F07": ("short_fact",),
    "F08": ("cross_section", "exact_numeric"),
    "F09": ("exact_numeric",),
    "F10": ("exact_numeric",),
}
FORMAT_SLICES = {
    "pdf": ("pdf",),
    "docx": ("text_markdown_docx",),
    "pptx": ("pptx",),
    "xlsx": ("xlsx_table",),
    "code": ("text_markdown_docx", "json_code"),
}
LANGUAGE_SLICES = {"zh": "chinese", "en": "english"}
QUERY_TEMPLATES = {
    "zh": {
        "F01": "编号 {source_id} 对应的项目名称是什么？",  # noqa: RUF001
        "F02": "编号 {source_id} 的文档列出的责任团队是什么？",  # noqa: RUF001
        "F03": "编号 {source_id} 的核定容量是多少？请保留单位。",  # noqa: RUF001
        "F04": "编号 {source_id} 的触发阈值是多少？请给出精确百分比。",  # noqa: RUF001
        "F05": "编号 {source_id} 的生效日期是哪一天？",  # noqa: RUF001
        "F06": "编号 {source_id} 的执行地点在哪里？",  # noqa: RUF001
        "F07": "编号 {source_id} 的例外代码是什么？",  # noqa: RUF001
        "F08": (
            "综合编号 {source_id} 的指标与交叉引用，容量和触发阈值分别是多少？"  # noqa: RUF001
        ),
        "F09": "编号 {source_id} 规定的升级时限是多少分钟？",  # noqa: RUF001
        "F10": "编号 {source_id} 规定记录保留多少天？",  # noqa: RUF001
    },
    "en": {
        "F01": "What project name is associated with source {source_id}?",
        "F02": "Which owning team is listed for source {source_id}?",
        "F03": "What is the approved capacity for source {source_id}? Keep the unit.",
        "F04": "What exact trigger threshold is specified for source {source_id}?",
        "F05": "What is the effective date for source {source_id}?",
        "F06": "Which operating location is listed for source {source_id}?",
        "F07": "What exception code is assigned to source {source_id}?",
        "F08": (
            "Across the metrics and cross-reference for source {source_id}, "
            "what are the capacity and trigger threshold?"
        ),
        "F09": "How many minutes is the escalation SLA for source {source_id}?",
        "F10": "How many days must source {source_id} records be retained?",
    },
}


def _sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _reject_duplicate_pairs(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON key {key!r}")
        result[key] = value
    return result


def _load_object(path: Path) -> dict[str, Any]:
    value = json.loads(
        path.read_text(encoding="utf-8"),
        object_pairs_hook=_reject_duplicate_pairs,
    )
    if not isinstance(value, dict):
        raise TypeError(f"{path} must contain one JSON object")
    return value


def _uuid(value: object, field: str) -> str:
    if not isinstance(value, str):
        raise TypeError(f"{field} must be a UUID")
    try:
        return str(uuid.UUID(value))
    except ValueError as error:
        raise ValueError(f"{field} must be a UUID") from error


def _text(value: object, field: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{field} must be non-empty text")
    return value


def _objects(value: object, field: str) -> list[dict[str, Any]]:
    if not isinstance(value, list) or not all(isinstance(item, dict) for item in value):
        raise ValueError(f"{field} must be an object array")
    return value


def _validate_document_binding(
    document: dict[str, Any],
    imported_item: dict[str, Any],
    source_id: str,
    content_hashes: set[str],
) -> tuple[str, str, str]:
    lane = _text(document.get("formatLane"), f"{source_id}.formatLane")
    language = _text(document.get("language"), f"{source_id}.language")
    digest = _text(document.get("sha256"), f"{source_id}.sha256")
    if lane not in LANES or language not in LANGUAGE_SLICES:
        raise ValueError(f"unsupported source lane or language for {source_id}")
    if (
        len(digest) != SHA256_HEX_LENGTH
        or any(character not in "0123456789abcdef" for character in digest)
        or digest in content_hashes
    ):
        raise ValueError(f"invalid or duplicate content hash for {source_id}")
    if document.get("synthetic") is not True or document.get("reviewState") != "draft":
        raise ValueError(f"source {source_id} is not synthetic draft-only")
    facts = _objects(document.get("facts"), f"{source_id}.facts")
    anchors = tuple(_text(fact.get("anchor"), "fact.anchor") for fact in facts)
    if len(facts) != FACT_COUNT or set(anchors) != set(FACT_ANCHORS):
        raise ValueError(f"source {source_id} must have F01..F10 exactly once")
    for fact in facts:
        anchor = str(fact["anchor"])
        _text(fact.get("label"), f"{source_id}.{anchor}.label")
        _text(fact.get("answer"), f"{source_id}.{anchor}.answer")
        _text(fact.get("section"), f"{source_id}.{anchor}.section")
        slices = fact.get("slices")
        if (
            not isinstance(slices, list)
            or tuple(slices) != EXPECTED_FACT_SLICES[anchor]
        ):
            raise ValueError(f"source {source_id} fact {anchor} slices are invalid")
    for field in ("filename", "sha256", "mimeType"):
        if imported_item.get(field) != document.get(field):
            raise ValueError(f"source {source_id} receipt {field} mismatch")
    if (
        imported_item.get("finalStatus") != "active"
        or imported_item.get("versionStatus") != "active"
    ):
        raise ValueError(f"source {source_id} is not active in the receipt")
    _uuid(imported_item.get("fileId"), f"{source_id}.fileId")
    _uuid(imported_item.get("documentId"), f"{source_id}.documentId")
    return lane, language, digest


def _validate_sources(
    manifest: dict[str, Any],
    receipt: dict[str, Any],
    manifest_sha256: str,
) -> tuple[list[dict[str, Any]], dict[str, dict[str, Any]], str]:
    if (
        manifest.get("schemaVersion") != SOURCE_SCHEMA
        or manifest.get("synthetic") is not True
        or manifest.get("promotionEligible") is not False
    ):
        raise ValueError("source manifest header is invalid")
    if (
        receipt.get("schemaVersion") != IMPORT_SCHEMA
        or receipt.get("manifestSha256") != manifest_sha256
    ):
        raise ValueError("import receipt is not bound to the source manifest")
    collection_id = _uuid(receipt.get("collectionId"), "receipt.collectionId")
    documents = _objects(manifest.get("documents"), "manifest.documents")
    imported = _objects(receipt.get("documents"), "receipt.documents")
    if len(documents) != SOURCE_COUNT or len(imported) != SOURCE_COUNT:
        raise ValueError("exactly 50 source and imported documents are required")
    imported_by_source: dict[str, dict[str, Any]] = {}
    for item in imported:
        source_id = _text(item.get("sourceId"), "receipt.sourceId")
        if source_id in imported_by_source:
            raise ValueError(f"duplicate imported source {source_id}")
        imported_by_source[source_id] = item

    source_ids: set[str] = set()
    content_hashes: set[str] = set()
    lane_counts: Counter[str] = Counter()
    language_counts: Counter[str] = Counter()
    for document in documents:
        source_id = _text(document.get("sourceId"), "manifest.sourceId")
        if source_id in source_ids:
            raise ValueError(f"duplicate source {source_id}")
        source_ids.add(source_id)
        imported_item = imported_by_source.get(source_id)
        if imported_item is None:
            raise ValueError(f"source {source_id} is missing from the import receipt")
        lane, language, digest = _validate_document_binding(
            document,
            imported_item,
            source_id,
            content_hashes,
        )
        content_hashes.add(digest)
        lane_counts[lane] += 1
        language_counts[language] += 1
    if set(imported_by_source) != source_ids:
        raise ValueError("import receipt source membership differs from the manifest")
    if lane_counts != Counter(dict.fromkeys(LANES, SOURCE_COUNT // len(LANES))):
        raise ValueError(f"source lane distribution is invalid: {dict(lane_counts)}")
    if language_counts != Counter({"zh": 25, "en": 25}):
        raise ValueError("source language distribution is invalid")
    return documents, imported_by_source, collection_id


def _split(document_ordinal: int, fact_ordinal: int) -> str:
    rotated = (fact_ordinal - document_ordinal % FACT_COUNT) % FACT_COUNT
    if rotated < DEVELOPMENT_FACTS_PER_SOURCE:
        return "development"
    if rotated < DEVELOPMENT_FACTS_PER_SOURCE + VALIDATION_FACTS_PER_SOURCE:
        return "validation"
    return "holdout"


def _case_id(source_id: str, anchor: str) -> str:
    return f"{source_id}-{anchor}".lower()


def _write_json(path: Path, value: object) -> str:
    body = (json.dumps(value, ensure_ascii=False, indent=2) + "\n").encode()
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_bytes(body)
    temporary.replace(path)
    return hashlib.sha256(body).hexdigest()


def build_queue(
    manifest_path: Path,
    receipt_path: Path,
    promotion_path: Path,
    curation_path: Path,
) -> dict[str, Any]:
    """Generate both the closed draft and its source-rich review queue."""
    manifest_sha256 = _sha256(manifest_path)
    receipt_sha256 = _sha256(receipt_path)
    manifest = _load_object(manifest_path)
    receipt = _load_object(receipt_path)
    documents, imported_by_source, collection_id = _validate_sources(
        manifest,
        receipt,
        manifest_sha256,
    )
    collection_alias = f"rag-eval-synthetic-{manifest_sha256[:12]}"
    promotion_cases: list[dict[str, Any]] = []
    curation_cases: list[dict[str, Any]] = []
    for document_ordinal, document in enumerate(documents):
        source_id = str(document["sourceId"])
        lane = str(document["formatLane"])
        language = str(document["language"])
        imported = imported_by_source[source_id]
        facts = sorted(
            _objects(document["facts"], f"{source_id}.facts"),
            key=lambda item: str(item["anchor"]),
        )
        for fact_ordinal, fact in enumerate(facts):
            anchor = str(fact["anchor"])
            fact_slices = [str(value) for value in fact.get("slices", [])]
            slices = list(
                dict.fromkeys(
                    (
                        *FORMAT_SLICES[lane],
                        LANGUAGE_SLICES[language],
                        *fact_slices,
                    )
                )
            )
            case = {
                "id": _case_id(source_id, anchor),
                "query": QUERY_TEMPLATES[language][anchor].format(source_id=source_id),
                "split": _split(document_ordinal, fact_ordinal),
                "slices": slices,
                "selectedCollectionAliases": [collection_alias],
                "expectedRelevantEvidenceIds": [f"{source_id}:{anchor}"],
                "expectedNoAnswer": False,
                "tableExactAnswerRequired": (
                    lane == "xlsx" and anchor in TABLE_EXACT_ANCHORS
                ),
                "review": {"state": "draft"},
            }
            promotion_cases.append(case)
            curation_cases.append(
                {
                    "promotionCase": case,
                    "sourceBinding": {
                        "sourceId": source_id,
                        "anchor": anchor,
                        "section": fact["section"],
                        "filename": document["filename"],
                        "sourceSha256": document["sha256"],
                        "fileId": imported["fileId"],
                        "documentId": imported["documentId"],
                        "formatLane": lane,
                        "language": language,
                    },
                    "expectedLabel": fact["label"],
                    "expectedAnswer": fact["answer"],
                }
            )

    split_counts = Counter(item["split"] for item in promotion_cases)
    slice_counts = Counter(name for item in promotion_cases for name in item["slices"])
    table_exact_count = sum(
        bool(item["tableExactAnswerRequired"]) for item in promotion_cases
    )
    if (
        len(promotion_cases) != EXPECTED_CASES
        or split_counts != EXPECTED_SPLITS
        or any(slice_counts[name] < MINIMUM_SLICE_CASES for name in CRITICAL_SLICES)
        or table_exact_count != MINIMUM_SLICE_CASES
        or any(item["review"] != {"state": "draft"} for item in promotion_cases)
    ):
        raise ValueError("generated queue violates the frozen draft distribution")

    corpus_id = f"rag-promotion-synthetic-draft-{manifest_sha256[:16]}"
    promotion = {
        "schemaVersion": PROMOTION_SCHEMA,
        "id": corpus_id,
        "description": (
            "DRAFT ONLY synthetic curation queue bound to source manifest "
            f"SHA-256 {manifest_sha256}; not human-reviewed promotion evidence."
        ),
        "lifecycle": {"state": "draft"},
        "criteria": {
            "maximumP95LatencyMilliseconds": 1000,
            "maximumAverageContextTokens": 4096,
            "minimumAggregateQualityImprovement": 0.005,
        },
        "cases": promotion_cases,
    }
    promotion_sha256 = _write_json(promotion_path, promotion)
    curation = {
        "schemaVersion": CURATION_SCHEMA,
        "synthetic": True,
        "reviewState": "draft",
        "promotionEligible": False,
        "sourceManifest": {
            "schemaVersion": SOURCE_SCHEMA,
            "sha256": manifest_sha256,
        },
        "importReceiptSha256": receipt_sha256,
        "collectionBinding": {
            "alias": collection_alias,
            "collectionId": collection_id,
        },
        "promotionGoldenDraft": {
            "schemaVersion": PROMOTION_SCHEMA,
            "id": corpus_id,
            "sha256": promotion_sha256,
        },
        "counts": {
            "cases": len(curation_cases),
            "development": split_counts["development"],
            "validation": split_counts["validation"],
            "holdout": split_counts["holdout"],
            "tableExactAnswerRequired": table_exact_count,
            "slices": dict(sorted(slice_counts.items())),
        },
        "cases": curation_cases,
    }
    curation_sha256 = _write_json(curation_path, curation)
    return {
        "sourceManifestSha256": manifest_sha256,
        "importReceiptSha256": receipt_sha256,
        "promotionGoldenDraftSha256": promotion_sha256,
        "curationQueueSha256": curation_sha256,
        "cases": len(promotion_cases),
        "splits": dict(split_counts),
        "tableExactAnswerRequired": table_exact_count,
        "promotionEligible": False,
    }


def main() -> None:
    """Generate the deterministic draft artifacts from bound source inputs."""
    parser = argparse.ArgumentParser()
    parser.add_argument("source_manifest", type=Path)
    parser.add_argument("import_receipt", type=Path)
    parser.add_argument("promotion_output", type=Path)
    parser.add_argument("curation_output", type=Path)
    args = parser.parse_args()
    if args.promotion_output.resolve() == args.curation_output.resolve():
        parser.error("promotion and curation outputs must differ")
    result = build_queue(
        args.source_manifest.resolve(),
        args.import_receipt.resolve(),
        args.promotion_output.resolve(),
        args.curation_output.resolve(),
    )
    print(json.dumps(result, ensure_ascii=False, sort_keys=True))  # noqa: T201


if __name__ == "__main__":
    main()
