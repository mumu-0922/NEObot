# Knowledge Components

Knowledge components manage local knowledge collections, document status, RAG source selection, and retrieved context display.

## Files

- `KnowledgeBase.tsx` renders collection management, file upload, parsing/indexing status, and knowledge-file actions.
- `KnowledgeSelectionModal.tsx` lets users choose knowledge sources for a conversation.
- `RAGBlock.tsx` renders retrieved RAG snippets and source metadata.

## Guidelines

- Keep file and vector helpers in `src/lib/utils` or `src/lib/knowledge`.
- Keep server Knowledge/RAG calls behind the typed Go API client; the legacy
  local `ragService.ts` path is fail-closed.
- Preserve status labels and error states for long-running document workflows.
- Avoid loading large file contents directly in presentational components.
- Render a Knowledge citation only when its exact marker is present in the
  completed answer; retrieval admission alone is not display authority.
