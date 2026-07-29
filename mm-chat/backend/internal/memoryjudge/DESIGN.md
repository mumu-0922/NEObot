# Memory candidate-judge adapter design

## Goals

- Reuse the chat Provider abstraction without copying Provider-specific wire
  formats into `usermemory`.
- Keep prompt construction and strict result decoding under shared
  `usermemory` authority.
- Bound every output and fail closed on malformed, late, or ambiguous Provider
  behavior.

## Non-goals

- Provider discovery, live authorization, credential access, or fallback.
- Ownership/scope/revision or Provider-egress authorization.
- Production reader activation, Validation selection, or promotion.
- Main-model `search_memory` Tool routing; that is owned by
  `internal/memoryroute`.

## Data flow

```text
already reauthorized, secret-redacted query + candidate ordinals/content
  -> usermemory shared strict prompt
  -> ChatAdapter bound to exact Provider/model
  -> non-reasoning stream, temperature=0, thinking disabled, max 128 tokens
  -> content deltas only, maximum 1024 bytes
  -> usermemory exact JSON/ordinal decoder
  -> raw validated bytes + model/prompt provenance
  -> usermemory intersects selected ordinals with BGE order
```

Memory IDs, revisions, scopes, retrieval scores, credentials, and database
authority are not present in the judge input. Query and candidate bodies remain
request-local and are never logged or persisted by this adapter.

## Key decisions

| Decision | Reason | Consequence |
| --- | --- | --- |
| Prompt/decoder live in `usermemory` | Capture and runtime adapters must not drift. | The adapter only transports and verifies the shared contract. |
| Exact Provider/model at construction | Model substitution would invalidate Development evidence. | Missing Provider ID/model fails before a request. |
| Bounded content-delta accumulator | Free-form streaming output can be oversized or mix event types. | More than 1024 bytes or an unknown event rejects the whole response. |
| Reasoning is not contract output | Hidden reasoning cannot be parsed as ordinal authority. | Reasoning deltas and Usage are ignored; only content is decoded. |
| Post-stream context check | A Provider may return apparent success after cancellation. | Late output is discarded and reported as failure. |

## Validation and error matrix

| Condition | Result |
| --- | --- |
| Provider, Provider ID, or model ID missing | Adapter construction fails. |
| Shared prompt input is invalid | Reject before Provider work. |
| Provider start/event reports an error | Return a bounded Provider failure. |
| Content exceeds 1024 bytes | Cancel and reject the entire response. |
| Event is neither content, reasoning, nor Usage | Reject the entire response. |
| Context is cancelled or expired by stream completion | Reject as late output. |
| JSON has missing/extra/duplicate keys, trailing data, invalid schema, or invalid ordinals | Shared strict decoder rejects it. |
| Valid empty `selectedOrdinals` | Valid `no_memory`; no candidate is selected. |

## Security considerations

- Candidate content is treated as untrusted data by the fixed server prompt.
- The model returns request-local ordinals, never Memory IDs or authority.
- Raw Provider errors are replaced by bounded messages.
- No request, response, query, candidate body, score, or credential is logged or
  persisted here.
- A valid ordinal result is relevance evidence only; `usermemory` still applies
  BGE ordering and post-Provider current-authority checks.

## Known limitations

- Three live hosted-judge Development profiles failed unchanged quality and/or
  latency gates. These failures are retained historical evidence, not
  production authorization.
- Server composition deliberately installs no candidate judge or frozen cloud
  policy.
- Current active research moved to the main-model `search_memory` Tool route;
  this package remains to replay and explain historical schema-v4/v5 evidence.

## Change history

- **2026-07-29**: Added the strict shared-prompt chat adapter for isolated
  owner-authorized cloud candidate-judge Development capture.
