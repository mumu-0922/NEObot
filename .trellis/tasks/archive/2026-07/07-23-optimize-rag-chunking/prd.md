# Optimize structure-aware RAG chunking

## Goal

Replace the active byte-window knowledge chunking baseline with a production
structure-first, token-aware, optionally semantic chunking profile that combines
Mastra's useful strategy selection with Neo Chat's Parent/Child, provenance,
locator, generation, and atomic-cutover contracts.

## What I already know

- Accuracy is the primary product goal; latency and cost remain bounded rather
  than minimized at the expense of answer quality.
- The live active generation uses chunk profile
  `48ac1810a92dcdd61db73646f3c8780e8ebc76b1525145452df7e3c0a819bb03`.
- The active baseline splits at 2,400 UTF-8 bytes, estimates tokens as
  `ceil(bytes / 4)`, has no overlap or heading path, and projects identical
  Parent and Child content.
- Live aggregate evidence contains 27 Parents and 27 Children across 19
  documents, with zero overlapped Children and zero non-empty heading paths.
- The repository already contains a deterministic structure planner with
  section-bounded Parents, 300-500 target Children, protected structural units,
  and exact adjacent overlap, but it is selected only by a generation bound to
  `STRUCTURE_CHUNK_PROFILE_HASH`.
- Mastra provides type-aware `recursive`, `sentence`, `token`, `markdown`,
  `semantic-markdown`, `html`, `json`, and `latex` strategies. Its general
  default length function is character count; token-aware strategies use a
  tokenizer. Markdown/HTML header modes can ignore general size limits.

## Assumptions (temporary)

- The solution should remain internal rather than add Mastra as a runtime
  dependency.
- Existing documents must be rebuilt into a candidate generation and promoted
  atomically; the active generation must not be mutated in place.
- Structure-first splitting remains authoritative for tables, code, formulas,
  slides, sheets, headings, and citation locators.

## Open Questions

- Candidate 8 passed the frozen Candidate-only gate on 2026-07-25. Activation
  remains a separate operator decision bound to the exact gate-report SHA-256;
  no activation approval has been granted.

## Resolved Decisions

- On 2026-07-25, the owner confirmed that the `1000ms` Retrieval P95 budget is
  a steady-state requirement. New-process cold-start latency may be higher and
  is reported separately rather than blocking Promotion. This decision does
  not rewrite the canonical failed supplemental report, create Activation
  authority, or approve automatic warm-up. No hard cold-start SLA is committed
  in this task.

## Requirements (evolving)

- Immediately after the Jina runtime fence lands, production Knowledge enters
  a temporary BM25-only mode until Candidate 8 completes its BGE-only gates and
  receives explicit Activation approval. The legacy Active Generation may
  contribute only its same-generation BM25/Citation projection during this
  interval: Query Embedding, Dense retrieval, Rerank, evaluation, rebuild, and
  every other provider-backed path must make zero Jina requests. The later BGE
  cutover remains an atomic pointer transition and must not mutate the legacy
  Generation.
- SiliconFlow BGE becomes the sole retrieval provider for all new passage/query
  Embedding, Rerank, rebuild, evaluation, and production Knowledge requests.
  No new runtime or evaluation request may call Jina, and BGE completion must
  not wait on Jina availability.
- Development/Validation tooling must evaluate the BGE Candidate independently;
  it may not require a paired live Jina baseline capture. Until the chosen
  cutover policy is complete, reports remain `promotionEligible=false`, never
  execute Holdout, and never activate implicitly.
- Jina is permanently non-executable. Remove its administrator UI, stored Key,
  provider selection, Worker dispatch, query Embedding, Rerank, evaluation, and
  rollback entrypoints. Retain existing Jina Generation/projection/schema rows
  only as read-only audit history; they may not become Active again or authorize
  any provider request. Physical deletion of that historical state is deferred
  to a separately approved purge after the observation window.
- BGE activation requires the complete BGE-only Development and Validation
  gates followed by exactly one execution of the precommitted frozen Holdout.
  Passing evidence still cannot activate automatically: the owner must review
  the exact report hash and explicitly approve the Candidate 8 pointer change.
- Remove every Jina Active-vs-Candidate relative-improvement and no-regression
  dependency from promotion. BGE is admitted only by its frozen strict absolute
  aggregate and per-slice gates, deterministic repeatability, provenance and
  Citation integrity, zero leakage, and explicit latency/context budgets. Old
  Jina v1 observations remain historical diagnostics and are not promotion
  evidence.
- Route by canonical document/block type rather than expose one global splitter.
- Use a pinned real tokenizer for chunk size accounting; do not use UTF-8 byte
  count as the production token authority.
- Preserve Parent/Child containment, heading boundaries, exact overlap ranges,
  source locators, provenance, deterministic replay, and hard complexity caps.
