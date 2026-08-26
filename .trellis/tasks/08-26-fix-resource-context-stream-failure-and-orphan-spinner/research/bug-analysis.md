# Bug Analysis: Resource context event left orphan streaming messages

## 1. Root Cause Category

- **Category**: B — Cross-Layer Contract, plus C — Change Propagation Failure.
- **Specific cause**: the Handler emitted `context.injected.source=resource-orchestrator`,
  while the durable event validator accepts only `system-prompt`, `skill-catalog`,
  `skill-instruction`, and `runtime-context`. The resulting append error marked the
  Agent Turn failed, but the pre-SSE return path skipped Assistant Message finalization,
  so the UI continued to treat the durable `streaming` Message as active.

## 2. Why Earlier Coverage Failed

1. Event normalizer tests covered the fixed vocabulary but no real Handler test exercised
   all three context injections with Resource orchestration enabled.
2. Failure-path tests asserted HTTP/Turn failure without a fault-injected repository that
   also asserted the Assistant Message terminal state.
3. The PostgreSQL drill was pinned to migration head 101; after head advanced to 106 it
   exited before reaching the relevant persistence proof.

## 3. Prevention Mechanisms

| Priority | Mechanism | Specific action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Reuse the declared `runtime-context` source | Done |
| P0 | Failure invariant | Centralize pre-SSE context failure finalization for Message and Turn | Done |
| P0 | Integration test | Exercise the real Handler with Resource context enabled | Done |
| P0 | Fault injection | Fail `context.injected` persistence and assert both terminal states | Done |
| P1 | Verification | Derive the PostgreSQL drill migration head dynamically | Done |
| P1 | Runtime repair | Back up and repair only the exact failed-Turn/streaming-Message cohort | Done |

## 4. Systematic Expansion

- **Similar issues**: every early return after Assistant Message creation can orphan durable
  state if it updates only local Turn variables.
- **Design improvement**: event-source values are protocol vocabulary, not display labels;
  new labels should map to an existing source unless the validator and contracts change.
- **Process improvement**: full Handler tests must accompany new prelude context producers;
  unit tests of the normalizer alone cannot prove end-to-end terminal consistency.

## 5. Knowledge Capture

- [x] Updated `backend/chat-tool-loop.md` with source and terminal-state invariants.
- [x] Updated `backend/resource-orchestration.md` with the AIHero discovery-only boundary.
- [x] Updated the PostgreSQL drill to follow the embedded migration head.
- [x] Added Handler, fault-injection, and supported-link regression tests.
