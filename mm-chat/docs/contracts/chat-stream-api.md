# Phase 5.2/5.4 Chat Stream API Contract

## 1. Purpose

Phase 5.2 adds the first provider-neutral streaming spine. It assumes the user
message already exists, creates a `streaming` assistant row, emits SSE frames,
and finalizes the assistant row as `completed`, `failed`, or `cancelled`.

```text
POST /v1/chat/conversations/{id}/messages -> persisted user message
POST /v1/chat/conversations/{id}/stream   -> streaming assistant message
```

Phase 5.3 adds the first real provider adapter for OpenAI-compatible
`/chat/completions` streaming APIs. Phase 5.4 adds the first durable cancel
endpoint for streaming assistant rows. Phase 7 adds Redis-backed temporary
cancellation flags for cross-process stream interruption. Later phases extend
this same stream with authenticated RAG, Tools, Agent events, Goals, workspace
execution, and bounded context replacement.

## 2. Endpoint

```http
POST /v1/chat/conversations/{id}/stream
Accept: text/event-stream
Content-Type: application/json
```

Cancel endpoint:

```http
POST /v1/chat/runs/{runId}/cancel
Content-Type: application/json
```

Cursor resume endpoint:

```http
GET /v1/chat/runs/{runId}/events?after={lastSequence}
Accept: text/event-stream
Last-Event-ID: {lastSequence}
```

`after` takes precedence over `Last-Event-ID`; either value must be a
non-negative integer. The endpoint is available only to the exact Run user and
reauthorizes the current Conversation before returning data. Unknown,
unauthorized, or expired retained Runs return the same bounded not-found shape.

Stream success response:

```http
HTTP/1.1 200 OK
Content-Type: text/event-stream; charset=utf-8
Cache-Control: no-cache, no-transform
X-Content-Type-Options: nosniff
```

`no-transform` is mandatory. The same-origin Next.js rewrite and any external
reverse proxy must not compress or buffer SSE frames; ordinary response
compression remains enabled outside streaming endpoints.

Cancel success response:

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"runId":"...","status":"cancelled","message":{"id":"...","status":"cancelled"}}
```

## 3. Request Body

```ts
export interface StreamAssistantMessageRequest {
  userMessageId: EntityId;
  modelRef: ModelRef;
  config?: JsonObject;
  systemInstruction?: string;
  systemPrompt?: string; // compatibility alias
  metadata?: JsonObject;
  idempotencyKey: string;
}

export type ReasoningEffort =
  | "auto"
  | "low"
  | "medium"
  | "high"
  | "xhigh"
  | "max";

export type ChatToolMode = "chat" | "agent";
```

Rules:

- `conversationId` is path-only and rejected in the body.
- `userMessageId` is required and must reference an existing `role="user"`
  message in the same conversation.
- `modelRef` is required and both IDs must be non-empty. `modelRef.modelId` is
  sent to the resolved provider; there is no environment model fallback.
- `idempotencyKey` is required and applies to the assistant streaming row only.
- Runtime mode authority is the persisted Conversation `config.toolMode`, not
  the stream request snapshot. Missing/invalid legacy mode means Agent. Stored
  Chat wins over a conflicting request and physically omits MCP, local Skill,
  File, Terminal, Job, and Goal runtimes.
- Agent requires a native `ToolRoundProvider` adapter. On an Auto-capability
  cache miss it waits for the shared bounded probe. Only confirmed
  `unsupported` downgrades the effective run to Chat; transient/inconclusive
  `unknown` preserves the real native Agent round rather than fabricating a
  Chat result. Knowledge, Memory, Web Search, and non-Tool image routing remain
  available in Chat.
- `content`, `attachments`, `role`, `status`, identity hints, and other
  server-managed message fields are rejected.
- `config.useReasoning=false` disables explicit provider reasoning. When true,
  `config.reasoningEffort` selects a semantic level. Missing or invalid legacy
  values normalize to `high`; provider payloads never receive an arbitrary
  browser string.
- `auto` omits OpenAI effort so the model chooses its default. `xhigh` and
  `max` are normalized against the selected model: GPT-5.6 retains `max`,
  known GPT-5.2+ families retain `xhigh`, and unknown compatible models clamp
  unsupported extended levels to `high`.
- Anthropic maps the same semantic level to a bounded `budget_tokens` value and
  always keeps `max_tokens` greater than the thinking budget.
- If the frontend has only text content, it must first call
  `POST /v1/chat/conversations/{id}/messages`, then pass the returned user
  message ID into `/stream`.

## 4. SSE Events

Every sequenced frame uses `id: <sequence>`, a named `event:` line matching
`data.type`, and a single JSON object in `data:`. The JSON `sequence`, SSE
`id`, and reconnect cursor are the same Run-local integer.

Required sequence for a successful mock/provider stream:

```text
message.started
message.delta        # zero or more; mock emits deterministic chunks
usage.updated        # emitted when provider usage is available
message.completed
```

For migration-`096` Agent Turns, `turn.started` is committed after the
assistant Message row is created and before the first SSE frame. Each
`process.step.updated` or `tool.call.updated` projection is committed to the
Chat Agent event log before the same sanitized projection is sent. The SSE
`sequence` remains stream-local and is not interchangeable with the per-Turn
durable event sequence. Terminal Message finalization precedes the deferred
`assistant.message`/`turn.ended` append; startup recovery repairs that explicit
torn-write window without changing an already terminal Message status.

The active Run keeps a process-local ring of at most 1,024 encoded SSE frames
and 4 MiB. Publishing never waits on a reconnect client. Each subscriber has a
bounded queue; a slow subscriber is closed and may reconnect from its last
cursor. At most 16 subscribers exist per Run. A completed stream is retained
for 30 seconds, with at most 64 finished streams retained process-wide, so a
client that loses the terminal frame can still replay it. This ring is delivery
state only: transient chunks are never appended to `chat_agent_events`, and the
final bounded Tool presentation remains the database reload authority.

If `after` predates an evicted or oversized frame, the resume stream first
emits an unsequenced explicit gap:

```text
event: stream.gap
data: {"type":"stream.gap","runId":"...","messageId":"...","after":12,
       "oldestSequence":18,"latestSequence":25,"reason":"cursor_evicted"}