- Keep protected structural units atomic when possible and apply explicit
  type-specific fallback splitting when a unit exceeds the hard token cap:
  tables and sheets split by row groups while retaining headers; code splits by
  class/function/method and then logical regions; JSON splits by subtrees while
  retaining its parent path; presentations split by slide and then shape; and
  formulas remain atomic with their source-adjacent explanation when possible.
  A generic hard token cut is the final bounded fallback, never the first
  strategy, and LLM summaries never replace source retrieval truth.
- For long unstructured narrative blocks only, perform sentence-embedding
  semantic boundary detection with the admitted embedding provider. Cache the
  result by profile and block-content hash; timeout, quota, or provider failure
  must fall back to deterministic sentence/recursive splitting rather than
  fail the document import.
- Replace the Jina retrieval provider profile with SiliconFlow's paid
  `Pro/BAAI/bge-m3` Embedding model and
  `Pro/BAAI/bge-reranker-v2-m3` Reranker. Pin the exact provider endpoint,
  model IDs, dimensions, maximum input, normalization/instruction policy, and
  response contract in a new versioned Search Profile. A BGE query vector may
  never search a Jina projection, and matching `1024` dimensions do not make
  the vector spaces compatible. The migration therefore requires a complete
  new Candidate Generation and may not rewrite or partially reuse the current
  Jina Generation. Provider failure may fail closed or use the existing BM25
  lane according to the explicit retrieval fallback contract; it must never
  substitute a different Embedding model against the BGE vector index.
- Rebuild through a non-active candidate Index Generation, evaluate it against
  the current active generation, and cut over atomically only after passing.
- The first production activation must require explicit operator approval after
  automated Golden, retrieval, citation, integrity, and performance gates pass.
  A passing candidate must never activate itself during the initial rollout.
- Query-time retrieval must rank Child chunks first, then expand high-confidence
  hits to their Parent adaptively under a shared Knowledge/Web context budget.
  Expansion must deduplicate Parents and preserve the exact matched Child as
  citation evidence; it must not attach every Parent unconditionally.
- Every Child may carry a bounded, deterministic derived-context prefix for
  embedding and answer assembly: heading path, table/sheet headers, JSON path,
  code file/class/signature, or slide title as applicable. Derived fragments
  must be labeled separately, hash-bound to their source structures, and never
  masquerade as original quoted text. Citations must resolve to the original
  header/path/row/code/shape source spans.
- Query-time retrieval, reranking, and answer assembly must also carry the
  current authorized source filename as a bounded, explicitly labeled metadata
  field. A normalized source-name lane may boost a document only when its
  basename is present in the query; it must then rank source-backed Children
  inside that document, rejoin the same document/version/materialization
  authority as the Child, and never turn the filename into quoted evidence or
  Citation authority. This prevents filename-addressed Office/PDF questions
  from losing the correct document before rerank without polluting Chunk source
  hashes or mutating an immutable Generation.
- Strategy selection is automatic and profile-versioned by canonical
  document/block type. The initial release exposes read-only diagnostics for
  the selected strategy and bounds, but no user-configurable chunk size,
  overlap, threshold, or per-collection profile override.
- Deterministically detected repeated page headers, footers, watermarks, and
  navigation boilerplate remain in parser-native artifacts and provenance but
  are marked `nonIndexable` for Child content, Embedding, and BM25. Detection
  must combine repeated text, page-position, and frequency evidence; it must
  not use an LLM or destructively delete source blocks.
- Undersized headings, paragraphs, list fragments, and captions merge only with
  structurally compatible adjacent content inside the same heading, slide,
  sheet, table, and reading flow. Headings follow their body, captions follow
  their owning asset/content, and list items stay in their list. Semantic score
  may break a tie between valid adjacent candidates but can never authorize a
  cross-boundary merge or exceed the Child hard token cap.
- Candidate activation requires strict absolute format- and language-sliced
  gates. PDF, text/Markdown/DOCX, PPTX, XLSX/table, JSON/code,
  Chinese/English, short fact, cross-section, and exact numeric cases must be
  represented. Citation/locator integrity is a 100% hard gate; every slice
  must meet its frozen absolute threshold while latency and context-token cost
  remain inside their explicit budgets. No Jina or legacy Active comparison is
  promotion evidence.
- The `1000ms` Retrieval P95 hard budget applies to steady-state evaluation.
  Cold-start observations must remain visible under a separate metric and may
  exceed `1000ms`; they are not Promotion failures unless a future versioned
  policy explicitly introduces a cold-start SLA. Existing reports remain
  immutable and retain the pass/fail semantics under which they were created.
- One frozen local tokenizer is bound to the Index/Chunk Profile with its name,
  revision, vocabulary/artifact hash, normalization, and special-token policy.
  All chat models share that persisted Chunk/Embedding projection. Query-time
  context assembly adapts the number of Children, expanded Parents, and Web
  sources to the selected answer model's context budget; changing chat models
  must not trigger knowledge reindexing.
- A full candidate rebuild starts from a frozen corpus snapshot while normal
  document uploads, replacements, reprocessing, and deletes remain available.
  Versioned Outbox changes after the snapshot are replayed into the candidate
  until it reaches the current corpus revision. Verification and activation
  must fail closed unless every current document/version, deletion, projection,
  and search payload is reconciled at one fenced revision.
