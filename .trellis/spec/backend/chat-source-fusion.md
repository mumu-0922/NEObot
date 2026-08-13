# Chat source-fusion contracts

> Runtime status: G19.3 promoted external Web planning/execution to the live
> Tool Loop, so [`chat-tool-loop.md`](./chat-tool-loop.md) is authoritative for
> external Search. This document remains the rollback contract for that path
> and the runtime contract for pre-answer Knowledge/source fusion until G19.6.
> Legacy fusion diagnostics remain query-free; the sanitized displayed Query
> belongs only to the process trace.

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
- Current-turn Web citation reconciliation preserves each source's originally
  minted marker. A used subset such as `[W1]`, `[W5]`, `[W7]`, and `[W10]`
  remains sparse in storage and transport; clients resolve these markers from
  `source.metadata.marker`, never from the source's compacted array position.
  Positional linking is only a legacy fallback when no authoritative Web
  marker metadata exists.

## 4. Validation & Error Matrix

| Condition                                           | Required result                                     |
| --------------------------------------------------- | --------------------------------------------------- |
| Search disabled                                     | no rewrite/provider I/O; `disabled`                 |
| Router skips Search                                 | no rewrite/provider I/O; `skipped`                  |
| External Search, no prior active-branch history     | current query; `unchanged`                          |
| Contextual follow-up rewrites successfully          | standalone query; `rewritten` and derived flag true |
| Rewrite returns the original query                  | original query; `unchanged`                         |
| Rewrite provider fails or output exceeds 2048 bytes | current query; `failed`; chat continues             |
| Built-in model Search                               | provider owns query planning; `provider_managed`    |
| Search resolution fails before rewrite              | no query call; `not_run`                            |
| Used Web citations have sparse original markers     | every exact marker links to its matching source     |
| Web marker is absent from authoritative metadata    | leave it unlinked; never mislink by array position  |

## 5. Good / Base / Bad Cases

- **Good:** after discussing DeepSeek V4 Flash context length, “你自己联网搜”
  searches a standalone DeepSeek V4 Flash context-window query.
- **Good:** “你知道你是谁吗？” resolves “you” to the current runtime model
  identifier rather than matching a same-named song.
- **Base:** a first-turn explicit topic has no history and searches the current
  message unchanged.
- **Base:** legacy source arrays without marker metadata retain positional
  citation linking.
- **Good:** a compacted four-source array carrying `[W1]`, `[W5]`, `[W7]`, and
  `[W10]` links all four exact markers to those four sources.
- **Base:** the rewrite provider fails once; external Search uses the current
  message and the final chat still completes.
- **Bad:** literal-searching an ambiguous current message, sending raw history
  to Tavily/Exa/Bocha/Firecrawl, persisting the rewritten query, or changing
  Search provider after a rewrite failure.
- **Bad:** interpreting `[W10]` as compacted source index 9 after unused
  citations have been removed.

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
6. Frontend citation tests must cover sparse authoritative Web markers, unknown
   markers, and the legacy positional fallback without interpolating raw URLs.

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

For current-turn Web citation rendering:

Wrong:

```ts
const sourceIndex = Number(marker.slice(2, -1)) - 1;
```

Correct:

```ts
const sourceIndex = sources.findIndex(
  (source) => source.metadata?.marker === marker,
);
```

The numeric suffix is a stable current-turn citation identity, not the
position of a later compacted source array.
