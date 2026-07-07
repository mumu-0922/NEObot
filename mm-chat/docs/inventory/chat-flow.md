# Chat Flow Inventory

This inventory captures the current chat generation path and the future server-backed target.

## Current Frontend Service Path

Main browser-facing service: `src/services/api/chatService.ts`.

Key responsibilities today:

- Build chat request payloads.
- Read settings, provider records, memory state, plugin state, and search/RAG settings from client stores.
- Prepare history and attachments.
- Call Next.js API routes such as `/api/chat` and `/api/chat/generate`.
- Read streamed response chunks with `ReadableStream.getReader()` and `TextDecoder`.
- Execute plugin/tool calls from the browser side when needed.
- Run background flows such as title generation, memory extraction, and context compression.

## Current Main Stream Path

```text
React chat UI
  ↓
src/features/chat hooks / chat store
  ↓
src/services/api/chatService.ts::streamChatResponse
  ↓ fetch('/api/chat')
Next.js route src/app/api/chat/route.ts
  ↓
src/lib/api/chat-handler.ts::handleChatStream
  ↓
ProviderFactory + provider-specific streaming implementation
  ↓
SSE-like stream chunks back to browser
```

## Current Supporting Generation Paths

```text
/api/chat/generate-title        title generation
/api/chat/related-questions     follow-up suggestions
/api/chat/rag-queries           query rewrite for RAG/search
/api/chat/generate-image        image generation
/api/chat/execute-code          code execution helper
```

## Target Go Chat Spine

```text
React chat UI
  ↓
Frontend API client contract
  ↓
POST /v1/chat/conversations/:id/messages
POST /v1/chat/conversations/:id/stream
  ↓
Go chat service
  ↓
Provider adapter
  ↓
OpenAI/Gemini/compatible provider
  ↓
SSE events
  ↓
Frontend renderer
```

## Required Server Events

```text
message.started
message.delta
message.completed
message.error
message.cancelled
usage.updated
```

## First MVP Behavior

- Create/list conversations.
- Append user message.
- Stream assistant message.
- Persist final assistant message.
- Support cancellation.
- Preserve history after refresh.
- Keep title generation and advanced helpers optional until the spine is stable.

## Extraction Risks

- `chatService.ts` currently mixes model calls, local memory, plugin tools, search decisioning, RAG query generation, and background compression.
- First Go MVP should not replicate every helper. It should implement the core message stream, then add helpers in separate phases.
- Tool/plugin execution has security implications and should be redesigned as a sandboxed backend capability later.

## Contract Boundary

The frontend should eventually call a stable client interface:

```ts
chatApi.createConversation(input)
chatApi.listConversations()
chatApi.listMessages(conversationId)
chatApi.sendMessage(conversationId, input)
chatApi.streamMessage(conversationId, input, handlers)
chatApi.cancelRun(runId)
```
