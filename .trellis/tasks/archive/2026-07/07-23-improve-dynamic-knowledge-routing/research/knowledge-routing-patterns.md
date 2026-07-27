# Knowledge routing patterns

Research date: 2026-07-23

## Neo Chat current behavior

* Native Tool-capable providers receive `search_knowledge` with
  `tool_choice=auto`.
* The instruction only says selected collections exist and lists broad classes
  of private facts. It exposes no collection name, description, document title,
  or topic hint.
* Consequently the model cannot distinguish “a generic template request” from
  “a template that exists in the selected private collection” until the user
  explicitly mentions the Knowledge base.
* The non-Tool compatibility path currently searches selected Knowledge before
  answering, so it is not dynamic.

## Cherry Studio

Inspected repository `CherryHQ/cherry-studio` at
`fae2dcb566c512c3f7095c9753257bb41c5723fc`.

Cherry Studio does not solve routing by making selected Knowledge mandatory.
It registers agentic `kb_list` and `kb_search` tools when Knowledge is in scope:

* `kb_search` tells the model to use private Knowledge for the user's own
  materials and for topics likely covered by stored documents.
* `kb_list` is the discovery step. It returns each reachable base's name, group,
  item count, and up to 8 sample sources (filenames, URLs, or note titles).
* Its Tool Description explicitly tells the model to call `kb_list` first when
  it needs to learn which base/content is relevant, then call `kb_search`.
* Scope is enforced server-side; tools only apply when a Knowledge base exists
  and the effective per-assistant/per-turn scope is non-empty.
* An empty search result steers the model back to `kb_list` rather than claiming
  the Knowledge base is empty or retrying blindly.

Relevant files:

* `src/main/ai/tools/adapters/aiSdk/builtin/KnowledgeListTool.ts`
* `src/main/ai/tools/adapters/aiSdk/builtin/KnowledgeSearchTool.ts`
* `src/main/ai/tools/knowledgeLookup.ts`

Takeaway: content discovery needs bounded names/sample titles, not only a
generic “Knowledge exists” instruction. Neo Chat already has a selected scope,
so it can inject a bounded catalog directly and avoid an extra `kb_list ->
kb_search` round for the common case.

## Kelivo

Inspected repository `Chevey339/kelivo` at
`545f7d67de250283232c9487aa5f4f42e85a7643`.

The current tree contains chat tools, file handling, and embedding model types,
but no native Knowledge/RAG collection, retrieval, or Knowledge routing flow.
It therefore provides no comparable dynamic Knowledge decision mechanism.

## Options mapped to Neo Chat

1. Prompt-only intent expansion is cheap but still content-blind.
2. A bounded selected-Knowledge catalog mirrors Cherry's most useful discovery
   signal while keeping Neo Chat's existing single `search_knowledge` Tool and
   server-authoritative selected scope.
3. A relevance probe can inspect semantic content even with poor titles, but it
   spends retrieval latency/cost on every selected-Knowledge turn. It is a
   sensible second-stage fallback only if catalog-guided Auto Tool accuracy is
   insufficient.

