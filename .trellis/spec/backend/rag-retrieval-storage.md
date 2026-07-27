# RAG retrieval storage contracts

## 1. Scope / Trigger

Apply this contract when changing `mm-chat` lexical or Dense retrieval,
PostgreSQL extensions/major version, search projections, candidate diagnostics,
the production retrieval profile pointer, or the cross-layer Knowledge
Citation display projection. It also applies when touching any already-applied
retrieval migration byte, including comments, line endings, or terminal blank
lines, because the live manifest hashes both SQL directions byte-for-byte.

The current schema head is migration `050` on PostgreSQL `17.10` with
`pgvector 0.8.5` and `pg_textsearch 1.3.1`. The durable retrieval pointer still
accepts `legacy` and `pg17_bm25_pgvector_v1`; migration `049` adds the BGE
Candidate vector space, and migration `050` permanently retires Jina runtime
execution without moving the pointer, activating a Generation, or consuming
Holdout. The retired PostgreSQL 16 directory at
`mm-chat/data/postgres` remains an observation-window rollback anchor. Never
mount it, or any other PG16 data directory, into the PG17 image.

## 2. Signatures

Applied migration identity is immutable under the runner signature:

```text
SHA256(<version_name> || NUL || <up.sql bytes> || NUL || <down.sql bytes>)
```

For `050_jina_runtime_retirement`, the live checksum is
`87302c2cf0dee5ce11388795891db4e64dfba4a7086a9906bcbaeab5397519e6`.
Its down file deliberately retains the terminal blank line represented by that
manifest.

The retained private diagnostic signatures are:

```sql
knowledge_backfill_bm25_shadow(
  p_index_generation_id UUID,
  p_search_profile_id UUID
) RETURNS TABLE(
  eligible_count BIGINT,
  inserted_count BIGINT,
  verified_shadow_count BIGINT
)

knowledge_fetch_hybrid_shadow_diagnostics(
  p_collection_ids UUID[],
  p_query_text TEXT,
  p_query_embedding VECTOR(1024),
  p_limit INTEGER
) RETURNS TABLE(
  -- immutable UUID/hash references only
  bm25_rank INTEGER,
  bm25_score DOUBLE PRECISION,
  dense_rank INTEGER,
  dense_score DOUBLE PRECISION,
  fused_rank INTEGER,
  fused_score DOUBLE PRECISION
)
```

Production callers remain on the stable signature:

```sql
knowledge_fetch_profiled_query_evidence_candidates(
  UUID[], TEXT, REAL[], INTEGER
)
```

Migration `049` resolves the immutable provider tuple through:

```sql
knowledge_retrieval_profile_id(
  TEXT, TEXT, TEXT, INTEGER, TEXT, TEXT
) RETURNS TEXT

knowledge_resolve_generation_retrieval_profile(UUID)
RETURNS TABLE(
  index_generation_id UUID,
  search_profile_id UUID,
  retrieval_profile_id TEXT,
  provider_id TEXT,
  embedding_model_id TEXT,
  embedding_dimensions INTEGER,
  rerank_model_id TEXT
)

knowledge_resolve_active_retrieval_profile()
RETURNS TABLE(
  index_generation_id UUID,
  search_profile_id UUID,
  retrieval_profile_id TEXT,
  provider_id TEXT,
  embedding_model_id TEXT,
  embedding_dimensions INTEGER,
  rerank_model_id TEXT
)
```

The active resolver intentionally returns no row while the durable retrieval
pointer is `legacy`. Go treats that result as legacy compatibility, not as
permission to choose an arbitrary configured provider.

The cross-layer administration/status payload exposes only active providers:

```text
RAGProviderId = "mineru" | "siliconflow"

GET /v1/rag/provider-status
providers.siliconflow = {
  configured: boolean,
  status: "ready" | "missing_secret" |
          "activation_required" | "unavailable",
  embeddingDimensions: 1024
}

POST   /v1/admin/rag/providers/siliconflow/configure
DELETE /v1/admin/rag/providers/siliconflow
```

Chat completion applies a turn-scoped source-marker reconciliation boundary:

```text
reconcileProviderSourceMarkers(content, knowledgeDecision, webResult)
  -> completedContent

reconcileMessageKnowledgeContent(content, knowledgeMetadata)
  -> displayContent
```

`completedContent` is the only value used for completed-source authority,
assistant persistence, and the terminal SSE message. `displayContent` is a
frontend compatibility guard for messages persisted before this contract.

Final-authority Knowledge hydration returns the matched Child and its complete
containing Parent from one authorization boundary:

```sql
knowledge_reauthorize_and_hydrate_evidence(
  UUID, UUID, UUID, JSONB
) RETURNS TABLE(
  -- immutable authority references,
  source_text TEXT,
  child_token_count INTEGER,
  parent_source_text TEXT,
  parent_token_count INTEGER,
  locator JSONB
)
```

The Child text, hashes, and locator remain citation authority. Parent text is
source-backed answer context only and never replaces the matched Child.

The additive Knowledge Citation display projection is:

```text
RAGCitation {
  sourceName?: string                 // UTF-8, <= 512 bytes, no controls
  displayLocator?:
    | { kind: "page", page: integer }
    | { kind: "slide", slide: integer }
    | { kind: "cell_range", startCell: A1, endCell: A1 }
    | { kind: "line_range", startLine: integer, endLine: integer }
  locator: raw authority JSON         // retained, never rendered as fallback
}

normalizeMessageKnowledgeMetadata(untrustedMetadata)
  -> bounded KnowledgeCitation display fields or safe omission

formatKnowledgeCitationTitle(citation, localizedFallback, localizedLabels)
  -> "[K#] · <source>" plus an optional localized location
```

Display page, slide, and line coordinates are one-based integers in
`[1, 1_000_000_000]`. The durable raw locator, IDs, and hashes are unchanged.

Migration `037` introduced the durable pointer and routes that stable signature
to the legacy `knowledge_fetch_hybrid_query_evidence_candidates(...)`
implementation while the pointer is `legacy`. Migration `038` owns the
qualified PG17 branch; it may serve only after the exact readiness gate and
compare-and-swap activation succeed.

Profile mutation is operator-only and compare-and-swap:

```sql
knowledge_set_retrieval_profile(
  p_expected_profile TEXT,
  p_target_profile TEXT,
  p_expected_revision BIGINT,
  p_reason TEXT
) RETURNS TABLE(active_profile TEXT, revision BIGINT)
```

The accepted profile identities are `legacy` and
`pg17_bm25_pgvector_v1`; the singleton starts at `legacy` revision `1`.

Migration `038` adds:

```sql
knowledge_assert_pg17_retrieval_profile_ready()
RETURNS TABLE(
  index_generation_id UUID,
  search_profile_id UUID,
  eligible_count BIGINT,
  vector_count BIGINT,
  bm25_count BIGINT
)
```

Active-generation publication maintenance uses:

```sql
knowledge_sync_pg17_retrieval_materialization(
  p_materialization_id UUID
) RETURNS TABLE(
  eligible_count BIGINT,
  vector_inserted_count BIGINT,
  bm25_inserted_count BIGINT,
  verified_count BIGINT
)
```

Generation promotion/rollback readiness uses:

```sql
knowledge_assert_pg17_generation_ready(
  p_index_generation_id UUID
) RETURNS TABLE(
  index_generation_id UUID,
  search_profile_id UUID,
  document_count BIGINT,
  eligible_count BIGINT,
  vector_count BIGINT,
  bm25_count BIGINT
)
```

Go API document lifecycle calls cross the projection boundary only through:

```sql
knowledge_allocate_parse_materialization(UUID, UUID, UUID)
RETURNS TABLE(
  index_generation_id UUID,
  materialization_id UUID,
  legacy_projection_unbound BOOLEAN,
  max_attempts INTEGER
)

knowledge_is_document_version_actively_projected(UUID, UUID)
RETURNS BOOLEAN

knowledge_resolve_purge_projection_binding(UUID, UUID)
RETURNS TABLE(
  index_generation_id UUID,
  materialization_id UUID,
  legacy_projection_unbound BOOLEAN,
  max_attempts INTEGER
)
```

