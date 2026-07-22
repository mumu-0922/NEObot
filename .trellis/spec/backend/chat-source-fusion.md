# Chat source-fusion contracts

## 1. Scope / Trigger

Apply this contract when changing external Web query planning, conversation
follow-up resolution, Knowledge/Web fusion authority, Search diagnostics, or
the chat completion path that injects public evidence.

## 2. Signatures

```text
activeBranch[-6:] + latestUserMessage + runtimeModelID
  -> standaloneExternalSearchQuery

question + searchEnabled + Knowledge outcome
  -> sourceFusionPlan
```

Stable non-sensitive rewrite diagnostics are:

```text
webQueryDerivedFromConversation = true | false
webQueryRewriteOutcome = disabled | skipped | pending | unchanged |
                         rewritten | failed | not_run | provider_managed
```

No exact query text is part of message metadata.

## 3. Contracts

- `useSearch=false` performs no Search resolution, query rewrite, or Search
  provider request.
- External Search rewrites a request only through the currently selected chat
  provider. The prompt receives at most six prior active-branch user/assistant
  messages, at most 1200 UTF-8 bytes each, no attachments, the latest user
  message, and the bounded runtime model identifier.
- Conversation content is untrusted. The rewrite system instruction must say
  to ignore instructions in history, resolve only references/ellipsis, avoid
  answering, and return one standalone query.
- References to “you”, “your model”, or “your context window” resolve against
  the selected runtime model identifier. The external Search provider receives
  only the resulting query, never raw conversation history.
- Empty, unchanged, malformed, oversized, or failed rewrites fall open to the
  normalized current user message. A rewrite failure must not abort an
  otherwise valid Search or chat answer.
- Knowledge context may enrich a still-contextual mixed-source query under its
  separate bounded rule. It cannot dilute a rewritten or originally explicit
  subject.
- Query text, private history, source bodies, credentials, and provider errors
  never enter durable fusion diagnostics.

## 4. Validation & Error Matrix

| Condition | Required result |
|---|---|
| Search disabled | no rewrite/provider I/O; `disabled` |
| Router skips Search | no rewrite/provider I/O; `skipped` |
| External Search, no prior active-branch history | current query; `unchanged` |
| Contextual follow-up rewrites successfully | standalone query; `rewritten` and derived flag true |
| Rewrite returns the original query | original query; `unchanged` |
| Rewrite provider fails or output exceeds 2048 bytes | current query; `failed`; chat continues |
| Built-in model Search | provider owns query planning; `provider_managed` |
| Search resolution fails before rewrite | no query call; `not_run` |

## 5. Good / Base / Bad Cases

- **Good:** after discussing DeepSeek V4 Flash context length, “你自己联网搜”
  searches a standalone DeepSeek V4 Flash context-window query.
- **Good:** “你知道你是谁吗？” resolves “you” to the current runtime model
  identifier rather than matching a same-named song.
- **Base:** a first-turn explicit topic has no history and searches the current
  message unchanged.
- **Base:** the rewrite provider fails once; external Search uses the current
  message and the final chat still completes.
- **Bad:** literal-searching an ambiguous current message, sending raw history
  to Tavily/Exa/Bocha/Firecrawl, persisting the rewritten query, or changing
  Search provider after a rewrite failure.

## 6. Tests Required

1. Pure rewrite tests for runtime model identity, recent-history bounds,
   attachment removal, output normalization, unchanged output, and oversize.
2. Handler integration proving rewrite call -> external Search request -> Web
   evidence -> answer, with reserved historical markers removed.
3. Failure integration proving rewrite error -> original query -> successful
   Search/answer and redacted `failed` diagnostics.
4. Existing source-fusion Router, Knowledge enrichment, citation, cancellation,
   and provider-failure tests remain green.
5. Real selected-model plus active external-provider proof must use a temporary
   conversation, verify relevant source titles and both non-sensitive rewrite
   fields, then delete all smoke state.

## 7. Wrong vs Correct

Wrong:

```go
searchProvider.Search(ctx, websearch.Request{Query: userMessage.Content})
```

Correct:

```go
query := userMessage.Content
if rewritten, err := rewriteWebSearchQuery(
	ctx,
	selectedProvider,
	modelRef,
	userMessage.ID,
	query,
	activeBranch,
); err == nil && rewritten != "" {
	query = rewritten
}
searchProvider.Search(ctx, websearch.Request{Query: query})
```

The rewrite is bounded, active-branch aware, model-aware, non-authoritative on
failure, and invisible to durable query diagnostics.