```

Retained frames then continue from `oldestSequence`. The frontend shows a safe
partial-view notice, resets only its delivery cursor, and converges on the
terminal Message snapshot. If the terminal frame is also unavailable, it reads
the persisted Message list after bounded resume attempts. It never fabricates
the evicted content.

Migration `097` adds same-Conversation Goal state. Goal mutations append
`goal.changed`, and every admitted automatic continuation appends
`goal.round.started`, into the same Turn sequence before the next Provider
Step. Their process projection uses `mode=goal` with
`classification=read|write` and contains no objective, blocker text, Tool
arguments, or workspace data.

Context compaction is an internal Provider event rather than a new public SSE
frame. Before a retried/continued Provider Step, the Handler durably appends a
content-free `context.replaced` event containing only reason and before/after,
pruned-result, and replaced-exchange counts. Removed Tool content is never
placed in the event log or SSE.

Process-local background Job Tool projections may contain only
`durability="process_local"` in addition to the normal allowlisted Tool facts.
This value is persisted and replayed so the frontend can warn that Backend
restart cannot recover the Job; command text and Job output remain forbidden.

Terminal events are mutually exclusive:

```text
message.completed
message.error
message.cancelled
```

Example:

```text
id: 1
event: message.started
data: {"type":"message.started","runId":"...","conversationId":"...","messageId":"...","sequence":1,"createdAt":"2026-07-07T10:00:00Z","role":"assistant","modelRef":{"providerId":"mock","modelId":"mock-chat"}}

id: 2
event: message.delta
data: {"type":"message.delta","runId":"...","conversationId":"...","messageId":"...","sequence":2,"createdAt":"2026-07-07T10:00:01Z","delta":"Mock response: "}

