# G18.2 PostgreSQL 17 restore drill design

## Goal and non-goals

This harness proves the PostgreSQL major-version transition and extension
runtime on disposable synthetic state before any production migration or
shadow retrieval DDL is allowed.

It does not read production data, migrate `mm-chat/data/postgres`, publish host
ports, call providers, cut over the retrieval reader, or model an in-place
PostgreSQL downgrade.

## Architecture

```text
current migration CLI
       |
fresh PG16.13 + synthetic authority/projection graph
       |
       +-- custom logical backup + checksum
                    |                 |
                    v                 v
              fresh PG17.10      fresh PG16 rollback
              + extensions       + migration no-op
              + migration no-op  + state verification
              + state verification
```

The Compose file provides isolated PG16, PG17, and migration services. SQL
fixtures create and verify only deterministic synthetic authority, object
reference, generation, materialization, Parent/Child, and search-projection
state. `run-g18-postgres17-restore-drill.sh` owns orchestration, checksums,
readiness, guard validation, reports, and cleanup.

## Decisions

- Use logical backup/restore across major versions because PostgreSQL 16 and 17
  physical data directories are incompatible. Both target databases are fresh.
- Restore rollback separately from the same preserved backup. An upgraded data
  directory is not a downgrade artifact.
- Use an internal network with no host ports because only database-to-CLI
  traffic is required.
- Use `trust` authentication only inside the disposable network to avoid
  embedding a synthetic password. It must never become a production default.
- Seed a synthetic full graph rather than cloning production. This proves
  reference preservation without private data; production-scale checks remain
  deferred.
- Use a unique Compose project and exit-trap cleanup to avoid collision and
  disposable-volume leaks. Reports survive only in the printed `/tmp` path.

## Known limits

- The drill proves schema/state compatibility and real extension queries, not
  production data size, migration duration, or resource budgets.
- Object-store blobs are not copied; object keys and hashes are verified as
  database references.
- The PG17 target activates extensions after restoring the extension-free PG16
  backup. Shadow pgvector and BM25 projection DDL belongs to later G18 groups.
- A production upgrade still requires its own backup checksum, restore proof,
  maintenance window, observation period, and rollback decision.

## Security and trust boundaries

- The harness does not load a project `.env` file or accept provider secrets.
- The Compose network is `internal: true`; no service publishes a port.
- All database volumes are project-scoped and removed by the exit trap.
- A pre-start entrypoint guard must reject non-17 `PG_VERSION` with exit `78`.
- Reports contain synthetic rows and operational logs only. They must not be
  populated with production dumps or committed as fixtures.

## Change history

- 2026-07-22: initial G18.2 restore, rollback, extension, and cleanup contract.
