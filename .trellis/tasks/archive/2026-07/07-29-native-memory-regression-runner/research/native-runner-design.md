# Native Memory regression runner design research

## Question

How should Neo Chat produce truthful v1 lexical and native v2 hybrid
observations for the protected 500-case machine-reviewed regression corpus
without changing production reader authority or exposing fixture plaintext?

## Existing runtime facts

- `cmd/memory-eval` only validates and scores an already assembled observation
  set. It does not execute a reader.
- `usermemory.Service.SearchRelevant` is the current Global-only v1 Top-5
  prompt reader.
- `SearchRelevantWithHybridShadow` executes the same v1 reader, then the
  production migration-059 Exact/CJK BM25/BGE-M3/RRF/rerank path. It returns
  only v1 items plus a bounded summary; final candidate IDs remain transient
  or content-free in PostgreSQL diagnostics.
- `HybridShadowRepository.PrepareHybridShadow` receives the ordered RRF Top-20
  candidates, while `RecordHybridShadow` receives reranked and final Top-5
  IDs. A decorator can capture those typed values without weakening the
  production content-free database contract.
- The v2 regression fixture has 500 synthetic fixtures, 625 logical Memories,
  all three scopes, and active/superseded/deleted/rejected/out-of-scope states.
  It is hash-bound and `promotionEligible=false`.
- The fixed production retrieval profile is BGE-M3 1024 dimensions,
  RRF(60), BGE reranker, candidate limit 20, final limit 5, target 600 tokens,
  maximum 900 tokens, and hard cutoff 2 seconds.
- The evaluator derives authority leaks from every ID surface, including
  Provider-sent IDs. A runner must not hide filtered or failed surfaces.

## Approaches considered

### A. Drive the live chat API and read shadow tables

Rejected. This is closest to a deployed request but would write synthetic
messages and observations into the live database. Runtime tables intentionally
do not expose all transient candidate/final IDs to an ordinary operator, so a
capture would also require broader database authority.

### B. Isolated PostgreSQL plus production reader decorators (selected)

Run all current migrations in a random disposable Compose project, seed only
the protected synthetic fixture, build real BGE projections, and invoke the
production `usermemory` services under an API-role connection. Decorate the
hybrid repository/provider in-process to capture candidate, final, fallback,
latency, token, and Provider-egress ID surfaces before they are reduced to
content-free durable diagnostics.

Benefits:

- exercises the actual Go normalization, PostgreSQL scope/authority filters,
  exact/BM25/vector/RRF logic, provider gateway, reranker, and token budget;
- cannot mutate the live server database or prompt;
- preserves the production least-privilege and content-free observation
  contract;
- supports deterministic fake-provider tests and an explicitly authorized
  live SiliconFlow run through the same interfaces.

Costs:

- needs a fixture seeder and disposable database lifecycle;
- a live candidate run performs external embedding/rerank calls and therefore
  needs an explicit credential and cost basis;
- the reader-quality lane does not evaluate extraction quality.

### C. Reimplement retrieval in an offline in-memory evaluator

Rejected. It would be fast and easy to unit test, but it would measure a copy
of the algorithm rather than the SQL and Go code that runs in Server mode.
Projection, CJK tokenization, scope generation, rerank fallback, and token
budget drift could all pass unnoticed.

## Selected capture semantics

- One run emits two exclusive observation artifacts: baseline
  `native_v1_lexical` and candidate `native_v2_hybrid`.
- v1 candidate/final/injected IDs are the actual production Top-5 because the
  current reader has no distinct Top-20 candidate surface.
- v2 candidate IDs are the actual authorized RRF Top-20; final IDs are the
  actual reranked/budgeted Top-5.
- v2 `injectedMemoryIds` mirrors final IDs only as an explicit counterfactual
  offline injection surface for current-fact/false-injection scoring. It does
  not change any prompt or active reader.
- `persistedMemoryIds` stays empty because this is retrieval-only capture;
  fixture seeding is not extraction output.
- `providerSentMemoryIds` contains exactly the authorized candidate documents
  passed to rerank. Query embeddings contain no Memory plaintext IDs.
- Rejected fixture states are not made canonical. Deleted and out-of-scope
  states are materialized only enough to test current retrieval authority.
- Capture configuration hashes bind corpus/audit/fixture bytes, binary reader
  version, profile/model IDs, limits, timeout, fixture-state mapping, and cost
  basis.

## Isolation and credential boundary

- The operator script creates a random Compose project with PostgreSQL 17,
  `pg_textsearch 1.3.1`, and `pgvector 0.8.5`; no port is published.
- PostgreSQL and every volume/network/container are destroyed on success,
  failure, or signal. Only exclusive JSON artifacts survive.
- The capture command refuses databases without an exact ephemeral benchmark
  marker and expected database name.
- Offline tests use a deterministic fake provider and make zero network calls.
- A live run accepts a fresh SiliconFlow credential through a temporary
  mode-0600 file/stdin boundary, never argv, report, Git, fixture output, or
  long-lived environment. It does not discover or decrypt the production
  provider vault.
- Provider costs must be explicit same-unit microunits from a versioned cost
  basis. Missing/invalid cost authority fails closed; the runner never invents
  a zero or nominal cost to make the evaluator pass.

## Failure and publication behavior

- Validate all four protected artifact hashes before starting external calls.
- Preserve corpus order exactly; each case gets a terminal observation even
  when hybrid falls back or reaches cutoff.
- Produce artifacts in a mode-0700 temporary directory with mode-0600 files.
- Validate them with `memory-eval` before exclusive publication. Partial or
  failed runs are not promoted to final filenames.
- Reports remain `machine_reviewed_regression`, `regression_only`, and
  `promotionEligible=false`; no command can call formal Golden admission or
  change a retrieval profile pointer.

## Extension points deliberately preserved

- The capture interface can later add a clean human Golden input without
  sharing its admission state.
- Metering and capture decorators can support a future L2/L3 candidate profile
  without changing the v1/v2 schema.
- A deterministic fake provider supports replay and CI while the live provider
  remains an explicit operator action.

