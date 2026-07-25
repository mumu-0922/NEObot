#!/usr/bin/env python3
"""Generate the deterministic Candidate 8 supplemental no-answer suite."""

from __future__ import annotations

import argparse
import hashlib
import json
from collections import Counter
from pathlib import Path
from typing import Any

SUITE_SCHEMA = "neo-chat.rag-supplemental-no-answer.v1"
HOLDOUT_SEAL_SCHEMA = "neo-chat.rag-promotion-holdout-seal.v1"
IMPORT_SCHEMA = "neo-chat.rag-evaluation-source-import.v1"
EXPECTED_PROFILE = {
    "profileId": "siliconflow_bge_m3_v1",
    "providerId": "siliconflow",
    "embeddingModelId": "Pro/BAAI/bge-m3",
    "embeddingDimensions": 1024,
    "rerankModelId": "Pro/BAAI/bge-reranker-v2-m3",
}
FORMATS = ("pdf", "docx", "pptx", "xlsx", "json_code")
LANGUAGES = ("chinese", "english")
LANGUAGE_CODES = {"chinese": "zh", "english": "en"}
EXTENSIONS = {
    "pdf": "pdf",
    "docx": "docx",
    "pptx": "pptx",
    "xlsx": "xlsx",
    "json_code": "md",
}
DEFAULT_CREATED_AT = "2026-07-25T11:00:00Z"


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


