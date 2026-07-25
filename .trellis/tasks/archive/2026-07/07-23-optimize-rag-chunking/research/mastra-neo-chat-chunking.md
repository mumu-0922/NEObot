# Mastra and Neo Chat chunking comparison

## Sources

* <https://mastra.ai/docs/rag/chunking-and-embedding>
* <https://mastra.ai/reference/rag/chunk>
* `mm-chat/rag/src/mm_chat_rag/native_text_baseline.py`
* `mm-chat/rag/src/mm_chat_rag/mineru_gateway.py`
* `mm-chat/rag/src/mm_chat_rag/structure_chunking.py`
* `mm-chat/rag/src/mm_chat_rag/native_gateway.py`
* Live PostgreSQL active-generation aggregate queries on 2026-07-23.

## Mastra strengths worth adopting

* Strategy selection by document type instead of one splitter for every input.
* Recursive separator fallback for unstructured text.
* Sentence-aware target/min/max sizing.
* Real tokenizer support for token-sensitive strategies.
* Markdown, HTML, JSON, and LaTeX structural strategies.
* Semantic Markdown merging for related small sections under a token threshold.

## Mastra constraints not to copy blindly

* The general length function defaults to character count, not model tokens.
* Markdown with headers and HTML with headers can ignore general size limits.
* The returned shape is a flat `DocumentNode` list; it does not replace Neo
  Chat's Parent/Child containment, exact source spans, projection generations,
  provenance, or citation locators.
* Semantic behavior must not make persisted retrieval truth non-reproducible.

## Neo Chat live baseline

The active profile is the text baseline hash. It uses fixed 2,400-byte,
UTF-8-safe windows, `ceil(bytes / 4)` token estimates, no overlap, empty heading
paths, and identical Parent/Child chunks. This is deterministic but weak at
semantic boundaries.

## Neo Chat structure candidate

The repository planner already models structural units, heading-bounded
Parents, 300-500 target Children, a 650 Child hard cap, 1,200-1,600 target
Parents, a 2,000 Parent hard cap, and exact 64-target/100-max adjacent overlap.
Protected table/code/formula units remain atomic while they fit. It preserves
UTF-8-safe source ranges and is deterministic.

## Recommended synthesis

Use a deterministic structure-first router:

1. Preserve parser-native block and reading-flow boundaries.
2. Count with one pinned tokenizer matching the retrieval/answer model family.
3. Use headings/sections for Parent envelopes.
4. Use sentence/recursive boundaries for paragraph-like Children.
5. Keep tables, code, formulas, slides, sheets, and JSON subtrees atomic until
   an explicit hard-cap fallback is required.
6. Optionally apply sentence-embedding breakpoint detection only inside long,
   unstructured narrative blocks; never across protected structure.
7. Retain exact source ranges, Parent/Child containment, deterministic overlap,
   generation isolation, and atomic cutover.

Do not import Mastra solely for chunking. Reuse its useful strategy ideas inside
the existing planner and projection contracts.
