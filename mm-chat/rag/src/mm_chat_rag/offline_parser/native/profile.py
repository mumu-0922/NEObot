"""Hash-bound C1.3A implementation and dependency manifest."""

from __future__ import annotations

import hashlib
from pathlib import Path
from typing import Final

from mm_chat_rag.offline_parser.canonical import JsonObject, JsonValue

_MODULE_DIRECTORY: Final = Path(__file__).parent
_MARKDOWN_IT_WHEEL_SHA256: Final = (
    "9f7ebbcd14fe59494226453aed97c1070d83f8d24b6fc3a3bcf9a38092641c4a"
)
_MDURL_WHEEL_SHA256: Final = (
    "84008a41e51615a49fc9966191ff91509e3c40b939176e643fd50a5c2196b8f8"
)


def native_parser_profile_manifest() -> JsonObject:
    """Return the closed implementation profile included in Config Hash."""
    components: list[JsonValue] = [
        _component("txt", "txt.py"),
        _component("markdown", "markdown.py"),
        _component("html", "html.py"),
    ]
    return {
        "artifactSchemaVersion": "parser-native-artifact.v1",
        "components": components,
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
        "schemaVersion": "native-parser-profile.internal.v1",
        "supportedFormats": ["txt", "markdown", "html"],
    }


def _component(parser_format: str, module_name: str) -> JsonObject:
    common = ("model.py", "decoding.py", module_name)
    digest = hashlib.sha256()
    digest.update(b"mm-chat.native-parser-component.v1\n")
    for name in common:
        content = _MODULE_DIRECTORY.joinpath(name).read_bytes()
        digest.update(name.encode("ascii") + b"\x00")
        digest.update(len(content).to_bytes(8, "big") + content)
    return {
        "format": parser_format,
        "implementationArtifactSha256": digest.hexdigest(),
        "implementationId": f"parser.{parser_format}",
        "implementationVersion": "0.1.0",
    }
