## Bug Analysis: Global generation state prevented Pi-style concurrent Conversations

### 1. Root Cause Category

- **Category**: E + B — Implicit Assumption and Cross-Layer Contract
- **Specific Cause**: the frontend treated generation, AbortController, and
  request identity as one application-global resource, while the backend keyed
  cancellation by Run ID but did not enforce or project the intended
  Conversation scheduling boundary. Workspace grouping, selected Conversation,
  and active Run ownership were therefore conflated.

### 2. Why Fixes Failed

1. Persisting the selected model fixed Conversation configuration but did not
   change Run ownership; switching still invalidated the global stream token.
2. Keeping Backend execution detached preserved durable completion, but the UI
   discarded background deltas and had no active projection after refresh.
3. Adding polling without a single-flight guard allowed timer, online, and
   visibility triggers to overlap and temporarily restore stale Run state.

### 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Key controllers, request IDs, server generations, and unread state by Conversation | DONE |
| P0 | Runtime | Backend atomically reserves one active Run per `(user, Conversation)` | DONE |
| P0 | Contract | Project `activeGeneration` from the Conversation list for refresh reconciliation | DONE |
| P0 | Tests | Run two deferred Conversations, reject a same-Conversation duplicate, and cancel exactly one | DONE |
| P1 | Documentation | Capture executable frontend/backend contracts and cross-layer checklist | DONE |

### 4. Systematic Expansion

- **Similar Issues**: image generation, regeneration, edited branches, Stop,
  Delete, reconnect, and refresh all shared the same global-state risk.
- **Design Improvement**: model Workspace as context/grouping, Conversation as
  Session/Run owner, and Run as the exact cancellable execution.
- **Process Improvement**: every long-running feature review must state
  per-entity concurrency and navigation semantics before implementation.

### 5. Knowledge Capture

- [x] Updated frontend and backend code-specs with signatures, errors, cases,
      and required tests.
- [x] Updated the cross-layer thinking guide with entity-scoped work checks.
- [x] Updated the product Chat Tool Loop contract.
- [x] Confirmed this repository has no `src/templates/markdown/spec/` mirror to
      synchronize; `.trellis/spec/` is the project-local authority.