Structure-generation lifecycle state changes are operator-only after migration
`044`. `rag_replay_operator` receives bounded status, rebuild allocation,
verification, activation, and rollback gateways. `go_api_runtime` receives none
of the generation mutation functions. Verification changes only
`building/building -> verified/ready`; it does not activate the candidate.
Activation additionally requires the exact verified artifact manifest, a
SHA-256-bound Golden Gate report, an explicit operator UUID, and the current
head revision, then writes an immutable activation audit.

The frozen Holdout uses a separate explicit command mode:

```text
rag-capture \
  -execute-frozen-holdout \
  -promotion-golden <frozen.json> \
  -curation-queue <curation.json> \
  -human-review-receipt <review.json> \
  -source-import-receipt <import.json> \
  -candidate-generation-id <uuid> \
  -candidate-artifact-manifest-hash <sha256> \
  -development-preflight <complete-300.json> \
  -validation-preflight <complete-100.json> \
  -holdout-seal <new-exclusive-path> \
  -output <new-exclusive-path> \
  -answer-model <frozen-model-id>
```

It emits one `neo-chat.rag-promotion-holdout-seal.v1` before provider work and
one complete `neo-chat.rag-promotion-observations.v1` after all 100 Holdout
cases succeed. Neither artifact is overwriteable.

Supplemental no-answer regression uses a third, mutually exclusive command
mode and never consumes or modifies the frozen Holdout seal:

```text
rag-capture \
  -supplemental-no-answer <bound-50-case-suite.json> \
  -promotion-golden <frozen.json> \
  -curation-queue <curation.json> \
  -human-review-receipt <review.json> \
  -source-import-receipt <import.json> \
  -candidate-generation-id <uuid> \
  -candidate-artifact-manifest-hash <sha256> \
  -output <new-exclusive-path> \
  -answer-model <frozen-model-id>
```

It emits `neo-chat.rag-supplemental-no-answer-report.v1` with
`promotionEvidence=false`. The command writes a complete failing report before
returning a non-zero status, but never overwrites an existing path.

A failed latency-only supplemental report can seed a separate paired diagnostic
mode:

```text
rag-capture \
  -supplemental-no-answer <same-bound-50-case-suite.json> \
  -supplemental-latency-source-report <failed-exclusive-report.json> \
  -promotion-golden <same-frozen.json> \
  -curation-queue <same-curation.json> \
  -human-review-receipt <same-review.json> \
  -source-import-receipt <same-import.json> \
  -candidate-generation-id <same-uuid> \
  -candidate-artifact-manifest-hash <same-sha256> \
  -concurrency 4 \
  -output <new-exclusive-path> \
  -answer-model <same-frozen-model-id>
```

It emits
`neo-chat.rag-supplemental-no-answer-latency-diagnostic.v1` with
`promotionEvidence=false`. The source-report flag is invalid without
`-supplemental-no-answer`; the mode accepts neither Holdout inputs nor ordinary
preflight case selection. Its output is a new exclusive artifact and can never
replace the failed source report.

## 3. Contracts

- Never normalize, reformat, or trim an applied migration. If source drift is
  discovered, prove the exact applied bytes from repository/runtime evidence,
  restore those bytes, and pin the live checksum in a regression test. Do not
  rewrite `schema_migrations.checksum` to bless changed source.
- BM25 reader-source admission requires the active corpus head and generation,
  the exact historical or BGE Search Profile row, active document projection
  head, published materialization, and current collection/document/version
  visibility. A historical Jina tuple is lineage only and authorizes no
  provider call.
- `pg_textsearch <@>` returns a negative raw BM25 score: lower is better and
  `0` is no match. Filter `score < 0`; never interpret it as a probability.
- Use explicit `to_bm25query(query, index_name)` for the intended BM25 index.
- Latin text uses `simple` tokenization. Add at most 512 CJK ideograph bigrams;
  never generate generic Latin bigrams.
- Dense shadow vectors are `vector(1024)` and retain exact provider profile,
  generation, float32 hash, finite components, and non-zero norm. Migration
  `050` makes historical Jina Dense rows read-only and non-queryable.
- Migration `049` admits exactly two retrieval tuples:
  `jina_v4_v3 = jina/jina-embeddings-v4/1024/jina-reranker-v3` and
  `siliconflow_bge_m3_v1 = siliconflow/Pro/BAAI/bge-m3/1024/`
  `Pro/BAAI/bge-reranker-v2-m3`. Equal dimensions never authorize projection,
  query-vector, shadow-row, diagnostic, or rerank reuse across those spaces.
- Migration `050` supersedes the runtime admission half of that schema
  contract: `jina_v4_v3` remains decodable for history and same-Generation
  BM25/Citation fencing, but only `siliconflow_bge_m3_v1` is executable.
- SiliconFlow uses fixed endpoints `https://api.siliconflow.cn/v1/embeddings`
  and `https://api.siliconflow.cn/v1/rerank`, `float` embeddings, no
  instruction/task field, `return_documents=false`, and `top_n` equal to the
  input document count. Its live Rerank response may still include an explicit
  `document:null` field. The strict decoder admits only an absent or JSON-null
  `document`; any returned source body remains invalid because the request
  forbids documents. Credentials exist only in the `RAG:SILICONFLOW` Vault
  record and never in Generation/Profile metadata.
- Passage Embedding is selected from the immutable BGE Candidate Generation.
  Query Embedding executes only for an active BGE Generation/Search Profile.
  A legacy or Jina binding uses fenced BM25 and never calls an Embedding or
  Rerank provider. BGE Rerank remains bound to the Generation/Search Profile
  carried by evidence; it must not re-read a later Active Profile.
- The stable reader preserves the migration-048 legacy lexical implementation
  while the pointer is `legacy`; its provider-backed hybrid branch is disabled
  by the unbound gateway. Under the PG17 pointer, lexical and BGE hybrid reads
  are fenced by pointer, active Generation, and Search Profile before and
  after the decisive read.
- If activation races after Query Embedding, discard that vector, resolve the
  new Active Generation/Profile, and retry the complete bind/embed/read flow at
  most once. Never send the old vector into the new profile.
- Embedding, Rerank, governance, or provider availability failures may degrade
  only to BM25 fenced to the same Generation/Search Profile. Jina is never a
  fallback, and BGE may never execute against a Jina Generation.
- Active diagnostics are Generation/Profile-bound and may not probe a second
  vector space. Historical Jina diagnostics are offline-only. A retrieval-
  pointer rollback makes a previously captured PG17 binding stale and must
  return `RAG_RETRIEVAL_PROFILE_CHANGED`.
- Fuse BM25/Dense and original/rewrite lanes with deterministic
  `1 / (60 + lane_rank)` RRF plus stable UUID tie-breaks.
- Preserve an explicit authorized source-name route after BGE Rerank. Policy
  `g18-profiled-reranker-golden-v2` normalizes the bounded filename basename
  with the migration-048 key, requires that complete key in the original or
  rewritten query, and adds `2.0` after the provider's validated `[0,1]`
  relevance score. BGE order remains authoritative among Children from the
  same named document. Filename metadata never becomes source text, quoted
  evidence, or Citation authority. Candidate capture must use the identical
  rule, never an evaluator-only repair.
- Candidate indexes are not authorization authorities. Rejoin current
  authority before diagnostics, then reauthorize/hydrate again in Go before
  answer context or citations.
- Rank and cite exact Child chunks before considering Parent expansion. Admit
  the top Parent and only further Parents whose score is at least `0.85` of the
  top Child score, deduplicate by Parent ID, and expand at most two Parents.
  Parent context never changes the Child hashes, locator, or citation marker.
- Knowledge and Web evidence share one turn-local, selected-model input budget.
  Reserve 512 tokens after current prompts/messages, cap retrieval at 40% of
  the model input budget, and split mixed-source retrieval 60% Knowledge / 40%
  Web. If only one lane is available, it may use the complete retrieval budget;
  neither lane may independently fill the model window.
- Knowledge/Web markers are minted, turn-scoped capabilities rather than model
  prose. The completion allowlist comes only from the current turn's hydrated
  Knowledge citations and bounded Web citations. Remove every unissued
  `[K<number>]` or `[W<number>]` before authority reconciliation, persistence,
  and the terminal SSE event.
