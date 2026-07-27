# Mainstream RAG routing in high-star OSS projects

Research date: 2026-07-23

## Scope and evidence

This comparison focuses on end-user RAG applications rather than generic
frameworks. Star counts are approximate values displayed by Shields.io on the
research date; they only establish that the projects are widely adopted.

| Project | Approx. stars | Inspected commit |
| --- | ---: | --- |
| Dify | 150k | `b56ac4af4d1470eedd68236d581174a0c29d57b2` |
| Open WebUI | 146k | `ecd48e2f718220a6400ecf49eafd4867a38feb10` |
| RAGFlow | 86k | `6467df343791529d4619ad37db7abfe4ef77982b` |
| AnythingLLM | 64k | `28fbff47f8d3dd57f7228f81355406e78065cbd5` |

The decisive pattern is not one universal router. Mature products usually
carry two paths:

1. a deterministic RAG-chat path that retrieves from attached datasets on
   every question, then filters/reranks results; and
2. an agentic path that exposes Knowledge as tools and lets the model decide
   whether to retrieve.

## Dify

Dify supports both `single` and `multiple` retrieval strategies.

* A dataset becomes a Tool whose description is the dataset description, or a
  fallback derived from its name. The description is the routing hint.
* In `single` mode, zero datasets produce no action, exactly one dataset is
  selected without an LLM routing decision, and multiple datasets use an LLM
  Function Call/ReAct router to choose one or none.
* In `multiple` mode, all available datasets are searched and results are
  globally thresholded/reranked.

This means Dify's model-based routing is primarily a dataset selector when
multiple candidates exist. It is not proof that a single selected Knowledge
base should always be queried in a general-purpose chat product.

Relevant files:

* `api/core/tools/utils/dataset_retriever/dataset_retriever_tool.py`
* `api/core/rag/retrieval/dataset_retrieval.py`
* `api/core/rag/retrieval/router/multi_dataset_function_call_router.py`

## Open WebUI

Open WebUI now has a progressive, agentic Knowledge tool set:

* `list_knowledge` tells the model to use it first for discovery. Without a
  Knowledge ID it returns Knowledge names, descriptions, and file counts; with
  an ID it returns a paginated file listing.
* `query_knowledge_files` performs semantic retrieval, while
  `view_knowledge_file` can inspect a selected file more directly.
* Tool exposure is scoped to the model's attached Knowledge and checked against
  user/resource access.

Open WebUI also retains the traditional attachment pipeline: attached
collections are converted to collection names, access-filtered, and queried by
`query_collection`, including hybrid search/reranking where enabled. Thus the
same product supports both deterministic RAG and model-driven Knowledge use.

Relevant files:

* `backend/open_webui/utils/tools.py`
* `backend/open_webui/tools/builtin.py`
* `backend/open_webui/retrieval/utils.py`

## RAGFlow

RAGFlow separates its standard Chat Assistant path from Agent Flow:

* In standard chat, when the prompt has the `knowledge` parameter and datasets
  are bound, the request proceeds through retrieval, similarity threshold,
  optional reranking/TOC/KG enrichment, and then injects resulting chunks into
  the answer prompt.
* Agent Flow exposes `search_my_dateset` as a Retrieval Tool. The agent decides
  when to call it and supplies important query terms/synonyms.

Relevant files:

* `api/db/services/dialog_service.py`
* `agent/tools/retrieval.py`

## AnythingLLM

AnythingLLM shows the same duality:

* Normal workspace chat calls `performSimilaritySearch` whenever the workspace
  contains embeddings. Threshold, top-N, and optional reranking control which
  context survives. Query mode refuses to answer without context; Chat mode can
  fall back to the model.
* Agent mode registers `rag-memory`, whose description and examples teach the
  model to search local documents when relevant. Only a Tool call triggers
  `performSimilaritySearch`; a miss can send the agent toward Web search.

Relevant files:

* `server/utils/chats/apiChatHandler.js`
* `server/utils/agents/aibitat/plugins/memory.js`

## Pattern comparison

### Deterministic RAG chat

```text
selected datasets -> retrieve every turn -> threshold/rerank -> inject or skip
```

Strengths:

* predictable recall for a dedicated "chat with these documents" experience;
* weak models do not need to make a Tool decision.

Costs:

* every turn pays embedding/vector-store latency and failure exposure;
* irrelevant questions still touch private Knowledge infrastructure;
* thresholding filters bad results after retrieval; it does not avoid retrieval.

### Agentic RAG

```text
capability/catalog hint -> model decides -> Knowledge Tool -> retrieve/rerank -> cite
```

Strengths:

* unrelated turns can use the model or Web with no Knowledge I/O;
* fits general chat where selected Knowledge is an allowed scope, not the only
  authority.

Costs:

* routing quality depends on model capability and the quality of names,
  descriptions, document titles, Tool descriptions, and examples;
* content-blind Tool descriptions cause false negatives; unbounded catalogs
  create token, privacy, and prompt-injection risks.

## Mapping to Neo Chat

Neo Chat is a general-purpose chat with model, Web, and Knowledge authorities,
and the product requirement explicitly rejects full RAG on every selected-
Knowledge turn. Therefore its closest mainstream pattern is Agentic RAG, not
the deterministic default in RAGFlow or AnythingLLM workspace chat.

Recommended MVP:

1. treat selected Knowledge IDs as an ACL-bounded authorization scope;
2. expose a small, governed catalog of collection name/description and active
   document titles as untrusted routing metadata;
3. keep native `tool_choice=auto` and let the model call `search_knowledge` only
   when the current question plausibly overlaps the catalog or explicitly asks
   for private materials;
4. keep retrieval, reranking, and citations server-authoritative after the Tool
   call;
5. use a bounded same-model routing planner for providers without native Tools;
6. add an always-on relevance probe only as a measured second phase if catalog-
   guided routing still misses too often.

This resembles a Skill registry through progressive disclosure, but the
catalog is only a capability/index hint. The Knowledge Tool still has to cross
ACL boundaries, retrieve untrusted evidence, rerank it, and produce citations;
the catalog itself must never be treated as answer evidence.
