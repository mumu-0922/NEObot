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
cancellation flags for cross-process stream interruption. Files, tools, RAG, and
auth remain later work.

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
```

Rules:

- `conversationId` is path-only and rejected in the body.
- `userMessageId` is required and must reference an existing `role="user"`
  message in the same conversation.
- `modelRef` is required and both IDs must be non-empty. `modelRef.modelId` is
  sent to the resolved provider; there is no environment model fallback.
- `idempotencyKey` is required and applies to the assistant streaming row only.
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

Every frame uses a named `event:` line matching `data.type` and a single JSON
object in `data:`.

Required sequence for a successful mock/provider stream:

```text
message.started
message.delta        # zero or more; mock emits deterministic chunks
usage.updated        # emitted when provider usage is available
message.completed
```

Terminal events are mutually exclusive:

```text
message.completed
message.error
message.cancelled
```

Example:

```text
event: message.started
data: {"type":"message.started","runId":"...","conversationId":"...","messageId":"...","sequence":1,"createdAt":"2026-07-07T10:00:00Z","role":"assistant","modelRef":{"providerId":"mock","modelId":"mock-chat"}}

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
   - provider error -> `status='failed'`
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
  partial streams and map to scrubbed `message.error` frames.
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
| Browser switches or creates a Conversation | Do not abort Server Run; stale visible state may ignore its deltas |
| Browser closes after generation is accepted | Backend continues without an attached client |
| User presses Stop after `message.started` | Client calls the Run cancel endpoint; Provider/Tool/Image context is cancelled and assistant is `cancelled` |
| Provider fails independently | Persist/emit the existing bounded provider failure; do not misclassify it as delivery detach |
| API process stops | In-process work may stop; this contract is not a durable external job queue |

### 10.5 Good / Base / Bad Cases

- Good: a long Tool-backed answer loses its socket halfway through, continues
  every continuation, and reloads as one completed full assistant.
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
activeRuns.register(runID, cancelRun)
w = newBestEffortStreamWriter(w)
```

## 11. Non-Goals

- Gemini and native OpenAI Responses API adapters.
- Stream endpoint auth enforcement through the new session-cache substrate.
- Streaming resume, cursor replay, or durable run records.
- Tool calls, plugins, attachments, MinIO/S3, RAG, title generation, and auth.