- Strip reserved source markers from historical assistant messages before
  sending conversation context to a provider. This preserves the answer text
  while preventing a model from copying a previous turn's citation capability.
- The frontend must derive visible Knowledge markers and citation cards from
  the current message's authoritative `metadata.knowledge.citations`, not from
  marker-looking prose. It may remove unissued markers while mapping old
  persisted messages, but it must not invent citation metadata.
- Citation minting may project `sourceName` only from the same reauthorized
  `HydratedEvidence` used to mint authority. It must unwrap only
  `g7.4-locator-summary.v1.primary.locator`, require matching kinds, convert
  zero-based page/slide/line coordinates to one-based display coordinates,
  uppercase bounded A1 cells, and omit unsupported or malformed locations.
- `page_bbox`, `slide_shape`, `sheet_cell`, and `line_range` map to the typed
  display DTO. Opaque sheet hashes, `ooxml_part_xpath`, `text_offset`, XPath
  payload references, raw locator JSON, UUIDs, and hashes must never become a
  visible fallback. Legacy top-level human-readable page/sheet/cell/section
  fields may still render after bounded validation.
- The frontend must normalize Citation metadata as untrusted input and render
  source names as ordinary React text. A missing or rejected `sourceName`
  becomes the localized generic Knowledge-source label; a rejected location
  is omitted without dropping the underlying Citation authority.
- Resolve bounded BM25/Dense probe IDs through a candidate-driven `LATERAL`
  current-authority lookup. Do not let PostgreSQL decorrelate the authority
  view into a corpus-wide or per-candidate repeated expansion; retain an
  optimizer fence such as `OFFSET 0` and prove it with representative latency.
- The BM25 build/active source views are internal accelerator authorities,
  owned and directly readable only by `rag_projection_owner`. They do not use
  `security_barrier`, because it blocks child-ID predicate pushdown and caused
  corpus-wide materialization. This exception is valid only while `PUBLIC` and
  runtime roles have no direct `SELECT`, the SECURITY DEFINER reader returns
  references only, and Go performs final hydration reauthorization.
- Shadow diagnostics may expose UUIDs, hashes, ranks, and scores only. Do not
  output source text, exact terms, provider credentials, or private queries.
- Only `rag_replay_operator` receives shadow EXECUTE. Production runtime roles
  receive no shadow table/function privileges.
- The retrieval profile head is a database-owned singleton with a monotonic
  revision. Profile mutations use compare-and-swap over expected profile and
  revision, append immutable transition history, and are executable only by
  `rag_replay_operator`.
- A profile target whose schema or verified backfill is unavailable must raise
  `RAG_RETRIEVAL_PROFILE_UNAVAILABLE` without mutating pointer state.
- After migration `050`, PG17 activation must bind the active generation to
  its unique SiliconFlow BGE Search Profile and re-verify every current source
  row against both physical
  projections. Matching counts alone are insufficient; identity, hashes,
  visibility revisions, vector round-trip, normalized terms, and derived BM25
  text must agree.
- Serialize vector backfill, BM25 backfill, and pointer activation with
  advisory locks `3`, `4`, and `5` acquired in ascending order. Readiness and
  pointer mutation occur inside the same activation call.
- Migration `038` is the production schema boundary. It requires PostgreSQL
  major `17`, the `pg_textsearch` preload, and exact pgvector `0.8.5` /
  `pg_textsearch 1.3.1` availability before creating extensions or retrieval
  objects. The complete migration manifest is no longer compatible with an
  ordinary PG16 database.
- `schema_migrations.version` is textual. Operator head checks must order or
  aggregate it as `version::INTEGER`; lexical descending order incorrectly
  reports `9` above `38`.
- When the PG17 profile is active, the AFTER trigger on
  `knowledge_document_projection_heads` is the publication boundary. It must
  populate and verify both projections in the same transaction as the head
  mutation; partial projection success or a query-visible unsynchronized head
  is forbidden.
- When the pointer is `legacy`, the maintenance trigger is a no-op. Before
  later PG17 activation, readiness/backfill must close any rows published in
  that interval.
- Direct materialization sync is operator-only, idempotent, and locks vector
  then BM25 (`3 -> 4`). Runtime publication reaches it only through the hardened
  projection-owner trigger.
- Separate write admission from read authority. The BM25 build source may admit
  current published heads from `building`, `verified`, `active`, or `retired`
  generations, matching the pgvector build source. The BM25 reader source must
  remain joined to the singleton active corpus head.
- While the PG17 profile is active, every corpus-head generation change must
  acquire advisory locks `3 -> 4` and verify complete current-document
  coverage, paired BM25/vector source identity, and exact physical projection
  coverage for the target generation. After migration `050`, the target must
  resolve to the admitted SiliconFlow BGE profile; historical Jina generations
  cannot be reactivated by either promotion or rollback. The fence aborts the
  surrounding transaction on failure.
- Migration rollback must fail atomically with
  `RAG_RETRIEVAL_PROFILE_ROLLBACK_REQUIRES_LEGACY` unless the active pointer is
  `legacy`. Application rollback precedes migration rollback.
- Pre-cutover resource qualification uses a hard 1 GiB / 2 CPU PG17 container,
  a 4096-child active-generation publication, 30 production-shaped profiled
  reads, and these gates: backfill `<= 120s`, query P95 `<= 500ms`, query max
  `<= 1000ms`, combined vector/BM25 physical storage `<= 512MiB`, and cgroup
  memory peak `<= 900MiB`.
- A qualified active-PG17 logical backup must be checksummed, restored into a
  fresh `template0` database, and re-verified for migration idempotence,
  profile revision, active/physical row counts, runtime reader behavior,
  operational functions, and role grants.
- The historical pre-`050` migration-038 qualification must remove unused
  preinstalled retrieval extensions from the isolated restored target, prove
  migration `038` creates
  the exact versions through the normal runner, backfill only current Jina
  v4/1024 authority, and exercise active-profile down refusal plus controlled
  down/re-up. Down retains both extensions, migration `037`, profile history,
  legacy `REAL[]` rows, the original PG16 data path, and its backup.
- Production Compose must use an immutable `POSTGRES_IMAGE` digest and
  `POSTGRES_DATA_DIR=./data/postgres17`. Local Compose may build the reviewed
  image from `mm-chat/postgres`; the production overlay must remove that build
  path. Preflight rejects the retired `./data/postgres` path.
- The production image must fail before PostgreSQL startup when `PG_VERSION`
  is not `17`. A major-version transition is logical backup/restore into fresh
  storage, never an in-place mount or downgrade.
- The Go API must connect as a dedicated `neo_chat_api` LOGIN that is not a
  superuser and inherits only `go_api_runtime`. It may execute the profiled
  reference-only reader but may not directly select either physical retrieval
  projection. The API must never use the bootstrap/migration owner at runtime.
- Migration `039` owns the Go API projection gateway. Upload, replacement,
  reprocess, and deletion code must not read or mutate corpus heads,
  generations, profiles, materializations, or document projection heads
  directly. The allocation gateway accepts only materialization/document/
  version UUIDs and derives file, hash, collection revisions, visibility
  epochs, generation, profile hash, and sequence under the projection owner.
- The token-fenced Go source-object HTTP endpoint uses the API database login,
  not the Python Worker's database login. Migration `040` therefore grants
  `go_api_runtime` EXECUTE on the existing hardened
  `knowledge_fetch_parse_source_metadata(...)` function. It must not grant the
  API any new relation privilege; migration `040` is function-only.
- Migration `041` hardens every SECURITY DEFINER function in the current
  application schema, including functions created before the dedicated runtime
  boundary was enforced. Each function must retain its owner and grants while
  pinning `search_path` to the current schema, `pg_catalog`, and `pg_temp`.
  Rollback must not restore `"$user", public`.
- Migration `043` extends the existing final-authority hydration function with
  complete Child/Parent text and token counts. It does not broaden candidate
  authorization, and all returned source bodies remain bounded to 64 KiB.
