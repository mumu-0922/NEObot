# G18.3 pgvector shadow projection

This disposable PG17-only harness validates a generation/profile-bound
`vector(1024)` projection beside the current Jina `REAL[]` projection. It does
not call Jina or change the production reader.

## Run

From any directory under the repository checkout:

```bash
mm-chat/scripts/run-g18-pgvector-shadow-drill.sh
```

The script builds the pinned G18 PostgreSQL image, starts a unique internal
Compose project, applies the current `1–37` production migrations, loads only
synthetic rows, and proves:

- transactional and idempotent conversion of compatible Jina v4 vectors;
- exact cosine order and real HNSW index execution;
- generation, profile, identity, hash, norm, and collection isolation;
- zero/non-finite source rejection without partial shadow writes;
- immediate tombstone invisibility while rollback data remains intact;
- shadow down migration without changing the current migration manifest or
  `REAL[]` production reader.

The successful report path is printed under
`/tmp/mm-chat-g18-pgvector-shadow.*`. The exit trap removes all project
containers, networks, and volumes.

## Files

- `00-shadow-schema.up.sql`: shadow view, table, validation/backfill functions,
  immutable triggers, exact-supporting scope index, and HNSW index.
- `00-shadow-schema.down.sql`: shadow-only rollback.
- `10-shadow-fixture.sql`: two-collection synthetic Jina vectors.
- `20-verify-shadow.sql`: positive, negative, ACL, deletion, exact, and HNSW
  proofs.
- `30-verify-rollback.sql`: legacy reader/data preservation proof.

See [`DESIGN.md`](./DESIGN.md) for the production boundary and promotion plan.
