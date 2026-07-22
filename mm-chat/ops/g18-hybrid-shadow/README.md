# G18.4 BM25 and hybrid shadow retrieval

This disposable PG17-only module validates true `pg_textsearch` BM25 beside the
G18.3 `vector(1024)` projection. It remains invisible to the production Go API
and RAG worker.

## Run

From any directory under the repository checkout:

```bash
mm-chat/scripts/run-g18-hybrid-shadow-drill.sh
```

The harness builds the pinned PostgreSQL 17 image, applies the current `1–36`
production migrations, loads only synthetic rows, and proves:

- current-generation, published, ready BM25 source admission;
- identifiers, paths, phrases, and bounded CJK-bigram recall;
- indexed pgvector Dense recall and deterministic `k=60` RRF;
- original/standalone-rewrite query-lane fusion;
- selected-collection isolation and immediate deletion invisibility;
- reference/rank/score-only diagnostics with no source text;
- shadow-only rollback with the legacy `REAL[]` reader preserved.

A successful report is retained under `/tmp/mm-chat-g18-hybrid-shadow.*`. The
exit trap removes every project-scoped container, network, and volume.

## Files

- `00-shadow-schema.up.sql`: text normalization, authority source view, BM25
  projection/index, verified backfill, and hybrid diagnostic function.
- `00-shadow-schema.down.sql`: G18.4-only rollback.
- `10-shadow-fixture.sql`: two-collection synthetic lexical fixture.
- `20-verify-shadow.sql`: backfill, Golden, plan, security, deletion, and RRF
  proofs.
- `30-verify-rollback.sql`: G18.4/G18.3/legacy rollback-state checks.

See [`DESIGN.md`](./DESIGN.md) for the trust boundary and promotion limits.