- Migration `044` registers the immutable Structure Chunk Profile v2 descriptor,
  revokes raw generation mutation from `go_api_runtime`, grants only the bounded
  lifecycle gateways to `rag_replay_operator`, and records explicit activation
  approval in immutable audit state. Candidate verification must remain
  non-activating; the first activation is forbidden until the frozen 500-case
  Golden Gate report passes and an operator issues a separate activation.
- Migration `045` makes `knowledge_child_search_projections.locator_summary`
  the exact matched-Child Citation locator authority. The verifier validates
  Parent and Child Search locators independently, promotion inherits that
  verifier fence, rollback checks every target Child, and hydration rejoins the
  ready Search row through generation, materialization, document, hashes, and
  Search Profile before returning its locator. Parent locator equality is not
  a valid integrity condition because one Parent may contain several narrower
  Children.
- Migration `046` removes direct `rag_replay_operator` access to
  `knowledge_fail_structure_generation(...)`. Verified Candidate abandonment
  is available only through the exact Candidate/manifest/head compare-and-swap
  gateway, with a bounded reason, explicit operator UUID, fixed
  `OPERATOR_ABANDONED` failure code, and immutable audit. The CLI is dry-run by
  default and requires `--confirm-abandon --execute`; the API runtime receives
  neither failure function.
- Migration `049` registers Structure Chunk Profile v3 plus the isolated
  SiliconFlow Pro BGE Search Profile contract. It leaves Jina Active and makes
  no automatic Candidate activation, Holdout execution, or corpus-head
  mutation. Its down migration succeeds only from a clean Jina baseline and
  fails atomically with `RAG_SILICONFLOW_ROLLBACK_REQUIRES_BGE_PURGE` while
  any BGE Index/Search Profile or physical BGE projection remains.
- Migration `049` derives auditable SiliconFlow consent rows from current Jina
  consent authority without rewriting the source rows. `decided_at`,
  `created_at`, and `updated_at` use the same `CURRENT_TIMESTAMP`; separate
  `clock_timestamp()` evaluations can make `decided_at < created_at` and are
  forbidden in this backfill.
- Migration `050` clears and soft-deletes the `RAG:JINA` provider record,
  deletes the `jina-web-reader` registry row, leaves the current historical
  Jina Active Generation unchanged for BM25-only service, and rejects every
  later insert/update transition into Jina Active. Its down migration is
  intentionally irreversible and must fail atomically.
- Migration `050` also wraps every runtime vector-consuming hybrid, Candidate
  evaluation, and operator diagnostic SQL function with an exact BGE tuple
  check. The stable hybrid compatibility signature routes a historical Active
  binding to its lexical reader without consuming the supplied vector. Only the
  lexical function accepts a historical Jina binding.
- Plugin registries, executors, redirects, provider credential resolution,
  capture tools, settings UI, and deployment status permanently reject Jina
  before secret access or network. Historical schema/evidence decoders do not
  authorize execution.
- The settings UI and deployment health view enumerate MinerU and SiliconFlow
  only. Older or partially rolled payloads are read defensively so one absent
  provider key does not crash settings.
- Native structural locator views outrank generic text positions during
  projection. Use page, sheet, slide, OOXML path, then line fallback order.
  PPTX text must resolve through Shape/Slide to `slide_shape`; XLSX rows must
  retain Cell/Sheet A1 ranges and project them to `sheet_cell`.
- Live MinerU tables may nest `table_body` blocks whose only cell authority is
  `span.type=table` HTML. Extract bounded `th`/`td` character data with a
  non-executing parser, emit deterministic escaped rows, and reject malformed
  or empty table state. Never retain or execute provider HTML.
- Stop every Candidate-capable Worker before Candidate allocation, deploy one
  pinned image, call `generation-begin`, and then start only that image. A
  Candidate claimed by multiple Worker image revisions is not certifiable; use
  audited abandonment and rebuild a new sequence rather than editing rows.
- Promotion evidence uses closed `neo-chat.rag-promotion-golden.v1` and one
  Candidate `neo-chat.rag-promotion-observations.v1` input to emit
  `neo-chat-rag-candidate-gate-report.v2`. Admission requires at least
  500 individually human-reviewed cases, an exact `60/20/20` split, at least
  50 cases in every critical slice and table-exact cohort, reviewer UUIDs and
  RFC3339 timestamps preceding freeze, and a matching canonical frozen-content
  SHA-256. Candidate observations bind that exact corpus and the verified
  Candidate manifest and use one precommitted `ordinal=1` Holdout run. Jina or
  Active comparisons, relative improvement, and per-slice no-regression do not
  participate. Gate-report v1 is historical only. Draft cases, hash drift, a
  repeated Holdout, and reports missing complete evaluator/input-hash
  provenance are never promotion evidence.
- Candidate-only absolute gates apply identically to Development, Validation,
  Holdout, aggregate results, and every critical slice: Recall@50 `0.95`, Final
  Recall@10 `0.90`, nDCG@10 `0.85`, MRR@10 `0.80`, Citation Correctness
  `0.95`, Citation Completeness `0.90`, Faithfulness `0.95`, Answer Correctness
  `0.95`, Table exact answer `0.95`, no-answer false-answer rate at most `0.02`,
  complete locator/provenance/cell lineage, zero leakage, and frozen budgets.
  `minimumAggregateQualityImprovement` remains hash-compatible but is ignored.
- Development/Validation preflight capture is serializable and atomic. Capture
  schema `v2`, capture version `v6`, uses 26 provider attempts, 500ms initial
  backoff, a 60-second backoff cap, and a 120-minute command deadline so a
  transient BGE cooldown can recover without increasing concurrency. It is
  Candidate-only and cannot execute Holdout. Exhaustion,
  cancellation, or deadline produces no partial evidence file and never
  authorizes Holdout or Activation.
- Frozen Holdout execution is an explicit, fail-closed one-shot path. Before
  sealing, it recomputes Development/Validation metrics and validates exact
  case order/coverage, raw and content hashes, BGE tuple, Candidate Generation,
  manifest, Chunk Profile, answer model, scoring policy, live head/corpus
  revisions, and the precommitted run UUID. It then writes an exclusive
  `execution_started`, `ordinal=1` seal before the first provider request,
  captures only the 100 Holdout cases, merges all 500 cases in Golden order,
  and publishes observations atomically without replacement. Any post-seal
  failure consumes the one shot; deleting or replacing the seal to retry is
  forbidden. A passing evaluator report still cannot activate the Candidate.
- Supplemental no-answer capture is diagnostic only. The suite contains
  exactly 50 synthetic absent-source questions: 25 Chinese, 25 English, and 10
  each for PDF, DOCX, PPTX, XLSX, and Markdown JSON/code. Every case carries a
  unique absent filename and subject token present in its query but absent from
  the imported source receipt. The suite binds the exact frozen Golden hashes,
  review/import hashes, collection, Candidate/manifest/Chunk Profile, BGE
  profile, answer model, and live head/corpus revisions before provider work.
  Aggregate false-answer rate is at most `0.02`; every language and format
  slice requires zero false answers. Citation evidence, `[K#]` markers, absent
  filename/subject context matches, and authority/secret leakage must all be
  zero. P95 retrieval latency is at most 1000ms and average context at most
  4096 tokens. The report stores only answer SHA-256, not answer text.
- Paired supplemental latency diagnosis is diagnostic only and uses selection
  policy `first-two-chinese-cases-across-pdf-docx-pptx-xlsx-v1`. In the same
  newly started helper process, it first runs the first Chinese PDF, DOCX,
  PPTX, and XLSX cases concurrently as the four-case `cold` phase. Only after
  that phase fully completes may it run the corresponding second cases as the
  four-case `warm` phase. Case and phase order are stable. Concurrency must be
  exactly four.
- Before any diagnostic provider work, reload and strictly recompute the source
  supplemental report from its 50 observations. Reject unknown fields,
  malformed observations, changed summary/slices/failures/pass state, a report
  that passed, a non-latency failure, P95 at or below the frozen budget, or any
  drift in suite hash, Candidate/manifest/Chunk Profile, BGE tuple, answer
  model, concurrency, scoring policy, criteria, or live head/corpus revision.
  Record the source file's computed SHA-256 in the diagnostic.
