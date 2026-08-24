# Bug Analysis: whole-Agent timeout killed progressing work

## 1. Root Cause Category

- **Category**: B / D / E — Cross-layer contract, test coverage gap, and
  implicit assumption.
- **Specific Cause**: local/MCP `RunTimeout` values owned Tool or compatibility
  runtime safety, but Handler and MCP orchestration promoted them to the parent
  Provider-loop Context. The code implicitly treated elapsed wall time as a
  completion signal. Separate Step/call/round counters repeated the same
  assumption at other layers.

## 2. Why Fixes Failed

1. Terminal schema and timeout fixes aligned foreground arguments but did not
   trace the parent Handler Context.
2. Provider/search failure fixes improved individual Tool recovery but left the
   whole-Run Deadline intact.
3. Existing unit tests covered each budget independently; none deliberately
   crossed the old boundary and then required `publish_file`, so a generated
   file could exist while the user-visible task still failed.

## 3. Prevention Mechanisms

| Priority | Mechanism | Specific Action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Effective Agent uses cancellation-scoped parent Context; Tool and Job timeouts stay with their owner. | DONE |
| P0 | Runtime | Three identical outcomes or five all-error rounds produce a typed blocked wrap-up. | DONE |
| P0 | Integration test | Wait past a one-second legacy boundary, then publish and reload the attachment. | DONE |
| P1 | Contract | Record completion-driven semantics, outcome metadata, and compatibility-budget scope in backend specs. | DONE |
| P1 | Review checklist | Require lifecycle ownership and post-boundary finalization proof for timeout changes. | DONE |

## 4. Systematic Expansion

- **Similar issues**: approval TTL, Provider HTTP idle timeout, background Job
  lifetime, reverse-proxy timeout, and MCP connector timeout can all be
  accidentally promoted to a parent workflow deadline.
- **Design improvement**: completion is an evidence/state decision; timeouts
  belong to leaf operations, while repetition/no-progress belongs to the
  orchestrator.
- **Process improvement**: timeout reviews must trace every Context parent and
  include one integration test whose decisive side effect occurs after the old
  cutoff.

## 5. Knowledge Capture

- [x] `.trellis/spec/backend/chat-tool-loop.md`
- [x] `.trellis/spec/backend/agent-runtime.md`
- [x] `.trellis/spec/backend/mcp-tools.md`
- [x] `.trellis/spec/guides/cross-layer-thinking-guide.md`
- [x] Product contracts and deployment docs
- [x] Project has no `src/templates/markdown/spec/` mirror to synchronize.