- After activation, retain exactly one complete Last-Known-Good generation for
  immediate guarded rollback through a default seven-day observation window.
  A later successful activation may retire and purge older generations, but a
  user document/collection deletion must purge its payloads from active,
  candidate, Last-Known-Good, retired, and failed materializations immediately;
  rollback retention never overrides deletion authority.
- The promotion Golden corpus follows the existing accuracy-first contract:
  at least 500 human-reviewed questions frozen as 60% Development, 20%
  Validation, and 20% one-shot Holdout, with at least 50 cases in every critical
  slice. Absolute gates are Candidate Recall@50 >= 0.95, Final Recall@10 >=
  0.90, nDCG@10 >= 0.85, MRR@10 >= 0.80, Citation Correctness >= 0.95,
  Citation Completeness >= 0.90, Faithfulness >= 0.95, No-answer false-answer
  rate <= 2%, Table exact-answer >= 0.95, and 100% provenance/cell lineage with
  zero ACL, deletion, secret, or unauthorized-evidence leakage. The candidate
  must also meet the format/language no-regression gates above.
- Keep rollback as an active-generation pointer transition, not destructive
  rewriting of existing projections.
- Curate the first Golden Gate against a dedicated, reversible synthetic source
  collection before human review. The source corpus must contain exactly 50
  unique documents: 10 PDF, 10 DOCX, 10 PPTX, 10 XLSX, and 10 Markdown
  JSON/code documents, split evenly between Chinese and English. Every source
  must carry stable evidence anchors plus short-fact, cross-section, and exact
  numeric material. Synthetic source facts and generated questions start as
  draft-only and machine generation alone is never review evidence. A case may
  transition to `human_reviewed` only after a human explicitly checks its
  question, exact answer, bound source evidence, slices, and no-answer/table
  requirements; the transition must record the reviewer UUID and RFC3339
  timestamp and preserve the original draft artifact separately.

## Acceptance Criteria (evolving)

- [ ] Narrative, Markdown, HTML, JSON, code, table, formula, slide, and sheet
      fixtures select documented deterministic strategies.
- [ ] Every persisted Child respects the pinned tokenizer hard cap and every
      Parent/Child/overlap range maps to admitted source spans.
- [ ] No Parent crosses heading, reading-flow, slide, sheet, or document
      boundaries.
- [x] Candidate retrieval independently passes every frozen absolute Recall@k,
      ranking, answer-faithfulness, citation-correctness, latency, and context
      budget gate without querying or comparing against Jina.
- [ ] Candidate build, verification, activation, rollback, and cleanup replay
      from a clean database baseline.
- [x] A separate synthetic evaluation collection contains 50/50 successfully
      indexed, content-hash-distinct documents with 10 documents in each source
      lane (PDF, DOCX, PPTX, XLSX, Markdown JSON/code), 25 Chinese and 25
      English documents, and a replayable SHA-256 manifest. No case in the
      machine-generated seed queue has `review.state=human_reviewed`; a separate
      hash-bound derivative may carry that state only after recorded
      case-by-case human review.

## Definition of Done

- Tests added or updated for planner, parser mapping, projection, retrieval,
  generation rebuild, cutover, and rollback.
- Ruff, mypy, Python tests, Go vet/tests, security checks, and relevant live
  replay are green.
- The executable chunking and generation contracts are updated.
- Active runtime evidence proves the new profile hash and non-zero structural
  metadata/overlap on representative documents.

## Out of Scope (explicit)

- Physically deleting historical Jina Generation, projection, migration, or
  audit rows during the initial BGE-only cutover.
- Adding Mastra as an application framework dependency.
- In-place mutation of the current active generation.
- LLM-authored rewriting or summarization of source text as retrieval truth.
- Removing BM25, vector, rerank, provenance, or citation lanes.

## Technical Notes

- "Semantic boundary detection" means splitting one long unstructured block
  into sentences, embedding those sentences with the admitted embedding model,
  and placing boundaries at strong adjacent-topic changes. It does not ask a
  chat LLM to rewrite, summarize, or author source text. It is an optional
  ingestion-only enhancement and must fail open to deterministic
  sentence/recursive splitting.
- Existing baseline:
  `mm-chat/rag/src/mm_chat_rag/native_text_baseline.py` and
  `mm-chat/rag/src/mm_chat_rag/mineru_gateway.py`.
- Existing candidate planner:
  `mm-chat/rag/src/mm_chat_rag/structure_chunking.py`.
- Generation routing:
  `mm-chat/rag/src/mm_chat_rag/native_gateway.py`.
- Executable contract:
  `mm-chat/docs/contracts/rag-structure-chunking.md`.

## Research References

- [`research/mastra-neo-chat-chunking.md`](research/mastra-neo-chat-chunking.md)
  - Mastra strategy strengths mapped to the live and candidate Neo Chat paths.