- `diagnosticIntegrityPassed` ignores only the frozen P95 latency failure when
  recomputing each phase. Any false answer, Citation evidence or marker,
  absent-source/subject match, context-budget breach, or authority/secret leak
  still fails integrity. `cold_start_effect_observed` requires Cold P95 above
  1000ms, Warm P95 at or below 1000ms, and Warm P95 below Cold P95. A valid
  diagnostic measures process-state sensitivity; it neither changes the
  canonical source report nor authorizes Holdout, Promotion, or Activation.
- The owner-approved latency policy treats `1000ms` as the steady-state
  Retrieval P95 hard budget. New-process Cold P95 remains mandatory diagnostic
  telemetry but may exceed `1000ms` and has no hard Promotion threshold in this
  version. Do not hide Cold observations, silently warm the runner, or
  retroactively change a report produced under the original criteria. A future
  cold-start SLA requires a new versioned policy/schema and independent review.
- Machine-generated cases must first exist as an immutable draft seed. A
  separate hash-bound derivative may set `human_reviewed` only after explicit
  case-by-case human checks of question, exact answer, source evidence, slices,
  and table/no-answer requirements. Preserve the seed and bind the derivative
  to a reviewer UUID, RFC3339 timestamp, and review receipt; generation success
  or schema validation alone never authorizes the transition.
- Production rollback first stops writers, restores the previous Compose/env
  authority, and starts PG16 against the preserved `data/postgres` directory or
  a fresh PG16 restore. Do not run migration `038` down as a substitute for a
  database-major rollback, and do not delete either data path or the final
  checksummed backup during the observation window.

## 4. Validation & Error Matrix

| Condition                                                                                  | Required result                                                                                                         |
| ------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------- |
| Embedded bytes for an applied migration differ from `schema_migrations.checksum`           | Deployment fails before later migrations; restore the exact applied bytes rather than editing the manifest             |
| PG major is not 17 or extension version differs                                            | DDL aborts before creating shadow objects                                                                               |
| Generation/profile is null, inactive, or incompatible                                      | `RAG_BM25_SHADOW_ARGUMENT_INVALID` or `RAG_BM25_SHADOW_PROFILE_MISMATCH`                                                |
| Insert identity does not match current source                                              | `RAG_BM25_SHADOW_SOURCE_MISMATCH`                                                                                       |
| Derived BM25 text/terms differ                                                             | `RAG_BM25_SHADOW_CONTENT_MISMATCH`                                                                                      |
| Backfill postcondition count differs                                                       | `RAG_BM25_SHADOW_BACKFILL_INCOMPLETE`                                                                                   |
| Collections/query/limit/vector invalid or vector norm is zero                              | `RAG_HYBRID_SHADOW_ARGUMENT_INVALID`                                                                                    |
| Document/collection becomes non-current                                                    | Candidate disappears immediately; immutable rollback row may remain                                                     |
| Reranker is configured but unavailable/unauthorized/malformed                              | No Knowledge evidence or citation is minted                                                                             |
| Query explicitly names one authorized filename basename                                    | Apply the versioned post-Rerank source-name boost; retain BGE order within that document                                |
| Filename is absent, generic/short, malformed, or only resembles a source                   | Apply no metadata boost; preserve ordinary BGE score order                                                              |
| Current turn has no evidence but model emits or copies `[K1]`                              | Marker is removed; `no_evidence`, `citationCount=0`, and no citation card remain                                        |
| Model emits a Knowledge/Web marker not present in the current turn allowlist               | Unissued marker is removed before authority, persistence, and terminal SSE                                              |
| Current turn emits a marker backed by current authoritative citation metadata              | Marker and matching citation metadata remain unchanged                                                                  |
| Authorized Citation has a valid source name and supported canonical locator               | Persist the bounded `sourceName` and typed one-based `displayLocator`; retain raw authority unchanged                   |
| Source name, schema, kind, coordinate, A1 range, or locator structure is invalid           | Omit the unsafe display field; keep the authoritative Citation and use the localized generic/source-only display        |
| Citation is old or uses OOXML/text-offset/unknown/opaque locator data                       | Show the safe filename or generic Knowledge-source label only; never stringify JSON or display UUID/hash fallbacks      |
| Profile compare-and-swap sees a stale expected profile/revision                            | `RAG_RETRIEVAL_PROFILE_CONFLICT`; pointer/history unchanged                                                             |
| PG17 profile is selected before its implementation is available                            | `RAG_RETRIEVAL_PROFILE_UNAVAILABLE`; pointer unchanged                                                                  |
| Active generation/profile is missing at PG17 activation                                    | `RAG_RETRIEVAL_PROFILE_ACTIVE_GENERATION_MISSING`; pointer unchanged                                                    |
| Either PG17 projection is incomplete or mismatched                                         | `RAG_RETRIEVAL_PROFILE_BACKFILL_INCOMPLETE`; pointer/history unchanged                                                  |
| Active-profile materialization source is absent/incomplete                                 | `RAG_RETRIEVAL_MATERIALIZATION_SOURCE_INCOMPLETE`; head/publish transaction aborts                                      |
| Post-insert vector or BM25 verification is incomplete                                      | `RAG_RETRIEVAL_MATERIALIZATION_SYNC_INCOMPLETE`; both projection writes and head mutation roll back                     |
| Target generation lacks any current document, paired source, vector, or BM25 row           | `RAG_RETRIEVAL_GENERATION_BACKFILL_INCOMPLETE`; generation/head transition rolls back atomically                        |
| Migration down is attempted under a non-legacy profile                                     | Rollback aborts atomically and migration `037` remains applied                                                          |
| Any representative backfill/latency/storage/memory gate is exceeded                        | The disposable qualification aborts; migration `038` remains forbidden                                                  |
| Restored PG17 state loses profile, rows, reader behavior, functions, or grants             | Restore qualification aborts; no Compose/data-path cutover                                                              |
| Production preflight receives a mutable PostgreSQL image or the PG16 data path             | Preflight fails before Compose execution                                                                                |
| Go API connects as the owner/superuser or gains direct projection access                   | Promotion/observation verification fails; traffic must not remain promoted                                              |
| API document lifecycle bypasses the `039` gateway                                          | Runtime role receives permission denial; do not grant tables, move the operation behind the gateway                     |
| Go source-object endpoint lacks the `040` function grant                                   | Parse retries with `GO_SOURCE_OBJECT_GATEWAY_REQUEST_FAILED`; add the narrow EXECUTE grant only                         |
| Hydration cannot match current Child/Parent authority or either body exceeds 64 KiB        | The reference is omitted; no Knowledge citation or Parent context is minted                                             |
| Parent score is below `0.85` of the top hit, is duplicated, or exceeds the shared budget   | Keep the matched Child only; do not expand that Parent                                                                  |
| Candidate verification succeeds but no explicit activation command is issued               | Candidate remains `verified/ready`; active generation and head revision remain unchanged                                |
| API runtime attempts generation begin/verify/fail/promote/rollback                         | Permission denied; do not restore runtime EXECUTE                                                                       |
| Activation gate hash, operator UUID, manifest, or head revision is invalid/stale           | Activation aborts atomically and no activation audit is written                                                         |
| Child Search locator is malformed, stale, missing, or substituted                          | Verification/rollback/hydration fails closed; no Citation is minted                                                     |
| PPTX/XLSX projection selects an XML line while a valid shape/cell view exists              | Reject the regression; structural locator authority must win                                                            |
| More than one Worker image revision claims jobs for one Candidate                          | Audited abandonment and a new Candidate sequence; never certify or repair it in place                                   |
| Nested MinerU table HTML is malformed, empty, or loses a text-bearing cell                 | `MINERU_STRUCTURE_ARTIFACT_INVALID`; no partial table projection                                                        |
| Active resolver returns no row while the pointer is `legacy`                               | Continue through migration-048 fenced BM25 only; make zero Jina provider calls                                          |
| Active Generation/Search Profile changes after Query Embedding                             | `RAG_RETRIEVAL_PROFILE_CHANGED`; discard the vector and rebind/retry the complete flow once                             |
| Binding changes again during the single retry                                              | Return no mixed-profile evidence; do not retry indefinitely or switch Embedding models                                  |
| SiliconFlow Embedding/Rerank is unavailable or its response is malformed                   | Use same-Generation/Profile fenced BM25 when admitted; otherwise fail closed with no Citation                           |
| A BGE vector is presented with a Jina binding, or vice versa                               | Reject as an invalid retrieval binding even when both vectors contain 1024 components                                   |
| Migration `049` down sees any BGE Index/Search Profile or physical BGE row                 | `RAG_SILICONFLOW_ROLLBACK_REQUIRES_BGE_PURGE`; migration head and every artifact remain unchanged                       |
| SiliconFlow API Key is absent, untested, or cannot be decrypted                            | `missing_secret`, `activation_required`, or `unavailable`; never return or log the credential                           |
| Stored Jina consent rows exist during `049` up                                             | Derive SiliconFlow rows with one stable timestamp; satisfy decided/created/updated ordering                             |
| Jina plugin ID/hostname/provider credential is requested after `050`                       | Reject before decryption/network; never restore a credential or executable adapter                                      |
| Retired/building/verified Jina Generation attempts to become Active                        | `RAG_RETIRED_RETRIEVAL_GENERATION_ACTIVATION_FORBIDDEN`; pointer and Candidate remain unchanged                         |
| Migration `050` down is requested                                                          | `RAG_JINA_RUNTIME_RETIREMENT_IS_IRREVERSIBLE`; migration row and retirement fences remain applied                       |
| Development/Validation report is partial, reordered, stale, drifted, or fails a gate       | Reject before creating the Holdout seal or making a Holdout provider request                                            |
| Holdout output or exclusive seal path already exists                                       | Reject permanently before runtime/provider initialization; never overwrite either artifact                              |
| Holdout capture fails after the exclusive seal is durable                                  | Produce no observation output; retain the seal and forbid retry or ordinal increment                                    |
| Holdout passes but no separately approved activation command exists                        | Keep Candidate `verified/ready`, Active pointer unchanged, and activation audit count unchanged                         |
| Supplemental suite is not 50/25/25/10-per-format or an absent filename exists in import    | Reject before provider work; write no report                                                                            |
| Supplemental binding, BGE profile, Candidate, model, or revision differs from live state   | Reject before provider work; do not touch Holdout or Activation                                                         |
| Supplemental output path already exists                                                    | Reject without replacement                                                                                              |
| Supplemental correctness, citation, leakage, latency, or context gate fails                | Publish the complete non-promotional report exclusively, return non-zero, and leave lifecycle state unchanged           |
| Supplemental latency source flag is supplied without the same suite mode                   | Reject before runtime/provider initialization                                                                           |
| Latency source report passed, has non-latency failures, was tampered, or drifts live state | Reject before either phase; write no diagnostic                                                                         |
| Latency diagnostic output path already exists                                              | Reject without replacement; preserve both the source report and existing diagnostic                                     |
| Cold/Warm phase has any non-latency correctness, Citation, context, or leakage failure     | Publish the exclusive diagnostic with `diagnosticIntegrityPassed=false`, return non-zero, and change no lifecycle state |

