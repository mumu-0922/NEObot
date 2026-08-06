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
  -> exact Provider/model through one versioned adapter
       -> ChatAdapter: SSE content deltas
       -> BufferedChatAdapter: one bounded JSON completion
  -> non-reasoning response, temperature=0, thinking disabled, max 128 tokens
  -> content only, maximum 1024 bytes
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
| Provider-owned buffered completion | OpenAI-compatible JSON wire handling must not leak into the Memory package or alter ordinary chat streaming. | Schema v17 uses `stream:false`, `Accept: application/json`, and the Provider's 2 MiB envelope cap while historical streaming remains unchanged. |
| Strict single-choice completion | A partial, ambiguous, or multi-choice envelope is not equivalent to one completed Judge answer. | Missing content, multiple choices, malformed/oversized JSON, or `finish_reason != "stop"` fails closed before ordinal decoding. |
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
| Buffered body read is interrupted | Return retryable `PROVIDER_TRANSPORT_FAILED` without exposing the body or raw error. |
| Buffered envelope is malformed, oversized, incomplete, or ambiguous | Return deterministic `PROVIDER_RESPONSE_INVALID`. |
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
- Candidate-blind `search_memory` Tool routing failed and is now historical.
  The schema-v10 Development successor reuses this adapter with the exact
  configured GPT or DeepSeek model after current-authorized candidate recall.
  No live schema-v10 result or production installation exists yet.
- The consumed schema-v17 buffered Development run passed all unchanged quality
  and safety gates with zero terminal Judge failures. Nine typed transport
  attempts recovered through the unchanged retry policy, so the evidence
  supports the bounded JSON lane but does not prove the upstream transport is
  failure-free or authorize production activation.

## Change history

- **2026-07-29**: Added the strict shared-prompt chat adapter for isolated
  owner-authorized cloud candidate-judge Development capture.
- **2026-07-31**: Versioned the same transport as
  `chat-configured-candidate-judge-v1` for the candidate-first configured-model
  Development lane without changing the shared prompt or decoder.
- **2026-08-06**: Added `chat-configured-candidate-judge-buffered-v1`, reusing
  the exact prompt, decoder, model controls, provenance, and bounded error
  contract through the Provider-owned non-streaming JSON completion seam.
