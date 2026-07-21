# Knowledge Components

Knowledge components manage server collections, document status, per-conversation selection, and citation display.

## Files

- `KnowledgeBase.tsx` exposes the server-backed Knowledge surface.
- `ServerKnowledgeBase.tsx` renders collection management, upload, indexing status, and document actions.
- `KnowledgeSelectionModal.tsx` binds server collections to the current conversation.
- `AddToKnowledgeModal.tsx` uploads generated text into a server collection.
- `RAGBlock.tsx` renders retrieved RAG snippets and source metadata.

## Guidelines

- Keep server Knowledge/RAG calls behind the typed Go API client.
- Do not reintroduce browser parsing, vector indexing, retrieval, or workspace-level Knowledge bindings.
- Preserve status labels and error states for long-running document workflows.
- Avoid loading large file contents directly in presentational components.
- Render a Knowledge citation only when its exact marker is present in the
  completed answer; retrieval admission alone is not display authority.