event: message.completed
data: {"type":"message.completed","runId":"...","conversationId":"...","messageId":"...","sequence":4,"createdAt":"2026-07-07T10:00:02Z","message":{"id":"...","conversationId":"...","role":"assistant","status":"completed","content":"Mock response: hello"}}
```

## 5. Persistence Contract

Repository flow:

1. Verify the fixed development user owns the conversation.
2. Verify `userMessageId` belongs to the same conversation and has `role='user'`.
3. Insert an assistant message with:
   - `role='assistant'`
   - `status='streaming'`
   - `parent_message_id=userMessageId`
   - `idempotency_key` scoped by conversation
4. Stream provider events.
5. Finalize the assistant row:
   - success -> `status='completed'`, final `content`, `completed_at=now()`
   - provider error -> `status='failed'`; already-emitted content remains a
     truthful partial answer rather than being marked complete
   - explicit Run cancellation -> `status='cancelled'`

Browser request-context cancellation is not Run-cancellation authority. After
request validation, navigation, tab close, and SSE delivery failure detach the
client while generation continues to a durable terminal row.

`message.completed` must include the persisted final `ChatMessageDto`.

Cancel flow:

1. Validate `runId` as a UUID.
2. Find the fixed development user's assistant message where
   `metadata.runId == runId`.
3. If the assistant message is `streaming`, mark it `cancelled`, set
   `completed_at=now()`, and merge cancel metadata.
4. If it is already `cancelled`, merge cancel metadata and return the message
   (idempotent success).
5. If it is `completed` or `failed`, return `409 RUN_NOT_CANCELLABLE`.

The cancel endpoint updates durable state and interrupts in-flight provider
requests inside the same API process. When Redis is configured, the endpoint
also writes a short-lived cancellation flag so another API process can stop the
matching stream. Redis is temporary coordination only; Postgres remains the
source of truth for message/run status.

## 6. Error Contract

Errors before the SSE response begins use the standard JSON envelope.

| HTTP | Code | When |
| --- | --- | --- |
| `400` | `INVALID_JSON` | Request body is malformed JSON. |
| `400` | `INVALID_CONVERSATION_ID` | Path conversation ID is not a UUID. |
| `400` | `INVALID_USER_MESSAGE_ID` | `userMessageId` is missing, invalid, missing, or not a user message in the conversation. |
| `400` | `MODEL_REF_REQUIRED` | `modelRef` is missing. |
| `400` | `UNSUPPORTED_PROVIDER` | `modelRef.providerId` does not match the configured single provider. |
| `400` | `INVALID_RUN_ID` | Cancel path `runId` is not a UUID. |
| `400` | `IDEMPOTENCY_KEY_REQUIRED` | `idempotencyKey` is blank or missing. |
| `400` | `VALIDATION_ERROR` | Unsupported stream fields such as `content` or `attachments`. |
| `400` | `FORBIDDEN_MESSAGE_FIELD` | Server-managed message fields or identity hints are present. |
| `404` | `CONVERSATION_NOT_FOUND` | Conversation is missing or not owned by the fixed dev user. |
| `404` | `RUN_NOT_FOUND` | Cancel target run does not exist for the fixed dev user. |
| `409` | `IDEMPOTENCY_CONFLICT` | Assistant stream key already exists for the conversation. |
| `409` | `RUN_NOT_CANCELLABLE` | Cancel target is already completed or failed. |
| `429` | `RATE_LIMITED` | Redis rate-limit middleware blocked the request before SSE begins. |
| `502` | `PROVIDER_ERROR` | Provider startup fails before SSE begins. |
| `503` | `DATABASE_REQUIRED` | DB runtime wiring is disabled. |
| `503` | `PROVIDER_REQUIRED` | No provider is configured for streaming. |
| `500` | `STREAMING_UNSUPPORTED` | Response writer cannot flush SSE. |

After SSE starts, provider or finalization failures are emitted as
`message.error` frames with scrubbed error details. HTTP `429 RATE_LIMITED` can
only be returned before the SSE response starts.

Goal/verification terminal failures use stable SSE error codes:

- `AGENT_VERIFICATION_REQUIRED`: a successful write/execute was not followed
  by accepted Tool evidence before the available budget ended.
- `AGENT_GOAL_PERSISTENCE_FAILED`: a Goal read/mutation/round append could not
  be durably committed.

Neither error may be replaced with a successful closing narration. A Goal
wrap-up runs with no Tool definitions; hallucinated calls are rejected as
`goal_concluded` and never dispatched.

## 7. Cancel Response

Success response:

```ts
export interface CancelRunResponse {
  runId: EntityId;
  status: "cancelled";
  message: ChatMessageDto;
}
```

Example:

```http
HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
```

```json
{
  "runId": "33333333-3333-4333-8333-333333333333",
  "status": "cancelled",
  "message": {
    "id": "...",
    "conversationId": "...",
    "role": "assistant",
    "status": "cancelled",
    "content": ""
  }
}
```

## 8. Server-Owned Provider Configuration

OpenAI-compatible execution uses the Chat Completions stream shape after Go
resolves an enabled, connection-tested Postgres/vault provider:

```http
POST {stored normalized base URL}/chat/completions
Authorization: Bearer {vault-decrypted API Key}
Content-Type: application/json
Accept: text/event-stream
```

Runtime rules:

- `provider.source="server-default"` resolves `SERVER_DEFAULT`; a stored custom
  provider uses `source="server-stored"` plus its ID.
- Disabled providers return `409 PROVIDER_DISABLED`; providers without a valid
  activation attestation return `409 PROVIDER_ACTIVATION_REQUIRED`.
- Unsupported stored types/configurations return a stable redacted validation
  error; they do not fall back to another provider or process environment.
- Provider API Keys exist only as encrypted Postgres vault envelopes at rest,
  are decrypted transiently in Go, and are never returned to the browser.
- `PROVIDER_TIMEOUT` bounds upstream execution but does not supply provider
  identity, endpoint, model, or credential.
- Non-`2xx` provider startup responses map to pre-SSE `502 PROVIDER_ERROR`.
- Malformed provider SSE frames after streaming begins map to scrubbed
  `message.error` frames.
- Provider streams that end without `data: [DONE]` are treated as failed
  partial streams and map to scrubbed `message.error` frames. Typed stream-read
  and incomplete-stream failures use `PROVIDER_STREAM_INTERRUPTED`; partial
  content is preserved with `status='failed'`, never replayed or represented as
  a complete answer. Upstream error text and response bodies remain redacted.
- HTTP `413` or an allowlisted stable JSON error code may classify a native
  Tool continuation as `PROVIDER_CONTEXT_OVERFLOW`. Free-form upstream message
  text cannot. The Agent loop may shrink the continuation and retry the same
  Provider/model/Step once only when it became smaller; a second overflow or a
  no-op compaction returns the ordinary scrubbed Provider error path.
- With Redis enabled, active streams poll the cancellation flag and emit
  `message.cancelled` when the flag appears. Redis errors are non-authoritative
  and do not overwrite Postgres status.

The adapter reads `data:` SSE frames, emits `message.delta` for
`choices[].delta.content`, emits `usage.updated` when a provider chunk includes
`usage`, and stops on `data: [DONE]`.

## 9. SSE Proxy Transformation Contract

### 9.1 Scope / Trigger

This contract applies whenever a streaming endpoint is served through the
frontend `/mm-api` rewrite or another compression-capable reverse proxy.

### 9.2 Signatures

Successful text and image streams must return:

```http
Content-Type: text/event-stream; charset=utf-8
Cache-Control: no-cache, no-transform
X-Accel-Buffering: no
```

### 9.3 Contracts

- Go flushes after each named SSE event.
- `no-transform` prevents Next or an upstream proxy from applying gzip,
  deflate, or Brotli to SSE.
- A proxied streaming response must not contain `Content-Encoding`.
- Compression for non-SSE pages, JSON, and static assets remains unchanged.

### 9.4 Validation and Error Matrix

| Condition                               | Required result                                                        |
| --------------------------------------- | ---------------------------------------------------------------------- |
| SSE response lacks `no-transform`       | regression test fails                                                  |
| Browser sends `Accept-Encoding`         | response remains unencoded and incremental                             |
| Provider emits a terminal startup error | existing pre-SSE JSON error contract applies                           |
| Proxy cannot preserve streaming         | deployment validation fails; do not simulate terminal text as live SSE |

### 9.5 Good / Base / Bad Cases

- Good: browser-like compressed request receives multiple deltas over time and
  no `Content-Encoding` header.
- Base: direct Go request receives the same ordered deltas.
- Bad: response carries `Content-Encoding: gzip` and all deltas arrive with the
  terminal frame.

### 9.6 Tests Required

- Handler test: every successful stream asserts `text/event-stream` and
  `Cache-Control` containing `no-transform`.
- Proxy integration: request `/mm-api/.../stream` with `Accept-Encoding` and
  assert no response `Content-Encoding` plus more than one delta arrival time.
- Browser smoke: assert assistant text length increases across multiple DOM
  samples before the Stop control disappears.

### 9.7 Wrong vs Correct

Wrong:

```http
Cache-Control: no-cache
Content-Encoding: gzip
```

Correct:

```http
Cache-Control: no-cache, no-transform
```

## 10. Detached Generation Contract

### 10.1 Scope / Trigger

This contract applies to Server-mode text and image generation when the user
switches Conversations, creates another Conversation, selects an assistant
preset, closes the page, or otherwise loses the SSE connection.

### 10.2 Signatures

Backend ownership split:

```go
generationCtx := context.WithoutCancel(r.Context())
streamCtx, cancelRun := context.WithCancel(generationCtx)
delivery := newBestEffortStreamWriter(w)
```

Frontend cancellation authority remains:

```text
explicit Stop -> AbortController.abort()
  -> POST /v1/chat/runs/{runId}/cancel
  -> durable CancelRun + activeRuns.cancel(runId)