Every SECURITY DEFINER function must pin the current schema followed by
`pg_catalog, pg_temp` and must not resolve through `$user`.

## 5. Good / Base / Bad Cases

- **Good:** runtime/repository evidence proves a checksum mismatch is one
  terminal blank line, the exact applied blob is restored, and a checksum test
  prevents later normalization.
- **Bad:** updating `schema_migrations.checksum`, skipping validation, or
  replaying later migrations while the embedded source still differs.
- **Good (historical qualification only):** a published Jina v4 row retains
  immutable lineage and tombstone behavior without authorizing new provider
  calls.
- **Base:** unrelated weather/cooking queries return zero candidates in both
  lexical and Dense lanes; ordinary Model/Web answering may continue without a
  Knowledge citation.
- **Good:** a grounded turn retains its issued `[K1]`; a later unrelated turn in
  the same conversation receives the prior answer text without reserved
  markers and persists no citation-looking prose or card.
- **Base:** an old persisted `no_evidence` message containing a false `[K1]` is
  rendered without that marker by the frontend compatibility guard.
- **Good:** a new PDF Citation renders `[K1] · source.pdf · Page 3`; PPTX,
  XLSX, and line-based evidence render localized slide, cell/range, or
  line/range labels while the raw locator remains available only as authority.
- **Base:** a DOCX or old Citation has no safe display locator or source name,
  so the card renders only the filename or localized `Knowledge source` label.
- **Bad:** deriving a filename from a UUID, fetching document metadata once per
  Citation, branching on file extensions, or using `JSON.stringify(locator)`
  as a visible fallback.
- **Bad:** mixing Jina and BGE vectors because both have 1024 dimensions,
  mounting a PG16 directory into PG17, accepting BM25 score `0`, granting a
  shadow diagnostic to `go_api_runtime`, emitting source text in a report, or
  joining a bounded probe to an authority view that expands the entire corpus.
- **Bad:** treating any `[K1]` found in model prose or conversation history as
  proof that current Knowledge evidence was used.
- **Good:** the matched Child is rendered and cited first; one high-confidence
  deduplicated Parent is added as context while the marker still resolves to the
  Child locator and hashes.
- **Base:** Knowledge and Web are both available, so their rendered bodies stay
  inside the shared 60/40 allocation after the 512-token reserve.
- **Bad:** attaching every Parent, citing Parent prose instead of the matched
  Child, or giving Knowledge and Web independent full-window budgets.
- **Good:** `neo_chat_api` binds a document through the allocation gateway,
  the Go source endpoint resolves metadata through the token-and-lease-fenced
  function, and the Worker publishes without direct API table privileges.
- **Base:** no active generation returns a legacy/unbound binding with eight
  attempts and performs no projection write.
- **Bad:** restoring upload by granting `go_api_runtime` SELECT/INSERT on
  projection tables or by switching `DATABASE_URL` back to the migrator.
- **Good:** stop all Candidate Workers, pin one image, allocate sequence `N`,
  start that image, and verify native shape/cell plus MinerU table evidence.
- **Base:** a Candidate built by one pinned image remains non-active after
  deterministic verification and awaits reviewed Golden evidence.
- **Bad:** allocate while an old Worker is still running, then certify the
  resulting mixed-image Candidate because its jobs and manifest are stable.
- **Good:** Active BGE evidence resolves `siliconflow_bge_m3_v1`, embeds the
  query with `Pro/BAAI/bge-m3`, searches only that Generation/Profile, and
  reranks the resulting immutable evidence with
  `Pro/BAAI/bge-reranker-v2-m3`.
- **Base:** the pointer remains `legacy`; the active-profile resolver returns
  no row and migration-048 lexical retrieval remains live while its unbound
  provider gateway fails closed, producing BM25-only service.
- **Base:** SiliconFlow Query Embedding fails, so only the BM25 lane for that
  same BGE Generation/Profile may return bounded references.
- **Bad:** treating a configured SiliconFlow Vault record as activation,
  searching an active Jina projection with a BGE query vector, reranking
  evidence with a newly activated Profile, or executing Holdout automatically.
- **Good:** validate complete Development/Validation v6 reports against live
  Candidate/revision state, create one exclusive seal, capture exactly 100
  Holdout cases, publish one ordered 500-case observation set, and stop before
  activation.
- **Base:** a passing formal report remains review evidence while Candidate 8
  stays `verified/ready` and Active remains unchanged.
- **Bad:** passing `-splits holdout` to ordinary preflight, creating the seal
  after provider work, overwriting a seal/output, retrying after a sealed
  failure, or treating evaluator success as activation approval.
- **Good:** run one Candidate-bound supplemental suite, preserve its complete
  pass/fail report under a new path, and keep the frozen Golden, Holdout seal,
  Active pointer, and activation audits unchanged.
- **Base:** all 50 cases return `INSUFFICIENT_EVIDENCE` with zero Citation or
  leakage, but a cold-start latency outlier fails the diagnostic performance
  budget; retain the failing report rather than reclassifying it as a pass.
- **Bad:** add synthetic no-answer cases to the already frozen Golden, reuse a
  Holdout flag/seal, overwrite a failing supplemental report, loosen gates
  after observing the result, or use the diagnostic as Promotion evidence.
