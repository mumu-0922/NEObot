# Services

The `src/services` directory contains browser-facing service modules. These modules call Next.js API routes, prepare client-owned data for requests, and coordinate feature workflows that do not belong inside React components.

## Directory Map

```text
src/services/
├── api/
│   ├── agentService.ts
│   ├── chatCrudService.ts
│   ├── chatService.ts
│   ├── chatStreamService.ts
│   ├── fileService.ts
│   ├── importService.ts
│   └── voiceService.ts
├── artifactService.ts
└── README.md
```

## API Client Services

### `chatService.ts`

Handles chat generation workflows from the browser side:

- Streams chat responses.
- Delegates provider-native Tool execution to the Go chat stream.
- Generates titles, related questions, and image outputs.
- Prepares history for model APIs.
- Adds local memory context when enabled by the chat workflow.
- Runs background context compression.
- Updates tool-call status while streaming and while executing tools.

### `agentService.ts`

Fetches assistant marketplace data and assistant details from app API routes.
The current Assistant library and Store lifecycle is server-authoritative
through `services/api/client/*/agentApi.ts`. Legacy registry helpers remain
only for compatibility; UI installation, custom editing, revisions, and
admission no longer use browser-owned Assistant state.

### `voiceService.ts`

Calls speech-to-text and text-to-speech routes. Browser-native, ElevenLabs, and Mimo-backed flows are selected from user settings or server defaults.

## Client-Only Services

### `artifactService.ts`

Manages generated artifact creation, editing, continuation, transformation, and preview behavior. This module is client logic and does not directly own server routes.

## Design Boundaries

- Components should call services rather than embedding fetch logic directly.
- Knowledge upload, indexing, retrieval, and citations use the typed Go API
  client; browser services do not parse or query documents locally.
- MCP server discovery, authorization, selection, and call timelines use the
  typed `/v1/mcp/*` client. The browser never executes MCP Tools itself.
- Legacy text-Skill services and prompt-context assembly were deleted in G20.9.
  Package Skills use the typed server API and never execute in the browser.
- Services may read local settings when a workflow requires browser-owned data.
- Sensitive user-entered secrets should travel as encrypted BYOK envelopes.
- Server-only validation and proxy policy should stay in `src/app/api` and `src/lib/security`.
- Store mutations should remain explicit at call sites or in store actions; avoid hidden writes inside low-level service helpers.

## Example

```typescript
import {
  generateChatTitle,
  prepareHistoryForLLM,
  streamChatResponse,
} from "@/services/api/chatService";

await streamChatResponse(
  sessionId,
  model,
  history,
  message,
  attachments,
  config,
  onChunk,
  systemInstruction,
);

const title = await generateChatTitle(history);
const preparedHistory = await prepareHistoryForLLM(
  messages,
  compression,
  model,
);
```

## Testing Guidance

- Mock `fetch` or service dependencies at the route boundary.
- Test streaming and tool-call behavior with representative chunks.
- Keep provider-specific request shaping covered by route tests.
- Add regression tests when service code coordinates several stores or APIs.
