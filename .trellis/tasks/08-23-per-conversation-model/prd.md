# Persist Model Selection per Conversation

## Goal

Make the composer model selection Conversation-owned: changing the model in
one Conversation must not change the saved model of other Conversations, and
returning to or refreshing a Conversation must restore its own last model.

## Requirements

- Persist the complete `providerId:modelId` on the active Conversation when
  the user changes the composer model.
- In Server mode, reuse the existing `PATCH /v1/chat/conversations/{id}`
  `modelRef` contract and PostgreSQL-backed Conversation fields.
- In Local mode, update the existing `Session.model` metadata persisted by
  `chatStore`.
- Selecting a Conversation must restore that Conversation's model before the
  next message is sent.
- Keep the browser-level selected model preference only as the default for a
  newly created Conversation or a legacy Conversation without a model.
- Normalize the Server-default Provider alias so a stored backend
  `openai_compatible` model maps back to the frontend `SERVER_DEFAULT`
  Provider selection.
- Serialize rapid Server model changes so the durable final model follows the
  user's selection order.
- If a saved Conversation model is no longer available, use the existing safe
  fallback for the current UI without silently overwriting the Conversation's
  stored model.
- Surface a bounded UI error when a Server model change cannot be saved.

## Acceptance Criteria

- [x] Conversation A can retain model A while Conversation B retains model B.
- [x] Switching A → B → A restores model A each time in both Local and Server
      mode.
- [x] Refreshing while Conversation A is active restores model A.
- [x] Selecting a model in one Conversation does not mutate any other
      Conversation record.
- [x] A new Conversation inherits the last explicitly selected browser model.
- [x] Rapid Server selections are persisted in order and the final selection
      wins.
- [x] A failed Server save leaves the previously persisted Conversation model
      authoritative and shows an error.
- [x] Focused tests and the complete frontend quality gate pass.

## Definition of Done

- Store/service regression tests cover Local and Server model updates,
  Conversation switching, alias normalization, and failure isolation.
- Frontend format, lint, typecheck, full Vitest, and production build pass.
- A new frontend image is deployed and reports healthy without restarting
  backend, RAG, database, or runtime data services.

## Technical Approach

- Add explicit `updateSessionModel` and `updateServerSessionModel` actions to
  `chatStore`; do not make the generic live `setModel` action mutate durable
  Conversations.
- Restore `selectedModel` inside Local/Server session selection actions and
  resolve the active Conversation model first in ChatApp's post-bootstrap
  availability check.
- Let `chatStore` serialize same-Conversation Server model writes. ChatApp
  updates the global browser default only after the latest successful save and
  reuses the existing action-error surface.
- Map the backend Server-default Provider alias only for Conversation model
  restoration while preserving existing message model projections.

## Decision (ADR-lite)

**Context**: The previous fix made the selected model survive refresh but kept
one global live value, so all Conversations appeared to share it even though
the Conversation API/schema already owns `modelRef`.

**Decision**: Conversation records are authoritative for existing chats;
browser preference is only the seed for new/legacy chats.

**Consequences**: Existing backend schema and API are reused. Model selection
becomes isolated per Conversation, while unavailable models still degrade to a
safe UI fallback without destroying the stored intent.

## Out of Scope

- Per-message model locking or changing historical Assistant message models.
- New backend migrations or Provider configuration changes.
- Synchronizing an unsaved empty composer across multiple browser tabs.

## Technical Notes

- `Session.model`, `ConversationDTO.modelRef`, and the Conversation PATCH
  `modelRef` body already exist.
- `chatStore.setModel` currently changes only one global live projection.
- `ChatApp.handleModelSelect` currently writes only the global browser
  preference.
- The existing Chat/Agent mode contract already establishes the desired
  pattern: persist intent per Conversation and keep a browser default for new
  Conversations.
