# Chat Library

The `src/lib/chat` directory contains chat-domain helpers that are shared by stores, services, API payload preparation, and UI flows. These modules should understand messages, sessions, branches, and generation lifecycle state, but they should not render UI.

## Files

- `effectiveChatContext.ts` builds the effective provider/search/RAG/plugin/skill context for a request.
- `entities.ts` normalizes chat-domain entities.
- `generationLifecycle.ts` coordinates generation state transitions.
- `generationProgress.ts` infers the pending in-thread status from the question and enabled server sources before the first stream event arrives.
- `htmlVisualPrompt.ts` builds the optional prompt guidance for safe inline HTML visual output.
- `messageProcessor.ts` prepares user messages and attachments before sending.
- `messageOutputBlocks.ts` builds streamed output blocks such as search, reasoning, tool, and content sections.
- `messageNavigation.ts` builds bounded user-message navigation entries, resolves active reading position, reveals old targets, and computes container-local scroll offsets.
- `messageTree.ts` manages branched message relationships.
- `postGenerationGuards.ts` handles post-generation safety checks.
- `progressiveMessageRendering.ts` defines the bounded recent-tail and idle-batch policy used to keep long conversation switches responsive.
- `searchUpdate.ts` merges streamed search sources and images without duplicating prior entries.
- `scrollFollow.ts` keeps live bottom-follow under explicit user control and computes floating-composer clearance.
- `serverConversationCache.ts` owns the bounded, memory-only server conversation snapshot cache used for stale-while-revalidate selection.
- `sessionExport.ts` serializes and imports chat session data.

## Message Processing

`messageProcessor.ts` prepares outgoing messages by combining plain text, attachments, RAG context, selected knowledge scope, model capability checks, and placeholder model messages.

```typescript
const processed = await processMessageForSending({
  text: "User message",
  attachments: [],
  selectedModel: "GEMINI:gemini-2.5-flash",
  modelMetadata,
  customModelMetadata,
  ragConfig,
  knowledgeCollections,
});

const { finalText, finalAttachments, ragSources, userMessage } = processed;
```

## Design Guidelines

- Keep message transformations deterministic.
- Preserve persisted-session compatibility when changing entity shapes.
- Keep provider-specific conversion in lower-level utilities where possible.
- Add tests for branch behavior, export/import compatibility, and lifecycle transitions.
- Avoid React imports in this directory.
- Keep current-public markers in `generationProgress.ts` aligned with the Go source-fusion router.
- Treat upward wheel/pointer intent as authoritative: streamed content must not pull a reader back to the bottom until they return there.
- Keep server conversation caches memory-only and bounded. Show a snapshot immediately, always revalidate against Go/Postgres, and suppress a second render when the authoritative tree is unchanged.
- Reveal older long-conversation messages in idle batches and compensate for prepended height so the reader's current viewport remains anchored.
- Resolve navigation from the full active message path, but scroll only by a stable rendered message ID; reveal a target preceding the current render window before completing the jump.
