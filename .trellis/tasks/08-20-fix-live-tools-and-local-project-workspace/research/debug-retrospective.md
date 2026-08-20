# Debug retrospective: live Tool parser drift hidden by reload

## 1. Root Cause Category

- **Category**: B/D — Cross-Layer Contract plus Test Coverage Gap.
- **Specific cause**: Backend live `local_direct` events intentionally omitted
  Provider `callId`, retained `executionId`, and used `classification=write`
  for File mutations. Frontend treated `callId` as mandatory and rejected
  `local_direct + write` even though the Backend contract and execution were
  valid.

## 2. Why Earlier Acceptance Failed

1. API/runtime inspection proved Tools executed but did not exercise the
   browser's live SSE normalizer.
2. Reload rendered the durable event projection after detached Backend work
   completed, masking the broken pre-reload stream path.
3. The Frontend wire type trusted a normalized DTO too early, so compile-time
   typing could not expose runtime projection drift.

## 3. Prevention Mechanisms

| Priority | Mechanism | Specific action | Status |
| --- | --- | --- | --- |
| P0 | Runtime boundary | Keep `ServerStreamEvent.toolCall` as `unknown` and normalize once | DONE |
| P0 | Regression | Cover absent `callId`, local `write`, malformed identity, and mode/classification mismatch | DONE |
| P0 | E2E acceptance | Require a complete live stream proof before any reload | DONE in spec; pending rollout |
| P1 | Documentation | Record separate live-versus-durable verification in the cross-layer guide | DONE |

## 4. Systematic Expansion

- **Similar issues**: any SSE/WebSocket path whose durable history uses a
  different redacted projection can pass reload-only tests while live UI fails.
- **Design improvement**: treat all wire events as untrusted; normalize at one
  boundary and make optional-field fallbacks distinguish absence from malformed
  presence.
- **Process improvement**: deployed acceptance must capture the stream before
  reload and verify replay independently.

## 5. Knowledge Capture

- [x] Updated Frontend MCP Tool event contract.
- [x] Updated Backend/Operations workspace alias contracts.
- [x] Updated the shared cross-layer thinking guide.
- [x] Added focused Frontend and Backend regressions.