- **Good:** bind an intact latency-only failed supplemental report, run the
  fixed four-case Cold phase and then the fixed four-case Warm phase in one new
  helper process, record both phases plus the source hash under a new path, and
  leave every lifecycle authority unchanged.
- **Base:** Cold exceeds 1000ms while Warm is within budget and every
  correctness/security check passes; report `cold_start_effect_observed`
  without reclassifying the 50-case source report.
- **Bad:** interleave Cold and Warm cases, pick faster cases after observing
  results, accept a hand-edited source summary, overwrite either artifact,
  treat latency as permission to ignore a false answer, or warm the process and
  rerun the canonical report without a separately approved SLA policy.

## 6. Tests Required

The disposable drill must assert:

1. all current production migrations apply and remain unchanged afterward;
   the embedded checksum for migration `050` equals the live manifest before
   any later migration is attempted;
2. idempotent backfill count/identity/content/hash postconditions;
3. exact identifiers, paths, phrases, Chinese lexical/bigram recall, semantic
   Dense recall, context rewrite, cross-collection selection, and two unrelated
   no-evidence cases;
4. repeated BM25/Dense and original/rewrite RRF ordering is deterministic;
5. EXPLAIN uses the intended BM25 and HNSW indexes;
6. production roles lack access while `rag_replay_operator` executes live;
7. a tombstone is immediately invisible without destroying rollback data;
8. G18.4-only rollback retains G18.3, final rollback retains `REAL[]`, and all
   disposable containers/volumes are removed.
9. The PG16-compatible profile reader is row-for-row identical to the legacy
   reader at the default pointer; runtime roles cannot mutate the pointer;
   unavailable/conflicting transitions and non-legacy migration rollback fail
   without partial state changes.
10. The PG17 candidate rejects partial backfill, activates only after exact
    readiness, returns reference-only results under both runtime roles,
    survives restart, rejects active-profile rollback, and restores exact
    legacy parity before removing PG17 objects.
11. Two concurrent active-generation head publications populate both
    projections atomically, become immediately query-visible, replay with zero
    inserts, retain physical rows after deletion, disappear from authorized
    reads immediately, and survive restart/rollback checks.
12. A partially indexed building generation receives physical rows but remains
    absent from the active reader; attempted corpus-head cutover is atomic and
    rejected. Complete publication permits promotion, serves only target-
    generation references, and permits exact rollback while retaining both
    generations' physical rows.
13. A 4096-child publication and 30 profiled reads pass the fixed single-server
    backfill/latency/storage/memory gates; restart and a checksummed logical
    backup/restore preserve exact active/physical rows, profile state, reader
    behavior, functions, and role boundaries.
14. An owned live PG16 dump restores into isolated PG17, the embedded runner
    applies `036 -> 037 -> 038`, migration `038` creates both exact extension
    versions, current live authority backfills, runtime roles read references
    without direct projection access, active down fails atomically, and
    controlled down/re-up/restart preserves rollback anchors.
15. Production Compose renders the immutable PG17 image, fresh
    `data/postgres17` bind mount, 1 GiB / 2 CPU envelope, and reviewed PostgreSQL
    settings without a production build path; the old PG16 directory remains
    intact. Live verification proves migration `038`, `11/11/11` readiness,
    dedicated API/Worker sessions, reference-only reads, MinIO object parity,
    direct/proxied health, and restart/reconnect behavior.
16. Under the real `neo_chat_api -> go_api_runtime` boundary, upload/bind,
    source-object fetch, publication, profiled BM25/Dense retrieval, citation,
    deletion invisibility, and fixture cleanup succeed while direct privileges
    on every internal projection relation remain false.
17. In one real-provider conversation, first ask a fixture-backed question and
    prove a legitimate `[K1]`; then ask an unrelated no-evidence question and
    prove the terminal SSE message, persisted reload, and frontend mapping all
    contain no `[K#]`, `citationCount` is zero, and no Knowledge citation card
    exists.
18. Migration `043` returns complete current-authority Child and Parent bodies
    plus frozen token counts, preserves Child citation hashes/locator, rejects
    stale or unauthorized references, and restores the previous signature on
    down migration.
19. Parent expansion admits at most two distinct high-confidence Parents,
    preserves every matched Child, and cannot exceed the shared selected-model
    retrieval budget. Mixed Knowledge/Web proofs must enforce the 40% retrieval
    cap, 512-token reserve, and 60/40 split.
20. Migration `044` proves `go_api_runtime` cannot mutate generation state,
    `rag_replay_operator` can execute only the bounded lifecycle, verification
    does not activate, activation requires the exact gate-report hash and
    operator identity, and the audit row is immutable.
21. Migration `045` proves a narrower Child locator may differ from its Parent,
    the Candidate still verifies, hydration returns the Child Search locator,
    malformed or lineage-substituted Search locators fail closed, and down
    restores the `044` function bodies without restoring API mutation grants.
22. Migration `046` proves only `rag_replay_operator` may execute audited
    Candidate abandonment, direct failure is unavailable to both runtime roles,
    the immutable audit binds Candidate/manifest/head/operator/reason, exact
    replay is idempotent, and conflicting replay fails without moving Active.
23. The Candidate-only v2 promotion evaluator rejects 499 cases, non-60/20/20 splits, any
    critical slice or table-exact cohort below 50, draft/unreviewed cases,
    frozen-hash drift, missing/repeated Holdout runs, stale Candidate bindings,
    any absolute aggregate/slice gate failure, non-finite or extra report
    fields, and any Citation locator/provenance/cell-lineage rate below 100%.
    Its passing report binds the exact raw Golden/Candidate hashes, contains no
    Active comparison, and never activates a Generation.
24. Parser/projection tests render observed nested MinerU Table HTML without
    retaining markup, reject malformed tables, preserve PPTX `slide_shape` and
    XLSX `sheet_cell` authority ahead of XML lines, and a live replay allocates
    only after old Workers stop. Any mixed-image Candidate is audited as
    abandoned and replaced without moving Active.
25. In an isolated pre-`050` schema, migration `049` applies from a fresh PG17
    database, rolls back cleanly to `048`, reapplies with a 64-character
    checksum, and refuses down atomically
    after a fake BGE Index Profile exists. Integration must prove legacy reader
    compatibility, Jina Active routing, BGE Active routing, pointer/Profile
    race retry, same-Profile BM25 fallback, immutable-evidence rerank routing,
    fenced diagnostics, pointer rollback rejection, exact provider status/UI
    enumeration, seeded Jina-consent backfill with identical decision/create/
    update timestamps, and zero automatic Activation or Holdout.
26. PostgreSQL integration tests that create a per-test schema receive a blank
    test database, not a database already migrated by the CLI. Migration `038`
    installs relocatable `vector`/`pg_textsearch` extensions into the current
    isolated schema. Preinstalling them into `public` makes the runner's strict
    `<test_schema>, pg_catalog, pg_temp` path unable to resolve `vector` and is
    an invalid test baseline.
27. Migration `050` applies over an existing Jina Active/BGE Candidate state,
    clears the Jina encrypted secret and connection attestation, soft-deletes
    the provider config, deletes the Web Reader registry row, preserves the
    Jina Active row for BM25-only transition service, preserves Candidate 8 as
    `verified/ready`, and rejects Jina reactivation. Its irreversible down path
    fails without deleting the migration record. Unit/live proofs show zero
    Jina decrypt and HTTP calls, including custom IDs and redirects targeting
    `jina.ai` or any subdomain.
28. Frozen Holdout units prove ordinary preflight still rejects `holdout`, a
    wrong run ID or incomplete/drifted Development/Validation report fails
    before sealing, the seal precedes every provider call, exactly 500 unique
    observations retain Golden order, and existing seal/output paths are never
    overwritten. The live replay creates one `ordinal=1` seal, evaluates the
    exact report hash, rejects a repeated invocation, and leaves Active,
    activation-gate hash, and activation audits unchanged.
29. Supplemental no-answer units prove all three capture modes are mutually
    exclusive, exact 50/25/25/10-per-format coverage is mandatory, imported
    filename collisions and every binding/profile/revision drift fail before
    capture, failing answer/Citation/leakage observations fail their slices,
    output is never overwritten, and concurrent results retain suite order. A
    live BGE-only replay preserves its complete report even on gate failure and
    leaves the frozen Holdout, Active sequence, activation hash, and activation
    audit count unchanged.
