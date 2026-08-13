# G19 Process Trace Reuse Boundary
## Evidence inspected

- `mm-chat/backend/internal/chat/process_trace.go`
- `mm-chat/backend/internal/chat/process_trace_runtime.go`
- `mm-chat/backend/internal/chat/process_trace_test.go`
- `.trellis/spec/backend/chat-tool-loop.md`

## Finding

G19 provides a useful presentation rule, not an Agent control-plane authority.
It keeps stable step identifiers, bounded durations and allowlisted detail while
redacting bearer/API-key/assignment-shaped secrets and dropping raw payload
keys. Its projection is message metadata and may be reconciled for display, so
it must not be imported as the durable Run/Step/Attempt state machine.

## Decision for G20.2

- Reuse the same **sanitized-fact principle**: durable Agent events contain
  stable IDs, state, generation, bounded actor/reason codes and low-cardinality
  JSON facts only.
- Do not persist prompt/Skill bodies, chain-of-thought, Tool arguments/results,
  stdout/stderr, Workspace content, Artifact bytes, Secret handles or values.
- Implement an independent `agentorchestrator` state machine and PostgreSQL
  event ledger. A later UI may derive a G19-shaped process view from it, but
  Chat metadata never drives Agent authorization or recovery.
