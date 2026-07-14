"""Hash-bound C1.3B implementation, dependency, and safety manifest."""

from __future__ import annotations

import hashlib
from pathlib import Path
from typing import Final

from mm_chat_rag.offline_parser.canonical import JsonObject, JsonValue

_OFFLINE_PARSER_DIRECTORY: Final = Path(__file__).parent.parent
_COMMON_SOURCE_FILES: Final = (
    "canonical.py",
    "config.py",
    "errors.py",
    "router.py",
    "native/model.py",
    "native/decoding.py",
    "native/dispatch.py",
    "native/internal_result.py",
    "native/profile.py",
)
_OOXML_SOURCE_FILES: Final = (
    "native/opc.py",
    "native/xml_source.py",
)
_MARKDOWN_IT_WHEEL_SHA256: Final = (
    "9f7ebbcd14fe59494226453aed97c1070d83f8d24b6fc3a3bcf9a38092641c4a"
)
_MDURL_WHEEL_SHA256: Final = (
    "84008a41e51615a49fc9966191ff91509e3c40b939176e643fd50a5c2196b8f8"
)


def native_parser_profile_manifest() -> JsonObject:
    """Return the closed implementation profile included in Config Hash."""
    components: list[JsonValue] = [
        _component("txt", "native/txt.py"),
        _component("markdown", "native/markdown.py"),
        _component("html", "native/html.py"),
        _component("docx", "native/docx.py", ooxml=True),
        _component("pptx", "native/pptx.py", ooxml=True),
        _component("xlsx", "native/xlsx.py", ooxml=True),
        _component("csv", "native/csv.py"),
    ]
    return {
        "artifactSchemaVersion": "parser-native-artifact.v2",
        "components": components,
        "csvDialect": {
            "delimiter": ",",
            "doublequote": True,
            "headerInference": False,
            "quotechar": '"',
            "recordTerminators": ["CRLF", "LF", "CR"],
            "snifferAllowed": False,
        },
        "dependencies": [
            {
                "license": "MIT",
                "name": "markdown-it-py",
                "version": "4.2.0",
                "wheelSha256": _MARKDOWN_IT_WHEEL_SHA256,
            },
            {
                "license": "MIT",
                "name": "mdurl",
                "version": "0.1.2",
                "wheelSha256": _MDURL_WHEEL_SHA256,
            },
        ],
        "markdownOptions": {
            "breaks": False,
            "html": True,
            "linkify": False,
            "plugins": ["table"],
            "preset": "commonmark",
            "runtimePluginDiscovery": False,
            "typographer": False,
        },
        "ooxmlPolicy": {
            "archiveRecursionAllowed": False,
            "compressionMethods": ["stored", "deflated"],
            "externalRelationshipDereferenceAllowed": False,
            "formulaEvaluationAllowed": False,
            "singleAdmissionCapability": True,
            "zip64Allowed": False,
        },
        "schemaVersion": "native-parser-profile.internal.v2",
        "supportedFormats": [
            "txt",
            "markdown",
            "html",
            "docx",
            "pptx",
            "xlsx",
            "csv",
        ],
        "xmlPolicy": {
            "dtdAllowed": False,
            "encodingPrecedence": ["utf-8-bom", "strict-utf-8"],
            "entitiesAllowed": False,
            "externalEntityResolutionAllowed": False,
            "processingInstructionsAllowed": False,
            "xincludeAllowed": False,
        },
    }


def _component(
    parser_format: str,
    module_name: str,
    *,
    ooxml: bool = False,
) -> JsonObject:
    source_files = (
        *_COMMON_SOURCE_FILES,
        *(_OOXML_SOURCE_FILES if ooxml else ()),
        module_name,
    )
    digest = hashlib.sha256()
    digest.update(b"mm-chat.native-parser-component.v2\n")
    for name in source_files:
        content = _OFFLINE_PARSER_DIRECTORY.joinpath(name).read_bytes()
        digest.update(name.encode("utf-8") + b"\x00")
        digest.update(len(content).to_bytes(8, "big") + content)
    return {
        "format": parser_format,
        "implementationArtifactSha256": digest.hexdigest(),
        "implementationId": f"parser.{parser_format}",
        "implementationVersion": "0.2.0",
        "sourceFiles": list(source_files),
    }