def _sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _required_text(value: object, field: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{field} must be non-empty text")
    return value


def _build_cases(imported_names: set[str]) -> list[dict[str, Any]]:
    cases: list[dict[str, Any]] = []
    for format_name in FORMATS:
        filename_lane = "code" if format_name == "json_code" else format_name
        token_lane = "CODE" if format_name == "json_code" else format_name.upper()
        for ordinal in range(1, 11):
            language = "chinese" if ordinal <= 5 else "english"
            language_code = LANGUAGE_CODES[language]
            filename = (
                f"no-answer-{filename_lane}-{language_code}-{ordinal:02d}."
                f"{EXTENSIONS[format_name]}"
            )
            token = f"QZ-NOANSWER-{token_lane}-{language_code.upper()}-{ordinal:02d}"
            if filename.casefold() in imported_names:
                raise ValueError(f"absent source collides with import receipt: {filename}")
            if language == "chinese":
                query = f"根据 {filename}，{token} 的核定值是多少？"
            else:
                query = f"According to {filename}, what is the approved value for {token}?"
            cases.append(
                {
                    "id": (
                        f"supplemental-no-answer-{filename_lane}-"
                        f"{language_code}-{ordinal:02d}"
                    ),
                    "query": query,
                    "language": language,
                    "format": format_name,
                    "expectedNoAnswer": True,
                    "absentSourceName": filename,
                    "absentSubjectToken": token,
                }
            )
    return cases


def _validate_cases(cases: list[dict[str, Any]]) -> None:
    if len(cases) != 50:
        raise ValueError("supplemental suite must contain exactly 50 cases")
    language_counts = Counter(str(item["language"]) for item in cases)
    format_counts = Counter(str(item["format"]) for item in cases)
    if language_counts != Counter({"chinese": 25, "english": 25}):
        raise ValueError(f"invalid language coverage: {dict(language_counts)}")
    if format_counts != Counter(dict.fromkeys(FORMATS, 10)):
        raise ValueError(f"invalid format coverage: {dict(format_counts)}")
    for field in ("id", "query", "absentSourceName", "absentSubjectToken"):
        values = [str(item[field]) for item in cases]
        if len(values) != len(set(values)):
            raise ValueError(f"supplemental case field {field} is not unique")


def _build_suite(
    seal: dict[str, Any],
    import_receipt: dict[str, Any],
    created_at: str,
) -> dict[str, Any]:
    if seal.get("schemaVersion") != HOLDOUT_SEAL_SCHEMA:
        raise ValueError("unexpected Holdout seal schema")
    if seal.get("retrievalProfile") != EXPECTED_PROFILE:
        raise ValueError("Holdout seal is not bound to the pinned SiliconFlow BGE profile")
    if import_receipt.get("schemaVersion") != IMPORT_SCHEMA:
        raise ValueError("unexpected source import receipt schema")
    documents = import_receipt.get("documents")
    if not isinstance(documents, list) or len(documents) != 50:
        raise ValueError("source import receipt must contain exactly 50 documents")
    imported_names = {
        _required_text(item.get("filename"), "documents[].filename").casefold()
        for item in documents
        if isinstance(item, dict)
    }
    if len(imported_names) != 50:
        raise ValueError("source import receipt filenames must be unique")
    cases = _build_cases(imported_names)
    _validate_cases(cases)
    return {
        "schemaVersion": SUITE_SCHEMA,
        "id": "candidate8-supplemental-no-answer-2026-07-25",
        "description": (
            "Synthetic 50-case no-answer regression bound to Candidate 8; "
            "never Promotion evidence and never an Activation authority."
        ),
        "synthetic": True,
        "promotionEvidence": False,
        "createdAt": created_at,
        "binding": {
            "goldenSetId": _required_text(seal.get("goldenSetId"), "goldenSetId"),
            "goldenRawSha256": _required_text(
                seal.get("goldenRawSha256"), "goldenRawSha256"
            ),
            "goldenContentSha256": _required_text(
                seal.get("goldenContentSha256"), "goldenContentSha256"
            ),
            "curationRawSha256": _required_text(
                seal.get("curationRawSha256"), "curationRawSha256"
            ),
            "humanReviewRawSha256": _required_text(
                seal.get("humanReviewRawSha256"), "humanReviewRawSha256"
            ),
            "sourceImportRawSha256": _required_text(
                seal.get("sourceImportRawSha256"), "sourceImportRawSha256"
            ),
            "collectionId": _required_text(
                import_receipt.get("collectionId"), "collectionId"
            ),
            "candidateGenerationId": _required_text(
                seal.get("candidateGenerationId"), "candidateGenerationId"
            ),
            "artifactManifestHash": _required_text(
                seal.get("artifactManifestHash"), "artifactManifestHash"
            ),
            "chunkProfileHash": _required_text(
                seal.get("chunkProfileHash"), "chunkProfileHash"
            ),
            "retrievalProfileId": EXPECTED_PROFILE["profileId"],
            "answerModelId": _required_text(seal.get("answerModelId"), "answerModelId"),
            "generationHeadRevision": seal.get("generationHeadRevision"),
            "corpusProjectionRevision": seal.get("corpusProjectionRevision"),
        },
        "criteria": {
            "maximumFalseAnswerRate": 0.02,
            "maximumP95LatencyMilliseconds": 1000,
            "maximumAverageContextTokens": 4096,
            "requireZeroCitationEvidence": True,
            "requireZeroCitationMarkers": True,
            "requireZeroAuthorityLeakage": True,
            "requireAbsentSourceAndSubject": True,
        },
        "cases": cases,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--holdout-seal", required=True, type=Path)
    parser.add_argument("--import-receipt", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--created-at", default=DEFAULT_CREATED_AT)
    args = parser.parse_args()

    seal = _load_object(args.holdout_seal)
    import_receipt = _load_object(args.import_receipt)
    if _sha256(args.import_receipt) != seal.get("sourceImportRawSha256"):
        raise ValueError("source import receipt SHA-256 differs from the Holdout seal")
    suite = _build_suite(
        seal,
        import_receipt,
        _required_text(args.created_at, "createdAt"),
    )
    body = (json.dumps(suite, ensure_ascii=False, indent=2) + "\n").encode()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("xb") as output:
        output.write(body)
    print(f"wrote {len(suite['cases'])} cases to {args.output}")
    print(f"SHA256 {hashlib.sha256(body).hexdigest()}")


if __name__ == "__main__":
    main()
