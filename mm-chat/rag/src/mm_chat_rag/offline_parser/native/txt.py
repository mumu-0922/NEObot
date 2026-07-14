"""C1.3 deterministic TXT Native Parser."""

from __future__ import annotations

from mm_chat_rag.offline_parser.errors import ParserFormat
from mm_chat_rag.offline_parser.native.decoding import (
    DecodedSource,
    source_unit_from_decoded,
)
from mm_chat_rag.offline_parser.native.model import (
    NativeDocument,
    NativeFragment,
    NativeNode,
    NativeNodeKind,
    NativeSourceUnitKind,
    NativeTransformKind,
)


def parse_txt(decoded: DecodedSource) -> NativeDocument:
    """Preserve decoded TXT exactly; C1.4 owns LF/NFC canonicalization."""
    source_unit = source_unit_from_decoded(
        decoded,
        kind=NativeSourceUnitKind.RAW_FILE,
        canonical_uri=None,
    )
    nodes = [
        NativeNode(
            ordinal=0,
            kind=NativeNodeKind.DOCUMENT,
            parent_ordinal=None,
            source_position=decoded.document_position(),
        )
    ]
    if decoded.text:
        position = decoded.position(0, len(decoded.text))
        nodes.append(
            NativeNode(
                ordinal=1,
                kind=NativeNodeKind.PARAGRAPH,
                parent_ordinal=0,
                source_position=position,
                fragments=(
                    NativeFragment(
                        ordinal=0,
                        text=decoded.text,
                        transform=NativeTransformKind.IDENTITY,
                        source_position=position,
                    ),
                ),
            )
        )
    return NativeDocument(
        source_format=ParserFormat.TXT,
        source_bytes=len(decoded.source),
        source_sha256=source_unit.source_sha256,
        source_units=(source_unit,),
        nodes=tuple(nodes),
    )
