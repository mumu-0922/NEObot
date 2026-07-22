# G18 BM25 and pgvector Retrieval Process

## 2026-07-22 — Intake and architecture lock

The current runtime was traced before changing storage. Migration `026` uses
PostgreSQL FTS `ts_rank` with exact-term, phrase, and bounded CJK-bigram boosts;
it is not BM25. Migration `027` stores Jina v4 1024-dimensional vectors as
`REAL[]`, expands both arrays during cosine calculation, applies a Dense `0.48`
gate for queries of at least eight characters, and fuses Lexical and Dense
ranks through RRF. Go fuses original and rewritten query lanes, hydrates under
current authority, calls `jina-reranker-v3`, and admits scores `>= 0.0`.

The target is PostgreSQL 17 with `pg_textsearch` plus pgvector, kept behind
shadow/dual-read gates. Existing Jina v4 vectors remain the first production
generation. BGE-M3 is deferred to an isolated shadow benchmark because equal
dimensions do not imply a compatible vector space.

Historical live evidence also ruled out blindly raising the global reranker
threshold. An unused candidate scored `0.11554055`, while useful cited
candidates scored `0.09505704`, `0.12736949`, and `0.21525481`. G18.1 therefore
starts with a synthetic versioned Golden Set and deterministic evaluator. Any
policy promotion must retain required cases and reject unrelated negatives;
one scalar score is not treated as a relevance probability.

Next: implement the G18.1 evaluator and fail-closed reranker degradation,
capture the current-engine synthetic baseline, run focused/full Go checks, and
commit G18.1 before touching the database image.

## 2026-07-22 — G18.1 evaluation and strict degradation closure

Added `internal/rageval` plus `cmd/rag-eval`. The closed JSON contracts accept
only synthetic queries, collection/evidence aliases, ranked lane observations,
final evidence identifiers, citation identifiers, no-evidence decisions, and
latency. Unknown fields are rejected, so source text cannot silently enter a
committed observation. The evaluator deterministically reports per-lane recall,
final-context precision, negative false-citation rate, no-evidence accuracy,
nearest-rank P95 latency, case failures, and a promotion result.

The versioned `g18-synthetic-retrieval-v1` fixture covers exact identifiers,
Chinese lexical and semantic questions, a contextual rewrite, two selected
collections, and Chinese/English unrelated negatives. Its current-engine
observation is recorded in `baseline-current-v1.json`; it contains only
synthetic aliases and scores, never live UUIDs, source text, or credentials.

Configured reranking now fails closed before Knowledge publication. Missing
governance, partial configuration, provider failure, or a malformed result
returns no Knowledge evidence, so the normal Model/Web path may continue but
unreranked RRF candidates cannot mint citations. The relevance policy is named
`g18-jina-reranker-v3-golden-v1` and requires a complete finite rerank result.
Its numeric floor intentionally remains `0.0`: historical useful/unused scores
overlap, and the live frozen negative set passes without a blind threshold
increase. The effective policy is the Golden gate plus mandatory rerank,
fail-closed degradation, and the existing terminal answer-marker citation
filter—not an assumption that a Jina score is a probability.

Live disposable baseline:

```text
engine profile                              ts_rank + REAL[] cosine + Jina
Golden cases                                7 (5 relevant / 2 unrelated)
Lexical / Dense / rewrite-Dense recall      1.0 / 1.0 / 1.0
final-context precision                     1.0
negative false-citation rate                0
no-evidence accuracy                        1.0
Knowledge-stage P95                         25.402s
contextual rewrite                          answered / queryRewritten=true / [K1]
cross-collection                            answered / 2 selected / [K1],[K2]
weather / cooking negatives                 no_evidence / no_evidence
```

The cooking negative deliberately produced one low lexical candidate, then
ended as `no_evidence`; this proves candidate recall alone is not citation
authority. The slowest case was contextual query rewrite, not a PostgreSQL
storage cutover, so later BM25/pgvector comparisons must separate provider and
database latency when diagnosing regressions.

Verification:

```text
focused rageval / CLI / chat tests           passed
go vet ./...                                 passed
go test ./...                                passed
recorded baseline evaluator gate             passed
Compose backend rebuild                      passed
backend database/redis/storage readiness     ready / ready / ready
temporary active collections/documents       0 / 0
temporary ready search projections           0
temporary active conversations               0
local live-probe artifacts                    deleted
```

Rollback: reverting the G18.1 code restores the earlier degraded RRF fallback;
there is no schema change. The Golden Set, baseline, and evaluator are inert
offline artifacts. Database cleanup touched only the deleted disposable
synthetic fixtures and left production Knowledge untouched.

Next: commit G18.1 alone, then start G18.2 with a digest-pinned PostgreSQL 17
extension image and disposable PostgreSQL 16 backup/restore drill. Do not point
PostgreSQL 17 at `data/postgres`.
