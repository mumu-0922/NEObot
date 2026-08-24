## Bug Analysis: Conversation navigation kept the shared composer locked

### 1. Root Cause Category

- **Category**: B/D/E - Cross-layer contract, test coverage gap, and implicit assumption
- **Specific Cause**: `MessageInput` correctly treated the `onSend` Promise as
  an admission lock, but Server mode returned that Promise only after the full
  Assistant SSE Run. The Pi-style Run change scoped generation state by
  Conversation without also shortening the shared composer's Promise lifetime.
  Separately, the Agent's ordinary eight-round budget had no reserved path for
  verification after a successful last-round mutation.

### 2. Why Fixes Failed

1. The earlier concurrency rollout moved AbortControllers, request IDs, and
   status maps to Conversation scope but did not trace `MessageInput`'s private
   `isSubmittingRef` across navigation.
2. Existing concurrency tests covered Store streams and sidebar state, not the
   UI admission Promise that wraps them.
3. Existing verification tests used `MaxRounds=8` with mutations early enough
   to finish, so a mutation exactly on the limit was absent.

### 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Resolve Server `onSend` at durable user-message acceptance; continue Assistant Run in a handled Conversation-owned task | DONE |
| P0 | Runtime safety | Latch four verification-only rounds and filter the Tool registry during grace | DONE |
| P0 | Test coverage | Pin acceptance-before-stream-completion, last-round verification success, and fail-closed grace exhaustion | DONE |
| P1 | Documentation | Record the admission-Promise and verification-grace contracts in frontend/backend specs and the product contract | DONE |
| P1 | Error truth | Localize durable `AGENT_VERIFICATION_REQUIRED` instead of falling back to `Server generation failed` | DONE |

### 4. Systematic Expansion

- **Similar Issues**: Regenerate/edit flows also await full generation, but
  they own modal/message actions rather than the reusable composer admission
  lock; any future shared input must use acceptance lifetime explicitly.
- **Design Improvement**: Keep three lifetimes distinct: user-message
  admission, Conversation Run, and browser SSE delivery.
- **Process Improvement**: Every concurrency review must trace component-local
  refs and Promises in addition to Store and Backend state.

### 5. Knowledge Capture

- [x] Updated `.trellis/spec/frontend/state-management.md`.
- [x] Updated `.trellis/spec/backend/chat-tool-loop.md`.
- [x] Updated `mm-chat/docs/contracts/chat-tool-loop.md`.
- [x] Added regression tests at the UI composition and Tool-loop boundaries.
- [x] Confirmed this repository has no `src/templates/markdown/spec/` mirror to sync.
