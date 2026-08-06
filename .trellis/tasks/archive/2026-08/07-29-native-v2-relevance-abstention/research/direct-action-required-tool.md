# Direct-action required Tool framing

Date: 2026-08-05

## Live failure

The screenshot turn `我喜欢喝生椰拿铁 -> 记住` reached the direct-action
lane. PostgreSQL and assistant metadata agreed on:

```text
status=failed
result_code=PLANNER_OUTPUT_INVALID
Activity failed=1
```

This rules out the lexical gate and SQL apply path as the first failing stage.
The historical implementation also collapsed Provider stream errors and strict
JSON failures into that one code, so transport and output validation require
separate classification.

## Protocol probe

Using the active stored Provider/vault path and a synthetic non-secret input:

```text
currentUserMessage=记住
referencedPreviousUserMessage=我喜欢喝生椰拿铁
```

- The configured `gpt-5.6-sol` returned valid free-form JSON on a later call;
  this did not make the unframed production path deterministic.
- A named required Tool returned complete semantic arguments on both
  `gpt-5.6-sol` and `deepseek-v4-flash`.
- DeepSeek omitted `schemaVersion` even when it was a required strict Tool
  property, while all action/content/scope/confidence/target fields were valid.
- Removing model control over `schemaVersion` and binding v1 through the Tool
  name produced valid calls from both Providers.

No credentials, Provider response bodies containing unrelated user data, or
raw database secrets were retained.

## Selected boundary

Use exactly one required `propose_memory_action_v1` Tool Call. The Tool
arguments contain only semantic proposal fields; Go derives the canonical
schema version from the versioned Tool name, then performs unchanged exact-key,
range, target, scope, sensitivity, and SQL authority checks.

Fail closed:

- no/multiple/wrong Tool, ordinary text, malformed/extra arguments, or semantic
  drift -> `PLANNER_OUTPUT_INVALID`;
- Provider construction, missing Tool-round support, transport, timeout, or
  event failure -> `PLANNER_PROVIDER_FAILED`.

There is no plain-chat fallback and no parser relaxation.

## Bug Analysis: referential write reached Planner but failed unpredictably

### 1. Root Cause Category

- **Category**: E — Implicit Assumption, with a B — Cross-Layer Contract
  component.
- **Specific cause**: The direct-action layer assumed a free-form chat stream
  would honor an exact JSON prompt consistently across Provider/model
  protocols. The code then collapsed stream/transport failures and strict
  decode failures into the same `PLANNER_OUTPUT_INVALID` result.

### 2. Why earlier fixes did not close the issue

1. Adding referential intent and schema-v2 input repaired the missing factual
   source but did not change the Planner output transport.
2. Prior unit fakes emitted perfect JSON deltas, so they proved parser behavior
   rather than real Provider framing.
3. The first deployed `记住` replay correctly exposed the next downstream
   boundary; treating it as another regex issue would have repeated the
   surface-fix loop.

### 3. Prevention mechanisms

| Priority | Mechanism | Specific action | Status |
| --- | --- | --- | --- |
| P0 | Architecture | Required versioned Tool Call; server-owned schema version | DONE |
| P0 | Test coverage | Reject text, zero/multiple/wrong calls, extra fields, and Provider failure misclassification | DONE |
| P0 | Runtime proof | Inspect assistant metadata/action row, then replay write to canonical Memory | DONE |
| P1 | Documentation | Record Tool framing and error taxonomy in Memory direct-action spec | DONE |

### 4. Systematic expansion

- **Similar issues**: Any auxiliary task that asks a Provider for strict JSON
  over ordinary chat streaming can have the same protocol ambiguity.
- **Design improvement**: Schema identity should be owned by a versioned server
  contract (Tool name or endpoint), never trusted as a model-generated field.
- **Process improvement**: A direct-action test is incomplete until it proves
  the durable result code and canonical mutation from a fresh source message;
  replaying the same source only proves SQL idempotency.

### 5. Knowledge capture

- [x] Updated `.trellis/spec/backend/memory-v2-actions-activity-usage.md`.
- [x] Updated `mm-chat/docs/architecture/memory-v2-foundation.md`.
- [x] Added Provider-shaped Tool and failure-classification regressions.
- [x] Retained this live/runtime analysis without credentials or unrelated user
  content.
