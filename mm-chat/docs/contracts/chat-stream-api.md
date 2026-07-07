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
endpoint for streaming assistant rows. Redis cancellation flags, files, tools,
RAG, and auth remain later work.

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
Cache-Control: no-cache
X-Content-Type-Options: nosniff
```

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
```

Rules:

- `conversationId` is path-only and rejected in the body.
- `userMessageId` is required and must reference an existing `role="user"`
  message in the same conversation.
- `modelRef` is required. `modelRef.modelId` is sent to the provider; if it is
  blank, the backend falls back to `PROVIDER_MODEL`.
- `idempotencyKey` is required and applies to the assistant streaming row only.
- `content`, `attachments`, `role`, `status`, identity hints, and other
  server-managed message fields are rejected.
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
   - request context cancellation -> `status='cancelled'`

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
requests inside the same API process. It does not yet interrupt streams across
workers, processes, or restarts; Redis cancellation flags are reserved for
Phase 7.

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
| `502` | `PROVIDER_ERROR` | Provider startup fails before SSE begins. |
| `503` | `DATABASE_REQUIRED` | DB runtime wiring is disabled. |
| `503` | `PROVIDER_REQUIRED` | No provider is configured for streaming. |
| `500` | `STREAMING_UNSUPPORTED` | Response writer cannot flush SSE. |

After SSE starts, provider or finalization failures are emitted as
`message.error` frames with scrubbed error details.

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

## 8. OpenAI-Compatible Provider Configuration

The first real adapter uses the OpenAI-compatible Chat Completions stream
shape:

```http
POST ${PROVIDER_BASE_URL}/chat/completions
Authorization: Bearer ${PROVIDER_API_KEY}
Content-Type: application/json
Accept: text/event-stream
```

Environment variables:

```env
PROVIDER_TYPE=openai_compatible
PROVIDER_BASE_URL=https://your-openai-compatible-relay.example/v1
PROVIDER_MODEL=gpt-5.5
PROVIDER_API_KEY=change-me
PROVIDER_TIMEOUT=2m
```

Runtime rules:

- If `PROVIDER_TYPE` is empty, streaming returns `503 PROVIDER_REQUIRED`.
- If `PROVIDER_TYPE=openai_compatible` but required provider fields are missing,
  the provider remains disabled and streaming returns `503 PROVIDER_REQUIRED`.
- Unsupported provider types fail API startup.
- `modelRef.providerId` must match the configured single provider:
  `openai_compatible` plus temporary aliases `openai-compatible` or `openai`.
  Accepted aliases are persisted and emitted as canonical `openai_compatible`.
- Provider API keys are process environment only; they are never returned in
  API responses or committed to the repository.
- Non-`2xx` provider startup responses map to pre-SSE `502 PROVIDER_ERROR`.
- Malformed provider SSE frames after streaming begins map to scrubbed
  `message.error` frames.
- Provider streams that end without `data: [DONE]` are treated as failed
  partial streams and map to scrubbed `message.error` frames.

The adapter reads `data:` SSE frames, emits `message.delta` for
`choices[].delta.content`, emits `usage.updated` when a provider chunk includes
`usage`, and stops on `data: [DONE]`.

## 9. Non-Goals

- Gemini and native OpenAI Responses API adapters.
- Redis-backed cross-process cancellation state.
- Streaming resume, cursor replay, or durable run records.
- Tool calls, plugins, attachments, MinIO/S3, RAG, title generation, and auth.
