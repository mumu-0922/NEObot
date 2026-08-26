# Bug Analysis: Provider 502 misattributed to Local Skill Tool Loop

## 1. Root Cause Category

- **Category**: B/D/E — Cross-layer contract, test coverage gap, implicit assumption
- **Specific Cause**: The native Tool loop used enabled runtime presence as
  failure attribution after a Provider startup error. It did not first preserve
  the existing typed `ProviderFailureCategory`. Explicit Resource-link install
  also implicitly assumed a healthy model would always call `resource_search`.

## 2. Why Earlier Behavior Failed

1. Runtime wrapper: converted every first-round error into the first enabled
   MCP/Local Skill wrapper, even when no Tool call existed.
2. Model-only orchestration: the Backend already owned complete explicit-link
   search authority but still delegated the decision to an unavailable model.
3. Isolated tests: Provider taxonomy and Resource Tools were tested separately;
   no regression combined typed Provider failure with an enabled Agent runtime.

## 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Preserve typed Provider failure before runtime wrappers | DONE |
| P0 | Test coverage | Combine transient Provider startup failures with Resource runtime | DONE |
| P0 | Security | Exact one-link parser plus existing admitted Store authority | DONE |
| P1 | Documentation | Update Chat Tool Loop and Resource Orchestration contracts | DONE |
| P1 | Review guide | Add failure-attribution/runtime-presence checklist | DONE |

## 4. Systematic Expansion

- **Similar issues**: MCP and retrieval first-round wrappers can mask typed
  Provider failures unless category precedence remains explicit.
- **Design improvement**: Deterministic human commands with complete bounded
  authority should bypass model routing but must reuse the domain service.
- **Process improvement**: Every Tool-loop change needs a combined fixture with
  runtime enabled, zero Tool calls, and a typed Provider startup error.

## 5. Knowledge Capture

- [x] Updated `.trellis/spec/backend/chat-tool-loop.md`.
- [x] Updated `.trellis/spec/backend/resource-orchestration.md`.
- [x] Updated `.trellis/spec/guides/cross-layer-thinking-guide.md`.
- [x] Added focused parser, orchestration, retry, and public-error tests.