30. Supplemental latency units prove the source flag requires the same
    supplemental suite mode, source report observations are strictly
    re-summarized, source hash/binding/non-latency drift and concurrency other
    than four fail before capture, and selection is the first two Chinese cases
    across PDF/DOCX/PPTX/XLSX. They assert two non-overlapping, stable-order
    four-case phases execute Cold completely before Warm, every conclusion and
    P95 delta is recomputed, non-latency failures fail diagnostic integrity,
    and an existing output is byte-for-byte preserved. The live replay records
    `promotionEvidence=false`, changes no Holdout/Active/activation state, and
    makes no Jina request.
31. Citation display units prove reauthorized source-name projection, every
    supported canonical locator mapping, one-based conversion, A1 and bound
    validation, legacy display compatibility, localized fallback, hostile or
    malformed omission, and the invariant that UUIDs, hashes, opaque sheet/
    OOXML data, and serialized raw locator JSON are never rendered. Existing
    Direct, Knowledge, Web, and Both composition tests must remain green.

After the drill, run `go vet ./...`, `go test ./...`, and the frozen G18
evaluator.

## 7. Wrong vs Correct

### Wrong

```sql
-- `0` is no match, not a useful probability score.
ORDER BY bm25_text <@> p_query
LIMIT 20;
```

This can return unrelated zero-score rows and does not bind the intended index
or current authority.

### Correct

```sql
WITH probe AS (
  SELECT child_chunk_id, content_hash,
    bm25_text <@> to_bm25query(p_query, 'reviewed_bm25_index') AS score
  FROM bm25_shadow
  WHERE bm25_text <@> to_bm25query(
    p_query,
    'reviewed_bm25_index'
  ) < 0
  ORDER BY score, child_chunk_id
  LIMIT p_oversample
)
SELECT probe.child_chunk_id, probe.score
FROM probe
CROSS JOIN LATERAL (
  SELECT source.child_chunk_id
  FROM current_authority source
  WHERE source.child_chunk_id = probe.child_chunk_id
    AND source.content_hash = probe.content_hash
  OFFSET 0
) authorized
ORDER BY probe.score, probe.child_chunk_id;
```

The index is explicit, unrelated zeros are rejected, ordering is deterministic,
and current authority remains the final visibility gate without being
decorrelated into a corpus-wide join.

### Turn-scoped citation authority

Wrong:

```go
// Marker-looking model prose is not citation authority.
completedDecision := decision.completed(providerContent)
persist(providerContent)
```

Correct:

```go
completedContent := reconcileProviderSourceMarkers(
	providerContent,
	decision,
	webResult,
)
completedDecision := decision.completed(completedContent)
persist(completedContent)
```

Only markers issued from the current turn's authoritative evidence survive.
Historical assistant markers are removed before provider context assembly, and
the frontend repeats the Knowledge-side check when loading legacy messages.

### Citation display projection boundary

Wrong:

```tsx
const source = citation.documentId;
const location = JSON.stringify(citation.locator);
return `${citation.marker} · ${source} · ${location}`;
```

Correct:

```tsx
const normalized = normalizeMessageKnowledgeMetadata(untrustedMetadata);
return formatKnowledgeCitationTitle(
  normalized.citations[0],
  t("citationSourceFallback"),
  localizedLocationLabels,
);
```

The backend emits only a bounded display projection from already reauthorized
evidence. The frontend validates it again, keeps raw locator authority out of
presentation, and degrades old or unsupported Citations without exposing
internal identifiers.

### API projection write boundary

Wrong:

```sql
GRANT SELECT, INSERT, UPDATE ON knowledge_document_materializations
TO go_api_runtime;
```

Correct:

```sql
GRANT EXECUTE ON FUNCTION knowledge_allocate_parse_materialization(
  UUID, UUID, UUID
) TO go_api_runtime;
```

The SECURITY DEFINER function pins the trusted schema path, derives mutable
authority fields inside PostgreSQL, and exposes only the binding required by
Go. Its owner remains `rag_projection_owner`; `PUBLIC` retains no EXECUTE.

### Candidate Worker image fence

Wrong:

```text
generation-begin -> stop old Worker -> start new Worker
```

The old Worker may lease a newly allocated job before shutdown, mixing image
revisions inside immutable Candidate evidence.

Correct:

```text
stop all Candidate Workers -> pin image -> generation-begin -> start pinned Worker
```

If the fence is violated, use audited `generation-abandon` and allocate a new
sequence. Never rewrite or certify the mixed Candidate in place.

### Retrieval vector-space binding

Wrong:

```go
// Equal dimensions do not imply equal vector semantics.
if len(queryVector) == binding.EmbeddingDimensions {
	return fetchHybrid(queryVector)
}
```

Correct:

```go
profile, err := ragproviders.ResolveRetrievalProfile(
	ragproviders.RetrievalProfileID(binding.RetrievalProfileID),
)
if err != nil ||
	profile.ProviderID != binding.ProviderID ||
	profile.EmbeddingModelID != binding.EmbeddingModelID ||
	profile.EmbeddingDimensions != binding.EmbeddingDimensions ||
	profile.RerankModelID != binding.RerankModelID {
	return nil, knowledge.ErrRetrievalProfileChanged
}
```

The provider/model/profile identity is the vector-space authority. Dimension
validation is necessary for wire integrity but never sufficient for search.

### Frozen Holdout execution boundary

Wrong:

```text
rag-capture -splits holdout -output observations.json
provider calls -> write/replace seal -> evaluate -> activate
```

Correct:

```text
validate frozen inputs + complete Dev/Validation + live Candidate/revisions
-> create exclusive ordinal=1 seal
-> capture exactly 100 Holdout cases
-> atomically publish one ordered 500-case observation set without replacement
-> evaluate
-> stop and request separate approval for the exact gate-report SHA-256
```

Ordinary preflight never accepts Holdout. The durable seal is the first
irreversible execution action, and evaluator success is not pointer authority.

### Supplemental no-answer evidence boundary

Wrong:

```text
append generated negatives to frozen Golden
-> rerun Holdout or overwrite its report
-> loosen failed latency/correctness gates
-> use the new result for Activation
```

Correct:

```text
validate an independently versioned 50-case suite against frozen hashes
and live Candidate/revisions
-> run SiliconFlow BGE Candidate capture only
-> publish one exclusive promotionEvidence=false report, pass or fail
-> preserve frozen Holdout and lifecycle state unchanged
```

Supplemental coverage closes a diagnostic blind spot; it does not retroactively
change frozen evidence or create pointer authority.

### Supplemental cold/warm latency diagnostic boundary

Wrong:

```text
edit the failed report summary or choose ad hoc fast cases
-> interleave Cold and Warm requests
-> overwrite the failed report with passed=true
-> use the diagnostic to activate
```

Correct:

```text
strictly reload and recompute the intact latency-only failed report
-> bind its computed SHA-256 plus the same suite/Candidate/profile/revisions
-> in one new helper process run fixed Cold[format case 1] completely
-> then run fixed Warm[format case 2]
-> exclusively publish promotionEvidence=false under a new path
-> preserve the canonical report, frozen Holdout, and lifecycle state
```

The paired phases can prove a new-process cold-start effect is observed, but
they do not identify whether connection-pool initialization, HTTP/TLS setup, or
database cache dominates. That attribution requires a separately designed
component-isolation experiment.

For the current owner-approved policy, use Warm P95 for the `<=1000ms`
steady-state decision and retain Cold P95 as visible non-blocking telemetry.
The immutable 50-case source report keeps its original `passed=false` result;
policy clarification is not permission to overwrite, rerun, or reclassify it.

### Applied migration byte drift

Wrong:

```sql
UPDATE schema_migrations
SET checksum = '<checksum of reformatted source>'
WHERE version = 50;
```

Correct:

```text
prove the exact applied up/down bytes
-> restore the embedded source byte-for-byte
-> assert the live checksum in a regression test
-> rerun the normal migration command
```

The live manifest is evidence of what executed, not mutable metadata for
making edited source appear valid.
