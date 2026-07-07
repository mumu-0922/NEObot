# Phase 5.2 Chat Stream API Contract

## 1. Purpose

Phase 5.2 adds the first provider-neutral streaming spine. It assumes the user
message already exists, creates a `streaming` assistant row, emits SSE frames,
and finalizes the assistant row as `completed`, `failed`, or `cancelled`.

```text
POST /v1/chat/conversations/{id}/messages -> persisted user message
POST /v1/chat/conversations/{id}/stream   -> streaming assistant message
```

This phase uses a mock provider in tests only. Real provider adapters, explicit
cancel endpoints, Redis cancellation flags, files, tools, RAG, and auth remain
later work.

## 2. Endpoint

```http
POST /v1/chat/conversations/{id}/stream
Accept: text/event-stream
Content-Type: application/json
```

Success response:

```http
HTTP/1.1 200 OK
Content-Type: text/event-stream; charset=utf-8
Cache-Control: no-cache
X-Content-Type-Options: nosniff
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
- `modelRef` is required; Phase 5.2 does not resolve provider settings yet.
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

## 6. Error Contract

Errors before the SSE response begins use the standard JSON envelope.

| HTTP | Code | When |
| --- | --- | --- |
| `400` | `INVALID_JSON` | Request body is malformed JSON. |
| `400` | `INVALID_CONVERSATION_ID` | Path conversation ID is not a UUID. |
| `400` | `INVALID_USER_MESSAGE_ID` | `userMessageId` is missing, invalid, missing, or not a user message in the conversation. |
| `400` | `MODEL_REF_REQUIRED` | `modelRef` is missing. |
| `400` | `IDEMPOTENCY_KEY_REQUIRED` | `idempotencyKey` is blank or missing. |
| `400` | `VALIDATION_ERROR` | Unsupported stream fields such as `content` or `attachments`. |
| `400` | `FORBIDDEN_MESSAGE_FIELD` | Server-managed message fields or identity hints are present. |
| `404` | `CONVERSATION_NOT_FOUND` | Conversation is missing or not owned by the fixed dev user. |
| `409` | `IDEMPOTENCY_CONFLICT` | Assistant stream key already exists for the conversation. |
| `502` | `PROVIDER_ERROR` | Provider startup fails before SSE begins. |
| `503` | `DATABASE_REQUIRED` | DB runtime wiring is disabled. |
| `503` | `PROVIDER_REQUIRED` | No provider is configured for streaming. |
| `500` | `STREAMING_UNSUPPORTED` | Response writer cannot flush SSE. |

After SSE starts, provider or finalization failures are emitted as
`message.error` frames with scrubbed error details.

## 7. Non-Goals

- Real OpenAI/Gemini/OpenAI-compatible adapters.
- Redis-backed cancellation state or `POST /v1/chat/runs/{runId}/cancel`.
- Streaming resume, cursor replay, or durable run records.
- Tool calls, plugins, attachments, MinIO/S3, RAG, title generation, and auth.
