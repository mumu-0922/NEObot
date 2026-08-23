## Bug Analysis: Global model projection overwrote Conversation intent

### 1. Root Cause Category

- **Category**: B/E - Cross-Layer Contract and Implicit Assumption
- **Specific Cause**: The refresh fix persisted the complete selected model in
  a browser preference, then treated `chatStore.selectedModel` as the owner for
  existing Conversations. The backend and Local Session already stored a
  Conversation model, but the selection and update paths did not project it
  into or persist it from the composer.

### 2. Why Fixes Failed

1. **Refresh-only fix**: made one global value survive reload but did not trace
   Conversation API → Session mapper → selection → composer in both directions.
2. **Provider alias gap**: backend `openai_compatible` did not exactly match the
   frontend `SERVER_DEFAULT` catalog identity after reload.

### 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Keep Conversation model authoritative and global model projection-only | DONE |
| P0 | Test coverage | Cover Local/Server A → B → A isolation, refresh source, ordered writes, and rejection | DONE |
| P1 | Documentation | Add executable state-management contract and cross-layer checklist | DONE |
| P1 | Runtime | Preserve old Conversation model and show bounded error on PATCH failure | DONE |

### 4. Systematic Expansion

- **Similar Issues**: Chat/Agent mode, reasoning, Knowledge selection, and Agent
  permission controls also combine Conversation ownership with an active UI
  projection; their save and restore paths must remain symmetric.
- **Design Improvement**: use explicit entity actions such as
  `updateServerSessionModel`; never give a generic runtime setter hidden durable
  side effects.
- **Process Improvement**: every persisted selector regression must test two
  entities and a reload/read mapper, not only one in-memory store value.

### 5. Knowledge Capture

- [x] Updated `.trellis/spec/frontend/state-management.md`.
- [x] Updated `.trellis/spec/guides/cross-layer-thinking-guide.md`.
- [x] Added Local/Server store, DTO alias, ordering, failure, and ChatApp
      composition regression coverage.
- [x] Template sync is not applicable: this repository has no
      `src/templates/markdown/spec/` tree.
