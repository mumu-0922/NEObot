# Add knowledge document download

## Goal

Allow an authorized user to download the original source file for a document
directly from the server-mode Knowledge Base document list.

## What I already know

- The server Knowledge page currently offers selection, reprocess, and delete,
  but no download action.
- The backend and typed frontend client already implement the authorized binary
  path `GET /v1/knowledge/documents/{documentId}/content` through
  `apiClient.knowledge.downloadDocumentContent(...)`.
- The API returns the current authorized document version as bounded binary
  content with its filename and content type. The missing link is the page UI.
- Local mode deliberately has no Knowledge server adapter and must not pretend
  that download succeeded.

## Requirements

- Download only the current effective original file represented by the current
  document row; do not expose historical-version selection.
- Add an accessible Download action to each server Knowledge document row.
- Reuse the existing authorized Knowledge binary API rather than minting a
  public MinIO URL or exposing an object key.
- Preserve the source filename through the shared filename sanitizer.
- Disable duplicate actions while a download is in flight and surface a clear
  localized failure without corrupting other row state.
- Keep reprocess, delete, selection, and responsive hover/focus behavior intact.

## Acceptance Criteria

- [ ] An authorized server-mode user can download an active Knowledge document
      and receives byte-identical original content under a safe source name.
- [ ] Download is unavailable or fails explicitly when no downloadable current
      version exists.
- [ ] A failed request revokes temporary browser resources and shows an error.
- [ ] Keyboard and screen-reader users can invoke the action.
- [ ] Existing reprocess, delete, bulk selection, and local-mode behavior do not
      regress.

## Definition of Done

- Frontend unit/composition coverage is added or updated.
- Relevant formatter, lint, typecheck, tests, and build pass.
- The existing backend authorization/download contract is verified; backend
  changes are made only if runtime evidence proves the route incomplete.
- Security review confirms no public object URL, path traversal, or cross-user
  download authority is introduced.

## Out of Scope (explicit)

- Historical-version browsing or download.
- Bulk ZIP export.
- Direct MinIO URLs or object-key display.
- Document preview redesign.

## Decision (ADR-lite)

**Context**: The existing server API already resolves and authorizes the
current downloadable document version. Adding history selection would require
a separate version-listing UX and broader authority contract.

**Decision**: Add one per-row action that downloads only the current effective
original file through the existing authenticated binary endpoint.

**Consequences**: The MVP is small, preserves the current authorization
boundary, and solves the immediate usability gap. Historical versions and bulk
exports remain explicit future work.

## Technical Notes

- UI: `mm-chat/frontend/src/components/knowledge/ServerKnowledgeBase.tsx`
- Client: `mm-chat/frontend/src/services/api/client/server/knowledgeApi.ts`
- Types: `mm-chat/frontend/src/services/api/client/types.ts`
- Existing client method:
  `apiClient.knowledge.downloadDocumentContent({ documentId })`
