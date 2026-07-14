"""Offline parser contract resources.

The package exposes versioned JSON Schema bytes without importing test-only
validation libraries or activating any RAG runtime handler.
"""

from mm_chat_rag.contracts.resources import read_schema_bytes, schema_names

__all__ = ["read_schema_bytes", "schema_names"]