```

### 10.3 Contracts

- `context.WithoutCancel` preserves authenticated/request-scoped values but
  removes the browser connection deadline and cancellation signal.
- `streamCtx` is the Provider/Tool/Image Run context. Only explicit active/durable
  Run cancellation cancels it while work is in progress.
- `bestEffortStreamWriter` delegates headers and healthy writes. The first
  write, short-write, or `ResponseController.Flush` error marks delivery
  detached; later writes return success without touching the socket.
- Before socket delivery, the writer records each sequenced frame in the
  bounded active-Run ring. Detachment changes only the primary socket; cursor
  replay and final persistence continue.
- Delivery detachment never invokes `cancelAssistantAfterWriteError`. Provider
  consumption, Tool/Memory/Search continuation, final assistant persistence,
  Usage, and Memory capture continue exactly once.
- Server-mode Conversation/new-chat/assistant navigation does not abort the
  active controller. Explicit Stop and deletion of the owning active
  Conversation retain cancellation behavior.
- After `appendUserMessage` succeeds, frontend read-request supersession may
  suppress stale UI deltas but must not prevent `/stream` dispatch. A later
  Conversation reload reads the durable terminal message.

### 10.4 Validation and Error Matrix

| Condition | Required result |
| --- | --- |
| HTTP request context is cancelled before the first SSE write | Provider/Run context remains live; assistant reaches its normal durable terminal state |
| SSE write or flush fails during a delta | Mark delivery detached; consume remaining events and persist the full assistant |
| Browser reconnects with retained cursor | Replay the exact suffix, then subscribe atomically to new frames |
| Browser reconnects behind eviction | Emit explicit `stream.gap`, replay retained suffix, converge on terminal Message |
| Reconnect subscriber stops reading | Close only that subscriber; do not block Provider/Tool execution |
| Browser switches or creates a Conversation | Do not abort Server Run; stale visible state may ignore its deltas |
| Browser closes after generation is accepted | Backend continues without an attached client |
| User presses Stop after `message.started` | Client calls the Run cancel endpoint; Provider/Tool/Image context is cancelled and assistant is `cancelled` |
| Provider fails independently | Persist/emit the existing bounded provider failure; do not misclassify it as delivery detach |
| API process stops | In-process work may stop; this contract is not a durable external job queue |

### 10.5 Good / Base / Bad Cases

- Good: a long Tool-backed answer loses its socket halfway through, continues
  every continuation, resumes from the last cursor, and finishes as one
  completed full assistant.
- Base: an attached browser receives the unchanged ordered SSE sequence.
- Bad: Sidebar navigation calls `AbortController.abort()`, the API client calls
  `/cancel`, and a healthy Run is persisted as cancelled.

### 10.6 Tests Required

- Handler: cancel the HTTP request and fail the writer before `message.started`
  delivery and during a delta; assert Provider context remains live and the
  exact full content is persisted `completed` once.
- Handler image: repeat with a blocked image generator and assert the generated
  attachment is persisted after disconnect.
- Handler cancel: preserve active-registry and durable-store cancellation tests
  that assert prompt Provider cancellation and one `cancelled` terminal state.
- Frontend store: supersede the read request after user-message acceptance and
  assert `/stream` is still dispatched while the selected Conversation remains
  unchanged.
- Frontend composition/API: navigation has no implicit abort; explicit abort
  after `message.started` still calls `/v1/chat/runs/{runId}/cancel`.
- Ring/API: prove bounded eviction, gap metadata, exact-user/current-
  Conversation authorization, replay/live race freedom, and terminal grace.
- Frontend: split one response after sequence N, assert the resume URL/header,
  duplicate suppression, gap notice, and final authoritative Message.

### 10.7 Wrong vs Correct

Wrong:

```go
streamCtx, cancel := context.WithCancel(r.Context())
if writeSSEEvent(w, event, payload) != nil {
    cancelAssistantAfterWriteError(...)
}
```

Correct:

```go
generationCtx := context.WithoutCancel(r.Context())
streamCtx, cancelRun := context.WithCancel(generationCtx)
runStream := newActiveRunStream(runID, conversationID, messageID)
activeRuns.registerStream(runID, cancelRun, userID, conversationID, runStream)
delivery := newBestEffortStreamWriter(w)
delivery.attachStream(runStream)
w = delivery
```

## 11. Non-Goals

- Gemini and native OpenAI Responses API adapters.
- Stream endpoint auth enforcement through the new session-cache substrate.
- Durable external Run records or cross-process stream replay.
- Tool calls, plugins, attachments, MinIO/S3, RAG, title generation, and auth.
